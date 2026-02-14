package checkpoint

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/store"
)

func newTestManager(t *testing.T) (*Manager, *annotation.Manager, store.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s := store.NewSQLiteStore(path)
	if err := s.Init(); err != nil {
		t.Fatalf("store Init failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	annMgr := annotation.NewManager(s)
	cpMgr := NewManager(s, annMgr)
	return cpMgr, annMgr, s
}

// createCheckpointAnnotation creates a checkpoint annotation for testing.
func createCheckpointAnnotation(t *testing.T, annMgr *annotation.Manager, netID, label string) *store.Annotation {
	t.Helper()
	ann, err := annMgr.Create(store.Annotation{
		Type:     "point",
		Label:    label,
		Geometry: `{"type":"Point","coordinates":[-118.24,34.05]}`,
		Category: "checkpoint",
		Status:   "planned",
		NetID:    netID,
	})
	if err != nil {
		t.Fatalf("create checkpoint annotation %q: %v", label, err)
	}
	return ann
}

func TestSetMeta(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	ann := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")

	meta, err := cpMgr.SetMeta(store.CheckpointMeta{
		AnnotationID:   ann.ID,
		NetID:          "net-1",
		SequenceNumber: 1,
	})
	if err != nil {
		t.Fatalf("SetMeta failed: %v", err)
	}
	if meta.AnnotationID != ann.ID {
		t.Errorf("annotationID mismatch")
	}
	if meta.SequenceNumber != 1 {
		t.Errorf("sequenceNumber: got %d, want 1", meta.SequenceNumber)
	}

	// Verify it was cached.
	cpMgr.mu.RLock()
	cached, ok := cpMgr.metas[ann.ID]
	cpMgr.mu.RUnlock()
	if !ok || cached.SequenceNumber != 1 {
		t.Error("meta not cached properly")
	}
}

func TestSetMeta_InvalidAnnotation(t *testing.T) {
	cpMgr, _, _ := newTestManager(t)

	_, err := cpMgr.SetMeta(store.CheckpointMeta{
		AnnotationID:   "nonexistent",
		NetID:          "net-1",
		SequenceNumber: 1,
	})
	if err == nil {
		t.Error("expected error for nonexistent annotation")
	}
}

func TestSetMeta_NonCheckpointAnnotation(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	// Create a non-checkpoint annotation.
	ann, err := annMgr.Create(store.Annotation{
		Type:     "point",
		Label:    "Not a CP",
		Geometry: `{"type":"Point","coordinates":[0,0]}`,
		Category: "general",
	})
	if err != nil {
		t.Fatalf("create annotation: %v", err)
	}

	_, err = cpMgr.SetMeta(store.CheckpointMeta{
		AnnotationID:   ann.ID,
		NetID:          "net-1",
		SequenceNumber: 1,
	})
	if err == nil {
		t.Error("expected error for non-checkpoint annotation")
	}
}

func TestLogPassage(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	ann := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")
	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann.ID, NetID: "net-1", SequenceNumber: 1})

	passage, err := cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann.ID,
		NetID:        "net-1",
		Label:        "lead",
		Direction:    "through",
		ReportedBy:   "user-1",
	})
	if err != nil {
		t.Fatalf("LogPassage failed: %v", err)
	}
	if passage.ID == "" {
		t.Error("expected non-empty passage ID")
	}
	if passage.Label != "lead" {
		t.Errorf("label: got %q, want %q", passage.Label, "lead")
	}
	if passage.PassageTime.IsZero() {
		t.Error("passage time should be set")
	}
}

func TestLogPassage_EmitsEvent(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	ann := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")

	events := cpMgr.Events()

	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann.ID, NetID: "net-1", SequenceNumber: 1})
	// Drain the meta update event.
	<-events

	cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann.ID,
		NetID:        "net-1",
		Label:        "lead",
		Direction:    "through",
	})

	select {
	case evt := <-events:
		if evt.Type != EventCheckpointPassage {
			t.Errorf("event type: got %q, want %q", evt.Type, EventCheckpointPassage)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for checkpoint passage event")
	}
}

func TestLogPassage_UnknownCheckpoint(t *testing.T) {
	cpMgr, _, _ := newTestManager(t)

	_, err := cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: "nonexistent",
		NetID:        "net-1",
		Label:        "lead",
		Direction:    "through",
	})
	if err == nil {
		t.Error("expected error for unknown checkpoint")
	}
}

func TestGetCheckpointsForNet(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	ann1 := createCheckpointAnnotation(t, annMgr, "net-1", "CP3")
	ann2 := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")
	ann3 := createCheckpointAnnotation(t, annMgr, "net-1", "CP2")

	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann1.ID, NetID: "net-1", SequenceNumber: 3})
	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann2.ID, NetID: "net-1", SequenceNumber: 1})
	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann3.ID, NetID: "net-1", SequenceNumber: 2})

	// Log a passage at CP1.
	cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann2.ID, NetID: "net-1", Label: "lead", Direction: "through",
	})

	checkpoints, err := cpMgr.GetCheckpointsForNet("net-1")
	if err != nil {
		t.Fatalf("GetCheckpointsForNet failed: %v", err)
	}
	if len(checkpoints) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(checkpoints))
	}
	// Should be ordered by sequence number.
	if checkpoints[0].Meta.SequenceNumber != 1 {
		t.Errorf("first checkpoint seq: got %d, want 1", checkpoints[0].Meta.SequenceNumber)
	}
	if checkpoints[1].Meta.SequenceNumber != 2 {
		t.Errorf("second checkpoint seq: got %d, want 2", checkpoints[1].Meta.SequenceNumber)
	}
	if checkpoints[2].Meta.SequenceNumber != 3 {
		t.Errorf("third checkpoint seq: got %d, want 3", checkpoints[2].Meta.SequenceNumber)
	}
	// CP1 should have 1 passage.
	if checkpoints[0].PassageCount != 1 {
		t.Errorf("CP1 passage count: got %d, want 1", checkpoints[0].PassageCount)
	}
}

func TestGetProgress(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	ann1 := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")
	ann2 := createCheckpointAnnotation(t, annMgr, "net-1", "CP2")
	ann3 := createCheckpointAnnotation(t, annMgr, "net-1", "CP3")

	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann1.ID, NetID: "net-1", SequenceNumber: 1})
	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann2.ID, NetID: "net-1", SequenceNumber: 2})
	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann3.ID, NetID: "net-1", SequenceNumber: 3})

	// Lead passes CP1, then CP2.
	cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann1.ID, NetID: "net-1", Label: "lead", Direction: "through",
	})
	cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann2.ID, NetID: "net-1", Label: "lead", Direction: "through",
	})
	// Sweep passes CP1 only.
	cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann1.ID, NetID: "net-1", Label: "sweep", Direction: "through",
	})

	progress, err := cpMgr.GetProgress("net-1")
	if err != nil {
		t.Fatalf("GetProgress failed: %v", err)
	}
	if progress.NetID != "net-1" {
		t.Errorf("netID: got %q", progress.NetID)
	}
	if len(progress.Checkpoints) != 3 {
		t.Errorf("expected 3 checkpoints, got %d", len(progress.Checkpoints))
	}
	if len(progress.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(progress.Elements))
	}

	// Find lead element — should be at CP2 (seq 2).
	var leadFound, sweepFound bool
	for _, elem := range progress.Elements {
		switch elem.Label {
		case "lead":
			leadFound = true
			if elem.LastCheckpointSeq != 2 {
				t.Errorf("lead lastCheckpointSeq: got %d, want 2", elem.LastCheckpointSeq)
			}
		case "sweep":
			sweepFound = true
			if elem.LastCheckpointSeq != 1 {
				t.Errorf("sweep lastCheckpointSeq: got %d, want 1", elem.LastCheckpointSeq)
			}
		}
	}
	if !leadFound {
		t.Error("lead element not found in progress")
	}
	if !sweepFound {
		t.Error("sweep element not found in progress")
	}
}

func TestGetProgress_LatestWins(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	ann1 := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")
	ann2 := createCheckpointAnnotation(t, annMgr, "net-1", "CP2")

	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann1.ID, NetID: "net-1", SequenceNumber: 1})
	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann2.ID, NetID: "net-1", SequenceNumber: 2})

	// Lead passes CP2, then CP1 (unusual backtrack) — highest sequence should win.
	cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann2.ID, NetID: "net-1", Label: "lead", Direction: "through",
	})
	cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann1.ID, NetID: "net-1", Label: "lead", Direction: "through",
	})

	progress, err := cpMgr.GetProgress("net-1")
	if err != nil {
		t.Fatalf("GetProgress failed: %v", err)
	}
	if len(progress.Elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(progress.Elements))
	}
	// The highest sequence checkpoint should win.
	if progress.Elements[0].LastCheckpointSeq != 2 {
		t.Errorf("lead should be at seq 2 (highest), got %d", progress.Elements[0].LastCheckpointSeq)
	}
}

func TestDeleteMetaForAnnotation(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	ann := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")
	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann.ID, NetID: "net-1", SequenceNumber: 1})

	if err := cpMgr.DeleteMetaForAnnotation(ann.ID); err != nil {
		t.Fatalf("DeleteMetaForAnnotation failed: %v", err)
	}

	cpMgr.mu.RLock()
	_, ok := cpMgr.metas[ann.ID]
	cpMgr.mu.RUnlock()
	if ok {
		t.Error("meta should be removed from cache")
	}
}

func TestDeletePassagesForNet(t *testing.T) {
	cpMgr, annMgr, _ := newTestManager(t)

	ann := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")
	cpMgr.SetMeta(store.CheckpointMeta{AnnotationID: ann.ID, NetID: "net-1", SequenceNumber: 1})
	cpMgr.LogPassage(store.CheckpointPassage{
		CheckpointID: ann.ID, NetID: "net-1", Label: "lead", Direction: "through",
	})

	if err := cpMgr.DeletePassagesForNet("net-1"); err != nil {
		t.Fatalf("DeletePassagesForNet failed: %v", err)
	}

	cpMgr.mu.RLock()
	passages := cpMgr.passages["net-1"]
	cpMgr.mu.RUnlock()
	if len(passages) != 0 {
		t.Errorf("expected 0 cached passages after delete, got %d", len(passages))
	}
}

func TestLoad(t *testing.T) {
	_, annMgr, s := newTestManager(t)

	ann := createCheckpointAnnotation(t, annMgr, "net-1", "CP1")

	// Persist data directly to store.
	s.SaveCheckpointMeta(store.CheckpointMeta{AnnotationID: ann.ID, NetID: "net-1", SequenceNumber: 1})
	s.SaveCheckpointPassage(store.CheckpointPassage{
		ID: "p1", CheckpointID: ann.ID, NetID: "net-1", Label: "lead",
		PassageTime: time.Now().UTC(), Direction: "through",
	})

	// Create fresh manager and load.
	cpMgr2 := NewManager(s, annMgr)
	if err := cpMgr2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cpMgr2.mu.RLock()
	meta, ok := cpMgr2.metas[ann.ID]
	passages := cpMgr2.passages["net-1"]
	cpMgr2.mu.RUnlock()

	if !ok || meta.SequenceNumber != 1 {
		t.Error("meta not loaded from store")
	}
	if len(passages) != 1 || passages[0].Label != "lead" {
		t.Error("passages not loaded from store")
	}
}
