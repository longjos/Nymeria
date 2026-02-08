package aprs

import (
	"fmt"
	"strings"
)

// parseMessagePayload parses a message payload (DTI ':').
// Format: :ADDRESSEE:message text{msgno
// Ack: :ADDRESSEE:ackNNN
// Rej: :ADDRESSEE:rejNNN
func parseMessagePayload(payload string) (*MessageData, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("message payload too short")
	}

	// Skip DTI ':'
	rest := payload[1:]

	// Addressee is 9 characters padded with spaces, followed by ':'
	if len(rest) < 10 {
		return nil, fmt.Errorf("message payload too short for addressee")
	}

	// Find the colon after addressee (should be at position 9)
	colonIdx := strings.IndexByte(rest[1:], ':')
	if colonIdx < 0 {
		return nil, fmt.Errorf("no colon after addressee in message")
	}
	colonIdx++ // adjust for the offset

	addressee := strings.TrimRight(rest[:colonIdx], " ")
	text := rest[colonIdx+1:]

	msg := &MessageData{
		Addressee: addressee,
	}

	// Check for ack/rej
	if strings.HasPrefix(text, "ack") {
		msg.IsAck = true
		msg.AckMsgNo = text[3:]
		return msg, nil
	}
	if strings.HasPrefix(text, "rej") {
		msg.IsRej = true
		msg.AckMsgNo = text[3:]
		return msg, nil
	}

	// Check for message number {NNN
	if idx := strings.LastIndexByte(text, '{'); idx >= 0 {
		msg.MessageNo = text[idx+1:]
		text = text[:idx]
	}

	msg.Text = text

	// Check for auto-answer convention: message starts with "AA:" prefix
	if strings.HasPrefix(msg.Text, "AA:") {
		msg.IsAutoAnswer = true
	}

	return msg, nil
}
