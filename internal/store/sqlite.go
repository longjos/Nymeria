package store

import (
	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/station"
)

// SQLiteStore implements Store using modernc.org/sqlite.
type SQLiteStore struct {
	path string
}

// NewSQLiteStore creates a new SQLite store at the given path.
func NewSQLiteStore(path string) *SQLiteStore {
	return &SQLiteStore{path: path}
}

func (s *SQLiteStore) Init() error {
	// TODO: create tables
	return nil
}

func (s *SQLiteStore) Close() error {
	return nil
}

func (s *SQLiteStore) SaveStation(_ station.Station) error {
	// TODO: implement
	return nil
}

func (s *SQLiteStore) LoadStations() ([]station.Station, error) {
	// TODO: implement
	return nil, nil
}

func (s *SQLiteStore) SaveMessage(_ message.Message) error {
	// TODO: implement
	return nil
}

func (s *SQLiteStore) LoadMessages() ([]message.Message, error) {
	// TODO: implement
	return nil, nil
}
