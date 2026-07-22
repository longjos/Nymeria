package transport

import (
	"context"
	"time"

	"github.com/narvel/nymeria/internal/aprs"
)

// Transport defines a pluggable APRS transport (APRS-IS, KISS TCP, Serial).
type Transport interface {
	// Connect establishes the transport connection.
	Connect(ctx context.Context) error

	// Close shuts down the transport.
	Close() error

	// Send transmits an APRS frame.
	Send(frame aprs.APRSFrame) error

	// Receive returns a channel of received APRS frames.
	Receive() <-chan aprs.APRSFrame

	// Status returns the current transport status.
	Status() TransportStatus

	// Type returns the transport type identifier.
	Type() string
}

// TransportConfig holds configuration for a transport.
type TransportConfig struct {
	Type string `yaml:"type" json:"type"`
	// Name is an optional human-friendly label for this transport instance
	// (e.g. "HA2 BT TNC"). When set it is used as the transport's display
	// identity in the UI and for per-transport station source tagging. When
	// empty, the transport Type is used, so behavior stays generic and
	// deployment-agnostic.
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Host     string `yaml:"host,omitempty" json:"host,omitempty"`
	Port     int    `yaml:"port,omitempty" json:"port,omitempty"`
	Device   string `yaml:"device,omitempty" json:"device,omitempty"`
	Baud     int    `yaml:"baud,omitempty" json:"baud,omitempty"`
	Filter   string `yaml:"filter,omitempty" json:"filter,omitempty"`
	Callsign string `yaml:"callsign,omitempty" json:"callsign,omitempty"`
	Passcode string `yaml:"passcode,omitempty" json:"passcode,omitempty"`
}

// TransportStatus represents the current state of a transport.
type TransportStatus struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// Name is the transport's display identity (custom Name if configured,
	// otherwise Type).
	Name         string    `json:"name,omitempty"`
	Connected    bool      `json:"connected"`
	LastActivity time.Time `json:"lastActivity,omitempty"`
	Error        string    `json:"error,omitempty"`
	PacketsRx    int64     `json:"packetsRx"`
	PacketsTx    int64     `json:"packetsTx"`
}

// TransportFrame wraps an APRSFrame with metadata about which transport delivered it.
type TransportFrame struct {
	Frame  aprs.APRSFrame
	Source string // Transport ID that received this frame (e.g. "aprsis-0")
	// SourceType is the transport's generic type name (e.g. "aprsis",
	// "kisstcp", "serial"). Used for source classification without
	// hardcoding deployment-specific transport instances.
	SourceType string
	// SourceName is the transport's display identity: its configured custom
	// Name when set, otherwise its Type. This is what stations are tagged
	// with so per-instance filtering works while remaining generic.
	SourceName string
}
