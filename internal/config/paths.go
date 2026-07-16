package config

import (
	"os"
	"path/filepath"
)

const (
	appDir   = "mcpctl"
	fileName = "config.toml"
)

// DefaultPath returns the platform-appropriate default config file path,
// preferring XDG_CONFIG_HOME when set.
func DefaultPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, appDir, fileName), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appDir, fileName), nil
}

// Resolve returns the config path to use. A non-empty override wins;
// otherwise the default path is used.
func Resolve(override string) (path string, isDefault bool, err error) {
	if override != "" {
		return override, false, nil
	}
	p, err := DefaultPath()
	return p, true, err
}
