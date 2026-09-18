package wxalert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"
)

// StateFor is the single source of truth for the four link-state words every
// UI surface uses: off (disabled), live (<=3m since last success), stale
// (3-15m), down (>15m or never succeeded).
func StateFor(enabled bool, lastOK time.Time, now time.Time) LinkState {
	if !enabled {
		return LinkOff
	}
	if lastOK.IsZero() {
		return LinkDown
	}
	age := now.Sub(lastOK)
	switch {
	case age <= 3*time.Minute:
		return LinkLive
	case age <= 15*time.Minute:
		return LinkStale
	default:
		return LinkDown
	}
}

// PollSink is what the Manager implements; the poller knows nothing about
// matching or storage.
type PollSink interface {
	// OnAlerts is called after a full fetch with the whole current active
	// set for the query (complete=true: absent ids may be considered
	// dropped by whatever tracks state — the Registry).
	OnAlerts(alerts []Alert, fetchedAt time.Time, complete bool)
	OnLinkStatus(LinkStatus)
}

// PollerConfig configures a Poller.
type PollerConfig struct {
	BaseURL        string
	UserAgent      string        // required, must include a parenthesized contact
	Client         *http.Client  // optional; a default is used if nil
	Interval       time.Duration // default 60s
	Timeout        time.Duration // default 20s (set on Client if Client is nil)
	ForceFullEvery time.Duration // default 5m
	ForceFullHot   time.Duration // default 2m — used while any IN warning-tier alert is active
	MaxBackoff     time.Duration // default 10m
	Sounds         bool          // mirrored onto LinkStatus
}

// PollResult is PollOnce's outcome, mainly useful to tests and to
// POST /wx/refresh's synchronous caller.
type PollResult struct {
	Changed bool
	Skipped string // "off" | "empty footprint" | "backoff" | ""
	Err     error
}

// Poller drives NWS polling: cadence, count-signature short-circuiting, a
// forced full fetch on a timer, and exponential backoff on failure. It knows
// nothing about matching, notification or storage — that is PollSink's job.
type Poller struct {
	provider Provider
	cfg      PollerConfig
	sink     PollSink
	clock    func() time.Time
	events   chan Event

	mu            sync.Mutex
	enabled       bool
	query         ActiveQuery
	watchedAreas  []string
	hot           bool
	lastSignature string
	lastBodyHash  string
	lastFullAt    time.Time
	lastOK        time.Time
	lastAttempt   time.Time
	nextAttempt   time.Time
	lastErr       string
	failures      int
	fromCache     bool
	activeInFeed  int
	pollNow       bool
	noWatchArea   bool
	linkState     LinkState
	downSince     time.Time
}

// New constructs a Poller. UserAgent must include a parenthesized contact
// (NWS terms); an empty or contact-less UserAgent is a configuration error.
func New(cfg PollerConfig, sink PollSink) (*Poller, error) {
	client := cfg.Client
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 20 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	provider, err := NewNWSProvider(NWSConfig{BaseURL: cfg.BaseURL, UserAgent: cfg.UserAgent, Client: client})
	if err != nil {
		return nil, err
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.ForceFullEvery <= 0 {
		cfg.ForceFullEvery = 5 * time.Minute
	}
	if cfg.ForceFullHot <= 0 {
		cfg.ForceFullHot = 2 * time.Minute
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 10 * time.Minute
	}
	return &Poller{
		provider: provider, cfg: cfg, sink: sink, clock: time.Now,
		events: make(chan Event, 16), enabled: true, fromCache: true, linkState: LinkOff,
	}, nil
}

// NewWithProvider is New's counterpart for tests that want to inject a fake
// Provider directly instead of going through NWSProvider/httptest.
func NewWithProvider(p Provider, cfg PollerConfig, sink PollSink) *Poller {
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.ForceFullEvery <= 0 {
		cfg.ForceFullEvery = 5 * time.Minute
	}
	if cfg.ForceFullHot <= 0 {
		cfg.ForceFullHot = 2 * time.Minute
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 10 * time.Minute
	}
	return &Poller{provider: p, cfg: cfg, sink: sink, clock: time.Now, events: make(chan Event, 16), enabled: true, fromCache: true, linkState: LinkOff}
}

// Events returns the poller's event channel (currently unused by the poller
// itself; reserved so callers can select on it alongside a ZoneCache's).
func (p *Poller) Events() <-chan Event { return p.events }

// SetClock injects a time source for tests.
func (p *Poller) SetClock(now func() time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clock = now
}

// SetEnabled toggles polling. Disabling immediately reports state "off".
func (p *Poller) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
}

// SetQuery updates the area filter and whether the footprint currently has
// an IN warning-tier alert active (which shortens the forced-full-fetch
// interval from ForceFullEvery to ForceFullHot).
func (p *Poller) SetQuery(q ActiveQuery, hot bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.query = q
	p.watchedAreas = append([]string{}, q.Areas...)
	p.hot = hot
}

// PollNow requests that the next PollOnce/Run iteration performs a full
// fetch regardless of the count signature or backoff timer.
func (p *Poller) PollNow() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pollNow = true
}

// Status reports the current link status.
func (p *Poller) Status() LinkStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.statusLocked()
}

func (p *Poller) statusLocked() LinkStatus {
	now := p.now()
	state := StateFor(p.enabled, p.lastOK, now)
	var reason LinkReason
	if p.enabled && p.noWatchArea {
		// Enabled, but the query is empty — report "off" with a reason rather
		// than live (nothing is fetched) or down (nothing is broken).
		state = LinkOff
		reason = ReasonNoWatchArea
	}
	st := LinkStatus{
		State: state, Reason: reason, Enabled: p.enabled, ContactConfigured: true,
		ConsecutiveFailures: p.failures, FromCache: p.fromCache,
		RegionCount: p.activeInFeed, Sounds: p.cfg.Sounds,
	}
	if !p.lastOK.IsZero() {
		t := p.lastOK
		st.LastSuccessAt = &t
	}
	if !p.lastAttempt.IsZero() {
		t := p.lastAttempt
		st.LastAttemptAt = &t
	}
	if !p.nextAttempt.IsZero() {
		t := p.nextAttempt
		st.NextAttemptAt = &t
	}
	st.LastError = p.lastErr
	return st
}

func (p *Poller) now() time.Time {
	if p.clock != nil {
		return p.clock()
	}
	return time.Now()
}

// PollOnce runs one tick of the poll algorithm synchronously.
func (p *Poller) PollOnce(ctx context.Context) (PollResult, error) {
	p.mu.Lock()
	enabled := p.enabled
	now := p.now()
	pollNowRequested := p.pollNow
	nextAttempt := p.nextAttempt
	query := p.query
	watchedAreas := p.watchedAreas
	hot := p.hot
	p.mu.Unlock()

	if !enabled {
		return PollResult{Skipped: "off"}, nil
	}
	if len(query.Areas) == 0 && query.Point == nil {
		// Nothing was fetched, so this is NOT a successful poll — leaving
		// lastOK alone is the whole point. See ReasonNoWatchArea.
		p.mu.Lock()
		p.noWatchArea = true
		p.mu.Unlock()
		p.emitStatus()
		return PollResult{Skipped: "empty footprint"}, nil
	}
	p.mu.Lock()
	p.noWatchArea = false
	p.mu.Unlock()
	if !pollNowRequested && !nextAttempt.IsZero() && now.Before(nextAttempt) {
		return PollResult{Skipped: "backoff"}, nil
	}

	p.mu.Lock()
	p.pollNow = false
	p.lastAttempt = now
	p.mu.Unlock()

	count, err := p.provider.FetchCount(ctx, query)
	if err != nil {
		return p.handleFailure(err), err
	}

	sig := CountSignature(count, watchedAreas)
	forceEvery := p.cfg.ForceFullEvery
	if hot {
		forceEvery = p.cfg.ForceFullHot
	}

	p.mu.Lock()
	needFull := pollNowRequested || sig != p.lastSignature || p.lastFullAt.IsZero() || now.Sub(p.lastFullAt) >= forceEvery
	p.mu.Unlock()

	if !needFull {
		p.mu.Lock()
		p.lastOK = now
		p.failures = 0
		p.lastErr = ""
		p.nextAttempt = time.Time{}
		p.mu.Unlock()
		p.emitStatus()
		return PollResult{Changed: false}, nil
	}

	decoded, body, err := p.provider.FetchActive(ctx, query)
	if err != nil {
		return p.handleFailure(err), err
	}

	bodyHash := hashBody(body)
	p.mu.Lock()
	changed := bodyHash != p.lastBodyHash
	p.lastSignature = sig
	p.lastBodyHash = bodyHash
	p.lastFullAt = now
	p.lastOK = now
	p.failures = 0
	p.lastErr = ""
	p.nextAttempt = time.Time{}
	p.fromCache = false
	p.activeInFeed = len(decoded.Alerts)
	p.mu.Unlock()

	p.sink.OnAlerts(decoded.Alerts, now, true)
	p.emitStatus()
	return PollResult{Changed: changed}, nil
}

func hashBody(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

func (p *Poller) handleFailure(err error) PollResult {
	p.mu.Lock()
	now := p.now()
	p.failures++
	p.lastErr = plainWordsFor(err)

	var delay time.Duration
	var rle *RateLimitError
	if errors.As(err, &rle) {
		// Use the server's Retry-After exactly (nws.go already defaults it
		// to 30s when the header is absent) — no additional flooring here.
		delay = rle.RetryAfter
	} else {
		delay = p.cfg.Interval
		for i := 1; i < p.failures; i++ {
			delay *= 2
			if delay >= p.cfg.MaxBackoff {
				delay = p.cfg.MaxBackoff
				break
			}
		}
	}
	p.nextAttempt = now.Add(delay)
	if StateFor(p.enabled, p.lastOK, now) == LinkDown && p.downSince.IsZero() {
		p.downSince = now
	}
	p.mu.Unlock()

	p.emitStatus()
	return PollResult{Err: err}
}

func plainWordsFor(err error) string {
	var rle *RateLimitError
	if errors.As(err, &rle) {
		return rle.Error()
	}
	switch {
	case errors.Is(err, ErrForbidden):
		return ErrForbidden.Error()
	case errors.Is(err, ErrTimeout):
		return "timed out"
	case errors.Is(err, ErrMalformed):
		return err.Error()
	case errors.Is(err, ErrUpstream):
		return err.Error()
	default:
		return err.Error()
	}
}

func (p *Poller) emitStatus() {
	st := p.Status()
	p.sink.OnLinkStatus(st)
}

// Run polls in a loop until ctx is cancelled, sleeping for either the
// backoff delay (on failure) or Interval (on success/skip) between ticks.
// It wakes early on PollNow.
func (p *Poller) Run(ctx context.Context) {
	for {
		res, _ := p.PollOnce(ctx)
		delay := p.cfg.Interval
		if res.Skipped == "backoff" {
			p.mu.Lock()
			delay = p.nextAttempt.Sub(p.now())
			p.mu.Unlock()
			if delay < 0 {
				delay = 0
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}
