package app

import (
	"context"
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
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/server"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/tilecache"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/transport/aprsis"
	"github.com/narvel/nymeria/internal/transport/kisstcp"
	"github.com/narvel/nymeria/internal/transport/serial"
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

	// Create beacon manager
	bcnCfg := beacon.Config{
		Enabled:  cfg.Beacon.Enabled,
		Interval: cfg.Beacon.Interval,
		Comment:  cfg.Beacon.Comment,
		Path:     stationPath(cfg.Station.BeaconPath),
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

	// Create config manager for settings API
	cfgMgr := config.NewManager(opts.ConfigPath, cfg)

	// Register config change callbacks for live reload
	cfgMgr.OnChange(func(old, newCfg config.Config) {
		// Beacon: update config and restart if needed
		bcn.UpdateConfig(beacon.Config{
			Enabled:  newCfg.Beacon.Enabled,
			Interval: newCfg.Beacon.Interval,
			Comment:  newCfg.Beacon.Comment,
			Path:     stationPath(newCfg.Station.BeaconPath),
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
		server.WithConfigManager(cfgMgr),
		server.WithStationConfig(cfg.Station),
		server.WithWeatherConfig(cfg.Weather),
	}
	if tc != nil {
		serverOpts = append(serverOpts, server.WithTileCache(tc))
	}
	srv := server.New(tracker, tm, msgEngine, db, serverOpts...)

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
