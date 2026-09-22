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

// --- bike disposition (a separate axis from rider disposition) ---

func TestSlotTakesRack(t *testing.T) {
	tests := []struct {
		name string
		slot store.SAGSlot
		want bool
	}{
		{"explicit with_rider", store.SAGSlot{Bike: BikeWithRider}, true},
		{"explicit none", store.SAGSlot{Bike: BikeNone, HasBike: true}, false},
		{"explicit left_behind", store.SAGSlot{Bike: BikeLeftBehind, HasBike: true}, false},
		{"explicit other_vehicle", store.SAGSlot{Bike: BikeOtherVehicle, HasBike: true}, false},
		{"legacy hasBike true", store.SAGSlot{HasBike: true}, true},
		{"legacy hasBike false", store.SAGSlot{HasBike: false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SlotTakesRack(tt.slot); got != tt.want {
				t.Errorf("SlotTakesRack(%+v) = %v, want %v", tt.slot, got, tt.want)
			}
		})
	}
}

func TestNormalizeSlotBike(t *testing.T) {
	tests := []struct {
		name        string
		in          store.SAGSlot
		wantBike    string
		wantHasBike bool
	}{
		{"legacy true becomes with_rider", store.SAGSlot{HasBike: true}, BikeWithRider, true},
		{"legacy false becomes none", store.SAGSlot{HasBike: false}, BikeNone, false},
		{"explicit left_behind clears hasBike", store.SAGSlot{Bike: BikeLeftBehind, HasBike: true}, BikeLeftBehind, false},
		{"explicit with_rider sets hasBike", store.SAGSlot{Bike: BikeWithRider}, BikeWithRider, true},
		{"unknown value falls back to legacy bool", store.SAGSlot{Bike: "wat", HasBike: true}, BikeWithRider, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.in
			NormalizeSlotBike(&s)
			if s.Bike != tt.wantBike || s.HasBike != tt.wantHasBike {
				t.Errorf("NormalizeSlotBike = (%q, %v), want (%q, %v)", s.Bike, s.HasBike, tt.wantBike, tt.wantHasBike)
			}
		})
	}
}

func TestCapacity_BikeDispositionFreesRack(t *testing.T) {
	vehicle := store.SAGVehicle{CheckInID: "ci-1", Seats: 3, RackSlots: 2}
	// Two riders aboard; one bike was left behind at the rest stop. The seat
	// is still taken, the rack is not.
	reqs := []store.SAGRequest{{
		ID: "r1",
		Slots: []store.SAGSlot{
			{ID: "a", Disposition: SlotLoaded, LegID: "leg1", Bike: BikeWithRider},
			{ID: "b", Disposition: SlotLoaded, LegID: "leg1", Bike: BikeLeftBehind},
		},
		Legs: []store.SAGLeg{{ID: "leg1", VehicleCheckInID: "ci-1", Status: LegLoaded, SlotIDs: []string{"a", "b"}}},
	}}
	seats, racks, _, _ := Capacity(vehicle, reqs)
	if seats != 2 || racks != 1 {
		t.Errorf("Capacity() = (%d, %d), want (2, 1)", seats, racks)
	}
}

// --- "who is waiting for a ride" ---

func TestUnassignedRiders(t *testing.T) {
	tests := []struct {
		name string
		req  store.SAGRequest
		want int
	}{
		{"fresh request, one rider", store.SAGRequest{Slots: []store.SAGSlot{slot("a", SlotWaiting, "")}}, 1},
		{
			name: "all reserved on a dispatched leg",
			req: store.SAGRequest{
				Slots: []store.SAGSlot{slot("a", SlotWaiting, "leg1"), slot("b", SlotWaiting, "leg1")},
				Legs:  []store.SAGLeg{leg("leg1", LegDispatched, "a", "b")},
			},
			want: 0,
		},
		{
			name: "rider added after the van left",
			req: store.SAGRequest{
				Slots: []store.SAGSlot{slot("a", SlotLoaded, "leg1"), slot("b", SlotWaiting, "")},
				Legs:  []store.SAGLeg{leg("leg1", LegLoaded, "a")},
			},
			want: 1,
		},
		{"everyone terminal", store.SAGRequest{Slots: []store.SAGSlot{slot("a", SlotDelivered, "leg1")}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UnassignedRiders(tt.req); got != tt.want {
				t.Errorf("UnassignedRiders() = %d, want %d", got, tt.want)
			}
			// needsVehicle must never disagree with the count it summarises.
			_, needs := DeriveStatus(tt.req)
			if needs != (tt.want > 0) {
				t.Errorf("DeriveStatus needsVehicle = %v, UnassignedRiders = %d", needs, tt.want)
			}
		})
	}
}

// --- "make it two riders" — the leg a late rider can still join ---

func TestActiveUnloadedLegs(t *testing.T) {
	tests := []struct {
		name string
		req  store.SAGRequest
		want []string
	}{
		{
			name: "a van still driving to the scene can take one more",
			req:  store.SAGRequest{Legs: []store.SAGLeg{leg("leg1", LegEnroute, "a")}},
			want: []string{"leg1"},
		},
		{
			name: "on scene still counts — the door is open",
			req:  store.SAGRequest{Legs: []store.SAGLeg{leg("leg1", LegOnScene, "a")}},
			want: []string{"leg1"},
		},
		{
			name: "a loaded van has left; nobody can join it",
			req:  store.SAGRequest{Legs: []store.SAGLeg{leg("leg1", LegLoaded, "a")}},
			want: []string{},
		},
		{
			name: "released and delivered legs never qualify",
			req:  store.SAGRequest{Legs: []store.SAGLeg{leg("leg1", LegReleased, "a"), leg("leg2", LegDelivered, "b")}},
			want: []string{},
		},
		{
			name: "two live legs, both offered",
			req:  store.SAGRequest{Legs: []store.SAGLeg{leg("leg1", LegDispatched, "a"), leg("leg2", LegOnScene, "b")}},
			want: []string{"leg1", "leg2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ActiveUnloadedLegs(tt.req)
			if len(got) != len(tt.want) {
				t.Fatalf("ActiveUnloadedLegs() = %v, want %v", got, tt.want)
			}
			for i, l := range got {
				if l.ID != tt.want[i] {
					t.Errorf("ActiveUnloadedLegs()[%d] = %q, want %q", i, l.ID, tt.want[i])
				}
			}
		})
	}
}

// The composer's kind pickers are built from these, so their ORDER is
// operator-facing: a list that reorders itself between two openings of the
// same dialog cannot be used at radio speed. Map iteration is random in Go,
// so the ordered slices are the source of truth and the validity sets are
// derived from them.
func TestKindOrderIsTheSourceOfTruth(t *testing.T) {
	if len(PickupKindOrder) != len(ValidPickupKinds) {
		t.Errorf("PickupKindOrder has %d entries, ValidPickupKinds has %d", len(PickupKindOrder), len(ValidPickupKinds))
	}
	for _, k := range PickupKindOrder {
		if !ValidPickupKinds[k] {
			t.Errorf("PickupKindOrder has %q, which is not a valid pickup kind", k)
		}
	}
	if len(DropoffKindOrder) != len(ValidDropoffKinds) {
		t.Errorf("DropoffKindOrder has %d entries, ValidDropoffKinds has %d", len(DropoffKindOrder), len(ValidDropoffKinds))
	}
	for _, k := range DropoffKindOrder {
		if !ValidDropoffKinds[k] {
			t.Errorf("DropoffKindOrder has %q, which is not a valid dropoff kind", k)
		}
	}
	// The first entry is the composer's default. A SAG call almost always
	// comes from a rider on the course, and that default is what makes the
	// HIGH priority default fire (DefaultPriority).
	if PickupKindOrder[0] != LocCourse {
		t.Errorf("PickupKindOrder[0] = %q, want %q", PickupKindOrder[0], LocCourse)
	}
	if DropoffKindOrder[0] != LocNextRestStop {
		t.Errorf("DropoffKindOrder[0] = %q, want %q", DropoffKindOrder[0], LocNextRestStop)
	}
}

// The load control offers these in order. with_rider is the default and the
// common answer; other_vehicle (shuttle / bike-rack truck) is real on very
// large rides but rare, so it must sort LAST and must never be the default
// or the first alternative an operator lands on.
func TestBikeDispositionOrder(t *testing.T) {
	if len(BikeDispositionOrder) != len(ValidBikeDispositions) {
		t.Fatalf("BikeDispositionOrder has %d entries, ValidBikeDispositions has %d",
			len(BikeDispositionOrder), len(ValidBikeDispositions))
	}
	seen := map[string]bool{}
	for _, b := range BikeDispositionOrder {
		if !ValidBikeDispositions[b] {
			t.Errorf("BikeDispositionOrder has %q, which is not a valid disposition", b)
		}
		if seen[b] {
			t.Errorf("BikeDispositionOrder repeats %q", b)
		}
		seen[b] = true
	}
	if BikeDispositionOrder[0] != BikeWithRider {
		t.Errorf("BikeDispositionOrder[0] = %q, want %q (the default)", BikeDispositionOrder[0], BikeWithRider)
	}
	last := BikeDispositionOrder[len(BikeDispositionOrder)-1]
	if last != BikeOtherVehicle {
		t.Errorf("BikeDispositionOrder last = %q, want %q (rare, never first alternative)", last, BikeOtherVehicle)
	}
	if BikeDispositionOrder[1] == BikeOtherVehicle {
		t.Error("other_vehicle must not be the first alternative")
	}
}
