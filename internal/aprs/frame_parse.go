package aprs

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseFrame parses a TNC2-format raw string into an APRSFrame.
// Format: SOURCE>DESTINATION,PATH1,PATH2:payload
func ParseFrame(raw string) (APRSFrame, error) {
	if raw == "" {
		return APRSFrame{}, fmt.Errorf("empty frame")
	}

	// Split header:payload on first colon
	colonIdx := strings.IndexByte(raw, ':')
	if colonIdx < 0 {
		return APRSFrame{}, fmt.Errorf("no colon separator in frame")
	}
	header := raw[:colonIdx]
	payload := raw[colonIdx+1:]

	// Split source>destination,path
	gtIdx := strings.IndexByte(header, '>')
	if gtIdx < 0 {
		return APRSFrame{}, fmt.Errorf("no > separator in frame header")
	}

	srcStr := header[:gtIdx]
	rest := header[gtIdx+1:]

	src, err := parseAddress(srcStr)
	if err != nil {
		return APRSFrame{}, fmt.Errorf("invalid source address: %w", err)
	}

	// Split destination and path on commas
	parts := strings.Split(rest, ",")
	if len(parts) == 0 || parts[0] == "" {
		return APRSFrame{}, fmt.Errorf("missing destination")
	}

	dst, err := parseAddress(parts[0])
	if err != nil {
		return APRSFrame{}, fmt.Errorf("invalid destination address: %w", err)
	}

	var path []Address
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		addr, err := parseAddress(p)
		if err != nil {
			return APRSFrame{}, fmt.Errorf("invalid path address %q: %w", p, err)
		}
		path = append(path, addr)
	}

	return APRSFrame{
		Source:      src,
		Destination: dst,
		Path:        path,
		Payload:     payload,
	}, nil
}

// parseAddress parses "CALL" or "CALL-SSID" into an Address.
func parseAddress(s string) (Address, error) {
	if s == "" {
		return Address{}, fmt.Errorf("empty address")
	}
	dashIdx := strings.LastIndexByte(s, '-')
	if dashIdx < 0 {
		return Address{Call: s}, nil
	}
	call := s[:dashIdx]
	ssidStr := s[dashIdx+1:]
	ssid, err := strconv.Atoi(ssidStr)
	if err != nil {
		return Address{}, fmt.Errorf("invalid SSID %q: %w", ssidStr, err)
	}
	return Address{Call: call, SSID: ssid}, nil
}
