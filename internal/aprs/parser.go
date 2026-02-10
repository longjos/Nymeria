package aprs

// PacketType identifies the type of an APRS packet.
type PacketType int

const (
	PacketTypeUnknown PacketType = iota
	PacketTypePosition
	PacketTypeMessage
	PacketTypeObject
	PacketTypeItem
	PacketTypeWeather
	PacketTypeStatus
	PacketTypeTelemetry
	PacketTypeMicE
	PacketTypeQuery
	PacketTypeThirdParty
)

// Packet represents a parsed APRS packet.
type Packet struct {
	Frame      APRSFrame
	Type       PacketType
	Position   *PositionData
	Message    *MessageData
	Object     *ObjectData
	Weather    *WeatherData
	Status     *StatusData
	Item       *ItemData
	Telemetry  *TelemetryData
	MicE       *MicEData
	DF         *DFData         // direction finding report data
	Query      string         // query type for PacketTypeQuery (e.g., "APRS", "WX")
	ThirdParty *Packet        // inner packet for PacketTypeThirdParty
	Frequency  *FrequencyData // APRS 1.2: parsed frequency from comment
}

// Parser parses raw APRS frames into structured packets.
type Parser interface {
	Parse(frame APRSFrame) (*Packet, error)
}

// DefaultParser is the standard APRS packet parser.
type DefaultParser struct{}

// NewParser creates a new DefaultParser.
func NewParser() *DefaultParser {
	return &DefaultParser{}
}
