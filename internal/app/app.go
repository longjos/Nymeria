package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

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
	"github.com/narvel/nymeria/internal/server"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/tilecache"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/transport/aprsis"
	"github.com/narvel/nymeria/internal/transport/kisstcp"
	"github.com/narvel/nymeria/internal/transport/serial"
	"github.com/narvel/nymeria/internal/wxalert"
)

// Options configures New.
type Options struct {
	Config     config.Config // fully loaded config (env overrides already applied by config.Load)
	ConfigPath string        // YAML path backing the settings API (config.NewManager); e.g. "nymeria.yaml"
	Version    string        // build version string for logging; "" is treated as "dev"
}

// App is the fully wired Nymeria runtime, minus the HTTP listener.
// The caller owns the http.Server / net.Listener and serves App.Handler().
type App struct {
	cancel     context.CancelFunc
	srv        *server.Server
	bcn        *beacon.Manager
	tm         *transport.Manager
	msgEngine  *message.MemoryEngine
	objMgr     *object.Manager
	store      store.Store
	cfg        config.Config
	version    string
	once       sync.Once
	fanoutDone chan struct{}
}

// New constructs and starts the whole Nymeria runtime (store, tracker, transports,
// message engine, object manager, beacon, sessions, managers, server, frame fan-out).
// The caller owns the HTTP listener and must call Handler() to serve requests.
func New(opts Options) (*App, error) {
	cfg := opts.Config
	version := opts.Version
	if version == "" {
		version = "dev"
	}

	// Initialize store
	db := store.NewSQLiteStore(cfg.Store.Path)
	if err := db.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	// Initialize tracker and transport manager
	tracker := station.NewMemoryTracker(cfg.Station)
	tm := transport.NewManager()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Start tracker sweep
	tracker.Start(ctx, time.Minute)

	// Hydrate tracker from database
	if stations, err := db.LoadStations(); err != nil {
		log.Printf("warning: failed to load stations from db: %v", err)
	} else {
		// Load latest weather readings to hydrate station weather data
		wxMap := make(map[string]store.WeatherReading)
		if wxStations, err := db.LoadWeatherStations(); err != nil {
			log.Printf("warning: failed to load weather stations from db: %v", err)
		} else {
			for _, wr := range wxStations {
				wxMap[wr.Callsign] = wr
			}
		}

		for _, s := range stations {
			key := aprs.Address{Call: s.Callsign, SSID: s.SSID}.String()
			if tracks, err := db.LoadTrackPoints(key, cfg.Station.TrackMaxPoints); err == nil {
				s.Track = tracks
			}
			if wr, ok := wxMap[key]; ok {
				s.Weather = &aprs.WeatherData{
					Temperature: wr.Temperature,
					WindDir:     wr.WindDir,
					WindSpeed:   wr.WindSpeed,
					WindGust:    wr.WindGust,
					Humidity:    wr.Humidity,
					Pressure:    wr.Pressure,
					Rain1h:      wr.Rain1h,
					Rain24h:     wr.Rain24h,
					RainToday:   wr.RainToday,
					Luminosity:  wr.Luminosity,
				}
			}
			tracker.Update(s)
		}
		if len(stations) > 0 {
			log.Printf("loaded %d stations from database", len(stations))
		}
		if len(wxMap) > 0 {
			log.Printf("hydrated %d weather stations from database", len(wxMap))
		}
	}

	// Configure transports from config
	for i, tc := range cfg.Transports {
		switch tc.Type {
		case "aprsis":
			if tc.Callsign == "" {
				tc.Callsign = cfg.Station.Callsign
			}
			t := aprsis.New(tc)
			tm.AddNamed(fmt.Sprintf("aprsis-%d", i), t, tc.Name)
		case "kisstcp":
			t := kisstcp.New(tc)
			tm.AddNamed(fmt.Sprintf("kisstcp-%d", i), t, tc.Name)
		case "serial":
			t := serial.New(tc)
			tm.AddNamed(fmt.Sprintf("serial-%d", i), t, tc.Name)
		default:
			log.Printf("warning: unknown transport type %q, skipping", tc.Type)
		}
	}

	// Create message engine
	parser := aprs.NewParser()
	msgEngine := message.NewMemoryEngine(
		cfg.Station.Callsign,
		func(frame aprs.APRSFrame) error { return tm.Send(frame) },
		message.DefaultRetryConfig(),
	)
	msgEngine.UpdateIdentity(cfg.Station.Callsign, cfg.Station.SSID)
	msgEngine.UpdatePath(stationPath(cfg.Station.MessagePath))

	// Hydrate message engine from database
	if msgs, err := db.LoadMessages(); err != nil {
		log.Printf("warning: failed to load messages from db: %v", err)
	} else {
		msgEngine.Import(msgs)
		if len(msgs) > 0 {
			log.Printf("loaded %d messages from database", len(msgs))
		}
	}

	// Hydrate per-conversation read markers so unread badges survive restart
	if reads, err := db.LoadConversationReads(); err != nil {
		log.Printf("warning: failed to load conversation read state from db: %v", err)
	} else {
		msgEngine.ImportReadState(reads)
	}

	// Create object manager
	objMgr := object.NewManager(
		cfg.Station.Callsign,
		cfg.Station.SSID,
		func(frame aprs.APRSFrame) error { return tm.Send(frame) },
		object.ManagerConfig{
			RetransmitInterval: 10 * time.Minute,
		},
	)
	objMgr.UpdatePath(stationPath(cfg.Station.BeaconPath))
	objMgr.Start(ctx)

	// Connect all transports
	if err := tm.ConnectAll(ctx); err != nil {
		log.Printf("warning: transport connect failed: %v", err)
	}

	// Create live GPS manager (host position source for the own-position
	// marker and GPS-driven smart beaconing). Nil when disabled — enabling
	// it from false->true requires a restart (see cfgMgr.OnChange below).
	var gpsMgr *gps.Manager
	if cfg.GPS.Enabled {
		gpsMgr = gps.NewManager(toGPSConfig(cfg.GPS))
		if err := gpsMgr.Start(ctx); err != nil {
			log.Printf("warning: gps start failed: %v", err)
		} else {
			log.Printf("gps enabled (%s %s)", cfg.GPS.Type, gpsMgr.Status().Target)
		}
	}

	// Create beacon manager
	bcnCfg := beacon.Config{
		Enabled:     cfg.Beacon.Enabled,
		Interval:    cfg.Beacon.Interval,
		Comment:     cfg.Beacon.Comment,
		Path:        stationPath(cfg.Station.BeaconPath),
		LiveMaxAge:  cfg.GPS.StaleAfter,
		SmartBeacon: toSmartConfig(cfg.Beacon.SmartBeacon),
	}
	if bcnCfg.Interval == 0 {
		bcnCfg.Interval = 10 * time.Minute
	}
	if bcnCfg.Comment == "" {
		bcnCfg.Comment = cfg.Station.Comment
	}
	bcn := beacon.New(bcnCfg, beacon.StationInfo{
		Callsign:    cfg.Station.Callsign,
		SSID:        cfg.Station.SSID,
		Lat:         cfg.Station.Lat,
		Lon:         cfg.Station.Lon,
		SymbolTable: cfg.Station.SymbolTable,
		SymbolCode:  cfg.Station.SymbolCode,
	}, func(f aprs.APRSFrame) error {
		return tm.Send(f)
	})
	if gpsMgr != nil && cfg.GPS.UseForBeacon {
		bcn.SetPositionSource(func() (beacon.LiveFix, bool) {
			fix, ok := gpsMgr.Current()
			if !ok || !fix.HasPosition() {
				return beacon.LiveFix{}, false
			}
			return beacon.LiveFix{
				Lat: fix.Lat, Lon: fix.Lon,
				SpeedKnots: fix.SpeedKnots, Course: fix.Course,
				HasCourse: fix.HasCourse, Age: fix.Age(time.Now()),
			}, true
		})
	}
	if bcnCfg.Enabled {
		bcn.Start(ctx)
		log.Printf("beaconing enabled (interval %s)", bcnCfg.Interval)
	}

	// Create session manager
	sessCfg := session.MemoryManagerConfig{
		PIN:               cfg.Session.PIN,
		InactivityTimeout: cfg.Session.InactivityTimeout,
		ReconnectWindow:   cfg.Session.ReconnectWindow,
	}
	sessMgr := session.NewMemoryManager(sessCfg)

	// Create activity logger
	actLogger := activity.NewStoreLogger(db)

	sessMgr.OnDisconnect = func(user *session.User) {
		actLogger.Log(activity.Entry{
			Timestamp: time.Now(),
			UserID:    user.ID,
			UserName:  user.Name,
			Action:    activity.ActionSessionEnded,
			Details:   "timeout",
		})
	}
	sessMgr.Start(ctx, time.Minute)
	log.Printf("session manager started (PIN %s, timeout %s)",
		func() string {
			if cfg.Session.PIN != "" {
				return "configured"
			}
			return "disabled"
		}(),
		cfg.Session.InactivityTimeout)

	// Create annotation manager
	annMgr := annotation.NewManager(db)
	if err := annMgr.Load(); err != nil {
		log.Printf("warning: failed to load annotations: %v", err)
	}

	// Create net control manager
	netMgr := netcontrol.NewManager(db, tracker)
	if err := netMgr.Load(); err != nil {
		log.Printf("warning: failed to load nets: %v", err)
	}

	// Create checkpoint manager
	cpMgr := checkpoint.NewManager(db, annMgr)
	if err := cpMgr.Load(); err != nil {
		log.Printf("warning: failed to load checkpoint data: %v", err)
	}

	// Clean up checkpoint metadata whenever a checkpoint annotation is removed,
	// on both the single and bulk delete paths, and restore it on undo.
	annMgr.SetBeforeDelete(func(a store.Annotation) {
		if a.Category == annotation.CategoryCheckpoint {
			if err := cpMgr.DeleteMetaForAnnotation(a.ID); err != nil {
				log.Printf("[app] delete checkpoint meta for %s: %v", a.ID, err)
			}
		}
	})
	annMgr.SetCheckpointSnapshot(cpMgr.MetaForAnnotation, cpMgr.SetMeta)

	// Create ride mode (internal/ride) manager — SAG (support-and-gear
	// transport) requests and vehicles. netMgr satisfies ride.CheckInSource
	// and annMgr satisfies ride.AnnotationSource; a package nobody
	// constructs is not done, so this is wired in even though no net may be
	// running "bike-ride" profile yet.
	rideMgr := ride.NewManager(db, netMgr, annMgr, ride.DefaultConfig())
	// Validate SAG request priorities against the net's own agency-configured
	// priority ladder (WP1's NetRideConfig.PriorityTiers), the same ladder
	// supply/medical traffic already use. Nets with no override keep the
	// shipped Marin ladder.
	rideMgr.SetTierSource(ride.NewNetProfileTiers(netMgr))
	if err := rideMgr.Load(); err != nil {
		log.Printf("warning: failed to load ride mode data: %v", err)
	}

	// Create ride mode's supply/medical traffic manager (WP4). netMgr
	// satisfies ride.NetLookup and ride.TimelineSink (via
	// AddTimelineEventWithDetails) structurally; ride.NewNetProfilePolicy
	// adapts netMgr's per-net ride config (WP1) into the bib-withholding
	// and priority-tier-vocabulary Policy this package needs, without this
	// package knowing netcontrol's field names.
	rideTraffic := ride.NewTrafficManager(db, netMgr, netMgr, actLogger, ride.NewNetProfilePolicy(netMgr))
	if err := rideTraffic.Load(); err != nil {
		log.Printf("warning: failed to load ride traffic data: %v", err)
	}

	// Create course closure (internal/course) manager — shutoff points,
	// rider support exceptions, sweep tracking, and the per-station closure
	// ladder. Wired in unconditionally like checkpoint/ride (the 503 guard
	// in each handler already covers "not enabled" for a general net).
	courseMgr := course.NewManager(db, cpMgr, annMgr)
	if err := courseMgr.Load(); err != nil {
		log.Printf("warning: failed to load course closure data: %v", err)
	}
	cpMgr.SetOnPassage(courseMgr.OnPassage)
	annMgr.SetStatusGuard(courseMgr.GuardAnnotationStatus)

	// Create the ride phase manager (internal/ride/phase, WP5b) — the
	// operator-set pre-start/launched/mid-ride/closing/collapse/reconcile
	// state the ride status strip is keyed off, plus the live-computed
	// "what should we move to next, and why" suggestion the NCS confirms.
	// Wired in unconditionally like checkpoint/ride/course (the 503 guard
	// in its handlers already covers "not enabled" for a general net, and
	// the manager itself refuses any net whose profile isn't bike-ride).
	phaseMgr := phase.NewManager(db, netMgr, courseMgr, cpMgr)
	if err := phaseMgr.Load(); err != nil {
		log.Printf("warning: failed to load ride phase data: %v", err)
	}

	// Create the ride reconciliation manager (internal/ride/reconcile, WP5)
	// — supported-rider accounting (reused from courseMgr, not duplicated),
	// the post-ride close-out checklist, SAG driver shift summaries, and
	// NCS shift-relief hand-off. Wired in unconditionally like
	// checkpoint/ride/course (the 503 guard in each handler already covers
	// "not enabled" for a general net).
	recMgr := reconcile.NewManager(db, netMgr, courseMgr, rideMgr, annMgr)
	if err := recMgr.Load(); err != nil {
		log.Printf("warning: failed to load ride reconciliation data: %v", err)
	}
	recMgr.SetMessageEngine(msgEngine)

	// Initialize tile cache
	var tc *tilecache.Cache
	if cfg.TileCache.Enabled {
		dataDir := cfg.TileCache.DataDir
		if dataDir == "" {
			dataDir = filepath.Join(filepath.Dir(cfg.Store.Path), "tiles")
		}
		var err error
		tc, err = tilecache.New(tilecache.Config{
			DataDir: dataDir,
			TileURL: cfg.TileCache.TileURL,
			MaxZoom: cfg.TileCache.MaxZoom,
		})
		if err != nil {
			log.Printf("warning: tile cache init failed: %v", err)
		} else {
			log.Printf("tile cache enabled (dir: %s, max zoom: %d)", dataDir, cfg.TileCache.MaxZoom)
		}
	}

	// Initialize the what3words proxy client. Always constructed — even
	// with an empty key, even with what3words.enabled: false at boot — so
	// that both an API key pasted in Settings and an Enabled toggle
	// flipped in Settings take effect live, with no restart. Configured()
	// (and therefore every proxy handler's guard) reflects the live
	// enabled+key state on every call, not just the state at boot.
	w3wClient, err := w3w.New(w3w.Config{
		APIKey:     cfg.What3Words.APIKey,
		BaseURL:    cfg.What3Words.BaseURL,
		ForwardTTL: cfg.What3Words.ForwardTTL,
		SuggestTTL: cfg.What3Words.SuggestTTL,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("what3words: %w", err)
	}
	w3wClient.SetEnabled(cfg.What3Words.Enabled)

	// Create config manager for settings API
	cfgMgr := config.NewManager(opts.ConfigPath, cfg)

	// Create the NWS weather alert manager — only when enabled, so an
	// unconfigured install never sends a single request to api.weather.gov
	// and s.wxMgr stays nil (the tile-cache pattern every wx route's 503
	// gate relies on). A construction failure (e.g. a malformed contact
	// that slipped past config.Validate) is logged and treated as "not
	// configured" rather than failing boot — this is an optional feature,
	// the same tolerance tile cache init gets below.
	wxCfg := toWxAlertConfig(cfg.WxAlerts, cfg.Store.Path, version)
	var wxMgr *wxalert.Manager
	if cfg.WxAlerts.Enabled {
		var err error
		wxMgr, err = wxalert.NewManager(wxCfg)
		if err != nil {
			log.Printf("warning: wx alerts manager init failed: %v", err)
			wxMgr = nil
		} else {
			hydrateWxAlerts(db, wxMgr)
		}
	}

	// Register config change callbacks for live reload
	cfgMgr.OnChange(func(old, newCfg config.Config) {
		// Beacon: update config and restart if needed
		bcn.UpdateConfig(beacon.Config{
			Enabled:     newCfg.Beacon.Enabled,
			Interval:    newCfg.Beacon.Interval,
			Comment:     newCfg.Beacon.Comment,
			Path:        stationPath(newCfg.Station.BeaconPath),
			LiveMaxAge:  newCfg.GPS.StaleAfter,
			SmartBeacon: toSmartConfig(newCfg.Beacon.SmartBeacon),
		})
		if newCfg.Beacon.Enabled && !bcn.IsRunning() {
			bcn.Start(ctx)
			log.Println("[config] beacon started")
		} else if !newCfg.Beacon.Enabled && bcn.IsRunning() {
			bcn.Stop()
			log.Println("[config] beacon stopped")
		}

		// Station identity: propagate to beacon, message engine, object manager
		if old.Station.Callsign != newCfg.Station.Callsign ||
			old.Station.SSID != newCfg.Station.SSID ||
			old.Station.Lat != newCfg.Station.Lat ||
			old.Station.Lon != newCfg.Station.Lon ||
			old.Station.SymbolTable != newCfg.Station.SymbolTable ||
			old.Station.SymbolCode != newCfg.Station.SymbolCode {
			bcn.UpdateStationInfo(beacon.StationInfo{
				Callsign:    newCfg.Station.Callsign,
				SSID:        newCfg.Station.SSID,
				Lat:         newCfg.Station.Lat,
				Lon:         newCfg.Station.Lon,
				SymbolTable: newCfg.Station.SymbolTable,
				SymbolCode:  newCfg.Station.SymbolCode,
			})
			msgEngine.UpdateIdentity(newCfg.Station.Callsign, newCfg.Station.SSID)
			objMgr.UpdateStationInfo(newCfg.Station.Callsign, newCfg.Station.SSID)
			log.Printf("[config] station identity updated: %s-%d", newCfg.Station.Callsign, newCfg.Station.SSID)
		}

		if old.Station.MessagePath != newCfg.Station.MessagePath {
			msgEngine.UpdatePath(stationPath(newCfg.Station.MessagePath))
			log.Printf("[config] message path updated: %q", newCfg.Station.MessagePath)
		}
		if old.Station.BeaconPath != newCfg.Station.BeaconPath {
			objMgr.UpdatePath(stationPath(newCfg.Station.BeaconPath))
			log.Printf("[config] beacon path updated: %q", newCfg.Station.BeaconPath)
		}

		// Station tracker settings: hot-reload
		tracker.UpdateConfig(newCfg.Station)

		// Transports: reconcile add/remove/reconfigure
		reconcileTransports(ctx, old, newCfg, tm)

		// Session: update PIN and timeout
		sessMgr.UpdateConfig(session.MemoryManagerConfig{
			PIN:               newCfg.Session.PIN,
			InactivityTimeout: newCfg.Session.InactivityTimeout,
			ReconnectWindow:   newCfg.Session.ReconnectWindow,
		})

		// Logging: update log level
		if old.Logging.Level != newCfg.Logging.Level {
			log.Printf("[config] log level changed: %s → %s", old.Logging.Level, newCfg.Logging.Level)
		}

		// GPS: hot-reload thresholds/target on an already-running manager.
		// Enabling GPS from false->true has no manager to reload (gpsMgr is
		// nil in that case) — the settings handler flags that as needing a
		// restart.
		if gpsMgr != nil {
			gpsMgr.UpdateConfig(toGPSConfig(newCfg.GPS))
		} else if newCfg.GPS.Enabled && !old.GPS.Enabled {
			log.Println("[config] gps.enabled turned on but no GPS manager is running; restart required")
		}
	})

	// Create and start server
	serverOpts := []server.Option{
		server.WithBeaconManager(bcn),
		server.WithObjectManager(objMgr),
		server.WithSessionManager(sessMgr),
		server.WithActivityLogger(actLogger),
		server.WithAnnotationManager(annMgr),
		server.WithNetControlManager(netMgr),
		server.WithCheckpointManager(cpMgr),
		server.WithRideManager(rideMgr),
		server.WithRideTrafficManager(rideTraffic),
		server.WithCourseManager(courseMgr),
		server.WithPhaseManager(phaseMgr),
		server.WithReconcileManager(recMgr),
		server.WithConfigManager(cfgMgr),
		server.WithStationConfig(cfg.Station),
		server.WithWeatherConfig(cfg.Weather),
	}
	if tc != nil {
		serverOpts = append(serverOpts, server.WithTileCache(tc))
	}
	if gpsMgr != nil {
		serverOpts = append(serverOpts, server.WithGPSManager(gpsMgr))
	}
	serverOpts = append(serverOpts, server.WithWhat3Words(w3wClient))
	serverOpts = append(serverOpts, server.WithWxAlertManager(wxMgr, wxCfg))
	srv := server.New(tracker, tm, msgEngine, db, serverOpts...)

	if wxMgr != nil {
		wxMgr.Start(ctx)
		log.Printf("wx alerts enabled (poll %s, buffer %g mi)", wxCfg.PollInterval, wxCfg.DefaultBufferMiles)
	}
	// Populate the watch footprint (own position, plus any open net's
	// course/roster) and regionally scope the poller before its first tick.
	srv.TriggerWxFootprintRefresh()

	// wx alerts settings changed live: a connection-detail change
	// (enabled/contact/base URL/data dir/poll interval/sounds — all fixed
	// at Manager/Poller construction, see wxalert.Manager.UpdateConfig's own
	// doc comment) rebuilds and hot-swaps the manager with no restart;
	// anything else is a policy-only change refreshWxFootprint's
	// UpdateConfig call already picks up live.
	cfgMgr.OnChange(func(old, newCfg config.Config) {
		needsRebuild := old.WxAlerts.Enabled != newCfg.WxAlerts.Enabled ||
			old.WxAlerts.Contact != newCfg.WxAlerts.Contact ||
			old.WxAlerts.BaseURL != newCfg.WxAlerts.BaseURL ||
			old.WxAlerts.DataDir != newCfg.WxAlerts.DataDir ||
			old.WxAlerts.PollInterval != newCfg.WxAlerts.PollInterval ||
			old.WxAlerts.Sounds != newCfg.WxAlerts.Sounds
		if !needsRebuild {
			if newCfg.WxAlerts.Enabled {
				srv.TriggerWxFootprintRefresh()
			}
			return
		}

		newWxCfg := toWxAlertConfig(newCfg.WxAlerts, newCfg.Store.Path, version)
		var newMgr *wxalert.Manager
		if newCfg.WxAlerts.Enabled {
			var err error
			newMgr, err = wxalert.NewManager(newWxCfg)
			if err != nil {
				log.Printf("[config] wx alerts rebuild failed: %v", err)
				return
			}
			hydrateWxAlerts(db, newMgr)
		}
		srv.SetWxAlertManager(newMgr, newWxCfg)
		if newMgr != nil {
			newMgr.Start(ctx)
		}
		srv.TriggerWxFootprintRefresh()
		log.Printf("[config] wx alerts manager rebuilt (enabled=%v)", newCfg.WxAlerts.Enabled)
	})

	fanoutDone := make(chan struct{})
	// Frame processing loop: parse frames → tracker + message engine + object manager + tactical
	go func() {
		defer close(fanoutDone)
		for {
			select {
			case <-ctx.Done():
				return
			case tf, ok := <-tm.TaggedFrames():
				if !ok {
					return
				}
				pkt, err := parser.Parse(tf.Frame)
				if err != nil {
					continue
				}
				srv.BroadcastRawPacket(pkt, tf.Source)
				tracker.HandlePacket(pkt, tf.SourceName)
				msgEngine.HandlePacket(pkt)
				objMgr.HandlePacket(pkt)
				srv.HandleTacticalPacket(pkt)
			}
		}
	}()

	return &App{
		cancel:     cancel,
		srv:        srv,
		bcn:        bcn,
		tm:         tm,
		msgEngine:  msgEngine,
		objMgr:     objMgr,
		store:      db,
		cfg:        cfg,
		version:    version,
		fanoutDone: fanoutDone,
	}, nil
}

// Handler returns the HTTP handler for the App. Never nil after successful New.
func (a *App) Handler() http.Handler {
	return a.srv
}

// Config returns the effective config the App was built with.
func (a *App) Config() config.Config {
	return a.cfg
}

// Shutdown tears down the runtime. Order: beacon → transports → cancel context
// (stops fan-out, tracker sweep, object retransmit, session sweeper) → wait for
// fan-out → message engine → object manager → store.
// The http.Server is owned by the caller and is not shut down here.
// Idempotent: second and later calls return nil immediately.
func (a *App) Shutdown(ctx context.Context) error {
	var err error
	a.once.Do(func() {
		if a.bcn != nil {
			a.bcn.Stop()
		}
		if a.tm != nil {
			_ = a.tm.CloseAll()
		}
		if a.cancel != nil {
			a.cancel()
		}
		if a.fanoutDone != nil {
			select {
			case <-a.fanoutDone:
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		if a.msgEngine != nil {
			a.msgEngine.Close()
		}
		if a.objMgr != nil {
			a.objMgr.Close()
		}
		if a.store != nil {
			_ = a.store.Close()
		}
	})
	return err
}

// toGPSConfig adapts config.GPSConfig (YAML/JSON settings) to gps.Config
// (the internal/gps package's own, dependency-free config shape).
func toGPSConfig(c config.GPSConfig) gps.Config {
	return gps.Config{
		Enabled:      c.Enabled,
		Type:         c.Type,
		Host:         c.Host,
		Port:         c.Port,
		Device:       c.Device,
		Baud:         c.Baud,
		MinInterval:  c.MinInterval,
		StaleAfter:   c.StaleAfter,
		UseForBeacon: c.UseForBeacon,
	}
}

// toSmartConfig adapts config.SmartBeaconConfig to beacon.SmartConfig (the
// beacon package deliberately has no config-package dependency). An
// enabled-but-unconfigured smart beacon (all-zero speeds/rates) is filled
// with sane defaults so flipping the toggle on alone is enough to use it.
func toSmartConfig(c *config.SmartBeaconConfig) *beacon.SmartConfig {
	if c == nil {
		return nil
	}
	sb := &beacon.SmartConfig{
		Enabled:   c.Enabled,
		FastSpeed: c.FastSpeed,
		SlowSpeed: c.SlowSpeed,
		FastRate:  c.FastRate,
		SlowRate:  c.SlowRate,
		TurnAngle: c.TurnAngle,
		TurnSlope: c.TurnSlope,
	}
	if sb.Enabled {
		if sb.FastSpeed == 0 {
			sb.FastSpeed = 60
		}
		if sb.SlowSpeed == 0 {
			sb.SlowSpeed = 5
		}
		if sb.FastRate == 0 {
			sb.FastRate = 60 * time.Second
		}
		if sb.SlowRate == 0 {
			sb.SlowRate = 30 * time.Minute
		}
		if sb.TurnAngle == 0 {
			sb.TurnAngle = 28
		}
		if sb.TurnSlope == 0 {
			sb.TurnSlope = 26
		}
	}
	return sb
}

// toWxAlertConfig adapts config.WxAlertsConfig (YAML/JSON settings) to
// wxalert.Config (wxalert never imports internal/config — see its own
// package doc). storePath's directory is the default zone-cache location
// (<store dir>/wxalerts) when DataDir is unset; version+contact become the
// NWS-policy-required User-Agent ("Nymeria/<version> (<contact>)").
func toWxAlertConfig(c config.WxAlertsConfig, storePath, version string) wxalert.Config {
	dataDir := c.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(filepath.Dir(storePath), "wxalerts")
	}
	ua := ""
	if c.Contact != "" {
		ua = fmt.Sprintf("Nymeria/%s (%s)", version, c.Contact)
	}
	return wxalert.Config{
		Enabled: c.Enabled, UserAgent: ua, BaseURL: c.BaseURL, PollInterval: c.PollInterval,
		ZoneDataDir: dataDir, DefaultBufferMiles: c.DefaultBufferMiles,
		InterruptEvents: append([]string{}, c.InterruptEvents...),
		WatchNotify:     wxalert.NotifyClass(c.WatchNotify),
		AdvisoryNotify:  wxalert.NotifyClass(c.AdvisoryNotify),
		StatementNotify: wxalert.NotifyClass(c.StatementNotify),
		Sounds:          c.Sounds,
	}
}

// hydrateWxAlerts restores the registry across a restart from the last
// snapshot internal/server's manager event bridge persisted. It round-trips
// through encoding/json even though RegistrySnapshot's element type is
// unexported — reflection only cares that the fields it walks are exported,
// which Active/Ended's MatchedAlert/PrevID are. A missing or corrupt
// snapshot (including a brand-new database) just starts empty, same as
// today with the feature off.
func hydrateWxAlerts(db store.Store, mgr *wxalert.Manager) {
	blob, ok, err := db.GetWxMeta("registry_snapshot")
	if err != nil {
		log.Printf("warning: read wx alerts registry snapshot: %v", err)
		return
	}
	if !ok || blob == "" {
		return
	}
	var snap wxalert.RegistrySnapshot
	if err := json.Unmarshal([]byte(blob), &snap); err != nil {
		log.Printf("warning: parse wx alerts registry snapshot: %v", err)
		return
	}
	mgr.RestoreRegistry(snap)
	if n := len(snap.Active); n > 0 {
		log.Printf("restored %d active wx alert(s) from database", n)
	}
}

// stationPath parses a configured TNC2 path. Invalid values fall back to WIDE1-1,WIDE2-1
// so a live reload cannot leave outbound traffic unpathable.
func stationPath(s string) []aprs.Address {
	path, err := aprs.ParsePath(s)
	if err != nil {
		log.Printf("[config] invalid path %q: %v; using WIDE1-1,WIDE2-1", s, err)
		return aprs.DefaultRFPath()
	}
	return path
}
