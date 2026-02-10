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
	Frame         APRSFrame              `json:"frame"`
	Type          PacketType             `json:"type"`
	Position      *PositionData          `json:"position,omitempty"`
	Message       *MessageData           `json:"message,omitempty"`
	Object        *ObjectData            `json:"object,omitempty"`
	Weather       *WeatherData           `json:"weather,omitempty"`
	Status        *StatusData            `json:"status,omitempty"`
	Item          *ItemData              `json:"item,omitempty"`
	Telemetry     *TelemetryData         `json:"telemetry,omitempty"`
	MicE          *MicEData              `json:"micE,omitempty"`
	DF            *DFData                `json:"df,omitempty"`            // direction finding report data
	Query         string                 `json:"query,omitempty"`         // query type for PacketTypeQuery (e.g., "APRS", "WX")
	ThirdParty    *Packet                `json:"thirdParty,omitempty"`    // inner packet for PacketTypeThirdParty
	Frequency     *FrequencyData         `json:"frequency,omitempty"`     // APRS 1.2: parsed frequency from comment
	TelemetryMeta *TelemetryMetaMessage  `json:"telemetryMeta,omitempty"` // PARM/UNIT/EQNS/BITS metadata
}

// String returns a lowercase string name for the packet type.
func (t PacketType) String() string {
	switch t {
	case PacketTypePosition:
		return "position"
	case PacketTypeMessage:
		return "message"
	case PacketTypeObject:
		return "object"
	case PacketTypeItem:
		return "item"
	case PacketTypeWeather:
		return "weather"
	case PacketTypeStatus:
		return "status"
	case PacketTypeTelemetry:
		return "telemetry"
	case PacketTypeMicE:
		return "micE"
	case PacketTypeQuery:
		return "query"
	case PacketTypeThirdParty:
		return "thirdParty"
	default:
		return "unknown"
	}
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
