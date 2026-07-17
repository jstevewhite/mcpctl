package config

import (
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"

	"mcpctl/internal/apperror"
)

// Save writes cfg to path as TOML with owner-only (0600) permissions, creating
// the parent directory (0700) if needed.
func Save(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "encode config")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return apperror.Wrap(apperror.KindConfig, err, "create config directory %s", dir)
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "write config %s", path)
	}
	return nil
}
