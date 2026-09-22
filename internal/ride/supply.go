package ride

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/store"
)

// CreateSupplyInput is the input to CreateSupplyRequest.
type CreateSupplyInput struct {
	RequestedByCheckInID string             `json:"requestedByCheckInId"`
	RequestedByCall      string             `json:"requestedByCall"`
	Location             string             `json:"location"`
	LocationAnnotationID string             `json:"locationAnnotationId"`
	MilesRemaining       *float64           `json:"milesRemaining"`
	RouteID              string             `json:"routeId"`
	Lat                  *float64           `json:"lat"`
	Lon                  *float64           `json:"lon"`
	Items                []store.SupplyItem `json:"items"`
	AskedWhatElse        bool               `json:"askedWhatElse"`
	Priority             string             `json:"priority"`
	Notes                string             `json:"notes"`
	Division             *string            `json:"division"`
}

// AddItemsInput is the input to AddSupplyItems.
type AddItemsInput struct {
	Items         []store.SupplyItem `json:"items"`
	AskedWhatElse *bool              `json:"askedWhatElse"` // lets the "what else?" prompt flip the flag on append
}

// ReadbackInput is the input to ReadbackSupply/ReadbackMedical.
type ReadbackInput struct {
	Confirmed  bool   `json:"confirmed"` // false = requester corrected; stays draft, Notes appended
	ReadBackBy string `json:"readBackBy"`
	Correction string `json:"correction"`
}

// RelayInput is the input to RelaySupply.
type RelayInput struct {
	RelayedTo string `json:"relayedTo"`
}

// ETAInput is the input to RecordSupplyETA.
type ETAInput struct {
	Minutes int    `json:"minutes"`
	Source  string `json:"source"`
}

// CancelInput is the input to CancelSupply/CancelMedical.
type CancelInput struct {
	Reason string `json:"reason"`
}

// SupplyCatalogEntry is a suggestion row for the composer, NOT a
// constraint. The shipped defaults encode the load-bearing tier
// distinction (governing fact 10): out-of-water is HIGH, running-low is
// MEDIUM.
type SupplyCatalogEntry struct {
	Item        string `json:"item"`
	DefaultTier string `json:"defaultTier"`
	When        string `json:"when,omitempty"` // "out of water" vs "running low"
}

// terminalSupplyStatuses are the statuses a supply request never leaves.
var terminalSupplyStatuses = map[string]bool{
	store.SupplyDelivered: true,
	store.SupplyCancelled: true,
	store.SupplyMerged:    true,
}

func indexOfSupply(reqs []store.SupplyRequest, id string) int {
	for i := range reqs {
		if reqs[i].ID == id {
			return i
		}
	}
	return -1
}

// normalizeSupply turns nil Items/ETAs into empty slices in place. Called
// on both create and load, per the project's nil-slice-freezes-the-UI rule.
func normalizeSupply(r *store.SupplyRequest) {
	if r == nil {
		return
	}
	if r.Items == nil {
		r.Items = []store.SupplyItem{}
	}
	if r.ETAs == nil {
		r.ETAs = []store.SupplyETA{}
	}
}

func deepCopySupply(r store.SupplyRequest) store.SupplyRequest {
	out := r
	if r.Division != nil {
		d := *r.Division
		out.Division = &d
	}
	if r.MilesRemaining != nil {
		v := *r.MilesRemaining
		out.MilesRemaining = &v
	}
	if r.Lat != nil {
		v := *r.Lat
		out.Lat = &v
	}
	if r.Lon != nil {
		v := *r.Lon
		out.Lon = &v
	}
	out.Items = append([]store.SupplyItem(nil), r.Items...)
	out.ETAs = append([]store.SupplyETA(nil), r.ETAs...)
	normalizeSupply(&out)
	return out
}

// formatSupplyItem renders one item for an on-air-style summary: "10 bags
// ice", "2 cases water", or just "tubes" when quantity is unspecified.
func formatSupplyItem(it store.SupplyItem) string {
	if it.Quantity > 0 {
		if it.Unit != "" {
			return fmt.Sprintf("%s %s %s", trimFloat(it.Quantity), it.Unit, it.Item)
		}
		return fmt.Sprintf("%s %s", trimFloat(it.Quantity), it.Item)
	}
	return it.Item
}

func formatSupplyItems(items []store.SupplyItem) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, formatSupplyItem(it))
	}
	return strings.Join(parts, ", ")
}

func (m *TrafficManager) findSupplyLocked(netID, id string) ([]store.SupplyRequest, int) {
	reqs := m.supply[netID]
	return reqs, indexOfSupply(reqs, id)
}

// GetSupplyRequest returns a copy of one supply request.
func (m *TrafficManager) GetSupplyRequest(netID, id string) (*store.SupplyRequest, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	reqs, idx := m.findSupplyLocked(netID, id)
	if idx == -1 {
		return nil, false
	}
	out := deepCopySupply(reqs[idx])
	return &out, true
}

// GetSupplyRequests returns a copy of every supply request for a net,
// ordered by CreatedAt ascending. Never nil.
func (m *TrafficManager) GetSupplyRequests(netID string) []store.SupplyRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	reqs := m.supply[netID]
	out := make([]store.SupplyRequest, len(reqs))
	for i, r := range reqs {
		out[i] = deepCopySupply(r)
	}
	return out
}

// SupplyCatalog returns the composer's suggestion list. v1 ships fixed
// defaults; a later agency-config override is an open question, not yet
// wired to netID.
func (m *TrafficManager) SupplyCatalog(_ string) []SupplyCatalogEntry {
	return []SupplyCatalogEntry{
		{Item: "water", DefaultTier: PriorityHigh, When: "out of water"},
		{Item: "water", DefaultTier: PriorityMedium, When: "running low"},
		{Item: "ice", DefaultTier: PriorityMedium},
		{Item: "food", DefaultTier: PriorityMedium},
		{Item: "cups", DefaultTier: PriorityLow},
		{Item: "tubes", DefaultTier: PriorityLow},
		{Item: "co2/pump", DefaultTier: PriorityLow},
		{Item: "portable toilet service", DefaultTier: PriorityMedium},
		{Item: "first aid resupply", DefaultTier: PriorityHigh},
	}
}

// CreateSupplyRequest creates a new draft supply request. Items must be
// non-empty at creation — a request with nothing on it is not a request
// yet, it is a composer draft that has not been sent.
func (m *TrafficManager) CreateSupplyRequest(netID string, in CreateSupplyInput, a Actor) (*store.SupplyRequest, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.RequestedByCall) == "" {
		return nil, fmt.Errorf("requestedByCall is required")
	}
	if len(in.Items) == 0 {
		return nil, fmt.Errorf("at least one item is required")
	}
	priority := strings.ToLower(strings.TrimSpace(in.Priority))
	if !m.policy.ValidTier(netID, priority) {
		return nil, fmt.Errorf("invalid priority %q", in.Priority)
	}

	now := m.now()
	items := make([]store.SupplyItem, len(in.Items))
	copy(items, in.Items)
	for i := range items {
		if items[i].AddedAt.IsZero() {
			items[i].AddedAt = now
		}
	}

	req := store.SupplyRequest{
		ID:                   uuid.New().String(),
		NetID:                netID,
		Division:             in.Division,
		RequestedByCheckInID: in.RequestedByCheckInID,
		RequestedByCall:      in.RequestedByCall,
		Location:             in.Location,
		LocationAnnotationID: in.LocationAnnotationID,
		MilesRemaining:       in.MilesRemaining,
		RouteID:              in.RouteID,
		Lat:                  in.Lat,
		Lon:                  in.Lon,
		Items:                items,
		AskedWhatElse:        in.AskedWhatElse,
		Priority:             priority,
		Notes:                in.Notes,
		Status:               store.SupplyDraft,
		CreatedAt:            now,
		ETAs:                 []store.SupplyETA{},
		UpdatedAt:            now,
	}

	m.mu.Lock()
	m.supply[netID] = append(m.supply[netID], req)
	m.mu.Unlock()

	if err := m.store.SaveSupplyRequest(req); err != nil {
		return nil, fmt.Errorf("persist supply request: %w", err)
	}

	summary := fmt.Sprintf("%s requests: %s [%s]", req.RequestedByCall, formatSupplyItems(req.Items), strings.ToUpper(req.Priority))
	m.logTimeline(netID, TLSupplyRequested, a.Callsign, summary, "")
	m.logActivity(activity.ActionSupplyRequestCreated, a, req.ID, summary)
	m.emit(Event{Type: EventSupplyCreated, Data: req})

	out := req
	return &out, nil
}

// AddSupplyItems appends items to a draft request — the published batching
// rule ("ask them what else they're running low on") in code: items added
// after CreatedAt carry their own AddedAt as the evidence of the batch.
// Refused once the request has been read back (draft only).
func (m *TrafficManager) AddSupplyItems(netID, id string, in AddItemsInput, a Actor) (*store.SupplyRequest, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}
	if len(in.Items) == 0 {
		return nil, fmt.Errorf("at least one item is required")
	}

	m.mu.Lock()
	reqs, idx := m.findSupplyLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("supply request %q not found: %w", id, ErrNotFound)
	}
	req := deepCopySupply(reqs[idx])
	if req.Status != store.SupplyDraft {
		m.mu.Unlock()
		return nil, illegalTransition("items can only be added while the request is a draft")
	}

	now := m.now()
	added := make([]store.SupplyItem, len(in.Items))
	copy(added, in.Items)
	for i := range added {
		if added[i].AddedAt.IsZero() {
			added[i].AddedAt = now
		}
	}
	req.Items = append(req.Items, added...)
	if in.AskedWhatElse != nil {
		req.AskedWhatElse = *in.AskedWhatElse
	}
	req.UpdatedAt = now
	reqs[idx] = req
	m.supply[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSupplyRequest(req); err != nil {
		return nil, fmt.Errorf("persist supply request: %w", err)
	}
	summary := fmt.Sprintf("%s adds to supply request: %s", req.RequestedByCall, formatSupplyItems(added))
	m.logTimeline(netID, TLSupplyItemsAdded, a.Callsign, summary, "")
	m.logActivity(activity.ActionSupplyRequestUpdated, a, req.ID, summary)
	m.emit(Event{Type: EventSupplyUpdated, Data: req})

	out := req
	return &out, nil
}

// ReadbackSupply is the transmission-boundary step: a request is not
// "transmitted" until its read-back is confirmed. Confirmed=false means the
// requester corrected something on the read-back; the request stays draft
// and the correction is appended to Notes, with no status timeline entry
// (nothing has actually happened to the ladder yet).
func (m *TrafficManager) ReadbackSupply(netID, id string, in ReadbackInput, a Actor) (*store.SupplyRequest, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs, idx := m.findSupplyLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("supply request %q not found: %w", id, ErrNotFound)
	}
	req := deepCopySupply(reqs[idx])
	if req.Status != store.SupplyDraft {
		m.mu.Unlock()
		return nil, illegalTransition("supply request is not awaiting read-back")
	}

	now := m.now()
	loggedTimeline := false
	if in.Confirmed {
		if len(req.Items) == 0 {
			m.mu.Unlock()
			return nil, fmt.Errorf("at least one item is required")
		}
		req.Status = store.SupplyConfirmed
		req.ReadBackAt = &now
		req.ReadBackBy = in.ReadBackBy
		loggedTimeline = true
	} else {
		note := strings.TrimSpace(in.Correction)
		if note == "" {
			note = "requester corrected the read-back"
		}
		if req.Notes == "" {
			req.Notes = "correction: " + note
		} else {
			req.Notes = req.Notes + "; correction: " + note
		}
	}
	req.UpdatedAt = now
	reqs[idx] = req
	m.supply[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSupplyRequest(req); err != nil {
		return nil, fmt.Errorf("persist supply request: %w", err)
	}
	if loggedTimeline {
		m.logTimeline(netID, TLSupplyReadbackConfirmed, a.Callsign, fmt.Sprintf("%s confirmed read-back of supply request", req.RequestedByCall), "")
	}
	m.logActivity(activity.ActionSupplyRequestUpdated, a, req.ID, fmt.Sprintf("readback confirmed=%v", in.Confirmed))
	m.emit(Event{Type: EventSupplyUpdated, Data: req})

	out := req
	return &out, nil
}

// RelaySupply hands a confirmed request to a supplier/logistics. Refused
// until the request has been read back — "not transmitted until read back"
// is the published rule.
func (m *TrafficManager) RelaySupply(netID, id string, in RelayInput, a Actor) (*store.SupplyRequest, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs, idx := m.findSupplyLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("supply request %q not found: %w", id, ErrNotFound)
	}
	req := deepCopySupply(reqs[idx])
	if req.Status != store.SupplyConfirmed {
		m.mu.Unlock()
		return nil, illegalTransition("supply request has not been read back")
	}

	now := m.now()
	req.Status = store.SupplyRelayed
	req.RelayedAt = &now
	req.RelayedTo = in.RelayedTo
	req.UpdatedAt = now
	reqs[idx] = req
	m.supply[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSupplyRequest(req); err != nil {
		return nil, fmt.Errorf("persist supply request: %w", err)
	}
	summary := fmt.Sprintf("Supply request from %s relayed to %s", req.RequestedByCall, req.RelayedTo)
	m.logTimeline(netID, TLSupplyRelayed, a.Callsign, summary, "")
	m.logActivity(activity.ActionSupplyRequestUpdated, a, req.ID, summary)
	m.emit(Event{Type: EventSupplyUpdated, Data: req})

	out := req
	return &out, nil
}

// RecordSupplyETA appends a supplier ETA statement. History is kept because
// the dashboard question is "how long since they SAID 20 minutes", and
// suppliers revise; every call appends, it never replaces.
func (m *TrafficManager) RecordSupplyETA(netID, id string, in ETAInput, a Actor) (*store.SupplyRequest, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}
	if in.Minutes <= 0 {
		return nil, fmt.Errorf("minutes must be greater than 0")
	}

	m.mu.Lock()
	reqs, idx := m.findSupplyLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("supply request %q not found: %w", id, ErrNotFound)
	}
	req := deepCopySupply(reqs[idx])
	if req.Status != store.SupplyRelayed && req.Status != store.SupplyEnRoute {
		m.mu.Unlock()
		return nil, illegalTransition("supply request must be relayed before an ETA can be recorded")
	}

	now := m.now()
	eta := store.SupplyETA{
		Minutes: in.Minutes,
		GivenAt: now,
		DueAt:   now.Add(time.Duration(in.Minutes) * time.Minute),
		Source:  in.Source,
	}
	req.ETAs = append(req.ETAs, eta)
	req.Status = store.SupplyEnRoute
	req.UpdatedAt = now
	reqs[idx] = req
	m.supply[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSupplyRequest(req); err != nil {
		return nil, fmt.Errorf("persist supply request: %w", err)
	}
	who := in.Source
	if who == "" {
		who = req.RelayedTo
	}
	if who == "" {
		who = "supplier"
	}
	summary := fmt.Sprintf("%s ETA %d min to %s", who, in.Minutes, req.RequestedByCall)
	m.logTimeline(netID, TLSupplyETA, a.Callsign, summary, "")
	m.logActivity(activity.ActionSupplyRequestUpdated, a, req.ID, summary)
	m.emit(Event{Type: EventSupplyUpdated, Data: req})

	out := req
	return &out, nil
}

// DeliverSupply marks a request delivered. Skipping the ETA step is legal —
// small drops arrive before anyone gives one.
func (m *TrafficManager) DeliverSupply(netID, id string, a Actor) (*store.SupplyRequest, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs, idx := m.findSupplyLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("supply request %q not found: %w", id, ErrNotFound)
	}
	req := deepCopySupply(reqs[idx])
	if req.Status != store.SupplyRelayed && req.Status != store.SupplyEnRoute {
		m.mu.Unlock()
		return nil, illegalTransition("supply request must be relayed before it can be delivered")
	}

	now := m.now()
	req.DeliveredAt = &now
	req.Status = store.SupplyDelivered
	req.UpdatedAt = now
	reqs[idx] = req
	m.supply[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSupplyRequest(req); err != nil {
		return nil, fmt.Errorf("persist supply request: %w", err)
	}
	summary := fmt.Sprintf("Supply delivered to %s", req.RequestedByCall)
	if len(req.ETAs) > 0 {
		last := req.ETAs[len(req.ETAs)-1]
		summary += fmt.Sprintf(" (%s after ETA given)", formatDurationMinutes(now.Sub(last.GivenAt)))
	}
	m.logTimeline(netID, TLSupplyDelivered, a.Callsign, summary, "")
	m.logActivity(activity.ActionSupplyRequestUpdated, a, req.ID, summary)
	m.emit(Event{Type: EventSupplyUpdated, Data: req})

	out := req
	return &out, nil
}

// CancelSupply cancels a non-terminal request.
func (m *TrafficManager) CancelSupply(netID, id string, in CancelInput, a Actor) (*store.SupplyRequest, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	reqs, idx := m.findSupplyLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("supply request %q not found: %w", id, ErrNotFound)
	}
	req := deepCopySupply(reqs[idx])
	if terminalSupplyStatuses[req.Status] {
		m.mu.Unlock()
		return nil, illegalTransition(fmt.Sprintf("supply request is already %s", req.Status))
	}

	now := m.now()
	req.Status = store.SupplyCancelled
	req.CancelledAt = &now
	req.CancelReason = in.Reason
	req.UpdatedAt = now
	reqs[idx] = req
	m.supply[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSupplyRequest(req); err != nil {
		return nil, fmt.Errorf("persist supply request: %w", err)
	}
	summary := fmt.Sprintf("Supply request from %s cancelled: %s", req.RequestedByCall, in.Reason)
	m.logTimeline(netID, TLSupplyCancelled, a.Callsign, summary, "")
	m.logActivity(activity.ActionSupplyRequestUpdated, a, req.ID, summary)
	m.emit(Event{Type: EventSupplyUpdated, Data: req})

	out := req
	return &out, nil
}

// MergeSupply absorbs sourceID into targetID — the batching rule applied
// after the fact, when two stations both radioed in about the same
// location. Both must still be draft, and at the same Location.
func (m *TrafficManager) MergeSupply(netID, targetID, sourceID string, a Actor) (*store.SupplyRequest, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}
	if targetID == sourceID {
		return nil, fmt.Errorf("cannot merge a request into itself")
	}

	m.mu.Lock()
	reqs := m.supply[netID]
	targetIdx := indexOfSupply(reqs, targetID)
	if targetIdx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("supply request %q not found: %w", targetID, ErrNotFound)
	}
	sourceIdx := indexOfSupply(reqs, sourceID)
	if sourceIdx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("supply request %q not found: %w", sourceID, ErrNotFound)
	}
	target := deepCopySupply(reqs[targetIdx])
	source := deepCopySupply(reqs[sourceIdx])
	if target.Status != store.SupplyDraft || source.Status != store.SupplyDraft {
		m.mu.Unlock()
		return nil, illegalTransition("both requests must be draft to merge")
	}
	if target.Location != source.Location {
		m.mu.Unlock()
		return nil, fmt.Errorf("requests are not at the same location")
	}

	now := m.now()
	target.Items = append(target.Items, source.Items...)
	target.UpdatedAt = now
	source.Status = store.SupplyMerged
	source.MergedIntoID = target.ID
	source.UpdatedAt = now
	reqs[targetIdx] = target
	reqs[sourceIdx] = source
	m.supply[netID] = reqs
	m.mu.Unlock()

	if err := m.store.SaveSupplyRequest(target); err != nil {
		return nil, fmt.Errorf("persist target supply request: %w", err)
	}
	if err := m.store.SaveSupplyRequest(source); err != nil {
		return nil, fmt.Errorf("persist source supply request: %w", err)
	}
	summary := fmt.Sprintf("Supply request from %s merged into %s", source.RequestedByCall, target.RequestedByCall)
	m.logTimeline(netID, TLSupplyMerged, a.Callsign, summary, "")
	m.logActivity(activity.ActionSupplyRequestUpdated, a, target.ID, summary)
	m.emit(Event{Type: EventSupplyUpdated, Data: target})
	m.emit(Event{Type: EventSupplyUpdated, Data: source})

	out := target
	return &out, nil
}
