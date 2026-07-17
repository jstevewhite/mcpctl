package cli

import (
	"io"
	"os"
	"testing"

	"github.com/jstevewhite/mcpctl/internal/apperror"
)

// TestVersionFlagWritesShortToStdout captures the real os.Stdout (no SetOut)
// so it verifies the version line lands on stdout, not stderr. A SetOut/SetErr
// two-buffer test cannot catch this: Cobra's OutOrStderr() resolves to the
// SetOut writer, so the buggy cmd.Println path would still hit the same buffer.
func TestVersionFlagWritesShortToStdout(t *testing.T) {
	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w

	root, _ := NewRootCmd()
	root.SetArgs([]string{"--version"})
	runErr := root.Execute()
	_ = w.Close()

	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if got := string(out); got != "mcpctl dev\n" {
		t.Fatalf("--version stdout = %q, want %q", got, "mcpctl dev\n")
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

func TestInvalidTimeoutIsUsageError(t *testing.T) {
	root, _ := NewRootCmd()
	root.SetArgs([]string{"--timeout", "0s", "version"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a usage error for a zero timeout")
	}
	if code := apperror.ExitCode(normalize(err)); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

// TestProtocolVersionIsUsageError verifies the spec §7 behavior: the flag is
// rejected (exit 2) rather than silently ignored, since the SDK pins the
// protocol version and exposes no override.
func TestProtocolVersionIsUsageError(t *testing.T) {
	root, _ := NewRootCmd()
	root.SetArgs([]string{"--protocol-version", "2025-01-01", "version"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a usage error for --protocol-version")
	}
	if code := apperror.ExitCode(normalize(err)); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

// TestMaxPagesZeroIsUsageError verifies the --max-pages guard rejects a
// non-positive cap. The check runs before any transport is dialed, so the
// stdio command after `--` is never executed.
func TestMaxPagesZeroIsUsageError(t *testing.T) {
	root, _ := NewRootCmd()
	root.SetArgs([]string{"tools", "list", "--max-pages", "0", "--stdio", "--", "true"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a usage error for --max-pages 0")
	}
	if code := apperror.ExitCode(normalize(err)); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
