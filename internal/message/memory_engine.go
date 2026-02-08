package message

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// RetryConfig controls message retry timing.
type RetryConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxRetries      int
}

// DefaultRetryConfig returns production retry settings.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		InitialInterval: 30 * time.Second,
		MaxInterval:     5 * time.Minute,
		MaxRetries:      7,
	}
}

// pendingMsg tracks an outbound message awaiting ack.
type pendingMsg struct {
	msg     *Message
	timer   *time.Timer
	retries int
}

// MemoryEngine is an in-memory message engine with retry support.
type MemoryEngine struct {
	mu       sync.Mutex
	callsign string
	sendFn   SendFunc
	retryCfg RetryConfig

	messages map[string][]Message // keyed by remote callsign
	pending  map[string]*pendingMsg // keyed by msgno
	seen     map[string]time.Time   // dedup: keyed by "from:msgno"
	events   chan Event

	msgCounter atomic.Int64
	closed     chan struct{}
}

// NewMemoryEngine creates a new in-memory message engine.
func NewMemoryEngine(callsign string, sendFn SendFunc, cfg RetryConfig) *MemoryEngine {
	return &MemoryEngine{
		callsign: callsign,
		sendFn:   sendFn,
		retryCfg: cfg,
		messages: make(map[string][]Message),
		pending:  make(map[string]*pendingMsg),
		seen:     make(map[string]time.Time),
		events:   make(chan Event, 64),
		closed:   make(chan struct{}),
	}
}

// Send queues an outbound message with automatic retry.
func (e *MemoryEngine) Send(to, body string) (*Message, error) {
	msgNo := e.nextMsgNo()

	msg := Message{
		ID:        fmt.Sprintf("%s-%s-%s", e.callsign, to, msgNo),
		From:      e.callsign,
		To:        to,
		Body:      body,
		MsgNo:     msgNo,
		State:     StateSent,
		Inbound:   false,
		Timestamp: time.Now(),
	}

	// Build and send the APRS frame
	frame := e.buildMessageFrame(to, body, msgNo)
	if err := e.sendFn(frame); err != nil {
		msg.State = StateFailed
		e.storeMessage(to, msg)
		return &msg, fmt.Errorf("send: %w", err)
	}

	e.storeMessage(to, msg)

	// Start retry timer
	e.mu.Lock()
	pm := &pendingMsg{msg: &msg, retries: 0}
	e.pending[msgNo] = pm
	pm.timer = time.AfterFunc(e.retryCfg.InitialInterval, func() {
		e.retryMessage(msgNo)
	})
	e.mu.Unlock()

	e.emit(Event{Type: "message_sent", Message: msg})
	return &msg, nil
}

// HandlePacket processes a parsed APRS packet for message content.
func (e *MemoryEngine) HandlePacket(pkt *aprs.Packet) {
	if pkt.Type != aprs.PacketTypeMessage || pkt.Message == nil {
		return
	}

	from := pkt.Frame.Source.String()
	msgData := pkt.Message

	// Handle ack/rej for our outbound messages
	if msgData.IsAck {
		e.handleAck(msgData.AckMsgNo)
		return
	}
	if msgData.IsRej {
		e.handleRej(msgData.AckMsgNo)
		return
	}

	// Determine if this is a bulletin
	isBulletin := strings.HasPrefix(msgData.Addressee, "BLN") ||
		strings.HasPrefix(msgData.Addressee, "ANN")

	// Only process messages addressed to us or bulletins
	if !isBulletin && !strings.EqualFold(msgData.Addressee, e.callsign) {
		return
	}

	// For bulletins, key by addressee; for DMs, key by sender
	convKey := from
	if isBulletin {
		convKey = msgData.Addressee
	}

	// Send ack if message has a number (always ack, even dupes)
	if msgData.MessageNo != "" {
		ackFrame := e.buildAckFrame(from, msgData.MessageNo)
		e.sendFn(ackFrame)
	}

	// Dedup check
	if msgData.MessageNo != "" {
		dedupKey := from + ":" + msgData.MessageNo
		e.mu.Lock()
		if _, seen := e.seen[dedupKey]; seen {
			e.mu.Unlock()
			return
		}
		e.seen[dedupKey] = time.Now()
		e.mu.Unlock()
	}

	msg := Message{
		ID:        fmt.Sprintf("%s-%s-%s", from, e.callsign, msgData.MessageNo),
		From:      from,
		To:        msgData.Addressee,
		Body:      msgData.Text,
		MsgNo:     msgData.MessageNo,
		State:     StateAcked,
		Inbound:   true,
		Timestamp: time.Now(),
	}

	e.storeMessage(convKey, msg)
	e.emit(Event{Type: "message_received", Message: msg})
}

// Messages returns all messages for a given callsign. Empty callsign returns all.
func (e *MemoryEngine) Messages(callsign string) []Message {
	e.mu.Lock()
	defer e.mu.Unlock()

	if callsign != "" {
		msgs := make([]Message, len(e.messages[callsign]))
		copy(msgs, e.messages[callsign])
		return msgs
	}

	var all []Message
	for _, msgs := range e.messages {
		all = append(all, msgs...)
	}
	return all
}

// Conversations returns grouped conversations.
func (e *MemoryEngine) Conversations() []Conversation {
	e.mu.Lock()
	defer e.mu.Unlock()

	convos := make([]Conversation, 0, len(e.messages))
	for callsign, msgs := range e.messages {
		unread := 0
		var lastActive time.Time
		for _, m := range msgs {
			if m.Inbound && m.State != StateAcked {
				unread++
			}
			if m.Timestamp.After(lastActive) {
				lastActive = m.Timestamp
			}
		}
		msgsCopy := make([]Message, len(msgs))
		copy(msgsCopy, msgs)
		convos = append(convos, Conversation{
			Callsign:    callsign,
			Messages:    msgsCopy,
			UnreadCount: unread,
			LastActive:  lastActive,
		})
	}
	return convos
}

// Events returns a channel that emits message lifecycle events.
func (e *MemoryEngine) Events() <-chan Event {
	return e.events
}

// Close shuts down the engine and cancels pending retries.
func (e *MemoryEngine) Close() {
	select {
	case <-e.closed:
		return
	default:
	}
	close(e.closed)

	e.mu.Lock()
	for _, pm := range e.pending {
		if pm.timer != nil {
			pm.timer.Stop()
		}
	}
	e.pending = make(map[string]*pendingMsg)
	e.mu.Unlock()

	close(e.events)
}

// --- internal ---

func (e *MemoryEngine) nextMsgNo() string {
	n := e.msgCounter.Add(1)
	return fmt.Sprintf("%d", n)
}

func (e *MemoryEngine) buildMessageFrame(to, body, msgNo string) aprs.APRSFrame {
	// Pad addressee to 9 characters
	padded := to
	for len(padded) < 9 {
		padded += " "
	}

	payload := fmt.Sprintf(":%s:%s{%s", padded, body, msgNo)

	return aprs.APRSFrame{
		Source:      aprs.Address{Call: e.callsign},
		Destination: aprs.Address{Call: "APRS"},
		Path:        []aprs.Address{{Call: "TCPIP*"}},
		Payload:     payload,
	}
}

func (e *MemoryEngine) buildAckFrame(to, msgNo string) aprs.APRSFrame {
	padded := to
	for len(padded) < 9 {
		padded += " "
	}

	payload := fmt.Sprintf(":%s:ack%s", padded, msgNo)

	return aprs.APRSFrame{
		Source:      aprs.Address{Call: e.callsign},
		Destination: aprs.Address{Call: "APRS"},
		Path:        []aprs.Address{{Call: "TCPIP*"}},
		Payload:     payload,
	}
}

func (e *MemoryEngine) storeMessage(convKey string, msg Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages[convKey] = append(e.messages[convKey], msg)
}

func (e *MemoryEngine) retryMessage(msgNo string) {
	select {
	case <-e.closed:
		return
	default:
	}

	e.mu.Lock()
	pm, ok := e.pending[msgNo]
	if !ok {
		e.mu.Unlock()
		return
	}

	pm.retries++
	if pm.retries > e.retryCfg.MaxRetries {
		// Max retries exhausted — mark as failed
		e.updateMessageStateLocked(pm, msgNo, StateFailed)
		delete(e.pending, msgNo)
		msgCopy := *pm.msg
		e.mu.Unlock()

		e.emit(Event{Type: "message_failed", Message: msgCopy})
		return
	}

	// Resend
	frame := e.buildMessageFrame(pm.msg.To, pm.msg.Body, msgNo)

	// Calculate next backoff
	interval := e.retryCfg.InitialInterval
	for i := 0; i < pm.retries; i++ {
		interval *= 2
	}
	if interval > e.retryCfg.MaxInterval {
		interval = e.retryCfg.MaxInterval
	}

	pm.timer = time.AfterFunc(interval, func() {
		e.retryMessage(msgNo)
	})
	e.mu.Unlock()

	e.sendFn(frame)
}

func (e *MemoryEngine) handleAck(msgNo string) {
	e.mu.Lock()
	pm, ok := e.pending[msgNo]
	if ok {
		if pm.timer != nil {
			pm.timer.Stop()
		}
		delete(e.pending, msgNo)
	}
	e.updateMessageStateLocked(pm, msgNo, StateAcked)
	e.mu.Unlock()

	if ok {
		e.emit(Event{Type: "message_acked", Message: *pm.msg})
	}
}

func (e *MemoryEngine) handleRej(msgNo string) {
	e.mu.Lock()
	pm, ok := e.pending[msgNo]
	if ok {
		if pm.timer != nil {
			pm.timer.Stop()
		}
		delete(e.pending, msgNo)
	}
	e.updateMessageStateLocked(pm, msgNo, StateRejected)
	e.mu.Unlock()

	if ok {
		e.emit(Event{Type: "message_rejected", Message: *pm.msg})
	}
}

// updateMessageState updates the state of a message in the conversation store.
// Must NOT hold e.mu when calling.
func (e *MemoryEngine) updateMessageState(convKey, msgNo string, state MessageState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.messages[convKey] {
		if e.messages[convKey][i].MsgNo == msgNo {
			e.messages[convKey][i].State = state
			return
		}
	}
}

// updateMessageStateLocked updates state when we have a pending msg reference.
// Caller MUST hold e.mu.
func (e *MemoryEngine) updateMessageStateLocked(pm *pendingMsg, msgNo string, state MessageState) {
	if pm == nil {
		return
	}
	pm.msg.State = state
	for convKey, msgs := range e.messages {
		for i := range msgs {
			if msgs[i].MsgNo == msgNo {
				e.messages[convKey][i].State = state
				return
			}
		}
	}
}

func (e *MemoryEngine) emit(evt Event) {
	select {
	case <-e.closed:
		return
	default:
	}
	select {
	case e.events <- evt:
	default:
	}
}
