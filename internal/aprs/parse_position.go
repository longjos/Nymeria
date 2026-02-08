package aprs

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// parsePositionPayload parses a position report payload.
// DTIs: ! = no timestamp no messaging, = with messaging, / timestamp no messaging, @ timestamp with messaging
func parsePositionPayload(payload string) (*PositionData, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("position payload too short")
	}

	dti := payload[0]
	rest := payload[1:]

	var pos PositionData
	var err error

	// Check if this DTI includes a timestamp
	hasTimestamp := dti == '/' || dti == '@'

	if hasTimestamp {
		if len(rest) < 7 {
			return nil, fmt.Errorf("position payload too short for timestamp")
		}
		pos.Timestamp, err = parseTimestamp(rest[:7])
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp: %w", err)
		}
		rest = rest[7:]
	}

	if len(rest) == 0 {
		return nil, fmt.Errorf("no position data after header")
	}

	// Detect compressed vs uncompressed
	// Compressed: first char is symbol table (/ \ or overlay A-Z 0-9)
	// Uncompressed: first char is a digit (latitude)
	if isCompressedPosition(rest) {
		err = parseCompressedPosition(rest, &pos)
	} else {
		err = parseUncompressedPosition(rest, &pos)
	}
	if err != nil {
		return nil, err
	}

	return &pos, nil
}

// isCompressedPosition checks if position data is compressed format.
// Compressed positions have the symbol table char first, followed by 4 base91 lat bytes.
// Uncompressed positions start with a digit (latitude degrees).
func isCompressedPosition(data string) bool {
	if len(data) < 13 {
		// Compressed requires at least 13 bytes: table + 4 lat + 4 lon + code + 2 cs + type
		// But some packets may be shorter. Check if first char looks like a digit.
		if len(data) > 0 && data[0] >= '0' && data[0] <= '9' {
			return false
		}
		if len(data) > 0 && data[0] == ' ' {
			return false // position ambiguity
		}
	}
	// If first char is not a digit or space, likely compressed
	if len(data) > 0 {
		c := data[0]
		if c >= '0' && c <= '9' {
			return false
		}
		if c == ' ' {
			return false
		}
	}
	return true
}

// parseUncompressedPosition parses uncompressed lat/lon from APRS format.
// Format: DDMM.SSN/DDDMM.SSW$ (19 chars for lat/lon/symbol)
func parseUncompressedPosition(data string, pos *PositionData) error {
	if len(data) < 19 {
		return fmt.Errorf("uncompressed position too short: %d chars", len(data))
	}

	// Parse latitude: DDMM.SSn where n = N/S (8 chars)
	latStr := data[:8]
	lat, ambiguity, err := parseLatitude(latStr)
	if err != nil {
		return fmt.Errorf("invalid latitude: %w", err)
	}
	pos.Lat = lat
	pos.Ambiguity = ambiguity

	// Symbol table identifier
	pos.Symbol.Table = data[8]

	// Parse longitude: DDDMM.SSw where w = E/W (9 chars)
	lonStr := data[9:18]
	lon, _, err := parseLongitude(lonStr)
	if err != nil {
		return fmt.Errorf("invalid longitude: %w", err)
	}
	pos.Lon = lon

	// Symbol code
	pos.Symbol.Code = data[18]

	// Remaining is comment + possible CSE/SPD and altitude
	if len(data) > 19 {
		comment := data[19:]
		parsePositionComment(comment, pos)
	}

	return nil
}

// parseLatitude parses "DDMM.SSn" where n is N or S.
// Spaces indicate position ambiguity.
func parseLatitude(s string) (float64, int, error) {
	if len(s) != 8 {
		return 0, 0, fmt.Errorf("latitude must be 8 chars, got %d", len(s))
	}

	// Count ambiguity (spaces in minute fields)
	ambiguity := 0
	cleaned := make([]byte, len(s))
	copy(cleaned, s)
	// Ambiguity replaces digits with spaces from right to left in MM.SS
	for i := 6; i >= 2; i-- {
		if i == 4 {
			continue // skip the dot
		}
		if cleaned[i] == ' ' {
			ambiguity++
			cleaned[i] = '0'
		}
	}

	ns := cleaned[7]
	degStr := string(cleaned[:2])
	minStr := string(cleaned[2:7])

	deg, err := strconv.ParseFloat(degStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude degrees: %w", err)
	}
	min, err := strconv.ParseFloat(minStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude minutes: %w", err)
	}

	lat := deg + min/60.0
	if ns == 'S' || ns == 's' {
		lat = -lat
	}

	return lat, ambiguity, nil
}

// parseLongitude parses "DDDMM.SSw" where w is E or W.
func parseLongitude(s string) (float64, int, error) {
	if len(s) != 9 {
		return 0, 0, fmt.Errorf("longitude must be 9 chars, got %d", len(s))
	}

	ambiguity := 0
	cleaned := make([]byte, len(s))
	copy(cleaned, s)
	for i := 7; i >= 3; i-- {
		if i == 5 {
			continue // skip the dot
		}
		if cleaned[i] == ' ' {
			ambiguity++
			cleaned[i] = '0'
		}
	}

	ew := cleaned[8]
	degStr := string(cleaned[:3])
	minStr := string(cleaned[3:8])

	deg, err := strconv.ParseFloat(degStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude degrees: %w", err)
	}
	min, err := strconv.ParseFloat(minStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude minutes: %w", err)
	}

	lon := deg + min/60.0
	if ew == 'W' || ew == 'w' {
		lon = -lon
	}

	return lon, ambiguity, nil
}

// parseCompressedPosition parses a compressed APRS position.
// Format: /YYYYXXXX$csT where YYYY=lat, XXXX=lon (base91), $=symbol code, cs=compressed course/speed, T=type
func parseCompressedPosition(data string, pos *PositionData) error {
	if len(data) < 10 {
		return fmt.Errorf("compressed position too short: %d chars", len(data))
	}

	pos.Symbol.Table = data[0]

	// Decode Base91 latitude (4 chars)
	latVal := base91Decode(data[1:5])
	pos.Lat = 90.0 - float64(latVal)/380926.0

	// Decode Base91 longitude (4 chars)
	lonVal := base91Decode(data[5:9])
	pos.Lon = -180.0 + float64(lonVal)/190463.0

	pos.Symbol.Code = data[9]

	// Parse compressed course/speed/altitude if present
	if len(data) >= 13 {
		c := int(data[10]) - 33
		s := int(data[11]) - 33
		t := int(data[12]) - 33

		// Type byte bits determine meaning of c/s bytes
		nmeaSrc := (t >> 3) & 0x03

		if c >= 0 && c <= 89 && nmeaSrc != 2 {
			// Course/speed
			pos.Course = float64(c) * 4.0
			pos.Speed = math.Pow(1.08, float64(s)) - 1.0
			pos.Speed *= 1.852 // knots to km/h
		}
	}

	// Comment after compressed data
	if len(data) > 13 {
		pos.Comment = data[13:]
	}

	return nil
}

// base91Decode decodes a 4-character base91 string to an integer.
func base91Decode(s string) int {
	val := 0
	for i := 0; i < len(s); i++ {
		val = val*91 + (int(s[i]) - 33)
	}
	return val
}

// parseTimestamp parses APRS timestamp formats:
// DDHHMMz = day/hours/minutes zulu
// DDHHMMl = day/hours/minutes local (we treat as zulu since we don't know local tz)
// HHMMSSh = hours/minutes/seconds
func parseTimestamp(s string) (time.Time, error) {
	if len(s) < 7 {
		return time.Time{}, fmt.Errorf("timestamp too short")
	}

	now := time.Now().UTC()
	suffix := s[6]

	switch suffix {
	case 'z', '/':
		// DDHHMMz format
		day, err := strconv.Atoi(s[0:2])
		if err != nil {
			return time.Time{}, err
		}
		hour, err := strconv.Atoi(s[2:4])
		if err != nil {
			return time.Time{}, err
		}
		min, err := strconv.Atoi(s[4:6])
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(now.Year(), now.Month(), day, hour, min, 0, 0, time.UTC), nil

	case 'h':
		// HHMMSSh format
		hour, err := strconv.Atoi(s[0:2])
		if err != nil {
			return time.Time{}, err
		}
		min, err := strconv.Atoi(s[2:4])
		if err != nil {
			return time.Time{}, err
		}
		sec, err := strconv.Atoi(s[4:6])
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(now.Year(), now.Month(), now.Day(), hour, min, sec, 0, time.UTC), nil

	default:
		// Treat as DDHHMMl (local time, but we just parse it)
		day, err := strconv.Atoi(s[0:2])
		if err != nil {
			return time.Time{}, err
		}
		hour, err := strconv.Atoi(s[2:4])
		if err != nil {
			return time.Time{}, err
		}
		min, err := strconv.Atoi(s[4:6])
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(now.Year(), now.Month(), day, hour, min, 0, 0, time.UTC), nil
	}
}

// parsePositionComment extracts CSE/SPD, altitude, and APRS 1.2 !DAO! from position comment.
func parsePositionComment(comment string, pos *PositionData) {
	// Check for CSE/SPD: DDD/SSS (7 chars at start)
	if len(comment) >= 7 && comment[3] == '/' {
		cseStr := comment[:3]
		spdStr := comment[4:7]
		cse, err1 := strconv.ParseFloat(cseStr, 64)
		spd, err2 := strconv.ParseFloat(spdStr, 64)
		if err1 == nil && err2 == nil {
			pos.Course = cse
			pos.Speed = spd * 1.852 // knots to km/h
			comment = comment[7:]
		}
	}

	// Check for altitude: /A=DDDDDD
	if idx := strings.Index(comment, "/A="); idx >= 0 {
		altStr := comment[idx+3:]
		if len(altStr) >= 6 {
			alt, err := strconv.ParseFloat(altStr[:6], 64)
			if err == nil {
				pos.Altitude = alt * 0.3048 // feet to meters
			}
			// Remove /A=DDDDDD from comment
			comment = comment[:idx] + comment[idx+9:]
		}
	}

	// APRS 1.2: Extract !DAO! high-precision position extension
	comment = parseDAO(comment, pos)

	pos.Comment = strings.TrimSpace(comment)
}

// parseDAO extracts the APRS 1.2 !DAO! datum/precision extension from comment.
// Format: !DAO! where D=datum char, A=lat extra, O=lon extra.
// Uppercase D: A and O are digits 0-9 (human-readable 3rd decimal, ~6 feet).
// Lowercase D: A and O are base-91 encoded (sub-foot precision).
// Returns the comment with !DAO! removed.
func parseDAO(comment string, pos *PositionData) string {
	idx := strings.Index(comment, "!")
	for idx >= 0 && idx+4 < len(comment) {
		if comment[idx+4] == '!' {
			d := comment[idx+1]
			a := comment[idx+2]
			o := comment[idx+3]

			if isDAODatum(d) {
				pos.Datum = string(d)
				applyDAO(pos, d, a, o)
				// Remove !DAO! from comment
				comment = comment[:idx] + comment[idx+5:]
				return comment
			}
		}
		// Search for next '!'
		next := strings.Index(comment[idx+1:], "!")
		if next < 0 {
			break
		}
		idx = idx + 1 + next
	}
	return comment
}

// isDAODatum checks if a byte is a valid DAO datum character (A-Z or a-z).
func isDAODatum(d byte) bool {
	return (d >= 'A' && d <= 'Z') || (d >= 'a' && d <= 'z')
}

// applyDAO applies the extra precision from !DAO! to the position.
func applyDAO(pos *PositionData, d, a, o byte) {
	if d >= 'A' && d <= 'Z' {
		// Uppercase: human-readable extra digit (0-9)
		// Adds a third decimal to minutes: DDMM.SS -> DDMM.SSx
		pos.Precision = 1
		if a >= '0' && a <= '9' {
			latExtra := float64(a-'0') / 6000.0 // extra digit in minutes/1000, converted to degrees
			if pos.Lat >= 0 {
				pos.Lat += latExtra
			} else {
				pos.Lat -= latExtra
			}
		}
		if o >= '0' && o <= '9' {
			lonExtra := float64(o-'0') / 6000.0
			if pos.Lon >= 0 {
				pos.Lon += lonExtra
			} else {
				pos.Lon -= lonExtra
			}
		}
	} else if d >= 'a' && d <= 'z' {
		// Lowercase: base-91 encoded extra precision (~1 foot)
		// Each byte encodes 0-90, scaled to 0-99 hundredths
		pos.Precision = 2
		if a >= '!' && a <= '{' {
			latExtra := float64(a-33) / 91.0 / 600.0 // base91 digit scaled to degree fraction
			if pos.Lat >= 0 {
				pos.Lat += latExtra
			} else {
				pos.Lat -= latExtra
			}
		}
		if o >= '!' && o <= '{' {
			lonExtra := float64(o-33) / 91.0 / 600.0
			if pos.Lon >= 0 {
				pos.Lon += lonExtra
			} else {
				pos.Lon -= lonExtra
			}
		}
	}
}
