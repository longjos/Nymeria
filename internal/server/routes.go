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
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
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
		r.Get("/bulletins", s.handleGetBulletins)
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

		// Tactical aliases — read access for observers
		r.Get("/tactical", s.handleGetTacticalAliases)

		// Plotter endpoints — annotations + tactical aliases write access
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RolePlotter))
			r.Post("/annotations", s.handleCreateAnnotation)
			r.Put("/annotations/{id}", s.handleUpdateAnnotation)
			r.Delete("/annotations/{id}", s.handleDeleteAnnotation)
			r.Post("/annotations/{id}/status", s.handleChangeAnnotationStatus)
			r.Put("/tactical/{callsign}", s.handleSetTacticalAlias)
			r.Delete("/tactical/{callsign}", s.handleDeleteTacticalAlias)
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

		// Net Control — read endpoints (observer+)
		r.Get("/nets", s.handleGetNets)
		r.Get("/nets/search", s.handleSearchOperators)
		r.Get("/nets/{id}", s.handleGetNet)
		r.Get("/nets/{id}/events", s.handleGetNetEvents)
		r.Get("/nets/{id}/notes", s.handleGetNetNotes)
		r.Get("/nets/{id}/roster/export", s.handleExportRosterCSV)

		// Net Control — write endpoints (operator+)
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleOperator))
			r.Post("/nets", s.handleCreateNet)
			r.Post("/nets/{id}/open", s.handleOpenNet)
			r.Post("/nets/{id}/close", s.handleCloseNet)
			r.Post("/nets/{id}/transfer", s.handleTransferNCS)
			r.Post("/nets/{id}/checkin", s.handleCheckIn)
			r.Put("/nets/{id}/checkin/{ciId}", s.handleUpdateCheckIn)
			r.Post("/nets/{id}/checkout/{ciId}", s.handleCheckOut)
			r.Post("/nets/{id}/missions", s.handleCreateMission)
			r.Put("/nets/{id}/missions/{mId}", s.handleUpdateMission)
			r.Post("/nets/{id}/notes", s.handleAddNetNote)
			r.Post("/nets/{id}/rollcall", s.handleInitiateRollCall)
			r.Post("/nets/{id}/rollcall/{ciId}", s.handleRecordRollCallResponse)
			r.Post("/nets/{id}/checkin/{ciId}/assign", s.handleAssignMission)
			r.Delete("/nets/{id}/checkin/{ciId}/assign", s.handleUnassignMission)
			r.Post("/nets/{id}/checkin/{ciId}/devices", s.handleAddTrackedStation)
			r.Delete("/nets/{id}/checkin/{ciId}/devices/{callsign}", s.handleRemoveTrackedStation)
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

func (s *Server) handleGetBulletins(w http.ResponseWriter, _ *http.Request) {
	if s.msgEngine == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, s.msgEngine.Bulletins())
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

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionMessageSent,
			Target:    req.To,
		})
	}
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
		"callsign":    callsign,
		"claimedBy":   req.UserID,
		"claimedName": req.UserName,
	})

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionMessageClaimed,
			Target:    callsign,
		})
	}
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

func (s *Server) handleBeaconNow(w http.ResponseWriter, r *http.Request) {
	if s.beaconMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "beaconing not configured"})
		return
	}
	if err := s.beaconMgr.BeaconNow(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "beacon sent"})

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionBeaconSent,
			Details:   "manual",
		})
	}
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

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionObjectCreated,
			Target:    obj.Name,
		})
	}
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

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionObjectKilled,
			Target:    id,
		})
	}
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

func (s *Server) handleGetAnnotations(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	q := r.URL.Query()
	filter := store.AnnotationFilter{
		Category:       q.Get("category"),
		Status:         q.Get("status"),
		Priority:       q.Get("priority"),
		OperationID:    q.Get("operationId"),
		IncludeExpired: q.Get("includeExpired") == "true",
	}

	// Use filtered query if any filter param is set.
	if filter.Category != "" || filter.Status != "" || filter.Priority != "" || filter.OperationID != "" || filter.IncludeExpired {
		writeJSON(w, http.StatusOK, s.annMgr.AllFiltered(filter))
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

	// Set creator attribution from session
	if user, ok := UserFromContext(r.Context()); ok {
		req.CreatedBy = user.ID
		req.CreatedByName = user.Name
	}

	ann, err := s.annMgr.Create(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, ann)

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionAnnotationCreated,
			Target:    ann.Label,
		})
	}
}

func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")

	// Fetch the existing annotation so omitted fields are preserved (partial update).
	existing, found := s.annMgr.Get(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "annotation not found"})
		return
	}

	updated := *existing
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	updated.ID = id // ensure ID cannot be changed

	ann, err := s.annMgr.Update(updated)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionAnnotationDeleted,
			Target:    id,
		})
	}
}

func (s *Server) handleChangeAnnotationStatus(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Status) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status is required"})
		return
	}

	ann, err := s.annMgr.ChangeStatus(id, req.Status)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ann)

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionAnnotationStatusChanged,
			Target:    ann.Label,
			Details:   req.Status,
		})
	}
}

// --- Tactical alias handlers ---

func (s *Server) handleGetTacticalAliases(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	aliases, err := s.store.LoadTacticalAliases()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if aliases == nil {
		aliases = []store.TacticalAlias{}
	}
	writeJSON(w, http.StatusOK, aliases)
}

func (s *Server) handleSetTacticalAlias(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	callsign := chi.URLParam(r, "callsign")

	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Alias) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "alias is required"})
		return
	}

	alias := store.TacticalAlias{
		Callsign:   strings.ToUpper(callsign),
		Alias:      strings.TrimSpace(req.Alias),
		AssignedBy: "ui",
		UpdatedAt:  time.Now().UTC(),
	}

	if err := s.store.SaveTacticalAlias(alias); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, alias)

	// Broadcast via WebSocket.
	s.broadcastTactical("tactical_set", alias)

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    "tactical_set",
			Target:    alias.Callsign,
			Details:   alias.Alias,
		})
	}
}

func (s *Server) handleDeleteTacticalAlias(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	callsign := strings.ToUpper(chi.URLParam(r, "callsign"))

	if err := s.store.DeleteTacticalAlias(callsign); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	// Broadcast via WebSocket.
	s.broadcastTactical("tactical_removed", map[string]string{"callsign": callsign})

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    "tactical_removed",
			Target:    callsign,
		})
	}
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

// --- Net Control handlers ---

func (s *Server) handleGetNets(w http.ResponseWriter, _ *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, s.netMgr.GetNets())
}

func (s *Server) handleGetNet(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	n, ok := s.netMgr.GetNet(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "net not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"net":      n,
		"checkIns": s.netMgr.GetCheckIns(id),
		"missions": s.netMgr.GetMissions(id),
	})
}

func (s *Server) handleCreateNet(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	var req store.Net
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Set NCS from session user if not provided.
	if user, ok := UserFromContext(r.Context()); ok && req.NCSUserID == "" {
		req.NCSUserID = user.ID
		if req.NCSCallsign == "" {
			req.NCSCallsign = user.Callsign
		}
	}

	n, err := s.netMgr.CreateNet(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleOpenNet(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.netMgr.OpenNet(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	n, _ := s.netMgr.GetNet(id)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleCloseNet(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	n, summary, err := s.netMgr.CloseNet(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"net": n, "summary": summary})
}

func (s *Server) handleTransferNCS(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Callsign string `json:"callsign"`
		UserID   string `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := s.netMgr.TransferNCS(id, req.Callsign, req.UserID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	n, _ := s.netMgr.GetNet(id)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleCheckIn(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Callsign string `json:"callsign"`
		Traffic  string `json:"traffic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	ci, err := s.netMgr.CheckIn(id, req.Callsign, req.Traffic)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, ci)
}

func (s *Server) handleUpdateCheckIn(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ciId := chi.URLParam(r, "ciId")

	// Get the existing check-in.
	existing := findCheckIn(s.netMgr.GetCheckIns(id), ciId)
	if existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "check-in not found"})
		return
	}

	updated := *existing
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	updated.ID = ciId
	updated.NetID = id

	ci, err := s.netMgr.UpdateCheckIn(updated)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ci)
}

func (s *Server) handleCheckOut(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ciId := chi.URLParam(r, "ciId")

	if err := s.netMgr.CheckOut(id, ciId); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "checked out"})
}

func (s *Server) handleCreateMission(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	var req store.NetMission
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.NetID = id

	m, err := s.netMgr.CreateMission(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleUpdateMission(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	mId := chi.URLParam(r, "mId")

	// Get the existing mission.
	existing := findMission(s.netMgr.GetMissions(id), mId)
	if existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "mission not found"})
		return
	}

	updated := *existing
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	updated.ID = mId
	updated.NetID = id

	m, err := s.netMgr.UpdateMission(updated)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleAddNetNote(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	var req store.NetNote
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.NetID = id

	if user, ok := UserFromContext(r.Context()); ok {
		req.AuthorID = user.ID
		req.AuthorName = user.Name
	}

	note, err := s.netMgr.AddNote(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, note)
}

func (s *Server) handleGetNetEvents(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	id := chi.URLParam(r, "id")
	events, err := s.netMgr.GetEvents(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleGetNetNotes(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	id := chi.URLParam(r, "id")
	notes, err := s.netMgr.GetNotes(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, notes)
}

func (s *Server) handleInitiateRollCall(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.netMgr.InitiateRollCall(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "roll call initiated"})
}

func (s *Server) handleRecordRollCallResponse(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ciId := chi.URLParam(r, "ciId")

	if err := s.netMgr.RecordRollCallResponse(id, ciId); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "response recorded"})
}

func (s *Server) handleSearchOperators(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	q := r.URL.Query().Get("q")
	results := s.netMgr.SearchOperators(q)
	if results == nil {
		results = []station.Station{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleAssignMission(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ciId := chi.URLParam(r, "ciId")

	var req struct {
		MissionID string `json:"missionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	ci, err := s.netMgr.AssignMission(id, ciId, req.MissionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ci)
}

func (s *Server) handleUnassignMission(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ciId := chi.URLParam(r, "ciId")

	ci, err := s.netMgr.UnassignMission(id, ciId)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ci)
}

func (s *Server) handleAddTrackedStation(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ciId := chi.URLParam(r, "ciId")

	var req struct {
		Callsign string `json:"callsign"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	ci, err := s.netMgr.AddTrackedStation(id, ciId, req.Callsign)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ci)
}

func (s *Server) handleRemoveTrackedStation(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ciId := chi.URLParam(r, "ciId")
	callsign := chi.URLParam(r, "callsign")

	ci, err := s.netMgr.RemoveTrackedStation(id, ciId, callsign)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ci)
}

func (s *Server) handleExportRosterCSV(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		http.Error(w, "net control not available", http.StatusServiceUnavailable)
		return
	}

	id := chi.URLParam(r, "id")
	checkIns := s.netMgr.GetCheckIns(id)

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=roster.csv")
	netcontrol.ExportRosterCSV(w, checkIns)
}

func findCheckIn(cis []store.NetCheckIn, id string) *store.NetCheckIn {
	for _, ci := range cis {
		if ci.ID == id {
			return &ci
		}
	}
	return nil
}

func findMission(missions []store.NetMission, id string) *store.NetMission {
	for _, m := range missions {
		if m.ID == id {
			return &m
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
