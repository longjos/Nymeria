// Package w3w is a thin, cached client for the public what3words v3 API
// (https://developer.what3words.com/public-api). It has no offline
// dataset — every uncached lookup is a real HTTP call — so the package
// leans hard on client-side validation (words.go) and TTL caching
// (cache.go) to keep quota usage low.
package w3w

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the public what3words v3 API root (no trailing slash).
const DefaultBaseURL = "https://api.what3words.com/v3"

// Config configures a Client. Zero values take sensible defaults — see New.
type Config struct {
	APIKey     string
	BaseURL    string       // "" -> DefaultBaseURL
	HTTPClient *http.Client // nil -> &http.Client{Timeout: 8 * time.Second}

	// Cache TTLs. Zero values take the defaults below.
	ForwardTTL time.Duration // words->coords and coords->words. Default 24h.
	SuggestTTL time.Duration // autosuggest. Default 5m.
	CacheMax   int           // entries per cache before eviction. Default 512.

	Now func() time.Time // test clock. nil -> time.Now
}

// Client is a what3words v3 API client with an in-memory TTL cache. Safe
// for concurrent use.
type Client struct {
	keyMu   sync.RWMutex
	apiKey  string
	enabled bool // guarded by keyMu; defaults true — see New.
	baseURL string
	hc      *http.Client
	fwd     *ttlCache[Result]       // key: normalized words
	rev     *ttlCache[Result]       // key: "lat,lon" at 6 decimals
	sug     *ttlCache[[]Suggestion] // key: "input|flat,flon|n"
}

// Result is one resolved square. Lon (not Lng) to match the rest of Nymeria.
type Result struct {
	Words        string  `json:"words"` // "filled.count.soap" — no slashes
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	NearestPlace string  `json:"nearestPlace"`
	Country      string  `json:"country"`
	Language     string  `json:"language"`
}

// Suggestion is one what3words autosuggest candidate.
type Suggestion struct {
	Words             string  `json:"words"`
	NearestPlace      string  `json:"nearestPlace"`
	Country           string  `json:"country"`
	DistanceToFocusKm float64 `json:"distanceToFocusKm"` // 0 when no focus given
	Rank              int     `json:"rank"`
}

// New creates a Client. It returns an error only when BaseURL is malformed;
// an empty APIKey is valid (Configured() reports false and every method
// returns ErrNotConfigured on a cache miss), which is what makes an API key
// entered later through Settings live-effective with no restart.
func New(cfg Config) (*Client, error) {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("w3w: invalid base URL %q: %w", base, err)
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 8 * time.Second}
	}
	fwdTTL := cfg.ForwardTTL
	if fwdTTL == 0 {
		fwdTTL = 24 * time.Hour
	}
	sugTTL := cfg.SuggestTTL
	if sugTTL == 0 {
		sugTTL = 5 * time.Minute
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &Client{
		apiKey:  cfg.APIKey,
		enabled: true, // live-toggled later via SetEnabled; see Configured.
		baseURL: base,
		hc:      hc,
		fwd:     newTTLCache[Result](fwdTTL, cfg.CacheMax, now),
		rev:     newTTLCache[Result](fwdTTL, cfg.CacheMax, now),
		sug:     newTTLCache[[]Suggestion](sugTTL, cfg.CacheMax, now),
	}, nil
}

// Configured reports whether the client is live: enabled AND an API key is
// set. Both what3words.enabled and the API key are live-toggleable via
// SetEnabled/SetAPIKey with no server restart, so this must reflect the
// current state on every call, not just the state at construction.
func (c *Client) Configured() bool {
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()
	return c.enabled && c.apiKey != ""
}

// Enabled reports the live what3words.enabled state (independent of
// whether a key is present) — the "off" escape hatch an org can flip in
// Settings even when a key is configured.
func (c *Client) Enabled() bool {
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()
	return c.enabled
}

// SetEnabled live-toggles the what3words.enabled flag. A Settings save
// that flips it takes effect immediately: Configured() (and therefore
// every proxy handler's guard) reflects it on the very next call, no
// restart required in either direction.
func (c *Client) SetEnabled(enabled bool) {
	c.keyMu.Lock()
	c.enabled = enabled
	c.keyMu.Unlock()
}

// APIKey returns the current API key.
func (c *Client) APIKey() string {
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()
	return c.apiKey
}

// SetAPIKey swaps the API key and flushes only the autosuggest cache.
// Forward/reverse results are key-independent facts about the world and are
// left alone; autosuggest results are not strictly key-dependent either, but
// flushing them is cheap insurance and keeps rotation simple to reason about.
func (c *Client) SetAPIKey(key string) {
	c.keyMu.Lock()
	c.apiKey = key
	c.keyMu.Unlock()
	c.sug.Clear()
}

// Resolve converts a full three-word address to coordinates. words may
// carry a "///" or "/" prefix and any case; it is normalized first.
// Returns ErrBadWords (wrapped), without any HTTP call, if the address is
// not a syntactically valid full 3wa — this is the quota guard.
func (c *Client) Resolve(ctx context.Context, words string) (Result, error) {
	if !IsFullAddress(words) {
		return Result{}, fmt.Errorf("%w: %q", ErrBadWords, words)
	}
	key := Normalize(words)
	if v, ok := c.fwd.Get(key); ok {
		return v, nil
	}
	if !c.Configured() {
		return Result{}, ErrNotConfigured
	}

	q := url.Values{}
	q.Set("words", key)
	q.Set("format", "json")

	var body forwardBody
	if err := c.get(ctx, "/convert-to-coordinates", q, &body); err != nil {
		return Result{}, err
	}
	res := body.toResult()
	c.fwd.Put(key, res)
	return res, nil
}

// Suggest returns up to n autosuggest candidates for a partial input.
// focusLat/focusLon steer results toward the operator's area; pass nil for
// no focus. n is clamped to 1..10 (default 3 when <= 0).
func (c *Client) Suggest(ctx context.Context, input string, focusLat, focusLon *float64, n int) ([]Suggestion, error) {
	switch {
	case n <= 0:
		n = 3
	case n > 10:
		n = 10
	}

	key := suggestCacheKey(input, focusLat, focusLon, n)
	if v, ok := c.sug.Get(key); ok {
		return v, nil
	}
	if !c.Configured() {
		return nil, ErrNotConfigured
	}

	q := url.Values{}
	q.Set("input", input)
	q.Set("n-results", strconv.Itoa(n))
	if focusLat != nil && focusLon != nil {
		q.Set("focus", fmtCoord(*focusLat)+","+fmtCoord(*focusLon))
	}
	q.Set("format", "json")

	var body struct {
		Suggestions []struct {
			Words             string  `json:"words"`
			NearestPlace      string  `json:"nearestPlace"`
			Country           string  `json:"country"`
			DistanceToFocusKm float64 `json:"distanceToFocusKm"`
			Rank              int     `json:"rank"`
		} `json:"suggestions"`
	}
	if err := c.get(ctx, "/autosuggest", q, &body); err != nil {
		return nil, err
	}

	out := make([]Suggestion, 0, len(body.Suggestions))
	for _, s := range body.Suggestions {
		out = append(out, Suggestion{
			Words:             s.Words,
			NearestPlace:      s.NearestPlace,
			Country:           s.Country,
			DistanceToFocusKm: s.DistanceToFocusKm,
			Rank:              s.Rank,
		})
	}
	c.sug.Put(key, out)
	return out, nil
}

// Reverse converts coordinates to the three-word address of that square.
func (c *Client) Reverse(ctx context.Context, lat, lon float64) (Result, error) {
	key := fmtCoord(lat) + "," + fmtCoord(lon)
	if v, ok := c.rev.Get(key); ok {
		return v, nil
	}
	if !c.Configured() {
		return Result{}, ErrNotConfigured
	}

	q := url.Values{}
	q.Set("coordinates", key)
	q.Set("format", "json")

	var body forwardBody
	if err := c.get(ctx, "/convert-to-3wa", q, &body); err != nil {
		return Result{}, err
	}
	res := body.toResult()
	c.rev.Put(key, res)
	return res, nil
}

// forwardBody is the shared upstream shape for convert-to-coordinates and
// convert-to-3wa.
type forwardBody struct {
	Coordinates struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"coordinates"`
	Words        string `json:"words"`
	NearestPlace string `json:"nearestPlace"`
	Country      string `json:"country"`
	Language     string `json:"language"`
}

func (b forwardBody) toResult() Result {
	return Result{
		Words:        b.Words,
		Lat:          b.Coordinates.Lat,
		Lon:          b.Coordinates.Lng,
		NearestPlace: b.NearestPlace,
		Country:      b.Country,
		Language:     b.Language,
	}
}

// fmtCoord formats a coordinate with 6 decimal places so cache keys and
// upstream requests agree bit-for-bit.
func fmtCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func suggestCacheKey(input string, focusLat, focusLon *float64, n int) string {
	focus := ""
	if focusLat != nil && focusLon != nil {
		focus = fmtCoord(*focusLat) + "," + fmtCoord(*focusLon)
	}
	return input + "|" + focus + "|" + strconv.Itoa(n)
}

// get issues a GET request against the what3words API and decodes a
// successful response into out. The key is sent via the X-Api-Key header —
// never as a "key=" query param — so it can never end up in an upstream
// access log or a redirect Location header.
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	reqURL := c.baseURL + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	req.Header.Set("X-Api-Key", c.APIKey())

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: read body: %v", ErrNetwork, err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("%w: decode: %v", ErrUpstream, err)
		}
		return nil
	}

	var eb struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	code := ""
	msg := string(body)
	if json.Unmarshal(body, &eb) == nil && eb.Error.Code != "" {
		code = eb.Error.Code
		msg = eb.Error.Message
	}
	return &APIError{
		Code:       code,
		Message:    msg,
		HTTPStatus: resp.StatusCode,
		sentinel:   mapUpstreamCode(code, resp.StatusCode),
	}
}
