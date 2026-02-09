package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/beacon"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/tilecache"
	"github.com/narvel/nymeria/internal/transport"
	nweb "github.com/narvel/nymeria/web"
)

// Server is the main HTTP server for Nymeria.
type Server struct {
	router     chi.Router
	hub        *ws.Hub
	tracker    station.Tracker
	transports *transport.Manager
	msgEngine  message.Engine
	objManager *object.Manager
	beaconMgr  *beacon.Manager
	store      store.Store
	sessions   session.Manager
	actLogger  activity.Logger
	annMgr     *annotation.Manager
	netMgr     *netcontrol.Manager
	tileCache  *tilecache.Cache
	stationCfg config.StationConfig
}

// New creates a new Server.
func New(tracker station.Tracker, tm *transport.Manager, eng message.Engine, db store.Store, opts ...Option) *Server {
	s := &Server{
		router:     chi.NewRouter(),
		hub:        ws.NewHub(),
		tracker:    tracker,
		transports: tm,
		msgEngine:  eng,
		store:      db,
	}
	for _, opt := range opts {
		opt(s)
	}

	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	if s.sessions != nil {
		s.router.Use(SessionMiddleware(s.sessions))
	}

	s.routes()
	s.serveFrontend()
	s.loadConfigAliases()

	go s.hub.Run()
	go s.bridgeTrackerEvents()
	if eng != nil {
		go s.bridgeMessageEvents()
	}
	if s.objManager != nil {
		go s.bridgeObjectEvents()
	}
	go s.bridgeTransportStatus()
	if s.annMgr != nil {
		if s.objManager != nil {
			s.annMgr.SetObjectManager(s.objManager)
		}
		go s.bridgeAnnotationEvents()
	}
	if s.actLogger != nil {
		go s.bridgeActivityEvents()
	}
	if s.netMgr != nil {
		go s.bridgeNetControlEvents()
	}
	if s.tileCache != nil {
		go s.bridgeTileCacheEvents()
	}

	return s
}

// Option configures the server.
type Option func(*Server)

// WithObjectManager sets the object/item manager on the server.
func WithObjectManager(mgr *object.Manager) Option {
	return func(s *Server) {
		s.objManager = mgr
	}
}

// WithBeaconManager sets the beacon manager on the server.
func WithBeaconManager(mgr *beacon.Manager) Option {
	return func(s *Server) {
		s.beaconMgr = mgr
	}
}

// WithSessionManager sets the session manager on the server.
func WithSessionManager(mgr session.Manager) Option {
	return func(s *Server) {
		s.sessions = mgr
	}
}

// WithActivityLogger sets the activity logger on the server.
func WithActivityLogger(l activity.Logger) Option {
	return func(s *Server) {
		s.actLogger = l
	}
}

// WithAnnotationManager sets the annotation manager on the server.
func WithAnnotationManager(mgr *annotation.Manager) Option {
	return func(s *Server) {
		s.annMgr = mgr
	}
}

// WithNetControlManager sets the net control manager on the server.
func WithNetControlManager(mgr *netcontrol.Manager) Option {
	return func(s *Server) {
		s.netMgr = mgr
	}
}

// WithTileCache sets the tile cache on the server.
func WithTileCache(tc *tilecache.Cache) Option {
	return func(s *Server) {
		s.tileCache = tc
	}
}

// WithStationConfig provides the station configuration for tactical alias loading.
func WithStationConfig(cfg config.StationConfig) Option {
	return func(s *Server) {
		s.stationCfg = cfg
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) serveFrontend() {
	// Serve embedded SvelteKit build
	buildFS, err := fs.Sub(nweb.Build, "build")
	if err != nil {
		panic("failed to access embedded build: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(buildFS))

	// Serve static files, fall back to index.html for SPA routing
	s.router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly
		f, err := buildFS.Open(r.URL.Path[1:]) // strip leading /
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fall back to index.html for SPA routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// Hub returns the WebSocket hub.
func (s *Server) Hub() *ws.Hub {
	return s.hub
}

// bridgeTrackerEvents reads station events, persists to DB, and broadcasts via WebSocket.
func (s *Server) bridgeTrackerEvents() {
	eventNames := map[station.EventType]string{
		station.EventNewStation:     "station_new",
		station.EventStationUpdate:  "station_update",
		station.EventStationExpired: "station_removed",
	}

	for evt := range s.tracker.Events() {
		// Persist to database
		if s.store != nil {
			switch evt.Type {
			case station.EventNewStation, station.EventStationUpdate:
				if err := s.store.SaveStation(evt.Station); err != nil {
					log.Printf("[server] save station: %v", err)
				}
				if len(evt.Station.Track) > 0 {
					tp := evt.Station.Track[len(evt.Station.Track)-1]
					key := aprs.Address{Call: evt.Station.Callsign, SSID: evt.Station.SSID}.String()
					if err := s.store.SaveTrackPoint(key, tp); err != nil {
						log.Printf("[server] save track point: %v", err)
					}
				}
			}
		}

		// Bridge to net control for tracked device position updates
		if s.netMgr != nil && evt.Type != station.EventStationExpired && evt.Station.Position != nil {
			key := aprs.Address{Call: evt.Station.Callsign, SSID: evt.Station.SSID}.String()
			s.netMgr.OnStationUpdate(key, evt.Station.Position, evt.Station.LastHeard)
		}

		// Broadcast via WebSocket
		name, ok := eventNames[evt.Type]
		if !ok {
			continue
		}
		msg := map[string]any{
			"type":    name,
			"station": evt.Station,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal station event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// bridgeMessageEvents reads message events, persists to DB, and broadcasts via WebSocket.
func (s *Server) bridgeMessageEvents() {
	for evt := range s.msgEngine.Events() {
		// Persist to database
		if s.store != nil {
			if err := s.store.SaveMessage(evt.Message); err != nil {
				log.Printf("[server] save message: %v", err)
			}
		}

		// Broadcast via WebSocket
		msg := map[string]any{
			"type":    evt.Type,
			"message": evt.Message,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal message event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// bridgeObjectEvents reads object manager events and broadcasts via WebSocket.
func (s *Server) bridgeObjectEvents() {
	for evt := range s.objManager.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal object event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// bridgeAnnotationEvents reads annotation events, broadcasts via WebSocket,
// and syncs status changes to linked net missions.
func (s *Server) bridgeAnnotationEvents() {
	for evt := range s.annMgr.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal annotation event: %v", err)
			continue
		}
		s.hub.Broadcast(data)

		// Sync annotation status change → mission status.
		if evt.Type == annotation.EventAnnotationStatusChanged && s.netMgr != nil && len(evt.Data.MissionIDs) > 0 {
			missionIDs, missionStatus, err := s.annMgr.SyncStatusToMission(evt.Data.ID)
			if err == nil {
				// Find each linked mission and update its status.
				for _, n := range s.netMgr.GetNets() {
					for _, m := range s.netMgr.GetMissions(n.ID) {
						for _, mid := range missionIDs {
							if m.ID == mid && m.Status != missionStatus {
								m.Status = missionStatus
								s.netMgr.UpdateMission(m)
								break
							}
						}
					}
				}
			}
		}
	}
}

// bridgeActivityEvents reads activity events and broadcasts via WebSocket.
func (s *Server) bridgeActivityEvents() {
	for entry := range s.actLogger.Events() {
		msg := map[string]any{
			"type":  "activity_logged",
			"entry": entry,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal activity event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// bridgeNetControlEvents reads net control events, broadcasts via WebSocket,
// and syncs mission status changes to linked annotations.
func (s *Server) bridgeNetControlEvents() {
	for evt := range s.netMgr.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal net control event: %v", err)
			continue
		}
		s.hub.Broadcast(data)

		// Sync mission status change → annotation status.
		if evt.Type == "mission_updated" && s.annMgr != nil {
			if mData, ok := evt.Data.(map[string]any); ok {
				if mID, _ := mData["id"].(string); mID != "" {
					if mStatus, _ := mData["status"].(string); mStatus != "" {
						s.annMgr.SyncStatusFromMission(mID, mStatus)
					}
				}
			} else {
				// Try JSON round-trip for typed data.
				raw, err := json.Marshal(evt.Data)
				if err == nil {
					var m struct {
						ID     string `json:"id"`
						Status string `json:"status"`
					}
					if json.Unmarshal(raw, &m) == nil && m.ID != "" && m.Status != "" {
						s.annMgr.SyncStatusFromMission(m.ID, m.Status)
					}
				}
			}
		}
	}
}

// bridgeTransportStatus periodically broadcasts transport statuses via WebSocket.
func (s *Server) bridgeTransportStatus() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		statuses := s.transports.Statuses()
		msg := map[string]any{
			"type":       "transport_status",
			"transports": statuses,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal transport status: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// loadConfigAliases seeds the tactical alias table from config on startup.
func (s *Server) loadConfigAliases() {
	if s.store == nil || len(s.stationCfg.TacticalAliases) == 0 {
		return
	}
	for callsign, alias := range s.stationCfg.TacticalAliases {
		a := store.TacticalAlias{
			Callsign:   strings.ToUpper(callsign),
			Alias:      alias,
			AssignedBy: "config",
			UpdatedAt:  time.Now().UTC(),
		}
		if err := s.store.SaveTacticalAlias(a); err != nil {
			log.Printf("[server] save config tactical alias %s: %v", callsign, err)
		}
	}
	log.Printf("loaded %d tactical aliases from config", len(s.stationCfg.TacticalAliases))
}

// broadcastTactical sends a tactical alias event via WebSocket.
func (s *Server) broadcastTactical(eventType string, payload any) {
	msg := map[string]any{
		"type": eventType,
		"data": payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[server] marshal tactical event: %v", err)
		return
	}
	s.hub.Broadcast(data)
}

// bridgeTileCacheEvents reads tile cache events and broadcasts via WebSocket.
func (s *Server) bridgeTileCacheEvents() {
	for evt := range s.tileCache.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal tile cache event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// HandleTacticalPacket detects APRS TACTICAL messages and upserts aliases.
// Called from the packet processing loop in main.go.
func (s *Server) HandleTacticalPacket(pkt *aprs.Packet) {
	if pkt.Type != aprs.PacketTypeMessage || pkt.Message == nil {
		return
	}
	if strings.ToUpper(strings.TrimSpace(pkt.Message.Addressee)) != "TACTICAL" {
		return
	}

	aliases := aprs.ParseTacticalMessage(pkt.Message.Text)
	if aliases == nil {
		return
	}

	for callsign, alias := range aliases {
		a := store.TacticalAlias{
			Callsign:   callsign,
			Alias:      alias,
			AssignedBy: "aprs",
			UpdatedAt:  time.Now().UTC(),
		}
		if err := s.store.SaveTacticalAlias(a); err != nil {
			log.Printf("[server] save aprs tactical alias %s: %v", callsign, err)
			continue
		}
		s.broadcastTactical("tactical_set", a)
		log.Printf("[tactical] APRS alias: %s → %s", callsign, alias)
	}
}
