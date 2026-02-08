package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/server/ws"
)

func (s *Server) routes() {
	s.router.Route("/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/stations", s.handleGetStations)
		r.Get("/stations/{callsign}", s.handleGetStation)
		r.Get("/messages", s.handleGetMessages)
		r.Get("/messages/{callsign}", s.handleGetMessagesForCallsign)
		r.Post("/messages", s.handleSendMessage)
		r.Get("/transports", s.handleGetTransports)
		r.Post("/beacon", s.handleBeaconNow)

		// Objects and items
		r.Get("/objects", s.handleGetObjects)
		r.Post("/objects", s.handleCreateObject)
		r.Delete("/objects/{id}", s.handleDeleteObject)
		r.Post("/objects/{id}/kill", s.handleKillObject)
		r.Get("/items", s.handleGetItems)
		r.Post("/items", s.handleCreateItem)
		r.Delete("/items/{id}", s.handleDeleteItem)
		r.Post("/items/{id}/kill", s.handleKillItem)

		r.Get("/config", s.handleGetConfig)
	})

	// WebSocket endpoint
	s.router.Get("/ws", ws.HandleWS(s.hub, func(to, body string) error {
		if s.msgEngine == nil {
			return nil
		}
		_, err := s.msgEngine.Send(to, body)
		return err
	}))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetStations(w http.ResponseWriter, r *http.Request) {
	// Support bounds query: ?bounds=south,west,north,east
	boundsStr := r.URL.Query().Get("bounds")
	if boundsStr != "" {
		parts := strings.Split(boundsStr, ",")
		if len(parts) == 4 {
			south, _ := strconv.ParseFloat(parts[0], 64)
			west, _ := strconv.ParseFloat(parts[1], 64)
			north, _ := strconv.ParseFloat(parts[2], 64)
			east, _ := strconv.ParseFloat(parts[3], 64)
			stations := s.tracker.InArea(south, west, north, east)
			writeJSON(w, http.StatusOK, stations)
			return
		}
	}

	// Support search: ?q=prefix
	query := r.URL.Query().Get("q")
	if query != "" {
		stations := s.tracker.Search(query)
		writeJSON(w, http.StatusOK, stations)
		return
	}

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
	if s.msgEngine == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	convos := s.msgEngine.Conversations()
	writeJSON(w, http.StatusOK, convos)
}

func (s *Server) handleGetMessagesForCallsign(w http.ResponseWriter, r *http.Request) {
	callsign := chi.URLParam(r, "callsign")
	if s.msgEngine == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	msgs := s.msgEngine.Messages(callsign)
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if s.msgEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "message engine not available"})
		return
	}

	var req struct {
		To   string `json:"to"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.To == "" || req.Body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to and body are required"})
		return
	}

	msg, err := s.msgEngine.Send(req.To, req.Body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, msg)
}

func (s *Server) handleGetTransports(w http.ResponseWriter, _ *http.Request) {
	statuses := s.transports.Statuses()
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleBeaconNow(w http.ResponseWriter, _ *http.Request) {
	if s.beaconMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "beaconing not configured"})
		return
	}
	if err := s.beaconMgr.BeaconNow(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "beacon sent"})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	// Return a safe subset of config info
	writeJSON(w, http.StatusOK, map[string]any{
		"transports": len(s.transports.Statuses()),
		"wsClients":  s.hub.ClientCount(),
	})
}

// --- Object/Item handlers ---

func (s *Server) handleGetObjects(w http.ResponseWriter, _ *http.Request) {
	if s.objManager == nil {
		writeJSON(w, http.StatusOK, map[string]any{"own": []any{}, "received": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"own":      s.objManager.OwnObjects(),
		"received": s.objManager.ReceivedObjects(),
	})
}

func (s *Server) handleCreateObject(w http.ResponseWriter, r *http.Request) {
	if s.objManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object manager not available"})
		return
	}

	var req struct {
		Name       string  `json:"name"`
		Lat        float64 `json:"lat"`
		Lon        float64 `json:"lon"`
		SymbolTable string `json:"symbolTable"`
		SymbolCode  string `json:"symbolCode"`
		Comment    string  `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	symTable := byte('/')
	symCode := byte('-')
	if len(req.SymbolTable) > 0 {
		symTable = req.SymbolTable[0]
	}
	if len(req.SymbolCode) > 0 {
		symCode = req.SymbolCode[0]
	}

	obj, err := s.objManager.CreateObject(object.Object{
		Name:    req.Name,
		Lat:     req.Lat,
		Lon:     req.Lon,
		Symbol:  aprs.Symbol{Table: symTable, Code: symCode},
		Comment: req.Comment,
		Live:    true,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, obj)
}

func (s *Server) handleDeleteObject(w http.ResponseWriter, r *http.Request) {
	if s.objManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object manager not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.objManager.DeleteObject(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleKillObject(w http.ResponseWriter, r *http.Request) {
	if s.objManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object manager not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.objManager.KillObject(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}

func (s *Server) handleGetItems(w http.ResponseWriter, _ *http.Request) {
	if s.objManager == nil {
		writeJSON(w, http.StatusOK, map[string]any{"own": []any{}, "received": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"own":      s.objManager.OwnItems(),
		"received": s.objManager.ReceivedItems(),
	})
}

func (s *Server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	if s.objManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object manager not available"})
		return
	}

	var req struct {
		Name       string  `json:"name"`
		Lat        float64 `json:"lat"`
		Lon        float64 `json:"lon"`
		SymbolTable string `json:"symbolTable"`
		SymbolCode  string `json:"symbolCode"`
		Comment    string  `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	symTable := byte('/')
	symCode := byte('-')
	if len(req.SymbolTable) > 0 {
		symTable = req.SymbolTable[0]
	}
	if len(req.SymbolCode) > 0 {
		symCode = req.SymbolCode[0]
	}

	item, err := s.objManager.CreateItem(object.Item{
		Name:    req.Name,
		Lat:     req.Lat,
		Lon:     req.Lon,
		Symbol:  aprs.Symbol{Table: symTable, Code: symCode},
		Comment: req.Comment,
		Live:    true,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	if s.objManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object manager not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.objManager.DeleteItem(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleKillItem(w http.ResponseWriter, r *http.Request) {
	if s.objManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object manager not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.objManager.KillItem(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
