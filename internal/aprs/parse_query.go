package aprs

import (
	"fmt"
	"strings"
)

// parseQueryPayload parses a query payload (DTI '?').
// Format: ?QUERYTYPE? or ?QUERYTYPE?...
func parseQueryPayload(payload string) (string, error) {
	if len(payload) < 2 {
		return "", fmt.Errorf("query payload too short")
	}

	// Skip DTI '?'
	rest := payload[1:]

	// Find closing '?'
	endIdx := strings.IndexByte(rest, '?')
	if endIdx < 0 {
		// No closing ?, treat whole thing as query type
		return rest, nil
	}

	return rest[:endIdx], nil
}
