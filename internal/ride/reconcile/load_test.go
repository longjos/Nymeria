package reconcile

import (
	"testing"

	"github.com/narvel/nymeria/internal/store"
)

func TestLoadHydratesEveryTable(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	sagID := ts.newSAGCheckIn(t, netID, "K6ABC", "SAG 1")

	// Save one row directly to the store for each table this package owns,
	// bypassing the in-memory manager (as if a previous process wrote it).
	if err := ts.Store.SaveSAGShiftSummary(store.SAGShiftSummary{
		ID: "shift-1", NetID: netID, CheckInID: sagID, Status: ShiftDraft,
	}); err != nil {
		t.Fatalf("SaveSAGShiftSummary: %v", err)
	}
	if err := ts.Store.SaveHandoffItem(store.HandoffItem{
		ID: "ho-1", NetID: netID, Kind: HandoffFYI, Summary: "fyi", Status: HandoffOpen,
	}); err != nil {
		t.Fatalf("SaveHandoffItem: %v", err)
	}
	if err := ts.Store.SaveShiftHandoff(store.ShiftHandoff{
		ID: "sh-1", NetID: netID, FromCallsign: "K6ABC", ToCallsign: "W6XYZ",
		OpenItemIDs: nil, // must come back as [] after Load
	}); err != nil {
		t.Fatalf("SaveShiftHandoff: %v", err)
	}

	// A fresh Manager over the same store, as at process restart.
	fresh := NewManager(ts.Store, ts.Net, ts.Course, ts.SAG, ts.Ann)
	if err := fresh.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	shifts := fresh.ShiftSummaries(netID)
	if len(shifts) != 1 || shifts[0].ID != "shift-1" {
		t.Errorf("ShiftSummaries after Load = %+v, want [shift-1]", shifts)
	}

	items := fresh.HandoffItems(netID, "all")
	if len(items) != 1 || items[0].ID != "ho-1" {
		t.Errorf("HandoffItems after Load = %+v, want [ho-1]", items)
	}

	handoffs := fresh.Handoffs(netID)
	if len(handoffs) != 1 || handoffs[0].ID != "sh-1" {
		t.Errorf("Handoffs after Load = %+v, want [sh-1]", handoffs)
	}
	if handoffs[0].OpenItemIDs == nil {
		t.Error("OpenItemIDs is nil after Load, want non-nil empty slice")
	}
}

func TestLoadWithNoNetControlManagerIsNoop(t *testing.T) {
	ts := newTestStack(t)
	m := NewManager(ts.Store, nil, nil, nil, nil)
	if err := m.Load(); err != nil {
		t.Fatalf("Load with nil netMgr: %v", err)
	}
}
