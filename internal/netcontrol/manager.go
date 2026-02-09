package netcontrol

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
)

// trackedRef maps a tracked station callsign back to its check-in.
type trackedRef struct {
	netID     string
	checkInID string
}

// Manager manages net control operations with persistence and events.
type Manager struct {
	store        store.Store
	tracker      station.Tracker
	mu           sync.RWMutex
	nets         map[string]store.Net
	checkIns     map[string][]store.NetCheckIn // keyed by netID
	missions     map[string][]store.NetMission // keyed by netID
	trackedIndex map[string]trackedRef         // station callsign → check-in reference
	events       chan Event
}

// NewManager creates a new net control Manager.
func NewManager(s store.Store, t station.Tracker) *Manager {
	return &Manager{
		store:        s,
		tracker:      t,
		nets:         make(map[string]store.Net),
		checkIns:     make(map[string][]store.NetCheckIn),
		missions:     make(map[string][]store.NetMission),
		trackedIndex: make(map[string]trackedRef),
		events:       make(chan Event, 64),
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

	m.rebuildTrackedIndex()

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
		Source:      "voice",
		CheckedInAt: now,
		LastHeard:   now,
	}

	// Auto-populate from tracker if known.
	if m.tracker != nil {
		m.autoPopulate(&ci)
		m.discoverDevices(&ci)
	}

	// Auto-populate tactical call from alias table if empty.
	if ci.TacticalCall == "" && m.store != nil {
		if aliases, err := m.store.LoadTacticalAliases(); err == nil {
			for _, a := range aliases {
				if a.Callsign == ci.Callsign {
					ci.TacticalCall = a.Alias
					break
				}
			}
		}
	}

	// Set source based on whether position data was found.
	if ci.Lat != nil && ci.Lon != nil {
		ci.Source = "aprs"
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

			// Remove tracked stations from index.
			for _, ts := range cis[i].TrackedStations {
				delete(m.trackedIndex, ts.Callsign)
			}

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
	for i := range cis {
		if cis[i].Status == OpReleased {
			continue
		}
		cis[i].MissedRollCalls++

		// Auto-mark missing after threshold.
		if cis[i].MissedRollCalls >= MissedRollCallThreshold && cis[i].Status != OpMissing {
			cis[i].Status = OpMissing
			m.store.SaveNetCheckIn(cis[i])
			// Emit update for the auto-marked operator (outside lock would deadlock, so just save).
		} else {
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

// AssignMission assigns a mission to an operator (appends to MissionIDs).
func (m *Manager) AssignMission(netID, checkInID, missionID string) (*store.NetCheckIn, error) {
	m.mu.Lock()
	cis, ok := m.checkIns[netID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("net %q not found", netID)
	}

	// Validate mission exists.
	missions := m.missions[netID]
	var mission *store.NetMission
	for i, ms := range missions {
		if ms.ID == missionID {
			mission = &missions[i]
			break
		}
	}
	if mission == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("mission %q not found", missionID)
	}

	found := false
	var updated store.NetCheckIn
	for i, ci := range cis {
		if ci.ID == checkInID {
			// Reject duplicate assignment.
			for _, mid := range cis[i].MissionIDs {
				if mid == missionID {
					m.mu.Unlock()
					return nil, fmt.Errorf("operator already assigned to mission %q", missionID)
				}
			}
			cis[i].MissionIDs = append(cis[i].MissionIDs, missionID)
			if cis[i].Status == OpAvailable {
				cis[i].Status = OpAssigned
			}
			updated = cis[i]
			found = true
			break
		}
	}

	if !found {
		m.mu.Unlock()
		return nil, fmt.Errorf("check-in %q not found", checkInID)
	}

	m.checkIns[netID] = cis
	m.mu.Unlock()

	if err := m.store.SaveNetCheckIn(updated); err != nil {
		return nil, fmt.Errorf("persist check-in: %w", err)
	}

	m.logEvent(netID, "assignment", updated.Callsign,
		fmt.Sprintf("%s assigned to mission %q", updated.Callsign, mission.Title))
	m.emit(Event{Type: EventCheckInUpdated, Data: updated})

	return &updated, nil
}

// UnassignMission removes a specific mission assignment from an operator.
func (m *Manager) UnassignMission(netID, checkInID, missionID string) (*store.NetCheckIn, error) {
	m.mu.Lock()
	cis, ok := m.checkIns[netID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("net %q not found", netID)
	}

	found := false
	var updated store.NetCheckIn
	for i, ci := range cis {
		if ci.ID == checkInID {
			// Remove specific mission from the list.
			newIDs := make([]string, 0, len(cis[i].MissionIDs))
			removed := false
			for _, mid := range cis[i].MissionIDs {
				if mid == missionID {
					removed = true
				} else {
					newIDs = append(newIDs, mid)
				}
			}
			if !removed {
				m.mu.Unlock()
				return nil, fmt.Errorf("operator not assigned to mission %q", missionID)
			}
			cis[i].MissionIDs = newIDs
			if len(cis[i].MissionIDs) == 0 && cis[i].Status == OpAssigned {
				cis[i].Status = OpAvailable
			}
			updated = cis[i]
			found = true
			break
		}
	}

	if !found {
		m.mu.Unlock()
		return nil, fmt.Errorf("check-in %q not found", checkInID)
	}

	m.checkIns[netID] = cis
	m.mu.Unlock()

	if err := m.store.SaveNetCheckIn(updated); err != nil {
		return nil, fmt.Errorf("persist check-in: %w", err)
	}

	m.logEvent(netID, "assignment", updated.Callsign,
		fmt.Sprintf("%s unassigned from mission", updated.Callsign))
	m.emit(Event{Type: EventCheckInUpdated, Data: updated})

	return &updated, nil
}

// UnassignAllMissions removes all mission assignments from an operator.
func (m *Manager) UnassignAllMissions(netID, checkInID string) (*store.NetCheckIn, error) {
	m.mu.Lock()
	cis, ok := m.checkIns[netID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("net %q not found", netID)
	}

	found := false
	var updated store.NetCheckIn
	for i, ci := range cis {
		if ci.ID == checkInID {
			cis[i].MissionIDs = nil
			if cis[i].Status == OpAssigned {
				cis[i].Status = OpAvailable
			}
			updated = cis[i]
			found = true
			break
		}
	}

	if !found {
		m.mu.Unlock()
		return nil, fmt.Errorf("check-in %q not found", checkInID)
	}

	m.checkIns[netID] = cis
	m.mu.Unlock()

	if err := m.store.SaveNetCheckIn(updated); err != nil {
		return nil, fmt.Errorf("persist check-in: %w", err)
	}

	m.logEvent(netID, "assignment", updated.Callsign,
		fmt.Sprintf("%s unassigned from all missions", updated.Callsign))
	m.emit(Event{Type: EventCheckInUpdated, Data: updated})

	return &updated, nil
}

// ExportRosterCSV writes the roster as CSV.
func ExportRosterCSV(w io.Writer, checkIns []store.NetCheckIn) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{
		"callsign", "tacticalCall", "operatorName", "status", "traffic",
		"source", "location", "assignment", "trackedDevices", "checkedInAt", "checkedOutAt",
		"lastHeard", "missedRollCalls",
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, ci := range checkIns {
		checkedOut := ""
		if ci.CheckedOutAt != nil {
			checkedOut = ci.CheckedOutAt.Format(time.RFC3339)
		}
		var deviceNames []string
		for _, ts := range ci.TrackedStations {
			deviceNames = append(deviceNames, ts.Callsign)
		}
		row := []string{
			ci.Callsign,
			ci.TacticalCall,
			ci.OperatorName,
			ci.Status,
			ci.Traffic,
			ci.Source,
			ci.Location,
			ci.Assignment,
			strings.Join(deviceNames, ","),
			ci.CheckedInAt.Format(time.RFC3339),
			checkedOut,
			ci.LastHeard.Format(time.RFC3339),
			fmt.Sprintf("%d", ci.MissedRollCalls),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}

	return nil
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

// discoverDevices auto-links SSID variants of the operator's callsign.
func (m *Manager) discoverDevices(ci *store.NetCheckIn) {
	root := RootCallsign(ci.Callsign)
	results := m.tracker.Search(root)
	if len(results) == 0 {
		return
	}

	seen := make(map[string]bool)
	for _, st := range results {
		key := st.Callsign
		if st.SSID > 0 {
			key = fmt.Sprintf("%s-%d", st.Callsign, st.SSID)
		}
		// Only link SSID variants of the same root callsign.
		if RootCallsign(key) != root {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		ci.TrackedStations = append(ci.TrackedStations, store.TrackedStation{
			Callsign:   key,
			AutoLinked: true,
		})
		m.trackedIndex[key] = trackedRef{netID: ci.NetID, checkInID: ci.ID}
	}

	m.resolveTrackedPosition(ci)
}

// resolveTrackedPosition picks the best position from all tracked devices.
func (m *Manager) resolveTrackedPosition(ci *store.NetCheckIn) {
	if m.tracker == nil || len(ci.TrackedStations) == 0 {
		return
	}

	var bestLat, bestLon float64
	var bestTime time.Time
	found := false

	for _, ts := range ci.TrackedStations {
		st, ok := m.tracker.Get(ts.Callsign)
		if !ok || st.Position == nil {
			continue
		}
		if !found || st.LastHeard.After(bestTime) {
			bestLat = st.Position.Lat
			bestLon = st.Position.Lon
			bestTime = st.LastHeard
			found = true
		}
	}

	if found {
		ci.Lat = &bestLat
		ci.Lon = &bestLon
		ci.LastHeard = bestTime
		ci.Source = "aprs"
	}
}

// AddTrackedStation manually associates a station with a check-in.
func (m *Manager) AddTrackedStation(netID, checkInID, callsign string) (*store.NetCheckIn, error) {
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	if callsign == "" {
		return nil, fmt.Errorf("callsign is required")
	}

	m.mu.Lock()
	cis, ok := m.checkIns[netID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("net %q not found", netID)
	}

	var ci *store.NetCheckIn
	var idx int
	for i := range cis {
		if cis[i].ID == checkInID {
			ci = &cis[i]
			idx = i
			break
		}
	}
	if ci == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("check-in %q not found", checkInID)
	}

	// Reject duplicates.
	for _, ts := range ci.TrackedStations {
		if ts.Callsign == callsign {
			m.mu.Unlock()
			return nil, fmt.Errorf("station %q already tracked", callsign)
		}
	}

	ci.TrackedStations = append(ci.TrackedStations, store.TrackedStation{
		Callsign:   callsign,
		AutoLinked: false,
	})
	m.trackedIndex[callsign] = trackedRef{netID: netID, checkInID: checkInID}

	m.resolveTrackedPosition(ci)
	cis[idx] = *ci
	m.checkIns[netID] = cis
	updated := *ci
	m.mu.Unlock()

	if err := m.store.SaveNetCheckIn(updated); err != nil {
		return nil, fmt.Errorf("persist check-in: %w", err)
	}

	m.logEvent(netID, "device_added", ci.Callsign, fmt.Sprintf("Added tracked device %s for %s", callsign, ci.Callsign))
	m.emit(Event{Type: EventCheckInUpdated, Data: updated})

	return &updated, nil
}

// RemoveTrackedStation removes a station association from a check-in.
func (m *Manager) RemoveTrackedStation(netID, checkInID, callsign string) (*store.NetCheckIn, error) {
	callsign = strings.ToUpper(strings.TrimSpace(callsign))

	m.mu.Lock()
	cis, ok := m.checkIns[netID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("net %q not found", netID)
	}

	var ci *store.NetCheckIn
	var idx int
	for i := range cis {
		if cis[i].ID == checkInID {
			ci = &cis[i]
			idx = i
			break
		}
	}
	if ci == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("check-in %q not found", checkInID)
	}

	found := false
	filtered := ci.TrackedStations[:0]
	for _, ts := range ci.TrackedStations {
		if ts.Callsign == callsign {
			found = true
			continue
		}
		filtered = append(filtered, ts)
	}
	if !found {
		m.mu.Unlock()
		return nil, fmt.Errorf("station %q not tracked", callsign)
	}

	ci.TrackedStations = filtered
	delete(m.trackedIndex, callsign)

	m.resolveTrackedPosition(ci)
	cis[idx] = *ci
	m.checkIns[netID] = cis
	updated := *ci
	m.mu.Unlock()

	if err := m.store.SaveNetCheckIn(updated); err != nil {
		return nil, fmt.Errorf("persist check-in: %w", err)
	}

	m.logEvent(netID, "device_removed", ci.Callsign, fmt.Sprintf("Removed tracked device %s from %s", callsign, ci.Callsign))
	m.emit(Event{Type: EventCheckInUpdated, Data: updated})

	return &updated, nil
}

// OnStationUpdate is called from the server's bridgeTrackerEvents when any
// station is heard. If the station is tracked by a net operator, the operator's
// position is re-resolved and updated.
func (m *Manager) OnStationUpdate(callsign string, pos *station.Position, lastHeard time.Time) {
	m.mu.Lock()
	ref, ok := m.trackedIndex[callsign]
	if !ok {
		m.mu.Unlock()
		return
	}

	cis, ok := m.checkIns[ref.netID]
	if !ok {
		m.mu.Unlock()
		return
	}

	var ci *store.NetCheckIn
	var idx int
	for i := range cis {
		if cis[i].ID == ref.checkInID {
			ci = &cis[i]
			idx = i
			break
		}
	}
	if ci == nil || ci.Status == OpReleased {
		m.mu.Unlock()
		return
	}

	oldLat := ci.Lat
	oldLon := ci.Lon

	m.resolveTrackedPosition(ci)

	// Check if position actually changed.
	changed := false
	if oldLat == nil && ci.Lat != nil {
		changed = true
	} else if oldLat != nil && ci.Lat != nil && (*oldLat != *ci.Lat || *oldLon != *ci.Lon) {
		changed = true
	}

	if !changed {
		m.mu.Unlock()
		return
	}

	cis[idx] = *ci
	m.checkIns[ref.netID] = cis
	updated := *ci
	m.mu.Unlock()

	if err := m.store.SaveNetCheckIn(updated); err != nil {
		return
	}

	m.emit(Event{Type: EventCheckInUpdated, Data: updated})
}

// rebuildTrackedIndex populates trackedIndex from loaded check-ins.
func (m *Manager) rebuildTrackedIndex() {
	m.trackedIndex = make(map[string]trackedRef)
	for netID, cis := range m.checkIns {
		for _, ci := range cis {
			if ci.Status == OpReleased {
				continue
			}
			for _, ts := range ci.TrackedStations {
				m.trackedIndex[ts.Callsign] = trackedRef{netID: netID, checkInID: ci.ID}
			}
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
