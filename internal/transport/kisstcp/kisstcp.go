package kisstcp

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

// Transport implements transport.Transport for KISS over TCP (e.g. Direwolf).
type Transport struct {
	config transport.TransportConfig
	frames chan aprs.APRSFrame
	status transport.TransportStatus

	mu   sync.Mutex
	conn net.Conn
}

// New creates a new KISS TCP transport.
func New(cfg transport.TransportConfig) *Transport {
	return &Transport{
		config: cfg,
		frames: make(chan aprs.APRSFrame, 256),
		status: transport.TransportStatus{
			Type: "kisstcp",
		},
	}
}

// Connect establishes the KISS TCP connection and starts the read loop.
func (t *Transport) Connect(ctx context.Context) error {
	if err := t.dial(ctx); err != nil {
		return err
	}

	go t.reconnectLoop(ctx)
	return nil
}

// dial connects to the KISS TCP server (e.g. Direwolf).
func (t *Transport) dial(ctx context.Context) error {
	host := t.config.Host
	if host == "" {
		host = "localhost"
	}
	port := t.config.Port
	if port == 0 {
		port = 8001
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.setStatus(false, err.Error())
		return fmt.Errorf("connect to %s: %w", addr, err)
	}

	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()

	t.setStatus(true, "")
	log.Printf("[kisstcp] connected to %s", addr)

	go t.readLoop(ctx, conn)
	return nil
}

// readLoop reads KISS frames from the connection, decodes AX.25, and delivers APRSFrames.
func (t *Transport) readLoop(ctx context.Context, conn net.Conn) {
	kr := aprs.NewKISSFrameReader(conn)

	for {
		// Set read deadline so we can check context periodically
		conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		ax25Data, err := kr.ReadFrame()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if err == io.EOF {
				log.Printf("[kisstcp] connection closed (EOF)")
				t.setStatus(false, "connection closed")
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				// Read timeout — check context and continue
				continue
			}
			log.Printf("[kisstcp] read error: %v", err)
			t.setStatus(false, err.Error())
			return
		}

		frame, err := aprs.DecodeAX25(ax25Data)
		if err != nil {
			log.Printf("[kisstcp] AX.25 decode error: %v", err)
			continue // Skip malformed frames
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

		log.Printf("[kisstcp] reconnecting in %v", backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if err := t.dial(ctx); err != nil {
			log.Printf("[kisstcp] reconnect failed: %v", err)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			backoff = 1 * time.Second
		}
	}
}

// Close shuts down the transport.
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		err := t.conn.Close()
		t.conn = nil
		t.status.Connected = false
		return err
	}
	return nil
}

// Send transmits an APRS frame as KISS-encoded AX.25 over the TCP connection.
func (t *Transport) Send(frame aprs.APRSFrame) error {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	ax25Data, err := aprs.EncodeAX25(frame)
	if err != nil {
		return fmt.Errorf("encode AX.25: %w", err)
	}

	kissFrame := aprs.KISSEncode(ax25Data)

	_, err = conn.Write(kissFrame)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

// Receive returns a channel of received APRS frames.
func (t *Transport) Receive() <-chan aprs.APRSFrame {
	return t.frames
}

// Status returns the current transport status.
func (t *Transport) Status() transport.TransportStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Type returns the transport type identifier.
func (t *Transport) Type() string {
	return "kisstcp"
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
