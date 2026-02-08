package kisstcp

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

func TestKISSTCPType(t *testing.T) {
	tr := New(transport.TransportConfig{})
	if tr.Type() != "kisstcp" {
		t.Errorf("Type() = %q, want %q", tr.Type(), "kisstcp")
	}
}

func TestKISSTCPReceive(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	cfg := transport.TransportConfig{
		Type: "kisstcp",
		Host: "localhost",
		Port: 8001,
	}

	tr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start read loop directly with the pipe connection
	go tr.readLoop(ctx, client)

	// Build a test frame: N0CALL>APRS:!4903.50N/07201.75W-
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "!4903.50N/07201.75W-",
	}

	// Encode to AX.25 then wrap in KISS
	ax25Data, err := aprs.EncodeAX25(frame)
	if err != nil {
		t.Fatalf("EncodeAX25: %v", err)
	}
	kissFrame := aprs.KISSEncode(ax25Data)

	// Write the KISS frame to the "server" end
	_, err = server.Write(kissFrame)
	if err != nil {
		t.Fatalf("server write: %v", err)
	}

	// Read the decoded frame from the transport's receive channel
	select {
	case got := <-tr.Receive():
		if got.Source.Call != "N0CALL" {
			t.Errorf("source call = %q, want %q", got.Source.Call, "N0CALL")
		}
		if got.Destination.Call != "APRS" {
			t.Errorf("destination call = %q, want %q", got.Destination.Call, "APRS")
		}
		if got.Payload != "!4903.50N/07201.75W-" {
			t.Errorf("payload = %q, want %q", got.Payload, "!4903.50N/07201.75W-")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame on Receive() channel")
	}
}

func TestKISSTCPReceiveWithPath(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	tr := New(transport.TransportConfig{Type: "kisstcp"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tr.readLoop(ctx, client)

	// Frame with digipeater path
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL", SSID: 5},
		Destination: aprs.Address{Call: "APRS"},
		Path: []aprs.Address{
			{Call: "RELAY", HBit: true},
			{Call: "WIDE"},
		},
		Payload: "!4903.50N/07201.75W-",
	}

	ax25Data, err := aprs.EncodeAX25(frame)
	if err != nil {
		t.Fatalf("EncodeAX25: %v", err)
	}
	kissFrame := aprs.KISSEncode(ax25Data)

	_, err = server.Write(kissFrame)
	if err != nil {
		t.Fatalf("server write: %v", err)
	}

	select {
	case got := <-tr.Receive():
		if got.Source.Call != "N0CALL" || got.Source.SSID != 5 {
			t.Errorf("source = %s, want N0CALL-5", got.Source.String())
		}
		if len(got.Path) != 2 {
			t.Fatalf("path len = %d, want 2", len(got.Path))
		}
		if got.Path[0].Call != "RELAY" || !got.Path[0].HBit {
			t.Errorf("path[0] = %s, want RELAY*", got.Path[0].String())
		}
		if got.Path[1].Call != "WIDE" {
			t.Errorf("path[1] = %s, want WIDE", got.Path[1].String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame")
	}
}

func TestKISSTCPSend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tr := New(transport.TransportConfig{Type: "kisstcp"})
	tr.mu.Lock()
	tr.conn = client
	tr.mu.Unlock()

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     ":W3ADO-5  :Hello{123",
	}

	// Read from server side in goroutine
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _ := server.Read(buf)
		done <- buf[:n]
	}()

	err := tr.Send(frame)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Read what was written and decode it
	raw := <-done

	// Should be a valid KISS frame wrapping AX.25 data
	ax25Data, err := aprs.KISSDecode(raw)
	if err != nil {
		t.Fatalf("KISSDecode sent data: %v", err)
	}

	decoded, err := aprs.DecodeAX25(ax25Data)
	if err != nil {
		t.Fatalf("DecodeAX25 sent data: %v", err)
	}

	if decoded.Source.Call != "N0CALL" {
		t.Errorf("source call = %q, want %q", decoded.Source.Call, "N0CALL")
	}
	if decoded.Destination.Call != "APRS" {
		t.Errorf("dest call = %q, want %q", decoded.Destination.Call, "APRS")
	}
	if decoded.Payload != ":W3ADO-5  :Hello{123" {
		t.Errorf("payload = %q, want %q", decoded.Payload, ":W3ADO-5  :Hello{123")
	}
}

func TestKISSTCPSendNotConnected(t *testing.T) {
	tr := New(transport.TransportConfig{Type: "kisstcp"})

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "test",
	}

	err := tr.Send(frame)
	if err == nil {
		t.Fatal("expected error when sending on disconnected transport, got nil")
	}
}

func TestKISSTCPMultipleFrames(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	tr := New(transport.TransportConfig{Type: "kisstcp"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tr.readLoop(ctx, client)

	// Build and send 5 frames with different payloads
	payloads := []string{
		"!4903.50N/07201.75W-",
		">status text",
		":N0CALL-5 :Hello{001",
		"@092345z4903.50N/07201.75W_",
		"=4903.50N/07201.75W-PHG2360",
	}

	// Write all KISS frames to the server end
	var allBytes []byte
	for _, payload := range payloads {
		frame := aprs.APRSFrame{
			Source:      aprs.Address{Call: "N0CALL"},
			Destination: aprs.Address{Call: "APRS"},
			Payload:     payload,
		}
		ax25Data, err := aprs.EncodeAX25(frame)
		if err != nil {
			t.Fatalf("EncodeAX25: %v", err)
		}
		allBytes = append(allBytes, aprs.KISSEncode(ax25Data)...)
	}

	_, err := server.Write(allBytes)
	if err != nil {
		t.Fatalf("server write: %v", err)
	}

	// Read all frames
	timeout := time.After(5 * time.Second)
	for i, wantPayload := range payloads {
		select {
		case got := <-tr.Receive():
			if got.Payload != wantPayload {
				t.Errorf("frame[%d] payload = %q, want %q", i, got.Payload, wantPayload)
			}
			if got.Source.Call != "N0CALL" {
				t.Errorf("frame[%d] source = %q, want %q", i, got.Source.Call, "N0CALL")
			}
		case <-timeout:
			t.Fatalf("timeout waiting for frame %d of %d", i, len(payloads))
		}
	}
}

func TestKISSTCPStatus(t *testing.T) {
	tr := New(transport.TransportConfig{Type: "kisstcp"})

	// Initial status: not connected
	status := tr.Status()
	if status.Connected {
		t.Error("initial status should be disconnected")
	}
	if status.Type != "kisstcp" {
		t.Errorf("type = %q, want %q", status.Type, "kisstcp")
	}

	// Simulate connection: set status via the helper
	tr.setStatus(true, "")
	status = tr.Status()
	if !status.Connected {
		t.Error("expected connected after setStatus(true)")
	}
	if status.LastActivity.IsZero() {
		t.Error("expected LastActivity to be set when connected")
	}

	// Simulate disconnection
	tr.setStatus(false, "connection lost")
	status = tr.Status()
	if status.Connected {
		t.Error("expected disconnected after setStatus(false)")
	}
	if status.Error != "connection lost" {
		t.Errorf("error = %q, want %q", status.Error, "connection lost")
	}
}

func TestKISSTCPClose(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	tr := New(transport.TransportConfig{Type: "kisstcp"})
	tr.mu.Lock()
	tr.conn = client
	tr.status.Connected = true
	tr.mu.Unlock()

	err := tr.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	status := tr.Status()
	if status.Connected {
		t.Error("expected disconnected after Close")
	}

	// Verify the connection is actually closed by trying to write
	_, writeErr := client.Write([]byte("test"))
	if writeErr == nil {
		t.Error("expected error writing to closed connection")
	}
}

func TestKISSTCPSendRoundtrip(t *testing.T) {
	// Full roundtrip: Send a frame via transport, read raw bytes,
	// decode KISS+AX.25, verify it matches the original
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tr := New(transport.TransportConfig{Type: "kisstcp"})
	tr.mu.Lock()
	tr.conn = client
	tr.mu.Unlock()

	original := aprs.APRSFrame{
		Source:      aprs.Address{Call: "W3ADO", SSID: 1},
		Destination: aprs.Address{Call: "APRS"},
		Path: []aprs.Address{
			{Call: "WIDE1", SSID: 1, HBit: true},
			{Call: "WIDE2", SSID: 1},
		},
		Payload: "!4903.50N/07201.75W-PHG2360",
	}

	// Read from server in goroutine
	done := make(chan []byte, 1)
	go func() {
		// Read enough bytes — KISS frame can be up to ~300 bytes
		buf := make([]byte, 1024)
		n, _ := server.Read(buf)
		done <- buf[:n]
	}()

	err := tr.Send(original)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	raw := <-done

	// Decode KISS → AX.25 → APRSFrame
	ax25Data, err := aprs.KISSDecode(raw)
	if err != nil {
		t.Fatalf("KISSDecode: %v", err)
	}

	decoded, err := aprs.DecodeAX25(ax25Data)
	if err != nil {
		t.Fatalf("DecodeAX25: %v", err)
	}

	if decoded.Source.Call != original.Source.Call || decoded.Source.SSID != original.Source.SSID {
		t.Errorf("source = %s, want %s", decoded.Source.String(), original.Source.String())
	}
	if decoded.Destination.Call != original.Destination.Call {
		t.Errorf("dest = %s, want %s", decoded.Destination.String(), original.Destination.String())
	}
	if len(decoded.Path) != len(original.Path) {
		t.Fatalf("path len = %d, want %d", len(decoded.Path), len(original.Path))
	}
	for i := range original.Path {
		if decoded.Path[i].String() != original.Path[i].String() {
			t.Errorf("path[%d] = %s, want %s", i, decoded.Path[i].String(), original.Path[i].String())
		}
	}
	if decoded.Payload != original.Payload {
		t.Errorf("payload = %q, want %q", decoded.Payload, original.Payload)
	}
}

func TestKISSTCPReadLoopClosesOnContextCancel(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	tr := New(transport.TransportConfig{Type: "kisstcp"})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		tr.readLoop(ctx, client)
		close(done)
	}()

	// Cancel context — readLoop should exit
	cancel()

	// Close the server side to unblock the read
	server.Close()

	select {
	case <-done:
		// readLoop exited
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not exit after context cancel")
	}
}

func TestKISSTCPReadLoopClosesOnConnectionClose(t *testing.T) {
	server, client := net.Pipe()

	tr := New(transport.TransportConfig{Type: "kisstcp"})
	tr.setStatus(true, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		tr.readLoop(ctx, client)
		close(done)
	}()

	// Close server side — readLoop should detect the error and exit
	server.Close()

	select {
	case <-done:
		// readLoop exited
		status := tr.Status()
		if status.Connected {
			t.Error("expected disconnected status after read error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not exit after connection close")
	}
}

func TestKISSTCPReceiveLargeFrame(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	tr := New(transport.TransportConfig{Type: "kisstcp"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tr.readLoop(ctx, client)

	// Create a frame with a large payload (256 chars)
	var payload bytes.Buffer
	payload.WriteByte('!')
	for i := 0; i < 255; i++ {
		payload.WriteByte(byte('A' + (i % 26)))
	}

	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     payload.String(),
	}

	ax25Data, err := aprs.EncodeAX25(frame)
	if err != nil {
		t.Fatalf("EncodeAX25: %v", err)
	}
	kissFrame := aprs.KISSEncode(ax25Data)

	_, err = server.Write(kissFrame)
	if err != nil {
		t.Fatalf("server write: %v", err)
	}

	select {
	case got := <-tr.Receive():
		if got.Payload != payload.String() {
			t.Errorf("payload length = %d, want %d", len(got.Payload), payload.Len())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for large frame")
	}
}

func TestKISSTCPConnectAndReceive(t *testing.T) {
	// Full integration: start a TCP listener, connect the transport, send a frame
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)

	cfg := transport.TransportConfig{
		Type: "kisstcp",
		Host: "127.0.0.1",
		Port: addr.Port,
	}

	tr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer tr.Close()

	// Accept connection in goroutine
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- conn
	}()

	// Connect the transport
	err = tr.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Get the accepted server-side connection
	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
		defer serverConn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server to accept")
	}

	// Verify connected status
	status := tr.Status()
	if !status.Connected {
		t.Error("expected connected status after Connect")
	}

	// Send a KISS frame from "Direwolf"
	frame := aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL"},
		Destination: aprs.Address{Call: "APRS"},
		Payload:     "!4903.50N/07201.75W-",
	}
	ax25Data, encErr := aprs.EncodeAX25(frame)
	if encErr != nil {
		t.Fatalf("EncodeAX25: %v", encErr)
	}
	kissFrame := aprs.KISSEncode(ax25Data)

	_, err = serverConn.Write(kissFrame)
	if err != nil {
		t.Fatalf("server write: %v", err)
	}

	// Read the decoded frame
	select {
	case got := <-tr.Receive():
		if got.Source.Call != "N0CALL" {
			t.Errorf("source = %q, want %q", got.Source.Call, "N0CALL")
		}
		if got.Payload != "!4903.50N/07201.75W-" {
			t.Errorf("payload = %q, want %q", got.Payload, "!4903.50N/07201.75W-")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame via full Connect path")
	}
}

// readAll reads all available bytes from a reader with a deadline.
func readAll(r io.Reader, timeout time.Duration) ([]byte, error) {
	ch := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := r.Read(buf)
		ch <- buf[:n]
	}()

	select {
	case data := <-ch:
		return data, nil
	case <-time.After(timeout):
		return nil, io.ErrNoProgress
	}
}
