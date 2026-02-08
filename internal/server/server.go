package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/station"
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
}

// New creates a new Server.
func New(tracker station.Tracker, tm *transport.Manager, eng message.Engine) *Server {
	s := &Server{
		router:     chi.NewRouter(),
		hub:        ws.NewHub(),
		tracker:    tracker,
		transports: tm,
		msgEngine:  eng,
	}

	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)

	s.routes()
	s.serveFrontend()

	go s.hub.Run()
	go s.bridgeTrackerEvents()
	if eng != nil {
		go s.bridgeMessageEvents()
	}

	return s
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

// bridgeTrackerEvents reads station events and broadcasts them as JSON via WebSocket.
func (s *Server) bridgeTrackerEvents() {
	eventNames := map[station.EventType]string{
		station.EventNewStation:     "station_new",
		station.EventStationUpdate:  "station_update",
		station.EventStationExpired: "station_removed",
	}

	for evt := range s.tracker.Events() {
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

// bridgeMessageEvents reads message events and broadcasts them via WebSocket.
func (s *Server) bridgeMessageEvents() {
	for evt := range s.msgEngine.Events() {
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
