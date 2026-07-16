# mcpctl Phase 1 — Project Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the `mcpctl` Go project skeleton — module, Cobra root command, version reporting, structured logging, a typed error/exit-code framework, TOML configuration loading, and CI — with no MCP SDK dependency yet.

**Architecture:** A thin Cobra command tree whose handlers return typed `*apperror.Error` values; `main.go` is the only place that classifies errors into process exit codes and calls `os.Exit`. Configuration is loaded from TOML into plain structs, validated, and never mutated on read. All diagnostics go to stderr; command results go to stdout. This phase establishes the cross-cutting foundations (errors, logging, config, version) that every later phase builds on.

**Tech Stack:** Go 1.24+, `github.com/spf13/cobra`, `github.com/pelletier/go-toml/v2`, standard library `log/slog`, GoReleaser + GitHub Actions for CI/release.

## Global Constraints

These apply to every task. Values are copied verbatim from `mcpctl-spec.md`.

- **Go version:** Go 1.24 or newer; Go modules.
- **Module path:** `mcpctl` — a bare local module path, since the project is private for now. Valid for local `go build`/`test` and GoReleaser. When a GitHub repo exists, migrate to `github.com/<owner>/mcpctl` via `go mod edit -module` plus a find/replace across imports.
- **No CGo:** prefer pure-Go dependencies; do not introduce CGo without a documented reason.
- **Output contract:** all diagnostics and logs go to **stderr**; command results go to **stdout**. Machine-readable stdout must never contain log prefixes, color, progress, or warnings.
- **`os.Exit` only in `main.go`.** Command/library packages return errors.
- **Exit codes (stable):** `0` success, `1` unspecified/internal, `2` usage, `3` config, `4` auth, `5` connection/transport, `6` protocol/init, `7` tool not found, `8` invalid arguments, `9` tool `isError`, `10` timeout, `130` interrupted.
- **Logging:** `log/slog`; levels `debug|info|warn|error`; default `warn`; output to stderr.
- **Error message style:** include context, avoid duplicated chains (`failed: failed to ...`). Prefer `connect to server "remote": HTTP 401 Unauthorized`.
- **Config default path:** honor `XDG_CONFIG_HOME` when set (`$XDG_CONFIG_HOME/mcpctl/config.toml`) before falling back to `os.UserConfigDir()`. Path resolved at runtime, never hard-coded per-OS.
- **Config validation:** `version` must equal `1`; unknown fields rejected.
- **Quality gates (must pass in CI):** `go test ./...`, `go test -race ./...`, `go vet ./...`, `staticcheck ./...`.
- **Version command output format:**
  ```
  mcpctl version <version>
  commit: <commit>
  built: <timestamp>
  go: <go version>
  ```
  `mcpctl --version` prints the concise one-line form `mcpctl <version>`.

---

## File Structure

Created in this phase (a subset of the spec §6 layout — SDK/transport/tools/output packages arrive in later phases):

- `go.mod`, `go.sum` — module definition and locked dependencies.
- `cmd/mcpctl/main.go` — entrypoint; runs the root command, classifies the returned error, exits.
- `internal/buildinfo/buildinfo.go` — version/commit/date vars (set by linker flags) and formatting.
- `internal/apperror/error.go` — typed `Error`, `Kind`, constructors.
- `internal/apperror/exitcode.go` — `Kind` → exit-code mapping.
- `internal/logging/logging.go` — slog logger setup and level parsing.
- `internal/cli/root.go` — root command, global/persistent flags, `Execute()`.
- `internal/cli/version.go` — `version` subcommand.
- `internal/config/config.go` — config structs + `Validate`.
- `internal/config/paths.go` — config path resolution.
- `internal/config/load.go` — TOML load with unknown-field rejection and missing-file policy.
- `.github/workflows/ci.yml` — CI: build, test, race, vet, staticcheck.
- `.goreleaser.yaml` — basic cross-platform release config.
- `Makefile` — local convenience targets mirroring CI.

Test files live beside their packages (`*_test.go`).

---

### Task 1: Go module + buildinfo package

**Files:**
- Create: `go.mod`
- Create: `internal/buildinfo/buildinfo.go`
- Test: `internal/buildinfo/buildinfo_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `buildinfo.Version`, `buildinfo.Commit`, `buildinfo.Date` — package-level `string` vars, overridable via `-ldflags -X`.
  - `func buildinfo.GoVersion() string`
  - `func buildinfo.Short() string` → `"mcpctl <Version>"`
  - `func buildinfo.Full() string` → the 4-line version block.

- [ ] **Step 1: Initialize the module**

Run:
```bash
go mod init mcpctl
go mod edit -go=1.24
```
Expected: `go.mod` created with `module mcpctl` and `go 1.24`.

- [ ] **Step 2: Write the failing test**

Create `internal/buildinfo/buildinfo_test.go`:
```go
package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestShort(t *testing.T) {
	Version = "1.2.3"
	if got, want := Short(), "mcpctl 1.2.3"; got != want {
		t.Fatalf("Short() = %q, want %q", got, want)
	}
}

func TestFull(t *testing.T) {
	Version, Commit, Date = "1.2.3", "abc123", "2026-07-16T00:00:00Z"
	got := Full()
	for _, want := range []string{
		"mcpctl version 1.2.3",
		"commit: abc123",
		"built: 2026-07-16T00:00:00Z",
		"go: " + runtime.Version(),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Full() missing %q\ngot:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/buildinfo/ -v`
Expected: FAIL — `undefined: Short` / `undefined: Full`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/buildinfo/buildinfo.go`:
```go
// Package buildinfo holds version metadata injected at build time.
package buildinfo

import (
	"fmt"
	"runtime"
)

// These are overridden at build time via -ldflags "-X".
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// GoVersion returns the Go runtime version the binary was built with.
func GoVersion() string { return runtime.Version() }

// Short returns the concise one-line version string.
func Short() string { return "mcpctl " + Version }

// Full returns the multi-line version block.
func Full() string {
	return fmt.Sprintf("mcpctl version %s\ncommit: %s\nbuilt: %s\ngo: %s",
		Version, Commit, Date, GoVersion())
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/buildinfo/ -v`
Expected: PASS (`TestShort`, `TestFull`).

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/buildinfo/
git commit -m "feat: add go module and buildinfo package"
```

---

### Task 2: apperror package (typed errors + exit codes)

**Files:**
- Create: `internal/apperror/error.go`
- Create: `internal/apperror/exitcode.go`
- Test: `internal/apperror/error_test.go`
- Test: `internal/apperror/exitcode_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type apperror.Kind int` with constants `KindInternal, KindUsage, KindConfig, KindAuth, KindConnection, KindProtocol, KindToolNotFound, KindInvalidArgs, KindToolError, KindTimeout, KindInterrupted`.
  - `type apperror.Error struct { Kind Kind; Msg string; Err error }` implementing `error` and `Unwrap()`.
  - `func apperror.New(kind Kind, format string, args ...any) *Error`
  - `func apperror.Wrap(kind Kind, err error, format string, args ...any) *Error`
  - Convenience constructors used this phase: `func Usage(format string, args ...any) *Error`, `func Config(format string, args ...any) *Error`, `func Internal(format string, args ...any) *Error`.
  - `func apperror.ExitCode(err error) int`.

- [ ] **Step 1: Write the failing test for error behavior**

Create `internal/apperror/error_test.go`:
```go
package apperror

import (
	"errors"
	"testing"
)

func TestErrorWrapsAndFormats(t *testing.T) {
	base := errors.New("boom")
	e := Wrap(KindConfig, base, "load %q", "cfg.toml")
	if e.Error() != `load "cfg.toml": boom` {
		t.Fatalf("unexpected message: %q", e.Error())
	}
	if !errors.Is(e, base) {
		t.Fatal("errors.Is should find the wrapped error")
	}
}

func TestErrorNoWrapOmitsColon(t *testing.T) {
	e := Usage("bad flag %s", "--x")
	if e.Error() != "bad flag --x" {
		t.Fatalf("unexpected message: %q", e.Error())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/apperror/ -run TestError -v`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Write the error implementation**

Create `internal/apperror/error.go`:
```go
// Package apperror defines typed application errors and their exit codes.
package apperror

import "fmt"

// Kind classifies an application error.
type Kind int

const (
	KindInternal Kind = iota
	KindUsage
	KindConfig
	KindAuth
	KindConnection
	KindProtocol
	KindToolNotFound
	KindInvalidArgs
	KindToolError
	KindTimeout
	KindInterrupted
)

// Error is a typed application error carrying an exit-code classification.
type Error struct {
	Kind Kind
	Msg  string
	Err  error // optional wrapped cause
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Msg
	}
	return e.Msg + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// New builds an Error with no wrapped cause.
func New(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// Wrap builds an Error around an existing cause.
func Wrap(kind Kind, err error, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...), Err: err}
}

// Convenience constructors.
func Usage(format string, args ...any) *Error    { return New(KindUsage, format, args...) }
func Config(format string, args ...any) *Error   { return New(KindConfig, format, args...) }
func Internal(format string, args ...any) *Error { return New(KindInternal, format, args...) }
```

- [ ] **Step 4: Run the error test to verify it passes**

Run: `go test ./internal/apperror/ -run TestError -v`
Expected: PASS.

- [ ] **Step 5: Write the failing test for exit-code mapping**

Create `internal/apperror/exitcode_test.go`:
```go
package apperror

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"usage", Usage("x"), 2},
		{"config", Config("x"), 3},
		{"internal", Internal("x"), 1},
		{"toolNotFound", New(KindToolNotFound, "x"), 7},
		{"invalidArgs", New(KindInvalidArgs, "x"), 8},
		{"toolError", New(KindToolError, "x"), 9},
		{"timeout", New(KindTimeout, "x"), 10},
		{"interrupted", New(KindInterrupted, "x"), 130},
		{"wrappedTyped", fmt.Errorf("outer: %w", Config("x")), 3},
		{"ctxDeadline", context.DeadlineExceeded, 10},
		{"ctxCanceled", context.Canceled, 130},
		{"unknown", errors.New("plain"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Fatalf("ExitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/apperror/ -run TestExitCode -v`
Expected: FAIL — `undefined: ExitCode`.

- [ ] **Step 7: Write the exit-code implementation**

Create `internal/apperror/exitcode.go`:
```go
package apperror

import (
	"context"
	"errors"
)

// ExitCode maps any error to a stable process exit code.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	var ae *Error
	if errors.As(err, &ae) {
		switch ae.Kind {
		case KindUsage:
			return 2
		case KindConfig:
			return 3
		case KindAuth:
			return 4
		case KindConnection:
			return 5
		case KindProtocol:
			return 6
		case KindToolNotFound:
			return 7
		case KindInvalidArgs:
			return 8
		case KindToolError:
			return 9
		case KindTimeout:
			return 10
		case KindInterrupted:
			return 130
		default:
			return 1
		}
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return 10
	case errors.Is(err, context.Canceled):
		return 130
	default:
		return 1
	}
}
```

- [ ] **Step 8: Run all apperror tests to verify they pass**

Run: `go test ./internal/apperror/ -v`
Expected: PASS (all cases).

- [ ] **Step 9: Commit**

```bash
git add internal/apperror/
git commit -m "feat: add typed application errors and exit-code mapping"
```

---

### Task 3: logging package

**Files:**
- Create: `internal/logging/logging.go`
- Test: `internal/logging/logging_test.go`

**Interfaces:**
- Consumes: `apperror.Usage`.
- Produces:
  - `func logging.ParseLevel(s string) (slog.Level, error)` — accepts `debug|info|warn|error` (case-insensitive); returns `apperror.Usage` on an unknown level.
  - `func logging.Setup(w io.Writer, level string) (*slog.Logger, error)` — text handler writing to `w`, filtered at the parsed level.

- [ ] **Step 1: Write the failing test**

Create `internal/logging/logging_test.go`:
```go
package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"Error": slog.LevelError,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = (%v, %v), want (%v, nil)", in, got, err, want)
		}
	}
	if _, err := ParseLevel("nope"); err == nil {
		t.Error("ParseLevel(\"nope\") should error")
	}
}

func TestSetupFiltersBelowLevel(t *testing.T) {
	var buf bytes.Buffer
	log, err := Setup(&buf, "warn")
	if err != nil {
		t.Fatal(err)
	}
	log.Info("hidden")
	log.Warn("shown")
	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Error("info should be filtered at warn level")
	}
	if !strings.Contains(out, "shown") {
		t.Error("warn should be emitted at warn level")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -v`
Expected: FAIL — `undefined: ParseLevel` / `undefined: Setup`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/logging/logging.go`:
```go
// Package logging configures the slog logger used for diagnostics.
package logging

import (
	"io"
	"log/slog"
	"strings"

	"mcpctl/internal/apperror"
)

// ParseLevel converts a level name to an slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, apperror.Usage("invalid log level %q (want debug, info, warn, or error)", s)
	}
}

// Setup builds a text logger writing to w, filtered at the given level.
func Setup(w io.Writer, level string) (*slog.Logger, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	return slog.New(h), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logging/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/logging/
git commit -m "feat: add slog logging setup and level parsing"
```

---

### Task 4: Cobra root command + global flags + main wiring

**Files:**
- Create: `internal/cli/root.go`
- Create: `cmd/mcpctl/main.go`
- Test: `internal/cli/root_test.go`

**Interfaces:**
- Consumes: `buildinfo.Short`, `apperror.Usage`, `apperror.ExitCode`, `logging.Setup`.
- Produces:
  - `type cli.GlobalFlags struct { Config, Output, LogLevel, ProtocolVersion string; Timeout, ConnectTimeout time.Duration; NoColor, NoValidate bool }`
  - `func cli.NewRootCmd() (*cobra.Command, *GlobalFlags)` — builds the root command, its persistent flags, and wires `--log-level` to the default slog logger via `PersistentPreRunE`. Subcommands are attached by their own constructors (Task 5).
  - `func cli.Execute() error` — runs the root command, normalizing any bare Cobra parse/usage error into an `*apperror.Error` of `KindUsage`.

- [ ] **Step 1: Add the Cobra dependency**

Run:
```bash
go get github.com/spf13/cobra@latest
```
Expected: `cobra` appears in `go.mod`.

- [ ] **Step 2: Write the failing test**

Create `internal/cli/root_test.go`:
```go
package cli

import (
	"bytes"
	"testing"

	"mcpctl/internal/apperror"
)

func TestVersionFlagPrintsShort(t *testing.T) {
	root, _ := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out.String(); got != "mcpctl dev\n" {
		t.Fatalf("--version = %q, want %q", got, "mcpctl dev\n")
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
```

Note: `normalize` is the internal helper `Execute()` uses to coerce bare Cobra errors; expose it for the test in the same package.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/cli/ -v`
Expected: FAIL — `undefined: NewRootCmd` / `undefined: normalize`.

- [ ] **Step 4: Write the root command implementation**

Create `internal/cli/root.go`:
```go
// Package cli builds the mcpctl command tree.
package cli

import (
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"mcpctl/internal/apperror"
	"mcpctl/internal/buildinfo"
	"mcpctl/internal/logging"
)

// GlobalFlags holds values bound to the root's persistent flags.
type GlobalFlags struct {
	Config          string
	Output          string
	Timeout         time.Duration
	ConnectTimeout  time.Duration
	LogLevel        string
	NoColor         bool
	NoValidate      bool
	ProtocolVersion string
}

// NewRootCmd builds the root command. Subcommands are attached by their own
// constructors (see version.go).
func NewRootCmd() (*cobra.Command, *GlobalFlags) {
	g := &GlobalFlags{}
	showVersion := false

	root := &cobra.Command{
		Use:           "mcpctl",
		Short:         "Connect to MCP servers and invoke their tools",
		Long:          "mcpctl connects to Model Context Protocol servers over stdio or Streamable HTTP and invokes their tools.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			logger, err := logging.Setup(os.Stderr, g.LogLevel)
			if err != nil {
				return err
			}
			slog.SetDefault(logger)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				cmd.Println(buildinfo.Short())
				return nil
			}
			return cmd.Help()
		},
	}

	f := root.PersistentFlags()
	f.StringVar(&g.Config, "config", "", "path to config file")
	f.StringVar(&g.Output, "output", "human", "output format: human|json|jsonl|yaml")
	f.DurationVar(&g.Timeout, "timeout", 30*time.Second, "overall command timeout")
	f.DurationVar(&g.ConnectTimeout, "connect-timeout", 15*time.Second, "connection/initialization timeout")
	f.StringVar(&g.LogLevel, "log-level", "warn", "log level: debug|info|warn|error")
	f.BoolVar(&g.NoColor, "no-color", false, "disable colored output")
	f.BoolVar(&g.NoValidate, "no-validate", false, "skip local argument validation")
	f.StringVar(&g.ProtocolVersion, "protocol-version", "", "override the negotiated MCP protocol version")

	root.Flags().BoolVar(&showVersion, "version", false, "print version and exit")

	// Coerce Cobra's own flag-parse errors into typed usage errors.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return apperror.Usage("%s", err.Error())
	})

	return root, g
}

// normalize coerces a bare Cobra error (unknown command, extra args) into a
// typed usage error so main can map it to exit code 2.
func normalize(err error) error {
	if err == nil {
		return nil
	}
	var ae *apperror.Error
	if errors.As(err, &ae) {
		return err
	}
	return apperror.Usage("%s", err.Error())
}

// Execute runs the root command and returns a typed error.
func Execute() error {
	root, _ := NewRootCmd()
	return normalize(root.Execute())
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/ -v`
Expected: PASS (`TestVersionFlagPrintsShort`, `TestUnknownCommandIsUsageError`, `TestInvalidLogLevelIsUsageError`). The package compiles on its own; the `version` subcommand is added in Task 5.

- [ ] **Step 6: Write the entrypoint**

Create `cmd/mcpctl/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"mcpctl/internal/apperror"
	"mcpctl/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(apperror.ExitCode(err))
	}
}
```

- [ ] **Step 7: Commit**

```bash
git add internal/cli/root.go cmd/mcpctl/main.go internal/cli/root_test.go go.mod go.sum
git commit -m "feat: add cobra root command, global flags, logging, and main wiring"
```

---

### Task 5: version subcommand

**Files:**
- Create: `internal/cli/version.go`
- Modify: `internal/cli/root.go` (attach the version subcommand)
- Test: `internal/cli/version_test.go`

**Interfaces:**
- Consumes: `buildinfo.Full`.
- Produces: `func newVersionCmd() *cobra.Command` (unexported; attached to the root inside `NewRootCmd`).

- [ ] **Step 1: Write the failing test**

Create `internal/cli/version_test.go`:
```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionSubcommand(t *testing.T) {
	root, _ := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"mcpctl version ", "commit: ", "built: ", "go: "} {
		if !strings.Contains(got, want) {
			t.Errorf("version output missing %q\ngot:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestVersion -v`
Expected: FAIL — `undefined: newVersionCmd` (still).

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/version.go`:
```go
package cli

import (
	"github.com/spf13/cobra"

	"mcpctl/internal/buildinfo"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print detailed version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(buildinfo.Full())
			return nil
		},
	}
}
```

- [ ] **Step 4: Attach the subcommand to the root**

In `internal/cli/root.go`, inside `NewRootCmd`, add the version command immediately before `return root, g`:
```go
	root.AddCommand(newVersionCmd())

	return root, g
```

- [ ] **Step 5: Run the full cli package tests to verify they pass**

Run: `go test ./internal/cli/ -v`
Expected: PASS (`TestVersionFlagPrintsShort`, `TestUnknownCommandIsUsageError`, `TestInvalidLogLevelIsUsageError`, `TestVersionSubcommand`).

- [ ] **Step 6: Build and smoke-test the binary**

Run:
```bash
go build -o /tmp/mcpctl ./cmd/mcpctl
/tmp/mcpctl --version
/tmp/mcpctl version
/tmp/mcpctl bogus; echo "exit=$?"
```
Expected: `--version` prints `mcpctl dev`; `version` prints the 4-line block; `bogus` prints a usage error to stderr and `exit=2`.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/version.go internal/cli/version_test.go internal/cli/root.go
git commit -m "feat: add version subcommand"
```

---

### Task 6: config data model + validation

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: `apperror.Config`.
- Produces:
  - `type config.Config struct` with fields `Version int`, `Defaults DefaultsConfig`, `Servers map[string]ServerConfig` (TOML-tagged per spec §5.3).
  - `type config.DefaultsConfig struct { Timeout, ConnectTimeout string }`
  - `type config.ServerConfig struct { Transport, Command string; Args []string; CWD string; Env map[string]string; URL string; Headers, HeaderEnv map[string]string; BearerToken *TokenSource }`
  - `type config.TokenSource struct { Env string }`
  - `func (c *Config) Validate() error` — enforces spec §5.3 rules, returns `apperror.Config` on the first violation.

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import "testing"

func validStdio() *Config {
	return &Config{
		Version: 1,
		Servers: map[string]ServerConfig{
			"local": {Transport: "stdio", Command: "echo"},
		},
	}
}

func TestValidateAcceptsValidStdio(t *testing.T) {
	if err := validStdio().Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateRejections(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"wrong version", func(c *Config) { c.Version = 2 }},
		{"unknown transport", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "carrier-pigeon", Command: "x"}
		}},
		{"stdio without command", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "stdio"}
		}},
		{"stdio with url", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "stdio", Command: "x", URL: "https://e.com"}
		}},
		{"http without url", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http"}
		}},
		{"http relative url", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "/mcp"}
		}},
		{"http with command", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com", Command: "x"}
		}},
		{"empty server name", func(c *Config) {
			c.Servers[""] = ServerConfig{Transport: "stdio", Command: "x"}
		}},
		{"empty header env var", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com",
				HeaderEnv: map[string]string{"Authorization": ""}}
		}},
		{"bearer and auth header", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com",
				HeaderEnv:   map[string]string{"Authorization": "MCP_AUTH"},
				BearerToken: &TokenSource{Env: "MCP_BEARER"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validStdio()
			tc.mutate(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected validation error for %q", tc.name)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestValidate -v`
Expected: FAIL — undefined types/methods.

- [ ] **Step 3: Write the implementation**

Create `internal/config/config.go`:
```go
// Package config defines the on-disk configuration model and validation.
package config

import (
	"net/url"
	"strings"

	"mcpctl/internal/apperror"
)

// Config is the top-level configuration document.
type Config struct {
	Version  int                     `toml:"version"`
	Defaults DefaultsConfig          `toml:"defaults"`
	Servers  map[string]ServerConfig `toml:"servers"`
}

// DefaultsConfig holds global default settings.
type DefaultsConfig struct {
	Timeout        string `toml:"timeout"`
	ConnectTimeout string `toml:"connect_timeout"`
}

// ServerConfig is a single named server definition.
type ServerConfig struct {
	Transport   string            `toml:"transport"`
	Command     string            `toml:"command"`
	Args        []string          `toml:"args"`
	CWD         string            `toml:"cwd"`
	Env         map[string]string `toml:"env"`
	URL         string            `toml:"url"`
	Headers     map[string]string `toml:"headers"`
	HeaderEnv   map[string]string `toml:"header_env"`
	BearerToken *TokenSource      `toml:"bearer_token"`
}

// TokenSource names the environment variable holding a bearer token.
type TokenSource struct {
	Env string `toml:"env"`
}

const (
	TransportStdio = "stdio"
	TransportHTTP  = "streamable-http"
)

// Validate enforces the configuration rules from the spec.
func (c *Config) Validate() error {
	if c.Version != 1 {
		return apperror.Config("config version must be 1, got %d", c.Version)
	}
	for name, s := range c.Servers {
		if strings.TrimSpace(name) == "" {
			return apperror.Config("server name must not be empty")
		}
		if err := s.validate(name); err != nil {
			return err
		}
	}
	return nil
}

func (s ServerConfig) validate(name string) error {
	switch s.Transport {
	case TransportStdio:
		if s.Command == "" {
			return apperror.Config("server %q: stdio transport requires a command", name)
		}
		if s.URL != "" {
			return apperror.Config("server %q: stdio transport must not set url", name)
		}
	case TransportHTTP:
		if s.Command != "" || len(s.Args) > 0 {
			return apperror.Config("server %q: streamable-http transport must not set command or args", name)
		}
		if err := validateHTTPURL(name, s.URL); err != nil {
			return err
		}
	default:
		return apperror.Config("server %q: unknown transport %q (want stdio or streamable-http)", name, s.Transport)
	}

	for header, envVar := range s.HeaderEnv {
		if strings.TrimSpace(envVar) == "" {
			return apperror.Config("server %q: header_env[%q] names an empty environment variable", name, header)
		}
	}
	if s.BearerToken != nil {
		if strings.TrimSpace(s.BearerToken.Env) == "" {
			return apperror.Config("server %q: bearer_token.env must name a non-empty environment variable", name)
		}
		if hasAuthorizationHeader(s) {
			return apperror.Config("server %q: bearer_token conflicts with an Authorization header", name)
		}
	}
	return nil
}

func validateHTTPURL(name, raw string) error {
	if raw == "" {
		return apperror.Config("server %q: streamable-http transport requires a url", name)
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return apperror.Config("server %q: url must be an absolute http or https URL", name)
	}
	return nil
}

func hasAuthorizationHeader(s ServerConfig) bool {
	for h := range s.Headers {
		if strings.EqualFold(h, "Authorization") {
			return true
		}
	}
	for h := range s.HeaderEnv {
		if strings.EqualFold(h, "Authorization") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestValidate -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add config data model and validation"
```

---

### Task 7: config path resolution

**Files:**
- Create: `internal/config/paths.go`
- Test: `internal/config/paths_test.go`

**Interfaces:**
- Consumes: nothing beyond stdlib.
- Produces:
  - `func config.DefaultPath() (string, error)` — `$XDG_CONFIG_HOME/mcpctl/config.toml` when `XDG_CONFIG_HOME` is set, else `<os.UserConfigDir()>/mcpctl/config.toml`.
  - `func config.Resolve(override string) (path string, isDefault bool, err error)` — returns `override` (isDefault=false) when non-empty, otherwise `DefaultPath()` (isDefault=true).

- [ ] **Step 1: Write the failing test**

Create `internal/config/paths_test.go`:
```go
package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.FromSlash("/xdg/mcpctl/config.toml"); got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestResolvePrefersOverride(t *testing.T) {
	got, isDefault, err := Resolve("/tmp/custom.toml")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/custom.toml" || isDefault {
		t.Fatalf("Resolve override = (%q, %v), want (/tmp/custom.toml, false)", got, isDefault)
	}
}

func TestResolveFallsBackToDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, isDefault, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.FromSlash("/xdg/mcpctl/config.toml"); got != want || !isDefault {
		t.Fatalf("Resolve default = (%q, %v), want (%q, true)", got, isDefault, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run 'TestDefaultPath|TestResolve' -v`
Expected: FAIL — `undefined: DefaultPath` / `undefined: Resolve`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/config/paths.go`:
```go
package config

import (
	"os"
	"path/filepath"
)

const (
	appDir   = "mcpctl"
	fileName = "config.toml"
)

// DefaultPath returns the platform-appropriate default config file path,
// preferring XDG_CONFIG_HOME when set.
func DefaultPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, appDir, fileName), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appDir, fileName), nil
}

// Resolve returns the config path to use. A non-empty override wins;
// otherwise the default path is used.
func Resolve(override string) (path string, isDefault bool, err error) {
	if override != "" {
		return override, false, nil
	}
	p, err := DefaultPath()
	return p, true, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run 'TestDefaultPath|TestResolve' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/paths.go internal/config/paths_test.go
git commit -m "feat: add config path resolution with XDG support"
```

---

### Task 8: config loading (TOML + unknown-field rejection + missing-file policy)

**Files:**
- Create: `internal/config/load.go`
- Test: `internal/config/load_test.go`

**Interfaces:**
- Consumes: `config.Config.Validate`, `config.Resolve`, `apperror.Config`, `github.com/pelletier/go-toml/v2`.
- Produces:
  - `func config.Load(path string) (*Config, error)` — strict load: reads and TOML-decodes `path` with unknown-field rejection, then `Validate`; returns `apperror.Config` if the file is missing.
  - `func config.LoadResolved(override string) (*Config, error)` — resolves the path; a missing **default** path yields an empty valid config (`Version: 1`, no servers); a missing **explicit** path is a config error.

- [ ] **Step 1: Add the TOML dependency**

Run:
```bash
go get github.com/pelletier/go-toml/v2@latest
```
Expected: dependency added to `go.mod`.

- [ ] **Step 2: Write the failing test**

Create `internal/config/load_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleTOML = `
version = 1

[servers.local-fs]
transport = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(writeTemp(t, sampleTOML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := cfg.Servers["local-fs"]
	if !ok || s.Command != "npx" || len(s.Args) != 3 {
		t.Fatalf("unexpected server config: %+v", s)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	_, err := Load(writeTemp(t, "version = 1\nbogus = true\n"))
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestLoadMissingExplicitFileErrors(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err == nil {
		t.Fatal("expected config error for missing explicit file")
	}
}

func TestLoadResolvedMissingDefaultIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no config.toml written here
	cfg, err := LoadResolved("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 1 || len(cfg.Servers) != 0 {
		t.Fatalf("expected empty valid config, got %+v", cfg)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad -v`
Expected: FAIL — `undefined: Load` / `undefined: LoadResolved`.

- [ ] **Step 4: Write the implementation**

Create `internal/config/load.go`:
```go
package config

import (
	"errors"
	"io/fs"
	"os"

	toml "github.com/pelletier/go-toml/v2"

	"mcpctl/internal/apperror"
)

// Load reads and validates a config file. A missing file is a config error.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, apperror.Config("config file not found: %s", path)
		}
		return nil, apperror.Wrap(apperror.KindConfig, err, "open config %s", path)
	}
	defer f.Close()

	var cfg Config
	dec := toml.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, apperror.Wrap(apperror.KindConfig, err, "parse config %s", path)
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadResolved resolves the config path from an optional override and loads it.
// A missing default-path file yields an empty valid config; a missing explicit
// override is an error.
func LoadResolved(override string) (*Config, error) {
	path, isDefault, err := Resolve(override)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindConfig, err, "resolve config path")
	}
	cfg, err := Load(path)
	if err != nil {
		var ae *apperror.Error
		if isDefault && errors.As(err, &ae) && ae.Kind == apperror.KindConfig && fileMissing(path) {
			return &Config{Version: 1, Servers: map[string]ServerConfig{}}, nil
		}
		return nil, err
	}
	return cfg, nil
}

func fileMissing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests).

- [ ] **Step 6: Commit**

```bash
git add internal/config/load.go internal/config/load_test.go go.mod go.sum
git commit -m "feat: add TOML config loading with unknown-field rejection"
```

---

### Task 9: CI, GoReleaser, and Makefile

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.goreleaser.yaml`
- Create: `Makefile`

**Interfaces:**
- Consumes: everything above (the whole module must build and test).
- Produces: repeatable local + CI quality gates and a release configuration that injects `buildinfo` vars.

- [ ] **Step 1: Write the Makefile**

Create `Makefile`:
```makefile
GO ?= go

.PHONY: build test race vet staticcheck check

build:
	$(GO) build ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

staticcheck:
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./...

check: build test race vet staticcheck
```

- [ ] **Step 2: Run the full local gate to verify it passes**

Run: `make check`
Expected: build, test, race, vet, and staticcheck all succeed with no findings.

- [ ] **Step 3: Write the CI workflow**

Create `.github/workflows/ci.yml`:
```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - run: go build ./...
      - run: go test ./...
      - run: go test -race ./...
      - run: go vet ./...
      - run: go run honnef.co/go/tools/cmd/staticcheck@latest ./...

  cross-build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - { goos: linux, goarch: amd64 }
          - { goos: darwin, goarch: arm64 }
          - { goos: windows, goarch: amd64 }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: go build ./cmd/mcpctl
```

- [ ] **Step 4: Write the GoReleaser config**

Create `.goreleaser.yaml`:
```yaml
version: 2

builds:
  - id: mcpctl
    main: ./cmd/mcpctl
    binary: mcpctl
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X mcpctl/internal/buildinfo.Version={{.Version}}
      - -X mcpctl/internal/buildinfo.Commit={{.Commit}}
      - -X mcpctl/internal/buildinfo.Date={{.Date}}

archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - README.md
      - LICENSE

checksum:
  name_template: 'checksums.txt'
```

- [ ] **Step 5: Verify the GoReleaser config and ldflags injection**

Run:
```bash
go run github.com/goreleaser/goreleaser/v2@latest check
go build -ldflags "-X mcpctl/internal/buildinfo.Version=9.9.9" -o /tmp/mcpctl ./cmd/mcpctl
/tmp/mcpctl --version
```
Expected: `goreleaser check` reports the config is valid; the binary prints `mcpctl 9.9.9` (confirming linker injection works).

- [ ] **Step 6: Commit**

```bash
git add Makefile .github/workflows/ci.yml .goreleaser.yaml
git commit -m "ci: add build/test/lint gates, cross-build matrix, and goreleaser config"
```

---

## Phase 1 Acceptance

Phase 1 is complete when:

- `make check` passes (build, test, `-race`, vet, staticcheck — all clean).
- `mcpctl --version` prints `mcpctl <version>`; `mcpctl version` prints the 4-line block.
- Unknown command / unknown flag exits `2`; the message goes to **stderr**.
- `internal/config` loads a valid TOML file, rejects unknown fields, errors on a missing explicit `--config` path, and returns an empty valid config for a missing default path.
- The binary cross-builds for `linux/amd64`, `darwin/arm64`, and `windows/amd64` (spec §20.9).
- Linker-flag version injection is proven (Task 9, Step 5).

---

## Roadmap: subsequent plans

Each later phase gets its own plan file, written when its predecessor lands. **The transport spike must precede Phase 2** — it is the first coding task of the Phase 2 plan and gates the two SDK facts flagged in spec §2.2 (§9 `http.Client` injection, §4.3.3 pagination shape).

- **Phase 2 — Stdio MVP:** transport spike → `internal/client` interface + stdio factory → MCP initialize → `tools list`/`describe`/`call` → process-group cleanup → in-process + subprocess test servers → human + JSON output.
- **Phase 3 — Streamable HTTP:** HTTP transport, header/bearer resolution, custom `CheckRedirect` credential stripping, timeout semantics, `httptest`-based integration tests.
- **Phase 4 — UX & validation:** `server add/list/show/remove` (adds `internal/config/save.go`), JSON Lines + YAML output, JSON Schema validation, pagination safeguards, terminal control-char sanitization, secret redaction in logs.
- **Phase 5 — Release readiness:** race-tested end-to-end, shell completions, README, full acceptance-test run, GoReleaser artifacts.
