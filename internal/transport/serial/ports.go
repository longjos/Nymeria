package serial

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	profileGeneric    = "generic"
	profileKenwoodUSB = "kenwood-thd7x-usb"
	profileKenwoodBT  = "kenwood-thd7x-bt"
	profileMobilinkd  = "mobilinkd"
)

// StandardBaudRates is the baud select catalog returned by GET /api/serial-ports.
var StandardBaudRates = []int{1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200}

// PortInfo is one enumerated serial port as shown in Settings.
type PortInfo struct {
	Name             string `json:"name"`
	Label            string `json:"label"`
	Present          bool   `json:"present"`
	IsUSB            bool   `json:"isUSB"`
	VID              string `json:"vid,omitempty"`
	PID              string `json:"pid,omitempty"`
	SerialNumber     string `json:"serialNumber,omitempty"`
	Product          string `json:"product,omitempty"`
	StablePath       string `json:"stablePath,omitempty"`
	SuggestedProfile string `json:"suggestedProfile,omitempty"`
	Highlight        bool   `json:"highlight,omitempty"`
}

// Profile is a TNC/radio preset. IDs are not persisted; they only fill baud + help.
type Profile struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Baud  int    `json:"baud"`
	Help  string `json:"help"`
}

type rawPort struct {
	Name, VID, PID, SerialNumber, Product string
	IsUSB                                 bool
}

type rawLister func() ([]rawPort, error)

var listRaw rawLister = listRawDefault

var listPortsOverride func() ([]PortInfo, error)

// sysfsRoot and serialByIDDir are overridable so tests can use a temp tree on any GOOS.
var sysfsRoot = "/sys"
var serialByIDDir = "/dev/serial/by-id"

// ListPorts returns the host's serial ports with friendly labels and profile hints.
// On error the caller should treat the list as empty; this function does not open ports.
func ListPorts() ([]PortInfo, error) {
	if listPortsOverride != nil {
		return listPortsOverride()
	}
	raw, err := listRaw()
	if err != nil {
		return nil, err
	}
	return listFromRaw(raw), nil
}

// SetListerForTest replaces ListPorts for server tests. The returned func restores the previous hook.
func SetListerForTest(fn func() ([]PortInfo, error)) func() {
	prev := listPortsOverride
	listPortsOverride = fn
	return func() { listPortsOverride = prev }
}

// persistDevice is the path written to TransportConfig.Device.
func persistDevice(p PortInfo) string {
	if p.StablePath != "" {
		return p.StablePath
	}
	return p.Name
}

func listFromRaw(raw []rawPort) []PortInfo {
	filtered := make([]rawPort, 0, len(raw))
	for _, r := range raw {
		if strings.TrimSpace(r.Name) == "" {
			continue
		}
		if isMotherboardUART(r.Name) {
			continue
		}
		r.VID = normalizeHexID(r.VID)
		r.PID = normalizeHexID(r.PID)
		if r.Product == "" {
			r.Product = enrichLinuxProduct(r.Name)
		}
		filtered = append(filtered, r)
	}
	filtered = dedupeDarwinCuOverTty(filtered)

	out := make([]PortInfo, 0, len(filtered))
	for _, r := range filtered {
		info := PortInfo{
			Name:         r.Name,
			Present:      true,
			IsUSB:        r.IsUSB,
			VID:          r.VID,
			PID:          r.PID,
			SerialNumber: r.SerialNumber,
			Product:      r.Product,
			StablePath:   resolveLinuxByID(r.Name),
		}
		info.SuggestedProfile = matchProfile(r)
		if info.SuggestedProfile == profileKenwoodUSB || info.SuggestedProfile == profileMobilinkd {
			info.Highlight = true
		}
		info.Label = portLabel(info)
		out = append(out, info)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Highlight != out[j].Highlight {
			return out[i].Highlight
		}
		if out[i].IsUSB != out[j].IsUSB {
			return out[i].IsUSB
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func isMotherboardUART(name string) bool {
	base := filepath.Base(name)
	if strings.HasPrefix(base, "ttyS") {
		rest := strings.TrimPrefix(base, "ttyS")
		return rest != "" && isAllDigits(rest)
	}
	if strings.HasPrefix(base, "ttyHS") {
		rest := strings.TrimPrefix(base, "ttyHS")
		return rest != "" && isAllDigits(rest)
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func normalizeHexID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	return strings.ToUpper(s)
}

func matchProfile(r rawPort) string {
	prod := strings.ToLower(r.Product)
	if strings.Contains(prod, "th-d74") || strings.Contains(prod, "th-d75") {
		return profileKenwoodUSB
	}
	if strings.Contains(prod, "mobilinkd") {
		return profileMobilinkd
	}
	id := r.VID + ":" + r.PID
	switch id {
	case "2166:600B", "2166:9023":
		return profileKenwoodUSB
	case "1D50:6018":
		return profileMobilinkd
	}
	return ""
}

func portLabel(p PortInfo) string {
	friendly := p.Product
	switch p.SuggestedProfile {
	case profileKenwoodUSB:
		friendly = "Kenwood TH-D74 / TH-D75"
	case profileMobilinkd:
		friendly = "Mobilinkd TNC"
	}

	// Windows-style Product that already includes the COM name.
	if p.Product != "" && containsFold(p.Product, p.Name) {
		return p.Product
	}

	path := p.Name
	if p.StablePath != "" {
		path = p.StablePath
	}

	// Linux: friendly — path (tty)
	if strings.HasPrefix(p.Name, "/dev/") && !strings.HasPrefix(filepath.Base(p.Name), "cu.") && !strings.HasPrefix(filepath.Base(p.Name), "tty.") {
		tty := filepath.Base(p.Name)
		if p.SuggestedProfile != "" || p.Product != "" {
			if p.StablePath != "" {
				return friendly + " — " + p.StablePath + " (" + tty + ")"
			}
			return friendly + " — " + p.Name
		}
		if p.VID != "" && p.PID != "" {
			return "USB " + p.VID + ":" + p.PID + " — " + path
		}
		return p.Name
	}

	if p.Product != "" {
		return p.Product + " (" + p.Name + ")"
	}
	return p.Name
}

func containsFold(hay, needle string) bool {
	return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}

func dedupeDarwinCuOverTty(raw []rawPort) []rawPort {
	cuSuffix := map[string]bool{}
	for _, r := range raw {
		base := filepath.Base(r.Name)
		if strings.HasPrefix(base, "cu.") {
			cuSuffix[strings.TrimPrefix(base, "cu.")] = true
		}
	}
	if len(cuSuffix) == 0 {
		return raw
	}
	out := make([]rawPort, 0, len(raw))
	for _, r := range raw {
		base := filepath.Base(r.Name)
		if strings.HasPrefix(base, "tty.") && cuSuffix[strings.TrimPrefix(base, "tty.")] {
			continue
		}
		out = append(out, r)
	}
	return out
}

func enrichLinuxProduct(name string) string {
	base := filepath.Base(name)
	if !strings.HasPrefix(base, "tty") && !strings.HasPrefix(base, "rfcomm") {
		return ""
	}
	start := filepath.Join(sysfsRoot, "class", "tty", base, "device")
	resolved, err := filepath.EvalSymlinks(start)
	if err != nil {
		// Walk the unresolved path anyway (tests may use a direct dir).
		resolved = start
	}
	dir := resolved
	for i := 0; i < 8; i++ {
		b, err := os.ReadFile(filepath.Join(dir, "product"))
		if err == nil {
			return strings.TrimSpace(string(b))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func resolveLinuxByID(name string) string {
	if serialByIDDir == "" {
		return ""
	}
	entries, err := os.ReadDir(serialByIDDir)
	if err != nil {
		return ""
	}
	want := filepath.Base(name)
	var matches []string
	for _, e := range entries {
		p := filepath.Join(serialByIDDir, e.Name())
		target, err := os.Readlink(p)
		if err != nil {
			continue
		}
		if filepath.Base(target) == want {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return filepath.Join(serialByIDDir, matches[0])
}

// Profiles returns the TNC/radio preset catalog.
func Profiles() []Profile {
	return []Profile{
		{
			ID:    profileGeneric,
			Label: "Generic KISS TNC",
			Baud:  9600,
			Help:  "Hardware TNC or unknown USB-UART. Put the TNC in KISS. Baud must match the TNC serial rate (often 9600 or 115200).",
		},
		{
			ID:    profileKenwoodUSB,
			Label: "Kenwood TH-D74 / TH-D75 USB",
			Baud:  9600,
			Help:  "Enter KISS with [F][LIST] until the display shows KISS+1200 or KISS+9600. Do not send KISS ON/RESTART. Menu 505 is the RF rate, not USB baud. Windows: install Kenwood USB CDC VCP (not Silicon Labs CP210x) before first plug; pick TH-Dxx (COMxx). Menu 980 Mass Storage removes the COM port. USB CDC usually ignores baud; 9600 is fine.",
		},
		{
			ID:    profileKenwoodBT,
			Label: "Kenwood TH-D74 / TH-D75 Bluetooth SPP",
			Baud:  115200,
			Help:  "Pair the radio, then pick its KISS SPP COM (not Bluetooth-Incoming-Port). Pairing can create several COMs; if one opens but is silent, try the other. Host rate 115200. Still enter KISS with [F][LIST]; do not send KISS ON.",
		},
		{
			ID:    profileMobilinkd,
			Label: "Mobilinkd TNC3 / TNC4 USB",
			Baud:  9600,
			Help:  "USB-CDC; host baud is often ignored. 9600 is fine. BLE is a different path (not this transport).",
		},
	}
}
