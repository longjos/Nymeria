package activity

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

// Action represents a type of logged activity.
type Action = string

const (
	ActionMessageSent         Action = "message_sent"
	ActionMessageClaimed      Action = "message_claimed"
	ActionObjectCreated       Action = "object_created"
	ActionObjectKilled        Action = "object_killed"
	ActionAnnotationCreated   Action = "annotation_created"
	ActionAnnotationDeleted   Action = "annotation_deleted"
	ActionConfigChanged       Action = "config_changed"
	ActionSessionStarted      Action = "session_started"
	ActionSessionEnded        Action = "session_ended"
	ActionBeaconSent          Action = "beacon_sent"
	ActionTransportConnect    Action = "transport_connect"
	ActionTransportDisconnect      Action = "transport_disconnect"
	ActionAnnotationStatusChanged  Action = "annotation_status_changed"
)

// Entry represents a single activity log entry.
type Entry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"userId,omitempty"`
	UserName  string    `json:"userName,omitempty"`
	Action    Action    `json:"action"`
	Target    string    `json:"target,omitempty"`
	Details   string    `json:"details,omitempty"`
}

// Filter controls activity log queries.
type Filter struct {
	Since  *time.Time
	Until  *time.Time
	UserID string
	Action string
	Offset int
	Limit  int
}

// Logger provides activity logging and querying.
type Logger interface {
	Log(entry Entry) error
	Query(filter Filter) ([]Entry, int, error)
	Events() <-chan Entry
}

// StoreLogger implements Logger using a store.Store backend.
type StoreLogger struct {
	store  store.Store
	events chan Entry
}

// NewStoreLogger creates a StoreLogger backed by the given store.
func NewStoreLogger(s store.Store) *StoreLogger {
	return &StoreLogger{
		store:  s,
		events: make(chan Entry, 64),
	}
}

func (l *StoreLogger) Log(entry Entry) error {
	se := store.ActivityLogEntry{
		Timestamp: entry.Timestamp,
		UserID:    entry.UserID,
		UserName:  entry.UserName,
		Action:    entry.Action,
		Target:    entry.Target,
		Details:   entry.Details,
	}
	if err := l.store.LogActivity(se); err != nil {
		return err
	}

	// Non-blocking send to events channel.
	select {
	case l.events <- entry:
	default:
	}

	return nil
}

func (l *StoreLogger) Query(filter Filter) ([]Entry, int, error) {
	sf := store.ActivityFilter{
		Since:  filter.Since,
		Until:  filter.Until,
		UserID: filter.UserID,
		Action: filter.Action,
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}

	results, total, err := l.store.QueryActivity(sf)
	if err != nil {
		return nil, 0, err
	}

	entries := make([]Entry, len(results))
	for i, r := range results {
		entries[i] = Entry{
			ID:        r.ID,
			Timestamp: r.Timestamp,
			UserID:    r.UserID,
			UserName:  r.UserName,
			Action:    r.Action,
			Target:    r.Target,
			Details:   r.Details,
		}
	}

	return entries, total, nil
}

// Events returns a channel that receives entries as they are logged.
func (l *StoreLogger) Events() <-chan Entry {
	return l.events
}

// ExportCSV writes activity entries as CSV to w.
func ExportCSV(w io.Writer, entries []Entry) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{"timestamp", "userId", "userName", "action", "target", "details"}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, e := range entries {
		row := []string{
			e.Timestamp.UTC().Format(time.RFC3339),
			e.UserID,
			e.UserName,
			e.Action,
			e.Target,
			e.Details,
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}

	return nil
}
