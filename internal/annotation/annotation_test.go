package annotation

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/store"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("store Init failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return NewManager(s)
}

func TestCreateAnnotation(t *testing.T) {
	mgr := newTestManager(t)

	ann, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Test Point",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ann.ID == "" {
		t.Error("expected non-empty ID")
	}
	if ann.Type != TypePoint {
		t.Errorf("type: got %q, want %q", ann.Type, TypePoint)
	}
	if ann.Label != "Test Point" {
		t.Errorf("label: got %q, want %q", ann.Label, "Test Point")
	}
	if ann.CreatedAt.IsZero() {
		t.Error("createdAt should not be zero")
	}
	if ann.UpdatedAt.IsZero() {
		t.Error("updatedAt should not be zero")
	}
}

func TestCreateAnnotationWithUserAttribution(t *testing.T) {
	mgr := newTestManager(t)

	ann, err := mgr.Create(Annotation{
		Type:          TypePoint,
		Label:         "My Marker",
		Geometry:      `{"type":"Point","coordinates":[0,0]}`,
		CreatedBy:     "user-1",
		CreatedByName: "Alice",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if ann.CreatedBy != "user-1" {
		t.Errorf("createdBy: got %q, want %q", ann.CreatedBy, "user-1")
	}
	if ann.CreatedByName != "Alice" {
		t.Errorf("createdByName: got %q, want %q", ann.CreatedByName, "Alice")
	}
}

func TestCreateAnnotationValidatesGeometryPoint(t *testing.T) {
	mgr := newTestManager(t)

	// Valid point.
	_, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "ok",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
	})
	if err != nil {
		t.Fatalf("valid point should not fail: %v", err)
	}

	// Wrong geometry type for point.
	_, err = mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "bad",
		Geometry: `{"type":"LineString","coordinates":[[0,0],[1,1]]}`,
	})
	if err == nil {
		t.Error("expected error for mismatched geometry type")
	}
}

func TestCreateAnnotationValidatesGeometryLine(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Create(Annotation{
		Type:     TypeLine,
		Label:    "ok",
		Geometry: `{"type":"LineString","coordinates":[[0,0],[1,1]]}`,
	})
	if err != nil {
		t.Fatalf("valid line should not fail: %v", err)
	}

	_, err = mgr.Create(Annotation{
		Type:     TypeLine,
		Label:    "bad",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	if err == nil {
		t.Error("expected error for mismatched geometry type")
	}
}

func TestCreateAnnotationValidatesGeometryArea(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Create(Annotation{
		Type:     TypeArea,
		Label:    "ok",
		Geometry: `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1],[0,0]]]}`,
	})
	if err != nil {
		t.Fatalf("valid area should not fail: %v", err)
	}

	_, err = mgr.Create(Annotation{
		Type:     TypeArea,
		Label:    "bad",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	if err == nil {
		t.Error("expected error for mismatched geometry type")
	}
}

func TestCreateAnnotationInvalidJSON(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "bad",
		Geometry: `not json`,
	})
	if err == nil {
		t.Error("expected error for invalid JSON geometry")
	}
}

func TestUpdateAnnotation(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Original",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})

	ann.Label = "Updated"
	ann.Description = "new description"
	updated, err := mgr.Update(*ann)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Label != "Updated" {
		t.Errorf("label: got %q, want %q", updated.Label, "Updated")
	}
	if updated.Description != "new description" {
		t.Errorf("description: got %q, want %q", updated.Description, "new description")
	}
	if !updated.UpdatedAt.After(ann.CreatedAt) || updated.UpdatedAt.Equal(ann.CreatedAt) {
		// UpdatedAt should be >= CreatedAt (within time resolution).
	}
}

func TestUpdateAnnotationNotFound(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Update(Annotation{ID: "nonexistent", Type: TypePoint, Label: "x", Geometry: `{"type":"Point","coordinates":[0,0]}`})
	if err == nil {
		t.Error("expected error for updating nonexistent annotation")
	}
}

func TestDeleteAnnotation(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "ToDelete",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})

	if err := mgr.Delete(ann.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, ok := mgr.Get(ann.ID)
	if ok {
		t.Error("annotation should not exist after delete")
	}
}

func TestDeleteAnnotationNotFound(t *testing.T) {
	mgr := newTestManager(t)

	err := mgr.Delete("nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent annotation")
	}
}

func TestAllAnnotations(t *testing.T) {
	mgr := newTestManager(t)

	mgr.Create(Annotation{Type: TypePoint, Label: "A", Geometry: `{"type":"Point","coordinates":[0,0]}`})
	mgr.Create(Annotation{Type: TypeLine, Label: "B", Geometry: `{"type":"LineString","coordinates":[[0,0],[1,1]]}`})
	mgr.Create(Annotation{Type: TypeArea, Label: "C", Geometry: `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`})

	all := mgr.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 annotations, got %d", len(all))
	}
}

func TestGetAnnotation(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "FindMe",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})

	got, ok := mgr.Get(ann.ID)
	if !ok {
		t.Fatal("expected annotation to be found")
	}
	if got.Label != "FindMe" {
		t.Errorf("label: got %q, want %q", got.Label, "FindMe")
	}
}

func TestGetAnnotationNotFound(t *testing.T) {
	mgr := newTestManager(t)

	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Error("expected annotation not found")
	}
}

func TestEventsOnCreate(t *testing.T) {
	mgr := newTestManager(t)
	events := mgr.Events()

	mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "EventTest",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})

	select {
	case evt := <-events:
		if evt.Type != EventAnnotationCreated {
			t.Errorf("event type: got %q, want %q", evt.Type, EventAnnotationCreated)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for create event")
	}
}

func TestEventsOnUpdate(t *testing.T) {
	mgr := newTestManager(t)
	events := mgr.Events()

	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Before",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	// Drain create event.
	<-events

	ann.Label = "After"
	mgr.Update(*ann)

	select {
	case evt := <-events:
		if evt.Type != EventAnnotationUpdated {
			t.Errorf("event type: got %q, want %q", evt.Type, EventAnnotationUpdated)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for update event")
	}
}

func TestEventsOnDelete(t *testing.T) {
	mgr := newTestManager(t)
	events := mgr.Events()

	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "ToDelete",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	// Drain create event.
	<-events

	mgr.Delete(ann.ID)

	select {
	case evt := <-events:
		if evt.Type != EventAnnotationDeleted {
			t.Errorf("event type: got %q, want %q", evt.Type, EventAnnotationDeleted)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for delete event")
	}
}

func TestCreateAnnotationEmptyLabel(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	if err == nil {
		t.Error("expected error for empty label")
	}
}

func TestCreateAnnotationEmptyGeometry(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "ok",
		Geometry: "",
	})
	if err == nil {
		t.Error("expected error for empty geometry")
	}
}

func TestCreateAnnotationUnknownType(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Create(Annotation{
		Type:     "hexagon",
		Label:    "ok",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	if err == nil {
		t.Error("expected error for unknown annotation type")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("store Init failed: %v", err)
	}

	// Create annotation directly in store
	mgr1 := NewManager(s)
	if err := mgr1.Load(); err != nil {
		t.Fatalf("load m1: %v", err)
	}
	mgr1.Create(Annotation{
		Type:     TypePoint,
		Label:    "LoadTest",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	s.Close()

	// Reopen and use Load()
	s2 := store.NewSQLiteStore(path)
	if err := s2.Init(); err != nil {
		t.Fatalf("store Init (2) failed: %v", err)
	}
	defer s2.Close()

	mgr2 := NewManager(s2)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("load m2: %v", err)
	}
	if len(mgr2.All()) != 1 {
		t.Fatalf("expected 1, got %d", len(mgr2.All()))
	}
}

func TestPersistenceAcrossReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Create store and manager, add an annotation.
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("store Init failed: %v", err)
	}
	mgr := NewManager(s)
	if err := mgr.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Persisted",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	s.Close()

	// Reopen store and manager, verify annotation is loaded.
	s2 := store.NewSQLiteStore(path)
	if err := s2.Init(); err != nil {
		t.Fatalf("store Init (2) failed: %v", err)
	}
	defer s2.Close()
	mgr2 := NewManager(s2)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("load (2): %v", err)
	}

	all := mgr2.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 persisted annotation, got %d", len(all))
	}
	if all[0].Label != "Persisted" {
		t.Errorf("label: got %q, want %q", all[0].Label, "Persisted")
	}
}
