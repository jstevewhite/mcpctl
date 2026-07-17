//go:build !windows

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// buildBinaries builds the mcpctl binary and the stdio test server once.
func buildBinaries(t *testing.T) (mcpctl, server string) {
	t.Helper()
	dir := t.TempDir()
	mcpctl = filepath.Join(dir, "mcpctl")
	server = filepath.Join(dir, "test-server")
	for _, b := range []struct{ out, pkg string }{
		{mcpctl, "github.com/jstevewhite/mcpctl/cmd/mcpctl"},
		{server, "github.com/jstevewhite/mcpctl/internal/testserver/stdio"},
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

func TestE2EToolsDescribe(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	stdout, _, code := run(t, mcpctl, "tools", "describe", "echo", "--stdio", "--", server)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "echo") {
		t.Fatalf("describe output missing tool name:\n%s", stdout)
	}
}

func TestE2EToolsDescribeNotFound(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	_, _, code := run(t, mcpctl, "tools", "describe", "nope", "--stdio", "--", server)
	if code != 7 {
		t.Fatalf("exit = %d, want 7 (tool not found)", code)
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

func TestE2EToolsCallEcho(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	stdout, _, code := run(t, mcpctl, "tools", "call", "echo", "--json", `{"message":"hello"}`, "--stdio", "--", server)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "hello") {
		t.Fatalf("expected echoed text:\n%s", stdout)
	}
}

func TestE2EToolsCallArgFlags(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	stdout, _, code := run(t, mcpctl, "tools", "call", "add", "--arg", "a=2", "--arg", "b=3", "--stdio", "--", server)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "5") {
		t.Fatalf("expected sum 5:\n%s", stdout)
	}
}

func TestE2EToolsCallIsErrorExit9(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	stdout, _, code := run(t, mcpctl, "tools", "call", "boom", "--stdio", "--", server)
	if code != 9 {
		t.Fatalf("exit = %d, want 9 (tool isError); out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "tool failed") {
		t.Fatalf("expected error content rendered:\n%s", stdout)
	}
}

func TestE2EToolsCallNotFoundExit7(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	_, _, code := run(t, mcpctl, "tools", "call", "nope", "--json", "{}", "--stdio", "--", server)
	if code != 7 {
		t.Fatalf("exit = %d, want 7", code)
	}
}

func TestE2EToolsCallValidationFailsExit8(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	// echo requires "message"; omit it → local validation fails, exit 8, no call sent.
	_, stderr, code := run(t, mcpctl, "tools", "call", "echo", "--json", "{}", "--stdio", "--", server)
	if code != 8 {
		t.Fatalf("exit = %d, want 8 (invalid arguments); stderr=%s", code, stderr)
	}
}

func TestE2EToolsCallNoValidateBypasses(t *testing.T) {
	mcpctl, server := buildBinaries(t)
	// "unchecked" declares a schema requiring "value", but (unlike every other
	// tool on the test server) is registered via the SDK's low-level, raw
	// AddTool, which performs no server-side schema validation of its own.
	// This isolates the assertion to mcpctl's own client-side validation:
	// echo's tools/call is auto-validated server-side by the SDK regardless
	// of what mcpctl does (confirmed empirically), so it can't demonstrate a
	// bypass — a schema-violating call to it fails the same way whether or
	// not mcpctl validates locally.
	//
	// Without --no-validate: local validation catches the missing required
	// "value" and exits 8 before the server is ever contacted.
	_, stderr, code := run(t, mcpctl, "tools", "call", "unchecked", "--json", "{}", "--stdio", "--", server)
	if code != 8 {
		t.Fatalf("control: exit = %d, want 8 (invalid arguments); stderr=%s", code, stderr)
	}
	// With --no-validate: local validation is skipped, the schema-violating
	// call is sent as-is, and the server's raw handler (which performs no
	// validation) accepts it.
	_, stderr, code = run(t, mcpctl, "--no-validate", "tools", "call", "unchecked", "--json", "{}", "--stdio", "--", server)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 with --no-validate; stderr=%s", code, stderr)
	}
}

// TestE2EToolsListMaxPagesCap exercises the --max-pages flag against the stdio
// test server, which exposes 8 tools at PageSize 2 → exactly 4 pages. A cap of
// 4 fetches them all; a cap of 3 trips the page-cap protocol error (exit 6)
// before the 4th page, proving the flag is wired through end-to-end.
func TestE2EToolsListMaxPagesCap(t *testing.T) {
	mcpctl, server := buildBinaries(t)

	stdout, _, code := run(t, mcpctl, "tools", "list", "--max-pages", "4", "--stdio", "--", server)
	if code != 0 {
		t.Fatalf("--max-pages 4 exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "echo") {
		t.Fatalf("expected full tool list with --max-pages 4:\n%s", stdout)
	}

	_, stderr, code := run(t, mcpctl, "tools", "list", "--max-pages", "3", "--stdio", "--", server)
	if code != 6 {
		t.Fatalf("--max-pages 3 exit = %d, want 6 (page-cap protocol error); stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "page cap") {
		t.Fatalf("expected page-cap message on stderr:\n%s", stderr)
	}
}

// newCLIHTTPServer returns an httptest.Server running a minimal MCP server
// with an `echo` tool over Streamable HTTP, for exercising the real mcpctl
// binary's --url path end-to-end. wrap, if non-nil, wraps the handler (e.g.
// to enforce auth before the MCP handler runs).
func newCLIHTTPServer(t *testing.T, wrap func(http.Handler) http.Handler) *httptest.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "cli-http-test-server", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo"},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			Message string `json:"message"`
		}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Message}}}, nil, nil
		})
	var h http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	if wrap != nil {
		h = wrap(h)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestE2EToolsListHTTP(t *testing.T) {
	srv := newCLIHTTPServer(t, nil)
	mcpctl, _ := buildBinaries(t)
	stdout, _, code := run(t, mcpctl, "tools", "list", "--url", srv.URL)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "echo") {
		t.Fatalf("expected echo tool over HTTP:\n%s", stdout)
	}
}

func TestE2EToolsCallEchoHTTP(t *testing.T) {
	srv := newCLIHTTPServer(t, nil)
	mcpctl, _ := buildBinaries(t)
	stdout, _, code := run(t, mcpctl, "tools", "call", "echo", "--json", `{"message":"hello-http"}`, "--url", srv.URL)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "hello-http") {
		t.Fatalf("expected echoed text over HTTP:\n%s", stdout)
	}
}

func TestE2EToolsListHTTPBearerAuth(t *testing.T) {
	srv := newCLIHTTPServer(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer topsecret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	mcpctl, _ := buildBinaries(t)
	t.Setenv("CLI_E2E_TOK", "topsecret")
	stdout, _, code := run(t, mcpctl, "tools", "list", "--url", srv.URL, "--bearer-env", "CLI_E2E_TOK")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "echo") {
		t.Fatalf("expected echo tool over authenticated HTTP:\n%s", stdout)
	}
}

func TestE2EToolsListHTTPMissingBearerEnvExit4(t *testing.T) {
	srv := newCLIHTTPServer(t, nil)
	mcpctl, _ := buildBinaries(t)
	_, _, code := run(t, mcpctl, "tools", "list", "--url", srv.URL, "--bearer-env", "CLI_E2E_UNSET_TOK")
	if code != 4 {
		t.Fatalf("exit = %d, want 4 (auth: missing env var before connecting)", code)
	}
}

func TestE2EServerAddListRemove(t *testing.T) {
	mcpctl, _ := buildBinaries(t)
	cfg := filepath.Join(t.TempDir(), "config.toml")

	// add
	_, _, code := run(t, mcpctl, "--config", cfg, "server", "add", "--name", "local", "--stdio", "--", "echo", "hi")
	if code != 0 {
		t.Fatalf("server add exit = %d", code)
	}
	// list shows it
	stdout, _, code := run(t, mcpctl, "--config", cfg, "server", "list")
	if code != 0 || !strings.Contains(stdout, "local") {
		t.Fatalf("server list exit=%d out=%s", code, stdout)
	}
	// duplicate add fails (exit 3)
	_, _, code = run(t, mcpctl, "--config", cfg, "server", "add", "--name", "local", "--stdio", "--", "echo")
	if code != 3 {
		t.Fatalf("duplicate add exit = %d, want 3", code)
	}
	// remove
	_, _, code = run(t, mcpctl, "--config", cfg, "server", "remove", "--name", "local")
	if code != 0 {
		t.Fatalf("server remove exit = %d", code)
	}
	// remove again fails (exit 3)
	_, _, code = run(t, mcpctl, "--config", cfg, "server", "remove", "--name", "local")
	if code != 3 {
		t.Fatalf("remove-missing exit = %d, want 3", code)
	}
}

func TestE2EToolsListHTTPViaConfigServer(t *testing.T) {
	srv := newCLIHTTPServer(t, nil)
	mcpctl, _ := buildBinaries(t)
	cfg := writeCfg(t, "version = 1\n[servers.remote]\ntransport = \"streamable-http\"\nurl = \""+srv.URL+"\"\n")
	stdout, _, code := run(t, mcpctl, "--config", cfg, "tools", "list", "--server", "remote")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "echo") {
		t.Fatalf("expected echo tool via config --server:\n%s", stdout)
	}
}
