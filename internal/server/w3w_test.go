package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/geocode/w3w"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/transport"
)

// newTestW3WServer builds a test server wired with a w3w.Client pointed at
// a mock what3words upstream, plus session + config managers so the
// settings and role-gate tests have something to exercise. mockAPIKey ""
// leaves the client unconfigured. The client is wired boot-enabled — use
// newTestW3WServerBoot to exercise a boot-time what3words.enabled=false
// state (the client is still constructed and wired either way, mirroring
// how internal/app/app.go builds the real one: SetEnabled reflects the
// config live, construction no longer gates on it).
func newTestW3WServer(t *testing.T, mockUpstream *httptest.Server, mockAPIKey string) (*Server, *session.MemoryManager, *config.Manager) {
	t.Helper()
	srv, sessMgr, cfgMgr, _ := newTestW3WServerBoot(t, mockUpstream, mockAPIKey, true)
	return srv, sessMgr, cfgMgr
}

// newTestW3WServerBoot is newTestW3WServer plus control over the boot-time
// what3words.enabled flag, and it also returns the wired *w3w.Client so a
// test can assert on its live state directly.
func newTestW3WServerBoot(t *testing.T, mockUpstream *httptest.Server, mockAPIKey string, bootEnabled bool) (*Server, *session.MemoryManager, *config.Manager, *w3w.Client) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nymeria.yaml")

	cfg := config.DefaultConfig()
	cfg.Station.Callsign = "W1AW"
	cfg.What3Words.Enabled = bootEnabled
	cfg.What3Words.APIKey = mockAPIKey
	cfg.What3Words.Results = 5
	if mockUpstream != nil {
		cfg.What3Words.BaseURL = mockUpstream.URL
	}
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	cfgMgr := config.NewManager(cfgPath, cfg)
	sessMgr := session.NewMemoryManager(session.MemoryManagerConfig{})
	tracker := station.NewMemoryTracker(cfg.Station)
	tm := transport.NewManager()

	// Mirrors internal/app/app.go: the client is always constructed, and
	// what3words.enabled is applied live via SetEnabled rather than gating
	// construction — that gate was the root cause of #1 (re-enabling from
	// a boot-disabled state had no client to ever wire up).
	client, err := w3w.New(w3w.Config{APIKey: cfg.What3Words.APIKey, BaseURL: cfg.What3Words.BaseURL})
	if err != nil {
		t.Fatalf("w3w.New: %v", err)
	}
	client.SetEnabled(bootEnabled)

	srv := New(tracker, tm, message.Engine(nil), nil,
		WithSessionManager(sessMgr),
		WithConfigManager(cfgMgr),
		WithWhat3Words(client),
	)

	return srv, sessMgr, cfgMgr, client
}

func mockW3WUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/convert-to-coordinates", func(w http.ResponseWriter, r *http.Request) {
		words := r.URL.Query().Get("words")
		if words == "bad.words.here" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "BadWords", "message": "no such address"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates":  map[string]float64{"lat": 51.520847, "lng": -0.195521},
			"words":        words,
			"nearestPlace": "Bayswater, London",
			"country":      "GB",
			"language":     "en",
		})
	})
	mux.HandleFunc("/autosuggest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"suggestions": []map[string]any{
				{"words": "filled.count.soap", "nearestPlace": "Bayswater, London", "country": "GB", "distanceToFocusKm": 12.4, "rank": 1},
			},
		})
	})
	mux.HandleFunc("/convert-to-3wa", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates": map[string]float64{"lat": 51.520847, "lng": -0.195521},
			"words":       "filled.count.soap",
		})
	})
	return httptest.NewServer(mux)
}

// --- Role gates ---

func TestW3WResolveRequiresOperator(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")

	adminToken(sessMgr) // consume the auto-promote slot so the next user is a real Observer
	u, _ := sessMgr.Create("observer", session.CreateOpts{})
	sessMgr.Approve(u.ID, session.RoleObserver)

	w := doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, u.Token)
	if w.Code != http.StatusForbidden {
		t.Errorf("observer: got %d, want 403", w.Code)
	}

	w = doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth: got %d, want 401", w.Code)
	}
}

func TestW3WStatusAllowsObserver(t *testing.T) {
	srv, sessMgr, _ := newTestW3WServer(t, nil, "secret")
	adminToken(sessMgr) // consume the auto-promote slot
	u, _ := sessMgr.Create("observer", session.CreateOpts{})
	sessMgr.Approve(u.ID, session.RoleObserver)

	w := doRequest(srv, "GET", "/api/w3w/status", nil, u.Token)
	if w.Code != http.StatusOK {
		t.Fatalf("observer status: got %d, want 200", w.Code)
	}
}

func TestW3WSettingsRequiresAdmin(t *testing.T) {
	srv, sessMgr, _ := newTestW3WServer(t, nil, "")
	adminToken(sessMgr) // consume the auto-promote slot
	opUser, _ := sessMgr.Create("op", session.CreateOpts{})
	sessMgr.Approve(opUser.ID, session.RoleOperator)

	w := doRequest(srv, "PUT", "/api/settings/what3words", what3wordsDTO{Enabled: true}, opUser.Token)
	if w.Code != http.StatusForbidden {
		t.Errorf("operator PUT settings: got %d, want 403", w.Code)
	}
}

// --- Status ---

func TestW3WStatusShape(t *testing.T) {
	srv, sessMgr, _ := newTestW3WServer(t, nil, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/status", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var body map[string]bool
	json.Unmarshal(w.Body.Bytes(), &body)
	if !body["configured"] {
		t.Error("configured = false, want true (key is set)")
	}
	if !body["enabled"] {
		t.Error("enabled = false, want true")
	}
}

func TestW3WStatusUnconfigured(t *testing.T) {
	srv, sessMgr, _ := newTestW3WServer(t, nil, "")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/status", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status must always be 200, got %d", w.Code)
	}
	var body map[string]bool
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["configured"] {
		t.Error("configured = true, want false (no key)")
	}
}

// --- Unconfigured proxy endpoints ---

func TestW3WProxyEndpointsReturn503WhenUnconfigured(t *testing.T) {
	srv, sessMgr, _ := newTestW3WServer(t, nil, "")
	token := adminToken(sessMgr)

	paths := []string{
		"/api/w3w/resolve?words=filled.count.soap",
		"/api/w3w/suggest?input=filled.count.so",
		"/api/w3w/reverse?lat=51.5&lon=-0.1",
	}
	for _, p := range paths {
		w := doRequest(srv, "GET", p, nil, token)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: got %d, want 503", p, w.Code)
		}
		var body map[string]string
		json.Unmarshal(w.Body.Bytes(), &body)
		if body["code"] != "not_configured" {
			t.Errorf("%s: code = %q, want not_configured", p, body["code"])
		}
	}
}

// --- Resolve ---

func TestW3WResolveSuccess(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/resolve?words=%2F%2F%2Ffilled.count.soap", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("resolve: got %d body=%s", w.Code, w.Body.String())
	}
	var res w3w.Result
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Words != "filled.count.soap" {
		t.Errorf("words = %q", res.Words)
	}
	if res.Lat != 51.520847 || res.Lon != -0.195521 {
		t.Errorf("lat/lon = %v/%v", res.Lat, res.Lon)
	}
}

func TestW3WResolveMissingWordsIsBadWords(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/resolve", nil, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "bad_words" {
		t.Errorf("code = %q, want bad_words", body["code"])
	}
}

func TestW3WResolveUpstreamBadWords(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/resolve?words=bad.words.here", nil, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "bad_words" {
		t.Errorf("code = %q, want bad_words", body["code"])
	}
}

func TestW3WResolveInvalidKeyMapsTo502NotUnauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/convert-to-coordinates", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "InvalidKey", "message": "bad key"}})
	})
	upstream := httptest.NewServer(mux)
	defer upstream.Close()

	srv, sessMgr, _ := newTestW3WServer(t, upstream, "wrong-key")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502 (never 401 — that would trip session handling)", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "invalid_key" {
		t.Errorf("code = %q, want invalid_key", body["code"])
	}
}

func TestW3WResolveQuotaExceeded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/convert-to-coordinates", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "QuotaExceeded", "message": "over quota"}})
	})
	upstream := httptest.NewServer(mux)
	defer upstream.Close()

	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "quota_exceeded" {
		t.Errorf("code = %q, want quota_exceeded", body["code"])
	}
}

// --- Suggest ---

func TestW3WSuggestSuccess(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/suggest?input=filled.count.so&lat=51.5&lon=-0.1", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("suggest: got %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Suggestions []w3w.Suggestion `json:"suggestions"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Suggestions) != 1 {
		t.Fatalf("suggestions len = %d, want 1", len(body.Suggestions))
	}
}

func TestW3WSuggestEmptyInputIsBadRequest(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/suggest", nil, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "bad_request" {
		t.Errorf("code = %q, want bad_request", body["code"])
	}
}

func TestW3WSuggestOmitsFocusWithoutLatLon(t *testing.T) {
	var sawFocus bool
	mux := http.NewServeMux()
	mux.HandleFunc("/autosuggest", func(w http.ResponseWriter, r *http.Request) {
		_, sawFocus = r.URL.Query()["focus"]
		json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{}})
	})
	upstream := httptest.NewServer(mux)
	defer upstream.Close()

	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	doRequest(srv, "GET", "/api/w3w/suggest?input=filled.count.so", nil, token)
	if sawFocus {
		t.Error("focus param present without lat/lon query params")
	}
}

// --- Reverse ---

func TestW3WReverseSuccess(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/w3w/reverse?lat=51.520847&lon=-0.195521", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("reverse: got %d body=%s", w.Code, w.Body.String())
	}
	var res w3w.Result
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Words != "filled.count.soap" {
		t.Errorf("words = %q", res.Words)
	}
}

func TestW3WReverseOutOfRangeIsBadRequest(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, _ := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	for _, q := range []string{"lat=95&lon=0", "lat=0&lon=200", "lat=abc&lon=0", "lon=0"} {
		w := doRequest(srv, "GET", "/api/w3w/reverse?"+q, nil, token)
		if w.Code != http.StatusBadRequest {
			t.Errorf("query %q: got %d, want 400", q, w.Code)
		}
		var body map[string]string
		json.Unmarshal(w.Body.Bytes(), &body)
		if body["code"] != "bad_request" {
			t.Errorf("query %q: code = %q, want bad_request", q, body["code"])
		}
	}
}

// --- Settings DTO ---

func TestToWhat3WordsDTONeverPopulatesAPIKey(t *testing.T) {
	cfg := config.What3WordsConfig{Enabled: true, APIKey: "super-secret-value", BaseURL: "https://api.what3words.com/v3", Results: 5}
	dto := toWhat3WordsDTO(cfg)

	data, _ := json.Marshal(dto)
	if strings.Contains(string(data), "super-secret-value") {
		t.Fatalf("toWhat3WordsDTO leaked the API key into JSON: %s", data)
	}
	if !dto.APIKeyConfigured {
		t.Error("APIKeyConfigured = false, want true")
	}
	if dto.APIKeySource != "config" {
		t.Errorf("APIKeySource = %q, want config", dto.APIKeySource)
	}
}

func TestFromWhat3WordsDTOPreservesKeyOnEmptyOrMask(t *testing.T) {
	existing := config.What3WordsConfig{APIKey: "existing-key", BaseURL: "https://x", Results: 5}

	got := fromWhat3WordsDTO(what3wordsDTO{APIKey: "", BaseURL: "https://y", Results: 3, Enabled: true}, existing)
	if got.APIKey != "existing-key" {
		t.Errorf("empty apiKey: got %q, want preserved existing-key", got.APIKey)
	}

	got = fromWhat3WordsDTO(what3wordsDTO{APIKey: "***", BaseURL: "https://y", Results: 3, Enabled: true}, existing)
	if got.APIKey != "existing-key" {
		t.Errorf("masked apiKey: got %q, want preserved existing-key", got.APIKey)
	}

	got = fromWhat3WordsDTO(what3wordsDTO{APIKey: "brand-new", BaseURL: "https://y", Results: 3, Enabled: true}, existing)
	if got.APIKey != "brand-new" {
		t.Errorf("real apiKey: got %q, want brand-new", got.APIKey)
	}
}

func TestGetSettingsW3WDoesNotLeakKey(t *testing.T) {
	srv, sessMgr, _ := newTestW3WServer(t, nil, "top-secret-key")
	token := adminToken(sessMgr)

	w := doRequest(srv, "GET", "/api/settings", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("GET settings: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "top-secret-key") {
		t.Fatalf("GET /api/settings leaked the what3words API key: %s", w.Body.String())
	}

	var resp settingsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.What3Words.APIKeyConfigured {
		t.Error("apiKeyConfigured = false, want true")
	}
}

func TestUpdateWhat3WordsIsLiveNoRestart(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, cfgMgr := newTestW3WServer(t, upstream, "")
	token := adminToken(sessMgr)

	// Unconfigured to start — proxy should 503.
	w := doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("pre-update resolve: got %d, want 503", w.Code)
	}

	w = doRequest(srv, "PUT", "/api/settings/what3words", what3wordsDTO{
		Enabled: true, APIKey: "new-secret", BaseURL: upstream.URL, Results: 5,
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT settings/what3words: got %d body=%s", w.Code, w.Body.String())
	}
	var resp updateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.RestartRequired {
		t.Error("RestartRequired = true, want false (key changes must be live)")
	}

	if cfgMgr.Get().What3Words.APIKey != "new-secret" {
		t.Errorf("persisted key = %q, want new-secret", cfgMgr.Get().What3Words.APIKey)
	}

	// Now the SAME already-running server (no restart) should resolve successfully.
	w = doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("post-update resolve: got %d body=%s", w.Code, w.Body.String())
	}
}

// TestBootDisabledThenEnabledViaSettingsGoesLive reproduces review finding
// #1: what3words.enabled: false at boot (a documented, supported config
// state) must not leave the feature permanently dead for that process —
// flipping Enabled to true through Settings has to work with no restart,
// the same way pasting a key does.
func TestBootDisabledThenEnabledViaSettingsGoesLive(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, cfgMgr, client := newTestW3WServerBoot(t, upstream, "", false)
	token := adminToken(sessMgr)

	if client.Enabled() {
		t.Fatal("precondition: client.Enabled() = true, want false (boot-disabled)")
	}

	w := doRequest(srv, "GET", "/api/w3w/status", nil, token)
	var status map[string]bool
	json.Unmarshal(w.Body.Bytes(), &status)
	if status["configured"] || status["enabled"] {
		t.Errorf("boot-disabled status = %+v, want both false", status)
	}

	w = doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("boot-disabled resolve: got %d, want 503", w.Code)
	}

	w = doRequest(srv, "PUT", "/api/settings/what3words", what3wordsDTO{
		Enabled: true, APIKey: "new-secret", BaseURL: upstream.URL, Results: 5,
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT settings/what3words: got %d body=%s", w.Code, w.Body.String())
	}
	var resp updateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.RestartRequired {
		t.Error("RestartRequired = true, want false")
	}
	if !cfgMgr.Get().What3Words.Enabled {
		t.Error("persisted Enabled = false, want true")
	}

	// Same already-running server, no restart: status and resolve must now
	// both be live.
	w = doRequest(srv, "GET", "/api/w3w/status", nil, token)
	json.Unmarshal(w.Body.Bytes(), &status)
	if !status["configured"] || !status["enabled"] {
		t.Errorf("post-enable status = %+v, want both true", status)
	}

	w = doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("post-enable resolve: got %d body=%s", w.Code, w.Body.String())
	}
}

// TestDisableViaSettingsGoesLiveImmediately reproduces review finding #4:
// unchecking "Enabled" in the what3words Settings section (the section
// labeled "Live") must take the feature down immediately on the running
// server, not only after a restart.
func TestDisableViaSettingsGoesLiveImmediately(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, cfgMgr, client := newTestW3WServerBoot(t, upstream, "secret", true)
	token := adminToken(sessMgr)

	if !client.Configured() {
		t.Fatal("precondition: client.Configured() = false, want true")
	}
	w := doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("precondition resolve: got %d, want 200", w.Code)
	}

	// apiKey omitted -> preserved; only Enabled flips.
	w = doRequest(srv, "PUT", "/api/settings/what3words", what3wordsDTO{
		Enabled: false, BaseURL: upstream.URL, Results: 5,
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT settings/what3words: got %d body=%s", w.Code, w.Body.String())
	}
	var resp updateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.RestartRequired {
		t.Error("RestartRequired = true, want false")
	}
	if cfgMgr.Get().What3Words.Enabled {
		t.Error("persisted Enabled = true, want false")
	}
	if cfgMgr.Get().What3Words.APIKey != "secret" {
		t.Errorf("persisted key = %q, want preserved \"secret\"", cfgMgr.Get().What3Words.APIKey)
	}

	w = doRequest(srv, "GET", "/api/w3w/status", nil, token)
	var status map[string]bool
	json.Unmarshal(w.Body.Bytes(), &status)
	if status["configured"] || status["enabled"] {
		t.Errorf("post-disable status = %+v, want both false", status)
	}

	w = doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("post-disable resolve: got %d, want 503", w.Code)
	}
}

func TestDeleteWhat3WordsKey(t *testing.T) {
	upstream := mockW3WUpstream(t)
	defer upstream.Close()
	srv, sessMgr, cfgMgr := newTestW3WServer(t, upstream, "secret")
	token := adminToken(sessMgr)

	w := doRequest(srv, "DELETE", "/api/settings/what3words/key", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE key: got %d", w.Code)
	}
	if cfgMgr.Get().What3Words.APIKey != "" {
		t.Errorf("persisted key = %q, want empty after delete", cfgMgr.Get().What3Words.APIKey)
	}

	w = doRequest(srv, "GET", "/api/w3w/resolve?words=filled.count.soap", nil, token)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("resolve after delete: got %d, want 503", w.Code)
	}
}
