// Package wxalert implements NWS weather watches, warnings and advisories:
// fetching the CAP/GeoJSON active-alerts feed, matching alerts against a
// net's geographic footprint, classifying how loudly to notify, and caching
// zone polygons to disk so zone-only alerts (about 85% of the live feed)
// still render offline.
//
// The package is a leaf: it does not import internal/config, internal/netcontrol,
// internal/annotation or internal/checkpoint. It imports internal/store only for
// the Annotation/NetCheckIn record shapes, internal/station for the tracked
// Station type and internal/gps for LatLon-shaped fixes. Callers (internal/app,
// internal/server) adapt their own managers to the small interfaces this
// package declares.
package wxalert

import (
	"encoding/json"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Typed string enums. The zero value is always the "Unknown"/empty member so a
// missing JSON field never panics a switch. Every value is CAP-spec-cased for
// Severity/Certainty/Urgency/Status/MessageType, and lowercase for Tier/
// Proximity/AlertState/NotifyClass/LinkState/GeometrySource — those are UI keys.
// ---------------------------------------------------------------------------

// Severity is the CAP <severity> value.
type Severity string

// Certainty is the CAP <certainty> value.
type Certainty string

// Urgency is the CAP <urgency> value.
type Urgency string

// Status is the CAP <status> value (message status, not the alert's lifecycle).
type Status string

// MessageType is the CAP <msgType> value.
type MessageType string

// Tier is the notification tier derived from the event name (tier.go).
type Tier string

// Proximity is how the alert's geometry relates to the watch footprint.
type Proximity string

// AlertState is the lifecycle state of a MatchedAlert as served to the frontend.
type AlertState string

// NotifyClass is how loudly an alert should be surfaced.
type NotifyClass string

// LinkState summarizes the poller's connection health.
type LinkState string

// GeometrySource says what geometry, if any, backs an alert's map rendering.
type GeometrySource string

const (
	SeverityExtreme  Severity = "Extreme"
	SeveritySevere   Severity = "Severe"
	SeverityModerate Severity = "Moderate"
	SeverityMinor    Severity = "Minor"
	SeverityUnknown  Severity = "Unknown"

	CertaintyObserved Certainty = "Observed"
	CertaintyLikely   Certainty = "Likely"
	CertaintyPossible Certainty = "Possible"
	CertaintyUnlikely Certainty = "Unlikely"
	CertaintyUnknown  Certainty = "Unknown"

	UrgencyImmediate Urgency = "Immediate"
	UrgencyExpected  Urgency = "Expected"
	UrgencyFuture    Urgency = "Future"
	UrgencyPast      Urgency = "Past"
	UrgencyUnknown   Urgency = "Unknown"

	StatusActual   Status = "Actual"
	StatusExercise Status = "Exercise"
	StatusSystem   Status = "System"
	StatusTest     Status = "Test"
	StatusDraft    Status = "Draft"

	MessageTypeAlert  MessageType = "Alert"
	MessageTypeUpdate MessageType = "Update"
	MessageTypeCancel MessageType = "Cancel"

	TierWarning   Tier = "warning"
	TierWatch     Tier = "watch"
	TierAdvisory  Tier = "advisory"
	TierStatement Tier = "statement"

	ProximityIn   Proximity = "in"
	ProximityNear Proximity = "near"
	ProximityFar  Proximity = "far"

	AlertStateActive     AlertState = "active"
	AlertStateExpired    AlertState = "expired"
	AlertStateCancelled  AlertState = "cancelled"
	AlertStateDropped    AlertState = "dropped"
	AlertStateSuperseded AlertState = "superseded" // internal only; never served on the wire

	NotifyInterrupt NotifyClass = "interrupt"
	NotifyToast     NotifyClass = "toast"
	NotifyBadge     NotifyClass = "badge"
	NotifyPanel     NotifyClass = "panel"

	LinkLive  LinkState = "live"
	LinkStale LinkState = "stale"
	LinkDown  LinkState = "down"
	LinkOff   LinkState = "off"

	GeometrySourcePolygon GeometrySource = "polygon"
	GeometrySourceZone    GeometrySource = "zone"
	GeometrySourceNone    GeometrySource = "none"
)

// SeverityRank gives Severity a total order: Extreme=4 .. Unknown=0.
func SeverityRank(s Severity) int {
	switch s {
	case SeverityExtreme:
		return 4
	case SeveritySevere:
		return 3
	case SeverityModerate:
		return 2
	case SeverityMinor:
		return 1
	default:
		return 0
	}
}

// UrgencyRank gives Urgency a total order: Immediate=4 .. Unknown=0.
func UrgencyRank(u Urgency) int {
	switch u {
	case UrgencyImmediate:
		return 4
	case UrgencyExpected:
		return 3
	case UrgencyFuture:
		return 2
	case UrgencyPast:
		return 1
	default:
		return 0
	}
}

// TierRank gives Tier a total order: warning=4 .. statement=1 (0 for garbage).
func TierRank(t Tier) int {
	switch t {
	case TierWarning:
		return 4
	case TierWatch:
		return 3
	case TierAdvisory:
		return 2
	case TierStatement:
		return 1
	default:
		return 0
	}
}

// NotifyRank gives NotifyClass a total order: interrupt=3 .. panel=0.
func NotifyRank(c NotifyClass) int {
	switch c {
	case NotifyInterrupt:
		return 3
	case NotifyToast:
		return 2
	case NotifyBadge:
		return 1
	case NotifyPanel:
		return 0
	default:
		return 0
	}
}

// Louder returns the more attention-grabbing of two classes.
func Louder(a, b NotifyClass) NotifyClass {
	if NotifyRank(a) >= NotifyRank(b) {
		return a
	}
	return b
}

// Quieter returns the less attention-grabbing of two classes.
func Quieter(a, b NotifyClass) NotifyClass {
	if NotifyRank(a) <= NotifyRank(b) {
		return a
	}
	return b
}

// StepQuieter returns the class one step quieter than c (floor Panel).
func StepQuieter(c NotifyClass) NotifyClass {
	switch c {
	case NotifyInterrupt:
		return NotifyToast
	case NotifyToast:
		return NotifyBadge
	case NotifyBadge:
		return NotifyPanel
	default:
		return NotifyPanel
	}
}

// ParseSeverity accepts any case and returns SeverityUnknown on garbage.
func ParseSeverity(s string) Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "extreme":
		return SeverityExtreme
	case "severe":
		return SeveritySevere
	case "moderate":
		return SeverityModerate
	case "minor":
		return SeverityMinor
	default:
		return SeverityUnknown
	}
}

// ParseCertainty accepts any case and returns CertaintyUnknown on garbage.
func ParseCertainty(s string) Certainty {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "observed":
		return CertaintyObserved
	case "likely":
		return CertaintyLikely
	case "possible":
		return CertaintyPossible
	case "unlikely":
		return CertaintyUnlikely
	default:
		return CertaintyUnknown
	}
}

// ParseUrgency accepts any case and returns UrgencyUnknown on garbage.
func ParseUrgency(s string) Urgency {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "immediate":
		return UrgencyImmediate
	case "expected":
		return UrgencyExpected
	case "future":
		return UrgencyFuture
	case "past":
		return UrgencyPast
	default:
		return UrgencyUnknown
	}
}

// ParseStatus accepts any case; unrecognized/empty returns "" (caller decides
// whether an empty status means "drop it" — see decode.go).
func ParseStatus(s string) Status {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "actual":
		return StatusActual
	case "exercise":
		return StatusExercise
	case "system":
		return StatusSystem
	case "test":
		return StatusTest
	case "draft":
		return StatusDraft
	default:
		return ""
	}
}

// ParseMessageType accepts any case; unrecognized returns "".
func ParseMessageType(s string) MessageType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "alert":
		return MessageTypeAlert
	case "update":
		return MessageTypeUpdate
	case "cancel":
		return MessageTypeCancel
	default:
		return ""
	}
}

// LatLon is a point in decimal degrees.
type LatLon struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// BBox is a geographic bounding box. West > East signals it crosses the
// antimeridian.
type BBox struct {
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}

// CrossesAntimeridian reports whether the box wraps the +/-180 line.
func (b BBox) CrossesAntimeridian() bool { return b.West > b.East }

// Reference is one entry of the CAP supersession chain: {id, sent}.
type Reference struct {
	ID   string    `json:"id"`
	Sent time.Time `json:"sent"`
}

// Geometry is the alert's own polygon when NWS published one. It marshals as
// the raw GeoJSON geometry object (byte-compatible with Leaflet); Rings/BBox
// are the parsed form the matcher uses and are not part of the wire shape.
type Geometry struct {
	Type        string          `json:"type"` // "Polygon" | "MultiPolygon"
	Coordinates json.RawMessage `json:"coordinates"`
	Rings       [][]LatLon      `json:"-"` // one outer ring per polygon member
	BBox        BBox            `json:"-"`
}

// Alert is the CAP-shaped, provider-neutral record. Only NWS fills it in v1.
type Alert struct {
	ID             string              `json:"id"`             // properties.id ("urn:oid:…"), never the feature URL
	Provider       string              `json:"provider"`       // "nws"
	ProviderURL    string              `json:"providerUrl"`    // feature.id, for "Open on weather.gov"
	Event          string              `json:"event"`          // verbatim
	EffectiveEvent string              `json:"effectiveEvent"` // "Flash Flood Emergency" etc. when applicable, else == Event
	Tier           Tier                `json:"tier"`
	ShortCode      string              `json:"shortCode"`
	Headline       string              `json:"headline"`
	Description    string              `json:"description"`
	Instruction    string              `json:"instruction"` // "" when NWS sends null
	Response       string              `json:"response"`
	Category       string              `json:"category"`
	Severity       Severity            `json:"severity"`
	Certainty      Certainty           `json:"certainty"`
	Urgency        Urgency             `json:"urgency"`
	Status         Status              `json:"status"`
	MessageType    MessageType         `json:"messageType"`
	Sent           time.Time           `json:"sent"`
	Effective      time.Time           `json:"effective"`
	Onset          *time.Time          `json:"onset,omitempty"`
	Expires        time.Time           `json:"expires"`
	Ends           *time.Time          `json:"ends,omitempty"`
	SenderName     string              `json:"senderName"`
	Sender         string              `json:"sender"`
	SenderID       string              `json:"senderId"` // from VTEC office, else ""
	AreaDesc       string              `json:"areaDesc"`
	UGC            []string            `json:"ugc"`        // never nil; dedup union of geocode.UGC and affectedZones
	SAME           []string            `json:"same"`       // never nil
	References     []Reference         `json:"references"` // never nil
	Parameters     map[string][]string `json:"parameters"` // never nil ({})
	Geometry       *Geometry           `json:"geometry,omitempty"`
}

// EndTime is the time the UI counts down to: Ends, else Parameters
// "eventEndingTime", else Expires.
func (a Alert) EndTime() time.Time {
	if a.Ends != nil {
		return *a.Ends
	}
	if v, ok := a.Parameters["eventEndingTime"]; ok && len(v) > 0 {
		if t, err := time.Parse(time.RFC3339, v[0]); err == nil {
			return t.UTC()
		}
	}
	return a.Expires
}

// EndsOrExpires is an alias for EndTime kept for readability at call sites
// that are specifically about "ends vs expires", mirroring the tests spec.
func (a Alert) EndsOrExpires() time.Time { return a.EndTime() }

// HasPolygon reports whether NWS published its own geometry.
func (a Alert) HasPolygon() bool { return a.Geometry != nil }

// HasLocation reports whether the alert carries any geometry to match against
// at all — a polygon, or at least one UGC zone code.
func (a Alert) HasLocation() bool { return a.HasPolygon() || len(a.UGC) > 0 }

// IsTest reports whether Status is anything other than Actual.
func (a Alert) IsTest() bool { return a.Status != StatusActual }

// IsCancel reports whether this message cancels a prior alert.
func (a Alert) IsCancel() bool { return a.MessageType == MessageTypeCancel }

// ReplacesIDs returns References[i].ID for all i (helper for supersession).
func (a Alert) ReplacesIDs() []string {
	ids := make([]string, 0, len(a.References))
	for _, r := range a.References {
		ids = append(ids, r.ID)
	}
	return ids
}

// VTECAction returns the action code ("NEW","CON","EXT","CAN","EXP", ...)
// from the first VTEC parameter string, or "" when absent/unparsable.
func (a Alert) VTECAction() string {
	v, ok := a.Parameters["VTEC"]
	if !ok || len(v) == 0 {
		return ""
	}
	parts := strings.Split(strings.Trim(v[0], "/"), ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// RouteSpan is a contiguous stretch of one route/line annotation touched by
// an alert (own-geometry fallback ribbon needs vertex indexes to draw it).
type RouteSpan struct {
	AnnotationID string `json:"annotationId"`
	StartIndex   int    `json:"startIndex"`
	EndIndex     int    `json:"endIndex"`
	UGC          string `json:"ugc,omitempty"`
}

// AffectedItem is one thing of ours an alert touches.
type AffectedItem struct {
	Kind      string  `json:"kind"` // "checkpoint" | "location" | "station" | "own"
	ID        string  `json:"id"`   // annotation id | callsign | "own"
	CheckInID string  `json:"checkInId,omitempty"`
	Label     string  `json:"label"`
	ShortName string  `json:"shortName,omitempty"`
	Seq       int     `json:"seq,omitempty"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	UGC       string  `json:"ugc,omitempty"` // zone this item resolved to (zone-only alerts)
}

// Affects is "what of ours it touches", computed server-side.
type Affects struct {
	Summary            string         `json:"summary"` // <=80 runes, "CP 4-CP 7 - Aid 2 - 3 stations", "+N more"
	EntireCourse       bool           `json:"entireCourse"`
	RouteMiles         float64        `json:"routeMiles"`
	Checkpoints        []AffectedItem `json:"checkpoints"`        // never nil, sorted by Seq
	Locations          []AffectedItem `json:"locations"`          // never nil
	Stations           []AffectedItem `json:"stations"`           // never nil; roster + tracked + own
	CheckpointSeqRange []int          `json:"checkpointSeqRange"` // [] or [lo, hi]
	RouteSpans         []RouteSpan    `json:"routeSpans"`         // never nil
}

// NetAck records the NCS "ack for net" — clears the banner for everyone.
type NetAck struct {
	UserID   string    `json:"userId"`
	UserName string    `json:"userName"`
	Callsign string    `json:"callsign"`
	At       time.Time `json:"at"`
}

// ZoneRef is a lightweight reference to a zone: enough to render a chip
// without shipping the whole polygon.
type ZoneRef struct {
	UGC    string `json:"ugc"`
	Name   string `json:"name"` // "" until cached
	State  string `json:"state"`
	Type   string `json:"type"` // "county"|"forecast"|"fire"|"marine"|"unknown"
	Cached bool   `json:"cached"`
}

// MatchedAlert is what every API/WS surface returns: the alert plus
// everything derived for the requesting net's footprint.
type MatchedAlert struct {
	Alert
	State          AlertState  `json:"state"`
	EndedAt        *time.Time  `json:"endedAt,omitempty"`
	EndedReason    string      `json:"endedReason,omitempty"` // "expired"|"cancelled"|"dropped"|"clock"
	EndsAt         time.Time   `json:"endsAt"`
	Proximity      Proximity   `json:"proximity"` // "in"|"near" on the wire (far never sent)
	DistanceMiles  float64     `json:"distanceMiles"`
	BearingDeg     int         `json:"bearingDeg"`
	NotifyClass    NotifyClass `json:"notifyClass"`
	NotifyReason   string      `json:"notifyReason"` // "new"|"update"|"escalated"|"ended"|"none"
	Floored        bool        `json:"floored"`
	Affects        Affects     `json:"affects"`
	GeometrySource string      `json:"geometrySource"` // "polygon"|"zone"|"none"
	Zones          []ZoneRef   `json:"zones"`          // never nil; one per UGC
	ReplacedBy     string      `json:"replacedBy,omitempty"`
	AckedForNet    *NetAck     `json:"ackedForNet,omitempty"`
	FetchedAt      time.Time   `json:"fetchedAt"`
	FirstSeenAt    time.Time   `json:"firstSeenAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
	NetID          string      `json:"netId"` // "" = no-net footprint
}

// EffectivePolicy is the resolved (config + per-net) notification policy,
// echoed on every snapshot so the UI never guesses.
type EffectivePolicy struct {
	NetID           string   `json:"netId"`
	BufferMiles     float64  `json:"bufferMiles"`
	InterruptEvents []string `json:"interruptEvents"`
	InterruptCustom bool     `json:"interruptCustom"`
	WatchNotify     string   `json:"watchNotify"`     // "toast"|"badge"
	AdvisoryNotify  string   `json:"advisoryNotify"`  // "badge"|"panel"
	StatementNotify string   `json:"statementNotify"` // "panel"|"badge"
	MuteAdvisories  bool     `json:"muteAdvisories"`
	FloorText       string   `json:"floorText"`
}

// NetWatch is the per-net weather-watch configuration (GET/PUT /nets/{id}/wxwatch).
type NetWatch struct {
	NetID           string          `json:"netId"`
	BufferMiles     float64         `json:"bufferMiles"` // 0 = inherit config default
	ExtraZones      []string        `json:"extraZones"`  // never nil
	MuteAdvisories  bool            `json:"muteAdvisories"`
	InterruptCustom bool            `json:"interruptCustom"` // false = inherit config allowlist
	InterruptEvents []string        `json:"interruptEvents"` // never nil; used only when custom
	Effective       EffectivePolicy `json:"effective"`       // read-only echo; ignored on PUT
}

// NetWatchZones is the resolved zone picker data for a net's watch area.
type NetWatchZones struct {
	Resolved  []ZoneRef `json:"resolved"`  // the IN set
	Neighbors []ZoneRef `json:"neighbors"` // the NEAR set union extra
}

// FootprintSummary is what the per-net "Weather watch area" screen shows.
type FootprintSummary struct {
	NetID             string    `json:"netId"` // "" when no net
	BufferMiles       float64   `json:"bufferMiles"`
	RouteMiles        float64   `json:"routeMiles"`
	LocationCount     int       `json:"locationCount"`
	CheckpointCount   int       `json:"checkpointCount"`
	RosterPositions   int       `json:"rosterPositions"`
	TrackedPositions  int       `json:"trackedPositions"`
	OwnStation        string    `json:"ownStation"` // "gps" | "config" | "none"
	OwnStationAgeSec  int       `json:"ownStationAgeSec"`
	Zones             []ZoneRef `json:"zones"`      // IN set, never nil
	NearZones         []ZoneRef `json:"nearZones"`  // never nil
	ExtraZones        []string  `json:"extraZones"` // never nil
	UnresolvedSamples int       `json:"unresolvedSamples"`
	Centroid          *LatLon   `json:"centroid,omitempty"`
	Empty             bool      `json:"empty"`
	ComputedAt        time.Time `json:"computedAt"`
}

// LinkReason explains a State of "off". "off" is a valid, non-error status —
// the client must be able to tell "the operator turned it off" from "the
// link is down", and to tell either from "enabled but contact not set",
// because those are three different fixes.
type LinkReason string

const (
	// ReasonDisabled — wx_alerts.enabled is false. The operator turned it off.
	ReasonDisabled LinkReason = "disabled"
	// ReasonContactMissing — enabled with no contact. config.Validate normally
	// prevents this; kept so a config that bypasses validation still reports
	// the actual fix rather than claiming the feature is off.
	ReasonContactMissing LinkReason = "contactMissing"
	// ReasonNoWatchArea — enabled and running, but no watch area has resolved
	// yet (cold start: zone lookups are bounded per refresh), so the upstream
	// query is empty and NOTHING is being fetched. Must never be reported as a
	// healthy link: a green "updated just now" over an empty pipe is the exact
	// failure the provenance design exists to prevent.
	ReasonNoWatchArea LinkReason = "noWatchArea"
	// ReasonInitFailed — configured and enabled, but the manager did not
	// start (see app.go). NOT the same as "off": nothing is switched off and
	// the operator's fix is to read the log, not to flip a toggle.
	ReasonInitFailed LinkReason = "initFailed"
)

// LinkStatus reports the poller's connection health.
type LinkStatus struct {
	State               LinkState  `json:"state"` // "live"|"stale"|"down"|"off"
	Reason              LinkReason `json:"reason,omitempty"`
	Enabled             bool       `json:"enabled"`
	ContactConfigured   bool       `json:"contactConfigured"`
	LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`
	LastAttemptAt       *time.Time `json:"lastAttemptAt,omitempty"`
	NextAttemptAt       *time.Time `json:"nextAttemptAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	FromCache           bool       `json:"fromCache"`
	RegionCount         int        `json:"regionCount"`
	InAreaCount         int        `json:"inAreaCount"`
	NearbyCount         int        `json:"nearbyCount"`
	ZonesCached         int        `json:"zonesCached"`
	ZonesMissing        int        `json:"zonesMissing"`
	Sounds              bool       `json:"sounds"`
}

// Snapshot is the top-level payload of GET /wx/alerts and the wx_alerts WS event.
type Snapshot struct {
	Alerts    []MatchedAlert    `json:"alerts"` // never nil
	Status    LinkStatus        `json:"status"`
	Footprint *FootprintSummary `json:"footprint"` // null only before the first build
	Policy    EffectivePolicy   `json:"policy"`
}

// ZoneRecord is the cached shape of GET /wx/zones/{ugc}.
type ZoneRecord struct {
	UGC       string          `json:"ugc"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	State     string          `json:"state"`
	Geometry  json.RawMessage `json:"geometry"` // Polygon | MultiPolygon (GeometryCollection flattened)
	FetchedAt time.Time       `json:"fetchedAt"`

	rings [][]LatLon // parsed lazily; not serialised
	holes [][]LatLon // parsed lazily; not serialised
}

// EventType is one row of GET /wx/event-types.
type EventType struct {
	Event string `json:"event"`
	Tier  Tier   `json:"tier"`
}
