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
)

// Packet represents a parsed APRS packet.
type Packet struct {
	Frame    APRSFrame
	Type     PacketType
	Position *PositionData
	Message  *MessageData
	Object   *ObjectData
	Weather  *WeatherData
}

// Parser parses raw APRS frames into structured packets.
type Parser interface {
	Parse(frame APRSFrame) (*Packet, error)
}
