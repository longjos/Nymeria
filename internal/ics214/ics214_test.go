package ics214

import (
	"bytes"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

func TestBuildRowsGeneral(t *testing.T) {
	base := time.Date(2026, 6, 13, 6, 0, 0, 0, time.UTC)
	from, to := base, base.Add(4*time.Hour)

	events := []store.NetEvent{
		{CreatedAt: base.Add(10 * time.Minute), Callsign: "K6ABC", Summary: "checked in", Details: "{}"},
		{CreatedAt: base.Add(20 * time.Minute), Callsign: "N6XYZ", Summary: "sag dispatched", Details: "{}"},
		{CreatedAt: base.Add(30 * time.Minute), Callsign: "", Summary: "net opened", Details: "{}"},
		{CreatedAt: base.Add(40 * time.Minute), Callsign: "K6ABC", Summary: "checked out", Details: "{}"},
		{CreatedAt: base.Add(5 * time.Hour), Callsign: "K6ABC", Summary: "outside window", Details: "{}"}, // outside [from,to]
	}
	notes := []store.NetNote{
		{CreatedAt: base.Add(15 * time.Minute), Content: "weather looks fine"},
	}

	rows := BuildRows(events, notes, from, to, UnitFilter{})
	if rows == nil {
		t.Fatal("rows is nil, want non-nil")
	}
	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5", len(rows))
	}
	// Chronological.
	for i := 1; i < len(rows); i++ {
		if rows[i].DateTime.Before(rows[i-1].DateTime) {
			t.Fatalf("rows not chronological at index %d: %v before %v", i, rows[i].DateTime, rows[i-1].DateTime)
		}
	}
	if rows[1].Activity != "NOTE: weather looks fine" {
		t.Errorf("note row activity = %q, want %q", rows[1].Activity, "NOTE: weather looks fine")
	}
	for _, row := range rows {
		if row.DateTime.After(base.Add(4 * time.Hour)) {
			t.Errorf("row outside window leaked in: %+v", row)
		}
	}
}

func TestBuildRowsSAGVariant(t *testing.T) {
	base := time.Date(2026, 6, 13, 6, 0, 0, 0, time.UTC)
	from, to := base, base.Add(4*time.Hour)

	events := []store.NetEvent{
		{CreatedAt: base.Add(1 * time.Minute), Callsign: "N6OTHER", Summary: "dispatched", Details: `{"checkInId":"ci1"}`},
		{CreatedAt: base.Add(2 * time.Minute), Callsign: "SAG 2", Summary: "enroute", Details: "{}"},
		{CreatedAt: base.Add(3 * time.Minute), Callsign: "K6ABC", Summary: "on scene", Details: "{}"},
		{CreatedAt: base.Add(4 * time.Minute), Callsign: "N6UNRELATED", Summary: "unrelated traffic", Details: "{}"},
	}

	rows := BuildRows(events, nil, from, to, UnitFilter{CheckInID: "ci1", Callsigns: []string{"K6ABC", "SAG 2"}})
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	for _, row := range rows {
		if row.Activity == "N6UNRELATED: unrelated traffic" {
			t.Errorf("unrelated row leaked in: %+v", row)
		}
	}
}

func TestBuildRowsRSVariant(t *testing.T) {
	base := time.Date(2026, 6, 13, 6, 0, 0, 0, time.UTC)
	from, to := base, base.Add(4*time.Hour)

	events := []store.NetEvent{
		{CreatedAt: base.Add(1 * time.Minute), Summary: "supplies logged", Details: `{"annotationId":"rs3"}`},
		{CreatedAt: base.Add(2 * time.Minute), Summary: "Rest Stop 3 closed", Details: "{}"},
		{CreatedAt: base.Add(3 * time.Minute), Summary: "RS3 out of water", Details: "{}"},
		{CreatedAt: base.Add(4 * time.Minute), Summary: "unrelated traffic", Details: "{}"},
	}

	rows := BuildRows(events, nil, from, to, UnitFilter{AnnotationID: "rs3", Labels: []string{"Rest Stop 3", "RS3"}})
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
}

func TestBuildRowsMalformedDetails(t *testing.T) {
	base := time.Date(2026, 6, 13, 6, 0, 0, 0, time.UTC)
	from, to := base, base.Add(4*time.Hour)

	events := []store.NetEvent{
		{CreatedAt: base.Add(1 * time.Minute), Callsign: "K6ABC", Summary: "sag update", Details: "not json"},
		{CreatedAt: base.Add(2 * time.Minute), Callsign: "N6OTHER", Summary: "unrelated", Details: "not json either"},
	}

	// Must not panic despite unparsable Details, and must still match by
	// callsign.
	rows := BuildRows(events, nil, from, to, UnitFilter{Callsigns: []string{"K6ABC"}})
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Activity != "K6ABC: sag update" {
		t.Errorf("Activity = %q, want %q", rows[0].Activity, "K6ABC: sag update")
	}
}

func TestExportCSV(t *testing.T) {
	base := time.Date(2026, 6, 13, 6, 0, 0, 0, time.UTC)

	t.Run("with resources", func(t *testing.T) {
		report := Report{
			Header: Header{
				IncidentName: "Test Ride", IncidentNumber: "2026-100",
				OperationalFrom: base, OperationalTo: base.Add(8 * time.Hour),
				Name: "SAG 2", ICSPosition: "SAG Driver", HomeAgency: "Marin Cyclists",
				Variant: VariantSAG,
				ResourcesAssigned: []Resource{
					{Name: "K6ABC", ICSPosition: "SAG Driver", HomeAgency: "Marin Cyclists"},
				},
			},
			Rows: []Row{{DateTime: base, Activity: "dispatched"}},
		}
		var buf bytes.Buffer
		if err := ExportCSV(&buf, report); err != nil {
			t.Fatalf("ExportCSV: %v", err)
		}
		out := buf.String()
		for _, want := range []string{
			"ICS 214 Activity Log", "Variant", "Operational Period From", "Operational Period To",
			"Name", "ICS Position", "Home Agency", "Resources Assigned", "#,Date/Time,Notable Activities",
		} {
			if !bytes.Contains(buf.Bytes(), []byte(want)) {
				t.Errorf("csv missing %q; got:\n%s", want, out)
			}
		}
	})

	t.Run("empty resources still writes section header", func(t *testing.T) {
		report := Report{
			Header: Header{Name: "NCS", Variant: VariantGeneral, OperationalFrom: base, OperationalTo: base},
			Rows:   nil,
		}
		var buf bytes.Buffer
		if err := ExportCSV(&buf, report); err != nil {
			t.Fatalf("ExportCSV: %v", err)
		}
		if !bytes.Contains(buf.Bytes(), []byte("Resources Assigned")) {
			t.Errorf("csv missing Resources Assigned section header with zero resources:\n%s", buf.String())
		}
	})
}
