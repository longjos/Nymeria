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
	Enabled     bool          `yaml:"enabled"`
	Interval    time.Duration `yaml:"interval"`      // Fixed interval (default 10min)
	Comment     string        `yaml:"comment"`        // Beacon comment text
	SmartBeacon *SmartConfig  `yaml:"smart_beacon"`   // nil = disabled
}

// SmartConfig holds smart beaconing parameters.
type SmartConfig struct {
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

// Manager handles periodic beaconing.
type Manager struct {
	cfg     Config
	station StationInfo
	send    SendFunc

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
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

// BeaconNow triggers an immediate beacon transmission.
func (m *Manager) BeaconNow() error {
	if m.send == nil {
		return fmt.Errorf("no send function configured")
	}
	frame := m.buildFrame()
	return m.send(frame)
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

// loop runs the periodic beacon timer.
func (m *Manager) loop(ctx context.Context) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	// Send an initial beacon immediately
	if m.send != nil {
		m.send(m.buildFrame())
	}

	ticker := time.NewTicker(m.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.send != nil {
				m.send(m.buildFrame())
			}
		}
	}
}

// buildFrame creates an APRS position report frame for this station.
// Snapshots config and station info under mutex to avoid races.
func (m *Manager) buildFrame() aprs.APRSFrame {
	m.mu.Lock()
	station := m.station
	comment := m.cfg.Comment
	m.mu.Unlock()

	symTable := station.SymbolTable
	symCode := station.SymbolCode
	if symTable == "" {
		symTable = "/"
	}
	if symCode == "" {
		symCode = "-"
	}

	payload := "!" + FormatLat(station.Lat) + symTable + FormatLon(station.Lon) + symCode + comment

	return aprs.APRSFrame{
		Source:      aprs.Address{Call: station.Callsign, SSID: station.SSID},
		Destination: aprs.Address{Call: "APNMRA"},
		Path: []aprs.Address{
			{Call: "WIDE1", SSID: 1},
			{Call: "WIDE2", SSID: 1},
		},
		Payload: payload,
	}
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
