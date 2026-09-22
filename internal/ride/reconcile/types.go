// Package reconcile implements bike-ride close-out and its paper trail:
// supported-rider accounting, the post-ride checklist, SAG driver shift
// summaries, and NCS shift-relief handoff.
//
// Rider accounting is deliberately NOT reimplemented here. internal/course
// (WP3) already owns store.RiderException and its supported/unsupported
// state — the exact population-definition transition governing fact 4
// describes: a rider leaves the accounted-for (supported) set by passing a
// shutoff or declining SAG, a status change, never a delete. This package
// reads that state (via the *course.Manager it is constructed with) rather
// than adding a second, differently-shaped rider-exception table; see
// store.SAGShiftSummary's doc comment for the same note from the other
// side.
package reconcile

import (
	"errors"
	"time"

	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/store"
)

// Shift summary statuses (mirrors store.ShiftDraft/store.ShiftFiled so
// callers outside this package need only import store's constants; kept
// here too for readability at call sites within reconcile).
const (
	ShiftDraft = store.ShiftDraft
	ShiftFiled = store.ShiftFiled
)

// Handoff item kinds and statuses.
const (
	HandoffAwaitingReply = store.HandoffAwaitingReply
	HandoffPendingAction = store.HandoffPendingAction
	HandoffFYI           = store.HandoffFYI

	HandoffOpen      = store.HandoffOpen
	HandoffResolved  = store.HandoffResolved
	HandoffCancelled = store.HandoffCancelled
)

// Event types for WebSocket broadcast (bridged by server.bridgeReconcileEvents).
const (
	EventShiftSummaryUpdated = "ride_shift_summary_updated"
	EventHandoffItemCreated  = "ride_handoff_item_created"
	EventHandoffItemUpdated  = "ride_handoff_item_updated"
	EventShiftHandoff        = "ride_shift_handoff"
)

// Timeline (store.NetEvent.Type) entry types, written via
// netcontrol.Manager.AddTimelineEvent/AddTimelineEventWithDetails.
const (
	TimelineCloseoutForced = "ride_closeout_forced"
	TimelineShiftFiled     = "ride_shift_summary_filed"
	TimelineHandoff        = "ride_ncs_handoff"
)

// Event is the WS envelope, the same shape as course.Event / checkpoint.Event.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Sentinel errors.
var (
	ErrNotFound     = errors.New("not found")
	ErrIllegal      = errors.New("illegal transition")
	ErrNotReady     = errors.New("close-out checklist incomplete")
	ErrNoNet        = errors.New("net not found")
	ErrForceBlocked = errors.New("force-close is not permitted for this net")
)

// Policy is the agency-configurable close-out behaviour not already owned
// by another package's own config (course.Manager owns the sweep label and
// CloseRequiresSweep; WP1's store.NetRideConfig owns agency/incident naming
// when present). Every field here is data, never a constant in a check.
type Policy struct {
	RequireRestStopsClosed bool // default true
	RequireFieldUnitsOut   bool // default true
	RequireShiftSummaries  bool // default false (advisory)
	AllowForceClose        bool // default true; always logged with reason
	NCSShiftMinutes        int  // default 120; briefing surfaces "shift due"
}

// DefaultPolicy returns the shipped defaults.
func DefaultPolicy() Policy {
	return Policy{
		RequireRestStopsClosed: true,
		RequireFieldUnitsOut:   true,
		RequireShiftSummaries:  false,
		AllowForceClose:        true,
		NCSShiftMinutes:        120,
	}
}

// ---- computed views ----

// ExceptionCounts summarizes internal/course's rider exceptions.
type ExceptionCounts struct {
	Supported   int            `json:"supported"`
	Unsupported int            `json:"unsupported"`
	ByKind      map[string]int `json:"byKind"` // never nil
}

// Accounting is the supported-rider accounting view (governing fact 4).
// There is deliberately no "riders total" — the population is edge-defined;
// what NCS needs is "is the trailing edge home" and "how many riders are
// still open exceptions".
type Accounting struct {
	NetID          string                 `json:"netId"`
	SweepComplete  bool                   `json:"sweepComplete"`
	SweepDetail    string                 `json:"sweepDetail"` // e.g. "sweep passed seq 7 (Finish)" or "no checkpoints configured"
	Counts         ExceptionCounts        `json:"counts"`
	OpenExceptions []store.RiderException `json:"openExceptions"` // supported (still-open) exceptions; never nil
	ComputedAt     time.Time              `json:"computedAt"`
}

// CloseoutItem is one line of the post-ride checklist.
type CloseoutItem struct {
	Key      string `json:"key"` // sweep_finished|exceptions_resolved|rest_stops_closed|field_units_out|handoff_cleared|shift_summaries_filed|net_closed
	Label    string `json:"label"`
	Done     bool   `json:"done"`
	Blocking bool   `json:"blocking"` // false = advisory; from Policy
	Count    int    `json:"count"`    // progress numerator (e.g. 11 units out)
	Total    int    `json:"total"`    // denominator (e.g. 12 units)
	Detail   string `json:"detail"`   // "RS2, RS5 still active"
}

// CloseoutStatus is the whole post-ride checklist.
type CloseoutStatus struct {
	NetID      string         `json:"netId"`
	Ready      bool           `json:"ready"` // every Blocking item except net_closed is Done
	Items      []CloseoutItem `json:"items"` // never nil, fixed order
	ComputedAt time.Time      `json:"computedAt"`
}

// SAGShiftDerived are the counts this package can derive from logged SAG
// requests for one check-in (internal/ride's Manager). Zero when the SAG
// manager was never wired in, or the vehicle worked no requests.
type SAGShiftDerived struct {
	Transports        int `json:"transports"`
	Assists           int `json:"assists"` // repairs, flats (self-resolved on scene)
	TubesProvided     int `json:"tubesProvided"`
	TiresProvided     int `json:"tiresProvided"`
	MinorFirstAid     int `json:"minorFirstAid"`
	IncidentsAttended int `json:"incidentsAttended"`
}

// ShiftSummaryView is what the API returns: the stored record plus the
// derived tally and the effective (entered ?? derived) numbers the printed
// form actually shows.
type ShiftSummaryView struct {
	store.SAGShiftSummary
	Derived   SAGShiftDerived `json:"derived"`
	Effective SAGShiftDerived `json:"effective"`
	NetMiles  *float64        `json:"netMiles,omitempty"` // OdometerEnd - OdometerStart when both set
}

// Effective merges driver-entered counts over the derived tally: an entered
// pointer (including an explicit zero) always wins; nil falls back to
// derived.
func Effective(entered store.SAGShiftCounts, derived SAGShiftDerived) SAGShiftDerived {
	pick := func(p *int, fallback int) int {
		if p != nil {
			return *p
		}
		return fallback
	}
	return SAGShiftDerived{
		Transports:        pick(entered.Transports, derived.Transports),
		Assists:           pick(entered.Assists, derived.Assists),
		TubesProvided:     pick(entered.TubesProvided, derived.TubesProvided),
		TiresProvided:     pick(entered.TiresProvided, derived.TiresProvided),
		MinorFirstAid:     pick(entered.MinorFirstAid, derived.MinorFirstAid),
		IncidentsAttended: pick(entered.IncidentsAttended, derived.IncidentsAttended),
	}
}

// BriefingLine is one row of a briefing section.
type BriefingLine struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Severity string `json:"severity"` // info|warn|urgent
	RefType  string `json:"refType,omitempty"`
	RefID    string `json:"refId,omitempty"`
}

// BriefingSection is a titled group of briefing lines contributed by a
// BriefingSource (e.g. "SAG open", "next shutoff", "rest stops").
type BriefingSection struct {
	Key   string         `json:"key"`
	Title string         `json:"title"`
	Lines []BriefingLine `json:"lines"` // never nil
}

// BriefingSource contributes sections to the shift-relief briefing. Other
// ride-mode packages (or a future one) register via AddBriefingSource.
type BriefingSource interface {
	BriefingSections(netID string) []BriefingSection
}

// AwaitingReply is a derived (not persisted) handoff line built from
// outbound APRS messages that are not yet acked.
type AwaitingReply struct {
	MessageID string    `json:"messageId"`
	To        string    `json:"to"`
	Body      string    `json:"body"`
	SentAt    time.Time `json:"sentAt"`
	State     string    `json:"state"` // pending|sent
}

// ShiftBriefing is the live shift-relief briefing: "any pending activity —
// messages you have sent and replies you expect, and who gets the reply."
type ShiftBriefing struct {
	NetID           string              `json:"netId"`
	NetName         string              `json:"netName"`
	NCSCallsign     string              `json:"ncsCallsign"`
	NCSSince        *time.Time          `json:"ncsSince,omitempty"`
	ShiftDueAt      *time.Time          `json:"shiftDueAt,omitempty"`
	OpenItems       []store.HandoffItem `json:"openItems"`       // never nil
	AwaitingReplies []AwaitingReply     `json:"awaitingReplies"` // never nil
	Accounting      Accounting          `json:"accounting"`
	Closeout        CloseoutStatus      `json:"closeout"`
	Sections        []BriefingSection   `json:"sections"`     // never nil
	RecentEvents    []store.NetEvent    `json:"recentEvents"` // last 30 minutes; never nil
	GeneratedAt     time.Time           `json:"generatedAt"`
}

// re-exported course constants so callers of this package's Accounting
// don't need their own import of internal/course just to compare a status.
const (
	SupportSupported   = course.SupportSupported
	SupportUnsupported = course.SupportUnsupported
)
