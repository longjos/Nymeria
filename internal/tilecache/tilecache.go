package tilecache

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultTileURL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"

// Config holds tile cache settings.
type Config struct {
	DataDir   string        `yaml:"data_dir"`
	TileURL   string        `yaml:"tile_url"`
	UserAgent string        `yaml:"user_agent"`
	RateLimit time.Duration `yaml:"rate_limit"`
	MaxZoom   int           `yaml:"max_zoom"`
}

// TileCoord identifies a single map tile.
type TileCoord struct {
	Z int `json:"z"`
	X int `json:"x"`
	Y int `json:"y"`
}

// CacheStatus reports cache state.
type CacheStatus struct {
	TileCount int   `json:"tileCount"`
	DiskUsage int64 `json:"diskUsage"`
}

// Event represents a tile cache event for WebSocket broadcast.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Cache manages offline tile storage.
type Cache struct {
	dataDir   string
	tileURL   string
	userAgent string
	rateLimit time.Duration
	maxZoom   int
	client    *http.Client
	mu        sync.RWMutex
	events    chan Event
}

// New creates a tile cache. Set RateLimit to 0 for no rate limiting (tests).
func New(cfg Config) (*Cache, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("tilecache: data_dir is required")
	}
	if cfg.TileURL == "" {
		cfg.TileURL = defaultTileURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Nymeria/1.0 (APRS Client)"
	}
	if cfg.RateLimit == 0 && cfg.TileURL != defaultTileURL {
		// No rate limit for custom/test URLs
	} else if cfg.RateLimit == 0 && cfg.TileURL == defaultTileURL {
		cfg.RateLimit = 500 * time.Millisecond // 2 req/sec for OSM compliance
	}
	if cfg.MaxZoom == 0 {
		cfg.MaxZoom = 16
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("tilecache: create data dir: %w", err)
	}

	return &Cache{
		dataDir:   cfg.DataDir,
		tileURL:   cfg.TileURL,
		userAgent: cfg.UserAgent,
		rateLimit: cfg.RateLimit,
		maxZoom:   cfg.MaxZoom,
		client:    &http.Client{Timeout: 15 * time.Second},
		events:    make(chan Event, 64),
	}, nil
}

// Events returns the event channel.
func (c *Cache) Events() <-chan Event {
	return c.events
}

// MaxZoom returns the configured max zoom level.
func (c *Cache) MaxZoom() int {
	return c.maxZoom
}

// Get returns tile data, fetching from upstream on cache miss.
func (c *Cache) Get(ctx context.Context, z, x, y int) ([]byte, error) {
	path := c.tilePath(z, x, y)

	// Cache hit
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}

	// Cache miss — fetch from upstream
	return c.fetchAndSave(ctx, z, x, y)
}

// Has returns true if the tile is cached on disk.
func (c *Cache) Has(z, x, y int) bool {
	_, err := os.Stat(c.tilePath(z, x, y))
	return err == nil
}

// Preload fetches a batch of tiles, skipping those already cached.
func (c *Cache) Preload(ctx context.Context, tiles []TileCoord) {
	total := len(tiles)
	done := 0
	skipped := 0

	for _, tc := range tiles {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if c.Has(tc.Z, tc.X, tc.Y) {
			skipped++
			done++
			continue
		}

		if c.rateLimit > 0 {
			time.Sleep(c.rateLimit)
		}

		if _, err := c.fetchAndSave(ctx, tc.Z, tc.X, tc.Y); err != nil {
			log.Printf("[tilecache] preload %d/%d/%d: %v", tc.Z, tc.X, tc.Y, err)
		}

		done++

		// Emit progress event
		select {
		case c.events <- Event{
			Type: "tile_preload_progress",
			Data: map[string]any{
				"done":    done,
				"total":   total,
				"skipped": skipped,
			},
		}:
		default:
		}
	}

	// Emit completion event
	select {
	case c.events <- Event{
		Type: "tile_preload_complete",
		Data: map[string]any{
			"total":   total,
			"skipped": skipped,
		},
	}:
	default:
	}
}

// Status returns the current cache statistics.
func (c *Cache) Status() (CacheStatus, error) {
	var status CacheStatus

	err := filepath.Walk(c.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() && strings.HasSuffix(path, ".png") {
			status.TileCount++
			status.DiskUsage += info.Size()
		}
		return nil
	})

	return status, err
}

func (c *Cache) tilePath(z, x, y int) string {
	return filepath.Join(c.dataDir, fmt.Sprintf("%d", z), fmt.Sprintf("%d", x), fmt.Sprintf("%d.png", y))
}

func (c *Cache) fetchAndSave(ctx context.Context, z, x, y int) ([]byte, error) {
	url := c.tileURL
	url = strings.Replace(url, "{z}", fmt.Sprintf("%d", z), 1)
	url = strings.Replace(url, "{x}", fmt.Sprintf("%d", x), 1)
	url = strings.Replace(url, "{y}", fmt.Sprintf("%d", y), 1)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Save to disk
	path := c.tilePath(z, x, y)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return data, nil // return data even if save fails
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("[tilecache] write %s: %v", path, err)
	}

	return data, nil
}

// TilesForBounds returns all tile coordinates within the given bounds and zoom range.
func TilesForBounds(south, west, north, east float64, zoomMin, zoomMax int) []TileCoord {
	var tiles []TileCoord
	for z := zoomMin; z <= zoomMax; z++ {
		xMin, yMax := lonLatToTile(west, south, z)
		xMax, yMin := lonLatToTile(east, north, z)

		for x := xMin; x <= xMax; x++ {
			for y := yMin; y <= yMax; y++ {
				tiles = append(tiles, TileCoord{Z: z, X: x, Y: y})
			}
		}
	}
	return tiles
}

// EstimateTileCount returns the number of tiles for bounds without allocating the slice.
func EstimateTileCount(south, west, north, east float64, zoomMin, zoomMax int) int {
	count := 0
	for z := zoomMin; z <= zoomMax; z++ {
		xMin, yMax := lonLatToTile(west, south, z)
		xMax, yMin := lonLatToTile(east, north, z)
		count += (xMax - xMin + 1) * (yMax - yMin + 1)
	}
	return count
}

// lonLatToTile converts geographic coordinates to tile numbers at a given zoom.
func lonLatToTile(lon, lat float64, zoom int) (x, y int) {
	n := math.Pow(2, float64(zoom))
	x = int(math.Floor((lon + 180.0) / 360.0 * n))
	latRad := lat * math.Pi / 180.0
	y = int(math.Floor((1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n))

	// Clamp
	maxTile := int(n) - 1
	if x < 0 {
		x = 0
	}
	if x > maxTile {
		x = maxTile
	}
	if y < 0 {
		y = 0
	}
	if y > maxTile {
		y = maxTile
	}
	return x, y
}
