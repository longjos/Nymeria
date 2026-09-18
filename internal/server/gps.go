package server

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/narvel/nymeria/internal/gps"
)

// handleGetGPS always returns 200 — GPS disabled or unconfigured returns the
// disabled shape, never 404/503, so the frontend can poll unconditionally.
func (s *Server) handleGetGPS(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.gpsStatus())
}

// gpsStatus returns the disabled-shaped Status when s.gpsMgr is nil.
func (s *Server) gpsStatus() gps.Status {
	if s.gpsMgr == nil {
		return gps.Status{Enabled: false, Connected: false, Fix: nil, AgeMillis: nil, Stale: true}
	}
	return s.gpsMgr.Status()
}

// gpsSignature is what counts as "changed" for own_position broadcast
// coalescing. ageMillis is deliberately excluded — the frontend derives age
// from Fix.receivedAt locally, so age drift alone never generates traffic.
type gpsSignature struct {
	enabled, connected, stale bool
	errText                   string
	mode                      int
	lat, lon                  int64 // round(value * 1e6)
	speed                     int64 // round(knots * 10)
	course                    int64 // round(degrees)
	hasFix                    bool
}

func signatureOf(st gps.Status) gpsSignature {
	sig := gpsSignature{
		enabled:   st.Enabled,
		connected: st.Connected,
		stale:     st.Stale,
		errText:   st.Error,
	}
	if st.Fix != nil {
		sig.hasFix = true
		sig.mode = int(st.Fix.Mode)
		sig.lat = int64(math.Round(st.Fix.Lat * 1e6))
		sig.lon = int64(math.Round(st.Fix.Lon * 1e6))
		sig.speed = int64(math.Round(st.Fix.SpeedKnots * 10))
		sig.course = int64(math.Round(st.Fix.Course))
	}
	return sig
}

// bridgeGPS watches the GPS manager's fix stream and broadcasts coalesced
// own_position WebSocket frames: rate-limited to at most 1/s, only when the
// signature actually changed, with a keepalive resync every 10s so a client
// that missed a frame (or just connected) converges on connection-state
// changes too.
func (s *Server) bridgeGPS() {
	fixes, unsub := s.gpsMgr.Subscribe()
	defer unsub()

	const minInterval = 1 * time.Second
	const keepalive = 10 * time.Second

	var lastSent gpsSignature
	var lastAt time.Time
	pending := false

	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	ka := time.NewTicker(keepalive)
	defer ka.Stop()

	emit := func(force bool) {
		st := s.gpsStatus()
		sig := signatureOf(st)
		if !force && sig == lastSent {
			return
		}
		if time.Since(lastAt) < minInterval {
			pending = true
			return
		}
		data, err := json.Marshal(map[string]any{"type": "own_position", "gps": st})
		if err != nil {
			log.Printf("[server] marshal own_position: %v", err)
			return
		}
		s.hub.Broadcast(data)
		lastSent, lastAt, pending = sig, time.Now(), false
		s.TriggerWxFootprintRefresh()
	}

	for {
		select {
		case _, ok := <-fixes:
			if !ok {
				return
			}
			emit(false)
		case <-tick.C:
			if pending {
				emit(false)
			}
		case <-ka.C:
			emit(true)
		}
	}
}
