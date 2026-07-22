package app

import (
	"testing"

	"github.com/narvel/nymeria/internal/transport"
)

func TestBuildTransportMap(t *testing.T) {
	tests := []struct {
		name    string
		configs []transport.TransportConfig
		want    map[string]transport.TransportConfig
	}{
		{
			name:    "empty slice",
			configs: nil,
			want:    map[string]transport.TransportConfig{},
		},
		{
			name: "two aprsis and one kisstcp",
			configs: []transport.TransportConfig{
				{Type: "aprsis", Host: "a.example.com", Port: 14580},
				{Type: "aprsis", Host: "b.example.com", Port: 14580},
				{Type: "kisstcp", Host: "localhost", Port: 8001},
			},
			want: map[string]transport.TransportConfig{
				"aprsis-0":  {Type: "aprsis", Host: "a.example.com", Port: 14580},
				"aprsis-1":  {Type: "aprsis", Host: "b.example.com", Port: 14580},
				"kisstcp-0": {Type: "kisstcp", Host: "localhost", Port: 8001},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTransportMap(tt.configs)
			if len(got) != len(tt.want) {
				t.Fatalf("len(got)=%d, want %d; got=%v", len(got), len(tt.want), got)
			}
			for id, wantTC := range tt.want {
				gotTC, ok := got[id]
				if !ok {
					t.Errorf("missing key %q", id)
					continue
				}
				if !transportConfigEqual(gotTC, wantTC) {
					t.Errorf("key %q: got %+v, want %+v", id, gotTC, wantTC)
				}
			}
		})
	}
}

func TestTransportConfigEqual(t *testing.T) {
	base := transport.TransportConfig{
		Type:     "aprsis",
		Name:     "main",
		Host:     "rotate.aprs2.net",
		Port:     14580,
		Device:   "/dev/ttyUSB0",
		Baud:     9600,
		Filter:   "r/0/0/100",
		Callsign: "N0CALL",
		Passcode: "12345",
	}

	tests := []struct {
		name string
		a, b transport.TransportConfig
		want bool
	}{
		{name: "identical", a: base, b: base, want: true},
		{name: "Type differs", a: base, b: withType(base, "kisstcp"), want: false},
		{name: "Name differs", a: base, b: withName(base, "other"), want: false},
		{name: "Host differs", a: base, b: withHost(base, "other.host"), want: false},
		{name: "Port differs", a: base, b: withPort(base, 9999), want: false},
		{name: "Device differs", a: base, b: withDevice(base, "/dev/ttyACM0"), want: false},
		{name: "Baud differs", a: base, b: withBaud(base, 115200), want: false},
		{name: "Filter differs", a: base, b: withFilter(base, "m/50"), want: false},
		{name: "Callsign differs", a: base, b: withCallsign(base, "W1AW"), want: false},
		{name: "Passcode differs", a: base, b: withPasscode(base, "99999"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transportConfigEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("transportConfigEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateTransport(t *testing.T) {
	tests := []struct {
		name    string
		tc      transport.TransportConfig
		wantNil bool
	}{
		{
			name:    "aprsis empty callsign",
			tc:      transport.TransportConfig{Type: "aprsis", Host: "rotate.aprs2.net", Port: 14580},
			wantNil: false,
		},
		{
			name:    "kisstcp",
			tc:      transport.TransportConfig{Type: "kisstcp", Host: "localhost", Port: 8001},
			wantNil: false,
		},
		{
			name:    "serial",
			tc:      transport.TransportConfig{Type: "serial", Device: "/dev/ttyUSB0", Baud: 9600},
			wantNil: false,
		},
		{
			name:    "bogus",
			tc:      transport.TransportConfig{Type: "bogus"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createTransport(tt.tc, "N0CALL")
			if tt.wantNil {
				if got != nil {
					t.Errorf("createTransport() = %T, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("createTransport() = nil, want non-nil")
			}
		})
	}
}

func withType(tc transport.TransportConfig, v string) transport.TransportConfig {
	tc.Type = v
	return tc
}
func withName(tc transport.TransportConfig, v string) transport.TransportConfig {
	tc.Name = v
	return tc
}
func withHost(tc transport.TransportConfig, v string) transport.TransportConfig {
	tc.Host = v
	return tc
}
func withPort(tc transport.TransportConfig, v int) transport.TransportConfig {
	tc.Port = v
	return tc
}
func withDevice(tc transport.TransportConfig, v string) transport.TransportConfig {
	tc.Device = v
	return tc
}
func withBaud(tc transport.TransportConfig, v int) transport.TransportConfig {
	tc.Baud = v
	return tc
}
func withFilter(tc transport.TransportConfig, v string) transport.TransportConfig {
	tc.Filter = v
	return tc
}
func withCallsign(tc transport.TransportConfig, v string) transport.TransportConfig {
	tc.Callsign = v
	return tc
}
func withPasscode(tc transport.TransportConfig, v string) transport.TransportConfig {
	tc.Passcode = v
	return tc
}
