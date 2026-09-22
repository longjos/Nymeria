// Package phase implements the bike-ride phase model (WP5b): the single
// piece of net-wide state the ride status strip's zone set is keyed off —
// pre-start, launched, mid-ride, closing, collapse, reconcile.
//
// The governing rule (docs/ride-strip-spec.md §8): phase is ALWAYS
// operator-set and NEVER silently auto-switched. Silent state change is the
// classic ICS failure of an assignment nobody logged. This package therefore
// exposes two separate things:
//
//   - GetState / SetPhase: what phase the net is actually in, an audit
//     trail (SetBy/Reason/UpdatedAt), and every transition logged as a
//     store.NetEvent so it shows up on the net timeline.
//   - Suggest (folded into GetState.Suggestion): what phase the system
//     THINKS the net should move to next, and why — computed fresh on every
//     read from live course/checkpoint data, never persisted, never acted
//     on by itself. The NCS confirms a suggestion by calling SetPhase; this
//     package never calls SetPhase on its own.
//
// Legal transition table (see LegalTransition): forward exactly one step at
// a time — pre-start -> launched -> mid-ride -> closing -> collapse ->
// reconcile — because each step is a real operational milestone (LEAD live,
// sweep past the first stop, a shutoff 60 minutes out, NCS declaring
// collapse, the last stop clear) that the strip's zone set depends on
// having actually happened, in order; skipping one is illegal. Backward is
// legal to ANY earlier phase, with no distance limit and no exception: an
// NCS who advanced too early (or mis-clicked) must always be able to
// correct it, and a full reset to pre-start (a voided false start) is rare
// but legitimate. A backward move requires a Reason; a forward move does
// not (the trigger that earned it is reason enough). Moving to the current
// phase is illegal — it is a no-op with nothing to log.
//
// Suggestion triggers (docs/ride-strip-spec.md §8), each read fresh, only
// evaluated for the CURRENT phase's own single next step:
//
//	pre-start -> launched:  first lead passage logged
//	launched  -> mid-ride:  sweep passes the first non-start station ("stop 1")
//	mid-ride  -> closing:   now is within 60 minutes of the earliest armed shutoff
//	closing   -> collapse:  NO machine trigger — "NCS declares, usually after
//	                        the lead finishes" is a judgment call the spec
//	                        deliberately leaves to the operator
//	collapse  -> reconcile: every sequenced station is closed ("last stop clear")
//
// Only applicable to bike-ride profile nets — GetState and SetPhase both
// answer ErrProfileMismatch for any other profile, so a general net is
// completely unaffected (nothing is ever read from or written to it).
package phase

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/netprofile"
	"github.com/narvel/nymeria/internal/store"
)

// Ride phase ids. Order matters: it is the canonical forward sequence
// LegalTransition and Suggest are both built on.
const (
	PreStart  = "pre-start"
	Launched  = "launched"
	MidRide   = "mid-ride"
	Closing   = "closing"
	Collapse  = "collapse"
	Reconcile = "reconcile"
)

// Order is every ride phase, in forward sequence. Never mutated by callers —
// treat as read-only.
var Order = []string{PreStart, Launched, MidRide, Closing, Collapse, Reconcile}

var phaseIndex = func() map[string]int {
	m := make(map[string]int, len(Order))
	for i, p := range Order {
		m[p] = i
	}
	return m
}()

// Valid reports whether phase is exactly one of the six registered ride
// phases.
func Valid(phase string) bool {
	_, ok := phaseIndex[phase]
	return ok
}

// indexOf returns phase's position in Order, or -1 if it is not a
// registered phase.
func indexOf(phase string) int {
	i, ok := phaseIndex[phase]
	if !ok {
		return -1
	}
	return i
}

// LegalTransition reports whether moving the ride phase from `from` to `to`
// is allowed by the state machine. See the package doc for the full
// rationale: forward exactly one step, backward to any earlier phase, same
// phase and unknown phases are always illegal.
func LegalTransition(from, to string) bool {
	fi, ti := indexOf(from), indexOf(to)
	if fi < 0 || ti < 0 {
		return false
	}
	if ti == fi+1 {
		return true
	}
	return ti < fi
}

// WS event type.
const EventPhaseUpdated = "ride_phase_updated"

// Timeline (store.NetEvent.Type) value written on every SetPhase, mirroring
// course.TimelineShutoffFired's pattern.
const TimelinePhaseChanged = "ride_phase_changed"

// Sentinel errors so handlers can pick the right HTTP status with errors.Is.
var (
	ErrNotFound          = errors.New("net not found")
	ErrProfileMismatch   = errors.New("net profile is not bike-ride")
	ErrIllegalTransition = errors.New("illegal ride phase transition")
)

// Suggestion is the system's read of "what phase should we be in, and why"
// — never authoritative and never persisted. Phase is "" when the current
// phase's own trigger has not fired (including whenever the current phase
// is reconcile, or has no defined trigger, e.g. closing -> collapse).
type Suggestion struct {
	Phase  string `json:"phase"`
	Reason string `json:"reason"`
}

// State is GET /nets/{id}/ride/phase's response: the authoritative current
// phase plus the live-computed suggestion.
type State struct {
	NetID      string     `json:"netId"`
	Phase      string     `json:"phase"`
	SetBy      string     `json:"setBy"`
	Reason     string     `json:"reason"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	Suggestion Suggestion `json:"suggestion"`
}

// SetInput is the payload for SetPhase.
type SetInput struct {
	To     string
	By     string
	Reason string // required for a backward move; optional for forward
}

// Event represents a ride-phase event for WebSocket broadcast.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Manager owns the current ride phase for every net. It mirrors
// course.Manager's shape: a store-backed in-memory cache guarded by an
// RWMutex, a buffered events channel, and an injectable clock for
// deterministic tests.
//
// netMgr must not be nil (GetState/SetPhase need it to look up the net and
// check its profile). courseMgr and cpMgr may both be nil — Suggest then
// simply never proposes anything, the same graceful-degradation contract
// internal/ride/reconcile's collaborators follow.
type Manager struct {
	store     store.Store
	netMgr    *netcontrol.Manager
	courseMgr *course.Manager
	cpMgr     *checkpoint.Manager

	mu     sync.RWMutex
	states map[string]store.RidePhaseState // netID -> current phase state
	now    func() time.Time
	events chan Event
}

// NewManager creates a Manager.
func NewManager(s store.Store, netMgr *netcontrol.Manager, courseMgr *course.Manager, cpMgr *checkpoint.Manager) *Manager {
	return &Manager{
		store:     s,
		netMgr:    netMgr,
		courseMgr: courseMgr,
		cpMgr:     cpMgr,
		states:    make(map[string]store.RidePhaseState),
		events:    make(chan Event, 64),
	}
}

// SetClock injects a time source (defaults to time.Now) for deterministic
// tests.
func (m *Manager) SetClock(now func() time.Time) {
	m.mu.Lock()
	m.now = now
	m.mu.Unlock()
}

func (m *Manager) clock() time.Time {
	m.mu.RLock()
	fn := m.now
	m.mu.RUnlock()
	if fn == nil {
		return time.Now().UTC()
	}
	return fn()
}

// Events returns the events channel for WebSocket broadcast.
func (m *Manager) Events() <-chan Event {
	return m.events
}

func (m *Manager) emit(evt Event) {
	select {
	case m.events <- evt:
	default:
	}
}

// Load hydrates the phase cache from the store for every known net. A net
// with no row simply defaults to pre-start (see getOrDefault) — this is not
// an error.
func (m *Manager) Load() error {
	nets, err := m.store.LoadNets()
	if err != nil {
		return fmt.Errorf("load nets for ride phase: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range nets {
		ps, err := m.store.LoadRidePhase(n.ID)
		if err != nil {
			return fmt.Errorf("load ride phase for net %s: %w", n.ID, err)
		}
		if ps != nil {
			m.states[n.ID] = *ps
		}
	}
	return nil
}

func (m *Manager) getOrDefault(netID string) store.RidePhaseState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ps, ok := m.states[netID]; ok {
		return ps
	}
	return store.RidePhaseState{NetID: netID, Phase: PreStart}
}

// checkBikeRideNet resolves netID to its Net and reports ErrNotFound /
// ErrProfileMismatch the same way for both GetState and SetPhase.
func (m *Manager) checkBikeRideNet(netID string) error {
	if m.netMgr == nil {
		return ErrNotFound
	}
	n, ok := m.netMgr.GetNet(netID)
	if !ok {
		return ErrNotFound
	}
	if n.Profile != netprofile.ProfileBikeRide {
		return ErrProfileMismatch
	}
	return nil
}

// GetState returns netID's current ride phase and the live-computed
// suggestion. ErrNotFound when the net does not exist; ErrProfileMismatch
// when its profile is not bike-ride (a general net is completely
// unaffected by this package).
func (m *Manager) GetState(netID string) (*State, error) {
	if err := m.checkBikeRideNet(netID); err != nil {
		return nil, err
	}
	ps := m.getOrDefault(netID)
	return &State{
		NetID:      netID,
		Phase:      ps.Phase,
		SetBy:      ps.SetBy,
		Reason:     ps.Reason,
		UpdatedAt:  ps.UpdatedAt,
		Suggestion: m.suggest(netID, ps.Phase),
	}, nil
}

// SetPhase moves netID's ride phase per the legal transition table. A
// backward move (To earlier than the current phase) requires a non-blank
// Reason; a forward move does not.
func (m *Manager) SetPhase(netID string, in SetInput) (*State, error) {
	if err := m.checkBikeRideNet(netID); err != nil {
		return nil, err
	}
	to := strings.ToLower(strings.TrimSpace(in.To))
	if !Valid(to) {
		return nil, fmt.Errorf("invalid ride phase %q", in.To)
	}

	current := m.getOrDefault(netID)
	if !LegalTransition(current.Phase, to) {
		return nil, ErrIllegalTransition
	}
	backward := indexOf(to) < indexOf(current.Phase)
	if backward && strings.TrimSpace(in.Reason) == "" {
		return nil, errors.New("reason is required to move the ride phase backward")
	}

	now := m.clock()
	updated := store.RidePhaseState{
		NetID:     netID,
		Phase:     to,
		SetBy:     in.By,
		Reason:    in.Reason,
		UpdatedAt: now,
	}
	if err := m.store.SaveRidePhase(updated); err != nil {
		return nil, fmt.Errorf("persist ride phase: %w", err)
	}

	m.mu.Lock()
	m.states[netID] = updated
	m.mu.Unlock()

	m.logTimeline(netID, in.By, current.Phase, to, in.Reason, backward)

	st := &State{
		NetID:      netID,
		Phase:      to,
		SetBy:      in.By,
		Reason:     in.Reason,
		UpdatedAt:  now,
		Suggestion: m.suggest(netID, to),
	}
	m.emit(Event{Type: EventPhaseUpdated, Data: st})
	return st, nil
}

// logTimeline writes a NetEvent the same fire-and-forget way
// course.Manager.logTimeline does (errors are not actionable here — the
// request already succeeded).
func (m *Manager) logTimeline(netID, by, from, to, reason string, backward bool) {
	direction := "advanced"
	if backward {
		direction = "moved back"
	}
	summary := fmt.Sprintf("ride phase %s: %s → %s", direction, from, to)
	if reason != "" {
		summary = fmt.Sprintf("%s (%s)", summary, reason)
	}
	m.store.SaveNetEvent(store.NetEvent{
		ID:        uuid.New().String(),
		NetID:     netID,
		Type:      TimelinePhaseChanged,
		Callsign:  by,
		Summary:   summary,
		Details:   "{}",
		CreatedAt: m.clock(),
	})
}

// suggest computes the live trigger read for the given current phase. It
// only ever evaluates the ONE trigger that would advance current to its
// immediate successor — Suggest never proposes skipping ahead, matching
// LegalTransition's own one-step-forward rule.
func (m *Manager) suggest(netID, current string) Suggestion {
	switch current {
	case PreStart:
		if m.leadHasPassage(netID) {
			return Suggestion{Phase: Launched, Reason: "first lead passage logged"}
		}
	case Launched:
		if label, ok := m.sweepPassedStop1(netID); ok {
			return Suggestion{Phase: MidRide, Reason: fmt.Sprintf("sweep passed %s", label)}
		}
	case MidRide:
		if name, at, ok := m.withinSixtyOfFirstShutoff(netID); ok {
			return Suggestion{Phase: Closing, Reason: fmt.Sprintf("T-60 to %s at %s", name, at.UTC().Format("15:04"))}
		}
	case Closing:
		// No machine trigger — see the package doc: NCS declares collapse,
		// usually after the lead finishes. Deliberately never suggested.
	case Collapse:
		if m.allStationsClosed(netID) {
			return Suggestion{Phase: Reconcile, Reason: "last stop clear"}
		}
	}
	return Suggestion{}
}

// leadHasPassage reports whether any passage has ever been logged under the
// net's configured LEAD label (course.CourseConfig.LeadLabel).
func (m *Manager) leadHasPassage(netID string) bool {
	if m.courseMgr == nil || m.cpMgr == nil {
		return false
	}
	cfg := m.courseMgr.GetConfig(netID)
	progress, err := m.cpMgr.GetProgress(netID)
	if err != nil {
		return false
	}
	for _, e := range progress.Elements {
		if strings.EqualFold(e.Label, cfg.LeadLabel) {
			return true
		}
	}
	return false
}

// sweepPassedStop1 reports whether the sweep has passed "stop 1" — the
// first sequenced station that is not the start line itself — and its
// label. Stations are evaluated in ascending sequence order; only the
// first non-start station is ever consulted; a not-yet-passed stop 1
// answers false even if some LATER station happens to already be closed
// out of order.
func (m *Manager) sweepPassedStop1(netID string) (string, bool) {
	if m.courseMgr == nil {
		return "", false
	}
	state, err := m.courseMgr.State(netID)
	if err != nil {
		return "", false
	}
	for _, sv := range state.Stations {
		if sv.Category == "start" {
			continue
		}
		if sv.Closure.State == course.StationSweepPassed || sv.Closure.State == course.StationClosed {
			return sv.Label, true
		}
		return "", false
	}
	return "", false
}

// withinSixtyOfFirstShutoff reports whether the clock is within 60 minutes
// of the earliest still-planned shutoff, along with its name and scheduled
// time.
func (m *Manager) withinSixtyOfFirstShutoff(netID string) (string, time.Time, bool) {
	if m.courseMgr == nil {
		return "", time.Time{}, false
	}
	state, err := m.courseMgr.State(netID)
	if err != nil || state.NextShutoff == nil {
		return "", time.Time{}, false
	}
	sp := state.NextShutoff
	if !m.clock().Before(sp.ScheduledAt.Add(-60 * time.Minute)) {
		return sp.Name, sp.ScheduledAt, true
	}
	return "", time.Time{}, false
}

// allStationsClosed reports whether every sequenced station on the course
// is closed ("last stop clear").
func (m *Manager) allStationsClosed(netID string) bool {
	if m.courseMgr == nil {
		return false
	}
	state, err := m.courseMgr.State(netID)
	if err != nil {
		return false
	}
	return state.AllStationsClosed
}
