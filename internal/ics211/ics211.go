// Package ics211 builds the ICS 211 Incident Check-In List from a net's
// roster of check-ins. It follows internal/ics309's Header/Row/Report +
// Build/ExportCSV shape.
package ics211

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

// Header holds ICS-211 form header fields.
type Header struct {
	IncidentName    string    `json:"incidentName"`
	IncidentNumber  string    `json:"incidentNumber"`
	CheckInLocation string    `json:"checkInLocation"` // net name / frequency
	StartDateTime   time.Time `json:"startDateTime"`
	PreparedBy      string    `json:"preparedBy"`
	PreparedAt      time.Time `json:"preparedAt"`
}

// Row is one resource's check-in record.
type Row struct {
	CheckInTime    time.Time  `json:"checkInTime"`
	NameOrID       string     `json:"nameOrId"`       // "SAG 2 (K6ABC)" or bare callsign
	LeaderName     string     `json:"leaderName"`     // OperatorName
	TotalPersonnel int        `json:"totalPersonnel"` // always 1 in v1 — single-operator check-ins
	HomeAgency     string     `json:"homeAgency"`
	Assignment     string     `json:"assignment"`     // "sag — Rover 2"
	Qualifications string     `json:"qualifications"` // Category
	CheckOutTime   *time.Time `json:"checkOutTime,omitempty"`
	Notes          string     `json:"notes"`
}

// Report is a complete ICS-211 report.
type Report struct {
	Header Header `json:"header"`
	Rows   []Row  `json:"rows"` // never nil
}

// Build converts a net's check-ins into ICS-211 rows, sorted by check-in
// time. A check-in with a tactical call is named "<tactical> (<callsign>)";
// otherwise the bare callsign. Released check-ins carry their check-out
// time.
func Build(h Header, checkIns []store.NetCheckIn, agency string) Report {
	sorted := append([]store.NetCheckIn(nil), checkIns...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CheckedInAt.Before(sorted[j].CheckedInAt) })

	rows := make([]Row, 0, len(sorted))
	for _, ci := range sorted {
		name := ci.Callsign
		if ci.TacticalCall != "" {
			name = fmt.Sprintf("%s (%s)", ci.TacticalCall, ci.Callsign)
		}
		assignment := ci.Category
		if ci.Location != "" {
			assignment = fmt.Sprintf("%s — %s", ci.Category, ci.Location)
		}
		rows = append(rows, Row{
			CheckInTime:    ci.CheckedInAt,
			NameOrID:       name,
			LeaderName:     ci.OperatorName,
			TotalPersonnel: 1,
			HomeAgency:     agency,
			Assignment:     assignment,
			Qualifications: ci.Category,
			CheckOutTime:   ci.CheckedOutAt,
		})
	}
	return Report{Header: h, Rows: rows}
}

// ExportCSV writes an ICS-211 report as CSV.
func ExportCSV(w io.Writer, r Report) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	cw.Write([]string{"ICS 211 Incident Check-In List"})
	cw.Write([]string{"Incident Name", r.Header.IncidentName})
	cw.Write([]string{"Incident Number", r.Header.IncidentNumber})
	cw.Write([]string{"Check-In Location", r.Header.CheckInLocation})
	cw.Write([]string{"Start Date/Time", r.Header.StartDateTime.Format(time.RFC3339)})
	cw.Write([]string{})

	cw.Write([]string{
		"#", "Check-In Time", "Name/ICS Position or ID", "Leader's Name",
		"Total # Personnel", "Home Agency", "Assignment", "Qualifications", "Check-Out Time",
	})

	for i, row := range r.Rows {
		checkOut := ""
		if row.CheckOutTime != nil {
			checkOut = row.CheckOutTime.Format("2006-01-02 15:04:05")
		}
		cw.Write([]string{
			fmt.Sprintf("%d", i+1),
			row.CheckInTime.Format("2006-01-02 15:04:05"),
			row.NameOrID,
			row.LeaderName,
			fmt.Sprintf("%d", row.TotalPersonnel),
			row.HomeAgency,
			row.Assignment,
			row.Qualifications,
			checkOut,
		})
	}

	return cw.Error()
}
