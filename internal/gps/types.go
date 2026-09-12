// Package gps provides live host-GPS position sources (gpsd and raw NMEA-0183)
// for the own-position marker and GPS-driven smart beaconing.
package gps

import (
	"encoding/json"
	"time"
)

// FixMode mirrors gpsd's TPV "mode" field exactly so gpsd values pass through
// untranslated. NMEA GSA mode 1/2/3 maps onto the same integers.
type FixMode int

const (
	ModeUnknown FixMode = 0
	ModeNoFix   FixMode = 1
	Mode2D      FixMode = 2
	Mode3D      FixMode = 3
)

// String returns the UI label: "unknown", "no fix", "2D", "3D".
func (m FixMode) String() string {
	switch m {
	case ModeNoFix:
		return "no fix"
	case Mode2D:
		return "2D"
	case Mode3D:
		return "3D"
	default:
		return "unknown"
	}
}

// Conversion constants, exported for use by tests and callers.
const (
	MetersPerSecondToKnots = 1.9438444924406046
	KnotsToMPH             = 1.15077945
	KnotsToKMH             = 1.852
)

// Fix is one position solution. Speed is ALWAYS in knots (NMEA native, APRS
// native, and gpsd m/s is converted on ingest). Altitude is meters MSL.
// Course is degrees true, 0-359.999.
type Fix struct {
	Mode        FixMode   `json:"mode"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	Altitude    float64   `json:"altitude,omitempty"` // meters MSL
	HasAltitude bool      `json:"hasAltitude"`
	SpeedKnots  float64   `json:"speedKnots"`
	Course      float64   `json:"course"`
	HasCourse   bool      `json:"hasCourse"` // false when stationary / field empty
	Satellites  int       `json:"satellites,omitempty"`
	HDOP        float64   `json:"hdop,omitempty"`
	Accuracy    float64   `json:"accuracy,omitempty"` // meters, 95% horizontal; 0 = unknown
	Time        time.Time `json:"time,omitempty"`     // GPS-reported UTC; zero if unknown
	ReceivedAt  time.Time `json:"receivedAt"`         // host clock when parsed — age is computed from this
}

// MarshalJSON omits Time when it is zero. `omitempty` alone cannot do this —
// time.Time is a struct, and encoding/json's omitempty only ever looks at
// Go's "empty" predicate (false/0/""/nil/len==0), which a zero time.Time
// never satisfies — so without this override a fix with no GPS-reported
// time (no-fix state, a void RMC, or a timestamp the rollover clamp
// rejected) would serialize Time as "0001-01-01T00:00:00Z" instead of
// leaving the field out. The frontend (web/src/lib/types.ts GpsFix.time)
// already declares this field optional, so omitting it is the only change.
func (f Fix) MarshalJSON() ([]byte, error) {
	type fixAlias Fix
	aux := struct {
		fixAlias
		Time *time.Time `json:"time,omitempty"`
	}{fixAlias: fixAlias(f)}
	if !f.Time.IsZero() {
		aux.Time = &f.Time
	}
	return json.Marshal(aux)
}

// HasPosition reports whether the fix carries a usable lat/lon.
func (f Fix) HasPosition() bool { return f.Mode >= Mode2D }

// SpeedMPH converts knots to statute mph (factor 1.15077945).
func (f Fix) SpeedMPH() float64 { return f.SpeedKnots * KnotsToMPH }

// Age returns now.Sub(f.ReceivedAt).
func (f Fix) Age(now time.Time) time.Duration { return now.Sub(f.ReceivedAt) }

// SourceStatus is the connection health of one Source.
type SourceStatus struct {
	Type      string    `json:"type"` // "gpsd" | "nmea" | "modemmanager"
	Target    string    `json:"target"`
	Connected bool      `json:"connected"`
	Error     string    `json:"error,omitempty"`
	LastRx    time.Time `json:"lastRx,omitempty"` // last sentence/JSON line of ANY kind
}
