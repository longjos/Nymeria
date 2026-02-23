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
}

type gpxWaypoint struct {
	Lat  float64 `xml:"lat,attr"`
	Lon  float64 `xml:"lon,attr"`
	Name string  `xml:"name"`
	Desc string  `xml:"desc"`
}

// ParseGPXWaypoints extracts waypoints from a GPX file as ImportItems.
func ParseGPXWaypoints(r io.Reader) ([]ImportItem, error) {
	var gpx gpxFile
	if err := xml.NewDecoder(r).Decode(&gpx); err != nil {
		return nil, fmt.Errorf("decode GPX: %w", err)
	}

	var items []ImportItem
	for i, wpt := range gpx.Waypoints {
		if wpt.Name == "" {
			continue
		}
		items = append(items, ImportItem{
			Name:        wpt.Name,
			Lat:         wpt.Lat,
			Lon:         wpt.Lon,
			Description: wpt.Desc,
			Category:    CategoryGeneral,
			ShortName:   "",
		})
		_ = i // sort order handled by ImportAnnotations
	}

	return items, nil
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
