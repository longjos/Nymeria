package annotation

import (
	"encoding/json"
	"os"
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

const testGPXTrack = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="ridewithgps.com" xmlns="http://www.topografix.com/GPX/1/1" xmlns:gpxdata="http://www.cluetrust.com/XML/GPXDATA/1/0">
  <trk>
    <name>Day 1 Course</name>
    <desc>48 mile out and back</desc>
    <trkseg>
      <trkpt lat="35.732921" lon="-87.626103"><ele>169.6</ele></trkpt>
      <trkpt lat="35.732791" lon="-87.626173"><ele>169.5</ele></trkpt>
      <trkpt lat="35.732608" lon="-87.626275"><ele>169.5</ele></trkpt>
    </trkseg>
  </trk>
</gpx>`

const testGPXTrackTwoSegments = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <trk>
    <name>Split Track</name>
    <trkseg>
      <trkpt lat="35.1" lon="-87.1"></trkpt>
      <trkpt lat="35.2" lon="-87.2"></trkpt>
    </trkseg>
    <trkseg>
      <trkpt lat="35.3" lon="-87.3"></trkpt>
      <trkpt lat="35.4" lon="-87.4"></trkpt>
    </trkseg>
  </trk>
</gpx>`

const testGPXMixed = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <wpt lat="34.0522" lon="-118.2437">
    <name>Command Post</name>
    <desc>Main command post</desc>
  </wpt>
  <trk>
    <name>Course</name>
    <trkseg>
      <trkpt lat="35.1" lon="-87.1"></trkpt>
      <trkpt lat="35.2" lon="-87.2"></trkpt>
    </trkseg>
  </trk>
</gpx>`

const testGPXRoute = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <rte>
    <name>Planned Route</name>
    <desc>Garmin style route</desc>
    <rtept lat="35.5" lon="-87.5"></rtept>
    <rtept lat="35.6" lon="-87.6"></rtept>
    <rtept lat="35.7" lon="-87.7"></rtept>
  </rte>
</gpx>`

const testGPXShortTrack = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <trk>
    <name>Too Short</name>
    <trkseg>
      <trkpt lat="35.1" lon="-87.1"></trkpt>
    </trkseg>
  </trk>
  <rte>
    <name>Short Route</name>
    <rtept lat="35.5" lon="-87.5"></rtept>
  </rte>
</gpx>`

const testGPXUnnamedTrack = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <trk>
    <trkseg>
      <trkpt lat="35.1" lon="-87.1"></trkpt>
      <trkpt lat="35.2" lon="-87.2"></trkpt>
    </trkseg>
  </trk>
  <rte>
    <rtept lat="35.5" lon="-87.5"></rtept>
    <rtept lat="35.6" lon="-87.6"></rtept>
  </rte>
</gpx>`

// decodeLineString decodes a GeoJSON LineString geometry into its coordinates.
func decodeLineString(t *testing.T, geometryJSON string) [][2]float64 {
	t.Helper()
	var geom struct {
		Type        string       `json:"type"`
		Coordinates [][2]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(geometryJSON), &geom); err != nil {
		t.Fatalf("unmarshal GeometryJSON %q: %v", geometryJSON, err)
	}
	if geom.Type != "LineString" {
		t.Errorf("geometry type = %q, want LineString", geom.Type)
	}
	return geom.Coordinates
}

func TestParseGPXLines(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantItems int
		// index of the line item to inspect within the returned items.
		lineIndex   int
		wantName    string
		wantDesc    string
		wantCoords  [][2]float64
		skipIfEmpty bool
	}{
		{
			name:       "track only",
			input:      testGPXTrack,
			wantItems:  1,
			lineIndex:  0,
			wantName:   "Day 1 Course",
			wantDesc:   "48 mile out and back",
			wantCoords: [][2]float64{{-87.626103, 35.732921}, {-87.626173, 35.732791}, {-87.626275, 35.732608}},
		},
		{
			name:       "track with two segments concatenated",
			input:      testGPXTrackTwoSegments,
			wantItems:  1,
			lineIndex:  0,
			wantName:   "Split Track",
			wantCoords: [][2]float64{{-87.1, 35.1}, {-87.2, 35.2}, {-87.3, 35.3}, {-87.4, 35.4}},
		},
		{
			name:       "mixed waypoint and track",
			input:      testGPXMixed,
			wantItems:  2,
			lineIndex:  1,
			wantName:   "Course",
			wantCoords: [][2]float64{{-87.1, 35.1}, {-87.2, 35.2}},
		},
		{
			name:       "route becomes one line",
			input:      testGPXRoute,
			wantItems:  1,
			lineIndex:  0,
			wantName:   "Planned Route",
			wantDesc:   "Garmin style route",
			wantCoords: [][2]float64{{-87.5, 35.5}, {-87.6, 35.6}, {-87.7, 35.7}},
		},
		{
			name:      "track and route with fewer than two points are skipped",
			input:     testGPXShortTrack,
			wantItems: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := ParseGPXWaypoints(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("ParseGPXWaypoints: %v", err)
			}
			if len(items) != tt.wantItems {
				t.Fatalf("got %d items, want %d: %+v", len(items), tt.wantItems, items)
			}
			if tt.wantItems == 0 {
				return
			}

			line := items[tt.lineIndex]
			if line.Name != tt.wantName {
				t.Errorf("name = %q, want %q", line.Name, tt.wantName)
			}
			if line.Description != tt.wantDesc {
				t.Errorf("desc = %q, want %q", line.Description, tt.wantDesc)
			}
			if line.ItemType != TypeLine {
				t.Errorf("type = %q, want %q", line.ItemType, TypeLine)
			}
			if line.Category != CategoryRoute {
				t.Errorf("category = %q, want %q", line.Category, CategoryRoute)
			}

			coords := decodeLineString(t, line.GeometryJSON)
			if len(coords) != len(tt.wantCoords) {
				t.Fatalf("got %d coords, want %d: %v", len(coords), len(tt.wantCoords), coords)
			}
			for i, c := range coords {
				if c != tt.wantCoords[i] {
					t.Errorf("coords[%d] = %v, want %v", i, c, tt.wantCoords[i])
				}
			}
		})
	}
}

func TestParseGPXUnnamedLineFallback(t *testing.T) {
	items, err := ParseGPXWaypoints(strings.NewReader(testGPXUnnamedTrack))
	if err != nil {
		t.Fatalf("ParseGPXWaypoints: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[0].Name != "Track 1" {
		t.Errorf("items[0].Name = %q, want Track 1", items[0].Name)
	}
	if items[1].Name != "Route 1" {
		t.Errorf("items[1].Name = %q, want Route 1", items[1].Name)
	}
	for i, it := range items {
		if it.ItemType != TypeLine {
			t.Errorf("items[%d].ItemType = %q, want %q", i, it.ItemType, TypeLine)
		}
		if it.Category != CategoryRoute {
			t.Errorf("items[%d].Category = %q, want %q", i, it.Category, CategoryRoute)
		}
	}
}

func TestParseGPXAlias(t *testing.T) {
	viaAlias, err := ParseGPXWaypoints(strings.NewReader(testGPXMixed))
	if err != nil {
		t.Fatalf("ParseGPXWaypoints: %v", err)
	}
	direct, err := ParseGPX(strings.NewReader(testGPXMixed))
	if err != nil {
		t.Fatalf("ParseGPX: %v", err)
	}
	if len(viaAlias) != len(direct) {
		t.Fatalf("alias returned %d items, ParseGPX returned %d", len(viaAlias), len(direct))
	}
	for i := range direct {
		if viaAlias[i] != direct[i] {
			t.Errorf("item %d differs: %+v vs %+v", i, viaAlias[i], direct[i])
		}
	}
}

const testGPXMissingCoords = `<?xml version="1.0"?>
<gpx version="1.1">
  <trk>
    <name>Partial Track</name>
    <trkseg>
      <trkpt lat="1.0" lon="2.0"/>
      <trkpt lon="4.0"/>
      <trkpt><extensions><foo>bar</foo></extensions></trkpt>
      <trkpt lat="5.0"/>
      <trkpt lat="6.0" lon="7.0"/>
    </trkseg>
  </trk>
  <rte>
    <name>Partial Route</name>
    <rtept lat="10.0" lon="20.0"/>
    <rtept lon="40.0"/>
    <rtept lat="60.0" lon="70.0"/>
  </rte>
  <trk>
    <name>Degenerate Track</name>
    <trkseg>
      <trkpt lat="1.0" lon="2.0"/>
      <trkpt lon="4.0"/>
    </trkseg>
  </trk>
</gpx>`

// Points missing a required lat or lon attribute must be dropped, not decoded
// as 0.0 — a null-island vertex would corrupt the whole line's shape.
func TestParseGPXSkipsPointsMissingCoords(t *testing.T) {
	items, err := ParseGPX(strings.NewReader(testGPXMissingCoords))
	if err != nil {
		t.Fatalf("ParseGPX: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (degenerate track dropped): %+v", len(items), items)
	}
	if got, want := items[0].GeometryJSON, `{"type":"LineString","coordinates":[[2,1],[7,6]]}`; got != want {
		t.Errorf("track geometry = %s, want %s", got, want)
	}
	if got, want := items[1].GeometryJSON, `{"type":"LineString","coordinates":[[20,10],[70,60]]}`; got != want {
		t.Errorf("route geometry = %s, want %s", got, want)
	}
	if items[0].Name != "Partial Track" || items[1].Name != "Partial Route" {
		t.Errorf("names = %q, %q", items[0].Name, items[1].Name)
	}
}

// --- Turn-cue classification ------------------------------------------------
//
// Route planners (RideWithGPS, Garmin) export turn-by-turn cues as <wpt>
// elements alongside the real stops. The real course file has 49 waypoints of
// which only 4 are places a rider cares about; the other 45 are literally named
// "Left"/"Right"/"Straight". The file classifies itself via <type> and <cmt>,
// which the parser used to discard.

func TestClassifyWaypoint(t *testing.T) {
	tests := []struct {
		name         string
		wptName      string
		cmt          string
		typ          string
		wantCategory string
		wantKeep     bool
	}{
		// Rule 1: rest areas.
		{"rest area by type", "Rest Stop Maxwell Chapel", "rest_stop", "rest_area", CategoryAid, true},
		{"rest area by cmt only", "Rest Stop Eakin Elementary", "rest_stop", "", CategoryAid, true},
		{"rest area by type only", "Rest Stop Flat Creek", "", "rest_area", CategoryAid, true},
		{"rest area mixed case and padding", "Rest Stop", "  REST_STOP  ", " Rest_Area ", CategoryAid, true},

		// Rule 2: finish.
		{"finish by cmt", "Finish  Jack Daniel's Visitor Parking", "finish", "generic", CategoryFinish, true},
		{"finish by name prefix", "Finish Line", "", "", CategoryFinish, true},
		{"finish name mixed case", "FINISH area", "", "", CategoryFinish, true},

		// Rule 3: start.
		{"start by cmt", "START LINE", "start", "", CategoryStart, true},
		{"start by name prefix", "Start Corral", "", "", CategoryStart, true},

		// Rule 4: cue comments.
		{"turn left cue", "Left", "Turn left onto Maxwell Chapel Rd", "Dot", "", false},
		{"turn right cue", "Right", "Turn right onto Shelbyville Hwy", "Dot", "", false},
		{"continue cue", "Straight", "Continue onto Highway 64", "Dot", "", false},
		{"slight cue", "Slight Left", "Slight left onto Old Tullahoma Rd", "Dot", "", false},
		{"keep cue", "Keep Right", "Keep right at the fork", "Dot", "", false},
		{"bear cue", "Bear Left", "Bear left onto County Rd", "Dot", "", false},
		{"sharp cue", "Sharp Right", "Sharp right onto Elm", "Dot", "", false},
		{"head cue", "Head North", "Head north on Main St", "Dot", "", false},
		{"make a cue", "Left", "Make a U-turn", "Dot", "", false},
		{"merge cue", "Merge", "Merge onto US-231", "Dot", "", false},
		{"exit cue", "Exit", "Exit onto the ramp", "Dot", "", false},
		{"cue cmt mixed case and padding", "Left", "  TURN LEFT onto Foo  ", "Dot", "", false},

		// Rule 5: bare turn-cue names with type Dot and no comment.
		{"dot left no cmt", "Left", "", "Dot", "", false},
		{"dot right no cmt", "Right", "", "Dot", "", false},
		{"dot straight no cmt", "Straight", "", "Dot", "", false},
		{"dot slight left no cmt", "Slight Left", "", "Dot", "", false},
		{"dot slight right no cmt", "slight right", "", "dot", "", false},
		{"dot sharp left no cmt", "Sharp Left", "", "Dot", "", false},
		{"dot sharp right no cmt", " SHARP RIGHT ", "", " DOT ", "", false},
		// A Dot-typed waypoint whose name is a real place is NOT a cue.
		{"dot with real name kept", "Water Stop", "", "Dot", CategoryGeneral, true},
		// A cue-looking name without type Dot is kept: we only drop when the
		// exporter itself marked it as a map dot.
		{"left name without dot type kept", "Left", "", "", CategoryGeneral, true},

		// Rule 6: fallthrough.
		{"plain waypoint", "Water", "", "", CategoryGeneral, true},
		{"unknown vocabulary", "Picnic Shelter", "", "picnic", CategoryGeneral, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCategory, gotKeep := classifyWaypoint(tt.wptName, tt.cmt, tt.typ)
			if gotKeep != tt.wantKeep {
				t.Fatalf("classifyWaypoint(%q,%q,%q) keep = %v, want %v", tt.wptName, tt.cmt, tt.typ, gotKeep, tt.wantKeep)
			}
			if gotCategory != tt.wantCategory {
				t.Errorf("classifyWaypoint(%q,%q,%q) category = %q, want %q", tt.wptName, tt.cmt, tt.typ, gotCategory, tt.wantCategory)
			}
		})
	}
}

const testGPXTurnCues = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="ridewithgps.com">
  <wpt lat="35.73728" lon="-86.64471">
    <name>Left</name>
    <cmt>Turn left onto Maxwell Chapel Rd</cmt>
    <type>Dot</type>
  </wpt>
  <wpt lat="35.61365" lon="-86.54982">
    <name>Rest Stop Maxwell Chapel</name>
    <desc>191 Maxwell Chapel Road</desc>
    <cmt>rest_stop</cmt>
    <type>rest_area</type>
  </wpt>
  <wpt lat="35.60000" lon="-86.54000">
    <name>Right</name>
    <cmt>Turn right onto Shelbyville Hwy</cmt>
    <type>Dot</type>
  </wpt>
  <wpt lat="35.59000" lon="-86.53000">
    <name>Straight</name>
    <cmt>Continue onto Highway 64</cmt>
    <type>Dot</type>
  </wpt>
  <wpt lat="35.28487" lon="-86.37205">
    <name>Finish  Jack Daniel's Visitor Parking</name>
    <cmt>finish</cmt>
    <type>generic</type>
  </wpt>
  <trk>
    <name>Day 1 48M Jack and Back</name>
    <trkseg>
      <trkpt lat="35.73864" lon="-86.64463"></trkpt>
      <trkpt lat="35.61365" lon="-86.54982"></trkpt>
      <trkpt lat="35.28487" lon="-86.37205"></trkpt>
    </trkseg>
  </trk>
</gpx>`

func TestParseGPXSkipsTurnCues(t *testing.T) {
	items, err := ParseGPX(strings.NewReader(testGPXTurnCues))
	if err != nil {
		t.Fatalf("ParseGPX: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (2 stops + 1 line): %+v", len(items), items)
	}

	if items[0].Name != "Rest Stop Maxwell Chapel" {
		t.Errorf("items[0].Name = %q, want Rest Stop Maxwell Chapel", items[0].Name)
	}
	if items[0].Category != CategoryAid {
		t.Errorf("items[0].Category = %q, want %q", items[0].Category, CategoryAid)
	}
	if items[0].ItemType != "" {
		t.Errorf("items[0].ItemType = %q, want empty (point)", items[0].ItemType)
	}

	if items[1].Name != "Finish  Jack Daniel's Visitor Parking" {
		t.Errorf("items[1].Name = %q", items[1].Name)
	}
	if items[1].Category != CategoryFinish {
		t.Errorf("items[1].Category = %q, want %q", items[1].Category, CategoryFinish)
	}

	if items[2].Category != CategoryRoute || items[2].ItemType != TypeLine {
		t.Errorf("items[2] = category %q type %q, want %q/%q", items[2].Category, items[2].ItemType, CategoryRoute, TypeLine)
	}
}

const testGPXCmtDescription = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test">
  <wpt lat="35.1" lon="-86.1">
    <name>Rest Stop One</name>
    <cmt>rest_stop</cmt>
    <type>rest_area</type>
  </wpt>
  <wpt lat="35.2" lon="-86.2">
    <name>Rest Stop Two</name>
    <desc>191 Maxwell Chapel Road</desc>
    <cmt>rest_stop</cmt>
    <type>rest_area</type>
  </wpt>
</gpx>`

// A kept waypoint must not lose the only text it has: when <desc> is absent the
// <cmt> becomes the description.
func TestParseGPXCmtFillsEmptyDescription(t *testing.T) {
	items, err := ParseGPX(strings.NewReader(testGPXCmtDescription))
	if err != nil {
		t.Fatalf("ParseGPX: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[0].Description != "rest_stop" {
		t.Errorf("items[0].Description = %q, want rest_stop (from cmt)", items[0].Description)
	}
	if items[1].Description != "191 Maxwell Chapel Road" {
		t.Errorf("items[1].Description = %q, want the desc to win over cmt", items[1].Description)
	}
}

const testGPXUnknownVocabulary = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="some-other-planner">
  <wpt lat="35.1" lon="-86.1">
    <name>Shady Picnic Area</name>
    <type>picnic</type>
  </wpt>
</gpx>`

// Safe degradation: an exporter whose <type> vocabulary we do not recognise must
// still have its waypoints imported (as general), never silently dropped.
func TestParseGPXUnknownVocabularyFallsThrough(t *testing.T) {
	items, err := ParseGPX(strings.NewReader(testGPXUnknownVocabulary))
	if err != nil {
		t.Fatalf("ParseGPX: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1: %+v", len(items), items)
	}
	if items[0].Category != CategoryGeneral {
		t.Errorf("category = %q, want %q", items[0].Category, CategoryGeneral)
	}
	if items[0].Name != "Shady Picnic Area" {
		t.Errorf("name = %q", items[0].Name)
	}
}

// TestParseGPXRealCourse runs against the operator's real event file when it is
// present in the repo root. 49 waypoints (45 of them turn cues) must reduce to
// the 3 rest stops plus the finish, alongside the single course line.
func TestParseGPXRealCourse(t *testing.T) {
	f, err := os.Open("../../Day_1_48M_Jack_and_Back.gpx")
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("real course file not present")
		}
		t.Fatalf("open real course: %v", err)
	}
	defer f.Close()

	items, err := ParseGPX(f)
	if err != nil {
		t.Fatalf("ParseGPX: %v", err)
	}

	var points, lines []ImportItem
	for _, it := range items {
		if it.ItemType == TypeLine {
			lines = append(lines, it)
		} else {
			points = append(points, it)
		}
	}
	if len(points) != 4 {
		names := make([]string, len(points))
		for i, p := range points {
			names[i] = p.Name
		}
		t.Fatalf("got %d point items, want 4: %v", len(points), names)
	}
	if len(lines) != 1 {
		t.Fatalf("got %d line items, want 1", len(lines))
	}

	wantNames := map[string]string{
		"Rest Stop Maxwell Chapel":              CategoryAid,
		"Rest Stop Eakin Elementary":            CategoryAid,
		"Rest Stop Flat Creek Community Center": CategoryAid,
		"Finish  Jack Daniel's Visitor Parking": CategoryFinish,
	}
	for _, p := range points {
		wantCat, ok := wantNames[p.Name]
		if !ok {
			t.Errorf("unexpected kept waypoint %q", p.Name)
			continue
		}
		if p.Category != wantCat {
			t.Errorf("%q category = %q, want %q", p.Name, p.Category, wantCat)
		}
		delete(wantNames, p.Name)
	}
	for missing := range wantNames {
		t.Errorf("missing expected stop %q", missing)
	}
}
