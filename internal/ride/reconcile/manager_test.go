package reconcile

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/course"
	"github.com/narvel/nymeria/internal/netcontrol"
	"github.com/narvel/nymeria/internal/ride"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/store"
)

// testStack bundles every collaborator a reconcile.Manager test might need,
// all backed by one real (temp-file) SQLite store — the same style
// course_test.go and checkpoint_test.go use.
type testStack struct {
	Store     store.Store
	Ann       *annotation.Manager
	CP        *checkpoint.Manager
	Course    *course.Manager
	Net       *netcontrol.Manager
	SAG       *ride.Manager
	Reconcile *Manager
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

	sagMgr := ride.NewManager(s, netMgr, annMgr, ride.DefaultConfig())

	recMgr := NewManager(s, netMgr, courseMgr, sagMgr, annMgr)

	return &testStack{Store: s, Ann: annMgr, CP: cpMgr, Course: courseMgr, Net: netMgr, SAG: sagMgr, Reconcile: recMgr}
}

// newOpenNet creates and opens a net, returning its id.
func (ts *testStack) newOpenNet(t *testing.T, name string) string {
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

// newSAGCheckIn checks a sag-category unit into a net and gives it a
// tactical call, returning the check-in id.
func (ts *testStack) newSAGCheckIn(t *testing.T, netID, callsign, tactical string) string {
	t.Helper()
	ci, err := ts.Net.CheckIn(netID, callsign, netcontrol.TrafficNone, netcontrol.CatSAG)
	if err != nil {
		t.Fatalf("CheckIn: %v", err)
	}
	ci.TacticalCall = tactical
	if _, err := ts.Net.UpdateCheckIn(*ci); err != nil {
		t.Fatalf("UpdateCheckIn: %v", err)
	}
	return ci.ID
}

// seedStations creates n "aid" annotations with CheckpointMeta sequence
// 1..n on netID, mirroring internal/course/course_test.go's helper.
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

// seedCheckpointStations creates n "checkpoint" category annotations (as
// distinct from "aid" rest stops) with CheckpointMeta sequence 1..n, so
// sweep-completion tests don't also create a rest stop that then blocks the
// separate rest_stops_closed checklist item.
func (ts *testStack) seedCheckpointStations(t *testing.T, netID string, n int) []*store.Annotation {
	t.Helper()
	out := make([]*store.Annotation, 0, n)
	for i := 1; i <= n; i++ {
		ann, err := ts.Ann.Create(store.Annotation{
			Type: "point", Label: "Checkpoint", Category: "checkpoint", Status: "active",
			Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
			NetID:    netID,
		})
		if err != nil {
			t.Fatalf("create checkpoint %d: %v", i, err)
		}
		if _, err := ts.CP.SetMeta(store.CheckpointMeta{AnnotationID: ann.ID, NetID: netID, SequenceNumber: i}); err != nil {
			t.Fatalf("SetMeta checkpoint %d: %v", i, err)
		}
		out = append(out, ann)
	}
	return out
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
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

func drainTypes(ch <-chan Event) []string {
	var out []string
	for {
		select {
		case evt := <-ch:
			out = append(out, evt.Type)
		default:
			return out
		}
	}
}
