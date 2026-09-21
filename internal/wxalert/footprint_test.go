package wxalert

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

var gr = LatLon{Lat: 42.9634, Lon: -85.6681}

// boxAt places a 2*halfMiles box whose nearest edge is distMiles from gr
// along bearing, as a closed lon/lat ring.
func boxAt(center LatLon, bearingDeg, distMiles, halfMiles float64) []LatLon {
	near := destPoint(center, bearingDeg, distMiles+halfMiles)
	return squareRingMiles(near, halfMiles)
}

func squareRingMiles(center LatLon, halfMiles float64) []LatLon {
	n := destPoint(center, 0, halfMiles)
	s := destPoint(center, 180, halfMiles)
	// Build a simple axis-aligned (in lat/lon) box using the half-mile as a
	// degree offset approximation via destPoint on each corner from centre.
	nw := destPoint(destPoint(center, 0, halfMiles), 270, halfMiles)
	ne := destPoint(destPoint(center, 0, halfMiles), 90, halfMiles)
	se := destPoint(destPoint(center, 180, halfMiles), 90, halfMiles)
	sw := destPoint(destPoint(center, 180, halfMiles), 270, halfMiles)
	_ = n
	_ = s
	return []LatLon{nw, ne, se, sw, nw}
}

func polygonAlert(event string, sev Severity, ring []LatLon) Alert {
	rings := [][]LatLon{ring}
	return Alert{
		ID: "test", Event: event, Severity: sev, Status: StatusActual,
		Geometry: &Geometry{Type: "Polygon", Rings: rings, BBox: bboxOf(ring)},
	}
}

func zoneAlert(event string, ugc ...string) Alert {
	return Alert{ID: "test-zone", Event: event, Status: StatusActual, UGC: ugc}
}

func TestFootprintProximityPolygon(t *testing.T) {
	buffer := 10.0
	own := &gr

	tests := []struct {
		name string
		ring []LatLon
		want Proximity
	}{
		{"contains own station", squareRingMiles(gr, 20), ProximityIn},
		{"edge 5 mi east", boxAt(gr, 90, 5, 3), ProximityIn},
		{"edge just inside buffer", boxAt(gr, 90, 9.9, 3), ProximityIn},
		{"edge just past buffer", boxAt(gr, 90, 10.5, 3), ProximityNear},
		{"edge 15 mi", boxAt(gr, 0, 15, 3), ProximityNear},
		{"edge just inside 2x buffer", boxAt(gr, 180, 19.9, 3), ProximityNear},
		{"edge just past 2x buffer", boxAt(gr, 180, 20.5, 3), ProximityFar},
		{"edge 25 mi", boxAt(gr, 270, 25, 3), ProximityFar},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Build(Inputs{BufferMiles: buffer, Own: own, OwnSource: "gps", Now: time.Now()})
			m := f.Classify(polygonAlert("Tornado Warning", SeverityExtreme, tt.ring), nil)
			if m.Proximity != tt.want {
				t.Errorf("Classify() proximity = %v, want %v (dist=%v)", m.Proximity, tt.want, m.DistanceMiles)
			}
		})
	}
}

func TestFootprintProximityBufferExtremes(t *testing.T) {
	ring := boxAt(gr, 0, 15, 3) // edge 15 mi north
	now := time.Now()

	small := Build(Inputs{BufferMiles: 2, Own: &gr, Now: now})
	if m := small.Classify(polygonAlert("Flood Warning", SeveritySevere, ring), nil); m.Proximity != ProximityFar {
		t.Errorf("buffer=2: proximity = %v, want far", m.Proximity)
	}

	big := Build(Inputs{BufferMiles: 50, Own: &gr, Now: now})
	if m := big.Classify(polygonAlert("Flood Warning", SeveritySevere, ring), nil); m.Proximity != ProximityIn {
		t.Errorf("buffer=50: proximity = %v, want in", m.Proximity)
	}
}

func TestFootprintProximityDegenerateAndUnclosedRings(t *testing.T) {
	now := time.Now()
	f := Build(Inputs{BufferMiles: 10, Own: &gr, Now: now})

	near5 := destPoint(gr, 45, 5)
	degenerate := []LatLon{near5, near5, near5}
	if m := f.Classify(polygonAlert("Flood Warning", SeveritySevere, degenerate), nil); m.Proximity != ProximityIn {
		t.Errorf("degenerate polygon 5mi away: proximity = %v, want in (treated as a point)", m.Proximity)
	}

	ring := squareRingMiles(gr, 3)
	unclosed := ring[:len(ring)-1]
	if m := f.Classify(polygonAlert("Flood Warning", SeveritySevere, unclosed), nil); m.Proximity != ProximityIn {
		t.Errorf("unclosed ring containing own station: proximity = %v, want in", m.Proximity)
	}
}

func TestFootprintProximityZoneOnly(t *testing.T) {
	extra := []string{"MIC139"}
	near := []string{"MIC005"}
	now := time.Now()

	tests := []struct {
		name string
		ugc  []string
		want Proximity
	}{
		{"county match", []string{"MIC081"}, ProximityIn},
		{"extra zone match", []string{"MIC139"}, ProximityIn},
		{"case-insensitive", []string{"mic081"}, ProximityIn},
		{"near-zone adjacency", []string{"MIC005"}, ProximityNear},
		{"far zone", []string{"TNZ005"}, ProximityFar},
		{"no location at all", nil, ProximityFar},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Build(Inputs{BufferMiles: 10, ExtraZones: extra, NearUGC: near, Own: &gr, Now: now,
				ZoneLookup: map[string][]string{"own": {"MIC081"}}})
			m := f.Classify(zoneAlert("Flood Warning", tt.ugc...), nil)
			if m.Proximity != tt.want {
				t.Errorf("Classify() proximity = %v, want %v", m.Proximity, tt.want)
			}
		})
	}
}

func TestFootprintProximityZoneOnlyEmptyNeverCrashes(t *testing.T) {
	f := Build(Inputs{BufferMiles: 10, Own: &gr, Now: time.Now()})
	a := Alert{ID: "empty", Event: "Flood Warning", Status: StatusActual}
	m := f.Classify(a, nil)
	if m.Proximity != ProximityFar {
		t.Errorf("alert with no UGC and no polygon: proximity = %v, want far", m.Proximity)
	}
	if a.HasLocation() {
		t.Errorf("alert with no UGC and no polygon reports HasLocation()")
	}
}

func TestFootprintPolygonAuthoritativeOverUGC(t *testing.T) {
	// Decision: when the alert carries its own polygon, that geometry
	// decides — even if the alert also lists a UGC code that is in the IN
	// set.
	now := time.Now()
	f := Build(Inputs{BufferMiles: 10, Own: &gr, Now: now, ZoneLookup: map[string][]string{"own": {"MIC081"}}})
	far := boxAt(gr, 270, 30, 3)
	a := polygonAlert("Flood Warning", SeveritySevere, far)
	a.UGC = []string{"MIC081"}
	m := f.Classify(a, nil)
	if m.Proximity != ProximityFar {
		t.Errorf("polygon-present alert with matching UGC: proximity = %v, want far (polygon wins)", m.Proximity)
	}
}

func TestFootprintZoneOnlyViaCachedRing(t *testing.T) {
	now := time.Now()
	// Own position 5 miles from centre; the alert only lists a UGC we do not
	// have in our IN/NEAR sets, but we do have its ring cached.
	ring := squareRingMiles(gr, 20)
	cache := map[string]*ZoneRecord{
		"MIZ999": {UGC: "MIZ999", rings: [][]LatLon{ring}},
	}
	f := Build(Inputs{BufferMiles: 10, Own: &gr, Now: now})
	m := f.Classify(zoneAlert("Flood Warning", "MIZ999"), cache)
	if m.Proximity != ProximityIn {
		t.Errorf("zone-only alert with a cached ring containing own station: proximity = %v, want in", m.Proximity)
	}
	if m.GeometrySource != "zone" {
		t.Errorf("GeometrySource = %q, want zone", m.GeometrySource)
	}
}

func TestFootprintZoneOnlyUncachedInIsGeometrySourceNone(t *testing.T) {
	now := time.Now()
	f := Build(Inputs{BufferMiles: 10, ExtraZones: []string{"MIC081"}, Own: &gr, Now: now})
	m := f.Classify(zoneAlert("Flood Watch", "MIC081"), nil)
	if m.Proximity != ProximityIn {
		t.Fatalf("proximity = %v, want in", m.Proximity)
	}
	if m.GeometrySource != "none" {
		t.Errorf("GeometrySource = %q, want none (NWS published no polygon and we have no cached zone ring)", m.GeometrySource)
	}
}

func TestFootprintInputsEmpty(t *testing.T) {
	f := Build(Inputs{BufferMiles: 10, Now: time.Now()})
	if !f.Empty() {
		t.Errorf("Empty() = false, want true for an input with nothing at all")
	}
	if got := f.Summary().OwnStation; got != "none" {
		t.Errorf("OwnStation = %q, want none", got)
	}
	m := f.Classify(polygonAlert("Tornado Warning", SeverityExtreme, squareRingMiles(gr, 1)), nil)
	if m.Proximity != ProximityFar {
		t.Errorf("empty footprint classified a polygon alert as %v, want far", m.Proximity)
	}
}

func TestFootprintInputsOwnOnly(t *testing.T) {
	f := Build(Inputs{BufferMiles: 10, Own: &gr, OwnSource: "config", Now: time.Now()})
	if f.Empty() {
		t.Errorf("Empty() = true, want false when own position is set")
	}
	s, w, n, e := f.Bounds()
	if s > gr.Lat || n < gr.Lat || w > gr.Lon || e < gr.Lon {
		t.Errorf("Bounds() = (%v,%v,%v,%v) does not contain own station %v", s, w, n, e, gr)
	}
}

func pointGeoJSON(p LatLon) string {
	return `{"type":"Point","coordinates":[` + f64(p.Lon) + "," + f64(p.Lat) + `]}`
}

func lineGeoJSON(pts []LatLon) string {
	s := `{"type":"LineString","coordinates":[`
	for i, p := range pts {
		if i > 0 {
			s += ","
		}
		s += "[" + f64(p.Lon) + "," + f64(p.Lat) + "]"
	}
	return s + `]}`
}

func areaGeoJSON(ring []LatLon) string {
	s := `{"type":"Polygon","coordinates":[[`
	for i, p := range ring {
		if i > 0 {
			s += ","
		}
		s += "[" + f64(p.Lon) + "," + f64(p.Lat) + "]"
	}
	return s + `]]}`
}

func f64(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestFootprintAnnotationCategoriesAndExclusions(t *testing.T) {
	now := time.Now()
	included := []store.Annotation{
		{ID: "cp1", Type: "point", Category: "checkpoint", Label: "CP 1", Geometry: pointGeoJSON(destPoint(gr, 0, 1))},
		{ID: "aid1", Type: "point", Category: "aid", Label: "Aid 1", Geometry: pointGeoJSON(destPoint(gr, 90, 1))},
	}
	excluded := []store.Annotation{
		{ID: "haz1", Type: "point", Category: "hazard", Label: "Hazard", Geometry: pointGeoJSON(destPoint(gr, 180, 1))},
		{ID: "inc1", Type: "point", Category: "incident", Label: "Incident", Geometry: pointGeoJSON(destPoint(gr, 270, 1))},
		{ID: "res1", Type: "point", Category: "resource", Label: "Resource", Geometry: pointGeoJSON(destPoint(gr, 45, 1))},
	}
	all := append(append([]store.Annotation{}, included...), excluded...)

	f := Build(Inputs{BufferMiles: 10, Annotations: all, Own: &gr, Now: now})
	if got := len(f.samples); got != len(included) {
		t.Fatalf("samples = %d, want %d (hazard/incident/resource must be excluded)", got, len(included))
	}
	for _, s := range f.samples {
		if s.refID == "haz1" || s.refID == "inc1" || s.refID == "res1" {
			t.Errorf("excluded-category annotation %q made it into samples", s.refID)
		}
	}
}

func TestFootprintExpiredAndClosedAnnotationsExcluded(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	anns := []store.Annotation{
		{ID: "expired", Type: "point", Category: "aid", Label: "Old Aid", Geometry: pointGeoJSON(gr), ExpiresAt: &past},
		{ID: "notyet", Type: "point", Category: "aid", Label: "Future Aid", Geometry: pointGeoJSON(gr), ExpiresAt: &future},
		{ID: "resolved", Type: "point", Category: "aid", Label: "Resolved Aid", Geometry: pointGeoJSON(gr), Status: "resolved"},
		{ID: "closed", Type: "point", Category: "aid", Label: "Closed Aid", Geometry: pointGeoJSON(gr), Status: "closed"},
		{ID: "ok", Type: "point", Category: "aid", Label: "Active Aid", Geometry: pointGeoJSON(gr)},
	}
	f := Build(Inputs{BufferMiles: 10, Annotations: anns, Now: now})
	ids := map[string]bool{}
	for _, s := range f.samples {
		ids[s.refID] = true
	}
	// "notyet" has not expired, so it stays; the already-expired and
	// resolved/closed ones must not.
	if ids["expired"] || ids["resolved"] || ids["closed"] {
		t.Errorf("expired/resolved/closed annotations were kept: %v", ids)
	}
	if !ids["notyet"] || !ids["ok"] {
		t.Errorf("active annotations were dropped: %v", ids)
	}
	if len(f.samples) != 2 {
		t.Errorf("samples = %d, want 2 (notyet + ok)", len(f.samples))
	}
}

func TestFootprintNetScoping(t *testing.T) {
	now := time.Now()
	anns := []store.Annotation{
		{ID: "mine", Type: "point", Category: "aid", Label: "Mine", Geometry: pointGeoJSON(gr), NetID: "net-1"},
		{ID: "other", Type: "point", Category: "aid", Label: "Other net", Geometry: pointGeoJSON(gr), NetID: "net-2"},
		{ID: "global", Type: "point", Category: "aid", Label: "No net", Geometry: pointGeoJSON(gr)},
	}
	f := Build(Inputs{NetID: "net-1", BufferMiles: 10, Annotations: anns, Now: now})
	ids := map[string]bool{}
	for _, s := range f.samples {
		ids[s.refID] = true
	}
	if !ids["mine"] || ids["other"] {
		t.Errorf("net scoping wrong: got %v, want mine but not other", ids)
	}
}

func TestFootprintPositionAgeFiltering(t *testing.T) {
	now := time.Now()
	positions := []PositionInput{
		{Kind: "roster", ID: "OLD", Lat: gr.Lat, Lon: gr.Lon, HeardAt: now.Add(-PositionMaxAge - time.Second)},
		{Kind: "roster", ID: "FRESH", Lat: gr.Lat, Lon: gr.Lon, HeardAt: now.Add(-PositionMaxAge + time.Minute)},
		{Kind: "roster", ID: "ZERO", Lat: 0, Lon: 0},
		{Kind: "roster", ID: "BADLAT", Lat: 91, Lon: 0},
	}
	f := Build(Inputs{BufferMiles: 10, Positions: positions, Now: now})
	ids := map[string]bool{}
	for _, s := range f.stations {
		ids[s.id] = true
	}
	if ids["OLD"] || ids["ZERO"] || ids["BADLAT"] {
		t.Errorf("stale/sentinel/invalid positions were kept: %v", ids)
	}
	if !ids["FRESH"] {
		t.Errorf("fresh position was dropped: %v", ids)
	}
}

func TestFootprintMalformedAnnotationGeometrySkipped(t *testing.T) {
	anns := []store.Annotation{
		{ID: "bad", Type: "point", Category: "aid", Label: "Bad", Geometry: "{not json"},
		{ID: "ok", Type: "point", Category: "aid", Label: "Ok", Geometry: pointGeoJSON(gr)},
	}
	f := Build(Inputs{BufferMiles: 10, Annotations: anns, Now: time.Now()})
	if len(f.samples) != 1 || f.samples[0].refID != "ok" {
		t.Fatalf("samples = %+v, want only the well-formed annotation", f.samples)
	}
}

func TestFootprintAffectsPolygonCheckpointsAndStations(t *testing.T) {
	now := time.Now()
	// 8 checkpoints spaced 5 miles apart due east of gr; CP4-CP7 (seq 4-7)
	// fall inside a box covering miles 15-35 east.
	var anns []store.Annotation
	seqs := map[string]int{}
	for i := 1; i <= 8; i++ {
		id := "cp" + itoa(i)
		p := destPoint(gr, 90, float64(i)*5)
		anns = append(anns, store.Annotation{ID: id, Type: "point", Category: "checkpoint", Label: "CP " + itoa(i), ShortName: "CP " + itoa(i), Geometry: pointGeoJSON(p)})
		seqs[id] = i
	}
	anns = append(anns, store.Annotation{ID: "aid2", Type: "point", Category: "aid", Label: "Aid 2", ShortName: "Aid 2", Geometry: pointGeoJSON(destPoint(gr, 90, 25))})
	anns = append(anns, store.Annotation{ID: "start", Type: "point", Category: "start", Label: "Start", Geometry: pointGeoJSON(gr)}) // outside the box (0 mi east)

	positions := []PositionInput{
		{Kind: "tracked", ID: "KD8ABC-9", Lat: destPoint(gr, 90, 25).Lat, Lon: destPoint(gr, 90, 25).Lon},
		{Kind: "roster", ID: "W8XYZ", CheckInID: "ci-1", Lat: destPoint(gr, 90, 25).Lat, Lon: destPoint(gr, 90, 25).Lon},
		{Kind: "roster", ID: "K8AAA", CheckInID: "ci-2", Lat: gr.Lat, Lon: gr.Lon}, // outside the box
	}

	// Box covering miles 18-32 east of gr (clear of CP3@15mi and CP8@40mi,
	// clearly containing CP4..CP7 at 20/25/30/35... note CP7 is at 35mi, so
	// extend to 37 to keep it inside with the 1mi buffer used below).
	box := []LatLon{
		destPoint(destPoint(gr, 90, 18), 0, 5),
		destPoint(destPoint(gr, 90, 37), 0, 5),
		destPoint(destPoint(gr, 90, 37), 180, 5),
		destPoint(destPoint(gr, 90, 18), 180, 5),
		destPoint(destPoint(gr, 90, 18), 0, 5),
	}

	f := Build(Inputs{BufferMiles: 1, Annotations: anns, CheckpointSeq: seqs, Positions: positions, Now: now})
	m := f.Classify(polygonAlert("Tornado Warning", SeverityExtreme, box), nil)
	if m.Proximity != ProximityIn {
		t.Fatalf("proximity = %v, want in", m.Proximity)
	}
	var seqsGot []int
	for _, c := range m.Affects.Checkpoints {
		seqsGot = append(seqsGot, c.Seq)
	}
	wantSeqs := []int{4, 5, 6, 7}
	if len(seqsGot) != len(wantSeqs) {
		t.Fatalf("checkpoint seqs = %v, want %v", seqsGot, wantSeqs)
	}
	for i, s := range wantSeqs {
		if seqsGot[i] != s {
			t.Errorf("checkpoint[%d].Seq = %d, want %d", i, seqsGot[i], s)
		}
	}
	if len(m.Affects.Locations) != 1 || m.Affects.Locations[0].ID != "aid2" {
		t.Errorf("Locations = %v, want just Aid 2", m.Affects.Locations)
	}
	if len(m.Affects.Stations) != 2 {
		t.Errorf("Stations = %v, want 2 (K8AAA excluded)", m.Affects.Stations)
	}
	if len(m.Affects.CheckpointSeqRange) != 2 || m.Affects.CheckpointSeqRange[0] != 4 || m.Affects.CheckpointSeqRange[1] != 7 {
		t.Errorf("CheckpointSeqRange = %v, want [4 7]", m.Affects.CheckpointSeqRange)
	}
}

func TestFootprintAffectsZoneOnly(t *testing.T) {
	now := time.Now()
	var anns []store.Annotation
	lookup := map[string][]string{}
	seqs := map[string]int{}
	for i := 1; i <= 5; i++ {
		id := "cp" + itoa(i)
		p := destPoint(gr, 90, float64(i)*5)
		anns = append(anns, store.Annotation{ID: id, Type: "point", Category: "checkpoint", Label: "CP " + itoa(i), Geometry: pointGeoJSON(p)})
		seqs[id] = i
		if i >= 2 && i <= 5 {
			lookup[id] = []string{"MIC081"}
		} else {
			lookup[id] = []string{"MIC999"}
		}
	}
	f := Build(Inputs{BufferMiles: 10, Annotations: anns, CheckpointSeq: seqs, ExtraZones: []string{"MIC081"}, ZoneLookup: lookup, Now: now})
	m := f.Classify(zoneAlert("Flood Watch", "MIC081"), nil)
	if m.Proximity != ProximityIn {
		t.Fatalf("proximity = %v, want in", m.Proximity)
	}
	if len(m.Affects.Checkpoints) != 4 {
		t.Fatalf("checkpoints = %v, want 4 (CP2-CP5)", m.Affects.Checkpoints)
	}
}

func TestFootprintAntimeridian(t *testing.T) {
	own := LatLon{Lat: 51.40, Lon: 179.95}
	now := time.Now()
	f := Build(Inputs{BufferMiles: 10, Own: &own, Now: now})

	crossing := []LatLon{
		{Lat: 51.0, Lon: 179.0}, {Lat: 51.0, Lon: -179.0}, {Lat: 51.8, Lon: -179.0}, {Lat: 51.8, Lon: 179.0}, {Lat: 51.0, Lon: 179.0},
	}
	if m := f.Classify(polygonAlert("Tornado Warning", SeverityExtreme, crossing), nil); m.Proximity != ProximityIn {
		t.Errorf("antimeridian-crossing polygon containing own station: proximity = %v, want in", m.Proximity)
	}

	far := boxAt(own, 90, 32, 0.1) // ~32 mi east across the line
	if m := f.Classify(polygonAlert("Tornado Warning", SeverityExtreme, far), nil); m.Proximity != ProximityFar {
		t.Errorf("32mi across the antimeridian: proximity = %v, want far", m.Proximity)
	}

	s, w, n, e := f.Bounds()
	_ = s
	_ = n
	if !(BBox{West: w, East: e}).CrossesAntimeridian() {
		t.Errorf("Bounds() west=%v east=%v does not cross the antimeridian as expected", w, e)
	}
}

func TestFootprintCrossesAntimeridianMethod(t *testing.T) {
	f := Build(Inputs{BufferMiles: 10, Own: &LatLon{Lat: 51.4, Lon: 179.95}, Now: time.Now()})
	if !f.CrossesAntimeridian() {
		t.Errorf("CrossesAntimeridian() = false, want true near the date line with a 10mi buffer")
	}
	f2 := Build(Inputs{BufferMiles: 10, Own: &gr, Now: time.Now()})
	if f2.CrossesAntimeridian() {
		t.Errorf("CrossesAntimeridian() = true for an ordinary footprint")
	}
}

func TestFootprintBoundaryAreaAnnotation(t *testing.T) {
	now := time.Now()
	boundary := squareRingMiles(destPoint(gr, 90, 30), 10)
	ann := store.Annotation{ID: "b1", Type: "area", Category: "boundary", Label: "Ops Area", Geometry: areaGeoJSON(boundary)}
	f := Build(Inputs{BufferMiles: 5, Annotations: []store.Annotation{ann}, Now: now})
	if len(f.areas) != 1 || f.areas[0].id != "b1" {
		t.Fatalf("areas = %+v, want the boundary polygon parsed", f.areas)
	}

	// An alert polygon overlapping the boundary, 30mi from own/other samples,
	// must still be In because the boundary itself is part of the footprint.
	f2 := Build(Inputs{BufferMiles: 5, Annotations: []store.Annotation{ann}, Now: now})
	overlap := squareRingMiles(destPoint(gr, 90, 30), 8)
	m := f2.Classify(polygonAlert("Flood Warning", SeveritySevere, overlap), nil)
	if m.Proximity != ProximityIn {
		t.Errorf("alert overlapping the boundary area: proximity = %v, want in", m.Proximity)
	}

	// A malformed area annotation is skipped, not fatal.
	bad := store.Annotation{ID: "bad-area", Type: "area", Category: "boundary", Label: "Bad", Geometry: "{not json"}
	f3 := Build(Inputs{BufferMiles: 5, Annotations: []store.Annotation{bad}, Now: now})
	if len(f3.areas) != 0 {
		t.Errorf("malformed area annotation was kept: %+v", f3.areas)
	}
}

func TestFootprintZoneTypeOfHelper(t *testing.T) {
	tests := []struct{ ugc, want string }{
		{"MIC081", "county"},
		{"MIZ057", "forecast"},
		{"MI", "unknown"},
		{"MIX081", "unknown"},
	}
	for _, tt := range tests {
		if got := zoneTypeOf(tt.ugc); got != tt.want {
			t.Errorf("zoneTypeOf(%q) = %q, want %q", tt.ugc, got, tt.want)
		}
	}
}

func TestParseLineGeometryInvalid(t *testing.T) {
	if _, ok := parseLineGeometry("{not json"); ok {
		t.Errorf("malformed JSON: want ok=false")
	}
	if _, ok := parseLineGeometry(pointGeoJSON(gr)); ok {
		t.Errorf("wrong geometry type (Point): want ok=false")
	}
	if _, ok := parseLineGeometry(`{"type":"LineString","coordinates":"nope"}`); ok {
		t.Errorf("malformed coordinates: want ok=false")
	}
}

func TestParsePointAndAreaGeometryInvalid(t *testing.T) {
	if _, ok := parsePointGeometry("{not json"); ok {
		t.Errorf("parsePointGeometry malformed JSON: want ok=false")
	}
	if _, ok := parsePointGeometry(`{"type":"Point","coordinates":"nope"}`); ok {
		t.Errorf("parsePointGeometry malformed coordinates: want ok=false")
	}
	if _, ok := parseAreaGeometry(lineGeoJSON([]LatLon{gr, gr})); ok {
		t.Errorf("parseAreaGeometry wrong type (LineString): want ok=false")
	}
	if _, ok := parseAreaGeometry(`{"type":"Polygon","coordinates":[]}`); ok {
		t.Errorf("parseAreaGeometry empty coordinates: want ok=false")
	}
}

func TestClassifyZoneOnlyWithZoneCachePresent(t *testing.T) {
	now := time.Now()
	f := Build(Inputs{BufferMiles: 10, ExtraZones: []string{"MIC081"}, Own: &gr, Now: now})
	cache := map[string]*ZoneRecord{"MIC081": {UGC: "MIC081", Name: "Kent"}}
	m := f.Classify(zoneAlert("Flood Watch", "MIC081"), cache)
	if m.Proximity != ProximityIn || m.GeometrySource != "zone" {
		t.Errorf("Classify() = %+v, want in/zone (cache hit for the matched IN zone)", m)
	}

	cacheMiss := map[string]*ZoneRecord{"MIC999": {UGC: "MIC999"}}
	m2 := f.Classify(zoneAlert("Flood Watch", "MIC081"), cacheMiss)
	if m2.Proximity != ProximityIn || m2.GeometrySource != "none" {
		t.Errorf("Classify() = %+v, want in/none (cache present but not for this zone)", m2)
	}
}

func TestBuildAffectsSummaryVariants(t *testing.T) {
	single := []AffectedItem{{Kind: "checkpoint", Label: "CP 4", Seq: 4}}
	if got := checkpointRunSummary(single); got != "CP 4" {
		t.Errorf("checkpointRunSummary(single) = %q, want CP 4", got)
	}

	locations := []AffectedItem{
		{Label: "Aid 1"}, {Label: "Aid 2"}, {Label: "Aid 3"}, {Label: "Water 1"}, {Label: "Water 2"},
	}
	summary := buildAffectsSummary(nil, locations, 3)
	if !contains(summary, "+2 more") {
		t.Errorf("buildAffectsSummary(5 locations) = %q, want a +2 more suffix", summary)
	}
	if !contains(summary, "3 stations") {
		t.Errorf("buildAffectsSummary = %q, want a station count", summary)
	}

	oneStation := buildAffectsSummary(nil, nil, 1)
	if oneStation != "1 station" {
		t.Errorf("buildAffectsSummary(1 station) = %q, want '1 station'", oneStation)
	}

	long := buildAffectsSummary(nil, []AffectedItem{{Label: string(make([]byte, 100))}}, 0)
	if len([]rune(long)) > 80 {
		t.Errorf("buildAffectsSummary result is %d runes, want <= 80", len([]rune(long)))
	}
}

func TestFootprintBuildRouteSpansWithGap(t *testing.T) {
	touched := map[string]map[int]bool{
		"route1": {0: true, 1: true, 2: true, 5: true, 6: true},
	}
	spans := buildRouteSpans(touched)
	if len(spans) != 2 {
		t.Fatalf("spans = %+v, want 2 (a gap between index 2 and 5)", spans)
	}
	if spans[0].StartIndex != 0 || spans[0].EndIndex != 2 {
		t.Errorf("spans[0] = %+v, want 0-2", spans[0])
	}
	if spans[1].StartIndex != 5 || spans[1].EndIndex != 6 {
		t.Errorf("spans[1] = %+v, want 5-6", spans[1])
	}

	if got := buildRouteSpans(map[string]map[int]bool{}); len(got) != 0 {
		t.Errorf("buildRouteSpans(empty) = %v, want empty non-nil slice", got)
	}
}

func TestFootprintSamplePointSpacingAndDedup(t *testing.T) {
	now := time.Now()
	var line []LatLon
	for i := 0; i <= 24; i++ {
		line = append(line, destPoint(gr, 90, float64(i)*2)) // 48 miles, 25 vertices, 2mi apart
	}
	ann := store.Annotation{ID: "route1", Type: "line", Category: "route", Label: "Route", Geometry: lineGeoJSON(line)}
	f := Build(Inputs{BufferMiles: 10, Annotations: []store.Annotation{ann}, Now: now})

	var routeSamples []sample
	for _, s := range f.samples {
		if s.kind == "route" {
			routeSamples = append(routeSamples, s)
		}
	}
	if len(routeSamples) < 20 {
		t.Fatalf("route samples = %d, want >= 20 for a 48mi route sampled every 2mi", len(routeSamples))
	}
	for i := 0; i+1 < len(routeSamples); i++ {
		d := haversineMiles(routeSamples[i].pt, routeSamples[i+1].pt)
		if d > 2.5 {
			t.Errorf("gap between route samples %d,%d = %.2f mi, want <= 2.5", i, i+1, d)
		}
	}
	if got := f.summary.RouteMiles; got < 47 || got > 49 {
		t.Errorf("RouteMiles = %v, want ~48", got)
	}
}

// A NEAR/FAR match never calls buildAffects, so its zero-value Affects used to
// marshal every slice as JSON null. The frontend's WxAffects declares those
// "never nil" and reads .length on them, so one null froze the whole alert
// detail. Every Classify result must carry empty slices, not nil.
func TestClassifyAffectsSlicesNeverNil(t *testing.T) {
	check := func(t *testing.T, where string, m Match) {
		t.Helper()
		if m.Affects.Checkpoints == nil {
			t.Errorf("%s: Affects.Checkpoints is nil, want empty slice", where)
		}
		if m.Affects.Locations == nil {
			t.Errorf("%s: Affects.Locations is nil, want empty slice", where)
		}
		if m.Affects.Stations == nil {
			t.Errorf("%s: Affects.Stations is nil, want empty slice", where)
		}
		if m.Affects.CheckpointSeqRange == nil {
			t.Errorf("%s: Affects.CheckpointSeqRange is nil, want empty slice", where)
		}
		if m.Affects.RouteSpans == nil {
			t.Errorf("%s: Affects.RouteSpans is nil, want empty slice", where)
		}
		b, err := json.Marshal(m.Affects)
		if err != nil {
			t.Fatalf("%s: marshal Affects: %v", where, err)
		}
		if bytes.Contains(b, []byte(":null")) {
			t.Errorf("%s: Affects marshalled a null: %s", where, b)
		}
	}

	f := Build(Inputs{BufferMiles: 10, Own: &gr, OwnSource: "gps", Now: time.Now()})

	// Polygon alerts at each proximity band.
	for _, tt := range []struct {
		name string
		ring []LatLon
	}{
		{"polygon in", boxAt(gr, 90, 5, 3)},
		{"polygon near", boxAt(gr, 90, 15, 3)},
		{"polygon far", boxAt(gr, 90, 25, 3)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			check(t, tt.name, f.Classify(polygonAlert("Severe Thunderstorm Warning", SeveritySevere, tt.ring), nil))
		})
	}

	// Zone-only alerts, with and without a zone cache.
	t.Run("zone only uncached", func(t *testing.T) {
		check(t, "zone only uncached", f.Classify(zoneAlert("Heat Advisory", "MIZ999"), nil))
	})
	t.Run("zone only empty cache", func(t *testing.T) {
		check(t, "zone only empty cache", f.Classify(zoneAlert("Heat Advisory", "MIZ999"), map[string]*ZoneRecord{}))
	})

	// An empty footprint (nothing of ours anywhere) must still be non-nil.
	t.Run("empty footprint", func(t *testing.T) {
		empty := Build(Inputs{BufferMiles: 10, Now: time.Now()})
		check(t, "empty footprint", empty.Classify(polygonAlert("Tornado Warning", SeverityExtreme, boxAt(gr, 90, 5, 3)), nil))
	})
}

// "Affects (yours)" answers "what of ours is inside this alert", so it must be
// a containment test. It used to reuse the IN/NEAR buffer, which listed every
// asset within BufferMiles *outside* the polygon: on a real 10-mile-buffer
// footprint, 49 of 49 listed course points were outside the alert.
func TestAffectsCountsOnlyWhatIsInsideThePolygon(t *testing.T) {
	// Course points at 0, 6 and 20 miles east of gr.
	inside := destPoint(gr, 90, 0)
	justOutside := destPoint(gr, 90, 6)
	wellOutside := destPoint(gr, 90, 20)
	anns := []store.Annotation{
		{ID: "a-in", Category: "aid", Type: "point", Label: "Inside", Geometry: pointGeoJSON(inside)},
		{ID: "a-near", Category: "aid", Type: "point", Label: "Six miles out", Geometry: pointGeoJSON(justOutside)},
		{ID: "a-far", Category: "aid", Type: "point", Label: "Twenty miles out", Geometry: pointGeoJSON(wellOutside)},
	}
	f := Build(Inputs{BufferMiles: 10, Annotations: anns, Now: time.Now()})

	// A 3-mile box centred on gr: contains only "Inside".
	m := f.Classify(polygonAlert("Severe Thunderstorm Warning", SeveritySevere, squareRingMiles(gr, 3)), nil)
	if m.Proximity != ProximityIn {
		t.Fatalf("Proximity = %v, want in", m.Proximity)
	}
	got := []string{}
	for _, l := range m.Affects.Locations {
		got = append(got, l.Label)
	}
	if len(got) != 1 || got[0] != "Inside" {
		t.Errorf("Affects.Locations = %v, want only [Inside] — the 6-mile point is within the buffer but outside the alert", got)
	}
	if m.Affects.Summary == "" {
		t.Errorf("Summary should still name what it found, got %q", m.Affects.Summary)
	}
}

// An alert can legitimately be IN (its edge is inside the buffer) while
// touching nothing of ours. That must read as an empty Affects, not as the
// whole course.
func TestAffectsEmptyWhenAlertIsNearButTouchesNothing(t *testing.T) {
	anns := []store.Annotation{{ID: "a1", Category: "aid", Type: "point", Label: "Aid 1", Geometry: pointGeoJSON(gr)}}
	f := Build(Inputs{BufferMiles: 10, Annotations: anns, Now: time.Now()})

	// A small box 5 miles east: inside the 10-mile buffer, so the alert is IN,
	// but the aid station is not inside the polygon.
	m := f.Classify(polygonAlert("Flood Warning", SeveritySevere, boxAt(gr, 90, 5, 1)), nil)
	if m.Proximity != ProximityIn {
		t.Fatalf("Proximity = %v, want in", m.Proximity)
	}
	if len(m.Affects.Locations) != 0 {
		t.Errorf("Affects.Locations = %+v, want empty", m.Affects.Locations)
	}
	if m.Affects.Summary != "" {
		t.Errorf("Summary = %q, want empty", m.Affects.Summary)
	}
}

// The real case that surfaced this: a Special Weather Statement over Coffee /
// Grundy counties (east of the course) listed all 49 course points as
// affected. The nearest one is 10.3 miles from the polygon — inside the
// 10-mile buffer, outside the alert. NWS drew the polygon; none of the course
// is in it.
func TestAffectsRealSpecialWeatherStatementTouchesNothing(t *testing.T) {
	ring := []LatLon{
		{Lat: 35.34, Lon: -86.03},
		{Lat: 35.31, Lon: -86},
		{Lat: 35.32, Lon: -85.98},
		{Lat: 35.29, Lon: -85.92},
		{Lat: 35.22, Lon: -85.86},
		{Lat: 35.16, Lon: -85.87},
		{Lat: 35.21, Lon: -86.19},
		{Lat: 35.35, Lon: -86.18},
		{Lat: 35.35, Lon: -86.16},
		{Lat: 35.36, Lon: -86.16},
		{Lat: 35.34, Lon: -86.03},
	}
	anns := []store.Annotation{
		{ID: "loc-0", Type: "point", Category: "aid", Label: "Right", Geometry: pointGeoJSON(LatLon{Lat: 35.31556, Lon: -86.35872})},
		{ID: "loc-1", Type: "point", Category: "aid", Label: "Right", Geometry: pointGeoJSON(LatLon{Lat: 35.31053, Lon: -86.36055})},
		{ID: "loc-2", Type: "point", Category: "aid", Label: "Right", Geometry: pointGeoJSON(LatLon{Lat: 35.32283, Lon: -86.3651})},
		{ID: "loc-3", Type: "point", Category: "aid", Label: "Right", Geometry: pointGeoJSON(LatLon{Lat: 35.29304, Lon: -86.36688})},
		{ID: "loc-4", Type: "point", Category: "aid", Label: "Right", Geometry: pointGeoJSON(LatLon{Lat: 35.28432, Lon: -86.37149})},
		{ID: "loc-5", Type: "point", Category: "aid", Label: "Left", Geometry: pointGeoJSON(LatLon{Lat: 35.28492, Lon: -86.37176})},
	}
	f := Build(Inputs{BufferMiles: 10, Annotations: anns, Now: time.Now()})
	m := f.Classify(polygonAlert("Special Weather Statement", SeverityModerate, ring), nil)

	if m.Proximity != ProximityIn {
		t.Fatalf("Proximity = %v, want in (the polygon is within the 10-mile buffer)", m.Proximity)
	}
	if n := len(m.Affects.Locations); n != 0 {
		labels := []string{}
		for _, l := range m.Affects.Locations {
			labels = append(labels, l.Label)
		}
		t.Errorf("Affects.Locations = %d %v, want 0 — none of the course is inside this polygon", n, labels)
	}
	if m.Affects.Summary != "" {
		t.Errorf("Summary = %q, want empty", m.Affects.Summary)
	}
}
