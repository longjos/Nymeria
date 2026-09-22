package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
	"github.com/narvel/nymeria/internal/transport"
)

// newTestRideServer builds a server with a real net control manager and a
// real ride.Manager, both backed by a real SQLite store. It is a sibling to
// newTestNetServer (net_test.go), kept separate rather than changing that
// helper's signature — a 3-value destructure at 20+ existing call sites
// would need to change for a helper only sag_test.go needs.
func newTestRideServer(t *testing.T) (*Server, *netcontrol.Manager, *ride.Manager, *session.MemoryManager) {
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
	rideMgr := ride.NewManager(db, netMgr, annMgr, ride.DefaultConfig())

	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr),
		WithNetControlManager(netMgr),
		WithAnnotationManager(annMgr),
		WithRideManager(rideMgr),
	)
	return srv, netMgr, rideMgr, sessMgr
}

func doJSON(t *testing.T, srv *Server, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, reader)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

func createSAGVehicle(t *testing.T, netMgr *netcontrol.Manager, netID, tactical string) *store.NetCheckIn {
	t.Helper()
	ci, err := netMgr.CheckIn(netID, "K"+tactical, netcontrol.TrafficNone, netcontrol.CatSAG)
	if err != nil {
		t.Fatalf("CheckIn: %v", err)
	}
	ci.TacticalCall = tactical
	updated, err := netMgr.UpdateCheckIn(*ci)
	if err != nil {
		t.Fatalf("UpdateCheckIn: %v", err)
	}
	return updated
}

func TestSAGRoutes_503WhenNoManager(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t) // no ride manager wired
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
	n, err := netMgr.CreateNet(store.Net{Name: "Net", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}

	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/sag", token, nil)
	if w.Code != 503 {
		t.Fatalf("GET /sag status = %d, want 503 (body %s)", w.Code, w.Body.String())
	}
}

func TestSAGRoutes_RoleTiers(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRideServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	// Observer: read endpoints OK.
	if w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/sag", obsToken, nil); w.Code != 200 {
		t.Errorf("observer GET /sag = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	// Observer: write endpoints forbidden.
	createBody := ride.CreateRequestInput{
		Pickup:  store.SAGLocation{Kind: ride.LocCourse},
		Dropoff: store.SAGLocation{Kind: ride.LocNextRestStop},
	}
	if w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/sag/requests", obsToken, createBody); w.Code != 403 {
		t.Errorf("observer POST /sag/requests = %d, want 403 (body %s)", w.Code, w.Body.String())
	}

	// Operator: write endpoint succeeds (201).
	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/sag/requests", opToken, createBody)
	if w.Code != 201 {
		t.Fatalf("operator POST /sag/requests = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	var req store.SAGRequest
	if err := json.Unmarshal(w.Body.Bytes(), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Operator: update (200) on the request it just created.
	updateBody := map[string]any{
		"pickup":  store.SAGLocation{Kind: ride.LocCourse},
		"dropoff": store.SAGLocation{Kind: ride.LocNextRestStop},
		"notes":   "updated",
	}
	if w := doJSON(t, srv, "PUT", "/api/nets/"+n.ID+"/sag/requests/"+req.ID, opToken, updateBody); w.Code != 200 {
		t.Errorf("operator PUT /sag/requests/{id} = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	// Unauthenticated: 401.
	if w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/sag", "", nil); w.Code != 401 {
		t.Errorf("no-auth GET /sag = %d, want 401 (body %s)", w.Code, w.Body.String())
	}
	if w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/sag/requests", "", createBody); w.Code != 401 {
		t.Errorf("no-auth POST /sag/requests = %d, want 401 (body %s)", w.Code, w.Body.String())
	}
}

func TestSAGDispatch_OverCapacity409(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRideServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	sag2 := createSAGVehicle(t, netMgr, n.ID, "SAG2")

	w := doJSON(t, srv, "PUT", "/api/nets/"+n.ID+"/sag/vehicles/"+sag2.ID, opToken, map[string]any{"seats": 1, "rackSlots": 1})
	if w.Code != 200 {
		t.Fatalf("PUT vehicle = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	createBody := ride.CreateRequestInput{
		Pickup: store.SAGLocation{Kind: ride.LocCourse}, Dropoff: store.SAGLocation{Kind: ride.LocNextRestStop},
		Slots: []ride.SlotInput{{Bib: "A"}, {Bib: "B"}},
	}
	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/sag/requests", opToken, createBody)
	if w.Code != 201 {
		t.Fatalf("create request = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	var req store.SAGRequest
	if err := json.Unmarshal(w.Body.Bytes(), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}

	dispatchBody := ride.DispatchInput{
		VehicleCheckInID: sag2.ID,
		SlotIDs:          []string{req.Slots[0].ID, req.Slots[1].ID},
	}
	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/sag/requests/"+req.ID+"/legs", opToken, dispatchBody)
	if w.Code != 409 {
		t.Fatalf("dispatch over capacity = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"committedSeats", "seats", "committedRacks", "rackSlots"} {
		if _, ok := body[key]; !ok {
			t.Errorf("409 body missing %q: %s", key, w.Body.String())
		}
	}
}

func TestSAGCreate_RequestedByDefaultsToSessionUser(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRideServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, opToken := userWithRole(t, sessMgr, "K1OP", session.RoleOperator)

	createBody := ride.CreateRequestInput{
		Pickup: store.SAGLocation{Kind: ride.LocCourse}, Dropoff: store.SAGLocation{Kind: ride.LocNextRestStop},
	}
	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/sag/requests", opToken, createBody)
	if w.Code != 201 {
		t.Fatalf("create request = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	var req store.SAGRequest
	if err := json.Unmarshal(w.Body.Bytes(), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.RequestedBy != "K1OP" {
		t.Errorf("RequestedBy = %q, want the session user's name %q", req.RequestedBy, "K1OP")
	}
}

func TestSAGBoard_NeverNullSlices(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRideServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/sag", token, nil)
	if w.Code != 200 {
		t.Fatalf("GET /sag = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if containsNull(w.Body.String()) {
		t.Errorf("empty board contains null: %s", w.Body.String())
	}

	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	createBody := ride.CreateRequestInput{
		Pickup: store.SAGLocation{Kind: ride.LocCourse}, Dropoff: store.SAGLocation{Kind: ride.LocNextRestStop},
	}
	if w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/sag/requests", opToken, createBody); w.Code != 201 {
		t.Fatalf("create request = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	createSAGVehicle(t, netMgr, n.ID, "SAG2")

	w = doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/sag", token, nil)
	if w.Code != 200 {
		t.Fatalf("GET /sag (2) = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if containsNull(w.Body.String()) {
		t.Errorf("populated board contains null: %s", w.Body.String())
	}
}

func containsNull(body string) bool {
	// A crude but effective check: the string "null" should never appear in
	// a well-formed SAGBoard response (every slice is normalized to []).
	for i := 0; i+4 <= len(body); i++ {
		if body[i:i+4] == "null" {
			return true
		}
	}
	return false
}

func TestBridge_SagVehicleCheckoutReleasesLegs(t *testing.T) {
	srv, netMgr, rideMgr, sessMgr := newTestRideServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	sag2 := createSAGVehicle(t, netMgr, n.ID, "SAG2")

	req, err := rideMgr.CreateRequest(n.ID, ride.CreateRequestInput{
		Pickup: store.SAGLocation{Kind: ride.LocCourse}, Dropoff: store.SAGLocation{Kind: ride.LocNextRestStop},
	}, "")
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	req, err = rideMgr.Dispatch(n.ID, req.ID, ride.DispatchInput{VehicleCheckInID: sag2.ID, SlotIDs: []string{req.Slots[0].ID}}, "NCS")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	legID := req.Legs[0].ID

	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/checkout/"+sag2.ID, opToken, nil)
	if w.Code != 200 {
		t.Fatalf("checkout = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	deadline := time.After(2 * time.Second)
	for {
		got, ok := rideMgr.GetRequest(n.ID, req.ID)
		if ok {
			for _, l := range got.Legs {
				if l.ID == legID && l.Status == ride.LegReleased {
					return
				}
			}
		}
		select {
		case <-deadline:
			t.Fatalf("leg was not released after checkout bridge; got = %+v", got)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
