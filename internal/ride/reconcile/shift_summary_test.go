package reconcile

import (
	"errors"
	"testing"

	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/store"
)

func intPtr(i int) *int { return &i }

func diffAbs(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func TestEffective(t *testing.T) {
	derived := SAGShiftDerived{Transports: 3, Assists: 1, IncidentsAttended: 2}

	t.Run("all nil falls back to derived", func(t *testing.T) {
		got := Effective(store.SAGShiftCounts{}, derived)
		if got != derived {
			t.Errorf("Effective = %+v, want %+v", got, derived)
		}
	})

	t.Run("entered overrides derived", func(t *testing.T) {
		got := Effective(store.SAGShiftCounts{Transports: intPtr(7)}, derived)
		if got.Transports != 7 {
			t.Errorf("Transports = %d, want 7", got.Transports)
		}
		if got.Assists != derived.Assists {
			t.Errorf("Assists = %d, want %d (falls back)", got.Assists, derived.Assists)
		}
	})

	t.Run("explicit zero beats derived", func(t *testing.T) {
		got := Effective(store.SAGShiftCounts{Transports: intPtr(0)}, derived)
		if got.Transports != 0 {
			t.Errorf("Transports = %d, want 0 (explicit zero must win)", got.Transports)
		}
	})
}

func TestCreateShiftSummary(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	sagID := ts.newSAGCheckIn(t, netID, "K6ABC", "SAG 1")

	view, err := ts.Reconcile.CreateShiftSummary(netID, sagID, "K6ABC")
	if err != nil {
		t.Fatalf("CreateShiftSummary: %v", err)
	}
	if view.Callsign != "K6ABC" || view.TacticalCall != "SAG 1" {
		t.Errorf("Callsign/TacticalCall = %q/%q, want K6ABC/SAG 1", view.Callsign, view.TacticalCall)
	}
	if view.Status != ShiftDraft {
		t.Errorf("Status = %q, want draft", view.Status)
	}
	if view.ShiftStart == nil {
		t.Error("ShiftStart is nil, want the check-in's CheckedInAt")
	}

	// A marshal category check-in is not a sag unit.
	marshalCi, err := ts.Net.CheckIn(netID, "N6MAR", netcontrol.TrafficNone, netcontrol.CatMarshal)
	if err != nil {
		t.Fatalf("CheckIn marshal: %v", err)
	}
	if _, err := ts.Reconcile.CreateShiftSummary(netID, marshalCi.ID, "K6ABC"); err == nil {
		t.Error("CreateShiftSummary(marshal check-in) = nil error, want error")
	}

	// A second draft for the same unit while one is already open: rejected.
	if _, err := ts.Reconcile.CreateShiftSummary(netID, sagID, "K6ABC"); err == nil {
		t.Error("CreateShiftSummary (second draft) = nil error, want error")
	}

	// Once filed, a new draft for the same unit is allowed.
	start := 100.0
	end := 150.0
	view.OdometerStart = &start
	view.OdometerEnd = &end
	if _, err := ts.Reconcile.UpdateShiftSummary(netID, view.SAGShiftSummary); err != nil {
		t.Fatalf("UpdateShiftSummary: %v", err)
	}
	if _, err := ts.Reconcile.FileShiftSummary(netID, view.ID, "K6ABC"); err != nil {
		t.Fatalf("FileShiftSummary: %v", err)
	}
	if _, err := ts.Reconcile.CreateShiftSummary(netID, sagID, "K6ABC"); err != nil {
		t.Errorf("CreateShiftSummary after filing previous: %v", err)
	}
}

func TestFileShiftSummary(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	sagID := ts.newSAGCheckIn(t, netID, "K6ABC", "SAG 1")

	view, err := ts.Reconcile.CreateShiftSummary(netID, sagID, "K6ABC")
	if err != nil {
		t.Fatalf("CreateShiftSummary: %v", err)
	}

	if _, err := ts.Reconcile.FileShiftSummary(netID, view.ID, "K6ABC"); err == nil {
		t.Error("FileShiftSummary with no odometer readings = nil error, want error")
	}

	start := 200.0
	view.OdometerStart = &start
	if _, err := ts.Reconcile.UpdateShiftSummary(netID, view.SAGShiftSummary); err != nil {
		t.Fatalf("UpdateShiftSummary: %v", err)
	}
	if _, err := ts.Reconcile.FileShiftSummary(netID, view.ID, "K6ABC"); err == nil {
		t.Error("FileShiftSummary with only start set = nil error, want error")
	}

	bad := 100.0 // before start
	view.OdometerEnd = &bad
	if _, err := ts.Reconcile.UpdateShiftSummary(netID, view.SAGShiftSummary); err != nil {
		t.Fatalf("UpdateShiftSummary: %v", err)
	}
	if _, err := ts.Reconcile.FileShiftSummary(netID, view.ID, "K6ABC"); err == nil {
		t.Error("FileShiftSummary with end < start = nil error, want error")
	}

	good := 284.2
	view.OdometerEnd = &good
	if _, err := ts.Reconcile.UpdateShiftSummary(netID, view.SAGShiftSummary); err != nil {
		t.Fatalf("UpdateShiftSummary: %v", err)
	}
	filed, err := ts.Reconcile.FileShiftSummary(netID, view.ID, "K6ABC")
	if err != nil {
		t.Fatalf("FileShiftSummary: %v", err)
	}
	if filed.Status != ShiftFiled {
		t.Errorf("Status = %q, want filed", filed.Status)
	}
	if filed.ShiftEnd == nil {
		t.Error("ShiftEnd is nil, want set to now")
	}
	if filed.NetMiles == nil || diffAbs(*filed.NetMiles, 84.2) > 0.001 {
		t.Errorf("NetMiles = %v, want ~84.2", filed.NetMiles)
	}

	events, _ := ts.Net.GetEvents(netID)
	found := false
	for _, e := range events {
		if e.Type == TimelineShiftFiled {
			found = true
		}
	}
	if !found {
		t.Error("no ride_shift_summary_filed timeline entry")
	}
}

func TestUpdateFiledSummaryRefused(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	sagID := ts.newSAGCheckIn(t, netID, "K6ABC", "SAG 1")
	view, err := ts.Reconcile.CreateShiftSummary(netID, sagID, "K6ABC")
	if err != nil {
		t.Fatalf("CreateShiftSummary: %v", err)
	}
	start, end := 0.0, 10.0
	view.OdometerStart, view.OdometerEnd = &start, &end
	if _, err := ts.Reconcile.UpdateShiftSummary(netID, view.SAGShiftSummary); err != nil {
		t.Fatalf("UpdateShiftSummary: %v", err)
	}
	filed, err := ts.Reconcile.FileShiftSummary(netID, view.ID, "K6ABC")
	if err != nil {
		t.Fatalf("FileShiftSummary: %v", err)
	}

	if _, err := ts.Reconcile.UpdateShiftSummary(netID, filed.SAGShiftSummary); !errors.Is(err, ErrIllegal) {
		t.Errorf("UpdateShiftSummary(filed) error = %v, want ErrIllegal", err)
	}
	if _, err := ts.Reconcile.FileShiftSummary(netID, filed.ID, "K6ABC"); !errors.Is(err, ErrIllegal) {
		t.Errorf("FileShiftSummary(already filed) error = %v, want ErrIllegal", err)
	}
}

func TestShiftSummaryDerivedTally(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	sagID := ts.newSAGCheckIn(t, netID, "K6ABC", "SAG 1")

	req, err := ts.SAG.CreateRequest(netID, ride.CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: ride.LocCourse},
		Dropoff: store.SAGLocation{Kind: ride.LocFinish},
		Reason:  "flat",
		Slots:   []ride.SlotInput{{Bib: "412"}},
	}, "K6ABC")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if _, err := ts.SAG.Dispatch(netID, req.ID, ride.DispatchInput{VehicleCheckInID: sagID}, "K6ABC"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	req, ok := ts.SAG.GetRequest(netID, req.ID)
	if !ok {
		t.Fatal("GetRequest: not found")
	}
	legID := req.Legs[0].ID
	if _, err := ts.SAG.LoadSlots(netID, req.ID, legID, []string{req.Slots[0].ID}, "K6ABC"); err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	if _, err := ts.SAG.DeliverSlots(netID, req.ID, legID, []string{req.Slots[0].ID}, nil, "K6ABC"); err != nil {
		t.Fatalf("DeliverSlots: %v", err)
	}

	view, err := ts.Reconcile.CreateShiftSummary(netID, sagID, "K6ABC")
	if err != nil {
		t.Fatalf("CreateShiftSummary: %v", err)
	}
	if view.Derived.Transports != 1 {
		t.Errorf("Derived.Transports = %d, want 1", view.Derived.Transports)
	}
	if view.Derived.IncidentsAttended != 1 {
		t.Errorf("Derived.IncidentsAttended = %d, want 1", view.Derived.IncidentsAttended)
	}
	if view.Effective.Transports != 1 {
		t.Errorf("Effective.Transports = %d, want 1 (no entered override)", view.Effective.Transports)
	}
}

func TestShiftSummariesNeverNil(t *testing.T) {
	ts := newTestStack(t)
	if list := ts.Reconcile.ShiftSummaries("no-such-net"); list == nil {
		t.Error("ShiftSummaries is nil, want empty non-nil slice")
	}
}
