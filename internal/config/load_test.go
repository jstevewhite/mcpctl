package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

const sampleTOML = `
version = 1

[servers.local-fs]
transport = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(writeTemp(t, sampleTOML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := cfg.Servers["local-fs"]
	if !ok || s.Command != "npx" || len(s.Args) != 3 {
		t.Fatalf("unexpected server config: %+v", s)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	_, err := Load(writeTemp(t, "version = 1\nbogus = true\n"))
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestLoadMissingExplicitFileErrors(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err == nil {
		t.Fatal("expected config error for missing explicit file")
	}
}

func TestLoadResolvedMissingDefaultIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no config.toml written here
	cfg, err := LoadResolved("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 1 || len(cfg.Servers) != 0 {
		t.Fatalf("expected empty valid config, got %+v", cfg)
	}
}

func TestLoadMissingWrapsNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load missing-file error should wrap fs.ErrNotExist, got %v", err)
	}
}

func TestLoadResolvedMissingExplicitErrors(t *testing.T) {
	_, err := LoadResolved(filepath.Join(t.TempDir(), "nope.toml"))
	if err == nil {
		t.Fatal("expected error for a missing explicit config path")
	}
}

func TestLoadRejectsInvalidVersion(t *testing.T) {
	_, err := Load(writeTemp(t, "version = 2\n"))
	if err == nil {
		t.Fatal("expected validation error for version = 2")
	}
}
