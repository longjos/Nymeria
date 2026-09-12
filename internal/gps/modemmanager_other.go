//go:build !linux

package gps

import "context"

// defaultMMDialer on non-Linux platforms: D-Bus/ModemManager is Linux-only.
// The error is surfaced through SourceStatus.Error (the GPS pill and
// Settings render it) and runLoop retries it harmlessly on backoff.
func defaultMMDialer(context.Context) (mmConn, error) { return nil, ErrUnsupportedPlatform }
