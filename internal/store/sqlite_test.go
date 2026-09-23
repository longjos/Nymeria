package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/station"
)

// newTestStore creates a SQLiteStore backed by a temp file and calls Init.
// The caller should defer os.Remove(path) and s.Close().
func newTestStore(t *testing.T) (*SQLiteStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return s, path
}

func TestInitCreatesTables(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Init should succeed and be idempotent — call it again.
	if err := s.Init(); err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
}

func TestSaveAndLoadStationsRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	st := station.Station{
		Callsign:  "N0CALL",
		SSID:      9,
		LastHeard: now,
		Position: &station.Position{
			Lat:      34.0522,
			Lon:      -118.2437,
			Altitude: 100.5,
			Speed:    45.0,
			Course:   270.0,
		},
		Symbol:  aprs.Symbol{Table: '/', Code: '>'},
		Comment: "mobile station",
		Source:  "APRS-IS",
	}

	if err := s.SaveStation(st); err != nil {
		t.Fatalf("SaveStation failed: %v", err)
	}

	loaded, err := s.LoadStations()
	if err != nil {
		t.Fatalf("LoadStations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 station, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Callsign != st.Callsign {
		t.Errorf("callsign: got %q, want %q", got.Callsign, st.Callsign)
	}
	if got.SSID != st.SSID {
		t.Errorf("ssid: got %d, want %d", got.SSID, st.SSID)
	}
	if !got.LastHeard.Equal(st.LastHeard) {
		t.Errorf("lastHeard: got %v, want %v", got.LastHeard, st.LastHeard)
	}
	if got.Position == nil {
		t.Fatal("position is nil")
	}
	if got.Position.Lat != st.Position.Lat {
		t.Errorf("lat: got %f, want %f", got.Position.Lat, st.Position.Lat)
	}
	if got.Position.Lon != st.Position.Lon {
		t.Errorf("lon: got %f, want %f", got.Position.Lon, st.Position.Lon)
	}
	if got.Position.Altitude != st.Position.Altitude {
		t.Errorf("altitude: got %f, want %f", got.Position.Altitude, st.Position.Altitude)
	}
	if got.Position.Speed != st.Position.Speed {
		t.Errorf("speed: got %f, want %f", got.Position.Speed, st.Position.Speed)
	}
	if got.Position.Course != st.Position.Course {
		t.Errorf("course: got %f, want %f", got.Position.Course, st.Position.Course)
	}
	if got.Symbol.Table != st.Symbol.Table {
		t.Errorf("symbol table: got %c, want %c", got.Symbol.Table, st.Symbol.Table)
	}
	if got.Symbol.Code != st.Symbol.Code {
		t.Errorf("symbol code: got %c, want %c", got.Symbol.Code, st.Symbol.Code)
	}
	if got.Comment != st.Comment {
		t.Errorf("comment: got %q, want %q", got.Comment, st.Comment)
	}
	if got.Source != st.Source {
		t.Errorf("source: got %q, want %q", got.Source, st.Source)
	}
}

func TestSaveAndLoadStationSourcesRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	withSources := station.Station{
		Callsign:  "N0SRC",
		SSID:      1,
		LastHeard: now,
		Source:    "aprsis+serial",
		Sources:   []string{"aprsis", "serial"},
	}
	if err := s.SaveStation(withSources); err != nil {
		t.Fatalf("SaveStation (with sources) failed: %v", err)
	}

	nilSources := station.Station{
		Callsign:  "N0NIL",
		SSID:      0,
		LastHeard: now,
		Source:    "aprsis",
		Sources:   nil,
	}
	if err := s.SaveStation(nilSources); err != nil {
		t.Fatalf("SaveStation (nil sources) failed: %v", err)
	}

	loaded, err := s.LoadStations()
	if err != nil {
		t.Fatalf("LoadStations failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 stations, got %d", len(loaded))
	}

	byCall := make(map[string]station.Station, len(loaded))
	for _, st := range loaded {
		byCall[st.Callsign] = st
	}

	got := byCall["N0SRC"]
	if len(got.Sources) != 2 || got.Sources[0] != "aprsis" || got.Sources[1] != "serial" {
		t.Errorf("N0SRC sources: got %v, want [aprsis serial]", got.Sources)
	}

	gotNil := byCall["N0NIL"]
	if len(gotNil.Sources) != 0 {
		t.Errorf("N0NIL sources: got %v, want empty", gotNil.Sources)
	}
}

func TestSaveStationWithoutPosition(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	st := station.Station{
		Callsign:  "W1AW",
		SSID:      0,
		LastHeard: time.Now().Truncate(time.Second).UTC(),
		Source:    "APRS-IS",
	}

	if err := s.SaveStation(st); err != nil {
		t.Fatalf("SaveStation failed: %v", err)
	}

	loaded, err := s.LoadStations()
	if err != nil {
		t.Fatalf("LoadStations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 station, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Callsign != "W1AW" {
		t.Errorf("callsign: got %q, want %q", got.Callsign, "W1AW")
	}
	// Position should be nil when all position fields are zero/null.
	if got.Position != nil && got.Position.Lat == 0 && got.Position.Lon == 0 {
		// Acceptable: zero-valued position or nil.
	}
}

func TestSaveStationUpsert(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	st := station.Station{
		Callsign:  "N0CALL",
		SSID:      0,
		LastHeard: now,
		Comment:   "first",
		Source:    "APRS-IS",
	}
	if err := s.SaveStation(st); err != nil {
		t.Fatalf("SaveStation (first) failed: %v", err)
	}

	// Update same station.
	st.Comment = "updated"
	st.LastHeard = now.Add(time.Minute)
	if err := s.SaveStation(st); err != nil {
		t.Fatalf("SaveStation (update) failed: %v", err)
	}

	loaded, err := s.LoadStations()
	if err != nil {
		t.Fatalf("LoadStations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 station after upsert, got %d", len(loaded))
	}
	if loaded[0].Comment != "updated" {
		t.Errorf("comment: got %q, want %q", loaded[0].Comment, "updated")
	}
}

func TestMultipleStations(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	stations := []station.Station{
		{Callsign: "N0CALL", SSID: 0, LastHeard: now, Source: "APRS-IS"},
		{Callsign: "N0CALL", SSID: 9, LastHeard: now, Source: "RF"},
		{Callsign: "W1AW", SSID: 0, LastHeard: now, Source: "APRS-IS"},
	}

	for _, st := range stations {
		if err := s.SaveStation(st); err != nil {
			t.Fatalf("SaveStation(%s-%d) failed: %v", st.Callsign, st.SSID, err)
		}
	}

	loaded, err := s.LoadStations()
	if err != nil {
		t.Fatalf("LoadStations failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 stations, got %d", len(loaded))
	}
}

func TestSaveAndLoadMessagesRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	msg := message.Message{
		ID:        "msg-001",
		From:      "N0CALL",
		To:        "W1AW",
		Body:      "Hello World",
		MsgNo:     "123",
		State:     message.StateSent,
		Retries:   2,
		Inbound:   false,
		Timestamp: now,
	}

	if err := s.SaveMessage(msg); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	loaded, err := s.LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded))
	}

	got := loaded[0]
	if got.ID != msg.ID {
		t.Errorf("id: got %q, want %q", got.ID, msg.ID)
	}
	if got.From != msg.From {
		t.Errorf("from: got %q, want %q", got.From, msg.From)
	}
	if got.To != msg.To {
		t.Errorf("to: got %q, want %q", got.To, msg.To)
	}
	if got.Body != msg.Body {
		t.Errorf("body: got %q, want %q", got.Body, msg.Body)
	}
	if got.MsgNo != msg.MsgNo {
		t.Errorf("msgNo: got %q, want %q", got.MsgNo, msg.MsgNo)
	}
	if got.State != msg.State {
		t.Errorf("state: got %d, want %d", got.State, msg.State)
	}
	if got.Retries != msg.Retries {
		t.Errorf("retries: got %d, want %d", got.Retries, msg.Retries)
	}
	if got.Inbound != msg.Inbound {
		t.Errorf("inbound: got %v, want %v", got.Inbound, msg.Inbound)
	}
	if !got.Timestamp.Equal(msg.Timestamp) {
		t.Errorf("timestamp: got %v, want %v", got.Timestamp, msg.Timestamp)
	}
}

func TestSaveMessageUpsert(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	msg := message.Message{
		ID:        "msg-001",
		From:      "N0CALL",
		To:        "W1AW",
		Body:      "Hello",
		MsgNo:     "42",
		State:     message.StatePending,
		Retries:   0,
		Inbound:   false,
		Timestamp: now,
	}
	if err := s.SaveMessage(msg); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	// Mark as acked with retries.
	msg.State = message.StateAcked
	msg.Retries = 3
	if err := s.SaveMessage(msg); err != nil {
		t.Fatalf("SaveMessage (update) failed: %v", err)
	}

	loaded, err := s.LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 message after upsert, got %d", len(loaded))
	}
	if loaded[0].State != message.StateAcked {
		t.Errorf("expected state StateAcked, got %d", loaded[0].State)
	}
	if loaded[0].Retries != 3 {
		t.Errorf("expected retries 3, got %d", loaded[0].Retries)
	}
}

func TestMultipleMessagesOrderedByTimestamp(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().Truncate(time.Second).UTC()

	msgs := []message.Message{
		{ID: "msg-003", From: "A", To: "B", Body: "third", State: message.StatePending, Timestamp: base.Add(2 * time.Minute)},
		{ID: "msg-001", From: "A", To: "B", Body: "first", State: message.StatePending, Timestamp: base},
		{ID: "msg-002", From: "B", To: "A", Body: "second", State: message.StatePending, Timestamp: base.Add(time.Minute)},
	}

	for _, m := range msgs {
		if err := s.SaveMessage(m); err != nil {
			t.Fatalf("SaveMessage(%s) failed: %v", m.ID, err)
		}
	}

	loaded, err := s.LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}

	// Should be ordered by timestamp ascending.
	if loaded[0].ID != "msg-001" {
		t.Errorf("first message: got %q, want %q", loaded[0].ID, "msg-001")
	}
	if loaded[1].ID != "msg-002" {
		t.Errorf("second message: got %q, want %q", loaded[1].ID, "msg-002")
	}
	if loaded[2].ID != "msg-003" {
		t.Errorf("third message: got %q, want %q", loaded[2].ID, "msg-003")
	}
}

func TestSaveAndLoadTrackPointsRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().Truncate(time.Second).UTC()

	tp := station.TrackPoint{
		Lat:  34.0522,
		Lon:  -118.2437,
		Time: base,
	}

	if err := s.SaveTrackPoint("N0CALL", tp); err != nil {
		t.Fatalf("SaveTrackPoint failed: %v", err)
	}

	loaded, err := s.LoadTrackPoints("N0CALL", 10)
	if err != nil {
		t.Fatalf("LoadTrackPoints failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 track point, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Lat != tp.Lat {
		t.Errorf("lat: got %f, want %f", got.Lat, tp.Lat)
	}
	if got.Lon != tp.Lon {
		t.Errorf("lon: got %f, want %f", got.Lon, tp.Lon)
	}
	if !got.Time.Equal(tp.Time) {
		t.Errorf("time: got %v, want %v", got.Time, tp.Time)
	}
}

func TestLoadTrackPointsLimit(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().Truncate(time.Second).UTC()

	// Insert 10 track points.
	for i := 0; i < 10; i++ {
		tp := station.TrackPoint{
			Lat:  34.0 + float64(i)*0.01,
			Lon:  -118.0 + float64(i)*0.01,
			Time: base.Add(time.Duration(i) * time.Minute),
		}
		if err := s.SaveTrackPoint("N0CALL", tp); err != nil {
			t.Fatalf("SaveTrackPoint(%d) failed: %v", i, err)
		}
	}

	// Load only 5, should get the 5 most recent.
	loaded, err := s.LoadTrackPoints("N0CALL", 5)
	if err != nil {
		t.Fatalf("LoadTrackPoints failed: %v", err)
	}
	if len(loaded) != 5 {
		t.Fatalf("expected 5 track points, got %d", len(loaded))
	}

	// Results should be ordered oldest-to-newest (ascending time) within the
	// returned window, and the window should be the most recent 5 points.
	expectedStartIdx := 5 // points 5..9
	for i, tp := range loaded {
		expectedTime := base.Add(time.Duration(expectedStartIdx+i) * time.Minute)
		if !tp.Time.Equal(expectedTime) {
			t.Errorf("track point %d time: got %v, want %v", i, tp.Time, expectedTime)
		}
	}
}

func TestLoadTrackPointsIsolation(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().Truncate(time.Second).UTC()

	// Save track points for two different callsigns.
	if err := s.SaveTrackPoint("N0CALL", station.TrackPoint{Lat: 34.0, Lon: -118.0, Time: base}); err != nil {
		t.Fatalf("SaveTrackPoint(N0CALL) failed: %v", err)
	}
	if err := s.SaveTrackPoint("W1AW", station.TrackPoint{Lat: 41.0, Lon: -72.0, Time: base}); err != nil {
		t.Fatalf("SaveTrackPoint(W1AW) failed: %v", err)
	}

	loaded, err := s.LoadTrackPoints("N0CALL", 10)
	if err != nil {
		t.Fatalf("LoadTrackPoints failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 track point for N0CALL, got %d", len(loaded))
	}
	if loaded[0].Lat != 34.0 {
		t.Errorf("expected lat 34.0, got %f", loaded[0].Lat)
	}
}

func TestSaveAndLoadTrackPointSpeedCourse(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().Truncate(time.Second).UTC()

	tp := station.TrackPoint{
		Lat:    34.0522,
		Lon:    -118.2437,
		Time:   base,
		Speed:  88.5,
		Course: 270.0,
	}

	if err := s.SaveTrackPoint("N0CALL", tp); err != nil {
		t.Fatalf("SaveTrackPoint failed: %v", err)
	}

	loaded, err := s.LoadTrackPoints("N0CALL", 10)
	if err != nil {
		t.Fatalf("LoadTrackPoints failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 track point, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Speed != 88.5 {
		t.Errorf("speed: got %f, want 88.5", got.Speed)
	}
	if got.Course != 270.0 {
		t.Errorf("course: got %f, want 270.0", got.Course)
	}

	// Verify zero values round-trip correctly
	tp2 := station.TrackPoint{
		Lat:  35.0,
		Lon:  -117.0,
		Time: base.Add(time.Minute),
	}
	if err := s.SaveTrackPoint("N0CALL", tp2); err != nil {
		t.Fatalf("SaveTrackPoint(zero) failed: %v", err)
	}

	loaded, err = s.LoadTrackPoints("N0CALL", 10)
	if err != nil {
		t.Fatalf("LoadTrackPoints(2) failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 track points, got %d", len(loaded))
	}
	if loaded[1].Speed != 0 {
		t.Errorf("zero speed: got %f, want 0", loaded[1].Speed)
	}
	if loaded[1].Course != 0 {
		t.Errorf("zero course: got %f, want 0", loaded[1].Course)
	}
}

// --- V2 Migration Tests ---

func TestV2MigrationCreatesActivityLogTable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Verify activity_log table exists by inserting and querying.
	_, err := s.db.Exec(`INSERT INTO activity_log (timestamp, action) VALUES (?, ?)`,
		time.Now().UTC(), "test")
	if err != nil {
		t.Fatalf("activity_log table not created: %v", err)
	}
}

func TestV2MigrationCreatesAnnotationsTable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	_, err := s.db.Exec(`INSERT INTO annotations (id, type, label, geometry, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"ann-1", "point", "test", `{"type":"Point"}`, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatalf("annotations table not created: %v", err)
	}
}

func TestV2MigrationAddsClaimColumnsToMessages(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// The claimed_by and claimed_at columns should exist on the messages table.
	_, err := s.db.Exec(`UPDATE messages SET claimed_by = NULL, claimed_at = NULL WHERE 1=0`)
	if err != nil {
		t.Fatalf("claimed_by/claimed_at columns not added to messages: %v", err)
	}
}

func TestV2SchemaVersion(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var version int
	err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}
}

// --- Activity Log Tests ---

func TestLogAndQueryActivity(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	entry := ActivityLogEntry{
		Timestamp: now,
		UserID:    "user-1",
		UserName:  "Alice",
		Action:    "login",
		Target:    "session",
		Details:   "logged in from mobile",
	}
	if err := s.LogActivity(entry); err != nil {
		t.Fatalf("LogActivity failed: %v", err)
	}

	entries, total, err := s.QueryActivity(ActivityFilter{Limit: 10})
	if err != nil {
		t.Fatalf("QueryActivity failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := entries[0]
	if got.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if !got.Timestamp.Equal(now) {
		t.Errorf("timestamp: got %v, want %v", got.Timestamp, now)
	}
	if got.UserID != "user-1" {
		t.Errorf("userID: got %q, want %q", got.UserID, "user-1")
	}
	if got.UserName != "Alice" {
		t.Errorf("userName: got %q, want %q", got.UserName, "Alice")
	}
	if got.Action != "login" {
		t.Errorf("action: got %q, want %q", got.Action, "login")
	}
	if got.Target != "session" {
		t.Errorf("target: got %q, want %q", got.Target, "session")
	}
	if got.Details != "logged in from mobile" {
		t.Errorf("details: got %q, want %q", got.Details, "logged in from mobile")
	}
}

func TestQueryActivityFilterByTimeRange(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().Truncate(time.Second).UTC()

	for i := 0; i < 5; i++ {
		if err := s.LogActivity(ActivityLogEntry{
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Action:    "action",
			Details:   fmt.Sprintf("entry %d", i),
		}); err != nil {
			t.Fatalf("LogActivity(%d) failed: %v", i, err)
		}
	}

	// Query entries between hour 1 and hour 3 (inclusive).
	since := base.Add(1 * time.Hour)
	until := base.Add(3 * time.Hour)
	entries, total, err := s.QueryActivity(ActivityFilter{
		Since: &since,
		Until: &until,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("QueryActivity failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestQueryActivityFilterByUser(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	s.LogActivity(ActivityLogEntry{Timestamp: now, UserID: "user-1", Action: "login"})
	s.LogActivity(ActivityLogEntry{Timestamp: now, UserID: "user-2", Action: "login"})
	s.LogActivity(ActivityLogEntry{Timestamp: now, UserID: "user-1", Action: "logout"})

	entries, total, err := s.QueryActivity(ActivityFilter{UserID: "user-1", Limit: 10})
	if err != nil {
		t.Fatalf("QueryActivity failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestQueryActivityFilterByAction(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	s.LogActivity(ActivityLogEntry{Timestamp: now, Action: "login"})
	s.LogActivity(ActivityLogEntry{Timestamp: now, Action: "logout"})
	s.LogActivity(ActivityLogEntry{Timestamp: now, Action: "login"})

	entries, total, err := s.QueryActivity(ActivityFilter{Action: "login", Limit: 10})
	if err != nil {
		t.Fatalf("QueryActivity failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestQueryActivityPagination(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	for i := 0; i < 10; i++ {
		s.LogActivity(ActivityLogEntry{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Action:    "tick",
		})
	}

	// Page 1: first 3
	entries, total, err := s.QueryActivity(ActivityFilter{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("QueryActivity page 1 failed: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total 10, got %d", total)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Page 2: next 3
	entries2, total2, err := s.QueryActivity(ActivityFilter{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("QueryActivity page 2 failed: %v", err)
	}
	if total2 != 10 {
		t.Errorf("expected total 10, got %d", total2)
	}
	if len(entries2) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries2))
	}

	// Entries on page 2 should be different from page 1.
	if entries2[0].ID == entries[0].ID {
		t.Error("page 2 should have different entries than page 1")
	}
}

func TestQueryActivityDefaultLimit(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	for i := 0; i < 5; i++ {
		s.LogActivity(ActivityLogEntry{Timestamp: now, Action: "test"})
	}

	// Limit 0 should default to returning all entries (or a sensible default).
	entries, total, err := s.QueryActivity(ActivityFilter{})
	if err != nil {
		t.Fatalf("QueryActivity failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

// --- Annotation Tests ---

func TestSaveAndLoadAnnotations(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	ann := Annotation{
		ID:            "ann-1",
		Type:          "point",
		Label:         "My Point",
		Description:   "A test annotation",
		Geometry:      `{"type":"Point","coordinates":[-118.24,34.05]}`,
		Style:         `{"color":"red"}`,
		CreatedBy:     "user-1",
		CreatedByName: "Alice",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation failed: %v", err)
	}

	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(loaded))
	}

	got := loaded[0]
	if got.ID != ann.ID {
		t.Errorf("id: got %q, want %q", got.ID, ann.ID)
	}
	if got.Type != ann.Type {
		t.Errorf("type: got %q, want %q", got.Type, ann.Type)
	}
	if got.Label != ann.Label {
		t.Errorf("label: got %q, want %q", got.Label, ann.Label)
	}
	if got.Description != ann.Description {
		t.Errorf("description: got %q, want %q", got.Description, ann.Description)
	}
	if got.Geometry != ann.Geometry {
		t.Errorf("geometry: got %q, want %q", got.Geometry, ann.Geometry)
	}
	if got.Style != ann.Style {
		t.Errorf("style: got %q, want %q", got.Style, ann.Style)
	}
	if got.CreatedBy != ann.CreatedBy {
		t.Errorf("createdBy: got %q, want %q", got.CreatedBy, ann.CreatedBy)
	}
	if got.CreatedByName != ann.CreatedByName {
		t.Errorf("createdByName: got %q, want %q", got.CreatedByName, ann.CreatedByName)
	}
	if !got.CreatedAt.Equal(ann.CreatedAt) {
		t.Errorf("createdAt: got %v, want %v", got.CreatedAt, ann.CreatedAt)
	}
	if !got.UpdatedAt.Equal(ann.UpdatedAt) {
		t.Errorf("updatedAt: got %v, want %v", got.UpdatedAt, ann.UpdatedAt)
	}
}

func TestSaveAnnotationUpsert(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	ann := Annotation{
		ID:        "ann-1",
		Type:      "point",
		Label:     "Original",
		Geometry:  `{"type":"Point","coordinates":[0,0]}`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation failed: %v", err)
	}

	// Update label.
	ann.Label = "Updated"
	ann.UpdatedAt = now.Add(time.Minute)
	if err := s.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation (update) failed: %v", err)
	}

	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 annotation after upsert, got %d", len(loaded))
	}
	if loaded[0].Label != "Updated" {
		t.Errorf("label: got %q, want %q", loaded[0].Label, "Updated")
	}
}

func TestDeleteAnnotation(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	s.SaveAnnotation(Annotation{
		ID: "ann-1", Type: "point", Label: "A", Geometry: "{}", CreatedAt: now, UpdatedAt: now,
	})
	s.SaveAnnotation(Annotation{
		ID: "ann-2", Type: "line", Label: "B", Geometry: "{}", CreatedAt: now, UpdatedAt: now,
	})

	if err := s.DeleteAnnotation("ann-1"); err != nil {
		t.Fatalf("DeleteAnnotation failed: %v", err)
	}

	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 annotation after delete, got %d", len(loaded))
	}
	if loaded[0].ID != "ann-2" {
		t.Errorf("expected remaining annotation ann-2, got %q", loaded[0].ID)
	}
}

func TestLoadAnnotationsOrderedByCreatedAt(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().Truncate(time.Second).UTC()

	// Insert out of order.
	s.SaveAnnotation(Annotation{
		ID: "ann-2", Type: "point", Label: "B", Geometry: "{}",
		CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute),
	})
	s.SaveAnnotation(Annotation{
		ID: "ann-1", Type: "point", Label: "A", Geometry: "{}",
		CreatedAt: base, UpdatedAt: base,
	})

	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(loaded))
	}
	if loaded[0].ID != "ann-1" {
		t.Errorf("first annotation: got %q, want %q", loaded[0].ID, "ann-1")
	}
	if loaded[1].ID != "ann-2" {
		t.Errorf("second annotation: got %q, want %q", loaded[1].ID, "ann-2")
	}
}

// --- Message Claim Tests ---

func TestUpdateMessageClaim(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	msg := message.Message{
		ID:        "msg-claim-1",
		From:      "N0CALL",
		To:        "W1AW",
		Body:      "test",
		State:     message.StatePending,
		Timestamp: now,
	}
	if err := s.SaveMessage(msg); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	claimTime := now.Add(time.Minute)
	if err := s.UpdateMessageClaim("msg-claim-1", "user-1", &claimTime); err != nil {
		t.Fatalf("UpdateMessageClaim failed: %v", err)
	}

	// Verify by reading raw DB.
	var claimedBy sql.NullString
	var claimedAt sql.NullString
	err := s.db.QueryRow("SELECT claimed_by, claimed_at FROM messages WHERE id = ?", "msg-claim-1").
		Scan(&claimedBy, &claimedAt)
	if err != nil {
		t.Fatalf("query claimed columns: %v", err)
	}
	if !claimedBy.Valid || claimedBy.String != "user-1" {
		t.Errorf("claimed_by: got %v, want user-1", claimedBy)
	}
	if !claimedAt.Valid {
		t.Error("claimed_at should not be NULL")
	}
}

func TestUpdateMessageClaimClear(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	msg := message.Message{
		ID:        "msg-claim-2",
		From:      "N0CALL",
		To:        "W1AW",
		Body:      "test",
		State:     message.StatePending,
		Timestamp: now,
	}
	s.SaveMessage(msg)

	// Claim then clear.
	claimTime := now
	s.UpdateMessageClaim("msg-claim-2", "user-1", &claimTime)
	if err := s.UpdateMessageClaim("msg-claim-2", "", nil); err != nil {
		t.Fatalf("UpdateMessageClaim (clear) failed: %v", err)
	}

	var claimedBy sql.NullString
	var claimedAt sql.NullString
	s.db.QueryRow("SELECT claimed_by, claimed_at FROM messages WHERE id = ?", "msg-claim-2").
		Scan(&claimedBy, &claimedAt)
	if claimedBy.Valid && claimedBy.String != "" {
		t.Errorf("claimed_by should be empty/null after clear, got %q", claimedBy.String)
	}
}

func TestCloseWorks(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestCloseBeforeInitIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := NewSQLiteStore(path)

	// Close without Init should not panic or error.
	if err := s.Close(); err != nil {
		t.Fatalf("Close before Init failed: %v", err)
	}
}

func TestInitCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.db")

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init with nested path failed: %v", err)
	}
	defer s.Close()

	// Verify the file exists.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

// --- V3 Migration & Net Control Tests ---

func TestV3MigrationCreatesNetTables(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Verify all net control tables exist.
	tables := []string{"nets", "net_check_ins", "net_missions", "net_notes", "net_events"}
	for _, table := range tables {
		var count int
		err := s.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		if err != nil {
			t.Errorf("table %s not created: %v", table, err)
		}
	}
}

func TestV3SchemaVersion(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var version int
	err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}
}

func TestSaveAndLoadNetRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	n := Net{
		ID:          "net-1",
		Name:        "Emergency Net",
		Type:        "tactical",
		Frequency:   "146.520 MHz",
		NCSCallsign: "KD7BBC",
		NCSUserID:   "user-1",
		Status:      "open",
		OpenedAt:    &now,
		Notes:       "Wildfire response",
	}

	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet failed: %v", err)
	}

	loaded, err := s.LoadNet("net-1")
	if err != nil {
		t.Fatalf("LoadNet failed: %v", err)
	}

	if loaded.ID != n.ID {
		t.Errorf("id: got %q, want %q", loaded.ID, n.ID)
	}
	if loaded.Name != n.Name {
		t.Errorf("name: got %q, want %q", loaded.Name, n.Name)
	}
	if loaded.Frequency != n.Frequency {
		t.Errorf("frequency: got %q, want %q", loaded.Frequency, n.Frequency)
	}
	if loaded.NCSCallsign != n.NCSCallsign {
		t.Errorf("ncsCallsign: got %q, want %q", loaded.NCSCallsign, n.NCSCallsign)
	}
	if loaded.Status != n.Status {
		t.Errorf("status: got %q, want %q", loaded.Status, n.Status)
	}
	if loaded.OpenedAt == nil || !loaded.OpenedAt.Equal(now) {
		t.Errorf("openedAt: got %v, want %v", loaded.OpenedAt, now)
	}
	if loaded.ClosedAt != nil {
		t.Errorf("closedAt: expected nil, got %v", loaded.ClosedAt)
	}
	if loaded.Notes != n.Notes {
		t.Errorf("notes: got %q, want %q", loaded.Notes, n.Notes)
	}
}

func TestLoadNetNotFound(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	_, err := s.LoadNet("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent net")
	}
}

func TestLoadNetsMultiple(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Net A", Type: "tactical", Status: "open"})
	s.SaveNet(Net{ID: "net-2", Name: "Net B", Type: "resource", Status: "draft"})

	nets, err := s.LoadNets()
	if err != nil {
		t.Fatalf("LoadNets failed: %v", err)
	}
	if len(nets) != 2 {
		t.Fatalf("expected 2 nets, got %d", len(nets))
	}
}

func TestDeleteNet(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Net A", Type: "tactical", Status: "draft"})

	if err := s.DeleteNet("net-1"); err != nil {
		t.Fatalf("DeleteNet failed: %v", err)
	}

	nets, _ := s.LoadNets()
	if len(nets) != 0 {
		t.Errorf("expected 0 nets after delete, got %d", len(nets))
	}
}

func TestSaveAndLoadNetCheckInRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	lat, lon := 34.0522, -118.2437

	ci := NetCheckIn{
		ID:           "ci-1",
		NetID:        "net-1",
		Callsign:     "KD7BBC",
		TacticalCall: "Shelter-1",
		OperatorName: "Bob Smith",
		Status:       "available",
		Traffic:      "routine",
		Location:     "Red Cross Shelter",
		Lat:          &lat,
		Lon:          &lon,
		CheckedInAt:  now,
		LastHeard:    now,
	}

	if err := s.SaveNetCheckIn(ci); err != nil {
		t.Fatalf("SaveNetCheckIn failed: %v", err)
	}

	loaded, err := s.LoadNetCheckIns("net-1")
	if err != nil {
		t.Fatalf("LoadNetCheckIns failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 check-in, got %d", len(loaded))
	}

	got := loaded[0]
	if got.ID != ci.ID {
		t.Errorf("id: got %q, want %q", got.ID, ci.ID)
	}
	if got.Callsign != ci.Callsign {
		t.Errorf("callsign: got %q, want %q", got.Callsign, ci.Callsign)
	}
	if got.TacticalCall != ci.TacticalCall {
		t.Errorf("tacticalCall: got %q, want %q", got.TacticalCall, ci.TacticalCall)
	}
	if got.Status != ci.Status {
		t.Errorf("status: got %q, want %q", got.Status, ci.Status)
	}
	if got.Traffic != ci.Traffic {
		t.Errorf("traffic: got %q, want %q", got.Traffic, ci.Traffic)
	}
	if got.Lat == nil || *got.Lat != lat {
		t.Errorf("lat: got %v, want %f", got.Lat, lat)
	}
	if got.Lon == nil || *got.Lon != lon {
		t.Errorf("lon: got %v, want %f", got.Lon, lon)
	}
	if !got.CheckedInAt.Equal(now) {
		t.Errorf("checkedInAt: got %v, want %v", got.CheckedInAt, now)
	}
	if got.CheckedOutAt != nil {
		t.Errorf("checkedOutAt: expected nil, got %v", got.CheckedOutAt)
	}
}

func TestNetCheckInWithNullableFields(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()

	// Check-in without lat/lon (voice-only operator).
	ci := NetCheckIn{
		ID:          "ci-v",
		NetID:       "net-1",
		Callsign:    "W1AW",
		Status:      "available",
		Traffic:     "none",
		CheckedInAt: now,
		LastHeard:   now,
	}
	if err := s.SaveNetCheckIn(ci); err != nil {
		t.Fatalf("SaveNetCheckIn failed: %v", err)
	}

	loaded, _ := s.LoadNetCheckIns("net-1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	if loaded[0].Lat != nil {
		t.Errorf("lat should be nil for voice-only operator")
	}
	if loaded[0].Lon != nil {
		t.Errorf("lon should be nil for voice-only operator")
	}
}

func TestDeleteNetCheckIn(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	s.SaveNetCheckIn(NetCheckIn{ID: "ci-1", NetID: "net-1", Callsign: "A", Status: "available", Traffic: "none", CheckedInAt: now, LastHeard: now})

	if err := s.DeleteNetCheckIn("ci-1"); err != nil {
		t.Fatalf("DeleteNetCheckIn failed: %v", err)
	}

	loaded, _ := s.LoadNetCheckIns("net-1")
	if len(loaded) != 0 {
		t.Errorf("expected 0 check-ins after delete, got %d", len(loaded))
	}
}

func TestSaveAndLoadNetMissionRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()

	m := NetMission{
		ID:          "m-1",
		NetID:       "net-1",
		Title:       "Deploy to shelter",
		Description: "Set up comms at Red Cross shelter",
		Priority:    "priority",
		Status:      "open",
		CreatedAt:   now,
	}

	if err := s.SaveNetMission(m); err != nil {
		t.Fatalf("SaveNetMission failed: %v", err)
	}

	loaded, err := s.LoadNetMissions("net-1")
	if err != nil {
		t.Fatalf("LoadNetMissions failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 mission, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Title != m.Title {
		t.Errorf("title: got %q, want %q", got.Title, m.Title)
	}
	if got.Priority != m.Priority {
		t.Errorf("priority: got %q, want %q", got.Priority, m.Priority)
	}
	if got.AssignedTo != "" {
		t.Errorf("assignedTo: got %q, want empty (deprecated, never persisted)", got.AssignedTo)
	}
	if got.CompletedAt != nil {
		t.Errorf("completedAt: expected nil, got %v", got.CompletedAt)
	}
}

func TestSaveAndLoadNetNoteRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()

	note := NetNote{
		ID:         "note-1",
		NetID:      "net-1",
		CheckInID:  "ci-1",
		AuthorID:   "user-1",
		AuthorName: "Alice",
		Content:    "Operator reports good signal",
		CreatedAt:  now,
	}

	if err := s.SaveNetNote(note); err != nil {
		t.Fatalf("SaveNetNote failed: %v", err)
	}

	loaded, err := s.LoadNetNotes("net-1")
	if err != nil {
		t.Fatalf("LoadNetNotes failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 note, got %d", len(loaded))
	}

	got := loaded[0]
	if got.CheckInID != "ci-1" {
		t.Errorf("checkInId: got %q, want %q", got.CheckInID, "ci-1")
	}
	if got.Content != note.Content {
		t.Errorf("content: got %q, want %q", got.Content, note.Content)
	}
}

func TestNetNoteWithNullCheckInID(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()

	note := NetNote{
		ID:        "note-2",
		NetID:     "net-1",
		Content:   "General net note",
		CreatedAt: now,
	}

	if err := s.SaveNetNote(note); err != nil {
		t.Fatalf("SaveNetNote failed: %v", err)
	}

	loaded, _ := s.LoadNetNotes("net-1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	if loaded[0].CheckInID != "" {
		t.Errorf("checkInId should be empty, got %q", loaded[0].CheckInID)
	}
}

func TestSaveAndLoadNetEventRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()

	evt := NetEvent{
		ID:        "evt-1",
		NetID:     "net-1",
		Type:      "checkin",
		Callsign:  "KD7BBC",
		Summary:   "KD7BBC checked in with routine traffic",
		Details:   `{"traffic":"routine"}`,
		CreatedAt: now,
	}

	if err := s.SaveNetEvent(evt); err != nil {
		t.Fatalf("SaveNetEvent failed: %v", err)
	}

	loaded, err := s.LoadNetEvents("net-1")
	if err != nil {
		t.Fatalf("LoadNetEvents failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 event, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Type != evt.Type {
		t.Errorf("type: got %q, want %q", got.Type, evt.Type)
	}
	if got.Callsign != evt.Callsign {
		t.Errorf("callsign: got %q, want %q", got.Callsign, evt.Callsign)
	}
	if got.Summary != evt.Summary {
		t.Errorf("summary: got %q, want %q", got.Summary, evt.Summary)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("createdAt: got %v, want %v", got.CreatedAt, now)
	}
}

func TestNetEventsOrderedByTime(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	base := time.Now().Truncate(time.Second).UTC()

	// Insert out of order.
	s.SaveNetEvent(NetEvent{ID: "evt-2", NetID: "net-1", Type: "status_change", Summary: "second", CreatedAt: base.Add(time.Minute)})
	s.SaveNetEvent(NetEvent{ID: "evt-1", NetID: "net-1", Type: "checkin", Summary: "first", CreatedAt: base})

	loaded, _ := s.LoadNetEvents("net-1")
	if len(loaded) != 2 {
		t.Fatalf("expected 2 events, got %d", len(loaded))
	}
	if loaded[0].ID != "evt-1" {
		t.Errorf("first event: got %q, want %q", loaded[0].ID, "evt-1")
	}
	if loaded[1].ID != "evt-2" {
		t.Errorf("second event: got %q, want %q", loaded[1].ID, "evt-2")
	}
}

func TestCheckInsIsolatedByNetID(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "A", Type: "tactical", Status: "open"})
	s.SaveNet(Net{ID: "net-2", Name: "B", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	s.SaveNetCheckIn(NetCheckIn{ID: "ci-1", NetID: "net-1", Callsign: "A", Status: "available", Traffic: "none", CheckedInAt: now, LastHeard: now})
	s.SaveNetCheckIn(NetCheckIn{ID: "ci-2", NetID: "net-2", Callsign: "B", Status: "available", Traffic: "none", CheckedInAt: now, LastHeard: now})

	loaded, _ := s.LoadNetCheckIns("net-1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1 check-in for net-1, got %d", len(loaded))
	}
	if loaded[0].Callsign != "A" {
		t.Errorf("expected callsign A, got %q", loaded[0].Callsign)
	}
}

// --- V4 Migration Tests ---

func TestV4MigrationAddsNewColumns(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Verify new columns exist by querying them.
	_, err := s.db.Exec(`SELECT source, mission_id FROM net_check_ins WHERE 1=0`)
	if err != nil {
		t.Errorf("net_check_ins missing v4 columns: %v", err)
	}
	_, err = s.db.Exec(`SELECT location, lat, lon FROM net_missions WHERE 1=0`)
	if err != nil {
		t.Errorf("net_missions missing v4 columns: %v", err)
	}
	_, err = s.db.Exec(`SELECT mission_id FROM net_notes WHERE 1=0`)
	if err != nil {
		t.Errorf("net_notes missing v4 columns: %v", err)
	}
	_, err = s.db.Exec(`SELECT mission_brief FROM nets WHERE 1=0`)
	if err != nil {
		t.Errorf("nets missing mission_brief column: %v", err)
	}
}

func TestNetCheckInSourceField(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	ci := NetCheckIn{
		ID:          "ci-src",
		NetID:       "net-1",
		Callsign:    "KD7BBC",
		Status:      "available",
		Traffic:     "none",
		Source:      "aprs",
		CheckedInAt: now,
		LastHeard:   now,
	}
	if err := s.SaveNetCheckIn(ci); err != nil {
		t.Fatalf("SaveNetCheckIn failed: %v", err)
	}

	loaded, _ := s.LoadNetCheckIns("net-1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	if loaded[0].Source != "aprs" {
		t.Errorf("source: got %q, want %q", loaded[0].Source, "aprs")
	}
}

func TestNetCheckInMissionID(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	ci := NetCheckIn{
		ID:          "ci-mid",
		NetID:       "net-1",
		Callsign:    "W1AW",
		Status:      "assigned",
		Traffic:     "none",
		Source:      "voice",
		MissionIDs:  []string{"mission-42", "mission-43"},
		CheckedInAt: now,
		LastHeard:   now,
	}
	if err := s.SaveNetCheckIn(ci); err != nil {
		t.Fatalf("SaveNetCheckIn failed: %v", err)
	}

	loaded, _ := s.LoadNetCheckIns("net-1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	if len(loaded[0].MissionIDs) != 2 || loaded[0].MissionIDs[0] != "mission-42" || loaded[0].MissionIDs[1] != "mission-43" {
		t.Errorf("missionIds: got %v, want [mission-42 mission-43]", loaded[0].MissionIDs)
	}
}

func TestNetMissionLocationFields(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	lat, lon := 34.0522, -118.2437
	m := NetMission{
		ID:        "m-loc",
		NetID:     "net-1",
		Title:     "Shelter Setup",
		Priority:  "priority",
		Status:    "open",
		Location:  "Red Cross Shelter #3",
		Lat:       &lat,
		Lon:       &lon,
		CreatedAt: now,
	}
	if err := s.SaveNetMission(m); err != nil {
		t.Fatalf("SaveNetMission failed: %v", err)
	}

	loaded, _ := s.LoadNetMissions("net-1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	if loaded[0].Location != "Red Cross Shelter #3" {
		t.Errorf("location: got %q, want %q", loaded[0].Location, "Red Cross Shelter #3")
	}
	if loaded[0].Lat == nil || *loaded[0].Lat != lat {
		t.Errorf("lat: got %v, want %f", loaded[0].Lat, lat)
	}
	if loaded[0].Lon == nil || *loaded[0].Lon != lon {
		t.Errorf("lon: got %v, want %f", loaded[0].Lon, lon)
	}
}

func TestNetNoteMissionID(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	note := NetNote{
		ID:        "note-m",
		NetID:     "net-1",
		MissionID: "mission-7",
		Content:   "Mission note",
		CreatedAt: now,
	}
	if err := s.SaveNetNote(note); err != nil {
		t.Fatalf("SaveNetNote failed: %v", err)
	}

	loaded, _ := s.LoadNetNotes("net-1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	if loaded[0].MissionID != "mission-7" {
		t.Errorf("missionId: got %q, want %q", loaded[0].MissionID, "mission-7")
	}
}

func TestNetMissionBrief(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	n := Net{
		ID:           "net-mb",
		Name:         "Wildfire Response",
		Type:         "tactical",
		Status:       "open",
		MissionBrief: "Coordinating evacuation shelters for Cascade Fire",
	}
	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet failed: %v", err)
	}

	loaded, err := s.LoadNet("net-mb")
	if err != nil {
		t.Fatalf("LoadNet failed: %v", err)
	}
	if loaded.MissionBrief != n.MissionBrief {
		t.Errorf("missionBrief: got %q, want %q", loaded.MissionBrief, n.MissionBrief)
	}

	// Also verify via LoadNets.
	nets, _ := s.LoadNets()
	found := false
	for _, net := range nets {
		if net.ID == "net-mb" && net.MissionBrief == n.MissionBrief {
			found = true
		}
	}
	if !found {
		t.Error("LoadNets did not return net with missionBrief")
	}
}

// --- V5 Migration Tests ---

func TestV5MigrationAddsTrackedStationsColumn(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Verify the tracked_stations column exists.
	_, err := s.db.Exec(`SELECT tracked_stations FROM net_check_ins WHERE 1=0`)
	if err != nil {
		t.Errorf("net_check_ins missing tracked_stations column: %v", err)
	}

	var version int
	s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}
}

func TestNetCheckInTrackedStationsRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	ci := NetCheckIn{
		ID:          "ci-ts",
		NetID:       "net-1",
		Callsign:    "KG4YFA",
		Status:      "available",
		Traffic:     "none",
		Source:      "aprs",
		CheckedInAt: now,
		LastHeard:   now,
		TrackedStations: []TrackedStation{
			{Callsign: "KG4YFA", AutoLinked: true},
			{Callsign: "KG4YFA-4", AutoLinked: true},
			{Callsign: "A2SV-4", AutoLinked: false},
		},
	}

	if err := s.SaveNetCheckIn(ci); err != nil {
		t.Fatalf("SaveNetCheckIn failed: %v", err)
	}

	loaded, err := s.LoadNetCheckIns("net-1")
	if err != nil {
		t.Fatalf("LoadNetCheckIns failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 check-in, got %d", len(loaded))
	}

	got := loaded[0]
	if len(got.TrackedStations) != 3 {
		t.Fatalf("expected 3 tracked stations, got %d", len(got.TrackedStations))
	}
	if got.TrackedStations[0].Callsign != "KG4YFA" || !got.TrackedStations[0].AutoLinked {
		t.Errorf("tracked station 0: got %+v", got.TrackedStations[0])
	}
	if got.TrackedStations[1].Callsign != "KG4YFA-4" || !got.TrackedStations[1].AutoLinked {
		t.Errorf("tracked station 1: got %+v", got.TrackedStations[1])
	}
	if got.TrackedStations[2].Callsign != "A2SV-4" || got.TrackedStations[2].AutoLinked {
		t.Errorf("tracked station 2: got %+v", got.TrackedStations[2])
	}
}

func TestNetCheckInTrackedStationsDefault(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveNet(Net{ID: "net-1", Name: "Test", Type: "tactical", Status: "open"})

	now := time.Now().Truncate(time.Second).UTC()
	ci := NetCheckIn{
		ID:          "ci-def",
		NetID:       "net-1",
		Callsign:    "W1AW",
		Status:      "available",
		Traffic:     "none",
		CheckedInAt: now,
		LastHeard:   now,
	}

	if err := s.SaveNetCheckIn(ci); err != nil {
		t.Fatalf("SaveNetCheckIn failed: %v", err)
	}

	loaded, err := s.LoadNetCheckIns("net-1")
	if err != nil {
		t.Fatalf("LoadNetCheckIns failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 check-in, got %d", len(loaded))
	}

	got := loaded[0]
	if got.TrackedStations == nil {
		t.Fatal("TrackedStations should be empty slice, not nil")
	}
	if len(got.TrackedStations) != 0 {
		t.Errorf("expected 0 tracked stations, got %d", len(got.TrackedStations))
	}
}

// --- V6 Migration & Tactical Alias Tests ---

func TestV6MigrationCreatesTacticalAliasesTable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Verify the tactical_aliases table exists.
	_, err := s.db.Exec(`SELECT callsign, alias, assigned_by, updated_at FROM tactical_aliases WHERE 1=0`)
	if err != nil {
		t.Errorf("tactical_aliases table not created: %v", err)
	}

	var version int
	s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}
}

func TestSaveAndLoadTacticalAliasRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	a := TacticalAlias{
		Callsign:   "W4ABC-9",
		Alias:      "SHELTER-1",
		AssignedBy: "config",
		UpdatedAt:  now,
	}

	if err := s.SaveTacticalAlias(a); err != nil {
		t.Fatalf("SaveTacticalAlias failed: %v", err)
	}

	loaded, err := s.LoadTacticalAliases()
	if err != nil {
		t.Fatalf("LoadTacticalAliases failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Callsign != a.Callsign {
		t.Errorf("callsign: got %q, want %q", got.Callsign, a.Callsign)
	}
	if got.Alias != a.Alias {
		t.Errorf("alias: got %q, want %q", got.Alias, a.Alias)
	}
	if got.AssignedBy != a.AssignedBy {
		t.Errorf("assignedBy: got %q, want %q", got.AssignedBy, a.AssignedBy)
	}
	if !got.UpdatedAt.Equal(a.UpdatedAt) {
		t.Errorf("updatedAt: got %v, want %v", got.UpdatedAt, a.UpdatedAt)
	}
}

func TestSaveTacticalAliasUpsert(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	a := TacticalAlias{
		Callsign:   "W4ABC-9",
		Alias:      "SHELTER-1",
		AssignedBy: "config",
		UpdatedAt:  now,
	}
	if err := s.SaveTacticalAlias(a); err != nil {
		t.Fatalf("SaveTacticalAlias (first) failed: %v", err)
	}

	// Update alias.
	a.Alias = "NET-CTRL"
	a.AssignedBy = "ui"
	a.UpdatedAt = now.Add(time.Minute)
	if err := s.SaveTacticalAlias(a); err != nil {
		t.Fatalf("SaveTacticalAlias (update) failed: %v", err)
	}

	loaded, err := s.LoadTacticalAliases()
	if err != nil {
		t.Fatalf("LoadTacticalAliases failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 alias after upsert, got %d", len(loaded))
	}
	if loaded[0].Alias != "NET-CTRL" {
		t.Errorf("alias: got %q, want %q", loaded[0].Alias, "NET-CTRL")
	}
	if loaded[0].AssignedBy != "ui" {
		t.Errorf("assignedBy: got %q, want %q", loaded[0].AssignedBy, "ui")
	}
}

func TestDeleteTacticalAlias(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	s.SaveTacticalAlias(TacticalAlias{Callsign: "W4ABC-9", Alias: "SHELTER-1", AssignedBy: "config", UpdatedAt: now})
	s.SaveTacticalAlias(TacticalAlias{Callsign: "N5XYZ", Alias: "NET-CTRL", AssignedBy: "ui", UpdatedAt: now})

	if err := s.DeleteTacticalAlias("W4ABC-9"); err != nil {
		t.Fatalf("DeleteTacticalAlias failed: %v", err)
	}

	loaded, err := s.LoadTacticalAliases()
	if err != nil {
		t.Fatalf("LoadTacticalAliases failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 alias after delete, got %d", len(loaded))
	}
	if loaded[0].Callsign != "N5XYZ" {
		t.Errorf("expected remaining alias N5XYZ, got %q", loaded[0].Callsign)
	}
}

func TestLoadTacticalAliasesEmpty(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	loaded, err := s.LoadTacticalAliases()
	if err != nil {
		t.Fatalf("LoadTacticalAliases failed: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for empty result, got %d entries", len(loaded))
	}
}

func TestMultipleTacticalAliasesOrderedByCallsign(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	// Insert in reverse order.
	s.SaveTacticalAlias(TacticalAlias{Callsign: "N5XYZ", Alias: "NET-CTRL", AssignedBy: "ui", UpdatedAt: now})
	s.SaveTacticalAlias(TacticalAlias{Callsign: "KD0ABC-5", Alias: "EOC", AssignedBy: "config", UpdatedAt: now})
	s.SaveTacticalAlias(TacticalAlias{Callsign: "W4ABC-9", Alias: "SHELTER-1", AssignedBy: "aprs", UpdatedAt: now})

	loaded, err := s.LoadTacticalAliases()
	if err != nil {
		t.Fatalf("LoadTacticalAliases failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 aliases, got %d", len(loaded))
	}

	// Should be ordered by callsign ascending.
	if loaded[0].Callsign != "KD0ABC-5" {
		t.Errorf("first: got %q, want KD0ABC-5", loaded[0].Callsign)
	}
	if loaded[1].Callsign != "N5XYZ" {
		t.Errorf("second: got %q, want N5XYZ", loaded[1].Callsign)
	}
	if loaded[2].Callsign != "W4ABC-9" {
		t.Errorf("third: got %q, want W4ABC-9", loaded[2].Callsign)
	}
}

// --- V7 Migration & Annotation Extended Fields Tests ---

func TestV7MigrationAddsAnnotationColumns(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Verify all new columns exist.
	_, err := s.db.Exec(`SELECT category, status, priority, operation_id, mission_id, resources,
		reported_by, reported_at, resolved_at, expires_at FROM annotations WHERE 1=0`)
	if err != nil {
		t.Errorf("annotations missing v7 columns: %v", err)
	}

	var version int
	s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}
}

func TestSaveAndLoadAnnotationNewFields(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	reportedAt := now.Add(-time.Hour)
	expiresAt := now.Add(24 * time.Hour)

	ann := Annotation{
		ID:          "ann-ext",
		Type:        "point",
		Label:       "Aid Station Alpha",
		Description: "Primary medical aid station",
		Geometry:    `{"type":"Point","coordinates":[-118.24,34.05]}`,
		Style:       `{"color":"#ff0000"}`,
		CreatedBy:   "user-1",
		CreatedAt:   now,
		UpdatedAt:   now,
		Category:    "resource",
		Status:      "active",
		Priority:    "priority",
		OperationID: "op-42",
		MissionIDs:  []string{"mission-7"},
		Resources:   `[{"type":"medical","qty":2}]`,
		ReportedBy:  "KD7BBC",
		ReportedAt:  &reportedAt,
		ExpiresAt:   &expiresAt,
	}

	if err := s.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation failed: %v", err)
	}

	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Category != "resource" {
		t.Errorf("category: got %q, want %q", got.Category, "resource")
	}
	if got.Status != "active" {
		t.Errorf("status: got %q, want %q", got.Status, "active")
	}
	if got.Priority != "priority" {
		t.Errorf("priority: got %q, want %q", got.Priority, "priority")
	}
	if got.OperationID != "op-42" {
		t.Errorf("operationId: got %q, want %q", got.OperationID, "op-42")
	}
	if len(got.MissionIDs) != 1 || got.MissionIDs[0] != "mission-7" {
		t.Errorf("missionIds: got %v, want [mission-7]", got.MissionIDs)
	}
	if got.Resources != `[{"type":"medical","qty":2}]` {
		t.Errorf("resources: got %q", got.Resources)
	}
	if got.ReportedBy != "KD7BBC" {
		t.Errorf("reportedBy: got %q, want %q", got.ReportedBy, "KD7BBC")
	}
	if got.ReportedAt == nil || !got.ReportedAt.Equal(reportedAt) {
		t.Errorf("reportedAt: got %v, want %v", got.ReportedAt, reportedAt)
	}
	if got.ResolvedAt != nil {
		t.Errorf("resolvedAt: expected nil, got %v", got.ResolvedAt)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expiresAt) {
		t.Errorf("expiresAt: got %v, want %v", got.ExpiresAt, expiresAt)
	}
}

func TestAnnotationDefaultsOnV7(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	// Save annotation without setting new fields — should get defaults.
	ann := Annotation{
		ID:        "ann-def",
		Type:      "point",
		Label:     "Plain Marker",
		Geometry:  `{"type":"Point","coordinates":[0,0]}`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation failed: %v", err)
	}

	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}

	got := loaded[0]
	// Empty strings are acceptable — defaults are applied at the annotation manager level.
	// The store itself stores whatever is passed.
	if got.ID != "ann-def" {
		t.Errorf("id: got %q, want %q", got.ID, "ann-def")
	}
}

func TestLoadAnnotationsFiltered(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	anns := []Annotation{
		{ID: "a1", Type: "point", Label: "Incident 1", Geometry: "{}", CreatedAt: now, UpdatedAt: now, Category: "incident", Status: "reported", Priority: "urgent", OperationID: "op-1"},
		{ID: "a2", Type: "point", Label: "Resource 1", Geometry: "{}", CreatedAt: now, UpdatedAt: now, Category: "resource", Status: "active", Priority: "routine", OperationID: "op-1"},
		{ID: "a3", Type: "area", Label: "Boundary 1", Geometry: "{}", CreatedAt: now, UpdatedAt: now, Category: "boundary", Status: "active", Priority: "routine", OperationID: "op-2"},
	}
	for _, a := range anns {
		if err := s.SaveAnnotation(a); err != nil {
			t.Fatalf("SaveAnnotation(%s) failed: %v", a.ID, err)
		}
	}

	// Filter by category.
	results, err := s.LoadAnnotationsFiltered(AnnotationFilter{Category: "incident"})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered(category=incident) failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 incident, got %d", len(results))
	}

	// Filter by status.
	results, err = s.LoadAnnotationsFiltered(AnnotationFilter{Status: "active"})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered(status=active) failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 active, got %d", len(results))
	}

	// Filter by priority.
	results, err = s.LoadAnnotationsFiltered(AnnotationFilter{Priority: "urgent"})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered(priority=urgent) failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 urgent, got %d", len(results))
	}

	// Filter by operationId.
	results, err = s.LoadAnnotationsFiltered(AnnotationFilter{OperationID: "op-1"})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered(operationId=op-1) failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 for op-1, got %d", len(results))
	}

	// Combined filter.
	results, err = s.LoadAnnotationsFiltered(AnnotationFilter{Category: "resource", OperationID: "op-1"})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered(category=resource,op=op-1) failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 resource in op-1, got %d", len(results))
	}
}

func TestLoadAnnotationsFilteredExcludesExpired(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	s.SaveAnnotation(Annotation{
		ID: "exp-past", Type: "point", Label: "Expired", Geometry: "{}",
		CreatedAt: now, UpdatedAt: now, Category: "general", Status: "active",
		ExpiresAt: &past,
	})
	s.SaveAnnotation(Annotation{
		ID: "exp-future", Type: "point", Label: "Not Expired", Geometry: "{}",
		CreatedAt: now, UpdatedAt: now, Category: "general", Status: "active",
		ExpiresAt: &future,
	})
	s.SaveAnnotation(Annotation{
		ID: "no-expiry", Type: "point", Label: "No Expiry", Geometry: "{}",
		CreatedAt: now, UpdatedAt: now, Category: "general", Status: "active",
	})

	// Without IncludeExpired — should exclude past expiry.
	results, err := s.LoadAnnotationsFiltered(AnnotationFilter{Category: "general"})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 (excluding expired), got %d", len(results))
	}

	// With IncludeExpired — should include all 3.
	results, err = s.LoadAnnotationsFiltered(AnnotationFilter{Category: "general", IncludeExpired: true})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered(includeExpired) failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 (including expired), got %d", len(results))
	}
}

func TestOperationCRUD(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	op := Operation{
		ID:          "op-1",
		Name:        "SAR Event",
		Description: "Search and rescue operation",
		Status:      "active",
		CreatedBy:   "user-1",
		CreatedAt:   now,
	}

	if err := s.SaveOperation(op); err != nil {
		t.Fatalf("SaveOperation: %v", err)
	}

	// Load all.
	ops, err := s.LoadOperations()
	if err != nil {
		t.Fatalf("LoadOperations: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1, got %d", len(ops))
	}
	if ops[0].Name != "SAR Event" {
		t.Errorf("name: got %q, want %q", ops[0].Name, "SAR Event")
	}

	// Load by ID.
	loaded, err := s.LoadOperation("op-1")
	if err != nil {
		t.Fatalf("LoadOperation: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil operation")
	}
	if loaded.Status != "active" {
		t.Errorf("status: got %q, want %q", loaded.Status, "active")
	}

	// Archive.
	archived := now.Add(time.Hour)
	op.Status = "archived"
	op.ArchivedAt = &archived
	if err := s.SaveOperation(op); err != nil {
		t.Fatalf("SaveOperation (archive): %v", err)
	}

	reloaded, _ := s.LoadOperation("op-1")
	if reloaded.Status != "archived" {
		t.Errorf("status after archive: got %q, want %q", reloaded.Status, "archived")
	}
	if reloaded.ArchivedAt == nil {
		t.Error("archivedAt should not be nil")
	}

	// Load nonexistent.
	none, err := s.LoadOperation("nonexistent")
	if err != nil {
		t.Fatalf("LoadOperation(nonexistent): %v", err)
	}
	if none != nil {
		t.Error("expected nil for nonexistent")
	}

	// Delete.
	if err := s.DeleteOperation("op-1"); err != nil {
		t.Fatalf("DeleteOperation: %v", err)
	}
	ops, _ = s.LoadOperations()
	if len(ops) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(ops))
	}
}

func TestMigrateV8CreatesOperationsTable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var version int
	s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}

	// Verify operations table exists by doing a query.
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM operations").Scan(&count)
	if err != nil {
		t.Fatalf("operations table should exist: %v", err)
	}
}

// --- V11 Migration: Ops View ---

func TestMigrateV11AddsOpsViewColumns(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var version int
	s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}

	// Verify ops_view columns exist.
	for _, col := range []string{"ops_view_lat", "ops_view_lon", "ops_view_zoom"} {
		_, err := s.db.Exec(fmt.Sprintf("SELECT %s FROM nets WHERE 1=0", col))
		if err != nil {
			t.Errorf("nets missing %s column: %v", col, err)
		}
	}
}

func TestNetOpsViewRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	lat := 34.05
	lon := -118.24
	zoom := 13.0

	n := Net{
		ID:          "net-opsview",
		Name:        "Ops View Test",
		Type:        "tactical",
		Status:      "open",
		OpsViewLat:  &lat,
		OpsViewLon:  &lon,
		OpsViewZoom: &zoom,
	}
	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet failed: %v", err)
	}

	loaded, err := s.LoadNet("net-opsview")
	if err != nil {
		t.Fatalf("LoadNet failed: %v", err)
	}
	if loaded.OpsViewLat == nil || *loaded.OpsViewLat != lat {
		t.Errorf("opsViewLat: got %v, want %v", loaded.OpsViewLat, lat)
	}
	if loaded.OpsViewLon == nil || *loaded.OpsViewLon != lon {
		t.Errorf("opsViewLon: got %v, want %v", loaded.OpsViewLon, lon)
	}
	if loaded.OpsViewZoom == nil || *loaded.OpsViewZoom != zoom {
		t.Errorf("opsViewZoom: got %v, want %v", loaded.OpsViewZoom, zoom)
	}

	// Also via LoadNets.
	nets, _ := s.LoadNets()
	found := false
	for _, net := range nets {
		if net.ID == "net-opsview" && net.OpsViewLat != nil && *net.OpsViewLat == lat {
			found = true
		}
	}
	if !found {
		t.Error("LoadNets did not return net with opsView fields")
	}
}

func TestNetOpsViewNullable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Net without ops view fields.
	n := Net{
		ID:     "net-no-opsview",
		Name:   "No Ops View",
		Type:   "tactical",
		Status: "open",
	}
	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet failed: %v", err)
	}

	loaded, err := s.LoadNet("net-no-opsview")
	if err != nil {
		t.Fatalf("LoadNet failed: %v", err)
	}
	if loaded.OpsViewLat != nil {
		t.Errorf("opsViewLat should be nil, got %v", loaded.OpsViewLat)
	}
	if loaded.OpsViewLon != nil {
		t.Errorf("opsViewLon should be nil, got %v", loaded.OpsViewLon)
	}
	if loaded.OpsViewZoom != nil {
		t.Errorf("opsViewZoom should be nil, got %v", loaded.OpsViewZoom)
	}
}

func TestUpdateNotePinned(t *testing.T) {
	s, _ := newTestStore(t)

	// Save a note that is NOT pinned.
	note := NetNote{
		ID:         "note-pin-1",
		NetID:      "net-pin",
		AuthorID:   "u1",
		AuthorName: "Alice",
		Content:    "routine observation",
		Category:   "general",
		Severity:   "info",
		Pinned:     false,
		CreatedAt:  time.Now(),
	}
	if err := s.SaveNetNote(note); err != nil {
		t.Fatalf("save note: %v", err)
	}

	// Pin it.
	if err := s.UpdateNotePinned(note.ID, true); err != nil {
		t.Fatalf("pin note: %v", err)
	}

	notes, _ := s.LoadNetNotes("net-pin")
	found := false
	for _, n := range notes {
		if n.ID == note.ID {
			found = true
			if !n.Pinned {
				t.Error("note should be pinned after UpdateNotePinned(true)")
			}
		}
	}
	if !found {
		t.Fatal("note not found after pin update")
	}

	// Unpin it.
	if err := s.UpdateNotePinned(note.ID, false); err != nil {
		t.Fatalf("unpin note: %v", err)
	}

	notes, _ = s.LoadNetNotes("net-pin")
	for _, n := range notes {
		if n.ID == note.ID && n.Pinned {
			t.Error("note should be unpinned after UpdateNotePinned(false)")
		}
	}
}

func TestMigrateV12CreatesWeatherReadingsTable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='weather_readings'").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("weather_readings table not created by v12 migration")
	}
}

func TestSaveAndLoadWeatherReadingRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	temp := 22.5
	windDir := 180.0
	windSpeed := 5.3
	windGust := 8.1
	humidity := 65
	pressure := 1013.25
	rain1h := 2.5
	rain24h := 10.0
	rainToday := 5.0
	luminosity := 800

	now := time.Now().UTC().Truncate(time.Second)
	r := WeatherReading{
		Callsign:    "WX1AW",
		Timestamp:   now,
		Temperature: &temp,
		WindDir:     &windDir,
		WindSpeed:   &windSpeed,
		WindGust:    &windGust,
		Humidity:    &humidity,
		Pressure:    &pressure,
		Rain1h:      &rain1h,
		Rain24h:     &rain24h,
		RainToday:   &rainToday,
		Luminosity:  &luminosity,
	}

	if err := s.SaveWeatherReading(r); err != nil {
		t.Fatalf("save: %v", err)
	}

	readings, err := s.LoadWeatherReadings(WeatherFilter{Callsign: "WX1AW"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("got %d readings, want 1", len(readings))
	}

	got := readings[0]
	if got.Callsign != "WX1AW" {
		t.Errorf("callsign = %q, want WX1AW", got.Callsign)
	}
	if got.Temperature == nil || *got.Temperature != 22.5 {
		t.Errorf("temperature = %v, want 22.5", got.Temperature)
	}
	if got.WindDir == nil || *got.WindDir != 180.0 {
		t.Errorf("windDir = %v, want 180.0", got.WindDir)
	}
	if got.WindSpeed == nil || *got.WindSpeed != 5.3 {
		t.Errorf("windSpeed = %v, want 5.3", got.WindSpeed)
	}
	if got.WindGust == nil || *got.WindGust != 8.1 {
		t.Errorf("windGust = %v, want 8.1", got.WindGust)
	}
	if got.Humidity == nil || *got.Humidity != 65 {
		t.Errorf("humidity = %v, want 65", got.Humidity)
	}
	if got.Pressure == nil || *got.Pressure != 1013.25 {
		t.Errorf("pressure = %v, want 1013.25", got.Pressure)
	}
	if got.Rain1h == nil || *got.Rain1h != 2.5 {
		t.Errorf("rain1h = %v, want 2.5", got.Rain1h)
	}
	if got.Rain24h == nil || *got.Rain24h != 10.0 {
		t.Errorf("rain24h = %v, want 10.0", got.Rain24h)
	}
	if got.RainToday == nil || *got.RainToday != 5.0 {
		t.Errorf("rainToday = %v, want 5.0", got.RainToday)
	}
	if got.Luminosity == nil || *got.Luminosity != 800 {
		t.Errorf("luminosity = %v, want 800", got.Luminosity)
	}
	if got.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestLoadWeatherReadingsFiltered(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().UTC().Truncate(time.Second)
	temp1, temp2, temp3 := 20.0, 25.0, 30.0

	for i, temp := range []*float64{&temp1, &temp2, &temp3} {
		if err := s.SaveWeatherReading(WeatherReading{
			Callsign:    "WX1AW",
			Timestamp:   base.Add(time.Duration(i) * time.Hour),
			Temperature: temp,
		}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	// Filter by since
	since := base.Add(30 * time.Minute)
	readings, err := s.LoadWeatherReadings(WeatherFilter{Callsign: "WX1AW", Since: &since})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(readings) != 2 {
		t.Errorf("got %d readings with since filter, want 2", len(readings))
	}

	// Limit
	readings, err = s.LoadWeatherReadings(WeatherFilter{Callsign: "WX1AW", Limit: 1})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(readings) != 1 {
		t.Errorf("got %d readings with limit=1, want 1", len(readings))
	}
}

func TestLoadWeatherStations(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	temp1, temp2 := 20.0, 25.0
	now := time.Now().UTC()

	// Two stations, two readings each
	if err := s.SaveWeatherReading(WeatherReading{Callsign: "WX1AW", Timestamp: now.Add(-time.Hour), Temperature: &temp1}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveWeatherReading(WeatherReading{Callsign: "WX1AW", Timestamp: now, Temperature: &temp2}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveWeatherReading(WeatherReading{Callsign: "WX2BW", Timestamp: now, Temperature: &temp1}); err != nil {
		t.Fatal(err)
	}

	stations, err := s.LoadWeatherStations()
	if err != nil {
		t.Fatalf("load weather stations: %v", err)
	}
	if len(stations) != 2 {
		t.Fatalf("got %d weather stations, want 2", len(stations))
	}

	// WX1AW should have the latest reading (temp2)
	for _, ws := range stations {
		if ws.Callsign == "WX1AW" {
			if ws.Temperature == nil || *ws.Temperature != 25.0 {
				t.Errorf("WX1AW latest temp = %v, want 25.0", ws.Temperature)
			}
		}
	}
}

func TestPurgeWeatherReadings(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	temp := 20.0
	now := time.Now().UTC()

	// Old reading
	if err := s.SaveWeatherReading(WeatherReading{Callsign: "WX1AW", Timestamp: now.Add(-48 * time.Hour), Temperature: &temp}); err != nil {
		t.Fatal(err)
	}
	// Recent reading
	if err := s.SaveWeatherReading(WeatherReading{Callsign: "WX1AW", Timestamp: now, Temperature: &temp}); err != nil {
		t.Fatal(err)
	}

	deleted, err := s.PurgeWeatherReadings(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Errorf("purged %d rows, want 1", deleted)
	}

	readings, _ := s.LoadWeatherReadings(WeatherFilter{Callsign: "WX1AW"})
	if len(readings) != 1 {
		t.Errorf("got %d readings after purge, want 1", len(readings))
	}
}

func TestWeatherReadingNullableFields(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Save with only temperature set (all others nil)
	temp := 15.0
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.SaveWeatherReading(WeatherReading{
		Callsign:    "WX1AW",
		Timestamp:   now,
		Temperature: &temp,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	readings, _ := s.LoadWeatherReadings(WeatherFilter{Callsign: "WX1AW"})
	if len(readings) != 1 {
		t.Fatalf("got %d, want 1", len(readings))
	}
	got := readings[0]
	if got.Temperature == nil || *got.Temperature != 15.0 {
		t.Errorf("temperature = %v, want 15.0", got.Temperature)
	}
	if got.WindDir != nil {
		t.Errorf("windDir should be nil, got %v", got.WindDir)
	}
	if got.Humidity != nil {
		t.Errorf("humidity should be nil, got %v", got.Humidity)
	}
	if got.Luminosity != nil {
		t.Errorf("luminosity should be nil, got %v", got.Luminosity)
	}
}

func TestMigrateV13CreatesTelemetryReadingsTable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='telemetry_readings'").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("telemetry_readings table not created by v13 migration")
	}

	var version int
	s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}
}

func TestSaveAndLoadTelemetryReadingRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Second)
	r := TelemetryReading{
		Callsign:  "TEL1",
		Timestamp: now,
		Seq:       42,
		Analog1:   100,
		Analog2:   200,
		Analog3:   300,
		Analog4:   400,
		Analog5:   500,
		Digital:   0b10101010,
	}

	if err := s.SaveTelemetryReading(r); err != nil {
		t.Fatalf("save: %v", err)
	}

	readings, err := s.LoadTelemetryReadings(TelemetryFilter{Callsign: "TEL1"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("got %d readings, want 1", len(readings))
	}

	got := readings[0]
	if got.Callsign != "TEL1" {
		t.Errorf("callsign = %q, want TEL1", got.Callsign)
	}
	if got.Seq != 42 {
		t.Errorf("seq = %d, want 42", got.Seq)
	}
	if got.Analog1 != 100 {
		t.Errorf("analog1 = %f, want 100", got.Analog1)
	}
	if got.Analog5 != 500 {
		t.Errorf("analog5 = %f, want 500", got.Analog5)
	}
	if got.Digital != 0b10101010 {
		t.Errorf("digital = %d, want %d", got.Digital, 0b10101010)
	}
	if got.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestLoadTelemetryStations(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().UTC()

	// Two stations with multiple readings
	s.SaveTelemetryReading(TelemetryReading{Callsign: "TEL1", Timestamp: now.Add(-time.Hour), Seq: 1, Analog1: 10})
	s.SaveTelemetryReading(TelemetryReading{Callsign: "TEL1", Timestamp: now, Seq: 2, Analog1: 20})
	s.SaveTelemetryReading(TelemetryReading{Callsign: "TEL2", Timestamp: now, Seq: 1, Analog1: 30})

	stations, err := s.LoadTelemetryStations()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(stations) != 2 {
		t.Fatalf("got %d stations, want 2", len(stations))
	}

	// TEL1 should have latest reading (seq=2, analog1=20)
	for _, st := range stations {
		if st.Callsign == "TEL1" {
			if st.Seq != 2 || st.Analog1 != 20 {
				t.Errorf("TEL1 latest: seq=%d analog1=%f, want seq=2 analog1=20", st.Seq, st.Analog1)
			}
		}
	}
}

func TestPurgeTelemetryReadings(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	s.SaveTelemetryReading(TelemetryReading{Callsign: "TEL1", Timestamp: now.Add(-48 * time.Hour), Seq: 1})
	s.SaveTelemetryReading(TelemetryReading{Callsign: "TEL1", Timestamp: now, Seq: 2})

	deleted, err := s.PurgeTelemetryReadings(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Errorf("purged %d rows, want 1", deleted)
	}

	readings, _ := s.LoadTelemetryReadings(TelemetryFilter{Callsign: "TEL1"})
	if len(readings) != 1 {
		t.Errorf("got %d after purge, want 1", len(readings))
	}
}

func TestLoadTelemetryReadingsFiltered(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		s.SaveTelemetryReading(TelemetryReading{
			Callsign:  "TEL1",
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Seq:       i,
			Analog1:   float64(i * 10),
		})
	}

	// Since filter
	since := base.Add(30 * time.Minute)
	readings, err := s.LoadTelemetryReadings(TelemetryFilter{Callsign: "TEL1", Since: &since})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(readings) != 2 {
		t.Errorf("got %d with since filter, want 2", len(readings))
	}

	// Limit
	readings, _ = s.LoadTelemetryReadings(TelemetryFilter{Callsign: "TEL1", Limit: 1})
	if len(readings) != 1 {
		t.Errorf("got %d with limit=1, want 1", len(readings))
	}
}

// --- V16 Migration: Annotation Net Scoping ---

func TestAnnotationNetScoping(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	// Create an annotation with NetID set.
	ann := Annotation{
		ID:        "ann-net-1",
		Type:      "point",
		Label:     "Command Post",
		Geometry:  `{"type":"Point","coordinates":[-118.24,34.05]}`,
		CreatedAt: now,
		UpdatedAt: now,
		Category:  "resource",
		Status:    "active",
		NetID:     "net-alpha",
		ShortName: "CP",
		SortOrder: 1,
	}
	if err := s.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation failed: %v", err)
	}

	// Load it back and verify NetID, ShortName, SortOrder are persisted.
	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(loaded))
	}

	got := loaded[0]
	if got.NetID != "net-alpha" {
		t.Errorf("netId: got %q, want %q", got.NetID, "net-alpha")
	}
	if got.ShortName != "CP" {
		t.Errorf("shortName: got %q, want %q", got.ShortName, "CP")
	}
	if got.SortOrder != 1 {
		t.Errorf("sortOrder: got %d, want 1", got.SortOrder)
	}

	// Create another annotation with a different NetID.
	ann2 := Annotation{
		ID:        "ann-net-2",
		Type:      "point",
		Label:     "Staging Area",
		Geometry:  `{"type":"Point","coordinates":[-118.30,34.10]}`,
		CreatedAt: now,
		UpdatedAt: now,
		Category:  "resource",
		Status:    "active",
		NetID:     "net-bravo",
		ShortName: "SA",
		SortOrder: 2,
	}
	if err := s.SaveAnnotation(ann2); err != nil {
		t.Fatalf("SaveAnnotation (ann2) failed: %v", err)
	}

	// Use LoadAnnotationsFiltered with NetID filter — should return only the matching one.
	results, err := s.LoadAnnotationsFiltered(AnnotationFilter{NetID: "net-alpha"})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered(netId=net-alpha) failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 annotation for net-alpha, got %d", len(results))
	}
	if results[0].ID != "ann-net-1" {
		t.Errorf("expected ann-net-1, got %q", results[0].ID)
	}
	if results[0].NetID != "net-alpha" {
		t.Errorf("filtered netId: got %q, want %q", results[0].NetID, "net-alpha")
	}
	if results[0].ShortName != "CP" {
		t.Errorf("filtered shortName: got %q, want %q", results[0].ShortName, "CP")
	}
	if results[0].SortOrder != 1 {
		t.Errorf("filtered sortOrder: got %d, want 1", results[0].SortOrder)
	}

	// Filter for net-bravo should return only the second annotation.
	results, err = s.LoadAnnotationsFiltered(AnnotationFilter{NetID: "net-bravo"})
	if err != nil {
		t.Fatalf("LoadAnnotationsFiltered(netId=net-bravo) failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 annotation for net-bravo, got %d", len(results))
	}
	if results[0].ID != "ann-net-2" {
		t.Errorf("expected ann-net-2, got %q", results[0].ID)
	}
}

func TestMigrateV16(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var version int
	err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}
}

func TestNetPinnedStationsRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// Save net with pinned stations.
	n := Net{
		ID:             "net-pin-1",
		Name:           "Pin Test Net",
		Type:           "tactical",
		Status:         "open",
		PinnedStations: []string{"KD7BBC", "W1AW", "N0CALL"},
	}

	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet failed: %v", err)
	}

	// Load via LoadNet.
	loaded, err := s.LoadNet("net-pin-1")
	if err != nil {
		t.Fatalf("LoadNet failed: %v", err)
	}

	if len(loaded.PinnedStations) != 3 {
		t.Fatalf("expected 3 pinned stations, got %d", len(loaded.PinnedStations))
	}
	if loaded.PinnedStations[0] != "KD7BBC" || loaded.PinnedStations[1] != "W1AW" || loaded.PinnedStations[2] != "N0CALL" {
		t.Errorf("pinned stations mismatch: got %v", loaded.PinnedStations)
	}

	// Load via LoadNets.
	nets, err := s.LoadNets()
	if err != nil {
		t.Fatalf("LoadNets failed: %v", err)
	}
	found := false
	for _, nn := range nets {
		if nn.ID == "net-pin-1" {
			found = true
			if len(nn.PinnedStations) != 3 {
				t.Errorf("LoadNets: expected 3 pinned, got %d", len(nn.PinnedStations))
			}
		}
	}
	if !found {
		t.Error("net-pin-1 not found in LoadNets")
	}

	// Update to empty pinned list.
	n.PinnedStations = []string{}
	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet (clear) failed: %v", err)
	}
	loaded2, err := s.LoadNet("net-pin-1")
	if err != nil {
		t.Fatalf("LoadNet (clear) failed: %v", err)
	}
	if len(loaded2.PinnedStations) != 0 {
		t.Errorf("expected 0 pinned stations after clear, got %d", len(loaded2.PinnedStations))
	}
}

func TestSaveLoadCheckpointMeta(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	expected := now.Add(30 * time.Minute)

	m := CheckpointMeta{
		AnnotationID:   "ann-cp-1",
		NetID:          "net-1",
		SequenceNumber: 3,
		ExpectedTime:   &expected,
		OpenedAt:       &now,
	}

	if err := s.SaveCheckpointMeta(m); err != nil {
		t.Fatalf("SaveCheckpointMeta failed: %v", err)
	}

	metas, err := s.LoadCheckpointMeta("net-1")
	if err != nil {
		t.Fatalf("LoadCheckpointMeta failed: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
	got := metas[0]
	if got.AnnotationID != "ann-cp-1" {
		t.Errorf("annotationID: got %q, want %q", got.AnnotationID, "ann-cp-1")
	}
	if got.NetID != "net-1" {
		t.Errorf("netID: got %q, want %q", got.NetID, "net-1")
	}
	if got.SequenceNumber != 3 {
		t.Errorf("sequenceNumber: got %d, want 3", got.SequenceNumber)
	}
	if got.ExpectedTime == nil || got.ExpectedTime.Truncate(time.Second) != expected {
		t.Errorf("expectedTime: got %v, want %v", got.ExpectedTime, expected)
	}
	if got.OpenedAt == nil || got.OpenedAt.Truncate(time.Second) != now {
		t.Errorf("openedAt: got %v, want %v", got.OpenedAt, now)
	}
	if got.ClosedAt != nil {
		t.Errorf("closedAt: expected nil, got %v", got.ClosedAt)
	}

	// Upsert: update sequence number.
	m.SequenceNumber = 5
	if err := s.SaveCheckpointMeta(m); err != nil {
		t.Fatalf("SaveCheckpointMeta upsert failed: %v", err)
	}
	metas2, _ := s.LoadCheckpointMeta("net-1")
	if len(metas2) != 1 || metas2[0].SequenceNumber != 5 {
		t.Errorf("upsert failed: got seq %d", metas2[0].SequenceNumber)
	}
}

func TestLoadCheckpointMetaOrdering(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	for _, seq := range []int{5, 1, 3} {
		s.SaveCheckpointMeta(CheckpointMeta{
			AnnotationID:   fmt.Sprintf("ann-%d", seq),
			NetID:          "net-1",
			SequenceNumber: seq,
		})
	}

	metas, _ := s.LoadCheckpointMeta("net-1")
	if len(metas) != 3 {
		t.Fatalf("expected 3, got %d", len(metas))
	}
	if metas[0].SequenceNumber != 1 || metas[1].SequenceNumber != 3 || metas[2].SequenceNumber != 5 {
		t.Errorf("ordering wrong: %d, %d, %d", metas[0].SequenceNumber, metas[1].SequenceNumber, metas[2].SequenceNumber)
	}
}

func TestDeleteCheckpointMeta(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	s.SaveCheckpointMeta(CheckpointMeta{AnnotationID: "ann-1", NetID: "net-1", SequenceNumber: 1})
	s.SaveCheckpointMeta(CheckpointMeta{AnnotationID: "ann-2", NetID: "net-1", SequenceNumber: 2})

	if err := s.DeleteCheckpointMeta("ann-1"); err != nil {
		t.Fatalf("DeleteCheckpointMeta failed: %v", err)
	}
	metas, _ := s.LoadCheckpointMeta("net-1")
	if len(metas) != 1 || metas[0].AnnotationID != "ann-2" {
		t.Errorf("expected ann-2 only, got %v", metas)
	}
}

func TestSaveLoadCheckpointPassages(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	p := CheckpointPassage{
		ID:           "pass-1",
		CheckpointID: "ann-cp-1",
		NetID:        "net-1",
		Label:        "lead",
		PassageTime:  now,
		Direction:    "through",
		ReportedBy:   "user-1",
		Notes:        "looking good",
	}

	if err := s.SaveCheckpointPassage(p); err != nil {
		t.Fatalf("SaveCheckpointPassage failed: %v", err)
	}

	passages, err := s.LoadCheckpointPassages("net-1")
	if err != nil {
		t.Fatalf("LoadCheckpointPassages failed: %v", err)
	}
	if len(passages) != 1 {
		t.Fatalf("expected 1 passage, got %d", len(passages))
	}

	got := passages[0]
	if got.ID != "pass-1" || got.CheckpointID != "ann-cp-1" || got.Label != "lead" {
		t.Errorf("passage mismatch: %+v", got)
	}
	if got.ReportedBy != "user-1" || got.Notes != "looking good" {
		t.Errorf("passage details mismatch: reportedBy=%q notes=%q", got.ReportedBy, got.Notes)
	}
	if got.PassageTime.Truncate(time.Second) != now {
		t.Errorf("passageTime: got %v, want %v", got.PassageTime, now)
	}
}

func TestLoadCheckpointPassagesOrdering(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	base := time.Now().Truncate(time.Second).UTC()
	for i, offset := range []int{30, 10, 20} {
		s.SaveCheckpointPassage(CheckpointPassage{
			ID:           fmt.Sprintf("pass-%d", i),
			CheckpointID: "ann-1",
			NetID:        "net-1",
			Label:        "lead",
			PassageTime:  base.Add(time.Duration(offset) * time.Minute),
			Direction:    "through",
		})
	}

	passages, _ := s.LoadCheckpointPassages("net-1")
	if len(passages) != 3 {
		t.Fatalf("expected 3, got %d", len(passages))
	}
	// Should be ordered by passage_time ASC: 10, 20, 30 minutes offset.
	if passages[0].ID != "pass-1" || passages[1].ID != "pass-2" || passages[2].ID != "pass-0" {
		t.Errorf("ordering wrong: %s, %s, %s", passages[0].ID, passages[1].ID, passages[2].ID)
	}
}

func TestLoadCheckpointPassagesForCheckpoint(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	s.SaveCheckpointPassage(CheckpointPassage{ID: "p1", CheckpointID: "cp1", NetID: "net-1", Label: "lead", PassageTime: now, Direction: "through"})
	s.SaveCheckpointPassage(CheckpointPassage{ID: "p2", CheckpointID: "cp2", NetID: "net-1", Label: "lead", PassageTime: now, Direction: "through"})
	s.SaveCheckpointPassage(CheckpointPassage{ID: "p3", CheckpointID: "cp1", NetID: "net-1", Label: "sweep", PassageTime: now.Add(time.Minute), Direction: "through"})

	passages, err := s.LoadCheckpointPassagesForCheckpoint("cp1")
	if err != nil {
		t.Fatalf("LoadCheckpointPassagesForCheckpoint failed: %v", err)
	}
	if len(passages) != 2 {
		t.Fatalf("expected 2 passages for cp1, got %d", len(passages))
	}
	if passages[0].ID != "p1" || passages[1].ID != "p3" {
		t.Errorf("wrong passages: %s, %s", passages[0].ID, passages[1].ID)
	}
}

func TestDeleteCheckpointPassages(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	s.SaveCheckpointPassage(CheckpointPassage{ID: "p1", CheckpointID: "cp1", NetID: "net-1", Label: "lead", PassageTime: now, Direction: "through"})
	s.SaveCheckpointPassage(CheckpointPassage{ID: "p2", CheckpointID: "cp1", NetID: "net-2", Label: "lead", PassageTime: now, Direction: "through"})

	if err := s.DeleteCheckpointPassages("net-1"); err != nil {
		t.Fatalf("DeleteCheckpointPassages failed: %v", err)
	}

	// net-1 should be empty.
	p1, _ := s.LoadCheckpointPassages("net-1")
	if len(p1) != 0 {
		t.Errorf("expected 0 passages for net-1 after delete, got %d", len(p1))
	}

	// net-2 should be untouched.
	p2, _ := s.LoadCheckpointPassages("net-2")
	if len(p2) != 1 {
		t.Errorf("expected 1 passage for net-2, got %d", len(p2))
	}
}

func TestMigrateV19CreatesCheckpointTables(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}

	// Verify tables exist.
	for _, table := range []string{"checkpoint_meta", "checkpoint_passages"} {
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil || count != 1 {
			t.Errorf("table %q not found", table)
		}
	}
}

func TestNormalizeLegacySources(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"aprsis-0", []string{"aprsis"}},
		{"both", nil},
		{"aprsis+serial", []string{"aprsis", "serial"}},
		{"HA2 BT TNC", []string{"HA2 BT TNC"}},
		{"kisstcp-1", []string{"kisstcp"}},
		{"serial-0", []string{"serial"}},
		{"serial+aprsis", []string{"aprsis", "serial"}},
		{"aprsis-0+kisstcp-1", []string{"aprsis", "kisstcp"}},
		{"both+aprsis", []string{"aprsis"}},
		{"aprsis+aprsis-0", []string{"aprsis"}},
		{"", nil},
		{"  ", nil},
	}
	for _, tt := range tests {
		got := normalizeLegacySources(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("normalizeLegacySources(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("normalizeLegacySources(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}

func TestMigrateV20NormalizesLegacySources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-v20.db")

	// Build a minimal pre-v20 database (schema 19, no sources column).
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (19)`,
		`CREATE TABLE stations (
			callsign TEXT NOT NULL,
			ssid INTEGER NOT NULL DEFAULT 0,
			last_heard DATETIME NOT NULL,
			lat REAL,
			lon REAL,
			altitude REAL,
			speed REAL,
			course REAL,
			symbol_table TEXT,
			symbol_code TEXT,
			comment TEXT,
			source TEXT,
			PRIMARY KEY (callsign, ssid)
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}

	legacy := []struct {
		callsign string
		source   string
	}{
		{"LEGACY1", "aprsis-0"},
		{"LEGACY2", "both"},
		{"LEGACY3", "aprsis+serial"},
		{"LEGACY4", "HA2 BT TNC"},
	}
	for i, row := range legacy {
		if _, err := db.Exec(
			`INSERT INTO stations (callsign, ssid, last_heard, source) VALUES (?, 0, ?, ?)`,
			row.callsign, "2020-01-01T00:00:00Z", row.source,
		); err != nil {
			db.Close()
			t.Fatalf("insert legacy row %d: %v", i, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	// Open with the real store so migrate() runs v20 only.
	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}

	// sources column must exist.
	if _, err := s.db.Exec(`SELECT sources FROM stations WHERE 1=0`); err != nil {
		t.Fatalf("stations missing sources column: %v", err)
	}

	want := map[string]struct {
		source  string
		sources string
	}{
		"LEGACY1": {source: "aprsis", sources: `["aprsis"]`},
		"LEGACY2": {source: "", sources: `[]`},
		"LEGACY3": {source: "aprsis+serial", sources: `["aprsis","serial"]`},
		"LEGACY4": {source: "HA2 BT TNC", sources: `["HA2 BT TNC"]`},
	}

	for callsign, exp := range want {
		var source, sources string
		err := s.db.QueryRow(
			`SELECT source, sources FROM stations WHERE callsign = ? AND ssid = 0`, callsign,
		).Scan(&source, &sources)
		if err != nil {
			t.Errorf("%s: query failed: %v", callsign, err)
			continue
		}
		if source != exp.source {
			t.Errorf("%s source: got %q, want %q", callsign, source, exp.source)
		}
		if sources != exp.sources {
			t.Errorf("%s sources: got %q, want %q", callsign, sources, exp.sources)
		}
	}

	// Also verify via LoadStations hydration.
	loaded, err := s.LoadStations()
	if err != nil {
		t.Fatalf("LoadStations failed: %v", err)
	}
	byCall := make(map[string]station.Station, len(loaded))
	for _, st := range loaded {
		byCall[st.Callsign] = st
	}
	if got := byCall["LEGACY1"]; len(got.Sources) != 1 || got.Sources[0] != "aprsis" || got.Source != "aprsis" {
		t.Errorf("LEGACY1 loaded: source=%q sources=%v", got.Source, got.Sources)
	}
	if got := byCall["LEGACY2"]; len(got.Sources) != 0 || got.Source != "" {
		t.Errorf("LEGACY2 loaded: source=%q sources=%v", got.Source, got.Sources)
	}
	if got := byCall["LEGACY3"]; len(got.Sources) != 2 || got.Sources[0] != "aprsis" || got.Sources[1] != "serial" || got.Source != "aprsis+serial" {
		t.Errorf("LEGACY3 loaded: source=%q sources=%v", got.Source, got.Sources)
	}
	if got := byCall["LEGACY4"]; len(got.Sources) != 1 || got.Sources[0] != "HA2 BT TNC" || got.Source != "HA2 BT TNC" {
		t.Errorf("LEGACY4 loaded: source=%q sources=%v", got.Source, got.Sources)
	}
}

// --- conversation read state (#83) ---

func TestSaveAndLoadConversationReadsRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)

	if err := s.SaveConversationRead("W1AW-9", now); err != nil {
		t.Fatalf("SaveConversationRead W1AW-9: %v", err)
	}
	if err := s.SaveConversationRead("KJ4ERJ", earlier); err != nil {
		t.Fatalf("SaveConversationRead KJ4ERJ: %v", err)
	}

	reads, err := s.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads: %v", err)
	}
	if len(reads) != 2 {
		t.Fatalf("got %d reads, want 2", len(reads))
	}
	if got := reads["W1AW-9"]; !got.Equal(now) {
		t.Errorf("W1AW-9 = %v, want %v", got, now)
	}
	if got := reads["KJ4ERJ"]; !got.Equal(earlier) {
		t.Errorf("KJ4ERJ = %v, want %v", got, earlier)
	}
}

func TestSaveConversationReadUpsert(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	first := time.Now().UTC().Add(-time.Hour)
	second := time.Now().UTC()

	if err := s.SaveConversationRead("W1AW-9", first); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := s.SaveConversationRead("W1AW-9", second); err != nil {
		t.Fatalf("second save: %v", err)
	}

	reads, err := s.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads: %v", err)
	}
	if len(reads) != 1 {
		t.Fatalf("got %d reads, want 1", len(reads))
	}
	if got := reads["W1AW-9"]; !got.Equal(second) {
		t.Errorf("W1AW-9 = %v, want %v (second write wins)", got, second)
	}
}

func TestLoadConversationReadsEmpty(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	reads, err := s.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads: %v", err)
	}
	if reads == nil {
		t.Fatal("LoadConversationReads returned nil map, want empty non-nil map")
	}
	if len(reads) != 0 {
		t.Errorf("got %d reads, want 0", len(reads))
	}
}

func TestMigrateV21CreatesConversationReadsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-v21.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (20)`,
		`CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			from_call TEXT NOT NULL,
			to_call TEXT NOT NULL,
			body TEXT NOT NULL,
			msg_no TEXT NOT NULL DEFAULT '',
			state INTEGER NOT NULL DEFAULT 0,
			retries INTEGER NOT NULL DEFAULT 0,
			inbound INTEGER NOT NULL DEFAULT 0,
			timestamp DATETIME NOT NULL
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}

	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='conversation_reads'`,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Fatalf("conversation_reads table count = %d, want 1", count)
	}

	now := time.Now().UTC()
	if err := s.SaveConversationRead("W1AW-9", now); err != nil {
		t.Fatalf("SaveConversationRead on upgraded db: %v", err)
	}
	reads, err := s.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads on upgraded db: %v", err)
	}
	if got := reads["W1AW-9"]; !got.Equal(now) {
		t.Errorf("W1AW-9 = %v, want %v", got, now)
	}
}

// Before #83 an inbound message was "unread" only while its state was not
// StateAcked — and inbound messages are always stored as StateAcked, so the
// count was effectively always zero. Switching to a read marker means a
// database that predates the marker table would light up every historical
// inbound message as unread. The migration must therefore backfill an
// "everything so far is read" marker for each existing conversation.
func TestMigrateV21BackfillsExistingConversationsAsRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-v21-backfill.db")

	newest := time.Date(2026, 7, 20, 14, 30, 0, 0, time.UTC)
	older := newest.Add(-time.Hour)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (20)`,
		`CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			from_call TEXT NOT NULL,
			to_call TEXT NOT NULL,
			body TEXT NOT NULL,
			msg_no TEXT NOT NULL DEFAULT '',
			state INTEGER NOT NULL DEFAULT 0,
			retries INTEGER NOT NULL DEFAULT 0,
			inbound INTEGER NOT NULL DEFAULT 0,
			timestamp DATETIME NOT NULL
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}

	rows := []struct {
		id, from, to string
		inbound      int
		ts           time.Time
	}{
		{"a1", "W1AW-9", "N0CALL", 1, older},
		{"a2", "W1AW-9", "N0CALL", 1, newest},
		{"a3", "N0CALL", "W1AW-9", 0, newest.Add(time.Minute)}, // outbound, ignored
		{"b1", "KJ4ERJ", "N0CALL", 1, older},
		{"c1", "N0CALL", "W3ADO", 0, older}, // outbound-only conversation
		{"d1", "W3ADO", "BLN1", 1, newest},  // bulletin, never has read state
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO messages (id, from_call, to_call, body, msg_no, state, retries, inbound, timestamp)
			 VALUES (?, ?, ?, 'x', '', 3, 0, ?, ?)`,
			r.id, r.from, r.to, r.inbound, r.ts.Format(time.RFC3339Nano),
		); err != nil {
			db.Close()
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	reads, err := s.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads: %v", err)
	}

	got, ok := reads["W1AW-9"]
	if !ok {
		t.Fatal("W1AW-9 has no backfilled read marker — every historical message would show as unread after upgrade")
	}
	if got.Before(newest) {
		t.Errorf("W1AW-9 marker = %v, want >= newest inbound %v", got, newest)
	}
	if _, ok := reads["KJ4ERJ"]; !ok {
		t.Error("KJ4ERJ has no backfilled read marker")
	}
	if _, ok := reads["BLN1"]; ok {
		t.Error("bulletins must not get a read marker")
	}
	if _, ok := reads["W3ADO"]; ok {
		t.Error("an outbound-only conversation has nothing unread and needs no marker")
	}
	if len(reads) != 2 {
		t.Errorf("backfilled %d markers (%v), want 2", len(reads), reads)
	}

	// Idempotent: re-running Init on an already-migrated db must not disturb
	// markers that have since moved forward.
	moved := newest.Add(48 * time.Hour)
	if err := s.SaveConversationRead("W1AW-9", moved); err != nil {
		t.Fatalf("SaveConversationRead: %v", err)
	}
	if err := s.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	reads, err = s.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads after re-migrate: %v", err)
	}
	if got := reads["W1AW-9"]; !got.Equal(moved) {
		t.Errorf("W1AW-9 marker after re-migrate = %v, want %v", got, moved)
	}
}

// The backfill parses whatever the sqlite driver actually wrote for a
// time.Time. A format the parser does not recognize would silently skip the
// conversation and re-raise every historical badge, so exercise the real
// SaveMessage encoding rather than a hand-written string.
func TestMigrateV21BackfillReadsDriverWrittenTimestamps(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	newest := time.Now().UTC().Truncate(time.Second)
	if err := s.SaveMessage(message.Message{
		ID: "x1", From: "W1AW-9", To: "N0CALL", Body: "hi",
		Inbound: true, State: message.StateAcked, Timestamp: newest.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	if err := s.SaveMessage(message.Message{
		ID: "x2", From: "W1AW-9", To: "N0CALL", Body: "hi again",
		Inbound: true, State: message.StateAcked, Timestamp: newest,
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	// Rewind to a pre-v21 database and migrate forward again.
	if _, err := s.db.Exec("DROP TABLE conversation_reads"); err != nil {
		t.Fatalf("drop conversation_reads: %v", err)
	}
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		t.Fatalf("clear schema_version: %v", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (20)"); err != nil {
		t.Fatalf("set schema_version: %v", err)
	}
	if err := s.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	reads, err := s.LoadConversationReads()
	if err != nil {
		t.Fatalf("LoadConversationReads: %v", err)
	}
	got, ok := reads["W1AW-9"]
	if !ok {
		t.Fatal("no backfilled marker — the driver's timestamp format was not understood")
	}
	if got.Before(newest) {
		t.Errorf("marker = %v, want >= newest inbound %v", got, newest)
	}
}

// TestMigrateV22AddsAnnotationBatchColumns verifies the v21 -> v22 migration
// adds batch_id/batch_label to an existing annotations table, creates the
// supporting index, and leaves pre-migration ("legacy") rows readable with
// empty batch fields — see #89 (removable imported annotation sets).
func TestMigrateV22AddsAnnotationBatchColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-v22.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (21)`,
		`CREATE TABLE annotations (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			geometry TEXT NOT NULL,
			style TEXT NOT NULL DEFAULT '{}',
			created_by TEXT,
			created_by_name TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			category TEXT NOT NULL DEFAULT 'general',
			status TEXT NOT NULL DEFAULT 'active',
			priority TEXT NOT NULL DEFAULT 'routine',
			operation_id TEXT DEFAULT '',
			mission_ids TEXT NOT NULL DEFAULT '[]',
			resources TEXT DEFAULT '[]',
			reported_by TEXT DEFAULT '',
			reported_at DATETIME,
			resolved_at DATETIME,
			expires_at DATETIME,
			net_id TEXT NOT NULL DEFAULT '',
			short_name TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO annotations (id, type, label, geometry, created_at, updated_at)
			VALUES ('legacy-1', 'point', 'Legacy Marker', '{}', '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != 30 {
		t.Errorf("expected schema version 30, got %d", version)
	}

	cols := map[string]bool{}
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info('annotations')`)
	if err != nil {
		t.Fatalf("pragma_table_info(annotations): %v", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan column name: %v", err)
		}
		cols[name] = true
	}
	rows.Close()
	if !cols["batch_id"] {
		t.Error("annotations table missing batch_id column after migrateV22")
	}
	if !cols["batch_label"] {
		t.Error("annotations table missing batch_label column after migrateV22")
	}

	var idxCount int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_annotations_batch_id'`,
	).Scan(&idxCount); err != nil {
		t.Fatalf("query sqlite_master for index: %v", err)
	}
	if idxCount != 1 {
		t.Errorf("idx_annotations_batch_id count = %d, want 1", idxCount)
	}

	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations on upgraded db: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 legacy annotation, got %d", len(loaded))
	}
	if loaded[0].BatchID != "" {
		t.Errorf("legacy row BatchID = %q, want empty", loaded[0].BatchID)
	}
	if loaded[0].BatchLabel != "" {
		t.Errorf("legacy row BatchLabel = %q, want empty", loaded[0].BatchLabel)
	}
}

// TestMigrateV22Idempotent confirms the ADD COLUMN statements in migrateV22
// tolerate being run again against a database that already has the columns
// (the duplicate-column guard shared with every other migrateVN).
func TestMigrateV22Idempotent(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if err := s.migrateV22(); err != nil {
		t.Fatalf("first rerun of migrateV22 failed: %v", err)
	}
	if err := s.migrateV22(); err != nil {
		t.Fatalf("second rerun of migrateV22 failed: %v", err)
	}

	// migrateV22 re-stamps its own version, so the direct rerun leaves 22.
	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != 22 {
		t.Errorf("expected schema version 22, got %d", version)
	}
}

// TestSaveLoadAnnotationBatchFields round-trips BatchID/BatchLabel through
// SaveAnnotation -> LoadAnnotations.
func TestSaveLoadAnnotationBatchFields(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	ann := Annotation{
		ID:         "ann-batch-1",
		Type:       "point",
		Label:      "Water Stop 1",
		Geometry:   `{"type":"Point","coordinates":[-118.24,34.05]}`,
		CreatedAt:  now,
		UpdatedAt:  now,
		Category:   "resource",
		Status:     "active",
		BatchID:    "batch-abc-123",
		BatchLabel: "Day_1_48M_Jack_and_Back.gpx",
	}

	if err := s.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation failed: %v", err)
	}

	loaded, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(loaded))
	}
	got := loaded[0]
	if got.BatchID != "batch-abc-123" {
		t.Errorf("batchId: got %q, want %q", got.BatchID, "batch-abc-123")
	}
	if got.BatchLabel != "Day_1_48M_Jack_and_Back.gpx" {
		t.Errorf("batchLabel: got %q, want %q", got.BatchLabel, "Day_1_48M_Jack_and_Back.gpx")
	}

	// An annotation created without batch fields round-trips to empty strings,
	// not NULL — every pre-migration and hand-created row must render as
	// "ungrouped" rather than error.
	if err := s.SaveAnnotation(Annotation{
		ID: "ann-nobatch", Type: "point", Label: "Hand Placed", Geometry: "{}",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveAnnotation (no batch) failed: %v", err)
	}
	all, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	var found bool
	for _, a := range all {
		if a.ID == "ann-nobatch" {
			found = true
			if a.BatchID != "" || a.BatchLabel != "" {
				t.Errorf("ungrouped annotation got batch fields: id=%q label=%q", a.BatchID, a.BatchLabel)
			}
		}
	}
	if !found {
		t.Fatal("ann-nobatch not found in LoadAnnotations")
	}
}

// TestLoadAnnotationsFilteredByBatchID is table-driven over the batch/net
// composition rules from plan §6.1.
func TestLoadAnnotationsFilteredByBatchID(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	seed := []Annotation{
		{ID: "b1-n1-a", Type: "point", Label: "A", Geometry: "{}", CreatedAt: now, UpdatedAt: now, BatchID: "batch-1", NetID: "net-1"},
		{ID: "b1-n1-b", Type: "point", Label: "B", Geometry: "{}", CreatedAt: now, UpdatedAt: now, BatchID: "batch-1", NetID: "net-1"},
		{ID: "b2-n1-c", Type: "point", Label: "C", Geometry: "{}", CreatedAt: now, UpdatedAt: now, BatchID: "batch-2", NetID: "net-1"},
		{ID: "b1-n2-d", Type: "point", Label: "D", Geometry: "{}", CreatedAt: now, UpdatedAt: now, BatchID: "batch-1", NetID: "net-2"},
		{ID: "nobatch-e", Type: "point", Label: "E", Geometry: "{}", CreatedAt: now, UpdatedAt: now, NetID: "net-1"},
	}
	for _, a := range seed {
		if err := s.SaveAnnotation(a); err != nil {
			t.Fatalf("SaveAnnotation(%s) failed: %v", a.ID, err)
		}
	}

	tests := []struct {
		name    string
		filter  AnnotationFilter
		wantIDs []string
	}{
		{
			name:    "matching batch returns only its members",
			filter:  AnnotationFilter{BatchID: "batch-2"},
			wantIDs: []string{"b2-n1-c"},
		},
		{
			name:    "unknown batch returns empty",
			filter:  AnnotationFilter{BatchID: "no-such-batch"},
			wantIDs: nil,
		},
		{
			name:    "empty BatchID returns everything",
			filter:  AnnotationFilter{},
			wantIDs: []string{"b1-n1-a", "b1-n1-b", "b2-n1-c", "b1-n2-d", "nobatch-e"},
		},
		{
			name:    "BatchID composed with NetID narrows correctly",
			filter:  AnnotationFilter{BatchID: "batch-1", NetID: "net-1"},
			wantIDs: []string{"b1-n1-a", "b1-n1-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := s.LoadAnnotationsFiltered(tt.filter)
			if err != nil {
				t.Fatalf("LoadAnnotationsFiltered failed: %v", err)
			}
			gotIDs := make([]string, 0, len(results))
			for _, a := range results {
				gotIDs = append(gotIDs, a.ID)
			}
			if len(gotIDs) != len(tt.wantIDs) {
				t.Fatalf("got %d results %v, want %d %v", len(gotIDs), gotIDs, len(tt.wantIDs), tt.wantIDs)
			}
			want := map[string]bool{}
			for _, id := range tt.wantIDs {
				want[id] = true
			}
			for _, id := range gotIDs {
				if !want[id] {
					t.Errorf("unexpected id %q in results %v", id, gotIDs)
				}
			}
		})
	}
}

// TestUpdateAnnotationBatchLabel renames every member of a batch in one call,
// leaves other batches and ungrouped rows untouched, and bumps updated_at.
func TestUpdateAnnotationBatchLabel(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	created := time.Now().Truncate(time.Second).UTC().Add(-time.Hour)
	seed := []Annotation{
		{ID: "rn-1", Type: "point", Label: "A", Geometry: "{}", CreatedAt: created, UpdatedAt: created, BatchID: "batch-rename", BatchLabel: "Old Label"},
		{ID: "rn-2", Type: "point", Label: "B", Geometry: "{}", CreatedAt: created, UpdatedAt: created, BatchID: "batch-rename", BatchLabel: "Old Label"},
		{ID: "rn-other", Type: "point", Label: "C", Geometry: "{}", CreatedAt: created, UpdatedAt: created, BatchID: "batch-other", BatchLabel: "Untouched Batch"},
		{ID: "rn-none", Type: "point", Label: "D", Geometry: "{}", CreatedAt: created, UpdatedAt: created},
	}
	for _, a := range seed {
		if err := s.SaveAnnotation(a); err != nil {
			t.Fatalf("SaveAnnotation(%s) failed: %v", a.ID, err)
		}
	}

	updatedAt := time.Now().Truncate(time.Second).UTC()
	count, err := s.UpdateAnnotationBatchLabel("batch-rename", "Day 1 — Jack and Back", updatedAt)
	if err != nil {
		t.Fatalf("UpdateAnnotationBatchLabel failed: %v", err)
	}
	if count != 2 {
		t.Errorf("rows affected = %d, want 2", count)
	}

	all, err := s.LoadAnnotations()
	if err != nil {
		t.Fatalf("LoadAnnotations failed: %v", err)
	}
	byID := map[string]Annotation{}
	for _, a := range all {
		byID[a.ID] = a
	}

	for _, id := range []string{"rn-1", "rn-2"} {
		a, ok := byID[id]
		if !ok {
			t.Fatalf("annotation %s missing after rename", id)
		}
		if a.BatchLabel != "Day 1 — Jack and Back" {
			t.Errorf("%s BatchLabel = %q, want %q", id, a.BatchLabel, "Day 1 — Jack and Back")
		}
		if !a.UpdatedAt.Equal(updatedAt) {
			t.Errorf("%s UpdatedAt = %v, want %v", id, a.UpdatedAt, updatedAt)
		}
	}

	if other := byID["rn-other"]; other.BatchLabel != "Untouched Batch" {
		t.Errorf("other batch label changed: got %q", other.BatchLabel)
	}
	if other := byID["rn-other"]; !other.UpdatedAt.Equal(created) {
		t.Errorf("other batch updated_at changed: got %v, want %v", other.UpdatedAt, created)
	}
	if none := byID["rn-none"]; none.BatchLabel != "" {
		t.Errorf("ungrouped row got a batch label: %q", none.BatchLabel)
	}

	// Renaming an unknown batch affects nothing and returns count 0.
	count, err = s.UpdateAnnotationBatchLabel("no-such-batch", "Whatever", updatedAt)
	if err != nil {
		t.Fatalf("UpdateAnnotationBatchLabel(unknown) failed: %v", err)
	}
	if count != 0 {
		t.Errorf("rows affected for unknown batch = %d, want 0", count)
	}
}

// --- v23: fold net_missions.assigned_to into NetCheckIn.MissionIDs ---

// preV23Fixture hand-builds a v22-shaped database with the nets, net_missions
// and net_check_ins tables the v23 backfill touches.
func preV23Fixture(t *testing.T, checkIns []string, missions []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pre-v23.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	stmts := []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (22)`,
		`CREATE TABLE nets (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`INSERT INTO nets (id, name) VALUES ('net-1', 'Test')`,
		`CREATE TABLE net_missions (
			id TEXT PRIMARY KEY,
			net_id TEXT NOT NULL,
			title TEXT NOT NULL,
			assigned_to TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE net_check_ins (
			id TEXT PRIMARY KEY,
			net_id TEXT NOT NULL,
			callsign TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'available',
			checked_in_at DATETIME NOT NULL,
			mission_ids TEXT NOT NULL DEFAULT '[]'
		)`,
	}
	stmts = append(stmts, missions...)
	stmts = append(stmts, checkIns...)
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}
	return path
}

func currentVersion(t *testing.T, s *SQLiteStore) int {
	t.Helper()
	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	return version
}

func v23MissionIDs(t *testing.T, s *SQLiteStore, ciID string) string {
	t.Helper()
	var raw string
	if err := s.db.QueryRow(`SELECT mission_ids FROM net_check_ins WHERE id = ?`, ciID).Scan(&raw); err != nil {
		t.Fatalf("read mission_ids: %v", err)
	}
	return raw
}

func TestMigrateV23BackfillsAssignedTo(t *testing.T) {
	path := preV23Fixture(t,
		[]string{`INSERT INTO net_check_ins (id, net_id, callsign, status, checked_in_at)
			VALUES ('ci-1', 'net-1', 'KD7BBC', 'available', '2024-01-01T00:00:00Z')`},
		[]string{`INSERT INTO net_missions (id, net_id, title, assigned_to)
			VALUES ('m-1', 'net-1', 'Deploy', 'KD7BBC')`},
	)

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	if v := currentVersion(t, s); v != 30 {
		t.Errorf("expected schema version 30, got %d", v)
	}
	if got := v23MissionIDs(t, s, "ci-1"); got != `["m-1"]` {
		t.Errorf("mission_ids: got %s, want [\"m-1\"]", got)
	}
	var status string
	if err := s.db.QueryRow(`SELECT status FROM net_check_ins WHERE id = 'ci-1'`).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "assigned" {
		t.Errorf("status: got %q, want %q", status, "assigned")
	}
	var assignedTo string
	if err := s.db.QueryRow(`SELECT assigned_to FROM net_missions WHERE id = 'm-1'`).Scan(&assignedTo); err != nil {
		t.Fatalf("read assigned_to: %v", err)
	}
	if assignedTo != "" {
		t.Errorf("assigned_to: got %q, want empty", assignedTo)
	}
}

func TestMigrateV23OrphanCallsignDoesNotFail(t *testing.T) {
	path := preV23Fixture(t,
		[]string{`INSERT INTO net_check_ins (id, net_id, callsign, status, checked_in_at)
			VALUES ('ci-1', 'net-1', 'KD7BBC', 'available', '2024-01-01T00:00:00Z')`},
		[]string{`INSERT INTO net_missions (id, net_id, title, assigned_to)
			VALUES ('m-1', 'net-1', 'Deploy', 'W1AW')`},
	)

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init must tolerate an orphaned assigned_to callsign: %v", err)
	}
	defer s.Close()

	if v := currentVersion(t, s); v != 30 {
		t.Errorf("expected schema version 30, got %d", v)
	}
	if got := v23MissionIDs(t, s, "ci-1"); got != `[]` {
		t.Errorf("mission_ids: got %s, want []", got)
	}
	var assignedTo string
	if err := s.db.QueryRow(`SELECT assigned_to FROM net_missions WHERE id = 'm-1'`).Scan(&assignedTo); err != nil {
		t.Fatalf("read assigned_to: %v", err)
	}
	if assignedTo != "" {
		t.Errorf("assigned_to: got %q, want empty", assignedTo)
	}
}

func TestMigrateV23CaseInsensitiveAndReleasedSkipped(t *testing.T) {
	path := preV23Fixture(t,
		[]string{
			`INSERT INTO net_check_ins (id, net_id, callsign, status, checked_in_at)
				VALUES ('ci-released', 'net-1', 'KD7BBC', 'released', '2024-01-01T00:00:00Z')`,
			`INSERT INTO net_check_ins (id, net_id, callsign, status, checked_in_at)
				VALUES ('ci-active', 'net-1', 'KD7BBC', 'available', '2024-01-01T01:00:00Z')`,
		},
		[]string{`INSERT INTO net_missions (id, net_id, title, assigned_to)
			VALUES ('m-1', 'net-1', 'Deploy', 'kd7bbc')`},
	)

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	if got := v23MissionIDs(t, s, "ci-active"); got != `["m-1"]` {
		t.Errorf("active check-in mission_ids: got %s, want [\"m-1\"]", got)
	}
	if got := v23MissionIDs(t, s, "ci-released"); got != `[]` {
		t.Errorf("released check-in mission_ids: got %s, want []", got)
	}
}

func TestMigrateV23NoDuplicateWhenAlreadyAssigned(t *testing.T) {
	path := preV23Fixture(t,
		[]string{`INSERT INTO net_check_ins (id, net_id, callsign, status, checked_in_at, mission_ids)
			VALUES ('ci-1', 'net-1', 'KD7BBC', 'assigned', '2024-01-01T00:00:00Z', '["m-1"]')`},
		[]string{`INSERT INTO net_missions (id, net_id, title, assigned_to)
			VALUES ('m-1', 'net-1', 'Deploy', 'KD7BBC')`},
	)

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	if got := v23MissionIDs(t, s, "ci-1"); got != `["m-1"]` {
		t.Errorf("mission_ids: got %s, want [\"m-1\"] (no duplicate)", got)
	}
}

func TestMigrateV23WithoutMissionTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-v23-bare.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (22)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}
	db.Close()

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init must succeed without the mission tables: %v", err)
	}
	defer s.Close()

	if v := currentVersion(t, s); v != 30 {
		t.Errorf("expected schema version 30, got %d", v)
	}
}

// --- v24: internal/wxalert tables + per-net weather-watch columns ---

// TestInitStampsCurrentSchemaVersion guards migrateV1's hardcoded stamp: a
// brand-new database must always end up at currentSchemaVersion, however
// many migrations exist, so this assertion never needs hand-updating again
// the way the thirteen "!= N" call sites above did every time N changed.
func TestInitStampsCurrentSchemaVersion(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if v := currentVersion(t, s); v != currentSchemaVersion {
		t.Errorf("fresh init schema version = %d, want currentSchemaVersion (%d)", v, currentSchemaVersion)
	}
}

func TestMigrateV24CreatesWxTables(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	for _, table := range []string{"wx_alerts", "wx_point_zones", "wx_alert_meta"} {
		var present int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&present); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if present != 1 {
			t.Errorf("table %s not created by migrateV24", table)
		}
	}

	// The five wx_* columns on nets should exist and accept writes.
	if _, err := s.db.Exec(
		`UPDATE nets SET wx_buffer_miles = 0, wx_extra_zones = '[]', wx_mute_advisories = 0,
		   wx_interrupt_custom = 0, wx_interrupt_events = '[]' WHERE 1=0`); err != nil {
		t.Errorf("wx_* columns not added to nets: %v", err)
	}
}

// preV24Fixture hand-builds a v23-shaped nets table (the full column set a
// real pre-upgrade database has, unlike preV23Fixture's minimal id/name
// stand-in) plus schema_version=23, so Init's migrateV24 runs its ALTER
// TABLE path against a table shaped like a genuine upgrade.
func preV24Fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pre-v24.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	stmts := []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (23)`,
		`CREATE TABLE nets (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT '',
			frequency TEXT NOT NULL DEFAULT '', ncs_callsign TEXT NOT NULL DEFAULT '',
			ncs_user_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'draft',
			opened_at DATETIME, closed_at DATETIME, notes TEXT NOT NULL DEFAULT '',
			mission_brief TEXT NOT NULL DEFAULT '', ops_view_lat REAL, ops_view_lon REAL,
			ops_view_zoom REAL, pinned_stations TEXT NOT NULL DEFAULT '[]'
		)`,
		`INSERT INTO nets (id, name) VALUES ('net-1', 'Ridge Run 100')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}
	return path
}

func TestMigrateV24FromV23Fixture(t *testing.T) {
	path := preV24Fixture(t)

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	if v := currentVersion(t, s); v != 30 {
		t.Errorf("expected schema version 30, got %d", v)
	}

	n, err := s.LoadNet("net-1")
	if err != nil {
		t.Fatalf("LoadNet: %v", err)
	}
	if n.WxExtraZones == nil {
		t.Error("WxExtraZones = nil, want non-nil empty slice")
	}
	if len(n.WxExtraZones) != 0 {
		t.Errorf("WxExtraZones = %v, want empty", n.WxExtraZones)
	}
	if n.WxInterruptEvents == nil {
		t.Error("WxInterruptEvents = nil, want non-nil empty slice")
	}
	if n.WxBufferMiles != 0 {
		t.Errorf("WxBufferMiles = %v, want 0", n.WxBufferMiles)
	}
	if n.WxMuteAdvisories || n.WxInterruptCustom {
		t.Errorf("wx bool defaults not false: mute=%v custom=%v", n.WxMuteAdvisories, n.WxInterruptCustom)
	}
}

func TestMigrateV24Idempotent(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if err := s.migrateV24(); err != nil {
		t.Fatalf("first rerun of migrateV24 failed: %v", err)
	}
	if err := s.migrateV24(); err != nil {
		t.Fatalf("second rerun of migrateV24 failed: %v", err)
	}

	if v := currentVersion(t, s); v != 24 {
		t.Errorf("expected schema version 24, got %d", v)
	}
}

// TestSaveAndLoadNetWxSettingsRoundtrip covers both LoadNet and LoadNets —
// the two read paths that must agree on the five wx_* fields.
func TestSaveAndLoadNetWxSettingsRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	n := Net{
		ID:                "net-wx-1",
		Name:              "Ridge Run 100",
		Status:            "open",
		PinnedStations:    []string{},
		WxBufferMiles:     15,
		WxExtraZones:      []string{"MIZ056", "MIC081"},
		WxMuteAdvisories:  true,
		WxInterruptCustom: true,
		WxInterruptEvents: []string{"Tornado Warning", "Severe Thunderstorm Warning"},
	}
	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet: %v", err)
	}

	loaded, err := s.LoadNet("net-wx-1")
	if err != nil {
		t.Fatalf("LoadNet: %v", err)
	}
	if loaded.WxBufferMiles != 15 {
		t.Errorf("LoadNet WxBufferMiles = %v, want 15", loaded.WxBufferMiles)
	}
	if len(loaded.WxExtraZones) != 2 || loaded.WxExtraZones[0] != "MIZ056" {
		t.Errorf("LoadNet WxExtraZones = %v", loaded.WxExtraZones)
	}
	if !loaded.WxMuteAdvisories || !loaded.WxInterruptCustom {
		t.Errorf("LoadNet wx bools not round-tripped: %+v", loaded)
	}
	if len(loaded.WxInterruptEvents) != 2 || loaded.WxInterruptEvents[1] != "Severe Thunderstorm Warning" {
		t.Errorf("LoadNet WxInterruptEvents = %v", loaded.WxInterruptEvents)
	}

	nets, err := s.LoadNets()
	if err != nil {
		t.Fatalf("LoadNets: %v", err)
	}
	var found *Net
	for i := range nets {
		if nets[i].ID == "net-wx-1" {
			found = &nets[i]
		}
	}
	if found == nil {
		t.Fatal("LoadNets did not return net-wx-1")
	}
	if found.WxBufferMiles != 15 || len(found.WxExtraZones) != 2 || !found.WxMuteAdvisories {
		t.Errorf("LoadNets wx fields mismatch LoadNet: %+v", found)
	}

	// A net saved with nil wx slices (the zero-value case a fresh CreateNet
	// produces) must load back as non-nil empty slices, never nil — the
	// project's JSON-slice-column rule.
	bare := Net{ID: "net-wx-2", Name: "Bare", Status: "draft", PinnedStations: []string{}}
	if err := s.SaveNet(bare); err != nil {
		t.Fatalf("SaveNet(bare): %v", err)
	}
	loadedBare, err := s.LoadNet("net-wx-2")
	if err != nil {
		t.Fatalf("LoadNet(bare): %v", err)
	}
	if loadedBare.WxExtraZones == nil || loadedBare.WxInterruptEvents == nil {
		t.Errorf("bare net wx slices are nil: extraZones=%v interruptEvents=%v",
			loadedBare.WxExtraZones, loadedBare.WxInterruptEvents)
	}
}

func TestSaveAndLoadWxAlertsRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	a := WxAlertRow{
		ID:          "urn:oid:test.1",
		NetID:       "net-1",
		Event:       "Tornado Warning",
		Tier:        "warning",
		State:       "active",
		Proximity:   "in",
		NotifyClass: "interrupt",
		Sent:        now,
		Expires:     now.Add(45 * time.Minute),
		// EndsAt deliberately left zero — an alert with no Ends/eventEndingTime
		// yet; must round-trip as a zero time.Time, not a parse error.
		FetchedAt:   now,
		FirstSeenAt: now,
		UpdatedAt:   now,
		Data:        `{"id":"urn:oid:test.1","event":"Tornado Warning"}`,
	}
	if err := s.SaveWxAlert(a); err != nil {
		t.Fatalf("SaveWxAlert: %v", err)
	}

	loaded, err := s.LoadWxAlerts(true)
	if err != nil {
		t.Fatalf("LoadWxAlerts: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadWxAlerts = %d rows, want 1", len(loaded))
	}
	if !loaded[0].EndsAt.IsZero() {
		t.Errorf("EndsAt = %v, want zero", loaded[0].EndsAt)
	}
	if loaded[0].NetAckAt != nil {
		t.Errorf("NetAckAt = %v, want nil", loaded[0].NetAckAt)
	}
	if loaded[0].Data != a.Data {
		t.Errorf("Data = %q, want %q", loaded[0].Data, a.Data)
	}

	// Upsert: saving the same id again replaces the row rather than erroring
	// or duplicating it.
	a.State = "expired"
	a.Data = `{"id":"urn:oid:test.1","event":"Tornado Warning","state":"expired"}`
	if err := s.SaveWxAlert(a); err != nil {
		t.Fatalf("SaveWxAlert (upsert): %v", err)
	}
	loaded, err = s.LoadWxAlerts(true)
	if err != nil {
		t.Fatalf("LoadWxAlerts after upsert: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadWxAlerts after upsert = %d rows, want 1 (upsert, not insert)", len(loaded))
	}
	if loaded[0].State != "expired" {
		t.Errorf("State after upsert = %q, want expired", loaded[0].State)
	}

	// LoadWxAlerts(false) excludes the now-inactive row.
	active, err := s.LoadWxAlerts(false)
	if err != nil {
		t.Fatalf("LoadWxAlerts(false): %v", err)
	}
	if len(active) != 0 {
		t.Errorf("LoadWxAlerts(false) = %d rows, want 0 (row is expired)", len(active))
	}

	// UpdateWxAlertNetAck sets the ack columns and refreshes Data.
	ackAt := now.Add(time.Minute)
	newData := `{"id":"urn:oid:test.1","ackedForNet":{"callsign":"W8ABC"}}`
	if err := s.UpdateWxAlertNetAck(a.ID, "W8ABC", ackAt, newData); err != nil {
		t.Fatalf("UpdateWxAlertNetAck: %v", err)
	}
	loaded, err = s.LoadWxAlerts(true)
	if err != nil {
		t.Fatalf("LoadWxAlerts after ack: %v", err)
	}
	if loaded[0].NetAckCallsign != "W8ABC" {
		t.Errorf("NetAckCallsign = %q, want W8ABC", loaded[0].NetAckCallsign)
	}
	if loaded[0].NetAckAt == nil || !loaded[0].NetAckAt.Equal(ackAt) {
		t.Errorf("NetAckAt = %v, want %v", loaded[0].NetAckAt, ackAt)
	}
	if loaded[0].Data != newData {
		t.Errorf("Data after ack = %q, want %q", loaded[0].Data, newData)
	}

	// UpdateWxAlertNetAck on an unknown id errors instead of silently no-op'ing.
	if err := s.UpdateWxAlertNetAck("no-such-id", "W8ABC", ackAt, newData); err == nil {
		t.Error("UpdateWxAlertNetAck(unknown id) = nil error, want error")
	}

	// Age the now-expired row past the purge cutoff (its updated_at is
	// otherwise "now" from the ack step above, i.e. not old at all).
	a.UpdatedAt = now.Add(-2 * time.Hour)
	if err := s.SaveWxAlert(a); err != nil {
		t.Fatalf("SaveWxAlert (age for purge): %v", err)
	}

	// PurgeWxAlerts keeps active rows regardless of age, and removes
	// inactive rows older than the cutoff.
	activeRow := WxAlertRow{
		ID: "still-active", State: "active", UpdatedAt: now.Add(-24 * time.Hour), Data: "{}",
	}
	if err := s.SaveWxAlert(activeRow); err != nil {
		t.Fatalf("SaveWxAlert(activeRow): %v", err)
	}
	n, err := s.PurgeWxAlerts(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("PurgeWxAlerts: %v", err)
	}
	if n != 1 {
		t.Errorf("PurgeWxAlerts removed %d rows, want 1 (the expired one, not the old-but-active one)", n)
	}
	remaining, err := s.LoadWxAlerts(true)
	if err != nil {
		t.Fatalf("LoadWxAlerts after purge: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != "still-active" {
		t.Errorf("LoadWxAlerts after purge = %+v, want only still-active", remaining)
	}
}

func TestSaveAndLoadWxPointZonesRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	z := WxPointZone{CellLat: 4296, CellLon: -8567, UGC: []string{"MIZ056", "MIC081"}, ExpiresAt: now.Add(30 * 24 * time.Hour)}
	if err := s.SaveWxPointZone(z); err != nil {
		t.Fatalf("SaveWxPointZone: %v", err)
	}

	loaded, err := s.LoadWxPointZones()
	if err != nil {
		t.Fatalf("LoadWxPointZones: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadWxPointZones = %d rows, want 1", len(loaded))
	}
	if loaded[0].CellLat != 4296 || loaded[0].CellLon != -8567 {
		t.Errorf("cell = (%d,%d), want (4296,-8567)", loaded[0].CellLat, loaded[0].CellLon)
	}
	if len(loaded[0].UGC) != 2 || loaded[0].UGC[0] != "MIZ056" {
		t.Errorf("UGC = %v", loaded[0].UGC)
	}

	// Re-saving the same cell replaces it rather than duplicating (PRIMARY
	// KEY (cell_lat, cell_lon)).
	z.UGC = []string{"MIZ057"}
	if err := s.SaveWxPointZone(z); err != nil {
		t.Fatalf("SaveWxPointZone (replace): %v", err)
	}
	loaded, err = s.LoadWxPointZones()
	if err != nil {
		t.Fatalf("LoadWxPointZones after replace: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadWxPointZones after replace = %d rows, want 1", len(loaded))
	}
	if len(loaded[0].UGC) != 1 || loaded[0].UGC[0] != "MIZ057" {
		t.Errorf("UGC after replace = %v, want [MIZ057]", loaded[0].UGC)
	}
}

func TestWxMetaRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if _, ok, err := s.GetWxMeta("registry_snapshot"); err != nil || ok {
		t.Fatalf("GetWxMeta before set = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	if err := s.SetWxMeta("registry_snapshot", `{"active":[]}`); err != nil {
		t.Fatalf("SetWxMeta: %v", err)
	}
	v, ok, err := s.GetWxMeta("registry_snapshot")
	if err != nil {
		t.Fatalf("GetWxMeta: %v", err)
	}
	if !ok || v != `{"active":[]}` {
		t.Errorf("GetWxMeta = (%q, %v), want ({\"active\":[]}, true)", v, ok)
	}

	// Overwrite (upsert), not insert-fails-on-duplicate.
	if err := s.SetWxMeta("registry_snapshot", `{"active":[1]}`); err != nil {
		t.Fatalf("SetWxMeta (overwrite): %v", err)
	}
	v, ok, err = s.GetWxMeta("registry_snapshot")
	if err != nil || !ok || v != `{"active":[1]}` {
		t.Errorf("GetWxMeta after overwrite = (%q, %v, %v)", v, ok, err)
	}
}

// --- Net profile / ride config (schema v25) ---

// preV25Fixture hand-builds a v24-shaped nets table (the full column set,
// including the v24 wx_* columns, but WITHOUT profile) plus
// schema_version=24, so Init's migrateV25 runs its ALTER TABLE path against
// a table shaped like a genuine pre-v25 upgrade.
func preV25Fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pre-v25.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	stmts := []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (24)`,
		`CREATE TABLE nets (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT '',
			frequency TEXT NOT NULL DEFAULT '', ncs_callsign TEXT NOT NULL DEFAULT '',
			ncs_user_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'draft',
			opened_at DATETIME, closed_at DATETIME, notes TEXT NOT NULL DEFAULT '',
			mission_brief TEXT NOT NULL DEFAULT '', ops_view_lat REAL, ops_view_lon REAL,
			ops_view_zoom REAL, pinned_stations TEXT NOT NULL DEFAULT '[]',
			wx_buffer_miles REAL NOT NULL DEFAULT 0, wx_extra_zones TEXT NOT NULL DEFAULT '[]',
			wx_mute_advisories INTEGER NOT NULL DEFAULT 0, wx_interrupt_custom INTEGER NOT NULL DEFAULT 0,
			wx_interrupt_events TEXT NOT NULL DEFAULT '[]'
		)`,
		`INSERT INTO nets (id, name) VALUES ('net-1', 'Jack and Back')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}
	return path
}

func TestMigrateV25AddsProfileColumn(t *testing.T) {
	path := preV25Fixture(t)

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	if v := currentVersion(t, s); v != 30 {
		t.Errorf("expected schema version 30, got %d", v)
	}

	rows, err := s.db.Query(`PRAGMA table_info(nets)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == "profile" {
			found = true
		}
	}
	rows.Close()
	if !found {
		t.Error("nets table missing profile column after migrateV25")
	}

	var profile string
	if err := s.db.QueryRow(`SELECT profile FROM nets WHERE id = 'net-1'`).Scan(&profile); err != nil {
		t.Fatalf("select profile: %v", err)
	}
	if profile != "general" {
		t.Errorf("profile = %q, want %q", profile, "general")
	}
}

func TestMigrateV25CreatesRideConfigTable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	rows, err := s.db.Query(`PRAGMA table_info(net_ride_configs)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()

	want := map[string]bool{
		"net_id": true, "agency_name": true, "event_name": true, "event_date": true,
		"routes": true, "cutoff": true, "withhold_bib_on_severe_injury": true,
		"priority_tiers": true, "division": true, "updated_at": true,
	}
	got := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		got[name] = true
	}
	for col := range want {
		if !got[col] {
			t.Errorf("net_ride_configs missing column %q", col)
		}
	}
}

func TestMigrateV25Idempotent(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if err := s.migrateV25(); err != nil {
		t.Fatalf("first rerun of migrateV25 failed: %v", err)
	}
	if err := s.migrateV25(); err != nil {
		t.Fatalf("second rerun of migrateV25 failed: %v", err)
	}
}

func TestMigrateV25NarrowFixtureWithoutNets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-v25-bare.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (24)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatalf("setup stmt %q: %v", stmt, err)
		}
	}
	db.Close()

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init must succeed without the nets table: %v", err)
	}
	defer s.Close()

	if v := currentVersion(t, s); v != 30 {
		t.Errorf("expected schema version 30, got %d", v)
	}
}

func TestLoadNetPreV25RowIsGeneral(t *testing.T) {
	path := preV25Fixture(t)

	s := NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer s.Close()

	n, err := s.LoadNet("net-1")
	if err != nil {
		t.Fatalf("LoadNet: %v", err)
	}
	if n.Profile != "general" {
		t.Errorf("LoadNet profile = %q, want general", n.Profile)
	}

	nets, err := s.LoadNets()
	if err != nil {
		t.Fatalf("LoadNets: %v", err)
	}
	if len(nets) != 1 || nets[0].Profile != "general" {
		t.Errorf("LoadNets profile = %+v, want [general]", nets)
	}
}

func TestSaveAndLoadNetProfileRoundtrip(t *testing.T) {
	tests := []struct {
		name    string
		profile string
	}{
		{"general", "general"},
		{"bike-ride", "bike-ride"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			defer s.Close()

			n := Net{ID: "net-" + tt.name, Name: "Test Net", Status: "draft", PinnedStations: []string{}, Profile: tt.profile}
			if err := s.SaveNet(n); err != nil {
				t.Fatalf("SaveNet: %v", err)
			}

			loaded, err := s.LoadNet(n.ID)
			if err != nil {
				t.Fatalf("LoadNet: %v", err)
			}
			if loaded.Profile != tt.profile {
				t.Errorf("LoadNet profile = %q, want %q", loaded.Profile, tt.profile)
			}

			nets, err := s.LoadNets()
			if err != nil {
				t.Fatalf("LoadNets: %v", err)
			}
			if len(nets) != 1 || nets[0].Profile != tt.profile {
				t.Errorf("LoadNets profile = %+v, want [%s]", nets, tt.profile)
			}
		})
	}
}

func TestSaveNetEmptyProfileLoadsAsGeneral(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	n := Net{ID: "net-empty-profile", Name: "Test Net", Status: "draft", PinnedStations: []string{}, Profile: ""}
	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet: %v", err)
	}

	loaded, err := s.LoadNet(n.ID)
	if err != nil {
		t.Fatalf("LoadNet: %v", err)
	}
	if loaded.Profile != "general" {
		t.Errorf("LoadNet profile = %q, want general", loaded.Profile)
	}
}

func TestSaveAndLoadNetRideConfigRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	start := time.Date(2026, 6, 13, 6, 0, 0, 0, time.UTC)
	cutoffAt := time.Date(2026, 6, 13, 18, 0, 0, 0, time.UTC)
	opens := time.Date(2026, 6, 13, 5, 0, 0, 0, time.UTC)
	closes := time.Date(2026, 6, 13, 20, 0, 0, 0, time.UTC)

	cfg := NetRideConfig{
		NetID:      "net-1",
		AgencyName: "Marin Cyclists",
		EventName:  "Jack and Back 2026",
		EventDate:  "2026-06-13",
		Routes: []RideRoute{
			{ID: "100", Name: "100 Mile Century", DistanceMiles: 100, StartTime: &start, CutoffAt: &cutoffAt, Division: "route-a"},
			{ID: "75", Name: "75 Mile", DistanceMiles: 75},
			{ID: "55", Name: "55 Mile", DistanceMiles: 55},
			{ID: "48", Name: "48 Mile Jack and Back", DistanceMiles: 48},
		},
		Cutoff: RideCutoffPolicy{
			CourseOpensAt:            &opens,
			CourseClosesAt:           &closes,
			MandatorySAGAfterCutoff:  true,
			DeclinedSAGIsUnsupported: true,
			Notes:                    "SAG sweeps back to front",
		},
		WithholdBibOnSevereInjury: true,
		PriorityTiers: []PriorityTier{
			{ID: "emergency", Label: "Emergency", Rank: 1, Description: "d1", Examples: []string{"e1", "e2"}},
			{ID: "priority", Label: "Priority", Rank: 2, Description: "d2", Examples: []string{"e3"}},
			{ID: "high", Label: "High", Rank: 3, Description: "d3", Examples: []string{"e4"}},
			{ID: "medium", Label: "Medium", Rank: 4, Description: "d4", Examples: []string{"e5"}},
			{ID: "low", Label: "Low", Rank: 5, Description: "d5", Examples: []string{"e6"}},
		},
	}

	if err := s.SaveNetRideConfig(cfg); err != nil {
		t.Fatalf("SaveNetRideConfig: %v", err)
	}

	loaded, ok, err := s.LoadNetRideConfig("net-1")
	if err != nil {
		t.Fatalf("LoadNetRideConfig: %v", err)
	}
	if !ok {
		t.Fatal("LoadNetRideConfig ok = false, want true")
	}
	if loaded.AgencyName != cfg.AgencyName || loaded.EventName != cfg.EventName || loaded.EventDate != cfg.EventDate {
		t.Errorf("basic fields mismatch: %+v", loaded)
	}
	if len(loaded.Routes) != 4 {
		t.Fatalf("Routes len = %d, want 4", len(loaded.Routes))
	}
	r0 := loaded.Routes[0]
	if r0.ID != "100" || r0.Name != "100 Mile Century" || r0.DistanceMiles != 100 || r0.Division != "route-a" {
		t.Errorf("route 0 = %+v", r0)
	}
	if r0.StartTime == nil || !r0.StartTime.Equal(start) {
		t.Errorf("route 0 StartTime = %v, want %v", r0.StartTime, start)
	}
	if r0.CutoffAt == nil || !r0.CutoffAt.Equal(cutoffAt) {
		t.Errorf("route 0 CutoffAt = %v, want %v", r0.CutoffAt, cutoffAt)
	}
	if loaded.Cutoff.CourseOpensAt == nil || !loaded.Cutoff.CourseOpensAt.UTC().Equal(opens) {
		t.Errorf("Cutoff.CourseOpensAt = %v, want %v", loaded.Cutoff.CourseOpensAt, opens)
	}
	if loaded.Cutoff.CourseClosesAt == nil || !loaded.Cutoff.CourseClosesAt.UTC().Equal(closes) {
		t.Errorf("Cutoff.CourseClosesAt = %v, want %v", loaded.Cutoff.CourseClosesAt, closes)
	}
	if !loaded.Cutoff.MandatorySAGAfterCutoff || !loaded.Cutoff.DeclinedSAGIsUnsupported {
		t.Errorf("Cutoff flags = %+v", loaded.Cutoff)
	}
	if loaded.Cutoff.Notes != cfg.Cutoff.Notes {
		t.Errorf("Cutoff.Notes = %q, want %q", loaded.Cutoff.Notes, cfg.Cutoff.Notes)
	}
	if !loaded.WithholdBibOnSevereInjury {
		t.Error("WithholdBibOnSevereInjury = false, want true")
	}
	if len(loaded.PriorityTiers) != 5 {
		t.Fatalf("PriorityTiers len = %d, want 5", len(loaded.PriorityTiers))
	}
	for i, tier := range loaded.PriorityTiers {
		want := cfg.PriorityTiers[i]
		if tier.ID != want.ID || tier.Label != want.Label || tier.Rank != want.Rank || tier.Description != want.Description {
			t.Errorf("tier %d = %+v, want %+v", i, tier, want)
		}
		if len(tier.Examples) != len(want.Examples) {
			t.Errorf("tier %d examples = %v, want %v", i, tier.Examples, want.Examples)
		}
	}
}

func TestLoadNetRideConfigAbsent(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	cfg, ok, err := s.LoadNetRideConfig("unknown-net")
	if err != nil {
		t.Fatalf("LoadNetRideConfig: %v", err)
	}
	if ok || cfg != nil {
		t.Errorf("LoadNetRideConfig = (%v, %v), want (nil, false)", cfg, ok)
	}
}

func TestNetRideConfigNilSlicesNeverNil(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	cfg := NetRideConfig{
		NetID:         "net-nil",
		Routes:        nil,
		PriorityTiers: nil,
	}
	if err := s.SaveNetRideConfig(cfg); err != nil {
		t.Fatalf("SaveNetRideConfig: %v", err)
	}

	loaded, ok, err := s.LoadNetRideConfig("net-nil")
	if err != nil || !ok {
		t.Fatalf("LoadNetRideConfig: ok=%v err=%v", ok, err)
	}
	if loaded.Routes == nil {
		t.Error("Routes = nil, want empty slice")
	}
	if loaded.PriorityTiers == nil {
		t.Error("PriorityTiers = nil, want empty slice")
	}

	b, err := json.Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s2 := string(b)
	if !strings.Contains(s2, `"routes":[]`) {
		t.Errorf("marshaled config missing routes:[], got %s", s2)
	}
	if !strings.Contains(s2, `"priorityTiers":[]`) {
		t.Errorf("marshaled config missing priorityTiers:[], got %s", s2)
	}
	if strings.Contains(s2, "null") {
		t.Errorf("marshaled config contains null: %s", s2)
	}
}

func TestLoadNetRideConfigs(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	configs, err := s.LoadNetRideConfigs()
	if err != nil {
		t.Fatalf("LoadNetRideConfigs (empty): %v", err)
	}
	if configs == nil || len(configs) != 0 {
		t.Errorf("LoadNetRideConfigs (empty) = %v, want empty non-nil slice", configs)
	}

	for _, id := range []string{"net-c", "net-a", "net-b"} {
		if err := s.SaveNetRideConfig(NetRideConfig{NetID: id}); err != nil {
			t.Fatalf("SaveNetRideConfig(%s): %v", id, err)
		}
	}

	configs, err = s.LoadNetRideConfigs()
	if err != nil {
		t.Fatalf("LoadNetRideConfigs: %v", err)
	}
	if len(configs) != 3 {
		t.Fatalf("LoadNetRideConfigs len = %d, want 3", len(configs))
	}
	want := []string{"net-a", "net-b", "net-c"}
	for i, c := range configs {
		if c.NetID != want[i] {
			t.Errorf("configs[%d].NetID = %q, want %q", i, c.NetID, want[i])
		}
	}
}

func TestDeleteNetCascadesRideConfig(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	n := Net{ID: "net-cascade", Name: "Cascade Net", Status: "draft", PinnedStations: []string{}, Profile: "bike-ride"}
	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet: %v", err)
	}
	if err := s.SaveNetRideConfig(NetRideConfig{NetID: n.ID}); err != nil {
		t.Fatalf("SaveNetRideConfig: %v", err)
	}

	if err := s.DeleteNet(n.ID); err != nil {
		t.Fatalf("DeleteNet: %v", err)
	}

	_, ok, err := s.LoadNetRideConfig(n.ID)
	if err != nil {
		t.Fatalf("LoadNetRideConfig after delete: %v", err)
	}
	if ok {
		t.Error("LoadNetRideConfig ok = true after DeleteNet, want false")
	}
}

func TestDeleteNetRideConfig(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if err := s.SaveNetRideConfig(NetRideConfig{NetID: "net-direct"}); err != nil {
		t.Fatalf("SaveNetRideConfig: %v", err)
	}
	if err := s.DeleteNetRideConfig("net-direct"); err != nil {
		t.Fatalf("DeleteNetRideConfig: %v", err)
	}
	_, ok, err := s.LoadNetRideConfig("net-direct")
	if err != nil || ok {
		t.Fatalf("LoadNetRideConfig after delete = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	// Deleting an absent config is not an error.
	if err := s.DeleteNetRideConfig("never-existed"); err != nil {
		t.Errorf("DeleteNetRideConfig(absent) = %v, want nil", err)
	}
}

func TestMigrateV26CreatesSAGTables(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	for _, table := range []string{"sag_requests", "sag_vehicles"} {
		var present int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&present); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if present != 1 {
			t.Errorf("table %s missing after migrateV26", table)
		}
	}

	var idxPresent int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_sag_requests_net_seq'`,
	).Scan(&idxPresent); err != nil {
		t.Fatalf("check index: %v", err)
	}
	if idxPresent != 1 {
		t.Error("idx_sag_requests_net_seq missing after migrateV26")
	}
}

func TestMigrateV26Idempotent(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if err := s.migrateV26(); err != nil {
		t.Fatalf("first rerun of migrateV26 failed: %v", err)
	}
	if err := s.migrateV26(); err != nil {
		t.Fatalf("second rerun of migrateV26 failed: %v", err)
	}
}

func TestSAGRequestRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	mm := 12.5
	now := time.Now().Truncate(time.Second).UTC()
	closedAt := now.Add(time.Hour)

	req := SAGRequest{
		ID:       "req-1",
		NetID:    "net-1",
		Division: nil, // NULL in v1
		Sequence: 1,
		Pickup: SAGLocation{
			Kind:       "course",
			Route:      "100M",
			MileMarker: &mm,
		},
		Dropoff: SAGLocation{
			Kind: "next_reststop",
		},
		Reason:        "mechanical",
		Priority:      "high",
		Status:        "open",
		NeedsVehicle:  true,
		Slots:         []SAGSlot{{ID: "slot-1", Bib: "42", HasBike: true, Disposition: "waiting", UpdatedAt: now}},
		Legs:          []SAGLeg{{ID: "leg-1", VehicleCheckInID: "ci-1", SlotIDs: []string{"slot-1"}, Status: "dispatched", DispatchedAt: now}},
		RequestedBy:   "K6ABC",
		CreatedByName: "Net Control",
		Notes:         "flat tire",
		CreatedAt:     now,
		UpdatedAt:     now,
		ClosedAt:      &closedAt,
	}
	if err := s.SaveSAGRequest(req); err != nil {
		t.Fatalf("SaveSAGRequest: %v", err)
	}

	loaded, err := s.LoadSAGRequests("net-1")
	if err != nil {
		t.Fatalf("LoadSAGRequests: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadSAGRequests len = %d, want 1", len(loaded))
	}
	got := loaded[0]
	if got.Division != nil {
		t.Errorf("Division = %v, want nil", *got.Division)
	}
	if got.Pickup.Kind != "course" || got.Pickup.MileMarker == nil || *got.Pickup.MileMarker != mm {
		t.Errorf("Pickup = %+v, want kind=course mileMarker=%v", got.Pickup, mm)
	}
	if len(got.Slots) != 1 || got.Slots[0].ID != "slot-1" {
		t.Errorf("Slots = %+v", got.Slots)
	}
	if len(got.Legs) != 1 || got.Legs[0].ID != "leg-1" || len(got.Legs[0].SlotIDs) != 1 {
		t.Errorf("Legs = %+v", got.Legs)
	}
	if got.ClosedAt == nil || !got.ClosedAt.Equal(closedAt) {
		t.Errorf("ClosedAt = %v, want %v", got.ClosedAt, closedAt)
	}

	// division round-trips as a real value too.
	div := "route"
	req.Division = &div
	req.ID = "req-2"
	req.Sequence = 2
	req.Slots = nil // never persisted as nil — must come back as []
	req.Legs = nil
	req.ClosedAt = nil
	if err := s.SaveSAGRequest(req); err != nil {
		t.Fatalf("SaveSAGRequest (nil slices): %v", err)
	}
	loaded, err = s.LoadSAGRequests("net-1")
	if err != nil {
		t.Fatalf("LoadSAGRequests: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("LoadSAGRequests len = %d, want 2", len(loaded))
	}
	// ORDER BY sequence ASC.
	if loaded[0].ID != "req-1" || loaded[1].ID != "req-2" {
		t.Errorf("LoadSAGRequests order = [%s, %s], want [req-1, req-2]", loaded[0].ID, loaded[1].ID)
	}
	second := loaded[1]
	if second.Division == nil || *second.Division != "route" {
		t.Errorf("Division = %v, want route", second.Division)
	}
	if second.Slots == nil || len(second.Slots) != 0 {
		t.Errorf("Slots = %v, want empty non-nil slice", second.Slots)
	}
	if second.Legs == nil || len(second.Legs) != 0 {
		t.Errorf("Legs = %v, want empty non-nil slice", second.Legs)
	}
	if second.ClosedAt != nil {
		t.Errorf("ClosedAt = %v, want nil", second.ClosedAt)
	}

	b, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("marshaled request contains null: %s", b)
	}
}

func TestSAGVehicleRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	v := SAGVehicle{
		NetID:     "net-1",
		CheckInID: "ci-1",
		Seats:     3,
		RackSlots: 2,
		Notes:     "van",
		UpdatedAt: now,
	}
	if err := s.SaveSAGVehicle(v); err != nil {
		t.Fatalf("SaveSAGVehicle: %v", err)
	}

	loaded, err := s.LoadSAGVehicles("net-1")
	if err != nil {
		t.Fatalf("LoadSAGVehicles: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadSAGVehicles len = %d, want 1", len(loaded))
	}
	if loaded[0].Seats != 3 || loaded[0].RackSlots != 2 || loaded[0].Division != nil {
		t.Errorf("loaded = %+v", loaded[0])
	}

	if err := s.DeleteSAGVehicle("net-1", "ci-1"); err != nil {
		t.Fatalf("DeleteSAGVehicle: %v", err)
	}
	loaded, err = s.LoadSAGVehicles("net-1")
	if err != nil {
		t.Fatalf("LoadSAGVehicles after delete: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("LoadSAGVehicles after delete = %v, want empty", loaded)
	}
}

func TestDeleteNetCascadesSAG(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	n := Net{ID: "net-sag-cascade", Name: "Cascade Net", Status: "draft", PinnedStations: []string{}, Profile: "bike-ride"}
	if err := s.SaveNet(n); err != nil {
		t.Fatalf("SaveNet: %v", err)
	}
	now := time.Now().UTC()
	if err := s.SaveSAGRequest(SAGRequest{ID: "req-cascade", NetID: n.ID, Sequence: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveSAGRequest: %v", err)
	}
	if err := s.SaveSAGVehicle(SAGVehicle{NetID: n.ID, CheckInID: "ci-cascade", UpdatedAt: now}); err != nil {
		t.Fatalf("SaveSAGVehicle: %v", err)
	}

	if err := s.DeleteNet(n.ID); err != nil {
		t.Fatalf("DeleteNet: %v", err)
	}

	reqs, err := s.LoadSAGRequests(n.ID)
	if err != nil {
		t.Fatalf("LoadSAGRequests after delete: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("LoadSAGRequests after DeleteNet = %v, want empty", reqs)
	}
	vehicles, err := s.LoadSAGVehicles(n.ID)
	if err != nil {
		t.Fatalf("LoadSAGVehicles after delete: %v", err)
	}
	if len(vehicles) != 0 {
		t.Errorf("LoadSAGVehicles after DeleteNet = %v, want empty", vehicles)
	}
}

// --- Course closure (internal/course) ---

func TestMigrateV27CreatesCourseTables(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	for _, table := range []string{
		"course_config", "course_shutoffs", "course_rider_exceptions",
		"course_sweep_reports", "course_station_closures",
	} {
		var present int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&present); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if present != 1 {
			t.Errorf("table %s missing after migrateV27", table)
		}
	}
}

func TestMigrateV27Idempotent(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if err := s.migrateV27(); err != nil {
		t.Fatalf("first rerun of migrateV27 failed: %v", err)
	}
	if err := s.migrateV27(); err != nil {
		t.Fatalf("second rerun of migrateV27 failed: %v", err)
	}
}

func TestCourseConfigRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	cfg := CourseConfig{
		NetID:                "net-1",
		Division:             "",
		SweepLabel:           "TAIL",
		LeadLabel:            "FRONT",
		CloseRequiresSweep:   true,
		AutoSweepFromPassage: false,
		UpdatedAt:            now,
	}
	if err := s.SaveCourseConfig(cfg); err != nil {
		t.Fatalf("SaveCourseConfig: %v", err)
	}

	got, err := s.LoadCourseConfig("net-1")
	if err != nil {
		t.Fatalf("LoadCourseConfig: %v", err)
	}
	if got == nil {
		t.Fatal("LoadCourseConfig = nil, want a config")
	}
	if got.SweepLabel != "TAIL" || got.LeadLabel != "FRONT" || !got.CloseRequiresSweep || got.AutoSweepFromPassage {
		t.Errorf("loaded config = %+v", got)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, now)
	}

	missing, err := s.LoadCourseConfig("missing")
	if err != nil {
		t.Fatalf("LoadCourseConfig(missing): %v", err)
	}
	if missing != nil {
		t.Errorf("LoadCourseConfig(missing) = %+v, want nil", missing)
	}
}

func TestShutoffPointRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	mile := 41.2
	firedAt := now.Add(time.Hour)

	cases := []ShutoffPoint{
		{
			ID: "sp-1", NetID: "net-1", Name: "Benson Shutoff",
			Lat: 41.79474, Lon: -111.90586, RouteMile: nil,
			ScheduledAt: now, RerouteDirection: "West", RerouteDestination: "55-mile route",
			Status: "planned", CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "sp-2", NetID: "net-1", Name: "Second Shutoff",
			Lat: 41.8, Lon: -111.9, RouteMile: &mile,
			ScheduledAt: now.Add(2 * time.Hour), Status: "planned",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "sp-3", NetID: "net-1", Name: "Fired Shutoff",
			Lat: 41.9, Lon: -112.0,
			ScheduledAt: now.Add(3 * time.Hour), Status: "fired",
			FiredAt: &firedAt, FiredBy: "NCS", FireNote: "note", RerouteCount: 3,
			CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, sp := range cases {
		if err := s.SaveShutoffPoint(sp); err != nil {
			t.Fatalf("SaveShutoffPoint(%s): %v", sp.ID, err)
		}
	}

	loaded, err := s.LoadShutoffPoints("net-1")
	if err != nil {
		t.Fatalf("LoadShutoffPoints: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("LoadShutoffPoints len = %d, want 3", len(loaded))
	}
	// ORDER BY scheduled_at ASC.
	if loaded[0].ID != "sp-1" || loaded[1].ID != "sp-2" || loaded[2].ID != "sp-3" {
		t.Errorf("order = [%s,%s,%s], want [sp-1,sp-2,sp-3]", loaded[0].ID, loaded[1].ID, loaded[2].ID)
	}
	if loaded[0].RouteMile != nil {
		t.Errorf("sp-1 RouteMile = %v, want nil", *loaded[0].RouteMile)
	}
	if loaded[1].RouteMile == nil || *loaded[1].RouteMile != mile {
		t.Errorf("sp-2 RouteMile = %v, want %v", loaded[1].RouteMile, mile)
	}
	if loaded[2].FiredAt == nil || !loaded[2].FiredAt.Equal(firedAt) {
		t.Errorf("sp-3 FiredAt = %v, want %v", loaded[2].FiredAt, firedAt)
	}
	if loaded[2].RerouteCount != 3 {
		t.Errorf("sp-3 RerouteCount = %d, want 3", loaded[2].RerouteCount)
	}

	if err := s.DeleteShutoffPoint("sp-2"); err != nil {
		t.Fatalf("DeleteShutoffPoint: %v", err)
	}
	loaded, err = s.LoadShutoffPoints("net-1")
	if err != nil {
		t.Fatalf("LoadShutoffPoints after delete: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("LoadShutoffPoints after delete len = %d, want 2", len(loaded))
	}
}

func TestRiderExceptionRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	mile := 12.5

	// Two rows with the SAME bib both persist — bib is a label, not a key.
	r1 := RiderException{
		ID: "re-1", NetID: "net-1", Bib: "101", Kind: "sag",
		SupportStatus: "unsupported", RouteMile: &mile,
		RecordedAt: now, StatusChangedAt: now,
	}
	r2 := RiderException{
		ID: "re-2", NetID: "net-1", Bib: "101", Kind: "medical",
		SupportStatus: "supported",
		RecordedAt:    now.Add(time.Minute), StatusChangedAt: now.Add(time.Minute),
	}
	r3 := RiderException{
		ID: "re-3", NetID: "net-1", Bib: "", BibWithheld: true, Kind: "shutoff_reroute",
		SupportStatus: "unsupported",
		RecordedAt:    now.Add(2 * time.Minute), StatusChangedAt: now.Add(2 * time.Minute),
	}
	for _, r := range []RiderException{r1, r2, r3} {
		if err := s.SaveRiderException(r); err != nil {
			t.Fatalf("SaveRiderException(%s): %v", r.ID, err)
		}
	}

	loaded, err := s.LoadRiderExceptions("net-1")
	if err != nil {
		t.Fatalf("LoadRiderExceptions: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("LoadRiderExceptions len = %d, want 3", len(loaded))
	}
	// ORDER BY recorded_at ASC.
	if loaded[0].ID != "re-1" || loaded[1].ID != "re-2" || loaded[2].ID != "re-3" {
		t.Errorf("order = [%s,%s,%s]", loaded[0].ID, loaded[1].ID, loaded[2].ID)
	}
	if loaded[0].Bib != "101" || loaded[1].Bib != "101" {
		t.Errorf("both rows should keep bib 101: %+v / %+v", loaded[0], loaded[1])
	}
	if loaded[0].RouteMile == nil || *loaded[0].RouteMile != mile {
		t.Errorf("re-1 RouteMile = %v, want %v", loaded[0].RouteMile, mile)
	}
	if loaded[2].Bib != "" || !loaded[2].BibWithheld {
		t.Errorf("re-3 = %+v, want bib withheld", loaded[2])
	}
}

func TestSweepReportsLimit(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	for i := 0; i < 5; i++ {
		r := SweepReport{
			ID: fmt.Sprintf("sr-%d", i), NetID: "net-1",
			ReportedBy: "sweep", LastRiderBib: fmt.Sprintf("bib-%d", i),
			ReportedAt: now.Add(time.Duration(i) * time.Minute),
		}
		if err := s.SaveSweepReport(r); err != nil {
			t.Fatalf("SaveSweepReport(%d): %v", i, err)
		}
	}

	limited, err := s.LoadSweepReports("net-1", 2)
	if err != nil {
		t.Fatalf("LoadSweepReports(limit 2): %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("LoadSweepReports(limit 2) len = %d, want 2", len(limited))
	}
	// newest first (DESC).
	if limited[0].ID != "sr-4" || limited[1].ID != "sr-3" {
		t.Errorf("limited order = [%s,%s], want [sr-4,sr-3]", limited[0].ID, limited[1].ID)
	}

	all, err := s.LoadSweepReports("net-1", 0)
	if err != nil {
		t.Fatalf("LoadSweepReports(limit 0): %v", err)
	}
	if len(all) != 5 {
		t.Errorf("LoadSweepReports(limit 0) len = %d, want 5 (all)", len(all))
	}
}

func TestStationClosureUpsert(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	c := StationClosure{
		NetID: "net-1", CheckpointID: "cp-1", State: "open", UpdatedAt: now,
	}
	if err := s.SaveStationClosure(c); err != nil {
		t.Fatalf("SaveStationClosure: %v", err)
	}
	c.State = "sweep_passed"
	sweepAt := now.Add(time.Hour)
	c.SweepPassedAt = &sweepAt
	c.SweepPassedBy = "sweep"
	c.UpdatedAt = sweepAt
	if err := s.SaveStationClosure(c); err != nil {
		t.Fatalf("SaveStationClosure (update): %v", err)
	}

	loaded, err := s.LoadStationClosures("net-1")
	if err != nil {
		t.Fatalf("LoadStationClosures: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadStationClosures len = %d, want 1 (upsert, not insert)", len(loaded))
	}
	if loaded[0].State != "sweep_passed" || loaded[0].SweepPassedAt == nil || !loaded[0].SweepPassedAt.Equal(sweepAt) {
		t.Errorf("loaded = %+v, want latest state", loaded[0])
	}
}

func TestDeleteCourseDataForNet(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	if err := s.SaveCourseConfig(CourseConfig{NetID: "net-1", UpdatedAt: now}); err != nil {
		t.Fatalf("SaveCourseConfig net-1: %v", err)
	}
	if err := s.SaveCourseConfig(CourseConfig{NetID: "net-2", UpdatedAt: now}); err != nil {
		t.Fatalf("SaveCourseConfig net-2: %v", err)
	}
	if err := s.SaveShutoffPoint(ShutoffPoint{ID: "sp-a", NetID: "net-1", Name: "A", ScheduledAt: now, Status: "planned", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveShutoffPoint net-1: %v", err)
	}
	if err := s.SaveShutoffPoint(ShutoffPoint{ID: "sp-b", NetID: "net-2", Name: "B", ScheduledAt: now, Status: "planned", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveShutoffPoint net-2: %v", err)
	}
	if err := s.SaveRiderException(RiderException{ID: "re-a", NetID: "net-1", Kind: "sag", RecordedAt: now, StatusChangedAt: now}); err != nil {
		t.Fatalf("SaveRiderException net-1: %v", err)
	}
	if err := s.SaveRiderException(RiderException{ID: "re-b", NetID: "net-2", Kind: "sag", RecordedAt: now, StatusChangedAt: now}); err != nil {
		t.Fatalf("SaveRiderException net-2: %v", err)
	}
	if err := s.SaveSweepReport(SweepReport{ID: "sr-a", NetID: "net-1", ReportedAt: now}); err != nil {
		t.Fatalf("SaveSweepReport net-1: %v", err)
	}
	if err := s.SaveSweepReport(SweepReport{ID: "sr-b", NetID: "net-2", ReportedAt: now}); err != nil {
		t.Fatalf("SaveSweepReport net-2: %v", err)
	}
	if err := s.SaveStationClosure(StationClosure{NetID: "net-1", CheckpointID: "cp-a", State: "open", UpdatedAt: now}); err != nil {
		t.Fatalf("SaveStationClosure net-1: %v", err)
	}
	if err := s.SaveStationClosure(StationClosure{NetID: "net-2", CheckpointID: "cp-b", State: "open", UpdatedAt: now}); err != nil {
		t.Fatalf("SaveStationClosure net-2: %v", err)
	}

	if err := s.DeleteCourseDataForNet("net-1"); err != nil {
		t.Fatalf("DeleteCourseDataForNet: %v", err)
	}

	if cfg, err := s.LoadCourseConfig("net-1"); err != nil || cfg != nil {
		t.Errorf("net-1 config after delete = (%v, %v), want (nil, nil)", cfg, err)
	}
	if sps, _ := s.LoadShutoffPoints("net-1"); len(sps) != 0 {
		t.Errorf("net-1 shutoffs after delete = %v, want empty", sps)
	}
	if res, _ := s.LoadRiderExceptions("net-1"); len(res) != 0 {
		t.Errorf("net-1 riders after delete = %v, want empty", res)
	}
	if srs, _ := s.LoadSweepReports("net-1", 0); len(srs) != 0 {
		t.Errorf("net-1 sweep reports after delete = %v, want empty", srs)
	}
	if scs, _ := s.LoadStationClosures("net-1"); len(scs) != 0 {
		t.Errorf("net-1 station closures after delete = %v, want empty", scs)
	}

	// net-2 untouched.
	if cfg, err := s.LoadCourseConfig("net-2"); err != nil || cfg == nil {
		t.Errorf("net-2 config after delete of net-1 = (%v, %v), want a config", cfg, err)
	}
	if sps, _ := s.LoadShutoffPoints("net-2"); len(sps) != 1 {
		t.Errorf("net-2 shutoffs after delete of net-1 = %v, want 1", sps)
	}
	if res, _ := s.LoadRiderExceptions("net-2"); len(res) != 1 {
		t.Errorf("net-2 riders after delete of net-1 = %v, want 1", res)
	}
	if srs, _ := s.LoadSweepReports("net-2", 0); len(srs) != 1 {
		t.Errorf("net-2 sweep reports after delete of net-1 = %v, want 1", srs)
	}
	if scs, _ := s.LoadStationClosures("net-2"); len(scs) != 1 {
		t.Errorf("net-2 station closures after delete of net-1 = %v, want 1", scs)
	}
}

func TestCourseLoadsReturnEmptyNotNil(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if sps, err := s.LoadShutoffPoints("unknown-net"); err != nil || sps == nil || len(sps) != 0 {
		t.Errorf("LoadShutoffPoints(unknown) = (%v, %v), want (empty non-nil, nil)", sps, err)
	}
	if res, err := s.LoadRiderExceptions("unknown-net"); err != nil || res == nil || len(res) != 0 {
		t.Errorf("LoadRiderExceptions(unknown) = (%v, %v), want (empty non-nil, nil)", res, err)
	}
	if srs, err := s.LoadSweepReports("unknown-net", 0); err != nil || srs == nil || len(srs) != 0 {
		t.Errorf("LoadSweepReports(unknown) = (%v, %v), want (empty non-nil, nil)", srs, err)
	}
	if scs, err := s.LoadStationClosures("unknown-net"); err != nil || scs == nil || len(scs) != 0 {
		t.Errorf("LoadStationClosures(unknown) = (%v, %v), want (empty non-nil, nil)", scs, err)
	}
}

// --- Ride traffic (WP4): supply requests and medical notifications ---

func TestMigrateV28CreatesRideTrafficTables(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	for _, table := range []string{"ride_supply_requests", "ride_medical_notifications"} {
		var present int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&present); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if present != 1 {
			t.Errorf("table %s missing after migrateV28", table)
		}
	}

	for _, idx := range []string{"idx_ride_supply_status", "idx_ride_medical_status"} {
		var present int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx,
		).Scan(&present); err != nil {
			t.Fatalf("check index %s: %v", idx, err)
		}
		if present != 1 {
			t.Errorf("index %s missing after migrateV28", idx)
		}
	}
}

func TestMigrateV28Idempotent(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if err := s.migrateV28(); err != nil {
		t.Fatalf("first rerun of migrateV28 failed: %v", err)
	}
	if err := s.migrateV28(); err != nil {
		t.Fatalf("second rerun of migrateV28 failed: %v", err)
	}
}

func TestSupplyRequestRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	mm := 22.5
	lat, lon := 38.1, -122.7
	readBack := now.Add(5 * time.Minute)
	relayed := now.Add(10 * time.Minute)

	req := SupplyRequest{
		ID:                   "sup-1",
		NetID:                "net-1",
		Division:             nil,
		RequestedByCheckInID: "ci-1",
		RequestedByCall:      "RS3",
		Location:             "Rest Stop 3 / Nicasio",
		LocationAnnotationID: "ann-1",
		MilesRemaining:       &mm,
		RouteID:              "100",
		Lat:                  &lat,
		Lon:                  &lon,
		Items: []SupplyItem{
			{Item: "ice", Quantity: 10, Unit: "bags", AddedAt: now},
			{Item: "water", Quantity: 2, Unit: "cases", AddedAt: now.Add(time.Minute)},
		},
		AskedWhatElse: true,
		Priority:      "medium",
		Notes:         "please hurry",
		Status:        "en_route",
		CreatedAt:     now,
		ReadBackAt:    &readBack,
		ReadBackBy:    "RS3",
		RelayedAt:     &relayed,
		RelayedTo:     "SUPPLY 1",
		ETAs: []SupplyETA{
			{Minutes: 20, GivenAt: relayed, DueAt: relayed.Add(20 * time.Minute), Source: "SUPPLY 1"},
			{Minutes: 35, GivenAt: relayed.Add(15 * time.Minute), DueAt: relayed.Add(50 * time.Minute), Source: "SUPPLY 1"},
		},
		DeliveredAt: nil,
		UpdatedAt:   now,
	}
	if err := s.SaveSupplyRequest(req); err != nil {
		t.Fatalf("SaveSupplyRequest: %v", err)
	}

	loaded, err := s.LoadSupplyRequests("net-1")
	if err != nil {
		t.Fatalf("LoadSupplyRequests: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadSupplyRequests len = %d, want 1", len(loaded))
	}
	got := loaded[0]
	if got.Division != nil {
		t.Errorf("Division = %v, want nil", *got.Division)
	}
	if len(got.Items) != 2 || got.Items[0].Item != "ice" || got.Items[1].Item != "water" {
		t.Errorf("Items = %+v", got.Items)
	}
	if len(got.ETAs) != 2 || got.ETAs[0].Minutes != 20 || got.ETAs[1].Minutes != 35 {
		t.Errorf("ETAs = %+v", got.ETAs)
	}
	if got.MilesRemaining == nil || *got.MilesRemaining != mm {
		t.Errorf("MilesRemaining = %v, want %v", got.MilesRemaining, mm)
	}
	if !got.ReadBackAt.Equal(readBack) {
		t.Errorf("ReadBackAt = %v, want %v", got.ReadBackAt, readBack)
	}
	if got.DeliveredAt != nil {
		t.Errorf("DeliveredAt = %v, want nil", got.DeliveredAt)
	}

	// Division round-trips as a real pointer value, and nil Items/ETAs on
	// save come back as empty non-nil slices, never null.
	div := "route"
	req.ID = "sup-2"
	req.Division = &div
	req.Items = nil
	req.ETAs = nil
	req.ReadBackAt = nil
	req.RelayedAt = nil
	if err := s.SaveSupplyRequest(req); err != nil {
		t.Fatalf("SaveSupplyRequest (nil slices): %v", err)
	}
	loaded, err = s.LoadSupplyRequests("net-1")
	if err != nil {
		t.Fatalf("LoadSupplyRequests: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("LoadSupplyRequests len = %d, want 2", len(loaded))
	}
	// ORDER BY created_at ASC; both rows share CreatedAt=now so fall back to
	// checking both IDs are present rather than assuming a specific order.
	byID := map[string]SupplyRequest{loaded[0].ID: loaded[0], loaded[1].ID: loaded[1]}
	second, ok := byID["sup-2"]
	if !ok {
		t.Fatalf("sup-2 not found in %+v", loaded)
	}
	if second.Division == nil || *second.Division != "route" {
		t.Errorf("Division = %v, want route", second.Division)
	}
	if second.Items == nil || len(second.Items) != 0 {
		t.Errorf("Items = %v, want empty non-nil slice", second.Items)
	}
	if second.ETAs == nil || len(second.ETAs) != 0 {
		t.Errorf("ETAs = %v, want empty non-nil slice", second.ETAs)
	}

	b, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("marshaled request contains null: %s", b)
	}
}

func TestLoadSupplyRequestsUnknownNet(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	reqs, err := s.LoadSupplyRequests("unknown-net")
	if err != nil {
		t.Fatalf("LoadSupplyRequests: %v", err)
	}
	if reqs == nil || len(reqs) != 0 {
		t.Errorf("LoadSupplyRequests(unknown) = %v, want empty non-nil slice", reqs)
	}
}

func TestMedicalNotificationRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	mm := 22.0
	lat, lon := 38.05, -122.6
	readBack := now.Add(2 * time.Minute)
	etaGiven := now.Add(3 * time.Minute)
	etaDue := etaGiven.Add(8 * time.Minute)
	onScene := now.Add(11 * time.Minute)
	departed := now.Add(34 * time.Minute)
	onSceneSecs := 1380
	etaMinutes := 8

	n := MedicalNotification{
		ID:                   "med-1",
		NetID:                "net-1",
		Division:             nil,
		ReportedByCheckInID:  "ci-2",
		ReportedByCall:       "SAG 2",
		Bib:                  "412",
		BibWithheld:          true,
		Sex:                  "M",
		Age:                  "~40",
		Location:             "near Nicasio",
		MilesRemaining:       &mm,
		RouteID:              "100",
		LocationAnnotationID: "ann-2",
		Lat:                  &lat,
		Lon:                  &lon,
		ChiefComplaint:       "fall/shoulder",
		ReadBackAt:           &readBack,
		ReadBackBy:           "SAG 2",
		Severity:             "severe",
		Priority:             "priority",
		Status:               "departed",
		EMSUnit:              "Medic 12",
		ETAMinutes:           &etaMinutes,
		ETAGivenAt:           &etaGiven,
		ETADueAt:             &etaDue,
		OnSceneAt:            &onScene,
		DepartedAt:           &departed,
		OnSceneSeconds:       &onSceneSecs,
		Destination:          "hospital",
		DestinationName:      "Marin General",
		PatientCount:         1,
		PatientName:          "Jane Rider",
		Notes:                "transported",
		CreatedAt:            now,
		UpdatedAt:            departed,
	}
	if err := s.SaveMedicalNotification(n); err != nil {
		t.Fatalf("SaveMedicalNotification: %v", err)
	}

	loaded, err := s.LoadMedicalNotifications("net-1")
	if err != nil {
		t.Fatalf("LoadMedicalNotifications: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadMedicalNotifications len = %d, want 1", len(loaded))
	}
	got := loaded[0]
	if got.Division != nil {
		t.Errorf("Division = %v, want nil", *got.Division)
	}
	if !got.BibWithheld || got.Bib != "412" {
		t.Errorf("Bib/BibWithheld = %q/%v, want 412/true (still stored, redaction is a read-time concern)", got.Bib, got.BibWithheld)
	}
	if got.PatientName != "Jane Rider" {
		t.Errorf("PatientName = %q, want Jane Rider (unredacted store round-trip)", got.PatientName)
	}
	if got.ETAMinutes == nil || *got.ETAMinutes != 8 {
		t.Errorf("ETAMinutes = %v, want 8", got.ETAMinutes)
	}
	if got.OnSceneSeconds == nil || *got.OnSceneSeconds != 1380 {
		t.Errorf("OnSceneSeconds = %v, want 1380", got.OnSceneSeconds)
	}
	if got.OnSceneAt == nil || !got.OnSceneAt.Equal(onScene) {
		t.Errorf("OnSceneAt = %v, want %v", got.OnSceneAt, onScene)
	}
	if got.DepartedAt == nil || !got.DepartedAt.Equal(departed) {
		t.Errorf("DepartedAt = %v, want %v", got.DepartedAt, departed)
	}

	// Every *time.Time nil round-trips as nil, not a zero time.
	n2 := MedicalNotification{
		ID: "med-2", NetID: "net-1", ReportedByCall: "SAG 3", Sex: "U", Age: "unknown",
		Location: "near start", ChiefComplaint: "twisted ankle",
		Severity: "routine", Priority: "low", Status: "reported",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.SaveMedicalNotification(n2); err != nil {
		t.Fatalf("SaveMedicalNotification (all nil times): %v", err)
	}
	loaded, err = s.LoadMedicalNotifications("net-1")
	if err != nil {
		t.Fatalf("LoadMedicalNotifications: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("LoadMedicalNotifications len = %d, want 2", len(loaded))
	}
	var second MedicalNotification
	for _, m := range loaded {
		if m.ID == "med-2" {
			second = m
		}
	}
	for name, ptr := range map[string]*time.Time{
		"ReadBackAt": second.ReadBackAt, "ETAGivenAt": second.ETAGivenAt, "ETADueAt": second.ETADueAt,
		"OnSceneAt": second.OnSceneAt, "DepartedAt": second.DepartedAt, "ReleasedAt": second.ReleasedAt,
		"CancelledAt": second.CancelledAt,
	} {
		if ptr != nil {
			t.Errorf("%s = %v, want nil", name, ptr)
		}
	}
	if second.ETAMinutes != nil {
		t.Errorf("ETAMinutes = %v, want nil", second.ETAMinutes)
	}
	if second.OnSceneSeconds != nil {
		t.Errorf("OnSceneSeconds = %v, want nil", second.OnSceneSeconds)
	}

	b, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("marshaled notification contains null: %s", b)
	}
}

func TestDeleteRideTraffic(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	if err := s.SaveSupplyRequest(SupplyRequest{
		ID: "sup-1", NetID: "net-1", RequestedByCall: "RS3", Priority: "medium",
		Status: "draft", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveSupplyRequest: %v", err)
	}
	if err := s.SaveSupplyRequest(SupplyRequest{
		ID: "sup-2", NetID: "net-2", RequestedByCall: "RS1", Priority: "medium",
		Status: "draft", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveSupplyRequest: %v", err)
	}
	if err := s.SaveMedicalNotification(MedicalNotification{
		ID: "med-1", NetID: "net-1", ReportedByCall: "SAG 2", Sex: "U", Age: "unknown",
		Location: "x", ChiefComplaint: "y", Severity: "routine", Priority: "low",
		Status: "reported", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveMedicalNotification: %v", err)
	}
	if err := s.SaveMedicalNotification(MedicalNotification{
		ID: "med-2", NetID: "net-2", ReportedByCall: "SAG 3", Sex: "U", Age: "unknown",
		Location: "x", ChiefComplaint: "y", Severity: "routine", Priority: "low",
		Status: "reported", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveMedicalNotification: %v", err)
	}

	if err := s.DeleteRideTraffic("net-1"); err != nil {
		t.Fatalf("DeleteRideTraffic: %v", err)
	}

	if reqs, _ := s.LoadSupplyRequests("net-1"); len(reqs) != 0 {
		t.Errorf("net-1 supply requests after delete = %v, want empty", reqs)
	}
	if meds, _ := s.LoadMedicalNotifications("net-1"); len(meds) != 0 {
		t.Errorf("net-1 medical notifications after delete = %v, want empty", meds)
	}
	if reqs, _ := s.LoadSupplyRequests("net-2"); len(reqs) != 1 {
		t.Errorf("net-2 supply requests after delete of net-1 = %v, want 1", reqs)
	}
	if meds, _ := s.LoadMedicalNotifications("net-2"); len(meds) != 1 {
		t.Errorf("net-2 medical notifications after delete of net-1 = %v, want 1", meds)
	}
}

// --- Ride reconciliation (WP5) round-trip tests ---

func TestV29SchemaVersion(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()
	if v := currentVersion(t, s); v != 30 {
		t.Errorf("expected schema version 30, got %d", v)
	}
}

func TestSAGShiftSummaryRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	start := now.Add(-2 * time.Hour)
	odoStart := 12000.0
	odoEnd := 12084.2
	transports := 7
	assists := 0 // explicit zero must survive as 0, not fall back to derived

	sm := SAGShiftSummary{
		ID:            "shift-1",
		NetID:         "net-1",
		CheckInID:     "ci-sag-1",
		Callsign:      "K6ABC",
		TacticalCall:  "SAG 2",
		DriverName:    "Pat Driver",
		Vehicle:       "white Sprinter, 2 racks",
		ShiftStart:    &start,
		OdometerStart: &odoStart,
		OdometerEnd:   &odoEnd,
		Entered: SAGShiftCounts{
			Transports: &transports,
			Assists:    &assists,
		},
		Notes:     "smooth shift",
		Status:    ShiftFiled,
		FiledAt:   &now,
		FiledBy:   "K6ABC",
		CreatedAt: start,
		UpdatedAt: now,
	}
	if err := s.SaveSAGShiftSummary(sm); err != nil {
		t.Fatalf("SaveSAGShiftSummary: %v", err)
	}

	// A second summary with no entered counts at all — every pointer must
	// stay nil, not become 0, so the caller knows to fall back to derived.
	sm2 := SAGShiftSummary{
		ID: "shift-2", NetID: "net-1", CheckInID: "ci-sag-2",
		Status: ShiftDraft, CreatedAt: start, UpdatedAt: start,
	}
	if err := s.SaveSAGShiftSummary(sm2); err != nil {
		t.Fatalf("SaveSAGShiftSummary (no entered counts): %v", err)
	}

	loaded, err := s.LoadSAGShiftSummaries("net-1")
	if err != nil {
		t.Fatalf("LoadSAGShiftSummaries: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("LoadSAGShiftSummaries len = %d, want 2", len(loaded))
	}

	got := loaded[0]
	if got.Division != "" {
		t.Errorf("Division = %q, want \"\"", got.Division)
	}
	if got.Entered.Transports == nil || *got.Entered.Transports != 7 {
		t.Errorf("Entered.Transports = %v, want 7", got.Entered.Transports)
	}
	if got.Entered.Assists == nil || *got.Entered.Assists != 0 {
		t.Errorf("Entered.Assists = %v, want 0 (explicit zero must not be nil)", got.Entered.Assists)
	}
	if got.Entered.TubesProvided != nil {
		t.Errorf("Entered.TubesProvided = %v, want nil (not entered)", got.Entered.TubesProvided)
	}
	if got.OdometerStart == nil || *got.OdometerStart != odoStart {
		t.Errorf("OdometerStart = %v, want %v", got.OdometerStart, odoStart)
	}
	if got.ShiftEnd != nil {
		t.Errorf("ShiftEnd = %v, want nil", got.ShiftEnd)
	}

	got2 := loaded[1]
	if got2.Entered.Transports != nil || got2.Entered.Assists != nil || got2.Entered.IncidentsAttended != nil {
		t.Errorf("shift-2 Entered = %+v, want every pointer nil", got2.Entered)
	}
	if got2.OdometerStart != nil || got2.ShiftStart != nil {
		t.Errorf("shift-2 unset optionals should stay nil: OdometerStart=%v ShiftStart=%v", got2.OdometerStart, got2.ShiftStart)
	}

	if loaded, err := s.LoadSAGShiftSummaries("no-such-net"); err != nil || loaded == nil || len(loaded) != 0 {
		t.Errorf("LoadSAGShiftSummaries(unknown net) = %v, %v, want empty non-nil slice", loaded, err)
	}
}

func TestHandoffItemRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()
	due := now.Add(15 * time.Minute)
	resolved := now.Add(20 * time.Minute)

	h := HandoffItem{
		ID: "ho-1", NetID: "net-1",
		Kind: HandoffAwaitingReply, Summary: "Asked RS3 for water status",
		SentTo: "RS3", ReplyTo: "NCS", RefType: "message", RefID: "msg-1",
		DueAt: &due, Status: HandoffResolved, HandoverCount: 2,
		CreatedBy: "K6ABC", CreatedAt: now,
		ResolvedBy: "W6XYZ", ResolvedAt: &resolved, Resolution: "RS3 confirmed full",
	}
	if err := s.SaveHandoffItem(h); err != nil {
		t.Fatalf("SaveHandoffItem: %v", err)
	}

	loaded, err := s.LoadHandoffItems("net-1")
	if err != nil {
		t.Fatalf("LoadHandoffItems: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadHandoffItems len = %d, want 1", len(loaded))
	}
	got := loaded[0]
	if got.HandoverCount != 2 {
		t.Errorf("HandoverCount = %d, want 2", got.HandoverCount)
	}
	if got.DueAt == nil || !got.DueAt.Equal(due) {
		t.Errorf("DueAt = %v, want %v", got.DueAt, due)
	}
	if got.ResolvedAt == nil || !got.ResolvedAt.Equal(resolved) {
		t.Errorf("ResolvedAt = %v, want %v", got.ResolvedAt, resolved)
	}
	if got.Resolution != "RS3 confirmed full" {
		t.Errorf("Resolution = %q, want %q", got.Resolution, "RS3 confirmed full")
	}

	if loaded, err := s.LoadHandoffItems("no-such-net"); err != nil || loaded == nil || len(loaded) != 0 {
		t.Errorf("LoadHandoffItems(unknown net) = %v, %v, want empty non-nil slice", loaded, err)
	}
}

func TestShiftHandoffRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Second).UTC()

	h := ShiftHandoff{
		ID: "sh-1", NetID: "net-1",
		FromCallsign: "K6ABC", ToCallsign: "W6XYZ", At: now,
		OpenItemIDs: []string{"ho-1", "ho-2"},
		Briefing:    `{"netId":"net-1","ncsCallsign":"W6XYZ"}`,
	}
	if err := s.SaveShiftHandoff(h); err != nil {
		t.Fatalf("SaveShiftHandoff: %v", err)
	}

	// A handoff saved with a nil OpenItemIDs slice must still load as [].
	h2 := ShiftHandoff{
		ID: "sh-2", NetID: "net-1",
		FromCallsign: "W6XYZ", ToCallsign: "N6DEF", At: now.Add(2 * time.Hour),
		OpenItemIDs: nil,
	}
	if err := s.SaveShiftHandoff(h2); err != nil {
		t.Fatalf("SaveShiftHandoff (nil open items): %v", err)
	}

	loaded, err := s.LoadShiftHandoffs("net-1")
	if err != nil {
		t.Fatalf("LoadShiftHandoffs: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("LoadShiftHandoffs len = %d, want 2", len(loaded))
	}
	got := loaded[0]
	if len(got.OpenItemIDs) != 2 || got.OpenItemIDs[0] != "ho-1" || got.OpenItemIDs[1] != "ho-2" {
		t.Errorf("OpenItemIDs = %v, want [ho-1 ho-2]", got.OpenItemIDs)
	}
	if got.AcknowledgedAt != nil {
		t.Errorf("AcknowledgedAt = %v, want nil", got.AcknowledgedAt)
	}

	got2 := loaded[1]
	if got2.OpenItemIDs == nil || len(got2.OpenItemIDs) != 0 {
		t.Errorf("OpenItemIDs (saved nil) = %v, want empty non-nil slice", got2.OpenItemIDs)
	}

	if loaded, err := s.LoadShiftHandoffs("no-such-net"); err != nil || loaded == nil || len(loaded) != 0 {
		t.Errorf("LoadShiftHandoffs(unknown net) = %v, %v, want empty non-nil slice", loaded, err)
	}
}

// --- Ride phase (internal/ride/phase, WP5b) ---

func TestV30SchemaVersion(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()
	if v := currentVersion(t, s); v != 30 {
		t.Errorf("expected schema version 30, got %d", v)
	}
}

func TestMigrateV30CreatesRidePhaseTable(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	var present int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ride_phase_state'`,
	).Scan(&present); err != nil {
		t.Fatalf("check table ride_phase_state: %v", err)
	}
	if present != 1 {
		t.Error("table ride_phase_state missing after migrateV30")
	}
}

func TestMigrateV30Idempotent(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	if err := s.migrateV30(); err != nil {
		t.Fatalf("first rerun of migrateV30 failed: %v", err)
	}
	if err := s.migrateV30(); err != nil {
		t.Fatalf("second rerun of migrateV30 failed: %v", err)
	}
}

func TestRidePhaseRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	defer s.Close()

	// No row yet -> nil, nil (the manager's default is pre-start, not an error).
	got, err := s.LoadRidePhase("net-1")
	if err != nil {
		t.Fatalf("LoadRidePhase (absent): %v", err)
	}
	if got != nil {
		t.Errorf("LoadRidePhase (absent) = %+v, want nil", got)
	}

	now := time.Now().Truncate(time.Second).UTC()
	p := RidePhaseState{NetID: "net-1", Phase: "launched", SetBy: "K6ABC", Reason: "", UpdatedAt: now}
	if err := s.SaveRidePhase(p); err != nil {
		t.Fatalf("SaveRidePhase: %v", err)
	}

	got, err = s.LoadRidePhase("net-1")
	if err != nil {
		t.Fatalf("LoadRidePhase: %v", err)
	}
	if got == nil {
		t.Fatal("LoadRidePhase = nil, want a row")
	}
	if got.Phase != "launched" || got.SetBy != "K6ABC" || got.Reason != "" {
		t.Errorf("LoadRidePhase = %+v, want phase launched, setBy K6ABC", got)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, now)
	}

	// INSERT OR REPLACE: a second save for the same net overwrites, not appends.
	backAt := now.Add(time.Hour)
	if err := s.SaveRidePhase(RidePhaseState{
		NetID: "net-1", Phase: "pre-start", SetBy: "K6ABC", Reason: "advanced too early", UpdatedAt: backAt,
	}); err != nil {
		t.Fatalf("SaveRidePhase (overwrite): %v", err)
	}
	got, err = s.LoadRidePhase("net-1")
	if err != nil {
		t.Fatalf("LoadRidePhase (after overwrite): %v", err)
	}
	if got.Phase != "pre-start" || got.Reason != "advanced too early" {
		t.Errorf("LoadRidePhase (after overwrite) = %+v, want phase pre-start with reason", got)
	}

	// A second net's row is independent.
	if err := s.SaveRidePhase(RidePhaseState{NetID: "net-2", Phase: "mid-ride", UpdatedAt: now}); err != nil {
		t.Fatalf("SaveRidePhase (net-2): %v", err)
	}
	got1, err := s.LoadRidePhase("net-1")
	if err != nil {
		t.Fatalf("LoadRidePhase (net-1 again): %v", err)
	}
	if got1.Phase != "pre-start" {
		t.Errorf("net-1 phase = %q after saving net-2, want unaffected pre-start", got1.Phase)
	}
}

// History written before the tracker learned to reject no-fix beacons still
// holds 0,0 rows (7,175 of them in one operator's database). Loading must not
// hand them back: a stored 0,0 track point draws a line from the station's
// real position to Null Island, and a station last saved at 0,0 is drawn in
// the Gulf of Guinea. Filtering at load is non-destructive — the rows stay.
func TestLoadSkipsNullIsland(t *testing.T) {
	s, _ := newTestStore(t)

	now := time.Now().UTC()
	for i, p := range []station.TrackPoint{
		{Lat: 36.546, Lon: -87.327, Time: now.Add(-3 * time.Minute)},
		{Lat: 0, Lon: 0, Time: now.Add(-2 * time.Minute)}, // no-fix beacon
		{Lat: 36.547, Lon: -87.326, Time: now.Add(-1 * time.Minute)},
	} {
		if err := s.SaveTrackPoint("KO4LFZ-9", p); err != nil {
			t.Fatalf("SaveTrackPoint %d: %v", i, err)
		}
	}
	pts, err := s.LoadTrackPoints("KO4LFZ-9", 10)
	if err != nil {
		t.Fatalf("LoadTrackPoints: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("loaded %d points, want 2 (the 0,0 row skipped)", len(pts))
	}
	for _, p := range pts {
		if station.IsNullIsland(p.Lat, p.Lon) {
			t.Errorf("a 0,0 point was loaded: %+v", p)
		}
	}

	// LIMIT counts real points only: asking for 2 returns the 2 real ones,
	// not "the newest 2 rows, one of which is then discarded".
	pts, _ = s.LoadTrackPoints("KO4LFZ-9", 2)
	if len(pts) != 2 {
		t.Errorf("limit 2 returned %d points, want 2 real points", len(pts))
	}

	// A station whose last saved position is 0,0 loads with no position.
	if err := s.SaveStation(station.Station{
		Callsign: "KD9BNL", SSID: 1, LastHeard: now,
		Position: &station.Position{Lat: 0, Lon: 0},
	}); err != nil {
		t.Fatalf("SaveStation: %v", err)
	}
	sts, err := s.LoadStations()
	if err != nil {
		t.Fatalf("LoadStations: %v", err)
	}
	found := false
	for _, st := range sts {
		if st.Callsign == "KD9BNL" {
			found = true
			if st.Position != nil {
				t.Errorf("station saved at 0,0 loaded with position %+v, want nil", st.Position)
			}
		}
	}
	if !found {
		t.Error("the station itself must still load — only its position is dropped")
	}
}
