package message

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/narvel/nymeria/internal/aprs"
)

// isBulletinKey returns true if the conversation key is a bulletin or announcement.
func isBulletinKey(key string) bool {
	return strings.HasPrefix(key, "BLN") || strings.HasPrefix(key, "ANN")
}

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

// claimInfo tracks which operator has claimed a conversation.
type claimInfo struct {
	UserID    string
	UserName  string
	ClaimedAt time.Time
}

// MemoryEngine is an in-memory message engine with retry support.
type MemoryEngine struct {
	mu       sync.Mutex
	callsign string
	sendFn   SendFunc
	retryCfg RetryConfig

	messages map[string][]Message   // keyed by remote callsign
	pending  map[string]*pendingMsg // keyed by msgno
	seen     map[string]time.Time   // dedup: keyed by "from:msgno"
	claims   map[string]*claimInfo  // keyed by remote callsign
	reads    map[string]time.Time   // read markers: keyed by remote callsign
	emitMu   sync.Mutex             // guards events channel close/send
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
		claims:   make(map[string]*claimInfo),
		reads:    make(map[string]time.Time),
		events:   make(chan Event, 64),
		closed:   make(chan struct{}),
	}
}

// Send queues an outbound message with automatic retry.
func (e *MemoryEngine) Send(to, body string) (*Message, error) {
	msgNo := e.nextMsgNo()

	// Snapshot callsign under lock
	e.mu.Lock()
	callsign := e.callsign
	e.mu.Unlock()

	msg := Message{
		ID:        uuid.NewString(),
		From:      callsign,
		To:        to,
		Body:      body,
		MsgNo:     msgNo,
		State:     StateSent,
		Inbound:   false,
		Timestamp: time.Now(),
	}

	// Build and send the APRS frame
	frame := e.buildMessageFrame(callsign, to, body, msgNo)
	if err := e.sendFn(frame); err != nil {
		msg.State = StateFailed
		e.storeMessage(to, msg)
		return &msg, fmt.Errorf("send: %w", err)
	}

	e.storeMessage(to, msg)

	// Start retry timer — use a separate copy so retries don't race with the caller's *Message.
	e.mu.Lock()
	pendingCopy := msg
	pm := &pendingMsg{msg: &pendingCopy, retries: 0}
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
		e.mu.Lock()
		cs := e.callsign
		e.mu.Unlock()
		ackFrame := e.buildAckFrame(cs, from, msgData.MessageNo)
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
		ID:        uuid.NewString(),
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

// Conversations returns grouped conversations (excludes bulletins).
func (e *MemoryEngine) Conversations() []Conversation {
	e.mu.Lock()
	defer e.mu.Unlock()

	convos := make([]Conversation, 0, len(e.messages))
	for callsign, msgs := range e.messages {
		if isBulletinKey(callsign) {
			continue
		}
		// Unread is derived from the conversation's read marker: an inbound
		// message is unread iff it is newer than lastReadAt. No marker means
		// every inbound message is unread. This is the single authority — the
		// browser never re-derives unread from timestamps (it cannot represent
		// nanoseconds), it only applies counts the server hands it.
		lastRead, hasRead := e.reads[callsign]
		unread := 0
		var lastActive time.Time
		for _, m := range msgs {
			if m.Inbound && (!hasRead || m.Timestamp.After(lastRead)) {
				unread++
			}
			if m.Timestamp.After(lastActive) {
				lastActive = m.Timestamp
			}
		}
		msgsCopy := make([]Message, len(msgs))
		copy(msgsCopy, msgs)
		conv := Conversation{
			Callsign:    callsign,
			Messages:    msgsCopy,
			UnreadCount: unread,
			LastActive:  lastActive,
		}
		if hasRead {
			lr := lastRead
			conv.LastReadAt = &lr
		}
		if ci, ok := e.claims[callsign]; ok {
			conv.ClaimedBy = ci.UserID
			conv.ClaimedName = ci.UserName
			t := ci.ClaimedAt
			conv.ClaimedAt = &t
		}
		convos = append(convos, conv)
	}
	return convos
}

// Bulletins returns deduplicated bulletin messages.
// Per APRS spec, same sender + same bulletin ID = replacement (latest wins).
func (e *MemoryEngine) Bulletins() []Bulletin {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Dedup: keyed by "from:bulletinID", latest timestamp wins
	dedup := make(map[string]Bulletin)

	for convKey, msgs := range e.messages {
		if !isBulletinKey(convKey) {
			continue
		}
		for _, m := range msgs {
			dedupKey := m.From + ":" + convKey
			existing, ok := dedup[dedupKey]
			if !ok || m.Timestamp.After(existing.Timestamp) {
				dedup[dedupKey] = Bulletin{
					ID:             m.ID,
					From:           m.From,
					BulletinID:     convKey,
					Body:           m.Body,
					Timestamp:      m.Timestamp,
					IsAnnouncement: strings.HasPrefix(convKey, "ANN"),
				}
			}
		}
	}

	result := make([]Bulletin, 0, len(dedup))
	for _, b := range dedup {
		result = append(result, b)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].BulletinID != result[j].BulletinID {
			return result[i].BulletinID < result[j].BulletinID
		}
		return result[i].From < result[j].From
	})

	return result
}

// ClaimConversation assigns an operator to a conversation.
func (e *MemoryEngine) ClaimConversation(callsign, userID, userName string) error {
	now := time.Now()
	e.mu.Lock()
	e.claims[callsign] = &claimInfo{
		UserID:    userID,
		UserName:  userName,
		ClaimedAt: now,
	}
	e.mu.Unlock()

	conv := Conversation{
		Callsign:    callsign,
		ClaimedBy:   userID,
		ClaimedName: userName,
		ClaimedAt:   &now,
	}
	e.emit(Event{Type: "conversation_claimed", Conversation: &conv})
	return nil
}

// UnclaimConversation removes the operator assignment from a conversation.
func (e *MemoryEngine) UnclaimConversation(callsign string) error {
	e.mu.Lock()
	delete(e.claims, callsign)
	e.mu.Unlock()

	conv := Conversation{Callsign: callsign}
	e.emit(Event{Type: "conversation_unclaimed", Conversation: &conv})
	return nil
}

// UnclaimByUser removes all claims held by the given user.
func (e *MemoryEngine) UnclaimByUser(userID string) {
	e.mu.Lock()
	for callsign, ci := range e.claims {
		if ci.UserID == userID {
			delete(e.claims, callsign)
		}
	}
	e.mu.Unlock()
}

// MarkRead records a read marker for a conversation, clearing its unread count.
//
// The marker is clamped UP to the newest message already in the conversation.
// Message timestamps are stamped from the same wall clock, so this only bites
// after the clock steps backwards (NTP correction) — and there it is what makes
// the badge clearable at all: without the clamp, messages received before the
// step would stay permanently newer than the marker, which is exactly the bug
// this feature exists to fix. The marker also never regresses below a
// previously stored value, so a late-arriving request from a second client
// cannot resurrect already-read messages.
//
// Bulletin (BLN*/ANN*) and empty keys are rejected — bulletins are excluded
// from Conversations() and must never carry a read marker. A callsign with no
// messages is accepted idempotently but produces NO marker: there is nothing to
// read, and the caller persists whatever marker comes back, so returning one
// would let any client grow the read table without bound.
func (e *MemoryEngine) MarkRead(callsign string, readAt time.Time) (*Conversation, error) {
	if callsign == "" || isBulletinKey(callsign) {
		return nil, fmt.Errorf("cannot mark read: %q", callsign)
	}

	// UTC() strips the monotonic reading so in-memory and DB-loaded values
	// compare consistently.
	effective := readAt.UTC()

	e.mu.Lock()
	msgs := e.messages[callsign]
	if len(msgs) == 0 {
		e.mu.Unlock()
		return &Conversation{Callsign: callsign, UnreadCount: 0}, nil
	}
	for _, m := range msgs {
		if m.Inbound && m.Timestamp.After(effective) {
			effective = m.Timestamp
		}
	}
	if prev, ok := e.reads[callsign]; ok && !effective.After(prev) {
		effective = prev
	}
	e.reads[callsign] = effective
	e.mu.Unlock()

	// Messages is deliberately left nil — this payload rides the WebSocket and
	// consumers merge fields rather than replacing the message list.
	conv := &Conversation{
		Callsign:    callsign,
		UnreadCount: 0,
		LastReadAt:  &effective,
	}
	e.emit(Event{Type: "conversation_read", Conversation: conv})
	return conv, nil
}

// ImportReadState loads persisted per-conversation read markers into the engine.
// It emits no events.
func (e *MemoryEngine) ImportReadState(reads map[string]time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for callsign, t := range reads {
		e.reads[callsign] = t.UTC()
	}
}

// Events returns a channel that emits message lifecycle events.
func (e *MemoryEngine) Events() <-chan Event {
	return e.events
}

// Import loads historical messages into the engine's conversation store.
// It populates conversation maps and dedup state without emitting events.
func (e *MemoryEngine) Import(msgs []Message) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, m := range msgs {
		// Key by remote callsign (same logic as runtime message handling)
		// For bulletins (BLN*/ANN), always key by To (the bulletin ID)
		convKey := m.From
		if !m.Inbound {
			convKey = m.To
		}
		if isBulletinKey(m.To) {
			convKey = m.To
		}
		e.messages[convKey] = append(e.messages[convKey], m)

		// Populate dedup for inbound messages so we don't re-ack them
		if m.Inbound && m.MsgNo != "" {
			dedupKey := m.From + ":" + m.MsgNo
			e.seen[dedupKey] = m.Timestamp
		}
	}
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

	e.emitMu.Lock()
	close(e.events)
	e.emitMu.Unlock()
}

// --- internal ---

// UpdateCallsign updates the station callsign used for outbound messages.
func (e *MemoryEngine) UpdateCallsign(callsign string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.callsign = callsign
}

func (e *MemoryEngine) nextMsgNo() string {
	n := e.msgCounter.Add(1)
	return fmt.Sprintf("%d", n)
}

func (e *MemoryEngine) buildMessageFrame(callsign, to, body, msgNo string) aprs.APRSFrame {
	// Pad addressee to 9 characters
	padded := to
	for len(padded) < 9 {
		padded += " "
	}

	payload := fmt.Sprintf(":%s:%s{%s", padded, body, msgNo)

	return aprs.APRSFrame{
		Source:      aprs.Address{Call: callsign},
		Destination: aprs.Address{Call: "APRS"},
		Path:        []aprs.Address{{Call: "TCPIP*"}},
		Payload:     payload,
	}
}

func (e *MemoryEngine) buildAckFrame(callsign, to, msgNo string) aprs.APRSFrame {
	padded := to
	for len(padded) < 9 {
		padded += " "
	}

	payload := fmt.Sprintf(":%s:ack%s", padded, msgNo)

	return aprs.APRSFrame{
		Source:      aprs.Address{Call: callsign},
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
		e.updateMessageStateLocked(pm, StateFailed)
		delete(e.pending, msgNo)
		msgCopy := *pm.msg
		e.mu.Unlock()

		e.emit(Event{Type: "message_failed", Message: msgCopy})
		return
	}

	// Resend
	frame := e.buildMessageFrame(e.callsign, pm.msg.To, pm.msg.Body, msgNo)

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
	e.updateMessageStateLocked(pm, StateAcked)
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
	e.updateMessageStateLocked(pm, StateRejected)
	e.mu.Unlock()

	if ok {
		e.emit(Event{Type: "message_rejected", Message: *pm.msg})
	}
}

// updateMessageStateLocked updates state when we have a pending msg reference.
// Caller MUST hold e.mu.
//
// The stored copy is located by message ID, not by MsgNo. MsgNo comes from a
// counter that restarts at 1 every process start, so after a restart today's
// first outbound message shares a MsgNo with one reloaded from the database;
// a MsgNo match would stamp the ack onto the historical message and leave the
// real in-flight one retrying until it failed. IDs are UUIDs and unique.
func (e *MemoryEngine) updateMessageStateLocked(pm *pendingMsg, state MessageState) {
	if pm == nil {
		return
	}
	pm.msg.State = state
	convKey := pm.msg.To
	for i := range e.messages[convKey] {
		if e.messages[convKey][i].ID == pm.msg.ID {
			e.messages[convKey][i].State = state
			return
		}
	}
}

func (e *MemoryEngine) emit(evt Event) {
	e.emitMu.Lock()
	defer e.emitMu.Unlock()
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
