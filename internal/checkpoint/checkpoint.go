package checkpoint

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/narvel/nymeria/internal/annotation"
	"github.com/narvel/nymeria/internal/store"
)

// Event types.
const (
	EventCheckpointPassage    = "checkpoint_passage"
	EventCheckpointMetaUpdate = "checkpoint_meta_updated"
)

// Event represents a checkpoint event for WebSocket broadcast.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// CheckpointWithPassages joins a checkpoint annotation with its metadata and passages.
type CheckpointWithPassages struct {
	Annotation    store.Annotation          `json:"annotation"`
	Meta          store.CheckpointMeta      `json:"meta"`
	Passages      []store.CheckpointPassage `json:"passages"`
	PassageCount  int                       `json:"passageCount"`
	LatestPassage *time.Time                `json:"latestPassage,omitempty"`
}

// ProgressElement represents a tracked element's latest position along the route.
type ProgressElement struct {
	Label             string    `json:"label"`
	LastCheckpointID  string    `json:"lastCheckpointId"`
	LastCheckpointSeq int       `json:"lastCheckpointSeq"`
	LastPassageTime   time.Time `json:"lastPassageTime"`
}

// CheckpointProgress is the full progress view for a net.
type CheckpointProgress struct {
	NetID       string                 `json:"netId"`
	Checkpoints []CheckpointWithPassages `json:"checkpoints"`
	Elements    []ProgressElement      `json:"elements"`
}

// Manager manages checkpoint metadata and passage tracking for linear events.
type Manager struct {
	store  store.Store
	annMgr *annotation.Manager
	mu     sync.RWMutex
	metas  map[string]store.CheckpointMeta      // annotationID → meta
	passages map[string][]store.CheckpointPassage // netID → passages
	events chan Event
}

// NewManager creates a new checkpoint Manager.
func NewManager(s store.Store, am *annotation.Manager) *Manager {
	return &Manager{
		store:    s,
		annMgr:   am,
		metas:    make(map[string]store.CheckpointMeta),
		passages: make(map[string][]store.CheckpointPassage),
		events:   make(chan Event, 64),
	}
}

// Load hydrates metas and passages from the store.
func (m *Manager) Load() error {
	// Collect net IDs from both active nets and net-scoped checkpoint annotations.
	netIDs := make(map[string]bool)

	nets, err := m.store.LoadNets()
	if err != nil {
		return fmt.Errorf("load nets for checkpoints: %w", err)
	}
	for _, n := range nets {
		netIDs[n.ID] = true
	}

	// Also check annotations for net IDs (checkpoint annotations may exist without a net record in tests).
	for _, ann := range m.annMgr.All() {
		if ann.Category == "checkpoint" && ann.NetID != "" {
			netIDs[ann.NetID] = true
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for netID := range netIDs {
		metas, err := m.store.LoadCheckpointMeta(netID)
		if err != nil {
			return fmt.Errorf("load checkpoint meta for net %s: %w", netID, err)
		}
		for _, meta := range metas {
			m.metas[meta.AnnotationID] = meta
		}

		passages, err := m.store.LoadCheckpointPassages(netID)
		if err != nil {
			return fmt.Errorf("load checkpoint passages for net %s: %w", netID, err)
		}
		if len(passages) > 0 {
			m.passages[netID] = passages
		}
	}

	return nil
}

// Events returns the events channel for WebSocket broadcast.
func (m *Manager) Events() <-chan Event {
	return m.events
}

// SetMeta sets or updates checkpoint metadata for an annotation.
func (m *Manager) SetMeta(meta store.CheckpointMeta) (*store.CheckpointMeta, error) {
	// Validate annotation exists and is a checkpoint.
	ann, ok := m.annMgr.Get(meta.AnnotationID)
	if !ok {
		return nil, fmt.Errorf("annotation %q not found", meta.AnnotationID)
	}
	if ann.Category != annotation.CategoryCheckpoint {
		return nil, fmt.Errorf("annotation %q is not a checkpoint (category: %s)", meta.AnnotationID, ann.Category)
	}

	if err := m.store.SaveCheckpointMeta(meta); err != nil {
		return nil, fmt.Errorf("persist checkpoint meta: %w", err)
	}

	m.mu.Lock()
	m.metas[meta.AnnotationID] = meta
	m.mu.Unlock()

	m.emit(Event{Type: EventCheckpointMetaUpdate, Data: meta})

	return &meta, nil
}

// LogPassage records a passage through a checkpoint.
func (m *Manager) LogPassage(p store.CheckpointPassage) (*store.CheckpointPassage, error) {
	// Validate checkpoint exists in metas.
	m.mu.RLock()
	meta, ok := m.metas[p.CheckpointID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("checkpoint %q not found in metas", p.CheckpointID)
	}

	p.ID = uuid.New().String()
	if p.PassageTime.IsZero() {
		p.PassageTime = time.Now().UTC()
	}
	if p.Direction == "" {
		p.Direction = "through"
	}

	if err := m.store.SaveCheckpointPassage(p); err != nil {
		return nil, fmt.Errorf("persist checkpoint passage: %w", err)
	}

	// Also persist a timeline event.
	ann, _ := m.annMgr.Get(p.CheckpointID)
	cpLabel := p.CheckpointID
	if ann != nil {
		cpLabel = ann.Label
	}
	m.store.SaveNetEvent(store.NetEvent{
		ID:        uuid.New().String(),
		NetID:     p.NetID,
		Type:      EventCheckpointPassage,
		Callsign:  p.ReportedBy,
		Summary:   fmt.Sprintf("%s passed %s (seq %d)", p.Label, cpLabel, meta.SequenceNumber),
		Details:   fmt.Sprintf(`{"checkpointId":"%s","label":"%s","direction":"%s"}`, p.CheckpointID, p.Label, p.Direction),
		CreatedAt: p.PassageTime,
	})

	m.mu.Lock()
	m.passages[p.NetID] = append(m.passages[p.NetID], p)
	m.mu.Unlock()

	m.emit(Event{Type: EventCheckpointPassage, Data: p})

	return &p, nil
}

// GetCheckpointsForNet returns all checkpoints with their passages, sorted by sequence.
func (m *Manager) GetCheckpointsForNet(netID string) ([]CheckpointWithPassages, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Collect metas for this net.
	var netMetas []store.CheckpointMeta
	for _, meta := range m.metas {
		if meta.NetID == netID {
			netMetas = append(netMetas, meta)
		}
	}

	// Sort by sequence number.
	sort.Slice(netMetas, func(i, j int) bool {
		return netMetas[i].SequenceNumber < netMetas[j].SequenceNumber
	})

	// Build passage map by checkpoint ID.
	passageMap := make(map[string][]store.CheckpointPassage)
	for _, p := range m.passages[netID] {
		passageMap[p.CheckpointID] = append(passageMap[p.CheckpointID], p)
	}

	var result []CheckpointWithPassages
	for _, meta := range netMetas {
		ann, ok := m.annMgr.Get(meta.AnnotationID)
		if !ok {
			continue // annotation was deleted
		}

		passages := passageMap[meta.AnnotationID]
		var latest *time.Time
		if len(passages) > 0 {
			t := passages[len(passages)-1].PassageTime
			latest = &t
		}

		result = append(result, CheckpointWithPassages{
			Annotation:    *ann,
			Meta:          meta,
			Passages:      passages,
			PassageCount:  len(passages),
			LatestPassage: latest,
		})
	}

	return result, nil
}

// GetProgress computes element positions from passage data.
func (m *Manager) GetProgress(netID string) (*CheckpointProgress, error) {
	checkpoints, err := m.GetCheckpointsForNet(netID)
	if err != nil {
		return nil, err
	}

	// Build a lookup from checkpoint annotation ID to sequence number.
	seqMap := make(map[string]int)
	for _, cp := range checkpoints {
		seqMap[cp.Meta.AnnotationID] = cp.Meta.SequenceNumber
	}

	// Compute element positions: for each unique label, find the highest-sequence checkpoint.
	m.mu.RLock()
	passages := m.passages[netID]
	m.mu.RUnlock()

	type elementInfo struct {
		label             string
		lastCheckpointID  string
		lastCheckpointSeq int
		lastPassageTime   time.Time
	}

	elemMap := make(map[string]*elementInfo)
	for _, p := range passages {
		seq, ok := seqMap[p.CheckpointID]
		if !ok {
			continue
		}

		existing, exists := elemMap[p.Label]
		if !exists || seq > existing.lastCheckpointSeq {
			elemMap[p.Label] = &elementInfo{
				label:             p.Label,
				lastCheckpointID:  p.CheckpointID,
				lastCheckpointSeq: seq,
				lastPassageTime:   p.PassageTime,
			}
		}
	}

	var elements []ProgressElement
	for _, info := range elemMap {
		elements = append(elements, ProgressElement{
			Label:             info.label,
			LastCheckpointID:  info.lastCheckpointID,
			LastCheckpointSeq: info.lastCheckpointSeq,
			LastPassageTime:   info.lastPassageTime,
		})
	}

	// Sort elements: lead first, then by sequence desc.
	sort.Slice(elements, func(i, j int) bool {
		return elements[i].LastCheckpointSeq > elements[j].LastCheckpointSeq
	})

	return &CheckpointProgress{
		NetID:       netID,
		Checkpoints: checkpoints,
		Elements:    elements,
	}, nil
}

// DeleteMetaForAnnotation removes checkpoint metadata for an annotation.
func (m *Manager) DeleteMetaForAnnotation(annotationID string) error {
	if err := m.store.DeleteCheckpointMeta(annotationID); err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.metas, annotationID)
	m.mu.Unlock()

	return nil
}

// DeletePassagesForNet removes all passages for a net.
func (m *Manager) DeletePassagesForNet(netID string) error {
	if err := m.store.DeleteCheckpointPassages(netID); err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.passages, netID)
	m.mu.Unlock()

	return nil
}

func (m *Manager) emit(evt Event) {
	select {
	case m.events <- evt:
	default:
	}
}
