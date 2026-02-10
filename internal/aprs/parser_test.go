package aprs

import (
	"math"
	"testing"
)

// approxEqual checks if two floats are within tolerance.
func approxEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}

func TestPacketTypeString(t *testing.T) {
	tests := []struct {
		pt   PacketType
		want string
	}{
		{PacketTypeUnknown, "unknown"},
		{PacketTypePosition, "position"},
		{PacketTypeMessage, "message"},
		{PacketTypeObject, "object"},
		{PacketTypeItem, "item"},
		{PacketTypeWeather, "weather"},
		{PacketTypeStatus, "status"},
		{PacketTypeTelemetry, "telemetry"},
		{PacketTypeMicE, "micE"},
		{PacketTypeQuery, "query"},
		{PacketTypeThirdParty, "thirdParty"},
		{PacketType(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.pt.String(); got != tt.want {
			t.Errorf("PacketType(%d).String() = %q, want %q", tt.pt, got, tt.want)
		}
	}
}

func TestParseFrame(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantSrc string
		wantDst string
		wantPath []string
		wantBody string
		wantErr  bool
	}{
		{
			name:    "simple frame no path",
			raw:     "N0CALL>APRS:!4903.50N/07201.75W-",
			wantSrc: "N0CALL",
			wantDst: "APRS",
			wantBody: "!4903.50N/07201.75W-",
		},
		{
			name:     "frame with path",
			raw:      "N0CALL-5>APRS,RELAY,WIDE:!4903.50N/07201.75W-",
			wantSrc:  "N0CALL-5",
			wantDst:  "APRS",
			wantPath: []string{"RELAY", "WIDE"},
			wantBody: "!4903.50N/07201.75W-",
		},
		{
			name:     "frame with SSID in destination",
			raw:      "N0CALL-2>APRS-3,WIDE1-1:>status",
			wantSrc:  "N0CALL-2",
			wantDst:  "APRS-3",
			wantPath: []string{"WIDE1-1"},
			wantBody: ">status",
		},
		{
			name:    "no colon",
			raw:     "N0CALL>APRS",
			wantErr: true,
		},
		{
			name:    "empty string",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "no > separator",
			raw:     "N0CALL:payload",
			wantErr: true,
		},
		{
			name:    "colon in payload preserved",
			raw:     "N0CALL>APRS::N0CALL-5 :Hello{123",
			wantSrc: "N0CALL",
			wantDst: "APRS",
			wantBody: ":N0CALL-5 :Hello{123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := ParseFrame(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if frame.Source.String() != tt.wantSrc {
				t.Errorf("source = %q, want %q", frame.Source.String(), tt.wantSrc)
			}
			if frame.Destination.String() != tt.wantDst {
				t.Errorf("destination = %q, want %q", frame.Destination.String(), tt.wantDst)
			}
			if len(tt.wantPath) > 0 {
				if len(frame.Path) != len(tt.wantPath) {
					t.Fatalf("path len = %d, want %d", len(frame.Path), len(tt.wantPath))
				}
				for i, p := range tt.wantPath {
					if frame.Path[i].String() != p {
						t.Errorf("path[%d] = %q, want %q", i, frame.Path[i].String(), p)
					}
				}
			}
			if frame.Payload != tt.wantBody {
				t.Errorf("payload = %q, want %q", frame.Payload, tt.wantBody)
			}
		})
	}
}

func TestParsePositionUncompressed(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name        string
		payload     string
		wantLat     float64
		wantLon     float64
		wantSymTbl  byte
		wantSymCode byte
		wantComment string
		wantCourse  float64
		wantSpeed   float64
		wantHasTime bool
		wantType    PacketType
		wantErr     bool
	}{
		{
			name:        "position without timestamp !",
			payload:     "!4903.50N/07201.75W-",
			wantLat:     49.05833,
			wantLon:     -72.02917,
			wantSymTbl:  '/',
			wantSymCode: '-',
			wantType:    PacketTypePosition,
		},
		{
			name:        "position with messaging =",
			payload:     "=4903.50N/07201.75W-Test 001",
			wantLat:     49.05833,
			wantLon:     -72.02917,
			wantSymTbl:  '/',
			wantSymCode: '-',
			wantComment: "Test 001",
			wantType:    PacketTypePosition,
		},
		{
			name:        "position with timestamp /",
			payload:     "/092345z4903.50N/07201.75W>",
			wantLat:     49.05833,
			wantLon:     -72.02917,
			wantSymTbl:  '/',
			wantSymCode: '>',
			wantHasTime: true,
			wantType:    PacketTypePosition,
		},
		{
			name:        "position with timestamp and CSE/SPD @",
			payload:     "@092345z4903.50N/07201.75W>088/036",
			wantLat:     49.05833,
			wantLon:     -72.02917,
			wantSymTbl:  '/',
			wantSymCode: '>',
			wantHasTime: true,
			wantCourse:  88,
			wantSpeed:   36 * 1.852, // knots to km/h
			wantType:    PacketTypePosition,
		},
		{
			name:        "altitude in comment",
			payload:     "!4903.50N/07201.75W-/A=001234",
			wantLat:     49.05833,
			wantLon:     -72.02917,
			wantSymTbl:  '/',
			wantSymCode: '-',
			wantType:    PacketTypePosition,
		},
		{
			name:        "position ambiguity level 1",
			payload:     "!4903.5 N/07201.7 W-",
			wantLat:     49.05833,
			wantLon:     -72.02833,
			wantSymTbl:  '/',
			wantSymCode: '-',
			wantType:    PacketTypePosition,
		},
		{
			name:    "payload too short",
			payload: "!49",
			wantErr: true,
		},
		{
			name:        "south and east coordinates",
			payload:     "!4903.50S/07201.75E-",
			wantLat:     -49.05833,
			wantLon:     72.02917,
			wantSymTbl:  '/',
			wantSymCode: '-',
			wantType:    PacketTypePosition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Position == nil {
				t.Fatal("position is nil")
			}
			if !approxEqual(pkt.Position.Lat, tt.wantLat, 0.001) {
				t.Errorf("lat = %f, want %f", pkt.Position.Lat, tt.wantLat)
			}
			if !approxEqual(pkt.Position.Lon, tt.wantLon, 0.001) {
				t.Errorf("lon = %f, want %f", pkt.Position.Lon, tt.wantLon)
			}
			if pkt.Position.Symbol.Table != tt.wantSymTbl {
				t.Errorf("symbol table = %c, want %c", pkt.Position.Symbol.Table, tt.wantSymTbl)
			}
			if pkt.Position.Symbol.Code != tt.wantSymCode {
				t.Errorf("symbol code = %c, want %c", pkt.Position.Symbol.Code, tt.wantSymCode)
			}
			if tt.wantComment != "" && pkt.Position.Comment != tt.wantComment {
				t.Errorf("comment = %q, want %q", pkt.Position.Comment, tt.wantComment)
			}
			if tt.wantHasTime && pkt.Position.Timestamp.IsZero() {
				t.Error("expected non-zero timestamp")
			}
			if tt.wantCourse > 0 && !approxEqual(pkt.Position.Course, tt.wantCourse, 0.1) {
				t.Errorf("course = %f, want %f", pkt.Position.Course, tt.wantCourse)
			}
			if tt.wantSpeed > 0 && !approxEqual(pkt.Position.Speed, tt.wantSpeed, 0.1) {
				t.Errorf("speed = %f, want %f", pkt.Position.Speed, tt.wantSpeed)
			}
		})
	}
}

func TestParsePositionCompressed(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name        string
		payload     string
		wantLat     float64
		wantLon     float64
		wantSymTbl  byte
		wantSymCode byte
		wantType    PacketType
	}{
		{
			// Compressed position from APRS101 spec example
			// DTI '!' + compressed data: /5L!!<*e7>7P[
			// '/' = symbol table
			// 5L!! = base91 lat, <*e7 = base91 lon
			// '>' = symbol code
			// 7P[ = cs + type
			name:    "compressed position !/5L!!<*e7>7P[",
			payload: "!/5L!!<*e7>7P[",
			// Lat: 5L!! -> (53-33)*91^3 + (76-33)*91^2 + (33-33)*91 + (33-33) = 20*753571 + 43*8281 = 15071420 + 356083 = 15427503
			// Lat = 90 - 15427503/380926.0 = 90 - 40.5028 = 49.497
			// Lon: <*e7 -> (60-33)*91^3 + (42-33)*91^2 + (101-33)*91 + (55-33)
			// = 27*753571 + 9*8281 + 68*91 + 22 = 20346417 + 74529 + 6188 + 22 = 20427156
			// Lon = -180 + 20427156/190463.0 = -180 + 107.247 = -72.753
			wantLat:     49.5,
			wantLon:     -72.75,
			wantSymTbl:  '/',
			wantSymCode: '>',
			wantType:    PacketTypePosition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Position == nil {
				t.Fatal("position is nil")
			}
			if !approxEqual(pkt.Position.Lat, tt.wantLat, 0.1) {
				t.Errorf("lat = %f, want %f", pkt.Position.Lat, tt.wantLat)
			}
			if !approxEqual(pkt.Position.Lon, tt.wantLon, 0.1) {
				t.Errorf("lon = %f, want %f", pkt.Position.Lon, tt.wantLon)
			}
		})
	}
}

func TestParseCompressedSupersonicSpeed(t *testing.T) {
	parser := NewParser()

	// Build a compressed position with high s value to verify supersonic speeds.
	// Using the same lat/lon as existing compressed test: /5L!!<*e7
	// Symbol table '/', symbol code '>'
	// c=45 -> course = 180 degrees. c byte = 45+33 = 78 = 'N'
	// s=90 -> speed = 1.08^90 - 1 = ~917 knots = ~1699 km/h (supersonic)
	// s byte = 90+33 = 123 = '{'
	// type byte: nmeaSrc=0 (bits 3-4 = 00), so type = 0x20 = 32+33 = 65 = 'A'
	// But we need nmeaSrc != 2 for course/speed. nmeaSrc = (t>>3)&0x03.
	// type=0x20 (32 decimal): bits 4-3 = 10 -> nmeaSrc=2 -> skips course/speed!
	// Let's use type=0x30 (48): bits 4-3 = 01 -> nmeaSrc=1. 48+33=81='Q'
	// Actually let's compute: we want nmeaSrc=0. t byte should have bits 4-3 = 00.
	// t=0 -> 0+33=33='!'. nmeaSrc=(0>>3)&3=0. Good.
	// So: c='N', s='{', type='!'
	payload := "!/5L!!<*e7>N{!"

	frame := APRSFrame{
		Source:      Address{Call: "N0CALL"},
		Destination: Address{Call: "APRS"},
		Payload:     payload,
	}
	pkt, err := parser.Parse(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkt.Position == nil {
		t.Fatal("position is nil")
	}

	// Expected course: 45 * 4.0 = 180 degrees
	if !approxEqual(pkt.Position.Course, 180.0, 0.1) {
		t.Errorf("course = %f, want 180.0", pkt.Position.Course)
	}

	// Expected speed: (1.08^90 - 1) * 1.852 km/h
	// 1.08^90 ≈ 917.6, so speed ≈ 917.6 * 1.852 ≈ 1699.3 km/h
	expectedSpeed := (math.Pow(1.08, 90) - 1.0) * 1.852
	if !approxEqual(pkt.Position.Speed, expectedSpeed, 1.0) {
		t.Errorf("speed = %f, want %f (supersonic)", pkt.Position.Speed, expectedSpeed)
	}

	// Verify it's actually supersonic (> speed of sound ~1235 km/h)
	if pkt.Position.Speed < 1235.0 {
		t.Errorf("speed %f km/h is not supersonic (< 1235 km/h)", pkt.Position.Speed)
	}
}

func TestParseMessage(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name             string
		payload          string
		wantAddressee    string
		wantText         string
		wantMsgNo        string
		wantIsAck        bool
		wantIsRej        bool
		wantAckMsgNo     string
		wantIsAutoAnswer bool
		wantType         PacketType
	}{
		{
			name:          "regular message with number",
			payload:       ":N0CALL-5 :Hello World{123",
			wantAddressee: "N0CALL-5",
			wantText:      "Hello World",
			wantMsgNo:     "123",
			wantType:      PacketTypeMessage,
		},
		{
			name:          "regular message no number",
			payload:       ":N0CALL-5 :Hello World",
			wantAddressee: "N0CALL-5",
			wantText:      "Hello World",
			wantType:      PacketTypeMessage,
		},
		{
			name:          "ack message",
			payload:       ":N0CALL-5 :ack123",
			wantAddressee: "N0CALL-5",
			wantIsAck:     true,
			wantAckMsgNo:  "123",
			wantType:      PacketTypeMessage,
		},
		{
			name:          "rej message",
			payload:       ":N0CALL-5 :rej456",
			wantAddressee: "N0CALL-5",
			wantIsRej:     true,
			wantAckMsgNo:  "456",
			wantType:      PacketTypeMessage,
		},
		{
			name:          "bulletin",
			payload:       ":BLN3     :Weather bulletin text",
			wantAddressee: "BLN3",
			wantText:      "Weather bulletin text",
			wantType:      PacketTypeMessage,
		},
		{
			name:             "auto-answer message",
			payload:          ":N0CALL-5 :AA:I am away from the radio{456",
			wantAddressee:    "N0CALL-5",
			wantText:         "AA:I am away from the radio",
			wantMsgNo:        "456",
			wantIsAutoAnswer: true,
			wantType:         PacketTypeMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "W3ADO", SSID: 1},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Message == nil {
				t.Fatal("message is nil")
			}
			if pkt.Message.Addressee != tt.wantAddressee {
				t.Errorf("addressee = %q, want %q", pkt.Message.Addressee, tt.wantAddressee)
			}
			if tt.wantText != "" && pkt.Message.Text != tt.wantText {
				t.Errorf("text = %q, want %q", pkt.Message.Text, tt.wantText)
			}
			if tt.wantMsgNo != "" && pkt.Message.MessageNo != tt.wantMsgNo {
				t.Errorf("msgno = %q, want %q", pkt.Message.MessageNo, tt.wantMsgNo)
			}
			if pkt.Message.IsAck != tt.wantIsAck {
				t.Errorf("isAck = %v, want %v", pkt.Message.IsAck, tt.wantIsAck)
			}
			if pkt.Message.IsRej != tt.wantIsRej {
				t.Errorf("isRej = %v, want %v", pkt.Message.IsRej, tt.wantIsRej)
			}
			if tt.wantAckMsgNo != "" && pkt.Message.AckMsgNo != tt.wantAckMsgNo {
				t.Errorf("ackMsgNo = %q, want %q", pkt.Message.AckMsgNo, tt.wantAckMsgNo)
			}
			if pkt.Message.IsAutoAnswer != tt.wantIsAutoAnswer {
				t.Errorf("isAutoAnswer = %v, want %v", pkt.Message.IsAutoAnswer, tt.wantIsAutoAnswer)
			}
		})
	}
}

func TestParseObject(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		payload  string
		wantName string
		wantLive bool
		wantLat  float64
		wantLon  float64
		wantType PacketType
	}{
		{
			name:     "live object",
			payload:  ";LEADER   *092345z4903.50N/07201.75W>",
			wantName: "LEADER",
			wantLive: true,
			wantLat:  49.05833,
			wantLon:  -72.02917,
			wantType: PacketTypeObject,
		},
		{
			name:     "killed object",
			payload:  ";LEADER   _092345z4903.50N/07201.75W>",
			wantName: "LEADER",
			wantLive: false,
			wantLat:  49.05833,
			wantLon:  -72.02917,
			wantType: PacketTypeObject,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Object == nil {
				t.Fatal("object is nil")
			}
			if pkt.Object.Name != tt.wantName {
				t.Errorf("name = %q, want %q", pkt.Object.Name, tt.wantName)
			}
			if pkt.Object.Live != tt.wantLive {
				t.Errorf("live = %v, want %v", pkt.Object.Live, tt.wantLive)
			}
			if !approxEqual(pkt.Object.Position.Lat, tt.wantLat, 0.001) {
				t.Errorf("lat = %f, want %f", pkt.Object.Position.Lat, tt.wantLat)
			}
			if !approxEqual(pkt.Object.Position.Lon, tt.wantLon, 0.001) {
				t.Errorf("lon = %f, want %f", pkt.Object.Position.Lon, tt.wantLon)
			}
		})
	}
}

func TestParseItem(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		payload  string
		wantName string
		wantLive bool
		wantLat  float64
		wantLon  float64
		wantType PacketType
	}{
		{
			name:     "live item",
			payload:  ")AID #2!4903.50N/07201.75W-",
			wantName: "AID #2",
			wantLive: true,
			wantLat:  49.05833,
			wantLon:  -72.02917,
			wantType: PacketTypeItem,
		},
		{
			name:     "killed item",
			payload:  ")AID #2_4903.50N/07201.75W-",
			wantName: "AID #2",
			wantLive: false,
			wantLat:  49.05833,
			wantLon:  -72.02917,
			wantType: PacketTypeItem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Item == nil {
				t.Fatal("item is nil")
			}
			if pkt.Item.Name != tt.wantName {
				t.Errorf("name = %q, want %q", pkt.Item.Name, tt.wantName)
			}
			if pkt.Item.Live != tt.wantLive {
				t.Errorf("live = %v, want %v", pkt.Item.Live, tt.wantLive)
			}
			if !approxEqual(pkt.Item.Position.Lat, tt.wantLat, 0.001) {
				t.Errorf("lat = %f, want %f", pkt.Item.Position.Lat, tt.wantLat)
			}
			if !approxEqual(pkt.Item.Position.Lon, tt.wantLon, 0.001) {
				t.Errorf("lon = %f, want %f", pkt.Item.Position.Lon, tt.wantLon)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name           string
		payload        string
		wantText       string
		wantMaidenhead string
		wantType       PacketType
	}{
		{
			name:     "simple status",
			payload:  ">Net Control Center",
			wantText: "Net Control Center",
			wantType: PacketTypeStatus,
		},
		{
			name:           "status with Maidenhead",
			payload:        ">IO91SX/G",
			wantMaidenhead: "IO91SX",
			wantText:       "G",
			wantType:       PacketTypeStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Status == nil {
				t.Fatal("status is nil")
			}
			if pkt.Status.Text != tt.wantText {
				t.Errorf("text = %q, want %q", pkt.Status.Text, tt.wantText)
			}
			if tt.wantMaidenhead != "" && pkt.Status.Maidenhead != tt.wantMaidenhead {
				t.Errorf("maidenhead = %q, want %q", pkt.Status.Maidenhead, tt.wantMaidenhead)
			}
		})
	}
}

func TestParseTelemetry(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name       string
		payload    string
		wantSeq    int
		wantAnalog [5]float64
		wantDigit  byte
		wantType   PacketType
	}{
		{
			name:       "standard telemetry",
			payload:    "T#005,199,000,255,073,123,01101001",
			wantSeq:    5,
			wantAnalog: [5]float64{199, 0, 255, 73, 123},
			wantDigit:  0b01101001,
			wantType:   PacketTypeTelemetry,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Telemetry == nil {
				t.Fatal("telemetry is nil")
			}
			if pkt.Telemetry.Seq != tt.wantSeq {
				t.Errorf("seq = %d, want %d", pkt.Telemetry.Seq, tt.wantSeq)
			}
			for i := 0; i < 5; i++ {
				if !approxEqual(pkt.Telemetry.Analog[i], tt.wantAnalog[i], 0.001) {
					t.Errorf("analog[%d] = %f, want %f", i, pkt.Telemetry.Analog[i], tt.wantAnalog[i])
				}
			}
			if pkt.Telemetry.Digital != tt.wantDigit {
				t.Errorf("digital = %08b, want %08b", pkt.Telemetry.Digital, tt.wantDigit)
			}
		})
	}
}

func TestParseWeather(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name          string
		payload       string
		wantWindDir   *float64
		wantWindSpd   *float64
		wantGust      *float64
		wantTemp      *float64
		wantHumid     *int
		wantPress     *float64
		wantRadiation *float64
		wantVoltage   *float64
		wantFlood     *float64
		wantType      PacketType
	}{
		{
			name:        "positionless weather",
			payload:     "_10090556c220s004g005t077r000p000P000h50b09900",
			wantWindDir: ptrFloat(220),
			wantWindSpd: ptrFloat(4 * 0.44704), // mph to m/s
			wantGust:    ptrFloat(5 * 0.44704),
			wantTemp:    ptrFloat((77 - 32) * 5.0 / 9.0), // F to C
			wantHumid:   ptrInt(50),
			wantPress:   ptrFloat(990.0),
			wantType:    PacketTypeWeather,
		},
		{
			name:        "position + weather",
			payload:     "@092345z4903.50N/07201.75W_090/000g005t077",
			wantWindDir: ptrFloat(90),
			wantWindSpd: ptrFloat(0),
			wantGust:    ptrFloat(5 * 0.44704),
			wantTemp:    ptrFloat((77 - 32) * 5.0 / 9.0),
			wantType:    PacketTypeWeather,
		},
		{
			name:          "positionless weather with radiation/voltage/flood",
			payload:       "_10090556c220s004g005t077X123V045F010",
			wantWindDir:   ptrFloat(220),
			wantWindSpd:   ptrFloat(4 * 0.44704),
			wantGust:      ptrFloat(5 * 0.44704),
			wantTemp:      ptrFloat((77 - 32) * 5.0 / 9.0),
			wantRadiation: ptrFloat(123),
			wantVoltage:   ptrFloat(45),
			wantFlood:     ptrFloat(10),
			wantType:      PacketTypeWeather,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Weather == nil {
				t.Fatal("weather is nil")
			}
			if tt.wantWindDir != nil {
				if pkt.Weather.WindDir == nil {
					t.Error("windDir is nil")
				} else if !approxEqual(*pkt.Weather.WindDir, *tt.wantWindDir, 0.1) {
					t.Errorf("windDir = %f, want %f", *pkt.Weather.WindDir, *tt.wantWindDir)
				}
			}
			if tt.wantWindSpd != nil {
				if pkt.Weather.WindSpeed == nil {
					t.Error("windSpeed is nil")
				} else if !approxEqual(*pkt.Weather.WindSpeed, *tt.wantWindSpd, 0.1) {
					t.Errorf("windSpeed = %f, want %f", *pkt.Weather.WindSpeed, *tt.wantWindSpd)
				}
			}
			if tt.wantGust != nil {
				if pkt.Weather.WindGust == nil {
					t.Error("windGust is nil")
				} else if !approxEqual(*pkt.Weather.WindGust, *tt.wantGust, 0.1) {
					t.Errorf("windGust = %f, want %f", *pkt.Weather.WindGust, *tt.wantGust)
				}
			}
			if tt.wantTemp != nil {
				if pkt.Weather.Temperature == nil {
					t.Error("temperature is nil")
				} else if !approxEqual(*pkt.Weather.Temperature, *tt.wantTemp, 0.1) {
					t.Errorf("temperature = %f, want %f", *pkt.Weather.Temperature, *tt.wantTemp)
				}
			}
			if tt.wantHumid != nil {
				if pkt.Weather.Humidity == nil {
					t.Error("humidity is nil")
				} else if *pkt.Weather.Humidity != *tt.wantHumid {
					t.Errorf("humidity = %d, want %d", *pkt.Weather.Humidity, *tt.wantHumid)
				}
			}
			if tt.wantPress != nil {
				if pkt.Weather.Pressure == nil {
					t.Error("pressure is nil")
				} else if !approxEqual(*pkt.Weather.Pressure, *tt.wantPress, 0.1) {
					t.Errorf("pressure = %f, want %f", *pkt.Weather.Pressure, *tt.wantPress)
				}
			}
			if tt.wantRadiation != nil {
				if pkt.Weather.Radiation == nil {
					t.Error("radiation is nil")
				} else if !approxEqual(*pkt.Weather.Radiation, *tt.wantRadiation, 0.1) {
					t.Errorf("radiation = %f, want %f", *pkt.Weather.Radiation, *tt.wantRadiation)
				}
			}
			if tt.wantVoltage != nil {
				if pkt.Weather.Voltage == nil {
					t.Error("voltage is nil")
				} else if !approxEqual(*pkt.Weather.Voltage, *tt.wantVoltage, 0.1) {
					t.Errorf("voltage = %f, want %f", *pkt.Weather.Voltage, *tt.wantVoltage)
				}
			}
			if tt.wantFlood != nil {
				if pkt.Weather.FloodLevel == nil {
					t.Error("floodLevel is nil")
				} else if !approxEqual(*pkt.Weather.FloodLevel, *tt.wantFlood, 0.1) {
					t.Errorf("floodLevel = %f, want %f", *pkt.Weather.FloodLevel, *tt.wantFlood)
				}
			}
		})
	}
}

func TestParseMicE(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		dest     string // Mic-E encodes lat in destination
		payload  string
		wantLat  float64
		wantLon  float64
		wantType PacketType
	}{
		{
			// Mic-E example:
			// Destination "S32UQT" encodes lat 33°25.64' N
			// Byte 4 'Q' (P-Y range) -> lonOffset=100
			// Byte 5 'T' (P-Y range) -> West
			// Info: (_fn"Oj/ -> lon deg=112, min=7, hundredths=74
			// Lon = -(112 + 7.74/60) = -112.129
			name:     "mic-e basic",
			dest:     "S32UQT",
			payload:  "`(_fn\"Oj/",
			wantLat:  33.427,
			wantLon:  -112.129,
			wantType: PacketTypeMicE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destAddr := parseAddressStr(tt.dest)
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: destAddr,
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.MicE == nil {
				t.Fatal("mic-e is nil")
			}
			if !approxEqual(pkt.MicE.Position.Lat, tt.wantLat, 0.5) {
				t.Errorf("lat = %f, want %f", pkt.MicE.Position.Lat, tt.wantLat)
			}
			if !approxEqual(pkt.MicE.Position.Lon, tt.wantLon, 0.5) {
				t.Errorf("lon = %f, want %f", pkt.MicE.Position.Lon, tt.wantLon)
			}
		})
	}
}

func TestParseThirdParty(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name         string
		payload      string
		wantInnerSrc string
		wantInnerType PacketType
		wantType     PacketType
	}{
		{
			name:         "third-party forwarded position",
			payload:      "}W3ADO-1>APRS,TCPIP:!4903.50N/07201.75W-",
			wantInnerSrc: "W3ADO-1",
			wantInnerType: PacketTypePosition,
			wantType:     PacketTypeThirdParty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "RELAY"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.ThirdParty == nil {
				t.Fatal("third-party is nil")
			}
			if pkt.ThirdParty.Frame.Source.String() != tt.wantInnerSrc {
				t.Errorf("inner source = %q, want %q", pkt.ThirdParty.Frame.Source.String(), tt.wantInnerSrc)
			}
			if pkt.ThirdParty.Type != tt.wantInnerType {
				t.Errorf("inner type = %d, want %d", pkt.ThirdParty.Type, tt.wantInnerType)
			}
		})
	}
}

func TestParseQuery(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name      string
		payload   string
		wantQuery string
		wantType  PacketType
	}{
		{
			name:      "APRS query",
			payload:   "?APRS?",
			wantQuery: "APRS",
			wantType:  PacketTypeQuery,
		},
		{
			name:      "WX query",
			payload:   "?WX?",
			wantQuery: "WX",
			wantType:  PacketTypeQuery,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if pkt.Query != tt.wantQuery {
				t.Errorf("query = %q, want %q", pkt.Query, tt.wantQuery)
			}
		})
	}
}

// --- APRS 1.2 Extension Tests ---

func TestParseDAO(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name      string
		payload   string
		wantLat   float64
		wantLon   float64
		wantDatum string
		wantPrec  int
		wantComment string
	}{
		{
			name: "DAO with uppercase W (human-readable extra digit)",
			// Base position: 49°05.50'N / 072°01.75'W
			// !W34! adds .003 to lat minutes and .004 to lon minutes
			// Lat: 49 + (5.503)/60 = 49.091717
			// Lon: -(72 + (1.754)/60) = -72.029233
			payload:   "!4905.50N/07201.75W-!W34!",
			wantLat:   49.091717,
			wantLon:   -72.029233,
			wantDatum: "W",
			wantPrec:  1,
		},
		{
			name: "DAO with lowercase w (base91 extra precision)",
			// !w&(! where & = ASCII 38, ( = ASCII 40
			// lat extra = (38-33)/91.0 * 100 = 5.49... hundredths
			// lon extra = (40-33)/91.0 * 100 = 7.69... hundredths
			// Base: 49°05.50'N / 072°01.75'W
			// Lat: 49 + (5.5 + 5/9100)/60  (very small adjustment)
			// Lon: -(72 + (1.75 + 7/9100)/60)
			payload:   "!4905.50N/07201.75W-!w&(!",
			wantLat:   49.0917, // very close to base
			wantLon:   -72.0292,
			wantDatum: "w",
			wantPrec:  2,
		},
		{
			name:        "DAO stripped from comment",
			payload:     "!4905.50N/07201.75W-Hello !W34! World",
			wantLat:     49.091717,
			wantLon:     -72.029233,
			wantDatum:   "W",
			wantPrec:    1,
			wantComment: "Hello  World",
		},
		{
			name:      "no DAO present",
			payload:   "!4903.50N/07201.75W-just a comment",
			wantLat:   49.05833,
			wantLon:   -72.02917,
			wantDatum: "",
			wantPrec:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Position == nil {
				t.Fatal("position is nil")
			}
			if !approxEqual(pkt.Position.Lat, tt.wantLat, 0.001) {
				t.Errorf("lat = %f, want %f", pkt.Position.Lat, tt.wantLat)
			}
			if !approxEqual(pkt.Position.Lon, tt.wantLon, 0.001) {
				t.Errorf("lon = %f, want %f", pkt.Position.Lon, tt.wantLon)
			}
			if pkt.Position.Datum != tt.wantDatum {
				t.Errorf("datum = %q, want %q", pkt.Position.Datum, tt.wantDatum)
			}
			if pkt.Position.Precision != tt.wantPrec {
				t.Errorf("precision = %d, want %d", pkt.Position.Precision, tt.wantPrec)
			}
			if tt.wantComment != "" && pkt.Position.Comment != tt.wantComment {
				t.Errorf("comment = %q, want %q", pkt.Position.Comment, tt.wantComment)
			}
		})
	}
}

func TestParseFrequency(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name       string
		payload    string
		wantFreq   float64
		wantTone   float64
		wantDCS    int
		wantOffset float64
		wantRange  float64
		wantType   PacketType
	}{
		{
			name:     "frequency in comment FFF.FFF MHz",
			payload:  "!4903.50N/07201.75W-146.520 MHz",
			wantFreq: 146.520,
			wantType: PacketTypePosition,
		},
		{
			name:     "frequency with tone",
			payload:  "!4903.50N/07201.75W-146.820 MHz T107",
			wantFreq: 146.820,
			wantTone: 107.0,
			wantType: PacketTypePosition,
		},
		{
			name:       "frequency with offset",
			payload:    "!4903.50N/07201.75W-146.820 MHz T107 +060",
			wantFreq:   146.820,
			wantTone:   107.0,
			wantOffset: 0.600, // +060 = +600kHz = +0.6 MHz
			wantType:   PacketTypePosition,
		},
		{
			name:     "frequency with DCS code",
			payload:  "!4903.50N/07201.75W-146.820 MHz D023",
			wantFreq: 146.820,
			wantDCS:  23,
			wantType: PacketTypePosition,
		},
		{
			name:      "frequency with range",
			payload:   "!4903.50N/07201.75W-146.820 MHz R25m",
			wantFreq:  146.820,
			wantRange: 25 * 1.60934, // 25 miles in km
			wantType:  PacketTypePosition,
		},
		{
			name:      "frequency with range in km",
			payload:   "!4903.50N/07201.75W-146.820 MHz R40k",
			wantFreq:  146.820,
			wantRange: 40.0,
			wantType:  PacketTypePosition,
		},
		{
			name:     "compact frequency FFF.FFFMHz",
			payload:  "!4903.50N/07201.75W-147.105MHz AARC Club",
			wantFreq: 147.105,
			wantType: PacketTypePosition,
		},
		{
			name:     "no frequency in comment",
			payload:  "!4903.50N/07201.75W-just a comment",
			wantFreq: 0,
			wantType: PacketTypePosition,
		},
		{
			name:     "microwave A prefix (23cm band)",
			payload:  "!4903.50N/07201.75W-A96.000 MHz",
			wantFreq: 1296.0,
			wantType: PacketTypePosition,
		},
		{
			name:     "microwave B prefix (13cm band)",
			payload:  "!4903.50N/07201.75W-B20.000 MHz",
			wantFreq: 2320.0,
			wantType: PacketTypePosition,
		},
		{
			name:     "microwave C prefix (9cm band)",
			payload:  "!4903.50N/07201.75W-C56.000 MHz",
			wantFreq: 3456.0,
			wantType: PacketTypePosition,
		},
		{
			name:     "microwave D prefix (6cm band)",
			payload:  "!4903.50N/07201.75W-D60.000 MHz",
			wantFreq: 5660.0,
			wantType: PacketTypePosition,
		},
		{
			name:     "microwave E prefix (3cm band)",
			payload:  "!4903.50N/07201.75W-E50.000 MHz",
			wantFreq: 10050.0,
			wantType: PacketTypePosition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.Type != tt.wantType {
				t.Errorf("type = %d, want %d", pkt.Type, tt.wantType)
			}
			if tt.wantFreq == 0 {
				if pkt.Frequency != nil {
					t.Errorf("expected nil frequency, got %+v", pkt.Frequency)
				}
				return
			}
			if pkt.Frequency == nil {
				t.Fatal("frequency is nil")
			}
			if !approxEqual(pkt.Frequency.Freq, tt.wantFreq, 0.001) {
				t.Errorf("freq = %f, want %f", pkt.Frequency.Freq, tt.wantFreq)
			}
			if tt.wantTone > 0 && !approxEqual(pkt.Frequency.Tone, tt.wantTone, 0.1) {
				t.Errorf("tone = %f, want %f", pkt.Frequency.Tone, tt.wantTone)
			}
			if tt.wantDCS > 0 && pkt.Frequency.DCS != tt.wantDCS {
				t.Errorf("dcs = %d, want %d", pkt.Frequency.DCS, tt.wantDCS)
			}
			if tt.wantOffset != 0 && !approxEqual(pkt.Frequency.Offset, tt.wantOffset, 0.001) {
				t.Errorf("offset = %f, want %f", pkt.Frequency.Offset, tt.wantOffset)
			}
			if tt.wantRange > 0 && !approxEqual(pkt.Frequency.Range, tt.wantRange, 0.1) {
				t.Errorf("range = %f, want %f", pkt.Frequency.Range, tt.wantRange)
			}
		})
	}
}

func TestMicERadioTypes(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name      string
		dest      string
		payload   string
		wantRadio string
	}{
		{
			name:      "Kenwood TH-D74",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/>^",
			wantRadio: "Kenwood TH-D74",
		},
		{
			name:      "Yaesu VX-8",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_b",
			wantRadio: "Yaesu VX-8",
		},
		{
			name:      "Yaesu FTM-350",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_\"",
			wantRadio: "Yaesu FTM-350",
		},
		{
			name:      "Yaesu FT1D",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_$",
			wantRadio: "Yaesu FT1D",
		},
		{
			name:      "Yaesu FTM-400DR",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_%",
			wantRadio: "Yaesu FTM-400DR",
		},
		{
			name:      "Yaesu FT2D",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_(",
			wantRadio: "Yaesu FT2D",
		},
		{
			name:      "Yaesu FT3D",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_0",
			wantRadio: "Yaesu FT3D",
		},
		{
			name:      "Yaesu FT5D",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_3",
			wantRadio: "Yaesu FT5D",
		},
		{
			name:      "Yaesu FTM-300D",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_1",
			wantRadio: "Yaesu FTM-300D",
		},
		{
			name:      "Yaesu FTM-100D",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_)",
			wantRadio: "Yaesu FTM-100D",
		},
		{
			name:      "Yaesu VX-8G",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`_#",
			wantRadio: "Yaesu VX-8G",
		},
		{
			name:      "Anytone D578UV",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/`(5",
			wantRadio: "Anytone D578UV",
		},
		{
			name:      "Kenwood TM-D710",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/]=",
			wantRadio: "Kenwood TM-D710",
		},
		{
			name:      "Kenwood TM-D700",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/]",
			wantRadio: "Kenwood TM-D700",
		},
		{
			name:      "Kenwood TH-D7",
			dest:      "S32UQT",
			payload:   "`(_fn\"Oj/>",
			wantRadio: "Kenwood TH-D7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destAddr := parseAddressStr(tt.dest)
			frame := APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: destAddr,
				Payload:     tt.payload,
			}
			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkt.MicE == nil {
				t.Fatal("mic-e is nil")
			}
			if pkt.MicE.RadioModel != tt.wantRadio {
				t.Errorf("radio = %q, want %q", pkt.MicE.RadioModel, tt.wantRadio)
			}
		})
	}
}

func TestParseUnknownDTI(t *testing.T) {
	parser := NewParser()
	frame := APRSFrame{
		Source:      Address{Call: "N0CALL"},
		Destination: Address{Call: "APRS"},
		Payload:     "Xsome unknown data",
	}
	pkt, err := parser.Parse(frame)
	if err != nil {
		t.Fatalf("unexpected error for unknown DTI: %v", err)
	}
	if pkt.Type != PacketTypeUnknown {
		t.Errorf("type = %d, want %d (unknown)", pkt.Type, PacketTypeUnknown)
	}
}

func TestParseEmptyPayload(t *testing.T) {
	parser := NewParser()
	frame := APRSFrame{
		Source:      Address{Call: "N0CALL"},
		Destination: Address{Call: "APRS"},
		Payload:     "",
	}
	_, err := parser.Parse(frame)
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}

// Helper to parse "CALL" or "CALL-N" to Address
func parseAddressStr(s string) Address {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '-' {
			ssid := 0
			for _, c := range s[i+1:] {
				ssid = ssid*10 + int(c-'0')
			}
			return Address{Call: s[:i], SSID: ssid}
		}
	}
	return Address{Call: s}
}

func ptrFloat(f float64) *float64 { return &f }
func ptrInt(i int) *int           { return &i }
