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

// Event types.
const (
	EventAnnotationCreated = "annotation_created"
	EventAnnotationUpdated = "annotation_updated"
	EventAnnotationDeleted = "annotation_deleted"
)

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

	return nil
}
