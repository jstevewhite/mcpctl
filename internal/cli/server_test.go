package cli

import (
	"mcpctl/internal/config"
	"testing"
)

func TestRedactServerHidesSecrets(t *testing.T) {
	sc := config.ServerConfig{
		Transport: "streamable-http",
		URL:       "https://x",
		Env:       map[string]string{"LOG_LEVEL": "warn", "API_KEY": "topsecret"},
		Headers:   map[string]string{"X-Api-Key": "literal-secret"},
		HeaderEnv: map[string]string{"Authorization": "MCP_AUTH"},
	}
	got := redactServer(sc)
	if got.Env["LOG_LEVEL"] != "warn" {
		t.Errorf("non-sensitive env value should be shown, got %q", got.Env["LOG_LEVEL"])
	}
	if got.Env["API_KEY"] == "topsecret" {
		t.Error("sensitive env value must be redacted")
	}
	if got.Headers["X-Api-Key"] == "literal-secret" {
		t.Error("literal header value must be redacted")
	}
	if got.HeaderEnv["Authorization"] != "MCP_AUTH" {
		t.Errorf("header_env reference (an env var name, not a secret) should be shown, got %q", got.HeaderEnv["Authorization"])
	}
}
