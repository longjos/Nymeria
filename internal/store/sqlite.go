package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/station"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 30

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

	// SQLite only supports one writer at a time. Limiting to a single
	// connection serializes all access and eliminates SQLITE_BUSY errors.
	db.SetMaxOpenConns(1)

	// Enable WAL mode for better concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("enable WAL mode: %w", err)
	}

	// Wait up to 5 seconds if the database is busy instead of failing immediately.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
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

	if version < 3 {
		if err := s.migrateV3(); err != nil {
			return err
		}
	}

	if version < 4 {
		if err := s.migrateV4(); err != nil {
			return err
		}
	}

	if version < 5 {
		if err := s.migrateV5(); err != nil {
			return err
		}
	}

	if version < 6 {
		if err := s.migrateV6(); err != nil {
			return err
		}
	}

	if version < 7 {
		if err := s.migrateV7(); err != nil {
			return err
		}
	}

	if version < 8 {
		if err := s.migrateV8(); err != nil {
			return err
		}
	}

	if version < 9 {
		if err := s.migrateV9(); err != nil {
			return err
		}
	}

	if version < 10 {
		if err := s.migrateV10(); err != nil {
			return err
		}
	}

	if version < 11 {
		if err := s.migrateV11(); err != nil {
			return err
		}
	}

	if version < 12 {
		if err := s.migrateV12(); err != nil {
			return err
		}
	}

	if version < 13 {
		if err := s.migrateV13(); err != nil {
			return err
		}
	}

	if version < 14 {
		if err := s.migrateV14(); err != nil {
			return err
		}
	}

	if version < 15 {
		if err := s.migrateV15(); err != nil {
			return err
		}
	}

	if version < 16 {
		if err := s.migrateV16(); err != nil {
			return err
		}
	}

	if version < 17 {
		if err := s.migrateV17(); err != nil {
			return err
		}
	}

	if version < 18 {
		if err := s.migrateV18(); err != nil {
			return err
		}
	}

	if version < 19 {
		if err := s.migrateV19(); err != nil {
			return err
		}
	}

	if version < 20 {
		if err := s.migrateV20(); err != nil {
			return err
		}
	}

	if version < 21 {
		if err := s.migrateV21(); err != nil {
			return err
		}
	}

	if version < 22 {
		if err := s.migrateV22(); err != nil {
			return fmt.Errorf("migrate v22: %w", err)
		}
	}

	if version < 23 {
		if err := s.migrateV23(); err != nil {
			return fmt.Errorf("migrate v23: %w", err)
		}
	}

	if version < 24 {
		if err := s.migrateV24(); err != nil {
			return fmt.Errorf("migrate v24: %w", err)
		}
	}

	if version < 25 {
		if err := s.migrateV25(); err != nil {
			return fmt.Errorf("migrate v25: %w", err)
		}
	}

	if version < 26 {
		if err := s.migrateV26(); err != nil {
			return fmt.Errorf("migrate v26: %w", err)
		}
	}

	if version < 27 {
		if err := s.migrateV27(); err != nil {
			return fmt.Errorf("migrate v27: %w", err)
		}
	}

	if version < 28 {
		if err := s.migrateV28(); err != nil {
			return fmt.Errorf("migrate v28: %w", err)
		}
	}

	if version < 29 {
		if err := s.migrateV29(); err != nil {
			return fmt.Errorf("migrate v29: %w", err)
		}
	}

	if version < 30 {
		if err := s.migrateV30(); err != nil {
			return fmt.Errorf("migrate v30: %w", err)
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

	sourcesJSON := "[]"
	if len(st.Sources) > 0 {
		b, err := json.Marshal(st.Sources)
		if err != nil {
			return fmt.Errorf("marshal sources: %w", err)
		}
		sourcesJSON = string(b)
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO stations
			(callsign, ssid, last_heard, lat, lon, altitude, speed, course, symbol_table, symbol_code, comment, source, sources)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.Callsign, st.SSID, st.LastHeard.UTC(),
		lat, lon, alt, spd, crs,
		symTable, symCode,
		st.Comment, st.Source, sourcesJSON,
	)
	if err != nil {
		return fmt.Errorf("save station: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadStations() ([]station.Station, error) {
	rows, err := s.db.Query(`
		SELECT callsign, ssid, last_heard, lat, lon, altitude, speed, course,
		       symbol_table, symbol_code, comment, source, sources
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
		var sourcesJSON string

		if err := rows.Scan(
			&st.Callsign, &st.SSID, &lastHeard,
			&lat, &lon, &alt, &spd, &crs,
			&symTable, &symCode,
			&comment, &source, &sourcesJSON,
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

		// A position saved at 0,0 is a no-fix beacon from before the tracker
		// rejected them; the station is real, its position is not.
		if lat.Valid && lon.Valid && !station.IsNullIsland(lat.Float64, lon.Float64) {
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

		st.Sources = []string{}
		if sourcesJSON != "" && sourcesJSON != "[]" {
			if err := json.Unmarshal([]byte(sourcesJSON), &st.Sources); err != nil {
				return nil, fmt.Errorf("unmarshal sources: %w", err)
			}
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
		INSERT INTO tracks (callsign, ssid, lat, lon, time, speed, course)
		VALUES (?, 0, ?, ?, ?, ?, ?)`,
		callsign, tp.Lat, tp.Lon, tp.Time.UTC(), tp.Speed, tp.Course,
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
		SELECT lat, lon, time, speed, course FROM (
			SELECT lat, lon, time, speed, course
			FROM tracks
			WHERE callsign = ?
			  -- Skip no-fix 0,0 beacons written before the tracker rejected
			  -- them (station.IsNullIsland; same 1e-6 tolerance). Filtered
			  -- here, before LIMIT, so the limit counts real points only.
			  AND NOT (ABS(lat) < 0.000001 AND ABS(lon) < 0.000001)
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

		if err := rows.Scan(&tp.Lat, &tp.Lon, &ts, &tp.Speed, &tp.Course); err != nil {
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
	var reportedAt, resolvedAt, expiresAt interface{}
	if a.ReportedAt != nil {
		reportedAt = a.ReportedAt.UTC()
	}
	if a.ResolvedAt != nil {
		resolvedAt = a.ResolvedAt.UTC()
	}
	if a.ExpiresAt != nil {
		expiresAt = a.ExpiresAt.UTC()
	}

	missionIDsJSON, err := json.Marshal(a.MissionIDs)
	if err != nil {
		return fmt.Errorf("marshal mission_ids: %w", err)
	}
	if a.MissionIDs == nil {
		missionIDsJSON = []byte("[]")
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO annotations
			(id, type, label, description, geometry, style, created_by, created_by_name,
			 created_at, updated_at, category, status, priority, operation_id, mission_ids,
			 resources, reported_by, reported_at, resolved_at, expires_at,
			 net_id, short_name, sort_order, batch_id, batch_label)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Type, a.Label, a.Description, a.Geometry, a.Style,
		a.CreatedBy, a.CreatedByName, a.CreatedAt.UTC(), a.UpdatedAt.UTC(),
		a.Category, a.Status, a.Priority, a.OperationID, string(missionIDsJSON),
		a.Resources, a.ReportedBy, reportedAt, resolvedAt, expiresAt,
		a.NetID, a.ShortName, a.SortOrder, a.BatchID, a.BatchLabel,
	)
	if err != nil {
		return fmt.Errorf("save annotation: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadAnnotations() ([]Annotation, error) {
	return s.loadAnnotationsQuery(`
		SELECT id, type, label, description, geometry, style,
		       created_by, created_by_name, created_at, updated_at,
		       category, status, priority, operation_id, mission_ids,
		       resources, reported_by, reported_at, resolved_at, expires_at,
		       net_id, short_name, sort_order, batch_id, batch_label
		FROM annotations
		ORDER BY created_at ASC`)
}

func (s *SQLiteStore) LoadAnnotationsFiltered(filter AnnotationFilter) ([]Annotation, error) {
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

	if filter.Category != "" {
		addFilter("category = ?", filter.Category)
	}
	if filter.Status != "" {
		addFilter("status = ?", filter.Status)
	}
	if filter.Priority != "" {
		addFilter("priority = ?", filter.Priority)
	}
	if filter.OperationID != "" {
		addFilter("operation_id = ?", filter.OperationID)
	}
	if !filter.IncludeExpired {
		addFilter("(expires_at IS NULL OR expires_at > ?)", time.Now().UTC())
	}
	if filter.NetID != "" {
		addFilter("net_id = ?", filter.NetID)
	}
	if filter.BatchID != "" {
		addFilter("batch_id = ?", filter.BatchID)
	}

	query := `SELECT id, type, label, description, geometry, style,
		       created_by, created_by_name, created_at, updated_at,
		       category, status, priority, operation_id, mission_ids,
		       resources, reported_by, reported_at, resolved_at, expires_at,
		       net_id, short_name, sort_order, batch_id, batch_label
		FROM annotations` + where + ` ORDER BY created_at ASC`

	return s.loadAnnotationsQueryArgs(query, args...)
}

func (s *SQLiteStore) loadAnnotationsQuery(query string) ([]Annotation, error) {
	return s.loadAnnotationsQueryArgs(query)
}

func (s *SQLiteStore) loadAnnotationsQueryArgs(query string, args ...interface{}) ([]Annotation, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()

	var annotations []Annotation
	for rows.Next() {
		var a Annotation
		var createdAt, updatedAt string
		var description, style, createdBy, createdByName sql.NullString
		var category, status, priority, operationID sql.NullString
		var missionIDsJSON string
		var resources, reportedBy sql.NullString
		var reportedAt, resolvedAt, expiresAt sql.NullString
		var netID, shortName sql.NullString

		if err := rows.Scan(
			&a.ID, &a.Type, &a.Label, &description,
			&a.Geometry, &style, &createdBy, &createdByName,
			&createdAt, &updatedAt,
			&category, &status, &priority, &operationID, &missionIDsJSON,
			&resources, &reportedBy, &reportedAt, &resolvedAt, &expiresAt,
			&netID, &shortName, &a.SortOrder, &a.BatchID, &a.BatchLabel,
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
		if category.Valid {
			a.Category = category.String
		}
		if status.Valid {
			a.Status = status.String
		}
		if priority.Valid {
			a.Priority = priority.String
		}
		if operationID.Valid {
			a.OperationID = operationID.String
		}
		if missionIDsJSON != "" && missionIDsJSON != "[]" {
			if err := json.Unmarshal([]byte(missionIDsJSON), &a.MissionIDs); err != nil {
				return nil, fmt.Errorf("unmarshal annotation mission_ids: %w", err)
			}
		}
		if resources.Valid {
			a.Resources = resources.String
		}
		if reportedBy.Valid {
			a.ReportedBy = reportedBy.String
		}
		if reportedAt.Valid {
			t, err := parseTime(reportedAt.String)
			if err == nil {
				a.ReportedAt = &t
			}
		}
		if resolvedAt.Valid {
			t, err := parseTime(resolvedAt.String)
			if err == nil {
				a.ResolvedAt = &t
			}
		}
		if expiresAt.Valid {
			t, err := parseTime(expiresAt.String)
			if err == nil {
				a.ExpiresAt = &t
			}
		}
		if netID.Valid {
			a.NetID = netID.String
		}
		if shortName.Valid {
			a.ShortName = shortName.String
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

// UpdateAnnotationBatchLabel renames every annotation in a batch in a single
// UPDATE (SetMaxOpenConns(1) means N round-trips per member is not an option).
// Returns the number of rows affected.
func (s *SQLiteStore) UpdateAnnotationBatchLabel(batchID, label string, updatedAt time.Time) (int, error) {
	res, err := s.db.Exec(
		"UPDATE annotations SET batch_label = ?, updated_at = ? WHERE batch_id = ?",
		label, updatedAt.UTC(), batchID,
	)
	if err != nil {
		return 0, fmt.Errorf("update annotation batch label: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("update annotation batch label rows affected: %w", err)
	}
	return int(affected), nil
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

func (s *SQLiteStore) migrateV3() error {
	ddl := `
CREATE TABLE IF NOT EXISTS nets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'tactical',
    frequency TEXT NOT NULL DEFAULT '',
    ncs_callsign TEXT NOT NULL DEFAULT '',
    ncs_user_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',
    opened_at DATETIME,
    closed_at DATETIME,
    notes TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS net_check_ins (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL REFERENCES nets(id),
    callsign TEXT NOT NULL,
    tactical_call TEXT NOT NULL DEFAULT '',
    operator_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'available',
    traffic TEXT NOT NULL DEFAULT 'none',
    location TEXT NOT NULL DEFAULT '',
    lat REAL,
    lon REAL,
    assignment TEXT NOT NULL DEFAULT '',
    assignment_lat REAL,
    assignment_lon REAL,
    checked_in_at DATETIME NOT NULL,
    checked_out_at DATETIME,
    last_heard DATETIME NOT NULL,
    missed_roll_calls INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_net_checkins_net ON net_check_ins(net_id);

CREATE TABLE IF NOT EXISTS net_missions (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL REFERENCES nets(id),
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    priority TEXT NOT NULL DEFAULT 'routine',
    status TEXT NOT NULL DEFAULT 'open',
    assigned_to TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    completed_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_net_missions_net ON net_missions(net_id);

CREATE TABLE IF NOT EXISTS net_notes (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL REFERENCES nets(id),
    check_in_id TEXT,
    author_id TEXT NOT NULL DEFAULT '',
    author_name TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_net_notes_net ON net_notes(net_id);
CREATE INDEX IF NOT EXISTS idx_net_notes_checkin ON net_notes(check_in_id);

CREATE TABLE IF NOT EXISTS net_events (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL REFERENCES nets(id),
    type TEXT NOT NULL,
    callsign TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_net_events_net ON net_events(net_id);
CREATE INDEX IF NOT EXISTS idx_net_events_time ON net_events(created_at);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("create v3 tables: %w", err)
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 3); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

// --- Net Control CRUD ---

func (s *SQLiteStore) SaveNet(n Net) error {
	var openedAt, closedAt interface{}
	if n.OpenedAt != nil {
		openedAt = n.OpenedAt.UTC()
	}
	if n.ClosedAt != nil {
		closedAt = n.ClosedAt.UTC()
	}

	pinnedJSON := "[]"
	if len(n.PinnedStations) > 0 {
		b, err := json.Marshal(n.PinnedStations)
		if err != nil {
			return fmt.Errorf("marshal pinned_stations: %w", err)
		}
		pinnedJSON = string(b)
	}

	wxExtraZonesJSON := "[]"
	if len(n.WxExtraZones) > 0 {
		b, err := json.Marshal(n.WxExtraZones)
		if err != nil {
			return fmt.Errorf("marshal wx_extra_zones: %w", err)
		}
		wxExtraZonesJSON = string(b)
	}
	wxInterruptEventsJSON := "[]"
	if len(n.WxInterruptEvents) > 0 {
		b, err := json.Marshal(n.WxInterruptEvents)
		if err != nil {
			return fmt.Errorf("marshal wx_interrupt_events: %w", err)
		}
		wxInterruptEventsJSON = string(b)
	}

	profile := n.Profile
	if profile == "" {
		profile = "general" // netprofile.ProfileGeneral; store must not import netprofile
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO nets
			(id, name, type, frequency, ncs_callsign, ncs_user_id, status, opened_at, closed_at, notes, mission_brief, ops_view_lat, ops_view_lon, ops_view_zoom, pinned_stations,
			 wx_buffer_miles, wx_extra_zones, wx_mute_advisories, wx_interrupt_custom, wx_interrupt_events, profile)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Name, n.Type, n.Frequency, n.NCSCallsign, n.NCSUserID,
		n.Status, openedAt, closedAt, n.Notes, n.MissionBrief,
		n.OpsViewLat, n.OpsViewLon, n.OpsViewZoom, pinnedJSON,
		n.WxBufferMiles, wxExtraZonesJSON, n.WxMuteAdvisories, n.WxInterruptCustom, wxInterruptEventsJSON,
		profile,
	)
	if err != nil {
		return fmt.Errorf("save net: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadNet(id string) (*Net, error) {
	var n Net
	var openedAt, closedAt sql.NullString
	var opsLat, opsLon, opsZoom sql.NullFloat64
	var pinnedJSON, wxExtraZonesJSON, wxInterruptEventsJSON string

	err := s.db.QueryRow(`
		SELECT id, name, type, frequency, ncs_callsign, ncs_user_id,
		       status, opened_at, closed_at, notes, mission_brief,
		       ops_view_lat, ops_view_lon, ops_view_zoom, pinned_stations,
		       wx_buffer_miles, wx_extra_zones, wx_mute_advisories, wx_interrupt_custom, wx_interrupt_events,
		       profile
		FROM nets WHERE id = ?`, id).Scan(
		&n.ID, &n.Name, &n.Type, &n.Frequency, &n.NCSCallsign, &n.NCSUserID,
		&n.Status, &openedAt, &closedAt, &n.Notes, &n.MissionBrief,
		&opsLat, &opsLon, &opsZoom, &pinnedJSON,
		&n.WxBufferMiles, &wxExtraZonesJSON, &n.WxMuteAdvisories, &n.WxInterruptCustom, &wxInterruptEventsJSON,
		&n.Profile,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("net %q not found", id)
		}
		return nil, fmt.Errorf("load net: %w", err)
	}
	if n.Profile == "" {
		n.Profile = "general" // netprofile.ProfileGeneral; store must not import netprofile
	}

	if openedAt.Valid {
		t, err := parseTime(openedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse opened_at: %w", err)
		}
		n.OpenedAt = &t
	}
	if closedAt.Valid {
		t, err := parseTime(closedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse closed_at: %w", err)
		}
		n.ClosedAt = &t
	}
	if opsLat.Valid {
		n.OpsViewLat = &opsLat.Float64
	}
	if opsLon.Valid {
		n.OpsViewLon = &opsLon.Float64
	}
	if opsZoom.Valid {
		n.OpsViewZoom = &opsZoom.Float64
	}

	n.PinnedStations = []string{}
	if pinnedJSON != "" && pinnedJSON != "[]" {
		json.Unmarshal([]byte(pinnedJSON), &n.PinnedStations)
	}
	n.WxExtraZones = []string{}
	if wxExtraZonesJSON != "" && wxExtraZonesJSON != "[]" {
		json.Unmarshal([]byte(wxExtraZonesJSON), &n.WxExtraZones)
	}
	n.WxInterruptEvents = []string{}
	if wxInterruptEventsJSON != "" && wxInterruptEventsJSON != "[]" {
		json.Unmarshal([]byte(wxInterruptEventsJSON), &n.WxInterruptEvents)
	}

	return &n, nil
}

func (s *SQLiteStore) LoadNets() ([]Net, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, frequency, ncs_callsign, ncs_user_id,
		       status, opened_at, closed_at, notes, mission_brief,
		       ops_view_lat, ops_view_lon, ops_view_zoom, pinned_stations,
		       wx_buffer_miles, wx_extra_zones, wx_mute_advisories, wx_interrupt_custom, wx_interrupt_events,
		       profile
		FROM nets ORDER BY rowid ASC`)
	if err != nil {
		return nil, fmt.Errorf("query nets: %w", err)
	}
	defer rows.Close()

	var nets []Net
	for rows.Next() {
		var n Net
		var openedAt, closedAt sql.NullString
		var opsLat, opsLon, opsZoom sql.NullFloat64
		var pinnedJSON, wxExtraZonesJSON, wxInterruptEventsJSON string

		if err := rows.Scan(
			&n.ID, &n.Name, &n.Type, &n.Frequency, &n.NCSCallsign, &n.NCSUserID,
			&n.Status, &openedAt, &closedAt, &n.Notes, &n.MissionBrief,
			&opsLat, &opsLon, &opsZoom, &pinnedJSON,
			&n.WxBufferMiles, &wxExtraZonesJSON, &n.WxMuteAdvisories, &n.WxInterruptCustom, &wxInterruptEventsJSON,
			&n.Profile,
		); err != nil {
			return nil, fmt.Errorf("scan net: %w", err)
		}
		if n.Profile == "" {
			n.Profile = "general" // netprofile.ProfileGeneral; store must not import netprofile
		}

		if openedAt.Valid {
			t, err := parseTime(openedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse opened_at: %w", err)
			}
			n.OpenedAt = &t
		}
		if closedAt.Valid {
			t, err := parseTime(closedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse closed_at: %w", err)
			}
			n.ClosedAt = &t
		}
		if opsLat.Valid {
			n.OpsViewLat = &opsLat.Float64
		}
		if opsLon.Valid {
			n.OpsViewLon = &opsLon.Float64
		}
		if opsZoom.Valid {
			n.OpsViewZoom = &opsZoom.Float64
		}

		n.PinnedStations = []string{}
		if pinnedJSON != "" && pinnedJSON != "[]" {
			json.Unmarshal([]byte(pinnedJSON), &n.PinnedStations)
		}
		n.WxExtraZones = []string{}
		if wxExtraZonesJSON != "" && wxExtraZonesJSON != "[]" {
			json.Unmarshal([]byte(wxExtraZonesJSON), &n.WxExtraZones)
		}
		n.WxInterruptEvents = []string{}
		if wxInterruptEventsJSON != "" && wxInterruptEventsJSON != "[]" {
			json.Unmarshal([]byte(wxInterruptEventsJSON), &n.WxInterruptEvents)
		}

		nets = append(nets, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nets: %w", err)
	}
	return nets, nil
}

func (s *SQLiteStore) DeleteNet(id string) error {
	_, err := s.db.Exec("DELETE FROM nets WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete net: %w", err)
	}
	if _, err := s.db.Exec("DELETE FROM net_ride_configs WHERE net_id = ?", id); err != nil {
		return fmt.Errorf("delete net ride config: %w", err)
	}
	if _, err := s.db.Exec("DELETE FROM sag_requests WHERE net_id = ?", id); err != nil {
		return fmt.Errorf("delete sag requests: %w", err)
	}
	if _, err := s.db.Exec("DELETE FROM sag_vehicles WHERE net_id = ?", id); err != nil {
		return fmt.Errorf("delete sag vehicles: %w", err)
	}
	if err := s.DeleteRideTraffic(id); err != nil {
		return err
	}
	return nil
}

// --- Net Ride Config CRUD ---

func (s *SQLiteStore) SaveNetRideConfig(c NetRideConfig) error {
	routesJSON := "[]"
	if len(c.Routes) > 0 {
		b, err := json.Marshal(c.Routes)
		if err != nil {
			return fmt.Errorf("marshal routes: %w", err)
		}
		routesJSON = string(b)
	}

	cutoffJSON := "{}"
	{
		b, err := json.Marshal(c.Cutoff)
		if err != nil {
			return fmt.Errorf("marshal cutoff: %w", err)
		}
		cutoffJSON = string(b)
	}

	tiersJSON := "[]"
	if len(c.PriorityTiers) > 0 {
		b, err := json.Marshal(c.PriorityTiers)
		if err != nil {
			return fmt.Errorf("marshal priority_tiers: %w", err)
		}
		tiersJSON = string(b)
	}

	updatedAt := c.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO net_ride_configs
			(net_id, agency_name, event_name, event_date, routes, cutoff, withhold_bib_on_severe_injury, priority_tiers, division, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.NetID, c.AgencyName, c.EventName, c.EventDate, routesJSON, cutoffJSON,
		c.WithholdBibOnSevereInjury, tiersJSON, c.Division, updatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save net ride config: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadNetRideConfig(netID string) (*NetRideConfig, bool, error) {
	var c NetRideConfig
	var routesJSON, cutoffJSON, tiersJSON, updatedAt string

	err := s.db.QueryRow(`
		SELECT net_id, agency_name, event_name, event_date, routes, cutoff, withhold_bib_on_severe_injury, priority_tiers, division, updated_at
		FROM net_ride_configs WHERE net_id = ?`, netID).Scan(
		&c.NetID, &c.AgencyName, &c.EventName, &c.EventDate, &routesJSON, &cutoffJSON,
		&c.WithholdBibOnSevereInjury, &tiersJSON, &c.Division, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load net ride config: %w", err)
	}

	c.Routes = []RideRoute{}
	if routesJSON != "" && routesJSON != "[]" {
		if err := json.Unmarshal([]byte(routesJSON), &c.Routes); err != nil {
			return nil, false, fmt.Errorf("unmarshal routes: %w", err)
		}
	}
	if c.Routes == nil {
		c.Routes = []RideRoute{}
	}

	if cutoffJSON != "" && cutoffJSON != "{}" {
		if err := json.Unmarshal([]byte(cutoffJSON), &c.Cutoff); err != nil {
			return nil, false, fmt.Errorf("unmarshal cutoff: %w", err)
		}
	}

	c.PriorityTiers = []PriorityTier{}
	if tiersJSON != "" && tiersJSON != "[]" {
		if err := json.Unmarshal([]byte(tiersJSON), &c.PriorityTiers); err != nil {
			return nil, false, fmt.Errorf("unmarshal priority_tiers: %w", err)
		}
	}
	if c.PriorityTiers == nil {
		c.PriorityTiers = []PriorityTier{}
	}
	for i := range c.PriorityTiers {
		if c.PriorityTiers[i].Examples == nil {
			c.PriorityTiers[i].Examples = []string{}
		}
	}

	if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, false, fmt.Errorf("parse updated_at: %w", err)
	}

	return &c, true, nil
}

func (s *SQLiteStore) LoadNetRideConfigs() ([]NetRideConfig, error) {
	rows, err := s.db.Query(`
		SELECT net_id, agency_name, event_name, event_date, routes, cutoff, withhold_bib_on_severe_injury, priority_tiers, division, updated_at
		FROM net_ride_configs ORDER BY net_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query net ride configs: %w", err)
	}
	defer rows.Close()

	configs := []NetRideConfig{}
	for rows.Next() {
		var c NetRideConfig
		var routesJSON, cutoffJSON, tiersJSON, updatedAt string

		if err := rows.Scan(
			&c.NetID, &c.AgencyName, &c.EventName, &c.EventDate, &routesJSON, &cutoffJSON,
			&c.WithholdBibOnSevereInjury, &tiersJSON, &c.Division, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan net ride config: %w", err)
		}

		c.Routes = []RideRoute{}
		if routesJSON != "" && routesJSON != "[]" {
			if err := json.Unmarshal([]byte(routesJSON), &c.Routes); err != nil {
				return nil, fmt.Errorf("unmarshal routes: %w", err)
			}
		}
		if c.Routes == nil {
			c.Routes = []RideRoute{}
		}

		if cutoffJSON != "" && cutoffJSON != "{}" {
			if err := json.Unmarshal([]byte(cutoffJSON), &c.Cutoff); err != nil {
				return nil, fmt.Errorf("unmarshal cutoff: %w", err)
			}
		}

		c.PriorityTiers = []PriorityTier{}
		if tiersJSON != "" && tiersJSON != "[]" {
			if err := json.Unmarshal([]byte(tiersJSON), &c.PriorityTiers); err != nil {
				return nil, fmt.Errorf("unmarshal priority_tiers: %w", err)
			}
		}
		if c.PriorityTiers == nil {
			c.PriorityTiers = []PriorityTier{}
		}
		for i := range c.PriorityTiers {
			if c.PriorityTiers[i].Examples == nil {
				c.PriorityTiers[i].Examples = []string{}
			}
		}

		if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}

		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate net ride configs: %w", err)
	}
	return configs, nil
}

func (s *SQLiteStore) DeleteNetRideConfig(netID string) error {
	_, err := s.db.Exec("DELETE FROM net_ride_configs WHERE net_id = ?", netID)
	if err != nil {
		return fmt.Errorf("delete net ride config: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveNetCheckIn(ci NetCheckIn) error {
	var lat, lon sql.NullFloat64
	var checkedOutAt interface{}

	if ci.Lat != nil {
		lat = sql.NullFloat64{Float64: *ci.Lat, Valid: true}
	}
	if ci.Lon != nil {
		lon = sql.NullFloat64{Float64: *ci.Lon, Valid: true}
	}
	if ci.CheckedOutAt != nil {
		checkedOutAt = ci.CheckedOutAt.UTC()
	}

	source := ci.Source
	if source == "" {
		source = "voice"
	}

	trackedJSON := "[]"
	if len(ci.TrackedStations) > 0 {
		b, err := json.Marshal(ci.TrackedStations)
		if err != nil {
			return fmt.Errorf("marshal tracked stations: %w", err)
		}
		trackedJSON = string(b)
	}

	missionIDsJSON := "[]"
	if len(ci.MissionIDs) > 0 {
		b, err := json.Marshal(ci.MissionIDs)
		if err != nil {
			return fmt.Errorf("marshal mission ids: %w", err)
		}
		missionIDsJSON = string(b)
	}

	category := ci.Category
	if category == "" {
		category = "general"
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO net_check_ins
			(id, net_id, callsign, tactical_call, operator_name, status, traffic,
			 source, category, location, lat, lon,
			 mission_ids, tracked_stations, checked_in_at, checked_out_at, last_heard, missed_roll_calls)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ci.ID, ci.NetID, ci.Callsign, ci.TacticalCall, ci.OperatorName,
		ci.Status, ci.Traffic, source, category, ci.Location,
		lat, lon,
		missionIDsJSON, trackedJSON, ci.CheckedInAt.UTC(), checkedOutAt, ci.LastHeard.UTC(), ci.MissedRollCalls,
	)
	if err != nil {
		return fmt.Errorf("save net check-in: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadNetCheckIns(netID string) ([]NetCheckIn, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, callsign, tactical_call, operator_name, status, traffic,
		       source, category, location, lat, lon,
		       mission_ids, tracked_stations, checked_in_at, checked_out_at, last_heard, missed_roll_calls
		FROM net_check_ins WHERE net_id = ? ORDER BY checked_in_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query net check-ins: %w", err)
	}
	defer rows.Close()

	var checkIns []NetCheckIn
	for rows.Next() {
		var ci NetCheckIn
		var lat, lon sql.NullFloat64
		var checkedInAt, lastHeard string
		var checkedOutAt sql.NullString
		var trackedJSON, missionIDsJSON string

		if err := rows.Scan(
			&ci.ID, &ci.NetID, &ci.Callsign, &ci.TacticalCall, &ci.OperatorName,
			&ci.Status, &ci.Traffic, &ci.Source, &ci.Category, &ci.Location,
			&lat, &lon,
			&missionIDsJSON, &trackedJSON, &checkedInAt, &checkedOutAt, &lastHeard, &ci.MissedRollCalls,
		); err != nil {
			return nil, fmt.Errorf("scan net check-in: %w", err)
		}

		// Unmarshal mission IDs JSON.
		ci.MissionIDs = []string{}
		if missionIDsJSON != "" {
			json.Unmarshal([]byte(missionIDsJSON), &ci.MissionIDs)
		}

		// Unmarshal tracked stations JSON.
		ci.TrackedStations = []TrackedStation{}
		if trackedJSON != "" {
			json.Unmarshal([]byte(trackedJSON), &ci.TrackedStations)
		}

		ci.CheckedInAt, err = parseTime(checkedInAt)
		if err != nil {
			return nil, fmt.Errorf("parse checked_in_at: %w", err)
		}
		ci.LastHeard, err = parseTime(lastHeard)
		if err != nil {
			return nil, fmt.Errorf("parse last_heard: %w", err)
		}
		if checkedOutAt.Valid {
			t, err := parseTime(checkedOutAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse checked_out_at: %w", err)
			}
			ci.CheckedOutAt = &t
		}

		if lat.Valid {
			ci.Lat = &lat.Float64
		}
		if lon.Valid {
			ci.Lon = &lon.Float64
		}

		checkIns = append(checkIns, ci)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate net check-ins: %w", err)
	}
	return checkIns, nil
}

func (s *SQLiteStore) DeleteNetCheckIn(id string) error {
	_, err := s.db.Exec("DELETE FROM net_check_ins WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete net check-in: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveNetMission(m NetMission) error {
	var completedAt interface{}
	if m.CompletedAt != nil {
		completedAt = m.CompletedAt.UTC()
	}

	var lat, lon sql.NullFloat64
	if m.Lat != nil {
		lat = sql.NullFloat64{Float64: *m.Lat, Valid: true}
	}
	if m.Lon != nil {
		lon = sql.NullFloat64{Float64: *m.Lon, Valid: true}
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO net_missions
			(id, net_id, title, description, priority, status, assigned_to, location, lat, lon, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.NetID, m.Title, m.Description, m.Priority,
		m.Status, "", m.Location, lat, lon, m.CreatedAt.UTC(), completedAt,
	)
	if err != nil {
		return fmt.Errorf("save net mission: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadNetMissions(netID string) ([]NetMission, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, title, description, priority, status,
		       location, lat, lon, created_at, completed_at
		FROM net_missions WHERE net_id = ? ORDER BY created_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query net missions: %w", err)
	}
	defer rows.Close()

	var missions []NetMission
	for rows.Next() {
		var m NetMission
		var createdAt string
		var completedAt sql.NullString
		var lat, lon sql.NullFloat64

		if err := rows.Scan(
			&m.ID, &m.NetID, &m.Title, &m.Description, &m.Priority,
			&m.Status, &m.Location, &lat, &lon, &createdAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("scan net mission: %w", err)
		}
		if lat.Valid {
			m.Lat = &lat.Float64
		}
		if lon.Valid {
			m.Lon = &lon.Float64
		}

		m.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if completedAt.Valid {
			t, err := parseTime(completedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse completed_at: %w", err)
			}
			m.CompletedAt = &t
		}

		missions = append(missions, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate net missions: %w", err)
	}
	return missions, nil
}

func (s *SQLiteStore) SaveNetNote(n NetNote) error {
	var checkInID interface{}
	if n.CheckInID != "" {
		checkInID = n.CheckInID
	}

	var missionID interface{}
	if n.MissionID != "" {
		missionID = n.MissionID
	}

	pinned := 0
	if n.Pinned {
		pinned = 1
	}

	category := n.Category
	if category == "" {
		category = "general"
	}

	severity := n.Severity
	if severity == "" {
		severity = "info"
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO net_notes
			(id, net_id, check_in_id, mission_id, author_id, author_name, content,
			 category, severity, pinned, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.NetID, checkInID, missionID, n.AuthorID, n.AuthorName,
		n.Content, category, severity, pinned, n.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save net note: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadNetNotes(netID string) ([]NetNote, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, check_in_id, mission_id, author_id, author_name, content,
		       category, severity, pinned, created_at
		FROM net_notes WHERE net_id = ? ORDER BY created_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query net notes: %w", err)
	}
	defer rows.Close()

	var notes []NetNote
	for rows.Next() {
		var n NetNote
		var createdAt string
		var checkInID, missionID sql.NullString
		var pinned int

		if err := rows.Scan(
			&n.ID, &n.NetID, &checkInID, &missionID, &n.AuthorID, &n.AuthorName,
			&n.Content, &n.Category, &n.Severity, &pinned, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan net note: %w", err)
		}

		n.Pinned = pinned != 0
		n.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if checkInID.Valid {
			n.CheckInID = checkInID.String
		}
		if missionID.Valid {
			n.MissionID = missionID.String
		}

		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate net notes: %w", err)
	}
	return notes, nil
}

func (s *SQLiteStore) UpdateNotePinned(noteID string, pinned bool) error {
	val := 0
	if pinned {
		val = 1
	}
	_, err := s.db.Exec(`UPDATE net_notes SET pinned = ? WHERE id = ?`, val, noteID)
	if err != nil {
		return fmt.Errorf("update note pinned: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveNetEvent(e NetEvent) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO net_events
			(id, net_id, type, callsign, summary, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.NetID, e.Type, e.Callsign, e.Summary,
		e.Details, e.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save net event: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadNetEvents(netID string) ([]NetEvent, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, type, callsign, summary, details, created_at
		FROM net_events WHERE net_id = ? ORDER BY created_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query net events: %w", err)
	}
	defer rows.Close()

	var events []NetEvent
	for rows.Next() {
		var e NetEvent
		var createdAt string

		if err := rows.Scan(
			&e.ID, &e.NetID, &e.Type, &e.Callsign, &e.Summary,
			&e.Details, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan net event: %w", err)
		}

		e.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}

		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate net events: %w", err)
	}
	return events, nil
}

func (s *SQLiteStore) migrateV4() error {
	for _, stmt := range []string{
		"ALTER TABLE net_check_ins ADD COLUMN source TEXT NOT NULL DEFAULT 'voice'",
		"ALTER TABLE net_check_ins ADD COLUMN mission_id TEXT DEFAULT NULL",
		"ALTER TABLE net_missions ADD COLUMN location TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE net_missions ADD COLUMN lat REAL",
		"ALTER TABLE net_missions ADD COLUMN lon REAL",
		"ALTER TABLE net_notes ADD COLUMN mission_id TEXT DEFAULT NULL",
		"ALTER TABLE nets ADD COLUMN mission_brief TEXT NOT NULL DEFAULT ''",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v4: %w", err)
			}
		}
	}

	// Create indexes.
	for _, idx := range []string{
		"CREATE INDEX IF NOT EXISTS idx_net_checkins_mission ON net_check_ins(mission_id)",
		"CREATE INDEX IF NOT EXISTS idx_net_notes_mission ON net_notes(mission_id)",
	} {
		if _, err := s.db.Exec(idx); err != nil {
			return fmt.Errorf("migrate v4 index: %w", err)
		}
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 4); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV5() error {
	for _, stmt := range []string{
		"ALTER TABLE net_check_ins ADD COLUMN tracked_stations TEXT NOT NULL DEFAULT '[]'",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v5: %w", err)
			}
		}
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 5); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV6() error {
	ddl := `
CREATE TABLE IF NOT EXISTS tactical_aliases (
    callsign TEXT PRIMARY KEY,
    alias TEXT NOT NULL,
    assigned_by TEXT NOT NULL DEFAULT 'ui',
    updated_at DATETIME NOT NULL
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("create v6 tables: %w", err)
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 6); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV7() error {
	for _, stmt := range []string{
		"ALTER TABLE annotations ADD COLUMN category TEXT NOT NULL DEFAULT 'general'",
		"ALTER TABLE annotations ADD COLUMN status TEXT NOT NULL DEFAULT 'active'",
		"ALTER TABLE annotations ADD COLUMN priority TEXT NOT NULL DEFAULT 'routine'",
		"ALTER TABLE annotations ADD COLUMN operation_id TEXT DEFAULT ''",
		"ALTER TABLE annotations ADD COLUMN mission_id TEXT DEFAULT ''",
		"ALTER TABLE annotations ADD COLUMN resources TEXT DEFAULT '[]'",
		"ALTER TABLE annotations ADD COLUMN reported_by TEXT DEFAULT ''",
		"ALTER TABLE annotations ADD COLUMN reported_at DATETIME",
		"ALTER TABLE annotations ADD COLUMN resolved_at DATETIME",
		"ALTER TABLE annotations ADD COLUMN expires_at DATETIME",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v7: %w", err)
			}
		}
	}

	// Create indexes.
	for _, idx := range []string{
		"CREATE INDEX IF NOT EXISTS idx_annotations_category ON annotations(category)",
		"CREATE INDEX IF NOT EXISTS idx_annotations_status ON annotations(status)",
		"CREATE INDEX IF NOT EXISTS idx_annotations_operation ON annotations(operation_id)",
	} {
		if _, err := s.db.Exec(idx); err != nil {
			return fmt.Errorf("migrate v7 index: %w", err)
		}
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 7); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV8() error {
	ddl := `
CREATE TABLE IF NOT EXISTS operations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_by TEXT DEFAULT '',
    created_at DATETIME NOT NULL,
    archived_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_operations_status ON operations(status);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v8 create operations: %w", err)
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 8); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

// --- Operation CRUD ---

func (s *SQLiteStore) SaveOperation(op Operation) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO operations (id, name, description, status, created_by, created_at, archived_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		op.ID, op.Name, op.Description, op.Status, op.CreatedBy, op.CreatedAt.UTC(), nullTimePtr(op.ArchivedAt),
	)
	if err != nil {
		return fmt.Errorf("save operation: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadOperations() ([]Operation, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, status, created_by, created_at, archived_at
		FROM operations ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query operations: %w", err)
	}
	defer rows.Close()

	var ops []Operation
	for rows.Next() {
		var op Operation
		var createdAt string
		var archivedAt sql.NullString
		if err := rows.Scan(&op.ID, &op.Name, &op.Description, &op.Status, &op.CreatedBy, &createdAt, &archivedAt); err != nil {
			return nil, fmt.Errorf("scan operation: %w", err)
		}
		op.CreatedAt, _ = parseTime(createdAt)
		if archivedAt.Valid {
			t, _ := parseTime(archivedAt.String)
			op.ArchivedAt = &t
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *SQLiteStore) LoadOperation(id string) (*Operation, error) {
	var op Operation
	var createdAt string
	var archivedAt sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, description, status, created_by, created_at, archived_at
		FROM operations WHERE id = ?`, id).Scan(
		&op.ID, &op.Name, &op.Description, &op.Status, &op.CreatedBy, &createdAt, &archivedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load operation: %w", err)
	}
	op.CreatedAt, _ = parseTime(createdAt)
	if archivedAt.Valid {
		t, _ := parseTime(archivedAt.String)
		op.ArchivedAt = &t
	}
	return &op, nil
}

func (s *SQLiteStore) DeleteOperation(id string) error {
	_, err := s.db.Exec("DELETE FROM operations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete operation: %w", err)
	}
	return nil
}

func nullTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

// nullFloatPtr converts a nullable float column value the same explicit way
// nullTimePtr does for times, rather than relying on the driver's reflection
// dereference of a bare *float64.
func nullFloatPtr(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

// --- Tactical Alias CRUD ---

func (s *SQLiteStore) SaveTacticalAlias(a TacticalAlias) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO tactical_aliases (callsign, alias, assigned_by, updated_at)
		VALUES (?, ?, ?, ?)`,
		a.Callsign, a.Alias, a.AssignedBy, a.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save tactical alias: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadTacticalAliases() ([]TacticalAlias, error) {
	rows, err := s.db.Query(`
		SELECT callsign, alias, assigned_by, updated_at
		FROM tactical_aliases
		ORDER BY callsign ASC`)
	if err != nil {
		return nil, fmt.Errorf("query tactical aliases: %w", err)
	}
	defer rows.Close()

	var aliases []TacticalAlias
	for rows.Next() {
		var a TacticalAlias
		var updatedAt string

		if err := rows.Scan(&a.Callsign, &a.Alias, &a.AssignedBy, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan tactical alias: %w", err)
		}

		a.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse tactical alias updated_at %q: %w", updatedAt, err)
		}

		aliases = append(aliases, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tactical aliases: %w", err)
	}

	return aliases, nil
}

func (s *SQLiteStore) DeleteTacticalAlias(callsign string) error {
	_, err := s.db.Exec("DELETE FROM tactical_aliases WHERE callsign = ?", callsign)
	if err != nil {
		return fmt.Errorf("delete tactical alias: %w", err)
	}
	return nil
}

func (s *SQLiteStore) migrateV9() error {
	// Add mission_ids JSON columns to net_check_ins and annotations.
	for _, stmt := range []string{
		"ALTER TABLE net_check_ins ADD COLUMN mission_ids TEXT NOT NULL DEFAULT '[]'",
		"ALTER TABLE annotations ADD COLUMN mission_ids TEXT NOT NULL DEFAULT '[]'",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v9: %w", err)
			}
		}
	}

	// Migrate existing single mission_id data into mission_ids JSON array.
	for _, stmt := range []string{
		`UPDATE net_check_ins SET mission_ids = '["' || mission_id || '"]' WHERE mission_id IS NOT NULL AND mission_id != ''`,
		`UPDATE annotations SET mission_ids = '["' || mission_id || '"]' WHERE mission_id IS NOT NULL AND mission_id != ''`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v9 data: %w", err)
		}
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 9); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV10() error {
	for _, stmt := range []string{
		"ALTER TABLE net_notes ADD COLUMN category TEXT NOT NULL DEFAULT 'general'",
		"ALTER TABLE net_notes ADD COLUMN severity TEXT NOT NULL DEFAULT 'info'",
		"ALTER TABLE net_notes ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v10: %w", err)
			}
		}
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 10); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV11() error {
	for _, stmt := range []string{
		"ALTER TABLE nets ADD COLUMN ops_view_lat REAL",
		"ALTER TABLE nets ADD COLUMN ops_view_lon REAL",
		"ALTER TABLE nets ADD COLUMN ops_view_zoom REAL",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v11: %w", err)
			}
		}
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 11); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV12() error {
	ddl := `
CREATE TABLE IF NOT EXISTS weather_readings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    callsign TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    temperature REAL,
    wind_dir REAL,
    wind_speed REAL,
    wind_gust REAL,
    humidity INTEGER,
    pressure REAL,
    rain_1h REAL,
    rain_24h REAL,
    rain_today REAL,
    luminosity INTEGER
);
CREATE INDEX IF NOT EXISTS idx_weather_callsign_time ON weather_readings(callsign, timestamp);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v12 create weather_readings: %w", err)
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 12); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) SaveWeatherReading(r WeatherReading) error {
	_, err := s.db.Exec(`INSERT INTO weather_readings
		(callsign, timestamp, temperature, wind_dir, wind_speed, wind_gust, humidity, pressure, rain_1h, rain_24h, rain_today, luminosity)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Callsign, r.Timestamp.UTC(),
		r.Temperature, r.WindDir, r.WindSpeed, r.WindGust,
		r.Humidity, r.Pressure,
		r.Rain1h, r.Rain24h, r.RainToday, r.Luminosity,
	)
	return err
}

func (s *SQLiteStore) LoadWeatherReadings(filter WeatherFilter) ([]WeatherReading, error) {
	query := "SELECT id, callsign, timestamp, temperature, wind_dir, wind_speed, wind_gust, humidity, pressure, rain_1h, rain_24h, rain_today, luminosity FROM weather_readings WHERE 1=1"
	var args []any

	if filter.Callsign != "" {
		query += " AND callsign = ?"
		args = append(args, filter.Callsign)
	}
	if filter.Since != nil {
		query += " AND timestamp >= ?"
		args = append(args, filter.Since.UTC())
	}
	if filter.Until != nil {
		query += " AND timestamp <= ?"
		args = append(args, filter.Until.UTC())
	}
	query += " ORDER BY timestamp DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []WeatherReading
	for rows.Next() {
		var r WeatherReading
		var ts string
		if err := rows.Scan(&r.ID, &r.Callsign, &ts,
			&r.Temperature, &r.WindDir, &r.WindSpeed, &r.WindGust,
			&r.Humidity, &r.Pressure,
			&r.Rain1h, &r.Rain24h, &r.RainToday, &r.Luminosity,
		); err != nil {
			return nil, err
		}
		r.Timestamp, _ = time.Parse("2006-01-02 15:04:05-07:00", ts)
		if r.Timestamp.IsZero() {
			r.Timestamp, _ = time.Parse("2006-01-02T15:04:05Z", ts)
		}
		if r.Timestamp.IsZero() {
			r.Timestamp, _ = time.Parse(time.RFC3339, ts)
		}
		readings = append(readings, r)
	}
	return readings, rows.Err()
}

func (s *SQLiteStore) LoadWeatherStations() ([]WeatherReading, error) {
	query := `SELECT w.id, w.callsign, w.timestamp, w.temperature, w.wind_dir, w.wind_speed, w.wind_gust,
		w.humidity, w.pressure, w.rain_1h, w.rain_24h, w.rain_today, w.luminosity
		FROM weather_readings w
		INNER JOIN (
			SELECT callsign, MAX(timestamp) as max_ts FROM weather_readings GROUP BY callsign
		) latest ON w.callsign = latest.callsign AND w.timestamp = latest.max_ts
		ORDER BY w.callsign`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []WeatherReading
	for rows.Next() {
		var r WeatherReading
		var ts string
		if err := rows.Scan(&r.ID, &r.Callsign, &ts,
			&r.Temperature, &r.WindDir, &r.WindSpeed, &r.WindGust,
			&r.Humidity, &r.Pressure,
			&r.Rain1h, &r.Rain24h, &r.RainToday, &r.Luminosity,
		); err != nil {
			return nil, err
		}
		r.Timestamp, _ = time.Parse("2006-01-02 15:04:05-07:00", ts)
		if r.Timestamp.IsZero() {
			r.Timestamp, _ = time.Parse("2006-01-02T15:04:05Z", ts)
		}
		if r.Timestamp.IsZero() {
			r.Timestamp, _ = time.Parse(time.RFC3339, ts)
		}
		readings = append(readings, r)
	}
	return readings, rows.Err()
}

func (s *SQLiteStore) PurgeWeatherReadings(olderThan time.Time) (int64, error) {
	result, err := s.db.Exec("DELETE FROM weather_readings WHERE timestamp < ?", olderThan.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *SQLiteStore) migrateV13() error {
	ddl := `
CREATE TABLE IF NOT EXISTS telemetry_readings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    callsign TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    seq INTEGER NOT NULL DEFAULT 0,
    analog1 REAL NOT NULL DEFAULT 0,
    analog2 REAL NOT NULL DEFAULT 0,
    analog3 REAL NOT NULL DEFAULT 0,
    analog4 REAL NOT NULL DEFAULT 0,
    analog5 REAL NOT NULL DEFAULT 0,
    digital INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_telemetry_callsign_time ON telemetry_readings(callsign, timestamp);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v13 create telemetry_readings: %w", err)
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 13); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) SaveTelemetryReading(r TelemetryReading) error {
	_, err := s.db.Exec(`INSERT INTO telemetry_readings
		(callsign, timestamp, seq, analog1, analog2, analog3, analog4, analog5, digital)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Callsign, r.Timestamp.UTC(), r.Seq,
		r.Analog1, r.Analog2, r.Analog3, r.Analog4, r.Analog5,
		r.Digital,
	)
	return err
}

func (s *SQLiteStore) LoadTelemetryReadings(filter TelemetryFilter) ([]TelemetryReading, error) {
	query := "SELECT id, callsign, timestamp, seq, analog1, analog2, analog3, analog4, analog5, digital FROM telemetry_readings WHERE 1=1"
	var args []any

	if filter.Callsign != "" {
		query += " AND callsign = ?"
		args = append(args, filter.Callsign)
	}
	if filter.Since != nil {
		query += " AND timestamp >= ?"
		args = append(args, filter.Since.UTC())
	}
	if filter.Until != nil {
		query += " AND timestamp <= ?"
		args = append(args, filter.Until.UTC())
	}
	query += " ORDER BY timestamp DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []TelemetryReading
	for rows.Next() {
		var r TelemetryReading
		var ts string
		if err := rows.Scan(&r.ID, &r.Callsign, &ts, &r.Seq,
			&r.Analog1, &r.Analog2, &r.Analog3, &r.Analog4, &r.Analog5,
			&r.Digital,
		); err != nil {
			return nil, err
		}
		r.Timestamp, _ = parseTime(ts)
		readings = append(readings, r)
	}
	return readings, rows.Err()
}

func (s *SQLiteStore) LoadTelemetryStations() ([]TelemetryReading, error) {
	query := `SELECT t.id, t.callsign, t.timestamp, t.seq, t.analog1, t.analog2, t.analog3, t.analog4, t.analog5, t.digital
		FROM telemetry_readings t
		INNER JOIN (
			SELECT callsign, MAX(timestamp) as max_ts FROM telemetry_readings GROUP BY callsign
		) latest ON t.callsign = latest.callsign AND t.timestamp = latest.max_ts
		ORDER BY t.callsign`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []TelemetryReading
	for rows.Next() {
		var r TelemetryReading
		var ts string
		if err := rows.Scan(&r.ID, &r.Callsign, &ts, &r.Seq,
			&r.Analog1, &r.Analog2, &r.Analog3, &r.Analog4, &r.Analog5,
			&r.Digital,
		); err != nil {
			return nil, err
		}
		r.Timestamp, _ = parseTime(ts)
		readings = append(readings, r)
	}
	return readings, rows.Err()
}

func (s *SQLiteStore) PurgeTelemetryReadings(olderThan time.Time) (int64, error) {
	result, err := s.db.Exec("DELETE FROM telemetry_readings WHERE timestamp < ?", olderThan.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *SQLiteStore) migrateV14() error {
	for _, stmt := range []string{
		"ALTER TABLE net_check_ins ADD COLUMN category TEXT NOT NULL DEFAULT 'general'",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v14: %w", err)
			}
		}
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 14); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV15() error {
	ddl := `
CREATE TABLE IF NOT EXISTS location_presets (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL REFERENCES nets(id),
    name TEXT NOT NULL,
    short_name TEXT NOT NULL DEFAULT '',
    lat REAL NOT NULL,
    lon REAL NOT NULL,
    category TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_location_presets_net ON location_presets(net_id);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v15 create location_presets: %w", err)
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 15); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV16() error {
	// Add net-scoping columns to annotations.
	for _, stmt := range []string{
		"ALTER TABLE annotations ADD COLUMN net_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE annotations ADD COLUMN short_name TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE annotations ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v16 alter annotations: %w", err)
			}
		}
	}

	// Create index for net-scoped queries.
	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_annotations_net ON annotations(net_id)"); err != nil {
		return fmt.Errorf("migrate v16 index: %w", err)
	}

	// Migrate existing location_presets into annotations.
	// Check if location_presets table exists before migrating.
	var tableExists int
	s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='location_presets'").Scan(&tableExists)
	if tableExists > 0 {
		_, err := s.db.Exec(`
			INSERT INTO annotations (id, type, label, description, geometry, category, status, priority, net_id, short_name, sort_order, created_at, updated_at)
			SELECT id, 'point', name, description,
				   '{"type":"Point","coordinates":[' || lon || ',' || lat || ']}',
				   CASE
					   WHEN category IN ('checkpoint','hazard','general') THEN category
					   WHEN category = 'aid' THEN 'aid'
					   WHEN category = 'staging' THEN 'staging'
					   WHEN category = 'shelter' THEN 'shelter'
					   WHEN category = 'parking' THEN 'parking'
					   WHEN category = 'start' THEN 'start'
					   WHEN category = 'finish' THEN 'finish'
					   ELSE 'general'
				   END,
				   'active', 'routine', net_id, short_name, sort_order,
				   datetime('now'), datetime('now')
			FROM location_presets`)
		if err != nil {
			return fmt.Errorf("migrate v16 data: %w", err)
		}

		// Drop old table.
		if _, err := s.db.Exec("DROP TABLE IF EXISTS location_presets"); err != nil {
			return fmt.Errorf("migrate v16 drop: %w", err)
		}
	}

	// Update schema version.
	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 16); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV17() error {
	for _, stmt := range []string{
		"ALTER TABLE nets ADD COLUMN pinned_stations TEXT NOT NULL DEFAULT '[]'",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v17: %w", err)
			}
		}
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 17); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV18() error {
	for _, stmt := range []string{
		"ALTER TABLE tracks ADD COLUMN speed REAL NOT NULL DEFAULT 0",
		"ALTER TABLE tracks ADD COLUMN course REAL NOT NULL DEFAULT 0",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v18: %w", err)
			}
		}
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 18); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrateV19() error {
	ddl := `
CREATE TABLE IF NOT EXISTS checkpoint_meta (
    annotation_id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL,
    sequence_number INTEGER NOT NULL DEFAULT 0,
    expected_time DATETIME,
    opened_at DATETIME,
    closed_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_checkpoint_meta_net ON checkpoint_meta(net_id);

CREATE TABLE IF NOT EXISTS checkpoint_passages (
    id TEXT PRIMARY KEY,
    checkpoint_id TEXT NOT NULL,
    net_id TEXT NOT NULL,
    label TEXT NOT NULL,
    passage_time DATETIME NOT NULL,
    direction TEXT NOT NULL DEFAULT 'through',
    reported_by TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_checkpoint_passages_net ON checkpoint_passages(net_id);
CREATE INDEX IF NOT EXISTS idx_checkpoint_passages_cp ON checkpoint_passages(checkpoint_id);
CREATE INDEX IF NOT EXISTS idx_checkpoint_passages_time ON checkpoint_passages(passage_time);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v19 create checkpoint tables: %w", err)
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 19); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

// legacySourceIDRe matches old transport instance IDs like "aprsis-0", "kisstcp-1".
var legacySourceIDRe = regexp.MustCompile(`^(aprsis|kisstcp|serial)-[0-9]+$`)

// normalizeLegacySources converts a legacy stations.source value into a sorted,
// deduplicated list of transport display names suitable for the sources column.
// Instance IDs ("aprsis-0") collapse to their type prefix; "both" and empty
// tokens are dropped; custom names are kept verbatim.
func normalizeLegacySources(source string) []string {
	if source == "" {
		return nil
	}
	parts := strings.Split(source, "+")
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		tok := strings.TrimSpace(p)
		if tok == "" || tok == "both" {
			continue
		}
		if m := legacySourceIDRe.FindStringSubmatch(tok); m != nil {
			tok = m[1]
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	sort.Strings(out)
	return out
}

func (s *SQLiteStore) migrateV20() error {
	if _, err := s.db.Exec("ALTER TABLE stations ADD COLUMN sources TEXT NOT NULL DEFAULT '[]'"); err != nil {
		if !isDuplicateColumnError(err) {
			return fmt.Errorf("migrate v20 add sources column: %w", err)
		}
	}

	rows, err := s.db.Query(`SELECT callsign, ssid, source FROM stations WHERE source IS NOT NULL AND source != ''`)
	if err != nil {
		return fmt.Errorf("migrate v20 select stations: %w", err)
	}
	defer rows.Close()

	type stationSourceRow struct {
		callsign string
		ssid     int
		source   string
	}
	var toUpdate []stationSourceRow
	for rows.Next() {
		var r stationSourceRow
		var source sql.NullString
		if err := rows.Scan(&r.callsign, &r.ssid, &source); err != nil {
			return fmt.Errorf("migrate v20 scan station: %w", err)
		}
		if source.Valid {
			r.source = source.String
		}
		toUpdate = append(toUpdate, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate v20 iterate stations: %w", err)
	}
	rows.Close()

	for _, r := range toUpdate {
		tokens := normalizeLegacySources(r.source)
		sourcesJSON := "[]"
		if len(tokens) > 0 {
			b, err := json.Marshal(tokens)
			if err != nil {
				return fmt.Errorf("migrate v20 marshal sources for %s-%d: %w", r.callsign, r.ssid, err)
			}
			sourcesJSON = string(b)
		}
		newSource := strings.Join(tokens, "+")
		if _, err := s.db.Exec(
			`UPDATE stations SET sources = ?, source = ? WHERE callsign = ? AND ssid = ?`,
			sourcesJSON, newSource, r.callsign, r.ssid,
		); err != nil {
			return fmt.Errorf("migrate v20 update station %s-%d: %w", r.callsign, r.ssid, err)
		}
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 20); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

// --- Checkpoint Progress CRUD ---

func (s *SQLiteStore) SaveCheckpointMeta(m CheckpointMeta) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO checkpoint_meta
			(annotation_id, net_id, sequence_number, expected_time, opened_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.AnnotationID, m.NetID, m.SequenceNumber,
		nullTimePtr(m.ExpectedTime), nullTimePtr(m.OpenedAt), nullTimePtr(m.ClosedAt),
	)
	if err != nil {
		return fmt.Errorf("save checkpoint meta: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadCheckpointMeta(netID string) ([]CheckpointMeta, error) {
	rows, err := s.db.Query(`
		SELECT annotation_id, net_id, sequence_number, expected_time, opened_at, closed_at
		FROM checkpoint_meta WHERE net_id = ? ORDER BY sequence_number ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query checkpoint meta: %w", err)
	}
	defer rows.Close()

	var metas []CheckpointMeta
	for rows.Next() {
		var m CheckpointMeta
		var expectedTime, openedAt, closedAt sql.NullString

		if err := rows.Scan(&m.AnnotationID, &m.NetID, &m.SequenceNumber,
			&expectedTime, &openedAt, &closedAt); err != nil {
			return nil, fmt.Errorf("scan checkpoint meta: %w", err)
		}
		if expectedTime.Valid {
			t, err := parseTime(expectedTime.String)
			if err == nil {
				m.ExpectedTime = &t
			}
		}
		if openedAt.Valid {
			t, err := parseTime(openedAt.String)
			if err == nil {
				m.OpenedAt = &t
			}
		}
		if closedAt.Valid {
			t, err := parseTime(closedAt.String)
			if err == nil {
				m.ClosedAt = &t
			}
		}
		metas = append(metas, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkpoint meta: %w", err)
	}
	return metas, nil
}

func (s *SQLiteStore) DeleteCheckpointMeta(annotationID string) error {
	_, err := s.db.Exec("DELETE FROM checkpoint_meta WHERE annotation_id = ?", annotationID)
	if err != nil {
		return fmt.Errorf("delete checkpoint meta: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveCheckpointPassage(p CheckpointPassage) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO checkpoint_passages
			(id, checkpoint_id, net_id, label, passage_time, direction, reported_by, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.CheckpointID, p.NetID, p.Label,
		p.PassageTime.UTC(), p.Direction, p.ReportedBy, p.Notes,
	)
	if err != nil {
		return fmt.Errorf("save checkpoint passage: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadCheckpointPassages(netID string) ([]CheckpointPassage, error) {
	return s.loadPassagesQuery(`
		SELECT id, checkpoint_id, net_id, label, passage_time, direction, reported_by, notes
		FROM checkpoint_passages WHERE net_id = ? ORDER BY passage_time ASC`, netID)
}

func (s *SQLiteStore) LoadCheckpointPassagesForCheckpoint(checkpointID string) ([]CheckpointPassage, error) {
	return s.loadPassagesQuery(`
		SELECT id, checkpoint_id, net_id, label, passage_time, direction, reported_by, notes
		FROM checkpoint_passages WHERE checkpoint_id = ? ORDER BY passage_time ASC`, checkpointID)
}

func (s *SQLiteStore) loadPassagesQuery(query string, args ...interface{}) ([]CheckpointPassage, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query checkpoint passages: %w", err)
	}
	defer rows.Close()

	var passages []CheckpointPassage
	for rows.Next() {
		var p CheckpointPassage
		var passageTime string

		if err := rows.Scan(&p.ID, &p.CheckpointID, &p.NetID, &p.Label,
			&passageTime, &p.Direction, &p.ReportedBy, &p.Notes); err != nil {
			return nil, fmt.Errorf("scan checkpoint passage: %w", err)
		}
		p.PassageTime, err = parseTime(passageTime)
		if err != nil {
			return nil, fmt.Errorf("parse passage_time %q: %w", passageTime, err)
		}
		passages = append(passages, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkpoint passages: %w", err)
	}
	return passages, nil
}

func (s *SQLiteStore) DeleteCheckpointPassages(netID string) error {
	_, err := s.db.Exec("DELETE FROM checkpoint_passages WHERE net_id = ?", netID)
	if err != nil {
		return fmt.Errorf("delete checkpoint passages: %w", err)
	}
	return nil
}

func (s *SQLiteStore) migrateV21() error {
	ddl := `
CREATE TABLE IF NOT EXISTS conversation_reads (
    callsign TEXT PRIMARY KEY,
    last_read_at DATETIME NOT NULL
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v21 create conversation_reads: %w", err)
	}

	if err := s.backfillConversationReads(); err != nil {
		return err
	}

	if _, err := s.db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema_version: %w", err)
	}
	if _, err := s.db.Exec("INSERT INTO schema_version (version) VALUES (?)", 21); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}

// migrateV22 stamps annotations with a provenance batch id/label so a bulk
// import (or copy, or template apply) can be identified and removed as a
// group later. See #89.
//
// The annotations table has existed since v2, so a real database always has
// it by the time it reaches v22. The presence check below only matters for
// narrow migration-test fixtures (e.g. the v19/v20/v21 tests in
// sqlite_test.go) that hand-build just the tables their own migration
// touches and seed a schema_version past v2 — mirroring the same
// present-table guard backfillConversationReads uses for "messages".
func (s *SQLiteStore) migrateV22() error {
	var present int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='annotations'`,
	).Scan(&present); err != nil {
		return fmt.Errorf("migrate v22 check annotations table: %w", err)
	}
	if present == 1 {
		for _, stmt := range []string{
			`ALTER TABLE annotations ADD COLUMN batch_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE annotations ADD COLUMN batch_label TEXT NOT NULL DEFAULT ''`,
		} {
			if _, err := s.db.Exec(stmt); err != nil && !isDuplicateColumnError(err) {
				return fmt.Errorf("add annotation batch column: %w", err)
			}
		}
		if _, err := s.db.Exec(
			`CREATE INDEX IF NOT EXISTS idx_annotations_batch_id ON annotations(batch_id)`); err != nil {
			return fmt.Errorf("index annotations batch_id: %w", err)
		}
	}
	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 22); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// migrateV23 folds the vestigial net_missions.assigned_to callsign into the
// real assignment record, NetCheckIn.MissionIDs, then clears the column.
// assigned_to was written only by the mission-create form and read by nothing,
// so every non-empty value is an assignment the operator never actually got.
//
// The present-table guard mirrors migrateV22: narrow migration-test fixtures
// hand-build only the tables their own migration touches.
func (s *SQLiteStore) migrateV23() error {
	var missionsPresent, checkInsPresent int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='net_missions'`,
	).Scan(&missionsPresent); err != nil {
		return fmt.Errorf("migrate v23 check net_missions table: %w", err)
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='net_check_ins'`,
	).Scan(&checkInsPresent); err != nil {
		return fmt.Errorf("migrate v23 check net_check_ins table: %w", err)
	}

	if missionsPresent == 1 && checkInsPresent == 1 {
		rows, err := s.db.Query(
			`SELECT id, net_id, assigned_to FROM net_missions WHERE assigned_to != ''`)
		if err != nil {
			return fmt.Errorf("migrate v23 read missions: %w", err)
		}
		type pending struct{ missionID, netID, callsign string }
		var todo []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.missionID, &p.netID, &p.callsign); err != nil {
				rows.Close()
				return fmt.Errorf("migrate v23 scan mission: %w", err)
			}
			todo = append(todo, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("migrate v23 iterate missions: %w", err)
		}

		for _, p := range todo {
			var ciID, raw string
			err := s.db.QueryRow(
				`SELECT id, mission_ids FROM net_check_ins
				 WHERE net_id = ? AND UPPER(callsign) = UPPER(?) AND status != 'released'
				 ORDER BY checked_in_at DESC LIMIT 1`,
				p.netID, p.callsign).Scan(&ciID, &raw)
			if err == sql.ErrNoRows {
				// Orphaned callsign — no matching roster entry. Nothing to
				// carry forward; the column is cleared below either way.
				continue
			}
			if err != nil {
				return fmt.Errorf("migrate v23 find check-in: %w", err)
			}

			ids := []string{}
			if raw != "" {
				_ = json.Unmarshal([]byte(raw), &ids)
			}
			dup := false
			for _, id := range ids {
				if id == p.missionID {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			ids = append(ids, p.missionID)
			enc, err := json.Marshal(ids)
			if err != nil {
				return fmt.Errorf("migrate v23 encode mission_ids: %w", err)
			}
			if _, err := s.db.Exec(
				`UPDATE net_check_ins SET mission_ids = ?,
				   status = CASE WHEN status = 'available' THEN 'assigned' ELSE status END
				 WHERE id = ?`, string(enc), ciID); err != nil {
				return fmt.Errorf("migrate v23 update check-in: %w", err)
			}
		}

		if _, err := s.db.Exec(`UPDATE net_missions SET assigned_to = '' WHERE assigned_to != ''`); err != nil {
			return fmt.Errorf("migrate v23 clear assigned_to: %w", err)
		}
	}

	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 23); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// migrateV24 adds internal/wxalert's tables and per-net weather-watch
// columns. The present-table guard on `nets` mirrors migrateV22/V23: narrow
// migration-test fixtures hand-build only the tables their own migration
// touches.
func (s *SQLiteStore) migrateV24() error {
	var netsPresent int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nets'`,
	).Scan(&netsPresent); err != nil {
		return fmt.Errorf("migrate v24 check nets table: %w", err)
	}
	if netsPresent == 1 {
		for _, stmt := range []string{
			`ALTER TABLE nets ADD COLUMN wx_buffer_miles REAL NOT NULL DEFAULT 0`,
			`ALTER TABLE nets ADD COLUMN wx_extra_zones TEXT NOT NULL DEFAULT '[]'`,
			`ALTER TABLE nets ADD COLUMN wx_mute_advisories INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE nets ADD COLUMN wx_interrupt_custom INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE nets ADD COLUMN wx_interrupt_events TEXT NOT NULL DEFAULT '[]'`,
		} {
			if _, err := s.db.Exec(stmt); err != nil && !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate v24 alter nets: %w", err)
			}
		}
	}

	ddl := `
CREATE TABLE IF NOT EXISTS wx_alerts (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL DEFAULT '',
    event TEXT NOT NULL DEFAULT '',
    tier TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    proximity TEXT NOT NULL DEFAULT '',
    notify_class TEXT NOT NULL DEFAULT '',
    sent DATETIME,
    expires DATETIME,
    ends_at DATETIME,
    replaced_by TEXT NOT NULL DEFAULT '',
    net_ack_callsign TEXT NOT NULL DEFAULT '',
    net_ack_at DATETIME,
    fetched_at DATETIME,
    first_seen_at DATETIME,
    updated_at DATETIME NOT NULL,
    data TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_wx_alerts_state ON wx_alerts(state);
CREATE INDEX IF NOT EXISTS idx_wx_alerts_net ON wx_alerts(net_id);
CREATE INDEX IF NOT EXISTS idx_wx_alerts_updated ON wx_alerts(updated_at);

CREATE TABLE IF NOT EXISTS wx_point_zones (
    cell_lat INTEGER NOT NULL,
    cell_lon INTEGER NOT NULL,
    ugc TEXT NOT NULL DEFAULT '[]',
    expires_at DATETIME NOT NULL,
    PRIMARY KEY (cell_lat, cell_lon)
);

CREATE TABLE IF NOT EXISTS wx_alert_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v24 create wx tables: %w", err)
	}

	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 24); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// migrateV25 adds the net profile column and its per-net ride-event
// configuration table (internal/netprofile). The present-table guard on
// `nets` mirrors migrateV22/V23/V24: narrow migration-test fixtures hand-
// build only the tables their own migration touches.
//
// The DEFAULT 'general' on the ALTER is the compatibility guarantee: every
// pre-existing row reads back as general with no data migration. The literal
// "general" here (rather than netprofile.ProfileGeneral) is deliberate —
// store must not import netprofile (netprofile is the higher-level
// registry); TestProfileGeneralLiteralMatchesStore pins the two equal.
func (s *SQLiteStore) migrateV25() error {
	var netsPresent int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nets'`,
	).Scan(&netsPresent); err != nil {
		return fmt.Errorf("migrate v25 check nets table: %w", err)
	}
	if netsPresent == 1 {
		if _, err := s.db.Exec(`ALTER TABLE nets ADD COLUMN profile TEXT NOT NULL DEFAULT 'general'`); err != nil && !isDuplicateColumnError(err) {
			return fmt.Errorf("migrate v25 alter nets: %w", err)
		}
	}

	ddl := `
CREATE TABLE IF NOT EXISTS net_ride_configs (
    net_id                         TEXT PRIMARY KEY,
    agency_name                    TEXT NOT NULL DEFAULT '',
    event_name                     TEXT NOT NULL DEFAULT '',
    event_date                     TEXT NOT NULL DEFAULT '',
    routes                         TEXT NOT NULL DEFAULT '[]',
    cutoff                         TEXT NOT NULL DEFAULT '{}',
    withhold_bib_on_severe_injury  INTEGER NOT NULL DEFAULT 0,
    priority_tiers                 TEXT NOT NULL DEFAULT '[]',
    division                       TEXT NOT NULL DEFAULT '',
    updated_at                     DATETIME NOT NULL
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v25 create net_ride_configs: %w", err)
	}

	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 25); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// backfillConversationReads seeds an "everything so far is read" marker for
// every conversation that already has inbound messages.
//
// Before schema 21, unread meant "inbound and not yet acked" — and inbound
// messages are always stored acked, so the count was effectively always zero.
// Read markers change that to "inbound and newer than the marker", so without
// this backfill the first launch after upgrade would light up every message an
// operator has ever received, with no way to clear it but opening every thread.
//
// The marker is the newest inbound timestamp per conversation rather than
// "now", so it can never mark a message read that has not arrived yet. Max is
// computed in Go: the timestamp column holds several historical text formats
// and SQL MAX() would compare them lexicographically. Bulletins are skipped —
// they are excluded from conversations and never carry read state.
func (s *SQLiteStore) backfillConversationReads() error {
	// Databases created by older partial fixtures may not have the messages
	// table at all; there is then nothing to backfill.
	var present int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='messages'`,
	).Scan(&present); err != nil {
		return fmt.Errorf("migrate v21 check messages table: %w", err)
	}
	if present == 0 {
		return nil
	}

	rows, err := s.db.Query(`SELECT from_call, to_call, timestamp FROM messages WHERE inbound = 1`)
	if err != nil {
		return fmt.Errorf("migrate v21 read messages: %w", err)
	}
	defer rows.Close()

	newest := make(map[string]time.Time)
	for rows.Next() {
		var fromCall, toCall, ts string
		if err := rows.Scan(&fromCall, &toCall, &ts); err != nil {
			return fmt.Errorf("migrate v21 scan message: %w", err)
		}
		if strings.HasPrefix(toCall, "BLN") || strings.HasPrefix(toCall, "ANN") {
			continue
		}
		t, err := parseTime(ts)
		if err != nil {
			// An unparseable legacy row must not block the upgrade; skipping it
			// only means the marker lands slightly earlier.
			continue
		}
		if cur, ok := newest[fromCall]; !ok || t.After(cur) {
			newest[fromCall] = t
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate v21 iterate messages: %w", err)
	}

	for callsign, t := range newest {
		if _, err := s.db.Exec(
			`INSERT INTO conversation_reads (callsign, last_read_at) VALUES (?, ?)
			 ON CONFLICT(callsign) DO NOTHING`,
			callsign, t.UTC(),
		); err != nil {
			return fmt.Errorf("migrate v21 backfill %s: %w", callsign, err)
		}
	}

	return nil
}

// SaveConversationRead persists the read marker for a conversation.
//
// Monotonicity is enforced in the message engine (the single in-process
// authority, under mutex) rather than in SQL: RFC3339Nano omits trailing
// zeros, so a lexicographic comparison of the stored strings would be wrong.
func (s *SQLiteStore) SaveConversationRead(callsign string, lastReadAt time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO conversation_reads (callsign, last_read_at)
		VALUES (?, ?)
		ON CONFLICT(callsign) DO UPDATE SET last_read_at = excluded.last_read_at`,
		callsign, lastReadAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save conversation read: %w", err)
	}
	return nil
}

// LoadConversationReads returns all conversation read markers keyed by callsign.
func (s *SQLiteStore) LoadConversationReads() (map[string]time.Time, error) {
	rows, err := s.db.Query(`SELECT callsign, last_read_at FROM conversation_reads`)
	if err != nil {
		return nil, fmt.Errorf("query conversation reads: %w", err)
	}
	defer rows.Close()

	reads := make(map[string]time.Time)
	for rows.Next() {
		var callsign, ts string
		if err := rows.Scan(&callsign, &ts); err != nil {
			return nil, fmt.Errorf("scan conversation read: %w", err)
		}
		t, err := parseTime(ts)
		if err != nil {
			return nil, fmt.Errorf("parse conversation read time %q: %w", ts, err)
		}
		reads[callsign] = t
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation reads: %w", err)
	}

	return reads, nil
}

// --- NWS Weather Alerts CRUD ---

func (s *SQLiteStore) SaveWxAlert(a WxAlertRow) error {
	_, err := s.db.Exec(`
		INSERT INTO wx_alerts
			(id, net_id, event, tier, state, proximity, notify_class, sent, expires, ends_at,
			 replaced_by, net_ack_callsign, net_ack_at, fetched_at, first_seen_at, updated_at, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			net_id = excluded.net_id, event = excluded.event, tier = excluded.tier,
			state = excluded.state, proximity = excluded.proximity, notify_class = excluded.notify_class,
			sent = excluded.sent, expires = excluded.expires, ends_at = excluded.ends_at,
			replaced_by = excluded.replaced_by, net_ack_callsign = excluded.net_ack_callsign,
			net_ack_at = excluded.net_ack_at, fetched_at = excluded.fetched_at,
			first_seen_at = excluded.first_seen_at, updated_at = excluded.updated_at, data = excluded.data`,
		a.ID, a.NetID, a.Event, a.Tier, a.State, a.Proximity, a.NotifyClass,
		nullTimeVal(a.Sent), nullTimeVal(a.Expires), nullTimeVal(a.EndsAt),
		a.ReplacedBy, a.NetAckCallsign, nullTimePtr(a.NetAckAt),
		nullTimeVal(a.FetchedAt), nullTimeVal(a.FirstSeenAt), a.UpdatedAt.UTC(), a.Data,
	)
	if err != nil {
		return fmt.Errorf("save wx alert: %w", err)
	}
	return nil
}

// nullTimeVal is nullTimePtr's by-value counterpart: a zero time.Time (never
// set — e.g. an alert with no Ends before EndsAt is resolved) stores as SQL
// NULL rather than SQLite's "0001-01-01..." text, so LoadWxAlerts round-trips
// it back to a zero time.Time instead of a parse error.
func nullTimeVal(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

func (s *SQLiteStore) LoadWxAlerts(includeInactive bool) ([]WxAlertRow, error) {
	query := `
		SELECT id, net_id, event, tier, state, proximity, notify_class, sent, expires, ends_at,
		       replaced_by, net_ack_callsign, net_ack_at, fetched_at, first_seen_at, updated_at, data
		FROM wx_alerts`
	if !includeInactive {
		query += ` WHERE state = 'active'`
	}
	query += ` ORDER BY updated_at ASC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query wx alerts: %w", err)
	}
	defer rows.Close()

	var out []WxAlertRow
	for rows.Next() {
		var a WxAlertRow
		var sent, expires, endsAt, netAckAt, fetchedAt, firstSeenAt sql.NullString
		var updatedAt string
		if err := rows.Scan(
			&a.ID, &a.NetID, &a.Event, &a.Tier, &a.State, &a.Proximity, &a.NotifyClass,
			&sent, &expires, &endsAt, &a.ReplacedBy, &a.NetAckCallsign, &netAckAt,
			&fetchedAt, &firstSeenAt, &updatedAt, &a.Data,
		); err != nil {
			return nil, fmt.Errorf("scan wx alert: %w", err)
		}
		if a.Sent, err = parseNullTime(sent); err != nil {
			return nil, fmt.Errorf("parse wx alert sent: %w", err)
		}
		if a.Expires, err = parseNullTime(expires); err != nil {
			return nil, fmt.Errorf("parse wx alert expires: %w", err)
		}
		if a.EndsAt, err = parseNullTime(endsAt); err != nil {
			return nil, fmt.Errorf("parse wx alert ends_at: %w", err)
		}
		if a.FetchedAt, err = parseNullTime(fetchedAt); err != nil {
			return nil, fmt.Errorf("parse wx alert fetched_at: %w", err)
		}
		if a.FirstSeenAt, err = parseNullTime(firstSeenAt); err != nil {
			return nil, fmt.Errorf("parse wx alert first_seen_at: %w", err)
		}
		if netAckAt.Valid {
			t, err := parseTime(netAckAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse wx alert net_ack_at: %w", err)
			}
			a.NetAckAt = &t
		}
		if a.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse wx alert updated_at: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate wx alerts: %w", err)
	}
	return out, nil
}

// parseNullTime returns the zero time.Time for a NULL column (mirrors
// nullTimeVal on the write side) instead of erroring.
func parseNullTime(s sql.NullString) (time.Time, error) {
	if !s.Valid || s.String == "" {
		return time.Time{}, nil
	}
	return parseTime(s.String)
}

func (s *SQLiteStore) UpdateWxAlertNetAck(id, callsign string, at time.Time, data string) error {
	res, err := s.db.Exec(`
		UPDATE wx_alerts SET net_ack_callsign = ?, net_ack_at = ?, data = ?, updated_at = ?
		WHERE id = ?`,
		callsign, at.UTC(), data, at.UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update wx alert net ack: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update wx alert net ack rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("wx alert %q not found", id)
	}
	return nil
}

// PurgeWxAlerts deletes rows that are not active and were last updated
// before olderThan. Active alerts are never purged regardless of age — the
// registry, not a purge sweep, decides when something stops being active.
func (s *SQLiteStore) PurgeWxAlerts(olderThan time.Time) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM wx_alerts WHERE state != 'active' AND updated_at < ?`, olderThan.UTC())
	if err != nil {
		return 0, fmt.Errorf("purge wx alerts: %w", err)
	}
	return res.RowsAffected()
}

func (s *SQLiteStore) SaveWxPointZone(z WxPointZone) error {
	ugcJSON := "[]"
	if len(z.UGC) > 0 {
		b, err := json.Marshal(z.UGC)
		if err != nil {
			return fmt.Errorf("marshal wx point zone ugc: %w", err)
		}
		ugcJSON = string(b)
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO wx_point_zones (cell_lat, cell_lon, ugc, expires_at)
		VALUES (?, ?, ?, ?)`,
		z.CellLat, z.CellLon, ugcJSON, z.ExpiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save wx point zone: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadWxPointZones() ([]WxPointZone, error) {
	rows, err := s.db.Query(`SELECT cell_lat, cell_lon, ugc, expires_at FROM wx_point_zones`)
	if err != nil {
		return nil, fmt.Errorf("query wx point zones: %w", err)
	}
	defer rows.Close()

	var out []WxPointZone
	for rows.Next() {
		var z WxPointZone
		var ugcJSON, expiresAt string
		if err := rows.Scan(&z.CellLat, &z.CellLon, &ugcJSON, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan wx point zone: %w", err)
		}
		z.UGC = []string{}
		if ugcJSON != "" && ugcJSON != "[]" {
			json.Unmarshal([]byte(ugcJSON), &z.UGC)
		}
		if z.ExpiresAt, err = parseTime(expiresAt); err != nil {
			return nil, fmt.Errorf("parse wx point zone expires_at: %w", err)
		}
		out = append(out, z)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate wx point zones: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) GetWxMeta(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM wx_alert_meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get wx meta %q: %w", key, err)
	}
	return value, true, nil
}

func (s *SQLiteStore) SetWxMeta(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO wx_alert_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set wx meta %q: %w", key, err)
	}
	return nil
}

// migrateV26 adds internal/ride's SAG (support-and-gear transport) tables.
// Both are brand new — no ALTER on an existing table, so unlike v22-v25
// there is no present-table guard to worry about; CREATE TABLE IF NOT
// EXISTS makes this idempotent on its own.
//
// Aggregate storage (slots + legs as JSON on the request row) follows the
// net_check_ins mission_ids/tracked_stations idiom: the ride.Manager holds
// the whole aggregate in memory and INSERT OR REPLACEs it; capacity is
// derived, never a stored counter. division is NULLABLE by design — single
// net for v1, parallel Route/RestStop/Medical/Supply nets land later
// without a painful migration.
func (s *SQLiteStore) migrateV26() error {
	ddl := `
CREATE TABLE IF NOT EXISTS sag_requests (
    id              TEXT PRIMARY KEY,
    net_id          TEXT NOT NULL,
    division        TEXT,                        -- NULL in v1
    sequence        INTEGER NOT NULL,
    pickup          TEXT NOT NULL DEFAULT '{}',   -- SAGLocation JSON
    dropoff         TEXT NOT NULL DEFAULT '{}',   -- SAGLocation JSON
    reason          TEXT NOT NULL DEFAULT '',
    priority        TEXT NOT NULL DEFAULT 'medium',
    status          TEXT NOT NULL DEFAULT 'open', -- derived, persisted for queries
    needs_vehicle   INTEGER NOT NULL DEFAULT 1,
    slots           TEXT NOT NULL DEFAULT '[]',   -- []SAGSlot JSON
    legs            TEXT NOT NULL DEFAULT '[]',   -- []SAGLeg JSON
    requested_by    TEXT NOT NULL DEFAULT '',
    created_by_name TEXT NOT NULL DEFAULT '',
    notes           TEXT NOT NULL DEFAULT '',
    cancel_reason   TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    closed_at       DATETIME
);
CREATE INDEX IF NOT EXISTS idx_sag_requests_net ON sag_requests(net_id, sequence);
CREATE INDEX IF NOT EXISTS idx_sag_requests_status ON sag_requests(net_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sag_requests_net_seq ON sag_requests(net_id, sequence);

CREATE TABLE IF NOT EXISTS sag_vehicles (
    net_id       TEXT NOT NULL,
    check_in_id  TEXT NOT NULL,
    division     TEXT,
    seats        INTEGER NOT NULL DEFAULT 3,
    rack_slots   INTEGER NOT NULL DEFAULT 2,
    notes        TEXT NOT NULL DEFAULT '',
    updated_at   DATETIME NOT NULL,
    PRIMARY KEY (net_id, check_in_id)
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v26 create sag tables: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 26); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// --- SAG Request / Vehicle CRUD (internal/ride) ---

func (s *SQLiteStore) SaveSAGRequest(r SAGRequest) error {
	var division interface{}
	if r.Division != nil {
		division = *r.Division
	}

	pickupJSON, err := json.Marshal(r.Pickup)
	if err != nil {
		return fmt.Errorf("marshal pickup: %w", err)
	}
	dropoffJSON, err := json.Marshal(r.Dropoff)
	if err != nil {
		return fmt.Errorf("marshal dropoff: %w", err)
	}

	slotsJSON := "[]"
	if len(r.Slots) > 0 {
		b, err := json.Marshal(r.Slots)
		if err != nil {
			return fmt.Errorf("marshal slots: %w", err)
		}
		slotsJSON = string(b)
	}

	legsJSON := "[]"
	if len(r.Legs) > 0 {
		b, err := json.Marshal(r.Legs)
		if err != nil {
			return fmt.Errorf("marshal legs: %w", err)
		}
		legsJSON = string(b)
	}

	var closedAt interface{}
	if r.ClosedAt != nil {
		closedAt = r.ClosedAt.UTC()
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO sag_requests
			(id, net_id, division, sequence, pickup, dropoff, reason, priority, status,
			 needs_vehicle, slots, legs, requested_by, created_by_name, notes, cancel_reason,
			 created_at, updated_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.NetID, division, r.Sequence, string(pickupJSON), string(dropoffJSON),
		r.Reason, r.Priority, r.Status, r.NeedsVehicle, slotsJSON, legsJSON,
		r.RequestedBy, r.CreatedByName, r.Notes, r.CancelReason,
		r.CreatedAt.UTC(), r.UpdatedAt.UTC(), closedAt,
	)
	if err != nil {
		return fmt.Errorf("save sag request: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadSAGRequests(netID string) ([]SAGRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, division, sequence, pickup, dropoff, reason, priority, status,
		       needs_vehicle, slots, legs, requested_by, created_by_name, notes, cancel_reason,
		       created_at, updated_at, closed_at
		FROM sag_requests WHERE net_id = ? ORDER BY sequence ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query sag requests: %w", err)
	}
	defer rows.Close()

	requests := []SAGRequest{}
	for rows.Next() {
		var r SAGRequest
		var division sql.NullString
		var pickupJSON, dropoffJSON, slotsJSON, legsJSON string
		var createdAt, updatedAt string
		var closedAt sql.NullString

		if err := rows.Scan(
			&r.ID, &r.NetID, &division, &r.Sequence, &pickupJSON, &dropoffJSON,
			&r.Reason, &r.Priority, &r.Status, &r.NeedsVehicle, &slotsJSON, &legsJSON,
			&r.RequestedBy, &r.CreatedByName, &r.Notes, &r.CancelReason,
			&createdAt, &updatedAt, &closedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sag request: %w", err)
		}

		if division.Valid {
			d := division.String
			r.Division = &d
		}

		if pickupJSON != "" && pickupJSON != "{}" {
			if err := json.Unmarshal([]byte(pickupJSON), &r.Pickup); err != nil {
				return nil, fmt.Errorf("unmarshal pickup: %w", err)
			}
		}
		if dropoffJSON != "" && dropoffJSON != "{}" {
			if err := json.Unmarshal([]byte(dropoffJSON), &r.Dropoff); err != nil {
				return nil, fmt.Errorf("unmarshal dropoff: %w", err)
			}
		}

		r.Slots = []SAGSlot{}
		if slotsJSON != "" && slotsJSON != "[]" {
			if err := json.Unmarshal([]byte(slotsJSON), &r.Slots); err != nil {
				return nil, fmt.Errorf("unmarshal slots: %w", err)
			}
		}
		if r.Slots == nil {
			r.Slots = []SAGSlot{}
		}

		r.Legs = []SAGLeg{}
		if legsJSON != "" && legsJSON != "[]" {
			if err := json.Unmarshal([]byte(legsJSON), &r.Legs); err != nil {
				return nil, fmt.Errorf("unmarshal legs: %w", err)
			}
		}
		if r.Legs == nil {
			r.Legs = []SAGLeg{}
		}
		for i := range r.Legs {
			if r.Legs[i].SlotIDs == nil {
				r.Legs[i].SlotIDs = []string{}
			}
		}

		if r.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if r.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		if closedAt.Valid {
			t, err := parseTime(closedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse closed_at: %w", err)
			}
			r.ClosedAt = &t
		}

		requests = append(requests, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sag requests: %w", err)
	}
	return requests, nil
}

func (s *SQLiteStore) SaveSAGVehicle(v SAGVehicle) error {
	var division interface{}
	if v.Division != nil {
		division = *v.Division
	}

	updatedAt := v.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO sag_vehicles
			(net_id, check_in_id, division, seats, rack_slots, notes, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.NetID, v.CheckInID, division, v.Seats, v.RackSlots, v.Notes, updatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save sag vehicle: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadSAGVehicles(netID string) ([]SAGVehicle, error) {
	rows, err := s.db.Query(`
		SELECT net_id, check_in_id, division, seats, rack_slots, notes, updated_at
		FROM sag_vehicles WHERE net_id = ? ORDER BY check_in_id ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query sag vehicles: %w", err)
	}
	defer rows.Close()

	vehicles := []SAGVehicle{}
	for rows.Next() {
		var v SAGVehicle
		var division sql.NullString
		var updatedAt string

		if err := rows.Scan(&v.NetID, &v.CheckInID, &division, &v.Seats, &v.RackSlots, &v.Notes, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan sag vehicle: %w", err)
		}
		if division.Valid {
			d := division.String
			v.Division = &d
		}
		if v.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		vehicles = append(vehicles, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sag vehicles: %w", err)
	}
	return vehicles, nil
}

func (s *SQLiteStore) DeleteSAGVehicle(netID, checkInID string) error {
	_, err := s.db.Exec(`DELETE FROM sag_vehicles WHERE net_id = ? AND check_in_id = ?`, netID, checkInID)
	if err != nil {
		return fmt.Errorf("delete sag vehicle: %w", err)
	}
	return nil
}

// migrateV27 adds internal/course's course-closure tables: per-net policy
// config, scheduled shutoffs, rider exceptions (the ONLY place an individual
// rider appears — bib is a label, never indexed as a key), sweep reports,
// and the per-station closure ladder. All five are brand new (no ALTER on an
// existing table), so — like v26 — CREATE TABLE IF NOT EXISTS makes this
// idempotent on its own with no present-table guard needed. checkpoint_meta
// is reused as-is; aid/start/finish annotations simply start appearing in it
// once internal/checkpoint's SetMeta relaxation lands.
func (s *SQLiteStore) migrateV27() error {
	ddl := `
CREATE TABLE IF NOT EXISTS course_config (
    net_id TEXT PRIMARY KEY,
    division TEXT NOT NULL DEFAULT '',
    sweep_label TEXT NOT NULL DEFAULT 'SWEEP',
    lead_label TEXT NOT NULL DEFAULT 'LEAD',
    close_requires_sweep INTEGER NOT NULL DEFAULT 1,
    auto_sweep_from_passage INTEGER NOT NULL DEFAULT 1,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS course_shutoffs (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL,
    division TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    lat REAL NOT NULL,
    lon REAL NOT NULL,
    route_mile REAL,
    annotation_id TEXT NOT NULL DEFAULT '',
    scheduled_at DATETIME NOT NULL,
    reroute_direction TEXT NOT NULL DEFAULT '',
    reroute_destination TEXT NOT NULL DEFAULT '',
    reroute_instructions TEXT NOT NULL DEFAULT '',
    staffed_by_checkin_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned',
    fired_at DATETIME,
    fired_by TEXT NOT NULL DEFAULT '',
    fire_note TEXT NOT NULL DEFAULT '',
    reroute_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_course_shutoffs_net ON course_shutoffs(net_id);
CREATE INDEX IF NOT EXISTS idx_course_shutoffs_sched ON course_shutoffs(net_id, scheduled_at);

CREATE TABLE IF NOT EXISTS course_rider_exceptions (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL,
    division TEXT NOT NULL DEFAULT '',
    bib TEXT NOT NULL DEFAULT '',
    bib_withheld INTEGER NOT NULL DEFAULT 0,
    kind TEXT NOT NULL,
    support_status TEXT NOT NULL DEFAULT 'supported',
    reason TEXT NOT NULL DEFAULT '',
    route_label TEXT NOT NULL DEFAULT '',
    route_mile REAL,
    lat REAL,
    lon REAL,
    shutoff_id TEXT NOT NULL DEFAULT '',
    sag_request_id TEXT NOT NULL DEFAULT '',
    reported_by TEXT NOT NULL DEFAULT '',
    recorded_at DATETIME NOT NULL,
    status_changed_at DATETIME NOT NULL,
    status_changed_by TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_course_riders_net ON course_rider_exceptions(net_id);
CREATE INDEX IF NOT EXISTS idx_course_riders_status ON course_rider_exceptions(net_id, support_status);

CREATE TABLE IF NOT EXISTS course_sweep_reports (
    id TEXT PRIMARY KEY,
    net_id TEXT NOT NULL,
    division TEXT NOT NULL DEFAULT '',
    route_label TEXT NOT NULL DEFAULT '',
    checkin_id TEXT NOT NULL DEFAULT '',
    reported_by TEXT NOT NULL DEFAULT '',
    route_mile REAL,
    lat REAL,
    lon REAL,
    last_rider_bib TEXT NOT NULL DEFAULT '',
    estimated_speed_mph REAL,
    note TEXT NOT NULL DEFAULT '',
    reported_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_course_sweep_net_time ON course_sweep_reports(net_id, reported_at);

CREATE TABLE IF NOT EXISTS course_station_closures (
    net_id TEXT NOT NULL,
    checkpoint_id TEXT NOT NULL,
    division TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'open',
    riders_clear_at DATETIME,
    riders_clear_by TEXT NOT NULL DEFAULT '',
    sweep_passed_at DATETIME,
    sweep_passed_by TEXT NOT NULL DEFAULT '',
    sweep_passage_id TEXT NOT NULL DEFAULT '',
    closed_at DATETIME,
    closed_by TEXT NOT NULL DEFAULT '',
    closed_by_override INTEGER NOT NULL DEFAULT 0,
    override_reason TEXT NOT NULL DEFAULT '',
    reopen_count INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (net_id, checkpoint_id)
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v27 create course tables: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 27); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// --- Course closure CRUD (internal/course) ---

func (s *SQLiteStore) SaveCourseConfig(c CourseConfig) error {
	updatedAt := c.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO course_config
			(net_id, division, sweep_label, lead_label, close_requires_sweep, auto_sweep_from_passage, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.NetID, c.Division, c.SweepLabel, c.LeadLabel, c.CloseRequiresSweep, c.AutoSweepFromPassage, updatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save course config: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadCourseConfig(netID string) (*CourseConfig, error) {
	var c CourseConfig
	var updatedAt string
	err := s.db.QueryRow(`
		SELECT net_id, division, sweep_label, lead_label, close_requires_sweep, auto_sweep_from_passage, updated_at
		FROM course_config WHERE net_id = ?`, netID).Scan(
		&c.NetID, &c.Division, &c.SweepLabel, &c.LeadLabel, &c.CloseRequiresSweep, &c.AutoSweepFromPassage, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load course config: %w", err)
	}
	if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &c, nil
}

func (s *SQLiteStore) SaveShutoffPoint(sp ShutoffPoint) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO course_shutoffs
			(id, net_id, division, name, lat, lon, route_mile, annotation_id, scheduled_at,
			 reroute_direction, reroute_destination, reroute_instructions, staffed_by_checkin_id,
			 status, fired_at, fired_by, fire_note, reroute_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sp.ID, sp.NetID, sp.Division, sp.Name, sp.Lat, sp.Lon, nullFloatPtr(sp.RouteMile), sp.AnnotationID, sp.ScheduledAt.UTC(),
		sp.RerouteDirection, sp.RerouteDestination, sp.RerouteInstructions, sp.StaffedByCheckInID,
		sp.Status, nullTimePtr(sp.FiredAt), sp.FiredBy, sp.FireNote, sp.RerouteCount, sp.CreatedAt.UTC(), sp.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save shutoff point: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadShutoffPoints(netID string) ([]ShutoffPoint, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, division, name, lat, lon, route_mile, annotation_id, scheduled_at,
		       reroute_direction, reroute_destination, reroute_instructions, staffed_by_checkin_id,
		       status, fired_at, fired_by, fire_note, reroute_count, created_at, updated_at
		FROM course_shutoffs WHERE net_id = ? ORDER BY scheduled_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query shutoff points: %w", err)
	}
	defer rows.Close()

	points := []ShutoffPoint{}
	for rows.Next() {
		var sp ShutoffPoint
		var routeMile sql.NullFloat64
		var scheduledAt, createdAt, updatedAt string
		var firedAt sql.NullString

		if err := rows.Scan(
			&sp.ID, &sp.NetID, &sp.Division, &sp.Name, &sp.Lat, &sp.Lon, &routeMile, &sp.AnnotationID, &scheduledAt,
			&sp.RerouteDirection, &sp.RerouteDestination, &sp.RerouteInstructions, &sp.StaffedByCheckInID,
			&sp.Status, &firedAt, &sp.FiredBy, &sp.FireNote, &sp.RerouteCount, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan shutoff point: %w", err)
		}
		if routeMile.Valid {
			v := routeMile.Float64
			sp.RouteMile = &v
		}
		if sp.ScheduledAt, err = parseTime(scheduledAt); err != nil {
			return nil, fmt.Errorf("parse scheduled_at: %w", err)
		}
		if firedAt.Valid {
			t, err := parseTime(firedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse fired_at: %w", err)
			}
			sp.FiredAt = &t
		}
		if sp.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if sp.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		points = append(points, sp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shutoff points: %w", err)
	}
	return points, nil
}

func (s *SQLiteStore) DeleteShutoffPoint(id string) error {
	_, err := s.db.Exec(`DELETE FROM course_shutoffs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete shutoff point: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveRiderException(r RiderException) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO course_rider_exceptions
			(id, net_id, division, bib, bib_withheld, kind, support_status, reason, route_label,
			 route_mile, lat, lon, shutoff_id, sag_request_id, reported_by, recorded_at,
			 status_changed_at, status_changed_by, note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.NetID, r.Division, r.Bib, r.BibWithheld, r.Kind, r.SupportStatus, r.Reason, r.RouteLabel,
		nullFloatPtr(r.RouteMile), nullFloatPtr(r.Lat), nullFloatPtr(r.Lon), r.ShutoffID, r.SAGRequestID, r.ReportedBy, r.RecordedAt.UTC(),
		r.StatusChangedAt.UTC(), r.StatusChangedBy, r.Note,
	)
	if err != nil {
		return fmt.Errorf("save rider exception: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadRiderExceptions(netID string) ([]RiderException, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, division, bib, bib_withheld, kind, support_status, reason, route_label,
		       route_mile, lat, lon, shutoff_id, sag_request_id, reported_by, recorded_at,
		       status_changed_at, status_changed_by, note
		FROM course_rider_exceptions WHERE net_id = ? ORDER BY recorded_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query rider exceptions: %w", err)
	}
	defer rows.Close()

	riders := []RiderException{}
	for rows.Next() {
		var r RiderException
		var routeMile, lat, lon sql.NullFloat64
		var recordedAt, statusChangedAt string

		if err := rows.Scan(
			&r.ID, &r.NetID, &r.Division, &r.Bib, &r.BibWithheld, &r.Kind, &r.SupportStatus, &r.Reason, &r.RouteLabel,
			&routeMile, &lat, &lon, &r.ShutoffID, &r.SAGRequestID, &r.ReportedBy, &recordedAt,
			&statusChangedAt, &r.StatusChangedBy, &r.Note,
		); err != nil {
			return nil, fmt.Errorf("scan rider exception: %w", err)
		}
		if routeMile.Valid {
			v := routeMile.Float64
			r.RouteMile = &v
		}
		if lat.Valid {
			v := lat.Float64
			r.Lat = &v
		}
		if lon.Valid {
			v := lon.Float64
			r.Lon = &v
		}
		if r.RecordedAt, err = parseTime(recordedAt); err != nil {
			return nil, fmt.Errorf("parse recorded_at: %w", err)
		}
		if r.StatusChangedAt, err = parseTime(statusChangedAt); err != nil {
			return nil, fmt.Errorf("parse status_changed_at: %w", err)
		}
		riders = append(riders, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rider exceptions: %w", err)
	}
	return riders, nil
}

func (s *SQLiteStore) SaveSweepReport(r SweepReport) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO course_sweep_reports
			(id, net_id, division, route_label, checkin_id, reported_by, route_mile, lat, lon,
			 last_rider_bib, estimated_speed_mph, note, reported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.NetID, r.Division, r.RouteLabel, r.CheckInID, r.ReportedBy, nullFloatPtr(r.RouteMile), nullFloatPtr(r.Lat), nullFloatPtr(r.Lon),
		r.LastRiderBib, nullFloatPtr(r.EstimatedSpeedMph), r.Note, r.ReportedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save sweep report: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadSweepReports(netID string, limit int) ([]SweepReport, error) {
	query := `
		SELECT id, net_id, division, route_label, checkin_id, reported_by, route_mile, lat, lon,
		       last_rider_bib, estimated_speed_mph, note, reported_at
		FROM course_sweep_reports WHERE net_id = ? ORDER BY reported_at DESC`
	args := []interface{}{netID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sweep reports: %w", err)
	}
	defer rows.Close()

	reports := []SweepReport{}
	for rows.Next() {
		var r SweepReport
		var routeMile, lat, lon, speed sql.NullFloat64
		var reportedAt string

		if err := rows.Scan(
			&r.ID, &r.NetID, &r.Division, &r.RouteLabel, &r.CheckInID, &r.ReportedBy, &routeMile, &lat, &lon,
			&r.LastRiderBib, &speed, &r.Note, &reportedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sweep report: %w", err)
		}
		if routeMile.Valid {
			v := routeMile.Float64
			r.RouteMile = &v
		}
		if lat.Valid {
			v := lat.Float64
			r.Lat = &v
		}
		if lon.Valid {
			v := lon.Float64
			r.Lon = &v
		}
		if speed.Valid {
			v := speed.Float64
			r.EstimatedSpeedMph = &v
		}
		if r.ReportedAt, err = parseTime(reportedAt); err != nil {
			return nil, fmt.Errorf("parse reported_at: %w", err)
		}
		reports = append(reports, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sweep reports: %w", err)
	}
	return reports, nil
}

func (s *SQLiteStore) SaveStationClosure(c StationClosure) error {
	updatedAt := c.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO course_station_closures
			(net_id, checkpoint_id, division, state, riders_clear_at, riders_clear_by,
			 sweep_passed_at, sweep_passed_by, sweep_passage_id, closed_at, closed_by,
			 closed_by_override, override_reason, reopen_count, note, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.NetID, c.CheckpointID, c.Division, c.State, nullTimePtr(c.RidersClearAt), c.RidersClearBy,
		nullTimePtr(c.SweepPassedAt), c.SweepPassedBy, c.SweepPassageID, nullTimePtr(c.ClosedAt), c.ClosedBy,
		c.ClosedByOverride, c.OverrideReason, c.ReopenCount, c.Note, updatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save station closure: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadStationClosures(netID string) ([]StationClosure, error) {
	rows, err := s.db.Query(`
		SELECT net_id, checkpoint_id, division, state, riders_clear_at, riders_clear_by,
		       sweep_passed_at, sweep_passed_by, sweep_passage_id, closed_at, closed_by,
		       closed_by_override, override_reason, reopen_count, note, updated_at
		FROM course_station_closures WHERE net_id = ?`, netID)
	if err != nil {
		return nil, fmt.Errorf("query station closures: %w", err)
	}
	defer rows.Close()

	closures := []StationClosure{}
	for rows.Next() {
		var c StationClosure
		var ridersClearAt, sweepPassedAt, closedAt sql.NullString
		var updatedAt string

		if err := rows.Scan(
			&c.NetID, &c.CheckpointID, &c.Division, &c.State, &ridersClearAt, &c.RidersClearBy,
			&sweepPassedAt, &c.SweepPassedBy, &c.SweepPassageID, &closedAt, &c.ClosedBy,
			&c.ClosedByOverride, &c.OverrideReason, &c.ReopenCount, &c.Note, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan station closure: %w", err)
		}
		if ridersClearAt.Valid {
			t, err := parseTime(ridersClearAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse riders_clear_at: %w", err)
			}
			c.RidersClearAt = &t
		}
		if sweepPassedAt.Valid {
			t, err := parseTime(sweepPassedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse sweep_passed_at: %w", err)
			}
			c.SweepPassedAt = &t
		}
		if closedAt.Valid {
			t, err := parseTime(closedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse closed_at: %w", err)
			}
			c.ClosedAt = &t
		}
		if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		closures = append(closures, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate station closures: %w", err)
	}
	return closures, nil
}

func (s *SQLiteStore) DeleteCourseDataForNet(netID string) error {
	tables := []string{
		"course_config", "course_shutoffs", "course_rider_exceptions",
		"course_sweep_reports", "course_station_closures",
	}
	for _, table := range tables {
		col := "net_id"
		if _, err := s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", table, col), netID); err != nil {
			return fmt.Errorf("delete %s for net: %w", table, err)
		}
	}
	return nil
}

// migrateV28 adds internal/ride's supply-request and medical-notification
// tables (WP4). Both are brand new — no ALTER on an existing table — so
// CREATE TABLE IF NOT EXISTS makes this idempotent on its own, mirroring
// migrateV19/migrateV26/migrateV27. division is NULLABLE on both, matching
// SAGRequest's own division column (fact 9: single net for v1, parallel
// Route/RestStop/Medical/Supply nets land later without a migration).
func (s *SQLiteStore) migrateV28() error {
	ddl := `
CREATE TABLE IF NOT EXISTS ride_supply_requests (
    id                        TEXT PRIMARY KEY,
    net_id                    TEXT NOT NULL,
    division                  TEXT,                       -- nullable on purpose (fact 9)
    requested_by_checkin_id   TEXT NOT NULL DEFAULT '',
    requested_by_call         TEXT NOT NULL DEFAULT '',
    location                  TEXT NOT NULL DEFAULT '',
    location_annotation_id    TEXT NOT NULL DEFAULT '',
    miles_remaining           REAL,
    route_id                  TEXT NOT NULL DEFAULT '',
    lat                       REAL,
    lon                       REAL,
    items                     TEXT NOT NULL DEFAULT '[]', -- JSON []SupplyItem
    asked_what_else           INTEGER NOT NULL DEFAULT 0,
    priority                  TEXT NOT NULL DEFAULT 'medium',
    notes                     TEXT NOT NULL DEFAULT '',
    status                    TEXT NOT NULL DEFAULT 'draft',
    created_at                DATETIME NOT NULL,
    read_back_at              DATETIME,
    read_back_by              TEXT NOT NULL DEFAULT '',
    relayed_at                DATETIME,
    relayed_to                TEXT NOT NULL DEFAULT '',
    etas                      TEXT NOT NULL DEFAULT '[]', -- JSON []SupplyETA
    delivered_at              DATETIME,
    cancelled_at              DATETIME,
    cancel_reason             TEXT NOT NULL DEFAULT '',
    merged_into_id            TEXT NOT NULL DEFAULT '',
    updated_at                DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ride_supply_net    ON ride_supply_requests(net_id);
CREATE INDEX IF NOT EXISTS idx_ride_supply_status ON ride_supply_requests(net_id, status);

CREATE TABLE IF NOT EXISTS ride_medical_notifications (
    id                        TEXT PRIMARY KEY,
    net_id                    TEXT NOT NULL,
    division                  TEXT,
    reported_by_checkin_id    TEXT NOT NULL DEFAULT '',
    reported_by_call          TEXT NOT NULL DEFAULT '',
    bib                       TEXT NOT NULL DEFAULT '',
    bib_withheld              INTEGER NOT NULL DEFAULT 0,
    sex                       TEXT NOT NULL DEFAULT 'U',
    age                       TEXT NOT NULL DEFAULT '',
    location                  TEXT NOT NULL DEFAULT '',
    miles_remaining           REAL,
    route_id                  TEXT NOT NULL DEFAULT '',
    location_annotation_id    TEXT NOT NULL DEFAULT '',
    lat                       REAL,
    lon                       REAL,
    chief_complaint           TEXT NOT NULL DEFAULT '',
    read_back_at              DATETIME,
    read_back_by              TEXT NOT NULL DEFAULT '',
    severity                  TEXT NOT NULL DEFAULT 'routine',
    priority                  TEXT NOT NULL DEFAULT 'priority',
    status                    TEXT NOT NULL DEFAULT 'reported',
    ems_unit                  TEXT NOT NULL DEFAULT '',
    eta_minutes               INTEGER,
    eta_given_at              DATETIME,
    eta_due_at                DATETIME,
    on_scene_at               DATETIME,
    departed_at               DATETIME,
    on_scene_seconds          INTEGER,
    destination               TEXT NOT NULL DEFAULT '',
    destination_name          TEXT NOT NULL DEFAULT '',
    patient_count             INTEGER NOT NULL DEFAULT 0,
    patient_name              TEXT NOT NULL DEFAULT '',   -- only for hospital/start transports
    released_at               DATETIME,
    cancelled_at              DATETIME,
    cancel_reason             TEXT NOT NULL DEFAULT '',
    notes                     TEXT NOT NULL DEFAULT '',
    created_at                DATETIME NOT NULL,
    updated_at                DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ride_medical_net    ON ride_medical_notifications(net_id);
CREATE INDEX IF NOT EXISTS idx_ride_medical_status ON ride_medical_notifications(net_id, status);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v28 create ride traffic tables: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 28); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// nullIntPtr converts a nullable int column value the same explicit way
// nullFloatPtr does for floats.
func nullIntPtr(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

// --- Ride traffic CRUD (internal/ride, WP4) ---

func (s *SQLiteStore) SaveSupplyRequest(r SupplyRequest) error {
	var division any
	if r.Division != nil {
		division = *r.Division
	}

	itemsJSON := "[]"
	if len(r.Items) > 0 {
		b, err := json.Marshal(r.Items)
		if err != nil {
			return fmt.Errorf("marshal items: %w", err)
		}
		itemsJSON = string(b)
	}
	etasJSON := "[]"
	if len(r.ETAs) > 0 {
		b, err := json.Marshal(r.ETAs)
		if err != nil {
			return fmt.Errorf("marshal etas: %w", err)
		}
		etasJSON = string(b)
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO ride_supply_requests
			(id, net_id, division, requested_by_checkin_id, requested_by_call, location,
			 location_annotation_id, miles_remaining, route_id, lat, lon, items, asked_what_else,
			 priority, notes, status, created_at, read_back_at, read_back_by, relayed_at, relayed_to,
			 etas, delivered_at, cancelled_at, cancel_reason, merged_into_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.NetID, division, r.RequestedByCheckInID, r.RequestedByCall, r.Location,
		r.LocationAnnotationID, nullFloatPtr(r.MilesRemaining), r.RouteID, nullFloatPtr(r.Lat), nullFloatPtr(r.Lon),
		itemsJSON, r.AskedWhatElse, r.Priority, r.Notes, r.Status, r.CreatedAt.UTC(),
		nullTimePtr(r.ReadBackAt), r.ReadBackBy, nullTimePtr(r.RelayedAt), r.RelayedTo,
		etasJSON, nullTimePtr(r.DeliveredAt), nullTimePtr(r.CancelledAt), r.CancelReason, r.MergedIntoID, r.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save supply request: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadSupplyRequests(netID string) ([]SupplyRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, division, requested_by_checkin_id, requested_by_call, location,
		       location_annotation_id, miles_remaining, route_id, lat, lon, items, asked_what_else,
		       priority, notes, status, created_at, read_back_at, read_back_by, relayed_at, relayed_to,
		       etas, delivered_at, cancelled_at, cancel_reason, merged_into_id, updated_at
		FROM ride_supply_requests WHERE net_id = ? ORDER BY created_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query supply requests: %w", err)
	}
	defer rows.Close()

	requests := []SupplyRequest{}
	for rows.Next() {
		var r SupplyRequest
		var division sql.NullString
		var milesRemaining, lat, lon sql.NullFloat64
		var itemsJSON, etasJSON string
		var createdAt, updatedAt string
		var readBackAt, relayedAt, deliveredAt, cancelledAt sql.NullString

		if err := rows.Scan(
			&r.ID, &r.NetID, &division, &r.RequestedByCheckInID, &r.RequestedByCall, &r.Location,
			&r.LocationAnnotationID, &milesRemaining, &r.RouteID, &lat, &lon, &itemsJSON, &r.AskedWhatElse,
			&r.Priority, &r.Notes, &r.Status, &createdAt, &readBackAt, &r.ReadBackBy, &relayedAt, &r.RelayedTo,
			&etasJSON, &deliveredAt, &cancelledAt, &r.CancelReason, &r.MergedIntoID, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan supply request: %w", err)
		}

		if division.Valid {
			d := division.String
			r.Division = &d
		}
		if milesRemaining.Valid {
			v := milesRemaining.Float64
			r.MilesRemaining = &v
		}
		if lat.Valid {
			v := lat.Float64
			r.Lat = &v
		}
		if lon.Valid {
			v := lon.Float64
			r.Lon = &v
		}

		r.Items = []SupplyItem{}
		if itemsJSON != "" && itemsJSON != "[]" {
			if err := json.Unmarshal([]byte(itemsJSON), &r.Items); err != nil {
				return nil, fmt.Errorf("unmarshal items: %w", err)
			}
		}
		if r.Items == nil {
			r.Items = []SupplyItem{}
		}
		r.ETAs = []SupplyETA{}
		if etasJSON != "" && etasJSON != "[]" {
			if err := json.Unmarshal([]byte(etasJSON), &r.ETAs); err != nil {
				return nil, fmt.Errorf("unmarshal etas: %w", err)
			}
		}
		if r.ETAs == nil {
			r.ETAs = []SupplyETA{}
		}

		if r.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if r.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		if readBackAt.Valid {
			t, err := parseTime(readBackAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse read_back_at: %w", err)
			}
			r.ReadBackAt = &t
		}
		if relayedAt.Valid {
			t, err := parseTime(relayedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse relayed_at: %w", err)
			}
			r.RelayedAt = &t
		}
		if deliveredAt.Valid {
			t, err := parseTime(deliveredAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse delivered_at: %w", err)
			}
			r.DeliveredAt = &t
		}
		if cancelledAt.Valid {
			t, err := parseTime(cancelledAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse cancelled_at: %w", err)
			}
			r.CancelledAt = &t
		}

		requests = append(requests, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate supply requests: %w", err)
	}
	return requests, nil
}

func (s *SQLiteStore) SaveMedicalNotification(n MedicalNotification) error {
	var division any
	if n.Division != nil {
		division = *n.Division
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO ride_medical_notifications
			(id, net_id, division, reported_by_checkin_id, reported_by_call, bib, bib_withheld, sex, age,
			 location, miles_remaining, route_id, location_annotation_id, lat, lon, chief_complaint,
			 read_back_at, read_back_by, severity, priority, status, ems_unit, eta_minutes, eta_given_at,
			 eta_due_at, on_scene_at, departed_at, on_scene_seconds, destination, destination_name,
			 patient_count, patient_name, released_at, cancelled_at, cancel_reason, notes,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.NetID, division, n.ReportedByCheckInID, n.ReportedByCall, n.Bib, n.BibWithheld, n.Sex, n.Age,
		n.Location, nullFloatPtr(n.MilesRemaining), n.RouteID, n.LocationAnnotationID, nullFloatPtr(n.Lat), nullFloatPtr(n.Lon), n.ChiefComplaint,
		nullTimePtr(n.ReadBackAt), n.ReadBackBy, n.Severity, n.Priority, n.Status, n.EMSUnit, nullIntPtr(n.ETAMinutes), nullTimePtr(n.ETAGivenAt),
		nullTimePtr(n.ETADueAt), nullTimePtr(n.OnSceneAt), nullTimePtr(n.DepartedAt), nullIntPtr(n.OnSceneSeconds), n.Destination, n.DestinationName,
		n.PatientCount, n.PatientName, nullTimePtr(n.ReleasedAt), nullTimePtr(n.CancelledAt), n.CancelReason, n.Notes,
		n.CreatedAt.UTC(), n.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save medical notification: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadMedicalNotifications(netID string) ([]MedicalNotification, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, division, reported_by_checkin_id, reported_by_call, bib, bib_withheld, sex, age,
		       location, miles_remaining, route_id, location_annotation_id, lat, lon, chief_complaint,
		       read_back_at, read_back_by, severity, priority, status, ems_unit, eta_minutes, eta_given_at,
		       eta_due_at, on_scene_at, departed_at, on_scene_seconds, destination, destination_name,
		       patient_count, patient_name, released_at, cancelled_at, cancel_reason, notes,
		       created_at, updated_at
		FROM ride_medical_notifications WHERE net_id = ? ORDER BY created_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query medical notifications: %w", err)
	}
	defer rows.Close()

	notifications := []MedicalNotification{}
	for rows.Next() {
		var n MedicalNotification
		var division sql.NullString
		var milesRemaining, lat, lon sql.NullFloat64
		var readBackAt, etaGivenAt, etaDueAt, onSceneAt, departedAt, releasedAt, cancelledAt sql.NullString
		var etaMinutes, onSceneSeconds sql.NullInt64
		var createdAt, updatedAt string

		if err := rows.Scan(
			&n.ID, &n.NetID, &division, &n.ReportedByCheckInID, &n.ReportedByCall, &n.Bib, &n.BibWithheld, &n.Sex, &n.Age,
			&n.Location, &milesRemaining, &n.RouteID, &n.LocationAnnotationID, &lat, &lon, &n.ChiefComplaint,
			&readBackAt, &n.ReadBackBy, &n.Severity, &n.Priority, &n.Status, &n.EMSUnit, &etaMinutes, &etaGivenAt,
			&etaDueAt, &onSceneAt, &departedAt, &onSceneSeconds, &n.Destination, &n.DestinationName,
			&n.PatientCount, &n.PatientName, &releasedAt, &cancelledAt, &n.CancelReason, &n.Notes,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan medical notification: %w", err)
		}

		if division.Valid {
			d := division.String
			n.Division = &d
		}
		if milesRemaining.Valid {
			v := milesRemaining.Float64
			n.MilesRemaining = &v
		}
		if lat.Valid {
			v := lat.Float64
			n.Lat = &v
		}
		if lon.Valid {
			v := lon.Float64
			n.Lon = &v
		}
		if etaMinutes.Valid {
			v := int(etaMinutes.Int64)
			n.ETAMinutes = &v
		}
		if onSceneSeconds.Valid {
			v := int(onSceneSeconds.Int64)
			n.OnSceneSeconds = &v
		}

		if n.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if n.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		for _, pair := range []struct {
			col  sql.NullString
			dest **time.Time
		}{
			{readBackAt, &n.ReadBackAt}, {etaGivenAt, &n.ETAGivenAt}, {etaDueAt, &n.ETADueAt},
			{onSceneAt, &n.OnSceneAt}, {departedAt, &n.DepartedAt}, {releasedAt, &n.ReleasedAt},
			{cancelledAt, &n.CancelledAt},
		} {
			if pair.col.Valid {
				t, err := parseTime(pair.col.String)
				if err != nil {
					return nil, fmt.Errorf("parse timestamp: %w", err)
				}
				*pair.dest = &t
			}
		}

		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate medical notifications: %w", err)
	}
	return notifications, nil
}

func (s *SQLiteStore) DeleteRideTraffic(netID string) error {
	for _, table := range []string{"ride_supply_requests", "ride_medical_notifications"} {
		if _, err := s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE net_id = ?", table), netID); err != nil {
			return fmt.Errorf("delete %s for net: %w", table, err)
		}
	}
	return nil
}

// migrateV29 adds the ride-reconciliation (WP5) tables: SAG driver shift
// summaries and the NCS shift-relief handoff paper trail. Rider exceptions
// reuse internal/course's ride_rider_exceptions table (migrateV27) — see
// store.go's own doc comment on SAGShiftSummary for why this WP does not
// add a second, differently-shaped rider table.
func (s *SQLiteStore) migrateV29() error {
	ddl := `
CREATE TABLE IF NOT EXISTS ride_sag_shift_summaries (
    id                 TEXT PRIMARY KEY,
    net_id             TEXT NOT NULL,
    division           TEXT NOT NULL DEFAULT '',
    check_in_id        TEXT NOT NULL,
    callsign           TEXT NOT NULL DEFAULT '',
    tactical_call      TEXT NOT NULL DEFAULT '',
    driver_name        TEXT NOT NULL DEFAULT '',
    vehicle            TEXT NOT NULL DEFAULT '',
    shift_start        DATETIME,
    shift_end          DATETIME,
    odometer_start     REAL,
    odometer_end       REAL,
    transports         INTEGER,                    -- NULL = not entered (falls back to derived)
    assists            INTEGER,
    tubes_provided     INTEGER,
    tires_provided     INTEGER,
    minor_first_aid    INTEGER,
    incidents_attended INTEGER,
    notes              TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL DEFAULT 'draft',
    filed_at           DATETIME,
    filed_by           TEXT NOT NULL DEFAULT '',
    created_at         DATETIME NOT NULL,
    updated_at         DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ride_shift_net     ON ride_sag_shift_summaries(net_id);
CREATE INDEX IF NOT EXISTS idx_ride_shift_checkin ON ride_sag_shift_summaries(check_in_id);

CREATE TABLE IF NOT EXISTS ride_handoff_items (
    id             TEXT PRIMARY KEY,
    net_id         TEXT NOT NULL,
    division       TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL,
    summary        TEXT NOT NULL,
    sent_to        TEXT NOT NULL DEFAULT '',
    reply_to       TEXT NOT NULL DEFAULT 'NCS',
    ref_type       TEXT NOT NULL DEFAULT '',
    ref_id         TEXT NOT NULL DEFAULT '',
    due_at         DATETIME,
    status         TEXT NOT NULL DEFAULT 'open',
    handover_count INTEGER NOT NULL DEFAULT 0,
    created_by     TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL,
    resolved_by    TEXT NOT NULL DEFAULT '',
    resolved_at    DATETIME,
    resolution     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ride_handoff_net_status ON ride_handoff_items(net_id, status);

CREATE TABLE IF NOT EXISTS ride_shift_handoffs (
    id              TEXT PRIMARY KEY,
    net_id          TEXT NOT NULL,
    division        TEXT NOT NULL DEFAULT '',
    from_callsign   TEXT NOT NULL DEFAULT '',
    to_callsign     TEXT NOT NULL DEFAULT '',
    at              DATETIME NOT NULL,
    open_item_ids   TEXT NOT NULL DEFAULT '[]',    -- JSON []string, never nil
    briefing        TEXT NOT NULL DEFAULT '{}',    -- frozen ShiftBriefing JSON
    acknowledged_at DATETIME,
    acknowledged_by TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ride_shift_handoffs_net ON ride_shift_handoffs(net_id, at);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v29 create ride reconciliation tables: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 29); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// migrateV30 adds the ride-phase table (internal/ride/phase, WP5b): one row
// per net recording the operator-set ride phase (pre-start | launched |
// mid-ride | closing | collapse | reconcile). Brand new table — no ALTER on
// an existing one — so CREATE TABLE IF NOT EXISTS makes this idempotent on
// its own, mirroring migrateV26/27/28/29. A net with no row is simply
// pre-start (the manager's default); this table only ever gains a row once
// an operator makes the net's first phase change.
func (s *SQLiteStore) migrateV30() error {
	ddl := `
CREATE TABLE IF NOT EXISTS ride_phase_state (
    net_id     TEXT PRIMARY KEY,
    phase      TEXT NOT NULL,
    set_by     TEXT NOT NULL DEFAULT '',
    reason     TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate v30 create ride_phase_state: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, 30); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// --- Ride phase CRUD (internal/ride/phase, WP5b) ---

// SaveRidePhase persists the current ride phase for one net (INSERT OR
// REPLACE — one row per net, mirroring SaveCourseConfig).
func (s *SQLiteStore) SaveRidePhase(p RidePhaseState) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO ride_phase_state (net_id, phase, set_by, reason, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		p.NetID, p.Phase, p.SetBy, p.Reason, p.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save ride phase: %w", err)
	}
	return nil
}

// LoadRidePhase returns the net's current ride phase row, or nil,nil when
// no phase has ever been set for it (the manager's default is pre-start).
func (s *SQLiteStore) LoadRidePhase(netID string) (*RidePhaseState, error) {
	var p RidePhaseState
	var updatedAt string
	err := s.db.QueryRow(`
		SELECT net_id, phase, set_by, reason, updated_at
		FROM ride_phase_state WHERE net_id = ?`, netID).Scan(
		&p.NetID, &p.Phase, &p.SetBy, &p.Reason, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load ride phase: %w", err)
	}
	if p.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &p, nil
}

// --- Ride reconciliation CRUD (WP5) ---

func (s *SQLiteStore) SaveSAGShiftSummary(sm SAGShiftSummary) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO ride_sag_shift_summaries
			(id, net_id, division, check_in_id, callsign, tactical_call, driver_name, vehicle,
			 shift_start, shift_end, odometer_start, odometer_end,
			 transports, assists, tubes_provided, tires_provided, minor_first_aid, incidents_attended,
			 notes, status, filed_at, filed_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sm.ID, sm.NetID, sm.Division, sm.CheckInID, sm.Callsign, sm.TacticalCall, sm.DriverName, sm.Vehicle,
		nullTimePtr(sm.ShiftStart), nullTimePtr(sm.ShiftEnd), nullFloatPtr(sm.OdometerStart), nullFloatPtr(sm.OdometerEnd),
		nullIntPtr(sm.Entered.Transports), nullIntPtr(sm.Entered.Assists), nullIntPtr(sm.Entered.TubesProvided),
		nullIntPtr(sm.Entered.TiresProvided), nullIntPtr(sm.Entered.MinorFirstAid), nullIntPtr(sm.Entered.IncidentsAttended),
		sm.Notes, sm.Status, nullTimePtr(sm.FiledAt), sm.FiledBy, sm.CreatedAt.UTC(), sm.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save sag shift summary: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadSAGShiftSummaries(netID string) ([]SAGShiftSummary, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, division, check_in_id, callsign, tactical_call, driver_name, vehicle,
		       shift_start, shift_end, odometer_start, odometer_end,
		       transports, assists, tubes_provided, tires_provided, minor_first_aid, incidents_attended,
		       notes, status, filed_at, filed_by, created_at, updated_at
		FROM ride_sag_shift_summaries WHERE net_id = ? ORDER BY created_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query sag shift summaries: %w", err)
	}
	defer rows.Close()

	out := []SAGShiftSummary{}
	for rows.Next() {
		var sm SAGShiftSummary
		var shiftStart, shiftEnd, filedAt sql.NullString
		var odoStart, odoEnd sql.NullFloat64
		var transports, assists, tubes, tires, minorAid, incidents sql.NullInt64
		var createdAt, updatedAt string

		if err := rows.Scan(
			&sm.ID, &sm.NetID, &sm.Division, &sm.CheckInID, &sm.Callsign, &sm.TacticalCall, &sm.DriverName, &sm.Vehicle,
			&shiftStart, &shiftEnd, &odoStart, &odoEnd,
			&transports, &assists, &tubes, &tires, &minorAid, &incidents,
			&sm.Notes, &sm.Status, &filedAt, &sm.FiledBy, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sag shift summary: %w", err)
		}

		if sm.ShiftStart, err = nullStringToTimePtr(shiftStart); err != nil {
			return nil, fmt.Errorf("parse shift_start: %w", err)
		}
		if sm.ShiftEnd, err = nullStringToTimePtr(shiftEnd); err != nil {
			return nil, fmt.Errorf("parse shift_end: %w", err)
		}
		if sm.FiledAt, err = nullStringToTimePtr(filedAt); err != nil {
			return nil, fmt.Errorf("parse filed_at: %w", err)
		}
		if odoStart.Valid {
			v := odoStart.Float64
			sm.OdometerStart = &v
		}
		if odoEnd.Valid {
			v := odoEnd.Float64
			sm.OdometerEnd = &v
		}
		sm.Entered.Transports = nullInt64Ptr(transports)
		sm.Entered.Assists = nullInt64Ptr(assists)
		sm.Entered.TubesProvided = nullInt64Ptr(tubes)
		sm.Entered.TiresProvided = nullInt64Ptr(tires)
		sm.Entered.MinorFirstAid = nullInt64Ptr(minorAid)
		sm.Entered.IncidentsAttended = nullInt64Ptr(incidents)

		if sm.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if sm.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}

		out = append(out, sm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sag shift summaries: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) SaveHandoffItem(h HandoffItem) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO ride_handoff_items
			(id, net_id, division, kind, summary, sent_to, reply_to, ref_type, ref_id, due_at,
			 status, handover_count, created_by, created_at, resolved_by, resolved_at, resolution)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.NetID, h.Division, h.Kind, h.Summary, h.SentTo, h.ReplyTo, h.RefType, h.RefID, nullTimePtr(h.DueAt),
		h.Status, h.HandoverCount, h.CreatedBy, h.CreatedAt.UTC(), h.ResolvedBy, nullTimePtr(h.ResolvedAt), h.Resolution,
	)
	if err != nil {
		return fmt.Errorf("save handoff item: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadHandoffItems(netID string) ([]HandoffItem, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, division, kind, summary, sent_to, reply_to, ref_type, ref_id, due_at,
		       status, handover_count, created_by, created_at, resolved_by, resolved_at, resolution
		FROM ride_handoff_items WHERE net_id = ? ORDER BY created_at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query handoff items: %w", err)
	}
	defer rows.Close()

	out := []HandoffItem{}
	for rows.Next() {
		var h HandoffItem
		var dueAt, resolvedAt sql.NullString
		var createdAt string

		if err := rows.Scan(
			&h.ID, &h.NetID, &h.Division, &h.Kind, &h.Summary, &h.SentTo, &h.ReplyTo, &h.RefType, &h.RefID, &dueAt,
			&h.Status, &h.HandoverCount, &h.CreatedBy, &createdAt, &h.ResolvedBy, &resolvedAt, &h.Resolution,
		); err != nil {
			return nil, fmt.Errorf("scan handoff item: %w", err)
		}

		if h.DueAt, err = nullStringToTimePtr(dueAt); err != nil {
			return nil, fmt.Errorf("parse due_at: %w", err)
		}
		if h.ResolvedAt, err = nullStringToTimePtr(resolvedAt); err != nil {
			return nil, fmt.Errorf("parse resolved_at: %w", err)
		}
		if h.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}

		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate handoff items: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) SaveShiftHandoff(h ShiftHandoff) error {
	openItemsJSON := "[]"
	if len(h.OpenItemIDs) > 0 {
		b, err := json.Marshal(h.OpenItemIDs)
		if err != nil {
			return fmt.Errorf("marshal open item ids: %w", err)
		}
		openItemsJSON = string(b)
	}
	briefing := h.Briefing
	if briefing == "" {
		briefing = "{}"
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO ride_shift_handoffs
			(id, net_id, division, from_callsign, to_callsign, at, open_item_ids, briefing,
			 acknowledged_at, acknowledged_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.NetID, h.Division, h.FromCallsign, h.ToCallsign, h.At.UTC(), openItemsJSON, briefing,
		nullTimePtr(h.AcknowledgedAt), h.AcknowledgedBy,
	)
	if err != nil {
		return fmt.Errorf("save shift handoff: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadShiftHandoffs(netID string) ([]ShiftHandoff, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, division, from_callsign, to_callsign, at, open_item_ids, briefing,
		       acknowledged_at, acknowledged_by
		FROM ride_shift_handoffs WHERE net_id = ? ORDER BY at ASC`, netID)
	if err != nil {
		return nil, fmt.Errorf("query shift handoffs: %w", err)
	}
	defer rows.Close()

	out := []ShiftHandoff{}
	for rows.Next() {
		var h ShiftHandoff
		var at string
		var openItemsJSON, briefing string
		var ackAt sql.NullString

		if err := rows.Scan(
			&h.ID, &h.NetID, &h.Division, &h.FromCallsign, &h.ToCallsign, &at, &openItemsJSON, &briefing,
			&ackAt, &h.AcknowledgedBy,
		); err != nil {
			return nil, fmt.Errorf("scan shift handoff: %w", err)
		}

		if h.At, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("parse at: %w", err)
		}
		h.Briefing = briefing

		h.OpenItemIDs = []string{}
		if openItemsJSON != "" && openItemsJSON != "[]" {
			if err := json.Unmarshal([]byte(openItemsJSON), &h.OpenItemIDs); err != nil {
				return nil, fmt.Errorf("unmarshal open item ids: %w", err)
			}
		}
		if h.OpenItemIDs == nil {
			h.OpenItemIDs = []string{}
		}

		if h.AcknowledgedAt, err = nullStringToTimePtr(ackAt); err != nil {
			return nil, fmt.Errorf("parse acknowledged_at: %w", err)
		}

		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shift handoffs: %w", err)
	}
	return out, nil
}

// nullStringToTimePtr parses a nullable DATETIME column scanned as
// sql.NullString into a *time.Time, nil when the column was NULL.
func nullStringToTimePtr(v sql.NullString) (*time.Time, error) {
	if !v.Valid {
		return nil, nil
	}
	t, err := parseTime(v.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// nullInt64Ptr converts a nullable INTEGER column scanned as sql.NullInt64
// into a *int, nil when the column was NULL.
func nullInt64Ptr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

// Compile-time check that SQLiteStore implements Store.
var _ Store = (*SQLiteStore)(nil)
