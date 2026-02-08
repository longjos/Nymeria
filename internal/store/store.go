package store

import (
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/station"
)

// Store provides persistent storage for stations, messages, and configuration.
type Store interface {
	// Init initializes the store (creates tables, etc).
	Init() error

	// Close closes the store.
	Close() error

	// SaveStation persists a station record.
	SaveStation(s station.Station) error

	// LoadStations loads all stored stations.
	LoadStations() ([]station.Station, error)

	// SaveMessage persists a message.
	SaveMessage(m message.Message) error

	// LoadMessages loads all messages.
	LoadMessages() ([]message.Message, error)
}
