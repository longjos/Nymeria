package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/transport/aprsis"
)

// setupRequest is the JSON body for POST /api/setup.
type setupRequest struct {
	Callsign    string  `json:"callsign"`
	SSID        int     `json:"ssid"`
	Comment     string  `json:"comment"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	APRISEnable bool    `json:"aprisEnabled"`
	APRISHost   string  `json:"aprisHost"`
	APRISPort   int     `json:"aprisPort"`
	APRISFilter string  `json:"aprisFilter"`
}

var callsignRe = regexp.MustCompile(`^[A-Z0-9]{3,9}$`)

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	// Self-disabling: refuse if already configured.
	cfg := s.configMgr.Get()
	if cfg.Station.Callsign != "N0CALL" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "setup already complete"})
		return
	}

	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate callsign
	call := strings.ToUpper(strings.TrimSpace(req.Callsign))
	if !callsignRe.MatchString(call) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid callsign"})
		return
	}

	// Validate lat/lon
	if req.Lat < -90 || req.Lat > 90 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat must be between -90 and 90"})
		return
	}
	if req.Lon < -180 || req.Lon > 180 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lon must be between -180 and 180"})
		return
	}

	// Validate SSID
	if req.SSID < 0 || req.SSID > 15 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ssid must be 0-15"})
		return
	}

	// Apply wizard values to a copy of the current config (preserving defaults).
	cfg.Station.Callsign = call
	cfg.Station.SSID = req.SSID
	cfg.Station.Comment = req.Comment
	cfg.Station.Lat = req.Lat
	cfg.Station.Lon = req.Lon

	// Build transports
	cfg.Transports = nil
	if req.APRISEnable {
		host := req.APRISHost
		if host == "" {
			host = "rotate.aprs2.net"
		}
		port := req.APRISPort
		if port == 0 {
			port = 14580
		}
		passcode := fmt.Sprintf("%d", aprsis.Passcode(call))

		cfg.Transports = append(cfg.Transports, transport.TransportConfig{
			Type:     "aprsis",
			Host:     host,
			Port:     port,
			Filter:   req.APRISFilter,
			Callsign: call,
			Passcode: passcode,
		})
	}

	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"restartRequired": true,
	})
}
