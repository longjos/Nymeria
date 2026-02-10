package aprs

import "time"

// MessageData holds parsed APRS message data.
type MessageData struct {
	Addressee    string `json:"addressee"`
	Text         string `json:"text,omitempty"`
	MessageNo    string `json:"messageNo,omitempty"`    // message number for ack/rej
	IsAck        bool   `json:"isAck,omitempty"`
	IsRej        bool   `json:"isRej,omitempty"`
	AckMsgNo     string `json:"ackMsgNo,omitempty"`     // the message number being acked/rejected
	IsAutoAnswer bool   `json:"isAutoAnswer,omitempty"` // true if message starts with "AA:" prefix
}

// ObjectData holds parsed APRS object data.
type ObjectData struct {
	Name      string       `json:"name"`
	Live      bool         `json:"live"`                // true = live object, false = killed
	Timestamp time.Time    `json:"timestamp,omitempty"`
	Position  PositionData `json:"position"`
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
	Text       string    `json:"text"`
	Timestamp  time.Time `json:"timestamp,omitempty"`
	Maidenhead string    `json:"maidenhead,omitempty"`
}

// ItemData holds parsed APRS item data.
type ItemData struct {
	Name     string       `json:"name"`
	Live     bool         `json:"live"` // true = live item, false = killed
	Position PositionData `json:"position"`
}

// TelemetryData holds parsed APRS telemetry data.
type TelemetryData struct {
	Seq     int        `json:"seq"`
	Analog  [5]float64 `json:"analog"`
	Digital byte       `json:"digital"`
	Comment string     `json:"comment,omitempty"`
}

// TelemetryParams holds PARM/UNIT/EQNS/BITS metadata for a telemetry station.
type TelemetryParams struct {
	ParamNames   [5]string      `json:"paramNames"`
	UnitLabels   [5]string      `json:"unitLabels"`
	Equations    [5][3]float64  `json:"equations"`    // a*x^2 + b*x + c per channel
	BitSense     byte           `json:"bitSense"`     // 1=active-high per bit
	BitLabels    [8]string      `json:"bitLabels"`    // labels for digital bits
	ProjectTitle string         `json:"projectTitle,omitempty"`
}

// ApplyEquation converts a raw analog value to engineering units using the
// EQNS coefficients for the given channel: a*v^2 + b*v + c.
// If no equation is set (all zeros), the raw value is returned unchanged.
func (p *TelemetryParams) ApplyEquation(channel int, raw float64) float64 {
	if channel < 0 || channel > 4 {
		return raw
	}
	a, b, c := p.Equations[channel][0], p.Equations[channel][1], p.Equations[channel][2]
	if a == 0 && b == 0 && c == 0 {
		return raw
	}
	return a*raw*raw + b*raw + c
}

// TelemetryMetaType identifies the kind of telemetry metadata message.
type TelemetryMetaType string

const (
	TelemetryMetaPARM TelemetryMetaType = "PARM"
	TelemetryMetaUNIT TelemetryMetaType = "UNIT"
	TelemetryMetaEQNS TelemetryMetaType = "EQNS"
	TelemetryMetaBITS TelemetryMetaType = "BITS"
)

// TelemetryMetaMessage holds a parsed PARM/UNIT/EQNS/BITS message.
type TelemetryMetaMessage struct {
	Target       string            `json:"target"`       // target callsign
	MetaType     TelemetryMetaType `json:"metaType"`
	ParamNames   [5]string         `json:"paramNames,omitempty"`
	UnitLabels   [5]string         `json:"unitLabels,omitempty"`
	Equations    [5][3]float64     `json:"equations,omitempty"`
	BitSense     byte              `json:"bitSense,omitempty"`
	BitLabels    [8]string         `json:"bitLabels,omitempty"`
	ProjectTitle string            `json:"projectTitle,omitempty"`
}

// DFData holds parsed APRS direction finding report data.
type DFData struct {
	Bearing float64 `json:"bearing"` // degrees 0-360
	Number  int     `json:"number"`  // 0-9 hits indicator
	Range   float64 `json:"range"`   // miles (decoded from 2^R)
	Quality int     `json:"quality"` // 0-9 bearing accuracy
}

// MicEData holds parsed Mic-E position data.
type MicEData struct {
	Position   PositionData `json:"position"`
	MicEMsg    string       `json:"micEMsg,omitempty"`    // Mic-E message type (e.g., "Off Duty", "En Route")
	RadioModel string       `json:"radioModel,omitempty"`
}
