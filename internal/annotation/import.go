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
	Name        string   `xml:"name"`
	Description string   `xml:"description"`
	Point       kmlPoint `xml:"Point"`
}

type kmlPoint struct {
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
		if pm.Point.Coordinates == "" {
			continue
		}

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
	}

	return items, nil
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
