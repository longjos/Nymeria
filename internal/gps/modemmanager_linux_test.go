//go:build linux

package gps

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestDefaultMMDialerSurvivesCallerContextCancel guards against wiring the
// caller's (readLoop's) context straight into dbus.WithContext. godbus's
// newConn does `conn.ctx, conn.cancelCtx = context.WithCancel(conn.ctx)`
// and spawns a watcher goroutine that calls conn.Close() the moment that
// context is Done (conn.go). MMSource.Start hands the source's own ctx to
// s.dial, and that ctx is cancelled on every shutdown BEFORE restore()
// runs — so if defaultMMDialer passed it straight through, the bus
// connection would already be torn down by the time restore() tries to
// issue its own Location.Setup(prior, priorSignals) call, silently
// discarding the mask restore this feature exists to perform.
//
// Requires a live system bus (present on this workstation, and in any CI
// container running dbus-daemon); it skips rather than fails if none is
// reachable, so it never flakes a bus-less box. It does NOT require
// ModemManager to be installed or running.
func TestDefaultMMDialerSurvivesCallerContextCancel(t *testing.T) {
	callerCtx, cancel := context.WithCancel(context.Background())
	c, err := defaultMMDialer(callerCtx)
	if err != nil {
		t.Skipf("no system bus reachable to dial: %v", err)
	}
	defer c.Close()

	// Cancel the CALLER's context — exactly what happens to readLoop's ctx
	// on every MMSource shutdown, before restore() is invoked.
	cancel()
	// Give godbus's watcher goroutine a chance to react, if it's wired to
	// (that reaction is precisely the bug: it would call conn.Close()).
	time.Sleep(100 * time.Millisecond)

	// A subsequent call using a FRESH, independent context — exactly what
	// restore() does via its own short detached context — must still reach
	// the bus. ModemManager itself is not installed on this workstation, so
	// the call is expected to fail, but with a real D-Bus response (the
	// daemon saying it doesn't know that service), never a torn-down local
	// transport.
	_, err = c.GetManagedObjects(context.Background())
	if err == nil {
		return // some other conflicting answer came back; either way, not a torn-down conn
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "closed") || strings.Contains(lower, "use of closed") {
		t.Fatalf("GetManagedObjects failed as if the bus connection were torn down, after only the CALLER's context was cancelled (godbus ties conn lifetime to dbus.WithContext's context): %v", err)
	}
}
