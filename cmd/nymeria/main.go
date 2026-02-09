package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/beacon"
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

var version = "dev"

func main() {
	configPath := flag.String("config", "nymeria.yaml", "path to config file")
	listenAddr := flag.String("listen", "", "override listen address (e.g. :9090)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("nymeria", version)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("failed to load config: %v", err)
		}
		log.Println("no config file found, using defaults")
		cfg = config.DefaultConfig()
	}

	// Initialize store
	db := store.NewSQLiteStore(cfg.Store.Path)
	if err := db.Init(); err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}
	defer db.Close()

	// Initialize tracker and transport manager
	tracker := station.NewMemoryTracker(cfg.Station)
	tm := transport.NewManager()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start tracker sweep
	tracker.Start(ctx, time.Minute)

	// Hydrate tracker from database
	if stations, err := db.LoadStations(); err != nil {
		log.Printf("warning: failed to load stations from db: %v", err)
	} else {
		for _, s := range stations {
			key := aprs.Address{Call: s.Callsign, SSID: s.SSID}.String()
			if tracks, err := db.LoadTrackPoints(key, cfg.Station.TrackMaxPoints); err == nil {
				s.Track = tracks
			}
			tracker.Update(s)
		}
		if len(stations) > 0 {
			log.Printf("loaded %d stations from database", len(stations))
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
			tm.Add(fmt.Sprintf("aprsis-%d", i), t)
		case "kisstcp":
			t := kisstcp.New(tc)
			tm.Add(fmt.Sprintf("kisstcp-%d", i), t)
		case "serial":
			t := serial.New(tc)
			tm.Add(fmt.Sprintf("serial-%d", i), t)
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
	defer msgEngine.Close()

	// Hydrate message engine from database
	if msgs, err := db.LoadMessages(); err != nil {
		log.Printf("warning: failed to load messages from db: %v", err)
	} else {
		msgEngine.Import(msgs)
		if len(msgs) > 0 {
			log.Printf("loaded %d messages from database", len(msgs))
		}
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
	objMgr.Start(ctx)
	defer objMgr.Close()

	// Connect all transports
	if err := tm.ConnectAll(ctx); err != nil {
		log.Printf("warning: transport connect failed: %v", err)
	}

	// Create beacon manager
	bcnCfg := beacon.Config{
		Enabled:  cfg.Beacon.Enabled,
		Interval: cfg.Beacon.Interval,
		Comment:  cfg.Beacon.Comment,
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

	// Override listen address if provided
	if *listenAddr != "" {
		cfg.Server.Listen = *listenAddr
	}

	// Create and start server
	serverOpts := []server.Option{
		server.WithBeaconManager(bcn),
		server.WithObjectManager(objMgr),
		server.WithSessionManager(sessMgr),
		server.WithActivityLogger(actLogger),
		server.WithAnnotationManager(annMgr),
		server.WithNetControlManager(netMgr),
		server.WithStationConfig(cfg.Station),
	}
	if tc != nil {
		serverOpts = append(serverOpts, server.WithTileCache(tc))
	}
	srv := server.New(tracker, tm, msgEngine, db, serverOpts...)

	// Frame processing loop: parse frames → tracker + message engine + object manager + tactical
	go func() {
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
				tracker.HandlePacket(pkt, tf.Source)
				msgEngine.HandlePacket(pkt)
				objMgr.HandlePacket(pkt)
				srv.HandleTacticalPacket(pkt)
			}
		}
	}()

	httpSrv := &http.Server{
		Addr:    cfg.Server.Listen,
		Handler: srv,
	}

	// Graceful shutdown on signal
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		httpSrv.Shutdown(shutdownCtx)
		bcn.Stop()
		tm.CloseAll()
		cancel()
	}()

	log.Printf("nymeria %s listening on %s", version, cfg.Server.Listen)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Println("stopped")
}
