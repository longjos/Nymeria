package wxalert

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestProvider(t *testing.T, srv *httptest.Server) *NWSProvider {
	t.Helper()
	p, err := NewNWSProvider(NWSConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)"})
	if err != nil {
		t.Fatalf("NewNWSProvider: %v", err)
	}
	return p
}

func TestValidateUserAgent(t *testing.T) {
	tests := []struct {
		ua      string
		wantErr bool
	}{
		{"", true},
		{"Nymeria/1.0", true},
		{"Nymeria/1.0 (a@b.c)", false},
		{"Nymeria/test (ops@example.org)", false},
	}
	for _, tt := range tests {
		err := ValidateUserAgent(tt.ua)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateUserAgent(%q) err=%v, wantErr=%v", tt.ua, err, tt.wantErr)
		}
	}
}

func TestNewNWSProviderDefaultBaseURL(t *testing.T) {
	p, err := NewNWSProvider(NWSConfig{UserAgent: "Nymeria/1.0 (a@b.c)"})
	if err != nil {
		t.Fatalf("NewNWSProvider: %v", err)
	}
	if p.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", p.baseURL, DefaultBaseURL)
	}
	if DefaultBaseURL != "https://api.weather.gov" {
		t.Errorf("DefaultBaseURL = %q", DefaultBaseURL)
	}
}

func TestFetchActiveHeadersAndQuery(t *testing.T) {
	var gotPath, gotUA, gotAccept, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write([]byte(`{"type":"FeatureCollection","features":[]}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, _, err := p.FetchActive(context.Background(), ActiveQuery{Areas: []string{"MIC081", "OHZ001"}})
	if err != nil {
		t.Fatalf("FetchActive: %v", err)
	}
	if gotPath != "/alerts/active" {
		t.Errorf("path = %q, want /alerts/active", gotPath)
	}
	if gotUA != "Nymeria/test (ops@example.org)" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if gotAccept != "application/geo+json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if !strings.Contains(gotQuery, "status=actual") {
		t.Errorf("query %q missing status=actual", gotQuery)
	}
	if strings.Contains(gotQuery, "limit") {
		t.Errorf("query %q must never send limit", gotQuery)
	}
	if !strings.Contains(gotQuery, "area=MIC081%2COHZ001") && !strings.Contains(gotQuery, "area=MIC081,OHZ001") {
		t.Errorf("query %q missing area=MIC081,OHZ001 (state-prefix dedup/sort is the poller's job, not the provider's)", gotQuery)
	}
}

func TestFetchActivePointFallback(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"type":"FeatureCollection","features":[]}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, _, err := p.FetchActive(context.Background(), ActiveQuery{Point: &LatLon{Lat: 42.9634, Lon: -85.6681}})
	if err != nil {
		t.Fatalf("FetchActive: %v", err)
	}
	if !strings.Contains(gotQuery, "point=42.9634%2C-85.6681") {
		t.Errorf("query = %q, want point=42.9634,-85.6681", gotQuery)
	}
}

func TestFetchActiveIncludeTestOmitsStatusFilter(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"type":"FeatureCollection","features":[]}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	if _, _, err := p.FetchActive(context.Background(), ActiveQuery{IncludeTest: true}); err != nil {
		t.Fatalf("FetchActive: %v", err)
	}
	if strings.Contains(gotQuery, "status=actual") {
		t.Errorf("query = %q, must not filter status when IncludeTest", gotQuery)
	}
}

func TestFetchActive403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"title":"Forbidden"}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, _, err := p.FetchActive(context.Background(), ActiveQuery{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestFetchActive429RetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, _, err := p.FetchActive(context.Background(), ActiveQuery{})
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("err = %v, want *RateLimitError", err)
	}
	if rle.RetryAfter != 5*time.Second {
		t.Errorf("RetryAfter = %v, want 5s", rle.RetryAfter)
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("errors.Is(err, ErrRateLimited) = false")
	}
}

func TestFetchActiveMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json at all`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, body, err := p.FetchActive(context.Background(), ActiveQuery{})
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
	if string(body) != "not json at all" {
		t.Errorf("body = %q, want the raw response echoed back for the provenance popover", body)
	}
}

func TestFetchActiveHTMLOutagePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body>Cloudflare says no</body></html>`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, _, err := p.FetchActive(context.Background(), ActiveQuery{})
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
	if !strings.Contains(err.Error(), "text/html") {
		t.Errorf("err = %v, want it to mention text/html", err)
	}
}

func TestFetchActive5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	_, _, err := p.FetchActive(context.Background(), ActiveQuery{})
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

func TestFetchActiveTimeout(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer func() { close(block); srv.Close() }()

	p, err := NewNWSProvider(NWSConfig{BaseURL: srv.URL, UserAgent: "Nymeria/test (ops@example.org)", Client: &http.Client{Timeout: 50 * time.Millisecond}})
	if err != nil {
		t.Fatalf("NewNWSProvider: %v", err)
	}
	start := time.Now()
	_, _, err = p.FetchActive(context.Background(), ActiveQuery{})
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("timeout took too long: %v", time.Since(start))
	}
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestFetchCount(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"total":233,"areas":{"MI":5,"OH":3},"zones":{}}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	c, err := p.FetchCount(context.Background(), ActiveQuery{})
	if err != nil {
		t.Fatalf("FetchCount: %v", err)
	}
	if gotPath != "/alerts/active/count" {
		t.Errorf("path = %q", gotPath)
	}
	if c.Total != 233 || c.Areas["MI"] != 5 {
		t.Errorf("count = %+v", c)
	}
}

func TestCountSignatureStableAndSensitive(t *testing.T) {
	c1 := Count{Total: 10, Areas: map[string]int{"MI": 2, "OH": 3}}
	c2 := Count{Total: 10, Areas: map[string]int{"OH": 3, "MI": 2}}
	if CountSignature(c1, []string{"MI", "OH"}) != CountSignature(c2, []string{"OH", "MI"}) {
		t.Errorf("signature should not depend on map/slice order")
	}
	c3 := Count{Total: 10, Areas: map[string]int{"MI": 3, "OH": 3}}
	if CountSignature(c1, []string{"MI", "OH"}) == CountSignature(c3, []string{"MI", "OH"}) {
		t.Errorf("signature did not change when a watched area's count changed")
	}
}

func TestFetchZoneCountyVsForecast(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"geometry":null,"properties":{"id":"MIC081","type":"county","name":"Kent","state":"MI"}}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	rec, err := p.FetchZone(context.Background(), "county", "MIC081")
	if err != nil {
		t.Fatalf("FetchZone: %v", err)
	}
	if gotPath != "/zones/county/MIC081" {
		t.Errorf("path = %q", gotPath)
	}
	if rec.Name != "Kent" {
		t.Errorf("Name = %q", rec.Name)
	}
}

func TestResolvePoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"properties":{"cwa":"GRR","forecastZone":"https://api.weather.gov/zones/forecast/MIZ057","county":"https://api.weather.gov/zones/county/MIC081","fireWeatherZone":"https://api.weather.gov/zones/fire/MIZ057"}}`))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv)
	pz, err := p.ResolvePoint(context.Background(), LatLon{Lat: 42.9634, Lon: -85.6681})
	if err != nil {
		t.Fatalf("ResolvePoint: %v", err)
	}
	if pz.Forecast != "MIZ057" || pz.County != "MIC081" || pz.Fire != "MIZ057" || pz.CWA != "GRR" {
		t.Errorf("PointZones = %+v", pz)
	}
}
