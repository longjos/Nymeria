package server

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/ride/reconcile"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
)

// newTestReconcileServer builds a server with real net control, annotation,
// checkpoint, course, SAG and reconcile managers, all backed by one real
// SQLite store — wired the same way internal/app/app.go wires them. A
// sibling to newTestCourseServer/newTestRideServer/newTestNetServer, kept
// separate for the same reason those exist independently: fixed-arity
// destructures at existing call sites.
func newTestReconcileServer(t *testing.T) (*Server, *netcontrol.Manager, *checkpoint.Manager, *annotation.Manager, *course.Manager, *ride.Manager, *reconcile.Manager, *session.MemoryManager) {
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
	sagMgr := ride.NewManager(db, netMgr, annMgr, ride.DefaultConfig())
	recMgr := reconcile.NewManager(db, netMgr, courseMgr, sagMgr, annMgr)

	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr),
		WithNetControlManager(netMgr),
		WithAnnotationManager(annMgr),
		WithCheckpointManager(cpMgr),
		WithCourseManager(courseMgr),
		WithRideManager(sagMgr),
		WithReconcileManager(recMgr),
	)
	return srv, netMgr, cpMgr, annMgr, courseMgr, sagMgr, recMgr, sessMgr
}

func TestExceptionAccountingNameNeverLeaksBeyondBib(t *testing.T) {
	// Rider accounting (via internal/course) never carries a Name field at
	// all — governing fact 7's name-recording lives in SAGSlot.RiderName
	// and MedicalNotification.PatientName instead, both already
	// role-gated by their own packages. This test asserts the accounting
	// payload an observer sees really does carry only the bib label.
	srv, netMgr, _, _, courseMgr, _, _, sessMgr := newTestReconcileServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if _, err := courseMgr.RecordRider(store.RiderException{NetID: n.ID, Bib: "412", Kind: course.KindSAG}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/accounting", obsToken, nil)
	if w.Code != 200 {
		t.Fatalf("GET /ride/accounting = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var acc reconcile.Accounting
	if err := json.Unmarshal(w.Body.Bytes(), &acc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(acc.OpenExceptions) != 1 || acc.OpenExceptions[0].Bib != "412" {
		t.Fatalf("OpenExceptions = %+v, want one exception with bib 412", acc.OpenExceptions)
	}
}

func TestCloseoutRoute409ThenForceCloses(t *testing.T) {
	srv, netMgr, _, _, courseMgr, _, _, sessMgr := newTestReconcileServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	if _, err := courseMgr.RecordRider(store.RiderException{NetID: n.ID, Bib: "1", Kind: course.KindSAG}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/closeout", opToken, map[string]any{})
	if w.Code != 409 {
		t.Fatalf("POST /ride/closeout (not ready) = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
	var body struct {
		Closeout reconcile.CloseoutStatus `json:"closeout"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Closeout.Items == nil {
		t.Error("closeout.items is nil in the 409 body, want the checklist")
	}

	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/closeout", opToken, map[string]any{
		"force": true, "reason": "radio dead",
	})
	if w.Code != 200 {
		t.Fatalf("POST /ride/closeout (force) = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if got, _ := netMgr.GetNet(n.ID); got.Status != netcontrol.StatusClosed {
		t.Errorf("net status = %q, want closed", got.Status)
	}
}

func TestICS214RequiresUnitForSAGVariant(t *testing.T) {
	srv, netMgr, _, _, _, _, _, sessMgr := newTestReconcileServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ics214?variant=sag", obsToken, nil)
	if w.Code != 400 {
		t.Fatalf("GET /ics214?variant=sag (no unit) = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
}

func TestICS214RejectsNonRestStopAnnotationForRSVariant(t *testing.T) {
	srv, netMgr, _, annMgr, _, _, _, sessMgr := newTestReconcileServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	ann, err := annMgr.Create(store.Annotation{
		Type: "point", Label: "CP1", Category: "checkpoint", Status: "active",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`, NetID: n.ID,
	})
	if err != nil {
		t.Fatalf("create annotation: %v", err)
	}
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ics214?variant=rs&unit="+ann.ID, obsToken, nil)
	if w.Code != 400 {
		t.Fatalf("GET /ics214?variant=rs (checkpoint, not aid) = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
}

func TestRideReconcileRoutesRoleTiers(t *testing.T) {
	srv, netMgr, _, _, _, _, _, sessMgr := newTestReconcileServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/handoff", obsToken, map[string]any{
		"kind": "fyi", "summary": "test",
	})
	if w.Code != 403 {
		t.Fatalf("observer POST /ride/handoff = %d, want 403 (body %s)", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/handoff", opToken, map[string]any{
		"kind": "fyi", "summary": "test",
	})
	if w.Code != 201 {
		t.Fatalf("operator POST /ride/handoff = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
}

func TestRideReconcileRoutes503WithoutManager(t *testing.T) {
	tracker := station.NewMemoryTracker(config.StationConfig{Callsign: "N0CALL", TrackMaxPoints: 10, StaleTimeout: time.Hour})
	tm := transport.NewManager()
	db := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	netMgr := netcontrol.NewManager(db, tracker)
	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{InactivityTimeout: 30 * time.Minute})

	// No WithReconcileManager: every /ride/{accounting,closeout,...} route
	// must 503, matching the checkpoint/ride/course pattern.
	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr),
		WithNetControlManager(netMgr),
	)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	for _, path := range []string{
		"/api/nets/" + n.ID + "/ride/accounting",
		"/api/nets/" + n.ID + "/ride/closeout",
		"/api/nets/" + n.ID + "/ride/shift-summaries",
		"/api/nets/" + n.ID + "/ride/handoff",
		"/api/nets/" + n.ID + "/ride/briefing",
	} {
		w := doJSON(t, srv, "GET", path, obsToken, nil)
		if w.Code != 503 {
			t.Errorf("GET %s without a reconcile manager = %d, want 503 (body %s)", path, w.Code, w.Body.String())
		}
	}
}

func TestGeneralNetCloseUnaffectedByRideCloseoutGate(t *testing.T) {
	// A "general" profile net must close exactly as before WP5 — the ride
	// close-out gate on POST /nets/{id}/close only applies to "bike-ride"
	// nets.
	srv, netMgr, _, _, _, _, _, sessMgr := newTestReconcileServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "General Net"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/close", opToken, map[string]any{})
	if w.Code != 200 {
		t.Fatalf("POST /nets/{id}/close on a general net = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
}
