package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/narvel/nymeria/internal/transport"
)

// BeaconConfig holds beaconing settings.
type BeaconConfig struct {
	Enabled  bool          `yaml:"enabled" json:"enabled"`
	Interval time.Duration `yaml:"interval" json:"interval"`
	Comment  string        `yaml:"comment" json:"comment"`
}

// SessionConfig holds multi-user session settings.
type SessionConfig struct {
	PIN               string        `yaml:"pin" json:"pin,omitempty"`
	InactivityTimeout time.Duration `yaml:"inactivity_timeout" json:"inactivityTimeout"`
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
	RetentionDays int                               `yaml:"retention_days" json:"retentionDays"`
	Alerts        map[string]WeatherAlertThreshold   `yaml:"alerts" json:"alerts"`
	Units         string                             `yaml:"units" json:"units"`
}

// Config holds the application configuration.
type Config struct {
	Server     ServerConfig               `yaml:"server" json:"server"`
	Station    StationConfig              `yaml:"station" json:"station"`
	Transports []transport.TransportConfig `yaml:"transports" json:"transports"`
	Store      StoreConfig                `yaml:"store" json:"store"`
	Logging    LoggingConfig              `yaml:"logging" json:"logging"`
	Beacon     BeaconConfig               `yaml:"beacon" json:"beacon"`
	Session    SessionConfig              `yaml:"session" json:"session"`
	TileCache  TileCacheConfig            `yaml:"tile_cache" json:"tileCache"`
	Weather    WeatherConfig              `yaml:"weather" json:"weather"`
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
	}
}

// Load reads a config file from the given path, applying env var overrides.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
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
