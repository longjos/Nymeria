// Package course implements bike-ride course closure: scheduled shutoff
// points, rider support-status exceptions (the only place an individual
// rider appears — a bib is a label, never a key), sweep tracking, and the
// per-station closure ladder that rolls up into a rolling, per-station
// course-clear accumulator NCS holds. Single net for v1 (fact 9): every
// record carries a nullable Division field so parallel Route/RestStop/
// Medical/Supply nets can land later without a migration.
//
// This package mirrors internal/checkpoint's manager+store+events pattern:
// a store-backed in-memory cache guarded by an RWMutex, a buffered events
// channel the WebSocket hub bridges, and an injectable clock for
// deterministic tests. It reuses internal/checkpoint for route-relative
// position rather than reimplementing it, and internal/annotation for the
// station identity (label/category/sequence) closures apply to.
package course

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/checkpoint"
	"github.com/narvel/nymeria/internal/store"
)

// Shutoff point status.
const (
	ShutoffPlanned   = "planned"
	ShutoffFired     = "fired"
	ShutoffCancelled = "cancelled"
)

// Rider support status — the population-definition transition. A rider
// leaves the accounted-for (supported) set by passing a shutoff or
// declining SAG; that is a status change, never a delete.
const (
	SupportSupported   = "supported"
	SupportUnsupported = "unsupported"
)

// Rider exception kinds. A bib is a label attached to one of these; there
// is no rider roster and nothing joins on Bib.
const (
	KindSAG            = "sag"
	KindMedical        = "medical"
	KindDNF            = "dnf"
	KindDeclinedSAG    = "declined_sag"
	KindShutoffReroute = "shutoff_reroute"
	KindOther          = "other"
)

// ValidKinds is the set of allowed RiderException.Kind values.
var ValidKinds = map[string]bool{
	KindSAG: true, KindMedical: true, KindDNF: true,
	KindDeclinedSAG: true, KindShutoffReroute: true, KindOther: true,
}

// Station closure ladder states.
const (
	StationOpen        = "open"
	StationRidersClear = "riders_clear"
	StationSweepPassed = "sweep_passed"
	StationClosed      = "closed"
)

// Config defaults.
const (
	DefaultSweepLabel = "SWEEP"
	DefaultLeadLabel  = "LEAD"
)

// Sentinel errors. Handlers map these to specific HTTP statuses/codes;
// everything else is treated as a plain validation error (400).
var (
	// ErrSweepNotPassed is returned by CloseStation (and the annotation
	// status guard) when the closure would need the sweep gate and no
	// override was requested. Handlers map it to 409 code "sweep_not_passed".
	ErrSweepNotPassed = errors.New("rest stop cannot close: sweep has not passed it")

	// ErrOverrideNotAllowed is returned when an override was requested by a
	// caller who is not NCS/Admin. The handler decides identity via
	// CloseInput.ActorIsNCSOrAdmin; the manager only trusts the bool.
	ErrOverrideNotAllowed = errors.New("override requires net control or an admin")

	// ErrIllegalTransition is returned for any state-machine move that is
	// not legal from the current state (shutoff or station ladder alike).
	ErrIllegalTransition = errors.New("illegal state transition")

	// ErrNotFound is returned when a shutoff or rider exception id does not
	// exist for the given net.
	ErrNotFound = errors.New("not found")
)

// WS event types.
const (
	EventShutoffCreated = "course_shutoff_created"
	EventShutoffUpdated = "course_shutoff_updated"
	EventShutoffFired   = "course_shutoff_fired"
	EventRiderCreated   = "course_rider_created"
	EventRiderUpdated   = "course_rider_updated"
	EventSweepReport    = "course_sweep_report"
	EventStationUpdated = "course_station_updated"
	EventConfigUpdated  = "course_config_updated"
	// EventCourseState is the full CourseState re-broadcast after every
	// mutation; what a course-status dashboard strip subscribes to.
	EventCourseState = "course_state"
)

// Timeline (store.NetEvent.Type) values, written via store.SaveNetEvent
// mirroring checkpoint.LogPassage's pattern.
const (
	TimelineShutoffFired         = "shutoff_fired"
	TimelineShutoffCancelled     = "shutoff_cancelled"
	TimelineShutoffReinstated    = "shutoff_reinstated"
	TimelineRiderException       = "rider_exception"
	TimelineRiderUnsupported     = "rider_unsupported"
	TimelineRiderResupported     = "rider_resupported"
	TimelineSweepReport          = "sweep_report"
	TimelineStationRidersClear   = "station_riders_clear"
	TimelineStationSweepPassed   = "station_sweep_passed"
	TimelineStationClosed        = "station_closed"
	TimelineStationCloseOverride = "station_close_override"
	TimelineStationReopened      = "station_reopened"
)

// Event represents a course event for WebSocket broadcast.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// StationView joins closure state with the station's identity and sequence.
type StationView struct {
	CheckpointID   string               `json:"checkpointId"`
	Label          string               `json:"label"`
	Category       string               `json:"category"` // checkpoint | aid | start | finish
	SequenceNumber int                  `json:"sequenceNumber"`
	Closure        store.StationClosure `json:"closure"`
	// OutOfOrder is reported, never blocked — a stop with no riders left
	// legitimately closes early on the far side of a loop.
	OutOfOrder   bool `json:"outOfOrder"`
	PassageCount int  `json:"passageCount"`
}

// SweepPosition is derived from checkpoint progress + the latest sweep
// report; never stored.
type SweepPosition struct {
	LastCheckpointID  string             `json:"lastCheckpointId"`
	LastCheckpointSeq int                `json:"lastCheckpointSeq"`
	LastPassageTime   *time.Time         `json:"lastPassageTime,omitempty"`
	LatestReport      *store.SweepReport `json:"latestReport,omitempty"`
	NextStationID     string             `json:"nextStationId"`
	NextStationLabel  string             `json:"nextStationLabel"`
	// EtaToNextMinutes is left nil until checkpoints carry a route-mile of
	// their own (open question: no per-checkpoint distance field exists
	// yet); see TestSweepEta's documented skip.
	EtaToNextMinutes *float64 `json:"etaToNextMinutes,omitempty"`
}

// CourseState is the rolling accumulator NCS holds. Slices are never nil —
// a nil slice marshals to JSON null and freezes the Svelte UI.
type CourseState struct {
	NetID             string             `json:"netId"`
	Config            store.CourseConfig `json:"config"`
	Stations          []StationView      `json:"stations"` // ordered by sequence asc (start = back of course)
	ClearThroughSeq   int                `json:"clearThroughSeq"`
	ClearThroughLabel string             `json:"clearThroughLabel"`
	StationsOpen      int                `json:"stationsOpen"`
	StationsClosed    int                `json:"stationsClosed"`
	// AllStationsClosed is the "course is done" input for a later
	// reconciliation package; still not a single boolean on the air — the
	// accumulator above is the real state.
	AllStationsClosed     bool                 `json:"allStationsClosed"`
	Sweep                 SweepPosition        `json:"sweep"`
	Shutoffs              []store.ShutoffPoint `json:"shutoffs"` // ordered by scheduledAt
	NextShutoff           *store.ShutoffPoint  `json:"nextShutoff,omitempty"`
	SupportedExceptions   int                  `json:"supportedExceptions"`
	UnsupportedExceptions int                  `json:"unsupportedExceptions"`
	RerouteCountTotal     int                  `json:"rerouteCountTotal"`
	UpdatedAt             time.Time            `json:"updatedAt"`
}

// FireInput is the payload for firing a planned shutoff.
type FireInput struct {
	By           string
	Note         string
	Bibs         []string
	RerouteCount int
	BibWithheld  bool
}

// CloseInput is the payload for closing a station. ActorIsNCSOrAdmin is
// decided by the HTTP layer (session identity); the manager only trusts it.
type CloseInput struct {
	By                string
	Override          bool
	OverrideReason    string
	ActorIsNCSOrAdmin bool
}

// Manager mirrors checkpoint.Manager: store-backed cache, RWMutex, buffered
// events channel, injectable clock.
type Manager struct {
	store  store.Store
	cpMgr  *checkpoint.Manager
	annMgr *annotation.Manager

	mu       sync.RWMutex
	configs  map[string]store.CourseConfig              // netID -> config
	shutoffs map[string][]store.ShutoffPoint            // netID -> shutoffs
	riders   map[string][]store.RiderException          // netID -> exceptions
	sweeps   map[string][]store.SweepReport             // netID -> reports
	stations map[string]map[string]store.StationClosure // netID -> checkpointID -> closure

	events chan Event
	now    func() time.Time
}

// NewManager creates a new course Manager.
func NewManager(s store.Store, cp *checkpoint.Manager, am *annotation.Manager) *Manager {
	return &Manager{
		store:    s,
		cpMgr:    cp,
		annMgr:   am,
		configs:  make(map[string]store.CourseConfig),
		shutoffs: make(map[string][]store.ShutoffPoint),
		riders:   make(map[string][]store.RiderException),
		sweeps:   make(map[string][]store.SweepReport),
		stations: make(map[string]map[string]store.StationClosure),
		events:   make(chan Event, 64),
	}
}

// SetClock injects a time source (defaults to time.Now) for deterministic tests.
func (m *Manager) SetClock(now func() time.Time) {
	m.mu.Lock()
	m.now = now
	m.mu.Unlock()
}

func (m *Manager) clock() time.Time {
	m.mu.RLock()
	fn := m.now
	m.mu.RUnlock()
	if fn == nil {
		return time.Now().UTC()
	}
	return fn()
}

// Events returns the events channel for WebSocket broadcast.
func (m *Manager) Events() <-chan Event {
	return m.events
}

func (m *Manager) emit(evt Event) {
	select {
	case m.events <- evt:
	default:
	}
}

// emitCourseState re-broadcasts the full accumulator after every mutation.
func (m *Manager) emitCourseState(netID string) {
	if m.cpMgr == nil {
		return
	}
	state, err := m.State(netID)
	if err != nil {
		return
	}
	m.emit(Event{Type: EventCourseState, Data: state})
}

// RefreshState re-broadcasts a net's course state without mutating anything.
//
// The course accumulator is derived from the net's sequenced stops, but it is
// only ever emitted after a COURSE mutation (a closure, a sweep report, a
// shutoff). Giving a stop its first sequence number is a checkpoint mutation,
// not a course one, so nothing re-broadcast: the newly built course existed
// server-side but no open client learned of it until a hard reload. The
// checkpoint bridge calls this so numbering a stop lights up the rail at once.
func (m *Manager) RefreshState(netID string) {
	if netID == "" {
		return
	}
	m.emitCourseState(netID)
}

// logTimeline writes a NetEvent the same fire-and-forget way
// checkpoint.LogPassage does (errors are not actionable here — the request
// already succeeded).
func (m *Manager) logTimeline(netID, eventType, callsign, summary, detailsJSON string) {
	if detailsJSON == "" {
		detailsJSON = "{}"
	}
	m.store.SaveNetEvent(store.NetEvent{
		ID:        uuid.New().String(),
		NetID:     netID,
		Type:      eventType,
		Callsign:  callsign,
		Summary:   summary,
		Details:   detailsJSON,
		CreatedAt: m.clock(),
	})
}

func (m *Manager) stationLabel(checkpointID string) string {
	if m.annMgr != nil {
		if a, ok := m.annMgr.Get(checkpointID); ok {
			return a.Label
		}
	}
	return checkpointID
}

// Load hydrates every cache from the store. Net IDs are collected the same
// way checkpoint.Manager.Load does: active nets, plus any net referenced by
// a sequenceable annotation (so narrow test fixtures without a Net row
// still load).
func (m *Manager) Load() error {
	netIDs := make(map[string]bool)

	nets, err := m.store.LoadNets()
	if err != nil {
		return fmt.Errorf("load nets for course: %w", err)
	}
	for _, n := range nets {
		netIDs[n.ID] = true
	}
	if m.annMgr != nil {
		for _, ann := range m.annMgr.All() {
			if annotation.SequenceableCategories[ann.Category] && ann.NetID != "" {
				netIDs[ann.NetID] = true
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for netID := range netIDs {
		cfg, err := m.store.LoadCourseConfig(netID)
		if err != nil {
			return fmt.Errorf("load course config for net %s: %w", netID, err)
		}
		if cfg != nil {
			m.configs[netID] = *cfg
		}

		sps, err := m.store.LoadShutoffPoints(netID)
		if err != nil {
			return fmt.Errorf("load shutoff points for net %s: %w", netID, err)
		}
		if len(sps) > 0 {
			m.shutoffs[netID] = sps
		}

		riders, err := m.store.LoadRiderExceptions(netID)
		if err != nil {
			return fmt.Errorf("load rider exceptions for net %s: %w", netID, err)
		}
		if len(riders) > 0 {
			m.riders[netID] = riders
		}

		reports, err := m.store.LoadSweepReports(netID, 0)
		if err != nil {
			return fmt.Errorf("load sweep reports for net %s: %w", netID, err)
		}
		if len(reports) > 0 {
			m.sweeps[netID] = reports
		}

		closures, err := m.store.LoadStationClosures(netID)
		if err != nil {
			return fmt.Errorf("load station closures for net %s: %w", netID, err)
		}
		if len(closures) > 0 {
			byCp := make(map[string]store.StationClosure, len(closures))
			for _, c := range closures {
				byCp[c.CheckpointID] = c
			}
			m.stations[netID] = byCp
		}
	}

	return nil
}

// --- Config ---

func defaultCourseConfig(netID string) store.CourseConfig {
	return store.CourseConfig{
		NetID:                netID,
		SweepLabel:           DefaultSweepLabel,
		LeadLabel:            DefaultLeadLabel,
		CloseRequiresSweep:   true,
		AutoSweepFromPassage: true,
	}
}

// GetConfig returns the net's course config, or agency-editable defaults
// when no row exists yet.
func (m *Manager) GetConfig(netID string) store.CourseConfig {
	m.mu.RLock()
	cfg, ok := m.configs[netID]
	m.mu.RUnlock()
	if ok {
		return cfg
	}
	return defaultCourseConfig(netID)
}

// SetConfig replaces the net's course config. Blank labels fall back to
// their defaults.
func (m *Manager) SetConfig(cfg store.CourseConfig) (*store.CourseConfig, error) {
	if strings.TrimSpace(cfg.NetID) == "" {
		return nil, errors.New("netId is required")
	}
	if strings.TrimSpace(cfg.SweepLabel) == "" {
		cfg.SweepLabel = DefaultSweepLabel
	}
	if strings.TrimSpace(cfg.LeadLabel) == "" {
		cfg.LeadLabel = DefaultLeadLabel
	}
	cfg.UpdatedAt = m.clock()

	if err := m.store.SaveCourseConfig(cfg); err != nil {
		return nil, fmt.Errorf("persist course config: %w", err)
	}
	m.mu.Lock()
	m.configs[cfg.NetID] = cfg
	m.mu.Unlock()

	m.emit(Event{Type: EventConfigUpdated, Data: cfg})
	m.emitCourseState(cfg.NetID)
	return &cfg, nil
}

// --- Shutoffs ---

func validateShutoffCore(sp store.ShutoffPoint) error {
	if strings.TrimSpace(sp.Name) == "" {
		return errors.New("name is required")
	}
	if sp.ScheduledAt.IsZero() {
		return errors.New("scheduledAt is required")
	}
	if sp.Lat < -90 || sp.Lat > 90 {
		return errors.New("lat out of range")
	}
	if sp.Lon < -180 || sp.Lon > 180 {
		return errors.New("lon out of range")
	}
	return nil
}

// checkInExists reports whether checkInID names a check-in of netID. An
// empty checkInID is always valid (the field is optional). Reuses
// store.Store's own LoadNetCheckIns rather than adding a narrow
// CheckInSource dependency the constructor doesn't take.
func (m *Manager) checkInExists(netID, checkInID string) (bool, error) {
	if checkInID == "" {
		return true, nil
	}
	cis, err := m.store.LoadNetCheckIns(netID)
	if err != nil {
		return false, err
	}
	for _, ci := range cis {
		if ci.ID == checkInID {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) findShutoff(netID, id string) (store.ShutoffPoint, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sp := range m.shutoffs[netID] {
		if sp.ID == id {
			return sp, true
		}
	}
	return store.ShutoffPoint{}, false
}

func (m *Manager) storeShutoff(sp store.ShutoffPoint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.shutoffs[sp.NetID]
	for i := range list {
		if list[i].ID == sp.ID {
			list[i] = sp
			return
		}
	}
	m.shutoffs[sp.NetID] = append(list, sp)
}

// CreateShutoff validates and persists a new planned shutoff point.
func (m *Manager) CreateShutoff(sp store.ShutoffPoint) (*store.ShutoffPoint, error) {
	if err := validateShutoffCore(sp); err != nil {
		return nil, err
	}
	if ok, err := m.checkInExists(sp.NetID, sp.StaffedByCheckInID); err != nil {
		return nil, fmt.Errorf("validate staffed-by check-in: %w", err)
	} else if !ok {
		return nil, fmt.Errorf("staffedByCheckInId %q is not a check-in of this net", sp.StaffedByCheckInID)
	}

	now := m.clock()
	sp.ID = uuid.New().String()
	sp.Status = ShutoffPlanned
	sp.FiredAt = nil
	sp.CreatedAt = now
	sp.UpdatedAt = now

	if err := m.store.SaveShutoffPoint(sp); err != nil {
		return nil, fmt.Errorf("persist shutoff point: %w", err)
	}
	m.storeShutoff(sp)

	m.emit(Event{Type: EventShutoffCreated, Data: sp})
	m.emitCourseState(sp.NetID)
	return &sp, nil
}

// UpdateShutoff replaces a planned shutoff's editable fields. Legal only in
// planned; fired and cancelled are otherwise terminal.
func (m *Manager) UpdateShutoff(sp store.ShutoffPoint) (*store.ShutoffPoint, error) {
	existing, ok := m.findShutoff(sp.NetID, sp.ID)
	if !ok {
		return nil, ErrNotFound
	}
	if existing.Status != ShutoffPlanned {
		return nil, ErrIllegalTransition
	}
	if err := validateShutoffCore(sp); err != nil {
		return nil, err
	}
	if ok, err := m.checkInExists(sp.NetID, sp.StaffedByCheckInID); err != nil {
		return nil, fmt.Errorf("validate staffed-by check-in: %w", err)
	} else if !ok {
		return nil, fmt.Errorf("staffedByCheckInId %q is not a check-in of this net", sp.StaffedByCheckInID)
	}

	updated := existing
	updated.Name = sp.Name
	updated.Lat = sp.Lat
	updated.Lon = sp.Lon
	updated.RouteMile = sp.RouteMile
	updated.AnnotationID = sp.AnnotationID
	updated.ScheduledAt = sp.ScheduledAt
	updated.RerouteDirection = sp.RerouteDirection
	updated.RerouteDestination = sp.RerouteDestination
	updated.RerouteInstructions = sp.RerouteInstructions
	updated.StaffedByCheckInID = sp.StaffedByCheckInID
	updated.Division = sp.Division
	updated.UpdatedAt = m.clock()

	if err := m.store.SaveShutoffPoint(updated); err != nil {
		return nil, fmt.Errorf("persist shutoff point: %w", err)
	}
	m.storeShutoff(updated)

	m.emit(Event{Type: EventShutoffUpdated, Data: updated})
	m.emitCourseState(updated.NetID)
	return &updated, nil
}

// DeleteShutoff removes a planned shutoff. Legal only in planned.
func (m *Manager) DeleteShutoff(netID, id string) error {
	existing, ok := m.findShutoff(netID, id)
	if !ok {
		return ErrNotFound
	}
	if existing.Status != ShutoffPlanned {
		return ErrIllegalTransition
	}
	if err := m.store.DeleteShutoffPoint(id); err != nil {
		return fmt.Errorf("delete shutoff point: %w", err)
	}
	m.mu.Lock()
	list := m.shutoffs[netID]
	for i, sp := range list {
		if sp.ID == id {
			m.shutoffs[netID] = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	m.mu.Unlock()

	m.emitCourseState(netID)
	return nil
}

// FireShutoff transitions a planned shutoff to fired: it stamps FiredAt/By,
// creates one RiderException (kind shutoff_reroute, unsupported) per named
// bib, adds RerouteCount for uncounted riders, logs a timeline entry, and
// broadcasts.
func (m *Manager) FireShutoff(netID, id string, in FireInput) (*store.ShutoffPoint, []store.RiderException, error) {
	existing, ok := m.findShutoff(netID, id)
	if !ok {
		return nil, nil, ErrNotFound
	}
	if existing.Status != ShutoffPlanned {
		return nil, nil, ErrIllegalTransition
	}

	now := m.clock()
	existing.Status = ShutoffFired
	existing.FiredAt = &now
	existing.FiredBy = in.By
	existing.FireNote = in.Note
	existing.RerouteCount += in.RerouteCount
	existing.UpdatedAt = now

	if err := m.store.SaveShutoffPoint(existing); err != nil {
		return nil, nil, fmt.Errorf("persist fired shutoff: %w", err)
	}
	m.storeShutoff(existing)

	bibs := in.Bibs
	if bibs == nil {
		bibs = []string{}
	}
	riders := make([]store.RiderException, 0, len(bibs))
	for _, bib := range bibs {
		r := store.RiderException{
			ID:              uuid.New().String(),
			NetID:           netID,
			Bib:             bib,
			BibWithheld:     in.BibWithheld,
			Kind:            KindShutoffReroute,
			SupportStatus:   SupportUnsupported,
			Reason:          fmt.Sprintf("shutoff: %s", existing.Name),
			ShutoffID:       id,
			ReportedBy:      in.By,
			RecordedAt:      now,
			StatusChangedAt: now,
			StatusChangedBy: in.By,
		}
		if err := m.store.SaveRiderException(r); err != nil {
			return nil, nil, fmt.Errorf("persist rider exception: %w", err)
		}
		m.mu.Lock()
		m.riders[netID] = append(m.riders[netID], r)
		m.mu.Unlock()
		riders = append(riders, r)
	}

	detailsBytes, _ := json.Marshal(map[string]any{"bibs": bibs, "rerouteCount": in.RerouteCount})
	summary := fmt.Sprintf(
		"%s FIRED %s — riders not past this point directed %s onto %s (%d bibs, +%d uncounted)",
		existing.Name, existing.ScheduledAt.Format("15:04"), existing.RerouteDirection, existing.RerouteDestination,
		len(bibs), in.RerouteCount,
	)
	m.logTimeline(netID, TimelineShutoffFired, in.By, summary, string(detailsBytes))

	m.emit(Event{Type: EventShutoffFired, Data: existing})
	for _, r := range riders {
		m.emit(Event{Type: EventRiderCreated, Data: r})
	}
	m.emitCourseState(netID)

	return &existing, riders, nil
}

// CancelShutoff transitions a planned shutoff to cancelled.
func (m *Manager) CancelShutoff(netID, id, by, reason string) (*store.ShutoffPoint, error) {
	existing, ok := m.findShutoff(netID, id)
	if !ok {
		return nil, ErrNotFound
	}
	if existing.Status != ShutoffPlanned {
		return nil, ErrIllegalTransition
	}
	existing.Status = ShutoffCancelled
	existing.UpdatedAt = m.clock()

	if err := m.store.SaveShutoffPoint(existing); err != nil {
		return nil, fmt.Errorf("persist cancelled shutoff: %w", err)
	}
	m.storeShutoff(existing)

	m.logTimeline(netID, TimelineShutoffCancelled, by, fmt.Sprintf("%s cancelled: %s", existing.Name, reason), "")
	m.emit(Event{Type: EventShutoffUpdated, Data: existing})
	m.emitCourseState(netID)
	return &existing, nil
}

// ReinstateShutoff transitions a cancelled shutoff back to planned. Any
// operator may do this.
func (m *Manager) ReinstateShutoff(netID, id, by string) (*store.ShutoffPoint, error) {
	existing, ok := m.findShutoff(netID, id)
	if !ok {
		return nil, ErrNotFound
	}
	if existing.Status != ShutoffCancelled {
		return nil, ErrIllegalTransition
	}
	existing.Status = ShutoffPlanned
	existing.UpdatedAt = m.clock()

	if err := m.store.SaveShutoffPoint(existing); err != nil {
		return nil, fmt.Errorf("persist reinstated shutoff: %w", err)
	}
	m.storeShutoff(existing)

	m.logTimeline(netID, TimelineShutoffReinstated, by, fmt.Sprintf("%s reinstated to planned", existing.Name), "")
	m.emit(Event{Type: EventShutoffUpdated, Data: existing})
	m.emitCourseState(netID)
	return &existing, nil
}

// UnfireShutoff is the correction path: fired back to planned. The
// exceptions the fire created are KEPT — they must be resupported
// individually via SetRiderSupport. A reason is required; the HTTP layer is
// responsible for restricting this to NCS/Admin.
func (m *Manager) UnfireShutoff(netID, id, by, reason string) (*store.ShutoffPoint, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("reason is required")
	}
	existing, ok := m.findShutoff(netID, id)
	if !ok {
		return nil, ErrNotFound
	}
	if existing.Status != ShutoffFired {
		return nil, ErrIllegalTransition
	}
	existing.Status = ShutoffPlanned
	existing.FiredAt = nil
	existing.FiredBy = ""
	existing.FireNote = reason
	existing.UpdatedAt = m.clock()

	if err := m.store.SaveShutoffPoint(existing); err != nil {
		return nil, fmt.Errorf("persist unfired shutoff: %w", err)
	}
	m.storeShutoff(existing)

	m.logTimeline(netID, TimelineShutoffReinstated, by, fmt.Sprintf("%s reinstated to planned: %s", existing.Name, reason), "")
	m.emit(Event{Type: EventShutoffUpdated, Data: existing})
	m.emitCourseState(netID)
	return &existing, nil
}

// GetShutoffs returns every shutoff for a net, ordered by ScheduledAt. Never nil.
func (m *Manager) GetShutoffs(netID string) []store.ShutoffPoint {
	m.mu.RLock()
	out := append([]store.ShutoffPoint(nil), m.shutoffs[netID]...)
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ScheduledAt.Before(out[j].ScheduledAt) })
	if out == nil {
		out = []store.ShutoffPoint{}
	}
	return out
}

// --- Riders (exceptions) ---

// RecordRider validates and persists a new rider exception — the only place
// an individual rider appears.
func (m *Manager) RecordRider(r store.RiderException) (*store.RiderException, error) {
	if !ValidKinds[r.Kind] {
		return nil, fmt.Errorf("invalid kind %q", r.Kind)
	}
	if r.Bib == "" && !r.BibWithheld && r.Kind != KindDNF && r.Kind != KindShutoffReroute {
		return nil, errors.New("bib is required unless withheld or kind is dnf/shutoff_reroute")
	}
	if r.SupportStatus == "" {
		switch r.Kind {
		case KindDNF, KindDeclinedSAG, KindShutoffReroute:
			r.SupportStatus = SupportUnsupported
		default:
			r.SupportStatus = SupportSupported
		}
	} else if r.SupportStatus != SupportSupported && r.SupportStatus != SupportUnsupported {
		return nil, fmt.Errorf("invalid supportStatus %q", r.SupportStatus)
	}

	now := m.clock()
	r.ID = uuid.New().String()
	r.RecordedAt = now
	r.StatusChangedAt = now
	if r.StatusChangedBy == "" {
		r.StatusChangedBy = r.ReportedBy
	}

	if err := m.store.SaveRiderException(r); err != nil {
		return nil, fmt.Errorf("persist rider exception: %w", err)
	}
	m.mu.Lock()
	m.riders[r.NetID] = append(m.riders[r.NetID], r)
	m.mu.Unlock()

	m.logTimeline(r.NetID, TimelineRiderException, r.ReportedBy, fmt.Sprintf("rider exception recorded (%s)", r.Kind), "")
	m.emit(Event{Type: EventRiderCreated, Data: r})
	m.emitCourseState(r.NetID)
	return &r, nil
}

func (m *Manager) findRider(netID, id string) (store.RiderException, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.riders[netID] {
		if r.ID == id {
			return r, true
		}
	}
	return store.RiderException{}, false
}

func riderLabel(r store.RiderException) string {
	if r.BibWithheld || r.Bib == "" {
		return "withheld"
	}
	return r.Bib
}

// SetRiderSupport transitions a rider exception's support status.
// unsupported -> supported requires a reason (a correction/override);
// supported -> unsupported never does.
func (m *Manager) SetRiderSupport(netID, id, status, by, reason string) (*store.RiderException, error) {
	if status != SupportSupported && status != SupportUnsupported {
		return nil, fmt.Errorf("invalid supportStatus %q", status)
	}
	existing, ok := m.findRider(netID, id)
	if !ok {
		return nil, ErrNotFound
	}
	if status == SupportSupported && existing.SupportStatus != SupportSupported && strings.TrimSpace(reason) == "" {
		return nil, errors.New("reason is required to restore supported status")
	}

	wasSupported := existing.SupportStatus
	existing.SupportStatus = status
	existing.StatusChangedAt = m.clock()
	existing.StatusChangedBy = by
	if reason != "" {
		existing.Reason = reason
	}

	if err := m.store.SaveRiderException(existing); err != nil {
		return nil, fmt.Errorf("persist rider exception: %w", err)
	}
	m.mu.Lock()
	list := m.riders[netID]
	for i := range list {
		if list[i].ID == id {
			list[i] = existing
			break
		}
	}
	m.mu.Unlock()

	if status == SupportUnsupported && wasSupported != SupportUnsupported {
		m.logTimeline(netID, TimelineRiderUnsupported, by, fmt.Sprintf("bib %s marked unsupported", riderLabel(existing)), "")
	} else if status == SupportSupported && wasSupported != SupportSupported {
		m.logTimeline(netID, TimelineRiderResupported, by, fmt.Sprintf("bib %s restored to supported: %s", riderLabel(existing), reason), "")
	}
	m.emit(Event{Type: EventRiderUpdated, Data: existing})
	m.emitCourseState(netID)
	return &existing, nil
}

// MarkUnsupportedFromSAG creates a declined_sag exception when a SAG slot's
// disposition becomes "declined" (called from internal/ride).
func (m *Manager) MarkUnsupportedFromSAG(netID, bib, sagRequestID, by, reason string) (*store.RiderException, error) {
	return m.RecordRider(store.RiderException{
		NetID:           netID,
		Bib:             bib,
		Kind:            KindDeclinedSAG,
		SupportStatus:   SupportUnsupported,
		SAGRequestID:    sagRequestID,
		ReportedBy:      by,
		StatusChangedBy: by,
		Reason:          reason,
	})
}

// GetRiders returns every rider exception for a net. Never nil.
func (m *Manager) GetRiders(netID string) []store.RiderException {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]store.RiderException, len(m.riders[netID]))
	copy(out, m.riders[netID])
	return out
}

// --- Sweep ---

// ReportSweep validates and persists a sweep report.
func (m *Manager) ReportSweep(r store.SweepReport) (*store.SweepReport, error) {
	if strings.TrimSpace(r.ReportedBy) == "" {
		return nil, errors.New("reportedBy is required")
	}
	if r.EstimatedSpeedMph != nil && (*r.EstimatedSpeedMph < 0 || *r.EstimatedSpeedMph > 60) {
		return nil, errors.New("estimatedSpeedMph out of range")
	}

	r.ID = uuid.New().String()
	if r.ReportedAt.IsZero() {
		r.ReportedAt = m.clock()
	}

	if err := m.store.SaveSweepReport(r); err != nil {
		return nil, fmt.Errorf("persist sweep report: %w", err)
	}
	m.mu.Lock()
	m.sweeps[r.NetID] = append(m.sweeps[r.NetID], r)
	m.mu.Unlock()

	m.logTimeline(r.NetID, TimelineSweepReport, r.ReportedBy, fmt.Sprintf("sweep report: last rider %s", r.LastRiderBib), "")
	m.emit(Event{Type: EventSweepReport, Data: r})
	m.emitCourseState(r.NetID)
	return &r, nil
}

// GetSweepReports returns up to limit reports for a net, newest first.
// limit <= 0 returns all. Never nil.
func (m *Manager) GetSweepReports(netID string, limit int) []store.SweepReport {
	m.mu.RLock()
	all := append([]store.SweepReport(nil), m.sweeps[netID]...)
	m.mu.RUnlock()

	sort.Slice(all, func(i, j int) bool { return all[i].ReportedAt.After(all[j].ReportedAt) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []store.SweepReport{}
	}
	return all
}

// SweepPosition derives the sweep's current position from checkpoint
// progress (the SweepLabel passage with the highest sequence) plus the
// latest sweep report. Never stored.
func (m *Manager) SweepPosition(netID string) SweepPosition {
	cfg := m.GetConfig(netID)
	pos := SweepPosition{}

	if m.cpMgr != nil {
		if progress, err := m.cpMgr.GetProgress(netID); err == nil {
			for _, elem := range progress.Elements {
				if strings.EqualFold(elem.Label, cfg.SweepLabel) {
					pos.LastCheckpointID = elem.LastCheckpointID
					pos.LastCheckpointSeq = elem.LastCheckpointSeq
					t := elem.LastPassageTime
					pos.LastPassageTime = &t
					break
				}
			}

			found := false
			bestSeq := 0
			for _, cp := range progress.Checkpoints {
				if cp.Meta.SequenceNumber > pos.LastCheckpointSeq && (!found || cp.Meta.SequenceNumber < bestSeq) {
					bestSeq = cp.Meta.SequenceNumber
					pos.NextStationID = cp.Annotation.ID
					pos.NextStationLabel = cp.Annotation.Label
					found = true
				}
			}
		}
	}

	if reports := m.GetSweepReports(netID, 1); len(reports) > 0 {
		pos.LatestReport = &reports[0]
	}
	return pos
}

// --- Stations ---

func (m *Manager) getClosure(netID, checkpointID string) store.StationClosure {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if byCp, ok := m.stations[netID]; ok {
		if c, ok := byCp[checkpointID]; ok {
			return c
		}
	}
	return store.StationClosure{NetID: netID, CheckpointID: checkpointID, State: StationOpen}
}

func (m *Manager) saveClosure(c store.StationClosure) error {
	if err := m.store.SaveStationClosure(c); err != nil {
		return fmt.Errorf("persist station closure: %w", err)
	}
	m.mu.Lock()
	if m.stations[c.NetID] == nil {
		m.stations[c.NetID] = make(map[string]store.StationClosure)
	}
	m.stations[c.NetID][c.CheckpointID] = c
	m.mu.Unlock()
	return nil
}

// ReportRidersClear transitions a station open -> riders_clear.
func (m *Manager) ReportRidersClear(netID, checkpointID, by string) (*store.StationClosure, error) {
	c := m.getClosure(netID, checkpointID)
	if c.State != StationOpen {
		return nil, ErrIllegalTransition
	}
	now := m.clock()
	c.RidersClearAt = &now
	c.RidersClearBy = by
	c.State = StationRidersClear
	c.UpdatedAt = now

	if err := m.saveClosure(c); err != nil {
		return nil, err
	}
	m.logTimeline(netID, TimelineStationRidersClear, by, fmt.Sprintf("%s riders clear", m.stationLabel(checkpointID)), "")
	m.emit(Event{Type: EventStationUpdated, Data: c})
	m.emitCourseState(netID)
	return &c, nil
}

// markSweepPassed is shared by the manual MarkSweepPassed API and the
// automatic OnPassage hook (auto=true), which logs a distinguishable
// timeline entry. sweep-passed on an already sweep_passed|closed station is
// a no-op success — the auto hook relies on this idempotence.
func (m *Manager) markSweepPassed(netID, checkpointID, by, passageID string, auto bool) (*store.StationClosure, error) {
	c := m.getClosure(netID, checkpointID)
	if c.State == StationSweepPassed || c.State == StationClosed {
		return &c, nil
	}
	if c.State != StationOpen && c.State != StationRidersClear {
		return nil, ErrIllegalTransition
	}

	now := m.clock()
	if c.RidersClearAt == nil {
		c.RidersClearAt = &now
		c.RidersClearBy = by
	}
	c.SweepPassedAt = &now
	c.SweepPassedBy = by
	c.SweepPassageID = passageID
	c.State = StationSweepPassed
	c.UpdatedAt = now

	if err := m.saveClosure(c); err != nil {
		return nil, err
	}
	detailsBytes, _ := json.Marshal(map[string]any{"auto": auto})
	m.logTimeline(netID, TimelineStationSweepPassed, by, fmt.Sprintf("%s sweep passed", m.stationLabel(checkpointID)), string(detailsBytes))
	m.emit(Event{Type: EventStationUpdated, Data: c})
	m.emitCourseState(netID)
	return &c, nil
}

// MarkSweepPassed transitions a station open|riders_clear -> sweep_passed
// (implying riders clear). Idempotent when already sweep_passed or closed.
func (m *Manager) MarkSweepPassed(netID, checkpointID, by, passageID string) (*store.StationClosure, error) {
	return m.markSweepPassed(netID, checkpointID, by, passageID, false)
}

// CloseStation transitions a station to closed. When the net's
// CloseRequiresSweep policy is set and the sweep has not passed, the close
// is refused with ErrSweepNotPassed unless Override is set by an NCS/Admin
// caller with a non-blank OverrideReason.
func (m *Manager) CloseStation(netID, checkpointID string, in CloseInput) (*store.StationClosure, error) {
	c := m.getClosure(netID, checkpointID)
	if c.State == StationClosed {
		return nil, ErrIllegalTransition
	}
	cfg := m.GetConfig(netID)

	overrideUsed := false
	if cfg.CloseRequiresSweep && c.SweepPassedAt == nil {
		if !in.Override {
			return nil, ErrSweepNotPassed
		}
		if !in.ActorIsNCSOrAdmin {
			return nil, ErrOverrideNotAllowed
		}
		if strings.TrimSpace(in.OverrideReason) == "" {
			return nil, errors.New("override reason is required")
		}
		overrideUsed = true
	}

	now := m.clock()
	c.State = StationClosed
	c.ClosedAt = &now
	c.ClosedBy = in.By
	c.ClosedByOverride = overrideUsed
	if overrideUsed {
		c.OverrideReason = in.OverrideReason
	}
	c.UpdatedAt = now

	if err := m.saveClosure(c); err != nil {
		return nil, err
	}

	// Sync the annotation + checkpoint meta. ChangeStatusUnguarded bypasses
	// the very guard this method just enforced with full context.
	if m.annMgr != nil {
		m.annMgr.ChangeStatusUnguarded(checkpointID, "closed")
	}
	if m.cpMgr != nil {
		if meta, ok := m.cpMgr.MetaForAnnotation(checkpointID); ok {
			meta.ClosedAt = &now
			m.cpMgr.SetMeta(meta)
		}
	}

	timelineType := TimelineStationClosed
	detailsJSON := ""
	if overrideUsed {
		timelineType = TimelineStationCloseOverride
		db, _ := json.Marshal(map[string]any{"reason": in.OverrideReason})
		detailsJSON = string(db)
	}
	m.logTimeline(netID, timelineType, in.By, fmt.Sprintf("%s closed", m.stationLabel(checkpointID)), detailsJSON)

	m.emit(Event{Type: EventStationUpdated, Data: c})
	m.emitCourseState(netID)
	return &c, nil
}

// ReopenStation reopens a station from any state. SweepPassedAt is kept — a
// sweep passing is a fact, independent of whether the station later
// reopens — only ClosedAt/By/override are cleared.
func (m *Manager) ReopenStation(netID, checkpointID, by, reason string) (*store.StationClosure, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("reason is required")
	}
	c := m.getClosure(netID, checkpointID)
	now := m.clock()
	c.State = StationOpen
	c.ClosedAt = nil
	c.ClosedBy = ""
	c.ClosedByOverride = false
	c.OverrideReason = ""
	c.ReopenCount++
	c.Note = reason
	c.UpdatedAt = now

	if err := m.saveClosure(c); err != nil {
		return nil, err
	}

	if m.annMgr != nil {
		m.annMgr.ChangeStatusUnguarded(checkpointID, "active")
	}
	if m.cpMgr != nil {
		if meta, ok := m.cpMgr.MetaForAnnotation(checkpointID); ok {
			meta.ClosedAt = nil
			m.cpMgr.SetMeta(meta)
		}
	}

	m.logTimeline(netID, TimelineStationReopened, by, fmt.Sprintf("%s reopened: %s", m.stationLabel(checkpointID), reason), "")
	m.emit(Event{Type: EventStationUpdated, Data: c})
	m.emitCourseState(netID)
	return &c, nil
}

// OnPassage is registered with checkpoint.Manager.SetOnPassage. A passage
// whose label matches the net's configured SweepLabel (case-insensitive)
// auto-advances that station to sweep_passed, when the net's
// AutoSweepFromPassage policy allows it. MarkSweepPassed's own idempotence
// means an already sweep_passed|closed station is silently a no-op.
func (m *Manager) OnPassage(p store.CheckpointPassage) {
	cfg := m.GetConfig(p.NetID)
	if !cfg.AutoSweepFromPassage {
		return
	}
	if !strings.EqualFold(p.Label, cfg.SweepLabel) {
		return
	}
	m.markSweepPassed(p.NetID, p.CheckpointID, p.ReportedBy, p.ID, true)
}

// GuardAnnotationStatus is registered with annotation.Manager.SetStatusGuard.
// It blocks closing a sequenced station's annotation directly (bypassing
// CloseStation's own override path) when the net's CloseRequiresSweep
// policy is set and the sweep has not passed it yet. A non-sequenced
// annotation, or one in a net without CloseRequiresSweep, is never gated.
func (m *Manager) GuardAnnotationStatus(a store.Annotation, newStatus string) error {
	if newStatus != StationClosed {
		return nil
	}
	if m.cpMgr == nil {
		return nil
	}
	meta, ok := m.cpMgr.MetaForAnnotation(a.ID)
	if !ok {
		return nil
	}
	cfg := m.GetConfig(meta.NetID)
	if !cfg.CloseRequiresSweep {
		return nil
	}
	c := m.getClosure(meta.NetID, a.ID)
	if c.SweepPassedAt == nil {
		return ErrSweepNotPassed
	}
	return nil
}

// --- State ---

// State builds the rolling course-clear accumulator for a net: every
// sequenced station's closure ladder position, the largest contiguous
// prefix (from the back of the course) that is sweep_passed or closed, the
// sweep's derived position, every shutoff, and rider-exception counts.
// Slices are never nil.
func (m *Manager) State(netID string) (*CourseState, error) {
	cfg := m.GetConfig(netID)

	checkpoints, err := m.cpMgr.GetCheckpointsForNet(netID)
	if err != nil {
		return nil, fmt.Errorf("get checkpoints for course state: %w", err)
	}

	stations := make([]StationView, 0, len(checkpoints))
	for _, cp := range checkpoints {
		closure := m.getClosure(netID, cp.Annotation.ID)
		stations = append(stations, StationView{
			CheckpointID:   cp.Annotation.ID,
			Label:          cp.Annotation.Label,
			Category:       cp.Annotation.Category,
			SequenceNumber: cp.Meta.SequenceNumber,
			Closure:        closure,
			PassageCount:   cp.PassageCount,
		})
	}
	sort.Slice(stations, func(i, j int) bool { return stations[i].SequenceNumber < stations[j].SequenceNumber })

	clearThroughSeq := 0
	clearThroughLabel := ""
	for _, sv := range stations {
		if sv.Closure.State == StationSweepPassed || sv.Closure.State == StationClosed {
			clearThroughSeq = sv.SequenceNumber
			clearThroughLabel = sv.Label
		} else {
			break
		}
	}

	hasOpenBelow := false
	closedCount := 0
	for i := range stations {
		st := stations[i].Closure.State
		if st == StationSweepPassed || st == StationClosed {
			if hasOpenBelow {
				stations[i].OutOfOrder = true
			}
		} else {
			hasOpenBelow = true
		}
		if st == StationClosed {
			closedCount++
		}
	}
	allClosed := len(stations) > 0 && closedCount == len(stations)

	shutoffs := m.GetShutoffs(netID)
	var nextShutoff *store.ShutoffPoint
	for i := range shutoffs {
		if shutoffs[i].Status != ShutoffPlanned {
			continue
		}
		if nextShutoff == nil || shutoffs[i].ScheduledAt.Before(nextShutoff.ScheduledAt) {
			sp := shutoffs[i]
			nextShutoff = &sp
		}
	}

	riders := m.GetRiders(netID)
	supportedCount, unsupportedCount, rerouteExceptionCount := 0, 0, 0
	for _, r := range riders {
		if r.SupportStatus == SupportSupported {
			supportedCount++
		} else {
			unsupportedCount++
		}
		if r.Kind == KindShutoffReroute {
			rerouteExceptionCount++
		}
	}
	rerouteTotal := rerouteExceptionCount
	for _, sp := range shutoffs {
		rerouteTotal += sp.RerouteCount
	}

	return &CourseState{
		NetID:                 netID,
		Config:                cfg,
		Stations:              stations,
		ClearThroughSeq:       clearThroughSeq,
		ClearThroughLabel:     clearThroughLabel,
		StationsOpen:          len(stations) - closedCount,
		StationsClosed:        closedCount,
		AllStationsClosed:     allClosed,
		Sweep:                 m.SweepPosition(netID),
		Shutoffs:              shutoffs,
		NextShutoff:           nextShutoff,
		SupportedExceptions:   supportedCount,
		UnsupportedExceptions: unsupportedCount,
		RerouteCountTotal:     rerouteTotal,
		UpdatedAt:             m.clock(),
	}, nil
}

// DeleteForNet clears every course-closure record for a net.
func (m *Manager) DeleteForNet(netID string) error {
	if err := m.store.DeleteCourseDataForNet(netID); err != nil {
		return fmt.Errorf("delete course data for net: %w", err)
	}
	m.mu.Lock()
	delete(m.configs, netID)
	delete(m.shutoffs, netID)
	delete(m.riders, netID)
	delete(m.sweeps, netID)
	delete(m.stations, netID)
	m.mu.Unlock()
	return nil
}
