package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/store"
)

func (s *Server) routes() {
	s.router.Route("/api", func(r chi.Router) {
		// Public endpoints — no auth required
		r.Get("/health", s.handleHealth)
		r.Get("/config", s.handleGetConfig)
		r.Post("/session", s.handleLogin)

		// Read-only endpoints — observers and above
		r.Get("/stations", s.handleGetStations)
		r.Get("/stations/{callsign}", s.handleGetStation)
		r.Get("/messages", s.handleGetMessages)
		r.Get("/messages/{callsign}", s.handleGetMessagesForCallsign)
		r.Get("/transports", s.handleGetTransports)
		r.Get("/objects", s.handleGetObjects)
		r.Get("/items", s.handleGetItems)
		r.Get("/annotations", s.handleGetAnnotations)
		r.Get("/activity", s.handleGetActivity)
		r.Get("/activity/export", s.handleExportActivityCSV)

		// Session management — requires a valid session
		r.Get("/session", s.handleGetSession)
		r.Delete("/session", s.handleLogout)
		r.Get("/users", s.handleGetUsers)

		// Plotter endpoints — annotations write access
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RolePlotter))
			r.Post("/annotations", s.handleCreateAnnotation)
			r.Put("/annotations/{id}", s.handleUpdateAnnotation)
			r.Delete("/annotations/{id}", s.handleDeleteAnnotation)
		})

		// Operator endpoints — messages, objects, beacon
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleOperator))
			r.Post("/messages", s.handleSendMessage)
			r.Post("/messages/{callsign}/claim", s.handleClaimConversation)
			r.Delete("/messages/{callsign}/claim", s.handleUnclaimConversation)
			r.Post("/beacon", s.handleBeaconNow)
			r.Post("/objects", s.handleCreateObject)
			r.Delete("/objects/{id}", s.handleDeleteObject)
			r.Post("/objects/{id}/kill", s.handleKillObject)
			r.Post("/items", s.handleCreateItem)
			r.Delete("/items/{id}", s.handleDeleteItem)
			r.Post("/items/{id}/kill", s.handleKillItem)
		})

		// Admin endpoints — user management
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleAdmin))
			r.Put("/users/{id}/role", s.handleUpdateUserRole)
			r.Delete("/users/{id}", s.handleRemoveUser)
		})
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

func (s *Server) handleClaimConversation(w http.ResponseWriter, r *http.Request) {
	if s.msgEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "message engine not available"})
		return
	}

	callsign := chi.URLParam(r, "callsign")

	var req struct {
		UserID   string `json:"userId"`
		UserName string `json:"userName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.UserID == "" || req.UserName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userId and userName are required"})
		return
	}

	if err := s.msgEngine.ClaimConversation(callsign, req.UserID, req.UserName); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"callsign": callsign,
		"claimedBy": req.UserID,
		"claimedName": req.UserName,
	})
}

func (s *Server) handleUnclaimConversation(w http.ResponseWriter, r *http.Request) {
	if s.msgEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "message engine not available"})
		return
	}

	callsign := chi.URLParam(r, "callsign")
	if err := s.msgEngine.UnclaimConversation(callsign); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"callsign": callsign, "status": "unclaimed"})
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
	pinRequired := s.sessions != nil
	writeJSON(w, http.StatusOK, map[string]any{
		"transports":  len(s.transports.Statuses()),
		"wsClients":   s.hub.ClientCount(),
		"pinRequired": pinRequired,
	})
}

// --- Session handlers ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions not configured"})
		return
	}

	var req struct {
		Name string `json:"name"`
		PIN  string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	user, err := s.sessions.Create(strings.TrimSpace(req.Name), req.PIN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if s.actLogger != nil {
		s.actLogger.Log(activity.Entry{
			Timestamp: user.ConnectedAt,
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionSessionStarted,
		})
	}

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	if s.sessions != nil {
		s.sessions.Remove(user.ID)
	}

	if s.actLogger != nil {
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionSessionEnded,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	users := s.sessions.All()
	// Strip tokens — only show public info
	type publicUser struct {
		ID          string       `json:"id"`
		Name        string       `json:"name"`
		Role        session.Role `json:"role"`
		Callsign    string       `json:"callsign,omitempty"`
		ConnectedAt time.Time    `json:"connectedAt"`
	}
	result := make([]publicUser, len(users))
	for i, u := range users {
		result[i] = publicUser{
			ID:          u.ID,
			Name:        u.Name,
			Role:        u.Role,
			Callsign:    u.Callsign,
			ConnectedAt: u.ConnectedAt,
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions not configured"})
		return
	}

	id := chi.URLParam(r, "id")

	var req struct {
		Role session.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if session.RoleLevel(req.Role) < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
		return
	}

	if err := s.sessions.UpdateRole(id, req.Role); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": id, "role": string(req.Role)})
}

func (s *Server) handleRemoveUser(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions not configured"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.sessions.Remove(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "removed"})
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

// --- Annotation handlers ---

func (s *Server) handleGetAnnotations(w http.ResponseWriter, _ *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, s.annMgr.All())
}

func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	var req annotation.Annotation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	ann, err := s.annMgr.Create(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, ann)
}

func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")

	var req annotation.Annotation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.ID = id

	ann, err := s.annMgr.Update(req)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ann)
}

func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.annMgr.Delete(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Activity log handlers ---

func (s *Server) handleGetActivity(w http.ResponseWriter, r *http.Request) {
	if s.actLogger == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "total": 0})
		return
	}

	filter := store.ActivityFilter{}
	if v := r.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = &t
		}
	}
	if v := r.URL.Query().Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Until = &t
		}
	}
	if v := r.URL.Query().Get("userId"); v != "" {
		filter.UserID = v
	}
	if v := r.URL.Query().Get("action"); v != "" {
		filter.Action = v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}

	// Convert store filter to activity filter.
	af := activity.Filter{
		Since:  filter.Since,
		Until:  filter.Until,
		UserID: filter.UserID,
		Action: filter.Action,
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}

	entries, total, err := s.actLogger.Query(af)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": total})
}

func (s *Server) handleExportActivityCSV(w http.ResponseWriter, r *http.Request) {
	if s.actLogger == nil {
		http.Error(w, "activity logging not available", http.StatusServiceUnavailable)
		return
	}

	// Query all entries (or filtered).
	af := activity.Filter{}
	if v := r.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			af.Since = &t
		}
	}
	if v := r.URL.Query().Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			af.Until = &t
		}
	}

	entries, _, err := s.actLogger.Query(af)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=activity.csv")
	activity.ExportCSV(w, entries)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
