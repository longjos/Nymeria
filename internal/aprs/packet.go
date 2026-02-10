package aprs

import "time"

// MessageData holds parsed APRS message data.
type MessageData struct {
	Addressee    string
	Text         string
	MessageNo    string // message number for ack/rej
	IsAck        bool
	IsRej        bool
	AckMsgNo     string // the message number being acked/rejected
	IsAutoAnswer bool   // true if message starts with "AA:" prefix
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
	WindDir     *float64 `json:"windDir,omitempty"`     // degrees
	WindSpeed   *float64 `json:"windSpeed,omitempty"`   // m/s
	WindGust    *float64 `json:"windGust,omitempty"`    // m/s
	Temperature *float64 `json:"temperature,omitempty"` // Celsius
	Humidity    *int     `json:"humidity,omitempty"`
	Pressure    *float64 `json:"pressure,omitempty"`    // hPa
	Rain1h      *float64 `json:"rain1h,omitempty"`      // mm
	Rain24h     *float64 `json:"rain24h,omitempty"`     // mm
	RainToday   *float64 `json:"rainToday,omitempty"`   // mm
	Luminosity  *int     `json:"luminosity,omitempty"`
	Radiation   *float64 `json:"radiation,omitempty"`   // nanosieverts/hour
	Voltage     *float64 `json:"voltage,omitempty"`     // volts
	FloodLevel  *float64 `json:"floodLevel,omitempty"`  // feet (raw from APRS)
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
