package aprsis

import (
	"context"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

// Transport implements transport.Transport for APRS-IS connections.
type Transport struct {
	config transport.TransportConfig
	frames chan aprs.APRSFrame
	status transport.TransportStatus
}

// New creates a new APRS-IS transport.
func New(cfg transport.TransportConfig) *Transport {
	return &Transport{
		config: cfg,
		frames: make(chan aprs.APRSFrame, 64),
		status: transport.TransportStatus{
			Type: "aprsis",
		},
	}
}

func (t *Transport) Connect(_ context.Context) error {
	// TODO: implement APRS-IS connection
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
	return "aprsis"
}
