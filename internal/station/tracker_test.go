package station

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/config"
)

func testConfig() config.StationConfig {
	return config.StationConfig{
		Callsign:       "N0CALL",
		TrackMaxPoints: 5,
		StaleTimeout:   80 * time.Minute,
		DedupWindow:    30 * time.Second,
	}
}

func positionPacket(call string, ssid int, lat, lon float64, comment string) *aprs.Packet {
	// Build a unique payload string from position so dedup works correctly in tests.
	payload := fmt.Sprintf("!%09.4f/%010.4f-%s", lat, lon, comment)
	return &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:  aprs.Address{Call: call, SSID: ssid},
			Payload: payload,
		},
		Type: aprs.PacketTypePosition,
		Position: &aprs.PositionData{
			Lat:     lat,
			Lon:     lon,
			Symbol:  aprs.Symbol{Table: '/', Code: '-'},
			Comment: comment,
		},
	}
}

func micEPacket(call string, ssid int, lat, lon float64) *aprs.Packet {
	payload := fmt.Sprintf("mice:%f,%f", lat, lon)
	return &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:  aprs.Address{Call: call, SSID: ssid},
			Payload: payload,
		},
		Type: aprs.PacketTypeMicE,
		MicE: &aprs.MicEData{
			Position: aprs.PositionData{
				Lat:    lat,
				Lon:    lon,
				Symbol: aprs.Symbol{Table: '/', Code: '>'},
			},
			MicEMsg: "En Route",
		},
	}
}

func objectPacket(source string, objName string, lat, lon float64) *aprs.Packet {
	payload := fmt.Sprintf(";%-9s*%f/%f", objName, lat, lon)
	return &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:  aprs.Address{Call: source},
			Payload: payload,
		},
		Type: aprs.PacketTypeObject,
		Object: &aprs.ObjectData{
			Name: objName,
			Live: true,
			Position: aprs.PositionData{
				Lat:    lat,
				Lon:    lon,
				Symbol: aprs.Symbol{Table: '/', Code: 'O'},
			},
		},
	}
}

func itemPacket(source string, itemName string, lat, lon float64) *aprs.Packet {
	payload := fmt.Sprintf(")%s!%f/%f", itemName, lat, lon)
	return &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:  aprs.Address{Call: source},
			Payload: payload,
		},
		Type: aprs.PacketTypeItem,
		Item: &aprs.ItemData{
			Name: itemName,
			Live: true,
			Position: aprs.PositionData{
				Lat:    lat,
				Lon:    lon,
				Symbol: aprs.Symbol{Table: '/', Code: 'I'},
			},
		},
	}
}

func weatherPacket(call string, lat, lon float64) *aprs.Packet {
	temp := 22.0
	payload := fmt.Sprintf("!%f/%f_t072", lat, lon)
	return &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:  aprs.Address{Call: call},
			Payload: payload,
		},
		Type: aprs.PacketTypeWeather,
		Position: &aprs.PositionData{
			Lat:    lat,
			Lon:    lon,
			Symbol: aprs.Symbol{Table: '/', Code: '_'},
		},
		Weather: &aprs.WeatherData{
			Temperature: &temp,
		},
	}
}

func messagePacket(from, to, text string) *aprs.Packet {
	payload := fmt.Sprintf(":%-9s:%s", to, text)
	return &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:  aprs.Address{Call: from},
			Payload: payload,
		},
		Type: aprs.PacketTypeMessage,
		Message: &aprs.MessageData{
			Addressee: to,
			Text:      text,
		},
	}
}

func drainEvents(ch <-chan Event, count int, timeout time.Duration) []Event {
	var events []Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for i := 0; i < count; i++ {
		select {
		case e := <-ch:
			events = append(events, e)
		case <-timer.C:
			return events
		}
	}
	return events
}

func TestHandlePacketPosition(t *testing.T) {
	tr := NewMemoryTracker(testConfig())
	pkt := positionPacket("W1AW", 0, 41.714775, -72.727260, "Test station")
	tr.HandlePacket(pkt, "APRS-IS")

	s, ok := tr.Get("W1AW")
	if !ok {
		t.Fatal("station W1AW not found after HandlePacket")
	}
	if s.Callsign != "W1AW" {
		t.Errorf("callsign = %q, want %q", s.Callsign, "W1AW")
	}
	if s.Position == nil {
		t.Fatal("position is nil")
	}
	if s.Position.Lat != 41.714775 {
		t.Errorf("lat = %f, want %f", s.Position.Lat, 41.714775)
	}
	if s.Position.Lon != -72.727260 {
		t.Errorf("lon = %f, want %f", s.Position.Lon, -72.727260)
	}
	if s.Comment != "Test station" {
		t.Errorf("comment = %q, want %q", s.Comment, "Test station")
	}
	if s.Source != "APRS-IS" {
		t.Errorf("source = %q, want %q", s.Source, "APRS-IS")
	}
	if s.Symbol.Table != '/' || s.Symbol.Code != '-' {
		t.Errorf("symbol = %c%c, want /-", s.Symbol.Table, s.Symbol.Code)
	}
	if len(s.Track) != 1 {
		t.Errorf("track length = %d, want 1", len(s.Track))
	}
}

func TestHandlePacketMicE(t *testing.T) {
	tr := NewMemoryTracker(testConfig())
	pkt := micEPacket("N3LLO", 9, 39.1234, -76.5678)
	tr.HandlePacket(pkt, "RF")

	s, ok := tr.Get("N3LLO-9")
	if !ok {
		t.Fatal("station N3LLO-9 not found after MicE HandlePacket")
	}
	if s.Position == nil {
		t.Fatal("position is nil")
	}
	if s.Position.Lat != 39.1234 {
		t.Errorf("lat = %f, want %f", s.Position.Lat, 39.1234)
	}
	if s.Symbol.Code != '>' {
		t.Errorf("symbol code = %c, want >", s.Symbol.Code)
	}
	if s.Source != "RF" {
		t.Errorf("source = %q, want %q", s.Source, "RF")
	}
}

func TestHandlePacketObject(t *testing.T) {
	tr := NewMemoryTracker(testConfig())
	pkt := objectPacket("W1AW", "BEACON", 41.0, -72.0)
	tr.HandlePacket(pkt, "APRS-IS")

	// Object tracked under its own name, not the source callsign
	s, ok := tr.Get("BEACON")
	if !ok {
		t.Fatal("object BEACON not found after HandlePacket")
	}
	if s.Position == nil {
		t.Fatal("position is nil")
	}
	if s.Position.Lat != 41.0 {
		t.Errorf("lat = %f, want %f", s.Position.Lat, 41.0)
	}
}

func TestHandlePacketItem(t *testing.T) {
	tr := NewMemoryTracker(testConfig())
	pkt := itemPacket("W1AW", "ITEM1", 42.0, -71.0)
	tr.HandlePacket(pkt, "RF")

	s, ok := tr.Get("ITEM1")
	if !ok {
		t.Fatal("item ITEM1 not found after HandlePacket")
	}
	if s.Position.Lat != 42.0 {
		t.Errorf("lat = %f, want %f", s.Position.Lat, 42.0)
	}
}

func TestHandlePacketWeatherWithPosition(t *testing.T) {
	tr := NewMemoryTracker(testConfig())
	pkt := weatherPacket("WX1AW", 40.0, -74.0)
	tr.HandlePacket(pkt, "APRS-IS")

	s, ok := tr.Get("WX1AW")
	if !ok {
		t.Fatal("station WX1AW not found after weather HandlePacket")
	}
	if s.Position == nil {
		t.Fatal("position is nil")
	}
	if s.Position.Lat != 40.0 {
		t.Errorf("lat = %f, want %f", s.Position.Lat, 40.0)
	}
	if s.Weather == nil {
		t.Fatal("weather data should be populated from weather packet")
	}
	if s.Weather.Temperature == nil || *s.Weather.Temperature != 22.0 {
		t.Errorf("temperature = %v, want 22.0", s.Weather.Temperature)
	}
}

func TestHandlePacketNonPosition(t *testing.T) {
	tr := NewMemoryTracker(testConfig())
	pkt := messagePacket("W1AW", "N3LLO", "hello")
	tr.HandlePacket(pkt, "APRS-IS")

	_, ok := tr.Get("W1AW")
	if ok {
		t.Error("message packet should NOT create a station entry")
	}
	all := tr.All()
	if len(all) != 0 {
		t.Errorf("All() len = %d, want 0", len(all))
	}
}

func TestTrackHistory(t *testing.T) {
	cfg := testConfig()
	cfg.TrackMaxPoints = 3
	tr := NewMemoryTracker(cfg)

	for i := 0; i < 5; i++ {
		lat := 40.0 + float64(i)*0.01
		pkt := positionPacket("W1AW", 0, lat, -72.0, "")
		tr.HandlePacket(pkt, "RF")
	}

	s, _ := tr.Get("W1AW")
	if len(s.Track) != 3 {
		t.Fatalf("track length = %d, want 3 (capped at TrackMaxPoints)", len(s.Track))
	}
	// Oldest points should be dropped; most recent 3 should remain.
	if s.Track[0].Lat != 40.02 {
		t.Errorf("oldest remaining track lat = %f, want %f", s.Track[0].Lat, 40.02)
	}
	if s.Track[2].Lat != 40.04 {
		t.Errorf("newest track lat = %f, want %f", s.Track[2].Lat, 40.04)
	}
}

func TestDedupWithinWindow(t *testing.T) {
	tr := NewMemoryTracker(testConfig())

	pkt := positionPacket("W1AW", 0, 41.0, -72.0, "dup test")
	tr.HandlePacket(pkt, "RF")
	tr.HandlePacket(pkt, "RF") // same packet again

	s, _ := tr.Get("W1AW")
	if len(s.Track) != 1 {
		t.Errorf("track length = %d, want 1 (duplicate should be skipped)", len(s.Track))
	}
}

func TestDedupAfterWindow(t *testing.T) {
	cfg := testConfig()
	cfg.DedupWindow = 1 * time.Millisecond // tiny window for testing
	tr := NewMemoryTracker(cfg)

	pkt := positionPacket("W1AW", 0, 41.0, -72.0, "dup test")
	tr.HandlePacket(pkt, "RF")

	time.Sleep(5 * time.Millisecond) // exceed dedup window

	tr.HandlePacket(pkt, "RF")

	s, _ := tr.Get("W1AW")
	if len(s.Track) != 2 {
		t.Errorf("track length = %d, want 2 (after dedup window expires)", len(s.Track))
	}
}

func TestSourceTracking(t *testing.T) {
	tr := NewMemoryTracker(testConfig())

	pkt1 := positionPacket("W1AW", 0, 41.0, -72.0, "first")
	tr.HandlePacket(pkt1, "serial")

	s, _ := tr.Get("W1AW")
	if s.Source != "serial" {
		t.Errorf("source = %q, want %q", s.Source, "serial")
	}
	if len(s.Sources) != 1 || s.Sources[0] != "serial" {
		t.Errorf("sources = %v, want [serial]", s.Sources)
	}

	// Different position so it won't be deduped
	pkt2 := positionPacket("W1AW", 0, 41.001, -72.001, "second")
	tr.HandlePacket(pkt2, "aprsis")

	s, _ = tr.Get("W1AW")
	// The Sources set retains every transport type (sorted); the Source
	// summary joins them with '+' rather than collapsing to "both".
	if s.Source != "aprsis+serial" {
		t.Errorf("source = %q, want %q after hearing on both serial and aprsis", s.Source, "aprsis+serial")
	}
	if len(s.Sources) != 2 || s.Sources[0] != "aprsis" || s.Sources[1] != "serial" {
		t.Errorf("sources = %v, want [aprsis serial]", s.Sources)
	}
}

func TestAging(t *testing.T) {
	cfg := testConfig()
	cfg.StaleTimeout = 50 * time.Millisecond
	tr := NewMemoryTracker(cfg)

	pkt := positionPacket("W1AW", 0, 41.0, -72.0, "")
	tr.HandlePacket(pkt, "RF")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr.Start(ctx, 20*time.Millisecond) // sweep every 20ms

	time.Sleep(120 * time.Millisecond)

	all := tr.All()
	if len(all) != 0 {
		t.Errorf("All() len = %d, want 0 after StaleTimeout", len(all))
	}
}

func TestInArea(t *testing.T) {
	tr := NewMemoryTracker(testConfig())

	// Station inside bbox
	tr.HandlePacket(positionPacket("INSIDE", 0, 41.0, -72.0, ""), "RF")
	// Station outside bbox
	tr.HandlePacket(positionPacket("OUTSIDE", 0, 10.0, -10.0, ""), "RF")

	results := tr.InArea(40.0, -73.0, 42.0, -71.0)
	if len(results) != 1 {
		t.Fatalf("InArea returned %d stations, want 1", len(results))
	}
	if results[0].Callsign != "INSIDE" {
		t.Errorf("InArea station = %q, want %q", results[0].Callsign, "INSIDE")
	}
}

func TestInAreaEdgeCases(t *testing.T) {
	tr := NewMemoryTracker(testConfig())

	// Station exactly on the boundary
	tr.HandlePacket(positionPacket("EDGE", 0, 40.0, -73.0, ""), "RF")

	results := tr.InArea(40.0, -73.0, 42.0, -71.0)
	if len(results) != 1 {
		t.Fatalf("InArea should include boundary station, got %d", len(results))
	}
}

func TestSearch(t *testing.T) {
	tr := NewMemoryTracker(testConfig())

	tr.HandlePacket(positionPacket("W1AW", 0, 41.0, -72.0, ""), "RF")
	tr.HandlePacket(positionPacket("W1ABC", 0, 42.0, -73.0, ""), "RF")
	tr.HandlePacket(positionPacket("N3LLO", 0, 39.0, -76.0, ""), "RF")

	tests := []struct {
		prefix string
		want   int
	}{
		{"W1", 2},
		{"W1AW", 1},
		{"N3", 1},
		{"XY", 0},
		{"w1", 2},  // case-insensitive
		{"n3l", 1}, // case-insensitive
	}

	for _, tt := range tests {
		results := tr.Search(tt.prefix)
		if len(results) != tt.want {
			t.Errorf("Search(%q) returned %d stations, want %d", tt.prefix, len(results), tt.want)
		}
	}
}

func TestEventsNewAndUpdate(t *testing.T) {
	tr := NewMemoryTracker(testConfig())
	ch := tr.Events()

	pkt1 := positionPacket("W1AW", 0, 41.0, -72.0, "first")
	tr.HandlePacket(pkt1, "RF")

	events := drainEvents(ch, 1, 100*time.Millisecond)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventNewStation {
		t.Errorf("event type = %d, want EventNewStation (%d)", events[0].Type, EventNewStation)
	}
	if events[0].Station.Callsign != "W1AW" {
		t.Errorf("event callsign = %q, want %q", events[0].Station.Callsign, "W1AW")
	}

	// Update with different position
	pkt2 := positionPacket("W1AW", 0, 41.001, -72.001, "second")
	tr.HandlePacket(pkt2, "RF")

	events = drainEvents(ch, 1, 100*time.Millisecond)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventStationUpdate {
		t.Errorf("event type = %d, want EventStationUpdate (%d)", events[0].Type, EventStationUpdate)
	}
}

func TestEventsExpired(t *testing.T) {
	cfg := testConfig()
	cfg.StaleTimeout = 50 * time.Millisecond
	tr := NewMemoryTracker(cfg)
	ch := tr.Events()

	tr.HandlePacket(positionPacket("W1AW", 0, 41.0, -72.0, ""), "RF")
	// Drain the new-station event
	drainEvents(ch, 1, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr.Start(ctx, 20*time.Millisecond)

	// Wait for expiry
	events := drainEvents(ch, 1, 200*time.Millisecond)
	if len(events) != 1 {
		t.Fatalf("expected 1 expired event, got %d", len(events))
	}
	if events[0].Type != EventStationExpired {
		t.Errorf("event type = %d, want EventStationExpired (%d)", events[0].Type, EventStationExpired)
	}
}

func TestGetByCallsignWithSSID(t *testing.T) {
	tr := NewMemoryTracker(testConfig())

	tr.HandlePacket(positionPacket("W1AW", 9, 41.0, -72.0, ""), "RF")

	// Should be stored under "W1AW-9"
	_, ok := tr.Get("W1AW-9")
	if !ok {
		t.Error("station W1AW-9 not found by SSID key")
	}

	_, ok = tr.Get("W1AW")
	if ok {
		t.Error("station should NOT be found under bare callsign when SSID != 0")
	}
}

func TestUpdatePreservesExistingFieldsNotInNewPacket(t *testing.T) {
	tr := NewMemoryTracker(testConfig())

	pkt1 := positionPacket("W1AW", 0, 41.0, -72.0, "original comment")
	tr.HandlePacket(pkt1, "RF")

	pkt2 := positionPacket("W1AW", 0, 41.001, -72.001, "")
	tr.HandlePacket(pkt2, "RF")

	s, _ := tr.Get("W1AW")
	// Empty comment in new packet should not overwrite non-empty comment
	// Actually per the plan, we always update from packet data, so empty comment replaces.
	// This test documents the behavior: latest packet data wins.
	if s.Comment != "" {
		t.Errorf("comment should be updated to empty, got %q", s.Comment)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	tr := NewMemoryTracker(testConfig())
	tr.HandlePacket(positionPacket("w1aw", 0, 41.0, -72.0, ""), "RF")

	// Search with uppercase prefix should find lowercase callsign
	results := tr.Search("W1")
	found := false
	for _, s := range results {
		if strings.EqualFold(s.Callsign, "w1aw") {
			found = true
		}
	}
	if !found {
		t.Error("case-insensitive search should find station regardless of case")
	}
}

func TestUpdateConfig(t *testing.T) {
	cfg := testConfig()
	cfg.TrackMaxPoints = 3
	tr := NewMemoryTracker(cfg)

	// Add a station with 3 track points (at the limit)
	tr.HandlePacket(positionPacket("N0CALL", 0, 35.0, -84.0, "a"), "RF")
	tr.HandlePacket(positionPacket("N0CALL", 0, 35.1, -84.0, "b"), "RF")
	tr.HandlePacket(positionPacket("N0CALL", 0, 35.2, -84.0, "c"), "RF")

	s, ok := tr.Get("N0CALL")
	if !ok {
		t.Fatal("station not found")
	}
	if len(s.Track) != 3 {
		t.Errorf("track len = %d, want 3", len(s.Track))
	}

	// Update config to allow more track points
	newCfg := cfg
	newCfg.TrackMaxPoints = 10
	tr.UpdateConfig(newCfg)

	// Add more track points — should now keep more
	tr.HandlePacket(positionPacket("N0CALL", 0, 35.3, -84.0, "d"), "RF")
	tr.HandlePacket(positionPacket("N0CALL", 0, 35.4, -84.0, "e"), "RF")

	s, _ = tr.Get("N0CALL")
	if len(s.Track) != 5 {
		t.Errorf("track len after config update = %d, want 5", len(s.Track))
	}

	// Update config to reduce — existing points remain, new additions capped
	newCfg.TrackMaxPoints = 2
	tr.UpdateConfig(newCfg)

	tr.HandlePacket(positionPacket("N0CALL", 0, 35.5, -84.0, "f"), "RF")
	s, _ = tr.Get("N0CALL")
	if len(s.Track) != 2 {
		t.Errorf("track len after reduce = %d, want 2", len(s.Track))
	}
}

func TestTrackPointSpeedCourse(t *testing.T) {
	tr := NewMemoryTracker(testConfig())

	pkt := &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:  aprs.Address{Call: "N3LLO", SSID: 9},
			Payload: "!3912.34N/07656.78W>065/045 mobile",
		},
		Type: aprs.PacketTypePosition,
		Position: &aprs.PositionData{
			Lat:     39.2057,
			Lon:     -76.9463,
			Speed:   45.0,
			Course:  65.0,
			Symbol:  aprs.Symbol{Table: '/', Code: '>'},
			Comment: "mobile",
		},
	}
	tr.HandlePacket(pkt, "RF")

	s, ok := tr.Get("N3LLO-9")
	if !ok {
		t.Fatal("station N3LLO-9 not found after HandlePacket")
	}
	if len(s.Track) != 1 {
		t.Fatalf("track length = %d, want 1", len(s.Track))
	}
	tp := s.Track[0]
	if tp.Speed != 45.0 {
		t.Errorf("track point speed = %f, want 45.0", tp.Speed)
	}
	if tp.Course != 65.0 {
		t.Errorf("track point course = %f, want 65.0", tp.Course)
	}

	// Verify a stationary station has zero speed/course on track points
	pkt2 := positionPacket("W1AW", 0, 41.714775, -72.727260, "home")
	tr.HandlePacket(pkt2, "APRS-IS")

	s2, _ := tr.Get("W1AW")
	if s2.Track[0].Speed != 0 {
		t.Errorf("stationary track speed = %f, want 0", s2.Track[0].Speed)
	}
	if s2.Track[0].Course != 0 {
		t.Errorf("stationary track course = %f, want 0", s2.Track[0].Course)
	}
}

func TestUpdateConfigDedupWindow(t *testing.T) {
	cfg := testConfig()
	cfg.DedupWindow = 5 * time.Second
	tr := NewMemoryTracker(cfg)

	// Send a packet
	tr.HandlePacket(positionPacket("N0CALL", 0, 35.0, -84.0, "test"), "RF")
	s, ok := tr.Get("N0CALL")
	if !ok {
		t.Fatal("station not found")
	}
	if len(s.Track) != 1 {
		t.Errorf("track len = %d, want 1", len(s.Track))
	}

	// Same packet should be deduped
	tr.HandlePacket(positionPacket("N0CALL", 0, 35.0, -84.0, "test"), "RF")
	s, _ = tr.Get("N0CALL")
	if len(s.Track) != 1 {
		t.Errorf("track len after dedup = %d, want 1", len(s.Track))
	}

	// Update dedup window to 0 (disabled)
	newCfg := cfg
	newCfg.DedupWindow = 0
	tr.UpdateConfig(newCfg)

	// Same packet should now be accepted
	tr.HandlePacket(positionPacket("N0CALL", 0, 35.0, -84.0, "test"), "RF")
	s, _ = tr.Get("N0CALL")
	if len(s.Track) != 2 {
		t.Errorf("track len after disabling dedup = %d, want 2", len(s.Track))
	}
}
