package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/netprofile"
	"github.com/narvel/nymeria/internal/store"
)

// setNetProfileRequest is the body for PUT /api/nets/{id}/profile.
type setNetProfileRequest struct {
	Profile string `json:"profile"`
}

// writeNetControlUnavailable answers a net-profile route the same way every
// other /nets/* handler does when net control was never wired up.
func writeNetControlUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
}

// --- Read endpoints (observer+) ---

// handleGetNetProfiles answers GET /api/net-profiles: the whole registry, in
// stable order. Never empty — it does not depend on s.netMgr.
func (s *Server) handleGetNetProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, netprofile.All())
}

// handleGetNetProfileByID answers GET /api/net-profiles/{id}.
func (s *Server) handleGetNetProfileByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := netprofile.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleGetNetProfileView answers GET /api/nets/{id}/profile: everything the
// frontend needs to mount a net in one round trip.
func (s *Server) handleGetNetProfileView(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeNetControlUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	view, err := s.netMgr.ProfileView(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleGetRideConfig answers GET /api/nets/{id}/ride-config. A bike-ride
// net always has a row (CreateNet/SetProfile seed one), so the only failure
// modes are the net not existing at all, or the net's profile not being one
// that carries ride config.
func (s *Server) handleGetRideConfig(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeNetControlUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.netMgr.GetNet(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		return
	}
	cfg, ok := s.netMgr.GetRideConfig(id)
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": netcontrol.ErrProfileMismatch.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// --- Write endpoints (operator+) ---

// handlePutNetProfile answers PUT /api/nets/{id}/profile. Gated the same as
// ending/handing over the net — changing the vocabulary a net speaks is
// net-control work.
func (s *Server) handlePutNetProfile(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeNetControlUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.netMgr.GetNet(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		return
	}
	if !s.allowNetControlAction(w, r, id, "change the net profile") {
		return
	}

	var req setNetProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if _, err := s.netMgr.SetProfile(id, req.Profile); err != nil {
		switch {
		case errors.Is(err, netcontrol.ErrInvalidProfile):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, netcontrol.ErrNotDraft):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		}
		return
	}

	view, err := s.netMgr.ProfileView(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handlePutRideConfig answers PUT /api/nets/{id}/ride-config. The body
// decodes directly into store.NetRideConfig; netId and updatedAt are always
// taken from the URL/server, never the client, so a spoofed body cannot
// retarget the write or fake a save time.
func (s *Server) handlePutRideConfig(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeNetControlUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.netMgr.GetNet(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		return
	}
	if !s.allowNetControlAction(w, r, id, "change the ride configuration") {
		return
	}

	var body store.NetRideConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg, err := s.netMgr.SetRideConfig(id, body)
	if err != nil {
		switch {
		case errors.Is(err, netcontrol.ErrInvalidRideConfig):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, netcontrol.ErrProfileMismatch), errors.Is(err, netcontrol.ErrNetClosed):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		}
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}
