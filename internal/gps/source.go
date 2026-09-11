package gps

import (
	"context"
	"io"
	"time"
)

// Source is a live position provider. Implementations own their own
// reconnect loop: Start returns nil once the loop is running, even if the
// first connect attempt failed (status reflects that). Fixes() is closed
// when the context passed to Start is cancelled.
type Source interface {
	Start(ctx context.Context) error
	Fixes() <-chan Fix
	Status() SourceStatus
	Close() error
}

// Reconnect backoff sequence: 1s, 2s, 4s, 8s, 16s, 32s, 60s, 60s… (cap 60s).
// Reset to 1s on every successful connect that then reads at least one line.
const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 60 * time.Second
)

// nextBackoff doubles cur, capped at maxBackoff.
func nextBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > maxBackoff {
		return maxBackoff
	}
	if next <= 0 {
		return initialBackoff
	}
	return next
}

// readDeadline is how long a connected source may go without receiving any
// bytes before it is treated as a dead link and reconnected.
const readDeadline = 20 * time.Second

// errString returns "" for a nil error, avoiding "<nil>" in status text.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// runLoop is the shared reconnect skeleton used by both NMEASource and
// GPSDSource: dial, mark connected, read until EOF/err/ctx-done, mark
// disconnected, back off, repeat. It never busy-polls — every wait is a
// blocking select on a timer or ctx.Done().
func runLoop[C io.Closer](ctx context.Context, dial func() (C, error), read func(context.Context, C) error, setStatus func(connected bool, errMsg string)) {
	backoff := initialBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		conn, err := dial()
		if err != nil {
			setStatus(false, err.Error())
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
			continue
		}
		setStatus(true, "")
		backoff = initialBackoff
		err = read(ctx, conn)
		conn.Close()
		if ctx.Err() != nil {
			return
		}
		setStatus(false, errString(err))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = nextBackoff(backoff)
	}
}

// idleWatchdog closes c if kick isn't called within timeout, treating a
// connected-but-silent link as dead. stop must be called once reading ends
// normally, to avoid a stray Close after the fact.
func idleWatchdog(c io.Closer, timeout time.Duration) (kick func(), stop func()) {
	timer := time.AfterFunc(timeout, func() { c.Close() })
	return func() { timer.Reset(timeout) }, func() { timer.Stop() }
}
