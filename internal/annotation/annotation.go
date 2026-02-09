package annotation

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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
	CategoryCheckpoint: {"planned": true, "open": true, "closed": true},
	CategoryHazard:     {"reported": true, "confirmed": true, "mitigated": true, "cleared": true},
	CategoryRoute:      {"planned": true, "active": true, "closed": true},
	CategoryBoundary:   {"planned": true, "active": true, "complete": true, "needs-re-search": true},
	CategoryAssignment: {"planned": true, "assigned": true, "in-progress": true, "complete": true, "incomplete": true},
	CategoryGeneral:    {"active": true, "resolved": true},
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
	store       store.Store
	mu          sync.RWMutex
	annotations map[string]Annotation
	events      chan Event
}

// NewManager creates a new annotation Manager backed by the given store.
func NewManager(s store.Store) *Manager {
	return &Manager{
		store:       s,
		annotations: make(map[string]Annotation),
		events:      make(chan Event, 64),
	}
}

// Load loads annotations from the store into the in-memory cache.
func (m *Manager) Load() error {
	loaded, err := m.store.LoadAnnotations()
	if err != nil {
		return fmt.Errorf("load annotations: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range loaded {
		m.annotations[a.ID] = a
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
