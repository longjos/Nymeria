package aprs

import "time"

// PositionData holds parsed position information from an APRS packet.
type PositionData struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Altitude  float64   `json:"altitude"`           // meters
	Speed     float64   `json:"speed"`              // km/h
	Course    float64   `json:"course"`             // degrees
	Symbol    Symbol    `json:"symbol"`
	Comment   string    `json:"comment,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	Ambiguity int       `json:"ambiguity,omitempty"` // 0-4, position ambiguity level
	Datum     string    `json:"datum,omitempty"`     // APRS 1.2: datum from !DAO! (e.g., "W" for WGS84)
	Precision int       `json:"precision,omitempty"` // APRS 1.2: extra precision digits applied (0=none, 1=human, 2=base91)
}

// FrequencyData holds parsed frequency information from APRS 1.2 comments.
type FrequencyData struct {
	Freq   float64 `json:"freq"`             // MHz
	Tone   float64 `json:"tone,omitempty"`   // PL tone Hz (0 if none)
	DCS    int     `json:"dcs,omitempty"`    // DCS code (0 if none)
	Offset float64 `json:"offset,omitempty"` // offset in MHz (signed)
	Range  float64 `json:"range,omitempty"`  // range in km (0 if none)
}
