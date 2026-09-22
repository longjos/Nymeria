package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/ride/phase"
)

// writeRidePhaseUnavailable answers a ride-phase route the same way every
// other feature-gated route in this codebase does when the manager was
// never wired up (checkpoint/ride/course 503 pattern).
func writeRidePhaseUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ride phase not available"})
}

// writeRidePhaseError maps a phase.Manager error to its HTTP status:
// phase.ErrNotFound -> 404, phase.ErrProfileMismatch -> 409 (a general net
// has no ride phase — the frontend's own profile check should have kept it
// from calling this at all), phase.ErrIllegalTransition -> 409, everything
// else (invalid target phase, missing backward-move reason) -> 400.
func writeRidePhaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, phase.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
	case errors.Is(err, phase.ErrProfileMismatch):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "profile_mismatch"})
	case errors.Is(err, phase.ErrIllegalTransition):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "illegal_transition"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

// handleGetRidePhase answers GET /nets/{id}/ride/phase: the net's current
// ride phase plus the live-computed suggestion.
func (s *Server) handleGetRidePhase(w http.ResponseWriter, r *http.Request) {
	if s.phaseMgr == nil {
		writeRidePhaseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	st, err := s.phaseMgr.GetState(id)
	if err != nil {
		writeRidePhaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// setRidePhaseRequest is the body for POST /nets/{id}/ride/phase.
type setRidePhaseRequest struct {
	Phase  string `json:"phase"`
	Reason string `json:"reason"`
}

// handleSetRidePhase answers POST /nets/{id}/ride/phase. Gated the same as
// SetProfile/CloseNet/TransferNCS: changing the ride phase is net-control
// work, restricted to the net's own NCS or an admin. Phase is never
// switched silently — this is the only way it ever moves.
func (s *Server) handleSetRidePhase(w http.ResponseWriter, r *http.Request) {
	if s.phaseMgr == nil {
		writeRidePhaseUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	if !s.allowNetControlAction(w, r, id, "change the ride phase") {
		return
	}

	var req setRidePhaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	st, err := s.phaseMgr.SetPhase(id, phase.SetInput{
		To:     req.Phase,
		By:     s.courseUserName(r),
		Reason: req.Reason,
	})
	if err != nil {
		writeRidePhaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
	s.logCourseActivity(r, "ride.phase.set", id, req.Phase)
}
