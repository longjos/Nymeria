package gps

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeSource is a hand-driven Source for manager tests: the test writes
// fixture Fixes onto ch and controls Status() directly.
type fakeSource struct {
	mu       sync.Mutex
	ch       chan Fix
	status   SourceStatus
	started  bool
	closed   bool
	startCtx context.Context
}

func newFakeSource() *fakeSource {
	return &fakeSource{ch: make(chan Fix, 64), status: SourceStatus{Type: "fake"}}
}

func (f *fakeSource) Start(ctx context.Context) error {
	f.mu.Lock()
	f.started = true
	f.startCtx = ctx
	f.mu.Unlock()
	go func() {
		<-ctx.Done()
		close(f.ch)
	}()
	return nil
}

func (f *fakeSource) Fixes() <-chan Fix { return f.ch }

func (f *fakeSource) Status() SourceStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeSource) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeSource) ctxDone() bool {
	f.mu.Lock()
	ctx := f.startCtx
	f.mu.Unlock()
	if ctx == nil {
		return false
	}
	return ctx.Err() != nil
}

func TestManagerDisabledIsInert(t *testing.T) {
	src := newFakeSource()
	m := NewManagerWithSource(Config{Enabled: false}, src)

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := m.Current(); ok {
		t.Error("Current() ok = true, want false when disabled")
	}
	if m.Status().Enabled {
		t.Error("Status().Enabled = true, want false")
	}

	ch, unsub := m.Subscribe()
	defer unsub()
	select {
	case fix := <-ch:
		t.Errorf("unexpected fix on disabled manager: %+v", fix)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestManagerCurrentAndStatus(t *testing.T) {
	src := newFakeSource()
	m := NewManagerWithSource(Config{Enabled: true, Type: "gpsd", StaleAfter: 30 * time.Second}, src)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	fix := Fix{Mode: Mode3D, Lat: 44, Lon: -121, ReceivedAt: time.Now()}
	src.ch <- fix

	deadline := time.Now().Add(time.Second)
	for {
		if _, ok := m.Current(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fix never accepted")
		}
		time.Sleep(5 * time.Millisecond)
	}

	got, ok := m.Current()
	if !ok || got.Lat != 44 {
		t.Fatalf("Current() = %+v, %v", got, ok)
	}

	st := m.Status()
	if st.Fix == nil {
		t.Fatal("Status().Fix = nil, want non-nil")
	}
	if st.AgeMillis == nil || *st.AgeMillis > 200 {
		t.Errorf("AgeMillis = %v, want small", st.AgeMillis)
	}
	if st.Stale {
		t.Error("Stale = true, want false")
	}
}

func TestManagerMinIntervalDrops(t *testing.T) {
	src := newFakeSource()
	m := NewManagerWithSource(Config{Enabled: true, MinInterval: time.Second}, src)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	base := time.Now()
	for i := 0; i < 5; i++ {
		src.ch <- Fix{Mode: Mode3D, Lat: 44, Lon: -121, ReceivedAt: base.Add(time.Duration(i) * 100 * time.Millisecond)}
	}

	accepted := drainSubscriber(t, m, 5)
	if accepted != 1 {
		t.Errorf("accepted = %d, want 1", accepted)
	}
}

func TestManagerModeChangeBypassesMinInterval(t *testing.T) {
	src := newFakeSource()
	m := NewManagerWithSource(Config{Enabled: true, MinInterval: time.Second}, src)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	base := time.Now()
	src.ch <- Fix{Mode: Mode3D, Lat: 44, Lon: -121, ReceivedAt: base}
	src.ch <- Fix{Mode: ModeNoFix, ReceivedAt: base.Add(100 * time.Millisecond)}

	accepted := drainSubscriber(t, m, 2)
	if accepted != 2 {
		t.Errorf("accepted = %d, want 2", accepted)
	}
}

// drainSubscriber subscribes, waits briefly for up to `sent` fixes to flow
// through ingest, and returns how many were actually forwarded.
func drainSubscriber(t *testing.T, m *Manager, sent int) int {
	t.Helper()
	ch, unsub := m.Subscribe()
	defer unsub()

	count := 0
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case <-ch:
			count++
			if count >= sent {
				return count
			}
		case <-timeout:
			return count
		}
	}
}

func TestManagerFreshVsStale(t *testing.T) {
	src := newFakeSource()
	m := NewManagerWithSource(Config{Enabled: true, StaleAfter: 30 * time.Second}, src)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	fixTime := time.Now()
	src.ch <- Fix{Mode: Mode3D, Lat: 1, Lon: 2, ReceivedAt: fixTime}

	deadline := time.Now().Add(time.Second)
	for {
		if _, ok := m.Current(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fix never accepted")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if _, ok := m.Fresh(fixTime.Add(31 * time.Second)); ok {
		t.Error("Fresh() ok = true at T+31s, want false")
	}
	if _, ok := m.Current(); !ok {
		t.Error("Current() ok = false, want true even when stale")
	}
}

func TestManagerSubscribeUnsubscribe(t *testing.T) {
	src := newFakeSource()
	m := NewManagerWithSource(Config{Enabled: true}, src)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ch1, unsub1 := m.Subscribe()
	ch2, unsub2 := m.Subscribe()
	defer unsub2()

	src.ch <- Fix{Mode: Mode3D, Lat: 1, Lon: 2, ReceivedAt: time.Now()}

	for _, ch := range []<-chan Fix{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive fix")
		}
	}

	unsub1()
	if _, ok := <-ch1; ok {
		t.Error("ch1 should be closed after unsubscribe")
	}

	src.ch <- Fix{Mode: Mode3D, Lat: 3, Lon: 4, ReceivedAt: time.Now().Add(2 * time.Second)}
	select {
	case <-ch2:
	case <-time.After(time.Second):
		t.Fatal("ch2 should still receive after ch1 unsubscribed")
	}
}

func TestManagerSlowSubscriberDoesNotBlock(t *testing.T) {
	src := newFakeSource()
	m := NewManagerWithSource(Config{Enabled: true, MinInterval: 0}, src)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	_, unsub := m.Subscribe() // never read from
	defer unsub()

	done := make(chan struct{})
	go func() {
		base := time.Now()
		for i := 0; i < 100; i++ {
			src.ch <- Fix{Mode: Mode3D, Lat: float64(i), ReceivedAt: base.Add(time.Duration(i) * time.Millisecond)}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ingest blocked on a slow subscriber")
	}

	m.Stop()
	deadline := time.Now().Add(time.Second)
	for !src.ctxDone() {
		if time.Now().After(deadline) {
			t.Fatal("source context was never cancelled by Stop()")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestManagerUpdateConfigRestartsOnTargetChange(t *testing.T) {
	var mu sync.Mutex
	count := 0

	m := NewManager(Config{Enabled: true, Type: "gpsd", Host: "127.0.0.1", Port: 2947})
	m.factory = func(cfg Config) Source {
		mu.Lock()
		count++
		mu.Unlock()
		return newFakeSource()
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	mu.Lock()
	if count != 1 {
		t.Fatalf("factory called %d times after Start, want 1", count)
	}
	mu.Unlock()

	// Unrelated field change: no rebuild.
	m.UpdateConfig(Config{Enabled: true, Type: "gpsd", Host: "127.0.0.1", Port: 2947, StaleAfter: 5 * time.Second})
	mu.Lock()
	if count != 1 {
		t.Errorf("factory called %d times after StaleAfter-only change, want 1", count)
	}
	mu.Unlock()

	// Host change: rebuild.
	m.UpdateConfig(Config{Enabled: true, Type: "gpsd", Host: "192.168.1.50", Port: 2947, StaleAfter: 5 * time.Second})
	mu.Lock()
	if count != 2 {
		t.Errorf("factory called %d times after Host change, want 2", count)
	}
	mu.Unlock()
}
