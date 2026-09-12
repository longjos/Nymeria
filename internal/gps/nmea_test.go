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

// ── clampGPSTime ─────────────────────────────────────────────────────

func TestClampGPSTime(t *testing.T) {
	now := time.Date(2026, 9, 11, 23, 40, 58, 0, time.UTC)
	const week = 7 * 24 * time.Hour

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "normal in-sync timestamp passes through untouched",
			in:   now.Add(-2 * time.Second),
			want: now.Add(-2 * time.Second),
		},
		{
			name: "small clock skew in seconds passes through",
			in:   now.Add(3 * time.Second),
			want: now.Add(3 * time.Second),
		},
		{
			name: "small clock skew in minutes passes through",
			in:   now.Add(-90 * time.Minute),
			want: now.Add(-90 * time.Minute),
		},
		{
			name: "several days of drifted RTC still passes",
			in:   now.Add(-10 * 24 * time.Hour),
			want: now.Add(-10 * 24 * time.Hour),
		},
		{
			name: "just past the threshold is rejected",
			in:   now.Add(-31 * 24 * time.Hour),
			want: time.Time{},
		},
		{
			name: "exactly 1024 GPS weeks in the past is rejected",
			in:   now.Add(-1024 * week),
			want: time.Time{},
		},
		{
			name: "exactly 1024 GPS weeks in the future is rejected",
			in:   now.Add(1024 * week),
			want: time.Time{},
		},
		{
			name: "already-zero time stays zero without error",
			in:   time.Time{},
			want: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampGPSTime(tt.in, now)
			if tt.want.IsZero() {
				if !got.IsZero() {
					t.Errorf("clampGPSTime(%v, %v) = %v, want zero", tt.in, now, got)
				}
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("clampGPSTime(%v, %v) = %v, want %v", tt.in, now, got, tt.want)
			}
		})
	}
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
	// This cycle's RMC date (280511 -> 2011-05-28) is ~14.6 years before the
	// fixed host clock above — well past gpsTimeSkewLimit — so Time must be
	// clamped to zero even though the rest of the fix assembles normally.
	if !fix.Time.IsZero() {
		t.Errorf("Time = %v, want zero (implausibly far from host clock)", fix.Time)
	}
}

// TestAssemblerConsumeClampsRolloverTime reproduces the exact CF-20 field
// bug through the streaming Consume path (used by NMEASource): a receiver
// hit by the classic 10-bit GPS week-number rollover reports a date exactly
// 1024 weeks (one GPS epoch) in the past. finalize() must clamp Time to
// zero rather than publish it, while everything else about the fix (a
// position from GGA+RMC) is unaffected.
func TestAssemblerConsumeClampsRolloverTime(t *testing.T) {
	// Matches the CF-20 field test: receivedAt 2026-09-11T23:40:58-05:00
	// (= 2026-09-12T04:40:58Z) vs. the modem's reported 2007-01-27T04:40:57Z
	// — exactly 1024 weeks (one GPS epoch) earlier.
	now := time.Date(2026, 9, 12, 4, 40, 58, 0, time.UTC)
	a := NewAssembler(func() time.Time { return now })

	if _, emitted, err := a.Consume("$GPGGA,044057.00,5321.6802,N,00630.3372,W,1,05,1.9,130.6,M,46.9,M,,*77"); err != nil || emitted {
		t.Fatalf("GGA: emitted=%v err=%v, want false/nil", emitted, err)
	}
	// Date field "270107" -> 2007-01-27, exactly 1024 weeks before `now`.
	fix, emitted, err := a.Consume("$GPRMC,044057.00,A,5321.6802,N,00630.3372,W,0.0,0.0,270107,,,A*44")
	if err != nil {
		t.Fatalf("RMC: err = %v", err)
	}
	if !emitted {
		t.Fatal("RMC: expected a completed cycle to emit")
	}
	if !fix.Time.IsZero() {
		t.Errorf("Time = %v, want zero (1024-week rollover clamp)", fix.Time)
	}
	if !fix.HasPosition() {
		t.Error("expected the fix to still carry a position despite the clamped time")
	}
	if !fix.ReceivedAt.Equal(now) {
		t.Errorf("ReceivedAt = %v, want %v", fix.ReceivedAt, now)
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

// ── Assembler.ConsumeSnapshot ────────────────────────────────────────

// Verified NMEA fixtures for ConsumeSnapshot, shared with modemmanager_test.go.
const (
	snapGGA = `$GPGGA,123519.00,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*69`
	snapRMC = `$GPRMC,123519.00,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*44`
	snapGSA = `$GPGSA,A,3,04,05,,09,12,,,24,,,,,2.5,1.3,2.1*39`

	// Same triad but RMC's time field is 9s behind GGA's — the regression
	// guard for the whole snapshot design (see the test using it below).
	snapRMCTimeSkew = `$GPRMC,123510.00,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*4D`

	// Straight from the ModemManager introspection XML's own worked example:
	// a void RMC and an all-empty GGA. MM publishes NMEA regardless of fix
	// validity, so the parser's validity checks carry the whole load.
	snapRMCVoid  = `$GPRMC,134526.92,V,,,,,,,030136,,,N*76`
	snapGGAEmpty = `$GPGGA,,,,,,0,00,0.5,,M,0.0001999,M,0.0000099,0000*45`

	// Multi-constellation talker IDs — 1s later than the GP triad.
	snapRMCGN = `$GNRMC,123520.00,A,4807.040,N,01131.002,E,024.0,085.0,230394,003.1,W*5A`
	snapGGAGN = `$GNGGA,123520.00,4807.040,N,01131.002,E,1,09,0.8,546.0,M,46.9,M,,*77`
	snapGSAGN = `$GNGSA,A,3,04,05,,09,12,,,24,,,,,2.4,1.2,2.0*26`

	// snapGSABadChecksum is snapGSA with one checksum hex digit flipped.
	snapGSABadChecksum = `$GPGSA,A,3,04,05,,09,12,,,24,,,,,2.5,1.3,2.1*49`
)

func TestAssemblerConsumeSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		sentences []string
		wantOK    bool
		now       time.Time // zero uses the table default of 2026-01-01
		check     func(t *testing.T, f Fix)
	}{
		{
			name:      "full triad in GSA,GGA,RMC order",
			sentences: []string{snapGSA, snapGGA, snapRMC},
			wantOK:    true,
			// Close to snapRMC's own date (230394 -> 1994-03-23) so this
			// case exercises date-field decoding, not the rollover clamp
			// (see the dedicated case below for that).
			now: time.Date(1994, 3, 23, 12, 40, 0, 0, time.UTC),
			check: func(t *testing.T, f Fix) {
				if f.Mode != Mode3D {
					t.Errorf("Mode = %v, want Mode3D", f.Mode)
				}
				approxEqual(t, "lat", f.Lat, 48.1173, 1e-4)
				approxEqual(t, "lon", f.Lon, 11.516667, 1e-4)
				if !f.HasAltitude || f.Altitude != 545.4 {
					t.Errorf("Altitude = %v (has=%v), want 545.4 (true)", f.Altitude, f.HasAltitude)
				}
				approxEqual(t, "speed", f.SpeedKnots, 22.4, 1e-9)
				approxEqual(t, "course", f.Course, 84.4, 1e-9)
				if !f.HasCourse {
					t.Error("HasCourse = false, want true")
				}
				if f.Satellites != 8 {
					t.Errorf("Satellites = %d, want 8", f.Satellites)
				}
				approxEqual(t, "hdop", f.HDOP, 1.3, 1e-9)
				approxEqual(t, "accuracy", f.Accuracy, 6.5, 1e-9)
				want := time.Date(1994, 3, 23, 12, 35, 19, 0, time.UTC)
				if !f.Time.Equal(want) {
					t.Errorf("Time = %v, want %v", f.Time, want)
				}
			},
		},
		{
			// Regression guard: Consume would emit at GGA (before GSA
			// arrives) and throw the GSA's Mode/HDOP away. ConsumeSnapshot
			// must produce the IDENTICAL fix regardless of arrival order.
			name:      "full triad in arrival order RMC,GGA,GSA",
			sentences: []string{snapRMC, snapGGA, snapGSA},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if f.Mode != Mode3D {
					t.Errorf("Mode = %v, want Mode3D", f.Mode)
				}
				approxEqual(t, "hdop", f.HDOP, 1.3, 1e-9)
				if f.Satellites != 8 {
					t.Errorf("Satellites = %d, want 8", f.Satellites)
				}
			},
		},
		{
			// THE single most important test in this file: MM caches one
			// sentence per type independently, so a real blob can carry a
			// GGA and RMC whose time fields disagree by a few seconds.
			// Consume treats that as a cycle boundary and — because GSA
			// hasn't arrived and GGA+RMC never coexist in the same
			// accumulator — emits NOTHING, forever. ConsumeSnapshot has no
			// such heuristic and must still assemble a fix.
			name:      "time-skewed blob still assembles (Consume would emit nothing forever)",
			sentences: []string{snapGGA, snapRMCTimeSkew},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if !f.HasPosition() {
					t.Error("expected a position fix despite the GGA/RMC time skew")
				}
			},
		},
		{
			name:      "GGA only",
			sentences: []string{snapGGA},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if f.Mode != Mode3D {
					t.Errorf("Mode = %v, want Mode3D", f.Mode)
				}
				if f.SpeedKnots != 0 || f.HasCourse {
					t.Errorf("expected zero speed/course from GGA-only, got speed=%v hasCourse=%v", f.SpeedKnots, f.HasCourse)
				}
			},
		},
		{
			name:      "RMC only, status A, with position",
			sentences: []string{snapRMC},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if f.Mode != Mode2D {
					t.Errorf("Mode = %v, want Mode2D", f.Mode)
				}
				approxEqual(t, "speed", f.SpeedKnots, 22.4, 1e-9)
				if !f.HasCourse {
					t.Error("HasCourse = false, want true")
				}
			},
		},
		{
			name:      "RMC only, status V",
			sentences: []string{snapRMCVoid},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if f.Mode != ModeNoFix {
					t.Errorf("Mode = %v, want ModeNoFix", f.Mode)
				}
				if f.Lat != 0 || f.Lon != 0 {
					t.Errorf("Lat/Lon = %v/%v, want 0/0", f.Lat, f.Lon)
				}
			},
		},
		{
			name:      "empty GGA + void RMC",
			sentences: []string{snapGGAEmpty, snapRMCVoid},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if f.Mode != ModeNoFix {
					t.Errorf("Mode = %v, want ModeNoFix", f.Mode)
				}
				if f.HasPosition() {
					t.Error("HasPosition() = true, want false")
				}
			},
		},
		{
			name:      "GSA only, no position sentence",
			sentences: []string{snapGSA},
			wantOK:    false,
		},
		{
			name:      "empty slice",
			sentences: nil,
			wantOK:    false,
		},
		{
			name:      "GN talkers",
			sentences: []string{snapGSAGN, snapGGAGN, snapRMCGN},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if f.Mode != Mode3D {
					t.Errorf("Mode = %v, want Mode3D", f.Mode)
				}
				if f.Satellites != 9 {
					t.Errorf("Satellites = %d, want 9", f.Satellites)
				}
				approxEqual(t, "hdop", f.HDOP, 1.2, 1e-9)
			},
		},
		{
			name:      "one corrupt checksum among good sentences",
			sentences: []string{snapGSABadChecksum, snapGGA, snapRMC},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if f.Satellites != 8 {
					t.Errorf("Satellites = %d, want 8 (from GGA, GSA rejected)", f.Satellites)
				}
			},
		},
		{
			name:      "junk and oversized line mixed in",
			sentences: []string{"not nmea at all", strings.Repeat("$GPGGA,", 30), snapGGA, snapRMC},
			wantOK:    true,
			check: func(t *testing.T, f Fix) {
				if !f.HasPosition() {
					t.Error("expected the good GGA/RMC to still assemble")
				}
			},
		},
		{
			// Reproduces the CF-20 field bug end-to-end through the exact
			// path ModemManager's cached-NMEA-blob decoding uses: an old
			// receiver's RMC date is decades away from the host clock (here,
			// the table default "now" of 2026-01-01 vs. snapRMC's parsed
			// 1994-03-23 — far past gpsTimeSkewLimit), so Time must come out
			// zero even though the fix itself (position, mode, etc.) is
			// still assembled and usable.
			name:      "RMC date implausibly far from host clock is clamped to zero",
			sentences: []string{snapGSA, snapGGA, snapRMC},
			wantOK:    true,
			now:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			check: func(t *testing.T, f Fix) {
				if !f.Time.IsZero() {
					t.Errorf("Time = %v, want zero (rollover/implausible-skew clamp)", f.Time)
				}
				if !f.HasPosition() {
					t.Error("expected the fix to still assemble despite the clamped time")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := tt.now
			if now.IsZero() {
				now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			asm := NewAssembler(func() time.Time { return now })
			f, ok, err := asm.ConsumeSnapshot(tt.sentences)
			if err != nil {
				t.Fatalf("ConsumeSnapshot: unexpected error %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

func TestAssemblerConsumeSnapshotCorruptChecksumCounted(t *testing.T) {
	asm := NewAssembler(nil)
	f, ok, err := asm.ConsumeSnapshot([]string{snapGSABadChecksum, snapGGA, snapRMC})
	if err != nil {
		t.Fatalf("ConsumeSnapshot: %v", err)
	}
	if !ok || !f.HasPosition() {
		t.Fatalf("expected a fix assembled from GGA+RMC despite bad GSA checksum")
	}
	if got := asm.ChecksumErrors(); got != 1 {
		t.Errorf("ChecksumErrors() = %d, want 1", got)
	}
}

// TestAssemblerConsumeSnapshotLeavesNoResidue proves ConsumeSnapshot resets
// state on both entry and exit: driving two different blobs through the SAME
// Assembler must not let the first blob's Satellites/HDOP leak into the
// second's result.
func TestAssemblerConsumeSnapshotLeavesNoResidue(t *testing.T) {
	asm := NewAssembler(nil)

	f1, ok, err := asm.ConsumeSnapshot([]string{snapGSA, snapGGA, snapRMC})
	if err != nil || !ok {
		t.Fatalf("first ConsumeSnapshot: ok=%v err=%v", ok, err)
	}
	if f1.Satellites != 8 {
		t.Fatalf("first fix satellites = %d, want 8", f1.Satellites)
	}

	f2, ok, err := asm.ConsumeSnapshot([]string{snapRMCVoid})
	if err != nil || !ok {
		t.Fatalf("second ConsumeSnapshot: ok=%v err=%v", ok, err)
	}
	if f2.Satellites != 0 {
		t.Errorf("second fix satellites = %d, want 0 (residue from first blob)", f2.Satellites)
	}
	if f2.HDOP != 0 {
		t.Errorf("second fix HDOP = %v, want 0 (residue from first blob)", f2.HDOP)
	}
	if f2.Mode != ModeNoFix {
		t.Errorf("second fix Mode = %v, want ModeNoFix", f2.Mode)
	}
}
