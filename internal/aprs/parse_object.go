package aprs

import (
	"fmt"
)

// parseObjectPayload parses an object payload (DTI ';').
// Format: ;NAME_____*DDHHMMzDDMM.SSN/DDDMM.SSW$...
// Name is exactly 9 chars, followed by * (live) or _ (killed), then timestamp + position.
func parseObjectPayload(payload string) (*ObjectData, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("object payload too short")
	}

	// Skip DTI ';'
	rest := payload[1:]

	if len(rest) < 10 {
		return nil, fmt.Errorf("object payload too short for name")
	}

	// Object name is 9 chars
	name := rest[:9]
	// Trim trailing spaces from name
	trimmedName := ""
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] != ' ' {
			trimmedName = name[:i+1]
			break
		}
	}

	// Live/killed indicator
	indicator := rest[9]
	live := indicator == '*'

	// Timestamp + position follows
	tsAndPos := rest[10:]
	if len(tsAndPos) < 7 {
		return nil, fmt.Errorf("object payload too short for timestamp")
	}

	ts, err := parseTimestamp(tsAndPos[:7])
	if err != nil {
		return nil, fmt.Errorf("invalid object timestamp: %w", err)
	}

	// Parse position after timestamp (use position DTI '/' to trigger uncompressed parse)
	posData := tsAndPos[7:]
	var pos PositionData
	pos.Timestamp = ts

	if len(posData) >= 19 {
		if err := parseUncompressedPosition(posData, &pos); err != nil {
			return nil, fmt.Errorf("invalid object position: %w", err)
		}
	} else if len(posData) >= 10 && isCompressedPosition(posData) {
		if err := parseCompressedPosition(posData, &pos); err != nil {
			return nil, fmt.Errorf("invalid compressed object position: %w", err)
		}
	} else {
		return nil, fmt.Errorf("object position data too short")
	}

	return &ObjectData{
		Name:      trimmedName,
		Live:      live,
		Timestamp: ts,
		Position:  pos,
	}, nil
}
