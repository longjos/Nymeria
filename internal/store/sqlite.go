package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/station"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 2

// SQLiteStore implements Store using modernc.org/sqlite.
type SQLiteStore struct {
	path string
	db   *sql.DB
}

// NewSQLiteStore creates a new SQLite store at the given path.
func NewSQLiteStore(path string) *SQLiteStore {
	return &SQLiteStore{path: path}
}

func (s *SQLiteStore) Init() error {
	// Ensure parent directory exists.
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		return fmt.Errorf("open sqlite db: %w", err)
	}
	s.db = db

	// Enable WAL mode for better concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("enable WAL mode: %w", err)
	}

	if err := s.migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrate() error {
	// Create schema_version table if it doesn't exist.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	var version int
	row := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1")
	if err := row.Scan(&version); err != nil {
		// No row yet — first init.
		version = 0
	}

	if version < 1 {
		if err := s.migrateV1(); err != nil {
			return err
		}
	}

	if version < 2 {
		if err := s.migrateV2(); err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLiteStore) migrateV1() error {
	ddl := `
CREATE TABLE IF NOT EXISTS stations (
    callsign TEXT NOT NULL,
    ssid INTEGER NOT NULL DEFAULT 0,
    last_heard DATETIME NOT NULL,
    lat REAL,
    lon REAL,
    altitude REAL,
    speed REAL,
    course REAL,
    symbol_table TEXT,
    symbol_code TEXT,
    comment TEXT,
    source TEXT,
    PRIMARY KEY (callsign, ssid)
);

CREATE TABLE IF NOT EXISTS tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    callsign TEXT NOT NULL,
    ssid INTEGER NOT NULL DEFAULT 0,
    lat REAL NOT NULL,
    lon REAL NOT NULL,
    time DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tracks_callsign ON tracks(callsign, ssid);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    from_call TEXT NOT NULL,
    to_call TEXT NOT NULL,
    body TEXT NOT NULL,
    msg_no TEXT NOT NULL DEFAULT '',
    state INTEGER NOT NULL DEFAULT 0,
    retries INTEGER NOT NULL DEFAULT 0,
    inbound INTEGER NOT NULL DEFAULT 0,
    timestamp DATETIME NOT NULL
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("create tables: %w", err)
	}

	// Set schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", currentSchemaVersion); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) SaveStation(st station.Station) error {
	var lat, lon, alt, spd, crs sql.NullFloat64
	var symTable, symCode sql.NullString

	if st.Position != nil {
		lat = sql.NullFloat64{Float64: st.Position.Lat, Valid: true}
		lon = sql.NullFloat64{Float64: st.Position.Lon, Valid: true}
		alt = sql.NullFloat64{Float64: st.Position.Altitude, Valid: true}
		spd = sql.NullFloat64{Float64: st.Position.Speed, Valid: true}
		crs = sql.NullFloat64{Float64: st.Position.Course, Valid: true}
	}

	if st.Symbol.Table != 0 {
		symTable = sql.NullString{String: string(st.Symbol.Table), Valid: true}
	}
	if st.Symbol.Code != 0 {
		symCode = sql.NullString{String: string(st.Symbol.Code), Valid: true}
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO stations
			(callsign, ssid, last_heard, lat, lon, altitude, speed, course, symbol_table, symbol_code, comment, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.Callsign, st.SSID, st.LastHeard.UTC(),
		lat, lon, alt, spd, crs,
		symTable, symCode,
		st.Comment, st.Source,
	)
	if err != nil {
		return fmt.Errorf("save station: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadStations() ([]station.Station, error) {
	rows, err := s.db.Query(`
		SELECT callsign, ssid, last_heard, lat, lon, altitude, speed, course,
		       symbol_table, symbol_code, comment, source
		FROM stations`)
	if err != nil {
		return nil, fmt.Errorf("query stations: %w", err)
	}
	defer rows.Close()

	var stations []station.Station
	for rows.Next() {
		var st station.Station
		var lastHeard string
		var lat, lon, alt, spd, crs sql.NullFloat64
		var symTable, symCode sql.NullString
		var comment, source sql.NullString

		if err := rows.Scan(
			&st.Callsign, &st.SSID, &lastHeard,
			&lat, &lon, &alt, &spd, &crs,
			&symTable, &symCode,
			&comment, &source,
		); err != nil {
			return nil, fmt.Errorf("scan station: %w", err)
		}

		st.LastHeard, err = time.Parse(time.RFC3339Nano, lastHeard)
		if err != nil {
			// Try a fallback format that SQLite might use.
			st.LastHeard, err = time.Parse("2006-01-02 15:04:05-07:00", lastHeard)
			if err != nil {
				st.LastHeard, err = time.Parse("2006-01-02T15:04:05Z", lastHeard)
				if err != nil {
					return nil, fmt.Errorf("parse last_heard %q: %w", lastHeard, err)
				}
			}
		}

		if lat.Valid && lon.Valid {
			st.Position = &station.Position{
				Lat: lat.Float64,
				Lon: lon.Float64,
			}
			if alt.Valid {
				st.Position.Altitude = alt.Float64
			}
			if spd.Valid {
				st.Position.Speed = spd.Float64
			}
			if crs.Valid {
				st.Position.Course = crs.Float64
			}
		}

		if symTable.Valid && len(symTable.String) > 0 {
			st.Symbol.Table = symTable.String[0]
		}
		if symCode.Valid && len(symCode.String) > 0 {
			st.Symbol.Code = symCode.String[0]
		}

		if comment.Valid {
			st.Comment = comment.String
		}
		if source.Valid {
			st.Source = source.String
		}

		stations = append(stations, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stations: %w", err)
	}

	return stations, nil
}

func (s *SQLiteStore) SaveMessage(m message.Message) error {
	inbound := 0
	if m.Inbound {
		inbound = 1
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO messages
			(id, from_call, to_call, body, msg_no, state, retries, inbound, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.From, m.To, m.Body, m.MsgNo,
		int(m.State), m.Retries, inbound, m.Timestamp.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadMessages() ([]message.Message, error) {
	rows, err := s.db.Query(`
		SELECT id, from_call, to_call, body, msg_no, state, retries, inbound, timestamp
		FROM messages
		ORDER BY timestamp ASC`)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []message.Message
	for rows.Next() {
		var m message.Message
		var ts string
		var state, inbound int

		if err := rows.Scan(&m.ID, &m.From, &m.To, &m.Body, &m.MsgNo, &state, &m.Retries, &inbound, &ts); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}

		m.Timestamp, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			m.Timestamp, err = time.Parse("2006-01-02 15:04:05-07:00", ts)
			if err != nil {
				m.Timestamp, err = time.Parse("2006-01-02T15:04:05Z", ts)
				if err != nil {
					return nil, fmt.Errorf("parse timestamp %q: %w", ts, err)
				}
			}
		}

		m.State = message.MessageState(state)
		m.Inbound = inbound != 0

		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return messages, nil
}

func (s *SQLiteStore) SaveTrackPoint(callsign string, tp station.TrackPoint) error {
	_, err := s.db.Exec(`
		INSERT INTO tracks (callsign, ssid, lat, lon, time)
		VALUES (?, 0, ?, ?, ?)`,
		callsign, tp.Lat, tp.Lon, tp.Time.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save track point: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadTrackPoints(callsign string, limit int) ([]station.TrackPoint, error) {
	// Select the most recent N track points, then return them in ascending
	// time order so the caller gets a chronological slice.
	rows, err := s.db.Query(`
		SELECT lat, lon, time FROM (
			SELECT lat, lon, time
			FROM tracks
			WHERE callsign = ?
			ORDER BY time DESC
			LIMIT ?
		) sub
		ORDER BY time ASC`,
		callsign, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query track points: %w", err)
	}
	defer rows.Close()

	var points []station.TrackPoint
	for rows.Next() {
		var tp station.TrackPoint
		var ts string

		if err := rows.Scan(&tp.Lat, &tp.Lon, &ts); err != nil {
			return nil, fmt.Errorf("scan track point: %w", err)
		}

		tp.Time, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			tp.Time, err = time.Parse("2006-01-02 15:04:05-07:00", ts)
			if err != nil {
				tp.Time, err = time.Parse("2006-01-02T15:04:05Z", ts)
				if err != nil {
					return nil, fmt.Errorf("parse track time %q: %w", ts, err)
				}
			}
		}

		points = append(points, tp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate track points: %w", err)
	}

	return points, nil
}

func (s *SQLiteStore) migrateV2() error {
	ddl := `
CREATE TABLE IF NOT EXISTS activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    user_id TEXT,
    user_name TEXT,
    action TEXT NOT NULL,
    target TEXT,
    details TEXT
);
CREATE INDEX IF NOT EXISTS idx_activity_timestamp ON activity_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_activity_user ON activity_log(user_id);

CREATE TABLE IF NOT EXISTS annotations (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    geometry TEXT NOT NULL,
    style TEXT NOT NULL DEFAULT '{}',
    created_by TEXT,
    created_by_name TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("create v2 tables: %w", err)
	}

	// Add claim columns to messages table.
	// ALTER TABLE ADD COLUMN is idempotent if run multiple times in some SQLite
	// versions, but we guard against errors for columns that already exist.
	for _, col := range []string{
		"ALTER TABLE messages ADD COLUMN claimed_by TEXT DEFAULT NULL",
		"ALTER TABLE messages ADD COLUMN claimed_at DATETIME DEFAULT NULL",
	} {
		if _, err := s.db.Exec(col); err != nil {
			// Ignore "duplicate column" errors for idempotency.
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("alter messages: %w", err)
			}
		}
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 2); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func isDuplicateColumnError(err error) bool {
	return err != nil && (contains(err.Error(), "duplicate column") || contains(err.Error(), "already exists"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (s *SQLiteStore) LogActivity(entry ActivityLogEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO activity_log (timestamp, user_id, user_name, action, target, details)
		VALUES (?, ?, ?, ?, ?, ?)`,
		entry.Timestamp.UTC(), entry.UserID, entry.UserName,
		entry.Action, entry.Target, entry.Details,
	)
	if err != nil {
		return fmt.Errorf("log activity: %w", err)
	}
	return nil
}

func (s *SQLiteStore) QueryActivity(filter ActivityFilter) ([]ActivityLogEntry, int, error) {
	where := ""
	args := []interface{}{}

	addFilter := func(clause string, val interface{}) {
		if where == "" {
			where = " WHERE "
		} else {
			where += " AND "
		}
		where += clause
		args = append(args, val)
	}

	if filter.Since != nil {
		addFilter("timestamp >= ?", filter.Since.UTC())
	}
	if filter.Until != nil {
		addFilter("timestamp <= ?", filter.Until.UTC())
	}
	if filter.UserID != "" {
		addFilter("user_id = ?", filter.UserID)
	}
	if filter.Action != "" {
		addFilter("action = ?", filter.Action)
	}

	// Count total matching entries.
	var total int
	countQuery := "SELECT COUNT(*) FROM activity_log" + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count activity: %w", err)
	}

	// Fetch entries with pagination.
	query := "SELECT id, timestamp, user_id, user_name, action, target, details FROM activity_log" +
		where + " ORDER BY timestamp DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query activity: %w", err)
	}
	defer rows.Close()

	var entries []ActivityLogEntry
	for rows.Next() {
		var e ActivityLogEntry
		var ts string
		var userID, userName, target, details sql.NullString

		if err := rows.Scan(&e.ID, &ts, &userID, &userName, &e.Action, &target, &details); err != nil {
			return nil, 0, fmt.Errorf("scan activity: %w", err)
		}

		e.Timestamp, err = parseTime(ts)
		if err != nil {
			return nil, 0, fmt.Errorf("parse activity timestamp %q: %w", ts, err)
		}

		if userID.Valid {
			e.UserID = userID.String
		}
		if userName.Valid {
			e.UserName = userName.String
		}
		if target.Valid {
			e.Target = target.String
		}
		if details.Valid {
			e.Details = details.String
		}

		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate activity: %w", err)
	}

	return entries, total, nil
}

func (s *SQLiteStore) SaveAnnotation(a Annotation) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO annotations
			(id, type, label, description, geometry, style, created_by, created_by_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Type, a.Label, a.Description, a.Geometry, a.Style,
		a.CreatedBy, a.CreatedByName, a.CreatedAt.UTC(), a.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save annotation: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadAnnotations() ([]Annotation, error) {
	rows, err := s.db.Query(`
		SELECT id, type, label, description, geometry, style,
		       created_by, created_by_name, created_at, updated_at
		FROM annotations
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()

	var annotations []Annotation
	for rows.Next() {
		var a Annotation
		var createdAt, updatedAt string
		var description, style, createdBy, createdByName sql.NullString

		if err := rows.Scan(
			&a.ID, &a.Type, &a.Label, &description,
			&a.Geometry, &style, &createdBy, &createdByName,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan annotation: %w", err)
		}

		a.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse annotation created_at %q: %w", createdAt, err)
		}
		a.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse annotation updated_at %q: %w", updatedAt, err)
		}

		if description.Valid {
			a.Description = description.String
		}
		if style.Valid {
			a.Style = style.String
		}
		if createdBy.Valid {
			a.CreatedBy = createdBy.String
		}
		if createdByName.Valid {
			a.CreatedByName = createdByName.String
		}

		annotations = append(annotations, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate annotations: %w", err)
	}

	return annotations, nil
}

func (s *SQLiteStore) DeleteAnnotation(id string) error {
	_, err := s.db.Exec("DELETE FROM annotations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete annotation: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateMessageClaim(messageID string, claimedBy string, claimedAt *time.Time) error {
	var claimedAtVal interface{}
	if claimedAt != nil {
		claimedAtVal = claimedAt.UTC()
	}

	var claimedByVal interface{}
	if claimedBy != "" {
		claimedByVal = claimedBy
	}

	_, err := s.db.Exec(
		"UPDATE messages SET claimed_by = ?, claimed_at = ? WHERE id = ?",
		claimedByVal, claimedAtVal, messageID,
	)
	if err != nil {
		return fmt.Errorf("update message claim: %w", err)
	}
	return nil
}

// parseTime tries multiple time formats that SQLite might produce.
func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05+00:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %s", s)
}

// Compile-time check that SQLiteStore implements Store.
var _ Store = (*SQLiteStore)(nil)
