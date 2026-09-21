package wxalert

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Config is the Manager's own configuration (wxalert never imports
// internal/config; internal/app adapts config.WxAlertsConfig into this).
type Config struct {
	Enabled            bool
	UserAgent          string
	BaseURL            string
	PollInterval       time.Duration
	ZoneDataDir        string
	DefaultBufferMiles float64
	InterruptEvents    []string
	InterruptCustom    bool
	WatchNotify        NotifyClass
	AdvisoryNotify     NotifyClass
	StatementNotify    NotifyClass
	MuteAdvisories     bool
	Sounds             bool
}

func (c Config) policy() Policy {
	p := Policy{
		InterruptEvents: c.InterruptEvents,
		WatchNotify:     c.WatchNotify,
		AdvisoryNotify:  c.AdvisoryNotify,
		StatementNotify: c.StatementNotify,
		MuteAdvisories:  c.MuteAdvisories,
	}
	if len(p.InterruptEvents) == 0 {
		p.InterruptEvents = DefaultInterruptEvents
	}
	if p.WatchNotify == "" {
		p.WatchNotify = NotifyToast
	}
	if p.AdvisoryNotify == "" {
		p.AdvisoryNotify = NotifyBadge
	}
	if p.StatementNotify == "" {
		p.StatementNotify = NotifyPanel
	}
	return p
}

// Manager wires the poller, registry, zone cache and footprint into one
// coherent notification pipeline. It is deliberately single-footprint in
// WP1 (SetFootprintInputs replaces the whole watch area); per-net
// multi-tenancy is internal/app/internal/server wiring in a later work
// package, built on the same primitives.
type Manager struct {
	mu        sync.Mutex
	cfg       Config
	policy    Policy
	poller    *Poller
	registry  *Registry
	zones     *ZoneCache
	footprint *Footprint
	clock     func() time.Time
	events    chan Event
	netID     string
	lastErr   error
}

// NewManager builds a Manager from cfg. When cfg.Enabled is false (or the
// contact is missing) the poller is left nil and every poll-dependent call
// is a documented no-op — the manager still answers Snapshot/Get/Zone from
// whatever the registry/zone cache already hold.
func NewManager(cfg Config) (*Manager, error) {
	m := &Manager{
		cfg: cfg, policy: cfg.policy(), registry: NewRegistry(),
		clock: time.Now, events: make(chan Event, 64),
	}
	if cfg.Enabled {
		poller, err := New(PollerConfig{
			BaseURL: cfg.BaseURL, UserAgent: cfg.UserAgent, Interval: cfg.PollInterval, Sounds: cfg.Sounds,
		}, m)
		if err != nil {
			return nil, fmt.Errorf("wxalert: %w", err)
		}
		m.poller = poller
	}
	if cfg.ZoneDataDir != "" {
		var provider ZoneProvider
		if cfg.Enabled {
			if p, err := NewNWSProvider(NWSConfig{BaseURL: cfg.BaseURL, UserAgent: cfg.UserAgent}); err == nil {
				provider = p
			}
		}
		zc, err := NewZoneCache(ZoneCacheConfig{DataDir: cfg.ZoneDataDir}, provider)
		if err != nil {
			return nil, fmt.Errorf("wxalert: %w", err)
		}
		m.zones = zc
	}
	m.footprint = Build(Inputs{BufferMiles: cfg.DefaultBufferMiles, Now: m.clock()})
	return m, nil
}

// SetClock injects a time source for tests.
func (m *Manager) SetClock(now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clock = now
	if m.poller != nil {
		m.poller.SetClock(now)
	}
}

// Start runs the poller loop until ctx is cancelled. It is a no-op when the
// manager was built with Enabled=false.
func (m *Manager) Start(ctx context.Context) {
	if m.poller != nil {
		go m.poller.Run(ctx)
	}
}

// PollNow requests an immediate out-of-cycle poll (POST /wx/refresh).
func (m *Manager) PollNow() {
	if m.poller != nil {
		m.poller.PollNow()
	}
}

// SetQuery narrows the poller's active-alerts fetch to q (state area codes
// or a single point — see ActiveQuery), and whether it should poll at the
// hot cadence. WP2's app/server wiring calls this whenever the watch
// footprint changes, so a hotspot deployment fetches a regionally-filtered
// feed (tens of KB) instead of the unfiltered nationwide one. A no-op when
// the manager has no poller (Enabled=false).
func (m *Manager) SetQuery(q ActiveQuery, hot bool) {
	if m.poller != nil {
		m.poller.SetQuery(q, hot)
	}
}

// Zones exposes the manager's zone cache so callers can search it (GET
// /wx/zones/search) and read its disk status, beyond the single-UGC Get
// already proxied by Zone. Nil when ZoneDataDir was never configured.
func (m *Manager) Zones() *ZoneCache {
	return m.zones
}

// Events returns the manager's WebSocket-bridge event channel.
func (m *Manager) Events() <-chan Event { return m.events }

// EventTypes returns the 111 canonical event names plus the two
// damage-threat-derived emergencies, each tagged with its tier, for the
// allowlist picker (GET /wx/event-types).
func (m *Manager) EventTypes() []EventType {
	names := KnownEvents()
	out := make([]EventType, 0, len(names)+2)
	for _, n := range names {
		tier, _ := TierForEvent(n)
		out = append(out, EventType{Event: n, Tier: tier})
	}
	for _, n := range []string{"Tornado Emergency", "Flash Flood Emergency"} {
		tier, _ := TierForEvent(n)
		out = append(out, EventType{Event: n, Tier: tier})
	}
	return out
}

// UpdateConfig applies a new configuration live (no restart): the policy
// takes effect on the next recompute, and the poller (if any) picks up a new
// interval/UserAgent lazily — WP2's app wiring rebuilds the Manager on a
// contact/enabled change since those affect the underlying HTTP client.
func (m *Manager) UpdateConfig(cfg Config) {
	m.mu.Lock()
	m.cfg = cfg
	m.policy = cfg.policy()
	m.mu.Unlock()
	m.recomputeAll()
}

// SetFootprintInputs rebuilds the watch footprint and re-classifies every
// tracked alert against it. Callers (the Manager's future app-level driver)
// call this whenever annotations, roster positions, GPS or the per-net
// buffer/extra-zones settings change.
func (m *Manager) SetFootprintInputs(in Inputs) {
	m.mu.Lock()
	if in.Now.IsZero() {
		in.Now = m.clock()
	}
	m.footprint = Build(in)
	m.netID = in.NetID
	m.mu.Unlock()
	m.recomputeAll()
}

// Footprint returns the current footprint summary.
func (m *Manager) Footprint() FootprintSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.footprint.Summary()
}

// Link reports the poller's link status (LinkOff with Enabled=false when the
// manager has no poller at all).
func (m *Manager) Link() LinkStatus {
	if m.poller == nil {
		return m.withCounts(LinkStatus{State: LinkOff})
	}
	return m.withCounts(m.poller.Status())
}

// withCounts fills the four provenance counters. The poller can't: it only
// sees the raw feed, while these describe the matched set and the zone cache.
// They were declared on LinkStatus and never assigned anywhere, so every
// surface that showed them ("2 in watch area · zones cached 5") reported 0
// while alerts were on screen.
func (m *Manager) withCounts(st LinkStatus) LinkStatus {
	for _, a := range m.registry.Active() {
		switch a.Proximity {
		case ProximityIn:
			st.InAreaCount++
		case ProximityNear:
			st.NearbyCount++
		}
	}

	m.mu.Lock()
	fp := m.footprint
	m.mu.Unlock()
	if fp == nil {
		return st
	}
	for _, z := range fp.Summary().Zones {
		if m.zones != nil && m.zones.Has(z.UGC) {
			st.ZonesCached++
		} else {
			st.ZonesMissing++
		}
	}
	return st
}

// Policy returns the effective policy as served on the wire.
func (m *Manager) Policy() EffectivePolicy {
	m.mu.Lock()
	pol := m.policy
	netID := m.netID
	buffer := m.cfg.DefaultBufferMiles
	custom := m.cfg.InterruptCustom
	m.mu.Unlock()
	return EffectivePolicy{
		NetID: netID, BufferMiles: buffer, InterruptEvents: pol.InterruptEvents, InterruptCustom: custom,
		WatchNotify: string(pol.WatchNotify), AdvisoryNotify: string(pol.AdvisoryNotify),
		StatementNotify: string(pol.StatementNotify), MuteAdvisories: pol.MuteAdvisories, FloorText: FloorText(),
	}
}

// Snapshot returns the full current payload (GET /wx/alerts and every
// wx_alerts WS event use this).
func (m *Manager) Snapshot() Snapshot {
	alerts := m.relevantAlerts()
	fp := m.Footprint()
	return Snapshot{Alerts: alerts, Status: m.Link(), Footprint: &fp, Policy: m.Policy()}
}

// relevantAlerts is every IN/NEAR active alert, plus anything ended within
// the last hour, sorted per BUILD-PLAN §4.5.
func (m *Manager) relevantAlerts() []MatchedAlert {
	out := make([]MatchedAlert, 0)
	now := m.clock()
	for _, a := range m.registry.Active() {
		if a.Proximity == ProximityIn || a.Proximity == ProximityNear {
			out = append(out, a)
		}
	}
	for _, er := range m.registry.Ended() {
		if now.Sub(er.At) <= time.Hour {
			out = append(out, er.Alert)
		}
	}
	sortMatchedAlerts(out)
	return out
}

// Get returns one alert by id (active or recently ended), or nil.
func (m *Manager) Get(id string) *MatchedAlert {
	if a := m.registry.Get(id); a != nil {
		return a
	}
	for _, er := range m.registry.Ended() {
		if er.Alert.ID == id {
			a := er.Alert
			return &a
		}
	}
	return nil
}

// History returns id's supersession chain, newest predecessor first.
func (m *Manager) History(id string) []MatchedAlert { return m.registry.History(id) }

// AcknowledgeLocal marks an alert acknowledged on this device only — the
// wire has no per-user ack state server-side (decision 5 covers only the
// NCS "ack for net" broadcast); this exists so the Manager's activity log
// hook (wired in WP2) has something to call.
func (m *Manager) AcknowledgeLocal(id string) error {
	if m.Get(id) == nil {
		return fmt.Errorf("wxalert: alert %s not found", id)
	}
	return nil
}

// AckForNet records the NCS "ack for net" — this clears the interrupt/toast
// banner for every operator on the net (user decision 5), which is why the
// WS broadcast this triggers, not a per-user flag, is what the frontend
// listens for.
func (m *Manager) AckForNet(id string, ack NetAck) (*MatchedAlert, error) {
	if ack.At.IsZero() {
		ack.At = m.clock()
	}
	if !m.registry.SetNetAck(id, ack) {
		return nil, fmt.Errorf("wxalert: alert %s not found", id)
	}
	a := m.Get(id)
	m.events <- Event{Type: EventAlertAckNet, Data: map[string]any{"id": id, "netId": m.netID, "ackedForNet": ack}}
	return a, nil
}

// Zone proxies to the zone cache, if configured.
func (m *Manager) Zone(ctx context.Context, ugc string, allowFetch bool) (*ZoneRecord, error) {
	if m.zones == nil {
		return nil, fmt.Errorf("wxalert: zone cache not configured")
	}
	return m.zones.Get(ctx, ugc, allowFetch)
}

// PurgeEndedOlderThan removes ended alerts older than cutoff (the Manager's
// daily-purge hook).
func (m *Manager) PurgeEndedOlderThan(cutoff time.Time) int {
	return m.registry.PurgeEndedBefore(cutoff)
}

// TickExpiry runs the registry's clock-based expiry check and re-derives
// notify classes for anything that just expired.
func (m *Manager) TickExpiry() {
	now := m.clock()
	changes := m.registry.Tick(now)
	m.applyNotifyForChanges(changes, now)
	if len(changes) > 0 {
		m.broadcastSnapshot("poll")
	}
}

// RegistrySnapshotForPersistence and RestoreRegistry are the restart-
// hydration hooks; WP2's SQLite store calls these across a restart.
func (m *Manager) RegistrySnapshotForPersistence() RegistrySnapshot { return m.registry.Snapshot() }

func (m *Manager) RestoreRegistry(snap RegistrySnapshot) {
	m.registry.Restore(snap)
	m.recomputeAll()
}

// ---------------------------------------------------------------------------
// PollSink
// ---------------------------------------------------------------------------

// OnAlerts implements PollSink: it feeds the decoded active set into the
// Registry, re-derives proximity/Affects for everything against the current
// footprint, re-derives notify classes for whatever changed, and broadcasts
// a snapshot when anything did.
func (m *Manager) OnAlerts(alerts []Alert, fetchedAt time.Time, complete bool) {
	changes := m.registry.Apply(alerts, fetchedAt)
	m.recomputeProximityAll(fetchedAt)
	m.applyNotifyForChanges(changes, fetchedAt)
	if len(changes) > 0 {
		m.broadcastSnapshot("poll")
	}
}

// OnLinkStatus implements PollSink. The counters are added here too: the
// provenance strip updates from this event as well as from the snapshot.
func (m *Manager) OnLinkStatus(st LinkStatus) {
	select {
	case m.events <- Event{Type: EventLinkStatus, Data: m.withCounts(st)}:
	default:
	}
}

func (m *Manager) broadcastSnapshot(reason string) {
	snap := m.Snapshot()
	select {
	case m.events <- Event{Type: EventAlerts, Data: map[string]any{"snapshot": snap, "reason": reason}}:
	default:
	}
}

func (m *Manager) recomputeAll() {
	now := m.clock()
	m.recomputeProximityAll(now)
}

func (m *Manager) recomputeProximityAll(now time.Time) {
	m.mu.Lock()
	fp := m.footprint
	netID := m.netID
	m.mu.Unlock()

	zones := m.cachedZoneMap()
	for _, a := range m.registry.Active() {
		match := fp.Classify(a.Alert, zones)
		m.registry.UpdateFields(a.ID, func(ma *MatchedAlert) {
			ma.Proximity = match.Proximity
			ma.DistanceMiles = match.DistanceMiles
			ma.BearingDeg = match.BearingDeg
			ma.Affects = match.Affects
			ma.GeometrySource = match.GeometrySource
			ma.NetID = netID
			ma.Zones = zonesForAlert(a.Alert, zones)
		})
	}
	_ = now
}

func zonesForAlert(a Alert, cache map[string]*ZoneRecord) []ZoneRef {
	if len(a.UGC) == 0 {
		return []ZoneRef{}
	}
	out := make([]ZoneRef, 0, len(a.UGC))
	for _, u := range a.UGC {
		ref := ZoneRef{UGC: u, Type: ZoneTypeOf(u)}
		if rec, ok := cache[u]; ok && rec != nil {
			ref.Name = rec.Name
			ref.State = rec.State
			ref.Cached = true
		}
		out = append(out, ref)
	}
	return out
}

func (m *Manager) cachedZoneMap() map[string]*ZoneRecord {
	if m.zones == nil {
		return nil
	}
	out := map[string]*ZoneRecord{}
	for _, a := range m.registry.Active() {
		for _, u := range a.UGC {
			if _, ok := out[u]; ok {
				continue
			}
			if rec, err := m.zones.Get(context.Background(), u, false); err == nil {
				out[u] = rec
			}
		}
	}
	return out
}

func (m *Manager) applyNotifyForChanges(changes []Change, now time.Time) {
	m.mu.Lock()
	pol := m.policy
	m.mu.Unlock()

	for _, ch := range changes {
		switch ch.Kind {
		case ChangeAdded, ChangeUpdated:
			a := m.registry.Get(ch.Alert.ID)
			if a == nil {
				continue
			}
			in := Input{
				Event: a.Event, EffectiveEvent: a.EffectiveEvent, Tier: a.Tier, Severity: a.Severity,
				Urgency: a.Urgency, Proximity: a.Proximity, IsTest: a.IsTest(), Active: true,
			}
			if ch.Prev != nil {
				in.IsUpdate = true
				in.PrevClass = ch.Prev.NotifyClass
				in.NetAcked = ch.Prev.AckedForNet != nil
				in.Escalated = SeverityRank(a.Severity) > SeverityRank(ch.Prev.Severity) ||
					TierRank(a.Tier) > TierRank(ch.Prev.Tier) ||
					(ch.Prev.Proximity == ProximityNear && a.Proximity == ProximityIn)
				if in.Escalated {
					m.registry.ClearNetAck(a.ID)
				}
			}
			res := Classify(in, pol)
			m.registry.UpdateFields(a.ID, func(ma *MatchedAlert) {
				ma.NotifyClass = res.Class
				ma.NotifyReason = res.Reason
				ma.Floored = res.Floored
				ma.UpdatedAt = now
			})

		case ChangeCancelled, ChangeExpired, ChangeVanished:
			in := Input{
				Event: ch.Alert.Event, EffectiveEvent: ch.Alert.EffectiveEvent, Tier: ch.Alert.Tier,
				Proximity: ch.Alert.Proximity, Active: false, PrevClass: ch.Alert.NotifyClass,
			}
			res := Classify(in, pol)
			m.registry.UpdateFields(ch.Alert.ID, func(ma *MatchedAlert) {
				ma.NotifyClass = res.Class
				ma.NotifyReason = "ended"
				ma.UpdatedAt = now
			})
		}
	}
}

func sortMatchedAlerts(alerts []MatchedAlert) {
	group := func(a MatchedAlert) int {
		switch a.State {
		case AlertStateActive:
			if a.Proximity == ProximityIn {
				return 0
			}
			return 1
		default:
			return 2
		}
	}
	less := func(i, j int) bool {
		a, b := alerts[i], alerts[j]
		if gi, gj := group(a), group(b); gi != gj {
			return gi < gj
		}
		if TierRank(a.Tier) != TierRank(b.Tier) {
			return TierRank(a.Tier) > TierRank(b.Tier)
		}
		if SeverityRank(a.Severity) != SeverityRank(b.Severity) {
			return SeverityRank(a.Severity) > SeverityRank(b.Severity)
		}
		if !a.EndsAt.Equal(b.EndsAt) {
			return a.EndsAt.Before(b.EndsAt)
		}
		return a.Sent.After(b.Sent)
	}
	insertionSort(alerts, less)
}

// insertionSort avoids pulling in sort.Slice's reflection cost for the small
// (typically <20) alert lists this runs on, and keeps the comparator legible.
func insertionSort(alerts []MatchedAlert, less func(i, j int) bool) {
	for i := 1; i < len(alerts); i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			alerts[j], alerts[j-1] = alerts[j-1], alerts[j]
		}
	}
}
