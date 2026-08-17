package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/ics309"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/tilecache"
)

func (s *Server) routes() {
	s.router.Route("/api", func(r chi.Router) {
		// Public endpoints — no auth required
		r.Get("/health", s.handleHealth)
		r.Get("/config", s.handleGetConfig)
		r.Post("/setup", s.handleSetup)
		r.Post("/session", s.handleLogin)

		// Read-only endpoints — observers and above
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleObserver))
			r.Get("/stations", s.handleGetStations)
			r.Get("/stations/{callsign}", s.handleGetStation)
			r.Get("/bulletins", s.handleGetBulletins)
			r.Get("/messages", s.handleGetMessages)
			r.Get("/messages/{callsign}", s.handleGetMessagesForCallsign)
			r.Post("/messages/{callsign}/read", s.handleMarkConversationRead)
			r.Get("/transports", s.handleGetTransports)
			r.Get("/objects", s.handleGetObjects)
			r.Get("/items", s.handleGetItems)
			r.Get("/annotations", s.handleGetAnnotations)
			r.Get("/annotation-templates", s.handleGetAnnotationTemplates)
			r.Get("/operations", s.handleGetOperations)
			r.Get("/operations/{id}", s.handleGetOperation)
			r.Get("/activity", s.handleGetActivity)
			r.Get("/activity/export", s.handleExportActivityCSV)

			// Weather
			r.Get("/weather/stations", s.handleGetWeatherStations)
			r.Get("/weather/config", s.handleGetWeatherConfig)
			r.Get("/weather/{callsign}", s.handleGetWeatherReadings)

			// Telemetry
			r.Get("/telemetry/stations", s.handleGetTelemetryStations)
			r.Get("/telemetry/{callsign}", s.handleGetTelemetryReadings)

			// ICS-309 Communications Log
			r.Get("/ics309", s.handleGetICS309)
			r.Get("/ics309/export", s.handleExportICS309CSV)

			// Tile cache — status (observer+)
			r.Get("/tiles/cache", s.handleTileCacheStatus)

			// Session management — requires a valid session
			r.Get("/session", s.handleGetSession)
			r.Delete("/session", s.handleLogout)
			r.Get("/users", s.handleGetUsers)

			// Tactical aliases — read access for observers
			r.Get("/tactical", s.handleGetTacticalAliases)
		})

		// Plotter endpoints — annotations + tactical aliases write access
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RolePlotter))
			r.Post("/annotations", s.handleCreateAnnotation)
			r.Put("/annotations/{id}", s.handleUpdateAnnotation)
			r.Delete("/annotations/{id}", s.handleDeleteAnnotation)
			r.Post("/annotations/import", s.handleImportAnnotations)
			r.Post("/annotations/{id}/status", s.handleChangeAnnotationStatus)
			r.Post("/annotations/{id}/promote", s.handlePromoteAnnotationToMission)
			r.Post("/annotations/{id}/link", s.handleLinkAnnotation)
			r.Delete("/annotations/{id}/link", s.handleUnlinkAnnotation)
			r.Post("/operations", s.handleCreateOperation)
			r.Post("/operations/{id}/archive", s.handleArchiveOperation)
			r.Put("/tactical/{callsign}", s.handleSetTacticalAlias)
			r.Delete("/tactical/{callsign}", s.handleDeleteTacticalAlias)
		})

		// Operator endpoints — messages, objects, beacon, annotation transmit
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleOperator))
			r.Post("/annotations/{id}/transmit", s.handleTransmitAnnotation)
			r.Delete("/annotations/{id}/transmit", s.handleStopTransmitAnnotation)
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
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleObserver))
			r.Get("/nets", s.handleGetNets)
			r.Get("/nets/search", s.handleSearchOperators)
			r.Get("/nets/{id}", s.handleGetNet)
			r.Get("/nets/{id}/events", s.handleGetNetEvents)
			r.Get("/nets/{id}/notes", s.handleGetNetNotes)
			r.Get("/nets/{id}/annotations", s.handleGetNetAnnotations)
			r.Get("/nets/{id}/roster/export", s.handleExportRosterCSV)

			// Checkpoint progress — read endpoints (observer+)
			r.Get("/nets/{id}/checkpoints", s.handleGetCheckpoints)
			r.Get("/nets/{id}/progress", s.handleGetProgress)
		})

		// Net Control — write endpoints (operator+)
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleOperator))
			r.Post("/nets", s.handleCreateNet)
			r.Post("/nets/{id}/open", s.handleOpenNet)
			r.Post("/nets/{id}/close", s.handleCloseNet)
			r.Post("/nets/{id}/transfer", s.handleTransferNCS)
			r.Post("/nets/{id}/opsview", s.handleSetOpsView)
			r.Post("/nets/{id}/checkin", s.handleCheckIn)
			r.Put("/nets/{id}/checkin/{ciId}", s.handleUpdateCheckIn)
			r.Post("/nets/{id}/checkout/{ciId}", s.handleCheckOut)
			r.Post("/nets/{id}/missions", s.handleCreateMission)
			r.Put("/nets/{id}/missions/{mId}", s.handleUpdateMission)
			r.Post("/nets/{id}/notes", s.handleAddNetNote)
			r.Patch("/nets/{id}/notes/{noteId}/pin", s.handleToggleNotePin)
			r.Post("/nets/{id}/rollcall", s.handleInitiateRollCall)
			r.Post("/nets/{id}/rollcall/{ciId}", s.handleRecordRollCallResponse)
			r.Post("/nets/{id}/checkin/{ciId}/assign", s.handleAssignMission)
			r.Delete("/nets/{id}/checkin/{ciId}/assign", s.handleUnassignMission)
			r.Post("/nets/{id}/checkin/{ciId}/devices", s.handleAddTrackedStation)
			r.Delete("/nets/{id}/checkin/{ciId}/devices/{callsign}", s.handleRemoveTrackedStation)
			r.Put("/nets/{id}/checkpoints/{cpId}/meta", s.handleUpdateCheckpointMeta)
			r.Post("/nets/{id}/checkpoints/{cpId}/passages", s.handleLogPassage)
			r.Post("/nets/{id}/annotations/import", s.handleImportNetAnnotations)
			r.Post("/nets/{id}/annotations/copy/{sourceNetId}", s.handleCopyNetAnnotations)
			r.Post("/nets/{id}/pin/{callsign}", s.handlePinStation)
			r.Delete("/nets/{id}/pin/{callsign}", s.handleUnpinStation)
			r.Put("/nets/{id}/pins", s.handleReorderPins)
		})

		// Tile cache — preload/estimate (operator+)
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleOperator))
			r.Post("/tiles/cache", s.handleTilePreload)
			r.Post("/tiles/estimate", s.handleTileEstimate)
		})

		// Admin endpoints — user management
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleAdmin))
			r.Put("/users/{id}/role", s.handleUpdateUserRole)
			r.Delete("/users/{id}", s.handleRemoveUser)
			r.Post("/session/approve", s.handleApproveUser)
			r.Post("/session/deny", s.handleDenyUser)
			r.Get("/session/pending", s.handleGetPending)
		})

		// Settings endpoints — admin only
		r.Group(func(r chi.Router) {
			r.Use(RequireRole(session.RoleAdmin))
			r.Get("/settings", s.handleGetSettings)
			r.Get("/serial-ports", s.handleListSerialPorts)
			r.Get("/kiss-tncs", s.handleListKissTNCs)
			r.Put("/settings/station", s.handleUpdateStation)
			r.Put("/settings/server", s.handleUpdateServer)
			r.Put("/settings/transports", s.handleUpdateTransports)
			r.Put("/settings/beacon", s.handleUpdateBeacon)
			r.Put("/settings/session", s.handleUpdateSession)
			r.Put("/settings/logging", s.handleUpdateLogging)
			r.Put("/settings/weather", s.handleUpdateWeather)
			r.Put("/settings/tilecache", s.handleUpdateTileCache)
		})
	})

	// Tile proxy endpoint — no auth, serves images directly
	if s.tileCache != nil {
		s.router.Get("/tiles/{z}/{x}/{y}", s.handleTileProxy)
	}

	// WebSocket endpoint
	var sessionLookup ws.SessionLookup
	if s.sessions != nil {
		sessionLookup = func(token string) (string, bool) {
			user, ok := s.sessions.Get(token)
			if !ok {
				return "", false
			}
			s.sessions.Touch(token)
			return user.ID, true
		}
	}
	s.router.Get("/ws", ws.HandleWS(s.hub, func(to, body string) error {
		if s.msgEngine == nil {
			return nil
		}
		_, err := s.msgEngine.Send(to, body)
		return err
	}, sessionLookup))
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
		To   string  `json:"to"`
		Body string  `json:"body"`
		Path *string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.To == "" || req.Body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to and body are required"})
		return
	}

	var (
		msg *message.Message
		err error
	)
	if req.Path != nil {
		msg, err = s.msgEngine.SendWithPath(req.To, req.Body, *req.Path)
	} else {
		msg, err = s.msgEngine.Send(req.To, req.Body)
	}
	if err != nil {
		status := http.StatusInternalServerError
		if req.Path != nil && strings.HasPrefix(err.Error(), "path:") {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
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

// handleMarkConversationRead clears the unread badge for a conversation.
//
// The request body is deliberately ignored — the frontend post() helper always
// sends one, and decoding it would only add a 400 path for no benefit. The
// callsign is used verbatim: engine conversation keys are SSID-suffixed source
// callsigns, so normalizing here would 200 while clearing nothing.
func (s *Server) handleMarkConversationRead(w http.ResponseWriter, r *http.Request) {
	if s.msgEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "message engine not available"})
		return
	}

	callsign := chi.URLParam(r, "callsign")
	readAt := time.Now().UTC()

	conv, err := s.msgEngine.MarkRead(callsign, readAt)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Persist synchronously here, not in the event bridge: engine emits are
	// non-blocking and may be dropped, which would lose the read marker.
	// Persist the engine-returned value — it may have been clamped forward.
	if s.store != nil && conv.LastReadAt != nil {
		if err := s.store.SaveConversationRead(callsign, *conv.LastReadAt); err != nil {
			log.Printf("[server] save conversation read: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	// Deliberately NOT written to the activity log. A read receipt fires again
	// every time a message lands on an open thread, for every viewing client;
	// logging it would bury the operational record (and its ICS-309 export)
	// under hundreds of rows that record nothing an operator did.
	writeJSON(w, http.StatusOK, map[string]any{
		"callsign":    callsign,
		"unreadCount": 0,
		"lastReadAt":  conv.LastReadAt,
	})
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
	needsSetup := false
	messagePath := "WIDE1-1,WIDE2-1"
	beaconPath := "WIDE1-1,WIDE2-1"
	if s.configMgr != nil {
		st := s.configMgr.Get().Station
		needsSetup = st.Callsign == "N0CALL"
		messagePath = st.MessagePath
		beaconPath = st.BeaconPath
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transports":  len(s.transports.Statuses()),
		"wsClients":   s.hub.ClientCount(),
		"authMode":    "invite",
		"needsSetup":  needsSetup,
		"messagePath": messagePath,
		"beaconPath":  beaconPath,
	})
}

// --- Session handlers ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions not configured"})
		return
	}

	var req struct {
		Name       string `json:"name"`
		PIN        string `json:"pin"`
		SavedToken string `json:"savedToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	user, err := s.sessions.Create(strings.TrimSpace(req.Name), session.CreateOpts{
		PIN:   req.PIN,
		Token: req.SavedToken,
	})
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

func (s *Server) handleApproveUser(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions not configured"})
		return
	}

	var req struct {
		UserID string       `json:"userId"`
		Role   session.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
		return
	}
	if session.RoleLevel(req.Role) < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
		return
	}

	user, err := s.sessions.Approve(req.UserID, req.Role)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	if s.actLogger != nil {
		admin, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    admin.ID,
			UserName:  admin.Name,
			Action:    "access_approved",
			Target:    user.Name,
			Details:   string(req.Role),
		})
	}

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleDenyUser(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions not configured"})
		return
	}

	var req struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
		return
	}

	if err := s.sessions.Deny(req.UserID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	if s.actLogger != nil {
		admin, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    admin.ID,
			UserName:  admin.Name,
			Action:    "access_denied",
			Target:    req.UserID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"userId": req.UserID, "status": "denied"})
}

func (s *Server) handleGetPending(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, s.sessions.Pending())
}

func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	users := s.sessions.All()
	// Strip tokens — only show public info
	type publicUser struct {
		ID          string         `json:"id"`
		Name        string         `json:"name"`
		Role        session.Role   `json:"role"`
		Status      session.Status `json:"status"`
		Callsign    string         `json:"callsign,omitempty"`
		ConnectedAt time.Time      `json:"connectedAt"`
	}
	result := make([]publicUser, len(users))
	for i, u := range users {
		result[i] = publicUser{
			ID:          u.ID,
			Name:        u.Name,
			Role:        u.Role,
			Status:      u.Status,
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
		Name        string  `json:"name"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		SymbolTable string  `json:"symbolTable"`
		SymbolCode  string  `json:"symbolCode"`
		Comment     string  `json:"comment"`
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
		Name        string  `json:"name"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		SymbolTable string  `json:"symbolTable"`
		SymbolCode  string  `json:"symbolCode"`
		Comment     string  `json:"comment"`
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

// annotationWithTx wraps an annotation with runtime transmitting state.
type annotationWithTx struct {
	store.Annotation
	Transmitting bool `json:"transmitting"`
}

func (s *Server) addTransmitState(anns []annotation.Annotation) []annotationWithTx {
	result := make([]annotationWithTx, len(anns))
	for i, a := range anns {
		result[i] = annotationWithTx{
			Annotation:   a,
			Transmitting: s.annMgr.IsTransmitting(a.ID),
		}
	}
	return result
}

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
		writeJSON(w, http.StatusOK, s.addTransmitState(s.annMgr.AllFiltered(filter)))
		return
	}

	writeJSON(w, http.StatusOK, s.addTransmitState(s.annMgr.All()))
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

func (s *Server) handlePromoteAnnotationToMission(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil || s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations or net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ann, found := s.annMgr.Get(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "annotation not found"})
		return
	}

	if len(ann.MissionIDs) > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "annotation already linked to a mission"})
		return
	}

	// Find an active net to create the mission in.
	nets := s.netMgr.GetNets()
	var activeNet *store.Net
	for _, n := range nets {
		if n.Status == "open" {
			activeNet = &n
			break
		}
	}
	if activeNet == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active net for mission creation"})
		return
	}

	// Extract lat/lon from GeoJSON geometry.
	type geomCoords struct {
		Coordinates []float64 `json:"coordinates"`
	}
	var gc geomCoords
	if err := json.Unmarshal([]byte(ann.Geometry), &gc); err != nil || len(gc.Coordinates) < 2 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot extract coordinates from annotation geometry"})
		return
	}
	lon, lat := gc.Coordinates[0], gc.Coordinates[1]

	mission, err := s.netMgr.CreateMission(store.NetMission{
		NetID:       activeNet.ID,
		Title:       ann.Label,
		Description: ann.Description,
		Priority:    ann.Priority,
		Status:      "open",
		Lat:         &lat,
		Lon:         &lon,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Link the annotation to the new mission.
	existing := *ann
	existing.MissionIDs = []string{mission.ID}
	updated, err := s.annMgr.Update(existing)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"annotation": updated, "mission": mission})

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    "annotation_promoted",
			Target:    ann.Label,
			Details:   mission.ID,
		})
	}
}

func (s *Server) handleLinkAnnotation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")

	var req struct {
		MissionID string `json:"missionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MissionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missionId required"})
		return
	}

	ann, err := s.annMgr.AddMissionLink(id, req.MissionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ann)
}

func (s *Server) handleUnlinkAnnotation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")

	// Check for specific missionId to unlink (query param).
	missionID := r.URL.Query().Get("missionId")
	if missionID != "" {
		ann, err := s.annMgr.RemoveMissionLink(id, missionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ann)
		return
	}

	// No missionId specified — clear all links.
	ann, err := s.annMgr.ClearMissionLink(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ann)
}

func (s *Server) handleGetAnnotationTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, annotation.AllTemplates())
}

func (s *Server) handleGetOperations(w http.ResponseWriter, _ *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, s.annMgr.AllOperations())
}

func (s *Server) handleGetOperation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")
	op, ok := s.annMgr.GetOperation(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "operation not found"})
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (s *Server) handleCreateOperation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	var req store.Operation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if user, ok := UserFromContext(r.Context()); ok {
		req.CreatedBy = user.ID
	}

	op, err := s.annMgr.CreateOperation(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, op)
}

func (s *Server) handleArchiveOperation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.annMgr.ArchiveOperation(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	op, _ := s.annMgr.GetOperation(id)
	writeJSON(w, http.StatusOK, op)
}

func (s *Server) handleTransmitAnnotation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ann, err := s.annMgr.PromoteToObject(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, annotationWithTx{Annotation: *ann, Transmitting: true})

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionAnnotationTransmitted,
			Target:    ann.Label,
		})
	}
}

func (s *Server) handleStopTransmitAnnotation(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotations not available"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.annMgr.StopTransmitting(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})

	if s.actLogger != nil {
		user, _ := UserFromContext(r.Context())
		s.actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionAnnotationStopTransmit,
			Target:    id,
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

// --- Weather handlers ---

func (s *Server) handleGetWeatherStations(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	stations, err := s.store.LoadWeatherStations()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if stations == nil {
		stations = []store.WeatherReading{}
	}
	writeJSON(w, http.StatusOK, stations)
}

func (s *Server) handleGetWeatherReadings(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	callsign := chi.URLParam(r, "callsign")
	filter := store.WeatherFilter{Callsign: callsign}

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
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}

	readings, err := s.store.LoadWeatherReadings(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if readings == nil {
		readings = []store.WeatherReading{}
	}
	writeJSON(w, http.StatusOK, readings)
}

func (s *Server) handleGetWeatherConfig(w http.ResponseWriter, _ *http.Request) {
	s.weatherMu.RLock()
	cfg := s.weatherCfg
	s.weatherMu.RUnlock()
	writeJSON(w, http.StatusOK, cfg)
}

// --- Telemetry handlers ---

func (s *Server) handleGetTelemetryStations(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	stations, err := s.store.LoadTelemetryStations()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if stations == nil {
		stations = []store.TelemetryReading{}
	}
	writeJSON(w, http.StatusOK, stations)
}

func (s *Server) handleGetTelemetryReadings(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"readings": []any{}, "params": nil})
		return
	}

	callsign := chi.URLParam(r, "callsign")
	filter := store.TelemetryFilter{Callsign: callsign}

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
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}

	readings, err := s.store.LoadTelemetryReadings(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if readings == nil {
		readings = []store.TelemetryReading{}
	}

	// Look up telemetry params from the live station tracker.
	var params *aprs.TelemetryParams
	st, ok := s.tracker.Get(callsign)
	if ok && st.TelemetryParams != nil {
		params = st.TelemetryParams
	}

	writeJSON(w, http.StatusOK, map[string]any{"readings": readings, "params": params})
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

// --- ICS-309 handlers ---

func (s *Server) handleGetICS309(w http.ResponseWriter, r *http.Request) {
	if s.msgEngine == nil {
		writeJSON(w, http.StatusOK, ics309.Report{})
		return
	}

	q := r.URL.Query()
	header := ics309.Header{
		IncidentName: q.Get("incidentName"),
		OperatorName: q.Get("operatorName"),
		StationID:    q.Get("stationId"),
	}

	from, to := s.parseICS309TimeRange(q.Get("from"), q.Get("to"))
	header.DateFrom = from
	header.DateTo = to

	// If netId provided, pre-populate header from net session.
	if netID := q.Get("netId"); netID != "" && s.netMgr != nil {
		if n, ok := s.netMgr.GetNet(netID); ok {
			if header.IncidentName == "" {
				header.IncidentName = n.Name
			}
			if header.StationID == "" {
				header.StationID = n.NCSCallsign
			}
			if n.OpenedAt != nil && q.Get("from") == "" {
				header.DateFrom = *n.OpenedAt
			}
			if n.ClosedAt != nil && q.Get("to") == "" {
				header.DateTo = *n.ClosedAt
			}
		}
	}

	msgs := s.msgEngine.Messages("")
	rows := ics309.BuildFromMessages(msgs, header.DateFrom, header.DateTo, q.Get("method"))

	writeJSON(w, http.StatusOK, ics309.Report{Header: header, Rows: rows})
}

func (s *Server) handleExportICS309CSV(w http.ResponseWriter, r *http.Request) {
	if s.msgEngine == nil {
		http.Error(w, "message engine not available", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	header := ics309.Header{
		IncidentName: q.Get("incidentName"),
		OperatorName: q.Get("operatorName"),
		StationID:    q.Get("stationId"),
	}

	from, to := s.parseICS309TimeRange(q.Get("from"), q.Get("to"))
	header.DateFrom = from
	header.DateTo = to

	if netID := q.Get("netId"); netID != "" && s.netMgr != nil {
		if n, ok := s.netMgr.GetNet(netID); ok {
			if header.IncidentName == "" {
				header.IncidentName = n.Name
			}
			if header.StationID == "" {
				header.StationID = n.NCSCallsign
			}
			if n.OpenedAt != nil && q.Get("from") == "" {
				header.DateFrom = *n.OpenedAt
			}
			if n.ClosedAt != nil && q.Get("to") == "" {
				header.DateTo = *n.ClosedAt
			}
		}
	}

	msgs := s.msgEngine.Messages("")
	rows := ics309.BuildFromMessages(msgs, header.DateFrom, header.DateTo, q.Get("method"))

	report := ics309.Report{Header: header, Rows: rows}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=ics309.csv")
	ics309.ExportCSV(w, report)
}

func (s *Server) parseICS309TimeRange(fromStr, toStr string) (time.Time, time.Time) {
	now := time.Now()
	from := now.Add(-24 * time.Hour)
	to := now

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

// --- Tile cache handlers ---

func (s *Server) handleTileProxy(w http.ResponseWriter, r *http.Request) {
	z, _ := strconv.Atoi(chi.URLParam(r, "z"))
	x, _ := strconv.Atoi(chi.URLParam(r, "x"))
	y := chi.URLParam(r, "y")
	// Strip .png extension if present
	y = strings.TrimSuffix(y, ".png")
	yInt, _ := strconv.Atoi(y)

	data, err := s.tileCache.Get(r.Context(), z, x, yInt)
	if err != nil {
		http.Error(w, "tile fetch failed", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

func (s *Server) handleTileCacheStatus(w http.ResponseWriter, _ *http.Request) {
	if s.tileCache == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":   false,
			"tileCount": 0,
			"diskUsage": 0,
		})
		return
	}

	status, err := s.tileCache.Status()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":   true,
		"tileCount": status.TileCount,
		"diskUsage": status.DiskUsage,
		"maxZoom":   s.tileCache.MaxZoom(),
	})
}

func (s *Server) handleTilePreload(w http.ResponseWriter, r *http.Request) {
	if s.tileCache == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tile cache not available"})
		return
	}

	var req struct {
		South   float64 `json:"south"`
		West    float64 `json:"west"`
		North   float64 `json:"north"`
		East    float64 `json:"east"`
		ZoomMin int     `json:"zoomMin"`
		ZoomMax int     `json:"zoomMax"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.ZoomMax > s.tileCache.MaxZoom() {
		req.ZoomMax = s.tileCache.MaxZoom()
	}

	tiles := tilecache.TilesForBounds(req.South, req.West, req.North, req.East, req.ZoomMin, req.ZoomMax)

	// Run preload in background with detached context (request context cancels on response)
	go s.tileCache.Preload(context.Background(), tiles)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "started",
		"tileCount": len(tiles),
	})
}

func (s *Server) handleTileEstimate(w http.ResponseWriter, r *http.Request) {
	if s.tileCache == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tile cache not available"})
		return
	}

	var req struct {
		South   float64 `json:"south"`
		West    float64 `json:"west"`
		North   float64 `json:"north"`
		East    float64 `json:"east"`
		ZoomMin int     `json:"zoomMin"`
		ZoomMax int     `json:"zoomMax"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.ZoomMax > s.tileCache.MaxZoom() {
		req.ZoomMax = s.tileCache.MaxZoom()
	}

	count := tilecache.EstimateTileCount(req.South, req.West, req.North, req.East, req.ZoomMin, req.ZoomMax)

	writeJSON(w, http.StatusOK, map[string]int{"tileCount": count})
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

	var netAnns []store.Annotation
	if s.annMgr != nil {
		netAnns = s.annMgr.AllForNet(id)
	}
	if netAnns == nil {
		netAnns = []store.Annotation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"net":         n,
		"checkIns":    s.netMgr.GetCheckIns(id),
		"missions":    s.netMgr.GetMissions(id),
		"annotations": netAnns,
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

func (s *Server) handleSetOpsView(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
		Zoom float64 `json:"zoom"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := s.netMgr.SetOpsView(id, req.Lat, req.Lon, req.Zoom); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	n, _ := s.netMgr.GetNet(id)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handlePinStation(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	callsign := chi.URLParam(r, "callsign")

	n, err := s.netMgr.PinStation(id, callsign)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleUnpinStation(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	callsign := chi.URLParam(r, "callsign")

	n, err := s.netMgr.UnpinStation(id, callsign)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleReorderPins(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Callsigns []string `json:"callsigns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	n, err := s.netMgr.ReorderPins(id, req.Callsigns)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

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
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	ci, err := s.netMgr.CheckIn(id, req.Callsign, req.Traffic, req.Category)
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

func (s *Server) handleToggleNotePin(w http.ResponseWriter, r *http.Request) {
	if s.netMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "net control not available"})
		return
	}

	netID := chi.URLParam(r, "id")
	noteID := chi.URLParam(r, "noteId")

	updated, err := s.netMgr.ToggleNotePin(netID, noteID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updated)
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

	// Check for missionId in query param or body.
	missionID := r.URL.Query().Get("missionId")
	if missionID == "" {
		var req struct {
			MissionID string `json:"missionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.MissionID != "" {
			missionID = req.MissionID
		}
	}

	var ci *store.NetCheckIn
	var err error
	if missionID != "" {
		ci, err = s.netMgr.UnassignMission(id, ciId, missionID)
	} else {
		ci, err = s.netMgr.UnassignAllMissions(id, ciId)
	}
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

func (s *Server) handleGetNetAnnotations(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	id := chi.URLParam(r, "id")
	anns := s.annMgr.AllForNet(id)
	if anns == nil {
		anns = []store.Annotation{}
	}
	writeJSON(w, http.StatusOK, anns)
}

func (s *Server) handleImportNetAnnotations(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotation manager not available"})
		return
	}

	id := chi.URLParam(r, "id")

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()

	filename := strings.ToLower(header.Filename)
	var items []annotation.ImportItem

	if strings.HasSuffix(filename, ".gpx") {
		items, err = annotation.ParseGPXWaypoints(file)
	} else if strings.HasSuffix(filename, ".kml") {
		items, err = annotation.ParseKMLPlacemarks(file)
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported file type, use .gpx or .kml"})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	imported, err := s.annMgr.ImportAnnotations(id, items)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, imported)
}

func (s *Server) handleImportAnnotations(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotation manager not available"})
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()

	filename := strings.ToLower(header.Filename)
	var items []annotation.ImportItem

	if strings.HasSuffix(filename, ".gpx") {
		items, err = annotation.ParseGPXWaypoints(file)
	} else if strings.HasSuffix(filename, ".kml") {
		items, err = annotation.ParseKMLPlacemarks(file)
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported file type, use .gpx or .kml"})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	imported, err := s.annMgr.ImportAnnotations("", items)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, imported)
}

func (s *Server) handleCopyNetAnnotations(w http.ResponseWriter, r *http.Request) {
	if s.annMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "annotation manager not available"})
		return
	}

	id := chi.URLParam(r, "id")
	sourceNetId := chi.URLParam(r, "sourceNetId")

	copied, err := s.annMgr.CopyAnnotationsFromNet(sourceNetId, id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, copied)
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

// --- Checkpoint Progress Handlers ---

func (s *Server) handleGetCheckpoints(w http.ResponseWriter, r *http.Request) {
	if s.cpMgr == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	netID := chi.URLParam(r, "id")
	checkpoints, err := s.cpMgr.GetCheckpointsForNet(netID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if checkpoints == nil {
		checkpoints = []checkpoint.CheckpointWithPassages{}
	}
	writeJSON(w, http.StatusOK, checkpoints)
}

func (s *Server) handleGetProgress(w http.ResponseWriter, r *http.Request) {
	if s.cpMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"netId": chi.URLParam(r, "id"), "checkpoints": []any{}, "elements": []any{}})
		return
	}
	netID := chi.URLParam(r, "id")
	progress, err := s.cpMgr.GetProgress(netID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, progress)
}

func (s *Server) handleUpdateCheckpointMeta(w http.ResponseWriter, r *http.Request) {
	if s.cpMgr == nil {
		http.Error(w, "checkpoint manager not available", http.StatusServiceUnavailable)
		return
	}
	netID := chi.URLParam(r, "id")
	cpID := chi.URLParam(r, "cpId")

	var meta store.CheckpointMeta
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	meta.AnnotationID = cpID
	meta.NetID = netID

	result, err := s.cpMgr.SetMeta(meta)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLogPassage(w http.ResponseWriter, r *http.Request) {
	if s.cpMgr == nil {
		http.Error(w, "checkpoint manager not available", http.StatusServiceUnavailable)
		return
	}
	netID := chi.URLParam(r, "id")
	cpID := chi.URLParam(r, "cpId")

	var passage store.CheckpointPassage
	if err := json.NewDecoder(r.Body).Decode(&passage); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	passage.CheckpointID = cpID
	passage.NetID = netID

	// Populate ReportedBy from session user if available.
	if passage.ReportedBy == "" {
		if user, ok := UserFromContext(r.Context()); ok {
			passage.ReportedBy = user.Name
		}
	}

	result, err := s.cpMgr.LogPassage(passage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
