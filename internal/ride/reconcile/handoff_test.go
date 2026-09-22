package reconcile

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/narvel/nymeria/internal/store"
)

func TestAddHandoffItemDefaults(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	h, err := ts.Reconcile.AddHandoffItem(store.HandoffItem{
		NetID: netID, Kind: HandoffAwaitingReply, Summary: "Asked RS3 for water status", SentTo: "RS3",
	})
	if err != nil {
		t.Fatalf("AddHandoffItem: %v", err)
	}
	if h.ReplyTo != "NCS" {
		t.Errorf("ReplyTo = %q, want NCS (default)", h.ReplyTo)
	}
	if h.Status != HandoffOpen {
		t.Errorf("Status = %q, want open", h.Status)
	}
	if h.HandoverCount != 0 {
		t.Errorf("HandoverCount = %d, want 0", h.HandoverCount)
	}

	if _, err := ts.Reconcile.AddHandoffItem(store.HandoffItem{NetID: netID, Summary: "x"}); err == nil {
		t.Error("AddHandoffItem with no Kind = nil error, want error")
	}
	if _, err := ts.Reconcile.AddHandoffItem(store.HandoffItem{NetID: netID, Kind: HandoffFYI}); err == nil {
		t.Error("AddHandoffItem with no Summary = nil error, want error")
	}
}

func TestResolveAndCancel(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	h, err := ts.Reconcile.AddHandoffItem(store.HandoffItem{NetID: netID, Kind: HandoffFYI, Summary: "fyi"})
	if err != nil {
		t.Fatalf("AddHandoffItem: %v", err)
	}
	resolved, err := ts.Reconcile.ResolveHandoffItem(netID, h.ID, "W6XYZ", "confirmed", false)
	if err != nil {
		t.Fatalf("ResolveHandoffItem: %v", err)
	}
	if resolved.Status != HandoffResolved || resolved.ResolvedBy != "W6XYZ" || resolved.ResolvedAt == nil {
		t.Errorf("resolved = %+v, want status=resolved resolvedBy=W6XYZ resolvedAt set", resolved)
	}

	if _, err := ts.Reconcile.ResolveHandoffItem(netID, h.ID, "W6XYZ", "again", false); !errors.Is(err, ErrIllegal) {
		t.Errorf("resolve again error = %v, want ErrIllegal", err)
	}

	h2, err := ts.Reconcile.AddHandoffItem(store.HandoffItem{NetID: netID, Kind: HandoffFYI, Summary: "fyi2"})
	if err != nil {
		t.Fatalf("AddHandoffItem: %v", err)
	}
	cancelled, err := ts.Reconcile.ResolveHandoffItem(netID, h2.ID, "W6XYZ", "moot", true)
	if err != nil {
		t.Fatalf("ResolveHandoffItem (cancel): %v", err)
	}
	if cancelled.Status != HandoffCancelled {
		t.Errorf("Status = %q, want cancelled", cancelled.Status)
	}
}

func TestRecordNCSTransferCarriesOpenItems(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	open1, err := ts.Reconcile.AddHandoffItem(store.HandoffItem{NetID: netID, Kind: HandoffFYI, Summary: "one"})
	if err != nil {
		t.Fatalf("AddHandoffItem: %v", err)
	}
	open2, err := ts.Reconcile.AddHandoffItem(store.HandoffItem{NetID: netID, Kind: HandoffFYI, Summary: "two"})
	if err != nil {
		t.Fatalf("AddHandoffItem: %v", err)
	}
	resolved, err := ts.Reconcile.AddHandoffItem(store.HandoffItem{NetID: netID, Kind: HandoffFYI, Summary: "three"})
	if err != nil {
		t.Fatalf("AddHandoffItem: %v", err)
	}
	if _, err := ts.Reconcile.ResolveHandoffItem(netID, resolved.ID, "K6ABC", "done", false); err != nil {
		t.Fatalf("ResolveHandoffItem: %v", err)
	}

	drain(ts.Reconcile.Events())
	ts.Reconcile.RecordNCSTransfer(netID, "K6ABC", "W6XYZ")

	for _, id := range []string{open1.ID, open2.ID} {
		found := false
		for _, h := range ts.Reconcile.HandoffItems(netID, "all") {
			if h.ID == id {
				found = true
				if h.HandoverCount != 1 {
					t.Errorf("item %s HandoverCount = %d, want 1", id, h.HandoverCount)
				}
			}
		}
		if !found {
			t.Fatalf("open item %s not found after transfer", id)
		}
	}
	for _, h := range ts.Reconcile.HandoffItems(netID, "all") {
		if h.ID == resolved.ID && h.HandoverCount != 0 {
			t.Errorf("resolved item HandoverCount = %d, want 0 (unchanged)", h.HandoverCount)
		}
	}

	handoffs := ts.Reconcile.Handoffs(netID)
	if len(handoffs) != 1 {
		t.Fatalf("len(Handoffs) = %d, want 1", len(handoffs))
	}
	sh := handoffs[0]
	if len(sh.OpenItemIDs) != 2 {
		t.Errorf("OpenItemIDs = %v, want 2 entries", sh.OpenItemIDs)
	}
	var briefing ShiftBriefing
	if err := json.Unmarshal([]byte(sh.Briefing), &briefing); err != nil {
		t.Fatalf("unmarshal frozen briefing: %v", err)
	}
	if briefing.NCSCallsign != "W6XYZ" {
		t.Errorf("frozen briefing NCSCallsign = %q, want W6XYZ", briefing.NCSCallsign)
	}

	types := drainTypes(ts.Reconcile.Events())
	foundShiftHandoff := false
	for _, ty := range types {
		if ty == EventShiftHandoff {
			foundShiftHandoff = true
		}
	}
	if !foundShiftHandoff {
		t.Errorf("Events() = %v, want to contain %s", types, EventShiftHandoff)
	}

	events, _ := ts.Net.GetEvents(netID)
	foundTimeline := false
	for _, e := range events {
		if e.Type == TimelineHandoff {
			foundTimeline = true
		}
	}
	if !foundTimeline {
		t.Error("no ride_ncs_handoff timeline entry")
	}

	// Second transfer: counts increment again.
	ts.Reconcile.RecordNCSTransfer(netID, "W6XYZ", "N6DEF")
	for _, h := range ts.Reconcile.HandoffItems(netID, "all") {
		if h.ID == open1.ID && h.HandoverCount != 2 {
			t.Errorf("after second transfer, HandoverCount = %d, want 2", h.HandoverCount)
		}
	}
}

func TestAcknowledgeHandoff(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	if _, err := ts.Reconcile.AcknowledgeHandoff(netID, "W6XYZ"); err == nil {
		t.Error("AcknowledgeHandoff with no transfer yet = nil error, want error")
	}

	ts.Reconcile.RecordNCSTransfer(netID, "K6ABC", "W6XYZ")
	ack, err := ts.Reconcile.AcknowledgeHandoff(netID, "W6XYZ")
	if err != nil {
		t.Fatalf("AcknowledgeHandoff: %v", err)
	}
	if ack.AcknowledgedAt == nil || ack.AcknowledgedBy != "W6XYZ" {
		t.Errorf("ack = %+v, want AcknowledgedAt set, AcknowledgedBy=W6XYZ", ack)
	}

	if _, err := ts.Reconcile.AcknowledgeHandoff(netID, "W6XYZ"); err == nil {
		t.Error("AcknowledgeHandoff again = nil error, want error (already acknowledged)")
	}
}

func TestHandoffsNeverNil(t *testing.T) {
	ts := newTestStack(t)
	if list := ts.Reconcile.HandoffItems("no-such-net", "open"); list == nil {
		t.Error("HandoffItems is nil, want empty non-nil slice")
	}
	if list := ts.Reconcile.Handoffs("no-such-net"); list == nil {
		t.Error("Handoffs is nil, want empty non-nil slice")
	}
}
