package w3w

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func mustClient(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNewDefaultBaseURL(t *testing.T) {
	c := mustClient(t, Config{})
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
}

func TestNewMalformedBaseURL(t *testing.T) {
	if _, err := New(Config{BaseURL: "://bad"}); err == nil {
		t.Fatal("New with malformed base URL: want error, got nil")
	}
}

func TestConfigured(t *testing.T) {
	c := mustClient(t, Config{})
	if c.Configured() {
		t.Error("Configured() with empty key = true, want false")
	}
	c.SetAPIKey("test-key")
	if !c.Configured() {
		t.Error("Configured() after SetAPIKey = false, want true")
	}
}

// TestNewDefaultsEnabled asserts a freshly constructed Client starts
// enabled, so every existing call site that never touches SetEnabled keeps
// its prior Configured() behavior (key present -> configured).
func TestNewDefaultsEnabled(t *testing.T) {
	c := mustClient(t, Config{APIKey: "test-key"})
	if !c.Enabled() {
		t.Error("Enabled() on a fresh Client = false, want true")
	}
	if !c.Configured() {
		t.Error("Configured() on a fresh Client with a key = false, want true")
	}
}

// TestSetEnabledGatesConfigured covers the live enable/disable toggle: a
// Settings save that flips what3words.enabled must take effect immediately
// on the already-running client, with no restart, in both directions.
func TestSetEnabledGatesConfigured(t *testing.T) {
	c := mustClient(t, Config{APIKey: "test-key"})
	if !c.Configured() {
		t.Fatal("precondition: Configured() = false, want true")
	}

	c.SetEnabled(false)
	if c.Enabled() {
		t.Error("Enabled() after SetEnabled(false) = true, want false")
	}
	if c.Configured() {
		t.Error("Configured() after SetEnabled(false) = true, want false (key untouched, only disabled)")
	}

	c.SetEnabled(true)
	if !c.Enabled() {
		t.Error("Enabled() after SetEnabled(true) = false, want true")
	}
	if !c.Configured() {
		t.Error("Configured() after SetEnabled(true) = false, want true (key still present)")
	}
}

// TestSetEnabledFalseBlocksResolveWithoutUpstreamCall mirrors the HTTP
// handler's guard (c == nil || !c.Configured()) one level down: a disabled
// client must never reach the network, matching the not_configured 503 the
// server layer returns.
func TestSetEnabledFalseBlocksResolveWithoutUpstreamCall(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates": map[string]float64{"lat": 1, "lng": 2},
			"words":       "filled.count.soap",
		})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "test-key", BaseURL: srv.URL})
	c.SetEnabled(false)

	_, err := c.Resolve(context.Background(), "filled.count.soap")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Resolve() while disabled: err = %v, want ErrNotConfigured", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("upstream called %d times while disabled, want 0", calls)
	}
}

// --- Resolve ---

func TestResolveHappyPath(t *testing.T) {
	var gotPath, gotWords, gotKeyHeader, gotKeyQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotWords = r.URL.Query().Get("words")
		gotKeyHeader = r.Header.Get("X-Api-Key")
		gotKeyQuery = r.URL.Query().Get("key")
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates":  map[string]float64{"lat": 51.520847, "lng": -0.195521},
			"words":        "filled.count.soap",
			"nearestPlace": "Bayswater, London",
			"country":      "GB",
			"language":     "en",
		})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	res, err := c.Resolve(context.Background(), "///Filled.Count.Soap")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if gotPath != "/convert-to-coordinates" {
		t.Errorf("path = %q, want /convert-to-coordinates", gotPath)
	}
	if gotWords != "filled.count.soap" {
		t.Errorf("words param = %q, want normalized filled.count.soap", gotWords)
	}
	if gotKeyHeader != "secret" {
		t.Errorf("X-Api-Key header = %q, want secret", gotKeyHeader)
	}
	if gotKeyQuery != "" {
		t.Errorf("key query param = %q, want empty (must never leak the key into the URL)", gotKeyQuery)
	}
	if res.Words != "filled.count.soap" || res.Lat != 51.520847 || res.Lon != -0.195521 {
		t.Errorf("Result = %+v, unexpected", res)
	}
	if res.NearestPlace != "Bayswater, London" || res.Country != "GB" || res.Language != "en" {
		t.Errorf("Result = %+v, unexpected", res)
	}
}

func TestResolveShortCircuitsOnInvalidWords(t *testing.T) {
	var called int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	_, err := c.Resolve(context.Background(), "not a valid address")
	if !errors.Is(err, ErrBadWords) {
		t.Fatalf("err = %v, want ErrBadWords", err)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatal("upstream was called for a syntactically invalid address")
	}
}

func TestResolveCacheHitSkipsUpstream(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates": map[string]float64{"lat": 1, "lng": 2},
			"words":       "filled.count.soap",
		})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	ctx := context.Background()
	if _, err := c.Resolve(ctx, "filled.count.soap"); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	if _, err := c.Resolve(ctx, "filled.count.soap"); err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (second call should hit cache)", got)
	}
}

func TestResolveCacheExpiresAfterTTL(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates": map[string]float64{"lat": 1, "lng": 2},
			"words":       "filled.count.soap",
		})
	}))
	defer srv.Close()

	now := time.Now()
	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL, ForwardTTL: time.Minute, Now: func() time.Time { return now }})
	ctx := context.Background()
	if _, err := c.Resolve(ctx, "filled.count.soap"); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := c.Resolve(ctx, "filled.count.soap"); err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2 (cache should have expired)", got)
	}
}

func TestResolveErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		httpStatus int
		body       string
		wantErr    error
	}{
		{"bad words", 400, `{"error":{"code":"BadWords","message":"bad"}}`, ErrBadWords},
		{"invalid key", 401, `{"error":{"code":"InvalidKey","message":"nope"}}`, ErrInvalidKey},
		{"suspended key", 401, `{"error":{"code":"SuspendedKey","message":"nope"}}`, ErrInvalidKey},
		{"quota exceeded", 402, `{"error":{"code":"QuotaExceeded","message":"over"}}`, ErrQuotaExceeded},
		{"http 429", 429, `{"error":{"code":"TooManyRequests","message":"slow down"}}`, ErrRateLimited},
		{"http 500 html body", 500, `<html>oops</html>`, ErrUpstream},
		{"unparseable json", 400, `not json at all`, ErrUpstream},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.httpStatus)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
			_, err := c.Resolve(context.Background(), "filled.count.soap")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want wrapping %v", err, tt.wantErr)
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				if apiErr.HTTPStatus != tt.httpStatus {
					t.Errorf("APIError.HTTPStatus = %d, want %d", apiErr.HTTPStatus, tt.httpStatus)
				}
			} else {
				t.Errorf("err is not an *APIError: %v", err)
			}
		})
	}
}

func TestResolveErrorsAreNotCached(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	ctx := context.Background()
	c.Resolve(ctx, "filled.count.soap")
	c.Resolve(ctx, "filled.count.soap")
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2 (errors must not be cached)", got)
	}
}

func TestResolveNetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // closed before the call — connection refused

	c := mustClient(t, Config{APIKey: "secret", BaseURL: url})
	_, err := c.Resolve(context.Background(), "filled.count.soap")
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v, want ErrNetwork", err)
	}
}

func TestResolveTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	c := mustClient(t, Config{
		APIKey:     "secret",
		BaseURL:    srv.URL,
		HTTPClient: &http.Client{Timeout: 20 * time.Millisecond},
	})
	_, err := c.Resolve(context.Background(), "filled.count.soap")
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v, want ErrNetwork", err)
	}
}

func TestResolveNotConfigured(t *testing.T) {
	c := mustClient(t, Config{})
	_, err := c.Resolve(context.Background(), "filled.count.soap")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

// --- Suggest ---

func TestSuggestBuildsParamsAndClamps(t *testing.T) {
	var gotInput, gotN, gotFocus string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotInput = r.URL.Query().Get("input")
		gotN = r.URL.Query().Get("n-results")
		gotFocus = r.URL.Query().Get("focus")
		json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{}})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	lat, lon := 51.520847, -0.195521
	_, err := c.Suggest(context.Background(), "filled.count.so", &lat, &lon, 99)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if gotInput != "filled.count.so" {
		t.Errorf("input = %q", gotInput)
	}
	if gotN != "10" {
		t.Errorf("n-results = %q, want clamped to 10", gotN)
	}
	if gotFocus != "51.520847,-0.195521" {
		t.Errorf("focus = %q, want 6dp lat,lng", gotFocus)
	}
}

func TestSuggestOmitsFocusWhenNil(t *testing.T) {
	var sawFocus bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawFocus = r.URL.Query()["focus"]
		json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{}})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	if _, err := c.Suggest(context.Background(), "filled.count.so", nil, nil, 0); err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if sawFocus {
		t.Error("focus param present when no focus was given")
	}
}

func TestSuggestDefaultNWhenZeroOrNegative(t *testing.T) {
	var gotN string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotN = r.URL.Query().Get("n-results")
		json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{}})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	if _, err := c.Suggest(context.Background(), "filled.count.so", nil, nil, -5); err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if gotN != "3" {
		t.Errorf("n-results = %q, want default 3", gotN)
	}
}

func TestSuggestReturnsNonNilEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{}})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	got, err := c.Suggest(context.Background(), "filled.count.so", nil, nil, 3)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if got == nil {
		t.Fatal("Suggest returned nil slice, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestSuggestDecodesFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"suggestions": []map[string]any{
				{"words": "filled.count.soap", "nearestPlace": "Bayswater, London", "country": "GB", "distanceToFocusKm": 12.4, "rank": 1},
			},
		})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	got, err := c.Suggest(context.Background(), "filled.count.so", nil, nil, 3)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	want := Suggestion{Words: "filled.count.soap", NearestPlace: "Bayswater, London", Country: "GB", DistanceToFocusKm: 12.4, Rank: 1}
	if got[0] != want {
		t.Errorf("got %+v, want %+v", got[0], want)
	}
}

func TestSuggestCacheKeyIncludesFocusAndN(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{}})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	ctx := context.Background()
	lat, lon := 1.0, 2.0
	c.Suggest(ctx, "filled.count.so", &lat, &lon, 3)
	c.Suggest(ctx, "filled.count.so", &lat, &lon, 3) // cache hit
	c.Suggest(ctx, "filled.count.so", nil, nil, 3)   // different focus -> miss
	c.Suggest(ctx, "filled.count.so", &lat, &lon, 5) // different n -> miss

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("upstream calls = %d, want 3 (one cache hit expected)", got)
	}
}

// --- Reverse ---

func TestReverseBuildsCoordinatesParam(t *testing.T) {
	var gotCoords string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCoords = r.URL.Query().Get("coordinates")
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates": map[string]float64{"lat": 51.520847, "lng": -0.195521},
			"words":       "filled.count.soap",
		})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	res, err := c.Reverse(context.Background(), 51.520847, -0.195521)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if gotCoords != "51.520847,-0.195521" {
		t.Errorf("coordinates param = %q", gotCoords)
	}
	if res.Words != "filled.count.soap" {
		t.Errorf("Words = %q", res.Words)
	}
}

func TestReverseCacheKeyRoundsToSixDecimals(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates": map[string]float64{"lat": 51.520847, "lng": -0.195521},
			"words":       "filled.count.soap",
		})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	ctx := context.Background()
	if _, err := c.Reverse(ctx, 51.5208470001, -0.195521); err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if _, err := c.Reverse(ctx, 51.520847, -0.195521); err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (6dp rounding should share the cache entry)", got)
	}
}

// --- Key rotation ---

func TestSetAPIKeyFlushesOnlySuggestCache(t *testing.T) {
	var forwardCalls, suggestCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/convert-to-coordinates", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&forwardCalls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates": map[string]float64{"lat": 1, "lng": 2},
			"words":       "filled.count.soap",
		})
	})
	mux.HandleFunc("/autosuggest", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&suggestCalls, 1)
		json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "old-key", BaseURL: srv.URL})
	ctx := context.Background()
	c.Resolve(ctx, "filled.count.soap")
	c.Suggest(ctx, "filled.count.so", nil, nil, 3)

	c.SetAPIKey("new-key")

	c.Resolve(ctx, "filled.count.soap")            // should still hit forward cache
	c.Suggest(ctx, "filled.count.so", nil, nil, 3) // should have been flushed

	if got := atomic.LoadInt32(&forwardCalls); got != 1 {
		t.Errorf("forward calls = %d, want 1 (forward cache survives key rotation)", got)
	}
	if got := atomic.LoadInt32(&suggestCalls); got != 2 {
		t.Errorf("suggest calls = %d, want 2 (suggest cache flushed on key rotation)", got)
	}
	if c.APIKey() != "new-key" {
		t.Errorf("APIKey() = %q, want new-key", c.APIKey())
	}
}

// --- Concurrency ---

func TestResolveConcurrentSameWords(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"coordinates": map[string]float64{"lat": 1, "lng": 2},
			"words":       "filled.count.soap",
		})
	}))
	defer srv.Close()

	c := mustClient(t, Config{APIKey: "secret", BaseURL: srv.URL})
	ctx := context.Background()

	results := make(chan Result, 50)
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		go func() {
			r, err := c.Resolve(ctx, "filled.count.soap")
			results <- r
			errs <- err
		}()
	}
	for i := 0; i < 50; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent Resolve error: %v", err)
		}
		r := <-results
		if r.Words != "filled.count.soap" {
			t.Fatalf("concurrent Resolve result mismatch: %+v", r)
		}
	}
	if got := atomic.LoadInt32(&calls); got < 1 || got > 50 {
		t.Fatalf("upstream calls = %d, want between 1 and 50", got)
	}
}
