package buildinfo

import (
	"runtime"
	"testing"
)

func saveVars(t *testing.T) {
	t.Helper()
	v, c, d := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = v, c, d })
}

func TestShort(t *testing.T) {
	saveVars(t)
	Version = "1.2.3"
	if got, want := Short(), "mcpctl 1.2.3"; got != want {
		t.Fatalf("Short() = %q, want %q", got, want)
	}
}

func TestFull(t *testing.T) {
	saveVars(t)
	Version, Commit, Date = "1.2.3", "abc123", "2026-07-16T00:00:00Z"
	want := "mcpctl version 1.2.3\ncommit: abc123\nbuilt: 2026-07-16T00:00:00Z\ngo: " + runtime.Version()
	if got := Full(); got != want {
		t.Fatalf("Full() = %q, want %q", got, want)
	}
}
