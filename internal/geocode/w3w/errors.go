package w3w

import (
	"errors"
	"fmt"
)

// Sentinel errors. Callers use errors.Is against these — APIError wraps one
// of them so the caller never needs to know the upstream error.code taxonomy.
var (
	ErrNotConfigured = errors.New("what3words: no API key configured")
	ErrBadWords      = errors.New("what3words: not a valid three-word address")
	ErrInvalidKey    = errors.New("what3words: API key rejected")
	ErrQuotaExceeded = errors.New("what3words: API quota exceeded")
	ErrRateLimited   = errors.New("what3words: rate limited")
	ErrUpstream      = errors.New("what3words: upstream error")
	ErrNetwork       = errors.New("what3words: network error")
)

// APIError carries the upstream {"error":{"code","message"}} body verbatim
// for logs; it wraps one of the sentinels above so callers use errors.Is.
type APIError struct {
	Code       string // upstream code, e.g. "BadWords" ("" when the body was unparseable)
	Message    string // upstream message
	HTTPStatus int
	sentinel   error
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("what3words: upstream error (HTTP %d): %s", e.HTTPStatus, e.Message)
	}
	return fmt.Sprintf("what3words: %s (HTTP %d): %s", e.Code, e.HTTPStatus, e.Message)
}

func (e *APIError) Unwrap() error { return e.sentinel }

// mapUpstreamCode maps a what3words error.code to a sentinel. This is
// checked before HTTP status because what3words returns 400 for several
// distinct conditions (bad words vs. a rejected key, for example).
func mapUpstreamCode(code string, httpStatus int) error {
	switch code {
	case "BadWords", "BadInput", "BadCoordinates", "BadNResults", "BadFocus":
		return ErrBadWords
	case "InvalidKey", "MissingKey", "SuspendedKey", "InvalidApiVersion":
		return ErrInvalidKey
	case "QuotaExceeded":
		return ErrQuotaExceeded
	case "TooManyRequests":
		return ErrRateLimited
	default:
		if httpStatus == 429 {
			return ErrRateLimited
		}
		return ErrUpstream
	}
}
