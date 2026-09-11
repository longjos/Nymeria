package server

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/gps"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/server/ws"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/transport"
)

// fakeGPSSource is a hand-driven gps.Source for server-level tests.
type fakeGPSSource struct {
	ch chan gps.Fix
}

func newFakeGPSSource() *fakeGPSSource {
	return &fakeGPSSource{ch: make(chan gps.Fix, 64)}
}

func (f *fakeGPSSource) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		close(f.ch)
	}()
	return nil
}
func (f *fakeGPSSource) Fixes() <-chan gps.Fix { return f.ch }
func (f *fakeGPSSource) Status() gps.SourceStatus {
	return gps.SourceStatus{Type: "gpsd", Target: "127.0.0.1:2947", Connected: true}
}
func (f *fakeGPSSource) Close() error { return nil }

// newTestGPSServer builds a server with a session manager (so RequireRole
// has something to authenticate against) and, optionally, a GPS manager.
func newTestGPSServer(t *testing.T, mgr *gps.Manager) (*Server, *session.MemoryManager) {
	t.Helper()
	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{PIN: "1234"})
	tracker := station.NewMemoryTracker(config.StationConfig{})
	tm := transport.NewManager()

	opts := []Option{WithSessionManager(sessMgr)}
	if mgr != nil {
		opts = append(opts, WithGPSManager(mgr))
	}
	srv := New(tracker, tm, message.Engine(nil), nil, opts...)
	return srv, sessMgr
}

func TestGetGPSDisabledShape(t *testing.T) {
	srv, sessMgr := newTestGPSServer(t, nil)
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/gps", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]any{
		"enabled":   false,
		"connected": false,
		"fix":       nil,
		"ageMillis": nil,
		"stale":     true,
	}
	if len(got) != len(want) {
		t.Errorf("got %d keys, want exactly %d: %#v", len(got), len(want), got)
	}
	for k, v := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		if !reflect.DeepEqual(gv, v) {
			t.Errorf("%s = %#v, want %#v", k, gv, v)
		}
	}
}

func TestGetGPSWithFix(t *testing.T) {
	src := newFakeGPSSource()
	mgr := gps.NewManagerWithSource(gps.Config{
		Enabled: true, Type: "gpsd", Host: "127.0.0.1", Port: 2947,
		StaleAfter: 30 * time.Second,
	}, src)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	src.ch <- gps.Fix{
		Mode: gps.Mode3D, Lat: 44.0, Lon: -121.0,
		SpeedKnots: 1.2, Course: 90, HasCourse: true,
		ReceivedAt: time.Now(),
	}

	deadline := time.Now().Add(time.Second)
	for {
		if _, ok := mgr.Current(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fix never accepted by manager")
		}
		time.Sleep(5 * time.Millisecond)
	}

	srv, sessMgr := newTestGPSServer(t, mgr)
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/gps", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["enabled"] != true {
		t.Errorf("enabled = %v, want true", got["enabled"])
	}

	fix, ok := got["fix"].(map[string]any)
	if !ok {
		t.Fatalf("fix missing or wrong shape: %v", got["fix"])
	}
	if fix["mode"] != float64(3) {
		t.Errorf("fix.mode = %v, want 3", fix["mode"])
	}
	for _, key := range []string{"hasAltitude", "speedKnots", "hasCourse", "receivedAt"} {
		if _, ok := fix[key]; !ok {
			t.Errorf("fix missing camelCase key %q", key)
		}
	}
	ms, ok := got["ageMillis"].(float64)
	if !ok || ms < 0 {
		t.Errorf("ageMillis = %v, want a number >= 0", got["ageMillis"])
	}
}

func TestGetGPSRequiresObserver(t *testing.T) {
	srv, _ := newTestGPSServer(t, nil)

	w := doRequest(srv, "GET", "/api/gps", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestGPSSignatureChange(t *testing.T) {
	base := gps.Status{
		Enabled: true, Connected: true, Stale: false,
		Fix: &gps.Fix{Mode: gps.Mode3D, Lat: 44.0, Lon: -121.0, SpeedKnots: 1.0, Course: 90},
	}
	withFix := func(mutate func(*gps.Fix)) gps.Status {
		f := *base.Fix
		mutate(&f)
		s := base
		s.Fix = &f
		return s
	}

	tests := []struct {
		name  string
		other gps.Status
		equal bool
	}{
		{"identical", base, true},
		{"lat differs by 1e-7 (sub-cm, ignorable)", withFix(func(f *gps.Fix) { f.Lat += 1e-7 }), true},
		{"lat differs by 1e-5 (~1m, meaningful)", withFix(func(f *gps.Fix) { f.Lat += 1e-5 }), false},
		{"course +1 degree", withFix(func(f *gps.Fix) { f.Course += 1 }), false},
		{"connected flips", func() gps.Status { s := base; s.Connected = false; return s }(), false},
		{"only ageMillis differs", func() gps.Status {
			s := base
			ms := int64(5000)
			s.AgeMillis = &ms
			return s
		}(), true},
	}

	a := signatureOf(base)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := signatureOf(tt.other)
			if (a == b) != tt.equal {
				t.Errorf("signatures equal = %v, want %v (a=%+v b=%+v)", a == b, tt.equal, a, b)
			}
		})
	}
}

func TestBridgeGPSRateLimit(t *testing.T) {
	src := newFakeGPSSource()
	mgr := gps.NewManagerWithSource(gps.Config{Enabled: true, Type: "gpsd", MinInterval: time.Millisecond}, src)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	s := &Server{hub: ws.NewHub(), gpsMgr: mgr}
	go s.hub.Run()

	client := &ws.Client{ID: "rate-limit", Send: make(chan []byte, 64)}
	s.hub.Register(client)
	defer s.hub.Unregister(client)

	go s.bridgeGPS()
	// Give the goroutine a chance to reach Subscribe() before fixes start
	// flowing — Subscribe only sees fixes that arrive after it registers.
	time.Sleep(50 * time.Millisecond)

	base := time.Now()
	const n = 20
	for i := 0; i < n; i++ {
		src.ch <- gps.Fix{
			Mode: gps.Mode3D, Lat: 44.0 + float64(i)*0.001, Lon: -121.0,
			ReceivedAt: base.Add(time.Duration(i) * 50 * time.Millisecond),
		}
	}

	var messages [][]byte
	deadline := time.After(1500 * time.Millisecond)
collect:
	for {
		select {
		case msg := <-client.Send:
			messages = append(messages, msg)
		case <-deadline:
			break collect
		}
	}

	if len(messages) == 0 || len(messages) > 2 {
		t.Fatalf("got %d broadcasts, want 1-2", len(messages))
	}

	var last struct {
		GPS struct {
			Fix struct {
				Lat float64 `json:"lat"`
			} `json:"fix"`
		} `json:"gps"`
	}
	if err := json.Unmarshal(messages[len(messages)-1], &last); err != nil {
		t.Fatalf("unmarshal last message: %v", err)
	}
	wantLat := 44.0 + float64(n-1)*0.001
	if diff := last.GPS.Fix.Lat - wantLat; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("last broadcast lat = %v, want %v (the newest fix)", last.GPS.Fix.Lat, wantLat)
	}
}

func TestBridgeGPSThrottlesUnchanged(t *testing.T) {
	src := newFakeGPSSource()
	mgr := gps.NewManagerWithSource(gps.Config{Enabled: true, Type: "gpsd", MinInterval: time.Millisecond}, src)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	s := &Server{hub: ws.NewHub(), gpsMgr: mgr}
	go s.hub.Run()

	client := &ws.Client{ID: "throttle", Send: make(chan []byte, 64)}
	s.hub.Register(client)
	defer s.hub.Unregister(client)

	go s.bridgeGPS()
	// Give the goroutine a chance to reach Subscribe() before fixes start
	// flowing — Subscribe only sees fixes that arrive after it registers.
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 10; i++ {
		src.ch <- gps.Fix{Mode: gps.Mode3D, Lat: 44.0, Lon: -121.0, ReceivedAt: time.Now()}
		time.Sleep(120 * time.Millisecond)
	}

	var count int
collect:
	for {
		select {
		case <-client.Send:
			count++
		case <-time.After(300 * time.Millisecond):
			break collect
		}
	}

	if count != 1 {
		t.Errorf("broadcasts = %d, want 1", count)
	}
}

func TestOwnPositionFrameShape(t *testing.T) {
	src := newFakeGPSSource()
	mgr := gps.NewManagerWithSource(gps.Config{Enabled: true, Type: "gpsd", MinInterval: time.Millisecond}, src)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	s := &Server{hub: ws.NewHub(), gpsMgr: mgr}
	go s.hub.Run()

	client := &ws.Client{ID: "shape", Send: make(chan []byte, 64)}
	s.hub.Register(client)
	defer s.hub.Unregister(client)

	go s.bridgeGPS()
	// Give the goroutine a chance to reach Subscribe() before fixes start
	// flowing — Subscribe only sees fixes that arrive after it registers.
	time.Sleep(50 * time.Millisecond)

	src.ch <- gps.Fix{Mode: gps.Mode3D, Lat: 44.0, Lon: -121.0, ReceivedAt: time.Now()}

	var msg []byte
	select {
	case msg = <-client.Send:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for own_position broadcast")
	}

	var envelope struct {
		Type string          `json:"type"`
		GPS  json.RawMessage `json:"gps"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Type != "own_position" {
		t.Errorf("type = %q, want own_position", envelope.Type)
	}

	var gotMap map[string]any
	if err := json.Unmarshal(envelope.GPS, &gotMap); err != nil {
		t.Fatalf("unmarshal gps: %v", err)
	}

	wantJSON, err := json.Marshal(s.gpsStatus())
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	var wantMap map[string]any
	json.Unmarshal(wantJSON, &wantMap)

	// ageMillis is a live, time.Since()-derived value: it legitimately
	// differs between the broadcast instant and this later GET, so it's
	// excluded from the equality check rather than the frame's shape.
	delete(gotMap, "ageMillis")
	delete(wantMap, "ageMillis")

	if !reflect.DeepEqual(gotMap, wantMap) {
		t.Errorf("gps body = %#v, want (GET /api/gps body) %#v", gotMap, wantMap)
	}
}
