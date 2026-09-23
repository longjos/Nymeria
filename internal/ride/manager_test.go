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
	req, err = m.LoadSlots(net.ID, req.ID, leg1, LoadInput{SlotIDs: []string{slotA, slotB}}, "NCS")
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
	// 4 riders into 3 seats: a seatbelt count, so this is refused outright and
	// AllowOvercommit does not open it (see TestDispatch_SeatsAreAHardStop).
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

	if _, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: slotIDs, AllowOvercommit: true}, "NCS"); !errors.Is(err, ErrSeatsExceeded) {
		t.Fatalf("Dispatch with AllowOvercommit over SEATS = %v, want ErrSeatsExceeded (no override)", err)
	}

	// Three of them fit the belts exactly, and the racks are not short.
	req2, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: slotIDs[:3]}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch of 3 into 3 seats / 3 racks: %v", err)
	}
	if len(req2.Legs) != 1 || req2.Legs[0].Overcommitted {
		t.Fatalf("Legs = %+v, want one leg that is NOT overcommitted", req2.Legs)
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
	if errors.Is(err, ErrSeatsExceeded) {
		t.Error("racks-exceeded dispatch must not present as a seat refusal")
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
	// Belts for everyone, racks for three: the shared-capacity refusal lands
	// on the RACKS, which is the dimension an operator may still override.
	if _, err := m.SetVehicle(net.ID, sag2.ID, 5, 3, ""); err != nil {
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

	req, err = m.LoadSlots(net.ID, req.ID, legID, LoadInput{SlotIDs: []string{a, b}}, "NCS")
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

	req, err = m.LoadSlots(net.ID, req.ID, legID, LoadInput{}, "NCS")
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
	req, err = m.LoadSlots(net.ID, req.ID, legID, LoadInput{}, "NCS")
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
		req, err = m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, LoadInput{}, "NCS")
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
	req3, err = m.LoadSlots(net.ID, req3.ID, req3.Legs[0].ID, LoadInput{}, "NCS")
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
	req, err = m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, LoadInput{}, "NCS")
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
	req, err = m.LoadSlots(net.ID, req.ID, legID2, LoadInput{}, "NCS")
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
	req, err = m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, LoadInput{}, "NCS")
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
	req, err = m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, LoadInput{}, "NCS")
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
	req2, err = m.LoadSlots(net.ID, req2.ID, req2.Legs[0].ID, LoadInput{}, "NCS")
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
	req, err = m1.LoadSlots(net.ID, req.ID, req.Legs[0].ID, LoadInput{}, "NCS")
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

// A vehicle's committed seats/racks are DERIVED from live leg and slot
// state, never stored — so every operation that changes a leg or a slot
// changes the vehicle's capacity too. Only SetVehicle and ReleaseVehicle
// used to emit EventSAGVehicleUpdated, so a board that had already loaded
// kept showing the vehicle at full capacity through dispatch, load and
// delivery; the numbers only corrected themselves on a fresh GET /sag.
func TestCapacityChangingOpsEmitVehicleUpdated(t *testing.T) {
	// collectVehicleStatus drains pending events and returns the last
	// vehicle status emitted for checkInID, if any.
	collect := func(m *Manager, checkInID string) (SAGVehicleStatus, bool) {
		var last SAGVehicleStatus
		found := false
		for {
			select {
			case evt := <-m.Events():
				if evt.Type != EventSAGVehicleUpdated {
					continue
				}
				st, ok := evt.Data.(SAGVehicleStatus)
				if ok && st.CheckInID == checkInID {
					last, found = st, true
				}
			default:
				return last, found
			}
		}
	}

	t.Run("dispatch commits capacity", func(t *testing.T) {
		m, netMgr, _, _ := newTestManager(t)
		net := createTestNet(t, netMgr)
		sag := sagCheckIn(t, netMgr, net.ID, "SAG1")
		if _, err := m.SetVehicle(net.ID, sag.ID, 3, 2, ""); err != nil {
			t.Fatalf("SetVehicle: %v", err)
		}

		req, _ := m.CreateRequest(net.ID, CreateRequestInput{
			Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		}, "")

		drainEvents(m.Events())
		if _, err := m.Dispatch(net.ID, req.ID, DispatchInput{
			VehicleCheckInID: sag.ID, SlotIDs: []string{req.Slots[0].ID},
		}, "NCS"); err != nil {
			t.Fatalf("Dispatch: %v", err)
		}

		st, ok := collect(m, sag.ID)
		if !ok {
			t.Fatal("dispatch emitted no sag_vehicle_updated for the dispatched vehicle")
		}
		if st.CommittedSeats != 1 {
			t.Errorf("committedSeats = %d, want 1", st.CommittedSeats)
		}
		if st.AvailableSeats != 2 {
			t.Errorf("availableSeats = %d, want 2 (of 3)", st.AvailableSeats)
		}
	})

	t.Run("delivery frees capacity", func(t *testing.T) {
		m, netMgr, _, _ := newTestManager(t)
		net := createTestNet(t, netMgr)
		sag := sagCheckIn(t, netMgr, net.ID, "SAG1")
		if _, err := m.SetVehicle(net.ID, sag.ID, 3, 2, ""); err != nil {
			t.Fatalf("SetVehicle: %v", err)
		}

		req, _ := m.CreateRequest(net.ID, CreateRequestInput{
			Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		}, "")
		req, err := m.Dispatch(net.ID, req.ID, DispatchInput{
			VehicleCheckInID: sag.ID, SlotIDs: []string{req.Slots[0].ID},
		}, "NCS")
		if err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		legID := req.Legs[0].ID
		if _, err := m.LoadSlots(net.ID, req.ID, legID, LoadInput{}, "NCS"); err != nil {
			t.Fatalf("LoadSlots: %v", err)
		}

		drainEvents(m.Events())
		if _, err := m.DeliverSlots(net.ID, req.ID, legID, nil, &store.SAGLocation{Kind: LocNextRestStop}, "NCS"); err != nil {
			t.Fatalf("DeliverSlots: %v", err)
		}

		st, ok := collect(m, sag.ID)
		if !ok {
			t.Fatal("delivery emitted no sag_vehicle_updated for the vehicle")
		}
		if st.CommittedSeats != 0 {
			t.Errorf("committedSeats = %d, want 0 after delivery", st.CommittedSeats)
		}
		if st.AvailableSeats != 3 {
			t.Errorf("availableSeats = %d, want 3 (of 3)", st.AvailableSeats)
		}
	})

	t.Run("load keeps capacity committed", func(t *testing.T) {
		m, netMgr, _, _ := newTestManager(t)
		net := createTestNet(t, netMgr)
		sag := sagCheckIn(t, netMgr, net.ID, "SAG1")
		if _, err := m.SetVehicle(net.ID, sag.ID, 3, 2, ""); err != nil {
			t.Fatalf("SetVehicle: %v", err)
		}

		req, _ := m.CreateRequest(net.ID, CreateRequestInput{
			Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		}, "")
		req, err := m.Dispatch(net.ID, req.ID, DispatchInput{
			VehicleCheckInID: sag.ID, SlotIDs: []string{req.Slots[0].ID},
		}, "NCS")
		if err != nil {
			t.Fatalf("Dispatch: %v", err)
		}

		drainEvents(m.Events())
		if _, err := m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, LoadInput{}, "NCS"); err != nil {
			t.Fatalf("LoadSlots: %v", err)
		}

		st, ok := collect(m, sag.ID)
		if !ok {
			t.Fatal("load emitted no sag_vehicle_updated for the vehicle")
		}
		if st.CommittedSeats != 1 {
			t.Errorf("committedSeats = %d, want 1 while aboard", st.CommittedSeats)
		}
	})
}

// --- defect 1: "I added another rider and the seat count didn't go up" ---
//
// The radio exchange is "SAG 4, make that TWO riders at Maxwell." The van is
// already rolling. AddSlot with AttachToLegID puts the new rider on the leg
// that is already on its way, which is what was said on the air, and the
// vehicle's committed seats move immediately.

func TestAddSlot_AttachesToAnInFlightLeg(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	van := sagCheckIn(t, netMgr, net.ID, "sag1")

	req, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: LocCourse},
		Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots:   []SlotInput{{Bib: "334"}},
	}, "NCS")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	req, err = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: van.ID}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if _, err := m.AdvanceLeg(net.ID, req.ID, req.Legs[0].ID, LegEnroute, "NCS"); err != nil {
		t.Fatalf("AdvanceLeg: %v", err)
	}
	legID := req.Legs[0].ID

	before, _ := m.VehicleStatus(net.ID, van.ID)
	if before.CommittedSeats != 1 {
		t.Fatalf("committed seats before = %d, want 1", before.CommittedSeats)
	}

	req, err = m.AddSlot(net.ID, req.ID, SlotInput{Bib: "512", AttachToLegID: legID})
	if err != nil {
		t.Fatalf("AddSlot(attach): %v", err)
	}

	after, _ := m.VehicleStatus(net.ID, van.ID)
	if after.CommittedSeats != 2 || after.CommittedRacks != 2 {
		t.Errorf("committed = (%d seats, %d racks), want (2, 2) — the seat count must move when a rider joins a rolling van",
			after.CommittedSeats, after.CommittedRacks)
	}
	if req.NeedsVehicle {
		t.Error("NeedsVehicle = true; the new rider is on the van that is already en route")
	}
	if req.Status != ReqEnroute {
		t.Errorf("Status = %q, want %q — attaching must not knock the request back to open", req.Status, ReqEnroute)
	}
	var newSlot *store.SAGSlot
	for i := range req.Slots {
		if req.Slots[i].Bib == "512" {
			newSlot = &req.Slots[i]
		}
	}
	if newSlot == nil || newSlot.LegID != legID {
		t.Fatalf("new slot legId = %v, want %q", newSlot, legID)
	}
	if len(req.Legs[0].SlotIDs) != 2 {
		t.Errorf("leg slotIds = %v, want both riders", req.Legs[0].SlotIDs)
	}
}

func TestAddSlot_AttachValidation(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	van := sagCheckIn(t, netMgr, net.ID, "sag1")

	// Seats to spare, one rack: only the bike can overflow, and a bike may ride
	// in the bed of a truck, so the override is legitimate here. (Seats are a
	// hard stop on this path too — TestAddSlot_AttachSeatsAreAHardStop.)
	if _, err := m.SetVehicle(net.ID, van.ID, 4, 1, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}
	req, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: LocCourse},
		Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots:   []SlotInput{{Bib: "334"}},
	}, "NCS")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	req, err = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: van.ID}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID

	// Unknown leg.
	if _, err := m.AddSlot(net.ID, req.ID, SlotInput{Bib: "1", AttachToLegID: "nope"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddSlot to unknown leg err = %v, want ErrNotFound", err)
	}

	// Over capacity is refused with the numbers, exactly like Dispatch.
	_, err = m.AddSlot(net.ID, req.ID, SlotInput{Bib: "512", AttachToLegID: legID})
	if !errors.Is(err, ErrOverCapacity) {
		t.Fatalf("AddSlot over capacity err = %v, want ErrOverCapacity", err)
	}
	var capErr *OverCapacityError
	if !errors.As(err, &capErr) || capErr.RackSlots != 1 || capErr.CommittedRacks != 1 {
		t.Errorf("OverCapacityError = %+v, want rackSlots 1 committed 1", capErr)
	}
	if capErr.SeatsExceeded {
		t.Error("a rack overflow must not be reported as a seat refusal")
	}

	// ...and accepted when the operator knowingly overrides.
	req, err = m.AddSlot(net.ID, req.ID, SlotInput{Bib: "512", AttachToLegID: legID, AllowOvercommit: true})
	if err != nil {
		t.Fatalf("AddSlot(allowOvercommit): %v", err)
	}
	if !req.Legs[0].Overcommitted {
		t.Error("leg must be flagged Overcommitted after a knowing override")
	}

	// A loaded leg has physically left; nobody can join it.
	if _, err := m.LoadSlots(net.ID, req.ID, legID, LoadInput{}, "NCS"); err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	if _, err := m.AddSlot(net.ID, req.ID, SlotInput{Bib: "999", AttachToLegID: legID}); err == nil {
		t.Error("AddSlot to a loaded leg must be refused — the van has already left")
	}
}

func TestAddSlot_WithoutAttachStillNeedsAVehicle(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	van := sagCheckIn(t, netMgr, net.ID, "sag1")

	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: LocCourse},
		Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots:   []SlotInput{{Bib: "334"}},
	}, "NCS")
	req, _ = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: van.ID}, "NCS")
	if _, err := m.LoadSlots(net.ID, req.ID, req.Legs[0].ID, LoadInput{}, "NCS"); err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}

	req, err := m.AddSlot(net.ID, req.ID, SlotInput{Bib: "512"})
	if err != nil {
		t.Fatalf("AddSlot: %v", err)
	}
	if !req.NeedsVehicle || UnassignedRiders(*req) != 1 {
		t.Errorf("NeedsVehicle=%v unassigned=%d, want true/1 — a rider added after the van loaded is standing on the roadside",
			req.NeedsVehicle, UnassignedRiders(*req))
	}
	if req.Status != ReqPartial {
		t.Errorf("Status = %q, want %q", req.Status, ReqPartial)
	}
}

// --- defect 2: "there's no way to indicate the load includes the bike" ---
//
// Bike disposition is decided by the driver at LOAD time, not by the caller
// at request time, and it is a separate axis from what happens to the rider.

func TestLoadSlots_RecordsBikeDisposition(t *testing.T) {
	m, netMgr, _, db := newTestManager(t)
	net := createTestNet(t, netMgr)
	van := sagCheckIn(t, netMgr, net.ID, "sag1")

	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: LocCourse},
		Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots:   []SlotInput{{Bib: "334"}, {Bib: "335"}},
	}, "NCS")
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: van.ID}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID
	a, b := req.Slots[0].ID, req.Slots[1].ID

	// Both bikes were assumed at request time; on scene the driver reports
	// one bike on the rack and one frame too bent to carry.
	req, err = m.LoadSlots(net.ID, req.ID, legID, LoadInput{
		SlotIDs: []string{a, b},
		Bike:    map[string]string{a: BikeWithRider, b: BikeLeftBehind},
	}, "NCS")
	if err != nil {
		t.Fatalf("LoadSlots: %v", err)
	}
	for _, s := range req.Slots {
		switch s.ID {
		case a:
			if s.Bike != BikeWithRider || !s.HasBike {
				t.Errorf("slot a bike = %q/%v, want with_rider/true", s.Bike, s.HasBike)
			}
		case b:
			if s.Bike != BikeLeftBehind || s.HasBike {
				t.Errorf("slot b bike = %q/%v, want left_behind/false", s.Bike, s.HasBike)
			}
		}
	}

	st, _ := m.VehicleStatus(net.ID, van.ID)
	if st.CommittedSeats != 2 || st.CommittedRacks != 1 {
		t.Errorf("committed = (%d, %d), want (2 seats, 1 rack) — the abandoned bike must free its rack",
			st.CommittedSeats, st.CommittedRacks)
	}

	// The timeline has to say what happened to the bikes: a bike left at a
	// rest stop is somebody's property and an ICS-214 line item.
	evs, _ := db.LoadNetEvents(net.ID)
	found := false
	for _, e := range evs {
		if e.Type == TLSAGLoaded && strings.Contains(e.Summary, "1 bike left behind") {
			found = true
		}
	}
	if !found {
		t.Error("expected the load timeline row to name the bike left behind")
	}
}

func TestLoadSlots_BikeValidation(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	van := sagCheckIn(t, netMgr, net.ID, "sag1")

	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: LocCourse},
		Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots:   []SlotInput{{Bib: "334"}},
	}, "NCS")
	req, _ = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: van.ID}, "NCS")
	legID := req.Legs[0].ID
	a := req.Slots[0].ID

	if _, err := m.LoadSlots(net.ID, req.ID, legID, LoadInput{Bike: map[string]string{a: "in the trunk"}}, "NCS"); err == nil {
		t.Error("an unknown bike disposition must be refused, not silently stored")
	}
	if _, err := m.LoadSlots(net.ID, req.ID, legID, LoadInput{Bike: map[string]string{"not-a-slot": BikeNone}}, "NCS"); err == nil {
		t.Error("a bike disposition for a slot that is not being loaded must be refused")
	}
}

func TestCreateAndUpdateSlot_BikeDisposition(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)

	req, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: LocCourse},
		Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots:   []SlotInput{{Bib: "334"}, {Bib: "335", Bike: BikeNone}},
	}, "NCS")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if req.Slots[0].Bike != BikeWithRider {
		t.Errorf("default bike = %q, want %q", req.Slots[0].Bike, BikeWithRider)
	}
	if req.Slots[1].Bike != BikeNone || req.Slots[1].HasBike {
		t.Errorf("slot 1 bike = %q/%v, want none/false", req.Slots[1].Bike, req.Slots[1].HasBike)
	}

	// The rider's bike went with a different vehicle — rider and bike part ways.
	req, err = m.UpdateSlot(net.ID, req.ID, req.Slots[0].ID, SlotInput{Bib: "334", Bike: BikeOtherVehicle})
	if err != nil {
		t.Fatalf("UpdateSlot: %v", err)
	}
	if req.Slots[0].Bike != BikeOtherVehicle || req.Slots[0].HasBike {
		t.Errorf("slot bike = %q/%v, want other_vehicle/false", req.Slots[0].Bike, req.Slots[0].HasBike)
	}
	if _, err := m.UpdateSlot(net.ID, req.ID, req.Slots[0].ID, SlotInput{Bike: "nonsense"}); err == nil {
		t.Error("UpdateSlot must refuse an unknown bike disposition")
	}
}

// Slots written before the bike axis existed carry only hasBike. Loading
// them must produce an explicit disposition, never an empty string the UI
// would have to guess at.
func TestLoad_NormalizesLegacyBikeFlag(t *testing.T) {
	m, netMgr, _, db := newTestManager(t)
	net := createTestNet(t, netMgr)

	legacy := store.SAGRequest{
		ID: "legacy-1", NetID: net.ID, Sequence: 1,
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Priority: PriorityHigh, Status: ReqOpen, NeedsVehicle: true,
		Slots: []store.SAGSlot{
			{ID: "s1", Bib: "334", HasBike: true, Disposition: SlotWaiting},
			{ID: "s2", Bib: "335", HasBike: false, Disposition: SlotWaiting},
		},
		Legs: []store.SAGLeg{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := db.SaveSAGRequest(legacy); err != nil {
		t.Fatalf("SaveSAGRequest: %v", err)
	}

	m2 := NewManager(db, netMgr, nil, DefaultConfig())
	if err := m2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := m2.GetRequest(net.ID, "legacy-1")
	if !ok {
		t.Fatal("legacy request not loaded")
	}
	if got.Slots[0].Bike != BikeWithRider {
		t.Errorf("legacy hasBike=true -> %q, want %q", got.Slots[0].Bike, BikeWithRider)
	}
	if got.Slots[1].Bike != BikeNone {
		t.Errorf("legacy hasBike=false -> %q, want %q", got.Slots[1].Bike, BikeNone)
	}
	_ = m
}

// --- asymmetric capacity: seats are a legal limit, racks are a judgement call ---
//
// A seat is a seatbelt. Exceeding it is refused outright and AllowOvercommit
// does not open it. A bike can ride in the bed of a truck, so exceeding the
// rack count is a question the operator is allowed to answer yes to.

func TestDispatch_SeatsAreAHardStop(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	van := sagCheckIn(t, netMgr, net.ID, "sag1")
	// Three belts, plenty of rack space: only the seat count can bite.
	if _, err := m.SetVehicle(net.ID, van.ID, 3, 9, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "A"}, {Bib: "B"}, {Bib: "C"}, {Bib: "D"}},
	}, "NCS")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	all := []string{req.Slots[0].ID, req.Slots[1].ID, req.Slots[2].ID, req.Slots[3].ID}

	for _, allow := range []bool{false, true} {
		drainEvents(m.Events())
		_, err := m.Dispatch(net.ID, req.ID, DispatchInput{
			VehicleCheckInID: van.ID, SlotIDs: all, AllowOvercommit: allow,
		}, "NCS")
		if !errors.Is(err, ErrSeatsExceeded) {
			t.Fatalf("AllowOvercommit=%v: err = %v, want ErrSeatsExceeded", allow, err)
		}
		// Still an over-capacity error for every existing handler.
		if !errors.Is(err, ErrOverCapacity) {
			t.Errorf("AllowOvercommit=%v: err = %v, want errors.Is ErrOverCapacity too", allow, err)
		}
		var capErr *OverCapacityError
		if !errors.As(err, &capErr) {
			t.Fatalf("err is not an *OverCapacityError: %v", err)
		}
		if !capErr.SeatsExceeded || capErr.RacksExceeded {
			t.Errorf("seatsExceeded/racksExceeded = %v/%v, want true/false", capErr.SeatsExceeded, capErr.RacksExceeded)
		}
		if capErr.Seats != 3 || capErr.CommittedSeats != 0 || capErr.NeedSeats != 4 {
			t.Errorf("seat numbers = seats %d committed %d need %d, want 3/0/4",
				capErr.Seats, capErr.CommittedSeats, capErr.NeedSeats)
		}
		got, _ := m.GetRequest(net.ID, req.ID)
		if len(got.Legs) != 0 {
			t.Errorf("AllowOvercommit=%v: a leg was created despite the refusal", allow)
		}
		select {
		case evt := <-m.Events():
			t.Errorf("AllowOvercommit=%v: unexpected event on refusal: %+v", allow, evt)
		default:
		}
	}

	// Exactly at the limit is fine — the refusal is "more than", not "at".
	if _, err := m.Dispatch(net.ID, req.ID, DispatchInput{
		VehicleCheckInID: van.ID, SlotIDs: all[:3],
	}, "NCS"); err != nil {
		t.Fatalf("Dispatch of exactly 3 into 3 seats: %v", err)
	}
}

func TestDispatch_RacksStayOverridable(t *testing.T) {
	m, netMgr, _, db := newTestManager(t)
	net := createTestNet(t, netMgr)
	// Four belts, one rack: two riders with bikes fit the cab, not the rack.
	van := sagCheckIn(t, netMgr, net.ID, "sag1")
	if _, err := m.SetVehicle(net.ID, van.ID, 4, 1, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req, err := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "E"}, {Bib: "F"}},
	}, "NCS")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	both := []string{req.Slots[0].ID, req.Slots[1].ID}

	_, err = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: van.ID, SlotIDs: both}, "NCS")
	if !errors.Is(err, ErrOverCapacity) {
		t.Fatalf("err = %v, want ErrOverCapacity", err)
	}
	if errors.Is(err, ErrSeatsExceeded) {
		t.Fatal("a rack overflow must never present as a seat refusal")
	}
	var capErr *OverCapacityError
	if !errors.As(err, &capErr) || capErr.SeatsExceeded || !capErr.RacksExceeded {
		t.Fatalf("capErr = %+v, want racksExceeded only", capErr)
	}
	if capErr.RackSlots != 1 || capErr.NeedRacks != 2 {
		t.Errorf("rack numbers = slots %d need %d, want 1/2", capErr.RackSlots, capErr.NeedRacks)
	}

	// The operator says "toss it in the bed" and it goes through.
	req2, err := m.Dispatch(net.ID, req.ID, DispatchInput{
		VehicleCheckInID: van.ID, SlotIDs: both, AllowOvercommit: true,
	}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch with AllowOvercommit: %v", err)
	}
	if len(req2.Legs) != 1 || !req2.Legs[0].Overcommitted {
		t.Fatalf("Legs = %+v, want one leg flagged Overcommitted", req2.Legs)
	}

	// Overcommitted means BIKES, and the timeline has to say which.
	evs, _ := db.LoadNetEvents(net.ID)
	found := false
	for _, e := range evs {
		if e.Type == TLSAGDispatched && strings.Contains(strings.ToLower(e.Summary), "rack") {
			found = true
		}
	}
	if !found {
		t.Error("the dispatch timeline row must name the rack overflow, not say 'over capacity'")
	}

	// A seat is never negative, because seats can never be overcommitted.
	st, _ := m.VehicleStatus(net.ID, van.ID)
	if st.AvailableSeats < 0 {
		t.Errorf("AvailableSeats = %d; seats can no longer go negative", st.AvailableSeats)
	}
	if st.AvailableRacks >= 0 {
		t.Errorf("AvailableRacks = %d, want negative after a knowing rack overcommit", st.AvailableRacks)
	}
}

func TestAddSlot_AttachSeatsAreAHardStop(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	van := sagCheckIn(t, netMgr, net.ID, "sag1")
	// One belt, nine racks.
	if _, err := m.SetVehicle(net.ID, van.ID, 1, 9, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "334"}},
	}, "NCS")
	req, err := m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: van.ID}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID

	for _, allow := range []bool{false, true} {
		_, err := m.AddSlot(net.ID, req.ID, SlotInput{Bib: "512", AttachToLegID: legID, AllowOvercommit: allow})
		if !errors.Is(err, ErrSeatsExceeded) {
			t.Fatalf("AllowOvercommit=%v: err = %v, want ErrSeatsExceeded", allow, err)
		}
		got, _ := m.GetRequest(net.ID, req.ID)
		if len(got.Slots) != 1 {
			t.Fatalf("AllowOvercommit=%v: the rider was added despite the seat refusal", allow)
		}
		if len(got.Legs[0].SlotIDs) != 1 {
			t.Errorf("AllowOvercommit=%v: the leg grew despite the seat refusal", allow)
		}
	}

	// Leaving them unassigned is always allowed — that is the fallback the
	// refusal is supposed to push the operator toward.
	got, err := m.AddSlot(net.ID, req.ID, SlotInput{Bib: "512"})
	if err != nil {
		t.Fatalf("AddSlot unassigned: %v", err)
	}
	if !got.NeedsVehicle || UnassignedRiders(*got) != 1 {
		t.Errorf("unassigned rider not recorded: needsVehicle=%v unassigned=%d", got.NeedsVehicle, UnassignedRiders(*got))
	}
}

func TestAddSlot_AttachRacksStayOverridable(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)
	van := sagCheckIn(t, netMgr, net.ID, "sag1")
	// Room for the person, no room for their bike.
	if _, err := m.SetVehicle(net.ID, van.ID, 4, 1, ""); err != nil {
		t.Fatalf("SetVehicle: %v", err)
	}

	req, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "334"}},
	}, "NCS")
	req, _ = m.Dispatch(net.ID, req.ID, DispatchInput{VehicleCheckInID: van.ID}, "NCS")
	legID := req.Legs[0].ID

	_, err := m.AddSlot(net.ID, req.ID, SlotInput{Bib: "512", AttachToLegID: legID})
	var capErr *OverCapacityError
	if !errors.As(err, &capErr) || capErr.SeatsExceeded || !capErr.RacksExceeded {
		t.Fatalf("err = %v (capErr %+v), want a racks-only overflow", err, capErr)
	}

	req, err = m.AddSlot(net.ID, req.ID, SlotInput{Bib: "512", AttachToLegID: legID, AllowOvercommit: true})
	if err != nil {
		t.Fatalf("AddSlot(allowOvercommit): %v", err)
	}
	if !req.Legs[0].Overcommitted {
		t.Error("leg must be flagged Overcommitted after a knowing rack override")
	}

	// The same rider WITHOUT a bike needs no override at all: the rack is the
	// only thing that was short.
	req2, _ := m.CreateRequest(net.ID, CreateRequestInput{
		Pickup: store.SAGLocation{Kind: LocCourse}, Dropoff: store.SAGLocation{Kind: LocNextRestStop},
		Slots: []SlotInput{{Bib: "700"}},
	}, "NCS")
	req2, _ = m.Dispatch(net.ID, req2.ID, DispatchInput{VehicleCheckInID: van.ID, AllowOvercommit: true}, "NCS")
	if _, err := m.AddSlot(net.ID, req2.ID, SlotInput{Bib: "701", Bike: BikeNone, AttachToLegID: req2.Legs[0].ID}); err != nil {
		t.Fatalf("AddSlot of a bike-less rider with a free seat: %v", err)
	}
}

// A station promoted to the sag category mid-net must reach every open SAG
// board immediately. Before this, sag_vehicle_updated was emitted only from
// SetVehicle and for vehicles already carrying a leg, so flipping a roster
// station to SAG left the dispatch picker empty until a full page reload —
// the operator saw "No SAG vehicles on the roster" with a SAG unit checked in.
func TestAnnounceVehicle_OnCategoryChange(t *testing.T) {
	m, netMgr, _, _ := newTestManager(t)
	net := createTestNet(t, netMgr)

	// Checked in as something else: not a vehicle, nothing to announce.
	ci, err := netMgr.CheckIn(net.ID, "KG4YFA-4", netcontrol.TrafficNone, netcontrol.CatMedical)
	if err != nil {
		t.Fatalf("CheckIn: %v", err)
	}
	drainEvents(m.Events())

	if ok := m.AnnounceVehicle(net.ID, ci.ID); ok {
		t.Error("AnnounceVehicle on a non-sag check-in = true, want false")
	}
	if got := findVehicleEvent(m, ci.ID); got != nil {
		t.Error("announced a vehicle for a non-sag check-in")
	}

	// Promote it in the roster, exactly as the category dropdown does.
	ci.Category = netcontrol.CatSAG
	if _, err := netMgr.UpdateCheckIn(*ci); err != nil {
		t.Fatalf("UpdateCheckIn: %v", err)
	}
	drainEvents(m.Events())

	if ok := m.AnnounceVehicle(net.ID, ci.ID); !ok {
		t.Fatal("AnnounceVehicle on a sag check-in = false, want true")
	}
	got := findVehicleEvent(m, ci.ID)
	if got == nil {
		t.Fatal("no sag_vehicle_updated emitted for the promoted check-in")
	}
	if got.Callsign != "KG4YFA-4" {
		t.Errorf("callsign = %q, want KG4YFA-4", got.Callsign)
	}
	// Unregistered vehicles must announce the configured defaults, not zeroes:
	// a vehicle advertising 0 seats is refused every dispatch.
	if got.Seats != m.cfg.DefaultSeats || got.RackSlots != m.cfg.DefaultRackSlots {
		t.Errorf("seats/racks = %d/%d, want the config defaults %d/%d",
			got.Seats, got.RackSlots, m.cfg.DefaultSeats, m.cfg.DefaultRackSlots)
	}
	if got.AvailableSeats != m.cfg.DefaultSeats {
		t.Errorf("availableSeats = %d, want %d", got.AvailableSeats, m.cfg.DefaultSeats)
	}

	// A check-in that does not exist is a no-op, never a panic.
	if ok := m.AnnounceVehicle(net.ID, "nope"); ok {
		t.Error("AnnounceVehicle for an unknown check-in = true, want false")
	}
}

// findVehicleEvent returns the first sag_vehicle_updated for checkInID.
func findVehicleEvent(m *Manager, checkInID string) *SAGVehicleStatus {
	for {
		select {
		case evt := <-m.Events():
			if evt.Type != EventSAGVehicleUpdated {
				continue
			}
			if v, ok := evt.Data.(SAGVehicleStatus); ok && v.CheckInID == checkInID {
				return &v
			}
		default:
			return nil
		}
	}
}
