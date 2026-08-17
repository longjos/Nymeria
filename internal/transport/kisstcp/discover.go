package kisstcp

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

const (
	// DefaultKISSPort is Direwolf's KISSPORT default.
	DefaultKISSPort = 8001

	sourceMDNS  = "mdns"
	sourceLocal = "local"

	kissServiceType = "_kiss-tnc._tcp"
	kissServiceDom  = "local."
)

// TNCInfo is one discovered KISS TCP endpoint for Settings.
type TNCInfo struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Source    string `json:"source"`
	Local     bool   `json:"local,omitempty"`
	Highlight bool   `json:"highlight,omitempty"`
	PortsNote string `json:"portsNote,omitempty"`
}

type rawTNC struct {
	Instance  string
	Host      string
	Port      int
	PortsNote string
}

type mdnsBrowser func() ([]rawTNC, error)
type localProber func() bool

var browseMDNS mdnsBrowser = browseMDNSDefault
var probeLocal localProber = probeLocalDefault
var discoverOverride func() ([]TNCInfo, error)

var (
	mdnsBrowseTimeout = 1500 * time.Millisecond
	localProbeTimeout = 200 * time.Millisecond
	localProbeAddr    = fmt.Sprintf("127.0.0.1:%d", DefaultKISSPort)
)

// Discover lists KISS TCP endpoints: localhost:8001 if something is listening,
// plus LAN Direwolf instances advertised as _kiss-tnc._tcp.
func Discover() ([]TNCInfo, error) {
	if discoverOverride != nil {
		return discoverOverride()
	}
	raw, browseErr := browseMDNS()
	local := probeLocal()
	out := mergeDiscover(raw, local)
	if browseErr != nil {
		return out, browseErr
	}
	return out, nil
}

// SetDiscoverForTest replaces Discover for server tests. Restore with the returned func.
func SetDiscoverForTest(fn func() ([]TNCInfo, error)) func() {
	prev := discoverOverride
	discoverOverride = fn
	return func() { discoverOverride = prev }
}

func mergeDiscover(raw []rawTNC, localOK bool) []TNCInfo {
	out := make([]TNCInfo, 0, len(raw)+1)
	seen := map[string]bool{}

	if localOK {
		info := localTNC()
		out = append(out, info)
		seen[addrKey(info.Host, info.Port)] = true
	}

	for _, r := range raw {
		host := strings.TrimSpace(r.Host)
		if host == "" {
			continue
		}
		port := r.Port
		if port == 0 {
			port = DefaultKISSPort
		}
		if seen[addrKey(host, port)] {
			continue
		}
		name := strings.TrimSpace(r.Instance)
		if name == "" {
			name = fmt.Sprintf("%s:%d", host, port)
		}
		info := TNCInfo{
			Name:      name,
			Host:      host,
			Port:      port,
			Source:    sourceMDNS,
			Highlight: true,
			PortsNote: strings.TrimSpace(r.PortsNote),
		}
		info.Label = tncLabel(info)
		out = append(out, info)
		seen[addrKey(host, port)] = true
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Local != out[j].Local {
			return out[i].Local
		}
		return out[i].Name < out[j].Name
	})
	if out == nil {
		return []TNCInfo{}
	}
	return out
}

func localTNC() TNCInfo {
	info := TNCInfo{
		Name:      "This computer",
		Host:      "localhost",
		Port:      DefaultKISSPort,
		Source:    sourceLocal,
		Local:     true,
		Highlight: true,
	}
	info.Label = "This computer (localhost:8001)"
	return info
}

func tncLabel(t TNCInfo) string {
	base := fmt.Sprintf("%s — %s:%d", t.Name, t.Host, t.Port)
	if t.PortsNote != "" {
		return base + " (" + t.PortsNote + ")"
	}
	return base
}

func normalizeKISSHost(host string) string {
	h := strings.TrimSpace(host)
	h = strings.TrimSuffix(h, ".")
	h = strings.ToLower(h)
	switch h {
	case "127.0.0.1", "::1", "[::1]", "localhost":
		return "localhost"
	}
	return h
}

func addrKey(host string, port int) string {
	if port == 0 {
		port = DefaultKISSPort
	}
	return normalizeKISSHost(host) + ":" + itoa(port)
}

func sameKISSAddr(aHost string, aPort int, bHost string, bPort int) bool {
	return addrKey(aHost, aPort) == addrKey(bHost, bPort)
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func probeLocalDefault() bool {
	conn, err := net.DialTimeout("tcp", localProbeAddr, localProbeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
