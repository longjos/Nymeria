package ride

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/narvel/nymeria/internal/store"
)

// The following mirror netcontrol constants by literal value. ride cannot
// import netcontrol — CheckInSource is a narrow interface deliberately kept
// free of a concrete netcontrol dependency, the same reason store cannot
// import netprofile (see sqlite.go's migrateV25 comment). Each is pinned to
// its netcontrol counterpart by a test in manager_test.go.
const (
	catSAG            = "sag"
	checkInReleased   = "released"
	netStatusClosed   = "closed"
	netStatusArchived = "archived"
)

// TimelineEntryEventType is the WS message "type" ride uses for timeline
// rows. It writes its own store.NetEvent (so Details can carry
// privacy-scoped content, e.g. bibs only) and emits it on its own Events()
// channel rather than going through netcontrol.Manager, but under the same
// literal type netcontrol.EventTimelineEntry uses, so the existing timeline
// panel shows ride's rows unchanged. Pinned in manager_test.go.
const TimelineEntryEventType = "net_timeline_entry"

// Manager owns SAG (support-and-gear transport) requests and vehicles for
// every net. It holds a single vocabulary Config for the whole process (not
// per net) — a future net-profile integration may replace that with a
// per-net lookup; SetConfig is the hook for it.
type Manager struct {
	store    store.Store
	checkIns CheckInSource
	anns     AnnotationSource // may be nil: skips annotation validation
	tiers    TierSource       // may be nil: keeps the process-wide cfg.Priorities

	mu       sync.RWMutex
	cfg      Config
	requests map[string][]store.SAGRequest          // netID -> requests, sequence order
	vehicles map[string]map[string]store.SAGVehicle // netID -> checkInID -> vehicle

	events chan Event
}

// NewManager creates a Manager. An empty cfg (no Priorities) falls back to
// DefaultConfig().
func NewManager(s store.Store, ci CheckInSource, anns AnnotationSource, cfg Config) *Manager {
	if len(cfg.Priorities) == 0 {
		cfg = DefaultConfig()
	}
	return &Manager{
		store:    s,
		checkIns: ci,
		anns:     anns,
		cfg:      copyConfig(cfg),
		requests: make(map[string][]store.SAGRequest),
		vehicles: make(map[string]map[string]store.SAGVehicle),
		events:   make(chan Event, 64),
	}
}

// Load loads every non-archived net's SAG requests and vehicles from the
// store.
func (m *Manager) Load() error {
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
		reqs, err := m.store.LoadSAGRequests(n.ID)
		if err != nil {
			return fmt.Errorf("load sag requests for net %s: %w", n.ID, err)
		}
		for i := range reqs {
			normalize(&reqs[i])
		}
		m.requests[n.ID] = reqs

		vs, err := m.store.LoadSAGVehicles(n.ID)
		if err != nil {
			return fmt.Errorf("load sag vehicles for net %s: %w", n.ID, err)
		}
		vmap := make(map[string]store.SAGVehicle, len(vs))
		for _, v := range vs {
			vmap[v.CheckInID] = v
		}
		m.vehicles[n.ID] = vmap
	}
	return nil
}

// Events returns the events channel for WebSocket broadcast.
func (m *Manager) Events() <-chan Event {
	return m.events
}

// Config returns a copy of the current vocabulary config.
func (m *Manager) Config() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return copyConfig(m.cfg)
}

// SetConfig hot-swaps the vocabulary config. An empty Priorities list is
// invalid and is silently ignored, leaving the previous config in effect.
func (m *Manager) SetConfig(cfg Config) {
	if len(cfg.Priorities) == 0 {
		return
	}
	m.mu.Lock()
	m.cfg = copyConfig(cfg)
	m.mu.Unlock()
}

// SetTierSource wires in a per-net priority-tier vocabulary. Without it (or
// for a net whose agency configured no override) the process-wide
// Config.Priorities is used, exactly as before.
func (m *Manager) SetTierSource(ts TierSource) {
	m.mu.Lock()
	m.tiers = ts
	m.mu.Unlock()
}

// ConfigForNet returns the vocabulary config as it applies to one net: the
// process-wide Config, with Priorities replaced by the net's own
// agency-configured ladder when it has one (governing fact 8). When the
// shipped default course/stop priorities are not rungs of that ladder they
// are blanked, which makes an explicit priority required on create — the
// same contract supply traffic already has — rather than silently writing a
// tier outside the net's vocabulary.
func (m *Manager) ConfigForNet(netID string) Config {
	m.mu.RLock()
	cfg := copyConfig(m.cfg)
	ts := m.tiers
	m.mu.RUnlock()
	if ts == nil {
		return cfg
	}
	priorities, ok := ts.EffectivePriorities(netID)
	if !ok {
		return cfg
	}
	cfg.Priorities = priorities
	if !containsString(cfg.Priorities, cfg.DefaultCoursePriority) {
		cfg.DefaultCoursePriority = ""
	}
	if !containsString(cfg.Priorities, cfg.DefaultStopPriority) {
		cfg.DefaultStopPriority = ""
	}
	return cfg
}

// --- Requests ---

// CreateRequestInput is the input to CreateRequest.
type CreateRequestInput struct {
	Pickup      store.SAGLocation `json:"pickup"`
	Dropoff     store.SAGLocation `json:"dropoff"`
	Reason      string            `json:"reason"`
	Priority    string            `json:"priority"` // "" -> derived from Pickup.Kind
	Slots       []SlotInput       `json:"slots"`    // len 0 -> one anonymous slot is created
	RequestedBy string            `json:"requestedBy"`
	Notes       string            `json:"notes"`
	Division    *string           `json:"division,omitempty"`
}

// SlotInput is one rider on a create/add-slot/update-slot request.
//
// AttachToLegID is the answer to "SAG 4, make that TWO riders": it puts the
// new rider straight onto a leg that is already dispatched/en route/on
// scene, so the vehicle's committed seats move the moment the operator logs
// what was just said on the air. Without it the rider is added unassigned
// and the request goes back to needing a vehicle — which is the truth when
// the van has already loaded and driven off.
//
// AllowOvercommit covers the BIKE RACKS only; a rider with no seatbelt to
// sit in is refused outright (see OverCapacityError).
type SlotInput struct {
	Bib       string `json:"bib"`
	RiderName string `json:"riderName"`
	Note      string `json:"note"`
	HasBike   *bool  `json:"hasBike"`        // legacy; nil -> true. Bike wins when both are set.
	Bike      string `json:"bike,omitempty"` // ride.Bike*; "" -> derived from HasBike

	AttachToLegID   string `json:"attachToLegId,omitempty"` // AddSlot only
	AllowOvercommit bool   `json:"allowOvercommit,omitempty"`
}

// checkCapacity applies the asymmetric capacity rule shared by Dispatch and
// AddSlot's attach path. It returns (overcommitRacks, error):
//
//   - seats short  -> always an error, whatever allowOvercommit says. A seat
//     is a seatbelt; there is no operator judgement to exercise.
//   - racks short  -> an error unless allowOvercommit, in which case it
//     returns true and the caller flags the leg Overcommitted. A bike can
//     ride in the bed of a truck.
//
// Both dimensions are reported in the error so the UI can say what is short
// even when only one of them is fatal.
func checkCapacity(v store.SAGVehicle, committedSeats, committedRacks, needSeats, needRacks int, allowOvercommit bool) (bool, error) {
	// A dimension can only be "exceeded" by an action that actually adds to
	// it. Without the needX > 0 guard, a vehicle whose racks are ALREADY over
	// (from an earlier knowing override) would refuse a rider who has no bike
	// at all — a refusal the operator cannot act on and did not cause.
	seatsExceeded := needSeats > 0 && committedSeats+needSeats > v.Seats
	racksExceeded := needRacks > 0 && committedRacks+needRacks > v.RackSlots
	if !seatsExceeded && !racksExceeded {
		return false, nil
	}
	if seatsExceeded || !allowOvercommit {
		return false, &OverCapacityError{
			CommittedSeats: committedSeats, Seats: v.Seats, NeedSeats: needSeats,
			CommittedRacks: committedRacks, RackSlots: v.RackSlots, NeedRacks: needRacks,
			SeatsExceeded: seatsExceeded, RacksExceeded: racksExceeded,
		}
	}
	return true, nil
}

// bikeFromInput resolves a SlotInput's bike disposition, preferring the
// explicit axis over the legacy bool. Returns an error for an unrecognised
// value rather than silently storing it.
func bikeFromInput(in SlotInput) (string, error) {
	if in.Bike != "" {
		if !ValidBikeDispositions[in.Bike] {
			return "", fmt.Errorf("invalid bike disposition %q", in.Bike)
		}
		return in.Bike, nil
	}
	if in.HasBike != nil && !*in.HasBike {
		return BikeNone, nil
	}
	return BikeWithRider, nil
}

// LoadInput is what LoadSlots takes. Bike maps a slot id being loaded to its
// bike disposition AS REPORTED BY THE DRIVER at the scene — the only moment
// anybody actually knows whether the bike went on the rack.
type LoadInput struct {
	SlotIDs []string          `json:"slotIds"`
	Bike    map[string]string `json:"bike,omitempty"`
}

// UpdateRequestInput is the input to UpdateRequest: everything editable
// about a request short of its slots/legs.
type UpdateRequestInput struct {
	Pickup      store.SAGLocation `json:"pickup"`
	Dropoff     store.SAGLocation `json:"dropoff"`
	Reason      string            `json:"reason"`
	Priority    string            `json:"priority"`
	Notes       string            `json:"notes"`
	RequestedBy string            `json:"requestedBy"`
}

// DispatchInput is the input to Dispatch.
type DispatchInput struct {
	VehicleCheckInID string   `json:"vehicleCheckInId"`
	SlotIDs          []string `json:"slotIds"` // empty -> all currently waiting, un-legged slots
	AllowOvercommit  bool     `json:"allowOvercommit"`
}

// requireNet is the shared "does this net exist" guard. It is the
// authoritative check (not map-key presence in m.requests, which a net can
// legitimately be absent from — zero SAG requests ever created for it).
func (m *Manager) requireNet(netID string) (*store.Net, error) {
	n, ok := m.checkIns.GetNet(netID)
	if !ok {
		return nil, fmt.Errorf("net %q not found: %w", netID, ErrNotFound)
	}
	return n, nil
}

func (m *Manager) findCheckIn(netID, checkInID string) *store.NetCheckIn {
	cis := m.checkIns.GetCheckIns(netID)
	for i := range cis {
		if cis[i].ID == checkInID {
			ci := cis[i]
			return &ci
		}
	}
	return nil
}

func validateLocationAnnotation(anns AnnotationSource, loc store.SAGLocation) error {
	if loc.AnnotationID == "" || anns == nil {
		return nil
	}
	if _, ok := anns.Get(loc.AnnotationID); !ok {
		return fmt.Errorf("annotation %q not found", loc.AnnotationID)
	}
	return nil
}

func indexOfRequest(reqs []store.SAGRequest, id string) int {
	for i := range reqs {
		if reqs[i].ID == id {
			return i
		}
	}
	return -1
}

func findSlot(req *store.SAGRequest, slotID string) (*store.SAGSlot, bool) {
	for i := range req.Slots {
		if req.Slots[i].ID == slotID {
			return &req.Slots[i], true
		}
	}
	return nil, false
}

func findLeg(req *store.SAGRequest, legID string) (*store.SAGLeg, bool) {
	for i := range req.Legs {
		if req.Legs[i].ID == legID {
			return &req.Legs[i], true
		}
	}
	return nil, false
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func vehicleLabel(ci store.NetCheckIn) string {
	if ci.TacticalCall != "" {
		return ci.TacticalCall
	}
	return ci.Callsign
}

// recomputeStatus applies DeriveStatus to r, and sets/clears ClosedAt as the
// status enters or leaves complete|cancelled (a request that reopens via
// AddSlot has its ClosedAt cleared here, automatically).
func recomputeStatus(r *store.SAGRequest, now time.Time) {
	wasClosed := r.Status == ReqComplete || r.Status == ReqCancelled
	status, needsVehicle := DeriveStatus(*r)
	r.Status = status
	r.NeedsVehicle = needsVehicle
	isClosed := status == ReqComplete || status == ReqCancelled
	if isClosed && r.ClosedAt == nil {
		t := now
		r.ClosedAt = &t
	} else if !isClosed && wasClosed {
		r.ClosedAt = nil
	}
	r.UpdatedAt = now
}

func copyConfig(cfg Config) Config {
	out := cfg
	out.Priorities = append([]string(nil), cfg.Priorities...)
	out.Reasons = append([]string(nil), cfg.Reasons...)
	if out.Priorities == nil {
		out.Priorities = []string{}
	}
	if out.Reasons == nil {
		out.Reasons = []string{}
	}
	return out
}

// logTimeline persists a store.NetEvent and emits it on ride's own channel
// under TimelineEntryEventType, so the existing net timeline panel shows it
// with no frontend changes.
func (m *Manager) logTimeline(netID, eventType, callsign, summary, details string) {
	if details == "" {
		details = "{}"
	}
	evt := store.NetEvent{
		ID:        uuid.New().String(),
		NetID:     netID,
		Type:      eventType,
		Callsign:  callsign,
		Summary:   summary,
		Details:   details,
		CreatedAt: time.Now().UTC(),
	}
	if err := m.store.SaveNetEvent(evt); err != nil {
		log.Printf("[ride] save timeline event: %v", err)
	}
	m.emit(Event{Type: TimelineEntryEventType, Data: evt})
}

func (m *Manager) emit(evt Event) {
	select {
	case m.events <- evt:
	default:
	}
}

// CreateRequest creates a new SAG request. requestedBy defaults are the
// caller's responsibility (the HTTP handler fills it from the session user);
// this layer accepts whatever it is given.
func (m *Manager) CreateRequest(netID string, in CreateRequestInput, createdByName string) (*store.SAGRequest, error) {
	net, err := m.requireNet(netID)
	if err != nil {
		return nil, err
	}
	if net.Status == netStatusClosed || net.Status == netStatusArchived {
		return nil, fmt.Errorf("net is %s", net.Status)
	}

	if !ValidPickupKinds[in.Pickup.Kind] {
		return nil, fmt.Errorf("invalid pickup kind %q", in.Pickup.Kind)
	}
	if !ValidDropoffKinds[in.Dropoff.Kind] {
		return nil, fmt.Errorf("invalid dropoff kind %q", in.Dropoff.Kind)
	}
	if err := validateLocationAnnotation(m.anns, in.Pickup); err != nil {
		return nil, err
	}
	if err := validateLocationAnnotation(m.anns, in.Dropoff); err != nil {
		return nil, err
	}

	cfg := m.ConfigForNet(netID)
	priority := strings.TrimSpace(in.Priority)
	if priority == "" {
		priority = DefaultPriority(cfg, in.Pickup.Kind)
		if priority == "" {
			return nil, fmt.Errorf("priority is required: this net's ladder has no default for pickup kind %q", in.Pickup.Kind)
		}
	} else if !containsString(cfg.Priorities, priority) {
		return nil, fmt.Errorf("invalid priority %q", priority)
	}

	now := time.Now().UTC()
	var slots []store.SAGSlot
	if len(in.Slots) == 0 {
		slots = []store.SAGSlot{{
			ID: uuid.New().String(), Bike: BikeWithRider, HasBike: true, Disposition: SlotWaiting, UpdatedAt: now,
		}}
	} else {
		for _, si := range in.Slots {
			bike, err := bikeFromInput(si)
			if err != nil {
				return nil, err
			}
			slot := store.SAGSlot{
				ID: uuid.New().String(), Bib: si.Bib, RiderName: si.RiderName, Note: si.Note,
				Bike: bike, Disposition: SlotWaiting, UpdatedAt: now,
			}
			NormalizeSlotBike(&slot)
			slots = append(slots, slot)
		}
	}

	req := store.SAGRequest{
		ID:            uuid.New().String(),
		NetID:         netID,
		Division:      in.Division,
		Pickup:        in.Pickup,
		Dropoff:       in.Dropoff,
		Reason:        in.Reason,
		Priority:      priority,
		Slots:         slots,
		Legs:          []store.SAGLeg{},
		RequestedBy:   in.RequestedBy,
		CreatedByName: createdByName,
		Notes:         in.Notes,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	req.Sequence = len(reqs) + 1
	recomputeStatus(&req, now)
	m.requests[netID] = append(reqs, req)
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}

	bibs := []string{}
	for _, s := range req.Slots {
		if s.Bib != "" {
			bibs = append(bibs, s.Bib)
		}
	}
	details, _ := json.Marshal(map[string]any{"bibs": bibs})
	m.logTimeline(netID, TLSAGRequested, req.RequestedBy, fmt.Sprintf("SAG %d requested: %s", req.Sequence, req.Reason), string(details))
	m.emit(Event{Type: EventSAGRequestCreated, Data: req})

	out := req
	return &out, nil
}

// UpdateRequest updates a request's pickup/dropoff/reason/priority/notes/
// requestedBy. Refused once the request is complete or cancelled.
func (m *Manager) UpdateRequest(netID, reqID string, in UpdateRequestInput) (*store.SAGRequest, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}
	if !ValidPickupKinds[in.Pickup.Kind] {
		return nil, fmt.Errorf("invalid pickup kind %q", in.Pickup.Kind)
	}
	if !ValidDropoffKinds[in.Dropoff.Kind] {
		return nil, fmt.Errorf("invalid dropoff kind %q", in.Dropoff.Kind)
	}
	if err := validateLocationAnnotation(m.anns, in.Pickup); err != nil {
		return nil, err
	}
	if err := validateLocationAnnotation(m.anns, in.Dropoff); err != nil {
		return nil, err
	}

	cfg := m.ConfigForNet(netID)
	priority := strings.TrimSpace(in.Priority)
	if priority == "" {
		priority = DefaultPriority(cfg, in.Pickup.Kind)
		if priority == "" {
			return nil, fmt.Errorf("priority is required: this net's ladder has no default for pickup kind %q", in.Pickup.Kind)
		}
	} else if !containsString(cfg.Priorities, priority) {
		return nil, fmt.Errorf("invalid priority %q", priority)
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	if req.Status == ReqComplete || req.Status == ReqCancelled {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request is %s, cannot update", req.Status)
	}

	req.Pickup = in.Pickup
	req.Dropoff = in.Dropoff
	req.Reason = in.Reason
	req.Priority = priority
	req.Notes = in.Notes
	req.RequestedBy = in.RequestedBy

	now := time.Now().UTC()
	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	m.logTimeline(netID, TLSAGUpdated, req.RequestedBy, fmt.Sprintf("SAG %d updated", req.Sequence), "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// CancelRequest cancels a request. Refused if any slot is loaded (deliver
// or hand off loaded riders first). Waiting slots become cancelled and
// active legs are released; the request itself becomes cancelled.
func (m *Manager) CancelRequest(netID, reqID, reason, byCallsign string) (*store.SAGRequest, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])

	for _, s := range req.Slots {
		if s.Disposition == SlotLoaded {
			m.mu.Unlock()
			return nil, fmt.Errorf("deliver or hand off loaded riders first")
		}
	}

	now := time.Now().UTC()
	for i := range req.Slots {
		if req.Slots[i].Disposition == SlotWaiting {
			req.Slots[i].Disposition = SlotCancelled
			req.Slots[i].UpdatedAt = now
		}
	}
	for i := range req.Legs {
		if activeLegStatuses[req.Legs[i].Status] && req.Legs[i].Status != LegLoaded {
			req.Legs[i].Status = LegReleased
			req.Legs[i].ReleaseReason = "request cancelled"
			t := now
			req.Legs[i].ReleasedAt = &t
		}
	}
	req.CancelReason = reason

	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	m.logTimeline(netID, TLSAGCancelled, byCallsign, fmt.Sprintf("SAG %d cancelled: %s", req.Sequence, reason), "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// GetRequest returns a copy of one request.
func (m *Manager) GetRequest(netID, reqID string) (*store.SAGRequest, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		return nil, false
	}
	out := deepCopyRequest(reqs[idx])
	return &out, true
}

// GetRequests returns a copy of every request for a net, in sequence order.
// Never nil.
func (m *Manager) GetRequests(netID string) []store.SAGRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	reqs := m.requests[netID]
	out := make([]store.SAGRequest, len(reqs))
	for i, r := range reqs {
		out[i] = deepCopyRequest(r)
	}
	return out
}

// Board assembles GET /nets/{id}/sag.
func (m *Manager) Board(netID string) SAGBoard {
	reqs := m.GetRequests(netID)
	vehicles := m.Vehicles(netID)
	counts := make(map[string]int, len(allRequestStatuses))
	for _, s := range allRequestStatuses {
		counts[s] = 0
	}
	for _, r := range reqs {
		counts[r.Status]++
	}
	return SAGBoard{NetID: netID, Requests: reqs, Vehicles: vehicles, Counts: counts, Config: m.Config()}
}

// --- Slots ---

// AddSlot adds a rider to a request. Refused only if the request is
// cancelled (a terminal, record-preserving state); adding a slot to a
// complete request reopens it — "found a third rider" is a legitimate
// operational event, not an error.
func (m *Manager) AddSlot(netID, reqID string, in SlotInput) (*store.SAGRequest, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	if req.Status == ReqCancelled {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request is cancelled, cannot add a rider")
	}

	bike, err := bikeFromInput(in)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}

	now := time.Now().UTC()
	slot := store.SAGSlot{
		ID: uuid.New().String(), Bib: in.Bib, RiderName: in.RiderName, Note: in.Note,
		Bike: bike, Disposition: SlotWaiting, UpdatedAt: now,
	}
	NormalizeSlotBike(&slot)

	// "SAG 4, make that TWO riders at Maxwell." The van is already rolling,
	// so the rider joins its leg rather than sitting unassigned behind a
	// board that would then say nothing had changed.
	attached := ""
	if in.AttachToLegID != "" {
		leg, ok := findLeg(&req, in.AttachToLegID)
		if !ok {
			m.mu.Unlock()
			return nil, fmt.Errorf("leg %q not found: %w", in.AttachToLegID, ErrNotFound)
		}
		switch leg.Status {
		case LegDispatched, LegEnroute, LegOnScene:
		default:
			m.mu.Unlock()
			return nil, fmt.Errorf("%s is %s — it cannot take another rider; dispatch a vehicle instead", leg.VehicleLabel, leg.Status)
		}

		vehicle := m.vehicleOrDefaultLocked(netID, leg.VehicleCheckInID)
		allReqs := make([]store.SAGRequest, len(reqs))
		copy(allReqs, reqs)
		allReqs[idx] = req
		committedSeats, committedRacks, _, _ := Capacity(vehicle, allReqs)
		newRacks := 0
		if SlotTakesRack(slot) {
			newRacks = 1
		}
		overRacks, capErr := checkCapacity(vehicle, committedSeats, committedRacks, 1, newRacks, in.AllowOvercommit)
		if capErr != nil {
			m.mu.Unlock()
			return nil, capErr
		}
		if overRacks {
			leg.Overcommitted = true
		}

		slot.LegID = leg.ID
		leg.SlotIDs = append(leg.SlotIDs, slot.ID)
		attached = leg.VehicleLabel
	}

	req.Slots = append(req.Slots, slot)
	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	summary := fmt.Sprintf("SAG %d: rider added", req.Sequence)
	if attached != "" {
		summary += fmt.Sprintf(" to %s", attached)
	} else {
		summary += " — needs a vehicle"
	}
	m.logTimeline(netID, TLSAGUpdated, req.RequestedBy, summary, "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// UpdateSlot edits a slot's labels only (bib/rider name/note/hasBike); it
// never changes disposition — that is ResolveSlot's job.
func (m *Manager) UpdateSlot(netID, reqID, slotID string, in SlotInput) (*store.SAGRequest, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	slot, ok := findSlot(&req, slotID)
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("slot %q not found: %w", slotID, ErrNotFound)
	}

	if in.Bike != "" && !ValidBikeDispositions[in.Bike] {
		m.mu.Unlock()
		return nil, fmt.Errorf("invalid bike disposition %q", in.Bike)
	}

	now := time.Now().UTC()
	slot.Bib = in.Bib
	slot.RiderName = in.RiderName
	slot.Note = in.Note
	if in.Bike != "" {
		slot.Bike = in.Bike
	} else if in.HasBike != nil {
		slot.Bike = ""
		slot.HasBike = *in.HasBike
	}
	NormalizeSlotBike(slot)
	slot.UpdatedAt = now
	req.UpdatedAt = now
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// ResolveSlot moves a slot to one of the exception dispositions
// (self_resolved/declined/not_found/handed_off/cancelled). If that empties
// out the leg the slot was on (no waiting slots left on a not-yet-loaded
// leg, or no loaded slots left on a loaded leg), the leg is settled too:
// auto-released or auto-delivered respectively.
func (m *Manager) ResolveSlot(netID, reqID, slotID, disposition, note, byCallsign string) (*store.SAGRequest, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	slot, ok := findSlot(&req, slotID)
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("slot %q not found: %w", slotID, ErrNotFound)
	}
	if !SlotCanResolve(slot.Disposition, disposition) {
		m.mu.Unlock()
		return nil, fmt.Errorf("cannot resolve slot from %q to %q", slot.Disposition, disposition)
	}

	now := time.Now().UTC()
	legID := slot.LegID
	bib := slot.Bib
	slot.Disposition = disposition
	if note != "" {
		slot.Note = note
	}
	slot.UpdatedAt = now

	settledSummary := ""
	if legID != "" {
		if leg, ok := findLeg(&req, legID); ok {
			switch leg.Status {
			case LegLoaded:
				anyLoaded := false
				for _, sid := range leg.SlotIDs {
					if s, ok := findSlot(&req, sid); ok && s.Disposition == SlotLoaded {
						anyLoaded = true
						break
					}
				}
				if !anyLoaded {
					leg.Status = LegDelivered
					t := now
					leg.DeliveredAt = &t
					settledSummary = fmt.Sprintf("; %s delivered (all riders resolved)", leg.VehicleLabel)
				}
			case LegDispatched, LegEnroute, LegOnScene:
				anyWaiting := false
				for _, sid := range leg.SlotIDs {
					if s, ok := findSlot(&req, sid); ok && s.Disposition == SlotWaiting {
						anyWaiting = true
						break
					}
				}
				if !anyWaiting {
					leg.Status = LegReleased
					leg.ReleaseReason = "all riders resolved"
					t := now
					leg.ReleasedAt = &t
					settledSummary = fmt.Sprintf("; %s released (all riders resolved)", leg.VehicleLabel)
				}
			}
		}
	}

	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	summary := fmt.Sprintf("rider -> %s", disposition)
	if bib != "" {
		summary = fmt.Sprintf("bib %s -> %s", bib, disposition)
	}
	m.logTimeline(netID, TLSAGSlotResolved, byCallsign, summary+settledSummary, "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// --- Legs ---

// Dispatch assigns a vehicle to some or all of a request's currently
// waiting, unassigned slots, creating a new leg. Capacity is checked across
// every request the vehicle is already committed to in the net, and the two
// dimensions are NOT symmetric (see OverCapacityError): too many riders for
// the seatbelts is refused outright and AllowOvercommit does not open it;
// too many bikes for the racks is refused unless AllowOvercommit is set, in
// which case the leg is created and flagged Overcommitted.
func (m *Manager) Dispatch(netID, reqID string, in DispatchInput, byCallsign string) (*store.SAGRequest, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	ci := m.findCheckIn(netID, in.VehicleCheckInID)
	if ci == nil {
		return nil, fmt.Errorf("vehicle check-in %q not found: %w", in.VehicleCheckInID, ErrNotFound)
	}
	if ci.Category != catSAG {
		return nil, fmt.Errorf("check-in %q is not a sag vehicle", in.VehicleCheckInID)
	}
	if ci.Status == checkInReleased {
		return nil, fmt.Errorf("vehicle %q has been released", in.VehicleCheckInID)
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	if req.Status == ReqComplete || req.Status == ReqCancelled {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request is %s", req.Status)
	}

	var targetIDs []string
	if len(in.SlotIDs) == 0 {
		for _, s := range req.Slots {
			if s.Disposition == SlotWaiting && s.LegID == "" {
				targetIDs = append(targetIDs, s.ID)
			}
		}
	} else {
		for _, sid := range in.SlotIDs {
			slot, ok := findSlot(&req, sid)
			if !ok || slot.Disposition != SlotWaiting || slot.LegID != "" {
				m.mu.Unlock()
				return nil, fmt.Errorf("slot %q is not waiting and unassigned", sid)
			}
			targetIDs = append(targetIDs, sid)
		}
	}
	if len(targetIDs) == 0 {
		m.mu.Unlock()
		return nil, fmt.Errorf("no waiting riders to dispatch")
	}

	vehicle := m.vehicleOrDefaultLocked(netID, in.VehicleCheckInID)
	allReqs := make([]store.SAGRequest, len(reqs))
	copy(allReqs, reqs)
	allReqs[idx] = req
	committedSeats, committedRacks, _, _ := Capacity(vehicle, allReqs)

	newRacks := 0
	for _, sid := range targetIDs {
		if slot, ok := findSlot(&req, sid); ok && slot.HasBike {
			newRacks++
		}
	}
	newSeats := len(targetIDs)

	overCommitted, capErr := checkCapacity(vehicle, committedSeats, committedRacks, newSeats, newRacks, in.AllowOvercommit)
	if capErr != nil {
		m.mu.Unlock()
		return nil, capErr
	}

	now := time.Now().UTC()
	leg := store.SAGLeg{
		ID:               uuid.New().String(),
		VehicleCheckInID: ci.ID,
		VehicleLabel:     vehicleLabel(*ci),
		SlotIDs:          append([]string(nil), targetIDs...),
		Status:           LegDispatched,
		Overcommitted:    overCommitted,
		DispatchedAt:     now,
	}
	for _, sid := range targetIDs {
		if slot, ok := findSlot(&req, sid); ok {
			slot.LegID = leg.ID
			slot.UpdatedAt = now
		}
	}
	req.Legs = append(req.Legs, leg)
	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	summary := fmt.Sprintf("%s dispatched to SAG %d (%d rider(s))", leg.VehicleLabel, req.Sequence, len(targetIDs))
	if overCommitted {
		// Overcommitted can only ever mean bikes now — a seat overflow is
		// refused outright — so the row says which, not "over capacity".
		summary += " — BIKES OVER RACK CAPACITY"
	}
	m.logTimeline(netID, TLSAGDispatched, byCallsign, summary, "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// AdvanceLeg moves a leg to enroute or onscene. loaded/delivered/released
// each have a dedicated verb (LoadSlots/DeliverSlots/ReleaseLeg) so their
// side effects on slots and capacity cannot be skipped.
func (m *Manager) AdvanceLeg(netID, reqID, legID, status, byCallsign string) (*store.SAGRequest, error) {
	if status != LegEnroute && status != LegOnScene {
		return nil, fmt.Errorf("invalid leg status %q for AdvanceLeg (use the dedicated verb for loaded/delivered/released)", status)
	}
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	leg, ok := findLeg(&req, legID)
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("leg %q not found: %w", legID, ErrNotFound)
	}
	if !LegCanTransition(leg.Status, status) {
		m.mu.Unlock()
		return nil, fmt.Errorf("cannot transition leg from %q to %q", leg.Status, status)
	}

	now := time.Now().UTC()
	leg.Status = status
	switch status {
	case LegEnroute:
		t := now
		leg.EnrouteAt = &t
	case LegOnScene:
		t := now
		leg.OnSceneAt = &t
	}
	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	evtType := TLSAGEnroute
	if status == LegOnScene {
		evtType = TLSAGOnScene
	}
	m.logTimeline(netID, evtType, byCallsign, fmt.Sprintf("%s %s", leg.VehicleLabel, status), "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// LoadSlots marks slots as physically aboard the vehicle. An empty slotIDs
// means every slot currently on the leg; slots on the leg that are NOT
// listed detach back to waiting (their legId is cleared) — the leg keeps
// only whoever was actually loaded, so the rest can be dispatched to a
// different vehicle.
func (m *Manager) LoadSlots(netID, reqID, legID string, in LoadInput, byCallsign string) (*store.SAGRequest, error) {
	slotIDs := in.SlotIDs
	for _, b := range in.Bike {
		if !ValidBikeDispositions[b] {
			return nil, fmt.Errorf("invalid bike disposition %q", b)
		}
	}
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	leg, ok := findLeg(&req, legID)
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("leg %q not found: %w", legID, ErrNotFound)
	}
	if leg.Status == LegDelivered || leg.Status == LegReleased {
		m.mu.Unlock()
		return nil, fmt.Errorf("leg is %s, cannot load", leg.Status)
	}

	targetSet := make(map[string]bool, len(leg.SlotIDs))
	if len(slotIDs) == 0 {
		for _, sid := range leg.SlotIDs {
			targetSet[sid] = true
		}
	} else {
		onLeg := make(map[string]bool, len(leg.SlotIDs))
		for _, sid := range leg.SlotIDs {
			onLeg[sid] = true
		}
		for _, sid := range slotIDs {
			if !onLeg[sid] {
				m.mu.Unlock()
				return nil, fmt.Errorf("slot %q is not on leg %q", sid, legID)
			}
			targetSet[sid] = true
		}
	}

	// A bike disposition may only be reported for a rider actually being
	// loaded onto this leg: anything else is a mis-click that would silently
	// rewrite an unrelated rider's record.
	for sid := range in.Bike {
		if !targetSet[sid] {
			m.mu.Unlock()
			return nil, fmt.Errorf("slot %q is not being loaded onto leg %q", sid, legID)
		}
	}

	now := time.Now().UTC()
	total := len(leg.SlotIDs)
	loadedCount := 0
	bikesCarried, bikesLeft, bikesSeparate := 0, 0, 0
	keptSlotIDs := []string{}
	for _, sid := range leg.SlotIDs {
		slot, _ := findSlot(&req, sid)
		if targetSet[sid] {
			if slot != nil {
				slot.Disposition = SlotLoaded
				if b, ok := in.Bike[sid]; ok {
					slot.Bike = b
				}
				NormalizeSlotBike(slot)
				switch slot.Bike {
				case BikeWithRider:
					bikesCarried++
				case BikeLeftBehind:
					bikesLeft++
				case BikeOtherVehicle:
					bikesSeparate++
				}
				slot.UpdatedAt = now
			}
			keptSlotIDs = append(keptSlotIDs, sid)
			loadedCount++
		} else if slot != nil {
			slot.LegID = ""
			slot.UpdatedAt = now
		}
	}
	leg.SlotIDs = keptSlotIDs
	if leg.LoadedAt == nil {
		t := now
		leg.LoadedAt = &t
	}
	leg.Status = LegLoaded

	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	// What happened to the bikes belongs in the log: a bike left at a rest
	// stop is somebody's property and an ICS-214 line item, and a bike
	// travelling apart from its rider is a second thing to reunite later.
	loadSummary := fmt.Sprintf("%s loaded %d of %d", leg.VehicleLabel, loadedCount, total)
	bikeParts := []string{}
	if bikesCarried > 0 {
		bikeParts = append(bikeParts, fmt.Sprintf("%d bike(s) on the rack", bikesCarried))
	}
	if bikesLeft > 0 {
		bikeParts = append(bikeParts, fmt.Sprintf("%d bike left behind", bikesLeft))
	}
	if bikesSeparate > 0 {
		bikeParts = append(bikeParts, fmt.Sprintf("%d bike on another vehicle", bikesSeparate))
	}
	if len(bikeParts) > 0 {
		loadSummary += "; " + strings.Join(bikeParts, ", ")
	}
	m.logTimeline(netID, TLSAGLoaded, byCallsign, loadSummary, "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// DeliverSlots marks loaded slots as delivered to a destination (the
// request's own Dropoff when dest is nil). An empty slotIDs means every
// currently loaded slot on the leg. The leg becomes delivered once none of
// its slots are loaded any more (partial delivery keeps it loaded).
func (m *Manager) DeliverSlots(netID, reqID, legID string, slotIDs []string, dest *store.SAGLocation, byCallsign string) (*store.SAGRequest, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	leg, ok := findLeg(&req, legID)
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("leg %q not found: %w", legID, ErrNotFound)
	}
	if leg.Status != LegLoaded {
		m.mu.Unlock()
		return nil, fmt.Errorf("leg is %s, not loaded", leg.Status)
	}

	destination := req.Dropoff
	if dest != nil {
		destination = *dest
	}

	var targetIDs []string
	if len(slotIDs) == 0 {
		for _, sid := range leg.SlotIDs {
			if slot, ok := findSlot(&req, sid); ok && slot.Disposition == SlotLoaded {
				targetIDs = append(targetIDs, sid)
			}
		}
	} else {
		onLeg := make(map[string]bool, len(leg.SlotIDs))
		for _, sid := range leg.SlotIDs {
			onLeg[sid] = true
		}
		for _, sid := range slotIDs {
			slot, ok := findSlot(&req, sid)
			if !ok || !onLeg[sid] || slot.Disposition != SlotLoaded {
				m.mu.Unlock()
				return nil, fmt.Errorf("slot %q is not a loaded slot on leg %q", sid, legID)
			}
			targetIDs = append(targetIDs, sid)
		}
	}
	if len(targetIDs) == 0 {
		m.mu.Unlock()
		return nil, fmt.Errorf("no loaded riders to deliver")
	}

	cfg := m.cfg
	if cfg.RequireNameForHospitalStart && (destination.Kind == LocHospital || destination.Kind == LocStart) {
		for _, sid := range targetIDs {
			slot, _ := findSlot(&req, sid)
			if strings.TrimSpace(slot.RiderName) == "" {
				m.mu.Unlock()
				return nil, fmt.Errorf("rider name required for hospital/start transport")
			}
		}
	}

	now := time.Now().UTC()
	for _, sid := range targetIDs {
		slot, _ := findSlot(&req, sid)
		slot.Disposition = SlotDelivered
		d := destination
		slot.DeliveredTo = &d
		slot.UpdatedAt = now
	}

	anyLoaded := false
	for _, sid := range leg.SlotIDs {
		if slot, ok := findSlot(&req, sid); ok && slot.Disposition == SlotLoaded {
			anyLoaded = true
			break
		}
	}
	if !anyLoaded {
		leg.Status = LegDelivered
		t := now
		leg.DeliveredAt = &t
	}

	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	m.logTimeline(netID, TLSAGDelivered, byCallsign, fmt.Sprintf("%s delivered %d rider(s) to %s", leg.VehicleLabel, len(targetIDs), destination.Kind), "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// ReleaseLeg frees a vehicle before it has loaded anyone. Only valid from
// dispatched/enroute/onscene — a loaded leg must be delivered or handed off
// first, never "unloaded".
func (m *Manager) ReleaseLeg(netID, reqID, legID, reason, byCallsign string) (*store.SAGRequest, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	idx := indexOfRequest(reqs, reqID)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("sag request %q not found: %w", reqID, ErrNotFound)
	}
	req := deepCopyRequest(reqs[idx])
	leg, ok := findLeg(&req, legID)
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("leg %q not found: %w", legID, ErrNotFound)
	}
	if leg.Status != LegDispatched && leg.Status != LegEnroute && leg.Status != LegOnScene {
		m.mu.Unlock()
		return nil, fmt.Errorf("leg is %s, cannot release (deliver or hand off loaded riders first)", leg.Status)
	}

	now := time.Now().UTC()
	for _, sid := range leg.SlotIDs {
		if slot, ok := findSlot(&req, sid); ok && slot.Disposition == SlotWaiting {
			slot.LegID = ""
			slot.UpdatedAt = now
		}
	}
	leg.Status = LegReleased
	leg.ReleaseReason = reason
	t := now
	leg.ReleasedAt = &t

	recomputeStatus(&req, now)
	reqs[idx] = req
	m.requests[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSAGRequest(req); err != nil {
		return nil, fmt.Errorf("persist sag request: %w", err)
	}
	summary := fmt.Sprintf("%s released", leg.VehicleLabel)
	if reason != "" {
		summary += ": " + reason
	}
	m.logTimeline(netID, TLSAGReleased, byCallsign, summary, "")
	m.emitRequestChanged(netID, req)

	out := req
	return &out, nil
}

// --- Vehicles ---

// vehicleOrDefaultLocked assumes m.mu is already held (read or write).
func (m *Manager) vehicleOrDefaultLocked(netID, checkInID string) store.SAGVehicle {
	if vm, ok := m.vehicles[netID]; ok {
		if v, ok := vm[checkInID]; ok {
			return v
		}
	}
	return store.SAGVehicle{NetID: netID, CheckInID: checkInID, Seats: m.cfg.DefaultSeats, RackSlots: m.cfg.DefaultRackSlots}
}

func (m *Manager) vehicleOrDefault(netID, checkInID string) store.SAGVehicle {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.vehicleOrDefaultLocked(netID, checkInID)
}

func (m *Manager) vehicleStatusFor(netID string, ci store.NetCheckIn, v store.SAGVehicle) SAGVehicleStatus {
	reqs := m.GetRequests(netID)
	committedSeats, committedRacks, activeLegIDs, activeRequestIDs := Capacity(v, reqs)
	return SAGVehicleStatus{
		SAGVehicle:       v,
		Callsign:         ci.Callsign,
		TacticalCall:     ci.TacticalCall,
		CheckInStatus:    ci.Status,
		CommittedSeats:   committedSeats,
		CommittedRacks:   committedRacks,
		AvailableSeats:   v.Seats - committedSeats,
		AvailableRacks:   v.RackSlots - committedRacks,
		ActiveLegIDs:     activeLegIDs,
		ActiveRequestIDs: activeRequestIDs,
	}
}

// SetVehicle configures a vehicle's capacity. The check-in must exist and be
// Category "sag".
func (m *Manager) SetVehicle(netID, checkInID string, seats, rackSlots int, notes string) (*SAGVehicleStatus, error) {
	if _, err := m.requireNet(netID); err != nil {
		return nil, err
	}
	if seats < 0 || rackSlots < 0 {
		return nil, fmt.Errorf("seats and rackSlots must be >= 0")
	}
	ci := m.findCheckIn(netID, checkInID)
	if ci == nil {
		return nil, fmt.Errorf("check-in %q not found: %w", checkInID, ErrNotFound)
	}
	if ci.Category != catSAG {
		return nil, fmt.Errorf("check-in %q is not a sag vehicle", checkInID)
	}

	now := time.Now().UTC()
	v := store.SAGVehicle{NetID: netID, CheckInID: checkInID, Seats: seats, RackSlots: rackSlots, Notes: notes, UpdatedAt: now}

	m.mu.Lock()
	if m.vehicles[netID] == nil {
		m.vehicles[netID] = make(map[string]store.SAGVehicle)
	}
	m.vehicles[netID][checkInID] = v
	m.mu.Unlock()

	if err := m.store.SaveSAGVehicle(v); err != nil {
		return nil, fmt.Errorf("persist sag vehicle: %w", err)
	}

	status := m.vehicleStatusFor(netID, *ci, v)
	m.emit(Event{Type: EventSAGVehicleUpdated, Data: status})
	return &status, nil
}

// emitRequestChanged emits the request update plus a vehicle update for
// every vehicle the request's legs touch. A vehicle's committed seats and
// racks are DERIVED from live leg/slot state and never stored, so any leg
// or slot change silently changes the capacity of the vehicles involved.
// Emitting only the request would leave an already-loaded board showing
// stale capacity until its next full GET /sag. Call this AFTER releasing
// m.mu — vehicleStatusFor takes the read lock.
func (m *Manager) emitRequestChanged(netID string, req store.SAGRequest) {
	m.emit(Event{Type: EventSAGRequestUpdated, Data: req})
	m.emitVehiclesFor(netID, req)
}

// emitVehiclesFor emits a vehicle update for each distinct vehicle
// referenced by req's legs. Must be called with m.mu released.
func (m *Manager) emitVehiclesFor(netID string, req store.SAGRequest) {
	seen := make(map[string]bool, len(req.Legs))
	for _, leg := range req.Legs {
		if leg.VehicleCheckInID == "" || seen[leg.VehicleCheckInID] {
			continue
		}
		seen[leg.VehicleCheckInID] = true
		ci := m.findCheckIn(netID, leg.VehicleCheckInID)
		if ci == nil {
			continue
		}
		v := m.vehicleOrDefault(netID, leg.VehicleCheckInID)
		m.emit(Event{Type: EventSAGVehicleUpdated, Data: m.vehicleStatusFor(netID, *ci, v)})
	}
}

// VehicleStatus returns one vehicle's live status. ok is false only if the
// check-in itself does not exist.
func (m *Manager) VehicleStatus(netID, checkInID string) (*SAGVehicleStatus, bool) {
	ci := m.findCheckIn(netID, checkInID)
	if ci == nil {
		return nil, false
	}
	v := m.vehicleOrDefault(netID, checkInID)
	status := m.vehicleStatusFor(netID, *ci, v)
	return &status, true
}

// Vehicles returns every sag-category check-in's live status, registered or
// not (unregistered ones get cfg's defaults). Never nil.
func (m *Manager) Vehicles(netID string) []SAGVehicleStatus {
	cis := m.checkIns.GetCheckIns(netID)
	out := make([]SAGVehicleStatus, 0, len(cis))
	for _, ci := range cis {
		if ci.Category != catSAG {
			continue
		}
		v := m.vehicleOrDefault(netID, ci.ID)
		out = append(out, m.vehicleStatusFor(netID, ci, v))
	}
	return out
}

// ReleaseVehicle is called when a sag vehicle checks out of the net: every
// dispatched/enroute/onscene leg on it is released (its waiting slots
// detach back to open); a loaded leg is left alone but logs a warning
// timeline row, since the riders are still physically aboard. Returns every
// request that had a leg on this vehicle, touched or not.
func (m *Manager) ReleaseVehicle(netID, checkInID, reason string) []store.SAGRequest {
	type change struct {
		req     store.SAGRequest
		touched bool
		warn    bool
	}

	m.mu.Lock()
	reqs := m.requests[netID]
	now := time.Now().UTC()
	var changes []change
	for i := range reqs {
		reqCopy := deepCopyRequest(reqs[i])
		touched := false
		warn := false
		for li := range reqCopy.Legs {
			leg := &reqCopy.Legs[li]
			if leg.VehicleCheckInID != checkInID {
				continue
			}
			switch leg.Status {
			case LegDispatched, LegEnroute, LegOnScene:
				for _, sid := range leg.SlotIDs {
					if s, ok := findSlot(&reqCopy, sid); ok && s.Disposition == SlotWaiting {
						s.LegID = ""
						s.UpdatedAt = now
					}
				}
				leg.Status = LegReleased
				leg.ReleaseReason = reason
				t := now
				leg.ReleasedAt = &t
				touched = true
			case LegLoaded:
				warn = true
			}
		}
		if !touched && !warn {
			continue
		}
		if touched {
			recomputeStatus(&reqCopy, now)
			reqs[i] = reqCopy
		}
		changes = append(changes, change{req: reqCopy, touched: touched, warn: warn})
	}
	if len(changes) > 0 {
		m.requests[netID] = reqs
	}
	m.mu.Unlock()

	affected := make([]store.SAGRequest, 0, len(changes))
	for _, c := range changes {
		if c.touched {
			if err := m.store.SaveSAGRequest(c.req); err != nil {
				log.Printf("[ride] persist sag request on vehicle release: %v", err)
			}
			m.logTimeline(netID, TLSAGReleased, "", fmt.Sprintf("SAG %d: vehicle checked out (%s)", c.req.Sequence, reason), "")
			m.emit(Event{Type: EventSAGRequestUpdated, Data: c.req})
		}
		if c.warn {
			m.logTimeline(netID, TLSAGUpdated, "", fmt.Sprintf("SAG %d: vehicle checked out with riders still aboard", c.req.Sequence), "")
		}
		affected = append(affected, c.req)
	}

	if len(changes) > 0 {
		if ci := m.findCheckIn(netID, checkInID); ci != nil {
			v := m.vehicleOrDefault(netID, checkInID)
			status := m.vehicleStatusFor(netID, *ci, v)
			m.emit(Event{Type: EventSAGVehicleUpdated, Data: status})
		}
	}

	return affected
}
