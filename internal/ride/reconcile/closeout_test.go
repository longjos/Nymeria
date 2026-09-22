package reconcile

import (
	"errors"
	"strings"
	"testing"

	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/store"
)

func closeoutItem(status CloseoutStatus, key string) (CloseoutItem, bool) {
	for _, it := range status.Items {
		if it.Key == key {
			return it, true
		}
	}
	return CloseoutItem{}, false
}

func TestCloseoutItemsOrderAndKeys(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	status := ts.Reconcile.Closeout(netID)
	if status.Items == nil {
		t.Fatal("Items is nil, want non-nil")
	}
	wantOrder := []string{
		"sweep_finished", "exceptions_resolved", "rest_stops_closed",
		"field_units_out", "handoff_cleared", "shift_summaries_filed", "net_closed",
	}
	if len(status.Items) != len(wantOrder) {
		t.Fatalf("len(Items) = %d, want %d", len(status.Items), len(wantOrder))
	}
	for i, key := range wantOrder {
		if status.Items[i].Key != key {
			t.Errorf("Items[%d].Key = %q, want %q", i, status.Items[i].Key, key)
		}
	}
}

func TestCloseoutBlockedWithNoCheckpointsConfigured(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	// A ride net that never configured any sequenced stations can never
	// confirm the sweep is done — sweep_finished blocks close-out until
	// either checkpoints are added or NCS force-closes with a reason.
	status := ts.Reconcile.Closeout(netID)
	if status.Ready {
		t.Fatal("Ready = true with no checkpoints configured, want false")
	}
	item, _ := closeoutItem(status, "sweep_finished")
	if item.Done {
		t.Error("sweep_finished.Done = true with no checkpoints, want false")
	}
	if item.Detail != "no checkpoints configured" {
		t.Errorf("sweep_finished.Detail = %q, want %q", item.Detail, "no checkpoints configured")
	}
}

func TestCloseoutReadyWhenEverythingClear(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	stations := ts.seedCheckpointStations(t, netID, 1)
	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[0].ID, NetID: netID, Label: "SWEEP",
	}); err != nil {
		t.Fatalf("LogPassage: %v", err)
	}
	// Sweep has passed the only (and therefore last) station, no rest
	// stops, no field units, no handoffs, no sag units: every remaining
	// item should report "nothing to do" and be Done.
	status := ts.Reconcile.Closeout(netID)
	if !status.Ready {
		for _, it := range status.Items {
			if it.Blocking && !it.Done {
				t.Logf("blocking+not done: %+v", it)
			}
		}
		t.Fatal("Ready = false with nothing outstanding, want true")
	}
}

func TestCloseoutBlockedByOpenException(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	if _, err := ts.Course.RecordRider(store.RiderException{NetID: netID, Bib: "1", Kind: course.KindSAG}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}

	status := ts.Reconcile.Closeout(netID)
	if status.Ready {
		t.Fatal("Ready = true with an open exception, want false")
	}
	item, _ := closeoutItem(status, "exceptions_resolved")
	if item.Done {
		t.Error("exceptions_resolved.Done = true, want false")
	}
}

func TestCloseoutRestStopsClosed(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	ann, err := ts.Ann.Create(store.Annotation{
		Type: "point", Label: "RS1", Category: "aid", Status: "active",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
		NetID:    netID,
	})
	if err != nil {
		t.Fatalf("create rest stop: %v", err)
	}

	status := ts.Reconcile.Closeout(netID)
	item, _ := closeoutItem(status, "rest_stops_closed")
	if item.Done {
		t.Error("rest_stops_closed.Done = true with an active rest stop, want false")
	}
	if !item.Blocking {
		t.Error("rest_stops_closed.Blocking = false, want true (default policy)")
	}

	if _, err := ts.Ann.ChangeStatusUnguarded(ann.ID, "closed"); err != nil {
		t.Fatalf("close rest stop: %v", err)
	}
	status = ts.Reconcile.Closeout(netID)
	item, _ = closeoutItem(status, "rest_stops_closed")
	if !item.Done {
		t.Error("rest_stops_closed.Done = false after closing the only rest stop, want true")
	}
}

func TestCloseoutRestStopsClosedNotBlockingWhenPolicyDisables(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	stations := ts.seedStations(t, netID, 1)
	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[0].ID, NetID: netID, Label: "SWEEP",
	}); err != nil {
		t.Fatalf("LogPassage: %v", err)
	}
	if _, err := ts.Ann.Create(store.Annotation{
		Type: "point", Label: "RS1", Category: "aid", Status: "active",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`, NetID: netID,
	}); err != nil {
		t.Fatalf("create rest stop: %v", err)
	}

	ts.Reconcile.SetPolicyProvider(func(string) Policy {
		p := DefaultPolicy()
		p.RequireRestStopsClosed = false
		return p
	})

	status := ts.Reconcile.Closeout(netID)
	item, _ := closeoutItem(status, "rest_stops_closed")
	if item.Blocking {
		t.Error("rest_stops_closed.Blocking = true with RequireRestStopsClosed=false, want false")
	}
	if !status.Ready {
		t.Error("Ready = false, want true — an active rest stop must not block when the policy makes it advisory")
	}
}

func TestCloseoutFieldUnitsOut(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	sagID := ts.newSAGCheckIn(t, netID, "K6ABC", "SAG 1")
	cmdCi, err := ts.Net.CheckIn(netID, "N6CMD", netcontrol.TrafficNone, netcontrol.CatCommand)
	if err != nil {
		t.Fatalf("CheckIn command: %v", err)
	}

	// Command check-ins never block: only the sag unit matters.
	status := ts.Reconcile.Closeout(netID)
	item, _ := closeoutItem(status, "field_units_out")
	if item.Done {
		t.Error("field_units_out.Done = true with an un-released sag unit, want false")
	}
	if item.Total != 1 {
		t.Errorf("field_units_out.Total = %d, want 1 (command excluded)", item.Total)
	}

	if err := ts.Net.CheckOut(netID, sagID); err != nil {
		t.Fatalf("CheckOut sag: %v", err)
	}
	status = ts.Reconcile.Closeout(netID)
	item, _ = closeoutItem(status, "field_units_out")
	if !item.Done {
		t.Error("field_units_out.Done = false after releasing the only sag unit, want true")
	}
	_ = cmdCi
}

func TestCloseoutNetGate(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")

	// Not ready (an open exception), no force: ErrNotReady, net stays open.
	if _, err := ts.Course.RecordRider(store.RiderException{NetID: netID, Bib: "1", Kind: course.KindSAG}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}
	_, _, status, err := ts.Reconcile.CloseNet(netID, "K6ABC", "", false)
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("CloseNet error = %v, want ErrNotReady", err)
	}
	if status.Ready {
		t.Error("returned status.Ready = true, want false")
	}
	if n, _ := ts.Net.GetNet(netID); n.Status != netcontrol.StatusOpen {
		t.Errorf("net status = %q, want still open", n.Status)
	}

	// Force without a reason: rejected.
	if _, _, _, err := ts.Reconcile.CloseNet(netID, "K6ABC", "", true); err == nil {
		t.Fatal("CloseNet(force, no reason) = nil error, want an error")
	}

	// Force with a reason: closes despite the incomplete checklist, and
	// logs a forced-close timeline entry naming the pending item.
	n, summary, status, err := ts.Reconcile.CloseNet(netID, "K6ABC", "sweep radio dead", true)
	if err != nil {
		t.Fatalf("CloseNet(force, reason): %v", err)
	}
	if n.Status != netcontrol.StatusClosed {
		t.Errorf("net status = %q, want closed", n.Status)
	}
	if summary == nil {
		t.Error("summary is nil, want a NetSummary")
	}
	item, _ := closeoutItem(status, "net_closed")
	if !item.Done {
		t.Error("net_closed.Done = false after CloseNet, want true")
	}

	events, err := ts.Net.GetEvents(netID)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == TimelineCloseoutForced && strings.Contains(e.Details, "exceptions_resolved") {
			found = true
		}
	}
	if !found {
		t.Error("no ride_closeout_forced timeline entry naming the pending item")
	}
}

func TestCloseoutForceBlockedByPolicy(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	if _, err := ts.Course.RecordRider(store.RiderException{NetID: netID, Bib: "1", Kind: course.KindSAG}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}
	ts.Reconcile.SetPolicyProvider(func(string) Policy {
		p := DefaultPolicy()
		p.AllowForceClose = false
		return p
	})

	_, _, _, err := ts.Reconcile.CloseNet(netID, "K6ABC", "please", true)
	if !errors.Is(err, ErrForceBlocked) {
		t.Fatalf("CloseNet error = %v, want ErrForceBlocked", err)
	}
	if n, _ := ts.Net.GetNet(netID); n.Status != netcontrol.StatusOpen {
		t.Errorf("net status = %q, want still open", n.Status)
	}
}

func TestCloseoutReadyClosesCleanlyNoForcedEvent(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newOpenNet(t, "Test Ride")
	stations := ts.seedCheckpointStations(t, netID, 1)
	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[0].ID, NetID: netID, Label: "SWEEP",
	}); err != nil {
		t.Fatalf("LogPassage: %v", err)
	}

	n, _, status, err := ts.Reconcile.CloseNet(netID, "K6ABC", "", false)
	if err != nil {
		t.Fatalf("CloseNet: %v", err)
	}
	if n.Status != netcontrol.StatusClosed {
		t.Errorf("net status = %q, want closed", n.Status)
	}
	if !status.Ready {
		t.Error("returned status.Ready = false, want true")
	}

	events, _ := ts.Net.GetEvents(netID)
	for _, e := range events {
		if e.Type == TimelineCloseoutForced {
			t.Error("found ride_closeout_forced timeline entry on a clean close, want none")
		}
	}
}
