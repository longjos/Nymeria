package object

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// SendFunc transmits an APRS frame via the transport layer.
type SendFunc func(frame aprs.APRSFrame) error

// ManagerConfig holds configuration for the object manager.
type ManagerConfig struct {
	RetransmitInterval time.Duration // how often to retransmit live objects
}

// Object represents a locally-created APRS object.
type Object struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Lat       float64     `json:"lat"`
	Lon       float64     `json:"lon"`
	Symbol    aprs.Symbol `json:"symbol"`
	Comment   string      `json:"comment"`
	Live      bool        `json:"live"`
	CreatedAt time.Time   `json:"createdAt"`
}

// Item represents a locally-created APRS item.
type Item struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Lat       float64     `json:"lat"`
	Lon       float64     `json:"lon"`
	Symbol    aprs.Symbol `json:"symbol"`
	Comment   string      `json:"comment"`
	Live      bool        `json:"live"`
	CreatedAt time.Time   `json:"createdAt"`
}

// ReceivedObject tracks an object received from another station.
type ReceivedObject struct {
	Name      string      `json:"name"`
	OwnerCall string      `json:"ownerCall"`
	Lat       float64     `json:"lat"`
	Lon       float64     `json:"lon"`
	Symbol    aprs.Symbol `json:"symbol"`
	Comment   string      `json:"comment"`
	Live      bool        `json:"live"`
	Timestamp time.Time   `json:"timestamp"`
	LastHeard time.Time   `json:"lastHeard"`
}

// ReceivedItem tracks an item received from another station.
type ReceivedItem struct {
	Name      string      `json:"name"`
	OwnerCall string      `json:"ownerCall"`
	Lat       float64     `json:"lat"`
	Lon       float64     `json:"lon"`
	Symbol    aprs.Symbol `json:"symbol"`
	Comment   string      `json:"comment"`
	Live      bool        `json:"live"`
	LastHeard time.Time   `json:"lastHeard"`
}

// Event represents an object manager event for WebSocket broadcast.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Manager manages creation, transmission, and tracking of APRS objects and items.
type Manager struct {
	callsign string
	ssid     int
	path     []aprs.Address
	send     SendFunc
	config   ManagerConfig

	mu       sync.Mutex
	objects  []Object
	items    []Item
	received []ReceivedObject
	rxItems  []ReceivedItem
	events   chan Event
	nextID   int
	closed   bool
}

// NewManager creates a new object/item manager.
func NewManager(callsign string, ssid int, send SendFunc, cfg ManagerConfig) *Manager {
	return &Manager{
		callsign: callsign,
		ssid:     ssid,
		path:     aprs.DefaultRFPath(),
		send:     send,
		config:   cfg,
		events:   make(chan Event, 64),
	}
}

// CreateObject creates and transmits a new APRS object.
func (m *Manager) CreateObject(obj Object) (*Object, error) {
	if err := ValidateObjectName(obj.Name); err != nil {
		return nil, err
	}

	m.mu.Lock()

	// Check for duplicate name among live own objects
	for _, existing := range m.objects {
		if existing.Name == obj.Name && existing.Live {
			m.mu.Unlock()
			return nil, fmt.Errorf("object %q already exists", obj.Name)
		}
	}

	m.nextID++
	obj.ID = fmt.Sprintf("obj-%d", m.nextID)
	obj.CreatedAt = time.Now()
	m.objects = append(m.objects, obj)
	m.mu.Unlock()

	// Transmit
	if err := m.transmitObject(obj); err != nil {
		return nil, fmt.Errorf("transmit object: %w", err)
	}

	// Emit event
	m.emitEvent("object_created", obj)

	return &obj, nil
}

// CreateItem creates and transmits a new APRS item.
func (m *Manager) CreateItem(item Item) (*Item, error) {
	if err := ValidateItemName(item.Name); err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.nextID++
	item.ID = fmt.Sprintf("item-%d", m.nextID)
	item.CreatedAt = time.Now()
	m.items = append(m.items, item)
	m.mu.Unlock()

	if err := m.transmitItem(item); err != nil {
		return nil, fmt.Errorf("transmit item: %w", err)
	}

	m.emitEvent("item_created", item)

	return &item, nil
}

// KillObject marks an object as killed and transmits the kill frame.
func (m *Manager) KillObject(id string) error {
	m.mu.Lock()
	var found *Object
	for i := range m.objects {
		if m.objects[i].ID == id {
			m.objects[i].Live = false
			found = &m.objects[i]
			break
		}
	}
	m.mu.Unlock()

	if found == nil {
		return fmt.Errorf("object %q not found", id)
	}

	if err := m.transmitObject(*found); err != nil {
		return fmt.Errorf("transmit kill: %w", err)
	}

	m.emitEvent("object_killed", *found)
	return nil
}

// KillItem marks an item as killed and transmits the kill frame.
func (m *Manager) KillItem(id string) error {
	m.mu.Lock()
	var found *Item
	for i := range m.items {
		if m.items[i].ID == id {
			m.items[i].Live = false
			found = &m.items[i]
			break
		}
	}
	m.mu.Unlock()

	if found == nil {
		return fmt.Errorf("item %q not found", id)
	}

	if err := m.transmitItem(*found); err != nil {
		return fmt.Errorf("transmit kill: %w", err)
	}

	m.emitEvent("item_killed", *found)
	return nil
}

// DeleteObject removes an object from the local list (stops retransmission).
func (m *Manager) DeleteObject(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.objects {
		if m.objects[i].ID == id {
			m.objects = append(m.objects[:i], m.objects[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("object %q not found", id)
}

// DeleteItem removes an item from the local list.
func (m *Manager) DeleteItem(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("item %q not found", id)
}

// OwnObjects returns all locally-created objects.
func (m *Manager) OwnObjects() []Object {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Object, len(m.objects))
	copy(result, m.objects)
	return result
}

// OwnItems returns all locally-created items.
func (m *Manager) OwnItems() []Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Item, len(m.items))
	copy(result, m.items)
	return result
}

// ReceivedObjects returns all objects received from other stations.
func (m *Manager) ReceivedObjects() []ReceivedObject {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]ReceivedObject, len(m.received))
	copy(result, m.received)
	return result
}

// ReceivedItems returns all items received from other stations.
func (m *Manager) ReceivedItems() []ReceivedItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]ReceivedItem, len(m.rxItems))
	copy(result, m.rxItems)
	return result
}

// HandlePacket processes a parsed APRS packet for object/item content.
func (m *Manager) HandlePacket(pkt *aprs.Packet) {
	if pkt == nil {
		return
	}

	switch pkt.Type {
	case aprs.PacketTypeObject:
		if pkt.Object == nil {
			return
		}
		m.handleReceivedObject(pkt)
	case aprs.PacketTypeItem:
		if pkt.Item == nil {
			return
		}
		m.handleReceivedItem(pkt)
	}
}

func (m *Manager) handleReceivedObject(pkt *aprs.Packet) {
	obj := ReceivedObject{
		Name:      pkt.Object.Name,
		OwnerCall: pkt.Frame.Source.String(),
		Lat:       pkt.Object.Position.Lat,
		Lon:       pkt.Object.Position.Lon,
		Symbol:    pkt.Object.Position.Symbol,
		Comment:   pkt.Object.Position.Comment,
		Live:      pkt.Object.Live,
		Timestamp: pkt.Object.Timestamp,
		LastHeard: time.Now(),
	}

	m.mu.Lock()
	// Update or add
	found := false
	for i := range m.received {
		if m.received[i].Name == obj.Name {
			m.received[i] = obj
			found = true
			break
		}
	}
	if !found {
		m.received = append(m.received, obj)
	}
	m.mu.Unlock()

	m.emitEvent("object_received", obj)
}

func (m *Manager) handleReceivedItem(pkt *aprs.Packet) {
	item := ReceivedItem{
		Name:      pkt.Item.Name,
		OwnerCall: pkt.Frame.Source.String(),
		Lat:       pkt.Item.Position.Lat,
		Lon:       pkt.Item.Position.Lon,
		Symbol:    pkt.Item.Position.Symbol,
		Comment:   pkt.Item.Position.Comment,
		Live:      pkt.Item.Live,
		LastHeard: time.Now(),
	}

	m.mu.Lock()
	found := false
	for i := range m.rxItems {
		if m.rxItems[i].Name == item.Name {
			m.rxItems[i] = item
			found = true
			break
		}
	}
	if !found {
		m.rxItems = append(m.rxItems, item)
	}
	m.mu.Unlock()

	m.emitEvent("item_received", item)
}

// Events returns a channel of object manager events.
func (m *Manager) Events() <-chan Event {
	return m.events
}

// Start begins the retransmit loop for live objects.
func (m *Manager) Start(ctx context.Context) {
	go m.retransmitLoop(ctx)
}

// Close shuts down the manager.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
	}
}

func (m *Manager) retransmitLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.RetransmitInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.retransmitAll()
		}
	}
}

func (m *Manager) retransmitAll() {
	m.mu.Lock()
	// Snapshot identity and live objects/items for retransmission
	callsign := m.callsign
	ssid := m.ssid
	path := append([]aprs.Address(nil), m.path...)
	var liveObjects []Object
	for _, obj := range m.objects {
		if obj.Live {
			liveObjects = append(liveObjects, obj)
		}
	}
	var liveItems []Item
	for _, item := range m.items {
		if item.Live {
			liveItems = append(liveItems, item)
		}
	}
	m.mu.Unlock()

	for _, obj := range liveObjects {
		if err := m.send(buildFrame(callsign, ssid, path, FormatObjectPayload(obj))); err != nil {
			log.Printf("[object] retransmit object %q: %v", obj.Name, err)
		}
	}
	for _, item := range liveItems {
		if err := m.send(buildFrame(callsign, ssid, path, FormatItemPayload(item))); err != nil {
			log.Printf("[object] retransmit item %q: %v", item.Name, err)
		}
	}
}

// UpdateStationInfo updates the station identity for object/item frames.
func (m *Manager) UpdateStationInfo(callsign string, ssid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callsign = callsign
	m.ssid = ssid
}

// UpdatePath updates the digipeater path used for object/item frames.
func (m *Manager) UpdatePath(path []aprs.Address) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.path = append([]aprs.Address(nil), path...)
}

// buildFrame creates an APRS frame with the given source identity and payload.
func buildFrame(callsign string, ssid int, path []aprs.Address, payload string) aprs.APRSFrame {
	return aprs.APRSFrame{
		Source:      aprs.Address{Call: callsign, SSID: ssid},
		Destination: aprs.Address{Call: "APNMRA"},
		Path:        append([]aprs.Address(nil), path...),
		Payload:     payload,
	}
}

// transmitObject sends an object frame using the current station identity.
func (m *Manager) transmitObject(obj Object) error {
	m.mu.Lock()
	callsign := m.callsign
	ssid := m.ssid
	path := append([]aprs.Address(nil), m.path...)
	m.mu.Unlock()
	return m.send(buildFrame(callsign, ssid, path, FormatObjectPayload(obj)))
}

// transmitItem sends an item frame using the current station identity.
func (m *Manager) transmitItem(item Item) error {
	m.mu.Lock()
	callsign := m.callsign
	ssid := m.ssid
	path := append([]aprs.Address(nil), m.path...)
	m.mu.Unlock()
	return m.send(buildFrame(callsign, ssid, path, FormatItemPayload(item)))
}

func (m *Manager) emitEvent(eventType string, data any) {
	select {
	case m.events <- Event{Type: eventType, Data: data}:
	default:
		// Channel full, drop event
	}
}

// --- Position Formatting ---

// FormatLatitude formats a decimal latitude as APRS uncompressed format "DDMM.hhN".
func FormatLatitude(lat float64) string {
	ns := byte('N')
	if lat < 0 {
		ns = 'S'
		lat = -lat
	}
	deg := int(lat)
	min := (lat - float64(deg)) * 60.0
	return fmt.Sprintf("%02d%05.2f%c", deg, min, ns)
}

// FormatLongitude formats a decimal longitude as APRS uncompressed format "DDDMM.hhW".
func FormatLongitude(lon float64) string {
	ew := byte('E')
	if lon < 0 {
		ew = 'W'
		lon = -lon
	}
	deg := int(lon)
	min := (lon - float64(deg)) * 60.0
	return fmt.Sprintf("%03d%05.2f%c", deg, min, ew)
}

// FormatTimestamp formats a time as APRS DDHHMMz.
func FormatTimestamp(t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("%02d%02d%02dz", t.Day(), t.Hour(), t.Minute())
}

// FormatObjectPayload formats an Object as an APRS object payload string.
// Format: ;name_____*DDHHMMzDDMM.hhN/DDDMM.hhW$Comment
func FormatObjectPayload(obj Object) string {
	// Pad name to exactly 9 characters
	name := obj.Name
	for len(name) < 9 {
		name += " "
	}

	// APRS101 spec: '*' = live object, '\' = killed object
	indicator := byte('*')
	if !obj.Live {
		indicator = '\\'
	}

	ts := FormatTimestamp(time.Now())
	lat := FormatLatitude(obj.Lat)
	lon := FormatLongitude(obj.Lon)

	return fmt.Sprintf(";%s%c%s%s%c%s%c%s",
		name, indicator, ts,
		lat, obj.Symbol.Table,
		lon, obj.Symbol.Code,
		obj.Comment)
}

// FormatItemPayload formats an Item as an APRS item payload string.
// Format: )name!DDMM.hhN/DDDMM.hhW$Comment (live)
// Format: )name_DDMM.hhN/DDDMM.hhW$Comment (killed)
func FormatItemPayload(item Item) string {
	indicator := byte('!')
	if !item.Live {
		indicator = '_'
	}

	lat := FormatLatitude(item.Lat)
	lon := FormatLongitude(item.Lon)

	return fmt.Sprintf(")%s%c%s%c%s%c%s",
		item.Name, indicator,
		lat, item.Symbol.Table,
		lon, item.Symbol.Code,
		item.Comment)
}

// --- Validation ---

// ValidateObjectName checks that an object name is valid (1-9 chars).
func ValidateObjectName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("object name is required")
	}
	if len(name) > 9 {
		return fmt.Errorf("object name %q exceeds 9 characters", name)
	}
	return nil
}

// ValidateItemName checks that an item name is valid (1-9 chars).
func ValidateItemName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("item name is required")
	}
	if len(name) > 9 {
		return fmt.Errorf("item name %q exceeds 9 characters", name)
	}
	return nil
}

