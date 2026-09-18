package wxalert

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is api.weather.gov, used when NWSConfig.BaseURL is empty.
const DefaultBaseURL = "https://api.weather.gov"

// Sentinel provider errors. The poller turns these into plain-words
// LinkStatus.LastError text; nothing in this package ever panics on them.
var (
	ErrForbidden   = errors.New("nws rejected the request — check the User-Agent contact")
	ErrRateLimited = errors.New("rate limited by nws")
	ErrUpstream    = errors.New("nws upstream error")
	ErrTimeout     = errors.New("timed out waiting for nws")
)

// RateLimitError carries the Retry-After duration for ErrRateLimited.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited by NWS (429), retrying in %s", e.RetryAfter)
}
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// ActiveQuery narrows GET /alerts/active. Areas are two-letter state codes;
// when empty and Point is set, the request uses ?point=lat,lon; when both
// are empty, the request is unfiltered (the whole feed).
type ActiveQuery struct {
	Areas       []string
	Point       *LatLon
	IncludeTest bool
}

// Count is the trimmed shape of GET /alerts/active/count.
type Count struct {
	Total int            `json:"total"`
	Areas map[string]int `json:"areas"`
	Zones map[string]int `json:"zones"`
}

// PointZones is the trimmed shape of GET /points/{lat},{lon}.
type PointZones struct {
	Forecast   string
	County     string
	Fire       string
	CWA        string
	ResolvedAt time.Time
}

// Provider is the CAP source abstraction; NWS is the only v1 implementation.
// It also satisfies ZoneProvider so a ZoneCache can be built directly from
// one.
type Provider interface {
	FetchActive(ctx context.Context, q ActiveQuery) (Decoded, []byte, error)
	FetchCount(ctx context.Context, q ActiveQuery) (Count, error)
	FetchZone(ctx context.Context, zoneType, ugc string) (ZoneRecord, error)
	ListZones(ctx context.Context, zoneType, state string) ([]ZoneRef, error)
	ResolvePoint(ctx context.Context, p LatLon) (PointZones, error)
}

// NWSConfig configures an NWSProvider.
type NWSConfig struct {
	BaseURL   string
	UserAgent string // required, must include a parenthesized contact
	Client    *http.Client
}

var userAgentContactPattern = regexp.MustCompile(`\([^)]+\)`)

// ValidateUserAgent reports whether ua looks like "Name/Version (contact)" —
// NWS requires a way to reach the operator; a bare "Nymeria/1.0" is rejected.
func ValidateUserAgent(ua string) error {
	if strings.TrimSpace(ua) == "" {
		return errors.New("nws user agent contact is required")
	}
	if !userAgentContactPattern.MatchString(ua) {
		return errors.New("nws user agent contact is required")
	}
	return nil
}

// NWSProvider implements Provider against the live (or test) NWS API.
type NWSProvider struct {
	baseURL   string
	userAgent string
	client    *http.Client
}

// NewNWSProvider validates cfg and returns a ready Provider.
func NewNWSProvider(cfg NWSConfig) (*NWSProvider, error) {
	if err := ValidateUserAgent(cfg.UserAgent); err != nil {
		return nil, err
	}
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &NWSProvider{baseURL: strings.TrimRight(base, "/"), userAgent: cfg.UserAgent, client: client}, nil
}

func (p *NWSProvider) newRequest(ctx context.Context, path string, query url.Values) (*http.Request, error) {
	u := p.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "application/geo+json")
	return req, nil
}

const maxBodyBytes = 8 << 20 // 8 MiB

func (p *NWSProvider) do(req *http.Request) ([]byte, error) {
	resp, err := p.client.Do(req)
	if err != nil {
		if req.Context().Err() != nil {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: reading body: %v", ErrUpstream, err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusForbidden:
		return nil, ErrForbidden
	case http.StatusTooManyRequests:
		retry := 30 * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				retry = time.Duration(secs) * time.Second
			}
		}
		return nil, &RateLimitError{RetryAfter: retry}
	default:
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("%w: HTTP %d", ErrUpstream, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrMalformed, resp.StatusCode, firstLine(body))
	}

	// NWS always answers with application/geo+json; the one shape we must
	// actively reject is an HTML outage/interstitial page (Cloudflare-style)
	// masquerading as a 200. A plain "text/plain" sniff (the Go default for
	// a handler that never sets Content-Type) is left alone — it still gets
	// a fair shot at JSON decoding.
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "html") {
		return nil, fmt.Errorf("%w: unexpected content type %s", ErrMalformed, ct)
	}
	return body, nil
}

func firstLine(body []byte) string {
	s := string(body)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// areasFromQuery returns the distinct, sorted, comma-joined area= value for
// q.Areas (deduplicated), or "" when q.Areas is empty.
func areasFromQuery(areas []string) string {
	seen := map[string]bool{}
	var out []string
	for _, a := range areas {
		a = strings.ToUpper(strings.TrimSpace(a))
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func (p *NWSProvider) activeQueryValues(q ActiveQuery) url.Values {
	v := url.Values{}
	if !q.IncludeTest {
		v.Set("status", "actual")
	}
	if areas := areasFromQuery(q.Areas); areas != "" {
		v.Set("area", areas)
	} else if q.Point != nil {
		v.Set("point", fmt.Sprintf("%g,%g", q.Point.Lat, q.Point.Lon))
	}
	return v
}

// FetchActive fetches and decodes GET /alerts/active. The raw body is also
// returned so the poller can hash it for change detection without a second
// round trip.
func (p *NWSProvider) FetchActive(ctx context.Context, q ActiveQuery) (Decoded, []byte, error) {
	req, err := p.newRequest(ctx, "/alerts/active", p.activeQueryValues(q))
	if err != nil {
		return Decoded{}, nil, err
	}
	body, err := p.do(req)
	if err != nil {
		return Decoded{}, nil, err
	}
	d, err := DecodeCollection(bytes.NewReader(body), time.Now())
	if err != nil {
		return Decoded{}, body, err
	}
	if !q.IncludeTest {
		d.Alerts = filterActual(d.Alerts)
	}
	return d, body, nil
}

func filterActual(alerts []Alert) []Alert {
	out := make([]Alert, 0, len(alerts))
	for _, a := range alerts {
		if a.Status == StatusActual {
			out = append(out, a)
		}
	}
	return out
}

// FetchCount fetches GET /alerts/active/count — a cheap (~15 KB) change
// detector the poller uses to avoid a full fetch every tick.
func (p *NWSProvider) FetchCount(ctx context.Context, q ActiveQuery) (Count, error) {
	req, err := p.newRequest(ctx, "/alerts/active/count", nil)
	if err != nil {
		return Count{}, err
	}
	body, err := p.do(req)
	if err != nil {
		return Count{}, err
	}
	var c Count
	if err := json.Unmarshal(body, &c); err != nil {
		return Count{}, fmt.Errorf("%w: count: %v", ErrMalformed, err)
	}
	return c, nil
}

// CountSignature is a stable hash of the parts of Count relevant to the
// current watch: total, plus the counts for exactly the areas being watched.
func CountSignature(c Count, areas []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "total:%d|", c.Total)
	sortedAreas := append([]string{}, areas...)
	sort.Strings(sortedAreas)
	for _, a := range sortedAreas {
		fmt.Fprintf(h, "%s:%d|", strings.ToUpper(a), c.Areas[strings.ToUpper(a)])
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// FetchZone implements ZoneProvider: GET /zones/{zoneType}/{ugc}.
func (p *NWSProvider) FetchZone(ctx context.Context, zoneType, ugc string) (ZoneRecord, error) {
	req, err := p.newRequest(ctx, fmt.Sprintf("/zones/%s/%s", zoneType, ugc), nil)
	if err != nil {
		return ZoneRecord{}, err
	}
	body, err := p.do(req)
	if err != nil {
		return ZoneRecord{}, err
	}
	var raw struct {
		Geometry   json.RawMessage `json:"geometry"`
		Properties struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ZoneRecord{}, fmt.Errorf("%w: zone: %v", ErrMalformed, err)
	}
	return ZoneRecord{
		UGC: ugc, Type: raw.Properties.Type, Name: raw.Properties.Name, State: raw.Properties.State,
		Geometry: raw.Geometry,
	}, nil
}

// ListZones implements ZoneProvider: GET /zones/{zoneType}?area={state}&include_geometry=false.
func (p *NWSProvider) ListZones(ctx context.Context, zoneType, state string) ([]ZoneRef, error) {
	v := url.Values{}
	v.Set("area", strings.ToUpper(state))
	v.Set("include_geometry", "false")
	req, err := p.newRequest(ctx, "/zones/"+zoneType, v)
	if err != nil {
		return nil, err
	}
	body, err := p.do(req)
	if err != nil {
		return nil, err
	}
	var coll struct {
		Features []struct {
			Properties struct {
				ID    string `json:"id"`
				Type  string `json:"type"`
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &coll); err != nil {
		return nil, fmt.Errorf("%w: zone list: %v", ErrMalformed, err)
	}
	out := make([]ZoneRef, 0, len(coll.Features))
	for _, f := range coll.Features {
		out = append(out, ZoneRef{UGC: f.Properties.ID, Name: f.Properties.Name, State: f.Properties.State, Type: f.Properties.Type})
	}
	return out, nil
}

// ResolvePoint implements GET /points/{lat},{lon}.
func (p *NWSProvider) ResolvePoint(ctx context.Context, pt LatLon) (PointZones, error) {
	path := fmt.Sprintf("/points/%g,%g", pt.Lat, pt.Lon)
	req, err := p.newRequest(ctx, path, nil)
	if err != nil {
		return PointZones{}, err
	}
	body, err := p.do(req)
	if err != nil {
		return PointZones{}, err
	}
	var raw struct {
		Properties struct {
			CWA             string `json:"cwa"`
			ForecastZone    string `json:"forecastZone"`
			County          string `json:"county"`
			FireWeatherZone string `json:"fireWeatherZone"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return PointZones{}, fmt.Errorf("%w: points: %v", ErrMalformed, err)
	}
	return PointZones{
		CWA: raw.Properties.CWA, ResolvedAt: time.Now().UTC(),
		Forecast: UGCFromZoneURL(raw.Properties.ForecastZone),
		County:   UGCFromZoneURL(raw.Properties.County),
		Fire:     UGCFromZoneURL(raw.Properties.FireWeatherZone),
	}, nil
}
