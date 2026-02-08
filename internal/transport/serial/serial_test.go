package serial

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

// newTestTransport creates a serial transport with a mock port backed by net.Pipe.
// Returns the transport and the "TNC side" of the pipe (write KISS data here to simulate TNC).
func newTestTransport(cfg transport.TransportConfig) (*Transport, net.Conn) {
	tncSide, nymeriaSide := net.Pipe()
	t := New(cfg)
	t.openPort = func(device string, baud int) (Port, error) {
		return nymeriaSide, nil
	}
	return t, tncSide
}

// testFrame returns a simple APRSFrame for testing.
func testFrame() aprs.APRSFrame {
	return aprs.APRSFrame{
		Source:      aprs.Address{Call: "N0CALL", SSID: 1},
		Destination: aprs.Address{Call: "APRS"},
		Path:        []aprs.Address{{Call: "WIDE1", SSID: 1}, {Call: "WIDE2", SSID: 1}},
		Payload:     "!3518.00N/13623.00E-Test",
	}
}

// encodeTestFrame encodes a frame into KISS-wrapped AX.25 bytes.
func encodeTestFrame(t *testing.T, frame aprs.APRSFrame) []byte {
	t.Helper()
	ax25, err := aprs.EncodeAX25(frame)
	if err != nil {
		t.Fatalf("EncodeAX25: %v", err)
	}
	return aprs.KISSEncode(ax25)
}

func TestSerialType(t *testing.T) {
	tr := New(transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/ttyUSB0",
		Baud:   9600,
	})
	if got := tr.Type(); got != "serial" {
		t.Errorf("Type() = %q, want %q", got, "serial")
	}
}

func TestSerialDefaultBaud(t *testing.T) {
	// When baud is 0 (unset), Connect should default to 9600
	var capturedBaud int
	tncSide, nymeriaSide := net.Pipe()
	defer tncSide.Close()
	defer nymeriaSide.Close()

	tr := New(transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/ttyUSB0",
		Baud:   0, // unset
	})
	tr.openPort = func(device string, baud int) (Port, error) {
		capturedBaud = baud
		return nymeriaSide, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	if capturedBaud != 9600 {
		t.Errorf("baud = %d, want 9600 (default)", capturedBaud)
	}
}

func TestSerialReceive(t *testing.T) {
	cfg := transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/ttyUSB0",
		Baud:   9600,
	}
	tr, tncSide := newTestTransport(cfg)
	defer tncSide.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	// Simulate TNC sending a KISS frame
	frame := testFrame()
	kissData := encodeTestFrame(t, frame)
	if _, err := tncSide.Write(kissData); err != nil {
		t.Fatalf("write KISS data: %v", err)
	}

	// Read from transport's Receive channel
	select {
	case got := <-tr.Receive():
		if got.Source.Call != frame.Source.Call {
			t.Errorf("Source.Call = %q, want %q", got.Source.Call, frame.Source.Call)
		}
		if got.Destination.Call != frame.Destination.Call {
			t.Errorf("Destination.Call = %q, want %q", got.Destination.Call, frame.Destination.Call)
		}
		if got.Payload != frame.Payload {
			t.Errorf("Payload = %q, want %q", got.Payload, frame.Payload)
		}
		if len(got.Path) != len(frame.Path) {
			t.Errorf("Path len = %d, want %d", len(got.Path), len(frame.Path))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame on Receive channel")
	}
}

func TestSerialSend(t *testing.T) {
	cfg := transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/ttyUSB0",
		Baud:   9600,
	}
	tr, tncSide := newTestTransport(cfg)
	defer tncSide.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	frame := testFrame()

	// Send in a goroutine because net.Pipe writes block until the other side reads
	errCh := make(chan error, 1)
	go func() {
		errCh <- tr.Send(frame)
	}()

	// Read from the TNC side and verify it's a valid KISS-wrapped AX.25 frame
	buf := make([]byte, 1024)
	tncSide.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := tncSide.Read(buf)
	if err != nil {
		t.Fatalf("read from TNC side: %v", err)
	}

	// Check Send didn't error
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send timed out")
	}

	// Decode the KISS frame
	ax25Data, err := aprs.KISSDecode(buf[:n])
	if err != nil {
		t.Fatalf("KISSDecode: %v", err)
	}

	// Decode the AX.25 data
	decoded, err := aprs.DecodeAX25(ax25Data)
	if err != nil {
		t.Fatalf("DecodeAX25: %v", err)
	}

	if decoded.Source.Call != frame.Source.Call {
		t.Errorf("decoded Source.Call = %q, want %q", decoded.Source.Call, frame.Source.Call)
	}
	if decoded.Destination.Call != frame.Destination.Call {
		t.Errorf("decoded Destination.Call = %q, want %q", decoded.Destination.Call, frame.Destination.Call)
	}
	if decoded.Payload != frame.Payload {
		t.Errorf("decoded Payload = %q, want %q", decoded.Payload, frame.Payload)
	}
}

func TestSerialSendNotConnected(t *testing.T) {
	tr := New(transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/ttyUSB0",
		Baud:   9600,
	})

	err := tr.Send(testFrame())
	if err == nil {
		t.Fatal("Send on disconnected transport should return error")
	}
}

func TestSerialMultipleFrames(t *testing.T) {
	cfg := transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/ttyUSB0",
		Baud:   9600,
	}
	tr, tncSide := newTestTransport(cfg)
	defer tncSide.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	// Build several distinct frames
	frames := []aprs.APRSFrame{
		{
			Source:      aprs.Address{Call: "N0CALL", SSID: 1},
			Destination: aprs.Address{Call: "APRS"},
			Payload:     "!3518.00N/13623.00E-Frame1",
		},
		{
			Source:      aprs.Address{Call: "W1AW", SSID: 0},
			Destination: aprs.Address{Call: "APRS"},
			Payload:     "!4100.00N/07200.00W-Frame2",
		},
		{
			Source:      aprs.Address{Call: "VE3XYZ", SSID: 9},
			Destination: aprs.Address{Call: "APRS"},
			Path:        []aprs.Address{{Call: "WIDE1", SSID: 1}},
			Payload:     ">Status message",
		},
	}

	// Write all frames to TNC side
	for _, f := range frames {
		kissData := encodeTestFrame(t, f)
		if _, err := tncSide.Write(kissData); err != nil {
			t.Fatalf("write KISS data: %v", err)
		}
	}

	// Read them all back
	for i, want := range frames {
		select {
		case got := <-tr.Receive():
			if got.Source.Call != want.Source.Call {
				t.Errorf("frame[%d]: Source.Call = %q, want %q", i, got.Source.Call, want.Source.Call)
			}
			if got.Payload != want.Payload {
				t.Errorf("frame[%d]: Payload = %q, want %q", i, got.Payload, want.Payload)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("frame[%d]: timeout waiting for frame", i)
		}
	}
}

func TestSerialStatus(t *testing.T) {
	cfg := transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/ttyUSB0",
		Baud:   9600,
	}
	tr, tncSide := newTestTransport(cfg)
	defer tncSide.Close()

	// Before connection
	status := tr.Status()
	if status.Connected {
		t.Error("status.Connected should be false before Connect")
	}
	if status.Type != "serial" {
		t.Errorf("status.Type = %q, want %q", status.Type, "serial")
	}

	// After connection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	status = tr.Status()
	if !status.Connected {
		t.Error("status.Connected should be true after Connect")
	}

	// After close
	tr.Close()

	status = tr.Status()
	if status.Connected {
		t.Error("status.Connected should be false after Close")
	}
}

func TestSerialCloseWithoutConnect(t *testing.T) {
	tr := New(transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/ttyUSB0",
		Baud:   9600,
	})

	// Close without Connect should not panic
	if err := tr.Close(); err != nil {
		t.Errorf("Close on unused transport: %v", err)
	}
}

func TestSerialConnectDeviceError(t *testing.T) {
	tr := New(transport.TransportConfig{
		Type:   "serial",
		Device: "/dev/nonexistent",
		Baud:   9600,
	})
	tr.openPort = func(device string, baud int) (Port, error) {
		return nil, &net.OpError{Op: "open", Err: fmt.Errorf("no such device")}
	}

	ctx := context.Background()
	err := tr.Connect(ctx)
	if err == nil {
		t.Fatal("Connect with bad device should return error")
	}
}
