package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/server"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/transport/aprsis"
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
			// Use station callsign if transport doesn't have one
			if tc.Callsign == "" {
				tc.Callsign = cfg.Station.Callsign
			}
			t := aprsis.New(tc)
			tm.Add(fmt.Sprintf("aprsis-%d", i), t)
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

	// Connect all transports
	if err := tm.ConnectAll(ctx); err != nil {
		log.Printf("warning: transport connect failed: %v", err)
	}

	// Frame processing loop: parse frames → tracker + message engine
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-tm.Frames():
				if !ok {
					return
				}
				pkt, err := parser.Parse(frame)
				if err != nil {
					continue
				}
				tracker.HandlePacket(pkt, "aprsis")
				msgEngine.HandlePacket(pkt)
			}
		}
	}()

	// Override listen address if provided
	if *listenAddr != "" {
		cfg.Server.Listen = *listenAddr
	}

	// Create and start server
	srv := server.New(tracker, tm, msgEngine, db)

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
		tm.CloseAll()
		cancel()
	}()

	log.Printf("nymeria %s listening on %s", version, cfg.Server.Listen)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Println("stopped")
}
