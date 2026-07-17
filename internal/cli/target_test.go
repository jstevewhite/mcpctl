package cli

import "testing"

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
