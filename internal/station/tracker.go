package station

import (
	"context"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/config"
)

// Tracker tracks APRS stations.
type Tracker interface {
	// Update adds or updates a station.
	Update(s Station)

	// Get returns a station by callsign-SSID key.
	Get(callsign string) (Station, bool)

	// All returns all tracked stations.
	All() []Station

	// InArea returns stations within a geographic bounding box.
	InArea(south, west, north, east float64) []Station

	// Search returns stations whose callsign matches the given prefix (case-insensitive).
	Search(prefix string) []Station

	// Events returns a channel that emits station lifecycle events.
	Events() <-chan Event

	// HandlePacket processes a parsed APRS packet and updates station state.
	HandlePacket(pkt *aprs.Packet, source string)

	// Start launches background goroutines (aging sweep). The sweep runs at the given interval.
	Start(ctx context.Context, sweepInterval time.Duration)
}

// dedupEntry records a recently seen payload hash for deduplication.
type dedupEntry struct {
	key  uint64
	time time.Time
}

// MemoryTracker is an in-memory station tracker.
type MemoryTracker struct {
	mu       sync.RWMutex
	stations map[string]Station

	cfg    config.StationConfig
	events chan Event
	done   chan struct{}

	dedupMu  sync.Mutex
	dedupBuf []dedupEntry
}

// NewMemoryTracker creates a new in-memory tracker with the given config.
func NewMemoryTracker(cfg config.StationConfig) *MemoryTracker {
	return &MemoryTracker{
		stations:  make(map[string]Station),
		cfg:       cfg,
		events:    make(chan Event, 256),
		done:      make(chan struct{}),
		dedupBuf:  make([]dedupEntry, 0, 256),
	}
}

// Events returns the event channel.
func (t *MemoryTracker) Events() <-chan Event {
	return t.events
}

// HandlePacket processes a parsed APRS packet into station state.
func (t *MemoryTracker) HandlePacket(pkt *aprs.Packet, source string) {
	callsign, ssid, pos := t.extractStationData(pkt)
	if pos == nil {
		return // non-position packet, nothing to track
	}

	key := t.stationKey(callsign, ssid)

	// Dedup check
	hash := t.payloadHash(pkt)
	if t.isDuplicate(key, hash) {
		return
	}

	now := time.Now()

	t.mu.Lock()
	existing, exists := t.stations[key]

	var s Station
	if exists {
		s = existing
	} else {
		s = Station{
			Callsign: callsign,
			SSID:     ssid,
		}
	}

	s.LastHeard = now
	s.Position = &Position{
		Lat:      pos.Lat,
		Lon:      pos.Lon,
		Altitude: pos.Altitude,
		Speed:    pos.Speed,
		Course:   pos.Course,
	}
	s.Symbol = pos.Symbol
	s.Comment = pos.Comment
	s.Source = t.mergeSource(s.Source, source)
	if pkt.Weather != nil {
		s.Weather = pkt.Weather
	}
	if pkt.DF != nil {
		s.DF = pkt.DF
	}

	// Append track point
	s.Track = append(s.Track, TrackPoint{
		Lat:  pos.Lat,
		Lon:  pos.Lon,
		Time: now,
	})
	// Cap at TrackMaxPoints (drop oldest)
	if max := t.cfg.TrackMaxPoints; max > 0 && len(s.Track) > max {
		s.Track = s.Track[len(s.Track)-max:]
	}

	t.stations[key] = s
	t.mu.Unlock()

	// Emit event
	if exists {
		t.emit(Event{Type: EventStationUpdate, Station: s})
	} else {
		t.emit(Event{Type: EventNewStation, Station: s})
	}
}

// Update adds or updates a station directly.
func (t *MemoryTracker) Update(s Station) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := t.stationKey(s.Callsign, s.SSID)
	t.stations[key] = s
}

// Get returns a station by its callsign-SSID key.
func (t *MemoryTracker) Get(callsign string) (Station, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.stations[callsign]
	return s, ok
}

// All returns all tracked stations.
func (t *MemoryTracker) All() []Station {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]Station, 0, len(t.stations))
	for _, s := range t.stations {
		result = append(result, s)
	}
	return result
}

// InArea returns stations within a geographic bounding box (inclusive).
func (t *MemoryTracker) InArea(south, west, north, east float64) []Station {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var result []Station
	for _, s := range t.stations {
		if s.Position == nil {
			continue
		}
		if s.Position.Lat >= south && s.Position.Lat <= north &&
			s.Position.Lon >= west && s.Position.Lon <= east {
			result = append(result, s)
		}
	}
	return result
}

// Search returns stations whose callsign matches the given prefix (case-insensitive).
func (t *MemoryTracker) Search(prefix string) []Station {
	upper := strings.ToUpper(prefix)
	t.mu.RLock()
	defer t.mu.RUnlock()
	var result []Station
	for key, s := range t.stations {
		if strings.HasPrefix(strings.ToUpper(key), upper) {
			result = append(result, s)
		}
	}
	return result
}

// Start launches the aging sweep goroutine. It removes stations that haven't
// been heard from within StaleTimeout, emitting EventStationExpired for each.
func (t *MemoryTracker) Start(ctx context.Context, sweepInterval time.Duration) {
	go func() {
		ticker := time.NewTicker(sweepInterval)
		defer ticker.Stop()
		defer close(t.events)
		defer close(t.done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.sweep()
			}
		}
	}()
}

// sweep removes stale stations.
func (t *MemoryTracker) sweep() {
	cutoff := time.Now().Add(-t.cfg.StaleTimeout)

	t.mu.Lock()
	var expired []Station
	for key, s := range t.stations {
		if s.LastHeard.Before(cutoff) {
			expired = append(expired, s)
			delete(t.stations, key)
		}
	}
	t.mu.Unlock()

	for _, s := range expired {
		t.emit(Event{Type: EventStationExpired, Station: s})
	}
}

// extractStationData returns the callsign, SSID, and position data from a packet.
// For objects/items, the callsign is the object/item name with SSID 0.
// Returns nil position if the packet doesn't carry position data.
func (t *MemoryTracker) extractStationData(pkt *aprs.Packet) (string, int, *aprs.PositionData) {
	switch pkt.Type {
	case aprs.PacketTypePosition:
		if pkt.Position == nil {
			return "", 0, nil
		}
		return pkt.Frame.Source.Call, pkt.Frame.Source.SSID, pkt.Position

	case aprs.PacketTypeMicE:
		if pkt.MicE == nil {
			return "", 0, nil
		}
		return pkt.Frame.Source.Call, pkt.Frame.Source.SSID, &pkt.MicE.Position

	case aprs.PacketTypeObject:
		if pkt.Object == nil {
			return "", 0, nil
		}
		return strings.TrimSpace(pkt.Object.Name), 0, &pkt.Object.Position

	case aprs.PacketTypeItem:
		if pkt.Item == nil {
			return "", 0, nil
		}
		return strings.TrimSpace(pkt.Item.Name), 0, &pkt.Item.Position

	case aprs.PacketTypeWeather:
		if pkt.Position == nil {
			return "", 0, nil
		}
		return pkt.Frame.Source.Call, pkt.Frame.Source.SSID, pkt.Position

	default:
		return "", 0, nil
	}
}

// stationKey builds the map key: "CALL" or "CALL-SSID".
func (t *MemoryTracker) stationKey(callsign string, ssid int) string {
	return aprs.Address{Call: callsign, SSID: ssid}.String()
}

// payloadHash returns a FNV hash of the packet's raw payload for dedup.
func (t *MemoryTracker) payloadHash(pkt *aprs.Packet) uint64 {
	h := fnv.New64a()
	h.Write([]byte(pkt.Frame.Payload))
	return h.Sum64()
}

// isDuplicate checks if a {stationKey, payloadHash} pair was seen within DedupWindow.
func (t *MemoryTracker) isDuplicate(stationKey string, hash uint64) bool {
	combined := t.combinedHash(stationKey, hash)
	now := time.Now()

	t.dedupMu.Lock()
	defer t.dedupMu.Unlock()

	// Prune old entries
	cutoff := now.Add(-t.cfg.DedupWindow)
	n := 0
	for _, e := range t.dedupBuf {
		if e.time.After(cutoff) {
			t.dedupBuf[n] = e
			n++
		}
	}
	t.dedupBuf = t.dedupBuf[:n]

	// Check for match
	for _, e := range t.dedupBuf {
		if e.key == combined {
			return true
		}
	}

	// Record this entry
	t.dedupBuf = append(t.dedupBuf, dedupEntry{key: combined, time: now})
	return false
}

// combinedHash combines a station key and payload hash into a single uint64.
func (t *MemoryTracker) combinedHash(stationKey string, payloadHash uint64) uint64 {
	h := fnv.New64a()
	h.Write([]byte(stationKey))
	b := [8]byte{
		byte(payloadHash), byte(payloadHash >> 8),
		byte(payloadHash >> 16), byte(payloadHash >> 24),
		byte(payloadHash >> 32), byte(payloadHash >> 40),
		byte(payloadHash >> 48), byte(payloadHash >> 56),
	}
	h.Write(b[:])
	return h.Sum64()
}

// mergeSource returns the combined source string.
func (t *MemoryTracker) mergeSource(existing, incoming string) string {
	if existing == "" || existing == incoming {
		return incoming
	}
	return "both"
}

// emit sends an event on the channel without blocking.
func (t *MemoryTracker) emit(e Event) {
	select {
	case <-t.done:
		return
	default:
	}
	select {
	case t.events <- e:
	default:
		// Channel full, drop event to avoid blocking.
	}
}
