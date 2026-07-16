package config

import (
	"errors"
	"io/fs"
	"os"

	toml "github.com/pelletier/go-toml/v2"

	"mcpctl/internal/apperror"
)

// Load reads and validates a config file. A missing file is a config error.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, apperror.Config("config file not found: %s", path)
		}
		return nil, apperror.Wrap(apperror.KindConfig, err, "open config %s", path)
	}
	defer f.Close()

	var cfg Config
	dec := toml.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, apperror.Wrap(apperror.KindConfig, err, "parse config %s", path)
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadResolved resolves the config path from an optional override and loads it.
// A missing default-path file yields an empty valid config; a missing explicit
// override is an error.
func LoadResolved(override string) (*Config, error) {
	path, isDefault, err := Resolve(override)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindConfig, err, "resolve config path")
	}
	cfg, err := Load(path)
	if err != nil {
		var ae *apperror.Error
		if isDefault && errors.As(err, &ae) && ae.Kind == apperror.KindConfig && fileMissing(path) {
			return &Config{Version: 1, Servers: map[string]ServerConfig{}}, nil
		}
		return nil, err
	}
	return cfg, nil
}

func fileMissing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}
