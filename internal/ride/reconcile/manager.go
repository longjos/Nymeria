package reconcile

import (
	"fmt"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/store"
)

// Manager owns SAG driver shift summaries and NCS shift-relief handoff, and
// computes the supported-rider accounting / post-ride close-out views from
// its collaborators (course for rider exceptions, netcontrol for check-ins
// and net lifecycle, the SAG manager for shift tallies, annotations for
// rest-stop status). Every collaborator but store and netMgr may be nil —
// each accessor degrades gracefully (see the doc comment on each field).
type Manager struct {
	store  store.Store
	netMgr *netcontrol.Manager

	// courseMgr supplies rider-exception accounting and sweep-vs-stations
	// status (fact 4, fact 5). Nil means "course closure not enabled":
	// Accounting reports zero exceptions and Closeout's sweep_finished item
	// reports "no checkpoints configured".
	courseMgr *course.Manager
	// sagMgr supplies the requests a SAG shift summary derives its tally
	// from. Nil means every derived count is zero.
	sagMgr *ride.Manager
	// annMgr supplies rest-stop (category "aid") annotations for the
	// rest_stops_closed checklist item. Nil means that item reports
	// "annotations not available" and is treated as not done.
	annMgr *annotation.Manager
	// msgs supplies outbound-message state for the briefing's
	// "awaiting reply" section. Nil means that section is always empty.
	msgs message.Engine

	policyFn func(netID string) Policy
	briefing []BriefingSource

	mu        sync.RWMutex
	shifts    map[string][]store.SAGShiftSummary // netID ->
	handoffs  map[string][]store.HandoffItem
	transfers map[string][]store.ShiftHandoff
	now       func() time.Time
	events    chan Event
}

// NewManager creates a Manager. netMgr must not be nil; every other
// collaborator may be nil (see the Manager doc comment).
func NewManager(s store.Store, netMgr *netcontrol.Manager, courseMgr *course.Manager, sagMgr *ride.Manager, annMgr *annotation.Manager) *Manager {
	return &Manager{
		store:     s,
		netMgr:    netMgr,
		courseMgr: courseMgr,
		sagMgr:    sagMgr,
		annMgr:    annMgr,
		shifts:    make(map[string][]store.SAGShiftSummary),
		handoffs:  make(map[string][]store.HandoffItem),
		transfers: make(map[string][]store.ShiftHandoff),
		now:       time.Now,
		events:    make(chan Event, 64),
	}
}

// SetClock injects a time source for deterministic tests.
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

// SetMessageEngine wires the message engine the briefing's awaiting-reply
// section reads from. May be left unset — the section is then always empty.
func (m *Manager) SetMessageEngine(e message.Engine) {
	m.mu.Lock()
	m.msgs = e
	m.mu.Unlock()
}

// SetPolicyProvider installs a per-net Policy lookup (e.g. a future WP1
// ride-config bridge). Nets with no policy from f, or when f is nil,
// get DefaultPolicy().
func (m *Manager) SetPolicyProvider(f func(netID string) Policy) {
	m.mu.Lock()
	m.policyFn = f
	m.mu.Unlock()
}

// PolicyFor returns the effective policy for a net.
func (m *Manager) PolicyFor(netID string) Policy {
	m.mu.RLock()
	fn := m.policyFn
	m.mu.RUnlock()
	if fn == nil {
		return DefaultPolicy()
	}
	return fn(netID)
}

// AddBriefingSource registers a section contributor for Briefing.
func (m *Manager) AddBriefingSource(s BriefingSource) {
	m.mu.Lock()
	m.briefing = append(m.briefing, s)
	m.mu.Unlock()
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

// Load hydrates shift summaries, handoff items and shift-handoff history
// from the store for every known net.
func (m *Manager) Load() error {
	if m.netMgr == nil {
		return nil
	}
	nets := m.netMgr.GetNets()

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range nets {
		shifts, err := m.store.LoadSAGShiftSummaries(n.ID)
		if err != nil {
			return fmt.Errorf("load sag shift summaries for net %s: %w", n.ID, err)
		}
		if len(shifts) > 0 {
			m.shifts[n.ID] = shifts
		}

		items, err := m.store.LoadHandoffItems(n.ID)
		if err != nil {
			return fmt.Errorf("load handoff items for net %s: %w", n.ID, err)
		}
		if len(items) > 0 {
			m.handoffs[n.ID] = items
		}

		transfers, err := m.store.LoadShiftHandoffs(n.ID)
		if err != nil {
			return fmt.Errorf("load shift handoffs for net %s: %w", n.ID, err)
		}
		if len(transfers) > 0 {
			m.transfers[n.ID] = transfers
		}
	}
	return nil
}

func (m *Manager) logTimeline(netID, eventType, callsign, summary, detailsJSON string) {
	if m.netMgr == nil {
		return
	}
	if detailsJSON == "" {
		if err := m.netMgr.AddTimelineEvent(netID, eventType, callsign, summary); err != nil {
			// Net may not exist in tests that build a Manager without a
			// backing net; not actionable beyond that.
			_ = err
		}
		return
	}
	_ = m.netMgr.AddTimelineEventWithDetails(netID, eventType, callsign, summary, detailsJSON)
}

// --- Accounting (governing fact 4) ---

// sweepStatus reports whether the sweep has passed every sequenced station
// in the net, and a human detail string. Delegates entirely to
// course.Manager, which already computes this from checkpoint progress
// (course.Manager.State's Sweep field) — see this package's own doc comment
// for why accounting does not reimplement that.
func (m *Manager) sweepStatus(netID string) (bool, string) {
	if m.courseMgr == nil {
		return false, "course closure not available"
	}
	state, err := m.courseMgr.State(netID)
	if err != nil || state == nil {
		return false, "no checkpoints configured"
	}
	if len(state.Stations) == 0 {
		return false, "no checkpoints configured"
	}
	maxSeq := 0
	for _, st := range state.Stations {
		if st.SequenceNumber > maxSeq {
			maxSeq = st.SequenceNumber
		}
	}
	if maxSeq > 0 && state.Sweep.LastCheckpointSeq >= maxSeq {
		return true, fmt.Sprintf("sweep passed seq %d (%s)", state.Sweep.LastCheckpointSeq, state.ClearThroughLabel)
	}
	return false, fmt.Sprintf("sweep last at seq %d of %d", state.Sweep.LastCheckpointSeq, maxSeq)
}

// Accounting builds the supported-rider accounting view for a net.
func (m *Manager) Accounting(netID string) Accounting {
	counts := ExceptionCounts{ByKind: map[string]int{}}
	open := []store.RiderException{}
	if m.courseMgr != nil {
		for _, r := range m.courseMgr.GetRiders(netID) {
			if r.Kind != "" {
				counts.ByKind[r.Kind]++
			}
			if r.SupportStatus == course.SupportUnsupported {
				counts.Unsupported++
				continue
			}
			counts.Supported++
			open = append(open, r)
		}
	}
	sweepDone, sweepDetail := m.sweepStatus(netID)
	return Accounting{
		NetID:          netID,
		SweepComplete:  sweepDone,
		SweepDetail:    sweepDetail,
		Counts:         counts,
		OpenExceptions: open,
		ComputedAt:     m.clock(),
	}
}
