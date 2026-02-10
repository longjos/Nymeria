package aprs

import (
	"fmt"
)

// Parse dispatches on the Data Type Identifier (DTI) — the first byte of the payload.
func (p *DefaultParser) Parse(frame APRSFrame) (*Packet, error) {
	if len(frame.Payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	pkt := &Packet{
		Frame: frame,
	}

	dti := frame.Payload[0]

	switch dti {
	case '!', '=':
		// Position without timestamp (! = no messaging, = = messaging capable)
		pos, err := parsePositionPayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("position parse: %w", err)
		}
		// Check if this is actually a weather report (symbol code '_')
		if pos.Symbol.Code == '_' {
			wx := parseWeatherFromPosition(pos)
			pkt.Type = PacketTypeWeather
			pkt.Position = pos
			pkt.Weather = wx
		} else {
			pkt.Type = PacketTypePosition
			pkt.Position = pos
		}

	case '/', '@':
		// Position with timestamp (/ = no messaging, @ = messaging capable)
		pos, err := parsePositionPayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("position parse: %w", err)
		}
		// Check if this is actually a weather report (symbol code '_')
		if pos.Symbol.Code == '_' {
			wx := parseWeatherFromPosition(pos)
			pkt.Type = PacketTypeWeather
			pkt.Position = pos
			pkt.Weather = wx
		} else {
			pkt.Type = PacketTypePosition
			pkt.Position = pos
		}

	case ':':
		// Message
		msg, err := parseMessagePayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("message parse: %w", err)
		}
		// Intercept telemetry metadata (PARM/UNIT/EQNS/BITS) before it
		// reaches the message engine as a normal user message.
		if msg != nil && !msg.IsAck && !msg.IsRej && IsTelemetryMeta(msg.Text) {
			meta, metaErr := ParseTelemetryMeta(msg.Text)
			if metaErr == nil {
				meta.Target = msg.Addressee
				pkt.Type = PacketTypeTelemetry
				pkt.TelemetryMeta = meta
				break
			}
		}
		pkt.Type = PacketTypeMessage
		pkt.Message = msg

	case ';':
		// Object
		obj, err := parseObjectPayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("object parse: %w", err)
		}
		pkt.Type = PacketTypeObject
		pkt.Object = obj

	case ')':
		// Item
		item, err := parseItemPayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("item parse: %w", err)
		}
		pkt.Type = PacketTypeItem
		pkt.Item = item

	case '_':
		// Positionless weather
		wx, _, err := parseWeatherPayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("weather parse: %w", err)
		}
		pkt.Type = PacketTypeWeather
		pkt.Weather = wx

	case '>':
		// Status
		status, err := parseStatusPayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("status parse: %w", err)
		}
		pkt.Type = PacketTypeStatus
		pkt.Status = status

	case 'T':
		// Telemetry
		tel, err := parseTelemetryPayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("telemetry parse: %w", err)
		}
		pkt.Type = PacketTypeTelemetry
		pkt.Telemetry = tel

	case '`', '\'':
		// Mic-E
		micE, err := parseMicEPayload(frame)
		if err != nil {
			return nil, fmt.Errorf("mic-e parse: %w", err)
		}
		pkt.Type = PacketTypeMicE
		pkt.MicE = micE

	case '}':
		// Third-party forwarding
		inner, err := parseThirdPartyPayload(frame.Payload, p)
		if err != nil {
			return nil, fmt.Errorf("third-party parse: %w", err)
		}
		pkt.Type = PacketTypeThirdParty
		pkt.ThirdParty = inner

	case '?':
		// Query
		query, err := parseQueryPayload(frame.Payload)
		if err != nil {
			return nil, fmt.Errorf("query parse: %w", err)
		}
		pkt.Type = PacketTypeQuery
		pkt.Query = query

	default:
		// Unknown DTI — not an error, just unknown type
		pkt.Type = PacketTypeUnknown
	}

	// Direction Finding: extract DF data when DF symbol detected
	if pkt.Position != nil && isDFSymbol(pkt.Position.Symbol) {
		df, remain := parseDFComment(pkt.Position.Comment)
		if df != nil {
			pkt.DF = df
			pkt.Position.Comment = remain
		}
	}

	// APRS 1.2: Extract frequency data from position comments
	if pkt.Position != nil && pkt.Position.Comment != "" {
		pkt.Frequency = parseFrequency(pkt.Position.Comment)
	}

	return pkt, nil
}
