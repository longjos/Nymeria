package aprs

import "time"

// MessageData holds parsed APRS message data.
type MessageData struct {
	Addressee string
	Text      string
	MessageNo string // message number for ack/rej
	IsAck     bool
	IsRej     bool
	AckMsgNo  string // the message number being acked/rejected
}

// ObjectData holds parsed APRS object data.
type ObjectData struct {
	Name      string
	Live      bool // true = live object, false = killed
	Timestamp time.Time
	Position  PositionData
}

// WeatherData holds parsed APRS weather data.
type WeatherData struct {
	WindDir     *float64 // degrees
	WindSpeed   *float64 // m/s
	WindGust    *float64 // m/s
	Temperature *float64 // Celsius
	Humidity    *int
	Pressure    *float64 // hPa
	Rain1h      *float64 // mm
	Rain24h     *float64 // mm
	RainToday   *float64 // mm
	Luminosity  *int
}

// StatusData holds parsed APRS status data.
type StatusData struct {
	Text       string
	Timestamp  time.Time
	Maidenhead string
}

// ItemData holds parsed APRS item data.
type ItemData struct {
	Name     string
	Live     bool // true = live item, false = killed
	Position PositionData
}

// TelemetryData holds parsed APRS telemetry data.
type TelemetryData struct {
	Seq     int
	Analog  [5]float64
	Digital byte
	Comment string
}

// MicEData holds parsed Mic-E position data.
type MicEData struct {
	Position   PositionData
	MicEMsg    string // Mic-E message type (e.g., "Off Duty", "En Route")
	RadioModel string
}
