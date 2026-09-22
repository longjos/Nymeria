package reconcile

import (
	"sort"
	"time"

	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/store"
)

// awaitingReplies derives "messages you have sent and replies you expect"
// from the wired message engine: every outbound message not yet acked
// (rejected/failed are dead ends, not something to expect a reply to),
// sent since the net opened, oldest first. Empty, never nil, when no
// message engine is wired.
func (m *Manager) awaitingReplies(netID string) []AwaitingReply {
	m.mu.RLock()
	eng := m.msgs
	m.mu.RUnlock()

	out := []AwaitingReply{}
	if eng == nil {
		return out
	}

	var cutoff time.Time
	if m.netMgr != nil {
		if n, ok := m.netMgr.GetNet(netID); ok && n.OpenedAt != nil {
			cutoff = *n.OpenedAt
		}
	}

	for _, msg := range eng.Messages("") {
		if msg.Inbound {
			continue
		}
		if !cutoff.IsZero() && msg.Timestamp.Before(cutoff) {
			continue
		}
		var state string
		switch msg.State {
		case message.StatePending:
			state = "pending"
		case message.StateSent:
			state = "sent"
		default:
			continue
		}
		out = append(out, AwaitingReply{
			MessageID: msg.ID, To: msg.To, Body: msg.Body, SentAt: msg.Timestamp, State: state,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SentAt.Before(out[j].SentAt) })
	return out
}

// briefingSections concatenates every registered BriefingSource's sections,
// in registration order, normalizing a nil Lines slice to empty.
func (m *Manager) briefingSections(netID string) []BriefingSection {
	m.mu.RLock()
	sources := append([]BriefingSource(nil), m.briefing...)
	m.mu.RUnlock()

	out := []BriefingSection{}
	for _, src := range sources {
		for _, sec := range src.BriefingSections(netID) {
			if sec.Lines == nil {
				sec.Lines = []BriefingLine{}
			}
			out = append(out, sec)
		}
	}
	return out
}

// Briefing builds the live shift-relief briefing for a net: who has the
// net, when the next shift change is due, every pending hand-off item and
// awaiting reply, current accounting/close-out status, and the last 30
// minutes of timeline activity.
func (m *Manager) Briefing(netID string) ShiftBriefing {
	now := m.clock()

	var netName, ncsCallsign string
	var ncsSince *time.Time
	if m.netMgr != nil {
		if n, ok := m.netMgr.GetNet(netID); ok {
			netName = n.Name
			ncsCallsign = n.NCSCallsign
			ncsSince = n.OpenedAt
		}
	}
	if transfers := m.Handoffs(netID); len(transfers) > 0 {
		t := transfers[0].At
		ncsSince = &t
	}

	p := m.PolicyFor(netID)
	var shiftDue *time.Time
	if ncsSince != nil {
		due := ncsSince.Add(time.Duration(p.NCSShiftMinutes) * time.Minute)
		shiftDue = &due
	}

	recent := []store.NetEvent{}
	if m.netMgr != nil {
		if evts, err := m.netMgr.GetEvents(netID); err == nil {
			cutoff := now.Add(-30 * time.Minute)
			for _, e := range evts {
				if e.CreatedAt.After(cutoff) {
					recent = append(recent, e)
				}
			}
		}
	}

	return ShiftBriefing{
		NetID:           netID,
		NetName:         netName,
		NCSCallsign:     ncsCallsign,
		NCSSince:        ncsSince,
		ShiftDueAt:      shiftDue,
		OpenItems:       m.HandoffItems(netID, HandoffOpen),
		AwaitingReplies: m.awaitingReplies(netID),
		Accounting:      m.Accounting(netID),
		Closeout:        m.Closeout(netID),
		Sections:        m.briefingSections(netID),
		RecentEvents:    recent,
		GeneratedAt:     now,
	}
}
