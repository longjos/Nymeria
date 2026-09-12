package gps

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ── Fix.MarshalJSON ──────────────────────────────────────────────────

// A zero time.Time is never "empty" per encoding/json's omitempty rules, so
// without Fix's custom MarshalJSON this would serialize as
// "0001-01-01T00:00:00Z" — exactly what was observed live in the no-fix
// state on the CF-20 field test.
func TestFixMarshalJSONOmitsZeroTime(t *testing.T) {
	f := Fix{Mode: ModeNoFix, ReceivedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), `"time"`) {
		t.Errorf("marshaled Fix with zero Time contains a \"time\" key: %s", data)
	}
	if strings.Contains(string(data), "0001-01-01") {
		t.Errorf("marshaled Fix contains the year-zero timestamp: %s", data)
	}

	// receivedAt has no omitempty and must still always be present.
	if !strings.Contains(string(data), `"receivedAt"`) {
		t.Errorf("marshaled Fix is missing receivedAt: %s", data)
	}
}

func TestFixMarshalJSONKeepsNonZeroTime(t *testing.T) {
	want := time.Date(2026, 9, 11, 23, 40, 58, 0, time.UTC)
	f := Fix{Mode: Mode3D, Time: want, ReceivedAt: want}

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded struct {
		Time *time.Time `json:"time"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Time == nil {
		t.Fatal("time field absent, want present")
	}
	if !decoded.Time.Equal(want) {
		t.Errorf("time = %v, want %v", decoded.Time, want)
	}
}

// Every other field must round-trip exactly as before — the custom
// MarshalJSON must not change the JSON shape of anything but Time.
func TestFixMarshalJSONPreservesOtherFields(t *testing.T) {
	f := Fix{
		Mode:        Mode3D,
		Lat:         44.068921,
		Lon:         -121.314012,
		Altitude:    1120.2,
		HasAltitude: true,
		SpeedKnots:  6.1,
		Course:      10.3,
		HasCourse:   true,
		Satellites:  5,
		HDOP:        1.9,
		Accuracy:    9.5,
		ReceivedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"mode", "lat", "lon", "altitude", "hasAltitude",
		"speedKnots", "course", "hasCourse", "satellites", "hdop", "accuracy", "receivedAt"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("marshaled Fix is missing expected key %q: %s", key, data)
		}
	}
	if _, ok := decoded["time"]; ok {
		t.Errorf("marshaled Fix unexpectedly has a time key: %s", data)
	}
}
