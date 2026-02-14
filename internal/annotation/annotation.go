package annotation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/narvel/nymeria/internal/object"
	"github.com/narvel/nymeria/internal/store"
)

// AnnotationType constants.
const (
	TypePoint = "point"
	TypeLine  = "line"
	TypeArea  = "area"
)

// Category constants.
const (
	CategoryIncident   = "incident"
	CategoryResource   = "resource"
	CategoryCheckpoint = "checkpoint"
	CategoryHazard     = "hazard"
	CategoryRoute      = "route"
	CategoryBoundary   = "boundary"
	CategoryAssignment = "assignment"
	CategoryGeneral    = "general"
	CategoryAid        = "aid"
	CategoryStaging    = "staging"
	CategoryShelter    = "shelter"
	CategoryParking    = "parking"
	CategoryStart      = "start"
	CategoryFinish     = "finish"
)

// Priority constants.
const (
	PriorityRoutine   = "routine"
	PriorityPriority  = "priority"
	PriorityUrgent    = "urgent"
	PriorityEmergency = "emergency"
)

// Event types.
const (
	EventAnnotationCreated       = "annotation_created"
	EventAnnotationUpdated       = "annotation_updated"
	EventAnnotationDeleted       = "annotation_deleted"
	EventAnnotationStatusChanged = "annotation_status_changed"
)

// validCategories is the set of allowed category values.
var validCategories = map[string]bool{
	CategoryIncident:   true,
	CategoryResource:   true,
	CategoryCheckpoint: true,
	CategoryHazard:     true,
	CategoryRoute:      true,
	CategoryBoundary:   true,
	CategoryAssignment: true,
	CategoryGeneral:    true,
	CategoryAid:        true,
	CategoryStaging:    true,
	CategoryShelter:    true,
	CategoryParking:    true,
	CategoryStart:      true,
	CategoryFinish:     true,
}

// validPriorities is the set of allowed priority values.
var validPriorities = map[string]bool{
	PriorityRoutine:   true,
	PriorityPriority:  true,
	PriorityUrgent:    true,
	PriorityEmergency: true,
}

// categoryStatuses maps each category to its valid statuses.
var categoryStatuses = map[string]map[string]bool{
	CategoryIncident:   {"reported": true, "responding": true, "on-scene": true, "resolved": true, "escalated": true},
	CategoryResource:   {"planned": true, "open": true, "active": true, "at-capacity": true, "closing": true, "closed": true},
	CategoryCheckpoint: {"planned": true, "open": true, "active": true, "closed": true},
	CategoryHazard:     {"reported": true, "confirmed": true, "mitigated": true, "cleared": true},
	CategoryRoute:      {"planned": true, "active": true, "closed": true},
	CategoryBoundary:   {"planned": true, "active": true, "complete": true, "needs-re-search": true},
	CategoryAssignment: {"planned": true, "assigned": true, "in-progress": true, "complete": true, "incomplete": true},
	CategoryGeneral:    {"active": true, "resolved": true},
	CategoryAid:        {"planned": true, "active": true, "closed": true},
	CategoryStaging:    {"planned": true, "active": true, "closed": true},
	CategoryShelter:    {"planned": true, "active": true, "closed": true},
	CategoryParking:    {"planned": true, "active": true, "closed": true},
	CategoryStart:      {"planned": true, "active": true, "closed": true},
	CategoryFinish:     {"planned": true, "active": true, "closed": true},
}

// categoryGeometry maps each category to allowed geometry types.
var categoryGeometry = map[string]map[string]bool{
	CategoryIncident:   {TypePoint: true},
	CategoryResource:   {TypePoint: true},
	CategoryCheckpoint: {TypePoint: true},
	CategoryHazard:     {TypePoint: true, TypeArea: true},
	CategoryRoute:      {TypeLine: true},
	CategoryBoundary:   {TypeArea: true},
	CategoryAssignment: {TypePoint: true, TypeArea: true},
	CategoryGeneral:    {TypePoint: true, TypeLine: true, TypeArea: true},
	CategoryAid:        {TypePoint: true},
	CategoryStaging:    {TypePoint: true},
	CategoryShelter:    {TypePoint: true},
	CategoryParking:    {TypePoint: true},
	CategoryStart:      {TypePoint: true},
	CategoryFinish:     {TypePoint: true},
}

// terminalStatuses are statuses that auto-set ResolvedAt.
var terminalStatuses = map[string]bool{
	"resolved":  true,
	"closed":    true,
	"cleared":   true,
	"complete":  true,
	"escalated": true,
}

// defaultStatus returns the first valid status for a category.
var defaultStatus = map[string]string{
	CategoryIncident:   "reported",
	CategoryResource:   "active",
	CategoryCheckpoint: "planned",
	CategoryHazard:     "reported",
	CategoryRoute:      "planned",
	CategoryBoundary:   "planned",
	CategoryAssignment: "planned",
	CategoryGeneral:    "active",
	CategoryAid:        "planned",
	CategoryStaging:    "planned",
	CategoryShelter:    "planned",
	CategoryParking:    "planned",
	CategoryStart:      "planned",
	CategoryFinish:     "planned",
}

// Annotation represents a local map annotation.
type Annotation = store.Annotation

// Style represents annotation styling options.
type Style struct {
	Color       string  `json:"color,omitempty"`
	Opacity     float64 `json:"opacity,omitempty"`
	Weight      int     `json:"weight,omitempty"`
	FillColor   string  `json:"fillColor,omitempty"`
	FillOpacity float64 `json:"fillOpacity,omitempty"`
	Icon        string  `json:"icon,omitempty"`
}

// Event represents an annotation event for WebSocket broadcast.
type Event struct {
	Type string          `json:"type"`
	Data store.Annotation `json:"data"`
}

// Manager manages annotations with persistence and events.
type Manager struct {
	store        store.Store
	mu           sync.RWMutex
	annotations  map[string]Annotation
	operations   map[string]store.Operation
	events       chan Event
	syncing      map[string]bool // loop guard for mission↔annotation status sync
	objMgr       *object.Manager
	transmitting map[string]string // annID → objID
}

// NewManager creates a new annotation Manager backed by the given store.
func NewManager(s store.Store) *Manager {
	return &Manager{
		store:        s,
		annotations:  make(map[string]Annotation),
		operations:   make(map[string]store.Operation),
		events:       make(chan Event, 64),
		syncing:      make(map[string]bool),
		transmitting: make(map[string]string),
	}
}

// Load loads annotations and operations from the store into the in-memory cache.
func (m *Manager) Load() error {
	loaded, err := m.store.LoadAnnotations()
	if err != nil {
		return fmt.Errorf("load annotations: %w", err)
	}

	ops, err := m.store.LoadOperations()
	if err != nil {
		return fmt.Errorf("load operations: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range loaded {
		m.annotations[a.ID] = a
	}
	for _, op := range ops {
		m.operations[op.ID] = op
	}
	return nil
}

// Create creates a new annotation, validates it, persists it, and emits an event.
func (m *Manager) Create(ann Annotation) (*Annotation, error) {
	// Apply defaults for category/status/priority.
	if ann.Category == "" {
		ann.Category = CategoryGeneral
	}
	if ann.Status == "" {
		if ds, ok := defaultStatus[ann.Category]; ok {
			ann.Status = ds
		} else {
			ann.Status = "active"
		}
	}
	if ann.Priority == "" {
		ann.Priority = PriorityRoutine
	}

	if err := validate(ann); err != nil {
		return nil, err
	}

	ann.ID = uuid.New().String()
	now := time.Now().UTC()
	ann.CreatedAt = now
	ann.UpdatedAt = now

	if err := m.store.SaveAnnotation(ann); err != nil {
		return nil, fmt.Errorf("persist annotation: %w", err)
	}

	m.mu.Lock()
	m.annotations[ann.ID] = ann
	m.mu.Unlock()

	m.emit(Event{Type: EventAnnotationCreated, Data: ann})

	return &ann, nil
}

// Update updates an existing annotation, persists it, and emits an event.
func (m *Manager) Update(ann Annotation) (*Annotation, error) {
	m.mu.RLock()
	existing, exists := m.annotations[ann.ID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("annotation %q not found", ann.ID)
	}

	if err := validate(ann); err != nil {
		return nil, err
	}

	ann.CreatedAt = existing.CreatedAt
	ann.UpdatedAt = time.Now().UTC()

	if err := m.store.SaveAnnotation(ann); err != nil {
		return nil, fmt.Errorf("persist annotation: %w", err)
	}

	m.mu.Lock()
	m.annotations[ann.ID] = ann
	m.mu.Unlock()

	m.emit(Event{Type: EventAnnotationUpdated, Data: ann})

	return &ann, nil
}

// Delete removes an annotation, persists the change, and emits an event.
func (m *Manager) Delete(id string) error {
	m.mu.RLock()
	ann, exists := m.annotations[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("annotation %q not found", id)
	}

	if err := m.store.DeleteAnnotation(id); err != nil {
		return fmt.Errorf("delete annotation: %w", err)
	}

	m.mu.Lock()
	delete(m.annotations, id)
	m.mu.Unlock()

	m.emit(Event{Type: EventAnnotationDeleted, Data: ann})

	return nil
}

// All returns all annotations.
func (m *Manager) All() []Annotation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Annotation, 0, len(m.annotations))
	for _, a := range m.annotations {
		result = append(result, a)
	}
	return result
}

// AllForNet returns annotations for a specific net, sorted by SortOrder.
func (m *Manager) AllForNet(netID string) []Annotation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Annotation
	for _, a := range m.annotations {
		if a.NetID == netID {
			result = append(result, a)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SortOrder < result[j].SortOrder
	})
	return result
}

// CloseNetAnnotations moves all non-terminal net annotations to their terminal status.
func (m *Manager) CloseNetAnnotations(netID string) error {
	m.mu.RLock()
	var toClose []Annotation
	for _, a := range m.annotations {
		if a.NetID == netID && !terminalStatuses[a.Status] {
			toClose = append(toClose, a)
		}
	}
	m.mu.RUnlock()

	for _, a := range toClose {
		// Determine the terminal status for this category.
		terminal := "closed"
		if categoryStatuses[a.Category] != nil {
			if categoryStatuses[a.Category]["closed"] {
				terminal = "closed"
			} else if categoryStatuses[a.Category]["resolved"] {
				terminal = "resolved"
			} else if categoryStatuses[a.Category]["complete"] {
				terminal = "complete"
			} else if categoryStatuses[a.Category]["cleared"] {
				terminal = "cleared"
			}
		}
		m.ChangeStatus(a.ID, terminal)
	}
	return nil
}

// ImportItem represents a parsed GPX/KML waypoint for import.
type ImportItem struct {
	Name        string
	ShortName   string
	Description string
	Lat         float64
	Lon         float64
	Category    string
}

// ImportAnnotations bulk-creates annotations from parsed GPX/KML items.
func (m *Manager) ImportAnnotations(netID string, items []ImportItem) ([]Annotation, error) {
	var created []Annotation
	for i, item := range items {
		cat := item.Category
		if cat == "" {
			cat = CategoryGeneral
		}
		geom := fmt.Sprintf(`{"type":"Point","coordinates":[%f,%f]}`, item.Lon, item.Lat)
		ann := Annotation{
			Type:      TypePoint,
			Label:     item.Name,
			ShortName: item.ShortName,
			Description: item.Description,
			Geometry:  geom,
			Category:  cat,
			NetID:     netID,
			SortOrder: i,
		}
		result, err := m.Create(ann)
		if err != nil {
			return created, fmt.Errorf("import item %q: %w", item.Name, err)
		}
		created = append(created, *result)
	}
	return created, nil
}

// CopyAnnotationsFromNet clones annotations from one net to another with new IDs.
func (m *Manager) CopyAnnotationsFromNet(sourceNetID, targetNetID string) ([]Annotation, error) {
	source := m.AllForNet(sourceNetID)

	var created []Annotation
	for _, a := range source {
		ann := Annotation{
			Type:        a.Type,
			Label:       a.Label,
			ShortName:   a.ShortName,
			Description: a.Description,
			Geometry:    a.Geometry,
			Style:       a.Style,
			Category:    a.Category,
			Priority:    a.Priority,
			NetID:       targetNetID,
			SortOrder:   a.SortOrder,
		}
		result, err := m.Create(ann)
		if err != nil {
			return created, fmt.Errorf("copy annotation %q: %w", a.Label, err)
		}
		created = append(created, *result)
	}
	return created, nil
}

// AllFiltered returns annotations matching the given filter from the in-memory cache.
func (m *Manager) AllFiltered(filter store.AnnotationFilter) []Annotation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	result := make([]Annotation, 0)
	for _, a := range m.annotations {
		if filter.Category != "" && a.Category != filter.Category {
			continue
		}
		if filter.Status != "" && a.Status != filter.Status {
			continue
		}
		if filter.Priority != "" && a.Priority != filter.Priority {
			continue
		}
		if filter.OperationID != "" && a.OperationID != filter.OperationID {
			continue
		}
		if !filter.IncludeExpired && a.ExpiresAt != nil && a.ExpiresAt.Before(now) {
			continue
		}
		result = append(result, a)
	}
	return result
}

// Get returns an annotation by ID.
func (m *Manager) Get(id string) (*Annotation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	a, ok := m.annotations[id]
	if !ok {
		return nil, false
	}
	return &a, true
}

// ChangeStatus changes an annotation's status, validates it, and emits an event.
func (m *Manager) ChangeStatus(id, newStatus string) (*Annotation, error) {
	m.mu.RLock()
	ann, exists := m.annotations[id]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("annotation %q not found", id)
	}

	if err := ValidateStatusForCategory(ann.Category, newStatus); err != nil {
		return nil, err
	}

	ann.Status = newStatus
	ann.UpdatedAt = time.Now().UTC()

	// Auto-set ResolvedAt on terminal statuses.
	if terminalStatuses[newStatus] && ann.ResolvedAt == nil {
		now := time.Now().UTC()
		ann.ResolvedAt = &now
	}

	if err := m.store.SaveAnnotation(ann); err != nil {
		return nil, fmt.Errorf("persist annotation: %w", err)
	}

	m.mu.Lock()
	m.annotations[id] = ann
	m.mu.Unlock()

	m.emit(Event{Type: EventAnnotationStatusChanged, Data: ann})

	return &ann, nil
}

// GetByMissionID returns the first annotation linked to a mission.
func (m *Manager) GetByMissionID(missionID string) (*Annotation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, a := range m.annotations {
		for _, mid := range a.MissionIDs {
			if mid == missionID {
				return &a, true
			}
		}
	}
	return nil, false
}

// GetAllByMissionID returns all annotations linked to a mission.
func (m *Manager) GetAllByMissionID(missionID string) []Annotation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Annotation
	for _, a := range m.annotations {
		for _, mid := range a.MissionIDs {
			if mid == missionID {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

// AddMissionLink adds a mission ID to an annotation's MissionIDs (rejects duplicates).
func (m *Manager) AddMissionLink(id, missionID string) (*Annotation, error) {
	m.mu.RLock()
	ann, exists := m.annotations[id]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("annotation %q not found", id)
	}

	for _, mid := range ann.MissionIDs {
		if mid == missionID {
			return nil, fmt.Errorf("annotation already linked to mission %q", missionID)
		}
	}

	ann.MissionIDs = append(ann.MissionIDs, missionID)
	ann.UpdatedAt = time.Now().UTC()

	if err := m.store.SaveAnnotation(ann); err != nil {
		return nil, fmt.Errorf("persist annotation: %w", err)
	}

	m.mu.Lock()
	m.annotations[id] = ann
	m.mu.Unlock()

	m.emit(Event{Type: EventAnnotationUpdated, Data: ann})
	return &ann, nil
}

// RemoveMissionLink removes a specific mission ID from an annotation's MissionIDs.
func (m *Manager) RemoveMissionLink(id, missionID string) (*Annotation, error) {
	m.mu.RLock()
	ann, exists := m.annotations[id]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("annotation %q not found", id)
	}

	newIDs := make([]string, 0, len(ann.MissionIDs))
	removed := false
	for _, mid := range ann.MissionIDs {
		if mid == missionID {
			removed = true
		} else {
			newIDs = append(newIDs, mid)
		}
	}
	if !removed {
		return nil, fmt.Errorf("annotation not linked to mission %q", missionID)
	}

	ann.MissionIDs = newIDs
	ann.UpdatedAt = time.Now().UTC()

	if err := m.store.SaveAnnotation(ann); err != nil {
		return nil, fmt.Errorf("persist annotation: %w", err)
	}

	m.mu.Lock()
	m.annotations[id] = ann
	m.mu.Unlock()

	m.emit(Event{Type: EventAnnotationUpdated, Data: ann})
	return &ann, nil
}

// CreateFromMission creates an annotation from a net mission's location data.
func (m *Manager) CreateFromMission(mission store.NetMission, createdBy, createdByName string) (*Annotation, error) {
	if mission.Lat == nil || mission.Lon == nil {
		return nil, fmt.Errorf("mission has no location")
	}

	geom := fmt.Sprintf(`{"type":"Point","coordinates":[%f,%f]}`, *mission.Lon, *mission.Lat)

	return m.Create(Annotation{
		Type:          TypePoint,
		Label:         mission.Title,
		Description:   mission.Description,
		Geometry:      geom,
		Category:      CategoryAssignment,
		Priority:      mission.Priority,
		MissionIDs:    []string{mission.ID},
		CreatedBy:     createdBy,
		CreatedByName: createdByName,
	})
}

// missionToAnnotationStatus maps net mission status to annotation status.
var missionToAnnotationStatus = map[string]string{
	"open":     "planned",
	"active":   "in-progress",
	"complete": "complete",
}

// annotationToMissionStatus maps annotation status to net mission status.
var annotationToMissionStatus = map[string]string{
	"planned":     "open",
	"assigned":    "active",
	"in-progress": "active",
	"complete":    "complete",
	"incomplete":  "complete",
	"resolved":    "complete",
	"closed":      "complete",
	"cleared":     "complete",
}

// SyncStatusFromMission updates an annotation's status when its linked mission changes.
func (m *Manager) SyncStatusFromMission(missionID, missionStatus string) error {
	m.mu.RLock()
	syncing := m.syncing[missionID]
	m.mu.RUnlock()
	if syncing {
		return nil // loop guard
	}

	ann, found := m.GetByMissionID(missionID)
	if !found {
		return nil
	}

	newStatus, ok := missionToAnnotationStatus[missionStatus]
	if !ok {
		return nil
	}
	if ann.Status == newStatus {
		return nil
	}

	// Verify the status is valid for this category.
	if err := ValidateStatusForCategory(ann.Category, newStatus); err != nil {
		return nil // silently skip if invalid
	}

	m.mu.Lock()
	m.syncing[missionID] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.syncing, missionID)
		m.mu.Unlock()
	}()

	_, err := m.ChangeStatus(ann.ID, newStatus)
	return err
}

// SyncStatusToMission returns the mapped mission status for an annotation's current status.
// Returns all linked mission IDs and the mapped status.
func (m *Manager) SyncStatusToMission(annID string) (missionIDs []string, missionStatus string, err error) {
	ann, found := m.Get(annID)
	if !found {
		return nil, "", fmt.Errorf("annotation %q not found", annID)
	}
	if len(ann.MissionIDs) == 0 {
		return nil, "", fmt.Errorf("annotation has no linked mission")
	}

	mapped, ok := annotationToMissionStatus[ann.Status]
	if !ok {
		mapped = "open"
	}
	return ann.MissionIDs, mapped, nil
}

// ClearMissionLink removes all mission links from an annotation.
func (m *Manager) ClearMissionLink(id string) (*Annotation, error) {
	m.mu.RLock()
	ann, exists := m.annotations[id]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("annotation %q not found", id)
	}

	ann.MissionIDs = nil
	ann.UpdatedAt = time.Now().UTC()

	if err := m.store.SaveAnnotation(ann); err != nil {
		return nil, fmt.Errorf("persist annotation: %w", err)
	}

	m.mu.Lock()
	m.annotations[id] = ann
	m.mu.Unlock()

	m.emit(Event{Type: EventAnnotationUpdated, Data: ann})
	return &ann, nil
}

// --- Operation management ---

// CreateOperation creates a new operation.
func (m *Manager) CreateOperation(op store.Operation) (*store.Operation, error) {
	if strings.TrimSpace(op.Name) == "" {
		return nil, fmt.Errorf("operation name is required")
	}

	op.ID = uuid.New().String()
	now := time.Now().UTC()
	op.CreatedAt = now
	if op.Status == "" {
		op.Status = "active"
	}

	if err := m.store.SaveOperation(op); err != nil {
		return nil, fmt.Errorf("persist operation: %w", err)
	}

	m.mu.Lock()
	m.operations[op.ID] = op
	m.mu.Unlock()

	return &op, nil
}

// AllOperations returns all operations.
func (m *Manager) AllOperations() []store.Operation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]store.Operation, 0, len(m.operations))
	for _, op := range m.operations {
		result = append(result, op)
	}
	return result
}

// GetOperation returns an operation by ID.
func (m *Manager) GetOperation(id string) (*store.Operation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	op, ok := m.operations[id]
	if !ok {
		return nil, false
	}
	return &op, true
}

// ArchiveOperation archives an operation and expires all linked annotations.
func (m *Manager) ArchiveOperation(id string) error {
	m.mu.RLock()
	op, ok := m.operations[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("operation %q not found", id)
	}

	now := time.Now().UTC()
	op.Status = "archived"
	op.ArchivedAt = &now

	if err := m.store.SaveOperation(op); err != nil {
		return fmt.Errorf("persist operation: %w", err)
	}

	m.mu.Lock()
	m.operations[id] = op

	// Expire all annotations linked to this operation.
	for annID, ann := range m.annotations {
		if ann.OperationID == id && ann.ExpiresAt == nil {
			ann.ExpiresAt = &now
			m.annotations[annID] = ann
			m.store.SaveAnnotation(ann)
		}
	}
	m.mu.Unlock()

	return nil
}

// --- APRS Object Bridge ---

// SetObjectManager sets the object manager for APRS object bridging.
func (m *Manager) SetObjectManager(om *object.Manager) {
	m.objMgr = om
}

// geojsonPoint is used for extracting coordinates from GeoJSON Point geometry.
type geojsonPoint struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

// PromoteToObject transmits an annotation as an APRS Object.
func (m *Manager) PromoteToObject(id string) (*Annotation, error) {
	if m.objMgr == nil {
		return nil, fmt.Errorf("object manager not available")
	}

	m.mu.RLock()
	ann, exists := m.annotations[id]
	_, alreadyTx := m.transmitting[id]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("annotation %q not found", id)
	}
	if alreadyTx {
		return nil, fmt.Errorf("annotation is already being transmitted")
	}
	if ann.Type != TypePoint {
		return nil, fmt.Errorf("only point annotations can be transmitted as APRS objects")
	}

	// Extract lat/lon from GeoJSON.
	var pt geojsonPoint
	if err := json.Unmarshal([]byte(ann.Geometry), &pt); err != nil || len(pt.Coordinates) < 2 {
		return nil, fmt.Errorf("cannot extract coordinates from geometry")
	}
	lon, lat := pt.Coordinates[0], pt.Coordinates[1]

	// Truncate label to 9 chars for APRS Object name.
	name := ann.Label
	if len(name) > 9 {
		name = name[:9]
	}

	sym := SymbolForCategory(ann.Category)

	obj, err := m.objMgr.CreateObject(object.Object{
		Name:    name,
		Lat:     lat,
		Lon:     lon,
		Symbol:  sym,
		Comment: ann.Description,
		Live:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("create APRS object: %w", err)
	}

	m.mu.Lock()
	m.transmitting[id] = obj.ID
	m.mu.Unlock()

	return &ann, nil
}

// StopTransmitting kills the APRS Object for an annotation.
func (m *Manager) StopTransmitting(id string) error {
	if m.objMgr == nil {
		return fmt.Errorf("object manager not available")
	}

	m.mu.RLock()
	objID, ok := m.transmitting[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("annotation %q is not being transmitted", id)
	}

	if err := m.objMgr.KillObject(objID); err != nil {
		return fmt.Errorf("kill APRS object: %w", err)
	}

	m.mu.Lock()
	delete(m.transmitting, id)
	m.mu.Unlock()

	return nil
}

// IsTransmitting returns whether an annotation is currently being transmitted.
func (m *Manager) IsTransmitting(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.transmitting[id]
	return ok
}

// Events returns the events channel for WebSocket broadcast.
func (m *Manager) Events() <-chan Event {
	return m.events
}

func (m *Manager) emit(evt Event) {
	select {
	case m.events <- evt:
	default:
	}
}

// --- Validation ---

// ValidateCategory checks that the category is valid.
func ValidateCategory(cat string) error {
	if !validCategories[cat] {
		return fmt.Errorf("unknown category: %q", cat)
	}
	return nil
}

// ValidatePriority checks that the priority is valid.
func ValidatePriority(pri string) error {
	if !validPriorities[pri] {
		return fmt.Errorf("unknown priority: %q", pri)
	}
	return nil
}

// ValidateStatusForCategory checks that the status is valid for the given category.
func ValidateStatusForCategory(cat, status string) error {
	statuses, ok := categoryStatuses[cat]
	if !ok {
		return fmt.Errorf("unknown category: %q", cat)
	}
	if !statuses[status] {
		return fmt.Errorf("status %q is not valid for category %q", status, cat)
	}
	return nil
}

// ValidateCategoryGeometry checks that the geometry type is valid for the category.
func ValidateCategoryGeometry(cat, geomType string) error {
	allowed, ok := categoryGeometry[cat]
	if !ok {
		return fmt.Errorf("unknown category: %q", cat)
	}
	if !allowed[geomType] {
		return fmt.Errorf("geometry type %q is not valid for category %q", geomType, cat)
	}
	return nil
}

// geojsonGeometry is used for validating GeoJSON geometry type.
type geojsonGeometry struct {
	Type string `json:"type"`
}

// expectedGeometryType maps annotation types to GeoJSON geometry types.
var expectedGeometryType = map[string]string{
	TypePoint: "Point",
	TypeLine:  "LineString",
	TypeArea:  "Polygon",
}

// validate checks that the annotation has a valid type, non-empty label,
// and valid GeoJSON geometry matching the annotation type.
func validate(ann Annotation) error {
	if strings.TrimSpace(ann.Label) == "" {
		return fmt.Errorf("label is required")
	}

	if strings.TrimSpace(ann.Geometry) == "" {
		return fmt.Errorf("geometry is required")
	}

	expected, ok := expectedGeometryType[ann.Type]
	if !ok {
		return fmt.Errorf("unknown annotation type: %q", ann.Type)
	}

	var g geojsonGeometry
	if err := json.Unmarshal([]byte(ann.Geometry), &g); err != nil {
		return fmt.Errorf("invalid geometry JSON: %w", err)
	}

	if g.Type != expected {
		return fmt.Errorf("geometry type %q does not match annotation type %q (expected %q)", g.Type, ann.Type, expected)
	}

	// Validate category-geometry compatibility when category is set and not general.
	if ann.Category != "" && ann.Category != CategoryGeneral {
		if err := ValidateCategoryGeometry(ann.Category, ann.Type); err != nil {
			return err
		}
	}

	return nil
}
