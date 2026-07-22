package app

import (
	"fmt"
	"log"
	"net"
)

// DesktopAddr converts the configured server.listen value into the TCP bind
// address desktop mode uses. Rules:
//   - ":<port>" (empty host, the headless default form) → "127.0.0.1:<port>"
//     (desktop is loopback-only by default)
//   - any explicit host — including "0.0.0.0", "[::]", "localhost", or a
//     concrete IP — is honored verbatim (LAN opt-in via config)
//   - a value with no port (no colon, or empty port) → error
//
// Implemented with net.SplitHostPort; returns a descriptive error on
// malformed input.
func DesktopAddr(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("desktop listen address %q: %w", listen, err)
	}
	if port == "" {
		return "", fmt.Errorf("desktop listen address %q: missing port", listen)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}

// ListenWithFallback opens a TCP listener on addr. If that exact port is
// unavailable (in use / access denied), it falls back to an OS-assigned free
// port on the same host ("host:0") and logs the substitution with
// log.Printf("desktop: port %s busy, using %s instead", ...). Any other
// error (bad host, etc.) is returned as-is.
func ListenWithFallback(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}

	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		return nil, err
	}
	if port == "0" {
		return nil, err
	}

	fallbackAddr := net.JoinHostPort(host, "0")
	fallback, fallbackErr := net.Listen("tcp", fallbackAddr)
	if fallbackErr != nil {
		return nil, err
	}
	log.Printf("desktop: port %s busy, using %s instead", addr, fallback.Addr().String())
	return fallback, nil
}

// LocalURL returns the http:// URL the desktop webview loads for a listener
// bound at addr (pass ln.Addr().String()). Wildcard hosts are mapped to
// loopback so the webview can actually connect:
//
//	"0.0.0.0:9090"  → "http://127.0.0.1:9090"
//	"[::]:9090"     → "http://127.0.0.1:9090"
//	"::" host       → "http://127.0.0.1:<port>"
//	anything else   → "http://<host>:<port>" (IPv6 hosts re-bracketed via
//	                  net.JoinHostPort)
func LocalURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	switch host {
	case "0.0.0.0", "::", "":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
