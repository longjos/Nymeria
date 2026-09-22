package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/store"
)

// writeRideTrafficUnavailable answers a ride-traffic route the same way
// every other feature-gated route in this package does when the manager
// was never wired up (checkpoint/wx-alert/SAG 503 pattern).
func writeRideTrafficUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ride traffic manager not available"})
}

// writeRideTrafficError maps a ride package error to the right HTTP status:
// ride.ErrNotFound -> 404, ride.ErrIllegalTransition -> 409, everything
// else (a manager validation error) -> 400.
func writeRideTrafficError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ride.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ride.ErrIllegalTransition):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

// rideTrafficActor builds a ride.Actor from the authenticated session user
// (if any) plus the request body's optional "callsign" field.
func (s *Server) rideTrafficActor(r *http.Request, callsign string) ride.Actor {
	a := ride.Actor{Callsign: callsign}
	if user, ok := UserFromContext(r.Context()); ok {
		a.UserID = user.ID
		a.UserName = user.Name
	}
	return a
}

func decodeRideTrafficBody(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return errBadRequestBody
	}
	return nil
}

var errBadRequestBody = errors.New("invalid request body")

// --- Supply: read endpoints (observer+) ---

func (s *Server) handleRideListSupply(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	reqs := s.rideTraffic.GetSupplyRequests(id)
	if r.URL.Query().Get("status") == "open" {
		filtered := make([]store.SupplyRequest, 0, len(reqs))
		for _, req := range reqs {
			if req.Status != store.SupplyDelivered && req.Status != store.SupplyCancelled && req.Status != store.SupplyMerged {
				filtered = append(filtered, req)
			}
		}
		reqs = filtered
	}
	writeJSON(w, http.StatusOK, reqs)
}

func (s *Server) handleRideGetSupply(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	req, ok := s.rideTraffic.GetSupplyRequest(id, sid)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "supply request not found"})
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleRideSupplyCatalog(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, s.rideTraffic.SupplyCatalog(id))
}

// --- Medical: read endpoints (observer+) ---

func (s *Server) handleRideListMedical(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	list := s.rideTraffic.GetMedical(id, true) // ALWAYS Redacted() on the list route
	if r.URL.Query().Get("status") == "open" {
		filtered := make([]store.MedicalNotification, 0, len(list))
		for _, n := range list {
			if n.Status != store.MedDeparted && n.Status != store.MedReleased && n.Status != store.MedCancelled {
				filtered = append(filtered, n)
			}
		}
		list = filtered
	}
	writeJSON(w, http.StatusOK, list)
}

// --- Medical: operator-only read (full, unredacted) ---

func (s *Server) handleRideGetMedical(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "mid")
	n, ok := s.rideTraffic.GetMedicalOne(id, mid, false)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "medical notification not found"})
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// --- Supply: write endpoints (operator+) ---

type createSupplyBody struct {
	ride.CreateSupplyInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideCreateSupply(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	var body createSupplyBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req, err := s.rideTraffic.CreateSupplyRequest(id, body.CreateSupplyInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

type addSupplyItemsBody struct {
	ride.AddItemsInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideSupplyItems(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	var body addSupplyItemsBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req, err := s.rideTraffic.AddSupplyItems(id, sid, body.AddItemsInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

type readbackBody struct {
	ride.ReadbackInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideSupplyReadback(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	var body readbackBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req, err := s.rideTraffic.ReadbackSupply(id, sid, body.ReadbackInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

type relayBody struct {
	ride.RelayInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideSupplyRelay(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	var body relayBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req, err := s.rideTraffic.RelaySupply(id, sid, body.RelayInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

type etaBody struct {
	ride.ETAInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideSupplyETA(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	var body etaBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req, err := s.rideTraffic.RecordSupplyETA(id, sid, body.ETAInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleRideSupplyDeliver(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	var body struct {
		Callsign string `json:"callsign"`
	}
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req, err := s.rideTraffic.DeliverSupply(id, sid, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

type cancelBody struct {
	ride.CancelInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideSupplyCancel(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	var body cancelBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req, err := s.rideTraffic.CancelSupply(id, sid, body.CancelInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleRideSupplyMerge(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	sid := chi.URLParam(r, "sid")
	otherSid := chi.URLParam(r, "otherSid")
	var body struct {
		Callsign string `json:"callsign"`
	}
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req, err := s.rideTraffic.MergeSupply(id, sid, otherSid, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// --- Medical: write endpoints (operator+) ---

type createMedicalBody struct {
	ride.CreateMedicalInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideCreateMedical(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	var body createMedicalBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n, err := s.rideTraffic.CreateMedical(id, body.CreateMedicalInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleRideMedicalReadback(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "mid")
	var body readbackBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n, err := s.rideTraffic.ReadbackMedical(id, mid, body.ReadbackInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

type medicalETABody struct {
	ride.MedicalETAInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideMedicalETA(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "mid")
	var body medicalETABody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n, err := s.rideTraffic.RecordMedicalETA(id, mid, body.MedicalETAInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

type onSceneBody struct {
	ride.OnSceneInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideMedicalOnScene(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "mid")
	var body onSceneBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n, err := s.rideTraffic.MedicalOnScene(id, mid, body.OnSceneInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

type departBody struct {
	ride.DepartInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideMedicalDepart(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "mid")
	var body departBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n, err := s.rideTraffic.MedicalDepart(id, mid, body.DepartInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

type releaseBody struct {
	ride.ReleaseInput
	Callsign string `json:"callsign"`
}

func (s *Server) handleRideMedicalRelease(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "mid")
	var body releaseBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n, err := s.rideTraffic.MedicalRelease(id, mid, body.ReleaseInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleRideMedicalCancel(w http.ResponseWriter, r *http.Request) {
	if s.rideTraffic == nil {
		writeRideTrafficUnavailable(w)
		return
	}
	id := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "mid")
	var body cancelBody
	if err := decodeRideTrafficBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n, err := s.rideTraffic.CancelMedical(id, mid, body.CancelInput, s.rideTrafficActor(r, body.Callsign))
	if err != nil {
		writeRideTrafficError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}
