package netcontrol

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
)

// Manager manages net control operations with persistence and events.
type Manager struct {
	store    store.Store
	tracker  station.Tracker
	mu       sync.RWMutex
	nets     map[string]store.Net
	checkIns map[string][]store.NetCheckIn // keyed by netID
	missions map[string][]store.NetMission // keyed by netID
	events   chan Event
}

// NewManager creates a new net control Manager.
func NewManager(s store.Store, t station.Tracker) *Manager {
	return &Manager{
		store:    s,
		tracker:  t,
		nets:     make(map[string]store.Net),
		checkIns: make(map[string][]store.NetCheckIn),
		missions: make(map[string][]store.NetMission),
		events:   make(chan Event, 64),
	}
}

// Load loads non-archived nets and their data from the store.
func (m *Manager) Load() error {
	nets, err := m.store.LoadNets()
	if err != nil {
		return fmt.Errorf("load nets: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, n := range nets {
		if n.Status == StatusArchived {
			continue
		}
		m.nets[n.ID] = n

		cis, err := m.store.LoadNetCheckIns(n.ID)
		if err != nil {
			return fmt.Errorf("load check-ins for net %s: %w", n.ID, err)
		}
		m.checkIns[n.ID] = cis

		missions, err := m.store.LoadNetMissions(n.ID)
		if err != nil {
			return fmt.Errorf("load missions for net %s: %w", n.ID, err)
		}
		m.missions[n.ID] = missions
	}

	return nil
}

// Events returns the events channel for WebSocket broadcast.
func (m *Manager) Events() <-chan Event {
	return m.events
}

// CreateNet creates a new net.
func (m *Manager) CreateNet(n store.Net) (*store.Net, error) {
	if strings.TrimSpace(n.Name) == "" {
		return nil, fmt.Errorf("net name is required")
	}

	n.ID = uuid.New().String()
	if n.Type == "" {
		n.Type = "tactical"
	}
	if n.Status == "" {
		n.Status = StatusDraft
	}

	if err := m.store.SaveNet(n); err != nil {
		return nil, fmt.Errorf("persist net: %w", err)
	}

	m.mu.Lock()
	m.nets[n.ID] = n
	m.checkIns[n.ID] = nil
	m.missions[n.ID] = nil
	m.mu.Unlock()

	m.emit(Event{Type: EventNetCreated, Data: n})

	return &n, nil
}

// OpenNet transitions a net from draft to open.
func (m *Manager) OpenNet(id string) error {
	m.mu.Lock()
	n, ok := m.nets[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("net %q not found", id)
	}

	now := time.Now().UTC()
	n.Status = StatusOpen
	n.OpenedAt = &now
	m.nets[id] = n
	m.mu.Unlock()

	if err := m.store.SaveNet(n); err != nil {
		return fmt.Errorf("persist net: %w", err)
	}

	m.logEvent(id, "net_opened", n.NCSCallsign, fmt.Sprintf("Net %q opened by %s", n.Name, n.NCSCallsign))
	m.emit(Event{Type: EventNetUpdated, Data: n})

	return nil
}

// CloseNet closes a net and generates a summary.
func (m *Manager) CloseNet(id string) (*store.Net, *NetSummary, error) {
	m.mu.Lock()
	n, ok := m.nets[id]
	if !ok {
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("net %q not found", id)
	}

	now := time.Now().UTC()
	n.Status = StatusClosed
	n.ClosedAt = &now
	m.nets[id] = n

	cis := m.checkIns[id]
	missions := m.missions[id]
	m.mu.Unlock()

	if err := m.store.SaveNet(n); err != nil {
		return nil, nil, fmt.Errorf("persist net: %w", err)
	}

	// Generate summary.
	trafficCounts := make(map[string]int)
	for _, ci := range cis {
		if ci.Traffic != TrafficNone {
			trafficCounts[ci.Traffic]++
		}
	}

	var duration string
	if n.OpenedAt != nil {
		d := now.Sub(*n.OpenedAt)
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if hours > 0 {
			duration = fmt.Sprintf("%dh %dm", hours, mins)
		} else {
			duration = fmt.Sprintf("%dm", mins)
		}
	}

	summary := &NetSummary{
		NetID:         id,
		Name:          n.Name,
		Duration:      duration,
		TotalCheckIns: len(cis),
		TotalMissions: len(missions),
		TrafficCounts: trafficCounts,
	}

	m.logEvent(id, "net_closed", n.NCSCallsign, fmt.Sprintf("Net %q closed — %d check-ins, %s", n.Name, len(cis), duration))
	m.emit(Event{Type: EventNetUpdated, Data: n})

	return &n, summary, nil
}

// TransferNCS transfers net control to a new operator.
func (m *Manager) TransferNCS(netID, newCallsign, newUserID string) error {
	m.mu.Lock()
	n, ok := m.nets[netID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("net %q not found", netID)
	}

	oldCallsign := n.NCSCallsign
	n.NCSCallsign = newCallsign
	n.NCSUserID = newUserID
	m.nets[netID] = n
	m.mu.Unlock()

	if err := m.store.SaveNet(n); err != nil {
		return fmt.Errorf("persist net: %w", err)
	}

	m.logEvent(netID, "ncs_transfer", newCallsign, fmt.Sprintf("NCS transferred from %s to %s", oldCallsign, newCallsign))
	m.emit(Event{Type: EventNetUpdated, Data: n})

	return nil
}

// CheckIn registers an operator in the net.
func (m *Manager) CheckIn(netID, callsign, traffic string) (*store.NetCheckIn, error) {
	m.mu.RLock()
	_, ok := m.nets[netID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("net %q not found", netID)
	}

	if strings.TrimSpace(callsign) == "" {
		return nil, fmt.Errorf("callsign is required")
	}

	callsign = strings.ToUpper(strings.TrimSpace(callsign))

	if traffic == "" {
		traffic = TrafficNone
	}

	now := time.Now().UTC()
	ci := store.NetCheckIn{
		ID:          uuid.New().String(),
		NetID:       netID,
		Callsign:    callsign,
		Status:      OpAvailable,
		Traffic:     traffic,
		CheckedInAt: now,
		LastHeard:   now,
	}

	// Auto-populate from tracker if known.
	if m.tracker != nil {
		m.autoPopulate(&ci)
	}

	if err := m.store.SaveNetCheckIn(ci); err != nil {
		return nil, fmt.Errorf("persist check-in: %w", err)
	}

	m.mu.Lock()
	m.checkIns[netID] = append(m.checkIns[netID], ci)
	m.mu.Unlock()

	trafficStr := ""
	if traffic != TrafficNone {
		trafficStr = fmt.Sprintf(" with %s traffic", traffic)
	}
	m.logEvent(netID, "checkin", callsign, fmt.Sprintf("%s checked in%s", callsign, trafficStr))
	m.emit(Event{Type: EventCheckInCreated, Data: ci})

	return &ci, nil
}

// UpdateCheckIn updates an operator's check-in.
func (m *Manager) UpdateCheckIn(ci store.NetCheckIn) (*store.NetCheckIn, error) {
	m.mu.Lock()
	cis, ok := m.checkIns[ci.NetID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("net %q not found", ci.NetID)
	}

	found := false
	for i, existing := range cis {
		if existing.ID == ci.ID {
			// Preserve immutable fields.
			ci.CheckedInAt = existing.CheckedInAt
			if ci.Callsign == "" {
				ci.Callsign = existing.Callsign
			}
			if ci.NetID == "" {
				ci.NetID = existing.NetID
			}

			cis[i] = ci
			found = true
			break
		}
	}

	if !found {
		m.mu.Unlock()
		return nil, fmt.Errorf("check-in %q not found", ci.ID)
	}

	m.checkIns[ci.NetID] = cis
	m.mu.Unlock()

	if err := m.store.SaveNetCheckIn(ci); err != nil {
		return nil, fmt.Errorf("persist check-in: %w", err)
	}

	m.logEvent(ci.NetID, "status_change", ci.Callsign, fmt.Sprintf("%s status → %s", ci.Callsign, ci.Status))
	m.emit(Event{Type: EventCheckInUpdated, Data: ci})

	return &ci, nil
}

// CheckOut releases an operator from the net.
func (m *Manager) CheckOut(netID, checkInID string) error {
	m.mu.Lock()
	cis, ok := m.checkIns[netID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("net %q not found", netID)
	}

	found := false
	for i, ci := range cis {
		if ci.ID == checkInID {
			now := time.Now().UTC()
			cis[i].Status = OpReleased
			cis[i].CheckedOutAt = &now

			if err := m.store.SaveNetCheckIn(cis[i]); err != nil {
				m.mu.Unlock()
				return fmt.Errorf("persist check-in: %w", err)
			}

			m.checkIns[netID] = cis
			m.mu.Unlock()

			m.logEvent(netID, "checkout", ci.Callsign, fmt.Sprintf("%s checked out", ci.Callsign))
			m.emit(Event{Type: EventCheckInUpdated, Data: cis[i]})
			return nil
		}
	}

	m.mu.Unlock()
	if !found {
		return fmt.Errorf("check-in %q not found", checkInID)
	}
	return nil
}

// CreateMission creates a new mission.
func (m *Manager) CreateMission(mission store.NetMission) (*store.NetMission, error) {
	m.mu.RLock()
	_, ok := m.nets[mission.NetID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("net %q not found", mission.NetID)
	}

	if strings.TrimSpace(mission.Title) == "" {
		return nil, fmt.Errorf("mission title is required")
	}

	mission.ID = uuid.New().String()
	if mission.Status == "" {
		mission.Status = "open"
	}
	if mission.Priority == "" {
		mission.Priority = TrafficRoutine
	}
	mission.CreatedAt = time.Now().UTC()

	if err := m.store.SaveNetMission(mission); err != nil {
		return nil, fmt.Errorf("persist mission: %w", err)
	}

	m.mu.Lock()
	m.missions[mission.NetID] = append(m.missions[mission.NetID], mission)
	m.mu.Unlock()

	assignStr := ""
	if mission.AssignedTo != "" {
		assignStr = fmt.Sprintf(" → %s", mission.AssignedTo)
	}
	m.logEvent(mission.NetID, "mission_created", mission.AssignedTo, fmt.Sprintf("Mission: %s%s", mission.Title, assignStr))
	m.emit(Event{Type: EventMissionCreated, Data: mission})

	return &mission, nil
}

// UpdateMission updates an existing mission.
func (m *Manager) UpdateMission(mission store.NetMission) (*store.NetMission, error) {
	m.mu.Lock()
	missions, ok := m.missions[mission.NetID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("net %q not found", mission.NetID)
	}

	found := false
	for i, existing := range missions {
		if existing.ID == mission.ID {
			mission.CreatedAt = existing.CreatedAt
			if mission.Status == "complete" && mission.CompletedAt == nil {
				now := time.Now().UTC()
				mission.CompletedAt = &now
			}
			missions[i] = mission
			found = true
			break
		}
	}

	if !found {
		m.mu.Unlock()
		return nil, fmt.Errorf("mission %q not found", mission.ID)
	}

	m.missions[mission.NetID] = missions
	m.mu.Unlock()

	if err := m.store.SaveNetMission(mission); err != nil {
		return nil, fmt.Errorf("persist mission: %w", err)
	}

	m.logEvent(mission.NetID, "mission_updated", mission.AssignedTo, fmt.Sprintf("Mission %q → %s", mission.Title, mission.Status))
	m.emit(Event{Type: EventMissionUpdated, Data: mission})

	return &mission, nil
}

// AddNote adds a note to a net.
func (m *Manager) AddNote(note store.NetNote) (*store.NetNote, error) {
	m.mu.RLock()
	_, ok := m.nets[note.NetID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("net %q not found", note.NetID)
	}

	if strings.TrimSpace(note.Content) == "" {
		return nil, fmt.Errorf("note content is required")
	}

	note.ID = uuid.New().String()
	note.CreatedAt = time.Now().UTC()

	if err := m.store.SaveNetNote(note); err != nil {
		return nil, fmt.Errorf("persist note: %w", err)
	}

	m.logEvent(note.NetID, "note", "", fmt.Sprintf("Note by %s: %s", note.AuthorName, truncate(note.Content, 80)))
	m.emit(Event{Type: EventTimelineEntry, Data: note})

	return &note, nil
}

// InitiateRollCall logs a roll call event.
func (m *Manager) InitiateRollCall(netID string) error {
	m.mu.RLock()
	n, ok := m.nets[netID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("net %q not found", netID)
	}

	// Increment missed roll calls for all active check-ins.
	m.mu.Lock()
	cis := m.checkIns[netID]
	for i, ci := range cis {
		if ci.Status != OpReleased {
			cis[i].MissedRollCalls++
			m.store.SaveNetCheckIn(cis[i])
		}
	}
	m.checkIns[netID] = cis
	m.mu.Unlock()

	m.logEvent(netID, "rollcall", n.NCSCallsign, "Roll call initiated")

	return nil
}

// RecordRollCallResponse records that an operator responded to roll call.
func (m *Manager) RecordRollCallResponse(netID, checkInID string) error {
	m.mu.Lock()
	cis, ok := m.checkIns[netID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("net %q not found", netID)
	}

	for i, ci := range cis {
		if ci.ID == checkInID {
			cis[i].MissedRollCalls = 0
			cis[i].LastHeard = time.Now().UTC()

			if err := m.store.SaveNetCheckIn(cis[i]); err != nil {
				m.mu.Unlock()
				return fmt.Errorf("persist check-in: %w", err)
			}

			m.checkIns[netID] = cis
			m.mu.Unlock()

			m.emit(Event{Type: EventCheckInUpdated, Data: cis[i]})
			return nil
		}
	}

	m.mu.Unlock()
	return fmt.Errorf("check-in %q not found", checkInID)
}

// SearchOperators performs a fuzzy substring match on tracked stations.
func (m *Manager) SearchOperators(query string) []station.Station {
	if m.tracker == nil || query == "" {
		return nil
	}

	upper := strings.ToUpper(query)
	all := m.tracker.All()
	var results []station.Station
	for _, s := range all {
		key := s.Callsign
		if s.SSID > 0 {
			key = fmt.Sprintf("%s-%d", s.Callsign, s.SSID)
		}
		if strings.Contains(strings.ToUpper(key), upper) ||
			strings.Contains(strings.ToUpper(RootCallsign(key)), upper) {
			results = append(results, s)
		}
	}
	return results
}

// ActiveNet returns the currently open net, if any.
func (m *Manager) ActiveNet() *store.Net {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, n := range m.nets {
		if n.Status == StatusOpen {
			return &n
		}
	}
	return nil
}

// GetNet returns a net by ID.
func (m *Manager) GetNet(id string) (*store.Net, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.nets[id]
	if !ok {
		return nil, false
	}
	return &n, true
}

// GetNets returns all cached nets.
func (m *Manager) GetNets() []store.Net {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]store.Net, 0, len(m.nets))
	for _, n := range m.nets {
		result = append(result, n)
	}
	return result
}

// GetCheckIns returns check-ins for a net.
func (m *Manager) GetCheckIns(netID string) []store.NetCheckIn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cis := m.checkIns[netID]
	result := make([]store.NetCheckIn, len(cis))
	copy(result, cis)
	return result
}

// GetMissions returns missions for a net.
func (m *Manager) GetMissions(netID string) []store.NetMission {
	m.mu.RLock()
	defer m.mu.RUnlock()
	missions := m.missions[netID]
	result := make([]store.NetMission, len(missions))
	copy(result, missions)
	return result
}

// GetNotes returns notes for a net from the store.
func (m *Manager) GetNotes(netID string) ([]store.NetNote, error) {
	return m.store.LoadNetNotes(netID)
}

// GetEvents returns timeline events for a net from the store.
func (m *Manager) GetEvents(netID string) ([]store.NetEvent, error) {
	return m.store.LoadNetEvents(netID)
}

// RootCallsign strips the "-N" SSID suffix from a callsign.
func RootCallsign(callsign string) string {
	if idx := strings.LastIndex(callsign, "-"); idx > 0 {
		return callsign[:idx]
	}
	return callsign
}

// autoPopulate fills in check-in fields from the tracker.
func (m *Manager) autoPopulate(ci *store.NetCheckIn) {
	// Try exact match first.
	st, ok := m.tracker.Get(ci.Callsign)
	if !ok {
		// Try root callsign.
		root := RootCallsign(ci.Callsign)
		if root != ci.Callsign {
			st, ok = m.tracker.Get(root)
		}
	}
	if !ok {
		// Search all stations for substring match.
		results := m.tracker.Search(RootCallsign(ci.Callsign))
		if len(results) > 0 {
			st = results[0]
			ok = true
		}
	}
	if ok && st.Position != nil {
		ci.Lat = &st.Position.Lat
		ci.Lon = &st.Position.Lon
		ci.LastHeard = st.LastHeard
		if ci.OperatorName == "" && st.Comment != "" {
			ci.OperatorName = st.Comment
		}
	}
}

// logEvent creates a timeline event and persists it.
func (m *Manager) logEvent(netID, eventType, callsign, summary string) {
	evt := store.NetEvent{
		ID:        uuid.New().String(),
		NetID:     netID,
		Type:      eventType,
		Callsign:  callsign,
		Summary:   summary,
		Details:   "{}",
		CreatedAt: time.Now().UTC(),
	}

	m.store.SaveNetEvent(evt)
	m.emit(Event{Type: EventTimelineEntry, Data: evt})
}

func (m *Manager) emit(evt Event) {
	select {
	case m.events <- evt:
	default:
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
