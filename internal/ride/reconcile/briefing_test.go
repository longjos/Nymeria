package reconcile

import (
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/message"
)

// fakeMessageEngine implements message.Engine with only Messages()
// returning canned data; every other method is an unused no-op/stub.
type fakeMessageEngine struct {
	msgs []message.Message
}

func (f *fakeMessageEngine) Send(to, body string) (*message.Message, error) { return nil, nil }
func (f *fakeMessageEngine) SendWithPath(to, body, path string) (*message.Message, error) {
	return nil, nil
}
func (f *fakeMessageEngine) HandlePacket(pkt *aprs.Packet)              {}
func (f *fakeMessageEngine) Messages(callsign string) []message.Message { return f.msgs }
func (f *fakeMessageEngine) Conversations() []message.Conversation      { return nil }
func (f *fakeMessageEngine) Bulletins() []message.Bulletin              { return nil }
func (f *fakeMessageEngine) Events() <-chan message.Event               { return nil }
func (f *fakeMessageEngine) Import(msgs []message.Message)              {}
func (f *fakeMessageEngine) MarkRead(callsign string, readAt time.Time) (*message.Conversation, error) {
	return nil, nil
}
func (f *fakeMessageEngine) ImportReadState(reads map[string]time.Time)            {}
func (f *fakeMessageEngine) ClaimConversation(callsign, userID, name string) error { return nil }
func (f *fakeMessageEngine) UnclaimConversation(callsign string) error             { return nil }
func (f *fakeMessageEngine) UnclaimByUser(userID string)                           {}
func (f *fakeMessageEngine) Close()                                                {}

// fakeBriefingSource returns fixed sections for BriefingSections.
type fakeBriefingSource struct {
	sections []BriefingSection
}

func (f *fakeBriefingSource) BriefingSections(netID string) []BriefingSection { return f.sections }

func TestBriefingAwaitingReplies(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	n, _ := ts.Net.GetNet(netID)
	opened := *n.OpenedAt

	sentAt := opened.Add(20 * time.Minute)
	pendingAt := opened.Add(10 * time.Minute)
	ackedAt := opened.Add(30 * time.Minute)
	beforeOpen := opened.Add(-5 * time.Minute)

	eng := &fakeMessageEngine{msgs: []message.Message{
		{ID: "m1", To: "RS3", Body: "water status?", State: message.StateSent, Timestamp: sentAt},
		{ID: "m2", To: "RS3", Body: "old ack", State: message.StateAcked, Timestamp: ackedAt},
		{ID: "m3", From: "RS3", Body: "inbound", Inbound: true, Timestamp: opened.Add(15 * time.Minute)},
		{ID: "m4", To: "SAG 2", Body: "status?", State: message.StatePending, Timestamp: pendingAt},
		{ID: "m5", To: "RS1", Body: "before net opened", State: message.StateSent, Timestamp: beforeOpen},
	}}
	ts.Reconcile.SetMessageEngine(eng)

	got := ts.Reconcile.awaitingReplies(netID)
	if len(got) != 2 {
		t.Fatalf("len(awaitingReplies) = %d, want 2: %+v", len(got), got)
	}
	if got[0].MessageID != "m4" || got[1].MessageID != "m1" {
		t.Errorf("order = [%s %s], want [m4 m1] (oldest first)", got[0].MessageID, got[1].MessageID)
	}
	if got[0].State != "pending" || got[1].State != "sent" {
		t.Errorf("states = [%s %s], want [pending sent]", got[0].State, got[1].State)
	}
}

func TestBriefingAwaitingRepliesNilEngine(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	got := ts.Reconcile.awaitingReplies(netID)
	if got == nil {
		t.Error("awaitingReplies is nil, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 with no engine wired", len(got))
	}
}

func TestBriefingSections(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	src1 := &fakeBriefingSource{sections: []BriefingSection{
		{Key: "sag_open", Title: "SAG Open", Lines: []BriefingLine{{Label: "SAG 7", Value: "en route"}}},
	}}
	src2 := &fakeBriefingSource{sections: []BriefingSection{
		{Key: "next_shutoff", Title: "Next Shutoff", Lines: nil}, // nil Lines must normalize to []
	}}
	ts.Reconcile.AddBriefingSource(src1)
	ts.Reconcile.AddBriefingSource(src2)

	sections := ts.Reconcile.briefingSections(netID)
	if len(sections) != 2 {
		t.Fatalf("len(sections) = %d, want 2", len(sections))
	}
	if sections[0].Key != "sag_open" || sections[1].Key != "next_shutoff" {
		t.Errorf("sections not in registration order: %+v", sections)
	}
	if sections[1].Lines == nil {
		t.Error("sections[1].Lines is nil, want normalized to empty slice")
	}
}

func TestBriefingShiftDue(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	n, _ := ts.Net.GetNet(netID)

	b := ts.Reconcile.Briefing(netID)
	if b.NCSSince == nil || !b.NCSSince.Equal(*n.OpenedAt) {
		t.Errorf("NCSSince = %v, want net OpenedAt %v (no handoff yet)", b.NCSSince, n.OpenedAt)
	}
	wantDue := n.OpenedAt.Add(120 * time.Minute)
	if b.ShiftDueAt == nil || !b.ShiftDueAt.Equal(wantDue) {
		t.Errorf("ShiftDueAt = %v, want %v (default 120m)", b.ShiftDueAt, wantDue)
	}

	ts.Reconcile.SetPolicyProvider(func(string) Policy {
		p := DefaultPolicy()
		p.NCSShiftMinutes = 90
		return p
	})
	ts.Reconcile.RecordNCSTransfer(netID, "K6ABC", "W6XYZ")
	b = ts.Reconcile.Briefing(netID)
	handoffs := ts.Reconcile.Handoffs(netID)
	if len(handoffs) != 1 {
		t.Fatalf("len(Handoffs) = %d, want 1", len(handoffs))
	}
	if b.NCSSince == nil || !b.NCSSince.Equal(handoffs[0].At) {
		t.Errorf("NCSSince = %v, want the handoff's At %v", b.NCSSince, handoffs[0].At)
	}
	wantDue = handoffs[0].At.Add(90 * time.Minute)
	if b.ShiftDueAt == nil || !b.ShiftDueAt.Equal(wantDue) {
		t.Errorf("ShiftDueAt = %v, want %v (90m policy)", b.ShiftDueAt, wantDue)
	}
}

func TestBriefingRecentEventsWindow(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	now := time.Now().UTC()
	ts.Reconcile.SetClock(fixedClock(now))

	if err := ts.Net.AddTimelineEvent(netID, "old_event", "K6ABC", "45 minutes ago"); err != nil {
		t.Fatalf("AddTimelineEvent: %v", err)
	}
	events, _ := ts.Net.GetEvents(netID)
	// AddTimelineEvent always stamps "now" (netcontrol has no injectable
	// clock), so backdate the row directly in the store — GetEvents reads
	// straight through to LoadNetEvents with no separate cache to go stale.
	old := events[len(events)-1]
	old.CreatedAt = now.Add(-45 * time.Minute)
	if err := ts.Store.SaveNetEvent(old); err != nil {
		t.Fatalf("SaveNetEvent: %v", err)
	}
	if err := ts.Net.AddTimelineEvent(netID, "recent_event", "K6ABC", "10 minutes ago"); err != nil {
		t.Fatalf("AddTimelineEvent: %v", err)
	}

	b := ts.Reconcile.Briefing(netID)
	if b.RecentEvents == nil {
		t.Fatal("RecentEvents is nil, want non-nil")
	}
	foundRecent, foundOld := false, false
	for _, e := range b.RecentEvents {
		if e.Type == "recent_event" {
			foundRecent = true
		}
		if e.Type == "old_event" {
			foundOld = true
		}
	}
	if !foundRecent {
		t.Error("recent_event missing from RecentEvents")
	}
	if foundOld {
		t.Error("old_event (45m ago) present in RecentEvents, want excluded (30m window)")
	}
}
