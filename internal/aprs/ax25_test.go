package aprs

import (
	"bytes"
	"testing"
)

func TestEncodeAX25Address(t *testing.T) {
	tests := []struct {
		name string
		addr Address
		last bool // is this the last address?
		want []byte
	}{
		{
			name: "APRS no SSID not last",
			addr: Address{Call: "APRS"},
			last: false,
			// A=0x41<<1=0x82, P=0x50<<1=0xA0, R=0x52<<1=0xA4, S=0x53<<1=0xA6
			// pad with space<<1=0x40 for 2 more
			// SSID byte: reserved bits 6,7 set (0x60), SSID=0, not last = 0x60
			want: []byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60},
		},
		{
			name: "N0CALL no SSID last",
			addr: Address{Call: "N0CALL"},
			last: true,
			// N=0x4E<<1=0x9C, 0=0x30<<1=0x60, C=0x43<<1=0x86, A=0x41<<1=0x82,
			// L=0x4C<<1=0x98... wait, let me recalculate
			// N=0x4E -> 0x9C, 0=0x30 -> 0x60, C=0x43 -> 0x86,
			// A=0x41 -> 0x82, L=0x4C -> 0x98, L=0x4C -> 0x98
			// Wait: the task description says source N0CALL = 9C 60 86 82 9A 9A
			// L=0x4C -> 0x98? But task says 0x9A. Let me check: 0x4C << 1 = 0x98, but
			// 0x4D << 1 = 0x9A. Hmm, L is 0x4C, so 0x4C<<1 = 0x98.
			// Actually 0x4C = 76, 76*2 = 152 = 0x98. But the task says 9A.
			// Let me re-check: N0CALL. N=0x4E, 0=0x30, C=0x43, A=0x41, L=0x4C, L=0x4C
			// Oh wait - the test vectors in the task description might have a typo.
			// Let me compute from scratch:
			// N: 0x4E << 1 = 0x9C ✓
			// 0: 0x30 << 1 = 0x60 ✓
			// C: 0x43 << 1 = 0x86 ✓
			// A: 0x41 << 1 = 0x82 ✓
			// L: 0x4C << 1 = 0x98
			// L: 0x4C << 1 = 0x98
			// SSID byte (last, SSID=0): 0x60 | 0x01 = 0x61
			want: []byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61},
		},
		{
			name: "N0CALL SSID 5 not last",
			addr: Address{Call: "N0CALL", SSID: 5},
			last: false,
			// Same callsign bytes
			// SSID byte: 0x60 | (5 << 1) = 0x60 | 0x0A = 0x6A
			want: []byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x6A},
		},
		{
			name: "short callsign A last",
			addr: Address{Call: "A"},
			last: true,
			// A=0x41<<1=0x82, then 5 spaces = 0x40 each
			// SSID byte: 0x60 | 0x01 = 0x61
			want: []byte{0x82, 0x40, 0x40, 0x40, 0x40, 0x40, 0x61},
		},
		{
			name: "SSID 15 last",
			addr: Address{Call: "N0CALL", SSID: 15},
			last: true,
			// SSID byte: 0x60 | (15 << 1) | 0x01 = 0x60 | 0x1E | 0x01 = 0x7F
			want: []byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x7F},
		},
		{
			name: "RELAY with H-bit not last",
			addr: Address{Call: "RELAY", HBit: true},
			last: false,
			// R=0x52<<1=0xA4, E=0x45<<1=0x8A, L=0x4C<<1=0x98,
			// A=0x41<<1=0x82, Y=0x59<<1=0xB2, space=0x20<<1=0x40
			// SSID byte: 0x60 | H-bit(0x80) = 0xE0
			want: []byte{0xA4, 0x8A, 0x98, 0x82, 0xB2, 0x40, 0xE0},
		},
		{
			name: "WIDE no H-bit last",
			addr: Address{Call: "WIDE"},
			last: true,
			// W=0x57<<1=0xAE, I=0x49<<1=0x92, D=0x44<<1=0x88,
			// E=0x45<<1=0x8A, space=0x40, space=0x40
			// SSID byte: 0x60 | 0x01 = 0x61
			want: []byte{0xAE, 0x92, 0x88, 0x8A, 0x40, 0x40, 0x61},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeAX25Address(tt.addr, tt.last)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("encodeAX25Address(%v, %v) = %x, want %x", tt.addr, tt.last, got, tt.want)
			}
		})
	}
}

func TestDecodeAX25Address(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantCall string
		wantSSID int
		wantHBit bool
		wantLast bool
	}{
		{
			name:     "APRS no SSID not last",
			data:     []byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60},
			wantCall: "APRS",
			wantSSID: 0,
			wantLast: false,
		},
		{
			name:     "N0CALL no SSID last",
			data:     []byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61},
			wantCall: "N0CALL",
			wantSSID: 0,
			wantLast: true,
		},
		{
			name:     "N0CALL SSID 5 not last",
			data:     []byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x6A},
			wantCall: "N0CALL",
			wantSSID: 5,
			wantLast: false,
		},
		{
			name:     "RELAY with H-bit not last",
			data:     []byte{0xA4, 0x8A, 0x98, 0x82, 0xB2, 0x40, 0xE0},
			wantCall: "RELAY",
			wantHBit: true,
			wantLast: false,
		},
		{
			name:     "WIDE last",
			data:     []byte{0xAE, 0x92, 0x88, 0x8A, 0x40, 0x40, 0x61},
			wantCall: "WIDE",
			wantSSID: 0,
			wantLast: true,
		},
		{
			name:     "SSID 15",
			data:     []byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x7F},
			wantCall: "N0CALL",
			wantSSID: 15,
			wantLast: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, last := decodeAX25Address(tt.data)
			if addr.Call != tt.wantCall {
				t.Errorf("call = %q, want %q", addr.Call, tt.wantCall)
			}
			if addr.SSID != tt.wantSSID {
				t.Errorf("ssid = %d, want %d", addr.SSID, tt.wantSSID)
			}
			if addr.HBit != tt.wantHBit {
				t.Errorf("hbit = %v, want %v", addr.HBit, tt.wantHBit)
			}
			if last != tt.wantLast {
				t.Errorf("last = %v, want %v", last, tt.wantLast)
			}
		})
	}
}

func TestEncodeAX25(t *testing.T) {
	tests := []struct {
		name    string
		frame   APRSFrame
		want    []byte
		wantErr bool
	}{
		{
			name: "simple no path",
			frame: APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     "!4903.50N/07201.75W-",
			},
			want: concat(
				[]byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60}, // APRS (not last)
				[]byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61}, // N0CALL (last)
				[]byte{0x03, 0xF0},                                 // Control, PID
				[]byte("!4903.50N/07201.75W-"),                     // Info
			),
		},
		{
			name: "with digipeater path",
			frame: APRSFrame{
				Source:      Address{Call: "N0CALL", SSID: 5},
				Destination: Address{Call: "APRS"},
				Path: []Address{
					{Call: "RELAY", HBit: true},
					{Call: "WIDE"},
				},
				Payload: "!4903.50N/07201.75W-",
			},
			want: concat(
				[]byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60}, // APRS (not last)
				[]byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x6A}, // N0CALL-5 (not last)
				[]byte{0xA4, 0x8A, 0x98, 0x82, 0xB2, 0x40, 0xE0}, // RELAY* (H-bit, not last)
				[]byte{0xAE, 0x92, 0x88, 0x8A, 0x40, 0x40, 0x61}, // WIDE (last)
				[]byte{0x03, 0xF0},                                 // Control, PID
				[]byte("!4903.50N/07201.75W-"),                     // Info
			),
		},
		{
			name: "callsign too long",
			frame: APRSFrame{
				Source:      Address{Call: "TOOLONG7"},
				Destination: Address{Call: "APRS"},
				Payload:     "test",
			},
			wantErr: true,
		},
		{
			name: "SSID out of range",
			frame: APRSFrame{
				Source:      Address{Call: "N0CALL", SSID: 16},
				Destination: Address{Call: "APRS"},
				Payload:     "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeAX25(tt.frame)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("EncodeAX25() =\n  %x\nwant:\n  %x", got, tt.want)
			}
		})
	}
}

func TestDecodeAX25(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantSrc string
		wantDst string
		wantPath []string
		wantBody string
		wantErr  bool
	}{
		{
			name: "simple no path",
			data: concat(
				[]byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60}, // APRS
				[]byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61}, // N0CALL (last)
				[]byte{0x03, 0xF0},
				[]byte("!4903.50N/07201.75W-"),
			),
			wantSrc:  "N0CALL",
			wantDst:  "APRS",
			wantBody: "!4903.50N/07201.75W-",
		},
		{
			name: "with digipeater path",
			data: concat(
				[]byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60}, // APRS
				[]byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x6A}, // N0CALL-5
				[]byte{0xA4, 0x8A, 0x98, 0x82, 0xB2, 0x40, 0xE0}, // RELAY* (H-bit)
				[]byte{0xAE, 0x92, 0x88, 0x8A, 0x40, 0x40, 0x61}, // WIDE (last)
				[]byte{0x03, 0xF0},
				[]byte("!4903.50N/07201.75W-"),
			),
			wantSrc:  "N0CALL-5",
			wantDst:  "APRS",
			wantPath: []string{"RELAY*", "WIDE"},
			wantBody: "!4903.50N/07201.75W-",
		},
		{
			name:    "too short for addresses",
			data:    []byte{0x82, 0xA0, 0xA4},
			wantErr: true,
		},
		{
			name: "missing control/PID",
			data: concat(
				[]byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60},
				[]byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61},
			),
			wantErr: true,
		},
		{
			name: "wrong control byte",
			data: concat(
				[]byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60},
				[]byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61},
				[]byte{0x13, 0xF0}, // wrong control
			),
			wantErr: true,
		},
		{
			name: "wrong PID byte",
			data: concat(
				[]byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60},
				[]byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61},
				[]byte{0x03, 0xE0}, // wrong PID
			),
			wantErr: true,
		},
		{
			name: "empty payload",
			data: concat(
				[]byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60},
				[]byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61},
				[]byte{0x03, 0xF0},
			),
			wantSrc:  "N0CALL",
			wantDst:  "APRS",
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := DecodeAX25(tt.data)
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
			} else if len(frame.Path) != 0 {
				t.Errorf("path len = %d, want 0", len(frame.Path))
			}
			if frame.Payload != tt.wantBody {
				t.Errorf("payload = %q, want %q", frame.Payload, tt.wantBody)
			}
		})
	}
}

func TestAX25Roundtrip(t *testing.T) {
	tests := []struct {
		name  string
		frame APRSFrame
	}{
		{
			name: "simple no path",
			frame: APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Payload:     "!4903.50N/07201.75W-",
			},
		},
		{
			name: "with SSID",
			frame: APRSFrame{
				Source:      Address{Call: "W3ADO", SSID: 1},
				Destination: Address{Call: "APRS"},
				Payload:     ":N0CALL-5 :Hello{123",
			},
		},
		{
			name: "with path and H-bit",
			frame: APRSFrame{
				Source:      Address{Call: "N0CALL", SSID: 5},
				Destination: Address{Call: "APRS"},
				Path: []Address{
					{Call: "RELAY", HBit: true},
					{Call: "WIDE"},
				},
				Payload: "!4903.50N/07201.75W-",
			},
		},
		{
			name: "max SSID",
			frame: APRSFrame{
				Source:      Address{Call: "AB1CDE", SSID: 15},
				Destination: Address{Call: "APRS", SSID: 0},
				Payload:     ">status",
			},
		},
		{
			name: "single char callsign",
			frame: APRSFrame{
				Source:      Address{Call: "A"},
				Destination: Address{Call: "B"},
				Payload:     "test",
			},
		},
		{
			name: "multiple digipeaters some repeated",
			frame: APRSFrame{
				Source:      Address{Call: "N0CALL"},
				Destination: Address{Call: "APRS"},
				Path: []Address{
					{Call: "WIDE1", SSID: 1, HBit: true},
					{Call: "WIDE2", SSID: 1, HBit: true},
					{Call: "WIDE2", SSID: 2},
				},
				Payload: "!4903.50N/07201.75W-",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeAX25(tt.frame)
			if err != nil {
				t.Fatalf("EncodeAX25 error: %v", err)
			}
			decoded, err := DecodeAX25(encoded)
			if err != nil {
				t.Fatalf("DecodeAX25 error: %v", err)
			}
			// Compare fields
			if decoded.Source.Call != tt.frame.Source.Call {
				t.Errorf("source call = %q, want %q", decoded.Source.Call, tt.frame.Source.Call)
			}
			if decoded.Source.SSID != tt.frame.Source.SSID {
				t.Errorf("source ssid = %d, want %d", decoded.Source.SSID, tt.frame.Source.SSID)
			}
			if decoded.Destination.Call != tt.frame.Destination.Call {
				t.Errorf("dest call = %q, want %q", decoded.Destination.Call, tt.frame.Destination.Call)
			}
			if decoded.Destination.SSID != tt.frame.Destination.SSID {
				t.Errorf("dest ssid = %d, want %d", decoded.Destination.SSID, tt.frame.Destination.SSID)
			}
			if len(decoded.Path) != len(tt.frame.Path) {
				t.Fatalf("path len = %d, want %d", len(decoded.Path), len(tt.frame.Path))
			}
			for i := range tt.frame.Path {
				if decoded.Path[i].Call != tt.frame.Path[i].Call {
					t.Errorf("path[%d] call = %q, want %q", i, decoded.Path[i].Call, tt.frame.Path[i].Call)
				}
				if decoded.Path[i].SSID != tt.frame.Path[i].SSID {
					t.Errorf("path[%d] ssid = %d, want %d", i, decoded.Path[i].SSID, tt.frame.Path[i].SSID)
				}
				if decoded.Path[i].HBit != tt.frame.Path[i].HBit {
					t.Errorf("path[%d] hbit = %v, want %v", i, decoded.Path[i].HBit, tt.frame.Path[i].HBit)
				}
			}
			if decoded.Payload != tt.frame.Payload {
				t.Errorf("payload = %q, want %q", decoded.Payload, tt.frame.Payload)
			}
		})
	}
}

func TestDecodeAX25MaxDigipeaters(t *testing.T) {
	// Build a frame with 8 digipeaters (maximum)
	frame := APRSFrame{
		Source:      Address{Call: "N0CALL"},
		Destination: Address{Call: "APRS"},
		Payload:     "test",
	}
	for i := 0; i < 8; i++ {
		call := "DIGI" + string(rune('A'+i))
		frame.Path = append(frame.Path, Address{Call: call})
	}
	encoded, err := EncodeAX25(frame)
	if err != nil {
		t.Fatalf("EncodeAX25 error: %v", err)
	}
	decoded, err := DecodeAX25(encoded)
	if err != nil {
		t.Fatalf("DecodeAX25 error: %v", err)
	}
	if len(decoded.Path) != 8 {
		t.Errorf("path len = %d, want 8", len(decoded.Path))
	}
}

func TestEncodeAX25TooManyDigipeaters(t *testing.T) {
	frame := APRSFrame{
		Source:      Address{Call: "N0CALL"},
		Destination: Address{Call: "APRS"},
		Payload:     "test",
	}
	for i := 0; i < 9; i++ {
		frame.Path = append(frame.Path, Address{Call: "DIGI"})
	}
	_, err := EncodeAX25(frame)
	if err == nil {
		t.Fatal("expected error for 9 digipeaters")
	}
}

// concat joins multiple byte slices.
func concat(slices ...[]byte) []byte {
	var result []byte
	for _, s := range slices {
		result = append(result, s...)
	}
	return result
}
