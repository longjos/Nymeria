package reconcile

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/narvel/nymeria/internal/store"
)

func (m *Manager) findHandoffItem(netID, id string) (store.HandoffItem, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.handoffs[netID] {
		if h.ID == id {
			return h, true
		}
	}
	return store.HandoffItem{}, false
}

func (m *Manager) storeHandoff(h store.HandoffItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.handoffs[h.NetID]
	for i := range list {
		if list[i].ID == h.ID {
			list[i] = h
			return
		}
	}
	m.handoffs[h.NetID] = append(list, h)
}

// AddHandoffItem creates a new open hand-off item. ReplyTo defaults to
// "NCS" (the on-duty net control position, not a specific callsign).
func (m *Manager) AddHandoffItem(h store.HandoffItem) (*store.HandoffItem, error) {
	switch h.Kind {
	case HandoffAwaitingReply, HandoffPendingAction, HandoffFYI:
	default:
		return nil, fmt.Errorf("invalid kind %q", h.Kind)
	}
	if strings.TrimSpace(h.Summary) == "" {
		return nil, errors.New("summary is required")
	}
	if h.ReplyTo == "" {
		h.ReplyTo = "NCS"
	}

	now := m.clock()
	h.ID = uuid.New().String()
	h.Status = HandoffOpen
	h.HandoverCount = 0
	h.CreatedAt = now
	h.ResolvedBy = ""
	h.ResolvedAt = nil
	h.Resolution = ""

	if err := m.store.SaveHandoffItem(h); err != nil {
		return nil, fmt.Errorf("persist handoff item: %w", err)
	}
	m.storeHandoff(h)
	m.emit(Event{Type: EventHandoffItemCreated, Data: h})
	return &h, nil
}

// ResolveHandoffItem moves an open item to resolved (cancel=false) or
// cancelled (cancel=true). Terminal statuses cannot be re-resolved.
func (m *Manager) ResolveHandoffItem(netID, id, by, resolution string, cancel bool) (*store.HandoffItem, error) {
	existing, ok := m.findHandoffItem(netID, id)
	if !ok {
		return nil, ErrNotFound
	}
	if existing.Status != HandoffOpen {
		return nil, fmt.Errorf("handoff item %s is not open: %w", id, ErrIllegal)
	}

	now := m.clock()
	if cancel {
		existing.Status = HandoffCancelled
	} else {
		existing.Status = HandoffResolved
	}
	existing.ResolvedBy = by
	existing.ResolvedAt = &now
	existing.Resolution = resolution

	if err := m.store.SaveHandoffItem(existing); err != nil {
		return nil, fmt.Errorf("persist handoff item: %w", err)
	}
	m.storeHandoff(existing)
	m.emit(Event{Type: EventHandoffItemUpdated, Data: existing})
	return &existing, nil
}

// HandoffItems returns items for a net, optionally filtered by status
// ("" or "all" returns every status). Never nil.
func (m *Manager) HandoffItems(netID, status string) []store.HandoffItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.handoffs[netID]
	out := make([]store.HandoffItem, 0, len(list))
	for _, h := range list {
		if status != "" && status != "all" && h.Status != status {
			continue
		}
		out = append(out, h)
	}
	return out
}

// RecordNCSTransfer freezes the current briefing, bumps HandoverCount on
// every open item (a survival counter, not a status change), and saves the
// resulting ShiftHandoff. Called from the netcontrol NCS-transfer path (see
// server's handleTransferNCS). A new transfer before the previous one is
// acknowledged leaves the previous un-acked — an auditable gap, not a bug.
func (m *Manager) RecordNCSTransfer(netID, from, to string) {
	now := m.clock()
	open := m.HandoffItems(netID, HandoffOpen)

	m.mu.Lock()
	list := m.handoffs[netID]
	for i := range list {
		if list[i].Status == HandoffOpen {
			list[i].HandoverCount++
			if err := m.store.SaveHandoffItem(list[i]); err != nil {
				log.Printf("[reconcile] persist handover count for item %s: %v", list[i].ID, err)
			}
		}
	}
	m.mu.Unlock()

	ids := make([]string, 0, len(open))
	for _, h := range open {
		ids = append(ids, h.ID)
	}

	briefing := m.Briefing(netID)
	briefing.NCSCallsign = to
	bj, err := json.Marshal(briefing)
	if err != nil {
		log.Printf("[reconcile] marshal shift briefing: %v", err)
		bj = []byte("{}")
	}

	sh := store.ShiftHandoff{
		ID: uuid.New().String(), NetID: netID,
		FromCallsign: from, ToCallsign: to, At: now,
		OpenItemIDs: ids, Briefing: string(bj),
	}
	if err := m.store.SaveShiftHandoff(sh); err != nil {
		log.Printf("[reconcile] persist shift handoff: %v", err)
	}
	m.mu.Lock()
	m.transfers[netID] = append(m.transfers[netID], sh)
	m.mu.Unlock()

	m.logTimeline(netID, TimelineHandoff, to,
		fmt.Sprintf("NCS shift-relief: %s -> %s (%d open item(s) carried over)", from, to, len(ids)), "")
	m.emit(Event{Type: EventShiftHandoff, Data: sh})
}

// AcknowledgeHandoff acknowledges the most recent un-acknowledged
// ShiftHandoff for a net. Only one may be un-acked at a time in the normal
// flow, but a second transfer before the first is acked can leave more than
// one — the most recent is always the one acknowledged.
func (m *Manager) AcknowledgeHandoff(netID, by string) (*store.ShiftHandoff, error) {
	// now is read before taking the write lock below: m.clock() itself
	// takes a read lock on the same mutex, and sync.RWMutex is not
	// re-entrant — calling it while this goroutine already holds the write
	// lock deadlocks forever.
	now := m.clock()

	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.transfers[netID]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].AcknowledgedAt == nil {
			list[i].AcknowledgedAt = &now
			list[i].AcknowledgedBy = by
			if err := m.store.SaveShiftHandoff(list[i]); err != nil {
				return nil, fmt.Errorf("persist acknowledged handoff: %w", err)
			}
			out := list[i]
			return &out, nil
		}
	}
	return nil, errors.New("no unacknowledged shift handoff for this net")
}

// Handoffs returns shift-handoff history for a net, newest first. Never nil.
func (m *Manager) Handoffs(netID string) []store.ShiftHandoff {
	m.mu.RLock()
	out := append([]store.ShiftHandoff(nil), m.transfers[netID]...)
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if out == nil {
		out = []store.ShiftHandoff{}
	}
	return out
}
