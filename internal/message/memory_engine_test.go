package message

import (
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// mockSendFunc captures sent frames for verification.
type mockSendFunc struct {
	frames []aprs.APRSFrame
}

func (m *mockSendFunc) send(frame aprs.APRSFrame) error {
	m.frames = append(m.frames, frame)
	return nil
}

func newTestEngine() (*MemoryEngine, *mockSendFunc) {
	mock := &mockSendFunc{}
	eng := NewMemoryEngine("N0CALL", mock.send, RetryConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		MaxRetries:      3,
	})
	return eng, mock
}

func TestSendFormat(t *testing.T) {
	eng, mock := newTestEngine()
	defer eng.Close()

	msg, err := eng.Send("W3ADO-5", "Hello World")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if msg.From != "N0CALL" {
		t.Errorf("from = %q, want N0CALL", msg.From)
	}
	if msg.To != "W3ADO-5" {
		t.Errorf("to = %q, want W3ADO-5", msg.To)
	}
	if msg.Body != "Hello World" {
		t.Errorf("body = %q, want Hello World", msg.Body)
	}
	if msg.MsgNo == "" {
		t.Error("msgNo should not be empty")
	}
	if msg.State != StateSent {
		t.Errorf("state = %d, want %d (StateSent)", msg.State, StateSent)
	}

	// Check frame was sent
	if len(mock.frames) == 0 {
		t.Fatal("no frames sent")
	}

	frame := mock.frames[0]
	if frame.Source.Call != "N0CALL" {
		t.Errorf("frame source = %q, want N0CALL", frame.Source.Call)
	}
	// Payload format: :ADDRESSEE :body{msgno
	// Addressee is padded to 9 chars
	payload := frame.Payload
	if !strings.HasPrefix(payload, ":W3ADO-5  :Hello World{") {
		t.Errorf("payload = %q, want prefix :W3ADO-5  :Hello World{", payload)
	}
}

func TestSequentialMsgNo(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	msg1, _ := eng.Send("W3ADO", "First")
	msg2, _ := eng.Send("W3ADO", "Second")

	// Message numbers should be sequential
	if msg1.MsgNo == msg2.MsgNo {
		t.Errorf("message numbers should differ: %q == %q", msg1.MsgNo, msg2.MsgNo)
	}
}

func TestAckCancelsRetry(t *testing.T) {
	eng, mock := newTestEngine()
	defer eng.Close()

	msg, _ := eng.Send("W3ADO", "Test")
	msgNo := msg.MsgNo

	// Wait a bit, then send ack
	time.Sleep(5 * time.Millisecond)
	initialFrameCount := len(mock.frames)

	// Simulate inbound ack packet
	ackFrame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "W3ADO"},
		Destination: aprs.Address{Call: "N0CALL"},
		Payload:     ":N0CALL   :ack" + msgNo,
	}
	parser := aprs.NewParser()
	pkt, err := parser.Parse(ackFrame)
	if err != nil {
		t.Fatalf("parse ack: %v", err)
	}
	eng.HandlePacket(pkt)

	// Check message is now acked
	msgs := eng.Messages("W3ADO")
	found := false
	for _, m := range msgs {
		if m.MsgNo == msgNo {
			if m.State != StateAcked {
				t.Errorf("state = %d, want %d (StateAcked)", m.State, StateAcked)
			}
			found = true
		}
	}
	if !found {
		t.Error("acked message not found in messages")
	}

	// Wait for what would be a retry interval — no new frames should arrive
	time.Sleep(30 * time.Millisecond)
	if len(mock.frames) > initialFrameCount {
		t.Errorf("got %d extra frames after ack, expected 0", len(mock.frames)-initialFrameCount)
	}
}

func TestInboundTriggersAck(t *testing.T) {
	eng, mock := newTestEngine()
	defer eng.Close()

	// Simulate inbound message with msgno
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "W3ADO", SSID: 1},
		Destination: aprs.Address{Call: "N0CALL"},
		Payload:     ":N0CALL   :Hello from W3ADO{789",
	}
	parser := aprs.NewParser()
	pkt, err := parser.Parse(frame)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eng.HandlePacket(pkt)

	// Should have sent an ack frame
	if len(mock.frames) == 0 {
		t.Fatal("no ack frame sent")
	}

	ackFrame := mock.frames[0]
	// Ack format: :ADDRESSEE :ackNNN
	if !strings.Contains(ackFrame.Payload, ":ack789") {
		t.Errorf("ack payload = %q, want to contain :ack789", ackFrame.Payload)
	}

	// Check inbound message was stored
	msgs := eng.Messages("W3ADO-1")
	if len(msgs) == 0 {
		t.Fatal("no inbound messages stored")
	}
	if msgs[0].Body != "Hello from W3ADO" {
		t.Errorf("body = %q, want Hello from W3ADO", msgs[0].Body)
	}
	if !msgs[0].Inbound {
		t.Error("message should be marked inbound")
	}
}

func TestInboundDedup(t *testing.T) {
	eng, mock := newTestEngine()
	defer eng.Close()

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "W3ADO"},
		Destination: aprs.Address{Call: "N0CALL"},
		Payload:     ":N0CALL   :Repeated msg{555",
	}
	parser := aprs.NewParser()
	pkt, _ := parser.Parse(frame)

	// Handle same packet twice
	eng.HandlePacket(pkt)
	eng.HandlePacket(pkt)

	// Should only have one message stored
	msgs := eng.Messages("W3ADO")
	if len(msgs) != 1 {
		t.Errorf("got %d messages, want 1 (dedup)", len(msgs))
	}

	// Should have sent two acks though (ack is always sent)
	ackCount := 0
	for _, f := range mock.frames {
		if strings.Contains(f.Payload, ":ack555") {
			ackCount++
		}
	}
	if ackCount != 2 {
		t.Errorf("got %d acks, want 2", ackCount)
	}
}

func TestConversations(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	// Send a message
	eng.Send("W3ADO", "Hello")

	// Receive a message from different station
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "KJ4ERJ"},
		Destination: aprs.Address{Call: "N0CALL"},
		Payload:     ":N0CALL   :Hi there",
	}
	parser := aprs.NewParser()
	pkt, _ := parser.Parse(frame)
	eng.HandlePacket(pkt)

	convos := eng.Conversations()
	if len(convos) != 2 {
		t.Fatalf("got %d conversations, want 2", len(convos))
	}

	// Check each conversation has messages
	for _, c := range convos {
		if len(c.Messages) == 0 {
			t.Errorf("conversation %q has no messages", c.Callsign)
		}
	}
}

func TestBulletinHandling(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	// Bulletin has addressee starting with BLN
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "W3ADO"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     ":BLN3     :Weather alert for area",
	}
	parser := aprs.NewParser()
	pkt, _ := parser.Parse(frame)
	eng.HandlePacket(pkt)

	// Bulletins should be stored in a "BLN" conversation
	msgs := eng.Messages("BLN3")
	if len(msgs) != 1 {
		t.Fatalf("got %d bulletin messages, want 1", len(msgs))
	}
	if msgs[0].Body != "Weather alert for area" {
		t.Errorf("body = %q, want Weather alert for area", msgs[0].Body)
	}
}

func TestRetryExhaustion(t *testing.T) {
	eng, mock := newTestEngine()
	defer eng.Close()

	eng.Send("W3ADO", "No reply expected")

	// Wait for retries to exhaust (3 retries * ~10ms intervals + some buffer)
	time.Sleep(200 * time.Millisecond)

	// Should have sent initial + retries
	if len(mock.frames) < 2 {
		t.Errorf("expected at least 2 frames (initial + retries), got %d", len(mock.frames))
	}

	// Check message state is now failed
	msgs := eng.Messages("W3ADO")
	for _, m := range msgs {
		if m.MsgNo != "" && m.State != StateFailed {
			t.Errorf("state = %d after retry exhaustion, want %d (StateFailed)", m.State, StateFailed)
		}
	}
}

func TestEventsChannel(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	// Send a message
	eng.Send("W3ADO", "Event test")

	// Should get an event
	select {
	case evt := <-eng.Events():
		if evt.Type != "message_sent" {
			t.Errorf("event type = %q, want message_sent", evt.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestClaimConversation(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	// Claim before any messages exist (pre-claim) should succeed
	err := eng.ClaimConversation("W3ADO", "user1", "Alice")
	if err != nil {
		t.Fatalf("ClaimConversation: %v", err)
	}

	// Verify claim shows up in Conversations after a message arrives
	eng.Send("W3ADO", "Hello")
	convos := eng.Conversations()
	var found *Conversation
	for i := range convos {
		if convos[i].Callsign == "W3ADO" {
			found = &convos[i]
			break
		}
	}
	if found == nil {
		t.Fatal("W3ADO conversation not found")
	}
	if found.ClaimedBy != "user1" {
		t.Errorf("ClaimedBy = %q, want user1", found.ClaimedBy)
	}
	if found.ClaimedName != "Alice" {
		t.Errorf("ClaimedName = %q, want Alice", found.ClaimedName)
	}
	if found.ClaimedAt == nil {
		t.Error("ClaimedAt should not be nil")
	}
}

func TestUnclaimConversation(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	eng.Send("W3ADO", "Hello")
	eng.ClaimConversation("W3ADO", "user1", "Alice")
	eng.UnclaimConversation("W3ADO")

	convos := eng.Conversations()
	for _, c := range convos {
		if c.Callsign == "W3ADO" {
			if c.ClaimedBy != "" {
				t.Errorf("ClaimedBy = %q after unclaim, want empty", c.ClaimedBy)
			}
			if c.ClaimedAt != nil {
				t.Error("ClaimedAt should be nil after unclaim")
			}
			return
		}
	}
	t.Fatal("W3ADO conversation not found")
}

func TestUnclaimByUser(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	eng.Send("W3ADO", "Hello")
	eng.Send("KJ4ERJ", "Hi")

	eng.ClaimConversation("W3ADO", "user1", "Alice")
	eng.ClaimConversation("KJ4ERJ", "user1", "Alice")

	// Unclaim all conversations owned by user1
	eng.UnclaimByUser("user1")

	convos := eng.Conversations()
	for _, c := range convos {
		if c.ClaimedBy != "" {
			t.Errorf("conversation %q still claimed by %q after UnclaimByUser", c.Callsign, c.ClaimedBy)
		}
	}
}

func TestClaimEvents(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	// Drain the events channel from Send
	eng.Send("W3ADO", "Hello")
	select {
	case <-eng.Events():
	case <-time.After(time.Second):
	}

	eng.ClaimConversation("W3ADO", "user1", "Alice")

	select {
	case evt := <-eng.Events():
		if evt.Type != "conversation_claimed" {
			t.Errorf("event type = %q, want conversation_claimed", evt.Type)
		}
		if evt.Conversation == nil {
			t.Fatal("event Conversation should not be nil")
		}
		if evt.Conversation.ClaimedBy != "user1" {
			t.Errorf("event ClaimedBy = %q, want user1", evt.Conversation.ClaimedBy)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for claim event")
	}

	eng.UnclaimConversation("W3ADO")

	select {
	case evt := <-eng.Events():
		if evt.Type != "conversation_unclaimed" {
			t.Errorf("event type = %q, want conversation_unclaimed", evt.Type)
		}
		if evt.Conversation == nil {
			t.Fatal("event Conversation should not be nil")
		}
		if evt.Conversation.Callsign != "W3ADO" {
			t.Errorf("event Callsign = %q, want W3ADO", evt.Conversation.Callsign)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for unclaim event")
	}
}

func TestClaimNonexistentConversation(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	// Pre-claiming a conversation before any messages exist should be OK
	err := eng.ClaimConversation("NOCALL", "user1", "Alice")
	if err != nil {
		t.Errorf("pre-claiming nonexistent conversation should not error, got: %v", err)
	}
}

func TestUnclaimNonexistentConversation(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	// Unclaiming a conversation that doesn't exist should not error
	err := eng.UnclaimConversation("NOCALL")
	if err != nil {
		t.Errorf("unclaiming nonexistent conversation should not error, got: %v", err)
	}
}

func TestInboundWithoutMsgNoUniqueIDs(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	parser := aprs.NewParser()

	// Two messages from the same station with no message number
	frame1 := aprs.APRSFrame{
		Source:      aprs.Address{Call: "KG4YFA", SSID: 10},
		Destination: aprs.Address{Call: "N0CALL"},
		Payload:     ":N0CALL   :First message",
	}
	frame2 := aprs.APRSFrame{
		Source:      aprs.Address{Call: "KG4YFA", SSID: 10},
		Destination: aprs.Address{Call: "N0CALL"},
		Payload:     ":N0CALL   :Second message",
	}

	pkt1, _ := parser.Parse(frame1)
	pkt2, _ := parser.Parse(frame2)

	eng.HandlePacket(pkt1)
	eng.HandlePacket(pkt2)

	msgs := eng.Messages("KG4YFA-10")
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}

	// IDs must be unique — this is what causes the Svelte each_key_duplicate crash
	if msgs[0].ID == msgs[1].ID {
		t.Errorf("duplicate message IDs: %q", msgs[0].ID)
	}
}

func TestImport(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	now := time.Now()
	msgs := []Message{
		{ID: "a", From: "W3ADO", To: "N0CALL", Body: "Hello", MsgNo: "1", State: StateAcked, Inbound: true, Timestamp: now.Add(-2 * time.Minute)},
		{ID: "b", From: "N0CALL", To: "W3ADO", Body: "Hi back", MsgNo: "1", State: StateAcked, Inbound: false, Timestamp: now.Add(-time.Minute)},
		{ID: "c", From: "KJ4ERJ", To: "N0CALL", Body: "CQ", MsgNo: "5", State: StateAcked, Inbound: true, Timestamp: now},
	}

	eng.Import(msgs)

	// Check conversations are populated
	convos := eng.Conversations()
	if len(convos) != 2 {
		t.Fatalf("got %d conversations, want 2", len(convos))
	}

	// Check W3ADO conversation has 2 messages
	w3ado := eng.Messages("W3ADO")
	if len(w3ado) != 2 {
		t.Errorf("W3ADO messages = %d, want 2", len(w3ado))
	}

	// Check KJ4ERJ conversation has 1 message
	kj4erj := eng.Messages("KJ4ERJ")
	if len(kj4erj) != 1 {
		t.Errorf("KJ4ERJ messages = %d, want 1", len(kj4erj))
	}

	// Check dedup: re-receiving the same inbound message should be ignored
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "W3ADO"},
		Destination: aprs.Address{Call: "N0CALL"},
		Payload:     ":N0CALL   :Hello{1",
	}
	parser := aprs.NewParser()
	pkt, _ := parser.Parse(frame)
	eng.HandlePacket(pkt)

	// Still 2 messages for W3ADO (the duplicate was ignored)
	w3ado = eng.Messages("W3ADO")
	if len(w3ado) != 2 {
		t.Errorf("after dedup W3ADO messages = %d, want 2", len(w3ado))
	}
}
