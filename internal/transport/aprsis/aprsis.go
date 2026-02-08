package aprsis

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

// Passcode computes the APRS-IS verification passcode for a callsign.
// The algorithm XOR-folds the uppercase callsign (without SSID) with 0x73E2 seed.
func Passcode(callsign string) int {
	// Strip SSID
	if idx := strings.IndexByte(callsign, '-'); idx >= 0 {
		callsign = callsign[:idx]
	}
	callsign = strings.ToUpper(callsign)

	hash := int(0x73E2)
	for i := 0; i < len(callsign); i += 2 {
		hash ^= int(callsign[i]) << 8
		if i+1 < len(callsign) {
			hash ^= int(callsign[i+1])
		}
	}
	return hash & 0x7FFF
}

// Transport implements transport.Transport for APRS-IS connections.
type Transport struct {
	config transport.TransportConfig
	frames chan aprs.APRSFrame
	status transport.TransportStatus

	mu   sync.Mutex
	conn net.Conn
}

// New creates a new APRS-IS transport.
func New(cfg transport.TransportConfig) *Transport {
	return &Transport{
		config: cfg,
		frames: make(chan aprs.APRSFrame, 256),
		status: transport.TransportStatus{
			Type: "aprsis",
		},
	}
}

// Connect establishes the APRS-IS connection and starts the read loop.
func (t *Transport) Connect(ctx context.Context) error {
	if err := t.dial(ctx); err != nil {
		return err
	}

	go t.reconnectLoop(ctx)
	return nil
}

// dial connects to the APRS-IS server and performs the login handshake.
func (t *Transport) dial(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", t.config.Host, t.config.Port)

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.setStatus(false, err.Error())
		return fmt.Errorf("connect to %s: %w", addr, err)
	}

	if err := t.login(conn); err != nil {
		conn.Close()
		t.setStatus(false, err.Error())
		return fmt.Errorf("login to %s: %w", addr, err)
	}

	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()

	t.setStatus(true, "")
	log.Printf("[aprsis] connected to %s", addr)

	go t.readLoop(ctx, conn)
	return nil
}

// login performs the APRS-IS authentication handshake on the given connection.
func (t *Transport) login(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	reader := bufio.NewReader(conn)

	// Read server banner (starts with #)
	banner, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read banner: %w", err)
	}
	banner = strings.TrimRight(banner, "\r\n")
	if !strings.HasPrefix(banner, "#") {
		return fmt.Errorf("unexpected banner: %q", banner)
	}

	// Send login line
	passcode := t.config.Passcode
	if passcode == "" {
		passcode = fmt.Sprintf("%d", Passcode(t.config.Callsign))
	}

	login := fmt.Sprintf("user %s pass %s vers Nymeria 0.1",
		t.config.Callsign, passcode)
	if t.config.Filter != "" {
		login += " filter " + t.config.Filter
	}
	login += "\r\n"

	if _, err := conn.Write([]byte(login)); err != nil {
		return fmt.Errorf("send login: %w", err)
	}

	// Read logresp
	resp, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read logresp: %w", err)
	}
	resp = strings.TrimRight(resp, "\r\n")
	if !strings.HasPrefix(resp, "# logresp") {
		return fmt.Errorf("unexpected logresp: %q", resp)
	}

	// Clear deadline
	conn.SetReadDeadline(time.Time{})
	return nil
}

// readLoop reads frames from the connection and sends them on the frames channel.
func (t *Transport) readLoop(ctx context.Context, conn net.Conn) {
	reader := bufio.NewReader(conn)

	for {
		// Check context periodically via read deadline
		conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		line, err := reader.ReadString('\n')
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				// Read timeout, check context and continue
				continue
			}
			log.Printf("[aprsis] read error: %v", err)
			t.setStatus(false, err.Error())
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		frame, err := aprs.ParseFrame(line)
		if err != nil {
			continue // Skip unparseable frames
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

		// Wait for connection to drop (read loop exits)
		// We detect this by checking if the connection is nil or status is disconnected
		time.Sleep(1 * time.Second)

		t.mu.Lock()
		connected := t.status.Connected
		t.mu.Unlock()

		if connected {
			backoff = 1 * time.Second // Reset backoff when connected
			continue
		}

		log.Printf("[aprsis] reconnecting in %v", backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if err := t.dial(ctx); err != nil {
			log.Printf("[aprsis] reconnect failed: %v", err)
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

// Send transmits an APRS frame to the APRS-IS server.
func (t *Transport) Send(frame aprs.APRSFrame) error {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	data := frame.String() + "\r\n"
	_, err := conn.Write([]byte(data))
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
	return "aprsis"
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
