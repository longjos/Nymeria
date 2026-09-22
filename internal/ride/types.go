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

// ValidPickupKinds and ValidDropoffKinds are the allowed store.SAGLocation
// Kind values on each end of a SAG job.
var ValidPickupKinds = map[string]bool{
	LocCourse: true, LocRestStop: true, LocCheckpoint: true,
	LocStart: true, LocFinish: true, LocOther: true,
}
var ValidDropoffKinds = map[string]bool{
	LocNextRestStop: true, LocRestStop: true, LocFinish: true,
	LocStart: true, LocHospital: true, LocOther: true,
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
var (
	ErrNotFound     = errors.New("not found")
	ErrOverCapacity = errors.New("vehicle over capacity")
)

// OverCapacityError carries the numbers a 409 response reports alongside
// ErrOverCapacity (Unwrap makes errors.Is(err, ErrOverCapacity) true).
type OverCapacityError struct {
	CommittedSeats int
	Seats          int
	CommittedRacks int
	RackSlots      int
}

func (e *OverCapacityError) Error() string { return ErrOverCapacity.Error() }
func (e *OverCapacityError) Unwrap() error { return ErrOverCapacity }

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
	Callsign         string   `json:"callsign"`
	TacticalCall     string   `json:"tacticalCall"`
	CheckInStatus    string   `json:"checkInStatus"` // NetCheckIn.Status
	CommittedSeats   int      `json:"committedSeats"`
	CommittedRacks   int      `json:"committedRacks"`
	AvailableSeats   int      `json:"availableSeats"` // may be negative when Overcommitted
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
