package serial

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"go.bug.st/serial"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

// Port abstracts a serial port connection (for testability).
type Port interface {
	io.ReadWriteCloser
}

// portOpener is the function used to open serial ports.
// Can be replaced in tests with a mock.
type portOpener func(device string, baud int) (Port, error)

// openRealPort opens a serial port using go.bug.st/serial.
func openRealPort(device string, baud int) (Port, error) {
	mode := &serial.Mode{BaudRate: baud}
	return serial.Open(device, mode)
}

// Transport implements transport.Transport for serial KISS TNCs.
type Transport struct {
	config   transport.TransportConfig
	frames   chan aprs.APRSFrame
	status   transport.TransportStatus
	openPort portOpener

	mu     sync.Mutex
	port   Port
	cancel context.CancelFunc
}

// New creates a new serial KISS transport.
func New(cfg transport.TransportConfig) *Transport {
	return &Transport{
		config:   cfg,
		frames:   make(chan aprs.APRSFrame, 256),
		status:   transport.TransportStatus{Type: "serial"},
		openPort: openRealPort,
	}
}

// Connect opens the serial port and starts the read/reconnect loops.
func (t *Transport) Connect(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	t.mu.Lock()
	t.cancel = cancel
	t.mu.Unlock()

	if err := t.open(); err != nil {
		cancel()
		return err
	}

	go t.reconnectLoop(ctx)
	return nil
}

// open connects to the serial port and starts the read loop.
func (t *Transport) open() error {
	baud := t.config.Baud
	if baud == 0 {
		baud = 9600
	}

	port, err := t.openPort(t.config.Device, baud)
	if err != nil {
		t.setStatus(false, err.Error())
		return fmt.Errorf("open %s: %w", t.config.Device, err)
	}

	t.mu.Lock()
	t.port = port
	t.mu.Unlock()

	t.setStatus(true, "")
	log.Printf("[serial] connected to %s at %d baud", t.config.Device, baud)

	go t.readLoop(port)
	return nil
}

// readLoop reads KISS frames from the port and delivers decoded APRSFrames.
func (t *Transport) readLoop(port Port) {
	kr := aprs.NewKISSFrameReader(port)

	for {
		ax25Data, err := kr.ReadFrame()
		if err != nil {
			t.setStatus(false, err.Error())
			return
		}

		frame, err := aprs.DecodeAX25(ax25Data)
		if err != nil {
			log.Printf("[serial] AX.25 decode error: %v", err)
			continue
		}

		t.setActivity()

		select {
		case t.frames <- frame:
		default:
			// Channel full, drop frame
		}
	}
}

// reconnectLoop handles automatic reconnection with exponential backoff.
func (t *Transport) reconnectLoop(ctx context.Context) {
	backoff := 1 * time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		time.Sleep(1 * time.Second)

		t.mu.Lock()
		connected := t.status.Connected
		t.mu.Unlock()

		if connected {
			backoff = 1 * time.Second
			continue
		}

		log.Printf("[serial] reconnecting in %v", backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if err := t.open(); err != nil {
			log.Printf("[serial] reconnect failed: %v", err)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			backoff = 1 * time.Second
		}
	}
}

// Send transmits an APRS frame over the serial port as KISS-wrapped AX.25.
func (t *Transport) Send(frame aprs.APRSFrame) error {
	t.mu.Lock()
	port := t.port
	t.mu.Unlock()

	if port == nil {
		return fmt.Errorf("not connected")
	}

	ax25Data, err := aprs.EncodeAX25(frame)
	if err != nil {
		return fmt.Errorf("AX.25 encode: %w", err)
	}

	kissData := aprs.KISSEncode(ax25Data)
	if _, err := port.Write(kissData); err != nil {
		return fmt.Errorf("serial write: %w", err)
	}

	return nil
}

// Receive returns a channel of received APRS frames.
func (t *Transport) Receive() <-chan aprs.APRSFrame {
	return t.frames
}

// Close shuts down the transport.
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	if t.port != nil {
		err := t.port.Close()
		t.port = nil
		t.status.Connected = false
		return err
	}

	t.status.Connected = false
	return nil
}

// Status returns the current transport status.
func (t *Transport) Status() transport.TransportStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Type returns the transport type identifier.
func (t *Transport) Type() string {
	return "serial"
}

func (t *Transport) setStatus(connected bool, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Connected = connected
	t.status.Error = errMsg
	if connected {
		t.status.LastActivity = time.Now()
	}
}

func (t *Transport) setActivity() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.LastActivity = time.Now()
}
