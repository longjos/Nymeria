package server

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
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

func newFullRideServer(t *testing.T) (*Server, *netcontrol.Manager, *session.MemoryManager) {
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
	cpMgr := checkpoint.NewManager(db, annMgr)
	courseMgr := course.NewManager(db, cpMgr, annMgr)
	cpMgr.SetOnPassage(courseMgr.OnPassage)
	annMgr.SetStatusGuard(courseMgr.GuardAnnotationStatus)
	rideMgr := ride.NewManager(db, netMgr, annMgr, ride.DefaultConfig())
	rideMgr.SetTierSource(ride.NewNetProfileTiers(netMgr))
	traffic := ride.NewTrafficManager(db, netMgr, netMgr, nil, ride.NewNetProfilePolicy(netMgr))
	recMgr := reconcile.NewManager(db, netMgr, courseMgr, rideMgr, annMgr)
	srv := New(tracker, tm, message.Engine(nil), db,
		WithSessionManager(sessMgr), WithNetControlManager(netMgr), WithAnnotationManager(annMgr),
		WithCheckpointManager(cpMgr), WithCourseManager(courseMgr), WithRideManager(rideMgr),
		WithRideTrafficManager(traffic), WithReconcileManager(recMgr))
	return srv, netMgr, sessMgr
}

// findNulls walks decoded JSON and reports every path whose value is null.
func findNulls(v any, path string, out *[]string) {
	switch t := v.(type) {
	case nil:
		*out = append(*out, path)
	case map[string]any:
		for k, vv := range t {
			findNulls(vv, path+"."+k, out)
		}
	case []any:
		for i, vv := range t {
			findNulls(vv, path+"["+itoa(i)+"]", out)
		}
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestRideReadRoutesNeverReturnNullOnEmptyNet(t *testing.T) {
	srv, netMgr, sessMgr := newFullRideServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	_, tok := userWithRole(t, sessMgr, "OBS", session.RoleObserver)
	paths := []string{
		"/api/net-profiles", "/api/nets/{id}/profile", "/api/nets/{id}/ride-config",
		"/api/nets/{id}/sag", "/api/nets/{id}/sag/requests", "/api/nets/{id}/sag/vehicles", "/api/nets/{id}/sag/config",
		"/api/nets/{id}/ride/supply", "/api/nets/{id}/ride/supply-catalog", "/api/nets/{id}/ride/medical",
		"/api/nets/{id}/course", "/api/nets/{id}/course/config", "/api/nets/{id}/course/shutoffs",
		"/api/nets/{id}/course/riders", "/api/nets/{id}/course/sweep", "/api/nets/{id}/course/stations",
		"/api/nets/{id}/ride/accounting", "/api/nets/{id}/ride/closeout",
		"/api/nets/{id}/ride/shift-summaries", "/api/nets/{id}/ride/handoff",
		"/api/nets/{id}/ride/handoffs", "/api/nets/{id}/ride/briefing",
		"/api/nets/{id}/ics211", "/api/nets/{id}/ics214",
	}
	for _, p := range paths {
		real := strings.ReplaceAll(p, "{id}", n.ID)
		useTok := tok
		w := doJSON(t, srv, "GET", real, useTok, nil)
		if w.Code != 200 {
			t.Errorf("GET %s = %d body=%s", p, w.Code, strings.TrimSpace(w.Body.String()))
			continue
		}
		body := w.Body.String()
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Errorf("GET %s: bad json: %v", p, err)
			continue
		}
		if body == "null" || strings.TrimSpace(body) == "null" {
			t.Errorf("NULL-BODY %s -> %s", p, body)
			continue
		}
		var nulls []string
		findNulls(v, "", &nulls)
		if len(nulls) > 0 {
			t.Errorf("GET %s returned JSON null at %v — a nil Go slice freezes the Svelte UI (body %s)", p, nulls, body)
		}
	}
}

// TestRideReadRoutesNeverReturnNullAfterReload populates every new record type through the real
// HTTP API, then rebuilds all managers from the SAME db file (simulating a
// process restart) and re-probes every read route for nulls.
func TestRideReadRoutesNeverReturnNullAfterReload(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "reload.db")

	build := func() (*Server, *netcontrol.Manager, *session.MemoryManager, *store.SQLiteStore) {
		tracker := station.NewMemoryTracker(config.StationConfig{Callsign: "N0CALL", TrackMaxPoints: 10, StaleTimeout: time.Hour})
		tm := transport.NewManager()
		db := store.NewSQLiteStore(dbPath)
		if err := db.Init(); err != nil {
			t.Fatalf("store init: %v", err)
		}
		netMgr := netcontrol.NewManager(db, tracker)
		if err := netMgr.Load(); err != nil {
			t.Fatalf("netMgr.Load: %v", err)
		}
		sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{InactivityTimeout: 30 * time.Minute})
		annMgr := annotation.NewManager(db)
		if err := annMgr.Load(); err != nil {
			t.Fatalf("annMgr.Load: %v", err)
		}
		cpMgr := checkpoint.NewManager(db, annMgr)
		if err := cpMgr.Load(); err != nil {
			t.Fatalf("cpMgr.Load: %v", err)
		}
		courseMgr := course.NewManager(db, cpMgr, annMgr)
		if err := courseMgr.Load(); err != nil {
			t.Fatalf("courseMgr.Load: %v", err)
		}
		cpMgr.SetOnPassage(courseMgr.OnPassage)
		annMgr.SetStatusGuard(courseMgr.GuardAnnotationStatus)
		rideMgr := ride.NewManager(db, netMgr, annMgr, ride.DefaultConfig())
		rideMgr.SetTierSource(ride.NewNetProfileTiers(netMgr))
		if err := rideMgr.Load(); err != nil {
			t.Fatalf("rideMgr.Load: %v", err)
		}
		traffic := ride.NewTrafficManager(db, netMgr, netMgr, nil, ride.NewNetProfilePolicy(netMgr))
		if err := traffic.Load(); err != nil {
			t.Fatalf("traffic.Load: %v", err)
		}
		recMgr := reconcile.NewManager(db, netMgr, courseMgr, rideMgr, annMgr)
		if err := recMgr.Load(); err != nil {
			t.Fatalf("recMgr.Load: %v", err)
		}
		srv := New(tracker, tm, message.Engine(nil), db,
			WithSessionManager(sessMgr), WithNetControlManager(netMgr), WithAnnotationManager(annMgr),
			WithCheckpointManager(cpMgr), WithCourseManager(courseMgr), WithRideManager(rideMgr),
			WithRideTrafficManager(traffic), WithReconcileManager(recMgr))
		return srv, netMgr, sessMgr, db
	}

	srv, netMgr, sessMgr, db := build()
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	_, opTok := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	base := "/api/nets/" + n.ID

	post := func(p string, body any) {
		t.Helper()
		w := doJSON(t, srv, "POST", base+p, opTok, body)
		if w.Code < 200 || w.Code >= 300 {
			t.Errorf("POST %s = %d body=%s", p, w.Code, strings.TrimSpace(w.Body.String()))
		}
	}
	post("/sag/requests", map[string]any{
		"pickup":  map[string]any{"kind": "course", "label": "mile 40"},
		"dropoff": map[string]any{"kind": "next_reststop", "label": "RS4"},
		"reason":  "mechanical",
		"slots":   []map[string]any{{"bib": "412"}},
	})
	post("/course/riders", map[string]any{"bib": "99", "kind": "sag"})
	post("/course/sweep", map[string]any{"routeLabel": "100", "lastRiderBib": "99", "reportedBy": "SWEEP1"})
	post("/ride/supply", map[string]any{
		"requestedByCall": "RS3", "location": "RS3",
		"items": []map[string]any{{"item": "water", "quantity": 2}}, "priority": "high",
	})
	post("/ride/medical", map[string]any{
		"reportedByCall": "RS3", "location": "mile 40", "bib": "77",
		"severity": "routine", "chiefComplaint": "road rash",
	})
	post("/ride/handoff", map[string]any{"kind": "pending_action", "summary": "watch RS4 water"})

	db.Close()

	// Fresh process: everything reloaded from disk.
	srv2, _, sessMgr2, db2 := build()
	defer db2.Close()
	_, tok2 := userWithRole(t, sessMgr2, "OBS2", session.RoleObserver)

	paths := []string{
		"/profile", "/ride-config", "/sag", "/sag/requests", "/sag/vehicles", "/sag/config",
		"/ride/supply", "/ride/supply-catalog", "/ride/medical",
		"/course", "/course/config", "/course/shutoffs", "/course/riders", "/course/sweep", "/course/stations",
		"/ride/accounting", "/ride/closeout", "/ride/shift-summaries", "/ride/handoff",
		"/ride/handoffs", "/ride/briefing", "/ics211", "/ics214",
	}
	for _, p := range paths {
		w := doJSON(t, srv2, "GET", base+p, tok2, nil)
		if w.Code != 200 {
			t.Errorf("RELOAD GET %s = %d body=%s", p, w.Code, strings.TrimSpace(w.Body.String()))
			continue
		}
		body := strings.TrimSpace(w.Body.String())
		if body == "null" {
			t.Errorf("RELOAD NULL-BODY %s", p)
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Errorf("RELOAD %s bad json: %v", p, err)
			continue
		}
		var nulls []string
		findNulls(v, "", &nulls)
		if len(nulls) > 0 {
			t.Errorf("RELOAD NULLS %s -> %v", p, nulls)
		}
	}
}

// TestRideAgencyPriorityLadderIsHonouredEverywhere pins governing fact 8
// (the priority vocabulary is DATA, not hardcoded): once an agency
// configures its own ladder on a net, BOTH the supply/medical traffic path
// and the SAG path must validate against that ladder — and must reject a
// tier the agency removed from it. Before ride.Manager grew a TierSource,
// SAG validated against a process-wide ride.Config instead, so it rejected
// the agency's own tiers and accepted deleted ones.
func TestRideAgencyPriorityLadderIsHonouredEverywhere(t *testing.T) {
	srv, netMgr, sessMgr := newFullRideServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	_, opTok := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	base := "/api/nets/" + n.ID

	// The agency runs a three-rung ladder of its own naming, which shares
	// no tier id with the shipped Marin ladder.
	w := doJSON(t, srv, "PUT", base+"/ride-config", opTok, map[string]any{
		"priorityTiers": []map[string]any{
			{"id": "urgent", "label": "URGENT", "rank": 1, "description": "life safety"},
			{"id": "routine", "label": "ROUTINE", "rank": 2, "description": "normal"},
			{"id": "info", "label": "INFO", "rank": 3, "description": "fyi"},
		},
	})
	if w.Code != 200 {
		t.Fatalf("PUT /ride-config = %d body=%s", w.Code, w.Body.String())
	}

	supply := map[string]any{
		"requestedByCall": "RS3", "location": "RS3", "priority": "urgent",
		"items": []map[string]any{{"item": "water", "quantity": 2}},
	}
	if w := doJSON(t, srv, "POST", base+"/ride/supply", opTok, supply); w.Code != 201 {
		t.Errorf("supply with agency tier \"urgent\" = %d, want 201 (body %s)", w.Code, strings.TrimSpace(w.Body.String()))
	}

	sagWith := func(priority, bib string) *httptest.ResponseRecorder {
		return doJSON(t, srv, "POST", base+"/sag/requests", opTok, map[string]any{
			"pickup":   map[string]any{"kind": "course"},
			"dropoff":  map[string]any{"kind": "next_reststop"},
			"reason":   "mechanical",
			"priority": priority,
			"slots":    []map[string]any{{"bib": bib}},
		})
	}
	if w := sagWith("urgent", "412"); w.Code != 201 {
		t.Errorf("SAG with agency tier \"urgent\" = %d, want 201 (body %s)", w.Code, strings.TrimSpace(w.Body.String()))
	}
	if w := sagWith("emergency", "413"); w.Code != 400 {
		t.Errorf("SAG with tier \"emergency\" (not in this net's ladder) = %d, want 400 (body %s)", w.Code, strings.TrimSpace(w.Body.String()))
	}

	// The composer reads its dropdown from GET /sag/config, so that must
	// advertise the net's ladder, not the shipped one.
	w = doJSON(t, srv, "GET", base+"/sag/config", opTok, nil)
	var cfg struct {
		Config struct {
			Priorities []string `json:"priorities"`
		} `json:"config"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal sag config: %v", err)
	}
	want := []string{"urgent", "routine", "info"}
	if len(cfg.Config.Priorities) != len(want) {
		t.Fatalf("GET /sag/config priorities = %v, want %v", cfg.Config.Priorities, want)
	}
	for i := range want {
		if cfg.Config.Priorities[i] != want[i] {
			t.Fatalf("GET /sag/config priorities = %v, want %v", cfg.Config.Priorities, want)
		}
	}
}

// TestRideNetWithoutAgencyLadderKeepsShippedTiers is the other half of the
// contract: a bike-ride net whose agency configured no override, and any
// general net, keep ride.Config's shipped Marin ladder exactly as before.
func TestRideNetWithoutAgencyLadderKeepsShippedTiers(t *testing.T) {
	srv, netMgr, sessMgr := newFullRideServer(t)
	n, err := netMgr.CreateNet(store.Net{Name: "Ride", Profile: "bike-ride"})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := netMgr.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	_, opTok := userWithRole(t, sessMgr, "OP", session.RoleOperator)
	base := "/api/nets/" + n.ID

	w := doJSON(t, srv, "POST", base+"/sag/requests", opTok, map[string]any{
		"pickup":   map[string]any{"kind": "course"},
		"dropoff":  map[string]any{"kind": "next_reststop"},
		"reason":   "mechanical",
		"priority": "emergency",
		"slots":    []map[string]any{{"bib": "1"}},
	})
	if w.Code != 201 {
		t.Errorf("SAG with shipped tier \"emergency\" on an unconfigured net = %d, want 201 (body %s)", w.Code, strings.TrimSpace(w.Body.String()))
	}

	// And the default derivation still applies when no priority is sent:
	// pickup from the course is HIGH (governing fact 10).
	w = doJSON(t, srv, "POST", base+"/sag/requests", opTok, map[string]any{
		"pickup":  map[string]any{"kind": "course"},
		"dropoff": map[string]any{"kind": "next_reststop"},
		"reason":  "mechanical",
		"slots":   []map[string]any{{"bib": "2"}},
	})
	if w.Code != 201 {
		t.Fatalf("SAG without priority = %d, want 201 (body %s)", w.Code, strings.TrimSpace(w.Body.String()))
	}
	var req store.SAGRequest
	if err := json.Unmarshal(w.Body.Bytes(), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Priority != "high" {
		t.Errorf("derived priority for a course pickup = %q, want \"high\"", req.Priority)
	}
}
