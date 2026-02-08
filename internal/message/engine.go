package message

import "github.com/narvel/nymeria/internal/aprs"

// SendFunc is a function that transmits an APRS frame.
// Used to decouple the engine from transport implementation.
type SendFunc func(frame aprs.APRSFrame) error

// Event represents a message engine event for WebSocket broadcast.
type Event struct {
	Type    string  `json:"type"`
	Message Message `json:"message"`
}

// Engine manages APRS message sending, receiving, and acknowledgements.
type Engine interface {
	// Send queues an outbound message with automatic retry.
	Send(to, body string) (*Message, error)

	// HandlePacket processes a parsed APRS packet for message content.
	HandlePacket(pkt *aprs.Packet)

	// Messages returns all messages for a given callsign (empty = all).
	Messages(callsign string) []Message

	// Conversations returns grouped conversations.
	Conversations() []Conversation

	// Events returns a channel that emits message lifecycle events.
	Events() <-chan Event

	// Import loads historical messages into the engine (e.g. from DB on startup).
	Import(msgs []Message)

	// Close shuts down the engine and cancels pending retries.
	Close()
}
