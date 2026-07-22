package app

import (
	"context"
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
