package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/transport"
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
}

type serverDTO struct {
	Listen string `json:"listen"`
}

type beaconDTO struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval"`
	Comment  string `json:"comment"`
}

type sessionDTO struct {
	PINConfigured bool   `json:"pinConfigured"`
	PIN           string `json:"pin,omitempty"`
	InactivityTimeout string `json:"inactivityTimeout"`
}

type loggingDTO struct {
	Level string `json:"level"`
}

type transportDTO struct {
	Type     string `json:"type"`
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

type weatherDTO struct {
	RetentionDays int                                 `json:"retentionDays"`
	Alerts        map[string]config.WeatherAlertThreshold `json:"alerts,omitempty"`
	Units         string                               `json:"units"`
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
}

type updateResponse struct {
	RestartRequired bool `json:"restartRequired"`
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
	}, nil
}

func toBeaconDTO(c config.BeaconConfig) beaconDTO {
	return beaconDTO{
		Enabled:  c.Enabled,
		Interval: c.Interval.String(),
		Comment:  c.Comment,
	}
}

func fromBeaconDTO(d beaconDTO) (config.BeaconConfig, error) {
	interval, err := time.ParseDuration(d.Interval)
	if err != nil && d.Interval != "" {
		return config.BeaconConfig{}, err
	}
	return config.BeaconConfig{
		Enabled:  d.Enabled,
		Interval: interval,
		Comment:  d.Comment,
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

// classifyRestart returns true if the section requires a server restart.
func classifyRestart(section string) bool {
	switch section {
	case "station", "server", "transports", "tilecache", "store":
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

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: true})
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

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: true})
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
	cfg.Beacon = bcnCfg
	if err := s.configMgr.Update(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateResponse{RestartRequired: false})
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
	s.weatherCfg = cfg.Weather

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
