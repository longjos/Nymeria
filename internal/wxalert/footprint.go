package wxalert

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

// PositionMaxAge is how old a roster/tracked position can be and still count
// toward the footprint.
const PositionMaxAge = 2 * time.Hour

// RouteSampleMiles is the spacing used to resample route/line annotations.
const RouteSampleMiles = 2.0

// footprintCategories are the annotation categories that widen the watch
// area. hazard/incident/resource are deliberately excluded (BUILD-PLAN §1
// row 45): a hazard we plotted 40 miles away must not widen the footprint.
// wxalert does not import internal/annotation, so these are the category
// strings verbatim rather than its constants.
var footprintCategories = map[string]bool{
	"checkpoint": true, "route": true, "boundary": true, "assignment": true,
	"general": true, "aid": true, "staging": true, "shelter": true,
	"parking": true, "start": true, "finish": true,
}

// PositionInput is one roster or tracked-station position. Manager builds
// these from netcontrol/station lookups; footprint.go itself has no
// dependency on either package.
type PositionInput struct {
	Kind      string // "roster" | "tracked"
	ID        string // callsign
	CheckInID string // roster check-in id; "" for tracked-only
	Label     string
	Lat       float64
	Lon       float64
	HeardAt   time.Time
}

// Inputs is everything Build needs to compute a net's watch footprint. Zone
// resolution (lat/lon -> UGC) touches the network/disk cache, so it happens
// outside this pure package: ZoneLookup and NearUGC arrive pre-resolved.
// ZoneLookup is keyed by sample key (see Footprint doc below); a missing key
// means "not resolved yet" (counts toward FootprintSummary.UnresolvedSamples)
// rather than "resolved to nothing".
type Inputs struct {
	NetID         string
	BufferMiles   float64
	Annotations   []store.Annotation
	CheckpointSeq map[string]int // annotation id -> sequence number
	Positions     []PositionInput
	Own           *LatLon
	OwnSource     string // "gps" | "config" | "none"
	OwnAgeSec     int
	ZoneLookup    map[string][]string // sample key -> resolved UGC codes
	ExtraZones    []string            // NCS-added extra watch zones, folded into the IN set
	NearUGC       []string            // whole-footprint NEAR zone set (probe-point resolved)
	Now           time.Time
}

// sample is one point the footprint matches alerts against.
type sample struct {
	key       string // stable identity used to look up ZoneLookup
	pt        LatLon
	kind      string // "route" | "checkpoint" | "location"
	refID     string // annotation id
	label     string
	shortName string
	seq       int
	lineID    string // set for "route" samples: which line annotation
	vertexIdx int    // set for "route" samples: index into that line
	zones     []string
}

type lineRef struct {
	id    string
	label string
	pts   []LatLon
}

type areaRef struct {
	id    string
	label string
	ring  []LatLon
}

type stationSample struct {
	kind      string // "roster" | "tracked" | "own"
	id        string // callsign, or "own"
	checkInID string
	label     string
	pt        LatLon
	zones     []string
}

// Footprint is the built watch area for one net (or no net, for an
// operator's own-position-only watch).
type Footprint struct {
	NetID       string
	BufferMiles float64
	samples     []sample
	lines       []lineRef
	areas       []areaRef
	stations    []stationSample
	ugc         map[string]bool
	nearUGC     map[string]bool
	bbox        BBox
	haveBBox    bool
	centroid    LatLon
	summary     FootprintSummary
}

// Summary returns the precomputed FootprintSummary.
func (f *Footprint) Summary() FootprintSummary { return f.summary }

// Empty reports whether there is nothing at all to watch: no course,
// positions or own position.
func (f *Footprint) Empty() bool { return f.summary.Empty }

// Bounds returns south, west, north, east.
func (f *Footprint) Bounds() (float64, float64, float64, float64) {
	return f.bbox.South, f.bbox.West, f.bbox.North, f.bbox.East
}

// CrossesAntimeridian reports whether the footprint's bounding box wraps the
// +/-180 line.
func (f *Footprint) CrossesAntimeridian() bool { return f.bbox.CrossesAntimeridian() }

// Build computes a Footprint from Inputs. It is pure: no I/O, no clock reads
// beyond in.Now.
func Build(in Inputs) *Footprint {
	f := &Footprint{
		NetID:       in.NetID,
		BufferMiles: in.BufferMiles,
		ugc:         map[string]bool{},
		nearUGC:     map[string]bool{},
	}
	for _, z := range in.ExtraZones {
		f.ugc[strings.ToUpper(strings.TrimSpace(z))] = true
	}
	for _, z := range in.NearUGC {
		f.nearUGC[strings.ToUpper(strings.TrimSpace(z))] = true
	}

	zonesFor := func(key string) []string {
		if in.ZoneLookup == nil {
			return nil
		}
		return in.ZoneLookup[key]
	}
	resolved := 0
	unresolved := 0
	addZones := func(key string) []string {
		z := zonesFor(key)
		if z == nil {
			unresolved++
			return nil
		}
		resolved++
		for _, u := range z {
			f.ugc[strings.ToUpper(strings.TrimSpace(u))] = true
		}
		return z
	}

	var allPts []LatLon
	var checkpointCount, locationCount int
	var routeMiles float64

	for _, ann := range in.Annotations {
		if in.NetID != "" && ann.NetID != "" && ann.NetID != in.NetID {
			continue
		}
		if ann.ExpiresAt != nil && !ann.ExpiresAt.After(in.Now) {
			continue
		}
		if ann.Status == "resolved" || ann.Status == "closed" {
			continue
		}
		if !footprintCategories[ann.Category] {
			continue
		}

		switch ann.Type {
		case "point":
			pt, ok := parsePointGeometry(ann.Geometry)
			if !ok {
				continue
			}
			s := sample{key: ann.ID, pt: pt, refID: ann.ID, label: ann.Label, shortName: ann.ShortName}
			if ann.Category == "checkpoint" {
				s.kind = "checkpoint"
				s.seq = in.CheckpointSeq[ann.ID]
				checkpointCount++
			} else {
				s.kind = "location"
				locationCount++
			}
			s.zones = addZones(s.key)
			f.samples = append(f.samples, s)
			allPts = append(allPts, pt)

		case "line":
			pts, ok := parseLineGeometry(ann.Geometry)
			if !ok || len(pts) < 2 {
				continue
			}
			resampled := resampleMiles(pts, RouteSampleMiles)
			f.lines = append(f.lines, lineRef{id: ann.ID, label: ann.Label, pts: pts})
			routeMiles += polylineMiles(pts)
			for i, p := range dedupeByCell(resampled) {
				key := fmt.Sprintf("%s#%d", ann.ID, i)
				s := sample{key: key, pt: p, kind: "route", refID: ann.ID, label: ann.Label, lineID: ann.ID, vertexIdx: i}
				s.zones = addZones(key)
				f.samples = append(f.samples, s)
				allPts = append(allPts, p)
			}

		case "area":
			ring, ok := parseAreaGeometry(ann.Geometry)
			if !ok || len(ring) < 3 {
				continue
			}
			f.areas = append(f.areas, areaRef{id: ann.ID, label: ann.Label, ring: ring})
			allPts = append(allPts, ring...)
		}
	}

	rosterCount, trackedCount := 0, 0
	for _, p := range in.Positions {
		if p.Lat == 0 && p.Lon == 0 {
			continue
		}
		if p.Lat < -90 || p.Lat > 90 || p.Lon < -180 || p.Lon > 180 {
			continue
		}
		if !p.HeardAt.IsZero() && in.Now.Sub(p.HeardAt) > PositionMaxAge {
			continue
		}
		pt := LatLon{Lat: p.Lat, Lon: p.Lon}
		ss := stationSample{kind: p.Kind, id: p.ID, checkInID: p.CheckInID, label: p.Label, pt: pt}
		key := "station:" + p.ID
		ss.zones = addZones(key)
		f.stations = append(f.stations, ss)
		allPts = append(allPts, pt)
		if p.Kind == "roster" {
			rosterCount++
		} else {
			trackedCount++
		}
	}

	ownStation := "none"
	if in.Own != nil {
		ownStation = in.OwnSource
		if ownStation == "" {
			ownStation = "gps"
		}
		ss := stationSample{kind: "own", id: "own", label: "Own position", pt: *in.Own}
		ss.zones = addZones("own")
		f.stations = append(f.stations, ss)
		allPts = append(allPts, *in.Own)
	}

	if len(allPts) > 0 {
		f.centroid = centroidOf(allPts)
		f.bbox = expandBBoxMiles(bboxOf(allPts), in.BufferMiles)
		f.haveBBox = true
	}

	empty := len(f.samples) == 0 && len(f.areas) == 0 && len(f.stations) == 0
	f.summary = FootprintSummary{
		NetID:             in.NetID,
		BufferMiles:       in.BufferMiles,
		RouteMiles:        routeMiles,
		LocationCount:     locationCount,
		CheckpointCount:   checkpointCount,
		RosterPositions:   rosterCount,
		TrackedPositions:  trackedCount,
		OwnStation:        ownStation,
		OwnStationAgeSec:  in.OwnAgeSec,
		Zones:             sortedZoneRefs(f.ugc),
		NearZones:         sortedZoneRefs(f.nearUGC),
		ExtraZones:        append([]string{}, in.ExtraZones...),
		UnresolvedSamples: unresolved,
		Empty:             empty,
		ComputedAt:        in.Now,
	}
	if in.ExtraZones == nil {
		f.summary.ExtraZones = []string{}
	}
	if f.haveBBox {
		c := f.centroid
		f.summary.Centroid = &c
	}
	return f
}

func centroidOf(pts []LatLon) LatLon {
	var sumLat, sumLon float64
	for _, p := range pts {
		sumLat += p.Lat
		sumLon += p.Lon
	}
	n := float64(len(pts))
	return LatLon{Lat: sumLat / n, Lon: sumLon / n}
}

func sortedZoneRefs(set map[string]bool) []ZoneRef {
	out := make([]ZoneRef, 0, len(set))
	codes := make([]string, 0, len(set))
	for z := range set {
		codes = append(codes, z)
	}
	sort.Strings(codes)
	for _, z := range codes {
		out = append(out, ZoneRef{UGC: z, Type: zoneTypeOf(z)})
	}
	return out
}

func zoneTypeOf(ugc string) string {
	if len(ugc) < 3 {
		return "unknown"
	}
	switch ugc[2] {
	case 'C':
		return "county"
	case 'Z':
		return "forecast"
	default:
		return "unknown"
	}
}

// dedupeByCell removes points that fall in the same 0.01-degree cell as one
// already kept, preserving order.
func dedupeByCell(pts []LatLon) []LatLon {
	seen := map[[2]int]bool{}
	out := make([]LatLon, 0, len(pts))
	for _, p := range pts {
		cell := [2]int{int(p.Lat * 100), int(p.Lon * 100)}
		if seen[cell] {
			continue
		}
		seen[cell] = true
		out = append(out, p)
	}
	return out
}

// ---------------------------------------------------------------------------
// Annotation geometry parsing (store.Annotation.Geometry is a raw GeoJSON
// string; coordinates are [lon,lat] as everywhere in this package).
// ---------------------------------------------------------------------------

type annGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

func parsePointGeometry(raw string) (LatLon, bool) {
	var g annGeometry
	if err := json.Unmarshal([]byte(raw), &g); err != nil || g.Type != "Point" {
		return LatLon{}, false
	}
	var c [2]float64
	if err := json.Unmarshal(g.Coordinates, &c); err != nil {
		return LatLon{}, false
	}
	return LatLon{Lon: c[0], Lat: c[1]}, true
}

func parseLineGeometry(raw string) ([]LatLon, bool) {
	var g annGeometry
	if err := json.Unmarshal([]byte(raw), &g); err != nil || g.Type != "LineString" {
		return nil, false
	}
	var coords [][2]float64
	if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
		return nil, false
	}
	return toLatLon(coords), true
}

func parseAreaGeometry(raw string) ([]LatLon, bool) {
	var g annGeometry
	if err := json.Unmarshal([]byte(raw), &g); err != nil || g.Type != "Polygon" {
		return nil, false
	}
	var coords [][][2]float64
	if err := json.Unmarshal(g.Coordinates, &coords); err != nil || len(coords) == 0 {
		return nil, false
	}
	return toLatLon(coords[0]), true
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// Match is the result of classifying one alert against a footprint.
type Match struct {
	Proximity      Proximity
	DistanceMiles  float64
	BearingDeg     int
	Affects        Affects
	GeometrySource string // "polygon" | "zone" | "none"
}

// Classify computes proximity, distance, bearing and Affects for one alert
// against this footprint. zones supplies cached zone rings for zone-only
// alerts (nil map or nil entries are fine — the match falls back to the UGC
// set / NEAR-by-probe result).
func (f *Footprint) Classify(a Alert, zones map[string]*ZoneRecord) Match {
	if a.HasPolygon() {
		return f.classifyPolygon(a.Geometry.Rings, "polygon")
	}
	return f.classifyZoneOnly(a, zones)
}

// boundaryEpsilonMiles absorbs floating-point/projection noise (the local
// equirectangular projection in geo.go is accurate to well under 1% at these
// ranges, but "exactly at the buffer" comparisons need a hair of slack so a
// physically-on-the-line case does not flip on rounding).
const boundaryEpsilonMiles = 0.05

func (f *Footprint) classifyPolygon(rings [][]LatLon, source string) Match {
	B := f.BufferMiles
	dmin := f.distanceToRings(rings)

	var prox Proximity
	switch {
	case dmin <= B+boundaryEpsilonMiles:
		prox = ProximityIn
	case dmin <= 2*B+boundaryEpsilonMiles:
		prox = ProximityNear
	default:
		prox = ProximityFar
	}

	m := Match{Proximity: prox, DistanceMiles: round2(dmin), GeometrySource: source}
	if prox == ProximityIn {
		m.DistanceMiles = 0
		m.Affects = f.affectsWithin(rings)
	} else if prox == ProximityNear && f.haveBBox {
		m.BearingDeg = bearingDeg(f.centroid, nearestRingPoint(f.centroid, rings))
	}
	return m
}

func (f *Footprint) distanceToRings(rings [][]LatLon) float64 {
	dmin := mathInf()
	consider := func(d float64) {
		if d < dmin {
			dmin = d
		}
	}
	for _, s := range f.samples {
		for _, ring := range rings {
			consider(ringDistanceMiles(s.pt, ring, nil))
		}
	}
	for _, ln := range f.lines {
		for _, ring := range rings {
			consider(lineToRingMiles(ln.pts, ring, nil))
		}
	}
	for _, ar := range f.areas {
		for _, ring := range rings {
			if ringsIntersect(ar.ring, ring) {
				consider(0)
			} else {
				for _, p := range ar.ring {
					consider(ringDistanceMiles(p, ring, nil))
				}
			}
		}
	}
	for _, st := range f.stations {
		for _, ring := range rings {
			consider(ringDistanceMiles(st.pt, ring, nil))
		}
	}
	if len(f.samples) == 0 && len(f.lines) == 0 && len(f.areas) == 0 && len(f.stations) == 0 {
		return mathInf()
	}
	return dmin
}

func mathInf() float64 { return 1e18 }

func round2(f float64) float64 {
	if f >= 1e17 {
		return f
	}
	return float64(int(f*100+0.5)) / 100
}

func nearestRingPoint(from LatLon, rings [][]LatLon) LatLon {
	best := from
	bestD := mathInf()
	for _, ring := range rings {
		for _, p := range ring {
			d := haversineMiles(from, p)
			if d < bestD {
				bestD = d
				best = p
			}
		}
	}
	return best
}

// classifyZoneOnly implements rule 2 of the classification spec: exact IN
// via the resolved UGC set, else geometric matching against any cached zone
// ring, else NEAR via the probe-resolved NearUGC set, else Far.
func (f *Footprint) classifyZoneOnly(a Alert, zoneCache map[string]*ZoneRecord) Match {
	alertZones := make(map[string]bool, len(a.UGC))
	for _, z := range a.UGC {
		alertZones[strings.ToUpper(strings.TrimSpace(z))] = true
	}

	if !a.HasLocation() {
		return Match{Proximity: ProximityFar, GeometrySource: "none"}
	}

	for z := range alertZones {
		if f.ugc[z] {
			source := "none"
			if zoneCache != nil {
				if rec, ok := zoneCache[z]; ok && rec != nil {
					source = "zone"
				}
			}
			return Match{Proximity: ProximityIn, GeometrySource: source, Affects: f.affectsWithinZones(alertZones)}
		}
	}

	// Any cached ring for a zone the alert covers? Apply geometric matching
	// against the union of those rings.
	var rings [][]LatLon
	anyCached := false
	if zoneCache != nil {
		for z := range alertZones {
			if rec, ok := zoneCache[z]; ok && rec != nil {
				anyCached = true
				rings = append(rings, rec.rings...)
			}
		}
	}
	if anyCached {
		m := f.classifyPolygon(rings, "zone")
		return m
	}

	for z := range alertZones {
		if f.nearUGC[z] {
			return Match{Proximity: ProximityNear, DistanceMiles: f.BufferMiles, GeometrySource: "none"}
		}
	}

	return Match{Proximity: ProximityFar, GeometrySource: "none"}
}

func (f *Footprint) affectsWithinZones(alertZones map[string]bool) Affects {
	within := func(zones []string) bool {
		for _, z := range zones {
			if alertZones[strings.ToUpper(strings.TrimSpace(z))] {
				return true
			}
		}
		return false
	}
	return f.buildAffects(func(s sample) bool { return within(s.zones) }, func(st stationSample) bool { return within(st.zones) })
}

func (f *Footprint) affectsWithin(rings [][]LatLon) Affects {
	within := func(p LatLon) bool {
		for _, ring := range rings {
			if ringDistanceMiles(p, ring, nil) <= f.BufferMiles {
				return true
			}
		}
		return false
	}
	return f.buildAffects(func(s sample) bool { return within(s.pt) }, func(st stationSample) bool { return within(st.pt) })
}

// buildAffects assembles the Affects payload from whichever samples/stations
// the caller's predicates say are touched.
func (f *Footprint) buildAffects(sampleHit func(sample) bool, stationHit func(stationSample) bool) Affects {
	var checkpoints, locations, stations []AffectedItem
	touchedRoute := map[string]map[int]bool{}
	totalSamples, touchedSamples := 0, 0

	for _, s := range f.samples {
		totalSamples++
		if !sampleHit(s) {
			continue
		}
		touchedSamples++
		switch s.kind {
		case "checkpoint":
			checkpoints = append(checkpoints, AffectedItem{Kind: "checkpoint", ID: s.refID, Label: s.label, ShortName: s.shortName, Seq: s.seq, Lat: s.pt.Lat, Lon: s.pt.Lon})
		case "location":
			locations = append(locations, AffectedItem{Kind: "location", ID: s.refID, Label: s.label, ShortName: s.shortName, Lat: s.pt.Lat, Lon: s.pt.Lon})
		case "route":
			if touchedRoute[s.lineID] == nil {
				touchedRoute[s.lineID] = map[int]bool{}
			}
			touchedRoute[s.lineID][s.vertexIdx] = true
		}
	}
	for _, st := range f.stations {
		if !stationHit(st) {
			continue
		}
		stations = append(stations, AffectedItem{Kind: iif(st.kind == "own", "own", "station"), ID: st.id, CheckInID: st.checkInID, Label: st.label, Lat: st.pt.Lat, Lon: st.pt.Lon})
	}

	sort.Slice(checkpoints, func(i, j int) bool { return checkpoints[i].Seq < checkpoints[j].Seq })

	var seqRange []int
	if len(checkpoints) > 0 {
		seqRange = []int{checkpoints[0].Seq, checkpoints[len(checkpoints)-1].Seq}
	} else {
		seqRange = []int{}
	}

	routeSpans := buildRouteSpans(touchedRoute)
	var routeMiles float64
	for _, ln := range f.lines {
		if idxs, ok := touchedRoute[ln.id]; ok {
			routeMiles += float64(len(idxs)) * RouteSampleMiles
		}
	}

	entireCourse := totalSamples > 0 && touchedSamples >= (totalSamples*9+9)/10 // >= 90%, integer-safe ceiling-free compare

	summary := buildAffectsSummary(checkpoints, locations, len(stations))

	return Affects{
		Summary:            summary,
		EntireCourse:       entireCourse,
		RouteMiles:         routeMiles,
		Checkpoints:        nonNilItems(checkpoints),
		Locations:          nonNilItems(locations),
		Stations:           nonNilItems(stations),
		CheckpointSeqRange: seqRange,
		RouteSpans:         routeSpans,
	}
}

func iif(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func nonNilItems(items []AffectedItem) []AffectedItem {
	if items == nil {
		return []AffectedItem{}
	}
	return items
}

func buildRouteSpans(touched map[string]map[int]bool) []RouteSpan {
	var spans []RouteSpan
	for lineID, idxSet := range touched {
		idxs := make([]int, 0, len(idxSet))
		for i := range idxSet {
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)
		start := idxs[0]
		prev := idxs[0]
		for _, i := range idxs[1:] {
			if i != prev+1 {
				spans = append(spans, RouteSpan{AnnotationID: lineID, StartIndex: start, EndIndex: prev})
				start = i
			}
			prev = i
		}
		spans = append(spans, RouteSpan{AnnotationID: lineID, StartIndex: start, EndIndex: prev})
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].AnnotationID != spans[j].AnnotationID {
			return spans[i].AnnotationID < spans[j].AnnotationID
		}
		return spans[i].StartIndex < spans[j].StartIndex
	})
	if spans == nil {
		spans = []RouteSpan{}
	}
	return spans
}

// buildAffectsSummary renders "CP 4-CP 7 - Aid 2, Aid 3 +2 more - 3 stations",
// truncated at 80 runes.
func buildAffectsSummary(checkpoints, locations []AffectedItem, stationCount int) string {
	var parts []string

	if len(checkpoints) > 0 {
		parts = append(parts, checkpointRunSummary(checkpoints))
	}
	if len(locations) > 0 {
		names := make([]string, 0, len(locations))
		for _, l := range locations {
			n := l.ShortName
			if n == "" {
				n = l.Label
			}
			names = append(names, n)
		}
		const maxNames = 3
		if len(names) > maxNames {
			parts = append(parts, fmt.Sprintf("%s +%d more", strings.Join(names[:maxNames], ", "), len(names)-maxNames))
		} else {
			parts = append(parts, strings.Join(names, ", "))
		}
	}
	if stationCount > 0 {
		noun := "stations"
		if stationCount == 1 {
			noun = "station"
		}
		parts = append(parts, fmt.Sprintf("%d %s", stationCount, noun))
	}

	summary := strings.Join(parts, " · ")
	const maxRunes = 80
	r := []rune(summary)
	if len(r) > maxRunes {
		summary = string(r[:maxRunes-1]) + "…"
	}
	return summary
}

func checkpointRunSummary(checkpoints []AffectedItem) string {
	label := func(it AffectedItem) string {
		if it.ShortName != "" {
			return it.ShortName
		}
		return it.Label
	}
	if len(checkpoints) == 1 {
		return label(checkpoints[0])
	}
	// Contiguous-by-Seq run collapses to "first-last"; a gap starts a new run.
	var runs []string
	runStart := 0
	for i := 1; i <= len(checkpoints); i++ {
		if i == len(checkpoints) || checkpoints[i].Seq != checkpoints[i-1].Seq+1 {
			if runStart == i-1 {
				runs = append(runs, label(checkpoints[runStart]))
			} else {
				runs = append(runs, label(checkpoints[runStart])+"–"+label(checkpoints[i-1]))
			}
			runStart = i
		}
	}
	return strings.Join(runs, ", ")
}
