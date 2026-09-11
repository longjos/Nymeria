package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/narvel/nymeria/internal/aprs"
	"github.com/narvel/nymeria/internal/transport"
)

// BeaconConfig holds beaconing settings.
type BeaconConfig struct {
	Enabled     bool               `yaml:"enabled" json:"enabled"`
	Interval    time.Duration      `yaml:"interval" json:"interval"`
	Comment     string             `yaml:"comment" json:"comment"`
	SmartBeacon *SmartBeaconConfig `yaml:"smart_beacon" json:"smartBeacon,omitempty"`
}

// SmartBeaconConfig mirrors beacon.SmartConfig; speeds in mph, angles in degrees.
type SmartBeaconConfig struct {
	Enabled   bool          `yaml:"enabled" json:"enabled"`
	FastSpeed float64       `yaml:"fast_speed" json:"fastSpeed"`
	SlowSpeed float64       `yaml:"slow_speed" json:"slowSpeed"`
	FastRate  time.Duration `yaml:"fast_rate" json:"fastRate"`
	SlowRate  time.Duration `yaml:"slow_rate" json:"slowRate"`
	TurnAngle float64       `yaml:"turn_angle" json:"turnAngle"`
	TurnSlope float64       `yaml:"turn_slope" json:"turnSlope"`
}

// GPSConfig holds live host-GPS settings.
type GPSConfig struct {
	Enabled      bool          `yaml:"enabled" json:"enabled"`
	Type         string        `yaml:"type" json:"type"` // gpsd | nmea
	Host         string        `yaml:"host" json:"host"` // gpsd, or nmea-over-TCP
	Port         int           `yaml:"port" json:"port"`
	Device       string        `yaml:"device" json:"device"` // nmea serial
	Baud         int           `yaml:"baud" json:"baud"`
	MinInterval  time.Duration `yaml:"min_interval" json:"minInterval"`
	StaleAfter   time.Duration `yaml:"stale_after" json:"staleAfter"`
	UseForBeacon bool          `yaml:"use_for_beacon" json:"useForBeacon"`
}

// SessionConfig holds multi-user session settings.
type SessionConfig struct {
	PIN               string        `yaml:"pin" json:"pin,omitempty"`
	InactivityTimeout time.Duration `yaml:"inactivity_timeout" json:"inactivityTimeout"`
	ReconnectWindow   time.Duration `yaml:"reconnect_window" json:"reconnectWindow"`
}

// TileCacheConfig holds offline map tile cache settings.
type TileCacheConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	DataDir string `yaml:"data_dir" json:"dataDir"`
	TileURL string `yaml:"tile_url" json:"tileUrl"`
	MaxZoom int    `yaml:"max_zoom" json:"maxZoom"`
}

// WeatherAlertThreshold defines min/max alert thresholds for a weather metric.
type WeatherAlertThreshold struct {
	Min *float64 `yaml:"min" json:"min,omitempty"`
	Max *float64 `yaml:"max" json:"max,omitempty"`
}

// WeatherConfig holds weather dashboard settings.
type WeatherConfig struct {
	RetentionDays int                              `yaml:"retention_days" json:"retentionDays"`
	Alerts        map[string]WeatherAlertThreshold `yaml:"alerts" json:"alerts"`
	Units         string                           `yaml:"units" json:"units"`
}

// Config holds the application configuration.
type Config struct {
	Server     ServerConfig                `yaml:"server" json:"server"`
	Station    StationConfig               `yaml:"station" json:"station"`
	Transports []transport.TransportConfig `yaml:"transports" json:"transports"`
	Store      StoreConfig                 `yaml:"store" json:"store"`
	Logging    LoggingConfig               `yaml:"logging" json:"logging"`
	Beacon     BeaconConfig                `yaml:"beacon" json:"beacon"`
	Session    SessionConfig               `yaml:"session" json:"session"`
	TileCache  TileCacheConfig             `yaml:"tile_cache" json:"tileCache"`
	Weather    WeatherConfig               `yaml:"weather" json:"weather"`
	GPS        GPSConfig                   `yaml:"gps" json:"gps"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Listen string `yaml:"listen" json:"listen"`
}

// StationConfig holds the operator's station identity and tracker tuning.
type StationConfig struct {
	Callsign        string            `yaml:"callsign" json:"callsign"`
	SSID            int               `yaml:"ssid" json:"ssid"`
	Lat             float64           `yaml:"lat" json:"lat"`
	Lon             float64           `yaml:"lon" json:"lon"`
	SymbolTable     string            `yaml:"symbol_table" json:"symbolTable"`
	SymbolCode      string            `yaml:"symbol_code" json:"symbolCode"`
	Comment         string            `yaml:"comment" json:"comment"`
	TrackMaxPoints  int               `yaml:"track_max_points" json:"trackMaxPoints"`
	StaleTimeout    time.Duration     `yaml:"stale_timeout" json:"staleTimeout"`
	DedupWindow     time.Duration     `yaml:"dedup_window" json:"dedupWindow"`
	TacticalAliases map[string]string `yaml:"tactical_aliases" json:"tacticalAliases,omitempty"`
	// MessagePath is the TNC2 digipeater path for messages and acks
	// (e.g. "WIDE1-1,WIDE2-1" or "TCPIP*"). Empty means direct / no path.
	MessagePath string `yaml:"message_path" json:"messagePath"`
	// BeaconPath is the TNC2 digipeater path for beacons and APRS objects/items.
	BeaconPath string `yaml:"beacon_path" json:"beaconPath"`
}

// StoreConfig holds storage settings.
type StoreConfig struct {
	Path string `yaml:"path" json:"path"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level string `yaml:"level" json:"level"` // debug, info, warn, error
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Listen: ":8080",
		},
		Station: StationConfig{
			Callsign:       "N0CALL",
			TrackMaxPoints: 100,
			StaleTimeout:   80 * time.Minute,
			DedupWindow:    30 * time.Second,
			MessagePath:    aprs.FormatPath(aprs.DefaultRFPath()),
			BeaconPath:     aprs.FormatPath(aprs.DefaultRFPath()),
		},
		Store: StoreConfig{
			Path: "./nymeria.db",
		},
		Logging: LoggingConfig{
			Level: "info",
		},
		Beacon: BeaconConfig{
			Enabled:  false,
			Interval: 10 * time.Minute,
		},
		Session: SessionConfig{
			InactivityTimeout: 30 * time.Minute,
		},
		TileCache: TileCacheConfig{
			Enabled: true,
			MaxZoom: 16,
		},
		Weather: WeatherConfig{
			RetentionDays: 7,
			Units:         "metric",
		},
		GPS: GPSConfig{
			Enabled:      false,
			Type:         "gpsd",
			Host:         "127.0.0.1",
			Port:         2947,
			Baud:         9600,
			MinInterval:  1 * time.Second,
			StaleAfter:   30 * time.Second,
			UseForBeacon: true,
		},
	}
}

// Load reads a config file from the given path, applying env var overrides.
// When the file cannot be read (including os.IsNotExist), the returned Config
// is still usable: defaults with env overrides applied. Callers that treat a
// missing file as "run with defaults" should use the returned Config as-is so
// NYMERIA_* overrides keep winning over defaults in that path too.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		applyEnvOverrides(&cfg)
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	applyEnvOverrides(&cfg)

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// Validate checks the config for errors.
func (c *Config) Validate() error {
	if c.Station.Callsign == "" {
		return fmt.Errorf("station.callsign is required")
	}
	if c.Station.SSID < 0 || c.Station.SSID > 15 {
		return fmt.Errorf("station.ssid must be 0-15, got %d", c.Station.SSID)
	}
	if c.Station.TrackMaxPoints < 0 {
		return fmt.Errorf("station.track_max_points must be >= 0")
	}
	if c.Station.StaleTimeout < 0 {
		return fmt.Errorf("station.stale_timeout must be >= 0")
	}
	if c.Station.Lat < -90 || c.Station.Lat > 90 {
		return fmt.Errorf("station.lat must be -90 to 90")
	}
	if c.Station.Lon < -180 || c.Station.Lon > 180 {
		return fmt.Errorf("station.lon must be -180 to 180")
	}
	if c.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if _, err := aprs.ParsePath(c.Station.MessagePath); err != nil {
		return fmt.Errorf("station.message_path: %w", err)
	}
	if _, err := aprs.ParsePath(c.Station.BeaconPath); err != nil {
		return fmt.Errorf("station.beacon_path: %w", err)
	}

	for i, t := range c.Transports {
		if t.Type == "" {
			return fmt.Errorf("transports[%d].type is required", i)
		}
		switch t.Type {
		case "aprsis":
			if t.Host == "" {
				return fmt.Errorf("transports[%d].host is required for aprsis", i)
			}
			if t.Port == 0 {
				return fmt.Errorf("transports[%d].port is required for aprsis", i)
			}
		case "kisstcp":
			if t.Host == "" {
				return fmt.Errorf("transports[%d].host is required for kisstcp", i)
			}
			if t.Port == 0 {
				return fmt.Errorf("transports[%d].port is required for kisstcp", i)
			}
		case "serial":
			if t.Device == "" {
				return fmt.Errorf("transports[%d].device is required for serial", i)
			}
		}
	}

	if c.GPS.Enabled {
		switch c.GPS.Type {
		case "gpsd":
			if c.GPS.Host == "" {
				return fmt.Errorf("gps.host is required for gpsd")
			}
			if c.GPS.Port <= 0 || c.GPS.Port > 65535 {
				return fmt.Errorf("gps.port must be 1-65535 for gpsd")
			}
		case "nmea":
			if c.GPS.Device == "" && c.GPS.Host == "" {
				return fmt.Errorf("gps.device or gps.host is required for nmea")
			}
			if c.GPS.Device != "" && c.GPS.Host != "" {
				return fmt.Errorf("gps: set only one of gps.device or gps.host for nmea")
			}
			if c.GPS.Host != "" && (c.GPS.Port <= 0 || c.GPS.Port > 65535) {
				return fmt.Errorf("gps.port must be 1-65535 for nmea over tcp")
			}
			if c.GPS.Device != "" && c.GPS.Baud <= 0 {
				return fmt.Errorf("gps.baud must be > 0 for nmea serial")
			}
		case "":
			return fmt.Errorf("gps.type is required when gps.enabled")
		default:
			return fmt.Errorf("gps.type must be gpsd or nmea, got %q", c.GPS.Type)
		}
		if c.GPS.MinInterval < 0 {
			return fmt.Errorf("gps.min_interval must be >= 0")
		}
		if c.GPS.StaleAfter < 0 {
			return fmt.Errorf("gps.stale_after must be >= 0")
		}
	}

	if sb := c.Beacon.SmartBeacon; sb != nil && sb.Enabled {
		if sb.FastSpeed <= sb.SlowSpeed {
			return fmt.Errorf("beacon.smart_beacon.fast_speed must be > slow_speed")
		}
		if sb.FastRate <= 0 || sb.SlowRate <= 0 {
			return fmt.Errorf("beacon.smart_beacon rates must be > 0")
		}
		if sb.FastRate > sb.SlowRate {
			return fmt.Errorf("beacon.smart_beacon.fast_rate must be <= slow_rate")
		}
	}

	return nil
}

// applyEnvOverrides reads environment variables and applies them to the config.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("NYMERIA_LISTEN"); v != "" {
		cfg.Server.Listen = v
	}
	if v := os.Getenv("NYMERIA_CALLSIGN"); v != "" {
		cfg.Station.Callsign = strings.ToUpper(v)
	}
	if v := os.Getenv("NYMERIA_DB_PATH"); v != "" {
		cfg.Store.Path = v
	}
	if v := os.Getenv("NYMERIA_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
}
