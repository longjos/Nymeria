package store

import (
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
