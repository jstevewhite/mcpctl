package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// Captures real os.Stdout so it verifies the block lands on stdout, not stderr.
func TestVersionSubcommandWritesToStdout(t *testing.T) {
	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w

	root, _ := NewRootCmd()
	root.SetArgs([]string{"version"})
	runErr := root.Execute()
	_ = w.Close()

	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	got := string(out)
	for _, want := range []string{"mcpctl version ", "commit: ", "built: ", "go: "} {
		if !strings.Contains(got, want) {
			t.Errorf("version output missing %q\ngot:\n%s", want, got)
		}
	}
}
