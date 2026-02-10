package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Manager provides thread-safe access to the application configuration
// and handles reading/writing the YAML config file.
type Manager struct {
	mu        sync.RWMutex
	cfg       Config
	path      string
	callbacks []func(old, new Config)
}

// NewManager creates a new config manager with the given file path and initial config.
func NewManager(path string, cfg Config) *Manager {
	return &Manager{
		cfg:  cfg,
		path: path,
	}
}

// Get returns a copy of the current configuration.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// Update validates the new config, writes it atomically to disk,
// updates the in-memory copy, and fires onChange callbacks.
func (m *Manager) Update(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Atomic write: write to temp file, then rename
	dir := filepath.Dir(m.path)
	tmp, err := os.CreateTemp(dir, "nymeria-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, m.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config file: %w", err)
	}

	m.mu.Lock()
	old := m.cfg
	m.cfg = cfg
	callbacks := make([]func(old, new Config), len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.mu.Unlock()

	for _, fn := range callbacks {
		fn(old, cfg)
	}

	return nil
}

// OnChange registers a callback that fires after a successful config update.
func (m *Manager) OnChange(fn func(old, new Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, fn)
}

// FilePath returns the path to the config file.
func (m *Manager) FilePath() string {
	return m.path
}
