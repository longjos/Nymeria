package message

// Engine manages APRS message sending, receiving, and acknowledgements.
type Engine interface {
	// Send queues an outbound message.
	Send(to, body string) (*Message, error)

	// Receive handles an inbound message.
	Receive(msg Message) error

	// Messages returns all messages.
	Messages() []Message

	// Conversations returns grouped conversations.
	Conversations() []Conversation
}
