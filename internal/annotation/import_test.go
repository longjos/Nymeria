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
