// Package ics214 builds the ICS 214 Activity Log, and its unit variants
// (214-RS for a rest stop, 214-SAG for a SAG unit), from a net's timeline
// and notes. It follows internal/ics309's Header/Row/Report +
// Build/ExportCSV shape.
package ics214

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

// Variant selects which slice of the timeline a report covers.
const (
	VariantGeneral = "general" // whole net (NCS log)
	VariantRS      = "rs"      // one rest stop (annotation category aid)
	VariantSAG     = "sag"     // one SAG unit (check-in category sag)
)

// Resource is one line of the header's "Resources Assigned" sub-table.
type Resource struct {
	Name        string `json:"name"`
	ICSPosition string `json:"icsPosition"`
	HomeAgency  string `json:"homeAgency"`
}

// Header holds ICS-214 form header fields.
type Header struct {
	IncidentName      string     `json:"incidentName"`
	IncidentNumber    string     `json:"incidentNumber"`
	OperationalFrom   time.Time  `json:"operationalFrom"`
	OperationalTo     time.Time  `json:"operationalTo"`
	Name              string     `json:"name"`        // unit name: NCS callsign, "Rest Stop 3", "SAG 2"
	ICSPosition       string     `json:"icsPosition"` // "Net Control", "Rest Stop Communicator", "SAG Driver"
	HomeAgency        string     `json:"homeAgency"`
	Variant           string     `json:"variant"`
	ResourcesAssigned []Resource `json:"resourcesAssigned"` // never nil
	PreparedBy        string     `json:"preparedBy"`
	PreparedAt        time.Time  `json:"preparedAt"`
}

// Row is one notable-activity line.
type Row struct {
	DateTime time.Time `json:"dateTime"`
	Activity string    `json:"activity"`
}

// Report is a complete ICS-214 report.
type Report struct {
	Header Header `json:"header"`
	Rows   []Row  `json:"rows"` // never nil
}

// eventRef is the subset of store.NetEvent.Details this package looks for
// to match a unit variant, written by ride/course/reconcile's timeline
// calls as {"checkInId":...} or {"annotationId":...}. Any subset of keys;
// unknown keys and malformed JSON are ignored (matched by callsign/label
// only in that case), never panicked on.
type eventRef struct {
	CheckInID    string `json:"checkInId"`
	AnnotationID string `json:"annotationId"`
}

// UnitFilter selects rows for a variant. For VariantGeneral both are zero.
type UnitFilter struct {
	CheckInID    string // VariantSAG: match eventRef.checkInId OR NetEvent.Callsign ∈ Callsigns
	Callsigns    []string
	AnnotationID string // VariantRS: match eventRef.annotationId OR Summary contains a Labels entry
	Labels       []string
}

func matchesUnit(e store.NetEvent, f UnitFilter) bool {
	if f.CheckInID == "" && f.AnnotationID == "" && len(f.Callsigns) == 0 && len(f.Labels) == 0 {
		return true // VariantGeneral: no filter
	}
	var ref eventRef
	_ = json.Unmarshal([]byte(e.Details), &ref) // malformed/absent JSON leaves ref zero-valued, never panics

	if f.CheckInID != "" && ref.CheckInID == f.CheckInID {
		return true
	}
	if f.AnnotationID != "" && ref.AnnotationID == f.AnnotationID {
		return true
	}
	for _, cs := range f.Callsigns {
		if cs != "" && strings.EqualFold(e.Callsign, cs) {
			return true
		}
	}
	for _, label := range f.Labels {
		if label != "" && strings.Contains(e.Summary, label) {
			return true
		}
	}
	return false
}

// BuildRows merges timeline events and notes into chronological ICS-214
// rows within [from, to], filtered to a unit when f names one. A note
// becomes "NOTE: <content>".
func BuildRows(events []store.NetEvent, notes []store.NetNote, from, to time.Time, f UnitFilter) []Row {
	rows := []Row{}
	for _, e := range events {
		if e.CreatedAt.Before(from) || e.CreatedAt.After(to) {
			continue
		}
		if !matchesUnit(e, f) {
			continue
		}
		activity := e.Summary
		if e.Callsign != "" {
			activity = fmt.Sprintf("%s: %s", e.Callsign, e.Summary)
		}
		rows = append(rows, Row{DateTime: e.CreatedAt, Activity: activity})
	}
	for _, n := range notes {
		if n.CreatedAt.Before(from) || n.CreatedAt.After(to) {
			continue
		}
		rows = append(rows, Row{DateTime: n.CreatedAt, Activity: "NOTE: " + n.Content})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].DateTime.Before(rows[j].DateTime) })
	return rows
}

// ExportCSV writes an ICS-214 report as CSV.
func ExportCSV(w io.Writer, r Report) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	cw.Write([]string{"ICS 214 Activity Log"})
	cw.Write([]string{"Incident Name", r.Header.IncidentName})
	cw.Write([]string{"Incident Number", r.Header.IncidentNumber})
	cw.Write([]string{"Operational Period From", r.Header.OperationalFrom.Format(time.RFC3339)})
	cw.Write([]string{"Operational Period To", r.Header.OperationalTo.Format(time.RFC3339)})
	cw.Write([]string{"Name", r.Header.Name})
	cw.Write([]string{"ICS Position", r.Header.ICSPosition})
	cw.Write([]string{"Home Agency", r.Header.HomeAgency})
	cw.Write([]string{"Variant", r.Header.Variant})
	cw.Write([]string{})

	cw.Write([]string{"Resources Assigned"})
	cw.Write([]string{"Name", "ICS Position", "Home Agency"})
	for _, res := range r.Header.ResourcesAssigned {
		cw.Write([]string{res.Name, res.ICSPosition, res.HomeAgency})
	}
	cw.Write([]string{})

	cw.Write([]string{"#", "Date/Time", "Notable Activities"})
	for i, row := range r.Rows {
		cw.Write([]string{
			fmt.Sprintf("%d", i+1),
			row.DateTime.Format("2006-01-02 15:04:05"),
			row.Activity,
		})
	}

	return cw.Error()
}
