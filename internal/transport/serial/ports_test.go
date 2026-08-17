package serial

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withRawLister(t *testing.T, fn rawLister) {
	t.Helper()
	prev := listRaw
	listRaw = fn
	t.Cleanup(func() { listRaw = prev })
}

func TestListPortsEmpty(t *testing.T) {
	withRawLister(t, func() ([]rawPort, error) {
		return []rawPort{}, nil
	})
	ports, err := ListPorts()
	if err != nil {
		t.Fatalf("ListPorts: %v", err)
	}
	if ports == nil {
		t.Fatal("ports is nil, want empty non-nil slice")
	}
	if len(ports) != 0 {
		t.Fatalf("len = %d, want 0", len(ports))
	}
}

func TestListPortsSkipsEmptyName(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: ""},
		{Name: "COM5", Product: "TH-D74", IsUSB: true},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (empty Name dropped)", len(got))
	}
	if got[0].Name != "COM5" {
		t.Errorf("Name = %q, want COM5", got[0].Name)
	}
}

func TestListPortsDropsTTYSAndTTYHS(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "/dev/ttyS0"},
		{Name: "/dev/ttyHS0"},
		{Name: "/dev/ttyACM0"},
		{Name: "/dev/ttyUSB0"},
		{Name: "/dev/rfcomm0"},
	})
	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
	}
	if names["/dev/ttyS0"] || names["/dev/ttyHS0"] {
		t.Errorf("motherboard UARTs should be omitted: %+v", names)
	}
	for _, want := range []string{"/dev/ttyACM0", "/dev/ttyUSB0", "/dev/rfcomm0"} {
		if !names[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestListPortsWindowsKenwoodLabel(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "COM5", Product: "TH-D74", IsUSB: true, VID: "2166", PID: "600B"},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	p := got[0]
	if !strings.Contains(p.Label, "TH-D74") || !strings.Contains(p.Label, "COM5") {
		t.Errorf("label = %q, want TH-D74 and COM5", p.Label)
	}
	if !p.Highlight {
		t.Error("highlight = false, want true")
	}
	if p.SuggestedProfile != profileKenwoodUSB {
		t.Errorf("suggestedProfile = %q, want %q", p.SuggestedProfile, profileKenwoodUSB)
	}
}

func TestListPortsWindowsProductAlreadyHasCOM(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "COM3", Product: "TH-D75 (COM3)", IsUSB: true},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Label != "TH-D75 (COM3)" {
		t.Errorf("label = %q, want %q", got[0].Label, "TH-D75 (COM3)")
	}
	if strings.Contains(got[0].Label, "(COM3) (COM3)") {
		t.Errorf("doubled COM in label: %q", got[0].Label)
	}
}

func TestListPortsKenwoodVIDFallback(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "/dev/ttyACM0", IsUSB: true, VID: "2166", PID: "600b"},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	p := got[0]
	if p.VID != "2166" || p.PID != "600B" {
		t.Errorf("VID:PID = %s:%s, want 2166:600B", p.VID, p.PID)
	}
	if !p.Highlight || p.SuggestedProfile != profileKenwoodUSB {
		t.Errorf("highlight=%v profile=%q", p.Highlight, p.SuggestedProfile)
	}
}

func TestListPortsKenwoodD75UnofficialVID(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "/dev/ttyACM0", IsUSB: true, VID: "2166", PID: "9023"},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].SuggestedProfile != profileKenwoodUSB {
		t.Errorf("suggestedProfile = %q, want %q", got[0].SuggestedProfile, profileKenwoodUSB)
	}
}

func TestListPortsMobilinkdMatch(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "COM8", Product: "TNC3 Mobilinkd", IsUSB: true},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].SuggestedProfile != profileMobilinkd {
		t.Errorf("suggestedProfile = %q, want %q", got[0].SuggestedProfile, profileMobilinkd)
	}
}

func TestListPortsMobilinkdVID(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "/dev/ttyACM1", IsUSB: true, VID: "1d50", PID: "6018"},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].SuggestedProfile != profileMobilinkd {
		t.Errorf("suggestedProfile = %q, want %q", got[0].SuggestedProfile, profileMobilinkd)
	}
	if got[0].VID != "1D50" || got[0].PID != "6018" {
		t.Errorf("VID:PID = %s:%s, want 1D50:6018", got[0].VID, got[0].PID)
	}
}

func TestListPortsGenericNoHighlight(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "COM1"},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Highlight {
		t.Error("highlight = true, want false")
	}
	if got[0].SuggestedProfile != "" {
		t.Errorf("suggestedProfile = %q, want empty", got[0].SuggestedProfile)
	}
}

func TestListPortsDoesNotMatchTHDxxPlaceholder(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "COM4", Product: "TH-Dxx", IsUSB: true},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Highlight || got[0].SuggestedProfile == profileKenwoodUSB {
		t.Errorf("docs placeholder TH-Dxx should not match: highlight=%v profile=%q",
			got[0].Highlight, got[0].SuggestedProfile)
	}
}

func TestListPortsSortHighlightFirst(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "COM1"},
		{Name: "COM5", Product: "TH-D74", IsUSB: true},
		{Name: "COM2", IsUSB: true, VID: "1234", PID: "5678"},
	})
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Name != "COM5" {
		t.Errorf("first = %q, want highlighted COM5", got[0].Name)
	}
}

func TestMatchProfileCaseInsensitive(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "COM5", Product: "th-d74", IsUSB: true},
	})
	if len(got) != 1 || got[0].SuggestedProfile != profileKenwoodUSB {
		t.Fatalf("did not match th-d74: %+v", got)
	}
}

func TestNormalizeVidPid(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2166", "2166"},
		{"600b", "600B"},
		{"0x2166", "2166"},
		{"0X600B", "600B"},
		{" 1d50 ", "1D50"},
	}
	for _, tc := range cases {
		if got := normalizeHexID(tc.in); got != tc.want {
			t.Errorf("normalizeHexID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStandardBaudRates(t *testing.T) {
	has9600, has115200 := false, false
	for _, b := range StandardBaudRates {
		if b == 9600 {
			has9600 = true
		}
		if b == 115200 {
			has115200 = true
		}
	}
	if !has9600 || !has115200 {
		t.Errorf("StandardBaudRates = %v, want 9600 and 115200", StandardBaudRates)
	}
}

func TestProfilesCatalogIDs(t *testing.T) {
	ids := map[string]bool{}
	for _, p := range Profiles() {
		ids[p.ID] = true
	}
	for _, want := range []string{profileGeneric, profileKenwoodUSB, profileKenwoodBT, profileMobilinkd} {
		if !ids[want] {
			t.Errorf("missing profile %q", want)
		}
	}
}

func TestListFromRawError(t *testing.T) {
	withRawLister(t, func() ([]rawPort, error) {
		return nil, errors.New("setupapi failed")
	})
	_, err := ListPorts()
	if err == nil || !strings.Contains(err.Error(), "setupapi failed") {
		t.Fatalf("err = %v, want setupapi failed", err)
	}
}

func TestPersistDeviceHelper(t *testing.T) {
	if got := persistDevice(PortInfo{Name: "COM5"}); got != "COM5" {
		t.Errorf("persistDevice name-only = %q", got)
	}
	if got := persistDevice(PortInfo{Name: "/dev/ttyACM0", StablePath: "/dev/serial/by-id/usb-Kenwood"}); got != "/dev/serial/by-id/usb-Kenwood" {
		t.Errorf("persistDevice stable = %q", got)
	}
}

func TestLinuxStablePathPrefersByID(t *testing.T) {
	root := t.TempDir()
	byID := filepath.Join(root, "by-id")
	if err := os.MkdirAll(byID, 0o755); err != nil {
		t.Fatal(err)
	}
	target := "../../ttyACM0"
	if err := os.Symlink(target, filepath.Join(byID, "usb-Kenwood_TH-D74_ABC")); err != nil {
		t.Fatal(err)
	}
	prev := serialByIDDir
	serialByIDDir = byID
	t.Cleanup(func() { serialByIDDir = prev })

	got := listFromRaw([]rawPort{{Name: "/dev/ttyACM0", IsUSB: true, Product: "TH-D74"}})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	want := filepath.Join(byID, "usb-Kenwood_TH-D74_ABC")
	if got[0].StablePath != want {
		t.Errorf("StablePath = %q, want %q", got[0].StablePath, want)
	}
	if persistDevice(got[0]) != want {
		t.Errorf("persistDevice = %q, want %q", persistDevice(got[0]), want)
	}
}

func TestLinuxByIDPicksDeterministic(t *testing.T) {
	root := t.TempDir()
	byID := filepath.Join(root, "by-id")
	if err := os.MkdirAll(byID, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two names for the same tty; plus one that points elsewhere.
	if err := os.Symlink("../../ttyACM0", filepath.Join(byID, "usb-Kenwood_if01")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../ttyACM0", filepath.Join(byID, "usb-Kenwood_if00")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../ttyUSB0", filepath.Join(byID, "usb-other")); err != nil {
		t.Fatal(err)
	}
	prev := serialByIDDir
	serialByIDDir = byID
	t.Cleanup(func() { serialByIDDir = prev })

	got := resolveLinuxByID("/dev/ttyACM0")
	want := filepath.Join(byID, "usb-Kenwood_if00") // sorted, first
	if got != want {
		t.Errorf("resolveLinuxByID = %q, want %q", got, want)
	}
}

func TestLinuxEnrichProductFromSysfs(t *testing.T) {
	root := t.TempDir()
	// /sys/class/tty/ttyACM0/device -> ../../devices/usb/1-1
	usb := filepath.Join(root, "devices", "usb", "1-1")
	if err := os.MkdirAll(usb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usb, "product"), []byte("TH-D74\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ttyDev := filepath.Join(root, "class", "tty", "ttyACM0")
	if err := os.MkdirAll(ttyDev, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(usb, filepath.Join(ttyDev, "device")); err != nil {
		t.Fatal(err)
	}

	prev := sysfsRoot
	sysfsRoot = root
	t.Cleanup(func() { sysfsRoot = prev })

	got := listFromRaw([]rawPort{{Name: "/dev/ttyACM0", IsUSB: true}})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Product != "TH-D74" {
		t.Errorf("Product = %q, want TH-D74", got[0].Product)
	}
	if got[0].SuggestedProfile != profileKenwoodUSB {
		t.Errorf("suggestedProfile = %q, want %q", got[0].SuggestedProfile, profileKenwoodUSB)
	}
}

func TestDarwinPreferCu(t *testing.T) {
	got := listFromRaw([]rawPort{
		{Name: "/dev/cu.usbmodem1"},
		{Name: "/dev/tty.usbmodem1"},
		{Name: "/dev/cu.usbmodem2"},
	})
	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
	}
	if names["/dev/tty.usbmodem1"] {
		t.Errorf("tty twin should be omitted: %+v", names)
	}
	if !names["/dev/cu.usbmodem1"] || !names["/dev/cu.usbmodem2"] {
		t.Errorf("cu ports missing: %+v", names)
	}
}
