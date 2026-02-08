package beacon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// ── FormatLat / FormatLon ────────────────────────────────────────────

func TestFormatLat(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		want string
	}{
		{"positive whole", 35.0, "3500.00N"},
		{"positive fractional", 35.928516, "3555.71N"},
		{"negative (south)", -33.861, "3351.66S"},
		{"zero", 0.0, "0000.00N"},
		{"north pole", 90.0, "9000.00N"},
		{"south pole", -90.0, "9000.00S"},
		{"small positive", 1.5, "0130.00N"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatLat(tt.lat)
			if got != tt.want {
				t.Errorf("FormatLat(%f) = %q, want %q", tt.lat, got, tt.want)
			}
		})
	}
}

func TestFormatLon(t *testing.T) {
	tests := []struct {
		name string
		lon  float64
		want string
	}{
		{"positive whole", 84.0, "08400.00E"},
		{"negative (west)", -84.331, "08419.86W"},
		{"zero", 0.0, "00000.00E"},
		{"positive 180", 180.0, "18000.00E"},
		{"negative 180", -180.0, "18000.00W"},
		{"small negative", -1.5, "00130.00W"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatLon(tt.lon)
			if got != tt.want {
				t.Errorf("FormatLon(%f) = %q, want %q", tt.lon, got, tt.want)
			}
		})
	}
}

// ── Beacon frame generation ─────────────────────────────────────────

func TestBuildFrame(t *testing.T) {
	m := New(Config{
		Enabled:  true,
		Interval: 10 * time.Minute,
		Comment:  "Nymeria APRS Client",
	}, StationInfo{
		Callsign:    "N0CALL",
		SSID:        5,
		Lat:         35.928516,
		Lon:         -84.331,
		SymbolTable: "/",
		SymbolCode:  "-",
	}, nil)

	frame := m.buildFrame()

	// Source
	if frame.Source.Call != "N0CALL" || frame.Source.SSID != 5 {
		t.Errorf("source = %v, want N0CALL-5", frame.Source)
	}

	// Destination (Nymeria tocall)
	if frame.Destination.Call != "APNMRA" {
		t.Errorf("destination = %q, want APNMRA", frame.Destination.Call)
	}

	// Path
	if len(frame.Path) != 2 {
		t.Fatalf("path len = %d, want 2", len(frame.Path))
	}
	if frame.Path[0].Call != "WIDE1" || frame.Path[0].SSID != 1 {
		t.Errorf("path[0] = %v, want WIDE1-1", frame.Path[0])
	}
	if frame.Path[1].Call != "WIDE2" || frame.Path[1].SSID != 1 {
		t.Errorf("path[1] = %v, want WIDE2-1", frame.Path[1])
	}

	// Payload: uncompressed position report
	want := "!3555.71N/08419.86W-Nymeria APRS Client"
	if frame.Payload != want {
		t.Errorf("payload = %q, want %q", frame.Payload, want)
	}
}

func TestBuildFrame_AlternateSymbolTable(t *testing.T) {
	m := New(Config{
		Enabled:  true,
		Interval: 10 * time.Minute,
		Comment:  "Test",
	}, StationInfo{
		Callsign:    "W1AW",
		Lat:         41.714775,
		Lon:         -72.727260,
		SymbolTable: `\`,
		SymbolCode:  "k",
	}, nil)

	frame := m.buildFrame()
	// Alternate symbol table uses \
	want := `!4142.89N\07243.64Wk`
	wantWithComment := want + "Test"
	if frame.Payload != wantWithComment {
		t.Errorf("payload = %q, want %q", frame.Payload, wantWithComment)
	}
}

func TestBuildFrame_DefaultSymbol(t *testing.T) {
	// When no symbol table/code specified, default to /- (house)
	m := New(Config{
		Enabled:  true,
		Interval: 10 * time.Minute,
	}, StationInfo{
		Callsign: "N0CALL",
		Lat:      0,
		Lon:      0,
	}, nil)

	frame := m.buildFrame()
	// Should use / table and - code
	if frame.Payload != "!0000.00N/00000.00E-" {
		t.Errorf("payload = %q, want default symbol", frame.Payload)
	}
}

// ── BeaconNow ───────────────────────────────────────────────────────

func TestBeaconNow(t *testing.T) {
	var mu sync.Mutex
	var sent []aprs.APRSFrame

	sendFn := func(f aprs.APRSFrame) error {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, f)
		return nil
	}

	m := New(Config{
		Enabled:  true,
		Interval: 10 * time.Minute,
		Comment:  "test beacon",
	}, StationInfo{
		Callsign:    "N0CALL",
		SSID:        9,
		Lat:         35.928516,
		Lon:         -84.331,
		SymbolTable: "/",
		SymbolCode:  "-",
	}, sendFn)

	if err := m.BeaconNow(); err != nil {
		t.Fatalf("BeaconNow: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 {
		t.Fatalf("sent %d frames, want 1", len(sent))
	}
	if sent[0].Source.Call != "N0CALL" || sent[0].Source.SSID != 9 {
		t.Errorf("source = %v, want N0CALL-9", sent[0].Source)
	}
}

func TestBeaconNow_NoSendFunc(t *testing.T) {
	m := New(Config{Enabled: true, Interval: time.Minute}, StationInfo{Callsign: "N0CALL"}, nil)
	err := m.BeaconNow()
	if err == nil {
		t.Error("expected error when send func is nil")
	}
}

// ── Start / Stop lifecycle ──────────────────────────────────────────

func TestStartStop(t *testing.T) {
	var mu sync.Mutex
	var count int

	sendFn := func(f aprs.APRSFrame) error {
		mu.Lock()
		defer mu.Unlock()
		count++
		return nil
	}

	m := New(Config{
		Enabled:  true,
		Interval: 20 * time.Millisecond, // very short for testing
		Comment:  "test",
	}, StationInfo{
		Callsign:    "N0CALL",
		Lat:         35.0,
		Lon:         -84.0,
		SymbolTable: "/",
		SymbolCode:  "-",
	}, sendFn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	if !m.IsRunning() {
		t.Error("expected IsRunning() == true after Start")
	}

	// Wait long enough for at least 2 beacon transmissions
	time.Sleep(80 * time.Millisecond)

	m.Stop()
	if m.IsRunning() {
		t.Error("expected IsRunning() == false after Stop")
	}

	mu.Lock()
	c := count
	mu.Unlock()

	// Should have sent at least 2 beacons (initial + at least one timer)
	if c < 2 {
		t.Errorf("beacon count = %d, want >= 2", c)
	}
}

func TestStartStop_ContextCancel(t *testing.T) {
	sendFn := func(f aprs.APRSFrame) error { return nil }

	m := New(Config{
		Enabled:  true,
		Interval: time.Hour, // long interval, won't fire
	}, StationInfo{
		Callsign:    "N0CALL",
		Lat:         35.0,
		Lon:         -84.0,
		SymbolTable: "/",
		SymbolCode:  "-",
	}, sendFn)

	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)

	if !m.IsRunning() {
		t.Error("expected running after Start")
	}

	cancel()
	// Give goroutine time to notice cancellation
	time.Sleep(20 * time.Millisecond)

	if m.IsRunning() {
		t.Error("expected stopped after context cancel")
	}
}

func TestDoubleStart(t *testing.T) {
	sendFn := func(f aprs.APRSFrame) error { return nil }
	m := New(Config{
		Enabled:  true,
		Interval: time.Hour,
	}, StationInfo{Callsign: "N0CALL", SymbolTable: "/", SymbolCode: "-"}, sendFn)

	ctx := context.Background()
	m.Start(ctx)
	defer m.Stop()

	// Second start should be a no-op (not panic)
	m.Start(ctx)
}

// ── Smart beaconing rate calculation ────────────────────────────────

func TestSmartRate(t *testing.T) {
	sc := &SmartConfig{
		FastSpeed: 60,              // mph
		SlowSpeed: 5,               // mph
		FastRate:  60 * time.Second, // 1 min at fast speed
		SlowRate:  30 * time.Minute, // 30 min at slow speed
		TurnAngle: 28,
		TurnSlope: 26,
	}

	tests := []struct {
		name    string
		speed   float64 // mph
		wantMin time.Duration
		wantMax time.Duration
	}{
		{"stationary", 0, 30 * time.Minute, 30 * time.Minute},
		{"below slow speed", 3, 30 * time.Minute, 30 * time.Minute},
		{"at slow speed", 5, 29 * time.Minute, 31 * time.Minute},
		{"at fast speed", 60, 59 * time.Second, 61 * time.Second},
		{"above fast speed", 100, 59 * time.Second, 61 * time.Second},
		{"mid speed ~30mph", 30, 5 * time.Minute, 20 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sc.Rate(tt.speed)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("Rate(%f mph) = %v, want between %v and %v",
					tt.speed, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSmartTurnThreshold(t *testing.T) {
	sc := &SmartConfig{
		FastSpeed: 60,
		SlowSpeed: 5,
		FastRate:  60 * time.Second,
		SlowRate:  30 * time.Minute,
		TurnAngle: 28,
		TurnSlope: 26,
	}

	tests := []struct {
		name  string
		speed float64
		want  float64 // expected turn threshold in degrees
	}{
		{"at 60mph", 60, 28},            // min angle = TurnAngle
		{"at 10mph", 10, 28 + 26.0/10}, // TurnAngle + TurnSlope/speed = 30.6
		{"at 1mph", 1, 28 + 26},        // TurnAngle + TurnSlope/speed = 54
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sc.TurnThreshold(tt.speed)
			// Allow small floating point tolerance
			if diff := got - tt.want; diff > 0.01 || diff < -0.01 {
				t.Errorf("TurnThreshold(%f) = %f, want %f", tt.speed, got, tt.want)
			}
		})
	}
}
