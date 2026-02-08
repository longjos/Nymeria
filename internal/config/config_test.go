package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Listen != ":8080" {
		t.Errorf("listen = %q, want :8080", cfg.Server.Listen)
	}
	if cfg.Station.Callsign != "N0CALL" {
		t.Errorf("callsign = %q, want N0CALL", cfg.Station.Callsign)
	}
	if cfg.Station.StaleTimeout != 80*time.Minute {
		t.Errorf("stale_timeout = %v, want 80m", cfg.Station.StaleTimeout)
	}
	if cfg.Station.DedupWindow != 30*time.Second {
		t.Errorf("dedup_window = %v, want 30s", cfg.Station.DedupWindow)
	}
	if cfg.Station.TrackMaxPoints != 100 {
		t.Errorf("track_max_points = %d, want 100", cfg.Station.TrackMaxPoints)
	}
	if cfg.Store.Path != "./nymeria.db" {
		t.Errorf("store.path = %q, want ./nymeria.db", cfg.Store.Path)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "default config valid",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "empty callsign",
			modify:  func(c *Config) { c.Station.Callsign = "" },
			wantErr: true,
		},
		{
			name:    "SSID too high",
			modify:  func(c *Config) { c.Station.SSID = 16 },
			wantErr: true,
		},
		{
			name:    "SSID negative",
			modify:  func(c *Config) { c.Station.SSID = -1 },
			wantErr: true,
		},
		{
			name:    "lat out of range",
			modify:  func(c *Config) { c.Station.Lat = 91 },
			wantErr: true,
		},
		{
			name:    "lon out of range",
			modify:  func(c *Config) { c.Station.Lon = -181 },
			wantErr: true,
		},
		{
			name:    "empty listen address",
			modify:  func(c *Config) { c.Server.Listen = "" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	yaml := `
server:
  listen: ":9090"
station:
  callsign: "W3ADO"
  ssid: 5
  lat: 49.0
  lon: -72.0
store:
  path: "./test.db"
transports:
  - type: aprsis
    host: rotate.aprs2.net
    port: 14580
    filter: "r/49/-72/100"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Listen != ":9090" {
		t.Errorf("listen = %q, want :9090", cfg.Server.Listen)
	}
	if cfg.Station.Callsign != "W3ADO" {
		t.Errorf("callsign = %q, want W3ADO", cfg.Station.Callsign)
	}
	if cfg.Station.SSID != 5 {
		t.Errorf("ssid = %d, want 5", cfg.Station.SSID)
	}
	if len(cfg.Transports) != 1 {
		t.Fatalf("transports = %d, want 1", len(cfg.Transports))
	}
	if cfg.Transports[0].Host != "rotate.aprs2.net" {
		t.Errorf("host = %q, want rotate.aprs2.net", cfg.Transports[0].Host)
	}
}

func TestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	yaml := `
server:
  listen: ":8080"
station:
  callsign: "N0CALL"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("NYMERIA_LISTEN", ":7070")
	t.Setenv("NYMERIA_CALLSIGN", "w3ado")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Listen != ":7070" {
		t.Errorf("listen = %q, want :7070", cfg.Server.Listen)
	}
	if cfg.Station.Callsign != "W3ADO" {
		t.Errorf("callsign = %q, want W3ADO (uppercased)", cfg.Station.Callsign)
	}
}
