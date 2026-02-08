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
	Ambiguity int // 0-4, position ambiguity level
}
