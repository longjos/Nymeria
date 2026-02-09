package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/beacon"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
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
		go s.bridgeAnnotationEvents()
	}
	if s.actLogger != nil {
		go s.bridgeActivityEvents()
	}
	if s.netMgr != nil {
		go s.bridgeNetControlEvents()
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

// bridgeAnnotationEvents reads annotation events and broadcasts via WebSocket.
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

// bridgeNetControlEvents reads net control events and broadcasts via WebSocket.
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
