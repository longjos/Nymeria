package message

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// mockSendFunc captures sent frames for verification.
type mockSendFunc struct {
	mu     sync.Mutex
	frames []aprs.APRSFrame
}

func (m *mockSendFunc) send(frame aprs.APRSFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frames = append(m.frames, frame)
	return nil
}

// getFrames returns a snapshot of all captured frames.
func (m *mockSendFunc) getFrames() []aprs.APRSFrame {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]aprs.APRSFrame, len(m.frames))
	copy(cp, m.frames)
	return cp
}

// frameCount returns the current number of captured frames.
func (m *mockSendFunc) frameCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.frames)
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
	frames := mock.getFrames()
	if len(frames) == 0 {
		t.Fatal("no frames sent")
	}

	frame := frames[0]
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
	initialFrameCount := mock.frameCount()

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
	finalCount := mock.frameCount()
	if finalCount > initialFrameCount {
		t.Errorf("got %d extra frames after ack, expected 0", finalCount-initialFrameCount)
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
	frames := mock.getFrames()
	if len(frames) == 0 {
		t.Fatal("no ack frame sent")
	}

	ackFrame := frames[0]
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
	for _, f := range mock.getFrames() {
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
	frameCount := mock.frameCount()
	if frameCount < 2 {
		t.Errorf("expected at least 2 frames (initial + retries), got %d", frameCount)
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

func TestBulletins_GroupedBySenderAndID(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	parser := aprs.NewParser()

	// Two different senders to the same bulletin ID
	frames := []aprs.APRSFrame{
		{Source: aprs.Address{Call: "W3ADO"}, Destination: aprs.Address{Call: "APRS"}, Payload: ":BLN3     :Weather alert for area"},
		{Source: aprs.Address{Call: "KJ4ERJ"}, Destination: aprs.Address{Call: "APRS"}, Payload: ":BLN3     :Another weather alert"},
	}
	for _, f := range frames {
		pkt, _ := parser.Parse(f)
		eng.HandlePacket(pkt)
	}

	bulletins := eng.Bulletins()
	if len(bulletins) != 2 {
		t.Fatalf("got %d bulletins, want 2", len(bulletins))
	}

	// Both should be BLN3
	for _, b := range bulletins {
		if b.BulletinID != "BLN3" {
			t.Errorf("bulletinId = %q, want BLN3", b.BulletinID)
		}
	}
}

func TestBulletins_LatestReplacesOlder(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	parser := aprs.NewParser()

	// Same sender, same BLN — the latest should replace the earlier one
	pkt1Frame := aprs.APRSFrame{
		Source: aprs.Address{Call: "W3ADO"}, Destination: aprs.Address{Call: "APRS"},
		Payload: ":BLN3     :Old weather alert",
	}
	pkt1, _ := parser.Parse(pkt1Frame)
	eng.HandlePacket(pkt1)

	// Small delay so timestamps differ
	time.Sleep(2 * time.Millisecond)

	pkt2Frame := aprs.APRSFrame{
		Source: aprs.Address{Call: "W3ADO"}, Destination: aprs.Address{Call: "APRS"},
		Payload: ":BLN3     :Updated weather alert",
	}
	pkt2, _ := parser.Parse(pkt2Frame)
	eng.HandlePacket(pkt2)

	bulletins := eng.Bulletins()
	if len(bulletins) != 1 {
		t.Fatalf("got %d bulletins, want 1 (latest replaces older)", len(bulletins))
	}
	if bulletins[0].Body != "Updated weather alert" {
		t.Errorf("body = %q, want Updated weather alert", bulletins[0].Body)
	}
}

func TestBulletins_Announcements(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	parser := aprs.NewParser()

	frame := aprs.APRSFrame{
		Source: aprs.Address{Call: "KG4YFA"}, Destination: aprs.Address{Call: "APRS"},
		Payload: ":ANN      :Club meeting Friday",
	}
	pkt, _ := parser.Parse(frame)
	eng.HandlePacket(pkt)

	bulletins := eng.Bulletins()
	if len(bulletins) != 1 {
		t.Fatalf("got %d bulletins, want 1", len(bulletins))
	}
	if !bulletins[0].IsAnnouncement {
		t.Error("expected announcement, got regular bulletin")
	}
	if bulletins[0].BulletinID != "ANN" {
		t.Errorf("bulletinId = %q, want ANN", bulletins[0].BulletinID)
	}
}

func TestBulletins_SortedByNumber(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	parser := aprs.NewParser()

	// Add bulletins in non-sorted order
	frames := []aprs.APRSFrame{
		{Source: aprs.Address{Call: "W3ADO"}, Destination: aprs.Address{Call: "APRS"}, Payload: ":BLN5     :Bulletin five"},
		{Source: aprs.Address{Call: "W3ADO"}, Destination: aprs.Address{Call: "APRS"}, Payload: ":BLN1     :Bulletin one"},
		{Source: aprs.Address{Call: "W3ADO"}, Destination: aprs.Address{Call: "APRS"}, Payload: ":BLN9     :Bulletin nine"},
	}
	for _, f := range frames {
		pkt, _ := parser.Parse(f)
		eng.HandlePacket(pkt)
	}

	bulletins := eng.Bulletins()
	if len(bulletins) != 3 {
		t.Fatalf("got %d bulletins, want 3", len(bulletins))
	}

	expected := []string{"BLN1", "BLN5", "BLN9"}
	for i, b := range bulletins {
		if b.BulletinID != expected[i] {
			t.Errorf("bulletins[%d].bulletinId = %q, want %q", i, b.BulletinID, expected[i])
		}
	}
}

func TestBulletins_ExcludedFromConversations(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	parser := aprs.NewParser()

	// One DM and one bulletin
	dmFrame := aprs.APRSFrame{
		Source: aprs.Address{Call: "W3ADO"}, Destination: aprs.Address{Call: "N0CALL"},
		Payload: ":N0CALL   :Hello there",
	}
	blnFrame := aprs.APRSFrame{
		Source: aprs.Address{Call: "KJ4ERJ"}, Destination: aprs.Address{Call: "APRS"},
		Payload: ":BLN3     :Weather alert",
	}

	pkt1, _ := parser.Parse(dmFrame)
	pkt2, _ := parser.Parse(blnFrame)
	eng.HandlePacket(pkt1)
	eng.HandlePacket(pkt2)

	convos := eng.Conversations()
	if len(convos) != 1 {
		t.Fatalf("got %d conversations, want 1 (bulletins excluded)", len(convos))
	}
	if convos[0].Callsign != "W3ADO" {
		t.Errorf("conversation callsign = %q, want W3ADO", convos[0].Callsign)
	}

	// Bulletins should still be available
	bulletins := eng.Bulletins()
	if len(bulletins) != 1 {
		t.Fatalf("got %d bulletins, want 1", len(bulletins))
	}
}

func TestImport_BulletinKeying(t *testing.T) {
	eng, _ := newTestEngine()
	defer eng.Close()

	now := time.Now()
	msgs := []Message{
		{ID: "b1", From: "W3ADO", To: "BLN3", Body: "Weather alert", MsgNo: "1", State: StateAcked, Inbound: true, Timestamp: now},
		{ID: "b2", From: "KJ4ERJ", To: "BLN3", Body: "Another alert", MsgNo: "2", State: StateAcked, Inbound: true, Timestamp: now},
		{ID: "d1", From: "W3ADO", To: "N0CALL", Body: "Hello", MsgNo: "3", State: StateAcked, Inbound: true, Timestamp: now},
	}

	eng.Import(msgs)

	// Bulletins should be keyed by "BLN3" (the To field), not by sender
	blnMsgs := eng.Messages("BLN3")
	if len(blnMsgs) != 2 {
		t.Fatalf("BLN3 messages = %d, want 2", len(blnMsgs))
	}

	// DM should still be keyed by sender
	dmMsgs := eng.Messages("W3ADO")
	if len(dmMsgs) != 1 {
		t.Fatalf("W3ADO messages = %d, want 1", len(dmMsgs))
	}

	// Bulletins should be available via Bulletins()
	bulletins := eng.Bulletins()
	if len(bulletins) != 2 {
		t.Fatalf("got %d bulletins, want 2", len(bulletins))
	}

	// Conversations should NOT include bulletins
	convos := eng.Conversations()
	for _, c := range convos {
		if strings.HasPrefix(c.Callsign, "BLN") || strings.HasPrefix(c.Callsign, "ANN") {
			t.Errorf("bulletin %q should not appear in Conversations()", c.Callsign)
		}
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

func TestUpdateCallsign(t *testing.T) {
	eng, mock := newTestEngine()
	defer eng.Close()

	// Send with original callsign
	msg1, err := eng.Send("W3ADO", "before change")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msg1.From != "N0CALL" {
		t.Errorf("msg1.From = %q, want N0CALL", msg1.From)
	}
	frames := mock.getFrames()
	if frames[0].Source.Call != "N0CALL" {
		t.Errorf("frame source = %q, want N0CALL", frames[0].Source.Call)
	}

	// Update callsign
	eng.UpdateCallsign("W1AW")

	// Send with new callsign
	msg2, err := eng.Send("KJ4ERJ", "after change")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msg2.From != "W1AW" {
		t.Errorf("msg2.From = %q, want W1AW", msg2.From)
	}

	// Verify the frame used new callsign
	frames = mock.getFrames()
	lastFrame := frames[len(frames)-1]
	if lastFrame.Source.Call != "W1AW" {
		t.Errorf("frame source after update = %q, want W1AW", lastFrame.Source.Call)
	}
}
