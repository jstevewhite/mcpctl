package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestShort(t *testing.T) {
	Version = "1.2.3"
	if got, want := Short(), "mcpctl 1.2.3"; got != want {
		t.Fatalf("Short() = %q, want %q", got, want)
	}
}

func TestFull(t *testing.T) {
	Version, Commit, Date = "1.2.3", "abc123", "2026-07-16T00:00:00Z"
	got := Full()
	for _, want := range []string{
		"mcpctl version 1.2.3",
		"commit: abc123",
		"built: 2026-07-16T00:00:00Z",
		"go: " + runtime.Version(),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Full() missing %q\ngot:\n%s", want, got)
		}
	}
}
