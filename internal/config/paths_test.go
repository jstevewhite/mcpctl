package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.FromSlash("/xdg/mcpctl/config.toml"); got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestResolvePrefersOverride(t *testing.T) {
	got, isDefault, err := Resolve("/tmp/custom.toml")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/custom.toml" || isDefault {
		t.Fatalf("Resolve override = (%q, %v), want (/tmp/custom.toml, false)", got, isDefault)
	}
}

func TestResolveFallsBackToDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, isDefault, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.FromSlash("/xdg/mcpctl/config.toml"); got != want || !isDefault {
		t.Fatalf("Resolve default = (%q, %v), want (%q, true)", got, isDefault, want)
	}
}

func TestDefaultPathFallsBackToUserConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("mcpctl", "config.toml")
	if !strings.HasSuffix(got, want) {
		t.Fatalf("DefaultPath() = %q, want suffix %q", got, want)
	}
}
