package message

import "time"

// Message represents an APRS message.
type Message struct {
	ID          string    `json:"id"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Body        string    `json:"body"`
	AckRequired bool      `json:"ackRequired"`
	Timestamp   time.Time `json:"timestamp"`
	Acked       bool      `json:"acked"`
}

// Conversation groups messages with a single remote station.
type Conversation struct {
	Callsign    string    `json:"callsign"`
	Messages    []Message `json:"messages"`
	UnreadCount int       `json:"unreadCount"`
}
