package serial

import (
	"context"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

// Transport implements transport.Transport for serial KISS TNCs.
type Transport struct {
	config transport.TransportConfig
	frames chan aprs.APRSFrame
	status transport.TransportStatus
}

// New creates a new serial KISS transport.
func New(cfg transport.TransportConfig) *Transport {
	return &Transport{
		config: cfg,
		frames: make(chan aprs.APRSFrame, 64),
		status: transport.TransportStatus{
			Type: "serial",
		},
	}
}

func (t *Transport) Connect(_ context.Context) error {
	// TODO: implement serial connection (requires go.bug.st/serial)
	return nil
}

func (t *Transport) Close() error {
	close(t.frames)
	return nil
}

func (t *Transport) Send(_ aprs.APRSFrame) error {
	// TODO: implement send
	return nil
}

func (t *Transport) Receive() <-chan aprs.APRSFrame {
	return t.frames
}

func (t *Transport) Status() transport.TransportStatus {
	return t.status
}

func (t *Transport) Type() string {
	return "serial"
}
