package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// dataDirName returns the app folder name appended to os.UserConfigDir()'s
// result: "Nymeria" on windows and darwin (conventionally capitalized),
// "nymeria" elsewhere (XDG lowercase convention).
func dataDirName(goos string) string {
	switch goos {
	case "windows", "darwin":
		return "Nymeria"
	default:
		return "nymeria"
	}
}

// UserDataDir returns the per-user Nymeria data directory for the current OS,
// built on os.UserConfigDir():
//
//	windows: %APPDATA%\Nymeria
//	darwin:  ~/Library/Application Support/Nymeria
//	other:   $XDG_CONFIG_HOME/nymeria (or ~/.config/nymeria)
//
// It does not create the directory.
func UserDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, dataDirName(runtime.GOOS)), nil
}

// EnsureUserDataDir returns UserDataDir after creating it (and parents) with
// permission 0o700 if it does not exist.
func EnsureUserDataDir() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create user data dir: %w", err)
	}
	return dir, nil
}

// DefaultUserConfigPath returns the default desktop config file path inside
// dir: filepath.Join(dir, "nymeria.yaml").
func DefaultUserConfigPath(dir string) string {
	return filepath.Join(dir, "nymeria.yaml")
}

// ResolveUserPaths returns a copy of cfg with paths that still hold their
// CWD-relative *defaults* retargeted into dir:
//
//	Store.Path == "./nymeria.db"  → filepath.Join(dir, "nymeria.db")
//
// Values the user (or an env override such as NYMERIA_DB_PATH) already
// changed are left untouched. TileCache.DataDir is never rewritten: when
// empty it is derived from the store path's directory downstream, so moving
// Store.Path automatically moves the tile cache to <dir>/tiles.
func ResolveUserPaths(cfg Config, dir string) Config {
	if cfg.Store.Path == DefaultConfig().Store.Path {
		cfg.Store.Path = filepath.Join(dir, "nymeria.db")
	}
	return cfg
}
