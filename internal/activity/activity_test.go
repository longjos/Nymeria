package activity

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

func newTestLogger(t *testing.T) *StoreLogger {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("store Init failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStoreLogger(s)
}

func TestLogAndQuery(t *testing.T) {
	logger := newTestLogger(t)

	now := time.Now().Truncate(time.Second).UTC()

	entry := Entry{
		Timestamp: now,
		UserID:    "user-1",
		UserName:  "Alice",
		Action:    ActionMessageSent,
		Target:    "W1AW",
		Details:   "Hello World",
	}
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	entries, total, err := logger.Query(Filter{Limit: 10})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := entries[0]
	if got.Action != ActionMessageSent {
		t.Errorf("action: got %q, want %q", got.Action, ActionMessageSent)
	}
	if got.UserID != "user-1" {
		t.Errorf("userID: got %q, want %q", got.UserID, "user-1")
	}
	if got.UserName != "Alice" {
		t.Errorf("userName: got %q, want %q", got.UserName, "Alice")
	}
	if got.Target != "W1AW" {
		t.Errorf("target: got %q, want %q", got.Target, "W1AW")
	}
	if got.Details != "Hello World" {
		t.Errorf("details: got %q, want %q", got.Details, "Hello World")
	}
}

func TestFilterByUser(t *testing.T) {
	logger := newTestLogger(t)
	now := time.Now().Truncate(time.Second).UTC()

	logger.Log(Entry{Timestamp: now, UserID: "user-1", Action: ActionMessageSent})
	logger.Log(Entry{Timestamp: now, UserID: "user-2", Action: ActionMessageSent})
	logger.Log(Entry{Timestamp: now, UserID: "user-1", Action: ActionAnnotationCreated})

	entries, total, err := logger.Query(Filter{UserID: "user-1", Limit: 10})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestFilterByAction(t *testing.T) {
	logger := newTestLogger(t)
	now := time.Now().Truncate(time.Second).UTC()

	logger.Log(Entry{Timestamp: now, Action: ActionSessionStarted})
	logger.Log(Entry{Timestamp: now, Action: ActionMessageSent})
	logger.Log(Entry{Timestamp: now, Action: ActionSessionStarted})

	entries, total, err := logger.Query(Filter{Action: string(ActionSessionStarted), Limit: 10})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestFilterByTimeRange(t *testing.T) {
	logger := newTestLogger(t)
	base := time.Now().Truncate(time.Second).UTC()

	for i := 0; i < 5; i++ {
		logger.Log(Entry{
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Action:    ActionBeaconSent,
		})
	}

	since := base.Add(1 * time.Hour)
	until := base.Add(3 * time.Hour)
	entries, total, err := logger.Query(Filter{Since: &since, Until: &until, Limit: 10})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestPagination(t *testing.T) {
	logger := newTestLogger(t)
	now := time.Now().Truncate(time.Second).UTC()

	for i := 0; i < 10; i++ {
		logger.Log(Entry{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Action:    ActionBeaconSent,
		})
	}

	// Page 1.
	page1, total, err := logger.Query(Filter{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("Query page 1 failed: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total 10, got %d", total)
	}
	if len(page1) != 3 {
		t.Errorf("expected 3 entries on page 1, got %d", len(page1))
	}

	// Page 2.
	page2, _, err := logger.Query(Filter{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("Query page 2 failed: %v", err)
	}
	if len(page2) != 3 {
		t.Errorf("expected 3 entries on page 2, got %d", len(page2))
	}

	// Pages should have different entries.
	if len(page1) > 0 && len(page2) > 0 && page1[0].Timestamp.Equal(page2[0].Timestamp) {
		t.Error("page 2 should have different entries than page 1")
	}
}

func TestEventsChannel(t *testing.T) {
	logger := newTestLogger(t)
	now := time.Now().Truncate(time.Second).UTC()

	events := logger.Events()

	entry := Entry{
		Timestamp: now,
		Action:    ActionSessionStarted,
		UserID:    "user-1",
	}

	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	select {
	case got := <-events:
		if got.Action != ActionSessionStarted {
			t.Errorf("event action: got %q, want %q", got.Action, ActionSessionStarted)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event on channel")
	}
}

func TestExportCSV(t *testing.T) {
	now := time.Now().Truncate(time.Second).UTC()

	entries := []Entry{
		{
			Timestamp: now,
			UserID:    "user-1",
			UserName:  "Alice",
			Action:    ActionMessageSent,
			Target:    "W1AW",
			Details:   "test msg",
		},
		{
			Timestamp: now.Add(time.Minute),
			UserID:    "user-2",
			UserName:  "Bob",
			Action:    ActionAnnotationCreated,
			Target:    "ann-1",
			Details:   "new marker",
		},
	}

	var buf bytes.Buffer
	if err := ExportCSV(&buf, entries); err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Header + 2 data rows.
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 data), got %d", len(lines))
	}

	// Verify header.
	if !strings.HasPrefix(lines[0], "timestamp,") {
		t.Errorf("unexpected header: %q", lines[0])
	}

	// Verify first data row contains expected fields.
	if !strings.Contains(lines[1], "user-1") {
		t.Errorf("first data row should contain user-1: %q", lines[1])
	}
	if !strings.Contains(lines[1], "message_sent") {
		t.Errorf("first data row should contain message_sent: %q", lines[1])
	}
}

func TestActionConstants(t *testing.T) {
	// Verify all action constants are non-empty strings.
	actions := []Action{
		ActionMessageSent,
		ActionMessageClaimed,
		ActionObjectCreated,
		ActionObjectKilled,
		ActionAnnotationCreated,
		ActionAnnotationDeleted,
		ActionConfigChanged,
		ActionSessionStarted,
		ActionSessionEnded,
		ActionBeaconSent,
		ActionTransportConnect,
		ActionTransportDisconnect,
	}

	for _, a := range actions {
		if a == "" {
			t.Error("action constant should not be empty")
		}
	}

	// All should be unique.
	seen := make(map[Action]bool)
	for _, a := range actions {
		if seen[a] {
			t.Errorf("duplicate action: %q", a)
		}
		seen[a] = true
	}
}
