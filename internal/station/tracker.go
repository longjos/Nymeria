package station

import (
	"sync"
)

// Tracker tracks APRS stations.
type Tracker interface {
	// Update adds or updates a station.
	Update(s Station)

	// Get returns a station by callsign.
	Get(callsign string) (Station, bool)

	// All returns all tracked stations.
	All() []Station
}

// MemoryTracker is an in-memory station tracker.
type MemoryTracker struct {
	mu       sync.RWMutex
	stations map[string]Station
}

// NewMemoryTracker creates a new in-memory tracker.
func NewMemoryTracker() *MemoryTracker {
	return &MemoryTracker{
		stations: make(map[string]Station),
	}
}

func (t *MemoryTracker) Update(s Station) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stations[s.Callsign] = s
}

func (t *MemoryTracker) Get(callsign string) (Station, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.stations[callsign]
	return s, ok
}

func (t *MemoryTracker) All() []Station {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]Station, 0, len(t.stations))
	for _, s := range t.stations {
		result = append(result, s)
	}
	return result
}
