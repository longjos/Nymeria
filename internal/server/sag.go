package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/store"
)

// writeRideUnavailable answers a ride-mode route the same way every other
// feature-gated route in this file does when the manager was never wired up
// (checkpoint/wx-alert 503 pattern).
func writeRideUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ride mode not available"})
}

// writeSAGError maps a ride package error to the right HTTP status:
// ride.ErrNotFound -> 404, an *ride.OverCapacityError -> 409 with the
// numbers the client needs to show, everything else -> 400 (a manager
// validation error).
func writeSAGError(w http.ResponseWriter, err error) {
	var capErr *ride.OverCapacityError
	switch {
	case errors.As(err, &capErr):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":          "vehicle over capacity",
			"committedSeats": capErr.CommittedSeats,
			"seats":          capErr.Seats,
			"committedRacks": capErr.CommittedRacks,
			"rackSlots":      capErr.RackSlots,
		})
	case errors.Is(err, ride.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

// sagUserName returns the session user's callsign/name for byCallsign
// parameters and requestedBy defaults, or "" when unauthenticated (unused
// server-to-server calls).
func (s *Server) sagUserName(r *http.Request) string {
	if user, ok := UserFromContext(r.Context()); ok {
		return user.Name
	}
	return ""
}

// --- Read endpoints (observer+) ---

func (s *Server) handleGetSAGBoard(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.rideMgr.Board(id))
}

// handleGetSAGRequests answers GET /nets/{id}/sag/requests. ?status is a
// comma list filter (e.g. "open,partial"); ?active=true means "not
// complete/cancelled".
func (s *Server) handleGetSAGRequests(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqs := s.rideMgr.GetRequests(id)

	if statusParam := r.URL.Query().Get("status"); statusParam != "" {
		want := make(map[string]bool)
		for _, st := range strings.Split(statusParam, ",") {
			st = strings.TrimSpace(st)
			if st != "" {
				want[st] = true
			}
		}
		filtered := make([]store.SAGRequest, 0, len(reqs))
		for _, req := range reqs {
			if want[req.Status] {
				filtered = append(filtered, req)
			}
		}
		reqs = filtered
	}

	if r.URL.Query().Get("active") == "true" {
		filtered := make([]store.SAGRequest, 0, len(reqs))
		for _, req := range reqs {
			if req.Status != ride.ReqComplete && req.Status != ride.ReqCancelled {
				filtered = append(filtered, req)
			}
		}
		reqs = filtered
	}

	writeJSON(w, http.StatusOK, reqs)
}

func (s *Server) handleGetSAGRequest(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	req, ok := s.rideMgr.GetRequest(id, reqID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "sag request not found"})
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleGetSAGVehicles(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.rideMgr.Vehicles(id))
}

// sagConfigResponse wraps ride.Config with the fixed vocabulary lists so the
// composer never hardcodes pickup/dropoff kinds.
type sagConfigResponse struct {
	Config       ride.Config `json:"config"`
	PickupKinds  []string    `json:"pickupKinds"`
	DropoffKinds []string    `json:"dropoffKinds"`
}

func (s *Server) handleGetSAGConfig(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	pickupKinds := make([]string, 0, len(ride.ValidPickupKinds))
	for k := range ride.ValidPickupKinds {
		pickupKinds = append(pickupKinds, k)
	}
	dropoffKinds := make([]string, 0, len(ride.ValidDropoffKinds))
	for k := range ride.ValidDropoffKinds {
		dropoffKinds = append(dropoffKinds, k)
	}
	writeJSON(w, http.StatusOK, sagConfigResponse{
		Config:       s.rideMgr.ConfigForNet(chi.URLParam(r, "id")),
		PickupKinds:  pickupKinds,
		DropoffKinds: dropoffKinds,
	})
}

// --- Write endpoints (operator+) ---

func (s *Server) handleCreateSAGRequest(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	var in ride.CreateRequestInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(in.RequestedBy) == "" {
		in.RequestedBy = s.sagUserName(r)
	}
	req, err := s.rideMgr.CreateRequest(id, in, s.sagUserName(r))
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

// updateSAGRequestBody is PUT /nets/{id}/sag/requests/{reqId}'s body:
// pickup/dropoff/reason/priority/notes/requestedBy, all optional. It
// decodes onto a copy of the request's current values (the handleUpdateMission
// idiom), so a field the client omits keeps its current value.
type updateSAGRequestBody struct {
	Pickup      store.SAGLocation `json:"pickup"`
	Dropoff     store.SAGLocation `json:"dropoff"`
	Reason      string            `json:"reason"`
	Priority    string            `json:"priority"`
	Notes       string            `json:"notes"`
	RequestedBy string            `json:"requestedBy"`
}

func (s *Server) handleUpdateSAGRequest(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	existing, ok := s.rideMgr.GetRequest(id, reqID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "sag request not found"})
		return
	}
	body := updateSAGRequestBody{
		Pickup: existing.Pickup, Dropoff: existing.Dropoff, Reason: existing.Reason,
		Priority: existing.Priority, Notes: existing.Notes, RequestedBy: existing.RequestedBy,
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.UpdateRequest(id, reqID, ride.UpdateRequestInput{
		Pickup: body.Pickup, Dropoff: body.Dropoff, Reason: body.Reason,
		Priority: body.Priority, Notes: body.Notes, RequestedBy: body.RequestedBy,
	})
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleCancelSAGRequest(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.CancelRequest(id, reqID, body.Reason, s.sagUserName(r))
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleAddSAGSlot(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	var in ride.SlotInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.AddSlot(id, reqID, in)
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) handleUpdateSAGSlot(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	slotID := chi.URLParam(r, "slotId")
	var in ride.SlotInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.UpdateSlot(id, reqID, slotID, in)
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleResolveSAGSlot(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	slotID := chi.URLParam(r, "slotId")
	var body struct {
		Disposition string `json:"disposition"`
		Note        string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.ResolveSlot(id, reqID, slotID, body.Disposition, body.Note, s.sagUserName(r))
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleDispatchSAGLeg(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	var in ride.DispatchInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.Dispatch(id, reqID, in, s.sagUserName(r))
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) handleAdvanceSAGLeg(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	legID := chi.URLParam(r, "legId")
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.AdvanceLeg(id, reqID, legID, body.Status, s.sagUserName(r))
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleLoadSAGSlots(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	legID := chi.URLParam(r, "legId")
	var body struct {
		SlotIDs []string `json:"slotIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.LoadSlots(id, reqID, legID, body.SlotIDs, s.sagUserName(r))
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleDeliverSAGSlots(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	legID := chi.URLParam(r, "legId")
	var body struct {
		SlotIDs     []string           `json:"slotIds"`
		Destination *store.SAGLocation `json:"destination,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.DeliverSlots(id, reqID, legID, body.SlotIDs, body.Destination, s.sagUserName(r))
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleReleaseSAGLeg(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqID := chi.URLParam(r, "reqId")
	legID := chi.URLParam(r, "legId")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req, err := s.rideMgr.ReleaseLeg(id, reqID, legID, body.Reason, s.sagUserName(r))
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handlePutSAGVehicle(w http.ResponseWriter, r *http.Request) {
	if s.rideMgr == nil {
		writeRideUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	ciID := chi.URLParam(r, "ciId")
	var body struct {
		Seats     int    `json:"seats"`
		RackSlots int    `json:"rackSlots"`
		Notes     string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	status, err := s.rideMgr.SetVehicle(id, ciID, body.Seats, body.RackSlots, body.Notes)
	if err != nil {
		writeSAGError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
