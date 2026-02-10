package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/transport/aprsis"
)

// newTestSetupServer creates a server with default (N0CALL) config for setup testing.
func newTestSetupServer(t *testing.T) (*Server, *config.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nymeria.yaml")

	cfg := config.DefaultConfig()
	// N0CALL is the default — setup should be allowed.
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	cfgMgr := config.NewManager(cfgPath, cfg)
	tracker := station.NewMemoryTracker(cfg.Station)
	tm := transport.NewManager()

	srv := New(tracker, tm, message.Engine(nil), nil,
		WithConfigManager(cfgMgr),
	)

	return srv, cfgMgr, cfgPath
}

func TestSetupSucceeds(t *testing.T) {
	srv, cfgMgr, cfgPath := newTestSetupServer(t)

	body := map[string]any{
		"callsign":     "KD7BBC",
		"ssid":         0,
		"comment":      "Nymeria APRS",
		"lat":          39.83,
		"lon":          -98.58,
		"aprisEnabled": true,
		"aprisHost":    "rotate.aprs2.net",
		"aprisPort":    14580,
		"aprisFilter":  "r/39.83/-98.58/200",
	}

	w := doRequest(srv, "POST", "/api/setup", body, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	if resp["restartRequired"] != false {
		t.Errorf("expected restartRequired false, got %v", resp["restartRequired"])
	}

	// Verify config was updated
	cfg := cfgMgr.Get()
	if cfg.Station.Callsign != "KD7BBC" {
		t.Errorf("callsign = %q, want %q", cfg.Station.Callsign, "KD7BBC")
	}
	if cfg.Station.Lat != 39.83 {
		t.Errorf("lat = %f, want %f", cfg.Station.Lat, 39.83)
	}
	if cfg.Station.Lon != -98.58 {
		t.Errorf("lon = %f, want %f", cfg.Station.Lon, -98.58)
	}
	if cfg.Station.Comment != "Nymeria APRS" {
		t.Errorf("comment = %q, want %q", cfg.Station.Comment, "Nymeria APRS")
	}

	// Should have one transport (APRS-IS)
	if len(cfg.Transports) != 1 {
		t.Fatalf("expected 1 transport, got %d", len(cfg.Transports))
	}
	tr := cfg.Transports[0]
	if tr.Type != "aprsis" {
		t.Errorf("transport type = %q, want aprsis", tr.Type)
	}
	if tr.Host != "rotate.aprs2.net" {
		t.Errorf("host = %q, want %q", tr.Host, "rotate.aprs2.net")
	}
	if tr.Port != 14580 {
		t.Errorf("port = %d, want %d", tr.Port, 14580)
	}
	if tr.Filter != "r/39.83/-98.58/200" {
		t.Errorf("filter = %q, want %q", tr.Filter, "r/39.83/-98.58/200")
	}

	// Passcode should be auto-computed
	expected := fmt.Sprintf("%d", aprsis.Passcode("KD7BBC"))
	if tr.Passcode != expected {
		t.Errorf("passcode = %q, want %q", tr.Passcode, expected)
	}

	// Verify YAML file was written
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if len(raw) == 0 {
		t.Error("config file is empty")
	}
}

func TestSetupWithoutAPRIS(t *testing.T) {
	srv, cfgMgr, _ := newTestSetupServer(t)

	body := map[string]any{
		"callsign":     "W1AW",
		"ssid":         5,
		"comment":      "Testing",
		"lat":          41.71,
		"lon":          -72.72,
		"aprisEnabled": false,
	}

	w := doRequest(srv, "POST", "/api/setup", body, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	cfg := cfgMgr.Get()
	if cfg.Station.Callsign != "W1AW" {
		t.Errorf("callsign = %q, want %q", cfg.Station.Callsign, "W1AW")
	}
	if cfg.Station.SSID != 5 {
		t.Errorf("ssid = %d, want %d", cfg.Station.SSID, 5)
	}
	if len(cfg.Transports) != 0 {
		t.Errorf("expected 0 transports, got %d", len(cfg.Transports))
	}
}

func TestSetup403WhenAlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nymeria.yaml")

	cfg := config.DefaultConfig()
	cfg.Station.Callsign = "W1AW" // Already configured
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	cfgMgr := config.NewManager(cfgPath, cfg)
	tracker := station.NewMemoryTracker(cfg.Station)
	tm := transport.NewManager()
	srv := New(tracker, tm, message.Engine(nil), nil,
		WithConfigManager(cfgMgr),
	)

	body := map[string]any{
		"callsign": "KD7BBC",
		"lat":      39.83,
		"lon":      -98.58,
	}

	w := doRequest(srv, "POST", "/api/setup", body, "")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSetup400InvalidCallsign(t *testing.T) {
	srv, _, _ := newTestSetupServer(t)

	tests := []struct {
		name     string
		callsign string
	}{
		{"empty", ""},
		{"too short", "A"},
		{"invalid chars", "@@BAD!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]any{
				"callsign": tt.callsign,
				"lat":      39.83,
				"lon":      -98.58,
			}
			w := doRequest(srv, "POST", "/api/setup", body, "")
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSetup400InvalidLatLon(t *testing.T) {
	srv, _, _ := newTestSetupServer(t)

	tests := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{"lat too high", 91.0, -98.58},
		{"lat too low", -91.0, -98.58},
		{"lon too high", 39.83, 181.0},
		{"lon too low", 39.83, -181.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]any{
				"callsign": "KD7BBC",
				"lat":      tt.lat,
				"lon":      tt.lon,
			}
			w := doRequest(srv, "POST", "/api/setup", body, "")
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSetupCallsignUppercased(t *testing.T) {
	srv, cfgMgr, _ := newTestSetupServer(t)

	body := map[string]any{
		"callsign": "kd7bbc",
		"lat":      39.83,
		"lon":      -98.58,
	}

	w := doRequest(srv, "POST", "/api/setup", body, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	cfg := cfgMgr.Get()
	if cfg.Station.Callsign != "KD7BBC" {
		t.Errorf("callsign = %q, want %q", cfg.Station.Callsign, "KD7BBC")
	}
}

func TestGetConfigNeedsSetup(t *testing.T) {
	srv, _, _ := newTestSetupServer(t)

	w := doRequest(srv, "GET", "/api/config", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["needsSetup"] != true {
		t.Errorf("expected needsSetup true, got %v", resp["needsSetup"])
	}
}

func TestGetConfigNoSetupNeeded(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nymeria.yaml")

	cfg := config.DefaultConfig()
	cfg.Station.Callsign = "W1AW"
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	cfgMgr := config.NewManager(cfgPath, cfg)
	tracker := station.NewMemoryTracker(cfg.Station)
	tm := transport.NewManager()
	srv := New(tracker, tm, message.Engine(nil), nil,
		WithConfigManager(cfgMgr),
	)

	w := doRequest(srv, "GET", "/api/config", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["needsSetup"] != false {
		t.Errorf("expected needsSetup false, got %v", resp["needsSetup"])
	}
}

func TestSetupNoConfigManager(t *testing.T) {
	cfg := config.DefaultConfig()
	tracker := station.NewMemoryTracker(cfg.Station)
	tm := transport.NewManager()
	srv := New(tracker, tm, message.Engine(nil), nil)

	body := map[string]any{
		"callsign": "KD7BBC",
		"lat":      39.83,
		"lon":      -98.58,
	}

	w := doRequest(srv, "POST", "/api/setup", body, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}
