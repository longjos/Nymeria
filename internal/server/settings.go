package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/transport"
	"github.com/narvel/nymeria/internal/transport/kisstcp"
	"github.com/narvel/nymeria/internal/transport/serial"
)

// --- DTO types for duration serialization ---

type stationDTO struct {
	Callsign        string            `json:"callsign"`
	SSID            int               `json:"ssid"`
	Lat             float64           `json:"lat"`
	Lon             float64           `json:"lon"`
	SymbolTable     string            `json:"symbolTable"`
	SymbolCode      string            `json:"symbolCode"`
	Comment         string            `json:"comment"`
	TrackMaxPoints  int               `json:"trackMaxPoints"`
	StaleTimeout    string            `json:"staleTimeout"`
	DedupWindow     string            `json:"dedupWindow"`
	TacticalAliases map[string]string `json:"tacticalAliases,omitempty"`
	MessagePath     string            `json:"messagePath"`
	BeaconPath      string            `json:"beaconPath"`
}

type serverDTO struct {
	Listen string `json:"listen"`
}

type beaconDTO struct {
	Enabled     bool            `json:"enabled"`
	Interval    string          `json:"interval"`
	Comment     string          `json:"comment"`
	SmartBeacon *smartBeaconDTO `json:"smartBeacon,omitempty"`
}

type smartBeaconDTO struct {
	Enabled   bool    `json:"enabled"`
	FastSpeed float64 `json:"fastSpeed"`
	SlowSpeed float64 `json:"slowSpeed"`
	FastRate  string  `json:"fastRate"`
	SlowRate  string  `json:"slowRate"`
	TurnAngle float64 `json:"turnAngle"`
	TurnSlope float64 `json:"turnSlope"`
}

type gpsDTO struct {
	Enabled      bool   `json:"enabled"`
	Type         string `json:"type"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Device       string `json:"device"`
	Baud         int    `json:"baud"`
	MinInterval  string `json:"minInterval"` // Go duration string, e.g. "1s"
	StaleAfter   string `json:"staleAfter"`
	UseForBeacon bool   `json:"useForBeacon"`
}

type sessionDTO struct {
	PINConfigured     bool   `json:"pinConfigured"`
	PIN               string `json:"pin,omitempty"`
	InactivityTimeout string `json:"inactivityTimeout"`
}

type loggingDTO struct {
	Level string `json:"level"`
}

type transportDTO struct {
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Device   string `json:"device,omitempty"`
	Baud     int    `json:"baud,omitempty"`
	Filter   string `json:"filter,omitempty"`
	Callsign string `json:"callsign,omitempty"`
	Passcode string `json:"passcode,omitempty"`
}

type tileCacheDTO struct {
	Enabled bool   `json:"enabled"`
	DataDir string `json:"dataDir"`
	TileURL string `json:"tileUrl"`
	MaxZoom int    `json:"maxZoom"`
}

type storeDTO struct {
	Path string `json:"path"`
}

type what3wordsDTO struct {
	Enabled          bool   `json:"enabled"`
	APIKeyConfigured bool   `json:"apiKeyConfigured"`
	APIKeySource     string `json:"apiKeySource"`     // "env" | "config" | "none"
	APIKey           string `json:"apiKey,omitempty"` // write-only; never populated on GET
	BaseURL          string `json:"baseUrl"`
	Results          int    `json:"results"`
}

type weatherDTO struct {
	RetentionDays int                                     `json:"retentionDays"`
	Alerts        map[string]config.WeatherAlertThreshold `json:"alerts,omitempty"`
	Units         string                                  `json:"units"`
}

type settingsResponse struct {
	Station    stationDTO     `json:"station"`
	Server     serverDTO      `json:"server"`
	Beacon     beaconDTO      `json:"beacon"`
	Session    sessionDTO     `json:"session"`
	Logging    loggingDTO     `json:"logging"`
	Transports []transportDTO `json:"transports"`
	TileCache  tileCacheDTO   `json:"tileCache"`
	Weather    weatherDTO     `json:"weather"`
	Store      storeDTO       `json:"store"`
	GPS        gpsDTO         `json:"gps"`
	What3Words what3wordsDTO  `json:"what3words"`
}

type updateResponse struct {
	RestartRequired bool `json:"restartRequired"`
}

type serialPortsResponse struct {
	HostOS    string            `json:"hostOS"`
	Ports     []serial.PortInfo `json:"ports"`
	Profiles  []serial.Profile  `json:"profiles"`
	BaudRates []int             `json:"baudRates"`
	Error     string            `json:"error,omitempty"`
}

type kissTNCsResponse struct {
	HostOS string            `json:"hostOS"`
	TNCs   []kisstcp.TNCInfo `json:"tncs"`
	Error  string            `json:"error,omitempty"`
}

// --- DTO converters ---

func toStationDTO(c config.StationConfig) stationDTO {
	return stationDTO{
		Callsign:        c.Callsign,
		SSID:            c.SSID,
		Lat:             c.Lat,
		Lon:             c.Lon,
		SymbolTable:     c.SymbolTable,
		SymbolCode:      c.SymbolCode,
		Comment:         c.Comment,
		TrackMaxPoints:  c.TrackMaxPoints,
		StaleTimeout:    c.StaleTimeout.String(),
		DedupWindow:     c.DedupWindow.String(),
		TacticalAliases: c.TacticalAliases,
		MessagePath:     c.MessagePath,
		BeaconPath:      c.BeaconPath,
	}
}

func fromStationDTO(d stationDTO) (config.StationConfig, error) {
	stale, err := time.ParseDuration(d.StaleTimeout)
	if err != nil && d.StaleTimeout != "" {
		return config.StationConfig{}, err
	}
	dedup, err := time.ParseDuration(d.DedupWindow)
	if err != nil && d.DedupWindow != "" {
		return config.StationConfig{}, err
	}
	return config.StationConfig{
		Callsign:        d.Callsign,
		SSID:            d.SSID,
		Lat:             d.Lat,
		Lon:             d.Lon,
		SymbolTable:     d.SymbolTable,
		SymbolCode:      d.SymbolCode,
		Comment:         d.Comment,
		TrackMaxPoints:  d.TrackMaxPoints,
		StaleTimeout:    stale,
		DedupWindow:     dedup,
		TacticalAliases: d.TacticalAliases,
		MessagePath:     d.MessagePath,
		BeaconPath:      d.BeaconPath,
	}, nil
}

func toBeaconDTO(c config.BeaconConfig) beaconDTO {
	d := beaconDTO{
		Enabled:  c.Enabled,
		Interval: c.Interval.String(),
		Comment:  c.Comment,
	}
	if c.SmartBeacon != nil {
		sb := c.SmartBeacon
		d.SmartBeacon = &smartBeaconDTO{
			Enabled:   sb.Enabled,
			FastSpeed: sb.FastSpeed,
			SlowSpeed: sb.SlowSpeed,
			FastRate:  sb.FastRate.String(),
			SlowRate:  sb.SlowRate.String(),
			TurnAngle: sb.TurnAngle,
			TurnSlope: sb.TurnSlope,
		}
	}
	return d
}

func fromBeaconDTO(d beaconDTO) (config.BeaconConfig, error) {
	interval, err := time.ParseDuration(d.Interval)
	if err != nil && d.Interval != "" {
		return config.BeaconConfig{}, err
	}
	cfg := config.BeaconConfig{
		Enabled:  d.Enabled,
		Interval: interval,
		Comment:  d.Comment,
	}
	if d.SmartBeacon != nil {
		fastRate, err := time.ParseDuration(d.SmartBeacon.FastRate)
		if err != nil && d.SmartBeacon.FastRate != "" {
			return config.BeaconConfig{}, err
		}
		slowRate, err := time.ParseDuration(d.SmartBeacon.SlowRate)
		if err != nil && d.SmartBeacon.SlowRate != "" {
			return config.BeaconConfig{}, err
		}
		cfg.SmartBeacon = &config.SmartBeaconConfig{
			Enabled:   d.SmartBeacon.Enabled,
			FastSpeed: d.SmartBeacon.FastSpeed,
			SlowSpeed: d.SmartBeacon.SlowSpeed,
			FastRate:  fastRate,
			SlowRate:  slowRate,
			TurnAngle: d.SmartBeacon.TurnAngle,
			TurnSlope: d.SmartBeacon.TurnSlope,
		}
	}
	return cfg, nil
}

func toGPSDTO(c config.GPSConfig) gpsDTO {
	return gpsDTO{
		Enabled:      c.Enabled,
		Type:         c.Type,
		Host:         c.Host,
		Port:         c.Port,
		Device:       c.Device,
		Baud:         c.Baud,
		MinInterval:  c.MinInterval.String(),
		StaleAfter:   c.StaleAfter.String(),
		UseForBeacon: c.UseForBeacon,
	}
}

func fromGPSDTO(d gpsDTO) (config.GPSConfig, error) {
	var minInterval, staleAfter time.Duration
	var err error
	if d.MinInterval != "" {
		if minInterval, err = time.ParseDuration(d.MinInterval); err != nil {
			return config.GPSConfig{}, err
		}
	}
	if d.StaleAfter != "" {
		if staleAfter, err = time.ParseDuration(d.StaleAfter); err != nil {
			return config.GPSConfig{}, err
		}
	}
	return config.GPSConfig{
		Enabled:      d.Enabled,
		Type:         d.Type,
		Host:         d.Host,
		Port:         d.Port,
		Device:       d.Device,
		Baud:         d.Baud,
		MinInterval:  minInterval,
		StaleAfter:   staleAfter,
		UseForBeacon: d.UseForBeacon,
	}, nil
}

func toSessionDTO(c config.SessionConfig) sessionDTO {
	return sessionDTO{
		PINConfigured:     c.PIN != "",
		InactivityTimeout: c.InactivityTimeout.String(),
	}
}

func fromSessionDTO(d sessionDTO, existing config.SessionConfig) (config.SessionConfig, error) {
	timeout, err := time.ParseDuration(d.InactivityTimeout)
	if err != nil && d.InactivityTimeout != "" {
		return config.SessionConfig{}, err
	}
	pin := d.PIN
	// Preserve existing PIN if client sends empty or masked value
	if pin == "" || pin == "***" {
		pin = existing.PIN
	}
	return config.SessionConfig{
		PIN:               pin,
		InactivityTimeout: timeout,
	}, nil
}

func toTransportDTOs(configs []transport.TransportConfig) []transportDTO {
	result := make([]transportDTO, len(configs))
	for i, c := range configs {
		result[i] = transportDTO{
			Type:     c.Type,
			Name:     c.Name,
			Host:     c.Host,
			Port:     c.Port,
			Device:   c.Device,
			Baud:     c.Baud,
			Filter:   c.Filter,
			Callsign: c.Callsign,
			Passcode: "***", // Always redact
		}
	}
	return result
}

func fromTransportDTOs(dtos []transportDTO, existing []transport.TransportConfig) []transport.TransportConfig {
	result := make([]transport.TransportConfig, len(dtos))
	for i, d := range dtos {
		passcode := d.Passcode
		// Preserve existing passcode if masked or empty
		if (passcode == "" || passcode == "***") && i < len(existing) {
			passcode = existing[i].Passcode
		}
		result[i] = transport.TransportConfig{
			Type:     d.Type,
			Name:     d.Name,
			Host:     d.Host,
			Port:     d.Port,
			Device:   d.Device,
			Baud:     d.Baud,
			Filter:   d.Filter,
			Callsign: d.Callsign,
			Passcode: passcode,
		}
	}
	return result
}

func toTileCacheDTO(c config.TileCacheConfig) tileCacheDTO {
	return tileCacheDTO{
		Enabled: c.Enabled,
		DataDir: c.DataDir,
		TileURL: c.TileURL,
		MaxZoom: c.MaxZoom,
	}
}

func fromTileCacheDTO(d tileCacheDTO) config.TileCacheConfig {
	return config.TileCacheConfig{
		Enabled: d.Enabled,
		DataDir: d.DataDir,
		TileURL: d.TileURL,
		MaxZoom: d.MaxZoom,
	}
}

func toWeatherDTO(c config.WeatherConfig) weatherDTO {
	return weatherDTO{
		RetentionDays: c.RetentionDays,
		Alerts:        c.Alerts,
		Units:         c.Units,
	}
}

func fromWeatherDTO(d weatherDTO) config.WeatherConfig {
	units := d.Units
	if units != "metric" && units != "imperial" {
		units = "metric"
	}
	return config.WeatherConfig{
		RetentionDays: d.RetentionDays,
		Alerts:        d.Alerts,
		Units:         units,
	}
}

func toWhat3WordsDTO(c config.What3WordsConfig) what3wordsDTO {
	src := "none"
	switch {
	case os.Getenv("NYMERIA_W3W_API_KEY") != "":
		src = "env"
	case c.APIKey != "":
		src = "config"
	}
	return what3wordsDTO{
		Enabled:          c.Enabled,
		APIKeyConfigured: c.APIKey != "",
		APIKeySource:     src,
		BaseURL:          c.BaseURL,
		Results:          c.Results,
		// APIKey deliberately left zero — omitempty drops it from the response.
	}
}

func fromWhat3WordsDTO(d what3wordsDTO, existing config.What3WordsConfig) config.What3WordsConfig {
	key := d.APIKey
	// Preserve existing key if client sends empty or masked value (same rule
	// as fromSessionDTO's PIN and fromTransportDTOs' passcode).
	if key == "" || key == "***" {
		key = existing.APIKey
	}
	out := existing
	out.Enabled = d.Enabled
	out.APIKey = key
	out.BaseURL = d.BaseURL
	if d.Results > 0 {
		out.Results = d.Results
	}
	return out
}

// classifyRestart returns true if the section requires a server restart.
func classifyRestart(section string) bool {
	switch section {
	case "server", "tilecache", "store":
		return true
	default:
		return false
	}
}

// --- Handlers ---

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	cfg := s.configMgr.Get()
	resp := settingsResponse{
		Station:    toStationDTO(cfg.Station),
		Server:     serverDTO{Listen: cfg.Server.Listen},
		Beacon:     toBeaconDTO(cfg.Beacon),
		Session:    toSessionDTO(cfg.Session),
		Logging:    loggingDTO{Level: cfg.Logging.Level},
		Transports: toTransportDTOs(cfg.Transports),
		TileCache:  toTileCacheDTO(cfg.TileCache),
		Weather:    toWeatherDTO(cfg.Weather),
		Store:      storeDTO{Path: cfg.Store.Path},
		GPS:        toGPSDTO(cfg.GPS),
		What3Words: toWhat3WordsDTO(cfg.What3Words),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUpdateStation(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto stationDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	stationCfg, err := fromStationDTO(dto)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cfg := s.configMgr.Get()
	cfg.Station = stationCfg
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.broadcastPaths(stationCfg.MessagePath, stationCfg.BeaconPath)

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
}

func (s *Server) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto serverDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg := s.configMgr.Get()
	cfg.Server.Listen = dto.Listen
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: true})
}

func (s *Server) handleListSerialPorts(w http.ResponseWriter, _ *http.Request) {
	resp := serialPortsResponse{
		HostOS:    runtime.GOOS,
		Ports:     make([]serial.PortInfo, 0),
		Profiles:  serial.Profiles(),
		BaudRates: serial.StandardBaudRates,
	}
	ports, err := serial.ListPorts()
	if err != nil {
		log.Printf("[serial] list ports: %v", err)
		resp.Error = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if ports != nil {
		resp.Ports = ports
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListKissTNCs(w http.ResponseWriter, _ *http.Request) {
	resp := kissTNCsResponse{
		HostOS: runtime.GOOS,
		TNCs:   make([]kisstcp.TNCInfo, 0),
	}
	tncs, err := kisstcp.Discover()
	if err != nil {
		log.Printf("[kisstcp] discover: %v", err)
		resp.Error = err.Error()
	}
	if tncs != nil {
		resp.TNCs = tncs
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUpdateTransports(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dtos []transportDTO
	if err := json.NewDecoder(r.Body).Decode(&dtos); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg := s.configMgr.Get()
	cfg.Transports = fromTransportDTOs(dtos, cfg.Transports)
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
}

func (s *Server) handleUpdateBeacon(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto beaconDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	bcnCfg, err := fromBeaconDTO(dto)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cfg := s.configMgr.Get()
	// The frontend has no UI for SmartBeacon yet, so an ordinary save omits
	// it entirely (dto.SmartBeacon == nil). Preserve whatever is already
	// configured rather than silently wiping it — same convention as the
	// masked-passcode/PIN preservation below.
	if dto.SmartBeacon == nil {
		bcnCfg.SmartBeacon = cfg.Beacon.SmartBeacon
	}
	cfg.Beacon = bcnCfg
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
}

// handleUpdateGPS follows handleUpdateBeacon, except restartRequired
// reflects only whether the enabled/disabled toggle flipped: an already
// -running GPS manager hot-reloads target/thresholds via cfgMgr.OnChange,
// but enabling GPS from off has no manager instance to reload (app.go only
// constructs one at startup when gps.enabled starts true), so it needs a
// restart.
func (s *Server) handleUpdateGPS(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto gpsDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	gpsCfg, err := fromGPSDTO(dto)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cfg := s.configMgr.Get()
	wasEnabled := cfg.GPS.Enabled
	cfg.GPS = gpsCfg
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: wasEnabled != gpsCfg.Enabled})
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto sessionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg := s.configMgr.Get()
	sessCfg, err := fromSessionDTO(dto, cfg.Session)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cfg.Session = sessCfg
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
}

func (s *Server) handleUpdateLogging(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto loggingDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg := s.configMgr.Get()
	cfg.Logging.Level = dto.Level
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
}

func (s *Server) handleUpdateWeather(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto weatherDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg := s.configMgr.Get()
	cfg.Weather = fromWeatherDTO(dto)
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Update live weather config so /weather/config reflects changes immediately
	s.weatherMu.Lock()
	s.weatherCfg = cfg.Weather
	s.weatherMu.Unlock()

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
}

func (s *Server) handleUpdateTileCache(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto tileCacheDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg := s.configMgr.Get()
	cfg.TileCache = fromTileCacheDTO(dto)
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: true})
}

// handleUpdateWhat3Words follows handleUpdateSession, not handleUpdateTileCache:
// an API key paste mid-event must be live-effective with no restart, so this
// applies the new key to the already-running w3w client directly rather than
// hardcoding RestartRequired: true.
func (s *Server) handleUpdateWhat3Words(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	var dto what3wordsDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg := s.configMgr.Get()
	w3wCfg := fromWhat3WordsDTO(dto, cfg.What3Words)
	cfg.What3Words = w3wCfg
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if c := s.w3wClient(); c != nil {
		c.SetAPIKey(w3wCfg.APIKey)
		c.SetEnabled(w3wCfg.Enabled)
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
}

// handleDeleteWhat3WordsKey clears the stored API key. A dedicated endpoint
// rather than a sentinel string in the DTO — explicit, un-spoofable, and it
// keeps the empty-means-preserve rule on PUT /settings/what3words intact.
func (s *Server) handleDeleteWhat3WordsKey(w http.ResponseWriter, r *http.Request) {
	if s.configMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "config manager not available"})
		return
	}

	cfg := s.configMgr.Get()
	cfg.What3Words.APIKey = ""
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if c := s.w3wClient(); c != nil {
		c.SetAPIKey("")
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
}
