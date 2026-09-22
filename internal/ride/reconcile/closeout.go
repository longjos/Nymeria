package reconcile

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/store"
)

// closeoutItemOrder is the fixed, documented order of the post-ride
// checklist. Ready is computed from this list minus net_closed.
var closeoutItemOrder = []string{
	"sweep_finished",
	"exceptions_resolved",
	"rest_stops_closed",
	"field_units_out",
	"handoff_cleared",
	"shift_summaries_filed",
	"net_closed",
}

func (m *Manager) restStopsStatus(netID string) (done bool, count, total int, detail string) {
	if m.annMgr == nil {
		return false, 0, 0, "annotations not available"
	}
	var stillActive []string
	for _, a := range m.annMgr.AllForNet(netID) {
		if a.Category != annotation.CategoryAid {
			continue
		}
		total++
		if a.Status == "closed" {
			count++
		} else {
			label := a.ShortName
			if label == "" {
				label = a.Label
			}
			stillActive = append(stillActive, label)
		}
	}
	if total == 0 {
		return true, 0, 0, "no rest stops"
	}
	if len(stillActive) == 0 {
		return true, count, total, ""
	}
	sort.Strings(stillActive)
	if len(stillActive) > 5 {
		stillActive = append(stillActive[:5], "…")
	}
	return false, count, total, strings.Join(stillActive, ", ") + " still active"
}

func (m *Manager) fieldUnitsStatus(netID string) (done bool, count, total int, detail string) {
	if m.netMgr == nil {
		return false, 0, 0, "net control not available"
	}
	var stillIn []string
	for _, ci := range m.netMgr.GetCheckIns(netID) {
		if ci.Category == netcontrol.CatCommand {
			continue
		}
		total++
		if ci.Status == netcontrol.OpReleased {
			count++
		} else {
			label := ci.TacticalCall
			if label == "" {
				label = ci.Callsign
			}
			stillIn = append(stillIn, label)
		}
	}
	if total == 0 {
		return true, 0, 0, "no field units"
	}
	if len(stillIn) == 0 {
		return true, count, total, ""
	}
	sort.Strings(stillIn)
	if len(stillIn) > 5 {
		stillIn = append(stillIn[:5], "…")
	}
	return false, count, total, strings.Join(stillIn, ", ") + " still checked in"
}

func (m *Manager) handoffClearedStatus(netID string) (done bool, detail string) {
	open := m.HandoffItems(netID, HandoffOpen)
	awaiting := m.awaitingReplies(netID)
	if len(open) == 0 && len(awaiting) == 0 {
		return true, ""
	}
	return false, fmt.Sprintf("%d open item(s), %d awaiting reply", len(open), len(awaiting))
}

func (m *Manager) shiftSummariesStatus(netID string) (done bool, count, total int, detail string) {
	if m.netMgr == nil {
		return false, 0, 0, "net control not available"
	}
	sagUnits := map[string]bool{}
	for _, ci := range m.netMgr.GetCheckIns(netID) {
		if ci.Category == netcontrol.CatSAG {
			sagUnits[ci.ID] = true
		}
	}
	total = len(sagUnits)
	filed := map[string]bool{}
	for _, sm := range m.ShiftSummaries(netID) {
		if sm.Status == ShiftFiled {
			filed[sm.CheckInID] = true
		}
	}
	for id := range sagUnits {
		if filed[id] {
			count++
		}
	}
	if total == 0 {
		return true, 0, 0, "no sag units"
	}
	if count == total {
		return true, count, total, ""
	}
	return false, count, total, fmt.Sprintf("%d of %d sag shift summaries filed", count, total)
}

// Closeout computes the post-ride checklist for a net.
func (m *Manager) Closeout(netID string) CloseoutStatus {
	p := m.PolicyFor(netID)
	items := make([]CloseoutItem, 0, len(closeoutItemOrder))

	sweepDone, sweepDetail := m.sweepStatus(netID)
	items = append(items, CloseoutItem{
		Key: "sweep_finished", Label: "Sweep has passed the last station",
		Done: sweepDone, Blocking: true, Detail: sweepDetail,
	})

	acc := m.Accounting(netID)
	excTotal := acc.Counts.Supported + acc.Counts.Unsupported
	items = append(items, CloseoutItem{
		Key: "exceptions_resolved", Label: "All supported riders have returned",
		Done: acc.Counts.Supported == 0, Blocking: true,
		Count: acc.Counts.Unsupported, Total: excTotal,
		Detail: fmt.Sprintf("%d still open", acc.Counts.Supported),
	})

	restDone, restCount, restTotal, restDetail := m.restStopsStatus(netID)
	items = append(items, CloseoutItem{
		Key: "rest_stops_closed", Label: "All rest stops closed",
		Done: restDone, Blocking: p.RequireRestStopsClosed,
		Count: restCount, Total: restTotal, Detail: restDetail,
	})

	fuDone, fuCount, fuTotal, fuDetail := m.fieldUnitsStatus(netID)
	items = append(items, CloseoutItem{
		Key: "field_units_out", Label: "All field units formally checked out",
		Done: fuDone, Blocking: p.RequireFieldUnitsOut,
		Count: fuCount, Total: fuTotal, Detail: fuDetail,
	})

	hoDone, hoDetail := m.handoffClearedStatus(netID)
	items = append(items, CloseoutItem{
		Key: "handoff_cleared", Label: "No pending hand-off items",
		Done: hoDone, Blocking: false, Detail: hoDetail,
	})

	ssDone, ssCount, ssTotal, ssDetail := m.shiftSummariesStatus(netID)
	items = append(items, CloseoutItem{
		Key: "shift_summaries_filed", Label: "All SAG shift summaries filed",
		Done: ssDone, Blocking: p.RequireShiftSummaries,
		Count: ssCount, Total: ssTotal, Detail: ssDetail,
	})

	netClosed := false
	if m.netMgr != nil {
		if n, ok := m.netMgr.GetNet(netID); ok {
			netClosed = n.Status == netcontrol.StatusClosed
		}
	}
	items = append(items, CloseoutItem{
		Key: "net_closed", Label: "Net formally closed",
		Done: netClosed, Blocking: true,
	})

	ready := true
	for _, it := range items {
		if it.Key == "net_closed" {
			continue
		}
		if it.Blocking && !it.Done {
			ready = false
			break
		}
	}

	return CloseoutStatus{NetID: netID, Ready: ready, Items: items, ComputedAt: m.clock()}
}

// pendingKeys returns the keys of every blocking, not-done item (excluding
// net_closed) in a CloseoutStatus, for the forced-close timeline entry.
func pendingKeys(status CloseoutStatus) []string {
	keys := []string{}
	for _, it := range status.Items {
		if it.Key == "net_closed" {
			continue
		}
		if it.Blocking && !it.Done {
			keys = append(keys, it.Key)
		}
	}
	return keys
}

// CloseNet gates netcontrol.Manager.CloseNet on the post-ride checklist.
// Ready nets close normally. A not-ready net without force returns
// ErrNotReady alongside the checklist so the caller can show it; force
// requires Policy.AllowForceClose and a non-empty reason, and logs a
// TimelineCloseoutForced entry naming the pending items before closing.
func (m *Manager) CloseNet(netID, by, reason string, force bool) (*store.Net, *netcontrol.NetSummary, CloseoutStatus, error) {
	status := m.Closeout(netID)
	if m.netMgr == nil {
		return nil, nil, status, errors.New("net control not available")
	}

	if !status.Ready {
		if !force {
			return nil, nil, status, ErrNotReady
		}
		if strings.TrimSpace(reason) == "" {
			return nil, nil, status, errors.New("reason is required to force-close an incomplete checklist")
		}
		if !m.PolicyFor(netID).AllowForceClose {
			return nil, nil, status, ErrForceBlocked
		}
		pending := pendingKeys(status)
		details, _ := json.Marshal(map[string]any{"pending": pending, "reason": reason})
		m.logTimeline(netID, TimelineCloseoutForced, by,
			fmt.Sprintf("net force-closed with checklist incomplete (%s): %s", strings.Join(pending, ", "), reason),
			string(details))
	}

	n, summary, err := m.netMgr.CloseNet(netID)
	if err != nil {
		return nil, nil, status, err
	}
	final := m.Closeout(netID)
	return n, summary, final, nil
}
