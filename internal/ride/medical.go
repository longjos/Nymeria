package ride

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/store"
)

// validSexes are the scripted "sex" field's allowed values: M/F/X/U
// (unknown/not stated). This is deliberately not a clinical field.
var validSexes = map[string]bool{"M": true, "F": true, "X": true, "U": true}

// validSeverities drives bib-withholding only (governing fact 7); it is not
// a clinical assessment field.
var validSeverities = map[string]bool{
	store.SeverityRoutine: true, store.SeveritySerious: true, store.SeveritySevere: true,
}

// validDestinations are the scripted "departed for" destinations.
var validDestinations = map[string]bool{
	store.DestHospital: true, store.DestStart: true, store.DestFinish: true,
	store.DestRestStop: true, store.DestOther: true,
}

// nameAllowedDestinations: PatientName (governing fact 7) is recorded only
// for riders transported to a hospital or back to the start.
var nameAllowedDestinations = map[string]bool{store.DestHospital: true, store.DestStart: true}

// terminalMedicalStatuses are the statuses a medical notification never
// leaves.
var terminalMedicalStatuses = map[string]bool{
	store.MedDeparted: true, store.MedReleased: true, store.MedCancelled: true,
}

// CreateMedicalInput is the input to CreateMedical. Fields are taken in
// COURSE-plan script order: bib / sex / age / exact location / chief
// complaint / readback. There are no clinical fields beyond ChiefComplaint.
type CreateMedicalInput struct {
	ReportedByCheckInID  string   `json:"reportedByCheckInId"`
	ReportedByCall       string   `json:"reportedByCall"`
	Bib                  string   `json:"bib"`
	Sex                  string   `json:"sex"`
	Age                  string   `json:"age"`
	Location             string   `json:"location"`
	MilesRemaining       *float64 `json:"milesRemaining"`
	RouteID              string   `json:"routeId"`
	LocationAnnotationID string   `json:"locationAnnotationId"`
	Lat                  *float64 `json:"lat"`
	Lon                  *float64 `json:"lon"`
	ChiefComplaint       string   `json:"chiefComplaint"`
	Severity             string   `json:"severity"`
	Priority             string   `json:"priority"`
	Notes                string   `json:"notes"`
	Division             *string  `json:"division"`
	// Composer may take the read-back in the same breath as the report.
	ReadbackConfirmed bool   `json:"readbackConfirmed"`
	ReadBackBy        string `json:"readBackBy"`
}

// MedicalETAInput is the input to RecordMedicalETA.
type MedicalETAInput struct {
	EMSUnit string `json:"emsUnit"`
	Minutes int    `json:"minutes"`
}

// OnSceneInput is the input to MedicalOnScene.
type OnSceneInput struct {
	EMSUnit string     `json:"emsUnit"`
	At      *time.Time `json:"at"` // nil = now; allows "they got there 5 min ago" corrections
}

// DepartInput is the input to MedicalDepart.
type DepartInput struct {
	Destination     string     `json:"destination"`
	DestinationName string     `json:"destinationName"`
	PatientCount    int        `json:"patientCount"`
	PatientName     string     `json:"patientName"` // only honored for hospital|start (fact 7); else 400
	At              *time.Time `json:"at"`
}

// ReleaseInput is the input to MedicalRelease.
type ReleaseInput struct {
	Reason string     `json:"reason"` // "treated and released", "refused transport"
	At     *time.Time `json:"at"`
}

func indexOfMedical(list []store.MedicalNotification, id string) int {
	for i := range list {
		if list[i].ID == id {
			return i
		}
	}
	return -1
}

func (m *TrafficManager) findMedicalLocked(netID, id string) ([]store.MedicalNotification, int) {
	list := m.medical[netID]
	return list, indexOfMedical(list, id)
}

// bibForSummary returns "bib withheld" or "bib <n>" for on-air-safe
// timeline text, never the raw bib when withheld.
func bibForSummary(n store.MedicalNotification) string {
	if n.BibWithheld || n.Bib == "" {
		return "bib withheld"
	}
	return "bib " + n.Bib
}

func medicalReportSummary(n store.MedicalNotification) string {
	loc := n.Location
	if n.MilesRemaining != nil {
		loc = fmt.Sprintf("mile %s near %s", trimFloat(*n.MilesRemaining), n.Location)
	}
	return fmt.Sprintf("MEDICAL from %s: %s, %s, %s, %s, %s [%s]",
		n.ReportedByCall, bibForSummary(n), n.Sex, n.Age, loc, n.ChiefComplaint, strings.ToUpper(n.Priority))
}

// GetMedical returns every medical notification for a net, ordered by
// CreatedAt ascending. Never nil. redacted controls whether PatientName and
// a withheld Bib are blanked (observer tier passes true; operator tier
// passes false).
func (m *TrafficManager) GetMedical(netID string, redacted bool) []store.MedicalNotification {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.medical[netID]
	out := make([]store.MedicalNotification, len(list))
	for i, n := range list {
		if redacted {
			out[i] = n.Redacted()
		} else {
			out[i] = n
		}
	}
	return out
}

// GetMedicalOne returns one medical notification.
func (m *TrafficManager) GetMedicalOne(netID, id string, redacted bool) (*store.MedicalNotification, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list, idx := m.findMedicalLocked(netID, id)
	if idx == -1 {
		return nil, false
	}
	n := list[idx]
	if redacted {
		n = n.Redacted()
	}
	return &n, true
}

// CreateMedical takes the scripted fields (bib/sex/age/location/chief
// complaint), applies the severe-injury bib-withholding policy, and
// optionally takes the read-back in the same call (the composer may do
// both in one breath).
func (m *TrafficManager) CreateMedical(netID string, in CreateMedicalInput, a Actor) (*store.MedicalNotification, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Location) == "" {
		return nil, fmt.Errorf("location is required")
	}
	if strings.TrimSpace(in.ChiefComplaint) == "" {
		return nil, fmt.Errorf("chiefComplaint is required")
	}
	sex := strings.ToUpper(strings.TrimSpace(in.Sex))
	if sex == "" {
		sex = "U"
	}
	if !validSexes[sex] {
		return nil, fmt.Errorf("invalid sex %q (must be M, F, X or U)", in.Sex)
	}
	severity := strings.ToLower(strings.TrimSpace(in.Severity))
	if severity == "" {
		severity = store.SeverityRoutine
	}
	if !validSeverities[severity] {
		return nil, fmt.Errorf("invalid severity %q", in.Severity)
	}
	priority := strings.ToLower(strings.TrimSpace(in.Priority))
	if priority == "" {
		priority = PriorityPriority
	}
	if !m.policy.ValidTier(netID, priority) {
		return nil, fmt.Errorf("invalid priority %q", in.Priority)
	}

	now := m.now()
	n := store.MedicalNotification{
		ID:                   uuid.New().String(),
		NetID:                netID,
		Division:             in.Division,
		ReportedByCheckInID:  in.ReportedByCheckInID,
		ReportedByCall:       in.ReportedByCall,
		Bib:                  in.Bib,
		Sex:                  sex,
		Age:                  in.Age,
		Location:             in.Location,
		MilesRemaining:       in.MilesRemaining,
		RouteID:              in.RouteID,
		LocationAnnotationID: in.LocationAnnotationID,
		Lat:                  in.Lat,
		Lon:                  in.Lon,
		ChiefComplaint:       in.ChiefComplaint,
		Severity:             severity,
		Priority:             priority,
		Status:               store.MedReported,
		Notes:                in.Notes,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	// Governing fact 7: per-net policy withholds even the bib for severe
	// injuries. The bib is still stored (dispatch/EMS coordination may
	// still need it off the air); Redacted() blanks it for anyone else.
	if severity == store.SeveritySevere && m.policy.WithholdSevereBib(netID) {
		n.BibWithheld = true
	}
	if in.ReadbackConfirmed {
		n.Status = store.MedConfirmed
		n.ReadBackAt = &now
		n.ReadBackBy = in.ReadBackBy
	}

	m.mu.Lock()
	m.medical[netID] = append(m.medical[netID], n)
	m.mu.Unlock()

	if err := m.store.SaveMedicalNotification(n); err != nil {
		return nil, fmt.Errorf("persist medical notification: %w", err)
	}

	summary := medicalReportSummary(n)
	details, _ := json.Marshal(map[string]any{"bibWithheld": n.BibWithheld, "severity": n.Severity})
	m.logTimeline(netID, TLMedicalReported, a.Callsign, summary, string(details))
	if in.ReadbackConfirmed {
		m.logTimeline(netID, TLMedicalReadbackConfirmed, a.Callsign, fmt.Sprintf("%s confirmed medical read-back", n.ReportedByCall), "")
	}
	m.logActivity(activity.ActionMedicalNotifCreated, a, n.ID, medicalActivityDetails(n))
	m.emit(Event{Type: EventMedicalCreated, Data: n.Redacted()})

	out := n
	return &out, nil
}

// medicalActivityDetails is a small, privacy-safe details blob for the
// activity log: bib (or "withheld") and status, never PatientName.
func medicalActivityDetails(n store.MedicalNotification) string {
	bib := n.Bib
	if n.BibWithheld {
		bib = "withheld"
	}
	b, _ := json.Marshal(map[string]any{"bib": bib, "status": n.Status})
	return string(b)
}

// ReadbackMedical is the transmission-boundary step. Confirmed=false keeps
// the notification at "reported" and appends a correction note.
func (m *TrafficManager) ReadbackMedical(netID, id string, in ReadbackInput, a Actor) (*store.MedicalNotification, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	list, idx := m.findMedicalLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("medical notification %q not found: %w", id, ErrNotFound)
	}
	n := list[idx]
	if n.Status != store.MedReported {
		m.mu.Unlock()
		return nil, illegalTransition("medical notification is not awaiting read-back")
	}

	now := m.now()
	loggedTimeline := false
	if in.Confirmed {
		n.Status = store.MedConfirmed
		n.ReadBackAt = &now
		n.ReadBackBy = in.ReadBackBy
		loggedTimeline = true
	} else {
		note := strings.TrimSpace(in.Correction)
		if note == "" {
			note = "requester corrected the read-back"
		}
		if n.Notes == "" {
			n.Notes = "correction: " + note
		} else {
			n.Notes = n.Notes + "; correction: " + note
		}
	}
	n.UpdatedAt = now
	list[idx] = n
	m.medical[netID] = list
	m.mu.Unlock()

	if err := m.store.SaveMedicalNotification(n); err != nil {
		return nil, fmt.Errorf("persist medical notification: %w", err)
	}
	if loggedTimeline {
		m.logTimeline(netID, TLMedicalReadbackConfirmed, a.Callsign, fmt.Sprintf("%s confirmed medical read-back", n.ReportedByCall), "")
	}
	m.logActivity(activity.ActionMedicalNotifUpdated, a, n.ID, medicalActivityDetails(n))
	m.emit(Event{Type: EventMedicalUpdated, Data: n.Redacted()})

	out := n
	return &out, nil
}

// RecordMedicalETA records EMS's stated arrival time. Legal from confirmed
// or ems_enroute (suppliers/EMS revise their ETA).
func (m *TrafficManager) RecordMedicalETA(netID, id string, in MedicalETAInput, a Actor) (*store.MedicalNotification, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}
	if in.Minutes <= 0 {
		return nil, fmt.Errorf("minutes must be greater than 0")
	}

	m.mu.Lock()
	list, idx := m.findMedicalLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("medical notification %q not found: %w", id, ErrNotFound)
	}
	n := list[idx]
	if n.Status != store.MedConfirmed && n.Status != store.MedEMSEnRoute {
		m.mu.Unlock()
		return nil, illegalTransition("medical notification has not been read back")
	}

	now := m.now()
	minutes := in.Minutes
	due := now.Add(time.Duration(in.Minutes) * time.Minute)
	if in.EMSUnit != "" {
		n.EMSUnit = in.EMSUnit
	}
	n.ETAMinutes = &minutes
	n.ETAGivenAt = &now
	n.ETADueAt = &due
	n.Status = store.MedEMSEnRoute
	n.UpdatedAt = now
	list[idx] = n
	m.medical[netID] = list
	m.mu.Unlock()

	if err := m.store.SaveMedicalNotification(n); err != nil {
		return nil, fmt.Errorf("persist medical notification: %w", err)
	}
	unit := n.EMSUnit
	if unit == "" {
		unit = "EMS"
	}
	summary := fmt.Sprintf("%s ETA %d min", unit, in.Minutes)
	m.logTimeline(netID, TLMedicalEMSETA, a.Callsign, summary, "")
	m.logActivity(activity.ActionMedicalNotifUpdated, a, n.ID, medicalActivityDetails(n))
	m.emit(Event{Type: EventMedicalUpdated, Data: n.Redacted()})

	out := n
	return &out, nil
}

// validateBackdatedAt checks an operator-supplied timestamp correction: it
// must not be before the notification was created, and never in the
// future.
func (m *TrafficManager) validateBackdatedAt(n store.MedicalNotification, at *time.Time) (time.Time, error) {
	if at == nil {
		return m.now(), nil
	}
	if at.Before(n.CreatedAt) {
		return time.Time{}, fmt.Errorf("at must not be before the report was created")
	}
	if at.After(m.now()) {
		return time.Time{}, fmt.Errorf("at must not be in the future")
	}
	return *at, nil
}

// MedicalOnScene marks EMS as physically present. Legal directly from
// confirmed (medic already there when notified) or from ems_enroute; ETA is
// optional either way.
func (m *TrafficManager) MedicalOnScene(netID, id string, in OnSceneInput, a Actor) (*store.MedicalNotification, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	list, idx := m.findMedicalLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("medical notification %q not found: %w", id, ErrNotFound)
	}
	n := list[idx]
	if n.Status != store.MedConfirmed && n.Status != store.MedEMSEnRoute {
		m.mu.Unlock()
		return nil, illegalTransition("medical notification has not been read back")
	}
	at, err := m.validateBackdatedAt(n, in.At)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}

	if in.EMSUnit != "" {
		n.EMSUnit = in.EMSUnit
	}
	n.OnSceneAt = &at
	n.Status = store.MedOnScene
	n.UpdatedAt = m.now()
	list[idx] = n
	m.medical[netID] = list
	m.mu.Unlock()

	if err := m.store.SaveMedicalNotification(n); err != nil {
		return nil, fmt.Errorf("persist medical notification: %w", err)
	}
	unit := n.EMSUnit
	if unit == "" {
		unit = "EMS"
	}
	summary := fmt.Sprintf("%s on scene", unit)
	m.logTimeline(netID, TLMedicalOnScene, a.Callsign, summary, "")
	m.logActivity(activity.ActionMedicalNotifUpdated, a, n.ID, medicalActivityDetails(n))
	m.emit(Event{Type: EventMedicalUpdated, Data: n.Redacted()})

	out := n
	return &out, nil
}

// MedicalDepart records EMS's departure with N patients aboard. Requires an
// on-scene timestamp — departure without one would make OnSceneSeconds
// unanswerable, which is exactly the log question the doctrine cares about.
func (m *TrafficManager) MedicalDepart(netID, id string, in DepartInput, a Actor) (*store.MedicalNotification, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}
	destination := strings.ToLower(strings.TrimSpace(in.Destination))
	if !validDestinations[destination] {
		return nil, fmt.Errorf("invalid destination %q", in.Destination)
	}
	if in.PatientCount < 1 {
		return nil, fmt.Errorf("patientCount must be at least 1")
	}
	if strings.TrimSpace(in.PatientName) != "" && !nameAllowedDestinations[destination] {
		return nil, fmt.Errorf("patient name is recorded only for hospital or start transports")
	}

	m.mu.Lock()
	list, idx := m.findMedicalLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("medical notification %q not found: %w", id, ErrNotFound)
	}
	n := list[idx]
	if n.Status != store.MedOnScene {
		m.mu.Unlock()
		return nil, illegalTransition("medical notification must be on scene before it can depart")
	}
	at, err := m.validateBackdatedAt(n, in.At)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if n.OnSceneAt != nil && at.Before(*n.OnSceneAt) {
		m.mu.Unlock()
		return nil, fmt.Errorf("at must not be before the on-scene time")
	}

	onSceneSecs := int(at.Sub(*n.OnSceneAt).Seconds())
	n.Destination = destination
	n.DestinationName = in.DestinationName
	n.PatientCount = in.PatientCount
	if nameAllowedDestinations[destination] {
		n.PatientName = in.PatientName
	}
	n.DepartedAt = &at
	n.OnSceneSeconds = &onSceneSecs
	n.Status = store.MedDeparted
	n.UpdatedAt = m.now()
	list[idx] = n
	m.medical[netID] = list
	m.mu.Unlock()

	if err := m.store.SaveMedicalNotification(n); err != nil {
		return nil, fmt.Errorf("persist medical notification: %w", err)
	}

	unit := n.EMSUnit
	if unit == "" {
		unit = "EMS"
	}
	patientWord := "patient"
	if n.PatientCount != 1 {
		patientWord = "patients"
	}
	summary := fmt.Sprintf("%s departed for %s with %d %s aboard (on scene %s)",
		unit, n.DestinationName, n.PatientCount, patientWord, formatDurationMinutes(time.Duration(onSceneSecs)*time.Second))
	details, _ := json.Marshal(map[string]any{
		"onSceneSeconds": onSceneSecs, "destination": n.Destination, "patientCount": n.PatientCount,
	})
	m.logTimeline(netID, TLMedicalDeparted, a.Callsign, summary, string(details))
	m.logActivity(activity.ActionMedicalNotifUpdated, a, n.ID, medicalActivityDetails(n))
	m.emit(Event{Type: EventMedicalUpdated, Data: n.Redacted()})

	out := n
	return &out, nil
}

// MedicalRelease records a patient treated/refused on scene, no transport.
func (m *TrafficManager) MedicalRelease(netID, id string, in ReleaseInput, a Actor) (*store.MedicalNotification, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	list, idx := m.findMedicalLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("medical notification %q not found: %w", id, ErrNotFound)
	}
	n := list[idx]
	if n.Status != store.MedOnScene {
		m.mu.Unlock()
		return nil, illegalTransition("medical notification must be on scene before it can be released")
	}
	at, err := m.validateBackdatedAt(n, in.At)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}

	n.ReleasedAt = &at
	n.PatientCount = 0
	n.Status = store.MedReleased
	n.UpdatedAt = m.now()
	list[idx] = n
	m.medical[netID] = list
	m.mu.Unlock()

	if err := m.store.SaveMedicalNotification(n); err != nil {
		return nil, fmt.Errorf("persist medical notification: %w", err)
	}
	summary := fmt.Sprintf("Patient released on scene: %s", in.Reason)
	m.logTimeline(netID, TLMedicalReleased, a.Callsign, summary, "")
	m.logActivity(activity.ActionMedicalNotifUpdated, a, n.ID, medicalActivityDetails(n))
	m.emit(Event{Type: EventMedicalUpdated, Data: n.Redacted()})

	out := n
	return &out, nil
}

// CancelMedical cancels a non-terminal notification.
func (m *TrafficManager) CancelMedical(netID, id string, in CancelInput, a Actor) (*store.MedicalNotification, error) {
	if _, err := m.requireOpenNet(netID); err != nil {
		return nil, err
	}

	m.mu.Lock()
	list, idx := m.findMedicalLocked(netID, id)
	if idx == -1 {
		m.mu.Unlock()
		return nil, fmt.Errorf("medical notification %q not found: %w", id, ErrNotFound)
	}
	n := list[idx]
	if terminalMedicalStatuses[n.Status] {
		m.mu.Unlock()
		return nil, illegalTransition(fmt.Sprintf("medical notification is already %s", n.Status))
	}

	now := m.now()
	n.Status = store.MedCancelled
	n.CancelledAt = &now
	n.CancelReason = in.Reason
	n.UpdatedAt = now
	list[idx] = n
	m.medical[netID] = list
	m.mu.Unlock()

	if err := m.store.SaveMedicalNotification(n); err != nil {
		return nil, fmt.Errorf("persist medical notification: %w", err)
	}
	summary := fmt.Sprintf("Medical notification cancelled: %s", in.Reason)
	m.logTimeline(netID, TLMedicalCancelled, a.Callsign, summary, "")
	m.logActivity(activity.ActionMedicalNotifUpdated, a, n.ID, medicalActivityDetails(n))
	m.emit(Event{Type: EventMedicalUpdated, Data: n.Redacted()})

	out := n
	return &out, nil
}
