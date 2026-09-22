package ride

import "github.com/narvel/nymeria/internal/store"

// terminalSlotDispositions are the dispositions a slot never leaves.
var terminalSlotDispositions = map[string]bool{
	SlotDelivered:    true,
	SlotSelfResolved: true,
	SlotDeclined:     true,
	SlotNotFound:     true,
	SlotHandedOff:    true,
	SlotCancelled:    true,
}

func isTerminalDisposition(d string) bool {
	return terminalSlotDispositions[d]
}

// activeLegStatuses are the leg statuses that still represent a vehicle
// committed to a job (dispatched all the way through loaded, but not yet
// delivered or released).
var activeLegStatuses = map[string]bool{
	LegDispatched: true,
	LegEnroute:    true,
	LegOnScene:    true,
	LegLoaded:     true,
}

// legRank orders the "not yet loaded" leg statuses by how far along they
// are, most advanced first — used by DeriveStatus rule 5.
var legRank = map[string]int{
	LegOnScene:    3,
	LegEnroute:    2,
	LegDispatched: 1,
}

// DeriveStatus computes a SAGRequest's status and needsVehicle flag purely
// from its Slots and Legs. Evaluated in this order, first match wins:
//
//  1. CancelReason != ""                                       -> cancelled
//  2. every slot terminal                                      -> complete
//  3. a waiting+unlegged slot AND a loaded|delivered slot exist -> partial
//  4. a loaded slot exists                                     -> transporting
//  5. an active (not-yet-loaded) leg exists; most advanced wins -> onscene | enroute | assigned
//  6. otherwise (waiting slots, no active leg)                 -> open
//
// needsVehicle is independent of status: it is true whenever any slot is
// still waiting with no leg assigned, even under transporting (a rider
// added after the vehicle left still needs a ride).
func DeriveStatus(r store.SAGRequest) (status string, needsVehicle bool) {
	for _, s := range r.Slots {
		if s.Disposition == SlotWaiting && s.LegID == "" {
			needsVehicle = true
			break
		}
	}

	if r.CancelReason != "" {
		return ReqCancelled, needsVehicle
	}

	allTerminal := len(r.Slots) > 0
	for _, s := range r.Slots {
		if !isTerminalDisposition(s.Disposition) {
			allTerminal = false
			break
		}
	}
	if allTerminal {
		return ReqComplete, needsVehicle
	}

	hasUnlegWaiting := false
	hasLoadedOrDelivered := false
	hasLoaded := false
	for _, s := range r.Slots {
		if s.Disposition == SlotWaiting && s.LegID == "" {
			hasUnlegWaiting = true
		}
		if s.Disposition == SlotLoaded || s.Disposition == SlotDelivered {
			hasLoadedOrDelivered = true
		}
		if s.Disposition == SlotLoaded {
			hasLoaded = true
		}
	}
	if hasUnlegWaiting && hasLoadedOrDelivered {
		return ReqPartial, needsVehicle
	}
	if hasLoaded {
		return ReqTransporting, needsVehicle
	}

	best := 0
	bestStatus := ""
	for _, leg := range r.Legs {
		if rank, ok := legRank[leg.Status]; ok && rank > best {
			best = rank
			bestStatus = leg.Status
		}
	}
	switch bestStatus {
	case LegOnScene:
		return ReqOnScene, needsVehicle
	case LegEnroute:
		return ReqEnroute, needsVehicle
	case LegDispatched:
		return ReqAssigned, needsVehicle
	}

	return ReqOpen, needsVehicle
}

// legTransitions is the forward-only leg ladder. Forward skips are allowed
// (radio reality: "SAG 2 has both riders" goes dispatched->loaded directly);
// backward and same->same are never allowed. loaded->delivered is reachable
// only through the dedicated DeliverSlots/ResolveSlot(handed_off) verbs, not
// through AdvanceLeg, but the transition itself is still "OK" here — it is
// AdvanceLeg's caller-side restriction (enroute|onscene only) that keeps the
// side effects from being skipped, not this table.
var legTransitions = map[string]map[string]bool{
	LegDispatched: {LegEnroute: true, LegOnScene: true, LegLoaded: true, LegReleased: true},
	LegEnroute:    {LegOnScene: true, LegLoaded: true, LegReleased: true},
	LegOnScene:    {LegLoaded: true, LegReleased: true},
	LegLoaded:     {LegDelivered: true},
	LegDelivered:  {},
	LegReleased:   {},
}

// LegCanTransition reports whether a leg may move from one status to
// another. from==to is never allowed.
func LegCanTransition(from, to string) bool {
	if from == to {
		return false
	}
	allowed, ok := legTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

// slotResolutions is what ResolveSlot may transition a slot to, keyed by its
// current disposition. waiting can resolve to any of the five exception
// outcomes; a loaded slot (already aboard) can only be handed off to
// medical — it cannot self-resolve, decline, go not-found, or be cancelled,
// and it is never "delivered" through ResolveSlot (that is DeliverSlots'
// job). Every terminal disposition resolves to nothing further.
var slotResolutions = map[string]map[string]bool{
	SlotWaiting: {
		SlotSelfResolved: true,
		SlotDeclined:     true,
		SlotNotFound:     true,
		SlotHandedOff:    true,
		SlotCancelled:    true,
	},
	SlotLoaded: {
		SlotHandedOff: true,
	},
}

// SlotCanResolve reports whether ResolveSlot may move a slot from its
// current disposition to the given one.
func SlotCanResolve(currentDisposition, to string) bool {
	allowed, ok := slotResolutions[currentDisposition]
	if !ok {
		return false
	}
	return allowed[to]
}

// DefaultPriority derives the priority for a new request from its pickup
// kind alone: transport requested FROM THE COURSE is high, from anywhere
// else (a rest stop, checkpoint, start or finish) is medium.
func DefaultPriority(cfg Config, pickupKind string) string {
	if pickupKind == LocCourse {
		return cfg.DefaultCoursePriority
	}
	return cfg.DefaultStopPriority
}

// Capacity derives a vehicle's currently committed seats/racks and its
// active legs/requests from the full set of requests in its net. Capacity
// is never stored — it is recomputed from live leg/slot state every time.
// A slot only counts while it is actually occupying the vehicle: waiting
// (reserved, assigned to the leg but not yet picked up) or loaded (aboard);
// once delivered or handed off it frees its seat immediately, even if the
// leg it rode on has not yet been marked delivered (partial delivery).
func Capacity(v store.SAGVehicle, reqs []store.SAGRequest) (committedSeats, committedRacks int, activeLegIDs, activeRequestIDs []string) {
	activeLegIDs = []string{}
	activeRequestIDs = []string{}

	for _, req := range reqs {
		slotByID := make(map[string]store.SAGSlot, len(req.Slots))
		for _, s := range req.Slots {
			slotByID[s.ID] = s
		}
		reqIsActive := false
		for _, leg := range req.Legs {
			if leg.VehicleCheckInID != v.CheckInID || !activeLegStatuses[leg.Status] {
				continue
			}
			activeLegIDs = append(activeLegIDs, leg.ID)
			reqIsActive = true
			for _, sid := range leg.SlotIDs {
				slot, ok := slotByID[sid]
				if !ok {
					continue
				}
				if slot.Disposition != SlotWaiting && slot.Disposition != SlotLoaded {
					continue
				}
				committedSeats++
				if slot.HasBike {
					committedRacks++
				}
			}
		}
		if reqIsActive {
			activeRequestIDs = append(activeRequestIDs, req.ID)
		}
	}
	return
}

// normalize turns nil Slots/Legs/SlotIDs into empty slices in place. Called
// on both create and load, per the project's nil-slice-freezes-the-UI rule.
func normalize(r *store.SAGRequest) {
	if r == nil {
		return
	}
	if r.Slots == nil {
		r.Slots = []store.SAGSlot{}
	}
	if r.Legs == nil {
		r.Legs = []store.SAGLeg{}
	}
	for i := range r.Legs {
		if r.Legs[i].SlotIDs == nil {
			r.Legs[i].SlotIDs = []string{}
		}
	}
}

// deepCopyRequest returns a copy of r whose Slots, Legs and each leg's
// SlotIDs are independent backing arrays, safe to mutate without aliasing
// whatever the manager still has cached.
func deepCopyRequest(r store.SAGRequest) store.SAGRequest {
	out := r
	if r.Division != nil {
		d := *r.Division
		out.Division = &d
	}
	out.Slots = append([]store.SAGSlot(nil), r.Slots...)
	out.Legs = make([]store.SAGLeg, len(r.Legs))
	for i, leg := range r.Legs {
		leg.SlotIDs = append([]string(nil), leg.SlotIDs...)
		out.Legs[i] = leg
	}
	normalize(&out)
	return out
}
