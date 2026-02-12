package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/narvel/nymeria/internal/message"
	"github.com/narvel/nymeria/internal/station"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 18

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
			 net_id, short_name, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Type, a.Label, a.Description, a.Geometry, a.Style,
		a.CreatedBy, a.CreatedByName, a.CreatedAt.UTC(), a.UpdatedAt.UTC(),
		a.Category, a.Status, a.Priority, a.OperationID, string(missionIDsJSON),
		a.Resources, a.ReportedBy, reportedAt, resolvedAt, expiresAt,
		a.NetID, a.ShortName, a.SortOrder,
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
		       net_id, short_name, sort_order
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

	query := `SELECT id, type, label, description, geometry, style,
		       created_by, created_by_name, created_at, updated_at,
		       category, status, priority, operation_id, mission_ids,
		       resources, reported_by, reported_at, resolved_at, expires_at,
		       net_id, short_name, sort_order
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
			&netID, &shortName, &a.SortOrder,
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

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO nets
			(id, name, type, frequency, ncs_callsign, ncs_user_id, status, opened_at, closed_at, notes, mission_brief, ops_view_lat, ops_view_lon, ops_view_zoom, pinned_stations)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Name, n.Type, n.Frequency, n.NCSCallsign, n.NCSUserID,
		n.Status, openedAt, closedAt, n.Notes, n.MissionBrief,
		n.OpsViewLat, n.OpsViewLon, n.OpsViewZoom, pinnedJSON,
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
	var pinnedJSON string

	err := s.db.QueryRow(`
		SELECT id, name, type, frequency, ncs_callsign, ncs_user_id,
		       status, opened_at, closed_at, notes, mission_brief,
		       ops_view_lat, ops_view_lon, ops_view_zoom, pinned_stations
		FROM nets WHERE id = ?`, id).Scan(
		&n.ID, &n.Name, &n.Type, &n.Frequency, &n.NCSCallsign, &n.NCSUserID,
		&n.Status, &openedAt, &closedAt, &n.Notes, &n.MissionBrief,
		&opsLat, &opsLon, &opsZoom, &pinnedJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("net %q not found", id)
		}
		return nil, fmt.Errorf("load net: %w", err)
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

	return &n, nil
}

func (s *SQLiteStore) LoadNets() ([]Net, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, frequency, ncs_callsign, ncs_user_id,
		       status, opened_at, closed_at, notes, mission_brief,
		       ops_view_lat, ops_view_lon, ops_view_zoom, pinned_stations
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
		var pinnedJSON string

		if err := rows.Scan(
			&n.ID, &n.Name, &n.Type, &n.Frequency, &n.NCSCallsign, &n.NCSUserID,
			&n.Status, &openedAt, &closedAt, &n.Notes, &n.MissionBrief,
			&opsLat, &opsLon, &opsZoom, &pinnedJSON,
		); err != nil {
			return nil, fmt.Errorf("scan net: %w", err)
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
			 source, category, location, lat, lon, assignment,
			 mission_ids, tracked_stations, checked_in_at, checked_out_at, last_heard, missed_roll_calls)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ci.ID, ci.NetID, ci.Callsign, ci.TacticalCall, ci.OperatorName,
		ci.Status, ci.Traffic, source, category, ci.Location,
		lat, lon, ci.Assignment,
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
		       source, category, location, lat, lon, assignment,
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
			&lat, &lon, &ci.Assignment,
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
		m.Status, m.AssignedTo, m.Location, lat, lon, m.CreatedAt.UTC(), completedAt,
	)
	if err != nil {
		return fmt.Errorf("save net mission: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadNetMissions(netID string) ([]NetMission, error) {
	rows, err := s.db.Query(`
		SELECT id, net_id, title, description, priority, status, assigned_to,
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
			&m.Status, &m.AssignedTo, &m.Location, &lat, &lon, &createdAt, &completedAt,
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

// Compile-time check that SQLiteStore implements Store.
var _ Store = (*SQLiteStore)(nil)
