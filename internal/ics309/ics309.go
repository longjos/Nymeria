package ics309

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/narvel/nymeria/internal/message"
)

// Header holds ICS-309 form header fields.
type Header struct {
	IncidentName string    `json:"incidentName"`
	DateFrom     time.Time `json:"dateFrom"`
	DateTo       time.Time `json:"dateTo"`
	OperatorName string    `json:"operatorName"`
	StationID    string    `json:"stationId"`
}

// Row is a single entry in the communications log.
type Row struct {
	DateTime time.Time `json:"dateTime"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	Subject  string    `json:"subject"`
	Method   string    `json:"method"`
}

// Report is a complete ICS-309 report.
type Report struct {
	Header Header `json:"header"`
	Rows   []Row  `json:"rows"`
}

// BuildFromMessages converts APRS messages into ICS-309 log rows,
// filtering by time range and sorting chronologically.
// If defaultMethod is non-empty, it is used as the Method for all rows.
func BuildFromMessages(msgs []message.Message, from, to time.Time, defaultMethod string) []Row {
	var rows []Row
	for _, m := range msgs {
		if m.Timestamp.Before(from) || m.Timestamp.After(to) {
			continue
		}

		method := defaultMethod
		if method == "" {
			method = "APRS"
		}

		rows = append(rows, Row{
			DateTime: m.Timestamp,
			From:     m.From,
			To:       m.To,
			Subject:  m.Body,
			Method:   method,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].DateTime.Before(rows[j].DateTime)
	})

	return rows
}

// ExportCSV writes an ICS-309 report in CSV format with header metadata.
func ExportCSV(w io.Writer, report Report) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// ICS-309 header block
	cw.Write([]string{"ICS 309 Communications Log"})
	cw.Write([]string{"Incident Name", report.Header.IncidentName})
	cw.Write([]string{"Operational Period From", report.Header.DateFrom.Format(time.RFC3339)})
	cw.Write([]string{"Operational Period To", report.Header.DateTo.Format(time.RFC3339)})
	cw.Write([]string{"Radio Operator Name", report.Header.OperatorName})
	cw.Write([]string{"Station ID", report.Header.StationID})
	cw.Write([]string{}) // blank separator

	// Column headers
	cw.Write([]string{"#", "Date/Time", "From", "To", "Subject", "Method"})

	// Data rows
	for i, row := range report.Rows {
		cw.Write([]string{
			fmt.Sprintf("%d", i+1),
			row.DateTime.Format("2006-01-02 15:04:05"),
			row.From,
			row.To,
			row.Subject,
			row.Method,
		})
	}

	return cw.Error()
}
