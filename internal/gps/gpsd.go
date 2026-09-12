package gps

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GPSDConfig configures a connection to a gpsd daemon.
type GPSDConfig struct {
	Host string // default "127.0.0.1"
	Port int    // default 2947
}

func (c GPSDConfig) resolved() GPSDConfig {
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port == 0 {
		c.Port = 2947
	}
	return c
}

// gpsdTPV is the wire struct for a gpsd TPV report. gpsd >= 3.20 replaced
// "alt" with "altMSL"/"altHAE"; all three are accepted, preferring altMSL,
// then alt, then altHAE.
type gpsdTPV struct {
	Class  string   `json:"class"`
	Mode   int      `json:"mode"`
	Time   string   `json:"time"`
	Lat    *float64 `json:"lat"`
	Lon    *float64 `json:"lon"`
	Alt    *float64 `json:"alt"`
	AltMSL *float64 `json:"altMSL"`
	AltHAE *float64 `json:"altHAE"`
	Track  *float64 `json:"track"` // degrees true
	Speed  *float64 `json:"speed"` // meters/second
	Eph    *float64 `json:"eph"`   // horizontal error, meters (95%)
	Epx    *float64 `json:"epx"`
	Epy    *float64 `json:"epy"`
}

// ParseTPV converts one TPV object to a Fix. now supplies ReceivedAt.
func ParseTPV(data []byte, now time.Time) (Fix, error) {
	var tpv gpsdTPV
	if err := json.Unmarshal(data, &tpv); err != nil {
		return Fix{}, ErrMalformed
	}

	var f Fix
	f.Mode = FixMode(tpv.Mode)

	if f.Mode >= Mode2D {
		if tpv.Lat == nil || tpv.Lon == nil {
			return Fix{}, ErrMalformed
		}
		f.Lat, f.Lon = *tpv.Lat, *tpv.Lon
	} else {
		if tpv.Lat != nil {
			f.Lat = *tpv.Lat
		}
		if tpv.Lon != nil {
			f.Lon = *tpv.Lon
		}
	}

	switch {
	case tpv.AltMSL != nil:
		f.Altitude, f.HasAltitude = *tpv.AltMSL, true
	case tpv.Alt != nil:
		f.Altitude, f.HasAltitude = *tpv.Alt, true
	case tpv.AltHAE != nil:
		f.Altitude, f.HasAltitude = *tpv.AltHAE, true
	}

	if tpv.Speed != nil {
		f.SpeedKnots = math.Round(*tpv.Speed*MetersPerSecondToKnots*1000) / 1000
	}
	if tpv.Track != nil {
		f.Course = *tpv.Track
		if f.SpeedKnots >= 1.0 {
			f.HasCourse = true
		}
	}

	switch {
	case tpv.Eph != nil:
		f.Accuracy = *tpv.Eph
	case tpv.Epx != nil && tpv.Epy != nil:
		f.Accuracy = math.Hypot(*tpv.Epx, *tpv.Epy)
	}

	if tpv.Time != "" {
		if t, err := time.Parse(time.RFC3339, tpv.Time); err == nil {
			// gpsd forwards whatever date its receiver computed; an old
			// receiver behind gpsd is just as capable of a GPS week-number
			// rollover as one read directly, so apply the same clamp used
			// for the NMEA sources (see gpsTimeSkewLimit in nmea.go).
			f.Time = clampGPSTime(t, now)
		}
	}

	f.ReceivedAt = now
	return f, nil
}

// gpsdEnvelope reads just enough of a gpsd JSON line to dispatch on class.
type gpsdEnvelope struct {
	Class string `json:"class"`
}

// gpsdSKY carries the used-satellite count from a SKY report.
type gpsdSKY struct {
	Class string `json:"class"`
	USat  *int   `json:"uSat"`
}

// gpsdConn is the read/write/close surface GPSDSource needs from a
// connection; satisfied by net.Conn and by test fakes.
type gpsdConn interface {
	io.Reader
	io.Writer
	io.Closer
}

// gpsdDialer opens a connection to gpsd. Replaced in tests.
type gpsdDialer func(GPSDConfig) (gpsdConn, error)

func defaultGPSDDialer(cfg GPSDConfig) (gpsdConn, error) {
	cfg = cfg.resolved()
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

const gpsdWatchCommand = `?WATCH={"enable":true,"json":true};` + "\n"

// GPSDSource is a Source that speaks the gpsd JSON (WATCH) protocol.
type GPSDSource struct {
	cfg   GPSDConfig
	dial  gpsdDialer
	fixes chan Fix

	mu          sync.Mutex
	status      SourceStatus
	satellites  int
	errorLogged bool
	cancel      context.CancelFunc
}

// NewGPSDSource creates a GPSDSource. Call Start to begin reading.
func NewGPSDSource(cfg GPSDConfig) *GPSDSource {
	cfg = cfg.resolved()
	return &GPSDSource{
		cfg:    cfg,
		dial:   defaultGPSDDialer,
		fixes:  make(chan Fix, 8),
		status: SourceStatus{Type: "gpsd", Target: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
	}
}

// Start begins the connect/watch/read/reconnect loop.
func (s *GPSDSource) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer close(s.fixes)
		runLoop(ctx,
			func() (gpsdConn, error) { return s.dial(s.cfg) },
			s.readLoop,
			s.setStatus,
		)
	}()
	return nil
}

func (s *GPSDSource) readLoop(ctx context.Context, conn gpsdConn) error {
	kick, stop := idleWatchdog(conn, readDeadline)
	defer stop()

	if _, err := conn.Write([]byte(gpsdWatchCommand)); err != nil {
		return err
	}

	s.mu.Lock()
	s.errorLogged = false
	s.mu.Unlock()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		kick()
		s.mu.Lock()
		s.status.LastRx = time.Now()
		s.mu.Unlock()

		var env gpsdEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}

		switch env.Class {
		case "TPV":
			fix, err := ParseTPV(line, time.Now())
			if err != nil {
				continue
			}
			s.mu.Lock()
			fix.Satellites = s.satellites
			s.mu.Unlock()
			select {
			case s.fixes <- fix:
			default:
			}
		case "SKY":
			var sky gpsdSKY
			if json.Unmarshal(line, &sky) == nil && sky.USat != nil {
				s.mu.Lock()
				s.satellites = *sky.USat
				s.mu.Unlock()
			}
		case "ERROR":
			s.mu.Lock()
			already := s.errorLogged
			s.errorLogged = true
			s.mu.Unlock()
			if !already {
				log.Printf("[gps] gpsd error: %s", strings.TrimSpace(string(line)))
			}
		default:
			// VERSION, DEVICES, WATCH, and anything else: ignore.
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func (s *GPSDSource) setStatus(connected bool, errMsg string) {
	s.mu.Lock()
	s.status.Connected = connected
	s.status.Error = errMsg
	s.mu.Unlock()
}

// Fixes returns the channel of parsed Fixes.
func (s *GPSDSource) Fixes() <-chan Fix { return s.fixes }

// Status returns the current connection status.
func (s *GPSDSource) Status() SourceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Close stops the read/reconnect loop.
func (s *GPSDSource) Close() error {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
