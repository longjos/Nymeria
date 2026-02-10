package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestManagerGet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Station.Callsign = "W1AW"
	mgr := NewManager("/tmp/nonexistent.yaml", cfg)

	got := mgr.Get()
	if got.Station.Callsign != "W1AW" {
		t.Errorf("Get() callsign = %q, want %q", got.Station.Callsign, "W1AW")
	}
}

func TestManagerFilePath(t *testing.T) {
	mgr := NewManager("/etc/nymeria.yaml", DefaultConfig())
	if mgr.FilePath() != "/etc/nymeria.yaml" {
		t.Errorf("FilePath() = %q, want %q", mgr.FilePath(), "/etc/nymeria.yaml")
	}
}

func TestManagerUpdateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write initial config
	cfg := DefaultConfig()
	cfg.Station.Callsign = "N0CALL"
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(path, data, 0644)

	mgr := NewManager(path, cfg)

	// Update the callsign
	updated := mgr.Get()
	updated.Station.Callsign = "W1AW"
	updated.Station.SSID = 5
	if err := mgr.Update(updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify in-memory
	got := mgr.Get()
	if got.Station.Callsign != "W1AW" {
		t.Errorf("after Update, callsign = %q, want %q", got.Station.Callsign, "W1AW")
	}
	if got.Station.SSID != 5 {
		t.Errorf("after Update, SSID = %d, want %d", got.Station.SSID, 5)
	}

	// Verify on disk — re-read from file
	fileData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	var fromDisk Config
	if err := yaml.Unmarshal(fileData, &fromDisk); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if fromDisk.Station.Callsign != "W1AW" {
		t.Errorf("disk callsign = %q, want %q", fromDisk.Station.Callsign, "W1AW")
	}
}

func TestManagerUpdateValidationRejection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Station.Callsign = "W1AW"
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(path, data, 0644)

	mgr := NewManager(path, cfg)

	// Try an invalid update (empty callsign)
	invalid := mgr.Get()
	invalid.Station.Callsign = ""
	err := mgr.Update(invalid)
	if err == nil {
		t.Fatal("Update() should reject invalid config")
	}

	// Verify original config unchanged
	got := mgr.Get()
	if got.Station.Callsign != "W1AW" {
		t.Errorf("after invalid Update, callsign = %q, want %q", got.Station.Callsign, "W1AW")
	}

	// Verify file unchanged
	fileData, _ := os.ReadFile(path)
	var fromDisk Config
	yaml.Unmarshal(fileData, &fromDisk)
	if fromDisk.Station.Callsign != "W1AW" {
		t.Errorf("disk should be unchanged, callsign = %q", fromDisk.Station.Callsign)
	}
}

func TestManagerOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Station.Callsign = "W1AW"
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(path, data, 0644)

	mgr := NewManager(path, cfg)

	var oldCall, newCall string
	mgr.OnChange(func(old, new Config) {
		oldCall = old.Station.Callsign
		newCall = new.Station.Callsign
	})

	updated := mgr.Get()
	updated.Station.Callsign = "K1ABC"
	if err := mgr.Update(updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if oldCall != "W1AW" {
		t.Errorf("OnChange old callsign = %q, want %q", oldCall, "W1AW")
	}
	if newCall != "K1ABC" {
		t.Errorf("OnChange new callsign = %q, want %q", newCall, "K1ABC")
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Station.Callsign = "W1AW"
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(path, data, 0644)

	mgr := NewManager(path, cfg)

	var wg sync.WaitGroup
	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.Get()
		}()
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := mgr.Get()
			c.Beacon.Interval = time.Duration(i+1) * time.Minute
			mgr.Update(c) // may fail on validation — that's fine
		}()
	}

	wg.Wait()
}

func TestManagerUpdateAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Station.Callsign = "W1AW"
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(path, data, 0644)

	mgr := NewManager(path, cfg)

	// Update and verify no temp file left behind
	updated := mgr.Get()
	updated.Beacon.Enabled = true
	if err := mgr.Update(updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Check no .tmp files in directory
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
