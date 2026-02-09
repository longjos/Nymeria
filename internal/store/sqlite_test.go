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
	if version != 5 {
		t.Errorf("expected schema version 5, got %d", version)
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
	if version != 5 {
		t.Errorf("expected schema version 5, got %d", version)
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
		MissionID:   "mission-42",
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
	if loaded[0].MissionID != "mission-42" {
		t.Errorf("missionId: got %q, want %q", loaded[0].MissionID, "mission-42")
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
	if version != 5 {
		t.Errorf("expected schema version 5, got %d", version)
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
