package gps

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func approxEqual(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > tol {
		t.Errorf("%s = %v, want %v (tol %v)", name, got, want, tol)
	}
}

// ── VerifyChecksum ───────────────────────────────────────────────────

func TestVerifyChecksum(t *testing.T) {
	pass := []string{
		"$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47",
		"$GPRMC,123519,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*6A",
		"$GNGGA,001043.00,4404.14036,N,12118.85961,W,1,12,0.98,1113.0,M,-21.3,M,,*47",
		"$GNRMC,001031.00,A,4404.13993,N,12118.86023,W,0.146,,100117,,,A*7B",
		"$GPGSA,A,3,10,07,05,02,29,04,08,13,,,,,1.72,1.03,1.38*0A",
		"$GPTXT,01,01,02,u-blox ag - www.u-blox.com*50",
	}
	for _, s := range pass {
		t.Run(s, func(t *testing.T) {
			if err := VerifyChecksum(s); err != nil {
				t.Errorf("VerifyChecksum(%q) = %v, want nil", s, err)
			}
		})
	}

	fail := map[string]string{
		"wrong checksum": "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*48",
		"truncated hex":  "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*4",
		"no dollar":      "GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47",
	}
	for name, s := range fail {
		t.Run(name, func(t *testing.T) {
			if err := VerifyChecksum(s); !errors.Is(err, ErrChecksum) {
				t.Errorf("VerifyChecksum(%q) = %v, want ErrChecksum", s, err)
			}
		})
	}
}

// ── ParseLatLon ──────────────────────────────────────────────────────

func TestParseLatLon(t *testing.T) {
	tests := []struct {
		name  string
		value string
		hemi  string
		want  float64
		ok    bool
		err   error
	}{
		{"lat N", "4807.038", "N", 48.1173, true, nil},
		{"lon E", "01131.000", "E", 11.516667, true, nil},
		{"lon W", "12118.85961", "W", -121.31432683, true, nil},
		{"lat N precise", "4404.14036", "N", 44.06900600, true, nil},
		{"empty", "", "", 0, false, nil},
		{"malformed", "48XX.0", "N", 0, false, ErrMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParseLatLon(tt.value, tt.hemi)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("err = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok {
				approxEqual(t, "value", got, tt.want, 1e-6)
			}
		})
	}
}

// ── ParseGGA ─────────────────────────────────────────────────────────

func TestParseGGA(t *testing.T) {
	t.Run("full fix", func(t *testing.T) {
		fields, err := Fields("$GPGGA,092750.000,5321.6802,N,00630.3372,W,1,8,1.03,61.7,M,55.2,M,,*76")
		if err != nil {
			t.Fatalf("Fields: %v", err)
		}
		f, err := ParseGGA(fields)
		if err != nil {
			t.Fatalf("ParseGGA: %v", err)
		}
		approxEqual(t, "lat", f.Lat, 53.361337, 1e-5)
		approxEqual(t, "lon", f.Lon, -6.505620, 1e-5)
		if f.Satellites != 8 {
			t.Errorf("satellites = %d, want 8", f.Satellites)
		}
		approxEqual(t, "hdop", f.HDOP, 1.03, 1e-9)
		approxEqual(t, "altitude", f.Altitude, 61.7, 1e-9)
		if !f.HasAltitude {
			t.Error("HasAltitude = false, want true")
		}
	})

	t.Run("quality 0 no fix", func(t *testing.T) {
		fields, err := Fields("$GPGGA,000000,0000.000,N,00000.000,E,0,00,99.9,0.0,M,0.0,M,,*4A")
		if err != nil {
			t.Fatalf("Fields: %v", err)
		}
		f, err := ParseGGA(fields)
		if err != nil {
			t.Fatalf("ParseGGA: %v", err)
		}
		if f.Mode != ModeNoFix {
			t.Errorf("mode = %v, want ModeNoFix", f.Mode)
		}
		if f.HasPosition() {
			t.Error("HasPosition() = true, want false")
		}
	})

	t.Run("quality 2 valid fix", func(t *testing.T) {
		fields, err := Fields("$GPGGA,092750.000,5321.6802,N,00630.3372,W,2,08,1.03,61.7,M,55.2,M,,*45")
		if err != nil {
			t.Fatalf("Fields: %v", err)
		}
		f, err := ParseGGA(fields)
		if err != nil {
			t.Fatalf("ParseGGA: %v", err)
		}
		if !f.HasPosition() {
			t.Error("HasPosition() = false, want true")
		}
	})
}

// ── ParseRMC ─────────────────────────────────────────────────────────

func TestParseRMC(t *testing.T) {
	t.Run("slow speed clears course", func(t *testing.T) {
		fields, err := Fields("$GPRMC,092750.000,A,5321.6802,N,00630.3372,W,0.02,31.66,280511,,,A*43")
		if err != nil {
			t.Fatalf("Fields: %v", err)
		}
		f, err := ParseRMC(fields)
		if err != nil {
			t.Fatalf("ParseRMC: %v", err)
		}
		approxEqual(t, "speed", f.SpeedKnots, 0.02, 1e-9)
		approxEqual(t, "course", f.Course, 31.66, 1e-9)
		if f.HasCourse {
			t.Error("HasCourse = true, want false (speed < 1 kt)")
		}
		want := time.Date(2011, 5, 28, 9, 27, 50, 0, time.UTC)
		if !f.Time.Equal(want) {
			t.Errorf("time = %v, want %v", f.Time, want)
		}
	})

	t.Run("moving has course", func(t *testing.T) {
		fields, err := Fields("$GPRMC,123519,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*6A")
		if err != nil {
			t.Fatalf("Fields: %v", err)
		}
		f, err := ParseRMC(fields)
		if err != nil {
			t.Fatalf("ParseRMC: %v", err)
		}
		approxEqual(t, "speed", f.SpeedKnots, 22.4, 1e-9)
		approxEqual(t, "course", f.Course, 84.4, 1e-9)
		if !f.HasCourse {
			t.Error("HasCourse = false, want true")
		}
		want := time.Date(1994, 3, 23, 12, 35, 19, 0, time.UTC)
		if !f.Time.Equal(want) {
			t.Errorf("time = %v, want %v", f.Time, want)
		}
	})

	t.Run("void status", func(t *testing.T) {
		fields, err := Fields("$GPRMC,123519,V,4807.038,N,01131.000,E,000.0,000.0,230394,003.1,W*71")
		if err != nil {
			t.Fatalf("Fields: %v", err)
		}
		f, err := ParseRMC(fields)
		if err != nil {
			t.Fatalf("ParseRMC: %v", err)
		}
		if f.Mode != ModeNoFix {
			t.Errorf("mode = %v, want ModeNoFix", f.Mode)
		}
		if f.Lat != 0 || f.Lon != 0 {
			t.Errorf("lat/lon = %v/%v, want zeroed", f.Lat, f.Lon)
		}
	})
}

// ── ParseGSA ─────────────────────────────────────────────────────────

func TestParseGSA(t *testing.T) {
	t.Run("3D", func(t *testing.T) {
		fields, err := Fields("$GPGSA,A,3,10,07,05,02,29,04,08,13,,,,,1.72,1.03,1.38*0A")
		if err != nil {
			t.Fatalf("Fields: %v", err)
		}
		mode, pdop, hdop, vdop, err := ParseGSA(fields)
		if err != nil {
			t.Fatalf("ParseGSA: %v", err)
		}
		if mode != Mode3D {
			t.Errorf("mode = %v, want Mode3D", mode)
		}
		approxEqual(t, "pdop", pdop, 1.72, 1e-9)
		approxEqual(t, "hdop", hdop, 1.03, 1e-9)
		approxEqual(t, "vdop", vdop, 1.38, 1e-9)
	})

	t.Run("2D", func(t *testing.T) {
		fields, err := Fields("$GPGSA,A,2,10,07,05,,,,,,,,,,2.72,2.03,1.38*0C")
		if err != nil {
			t.Fatalf("Fields: %v", err)
		}
		mode, _, _, _, err := ParseGSA(fields)
		if err != nil {
			t.Fatalf("ParseGSA: %v", err)
		}
		if mode != Mode2D {
			t.Errorf("mode = %v, want Mode2D", mode)
		}
	})
}

// ── Assembler ────────────────────────────────────────────────────────

func TestAssemblerCompleteCycle(t *testing.T) {
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := NewAssembler(func() time.Time { return fixed })

	if _, emitted, err := a.Consume("$GPGGA,092750.000,5321.6802,N,00630.3372,W,1,8,1.03,61.7,M,55.2,M,,*76"); err != nil || emitted {
		t.Fatalf("GGA: emitted=%v err=%v, want false/nil", emitted, err)
	}
	if _, emitted, err := a.Consume("$GPGSA,A,3,10,07,05,02,29,04,08,13,,,,,1.72,1.03,1.38*0A"); err != nil || emitted {
		t.Fatalf("GSA: emitted=%v err=%v, want false/nil", emitted, err)
	}
	fix, emitted, err := a.Consume("$GPRMC,092750.000,A,5321.6802,N,00630.3372,W,0.02,31.66,280511,,,A*43")
	if err != nil {
		t.Fatalf("RMC: err = %v", err)
	}
	if !emitted {
		t.Fatal("RMC: expected a completed cycle to emit")
	}

	approxEqual(t, "lat", fix.Lat, 53.361337, 1e-5)
	approxEqual(t, "lon", fix.Lon, -6.505620, 1e-5)
	if fix.Mode != Mode3D {
		t.Errorf("mode = %v, want Mode3D (from GSA)", fix.Mode)
	}
	approxEqual(t, "hdop", fix.HDOP, 1.03, 1e-9)
	approxEqual(t, "accuracy", fix.Accuracy, 5.15, 1e-9)
	approxEqual(t, "speed", fix.SpeedKnots, 0.02, 1e-9)
	if !fix.ReceivedAt.Equal(fixed) {
		t.Errorf("ReceivedAt = %v, want %v", fix.ReceivedAt, fixed)
	}
}

func TestAssemblerIgnoresUnsupported(t *testing.T) {
	a := NewAssembler(nil)

	if _, emitted, err := a.Consume("$GPGSV,3,1,11,03,03,111,00,04,15,270,00,06,01,010,00,13,06,292,00*74"); err != nil || emitted {
		t.Fatalf("GSV: emitted=%v err=%v, want false/nil", emitted, err)
	}
	if _, emitted, err := a.Consume("$GPTXT,01,01,02,u-blox ag - www.u-blox.com*50"); err != nil || emitted {
		t.Fatalf("TXT: emitted=%v err=%v, want false/nil", emitted, err)
	}

	// A following real cycle must still complete correctly.
	if _, emitted, err := a.Consume("$GPGGA,092750.000,5321.6802,N,00630.3372,W,1,8,1.03,61.7,M,55.2,M,,*76"); err != nil || emitted {
		t.Fatalf("GGA: emitted=%v err=%v, want false/nil", emitted, err)
	}
	fix, emitted, err := a.Consume("$GPRMC,092750.000,A,5321.6802,N,00630.3372,W,0.02,31.66,280511,,,A*43")
	if err != nil || !emitted {
		t.Fatalf("RMC: emitted=%v err=%v, want true/nil", emitted, err)
	}
	if !fix.HasPosition() {
		t.Error("expected a position fix after unsupported sentences")
	}
}

func TestAssemblerBadChecksumDoesNotEmit(t *testing.T) {
	a := NewAssembler(nil)

	// Corrupt checksum on an otherwise-valid GGA.
	_, emitted, err := a.Consume("$GPGGA,092750.000,5321.6802,N,00630.3372,W,1,8,1.03,61.7,M,55.2,M,,*77")
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v, want ErrChecksum", err)
	}
	if emitted {
		t.Fatal("corrupt sentence must not emit")
	}
	if a.ChecksumErrors() != 1 {
		t.Errorf("ChecksumErrors() = %d, want 1", a.ChecksumErrors())
	}

	// A following good cycle must still assemble correctly.
	if _, emitted, err := a.Consume("$GPGGA,092750.000,5321.6802,N,00630.3372,W,1,8,1.03,61.7,M,55.2,M,,*76"); err != nil || emitted {
		t.Fatalf("GGA: emitted=%v err=%v, want false/nil", emitted, err)
	}
	fix, emitted, err := a.Consume("$GPRMC,092750.000,A,5321.6802,N,00630.3372,W,0.02,31.66,280511,,,A*43")
	if err != nil || !emitted {
		t.Fatalf("RMC: emitted=%v err=%v, want true/nil", emitted, err)
	}
	if !fix.HasPosition() {
		t.Error("expected a position fix on the good cycle")
	}
}

func TestAssemblerTalkerAgnostic(t *testing.T) {
	a := NewAssembler(nil)

	if _, emitted, err := a.Consume("$GNGGA,001043.00,4404.14036,N,12118.85961,W,1,12,0.98,1113.0,M,-21.3,M,,*47"); err != nil || emitted {
		t.Fatalf("GNGGA: emitted=%v err=%v, want false/nil", emitted, err)
	}
	fix, emitted, err := a.Consume("$GNRMC,001043.00,A,4404.13993,N,12118.86023,W,0.146,,100117,,,A*7E")
	if err != nil || !emitted {
		t.Fatalf("GNRMC: emitted=%v err=%v, want true/nil", emitted, err)
	}
	approxEqual(t, "lat", fix.Lat, 44.069006, 1e-5)
	approxEqual(t, "lon", fix.Lon, -121.314327, 1e-5)
}

func TestAssemblerOneEmitPerCycle(t *testing.T) {
	a := NewAssembler(nil)
	emits := 0

	seq := []string{
		"$GPGGA,092750.000,5321.6802,N,00630.3372,W,1,8,1.03,61.7,M,55.2,M,,*76",
		"$GPRMC,092750.000,A,5321.6802,N,00630.3372,W,0.02,31.66,280511,,,A*43",
		"$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47",
		"$GPRMC,123519,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*6A",
	}
	for _, s := range seq {
		_, emitted, err := a.Consume(s)
		if err != nil {
			t.Fatalf("Consume(%q): %v", s, err)
		}
		if emitted {
			emits++
		}
	}
	if emits != 2 {
		t.Errorf("emits = %d, want 2", emits)
	}
}

// ── NMEASource reconnect ─────────────────────────────────────────────

type stringReadCloser struct{ *strings.Reader }

func (stringReadCloser) Close() error { return nil }

func TestNMEASourceReconnects(t *testing.T) {
	const oneCycle = "$GPGGA,092750.000,5321.6802,N,00630.3372,W,1,8,1.03,61.7,M,55.2,M,,*76\r\n" +
		"$GPRMC,092750.000,A,5321.6802,N,00630.3372,W,0.02,31.66,280511,,,A*43\r\n"

	var mu sync.Mutex
	var calls []time.Time

	src := NewNMEASource(NMEAConfig{Host: "127.0.0.1", Port: 9999})
	src.dial = func(NMEAConfig) (io.ReadCloser, error) {
		mu.Lock()
		calls = append(calls, time.Now())
		mu.Unlock()
		return stringReadCloser{strings.NewReader(oneCycle)}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case fix := <-src.Fixes():
		if !fix.HasPosition() {
			t.Error("expected a position fix from the first connection")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first fix")
	}

	deadline := time.Now().Add(4 * time.Second)
	for {
		mu.Lock()
		n := len(calls)
		mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dialer called %d times, want >= 2", n)
		}
		time.Sleep(25 * time.Millisecond)
	}

	mu.Lock()
	delta := calls[1].Sub(calls[0])
	mu.Unlock()
	if delta < 900*time.Millisecond {
		t.Errorf("2nd dial after %v, want >= ~1s (initial backoff)", delta)
	}

	if status := src.Status(); status.Type != "nmea" {
		t.Errorf("status.Type = %q, want nmea", status.Type)
	}
}
