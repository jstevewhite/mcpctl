package config

import (
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/jstevewhite/mcpctl/internal/apperror"
)

// Save writes cfg to path as TOML with owner-only (0600) permissions, creating
// the parent directory (0700) if needed.
func Save(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "encode config")
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return apperror.Wrap(apperror.KindConfig, err, "create config directory %s", dir)
		}
	}
	// Write to a temp file in the same directory, then rename over the target.
	// This is atomic on POSIX filesystems: a crash mid-write can't truncate the
	// existing config, and there's no window where the file exists with
	// looser-than-0600 permissions (CreateTemp always creates with 0600).
	tmpDir := dir
	if tmpDir == "" || tmpDir == "." {
		tmpDir = "."
	}
	tmp, err := os.CreateTemp(tmpDir, ".mcpctl-config-*.tmp") // CreateTemp uses 0600
	if err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "create temp config")
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return apperror.Wrap(apperror.KindConfig, err, "write temp config")
	}
	if err := tmp.Close(); err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "close temp config")
	}
	if err := os.Rename(tmpName, path); err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "replace config %s", path)
	}
	return nil
}
