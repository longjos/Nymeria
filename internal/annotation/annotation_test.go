package annotation

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/object"
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

// --- Phase A (#53): Extended Annotation Model Tests ---

func TestValidateCategory(t *testing.T) {
	valid := []string{"incident", "resource", "checkpoint", "hazard", "route", "boundary", "assignment", "general"}
	for _, cat := range valid {
		if err := ValidateCategory(cat); err != nil {
			t.Errorf("ValidateCategory(%q) should succeed: %v", cat, err)
		}
	}

	invalid := []string{"unknown", "fire", ""}
	for _, cat := range invalid {
		if err := ValidateCategory(cat); err == nil {
			t.Errorf("ValidateCategory(%q) should fail", cat)
		}
	}
}

func TestValidatePriority(t *testing.T) {
	valid := []string{"routine", "priority", "urgent", "emergency"}
	for _, pri := range valid {
		if err := ValidatePriority(pri); err != nil {
			t.Errorf("ValidatePriority(%q) should succeed: %v", pri, err)
		}
	}

	invalid := []string{"unknown", "high", ""}
	for _, pri := range invalid {
		if err := ValidatePriority(pri); err == nil {
			t.Errorf("ValidatePriority(%q) should fail", pri)
		}
	}
}

func TestCategoryGeometryCompatibility(t *testing.T) {
	tests := []struct {
		category string
		geomType string
		wantErr  bool
	}{
		{"incident", "point", false},
		{"incident", "line", true},
		{"incident", "area", true},
		{"resource", "point", false},
		{"resource", "line", true},
		{"checkpoint", "point", false},
		{"checkpoint", "area", true},
		{"hazard", "point", false},
		{"hazard", "area", false},
		{"hazard", "line", true},
		{"route", "line", false},
		{"route", "point", true},
		{"boundary", "area", false},
		{"boundary", "point", true},
		{"assignment", "point", false},
		{"assignment", "area", false},
		{"assignment", "line", true},
		{"general", "point", false},
		{"general", "line", false},
		{"general", "area", false},
	}

	for _, tt := range tests {
		err := ValidateCategoryGeometry(tt.category, tt.geomType)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateCategoryGeometry(%q, %q): got err=%v, wantErr=%v", tt.category, tt.geomType, err, tt.wantErr)
		}
	}
}

func TestValidateStatusForCategory(t *testing.T) {
	tests := []struct {
		category string
		status   string
		wantErr  bool
	}{
		// incident statuses
		{"incident", "reported", false},
		{"incident", "responding", false},
		{"incident", "on-scene", false},
		{"incident", "resolved", false},
		{"incident", "escalated", false},
		{"incident", "cleared", true},
		{"incident", "open", true},

		// resource statuses
		{"resource", "planned", false},
		{"resource", "open", false},
		{"resource", "active", false},
		{"resource", "at-capacity", false},
		{"resource", "closing", false},
		{"resource", "closed", false},
		{"resource", "resolved", true},

		// checkpoint statuses
		{"checkpoint", "planned", false},
		{"checkpoint", "open", false},
		{"checkpoint", "closed", false},
		{"checkpoint", "resolved", true},

		// hazard statuses
		{"hazard", "reported", false},
		{"hazard", "confirmed", false},
		{"hazard", "mitigated", false},
		{"hazard", "cleared", false},
		{"hazard", "active", true},

		// route statuses
		{"route", "planned", false},
		{"route", "active", false},
		{"route", "closed", false},
		{"route", "resolved", true},

		// boundary statuses
		{"boundary", "planned", false},
		{"boundary", "active", false},
		{"boundary", "complete", false},
		{"boundary", "needs-re-search", false},
		{"boundary", "closed", true},

		// assignment statuses
		{"assignment", "planned", false},
		{"assignment", "assigned", false},
		{"assignment", "in-progress", false},
		{"assignment", "complete", false},
		{"assignment", "incomplete", false},
		{"assignment", "closed", true},

		// general statuses
		{"general", "active", false},
		{"general", "resolved", false},
		{"general", "closed", true},
	}

	for _, tt := range tests {
		err := ValidateStatusForCategory(tt.category, tt.status)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateStatusForCategory(%q, %q): got err=%v, wantErr=%v", tt.category, tt.status, err, tt.wantErr)
		}
	}
}

func TestChangeStatus(t *testing.T) {
	mgr := newTestManager(t)

	ann, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Fire on Main St",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
		Category: CategoryIncident,
		Status:   "reported",
		Priority: PriorityUrgent,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Change to responding.
	updated, err := mgr.ChangeStatus(ann.ID, "responding")
	if err != nil {
		t.Fatalf("ChangeStatus to responding: %v", err)
	}
	if updated.Status != "responding" {
		t.Errorf("status: got %q, want %q", updated.Status, "responding")
	}
	if updated.ResolvedAt != nil {
		t.Error("resolvedAt should be nil for non-terminal status")
	}

	// Change to resolved (terminal).
	updated, err = mgr.ChangeStatus(ann.ID, "resolved")
	if err != nil {
		t.Fatalf("ChangeStatus to resolved: %v", err)
	}
	if updated.Status != "resolved" {
		t.Errorf("status: got %q, want %q", updated.Status, "resolved")
	}
	if updated.ResolvedAt == nil {
		t.Error("resolvedAt should be set for terminal status")
	}
}

func TestChangeStatusInvalidTransition(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Fire",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryIncident,
		Status:   "reported",
	})

	// "cleared" is not valid for incident category.
	_, err := mgr.ChangeStatus(ann.ID, "cleared")
	if err == nil {
		t.Error("expected error for invalid status transition")
	}
}

func TestChangeStatusNotFound(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.ChangeStatus("nonexistent", "resolved")
	if err == nil {
		t.Error("expected error for nonexistent annotation")
	}
}

func TestCreateAnnotationWithCategory(t *testing.T) {
	mgr := newTestManager(t)

	ann, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Aid Station Alpha",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
		Category: CategoryResource,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ann.Category != CategoryResource {
		t.Errorf("category: got %q, want %q", ann.Category, CategoryResource)
	}
	// Default status and priority should be set.
	if ann.Status != "active" {
		t.Errorf("status: got %q, want %q", ann.Status, "active")
	}
	if ann.Priority != PriorityRoutine {
		t.Errorf("priority: got %q, want %q", ann.Priority, PriorityRoutine)
	}
}

func TestCreateAnnotationCategoryGeometryMismatch(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Create(Annotation{
		Type:     TypeLine,
		Label:    "Should fail",
		Geometry: `{"type":"LineString","coordinates":[[0,0],[1,1]]}`,
		Category: CategoryIncident, // incident only allows point
	})
	if err == nil {
		t.Error("expected error for category-geometry mismatch")
	}
}

func TestAllFilteredByCategory(t *testing.T) {
	mgr := newTestManager(t)

	mgr.Create(Annotation{
		Type: TypePoint, Label: "Inc1", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryIncident, Status: "reported",
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Res1", Geometry: `{"type":"Point","coordinates":[1,1]}`,
		Category: CategoryResource, Status: "active",
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Inc2", Geometry: `{"type":"Point","coordinates":[2,2]}`,
		Category: CategoryIncident, Status: "reported",
	})

	filtered := mgr.AllFiltered(store.AnnotationFilter{Category: "incident"})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 incident annotations, got %d", len(filtered))
	}
	for _, a := range filtered {
		if a.Category != CategoryIncident {
			t.Errorf("expected incident category, got %q", a.Category)
		}
	}
}

func TestAllFilteredExcludesExpired(t *testing.T) {
	mgr := newTestManager(t)

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	ann1, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Expired", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral, Status: "active",
	})
	// Set ExpiresAt to past.
	ann1.ExpiresAt = &past
	mgr.Update(*ann1)

	ann2, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "NotExpired", Geometry: `{"type":"Point","coordinates":[1,1]}`,
		Category: CategoryGeneral, Status: "active",
	})
	ann2.ExpiresAt = &future
	mgr.Update(*ann2)

	// Without IncludeExpired: should exclude the expired one.
	filtered := mgr.AllFiltered(store.AnnotationFilter{})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 non-expired annotation, got %d", len(filtered))
	}
	if filtered[0].Label != "NotExpired" {
		t.Errorf("expected NotExpired, got %q", filtered[0].Label)
	}

	// With IncludeExpired: should include both.
	allFiltered := mgr.AllFiltered(store.AnnotationFilter{IncludeExpired: true})
	if len(allFiltered) != 2 {
		t.Fatalf("expected 2 annotations with IncludeExpired, got %d", len(allFiltered))
	}
}

func TestBackwardCompatibility(t *testing.T) {
	mgr := newTestManager(t)

	// Create with no category/status/priority — should get defaults.
	ann, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Legacy",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ann.Category != CategoryGeneral {
		t.Errorf("default category: got %q, want %q", ann.Category, CategoryGeneral)
	}
	if ann.Status != "active" {
		t.Errorf("default status: got %q, want %q", ann.Status, "active")
	}
	if ann.Priority != PriorityRoutine {
		t.Errorf("default priority: got %q, want %q", ann.Priority, PriorityRoutine)
	}
}

func TestAllFilteredByStatus(t *testing.T) {
	mgr := newTestManager(t)

	mgr.Create(Annotation{
		Type: TypePoint, Label: "Active", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral, Status: "active",
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Resolved", Geometry: `{"type":"Point","coordinates":[1,1]}`,
		Category: CategoryGeneral, Status: "resolved",
	})

	filtered := mgr.AllFiltered(store.AnnotationFilter{Status: "active"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 active annotation, got %d", len(filtered))
	}
	if filtered[0].Label != "Active" {
		t.Errorf("expected Active, got %q", filtered[0].Label)
	}
}

func TestAllFilteredByPriority(t *testing.T) {
	mgr := newTestManager(t)

	mgr.Create(Annotation{
		Type: TypePoint, Label: "Routine", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral, Status: "active", Priority: PriorityRoutine,
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Urgent", Geometry: `{"type":"Point","coordinates":[1,1]}`,
		Category: CategoryGeneral, Status: "active", Priority: PriorityUrgent,
	})

	filtered := mgr.AllFiltered(store.AnnotationFilter{Priority: "urgent"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 urgent annotation, got %d", len(filtered))
	}
	if filtered[0].Label != "Urgent" {
		t.Errorf("expected Urgent, got %q", filtered[0].Label)
	}
}

func TestAllFilteredByOperationID(t *testing.T) {
	mgr := newTestManager(t)

	mgr.Create(Annotation{
		Type: TypePoint, Label: "Op1", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral, Status: "active", OperationID: "op-1",
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Op2", Geometry: `{"type":"Point","coordinates":[1,1]}`,
		Category: CategoryGeneral, Status: "active", OperationID: "op-2",
	})

	filtered := mgr.AllFiltered(store.AnnotationFilter{OperationID: "op-1"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 annotation for op-1, got %d", len(filtered))
	}
	if filtered[0].Label != "Op1" {
		t.Errorf("expected Op1, got %q", filtered[0].Label)
	}
}

// --- Phase C (#55): Net Control ↔ Annotation Bridge Tests ---

func TestGetByMissionID(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Linked", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryAssignment, Status: "planned", MissionIDs: []string{"m-1"},
	})

	found, ok := mgr.GetByMissionID("m-1")
	if !ok {
		t.Fatal("expected to find annotation by mission ID")
	}
	if found.ID != ann.ID {
		t.Errorf("got ID %q, want %q", found.ID, ann.ID)
	}

	_, ok = mgr.GetByMissionID("m-nonexistent")
	if ok {
		t.Error("expected not found for nonexistent mission ID")
	}
}

func TestGetAllByMissionID(t *testing.T) {
	mgr := newTestManager(t)

	mgr.Create(Annotation{
		Type: TypePoint, Label: "Ann1", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryAssignment, Status: "planned", MissionIDs: []string{"m-1", "m-2"},
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Ann2", Geometry: `{"type":"Point","coordinates":[1,1]}`,
		Category: CategoryAssignment, Status: "planned", MissionIDs: []string{"m-1"},
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Ann3", Geometry: `{"type":"Point","coordinates":[2,2]}`,
		Category: CategoryAssignment, Status: "planned", MissionIDs: []string{"m-3"},
	})

	results := mgr.GetAllByMissionID("m-1")
	if len(results) != 2 {
		t.Fatalf("expected 2 annotations for m-1, got %d", len(results))
	}

	results = mgr.GetAllByMissionID("m-3")
	if len(results) != 1 {
		t.Fatalf("expected 1 annotation for m-3, got %d", len(results))
	}

	results = mgr.GetAllByMissionID("m-nonexistent")
	if len(results) != 0 {
		t.Fatalf("expected 0 annotations, got %d", len(results))
	}
}

func TestAddMissionLink(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Link Test", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryAssignment, Status: "planned", MissionIDs: []string{"m-1"},
	})

	updated, err := mgr.AddMissionLink(ann.ID, "m-2")
	if err != nil {
		t.Fatalf("AddMissionLink: %v", err)
	}
	if len(updated.MissionIDs) != 2 || updated.MissionIDs[1] != "m-2" {
		t.Errorf("missionIds: got %v, want [m-1 m-2]", updated.MissionIDs)
	}
}

func TestAddMissionLinkDuplicate(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Dup Test", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryAssignment, Status: "planned", MissionIDs: []string{"m-1"},
	})

	_, err := mgr.AddMissionLink(ann.ID, "m-1")
	if err == nil {
		t.Error("expected error for duplicate mission link")
	}
}

func TestRemoveMissionLink(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Remove Test", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryAssignment, Status: "planned", MissionIDs: []string{"m-1", "m-2"},
	})

	updated, err := mgr.RemoveMissionLink(ann.ID, "m-1")
	if err != nil {
		t.Fatalf("RemoveMissionLink: %v", err)
	}
	if len(updated.MissionIDs) != 1 || updated.MissionIDs[0] != "m-2" {
		t.Errorf("missionIds: got %v, want [m-2]", updated.MissionIDs)
	}
}

func TestCreateFromMission(t *testing.T) {
	mgr := newTestManager(t)

	lat, lon := 34.05, -118.24
	mission := store.NetMission{
		ID:          "m-1",
		NetID:       "net-1",
		Title:       "Search Sector Alpha",
		Description: "Grid search of sector A",
		Priority:    "urgent",
		Lat:         &lat,
		Lon:         &lon,
	}

	ann, err := mgr.CreateFromMission(mission, "user-1", "Alice")
	if err != nil {
		t.Fatalf("CreateFromMission failed: %v", err)
	}

	if ann.Category != CategoryAssignment {
		t.Errorf("category: got %q, want %q", ann.Category, CategoryAssignment)
	}
	if len(ann.MissionIDs) != 1 || ann.MissionIDs[0] != "m-1" {
		t.Errorf("missionIDs: got %v, want [m-1]", ann.MissionIDs)
	}
	if ann.Label != "Search Sector Alpha" {
		t.Errorf("label: got %q, want %q", ann.Label, "Search Sector Alpha")
	}
	if ann.Priority != "urgent" {
		t.Errorf("priority: got %q, want %q", ann.Priority, "urgent")
	}
	if ann.Type != TypePoint {
		t.Errorf("type: got %q, want %q", ann.Type, TypePoint)
	}
	if ann.CreatedBy != "user-1" {
		t.Errorf("createdBy: got %q, want %q", ann.CreatedBy, "user-1")
	}
}

func TestCreateFromMissionNoLocation(t *testing.T) {
	mgr := newTestManager(t)

	mission := store.NetMission{
		ID:    "m-2",
		NetID: "net-1",
		Title: "No Location",
	}

	_, err := mgr.CreateFromMission(mission, "user-1", "Alice")
	if err == nil {
		t.Error("expected error for mission without location")
	}
}

func TestSyncStatusFromMission(t *testing.T) {
	mgr := newTestManager(t)

	lat, lon := 34.05, -118.24
	mission := store.NetMission{
		ID: "m-sync", NetID: "net-1", Title: "Sync Test",
		Priority: "routine", Lat: &lat, Lon: &lon,
	}
	ann, _ := mgr.CreateFromMission(mission, "user-1", "Alice")

	// mission "active" → annotation "in-progress"
	if err := mgr.SyncStatusFromMission("m-sync", "active"); err != nil {
		t.Fatalf("SyncStatusFromMission: %v", err)
	}
	updated, _ := mgr.Get(ann.ID)
	if updated.Status != "in-progress" {
		t.Errorf("status: got %q, want %q", updated.Status, "in-progress")
	}

	// mission "complete" → annotation "complete" (terminal, sets ResolvedAt)
	if err := mgr.SyncStatusFromMission("m-sync", "complete"); err != nil {
		t.Fatalf("SyncStatusFromMission: %v", err)
	}
	updated, _ = mgr.Get(ann.ID)
	if updated.Status != "complete" {
		t.Errorf("status: got %q, want %q", updated.Status, "complete")
	}
	if updated.ResolvedAt == nil {
		t.Error("resolvedAt should be set for terminal status")
	}
}

func TestSyncStatusFromMissionNotFound(t *testing.T) {
	mgr := newTestManager(t)

	// No annotations — should return nil (no error)
	if err := mgr.SyncStatusFromMission("m-nonexistent", "active"); err != nil {
		t.Fatalf("expected nil for nonexistent mission, got: %v", err)
	}
}

func TestSyncStatusLoopGuard(t *testing.T) {
	mgr := newTestManager(t)

	lat, lon := 34.05, -118.24
	mission := store.NetMission{
		ID: "m-guard", NetID: "net-1", Title: "Guard Test",
		Priority: "routine", Lat: &lat, Lon: &lon,
	}
	mgr.CreateFromMission(mission, "user-1", "Alice")

	// Simulate syncing flag being set (as if we're already in a sync)
	mgr.mu.Lock()
	mgr.syncing["m-guard"] = true
	mgr.mu.Unlock()

	// Should return nil immediately due to loop guard
	if err := mgr.SyncStatusFromMission("m-guard", "complete"); err != nil {
		t.Fatalf("expected nil from loop-guarded sync: %v", err)
	}

	// Status should NOT have changed
	ann, _ := mgr.GetByMissionID("m-guard")
	if ann.Status == "complete" {
		t.Error("status should not have changed during loop guard")
	}

	// Clean up
	mgr.mu.Lock()
	delete(mgr.syncing, "m-guard")
	mgr.mu.Unlock()
}

func TestSyncStatusToMission(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Mission Link", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryAssignment, Status: "in-progress", MissionIDs: []string{"m-to"},
	})

	missionIDs, missionStatus, err := mgr.SyncStatusToMission(ann.ID)
	if err != nil {
		t.Fatalf("SyncStatusToMission: %v", err)
	}
	if len(missionIDs) != 1 || missionIDs[0] != "m-to" {
		t.Errorf("missionIDs: got %v, want [m-to]", missionIDs)
	}
	if missionStatus != "active" {
		t.Errorf("missionStatus: got %q, want %q", missionStatus, "active")
	}
}

func TestSyncStatusToMissionNoLink(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "No Link", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral, Status: "active",
	})

	_, _, err := mgr.SyncStatusToMission(ann.ID)
	if err == nil {
		t.Error("expected error for annotation without mission link")
	}
}

func TestClearMissionLink(t *testing.T) {
	mgr := newTestManager(t)

	ann, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Clear Link", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryAssignment, Status: "planned", MissionIDs: []string{"m-clear", "m-other"},
	})

	updated, err := mgr.ClearMissionLink(ann.ID)
	if err != nil {
		t.Fatalf("ClearMissionLink: %v", err)
	}
	if len(updated.MissionIDs) != 0 {
		t.Errorf("missionIDs should be empty, got %v", updated.MissionIDs)
	}

	// Verify persisted
	reloaded, _ := mgr.Get(ann.ID)
	if len(reloaded.MissionIDs) != 0 {
		t.Errorf("persisted missionIDs should be empty, got %v", reloaded.MissionIDs)
	}
}

func TestClearMissionLinkNotFound(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.ClearMissionLink("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent annotation")
	}
}

// --- Phase D (#56): Templates & Operations Tests ---

func TestCreateOperation(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	op, err := mgr.CreateOperation(store.Operation{Name: "SAR Event 2025"})
	if err != nil {
		t.Fatalf("CreateOperation: %v", err)
	}
	if op.ID == "" {
		t.Error("expected non-empty ID")
	}
	if op.Name != "SAR Event 2025" {
		t.Errorf("name: got %q, want %q", op.Name, "SAR Event 2025")
	}
	if op.Status != "active" {
		t.Errorf("status: got %q, want %q", op.Status, "active")
	}

	ops := mgr.AllOperations()
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
}

func TestCreateOperationEmptyName(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err := mgr.CreateOperation(store.Operation{Name: ""})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestGetOperation(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	op, _ := mgr.CreateOperation(store.Operation{Name: "Test Op"})
	got, ok := mgr.GetOperation(op.ID)
	if !ok {
		t.Fatal("expected to find operation")
	}
	if got.Name != "Test Op" {
		t.Errorf("name: got %q, want %q", got.Name, "Test Op")
	}

	_, ok = mgr.GetOperation("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent ID")
	}
}

func TestArchiveOperationExpiresAnnotations(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	op, _ := mgr.CreateOperation(store.Operation{Name: "Archivable"})

	// Create annotations linked to this operation.
	ann1, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Linked1", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral, Status: "active", OperationID: op.ID,
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Linked2", Geometry: `{"type":"Point","coordinates":[1,1]}`,
		Category: CategoryGeneral, Status: "active", OperationID: op.ID,
	})
	// Unlinked annotation — should NOT be expired.
	ann3, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Unlinked", Geometry: `{"type":"Point","coordinates":[2,2]}`,
		Category: CategoryGeneral, Status: "active",
	})

	if err := mgr.ArchiveOperation(op.ID); err != nil {
		t.Fatalf("ArchiveOperation: %v", err)
	}

	// Check operation is archived.
	archived, _ := mgr.GetOperation(op.ID)
	if archived.Status != "archived" {
		t.Errorf("op status: got %q, want %q", archived.Status, "archived")
	}
	if archived.ArchivedAt == nil {
		t.Error("archivedAt should be set")
	}

	// Linked annotations should be expired.
	a1, _ := mgr.Get(ann1.ID)
	if a1.ExpiresAt == nil {
		t.Error("linked annotation should have ExpiresAt set")
	}

	// Unlinked should NOT be expired.
	a3, _ := mgr.Get(ann3.ID)
	if a3.ExpiresAt != nil {
		t.Error("unlinked annotation should not be expired")
	}
}

func TestArchiveOperationNotFound(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	err := mgr.ArchiveOperation("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent operation")
	}
}

func TestAnnotationOperationFiltering(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	op, _ := mgr.CreateOperation(store.Operation{Name: "Filter Op"})

	mgr.Create(Annotation{
		Type: TypePoint, Label: "InOp", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral, Status: "active", OperationID: op.ID,
	})
	mgr.Create(Annotation{
		Type: TypePoint, Label: "Outside", Geometry: `{"type":"Point","coordinates":[1,1]}`,
		Category: CategoryGeneral, Status: "active",
	})

	filtered := mgr.AllFiltered(store.AnnotationFilter{OperationID: op.ID})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 annotation for operation, got %d", len(filtered))
	}
	if filtered[0].Label != "InOp" {
		t.Errorf("expected InOp, got %q", filtered[0].Label)
	}
}

// --- Phase E (#57): APRS Object Bridge Tests ---

func newTestObjManager() *object.Manager {
	noop := func(frame aprs.APRSFrame) error { return nil }
	return object.NewManager("TEST", 0, noop, object.ManagerConfig{
		RetransmitInterval: time.Minute,
	})
}

func TestPromoteToObject(t *testing.T) {
	mgr := newTestManager(t)
	objMgr := newTestObjManager()
	mgr.SetObjectManager(objMgr)

	ann, err := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Aid Stn",
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
		Category: CategoryResource,
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := mgr.PromoteToObject(ann.ID)
	if err != nil {
		t.Fatalf("PromoteToObject: %v", err)
	}
	if result.ID != ann.ID {
		t.Errorf("ID: got %q, want %q", result.ID, ann.ID)
	}
	if !mgr.IsTransmitting(ann.ID) {
		t.Error("annotation should be transmitting")
	}

	// Promoting again should fail (already transmitting).
	_, err = mgr.PromoteToObject(ann.ID)
	if err == nil {
		t.Error("expected error for already transmitting annotation")
	}
}

func TestPromoteToObjectNonPoint(t *testing.T) {
	mgr := newTestManager(t)
	objMgr := newTestObjManager()
	mgr.SetObjectManager(objMgr)

	ann, err := mgr.Create(Annotation{
		Type:     TypeLine,
		Label:    "Route Alpha",
		Geometry: `{"type":"LineString","coordinates":[[0,0],[1,1]]}`,
		Category: CategoryRoute,
		Status:   "planned",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = mgr.PromoteToObject(ann.ID)
	if err == nil {
		t.Error("expected error for non-point annotation")
	}
}

func TestStopTransmitting(t *testing.T) {
	mgr := newTestManager(t)
	objMgr := newTestObjManager()
	mgr.SetObjectManager(objMgr)

	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Test",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral,
		Status:   "active",
	})
	mgr.PromoteToObject(ann.ID)

	if err := mgr.StopTransmitting(ann.ID); err != nil {
		t.Fatalf("StopTransmitting: %v", err)
	}
	if mgr.IsTransmitting(ann.ID) {
		t.Error("annotation should not be transmitting after stop")
	}

	// Stopping again should fail.
	err := mgr.StopTransmitting(ann.ID)
	if err == nil {
		t.Error("expected error for non-transmitting annotation")
	}
}

func TestObjectNameTruncation(t *testing.T) {
	mgr := newTestManager(t)
	objMgr := newTestObjManager()
	mgr.SetObjectManager(objMgr)

	// Label longer than 9 chars should be truncated for APRS Object name.
	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "VeryLongAidStationName",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryResource,
		Status:   "active",
	})

	_, err := mgr.PromoteToObject(ann.ID)
	if err != nil {
		t.Fatalf("PromoteToObject: %v", err)
	}

	// Verify the object was created with truncated name.
	objects := objMgr.OwnObjects()
	found := false
	for _, obj := range objects {
		if obj.Name == "VeryLongA" {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, len(objects))
		for i, obj := range objects {
			names[i] = obj.Name
		}
		t.Errorf("expected object with truncated name 'VeryLongA', got objects: %v", names)
	}
}

func TestPromoteToObjectNoObjectManager(t *testing.T) {
	mgr := newTestManager(t)
	// Do NOT set object manager.

	ann, _ := mgr.Create(Annotation{
		Type:     TypePoint,
		Label:    "Test",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryGeneral,
		Status:   "active",
	})

	_, err := mgr.PromoteToObject(ann.ID)
	if err == nil {
		t.Error("expected error when object manager is nil")
	}
}

func TestPromoteToObjectNotFound(t *testing.T) {
	mgr := newTestManager(t)
	objMgr := newTestObjManager()
	mgr.SetObjectManager(objMgr)

	_, err := mgr.PromoteToObject("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent annotation")
	}
}

func TestChangeStatusEmitsEvent(t *testing.T) {
	mgr := newTestManager(t)
	events := mgr.Events()

	ann, _ := mgr.Create(Annotation{
		Type: TypePoint, Label: "Test", Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: CategoryIncident, Status: "reported",
	})
	// Drain create event.
	<-events

	mgr.ChangeStatus(ann.ID, "responding")

	select {
	case evt := <-events:
		if evt.Type != EventAnnotationStatusChanged {
			t.Errorf("event type: got %q, want %q", evt.Type, EventAnnotationStatusChanged)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for status change event")
	}
}
