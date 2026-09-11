package gps

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── ParseTPV ─────────────────────────────────────────────────────────

func TestParseTPV3D(t *testing.T) {
	data := []byte(`{"class":"TPV","device":"/dev/ttyUSB0","mode":3,"time":"2021-05-14T20:11:04.000Z","ept":0.005,"lat":44.068921,"lon":-121.314012,"alt":1120.2,"epx":15.0,"epy":19.0,"epv":45.0,"track":10.3221,"speed":6.091,"climb":0.0,"eps":38.0}`)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	f, err := ParseTPV(data, now)
	if err != nil {
		t.Fatalf("ParseTPV: %v", err)
	}
	if f.Mode != Mode3D {
		t.Errorf("mode = %v, want Mode3D", f.Mode)
	}
	approxEqual(t, "altitude", f.Altitude, 1120.2, 1e-9)
	approxEqual(t, "speed", f.SpeedKnots, 11.84, 0.01)
	if !f.HasCourse {
		t.Error("HasCourse = false, want true")
	}
	approxEqual(t, "accuracy", f.Accuracy, math.Hypot(15, 19), 1e-9)
	wantTime := time.Date(2021, 5, 14, 20, 11, 4, 0, time.UTC)
	if !f.Time.Equal(wantTime) {
		t.Errorf("time = %v, want %v", f.Time, wantTime)
	}
	if !f.ReceivedAt.Equal(now) {
		t.Errorf("ReceivedAt = %v, want %v", f.ReceivedAt, now)
	}
}

func TestParseTPVAltMSL(t *testing.T) {
	data := []byte(`{"class":"TPV","mode":3,"lat":44.0,"lon":-121.0,"altMSL":1113.0,"altHAE":1091.7,"eph":11.2}`)
	f, err := ParseTPV(data, time.Now())
	if err != nil {
		t.Fatalf("ParseTPV: %v", err)
	}
	approxEqual(t, "altitude", f.Altitude, 1113.0, 1e-9)
	approxEqual(t, "accuracy", f.Accuracy, 11.2, 1e-9)
}

func TestParseTPVNoFix(t *testing.T) {
	data := []byte(`{"class":"TPV","device":"/dev/ttyUSB0","mode":1}`)
	f, err := ParseTPV(data, time.Now())
	if err != nil {
		t.Fatalf("ParseTPV: %v", err)
	}
	if f.Mode != ModeNoFix {
		t.Errorf("mode = %v, want ModeNoFix", f.Mode)
	}
	if f.HasPosition() {
		t.Error("HasPosition() = true, want false")
	}
}

func TestParseTPVSlowSpeedClearsCourse(t *testing.T) {
	data := []byte(`{"class":"TPV","mode":3,"lat":44.0,"lon":-121.0,"track":123.4,"speed":0.09}`)
	f, err := ParseTPV(data, time.Now())
	if err != nil {
		t.Fatalf("ParseTPV: %v", err)
	}
	if f.HasCourse {
		t.Error("HasCourse = true, want false (speed < 1 kt)")
	}
}

func TestParseTPVMalformed(t *testing.T) {
	data := []byte(`{"class":"TPV","mode":3}`)
	if _, err := ParseTPV(data, time.Now()); !errors.Is(err, ErrMalformed) {
		t.Errorf("err = %v, want ErrMalformed", err)
	}
}

// ── GPSDSource ───────────────────────────────────────────────────────

type fakeGPSDConn struct {
	mu     sync.Mutex
	reader *bytes.Reader
	writes [][]byte
	closed bool
}

func (c *fakeGPSDConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *fakeGPSDConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writes = append(c.writes, append([]byte(nil), p...))
	c.mu.Unlock()
	return len(p), nil
}

func (c *fakeGPSDConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func (c *fakeGPSDConn) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.writes)
}

func TestGPSDSendsWatch(t *testing.T) {
	conn := &fakeGPSDConn{reader: bytes.NewReader(nil)}
	src := NewGPSDSource(GPSDConfig{Host: "127.0.0.1", Port: 2947})
	src.dial = func(GPSDConfig) (gpsdConn, error) { return conn, nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for conn.writeCount() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for WATCH write")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	time.Sleep(20 * time.Millisecond) // let the loop notice cancellation before we inspect writes

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(conn.writes))
	}
	want := `?WATCH={"enable":true,"json":true};` + "\n"
	if string(conn.writes[0]) != want {
		t.Errorf("write = %q, want %q", conn.writes[0], want)
	}
}

func TestGPSDIgnoresNonTPV(t *testing.T) {
	script := strings.Join([]string{
		`{"class":"VERSION","release":"3.20"}`,
		`{"class":"DEVICES","devices":[]}`,
		`{"class":"WATCH","enable":true,"json":true}`,
		`{"class":"SKY","uSat":9}`,
		`{"class":"TPV","mode":3,"lat":44.0,"lon":-121.0}`,
	}, "\n") + "\n"

	conn := &fakeGPSDConn{reader: bytes.NewReader([]byte(script))}
	src := NewGPSDSource(GPSDConfig{})
	src.dial = func(GPSDConfig) (gpsdConn, error) { return conn, nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case fix := <-src.Fixes():
		if fix.Satellites != 9 {
			t.Errorf("satellites = %d, want 9", fix.Satellites)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for TPV fix")
	}

	select {
	case fix, ok := <-src.Fixes():
		if ok {
			t.Errorf("unexpected second fix: %+v", fix)
		}
	case <-time.After(200 * time.Millisecond):
		// expected: nothing else arrives quickly
	}
}

func TestGPSDReconnectBackoff(t *testing.T) {
	dialErr := errors.New("connection refused")

	var mu sync.Mutex
	var calls []time.Time

	src := NewGPSDSource(GPSDConfig{Host: "127.0.0.1", Port: 2947})
	src.dial = func(GPSDConfig) (gpsdConn, error) {
		mu.Lock()
		calls = append(calls, time.Now())
		n := len(calls)
		mu.Unlock()
		if n <= 3 {
			return nil, dialErr
		}
		return &fakeGPSDConn{reader: bytes.NewReader(nil)}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The first failure should be reflected in status quickly.
	statusDeadline := time.Now().Add(2 * time.Second)
	for {
		st := src.Status()
		if st.Error != "" {
			if st.Error != dialErr.Error() {
				t.Errorf("status error = %q, want %q", st.Error, dialErr.Error())
			}
			break
		}
		if time.Now().After(statusDeadline) {
			t.Fatal("status error was never set while down")
		}
		time.Sleep(10 * time.Millisecond)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		mu.Lock()
		n := len(calls)
		mu.Unlock()
		if n >= 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dialer called %d times, want >= 4", n)
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	d1 := calls[1].Sub(calls[0])
	d2 := calls[2].Sub(calls[1])
	d3 := calls[3].Sub(calls[2])
	mu.Unlock()

	if d1 < 900*time.Millisecond {
		t.Errorf("1st retry delay = %v, want >= ~1s", d1)
	}
	if d2 < 1800*time.Millisecond {
		t.Errorf("2nd retry delay = %v, want >= ~2s", d2)
	}
	if d3 < 3600*time.Millisecond {
		t.Errorf("3rd retry delay = %v, want >= ~4s", d3)
	}
}
