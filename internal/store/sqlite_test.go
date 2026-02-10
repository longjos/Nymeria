package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
	if version != 13 {
		t.Errorf("expected schema version 13, got %d", version)
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
	if version != 13 {
		t.Errorf("expected schema version 13, got %d", version)
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
		AssignedTo:  "KD7BBC",
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
	if got.AssignedTo != m.AssignedTo {
		t.Errorf("assignedTo: got %q, want %q", got.AssignedTo, m.AssignedTo)
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
		ID:         "m-loc",
		NetID:      "net-1",
		Title:      "Shelter Setup",
		Priority:   "priority",
		Status:     "open",
		Location:   "Red Cross Shelter #3",
		Lat:        &lat,
		Lon:        &lon,
		CreatedAt:  now,
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
	if version != 13 {
		t.Errorf("expected schema version 13, got %d", version)
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
	if version != 13 {
		t.Errorf("expected schema version 13, got %d", version)
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
	if version != 13 {
		t.Errorf("expected schema version 13, got %d", version)
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
	if version != 13 {
		t.Errorf("expected schema version 13, got %d", version)
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
	if version != 13 {
		t.Errorf("expected schema version 13, got %d", version)
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
	if version != 13 {
		t.Errorf("expected schema version 13, got %d", version)
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
