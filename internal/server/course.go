package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/store"
)

// writeCourseUnavailable answers a course-closure route the same way every
// other feature-gated route in this codebase does when the manager was
// never wired up (checkpoint/ride/wx-alert 503 pattern).
func writeCourseUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "course closure not available"})
}

// courseUserName returns the session user's name for reportedBy/by fields,
// or "" when unauthenticated.
func (s *Server) courseUserName(r *http.Request) string {
	if user, ok := UserFromContext(r.Context()); ok {
		return user.Name
	}
	return ""
}

// isNetNCSOrAdmin reports whether the session user may exercise an
// NCS/Admin-only override for netID. It mirrors allowNetControlAction's
// orphaned-NCS handling (a recorded NCS whose session is gone no longer
// blocks anyone) but only reports the answer — it never writes a response,
// so callers can shape their own error body.
func (s *Server) isNetNCSOrAdmin(r *http.Request, netID string) bool {
	user, ok := UserFromContext(r.Context())
	if !ok {
		return false
	}
	if session.RoleLevel(user.Role) >= session.RoleLevel(session.RoleAdmin) {
		return true
	}
	if s.netMgr == nil {
		return false
	}
	n, ok := s.netMgr.GetNet(netID)
	if !ok {
		return false
	}
	if n.NCSUserID == "" || n.NCSUserID == user.ID {
		return true
	}
	if s.sessions != nil {
		if _, live := s.sessions.GetByID(n.NCSUserID); !live {
			return true
		}
	}
	return false
}

func (s *Server) logCourseActivity(r *http.Request, action, target, details string) {
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

// writeCourseShutoffError maps a shutoff-transition error to its HTTP
// status: course.ErrNotFound -> 404, course.ErrIllegalTransition -> 409
// code "not_planned" (every shutoff mutation but create requires planned),
// everything else -> 400 (a validation error).
func writeCourseShutoffError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, course.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "shutoff not found"})
	case errors.Is(err, course.ErrIllegalTransition):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "shutoff is not in planned status", "code": "not_planned"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

// writeCourseStationError maps a station-ladder error (other than the sweep
// gate, which handleStationClose shapes itself with more context) to its
// HTTP status.
func writeCourseStationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, course.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "station not found"})
	case errors.Is(err, course.ErrIllegalTransition):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "illegal_transition"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

// --- Read endpoints (observer+) ---

func (s *Server) handleGetCourseState(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	state, err := s.courseMgr.State(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleGetCourseConfig(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.courseMgr.GetConfig(id))
}

func (s *Server) handleGetShutoffs(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.courseMgr.GetShutoffs(id))
}

// handleGetRiders answers GET /nets/{id}/course/riders. ?status filters to
// supported|unsupported.
func (s *Server) handleGetRiders(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	riders := s.courseMgr.GetRiders(id)
	if status := r.URL.Query().Get("status"); status != "" {
		filtered := make([]store.RiderException, 0, len(riders))
		for _, ri := range riders {
			if ri.SupportStatus == status {
				filtered = append(filtered, ri)
			}
		}
		riders = filtered
	}
	writeJSON(w, http.StatusOK, riders)
}

func (s *Server) handleGetSweep(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	limit := 20
	if lp := r.URL.Query().Get("limit"); lp != "" {
		if n, err := strconv.Atoi(lp); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"position": s.courseMgr.SweepPosition(id),
		"reports":  s.courseMgr.GetSweepReports(id, limit),
	})
}

func (s *Server) handleGetCourseStations(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	state, err := s.courseMgr.State(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state.Stations)
}

// --- Write endpoints (operator+) ---

// handlePutCourseConfig decodes onto the net's current config (the
// handleUpdateSAGRequest/handleUpdateMission idiom) so a field the client
// omits keeps its current value; blank labels still fall back to their
// package defaults inside SetConfig.
func (s *Server) handlePutCourseConfig(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	body := s.courseMgr.GetConfig(id)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	body.NetID = id
	cfg, err := s.courseMgr.SetConfig(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
	s.logCourseActivity(r, "course.config.update", id, "")
}

func (s *Server) handleCreateShutoff(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	var sp store.ShutoffPoint
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	sp.NetID = id
	created, err := s.courseMgr.CreateShutoff(sp)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
	s.logCourseActivity(r, "course.shutoff.create", created.ID, created.Name)
}

func (s *Server) handleUpdateShutoff(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sID := chi.URLParam(r, "sId")
	var sp store.ShutoffPoint
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	sp.NetID = id
	sp.ID = sID
	updated, err := s.courseMgr.UpdateShutoff(sp)
	if err != nil {
		writeCourseShutoffError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
	s.logCourseActivity(r, "course.shutoff.update", sID, "")
}

func (s *Server) handleDeleteShutoff(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sID := chi.URLParam(r, "sId")
	if err := s.courseMgr.DeleteShutoff(id, sID); err != nil {
		writeCourseShutoffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	s.logCourseActivity(r, "course.shutoff.delete", sID, "")
}

func (s *Server) handleFireShutoff(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sID := chi.URLParam(r, "sId")
	var body struct {
		Note         string   `json:"note"`
		By           string   `json:"by"`
		Bibs         []string `json:"bibs"`
		RerouteCount int      `json:"rerouteCount"`
		BibWithheld  bool     `json:"bibWithheld"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	by := s.courseUserName(r)
	if by == "" {
		by = body.By
	}
	sp, riders, err := s.courseMgr.FireShutoff(id, sID, course.FireInput{
		By: by, Note: body.Note, Bibs: body.Bibs, RerouteCount: body.RerouteCount, BibWithheld: body.BibWithheld,
	})
	if err != nil {
		writeCourseShutoffError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shutoff": sp, "riders": riders})
	s.logCourseActivity(r, "course.shutoff.fire", sID, fmt.Sprintf(`{"bibs":%d,"rerouteCount":%d}`, len(body.Bibs), body.RerouteCount))
}

func (s *Server) handleCancelShutoff(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sID := chi.URLParam(r, "sId")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	sp, err := s.courseMgr.CancelShutoff(id, sID, s.courseUserName(r), body.Reason)
	if err != nil {
		writeCourseShutoffError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sp)
	s.logCourseActivity(r, "course.shutoff.cancel", sID, body.Reason)
}

// handleReinstateShutoff serves the single /reinstate route for both
// directions the design's state machine allows back to planned:
// cancelled->planned (any operator) and fired->planned (NCS/Admin only,
// reason required) — dispatched here on the shutoff's current status since
// internal/course exposes them as two distinct manager methods.
func (s *Server) handleReinstateShutoff(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sID := chi.URLParam(r, "sId")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var current *store.ShutoffPoint
	for _, sp := range s.courseMgr.GetShutoffs(id) {
		if sp.ID == sID {
			c := sp
			current = &c
			break
		}
	}
	if current == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "shutoff not found"})
		return
	}

	by := s.courseUserName(r)
	var (
		sp  *store.ShutoffPoint
		err error
	)
	if current.Status == course.ShutoffFired {
		if !s.isNetNCSOrAdmin(r, id) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "only net control or an admin can reinstate a fired shutoff",
				"code":  "ncs_or_admin_required",
			})
			return
		}
		if strings.TrimSpace(body.Reason) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason is required"})
			return
		}
		sp, err = s.courseMgr.UnfireShutoff(id, sID, by, body.Reason)
	} else {
		sp, err = s.courseMgr.ReinstateShutoff(id, sID, by)
	}
	if err != nil {
		writeCourseShutoffError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sp)
	s.logCourseActivity(r, "course.shutoff.reinstate", sID, body.Reason)
}

func (s *Server) handleRecordRider(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	var ri store.RiderException
	if err := json.NewDecoder(r.Body).Decode(&ri); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	ri.NetID = id
	if ri.ReportedBy == "" {
		ri.ReportedBy = s.courseUserName(r)
	}
	created, err := s.courseMgr.RecordRider(ri)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
	s.logCourseActivity(r, "course.rider.record", created.ID, created.Kind)
}

func (s *Server) handleSetRiderStatus(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	rID := chi.URLParam(r, "rId")
	var body struct {
		SupportStatus string `json:"supportStatus"`
		Reason        string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	updated, err := s.courseMgr.SetRiderSupport(id, rID, body.SupportStatus, s.courseUserName(r), body.Reason)
	if err != nil {
		if errors.Is(err, course.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "rider exception not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
	s.logCourseActivity(r, "course.rider.status", rID, body.SupportStatus)
}

func (s *Server) handleReportSweep(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	var sr store.SweepReport
	if err := json.NewDecoder(r.Body).Decode(&sr); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	sr.NetID = id
	if sr.ReportedBy == "" {
		sr.ReportedBy = s.courseUserName(r)
	}
	created, err := s.courseMgr.ReportSweep(sr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
	s.logCourseActivity(r, "course.sweep.report", created.ID, created.LastRiderBib)
}

func (s *Server) handleStationRidersClear(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	cpID := chi.URLParam(r, "cpId")
	c, err := s.courseMgr.ReportRidersClear(id, cpID, s.courseUserName(r))
	if err != nil {
		writeCourseStationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
	s.logCourseActivity(r, "course.station.riders_clear", cpID, "")
}

func (s *Server) handleStationSweepPassed(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	cpID := chi.URLParam(r, "cpId")
	c, err := s.courseMgr.MarkSweepPassed(id, cpID, s.courseUserName(r), "")
	if err != nil {
		writeCourseStationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
	s.logCourseActivity(r, "course.station.sweep_passed", cpID, "")
}

func (s *Server) handleStationClose(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	cpID := chi.URLParam(r, "cpId")
	var body struct {
		Override bool   `json:"override"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Override && strings.TrimSpace(body.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "override reason is required"})
		return
	}

	actorIsNCS := false
	if body.Override {
		actorIsNCS = s.isNetNCSOrAdmin(r, id)
		if !actorIsNCS {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "only net control or an admin can override the sweep gate",
				"code":  "ncs_or_admin_required",
			})
			return
		}
	}

	c, err := s.courseMgr.CloseStation(id, cpID, course.CloseInput{
		By: s.courseUserName(r), Override: body.Override, OverrideReason: body.Reason, ActorIsNCSOrAdmin: actorIsNCS,
	})
	if err != nil {
		switch {
		case errors.Is(err, course.ErrSweepNotPassed):
			label := cpID
			if s.annMgr != nil {
				if a, ok := s.annMgr.Get(cpID); ok {
					label = a.Label
				}
			}
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":         "rest stop cannot close: sweep has not passed it",
				"code":          "sweep_not_passed",
				"sweepPassedAt": nil,
				"stationLabel":  label,
			})
		case errors.Is(err, course.ErrOverrideNotAllowed):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error(), "code": "ncs_or_admin_required"})
		default:
			writeCourseStationError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, c)

	action := "course.station.close"
	details := ""
	if body.Override {
		action = "course.station.close_override"
		details = body.Reason
	}
	s.logCourseActivity(r, action, cpID, details)
}

func (s *Server) handleStationReopen(w http.ResponseWriter, r *http.Request) {
	if s.courseMgr == nil {
		writeCourseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	cpID := chi.URLParam(r, "cpId")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason is required"})
		return
	}
	c, err := s.courseMgr.ReopenStation(id, cpID, s.courseUserName(r), body.Reason)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, c)
	s.logCourseActivity(r, "course.station.reopen", cpID, body.Reason)
}
