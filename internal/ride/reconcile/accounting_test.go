package reconcile

import (
	"testing"

	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/store"
)

func TestAccountingNeverNil(t *testing.T) {
	ts := newTestStack(t)
	acc := ts.Reconcile.Accounting("no-such-net")
	if acc.OpenExceptions == nil {
		t.Error("OpenExceptions is nil, want non-nil empty slice")
	}
	if acc.Counts.ByKind == nil {
		t.Error("Counts.ByKind is nil, want non-nil empty map")
	}
	if acc.SweepComplete {
		t.Error("SweepComplete = true for an unknown net, want false")
	}
	if acc.SweepDetail != "no checkpoints configured" {
		t.Errorf("SweepDetail = %q, want %q", acc.SweepDetail, "no checkpoints configured")
	}
}

func TestAccountingCounts(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	// Two exceptions with the SAME bib (bib is a label, not a key — both
	// must count).
	if _, err := ts.Course.RecordRider(store.RiderException{NetID: netID, Bib: "412", Kind: course.KindSAG}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}
	if _, err := ts.Course.RecordRider(store.RiderException{NetID: netID, Bib: "412", Kind: course.KindSAG}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}
	if _, err := ts.Course.RecordRider(store.RiderException{NetID: netID, Bib: "500", Kind: course.KindDNF}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}

	acc := ts.Reconcile.Accounting(netID)
	if acc.Counts.Supported != 2 {
		t.Errorf("Counts.Supported = %d, want 2 (two sag exceptions default to supported)", acc.Counts.Supported)
	}
	if acc.Counts.Unsupported != 1 {
		t.Errorf("Counts.Unsupported = %d, want 1 (dnf defaults to unsupported)", acc.Counts.Unsupported)
	}
	if acc.Counts.ByKind[course.KindSAG] != 2 {
		t.Errorf("Counts.ByKind[sag] = %d, want 2", acc.Counts.ByKind[course.KindSAG])
	}
	if len(acc.OpenExceptions) != 2 {
		t.Errorf("len(OpenExceptions) = %d, want 2 (only supported ones are still open)", len(acc.OpenExceptions))
	}
}

func TestAccountingSweepFromCourse(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	stations := ts.seedStations(t, netID, 3)

	// No passages yet: sweep has not reached the last station.
	acc := ts.Reconcile.Accounting(netID)
	if acc.SweepComplete {
		t.Error("SweepComplete = true with no sweep passages, want false")
	}

	// Sweep passes station 1 and 2, but not 3 (the last).
	for _, seq := range []int{1, 2} {
		if _, err := ts.CP.LogPassage(store.CheckpointPassage{
			CheckpointID: stations[seq-1].ID, NetID: netID, Label: "SWEEP",
		}); err != nil {
			t.Fatalf("LogPassage seq %d: %v", seq, err)
		}
	}
	acc = ts.Reconcile.Accounting(netID)
	if acc.SweepComplete {
		t.Errorf("SweepComplete = true after passing only 2 of 3 stations, want false (detail: %s)", acc.SweepDetail)
	}

	// Sweep passes the final station too.
	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[2].ID, NetID: netID, Label: "SWEEP",
	}); err != nil {
		t.Fatalf("LogPassage seq 3: %v", err)
	}
	acc = ts.Reconcile.Accounting(netID)
	if !acc.SweepComplete {
		t.Errorf("SweepComplete = false after sweep passed every station, want true (detail: %s)", acc.SweepDetail)
	}
}
