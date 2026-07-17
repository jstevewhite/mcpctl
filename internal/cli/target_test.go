package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEphemeralStdio(t *testing.T) {
	spec, toolArgs, err := resolveTarget(
		ServerFlags{Stdio: true},
		[]string{"echo"},             // tool-side (before --)
		[]string{"npx", "-y", "srv"}, // after --
		true, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != "npx" || len(spec.Args) != 2 || spec.Args[0] != "-y" {
		t.Fatalf("bad spec: %+v", spec)
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

func TestResolveURLUnsupported(t *testing.T) {
	_, _, err := resolveTarget(ServerFlags{URL: "https://x"}, nil, nil, false, "")
	if err == nil {
		t.Fatal("--url must return an unsupported-transport error in Phase 2")
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
	spec, _, err := resolveTarget(ServerFlags{Server: "local"}, nil, nil, false, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != "echo" || len(spec.Args) != 1 || spec.Args[0] != "hi" {
		t.Fatalf("bad spec: %+v", spec)
	}
}

func TestResolveNamedServerNotFound(t *testing.T) {
	cfg := writeCfg(t, "version = 1\n")
	if _, _, err := resolveTarget(ServerFlags{Server: "nope"}, nil, nil, false, cfg); err == nil {
		t.Fatal("expected a not-found error for an unknown --server")
	}
}

func TestResolveNamedServerNonStdio(t *testing.T) {
	cfg := writeCfg(t, "version = 1\n[servers.remote]\ntransport = \"streamable-http\"\nurl = \"https://x\"\n")
	if _, _, err := resolveTarget(ServerFlags{Server: "remote"}, nil, nil, false, cfg); err == nil {
		t.Fatal("expected a non-stdio-transport error")
	}
}
