package aprs

import "time"

// PositionData holds parsed position information from an APRS packet.
type PositionData struct {
	Lat       float64
	Lon       float64
	Altitude  float64 // meters
	Speed     float64 // km/h
	Course    float64 // degrees
	Symbol    Symbol
	Comment   string
	Timestamp time.Time
	Ambiguity int    // 0-4, position ambiguity level
	Datum     string // APRS 1.2: datum from !DAO! (e.g., "W" for WGS84)
	Precision int    // APRS 1.2: extra precision digits applied (0=none, 1=human, 2=base91)
}

// FrequencyData holds parsed frequency information from APRS 1.2 comments.
type FrequencyData struct {
	Freq   float64 // MHz
	Tone   float64 // PL tone Hz (0 if none)
	DCS    int     // DCS code (0 if none)
	Offset float64 // offset in MHz (signed)
	Range  float64 // range in km (0 if none)
}
