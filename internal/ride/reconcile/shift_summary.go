package reconcile

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/store"
)

// deriveSAGTally computes the counts this package can derive from logged
// SAG requests for one vehicle check-in: a transport per delivered slot, an
// assist per self-resolved slot, and an incident per distinct request the
// vehicle worked. Tubes/tires/first-aid have no logged source and are
// always 0 — the driver enters those. Zero when no SAG manager is wired.
func (m *Manager) deriveSAGTally(netID, checkInID string) SAGShiftDerived {
	if m.sagMgr == nil {
		return SAGShiftDerived{}
	}
	var d SAGShiftDerived
	seen := map[string]bool{}
	for _, r := range m.sagMgr.GetRequests(netID) {
		involved := false
		for _, leg := range r.Legs {
			if leg.VehicleCheckInID != checkInID {
				continue
			}
			involved = true
			for _, sid := range leg.SlotIDs {
				for _, slot := range r.Slots {
					if slot.ID != sid {
						continue
					}
					switch slot.Disposition {
					case ride.SlotDelivered:
						d.Transports++
					case ride.SlotSelfResolved:
						d.Assists++
					}
				}
			}
		}
		if involved {
			seen[r.ID] = true
		}
	}
	d.IncidentsAttended = len(seen)
	return d
}

func (m *Manager) view(sm store.SAGShiftSummary) ShiftSummaryView {
	derived := m.deriveSAGTally(sm.NetID, sm.CheckInID)
	eff := Effective(sm.Entered, derived)
	var netMiles *float64
	if sm.OdometerStart != nil && sm.OdometerEnd != nil {
		d := *sm.OdometerEnd - *sm.OdometerStart
		netMiles = &d
	}
	return ShiftSummaryView{SAGShiftSummary: sm, Derived: derived, Effective: eff, NetMiles: netMiles}
}

func (m *Manager) findShiftSummary(netID, id string) (store.SAGShiftSummary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sm := range m.shifts[netID] {
		if sm.ID == id {
			return sm, true
		}
	}
	return store.SAGShiftSummary{}, false
}

func (m *Manager) storeShift(sm store.SAGShiftSummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.shifts[sm.NetID]
	for i := range list {
		if list[i].ID == sm.ID {
			list[i] = sm
			return
		}
	}
	m.shifts[sm.NetID] = append(list, sm)
}

func labelForShift(sm store.SAGShiftSummary) string {
	if sm.TacticalCall != "" {
		return sm.TacticalCall
	}
	return sm.Callsign
}

// CreateShiftSummary starts a new draft shift summary for a sag check-in,
// pre-filled from the check-in. Only one draft may exist per check-in at a
// time; once a draft is filed, a new one may be created for the same unit
// (a correction shift, or the next shift).
func (m *Manager) CreateShiftSummary(netID, checkInID, by string) (*ShiftSummaryView, error) {
	if m.netMgr == nil {
		return nil, errors.New("net control not available")
	}
	var ci *store.NetCheckIn
	for _, c := range m.netMgr.GetCheckIns(netID) {
		if c.ID == checkInID {
			cc := c
			ci = &cc
			break
		}
	}
	if ci == nil {
		return nil, fmt.Errorf("check-in %q not found", checkInID)
	}
	if ci.Category != netcontrol.CatSAG {
		return nil, fmt.Errorf("check-in %q is not a sag unit (category %q)", checkInID, ci.Category)
	}
	for _, sm := range m.ShiftSummaries(netID) {
		if sm.CheckInID == checkInID && sm.Status == ShiftDraft {
			return nil, fmt.Errorf("a draft shift summary already exists for this unit: %s", sm.ID)
		}
	}

	now := m.clock()
	start := ci.CheckedInAt
	sm := store.SAGShiftSummary{
		ID: uuid.New().String(), NetID: netID, CheckInID: checkInID,
		Callsign: ci.Callsign, TacticalCall: ci.TacticalCall,
		ShiftStart: &start,
		Status:     ShiftDraft,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := m.store.SaveSAGShiftSummary(sm); err != nil {
		return nil, fmt.Errorf("persist shift summary: %w", err)
	}
	m.storeShift(sm)
	view := m.view(sm)
	m.emit(Event{Type: EventShiftSummaryUpdated, Data: view})
	return &view, nil
}

// UpdateShiftSummary replaces the editable fields of a draft shift summary.
// Refuses once the summary is filed.
func (m *Manager) UpdateShiftSummary(netID string, in store.SAGShiftSummary) (*ShiftSummaryView, error) {
	existing, ok := m.findShiftSummary(netID, in.ID)
	if !ok {
		return nil, ErrNotFound
	}
	if existing.Status == ShiftFiled {
		return nil, fmt.Errorf("shift summary %s is filed and cannot be edited: %w", in.ID, ErrIllegal)
	}

	existing.DriverName = in.DriverName
	existing.Vehicle = in.Vehicle
	existing.OdometerStart = in.OdometerStart
	existing.OdometerEnd = in.OdometerEnd
	existing.ShiftStart = in.ShiftStart
	existing.ShiftEnd = in.ShiftEnd
	existing.Entered = in.Entered
	existing.Notes = in.Notes
	existing.UpdatedAt = m.clock()

	if err := m.store.SaveSAGShiftSummary(existing); err != nil {
		return nil, fmt.Errorf("persist shift summary: %w", err)
	}
	m.storeShift(existing)
	view := m.view(existing)
	m.emit(Event{Type: EventShiftSummaryUpdated, Data: view})
	return &view, nil
}

// FileShiftSummary requires both odometer readings (end >= start) and locks
// the summary. ShiftEnd defaults to now when the driver never set one.
func (m *Manager) FileShiftSummary(netID, id, by string) (*ShiftSummaryView, error) {
	existing, ok := m.findShiftSummary(netID, id)
	if !ok {
		return nil, ErrNotFound
	}
	if existing.Status == ShiftFiled {
		return nil, fmt.Errorf("shift summary %s is already filed: %w", id, ErrIllegal)
	}
	if existing.OdometerStart == nil || existing.OdometerEnd == nil {
		return nil, errors.New("odometer start and end are required")
	}
	if *existing.OdometerEnd < *existing.OdometerStart {
		return nil, errors.New("odometer end is before start")
	}

	now := m.clock()
	if existing.ShiftEnd == nil {
		existing.ShiftEnd = &now
	}
	existing.Status = ShiftFiled
	existing.FiledAt = &now
	existing.FiledBy = by
	existing.UpdatedAt = now

	if err := m.store.SaveSAGShiftSummary(existing); err != nil {
		return nil, fmt.Errorf("persist filed shift summary: %w", err)
	}
	m.storeShift(existing)
	m.logTimeline(netID, TimelineShiftFiled, by, fmt.Sprintf("SAG shift summary filed for %s", labelForShift(existing)), "")
	view := m.view(existing)
	m.emit(Event{Type: EventShiftSummaryUpdated, Data: view})
	return &view, nil
}

// ShiftSummaries returns every shift summary for a net. Never nil.
func (m *Manager) ShiftSummaries(netID string) []ShiftSummaryView {
	m.mu.RLock()
	list := append([]store.SAGShiftSummary(nil), m.shifts[netID]...)
	m.mu.RUnlock()

	out := make([]ShiftSummaryView, 0, len(list))
	for _, sm := range list {
		out = append(out, m.view(sm))
	}
	return out
}

// GetShiftSummary returns one shift summary view by id.
func (m *Manager) GetShiftSummary(netID, id string) (*ShiftSummaryView, bool) {
	sm, ok := m.findShiftSummary(netID, id)
	if !ok {
		return nil, false
	}
	v := m.view(sm)
	return &v, true
}
