package aprs

import (
	"fmt"
)

// parseThirdPartyPayload parses a third-party forwarded packet (DTI '}').
// Format: }INNER_PACKET (full TNC2 format)
func parseThirdPartyPayload(payload string, parser *DefaultParser) (*Packet, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("third-party payload too short")
	}

	// Skip DTI '}'
	innerRaw := payload[1:]

	// Parse the inner packet as a full TNC2 frame
	innerFrame, err := ParseFrame(innerRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid third-party inner frame: %w", err)
	}

	// Recursively parse the inner packet
	innerPacket, err := parser.Parse(innerFrame)
	if err != nil {
		return nil, fmt.Errorf("failed to parse third-party inner packet: %w", err)
	}

	return innerPacket, nil
}
