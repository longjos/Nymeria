package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/narvel/nymeria/internal/geocode/w3w"
)

// w3wClient returns the current what3words client (nil when the feature was
// never wired at startup, i.e. what3words.enabled was false in the boot
// config).
func (s *Server) w3wClient() *w3w.Client {
	s.w3wMu.RLock()
	defer s.w3wMu.RUnlock()
	return s.w3w
}

// w3wSuggestDefault returns the configured autosuggest row count (0 when no
// config manager is wired, which the w3w client itself defaults to 3).
func (s *Server) w3wSuggestDefault() int {
	if s.configMgr == nil {
		return 0
	}
	return s.configMgr.Get().What3Words.Results
}

// handleW3WStatus always returns 200 — configured/enabled state, never an
// error — so the frontend can distinguish "off" from "server error" and
// poll unconditionally.
func (s *Server) handleW3WStatus(w http.ResponseWriter, _ *http.Request) {
	c := s.w3wClient()
	writeJSON(w, http.StatusOK, map[string]bool{
		"configured": c != nil && c.Configured(),
		"enabled":    c != nil && c.Enabled(),
	})
}

// handleW3WResolve converts a full three-word address to coordinates.
func (s *Server) handleW3WResolve(w http.ResponseWriter, r *http.Request) {
	c := s.w3wClient()
	if c == nil || !c.Configured() {
		writeW3WNotConfigured(w)
		return
	}

	words := r.URL.Query().Get("words")
	res, err := c.Resolve(r.Context(), words)
	if err != nil {
		s.logW3WFailure("resolve", err)
		writeW3WError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleW3WSuggest returns autosuggest candidates for a partial input,
// optionally focused on a map point.
func (s *Server) handleW3WSuggest(w http.ResponseWriter, r *http.Request) {
	c := s.w3wClient()
	if c == nil || !c.Configured() {
		writeW3WNotConfigured(w)
		return
	}

	input := r.URL.Query().Get("input")
	if strings.TrimSpace(input) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required", "code": "bad_request"})
		return
	}

	var focusLat, focusLon *float64
	if latStr, lonStr := r.URL.Query().Get("lat"), r.URL.Query().Get("lon"); latStr != "" && lonStr != "" {
		if lat, errLat := strconv.ParseFloat(latStr, 64); errLat == nil {
			if lon, errLon := strconv.ParseFloat(lonStr, 64); errLon == nil {
				focusLat, focusLon = &lat, &lon
			}
		}
	}

	n := 0
	if nStr := r.URL.Query().Get("n"); nStr != "" {
		if v, err := strconv.Atoi(nStr); err == nil {
			n = v
		}
	}
	if n <= 0 {
		n = s.w3wSuggestDefault()
	}

	suggestions, err := c.Suggest(r.Context(), input, focusLat, focusLon, n)
	if err != nil {
		s.logW3WFailure("suggest", err)
		writeW3WError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}

// handleW3WReverse converts coordinates to the three-word address of that
// square. lat/lon are validated locally (no upstream call) before Reverse.
func (s *Server) handleW3WReverse(w http.ResponseWriter, r *http.Request) {
	c := s.w3wClient()
	if c == nil || !c.Configured() {
		writeW3WNotConfigured(w)
		return
	}

	lat, errLat := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, errLon := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if errLat != nil || errLon != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat/lon must be valid coordinates", "code": "bad_request"})
		return
	}

	res, err := c.Reverse(r.Context(), lat, lon)
	if err != nil {
		s.logW3WFailure("reverse", err)
		writeW3WError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func writeW3WNotConfigured(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"error": "what3words is not configured", "code": "not_configured",
	})
}

func writeW3WError(w http.ResponseWriter, err error) {
	status, code, msg := w3wErrorResponse(err)
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// w3wErrorResponse maps a w3w package error to the HTTP status + error code
// the frontend contract expects. ErrInvalidKey maps to 502, never 401 — a
// 401 from /api/w3w/* would trip the frontend's session handling and log
// the NCS out mid-incident.
func w3wErrorResponse(err error) (status int, code string, message string) {
	switch {
	case errors.Is(err, w3w.ErrNotConfigured):
		return http.StatusServiceUnavailable, "not_configured", "what3words is not configured"
	case errors.Is(err, w3w.ErrBadWords):
		return http.StatusBadRequest, "bad_words", "That is not a valid three-word address"
	case errors.Is(err, w3w.ErrInvalidKey):
		return http.StatusBadGateway, "invalid_key", "what3words rejected the API key"
	case errors.Is(err, w3w.ErrQuotaExceeded):
		return http.StatusTooManyRequests, "quota_exceeded", "what3words quota exceeded"
	case errors.Is(err, w3w.ErrRateLimited):
		return http.StatusTooManyRequests, "rate_limited", "what3words rate limited the request"
	case errors.Is(err, w3w.ErrNetwork):
		return http.StatusGatewayTimeout, "timeout", "could not reach what3words"
	default:
		return http.StatusBadGateway, "upstream", "what3words upstream error"
	}
}

// logW3WFailure logs a real upstream/network failure at warn, including the
// mapped upstream code — never the API key, and never the full upstream URL
// (err.Error() on a transport failure can embed it). Local validation
// failures (bad words, not configured) are not upstream failures and are
// not logged.
func (s *Server) logW3WFailure(op string, err error) {
	if errors.Is(err, w3w.ErrBadWords) || errors.Is(err, w3w.ErrNotConfigured) {
		return
	}
	code := "unknown"
	var apiErr *w3w.APIError
	switch {
	case errors.As(err, &apiErr):
		code = apiErr.Code
		if code == "" {
			code = "unparseable_body"
		}
	case errors.Is(err, w3w.ErrNetwork):
		code = "network"
	}
	slog.Warn("what3words upstream failure", "op", op, "code", code)
}
