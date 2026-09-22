// traffic.go implements ride-mode's scripted radio traffic types (WP4):
// SupplyRequest and MedicalNotification. Both are notification-and-logistics
// records, not rider-roster entries (governing fact 1 — a bib number here is
// a label, never a foreign key), and both feed the net timeline and the
// activity log the same way SAG requests do.
//
// TrafficManager is a second manager type in this package, deliberately
// separate from Manager (SAG): the two domains share nothing but the net
// they run against, and keeping them apart means CreateSupplyRequest's
// state machine is never tempted to reach into SAG's leg/slot internals or
// vice versa.
package ride

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/netprofile"
	"github.com/narvel/nymeria/internal/store"
)

// netStatusOpen mirrors netcontrol.StatusOpen by literal value — this
// package cannot import netcontrol (see manager.go's own netStatusClosed/
// netStatusArchived for the same reason). Pinned in manager_test.go's
// TestNetControlLiteralsMatch alongside the others.
const netStatusOpen = "open"

// WebSocket event types (payload is always the Redacted() form for medical
// notifications; supply requests have nothing to redact).
const (
	EventSupplyCreated  = "ride_supply_created"
	EventSupplyUpdated  = "ride_supply_updated"
	EventMedicalCreated = "ride_medical_created"
	EventMedicalUpdated = "ride_medical_updated"
)

// Net timeline entry types (store.NetEvent.Type). One per ladder step: the
// log must answer elapsed-time questions, so steps are never collapsed.
const (
	TLSupplyRequested         = "supply_requested"
	TLSupplyItemsAdded        = "supply_items_added"
	TLSupplyReadbackConfirmed = "supply_readback_confirmed"
	TLSupplyRelayed           = "supply_relayed"
	TLSupplyETA               = "supply_eta"
	TLSupplyDelivered         = "supply_delivered"
	TLSupplyCancelled         = "supply_cancelled"
	TLSupplyMerged            = "supply_merged"

	TLMedicalReported          = "medical_reported"
	TLMedicalReadbackConfirmed = "medical_readback_confirmed"
	TLMedicalEMSETA            = "medical_ems_eta"
	TLMedicalOnScene           = "medical_on_scene"
	TLMedicalDeparted          = "medical_departed"
	TLMedicalReleased          = "medical_released"
	TLMedicalCancelled         = "medical_cancelled"
)

// ErrIllegalTransition is the sentinel every forward-only-ladder violation
// wraps, so handlers can map it to 409 with errors.Is.
var ErrIllegalTransition = errors.New("illegal transition")

func illegalTransition(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrIllegalTransition)
}

// Actor is who performed the mutation (from server.UserFromContext plus the
// request body's optional station callsign). Never nil-checked: the zero
// value is fine — an empty UserID/UserName/Callsign just means "unknown
// operator" in the log.
type Actor struct {
	UserID   string
	UserName string
	Callsign string // NCS/logging station; goes in NetEvent.Callsign
}

// NetLookup is the narrow dependency TrafficManager needs to check a net's
// existence and status, satisfied structurally by *netcontrol.Manager.
type NetLookup interface {
	GetNet(id string) (*store.Net, bool)
}

// TimelineSink is the narrow dependency TrafficManager uses to write
// timeline rows, satisfied by *netcontrol.Manager (see its
// AddTimelineEventWithDetails).
type TimelineSink interface {
	AddTimelineEventWithDetails(netID, eventType, callsign, summary, details string) error
}

// Policy exposes the WP1 per-net ride policy without this package knowing
// its field names. app.go wires a NewNetProfilePolicy over *netcontrol.Manager;
// DefaultPolicy is the fallback when no per-net policy source is available.
type Policy interface {
	// WithholdSevereBib reports whether netID's ride config withholds the
	// bib number for severe-injury medical notifications (governing fact
	// 7). When true, Severity==SeveritySevere forces BibWithheld=true on
	// create.
	WithholdSevereBib(netID string) bool
	// ValidTier reports whether tier is in netID's priority-tier
	// vocabulary.
	ValidTier(netID, tier string) bool
}

// NetPolicySource is the narrow dependency NewNetProfilePolicy needs:
// the net's profile (to resolve the shipped tier ladder via
// internal/netprofile) and its ride config (for a tier override and the
// bib-withholding flag). Satisfied structurally by *netcontrol.Manager.
type NetPolicySource interface {
	NetLookup
	GetRideConfig(netID string) (*store.NetRideConfig, bool)
}

// netProfilePolicy adapts a NetPolicySource into a Policy.
type netProfilePolicy struct {
	src NetPolicySource
}

// NewNetProfilePolicy adapts src (namely *netcontrol.Manager) into a
// Policy. WithholdSevereBib reads store.NetRideConfig.WithholdBibOnSevereInjury
// (WP1); ValidTier resolves the net's effective ladder via
// netprofile.EffectiveTiers — the ride config's PriorityTiers override when
// set, otherwise the net's profile's shipped ladder.
func NewNetProfilePolicy(src NetPolicySource) Policy {
	return &netProfilePolicy{src: src}
}

func (p *netProfilePolicy) WithholdSevereBib(netID string) bool {
	cfg, ok := p.src.GetRideConfig(netID)
	if !ok || cfg == nil {
		return false
	}
	return cfg.WithholdBibOnSevereInjury
}

func (p *netProfilePolicy) ValidTier(netID, tier string) bool {
	tier = strings.ToLower(strings.TrimSpace(tier))
	if tier == "" {
		return false
	}
	n, ok := p.src.GetNet(netID)
	if !ok {
		return false
	}
	var override []store.PriorityTier
	if cfg, ok := p.src.GetRideConfig(netID); ok && cfg != nil {
		override = cfg.PriorityTiers
	}
	for _, t := range netprofile.EffectiveTiers(n.Profile, override) {
		if t.ID == tier {
			return true
		}
	}
	return false
}

// defaultPolicy is the fallback Policy: never withholds a bib, and accepts
// exactly the five Marin ARS tiers. Used when no per-net policy source is
// wired in.
type defaultPolicy struct{}

func (defaultPolicy) WithholdSevereBib(string) bool { return false }

func (defaultPolicy) ValidTier(_ string, tier string) bool {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case PriorityEmergency, PriorityPriority, PriorityHigh, PriorityMedium, PriorityLow:
		return true
	default:
		return false
	}
}

// DefaultPolicy returns the fallback Policy (see defaultPolicy).
func DefaultPolicy() Policy { return defaultPolicy{} }

// TrafficManager owns supply and medical traffic for ride-profile nets.
type TrafficManager struct {
	store    store.Store
	nets     NetLookup
	timeline TimelineSink
	act      activity.Logger // may be nil in tests
	policy   Policy
	now      func() time.Time // injectable clock for elapsed-time tests

	mu      sync.RWMutex
	supply  map[string][]store.SupplyRequest       // netID -> requests
	medical map[string][]store.MedicalNotification // netID -> notifications

	events chan Event
}

// NewTrafficManager creates a TrafficManager. A nil policy falls back to
// DefaultPolicy().
func NewTrafficManager(s store.Store, nets NetLookup, tl TimelineSink, act activity.Logger, policy Policy) *TrafficManager {
	if policy == nil {
		policy = DefaultPolicy()
	}
	return &TrafficManager{
		store:    s,
		nets:     nets,
		timeline: tl,
		act:      act,
		policy:   policy,
		now:      time.Now,
		supply:   make(map[string][]store.SupplyRequest),
		medical:  make(map[string][]store.MedicalNotification),
		events:   make(chan Event, 64),
	}
}

// Load loads every non-archived net's supply requests and medical
// notifications from the store.
func (m *TrafficManager) Load() error {
	nets, err := m.store.LoadNets()
	if err != nil {
		return fmt.Errorf("load nets: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range nets {
		if n.Status == netStatusArchived {
			continue
		}
		reqs, err := m.store.LoadSupplyRequests(n.ID)
		if err != nil {
			return fmt.Errorf("load supply requests for net %s: %w", n.ID, err)
		}
		for i := range reqs {
			normalizeSupply(&reqs[i])
		}
		m.supply[n.ID] = reqs

		meds, err := m.store.LoadMedicalNotifications(n.ID)
		if err != nil {
			return fmt.Errorf("load medical notifications for net %s: %w", n.ID, err)
		}
		m.medical[n.ID] = meds
	}
	return nil
}

// Events returns the events channel for WebSocket broadcast.
func (m *TrafficManager) Events() <-chan Event {
	return m.events
}

func (m *TrafficManager) emit(evt Event) {
	select {
	case m.events <- evt:
	default:
		log.Printf("[ride] traffic events channel full, dropping %s", evt.Type)
	}
}

// requireOpenNet is the shared "does this net exist, and is it open" guard.
// Supply and medical traffic only start once the net is actually open
// (unlike SAG, which also accepts a draft net).
func (m *TrafficManager) requireOpenNet(netID string) (*store.Net, error) {
	n, ok := m.nets.GetNet(netID)
	if !ok {
		return nil, fmt.Errorf("net %q not found", netID)
	}
	if n.Status != netStatusOpen {
		return nil, fmt.Errorf("net is not open")
	}
	return n, nil
}

// logTimeline writes a timeline row through the TimelineSink (netcontrol),
// so ride's traffic rows flow through the net's own already-running WS
// bridge exactly like every other netcontrol timeline entry. Errors are
// logged, not returned — a broken timeline write must never roll back a
// state transition that has already been persisted.
func (m *TrafficManager) logTimeline(netID, eventType, callsign, summary, details string) {
	if m.timeline == nil {
		return
	}
	if err := m.timeline.AddTimelineEventWithDetails(netID, eventType, callsign, summary, details); err != nil {
		log.Printf("[ride] save timeline event: %v", err)
	}
}

func (m *TrafficManager) logActivity(action activity.Action, a Actor, target, details string) {
	if m.act == nil {
		return
	}
	if err := m.act.Log(activity.Entry{
		Timestamp: m.now(),
		UserID:    a.UserID,
		UserName:  a.UserName,
		Action:    action,
		Target:    target,
		Details:   details,
	}); err != nil {
		log.Printf("[ride] log activity: %v", err)
	}
}

// trimFloat formats a float without a trailing ".0" for on-air-style
// summaries ("10 bags ice", not "10.0 bags ice").
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// formatDurationMinutes renders a duration to the nearest whole minute, e.g.
// "34m". Elapsed-time answers are always derived from stored timestamps at
// the point of formatting, never recomputed and stored back.
func formatDurationMinutes(d time.Duration) string {
	mins := int(d.Round(time.Minute) / time.Minute)
	if mins < 0 {
		mins = 0
	}
	return fmt.Sprintf("%dm", mins)
}

// TierSource resolves a net's own priority-tier vocabulary for the SAG
// manager. It reports ok=false when the net has no agency-supplied override,
// in which case the caller keeps its process-wide Config.Priorities. May be
// nil on a Manager, which has the same effect.
type TierSource interface {
	EffectivePriorities(netID string) (priorities []string, ok bool)
}

type netProfileTiers struct {
	src NetPolicySource
}

// NewNetProfileTiers adapts src (namely *netcontrol.Manager) into a
// TierSource, so SAG validates a request's priority against the same
// agency-configured ladder supply and medical traffic already use
// (governing fact 8: the priority vocabulary is DATA, not hardcoded).
//
// It deliberately reports ok=false unless the net's ride config carries an
// explicit PriorityTiers override. Without one, SAG keeps its shipped
// Config.Priorities rather than adopting the profile's ladder — that keeps
// general-profile nets, and bike-ride nets that never customized, behaving
// exactly as before.
func NewNetProfileTiers(src NetPolicySource) TierSource {
	return &netProfileTiers{src: src}
}

func (t *netProfileTiers) EffectivePriorities(netID string) ([]string, bool) {
	if t.src == nil {
		return nil, false
	}
	cfg, ok := t.src.GetRideConfig(netID)
	if !ok || cfg == nil || len(cfg.PriorityTiers) == 0 {
		return nil, false
	}
	n, ok := t.src.GetNet(netID)
	if !ok {
		return nil, false
	}
	tiers := netprofile.EffectiveTiers(n.Profile, cfg.PriorityTiers)
	out := make([]string, 0, len(tiers))
	for _, x := range tiers {
		out = append(out, x.ID)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
