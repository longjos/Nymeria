package ride

import (
	"testing"

	"github.com/narvel/nymeria/internal/store"
)

func slot(id, disposition, legID string) store.SAGSlot {
	return store.SAGSlot{ID: id, Disposition: disposition, LegID: legID}
}

func slotBike(id, disposition, legID string, hasBike bool) store.SAGSlot {
	return store.SAGSlot{ID: id, Disposition: disposition, LegID: legID, HasBike: hasBike}
}

func leg(id, status string, slotIDs ...string) store.SAGLeg {
	return store.SAGLeg{ID: id, Status: status, SlotIDs: slotIDs}
}

func TestDeriveStatus(t *testing.T) {
	tests := []struct {
		name             string
		slots            []store.SAGSlot
		legs             []store.SAGLeg
		cancelReason     string
		wantStatus       string
		wantNeedsVehicle bool
	}{
		{
			name:             "no slots taken, no legs",
			slots:            []store.SAGSlot{slot("a", SlotWaiting, "")},
			wantStatus:       ReqOpen,
			wantNeedsVehicle: true,
		},
		{
			name:             "leg dispatched covering all",
			slots:            []store.SAGSlot{slot("a", SlotWaiting, "leg1")},
			legs:             []store.SAGLeg{leg("leg1", LegDispatched, "a")},
			wantStatus:       ReqAssigned,
			wantNeedsVehicle: false,
		},
		{
			name:             "leg enroute",
			slots:            []store.SAGSlot{slot("a", SlotWaiting, "leg1")},
			legs:             []store.SAGLeg{leg("leg1", LegEnroute, "a")},
			wantStatus:       ReqEnroute,
			wantNeedsVehicle: false,
		},
		{
			name:  "two legs dispatched+onscene",
			slots: []store.SAGSlot{slot("a", SlotWaiting, "leg1"), slot("b", SlotWaiting, "leg2")},
			legs: []store.SAGLeg{
				leg("leg1", LegDispatched, "a"),
				leg("leg2", LegOnScene, "b"),
			},
			wantStatus:       ReqOnScene,
			wantNeedsVehicle: false,
		},
		{
			name:             "one loaded, none waiting",
			slots:            []store.SAGSlot{slot("a", SlotLoaded, "leg1")},
			legs:             []store.SAGLeg{leg("leg1", LegLoaded, "a")},
			wantStatus:       ReqTransporting,
			wantNeedsVehicle: false,
		},
		{
			name:             "one loaded, one waiting unlegged",
			slots:            []store.SAGSlot{slot("a", SlotLoaded, "leg1"), slot("b", SlotWaiting, "")},
			legs:             []store.SAGLeg{leg("leg1", LegLoaded, "a")},
			wantStatus:       ReqPartial,
			wantNeedsVehicle: true,
		},
		{
			name:             "one delivered, one waiting unlegged, no active leg",
			slots:            []store.SAGSlot{slot("a", SlotDelivered, "leg1"), slot("b", SlotWaiting, "")},
			legs:             []store.SAGLeg{leg("leg1", LegDelivered, "a")},
			wantStatus:       ReqPartial,
			wantNeedsVehicle: true,
		},
		{
			name:  "one delivered, one waiting on dispatched leg2",
			slots: []store.SAGSlot{slot("a", SlotDelivered, "leg1"), slot("b", SlotWaiting, "leg2")},
			legs: []store.SAGLeg{
				leg("leg1", LegDelivered, "a"),
				leg("leg2", LegDispatched, "b"),
			},
			wantStatus:       ReqAssigned,
			wantNeedsVehicle: false,
		},
		{
			name:  "one loaded on leg1, one waiting on dispatched leg2",
			slots: []store.SAGSlot{slot("a", SlotLoaded, "leg1"), slot("b", SlotWaiting, "leg2")},
			legs: []store.SAGLeg{
				leg("leg1", LegLoaded, "a"),
				leg("leg2", LegDispatched, "b"),
			},
			wantStatus:       ReqTransporting,
			wantNeedsVehicle: false,
		},
		{
			name:             "all delivered",
			slots:            []store.SAGSlot{slot("a", SlotDelivered, "leg1"), slot("b", SlotDelivered, "leg1")},
			legs:             []store.SAGLeg{leg("leg1", LegDelivered, "a", "b")},
			wantStatus:       ReqComplete,
			wantNeedsVehicle: false,
		},
		{
			name:             "all self_resolved",
			slots:            []store.SAGSlot{slot("a", SlotSelfResolved, ""), slot("b", SlotSelfResolved, "")},
			wantStatus:       ReqComplete,
			wantNeedsVehicle: false,
		},
		{
			name:             "mixed delivered+not_found+declined",
			slots:            []store.SAGSlot{slot("a", SlotDelivered, "leg1"), slot("b", SlotNotFound, ""), slot("c", SlotDeclined, "")},
			legs:             []store.SAGLeg{leg("leg1", LegDelivered, "a")},
			wantStatus:       ReqComplete,
			wantNeedsVehicle: false,
		},
		{
			name:             "cancelReason set with waiting cancelled slots",
			slots:            []store.SAGSlot{slot("a", SlotCancelled, "")},
			cancelReason:     "rider declined",
			wantStatus:       ReqCancelled,
			wantNeedsVehicle: false,
		},
		{
			name:             "waiting slot added after all others delivered",
			slots:            []store.SAGSlot{slot("a", SlotDelivered, "leg1"), slot("b", SlotWaiting, "")},
			legs:             []store.SAGLeg{leg("leg1", LegDelivered, "a")},
			wantStatus:       ReqPartial,
			wantNeedsVehicle: true,
		},
		{
			name:             "transporting with extra waiting slot added",
			slots:            []store.SAGSlot{slot("a", SlotLoaded, "leg1"), slot("b", SlotWaiting, "")},
			legs:             []store.SAGLeg{leg("leg1", LegLoaded, "a")},
			wantStatus:       ReqPartial,
			wantNeedsVehicle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := store.SAGRequest{Slots: tt.slots, Legs: tt.legs, CancelReason: tt.cancelReason}
			status, needsVehicle := DeriveStatus(r)
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
			if needsVehicle != tt.wantNeedsVehicle {
				t.Errorf("needsVehicle = %v, want %v", needsVehicle, tt.wantNeedsVehicle)
			}
		})
	}
}

func TestLegCanTransition(t *testing.T) {
	statuses := []string{LegDispatched, LegEnroute, LegOnScene, LegLoaded, LegDelivered, LegReleased}
	want := map[string]map[string]bool{
		LegDispatched: {LegEnroute: true, LegOnScene: true, LegLoaded: true, LegReleased: true},
		LegEnroute:    {LegOnScene: true, LegLoaded: true, LegReleased: true},
		LegOnScene:    {LegLoaded: true, LegReleased: true},
		LegLoaded:     {LegDelivered: true},
		LegDelivered:  {},
		LegReleased:   {},
	}
	for _, from := range statuses {
		for _, to := range statuses {
			got := LegCanTransition(from, to)
			wantOK := want[from][to]
			if got != wantOK {
				t.Errorf("LegCanTransition(%q, %q) = %v, want %v", from, to, got, wantOK)
			}
		}
	}
}

func TestSlotCanResolve(t *testing.T) {
	allDispositions := []string{
		SlotWaiting, SlotLoaded, SlotDelivered, SlotSelfResolved,
		SlotDeclined, SlotNotFound, SlotHandedOff, SlotCancelled,
	}

	waitingOK := map[string]bool{
		SlotSelfResolved: true, SlotDeclined: true, SlotNotFound: true,
		SlotHandedOff: true, SlotCancelled: true,
	}
	for _, to := range allDispositions {
		got := SlotCanResolve(SlotWaiting, to)
		want := waitingOK[to]
		if got != want {
			t.Errorf("SlotCanResolve(waiting, %q) = %v, want %v", to, got, want)
		}
	}

	loadedOK := map[string]bool{SlotHandedOff: true}
	for _, to := range allDispositions {
		got := SlotCanResolve(SlotLoaded, to)
		want := loadedOK[to]
		if got != want {
			t.Errorf("SlotCanResolve(loaded, %q) = %v, want %v", to, got, want)
		}
	}

	terminal := []string{SlotDelivered, SlotSelfResolved, SlotDeclined, SlotNotFound, SlotHandedOff, SlotCancelled}
	for _, from := range terminal {
		for _, to := range allDispositions {
			if SlotCanResolve(from, to) {
				t.Errorf("SlotCanResolve(%q, %q) = true, want false (terminal)", from, to)
			}
		}
	}
}

func TestDefaultPriority(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		pickupKind string
		want       string
	}{
		{LocCourse, "high"},
		{LocRestStop, "medium"},
		{LocCheckpoint, "medium"},
		{LocStart, "medium"},
		{LocFinish, "medium"},
		{LocOther, "medium"},
		{"unknown", "medium"},
	}
	for _, tt := range tests {
		got := DefaultPriority(cfg, tt.pickupKind)
		if got != tt.want {
			t.Errorf("DefaultPriority(default, %q) = %q, want %q", tt.pickupKind, got, tt.want)
		}
	}

	custom := cfg
	custom.DefaultCoursePriority = "priority"
	if got := DefaultPriority(custom, LocCourse); got != "priority" {
		t.Errorf("DefaultPriority(custom, course) = %q, want %q", got, "priority")
	}
}

func TestCapacity(t *testing.T) {
	vehicle := store.SAGVehicle{CheckInID: "ci-1", Seats: 3, RackSlots: 2}

	tests := []struct {
		name      string
		reqs      []store.SAGRequest
		wantSeats int
		wantRacks int
	}{
		{
			name: "dispatched 2 bikes",
			reqs: []store.SAGRequest{{
				ID:    "r1",
				Slots: []store.SAGSlot{slotBike("a", SlotWaiting, "leg1", true), slotBike("b", SlotWaiting, "leg1", true)},
				Legs:  []store.SAGLeg{{ID: "leg1", VehicleCheckInID: "ci-1", Status: LegDispatched, SlotIDs: []string{"a", "b"}}},
			}},
			wantSeats: 2, wantRacks: 2,
		},
		{
			name: "loaded 1 nobike",
			reqs: []store.SAGRequest{{
				ID:    "r1",
				Slots: []store.SAGSlot{slotBike("a", SlotLoaded, "leg1", false)},
				Legs:  []store.SAGLeg{{ID: "leg1", VehicleCheckInID: "ci-1", Status: LegLoaded, SlotIDs: []string{"a"}}},
			}},
			wantSeats: 1, wantRacks: 0,
		},
		{
			name: "delivered 3",
			reqs: []store.SAGRequest{{
				ID:    "r1",
				Slots: []store.SAGSlot{slotBike("a", SlotDelivered, "leg1", true), slotBike("b", SlotDelivered, "leg1", true), slotBike("c", SlotDelivered, "leg1", true)},
				Legs:  []store.SAGLeg{{ID: "leg1", VehicleCheckInID: "ci-1", Status: LegDelivered, SlotIDs: []string{"a", "b", "c"}}},
			}},
			wantSeats: 0, wantRacks: 0,
		},
		{
			name: "released 3",
			reqs: []store.SAGRequest{{
				ID:    "r1",
				Slots: []store.SAGSlot{slot("a", SlotWaiting, ""), slot("b", SlotWaiting, ""), slot("c", SlotWaiting, "")},
				Legs:  []store.SAGLeg{{ID: "leg1", VehicleCheckInID: "ci-1", Status: LegReleased, SlotIDs: []string{"a", "b", "c"}}},
			}},
			wantSeats: 0, wantRacks: 0,
		},
		{
			name: "two legs on two requests 2+2",
			reqs: []store.SAGRequest{
				{
					ID:    "r1",
					Slots: []store.SAGSlot{slotBike("a", SlotWaiting, "leg1", true), slotBike("b", SlotWaiting, "leg1", true)},
					Legs:  []store.SAGLeg{{ID: "leg1", VehicleCheckInID: "ci-1", Status: LegDispatched, SlotIDs: []string{"a", "b"}}},
				},
				{
					ID:    "r2",
					Slots: []store.SAGSlot{slotBike("c", SlotWaiting, "leg2", true), slotBike("d", SlotWaiting, "leg2", true)},
					Legs:  []store.SAGLeg{{ID: "leg2", VehicleCheckInID: "ci-1", Status: LegDispatched, SlotIDs: []string{"c", "d"}}},
				},
			},
			wantSeats: 4, wantRacks: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSeats, gotRacks, _, _ := Capacity(vehicle, tt.reqs)
			if gotSeats != tt.wantSeats || gotRacks != tt.wantRacks {
				t.Errorf("Capacity() = (%d, %d), want (%d, %d)", gotSeats, gotRacks, tt.wantSeats, tt.wantRacks)
			}
		})
	}
}
