package wxalert

import (
	"math"
	"testing"
)

func approxEqual(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestHaversineMiles(t *testing.T) {
	gr := LatLon{Lat: 42.9634, Lon: -85.6681}
	tests := []struct {
		name string
		a, b LatLon
		want float64
		tol  float64
	}{
		{"same point", gr, gr, 0, 0.01},
		{"one degree latitude", LatLon{42.9634, -85.6681}, LatLon{43.9634, -85.6681}, 69.09, 0.5},
		{"equator one degree longitude", LatLon{0, 0}, LatLon{0, 1}, 69.09, 0.5},
		{"antimeridian 0.2 deg lon at 51.5N", LatLon{51.5, 179.9}, LatLon{51.5, -179.9}, 8.60, 0.3},
		{"across the pole", LatLon{89.9, 0}, LatLon{89.9, 180}, 13.82, 0.3},
		{"KC to STL", LatLon{39.0997, -94.5786}, LatLon{38.6270, -90.1994}, 237.7, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := haversineMiles(tt.a, tt.b)
			if !approxEqual(got, tt.want, tt.tol) {
				t.Errorf("haversineMiles(%v,%v) = %v, want %v +/- %v", tt.a, tt.b, got, tt.want, tt.tol)
			}
		})
	}
}

func TestDestPointRoundTrip(t *testing.T) {
	gr := LatLon{Lat: 42.9634, Lon: -85.6681}
	for _, bearing := range []float64{0, 45, 90, 135, 180, 225, 270, 315} {
		p := destPoint(gr, bearing, 10)
		got := haversineMiles(gr, p)
		if !approxEqual(got, 10, 0.05) {
			t.Errorf("destPoint bearing %v: haversineMiles back = %v, want ~10", bearing, got)
		}
	}
}

func squareRing(center LatLon, halfDeg float64) []LatLon {
	return []LatLon{
		{Lat: center.Lat - halfDeg, Lon: center.Lon - halfDeg},
		{Lat: center.Lat - halfDeg, Lon: center.Lon + halfDeg},
		{Lat: center.Lat + halfDeg, Lon: center.Lon + halfDeg},
		{Lat: center.Lat + halfDeg, Lon: center.Lon - halfDeg},
		{Lat: center.Lat - halfDeg, Lon: center.Lon - halfDeg},
	}
}

func TestPointInRingSquare(t *testing.T) {
	center := LatLon{Lat: 40, Lon: -90}
	ring := squareRing(center, 0.5)

	if !pointInRing(center, ring, nil) {
		t.Errorf("center of 1x1 deg square not reported inside")
	}
	outside := LatLon{Lat: 40, Lon: -88.99} // 1.01 deg east of center, outside 0.5 deg half-width
	if pointInRing(outside, ring, nil) {
		t.Errorf("point 0.01 deg outside the square reported inside")
	}
	edge := LatLon{Lat: 40, Lon: -89.5}
	if !pointInRing(edge, ring, nil) {
		t.Errorf("point exactly on the edge not reported inside")
	}
}

func TestPointInRingHoles(t *testing.T) {
	outer := squareRing(LatLon{Lat: 0, Lon: 0}, 1.0) // ~138 mi across
	hole := squareRing(LatLon{Lat: 0, Lon: 0}, 0.3)  // inner hole
	inHole := LatLon{Lat: 0, Lon: 0}
	inRingNotHole := LatLon{Lat: 0.6, Lon: 0}

	if pointInRing(inHole, outer, [][]LatLon{hole}) {
		t.Errorf("point inside the hole reported inside the ring")
	}
	if !pointInRing(inRingNotHole, outer, [][]LatLon{hole}) {
		t.Errorf("point inside the outer ring but outside the hole reported outside")
	}
}

func TestPointToSegmentMiles(t *testing.T) {
	// A horizontal segment through the origin; a point exactly 1 mile due
	// north of its midpoint should be ~1.0 mile away.
	a := LatLon{Lat: 0, Lon: -1}
	b := LatLon{Lat: 0, Lon: 1}
	mid := LatLon{Lat: 0, Lon: 0}
	p := destPoint(mid, 0, 1) // 1 mile due north
	got := pointToSegmentMiles(p, a, b)
	if !approxEqual(got, 1.0, 0.02) {
		t.Errorf("pointToSegmentMiles = %v, want ~1.0", got)
	}

	// Point beyond the segment's end projects to the nearest endpoint.
	far := destPoint(LatLon{Lat: 0, Lon: 2}, 90, 5)
	got2 := pointToSegmentMiles(far, a, b)
	wantMin := haversineMiles(far, b)
	if !approxEqual(got2, wantMin, 0.1) {
		t.Errorf("pointToSegmentMiles beyond endpoint = %v, want ~%v", got2, wantMin)
	}
}

func TestRingDistanceMiles(t *testing.T) {
	ring := squareRing(LatLon{Lat: 40, Lon: -90}, 0.5)
	inside := LatLon{Lat: 40, Lon: -90}
	if d := ringDistanceMiles(inside, ring, nil); d != 0 {
		t.Errorf("ringDistanceMiles(inside) = %v, want 0", d)
	}

	// A degenerate "ring" (3 identical points) is a point.
	degenerate := []LatLon{{Lat: 1, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 1, Lon: 1}}
	p := destPoint(LatLon{Lat: 1, Lon: 1}, 0, 5)
	if d := ringDistanceMiles(p, degenerate, nil); !approxEqual(d, 5, 0.1) {
		t.Errorf("ringDistanceMiles(degenerate) = %v, want ~5", d)
	}

	// Unclosed ring (first != last) is closed implicitly.
	unclosed := ring[:len(ring)-1]
	if d := ringDistanceMiles(inside, unclosed, nil); d != 0 {
		t.Errorf("ringDistanceMiles(unclosed, inside) = %v, want 0", d)
	}
}

func TestRingsIntersect(t *testing.T) {
	a := squareRing(LatLon{Lat: 40, Lon: -90}, 0.5)
	touching := squareRing(LatLon{Lat: 40, Lon: -89}, 0.5) // shares the corner at lon -89.5
	disjoint := squareRing(LatLon{Lat: 40, Lon: -80}, 0.5) // far away

	if !ringsIntersect(a, touching) {
		t.Errorf("touching-corner squares reported disjoint")
	}
	if ringsIntersect(a, disjoint) {
		t.Errorf("disjoint squares reported intersecting")
	}

	// Squares 0.5 mi apart in longitude (well under a degree) are disjoint.
	b := squareRing(LatLon{Lat: 40, Lon: -89.0}, 0.001)
	c := squareRing(LatLon{Lat: 40, Lon: -80.0}, 0.001)
	if ringsIntersect(b, c) {
		t.Errorf("small disjoint squares reported intersecting")
	}
}

func TestLineToRingMiles(t *testing.T) {
	ring := squareRing(LatLon{Lat: 40, Lon: -90}, 0.2) // roughly a 27x27 mi box

	// A line passing well clear of the box.
	far := []LatLon{{Lat: 40, Lon: -85}, {Lat: 41, Lon: -85}}
	d := lineToRingMiles(far, ring, nil)
	if d <= 0 {
		t.Errorf("lineToRingMiles(far line) = %v, want > 0", d)
	}

	// A line that clearly crosses the box.
	crossing := []LatLon{{Lat: 40, Lon: -91}, {Lat: 40, Lon: -89}}
	if got := lineToRingMiles(crossing, ring, nil); got != 0 {
		t.Errorf("lineToRingMiles(crossing line) = %v, want 0", got)
	}
}

func TestBBoxHelpers(t *testing.T) {
	pts := []LatLon{{Lat: 10, Lon: 10}, {Lat: 12, Lon: 8}, {Lat: 9, Lon: 15}}
	b := bboxOf(pts)
	if b.South != 9 || b.North != 12 || b.West != 8 || b.East != 15 {
		t.Errorf("bboxOf = %+v, want S9 N12 W8 E15", b)
	}

	expanded := expandBBoxMiles(BBox{South: 0, North: 0, West: 0, East: 0}, 69)
	if !approxEqual(expanded.North, 1.0, 0.05) {
		t.Errorf("expandBBoxMiles(69mi) North = %v, want ~1.0 deg", expanded.North)
	}

	if !bboxesOverlap(BBox{South: 0, North: 10, West: 0, East: 10}, BBox{South: 5, North: 15, West: 5, East: 15}) {
		t.Errorf("overlapping bboxes reported disjoint")
	}
	if bboxesOverlap(BBox{South: 0, North: 10, West: 0, East: 10}, BBox{South: 20, North: 30, West: 20, East: 30}) {
		t.Errorf("disjoint bboxes reported overlapping")
	}

	// Antimeridian-crossing box overlap.
	crossBox := BBox{South: 50, North: 52, West: 179, East: -179}
	other := BBox{South: 50, North: 52, West: -179.5, East: -179.2}
	if !bboxesOverlap(crossBox, other) {
		t.Errorf("antimeridian box failed to overlap a box just west of the line")
	}
}

func TestPolylineAndResample(t *testing.T) {
	line := []LatLon{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}} // ~69 miles
	got := polylineMiles(line)
	if !approxEqual(got, 69.09, 0.5) {
		t.Errorf("polylineMiles = %v, want ~69.09", got)
	}

	resampled := resampleMiles(line, 20)
	if len(resampled) < 3 {
		t.Fatalf("resampleMiles produced %d points, want >= 3", len(resampled))
	}
	if resampled[0] != line[0] {
		t.Errorf("resampleMiles dropped the start point")
	}
	last := resampled[len(resampled)-1]
	if last != line[len(line)-1] {
		t.Errorf("resampleMiles dropped the end point")
	}
	for i := 0; i+1 < len(resampled); i++ {
		if d := haversineMiles(resampled[i], resampled[i+1]); d > 20.5 {
			t.Errorf("resample gap %v > 20.5 miles between points %d,%d", d, i, i+1)
		}
	}
}

func TestRingsFromGeoJSONPolygon(t *testing.T) {
	raw := []byte(`{"type":"Polygon","coordinates":[[[-1,-1],[1,-1],[1,1],[-1,1],[-1,-1]]]}`)
	typ, outer, holes, err := ringsFromGeoJSON(raw)
	if err != nil {
		t.Fatalf("ringsFromGeoJSON error: %v", err)
	}
	if typ != "Polygon" {
		t.Errorf("type = %q, want Polygon", typ)
	}
	if len(outer) != 1 || len(outer[0]) != 5 {
		t.Fatalf("outer = %v, want one 5-point ring", outer)
	}
	if outer[0][0].Lon != -1 || outer[0][0].Lat != -1 {
		t.Errorf("first point = %+v, want lon=-1 lat=-1 (GeoJSON is [lon,lat])", outer[0][0])
	}
	if len(holes) != 0 {
		t.Errorf("holes = %v, want none", holes)
	}
}

func TestRingsFromGeoJSONPolygonWithHole(t *testing.T) {
	raw := []byte(`{"type":"Polygon","coordinates":[
		[[-2,-2],[2,-2],[2,2],[-2,2],[-2,-2]],
		[[-1,-1],[1,-1],[1,1],[-1,1],[-1,-1]]
	]}`)
	_, outer, holes, err := ringsFromGeoJSON(raw)
	if err != nil {
		t.Fatalf("ringsFromGeoJSON error: %v", err)
	}
	if len(outer) != 1 {
		t.Fatalf("outer rings = %d, want 1", len(outer))
	}
	if len(holes) != 1 {
		t.Fatalf("holes = %d, want 1", len(holes))
	}
	// The centre of the outer ring sits inside the hole, so it must not be
	// treated as solid.
	if pointInRing(LatLon{Lat: 0, Lon: 0}, outer[0], holes) {
		t.Errorf("point in the hole reported inside the polygon")
	}
}

func TestRingsFromGeoJSONMultiPolygon(t *testing.T) {
	raw := []byte(`{"type":"MultiPolygon","coordinates":[
		[[[-1,-1],[1,-1],[1,1],[-1,1],[-1,-1]]],
		[[[10,10],[11,10],[11,11],[10,11],[10,10]]]
	]}`)
	typ, outer, _, err := ringsFromGeoJSON(raw)
	if err != nil {
		t.Fatalf("ringsFromGeoJSON error: %v", err)
	}
	if typ != "MultiPolygon" {
		t.Errorf("type = %q, want MultiPolygon", typ)
	}
	if len(outer) != 2 {
		t.Fatalf("outer rings = %d, want 2", len(outer))
	}
}

func TestRingsFromGeoJSONGeometryCollection(t *testing.T) {
	raw := []byte(`{"type":"GeometryCollection","geometries":[
		{"type":"Polygon","coordinates":[[[-1,-1],[1,-1],[1,1],[-1,1],[-1,-1]]]},
		{"type":"MultiPolygon","coordinates":[[[[10,10],[11,10],[11,11],[10,11],[10,10]]]]}
	]}`)
	typ, outer, _, err := ringsFromGeoJSON(raw)
	if err != nil {
		t.Fatalf("ringsFromGeoJSON error: %v", err)
	}
	if typ != "GeometryCollection" {
		t.Errorf("type = %q, want GeometryCollection", typ)
	}
	if len(outer) != 2 {
		t.Fatalf("flattened outer rings = %d, want 2", len(outer))
	}
}

func TestRingsFromGeoJSONUnsupportedType(t *testing.T) {
	raw := []byte(`{"type":"Point","coordinates":[1,2]}`)
	_, _, _, err := ringsFromGeoJSON(raw)
	if err == nil {
		t.Fatalf("expected an error for an unsupported geometry type")
	}
}

func TestRingAreaSqMiles(t *testing.T) {
	if got := ringAreaSqMiles([]LatLon{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}}); got != 0 {
		t.Errorf("ringAreaSqMiles(2 points) = %v, want 0", got)
	}
	// A roughly 1deg x 1deg square near the equator is about 69x69 miles.
	sq := squareRing(LatLon{Lat: 0, Lon: 0}, 0.5)
	got := ringAreaSqMiles(sq)
	want := 69.09 * 69.09
	if !approxEqual(got, want, want*0.05) {
		t.Errorf("ringAreaSqMiles(1deg square) = %v, want ~%v", got, want)
	}
}

func TestCloseRingEmpty(t *testing.T) {
	if got := closeRing(nil); len(got) != 0 {
		t.Errorf("closeRing(nil) = %v, want empty", got)
	}
}

func TestBBoxOfEmpty(t *testing.T) {
	if got := bboxOf(nil); got != (BBox{}) {
		t.Errorf("bboxOf(nil) = %+v, want zero value", got)
	}
}

func TestExpandBBoxMilesNoop(t *testing.T) {
	b := BBox{South: 1, North: 2, West: 3, East: 4}
	if got := expandBBoxMiles(b, 0); got != b {
		t.Errorf("expandBBoxMiles(0) = %+v, want unchanged %+v", got, b)
	}
}

func TestExpandBBoxMilesClampsPoles(t *testing.T) {
	near := BBox{South: 89, North: 89.5, West: 0, East: 1}
	got := expandBBoxMiles(near, 500)
	if got.North != 90 {
		t.Errorf("North = %v, want clamped to 90", got.North)
	}
	nearSouth := BBox{South: -89.5, North: -89, West: 0, East: 1}
	got2 := expandBBoxMiles(nearSouth, 500)
	if got2.South != -90 {
		t.Errorf("South = %v, want clamped to -90", got2.South)
	}
}

func TestWrapLon(t *testing.T) {
	tests := []struct{ in, want float64 }{
		{190, -170},
		{-190, 170},
		{0, 0},
		{180, 180},
		{-180, -180},
	}
	for _, tt := range tests {
		if got := wrapLon(tt.in); !approxEqual(got, tt.want, 1e-9) {
			t.Errorf("wrapLon(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPointToLineMilesSinglePoint(t *testing.T) {
	line := []LatLon{{Lat: 0, Lon: 0}}
	p := destPoint(LatLon{Lat: 0, Lon: 0}, 90, 7)
	if got := pointToLineMiles(p, line); !approxEqual(got, 7, 0.1) {
		t.Errorf("pointToLineMiles(single point line) = %v, want ~7", got)
	}
}

func TestIsDegenerateRingVariants(t *testing.T) {
	if !isDegenerateRing([]LatLon{{Lat: 1, Lon: 1}, {Lat: 1, Lon: 1}}) {
		t.Errorf("2-point ring should be degenerate")
	}
	if isDegenerateRing(squareRing(LatLon{}, 1)) {
		t.Errorf("square ring should not be degenerate")
	}
}

func TestOnSegmentOffLine(t *testing.T) {
	if onSegment(LatLon{Lat: 5, Lon: 0}, LatLon{Lat: 0, Lon: 0}, LatLon{Lat: 0, Lon: 10}) {
		t.Errorf("point far off the segment reported on it")
	}
}

func TestSegmentsIntersectColinear(t *testing.T) {
	// Overlapping colinear segments on the same line.
	a, b := LatLon{Lat: 0, Lon: 0}, LatLon{Lat: 0, Lon: 10}
	c, d := LatLon{Lat: 0, Lon: 5}, LatLon{Lat: 0, Lon: 15}
	if !segmentsIntersect(a, b, c, d) {
		t.Errorf("overlapping colinear segments reported disjoint")
	}
	// Disjoint colinear segments.
	e, f := LatLon{Lat: 0, Lon: 20}, LatLon{Lat: 0, Lon: 30}
	if segmentsIntersect(a, b, e, f) {
		t.Errorf("disjoint colinear segments reported intersecting")
	}
}

func TestRingCrossesAntimeridianFalseForOrdinaryRing(t *testing.T) {
	if ringCrossesAntimeridian(squareRing(LatLon{Lat: 40, Lon: -90}, 0.5)) {
		t.Errorf("an ordinary ring far from the date line reported crossing it")
	}
	if ringCrossesAntimeridian(nil) {
		t.Errorf("nil ring reported crossing")
	}
}

func TestRingsFromGeoJSONEmptyPolygon(t *testing.T) {
	typ, outer, holes, err := ringsFromGeoJSON([]byte(`{"type":"Polygon","coordinates":[]}`))
	if err != nil || typ != "Polygon" || outer != nil || holes != nil {
		t.Errorf("empty polygon coords = (%q,%v,%v,%v)", typ, outer, holes, err)
	}
}

func TestRingsFromGeoJSONMalformedCoordinates(t *testing.T) {
	if _, _, _, err := ringsFromGeoJSON([]byte(`{"type":"Polygon","coordinates":"nope"}`)); err == nil {
		t.Errorf("malformed Polygon coordinates: want an error")
	}
	if _, _, _, err := ringsFromGeoJSON([]byte(`{"type":"MultiPolygon","coordinates":"nope"}`)); err == nil {
		t.Errorf("malformed MultiPolygon coordinates: want an error")
	}
	if _, _, _, err := ringsFromGeoJSON([]byte(`not json`)); err == nil {
		t.Errorf("malformed top-level geometry JSON: want an error")
	}
	if _, _, _, err := ringsFromGeoJSON([]byte(`{"type":"GeometryCollection","geometries":[{"type":"Point","coordinates":[1,2]}]}`)); err == nil {
		t.Errorf("GeometryCollection with an unsupported member: want an error")
	}
}

func TestResampleMilesNoSubdivisionNeeded(t *testing.T) {
	line := []LatLon{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 0.001}}
	got := resampleMiles(line, 50)
	if len(got) != 2 {
		t.Errorf("resampleMiles(short segment) = %v, want the original 2 points", got)
	}
}

func TestRingsFromGeoJSONNull(t *testing.T) {
	typ, outer, holes, err := ringsFromGeoJSON(nil)
	if err != nil || typ != "" || outer != nil || holes != nil {
		t.Errorf("ringsFromGeoJSON(nil) = (%q,%v,%v,%v), want all zero", typ, outer, holes, err)
	}
	typ, outer, holes, err = ringsFromGeoJSON([]byte("null"))
	if err != nil || typ != "" || outer != nil || holes != nil {
		t.Errorf(`ringsFromGeoJSON("null") = (%q,%v,%v,%v), want all zero`, typ, outer, holes, err)
	}
}
