package aprs

import (
	"strings"
	"testing"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []Address
		wantErr bool
	}{
		{
			name: "empty is direct",
			in:   "",
			want: []Address{},
		},
		{
			name: "whitespace is direct",
			in:   "  ",
			want: []Address{},
		},
		{
			name: "standard RF path",
			in:   "WIDE1-1,WIDE2-1",
			want: []Address{{Call: "WIDE1", SSID: 1}, {Call: "WIDE2", SSID: 1}},
		},
		{
			name: "spaces after commas",
			in:   "WIDE1-1, WIDE2-1",
			want: []Address{{Call: "WIDE1", SSID: 1}, {Call: "WIDE2", SSID: 1}},
		},
		{
			name: "TCPIP star sets H-bit",
			in:   "TCPIP*",
			want: []Address{{Call: "TCPIP", HBit: true}},
		},
		{
			name: "single hop",
			in:   "WIDE1-1",
			want: []Address{{Call: "WIDE1", SSID: 1}},
		},
		{
			name: "used hop in path",
			in:   "WIDE1-1*,WIDE2-1",
			want: []Address{{Call: "WIDE1", SSID: 1, HBit: true}, {Call: "WIDE2", SSID: 1}},
		},
		{
			name:    "callsign too long",
			in:      "TOOLONG-1",
			wantErr: true,
		},
		{
			name:    "invalid SSID",
			in:      "WIDE1-99",
			wantErr: true,
		},
		{
			name:    "too many hops",
			in:      "A-1,B-1,C-1,D-1,E-1,F-1,G-1,H-1,I-1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePath(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePath(%q) error = nil, want error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePath(%q): %v", tt.in, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("path[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFormatPath(t *testing.T) {
	if got := FormatPath(nil); got != "" {
		t.Errorf("FormatPath(nil) = %q, want empty", got)
	}
	got := FormatPath([]Address{{Call: "WIDE1", SSID: 1}, {Call: "WIDE2", SSID: 1}})
	if got != "WIDE1-1,WIDE2-1" {
		t.Errorf("FormatPath = %q, want WIDE1-1,WIDE2-1", got)
	}
	got = FormatPath([]Address{{Call: "TCPIP", HBit: true}})
	if got != "TCPIP*" {
		t.Errorf("FormatPath TCPIP = %q, want TCPIP*", got)
	}
}

func TestParsePathTCPIPEncodesAsAX25(t *testing.T) {
	path, err := ParsePath("TCPIP*")
	if err != nil {
		t.Fatal(err)
	}
	frame := APRSFrame{
		Source:      Address{Call: "N0CALL"},
		Destination: Address{Call: "APRS"},
		Path:        path,
		Payload:     ":W3ADO-5  :hi{1",
	}
	data, err := EncodeAX25(frame)
	if err != nil {
		t.Fatalf("EncodeAX25: %v", err)
	}
	decoded, err := DecodeAX25(data)
	if err != nil {
		t.Fatalf("DecodeAX25: %v", err)
	}
	if len(decoded.Path) != 1 {
		t.Fatalf("path len = %d, want 1", len(decoded.Path))
	}
	if decoded.Path[0].Call != "TCPIP" || !decoded.Path[0].HBit {
		t.Errorf("path[0] = %+v, want TCPIP with HBit", decoded.Path[0])
	}
}

func TestDefaultRFPath(t *testing.T) {
	p := DefaultRFPath()
	if FormatPath(p) != "WIDE1-1,WIDE2-1" {
		t.Errorf("DefaultRFPath = %q", FormatPath(p))
	}
	p[0].Call = "MUTATED"
	if DefaultRFPath()[0].Call != "WIDE1" {
		t.Error("DefaultRFPath should return a fresh slice")
	}
}

func TestParsePathUppercases(t *testing.T) {
	path, err := ParsePath("wide1-1, wide2-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatPath(path); got != "WIDE1-1,WIDE2-1" {
		t.Errorf("got %q, want WIDE1-1,WIDE2-1", got)
	}
}

func TestParsePathRejectsDeprecated(t *testing.T) {
	tests := []string{
		"RELAY",
		"RELAY,WIDE",
		"WIDE",
		"TRACE",
		"TRACE2-2",
		"WIDE3-1",
		"WIDE7-7",
		"WIDE1-0",
		"WIDE2-0",
	}
	for _, in := range tests {
		if _, err := ParsePath(in); err == nil {
			t.Errorf("ParsePath(%q) succeeded, want New-N rejection", in)
		}
	}
}

func TestParsePathAllowsNewNAndSpecials(t *testing.T) {
	tests := []string{
		"WIDE1-1",
		"WIDE2-1",
		"WIDE2-2",
		"WIDE1-1,WIDE2-1",
		"WIDE1-1,WIDE2-2",
		"TCPIP*",
		"RFONLY",
		"NOGATE",
		"WIDE1-1,WIDE2-1,NOGATE",
		"N0CALL-7,WIDE2-1",
	}
	for _, in := range tests {
		if _, err := ParsePath(in); err != nil {
			t.Errorf("ParsePath(%q): %v", in, err)
		}
	}
}

func TestParsePathRoundTrip(t *testing.T) {
	in := "WIDE1-1, WIDE2-1"
	path, err := ParsePath(in)
	if err != nil {
		t.Fatal(err)
	}
	got := FormatPath(path)
	if got != "WIDE1-1,WIDE2-1" {
		t.Errorf("round trip = %q", got)
	}
	if !strings.Contains(in, " ") {
		t.Fatal("test setup: input should contain a space")
	}
}
