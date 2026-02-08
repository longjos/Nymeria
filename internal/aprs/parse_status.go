package aprs

import (
	"fmt"
	"regexp"
)

var maidenheadRe = regexp.MustCompile(`^([A-R]{2}[0-9]{2}[A-X]{2})/?(.*)$`)

// parseStatusPayload parses a status payload (DTI '>').
// Format: >status text
// With Maidenhead: >IO91SX/G (6-char grid locator followed by / and optional text)
func parseStatusPayload(payload string) (*StatusData, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("status payload too short")
	}

	// Skip DTI '>'
	text := payload[1:]

	status := &StatusData{}

	// Check for Maidenhead grid locator pattern
	if m := maidenheadRe.FindStringSubmatch(text); m != nil {
		status.Maidenhead = m[1]
		status.Text = m[2]
	} else {
		status.Text = text
	}

	return status, nil
}
