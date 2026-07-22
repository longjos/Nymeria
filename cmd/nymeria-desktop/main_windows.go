//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/app"
	"github.com/narvel/nymeria/internal/config"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/sys/windows"
)

var version = "dev"

// fatalf logs the message and exits 1. The release exe is built with
// -H=windowsgui (no console), so it also shows a message box — otherwise
// startup failures would be completely silent.
func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	if text, err := windows.UTF16PtrFromString(msg); err == nil {
		caption, _ := windows.UTF16PtrFromString("Nymeria")
		_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR)
	}
	os.Exit(1)
}

// infoBox shows an informational message box; -H=windowsgui builds have no
// console, so this is the only way to talk to the user outside the webview.
func infoBox(msg string) {
	if text, err := windows.UTF16PtrFromString(msg); err == nil {
		caption, _ := windows.UTF16PtrFromString("Nymeria")
		_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONINFORMATION)
	}
}

// openBrowser opens url in the user's default browser.
func openBrowser(url string) {
	verb, _ := windows.UTF16PtrFromString("open")
	u, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return
	}
	if err := windows.ShellExecute(0, verb, u, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		log.Printf("failed to open browser: %v", err)
	}
}

func main() {
	configFlag := flag.String("config", "", "path to config file")
	listenFlag := flag.String("listen", "", "override listen address (e.g. :9090)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("nymeria-desktop", version)
		os.Exit(0)
	}

	// Backend handles are wired after application.New below; shutdown guards
	// against nil so it is safe from every exit path.
	var (
		win          *application.WebviewWindow
		browserURL   string
		httpSrv      *http.Server
		a            *app.App
		shutdownOnce sync.Once
	)
	shutdown := func() {
		shutdownOnce.Do(func() {
			log.Println("shutting down...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if httpSrv != nil {
				_ = httpSrv.Shutdown(ctx)
			}
			if a != nil {
				_ = a.Shutdown(ctx)
			}
		})
	}

	// application.New performs the single-instance check and hard-exits a
	// second instance. It must run BEFORE the backend starts so a double
	// launch never opens the SQLite store, logs into APRS-IS twice, or
	// truncates the first instance's log file.
	wapp := application.New(application.Options{
		Name: "Nymeria",
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.nymeria.desktop",
			OnSecondInstanceLaunch: func(_ application.SecondInstanceData) {
				if win != nil {
					win.Show()
					win.Focus()
					return
				}
				// Browser-mode fallback: no window to focus, re-open the UI.
				if browserURL != "" {
					openBrowser(browserURL)
				}
			},
		},
		OnShutdown: shutdown,
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	dir, err := config.EnsureUserDataDir()
	if err != nil {
		fatalf("failed to ensure user data dir: %v", err)
	}

	// -H=windowsgui builds have no console, so mirror all logging to a file
	// in the user data dir; without it log output goes nowhere.
	logPath := filepath.Join(dir, "nymeria.log")
	if f, lerr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); lerr == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	} else {
		log.Printf("warning: cannot open log file %s: %v", logPath, lerr)
	}

	cfgPath := *configFlag
	if cfgPath == "" {
		cfgPath = config.DefaultUserConfigPath(dir)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fatalf("failed to load config: %v", err)
		}
		// Load already returned defaults with env overrides applied.
		log.Println("no config file found, using defaults")
	}

	cfg = config.ResolveUserPaths(cfg, dir)

	// CLI override kept out of cfg on purpose: the settings API snapshots
	// cfg, and a settings save must never persist the override to the YAML.
	listen := cfg.Server.Listen
	if *listenFlag != "" {
		listen = *listenFlag
	}

	addr, err := app.DesktopAddr(listen)
	if err != nil {
		fatalf("invalid listen address: %v", err)
	}

	ln, err := app.ListenWithFallback(addr)
	if err != nil {
		fatalf("failed to listen: %v", err)
	}

	a, err = app.New(app.Options{
		Config:     cfg,
		ConfigPath: cfgPath,
		Version:    version,
	})
	if err != nil {
		fatalf("%v", err)
	}

	httpSrv = &http.Server{Handler: a.Handler()}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	url := app.LocalURL(ln.Addr().String())
	browserURL = url
	log.Printf("nymeria %s (desktop) serving %s", version, url)

	// Without the WebView2 runtime a webview window cannot exist; fall back
	// to browser mode: keep the tray (pure win32) and the local server, and
	// hand the UI to the user's default browser.
	hasWebView2 := webview2Available()
	if hasWebView2 {
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
	} else {
		log.Printf("WebView2 runtime not found; running in browser mode at %s", url)
	}

	tray := wapp.SystemTray.New()
	trayMenu := application.NewMenu()
	if hasWebView2 {
		trayMenu.Add("Show Nymeria").OnClick(func(*application.Context) {
			win.Show()
			win.Focus()
		})
	} else {
		trayMenu.Add("Open in Browser").OnClick(func(*application.Context) {
			openBrowser(url)
		})
	}
	trayMenu.Add("Quit").OnClick(func(*application.Context) {
		wapp.Quit()
	})
	tray.SetMenu(trayMenu)
	if hasWebView2 {
		tray.AttachWindow(win)
	} else {
		infoBox("The WebView2 runtime was not found, so Nymeria is running in browser mode.\n\n" +
			"The app keeps running in the system tray and serves the UI at " + url + " — " +
			"your default browser will open it now.\n\n" +
			"To get the built-in window, install the WebView2 runtime from:\n" +
			"https://developer.microsoft.com/microsoft-edge/webview2/")
		openBrowser(url)
	}

	if err := wapp.Run(); err != nil {
		shutdown()
		fatalf("desktop app error: %v", err)
	}
	// Defensive: ensure teardown even if the shutdown hook didn't fire.
	shutdown()
}
