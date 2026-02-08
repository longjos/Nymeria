package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/server"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
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
	tracker := station.NewMemoryTracker()
	tm := transport.NewManager()

	// Override listen address if provided
	if *listenAddr != "" {
		cfg.Server.Listen = *listenAddr
	}

	// Create and start server
	srv := server.New(tracker, tm)

	log.Printf("nymeria %s listening on %s", version, cfg.Server.Listen)
	if err := http.ListenAndServe(cfg.Server.Listen, srv); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
