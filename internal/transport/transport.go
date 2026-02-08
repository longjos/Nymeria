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
	Type     string `yaml:"type"`
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Device   string `yaml:"device,omitempty"`
	Baud     int    `yaml:"baud,omitempty"`
	Filter   string `yaml:"filter,omitempty"`
	Callsign string `yaml:"callsign,omitempty"`
	Passcode string `yaml:"passcode,omitempty"`
}

// TransportStatus represents the current state of a transport.
type TransportStatus struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Connected    bool      `json:"connected"`
	LastActivity time.Time `json:"lastActivity,omitempty"`
	Error        string    `json:"error,omitempty"`
}
