package course

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/store"
)

// newTestManager builds a real-SQLite-backed store -> annotation.Manager ->
// checkpoint.Manager -> course.Manager stack, matching
// internal/checkpoint/checkpoint_test.go's newTestManager pattern.
func newTestManager(t *testing.T) (*Manager, *checkpoint.Manager, *annotation.Manager, store.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("store Init failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	annMgr := annotation.NewManager(s)
	cpMgr := checkpoint.NewManager(s, annMgr)
	courseMgr := NewManager(s, cpMgr, annMgr)
	annMgr.SetStatusGuard(courseMgr.GuardAnnotationStatus)
	cpMgr.SetOnPassage(courseMgr.OnPassage)
	return courseMgr, cpMgr, annMgr, s
}

// seedStations creates n aid annotations with CheckpointMeta seq 1..n for
// net "net-1", returning them in sequence order.
func seedStations(t *testing.T, cpMgr *checkpoint.Manager, annMgr *annotation.Manager, n int) []*store.Annotation {
	t.Helper()
	out := make([]*store.Annotation, 0, n)
	for i := 1; i <= n; i++ {
		ann, err := annMgr.Create(store.Annotation{
			Type:     "point",
			Label:    "Station",
			Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
			Category: "aid",
			Status:   "active",
			NetID:    "net-1",
		})
		if err != nil {
			t.Fatalf("create station %d: %v", i, err)
		}
		if _, err := cpMgr.SetMeta(store.CheckpointMeta{
			AnnotationID:   ann.ID,
			NetID:          "net-1",
			SequenceNumber: i,
		}); err != nil {
			t.Fatalf("SetMeta station %d: %v", i, err)
		}
		out = append(out, ann)
	}
	return out
}

func drain(ch <-chan Event) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// --- Shutoff transitions ---

func TestShutoffTransitions(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)

	newPlanned := func(m *Manager) *store.ShutoffPoint {
		sp, err := m.CreateShutoff(store.ShutoffPoint{
			NetID: "net-1", Name: "Test Shutoff", Lat: 41.0, Lon: -111.0,
			ScheduledAt: now,
		})
		if err != nil {
			t.Fatalf("CreateShutoff: %v", err)
		}
		return sp
	}

	t.Run("planned fire succeeds", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		fired, _, err := m.FireShutoff("net-1", sp.ID, FireInput{By: "NCS"})
		if err != nil {
			t.Fatalf("FireShutoff: %v", err)
		}
		if fired.Status != ShutoffFired {
			t.Errorf("status = %q, want fired", fired.Status)
		}
	})

	t.Run("planned cancel succeeds", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		cancelled, err := m.CancelShutoff("net-1", sp.ID, "NCS", "weather")
		if err != nil {
			t.Fatalf("CancelShutoff: %v", err)
		}
		if cancelled.Status != ShutoffCancelled {
			t.Errorf("status = %q, want cancelled", cancelled.Status)
		}
	})

	t.Run("cancelled reinstate succeeds", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		m.CancelShutoff("net-1", sp.ID, "NCS", "weather")
		reinstated, err := m.ReinstateShutoff("net-1", sp.ID, "NCS")
		if err != nil {
			t.Fatalf("ReinstateShutoff: %v", err)
		}
		if reinstated.Status != ShutoffPlanned {
			t.Errorf("status = %q, want planned", reinstated.Status)
		}
	})

	t.Run("fired reinstate without reason fails", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		m.FireShutoff("net-1", sp.ID, FireInput{By: "NCS"})
		if _, err := m.UnfireShutoff("net-1", sp.ID, "NCS", ""); err == nil {
			t.Error("expected error for blank reason")
		}
	})

	t.Run("fired reinstate with reason succeeds", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		m.FireShutoff("net-1", sp.ID, FireInput{By: "NCS"})
		reinstated, err := m.UnfireShutoff("net-1", sp.ID, "NCS", "entered in error")
		if err != nil {
			t.Fatalf("UnfireShutoff: %v", err)
		}
		if reinstated.Status != ShutoffPlanned {
			t.Errorf("status = %q, want planned", reinstated.Status)
		}
	})

	t.Run("cancelled fire fails", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		m.CancelShutoff("net-1", sp.ID, "NCS", "weather")
		if _, _, err := m.FireShutoff("net-1", sp.ID, FireInput{By: "NCS"}); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("FireShutoff(cancelled) = %v, want ErrIllegalTransition", err)
		}
	})

	t.Run("fired fire fails", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		m.FireShutoff("net-1", sp.ID, FireInput{By: "NCS"})
		if _, _, err := m.FireShutoff("net-1", sp.ID, FireInput{By: "NCS"}); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("FireShutoff(fired) = %v, want ErrIllegalTransition", err)
		}
	})

	t.Run("fired update fails", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		m.FireShutoff("net-1", sp.ID, FireInput{By: "NCS"})
		sp.Name = "Changed"
		if _, err := m.UpdateShutoff(*sp); err == nil {
			t.Error("expected error updating a fired shutoff")
		}
	})

	t.Run("planned delete succeeds", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		if err := m.DeleteShutoff("net-1", sp.ID); err != nil {
			t.Errorf("DeleteShutoff: %v", err)
		}
	})

	t.Run("fired delete fails", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		m.SetClock(fixedClock(now))
		sp := newPlanned(m)
		m.FireShutoff("net-1", sp.ID, FireInput{By: "NCS"})
		if err := m.DeleteShutoff("net-1", sp.ID); err == nil {
			t.Error("expected error deleting a fired shutoff")
		}
	})
}

func TestCreateShutoffValidation(t *testing.T) {
	now := time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		sp      store.ShutoffPoint
		wantErr bool
	}{
		{"missing name", store.ShutoffPoint{NetID: "net-1", ScheduledAt: now, Lat: 41, Lon: -111}, true},
		{"zero scheduledAt", store.ShutoffPoint{NetID: "net-1", Name: "X", Lat: 41, Lon: -111}, true},
		{"lat out of range", store.ShutoffPoint{NetID: "net-1", Name: "X", ScheduledAt: now, Lat: 91, Lon: -111}, true},
		{"lon out of range", store.ShutoffPoint{NetID: "net-1", Name: "X", ScheduledAt: now, Lat: 41, Lon: -181}, true},
		{
			"benson example", store.ShutoffPoint{
				NetID: "net-1", Name: "Benson Shutoff", Lat: 41.79474, Lon: -111.90586,
				ScheduledAt: now, RerouteDirection: "West", RerouteDestination: "55-mile route",
			}, false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _, s := newTestManager(t)
			if tc.name == "staffed by check-in from other net" {
				_ = s
			}
			sp, err := m.CreateShutoff(tc.sp)
			if tc.wantErr && err == nil {
				t.Errorf("CreateShutoff(%s) = nil error, want error", tc.name)
			}
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("CreateShutoff(%s): %v", tc.name, err)
				}
				if sp.Status != ShutoffPlanned {
					t.Errorf("status = %q, want planned", sp.Status)
				}
			}
		})
	}

	t.Run("staffed by check-in from other net", func(t *testing.T) {
		m, _, _, s := newTestManager(t)
		otherNow := time.Now().UTC()
		if err := s.SaveNetCheckIn(store.NetCheckIn{
			ID: "ci-other-net", NetID: "net-2", Callsign: "K6ABC",
			CheckedInAt: otherNow, LastHeard: otherNow,
			MissionIDs: []string{}, TrackedStations: []store.TrackedStation{},
		}); err != nil {
			t.Fatalf("SaveNetCheckIn: %v", err)
		}
		_, err := m.CreateShutoff(store.ShutoffPoint{
			NetID: "net-1", Name: "X", ScheduledAt: now, Lat: 41, Lon: -111,
			StaffedByCheckInID: "ci-other-net",
		})
		if err == nil {
			t.Error("expected error for a check-in belonging to a different net")
		}
	})
}

func TestFireShutoffCreatesExceptionsAndTimeline(t *testing.T) {
	now := time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC)
	m, _, _, s := newTestManager(t)
	m.SetClock(fixedClock(now))

	sp, err := m.CreateShutoff(store.ShutoffPoint{
		NetID: "net-1", Name: "Benson Shutoff", Lat: 41.79474, Lon: -111.90586,
		ScheduledAt: now, RerouteDirection: "West", RerouteDestination: "55-mile route",
	})
	if err != nil {
		t.Fatalf("CreateShutoff: %v", err)
	}

	events := m.Events()
	drain(events)

	fired, riders, err := m.FireShutoff("net-1", sp.ID, FireInput{
		By: "NCS", Bibs: []string{"101", "102"}, RerouteCount: 12,
	})
	if err != nil {
		t.Fatalf("FireShutoff: %v", err)
	}
	if len(riders) != 2 {
		t.Fatalf("riders len = %d, want 2", len(riders))
	}
	for _, r := range riders {
		if r.Kind != KindShutoffReroute {
			t.Errorf("rider kind = %q, want shutoff_reroute", r.Kind)
		}
		if r.SupportStatus != SupportUnsupported {
			t.Errorf("rider supportStatus = %q, want unsupported", r.SupportStatus)
		}
		if r.ShutoffID != sp.ID {
			t.Errorf("rider shutoffId = %q, want %q", r.ShutoffID, sp.ID)
		}
	}
	if fired.RerouteCount != 12 {
		t.Errorf("RerouteCount = %d, want 12", fired.RerouteCount)
	}

	netEvents, err := s.LoadNetEvents("net-1")
	if err != nil {
		t.Fatalf("LoadNetEvents: %v", err)
	}
	var found *store.NetEvent
	for i := range netEvents {
		if netEvents[i].Type == TimelineShutoffFired {
			found = &netEvents[i]
		}
	}
	if found == nil {
		t.Fatal("no shutoff_fired timeline event found")
	}
	var details struct {
		Bibs         []string `json:"bibs"`
		RerouteCount int      `json:"rerouteCount"`
	}
	if err := json.Unmarshal([]byte(found.Details), &details); err != nil {
		t.Fatalf("unmarshal timeline details: %v", err)
	}
	if len(details.Bibs) != 2 || details.RerouteCount != 12 {
		t.Errorf("timeline details = %+v, want bibs=[101,102] rerouteCount=12", details)
	}

	// Events channel yields course_shutoff_fired then (eventually) course_rider_created x2, then course_state.
	var sawFired, sawState bool
	for i := 0; i < 6; i++ {
		select {
		case evt := <-events:
			switch evt.Type {
			case EventShutoffFired:
				sawFired = true
			case EventCourseState:
				sawState = true
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for events")
		}
		if sawFired && sawState {
			break
		}
	}
	if !sawFired {
		t.Error("did not see course_shutoff_fired event")
	}
	if !sawState {
		t.Error("did not see course_state event")
	}

	state, err := m.State("net-1")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.RerouteCountTotal != 14 {
		t.Errorf("RerouteCountTotal = %d, want 14", state.RerouteCountTotal)
	}
}

func TestFireShutoffWithheldBibs(t *testing.T) {
	now := time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC)
	m, _, _, _ := newTestManager(t)
	m.SetClock(fixedClock(now))

	sp, err := m.CreateShutoff(store.ShutoffPoint{
		NetID: "net-1", Name: "X", ScheduledAt: now, Lat: 41, Lon: -111,
	})
	if err != nil {
		t.Fatalf("CreateShutoff: %v", err)
	}

	_, riders, err := m.FireShutoff("net-1", sp.ID, FireInput{
		By: "NCS", Bibs: nil, BibWithheld: true, RerouteCount: 3,
	})
	if err != nil {
		t.Fatalf("FireShutoff: %v", err)
	}
	if len(riders) != 0 {
		t.Errorf("riders len = %d, want 0", len(riders))
	}

	state, err := m.State("net-1")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.RerouteCountTotal != 3 {
		t.Errorf("RerouteCountTotal = %d, want 3", state.RerouteCountTotal)
	}
}

// --- Riders ---

func TestRiderSupportTransitions(t *testing.T) {
	cases := []struct {
		name        string
		op          func(m *Manager) (*store.RiderException, error)
		wantErr     bool
		wantSupport string
	}{
		{
			"sag create", func(m *Manager) (*store.RiderException, error) {
				return m.RecordRider(store.RiderException{NetID: "net-1", Bib: "1", Kind: KindSAG})
			}, false, SupportSupported,
		},
		{
			"dnf create", func(m *Manager) (*store.RiderException, error) {
				return m.RecordRider(store.RiderException{NetID: "net-1", Kind: KindDNF})
			}, false, SupportUnsupported,
		},
		{
			"declined_sag create", func(m *Manager) (*store.RiderException, error) {
				return m.RecordRider(store.RiderException{NetID: "net-1", Bib: "2", Kind: KindDeclinedSAG})
			}, false, SupportUnsupported,
		},
		{
			"invalid kind", func(m *Manager) (*store.RiderException, error) {
				return m.RecordRider(store.RiderException{NetID: "net-1", Bib: "3", Kind: "bogus"})
			}, true, "",
		},
		{
			"bib blank not withheld kind sag", func(m *Manager) (*store.RiderException, error) {
				return m.RecordRider(store.RiderException{NetID: "net-1", Kind: KindSAG})
			}, true, "",
		},
		{
			"bib blank withheld kind sag", func(m *Manager) (*store.RiderException, error) {
				return m.RecordRider(store.RiderException{NetID: "net-1", Kind: KindSAG, BibWithheld: true})
			}, false, SupportSupported,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _, _ := newTestManager(t)
			r, err := tc.op(m)
			if tc.wantErr {
				if err == nil {
					t.Errorf("%s: expected error", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if r.SupportStatus != tc.wantSupport {
				t.Errorf("%s: supportStatus = %q, want %q", tc.name, r.SupportStatus, tc.wantSupport)
			}
		})
	}

	t.Run("supported to unsupported no reason ok", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		r, _ := m.RecordRider(store.RiderException{NetID: "net-1", Bib: "1", Kind: KindSAG})
		updated, err := m.SetRiderSupport("net-1", r.ID, SupportUnsupported, "NCS", "")
		if err != nil {
			t.Fatalf("SetRiderSupport: %v", err)
		}
		if updated.SupportStatus != SupportUnsupported {
			t.Errorf("supportStatus = %q, want unsupported", updated.SupportStatus)
		}
	})

	t.Run("unsupported to supported no reason fails", func(t *testing.T) {
		m, _, _, _ := newTestManager(t)
		r, _ := m.RecordRider(store.RiderException{NetID: "net-1", Kind: KindDNF})
		if _, err := m.SetRiderSupport("net-1", r.ID, SupportSupported, "NCS", ""); err == nil {
			t.Error("expected error for blank reason")
		}
	})

	t.Run("unsupported to supported with reason ok", func(t *testing.T) {
		m, _, _, s := newTestManager(t)
		r, _ := m.RecordRider(store.RiderException{NetID: "net-1", Kind: KindDNF})
		updated, err := m.SetRiderSupport("net-1", r.ID, SupportSupported, "NCS", "entered in error")
		if err != nil {
			t.Fatalf("SetRiderSupport: %v", err)
		}
		if updated.SupportStatus != SupportSupported {
			t.Errorf("supportStatus = %q, want supported", updated.SupportStatus)
		}
		netEvents, _ := s.LoadNetEvents("net-1")
		var found bool
		for _, e := range netEvents {
			if e.Type == TimelineRiderResupported {
				found = true
			}
		}
		if !found {
			t.Error("expected a rider_resupported timeline entry")
		}
	})
}

func TestMarkUnsupportedFromSAG(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	r, err := m.MarkUnsupportedFromSAG("net-1", "77", "sag-req-1", "NCS", "declined transport")
	if err != nil {
		t.Fatalf("MarkUnsupportedFromSAG: %v", err)
	}
	if r.Kind != KindDeclinedSAG || r.SupportStatus != SupportUnsupported || r.SAGRequestID != "sag-req-1" {
		t.Errorf("rider = %+v, want kind=declined_sag unsupported sagRequestId=sag-req-1", r)
	}
}

func TestStateCountsExceptions(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	for i := 0; i < 3; i++ {
		if _, err := m.RecordRider(store.RiderException{NetID: "net-1", Bib: "s", Kind: KindSAG}); err != nil {
			t.Fatalf("RecordRider supported: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := m.RecordRider(store.RiderException{NetID: "net-1", Kind: KindDNF}); err != nil {
			t.Fatalf("RecordRider unsupported: %v", err)
		}
	}
	state, err := m.State("net-1")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.SupportedExceptions != 3 || state.UnsupportedExceptions != 2 {
		t.Errorf("counts = supported %d unsupported %d, want 3/2", state.SupportedExceptions, state.UnsupportedExceptions)
	}
}

// --- Sweep ---

func TestReportSweepValidation(t *testing.T) {
	cases := []struct {
		name    string
		r       store.SweepReport
		wantErr bool
	}{
		{"missing reportedBy", store.SweepReport{NetID: "net-1"}, true},
		{"speed too low", store.SweepReport{NetID: "net-1", ReportedBy: "sweep", EstimatedSpeedMph: floatPtr(-1)}, true},
		{"speed too high", store.SweepReport{NetID: "net-1", ReportedBy: "sweep", EstimatedSpeedMph: floatPtr(61)}, true},
		{"nil speed ok", store.SweepReport{NetID: "net-1", ReportedBy: "sweep"}, false},
		{"valid", store.SweepReport{NetID: "net-1", ReportedBy: "sweep", EstimatedSpeedMph: floatPtr(12)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _, s := newTestManager(t)
			_, err := m.ReportSweep(tc.r)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ReportSweep: %v", err)
			}
			netEvents, _ := s.LoadNetEvents("net-1")
			var found bool
			for _, e := range netEvents {
				if e.Type == TimelineSweepReport {
					found = true
				}
			}
			if !found {
				t.Error("expected a sweep_report timeline entry")
			}
		})
	}
}

func floatPtr(f float64) *float64 { return &f }

func TestSweepPositionFromCheckpointProgress(t *testing.T) {
	m, cpMgr, annMgr, _ := newTestManager(t)
	stations := seedStations(t, cpMgr, annMgr, 4)

	if _, err := cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[1].ID, NetID: "net-1", Label: "SWEEP", Direction: "through",
	}); err != nil {
		t.Fatalf("LogPassage SWEEP: %v", err)
	}
	if _, err := cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[3].ID, NetID: "net-1", Label: "LEAD", Direction: "through",
	}); err != nil {
		t.Fatalf("LogPassage LEAD: %v", err)
	}

	pos := m.SweepPosition("net-1")
	if pos.LastCheckpointSeq != 2 {
		t.Errorf("LastCheckpointSeq = %d, want 2", pos.LastCheckpointSeq)
	}
	if pos.NextStationLabel != "Station" || pos.NextStationID != stations[2].ID {
		t.Errorf("next station = %q/%q, want station 3", pos.NextStationID, pos.NextStationLabel)
	}

	// Custom SweepLabel "TAIL" — only "TAIL" passages count now.
	if _, err := m.SetConfig(store.CourseConfig{NetID: "net-1", SweepLabel: "TAIL"}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	pos2 := m.SweepPosition("net-1")
	if pos2.LastCheckpointSeq != 0 {
		t.Errorf("LastCheckpointSeq after relabel = %d, want 0 (no TAIL passages yet)", pos2.LastCheckpointSeq)
	}
	if _, err := cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: stations[2].ID, NetID: "net-1", Label: "TAIL", Direction: "through",
	}); err != nil {
		t.Fatalf("LogPassage TAIL: %v", err)
	}
	pos3 := m.SweepPosition("net-1")
	if pos3.LastCheckpointSeq != 3 {
		t.Errorf("LastCheckpointSeq after TAIL passage = %d, want 3", pos3.LastCheckpointSeq)
	}
}

// TestSweepEta is skipped: ETA-to-next needs a per-checkpoint route-mile,
// which CheckpointMeta does not carry today (open question 3 in the WP3
// design was never resolved by the architect pass, whose per-package
// directives came back empty for this package). SweepPosition.EtaToNextMinutes
// is always nil until that field exists.
func TestSweepEta(t *testing.T) {
	t.Skip("no per-checkpoint route-mile field exists yet to compute ETA from (design open question 3, unresolved)")
}

// --- Station ladder ---

func setupStation(t *testing.T) (*Manager, *checkpoint.Manager, *annotation.Manager, string) {
	t.Helper()
	m, cpMgr, annMgr, _ := newTestManager(t)
	stations := seedStations(t, cpMgr, annMgr, 1)
	return m, cpMgr, annMgr, stations[0].ID
}

func TestStationLadder(t *testing.T) {
	t.Run("riders-clear from open", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		c, err := m.ReportRidersClear("net-1", cpID, "NCS")
		if err != nil {
			t.Fatalf("ReportRidersClear: %v", err)
		}
		if c.State != StationRidersClear {
			t.Errorf("state = %q, want riders_clear", c.State)
		}
	})

	t.Run("sweep-passed from open sets ridersClearAt", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		c, err := m.MarkSweepPassed("net-1", cpID, "sweep", "passage-1")
		if err != nil {
			t.Fatalf("MarkSweepPassed: %v", err)
		}
		if c.State != StationSweepPassed {
			t.Errorf("state = %q, want sweep_passed", c.State)
		}
		if c.RidersClearAt == nil {
			t.Error("RidersClearAt should be set implicitly")
		}
	})

	t.Run("riders-clear, sweep-passed, close -> closed", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		m.ReportRidersClear("net-1", cpID, "NCS")
		m.MarkSweepPassed("net-1", cpID, "sweep", "p1")
		c, err := m.CloseStation("net-1", cpID, CloseInput{By: "NCS"})
		if err != nil {
			t.Fatalf("CloseStation: %v", err)
		}
		if c.State != StationClosed {
			t.Errorf("state = %q, want closed", c.State)
		}
	})

	t.Run("riders-clear twice is illegal", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		m.ReportRidersClear("net-1", cpID, "NCS")
		if _, err := m.ReportRidersClear("net-1", cpID, "NCS"); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("second ReportRidersClear = %v, want ErrIllegalTransition", err)
		}
	})

	t.Run("sweep-passed twice is idempotent", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		m.MarkSweepPassed("net-1", cpID, "sweep", "p1")
		c, err := m.MarkSweepPassed("net-1", cpID, "sweep", "p2")
		if err != nil {
			t.Errorf("second MarkSweepPassed = %v, want nil (idempotent)", err)
		}
		if c.State != StationSweepPassed {
			t.Errorf("state = %q, want sweep_passed", c.State)
		}
	})

	t.Run("close without sweep passed fails", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		if _, err := m.CloseStation("net-1", cpID, CloseInput{By: "NCS"}); !errors.Is(err, ErrSweepNotPassed) {
			t.Errorf("CloseStation = %v, want ErrSweepNotPassed", err)
		}
	})

	t.Run("riders-clear then close fails", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		m.ReportRidersClear("net-1", cpID, "NCS")
		if _, err := m.CloseStation("net-1", cpID, CloseInput{By: "NCS"}); !errors.Is(err, ErrSweepNotPassed) {
			t.Errorf("CloseStation = %v, want ErrSweepNotPassed", err)
		}
	})

	t.Run("close override no reason ncs fails", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		_, err := m.CloseStation("net-1", cpID, CloseInput{By: "NCS", Override: true, ActorIsNCSOrAdmin: true})
		if err == nil {
			t.Error("expected error for blank override reason")
		}
	})

	t.Run("close override reason non-ncs fails", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		_, err := m.CloseStation("net-1", cpID, CloseInput{By: "OP", Override: true, OverrideReason: "no sweep coming", ActorIsNCSOrAdmin: false})
		if !errors.Is(err, ErrOverrideNotAllowed) {
			t.Errorf("CloseStation = %v, want ErrOverrideNotAllowed", err)
		}
	})

	t.Run("close override reason ncs succeeds", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		c, err := m.CloseStation("net-1", cpID, CloseInput{By: "NCS", Override: true, OverrideReason: "storm coming", ActorIsNCSOrAdmin: true})
		if err != nil {
			t.Fatalf("CloseStation: %v", err)
		}
		if c.State != StationClosed || !c.ClosedByOverride {
			t.Errorf("closure = %+v, want closed with override", c)
		}
	})

	t.Run("sweep-passed close reopen keeps sweep passed", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		m.MarkSweepPassed("net-1", cpID, "sweep", "p1")
		m.CloseStation("net-1", cpID, CloseInput{By: "NCS"})
		c, err := m.ReopenStation("net-1", cpID, "NCS", "rider arrived")
		if err != nil {
			t.Fatalf("ReopenStation: %v", err)
		}
		if c.State != StationOpen {
			t.Errorf("state = %q, want open", c.State)
		}
		if c.ReopenCount != 1 {
			t.Errorf("ReopenCount = %d, want 1", c.ReopenCount)
		}
		if c.SweepPassedAt == nil {
			t.Error("SweepPassedAt should still be set after reopen")
		}
		if c.ClosedAt != nil {
			t.Error("ClosedAt should be nil after reopen")
		}
	})

	t.Run("reopen blank reason fails", func(t *testing.T) {
		m, _, _, cpID := setupStation(t)
		if _, err := m.ReopenStation("net-1", cpID, "NCS", ""); err == nil {
			t.Error("expected error for blank reopen reason")
		}
	})
}

func TestCloseRequiresSweepFalse(t *testing.T) {
	m, _, _, cpID := setupStation(t)
	if _, err := m.SetConfig(store.CourseConfig{NetID: "net-1", CloseRequiresSweep: false}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	c, err := m.CloseStation("net-1", cpID, CloseInput{By: "NCS"})
	if err != nil {
		t.Fatalf("CloseStation: %v", err)
	}
	if c.State != StationClosed {
		t.Errorf("state = %q, want closed", c.State)
	}
	if c.ClosedByOverride {
		t.Error("ClosedByOverride should be false when the policy didn't require sweep")
	}
}

func TestCloseSyncsAnnotationAndMeta(t *testing.T) {
	m, cpMgr, annMgr, cpID := setupStation(t)
	m.MarkSweepPassed("net-1", cpID, "sweep", "p1")
	if _, err := m.CloseStation("net-1", cpID, CloseInput{By: "NCS"}); err != nil {
		t.Fatalf("CloseStation: %v", err)
	}
	ann, ok := annMgr.Get(cpID)
	if !ok || ann.Status != "closed" {
		t.Errorf("annotation status = %v, want closed", ann)
	}
	meta, ok := cpMgr.MetaForAnnotation(cpID)
	if !ok || meta.ClosedAt == nil {
		t.Errorf("meta = %+v, want ClosedAt set", meta)
	}

	if _, err := m.ReopenStation("net-1", cpID, "NCS", "reopening"); err != nil {
		t.Fatalf("ReopenStation: %v", err)
	}
	ann2, ok := annMgr.Get(cpID)
	if !ok || ann2.Status != "active" {
		t.Errorf("annotation status after reopen = %v, want active", ann2)
	}
}

func TestAutoSweepPassedFromPassage(t *testing.T) {
	t.Run("default config auto-advances", func(t *testing.T) {
		m, cpMgr, annMgr, _ := newTestManager(t)
		stations := seedStations(t, cpMgr, annMgr, 3)
		p, err := cpMgr.LogPassage(store.CheckpointPassage{
			CheckpointID: stations[1].ID, NetID: "net-1", Label: "SWEEP", Direction: "through",
		})
		if err != nil {
			t.Fatalf("LogPassage: %v", err)
		}
		state, err := m.State("net-1")
		if err != nil {
			t.Fatalf("State: %v", err)
		}
		var st2 *StationView
		for i := range state.Stations {
			if state.Stations[i].SequenceNumber == 2 {
				st2 = &state.Stations[i]
			}
		}
		if st2 == nil || st2.Closure.State != StationSweepPassed {
			t.Fatalf("station 2 = %+v, want sweep_passed", st2)
		}
		if st2.Closure.SweepPassageID != p.ID {
			t.Errorf("SweepPassageID = %q, want %q", st2.Closure.SweepPassageID, p.ID)
		}

		netEvents, _ := cpMgr.GetCheckpointsForNet("net-1")
		_ = netEvents
	})

	t.Run("disabled config does not auto-advance", func(t *testing.T) {
		m, cpMgr, annMgr, _ := newTestManager(t)
		stations := seedStations(t, cpMgr, annMgr, 3)
		if _, err := m.SetConfig(store.CourseConfig{NetID: "net-1", AutoSweepFromPassage: false}); err != nil {
			t.Fatalf("SetConfig: %v", err)
		}
		if _, err := cpMgr.LogPassage(store.CheckpointPassage{
			CheckpointID: stations[1].ID, NetID: "net-1", Label: "SWEEP", Direction: "through",
		}); err != nil {
			t.Fatalf("LogPassage: %v", err)
		}
		closure := m.getClosure("net-1", stations[1].ID)
		if closure.State != StationOpen {
			t.Errorf("state = %q, want open (auto-sweep disabled)", closure.State)
		}
	})

	t.Run("lowercase label still matches", func(t *testing.T) {
		m, cpMgr, annMgr, _ := newTestManager(t)
		stations := seedStations(t, cpMgr, annMgr, 2)
		if _, err := cpMgr.LogPassage(store.CheckpointPassage{
			CheckpointID: stations[0].ID, NetID: "net-1", Label: "sweep", Direction: "through",
		}); err != nil {
			t.Fatalf("LogPassage: %v", err)
		}
		closure := m.getClosure("net-1", stations[0].ID)
		if closure.State != StationSweepPassed {
			t.Errorf("state = %q, want sweep_passed (case-insensitive match)", closure.State)
		}
	})
}

func TestAnnotationStatusGuard(t *testing.T) {
	m, cpMgr, annMgr, _ := newTestManager(t)
	stations := seedStations(t, cpMgr, annMgr, 1)
	cpID := stations[0].ID

	if _, err := annMgr.ChangeStatus(cpID, "closed"); !errors.Is(err, ErrSweepNotPassed) {
		t.Errorf("ChangeStatus before sweep = %v, want ErrSweepNotPassed", err)
	}
	ann, _ := annMgr.Get(cpID)
	if ann.Status == "closed" {
		t.Error("annotation should be unchanged after a blocked ChangeStatus")
	}

	if _, err := m.MarkSweepPassed("net-1", cpID, "sweep", "p1"); err != nil {
		t.Fatalf("MarkSweepPassed: %v", err)
	}
	if _, err := annMgr.ChangeStatus(cpID, "closed"); err != nil {
		t.Errorf("ChangeStatus after sweep = %v, want nil", err)
	}

	// A non-sequenced "aid" annotation (no CheckpointMeta) is not gated.
	plainAid, err := annMgr.Create(store.Annotation{
		Type: "point", Label: "Plain Aid", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: "aid", Status: "active", NetID: "net-1",
	})
	if err != nil {
		t.Fatalf("create plain aid: %v", err)
	}
	if _, err := annMgr.ChangeStatus(plainAid.ID, "closed"); err != nil {
		t.Errorf("ChangeStatus on non-sequenced annotation = %v, want nil", err)
	}

	// A net with CloseRequiresSweep=false is not gated.
	stations2 := seedStations(t, cpMgr, annMgr, 1)
	_ = stations2 // still net-1; use a distinct net for the false-policy case below.

	otherAnn, err := annMgr.Create(store.Annotation{
		Type: "point", Label: "Other Net Station", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: "aid", Status: "active", NetID: "net-2",
	})
	if err != nil {
		t.Fatalf("create other net station: %v", err)
	}
	if _, err := cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: otherAnn.ID, NetID: "net-2", SequenceNumber: 1}); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if _, err := m.SetConfig(store.CourseConfig{NetID: "net-2", CloseRequiresSweep: false}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if _, err := annMgr.ChangeStatus(otherAnn.ID, "closed"); err != nil {
		t.Errorf("ChangeStatus with CloseRequiresSweep=false = %v, want nil", err)
	}
}

// --- Accumulator ---

func TestClearThroughAccumulator(t *testing.T) {
	cases := []struct {
		name           string
		states         map[int]string // seq -> state
		wantClear      int
		wantOutOfOrder []int
		wantAllClosed  bool
	}{
		{"all open", map[int]string{1: StationOpen, 2: StationOpen, 3: StationOpen, 4: StationOpen}, 0, nil, false},
		{"1 sweep_passed", map[int]string{1: StationSweepPassed, 2: StationOpen, 3: StationOpen, 4: StationOpen}, 1, nil, false},
		{"1 closed 2 sweep_passed", map[int]string{1: StationClosed, 2: StationSweepPassed, 3: StationOpen, 4: StationOpen}, 2, nil, false},
		{"1 closed 2 open 3 closed", map[int]string{1: StationClosed, 2: StationOpen, 3: StationClosed, 4: StationOpen}, 1, []int{3}, false},
		{"1 riders_clear 2 closed", map[int]string{1: StationRidersClear, 2: StationClosed, 3: StationOpen, 4: StationOpen}, 0, []int{2}, false},
		{"all closed", map[int]string{1: StationClosed, 2: StationClosed, 3: StationClosed, 4: StationClosed}, 4, nil, true},
		{"1-3 closed 4 riders_clear", map[int]string{1: StationClosed, 2: StationClosed, 3: StationClosed, 4: StationRidersClear}, 3, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, cpMgr, annMgr, _ := newTestManager(t)
			stations := seedStations(t, cpMgr, annMgr, 4)

			for seq, state := range tc.states {
				cpID := stations[seq-1].ID
				switch state {
				case StationOpen:
					// default; nothing to do.
				case StationRidersClear:
					if _, err := m.ReportRidersClear("net-1", cpID, "NCS"); err != nil {
						t.Fatalf("ReportRidersClear seq %d: %v", seq, err)
					}
				case StationSweepPassed:
					if _, err := m.MarkSweepPassed("net-1", cpID, "sweep", "p"); err != nil {
						t.Fatalf("MarkSweepPassed seq %d: %v", seq, err)
					}
				case StationClosed:
					if _, err := m.MarkSweepPassed("net-1", cpID, "sweep", "p"); err != nil {
						t.Fatalf("MarkSweepPassed (pre-close) seq %d: %v", seq, err)
					}
					if _, err := m.CloseStation("net-1", cpID, CloseInput{By: "NCS"}); err != nil {
						t.Fatalf("CloseStation seq %d: %v", seq, err)
					}
				}
			}

			state, err := m.State("net-1")
			if err != nil {
				t.Fatalf("State: %v", err)
			}
			if state.ClearThroughSeq != tc.wantClear {
				t.Errorf("ClearThroughSeq = %d, want %d", state.ClearThroughSeq, tc.wantClear)
			}
			if state.AllStationsClosed != tc.wantAllClosed {
				t.Errorf("AllStationsClosed = %v, want %v", state.AllStationsClosed, tc.wantAllClosed)
			}
			var gotOutOfOrder []int
			for _, sv := range state.Stations {
				if sv.OutOfOrder {
					gotOutOfOrder = append(gotOutOfOrder, sv.SequenceNumber)
				}
			}
			if len(gotOutOfOrder) != len(tc.wantOutOfOrder) {
				t.Errorf("outOfOrder = %v, want %v", gotOutOfOrder, tc.wantOutOfOrder)
				return
			}
			for i := range gotOutOfOrder {
				if gotOutOfOrder[i] != tc.wantOutOfOrder[i] {
					t.Errorf("outOfOrder = %v, want %v", gotOutOfOrder, tc.wantOutOfOrder)
					break
				}
			}
		})
	}
}

func TestStateNeverNilSlices(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	state, err := m.State("net-empty")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !contains(data, `"stations":[]`) {
		t.Errorf("expected stations:[] in %s", data)
	}
	if !contains(data, `"shutoffs":[]`) {
		t.Errorf("expected shutoffs:[] in %s", data)
	}

	if err := m.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	state2, err := m.State("net-empty")
	if err != nil {
		t.Fatalf("State after Load: %v", err)
	}
	data2, err := json.Marshal(state2)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !contains(data2, `"stations":[]`) || !contains(data2, `"shutoffs":[]`) {
		t.Errorf("expected empty arrays after Load in %s", data2)
	}
}

func contains(haystack []byte, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if string(haystack[i:i+len(needle)]) == needle {
				return true
			}
		}
		return false
	})()
}

func TestLoadRehydrates(t *testing.T) {
	m, cpMgr, annMgr, s := newTestManager(t)
	stations := seedStations(t, cpMgr, annMgr, 2)

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	m.SetClock(fixedClock(now))

	sp, err := m.CreateShutoff(store.ShutoffPoint{NetID: "net-1", Name: "X", ScheduledAt: now, Lat: 1, Lon: 1})
	if err != nil {
		t.Fatalf("CreateShutoff: %v", err)
	}
	if _, err := m.RecordRider(store.RiderException{NetID: "net-1", Bib: "5", Kind: KindSAG}); err != nil {
		t.Fatalf("RecordRider: %v", err)
	}
	if _, err := m.ReportSweep(store.SweepReport{NetID: "net-1", ReportedBy: "sweep", LastRiderBib: "5"}); err != nil {
		t.Fatalf("ReportSweep: %v", err)
	}
	if _, err := m.MarkSweepPassed("net-1", stations[0].ID, "sweep", "p1"); err != nil {
		t.Fatalf("MarkSweepPassed: %v", err)
	}
	if _, err := m.CloseStation("net-1", stations[0].ID, CloseInput{By: "NCS"}); err != nil {
		t.Fatalf("CloseStation: %v", err)
	}
	_ = sp

	before, err := m.State("net-1")
	if err != nil {
		t.Fatalf("State (before): %v", err)
	}
	beforeJSON, _ := json.Marshal(before)

	// A fresh Manager over the same store + a fresh annotation/checkpoint
	// stack (so nothing is shared in-process) must reload identically.
	annMgr2 := annotation.NewManager(s)
	if err := annMgr2.Load(); err != nil {
		t.Fatalf("annMgr2.Load: %v", err)
	}
	cpMgr2 := checkpoint.NewManager(s, annMgr2)
	if err := cpMgr2.Load(); err != nil {
		t.Fatalf("cpMgr2.Load: %v", err)
	}
	m2 := NewManager(s, cpMgr2, annMgr2)
	m2.SetClock(fixedClock(now))
	if err := m2.Load(); err != nil {
		t.Fatalf("m2.Load: %v", err)
	}

	after, err := m2.State("net-1")
	if err != nil {
		t.Fatalf("State (after): %v", err)
	}
	afterJSON, _ := json.Marshal(after)

	if string(beforeJSON) != string(afterJSON) {
		t.Errorf("state mismatch after reload:\nbefore: %s\nafter:  %s", beforeJSON, afterJSON)
	}
}

func TestDeletedAnnotationDroppedFromView(t *testing.T) {
	m, cpMgr, annMgr, _ := newTestManager(t)
	stations := seedStations(t, cpMgr, annMgr, 3)

	if _, err := m.MarkSweepPassed("net-1", stations[1].ID, "sweep", "p1"); err != nil {
		t.Fatalf("MarkSweepPassed: %v", err)
	}
	if _, err := m.CloseStation("net-1", stations[1].ID, CloseInput{By: "NCS"}); err != nil {
		t.Fatalf("CloseStation: %v", err)
	}

	if err := annMgr.Delete(stations[1].ID); err != nil {
		t.Fatalf("Delete annotation: %v", err)
	}

	state, err := m.State("net-1")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Stations) != 2 {
		t.Errorf("Stations len = %d, want 2 (deleted annotation dropped)", len(state.Stations))
	}
}

// Numbering a stop builds the course, but that is a CHECKPOINT mutation, so
// nothing in this package re-emitted the derived course state: the rail and
// ride strip kept reporting no course until the operator hard-reloaded.
// RefreshState is what the checkpoint bridge calls to close that gap.
func TestRefreshState_EmitsCourseState(t *testing.T) {
	m, cpMgr, annMgr, _ := newTestManager(t)
	seedStations(t, cpMgr, annMgr, 3)
	drainCourseEvents(m)

	m.RefreshState("net-1")

	select {
	case evt := <-m.Events():
		if evt.Type != EventCourseState {
			t.Fatalf("event type = %q, want %q", evt.Type, EventCourseState)
		}
		state, ok := evt.Data.(*CourseState)
		if !ok {
			t.Fatalf("event data = %T, want CourseState", evt.Data)
		}
		// The point of the refresh: the freshly numbered stops are present,
		// which is exactly what rideHasCourse checks on the client.
		if len(state.Stations) != 3 {
			t.Errorf("stations = %d, want 3", len(state.Stations))
		}
	default:
		t.Fatal("RefreshState emitted no course_state event")
	}

	// An empty net id is a no-op, never a panic or a stray broadcast.
	drainCourseEvents(m)
	m.RefreshState("")
	select {
	case evt := <-m.Events():
		t.Fatalf("RefreshState(\"\") emitted %q, want nothing", evt.Type)
	default:
	}
}

func drainCourseEvents(m *Manager) {
	for {
		select {
		case <-m.Events():
		default:
			return
		}
	}
}
