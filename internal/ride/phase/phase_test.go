package phase

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/netprofile"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
)

// --- LegalTransition: the state machine, tested independently of any I/O ---

func TestLegalTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		// Forward, one step at a time: legal.
		{PreStart, Launched, true},
		{Launched, MidRide, true},
		{MidRide, Closing, true},
		{Closing, Collapse, true},
		{Collapse, Reconcile, true},

		// Forward, skipping a step: illegal — each step is a real
		// operational milestone the strip's zone set depends on.
		{PreStart, MidRide, false},
		{PreStart, Closing, false},
		{PreStart, Collapse, false},
		{PreStart, Reconcile, false},
		{Launched, Closing, false},
		{Launched, Collapse, false},
		{Launched, Reconcile, false},
		{MidRide, Collapse, false},
		{MidRide, Reconcile, false},
		{Closing, Reconcile, false},

		// Backward, any distance: legal. An NCS who advanced too early
		// must be able to correct it, all the way back to pre-start.
		{Launched, PreStart, true},
		{MidRide, PreStart, true},
		{MidRide, Launched, true},
		{Closing, PreStart, true},
		{Closing, Launched, true},
		{Closing, MidRide, true},
		{Collapse, PreStart, true},
		{Collapse, Closing, true},
		{Reconcile, PreStart, true},
		{Reconcile, Collapse, true},

		// Same phase: illegal — a no-op with nothing to log.
		{PreStart, PreStart, false},
		{Reconcile, Reconcile, false},

		// Unknown phases: illegal.
		{PreStart, "warp-speed", false},
		{"warp-speed", PreStart, false},
		{"", "", false},
	}
	for _, tt := range tests {
		if got := LegalTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("LegalTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestValid(t *testing.T) {
	for _, p := range Order {
		if !Valid(p) {
			t.Errorf("Valid(%q) = false, want true", p)
		}
	}
	if Valid("") || Valid("bogus") {
		t.Error("Valid should reject unknown phases")
	}
}

// --- Manager test harness ---

type testStack struct {
	Store  store.Store
	Ann    *annotation.Manager
	CP     *checkpoint.Manager
	Course *course.Manager
	Net    *netcontrol.Manager
	Phase  *Manager
}

func newTestStack(t *testing.T) *testStack {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("store Init failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	tracker := station.NewMemoryTracker(config.StationConfig{
		StaleTimeout:   time.Hour,
		TrackMaxPoints: 10,
		DedupWindow:    30 * time.Second,
	})
	netMgr := netcontrol.NewManager(s, tracker)

	annMgr := annotation.NewManager(s)
	cpMgr := checkpoint.NewManager(s, annMgr)
	courseMgr := course.NewManager(s, cpMgr, annMgr)
	annMgr.SetStatusGuard(courseMgr.GuardAnnotationStatus)
	cpMgr.SetOnPassage(courseMgr.OnPassage)

	phaseMgr := NewManager(s, netMgr, courseMgr, cpMgr)

	return &testStack{Store: s, Ann: annMgr, CP: cpMgr, Course: courseMgr, Net: netMgr, Phase: phaseMgr}
}

// newBikeRideNet creates a draft net, switches it to the bike-ride profile
// (must happen while still a draft), then opens it.
func (ts *testStack) newBikeRideNet(t *testing.T, name string) string {
	t.Helper()
	n, err := ts.Net.CreateNet(store.Net{Name: name})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if _, err := ts.Net.SetProfile(n.ID, netprofile.ProfileBikeRide); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	if err := ts.Net.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	return n.ID
}

func (ts *testStack) newGeneralNet(t *testing.T, name string) string {
	t.Helper()
	n, err := ts.Net.CreateNet(store.Net{Name: name})
	if err != nil {
		t.Fatalf("CreateNet: %v", err)
	}
	if err := ts.Net.OpenNet(n.ID); err != nil {
		t.Fatalf("OpenNet: %v", err)
	}
	return n.ID
}

// seedStations creates n "aid" annotations with CheckpointMeta sequence
// 1..n on netID (mirrors internal/course/course_test.go's helper).
func (ts *testStack) seedStations(t *testing.T, netID string, n int) []*store.Annotation {
	t.Helper()
	out := make([]*store.Annotation, 0, n)
	for i := 1; i <= n; i++ {
		ann, err := ts.Ann.Create(store.Annotation{
			Type: "point", Label: "Station", Category: "aid", Status: "active",
			Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
			NetID:    netID,
		})
		if err != nil {
			t.Fatalf("create station %d: %v", i, err)
		}
		if _, err := ts.CP.SetMeta(store.CheckpointMeta{AnnotationID: ann.ID, NetID: netID, SequenceNumber: i}); err != nil {
			t.Fatalf("SetMeta station %d: %v", i, err)
		}
		out = append(out, ann)
	}
	return out
}

// --- GetState: profile gating and defaults ---

func TestGetStateUnknownNet(t *testing.T) {
	ts := newTestStack(t)
	if _, err := ts.Phase.GetState("no-such-net"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetState(unknown) err = %v, want ErrNotFound", err)
	}
}

func TestGetStateGeneralNetIsUnaffected(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newGeneralNet(t, "General Net")
	if _, err := ts.Phase.GetState(netID); !errors.Is(err, ErrProfileMismatch) {
		t.Errorf("GetState(general net) err = %v, want ErrProfileMismatch", err)
	}
	if _, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"}); !errors.Is(err, ErrProfileMismatch) {
		t.Errorf("SetPhase(general net) err = %v, want ErrProfileMismatch", err)
	}
}

func TestGetStateDefaultsToPreStart(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	st, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Phase != PreStart {
		t.Errorf("Phase = %q, want %q", st.Phase, PreStart)
	}
	if st.Suggestion.Phase != "" {
		t.Errorf("Suggestion = %+v, want empty (no lead passage yet)", st.Suggestion)
	}
}

// --- SetPhase: legality, reason requirement, persistence, logging ---

func TestSetPhaseForwardStep(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	st, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"})
	if err != nil {
		t.Fatalf("SetPhase: %v", err)
	}
	if st.Phase != Launched {
		t.Errorf("Phase = %q, want %q", st.Phase, Launched)
	}

	// Persisted: a fresh GetState (and a fresh manager reading from the same
	// store) both see it.
	got, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got.Phase != Launched {
		t.Errorf("GetState after SetPhase = %q, want %q", got.Phase, Launched)
	}

	events, err := ts.Store.LoadNetEvents(netID)
	if err != nil {
		t.Fatalf("LoadNetEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == TimelinePhaseChanged {
			found = true
			if e.Callsign != "K6ABC" {
				t.Errorf("event callsign = %q, want K6ABC", e.Callsign)
			}
		}
	}
	if !found {
		t.Errorf("no %s NetEvent logged; events = %+v", TimelinePhaseChanged, events)
	}
}

func TestSetPhaseSkipForwardIsIllegal(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: MidRide, By: "K6ABC"}); !errors.Is(err, ErrIllegalTransition) {
		t.Errorf("SetPhase(skip forward) err = %v, want ErrIllegalTransition", err)
	}
}

func TestSetPhaseSamePhaseIsIllegal(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: PreStart, By: "K6ABC"}); !errors.Is(err, ErrIllegalTransition) {
		t.Errorf("SetPhase(same phase) err = %v, want ErrIllegalTransition", err)
	}
}

func TestSetPhaseInvalidTarget(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: "warp-speed", By: "K6ABC"}); err == nil {
		t.Error("SetPhase(invalid target) = nil error, want one")
	}
}

func TestSetPhaseBackwardRequiresReason(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"}); err != nil {
		t.Fatalf("advance to launched: %v", err)
	}
	if _, err := ts.Phase.SetPhase(netID, SetInput{To: MidRide, By: "K6ABC"}); err != nil {
		t.Fatalf("advance to mid-ride: %v", err)
	}

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"}); err == nil {
		t.Error("SetPhase(backward, no reason) = nil error, want one")
	}

	st, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC", Reason: "advanced too early"})
	if err != nil {
		t.Fatalf("SetPhase(backward, with reason): %v", err)
	}
	if st.Phase != Launched {
		t.Errorf("Phase = %q, want %q", st.Phase, Launched)
	}
	if st.Reason != "advanced too early" {
		t.Errorf("Reason = %q, want %q", st.Reason, "advanced too early")
	}
}

func TestSetPhaseBackwardAllTheWayToPreStart(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	for _, p := range []string{Launched, MidRide, Closing, Collapse, Reconcile} {
		if _, err := ts.Phase.SetPhase(netID, SetInput{To: p, By: "K6ABC"}); err != nil {
			t.Fatalf("advance to %s: %v", p, err)
		}
	}

	st, err := ts.Phase.SetPhase(netID, SetInput{To: PreStart, By: "K6ABC", Reason: "false start, re-running"})
	if err != nil {
		t.Fatalf("SetPhase(reconcile -> pre-start): %v", err)
	}
	if st.Phase != PreStart {
		t.Errorf("Phase = %q, want %q", st.Phase, PreStart)
	}
}

func TestSetPhaseOnUnknownOrGeneralNet(t *testing.T) {
	ts := newTestStack(t)

	if _, err := ts.Phase.SetPhase("no-such-net", SetInput{To: Launched, By: "K6ABC"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetPhase(unknown net) err = %v, want ErrNotFound", err)
	}

	generalID := ts.newGeneralNet(t, "General Net")
	if _, err := ts.Phase.SetPhase(generalID, SetInput{To: Launched, By: "K6ABC"}); !errors.Is(err, ErrProfileMismatch) {
		t.Errorf("SetPhase(general net) err = %v, want ErrProfileMismatch", err)
	}
}

// --- Load: hydrates from the store ---

func TestLoadHydratesFromStore(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"}); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}

	// A fresh manager over the same store, before Load, defaults to
	// pre-start; after Load it must see the persisted phase.
	fresh := NewManager(ts.Store, ts.Net, ts.Course, ts.CP)
	if st, err := fresh.GetState(netID); err != nil || st.Phase != PreStart {
		t.Fatalf("fresh manager before Load: phase=%v err=%v, want pre-start", st, err)
	}
	if err := fresh.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	st, err := fresh.GetState(netID)
	if err != nil {
		t.Fatalf("GetState after Load: %v", err)
	}
	if st.Phase != Launched {
		t.Errorf("Phase after Load = %q, want %q", st.Phase, Launched)
	}
}

// --- Suggest: trigger conditions ---

func TestSuggestLaunchedOnFirstLeadPassage(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")
	stations := ts.seedStations(t, netID, 3)

	st, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != "" {
		t.Errorf("Suggestion before any passage = %+v, want empty", st.Suggestion)
	}

	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[0].ID, NetID: netID, Label: "LEAD", ReportedBy: "K6ABC",
	}); err != nil {
		t.Fatalf("LogPassage LEAD: %v", err)
	}

	st, err = ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != Launched {
		t.Errorf("Suggestion = %+v, want phase %q", st.Suggestion, Launched)
	}
	if st.Suggestion.Reason == "" {
		t.Error("Suggestion.Reason is empty, want an explanation")
	}
}

func TestSuggestOnlyProposesTheNextStep(t *testing.T) {
	// Even if a later trigger (e.g. sweep passing stop 1) were somehow also
	// true, Suggest never proposes skipping past the immediate next phase.
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")
	stations := ts.seedStations(t, netID, 2)

	// Sweep passes stop 1 directly (bypassing a lead passage entirely).
	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[0].ID, NetID: netID, Label: "SWEEP", ReportedBy: "SAG 1",
	}); err != nil {
		t.Fatalf("LogPassage SWEEP: %v", err)
	}

	st, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	// Still pre-start: only pre-start's own trigger (lead passage) is
	// evaluated while the phase is pre-start.
	if st.Suggestion.Phase != "" {
		t.Errorf("Suggestion = %+v, want empty (current phase is pre-start, not launched)", st.Suggestion)
	}
}

func TestSuggestMidRideOnSweepPastStop1(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")
	stations := ts.seedStations(t, netID, 2)

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"}); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}

	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[0].ID, NetID: netID, Label: "SWEEP", ReportedBy: "SAG 1",
	}); err != nil {
		t.Fatalf("LogPassage SWEEP: %v", err)
	}

	st, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != MidRide {
		t.Errorf("Suggestion = %+v, want phase %q", st.Suggestion, MidRide)
	}
}

func TestSuggestSkipsStartCategoryForStop1(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	start, err := ts.Ann.Create(store.Annotation{
		Type: "point", Label: "Start Line", Category: "start", Status: "active",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
		NetID:    netID,
	})
	if err != nil {
		t.Fatalf("create start: %v", err)
	}
	if _, err := ts.CP.SetMeta(store.CheckpointMeta{AnnotationID: start.ID, NetID: netID, SequenceNumber: 1}); err != nil {
		t.Fatalf("SetMeta start: %v", err)
	}
	stations := ts.seedStations(t, netID, 1) // sequence 2 (RS1, "stop 1")
	rs1 := stations[0]

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"}); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}

	// Sweep passing the START line must NOT trigger the suggestion — only
	// passing the first real stop (RS1) should.
	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: start.ID, NetID: netID, Label: "SWEEP", ReportedBy: "SAG 1",
	}); err != nil {
		t.Fatalf("LogPassage SWEEP at start: %v", err)
	}
	st, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != "" {
		t.Errorf("Suggestion after sweep passes only the start line = %+v, want empty", st.Suggestion)
	}

	if _, err := ts.CP.LogPassage(store.CheckpointPassage{
		CheckpointID: rs1.ID, NetID: netID, Label: "SWEEP", ReportedBy: "SAG 1",
	}); err != nil {
		t.Fatalf("LogPassage SWEEP at RS1: %v", err)
	}
	st, err = ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != MidRide {
		t.Errorf("Suggestion after sweep passes RS1 = %+v, want phase %q", st.Suggestion, MidRide)
	}
}

func TestSuggestClosingAtT60ToFirstShutoff(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	ts.Course.SetClock(clock)
	ts.Phase.SetClock(clock)

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"}); err != nil {
		t.Fatalf("advance to launched: %v", err)
	}
	if _, err := ts.Phase.SetPhase(netID, SetInput{To: MidRide, By: "K6ABC"}); err != nil {
		t.Fatalf("advance to mid-ride: %v", err)
	}

	if _, err := ts.Course.CreateShutoff(store.ShutoffPoint{
		NetID: netID, Name: "Benson", Lat: 34.05, Lon: -118.24,
		ScheduledAt: now.Add(90 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateShutoff: %v", err)
	}

	st, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != "" {
		t.Errorf("Suggestion at T-90 = %+v, want empty", st.Suggestion)
	}

	// Move the clock to T-45 (inside the T-60 window).
	now = now.Add(45 * time.Minute)
	st, err = ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != Closing {
		t.Errorf("Suggestion at T-45 = %+v, want phase %q", st.Suggestion, Closing)
	}
}

func TestSuggestNeverProposesCollapse(t *testing.T) {
	// "NCS declares, usually after the lead finishes" has no machine
	// trigger — the spec is explicit that this is a judgment call.
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	now := time.Now().UTC()
	ts.Course.SetClock(func() time.Time { return now })
	ts.Phase.SetClock(func() time.Time { return now })

	for _, p := range []string{Launched, MidRide, Closing} {
		if _, err := ts.Phase.SetPhase(netID, SetInput{To: p, By: "K6ABC"}); err != nil {
			t.Fatalf("advance to %s: %v", p, err)
		}
	}

	if _, err := ts.Course.CreateShutoff(store.ShutoffPoint{
		NetID: netID, Name: "Benson", Lat: 34.05, Lon: -118.24,
		ScheduledAt: now.Add(-time.Minute), // already past due -> deep inside any T-60 window
	}); err != nil {
		t.Fatalf("CreateShutoff: %v", err)
	}

	st, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != "" {
		t.Errorf("Suggestion while closing = %+v, want empty (collapse is never suggested)", st.Suggestion)
	}
}

func TestSuggestReconcileOnLastStopClear(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")
	stations := ts.seedStations(t, netID, 2)

	for _, p := range []string{Launched, MidRide, Closing, Collapse} {
		if _, err := ts.Phase.SetPhase(netID, SetInput{To: p, By: "K6ABC"}); err != nil {
			t.Fatalf("advance to %s: %v", p, err)
		}
	}

	for _, st := range stations {
		if _, err := ts.Course.MarkSweepPassed(netID, st.ID, "SAG 1", ""); err != nil {
			t.Fatalf("MarkSweepPassed %s: %v", st.ID, err)
		}
		if _, err := ts.Course.CloseStation(netID, st.ID, course.CloseInput{By: "K6ABC"}); err != nil {
			t.Fatalf("CloseStation %s: %v", st.ID, err)
		}
	}

	got, err := ts.Phase.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got.Suggestion.Phase != Reconcile {
		t.Errorf("Suggestion = %+v, want phase %q", got.Suggestion, Reconcile)
	}
}

func TestSuggestGracefulWithoutCollaborators(t *testing.T) {
	// A phase.Manager constructed without course/checkpoint collaborators
	// (as if course closure were never wired up) must never panic, and
	// simply never suggests anything.
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	bare := NewManager(ts.Store, ts.Net, nil, nil)
	st, err := bare.GetState(netID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.Suggestion.Phase != "" {
		t.Errorf("Suggestion with no collaborators = %+v, want empty", st.Suggestion)
	}
}

// --- Events ---

func TestSetPhaseEmitsEvent(t *testing.T) {
	ts := newTestStack(t)
	netID := ts.newBikeRideNet(t, "Ride Net")

	if _, err := ts.Phase.SetPhase(netID, SetInput{To: Launched, By: "K6ABC"}); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}

	select {
	case evt := <-ts.Phase.Events():
		if evt.Type != EventPhaseUpdated {
			t.Errorf("event type = %q, want %q", evt.Type, EventPhaseUpdated)
		}
	default:
		t.Error("no event emitted on SetPhase")
	}
}
