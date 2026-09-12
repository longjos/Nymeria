package gps

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// ── fixtures ──────────────────────────────────────────────────────────
//
// Checksums verified by hand (see the plan) against the shared snapNMEA*
// fixtures in nmea_test.go — reused here rather than redeclared.

// blobCRLF joins sentences the way ModemManager's own spec does: CRLF.
func blobCRLF(s ...string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += "\r\n"
		}
		out += v
	}
	return out
}

func locNMEA(blob string) dbus.Variant {
	return dbus.MakeVariant(map[uint32]dbus.Variant{srcGPSNMEA: dbus.MakeVariant(blob)})
}

func locBoth(nmeaBlob string, lat, lon float64) dbus.Variant {
	raw := map[string]dbus.Variant{
		"latitude":  dbus.MakeVariant(lat),
		"longitude": dbus.MakeVariant(lon),
	}
	return dbus.MakeVariant(map[uint32]dbus.Variant{
		srcGPSNMEA: dbus.MakeVariant(nmeaBlob),
		srcGPSRaw:  dbus.MakeVariant(raw),
	})
}

func locRaw(lat, lon float64, alt *float64, utc string) dbus.Variant {
	m := map[string]dbus.Variant{
		"utc-time":  dbus.MakeVariant(utc), // STRING, not a double — common mis-decode
		"latitude":  dbus.MakeVariant(lat),
		"longitude": dbus.MakeVariant(lon),
	}
	if alt != nil {
		m["altitude"] = dbus.MakeVariant(*alt)
	}
	return dbus.MakeVariant(map[uint32]dbus.Variant{srcGPSRaw: dbus.MakeVariant(m)})
}

func locLACOnly() dbus.Variant {
	return dbus.MakeVariant(map[uint32]dbus.Variant{src3GPPLacCI: dbus.MakeVariant("310,260,8BE3,2BAF")})
}

func locEmpty() dbus.Variant {
	return dbus.MakeVariant(map[uint32]dbus.Variant{})
}

// ── pure helpers ──────────────────────────────────────────────────────

func TestSplitNMEA(t *testing.T) {
	tests := []struct {
		name string
		blob string
		want []string
	}{
		{"CRLF joined", blobCRLF(snapGGA, snapRMC), []string{snapGGA, snapRMC}},
		{"bare LF joined", snapGGA + "\n" + snapRMC, []string{snapGGA, snapRMC}},
		{"trailing CRLF, no empty entry", blobCRLF(snapGGA, snapRMC) + "\r\n", []string{snapGGA, snapRMC}},
		{"leading/trailing spaces trimmed", "  " + snapGGA + "  \r\n" + snapRMC, []string{snapGGA, snapRMC}},
		{"non-$ line dropped", "junk\r\n" + snapGGA, []string{snapGGA}},
		{"empty blob", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitNMEA(tt.blob)
			if len(got) != len(tt.want) {
				t.Fatalf("splitNMEA(%q) = %v, want %v", tt.blob, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitNMEA(%q)[%d] = %q, want %q", tt.blob, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestOrderSnapshot(t *testing.T) {
	in := []string{snapRMC, snapGGA, snapGSA, "$GPGSV,1,1,00*79"}
	got := orderSnapshot(in)
	if len(got) != 4 {
		t.Fatalf("orderSnapshot returned %d entries, want 4", len(got))
	}
	if got[0] != snapGSA || got[1] != snapGGA || got[2] != snapRMC {
		t.Errorf("orderSnapshot(%v) = %v, want GSA,GGA,RMC first", in, got)
	}
}

func TestNMEAFromLocation(t *testing.T) {
	t.Run("key 4 present", func(t *testing.T) {
		blob, ok, err := nmeaFromLocation(locNMEA(snapGGA))
		if err != nil || !ok || blob != snapGGA {
			t.Fatalf("nmeaFromLocation = (%q, %v, %v), want (%q, true, nil)", blob, ok, err, snapGGA)
		}
	})
	t.Run("only LAC/CI, key 1", func(t *testing.T) {
		blob, ok, err := nmeaFromLocation(locLACOnly())
		if err != nil || ok || blob != "" {
			t.Fatalf("nmeaFromLocation = (%q, %v, %v), want (\"\", false, nil)", blob, ok, err)
		}
	})
	t.Run("key 1 and key 4 both present returns key 4", func(t *testing.T) {
		v := dbus.MakeVariant(map[uint32]dbus.Variant{
			src3GPPLacCI: dbus.MakeVariant("310,260,8BE3,2BAF"),
			srcGPSNMEA:   dbus.MakeVariant(snapRMC),
		})
		blob, ok, err := nmeaFromLocation(v)
		if err != nil || !ok || blob != snapRMC {
			t.Fatalf("nmeaFromLocation = (%q, %v, %v), want (%q, true, nil)", blob, ok, err, snapRMC)
		}
	})
	t.Run("empty a{uv} is not a fix loss", func(t *testing.T) {
		blob, ok, err := nmeaFromLocation(locEmpty())
		if err != nil || ok || blob != "" {
			t.Fatalf("nmeaFromLocation(empty) = (%q, %v, %v), want (\"\", false, nil)", blob, ok, err)
		}
	})
	t.Run("wrong outer signature errors", func(t *testing.T) {
		_, ok, err := nmeaFromLocation(dbus.MakeVariant("not a map"))
		if ok || err == nil {
			t.Fatalf("nmeaFromLocation(bad sig) = (_, %v, %v), want (_, false, non-nil)", ok, err)
		}
	})
	t.Run("key 4 inner wrong type errors, no panic", func(t *testing.T) {
		v := dbus.MakeVariant(map[uint32]dbus.Variant{
			srcGPSNMEA: dbus.MakeVariant(map[string]dbus.Variant{"x": dbus.MakeVariant("y")}),
		})
		_, ok, err := nmeaFromLocation(v)
		if ok || err == nil {
			t.Fatalf("nmeaFromLocation(wrong inner type) = (_, %v, %v), want (_, false, non-nil)", ok, err)
		}
	})
}

func TestRawFixFromLocation(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("with altitude", func(t *testing.T) {
		alt := 545.4
		f, ok, err := rawFixFromLocation(locRaw(48.1173, 11.516667, &alt, "123519"), now)
		if err != nil || !ok {
			t.Fatalf("rawFixFromLocation = (_, %v, %v)", ok, err)
		}
		if f.Mode != Mode3D || !f.HasAltitude || f.Altitude != alt {
			t.Errorf("f = %+v, want Mode3D with altitude %v", f, alt)
		}
		if f.SpeedKnots != 0 || f.HasCourse || f.Satellites != 0 || f.HDOP != 0 {
			t.Errorf("f = %+v, want all RAW-absent fields zero", f)
		}
	})
	t.Run("without altitude", func(t *testing.T) {
		f, ok, err := rawFixFromLocation(locRaw(48.1173, 11.516667, nil, "123519"), now)
		if err != nil || !ok {
			t.Fatalf("rawFixFromLocation = (_, %v, %v)", ok, err)
		}
		if f.Mode != Mode2D || f.HasAltitude {
			t.Errorf("f = %+v, want Mode2D without altitude", f)
		}
	})
	t.Run("utc-time as RFC3339 decodes", func(t *testing.T) {
		f, ok, err := rawFixFromLocation(locRaw(1, 2, nil, "1994-03-23T12:35:19Z"), now)
		if err != nil || !ok {
			t.Fatalf("rawFixFromLocation = (_, %v, %v)", ok, err)
		}
		want := time.Date(1994, 3, 23, 12, 35, 19, 0, time.UTC)
		if !f.Time.Equal(want) {
			t.Errorf("Time = %v, want %v", f.Time, want)
		}
	})
	t.Run("bare hhmmss utc-time anchors to now's UTC calendar day, not year zero", func(t *testing.T) {
		anchoredNow := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
		f, ok, err := rawFixFromLocation(locRaw(1, 2, nil, "123519"), anchoredNow)
		if err != nil || !ok {
			t.Fatalf("rawFixFromLocation = (_, %v, %v)", ok, err)
		}
		want := time.Date(2026, 6, 15, 12, 35, 19, 0, time.UTC)
		if !f.Time.Equal(want) {
			t.Errorf("Time = %v, want %v (hour/min/sec from utc-time, date from now) — a year-zero Fix.Time serializes to the frontend as \"0000-01-01T12:35:19Z\"", f.Time, want)
		}
		if f.Time.Year() < 2000 {
			t.Errorf("Time = %v has a year-zero-shaped date, want it anchored to now's calendar day", f.Time)
		}
	})
	t.Run("malformed utc-time leaves Time zero without failing the fix", func(t *testing.T) {
		f, ok, err := rawFixFromLocation(locRaw(1, 2, nil, "not-a-time"), now)
		if err != nil || !ok {
			t.Fatalf("rawFixFromLocation = (_, %v, %v), want ok with no error", ok, err)
		}
		if !f.Time.IsZero() {
			t.Errorf("Time = %v, want zero", f.Time)
		}
	})
	t.Run("missing latitude", func(t *testing.T) {
		v := dbus.MakeVariant(map[uint32]dbus.Variant{
			srcGPSRaw: dbus.MakeVariant(map[string]dbus.Variant{"longitude": dbus.MakeVariant(2.0)}),
		})
		_, ok, err := rawFixFromLocation(v, now)
		if ok || err != nil {
			t.Fatalf("rawFixFromLocation(missing lat) = (_, %v, %v), want (_, false, nil)", ok, err)
		}
	})
	t.Run("key absent entirely", func(t *testing.T) {
		_, ok, err := rawFixFromLocation(locLACOnly(), now)
		if ok || err != nil {
			t.Fatalf("rawFixFromLocation(no RAW key) = (_, %v, %v), want (_, false, nil)", ok, err)
		}
	})
}

// ── selectModem ───────────────────────────────────────────────────────

func objTree(m map[dbus.ObjectPath]uint32, noLoc ...dbus.ObjectPath) map[dbus.ObjectPath]map[string]map[string]dbus.Variant {
	out := make(map[dbus.ObjectPath]map[string]map[string]dbus.Variant)
	for path, caps := range m {
		out[path] = map[string]map[string]dbus.Variant{
			ifaceLoc: {"Capabilities": dbus.MakeVariant(caps)},
		}
	}
	for _, path := range noLoc {
		out[path] = map[string]map[string]dbus.Variant{} // exists, no Location interface at all
	}
	return out
}

func TestSelectModem(t *testing.T) {
	t.Run("only one modem exports Location", func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{
			"/org/freedesktop/ModemManager1/Modem/1": 6,
		}, "/org/freedesktop/ModemManager1/Modem/0")
		path, caps, err := selectModem(objs, "")
		if err != nil {
			t.Fatalf("selectModem: %v", err)
		}
		if path != "/org/freedesktop/ModemManager1/Modem/1" || caps != 6 {
			t.Errorf("selectModem = (%v, %v), want (/Modem/1, 6)", path, caps)
		}
	})

	t.Run("deterministic: always picks the lexicographically smallest", func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{
			"/org/freedesktop/ModemManager1/Modem/0": 6,
			"/org/freedesktop/ModemManager1/Modem/1": 6,
		})
		for i := 0; i < 50; i++ {
			path, _, err := selectModem(objs, "")
			if err != nil {
				t.Fatalf("selectModem: %v", err)
			}
			if path != "/org/freedesktop/ModemManager1/Modem/0" {
				t.Fatalf("iteration %d: selectModem = %v, want /Modem/0", i, path)
			}
		}
	})

	t.Run("LAC/CI only reports no GPS capability", func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{
			"/org/freedesktop/ModemManager1/Modem/0": 1,
		})
		_, _, err := selectModem(objs, "")
		if err == nil {
			t.Fatal("selectModem: want error, got nil")
		}
		if !containsAll(err.Error(), "has no GPS capability", "3gpp-lac-ci") {
			t.Errorf("error = %q, want it to mention 'has no GPS capability' and '3gpp-lac-ci'", err.Error())
		}
	})

	t.Run("no Location interface at all -> no modem found, no panic", func(t *testing.T) {
		objs := objTree(nil, "/org/freedesktop/ModemManager1/Modem/0")
		_, caps, err := selectModem(objs, "")
		if err == nil {
			t.Fatal("selectModem: want error, got nil")
		}
		if caps != 0 {
			t.Errorf("caps = %v, want 0", caps)
		}
		if !containsAll(err.Error(), "no modem found") {
			t.Errorf("error = %q, want 'no modem found'", err.Error())
		}
	})

	t.Run("RAW-only capability is selected", func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{
			"/org/freedesktop/ModemManager1/Modem/0": 2,
		})
		path, caps, err := selectModem(objs, "")
		if err != nil || caps != 2 {
			t.Fatalf("selectModem = (%v, %v, %v), want (/Modem/0, 2, nil)", path, caps, err)
		}
	})

	t.Run(`device "1" matches by index`, func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{
			"/org/freedesktop/ModemManager1/Modem/0": 6,
			"/org/freedesktop/ModemManager1/Modem/1": 4,
		})
		path, caps, err := selectModem(objs, "1")
		if err != nil || path != "/org/freedesktop/ModemManager1/Modem/1" || caps != 4 {
			t.Fatalf("selectModem(device=1) = (%v, %v, %v)", path, caps, err)
		}
	})

	t.Run("device as full object path matches", func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{
			"/org/freedesktop/ModemManager1/Modem/0": 6,
			"/org/freedesktop/ModemManager1/Modem/1": 4,
		})
		path, caps, err := selectModem(objs, "/org/freedesktop/ModemManager1/Modem/1")
		if err != nil || path != "/org/freedesktop/ModemManager1/Modem/1" || caps != 4 {
			t.Fatalf("selectModem(device=path) = (%v, %v, %v)", path, caps, err)
		}
	})

	t.Run("explicit device with no GPS never falls through to another modem", func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{
			"/org/freedesktop/ModemManager1/Modem/0": 6,
			"/org/freedesktop/ModemManager1/Modem/1": 1,
		})
		_, _, err := selectModem(objs, "1")
		if err == nil {
			t.Fatal("selectModem: want error, got nil")
		}
		if !containsAll(err.Error(), "has no GPS capability", "/Modem/1") {
			t.Errorf("error = %q, want it to name /Modem/1 and 'has no GPS capability'", err.Error())
		}
	})

	t.Run("garbage device string", func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{"/org/freedesktop/ModemManager1/Modem/0": 6})
		_, _, err := selectModem(objs, "usb-1")
		if err == nil || !containsAll(err.Error(), "must be a modem index", "or a D-Bus object path") {
			t.Errorf("error = %v, want the 'must be a modem index...' message", err)
		}
	})

	t.Run("empty object map", func(t *testing.T) {
		_, _, err := selectModem(map[dbus.ObjectPath]map[string]map[string]dbus.Variant{}, "")
		if err == nil || !containsAll(err.Error(), "no modem found") {
			t.Errorf("error = %v, want 'no modem found'", err)
		}
	})

	t.Run("device index matching nothing", func(t *testing.T) {
		objs := objTree(map[dbus.ObjectPath]uint32{"/org/freedesktop/ModemManager1/Modem/0": 6})
		_, _, err := selectModem(objs, "9")
		if err == nil || !containsAll(err.Error(), "no modem matching device") {
			t.Errorf("error = %v, want 'no modem matching device'", err)
		}
	})
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !stringsContains(s, sub) {
			return false
		}
	}
	return true
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestCapabilityNames(t *testing.T) {
	tests := []struct {
		caps uint32
		want string
	}{
		{0, "none"},
		{6, "gps-raw, gps-nmea"},
		{1 | 2 | 4, "3gpp-lac-ci, gps-raw, gps-nmea"},
	}
	for _, tt := range tests {
		if got := capabilityNames(tt.caps); got != tt.want {
			t.Errorf("capabilityNames(%d) = %q, want %q", tt.caps, got, tt.want)
		}
	}
}

func TestRefreshRateSeconds(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want uint32
	}{
		{0, 1},
		{500 * time.Millisecond, 1},
		{time.Second, 1},
		{5 * time.Second, 5},
		{10 * time.Minute, 30},
	}
	for _, tt := range tests {
		if got := refreshRateSeconds(tt.in); got != tt.want {
			t.Errorf("refreshRateSeconds(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPollInterval(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want time.Duration
	}{
		{0, time.Second},
		{500 * time.Millisecond, time.Second},
		{time.Second, time.Second},
		{5 * time.Second, 5 * time.Second},
		{10 * time.Minute, 10 * time.Second},
	}
	for _, tt := range tests {
		if got := pollInterval(tt.in); got != tt.want {
			t.Errorf("pollInterval(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestClassifyErrors(t *testing.T) {
	t.Run("AccessDenied -> not authorized", func(t *testing.T) {
		err := classifySetupError(dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"})
		if err.Error() != msgNotAuthorized {
			t.Errorf("err = %q, want the exact msgNotAuthorized string", err.Error())
		}
	})
	t.Run("Core.Unauthorized -> not authorized", func(t *testing.T) {
		err := classifySetupError(dbus.Error{Name: "org.freedesktop.ModemManager1.Error.Core.Unauthorized"})
		if err.Error() != msgNotAuthorized {
			t.Errorf("err = %q, want msgNotAuthorized", err.Error())
		}
	})
	t.Run("InteractiveAuthorizationRequired -> not authorized", func(t *testing.T) {
		err := classifySetupError(dbus.Error{Name: "org.freedesktop.DBus.Error.InteractiveAuthorizationRequired"})
		if err.Error() != msgNotAuthorized {
			t.Errorf("err = %q, want msgNotAuthorized", err.Error())
		}
	})
	t.Run("Core.WrongState -> modem not enabled", func(t *testing.T) {
		err := classifySetupError(dbus.Error{Name: "org.freedesktop.ModemManager1.Error.Core.WrongState"})
		if err.Error() != msgModemNotEnabled {
			t.Errorf("err = %q, want msgModemNotEnabled", err.Error())
		}
	})
	t.Run("Core.Unsupported -> rejected sources", func(t *testing.T) {
		err := classifySetupError(dbus.Error{Name: "org.freedesktop.ModemManager1.Error.Core.Unsupported"})
		if !stringsContains(err.Error(), "rejected the requested GPS sources") {
			t.Errorf("err = %q, want it to mention 'rejected the requested GPS sources'", err.Error())
		}
	})
	t.Run("ServiceUnknown via classifyDialError -> service not running", func(t *testing.T) {
		err := classifyDialError(dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"})
		if err.Error() != msgServiceUnknown {
			t.Errorf("err = %q, want msgServiceUnknown", err.Error())
		}
	})
	t.Run("plain net.OpError -> wrapped as cannot connect, not misclassified", func(t *testing.T) {
		opErr := &net.OpError{Op: "dial", Err: errors.New("no such file or directory")}
		err := classifyDialError(opErr)
		if !stringsContains(err.Error(), "cannot connect to the system D-Bus") {
			t.Errorf("err = %q, want it to mention 'cannot connect to the system D-Bus'", err.Error())
		}
	})
}

// ── the fake bus, end to end ──────────────────────────────────────────

type fakeCall struct {
	Path   dbus.ObjectPath
	Method string
	Args   []any
}

// fakeMMConn is a hand-driven mmConn: the test seeds the object tree and the
// property values, pushes PropertiesChanged events onto sigs, and records
// every method call so Setup's mask and SetGpsRefreshRate can be asserted.
type fakeMMConn struct {
	mu       sync.Mutex
	objects  map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	props    map[string]dbus.Variant // key: string(path) + "/" + prop
	calls    []fakeCall
	callErr  map[string]error // method -> error to return
	getErr   error            // makes GetProperty fail (dead-bus probe)
	sigs     chan mmPropsChanged
	closed   bool
	gmoTimes []time.Time // one entry per GetManagedObjects call, for backoff-timing tests
}

var _ mmConn = (*fakeMMConn)(nil)

// newFakeMMConn returns a fake pre-populated the way a real modem's Location
// interface always is: Enabled=0, SignalsLocation=false, Location=empty
// a{uv} (signal_location not yet turned on). Tests override individual
// properties with setLocation / direct props[...] writes as needed.
func newFakeMMConn() *fakeMMConn {
	f := &fakeMMConn{
		objects: objTree(map[dbus.ObjectPath]uint32{
			mmTestModemPath: 6, // NMEA + RAW
		}),
		props:   make(map[string]dbus.Variant),
		callErr: make(map[string]error),
		sigs:    make(chan mmPropsChanged, 8),
	}
	f.props[propKey(mmTestModemPath, "Enabled")] = dbus.MakeVariant(uint32(0))
	f.props[propKey(mmTestModemPath, "SignalsLocation")] = dbus.MakeVariant(false)
	f.props[propKey(mmTestModemPath, "Location")] = locEmpty()
	return f
}

func propKey(path dbus.ObjectPath, prop string) string { return string(path) + "/" + prop }

func (f *fakeMMConn) setLocation(path dbus.ObjectPath, v dbus.Variant) {
	f.mu.Lock()
	f.props[propKey(path, "Location")] = v
	f.mu.Unlock()
}

func (f *fakeMMConn) pushChanged(iface string, changed map[string]dbus.Variant) {
	f.sigs <- mmPropsChanged{Interface: iface, Changed: changed}
}

func (f *fakeMMConn) pushInvalidated(iface string, names ...string) {
	f.sigs <- mmPropsChanged{Interface: iface, Invalidated: names}
}

func (f *fakeMMConn) recordedCalls(method string) []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeCall
	for _, c := range f.calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeMMConn) GetManagedObjects(ctx context.Context) (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{Method: "GetManagedObjects"})
	f.gmoTimes = append(f.gmoTimes, time.Now())
	return f.objects, nil
}

func (f *fakeMMConn) GetProperty(ctx context.Context, path dbus.ObjectPath, iface, prop string) (dbus.Variant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{Path: path, Method: "GetProperty:" + prop})
	if f.getErr != nil {
		return dbus.Variant{}, f.getErr
	}
	v, ok := f.props[propKey(path, prop)]
	if !ok {
		return dbus.Variant{}, errors.New("fakeMMConn: no such property")
	}
	return v, nil
}

func (f *fakeMMConn) Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) error {
	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{Path: path, Method: methodName(method), Args: args})
	err := f.callErr[methodName(method)]
	f.mu.Unlock()
	return err
}

func methodName(full string) string {
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '.' {
			return full[i+1:]
		}
	}
	return full
}

func (f *fakeMMConn) WatchProperties(ctx context.Context, path dbus.ObjectPath) (<-chan mmPropsChanged, error) {
	return f.sigs, nil
}

func (f *fakeMMConn) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

const mmTestModemPath = dbus.ObjectPath("/org/freedesktop/ModemManager1/Modem/0")

func newTestMMSource(fake *fakeMMConn) *MMSource {
	s := NewMMSource(MMConfig{MinInterval: 50 * time.Millisecond})
	s.dial = func(context.Context) (mmConn, error) { return fake, nil }
	return s
}

func waitForFix(t *testing.T, s *MMSource) Fix {
	t.Helper()
	select {
	case f := <-s.Fixes():
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a fix")
		return Fix{}
	}
}

func expectNoFix(t *testing.T, s *MMSource, within time.Duration) {
	t.Helper()
	select {
	case f, ok := <-s.Fixes():
		if ok {
			t.Fatalf("unexpected fix: %+v", f)
		}
	case <-time.After(within):
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition never became true")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMMSourceSetupMask(t *testing.T) {
	fake := newFakeMMConn()
	fake.props[propKey(mmTestModemPath, "Enabled")] = dbus.MakeVariant(uint32(1)) // 3GPP_LAC_CI already on

	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return len(fake.recordedCalls("Setup")) >= 1 })

	call := fake.recordedCalls("Setup")[0]
	if len(call.Args) != 2 {
		t.Fatalf("Setup args = %v, want 2 args", call.Args)
	}
	mask, ok := call.Args[0].(uint32)
	if !ok {
		t.Fatalf("Setup mask arg has type %T, want uint32 (an untyped int marshals as 'i' and fails InvalidArgs)", call.Args[0])
	}
	if mask != uint32(1|2|4) {
		t.Errorf("Setup mask = %d, want 7 (prior 1 | requested 2|4)", mask)
	}
	enable, ok := call.Args[1].(bool)
	if !ok || !enable {
		t.Fatalf("Setup signal-enable arg = %v (%T), want true (bool)", call.Args[1], call.Args[1])
	}
}

func TestMMSourceSetsRefreshRate(t *testing.T) {
	fake := newFakeMMConn()
	s := NewMMSource(MMConfig{MinInterval: time.Second})
	s.dial = func(context.Context) (mmConn, error) { return fake, nil }
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return len(fake.recordedCalls("SetGpsRefreshRate")) >= 1 })
	call := fake.recordedCalls("SetGpsRefreshRate")[0]
	if len(call.Args) != 1 || call.Args[0] != uint32(1) {
		t.Errorf("SetGpsRefreshRate args = %v, want [uint32(1)]", call.Args)
	}
}

func TestMMSourceRefreshRateFailureIsNonFatal(t *testing.T) {
	fake := newFakeMMConn()
	fake.callErr["SetGpsRefreshRate"] = errors.New("rejected")

	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	fake.setLocation(mmTestModemPath, locNMEA(blobCRLF(snapGSA, snapGGA, snapRMC)))
	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locNMEA(blobCRLF(snapGSA, snapGGA, snapRMC))})
	f := waitForFix(t, s)
	if !f.HasPosition() {
		t.Error("expected a fix despite SetGpsRefreshRate failure")
	}
}

func TestMMSourceSeedsFromPropertyRead(t *testing.T) {
	fake := newFakeMMConn()
	fake.setLocation(mmTestModemPath, locNMEA(blobCRLF(snapGSA, snapGGA, snapRMC)))

	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	f := waitForFix(t, s)
	if !f.HasPosition() {
		t.Error("expected a seeded fix with no signal pushed")
	}
}

func TestMMSourceEmitsOnPropertiesChanged(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	blob := blobCRLF(snapGSA, snapGGA, snapRMC)
	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locNMEA(blob)})

	f := waitForFix(t, s)
	if f.Satellites != 8 {
		t.Errorf("Satellites = %d, want 8", f.Satellites)
	}
}

func TestMMSourceHandlesInvalidatedLocation(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	fake.setLocation(mmTestModemPath, locNMEA(blobCRLF(snapGSA, snapGGA, snapRMC)))
	fake.pushInvalidated(ifaceLoc, "Location")

	f := waitForFix(t, s)
	if !f.HasPosition() {
		t.Error("expected a fix from the fallback Get after Invalidated")
	}
}

func TestMMSourceIgnoresOtherInterfaces(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	fake.pushChanged("org.freedesktop.ModemManager1.Modem", map[string]dbus.Variant{"State": dbus.MakeVariant(int32(8))})
	expectNoFix(t, s, 300*time.Millisecond)
}

func TestMMSourceDropsRepeatedBlob(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	blob := blobCRLF(snapGSA, snapGGA, snapRMC)
	v := locNMEA(blob)
	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": v})
	waitForFix(t, s)

	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": v})
	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": v})
	expectNoFix(t, s, 300*time.Millisecond)
}

func TestMMSourceDropsRepeatedFixTime(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	blob1 := blobCRLF(snapGGA, snapRMC)
	blob2 := blobCRLF(snapGSA, snapGGA, snapRMC) // same RMC/GGA time, GSA added
	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locNMEA(blob1)})
	waitForFix(t, s)

	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locNMEA(blob2)})
	expectNoFix(t, s, 300*time.Millisecond)
}

func TestMMSourceEmitsOnNewFixTime(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locNMEA(blobCRLF(snapGSA, snapGGA, snapRMC))})
	f1 := waitForFix(t, s)
	if f1.Satellites != 8 {
		t.Errorf("f1.Satellites = %d, want 8", f1.Satellites)
	}

	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locNMEA(blobCRLF(snapGSAGN, snapGGAGN, snapRMCGN))})
	f2 := waitForFix(t, s)
	if f2.Satellites != 9 {
		t.Errorf("f2.Satellites = %d, want 9", f2.Satellites)
	}
}

func TestMMSourceEmptyLocationIsNotFixLoss(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locNMEA(blobCRLF(snapGSA, snapGGA, snapRMC))})
	waitForFix(t, s)

	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locEmpty()})
	expectNoFix(t, s, 300*time.Millisecond)

	st := s.Status()
	if !st.Connected || st.Error != "" {
		t.Errorf("Status = %+v, want still connected with no error", st)
	}
}

func TestMMSourceRawFallback(t *testing.T) {
	fake := newFakeMMConn()
	fake.objects = objTree(map[dbus.ObjectPath]uint32{mmTestModemPath: srcGPSRaw}) // RAW only

	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	alt := 100.0
	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locRaw(48.0, 11.0, &alt, "123519")})
	f := waitForFix(t, s)
	if f.Mode != Mode3D || f.SpeedKnots != 0 || f.HasCourse {
		t.Errorf("f = %+v, want Mode3D with zero speed/course", f)
	}

	call := fake.recordedCalls("Setup")[0]
	if call.Args[0] != uint32(2) {
		t.Errorf("Setup mask = %v, want uint32(2) (RAW only)", call.Args[0])
	}
}

func TestMMSourcePrefersNMEAOverRaw(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	blob := blobCRLF(snapGSA, snapGGA, snapRMC)
	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locBoth(blob, 1, 2)})
	f := waitForFix(t, s)
	if f.SpeedKnots != 22.4 {
		t.Errorf("SpeedKnots = %v, want 22.4 (NMEA path), proving RAW is only a fallback", f.SpeedKnots)
	}
}

func TestMMSourceVoidFixReported(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	fake.pushChanged(ifaceLoc, map[string]dbus.Variant{"Location": locNMEA(blobCRLF(snapRMCVoid, snapGGAEmpty))})
	f := waitForFix(t, s)
	if f.Mode != ModeNoFix {
		t.Errorf("Mode = %v, want ModeNoFix", f.Mode)
	}
}

func TestMMSourcePolkitDenialStatus(t *testing.T) {
	fake := newFakeMMConn()
	fake.callErr["Setup"] = dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}

	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return s.Status().Error != "" })
	st := s.Status()
	if st.Connected {
		t.Error("Connected = true, want false")
	}
	if st.Error != msgNotAuthorized {
		t.Errorf("Error = %q, want the exact msgNotAuthorized constant", st.Error)
	}
}

func TestMMSourceNoGPSCapabilityStatus(t *testing.T) {
	fake := newFakeMMConn()
	fake.objects = objTree(map[dbus.ObjectPath]uint32{mmTestModemPath: 1}) // LAC/CI only

	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return s.Status().Error != "" })
	if err := s.Status().Error; !stringsContains(err, "has no GPS capability") || !stringsContains(err, "--location-status") {
		t.Errorf("Error = %q, want it to mention 'has no GPS capability' and '--location-status'", err)
	}
}

func TestMMSourceUnsupportedPlatformSurfaces(t *testing.T) {
	s := NewMMSource(MMConfig{})
	s.dial = func(context.Context) (mmConn, error) { return nil, ErrUnsupportedPlatform }

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool { return stringsContains(s.Status().Error, "only supported on Linux") })

	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return promptly")
	}
}

func TestMMSourceReconnectsWhenPollFails(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	fake.mu.Lock()
	fake.getErr = errors.New("bus is gone")
	fake.mu.Unlock()

	waitFor(t, 4*time.Second, func() bool { return len(fake.recordedCalls("GetManagedObjects")) >= 2 })
}

// TestMMSourceBacksOffOnPermanentSelectModemFailure guards against MMSource
// retrying a permanent failure (here: no modem at all) at a flat ~1Hz
// forever. Before the fix, MMSource's "dial" (as far as runLoop's generic
// reconnect skeleton was concerned) was just opening the D-Bus connection —
// which trivially succeeds every time — so runLoop reset its backoff to 1s
// on every cycle even though selectModem failed instantly and permanently
// every time, producing a flat retry cadence instead of the intended
// exponential backoff. The fix folds modem-selection (and the rest of the
// connect handshake) into dialSession, which IS runLoop's "dial" now, so a
// permanent failure there grows backoff normally (1s, 2s, 4s, ...).
//
// This asserts the fix by timing gaps between successive
// GetManagedObjects calls rather than counting attempts in a fixed window,
// so it isn't sensitive to scheduler jitter: a fixed MMSource must show
// meaningfully growing gaps; a regressed one would show a flat ~1s gap
// throughout.
func TestMMSourceBacksOffOnPermanentSelectModemFailure(t *testing.T) {
	fake := newFakeMMConn()
	fake.objects = objTree(nil) // no modem present at all -> selectModem fails every attempt

	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, 6*time.Second, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.gmoTimes) >= 3
	})

	fake.mu.Lock()
	times := append([]time.Time(nil), fake.gmoTimes...)
	fake.mu.Unlock()
	if len(times) < 3 {
		t.Fatalf("only %d GetManagedObjects attempts recorded, want >= 3", len(times))
	}

	gap1 := times[1].Sub(times[0])
	gap2 := times[2].Sub(times[1])
	if gap2 < gap1+300*time.Millisecond {
		t.Errorf("gap1=%v gap2=%v between successive connect attempts; want gap2 meaningfully larger than gap1 (exponential backoff on a permanent failure, not a flat ~1Hz retry loop)", gap1, gap2)
	}

	if st := s.Status(); st.Error == "" {
		t.Error("Status().Error is empty, want the 'no modem found' error surfaced")
	}
}

// TestMMSourceAbortsConnectWhenEnabledReadFails guards against
// readEnabledAndSignals silently defaulting to (0, false) on a GetProperty
// error. Before the fix, an Enabled/SignalsLocation read failure (e.g. the
// modem re-enumerating mid-connect) fell through to prior=0, so the
// subsequent Location.Setup(0|want, true) call would clobber whatever
// location sources another client (geoclue, NetworkManager) already had
// enabled on the modem — and restore() at shutdown would then disable
// location entirely for every client on the machine, with nothing logged.
// The fix aborts the connect attempt instead (letting runLoop retry) rather
// than ever guessing prior=0.
func TestMMSourceAbortsConnectWhenEnabledReadFails(t *testing.T) {
	fake := newFakeMMConn()
	fake.mu.Lock()
	fake.getErr = errors.New("modem re-enumerated mid-read")
	fake.mu.Unlock()

	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool { return s.Status().Error != "" })

	if s.Status().Connected {
		t.Error("Connected = true, want false: the connect attempt should have aborted")
	}
	if n := len(fake.recordedCalls("Setup")); n != 0 {
		t.Errorf("Setup called %d times, want 0 — a failed Enabled/SignalsLocation read must never fall through to Setup(prior=0, ...)", n)
	}
}

func TestMMSourceSurvivesLongSilence(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake) // MinInterval 50ms -> pollInterval 1s (floor)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	// pollInterval floors at 1s (see the config above), so the poll tick
	// this test is meant to exercise doesn't fire until >1s in. Wait for at
	// least one tick-driven "Location" read beyond the seed read at
	// connect-time (2 total), so the assertions below actually cover a
	// no-op poll firing — not just the window before the first tick, which
	// would pass trivially regardless of whether idleWatchdog got re-added.
	waitFor(t, 3*time.Second, func() bool {
		return len(fake.recordedCalls("GetProperty:Location")) >= 2
	})

	// No idleWatchdog: a long silence (bounded here, well past readDeadline
	// would be 20s) must not force a reconnect on its own. We can't wait out
	// the full 20s in a unit test, but having proven at least one no-op
	// poll fired above, assert no second Setup call has happened and the
	// source is still connected — proving nothing spuriously restarted the
	// GNSS engine.
	if !s.Status().Connected {
		t.Fatal("source disconnected during silence with no error condition")
	}
	if n := len(fake.recordedCalls("Setup")); n != 1 {
		t.Errorf("Setup called %d times during silence, want 1 (no restart)", n)
	}
}

func TestMMSourceRestoresPriorSourcesOnShutdown(t *testing.T) {
	fake := newFakeMMConn()
	fake.props[propKey(mmTestModemPath, "Enabled")] = dbus.MakeVariant(uint32(1))
	fake.props[propKey(mmTestModemPath, "SignalsLocation")] = dbus.MakeVariant(true)

	s := newTestMMSource(fake)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	cancel()
	waitFor(t, 2*time.Second, func() bool { return len(fake.recordedCalls("Setup")) >= 2 })

	calls := fake.recordedCalls("Setup")
	last := calls[len(calls)-1]
	if last.Args[0] != uint32(1) {
		t.Errorf("restore Setup mask = %v, want uint32(1) (the prior mask)", last.Args[0])
	}
	if last.Args[1] != true {
		t.Errorf("restore Setup signals-enable = %v, want true (the prior value)", last.Args[1])
	}
}

func TestMMSourceNoRestoreOnTransientError(t *testing.T) {
	// prior=0, priorSignals=false (this fixture's defaults) means a
	// restore-shaped Setup call is Setup(uint32(0), false) — distinct from
	// every normal enable call, which ORs in the GPS want bits and passes
	// true. Reconnecting after a transient error legitimately calls Setup
	// again (with the normal enable mask) — that is not what this test
	// guards against. It guards against the shutdown-only restore(prior)
	// firing on a transient error, which would restart the GNSS engine
	// mid-acquisition instead of just retrying the connection.
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()
	waitFor(t, 2*time.Second, func() bool { return s.Status().Connected })

	fake.mu.Lock()
	fake.getErr = errors.New("transient")
	fake.mu.Unlock()

	waitFor(t, 4*time.Second, func() bool { return len(fake.recordedCalls("GetManagedObjects")) >= 2 })

	for _, call := range fake.recordedCalls("Setup") {
		if call.Args[0] == uint32(0) && call.Args[1] == false {
			t.Fatalf("a restore-shaped Setup(0, false) call was recorded after a transient error: %+v", call)
		}
	}
}

func TestMMSourceCloseIsIdempotent(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	select {
	case _, ok := <-s.Fixes():
		if ok {
			t.Fatal("Fixes() yielded a value after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fixes() channel was never closed")
	}
}

func TestMMSourceTargetBecomesObjectPath(t *testing.T) {
	fake := newFakeMMConn()
	s := newTestMMSource(fake)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	waitFor(t, 2*time.Second, func() bool {
		return s.Status().Target == string(mmTestModemPath)
	})
}
