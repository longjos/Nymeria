package object

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

func TestFormatLatitude(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		want string
	}{
		{name: "north", lat: 49.0583333, want: "4903.50N"},
		{name: "south", lat: -34.15, want: "3409.00S"},
		{name: "equator", lat: 0.0, want: "0000.00N"},
		{name: "high north", lat: 89.999, want: "8959.94N"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatLatitude(tt.lat)
			if got != tt.want {
				t.Errorf("FormatLatitude(%f) = %q, want %q", tt.lat, got, tt.want)
			}
		})
	}
}

func TestFormatLongitude(t *testing.T) {
	tests := []struct {
		name string
		lon  float64
		want string
	}{
		{name: "west", lon: -72.0291667, want: "07201.75W"},
		{name: "east", lon: 11.95, want: "01157.00E"},
		{name: "zero", lon: 0.0, want: "00000.00E"},
		{name: "far west", lon: -179.999, want: "17959.94W"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatLongitude(tt.lon)
			if got != tt.want {
				t.Errorf("FormatLongitude(%f) = %q, want %q", tt.lon, got, tt.want)
			}
		})
	}
}

func TestFormatObjectPayload(t *testing.T) {
	tests := []struct {
		name      string
		obj       Object
		wantStart string // verify prefix
		wantLive  byte   // * or _
	}{
		{
			name: "live object with short name",
			obj: Object{
				Name:    "TORNADO",
				Lat:     49.0583333,
				Lon:     -72.0291667,
				Symbol:  aprs.Symbol{Table: '/', Code: '@'},
				Comment: "F3 tornado",
				Live:    true,
			},
			wantStart: ";TORNADO  *",
			wantLive:  '*',
		},
		{
			name: "killed object",
			obj: Object{
				Name:    "TORNADO",
				Lat:     49.0583333,
				Lon:     -72.0291667,
				Symbol:  aprs.Symbol{Table: '/', Code: '@'},
				Comment: "F3 tornado",
				Live:    false,
			},
			wantStart: ";TORNADO  \\",
			wantLive:  '\\',
		},
		{
			name: "9-char name",
			obj: Object{
				Name:    "123456789",
				Lat:     49.0583333,
				Lon:     -72.0291667,
				Symbol:  aprs.Symbol{Table: '/', Code: '-'},
				Comment: "test",
				Live:    true,
			},
			wantStart: ";123456789*",
			wantLive:  '*',
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatObjectPayload(tt.obj)
			if !strings.HasPrefix(got, tt.wantStart) {
				t.Errorf("FormatObjectPayload() prefix = %q, want prefix %q", got[:len(tt.wantStart)], tt.wantStart)
			}
			// Verify position is present after timestamp (7 chars)
			// Payload: ;name(9)indicator(1)timestamp(7)position(19)comment
			if len(got) < 37 {
				t.Fatalf("payload too short: %d chars", len(got))
			}
		})
	}
}

func TestFormatItemPayload(t *testing.T) {
	tests := []struct {
		name      string
		item      Item
		wantStart string
	}{
		{
			name: "live item",
			item: Item{
				Name:    "FUEL",
				Lat:     49.0583333,
				Lon:     -72.0291667,
				Symbol:  aprs.Symbol{Table: '/', Code: '-'},
				Comment: "gas station",
				Live:    true,
			},
			wantStart: ")FUEL!",
		},
		{
			name: "killed item",
			item: Item{
				Name:    "FUEL",
				Lat:     49.0583333,
				Lon:     -72.0291667,
				Symbol:  aprs.Symbol{Table: '/', Code: '-'},
				Comment: "gas station",
				Live:    false,
			},
			wantStart: ")FUEL_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatItemPayload(tt.item)
			if !strings.HasPrefix(got, tt.wantStart) {
				t.Errorf("FormatItemPayload() = %q, want prefix %q", got, tt.wantStart)
			}
			// Verify position data follows the name+indicator
			// Should contain latitude and longitude
			if !strings.Contains(got, "N") && !strings.Contains(got, "S") {
				t.Error("missing N/S indicator in position")
			}
		})
	}
}

func TestFormatObjectPayloadRoundtrip(t *testing.T) {
	obj := Object{
		Name:    "TORNADO",
		Lat:     49.0583333,
		Lon:     -72.0291667,
		Symbol:  aprs.Symbol{Table: '/', Code: '@'},
		Comment: "F3 tornado",
		Live:    true,
	}

	payload := FormatObjectPayload(obj)

	// Parse it back using the APRS parser
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     payload,
	}

	parser := aprs.NewParser()
	pkt, err := parser.Parse(frame)
	if err != nil {
		t.Fatalf("round-trip parse error: %v", err)
	}

	if pkt.Type != aprs.PacketTypeObject {
		t.Fatalf("expected PacketTypeObject, got %d", pkt.Type)
	}
	if pkt.Object == nil {
		t.Fatal("parsed object is nil")
	}
	if pkt.Object.Name != "TORNADO" {
		t.Errorf("name = %q, want %q", pkt.Object.Name, "TORNADO")
	}
	if !pkt.Object.Live {
		t.Error("expected live object")
	}
	// Position should be approximately correct (format precision limits)
	if diff := abs(pkt.Object.Position.Lat - 49.0583333); diff > 0.002 {
		t.Errorf("lat = %f, want ~49.058", pkt.Object.Position.Lat)
	}
	if diff := abs(pkt.Object.Position.Lon - (-72.0291667)); diff > 0.002 {
		t.Errorf("lon = %f, want ~-72.029", pkt.Object.Position.Lon)
	}
}

func TestFormatItemPayloadRoundtrip(t *testing.T) {
	item := Item{
		Name:    "FUEL",
		Lat:     49.0583333,
		Lon:     -72.0291667,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Comment: "gas station",
		Live:    true,
	}

	payload := FormatItemPayload(item)

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     payload,
	}

	parser := aprs.NewParser()
	pkt, err := parser.Parse(frame)
	if err != nil {
		t.Fatalf("round-trip parse error: %v", err)
	}

	if pkt.Type != aprs.PacketTypeItem {
		t.Fatalf("expected PacketTypeItem, got %d", pkt.Type)
	}
	if pkt.Item == nil {
		t.Fatal("parsed item is nil")
	}
	if pkt.Item.Name != "FUEL" {
		t.Errorf("name = %q, want %q", pkt.Item.Name, "FUEL")
	}
	if !pkt.Item.Live {
		t.Error("expected live item")
	}
}

func TestValidateObjectName(t *testing.T) {
	tests := []struct {
		name    string
		objName string
		wantErr bool
	}{
		{name: "valid short", objName: "TEST", wantErr: false},
		{name: "valid 9 chars", objName: "123456789", wantErr: false},
		{name: "valid 1 char", objName: "A", wantErr: false},
		{name: "empty", objName: "", wantErr: true},
		{name: "too long", objName: "1234567890", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateObjectName(tt.objName)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateItemName(t *testing.T) {
	tests := []struct {
		name     string
		itemName string
		wantErr  bool
	}{
		{name: "valid 4 chars", itemName: "FUEL", wantErr: false},
		{name: "valid 9 chars", itemName: "123456789", wantErr: false},
		{name: "valid 1 char", itemName: "A", wantErr: false},
		{name: "empty", itemName: "", wantErr: true},
		{name: "too long", itemName: "1234567890", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateItemName(tt.itemName)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestManagerCreateObject(t *testing.T) {
	sent := make(chan aprs.APRSFrame, 10)
	sendFunc := func(frame aprs.APRSFrame) error {
		sent <- frame
		return nil
	}

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	obj, err := mgr.CreateObject(Object{
		Name:    "TORNADO",
		Lat:     49.0583333,
		Lon:     -72.0291667,
		Symbol:  aprs.Symbol{Table: '/', Code: '@'},
		Comment: "F3 tornado",
		Live:    true,
	})
	if err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	if obj.ID == "" {
		t.Error("expected non-empty ID")
	}
	if obj.Name != "TORNADO" {
		t.Errorf("name = %q, want %q", obj.Name, "TORNADO")
	}

	// Should have sent the frame
	select {
	case frame := <-sent:
		if frame.Source.Call != "N0CALL" {
			t.Errorf("source = %q, want %q", frame.Source.Call, "N0CALL")
		}
		if frame.Destination.Call != "APNMRA" {
			t.Errorf("destination = %q, want %q", frame.Destination.Call, "APNMRA")
		}
		if got := aprs.FormatPath(frame.Path); got != "WIDE1-1,WIDE2-1" {
			t.Errorf("default path = %q, want WIDE1-1,WIDE2-1", got)
		}
		if !strings.HasPrefix(frame.Payload, ";TORNADO") {
			t.Errorf("payload = %q, expected object format", frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sent frame")
	}
}

func TestManagerCreateObjectCustomPath(t *testing.T) {
	sent := make(chan aprs.APRSFrame, 10)
	sendFunc := func(frame aprs.APRSFrame) error {
		sent <- frame
		return nil
	}
	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: time.Hour,
	})
	defer mgr.Close()
	path, err := aprs.ParsePath("WIDE1-1")
	if err != nil {
		t.Fatal(err)
	}
	mgr.UpdatePath(path)

	_, err = mgr.CreateObject(Object{
		Name:    "TORNADO",
		Lat:     49.0583333,
		Lon:     -72.0291667,
		Symbol:  aprs.Symbol{Table: '/', Code: '@'},
		Comment: "F3 tornado",
		Live:    true,
	})
	if err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	select {
	case frame := <-sent:
		if got := aprs.FormatPath(frame.Path); got != "WIDE1-1" {
			t.Errorf("path = %q, want WIDE1-1", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sent frame")
	}
}

func TestManagerCreateItem(t *testing.T) {
	sent := make(chan aprs.APRSFrame, 10)
	sendFunc := func(frame aprs.APRSFrame) error {
		sent <- frame
		return nil
	}

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	item, err := mgr.CreateItem(Item{
		Name:    "FUEL",
		Lat:     49.0583333,
		Lon:     -72.0291667,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Comment: "gas station",
		Live:    true,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	if item.ID == "" {
		t.Error("expected non-empty ID")
	}

	select {
	case frame := <-sent:
		if !strings.HasPrefix(frame.Payload, ")FUEL!") {
			t.Errorf("payload = %q, expected item format", frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sent frame")
	}
}

func TestManagerKillObject(t *testing.T) {
	sent := make(chan aprs.APRSFrame, 10)
	sendFunc := func(frame aprs.APRSFrame) error {
		sent <- frame
		return nil
	}

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	obj, _ := mgr.CreateObject(Object{
		Name:    "TORNADO",
		Lat:     49.0583333,
		Lon:     -72.0291667,
		Symbol:  aprs.Symbol{Table: '/', Code: '@'},
		Comment: "F3 tornado",
		Live:    true,
	})

	// Drain the create frame
	<-sent

	err := mgr.KillObject(obj.ID)
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}

	// Should send a kill frame with '\' indicator (APRS101 spec for killed objects)
	select {
	case frame := <-sent:
		if !strings.Contains(frame.Payload, "\\") {
			t.Errorf("kill payload should contain '\\' indicator: %q", frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for kill frame")
	}

	// Verify the object is marked as killed in the list
	objs := mgr.OwnObjects()
	found := false
	for _, o := range objs {
		if o.ID == obj.ID {
			found = true
			if o.Live {
				t.Error("expected object to be killed")
			}
		}
	}
	if !found {
		t.Error("killed object not found in list")
	}
}

func TestManagerDeleteObject(t *testing.T) {
	sendFunc := func(frame aprs.APRSFrame) error { return nil }

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	obj, _ := mgr.CreateObject(Object{
		Name:    "TEST",
		Lat:     49.0,
		Lon:     -72.0,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Comment: "",
		Live:    true,
	})

	err := mgr.DeleteObject(obj.ID)
	if err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}

	objs := mgr.OwnObjects()
	for _, o := range objs {
		if o.ID == obj.ID {
			t.Error("expected object to be removed from list after delete")
		}
	}
}

func TestManagerOwnObjects(t *testing.T) {
	sendFunc := func(frame aprs.APRSFrame) error { return nil }

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	// Create 3 objects
	for i, name := range []string{"OBJ1", "OBJ2", "OBJ3"} {
		_, err := mgr.CreateObject(Object{
			Name:    name,
			Lat:     float64(49 + i),
			Lon:     -72.0,
			Symbol:  aprs.Symbol{Table: '/', Code: '-'},
			Comment: "",
			Live:    true,
		})
		if err != nil {
			t.Fatalf("CreateObject %s: %v", name, err)
		}
	}

	objs := mgr.OwnObjects()
	if len(objs) != 3 {
		t.Errorf("expected 3 objects, got %d", len(objs))
	}
}

func TestManagerHandlePacketObject(t *testing.T) {
	sendFunc := func(frame aprs.APRSFrame) error { return nil }

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	// Simulate receiving an object from another station
	pkt := &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:      aprs.Address{Call: "W3ADO"},
			Destination: aprs.Address{Call: "APRS"},
			Payload:     ";TORNADO  *092345z4903.50N/07201.75W@F3 tornado",
		},
		Type: aprs.PacketTypeObject,
		Object: &aprs.ObjectData{
			Name:      "TORNADO",
			Live:      true,
			Timestamp: time.Now(),
			Position: aprs.PositionData{
				Lat:    49.0583333,
				Lon:    -72.0291667,
				Symbol: aprs.Symbol{Table: '/', Code: '@'},
			},
		},
	}

	mgr.HandlePacket(pkt)

	received := mgr.ReceivedObjects()
	if len(received) != 1 {
		t.Fatalf("expected 1 received object, got %d", len(received))
	}
	if received[0].Name != "TORNADO" {
		t.Errorf("name = %q, want %q", received[0].Name, "TORNADO")
	}
	if received[0].OwnerCall != "W3ADO" {
		t.Errorf("owner = %q, want %q", received[0].OwnerCall, "W3ADO")
	}
}

func TestManagerHandlePacketItem(t *testing.T) {
	sendFunc := func(frame aprs.APRSFrame) error { return nil }

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	pkt := &aprs.Packet{
		Frame: aprs.APRSFrame{
			Source:      aprs.Address{Call: "W3ADO"},
			Destination: aprs.Address{Call: "APRS"},
			Payload:     ")FUEL!4903.50N/07201.75W-gas station",
		},
		Type: aprs.PacketTypeItem,
		Item: &aprs.ItemData{
			Name: "FUEL",
			Live: true,
			Position: aprs.PositionData{
				Lat:    49.0583333,
				Lon:    -72.0291667,
				Symbol: aprs.Symbol{Table: '/', Code: '-'},
			},
		},
	}

	mgr.HandlePacket(pkt)

	received := mgr.ReceivedItems()
	if len(received) != 1 {
		t.Fatalf("expected 1 received item, got %d", len(received))
	}
	if received[0].Name != "FUEL" {
		t.Errorf("name = %q, want %q", received[0].Name, "FUEL")
	}
}

func TestManagerRetransmit(t *testing.T) {
	var sentCount atomic.Int64
	sendFunc := func(frame aprs.APRSFrame) error {
		sentCount.Add(1)
		return nil
	}

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 100 * time.Millisecond, // fast for testing
	})

	_, err := mgr.CreateObject(Object{
		Name:    "TEST",
		Lat:     49.0,
		Lon:     -72.0,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Comment: "",
		Live:    true,
	})
	if err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	// Start the retransmit loop
	ctx, cancel := context.WithCancel(context.Background())
	mgr.Start(ctx)

	// Wait for a few retransmissions
	time.Sleep(350 * time.Millisecond)
	cancel()
	mgr.Close()

	// Should have initial send + at least 2 retransmissions
	count := sentCount.Load()
	if count < 3 {
		t.Errorf("expected at least 3 transmissions, got %d", count)
	}
}

func TestManagerEvents(t *testing.T) {
	sendFunc := func(frame aprs.APRSFrame) error { return nil }

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	// Create an object
	_, err := mgr.CreateObject(Object{
		Name:    "TEST",
		Lat:     49.0,
		Lon:     -72.0,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Comment: "",
		Live:    true,
	})
	if err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	// Should receive an event
	select {
	case evt := <-mgr.Events():
		if evt.Type != "object_created" {
			t.Errorf("event type = %q, want %q", evt.Type, "object_created")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestManagerDuplicateObjectName(t *testing.T) {
	sendFunc := func(frame aprs.APRSFrame) error { return nil }

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	_, err := mgr.CreateObject(Object{
		Name:    "TEST",
		Lat:     49.0,
		Lon:     -72.0,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Live:    true,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Creating another with the same name should return an error
	_, err = mgr.CreateObject(Object{
		Name:    "TEST",
		Lat:     50.0,
		Lon:     -73.0,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Live:    true,
	})
	if err == nil {
		t.Error("expected error for duplicate object name, got nil")
	}
}

func TestManagerCreateObjectWithSSID(t *testing.T) {
	sent := make(chan aprs.APRSFrame, 10)
	sendFunc := func(frame aprs.APRSFrame) error {
		sent <- frame
		return nil
	}

	mgr := NewManager("N0CALL", 5, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	_, err := mgr.CreateObject(Object{
		Name:   "TEST",
		Lat:    49.0,
		Lon:    -72.0,
		Symbol: aprs.Symbol{Table: '/', Code: '-'},
		Live:   true,
	})
	if err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	select {
	case frame := <-sent:
		if frame.Source.SSID != 5 {
			t.Errorf("source SSID = %d, want 5", frame.Source.SSID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sent frame")
	}
}

func TestFormatKilledObjectRoundtrip(t *testing.T) {
	obj := Object{
		Name:    "SHELTER1",
		Lat:     49.0583333,
		Lon:     -72.0291667,
		Symbol:  aprs.Symbol{Table: '/', Code: '@'},
		Comment: "closed",
		Live:    false,
	}

	payload := FormatObjectPayload(obj)

	// Verify it starts with killed indicator
	if payload[10] != '\\' {
		t.Errorf("killed indicator = %c, want '\\'", payload[10])
	}

	// Parse it back
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APNMRA"},
		Payload:     payload,
	}

	parser := aprs.NewParser()
	pkt, err := parser.Parse(frame)
	if err != nil {
		t.Fatalf("round-trip parse error: %v", err)
	}

	if pkt.Type != aprs.PacketTypeObject {
		t.Fatalf("expected PacketTypeObject, got %d", pkt.Type)
	}
	if pkt.Object.Live {
		t.Error("expected killed object, got live")
	}
	if pkt.Object.Name != "SHELTER1" {
		t.Errorf("name = %q, want %q", pkt.Object.Name, "SHELTER1")
	}
}

func TestFormatTimestamp(t *testing.T) {
	// Use a fixed time for deterministic testing
	ts := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
	got := FormatTimestamp(ts)
	want := "151430z"
	if got != want {
		t.Errorf("FormatTimestamp() = %q, want %q", got, want)
	}
}

func TestManagerUpdateStationInfo(t *testing.T) {
	sent := make(chan aprs.APRSFrame, 10)
	sendFunc := func(frame aprs.APRSFrame) error {
		sent <- frame
		return nil
	}

	mgr := NewManager("N0CALL", 0, sendFunc, ManagerConfig{
		RetransmitInterval: 10 * time.Minute,
	})
	defer mgr.Close()

	// Create object with original callsign
	_, err := mgr.CreateObject(Object{
		Name:    "TEST",
		Lat:     35.0,
		Lon:     -84.0,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Comment: "test",
		Live:    true,
	})
	if err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	select {
	case frame := <-sent:
		if frame.Source.Call != "N0CALL" || frame.Source.SSID != 0 {
			t.Errorf("source = %v, want N0CALL-0", frame.Source)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// Update station info
	mgr.UpdateStationInfo("W1AW", 9)

	// Create another object — should use new callsign
	_, err = mgr.CreateObject(Object{
		Name:    "TEST2",
		Lat:     35.0,
		Lon:     -84.0,
		Symbol:  aprs.Symbol{Table: '/', Code: '-'},
		Comment: "test2",
		Live:    true,
	})
	if err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	select {
	case frame := <-sent:
		if frame.Source.Call != "W1AW" || frame.Source.SSID != 9 {
			t.Errorf("source after update = %v, want W1AW-9", frame.Source)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
