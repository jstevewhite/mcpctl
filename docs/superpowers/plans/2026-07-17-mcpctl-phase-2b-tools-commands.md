# mcpctl Phase 2B — Tools Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the user-facing `tools list`, `tools describe`, and `tools call` commands on top of the Plan 2A stdio client, with `--json`/`--json-file`/`--arg` argument input, named + ephemeral-stdio server selection, human and JSON output that faithfully preserves MCP results, command-scoped timeouts/signals, and correct exit codes.

**Architecture:** Command handlers in `internal/cli` resolve a server (named from config, or ephemeral `--stdio`) into an SDK-free `client.StdioSpec`, dial it via `client.DialStdio`, and render results through `internal/output` — never importing the MCP SDK. Argument parsing lives in `internal/arguments`. `--url`/HTTP is deferred to Phase 3 (the flag resolves to an unsupported-transport error). This plan first closes the boundary-type gaps the 2A review flagged (annotations, metadata, resource fields, JSON tags) so `describe`/output can satisfy §10.2/§11.1.

**Tech Stack:** Go 1.25, `github.com/spf13/cobra`, the Plan 2A `internal/client`, standard library `encoding/json`.

## Global Constraints

Copied from `mcpctl-spec.md`.

- **SDK confinement:** command/output/arguments packages must NOT import `github.com/modelcontextprotocol/go-sdk`. Only `internal/client` does.
- **Output contract:** results→stdout, diagnostics/logs→stderr. Machine-readable stdout carries no logs/color/warnings. JSON output is one valid JSON document.
- **Server selection:** exactly one of `--server`, `--stdio`, `--url` per tools command (mutually exclusive). `--url` returns an unsupported-transport error (Phase 3).
- **Ephemeral stdio grammar (§4.3.1):** `mcpctl tools <sub> --stdio [TOOL] [tool-arg flags] -- <server-command> [args...]`. Tool name + `--json`/`--json-file`/`--arg` come **before** `--`; everything **after the first `--`** is the server argv, verbatim. `--stdio` with no server command after `--` is a usage error. The command is executed directly, never via a shell.
- **Argument input (§4.3.2):** exactly one of `--json STRING`, `--json-file PATH` (`-` = stdin), repeated `--arg KEY=VALUE` (mutually exclusive). `--json`/`--json-file` must decode to a JSON **object** (reject array/string/number/bool/null at top level). `--arg`: split on first `=`; parse value as JSON if valid, else string; reject duplicate keys; empty key invalid. Document that numeric-looking values decode as numbers (`version=1.10` → `1.1`).
- **Exit codes:** usage → 2; config → 3; auth → 4; connection/transport → 5; MCP protocol/init → 6; tool not found → 7; invalid arguments → 8; tool `isError=true` → 9; timeout → 10; interrupt → 130. `main.go` maps via `apperror.ExitCode` (Phase 1).
- **Tool invocation (§11):** before calling, retrieve the tool list and confirm the tool exists; if not, do NOT send `tools/call` — return tool-not-found (7). A tool result with `isError=true` is a successful exchange but a failed execution → exit 9. Local JSON-Schema validation is Phase 4 (not this plan).
- **Timeouts (§14):** `connect-timeout` (default 15s) bounds dial+initialize; `timeout` (default 30s) bounds the operation. Cancellation propagates to the client. Signals (SIGINT/SIGTERM) cancel the command context → exit 130.
- **Pagination:** `tools list`/`describe` retrieve all pages via `client.ListAllTools`, default cap **1000** pages (spec §4.3.3).
- **Human output truncation:** `tools list` truncates descriptions to a reasonable width; sanitize control characters is Phase 4 (note only).
- **Verified SDK field names (for the Task 1 boundary extension):** `mcp.ToolAnnotations{Title string, ReadOnlyHint bool, DestructiveHint *bool, IdempotentHint bool, OpenWorldHint *bool}`; `mcp.Tool.Annotations *ToolAnnotations`, `mcp.Tool.Meta` (embedded `Meta`, `map[string]any`); `mcp.CallToolResult.Meta` (embedded); `mcp.EmbeddedResource.Resource *ResourceContents{URI, MIMEType, Text string, Blob []byte}`; `mcp.ResourceLink{URI, Name, MIMEType string}`.

---

## File Structure

Created/modified in this plan:

- Modify: `internal/client/types.go` (add fields + JSON tags), `internal/client/convert.go` (convert new fields), `internal/client/stdio.go` (extract `collectAllTools`), plus tests.
- Create: `internal/arguments/parse.go` (+ test).
- Create: `internal/cli/target.go` (server selection + ephemeral grammar → `client.StdioSpec`) (+ test).
- Create: `internal/cli/context.go` (command-scoped signal + timeout context).
- Create: `internal/output/output.go`, `internal/output/human.go`, `internal/output/json.go` (+ tests).
- Create: `internal/cli/tools.go` (the `tools` parent + `list`/`describe`/`call` subcommands); modify `internal/cli/root.go` to attach `tools`.
- Create: `internal/cli/e2e_test.go` (builds the binary; runs commands against the test server).

---

### Task 1: Boundary completeness (annotations, metadata, resources, JSON tags) + pagination testability

**Files:**
- Modify: `internal/client/types.go`, `internal/client/convert.go`, `internal/client/stdio.go`
- Test: `internal/client/convert_test.go` (extend), `internal/client/list_test.go` (add cursor-loop unit test)

**Interfaces:**
- Consumes: SDK types.
- Produces: extended `ToolInfo` (`Annotations *ToolAnnotations`, `Meta map[string]any`), `ToolAnnotations` struct, `ToolResult.Meta`, `ContentBlock` resource fields (`URI`, `Name`), JSON tags on all boundary types; `func collectAllTools(maxPages int, fetch func(cursor string) (ToolPage, error)) ([]ToolInfo, error)` (extracted from `ListAllTools`).

- [ ] **Step 1: Extend the boundary types with JSON tags**

In `internal/client/types.go`, replace the `ToolInfo`, `ContentBlock`, and `ToolResult` structs and add `ToolAnnotations`:
```go
// ToolInfo is an SDK-free description of a tool.
type ToolInfo struct {
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	Title        string           `json:"title,omitempty"`
	InputSchema  any              `json:"inputSchema,omitempty"`
	OutputSchema any              `json:"outputSchema,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Meta         map[string]any   `json:"meta,omitempty"`
}

// ToolAnnotations mirrors the SDK's tool hints, SDK-free.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// ContentBlock is an SDK-free representation of one content item.
type ContentBlock struct {
	Kind     ContentKind `json:"kind"`
	Text     string      `json:"text,omitempty"`     // KindText / embedded resource text
	MIMEType string      `json:"mimeType,omitempty"` // image/audio/resource
	Data     []byte      `json:"data,omitempty"`     // image/audio bytes / resource blob
	URI      string      `json:"uri,omitempty"`      // KindResource
	Name     string      `json:"name,omitempty"`     // resource link name
}

// ToolResult is an SDK-free representation of a tools/call result.
type ToolResult struct {
	Content    []ContentBlock `json:"content"`
	Structured any            `json:"structuredContent,omitempty"`
	IsError    bool           `json:"isError,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
}
```
Leave `ContentKind` (+consts), `ServerInfo`, `ToolPage`, `StdioSpec` unchanged.

- [ ] **Step 2: Write failing conversion tests**

In `internal/client/convert_test.go`, add:
```go
func TestToToolInfoAnnotationsAndMeta(t *testing.T) {
	ro := true
	got := toToolInfo(&mcp.Tool{
		Name:        "t",
		Annotations: &mcp.ToolAnnotations{Title: "T", ReadOnlyHint: true, DestructiveHint: &ro},
		Meta:        mcp.Meta{"k": "v"},
	})
	if got.Annotations == nil || got.Annotations.Title != "T" || !got.Annotations.ReadOnlyHint {
		t.Fatalf("annotations not converted: %+v", got.Annotations)
	}
	if got.Annotations.DestructiveHint == nil || !*got.Annotations.DestructiveHint {
		t.Fatalf("destructiveHint pointer not preserved")
	}
	if got.Meta["k"] != "v" {
		t.Fatalf("meta not converted: %+v", got.Meta)
	}
}

func TestToContentBlockResource(t *testing.T) {
	emb := toContentBlock(&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{
		URI: "file:///x", MIMEType: "text/plain", Text: "hi", Blob: []byte{1},
	}})
	if emb.Kind != KindResource || emb.URI != "file:///x" || emb.Text != "hi" || len(emb.Data) != 1 {
		t.Fatalf("embedded resource not converted: %+v", emb)
	}
	link := toContentBlock(&mcp.ResourceLink{URI: "https://y", Name: "y", MIMEType: "text/html"})
	if link.Kind != KindResource || link.URI != "https://y" || link.Name != "y" {
		t.Fatalf("resource link not converted: %+v", link)
	}
}
```

- [ ] **Step 3: Run the tests to see them fail**

Run: `go test ./internal/client/ -run 'AnnotationsAndMeta|ContentBlockResource' -v`
Expected: FAIL — new fields/mappings not present.

- [ ] **Step 4: Extend the conversions**

In `internal/client/convert.go`, update `toToolInfo`, `toToolResult`, `toContentBlock` and add `toAnnotations`:
```go
func toToolInfo(t *mcp.Tool) ToolInfo {
	return ToolInfo{
		Name:         t.Name,
		Description:  t.Description,
		Title:        t.Title,
		InputSchema:  t.InputSchema,
		OutputSchema: t.OutputSchema,
		Annotations:  toAnnotations(t.Annotations),
		Meta:         map[string]any(t.Meta),
	}
}

func toAnnotations(a *mcp.ToolAnnotations) *ToolAnnotations {
	if a == nil {
		return nil
	}
	return &ToolAnnotations{
		Title:           a.Title,
		ReadOnlyHint:    a.ReadOnlyHint,
		DestructiveHint: a.DestructiveHint,
		IdempotentHint:  a.IdempotentHint,
		OpenWorldHint:   a.OpenWorldHint,
	}
}

func toToolResult(r *mcp.CallToolResult) ToolResult {
	out := ToolResult{
		Structured: r.StructuredContent,
		IsError:    r.IsError,
		Meta:       map[string]any(r.Meta),
	}
	for _, c := range r.Content {
		out.Content = append(out.Content, toContentBlock(c))
	}
	return out
}

func toContentBlock(c mcp.Content) ContentBlock {
	switch v := c.(type) {
	case *mcp.TextContent:
		return ContentBlock{Kind: KindText, Text: v.Text}
	case *mcp.ImageContent:
		return ContentBlock{Kind: KindImage, MIMEType: v.MIMEType, Data: v.Data}
	case *mcp.AudioContent:
		return ContentBlock{Kind: KindAudio, MIMEType: v.MIMEType, Data: v.Data}
	case *mcp.EmbeddedResource:
		cb := ContentBlock{Kind: KindResource}
		if v.Resource != nil {
			cb.URI = v.Resource.URI
			cb.MIMEType = v.Resource.MIMEType
			cb.Text = v.Resource.Text
			cb.Data = v.Resource.Blob
		}
		return cb
	case *mcp.ResourceLink:
		return ContentBlock{Kind: KindResource, URI: v.URI, MIMEType: v.MIMEType, Name: v.Name}
	default:
		return ContentBlock{Kind: KindUnknown}
	}
}
```

- [ ] **Step 5: Extract the pagination loop for testability**

In `internal/client/stdio.go`, replace the body of `ListAllTools` so it delegates to a free function, and add that function:
```go
func (c *stdioClient) ListAllTools(ctx context.Context, maxPages int) ([]ToolInfo, error) {
	return collectAllTools(maxPages, func(cursor string) (ToolPage, error) {
		return c.ListTools(ctx, cursor)
	})
}

// collectAllTools follows pagination via fetch, capping at maxPages and
// erroring on a repeated cursor (a looping server).
func collectAllTools(maxPages int, fetch func(cursor string) (ToolPage, error)) ([]ToolInfo, error) {
	var all []ToolInfo
	seen := map[string]bool{}
	cursor := ""
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, apperror.New(apperror.KindProtocol,
				"tools/list exceeded the page cap (%d pages); server may be paginating without end", maxPages)
		}
		p, err := fetch(cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, p.Tools...)
		if p.NextCursor == "" {
			return all, nil
		}
		if seen[p.NextCursor] {
			return nil, apperror.New(apperror.KindProtocol,
				"tools/list returned a repeated cursor; server is looping")
		}
		seen[p.NextCursor] = true
		cursor = p.NextCursor
	}
}
```

- [ ] **Step 6: Add a repeated-cursor unit test**

In `internal/client/list_test.go`, add (no live server needed):
```go
func TestCollectAllToolsRepeatedCursor(t *testing.T) {
	calls := 0
	_, err := collectAllTools(1000, func(cursor string) (ToolPage, error) {
		calls++
		return ToolPage{Tools: []ToolInfo{{Name: "x"}}, NextCursor: "same"}, nil
	})
	if err == nil {
		t.Fatal("expected a repeated-cursor error")
	}
	if calls > 3 {
		t.Fatalf("should stop quickly on a repeated cursor, made %d calls", calls)
	}
}

func TestCollectAllToolsPageCap(t *testing.T) {
	_, err := collectAllTools(2, func(cursor string) (ToolPage, error) {
		return ToolPage{Tools: []ToolInfo{{Name: "x"}}, NextCursor: "next-" + cursor}, nil
	})
	if err == nil {
		t.Fatal("expected a page-cap error")
	}
}
```

- [ ] **Step 7: Run all client tests + vet + gofmt**

Run: `go test ./internal/client/ -v && go vet ./... && gofmt -l internal/client/`
Expected: PASS; clean.

- [ ] **Step 8: Commit**

```bash
git add internal/client/
git commit -m "feat(client): carry annotations/metadata/resources across the boundary; make pagination unit-testable"
```

---

### Task 2: Argument parsing (`internal/arguments`)

**Files:**
- Create: `internal/arguments/parse.go`
- Test: `internal/arguments/parse_test.go`

**Interfaces:**
- Consumes: `apperror.Usage`/`KindInvalidArgs`, `encoding/json`.
- Produces: `func arguments.Parse(jsonStr, jsonFile string, argKVs []string, stdin io.Reader) (map[string]any, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/arguments/parse_test.go`:
```go
package arguments

import (
	"strings"
	"testing"
)

func TestParseMutualExclusion(t *testing.T) {
	if _, err := Parse(`{"a":1}`, "", []string{"b=2"}, nil); err == nil {
		t.Fatal("expected error when --json and --arg are both set")
	}
}

func TestParseJSONMustBeObject(t *testing.T) {
	if _, err := Parse(`[1,2]`, "", nil, nil); err == nil {
		t.Fatal("array top-level must be rejected")
	}
	got, err := Parse(`{"q":"x"}`, "", nil, nil)
	if err != nil || got["q"] != "x" {
		t.Fatalf("object parse failed: %v %v", got, err)
	}
}

func TestParseJSONFileStdin(t *testing.T) {
	got, err := Parse("", "-", nil, strings.NewReader(`{"n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if got["n"].(float64) != 1 {
		t.Fatalf("stdin json not parsed: %v", got)
	}
}

func TestParseArgs(t *testing.T) {
	got, err := Parse("", "", []string{
		`query=MCP`, `limit=10`, `enabled=true`, `tags=["go","mcp"]`, `zip=01234`, `str="true"`,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got["query"] != "MCP" {
		t.Errorf("query = %v, want string MCP", got["query"])
	}
	if got["limit"].(float64) != 10 {
		t.Errorf("limit = %v, want number 10", got["limit"])
	}
	if got["enabled"] != true {
		t.Errorf("enabled = %v, want bool true", got["enabled"])
	}
	if _, ok := got["tags"].([]any); !ok {
		t.Errorf("tags = %T, want array", got["tags"])
	}
	if got["zip"] != "01234" {
		t.Errorf("zip = %v, want string 01234 (leading zero is not valid JSON)", got["zip"])
	}
	if got["str"] != "true" {
		t.Errorf(`str = %v, want string "true"`, got["str"])
	}
}

func TestParseArgErrors(t *testing.T) {
	if _, err := Parse("", "", []string{"noequals"}, nil); err == nil {
		t.Error("missing = should error")
	}
	if _, err := Parse("", "", []string{"=v"}, nil); err == nil {
		t.Error("empty key should error")
	}
	if _, err := Parse("", "", []string{"a=1", "a=2"}, nil); err == nil {
		t.Error("duplicate key should error")
	}
}

func TestParseNoneIsNil(t *testing.T) {
	got, err := Parse("", "", nil, nil)
	if err != nil || got != nil {
		t.Fatalf("no args should be (nil, nil), got %v %v", got, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/arguments/ -v`
Expected: FAIL — `undefined: Parse`.

- [ ] **Step 3: Write the implementation**

Create `internal/arguments/parse.go`:
```go
// Package arguments parses tool-call arguments from the CLI's three
// mutually-exclusive input modes into a JSON object.
package arguments

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"

	"mcpctl/internal/apperror"
)

// Parse builds the arguments object. Exactly one of jsonStr, jsonFile, or
// argKVs may be provided; none yields (nil, nil). stdin is used when jsonFile
// is "-".
func Parse(jsonStr, jsonFile string, argKVs []string, stdin io.Reader) (map[string]any, error) {
	modes := 0
	if jsonStr != "" {
		modes++
	}
	if jsonFile != "" {
		modes++
	}
	if len(argKVs) > 0 {
		modes++
	}
	if modes > 1 {
		return nil, apperror.Usage("use only one of --json, --json-file, or --arg")
	}

	switch {
	case jsonStr != "":
		return decodeObject([]byte(jsonStr))
	case jsonFile != "":
		data, err := readFileOrStdin(jsonFile, stdin)
		if err != nil {
			return nil, err
		}
		return decodeObject(data)
	case len(argKVs) > 0:
		return parseArgs(argKVs)
	default:
		return nil, nil
	}
}

func readFileOrStdin(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, apperror.Wrap(apperror.KindInvalidArgs, err, "read arguments from stdin")
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInvalidArgs, err, "read arguments file %q", path)
	}
	return data, nil
}

// decodeObject requires the JSON to be a top-level object.
func decodeObject(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, apperror.Wrap(apperror.KindInvalidArgs, err, "parse JSON arguments")
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, apperror.New(apperror.KindInvalidArgs,
			"arguments must be a JSON object, got %T", v)
	}
	return obj, nil
}

// parseArgs parses repeated KEY=VALUE pairs. Each value is decoded as JSON if
// it is valid JSON, otherwise treated as a string. Numeric-looking values
// therefore decode as numbers (e.g. version=1.10 -> 1.1); quote as a JSON
// string ("1.10") or use --json to force text.
func parseArgs(kvs []string) (map[string]any, error) {
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			return nil, apperror.New(apperror.KindInvalidArgs, "invalid --arg %q: expected KEY=VALUE", kv)
		}
		key := kv[:eq]
		if key == "" {
			return nil, apperror.New(apperror.KindInvalidArgs, "invalid --arg %q: empty key", kv)
		}
		if _, dup := out[key]; dup {
			return nil, apperror.New(apperror.KindInvalidArgs, "duplicate --arg key %q", key)
		}
		raw := kv[eq+1:]
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			out[key] = parsed
		} else {
			out[key] = raw
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/arguments/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/arguments/
git commit -m "feat(arguments): parse --json/--json-file/--arg tool arguments"
```

---

### Task 3: Server selection + ephemeral-stdio grammar (`internal/cli/target.go`)

**Files:**
- Create: `internal/cli/target.go`
- Test: `internal/cli/target_test.go`

**Interfaces:**
- Consumes: `config.LoadResolved`/`config.ServerConfig`, `client.StdioSpec`, `apperror`.
- Produces:
  - `type ServerFlags struct { Server, URL string; Stdio bool }`
  - `func resolveTarget(sf ServerFlags, toolSide, afterDash []string, hasDash bool, configPath string) (spec client.StdioSpec, toolArgs []string, err error)` — validates mutual exclusion and returns the stdio spec plus the tool-side positionals (with the server argv stripped for `--stdio`).

- [ ] **Step 1: Write the failing test**

Create `internal/cli/target_test.go`:
```go
package cli

import "testing"

func TestResolveEphemeralStdio(t *testing.T) {
	spec, toolArgs, err := resolveTarget(
		ServerFlags{Stdio: true},
		[]string{"echo"},                 // tool-side (before --)
		[]string{"npx", "-y", "srv"},     // after --
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestResolve -v`
Expected: FAIL — `undefined: resolveTarget` / `ServerFlags`.

- [ ] **Step 3: Write the implementation**

Create `internal/cli/target.go`:
```go
package cli

import (
	"mcpctl/internal/apperror"
	"mcpctl/internal/client"
	"mcpctl/internal/config"
)

// ServerFlags holds the mutually-exclusive server selectors bound on a tools command.
type ServerFlags struct {
	Server string
	URL    string
	Stdio  bool
}

// resolveTarget validates the server selectors and returns a stdio spec plus
// the tool-side positional args. toolSide is the positional args before `--`;
// afterDash is the args after `--` (the ephemeral server command); hasDash
// reports whether a `--` was present.
func resolveTarget(sf ServerFlags, toolSide, afterDash []string, hasDash bool, configPath string) (client.StdioSpec, []string, error) {
	selected := 0
	if sf.Server != "" {
		selected++
	}
	if sf.URL != "" {
		selected++
	}
	if sf.Stdio {
		selected++
	}
	if selected != 1 {
		return client.StdioSpec{}, nil, apperror.Usage(
			"exactly one of --server, --stdio, or --url is required")
	}

	switch {
	case sf.URL != "":
		return client.StdioSpec{}, nil, apperror.New(apperror.KindConnection,
			"streamable-http (--url) is not supported yet; it arrives in a later version")

	case sf.Stdio:
		if !hasDash || len(afterDash) == 0 {
			return client.StdioSpec{}, nil, apperror.Usage(
				"--stdio requires a server command after `--`, e.g. --stdio -- npx -y server")
		}
		spec := client.StdioSpec{Command: afterDash[0], Args: afterDash[1:]}
		return spec, toolSide, nil

	default: // --server
		spec, err := specFromConfig(sf.Server, configPath)
		if err != nil {
			return client.StdioSpec{}, nil, err
		}
		return spec, toolSide, nil
	}
}

// specFromConfig loads the named server from config and builds a stdio spec.
func specFromConfig(name, configPath string) (client.StdioSpec, error) {
	cfg, err := config.LoadResolved(configPath)
	if err != nil {
		return client.StdioSpec{}, err
	}
	sc, ok := cfg.Servers[name]
	if !ok {
		return client.StdioSpec{}, apperror.Config("no server named %q in configuration", name)
	}
	if sc.Transport != config.TransportStdio {
		return client.StdioSpec{}, apperror.New(apperror.KindConnection,
			"server %q uses transport %q; only stdio is supported yet", name, sc.Transport)
	}
	return client.StdioSpec{
		Command: sc.Command,
		Args:    sc.Args,
		CWD:     sc.CWD,
		Env:     sc.Env,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestResolve -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/target.go internal/cli/target_test.go
git commit -m "feat(cli): resolve named/ephemeral server selection into a stdio spec"
```

---

### Task 4: Command context (signals + timeout) + output rendering (`internal/output`)

**Files:**
- Create: `internal/cli/context.go`
- Create: `internal/output/output.go`, `internal/output/human.go`, `internal/output/json.go`
- Test: `internal/output/output_test.go`

**Interfaces:**
- Consumes: `client.ToolInfo`/`ToolResult`, `apperror`.
- Produces:
  - `func commandContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc)` — derives a signal-aware, timeout-bounded context.
  - `type output.Format string` + consts + `func ParseFormat(string) (Format, error)`.
  - `func output.ToolList(w io.Writer, f Format, server string, tools []client.ToolInfo) error`
  - `func output.ToolDescribe(w io.Writer, f Format, tool client.ToolInfo) error`
  - `func output.ToolResult(w io.Writer, f Format, r client.ToolResult) error`

- [ ] **Step 1: Write the command context helper**

Create `internal/cli/context.go`:
```go
package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// commandContext derives a context that is canceled on SIGINT/SIGTERM and that
// times out after the given duration. The returned CancelFunc must be called
// to release resources (and stop the signal handler).
func commandContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	sigCtx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(sigCtx, timeout)
	return ctx, func() {
		cancel()
		stop()
	}
}
```

- [ ] **Step 2: Write the failing output tests**

Create `internal/output/output_test.go`:
```go
package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"mcpctl/internal/client"
)

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"human", "json"} {
		if _, err := ParseFormat(ok); err != nil {
			t.Errorf("ParseFormat(%q) errored: %v", ok, err)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("yaml should be unsupported in Phase 2")
	}
	if _, err := ParseFormat("bogus"); err == nil {
		t.Error("bogus format should error")
	}
}

func TestToolListJSON(t *testing.T) {
	var buf bytes.Buffer
	err := ToolList(&buf, FormatJSON, "local", []client.ToolInfo{{Name: "echo", Description: "d"}})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Server string             `json:"server"`
		Tools  []client.ToolInfo  `json:"tools"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if got.Server != "local" || len(got.Tools) != 1 || got.Tools[0].Name != "echo" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestToolListHuman(t *testing.T) {
	var buf bytes.Buffer
	if err := ToolList(&buf, FormatHuman, "local", []client.ToolInfo{{Name: "echo", Description: "say hi"}}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "echo") || !strings.Contains(out, "say hi") {
		t.Fatalf("human list missing content:\n%s", out)
	}
}

func TestToolResultHumanText(t *testing.T) {
	var buf bytes.Buffer
	err := ToolResult(&buf, FormatHuman, client.ToolResult{
		Content: []client.ContentBlock{{Kind: client.KindText, Text: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("expected text content, got:\n%s", buf.String())
	}
}

func TestToolResultJSONFaithful(t *testing.T) {
	var buf bytes.Buffer
	err := ToolResult(&buf, FormatJSON, client.ToolResult{
		IsError: true,
		Content: []client.ContentBlock{{Kind: client.KindText, Text: "boom"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("invalid JSON: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"isError":true`) {
		t.Fatalf("isError not preserved: %s", buf.String())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/output/ -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 4: Write the format dispatcher**

Create `internal/output/output.go`:
```go
// Package output renders command results in the supported formats. It never
// imports the MCP SDK; it operates on the SDK-free client boundary types.
package output

import (
	"io"

	"mcpctl/internal/apperror"
	"mcpctl/internal/client"
)

type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
	FormatJSONL Format = "jsonl"
	FormatYAML  Format = "yaml"
)

// ParseFormat validates a format name. jsonl and yaml are recognized but not
// yet implemented (Phase 4).
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatHuman, FormatJSON:
		return Format(s), nil
	case FormatJSONL, FormatYAML:
		return "", apperror.Usage("output format %q is not supported yet (arrives in a later version)", s)
	default:
		return "", apperror.Usage("unknown output format %q (want human or json)", s)
	}
}

func ToolList(w io.Writer, f Format, server string, tools []client.ToolInfo) error {
	switch f {
	case FormatJSON:
		return toolListJSON(w, server, tools)
	default:
		return toolListHuman(w, tools)
	}
}

func ToolDescribe(w io.Writer, f Format, tool client.ToolInfo) error {
	switch f {
	case FormatJSON:
		return writeJSON(w, tool)
	default:
		return toolDescribeHuman(w, tool)
	}
}

func ToolResult(w io.Writer, f Format, r client.ToolResult) error {
	switch f {
	case FormatJSON:
		return writeJSON(w, r)
	default:
		return toolResultHuman(w, r)
	}
}
```

- [ ] **Step 5: Write the JSON renderers**

Create `internal/output/json.go`:
```go
package output

import (
	"encoding/json"
	"io"

	"mcpctl/internal/client"
)

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func toolListJSON(w io.Writer, server string, tools []client.ToolInfo) error {
	if tools == nil {
		tools = []client.ToolInfo{}
	}
	return writeJSON(w, struct {
		Server string            `json:"server"`
		Tools  []client.ToolInfo `json:"tools"`
	}{Server: server, Tools: tools})
}
```

- [ ] **Step 6: Write the human renderers**

Create `internal/output/human.go`:
```go
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"mcpctl/internal/client"
)

const descWidth = 60

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func toolListHuman(w io.Writer, tools []client.ToolInfo) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDESCRIPTION")
	for _, t := range tools {
		fmt.Fprintf(tw, "%s\t%s\n", t.Name, truncate(t.Description, descWidth))
	}
	return tw.Flush()
}

func toolDescribeHuman(w io.Writer, t client.ToolInfo) error {
	fmt.Fprintf(w, "Name:        %s\n", t.Name)
	if t.Title != "" {
		fmt.Fprintf(w, "Title:       %s\n", t.Title)
	}
	fmt.Fprintf(w, "Description: %s\n", t.Description)
	if t.InputSchema != nil {
		fmt.Fprintf(w, "Input schema:\n%s\n", indentJSON(t.InputSchema))
	}
	if t.OutputSchema != nil {
		fmt.Fprintf(w, "Output schema:\n%s\n", indentJSON(t.OutputSchema))
	}
	if t.Annotations != nil {
		fmt.Fprintf(w, "Annotations:\n%s\n", indentJSON(t.Annotations))
	}
	return nil
}

func toolResultHuman(w io.Writer, r client.ToolResult) error {
	if r.IsError {
		fmt.Fprintln(w, "TOOL ERROR:")
	}
	for _, c := range r.Content {
		switch c.Kind {
		case client.KindText:
			fmt.Fprintln(w, c.Text)
		case client.KindImage, client.KindAudio:
			fmt.Fprintf(w, "[%s content, %s, %d bytes]\n", c.Kind, c.MIMEType, len(c.Data))
		case client.KindResource:
			fmt.Fprintf(w, "[resource %s %s]\n", c.URI, c.MIMEType)
		default:
			fmt.Fprintf(w, "[%s content]\n", c.Kind)
		}
	}
	if r.Structured != nil {
		fmt.Fprintf(w, "Structured:\n%s\n", indentJSON(r.Structured))
	}
	return nil
}

func indentJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
```

- [ ] **Step 7: Run tests + vet + gofmt**

Run: `go test ./internal/output/ -v && go vet ./... && gofmt -l internal/output/ internal/cli/`
Expected: PASS; clean.

- [ ] **Step 8: Commit**

```bash
git add internal/cli/context.go internal/output/
git commit -m "feat(output): human/json rendering; add signal+timeout command context"
```

---

### Task 5: `tools list` command

**Files:**
- Create: `internal/cli/tools.go`
- Modify: `internal/cli/root.go` (attach the `tools` command)
- Test: `internal/cli/e2e_test.go`

**Interfaces:**
- Consumes: `resolveTarget`, `commandContext`, `client.DialStdio`, `output.*`, `GlobalFlags`.
- Produces: `func newToolsCmd(g *GlobalFlags) *cobra.Command` with a `list` subcommand; `internal/cli/e2e_test.go` building the binary.

- [ ] **Step 1: Write the tools command tree + list**

Create `internal/cli/tools.go`:
```go
package cli

import (
	"context"

	"github.com/spf13/cobra"

	"mcpctl/internal/apperror"
	"mcpctl/internal/client"
	"mcpctl/internal/output"
)

const defaultMaxPages = 1000

func newToolsCmd(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List, describe, and call tools on an MCP server",
	}
	cmd.AddCommand(newToolsListCmd(g))
	return cmd
}

// addServerFlags binds the shared server-selection flags to a tools subcommand.
func addServerFlags(cmd *cobra.Command, sf *ServerFlags) {
	cmd.Flags().StringVar(&sf.Server, "server", "", "named server from configuration")
	cmd.Flags().StringVar(&sf.URL, "url", "", "ephemeral Streamable HTTP URL (not yet supported)")
	cmd.Flags().BoolVar(&sf.Stdio, "stdio", false, "ephemeral stdio server (command follows `--`)")
}

// dial resolves the target from flags/args and connects using ctx. A single
// command-scoped, signal-aware, timeout-bounded context is used for the whole
// command (spec §14 permits this when separating connect/op is impractical):
// the SDK session's lifetime is tied to this context, so it must outlive every
// call made on the returned client.
func dial(ctx context.Context, cmd *cobra.Command, g *GlobalFlags, sf ServerFlags, args []string) (client.Client, []string, error) {
	dash := cmd.ArgsLenAtDash()
	var toolSide, afterDash []string
	hasDash := dash >= 0
	if hasDash {
		toolSide, afterDash = args[:dash], args[dash:]
	} else {
		toolSide = args
	}
	spec, toolArgs, err := resolveTarget(sf, toolSide, afterDash, hasDash, g.Config)
	if err != nil {
		return nil, nil, err
	}
	c, err := client.DialStdio(ctx, spec)
	if err != nil {
		return nil, nil, err
	}
	if !c.ServerInfo().SupportsTools {
		c.Close()
		return nil, nil, apperror.New(apperror.KindProtocol, "server does not support tools")
	}
	return c, toolArgs, nil
}

func newToolsListCmd(g *GlobalFlags) *cobra.Command {
	var sf ServerFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			ctx, cancel := commandContext(cmd.Context(), g.Timeout)
			defer cancel()
			c, _, err := dial(ctx, cmd, g, sf, args)
			if err != nil {
				return err
			}
			defer c.Close()

			tools, err := c.ListAllTools(ctx, defaultMaxPages)
			if err != nil {
				return err
			}
			serverName := sf.Server
			if serverName == "" {
				serverName = "(ephemeral)"
			}
			return output.ToolList(cmd.OutOrStdout(), f, serverName, tools)
		},
	}
	addServerFlags(cmd, &sf)
	return cmd
}
```
Defer order matters: register `defer cancel()` first, then `defer c.Close()`, so Close runs before the context is canceled.

- [ ] **Step 2: Attach `tools` to the root**

In `internal/cli/root.go`, inside `NewRootCmd`, next to `root.AddCommand(newVersionCmd())`, add:
```go
	root.AddCommand(newToolsCmd(g))
```
(`g` is the `*GlobalFlags` in scope in `NewRootCmd`.)

- [ ] **Step 3: Write the e2e test**

Create `internal/cli/e2e_test.go`:
```go
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
```

- [ ] **Step 4: Run tests + the full gate**

Run:
```bash
go test ./internal/cli/ -run 'TestResolve|TestE2EToolsList' -v
go test ./... && go vet ./... && gofmt -l internal/cli/
```
Expected: PASS; `tools list` works end-to-end over stdio; JSON stdout is clean.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/tools.go internal/cli/root.go internal/cli/e2e_test.go
git commit -m "feat(cli): tools list command over stdio"
```

---

### Task 6: `tools describe` command

**Files:**
- Modify: `internal/cli/tools.go` (add `describe`)
- Test: `internal/cli/e2e_test.go` (add describe cases)

**Interfaces:**
- Consumes: `dial`, `client.ListAllTools`, `output.ToolDescribe`.
- Produces: `newToolsDescribeCmd(g)`, attached in `newToolsCmd`.

- [ ] **Step 1: Add the describe command**

In `internal/cli/tools.go`, add to `newToolsCmd`'s body (before `return cmd`): `cmd.AddCommand(newToolsDescribeCmd(g))`. Then add:
```go
func newToolsDescribeCmd(g *GlobalFlags) *cobra.Command {
	var sf ServerFlags
	cmd := &cobra.Command{
		Use:   "describe TOOL",
		Short: "Show a tool's description and schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			ctx, cancel := commandContext(cmd.Context(), g.Timeout)
			defer cancel()
			c, toolArgs, err := dial(ctx, cmd, g, sf, args)
			if err != nil {
				return err
			}
			defer c.Close()
			if len(toolArgs) != 1 {
				return apperror.Usage("describe requires exactly one TOOL name")
			}
			name := toolArgs[0]

			tools, err := c.ListAllTools(ctx, defaultMaxPages)
			if err != nil {
				return err
			}
			for _, t := range tools {
				if t.Name == name {
					return output.ToolDescribe(cmd.OutOrStdout(), f, t)
				}
			}
			return apperror.New(apperror.KindToolNotFound, "tool %q not found on this server", name)
		},
	}
	addServerFlags(cmd, &sf)
	return cmd
}
```

- [ ] **Step 2: Add e2e tests**

In `internal/cli/e2e_test.go`, add:
```go
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
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/cli/ -run TestE2EToolsDescribe -v && go test ./... && gofmt -l internal/cli/`
Expected: PASS; not-found exits 7.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/tools.go internal/cli/e2e_test.go
git commit -m "feat(cli): tools describe command with tool-not-found exit 7"
```

---

### Task 7: `tools call` command

**Files:**
- Modify: `internal/cli/tools.go` (add `call`)
- Test: `internal/cli/e2e_test.go` (add call cases)

**Interfaces:**
- Consumes: `dial`, `arguments.Parse`, `client.CallTool`, `output.ToolResult`.
- Produces: `newToolsCallCmd(g)`, attached in `newToolsCmd`.

- [ ] **Step 1: Add the call command**

In `internal/cli/tools.go`, add `cmd.AddCommand(newToolsCallCmd(g))` in `newToolsCmd`. Add the import `"mcpctl/internal/arguments"`. Then:
```go
func newToolsCallCmd(g *GlobalFlags) *cobra.Command {
	var sf ServerFlags
	var jsonStr, jsonFile string
	var argKVs []string
	cmd := &cobra.Command{
		Use:   "call TOOL",
		Short: "Call a tool with JSON arguments",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			toolArguments, err := arguments.Parse(jsonStr, jsonFile, argKVs, cmd.InOrStdin())
			if err != nil {
				return err
			}
			ctx, cancel := commandContext(cmd.Context(), g.Timeout)
			defer cancel()
			c, toolArgs, err := dial(ctx, cmd, g, sf, args)
			if err != nil {
				return err
			}
			defer c.Close()
			if len(toolArgs) != 1 {
				return apperror.Usage("call requires exactly one TOOL name")
			}
			name := toolArgs[0]

			// Confirm the tool exists before calling (spec §11): not-found -> exit 7.
			tools, err := c.ListAllTools(ctx, defaultMaxPages)
			if err != nil {
				return err
			}
			found := false
			for _, t := range tools {
				if t.Name == name {
					found = true
					break
				}
			}
			if !found {
				return apperror.New(apperror.KindToolNotFound, "tool %q not found on this server", name)
			}

			result, err := c.CallTool(ctx, name, toolArguments)
			if err != nil {
				return err
			}
			if rerr := output.ToolResult(cmd.OutOrStdout(), f, result); rerr != nil {
				return rerr
			}
			if result.IsError {
				return apperror.New(apperror.KindToolError, "tool %q reported an error", name)
			}
			return nil
		},
	}
	addServerFlags(cmd, &sf)
	cmd.Flags().StringVar(&jsonStr, "json", "", "arguments as a JSON object")
	cmd.Flags().StringVar(&jsonFile, "json-file", "", "arguments from a JSON file (`-` for stdin)")
	cmd.Flags().StringArrayVar(&argKVs, "arg", nil, "argument as KEY=VALUE (repeatable)")
	return cmd
}
```
Stdin for `--json-file -` comes from `cmd.InOrStdin()`, so no `os` import is needed.

- [ ] **Step 2: Add e2e tests**

In `internal/cli/e2e_test.go`, add:
```go
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
```

- [ ] **Step 3: Run the full gate**

Run:
```bash
go test ./internal/cli/ -run TestE2EToolsCall -v
go test ./... && go test -race ./internal/client/ ./internal/cli/ && go vet ./... && gofmt -l ./internal/
```
Expected: PASS — echo returns `hello`; `--arg` sum is `5`; `boom` exits 9 with rendered content; unknown tool exits 7.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/tools.go internal/cli/e2e_test.go
git commit -m "feat(cli): tools call command with arg parsing, isError exit 9, not-found exit 7"
```

---

## Phase 2B Acceptance

Phase 2B is complete when:

- `make check` passes (build, test, `-race`, vet, staticcheck) under Go 1.25.
- `mcpctl tools list --stdio -- <server>` initializes, lists all tool pages, and prints them; `--output json` emits clean JSON to stdout with server stderr on stderr (spec §20.1, §20.4).
- `mcpctl tools describe <tool> --stdio -- <server>` prints name/description/schema; an unknown tool exits `7`.
- `mcpctl tools call <tool> --json '{...}' --stdio -- <server>` sends the object, renders the result, and exits `0`; `--arg K=V` builds the object; a tool returning `isError` renders its content and exits `9`; calling an unknown tool exits `7` without sending `tools/call`.
- `--url` and `--output yaml`/`jsonl` return clear unsupported errors; the three server selectors are mutually exclusive.
- The SDK is imported only by `internal/client` (grep confirms no `go-sdk` import in `internal/cli`, `internal/output`, `internal/arguments`).

---

## After 2B: finish Phase 2

- Whole-branch review of the full `v2-stdio-mvp` branch (2A + 2B) on the strongest model.
- Update the README's quick-start + exit-code table if in scope, or defer to Phase 5.
- Merge `v2-stdio-mvp` to `main` (Phase 2 = 2A + 2B together).
- Phase 3 (Streamable HTTP) begins with its own transport spike confirming the `*http.Client` injection point (§9).
