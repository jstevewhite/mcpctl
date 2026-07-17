# mcpctl Phase 4B — Server Management Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `server list/show/add/remove` to manage the TOML configuration — `add` writes a named server definition from the transport flags without executing or connecting, `remove` deletes one, and `list`/`show` display them with secrets redacted.

**Architecture:** `internal/config` gains a `Save` (write TOML, owner-only perms). `internal/cli/server.go` holds the four subcommands; a redaction helper (using `auth.IsSensitive`) hides literal secret values before display while keeping env-var *references* visible. `add` reuses the same transport-flag parsing shape as the `tools` commands but builds a `config.ServerConfig` instead of dialing.

**Tech Stack:** Go 1.25, `github.com/pelletier/go-toml/v2`, the existing `internal/config`/`internal/auth`/`internal/output`.

## Global Constraints

From `mcpctl-spec.md` §4.2, §5, §12, §15.

- **`server add` never executes or connects** (§4.2): it validates the name is non-empty and does not conflict with an existing server, writes the config, and returns. It need not verify the command exists or the URL is reachable — but the resulting `ServerConfig` must pass `config.Validate`.
- **Secrets not literal by default (§15.3):** prefer env-var references. `--header-literal` is allowed but its help text must warn it writes a secret to disk; literal header/env values are redacted in `list`/`show` output.
- **Redaction (§4.2.1/§4.2.2/§12.2):** command, args, URL, env-variable *names*, and header *names* are shown. Environment-variable *values* and literal header *values* are redacted. In `json`/`yaml`, env-sourced fields are shown as their reference (env var name); literal secret values are replaced with a placeholder. Never print a token.
- **Config file perms (§15.6):** write the config with `0600` (owner-only); create the parent dir `0700` if missing.
- **Read-only commands must not create a file** (§5.1): `list`/`show` on a missing default config succeed with an empty/no-servers result; they do not write.
- **Exit codes:** config error → 3; usage → 2; a `show`/`remove` of an unknown server → config error (3).
- **Output:** `server list` is a collection (human table + json/jsonl/yaml, one server per line in jsonl). `server show` is a single object. Reuse `internal/output` format parsing.

---

## File Structure

- Create: `internal/config/save.go` (+ test).
- Modify: `internal/config/config.go` (add `omitempty` to optional `ServerConfig` toml tags so saved files stay clean).
- Create: `internal/cli/server.go` (the four subcommands + a redaction helper + `newServerCmd`).
- Modify: `internal/cli/root.go` (attach `newServerCmd`).
- Test: `internal/cli/server_test.go`, and `internal/cli/e2e_test.go` (add/list/remove round trip).

---

### Task 1: `config.Save` + clean marshalling

**Files:**
- Create: `internal/config/save.go`
- Modify: `internal/config/config.go` (toml tag `omitempty`)
- Test: `internal/config/save_test.go`

**Interfaces:**
- Consumes: `github.com/pelletier/go-toml/v2`, `apperror`, `os`, `path/filepath`.
- Produces: `func config.Save(path string, cfg *Config) error` — marshals to TOML, creates the parent dir (0700), writes 0600.

- [ ] **Step 1: Add omitempty to optional ServerConfig fields**

In `internal/config/config.go`, update the `ServerConfig` struct tags so optional fields are omitted when empty (keeps saved TOML clean; does not affect loading):
```go
type ServerConfig struct {
	Transport   string            `toml:"transport"`
	Command     string            `toml:"command,omitempty"`
	Args        []string          `toml:"args,omitempty"`
	CWD         string            `toml:"cwd,omitempty"`
	Env         map[string]string `toml:"env,omitempty"`
	URL         string            `toml:"url,omitempty"`
	Headers     map[string]string `toml:"headers,omitempty"`
	HeaderEnv   map[string]string `toml:"header_env,omitempty"`
	BearerToken *TokenSource      `toml:"bearer_token,omitempty"`
}
```
(Leave `Transport` without omitempty — it is always required.)

- [ ] **Step 2: Write the failing round-trip test**

Create `internal/config/save_test.go`:
```go
package config

import (
	"path/filepath"
	"testing"
)

func TestSaveRoundTrip(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Servers: map[string]ServerConfig{
			"local": {Transport: "stdio", Command: "npx", Args: []string{"-y", "srv"}},
			"remote": {
				Transport: "streamable-http", URL: "https://example.com/mcp",
				HeaderEnv:   map[string]string{"Authorization": "MCP_AUTH"},
				BearerToken: nil,
			},
		},
	}
	path := filepath.Join(t.TempDir(), "sub", "config.toml") // parent dir does not exist yet
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if got.Version != 1 || len(got.Servers) != 2 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	l := got.Servers["local"]
	if l.Command != "npx" || len(l.Args) != 2 {
		t.Fatalf("local server not preserved: %+v", l)
	}
	r := got.Servers["remote"]
	if r.URL != "https://example.com/mcp" || r.HeaderEnv["Authorization"] != "MCP_AUTH" {
		t.Fatalf("remote server not preserved: %+v", r)
	}
}

func TestSaveWritesOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, &Config{Version: 1}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perms = %o, want 600", perm)
	}
}
```
(Add `"os"` to the test imports.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestSave -v`
Expected: FAIL — `undefined: Save`.

- [ ] **Step 4: Write save.go**

Create `internal/config/save.go`:
```go
package config

import (
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"

	"mcpctl/internal/apperror"
)

// Save writes cfg to path as TOML with owner-only (0600) permissions, creating
// the parent directory (0700) if needed.
func Save(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "encode config")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return apperror.Wrap(apperror.KindConfig, err, "create config directory %s", dir)
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "write config %s", path)
	}
	// os.WriteFile only sets perms when creating a file; enforce 0600 so a
	// pre-existing looser-perm config is tightened (literal secrets stay owner-only).
	if err := os.Chmod(path, 0o600); err != nil {
		return apperror.Wrap(apperror.KindConfig, err, "restrict config permissions %s", path)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -v && go vet ./... && gofmt -l internal/config/`
Expected: PASS — round-trip preserves servers; file is 0600.

- [ ] **Step 6: Commit**

```bash
git add internal/config/save.go internal/config/config.go internal/config/save_test.go
git commit -m "feat(config): Save config to TOML (0600, clean omitempty output)"
```

---

### Task 2: `server list` + `server show` (read-only, redacted)

**Files:**
- Create: `internal/cli/server.go`
- Modify: `internal/cli/root.go` (attach `newServerCmd`)
- Test: `internal/cli/server_test.go`

**Interfaces:**
- Consumes: `config.LoadResolved`, `config.ServerConfig`, `auth.IsSensitive`, `output.ParseFormat`, `GlobalFlags`.
- Produces: `func newServerCmd(g *GlobalFlags) *cobra.Command` with `list` and `show` subcommands; `func redactServer(config.ServerConfig) config.ServerConfig` (unexported).

- [ ] **Step 1: Write the failing test**

Create `internal/cli/server_test.go`:
```go
package cli

import (
	"mcpctl/internal/config"
	"testing"
)

func TestRedactServerHidesSecrets(t *testing.T) {
	sc := config.ServerConfig{
		Transport: "streamable-http",
		URL:       "https://x",
		Env:       map[string]string{"LOG_LEVEL": "warn", "API_KEY": "topsecret"},
		Headers:   map[string]string{"X-Api-Key": "literal-secret"},
		HeaderEnv: map[string]string{"Authorization": "MCP_AUTH"},
	}
	got := redactServer(sc)
	if got.Env["LOG_LEVEL"] != "warn" {
		t.Errorf("non-sensitive env value should be shown, got %q", got.Env["LOG_LEVEL"])
	}
	if got.Env["API_KEY"] == "topsecret" {
		t.Error("sensitive env value must be redacted")
	}
	if got.Headers["X-Api-Key"] == "literal-secret" {
		t.Error("literal header value must be redacted")
	}
	if got.HeaderEnv["Authorization"] != "MCP_AUTH" {
		t.Errorf("header_env reference (an env var name, not a secret) should be shown, got %q", got.HeaderEnv["Authorization"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestRedactServer -v`
Expected: FAIL — `undefined: redactServer`.

- [ ] **Step 3: Write server.go (list + show + redaction)**

Create `internal/cli/server.go`:
```go
package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"mcpctl/internal/apperror"
	"mcpctl/internal/auth"
	"mcpctl/internal/config"
	"mcpctl/internal/output"
)

const redacted = "<redacted>"

func newServerCmd(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage saved MCP server definitions",
	}
	cmd.AddCommand(newServerListCmd(g), newServerShowCmd(g))
	return cmd
}

// redactServer returns a copy safe to display: literal secret values hidden,
// env-var references (which are names, not secrets) shown as-is.
func redactServer(sc config.ServerConfig) config.ServerConfig {
	out := sc
	if sc.Env != nil {
		out.Env = make(map[string]string, len(sc.Env))
		for k, v := range sc.Env {
			if auth.IsSensitive(k) {
				out.Env[k] = redacted
			} else {
				out.Env[k] = v
			}
		}
	}
	if sc.Headers != nil {
		out.Headers = make(map[string]string, len(sc.Headers))
		for k := range sc.Headers {
			out.Headers[k] = redacted // literal header values are potential secrets
		}
	}
	// HeaderEnv values are env var NAMES (references), not secrets — left as-is.
	// BearerToken.Env is likewise a name — left as-is.
	return out
}

func newServerListCmd(g *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			cfg, err := config.LoadResolved(g.Config)
			if err != nil {
				return err
			}
			names := make([]string, 0, len(cfg.Servers))
			for n := range cfg.Servers {
				names = append(names, n)
			}
			sort.Strings(names)

			if f == output.FormatHuman {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tTRANSPORT\tURL / COMMAND")
				for _, n := range names {
					sc := cfg.Servers[n]
					fmt.Fprintf(tw, "%s\t%s\t%s\n", n, sc.Transport, endpointSummary(sc))
				}
				return tw.Flush()
			}
			return output.Servers(cmd.OutOrStdout(), f, redactServerMap(cfg.Servers, names))
		},
	}
}

func newServerShowCmd(g *GlobalFlags) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a saved server's details",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			if name == "" {
				return apperror.Usage("server show requires --name")
			}
			cfg, err := config.LoadResolved(g.Config)
			if err != nil {
				return err
			}
			sc, ok := cfg.Servers[name]
			if !ok {
				return apperror.Config("no server named %q in configuration", name)
			}
			red := redactServer(sc)
			if f == output.FormatHuman {
				return showServerHuman(cmd.OutOrStdout(), name, red)
			}
			return output.Server(cmd.OutOrStdout(), f, name, red)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "server name")
	return cmd
}

func endpointSummary(sc config.ServerConfig) string {
	if sc.Transport == config.TransportStdio {
		return strings.TrimSpace(sc.Command + " " + strings.Join(sc.Args, " "))
	}
	return sc.URL
}

func redactServerMap(servers map[string]config.ServerConfig, order []string) []output.NamedServer {
	out := make([]output.NamedServer, 0, len(order))
	for _, n := range order {
		out = append(out, output.NamedServer{Name: n, Server: redactServer(servers[n])})
	}
	return out
}

func showServerHuman(w io.Writer, name string, sc config.ServerConfig) error {
	fmt.Fprintf(w, "Name:        %s\n", name)
	fmt.Fprintf(w, "Transport:   %s\n", sc.Transport)
	if sc.Transport == config.TransportStdio {
		fmt.Fprintf(w, "Command:     %s\n", endpointSummary(sc))
		if sc.CWD != "" {
			fmt.Fprintf(w, "CWD:         %s\n", sc.CWD)
		}
	} else {
		fmt.Fprintf(w, "URL:         %s\n", sc.URL)
	}
	fmt.Fprintln(w, "Environment:")
	if len(sc.Env) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, k := range sortedKeys(sc.Env) {
			fmt.Fprintf(w, "  %s=%s\n", k, sc.Env[k])
		}
	}
	writeNameSet(w, "Headers (literal):", sortedKeys(sc.Headers))
	writeEnvRefs(w, "Headers (from env):", sc.HeaderEnv)
	if sc.BearerToken != nil {
		fmt.Fprintf(w, "Bearer:      <env:%s>\n", sc.BearerToken.Env)
	} else {
		fmt.Fprintln(w, "Bearer:      (none)")
	}
	if len(sc.Headers) > 0 {
		fmt.Fprintln(w, "\nwarning: literal header values are stored in the config file in plaintext")
	}
	return nil
}

func writeNameSet(w io.Writer, label string, names []string) {
	fmt.Fprintln(w, label)
	if len(names) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	for _, n := range names {
		fmt.Fprintf(w, "  %s\n", n)
	}
}

func writeEnvRefs(w io.Writer, label string, m map[string]string) {
	fmt.Fprintln(w, label)
	if len(m) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	for _, k := range sortedKeys(m) {
		fmt.Fprintf(w, "  %s=<env:%s>\n", k, m[k])
	}
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
```
Add `"io"` to the imports. NOTE: this references `output.Servers`, `output.Server`, and `output.NamedServer` — add them in the next step.

- [ ] **Step 4: Add server renderers to internal/output**

Create `internal/output/server.go`:
```go
package output

import (
	"io"

	"mcpctl/internal/config"
)

// NamedServer pairs a server name with its (already-redacted) definition.
type NamedServer struct {
	Name   string              `json:"name"`
	Server config.ServerConfig `json:"server"`
}

// Servers renders a list of servers in a machine-readable format.
func Servers(w io.Writer, f Format, servers []NamedServer) error {
	switch f {
	case FormatYAML:
		return writeYAML(w, servers)
	case FormatJSONL:
		items := make([]any, len(servers))
		for i := range servers {
			items[i] = servers[i]
		}
		return writeJSONLines(w, items)
	default: // json
		return writeJSONIndent(w, servers)
	}
}

// Server renders a single server in a machine-readable format.
func Server(w io.Writer, f Format, name string, sc config.ServerConfig) error {
	ns := NamedServer{Name: name, Server: sc}
	switch f {
	case FormatYAML:
		return writeYAML(w, ns)
	default: // json or jsonl (single object)
		if f == FormatJSONL {
			return writeJSONCompact(w, ns)
		}
		return writeJSONIndent(w, ns)
	}
}
```
This makes `internal/output` import `internal/config` — that is allowed (config is SDK-free; the SDK-confinement rule only bars the MCP SDK). NOTE: `config.ServerConfig`'s toml-tagged fields need JSON tags for clean json/yaml output; if `ServerConfig` lacks json tags, add them alongside the toml tags (e.g. `json:"command,omitempty"`), or the JSON keys will be Go-cased — do this in `internal/config/config.go` in this step and note it in your report.

- [ ] **Step 5: Attach server to root**

In `internal/cli/root.go`, next to the other `AddCommand` calls, add:
```go
	root.AddCommand(newServerCmd(g))
```

- [ ] **Step 6: Run tests + gate**

Run: `go test ./internal/cli/ ./internal/output/ ./internal/config/ -v && go test ./... && go vet ./... && gofmt -l ./internal/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/server.go internal/cli/server_test.go internal/cli/root.go internal/output/server.go internal/config/config.go
git commit -m "feat(cli): server list and show with secret redaction"
```

---

### Task 3: `server add` + `server remove`

**Files:**
- Modify: `internal/cli/server.go` (add `add`/`remove`)
- Test: `internal/cli/server_test.go` (extend), `internal/cli/e2e_test.go` (round trip)

**Interfaces:**
- Consumes: `config.LoadResolved`/`config.Save`/`config.DefaultPath`, `ServerFlags`/`authSpecFromFlags`, `config.Validate`.
- Produces: `newServerAddCmd(g)`, `newServerRemoveCmd(g)`; `func serverConfigFromFlags(sf ServerFlags, afterDash []string, hasDash bool) (config.ServerConfig, error)`.

- [ ] **Step 1: Add the add/remove commands**

In `internal/cli/server.go`, attach them in `newServerCmd`: `cmd.AddCommand(newServerListCmd(g), newServerShowCmd(g), newServerAddCmd(g), newServerRemoveCmd(g))`. Then add:
```go
func newServerAddCmd(g *GlobalFlags) *cobra.Command {
	var name string
	var sf ServerFlags
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a server definition to the configuration (does not connect)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return apperror.Usage("server add requires a non-empty --name")
			}
			dash := cmd.ArgsLenAtDash()
			var afterDash []string
			hasDash := dash >= 0
			if hasDash {
				afterDash = args[dash:]
			}
			sc, err := serverConfigFromFlags(sf, afterDash, hasDash)
			if err != nil {
				return err
			}

			path, _, err := config.Resolve(g.Config)
			if err != nil {
				return apperror.Wrap(apperror.KindConfig, err, "resolve config path")
			}
			cfg, err := config.LoadResolved(g.Config)
			if err != nil {
				return err
			}
			if _, exists := cfg.Servers[name]; exists {
				return apperror.Config("a server named %q already exists (remove it first)", name)
			}
			if cfg.Servers == nil {
				cfg.Servers = map[string]config.ServerConfig{}
			}
			cfg.Servers[name] = sc
			if err := cfg.Validate(); err != nil { // the new entry must be valid
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "added server %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "server name (required)")
	addServerFlags(cmd, &sf) // reuse --stdio/--url/--header-env/--header-literal/--bearer-env
	return cmd
}

func newServerRemoveCmd(g *GlobalFlags) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a server definition from the configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return apperror.Usage("server remove requires --name")
			}
			path, _, err := config.Resolve(g.Config)
			if err != nil {
				return apperror.Wrap(apperror.KindConfig, err, "resolve config path")
			}
			cfg, err := config.LoadResolved(g.Config)
			if err != nil {
				return err
			}
			if _, ok := cfg.Servers[name]; !ok {
				return apperror.Config("no server named %q in configuration", name)
			}
			delete(cfg.Servers, name)
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "removed server %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "server name (required)")
	return cmd
}

// serverConfigFromFlags builds a ServerConfig from the transport flags without
// resolving env vars (only names are stored). Exactly one of --stdio/--url.
func serverConfigFromFlags(sf ServerFlags, afterDash []string, hasDash bool) (config.ServerConfig, error) {
	switch {
	case sf.Stdio && sf.URL != "":
		return config.ServerConfig{}, apperror.Usage("--stdio and --url are mutually exclusive")
	case sf.Stdio:
		if !hasDash || len(afterDash) == 0 {
			return config.ServerConfig{}, apperror.Usage("--stdio requires a server command after `--`")
		}
		return config.ServerConfig{Transport: config.TransportStdio, Command: afterDash[0], Args: afterDash[1:]}, nil
	case sf.URL != "":
		as, err := authSpecFromFlags(sf.HeaderEnv, sf.HeaderLiteral, sf.BearerEnv)
		if err != nil {
			return config.ServerConfig{}, err
		}
		sc := config.ServerConfig{Transport: config.TransportHTTP, URL: sf.URL, Headers: as.Headers, HeaderEnv: as.HeaderEnv}
		if as.BearerEnv != "" {
			sc.BearerToken = &config.TokenSource{Env: as.BearerEnv}
		}
		return sc, nil
	default:
		return config.ServerConfig{}, apperror.Usage("server add requires --stdio (with `-- command`) or --url")
	}
}
```
NOTE: normalize empty maps to nil before storing so `omitempty` keeps the saved TOML clean — if `as.Headers`/`as.HeaderEnv` are empty maps, set them to nil in `sc`.

- [ ] **Step 2: Add tests**

In `internal/cli/server_test.go`, add a unit test for `serverConfigFromFlags` (stdio + url + mutual exclusion + missing). In `internal/cli/e2e_test.go`, add a round-trip e2e using an explicit `--config` path in a temp dir:
```go
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
```
(`filepath` is already imported in e2e_test.go.)

- [ ] **Step 3: Run the gate**

Run:
```bash
go test ./internal/cli/ -run 'TestServerConfig|TestRedactServer|TestE2EServer' -v
go test ./... && go test -race ./internal/cli/ && go vet ./... && gofmt -l ./internal/
```
Expected: PASS — add writes config, list shows it, duplicate/remove-missing exit 3, remove deletes.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/server.go internal/cli/server_test.go internal/cli/e2e_test.go
git commit -m "feat(cli): server add and remove (validate, write config, no connect)"
```

---

## Phase 4B Acceptance

- `make check` passes (build, test, `-race`, vet, staticcheck) under Go 1.25.
- `server add --name N --stdio -- cmd...` / `--url ... --bearer-env VAR` writes a valid config entry (0600) without connecting; a duplicate name errors (exit 3); an invalid resulting config errors (exit 3).
- `server list` shows NAME/TRANSPORT/URL-or-COMMAND (human) and one server per line (jsonl); `server show --name N` shows details; both redact env values and literal header values, showing env-var *names/references*; a missing config yields an empty list without creating a file; an unknown `show`/`remove` name errors (exit 3).
- `server remove --name N` deletes and rewrites the config.
- SDK not imported by any new non-test code.

---

## Post-review hardening (whole-Phase-4 review)

Applied on top of 4A + 4B (commits `dee7cf7`, `f75af3a`, `dae2dc6`, `cc07693`):

- **Terminal-escape hardening** (`internal/output/human.go`, §15.9): `toolResultHuman` sanitizes server-controlled `MIMEType`/`URI` on image/audio/resource blocks (previously only text was sanitized — a real injection gap); tool-list table cells collapse newlines/tabs so an untrusted tool Name/Description can't forge a table row/column.
- **Full env redaction in machine output** (`internal/cli/server.go`, §4.2.1): `redactServer(sc, machine)` — json/yaml `server show`/`list` (the secret-exfil channel) redact ALL env values, while interactive human `show` keeps the key-heuristic so benign values like `LOG_LEVEL=warn` stay visible (§4.2.2). `HeaderEnv` is now deep-copied.
- **Atomic config write** (`internal/config/save.go`, §15.6): `Save` writes a 0600 temp file then `os.Rename`s over the target — a crash can't truncate the config, and there's no loose-perm window on overwrite.
- **`go mod tidy`**: `jsonschema/v6` and `sigs.k8s.io/yaml` moved to direct requires.

Deferred (test backlog): jsonl single-object + `SchemaUnusableError` warn-path e2e; `server add` rejecting stray pre-`--` positional args; the corrupt-existing-config error path in `add`.

---

## After Phase 4

Merge `v4-ux-validation` to `main`. Remaining: Phase 5 (release — README, shell completions, GoReleaser artifacts); module-path migration from local `mcpctl` to `github.com/<owner>/mcpctl` on GitHub-repo creation.
