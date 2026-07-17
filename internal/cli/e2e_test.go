//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinaries builds the mcpctl binary and the stdio test server once.
func buildBinaries(t *testing.T) (mcpctl, server string) {
	t.Helper()
	dir := t.TempDir()
	mcpctl = filepath.Join(dir, "mcpctl")
	server = filepath.Join(dir, "test-server")
	for _, b := range []struct{ out, pkg string }{
		{mcpctl, "mcpctl/cmd/mcpctl"},
		{server, "mcpctl/internal/testserver/stdio"},
	} {
		build := exec.Command("go", "build", "-o", b.out, b.pkg)
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			t.Fatalf("building %s: %v", b.pkg, err)
		}
	}
	return mcpctl, server
}

func run(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code = 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", bin, err)
	}
	return out.String(), errb.String(), code
}

func TestE2EToolsListHuman(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	stdout, _, code := run(t, mcpctl, "tools", "list", "--stdio", "--", server)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "echo") || !strings.Contains(stdout, "add") {
		t.Fatalf("expected tool names in output:\n%s", stdout)
	}
}

func TestE2EToolsListJSONCleanStdout(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	stdout, stderr, code := run(t, mcpctl, "--output", "json", "tools", "list", "--stdio", "--", server)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("stdout is not JSON:\n%s", stdout)
	}
	// The test server logs one startup line to stderr; stdout must stay clean JSON.
	if strings.Contains(stdout, "test server") {
		t.Fatalf("server stderr leaked into stdout:\n%s", stdout)
	}
	_ = stderr
}
