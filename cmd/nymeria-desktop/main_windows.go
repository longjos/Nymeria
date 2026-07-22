//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/app"
	"github.com/narvel/nymeria/internal/config"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var version = "dev"

func main() {
	configFlag := flag.String("config", "", "path to config file")
	listenFlag := flag.String("listen", "", "override listen address (e.g. :9090)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("nymeria-desktop", version)
		os.Exit(0)
	}

	dir, err := config.EnsureUserDataDir()
	if err != nil {
		log.Fatalf("failed to ensure user data dir: %v", err)
	}

	cfgPath := *configFlag
	if cfgPath == "" {
		cfgPath = config.DefaultUserConfigPath(dir)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("failed to load config: %v", err)
		}
		log.Println("no config file found, using defaults")
		cfg = config.DefaultConfig()
	}

	cfg = config.ResolveUserPaths(cfg, dir)

	if *listenFlag != "" {
		cfg.Server.Listen = *listenFlag
	}

	addr, err := app.DesktopAddr(cfg.Server.Listen)
	if err != nil {
		log.Fatalf("invalid listen address: %v", err)
	}

	ln, err := app.ListenWithFallback(addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	a, err := app.New(app.Options{
		Config:     cfg,
		ConfigPath: cfgPath,
		Version:    version,
	})
	if err != nil {
		log.Fatalf("%v", err)
	}

	httpSrv := &http.Server{Handler: a.Handler()}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	url := app.LocalURL(ln.Addr().String())
	log.Printf("nymeria %s (desktop) serving %s", version, url)

	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			log.Println("shutting down...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpSrv.Shutdown(ctx)
			_ = a.Shutdown(ctx)
		})
	}

	var win *application.WebviewWindow

	wapp := application.New(application.Options{
		Name: "Nymeria",
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.nymeria.desktop",
			OnSecondInstanceLaunch: func(_ application.SecondInstanceData) {
				if win != nil {
					win.Show()
					win.Focus()
				}
			},
		},
		OnShutdown: shutdown,
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	win = wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Nymeria",
		URL:    url,
		Width:  1280,
		Height: 800,
	})

	// Closing the window hides to tray; the HTTP server keeps serving.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		win.Hide()
		e.Cancel()
	})

	tray := wapp.SystemTray.New()
	trayMenu := application.NewMenu()
	trayMenu.Add("Show Nymeria").OnClick(func(*application.Context) {
		win.Show()
		win.Focus()
	})
	trayMenu.Add("Quit").OnClick(func(*application.Context) {
		wapp.Quit()
	})
	tray.SetMenu(trayMenu)
	tray.AttachWindow(win)

	if err := wapp.Run(); err != nil {
		log.Fatalf("desktop app error: %v", err)
	}
	// Defensive: ensure teardown even if the shutdown hook didn't fire.
	shutdown()
}
