package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/beacon"
	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/geocode/w3w"
	"github.com/narvel/nymeria/internal/gps"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/ride/phase"
	"github.com/narvel/nymeria/internal/ride/reconcile"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/tilecache"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/wxalert"
	nweb "github.com/narvel/nymeria/web"
)

// Server is the main HTTP server for Nymeria.
type Server struct {
	router      chi.Router
	hub         *ws.Hub
	tracker     station.Tracker
	transports  *transport.Manager
	msgEngine   message.Engine
	objManager  *object.Manager
	beaconMgr   *beacon.Manager
	store       store.Store
	sessions    session.Manager
	actLogger   activity.Logger
	annMgr      *annotation.Manager
	netMgr      *netcontrol.Manager
	cpMgr       *checkpoint.Manager
	rideMgr     *ride.Manager
	rideTraffic *ride.TrafficManager
	courseMgr   *course.Manager
	phaseMgr    *phase.Manager
	recMgr      *reconcile.Manager
	tileCache   *tilecache.Cache
	gpsMgr      *gps.Manager
	configMgr   *config.Manager
	stationCfg  config.StationConfig
	weatherMu   sync.RWMutex
	weatherCfg  config.WeatherConfig
	w3w         *w3w.Client
	w3wMu       sync.RWMutex

	// NWS weather alerts (internal/wxalert). wxMgr is hot-swappable: enabling
	// wx_alerts, or changing its contact/base URL/data dir, needs a fresh
	// Manager (its HTTP client is fixed at construction), which app.go's
	// config.Manager.OnChange callback builds and installs via
	// SetWxAlertManager with no server restart. wxBaseConfig is that
	// manager's own config, carried forward so the per-net-refresh path
	// (buffer/interrupt/mute overrides) can rebuild it without knowing
	// connection details it was never given.
	wxMu         sync.RWMutex
	wxMgr        *wxalert.Manager
	wxMgrStop    chan struct{}
	wxBaseConfig wxalert.Config

	// wxSeen dedupes activity-log/timeline entries derived from polled
	// snapshots: a MatchedAlert's UpdatedAt changes exactly when the Manager
	// just recomputed its notify class in reaction to a real change, so
	// "have we logged this UpdatedAt for this id yet" is a reliable,
	// Manager-API-only way to log each transition exactly once without the
	// Events() channel exposing the underlying Change list itself.
	wxSeenMu sync.Mutex
	wxSeen   map[string]time.Time

	wxLinkMu      sync.Mutex
	wxLinkWasDown bool

	// wxFootprintDirty debounces footprint recomputation: annotation/roster/
	// GPS/net-lifecycle changes all send here (non-blocking), and a single
	// background loop coalesces bursts into one refreshWxFootprint call a
	// few seconds later instead of one per event.
	wxFootprintDirty chan struct{}
}

// New creates a new Server.
func New(tracker station.Tracker, tm *transport.Manager, eng message.Engine, db store.Store, opts ...Option) *Server {
	s := &Server{
		router:           chi.NewRouter(),
		hub:              ws.NewHub(),
		tracker:          tracker,
		transports:       tm,
		msgEngine:        eng,
		store:            db,
		wxFootprintDirty: make(chan struct{}, 1),
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
	go s.weatherPurgeLoop()
	go s.telemetryPurgeLoop()
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
	if s.cpMgr != nil {
		go s.bridgeCheckpointEvents()
	}
	if s.rideMgr != nil {
		go s.bridgeRideEvents()
	}
	if s.rideTraffic != nil {
		go s.bridgeRideTrafficEvents()
	}
	if s.courseMgr != nil {
		go s.bridgeCourseEvents()
	}
	if s.phaseMgr != nil {
		go s.bridgeRidePhaseEvents()
	}
	if s.recMgr != nil {
		go s.bridgeReconcileEvents()
	}
	if s.tileCache != nil {
		go s.bridgeTileCacheEvents()
	}
	if s.sessions != nil {
		go s.bridgeSessionEvents()
	}
	if s.gpsMgr != nil {
		go s.bridgeGPS()
	}
	if s.wxMgr != nil {
		s.wxMgrStop = make(chan struct{})
		go s.bridgeWxAlertEvents(s.wxMgr, s.wxMgrStop)
	}
	go s.wxFootprintDebounceLoop()

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

// WithCheckpointManager sets the checkpoint manager on the server.
func WithCheckpointManager(mgr *checkpoint.Manager) Option {
	return func(s *Server) {
		s.cpMgr = mgr
	}
}

// WithRideManager sets the ride mode (internal/ride) SAG manager on the
// server. Every /nets/{id}/sag/* route answers 503 when this is nil.
func WithRideManager(mgr *ride.Manager) Option {
	return func(s *Server) {
		s.rideMgr = mgr
	}
}

// WithCourseManager sets the course closure (internal/course) manager on
// the server. Every /nets/{id}/course/* route answers 503 when this is nil.
func WithCourseManager(mgr *course.Manager) Option {
	return func(s *Server) {
		s.courseMgr = mgr
	}
}

// WithRideTrafficManager sets the ride-mode supply/medical traffic manager
// (internal/ride's TrafficManager, WP4) on the server. Every
// /nets/{id}/ride/* route answers 503 when this is nil.
func WithRideTrafficManager(mgr *ride.TrafficManager) Option {
	return func(s *Server) {
		s.rideTraffic = mgr
	}
}

// WithPhaseManager sets the ride phase manager (internal/ride/phase, WP5b)
// on the server. GET/POST /nets/{id}/ride/phase answer 503 when this is
// nil; every other route is unaffected either way.
func WithPhaseManager(mgr *phase.Manager) Option {
	return func(s *Server) {
		s.phaseMgr = mgr
	}
}

// WithReconcileManager sets the ride-mode close-out/reconciliation manager
// (internal/ride/reconcile, WP5) on the server. Every
// /nets/{id}/ride/{accounting,closeout,shift-summaries,handoff*,briefing}
// and /nets/{id}/ics21{1,4} route answers 503 when this is nil.
func WithReconcileManager(mgr *reconcile.Manager) Option {
	return func(s *Server) {
		s.recMgr = mgr
	}
}

// WithGPSManager sets the live GPS manager on the server.
func WithGPSManager(mgr *gps.Manager) Option {
	return func(s *Server) {
		s.gpsMgr = mgr
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

// WithWeatherConfig provides weather dashboard configuration.
func WithWeatherConfig(cfg config.WeatherConfig) Option {
	return func(s *Server) {
		s.weatherCfg = cfg
	}
}

// WithConfigManager sets the config manager for settings API.
func WithConfigManager(mgr *config.Manager) Option {
	return func(s *Server) {
		s.configMgr = mgr
	}
}

// WithWxAlertManager sets the initial NWS weather alert manager (nil is
// valid — every wx route then answers 503, the tile-cache-style pattern).
// baseConfig is the exact Config the manager was constructed with; the
// per-net footprint refresh path rebuilds Config from it plus a net's
// overrides rather than guessing connection details.
func WithWxAlertManager(mgr *wxalert.Manager, baseConfig wxalert.Config) Option {
	return func(s *Server) {
		s.wxMgr = mgr
		s.wxBaseConfig = baseConfig
	}
}

// WithWhat3Words sets the what3words client on the server. Pass a client
// even when unconfigured (empty API key) — Configured() reporting false is
// what drives the 503 "not configured" response, and it's what makes a key
// entered later through Settings live-effective with no restart.
func WithWhat3Words(c *w3w.Client) Option {
	return func(s *Server) {
		s.w3w = c
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

		// Persist weather readings
		if s.store != nil && evt.Station.Weather != nil && (evt.Type == station.EventNewStation || evt.Type == station.EventStationUpdate) {
			wx := evt.Station.Weather
			key := aprs.Address{Call: evt.Station.Callsign, SSID: evt.Station.SSID}.String()
			wr := store.WeatherReading{
				Callsign:    key,
				Timestamp:   evt.Station.LastHeard,
				Temperature: wx.Temperature,
				WindDir:     wx.WindDir,
				WindSpeed:   wx.WindSpeed,
				WindGust:    wx.WindGust,
				Humidity:    wx.Humidity,
				Pressure:    wx.Pressure,
				Rain1h:      wx.Rain1h,
				Rain24h:     wx.Rain24h,
				RainToday:   wx.RainToday,
				Luminosity:  wx.Luminosity,
			}
			if err := s.store.SaveWeatherReading(wr); err != nil {
				log.Printf("[server] save weather reading: %v", err)
			}
		}

		// Persist telemetry readings
		if s.store != nil && evt.Station.Telemetry != nil && (evt.Type == station.EventNewStation || evt.Type == station.EventStationUpdate) {
			tel := evt.Station.Telemetry
			key := aprs.Address{Call: evt.Station.Callsign, SSID: evt.Station.SSID}.String()
			tr := store.TelemetryReading{
				Callsign:  key,
				Timestamp: evt.Station.LastHeard,
				Seq:       tel.Seq,
				Analog1:   tel.Analog[0],
				Analog2:   tel.Analog[1],
				Analog3:   tel.Analog[2],
				Analog4:   tel.Analog[3],
				Analog5:   tel.Analog[4],
				Digital:   int(tel.Digital),
			}
			if err := s.store.SaveTelemetryReading(tr); err != nil {
				log.Printf("[server] save telemetry reading: %v", err)
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
		// Persist only real message payloads — conversation-scoped events
		// (claim/unclaim/read) carry a zero Message and would otherwise
		// write a junk row keyed by an empty ID.
		if s.store != nil && evt.Message.ID != "" {
			if err := s.store.SaveMessage(evt.Message); err != nil {
				log.Printf("[server] save message: %v", err)
			}
		}

		// Broadcast via WebSocket
		msg := map[string]any{
			"type":    evt.Type,
			"message": evt.Message,
		}
		if evt.Conversation != nil {
			msg["conversation"] = evt.Conversation
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
		var payload any = evt.Data
		if evt.Batch != nil {
			payload = evt.Batch
		}
		msg := map[string]any{
			"type": evt.Type,
			"data": payload,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal annotation event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
		s.TriggerWxFootprintRefresh()

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
		s.TriggerWxFootprintRefresh()

		// A sag-category check-in being released (explicit checkout, or any
		// other path that ends up setting Status=released) frees its
		// dispatched/enroute/onscene legs; a loaded leg is left for the crew
		// to resolve and logs a warning instead. evt.Data is the live Go
		// value netcontrol just emitted, not a JSON round-trip, so a direct
		// type assertion is enough (mission_updated below round-trips
		// because it also has to handle a Data shape from elsewhere).
		if evt.Type == netcontrol.EventCheckInUpdated && s.rideMgr != nil {
			if ci, ok := evt.Data.(store.NetCheckIn); ok {
				if ci.Status == netcontrol.OpReleased && ci.Category == netcontrol.CatSAG {
					s.rideMgr.ReleaseVehicle(ci.NetID, ci.ID, "vehicle checked out")
				}
				// Any update to a sag unit re-announces it, which is what
				// carries a roster CATEGORY change onto open SAG boards: the
				// check-in only becomes a vehicle at the moment its category
				// is set, and nothing else emits sag_vehicle_updated for it.
				// AnnounceVehicle no-ops on non-sag units, so this needs no
				// guard of its own.
				s.rideMgr.AnnounceVehicle(ci.NetID, ci.ID)
			}
		}

		// Closing a net deliberately does NOT resolve its annotations. Bulk-resolving
		// them stamped a ResolvedAt nobody earned, destroyed the record of what was
		// still outstanding when the net ended, and could not be undone. Whether a
		// net's annotations are still live is already carried by the net's own
		// Status/ClosedAt (#117).
		//
		// It DOES release their net scope. That is a different operation and a
		// reversible one: the rows keep their status, geometry and history and
		// simply return to the unscoped pool, where "Link existing" can find
		// them again. Without it a course could be run exactly once — every
		// location stayed owned by the net that closed, and the only way to
		// reuse it was Copy from, which duplicates rows.
		if evt.Type == netcontrol.EventNetUpdated && s.annMgr != nil {
			if n, ok := evt.Data.(store.Net); ok && n.Status == netcontrol.StatusClosed {
				if released, err := s.annMgr.ReleaseNetAnnotations(n.ID); err != nil {
					log.Printf("[server] release annotations for net %s: %v", n.ID, err)
				} else if len(released) > 0 {
					log.Printf("[server] net %s closed: released %d annotation(s) back to the unscoped pool", n.ID, len(released))
				}
			}
		}

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

// bridgeCheckpointEvents reads checkpoint events and broadcasts via WebSocket.
func (s *Server) bridgeCheckpointEvents() {
	for evt := range s.cpMgr.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal checkpoint event: %v", err)
			continue
		}
		s.hub.Broadcast(data)

		// Numbering a stop BUILDS the course, so the derived course state has
		// to be recomputed and re-sent. Nothing else does it: course state is
		// emitted after course mutations, and this is a checkpoint mutation.
		if evt.Type == checkpoint.EventCheckpointMetaUpdate && s.courseMgr != nil {
			if meta, ok := evt.Data.(store.CheckpointMeta); ok {
				s.courseMgr.RefreshState(meta.NetID)
			}
		}
	}
}

// bridgeRideEvents reads ride mode (internal/ride) events and broadcasts
// them via WebSocket. Its timeline rows are emitted under the same
// "net_timeline_entry" type netcontrol's own timeline uses (see
// ride.TimelineEntryEventType), so the existing timeline panel shows them
// with no frontend changes.
func (s *Server) bridgeRideEvents() {
	for evt := range s.rideMgr.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal ride event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// bridgeRideTrafficEvents reads ride-mode supply/medical traffic
// (internal/ride's TrafficManager, WP4) domain events and broadcasts them
// via WebSocket. Its timeline rows go through netcontrol's own
// AddTimelineEventWithDetails instead, so they are already broadcast by
// bridgeNetControlEvents — this bridge only carries the
// ride_supply_*/ride_medical_* domain events.
func (s *Server) bridgeRideTrafficEvents() {
	for evt := range s.rideTraffic.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal ride traffic event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// bridgeCourseEvents reads course closure (internal/course) events and
// broadcasts them via WebSocket.
func (s *Server) bridgeCourseEvents() {
	for evt := range s.courseMgr.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal course event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// bridgeRidePhaseEvents reads ride phase (internal/ride/phase, WP5b) events
// and broadcasts them via WebSocket, so the strip's phase chip updates live.
func (s *Server) bridgeRidePhaseEvents() {
	for evt := range s.phaseMgr.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal ride phase event: %v", err)
			continue
		}
		s.hub.Broadcast(data)
	}
}

// bridgeReconcileEvents reads ride reconciliation (internal/ride/reconcile,
// WP5) events — shift-summary updates and hand-off activity — and
// broadcasts them via WebSocket. Its close-out-forced/shift-filed/handoff
// timeline rows go through netcontrol's own AddTimelineEvent(WithDetails),
// so they are already broadcast by bridgeNetControlEvents.
func (s *Server) bridgeReconcileEvents() {
	for evt := range s.recMgr.Events() {
		msg := map[string]any{
			"type": evt.Type,
			"data": evt.Data,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal reconcile event: %v", err)
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

// broadcastPaths notifies connected browsers that outbound AX.25 paths changed.
func (s *Server) broadcastPaths(messagePath, beaconPath string) {
	if s.hub == nil {
		return
	}
	msg := map[string]any{
		"type":        "paths_updated",
		"messagePath": messagePath,
		"beaconPath":  beaconPath,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[server] marshal paths event: %v", err)
		return
	}
	s.hub.Broadcast(data)
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

// WxAlertManager returns the live NWS weather alert manager, or nil when the
// feature has never been enabled. Every wx route reads through this rather
// than a captured field, so a hot-swap (SetWxAlertManager) takes effect on
// the very next request.
func (s *Server) WxAlertManager() *wxalert.Manager {
	s.wxMu.RLock()
	defer s.wxMu.RUnlock()
	return s.wxMgr
}

// WxBaseConfig returns the config the live manager was constructed with
// (zero value when there is none), for the per-net-refresh path to rebuild
// Config from without needing its own copy of connection details.
func (s *Server) WxBaseConfig() wxalert.Config {
	s.wxMu.RLock()
	defer s.wxMu.RUnlock()
	return s.wxBaseConfig
}

// SetWxAlertManager hot-swaps the live manager: app.go's config.Manager.
// OnChange callback calls this whenever enabling/disabling wx alerts, or a
// contact/base URL/data dir change, requires building a fresh Manager (its
// HTTP client and zone cache are fixed at construction — see
// wxalert.Manager.UpdateConfig's own doc comment). The previous manager's
// event bridge goroutine is stopped so it cannot leak; mgr may be nil to
// disable the feature entirely.
func (s *Server) SetWxAlertManager(mgr *wxalert.Manager, baseConfig wxalert.Config) {
	s.wxMu.Lock()
	oldStop := s.wxMgrStop
	s.wxMgr = mgr
	s.wxBaseConfig = baseConfig
	var stop chan struct{}
	if mgr != nil {
		stop = make(chan struct{})
	}
	s.wxMgrStop = stop
	s.wxMu.Unlock()

	if oldStop != nil {
		close(oldStop)
	}
	s.wxSeenMu.Lock()
	s.wxSeen = nil
	s.wxSeenMu.Unlock()
	if mgr != nil {
		go s.bridgeWxAlertEvents(mgr, stop)
	}
}

// bridgeWxAlertEvents reads one manager instance's events until either its
// channel closes or stop fires (the manager was swapped out from under it).
func (s *Server) bridgeWxAlertEvents(mgr *wxalert.Manager, stop <-chan struct{}) {
	for {
		select {
		case evt, ok := <-mgr.Events():
			if !ok {
				return
			}
			s.handleWxAlertEvent(mgr, evt)
		case <-stop:
			return
		}
	}
}

// bridgeSessionEvents reads session lifecycle events and broadcasts/sends via WebSocket.
func (s *Server) bridgeSessionEvents() {
	for evt := range s.sessions.Events() {
		msg := map[string]any{
			"type": string(evt.Type),
			"user": evt.User,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[server] marshal session event: %v", err)
			continue
		}

		switch evt.Type {
		case session.EventAccessApproved, session.EventAccessDenied:
			// Send targeted message to the affected user + broadcast for admin UI updates
			s.hub.SendTo(evt.User.ID, data)
			s.hub.Broadcast(data)
		default:
			// access_request: broadcast to all (admins filter on frontend)
			s.hub.Broadcast(data)
		}
	}
}

// TriggerWxFootprintRefresh queues a debounced weather-watch-footprint
// recompute. Safe to call from anywhere (a full buffer just means a refresh
// is already queued) and safe when wx alerts was never enabled (the loop's
// refreshWxFootprint call is then a no-op).
func (s *Server) TriggerWxFootprintRefresh() {
	select {
	case s.wxFootprintDirty <- struct{}{}:
	default:
	}
}

// wxFootprintDebounceLoop coalesces bursts of footprint-affecting changes
// (an annotation import, a roster of check-ins arriving, GPS ticks) into one
// refreshWxFootprint call a few seconds after the burst quiets down, plus an
// unconditional periodic sweep so a change with no dedicated trigger (e.g. a
// checkpoint sequence edit) still converges eventually.
func (s *Server) wxFootprintDebounceLoop() {
	const debounce = 3 * time.Second
	const safetyNet = 5 * time.Minute

	ticker := time.NewTicker(safetyNet)
	defer ticker.Stop()

	for {
		select {
		case <-s.wxFootprintDirty:
			time.Sleep(debounce)
			for drained := false; !drained; {
				select {
				case <-s.wxFootprintDirty:
				default:
					drained = true
				}
			}
			s.refreshWxFootprint("footprint")
		case <-ticker.C:
			s.refreshWxFootprint("footprint")
		}
	}
}

// weatherPurgeLoop periodically deletes old weather readings based on RetentionDays config.
func (s *Server) weatherPurgeLoop() {
	s.weatherMu.RLock()
	days := s.weatherCfg.RetentionDays
	s.weatherMu.RUnlock()
	if days <= 0 {
		days = 7
	}
	if s.store == nil {
		return
	}

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		if n, err := s.store.PurgeWeatherReadings(cutoff); err != nil {
			log.Printf("[server] purge weather readings: %v", err)
		} else if n > 0 {
			log.Printf("[server] purged %d old weather readings", n)
		}
	}
}

// telemetryPurgeLoop periodically deletes old telemetry readings (reuses weather retention config).
func (s *Server) telemetryPurgeLoop() {
	s.weatherMu.RLock()
	days := s.weatherCfg.RetentionDays
	s.weatherMu.RUnlock()
	if days <= 0 {
		days = 7
	}
	if s.store == nil {
		return
	}

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		if n, err := s.store.PurgeTelemetryReadings(cutoff); err != nil {
			log.Printf("[server] purge telemetry readings: %v", err)
		} else if n > 0 {
			log.Printf("[server] purged %d old telemetry readings", n)
		}
	}
}

// BroadcastRawPacket sends a parsed packet to all WebSocket clients for the protocol inspector.
func (s *Server) BroadcastRawPacket(pkt *aprs.Packet, source string) {
	msg := struct {
		Type       string         `json:"type"`
		Raw        string         `json:"raw"`
		Timestamp  time.Time      `json:"timestamp"`
		Source     string         `json:"source"`
		PacketType string         `json:"packetType"`
		From       aprs.Address   `json:"from"`
		To         aprs.Address   `json:"to"`
		Path       []aprs.Address `json:"path"`
		Packet     *aprs.Packet   `json:"packet"`
	}{
		Type:       "packet",
		Raw:        pkt.Frame.String(),
		Timestamp:  time.Now().UTC(),
		Source:     source,
		PacketType: pkt.Type.String(),
		From:       pkt.Frame.Source,
		To:         pkt.Frame.Destination,
		Path:       pkt.Frame.Path,
		Packet:     pkt,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[server] marshal raw packet: %v", err)
		return
	}
	s.hub.Broadcast(data)
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
