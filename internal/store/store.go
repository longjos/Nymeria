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

// Annotation represents a local map annotation.
type Annotation struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Label         string    `json:"label"`
	Description   string    `json:"description,omitempty"`
	Geometry      string    `json:"geometry"`
	Style         string    `json:"style,omitempty"`
	CreatedBy     string    `json:"createdBy,omitempty"`
	CreatedByName string    `json:"createdByName,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
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

	// UpdateMessageClaim sets the claimed_by and claimed_at fields on a message.
	UpdateMessageClaim(messageID string, claimedBy string, claimedAt *time.Time) error
}
