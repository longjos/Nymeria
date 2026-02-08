package transport

import (
	"context"
	"sync"

	"github.com/narvel/nymeria/internal/aprs"
)

// Manager multiplexes multiple transports and merges their received frames.
type Manager struct {
	mu         sync.RWMutex
	transports map[string]Transport
	frames     chan aprs.APRSFrame
}

// NewManager creates a new transport manager.
func NewManager() *Manager {
	return &Manager{
		transports: make(map[string]Transport),
		frames:     make(chan aprs.APRSFrame, 256),
	}
}

// Add registers a transport with the given ID.
func (m *Manager) Add(id string, t Transport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transports[id] = t
}

// ConnectAll connects all registered transports.
func (m *Manager) ConnectAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, t := range m.transports {
		if err := t.Connect(ctx); err != nil {
			return err
		}
	}
	return nil
}

// CloseAll shuts down all transports.
func (m *Manager) CloseAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, t := range m.transports {
		if err := t.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Frames returns a merged channel of frames from all transports.
func (m *Manager) Frames() <-chan aprs.APRSFrame {
	return m.frames
}

// Statuses returns the status of all registered transports.
func (m *Manager) Statuses() []TransportStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	statuses := make([]TransportStatus, 0, len(m.transports))
	for _, t := range m.transports {
		statuses = append(statuses, t.Status())
	}
	return statuses
}
