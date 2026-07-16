package cli

import (
	"bytes"
	"testing"

	"mcpctl/internal/apperror"
)

func TestVersionFlagPrintsShort(t *testing.T) {
	root, _ := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out.String(); got != "mcpctl dev\n" {
		t.Fatalf("--version = %q, want %q", got, "mcpctl dev\n")
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	root, _ := NewRootCmd()
	root.SetArgs([]string{"nonexistent"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for unknown command")
	}
	if code := apperror.ExitCode(normalize(err)); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestInvalidLogLevelIsUsageError(t *testing.T) {
	root, _ := NewRootCmd()
	root.SetArgs([]string{"--log-level", "nope"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for invalid log level")
	}
	if code := apperror.ExitCode(normalize(err)); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
