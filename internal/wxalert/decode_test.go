package wxalert

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustLoadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("loading fixture %s: %v", name, err)
	}
	return data
}

// wrapFeature loads a single-Feature fixture and wraps it as a one-element
// FeatureCollection so DecodeCollection can parse it directly.
func wrapFeature(t *testing.T, name string) []byte {
	t.Helper()
	feature := mustLoadFixture(t, name)
	var buf bytes.Buffer
	buf.WriteString(`{"type":"FeatureCollection","updated":"2026-09-18T02:51:00+00:00","features":[`)
	buf.Write(feature)
	buf.WriteString(`]}`)
	return buf.Bytes()
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parsing time %q: %v", s, err)
	}
	return tm.UTC()
}

func decodeOne(t *testing.T, fixture string) Alert {
	t.Helper()
	body := wrapFeature(t, fixture)
	d, err := DecodeCollection(bytes.NewReader(body), time.Now())
	if err != nil {
		t.Fatalf("DecodeCollection(%s): %v", fixture, err)
	}
	if len(d.Alerts) != 1 {
		t.Fatalf("DecodeCollection(%s) = %d alerts, want 1 (dropped=%v)", fixture, len(d.Alerts), d.Dropped)
	}
	return d.Alerts[0]
}

func TestDecodePolygonAlert(t *testing.T) {
	a := decodeOne(t, "alert_polygon_ffw_update.json")

	wantID := "urn:oid:2.49.0.1.840.0.1c8973854eca5dd9b43de6a39a2e8df328acae4e.001.1"
	if a.ID != wantID {
		t.Errorf("ID = %q, want %q", a.ID, wantID)
	}
	if strings.HasPrefix(a.ID, "https://") {
		t.Errorf("ID looks like the feature URL, not properties.id: %q", a.ID)
	}
	if a.Event != "Flash Flood Warning" || a.Tier != TierWarning {
		t.Errorf("Event/Tier = %q/%v, want Flash Flood Warning/warning", a.Event, a.Tier)
	}
	if a.Severity != SeveritySevere || a.Urgency != UrgencyImmediate || a.Certainty != CertaintyLikely {
		t.Errorf("Severity/Urgency/Certainty = %v/%v/%v", a.Severity, a.Urgency, a.Certainty)
	}
	if a.Status != StatusActual || a.MessageType != MessageTypeUpdate || a.Response != "Avoid" {
		t.Errorf("Status/MessageType/Response = %v/%v/%q", a.Status, a.MessageType, a.Response)
	}
	wantSent := mustParseTime(t, "2026-09-17T19:38:00-06:00")
	if !a.Sent.Equal(wantSent) {
		t.Errorf("Sent = %v, want %v", a.Sent, wantSent)
	}
	wantEnds := mustParseTime(t, "2026-09-17T22:00:00-06:00")
	if a.Ends == nil || !a.Ends.Equal(wantEnds) {
		t.Errorf("Ends = %v, want %v", a.Ends, wantEnds)
	}
	if !a.Expires.Equal(wantEnds) {
		t.Errorf("Expires = %v, want %v", a.Expires, wantEnds)
	}
	if a.SenderName != "NWS Albuquerque NM" {
		t.Errorf("SenderName = %q, want NWS Albuquerque NM", a.SenderName)
	}
	if a.SenderID != "KABQ" {
		t.Errorf("SenderID = %q, want KABQ (parsed from VTEC)", a.SenderID)
	}
	if a.AreaDesc != "Sandoval, NM" {
		t.Errorf("AreaDesc = %q", a.AreaDesc)
	}
	if len(a.UGC) != 1 || a.UGC[0] != "NMC043" {
		t.Errorf("UGC = %v, want [NMC043]", a.UGC)
	}
	if len(a.SAME) != 1 || a.SAME[0] != "035043" {
		t.Errorf("SAME = %v, want [035043]", a.SAME)
	}
	if a.Geometry == nil {
		t.Fatalf("Geometry is nil, want a polygon")
	}
	if len(a.Geometry.Rings) != 1 || len(a.Geometry.Rings[0]) != 10 {
		t.Fatalf("Rings = %v, want one 10-point ring", a.Geometry.Rings)
	}
	first := a.Geometry.Rings[0][0]
	if first.Lon != -107.22 || first.Lat != 36.22 {
		t.Errorf("first ring point = %+v, want lon=-107.22 lat=36.22", first)
	}
	last := a.Geometry.Rings[0][len(a.Geometry.Rings[0])-1]
	if last != first {
		t.Errorf("ring is not closed: first=%+v last=%+v", first, last)
	}
	if len(a.References) != 1 {
		t.Fatalf("References = %v, want 1 entry", a.References)
	}
	wantRefSent := mustParseTime(t, "2026-09-17T19:03:00-06:00")
	if !a.References[0].Sent.Equal(wantRefSent) {
		t.Errorf("References[0].Sent = %v, want %v", a.References[0].Sent, wantRefSent)
	}
	if got := a.Parameters["flashFloodDamageThreat"]; len(got) != 1 || got[0] != "CONSIDERABLE" {
		t.Errorf("flashFloodDamageThreat = %v, want [CONSIDERABLE]", got)
	}
	if a.EffectiveEvent != "Flash Flood Warning" {
		t.Errorf("EffectiveEvent = %q, want Flash Flood Warning (CONSIDERABLE does not escalate)", a.EffectiveEvent)
	}
	if !strings.HasPrefix(a.Headline, "Flash Flood Warning issued September 17 at 7:38PM MDT") {
		t.Errorf("Headline = %q", a.Headline)
	}
}

func TestDecodeZoneOnlyAlert(t *testing.T) {
	a := decodeOne(t, "alert_zone_heat_advisory_update.json")

	if a.Geometry != nil {
		t.Errorf("Geometry = %+v, want nil for a zone-only alert", a.Geometry)
	}
	if len(a.UGC) != 33 {
		t.Fatalf("len(UGC) = %d, want 33", len(a.UGC))
	}
	if a.UGC[0] != "TNZ005" || a.UGC[32] != "TNZ095" {
		t.Errorf("UGC[0]/[32] = %q/%q, want TNZ005/TNZ095", a.UGC[0], a.UGC[32])
	}
	if a.Tier != TierAdvisory || a.Severity != SeverityModerate || a.Urgency != UrgencyExpected {
		t.Errorf("Tier/Severity/Urgency = %v/%v/%v", a.Tier, a.Severity, a.Urgency)
	}
	if a.Ends == nil || a.Expires.Equal(*a.Ends) {
		t.Fatalf("Expires and Ends must differ for this fixture; Expires=%v Ends=%v", a.Expires, a.Ends)
	}
	if !a.EndTime().Equal(*a.Ends) {
		t.Errorf("EndTime() = %v, want Ends %v", a.EndTime(), *a.Ends)
	}
	if got := a.Parameters["VTEC"]; len(got) != 1 || got[0] != "/O.EXT.KOHX.HT.Y.0009.000000T0000Z-260921T0000Z/" {
		t.Errorf("VTEC = %v", got)
	}
	if a.VTECAction() != "EXT" {
		t.Errorf("VTECAction() = %q, want EXT", a.VTECAction())
	}
	if a.SenderID != "KOHX" {
		t.Errorf("SenderID = %q, want KOHX", a.SenderID)
	}
	if len(a.References) != 1 {
		t.Errorf("References = %v, want 1 entry", a.References)
	}
}

func TestDecodeNulls(t *testing.T) {
	t.Run("air quality alert", func(t *testing.T) {
		a := decodeOne(t, "alert_zone_air_quality.json")
		if a.Ends != nil {
			t.Errorf("Ends = %v, want nil", a.Ends)
		}
		if !a.EndTime().Equal(a.Expires) {
			t.Errorf("EndTime() = %v, want Expires %v", a.EndTime(), a.Expires)
		}
		if a.Instruction != "" {
			t.Errorf("Instruction = %q, want empty", a.Instruction)
		}
		if a.Severity != SeverityUnknown || a.Urgency != UrgencyUnknown || a.Certainty != CertaintyUnknown {
			t.Errorf("Severity/Urgency/Certainty = %v/%v/%v, want Unknown/Unknown/Unknown", a.Severity, a.Urgency, a.Certainty)
		}
		if a.Tier != TierAdvisory {
			t.Errorf("Tier = %v, want advisory", a.Tier)
		}
		if len(a.Parameters["VTEC"]) != 0 {
			t.Errorf("VTEC = %v, want none", a.Parameters["VTEC"])
		}
		if a.VTECAction() != "" {
			t.Errorf("VTECAction() = %q, want empty", a.VTECAction())
		}
		if a.SenderID != "" {
			t.Errorf("SenderID = %q, want empty (no guessing from senderName)", a.SenderID)
		}
	})

	t.Run("special weather statement null ends", func(t *testing.T) {
		a := decodeOne(t, "alert_zone_sws_null_ends.json")
		if a.Ends != nil {
			t.Errorf("Ends = %v, want nil", a.Ends)
		}
		if a.Tier != TierStatement {
			t.Errorf("Tier = %v, want statement", a.Tier)
		}
		if a.Severity != SeverityModerate {
			t.Errorf("Severity = %v, want Moderate", a.Severity)
		}
		if a.Response != "Execute" {
			t.Errorf("Response = %q, want Execute", a.Response)
		}
	})
}

func TestDecodeDropsTestAndExercise(t *testing.T) {
	body := mustLoadFixture(t, "active_collection.json")
	d, err := DecodeCollection(bytes.NewReader(body), time.Now())
	if err != nil {
		t.Fatalf("DecodeCollection: %v", err)
	}
	if len(d.Alerts) != 5 {
		t.Errorf("len(Alerts) = %d, want 5", len(d.Alerts))
	}
	if len(d.Dropped) != 2 {
		t.Fatalf("len(Dropped) = %d, want 2 (got %v)", len(d.Dropped), d.Dropped)
	}
	reasons := map[string]bool{}
	for _, dr := range d.Dropped {
		reasons[dr.Reason] = true
		if dr.ID == "" {
			t.Errorf("DropReason has no ID: %+v", dr)
		}
	}
	if !reasons["status test"] || !reasons["status exercise"] {
		t.Errorf("Dropped reasons = %v, want status test + status exercise", d.Dropped)
	}
	for _, a := range d.Alerts {
		if a.IsTest() {
			t.Errorf("kept alert %q has non-Actual status %v", a.ID, a.Status)
		}
	}
}

// TestDecodeStatusGuardTable covers the status/event edge cases the two live
// fixtures above don't exercise: System/Draft statuses, case-insensitive
// "actual", an empty status, and event:"Test" arriving with status:"Actual".
func TestDecodeStatusGuardTable(t *testing.T) {
	mk := func(id, status, event string) []byte {
		return []byte(`{
			"id": "https://api.weather.gov/alerts/` + id + `",
			"type": "Feature", "geometry": null,
			"properties": {
				"id": "` + id + `", "event": "` + event + `",
				"sent": "2026-09-17T12:00:00-04:00", "effective": "2026-09-17T12:00:00-04:00",
				"expires": "2026-09-17T13:00:00-04:00",
				"status": "` + status + `", "messageType": "Alert",
				"severity": "Minor", "certainty": "Observed", "urgency": "Future"
			}
		}`)
	}
	tests := []struct {
		name       string
		status     string
		event      string
		wantKept   bool
		wantReason string
	}{
		{"System dropped", "System", "Special Weather Statement", false, "status system"},
		{"Draft dropped", "Draft", "Special Weather Statement", false, "status draft"},
		{"Actual kept", "Actual", "Special Weather Statement", true, ""},
		{"lowercase actual kept", "actual", "Special Weather Statement", true, ""},
		{"empty status dropped", "", "Special Weather Statement", false, "status empty"},
		{"event Test with Actual status dropped", "Actual", "Test", false, "event test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feature := mk("urn:oid:statusguard."+strings.ReplaceAll(tt.name, " ", "-")+".001.1", tt.status, tt.event)
			var buf bytes.Buffer
			buf.WriteString(`{"type":"FeatureCollection","features":[`)
			buf.Write(feature)
			buf.WriteString(`]}`)
			d, err := DecodeCollection(bytes.NewReader(buf.Bytes()), time.Now())
			if err != nil {
				t.Fatalf("DecodeCollection: %v", err)
			}
			if tt.wantKept {
				if len(d.Alerts) != 1 || len(d.Dropped) != 0 {
					t.Fatalf("alerts=%d dropped=%v, want 1 alert kept", len(d.Alerts), d.Dropped)
				}
				return
			}
			if len(d.Alerts) != 0 || len(d.Dropped) != 1 {
				t.Fatalf("alerts=%d dropped=%v, want 0 alerts / 1 dropped", len(d.Alerts), d.Dropped)
			}
			if d.Dropped[0].Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", d.Dropped[0].Reason, tt.wantReason)
			}
		})
	}
}

func TestDecodeUpdateAndCancel(t *testing.T) {
	upd := decodeOne(t, "alert_polygon_tor_synthetic.json")
	if upd.MessageType != MessageTypeAlert {
		t.Errorf("synthetic TOR MessageType = %v, want Alert", upd.MessageType)
	}
	if upd.IsCancel() {
		t.Errorf("synthetic TOR reported IsCancel()")
	}

	c := decodeOne(t, "alert_cancel_tor_synthetic.json")
	if c.MessageType != MessageTypeCancel || !c.IsCancel() {
		t.Errorf("cancel MessageType/IsCancel = %v/%v, want Cancel/true", c.MessageType, c.IsCancel())
	}
	if c.Geometry != nil {
		t.Errorf("Cancel Geometry = %+v, want nil", c.Geometry)
	}
	if len(c.References) != 1 || c.References[0].ID != upd.ID {
		t.Errorf("Cancel References = %v, want a single reference to %q", c.References, upd.ID)
	}
	if !c.Expires.Equal(c.Sent) {
		t.Errorf("Cancel Expires = %v, want == Sent %v (instantly expired)", c.Expires, c.Sent)
	}
}

func TestDecodeMalformed(t *testing.T) {
	t.Run("truncated", func(t *testing.T) {
		body := mustLoadFixture(t, "alert_malformed_truncated.json")
		_, err := DecodeCollection(bytes.NewReader(body), time.Now())
		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("error %v does not wrap ErrMalformed", err)
		}
		if !strings.Contains(err.Error(), "unexpected end") {
			t.Errorf("error %q does not mention 'unexpected end'", err.Error())
		}
	})

	t.Run("not geojson (NWS error body)", func(t *testing.T) {
		body := mustLoadFixture(t, "alert_malformed_not_geojson.json")
		_, err := DecodeCollection(bytes.NewReader(body), time.Now())
		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("error %v does not wrap ErrMalformed", err)
		}
		if !strings.Contains(err.Error(), "Bad Request") {
			t.Errorf("error %q does not mention the NWS title 'Bad Request'", err.Error())
		}
	})

	t.Run("features not array", func(t *testing.T) {
		body := mustLoadFixture(t, "alert_malformed_features_not_array.json")
		_, err := DecodeCollection(bytes.NewReader(body), time.Now())
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("error %v does not wrap ErrMalformed", err)
		}
	})

	t.Run("feature missing properties degrades, does not blank the panel", func(t *testing.T) {
		body := mustLoadFixture(t, "alert_malformed_feature_missing_properties.json")
		d, err := DecodeCollection(bytes.NewReader(body), time.Now())
		if err != nil {
			t.Fatalf("DecodeCollection: %v", err)
		}
		if len(d.Alerts) != 1 {
			t.Fatalf("Alerts = %d, want 1 (the other feature should still decode)", len(d.Alerts))
		}
		if len(d.Dropped) != 1 || d.Dropped[0].Reason != "missing properties" {
			t.Errorf("Dropped = %v, want one 'missing properties'", d.Dropped)
		}
	})

	t.Run("unsupported geometry Point is dropped, not fatal", func(t *testing.T) {
		feature := []byte(`{
			"id": "https://api.weather.gov/alerts/urn:oid:pointgeom.001.1",
			"type": "Feature",
			"geometry": {"type": "Point", "coordinates": [-85.5, 42.9]},
			"properties": {
				"id": "urn:oid:pointgeom.001.1", "event": "Special Weather Statement",
				"sent": "2026-09-17T12:00:00-04:00", "expires": "2026-09-17T13:00:00-04:00",
				"status": "Actual", "messageType": "Alert",
				"severity": "Minor", "certainty": "Observed", "urgency": "Future"
			}
		}`)
		var buf bytes.Buffer
		buf.WriteString(`{"type":"FeatureCollection","features":[`)
		buf.Write(feature)
		buf.WriteString(`]}`)
		d, err := DecodeCollection(bytes.NewReader(buf.Bytes()), time.Now())
		if err != nil {
			t.Fatalf("DecodeCollection: %v", err)
		}
		if len(d.Alerts) != 0 || len(d.Dropped) != 1 {
			t.Fatalf("alerts=%d dropped=%v, want 0/1", len(d.Alerts), d.Dropped)
		}
		if d.Dropped[0].Reason != "unsupported geometry Point" {
			t.Errorf("Reason = %q, want %q", d.Dropped[0].Reason, "unsupported geometry Point")
		}
	})

	t.Run("bad sent is dropped, bad ends is kept with Ends nil", func(t *testing.T) {
		feature := []byte(`{
			"id": "https://api.weather.gov/alerts/urn:oid:badsent.001.1",
			"type": "Feature", "geometry": null,
			"properties": {
				"id": "urn:oid:badsent.001.1", "event": "Special Weather Statement",
				"sent": "not-a-date", "expires": "2026-09-17T13:00:00-04:00",
				"status": "Actual", "messageType": "Alert",
				"severity": "Minor", "certainty": "Observed", "urgency": "Future"
			}
		}`)
		var buf bytes.Buffer
		buf.WriteString(`{"type":"FeatureCollection","features":[`)
		buf.Write(feature)
		buf.WriteString(`]}`)
		d, err := DecodeCollection(bytes.NewReader(buf.Bytes()), time.Now())
		if err != nil {
			t.Fatalf("DecodeCollection: %v", err)
		}
		if len(d.Alerts) != 0 || len(d.Dropped) != 1 || d.Dropped[0].Reason != "bad sent" {
			t.Fatalf("alerts=%d dropped=%v, want 0/[bad sent]", len(d.Alerts), d.Dropped)
		}

		featureBadEnds := []byte(`{
			"id": "https://api.weather.gov/alerts/urn:oid:badends.001.1",
			"type": "Feature", "geometry": null,
			"properties": {
				"id": "urn:oid:badends.001.1", "event": "Special Weather Statement",
				"sent": "2026-09-17T12:00:00-04:00", "expires": "2026-09-17T13:00:00-04:00",
				"ends": "bad",
				"status": "Actual", "messageType": "Alert",
				"severity": "Minor", "certainty": "Observed", "urgency": "Future"
			}
		}`)
		buf.Reset()
		buf.WriteString(`{"type":"FeatureCollection","features":[`)
		buf.Write(featureBadEnds)
		buf.WriteString(`]}`)
		d2, err := DecodeCollection(bytes.NewReader(buf.Bytes()), time.Now())
		if err != nil {
			t.Fatalf("DecodeCollection: %v", err)
		}
		if len(d2.Alerts) != 1 {
			t.Fatalf("alerts=%d, want 1 (a bad 'ends' must not lose the whole alert)", len(d2.Alerts))
		}
		if d2.Alerts[0].Ends != nil {
			t.Errorf("Ends = %v, want nil", d2.Alerts[0].Ends)
		}
	})

	t.Run("missing geocode is kept with empty UGC", func(t *testing.T) {
		feature := []byte(`{
			"id": "https://api.weather.gov/alerts/urn:oid:nogeocode.001.1",
			"type": "Feature", "geometry": null,
			"properties": {
				"id": "urn:oid:nogeocode.001.1", "event": "Special Weather Statement",
				"sent": "2026-09-17T12:00:00-04:00", "expires": "2026-09-17T13:00:00-04:00",
				"status": "Actual", "messageType": "Alert",
				"severity": "Minor", "certainty": "Observed", "urgency": "Future"
			}
		}`)
		var buf bytes.Buffer
		buf.WriteString(`{"type":"FeatureCollection","features":[`)
		buf.Write(feature)
		buf.WriteString(`]}`)
		d, err := DecodeCollection(bytes.NewReader(buf.Bytes()), time.Now())
		if err != nil {
			t.Fatalf("DecodeCollection: %v", err)
		}
		if len(d.Alerts) != 1 {
			t.Fatalf("alerts=%d, want 1", len(d.Alerts))
		}
		a := d.Alerts[0]
		if a.UGC == nil || len(a.UGC) != 0 {
			t.Errorf("UGC = %v, want non-nil empty slice", a.UGC)
		}
		if a.HasLocation() {
			t.Errorf("HasLocation() = true, want false (no polygon, no UGC)")
		}
	})
}

func TestDecodeCollectionMetadata(t *testing.T) {
	body := mustLoadFixture(t, "active_collection.json")
	d, err := DecodeCollection(bytes.NewReader(body), time.Now())
	if err != nil {
		t.Fatalf("DecodeCollection: %v", err)
	}
	want := mustParseTime(t, "2026-09-18T02:51:00+00:00")
	if !d.Updated.Equal(want) {
		t.Errorf("Updated = %v, want %v", d.Updated, want)
	}

	t.Run("missing updated", func(t *testing.T) {
		body := []byte(`{"type":"FeatureCollection","features":[]}`)
		d, err := DecodeCollection(bytes.NewReader(body), time.Now())
		if err != nil {
			t.Fatalf("DecodeCollection: %v", err)
		}
		if !d.Updated.IsZero() {
			t.Errorf("Updated = %v, want zero", d.Updated)
		}
	})
}

func TestUGCFromZoneURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"https://api.weather.gov/zones/county/MIC081", "MIC081"},
		{"https://api.weather.gov/zones/forecast/MIZ056", "MIZ056"},
		{"MIC081", "MIC081"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := UGCFromZoneURL(tt.in); got != tt.want {
			t.Errorf("UGCFromZoneURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestOfficeFromVTEC(t *testing.T) {
	tests := []struct {
		vtec []string
		want string
	}{
		{[]string{"/O.NEW.KGRR.TO.W.0012.260917T2000Z-260917T2045Z/"}, "KGRR"},
		{[]string{"/O.CON.KABQ.FF.W.0150.000000T0000Z-260918T0400Z/"}, "KABQ"},
		{nil, ""},
		{[]string{""}, ""},
	}
	for _, tt := range tests {
		if got := OfficeFromVTEC(tt.vtec); got != tt.want {
			t.Errorf("OfficeFromVTEC(%v) = %q, want %q", tt.vtec, got, tt.want)
		}
	}
}
