package message

import "time"

// MessageState represents the delivery state of an outbound message.
type MessageState int

const (
	StatePending MessageState = iota
	StateSent
	StateAcked
	StateRejected
	StateFailed
)

// Message represents an APRS message.
type Message struct {
	ID        string       `json:"id"`
	From      string       `json:"from"`
	To        string       `json:"to"`
	Body      string       `json:"body"`
	MsgNo     string       `json:"msgNo,omitempty"`
	State     MessageState `json:"state"`
	Retries   int          `json:"retries"`
	Inbound   bool         `json:"inbound"`
	Timestamp time.Time    `json:"timestamp"`
}

// Conversation groups messages with a single remote station.
type Conversation struct {
	Callsign    string     `json:"callsign"`
	Messages    []Message  `json:"messages"`
	UnreadCount int        `json:"unreadCount"`
	LastActive  time.Time  `json:"lastActive"`
	ClaimedBy   string     `json:"claimedBy,omitempty"`
	ClaimedName string     `json:"claimedName,omitempty"`
	ClaimedAt   *time.Time `json:"claimedAt,omitempty"`
}
