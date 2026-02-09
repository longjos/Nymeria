package ics309

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/message"
)

func TestBuildFromMessages(t *testing.T) {
	now := time.Now()
	msgs := []message.Message{
		{ID: "1", From: "N0ABC", To: "N0XYZ", Body: "Status update", Inbound: true, Timestamp: now.Add(-3 * time.Minute)},
		{ID: "2", From: "N0XYZ", To: "N0ABC", Body: "Copy that", Inbound: false, Timestamp: now.Add(-2 * time.Minute)},
		{ID: "3", From: "N0DEF", To: "N0ABC", Body: "Welfare check", Inbound: true, Timestamp: now.Add(-1 * time.Minute)},
	}

	from := now.Add(-10 * time.Minute)
	to := now.Add(time.Minute)

	rows := BuildFromMessages(msgs, from, to, "")
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Should be chronological order
	if rows[0].From != "N0ABC" {
		t.Errorf("row[0].From = %q, want N0ABC", rows[0].From)
	}
	if rows[1].From != "N0XYZ" {
		t.Errorf("row[1].From = %q, want N0XYZ", rows[1].From)
	}
	if rows[2].From != "N0DEF" {
		t.Errorf("row[2].From = %q, want N0DEF", rows[2].From)
	}

	// Verify subject extraction
	if rows[0].Subject != "Status update" {
		t.Errorf("row[0].Subject = %q, want 'Status update'", rows[0].Subject)
	}
}

func TestBuildFromMessages_TimeFilter(t *testing.T) {
	now := time.Now()
	msgs := []message.Message{
		{ID: "1", From: "N0ABC", To: "N0XYZ", Body: "Before range", Timestamp: now.Add(-10 * time.Minute)},
		{ID: "2", From: "N0ABC", To: "N0XYZ", Body: "In range", Timestamp: now.Add(-5 * time.Minute)},
		{ID: "3", From: "N0ABC", To: "N0XYZ", Body: "After range", Timestamp: now.Add(10 * time.Minute)},
	}

	from := now.Add(-7 * time.Minute)
	to := now

	rows := BuildFromMessages(msgs, from, to, "")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Subject != "In range" {
		t.Errorf("row[0].Subject = %q, want 'In range'", rows[0].Subject)
	}
}

func TestBuildFromMessages_Empty(t *testing.T) {
	now := time.Now()
	rows := BuildFromMessages(nil, now.Add(-time.Hour), now, "")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}

	rows = BuildFromMessages([]message.Message{}, now.Add(-time.Hour), now, "")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for empty slice, got %d", len(rows))
	}
}

func TestBuildFromMessages_SourceFilter(t *testing.T) {
	now := time.Now()
	msgs := []message.Message{
		{ID: "1", From: "N0ABC", To: "N0XYZ", Body: "via RF", Timestamp: now},
	}

	from := now.Add(-time.Minute)
	to := now.Add(time.Minute)

	rows := BuildFromMessages(msgs, from, to, "RF")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Method != "RF" {
		t.Errorf("row[0].Method = %q, want 'RF'", rows[0].Method)
	}
}

func TestExportCSV(t *testing.T) {
	now := time.Now()
	report := Report{
		Header: Header{
			IncidentName: "Spring Flood 2024",
			DateFrom:     now.Add(-2 * time.Hour),
			DateTo:       now,
			OperatorName: "John Doe",
			StationID:    "N0ABC",
		},
		Rows: []Row{
			{
				DateTime: now.Add(-90 * time.Minute),
				From:     "N0ABC",
				To:       "N0XYZ",
				Subject:  "Requesting status",
				Method:   "RF",
			},
			{
				DateTime: now.Add(-85 * time.Minute),
				From:     "N0XYZ",
				To:       "N0ABC",
				Subject:  "All clear sector 3",
				Method:   "RF",
			},
		},
	}

	var buf bytes.Buffer
	if err := ExportCSV(&buf, report); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}

	csv := buf.String()

	// Check ICS-309 header row
	if !strings.Contains(csv, "ICS 309 Communications Log") {
		t.Error("CSV missing ICS 309 header")
	}
	if !strings.Contains(csv, "Spring Flood 2024") {
		t.Error("CSV missing incident name")
	}
	if !strings.Contains(csv, "N0ABC") {
		t.Error("CSV missing station ID")
	}
	if !strings.Contains(csv, "John Doe") {
		t.Error("CSV missing operator name")
	}

	// Check column headers
	if !strings.Contains(csv, "Date/Time") {
		t.Error("CSV missing Date/Time column")
	}
	if !strings.Contains(csv, "From") {
		t.Error("CSV missing From column")
	}
	if !strings.Contains(csv, "To") {
		t.Error("CSV missing To column")
	}
	if !strings.Contains(csv, "Subject") {
		t.Error("CSV missing Subject column")
	}
	if !strings.Contains(csv, "Method") {
		t.Error("CSV missing Method column")
	}

	// Check data rows
	if !strings.Contains(csv, "Requesting status") {
		t.Error("CSV missing first data row")
	}
	if !strings.Contains(csv, "All clear sector 3") {
		t.Error("CSV missing second data row")
	}
}

func TestExportCSV_Empty(t *testing.T) {
	report := Report{
		Header: Header{IncidentName: "Test"},
		Rows:   []Row{},
	}

	var buf bytes.Buffer
	if err := ExportCSV(&buf, report); err != nil {
		t.Fatalf("ExportCSV error: %v", err)
	}

	csv := buf.String()
	if !strings.Contains(csv, "ICS 309 Communications Log") {
		t.Error("CSV should still have headers for empty report")
	}
}
