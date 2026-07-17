package cli

import (
	"github.com/jstevewhite/mcpctl/internal/config"
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
	got := redactServer(sc, false)
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

func TestRedactServerMachineRedactsAllEnv(t *testing.T) {
	sc := config.ServerConfig{Transport: "stdio", Env: map[string]string{"LOG_LEVEL": "warn", "DATABASE_URL": "postgres://u:p@h/db"}}
	got := redactServer(sc, true)
	if got.Env["LOG_LEVEL"] == "warn" || got.Env["DATABASE_URL"] == "postgres://u:p@h/db" {
		t.Fatalf("machine mode must redact ALL env values, got %+v", got.Env)
	}
}

func TestRedactServerDoesNotMutateOriginal(t *testing.T) {
	sc := config.ServerConfig{Env: map[string]string{"API_KEY": "topsecret"}, Headers: map[string]string{"X-Api-Key": "lit"}}
	_ = redactServer(sc, false)
	if sc.Env["API_KEY"] != "topsecret" || sc.Headers["X-Api-Key"] != "lit" {
		t.Fatal("redactServer must not mutate the caller's config")
	}
}

func TestServerConfigFromFlagsStdio(t *testing.T) {
	sf := ServerFlags{Stdio: true}
	sc, err := serverConfigFromFlags(sf, []string{"echo", "hi"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.Transport != config.TransportStdio {
		t.Errorf("Transport = %q, want %q", sc.Transport, config.TransportStdio)
	}
	if sc.Command != "echo" || len(sc.Args) != 1 || sc.Args[0] != "hi" {
		t.Errorf("Command/Args = %q %v, want echo [hi]", sc.Command, sc.Args)
	}
}

func TestServerConfigFromFlagsStdioMissingCommand(t *testing.T) {
	sf := ServerFlags{Stdio: true}
	if _, err := serverConfigFromFlags(sf, nil, false); err == nil {
		t.Fatal("expected error when --stdio has no command after `--`")
	}
	if _, err := serverConfigFromFlags(sf, nil, true); err == nil {
		t.Fatal("expected error when -- is present but empty")
	}
}

func TestServerConfigFromFlagsURL(t *testing.T) {
	sf := ServerFlags{URL: "https://example.com", BearerEnv: "MY_TOKEN"}
	sc, err := serverConfigFromFlags(sf, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.Transport != config.TransportHTTP {
		t.Errorf("Transport = %q, want %q", sc.Transport, config.TransportHTTP)
	}
	if sc.URL != "https://example.com" {
		t.Errorf("URL = %q", sc.URL)
	}
	if sc.BearerToken == nil || sc.BearerToken.Env != "MY_TOKEN" {
		t.Errorf("BearerToken = %+v, want env MY_TOKEN", sc.BearerToken)
	}
	if sc.Headers != nil {
		t.Errorf("Headers = %v, want nil (empty maps normalized)", sc.Headers)
	}
	if sc.HeaderEnv != nil {
		t.Errorf("HeaderEnv = %v, want nil (empty maps normalized)", sc.HeaderEnv)
	}
}

func TestServerConfigFromFlagsURLWithHeaders(t *testing.T) {
	sf := ServerFlags{URL: "https://example.com", HeaderLiteral: []string{"X-Api-Key=secret"}, HeaderEnv: []string{"Authorization=MCP_AUTH"}}
	sc, err := serverConfigFromFlags(sf, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.Headers["X-Api-Key"] != "secret" {
		t.Errorf("Headers[X-Api-Key] = %q, want secret", sc.Headers["X-Api-Key"])
	}
	if sc.HeaderEnv["Authorization"] != "MCP_AUTH" {
		t.Errorf("HeaderEnv[Authorization] = %q, want MCP_AUTH", sc.HeaderEnv["Authorization"])
	}
}

func TestServerConfigFromFlagsMutuallyExclusive(t *testing.T) {
	sf := ServerFlags{Stdio: true, URL: "https://example.com"}
	if _, err := serverConfigFromFlags(sf, []string{"echo"}, true); err == nil {
		t.Fatal("expected error when both --stdio and --url are set")
	}
}

func TestServerConfigFromFlagsNeitherSet(t *testing.T) {
	if _, err := serverConfigFromFlags(ServerFlags{}, nil, false); err == nil {
		t.Fatal("expected error when neither --stdio nor --url is set")
	}
}
