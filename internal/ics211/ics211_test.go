package ics211

import (
	"bytes"
	"encoding/csv"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

func TestBuild(t *testing.T) {
	t1 := time.Date(2026, 6, 13, 6, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 13, 5, 30, 0, 0, time.UTC)
	t3 := time.Date(2026, 6, 13, 5, 45, 0, 0, time.UTC)
	checkOut := t1.Add(8 * time.Hour)

	checkIns := []store.NetCheckIn{
		{
			ID: "ci-1", Callsign: "K6ABC", TacticalCall: "SAG 2", OperatorName: "Pat Driver",
			Category: "sag", Location: "Rover 2", CheckedInAt: t1, CheckedOutAt: &checkOut,
			Status: "released",
		},
		{
			ID: "ci-2", Callsign: "N6CMD", OperatorName: "Chris NCS",
			Category: "command", CheckedInAt: t2,
		},
		{
			ID: "ci-3", Callsign: "W6SAG", TacticalCall: "SAG 1", OperatorName: "Sam Driver",
			Category: "sag", Location: "Rover 1", CheckedInAt: t3,
		},
	}

	report := Build(Header{IncidentName: "Test Ride"}, checkIns, "Marin Cyclists")

	if report.Rows == nil {
		t.Fatal("Rows is nil, want non-nil")
	}
	if len(report.Rows) != 3 {
		t.Fatalf("len(Rows) = %d, want 3", len(report.Rows))
	}

	// Sorted by CheckedInAt ascending: ci-2 (5:30), ci-3 (5:45), ci-1 (6:00).
	if report.Rows[0].NameOrID != "N6CMD" {
		t.Errorf("Rows[0].NameOrID = %q, want N6CMD (bare callsign, no tactical)", report.Rows[0].NameOrID)
	}
	if report.Rows[1].NameOrID != "SAG 1 (W6SAG)" {
		t.Errorf("Rows[1].NameOrID = %q, want %q", report.Rows[1].NameOrID, "SAG 1 (W6SAG)")
	}
	if report.Rows[2].NameOrID != "SAG 2 (K6ABC)" {
		t.Errorf("Rows[2].NameOrID = %q, want %q", report.Rows[2].NameOrID, "SAG 2 (K6ABC)")
	}
	if report.Rows[2].CheckOutTime == nil || !report.Rows[2].CheckOutTime.Equal(checkOut) {
		t.Errorf("Rows[2].CheckOutTime = %v, want %v", report.Rows[2].CheckOutTime, checkOut)
	}
	if report.Rows[1].CheckOutTime != nil {
		t.Errorf("Rows[1].CheckOutTime = %v, want nil (not checked out)", report.Rows[1].CheckOutTime)
	}
	if report.Rows[2].Assignment != "sag — Rover 2" {
		t.Errorf("Assignment = %q, want %q", report.Rows[2].Assignment, "sag — Rover 2")
	}
	for _, row := range report.Rows {
		if row.HomeAgency != "Marin Cyclists" {
			t.Errorf("HomeAgency = %q, want Marin Cyclists", row.HomeAgency)
		}
		if row.TotalPersonnel != 1 {
			t.Errorf("TotalPersonnel = %d, want 1", row.TotalPersonnel)
		}
	}
}

func TestBuildZeroCheckIns(t *testing.T) {
	report := Build(Header{}, nil, "Agency")
	if report.Rows == nil {
		t.Fatal("Rows is nil, want empty non-nil slice")
	}
	if len(report.Rows) != 0 {
		t.Errorf("len(Rows) = %d, want 0", len(report.Rows))
	}
}

func TestExportCSV(t *testing.T) {
	t1 := time.Date(2026, 6, 13, 6, 0, 0, 0, time.UTC)
	report := Report{
		Header: Header{
			IncidentName: "Test Ride", IncidentNumber: "2026-100",
			CheckInLocation: "Net Control / 146.520", StartDateTime: t1,
		},
		Rows: []Row{
			{CheckInTime: t1, NameOrID: "SAG 2 (K6ABC)", LeaderName: "Pat", TotalPersonnel: 1, HomeAgency: "Agency", Assignment: "sag", Qualifications: "sag"},
		},
	}

	var buf bytes.Buffer
	if err := ExportCSV(&buf, report); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}

	cr := csv.NewReader(&buf)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}

	if rows[0][0] != "ICS 211 Incident Check-In List" {
		t.Errorf("rows[0] = %v, want title row", rows[0])
	}
	if rows[1][0] != "Incident Name" || rows[1][1] != "Test Ride" {
		t.Errorf("rows[1] = %v, want Incident Name row", rows[1])
	}
	if rows[2][0] != "Incident Number" {
		t.Errorf("rows[2] = %v, want Incident Number row", rows[2])
	}
	if rows[3][0] != "Check-In Location" {
		t.Errorf("rows[3] = %v, want Check-In Location row", rows[3])
	}
	if rows[4][0] != "Start Date/Time" {
		t.Errorf("rows[4] = %v, want Start Date/Time row", rows[4])
	}
	// The blank separator line is dropped by csv.Reader (blank lines are
	// ignored), so the column header row lands at index 5.
	if len(rows[5]) != 9 {
		t.Errorf("column header row has %d columns, want 9: %v", len(rows[5]), rows[5])
	}
	// 5 header-block rows (title..start date, blank dropped) + 1 column
	// header row + 1 data row.
	if len(rows) != 7 {
		t.Errorf("len(rows) = %d, want 7", len(rows))
	}
}
