package wxalert

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeSink records every OnAlerts/OnLinkStatus call for assertions.
type fakeSink struct {
	mu          sync.Mutex
	alertsCalls int
	lastAlerts  []Alert
	statuses    []LinkStatus
}

func (f *fakeSink) OnAlerts(alerts []Alert, fetchedAt time.Time, complete bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alertsCalls++
	f.lastAlerts = alerts
}

func (f *fakeSink) OnLinkStatus(st LinkStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, st)
}

func (f *fakeSink) lastStatus() LinkStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.statuses) == 0 {
		return LinkStatus{}
	}
	return f.statuses[len(f.statuses)-1]
}

func (f *fakeSink) alertsCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alertsCalls
}

func emptyCollection() []byte {
	return []byte(`{"type":"FeatureCollection","features":[]}`)
}

func countBody(total int, areas map[string]int) []byte {
	c := Count{Total: total, Areas: areas}
	data, _ := json.Marshal(c)
	return data
}

func TestPollConfigRequiresContact(t *testing.T) {
	if _, err := New(PollerConfig{UserAgent: ""}, &fakeSink{}); err == nil {
		t.Errorf("empty UserAgent: want error")
	}
	if _, err := New(PollerConfig{UserAgent: "Nymeria/1.0"}, &fakeSink{}); err == nil {
		t.Errorf("no-contact UserAgent: want error")
	}
	if _, err := New(PollerConfig{UserAgent: "Nymeria/1.0 (a@b.c)"}, &fakeSink{}); err != nil {
		t.Errorf("valid UserAgent: unexpected error %v", err)
	}
}

func TestPollSendsRequiredHeadersAndQuery(t *testing.T) {
	var gotQuery string
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/alerts/active/count" {
			w.Write(countBody(2, map[string]int{"MI": 2}))
			return
		}
		gotQuery = r.URL.RawQuery
		w.Write(emptyCollection())
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)"}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Date(2026, 9, 17, 19, 30, 0, 0, time.UTC)
	p.SetClock(func() time.Time { return now })
	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)

	if _, err := p.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if gotQuery == "" {
		t.Fatalf("no active-alerts request observed")
	}
	if !contains(gotQuery, "area=MI") {
		t.Errorf("query = %q, want area=MI", gotQuery)
	}
	if contains(gotQuery, "limit") {
		t.Errorf("query = %q, must never send limit", gotQuery)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

func TestPollEmptyFootprintSkipsRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write(emptyCollection())
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)"}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// No SetQuery call: Areas empty, Point nil.
	res, err := p.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if res.Skipped != "empty footprint" {
		t.Errorf("Skipped = %q, want 'empty footprint'", res.Skipped)
	}
	if called {
		t.Errorf("a request was made for an empty footprint")
	}
}

func TestPollNoChangeShortCircuit(t *testing.T) {
	var countCalls, activeCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alerts/active/count" {
			countCalls++
			w.Write(countBody(1, map[string]int{"MI": 1}))
			return
		}
		activeCalls++
		w.Write(emptyCollection())
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)", ForceFullEvery: time.Hour}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Date(2026, 9, 17, 19, 30, 0, 0, time.UTC)
	p.SetClock(func() time.Time { return now })
	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)

	res1, _ := p.PollOnce(context.Background())
	if !res1.Changed {
		t.Errorf("first poll Changed = false, want true (first full fetch)")
	}
	now = now.Add(60 * time.Second)
	res2, _ := p.PollOnce(context.Background())
	if res2.Changed {
		t.Errorf("second poll Changed = true, want false (unchanged signature)")
	}
	now = now.Add(60 * time.Second)
	res3, _ := p.PollOnce(context.Background())
	if res3.Changed {
		t.Errorf("third poll Changed = true, want false")
	}

	if activeCalls != 1 {
		t.Errorf("active fetches = %d, want 1 (only the first, full, poll)", activeCalls)
	}
	if countCalls != 3 {
		t.Errorf("count fetches = %d, want 3", countCalls)
	}
	if sink.alertsCallCount() != 1 {
		t.Errorf("OnAlerts calls = %d, want 1", sink.alertsCallCount())
	}
}

func TestPollDetectsChangesOnSignatureChange(t *testing.T) {
	total := 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alerts/active/count" {
			w.Write(countBody(total, map[string]int{"MI": total}))
			return
		}
		w.Write(emptyCollection())
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)", ForceFullEvery: time.Hour}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)

	if _, err := p.PollOnce(context.Background()); err != nil {
		t.Fatalf("poll1: %v", err)
	}
	total = 2 // signature changes
	res2, err := p.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("poll2: %v", err)
	}
	if sink.alertsCallCount() != 2 {
		t.Errorf("OnAlerts calls = %d, want 2 (signature change forces a second full fetch)", sink.alertsCallCount())
	}
	_ = res2
}

func TestPollRateLimit429Backoff(t *testing.T) {
	var attempt int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alerts/active/count" {
			w.Write(emptyCollection())
			return
		}
		attempt++
		if attempt <= 3 {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(countBody(1, map[string]int{"MI": 1}))
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)", MaxBackoff: 300 * time.Second}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Date(2026, 9, 17, 19, 30, 0, 0, time.UTC)
	p.SetClock(func() time.Time { return now })
	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)

	res, err := p.PollOnce(context.Background())
	if err == nil {
		t.Fatalf("expected an error on 429")
	}
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("err = %v, want *RateLimitError", err)
	}
	st := sink.lastStatus()
	if st.NextAttemptAt == nil || !st.NextAttemptAt.Equal(now.Add(5*time.Second)) {
		t.Errorf("NextAttemptAt = %v, want %v", st.NextAttemptAt, now.Add(5*time.Second))
	}

	// Polling before NextAttemptAt is a no-op (still within backoff).
	now = now.Add(3 * time.Second)
	res2, err := p.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce during backoff: %v", err)
	}
	if res2.Skipped != "backoff" {
		t.Errorf("Skipped = %q, want backoff", res2.Skipped)
	}

	// After the retry delay, it tries again (and still fails: attempt 2).
	now = now.Add(3 * time.Second) // now = +6s from first failure
	if _, err := p.PollOnce(context.Background()); err == nil {
		t.Fatalf("expected the second 429 to still error")
	}
	_ = res
}

func TestPollMalformedBodyIsNonDestructive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alerts/active/count" {
			w.Write(countBody(1, map[string]int{"MI": 1}))
			return
		}
		w.Write([]byte(`{"title":"Bad Request","status":400}`))
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)"}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)

	_, err = p.PollOnce(context.Background())
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
	st := sink.lastStatus()
	if st.LastSuccessAt != nil {
		t.Errorf("LastSuccessAt = %v, want nil (a bad body must not count as success)", st.LastSuccessAt)
	}
	if !contains(st.LastError, "Bad Request") {
		t.Errorf("LastError = %q, want it to mention Bad Request", st.LastError)
	}
	if sink.alertsCallCount() != 0 {
		t.Errorf("OnAlerts calls = %d, want 0 (a bad body never empties the list)", sink.alertsCallCount())
	}
}

func TestPollTimeoutDoesNotHang(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alerts/active/count" {
			<-block
			return
		}
		w.Write(emptyCollection())
	}))
	defer func() { close(block); srv.Close() }()

	sink := &fakeSink{}
	p, err := New(PollerConfig{
		BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)",
		Client: &http.Client{Timeout: 50 * time.Millisecond},
	}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)

	start := time.Now()
	_, err = p.PollOnce(context.Background())
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("PollOnce took too long: %v", time.Since(start))
	}
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestLinkStateTransitions(t *testing.T) {
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/alerts/active/count" {
			w.Write(countBody(1, map[string]int{"MI": 1}))
			return
		}
		w.Write(emptyCollection())
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)", ForceFullEvery: time.Hour}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t0 := time.Date(2026, 9, 17, 19, 30, 0, 0, time.UTC)
	now := t0
	p.SetClock(func() time.Time { return now })
	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)

	// step 1: OK -> live
	p.PollOnce(context.Background())
	if got := p.Status().State; got != LinkLive {
		t.Fatalf("step1 state = %v, want live", got)
	}

	// step 2: down for 3m01s -> stale
	up = false
	now = t0.Add(3*time.Minute + time.Second)
	p.PollNow()
	p.PollOnce(context.Background())
	if got := p.Status().State; got != LinkStale {
		t.Fatalf("step2 (3m01s) state = %v, want stale", got)
	}

	// step 3: 15m01s -> down
	now = t0.Add(15*time.Minute + time.Second)
	p.PollNow()
	p.PollOnce(context.Background())
	if got := p.Status().State; got != LinkDown {
		t.Fatalf("step3 (15m01s) state = %v, want down", got)
	}

	// step 4: comes back -> live
	up = true
	now = t0.Add(21 * time.Minute)
	p.PollNow()
	p.PollOnce(context.Background())
	if got := p.Status().State; got != LinkLive {
		t.Fatalf("step4 (restored) state = %v, want live", got)
	}
}

func TestPollerOffState(t *testing.T) {
	sink := &fakeSink{}
	p, err := New(PollerConfig{UserAgent: "Nymeria/test (ops@example.org)"}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.SetEnabled(false)
	res, err := p.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if res.Skipped != "off" {
		t.Errorf("Skipped = %q, want off", res.Skipped)
	}
	if got := p.Status().State; got != LinkOff {
		t.Errorf("State = %v, want off", got)
	}
}

func TestRunLoopRespectsContext(t *testing.T) {
	var calls int32
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alerts/active/count" {
			mu.Lock()
			calls++
			mu.Unlock()
			w.Write(countBody(1, map[string]int{"MI": 1}))
			return
		}
		w.Write(emptyCollection())
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)", Interval: 10 * time.Millisecond}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := calls
		mu.Unlock()
		if n >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for 3 poll calls, got %d", n)
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("Run did not return within 200ms of cancellation")
	}

	mu.Lock()
	after := calls
	mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	settled := calls
	mu.Unlock()
	if settled != after {
		t.Errorf("Run kept polling after cancellation: %d -> %d", after, settled)
	}
}

// An empty query means no watch area has resolved yet (cold start: zone
// lookups are bounded per refresh). The poller must NOT report that as a
// healthy, successful poll — doing so shows a green "updated just now" link
// over a pipe that is fetching nothing, which is the exact failure the
// provenance design exists to prevent.
func TestPollOnceEmptyQueryDoesNotReportHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alerts/active/count" {
			w.Write(countBody(1, map[string]int{"MI": 1}))
			return
		}
		w.Write(emptyCollection())
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)"}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.SetEnabled(true)
	p.SetQuery(ActiveQuery{}, false)

	res, err := p.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if res.Skipped == "" {
		t.Fatalf("expected a skip, got %+v", res)
	}

	st := p.Status()
	if st.LastSuccessAt != nil {
		t.Errorf("LastSuccessAt = %v, want nil — nothing was fetched", st.LastSuccessAt)
	}
	if st.State == LinkLive {
		t.Errorf("State = %q, want anything but live while monitoring nothing", st.State)
	}
	if st.Reason != ReasonNoWatchArea {
		t.Errorf("Reason = %q, want %q", st.Reason, ReasonNoWatchArea)
	}
	_ = sink
}

// Once a watch area does resolve, the poller reports normally again.
func TestPollOnceRecoversOnceQueryHasArea(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alerts/active/count" {
			w.Write(countBody(1, map[string]int{"MI": 1}))
			return
		}
		w.Write(emptyCollection())
	}))
	defer srv.Close()

	sink := &fakeSink{}
	p, err := New(PollerConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)"}, sink)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.SetEnabled(true)
	p.SetQuery(ActiveQuery{}, false)
	if _, err := p.PollOnce(context.Background()); err != nil {
		t.Fatalf("first PollOnce: %v", err)
	}

	p.SetQuery(ActiveQuery{Areas: []string{"MI"}}, false)
	if _, err := p.PollOnce(context.Background()); err != nil {
		t.Fatalf("second PollOnce: %v", err)
	}
	st := p.Status()
	if st.Reason == ReasonNoWatchArea {
		t.Errorf("Reason still %q after a real poll", st.Reason)
	}
	if st.LastSuccessAt == nil {
		t.Error("LastSuccessAt still nil after a successful poll")
	}
}
