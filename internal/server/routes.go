package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) routes() {
	s.router.Route("/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/stations", s.handleGetStations)
		r.Get("/stations/{callsign}", s.handleGetStation)
		r.Get("/messages", s.handleGetMessages)
		r.Post("/messages", s.handleSendMessage)
		r.Get("/transports", s.handleGetTransports)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetStations(w http.ResponseWriter, _ *http.Request) {
	stations := s.tracker.All()
	writeJSON(w, http.StatusOK, stations)
}

func (s *Server) handleGetStation(w http.ResponseWriter, r *http.Request) {
	callsign := chi.URLParam(r, "callsign")
	st, ok := s.tracker.Get(callsign)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "station not found"})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleGetMessages(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

func (s *Server) handleGetTransports(w http.ResponseWriter, _ *http.Request) {
	statuses := s.transports.Statuses()
	writeJSON(w, http.StatusOK, statuses)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
