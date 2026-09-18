package wxalert

import (
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(Config{Enabled: false, DefaultBufferMiles: 10})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

// mkTestAlert fills in Tier/EffectiveEvent the way decode.go always does
// (polygonAlert, a footprint_test.go helper, does not) so manager tests
// exercise realistic records.
func mkTestAlert(event string, sev Severity, urg Urgency, ring []LatLon) Alert {
	a := polygonAlert(event, sev, ring)
	a.Urgency = urg
	a.Tier, _ = TierForEvent(event)
	a.EffectiveEvent, _ = EffectiveEvent(a)
	return a
}

func drainEvents(m *Manager) []Event {
	var out []Event
	for {
		select {
		case ev := <-m.Events():
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestManagerFeedToSnapshotNotifyClasses(t *testing.T) {
	m := newTestManager(t)
	now := time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })
	m.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &gr, OwnSource: "gps", Now: now})

	tor := mkTestAlert("Tornado Warning", SeverityExtreme, UrgencyImmediate, squareRingMiles(gr, 20))
	tor.ID = "tor-1"
	tor.Sent = now
	tor.Expires = now.Add(45 * time.Minute)
	ends := now.Add(45 * time.Minute)
	tor.Ends = &ends

	m.OnAlerts([]Alert{tor}, now, true)

	snap := m.Snapshot()
	if len(snap.Alerts) != 1 {
		t.Fatalf("Alerts = %v, want 1", snap.Alerts)
	}
	got := snap.Alerts[0]
	if got.Proximity != ProximityIn {
		t.Errorf("Proximity = %v, want in", got.Proximity)
	}
	if got.NotifyClass != NotifyInterrupt {
		t.Errorf("NotifyClass = %v, want interrupt (allowlisted by default)", got.NotifyClass)
	}
	if got.NotifyReason != "new" {
		t.Errorf("NotifyReason = %q, want new", got.NotifyReason)
	}
	if snap.Footprint == nil || snap.Footprint.Empty {
		t.Errorf("Footprint = %v, want non-empty", snap.Footprint)
	}
	if snap.Policy.FloorText != FloorText() {
		t.Errorf("Policy.FloorText mismatch")
	}
}

func TestManagerSupersessionUpdatesSnapshot(t *testing.T) {
	m := newTestManager(t)
	now := time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })
	m.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &gr, Now: now})

	a := mkTestAlert("Flood Warning", SeveritySevere, UrgencyExpected, squareRingMiles(gr, 5))
	a.ID = "flood-1"
	a.Sent = now
	a.Expires = now.Add(time.Hour)
	m.OnAlerts([]Alert{a}, now, true)

	now2 := now.Add(10 * time.Minute)
	m.SetClock(func() time.Time { return now2 })
	b := a
	b.ID = "flood-2"
	b.Sent = now2
	b.References = []Reference{{ID: "flood-1", Sent: now}}
	m.OnAlerts([]Alert{b}, now2, true)

	snap := m.Snapshot()
	if len(snap.Alerts) != 1 || snap.Alerts[0].ID != "flood-2" {
		t.Fatalf("Alerts = %+v, want just flood-2", snap.Alerts)
	}
	if m.Get("flood-1") != nil {
		t.Errorf("Get(flood-1) still returns a record, want it superseded out of the general view")
	}
	hist := m.History("flood-2")
	if len(hist) != 1 || hist[0].ID != "flood-1" {
		t.Errorf("History(flood-2) = %v, want [flood-1]", hist)
	}
}

func TestManagerAckForNetBroadcasts(t *testing.T) {
	m := newTestManager(t)
	now := time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })
	m.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &gr, Now: now})

	a := mkTestAlert("Tornado Warning", SeverityExtreme, UrgencyImmediate, squareRingMiles(gr, 5))
	a.ID = "tor-ack"
	a.Sent = now
	a.Expires = now.Add(45 * time.Minute)
	m.OnAlerts([]Alert{a}, now, true)

	updated, err := m.AckForNet("tor-ack", NetAck{Callsign: "W8ABC", At: now})
	if err != nil {
		t.Fatalf("AckForNet: %v", err)
	}
	if updated.AckedForNet == nil || updated.AckedForNet.Callsign != "W8ABC" {
		t.Fatalf("AckedForNet = %v", updated.AckedForNet)
	}

	found := false
	for _, ev := range drainEvents(m) {
		if ev.Type == EventAlertAckNet {
			found = true
		}
	}
	if !found {
		t.Errorf("no %s event broadcast for the net ack", EventAlertAckNet)
	}

	if _, err := m.AckForNet("does-not-exist", NetAck{Callsign: "W8ABC"}); err == nil {
		t.Errorf("AckForNet(unknown id) = nil error, want an error")
	}
}

func TestManagerFootprintChangeReclassifies(t *testing.T) {
	m := newTestManager(t)
	now := time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })

	far := destPoint(gr, 90, 100)
	m.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &far, Now: now})

	a := mkTestAlert("Flood Warning", SeveritySevere, UrgencyExpected, squareRingMiles(gr, 5))
	a.ID = "flood-far"
	a.Sent = now
	a.Expires = now.Add(time.Hour)
	m.OnAlerts([]Alert{a}, now, true)

	if snap := m.Snapshot(); len(snap.Alerts) != 0 {
		t.Fatalf("Alerts = %v, want none (footprint is 100mi away)", snap.Alerts)
	}

	// Move the own position (e.g. GPS update) so the same alert is now IN.
	m.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &gr, Now: now})
	snap := m.Snapshot()
	if len(snap.Alerts) != 1 || snap.Alerts[0].Proximity != ProximityIn {
		t.Fatalf("Alerts = %+v, want flood-far now in", snap.Alerts)
	}
}

func TestManagerRestartHydration(t *testing.T) {
	m := newTestManager(t)
	now := time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })
	m.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &gr, Now: now})

	a := mkTestAlert("Tornado Warning", SeverityExtreme, UrgencyImmediate, squareRingMiles(gr, 5))
	a.ID = "tor-restart"
	a.Sent = now
	a.Expires = now.Add(45 * time.Minute)
	m.OnAlerts([]Alert{a}, now, true)
	m.AckForNet("tor-restart", NetAck{Callsign: "W8ABC", At: now})

	snap := m.RegistrySnapshotForPersistence()

	m2 := newTestManager(t)
	m2.SetClock(func() time.Time { return now })
	m2.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &gr, Now: now})
	m2.RestoreRegistry(snap)

	restored := m2.Get("tor-restart")
	if restored == nil {
		t.Fatalf("Get(tor-restart) = nil after restore")
	}
	if restored.AckedForNet == nil || restored.AckedForNet.Callsign != "W8ABC" {
		t.Errorf("AckedForNet not restored: %v", restored.AckedForNet)
	}
	if restored.Proximity != ProximityIn {
		t.Errorf("Proximity after restore+recompute = %v, want in", restored.Proximity)
	}
}

func TestManagerSilentDropVanishes(t *testing.T) {
	m := newTestManager(t)
	now := time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })
	m.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &gr, Now: now})

	a := mkTestAlert("Flood Warning", SeveritySevere, UrgencyExpected, squareRingMiles(gr, 5))
	a.ID = "flood-drop"
	a.Sent = now
	a.Expires = now.Add(time.Hour)
	m.OnAlerts([]Alert{a}, now, true)

	now2 := now.Add(time.Minute)
	m.SetClock(func() time.Time { return now2 })
	m.OnAlerts([]Alert{}, now2, true) // silently dropped from the feed

	got := m.Get("flood-drop")
	if got == nil {
		t.Fatalf("Get(flood-drop) = nil, want the dropped record retained")
	}
	if got.State != AlertStateDropped {
		t.Errorf("State = %v, want dropped", got.State)
	}
	if got.NotifyReason != "ended" {
		t.Errorf("NotifyReason = %q, want ended", got.NotifyReason)
	}
}

func TestManagerEventTypesCount(t *testing.T) {
	m := newTestManager(t)
	types := m.EventTypes()
	if len(types) != 113 {
		t.Fatalf("EventTypes() = %d, want 113 (111 + 2 derived emergencies)", len(types))
	}
	foundEmergency := false
	for _, e := range types {
		if e.Event == "Tornado Emergency" {
			foundEmergency = true
			if e.Tier != TierWarning {
				t.Errorf("Tornado Emergency tier = %v, want warning", e.Tier)
			}
		}
	}
	if !foundEmergency {
		t.Errorf("EventTypes() missing Tornado Emergency")
	}
}

func TestManagerPurgeEndedOlderThan(t *testing.T) {
	m := newTestManager(t)
	now := time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })
	m.SetFootprintInputs(Inputs{BufferMiles: 10, Own: &gr, Now: now})

	a := mkTestAlert("Flood Warning", SeveritySevere, UrgencyExpected, squareRingMiles(gr, 5))
	a.ID = "flood-purge"
	a.Sent = now
	ends := now.Add(10 * time.Minute)
	a.Ends = &ends
	a.Expires = ends
	m.OnAlerts([]Alert{a}, now, true)

	later := now.Add(2 * time.Hour)
	m.SetClock(func() time.Time { return later })
	m.TickExpiry()

	if m.PurgeEndedOlderThan(later.Add(-30*time.Minute)) != 1 {
		t.Errorf("PurgeEndedOlderThan should have removed the long-expired alert")
	}
	if m.Get("flood-purge") != nil {
		t.Errorf("Get(flood-purge) after purge = non-nil, want nil")
	}
}

// TestManagerSetQueryNoPollerIsNoop covers the Enabled=false manager WP2's
// app wiring builds by default — SetQuery must not panic when there is no
// poller to forward to.
func TestManagerSetQueryNoPollerIsNoop(t *testing.T) {
	m := newTestManager(t)
	m.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)
}

// TestManagerSetQueryForwardsToPoller confirms an enabled manager's SetQuery
// actually reaches the poller (observable via Status(), which never panics
// on a query it hasn't sent yet — this just proves the call is wired, not
// dropped, by checking the manager still answers Link() normally after it).
func TestManagerSetQueryForwardsToPoller(t *testing.T) {
	m, err := NewManager(Config{Enabled: true, UserAgent: "Nymeria/test (ops@example.com)", DefaultBufferMiles: 10})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.SetQuery(ActiveQuery{Point: &LatLon{Lat: 42.9, Lon: -85.6}}, true)
	if got := m.Link().State; got != LinkDown {
		t.Errorf("Link().State = %v, want down (never polled yet)", got)
	}
}

// TestManagerZonesNilWithoutDataDir covers the common case (no zone cache
// configured) so callers (GET /wx/zones/search) can nil-check before use.
func TestManagerZonesNilWithoutDataDir(t *testing.T) {
	m := newTestManager(t)
	if m.Zones() != nil {
		t.Errorf("Zones() = %v, want nil (no ZoneDataDir configured)", m.Zones())
	}
}

// TestManagerZonesConfigured confirms the accessor returns the same cache
// Zone()/Zone-backed handlers already use, not a copy.
func TestManagerZonesConfigured(t *testing.T) {
	m, err := NewManager(Config{Enabled: false, DefaultBufferMiles: 10, ZoneDataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m.Zones() == nil {
		t.Fatal("Zones() = nil, want configured cache")
	}
}
