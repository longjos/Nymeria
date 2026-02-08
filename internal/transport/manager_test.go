package transport

import (
	"context"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// mockTransport is a fake transport for testing.
type mockTransport struct {
	frames chan aprs.APRSFrame
	sent   []aprs.APRSFrame
	status TransportStatus
	typ    string
}

func newMockTransport(typ string) *mockTransport {
	return &mockTransport{
		frames: make(chan aprs.APRSFrame, 256),
		status: TransportStatus{Type: typ, Connected: true},
		typ:    typ,
	}
}

func (m *mockTransport) Connect(_ context.Context) error { return nil }
func (m *mockTransport) Close() error                    { close(m.frames); return nil }
func (m *mockTransport) Send(f aprs.APRSFrame) error     { m.sent = append(m.sent, f); return nil }
func (m *mockTransport) Receive() <-chan aprs.APRSFrame   { return m.frames }
func (m *mockTransport) Status() TransportStatus          { return m.status }
func (m *mockTransport) Type() string                     { return m.typ }

func TestManagerTaggedFrames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager()
	rf := newMockTransport("kisstcp")
	is := newMockTransport("aprsis")

	mgr.Add("rf-0", rf)
	mgr.Add("is-0", is)
	mgr.ConnectAll(ctx)

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "!4903.50N/07201.75W-",
	}

	// Send frame from RF transport
	rf.frames <- frame

	// Read from TaggedFrames
	select {
	case tf := <-mgr.TaggedFrames():
		if tf.Source != "rf-0" {
			t.Errorf("Source = %q, want %q", tf.Source, "rf-0")
		}
		if tf.Frame.Payload != frame.Payload {
			t.Errorf("Payload mismatch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tagged frame")
	}
}

func TestManagerDedup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager()
	mgr.DedupWindow = 30 * time.Second

	rf := newMockTransport("kisstcp")
	is := newMockTransport("aprsis")

	mgr.Add("rf-0", rf)
	mgr.Add("is-0", is)
	mgr.ConnectAll(ctx)

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "!4903.50N/07201.75W-",
	}

	// Send same frame from both transports
	rf.frames <- frame

	// Small delay to let it be processed
	time.Sleep(50 * time.Millisecond)
	is.frames <- frame

	// Should only receive one frame (the first one)
	select {
	case tf := <-mgr.TaggedFrames():
		if tf.Source != "rf-0" {
			t.Errorf("first frame Source = %q, want rf-0", tf.Source)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first frame")
	}

	// Second frame should be deduped — shouldn't arrive
	select {
	case tf := <-mgr.TaggedFrames():
		t.Errorf("received duplicate frame from %q, should have been deduped", tf.Source)
	case <-time.After(200 * time.Millisecond):
		// Good — no duplicate
	}
}

func TestManagerDedupDifferentPackets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager()
	mgr.DedupWindow = 30 * time.Second

	rf := newMockTransport("kisstcp")
	mgr.Add("rf-0", rf)
	mgr.ConnectAll(ctx)

	frame1 := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "!4903.50N/07201.75W-",
	}
	frame2 := aprs.APRSFrame{
		Source:      aprs.Address{Call: "W3ADO"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     ">status text",
	}

	rf.frames <- frame1
	rf.frames <- frame2

	// Both should come through (different packets)
	for i := 0; i < 2; i++ {
		select {
		case <-mgr.TaggedFrames():
			// Good
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for frame %d", i+1)
		}
	}
}

func TestManagerDedupExpiry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager()
	mgr.DedupWindow = 100 * time.Millisecond // Short window for testing

	rf := newMockTransport("kisstcp")
	mgr.Add("rf-0", rf)
	mgr.ConnectAll(ctx)

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "!4903.50N/07201.75W-",
	}

	rf.frames <- frame

	select {
	case <-mgr.TaggedFrames():
		// Good, first frame received
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// Wait for dedup window to expire
	time.Sleep(150 * time.Millisecond)

	// Same frame should now be accepted again
	rf.frames <- frame

	select {
	case <-mgr.TaggedFrames():
		// Good — dedup expired, frame accepted
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame after dedup expiry")
	}
}

func TestManagerSendRouting(t *testing.T) {
	mgr := NewManager()

	rf := newMockTransport("kisstcp")
	is := newMockTransport("aprsis")

	mgr.Add("rf-0", rf)
	mgr.Add("is-0", is)

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "!4903.50N/07201.75W-",
	}

	// Send to all
	err := mgr.Send(frame)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(rf.sent) != 1 {
		t.Errorf("RF sent %d frames, want 1", len(rf.sent))
	}
	if len(is.sent) != 1 {
		t.Errorf("IS sent %d frames, want 1", len(is.sent))
	}

	// Send to specific transport
	err = mgr.SendVia("rf-0", frame)
	if err != nil {
		t.Fatalf("SendVia: %v", err)
	}
	if len(rf.sent) != 2 {
		t.Errorf("RF sent %d frames after SendVia, want 2", len(rf.sent))
	}
	if len(is.sent) != 1 {
		t.Errorf("IS sent %d frames after SendVia, want 1 (unchanged)", len(is.sent))
	}
}

func TestManagerSendViaUnknown(t *testing.T) {
	mgr := NewManager()

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "test",
	}

	err := mgr.SendVia("nonexistent", frame)
	if err == nil {
		t.Error("expected error for unknown transport, got nil")
	}
}

func TestManagerStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager()
	rf := newMockTransport("kisstcp")
	mgr.Add("rf-0", rf)
	mgr.ConnectAll(ctx)

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "!4903.50N/07201.75W-",
	}

	// Receive a frame
	rf.frames <- frame
	select {
	case <-mgr.TaggedFrames():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// Send a frame
	mgr.Send(frame)

	// Check stats
	statuses := mgr.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	if statuses[0].PacketsRx != 1 {
		t.Errorf("PacketsRx = %d, want 1", statuses[0].PacketsRx)
	}
	if statuses[0].PacketsTx != 1 {
		t.Errorf("PacketsTx = %d, want 1", statuses[0].PacketsTx)
	}
}
