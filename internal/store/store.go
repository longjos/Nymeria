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
