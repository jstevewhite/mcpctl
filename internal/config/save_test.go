package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveRoundTrip(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Servers: map[string]ServerConfig{
			"local": {Transport: "stdio", Command: "npx", Args: []string{"-y", "srv"}},
			"remote": {
				Transport: "streamable-http", URL: "https://example.com/mcp",
				HeaderEnv:   map[string]string{"Authorization": "MCP_AUTH"},
				BearerToken: nil,
			},
		},
	}
	path := filepath.Join(t.TempDir(), "sub", "config.toml") // parent dir does not exist yet
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if got.Version != 1 || len(got.Servers) != 2 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	l := got.Servers["local"]
	if l.Command != "npx" || len(l.Args) != 2 {
		t.Fatalf("local server not preserved: %+v", l)
	}
	r := got.Servers["remote"]
	if r.URL != "https://example.com/mcp" || r.HeaderEnv["Authorization"] != "MCP_AUTH" {
		t.Fatalf("remote server not preserved: %+v", r)
	}
}

func TestSaveWritesOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, &Config{Version: 1}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perms = %o, want 600", perm)
	}
}
