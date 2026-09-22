package ride

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/store"
)

// newTestManager builds a ride.Manager backed by a real SQLite store, a
// real netcontrol.Manager (as CheckInSource) and a real annotation.Manager.
func newTestManager(t *testing.T) (*Manager, *netcontrol.Manager, *annotation.Manager, *store.SQLiteStore) {
	t.Helper()
	db := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	netMgr := netcontrol.NewManager(db, nil)
	if err := netMgr.Load(); err != nil {
		t.Fatalf("netcontrol Load: %v", err)
	}
	annMgr := annotation.NewManager(db)
	if err := annMgr.Load(); err != nil {
		t.Fatalf("annotation Load: %v", err)
	}

	m := NewManager(db, netMgr, annMgr, DefaultConfig())
	return m, netMgr, annMgr, db
}

// createTestNet creates a draft bike-ride net.
func createTestNet(t *testing.T, netMgr *netcontrol.Manager) *store.Net {
	t.Helper()
	n, err := netMgr.CreateNet(store.Net{Name: "Test Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	return n
}

// sagCheckIn checks in a sag-category vehicle and gives it a tactical call.
func sagCheckIn(t *testing.T, netMgr *netcontrol.Manager, netID, tactical string) *store.NetCheckIn {
	t.Helper()
	callsign := "K" + strings.ToUpper(tactical)
	ci, err := netMgr.CheckIn(netID, callsign, netcontrol.TrafficNone, netcontrol.CatSAG)
	if err != nil {
		t.Fatalf("CheckIn: %v", err)
	}
	ci.TacticalCall = tactical
	updated, err := netMgr.UpdateCheckIn(*ci)
	if err != nil {
		t.Fatalf("UpdateCheckIn: %v", err)
	}
	return updated
}

func drainEvents(ch <-chan Event) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// expectEventType reads events off ch until it finds one of wantType (a
// mutation emits its own domain event alongside a net_timeline_entry row,
// and the two are not required to come out in any particular order) or
// gives up after a handful of reads.
func expectEventType(t *testing.T, ch <-chan Event, wantType string) {
	t.Helper()
	for i := 0; i < 5; i++ {
		select {
		case evt := <-ch:
			if evt.Type == wantType {
				return
			}
		default:
			t.Fatalf("expected an event of type %q, channel drained without finding one", wantType)
			return
		}
	}
	t.Fatalf("expected an event of type %q within 5 reads", wantType)
}

// --- Pinning: literals this package hardcodes to avoid importing netcontrol ---

func TestNetControlLiteralsMatch(t *testing.T) {
	if catSAG != netcontrol.CatSAG {
		t.Errorf("catSAG = %q, want %q", catSAG, netcontrol.CatSAG)
	}
	if checkInReleased != netcontrol.OpReleased {
		t.Errorf("checkInReleased = %q, want %q", checkInReleased, netcontrol.OpReleased)
	}
	if netStatusClosed != netcontrol.StatusClosed {
		t.Errorf("netStatusClosed = %q, want %q", netStatusClosed, netcontrol.StatusClosed)
	}
	if netStatusArchived != netcontrol.StatusArchived {
		t.Errorf("netStatusArchived = %q, want %q", netStatusArchived, netcontrol.StatusArchived)
	}
	if netStatusOpen != netcontrol.StatusOpen {
		t.Errorf("netStatusOpen = %q, want %q", netStatusOpen, netcontrol.StatusOpen)
	}
	if TimelineEntryEventType != netcontrol.EventTimelineEntry {
		t.Errorf("TimelineEntryEventType = %q, want %q", TimelineEntryEventType, netcontrol.EventTimelineEntry)
	}
}

// --- CreateRequest ---

func TestCreateRequest_Defaults(t *testing.T) {
	m, netMgr, _, db := newTestManager(t)
	net := createTestNet(t, netMgr)

	req1, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:      store.SAGLocation{Kind: LocCourse},
		Dropoff:     store.SAGLocation{Kind: LocNextRestStop},
		Reason:      "flat",
		RequestedBy: "K6ABC",
	}, "Net Control")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if len(req1.Slots) != 1 {
		t.Fatalf("Slots len = %d, want 1", len(req1.Slots))
	}
	if req1.Slots[0].Disposition != SlotWaiting || !req1.Slots[0].HasBike {
		t.Errorf("slot = %+v, want waiting+hasBike", req1.Slots[0])
	}
	if req1.Priority != PriorityHigh {
		t.Errorf("Priority = %q, want %q (course pickup)", req1.Priority, PriorityHigh)
	}
	if req1.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", req1.Sequence)
	}
	if req1.Status != ReqOpen || !req1.NeedsVehicle {
		t.Errorf("status/needsVehicle = %s/%v, want open/true", req1.Status, req1.NeedsVehicle)
	}
	if req1.Slots == nil || req1.Legs == nil {
		t.Error("Slots/Legs must never be nil")
	}
	if req1.RequestedBy != "K6ABC" {
		t.Errorf("RequestedBy = %q, want K6ABC", req1.RequestedBy)
	}

	expectEventType(t, m.Events(), EventSAGRequestCreated)

	req2, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: LocRestStop},
		Dropoff: store.SAGLocation{Kind: LocFinish},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest 2: %v", err)
	}
	if req2.Sequence != 2 {
		t.Errorf("Sequence = %d, want 2", req2.Sequence)
	}
	if req2.Priority != PriorityMedium {
		t.Errorf("Priority = %q, want %q (reststop pickup)", req2.Priority, PriorityMedium)
	}

	evs, err := db.LoadNetEvents(net.ID)
	if err != nil {
		t.Fatalf("LoadNetEvents: %v", err)
	}
	found := false
	for _, e := range evs {
		if e.Type != TLSAGRequested {
			continue
		}
		found = true
		var details map[string]any
		if err := json.Unmarshal([]byte(e.Details), &details); err != nil {
			t.Fatalf("unmarshal details: %v", err)
		}
		if _, ok := details["bibs"]; !ok {
			t.Errorf("details = %v, want a bibs key", details)
		}
		for k := range details {
			if k != "bibs" {
				t.Errorf("details has unexpected key %q (privacy: bibs only)", k)
			}
		}
		if strings.Contains(e.Details, "riderName") || strings.Contains(e.Summary, "riderName") {
			t.Error("timeline row must never contain a rider name")
		}
	}
	if !found {
		t.Error("expected a sag_requested timeline row")
	}
}

func TestCreateRequest_Validation(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	validPickup := store.SAGLocation{Kind: LocCourse}
	validDropoff := store.SAGLocation{Kind: LocNextRestStop}

	tests := []struct {
		name    string
		netID   string
		in      CreateRequestInput
		wantErr string
	}{
		{
			name:    "missing pickup kind",
			netID:   net.ID,
			in:      CreateRequestInput{Pickup: store.SAGLocation{}, Dropoff: validDropoff},
			wantErr: "pickup kind",
		},
		{
			name:    "invalid pickup kind",
			netID:   net.ID,
			in:      CreateRequestInput{Pickup: store.SAGLocation{Kind: "nowhere"}, Dropoff: validDropoff},
			wantErr: "pickup kind",
		},
		{
			name:    "invalid dropoff kind",
			netID:   net.ID,
			in:      CreateRequestInput{Pickup: validPickup, Dropoff: store.SAGLocation{Kind: "nowhere"}},
			wantErr: "dropoff kind",
		},
		{
			name:    "priority not in vocabulary",
			netID:   net.ID,
			in:      CreateRequestInput{Pickup: validPickup, Dropoff: validDropoff, Priority: "urgent"},
			wantErr: "priority",
		},
		{
			name:    "net not found",
			netID:   "does-not-exist",
			in:      CreateRequestInput{Pickup: validPickup, Dropoff: validDropoff},
			wantErr: "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.CreateRequest(tt.netID, tt.in, "")
			if err == nil {
				t.Fatalf("CreateRequest() = nil error, want one containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}

	t.Run("closed net", func(t *testing.T) {
		if err := netMgr.OpenNet(net.ID); err != nil {
			t.Fatalf("OpenNet: %v", err)
		}
		if _, _, err := netMgr.CloseNet(net.ID); err != nil {
			t.Fatalf("CloseNet: %v", err)
		}
		_, err := m.CreateRequest(net.ID, CreateRequestInput{Pickup: validPickup, Dropoff: validDropoff}, "")
		if err == nil {
			t.Fatal("CreateRequest on a closed net = nil error, want a refusal")
		}
	})
}

// --- The canonical partial-pickup trace ---

func TestPartialPickupTrace(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	sag5 := sagCheckIn(t, netMgr, net.ID, "SAG5")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 2, ""); err != nil {
		t.Fatalf("SetVehicle sag2: %v", err)
	}
	if _, err := m.SetVehicle(net.ID, sag5.ID, 3, 2, ""); err != nil {
		t.Fatalf("SetVehicle sag5: %v", err)
	}

	req, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: LocCourse},
		Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{
			{Bib: "A"}, {Bib: "B"}, {Bib: "C"},
		},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if req.Status != ReqOpen || !req.NeedsVehicle {
		t.Fatalf("initial status/needsVehicle = %s/%v, want open/true", req.Status, req.NeedsVehicle)
	}
	slotA, slotB, slotC := req.Slots[0].ID, req.Slots[1].ID, req.Slots[2].ID

	// Dispatch(SAG2, [A,B]) -> assigned; C still waiting+unlegged.
	req, err = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{slotA, slotB}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch(sag2, A,B): %v", err)
	}
	if req.Status != ReqAssigned || !req.NeedsVehicle {
		t.Fatalf("after dispatch: status/needsVehicle = %s/%v, want assigned/true", req.Status, req.NeedsVehicle)
	}
	if len(req.Legs) != 1 {
		t.Fatalf("Legs len = %d, want 1", len(req.Legs))
	}
	leg1 := req.Legs[0].ID

	vs, ok := m.VehicleStatus(net.ID, sag2.ID)
	if !ok || vs.CommittedSeats != 2 {
		t.Fatalf("sag2 committed seats = %v (ok=%v), want 2", vs, ok)
	}

	// AdvanceLeg(onscene) -> onscene.
	req, err = m.AdvanceLeg(net.ID, req.ID, leg1, LegOnScene, "NCS")
	if err != nil {
		t.Fatalf("AdvanceLeg onscene: %v", err)
	}
	if req.Status != ReqOnScene {
		t.Fatalf("status = %s, want onscene", req.Status)
	}

	// LoadSlots(leg1, [A,B]) -> A,B loaded; leg1 loaded -> partial (C waiting+unlegged).
	req, err = m.LoadSlots(net.ID, req.ID, leg1, []string{slotA, slotB}, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	if req.Status != ReqPartial || !req.NeedsVehicle {
		t.Fatalf("after load: status/needsVehicle = %s/%v, want partial/true", req.Status, req.NeedsVehicle)
	}
	for _, s := range req.Slots {
		if s.ID == slotA || s.ID == slotB {
			if s.Disposition != SlotLoaded {
				t.Errorf("slot %s disposition = %q, want loaded", s.ID, s.Disposition)
			}
		}
		if s.ID == slotC && (s.Disposition != SlotWaiting || s.LegID != "") {
			t.Errorf("slot C = %+v, want waiting+unlegged", s)
		}
	}

	// Dispatch(SAG5, [C]) -> leg2 dispatched; transporting (no unlegged waiting).
	req, err = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag5.ID, SlotIDs: []string{slotC}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch(sag5, C): %v", err)
	}
	if req.Status != ReqTransporting || req.NeedsVehicle {
		t.Fatalf("after 2nd dispatch: status/needsVehicle = %s/%v, want transporting/false", req.Status, req.NeedsVehicle)
	}
	if len(req.Legs) != 2 {
		t.Fatalf("Legs len = %d, want 2", len(req.Legs))
	}
	leg2 := req.Legs[1].ID

	// DeliverSlots(leg1, [A,B]) -> A,B delivered, leg1 delivered -> assigned (via leg2).
	req, err = m.DeliverSlots(net.ID, req.ID, leg1, []string{slotA, slotB}, nil, "NCS")
	if err != nil {
		t.Fatalf("DeliverSlots leg1: %v", err)
	}
	if req.Status != ReqAssigned {
		t.Fatalf("after delivering leg1: status = %s, want assigned (via leg2 dispatched)", req.Status)
	}
	for _, l := range req.Legs {
		if l.ID == leg1 && l.Status != LegDelivered {
			t.Errorf("leg1 status = %q, want delivered", l.Status)
		}
	}

	vs, ok = m.VehicleStatus(net.ID, sag2.ID)
	if !ok || vs.CommittedSeats != 0 {
		t.Fatalf("sag2 committed seats after delivery = %v (ok=%v), want 0", vs, ok)
	}

	// ResolveSlot(C, self_resolved) -> leg2 auto-released; every slot terminal -> complete.
	req, err = m.ResolveSlot(net.ID, req.ID, slotC, SlotSelfResolved, "", "NCS")
	if err != nil {
		t.Fatalf("ResolveSlot C: %v", err)
	}
	if req.Status != ReqComplete {
		t.Fatalf("final status = %s, want complete", req.Status)
	}
	if req.ClosedAt == nil {
		t.Fatal("ClosedAt = nil, want set")
	}
	for _, l := range req.Legs {
		if l.ID == leg2 {
			if l.Status != LegReleased {
				t.Errorf("leg2 status = %q, want released", l.Status)
			}
			if !strings.Contains(l.ReleaseReason, "resolved") {
				t.Errorf("leg2 ReleaseReason = %q, want it to mention resolved", l.ReleaseReason)
			}
		}
	}
}

// --- Dispatch / capacity ---

func TestDispatch_CapacityGuard(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}, {Bib: "B"}, {Bib: "C"}, {Bib: "D"}},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	slotIDs := []string{req.Slots[0].ID, req.Slots[1].ID, req.Slots[2].ID, req.Slots[3].ID}

	drainEvents(m.Events())
	_, err = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: slotIDs}, "NCS")
	if err == nil {
		t.Fatal("Dispatch over capacity = nil error, want ErrOverCapacity")
	}
	if !errors.Is(err, ErrOverCapacity) {
		t.Errorf("error = %v, want errors.Is ErrOverCapacity", err)
	}
	got, _ := m.GetRequest(net.ID, req.ID)
	if len(got.Legs) != 0 {
		t.Errorf("Legs len = %d, want 0 (no leg created on refusal)", len(got.Legs))
	}
	select {
	case evt := <-m.Events():
		t.Errorf("unexpected event on refusal: %+v", evt)
	default:
	}

	req2, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: slotIDs, AllowOvercommit: true}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch with AllowOvercommit: %v", err)
	}
	if len(req2.Legs) != 1 || !req2.Legs[0].Overcommitted {
		t.Fatalf("Legs = %+v, want one Overcommitted leg", req2.Legs)
	}

	// Racks exceeded with seats OK is also refused.
	sag9 := sagCheckIn(t, netMgr, net.ID, "SAG9")
	if _, err := m.SetVehicle(net.ID, sag9.ID, 4, 1, ""); err != nil {
		t.Fatalf("SetVehicle sag9: %v", err)
	}
	req3, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "E"}, {Bib: "F"}},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest 3: %v", err)
	}
	_, err = m.Dispatch(net.ID, req3.ID, DispatchInput{
		VehicleCheckInID: sag9.ID,
		SlotIDs:          []string{req3.Slots[0].ID, req3.Slots[1].ID},
	}, "NCS")
	if !errors.Is(err, ErrOverCapacity) {
		t.Errorf("racks-exceeded dispatch error = %v, want ErrOverCapacity", err)
	}
}

func TestDispatch_VehicleValidation(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)

	medCI, err := netMgr.CheckIn(net.ID, "KMED1", netcontrol.TrafficNone, netcontrol.CatMedical)
	if err != nil {
		t.Fatalf("CheckIn medical: %v", err)
	}
	released := sagCheckIn(t, netMgr, net.ID, "SAG-OLD")
	if err := netMgr.CheckOut(net.ID, released.ID); err != nil {
		t.Fatalf("CheckOut: %v", err)
	}
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	slotA := req.Slots[0].ID

	tests := []struct {
		name string
		in   DispatchInput
	}{
		{"check-in id unknown", DispatchInput{VehicleCheckInID: "nope", SlotIDs: []string{slotA}}},
		{"category medical", DispatchInput{VehicleCheckInID: medCI.ID, SlotIDs: []string{slotA}}},
		{"status released", DispatchInput{VehicleCheckInID: released.ID, SlotIDs: []string{slotA}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := m.Dispatch(net.ID, req.ID, tt.in, "NCS"); err == nil {
				t.Error("Dispatch() = nil error, want a refusal")
			}
			got, _ := m.GetRequest(net.ID, req.ID)
			if len(got.Legs) != 0 {
				t.Errorf("Legs len = %d, want 0 (no state change)", len(got.Legs))
			}
		})
	}

	// Dispatch the real vehicle to legitimately occupy slot A.
	req, err = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{slotA}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	t.Run("slot already on active leg", func(t *testing.T) {
		if _, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{slotA}}, "NCS"); err == nil {
			t.Error("Dispatch() = nil error, want refusal (slot not waiting+unlegged)")
		}
	})

	// A slot that is not waiting (e.g. cancelled) cannot be dispatched.
	req2, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "Z"}},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest 2: %v", err)
	}
	slotZ := req2.Slots[0].ID
	if _, err := m.ResolveSlot(net.ID, req2.ID, slotZ, SlotDeclined, "", "NCS"); err != nil {
		t.Fatalf("ResolveSlot: %v", err)
	}
	t.Run("slot not waiting", func(t *testing.T) {
		if _, err := m.Dispatch(net.ID, req2.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{slotZ}}, "NCS"); err == nil {
			t.Error("Dispatch() = nil error, want refusal (slot not waiting)")
		}
	})
}

func TestDispatch_AcrossRequests_SharesCapacity(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req1, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}, {Bib: "B"}},
	}, "")
	req2, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "C"}, {Bib: "D"}},
	}, "")

	if _, err := m.Dispatch(net.ID, req1.ID, DispatchInput{
		VehicleCheckInID: sag2.ID, SlotIDs: []string{req1.Slots[0].ID, req1.Slots[1].ID},
	}, "NCS"); err != nil {
		t.Fatalf("Dispatch req1: %v", err)
	}

	_, err := m.Dispatch(net.ID, req2.ID, DispatchInput{
		VehicleCheckInID: sag2.ID, SlotIDs: []string{req2.Slots[0].ID, req2.Slots[1].ID},
	}, "NCS")
	if !errors.Is(err, ErrOverCapacity) {
		t.Fatalf("Dispatch req2 error = %v, want ErrOverCapacity (shared vehicle capacity)", err)
	}

	req2b, err := m.Dispatch(net.ID, req2.ID, DispatchInput{
		VehicleCheckInID: sag2.ID, SlotIDs: []string{req2.Slots[0].ID, req2.Slots[1].ID}, AllowOvercommit: true,
	}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch req2 with overcommit: %v", err)
	}
	if !req2b.Legs[0].Overcommitted {
		t.Error("req2 leg not marked Overcommitted")
	}
}

// --- LoadSlots / DeliverSlots / ResolveSlot / ReleaseLeg ---

func TestLoadSlots_SubsetDetachesRest(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}, {Bib: "B"}, {Bib: "C"}},
	}, "")
	a, b, c := req.Slots[0].ID, req.Slots[1].ID, req.Slots[2].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a, b, c}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID

	req, err = m.LoadSlots(net.ID, req.ID, legID, []string{a, b}, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	if req.Status != ReqPartial {
		t.Errorf("status = %s, want partial", req.Status)
	}
	for _, s := range req.Slots {
		if s.ID == c {
			if s.Disposition != SlotWaiting || s.LegID != "" {
				t.Errorf("slot C = %+v, want waiting+unlegged", s)
			}
		}
	}
	for _, l := range req.Legs {
		if l.ID == legID {
			for _, sid := range l.SlotIDs {
				if sid == c {
					t.Error("leg still lists slot C after partial load")
				}
			}
		}
	}

	// C is now dispatchable to another vehicle.
	sag5 := sagCheckIn(t, netMgr, net.ID, "SAG5")
	if _, err := m.SetVehicle(net.ID, sag5.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle sag5: %v", err)
	}
	if _, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag5.ID, SlotIDs: []string{c}}, "NCS"); err != nil {
		t.Fatalf("Dispatch(sag5, C): %v", err)
	}
}

func TestLoadSlots_Skips(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	a := req.Slots[0].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID
	if req.Legs[0].Status != LegDispatched {
		t.Fatalf("leg status = %q, want dispatched", req.Legs[0].Status)
	}

	req, err = m.LoadSlots(net.ID, req.ID, legID, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots (skipping enroute/onscene): %v", err)
	}
	if req.Legs[0].Status != LegLoaded {
		t.Fatalf("leg status = %q, want loaded", req.Legs[0].Status)
	}
	if req.Legs[0].LoadedAt == nil {
		t.Error("LoadedAt = nil, want set")
	}
	if req.Legs[0].OnSceneAt != nil {
		t.Error("OnSceneAt should remain nil, was never visited")
	}
}

func TestDeliverSlots_PartialDelivery(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}, {Bib: "B"}},
	}, "")
	a, b := req.Slots[0].ID, req.Slots[1].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a, b}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID
	req, err = m.LoadSlots(net.ID, req.ID, legID, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}

	destX := store.SAGLocation{Kind: LocRestStop, Description: "reststop X"}
	req, err = m.DeliverSlots(net.ID, req.ID, legID, []string{a}, &destX, "NCS")
	if err != nil {
		t.Fatalf("DeliverSlots A: %v", err)
	}
	for _, s := range req.Slots {
		if s.ID == a {
			if s.Disposition != SlotDelivered || s.DeliveredTo == nil || s.DeliveredTo.Description != "reststop X" {
				t.Errorf("slot A = %+v, want delivered to reststop X", s)
			}
		}
		if s.ID == b && s.Disposition != SlotLoaded {
			t.Errorf("slot B = %+v, want still loaded", s)
		}
	}
	for _, l := range req.Legs {
		if l.ID == legID && l.Status != LegLoaded {
			t.Errorf("leg status = %q, want still loaded (partial delivery)", l.Status)
		}
	}

	req, err = m.DeliverSlots(net.ID, req.ID, legID, []string{b}, nil, "NCS")
	if err != nil {
		t.Fatalf("DeliverSlots B: %v", err)
	}
	for _, l := range req.Legs {
		if l.ID == legID && l.Status != LegDelivered {
			t.Errorf("leg status = %q, want delivered", l.Status)
		}
	}
	if req.Status != ReqComplete {
		t.Errorf("status = %s, want complete", req.Status)
	}
}

func TestDeliverSlots_HospitalRequiresName(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	setup := func(t *testing.T) (*store.SAGRequest, string) {
		req, _ := m.CreateRequest(net.ID, CreateRequestInput{
			Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocHospital},
			Slots: []SlotInput{{Bib: "A"}},
		}, "")
		a := req.Slots[0].ID
		req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a}}, "NCS")
		if err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		req, err = m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, nil, "NCS")
		if err != nil {
			t.Fatalf("LoadSlots: %v", err)
		}
		return req, req.Legs[0].ID
	}

	req, legID := setup(t)
	if _, err := m.DeliverSlots(net.ID, req.ID, legID, nil, nil, "NCS"); err == nil {
		t.Error("DeliverSlots to hospital without a rider name = nil error, want refusal")
	}

	if _, err := m.UpdateSlot(net.ID, req.ID, req.Slots[0].ID, SlotInput{Bib: "A", RiderName: "Jane Doe"}); err != nil {
		t.Fatalf("UpdateSlot: %v", err)
	}
	if _, err := m.DeliverSlots(net.ID, req.ID, legID, nil, nil, "NCS"); err != nil {
		t.Fatalf("DeliverSlots with rider name: %v", err)
	}

	// With RequireNameForHospitalStart off, no name is needed.
	cfg := m.Config()
	cfg.RequireNameForHospitalStart = false
	m.SetConfig(cfg)

	req2, legID2 := setup(t)
	if _, err := m.DeliverSlots(net.ID, req2.ID, legID2, nil, nil, "NCS"); err != nil {
		t.Fatalf("DeliverSlots (name not required): %v", err)
	}

	// "start" (their own car) behaves the same as hospital.
	cfg.RequireNameForHospitalStart = true
	m.SetConfig(cfg)
	req3, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocStart},
		Slots: []SlotInput{{Bib: "Z"}},
	}, "")
	z := req3.Slots[0].ID
	req3, err := m.Dispatch(net.ID, req3.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{z}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch req3: %v", err)
	}
	req3, err = m.LoadSlots(net.ID, req3.ID, req3.Legs[0].ID, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots req3: %v", err)
	}
	if _, err := m.DeliverSlots(net.ID, req3.ID, req3.Legs[0].ID, nil, nil, "NCS"); err == nil {
		t.Error("DeliverSlots to start without a rider name = nil error, want refusal")
	}
}

func TestResolveSlot_SelfResolvedAutoReleasesEmptyLeg(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}},
	}, "")
	a := req.Slots[0].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID

	req, err = m.ResolveSlot(net.ID, req.ID, a, SlotSelfResolved, "fixed own flat", "NCS")
	if err != nil {
		t.Fatalf("ResolveSlot: %v", err)
	}
	for _, l := range req.Legs {
		if l.ID == legID {
			if l.Status != LegReleased {
				t.Errorf("leg status = %q, want released", l.Status)
			}
			if !strings.Contains(l.ReleaseReason, "resolved") {
				t.Errorf("ReleaseReason = %q, want it to mention resolved", l.ReleaseReason)
			}
		}
	}
	vs, ok := m.VehicleStatus(net.ID, sag2.ID)
	if !ok || vs.CommittedSeats != 0 {
		t.Fatalf("committed seats = %v (ok=%v), want 0", vs, ok)
	}
	if req.Status != ReqComplete {
		t.Errorf("status = %s, want complete", req.Status)
	}
}

func TestResolveSlot_OnLegWithOthers(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}, {Bib: "B"}},
	}, "")
	a, b := req.Slots[0].ID, req.Slots[1].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a, b}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID

	req, err = m.ResolveSlot(net.ID, req.ID, a, SlotNotFound, "", "NCS")
	if err != nil {
		t.Fatalf("ResolveSlot: %v", err)
	}
	for _, l := range req.Legs {
		if l.ID == legID {
			if l.Status != LegDispatched {
				t.Errorf("leg status = %q, want still dispatched", l.Status)
			}
			if len(l.SlotIDs) != 2 || (l.SlotIDs[0] != b && l.SlotIDs[1] != b) {
				t.Errorf("leg SlotIDs = %v, want to still include B", l.SlotIDs)
			}
		}
	}
}

func TestResolveSlot_LoadedOnlyHandedOff(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}},
	}, "")
	a := req.Slots[0].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	req, err = m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	legID := req.Legs[0].ID

	if _, err := m.ResolveSlot(net.ID, req.ID, a, SlotSelfResolved, "", "NCS"); err == nil {
		t.Error("ResolveSlot(loaded, self_resolved) = nil error, want refusal")
	}

	req, err = m.ResolveSlot(net.ID, req.ID, a, SlotHandedOff, "ambulance on scene", "NCS")
	if err != nil {
		t.Fatalf("ResolveSlot(loaded, handed_off): %v", err)
	}
	for _, l := range req.Legs {
		if l.ID == legID && l.Status != LegDelivered {
			t.Errorf("leg status = %q, want delivered (last loaded slot resolved)", l.Status)
		}
	}
}

func TestReleaseLeg(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	a := req.Slots[0].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID
	req, err = m.AdvanceLeg(net.ID, req.ID, legID, LegOnScene, "NCS")
	if err != nil {
		t.Fatalf("AdvanceLeg: %v", err)
	}

	req, err = m.ReleaseLeg(net.ID, req.ID, legID, "wrong vehicle sent", "NCS")
	if err != nil {
		t.Fatalf("ReleaseLeg: %v", err)
	}
	if req.Status != ReqOpen || !req.NeedsVehicle {
		t.Errorf("status/needsVehicle = %s/%v, want open/true", req.Status, req.NeedsVehicle)
	}
	for _, s := range req.Slots {
		if s.ID == a && (s.Disposition != SlotWaiting || s.LegID != "") {
			t.Errorf("slot = %+v, want waiting+unlegged", s)
		}
	}

	req, err = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a}}, "NCS")
	if err != nil {
		t.Fatalf("re-dispatch: %v", err)
	}
	legID2 := req.Legs[len(req.Legs)-1].ID
	req, err = m.LoadSlots(net.ID, req.ID, legID2, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	if _, err := m.ReleaseLeg(net.ID, req.ID, legID2, "changed my mind", "NCS"); err == nil {
		t.Error("ReleaseLeg(loaded) = nil error, want refusal")
	}
}

// --- Cancel / AddSlot / UpdateRequest ---

func TestCancelRequest(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	a := req.Slots[0].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	req, err = m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	if _, err := m.CancelRequest(net.ID, req.ID, "duplicate", "NCS"); err == nil {
		t.Error("CancelRequest with a loaded slot = nil error, want refusal")
	}

	req2, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	b := req2.Slots[0].ID
	req2, err = m.Dispatch(net.ID, req2.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{b}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch req2: %v", err)
	}

	drainEvents(m.Events())
	req2, err = m.CancelRequest(net.ID, req2.ID, "duplicate entry", "NCS")
	if err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}
	if req2.Status != ReqCancelled || req2.ClosedAt == nil {
		t.Errorf("status/closedAt = %s/%v, want cancelled/set", req2.Status, req2.ClosedAt)
	}
	for _, s := range req2.Slots {
		if s.Disposition != SlotCancelled {
			t.Errorf("slot disposition = %q, want cancelled", s.Disposition)
		}
	}
	for _, l := range req2.Legs {
		if l.Status != LegReleased {
			t.Errorf("leg status = %q, want released", l.Status)
		}
	}
	expectEventType(t, m.Events(), EventSAGRequestUpdated)
}

func TestAddSlot_ReopensComplete(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 3, 3, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	a := req.Slots[0].ID
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{a}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	req, err = m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	req, err = m.DeliverSlots(net.ID, req.ID, req.Legs[0].ID, nil, nil, "NCS")
	if err != nil {
		t.Fatalf("DeliverSlots: %v", err)
	}
	if req.Status != ReqComplete || req.ClosedAt == nil {
		t.Fatalf("precondition: status/closedAt = %s/%v, want complete/set", req.Status, req.ClosedAt)
	}

	req, err = m.AddSlot(net.ID, req.ID, SlotInput{Bib: "Z"})
	if err != nil {
		t.Fatalf("AddSlot on a complete request: %v", err)
	}
	if req.Status != ReqPartial || !req.NeedsVehicle {
		t.Errorf("status/needsVehicle = %s/%v, want partial/true", req.Status, req.NeedsVehicle)
	}
	if req.ClosedAt != nil {
		t.Error("ClosedAt still set after reopening")
	}
}

func TestUpdateRequest_RefusedWhenClosed(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	if _, err := m.CancelRequest(net.ID, req.ID, "test", "NCS"); err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}
	_, err := m.UpdateRequest(net.ID, req.ID, UpdateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	})
	if err == nil {
		t.Error("UpdateRequest on a cancelled request = nil error, want refusal")
	}
}

func TestUpdateRequest_PriorityValidated(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	_, err := m.UpdateRequest(net.ID, req.ID, UpdateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Priority: "urgent",
	})
	if err == nil {
		t.Error("UpdateRequest with an invalid priority = nil error, want refusal")
	}

	updated, err := m.UpdateRequest(net.ID, req.ID, UpdateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Priority: PriorityLow, Notes: "changed",
	})
	if err != nil {
		t.Fatalf("UpdateRequest: %v", err)
	}
	if updated.Priority != PriorityLow || updated.Notes != "changed" {
		t.Errorf("updated = %+v, want priority=low notes=changed", updated)
	}
}

// --- Vehicles ---

func TestReleaseVehicle_OnCheckout(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, 4, 4, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req1, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	req1, err := m.Dispatch(net.ID, req1.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{req1.Slots[0].ID}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch req1: %v", err)
	}

	req2, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	req2, err = m.Dispatch(net.ID, req2.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{req2.Slots[0].ID}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch req2: %v", err)
	}
	req2, err = m.LoadSlots(net.ID, req2.ID, req2.Legs[0].ID, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots req2: %v", err)
	}

	drainEvents(m.Events())
	affected := m.ReleaseVehicle(net.ID, sag2.ID, "vehicle checked out")
	if len(affected) != 2 {
		t.Fatalf("affected len = %d, want 2", len(affected))
	}

	got1, _ := m.GetRequest(net.ID, req1.ID)
	if got1.Legs[0].Status != LegReleased {
		t.Errorf("req1 leg status = %q, want released", got1.Legs[0].Status)
	}
	got2, _ := m.GetRequest(net.ID, req2.ID)
	if got2.Legs[0].Status != LegLoaded {
		t.Errorf("req2 leg status = %q, want still loaded", got2.Legs[0].Status)
	}

	sawVehicleUpdated := false
	timeout := time.After(time.Second)
	for {
		select {
		case evt := <-m.Events():
			if evt.Type == EventSAGVehicleUpdated {
				sawVehicleUpdated = true
			}
		case <-timeout:
			goto done
		default:
			goto done
		}
	}
done:
	if !sawVehicleUpdated {
		t.Error("expected a sag_vehicle_updated event")
	}

	evs, err := m.store.LoadNetEvents(net.ID)
	if err != nil {
		t.Fatalf("LoadNetEvents: %v", err)
	}
	sawWarning := false
	for _, e := range evs {
		if strings.Contains(e.Summary, "riders still aboard") {
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Error("expected a warning timeline row about riders still aboard")
	}
}

func TestSetVehicle(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)

	medCI, err := netMgr.CheckIn(net.ID, "KMED1", netcontrol.TrafficNone, netcontrol.CatMedical)
	if err != nil {
		t.Fatalf("CheckIn medical: %v", err)
	}
	if _, err := m.SetVehicle(net.ID, medCI.ID, 3, 2, ""); err == nil {
		t.Error("SetVehicle on a non-sag check-in = nil error, want refusal")
	}

	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")
	if _, err := m.SetVehicle(net.ID, sag2.ID, -1, 2, ""); err == nil {
		t.Error("SetVehicle with negative seats = nil error, want refusal")
	}

	status, err := m.SetVehicle(net.ID, sag2.ID, 5, 1, "van")
	if err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	if status.Seats != 5 || status.RackSlots != 1 {
		t.Errorf("status = %+v, want seats=5 rackSlots=1", status)
	}

	sag5 := sagCheckIn(t, netMgr, net.ID, "SAG5")
	vehicles := m.Vehicles(net.ID)
	var unregistered *SAGVehicleStatus
	for i := range vehicles {
		if vehicles[i].CheckInID == sag5.ID {
			unregistered = &vehicles[i]
		}
	}
	if unregistered == nil {
		t.Fatal("unregistered sag vehicle missing from Vehicles()")
	}
	if unregistered.Seats != 3 || unregistered.RackSlots != 2 {
		t.Errorf("unregistered = %+v, want defaults 3/2", unregistered)
	}
	if unregistered.ActiveLegIDs == nil || unregistered.ActiveRequestIDs == nil {
		t.Error("ActiveLegIDs/ActiveRequestIDs must never be nil")
	}
}

func TestBoard_CountsHaveEveryStatusKey(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)

	board := m.Board(net.ID)
	if board.Requests == nil || board.Vehicles == nil {
		t.Error("Requests/Vehicles must never be nil")
	}
	for _, status := range allRequestStatuses {
		if _, ok := board.Counts[status]; !ok {
			t.Errorf("Counts missing status key %q", status)
		}
	}

	m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	board = m.Board(net.ID)
	if board.Counts[ReqOpen] != 1 {
		t.Errorf("Counts[open] = %d, want 1", board.Counts[ReqOpen])
	}
}

func TestSetConfig_HotSwap(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)

	m.SetConfig(Config{
		Priorities:            []string{"red", "yellow", "green"},
		DefaultCoursePriority: "red", DefaultStopPriority: "green",
	})

	if _, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop}, Priority: "red",
	}, ""); err != nil {
		t.Errorf("CreateRequest with priority in new vocabulary: %v", err)
	}
	if _, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop}, Priority: "high",
	}, ""); err == nil {
		t.Error("CreateRequest with the old vocabulary's priority = nil error, want refusal")
	}

	before := m.Config()
	m.SetConfig(Config{})
	after := m.Config()
	if len(after.Priorities) != len(before.Priorities) {
		t.Errorf("SetConfig with empty Priorities changed the config: before=%v after=%v", before, after)
	}
}

func TestLoadPersistence(t *testing.T) {
	db := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	defer db.Close()

	netMgr := netcontrol.NewManager(db, nil)
	if err := netMgr.Load(); err != nil {
		t.Fatalf("netcontrol Load: %v", err)
	}
	annMgr := annotation.NewManager(db)
	if err := annMgr.Load(); err != nil {
		t.Fatalf("annotation Load: %v", err)
	}
	net := createTestNet(t, netMgr)
	sag2 := sagCheckIn(t, netMgr, net.ID, "SAG2")

	m1 := NewManager(db, netMgr, annMgr, DefaultConfig())
	if _, err := m1.SetVehicle(net.ID, sag2.ID, 3, 2, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, err := m1.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	req, err = m1.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{req.Slots[0].ID}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	req, err = m1.LoadSlots(net.ID, req.ID, req.Legs[0].ID, nil, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}

	m2 := NewManager(db, netMgr, annMgr, DefaultConfig())
	if err := m2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	loaded, ok := m2.GetRequest(net.ID, req.ID)
	if !ok {
		t.Fatal("GetRequest after reload: not found")
	}
	if loaded.Status != req.Status || len(loaded.Legs) != len(req.Legs) {
		t.Errorf("reloaded = %+v, want matching status/legs of %+v", loaded, req)
	}

	req2, err := m2.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest after reload: %v", err)
	}
	if req2.Sequence != req.Sequence+1 {
		t.Errorf("Sequence = %d, want %d (continues from persisted count)", req2.Sequence, req.Sequence+1)
	}

	b, err := json.Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("marshaled request contains null: %s", b)
	}
}

func TestEventsNeverBlock(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 65; i++ {
			if _, err := m.CreateRequest(net.ID, CreateRequestInput{
				Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
			}, ""); err != nil {
				t.Errorf("CreateRequest %d: %v", i, err)
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CreateRequest deadlocked on a full events channel")
	}
}
