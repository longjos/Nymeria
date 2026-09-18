package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/store"
)

// loginToken logs a fresh named user in through the real HTTP path (no PIN
// configured, so the first caller is auto-approved) and returns its bearer
// token, for tests that need to pass a role-gated route's RequireRole check.
func loginToken(t *testing.T, h http.Handler, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/session", bytes.NewReader(body))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login: status %d, body %s", rr.Code, rr.Body.String())
	}
	var user struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &user); err != nil {
		t.Fatalf("login: unmarshal: %v", err)
	}
	if user.Token == "" {
		t.Fatal("login: empty token")
	}
	return user.Token
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Store.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.TileCache.Enabled = false
	cfg.Transports = nil
	cfg.Beacon.Enabled = false
	return cfg
}

func TestNewAndHandler(t *testing.T) {
	cfg := testConfig(t)
	a, err := New(Options{
		Config:     cfg,
		ConfigPath: filepath.Join(t.TempDir(), "nymeria.yaml"),
		Version:    "test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a == nil {
		t.Fatal("New returned nil App")
	}

	h := a.Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /api/health status = %d, want 200", rr.Code)
	}

	if got := a.Config().Station.Callsign; got != "N0CALL" {
		t.Errorf("Config().Station.Callsign = %q, want N0CALL", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestShutdownIdempotent(t *testing.T) {
	cfg := testConfig(t)
	a, err := New(Options{
		Config:     cfg,
		ConfigPath: filepath.Join(t.TempDir(), "nymeria.yaml"),
		Version:    "test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- a.Shutdown(ctx)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Shutdown: %v", err)
		}
	}
}

func TestShutdownReleasesStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	cfg := testConfig(t)
	cfg.Store.Path = dbPath

	a, err := New(Options{
		Config:     cfg,
		ConfigPath: filepath.Join(t.TempDir(), "nymeria.yaml"),
		Version:    "test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Prove the DB file was closed: a second store can open and init it.
	db2 := store.NewSQLiteStore(dbPath)
	if err := db2.Init(); err != nil {
		t.Fatalf("second store Init after Shutdown: %v", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatalf("second store Close: %v", err)
	}
}

func TestNewStoreInitError(t *testing.T) {
	// Parent of store path is a regular file so MkdirAll fails.
	parentFile := filepath.Join(t.TempDir(), "notadir")
	if err := writePlainFile(parentFile); err != nil {
		t.Fatalf("create parent file: %v", err)
	}

	cfg := testConfig(t)
	cfg.Store.Path = filepath.Join(parentFile, "sub", "db.db")

	a, err := New(Options{
		Config:     cfg,
		ConfigPath: filepath.Join(t.TempDir(), "nymeria.yaml"),
		Version:    "test",
	})
	if a != nil {
		t.Error("New returned non-nil App on store init error")
		_ = a.Shutdown(context.Background())
	}
	if err == nil {
		t.Fatal("New returned nil error, want store init failure")
	}
	if !strings.Contains(err.Error(), "failed to initialize store") {
		t.Errorf("error = %q, want substring %q", err.Error(), "failed to initialize store")
	}
}

func writePlainFile(path string) error {
	return os.WriteFile(path, []byte("not a directory"), 0o644)
}

// TestNewBootsWithWxAlertsDisabled covers the default boot path (testConfig
// inherits config.DefaultConfig's WxAlerts.Enabled == false): the feature
// answers 503 everywhere rather than being absent or crashing boot.
func TestNewBootsWithWxAlertsDisabled(t *testing.T) {
	cfg := testConfig(t)
	if cfg.WxAlerts.Enabled {
		t.Fatal("testConfig: WxAlerts.Enabled = true, want false (default)")
	}

	a, err := New(Options{Config: cfg, ConfigPath: filepath.Join(t.TempDir(), "nymeria.yaml"), Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		a.Shutdown(ctx)
	}()

	// Reading alerts with the feature off is not an error: it answers 200
	// with an "off" status naming the reason, so the UI can say "turned off"
	// rather than "can't connect". Only the endpoints that genuinely need a
	// manager (ack, relay, zone lookup) still 503.
	token := loginToken(t, a.Handler(), "OBS")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/wx/alerts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/wx/alerts with wx disabled = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	var snap struct {
		Status struct {
			State  string `json:"state"`
			Reason string `json:"reason"`
		} `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Status.State != "off" || snap.Status.Reason != "disabled" {
		t.Errorf("status = %s/%s, want off/disabled", snap.Status.State, snap.Status.Reason)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/wx/alerts/x/ack", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("POST ack with wx disabled = %d, want 503", rr.Code)
	}
}

// TestNewBootsWithWxAlertsEnabled boots with wx_alerts.enabled: true against
// an httptest upstream standing in for api.weather.gov — no real network
// reaches the manager's poller, matching the count-signature short-circuit
// poller.go implements (an empty count first, so PollOnce never even
// requests /alerts/active on the very first tick this test observes).
func TestNewBootsWithWxAlertsEnabled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		if strings.Contains(r.URL.Path, "/alerts/active/count") {
			w.Write([]byte(`{"total":0,"areas":{},"zones":{}}`))
			return
		}
		w.Write([]byte(`{"type":"FeatureCollection","features":[]}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.WxAlerts.Enabled = true
	cfg.WxAlerts.Contact = "ops@example.com"
	cfg.WxAlerts.BaseURL = upstream.URL
	cfg.WxAlerts.PollInterval = time.Minute

	a, err := New(Options{Config: cfg, ConfigPath: filepath.Join(t.TempDir(), "nymeria.yaml"), Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		a.Shutdown(ctx)
	}()

	token := loginToken(t, a.Handler(), "OBS")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/wx/event-types", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/wx/event-types with wx enabled = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	var types []struct {
		Event string `json:"event"`
		Tier  string `json:"tier"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &types); err != nil {
		t.Fatalf("unmarshal event types: %v", err)
	}
	if len(types) != 113 {
		t.Errorf("len(event types) = %d, want 113", len(types))
	}

	// GET /api/settings round-trips the wxAlerts DTO with the boot config.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/settings = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	var settings struct {
		WxAlerts struct {
			Enabled bool   `json:"enabled"`
			Contact string `json:"contact"`
		} `json:"wxAlerts"`
		Weather struct {
			Alerts map[string]any `json:"alerts,omitempty"`
		} `json:"weather"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if !settings.WxAlerts.Enabled || settings.WxAlerts.Contact != "ops@example.com" {
		t.Errorf("settings.wxAlerts = %+v", settings.WxAlerts)
	}
}
