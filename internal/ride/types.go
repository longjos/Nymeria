// Package ride implements bike-ride SAG (support-and-gear transport)
// operations: SAGRequest is the central domain object, and is NOT a
// netcontrol.NetMission — a mission is one task at one location, a SAG
// request is a transport job that may span several vehicle legs and
// partial pickups.
//
// Rider accounting here is EDGE-based, not roster-based (there is no rider
// table anywhere in this package): a bib number is a label on a slot, never
// a foreign key. Route-relative position (mile marker / miles remaining) is
// the primary coordinate system; lat/lon is secondary.
package ride

import (
	"errors"

	"github.com/narvel/nymeria/internal/store"
)

// Five-tier Marin ARS ladder (bike-ride v1). This is NOT
// netcontrol.Traffic*; ride mode uses its own vocabulary.
const (
	PriorityEmergency = "emergency"
	PriorityPriority  = "priority"
	PriorityHigh      = "high"
	PriorityMedium    = "medium"
	PriorityLow       = "low"
)

// Request statuses (derived by DeriveStatus, persisted only so the board can
// sort/filter).
const (
	ReqOpen         = "open"     // waiting slots, no active leg
	ReqAssigned     = "assigned" // leg dispatched, vehicle not yet moving
	ReqEnroute      = "enroute"
	ReqOnScene      = "onscene"
	ReqTransporting = "transporting" // >=1 slot loaded, nothing left waiting
	ReqPartial      = "partial"      // some slots taken, others still waiting with NO active leg
	ReqComplete     = "complete"     // every slot terminal, not cancelled
	ReqCancelled    = "cancelled"    // explicit NCS cancel
)

// Leg statuses.
const (
	LegDispatched = "dispatched"
	LegEnroute    = "enroute"
	LegOnScene    = "onscene"
	LegLoaded     = "loaded"
	LegDelivered  = "delivered"
	LegReleased   = "released" // vehicle freed before loading anyone
)

// Slot dispositions.
const (
	SlotWaiting      = "waiting"
	SlotLoaded       = "loaded"
	SlotDelivered    = "delivered"
	SlotSelfResolved = "self_resolved" // fixed own flat, rode on
	SlotDeclined     = "declined"      // refused SAG
	SlotNotFound     = "not_found"     // vehicle arrived, rider gone
	SlotHandedOff    = "handed_off"    // to medical/ambulance
	SlotCancelled    = "cancelled"     // duplicate/erroneous entry
)

// Pickup and dropoff kinds.
const (
	LocCourse       = "course"
	LocRestStop     = "reststop"
	LocCheckpoint   = "checkpoint"
	LocStart        = "start"
	LocFinish       = "finish"
	LocHospital     = "hospital"
	LocNextRestStop = "next_reststop"
	LocOther        = "other"
)

// PickupKindOrder and DropoffKindOrder are the allowed store.SAGLocation
// Kind values on each end of a SAG job, IN THE ORDER THE COMPOSER SHOWS
// THEM. The order is operator-facing, so it is a slice and not a map: Go
// randomises map iteration, and a picker whose options reshuffle between two
// openings of the same dialog cannot be driven at radio speed. The first
// entry of each is the composer's default — a SAG call almost always comes
// from a rider stopped on the course, heading for the next rest stop.
var PickupKindOrder = []string{
	LocCourse, LocRestStop, LocCheckpoint, LocStart, LocFinish, LocOther,
}
var DropoffKindOrder = []string{
	LocNextRestStop, LocRestStop, LocFinish, LocStart, LocHospital, LocOther,
}

// ValidPickupKinds and ValidDropoffKinds are the validity sets, derived from
// the ordered slices so the two can never disagree.
var ValidPickupKinds = kindSet(PickupKindOrder)
var ValidDropoffKinds = kindSet(DropoffKindOrder)

func kindSet(kinds []string) map[string]bool {
	out := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		out[k] = true
	}
	return out
}

// Bike dispositions. Whether the bike travels is a SEPARATE AXIS from what
// happens to the rider: a rider can be transported while the bike is left
// at the rest stop, or ride in one van while the bike follows in another.
// It is decided by the driver at LOAD time, not by the caller at request
// time, so it is settable all the way through LoadSlots.
//
// Only BikeWithRider consumes one of the vehicle's rack slots.
const (
	BikeWithRider    = "with_rider"    // on the rack of the vehicle carrying the rider
	BikeNone         = "none"          // no bike to carry
	BikeLeftBehind   = "left_behind"   // stays at the scene — somebody's property, an ICS-214 line
	BikeOtherVehicle = "other_vehicle" // travelling separately from its rider
)

// BikeDispositionOrder is the order the load control offers them in: the
// overwhelmingly common answer first.
var BikeDispositionOrder = []string{BikeWithRider, BikeNone, BikeLeftBehind, BikeOtherVehicle}

// ValidBikeDispositions is the accepted set. An unrecognised value is
// refused rather than stored, so the board never has to render an unknown.
var ValidBikeDispositions = map[string]bool{
	BikeWithRider: true, BikeNone: true, BikeLeftBehind: true, BikeOtherVehicle: true,
}

// Event types for the WebSocket hub.
const (
	EventSAGRequestCreated = "sag_request_created"
	EventSAGRequestUpdated = "sag_request_updated"
	EventSAGVehicleUpdated = "sag_vehicle_updated"
	// Timeline rows are emitted as netcontrol.EventTimelineEntry
	// ("net_timeline_entry") carrying a store.NetEvent so the existing
	// timeline panel shows them unchanged.
)

// Timeline NetEvent.Type values written by this package.
const (
	TLSAGRequested    = "sag_requested"
	TLSAGDispatched   = "sag_dispatched"
	TLSAGEnroute      = "sag_enroute"
	TLSAGOnScene      = "sag_onscene"
	TLSAGLoaded       = "sag_loaded"
	TLSAGDelivered    = "sag_delivered"
	TLSAGReleased     = "sag_released"
	TLSAGSlotResolved = "sag_slot_resolved"
	TLSAGCancelled    = "sag_cancelled"
	TLSAGUpdated      = "sag_updated"
)

// Event represents a ride-mode event for WebSocket broadcast.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Sentinel errors so handlers can pick the right HTTP status with errors.Is.
//
// ErrSeatsExceeded is a SUBSET of ErrOverCapacity: every seat refusal is also
// an over-capacity condition, so an existing errors.Is(err, ErrOverCapacity)
// check keeps working, but a caller that needs to know whether the operator
// may override can ask for ErrSeatsExceeded specifically.
var (
	ErrNotFound      = errors.New("not found")
	ErrOverCapacity  = errors.New("vehicle over capacity")
	ErrSeatsExceeded = errors.New("vehicle has no seat for this rider")
)

// OverCapacityError carries the numbers a 409 response reports, and — the
// load-bearing part — WHICH DIMENSION overflowed.
//
// The two dimensions are NOT symmetric:
//
//   - SEATS are a seatbelt count, a legal limit and not a judgement call.
//     SeatsExceeded is FATAL: AllowOvercommit does not open it, on any path.
//   - RACKS are a preference. A bike can ride in the bed of a truck if the
//     rider is happy with that, so RacksExceeded is a question the operator
//     is allowed to answer yes to via AllowOvercommit, and the resulting leg
//     carries Overcommitted.
//
// Consequently store.SAGLeg.Overcommitted now means "bikes exceed the rack
// count", never "riders exceed the seat count" — the latter cannot happen.
type OverCapacityError struct {
	CommittedSeats int
	Seats          int
	NeedSeats      int // seats this action would add
	CommittedRacks int
	RackSlots      int
	NeedRacks      int // racks this action would add

	SeatsExceeded bool // fatal, never overridable
	RacksExceeded bool // overridable with AllowOvercommit
}

func (e *OverCapacityError) Error() string {
	if e.SeatsExceeded {
		return ErrSeatsExceeded.Error()
	}
	return ErrOverCapacity.Error()
}

// Unwrap returns both sentinels for a seat refusal so errors.Is matches
// either one.
func (e *OverCapacityError) Unwrap() []error {
	if e.SeatsExceeded {
		return []error{ErrOverCapacity, ErrSeatsExceeded}
	}
	return []error{ErrOverCapacity}
}

// Config is the agency-configurable vocabulary. A future net-profile
// integration may supply this per net; until then DefaultConfig() ships the
// Marin ladder.
type Config struct {
	Priorities                  []string `json:"priorities"`            // ordered most->least urgent
	DefaultCoursePriority       string   `json:"defaultCoursePriority"` // pickup from course -> "high"
	DefaultStopPriority         string   `json:"defaultStopPriority"`   // pickup from reststop/checkpoint/start/finish -> "medium"
	Reasons                     []string `json:"reasons"`               // advisory list for the composer
	DefaultSeats                int      `json:"defaultSeats"`          // used when a leg is dispatched to an unregistered sag vehicle
	DefaultRackSlots            int      `json:"defaultRackSlots"`
	RequireNameForHospitalStart bool     `json:"requireNameForHospitalStart"`
}

// DefaultConfig returns the Marin ARS defaults.
func DefaultConfig() Config {
	return Config{
		Priorities:                  []string{PriorityEmergency, PriorityPriority, PriorityHigh, PriorityMedium, PriorityLow},
		DefaultCoursePriority:       PriorityHigh,
		DefaultStopPriority:         PriorityMedium,
		Reasons:                     []string{"mechanical", "flat", "fatigue", "medical_minor", "weather", "cutoff", "other"},
		DefaultSeats:                3,
		DefaultRackSlots:            2,
		RequireNameForHospitalStart: true,
	}
}

// SAGVehicleStatus is the read model for the board/strip: configured
// capacity plus committed/available derived from active legs across ALL
// requests.
type SAGVehicleStatus struct {
	store.SAGVehicle
	Callsign       string `json:"callsign"`
	TacticalCall   string `json:"tacticalCall"`
	CheckInStatus  string `json:"checkInStatus"` // NetCheckIn.Status
	CommittedSeats int    `json:"committedSeats"`
	CommittedRacks int    `json:"committedRacks"`
	// AvailableSeats never goes negative through a DISPATCH — a seat overflow
	// is refused outright — but it can if SetVehicle later shrinks a vehicle
	// below what is already committed to it, which the board must still be
	// able to render. AvailableRacks additionally goes negative after a
	// knowing rack overcommit.
	AvailableSeats   int      `json:"availableSeats"`
	AvailableRacks   int      `json:"availableRacks"`
	ActiveLegIDs     []string `json:"activeLegIds"`     // never nil
	ActiveRequestIDs []string `json:"activeRequestIds"` // never nil
}

// SAGBoard is GET /nets/{id}/sag.
type SAGBoard struct {
	NetID    string             `json:"netId"`
	Requests []store.SAGRequest `json:"requests"` // never nil
	Vehicles []SAGVehicleStatus `json:"vehicles"` // never nil
	Counts   map[string]int     `json:"counts"`   // request status -> count, every status key present
	Config   Config             `json:"config"`
}

// allRequestStatuses lists every request status, used to seed SAGBoard.Counts
// with a zero for each one so the frontend never has to guard a missing key.
var allRequestStatuses = []string{
	ReqOpen, ReqAssigned, ReqEnroute, ReqOnScene, ReqTransporting, ReqPartial, ReqComplete, ReqCancelled,
}

// CheckInSource is the narrow dependency this package needs from net
// control, satisfied by *netcontrol.Manager.
type CheckInSource interface {
	GetNet(id string) (*store.Net, bool)
	GetCheckIns(netID string) []store.NetCheckIn
}

// AnnotationSource is the narrow dependency this package needs from
// annotations, satisfied by *annotation.Manager. May be nil: skips
// annotation validation.
type AnnotationSource interface {
	Get(id string) (*store.Annotation, bool)
}
