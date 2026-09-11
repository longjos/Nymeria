package gps

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Config configures the live GPS manager.
type Config struct {
	Enabled      bool
	Type         string // "gpsd" | "nmea"
	Host         string
	Port         int
	Device       string
	Baud         int
	MinInterval  time.Duration // drop fixes arriving faster than this; 0 = 1s
	StaleAfter   time.Duration // fixes older than this are not "fresh"; 0 = 30s
	UseForBeacon bool
}

func (c Config) minInterval() time.Duration {
	if c.MinInterval <= 0 {
		return 1 * time.Second
	}
	return c.MinInterval
}

func (c Config) staleAfter() time.Duration {
	if c.StaleAfter <= 0 {
		return 30 * time.Second
	}
	return c.StaleAfter
}

func (c Config) target() string {
	if c.Type == "nmea" && c.Device != "" {
		baud := c.Baud
		if baud <= 0 {
			baud = 9600
		}
		return fmt.Sprintf("%s@%d", c.Device, baud)
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Status is the JSON-serializable snapshot returned by the REST/WS bridge.
type Status struct {
	Enabled   bool   `json:"enabled"`
	Type      string `json:"type,omitempty"`
	Target    string `json:"target,omitempty"`
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
	Fix       *Fix   `json:"fix"`       // nil when never fixed
	AgeMillis *int64 `json:"ageMillis"` // nil when Fix is nil
	Stale     bool   `json:"stale"`     // Fix==nil || age > StaleAfter
}

// sourceFactory builds the Source implied by a Config. A field on Manager
// (not a package-level var) so tests can substitute a counting fake.
type sourceFactory func(Config) Source

func defaultSourceFactory(cfg Config) Source {
	if cfg.Type == "nmea" {
		return NewNMEASource(NMEAConfig{Device: cfg.Device, Baud: cfg.Baud, Host: cfg.Host, Port: cfg.Port})
	}
	return NewGPSDSource(GPSDConfig{Host: cfg.Host, Port: cfg.Port})
}

// Manager owns one live GPS Source, applies acceptance/rate-limiting rules,
// and fans accepted fixes out to subscribers.
type Manager struct {
	factory sourceFactory

	mu      sync.Mutex
	cfg     Config
	src     Source
	baseCtx context.Context
	cancel  context.CancelFunc
	started bool

	haveFix        bool
	lastFix        Fix
	lastAcceptedAt time.Time

	subMu sync.Mutex
	subs  map[chan Fix]struct{}
}

// NewManager creates a Manager that builds its Source from cfg via the
// default factory (gpsd or nmea, per cfg.Type).
func NewManager(cfg Config) *Manager {
	return &Manager{
		cfg:     cfg,
		factory: defaultSourceFactory,
		subs:    make(map[chan Fix]struct{}),
	}
}

// NewManagerWithSource creates a Manager bound to a pre-built Source —
// a test seam for driving fixtures directly through the acceptance logic.
func NewManagerWithSource(cfg Config, src Source) *Manager {
	m := NewManager(cfg)
	m.src = src
	m.factory = func(Config) Source { return src }
	return m
}

// Start is a no-op when !cfg.Enabled. Safe to call twice.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Enabled {
		return nil
	}
	if m.started {
		return nil
	}
	m.baseCtx = ctx
	return m.startLocked()
}

// startLocked assumes m.mu is held, m.cfg.Enabled, !m.started, and m.baseCtx set.
func (m *Manager) startLocked() error {
	if m.src == nil {
		m.src = m.factory(m.cfg)
	}
	runCtx, cancel := context.WithCancel(m.baseCtx)
	m.cancel = cancel
	if err := m.src.Start(runCtx); err != nil {
		return err
	}
	m.started = true
	go m.pump(m.src.Fixes())
	return nil
}

// Stop halts the source's reconnect loop. Safe to call when not started.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	if m.src != nil {
		_ = m.src.Close()
	}
	m.started = false
}

func (m *Manager) pump(fixes <-chan Fix) {
	for fix := range fixes {
		m.ingest(fix)
	}
}

// ingest applies the acceptance rules and, if the fix is kept, stores it and
// fans it out to subscribers.
func (m *Manager) ingest(fix Fix) {
	m.mu.Lock()
	if m.haveFix {
		modeChanged := fix.Mode != m.lastFix.Mode
		if !modeChanged {
			if fix.ReceivedAt.Sub(m.lastAcceptedAt) < m.cfg.minInterval() {
				m.mu.Unlock()
				return
			}
			if !fix.HasPosition() && !m.lastFix.HasPosition() {
				m.mu.Unlock()
				return
			}
		}
	}

	m.lastFix = fix
	m.haveFix = true
	m.lastAcceptedAt = fix.ReceivedAt
	m.mu.Unlock()

	m.fanout(fix)
}

func (m *Manager) fanout(fix Fix) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for ch := range m.subs {
		select {
		case ch <- fix:
		default:
			// Slow subscriber: drop rather than block ingest.
		}
	}
}

// Current returns the last accepted fix. ok is false when there has never
// been one, or GPS is disabled.
func (m *Manager) Current() (Fix, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Enabled || !m.haveFix {
		return Fix{}, false
	}
	return m.lastFix, true
}

// Fresh returns the last fix only when it is newer than StaleAfter.
func (m *Manager) Fresh(now time.Time) (Fix, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Enabled || !m.haveFix {
		return Fix{}, false
	}
	if now.Sub(m.lastFix.ReceivedAt) > m.cfg.staleAfter() {
		return Fix{}, false
	}
	return m.lastFix, true
}

// Status returns a JSON-serializable snapshot of GPS state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.cfg.Enabled {
		return Status{Enabled: false, Connected: false, Fix: nil, AgeMillis: nil, Stale: true}
	}

	var srcStatus SourceStatus
	if m.src != nil {
		srcStatus = m.src.Status()
	}

	st := Status{
		Enabled:   true,
		Type:      m.cfg.Type,
		Target:    m.cfg.target(),
		Connected: srcStatus.Connected,
		Error:     srcStatus.Error,
	}

	if m.haveFix {
		fixCopy := m.lastFix
		st.Fix = &fixCopy
		age := time.Since(m.lastFix.ReceivedAt)
		ms := age.Milliseconds()
		st.AgeMillis = &ms
		st.Stale = age > m.cfg.staleAfter()
	} else {
		st.Stale = true
	}

	return st
}

// Subscribe returns a buffered (cap 8) channel of accepted fixes and an
// unsubscribe func. A slow subscriber drops fixes; it is never blocked on.
func (m *Manager) Subscribe() (<-chan Fix, func()) {
	ch := make(chan Fix, 8)
	m.subMu.Lock()
	m.subs[ch] = struct{}{}
	m.subMu.Unlock()

	unsub := func() {
		m.subMu.Lock()
		if _, ok := m.subs[ch]; ok {
			delete(m.subs, ch)
			close(ch)
		}
		m.subMu.Unlock()
	}
	return ch, unsub
}

// UpdateConfig applies a new config. When Type/Host/Port/Device/Baud/Enabled
// changed it tears the source down and rebuilds it; otherwise it only
// updates the thresholds (MinInterval/StaleAfter/UseForBeacon).
func (m *Manager) UpdateConfig(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	old := m.cfg
	targetChanged := old.Type != cfg.Type ||
		old.Host != cfg.Host ||
		old.Port != cfg.Port ||
		old.Device != cfg.Device ||
		old.Baud != cfg.Baud ||
		old.Enabled != cfg.Enabled

	m.cfg = cfg
	if !targetChanged {
		return
	}

	wasStarted := m.started
	if m.cancel != nil {
		m.cancel()
	}
	if m.src != nil {
		_ = m.src.Close()
	}
	m.src = nil
	m.started = false
	m.haveFix = false

	if cfg.Enabled && wasStarted && m.baseCtx != nil {
		_ = m.startLocked()
	}
}
