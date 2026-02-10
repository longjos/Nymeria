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
	Callsign  string            `json:"callsign"`
	SSID      int               `json:"ssid"`
	LastHeard time.Time         `json:"lastHeard"`
	Position  *Position         `json:"position,omitempty"`
	Symbol    aprs.Symbol       `json:"symbol"`
	Comment   string            `json:"comment,omitempty"`
	Track     []TrackPoint      `json:"track"`
	Source    string            `json:"source"`
	Weather   *aprs.WeatherData `json:"weather,omitempty"`
}

// EventType identifies the kind of station event.
type EventType int

const (
	EventNewStation    EventType = iota // A station was seen for the first time.
	EventStationUpdate                  // An existing station was updated.
	EventStationExpired                 // A station was removed due to age.
)

// Event represents a change in station tracking state.
type Event struct {
	Type    EventType `json:"type"`
	Station Station   `json:"station"`
}
