package transport

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// dedupEntry records a recently seen frame hash for deduplication.
type dedupEntry struct {
	hash uint64
	time time.Time
}

// transportStats tracks per-transport packet counts.
type transportStats struct {
	packetsRx atomic.Int64
	packetsTx atomic.Int64
}

// Manager multiplexes multiple transports and merges their received frames.
type Manager struct {
	mu         sync.RWMutex
	transports map[string]Transport
	stats      map[string]*transportStats
	frames     chan aprs.APRSFrame
	tagged     chan TransportFrame

	// DedupWindow is the time window for deduplication. Zero disables dedup.
	DedupWindow time.Duration

	dedupMu  sync.Mutex
	dedupBuf []dedupEntry
}

// NewManager creates a new transport manager.
func NewManager() *Manager {
	return &Manager{
		transports:  make(map[string]Transport),
		stats:       make(map[string]*transportStats),
		frames:      make(chan aprs.APRSFrame, 256),
		tagged:      make(chan TransportFrame, 256),
		DedupWindow: 30 * time.Second,
	}
}

// Add registers a transport with the given ID.
func (m *Manager) Add(id string, t Transport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transports[id] = t
	m.stats[id] = &transportStats{}
}

// ConnectAll connects all registered transports and starts frame forwarding.
func (m *Manager) ConnectAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, t := range m.transports {
		if err := t.Connect(ctx); err != nil {
			log.Printf("[transport] failed to connect %s: %v", id, err)
			return err
		}
		go m.forwardFrames(ctx, id, t)
	}
	return nil
}

// forwardFrames reads from a single transport's Receive channel and
// forwards frames to the merged channels with deduplication.
func (m *Manager) forwardFrames(ctx context.Context, id string, t Transport) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-t.Receive():
			if !ok {
				return
			}

			// Dedup check
			if m.DedupWindow > 0 && m.isDuplicate(frame) {
				continue
			}

			// Track stats
			if st, ok := m.stats[id]; ok {
				st.packetsRx.Add(1)
			}

			tf := TransportFrame{Frame: frame, Source: id}

			// Send on tagged channel (non-blocking)
			select {
			case m.tagged <- tf:
			default:
			}

			// Send on legacy frames channel (non-blocking)
			select {
			case m.frames <- frame:
			default:
			}
		}
	}
}

// isDuplicate checks if a frame was recently seen (within DedupWindow).
func (m *Manager) isDuplicate(frame aprs.APRSFrame) bool {
	hash := m.frameHash(frame)
	now := time.Now()

	m.dedupMu.Lock()
	defer m.dedupMu.Unlock()

	// Prune expired entries
	cutoff := now.Add(-m.DedupWindow)
	n := 0
	for _, e := range m.dedupBuf {
		if e.time.After(cutoff) {
			m.dedupBuf[n] = e
			n++
		}
	}
	m.dedupBuf = m.dedupBuf[:n]

	// Check for match
	for _, e := range m.dedupBuf {
		if e.hash == hash {
			return true
		}
	}

	// Record this frame
	m.dedupBuf = append(m.dedupBuf, dedupEntry{hash: hash, time: now})
	return false
}

// frameHash computes a hash of the frame's source callsign and payload for dedup.
func (m *Manager) frameHash(frame aprs.APRSFrame) uint64 {
	h := fnv.New64a()
	h.Write([]byte(frame.Source.String()))
	h.Write([]byte{0})
	h.Write([]byte(frame.Payload))
	return h.Sum64()
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

// Send transmits an APRS frame on all connected transports.
func (m *Manager) Send(frame aprs.APRSFrame) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, t := range m.transports {
		if s := t.Status(); s.Connected {
			if err := t.Send(frame); err != nil {
				return err
			}
			if st, ok := m.stats[id]; ok {
				st.packetsTx.Add(1)
			}
		}
	}
	return nil
}

// SendVia transmits an APRS frame on a specific transport by ID.
func (m *Manager) SendVia(id string, frame aprs.APRSFrame) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.transports[id]
	if !ok {
		return fmt.Errorf("transport %q not found", id)
	}
	if err := t.Send(frame); err != nil {
		return err
	}
	if st, ok := m.stats[id]; ok {
		st.packetsTx.Add(1)
	}
	return nil
}

// Frames returns a merged channel of frames from all transports (without source info).
func (m *Manager) Frames() <-chan aprs.APRSFrame {
	return m.frames
}

// TaggedFrames returns a channel of frames with source transport metadata.
func (m *Manager) TaggedFrames() <-chan TransportFrame {
	return m.tagged
}

// Statuses returns the status of all registered transports with stats.
func (m *Manager) Statuses() []TransportStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	statuses := make([]TransportStatus, 0, len(m.transports))
	for id, t := range m.transports {
		s := t.Status()
		s.ID = id
		if st, ok := m.stats[id]; ok {
			s.PacketsRx = st.packetsRx.Load()
			s.PacketsTx = st.packetsTx.Load()
		}
		statuses = append(statuses, s)
	}
	return statuses
}
