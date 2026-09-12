package gps

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

// ErrChecksum, ErrMalformed, ErrUnsupported are returned by sentence parsing.
var (
	ErrChecksum    = errors.New("nmea: checksum mismatch")
	ErrMalformed   = errors.New("nmea: malformed sentence")
	ErrUnsupported = errors.New("nmea: unsupported sentence")
)

// VerifyChecksum validates "$....*HH" — the leading "$" and a trailing
// "*HH" checksum are both required. Talker-agnostic.
func VerifyChecksum(sentence string) error {
	if len(sentence) == 0 || sentence[0] != '$' {
		return ErrChecksum
	}
	star := strings.LastIndexByte(sentence, '*')
	if star < 0 || star+3 > len(sentence) {
		return ErrChecksum
	}
	want, err := strconv.ParseUint(sentence[star+1:star+3], 16, 8)
	if err != nil {
		return ErrChecksum
	}
	var got byte
	for i := 1; i < star; i++ {
		got ^= sentence[i]
	}
	if byte(want) != got {
		return ErrChecksum
	}
	return nil
}

// Fields splits a validated sentence into its comma-separated fields with
// the leading "$" and trailing "*HH" removed. fields[0] is e.g. "GPGGA".
func Fields(sentence string) ([]string, error) {
	if err := VerifyChecksum(sentence); err != nil {
		return nil, err
	}
	star := strings.LastIndexByte(sentence, '*')
	return strings.Split(sentence[1:star], ","), nil
}

// fieldAt returns fields[i], or "" if out of range.
func fieldAt(fields []string, i int) string {
	if i < 0 || i >= len(fields) {
		return ""
	}
	return fields[i]
}

// ParseLatLon converts NMEA "ddmm.mmmm" + hemisphere to signed decimal degrees.
// Empty value returns (0, false, nil). Bad value returns ErrMalformed.
func ParseLatLon(value, hemi string) (float64, bool, error) {
	if value == "" {
		return 0, false, nil
	}
	dot := strings.IndexByte(value, '.')
	if dot < 2 {
		return 0, false, ErrMalformed
	}
	degStr := value[:dot-2]
	minStr := value[dot-2:]
	deg, err := strconv.ParseFloat(degStr, 64)
	if err != nil {
		return 0, false, ErrMalformed
	}
	min, err := strconv.ParseFloat(minStr, 64)
	if err != nil {
		return 0, false, ErrMalformed
	}
	result := deg + min/60
	switch strings.ToUpper(hemi) {
	case "S", "W":
		result = -result
	case "N", "E", "":
	default:
		return 0, false, ErrMalformed
	}
	return result, true, nil
}

// parseRMCDateTime combines NMEA date "DDMMYY" and time "hhmmss[.sss]" into
// a UTC time.Time. Two-digit years pivot at 80: <80 -> 20xx, >=80 -> 19xx.
func parseRMCDateTime(dateStr, timeStr string) (time.Time, bool) {
	hh, mm, ss, nsec, ok := parseTimeOfDay(timeStr)
	if !ok || len(dateStr) < 6 {
		return time.Time{}, false
	}
	day, err1 := strconv.Atoi(dateStr[0:2])
	month, err2 := strconv.Atoi(dateStr[2:4])
	yy, err3 := strconv.Atoi(dateStr[4:6])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, false
	}
	year := yy + 1900
	if yy < 80 {
		year = yy + 2000
	}
	return time.Date(year, time.Month(month), day, hh, mm, ss, nsec, time.UTC), true
}

// parseTimeOfDay parses NMEA "hhmmss[.sss]" into hour/min/sec/nanosecond.
func parseTimeOfDay(timeStr string) (hh, mm, ss, nsec int, ok bool) {
	if len(timeStr) < 6 {
		return 0, 0, 0, 0, false
	}
	var err1, err2, err3 error
	hh, err1 = strconv.Atoi(timeStr[0:2])
	mm, err2 = strconv.Atoi(timeStr[2:4])
	ss, err3 = strconv.Atoi(timeStr[4:6])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, 0, false
	}
	if dot := strings.IndexByte(timeStr, '.'); dot >= 0 && dot+1 < len(timeStr) {
		frac := timeStr[dot+1:]
		if len(frac) > 3 {
			frac = frac[:3]
		}
		for len(frac) < 3 {
			frac += "0"
		}
		if ms, err := strconv.Atoi(frac); err == nil {
			nsec = ms * 1e6
		}
	}
	return hh, mm, ss, nsec, true
}

// combineDateAndTimeOfDay applies an "hhmmss[.sss]" time-of-day field to the
// UTC calendar date of `today`. Used as the fallback Time source when only
// GGA (no RMC) is available in a cycle.
func combineDateAndTimeOfDay(timeStr string, today time.Time) (time.Time, bool) {
	hh, mm, ss, nsec, ok := parseTimeOfDay(timeStr)
	if !ok {
		return time.Time{}, false
	}
	today = today.UTC()
	return time.Date(today.Year(), today.Month(), today.Day(), hh, mm, ss, nsec, time.UTC), true
}

func parseFloatOrZero(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// ParseGGA decodes a GGA sentence into a partial Fix. Absent optional fields
// leave the corresponding Fix field zero and its Has* flag false. Mode is
// derived from the fix-quality field per the GGA fallback rule (used when no
// GSA is available in the same cycle): 0 -> ModeNoFix; other qualities ->
// Mode3D when altitude is present, else Mode2D.
func ParseGGA(fields []string) (Fix, error) {
	if len(fields) < 3 || !strings.HasSuffix(fields[0], "GGA") {
		return Fix{}, ErrUnsupported
	}
	if len(fields) < 10 {
		return Fix{}, ErrMalformed
	}

	var f Fix

	lat, latOK, err := ParseLatLon(fieldAt(fields, 2), fieldAt(fields, 3))
	if err != nil {
		return Fix{}, err
	}
	lon, lonOK, err := ParseLatLon(fieldAt(fields, 4), fieldAt(fields, 5))
	if err != nil {
		return Fix{}, err
	}
	if latOK && lonOK {
		f.Lat, f.Lon = lat, lon
	}

	quality := 0
	if q := fieldAt(fields, 6); q != "" {
		qi, err := strconv.Atoi(q)
		if err != nil {
			return Fix{}, ErrMalformed
		}
		quality = qi
	}

	if sat := fieldAt(fields, 7); sat != "" {
		n, err := strconv.Atoi(sat)
		if err != nil {
			return Fix{}, ErrMalformed
		}
		f.Satellites = n
	}
	if hd := fieldAt(fields, 8); hd != "" {
		v, err := strconv.ParseFloat(hd, 64)
		if err != nil {
			return Fix{}, ErrMalformed
		}
		f.HDOP = v
	}
	if alt := fieldAt(fields, 9); alt != "" {
		v, err := strconv.ParseFloat(alt, 64)
		if err != nil {
			return Fix{}, ErrMalformed
		}
		f.Altitude = v
		f.HasAltitude = true
	}

	if quality == 0 {
		f.Mode = ModeNoFix
	} else if f.HasAltitude {
		f.Mode = Mode3D
	} else {
		f.Mode = Mode2D
	}

	return f, nil
}

// ParseRMC decodes an RMC sentence into a partial Fix. HasCourse is true
// only when the course field is present and SpeedKnots >= 1.0 (course is
// jittery/meaningless near a stop). Status "V" (void) forces ModeNoFix and
// zeroes lat/lon even if the position fields happen to be populated.
func ParseRMC(fields []string) (Fix, error) {
	if len(fields) < 3 || !strings.HasSuffix(fields[0], "RMC") {
		return Fix{}, ErrUnsupported
	}
	if len(fields) < 10 {
		return Fix{}, ErrMalformed
	}

	var f Fix

	lat, latOK, err := ParseLatLon(fieldAt(fields, 3), fieldAt(fields, 4))
	if err != nil {
		return Fix{}, err
	}
	lon, lonOK, err := ParseLatLon(fieldAt(fields, 5), fieldAt(fields, 6))
	if err != nil {
		return Fix{}, err
	}
	if latOK && lonOK {
		f.Lat, f.Lon = lat, lon
	}

	if sp := fieldAt(fields, 7); sp != "" {
		v, err := strconv.ParseFloat(sp, 64)
		if err != nil {
			return Fix{}, ErrMalformed
		}
		f.SpeedKnots = v
	}

	courseField := fieldAt(fields, 8)
	if courseField != "" {
		v, err := strconv.ParseFloat(courseField, 64)
		if err != nil {
			return Fix{}, ErrMalformed
		}
		f.Course = v
		if f.SpeedKnots >= 1.0 {
			f.HasCourse = true
		}
	}

	if t, ok := parseRMCDateTime(fieldAt(fields, 9), fieldAt(fields, 1)); ok {
		f.Time = t
	}

	status := strings.ToUpper(fieldAt(fields, 2))
	if status == "V" {
		f.Mode = ModeNoFix
		f.Lat, f.Lon = 0, 0
	}

	return f, nil
}

// ParseGSA decodes a GSA sentence's fix mode and dilution-of-precision values.
func ParseGSA(fields []string) (mode FixMode, pdop, hdop, vdop float64, err error) {
	if len(fields) < 3 || !strings.HasSuffix(fields[0], "GSA") {
		return ModeUnknown, 0, 0, 0, ErrUnsupported
	}
	if len(fields) < 18 {
		return ModeUnknown, 0, 0, 0, ErrMalformed
	}
	modeInt, convErr := strconv.Atoi(fieldAt(fields, 2))
	if convErr != nil {
		return ModeUnknown, 0, 0, 0, ErrMalformed
	}
	mode = FixMode(modeInt)
	pdop = parseFloatOrZero(fieldAt(fields, 15))
	hdop = parseFloatOrZero(fieldAt(fields, 16))
	vdop = parseFloatOrZero(fieldAt(fields, 17))
	return mode, pdop, hdop, vdop, nil
}

// Assembler merges GGA/RMC/GSA sentences from one NMEA cycle into a complete
// Fix. It emits once a cycle has both a GGA and an RMC sentence (the two
// sentence types that carry position); GSA, when present in the same cycle,
// enriches Mode/HDOP. A repeat of an already-seen sentence type, or a time
// field that has moved on, flushes (and discards, if incomplete) whatever
// was accumulated and starts a fresh cycle — this bounds memory and latency
// for receivers that don't always send a full GGA+GSA+RMC triad.
type Assembler struct {
	now func() time.Time

	haveGGA, haveRMC, haveGSA bool
	gga                       Fix
	rmc                       Fix
	gsaMode                   FixMode
	gsaHDOP                   float64
	cycleTimeField            string
	ggaTimeField              string

	checksumErrors int
}

// NewAssembler creates an Assembler. now supplies ReceivedAt (and the
// calendar date used to complete a GGA-only time-of-day); nil uses time.Now.
func NewAssembler(now func() time.Time) *Assembler {
	if now == nil {
		now = time.Now
	}
	return &Assembler{now: now}
}

// ChecksumErrors returns the count of checksum failures seen so far.
func (a *Assembler) ChecksumErrors() int { return a.checksumErrors }

func (a *Assembler) reset() {
	a.haveGGA, a.haveRMC, a.haveGSA = false, false, false
	a.gga, a.rmc = Fix{}, Fix{}
	a.gsaMode = ModeUnknown
	a.gsaHDOP = 0
	a.cycleTimeField = ""
	a.ggaTimeField = ""
}

func (a *Assembler) finalize() Fix {
	var f Fix

	if a.haveGGA {
		f.Lat, f.Lon = a.gga.Lat, a.gga.Lon
		f.Altitude, f.HasAltitude = a.gga.Altitude, a.gga.HasAltitude
		f.Satellites = a.gga.Satellites
		f.HDOP = a.gga.HDOP
		f.Mode = a.gga.Mode
	}
	if a.haveRMC {
		if !a.haveGGA || a.rmc.Mode == ModeNoFix {
			f.Lat, f.Lon = a.rmc.Lat, a.rmc.Lon
		}
		f.SpeedKnots = a.rmc.SpeedKnots
		f.Course = a.rmc.Course
		f.HasCourse = a.rmc.HasCourse
		if a.rmc.Mode == ModeNoFix {
			f.Mode = ModeNoFix
		}
	}
	if a.haveGSA {
		f.Mode = a.gsaMode
		if a.gsaHDOP > 0 {
			f.HDOP = a.gsaHDOP
		}
	}
	if f.HDOP > 0 {
		f.Accuracy = f.HDOP * 5.0
	}

	if a.haveRMC && !a.rmc.Time.IsZero() {
		f.Time = a.rmc.Time
	} else if a.haveGGA && a.ggaTimeField != "" {
		if t, ok := combineDateAndTimeOfDay(a.ggaTimeField, a.now()); ok {
			f.Time = t
		}
	}

	f.ReceivedAt = a.now()
	return f
}

// Consume feeds one raw sentence. Returns (fix, true) when a cycle completed.
// Unsupported sentences (GSV, TXT, VTG, GLL…) return (Fix{}, false, nil).
// Checksum failures return (Fix{}, false, ErrChecksum) and are counted, not
// fatal — accumulator state is left untouched so a following good cycle
// still assembles correctly.
func (a *Assembler) Consume(sentence string) (Fix, bool, error) {
	sentence = strings.TrimSpace(sentence)
	if len(sentence) > 128 {
		return Fix{}, false, ErrMalformed
	}
	if idx := strings.IndexByte(sentence, '$'); idx != 0 {
		return Fix{}, false, ErrMalformed
	}
	if strings.IndexByte(sentence[1:], '$') >= 0 {
		return Fix{}, false, ErrMalformed
	}

	fields, err := Fields(sentence)
	if err != nil {
		if errors.Is(err, ErrChecksum) {
			a.checksumErrors++
		}
		return Fix{}, false, err
	}
	if len(fields) == 0 || len(fields[0]) < 3 {
		return Fix{}, false, ErrMalformed
	}

	kind := strings.ToUpper(fields[0])
	kind = kind[len(kind)-3:]

	var timeField string
	switch kind {
	case "GGA", "RMC":
		timeField = fieldAt(fields, 1)
	case "GSA":
		// no time field of its own
	default:
		return Fix{}, false, nil
	}

	isRepeat := (kind == "GGA" && a.haveGGA) || (kind == "RMC" && a.haveRMC) || (kind == "GSA" && a.haveGSA)
	isTimeChange := timeField != "" && a.cycleTimeField != "" && timeField != a.cycleTimeField

	var emitted Fix
	didEmit := false
	if isRepeat || isTimeChange {
		if a.haveGGA && a.haveRMC {
			emitted = a.finalize()
			didEmit = true
		}
		a.reset()
	}

	if timeField != "" && a.cycleTimeField == "" {
		a.cycleTimeField = timeField
	}

	switch kind {
	case "GGA":
		fix, perr := ParseGGA(fields)
		if perr != nil {
			return Fix{}, false, perr
		}
		a.gga = fix
		a.haveGGA = true
		a.ggaTimeField = timeField
	case "RMC":
		fix, perr := ParseRMC(fields)
		if perr != nil {
			return Fix{}, false, perr
		}
		a.rmc = fix
		a.haveRMC = true
	case "GSA":
		mode, _, hdop, _, perr := ParseGSA(fields)
		if perr != nil {
			return Fix{}, false, perr
		}
		a.gsaMode = mode
		a.gsaHDOP = hdop
		a.haveGSA = true
	}

	if !didEmit && a.haveGGA && a.haveRMC {
		emitted = a.finalize()
		didEmit = true
		a.reset()
	}

	if didEmit {
		return emitted, true, nil
	}
	return Fix{}, false, nil
}

// ConsumeSnapshot feeds a complete, already-grouped set of sentences that are
// known to belong to ONE cycle — e.g. one ModemManager Location NMEA blob,
// which caches exactly one sentence of each type. Unlike Consume it does not
// apply the repeat/time-change cycle heuristics (a snapshot's per-type
// sentences may carry slightly different time fields, which would make
// Consume reset and never emit) and it emits when EITHER a GGA or an RMC was
// present, not only when both were. Returns (fix, true, nil) when a fix was
// assembled. Per-sentence parse and checksum failures are counted and
// skipped, never fatal.
func (a *Assembler) ConsumeSnapshot(sentences []string) (Fix, bool, error) {
	a.reset()
	sawValidRMCPosition := false

	for _, raw := range sentences {
		s := strings.TrimSpace(raw)
		if s == "" || len(s) > 128 || s[0] != '$' {
			continue
		}
		if strings.IndexByte(s[1:], '$') >= 0 {
			continue
		}

		fields, err := Fields(s)
		if err != nil {
			if errors.Is(err, ErrChecksum) {
				a.checksumErrors++
			}
			continue
		}
		if len(fields) == 0 || len(fields[0]) < 3 {
			continue
		}
		kind := strings.ToUpper(fields[0])
		kind = kind[len(kind)-3:]

		switch kind {
		case "GGA":
			if f, perr := ParseGGA(fields); perr == nil {
				a.gga = f
				a.haveGGA = true
				a.ggaTimeField = fieldAt(fields, 1)
			}
		case "RMC":
			if f, perr := ParseRMC(fields); perr == nil {
				a.rmc = f
				a.haveRMC = true
				if strings.ToUpper(fieldAt(fields, 2)) == "A" && fieldAt(fields, 3) != "" && fieldAt(fields, 5) != "" {
					sawValidRMCPosition = true
				}
			}
		case "GSA":
			if mode, _, hdop, _, perr := ParseGSA(fields); perr == nil {
				a.gsaMode = mode
				a.gsaHDOP = hdop
				a.haveGSA = true
			}
		}
	}

	if !a.haveGGA && !a.haveRMC {
		a.reset()
		return Fix{}, false, nil
	}

	f := a.finalize()
	// RMC alone carries no fix-quality field; ParseRMC only sets Mode for
	// status "V". A valid RMC with a position is at least a 2D fix.
	if f.Mode == ModeUnknown && sawValidRMCPosition {
		f.Mode = Mode2D
	}
	a.reset()
	return f, true, nil
}

// NMEAConfig configures an NMEA-0183 source: either a serial device or a
// TCP host, never both.
type NMEAConfig struct {
	Device string // serial device path; mutually exclusive with Host
	Baud   int    // default 9600
	Host   string // TCP host; mutually exclusive with Device
	Port   int
}

// nmeaDialer opens the underlying byte stream for an NMEA source. Replaced
// in tests.
type nmeaDialer func(NMEAConfig) (io.ReadCloser, error)

func defaultNMEADialer(cfg NMEAConfig) (io.ReadCloser, error) {
	if cfg.Device != "" {
		baud := cfg.Baud
		if baud <= 0 {
			baud = 9600
		}
		port, err := serial.Open(cfg.Device, &serial.Mode{BaudRate: baud})
		if err != nil {
			return nil, err
		}
		return port, nil
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// NMEASource is a Source that reads NMEA-0183 sentences from a serial
// device or a TCP host and assembles them into Fixes.
type NMEASource struct {
	cfg   NMEAConfig
	dial  nmeaDialer
	fixes chan Fix

	mu     sync.Mutex
	status SourceStatus
	cancel context.CancelFunc
}

// NewNMEASource creates an NMEASource. Call Start to begin reading.
func NewNMEASource(cfg NMEAConfig) *NMEASource {
	return &NMEASource{
		cfg:    cfg,
		dial:   defaultNMEADialer,
		fixes:  make(chan Fix, 8),
		status: SourceStatus{Type: "nmea", Target: nmeaTarget(cfg)},
	}
}

func nmeaTarget(cfg NMEAConfig) string {
	if cfg.Device != "" {
		baud := cfg.Baud
		if baud <= 0 {
			baud = 9600
		}
		return fmt.Sprintf("%s@%d", cfg.Device, baud)
	}
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

// Start begins the connect/read/reconnect loop. Returns nil once the loop
// goroutine is running.
func (s *NMEASource) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer close(s.fixes)
		runLoop(ctx,
			func() (io.ReadCloser, error) { return s.dial(s.cfg) },
			s.readLoop,
			s.setStatus,
		)
	}()
	return nil
}

func (s *NMEASource) readLoop(ctx context.Context, rc io.ReadCloser) error {
	kick, stop := idleWatchdog(rc, readDeadline)
	defer stop()

	asm := NewAssembler(time.Now)
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 4096), 4096)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		kick()
		s.mu.Lock()
		s.status.LastRx = time.Now()
		s.mu.Unlock()

		fix, ok, err := asm.Consume(line)
		if err != nil {
			continue
		}
		if ok {
			select {
			case s.fixes <- fix:
			default:
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func (s *NMEASource) setStatus(connected bool, errMsg string) {
	s.mu.Lock()
	s.status.Connected = connected
	s.status.Error = errMsg
	s.mu.Unlock()
}

// Fixes returns the channel of assembled Fixes.
func (s *NMEASource) Fixes() <-chan Fix { return s.fixes }

// Status returns the current connection status.
func (s *NMEASource) Status() SourceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Close stops the read/reconnect loop.
func (s *NMEASource) Close() error {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
