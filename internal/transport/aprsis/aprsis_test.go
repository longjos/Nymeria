package aprsis

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

func TestPasscode(t *testing.T) {
	tests := []struct {
		callsign string
		want     int
	}{
		{"N0CALL", 13023},
		{"N0CALL-5", 13023}, // SSID stripped
		{"n0call", 13023},   // case-insensitive (uppercased)
		{"W3ADO", 10901},
		{"KJ4ERJ", 24231},
	}

	for _, tt := range tests {
		t.Run(tt.callsign, func(t *testing.T) {
			got := Passcode(tt.callsign)
			if got != tt.want {
				t.Errorf("Passcode(%q) = %d, want %d", tt.callsign, got, tt.want)
			}
		})
	}
}

func TestFrameStringRoundtrip(t *testing.T) {
	raw := "N0CALL-5>APRS,RELAY,WIDE:!4903.50N/07201.75W-"
	frame, err := aprs.ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	got := frame.String()
	if got != raw {
		t.Errorf("String() = %q, want %q", got, raw)
	}
}

func TestLoginHandshake(t *testing.T) {
	// Create a pipe to simulate server/client
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cfg := transport.TransportConfig{
		Type:     "aprsis",
		Host:     "localhost",
		Port:     14580,
		Callsign: "N0CALL",
		Passcode: "13023",
		Filter:   "r/49/-72/100",
	}

	tr := New(cfg)

	// Simulate server side in goroutine
	serverDone := make(chan error, 1)
	go func() {
		// Server sends banner
		fmt.Fprintf(server, "# javAPRSSrvr 4.2.0b05\r\n")

		// Server reads login line
		reader := bufio.NewReader(server)
		line, err := reader.ReadString('\n')
		if err != nil {
			serverDone <- fmt.Errorf("server read: %w", err)
			return
		}

		// Verify login format
		line = strings.TrimRight(line, "\r\n")
		if !strings.HasPrefix(line, "user N0CALL pass 13023") {
			serverDone <- fmt.Errorf("unexpected login: %q", line)
			return
		}
		if !strings.Contains(line, "vers Nymeria") {
			serverDone <- fmt.Errorf("missing version in login: %q", line)
			return
		}
		if !strings.Contains(line, "filter r/49/-72/100") {
			serverDone <- fmt.Errorf("missing filter in login: %q", line)
			return
		}

		// Server sends logresp
		fmt.Fprintf(server, "# logresp N0CALL verified, server T2ONTARIO\r\n")

		serverDone <- nil
	}()

	// Perform login
	err := tr.login(client)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Check server side completed ok
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestReadLoop(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	cfg := transport.TransportConfig{
		Type:     "aprsis",
		Callsign: "N0CALL",
		Passcode: "13023",
	}

	tr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start read loop
	go tr.readLoop(ctx, client)

	// Send some lines from "server"
	fmt.Fprintf(server, "# comment line should be skipped\r\n")
	fmt.Fprintf(server, "N0CALL-5>APRS:!4903.50N/07201.75W-\r\n")
	fmt.Fprintf(server, "# another comment\r\n")
	fmt.Fprintf(server, "W3ADO-1>APRS:>status text\r\n")

	// Read frames
	var frames []aprs.APRSFrame
	timeout := time.After(2 * time.Second)
	for len(frames) < 2 {
		select {
		case f := <-tr.frames:
			frames = append(frames, f)
		case <-timeout:
			t.Fatalf("timeout waiting for frames, got %d", len(frames))
		}
	}

	if frames[0].Source.Call != "N0CALL" || frames[0].Source.SSID != 5 {
		t.Errorf("frame[0] source = %s, want N0CALL-5", frames[0].Source.String())
	}
	if frames[1].Source.Call != "W3ADO" || frames[1].Source.SSID != 1 {
		t.Errorf("frame[1] source = %s, want W3ADO-1", frames[1].Source.String())
	}
}

func TestSend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cfg := transport.TransportConfig{
		Type:     "aprsis",
		Callsign: "N0CALL",
		Passcode: "13023",
	}

	tr := New(cfg)
	tr.conn = client

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     ":W3ADO-5  :Hello{123",
	}

	// Read from server side in goroutine
	done := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(server)
		line, _ := reader.ReadString('\n')
		done <- line
	}()

	err := tr.Send(frame)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := <-done
	want := "N0CALL>APRS::W3ADO-5  :Hello{123\r\n"
	if got != want {
		t.Errorf("sent = %q, want %q", got, want)
	}
}

func TestTransportType(t *testing.T) {
	tr := New(transport.TransportConfig{})
	if tr.Type() != "aprsis" {
		t.Errorf("Type() = %q, want %q", tr.Type(), "aprsis")
	}
}
