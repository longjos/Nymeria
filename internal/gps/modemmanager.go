package gps

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// D-Bus identifiers. mmRootPath is also where ObjectManager lives
// (src/mm-base-manager.c). Modem object paths are /…/Modem/<N> — the
// published HTML docs saying /Modems/# are a doc bug; always take the path
// from ObjectManager rather than building one.
const (
	mmBusName   = "org.freedesktop.ModemManager1"
	mmRootPath  = dbus.ObjectPath("/org/freedesktop/ModemManager1")
	ifaceLoc    = "org.freedesktop.ModemManager1.Modem.Location"
	ifaceProps  = "org.freedesktop.DBus.Properties"
	ifaceObjMgr = "org.freedesktop.DBus.ObjectManager"
)

// MMModemLocationSource bits, from include/ModemManager-enums.h.
const (
	src3GPPLacCI    = uint32(1 << 0) // 1  — cell identity, NOT a position
	srcGPSRaw       = uint32(1 << 1) // 2
	srcGPSNMEA      = uint32(1 << 2) // 4
	srcCDMABS       = uint32(1 << 3) // 8
	srcGPSUnmanaged = uint32(1 << 4) // 16 — never appears in Location; out of scope
	srcAGPSMSA      = uint32(1 << 5) // 32
	srcAGPSMSB      = uint32(1 << 6) // 64
)

// ErrUnsupportedPlatform is returned by the modemmanager source on any OS
// without D-Bus/ModemManager. Defined here (not in the platform-tagged
// files) so both builds see the same sentinel.
var ErrUnsupportedPlatform = errors.New("modemmanager: live GPS via ModemManager is only supported on Linux — use gpsd or nmea on this platform")

// Actionable, grep-able error strings surfaced through SourceStatus.Error /
// the GPS pill / Settings. Keep these exact — the wiki quotes them verbatim.
const (
	msgNotAuthorized   = "modemmanager: not authorized to enable GPS — add a polkit rule granting this user org.freedesktop.ModemManager1.Device.Control and .Location, or run Nymeria as root (see the ModemManager GPS wiki page)"
	msgModemNotEnabled = `modemmanager: modem is not enabled — run "sudo mmcli -m 0 --enable" (a Sierra modem stuck in low power mode usually needs the FCC unlock; see the ModemManager GPS wiki page)`
	msgServiceUnknown  = "modemmanager: ModemManager is not running (start it: sudo systemctl start ModemManager)"
	noGPSCapabilityFmt = "modemmanager: modem %s has no GPS capability (location capabilities: %s) — check \"mmcli -m 0 --location-status\"; a Sierra modem in FCC lock or MBIM-only USB composition reports none"
)

// mmPropsChanged mirrors org.freedesktop.DBus.Properties.PropertiesChanged.
type mmPropsChanged struct {
	Interface   string
	Changed     map[string]dbus.Variant
	Invalidated []string
}

// mmConn is the narrow D-Bus surface this source needs. sysbusConn satisfies
// it on linux; a fake satisfies it in tests, so every fixture runs with no
// bus. The seam deliberately speaks dbus.Variant/dbus.ObjectPath rather than
// domain types — godbus is pure Go and compiles on windows/darwin, so this
// whole decode/select/assemble/dedup path is exercised on every platform's
// `go test`, with no build tag on the test file.
type mmConn interface {
	GetManagedObjects(ctx context.Context) (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error)
	GetProperty(ctx context.Context, path dbus.ObjectPath, iface, prop string) (dbus.Variant, error)
	Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) error
	WatchProperties(ctx context.Context, path dbus.ObjectPath) (<-chan mmPropsChanged, error)
	Close() error
}

// mmDialer opens the ModemManager connection. Replaced in tests.
type mmDialer func(ctx context.Context) (mmConn, error)

// MMConfig configures a ModemManager-backed GPS source.
type MMConfig struct {
	Device      string        // "" = first GPS-capable modem | modem index ("0") | D-Bus object path
	MinInterval time.Duration // drives both the poll tick and SetGpsRefreshRate
}

// MMSource is a Source that reads the GNSS receiver inside an LTE modem via
// ModemManager over D-Bus (Linux only).
type MMSource struct {
	cfg   MMConfig
	dial  mmDialer
	fixes chan Fix

	mu          sync.Mutex
	status      SourceStatus
	cancel      context.CancelFunc
	lastBlob    string    // NMEA snapshot dedup: MM re-serves the same cached blob
	lastRawUTC  string    // GPS_RAW utc-time dedup, same reasoning
	lastFixTime time.Time // GPS-time dedup: catches a change that doesn't move the blob's identity
	rawWarned   bool      // one-shot "RAW only" log per connection
}

// NewMMSource creates an MMSource. Call Start to begin reading.
func NewMMSource(cfg MMConfig) *MMSource {
	return &MMSource{
		cfg:    cfg,
		dial:   defaultMMDialer,
		fixes:  make(chan Fix, 8),
		status: SourceStatus{Type: "modemmanager", Target: mmTarget(cfg)},
	}
}

func mmTarget(cfg MMConfig) string {
	if cfg.Device != "" {
		return "modemmanager " + cfg.Device
	}
	return "modemmanager"
}

// Start begins the connect/read/reconnect loop. Returns nil once the loop
// goroutine is running.
func (s *MMSource) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer close(s.fixes)
		runLoop(ctx,
			func() (*mmSession, error) { return s.dialSession(ctx) },
			s.readLoop,
			s.setStatus,
		)
	}()
	return nil
}

// mmSession is a fully-established ModemManager session: the modem is
// selected, its GPS sources are enabled (Location.Setup succeeded), and the
// property-change watch is registered. runLoop treats *building* this as
// "dial" — deliberately more than just opening the bus — so that only a
// real success (a modem exists, advertises GPS, and Setup was authorized)
// resets the reconnect backoff to 1s. See dialSession's doc comment.
type mmSession struct {
	conn         mmConn
	path         dbus.ObjectPath
	prior        uint32
	priorSignals bool
	watch        <-chan mmPropsChanged
}

func (sess *mmSession) Close() error { return sess.conn.Close() }

// dialSession opens a D-Bus connection, selects a modem, enables the GPS
// sources it advertises, and registers the property-change watch — i.e.
// everything readLoop needs before it can just wait on signals.
//
// This is deliberately treated as runLoop's "dial" step, not folded into
// readLoop, for the reconnect backoff's sake: opening a private system-bus
// connection trivially succeeds on any Linux box with D-Bus running,
// whether or not ModemManager (let alone a GPS-capable modem) is present.
// If only THAT were "dial", runLoop would reset its backoff to 1s on every
// cycle even though selecting a modem, or enabling its location sources,
// fails instantly and permanently — retrying at 1Hz forever (from boot,
// against a nonexistent modem or a polkit rule that will never appear)
// instead of the intended exponential backoff up to the 60s cap. Making the
// whole handshake part of "dial" means a permanent condition (no modem, no
// GPS capability, polkit denial) is what fails, and grows backoff normally;
// only a session that actually reached the property-watch stage is treated
// as "connected".
func (s *MMSource) dialSession(ctx context.Context) (*mmSession, error) {
	c, err := s.dial(ctx)
	if err != nil {
		return nil, classifyDialError(err)
	}

	objs, err := c.GetManagedObjects(ctx)
	if err != nil {
		_ = c.Close()
		return nil, classifyDialError(err)
	}
	path, caps, err := selectModem(objs, s.cfg.Device)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	want := caps & (srcGPSNMEA | srcGPSRaw)

	prior, priorSignals, err := s.readEnabledAndSignals(ctx, c, path)
	if err != nil {
		_ = c.Close()
		return nil, classifyDialError(err)
	}

	// Setup takes an ABSOLUTE mask: sources omitted get disabled. OR our
	// want bits into whatever another client already enabled so we never
	// silently turn off NetworkManager's or gpsd's location sources.
	if err := c.Call(ctx, path, ifaceLoc+".Setup", prior|want, true); err != nil {
		_ = c.Close()
		return nil, classifySetupError(err)
	}

	// Non-fatal: some plugins/versions reject it. The default refresh rate is
	// whatever the modem reports, and it can be far slower than it sounds — a
	// Sierra EM7355 was measured at 3600s, i.e. one fix an hour, which is
	// useless for a live track or for smart beaconing but is not an error.
	rate := refreshRateSeconds(s.cfg.MinInterval)
	if err := c.Call(ctx, path, ifaceLoc+".SetGpsRefreshRate", rate); err != nil {
		log.Printf("[gps] modemmanager: SetGpsRefreshRate failed (%v); "+
			"fixes may arrive only as often as the modem's default (3600s on some Sierra modems)", err)
	}

	watch, err := c.WatchProperties(ctx, path)
	if err != nil {
		_ = c.Close()
		return nil, err
	}

	return &mmSession{conn: c, path: path, prior: prior, priorSignals: priorSignals, watch: watch}, nil
}

// readLoop drives one established session: seed a read, then wait on
// property-change signals with a slow poll as both a top-up and a liveness
// probe.
//
// Deliberately NOT using idleWatchdog here: cold-start TTFF on hardware like
// the EM7455 is 30-90s, during which MM publishes nothing. A 20s idle
// deadline would close the bus connection mid-acquisition, forcing a
// reconnect that re-runs Setup and restarts the GNSS engine — an infinite
// no-fix loop on exactly the hardware this feature targets. The periodic
// property Get below (tick.C) is the liveness probe instead: a dead bus
// errors there, which is what triggers reconnect.
func (s *MMSource) readLoop(ctx context.Context, sess *mmSession) error {
	c, path := sess.conn, sess.path

	s.setTarget(string(path))

	s.mu.Lock()
	s.rawWarned = false
	s.mu.Unlock()

	// Seed read so we are not blind until the first signal — on a cold
	// start that can be up to ~90s away.
	_ = s.handleLocation(ctx, c, path)

	tick := time.NewTicker(pollInterval(s.cfg.MinInterval))
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			// Shutdown only — never on a transient read error, which would
			// restart the GNSS engine and destroy TTFF.
			s.restore(c, path, sess.prior, sess.priorSignals)
			return ctx.Err()
		case ev, ok := <-sess.watch:
			if !ok {
				return errors.New("modemmanager: property signal stream closed")
			}
			if ev.Interface != ifaceLoc {
				continue
			}
			if v, ok := ev.Changed["Location"]; ok {
				s.ingestLocation(v)
				continue
			}
			for _, name := range ev.Invalidated {
				if name == "Location" {
					_ = s.handleLocation(ctx, c, path)
					break
				}
			}
		case <-tick.C:
			// Doubles as the liveness probe: a dead bus errors here, which
			// is what triggers reconnect. Silence alone never does.
			if err := s.handleLocation(ctx, c, path); err != nil {
				return err
			}
		}
	}
}

// readEnabledAndSignals reads the modem's current Enabled mask and
// SignalsLocation flag — used to OR our requested sources into what
// another client already has enabled, and to restore them on shutdown.
//
// A read error is returned, never swallowed into a false "0/false" prior:
// Setup takes an ABSOLUTE mask, so treating a failed read as "nothing else
// is enabled" would make Nymeria's own connect clobber another client's
// location sources, and restore() at shutdown would then disable location
// (and signal emission) for every client on the machine — silently, since
// nothing here would have logged an error. Aborting the connect instead
// (the caller closes the connection and lets runLoop retry) is the safe
// failure mode.
func (s *MMSource) readEnabledAndSignals(ctx context.Context, c mmConn, path dbus.ObjectPath) (uint32, bool, error) {
	v, err := c.GetProperty(ctx, path, ifaceLoc, "Enabled")
	if err != nil {
		return 0, false, fmt.Errorf("modemmanager: get Location.Enabled: %w", err)
	}
	var enabled uint32
	if err := v.Store(&enabled); err != nil {
		return 0, false, fmt.Errorf("modemmanager: Location.Enabled is %s, want u: %w", v.Signature().String(), err)
	}

	sv, err := c.GetProperty(ctx, path, ifaceLoc, "SignalsLocation")
	if err != nil {
		return 0, false, fmt.Errorf("modemmanager: get Location.SignalsLocation: %w", err)
	}
	var signals bool
	if err := sv.Store(&signals); err != nil {
		return 0, false, fmt.Errorf("modemmanager: Location.SignalsLocation is %s, want b: %w", sv.Signature().String(), err)
	}
	return enabled, signals, nil
}

// handleLocation fetches the Location property explicitly (used for the
// seed read, the Invalidated fallback, and the liveness-probe poll) and
// ingests it exactly like a PropertiesChanged value would be.
func (s *MMSource) handleLocation(ctx context.Context, c mmConn, path dbus.ObjectPath) error {
	v, err := c.GetProperty(ctx, path, ifaceLoc, "Location")
	if err != nil {
		return err
	}
	s.ingestLocation(v)
	return nil
}

// ingestLocation decodes one Location property value (from either a signal
// or an explicit Get) and, if it yields new position data, pushes a Fix.
// An empty map, or one carrying only 3GPP_LAC_CI, is NOT a fix loss — MM
// blanks the property when signal_location is false and at interface init —
// so it is silently ignored rather than treated as an error.
func (s *MMSource) ingestLocation(v dbus.Variant) {
	blob, haveNMEA, err := nmeaFromLocation(v)
	if err != nil {
		return
	}
	if haveNMEA {
		s.ingestNMEABlob(blob)
		return
	}

	fix, haveRaw, err := rawFixFromLocation(v, time.Now())
	if err != nil {
		return
	}
	if !haveRaw {
		return
	}
	s.warnRawOnce()
	utc, _ := rawUTCTime(v)
	s.ingestRawFix(fix, utc)
}

func (s *MMSource) ingestNMEABlob(blob string) {
	s.mu.Lock()
	if blob == s.lastBlob {
		s.mu.Unlock()
		return
	}
	s.lastBlob = blob
	s.mu.Unlock()

	if blob == "" {
		return
	}

	sentences := orderSnapshot(splitNMEA(blob))
	asm := NewAssembler(time.Now)
	fix, ok, _ := asm.ConsumeSnapshot(sentences)
	if !ok {
		return
	}
	s.emitFix(fix)
}

func (s *MMSource) ingestRawFix(fix Fix, utc string) {
	s.mu.Lock()
	if utc != "" && utc == s.lastRawUTC {
		s.mu.Unlock()
		return
	}
	if utc != "" {
		s.lastRawUTC = utc
	}
	s.mu.Unlock()
	s.emitFix(fix)
}

// emitFix applies the fix-time dedup layer and pushes to the channel. A
// zero fix.Time (void RMC, empty GGA, or an unparsed RAW utc-time) is exempt
// from this dedup and always passes through — it carries the mode
// transition the UI needs to show "No fix".
func (s *MMSource) emitFix(fix Fix) {
	s.mu.Lock()
	if !fix.Time.IsZero() && fix.Time.Equal(s.lastFixTime) {
		s.mu.Unlock()
		return
	}
	if !fix.Time.IsZero() {
		s.lastFixTime = fix.Time
	}
	s.status.LastRx = time.Now()
	s.mu.Unlock()

	select {
	case s.fixes <- fix:
	default:
	}
}

func (s *MMSource) warnRawOnce() {
	s.mu.Lock()
	already := s.rawWarned
	s.rawWarned = true
	s.mu.Unlock()
	if !already {
		log.Printf("[gps] modemmanager: modem reports GPS_RAW only — no speed/course, smart beaconing will fall back to time-based rate")
	}
}

func (s *MMSource) setTarget(path string) {
	s.mu.Lock()
	s.status.Target = path
	s.mu.Unlock()
}

// restore best-effort restores the modem's prior Setup mask on deliberate
// shutdown, using a short detached context since ctx is already cancelled.
// Setup takes an absolute mask, so never call this on a transient read
// error — that would restart the GNSS engine mid-acquisition.
func (s *MMSource) restore(c mmConn, path dbus.ObjectPath, prior uint32, priorSignals bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 2*time.Second)
	defer cancel()
	_ = c.Call(ctx, path, ifaceLoc+".Setup", prior, priorSignals)
}

func (s *MMSource) setStatus(connected bool, errMsg string) {
	s.mu.Lock()
	s.status.Connected = connected
	s.status.Error = errMsg
	s.mu.Unlock()
}

// Fixes returns the channel of assembled Fixes.
func (s *MMSource) Fixes() <-chan Fix { return s.fixes }

// Status returns the current connection status.
func (s *MMSource) Status() SourceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Close stops the read/reconnect loop.
func (s *MMSource) Close() error {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// ── Pure helpers (no bus; table-tested) ──────────────────────────────

// splitNMEA splits a ModemManager NMEA blob into sentences. MM's own spec
// joins with CRLF, but real modems are sloppy, so this also tolerates bare
// LF. Only lines starting with "$" are kept.
func splitNMEA(blob string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(blob, func(r rune) bool { return r == '\r' || r == '\n' }) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$") {
			out = append(out, line)
		}
	}
	return out
}

// orderSnapshot stably reorders sentences GSA, GGA, RMC, other — a
// belt-and-braces measure so Assembler.finalize sees GSA's Mode/HDOP
// enrichment applied last regardless of the order MM cached them in (this
// is already order-independent for ConsumeSnapshot itself, but keeping
// input order predictable makes fixtures and debugging easier).
func orderSnapshot(sentences []string) []string {
	rank := func(s string) int {
		id := s
		if comma := strings.IndexByte(s, ','); comma >= 0 {
			id = s[:comma]
		}
		if len(id) < 3 {
			return 3
		}
		switch strings.ToUpper(id[len(id)-3:]) {
		case "GSA":
			return 0
		case "GGA":
			return 1
		case "RMC":
			return 2
		default:
			return 3
		}
	}
	out := make([]string, len(sentences))
	copy(out, sentences)
	// Stable insertion sort by rank — the input is at most a handful of
	// sentences, and stability preserves relative order within a class.
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && rank(out[j-1]) > rank(out[j]) {
			out[j-1], out[j] = out[j], out[j-1]
			j--
		}
	}
	return out
}

// nmeaFromLocation decodes the a{uv} Location property and returns the
// cached NMEA blob under MM_MODEM_LOCATION_SOURCE_GPS_NMEA (key 4). A map
// present but lacking that key (e.g. only 3GPP_LAC_CI) is not an error —
// (blob="", ok=false, err=nil).
func nmeaFromLocation(v dbus.Variant) (string, bool, error) {
	var loc map[uint32]dbus.Variant
	if err := v.Store(&loc); err != nil {
		return "", false, fmt.Errorf("modemmanager: Location is %s, want a{uv}: %w", v.Signature().String(), err)
	}
	inner, ok := loc[srcGPSNMEA]
	if !ok {
		return "", false, nil
	}
	var blob string
	if err := inner.Store(&blob); err != nil {
		return "", false, fmt.Errorf("modemmanager: NMEA entry is %s, want s: %w", inner.Signature().String(), err)
	}
	return blob, true, nil
}

// rawFixFromLocation decodes the a{uv} Location property's
// MM_MODEM_LOCATION_SOURCE_GPS_RAW (key 2) entry — an a{sv} dict with
// "latitude"/"longitude" (d), optional "altitude" (d), and an optional
// "utc-time" (s). Mode is Mode3D when altitude is present, else Mode2D;
// Satellites/HDOP/SpeedKnots/Course are always zero (RAW carries none of
// them). utc-time is parsed best-effort (RFC3339, then bare "hhmmss" — the
// latter carries no date, so it is anchored to now's UTC calendar day, same
// as NMEA time-only fields with no accompanying date sentence); a malformed
// or absent value leaves Fix.Time zero without failing the fix.
func rawFixFromLocation(v dbus.Variant, now time.Time) (Fix, bool, error) {
	var loc map[uint32]dbus.Variant
	if err := v.Store(&loc); err != nil {
		return Fix{}, false, fmt.Errorf("modemmanager: Location is %s, want a{uv}: %w", v.Signature().String(), err)
	}
	inner, ok := loc[srcGPSRaw]
	if !ok {
		return Fix{}, false, nil
	}
	var m map[string]dbus.Variant
	if err := inner.Store(&m); err != nil {
		return Fix{}, false, fmt.Errorf("modemmanager: RAW entry is %s, want a{sv}: %w", inner.Signature().String(), err)
	}

	latV, latOK := m["latitude"]
	lonV, lonOK := m["longitude"]
	if !latOK || !lonOK {
		return Fix{}, false, nil
	}
	var lat, lon float64
	if err := latV.Store(&lat); err != nil {
		return Fix{}, false, fmt.Errorf("modemmanager: RAW latitude is %s, want d: %w", latV.Signature().String(), err)
	}
	if err := lonV.Store(&lon); err != nil {
		return Fix{}, false, fmt.Errorf("modemmanager: RAW longitude is %s, want d: %w", lonV.Signature().String(), err)
	}

	f := Fix{Lat: lat, Lon: lon, Mode: Mode2D, ReceivedAt: now}
	if altV, ok := m["altitude"]; ok {
		var alt float64
		if err := altV.Store(&alt); err == nil {
			f.Altitude, f.HasAltitude = alt, true
			f.Mode = Mode3D
		}
	}
	if utcV, ok := m["utc-time"]; ok {
		var utc string
		if err := utcV.Store(&utc); err == nil && utc != "" {
			if t, perr := time.Parse(time.RFC3339, utc); perr == nil {
				f.Time = t
			} else if t, perr := time.Parse("150405", utc); perr == nil {
				// "150405" alone parses to year 0000 — MM's RAW source
				// gives no date, only time-of-day, so anchor it to now's
				// UTC calendar day rather than serializing a year-zero
				// timestamp to the frontend.
				d := now.UTC()
				f.Time = time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
			}
			// else: leave zero — optional field, never fail the fix.
		}
	}
	return f, true, nil
}

// rawUTCTime extracts the raw "utc-time" string from a RAW Location entry,
// for dedup purposes only — independent of whether it parses into a
// time.Time, so a value MM re-serves unchanged is still recognized as a
// repeat even when it fails to parse.
func rawUTCTime(v dbus.Variant) (string, bool) {
	var loc map[uint32]dbus.Variant
	if err := v.Store(&loc); err != nil {
		return "", false
	}
	inner, ok := loc[srcGPSRaw]
	if !ok {
		return "", false
	}
	var m map[string]dbus.Variant
	if err := inner.Store(&m); err != nil {
		return "", false
	}
	uv, ok := m["utc-time"]
	if !ok {
		return "", false
	}
	var s string
	if err := uv.Store(&s); err != nil {
		return "", false
	}
	return s, true
}

// selectModem picks the modem to use: an object whose interface set contains
// Location and whose Capabilities advertise GPS_NMEA or GPS_RAW. Absence of
// the Location interface key IS the answer for "no GPS support" — no extra
// round trip needed, Capabilities is already in the nested props map.
func selectModem(objs map[dbus.ObjectPath]map[string]map[string]dbus.Variant, device string) (dbus.ObjectPath, uint32, error) {
	if len(objs) == 0 {
		return "", 0, errors.New("modemmanager: no modem found (check: mmcli -L)")
	}

	capsFor := func(path dbus.ObjectPath) (uint32, bool) {
		props, ok := objs[path][ifaceLoc]
		if !ok {
			return 0, false
		}
		var c uint32
		if v, ok := props["Capabilities"]; ok {
			_ = v.Store(&c)
		}
		return c, true
	}
	hasGPS := func(c uint32) bool { return c&(srcGPSNMEA|srcGPSRaw) != 0 }

	if device != "" {
		var path dbus.ObjectPath
		found := false
		switch {
		case strings.HasPrefix(device, "/"):
			if _, ok := objs[dbus.ObjectPath(device)]; ok {
				path, found = dbus.ObjectPath(device), true
			}
		case isAllDigits(device):
			suffix := "/" + device
			for p := range objs {
				if strings.HasSuffix(string(p), suffix) {
					path, found = p, true
					break
				}
			}
		default:
			return "", 0, fmt.Errorf("modemmanager: gps.device must be a modem index (\"0\") or a D-Bus object path, got %q", device)
		}
		if !found {
			return "", 0, fmt.Errorf("modemmanager: no modem matching device %q (check: mmcli -L)", device)
		}
		caps, hasLoc := capsFor(path)
		if !hasLoc || !hasGPS(caps) {
			// Never silently fall through to another modem once the
			// operator named one explicitly.
			return "", 0, fmt.Errorf(noGPSCapabilityFmt, path, capabilityNames(caps))
		}
		return path, caps, nil
	}

	// device == "": pick the lexicographically smallest GPS-capable
	// candidate. Map iteration order in Go is random; without sorting,
	// which modem gets used would flip-flop across restarts.
	var best dbus.ObjectPath
	var bestCaps uint32
	haveBest := false
	var anyLoc dbus.ObjectPath
	var anyLocCaps uint32
	sawLoc := false
	for path := range objs {
		caps, hasLoc := capsFor(path)
		if !hasLoc {
			continue
		}
		if !sawLoc || path < anyLoc {
			sawLoc, anyLoc, anyLocCaps = true, path, caps
		}
		if hasGPS(caps) && (!haveBest || path < best) {
			haveBest, best, bestCaps = true, path, caps
		}
	}
	if haveBest {
		return best, bestCaps, nil
	}
	if sawLoc {
		return "", 0, fmt.Errorf(noGPSCapabilityFmt, anyLoc, capabilityNames(anyLocCaps))
	}
	return "", 0, errors.New("modemmanager: no modem found (check: mmcli -L)")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// capabilityNames renders mmcli's own capability names, so an error message
// can be pasted next to `mmcli` output.
func capabilityNames(caps uint32) string {
	if caps == 0 {
		return "none"
	}
	order := []struct {
		bit  uint32
		name string
	}{
		{src3GPPLacCI, "3gpp-lac-ci"},
		{srcGPSRaw, "gps-raw"},
		{srcGPSNMEA, "gps-nmea"},
		{srcCDMABS, "cdma-bs"},
		{srcGPSUnmanaged, "gps-unmanaged"},
		{srcAGPSMSA, "agps-msa"},
		{srcAGPSMSB, "agps-msb"},
	}
	var parts []string
	for _, o := range order {
		if caps&o.bit != 0 {
			parts = append(parts, o.name)
		}
	}
	return strings.Join(parts, ", ")
}

// refreshRateSeconds converts MinInterval to the GpsRefreshRate argument:
// 1 when min <= 1s, else the whole-second value, capped at 30 (MM's own
// default, and the ceiling SetGpsRefreshRate accepts meaningfully).
func refreshRateSeconds(min time.Duration) uint32 {
	if min <= time.Second {
		return 1
	}
	sec := uint32(min / time.Second)
	if sec > 30 {
		return 30
	}
	return sec
}

// pollInterval is max(1s, min), capped at 10s — the poll doubles as the
// liveness probe, so it must never be so slow that a dead bus goes
// undetected for a long time.
func pollInterval(min time.Duration) time.Duration {
	if min <= time.Second {
		return time.Second
	}
	if min > 10*time.Second {
		return 10 * time.Second
	}
	return min
}

// classifyDialError turns raw dial/first-bus-call errors into actionable
// messages. A missing bus socket surfaces as a *net.OpError, not a
// dbus.Error — never type-switch on dbus.Error alone to detect "no bus".
func classifyDialError(err error) error {
	if err == nil {
		return nil
	}
	var de dbus.Error
	if errors.As(err, &de) {
		if de.Name == "org.freedesktop.DBus.Error.ServiceUnknown" {
			return errors.New(msgServiceUnknown)
		}
		return err
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Errorf("modemmanager: cannot connect to the system D-Bus: %w", err)
	}
	return err
}

// classifySetupError turns Location.Setup's D-Bus error into an actionable
// message. Setup is gated by Device.Control, not by Location — polkit's
// shipped default has nobody to prompt for a session-less daemon, so this
// must never render as a silent "no fix".
func classifySetupError(err error) error {
	if err == nil {
		return nil
	}
	var de dbus.Error
	if errors.As(err, &de) {
		name := de.Name
		switch {
		case name == "org.freedesktop.DBus.Error.AccessDenied",
			name == "org.freedesktop.DBus.Error.AuthFailed",
			name == "org.freedesktop.DBus.Error.InteractiveAuthorizationRequired",
			strings.Contains(name, "Unauthorized"),
			strings.Contains(name, "NotAuthorized"),
			strings.Contains(name, "PolicyKit"):
			return errors.New(msgNotAuthorized)
		case strings.HasSuffix(name, ".Core.WrongState"):
			return errors.New(msgModemNotEnabled)
		case strings.HasSuffix(name, ".Core.Unsupported"):
			return fmt.Errorf("modemmanager: modem rejected the requested GPS sources: %w", err)
		}
	}
	return fmt.Errorf("modemmanager: Setup: %w", err)
}
