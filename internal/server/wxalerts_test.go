package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/wxalert"
)

// newTestWxServer builds a server wired the way app.go wires one, backed by
// a real SQLite store. mgr may be nil to exercise the "feature never
// enabled" 503 path every wx route must answer.
func newTestWxServer(t *testing.T, mgr *wxalert.Manager) (*Server, *netcontrol.Manager, *session.MemoryManager, *config.Manager) {
	t.Helper()

	tracker := station.NewMemoryTracker(config.StationConfig{Callsign: "N0CALL", TrackMaxPoints: 10, StaleTimeout: time.Hour})
	tm := transport.NewManager()

	db := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	netMgr := netcontrol.NewManager(db, tracker)
	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{InactivityTimeout: 30 * time.Minute})
	annMgr := annotation.NewManager(db)

	cfg := config.DefaultConfig()
	cfg.Station.Callsign = "N0CALL"
	cfg.WxAlerts.Enabled = mgr != nil
	cfg.WxAlerts.Contact = "ops@example.com"
	cfgPath := filepath.Join(t.TempDir(), "nymeria.yaml")
	cfgMgr := config.NewManager(cfgPath, cfg)

	opts := []Option{
		WithSessionManager(sessMgr),
		WithNetControlManager(netMgr),
		WithAnnotationManager(annMgr),
		WithConfigManager(cfgMgr),
	}
	var baseCfg wxalert.Config
	if mgr != nil {
		baseCfg = wxalert.Config{Enabled: true, UserAgent: "Nymeria/test (ops@example.com)", DefaultBufferMiles: 10}
		opts = append(opts, WithWxAlertManager(mgr, baseCfg))
	}

	srv := New(tracker, tm, message.Engine(nil), db, opts...)
	return srv, netMgr, sessMgr, cfgMgr
}

func newTestWxManager(t *testing.T) *wxalert.Manager {
	t.Helper()
	mgr, err := wxalert.NewManager(wxalert.Config{Enabled: false, DefaultBufferMiles: 10})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

// testWxPolygon returns a small square ring (already parsed — Rings, not
// just raw Coordinates) around the given center, so Classify() has real
// geometry to match against without going through the decode.go feed
// parser this package-external test cannot reach.
func testWxPolygon(centerLat, centerLon, milesRadius float64) *wxalert.Geometry {
	d := milesRadius / 69.0
	ring := []wxalert.LatLon{
		{Lat: centerLat - d, Lon: centerLon - d},
		{Lat: centerLat + d, Lon: centerLon - d},
		{Lat: centerLat + d, Lon: centerLon + d},
		{Lat: centerLat - d, Lon: centerLon + d},
		{Lat: centerLat - d, Lon: centerLon - d},
	}
	return &wxalert.Geometry{Type: "Polygon", Rings: [][]wxalert.LatLon{ring}}
}

// testWxAlert builds a minimal, valid Alert good enough for Manager.OnAlerts
// — it deliberately mirrors what decode.go always fills in (never-nil
// slices/maps) since a hand-built Alert skips that parser.
func testWxAlert(id, event string, tier wxalert.Tier, geom *wxalert.Geometry) wxalert.Alert {
	now := time.Now().UTC()
	return wxalert.Alert{
		ID: id, Provider: "nws", Event: event, EffectiveEvent: event, Tier: tier,
		Headline: event + " issued for Kent County", Instruction: "Seek shelter immediately",
		Severity: wxalert.SeveritySevere, Certainty: wxalert.CertaintyLikely, Urgency: wxalert.UrgencyExpected,
		Status: wxalert.StatusActual, MessageType: wxalert.MessageTypeAlert,
		Sent: now, Effective: now, Expires: now.Add(time.Hour),
		SenderName: "NWS Grand Rapids MI", SenderID: "KGRR", AreaDesc: "Kent, MI",
		UGC: []string{}, SAME: []string{}, References: []wxalert.Reference{}, Parameters: map[string][]string{},
		Geometry: geom,
	}
}

// seedInAreaAlert feeds one alert into mgr, in the footprint (own position
// at the polygon's center), so it shows up as proximity "in" everywhere a
// test needs a realistic, currently-active alert.
func seedInAreaAlert(t *testing.T, mgr *wxalert.Manager, id, event string, tier wxalert.Tier) {
	t.Helper()
	center := wxalert.LatLon{Lat: 42.9634, Lon: -85.6681}
	mgr.SetFootprintInputs(wxalert.Inputs{BufferMiles: 10, Own: &center, OwnSource: "config", Now: time.Now().UTC()})
	a := testWxAlert(id, event, tier, testWxPolygon(center.Lat, center.Lon, 5))
	mgr.OnAlerts([]wxalert.Alert{a}, time.Now().UTC(), true)
}

// --- disabled feature: reads answer 200 "off", writes still 503 ----------

// When wx_alerts is disabled the manager is nil. Read endpoints must still
// answer 200 with an "off" status — "the feature is off" is a valid status,
// not a failure — so the UI can say "turned off" instead of "can't connect".
func TestWxReadRoutesReportOffWhenDisabled(t *testing.T) {
	srv, _, sessMgr, _ := newTestWxServer(t, nil)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	for _, path := range []string{"/api/wx/alerts", "/api/wx/status"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("GET %s = %d, want 200 (body %s)", path, w.Code, w.Body.String())
		}
		var body struct {
			Alerts []any `json:"alerts"`
			Status struct {
				State   string `json:"state"`
				Enabled bool   `json:"enabled"`
				Reason  string `json:"reason"`
			} `json:"status"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("GET %s decode: %v (body %s)", path, err, w.Body.String())
		}
		st := body.Status
		if path == "/api/wx/status" {
			if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
				t.Fatalf("GET %s decode status: %v", path, err)
			}
		}
		if st.State != "off" {
			t.Errorf("GET %s state = %q, want \"off\"", path, st.State)
		}
		if st.Enabled {
			t.Errorf("GET %s enabled = true, want false", path)
		}
		if st.Reason != "disabled" {
			t.Errorf("GET %s reason = %q, want \"disabled\"", path, st.Reason)
		}
		if body.Alerts == nil && path == "/api/wx/alerts" {
			t.Errorf("GET %s alerts = null, want []", path)
		}
	}
}

// Enabled and validly configured, but the manager failed to start, is a
// DIFFERENT operator fix than "turned off" — the feature is not off and
// saying so would send them to the wrong place. config.Validate already
// rejects enabled-without-contact, so this is the reachable variant.
func TestWxStatusReasonInitFailed(t *testing.T) {
	srv, _, sessMgr, cfgMgr := newTestWxServer(t, nil)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	cfg := cfgMgr.Get()
	cfg.WxAlerts.Enabled = true
	cfg.WxAlerts.Contact = "ops@example.com"
	if err := cfgMgr.Update(cfg); err != nil {
		t.Fatalf("config update: %v", err)
	}

	r := httptest.NewRequest("GET", "/api/wx/status", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET /api/wx/status = %d, want 200", w.Code)
	}
	var st struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.State != "off" || st.Reason != "initFailed" {
		t.Errorf("state=%q reason=%q, want off/initFailed", st.State, st.Reason)
	}
}

// Everything that mutates or needs live upstream data still 503s — a
// disabled feature genuinely cannot ack, relay, or resolve a zone.
func TestWxRoutes503WithoutManager(t *testing.T) {
	srv, _, sessMgr, _ := newTestWxServer(t, nil)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	routes := []struct{ method, path string }{
		{"GET", "/api/wx/alerts/x"},
		{"GET", "/api/wx/footprint"},
		{"GET", "/api/wx/event-types"},
		{"GET", "/api/wx/zones/search?state=MI"},
		{"GET", "/api/wx/zones/MIC081"},
		{"POST", "/api/wx/alerts/x/ack"},
	}
	for _, rt := range routes {
		r := httptest.NewRequest(rt.method, rt.path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Errorf("%s %s = %d, want 503 (body %s)", rt.method, rt.path, w.Code, w.Body.String())
		}
	}
}

// --- GET /wx/alerts ---------------------------------------------------------

func TestGetWxAlertsSnapshotIsCamelCase(t *testing.T) {
	mgr := newTestWxManager(t)
	seedInAreaAlert(t, mgr, "urn:oid:1", "Tornado Warning", wxalert.TierWarning)

	srv, _, sessMgr, _ := newTestWxServer(t, mgr)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	r := httptest.NewRequest("GET", "/api/wx/alerts", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	for _, key := range []string{`"effectiveEvent"`, `"notifyClass"`, `"floorText"`, `"fetchedAt"`} {
		if !strings.Contains(raw, key) {
			t.Errorf("snapshot JSON missing camelCase key %s:\n%s", key, raw)
		}
	}
	var snap wxalert.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if len(snap.Alerts) != 1 || snap.Alerts[0].Proximity != wxalert.ProximityIn {
		t.Errorf("Alerts = %+v, want one in-area alert", snap.Alerts)
	}
}

// --- Local ack --------------------------------------------------------------

func TestAckWxAlertReturns204(t *testing.T) {
	mgr := newTestWxManager(t)
	seedInAreaAlert(t, mgr, "urn:oid:ack1", "Flood Warning", wxalert.TierWarning)

	srv, _, sessMgr, _ := newTestWxServer(t, mgr)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	r := httptest.NewRequest("POST", "/api/wx/alerts/urn:oid:ack1/ack", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 204 {
		t.Fatalf("status = %d, want 204 (body %s)", w.Code, w.Body.String())
	}
}

func TestAckWxAlertUnknown404(t *testing.T) {
	mgr := newTestWxManager(t)
	srv, _, sessMgr, _ := newTestWxServer(t, mgr)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	r := httptest.NewRequest("POST", "/api/wx/alerts/no-such-id/ack", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// TestWxAlertRoutesResolvePercentEncodedID pins the real client shape: every
// NWS alert id is "urn:oid:..." and lib/api.ts builds these URLs with
// encodeURIComponent(id), which percent-encodes the colons. chi v5 captures
// {id} from r.URL.RawPath (not the decoded r.URL.Path) whenever the two
// differ — exactly the case for an encoded colon, since ':' doesn't need
// escaping in a path segment — so a handler reading chi.URLParam(r, "id")
// directly gets "urn%3Aoid%3A..." and never matches a stored alert. Every
// /wx/alerts/{id}... handler must decode through wxAlertIDParam instead.
// A regression here means Acknowledge, Ack for net and Relay all silently
// 404 for every real alert while every other test (which hand-builds the
// path with a literal, un-encoded colon) keeps passing.
func TestWxAlertRoutesResolvePercentEncodedID(t *testing.T) {
	mgr := newTestWxManager(t)
	seedInAreaAlert(t, mgr, "urn:oid:enc1", "Flood Warning", wxalert.TierWarning)

	srv, _, sessMgr, _ := newTestWxServer(t, mgr)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	// JS encodeURIComponent (what lib/api.ts actually calls) percent-encodes
	// ':' — Go's url.PathEscape does not, since a colon is a legal
	// unreserved character in a URL path segment, so it can't be used here
	// to build the reproducing request; build the exact bytes by hand.
	encoded := strings.ReplaceAll("urn:oid:enc1", ":", "%3A")

	r := httptest.NewRequest("GET", "/api/wx/alerts/"+encoded, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET /wx/alerts/%s = %d, want 200 (body %s)", encoded, w.Code, w.Body.String())
	}

	r2 := httptest.NewRequest("POST", "/api/wx/alerts/"+encoded+"/ack", nil)
	r2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, r2)
	if w2.Code != 204 {
		t.Fatalf("POST /wx/alerts/%s/ack = %d, want 204 (body %s)", encoded, w2.Code, w2.Body.String())
	}
}

// --- NCS ack-for-net (decision 5) -------------------------------------------

func TestAckWxAlertForNet(t *testing.T) {
	mgr := newTestWxManager(t)
	seedInAreaAlert(t, mgr, "urn:oid:net1", "Tornado Warning", wxalert.TierWarning)

	srv, netMgr, sessMgr, _ := newTestWxServer(t, mgr)

	ncs, ncsToken := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	_, otherToken := userWithRole(t, sessMgr, "OTHER", session.RoleOperator)
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	n, err := netMgr.CreateNet(store.Net{Name: "Test Net", NCSCallsign: "W8ABC", NCSUserID: ncs.ID})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}

	// Observer forbidden by role (RequireRole(RoleOperator), not the handler).
	r := httptest.NewRequest("POST", "/api/wx/alerts/urn:oid:net1/ack-net", nil)
	r.Header.Set("Authorization", "Bearer "+obsToken)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("observer ack-net = %d, want 403", w.Code)
	}

	// A non-NCS operator is refused by allowNetControlAction.
	r = httptest.NewRequest("POST", "/api/wx/alerts/urn:oid:net1/ack-net", nil)
	r.Header.Set("Authorization", "Bearer "+otherToken)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("non-NCS operator ack-net = %d, want 403 (body %s)", w.Code, w.Body.String())
	}

	// The NCS succeeds.
	r = httptest.NewRequest("POST", "/api/wx/alerts/urn:oid:net1/ack-net", nil)
	r.Header.Set("Authorization", "Bearer "+ncsToken)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("NCS ack-net = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got wxalert.MatchedAlert
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.AckedForNet == nil || got.AckedForNet.Callsign != "W8ABC" {
		t.Errorf("AckedForNet = %+v, want callsign W8ABC", got.AckedForNet)
	}

	// Unknown alert 404s under the same NCS gate.
	r = httptest.NewRequest("POST", "/api/wx/alerts/no-such-id/ack-net", nil)
	r.Header.Set("Authorization", "Bearer "+ncsToken)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("unknown alert ack-net = %d, want 404", w.Code)
	}

	// Idempotent: acking again succeeds (does not error just because it was
	// already acked).
	r = httptest.NewRequest("POST", "/api/wx/alerts/urn:oid:net1/ack-net", nil)
	r.Header.Set("Authorization", "Bearer "+ncsToken)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("second ack-net = %d, want 200 (idempotent)", w.Code)
	}
}

// --- Relay ------------------------------------------------------------------

func TestRelayWxAlertNoteOnlyWithoutMessageEngine(t *testing.T) {
	mgr := newTestWxManager(t)
	seedInAreaAlert(t, mgr, "urn:oid:relay1", "Winter Storm Warning", wxalert.TierWarning)

	srv, netMgr, sessMgr, _ := newTestWxServer(t, mgr)
	ncs, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n, _ := netMgr.CreateNet(store.Net{Name: "Test Net", NCSCallsign: "W8ABC", NCSUserID: ncs.ID})
	netMgr.OpenNet(n.ID)

	body, _ := json.Marshal(map[string]any{"note": true})
	r := httptest.NewRequest("POST", "/api/wx/alerts/urn:oid:relay1/relay", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("note-only relay = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var result wxRelayResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.NoteID == "" {
		t.Error("NoteID empty, want a posted note id")
	}

	notes, err := netMgr.GetNotes(n.ID)
	if err != nil || len(notes) != 1 {
		t.Fatalf("GetNotes = %v, %v; want 1 note", notes, err)
	}
	if notes[0].Category != "weather" || !notes[0].Pinned {
		t.Errorf("note = %+v, want category weather, pinned", notes[0])
	}
}

func TestRelayWxAlertBulletinWithoutMessageEngine503(t *testing.T) {
	mgr := newTestWxManager(t)
	seedInAreaAlert(t, mgr, "urn:oid:relay2", "Winter Storm Warning", wxalert.TierWarning)

	srv, netMgr, sessMgr, _ := newTestWxServer(t, mgr)
	ncs, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n, _ := netMgr.CreateNet(store.Net{Name: "Test Net", NCSCallsign: "W8ABC", NCSUserID: ncs.ID})
	netMgr.OpenNet(n.ID)

	body, _ := json.Marshal(map[string]any{"bulletin": true, "text": "Winter storm warning until 6pm"})
	r := httptest.NewRequest("POST", "/api/wx/alerts/urn:oid:relay2/relay", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 503 {
		t.Fatalf("bulletin relay without msgEngine = %d, want 503 (body %s)", w.Code, w.Body.String())
	}
}

func TestRelayWxAlertTextTooLong400(t *testing.T) {
	mgr := newTestWxManager(t)
	seedInAreaAlert(t, mgr, "urn:oid:relay3", "Winter Storm Warning", wxalert.TierWarning)

	srv, netMgr, sessMgr, _ := newTestWxServer(t, mgr)
	ncs, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n, _ := netMgr.CreateNet(store.Net{Name: "Test Net", NCSCallsign: "W8ABC", NCSUserID: ncs.ID})
	netMgr.OpenNet(n.ID)

	longText := strings.Repeat("x", 68)
	body, _ := json.Marshal(map[string]any{"bulletin": true, "text": longText})
	r := httptest.NewRequest("POST", "/api/wx/alerts/urn:oid:relay3/relay", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 400 {
		t.Fatalf("68-byte text relay = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
}

// --- Per-net weather watch --------------------------------------------------

func TestUpdateNetWxWatchNCSOnly(t *testing.T) {
	mgr := newTestWxManager(t)
	srv, netMgr, sessMgr, _ := newTestWxServer(t, mgr)

	ncs, ncsToken := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	_, otherToken := userWithRole(t, sessMgr, "OTHER", session.RoleOperator)
	n, _ := netMgr.CreateNet(store.Net{Name: "Test Net", NCSCallsign: "W8ABC", NCSUserID: ncs.ID})
	netMgr.OpenNet(n.ID)

	body, _ := json.Marshal(wxalert.NetWatch{NetID: n.ID, BufferMiles: 15})
	r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/wxwatch", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+otherToken)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("non-NCS PUT wxwatch = %d, want 403 (body %s)", w.Code, w.Body.String())
	}

	r = httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/wxwatch", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+ncsToken)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("NCS PUT wxwatch = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got wxalert.NetWatch
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.BufferMiles != 15 {
		t.Errorf("BufferMiles = %v, want 15", got.BufferMiles)
	}
	if got.Effective.FloorText != wxalert.FloorText() {
		t.Errorf("Effective.FloorText = %q, want the exact floor text", got.Effective.FloorText)
	}
}

func TestUpdateNetWxWatchBufferOutOfRange400(t *testing.T) {
	mgr := newTestWxManager(t)
	srv, netMgr, sessMgr, _ := newTestWxServer(t, mgr)

	ncs, token := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n, _ := netMgr.CreateNet(store.Net{Name: "Test Net", NCSCallsign: "W8ABC", NCSUserID: ncs.ID})
	netMgr.OpenNet(n.ID)

	body, _ := json.Marshal(wxalert.NetWatch{NetID: n.ID, BufferMiles: 1})
	r := httptest.NewRequest("PUT", "/api/nets/"+n.ID+"/wxwatch", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 400 {
		t.Fatalf("bufferMiles=1 PUT wxwatch = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
}

// --- Settings ----------------------------------------------------------------

func TestUpdateWxAlertsSettingsAdminOnly(t *testing.T) {
	mgr := newTestWxManager(t)
	srv, _, sessMgr, _ := newTestWxServer(t, mgr)

	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	_, adminToken := userWithRole(t, sessMgr, "ADMIN", session.RoleAdmin)

	body, _ := json.Marshal(map[string]any{
		"enabled": true, "pollInterval": "60s", "contact": "ops@example.com",
		"defaultBufferMiles": 10, "interruptEvents": []string{"Tornado Warning"},
		"watchNotify": "toast", "advisoryNotify": "badge", "statementNotify": "panel",
	})

	r := httptest.NewRequest("PUT", "/api/settings/wxalerts", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+opToken)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("operator PUT settings/wxalerts = %d, want 403", w.Code)
	}

	r = httptest.NewRequest("PUT", "/api/settings/wxalerts", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("admin PUT settings/wxalerts = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
}

func TestUpdateWxAlertsSettingsRejectsNonWarningEvent(t *testing.T) {
	mgr := newTestWxManager(t)
	srv, _, sessMgr, _ := newTestWxServer(t, mgr)
	_, adminToken := userWithRole(t, sessMgr, "ADMIN", session.RoleAdmin)

	body, _ := json.Marshal(map[string]any{
		"enabled": true, "pollInterval": "60s", "contact": "ops@example.com",
		"defaultBufferMiles": 10, "interruptEvents": []string{"Heat Advisory"},
		"watchNotify": "toast", "advisoryNotify": "badge", "statementNotify": "panel",
	})
	r := httptest.NewRequest("PUT", "/api/settings/wxalerts", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 400 {
		t.Fatalf("'Heat Advisory' in interruptEvents = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
}

// --- Event types --------------------------------------------------------------

func TestGetWxEventTypesCount(t *testing.T) {
	mgr := newTestWxManager(t)
	srv, _, sessMgr, _ := newTestWxServer(t, mgr)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	r := httptest.NewRequest("GET", "/api/wx/event-types", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var types []wxalert.EventType
	if err := json.Unmarshal(w.Body.Bytes(), &types); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(types) != 113 {
		t.Errorf("len(types) = %d, want 113 (111 known events + 2 derived emergencies)", len(types))
	}
}

// Settings calls default_zones "Home zones — always watched, even with no net
// open". They were validated and echoed back by the settings API but never
// reached the footprint, so with no net open they watched nothing.
func TestDefaultZonesWatchedWithNoNetOpen(t *testing.T) {
	mgr := newTestWxManager(t)
	srv, _, sessMgr, cfgMgr := newTestWxServer(t, mgr)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	cfg := cfgMgr.Get()
	cfg.WxAlerts.DefaultZones = []string{"MIC081", "INC003"}
	if err := cfgMgr.Update(cfg); err != nil {
		t.Fatalf("config update: %v", err)
	}
	// TriggerWxFootprintRefresh is debounced behind a loop Start() owns, so
	// drive the recompute directly.
	srv.refreshWxFootprint("test")

	r := httptest.NewRequest("GET", "/api/wx/footprint", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET /api/wx/footprint = %d (body %s)", w.Code, w.Body.String())
	}
	var body struct {
		Summary struct {
			ExtraZones []string `json:"extraZones"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %s)", err, w.Body.String())
	}
	got := strings.Join(body.Summary.ExtraZones, ",")
	if !strings.Contains(got, "MIC081") || !strings.Contains(got, "INC003") {
		t.Errorf("extraZones = %v, want the configured home zones with no net open", body.Summary.ExtraZones)
	}
}

// POST /wx/refresh used to only call PollNow, which re-entered the poller's
// empty-query skip when no watch area had resolved yet — so the one control
// an operator has to say "go and check" could not break a cold start out of
// "not monitoring". It must rebuild the footprint (and therefore the upstream
// query) first.
func TestWxRefreshRebuildsFootprint(t *testing.T) {
	mgr := newTestWxManager(t)
	srv, _, sessMgr, cfgMgr := newTestWxServer(t, mgr)
	_, token := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	// Configure home zones but never trigger a footprint rebuild, which is
	// exactly the cold-start state.
	cfg := cfgMgr.Get()
	cfg.WxAlerts.DefaultZones = []string{"MIC081"}
	if err := cfgMgr.Update(cfg); err != nil {
		t.Fatalf("config update: %v", err)
	}

	r := httptest.NewRequest("POST", "/api/wx/refresh", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 202 {
		t.Fatalf("POST /api/wx/refresh = %d (body %s)", w.Code, w.Body.String())
	}

	if got := mgr.Footprint().ExtraZones; len(got) == 0 {
		t.Errorf("footprint ExtraZones = %v after refresh, want the configured home zones", got)
	}
}

// ZoneCache.Prefetch existed, was unit-tested, and was never called by the
// running app — so every zone stayed uncached. The client only asks for zone
// polygons the server reports as cached (Map.svelte), so 85% of alerts (the
// zone-only ones) could never draw a polygon, and none of them survived going
// offline. Refreshing the footprint must warm the cache for its own zones.
func TestFootprintRefreshPrefetchesItsZones(t *testing.T) {
	var mu sync.Mutex
	zoneHits := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/zones/") {
			mu.Lock()
			zoneHits[r.URL.Path]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/geo+json")
			w.Write([]byte(`{"properties":{"id":"MIC081","name":"Kent","state":"MI","type":"county"},` +
				`"geometry":{"type":"Polygon","coordinates":[[[-85.8,42.8],[-85.4,42.8],[-85.4,43.1],[-85.8,43.1],[-85.8,42.8]]]}}`))
			return
		}
		w.Write([]byte(`{"features":[]}`))
	}))
	defer upstream.Close()

	mgr, err := wxalert.NewManager(wxalert.Config{
		Enabled:            true,
		UserAgent:          "Nymeria/test (ops@example.org)",
		BaseURL:            upstream.URL,
		ZoneDataDir:        t.TempDir(),
		DefaultBufferMiles: 10,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	srv, _, _, cfgMgr := newTestWxServer(t, mgr)
	cfg := cfgMgr.Get()
	cfg.WxAlerts.DefaultZones = []string{"MIC081"}
	if err := cfgMgr.Update(cfg); err != nil {
		t.Fatalf("config update: %v", err)
	}

	srv.refreshWxFootprint("test")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if rec, err := mgr.Zone(context.Background(), "MIC081", false); err == nil && rec != nil {
			return // cached without an upstream fetch: the prefetch warmed it
		}
		time.Sleep(25 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("MIC081 never became cached after a footprint refresh (zone hits: %v)", zoneHits)
}
