package annotation

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// gpxFile represents the top-level GPX structure.
type gpxFile struct {
	XMLName   xml.Name      `xml:"gpx"`
	Waypoints []gpxWaypoint `xml:"wpt"`
	Tracks    []gpxTrack    `xml:"trk"`
	Routes    []gpxRoute    `xml:"rte"`
}

type gpxWaypoint struct {
	Lat float64 `xml:"lat,attr"`
	Lon float64 `xml:"lon,attr"`
	// Name, Desc are free text. Cmt and Type are the exporter's own
	// classification of the waypoint and are what separate a rest stop from a
	// turn cue; see classifyWaypoint.
	Name string `xml:"name"`
	Desc string `xml:"desc"`
	Cmt  string `xml:"cmt"`
	Type string `xml:"type"`
}

// turnCueCmtPrefixes are the leading words of a turn-by-turn navigation
// instruction. Route planners (RideWithGPS, Garmin) emit one <wpt> per cue
// alongside the handful of waypoints a rider actually cares about — the real
// 48-mile course file carries 45 cues and 4 stops — and an imported cue named
// "Left" is worse than no data, because "next stop: Left, 0.3 mi" sounds like
// an answer over the radio.
var turnCueCmtPrefixes = []string{
	"turn ", "continue ", "slight ", "keep ", "bear ",
	"sharp ", "head ", "make a ", "merge ", "exit ",
}

// turnCueNames are the bare cue labels planners use when there is no <cmt> to
// go on. They are only treated as cues when <type> is also "dot" (the generic
// map-dot symbol), so a real place that happens to be called "Left" but is
// typed as something meaningful survives.
var turnCueNames = map[string]bool{
	"left": true, "right": true, "straight": true,
	"slight left": true, "slight right": true,
	"sharp left": true, "sharp right": true,
}

// classifyWaypoint maps a GPX waypoint to an annotation category, reporting
// false when the waypoint is a turn cue and should not be imported at all.
//
// Unknown vocabularies fall through to CategoryGeneral and are kept: a stop we
// fail to recognise can be recategorised by hand, but a stop we drop is gone.
func classifyWaypoint(name, cmt, typ string) (category string, keep bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	cmt = strings.ToLower(strings.TrimSpace(cmt))
	typ = strings.ToLower(strings.TrimSpace(typ))

	switch {
	case typ == "rest_area" || cmt == "rest_stop":
		return CategoryAid, true
	case cmt == "finish" || strings.HasPrefix(name, "finish"):
		return CategoryFinish, true
	case cmt == "start" || strings.HasPrefix(name, "start"):
		return CategoryStart, true
	}

	for _, p := range turnCueCmtPrefixes {
		if strings.HasPrefix(cmt, p) {
			return "", false
		}
	}
	if typ == "dot" && turnCueNames[name] {
		return "", false
	}

	return CategoryGeneral, true
}

// gpxTrack is a <trk>: a named collection of segments. Segments are just GPS
// dropouts in a single recorded path, so they are concatenated into one line.
type gpxTrack struct {
	Name     string        `xml:"name"`
	Desc     string        `xml:"desc"`
	Segments []gpxTrackSeg `xml:"trkseg"`
}

type gpxTrackSeg struct {
	Points []gpxPoint `xml:"trkpt"`
}

// gpxRoute is a <rte>: the planned-path line form emitted by Garmin and others.
type gpxRoute struct {
	Name   string     `xml:"name"`
	Desc   string     `xml:"desc"`
	Points []gpxPoint `xml:"rtept"`
}

// gpxPoint is a <trkpt>/<rtept>. lat and lon are required by the GPX 1.1 XSD,
// so they are pointers: a point missing either is malformed and must be dropped
// rather than silently decoded as 0.0, which would splice a null-island vertex
// into the middle of the line.
type gpxPoint struct {
	Lat *float64 `xml:"lat,attr"`
	Lon *float64 `xml:"lon,attr"`
}

// ParseGPXWaypoints extracts waypoints, tracks and routes from a GPX file as
// ImportItems. It is an alias for ParseGPX, kept for existing callers.
func ParseGPXWaypoints(r io.Reader) ([]ImportItem, error) {
	return ParseGPX(r)
}

// ParseGPX extracts waypoints (<wpt>), tracks (<trk>) and routes (<rte>) from a
// GPX file as ImportItems. Each track and each route becomes a single route line
// annotation; tracks or routes with fewer than two points are skipped.
func ParseGPX(r io.Reader) ([]ImportItem, error) {
	var gpx gpxFile
	if err := xml.NewDecoder(r).Decode(&gpx); err != nil {
		return nil, fmt.Errorf("decode GPX: %w", err)
	}

	var items []ImportItem
	for _, wpt := range gpx.Waypoints {
		if wpt.Name == "" {
			continue
		}
		category, keep := classifyWaypoint(wpt.Name, wpt.Cmt, wpt.Type)
		if !keep {
			continue // turn cue: never import it
		}
		desc := wpt.Desc
		if desc == "" {
			desc = wpt.Cmt // keep the only text this waypoint has
		}
		items = append(items, ImportItem{
			Name:        wpt.Name,
			Lat:         wpt.Lat,
			Lon:         wpt.Lon,
			Description: desc,
			Category:    category,
			ShortName:   "",
		})
	}

	for i, trk := range gpx.Tracks {
		var pts []gpxPoint
		for _, seg := range trk.Segments {
			pts = append(pts, seg.Points...)
		}
		if item, ok := gpxLineItem(trk.Name, trk.Desc, pts, fmt.Sprintf("Track %d", i+1)); ok {
			items = append(items, item)
		}
	}

	for i, rte := range gpx.Routes {
		if item, ok := gpxLineItem(rte.Name, rte.Desc, rte.Points, fmt.Sprintf("Route %d", i+1)); ok {
			items = append(items, item)
		}
	}

	return items, nil
}

// gpxLineItem builds a route line ImportItem from GPX points. It reports false
// when there are too few points to form a line.
func gpxLineItem(name, desc string, pts []gpxPoint, fallbackName string) (ImportItem, bool) {
	if len(pts) < 2 {
		return ImportItem{}, false
	}

	coords := make([][2]float64, 0, len(pts))
	for _, p := range pts {
		if p.Lat == nil || p.Lon == nil {
			continue // malformed point: drop it rather than emit [0,0]
		}
		coords = append(coords, [2]float64{*p.Lon, *p.Lat})
	}
	if len(coords) < 2 {
		return ImportItem{}, false
	}

	if name == "" {
		name = fallbackName
	}

	return ImportItem{
		Name:         name,
		Description:  desc,
		Category:     CategoryRoute,
		ItemType:     TypeLine,
		GeometryJSON: buildLineStringGeoJSON(coords),
	}, true
}

// kmlFile represents the top-level KML structure.
type kmlFile struct {
	XMLName  xml.Name    `xml:"kml"`
	Document kmlDocument `xml:"Document"`
}

type kmlDocument struct {
	Placemarks []kmlPlacemark `xml:"Placemark"`
}

type kmlPlacemark struct {
	Name        string        `xml:"name"`
	Description string        `xml:"description"`
	Point       kmlPoint      `xml:"Point"`
	LineString  kmlLineString `xml:"LineString"`
}

type kmlPoint struct {
	Coordinates string `xml:"coordinates"`
}

type kmlLineString struct {
	Coordinates string `xml:"coordinates"`
}

// ParseKMLPlacemarks extracts placemarks from a KML file as ImportItems.
func ParseKMLPlacemarks(r io.Reader) ([]ImportItem, error) {
	var kml kmlFile
	if err := xml.NewDecoder(r).Decode(&kml); err != nil {
		return nil, fmt.Errorf("decode KML: %w", err)
	}

	var items []ImportItem
	for i, pm := range kml.Document.Placemarks {
		// Point placemarks.
		if pm.Point.Coordinates != "" {
			lat, lon, err := parseKMLCoordinates(pm.Point.Coordinates)
			if err != nil {
				continue
			}

			name := pm.Name
			if name == "" {
				name = fmt.Sprintf("Point %d", i+1)
			}

			items = append(items, ImportItem{
				Name:        name,
				Lat:         lat,
				Lon:         lon,
				Description: pm.Description,
				Category:    CategoryGeneral,
			})
			continue
		}

		// LineString placemarks.
		if pm.LineString.Coordinates != "" {
			coords, err := parseKMLCoordinateList(pm.LineString.Coordinates)
			if err != nil || len(coords) < 2 {
				continue
			}

			name := pm.Name
			if name == "" {
				name = fmt.Sprintf("Route %d", i+1)
			}

			geojson := buildLineStringGeoJSON(coords)

			items = append(items, ImportItem{
				Name:         name,
				Description:  pm.Description,
				Category:     CategoryRoute,
				ItemType:     TypeLine,
				GeometryJSON: geojson,
			})
			continue
		}
	}

	return items, nil
}

// parseKMLCoordinateList parses a whitespace-separated list of "lon,lat,alt" tuples
// and returns [][2]float64 as [lon, lat] pairs (GeoJSON order).
func parseKMLCoordinateList(s string) ([][2]float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty coordinate list")
	}

	fields := strings.Fields(s)
	coords := make([][2]float64, 0, len(fields))
	for _, f := range fields {
		lat, lon, err := parseKMLCoordinates(f)
		if err != nil {
			continue // skip unparseable tuples
		}
		coords = append(coords, [2]float64{lon, lat})
	}
	return coords, nil
}

// buildLineStringGeoJSON builds a GeoJSON LineString from [lon,lat] pairs.
func buildLineStringGeoJSON(coords [][2]float64) string {
	var sb strings.Builder
	sb.WriteString(`{"type":"LineString","coordinates":[`)
	for i, c := range coords {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("[%g,%g]", c[0], c[1]))
	}
	sb.WriteString("]}")
	return sb.String()
}

// parseKMLCoordinates parses "lon,lat,alt" KML coordinate string.
func parseKMLCoordinates(s string) (lat, lon float64, err error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ",")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid KML coordinates: %q", s)
	}

	lon, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse longitude: %w", err)
	}

	lat, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse latitude: %w", err)
	}

	return lat, lon, nil
}
