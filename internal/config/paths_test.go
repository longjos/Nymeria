package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDataDirName(t *testing.T) {
	tests := []struct {
		goos string
		want string
	}{
		{"windows", "Nymeria"},
		{"darwin", "Nymeria"},
		{"linux", "nymeria"},
		{"freebsd", "nymeria"},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			got := dataDirName(tt.goos)
			if got != tt.want {
				t.Errorf("dataDirName(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
}

func TestUserDataDirShape(t *testing.T) {
	dir, err := UserDataDir()
	if err != nil {
		t.Fatalf("UserDataDir() error: %v", err)
	}

	wantBase := dataDirName(runtime.GOOS)
	if filepath.Base(dir) != wantBase {
		t.Errorf("filepath.Base(dir) = %q, want %q", filepath.Base(dir), wantBase)
	}

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("os.UserConfigDir() error: %v", err)
	}
	if filepath.Dir(dir) != base {
		t.Errorf("filepath.Dir(dir) = %q, want %q", filepath.Dir(dir), base)
	}
}

func TestEnsureUserDataDirCreates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME has no effect on windows")
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir, err := EnsureUserDataDir()
	if err != nil {
		t.Fatalf("EnsureUserDataDir() error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("os.Stat(%q) error: %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("%q is not a directory", dir)
	}
	if perm := info.Mode().Perm(); perm&0o700 != 0o700 {
		t.Errorf("mode = %04o, want user bits 0700 set", perm)
	}

	dir2, err := EnsureUserDataDir()
	if err != nil {
		t.Fatalf("EnsureUserDataDir() second call error: %v", err)
	}
	if dir2 != dir {
		t.Errorf("second call path = %q, want %q", dir2, dir)
	}
}

func TestDefaultUserConfigPath(t *testing.T) {
	got := DefaultUserConfigPath("/x/y")
	want := filepath.Join("/x/y", "nymeria.yaml")
	if got != want {
		t.Errorf("DefaultUserConfigPath(%q) = %q, want %q", "/x/y", got, want)
	}
}

func TestResolveUserPaths(t *testing.T) {
	const dir = "/data/dir"

	tests := []struct {
		name      string
		modify    func(*Config)
		check     func(t *testing.T, got Config)
		checkOrig func(t *testing.T, orig Config)
	}{
		{
			name:   "default store path retargeted",
			modify: func(c *Config) {},
			check: func(t *testing.T, got Config) {
				want := filepath.Join(dir, "nymeria.db")
				if got.Store.Path != want {
					t.Errorf("Store.Path = %q, want %q", got.Store.Path, want)
				}
			},
		},
		{
			name: "user-set store path unchanged",
			modify: func(c *Config) {
				c.Store.Path = "/srv/nymeria.db"
			},
			check: func(t *testing.T, got Config) {
				if got.Store.Path != "/srv/nymeria.db" {
					t.Errorf("Store.Path = %q, want /srv/nymeria.db", got.Store.Path)
				}
			},
		},
		{
			name: "env-override simulation store path unchanged",
			modify: func(c *Config) {
				c.Store.Path = "/docker/volume/nymeria.db"
			},
			check: func(t *testing.T, got Config) {
				if got.Store.Path != "/docker/volume/nymeria.db" {
					t.Errorf("Store.Path = %q, want /docker/volume/nymeria.db", got.Store.Path)
				}
			},
		},
		{
			name: "explicit tile cache dir preserved while store rewritten",
			modify: func(c *Config) {
				c.TileCache.DataDir = "/tiles"
			},
			check: func(t *testing.T, got Config) {
				if got.TileCache.DataDir != "/tiles" {
					t.Errorf("TileCache.DataDir = %q, want /tiles", got.TileCache.DataDir)
				}
				want := filepath.Join(dir, "nymeria.db")
				if got.Store.Path != want {
					t.Errorf("Store.Path = %q, want %q", got.Store.Path, want)
				}
			},
		},
		{
			name: "empty tile cache dir stays empty",
			modify: func(c *Config) {
				c.TileCache.DataDir = ""
			},
			check: func(t *testing.T, got Config) {
				if got.TileCache.DataDir != "" {
					t.Errorf("TileCache.DataDir = %q, want empty", got.TileCache.DataDir)
				}
			},
		},
		{
			name:   "does not mutate original config",
			modify: func(c *Config) {},
			check:  func(t *testing.T, got Config) {},
			checkOrig: func(t *testing.T, orig Config) {
				if orig.Store.Path != "./nymeria.db" {
					t.Errorf("original Store.Path = %q, want ./nymeria.db", orig.Store.Path)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(&cfg)
			got := ResolveUserPaths(cfg, dir)
			tt.check(t, got)
			if tt.checkOrig != nil {
				tt.checkOrig(t, cfg)
			}
		})
	}
}

func TestHeadlessDefaultsUnchanged(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Store.Path != "./nymeria.db" {
		t.Errorf("DefaultConfig().Store.Path = %q, want ./nymeria.db", cfg.Store.Path)
	}
	if cfg.Server.Listen != ":8080" {
		t.Errorf("DefaultConfig().Server.Listen = %q, want :8080", cfg.Server.Listen)
	}

	missing, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("Load(nonexistent) error = nil, want os.IsNotExist")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("Load(nonexistent) error = %v, want os.IsNotExist", err)
	}
	if missing.Store.Path != "./nymeria.db" || missing.Server.Listen != ":8080" {
		t.Errorf("Load(nonexistent) returned Store.Path=%q Listen=%q, want defaults untouched", missing.Store.Path, missing.Server.Listen)
	}

	t.Setenv("NYMERIA_DB_PATH", "/env/db.db")
	path := filepath.Join(t.TempDir(), "nymeria.yaml")
	if err := os.WriteFile(path, []byte("station: {callsign: TEST}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Store.Path != "/env/db.db" {
		t.Errorf("Store.Path = %q, want /env/db.db", loaded.Store.Path)
	}
}
