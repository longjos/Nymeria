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
	"github.com/narvel/nymeria/internal/netprofile"
	"github.com/narvel/nymeria/internal/ride/phase"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
)

// newTestRidePhaseServer builds a server with real net control, annotation,
// checkpoint, course, and ride-phase managers, wired the same way
// internal/app/app.go wires them. A sibling to newTestCourseServer, kept
// separate for the same reason that helper documents.
func newTestRidePhaseServer(t *testing.T) (*Server, *netcontrol.Manager, *phase.Manager, *session.MemoryManager) {
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
	phaseMgr := phase.NewManager(db, netMgr, courseMgr, cpMgr)

	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr),
		WithNetControlManager(netMgr),
		WithAnnotationManager(annMgr),
		WithCheckpointManager(cpMgr),
		WithCourseManager(courseMgr),
		WithPhaseManager(phaseMgr),
	)
	return srv, netMgr, phaseMgr, sessMgr
}

func createBikeRideNet(t *testing.T, netMgr *netcontrol.Manager, name string, ncsUserID string) *store.Net {
	t.Helper()
	n, err := netMgr.CreateNet(store.Net{Name: name, Profile: netprofile.ProfileBikeRide, NCSUserID: ncsUserID})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	return n
}

func TestRidePhaseUnavailable(t *testing.T) {
	// The plain net-control server (no phaseMgr wired) must answer 503, not
	// 404/500 — mirrors every other feature-gated route's guard.
	srv, netMgr, sessMgr := newTestNetServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: netprofile.ProfileBikeRide})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/phase", token, nil)
	if w.Code != 503 {
		t.Fatalf("GET /ride/phase (no manager) = %d, want 503 (body %s)", w.Code, w.Body.String())
	}
}

func TestGetRidePhaseDefaultsToPreStart(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	n := createBikeRideNet(t, netMgr, "Ride", "")
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/phase", token, nil)
	if w.Code != 200 {
		t.Fatalf("GET /ride/phase = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var st phase.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Phase != phase.PreStart {
		t.Errorf("Phase = %q, want %q", st.Phase, phase.PreStart)
	}
	if st.Suggestion.Phase != "" {
		t.Errorf("Suggestion = %+v, want empty", st.Suggestion)
	}
}

func TestGetRidePhaseGeneralNet409(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "General Net"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/phase", token, nil)
	if w.Code != 409 {
		t.Fatalf("GET /ride/phase (general net) = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] != "profile_mismatch" {
		t.Errorf("code = %q, want profile_mismatch", body["code"])
	}
}

func TestGetRidePhaseUnknownNet404(t *testing.T) {
	srv, _, _, sessMgr := newTestRidePhaseServer(t)
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "GET", "/api/nets/no-such-net/ride/phase", token, nil)
	if w.Code != 404 {
		t.Fatalf("GET /ride/phase (unknown net) = %d, want 404 (body %s)", w.Code, w.Body.String())
	}
}

func TestPostRidePhaseObserverForbidden(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	n := createBikeRideNet(t, netMgr, "Ride", "")
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", token, map[string]any{"phase": phase.Launched})
	if w.Code != 403 {
		t.Fatalf("observer POST /ride/phase = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
}

func TestPostRidePhaseNonNCSOperatorForbidden(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	ncs, _ := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n := createBikeRideNet(t, netMgr, "Ride", ncs.ID)
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", opToken, map[string]any{"phase": phase.Launched})
	if w.Code != 403 {
		t.Fatalf("non-NCS operator POST /ride/phase = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected an error message naming the net's NCS")
	}
}

func TestPostRidePhaseNCSAdvancesForward(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	ncs, ncsToken := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n := createBikeRideNet(t, netMgr, "Ride", ncs.ID)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", ncsToken, map[string]any{"phase": phase.Launched})
	if w.Code != 200 {
		t.Fatalf("NCS POST /ride/phase = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var st phase.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Phase != phase.Launched {
		t.Errorf("Phase = %q, want %q", st.Phase, phase.Launched)
	}
	if st.SetBy != "NCS" {
		t.Errorf("SetBy = %q, want %q", st.SetBy, "NCS")
	}
}

func TestPostRidePhaseAdminCanAlwaysAdvance(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	other, _ := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n := createBikeRideNet(t, netMgr, "Ride", other.ID)
	_, adminToken := userWithRole(t, sessMgr, "ADMIN", session.RoleAdmin)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", adminToken, map[string]any{"phase": phase.Launched})
	if w.Code != 200 {
		t.Fatalf("admin POST /ride/phase = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
}

func TestPostRidePhaseSkipForward409(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	ncs, ncsToken := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n := createBikeRideNet(t, netMgr, "Ride", ncs.ID)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", ncsToken, map[string]any{"phase": phase.MidRide})
	if w.Code != 409 {
		t.Fatalf("skip-forward POST /ride/phase = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] != "illegal_transition" {
		t.Errorf("code = %q, want illegal_transition", body["code"])
	}
}

func TestPostRidePhaseBackwardWithoutReason400(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	ncs, ncsToken := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n := createBikeRideNet(t, netMgr, "Ride", ncs.ID)

	if w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", ncsToken, map[string]any{"phase": phase.Launched}); w.Code != 200 {
		t.Fatalf("advance to launched = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", ncsToken, map[string]any{"phase": phase.PreStart})
	if w.Code != 400 {
		t.Fatalf("backward without reason = %d, want 400 (body %s)", w.Code, w.Body.String())
	}

	w2 := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", ncsToken,
		map[string]any{"phase": phase.PreStart, "reason": "advanced too early"})
	if w2.Code != 200 {
		t.Fatalf("backward with reason = %d, want 200 (body %s)", w2.Code, w2.Body.String())
	}
}

func TestPostRidePhaseInvalidPhase400(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	ncs, ncsToken := userWithRole(t, sessMgr, "NCS", session.RoleOperator)
	n := createBikeRideNet(t, netMgr, "Ride", ncs.ID)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", ncsToken, map[string]any{"phase": "warp-speed"})
	if w.Code != 400 {
		t.Fatalf("invalid phase = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
}

func TestPostRidePhaseGeneralNet409(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRidePhaseServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "General Net"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, token := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/phase", token, map[string]any{"phase": phase.Launched})
	if w.Code != 409 {
		t.Fatalf("POST /ride/phase (general net) = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
}
