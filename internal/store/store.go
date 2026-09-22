package store

import (
	"time"

	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/station"
)

// ActivityLogEntry represents a logged action.
type ActivityLogEntry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"userId,omitempty"`
	UserName  string    `json:"userName,omitempty"`
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
	Details   string    `json:"details,omitempty"`
}

// ActivityFilter controls activity log queries.
type ActivityFilter struct {
	Since  *time.Time
	Until  *time.Time
	UserID string
	Action string
	Offset int
	Limit  int
}

// Net represents a net control session.
type Net struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	Frequency      string     `json:"frequency"`
	NCSCallsign    string     `json:"ncsCallsign"`
	NCSUserID      string     `json:"ncsUserId"`
	Status         string     `json:"status"`
	OpenedAt       *time.Time `json:"openedAt,omitempty"`
	ClosedAt       *time.Time `json:"closedAt,omitempty"`
	Notes          string     `json:"notes"`
	MissionBrief   string     `json:"missionBrief"`
	OpsViewLat     *float64   `json:"opsViewLat,omitempty"`
	OpsViewLon     *float64   `json:"opsViewLon,omitempty"`
	OpsViewZoom    *float64   `json:"opsViewZoom,omitempty"`
	PinnedStations []string   `json:"pinnedStations"`

	// NWS weather watch area (internal/wxalert), per net. WxBufferMiles 0
	// means "inherit the config default"; WxInterruptEvents is used only
	// when WxInterruptCustom is true (false means "inherit the config
	// allowlist"). The two slices are never nil ('[]' SQL default).
	WxBufferMiles     float64  `json:"wxBufferMiles"`
	WxExtraZones      []string `json:"wxExtraZones"`
	WxMuteAdvisories  bool     `json:"wxMuteAdvisories"`
	WxInterruptCustom bool     `json:"wxInterruptCustom"`
	WxInterruptEvents []string `json:"wxInterruptEvents"`

	// Profile selects the net-control vocabulary set (internal/netprofile):
	// "general" (default, zero behavior change) or "bike-ride". Never ""
	// after CreateNet or any Load* — the store normalizes ""->"general" on
	// load, the manager on create.
	Profile string `json:"profile"`
}

// TrackedStation represents a device linked to a checked-in operator.
type TrackedStation struct {
	Callsign   string `json:"callsign"`
	AutoLinked bool   `json:"autoLinked"`
}

// NetCheckIn represents an operator check-in to a net.
type NetCheckIn struct {
	ID              string           `json:"id"`
	NetID           string           `json:"netId"`
	Callsign        string           `json:"callsign"`
	TacticalCall    string           `json:"tacticalCall"`
	OperatorName    string           `json:"operatorName"`
	Status          string           `json:"status"`
	Traffic         string           `json:"traffic"`
	Source          string           `json:"source"`
	Category        string           `json:"category"`
	Location        string           `json:"location"`
	Lat             *float64         `json:"lat,omitempty"`
	Lon             *float64         `json:"lon,omitempty"`
	MissionIDs      []string         `json:"missionIds"`
	TrackedStations []TrackedStation `json:"trackedStations"`
	CheckedInAt     time.Time        `json:"checkedInAt"`
	CheckedOutAt    *time.Time       `json:"checkedOutAt,omitempty"`
	LastHeard       time.Time        `json:"lastHeard"`
	MissedRollCalls int              `json:"missedRollCalls"`
}

// NetMission represents a task assigned during a net.
type NetMission struct {
	ID          string `json:"id"`
	NetID       string `json:"netId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Status      string `json:"status"`
	// Deprecated: superseded by NetCheckIn.MissionIDs. Accepted as a
	// create-time assignee input only; never persisted, always "" on load.
	AssignedTo  string     `json:"assignedTo"`
	Location    string     `json:"location"`
	Lat         *float64   `json:"lat,omitempty"`
	Lon         *float64   `json:"lon,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// NetNote represents a note attached to a net or check-in.
type NetNote struct {
	ID         string    `json:"id"`
	NetID      string    `json:"netId"`
	CheckInID  string    `json:"checkInId,omitempty"`
	MissionID  string    `json:"missionId,omitempty"`
	AuthorID   string    `json:"authorId"`
	AuthorName string    `json:"authorName"`
	Content    string    `json:"content"`
	Category   string    `json:"category"`
	Severity   string    `json:"severity,omitempty"`
	Pinned     bool      `json:"pinned"`
	CreatedAt  time.Time `json:"createdAt"`
}

// NetEvent represents a timeline event in a net.
type NetEvent struct {
	ID        string    `json:"id"`
	NetID     string    `json:"netId"`
	Type      string    `json:"type"`
	Callsign  string    `json:"callsign"`
	Summary   string    `json:"summary"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"createdAt"`
}

// TacticalAlias maps an APRS callsign to a tactical name.
type TacticalAlias struct {
	Callsign   string    `json:"callsign"`
	Alias      string    `json:"alias"`
	AssignedBy string    `json:"assignedBy"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Annotation represents a local map annotation.
type Annotation struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	Label         string     `json:"label"`
	Description   string     `json:"description,omitempty"`
	Geometry      string     `json:"geometry"`
	Style         string     `json:"style,omitempty"`
	CreatedBy     string     `json:"createdBy,omitempty"`
	CreatedByName string     `json:"createdByName,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	Category      string     `json:"category"`
	Status        string     `json:"status"`
	Priority      string     `json:"priority"`
	OperationID   string     `json:"operationId,omitempty"`
	MissionIDs    []string   `json:"missionIds"`
	Resources     string     `json:"resources,omitempty"`
	ReportedBy    string     `json:"reportedBy,omitempty"`
	ReportedAt    *time.Time `json:"reportedAt,omitempty"`
	ResolvedAt    *time.Time `json:"resolvedAt,omitempty"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	NetID         string     `json:"netId,omitempty"`
	ShortName     string     `json:"shortName,omitempty"`
	SortOrder     int        `json:"sortOrder"`
	BatchID       string     `json:"batchId,omitempty"`
	BatchLabel    string     `json:"batchLabel,omitempty"`
}

// Operation represents a named grouping of annotations for a specific event or mission.
type Operation struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"createdBy,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ArchivedAt  *time.Time `json:"archivedAt,omitempty"`
}

// AnnotationFilter controls filtered annotation queries.
type AnnotationFilter struct {
	Category       string
	Status         string
	Priority       string
	OperationID    string
	IncludeExpired bool
	NetID          string
	BatchID        string
}

// WeatherReading represents a single weather observation stored in the database.
type WeatherReading struct {
	ID          int64     `json:"id"`
	Callsign    string    `json:"callsign"`
	Timestamp   time.Time `json:"timestamp"`
	Temperature *float64  `json:"temperature,omitempty"`
	WindDir     *float64  `json:"windDir,omitempty"`
	WindSpeed   *float64  `json:"windSpeed,omitempty"`
	WindGust    *float64  `json:"windGust,omitempty"`
	Humidity    *int      `json:"humidity,omitempty"`
	Pressure    *float64  `json:"pressure,omitempty"`
	Rain1h      *float64  `json:"rain1h,omitempty"`
	Rain24h     *float64  `json:"rain24h,omitempty"`
	RainToday   *float64  `json:"rainToday,omitempty"`
	Luminosity  *int      `json:"luminosity,omitempty"`
}

// WeatherFilter controls weather reading queries.
type WeatherFilter struct {
	Callsign string
	Since    *time.Time
	Until    *time.Time
	Limit    int
}

// TelemetryReading represents a single telemetry observation stored in the database.
type TelemetryReading struct {
	ID        int64     `json:"id"`
	Callsign  string    `json:"callsign"`
	Timestamp time.Time `json:"timestamp"`
	Seq       int       `json:"seq"`
	Analog1   float64   `json:"analog1"`
	Analog2   float64   `json:"analog2"`
	Analog3   float64   `json:"analog3"`
	Analog4   float64   `json:"analog4"`
	Analog5   float64   `json:"analog5"`
	Digital   int       `json:"digital"`
}

// TelemetryFilter controls telemetry reading queries.
type TelemetryFilter struct {
	Callsign string
	Since    *time.Time
	Until    *time.Time
	Limit    int
}

// CheckpointMeta holds checkpoint-specific metadata for progress tracking.
type CheckpointMeta struct {
	AnnotationID   string     `json:"annotationId"`
	NetID          string     `json:"netId"`
	SequenceNumber int        `json:"sequenceNumber"`
	ExpectedTime   *time.Time `json:"expectedTime,omitempty"`
	OpenedAt       *time.Time `json:"openedAt,omitempty"`
	ClosedAt       *time.Time `json:"closedAt,omitempty"`
}

// CheckpointPassage records an element passing through a checkpoint.
type CheckpointPassage struct {
	ID           string    `json:"id"`
	CheckpointID string    `json:"checkpointId"`
	NetID        string    `json:"netId"`
	Label        string    `json:"label"`
	PassageTime  time.Time `json:"passageTime"`
	Direction    string    `json:"direction"`
	ReportedBy   string    `json:"reportedBy"`
	Notes        string    `json:"notes,omitempty"`
}

// RideRoute is one distance option a ride event offers (e.g. a 100/75/55/48
// mile choice). ID is the stable key later ride-mode records reference (SAG
// pickup route, lead/trailing edges); Name is what the agency calls it on
// the air.
type RideRoute struct {
	ID            string     `json:"id"`                  // lowercase slug, unique within the net, e.g. "100", "metric"
	Name          string     `json:"name"`                // "100 Mile Century"
	DistanceMiles float64    `json:"distanceMiles"`       // > 0
	StartTime     *time.Time `json:"startTime,omitempty"` // route-specific finish cut-off, nil = use Cutoff.CourseClosesAt
	CutoffAt      *time.Time `json:"cutoffAt,omitempty"`
	Division      string     `json:"division,omitempty"` // nullable division for future parallel nets; "" in v1
}

// RideCutoffPolicy is the event-wide rule set applied to a ride net.
// Individual shutoff points (coordinate + wall clock + reroute) are a later
// ride-mode package's concern; this is the policy those points are applied
// under.
type RideCutoffPolicy struct {
	CourseOpensAt  *time.Time `json:"courseOpensAt,omitempty"`
	CourseClosesAt *time.Time `json:"courseClosesAt,omitempty"`
	// Riders still on course after a cut-off are offered (false) or required
	// (true) to take SAG; either way they leave the supported set.
	MandatorySAGAfterCutoff bool `json:"mandatorySagAfterCutoff"`
	// A rider who declines SAG becomes "unsupported". Shipped true; exposed
	// because some agencies keep declining riders in the count.
	DeclinedSAGIsUnsupported bool   `json:"declinedSagIsUnsupported"`
	Notes                    string `json:"notes"`
}

// PriorityTier is one rung of a traffic-priority ladder (internal/netprofile
// ships the defaults). Rank 1 is the most urgent. Records store the tier ID
// string, never the rank or label.
type PriorityTier struct {
	ID          string   `json:"id"`          // lowercase slug: "emergency", "priority", "high", "medium", "low"
	Label       string   `json:"label"`       // on-air word, upper-cased by the UI: "EMERGENCY"
	Rank        int      `json:"rank"`        // 1..N, contiguous, unique
	Description string   `json:"description"` // one sentence of intent
	Examples    []string `json:"examples"`    // never nil
}

// NetRideConfig is the per-net agency configuration for a "bike-ride"
// profile net. Exactly one row per net with Profile == "bike-ride" (created
// with defaults when the profile is set, so GET never 404s).
type NetRideConfig struct {
	NetID      string           `json:"netId"`
	AgencyName string           `json:"agencyName"` // "Marin Cyclists", "Bike MS"
	EventName  string           `json:"eventName"`  // "Jack and Back 2026"
	EventDate  string           `json:"eventDate"`  // "YYYY-MM-DD" or ""
	Routes     []RideRoute      `json:"routes"`     // never nil
	Cutoff     RideCutoffPolicy `json:"cutoff"`
	// Some events withhold even the bib for severe injuries. Medical
	// notification handling reads this to redact the bib on the
	// timeline/broadcast.
	WithholdBibOnSevereInjury bool `json:"withholdBibOnSevereInjury"`
	// Empty = use the profile's shipped ladder (netprofile.EffectiveTiers).
	// Non-empty = the complete ladder for this net.
	PriorityTiers []PriorityTier `json:"priorityTiers"` // never nil
	Division      string         `json:"division,omitempty"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

// SAGLocation is a place expressed the way it is said on the air: route-
// relative first, lat/lon second. Any subset of fields may be set; Kind is
// required. Pickup kinds: course | reststop | checkpoint | start | finish |
// other. Dropoff kinds: next_reststop | reststop | finish | start |
// hospital | other.
type SAGLocation struct {
	Kind           string   `json:"kind"`
	AnnotationID   string   `json:"annotationId,omitempty"`   // rest stop / checkpoint / start / finish annotation when Kind names one
	Route          string   `json:"route,omitempty"`          // agency route label, e.g. "100M", "Day 1 48M"
	MileMarker     *float64 `json:"mileMarker,omitempty"`     // miles from route start (frontend chainage)
	MilesRemaining *float64 `json:"milesRemaining,omitempty"` // as reported on the air; not converted server-side
	Lat            *float64 `json:"lat,omitempty"`
	Lon            *float64 `json:"lon,omitempty"`
	Description    string   `json:"description,omitempty"` // "just past the cattle guard on Skyline"
}

// SAGSlot is one rider on a request. A bib is a label, never a key; there is
// no rider table. RiderName is recorded only for hospital/start transports.
type SAGSlot struct {
	ID          string       `json:"id"`
	Bib         string       `json:"bib,omitempty"`
	RiderName   string       `json:"riderName,omitempty"`
	Note        string       `json:"note,omitempty"`
	HasBike     bool         `json:"hasBike"`     // legacy mirror of Bike == "with_rider"; kept in step by ride.NormalizeSlotBike
	Bike        string       `json:"bike"`        // ride.Bike*: a SEPARATE axis from Disposition; only "with_rider" consumes a rack
	Disposition string       `json:"disposition"` // see internal/ride's state machine
	LegID       string       `json:"legId,omitempty"`
	DeliveredTo *SAGLocation `json:"deliveredTo,omitempty"` // actual dropoff, set on delivery
	UpdatedAt   time.Time    `json:"updatedAt"`
}

// SAGLeg is one vehicle's trip against one request: a subset of the
// request's slots from pickup to dropoff. Partial pickup = two legs.
type SAGLeg struct {
	ID               string     `json:"id"`
	VehicleCheckInID string     `json:"vehicleCheckInId"` // NetCheckIn.ID, Category must be "sag"
	VehicleLabel     string     `json:"vehicleLabel"`     // tactical call or callsign at dispatch time
	SlotIDs          []string   `json:"slotIds"`          // never nil
	Status           string     `json:"status"`
	ReleaseReason    string     `json:"releaseReason,omitempty"`
	Overcommitted    bool       `json:"overcommitted"` // NCS knowingly exceeded the BIKE RACK count (seats are a hard stop and can never be exceeded)
	DispatchedAt     time.Time  `json:"dispatchedAt"`
	EnrouteAt        *time.Time `json:"enrouteAt,omitempty"`
	OnSceneAt        *time.Time `json:"onSceneAt,omitempty"`
	LoadedAt         *time.Time `json:"loadedAt,omitempty"`
	DeliveredAt      *time.Time `json:"deliveredAt,omitempty"`
	ReleasedAt       *time.Time `json:"releasedAt,omitempty"`
}

// SAGRequest is a transport job (NOT a NetMission — a mission is one task at
// one location, a SAG request is a transport job). Status is DERIVED from
// Slots+Legs by ride.DeriveStatus and persisted only so the board can
// sort/filter.
type SAGRequest struct {
	ID            string      `json:"id"`
	NetID         string      `json:"netId"`
	Division      *string     `json:"division,omitempty"` // nullable; unused in v1
	Sequence      int         `json:"sequence"`           // per-net human number "SAG 7"
	Pickup        SAGLocation `json:"pickup"`
	Dropoff       SAGLocation `json:"dropoff"`
	Reason        string      `json:"reason"`   // agency vocabulary; free string accepted
	Priority      string      `json:"priority"` // ride.Config.Priorities
	Status        string      `json:"status"`
	NeedsVehicle  bool        `json:"needsVehicle"` // derived: waiting slots with no active leg
	Slots         []SAGSlot   `json:"slots"`        // never nil, len >= 1
	Legs          []SAGLeg    `json:"legs"`         // never nil
	RequestedBy   string      `json:"requestedBy"`  // reporting station callsign/tactical
	CreatedByName string      `json:"createdByName,omitempty"`
	Notes         string      `json:"notes"`
	CancelReason  string      `json:"cancelReason,omitempty"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	ClosedAt      *time.Time  `json:"closedAt,omitempty"` // set when Status becomes complete|cancelled
}

// SAGVehicle is per-vehicle configured capacity, keyed by the check-in.
// Capacity is per-vehicle configurable; committed/available is always
// derived from assigned requests, never stored.
type SAGVehicle struct {
	NetID     string    `json:"netId"`
	CheckInID string    `json:"checkInId"`
	Division  *string   `json:"division,omitempty"`
	Seats     int       `json:"seats"`
	RackSlots int       `json:"rackSlots"`
	Notes     string    `json:"notes"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CourseConfig is the per-net, agency-editable closure policy for
// internal/course. Division is "" for the single-net v1 (never a pointer;
// ” SQL default) so parallel Route/RestStop/Medical/Supply nets can filter
// on it later without a migration.
type CourseConfig struct {
	NetID                string    `json:"netId"`
	Division             string    `json:"division"`
	SweepLabel           string    `json:"sweepLabel"`           // passage Label that means "the sweep vehicle"; default "SWEEP"
	LeadLabel            string    `json:"leadLabel"`            // default "LEAD"
	CloseRequiresSweep   bool      `json:"closeRequiresSweep"`   // default true; false = agency lets stops close on time cut-off
	AutoSweepFromPassage bool      `json:"autoSweepFromPassage"` // default true; a SweepLabel passage flips the station to sweep_passed
	UpdatedAt            time.Time `json:"updatedAt"`
}

// ShutoffPoint is a scheduled course closure (internal/course). Firing it is
// an event, not an edit.
type ShutoffPoint struct {
	ID                  string     `json:"id"`
	NetID               string     `json:"netId"`
	Division            string     `json:"division"`
	Name                string     `json:"name"` // "Benson Shutoff"
	Lat                 float64    `json:"lat"`
	Lon                 float64    `json:"lon"`
	RouteMile           *float64   `json:"routeMile,omitempty"`    // operator-entered route-relative position
	AnnotationID        string     `json:"annotationId,omitempty"` // optional: coincides with an existing net annotation
	ScheduledAt         time.Time  `json:"scheduledAt"`            // wall clock, stored UTC, displayed in net-local time
	RerouteDirection    string     `json:"rerouteDirection"`       // "West"
	RerouteDestination  string     `json:"rerouteDestination"`     // "55-mile route"
	RerouteInstructions string     `json:"rerouteInstructions"`    // full on-air phrasing
	StaffedByCheckInID  string     `json:"staffedByCheckInId"`     // NetCheckIn.ID, category sag/marshal expected but not enforced
	Status              string     `json:"status"`                 // planned | fired | cancelled
	FiredAt             *time.Time `json:"firedAt,omitempty"`
	FiredBy             string     `json:"firedBy"` // callsign/user name
	FireNote            string     `json:"fireNote"`
	RerouteCount        int        `json:"rerouteCount"` // riders directed onto the reroute without a bib recorded
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

// RiderException is the ONLY place an individual rider appears
// (internal/course). Bib is a label; there is no rider table and nothing
// joins on Bib.
type RiderException struct {
	ID              string    `json:"id"`
	NetID           string    `json:"netId"`
	Division        string    `json:"division"`
	Bib             string    `json:"bib"`           // "" when withheld or unknown
	BibWithheld     bool      `json:"bibWithheld"`   // per-net privacy policy or operator choice
	Kind            string    `json:"kind"`          // sag | medical | dnf | declined_sag | shutoff_reroute | other
	SupportStatus   string    `json:"supportStatus"` // supported | unsupported
	Reason          string    `json:"reason"`
	RouteLabel      string    `json:"routeLabel"` // which route ("100-mile"); free text
	RouteMile       *float64  `json:"routeMile,omitempty"`
	Lat             *float64  `json:"lat,omitempty"`
	Lon             *float64  `json:"lon,omitempty"`
	ShutoffID       string    `json:"shutoffId,omitempty"`    // set when kind == shutoff_reroute
	SAGRequestID    string    `json:"sagRequestId,omitempty"` // opaque string link, no FK
	ReportedBy      string    `json:"reportedBy"`
	RecordedAt      time.Time `json:"recordedAt"`
	StatusChangedAt time.Time `json:"statusChangedAt"`
	StatusChangedBy string    `json:"statusChangedBy"`
	Note            string    `json:"note"`
}

// SweepReport is one "sweep here, last rider is bib X doing Y mph" report
// (internal/course).
type SweepReport struct {
	ID                string    `json:"id"`
	NetID             string    `json:"netId"`
	Division          string    `json:"division"`
	RouteLabel        string    `json:"routeLabel"`
	CheckInID         string    `json:"checkInId"` // sweep vehicle's NetCheckIn, optional
	ReportedBy        string    `json:"reportedBy"`
	RouteMile         *float64  `json:"routeMile,omitempty"`
	Lat               *float64  `json:"lat,omitempty"`
	Lon               *float64  `json:"lon,omitempty"`
	LastRiderBib      string    `json:"lastRiderBib"` // last SUPPORTED rider
	EstimatedSpeedMph *float64  `json:"estimatedSpeedMph,omitempty"`
	Note              string    `json:"note"`
	ReportedAt        time.Time `json:"reportedAt"`
}

// StationClosure is the closure ladder for one sequenced station (a
// checkpoint/aid/start/finish annotation that has CheckpointMeta), keyed by
// (net, checkpoint). internal/course.
type StationClosure struct {
	NetID            string     `json:"netId"`
	CheckpointID     string     `json:"checkpointId"` // Annotation.ID
	Division         string     `json:"division"`
	State            string     `json:"state"` // open | riders_clear | sweep_passed | closed
	RidersClearAt    *time.Time `json:"ridersClearAt,omitempty"`
	RidersClearBy    string     `json:"ridersClearBy"`
	SweepPassedAt    *time.Time `json:"sweepPassedAt,omitempty"`
	SweepPassedBy    string     `json:"sweepPassedBy"`
	SweepPassageID   string     `json:"sweepPassageId,omitempty"` // CheckpointPassage.ID when auto-set
	ClosedAt         *time.Time `json:"closedAt,omitempty"`
	ClosedBy         string     `json:"closedBy"`
	ClosedByOverride bool       `json:"closedByOverride"`
	OverrideReason   string     `json:"overrideReason"`
	ReopenCount      int        `json:"reopenCount"`
	Note             string     `json:"note"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// RidePhaseState is the current ride phase for one net (internal/ride/phase,
// WP5b): pre-start | launched | mid-ride | closing | collapse | reconcile.
// One row per net, only meaningful for a net whose profile is bike-ride.
// Phase is ALWAYS operator-set — see that package's doc comment for the
// legal transition table and why nothing auto-advances it. Reason is
// required by the manager when SetBy moves the phase backward (a
// correction), optional otherwise.
type RidePhaseState struct {
	NetID     string    `json:"netId"`
	Phase     string    `json:"phase"`
	SetBy     string    `json:"setBy"`
	Reason    string    `json:"reason"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// --- Ride traffic (WP4): supply requests and medical notifications ---

// Supply request statuses (see internal/ride's supply state machine).
const (
	SupplyDraft     = "draft"
	SupplyConfirmed = "confirmed" // read-back confirmed by requester; now "transmitted"
	SupplyRelayed   = "relayed"   // handed to supplier/logistics
	SupplyEnRoute   = "en_route"  // supplier gave an ETA
	SupplyDelivered = "delivered"
	SupplyCancelled = "cancelled"
	SupplyMerged    = "merged" // absorbed into MergedIntoID
)

// SupplyItem is one line of a (possibly batched) request. Free text on
// purpose: the item vocabulary is agency data (see SupplyCatalogEntry), not
// an enum.
type SupplyItem struct {
	Item     string    `json:"item"`           // "ice", "water", "bananas", "tubes 700x25"
	Quantity float64   `json:"quantity"`       // 0 = unspecified
	Unit     string    `json:"unit,omitempty"` // "bags", "gal", "cases", "ea"
	Note     string    `json:"note,omitempty"`
	AddedAt  time.Time `json:"addedAt"` // batching: which items came from "what else?"
}

// SupplyETA is one supplier ETA statement. History is kept because the
// dashboard question is "how long since they SAID 20 minutes", and
// suppliers revise.
type SupplyETA struct {
	Minutes int       `json:"minutes"`
	GivenAt time.Time `json:"givenAt"`
	DueAt   time.Time `json:"dueAt"`            // GivenAt + Minutes; stored so the UI never recomputes
	Source  string    `json:"source,omitempty"` // "ice truck via phone", "SUPPLY 2"
}

// SupplyRequest is a batchable supply/logistics request from a rest stop or
// checkpoint. Rider accounting is edge-based elsewhere in ride mode; this
// record never references an individual rider.
type SupplyRequest struct {
	ID       string  `json:"id"`
	NetID    string  `json:"netId"`
	Division *string `json:"division,omitempty"` // governing fact 9; nil in v1

	// Who/where. RequestedByCall is a snapshot (tactical call preferred) so
	// the record survives the check-in checking out.
	RequestedByCheckInID string   `json:"requestedByCheckInId,omitempty"`
	RequestedByCall      string   `json:"requestedByCall"`
	Location             string   `json:"location"`                       // on-air text: "Rest Stop 3 / Nicasio"
	LocationAnnotationID string   `json:"locationAnnotationId,omitempty"` // aid/checkpoint annotation if known
	MilesRemaining       *float64 `json:"milesRemaining,omitempty"`       // governing fact 2: primary coordinate
	RouteID              string   `json:"routeId,omitempty"`              // WP1 route config id; "" in v1 single-route
	Lat                  *float64 `json:"lat,omitempty"`
	Lon                  *float64 `json:"lon,omitempty"`

	Items         []SupplyItem `json:"items"`         // never nil; '[]' SQL default
	AskedWhatElse bool         `json:"askedWhatElse"` // published batching rule; recorded, not gated
	Priority      string       `json:"priority"`      // net's tier vocabulary (ride.Policy.ValidTier)
	Notes         string       `json:"notes,omitempty"`
	Status        string       `json:"status"`

	// Ladder timestamps — each step separately.
	CreatedAt    time.Time   `json:"createdAt"`
	ReadBackAt   *time.Time  `json:"readBackAt,omitempty"`
	ReadBackBy   string      `json:"readBackBy,omitempty"` // callsign who confirmed
	RelayedAt    *time.Time  `json:"relayedAt,omitempty"`
	RelayedTo    string      `json:"relayedTo,omitempty"` // "SUPPLY 1", "Event logistics (phone)"
	ETAs         []SupplyETA `json:"etas"`                // never nil
	DeliveredAt  *time.Time  `json:"deliveredAt,omitempty"`
	CancelledAt  *time.Time  `json:"cancelledAt,omitempty"`
	CancelReason string      `json:"cancelReason,omitempty"`
	MergedIntoID string      `json:"mergedIntoId,omitempty"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

// Medical notification statuses (see internal/ride's medical state
// machine).
const (
	MedReported   = "reported"    // fields taken, read-back not yet confirmed
	MedConfirmed  = "confirmed"   // read-back confirmed -> notification is transmitted
	MedEMSEnRoute = "ems_enroute" // ETA given
	MedOnScene    = "on_scene"
	MedDeparted   = "departed"  // terminal: transported N patients to Destination
	MedReleased   = "released"  // terminal: treated/refused, no transport
	MedCancelled  = "cancelled" // terminal
)

// Severity drives the bib-withholding policy only. Deliberately three
// values.
const (
	SeverityRoutine = "routine"
	SeveritySerious = "serious"
	SeveritySevere  = "severe"
)

// Medical destinations.
const (
	DestHospital = "hospital"
	DestStart    = "start"
	DestFinish   = "finish"
	DestRestStop = "rest_stop"
	DestOther    = "other"
)

// MedicalNotification is notification-and-logistics ONLY (governing fact 6).
// Fields appear in COURSE-plan script order; there are no clinical fields
// beyond ChiefComplaint and there must never be.
type MedicalNotification struct {
	ID       string  `json:"id"`
	NetID    string  `json:"netId"`
	Division *string `json:"division,omitempty"`

	ReportedByCheckInID string `json:"reportedByCheckInId,omitempty"`
	ReportedByCall      string `json:"reportedByCall"`

	// --- scripted fields, in order ---
	Bib                  string     `json:"bib,omitempty"` // label, never a key
	BibWithheld          bool       `json:"bibWithheld"`   // server-set per policy; Redacted() blanks Bib when true
	Sex                  string     `json:"sex"`           // "M" | "F" | "X" | "U" (unknown/not stated)
	Age                  string     `json:"age"`           // text: "34", "~40s", "unknown"
	Location             string     `json:"location"`      // exact location, on-air text
	MilesRemaining       *float64   `json:"milesRemaining,omitempty"`
	RouteID              string     `json:"routeId,omitempty"`
	LocationAnnotationID string     `json:"locationAnnotationId,omitempty"`
	Lat                  *float64   `json:"lat,omitempty"`
	Lon                  *float64   `json:"lon,omitempty"`
	ChiefComplaint       string     `json:"chiefComplaint"`
	ReadBackAt           *time.Time `json:"readBackAt,omitempty"`
	ReadBackBy           string     `json:"readBackBy,omitempty"`

	Severity string `json:"severity"` // routine|serious|severe
	Priority string `json:"priority"` // net tier vocabulary; default "priority"
	Status   string `json:"status"`

	// --- EMS logistics ladder ---
	EMSUnit         string     `json:"emsUnit,omitempty"` // "Medic 12", "SAG MED 1"
	ETAMinutes      *int       `json:"etaMinutes,omitempty"`
	ETAGivenAt      *time.Time `json:"etaGivenAt,omitempty"`
	ETADueAt        *time.Time `json:"etaDueAt,omitempty"`
	OnSceneAt       *time.Time `json:"onSceneAt,omitempty"`
	DepartedAt      *time.Time `json:"departedAt,omitempty"`
	OnSceneSeconds  *int       `json:"onSceneSeconds,omitempty"`  // DepartedAt-OnSceneAt, stored for the log
	Destination     string     `json:"destination,omitempty"`     // hospital|start|finish|rest_stop|other
	DestinationName string     `json:"destinationName,omitempty"` // "Marin General"
	PatientCount    int        `json:"patientCount"`              // "with N patients aboard"
	// Governing fact 7: recorded for transported riders only. Never in
	// Redacted(), never in timeline text, never on WS.
	PatientName string `json:"patientName,omitempty"`

	ReleasedAt   *time.Time `json:"releasedAt,omitempty"`
	CancelledAt  *time.Time `json:"cancelledAt,omitempty"`
	CancelReason string     `json:"cancelReason,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// Redacted returns the copy safe for WS broadcast, observer GETs and
// timeline details: PatientName always blanked, Bib blanked when
// BibWithheld.
func (n MedicalNotification) Redacted() MedicalNotification {
	out := n
	out.PatientName = ""
	if out.BibWithheld {
		out.Bib = ""
	}
	return out
}

// --- Ride reconciliation (WP5): SAG shift summaries, NCS shift-relief
// handoff, and the close-out paper trail. Rider accounting itself reuses
// internal/course's RiderException (see that type's own doc comment) rather
// than a second, differently-shaped table — course.Manager already
// implements governing fact 4 (supported/unsupported population
// accounting) exactly, so WP5 reads it instead of duplicating it.

// SAG shift summary statuses.
const (
	ShiftDraft = "draft"
	ShiftFiled = "filed"
)

// Handoff item kinds and statuses.
const (
	HandoffAwaitingReply = "awaiting_reply"
	HandoffPendingAction = "pending_action"
	HandoffFYI           = "fyi"

	HandoffOpen      = "open"
	HandoffResolved  = "resolved"
	HandoffCancelled = "cancelled"
)

// SAGShiftCounts mirrors the SLOBC "SAG Driver Report" form verbatim.
// Pointer ints are the "did the driver enter this" tri-state — nil falls
// back to a derived value, an explicit 0 is a real, driver-confirmed zero.
// Scalars, so the nil-slice hazard does not apply.
type SAGShiftCounts struct {
	Transports        *int `json:"transports,omitempty"`
	Assists           *int `json:"assists,omitempty"` // repairs, flats
	TubesProvided     *int `json:"tubesProvided,omitempty"`
	TiresProvided     *int `json:"tiresProvided,omitempty"`
	MinorFirstAid     *int `json:"minorFirstAid,omitempty"`
	IncidentsAttended *int `json:"incidentsAttended,omitempty"`
}

// SAGShiftSummary is the SLOBC "SAG Driver Report": one per driver per
// shift, a per-shift TALLY — not a per-incident form. Counts the driver did
// not type fall back to values derived from logged SAG requests (see
// reconcile.Effective). Division is "" for single-net v1 (fact 9).
type SAGShiftSummary struct {
	ID        string `json:"id"`
	NetID     string `json:"netId"`
	Division  string `json:"division,omitempty"`
	CheckInID string `json:"checkInId"` // the category "sag" NetCheckIn this shift belongs to

	Callsign     string `json:"callsign"`
	TacticalCall string `json:"tacticalCall"`
	DriverName   string `json:"driverName"`
	Vehicle      string `json:"vehicle"` // free text: "white Sprinter, 2 racks"

	ShiftStart *time.Time `json:"shiftStart,omitempty"`
	ShiftEnd   *time.Time `json:"shiftEnd,omitempty"`

	OdometerStart *float64 `json:"odometerStart,omitempty"`
	OdometerEnd   *float64 `json:"odometerEnd,omitempty"`

	Entered SAGShiftCounts `json:"entered"` // driver overrides; nil field = not entered
	Notes   string         `json:"notes"`

	Status  string     `json:"status"` // draft|filed
	FiledAt *time.Time `json:"filedAt,omitempty"`
	FiledBy string     `json:"filedBy,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// HandoffItem is one line of "pending activity" for the NCS shift-relief
// briefing: a message sent and the reply expected, and who gets that reply.
type HandoffItem struct {
	ID       string `json:"id"`
	NetID    string `json:"netId"`
	Division string `json:"division,omitempty"`

	Kind    string `json:"kind"`    // awaiting_reply|pending_action|fyi
	Summary string `json:"summary"` // "Asked RS3 for water status"
	SentTo  string `json:"sentTo"`  // who owes the reply (callsign/tactical)
	ReplyTo string `json:"replyTo"` // who gets the reply — "NCS" or a callsign/tactical

	RefType string     `json:"refType,omitempty"` // sag_request|supply_request|medical_notification|message|mission|""
	RefID   string     `json:"refId,omitempty"`
	DueAt   *time.Time `json:"dueAt,omitempty"`

	Status        string `json:"status"`        // open|resolved|cancelled
	HandoverCount int    `json:"handoverCount"` // how many NCS changes this item has survived

	CreatedBy  string     `json:"createdBy"` // NCS callsign
	CreatedAt  time.Time  `json:"createdAt"`
	ResolvedBy string     `json:"resolvedBy,omitempty"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	Resolution string     `json:"resolution,omitempty"`
}

// ShiftHandoff is the frozen briefing generated at an NCS transfer, so the
// paper trail shows exactly what the relieving operator inherited.
type ShiftHandoff struct {
	ID           string    `json:"id"`
	NetID        string    `json:"netId"`
	Division     string    `json:"division,omitempty"`
	FromCallsign string    `json:"fromCallsign"`
	ToCallsign   string    `json:"toCallsign"`
	At           time.Time `json:"at"`
	OpenItemIDs  []string  `json:"openItemIds"` // never nil
	Briefing     string    `json:"briefing"`    // reconcile.ShiftBriefing JSON, frozen

	AcknowledgedAt *time.Time `json:"acknowledgedAt,omitempty"`
	AcknowledgedBy string     `json:"acknowledgedBy,omitempty"`
}

// WxAlertRow is the persisted audit record of one NWS alert
// (internal/wxalert.MatchedAlert), keyed by its CAP id. It is a plain
// bookkeeping row, not a wire type — it never serializes directly to the
// frontend (Data does, as opaque JSON the caller already marshalled), so it
// carries no json tags. The scalar columns exist for indexed queries
// (state/net/updated_at); Data is the full record for exact restore/display.
type WxAlertRow struct {
	ID             string
	NetID          string
	Event          string
	Tier           string
	State          string // active|expired|cancelled|dropped|superseded
	Proximity      string // in|near|far
	NotifyClass    string
	Sent           time.Time
	Expires        time.Time
	EndsAt         time.Time
	ReplacedBy     string
	NetAckCallsign string
	NetAckAt       *time.Time
	FetchedAt      time.Time
	FirstSeenAt    time.Time
	UpdatedAt      time.Time
	Data           string // full MatchedAlert, JSON-encoded by the caller
}

// WxPointZone caches one lat/lon grid cell's zone resolution so repeated
// footprint rebuilds do not re-hit NWS /points for the same neighborhood.
// The cell key is two integer columns (floor(lat*100), floor(lon*100)) —
// never a formatted float string, which sorts and compares unambiguously.
type WxPointZone struct {
	CellLat   int
	CellLon   int
	UGC       []string // never nil
	ExpiresAt time.Time
}

// Store provides persistent storage for stations, messages, and configuration.
type Store interface {
	// Init initializes the store (creates tables, etc).
	Init() error

	// Close closes the store.
	Close() error

	// SaveStation persists a station record.
	SaveStation(s station.Station) error

	// LoadStations loads all stored stations.
	LoadStations() ([]station.Station, error)

	// SaveMessage persists a message.
	SaveMessage(m message.Message) error

	// LoadMessages loads all messages.
	LoadMessages() ([]message.Message, error)

	// SaveTrackPoint persists a single track point for a callsign.
	SaveTrackPoint(callsign string, tp station.TrackPoint) error

	// LoadTrackPoints loads the last N track points for a callsign.
	LoadTrackPoints(callsign string, limit int) ([]station.TrackPoint, error)

	// LogActivity records an activity log entry.
	LogActivity(entry ActivityLogEntry) error

	// QueryActivity returns matching activity log entries and the total count.
	QueryActivity(filter ActivityFilter) ([]ActivityLogEntry, int, error)

	// SaveAnnotation persists an annotation (insert or replace).
	SaveAnnotation(a Annotation) error

	// LoadAnnotations loads all annotations ordered by creation time.
	LoadAnnotations() ([]Annotation, error)

	// DeleteAnnotation removes an annotation by ID.
	DeleteAnnotation(id string) error

	// LoadAnnotationsFiltered loads annotations matching the given filter.
	LoadAnnotationsFiltered(filter AnnotationFilter) ([]Annotation, error)

	// UpdateAnnotationBatchLabel renames every annotation in a batch.
	// Returns the number of rows affected.
	UpdateAnnotationBatchLabel(batchID, label string, updatedAt time.Time) (int, error)

	// UpdateMessageClaim sets the claimed_by and claimed_at fields on a message.
	UpdateMessageClaim(messageID string, claimedBy string, claimedAt *time.Time) error

	// SaveConversationRead persists the read marker for a conversation.
	SaveConversationRead(callsign string, lastReadAt time.Time) error

	// LoadConversationReads returns all persisted conversation read markers
	// keyed by remote callsign. The map is never nil.
	LoadConversationReads() (map[string]time.Time, error)

	// Net Control
	SaveNet(n Net) error
	LoadNet(id string) (*Net, error)
	LoadNets() ([]Net, error)
	DeleteNet(id string) error

	SaveNetCheckIn(ci NetCheckIn) error
	LoadNetCheckIns(netID string) ([]NetCheckIn, error)
	DeleteNetCheckIn(id string) error

	SaveNetMission(m NetMission) error
	LoadNetMissions(netID string) ([]NetMission, error)

	SaveNetNote(n NetNote) error
	LoadNetNotes(netID string) ([]NetNote, error)
	UpdateNotePinned(noteID string, pinned bool) error

	SaveNetEvent(e NetEvent) error
	LoadNetEvents(netID string) ([]NetEvent, error)

	// Operations
	SaveOperation(op Operation) error
	LoadOperations() ([]Operation, error)
	LoadOperation(id string) (*Operation, error)
	DeleteOperation(id string) error

	// Tactical Aliases
	SaveTacticalAlias(a TacticalAlias) error
	LoadTacticalAliases() ([]TacticalAlias, error)
	DeleteTacticalAlias(callsign string) error

	// Weather
	SaveWeatherReading(r WeatherReading) error
	LoadWeatherReadings(filter WeatherFilter) ([]WeatherReading, error)
	LoadWeatherStations() ([]WeatherReading, error)
	PurgeWeatherReadings(olderThan time.Time) (int64, error)

	// Telemetry
	SaveTelemetryReading(r TelemetryReading) error
	LoadTelemetryReadings(filter TelemetryFilter) ([]TelemetryReading, error)
	LoadTelemetryStations() ([]TelemetryReading, error)
	PurgeTelemetryReadings(olderThan time.Time) (int64, error)

	// Checkpoint Progress
	SaveCheckpointMeta(m CheckpointMeta) error
	LoadCheckpointMeta(netID string) ([]CheckpointMeta, error)
	DeleteCheckpointMeta(annotationID string) error
	SaveCheckpointPassage(p CheckpointPassage) error
	LoadCheckpointPassages(netID string) ([]CheckpointPassage, error)
	LoadCheckpointPassagesForCheckpoint(checkpointID string) ([]CheckpointPassage, error)
	DeleteCheckpointPassages(netID string) error

	// Net profile / ride config
	SaveNetRideConfig(c NetRideConfig) error
	LoadNetRideConfig(netID string) (*NetRideConfig, bool, error)
	LoadNetRideConfigs() ([]NetRideConfig, error)
	DeleteNetRideConfig(netID string) error

	// Ride mode (internal/ride)
	SaveSAGRequest(r SAGRequest) error
	LoadSAGRequests(netID string) ([]SAGRequest, error) // ORDER BY sequence ASC; Slots/Legs/SlotIDs never nil
	SaveSAGVehicle(v SAGVehicle) error
	LoadSAGVehicles(netID string) ([]SAGVehicle, error)
	DeleteSAGVehicle(netID, checkInID string) error

	// Course closure (internal/course)
	SaveCourseConfig(c CourseConfig) error
	LoadCourseConfig(netID string) (*CourseConfig, error)   // nil,nil when absent
	SaveShutoffPoint(sp ShutoffPoint) error                 // INSERT OR REPLACE
	LoadShutoffPoints(netID string) ([]ShutoffPoint, error) // ORDER BY scheduled_at ASC
	DeleteShutoffPoint(id string) error
	SaveRiderException(r RiderException) error
	LoadRiderExceptions(netID string) ([]RiderException, error) // ORDER BY recorded_at ASC
	SaveSweepReport(r SweepReport) error
	LoadSweepReports(netID string, limit int) ([]SweepReport, error) // ORDER BY reported_at DESC LIMIT ?; limit<=0 -> all
	SaveStationClosure(c StationClosure) error
	LoadStationClosures(netID string) ([]StationClosure, error)
	DeleteCourseDataForNet(netID string) error // all five tables

	// Ride phase (internal/ride/phase, WP5b)
	SaveRidePhase(p RidePhaseState) error                // INSERT OR REPLACE, one row per net
	LoadRidePhase(netID string) (*RidePhaseState, error) // nil,nil when absent

	// Ride traffic (WP4): supply requests and medical notifications
	SaveSupplyRequest(r SupplyRequest) error
	LoadSupplyRequests(netID string) ([]SupplyRequest, error) // ORDER BY created_at ASC; Items/ETAs never nil
	SaveMedicalNotification(n MedicalNotification) error
	LoadMedicalNotifications(netID string) ([]MedicalNotification, error) // ORDER BY created_at ASC
	DeleteRideTraffic(netID string) error                                 // both tables, for this net only

	// Ride reconciliation (WP5): SAG shift summaries, NCS shift-relief
	// handoff. Rider exceptions reuse SaveRiderException/LoadRiderExceptions
	// above (internal/course). No delete methods: these are audit records —
	// a shift summary is voided by superseding it with a new draft, not
	// removed.
	SaveSAGShiftSummary(s SAGShiftSummary) error
	LoadSAGShiftSummaries(netID string) ([]SAGShiftSummary, error) // ORDER BY created_at ASC
	SaveHandoffItem(h HandoffItem) error
	LoadHandoffItems(netID string) ([]HandoffItem, error) // ORDER BY created_at ASC
	SaveShiftHandoff(h ShiftHandoff) error
	LoadShiftHandoffs(netID string) ([]ShiftHandoff, error) // ORDER BY at ASC; OpenItemIDs never nil

	// NWS Weather Alerts (internal/wxalert)
	SaveWxAlert(a WxAlertRow) error
	LoadWxAlerts(includeInactive bool) ([]WxAlertRow, error)
	UpdateWxAlertNetAck(id, callsign string, at time.Time, data string) error
	PurgeWxAlerts(olderThan time.Time) (int64, error)

	SaveWxPointZone(z WxPointZone) error
	LoadWxPointZones() ([]WxPointZone, error)

	GetWxMeta(key string) (string, bool, error)
	SetWxMeta(key, value string) error
}
