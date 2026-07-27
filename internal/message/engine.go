package message

import (
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// SendFunc is a function that transmits an APRS frame.
// Used to decouple the engine from transport implementation.
type SendFunc func(frame aprs.APRSFrame) error

// Event represents a message engine event for WebSocket broadcast.
type Event struct {
	Type         string        `json:"type"`
	Message      Message       `json:"message,omitempty"`
	Conversation *Conversation `json:"conversation,omitempty"`
}

// Engine manages APRS message sending, receiving, and acknowledgements.
type Engine interface {
	// Send queues an outbound message with automatic retry.
	Send(to, body string) (*Message, error)

	// HandlePacket processes a parsed APRS packet for message content.
	HandlePacket(pkt *aprs.Packet)

	// Messages returns all messages for a given callsign (empty = all).
	Messages(callsign string) []Message

	// Conversations returns grouped conversations (excludes bulletins).
	Conversations() []Conversation

	// Bulletins returns deduplicated bulletin messages.
	Bulletins() []Bulletin

	// Events returns a channel that emits message lifecycle events.
	Events() <-chan Event

	// Import loads historical messages into the engine (e.g. from DB on startup).
	Import(msgs []Message)

	// MarkRead records a read marker for a conversation, clearing its unread
	// count. Bulletin and empty keys are rejected. Unknown callsigns are
	// accepted idempotently and create no conversation.
	MarkRead(callsign string, readAt time.Time) (*Conversation, error)

	// ImportReadState loads persisted per-conversation read markers into the
	// engine (e.g. from DB on startup). It emits no events.
	ImportReadState(reads map[string]time.Time)

	// ClaimConversation assigns an operator to a conversation.
	ClaimConversation(callsign, userID, userName string) error

	// UnclaimConversation removes the operator assignment from a conversation.
	UnclaimConversation(callsign string) error

	// UnclaimByUser removes all claims held by the given user.
	UnclaimByUser(userID string)

	// Close shuts down the engine and cancels pending retries.
	Close()
}
