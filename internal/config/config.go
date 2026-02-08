package config

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/narvel/nymeria/internal/transport"
)

// Config holds the application configuration.
type Config struct {
	Server     ServerConfig             `yaml:"server"`
	Station    StationConfig            `yaml:"station"`
	Transports []transport.TransportConfig `yaml:"transports"`
	Store      StoreConfig              `yaml:"store"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Listen string `yaml:"listen"`
}

// StationConfig holds the operator's station identity.
type StationConfig struct {
	Callsign string `yaml:"callsign"`
	SSID     int    `yaml:"ssid"`
}

// StoreConfig holds storage settings.
type StoreConfig struct {
	Path string `yaml:"path"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Listen: ":8080",
		},
		Station: StationConfig{
			Callsign: "N0CALL",
		},
		Store: StoreConfig{
			Path: "./nymeria.db",
		},
	}
}

// Load reads a config file from the given path.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
