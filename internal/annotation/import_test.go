package annotation

import (
	"strings"
	"testing"
)

const testGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <wpt lat="34.0522" lon="-118.2437">
    <name>Command Post</name>
    <desc>Main command post at city hall</desc>
  </wpt>
  <wpt lat="34.0600" lon="-118.2500">
    <name>Aid Station 1</name>
    <desc>First aid at mile 3</desc>
  </wpt>
  <wpt lat="34.0700" lon="-118.2600">
    <name></name>
    <desc>Unnamed waypoint should be skipped</desc>
  </wpt>
</gpx>`

const testKML = `<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2">
  <Document>
    <Placemark>
      <name>Staging Area</name>
      <description>Main staging area</description>
      <Point>
        <coordinates>-118.2437,34.0522,0</coordinates>
      </Point>
    </Placemark>
    <Placemark>
      <name>Water Stop</name>
      <description>Water stop at mile 7</description>
      <Point>
        <coordinates>-118.2600,34.0700,100</coordinates>
      </Point>
    </Placemark>
    <Placemark>
      <name>No Point</name>
      <description>This placemark has no point and should be skipped</description>
    </Placemark>
  </Document>
</kml>`

func TestParseGPXWaypoints(t *testing.T) {
	items, err := ParseGPXWaypoints(strings.NewReader(testGPX))
	if err != nil {
		t.Fatalf("ParseGPXWaypoints: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (unnamed should be skipped)", len(items))
	}

	cp := items[0]
	if cp.Name != "Command Post" {
		t.Errorf("name = %q, want Command Post", cp.Name)
	}
	if cp.Lat != 34.0522 {
		t.Errorf("lat = %f, want 34.0522", cp.Lat)
	}
	if cp.Lon != -118.2437 {
		t.Errorf("lon = %f, want -118.2437", cp.Lon)
	}
	if cp.Description != "Main command post at city hall" {
		t.Errorf("desc = %q", cp.Description)
	}

	as := items[1]
	if as.Name != "Aid Station 1" {
		t.Errorf("name = %q, want Aid Station 1", as.Name)
	}
}

func TestParseGPXInvalid(t *testing.T) {
	_, err := ParseGPXWaypoints(strings.NewReader("not xml"))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestParseKMLPlacemarks(t *testing.T) {
	items, err := ParseKMLPlacemarks(strings.NewReader(testKML))
	if err != nil {
		t.Fatalf("ParseKMLPlacemarks: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (no-point should be skipped)", len(items))
	}

	sa := items[0]
	if sa.Name != "Staging Area" {
		t.Errorf("name = %q, want Staging Area", sa.Name)
	}
	if sa.Lat != 34.0522 {
		t.Errorf("lat = %f, want 34.0522", sa.Lat)
	}
	if sa.Lon != -118.2437 {
		t.Errorf("lon = %f, want -118.2437", sa.Lon)
	}
	if sa.Description != "Main staging area" {
		t.Errorf("desc = %q", sa.Description)
	}

	ws := items[1]
	if ws.Name != "Water Stop" {
		t.Errorf("name = %q, want Water Stop", ws.Name)
	}
	if ws.Lat != 34.0700 {
		t.Errorf("lat = %f, want 34.0700", ws.Lat)
	}
	if ws.Lon != -118.2600 {
		t.Errorf("lon = %f, want -118.2600", ws.Lon)
	}
}

const testKMLLineString = `<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2">
  <Document>
    <name>Test Route</name>
    <Placemark>
      <name>My Route</name>
      <description>A test route</description>
      <LineString>
        <coordinates>
-87.626103,35.732921,169.6
-87.626173,35.732791,169.5
-87.626275,35.732608,169.5
-87.62635,35.732518,168.8
        </coordinates>
      </LineString>
    </Placemark>
  </Document>
</kml>`

const testKMLMixed = `<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2">
  <Document>
    <Placemark>
      <name>Start Point</name>
      <description>The start</description>
      <Point>
        <coordinates>-87.626103,35.732921,0</coordinates>
      </Point>
    </Placemark>
    <Placemark>
      <name>Course Route</name>
      <description>Main course</description>
      <LineString>
        <coordinates>
-87.626103,35.732921,0
-87.626173,35.732791,0
-87.626275,35.732608,0
        </coordinates>
      </LineString>
    </Placemark>
    <Placemark>
      <name>Finish Point</name>
      <description>The finish</description>
      <Point>
        <coordinates>-87.62635,35.732518,0</coordinates>
      </Point>
    </Placemark>
  </Document>
</kml>`

func TestParseKMLLineString(t *testing.T) {
	items, err := ParseKMLPlacemarks(strings.NewReader(testKMLLineString))
	if err != nil {
		t.Fatalf("ParseKMLPlacemarks: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}

	route := items[0]
	if route.Name != "My Route" {
		t.Errorf("name = %q, want My Route", route.Name)
	}
	if route.Description != "A test route" {
		t.Errorf("desc = %q, want A test route", route.Description)
	}
	if route.ItemType != "line" {
		t.Errorf("type = %q, want line", route.ItemType)
	}
	if route.Category != "route" {
		t.Errorf("category = %q, want route", route.Category)
	}
	if route.GeometryJSON == "" {
		t.Fatal("GeometryJSON is empty")
	}
	// Verify it contains valid GeoJSON structure.
	if !strings.Contains(route.GeometryJSON, `"type":"LineString"`) {
		t.Errorf("GeometryJSON missing LineString type: %s", route.GeometryJSON)
	}
	if !strings.Contains(route.GeometryJSON, `"coordinates":[`) {
		t.Errorf("GeometryJSON missing coordinates: %s", route.GeometryJSON)
	}
}

func TestParseKMLMixed(t *testing.T) {
	items, err := ParseKMLPlacemarks(strings.NewReader(testKMLMixed))
	if err != nil {
		t.Fatalf("ParseKMLPlacemarks: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}

	// First item: Point.
	if items[0].Name != "Start Point" {
		t.Errorf("items[0].Name = %q, want Start Point", items[0].Name)
	}
	if items[0].ItemType != "" {
		t.Errorf("items[0].ItemType = %q, want empty (default point)", items[0].ItemType)
	}
	if items[0].Lat != 35.732921 {
		t.Errorf("items[0].Lat = %f, want 35.732921", items[0].Lat)
	}

	// Second item: LineString.
	if items[1].Name != "Course Route" {
		t.Errorf("items[1].Name = %q, want Course Route", items[1].Name)
	}
	if items[1].ItemType != "line" {
		t.Errorf("items[1].ItemType = %q, want line", items[1].ItemType)
	}
	if items[1].Category != "route" {
		t.Errorf("items[1].Category = %q, want route", items[1].Category)
	}

	// Third item: Point.
	if items[2].Name != "Finish Point" {
		t.Errorf("items[2].Name = %q, want Finish Point", items[2].Name)
	}
	if items[2].ItemType != "" {
		t.Errorf("items[2].ItemType = %q, want empty (default point)", items[2].ItemType)
	}
}

func TestParseKMLInvalid(t *testing.T) {
	_, err := ParseKMLPlacemarks(strings.NewReader("not xml"))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestParseKMLCoordinates(t *testing.T) {
	tests := []struct {
		input   string
		wantLat float64
		wantLon float64
		wantErr bool
	}{
		{"-118.2437,34.0522,0", 34.0522, -118.2437, false},
		{"-118.2437,34.0522", 34.0522, -118.2437, false},
		{" -118.2437 , 34.0522 , 100 ", 34.0522, -118.2437, false},
		{"invalid", 0, 0, true},
		{"", 0, 0, true},
	}

	for _, tt := range tests {
		lat, lon, err := parseKMLCoordinates(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseKMLCoordinates(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseKMLCoordinates(%q): %v", tt.input, err)
			continue
		}
		if lat != tt.wantLat {
			t.Errorf("parseKMLCoordinates(%q): lat = %f, want %f", tt.input, lat, tt.wantLat)
		}
		if lon != tt.wantLon {
			t.Errorf("parseKMLCoordinates(%q): lon = %f, want %f", tt.input, lon, tt.wantLon)
		}
	}
}
