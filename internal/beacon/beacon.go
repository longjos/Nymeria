package beacon

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// Config holds beacon configuration.
type Config struct {
	Enabled     bool           `yaml:"enabled"`
	Interval    time.Duration  `yaml:"interval"`     // Fixed interval (default 10min)
	Comment     string         `yaml:"comment"`      // Beacon comment text
	SmartBeacon *SmartConfig   `yaml:"smart_beacon"` // nil = disabled
	Path        []aprs.Address // nil = default WIDE1-1,WIDE2-1; empty = direct

	// LiveMaxAge is how fresh a live fix must be to be used. Zero means 30s.
	LiveMaxAge time.Duration `yaml:"live_max_age"`
}

// SmartConfig holds smart beaconing parameters.
type SmartConfig struct {
	Enabled   bool          `yaml:"enabled"`    // false (or a nil *SmartConfig) = fixed-interval beaconing
	FastSpeed float64       `yaml:"fast_speed"` // mph, above this use fast rate
	SlowSpeed float64       `yaml:"slow_speed"` // mph, below this use slow rate
	FastRate  time.Duration `yaml:"fast_rate"`  // beacon interval when fast (default 60s)
	SlowRate  time.Duration `yaml:"slow_rate"`  // beacon interval when slow (default 30min)
	TurnAngle float64       `yaml:"turn_angle"` // degrees, beacon on heading change (default 28)
	TurnSlope float64       `yaml:"turn_slope"` // additional angle per mph (default 26)
}

// Rate returns the beacon interval for the given speed in mph.
// Uses linear interpolation between slow and fast rates.
func (sc *SmartConfig) Rate(speed float64) time.Duration {
	if speed <= sc.SlowSpeed {
		return sc.SlowRate
	}
	if speed >= sc.FastSpeed {
		return sc.FastRate
	}
	// Linear interpolation between slow and fast
	fraction := (speed - sc.SlowSpeed) / (sc.FastSpeed - sc.SlowSpeed)
	interval := sc.SlowRate - time.Duration(fraction*float64(sc.SlowRate-sc.FastRate))
	return interval
}

// TurnThreshold returns the heading-change threshold (degrees) for the given speed.
// threshold = TurnAngle + TurnSlope / speed
func (sc *SmartConfig) TurnThreshold(speed float64) float64 {
	if speed >= sc.FastSpeed {
		return sc.TurnAngle
	}
	if speed <= 0 {
		return 360 // effectively never trigger
	}
	return sc.TurnAngle + sc.TurnSlope/speed
}

// StationInfo holds the station identity needed for beacon generation.
// Defined here to avoid circular imports with the config package.
type StationInfo struct {
	Callsign    string
	SSID        int
	Lat         float64
	Lon         float64
	SymbolTable string
	SymbolCode  string
}

// SendFunc is the function used to transmit a beacon frame.
type SendFunc func(aprs.APRSFrame) error

// LiveFix is a position sample from a live GPS source. The beacon package
// deliberately does not import internal/gps — app.go adapts.
type LiveFix struct {
	Lat        float64
	Lon        float64
	SpeedKnots float64
	Course     float64
	HasCourse  bool
	Age        time.Duration
}

// PositionSource returns the current live fix. ok=false means "no live
// position available"; the beacon then uses the static StationInfo lat/lon.
type PositionSource func() (LiveFix, bool)

// knotsToMPH converts knots to statute mph. Duplicated from internal/gps
// (rather than imported) to keep this package's dependency graph shallow.
const knotsToMPH = 1.15077945

// minTurnInterval is the minimum time between two turn-triggered smart
// beacons, even if the heading change threshold is exceeded again quickly.
const minTurnInterval = 15 * time.Second

// Manager handles periodic beaconing.
type Manager struct {
	cfg     Config
	station StationInfo
	send    SendFunc

	mu              sync.Mutex
	running         bool
	cancel          context.CancelFunc
	posSource       PositionSource
	lastCourse      float64
	lastCourseValid bool
	lastBeaconAt    time.Time
}

// New creates a new beacon manager.
func New(cfg Config, station StationInfo, send SendFunc) *Manager {
	return &Manager{
		cfg:     cfg,
		station: station,
		send:    send,
	}
}

// Start begins the beaconing timer loop.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}

	ctx, m.cancel = context.WithCancel(ctx)
	m.running = true
	go m.loop(ctx)
}

// Stop stops beaconing.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.cancel()
	m.running = false
}

// BeaconNow triggers an immediate beacon transmission. It also resets the
// smart-beacon timer, the same as a periodic beacon would.
func (m *Manager) BeaconNow() error {
	m.mu.Lock()
	hasSend := m.send != nil
	m.mu.Unlock()
	if !hasSend {
		return fmt.Errorf("no send function configured")
	}
	return m.sendBeacon(time.Now())
}

// IsRunning returns whether beaconing is active.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// UpdateConfig updates the beacon configuration under mutex.
func (m *Manager) UpdateConfig(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// UpdateStationInfo updates the station identity for beacon frames.
func (m *Manager) UpdateStationInfo(info StationInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.station = info
}

// SetPositionSource installs (or clears, with nil) the live-position hook.
func (m *Manager) SetPositionSource(fn PositionSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.posSource = fn
}

// loop runs the beacon timer: a fixed interval when smart beaconing is off
// (or no live fix is available), or GPS-driven rate/turn triggers when it's
// on and fed by a fresh live fix.
func (m *Manager) loop(ctx context.Context) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	// Send an initial beacon immediately.
	m.mu.Lock()
	hasSend := m.send != nil
	m.mu.Unlock()
	if hasSend {
		m.sendBeacon(time.Now())
	}

	const tick = 1 * time.Second
	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.mu.Lock()
			hasSend := m.send != nil
			m.mu.Unlock()
			if hasSend && m.shouldBeacon(time.Now()) {
				m.sendBeacon(time.Now())
			}
		}
	}
}

// shouldBeacon decides, on each 1s tick, whether it's time for another
// beacon. Without smart beaconing (or without a fresh live fix), it fires on
// the fixed cfg.Interval. With a fresh fix, it fires on the GPS-driven rate
// (faster while moving fast) or on a heading change past the speed-scaled
// turn threshold, whichever comes first — never more often than
// minTurnInterval for a turn-triggered beacon.
func (m *Manager) shouldBeacon(now time.Time) bool {
	m.mu.Lock()
	interval := m.cfg.Interval
	sb := m.cfg.SmartBeacon
	posFn := m.posSource
	maxAge := m.cfg.LiveMaxAge
	lastBeaconAt := m.lastBeaconAt
	lastCourse := m.lastCourse
	lastCourseValid := m.lastCourseValid
	m.mu.Unlock()

	if maxAge <= 0 {
		maxAge = 30 * time.Second
	}
	if lastBeaconAt.IsZero() {
		return true
	}
	elapsed := now.Sub(lastBeaconAt)

	if sb == nil || !sb.Enabled || posFn == nil {
		return elapsed >= interval
	}
	fix, ok := posFn()
	if !ok || fix.Age > maxAge {
		return elapsed >= interval
	}

	mph := fix.SpeedKnots * knotsToMPH
	if elapsed >= sb.Rate(mph) {
		return true
	}
	if lastCourseValid && fix.HasCourse && elapsed >= minTurnInterval {
		if angleDelta(fix.Course, lastCourse) >= sb.TurnThreshold(mph) {
			return true
		}
	}
	return false
}

// angleDelta returns the smallest absolute difference between two bearings,
// 0-180 degrees.
func angleDelta(a, b float64) float64 {
	d := math.Mod(a-b, 360)
	if d < 0 {
		d += 360
	}
	if d > 180 {
		d = 360 - d
	}
	return d
}

// sendBeacon builds the current frame, transmits it, and records the
// smart-beacon state (lastBeaconAt, and lastCourse when the frame included
// a moving live fix) used by the next shouldBeacon decision.
func (m *Manager) sendBeacon(now time.Time) error {
	frame, course, moving := m.buildFrameDetailed()

	m.mu.Lock()
	send := m.send
	m.mu.Unlock()

	var err error
	if send != nil {
		err = send(frame)
	}

	m.mu.Lock()
	m.lastBeaconAt = now
	if moving {
		m.lastCourse = course
		m.lastCourseValid = true
	}
	m.mu.Unlock()

	return err
}

// buildFrame creates an APRS position report frame for this station.
func (m *Manager) buildFrame() aprs.APRSFrame {
	frame, _, _ := m.buildFrameDetailed()
	return frame
}

// buildFrameDetailed builds the frame and also reports the course used and
// whether the frame represents a moving live fix (for smart-beacon state).
// Snapshots config and station info under mutex to avoid races.
func (m *Manager) buildFrameDetailed() (aprs.APRSFrame, float64, bool) {
	m.mu.Lock()
	station := m.station
	comment := m.cfg.Comment
	path := m.cfg.Path
	maxAge := m.cfg.LiveMaxAge
	posFn := m.posSource
	m.mu.Unlock()

	if maxAge <= 0 {
		maxAge = 30 * time.Second
	}

	lat, lon := station.Lat, station.Lon
	var course, speed float64
	var moving bool
	if posFn != nil {
		if fix, ok := posFn(); ok && fix.Age <= maxAge {
			lat, lon = fix.Lat, fix.Lon
			if fix.HasCourse && fix.SpeedKnots >= 1.0 {
				course, speed, moving = fix.Course, fix.SpeedKnots, true
			}
		}
	}

	symTable := station.SymbolTable
	symCode := station.SymbolCode
	if symTable == "" {
		symTable = "/"
	}
	if symCode == "" {
		symCode = "-"
	}

	if path == nil {
		path = aprs.DefaultRFPath()
	} else {
		path = append([]aprs.Address(nil), path...)
	}

	payload := "!" + FormatLat(lat) + symTable + FormatLon(lon) + symCode
	if moving {
		payload += FormatCourseSpeed(course, speed)
	}
	payload += comment

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: station.Callsign, SSID: station.SSID},
		Destination: aprs.Address{Call: "APNMRA"},
		Path:        path,
		Payload:     payload,
	}
	return frame, course, moving
}

// FormatCourseSpeed renders the APRS 7-character CSE/SPD extension
// "CCC/SSS" — course degrees true (000 is encoded as 360 per APRS101),
// speed in knots, both zero-padded and clamped to 999.
func FormatCourseSpeed(course, speedKnots float64) string {
	c := math.Mod(course, 360)
	if c < 0 {
		c += 360
	}
	c = math.Round(c)
	ci := int(c)
	if ci <= 0 || ci >= 360 {
		ci = 360
	}

	s := math.Round(speedKnots)
	if s < 0 {
		s = 0
	}
	if s > 999 {
		s = 999
	}

	return fmt.Sprintf("%03d/%03d", ci, int(s))
}

// FormatLat converts decimal latitude to APRS format "DDMM.hhN".
func FormatLat(lat float64) string {
	hemi := 'N'
	if lat < 0 {
		hemi = 'S'
		lat = -lat
	}
	deg := int(lat)
	min := (lat - float64(deg)) * 60
	return fmt.Sprintf("%02d%05.2f%c", deg, math.Abs(min), hemi)
}

// FormatLon converts decimal longitude to APRS format "DDDMM.hhW".
func FormatLon(lon float64) string {
	hemi := 'E'
	if lon < 0 {
		hemi = 'W'
		lon = -lon
	}
	deg := int(lon)
	min := (lon - float64(deg)) * 60
	return fmt.Sprintf("%03d%05.2f%c", deg, math.Abs(min), hemi)
}
