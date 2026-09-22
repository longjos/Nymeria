// ride_reconcile.go implements the bike-ride close-out and paper-trail
// routes (WP5): supported-rider accounting, the post-ride checklist, SAG
// driver shift summaries, NCS shift-relief handoff, and the ICS 211/214
// exports. It follows sag.go/course.go's 503-when-unwired pattern and
// ics309's Header/Report + ExportCSV shape.
package server

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/ics211"
	"github.com/narvel/nymeria/internal/ics214"
	"github.com/narvel/nymeria/internal/netprofile"
	"github.com/narvel/nymeria/internal/ride/reconcile"
	"github.com/narvel/nymeria/internal/store"
)

func writeReconcileUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ride reconciliation not available"})
}

// writeReconcileError maps a reconcile package error to its HTTP status:
// reconcile.ErrNotFound -> 404, reconcile.ErrIllegal -> 409, everything else
// (validation) -> 400.
func writeReconcileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, reconcile.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, reconcile.ErrIllegal):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "illegal_transition"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func (s *Server) reconcileUserName(r *http.Request) string {
	if user, ok := UserFromContext(r.Context()); ok {
		return user.Name
	}
	return ""
}

// --- Accounting / close-out (read, observer+) ---

func (s *Server) handleGetRideAccounting(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.recMgr.Accounting(id))
}

func (s *Server) handleGetRideCloseout(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.recMgr.Closeout(id))
}

// handlePostRideCloseout answers POST /nets/{id}/ride/closeout. Gated the
// same as ending a net (allowNetControlAction): closing the net is itself a
// net-control action regardless of which route triggers it.
func (s *Server) handlePostRideCloseout(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	if !s.allowNetControlAction(w, r, id, "end this net") {
		return
	}
	var body struct {
		Force  bool   `json:"force"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	n, summary, status, err := s.recMgr.CloseNet(id, s.reconcileUserName(r), body.Reason, body.Force)
	if err != nil {
		switch {
		case errors.Is(err, reconcile.ErrNotReady):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "close-out checklist incomplete", "closeout": status})
		case errors.Is(err, reconcile.ErrForceBlocked):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"net": n, "summary": summary, "closeout": status})
	s.logReconcileActivity(r, "ride.closeout", id, body.Reason)
}

func (s *Server) logReconcileActivity(r *http.Request, action, target, details string) {
	if s.actLogger == nil {
		return
	}
	user, _ := UserFromContext(r.Context())
	s.actLogger.Log(activity.Entry{
		Timestamp: time.Now(),
		UserID:    user.ID,
		UserName:  user.Name,
		Action:    action,
		Target:    target,
		Details:   details,
	})
}

// --- Shift summaries ---

func (s *Server) handleGetShiftSummaries(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.recMgr.ShiftSummaries(id))
}

func (s *Server) handleGetShiftSummary(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	view, ok := s.recMgr.GetShiftSummary(id, sid)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "shift summary not found"})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleCreateShiftSummary(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		CheckInID string `json:"checkInId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	view, err := s.recMgr.CreateShiftSummary(id, body.CheckInID, s.reconcileUserName(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, view)
	s.logReconcileActivity(r, "ride.shift_summary.create", view.ID, body.CheckInID)
}

func (s *Server) handleUpdateShiftSummary(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	var body store.SAGShiftSummary
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	body.ID = sid
	body.NetID = id
	view, err := s.recMgr.UpdateShiftSummary(id, body)
	if err != nil {
		writeReconcileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
	s.logReconcileActivity(r, "ride.shift_summary.update", sid, "")
}

func (s *Server) handleFileShiftSummary(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	view, err := s.recMgr.FileShiftSummary(id, sid, s.reconcileUserName(r))
	if err != nil {
		writeReconcileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
	s.logReconcileActivity(r, "ride.shift_summary.file", sid, "")
}

// handleExportShiftSummaryCSV writes the SLOBC SAG Driver Report as a
// two-column key/value CSV, field names verbatim from the paper form, with
// a Source column (entered|derived) on every count.
func (s *Server) handleExportShiftSummaryCSV(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	view, ok := s.recMgr.GetShiftSummary(id, sid)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "shift summary not found"})
		return
	}

	name := view.TacticalCall
	if name == "" {
		name = view.Callsign
	}
	if name == "" {
		name = sid
	}
	filename := fmt.Sprintf("sag-shift-%s.csv", sanitizeFilenamePart(name))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	cw.Write([]string{"SAG Driver Report"})
	cw.Write([]string{"Driver/Unit", name})
	cw.Write([]string{"Odometer at start", floatOrBlank(view.OdometerStart), ""})
	cw.Write([]string{"Odometer at end", floatOrBlank(view.OdometerEnd), ""})
	cw.Write([]string{"Net mileage on SAG duty", floatOrBlank(view.NetMiles), ""})

	rows := []struct {
		label   string
		entered *int
		eff     int
	}{
		{"Number of transports", view.Entered.Transports, view.Effective.Transports},
		{"Number of assists (repairs, flats)", view.Entered.Assists, view.Effective.Assists},
		{"Number of tubes provided", view.Entered.TubesProvided, view.Effective.TubesProvided},
		{"Number of tires provided", view.Entered.TiresProvided, view.Effective.TiresProvided},
		{"Number of minor first aid assists", view.Entered.MinorFirstAid, view.Effective.MinorFirstAid},
		{"Number of incidents attended", view.Entered.IncidentsAttended, view.Effective.IncidentsAttended},
	}
	for _, row := range rows {
		source := "derived"
		if row.entered != nil {
			source = "entered"
		}
		cw.Write([]string{row.label, strconv.Itoa(row.eff), source})
	}
	cw.Write([]string{"Notes", view.Notes, ""})
}

func floatOrBlank(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}

func sanitizeFilenamePart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// --- Hand-off ---

func (s *Server) handleGetHandoffItems(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	status := r.URL.Query().Get("status")
	if status == "" {
		status = reconcile.HandoffOpen
	}
	writeJSON(w, http.StatusOK, s.recMgr.HandoffItems(id, status))
}

func (s *Server) handleGetShiftHandoffs(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.recMgr.Handoffs(id))
}

func (s *Server) handleAddHandoffItem(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	var body store.HandoffItem
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	body.NetID = id
	body.CreatedBy = s.reconcileUserName(r)
	item, err := s.recMgr.AddHandoffItem(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, item)
	s.logReconcileActivity(r, "ride.handoff.add", item.ID, item.Summary)
}

func (s *Server) handleUpdateHandoffItem(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	hid := chi.URLParam(r, "hid")
	var body struct {
		Action     string `json:"action"` // resolve|cancel
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	var cancel bool
	switch body.Action {
	case "resolve":
		cancel = false
	case "cancel":
		cancel = true
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be \"resolve\" or \"cancel\""})
		return
	}
	item, err := s.recMgr.ResolveHandoffItem(id, hid, s.reconcileUserName(r), body.Resolution, cancel)
	if err != nil {
		writeReconcileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
	s.logReconcileActivity(r, "ride.handoff."+body.Action, hid, body.Resolution)
}

// handleAckHandoff answers POST /nets/{id}/ride/handoff/ack. Gated by
// allowNetControlAction: only the current NCS (or an admin) acknowledges
// their own briefing.
func (s *Server) handleAckHandoff(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	if !s.allowNetControlAction(w, r, id, "acknowledge the shift-relief briefing") {
		return
	}
	sh, err := s.recMgr.AcknowledgeHandoff(id, s.reconcileUserName(r))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sh)
	s.logReconcileActivity(r, "ride.handoff.ack", id, "")
}

func (s *Server) handleGetBriefing(w http.ResponseWriter, r *http.Request) {
	if s.recMgr == nil {
		writeReconcileUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.recMgr.Briefing(id))
}

// --- ICS 211 ---

func (s *Server) ics211Header(r *http.Request, netID string) ics211.Header {
	q := r.URL.Query()
	h := ics211.Header{
		IncidentName:   q.Get("incidentName"),
		IncidentNumber: q.Get("incidentNumber"),
		PreparedBy:     q.Get("preparedBy"),
		PreparedAt:     time.Now().UTC(),
	}
	if h.PreparedBy == "" {
		h.PreparedBy = s.reconcileUserName(r)
	}
	if s.netMgr != nil {
		if n, ok := s.netMgr.GetNet(netID); ok {
			if h.IncidentName == "" {
				h.IncidentName = n.Name
			}
			h.CheckInLocation = n.Name
			if n.Frequency != "" {
				h.CheckInLocation = fmt.Sprintf("%s / %s", n.Name, n.Frequency)
			}
			if n.OpenedAt != nil {
				h.StartDateTime = *n.OpenedAt
			}
		}
	}
	return h
}

func (s *Server) rideAgencyName(netID string) string {
	if s.netMgr == nil {
		return ""
	}
	if cfg, ok := s.netMgr.GetRideConfig(netID); ok && cfg != nil {
		return cfg.AgencyName
	}
	return ""
}

func (s *Server) handleGetICS211(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}
	id := chi.URLParam(r, "id")
	report := ics211.Build(s.ics211Header(r, id), s.netMgr.GetCheckIns(id), s.rideAgencyName(id))
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleExportICS211CSV(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		http.Error(w, "net control not available", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	report := ics211.Build(s.ics211Header(r, id), s.netMgr.GetCheckIns(id), s.rideAgencyName(id))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=ics211.csv")
	ics211.ExportCSV(w, report)
}

// --- ICS 214 ---

// ics214TimeRange mirrors parseICS309TimeRange's contract but defaults to
// the net's own OpenedAt/ClosedAt (or now) rather than a rolling 24h.
func ics214TimeRange(n *store.Net, fromStr, toStr string) (time.Time, time.Time) {
	now := time.Now().UTC()
	from, to := now.Add(-24*time.Hour), now
	if n != nil {
		if n.OpenedAt != nil {
			from = *n.OpenedAt
		}
		if n.ClosedAt != nil {
			to = *n.ClosedAt
		}
	}
	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}
	return from, to
}

// buildICS214 resolves the variant/unit into a Report, or an error string
// for a 400 (variant=sag with no unit, or variant=rs whose annotation is
// not a rest stop).
func (s *Server) buildICS214(r *http.Request, netID string) (ics214.Report, string) {
	q := r.URL.Query()
	variant := q.Get("variant")
	if variant == "" {
		variant = ics214.VariantGeneral
	}
	unit := q.Get("unit")

	var n *store.Net
	if s.netMgr != nil {
		if got, ok := s.netMgr.GetNet(netID); ok {
			n = got
		}
	}
	from, to := ics214TimeRange(n, q.Get("from"), q.Get("to"))

	h := ics214.Header{
		IncidentName:    q.Get("incidentName"),
		IncidentNumber:  q.Get("incidentNumber"),
		OperationalFrom: from, OperationalTo: to,
		HomeAgency: s.rideAgencyName(netID),
		Variant:    variant,
		PreparedBy: s.reconcileUserName(r),
		PreparedAt: time.Now().UTC(),
	}
	if n != nil && h.IncidentName == "" {
		h.IncidentName = n.Name
	}

	filter := ics214.UnitFilter{}
	switch variant {
	case ics214.VariantSAG:
		if unit == "" {
			return ics214.Report{}, "unit (checkInId) is required for variant=sag"
		}
		filter.CheckInID = unit
		h.ICSPosition = "SAG Driver"
		h.Name = unit
		if s.netMgr != nil {
			for _, ci := range s.netMgr.GetCheckIns(netID) {
				if ci.ID == unit {
					if ci.TacticalCall != "" {
						filter.Callsigns = append(filter.Callsigns, ci.TacticalCall)
						h.Name = ci.TacticalCall
					}
					filter.Callsigns = append(filter.Callsigns, ci.Callsign)
					h.ResourcesAssigned = []ics214.Resource{{Name: ci.Callsign, ICSPosition: "SAG Driver", HomeAgency: h.HomeAgency}}
				}
			}
		}
	case ics214.VariantRS:
		if unit == "" {
			return ics214.Report{}, "unit (annotationId) is required for variant=rs"
		}
		filter.AnnotationID = unit
		h.ICSPosition = "Rest Stop Communicator"
		h.Name = unit
		if s.annMgr != nil {
			if ann, ok := s.annMgr.Get(unit); ok {
				if ann.Category != annotation.CategoryAid {
					return ics214.Report{}, fmt.Sprintf("annotation %q is not a rest stop (category %q)", unit, ann.Category)
				}
				label := ann.ShortName
				if label == "" {
					label = ann.Label
				}
				h.Name = label
				filter.Labels = append(filter.Labels, ann.Label)
				if ann.ShortName != "" {
					filter.Labels = append(filter.Labels, ann.ShortName)
				}
			}
		}
	default:
		h.ICSPosition = "Net Control"
		if n != nil {
			h.Name = n.NCSCallsign
		}
	}
	if h.ResourcesAssigned == nil {
		h.ResourcesAssigned = []ics214.Resource{}
	}

	var events []store.NetEvent
	var notes []store.NetNote
	if s.netMgr != nil {
		events, _ = s.netMgr.GetEvents(netID)
		notes, _ = s.netMgr.GetNotes(netID)
	}
	rows := ics214.BuildRows(events, notes, from, to, filter)

	return ics214.Report{Header: h, Rows: rows}, ""
}

func (s *Server) handleGetICS214(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	report, errMsg := s.buildICS214(r, id)
	if errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleExportICS214CSV(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	report, errMsg := s.buildICS214(r, id)
	if errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}
	filename := "ics214-" + report.Header.Variant
	if report.Header.Name != "" {
		filename += "-" + sanitizeFilenamePart(report.Header.Name)
	}
	filename += ".csv"
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	ics214.ExportCSV(w, report)
}

// --- Modification hook for POST /nets/{id}/close ---

// rideCloseoutGate reports whether closing netID through the general
// /nets/{id}/close route must first pass the ride-mode close-out checklist:
// only for nets running the "bike-ride" profile, so a general net's
// behavior never changes. Returns (ready, applicable).
func (s *Server) rideCloseoutGate(netID string) (ready bool, applicable bool) {
	if s.recMgr == nil || s.netMgr == nil {
		return true, false
	}
	n, ok := s.netMgr.GetNet(netID)
	if !ok || n.Profile != netprofile.ProfileBikeRide {
		return true, false
	}
	status := s.recMgr.Closeout(netID)
	return status.Ready, true
}
