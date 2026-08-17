package aprs

import (
	"fmt"
	"strings"
)

// DefaultRFPath returns a fresh copy of the standard New-N RF path (WIDE1-1,WIDE2-1).
func DefaultRFPath() []Address {
	return []Address{{Call: "WIDE1", SSID: 1}, {Call: "WIDE2", SSID: 1}}
}

// ParsePath parses a TNC2 digipeater path such as "WIDE1-1,WIDE2-1" or "TCPIP*".
// Empty and whitespace-only strings yield a zero-length path (direct / simplex).
// Spaces around commas are ignored.
func ParsePath(s string) ([]Address, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []Address{}, nil
	}

	parts := strings.Split(s, ",")
	path := make([]Address, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		addr, err := parseAddress(p)
		if err != nil {
			return nil, fmt.Errorf("invalid path address %q: %w", p, err)
		}
		addr.Call = strings.ToUpper(addr.Call)
		if err := validateAddress(addr); err != nil {
			return nil, fmt.Errorf("invalid path address %q: %w", p, err)
		}
		if err := validateOutboundHop(addr); err != nil {
			return nil, err
		}
		path = append(path, addr)
	}
	if len(path) > ax25MaxDigis {
		return nil, fmt.Errorf("too many digipeaters: %d (max %d)", len(path), ax25MaxDigis)
	}
	return path, nil
}

// validateOutboundHop enforces the New-N paradigm for paths we transmit.
// Inbound parsing still accepts historical aliases via parseAddress.
func validateOutboundHop(addr Address) error {
	call := addr.Call
	switch call {
	case "RELAY", "WIDE", "TRACE":
		return fmt.Errorf("%s is deprecated; use WIDEn-N (WIDE1-1,WIDE2-1)", call)
	case "TCPIP", "RFONLY", "NOGATE", "TCPXX":
		return nil
	}
	if strings.HasPrefix(call, "TRACE") {
		return fmt.Errorf("TRACE paths are deprecated; use WIDEn-N")
	}
	if len(call) == 5 && strings.HasPrefix(call, "WIDE") {
		n := call[4]
		if n < '1' || n > '9' {
			return fmt.Errorf("invalid WIDEn alias %s", call)
		}
		if n > '2' {
			return fmt.Errorf("%s exceeds the New-N maximum of WIDE2", call)
		}
		if !addr.HBit && addr.SSID < 1 {
			return fmt.Errorf("%s-0 is not a usable unused path; use %s-1 or %s-2", call, call, call)
		}
	}
	return nil
}

// FormatPath renders a path as a TNC2 string (e.g. "WIDE1-1,WIDE2-1" or "TCPIP*").
func FormatPath(path []Address) string {
	if len(path) == 0 {
		return ""
	}
	parts := make([]string, len(path))
	for i, a := range path {
		parts[i] = a.String()
	}
	return strings.Join(parts, ",")
}
