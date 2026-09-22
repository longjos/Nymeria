package server

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/activity"
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

// newTestRideTrafficServer builds a server with a real net control manager
// and a real ride.TrafficManager, both backed by a real SQLite store, wired
// the same way internal/app/app.go wires them (netMgr satisfies
// ride.NetLookup/TimelineSink/NetPolicySource structurally). A sibling to
// newTestRideServer/newTestCourseServer, kept separate for the same reason
// those are: fixed-arity destructures at existing call sites.
func newTestRideTrafficServer(t *testing.T) (*Server, *netcontrol.Manager, *ride.TrafficManager, *session.MemoryManager) {
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
	actLogger := activity.NewStoreLogger(db)
	rideTraffic := ride.NewTrafficManager(db, netMgr, netMgr, actLogger, ride.NewNetProfilePolicy(netMgr))

	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr),
		WithNetControlManager(netMgr),
		WithAnnotationManager(annMgr),
		WithActivityLogger(actLogger),
		WithRideTrafficManager(rideTraffic),
	)
	return srv, netMgr, rideTraffic, sessMgr
}

// openBikeRideNet creates and opens a "bike-ride" profile net.
func openBikeRideNet(t *testing.T, netMgr *netcontrol.Manager) *store.Net {
	t.Helper()
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	n, _ = netMgr.GetNet(n.ID)
	return n
}

func TestRideRoutesRoleTiers(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRideTrafficServer(t)
	n := openBikeRideNet(t, netMgr)
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	if w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/supply", obsToken, nil); w.Code != 200 {
		t.Errorf("observer GET /ride/supply = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	createBody := ride.CreateSupplyInput{
		RequestedByCall: "RS3", Items: []store.SupplyItem{{Item: "ice", Quantity: 10}}, Priority: ride.PriorityMedium,
	}
	if w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/supply", obsToken, createBody); w.Code != 403 {
		t.Errorf("observer POST /ride/supply = %d, want 403 (body %s)", w.Code, w.Body.String())
	}

	// Create a severe medical notification with bib withholding forced on
	// via the net's ride config, so the observer list route's redaction can
	// be checked meaningfully.
	cfg, ok := netMgr.GetRideConfig(n.ID)
	if !ok {
		t.Fatalf("GetRideConfig: not found")
	}
	cfg.WithholdBibOnSevereInjury = true
	if _, err := netMgr.SetRideConfig(n.ID, *cfg); err != nil {
		t.Fatalf("SetRideConfig: %v", err)
	}
	medBody := ride.CreateMedicalInput{
		ReportedByCall: "SAG 2", Bib: "412", Sex: "M", Age: "40", Location: "mile 22",
		ChiefComplaint: "fall", Severity: store.SeveritySevere, Priority: ride.PriorityPriority,
	}
	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/medical", opToken, medBody)
	if w.Code != 201 {
		t.Fatalf("operator POST /ride/medical = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	var created store.MedicalNotification
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !created.BibWithheld {
		t.Fatalf("created.BibWithheld = false, want true")
	}

	w = doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/medical", obsToken, nil)
	if w.Code != 200 {
		t.Fatalf("observer GET /ride/medical = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var list []store.MedicalNotification
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0].Bib != "" {
		t.Errorf("observer list Bib = %q, want empty (withheld)", list[0].Bib)
	}
	if list[0].PatientName != "" {
		t.Errorf("observer list PatientName = %q, want empty", list[0].PatientName)
	}
	if strings.Contains(w.Body.String(), "412") {
		t.Errorf("observer list body leaks raw bib: %s", w.Body.String())
	}

	if w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/medical/"+created.ID, obsToken, nil); w.Code != 403 {
		t.Errorf("observer GET /ride/medical/{mid} = %d, want 403 (body %s)", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/medical/"+created.ID, opToken, nil)
	if w.Code != 200 {
		t.Fatalf("operator GET /ride/medical/{mid} = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var full store.MedicalNotification
	if err := json.Unmarshal(w.Body.Bytes(), &full); err != nil {
		t.Fatalf("decode full: %v", err)
	}
	if full.Bib != "412" {
		t.Errorf("operator full Bib = %q, want 412", full.Bib)
	}
}

func TestRideRoutes503WithoutManager(t *testing.T) {
	srv, netMgr, sessMgr := newTestNetServer(t) // no ride traffic manager wired
	_, token := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}

	routes := []struct {
		method, path string
	}{
		{"GET", "/api/nets/" + n.ID + "/ride/supply"},
		{"GET", "/api/nets/" + n.ID + "/ride/supply/x"},
		{"GET", "/api/nets/" + n.ID + "/ride/supply-catalog"},
		{"GET", "/api/nets/" + n.ID + "/ride/medical"},
	}
	for _, rt := range routes {
		w := doJSON(t, srv, rt.method, rt.path, token, nil)
		if w.Code != 503 {
			t.Errorf("%s %s = %d, want 503 (body %s)", rt.method, rt.path, w.Code, w.Body.String())
		}
	}

	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	writeRoutes := []struct {
		method, path string
	}{
		{"POST", "/api/nets/" + n.ID + "/ride/supply"},
		{"POST", "/api/nets/" + n.ID + "/ride/medical"},
		{"GET", "/api/nets/" + n.ID + "/ride/medical/x"},
	}
	for _, rt := range writeRoutes {
		w := doJSON(t, srv, rt.method, rt.path, opToken, map[string]any{})
		if w.Code != 503 {
			t.Errorf("%s %s = %d, want 503 (body %s)", rt.method, rt.path, w.Code, w.Body.String())
		}
	}
}

func TestRideTransitionErrorsMap(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRideTrafficServer(t)
	n := openBikeRideNet(t, netMgr)
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)

	// Unknown id -> 404.
	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/supply/unknown-id/relay", opToken, ride.RelayInput{RelayedTo: "SUPPLY 1"})
	if w.Code != 404 {
		t.Errorf("relay unknown id = %d, want 404 (body %s)", w.Code, w.Body.String())
	}

	// Validation error -> 400 (no items).
	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/supply", opToken, ride.CreateSupplyInput{RequestedByCall: "RS3", Priority: ride.PriorityMedium})
	if w.Code != 400 {
		t.Errorf("create with no items = %d, want 400 (body %s)", w.Code, w.Body.String())
	}

	// Illegal transition -> 409 (relay before readback).
	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/supply", opToken, ride.CreateSupplyInput{
		RequestedByCall: "RS3", Items: []store.SupplyItem{{Item: "ice"}}, Priority: ride.PriorityMedium,
	})
	if w.Code != 201 {
		t.Fatalf("create supply = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	var req store.SupplyRequest
	if err := json.Unmarshal(w.Body.Bytes(), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/supply/"+req.ID+"/relay", opToken, ride.RelayInput{RelayedTo: "SUPPLY 1"})
	if w.Code != 409 {
		t.Errorf("relay before readback = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
}

func TestRideListNeverNull(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRideTrafficServer(t)
	n := openBikeRideNet(t, netMgr)
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	for _, path := range []string{"/ride/supply", "/ride/medical"} {
		w := doJSON(t, srv, "GET", "/api/nets/"+n.ID+path, obsToken, nil)
		if w.Code != 200 {
			t.Fatalf("GET %s = %d, want 200 (body %s)", path, w.Code, w.Body.String())
		}
		body := strings.TrimSpace(w.Body.String())
		if body != "[]" {
			t.Errorf("GET %s body = %q, want []", path, body)
		}
	}
}

func TestRideOpenFilter(t *testing.T) {
	srv, netMgr, _, sessMgr := newTestRideTrafficServer(t)
	n := openBikeRideNet(t, netMgr)
	_, opToken := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	_, obsToken := userWithRole(t, sessMgr, "OBS", session.RoleObserver)

	// One open supply request, one cancelled.
	w := doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/supply", opToken, ride.CreateSupplyInput{
		RequestedByCall: "RS1", Items: []store.SupplyItem{{Item: "ice"}}, Priority: ride.PriorityMedium,
	})
	var open store.SupplyRequest
	json.Unmarshal(w.Body.Bytes(), &open)

	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/supply", opToken, ride.CreateSupplyInput{
		RequestedByCall: "RS2", Items: []store.SupplyItem{{Item: "water"}}, Priority: ride.PriorityMedium,
	})
	var toCancel store.SupplyRequest
	json.Unmarshal(w.Body.Bytes(), &toCancel)
	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/supply/"+toCancel.ID+"/cancel", opToken, ride.CancelInput{Reason: "not needed"})
	if w.Code != 200 {
		t.Fatalf("cancel = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/supply?status=open", obsToken, nil)
	var filtered []store.SupplyRequest
	if err := json.Unmarshal(w.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != open.ID {
		t.Errorf("filtered supply = %+v, want just %s", filtered, open.ID)
	}

	// One departed (terminal) medical notification, one still reported.
	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/medical", opToken, ride.CreateMedicalInput{
		ReportedByCall: "SAG 1", Sex: "U", Age: "unknown", Location: "x", ChiefComplaint: "y",
		Severity: store.SeverityRoutine, Priority: ride.PriorityPriority, ReadbackConfirmed: true,
	})
	var toDepart store.MedicalNotification
	json.Unmarshal(w.Body.Bytes(), &toDepart)
	doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/medical/"+toDepart.ID+"/on-scene", opToken, ride.OnSceneInput{})
	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/medical/"+toDepart.ID+"/depart", opToken, ride.DepartInput{
		Destination: store.DestHospital, DestinationName: "Marin General", PatientCount: 1,
	})
	if w.Code != 200 {
		t.Fatalf("depart = %d, want 200 (body %s)", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, "POST", "/api/nets/"+n.ID+"/ride/medical", opToken, ride.CreateMedicalInput{
		ReportedByCall: "SAG 2", Sex: "U", Age: "unknown", Location: "x", ChiefComplaint: "y",
		Severity: store.SeverityRoutine, Priority: ride.PriorityPriority,
	})
	var stillOpen store.MedicalNotification
	json.Unmarshal(w.Body.Bytes(), &stillOpen)

	w = doJSON(t, srv, "GET", "/api/nets/"+n.ID+"/ride/medical?status=open", obsToken, nil)
	var filteredMed []store.MedicalNotification
	if err := json.Unmarshal(w.Body.Bytes(), &filteredMed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(filteredMed) != 1 || filteredMed[0].ID != stillOpen.ID {
		t.Errorf("filtered medical = %+v, want just %s", filteredMed, stillOpen.ID)
	}
}
