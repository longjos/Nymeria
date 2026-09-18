package wxalert

import (
	"encoding/json"
	"fmt"
	"math"
)

// earthRadiusMiles is the mean Earth radius used by every distance
// calculation in this package.
const earthRadiusMiles = 3958.7613

// degToRad / radToDeg are the conversion factors used throughout.
const degToRad = math.Pi / 180.0
const radToDeg = 180.0 / math.Pi

// haversineMiles returns the great-circle distance between two points in
// statute miles. Correct across the antimeridian and near the poles because
// it operates on the central angle, never on a naive longitude subtraction.
func haversineMiles(a, b LatLon) float64 {
	lat1, lat2 := a.Lat*degToRad, b.Lat*degToRad
	dLat := (b.Lat - a.Lat) * degToRad
	dLon := (b.Lon - a.Lon) * degToRad

	sinDLat := math.Sin(dLat / 2)
	sinDLon := math.Sin(dLon / 2)
	h := sinDLat*sinDLat + math.Cos(lat1)*math.Cos(lat2)*sinDLon*sinDLon
	h = math.Min(1, math.Max(0, h))
	c := 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
	return earthRadiusMiles * c
}

// bearingDeg returns the initial great-circle bearing from a to b, in
// [0,360).
func bearingDeg(a, b LatLon) int {
	lat1, lat2 := a.Lat*degToRad, b.Lat*degToRad
	dLon := (b.Lon - a.Lon) * degToRad
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	deg := math.Atan2(y, x) * radToDeg
	deg = math.Mod(deg+360, 360)
	return int(math.Round(deg)) % 360
}

// project is a local equirectangular projection around ref, in miles.
// Accurate to well under 1% for the +/-100 mile spans this package cares
// about; it is never used for anything longer than that.
func project(ref, p LatLon) (x, y float64) {
	dLat := (p.Lat - ref.Lat) * degToRad
	dLon := (p.Lon - ref.Lon) * degToRad
	x = dLon * math.Cos(ref.Lat*degToRad) * earthRadiusMiles
	y = dLat * earthRadiusMiles
	return x, y
}

// pointToSegmentMiles returns the shortest distance from p to the segment ab.
func pointToSegmentMiles(p, a, b LatLon) float64 {
	// Project into a local plane around the segment midpoint so ordinary
	// planar segment-distance math applies without a lon/lat distortion.
	ref := LatLon{Lat: (a.Lat + b.Lat) / 2, Lon: (a.Lon + b.Lon) / 2}
	ax, ay := project(ref, a)
	bx, by := project(ref, b)
	px, py := project(ref, p)

	dx, dy := bx-ax, by-ay
	lenSq := dx*dx + dy*dy
	if lenSq == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / lenSq
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	cx, cy := ax+t*dx, ay+t*dy
	return math.Hypot(px-cx, py-cy)
}

// closeRing appends the first point if the ring is not already closed.
func closeRing(ring []LatLon) []LatLon {
	if len(ring) == 0 {
		return ring
	}
	first, last := ring[0], ring[len(ring)-1]
	if first.Lat == last.Lat && first.Lon == last.Lon {
		return ring
	}
	out := make([]LatLon, len(ring)+1)
	copy(out, ring)
	out[len(ring)] = first
	return out
}

// shiftLon shifts a negative longitude into [180,360) so a ring that crosses
// the +/-180 line becomes a contiguous range for ordinary interpolation math.
func shiftLon(lon float64) float64 {
	if lon < 0 {
		return lon + 360
	}
	return lon
}

// ringCrossesAntimeridian reports whether ring's raw longitudes span more
// than 180 degrees — the signal that it actually crosses the date line
// rather than just being a wide ring.
func ringCrossesAntimeridian(ring []LatLon) bool {
	if len(ring) == 0 {
		return false
	}
	minLon, maxLon := ring[0].Lon, ring[0].Lon
	for _, p := range ring {
		if p.Lon < minLon {
			minLon = p.Lon
		}
		if p.Lon > maxLon {
			maxLon = p.Lon
		}
	}
	return maxLon-minLon > 180
}

func shiftRingLon(ring []LatLon) []LatLon {
	out := make([]LatLon, len(ring))
	for i, p := range ring {
		out[i] = LatLon{Lat: p.Lat, Lon: shiftLon(p.Lon)}
	}
	return out
}

// normalizeForRing brings ring and p into the same longitude space, shifting
// both consistently when ring crosses the antimeridian so plain planar/
// interpolation math (ray casting, local projection) is not fooled by the
// +180/-180 wraparound.
func normalizeForRing(ring []LatLon, p LatLon) ([]LatLon, LatLon) {
	if !ringCrossesAntimeridian(ring) {
		return ring, p
	}
	return shiftRingLon(ring), LatLon{Lat: p.Lat, Lon: shiftLon(p.Lon)}
}

// pointInRing reports whether p is inside ring (ray casting; on-edge counts
// as inside) and not inside any of holes. A degenerate ring (fewer than 3
// distinct points) is treated as a single point: p "is in" it only if pointInRing
// is asked to test containment via ringDistanceMiles instead (this function
// returns false for a degenerate ring so callers fall back to distance).
func pointInRing(p LatLon, ring []LatLon, holes [][]LatLon) bool {
	nRing, nP := normalizeForRing(ring, p)
	if !rayCast(nP, nRing) {
		return false
	}
	for _, h := range holes {
		nHole, nP2 := normalizeForRing(h, p)
		if rayCast(nP2, nHole) {
			return false
		}
	}
	return true
}

// rayCast is the plain ray-casting containment test (no holes), with an
// on-edge point counted as inside. ring and p must already share a
// consistent longitude space (see normalizeForRing).
func rayCast(p LatLon, ring []LatLon) bool {
	ring = closeRing(ring)
	n := len(ring)
	if n < 4 { // closed ring needs >= 3 distinct points + closing point
		return false
	}
	inside := false
	for i := 0; i < n-1; i++ {
		a, b := ring[i], ring[i+1]
		if onSegment(p, a, b) {
			return true
		}
		if (a.Lat > p.Lat) != (b.Lat > p.Lat) {
			lonAtP := a.Lon + (p.Lat-a.Lat)*(b.Lon-a.Lon)/(b.Lat-a.Lat)
			if p.Lon < lonAtP {
				inside = !inside
			}
		}
	}
	return inside
}

func onSegment(p, a, b LatLon) bool {
	const eps = 1e-9
	cross := (b.Lat-a.Lat)*(p.Lon-a.Lon) - (b.Lon-a.Lon)*(p.Lat-a.Lat)
	if math.Abs(cross) > eps {
		return false
	}
	if p.Lon < math.Min(a.Lon, b.Lon)-eps || p.Lon > math.Max(a.Lon, b.Lon)+eps {
		return false
	}
	if p.Lat < math.Min(a.Lat, b.Lat)-eps || p.Lat > math.Max(a.Lat, b.Lat)+eps {
		return false
	}
	return true
}

// ringDistanceMiles is 0 when p is inside ring (holes-aware); otherwise the
// minimum distance from p to any edge of the outer ring. A degenerate ring
// (all points identical, or fewer than 3 distinct vertices) is treated as a
// point at its first vertex.
func ringDistanceMiles(p LatLon, ring []LatLon, holes [][]LatLon) float64 {
	if len(ring) == 0 {
		return math.Inf(1)
	}
	if isDegenerateRing(ring) {
		return haversineMiles(p, ring[0])
	}
	if pointInRing(p, ring, holes) {
		return 0
	}
	nRing, nP := normalizeForRing(ring, p)
	return edgeDistanceMiles(nP, nRing)
}

func edgeDistanceMiles(p LatLon, ring []LatLon) float64 {
	closed := closeRing(ring)
	min := math.Inf(1)
	for i := 0; i < len(closed)-1; i++ {
		d := pointToSegmentMiles(p, closed[i], closed[i+1])
		if d < min {
			min = d
		}
	}
	return min
}

func isDegenerateRing(ring []LatLon) bool {
	if len(ring) < 3 {
		return true
	}
	for _, pt := range ring[1:] {
		if pt.Lat != ring[0].Lat || pt.Lon != ring[0].Lon {
			return false
		}
	}
	return true
}

// segmentsIntersect reports whether segment ab crosses segment cd (using the
// local projection so it works near the antimeridian). Touching endpoints
// count as intersecting.
func segmentsIntersect(a, b, c, d LatLon) bool {
	ref := a
	bx, by := project(ref, b)
	cx, cy := project(ref, c)
	dx, dy := project(ref, d)

	d1 := orientation(0, 0, bx, by, cx, cy)
	d2 := orientation(0, 0, bx, by, dx, dy)
	d3 := orientation(cx, cy, dx, dy, 0, 0)
	d4 := orientation(cx, cy, dx, dy, bx, by)

	if d1 != d2 && d3 != d4 {
		return true
	}
	if d1 == 0 && onSeg2(0, 0, bx, by, cx, cy) {
		return true
	}
	if d2 == 0 && onSeg2(0, 0, bx, by, dx, dy) {
		return true
	}
	if d3 == 0 && onSeg2(cx, cy, dx, dy, 0, 0) {
		return true
	}
	if d4 == 0 && onSeg2(cx, cy, dx, dy, bx, by) {
		return true
	}
	return false
}

// orientation returns -1, 0 or 1 for the turn (p,q,r).
func orientation(px, py, qx, qy, rx, ry float64) int {
	val := (qy-py)*(rx-qx) - (qx-px)*(ry-qy)
	const eps = 1e-9
	if val > eps {
		return 1
	}
	if val < -eps {
		return -1
	}
	return 0
}

func onSeg2(px, py, qx, qy, rx, ry float64) bool {
	return rx <= math.Max(px, qx)+1e-9 && rx >= math.Min(px, qx)-1e-9 &&
		ry <= math.Max(py, qy)+1e-9 && ry >= math.Min(py, qy)-1e-9
}

// lineIntersectsRing reports whether any vertex of line is inside ring, or
// any segment of line crosses any edge of ring.
func lineIntersectsRing(line []LatLon, ring []LatLon, holes [][]LatLon) bool {
	for _, p := range line {
		if pointInRing(p, ring, holes) {
			return true
		}
	}
	closed := closeRing(ring)
	for i := 0; i+1 < len(line); i++ {
		for j := 0; j+1 < len(closed); j++ {
			if segmentsIntersect(line[i], line[i+1], closed[j], closed[j+1]) {
				return true
			}
		}
	}
	return false
}

// ringsIntersect reports whether ring a and ring b overlap: a vertex of
// either is inside the other, or an edge of one crosses an edge of the other.
func ringsIntersect(a, b []LatLon) bool {
	for _, p := range a {
		if pointInRing(p, b, nil) {
			return true
		}
	}
	for _, p := range b {
		if pointInRing(p, a, nil) {
			return true
		}
	}
	ca, cb := closeRing(a), closeRing(b)
	for i := 0; i+1 < len(ca); i++ {
		for j := 0; j+1 < len(cb); j++ {
			if segmentsIntersect(ca[i], ca[i+1], cb[j], cb[j+1]) {
				return true
			}
		}
	}
	return false
}

// lineToRingMiles is 0 if line intersects ring; otherwise the minimum
// distance between any line vertex and the ring, and any ring vertex and the
// line.
func lineToRingMiles(line []LatLon, ring []LatLon, holes [][]LatLon) float64 {
	if lineIntersectsRing(line, ring, holes) {
		return 0
	}
	min := math.Inf(1)
	for _, p := range line {
		if d := ringDistanceMiles(p, ring, holes); d < min {
			min = d
		}
	}
	closed := closeRing(ring)
	for _, p := range closed {
		if d := pointToLineMiles(p, line); d < min {
			min = d
		}
	}
	return min
}

func pointToLineMiles(p LatLon, line []LatLon) float64 {
	if len(line) == 1 {
		return haversineMiles(p, line[0])
	}
	min := math.Inf(1)
	for i := 0; i+1 < len(line); i++ {
		if d := pointToSegmentMiles(p, line[i], line[i+1]); d < min {
			min = d
		}
	}
	return min
}

// bboxOf returns the bounding box of pts. Does not attempt antimeridian
// unwrapping — callers that need it use expandBBoxMiles/CrossesAntimeridian.
func bboxOf(pts []LatLon) BBox {
	if len(pts) == 0 {
		return BBox{}
	}
	b := BBox{South: pts[0].Lat, North: pts[0].Lat, West: pts[0].Lon, East: pts[0].Lon}
	for _, p := range pts[1:] {
		if p.Lat < b.South {
			b.South = p.Lat
		}
		if p.Lat > b.North {
			b.North = p.Lat
		}
		if p.Lon < b.West {
			b.West = p.Lon
		}
		if p.Lon > b.East {
			b.East = p.Lon
		}
	}
	return b
}

// expandBBoxMiles grows b by miles in every direction. The result's
// West/East are wrapped into [-180,180]; if that expansion pushes past the
// date line, West ends up greater than East, which is exactly how
// BBox.CrossesAntimeridian is defined.
func expandBBoxMiles(b BBox, miles float64) BBox {
	if miles <= 0 {
		return b
	}
	centerLat := (b.South + b.North) / 2
	dLat := miles / 69.0 // ~69 statute miles per degree latitude
	cosLat := math.Cos(centerLat * degToRad)
	if cosLat < 0.01 {
		cosLat = 0.01
	}
	dLon := miles / (69.172 * cosLat)
	south := b.South - dLat
	north := b.North + dLat
	if south < -90 {
		south = -90
	}
	if north > 90 {
		north = 90
	}
	return BBox{South: south, North: north, West: wrapLon(b.West - dLon), East: wrapLon(b.East + dLon)}
}

// wrapLon normalizes a longitude into [-180,180].
func wrapLon(lon float64) float64 {
	for lon > 180 {
		lon -= 360
	}
	for lon < -180 {
		lon += 360
	}
	return lon
}

// bboxesOverlap reports whether a and b share any area. Antimeridian-crossing
// boxes are handled by splitting into the two halves.
func bboxesOverlap(a, b BBox) bool {
	aSegs := bboxLonSegments(a)
	bSegs := bboxLonSegments(b)
	latOverlap := a.South <= b.North && b.South <= a.North
	if !latOverlap {
		return false
	}
	for _, as := range aSegs {
		for _, bs := range bSegs {
			if as[0] <= bs[1] && bs[0] <= as[1] {
				return true
			}
		}
	}
	return false
}

func bboxLonSegments(b BBox) [][2]float64 {
	if b.West <= b.East {
		return [][2]float64{{b.West, b.East}}
	}
	return [][2]float64{{b.West, 180}, {-180, b.East}}
}

// ringAreaSqMiles computes the (unsigned) area of ring via the shoelace
// formula on the local projection. Used only for "entire course" heuristics
// and tests, never for matching.
func ringAreaSqMiles(ring []LatLon) float64 {
	if len(ring) < 3 {
		return 0
	}
	ref := ring[0]
	closed := closeRing(ring)
	sum := 0.0
	for i := 0; i+1 < len(closed); i++ {
		x1, y1 := project(ref, closed[i])
		x2, y2 := project(ref, closed[i+1])
		sum += x1*y2 - x2*y1
	}
	return math.Abs(sum) / 2
}

// polylineMiles returns the total length of line.
func polylineMiles(line []LatLon) float64 {
	total := 0.0
	for i := 0; i+1 < len(line); i++ {
		total += haversineMiles(line[i], line[i+1])
	}
	return total
}

// resampleMiles returns line's vertices plus interpolated points every
// everyMiles, keeping both endpoints. everyMiles <= 0 returns line unchanged.
func resampleMiles(line []LatLon, everyMiles float64) []LatLon {
	if everyMiles <= 0 || len(line) < 2 {
		return line
	}
	out := []LatLon{line[0]}
	for i := 0; i+1 < len(line); i++ {
		a, b := line[i], line[i+1]
		segLen := haversineMiles(a, b)
		if segLen > everyMiles {
			steps := int(math.Floor(segLen / everyMiles))
			for s := 1; s <= steps; s++ {
				f := float64(s) * everyMiles / segLen
				if f >= 1 {
					break
				}
				out = append(out, LatLon{
					Lat: a.Lat + (b.Lat-a.Lat)*f,
					Lon: a.Lon + (b.Lon-a.Lon)*f,
				})
			}
		}
		out = append(out, b)
	}
	return out
}

// destPoint returns the point that is miles away from p along bearingDeg
// (test helper: inverse haversine, used to build exact-by-construction
// fixtures instead of magic decimals).
func destPoint(p LatLon, bearing float64, miles float64) LatLon {
	angDist := miles / earthRadiusMiles
	brng := bearing * degToRad
	lat1 := p.Lat * degToRad
	lon1 := p.Lon * degToRad

	lat2 := math.Asin(math.Sin(lat1)*math.Cos(angDist) + math.Cos(lat1)*math.Sin(angDist)*math.Cos(brng))
	lon2 := lon1 + math.Atan2(
		math.Sin(brng)*math.Sin(angDist)*math.Cos(lat1),
		math.Cos(angDist)-math.Sin(lat1)*math.Sin(lat2),
	)
	// Normalize longitude to [-180,180].
	lonDeg := lon2 * radToDeg
	lonDeg = math.Mod(lonDeg+540, 360) - 180
	return LatLon{Lat: lat2 * radToDeg, Lon: lonDeg}
}

// ---------------------------------------------------------------------------
// GeoJSON geometry parsing shared by decode.go (alert polygons) and
// zonecache.go (zone polygons, which can be Polygon, MultiPolygon or
// GeometryCollection and can carry holes).
// ---------------------------------------------------------------------------

type geoJSONGeometry struct {
	Type        string            `json:"type"`
	Coordinates json.RawMessage   `json:"coordinates"`
	Geometries  []json.RawMessage `json:"geometries"`
}

// ringsFromGeoJSON parses a GeoJSON geometry object (Polygon, MultiPolygon,
// or GeometryCollection of either) into its outer rings and holes. Holes are
// every ring after the first in a Polygon; a MultiPolygon/GeometryCollection
// contributes one outer ring (its first polygon's first ring) per member,
// consistent with the rest of the package treating a warning polygon as its
// outer boundary. Point/LineString/other geometry types return an error.
func ringsFromGeoJSON(raw json.RawMessage) (typ string, outer [][]LatLon, holes [][]LatLon, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil, nil, nil
	}
	var g geoJSONGeometry
	if err := json.Unmarshal(raw, &g); err != nil {
		return "", nil, nil, fmt.Errorf("%w: geometry: %v", ErrMalformed, err)
	}
	switch g.Type {
	case "Polygon":
		var coords [][][2]float64
		if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
			return "", nil, nil, fmt.Errorf("%w: polygon coordinates: %v", ErrMalformed, err)
		}
		if len(coords) == 0 {
			return g.Type, nil, nil, nil
		}
		outer = append(outer, toLatLon(coords[0]))
		for _, h := range coords[1:] {
			holes = append(holes, toLatLon(h))
		}
		return g.Type, outer, holes, nil
	case "MultiPolygon":
		var coords [][][][2]float64
		if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
			return "", nil, nil, fmt.Errorf("%w: multipolygon coordinates: %v", ErrMalformed, err)
		}
		for _, poly := range coords {
			if len(poly) == 0 {
				continue
			}
			outer = append(outer, toLatLon(poly[0]))
			for _, h := range poly[1:] {
				holes = append(holes, toLatLon(h))
			}
		}
		return g.Type, outer, holes, nil
	case "GeometryCollection":
		for _, member := range g.Geometries {
			_, mOuter, mHoles, err := ringsFromGeoJSON(member)
			if err != nil {
				return "", nil, nil, err
			}
			outer = append(outer, mOuter...)
			holes = append(holes, mHoles...)
		}
		return g.Type, outer, holes, nil
	default:
		return g.Type, nil, nil, fmt.Errorf("unsupported geometry %s", g.Type)
	}
}

func toLatLon(coords [][2]float64) []LatLon {
	out := make([]LatLon, len(coords))
	for i, c := range coords {
		out[i] = LatLon{Lon: c[0], Lat: c[1]}
	}
	return out
}
