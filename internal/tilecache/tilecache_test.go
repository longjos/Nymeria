package tilecache

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTilesForBounds(t *testing.T) {
	// Small area around Kansas City at zoom 10
	tiles := TilesForBounds(38.5, -95.0, 39.5, -94.0, 10, 10)
	if len(tiles) == 0 {
		t.Fatal("expected at least one tile for valid bounds")
	}

	// All tiles should have z=10
	for _, tc := range tiles {
		if tc.Z != 10 {
			t.Errorf("expected z=10, got z=%d", tc.Z)
		}
	}
}

func TestTilesForBounds_MultiZoom(t *testing.T) {
	tiles := TilesForBounds(38.5, -95.0, 39.5, -94.0, 8, 10)
	if len(tiles) == 0 {
		t.Fatal("expected tiles across multiple zoom levels")
	}

	zoomLevels := make(map[int]bool)
	for _, tc := range tiles {
		zoomLevels[tc.Z] = true
	}

	for z := 8; z <= 10; z++ {
		if !zoomLevels[z] {
			t.Errorf("missing tiles for zoom level %d", z)
		}
	}
}

func TestEstimateTileCount(t *testing.T) {
	tiles := TilesForBounds(38.5, -95.0, 39.5, -94.0, 10, 12)
	estimate := EstimateTileCount(38.5, -95.0, 39.5, -94.0, 10, 12)

	if estimate != len(tiles) {
		t.Errorf("EstimateTileCount=%d, len(TilesForBounds)=%d", estimate, len(tiles))
	}
}

func TestTilePath(t *testing.T) {
	c := &Cache{dataDir: "/tmp/tiles"}
	path := c.tilePath(10, 100, 200)
	expected := filepath.Join("/tmp/tiles", "10", "100", "200.png")
	if path != expected {
		t.Errorf("tilePath = %q, want %q", path, expected)
	}
}

func TestGet_CacheHit(t *testing.T) {
	dir := t.TempDir()
	c, err := New(Config{DataDir: dir, MaxZoom: 19})
	if err != nil {
		t.Fatal(err)
	}

	// Seed a tile file
	tilePath := c.tilePath(10, 100, 200)
	os.MkdirAll(filepath.Dir(tilePath), 0o755)
	expected := []byte("fake-tile-data")
	os.WriteFile(tilePath, expected, 0o644)

	data, err := c.Get(context.Background(), 10, 100, 200)
	if err != nil {
		t.Fatalf("Get cache hit error: %v", err)
	}
	if string(data) != string(expected) {
		t.Errorf("Get returned %q, want %q", data, expected)
	}
}

func TestGet_CacheMiss(t *testing.T) {
	dir := t.TempDir()

	// Set up a test upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("upstream-tile-png"))
	}))
	defer upstream.Close()

	c, err := New(Config{
		DataDir: dir,
		TileURL: upstream.URL + "/{z}/{x}/{y}.png",
		MaxZoom: 19,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := c.Get(context.Background(), 5, 10, 15)
	if err != nil {
		t.Fatalf("Get cache miss error: %v", err)
	}
	if string(data) != "upstream-tile-png" {
		t.Errorf("Get returned %q, want 'upstream-tile-png'", data)
	}

	// Verify tile was saved to disk
	saved, err := os.ReadFile(c.tilePath(5, 10, 15))
	if err != nil {
		t.Fatalf("tile not saved to disk: %v", err)
	}
	if string(saved) != "upstream-tile-png" {
		t.Errorf("saved tile = %q, want 'upstream-tile-png'", saved)
	}
}

func TestHas(t *testing.T) {
	dir := t.TempDir()
	c, err := New(Config{DataDir: dir, MaxZoom: 19})
	if err != nil {
		t.Fatal(err)
	}

	if c.Has(10, 100, 200) {
		t.Error("Has returned true for non-existent tile")
	}

	// Create the tile
	tilePath := c.tilePath(10, 100, 200)
	os.MkdirAll(filepath.Dir(tilePath), 0o755)
	os.WriteFile(tilePath, []byte("data"), 0o644)

	if !c.Has(10, 100, 200) {
		t.Error("Has returned false for existing tile")
	}
}

func TestStatus(t *testing.T) {
	dir := t.TempDir()
	c, err := New(Config{DataDir: dir, MaxZoom: 19})
	if err != nil {
		t.Fatal(err)
	}

	status, err := c.Status()
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.TileCount != 0 {
		t.Errorf("empty cache TileCount = %d, want 0", status.TileCount)
	}

	// Add some tiles
	for i := 0; i < 3; i++ {
		path := c.tilePath(10, i, 0)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(fmt.Sprintf("tile-%d", i)), 0o644)
	}

	status, err = c.Status()
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.TileCount != 3 {
		t.Errorf("TileCount = %d, want 3", status.TileCount)
	}
	if status.DiskUsage == 0 {
		t.Error("DiskUsage should be > 0")
	}
}

func TestPreload(t *testing.T) {
	dir := t.TempDir()

	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("tile"))
	}))
	defer upstream.Close()

	c, err := New(Config{
		DataDir:   dir,
		TileURL:   upstream.URL + "/{z}/{x}/{y}.png",
		RateLimit: 0, // No rate limit for tests
		MaxZoom:   19,
	})
	if err != nil {
		t.Fatal(err)
	}

	tiles := []TileCoord{
		{Z: 5, X: 10, Y: 15},
		{Z: 5, X: 11, Y: 15},
		{Z: 5, X: 10, Y: 16},
	}

	c.Preload(context.Background(), tiles)

	if callCount != 3 {
		t.Errorf("expected 3 upstream fetches, got %d", callCount)
	}

	// All tiles should now be cached
	for _, tc := range tiles {
		if !c.Has(tc.Z, tc.X, tc.Y) {
			t.Errorf("tile %d/%d/%d not cached after preload", tc.Z, tc.X, tc.Y)
		}
	}
}

func TestPreload_SkipsExisting(t *testing.T) {
	dir := t.TempDir()

	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("tile"))
	}))
	defer upstream.Close()

	c, err := New(Config{
		DataDir:   dir,
		TileURL:   upstream.URL + "/{z}/{x}/{y}.png",
		RateLimit: 0,
		MaxZoom:   19,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Pre-seed one tile
	path := c.tilePath(5, 10, 15)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("existing"), 0o644)

	tiles := []TileCoord{
		{Z: 5, X: 10, Y: 15}, // exists
		{Z: 5, X: 11, Y: 15}, // new
	}

	c.Preload(context.Background(), tiles)

	if callCount != 1 {
		t.Errorf("expected 1 upstream fetch (skip existing), got %d", callCount)
	}
}
