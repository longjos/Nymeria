package server

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
)

// newTestCourseServer builds a server with real net control, annotation,
// checkpoint and course managers, all backed by a real SQLite store, wired
// the same way internal/app/app.go wires them (SetOnPassage/SetStatusGuard).
// A sibling to newTestRideServer/newTestNetServer, kept separate for the same
// reason: those helpers' fixed-arity destructures are used at 20+ existing
// call sites.
func newTestCourseServer(t *testing.T) (*Server, *netcontrol.Manager, *checkpoint.Manager, *annotation.Manager, *course.Manager, *session.MemoryManager) {
	t.Helper()

	tracker := station.NewMemoryTracker(config.StationConfig{
		Callsign:       "N0CALL",
		TrackMaxPoints: 10,
		StaleTimeout:   time.Hour,
	})
	tm := transport.NewManager()

	db := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	netMgr := netcontrol.NewManager(db, tracker)
	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{InactivityTimeout: 30 * time.Minute})
	annMgr := annotation.NewManager(db)
	cpMgr := checkpoint.NewManager(db, annMgr)
	courseMgr := course.NewManager(db, cpMgr, annMgr)
	cpMgr.SetOnPassage(courseMgr.OnPassage)
	annMgr.SetStatusGuard(courseMgr.GuardAnnotationStatus)

	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr),
		WithNetControlManager(netMgr),
		WithAnnotationManager(annMgr),
		WithCheckpointManager(cpMgr),
		WithCourseManager(courseMgr),
	)
	return srv, netMgr, cpMgr, annMgr, courseMgr, sessMgr
}

// seedCourseStation creates one aid annotation with CheckpointMeta at the
// given sequence number for netID, directly through the managers (not HTTP)
// exactly like internal/course/course_test.go's seedStations helper.
func seedCourseStation(t *testing.T, cpMgr *checkpoint.Manager, annMgr *annotation.Manager, netID string, seq int) *store.Annotation {
	t.Helper()
	ann, err := annMgr.Create(store.Annotation{
		Type:     "point",
		Label:    "Rest Stop 2",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
		Category: "aid",
		Status:   "active",
		NetID:    netID,
	})
	if err != nil {
		t.Fatalf("create station: %v", err)
	}
	if _, err := cpMgr.SetMeta(store.CheckpointMeta{
		AnnotationID:   ann.ID,
		NetID:          netID,
		SequenceNumber: seq,
	}); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	return ann
}

func TestCourseRoutes_Authorization(t *testing.T) {
	t.Run("observer GET course 200", func(t *testing.T) {
		srv, netMgr, _, _, _, sessMgr := newTestCourseServer(t)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
		w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/course", obsToken, nil)
		if w.Code != 200 {
			t.Fatalf("observer GET /course = %d, want 200 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("observer POST sweep 403", func(t *testing.T) {
		srv, netMgr, _, _, _, sessMgr := newTestCourseServer(t)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
		w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/course/sweep", obsToken, map[string]any{"reportedBy": "SWEEP1"})
		if w.Code != 403 {
			t.Fatalf("observer POST /course/sweep = %d, want 403 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("operator POST sweep 201", func(t *testing.T) {
		srv, netMgr, _, _, _, sessMgr := newTestCourseServer(t)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)
		w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/course/sweep", opToken, map[string]any{"reportedBy": "SWEEP1"})
		if w.Code != 201 {
			t.Fatalf("operator POST /course/sweep = %d, want 201 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("operator override close forbidden", func(t *testing.T) {
		srv, netMgr, cpMgr, annMgr, _, sessMgr := newTestCourseServer(t)
		ncs, _ := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		// A net with an NCS assigned to someone else — an ordinary operator
		// who is not that NCS may not exercise the override.
		n, err := netMgr.CreateNet(store.Net{Name: "Ride", NCSUserID: ncs.ID})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		st := seedCourseStation(t, cpMgr, annMgr, n.ID, 1)
		_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

		w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/course/stations/"+st.ID+"/close", opToken,
			map[string]any{"override": true, "reason": "sweep skipped this loop"})
		if w.Code != 403 {
			t.Fatalf("operator override close = %d, want 403 (body %s)", w.Code, w.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["code"] != "ncs_or_admin_required" {
			t.Errorf("code = %q, want ncs_or_admin_required", body["code"])
		}
	})

	t.Run("NCS override close 200", func(t *testing.T) {
		srv, netMgr, cpMgr, annMgr, _, sessMgr := newTestCourseServer(t)
		ncs, ncsToken := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride", NCSUserID: ncs.ID})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		st := seedCourseStation(t, cpMgr, annMgr, n.ID, 1)

		w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/course/stations/"+st.ID+"/close", ncsToken,
			map[string]any{"override": true, "reason": "sweep skipped this loop"})
		if w.Code != 200 {
			t.Fatalf("NCS override close = %d, want 200 (body %s)", w.Code, w.Body.String())
		}
	})

	t.Run("admin override close 200", func(t *testing.T) {
		srv, netMgr, cpMgr, annMgr, _, sessMgr := newTestCourseServer(t)
		other, _ := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
		n, err := netMgr.CreateNet(store.Net{Name: "Ride", NCSUserID: other.ID})
		if err != nil {
			t.Fatalf("CreateNet: %v", err)
		}
		st := seedCourseStation(t, cpMgr, annMgr, n.ID, 1)
		_, adminToken := userWithRole(t, sessMgr, "ADMIN", session.RoleAdmin)

		w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/course/stations/"+st.ID+"/close", adminToken,
			map[string]any{"override": true, "reason": "sweep skipped this loop"})
		if w.Code != 200 {
			t.Fatalf("admin override close = %d, want 200 (body %s)", w.Code, w.Body.String())
		}
	})
}

func TestCloseStation_GateReturns409(t *testing.T) {
	srv, netMgr, cpMgr, annMgr, _, sessMgr := newTestCourseServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	st := seedCourseStation(t, cpMgr, annMgr, n.ID, 1)
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/course/stations/"+st.ID+"/close", opToken, map[string]any{})
	if w.Code != 409 {
		t.Fatalf("close before sweep = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] != "sweep_not_passed" {
		t.Errorf("code = %v, want sweep_not_passed", body["code"])
	}
	if body["stationLabel"] != "Rest Stop 2" {
		t.Errorf("stationLabel = %v, want %q", body["stationLabel"], "Rest Stop 2")
	}
}

func TestAnnotationStatusRoute_GateReturns409(t *testing.T) {
	srv, netMgr, cpMgr, annMgr, _, sessMgr := newTestCourseServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	st := seedCourseStation(t, cpMgr, annMgr, n.ID, 1)
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	w := doJSON(t, srv, "POST", "/api/annotations/"+st.ID+"/status", opToken, map[string]any{"status": "closed"})
	if w.Code != 409 {
		t.Fatalf("POST /annotations/{id}/status closed = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] != "sweep_not_passed" {
		t.Errorf("code = %q, want sweep_not_passed", body["code"])
	}
}

func TestFireShutoffRoute(t *testing.T) {
	srv, netMgr, _, _, _, sessMgr := newTestCourseServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	created := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/course/shutoffs", opToken, map[string]any{
		"name":                "Benson Shutoff",
		"lat":                 41.79474,
		"lon":                 -111.90586,
		"scheduledAt":         time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC),
		"rerouteDirection":    "West",
		"rerouteDestination":  "55-mile route",
		"rerouteInstructions": "all riders not past this point directed West onto the 55-mile route",
	})
	if created.Code != 201 {
		t.Fatalf("create shutoff = %d, want 201 (body %s)", created.Code, created.Body.String())
	}
	var sp store.ShutoffPoint
	if err := json.Unmarshal(created.Body.Bytes(), &sp); err != nil {
		t.Fatalf("decode created shutoff: %v", err)
	}

	bibs := []string{"101", "102"}
	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/course/shutoffs/"+sp.ID+"/fire", opToken, map[string]any{
		"bibs":         bibs,
		"rerouteCount": 12,
	})
	if w.Code != 200 {
		t.Fatalf("fire shutoff = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var body struct {
		Shutoff store.ShutoffPoint     `json:"shutoff"`
		Riders  []store.RiderException `json:"riders"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode fire response: %v", err)
	}
	if body.Shutoff.Status != course.ShutoffFired {
		t.Errorf("shutoff.status = %q, want fired", body.Shutoff.Status)
	}
	if len(body.Riders) != len(bibs) {
		t.Errorf("riders length = %d, want %d", len(body.Riders), len(bibs))
	}

	evts := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/events", opToken, nil)
	if evts.Code != 200 {
		t.Fatalf("GET /events = %d, want 200 (body %s)", evts.Code, evts.Body.String())
	}
	var events []store.NetEvent
	if err := json.Unmarshal(evts.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == course.TimelineShutoffFired {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("events did not contain %q: %+v", course.TimelineShutoffFired, events)
	}
}

func TestCourseState_EmptyNetIsValidJSON(t *testing.T) {
	srv, netMgr, _, _, _, sessMgr := newTestCourseServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Fresh"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/course", obsToken, nil)
	if w.Code != 200 {
		t.Fatalf("GET /course = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"stations":[]`, `"shutoffs":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}

	var state course.CourseState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode course state: %v", err)
	}
	if state.Stations == nil || state.Shutoffs == nil {
		t.Errorf("Stations/Shutoffs must be non-nil, got %#v / %#v", state.Stations, state.Shutoffs)
	}
}
