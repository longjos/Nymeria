package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/transport/kisstcp"
	"github.com/narvel/nymeria/internal/transport/serial"
)

// helper to create a test server with session + config manager
func newTestSettingsServer(t *testing.T) (*Server, *session.MemoryManager, *config.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nymeria.yaml")

	cfg := config.DefaultConfig()
	cfg.Station.Callsign = "W1AW"
	cfg.Transports = []transport.TransportConfig{
		{Type: "aprsis", Host: "rotate.aprs2.net", Port: 14580, Passcode: "12345"},
	}
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	cfgMgr := config.NewManager(cfgPath, cfg)
	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{PIN: "1234"})
	tracker := station.NewMemoryTracker(cfg.Station)
	tm := transport.NewManager()

	srv := New(tracker, tm, message.Engine(nil), nil,
		WithSessionManager(sessMgr),
		WithConfigManager(cfgMgr),
	)

	return srv, sessMgr, cfgMgr, cfgPath
}

func adminToken(sessMgr *session.MemoryManager) string {
	user, _ := sessMgr.Create("admin", session.CreateOpts{})
	// First user auto-becomes admin; UpdateRole is harmless redundancy
	sessMgr.UpdateRole(user.ID, session.RoleAdmin)
	return user.Token
}

func operatorToken(sessMgr *session.MemoryManager) string {
	user, _ := sessMgr.Create("operator", session.CreateOpts{})
	// Subsequent users are pending — approve as operator
	sessMgr.Approve(user.ID, session.RoleOperator)
	return user.Token
}

func doRequest(srv *Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestSettingsRequiresAdmin(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)

	// No auth → 401
	w := doRequest(srv, "GET", "/api/settings", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth: got %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Admin → 200 (create admin first so the auto-promote slot is consumed)
	adToken := adminToken(sessMgr)
	w = doRequest(srv, "GET", "/api/settings", nil, adToken)
	if w.Code != http.StatusOK {
		t.Errorf("admin: got %d, want %d", w.Code, http.StatusOK)
	}

	// Operator → 403 (admin slot already consumed, so this user stays Operator)
	opToken := operatorToken(sessMgr)
	w = doRequest(srv, "GET", "/api/settings", nil, opToken)
	if w.Code != http.StatusForbidden {
		t.Errorf("operator: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestGetSettingsRedactsPasscodes(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/settings", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("GET settings: %d", w.Code)
	}

	var resp settingsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Transports) != 1 {
		t.Fatalf("expected 1 transport, got %d", len(resp.Transports))
	}
	if resp.Transports[0].Passcode != "***" {
		t.Errorf("passcode should be redacted to '***', got %q", resp.Transports[0].Passcode)
	}
	if resp.Station.Callsign != "W1AW" {
		t.Errorf("callsign = %q, want %q", resp.Station.Callsign, "W1AW")
	}
	if resp.Station.MessagePath != "WIDE1-1,WIDE2-1" {
		t.Errorf("messagePath = %q, want WIDE1-1,WIDE2-1", resp.Station.MessagePath)
	}
	if resp.Station.BeaconPath != "WIDE1-1,WIDE2-1" {
		t.Errorf("beaconPath = %q, want WIDE1-1,WIDE2-1", resp.Station.BeaconPath)
	}
}

func TestUpdateStation(t *testing.T) {
	srv, sessMgr, cfgMgr, cfgPath := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	dto := stationDTO{
		Callsign:       "K1ABC",
		SSID:           5,
		Lat:            42.36,
		Lon:            -71.06,
		TrackMaxPoints: 200,
		StaleTimeout:   "2h0m0s",
		DedupWindow:    "1m0s",
		MessagePath:    "WIDE1-1,WIDE2-1",
		BeaconPath:     "WIDE1-1,WIDE2-1",
	}

	w := doRequest(srv, "PUT", "/api/settings/station", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT station: %d %s", w.Code, w.Body.String())
	}

	var resp updateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.RestartRequired {
		t.Error("station update should not require restart (hot-reloaded)")
	}

	// Verify in-memory
	cfg := cfgMgr.Get()
	if cfg.Station.Callsign != "K1ABC" {
		t.Errorf("callsign = %q, want %q", cfg.Station.Callsign, "K1ABC")
	}
	if cfg.Station.SSID != 5 {
		t.Errorf("SSID = %d, want %d", cfg.Station.SSID, 5)
	}

	// Verify on disk
	data, _ := os.ReadFile(cfgPath)
	var fromDisk config.Config
	yaml.Unmarshal(data, &fromDisk)
	if fromDisk.Station.Callsign != "K1ABC" {
		t.Errorf("disk callsign = %q, want %q", fromDisk.Station.Callsign, "K1ABC")
	}
}

func TestUpdateStationPaths(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	cfg := cfgMgr.Get()
	dto := stationDTO{
		Callsign:       cfg.Station.Callsign,
		SSID:           cfg.Station.SSID,
		Lat:            cfg.Station.Lat,
		Lon:            cfg.Station.Lon,
		TrackMaxPoints: cfg.Station.TrackMaxPoints,
		StaleTimeout:   cfg.Station.StaleTimeout.String(),
		DedupWindow:    cfg.Station.DedupWindow.String(),
		MessagePath:    "WIDE1-1",
		BeaconPath:     "TCPIP*",
	}

	w := doRequest(srv, "PUT", "/api/settings/station", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT station paths: %d %s", w.Code, w.Body.String())
	}

	got := cfgMgr.Get()
	if got.Station.MessagePath != "WIDE1-1" {
		t.Errorf("messagePath = %q, want WIDE1-1", got.Station.MessagePath)
	}
	if got.Station.BeaconPath != "TCPIP*" {
		t.Errorf("beaconPath = %q, want TCPIP*", got.Station.BeaconPath)
	}

	w = doRequest(srv, "GET", "/api/settings", nil, token)
	var resp settingsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Station.MessagePath != "WIDE1-1" {
		t.Errorf("GET messagePath = %q", resp.Station.MessagePath)
	}
	if resp.Station.BeaconPath != "TCPIP*" {
		t.Errorf("GET beaconPath = %q", resp.Station.BeaconPath)
	}

	w = doRequest(srv, "GET", "/api/config", nil, "")
	var pub map[string]any
	json.Unmarshal(w.Body.Bytes(), &pub)
	if pub["messagePath"] != "WIDE1-1" {
		t.Errorf("public config messagePath = %v, want WIDE1-1", pub["messagePath"])
	}
	if pub["beaconPath"] != "TCPIP*" {
		t.Errorf("public config beaconPath = %v, want TCPIP*", pub["beaconPath"])
	}
}

func TestUpdateStationRejectsInvalidPath(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	cfg := cfgMgr.Get()
	dto := stationDTO{
		Callsign:       cfg.Station.Callsign,
		SSID:           cfg.Station.SSID,
		TrackMaxPoints: cfg.Station.TrackMaxPoints,
		StaleTimeout:   cfg.Station.StaleTimeout.String(),
		DedupWindow:    cfg.Station.DedupWindow.String(),
		MessagePath:    "TOOLONG-1",
		BeaconPath:     cfg.Station.BeaconPath,
	}

	w := doRequest(srv, "PUT", "/api/settings/station", dto, token)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid path: got %d, want %d (%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if cfgMgr.Get().Station.MessagePath != cfg.Station.MessagePath {
		t.Error("config should be unchanged after rejected path")
	}
}

func TestUpdateStationValidationRejection(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	dto := stationDTO{
		Callsign: "", // invalid
		SSID:     0,
	}

	w := doRequest(srv, "PUT", "/api/settings/station", dto, token)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid station: got %d, want %d", w.Code, http.StatusBadRequest)
	}

	// Config unchanged
	if cfgMgr.Get().Station.Callsign != "W1AW" {
		t.Error("config should be unchanged after rejected update")
	}
}

func TestUpdateBeaconIsLive(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	dto := beaconDTO{
		Enabled:  true,
		Interval: "5m0s",
		Comment:  "test beacon",
	}

	w := doRequest(srv, "PUT", "/api/settings/beacon", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT beacon: %d %s", w.Code, w.Body.String())
	}

	var resp updateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.RestartRequired {
		t.Error("beacon update should NOT require restart")
	}
}

func TestUpdateTransportsPreservesPasscode(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	// Send transports with masked passcode
	dtos := []transportDTO{
		{Type: "aprsis", Host: "rotate.aprs2.net", Port: 14580, Passcode: "***"},
	}

	w := doRequest(srv, "PUT", "/api/settings/transports", dtos, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT transports: %d %s", w.Code, w.Body.String())
	}

	// Passcode should be preserved from original
	cfg := cfgMgr.Get()
	if len(cfg.Transports) != 1 {
		t.Fatalf("expected 1 transport, got %d", len(cfg.Transports))
	}
	if cfg.Transports[0].Passcode != "12345" {
		t.Errorf("passcode = %q, want %q", cfg.Transports[0].Passcode, "12345")
	}
}

func TestUpdateSessionPreservesExistingPIN(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	// First set a PIN via direct config
	cfg := cfgMgr.Get()
	cfg.Session.PIN = "secretpin"
	cfgMgr.Update(cfg)

	// Update with masked PIN
	dto := sessionDTO{
		PIN:               "***",
		InactivityTimeout: "45m0s",
	}

	w := doRequest(srv, "PUT", "/api/settings/session", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT session: %d %s", w.Code, w.Body.String())
	}

	// PIN should be preserved
	got := cfgMgr.Get()
	if got.Session.PIN != "secretpin" {
		t.Errorf("PIN = %q, want %q", got.Session.PIN, "secretpin")
	}
	if got.Session.InactivityTimeout != 45*time.Minute {
		t.Errorf("timeout = %v, want %v", got.Session.InactivityTimeout, 45*time.Minute)
	}
}

func TestUpdateServerRequiresRestart(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	dto := serverDTO{Listen: ":9999"}
	w := doRequest(srv, "PUT", "/api/settings/server", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT server: %d %s", w.Code, w.Body.String())
	}

	var resp updateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.RestartRequired {
		t.Error("server.listen update should require restart")
	}
}

func TestUpdateLogging(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	dto := loggingDTO{Level: "debug"}
	w := doRequest(srv, "PUT", "/api/settings/logging", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT logging: %d %s", w.Code, w.Body.String())
	}

	if cfgMgr.Get().Logging.Level != "debug" {
		t.Errorf("level = %q, want %q", cfgMgr.Get().Logging.Level, "debug")
	}
}

func TestListSerialPortsRequiresAdmin(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	restore := serial.SetListerForTest(func() ([]serial.PortInfo, error) {
		return []serial.PortInfo{}, nil
	})
	t.Cleanup(restore)

	w := doRequest(srv, "GET", "/api/serial-ports", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth: got %d, want %d", w.Code, http.StatusUnauthorized)
	}

	admin := adminToken(sessMgr)
	w = doRequest(srv, "GET", "/api/serial-ports", nil, admin)
	if w.Code != http.StatusOK {
		t.Errorf("admin: got %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	op := operatorToken(sessMgr)
	w = doRequest(srv, "GET", "/api/serial-ports", nil, op)
	if w.Code != http.StatusForbidden {
		t.Errorf("operator: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestListSerialPortsEmpty(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	restore := serial.SetListerForTest(func() ([]serial.PortInfo, error) {
		return []serial.PortInfo{}, nil
	})
	t.Cleanup(restore)

	w := doRequest(srv, "GET", "/api/serial-ports", nil, adminToken(sessMgr))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}

	var resp serialPortsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.HostOS == "" {
		t.Error("hostOS empty")
	}
	if resp.Ports == nil {
		t.Fatal("ports is null, want []")
	}
	if len(resp.Ports) != 0 {
		t.Errorf("ports len = %d, want 0", len(resp.Ports))
	}
	if len(resp.Profiles) == 0 {
		t.Error("profiles empty")
	}
	if len(resp.BaudRates) == 0 {
		t.Error("baudRates empty")
	}
	if resp.Error != "" {
		t.Errorf("error = %q, want empty", resp.Error)
	}

	// camelCase on the wire
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"hostOS", "ports", "profiles", "baudRates"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestListSerialPortsKenwoodHighlight(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	restore := serial.SetListerForTest(func() ([]serial.PortInfo, error) {
		return []serial.PortInfo{{
			Name:             "COM5",
			Label:            "TH-D74 (COM5)",
			Present:          true,
			IsUSB:            true,
			SuggestedProfile: "kenwood-thd7x-usb",
			Highlight:        true,
		}}, nil
	})
	t.Cleanup(restore)

	w := doRequest(srv, "GET", "/api/serial-ports", nil, adminToken(sessMgr))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var resp serialPortsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Ports) != 1 {
		t.Fatalf("ports = %d", len(resp.Ports))
	}
	if !resp.Ports[0].Highlight || resp.Ports[0].SuggestedProfile != "kenwood-thd7x-usb" {
		t.Errorf("port = %+v", resp.Ports[0])
	}

	var raw map[string]any
	json.Unmarshal(w.Body.Bytes(), &raw)
	ports, _ := raw["ports"].([]any)
	first, _ := ports[0].(map[string]any)
	if _, ok := first["isUSB"]; !ok {
		t.Error("missing isUSB")
	}
	if _, ok := first["suggestedProfile"]; !ok {
		t.Error("missing suggestedProfile")
	}
}

func TestListSerialPortsListerError(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	restore := serial.SetListerForTest(func() ([]serial.PortInfo, error) {
		return nil, errors.New("setupapi failed")
	})
	t.Cleanup(restore)

	w := doRequest(srv, "GET", "/api/serial-ports", nil, adminToken(sessMgr))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d (want 200) %s", w.Code, w.Body.String())
	}
	var resp serialPortsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" {
		t.Error("error empty")
	}
	if resp.Ports == nil {
		t.Fatal("ports is null")
	}
	if len(resp.Ports) != 0 {
		t.Errorf("ports len = %d", len(resp.Ports))
	}
	if len(resp.Profiles) == 0 || len(resp.BaudRates) == 0 {
		t.Error("catalogs missing on error")
	}
}

func TestListKissTNCsRequiresAdmin(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	restore := kisstcp.SetDiscoverForTest(func() ([]kisstcp.TNCInfo, error) {
		return []kisstcp.TNCInfo{}, nil
	})
	t.Cleanup(restore)

	w := doRequest(srv, "GET", "/api/kiss-tncs", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth: got %d", w.Code)
	}
	w = doRequest(srv, "GET", "/api/kiss-tncs", nil, adminToken(sessMgr))
	if w.Code != http.StatusOK {
		t.Errorf("admin: got %d %s", w.Code, w.Body.String())
	}
	w = doRequest(srv, "GET", "/api/kiss-tncs", nil, operatorToken(sessMgr))
	if w.Code != http.StatusForbidden {
		t.Errorf("operator: got %d", w.Code)
	}
}

func TestListKissTNCsEmpty(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	restore := kisstcp.SetDiscoverForTest(func() ([]kisstcp.TNCInfo, error) {
		return []kisstcp.TNCInfo{}, nil
	})
	t.Cleanup(restore)

	w := doRequest(srv, "GET", "/api/kiss-tncs", nil, adminToken(sessMgr))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var resp kissTNCsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.TNCs == nil {
		t.Fatal("tncs is null")
	}
	if len(resp.TNCs) != 0 {
		t.Errorf("len = %d", len(resp.TNCs))
	}
	var raw map[string]any
	json.Unmarshal(w.Body.Bytes(), &raw)
	if _, ok := raw["hostOS"]; !ok {
		t.Error("missing hostOS")
	}
	if _, ok := raw["tncs"]; !ok {
		t.Error("missing tncs")
	}
}

func TestListKissTNCsDirewolf(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	restore := kisstcp.SetDiscoverForTest(func() ([]kisstcp.TNCInfo, error) {
		return []kisstcp.TNCInfo{{
			Name:      "Dire Wolf on radiopi",
			Label:     "Dire Wolf on radiopi — 192.168.1.40:8001",
			Host:      "192.168.1.40",
			Port:      8001,
			Source:    "mdns",
			Highlight: true,
		}}, nil
	})
	t.Cleanup(restore)

	w := doRequest(srv, "GET", "/api/kiss-tncs", nil, adminToken(sessMgr))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var resp kissTNCsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.TNCs) != 1 || resp.TNCs[0].Host != "192.168.1.40" || !resp.TNCs[0].Highlight {
		t.Fatalf("%+v", resp.TNCs)
	}
}

func TestListKissTNCsDiscoverError(t *testing.T) {
	srv, sessMgr, _, _ := newTestSettingsServer(t)
	restore := kisstcp.SetDiscoverForTest(func() ([]kisstcp.TNCInfo, error) {
		return []kisstcp.TNCInfo{{Name: "This computer", Host: "localhost", Port: 8001, Local: true}}, errors.New("no multicast")
	})
	t.Cleanup(restore)

	w := doRequest(srv, "GET", "/api/kiss-tncs", nil, adminToken(sessMgr))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var resp kissTNCsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("error empty")
	}
	if len(resp.TNCs) != 1 {
		t.Fatalf("want local fallback, got %+v", resp.TNCs)
	}
}

func TestUpdateWeather(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	dto := weatherDTO{RetentionDays: 14}
	w := doRequest(srv, "PUT", "/api/settings/weather", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT weather: %d %s", w.Code, w.Body.String())
	}

	if cfgMgr.Get().Weather.RetentionDays != 14 {
		t.Errorf("retentionDays = %d, want %d", cfgMgr.Get().Weather.RetentionDays, 14)
	}
}

func TestUpdateWeatherUnits(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	// Default should be metric
	cfg := cfgMgr.Get()
	if cfg.Weather.Units != "metric" {
		t.Errorf("default units = %q, want %q", cfg.Weather.Units, "metric")
	}

	// Set to imperial
	dto := weatherDTO{RetentionDays: 7, Units: "imperial"}
	w := doRequest(srv, "PUT", "/api/settings/weather", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT weather imperial: %d %s", w.Code, w.Body.String())
	}

	got := cfgMgr.Get()
	if got.Weather.Units != "imperial" {
		t.Errorf("units = %q, want %q", got.Weather.Units, "imperial")
	}

	// Verify GET settings returns imperial
	w = doRequest(srv, "GET", "/api/settings", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("GET settings: %d", w.Code)
	}
	var resp settingsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Weather.Units != "imperial" {
		t.Errorf("GET settings weather.units = %q, want %q", resp.Weather.Units, "imperial")
	}

	// Invalid units should default to metric
	dto = weatherDTO{RetentionDays: 7, Units: "invalid"}
	w = doRequest(srv, "PUT", "/api/settings/weather", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT weather invalid: %d %s", w.Code, w.Body.String())
	}
	got = cfgMgr.Get()
	if got.Weather.Units != "metric" {
		t.Errorf("invalid units should default to metric, got %q", got.Weather.Units)
	}
}

func TestUpdateTileCache(t *testing.T) {
	srv, sessMgr, cfgMgr, _ := newTestSettingsServer(t)
	token := adminToken(sessMgr)

	dto := tileCacheDTO{Enabled: true, DataDir: "/tmp/tiles", MaxZoom: 18}
	w := doRequest(srv, "PUT", "/api/settings/tilecache", dto, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT tilecache: %d %s", w.Code, w.Body.String())
	}

	var resp updateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.RestartRequired {
		t.Error("tilecache update should require restart")
	}

	cfg := cfgMgr.Get()
	if cfg.TileCache.MaxZoom != 18 {
		t.Errorf("maxZoom = %d, want %d", cfg.TileCache.MaxZoom, 18)
	}
}

// Suppress unused import warning — context is used in session.Start but our test doesn't call it.
var _ = context.Background
