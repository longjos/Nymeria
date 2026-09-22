package ride

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/store"
)

// --- Test doubles shared by supply_test.go and medical_test.go ---

// fakeNets is a minimal NetLookup with one open net and one draft net.
type fakeNets struct {
	nets map[string]store.Net
}

func newFakeNets() *fakeNets {
	return &fakeNets{nets: map[string]store.Net{
		"open-net":  {ID: "open-net", Status: "open", Profile: "bike-ride"},
		"draft-net": {ID: "draft-net", Status: "draft", Profile: "bike-ride"},
	}}
}

func (f *fakeNets) GetNet(id string) (*store.Net, bool) {
	n, ok := f.nets[id]
	if !ok {
		return nil, false
	}
	cp := n
	return &cp, true
}

// timelineEntry captures one recordingTimeline call. At is the wall-clock
// instant the fake recorded the call (not the manager's injected clock,
// which is deliberately held fixed across a whole test step) — it exists
// only so tests can assert that separate ladder steps produced separate
// timeline rows, the way real NetEvent.CreatedAt values would.
type timelineEntry struct {
	NetID, EventType, Callsign, Summary, Details string
	At                                           time.Time
}

// recordingTimeline is a TimelineSink that just remembers every call, so
// tests can assert on timeline step counts/content without a real
// netcontrol.Manager.
type recordingTimeline struct {
	mu      sync.Mutex
	entries []timelineEntry
	err     error // when set, AddTimelineEventWithDetails returns it without recording
}

func (r *recordingTimeline) AddTimelineEventWithDetails(netID, eventType, callsign, summary, details string) error {
	if r.err != nil {
		return r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, timelineEntry{netID, eventType, callsign, summary, details, time.Now()})
	return nil
}

func (r *recordingTimeline) all() []timelineEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]timelineEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

func (r *recordingTimeline) countType(eventType string) int {
	n := 0
	for _, e := range r.all() {
		if e.EventType == eventType {
			n++
		}
	}
	return n
}

// fixedPolicy is a Policy test double. A nil Tiers map accepts exactly the
// five Marin ARS tiers.
type fixedPolicy struct {
	withhold bool
	tiers    map[string]bool
}

func (p *fixedPolicy) WithholdSevereBib(string) bool { return p.withhold }

func (p *fixedPolicy) ValidTier(_ string, tier string) bool {
	if p.tiers == nil {
		switch tier {
		case PriorityEmergency, PriorityPriority, PriorityHigh, PriorityMedium, PriorityLow:
			return true
		default:
			return false
		}
	}
	return p.tiers[tier]
}

// newTestTrafficManager builds a TrafficManager backed by a real SQLite
// store (so persistence/Load() round-trips are exercised, per the
// codebase's convention of a real store over a fake one) plus fake
// NetLookup/TimelineSink/Policy so ladder tests can inject a clock.
func newTestTrafficManager(t *testing.T) (m *TrafficManager, nets *fakeNets, tl *recordingTimeline, pol *fixedPolicy, db *store.SQLiteStore) {
	t.Helper()
	db = store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	nets = newFakeNets()
	tl = &recordingTimeline{}
	pol = &fixedPolicy{}
	m = NewTrafficManager(db, nets, tl, nil, pol)
	return m, nets, tl, pol, db
}

// newTestTrafficManagerWithActivity is newTestTrafficManager plus a real
// activity.StoreLogger, for tests that assert on logged activity Details.
func newTestTrafficManagerWithActivity(t *testing.T) (m *TrafficManager, nets *fakeNets, tl *recordingTimeline, act *activity.StoreLogger, db *store.SQLiteStore) {
	t.Helper()
	db = store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err := db.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	nets = newFakeNets()
	tl = &recordingTimeline{}
	act = activity.NewStoreLogger(db)
	m = NewTrafficManager(db, nets, tl, act, &fixedPolicy{})
	return m, nets, tl, act, db
}

// setClock installs an injectable clock on m, starting at start, and
// returns a function that advances it by d.
func setClock(m *TrafficManager, start time.Time) (advance func(d time.Duration)) {
	clock := start
	m.now = func() time.Time { return clock }
	return func(d time.Duration) { clock = clock.Add(d) }
}
