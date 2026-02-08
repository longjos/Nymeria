package aprs

import (
	"fmt"
	"strings"
)

// parseItemPayload parses an item payload (DTI ')').
// Format: )NAME!DDMM.SSN/DDDMM.SSW$... (live)
// Format: )NAME_DDMM.SSN/DDDMM.SSW$... (killed)
// Name is 3-9 chars, terminated by ! (live) or _ (killed).
func parseItemPayload(payload string) (*ItemData, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("item payload too short")
	}

	// Skip DTI ')'
	rest := payload[1:]

	// Find the live/killed separator (! or _)
	liveIdx := strings.IndexByte(rest, '!')
	killedIdx := strings.IndexByte(rest, '_')

	var name string
	var live bool
	var posStart int

	switch {
	case liveIdx >= 0 && (killedIdx < 0 || liveIdx < killedIdx):
		name = rest[:liveIdx]
		live = true
		posStart = liveIdx + 1
	case killedIdx >= 0:
		name = rest[:killedIdx]
		live = false
		posStart = killedIdx + 1
	default:
		return nil, fmt.Errorf("no live/killed indicator in item")
	}

	if len(name) < 1 || len(name) > 9 {
		return nil, fmt.Errorf("item name must be 1-9 chars, got %d", len(name))
	}

	posData := rest[posStart:]
	var pos PositionData

	if isCompressedPosition(posData) {
		if err := parseCompressedPosition(posData, &pos); err != nil {
			return nil, fmt.Errorf("invalid compressed item position: %w", err)
		}
	} else {
		if err := parseUncompressedPosition(posData, &pos); err != nil {
			return nil, fmt.Errorf("invalid item position: %w", err)
		}
	}

	return &ItemData{
		Name:     name,
		Live:     live,
		Position: pos,
	}, nil
}
