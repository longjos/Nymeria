package station

import (
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// Position represents a station's geographic position.
type Position struct {
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Altitude float64 `json:"altitude,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
	Course   float64 `json:"course,omitempty"`
}

// TrackPoint is a timestamped position for track history.
type TrackPoint struct {
	Lat  float64   `json:"lat"`
	Lon  float64   `json:"lon"`
	Time time.Time `json:"time"`
}

// Station represents a tracked APRS station.
type Station struct {
	Callsign  string      `json:"callsign"`
	SSID      int         `json:"ssid"`
	LastHeard time.Time   `json:"lastHeard"`
	Position  *Position   `json:"position,omitempty"`
	Symbol    aprs.Symbol `json:"symbol"`
	Comment   string      `json:"comment,omitempty"`
	Track     []TrackPoint `json:"track"`
}
