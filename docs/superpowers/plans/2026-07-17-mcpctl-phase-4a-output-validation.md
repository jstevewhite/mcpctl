# mcpctl Phase 4A — Output Formats + Validation + Sanitization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the output-format matrix (`jsonl`, `yaml`, and pretty `json`), validate tool-call arguments against the tool's JSON Schema locally before calling (unless `--no-validate`), and sanitize untrusted server text before printing it to a terminal.

**Architecture:** `internal/output` gains YAML (via `sigs.k8s.io/yaml`, which marshals through the boundary types' JSON tags) and JSON-Lines renderers, and pretty-prints `json`; human output routes untrusted strings through a control-character sanitizer. `internal/arguments` gains a `Validate` that compiles the tool's input schema with `github.com/santhosh-tekuri/jsonschema/v6` and validates the parsed args; a schema that will not compile degrades to a warning (the server still validates), a schema violation is a local exit-8 failure that never sends `tools/call`. All of this stays SDK-free.

**Tech Stack:** Go 1.25, `sigs.k8s.io/yaml`, `github.com/santhosh-tekuri/jsonschema/v6`, standard library.

## Global Constraints

From `mcpctl-spec.md` §2.5, §2.6, §11, §12, §15.

- **SDK confinement:** `internal/output` and `internal/arguments` do not import the MCP SDK.
- **Output formats (§4.1, §12):** `human`, `json`, `jsonl`, `yaml`. Collections (`tools list`) emit one object per line in `jsonl`. `json` is one valid document; `yaml` is one valid document; `jsonl` is one JSON object per line. Machine-readable stdout carries no logs/warnings/color.
- **YAML (§2.6):** `sigs.k8s.io/yaml` — marshals through JSON struct tags, so YAML field names match JSON output exactly.
- **JSON-Schema validation (§2.5, §11):** validate arguments against the tool's `inputSchema` unless `--no-validate`. On validation failure: do NOT send `tools/call`; report the failures (JSON path, expected, actual — without leaking secrets unnecessarily); exit **8**. If the schema cannot be compiled locally: in human mode print a warning to **stderr** and continue (let the server validate); in machine-readable modes do not contaminate stdout; log details at debug. An invalid/unsupported server schema must never make a tool permanently unusable.
- **Terminal sanitization (§15.9):** treat MCP server output (tool descriptions, schemas, results) as untrusted. Strip or escape unsafe control characters (ANSI escapes, other C0/C1 controls) before rendering in human mode; preserve normal whitespace (newline, tab). Do not alter machine-readable (`json`/`jsonl`/`yaml`) output — that is structured data, not terminal-rendered.
- **Exit codes:** invalid arguments → **8**; usage → 2.
- **Confirm external library APIs** for `sigs.k8s.io/yaml` and `jsonschema/v6` via `go doc` before writing against them; adapt the plan's calls if the signatures differ, and report any adaptation.

---

## File Structure

- Modify: `internal/output/output.go` (ParseFormat accepts all four; dispatch jsonl/yaml), `internal/output/json.go` (indented json + compact jsonl helpers), `internal/output/human.go` (sanitize untrusted text).
- Create: `internal/output/yaml.go`, `internal/output/jsonl.go`, `internal/output/sanitize.go` (+ tests).
- Create: `internal/arguments/validate.go` (+ test).
- Modify: `internal/cli/tools.go` (wire validation into `tools call`).
- Modify: `internal/cli/e2e_test.go` (validation e2e).

---

### Task 1: JSONL + YAML output; pretty JSON

**Files:**
- Modify: `internal/output/output.go`, `internal/output/json.go`
- Create: `internal/output/yaml.go`, `internal/output/jsonl.go`
- Test: `internal/output/output_test.go` (extend)

**Interfaces:**
- Consumes: `sigs.k8s.io/yaml`, `encoding/json`, the boundary types.
- Produces: `ParseFormat` accepts `human|json|jsonl|yaml`; `ToolList`/`ToolDescribe`/`ToolResult` render all four; helpers `writeJSONIndent`, `writeJSONCompact`, `writeYAML`, `writeJSONLines`.

- [ ] **Step 1: Add the YAML dependency**

Run:
```bash
go get sigs.k8s.io/yaml@latest
```
Confirm its `Marshal` signature: `go doc sigs.k8s.io/yaml.Marshal` (expected `func Marshal(o any) ([]byte, error)`). Adapt if different.

- [ ] **Step 2: Write the failing tests**

In `internal/output/output_test.go`, extend `TestParseFormat` to accept `jsonl` and `yaml`, and add:
```go
func TestToolListYAML(t *testing.T) {
	var buf bytes.Buffer
	if err := ToolList(&buf, FormatYAML, "local", []client.ToolInfo{{Name: "echo", Description: "d"}}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "server: local") || !strings.Contains(out, "name: echo") {
		t.Fatalf("unexpected yaml:\n%s", out)
	}
}

func TestToolListJSONL(t *testing.T) {
	var buf bytes.Buffer
	if err := ToolList(&buf, FormatJSONL, "local", []client.ToolInfo{{Name: "a"}, {Name: "b"}}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("jsonl should be one tool per line, got %d lines:\n%s", len(lines), buf.String())
	}
	for _, ln := range lines {
		if !json.Valid([]byte(ln)) {
			t.Fatalf("jsonl line is not valid JSON: %q", ln)
		}
	}
}

func TestToolResultYAML(t *testing.T) {
	var buf bytes.Buffer
	if err := ToolResult(&buf, FormatYAML, client.ToolResult{IsError: true, Content: []client.ContentBlock{{Kind: client.KindText, Text: "boom"}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "isError: true") {
		t.Fatalf("yaml missing isError:\n%s", buf.String())
	}
}
```
Update the existing `TestToolResultJSONFaithful` assertion from `"isError":true` to `"isError": true` (json is now indented — a space follows the colon).

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/output/ -v`
Expected: FAIL — yaml/jsonl not rendered; ParseFormat rejects them.

- [ ] **Step 4: Update ParseFormat + the format dispatch (output.go)**

Replace `ParseFormat` and the three render dispatchers in `internal/output/output.go`:
```go
// ParseFormat validates a format name.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatHuman, FormatJSON, FormatJSONL, FormatYAML:
		return Format(s), nil
	default:
		return "", apperror.Usage("unknown output format %q (want human, json, jsonl, or yaml)", s)
	}
}

func ToolList(w io.Writer, f Format, server string, tools []client.ToolInfo) error {
	switch f {
	case FormatJSON:
		return writeJSONIndent(w, toolListDoc(server, tools))
	case FormatYAML:
		return writeYAML(w, toolListDoc(server, tools))
	case FormatJSONL:
		items := make([]any, len(tools))
		for i := range tools {
			items[i] = tools[i]
		}
		return writeJSONLines(w, items)
	default:
		return toolListHuman(w, tools)
	}
}

func ToolDescribe(w io.Writer, f Format, tool client.ToolInfo) error {
	switch f {
	case FormatJSON:
		return writeJSONIndent(w, tool)
	case FormatYAML:
		return writeYAML(w, tool)
	case FormatJSONL:
		return writeJSONCompact(w, tool)
	default:
		return toolDescribeHuman(w, tool)
	}
}

func ToolResult(w io.Writer, f Format, r client.ToolResult) error {
	switch f {
	case FormatJSON:
		return writeJSONIndent(w, r)
	case FormatYAML:
		return writeYAML(w, r)
	case FormatJSONL:
		return writeJSONCompact(w, r)
	default:
		return toolResultHuman(w, r)
	}
}
```

- [ ] **Step 5: JSON helpers (json.go)**

Replace `internal/output/json.go` with indented + compact + jsonl helpers, and a `toolListDoc` constructor:
```go
package output

import (
	"encoding/json"
	"io"

	"mcpctl/internal/client"
)

func toolListDoc(server string, tools []client.ToolInfo) any {
	if tools == nil {
		tools = []client.ToolInfo{}
	}
	return struct {
		Server string            `json:"server"`
		Tools  []client.ToolInfo `json:"tools"`
	}{Server: server, Tools: tools}
}

func writeJSONIndent(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeJSONCompact(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v) // Encoder writes one line + newline
}

func writeJSONLines(w io.Writer, items []any) error {
	enc := json.NewEncoder(w)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 6: YAML helper (yaml.go)**

Create `internal/output/yaml.go`:
```go
package output

import (
	"io"

	"sigs.k8s.io/yaml"
)

// writeYAML marshals v to YAML via its JSON tags (so YAML keys match JSON).
func writeYAML(w io.Writer, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
```
(If you prefer, an empty `internal/output/jsonl.go` is unnecessary — the JSONL helpers live in `json.go`. Do not create an empty file.)

- [ ] **Step 7: Run tests + vet + gofmt**

Run: `go test ./internal/output/ -v && go test ./... && go vet ./... && gofmt -l internal/output/`
Expected: PASS; json is now indented, jsonl is one-object-per-line, yaml matches json field names.

- [ ] **Step 8: Commit**

```bash
git add internal/output/ go.mod go.sum
git commit -m "feat(output): jsonl and yaml rendering; pretty json"
```

---

### Task 2: Terminal sanitization of untrusted server text

**Files:**
- Create: `internal/output/sanitize.go`
- Modify: `internal/output/human.go`
- Test: `internal/output/sanitize_test.go`

**Interfaces:**
- Produces: `func sanitize(s string) string` — removes unsafe control characters (keeps `\n`, `\t`), used by the human renderers.

- [ ] **Step 1: Write the failing test**

Create `internal/output/sanitize_test.go`:
```go
package output

import (
	"strings"
	"testing"
)

func TestSanitizeStripsControlChars(t *testing.T) {
	// An ANSI escape sequence and a bell should be removed; text and newline kept.
	in := "hello\x1b[31mRED\x1b[0m\x07 world\nline2\ttab"
	got := sanitize(in)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\x07") {
		t.Fatalf("control chars not stripped: %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("visible text lost: %q", got)
	}
	if !strings.Contains(got, "\n") || !strings.Contains(got, "\t") {
		t.Fatalf("newline/tab should be preserved: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -run TestSanitize -v`
Expected: FAIL — `undefined: sanitize`.

- [ ] **Step 3: Write sanitize.go**

Create `internal/output/sanitize.go`:
```go
package output

import "strings"

// sanitize removes control characters that could corrupt or spoof a terminal
// (ANSI escapes, other C0/C1 controls) from untrusted server text, keeping
// ordinary whitespace (newline, tab). It is applied only to human-mode output;
// machine-readable formats carry structured data verbatim.
func sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f: // C0 controls + DEL
			// drop
		case r >= 0x80 && r <= 0x9f: // C1 controls
			// drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Apply sanitize in the human renderers (human.go)**

In `internal/output/human.go`, route untrusted server-provided strings through `sanitize`: the tool description in `toolListHuman` (before `truncate`) and `toolDescribeHuman`; the `Name`/`Description`/`Title` fields; and text content in `toolResultHuman` (the `c.Text` printed for `KindText`). Do NOT sanitize `mcpctl`-generated labels. Concretely, wrap each server-sourced string, e.g.:
```go
// in toolListHuman:
fmt.Fprintf(tw, "%s\t%s\n", sanitize(t.Name), truncate(sanitize(t.Description), descWidth))
// in toolResultHuman, KindText case:
fmt.Fprintln(w, sanitize(c.Text))
```
Apply the same to `toolDescribeHuman`'s `Name`/`Title`/`Description` prints. (Schema/annotations rendered via `indentJSON` are structured JSON, already escaped by the encoder — leave them.)

- [ ] **Step 5: Run tests + vet + gofmt**

Run: `go test ./internal/output/ -v && go vet ./... && gofmt -l internal/output/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/output/sanitize.go internal/output/sanitize_test.go internal/output/human.go
git commit -m "feat(output): sanitize control characters in human-mode server text"
```

---

### Task 3: JSON-Schema argument validation (`internal/arguments/validate.go`)

**Files:**
- Create: `internal/arguments/validate.go`
- Test: `internal/arguments/validate_test.go`

**Interfaces:**
- Consumes: `github.com/santhosh-tekuri/jsonschema/v6`, `apperror`.
- Produces:
  - `type arguments.SchemaUnusableError struct { Err error }` (implements `error`).
  - `func arguments.Validate(schema any, args map[string]any) error` — returns nil if valid or `schema == nil`; a `*SchemaUnusableError` if the schema cannot be compiled; an `apperror` `KindInvalidArgs` error if the args violate the schema.

- [ ] **Step 1: Add the dependency and confirm the API**

Run:
```bash
go get github.com/santhosh-tekuri/jsonschema/v6@latest
go doc github.com/santhosh-tekuri/jsonschema/v6.Compiler
go doc github.com/santhosh-tekuri/jsonschema/v6.Schema.Validate
```
Confirm the v6 API: a `Compiler` with `AddResource(url string, doc any) error` and `Compile(url string) (*Schema, error)`, and `(*Schema).Validate(v any) error`. Adapt the code below to the real signatures if they differ, and note the adaptation in your report.

- [ ] **Step 2: Write the failing test**

Create `internal/arguments/validate_test.go`:
```go
package arguments

import (
	"errors"
	"testing"
)

var objSchema = map[string]any{
	"type":     "object",
	"required": []any{"path"},
	"properties": map[string]any{
		"path": map[string]any{"type": "string"},
	},
}

func TestValidateNilSchemaSkips(t *testing.T) {
	if err := Validate(nil, map[string]any{"x": 1}); err != nil {
		t.Fatalf("nil schema should skip validation, got %v", err)
	}
}

func TestValidateAccepts(t *testing.T) {
	if err := Validate(objSchema, map[string]any{"path": "/tmp/x"}); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
}

func TestValidateRejectsMissingRequired(t *testing.T) {
	err := Validate(objSchema, map[string]any{})
	if err == nil {
		t.Fatal("missing required 'path' should fail validation")
	}
	var unusable *SchemaUnusableError
	if errors.As(err, &unusable) {
		t.Fatalf("a schema violation must not be reported as unusable-schema: %v", err)
	}
}

func TestValidateRejectsWrongType(t *testing.T) {
	if err := Validate(objSchema, map[string]any{"path": 123}); err == nil {
		t.Fatal("path as a number should fail validation")
	}
}

func TestValidateUnusableSchema(t *testing.T) {
	// A schema the compiler cannot make sense of degrades to SchemaUnusableError.
	bad := map[string]any{"type": 12345} // "type" must be a string/array, not a number
	err := Validate(bad, map[string]any{"x": 1})
	var unusable *SchemaUnusableError
	if err != nil && !errors.As(err, &unusable) {
		t.Fatalf("an uncompilable schema should yield SchemaUnusableError, got %T: %v", err, err)
	}
	// (If the compiler happens to accept it, err is nil — also acceptable.)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/arguments/ -run TestValidate -v`
Expected: FAIL — `undefined: Validate` / `SchemaUnusableError`.

- [ ] **Step 4: Write validate.go**

Create `internal/arguments/validate.go` (adapt to the confirmed v6 API):
```go
package arguments

import (
	"github.com/santhosh-tekuri/jsonschema/v6"

	"mcpctl/internal/apperror"
)

// SchemaUnusableError signals that a tool's input schema could not be compiled
// locally. The caller should warn and let the server validate (spec §2.5) — an
// unusable schema must never make a tool permanently unusable.
type SchemaUnusableError struct{ Err error }

func (e *SchemaUnusableError) Error() string {
	return "tool input schema could not be compiled: " + e.Err.Error()
}
func (e *SchemaUnusableError) Unwrap() error { return e.Err }

// Validate checks args against the tool's JSON Schema. A nil schema (or a schema
// that will not compile) does not fail the call: nil → nil; uncompilable →
// *SchemaUnusableError. A genuine schema violation → apperror KindInvalidArgs.
func Validate(schema any, args map[string]any) error {
	if schema == nil {
		return nil
	}
	if args == nil {
		args = map[string]any{} // validate a no-args call as an empty object
	}
	c := jsonschema.NewCompiler()
	const url = "mem://tool-input-schema"
	if err := c.AddResource(url, schema); err != nil {
		return &SchemaUnusableError{Err: err}
	}
	sch, err := c.Compile(url)
	if err != nil {
		return &SchemaUnusableError{Err: err}
	}
	if err := sch.Validate(any(args)); err != nil {
		return apperror.Wrap(apperror.KindInvalidArgs, err, "arguments do not match the tool's input schema")
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/arguments/ -run TestValidate -v && go test ./... && go vet ./... && gofmt -l internal/arguments/`
Expected: PASS — valid accepted, missing-required/wrong-type rejected (not as unusable), uncompilable schema → SchemaUnusableError.

- [ ] **Step 6: Commit**

```bash
git add internal/arguments/validate.go internal/arguments/validate_test.go go.mod go.sum
git commit -m "feat(arguments): local JSON-Schema validation of tool arguments"
```

---

### Task 4: Wire validation into `tools call`

**Files:**
- Modify: `internal/cli/tools.go`
- Modify: `internal/cli/e2e_test.go`

**Interfaces:**
- Consumes: `arguments.Validate`/`SchemaUnusableError`, `g.NoValidate`, `findTool` (returns the `ToolInfo`), `output.Format`.

- [ ] **Step 1: Add validation to the call command**

In `internal/cli/tools.go`, the `call` RunE currently confirms the tool exists via `findTool`. Change it to capture the tool, and validate before calling. Replace the existence check + call block:
```go
			tool, ok := findTool(tools, name)
			if !ok {
				return apperror.New(apperror.KindToolNotFound, "tool %q not found on this server", name)
			}
			if !g.NoValidate {
				if verr := arguments.Validate(tool.InputSchema, toolArguments); verr != nil {
					var unusable *arguments.SchemaUnusableError
					if errors.As(verr, &unusable) {
						slog.Debug("skipping local argument validation", "tool", name, "reason", unusable.Err)
						if f == output.FormatHuman {
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not validate arguments locally for %q; the server will validate\n", name)
						}
					} else {
						return verr // KindInvalidArgs → exit 8, tools/call not sent
					}
				}
			}

			result, err := c.CallTool(ctx, name, toolArguments)
```
Add imports: `"errors"`, `"fmt"`, `"log/slog"`. (`arguments.Validate` normalizes a nil `toolArguments` to an empty object internally, so pass `toolArguments` directly.)

- [ ] **Step 2: Add e2e tests**

The stdio test server's `echo` tool has an input schema requiring `message` (string). Add to `internal/cli/e2e_test.go`:
```go
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
	// With --no-validate, the (empty) args are sent; the server accepts and echoes empty.
	_, _, code := run(t, mcpctl, "--no-validate", "tools", "call", "echo", "--json", "{}", "--stdio", "--", server)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 with --no-validate", code)
	}
}
```
NOTE: this assumes the stdio test server's `echo` tool advertises a `required: [message]` input schema (the SDK generates it from the handler's `echoArgs` struct). Confirm by running `tools describe echo` against the server; if `message` is not marked required by the SDK's schema generator, adjust the test to a tool/argument that is (or add a `required`-bearing tool to the test server) — note the choice in your report.

- [ ] **Step 3: Run the gate**

Run:
```bash
go test ./internal/cli/ -run 'TestE2EToolsCall' -v
go test ./... && go vet ./... && gofmt -l internal/cli/
```
Expected: PASS — missing required arg exits 8 without calling; `--no-validate` bypasses.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/tools.go internal/cli/e2e_test.go
git commit -m "feat(cli): validate tool arguments before calling; --no-validate bypass"
```

---

## Phase 4A Acceptance

- `make check` passes (build, test, `-race`, vet, staticcheck) under Go 1.25.
- `--output yaml` and `--output jsonl` work for `tools list`/`describe`/`call`; `json` is pretty; yaml field names match json; jsonl is one object per line for the collection.
- `tools call` with arguments violating the tool's input schema exits **8** and does not send `tools/call`; `--no-validate` bypasses; a tool whose schema can't be compiled still works (a warning on stderr in human mode, clean stdout in machine modes).
- Human-mode output strips control characters from untrusted server text; machine-readable output is unaltered.
- SDK not imported by `internal/output` or `internal/arguments`.

---

## Then: Phase 4B (server management)

`server list/show/add/remove`: `internal/config/save.go` (write TOML, 0600 perms), `internal/cli/server.go` (the four subcommands), secret redaction in `list`/`show` output (env-var names shown; values → `<env:VAR>` placeholder; literal secrets flagged). After 4B: whole-Phase-4 review + merge to main.
