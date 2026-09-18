package wxalert

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeZoneProvider serves fixed responses per (zoneType, ugc) and counts
// calls, so tests can assert exactly how many times (and which path) the
// network would have been hit.
type fakeZoneProvider struct {
	byKey map[string]ZoneRecord // key: zoneType+"/"+ugc
	err   map[string]error      // key: zoneType+"/"+ugc
	calls []string
}

func newFakeZoneProvider() *fakeZoneProvider {
	return &fakeZoneProvider{byKey: map[string]ZoneRecord{}, err: map[string]error{}}
}

func (f *fakeZoneProvider) key(zoneType, ugc string) string { return zoneType + "/" + ugc }

func (f *fakeZoneProvider) serve(zoneType, ugc string, rec ZoneRecord) {
	f.byKey[f.key(zoneType, ugc)] = rec
}

func (f *fakeZoneProvider) fail(zoneType, ugc string, err error) {
	f.err[f.key(zoneType, ugc)] = err
}

func (f *fakeZoneProvider) FetchZone(ctx context.Context, zoneType, ugc string) (ZoneRecord, error) {
	f.calls = append(f.calls, f.key(zoneType, ugc))
	if err, ok := f.err[f.key(zoneType, ugc)]; ok {
		return ZoneRecord{}, err
	}
	if rec, ok := f.byKey[f.key(zoneType, ugc)]; ok {
		return rec, nil
	}
	return ZoneRecord{}, errors.New("404 not found")
}

func (f *fakeZoneProvider) ListZones(ctx context.Context, zoneType, state string) ([]ZoneRef, error) {
	return nil, errors.New("not implemented in fake")
}

func mustZoneCache(t *testing.T, p ZoneProvider) *ZoneCache {
	t.Helper()
	zc, err := NewZoneCache(ZoneCacheConfig{DataDir: t.TempDir()}, p)
	if err != nil {
		t.Fatalf("NewZoneCache: %v", err)
	}
	return zc
}

func boxGeoJSON(ring []LatLon) json.RawMessage {
	pts := make([][2]float64, len(ring))
	for i, p := range ring {
		pts[i] = [2]float64{p.Lon, p.Lat}
	}
	data, _ := json.Marshal(struct {
		Type        string         `json:"type"`
		Coordinates [][][2]float64 `json:"coordinates"`
	}{"Polygon", [][][2]float64{pts}})
	return data
}

func TestZonePath(t *testing.T) {
	zc := &ZoneCache{dataDir: "/tmp/zones"}
	p, err := zc.zonePath("MIZ056")
	if err != nil {
		t.Fatalf("zonePath: %v", err)
	}
	want := filepath.Join("/tmp/zones", "zones", "MIZ056.json")
	if p != want {
		t.Errorf("zonePath = %q, want %q", p, want)
	}
	if _, err := zc.zonePath("../etc"); !errors.Is(err, ErrBadUGC) {
		t.Errorf("zonePath(../etc) err = %v, want ErrBadUGC", err)
	}
	pLower, err := zc.zonePath("mic081")
	if err != nil || filepath.Base(pLower) != "MIC081.json" {
		t.Errorf("zonePath lowercase not normalized: %q err=%v", pLower, err)
	}
}

func TestZoneGet_CacheHit(t *testing.T) {
	// No provider at all — a call would panic/fail the fake; the point is
	// that a cache hit never reaches it.
	zc := mustZoneCache(t, nil)
	ring := squareRingMiles(LatLon{Lat: 42.9, Lon: -85.6}, 5)
	rec := ZoneRecord{UGC: "MIC081", Name: "Kent", State: "MI", Type: "county", Geometry: boxGeoJSON(ring)}
	data, _ := json.Marshal(rec)
	path, _ := zc.zonePath("MIC081")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	got, err := zc.Get(context.Background(), "MIC081", false)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UGC != "MIC081" || got.Name != "Kent" || got.State != "MI" {
		t.Errorf("got = %+v", got)
	}
	if len(got.rings) != 1 || len(got.rings[0]) != 5 {
		t.Errorf("rings = %v, want one 5-point ring", got.rings)
	}
}

func TestZoneGet_CacheMiss(t *testing.T) {
	fp := newFakeZoneProvider()
	ring := squareRingMiles(LatLon{Lat: 42.9, Lon: -85.6}, 5)
	fp.serve("county", "MIC081", ZoneRecord{Name: "Kent", State: "MI", Geometry: boxGeoJSON(ring)})
	zc := mustZoneCache(t, fp)

	got, err := zc.Get(context.Background(), "MIC081", true)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(fp.calls) != 1 || fp.calls[0] != "county/MIC081" {
		t.Fatalf("calls = %v, want exactly one county/MIC081", fp.calls)
	}
	if got.Name != "Kent" {
		t.Errorf("Name = %q", got.Name)
	}
	if !zc.Has("MIC081") {
		t.Errorf("Has(MIC081) = false after caching")
	}

	// Second Get is a cache hit: no more upstream calls.
	if _, err := zc.Get(context.Background(), "MIC081", true); err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if len(fp.calls) != 1 {
		t.Errorf("calls after cache hit = %v, want still 1", fp.calls)
	}

	// A 'Z' code probes forecast first.
	fp.serve("forecast", "MIZ056", ZoneRecord{Name: "Ottawa", State: "MI"})
	if _, err := zc.Get(context.Background(), "MIZ056", true); err != nil {
		t.Fatalf("Get MIZ056: %v", err)
	}
	if len(fp.calls) != 2 || fp.calls[1] != "forecast/MIZ056" {
		t.Fatalf("calls = %v, want forecast/MIZ056 second", fp.calls)
	}
}

func TestZoneGet_GeometryCollection(t *testing.T) {
	data := mustLoadFixture(t, "zone_MIZ056.json")
	var feature struct {
		Geometry json.RawMessage `json:"geometry"`
	}
	if err := json.Unmarshal(data, &feature); err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	fp := newFakeZoneProvider()
	fp.serve("forecast", "MIZ056", ZoneRecord{Name: "Ottawa", State: "MI", Geometry: feature.Geometry})
	zc := mustZoneCache(t, fp)

	got, err := zc.Get(context.Background(), "MIZ056", true)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.rings) != 2 {
		t.Fatalf("rings = %d, want 2 (flattened GeometryCollection)", len(got.rings))
	}
}

func TestZoneGet_PolygonWithHoleContainsPoint(t *testing.T) {
	outer := squareRingMiles(LatLon{Lat: 0, Lon: 0}, 20)
	hole := squareRingMiles(LatLon{Lat: 0, Lon: 0}, 5)
	geo := map[string]any{
		"type": "Polygon",
		"coordinates": [][][2]float64{
			toCoordPairs(outer), toCoordPairs(hole),
		},
	}
	raw, _ := json.Marshal(geo)
	fp := newFakeZoneProvider()
	fp.serve("county", "MIC081", ZoneRecord{Geometry: raw})
	zc := mustZoneCache(t, fp)
	if _, err := zc.Get(context.Background(), "MIC081", true); err != nil {
		t.Fatalf("Get: %v", err)
	}

	inHole, err := zc.ContainsPoint("MIC081", LatLon{Lat: 0, Lon: 0})
	if err != nil {
		t.Fatalf("ContainsPoint: %v", err)
	}
	if inHole {
		t.Errorf("ContainsPoint(centre, in the hole) = true, want false")
	}
	inRing, err := zc.ContainsPoint("MIC081", destPoint(LatLon{Lat: 0, Lon: 0}, 0, 15))
	if err != nil {
		t.Fatalf("ContainsPoint: %v", err)
	}
	if !inRing {
		t.Errorf("ContainsPoint(inside ring, outside hole) = false, want true")
	}
}

func toCoordPairs(ring []LatLon) [][2]float64 {
	out := make([][2]float64, len(ring))
	for i, p := range ring {
		out[i] = [2]float64{p.Lon, p.Lat}
	}
	return out
}

func TestZoneGet_Offline(t *testing.T) {
	zc := mustZoneCache(t, newFakeZoneProvider())
	_, err := zc.Get(context.Background(), "MIZ057", false)
	if !errors.Is(err, ErrZoneNotCached) {
		t.Fatalf("err = %v, want ErrZoneNotCached", err)
	}
	if got := FallbackReason(err); got != "zone geometry not cached" {
		t.Errorf("FallbackReason = %q", got)
	}
}

func TestZoneGet_CorruptFile(t *testing.T) {
	corrupt := mustLoadFixture(t, "zone_corrupt.json")

	t.Run("recovers via fetch", func(t *testing.T) {
		fp := newFakeZoneProvider()
		fp.serve("forecast", "MIZ057", ZoneRecord{Name: "Good", Geometry: boxGeoJSON(squareRingMiles(LatLon{}, 5))})
		zc := mustZoneCache(t, fp)
		path, _ := zc.zonePath("MIZ057")
		if err := os.WriteFile(path, corrupt, 0o644); err != nil {
			t.Fatalf("seed corrupt: %v", err)
		}
		got, err := zc.Get(context.Background(), "MIZ057", true)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Name != "Good" {
			t.Errorf("Name = %q, want Good (fetched fresh copy)", got.Name)
		}
		if len(fp.calls) != 1 {
			t.Errorf("calls = %v, want exactly one", fp.calls)
		}
	})

	t.Run("no server: not cached, corrupt file removed", func(t *testing.T) {
		zc := mustZoneCache(t, newFakeZoneProvider())
		path, _ := zc.zonePath("MIZ057")
		if err := os.WriteFile(path, corrupt, 0o644); err != nil {
			t.Fatalf("seed corrupt: %v", err)
		}
		_, err := zc.Get(context.Background(), "MIZ057", false)
		if !errors.Is(err, ErrZoneNotCached) {
			t.Fatalf("err = %v, want ErrZoneNotCached", err)
		}
		if zc.Has("MIZ057") {
			t.Errorf("Has() = true, want the corrupt file to have been removed")
		}
	})
}

func TestZonePrefetchBoundsAndSkip(t *testing.T) {
	fp := newFakeZoneProvider()
	for _, ugc := range []string{"MIC081", "MIZ057", "MIC005", "MIC139"} {
		fp.serve(ZoneTypeOf(ugc), ugc, ZoneRecord{Name: ugc})
	}
	zc := mustZoneCache(t, fp)

	res := zc.Prefetch(context.Background(), []string{"MIC081", "MIZ057", "MIC005", "MIC139"})
	if res.Fetched != 4 || res.Failed != 0 {
		t.Fatalf("first Prefetch = %+v, want Fetched=4", res)
	}

	res2 := zc.Prefetch(context.Background(), []string{"MIC081", "MIZ057", "MIC005", "MIC139"})
	if res2.Skipped != 4 || res2.Fetched != 0 {
		t.Fatalf("second Prefetch = %+v, want Skipped=4", res2)
	}
}

func TestZonePrefetchCap(t *testing.T) {
	fp := newFakeZoneProvider()
	ugcs := []string{"MIC001", "MIC002", "MIC003", "MIC004", "MIC005"}
	for _, u := range ugcs {
		fp.serve("county", u, ZoneRecord{Name: u})
	}
	zc, err := NewZoneCache(ZoneCacheConfig{DataDir: t.TempDir(), MaxZones: 3}, fp)
	if err != nil {
		t.Fatalf("NewZoneCache: %v", err)
	}
	res := zc.Prefetch(context.Background(), ugcs)
	if res.Fetched != 3 || res.Capped != 2 {
		t.Fatalf("Prefetch = %+v, want Fetched=3 Capped=2", res)
	}
	for _, u := range ugcs[3:] {
		if zc.Has(u) {
			t.Errorf("capped zone %s should not have a partial file", u)
		}
	}
}

func TestZonePrefetchContinuesOnFailure(t *testing.T) {
	fp := newFakeZoneProvider()
	ugcs := []string{"MIC001", "MIC002", "MIC139", "MIC004"}
	for _, u := range ugcs {
		fp.serve("county", u, ZoneRecord{Name: u})
	}
	fp.fail("county", "MIC139", errors.New("500 internal server error"))
	zc := mustZoneCache(t, fp)

	res := zc.Prefetch(context.Background(), ugcs)
	if res.Fetched != 3 || res.Failed != 1 {
		t.Fatalf("Prefetch = %+v, want Fetched=3 Failed=1", res)
	}
	if msg := res.Failures["MIC139"]; msg == "" {
		t.Errorf("Failures[MIC139] empty, want the error text")
	}
	for _, u := range []string{"MIC001", "MIC002", "MIC004"} {
		if !zc.Has(u) {
			t.Errorf("%s should be cached despite MIC139 failing", u)
		}
	}
}

func TestZonePrefetchRateLimit(t *testing.T) {
	fp := newFakeZoneProvider()
	ugcs := []string{"MIC001", "MIC002", "MIC003"}
	for _, u := range ugcs {
		fp.serve("county", u, ZoneRecord{Name: u})
	}
	zc, err := NewZoneCache(ZoneCacheConfig{DataDir: t.TempDir(), RateLimit: 20 * time.Millisecond}, fp)
	if err != nil {
		t.Fatalf("NewZoneCache: %v", err)
	}
	start := time.Now()
	zc.Prefetch(context.Background(), ugcs)
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 40ms for 3 zones at 20ms/zone", elapsed)
	}
}

func TestZoneStatus(t *testing.T) {
	fp := newFakeZoneProvider()
	ugcs := []string{"MIC001", "MIC002", "MIC003"}
	for _, u := range ugcs {
		fp.serve("county", u, ZoneRecord{Name: u})
	}
	zc := mustZoneCache(t, fp)
	zc.Prefetch(context.Background(), ugcs)

	st := zc.Status()
	if st.ZoneCount != 3 {
		t.Errorf("ZoneCount = %d, want 3", st.ZoneCount)
	}
	if st.DiskUsage <= 0 {
		t.Errorf("DiskUsage = %d, want > 0", st.DiskUsage)
	}
	want := []string{"MIC001", "MIC002", "MIC003"}
	if len(st.IDs) != len(want) {
		t.Fatalf("IDs = %v, want %v", st.IDs, want)
	}
	for i := range want {
		if st.IDs[i] != want[i] {
			t.Errorf("IDs[%d] = %q, want %q", i, st.IDs[i], want[i])
		}
	}
}

func TestZoneEvict(t *testing.T) {
	fp := newFakeZoneProvider()
	ugcs := []string{"MIC001", "MIC002", "MIC003", "MIC004"}
	for _, u := range ugcs {
		fp.serve("county", u, ZoneRecord{Name: u})
	}
	zc, err := NewZoneCache(ZoneCacheConfig{DataDir: t.TempDir(), MaxZones: 2}, fp)
	if err != nil {
		t.Fatalf("NewZoneCache: %v", err)
	}
	for _, u := range ugcs {
		if _, err := zc.Get(context.Background(), u, true); err != nil {
			t.Fatalf("Get(%s): %v", u, err)
		}
		time.Sleep(time.Millisecond) // ensure distinct FetchedAt ordering
	}
	removed, err := zc.Evict(map[string]bool{"MIC004": true})
	if err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2 (down to MaxZones=2, MIC004 kept)", removed)
	}
	if !zc.Has("MIC004") {
		t.Errorf("kept zone MIC004 was evicted")
	}
	if zc.Has("MIC001") {
		t.Errorf("oldest zone MIC001 should have been evicted first")
	}
}

func TestZoneTypeOf(t *testing.T) {
	tests := []struct{ ugc, want string }{
		{"MIC081", "county"},
		{"MIZ056", "forecast"},
		{"", "unknown"},
		{"MI", "unknown"},
	}
	for _, tt := range tests {
		if got := ZoneTypeOf(tt.ugc); got != tt.want {
			t.Errorf("ZoneTypeOf(%q) = %q, want %q", tt.ugc, got, tt.want)
		}
	}
}
