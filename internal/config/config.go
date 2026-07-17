// Package config defines the on-disk configuration model and validation.
package config

import (
	"net/url"
	"strings"

	"github.com/jstevewhite/mcpctl/internal/apperror"
)

// Config is the top-level configuration document.
type Config struct {
	Version  int                     `toml:"version"`
	Defaults DefaultsConfig          `toml:"defaults"`
	Servers  map[string]ServerConfig `toml:"servers"`
}

// DefaultsConfig holds global default settings.
type DefaultsConfig struct {
	Timeout        string `toml:"timeout"`
	ConnectTimeout string `toml:"connect_timeout"`
}

// ServerConfig is a single named server definition.
type ServerConfig struct {
	Transport   string            `toml:"transport" json:"transport"`
	Command     string            `toml:"command,omitempty" json:"command,omitempty"`
	Args        []string          `toml:"args,omitempty" json:"args,omitempty"`
	CWD         string            `toml:"cwd,omitempty" json:"cwd,omitempty"`
	Env         map[string]string `toml:"env,omitempty" json:"env,omitempty"`
	URL         string            `toml:"url,omitempty" json:"url,omitempty"`
	Headers     map[string]string `toml:"headers,omitempty" json:"headers,omitempty"`
	HeaderEnv   map[string]string `toml:"header_env,omitempty" json:"header_env,omitempty"`
	BearerToken *TokenSource      `toml:"bearer_token,omitempty" json:"bearer_token,omitempty"`
}

// TokenSource names the environment variable holding a bearer token.
type TokenSource struct {
	Env string `toml:"env" json:"env"`
}

const (
	TransportStdio = "stdio"
	TransportHTTP  = "streamable-http"
)

// Validate enforces the configuration rules from the spec.
func (c *Config) Validate() error {
	if c.Version != 1 {
		return apperror.Config("config version must be 1, got %d", c.Version)
	}
	for name, s := range c.Servers {
		if strings.TrimSpace(name) == "" {
			return apperror.Config("server name must not be empty")
		}
		if err := s.validate(name); err != nil {
			return err
		}
	}
	return nil
}

func (s ServerConfig) validate(name string) error {
	switch s.Transport {
	case TransportStdio:
		if s.Command == "" {
			return apperror.Config("server %q: stdio transport requires a command", name)
		}
		if s.URL != "" {
			return apperror.Config("server %q: stdio transport must not set url", name)
		}
	case TransportHTTP:
		if s.Command != "" || len(s.Args) > 0 {
			return apperror.Config("server %q: streamable-http transport must not set command or args", name)
		}
		if err := validateHTTPURL(name, s.URL); err != nil {
			return err
		}
	default:
		return apperror.Config("server %q: unknown transport %q (want stdio or streamable-http)", name, s.Transport)
	}

	for header, envVar := range s.HeaderEnv {
		if strings.TrimSpace(envVar) == "" {
			return apperror.Config("server %q: header_env[%q] names an empty environment variable", name, header)
		}
	}
	if s.BearerToken != nil && strings.TrimSpace(s.BearerToken.Env) == "" {
		return apperror.Config("server %q: bearer_token.env must name a non-empty environment variable", name)
	}
	if authMechanismCount(s) > 1 {
		return apperror.Config("server %q: multiple Authorization mechanisms configured; use exactly one of bearer_token, an Authorization header, or an Authorization header_env", name)
	}
	return nil
}

func validateHTTPURL(name, raw string) error {
	if raw == "" {
		return apperror.Config("server %q: streamable-http transport requires a url", name)
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return apperror.Config("server %q: url must be an absolute http or https URL", name)
	}
	return nil
}

func headerMapHasAuthorization(m map[string]string) bool {
	for h := range m {
		if strings.EqualFold(h, "Authorization") {
			return true
		}
	}
	return false
}

func authMechanismCount(s ServerConfig) int {
	n := 0
	if headerMapHasAuthorization(s.Headers) {
		n++
	}
	if headerMapHasAuthorization(s.HeaderEnv) {
		n++
	}
	if s.BearerToken != nil {
		n++
	}
	return n
}
