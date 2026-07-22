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

	"github.com/narvel/nymeria/internal/app"
	"github.com/narvel/nymeria/internal/config"
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
		// Load already returned defaults with env overrides applied.
		log.Println("no config file found, using defaults")
	}

	// Override listen address if provided. Kept out of cfg on purpose: the
	// settings API snapshots cfg, and a CLI override must never be persisted
	// back into the YAML by a settings save.
	listen := cfg.Server.Listen
	if *listenAddr != "" {
		listen = *listenAddr
	}

	a, err := app.New(app.Options{
		Config:     cfg,
		ConfigPath: *configPath,
		Version:    version,
	})
	if err != nil {
		log.Fatalf("%v", err)
	}

	httpSrv := &http.Server{
		Addr:    listen,
		Handler: a.Handler(),
	}

	// Graceful shutdown on signal. ListenAndServe returns as soon as
	// Shutdown is *called*, so main must block on done until teardown
	// (connection drain + app.Shutdown) has actually finished.
	done := make(chan struct{})
	go func() {
		defer close(done)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		_ = httpSrv.Shutdown(shutdownCtx)

		appShutdownCtx, appShutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer appShutdownCancel()
		_ = a.Shutdown(appShutdownCtx)
	}()

	log.Printf("nymeria %s listening on %s", version, listen)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	<-done
	log.Println("stopped")
}
