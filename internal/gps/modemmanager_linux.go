//go:build linux

package gps

import (
	"context"
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

// sysbusConn is the real mmConn, backed by a private connection to the
// system bus. defaultMMDialer is the only place that opens one.
type sysbusConn struct {
	conn   *dbus.Conn
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	watches []mmWatch
}

type mmWatch struct {
	opts []dbus.MatchOption
	ch   chan *dbus.Signal
}

// defaultMMDialer opens a PRIVATE connection to the system bus.
// dbus.ConnectSystemBus, never dbus.SystemBus(): the latter returns a
// process-global shared *Conn that Wails may also hold, and Close() on it
// would tear down the bus for the whole process.
func defaultMMDialer(ctx context.Context) (mmConn, error) {
	// dbus.WithContext must NOT receive ctx as-is: godbus's newConn does
	// `conn.ctx, conn.cancelCtx = context.WithCancel(conn.ctx)` and spawns a
	// watcher goroutine that calls conn.Close() the instant that context is
	// Done (conn.go). ctx here is the caller's (readLoop's) context, which
	// is cancelled at shutdown BEFORE Close()'s restore() call runs — wiring
	// it straight in would tear the transport down out from under that
	// restore, silently discarding whatever prior Setup mask we owe another
	// client. context.WithoutCancel decouples the underlying *dbus.Conn's
	// lifetime from ctx; Close() (via cancel below) is the only thing that
	// tears it down.
	dialCtx := context.WithoutCancel(ctx)
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(dialCtx))
	if err != nil {
		return nil, fmt.Errorf("modemmanager: connect system bus: %w", err)
	}
	// The conn's lifetime is owned by Close(), not by ctx (which belongs to
	// the caller and will be re-cancelled/replaced across reconnects).
	cctx, cancel := context.WithCancel(dialCtx)
	return &sysbusConn{conn: conn, ctx: cctx, cancel: cancel}, nil
}

func (c *sysbusConn) GetManagedObjects(ctx context.Context) (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error) {
	var out map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	obj := c.conn.Object(mmBusName, mmRootPath)
	if err := obj.CallWithContext(ctx, ifaceObjMgr+".GetManagedObjects", 0).Store(&out); err != nil {
		return nil, fmt.Errorf("modemmanager: GetManagedObjects: %w", err)
	}
	return out, nil
}

func (c *sysbusConn) GetProperty(ctx context.Context, path dbus.ObjectPath, iface, prop string) (dbus.Variant, error) {
	var v dbus.Variant
	obj := c.conn.Object(mmBusName, path)
	if err := obj.CallWithContext(ctx, ifaceProps+".Get", 0, iface, prop).Store(&v); err != nil {
		return dbus.Variant{}, fmt.Errorf("modemmanager: get %s.%s: %w", iface, prop, err)
	}
	return v, nil
}

func (c *sysbusConn) Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) error {
	obj := c.conn.Object(mmBusName, path)
	if err := obj.CallWithContext(ctx, method, 0, args...).Store(); err != nil {
		return fmt.Errorf("modemmanager: %s: %w", method, err)
	}
	return nil
}

func (c *sysbusConn) WatchProperties(ctx context.Context, path dbus.ObjectPath) (<-chan mmPropsChanged, error) {
	opts := []dbus.MatchOption{
		dbus.WithMatchSender(mmBusName),
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(ifaceProps),
		dbus.WithMatchMember("PropertiesChanged"),
	}
	if err := c.conn.AddMatchSignalContext(ctx, opts...); err != nil {
		return nil, fmt.Errorf("modemmanager: AddMatch: %w", err)
	}

	raw := make(chan *dbus.Signal, 32)
	c.conn.Signal(raw)

	c.mu.Lock()
	c.watches = append(c.watches, mmWatch{opts: opts, ch: raw})
	c.mu.Unlock()

	out := make(chan mmPropsChanged, 8)
	go func() {
		defer close(out)
		for {
			var sig *dbus.Signal
			var ok bool
			select {
			case <-c.ctx.Done():
				return
			case <-ctx.Done():
				return
			case sig, ok = <-raw:
				if !ok {
					return
				}
			}
			if sig == nil || sig.Name != ifaceProps+".PropertiesChanged" || len(sig.Body) < 2 {
				continue
			}
			iface, ifaceOK := sig.Body[0].(string)
			changed, changedOK := sig.Body[1].(map[string]dbus.Variant)
			if !ifaceOK || !changedOK {
				continue
			}
			var invalid []string
			if len(sig.Body) >= 3 {
				invalid, _ = sig.Body[2].([]string)
			}
			ev := mmPropsChanged{Interface: iface, Changed: changed, Invalidated: invalid}
			select {
			case out <- ev:
			default:
				// Never block the shared signal pump: godbus's default
				// handler spawns a goroutine per undeliverable signal, so a
				// slow consumer here would leak goroutines and reorder
				// signals.
			}
		}
	}()
	return out, nil
}

func (c *sysbusConn) Close() error {
	c.cancel()
	c.mu.Lock()
	ws := c.watches
	c.watches = nil
	c.mu.Unlock()
	for _, w := range ws {
		// RemoveMatchSignal's error is ignored: the bus may already be
		// gone. RemoveSignal does NOT close w.ch by itself — only
		// conn.Close() below does — so the watch goroutine's own ctx.Done()
		// select (not a range over the channel) is what lets it exit.
		_ = c.conn.RemoveMatchSignal(w.opts...)
		c.conn.RemoveSignal(w.ch)
	}
	return c.conn.Close()
}
