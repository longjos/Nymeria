package wxalert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ErrZoneNotCached is returned by Get when a zone has no cached polygon and
// allowFetch is false, or offline fetch fails. Callers use it to switch to
// the "NWS did not publish a polygon" / own-geometry fallback.
var ErrZoneNotCached = errors.New("wxalert: zone geometry not cached")

// ErrBadUGC is returned for a UGC code that does not match the expected
// shape (two letters, C or Z, three digits) — also guards against path
// traversal through a hand-built cache path.
var ErrBadUGC = errors.New("wxalert: invalid UGC code")

var ugcPattern = regexp.MustCompile(`^[A-Z]{2}[CZ][0-9]{3}$`)

// FallbackReason renders err for the "NWS did not publish a polygon and the
// zone outline is not cached" UI copy.
func FallbackReason(err error) string {
	if errors.Is(err, ErrZoneNotCached) {
		return "zone geometry not cached"
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// ZoneProvider is the network dependency ZoneCache needs. NWSProvider (nws.go)
// implements it; tests use a fake.
type ZoneProvider interface {
	FetchZone(ctx context.Context, zoneType, ugc string) (ZoneRecord, error)
	ListZones(ctx context.Context, zoneType, state string) ([]ZoneRef, error)
}

// ZoneCacheConfig configures a ZoneCache.
type ZoneCacheConfig struct {
	DataDir      string        // required
	RateLimit    time.Duration // delay between upstream fetches during Prefetch; 0 = no delay (tests)
	MaxZones     int           // 0 = default 600
	TombstoneTTL time.Duration // 0 = default 24h
}

type tombstone struct {
	At time.Time `json:"at"`
}

// ZoneCacheStatus reports the cache's disk footprint.
type ZoneCacheStatus struct {
	ZoneCount int      `json:"zoneCount"`
	DiskUsage int64    `json:"diskUsage"`
	IDs       []string `json:"ids"`
}

// PrefetchResult summarizes one Prefetch call.
type PrefetchResult struct {
	Fetched  int
	Skipped  int
	Failed   int
	Capped   int
	Failures map[string]string
}

// ZoneCache caches NWS zone polygons on disk, one file per UGC, following the
// tilecache pattern: disk first, fetch only when asked to.
type ZoneCache struct {
	dataDir  string
	rate     time.Duration
	maxZones int
	tombTTL  time.Duration
	provider ZoneProvider
	events   chan Event
}

// NewZoneCache creates a ZoneCache rooted at cfg.DataDir (created if absent).
func NewZoneCache(cfg ZoneCacheConfig, p ZoneProvider) (*ZoneCache, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("wxalert: zonecache data dir is required")
	}
	if cfg.MaxZones == 0 {
		cfg.MaxZones = 600
	}
	if cfg.TombstoneTTL == 0 {
		cfg.TombstoneTTL = 24 * time.Hour
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "zones"), 0o755); err != nil {
		return nil, fmt.Errorf("wxalert: create zone cache dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "index"), 0o755); err != nil {
		return nil, fmt.Errorf("wxalert: create zone index dir: %w", err)
	}
	return &ZoneCache{
		dataDir:  cfg.DataDir,
		rate:     cfg.RateLimit,
		maxZones: cfg.MaxZones,
		tombTTL:  cfg.TombstoneTTL,
		provider: p,
		events:   make(chan Event, 64),
	}, nil
}

// Events returns the cache's progress-event channel (zone_prefetch messages).
func (z *ZoneCache) Events() <-chan Event { return z.events }

func normalizeUGC(ugc string) string { return strings.ToUpper(strings.TrimSpace(ugc)) }

func (z *ZoneCache) zonePath(ugc string) (string, error) {
	n := normalizeUGC(ugc)
	if !ugcPattern.MatchString(n) {
		return "", ErrBadUGC
	}
	return filepath.Join(z.dataDir, "zones", n+".json"), nil
}

func (z *ZoneCache) tombstonePath(ugc string) string {
	return filepath.Join(z.dataDir, "zones", normalizeUGC(ugc)+".tombstone.json")
}

// ZoneTypeOf reports the zone type implied by a UGC's third character.
// 'C' is unambiguously county; 'Z' can be forecast or fire, so callers probe
// forecast first, then fire.
func ZoneTypeOf(ugc string) string {
	n := normalizeUGC(ugc)
	if len(n) < 3 {
		return "unknown"
	}
	switch n[2] {
	case 'C':
		return "county"
	case 'Z':
		return "forecast"
	default:
		return "unknown"
	}
}

// Has reports whether ugc has a cached (non-tombstoned) record on disk.
func (z *ZoneCache) Has(ugc string) bool {
	path, err := z.zonePath(ugc)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Get returns ugc's cached zone record. If it is not cached: allowFetch=false
// returns ErrZoneNotCached; allowFetch=true fetches it (county/forecast/fire
// probe order), caches it, and returns it — or ErrZoneNotCached if the
// provider has nothing for it either (after writing a tombstone so the next
// call within TombstoneTTL does not hit the network again). A corrupt cache
// file is treated as a miss: deleted, and re-fetched if allowFetch.
func (z *ZoneCache) Get(ctx context.Context, ugc string, allowFetch bool) (*ZoneRecord, error) {
	path, err := z.zonePath(ugc)
	if err != nil {
		return nil, err
	}
	n := normalizeUGC(ugc)

	if data, err := os.ReadFile(path); err == nil {
		rec, perr := parseZoneRecord(data)
		if perr == nil {
			return rec, nil
		}
		_ = os.Remove(path) // corrupt: drop it and fall through to (re)fetch
	}

	if !allowFetch {
		return nil, ErrZoneNotCached
	}

	if z.tombstoned(n) {
		return nil, ErrZoneNotCached
	}

	rec, err := z.fetchAndSave(ctx, n)
	if err != nil {
		z.writeTombstone(n)
		return nil, ErrZoneNotCached
	}
	return rec, nil
}

func (z *ZoneCache) tombstoned(ugc string) bool {
	data, err := os.ReadFile(z.tombstonePath(ugc))
	if err != nil {
		return false
	}
	var ts tombstone
	if err := json.Unmarshal(data, &ts); err != nil {
		return false
	}
	return time.Since(ts.At) < z.tombTTL
}

func (z *ZoneCache) writeTombstone(ugc string) {
	data, _ := json.Marshal(tombstone{At: time.Now().UTC()})
	_ = os.WriteFile(z.tombstonePath(ugc), data, 0o644)
}

func (z *ZoneCache) fetchAndSave(ctx context.Context, ugc string) (*ZoneRecord, error) {
	order := []string{ZoneTypeOf(ugc)}
	if order[0] == "forecast" {
		order = append(order, "fire")
	} else if order[0] == "unknown" {
		order = []string{"county", "forecast", "fire"}
	}

	var lastErr error
	for _, zt := range order {
		rec, err := z.provider.FetchZone(ctx, zt, ugc)
		if err != nil {
			lastErr = err
			continue
		}
		rec.UGC = ugc
		if rec.Type == "" {
			rec.Type = zt
		}
		rec.FetchedAt = time.Now().UTC()
		if err := populateRings(&rec); err != nil {
			return nil, err
		}
		if err := z.save(ugc, &rec); err != nil {
			return nil, err
		}
		return &rec, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no provider succeeded for %s", ugc)
	}
	return nil, lastErr
}

func (z *ZoneCache) save(ugc string, rec *ZoneRecord) error {
	path, err := z.zonePath(ugc)
	if err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func parseZoneRecord(data []byte) (*ZoneRecord, error) {
	var rec ZoneRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	if err := populateRings(&rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// populateRings fills rec.rings/rec.holes from rec.Geometry. A record with
// no geometry (a tombstone-shaped record, or a zone NWS has no polygon for)
// is left with nil rings, not an error.
func populateRings(rec *ZoneRecord) error {
	if len(rec.Geometry) == 0 {
		return nil
	}
	_, outer, holes, err := ringsFromGeoJSON(rec.Geometry)
	if err != nil {
		return err
	}
	rec.rings = outer
	rec.holes = holes
	return nil
}

// ContainsPoint reports whether p falls inside ugc's cached polygon (holes
// honoured). Returns ErrZoneNotCached if the zone has no cached geometry.
func (z *ZoneCache) ContainsPoint(ugc string, p LatLon) (bool, error) {
	path, err := z.zonePath(ugc)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, ErrZoneNotCached
	}
	rec, err := parseZoneRecord(data)
	if err != nil {
		return false, ErrZoneNotCached
	}
	for _, ring := range rec.rings {
		if pointInRing(p, ring, rec.holes) {
			return true, nil
		}
	}
	return false, nil
}

// Prefetch fetches every UGC in ugcs that is not already cached, rate
// limited by cfg.RateLimit, capped at MaxZones per call, continuing past any
// individual failure. It is cancellable via ctx.
func (z *ZoneCache) Prefetch(ctx context.Context, ugcs []string) PrefetchResult {
	result := PrefetchResult{Failures: map[string]string{}}
	fetched := 0
	for i, ugc := range ugcs {
		select {
		case <-ctx.Done():
			return result
		default:
		}
		if z.Has(ugc) {
			result.Skipped++
			continue
		}
		if fetched >= z.maxZones {
			result.Capped += len(ugcs) - i
			z.emit(Event{Type: "zone_prefetch", Data: map[string]any{"capped": result.Capped}})
			break
		}
		if i > 0 && z.rate > 0 {
			time.Sleep(z.rate)
		}
		if _, err := z.Get(ctx, ugc, true); err != nil {
			result.Failed++
			result.Failures[normalizeUGC(ugc)] = err.Error()
			continue
		}
		fetched++
		result.Fetched++
	}
	return result
}

func (z *ZoneCache) emit(e Event) {
	select {
	case z.events <- e:
	default:
	}
}

// Status reports the current cache footprint.
func (z *ZoneCache) Status() ZoneCacheStatus {
	var st ZoneCacheStatus
	dir := filepath.Join(z.dataDir, "zones")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return st
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.Contains(e.Name(), ".tombstone.") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		st.ZoneCount++
		st.DiskUsage += info.Size()
		st.IDs = append(st.IDs, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(st.IDs)
	return st
}

// Evict removes cached zone files not in keep, oldest FetchedAt first, down
// to at most MaxZones remaining.
func (z *ZoneCache) Evict(keep map[string]bool) (int, error) {
	dir := filepath.Join(z.dataDir, "zones")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	type fileInfo struct {
		ugc       string
		path      string
		fetchedAt time.Time
	}
	var files []fileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.Contains(e.Name(), ".tombstone.") {
			continue
		}
		ugc := strings.TrimSuffix(e.Name(), ".json")
		path := filepath.Join(dir, e.Name())
		if keep[ugc] {
			continue
		}
		data, err := os.ReadFile(path)
		var fetchedAt time.Time
		if err == nil {
			var rec ZoneRecord
			if json.Unmarshal(data, &rec) == nil {
				fetchedAt = rec.FetchedAt
			}
		}
		files = append(files, fileInfo{ugc: ugc, path: path, fetchedAt: fetchedAt})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].fetchedAt.Before(files[j].fetchedAt) })

	keptCount := len(entries) - len(files)
	allowedNonKept := z.maxZones - keptCount
	if allowedNonKept < 0 {
		allowedNonKept = 0
	}
	toRemove := len(files) - allowedNonKept
	if toRemove < 0 {
		toRemove = 0
	}
	removed := 0
	for i := 0; i < toRemove; i++ {
		if err := os.Remove(files[i].path); err == nil {
			removed++
		}
	}
	return removed, nil
}

// ListState returns the cached zone index for a state, fetching it if
// allowFetch and not already cached (or stale).
func (z *ZoneCache) ListState(ctx context.Context, zoneType, state string, allowFetch bool) ([]ZoneRef, error) {
	idxPath := filepath.Join(z.dataDir, "index", fmt.Sprintf("%s_%s.json", strings.ToUpper(zoneType), strings.ToUpper(state)))
	const indexTTL = 30 * 24 * time.Hour

	if info, err := os.Stat(idxPath); err == nil {
		if time.Since(info.ModTime()) < indexTTL || !allowFetch {
			data, rerr := os.ReadFile(idxPath)
			if rerr == nil {
				var refs []ZoneRef
				if json.Unmarshal(data, &refs) == nil {
					return refs, nil
				}
			}
		}
	} else if !allowFetch {
		return nil, ErrZoneNotCached
	}

	refs, err := z.provider.ListZones(ctx, zoneType, state)
	if err != nil {
		// Fall back to a stale cached copy rather than failing outright.
		if data, rerr := os.ReadFile(idxPath); rerr == nil {
			var cached []ZoneRef
			if json.Unmarshal(data, &cached) == nil {
				return cached, nil
			}
		}
		return nil, err
	}
	if data, merr := json.Marshal(refs); merr == nil {
		_ = os.WriteFile(idxPath, data, 0o644)
	}
	return refs, nil
}
