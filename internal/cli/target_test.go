package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEphemeralStdio(t *testing.T) {
	target, toolArgs, err := resolveTarget(
		ServerFlags{Stdio: true},
		[]string{"echo"},             // tool-side (before --)
		[]string{"npx", "-y", "srv"}, // after --
		true, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if target.Stdio == nil || target.Stdio.Command != "npx" || len(target.Stdio.Args) != 2 || target.Stdio.Args[0] != "-y" {
		t.Fatalf("bad target: %+v", target)
	}
	if len(toolArgs) != 1 || toolArgs[0] != "echo" {
		t.Fatalf("bad toolArgs: %v", toolArgs)
	}
}

func TestResolveStdioRequiresServerArgv(t *testing.T) {
	_, _, err := resolveTarget(ServerFlags{Stdio: true}, nil, nil, false, "")
	if err == nil {
		t.Fatal("--stdio with no server command after -- must error")
	}
}

func TestResolveMutualExclusion(t *testing.T) {
	_, _, err := resolveTarget(ServerFlags{Stdio: true, Server: "x"}, nil, []string{"srv"}, true, "")
	if err == nil {
		t.Fatal("--stdio and --server together must error")
	}
	_, _, err = resolveTarget(ServerFlags{}, nil, nil, false, "")
	if err == nil {
		t.Fatal("no server selector must error")
	}
}

func TestResolveURLMutualExclusion(t *testing.T) {
	if _, _, err := resolveTarget(ServerFlags{URL: "https://x", Stdio: true}, nil, []string{"srv"}, true, ""); err == nil {
		t.Fatal("--url and --stdio together must error")
	}
	if _, _, err := resolveTarget(ServerFlags{URL: "https://x", Server: "s"}, nil, nil, false, ""); err == nil {
		t.Fatal("--url and --server together must error")
	}
}

func TestResolveEphemeralURL(t *testing.T) {
	t.Setenv("MCP_TOK", "secret")
	target, _, err := resolveTarget(ServerFlags{URL: "https://example.com/mcp", BearerEnv: "MCP_TOK"}, nil, nil, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.HTTP == nil || target.HTTP.URL != "https://example.com/mcp" {
		t.Fatalf("expected http target: %+v", target)
	}
	if target.HTTP.Header.Get("Authorization") != "Bearer secret" {
		t.Fatalf("bearer not resolved: %q", target.HTTP.Header.Get("Authorization"))
	}
}

func TestResolveURLMissingEnvErrors(t *testing.T) {
	_, _, err := resolveTarget(ServerFlags{URL: "https://x", BearerEnv: "NOPE_UNSET"}, nil, nil, false, "")
	if err == nil {
		t.Fatal("missing bearer env var must error before connecting")
	}
}

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveNamedServer(t *testing.T) {
	cfg := writeCfg(t, "version = 1\n[servers.local]\ntransport = \"stdio\"\ncommand = \"echo\"\nargs = [\"hi\"]\n")
	target, _, err := resolveTarget(ServerFlags{Server: "local"}, nil, nil, false, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.Stdio == nil || target.Stdio.Command != "echo" || len(target.Stdio.Args) != 1 || target.Stdio.Args[0] != "hi" {
		t.Fatalf("bad target: %+v", target)
	}
}

func TestResolveNamedServerNotFound(t *testing.T) {
	cfg := writeCfg(t, "version = 1\n")
	if _, _, err := resolveTarget(ServerFlags{Server: "nope"}, nil, nil, false, cfg); err == nil {
		t.Fatal("expected a not-found error for an unknown --server")
	}
}

func TestResolveNamedServerHTTP(t *testing.T) {
	t.Setenv("REMOTE_TOK", "s3cret")
	cfg := writeCfg(t, "version = 1\n[servers.remote]\ntransport = \"streamable-http\"\nurl = \"https://x\"\n[servers.remote.bearer_token]\nenv = \"REMOTE_TOK\"\n")
	target, _, err := resolveTarget(ServerFlags{Server: "remote"}, nil, nil, false, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.HTTP == nil || target.HTTP.URL != "https://x" {
		t.Fatalf("bad target: %+v", target)
	}
	if target.HTTP.Header.Get("Authorization") != "Bearer s3cret" {
		t.Fatalf("bearer not resolved: %q", target.HTTP.Header.Get("Authorization"))
	}
}
