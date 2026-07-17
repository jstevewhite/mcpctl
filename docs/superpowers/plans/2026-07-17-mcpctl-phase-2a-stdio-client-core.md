# mcpctl Phase 2A — Stdio Client Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a tested internal MCP **stdio** client — an SDK-free `Client` interface backed by the official Go MCP SDK, with correct process-group cleanup and a deterministic in-repo test server — with no user-facing CLI commands yet (those are Plan 2B).

**Architecture:** All SDK (`github.com/modelcontextprotocol/go-sdk`) types are confined to `internal/client`; command handlers (Plan 2B) will see only the SDK-free boundary types (`ToolInfo`, `ToolResult`, `ServerInfo`, `ToolPage`) defined here. The SDK's `CommandTransport` starts the child from a caller-built `*exec.Cmd`, so we set the child into its own process group (`Setpgid`) before starting and sweep the group with `SIGKILL` after the SDK's own graceful `Close()`, because the SDK only terminates the direct child (confirmed by the transport spike — an orphaned grandchild otherwise survives). Pagination is a manual `ListTools`/`NextCursor` loop with repeated-cursor detection and a page cap.

**Tech Stack:** Go 1.25, `github.com/modelcontextprotocol/go-sdk` v1.5.0, standard library `os/exec`/`syscall`.

## Global Constraints

Values copied verbatim from `mcpctl-spec.md` (as corrected by the Phase 2 spike) and the spike findings.

- **Go version:** Go 1.25+ (the SDK's module declares `go 1.25`; `GOTOOLCHAIN=auto` fetches it). Module `go` directive and CI `go-version` are `1.25`.
- **SDK pin:** `github.com/modelcontextprotocol/go-sdk` at **v1.5.0** exactly. Primary package `.../mcp`; JSON-RPC error type in `.../jsonrpc`.
- **SDK confinement:** no `mcp.*` or `jsonrpc.*` type may appear outside `internal/client`. Conversions happen at the package boundary.
- **No CGo.** Pure-Go only.
- **Output contract:** results→stdout, diagnostics/logs→stderr. The child server's stderr is forwarded to our stderr (we must set `cmd.Stderr`; the SDK does not).
- **Process cleanup (spec §8.3):** on Unix, the child runs in its own process group (`SysProcAttr.Setpgid = true`) and the group is swept with `SIGKILL` after the session closes; a misbehaving server's whole tree must not survive. Graceful direct-child shutdown is the SDK's `CommandTransport.Close()` (stdin-close → SIGTERM → SIGKILL), with `TerminateDuration` set to `2s` to match the spec's suggested timing.
- **Exit-code kinds** (from `internal/apperror`, already built): connection/transport → `KindConnection` (5); MCP protocol/init → `KindProtocol` (6). The client wraps SDK errors into these.
- **Verified SDK API (from the spike — use exactly):**
  - `mcp.NewClient(&mcp.Implementation{Name, Version}, nil) *mcp.Client`
  - `&mcp.CommandTransport{Command: *exec.Cmd, TerminateDuration: time.Duration}`
  - `client.Connect(ctx, transport, nil) (*mcp.ClientSession, error)` — performs the full initialize handshake.
  - `session.InitializeResult() *mcp.InitializeResult` → `.ProtocolVersion string`, `.ServerInfo *mcp.Implementation` (`.Name`, `.Version`), `.Capabilities.Tools != nil` (bool: supports tools).
  - `session.ListTools(ctx, &mcp.ListToolsParams{Cursor: string}) (*mcp.ListToolsResult, error)` → `.Tools []*mcp.Tool`, `.NextCursor string`. No client page size.
  - `mcp.Tool` fields: `.Name`, `.Description`, `.InputSchema any` (client-side: `map[string]any`), `.OutputSchema any`, `.Title`.
  - `session.CallTool(ctx, &mcp.CallToolParams{Name: string, Arguments: any}) (*mcp.CallToolResult, error)` — pass `map[string]any` args.
  - `mcp.CallToolResult` fields: `.Content []mcp.Content`, `.StructuredContent any`, `.IsError bool`.
  - `mcp.Content` is an interface; concrete types `*mcp.TextContent{Text string}`, `*mcp.ImageContent{Data []byte, MIMEType string}`, `*mcp.AudioContent{Data []byte, MIMEType string}`, `*mcp.EmbeddedResource`, `*mcp.ResourceLink`.
  - `session.Close() error` — terminates only the direct child.
  - Protocol errors: `errors.As(err, &wireErr)` with `var wireErr *jsonrpc.Error` (`jsonrpc "github.com/modelcontextprotocol/go-sdk/jsonrpc"`); fields `.Code int64`, `.Message string`.
  - Server side (test server): `mcp.NewServer(&mcp.Implementation{...}, &mcp.ServerOptions{PageSize: int})`, `mcp.AddTool(server, &mcp.Tool{Name, Description}, func(ctx, *mcp.CallToolRequest, ArgsStruct) (*mcp.CallToolResult, any, error))`, `server.Run(ctx, &mcp.StdioTransport{}) error` (returns a non-nil error wrapping `io.EOF` on normal client-initiated shutdown — treat EOF as clean exit 0).

---

## File Structure

Created in this plan:

- `internal/client/types.go` — SDK-free boundary types.
- `internal/client/client.go` — the `Client` interface + `StdioSpec` + `clientInfo()`.
- `internal/client/convert.go` — SDK→boundary conversions.
- `internal/client/stdio.go` — build `*exec.Cmd`, transport, connect; the SDK-backed `Client` impl (connect/list/call/close).
- `internal/client/*_test.go` — conversion unit tests + integration tests against the test server.
- `internal/client/main_test.go` — `TestMain` builds the test server binary once.
- `internal/process/process.go` — portable `Setup`/`KillGroup` API.
- `internal/process/process_unix.go` — `//go:build !windows` process-group setup + group kill.
- `internal/process/process_windows.go` — `//go:build windows` best-effort (direct child only; job objects deferred).
- `internal/process/process_unix_test.go` — group-kill reaps a grandchild.
- `internal/testserver/stdio/main.go` — deterministic MCP test server binary.

Modified: `go.mod` (go 1.25 + SDK dep), `go.sum`, `.github/workflows/ci.yml` (go-version 1.25).

---

### Task 1: Toolchain bump + SDK dependency + interface/boundary types

**Files:**
- Modify: `go.mod`, `.github/workflows/ci.yml`
- Create: `internal/client/types.go`, `internal/client/client.go`
- Test: `internal/client/client_test.go`

**Interfaces:**
- Consumes: `buildinfo.Version` (existing).
- Produces:
  - Boundary types `ToolInfo`, `ContentKind` (+consts), `ContentBlock`, `ToolResult`, `ServerInfo`, `ToolPage`, `StdioSpec`.
  - `type client.Client interface { ServerInfo() ServerInfo; ListTools(ctx, cursor string) (ToolPage, error); ListAllTools(ctx, maxPages int) ([]ToolInfo, error); CallTool(ctx, name string, args map[string]any) (ToolResult, error); Close() error }`
  - `func clientInfo() *mcp.Implementation` (unexported; anchors the SDK dependency).

- [ ] **Step 1: Bump the Go directive and add the SDK**

Run:
```bash
go mod edit -go=1.25
go get github.com/modelcontextprotocol/go-sdk@v1.5.0
```
Expected: `go.mod` shows `go 1.25` and requires `github.com/modelcontextprotocol/go-sdk v1.5.0`. (`GOTOOLCHAIN=auto` may fetch go1.25 — that's expected.)

- [ ] **Step 2: Update CI to Go 1.25**

In `.github/workflows/ci.yml`, change every `go-version: '1.24'` to `go-version: '1.25'` (there are two: the `test` job and the `cross-build` job).

- [ ] **Step 3: Write the failing test**

Create `internal/client/client_test.go`:
```go
package client

import "testing"

func TestClientInfo(t *testing.T) {
	info := clientInfo()
	if info.Name != "mcpctl" {
		t.Fatalf("clientInfo().Name = %q, want %q", info.Name, "mcpctl")
	}
	if info.Version == "" {
		t.Fatal("clientInfo().Version must not be empty")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/client/ -run TestClientInfo -v`
Expected: FAIL — package does not compile (`undefined: clientInfo` and missing types).

- [ ] **Step 5: Write the boundary types**

Create `internal/client/types.go`:
```go
// Package client is the SDK-free boundary between mcpctl's command handlers
// and the official Go MCP SDK. Only this package imports the SDK.
package client

// ToolInfo is an SDK-free description of a tool.
type ToolInfo struct {
	Name         string
	Description  string
	Title        string
	InputSchema  any // JSON schema as decoded by the SDK (map[string]any) or nil
	OutputSchema any
}

// ContentKind classifies a content block in a tool result.
type ContentKind string

const (
	KindText     ContentKind = "text"
	KindImage    ContentKind = "image"
	KindAudio    ContentKind = "audio"
	KindResource ContentKind = "resource"
	KindUnknown  ContentKind = "unknown"
)

// ContentBlock is an SDK-free representation of one content item.
type ContentBlock struct {
	Kind     ContentKind
	Text     string // KindText
	MIMEType string // KindImage/KindAudio
	Data     []byte // KindImage/KindAudio (raw bytes)
}

// ToolResult is an SDK-free representation of a tools/call result.
type ToolResult struct {
	Content    []ContentBlock
	Structured any // structuredContent, or nil
	IsError    bool
}

// ServerInfo captures the initialized server's identity and capabilities.
type ServerInfo struct {
	Name            string
	Version         string
	ProtocolVersion string
	SupportsTools   bool
}

// ToolPage is one page of a tools/list response.
type ToolPage struct {
	Tools      []ToolInfo
	NextCursor string
}

// StdioSpec describes a stdio server to launch. It is SDK-free so callers
// (the CLI) build it from configuration without importing the SDK.
type StdioSpec struct {
	Command string
	Args    []string
	CWD     string            // working directory; "" = inherit
	Env     map[string]string // additions/overrides to the inherited environment
}
```

- [ ] **Step 6: Write the interface**

Create `internal/client/client.go`:
```go
package client

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcpctl/internal/buildinfo"
)

// Client is the SDK-free interface command handlers use. Implementations
// convert SDK types to the boundary types in types.go and confine all
// SDK-specific behavior to this package.
type Client interface {
	// ServerInfo returns identity/capability info captured at initialize time.
	ServerInfo() ServerInfo
	// ListTools returns a single page of tools; cursor "" requests the first.
	ListTools(ctx context.Context, cursor string) (ToolPage, error)
	// ListAllTools follows pagination to completion, detecting a repeated
	// cursor and capping the number of pages at maxPages.
	ListAllTools(ctx context.Context, maxPages int) ([]ToolInfo, error)
	// CallTool invokes a tool with JSON-object arguments.
	CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error)
	// Close terminates the session and any child process group.
	Close() error
}

// clientInfo is the Implementation mcpctl advertises to servers.
func clientInfo() *mcp.Implementation {
	return &mcp.Implementation{Name: "mcpctl", Version: buildinfo.Version}
}
```

- [ ] **Step 7: Run test to verify it passes; verify the whole module still builds on Go 1.25**

Run:
```bash
go test ./internal/client/ -run TestClientInfo -v
go build ./...
go test ./...
```
Expected: `TestClientInfo` PASS; whole module builds and all existing Phase 1 tests still pass under Go 1.25.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum .github/workflows/ci.yml internal/client/
git commit -m "feat(client): add SDK dependency, boundary types, and Client interface"
```

---

### Task 2: SDK→boundary conversions

**Files:**
- Create: `internal/client/convert.go`
- Test: `internal/client/convert_test.go`

**Interfaces:**
- Consumes: SDK types `*mcp.Tool`, `*mcp.CallToolResult`, `mcp.Content`.
- Produces (unexported, used by Task 5–7): `func toToolInfo(*mcp.Tool) ToolInfo`, `func toToolResult(*mcp.CallToolResult) ToolResult`, `func toContentBlock(mcp.Content) ContentBlock`.

- [ ] **Step 1: Write the failing test**

Create `internal/client/convert_test.go`:
```go
package client

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToToolInfo(t *testing.T) {
	got := toToolInfo(&mcp.Tool{
		Name:        "echo",
		Description: "echo back",
		Title:       "Echo",
		InputSchema: map[string]any{"type": "object"},
	})
	if got.Name != "echo" || got.Description != "echo back" || got.Title != "Echo" {
		t.Fatalf("unexpected ToolInfo: %+v", got)
	}
	if _, ok := got.InputSchema.(map[string]any); !ok {
		t.Fatalf("InputSchema not preserved: %T", got.InputSchema)
	}
}

func TestToToolResult(t *testing.T) {
	got := toToolResult(&mcp.CallToolResult{
		IsError:           true,
		StructuredContent: map[string]any{"sum": 3},
		Content: []mcp.Content{
			&mcp.TextContent{Text: "hi"},
			&mcp.ImageContent{MIMEType: "image/png", Data: []byte{1, 2, 3}},
		},
	})
	if !got.IsError {
		t.Error("IsError not preserved")
	}
	if got.Structured == nil {
		t.Error("StructuredContent not preserved")
	}
	if len(got.Content) != 2 {
		t.Fatalf("want 2 content blocks, got %d", len(got.Content))
	}
	if got.Content[0].Kind != KindText || got.Content[0].Text != "hi" {
		t.Errorf("text block wrong: %+v", got.Content[0])
	}
	if got.Content[1].Kind != KindImage || got.Content[1].MIMEType != "image/png" || len(got.Content[1].Data) != 3 {
		t.Errorf("image block wrong: %+v", got.Content[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/client/ -run 'TestToTool' -v`
Expected: FAIL — `undefined: toToolInfo` / `toToolResult`.

- [ ] **Step 3: Write the implementation**

Create `internal/client/convert.go`:
```go
package client

import "github.com/modelcontextprotocol/go-sdk/mcp"

func toToolInfo(t *mcp.Tool) ToolInfo {
	return ToolInfo{
		Name:         t.Name,
		Description:  t.Description,
		Title:        t.Title,
		InputSchema:  t.InputSchema,
		OutputSchema: t.OutputSchema,
	}
}

func toToolResult(r *mcp.CallToolResult) ToolResult {
	out := ToolResult{
		Structured: r.StructuredContent,
		IsError:    r.IsError,
	}
	for _, c := range r.Content {
		out.Content = append(out.Content, toContentBlock(c))
	}
	return out
}

// toContentBlock converts one SDK content item. Text/image/audio are captured
// in full; other kinds (embedded resource, resource link) are recorded as
// KindResource with kind only — richer resource rendering is added when
// binary/resource output is built (Plan 2B / Phase 4).
func toContentBlock(c mcp.Content) ContentBlock {
	switch v := c.(type) {
	case *mcp.TextContent:
		return ContentBlock{Kind: KindText, Text: v.Text}
	case *mcp.ImageContent:
		return ContentBlock{Kind: KindImage, MIMEType: v.MIMEType, Data: v.Data}
	case *mcp.AudioContent:
		return ContentBlock{Kind: KindAudio, MIMEType: v.MIMEType, Data: v.Data}
	case *mcp.EmbeddedResource, *mcp.ResourceLink:
		return ContentBlock{Kind: KindResource}
	default:
		return ContentBlock{Kind: KindUnknown}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/client/ -run 'TestToTool' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/client/convert.go internal/client/convert_test.go
git commit -m "feat(client): convert SDK tool/result types to boundary types"
```

---

### Task 3: Process-group launch + cleanup (`internal/process`)

**Files:**
- Create: `internal/process/process.go`, `internal/process/process_unix.go`, `internal/process/process_windows.go`
- Test: `internal/process/process_unix_test.go`

**Interfaces:**
- Consumes: `os/exec`.
- Produces: `func process.Setup(cmd *exec.Cmd)` (call before `Start`); `func process.KillGroup(cmd *exec.Cmd)` (best-effort SIGKILL of the whole group; call after the SDK's graceful `Close`).

- [ ] **Step 1: Write the portable API**

Create `internal/process/process.go`:
```go
// Package process handles OS-specific process-group setup and teardown for
// spawned stdio MCP servers. The MCP SDK terminates only the direct child;
// this package ensures a misbehaving server's descendants are cleaned up too.
package process

import "os/exec"

// Setup configures cmd so its descendants can be terminated as a group.
// Must be called before cmd.Start().
func Setup(cmd *exec.Cmd) { setup(cmd) }

// KillGroup force-terminates cmd's entire process group (best-effort; errors
// are ignored because the group may already be gone). Call it after the SDK's
// own graceful session Close, to sweep any orphaned descendants.
func KillGroup(cmd *exec.Cmd) { killGroup(cmd) }
```

- [ ] **Step 2: Write the Unix implementation**

Create `internal/process/process_unix.go`:
```go
//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

func setup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Put the child in its own process group (pgid == child pid) so a single
	// negative-pid signal reaches the whole tree.
	cmd.SysProcAttr.Setpgid = true
}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// Negative pid targets the process group led by the child.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
```

- [ ] **Step 3: Write the Windows implementation**

Create `internal/process/process_windows.go`:
```go
//go:build windows

package process

import "os/exec"

// On Windows there is no process-group signaling equivalent to Unix pgids
// without job objects. The SDK's CommandTransport.Close already terminates the
// direct child; full descendant cleanup via a Job Object is deferred (tracked).
func setup(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
```

- [ ] **Step 4: Write the failing Unix test**

Create `internal/process/process_unix_test.go`:
```go
//go:build !windows

package process

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func alive(pid int) bool { return syscall.Kill(pid, syscall.Signal(0)) == nil }

// The child shell backgrounds a grandchild sleep in the same process group and
// prints the grandchild's PID. KillGroup must reap the grandchild, not just the
// direct child.
func TestKillGroupReapsGrandchild(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $!; wait")
	Setup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("reading grandchild pid: %v", err)
	}
	gpid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parsing grandchild pid %q: %v", line, err)
	}
	if !alive(gpid) {
		t.Fatalf("grandchild %d should be alive before KillGroup", gpid)
	}

	KillGroup(cmd)
	_ = cmd.Wait() // reap the direct child (killed by the group signal)

	deadline := time.Now().Add(2 * time.Second)
	for alive(gpid) {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still alive 2s after KillGroup", gpid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/process/ -run TestKillGroup -v`
Expected: PASS — the grandchild is reaped. (If it fails "still alive", `Setpgid`/negative-pid signaling is wrong.)

- [ ] **Step 6: Confirm Windows build compiles**

Run: `GOOS=windows GOARCH=amd64 go build ./internal/process/`
Expected: builds (the `//go:build` split is correct).

- [ ] **Step 7: Commit**

```bash
git add internal/process/
git commit -m "feat(process): process-group setup and group kill for stdio servers"
```

---

### Task 4: Deterministic stdio test MCP server

**Files:**
- Create: `internal/testserver/stdio/main.go`

**Interfaces:**
- Consumes: the SDK server API.
- Produces: a standalone binary `internal/testserver/stdio` exposing tools used by the Task 5–7 integration tests: `echo`, `add` (structured), `boom` (tool error), `slow`, and padding tools (`pad_1..pad_4`) so that with `PageSize=2` the list spans multiple pages. Writes one startup line to stderr.

- [ ] **Step 1: Write the server**

Create `internal/testserver/stdio/main.go`:
```go
// Command stdio is a deterministic MCP test server used by internal/client
// integration tests. It talks MCP over stdio and exposes a fixed set of tools.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoArgs struct {
	Message string `json:"message" jsonschema:"the message to echo back"`
}

type addArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

type slowArgs struct {
	Seconds int `json:"seconds"`
}

func main() {
	logger := log.New(os.Stderr, "[test-server] ", 0)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcpctl-test-server",
		Version: "0.0.1",
	}, &mcp.ServerOptions{PageSize: 2})

	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo back the given message"},
		func(ctx context.Context, req *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Message}}}, nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "add", Description: "add two integers, returning structured content"},
		func(ctx context.Context, req *mcp.CallToolRequest, args addArgs) (*mcp.CallToolResult, any, error) {
			sum := args.A + args.B
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%d", sum)}},
				StructuredContent: map[string]any{"sum": sum},
			}, nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "boom", Description: "always fails with a tool-level error"},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "tool failed on purpose"}},
			}, nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "slow", Description: "sleeps for the given number of seconds"},
		func(ctx context.Context, req *mcp.CallToolRequest, args slowArgs) (*mcp.CallToolResult, any, error) {
			select {
			case <-time.After(time.Duration(args.Seconds) * time.Second):
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "done"}}}, nil, nil
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		})

	// Padding tools so that with PageSize=2 and 8 tools the list spans 4 pages.
	for i := 1; i <= 4; i++ {
		name := fmt.Sprintf("pad_%d", i)
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: "padding tool"},
			func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pad"}}}, nil, nil
			})
	}

	logger.Printf("test server starting pid=%d", os.Getpid())
	err := server.Run(context.Background(), &mcp.StdioTransport{})
	// Normal client-initiated shutdown surfaces as an error wrapping io.EOF
	// (the SDK exports no sentinel for it). Treat EOF as a clean exit.
	if err != nil && !errors.Is(err, io.EOF) {
		logger.Fatalf("server exited with error: %v", err)
	}
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build -o /tmp/test-server ./internal/testserver/stdio && echo built`
Expected: `built` (binary compiles). It is exercised end-to-end by Task 5.

- [ ] **Step 3: Commit**

```bash
git add internal/testserver/
git commit -m "test(testserver): deterministic stdio MCP test server"
```

---

### Task 5: Stdio client connect + capabilities + cleanup

**Files:**
- Create: `internal/client/stdio.go`
- Create: `internal/client/main_test.go`
- Test: `internal/client/stdio_test.go`

**Interfaces:**
- Consumes: `clientInfo()`, `process.Setup`/`process.KillGroup`, `apperror.Wrap`/`KindConnection`, the SDK.
- Produces:
  - `func DialStdio(ctx context.Context, spec StdioSpec) (Client, error)` — builds the `*exec.Cmd`, sets the process group and stderr forwarding, connects (initialize), and returns a live `Client`.
  - `*stdioClient` implementing `ServerInfo()` and `Close()` (list/call methods added in Tasks 6–7).
  - Test helper: `testServerPath` (package var set by `TestMain`).

- [ ] **Step 1: Write `TestMain` to build the test server once**

Create `internal/client/main_test.go`:
```go
package client

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var testServerPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mcpctl-testserver-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	testServerPath = filepath.Join(dir, "test-server")
	build := exec.Command("go", "build", "-o", testServerPath, "mcpctl/internal/testserver/stdio")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("building test server: " + err.Error())
	}
	os.Exit(m.Run())
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/client/stdio_test.go`:
```go
package client

import (
	"context"
	"syscall"
	"testing"
	"time"
)

func procAlive(pid int) bool { return syscall.Kill(pid, syscall.Signal(0)) == nil }

func dialTestServer(t *testing.T) (*stdioClient, context.Context) {
	t.Helper()
	ctx := context.Background()
	c, err := DialStdio(ctx, StdioSpec{Command: testServerPath})
	if err != nil {
		t.Fatalf("DialStdio: %v", err)
	}
	return c.(*stdioClient), ctx
}

func TestDialAndServerInfo(t *testing.T) {
	c, _ := dialTestServer(t)
	defer c.Close()

	info := c.ServerInfo()
	if info.Name != "mcpctl-test-server" {
		t.Errorf("ServerInfo().Name = %q, want mcpctl-test-server", info.Name)
	}
	if !info.SupportsTools {
		t.Error("expected SupportsTools = true")
	}
	if info.ProtocolVersion == "" {
		t.Error("expected a negotiated protocol version")
	}
}

func TestCloseTerminatesChild(t *testing.T) {
	c, _ := dialTestServer(t)
	pid := c.cmd.Process.Pid
	if !procAlive(pid) {
		t.Fatalf("child %d should be alive after dial", pid)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for procAlive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d still alive 3s after Close", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/client/ -run 'TestDial|TestClose' -v`
Expected: FAIL — `undefined: DialStdio` / `stdioClient`.

- [ ] **Step 4: Write the implementation**

Create `internal/client/stdio.go`:
```go
package client

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcpctl/internal/apperror"
	"mcpctl/internal/process"
)

type stdioClient struct {
	session *mcp.ClientSession
	cmd     *exec.Cmd
	info    ServerInfo
}

// DialStdio launches the stdio server described by spec, performs the MCP
// initialize handshake, and returns a live Client.
func DialStdio(ctx context.Context, spec StdioSpec) (Client, error) {
	cmd := exec.Command(spec.Command, spec.Args...)
	if spec.CWD != "" {
		cmd.Dir = spec.CWD
	}
	cmd.Env = mergedEnv(spec.Env)
	// Forward the server's stderr to ours; the SDK does not wire it.
	cmd.Stderr = os.Stderr
	// Put the child in its own process group so we can reap descendants.
	process.Setup(cmd)

	transport := &mcp.CommandTransport{Command: cmd, TerminateDuration: 2 * time.Second}
	client := mcp.NewClient(clientInfo(), nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		process.KillGroup(cmd) // Connect may have started the child before failing.
		return nil, apperror.Wrap(apperror.KindConnection, err, "connect to stdio server %q", spec.Command)
	}

	init := session.InitializeResult()
	c := &stdioClient{
		session: session,
		cmd:     cmd,
		info: ServerInfo{
			Name:            init.ServerInfo.Name,
			Version:         init.ServerInfo.Version,
			ProtocolVersion: init.ProtocolVersion,
			SupportsTools:   init.Capabilities.Tools != nil,
		},
	}
	return c, nil
}

// mergedEnv returns the inherited environment plus the configured overrides.
// A nil/empty map leaves the environment untouched.
func mergedEnv(overrides map[string]string) []string {
	if len(overrides) == 0 {
		return nil // nil means inherit os.Environ() (standard os/exec behavior)
	}
	env := os.Environ()
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}

func (c *stdioClient) ServerInfo() ServerInfo { return c.info }

// Close gracefully closes the session (the SDK terminates the direct child)
// and then sweeps the process group to reap any orphaned descendants.
func (c *stdioClient) Close() error {
	err := c.session.Close()
	process.KillGroup(c.cmd)
	return err
}
```

Note on `mergedEnv`: appending duplicate keys to `os.Environ()` is valid — for `os/exec`, the last value of a duplicated key wins, so overrides placed after the inherited entries take effect.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/client/ -run 'TestDial|TestClose' -v`
Expected: PASS — server info is read, and the child is gone within 3s of Close.

- [ ] **Step 6: Commit**

```bash
git add internal/client/stdio.go internal/client/main_test.go internal/client/stdio_test.go
git commit -m "feat(client): dial stdio server, read capabilities, clean up on close"
```

---

### Task 6: Tool listing with pagination + repeated-cursor detection

**Files:**
- Modify: `internal/client/stdio.go` (add `ListTools`, `ListAllTools`)
- Test: `internal/client/list_test.go`

**Interfaces:**
- Consumes: `session.ListTools`, `toToolInfo`, `apperror`.
- Produces: `(*stdioClient) ListTools(ctx, cursor) (ToolPage, error)` and `(*stdioClient) ListAllTools(ctx, maxPages) ([]ToolInfo, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/client/list_test.go`:
```go
package client

import (
	"context"
	"testing"
)

func TestListToolsPaginates(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	// First page (server PageSize=2) must be a partial page with a cursor.
	page, err := c.ListTools(ctx, "")
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(page.Tools) != 2 {
		t.Fatalf("first page = %d tools, want 2", len(page.Tools))
	}
	if page.NextCursor == "" {
		t.Fatal("expected a NextCursor on the first page")
	}
}

func TestListAllToolsFollowsPages(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	all, err := c.ListAllTools(ctx, 100)
	if err != nil {
		t.Fatalf("ListAllTools: %v", err)
	}
	// Test server registers 8 tools (echo, add, boom, slow, pad_1..pad_4).
	if len(all) != 8 {
		t.Fatalf("ListAllTools = %d tools, want 8", len(all))
	}
	names := map[string]bool{}
	for _, tl := range all {
		names[tl.Name] = true
	}
	for _, want := range []string{"echo", "add", "boom", "slow"} {
		if !names[want] {
			t.Errorf("missing expected tool %q", want)
		}
	}
}

func TestListAllToolsPageCap(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()
	// maxPages=1 with PageSize=2 and 8 tools must hit the cap and error.
	_, err := c.ListAllTools(ctx, 1)
	if err == nil {
		t.Fatal("expected a page-cap error with maxPages=1")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/client/ -run TestList -v`
Expected: FAIL — `ListTools`/`ListAllTools` not defined on `*stdioClient`.

- [ ] **Step 3: Write the implementation (append to `internal/client/stdio.go`)**

Add to `internal/client/stdio.go` (and add `"github.com/modelcontextprotocol/go-sdk/mcp"` is already imported):
```go
func (c *stdioClient) ListTools(ctx context.Context, cursor string) (ToolPage, error) {
	res, err := c.session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
	if err != nil {
		return ToolPage{}, apperror.Wrap(apperror.KindProtocol, err, "list tools")
	}
	page := ToolPage{NextCursor: res.NextCursor}
	for _, t := range res.Tools {
		page.Tools = append(page.Tools, toToolInfo(t))
	}
	return page, nil
}

// ListAllTools follows NextCursor to completion. It caps at maxPages and
// terminates with a protocol error if a cursor repeats (a misbehaving server
// that loops) rather than paginating forever.
func (c *stdioClient) ListAllTools(ctx context.Context, maxPages int) ([]ToolInfo, error) {
	var all []ToolInfo
	seen := map[string]bool{}
	cursor := ""
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, apperror.New(apperror.KindProtocol,
				"tools/list exceeded the page cap (%d pages); server may be paginating without end", maxPages)
		}
		p, err := c.ListTools(ctx, cursor)
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

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/client/ -run TestList -v`
Expected: PASS — pagination collects all 8 tools; `maxPages=1` errors.

- [ ] **Step 5: Commit**

```bash
git add internal/client/stdio.go internal/client/list_test.go
git commit -m "feat(client): paginated tool listing with repeated-cursor detection"
```

---

### Task 7: Tool invocation + result/error handling

**Files:**
- Modify: `internal/client/stdio.go` (add `CallTool`)
- Test: `internal/client/call_test.go`

**Interfaces:**
- Consumes: `session.CallTool`, `toToolResult`, `apperror`.
- Produces: `(*stdioClient) CallTool(ctx, name, args) (ToolResult, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/client/call_test.go`:
```go
package client

import (
	"context"
	"testing"
	"time"
)

func TestCallToolEchoText(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	res, err := c.CallTool(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("CallTool(echo): %v", err)
	}
	if res.IsError {
		t.Fatal("echo unexpectedly returned IsError")
	}
	if len(res.Content) != 1 || res.Content[0].Kind != KindText || res.Content[0].Text != "hello" {
		t.Fatalf("unexpected echo result: %+v", res.Content)
	}
}

func TestCallToolStructured(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	res, err := c.CallTool(ctx, "add", map[string]any{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("CallTool(add): %v", err)
	}
	if res.Structured == nil {
		t.Fatal("expected structured content from add")
	}
}

func TestCallToolIsError(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	res, err := c.CallTool(ctx, "boom", nil)
	if err != nil {
		t.Fatalf("boom should be a normal result, got Go error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError = true from boom")
	}
}

func TestCallToolUnknownIsGoError(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	_, err := c.CallTool(ctx, "does_not_exist", nil)
	if err == nil {
		t.Fatal("expected a Go error calling an unknown tool")
	}
}

func TestCallToolContextCancel(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.CallTool(cctx, "slow", map[string]any{"seconds": 5})
	if err == nil {
		t.Fatal("expected a cancellation error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("cancellation took too long: %v", time.Since(start))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/client/ -run TestCallTool -v`
Expected: FAIL — `CallTool` not defined on `*stdioClient`.

- [ ] **Step 3: Write the implementation (append to `internal/client/stdio.go`)**

```go
func (c *stdioClient) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return ToolResult{}, apperror.Wrap(apperror.KindProtocol, err, "call tool %q", name)
	}
	return toToolResult(res), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/client/ -run TestCallTool -v`
Expected: PASS — echo text, structured `add`, `boom` as `IsError`, unknown-tool as a Go error, and `slow` cancels promptly.

- [ ] **Step 5: Run the full package with the race detector**

Run: `go test -race ./internal/client/ ./internal/process/`
Expected: PASS with no data races (exercises the real subprocess lifecycle under `-race`).

- [ ] **Step 6: Commit**

```bash
git add internal/client/stdio.go internal/client/call_test.go
git commit -m "feat(client): tool invocation with result and error mapping"
```

---

## Phase 2A Acceptance

Phase 2A is complete when:

- `make check` passes (build, test, `-race`, vet, staticcheck) under Go 1.25.
- `internal/client` exposes only SDK-free types across its public surface (`Client`, `StdioSpec`, `ToolInfo`, `ToolResult`, `ServerInfo`, `ToolPage`, `DialStdio`); no `mcp.*`/`jsonrpc.*` type escapes the package.
- Against the in-repo test server: dialing initializes and reports capabilities; `ListAllTools` collects all pages and both safeguards fire (page cap; repeated cursor); `CallTool` returns text and structured results, surfaces `IsError` as data, returns a Go error for an unknown tool, and cancels promptly on context timeout.
- `TestKillGroupReapsGrandchild` proves the process group is swept (no orphaned grandchild) — the spec §8.3 guarantee the SDK does not provide.
- The module cross-compiles for `windows/amd64` (the `process_windows.go` split builds).

---

## Roadmap: Plan 2B (tools commands)

Written after 2A lands. Builds the user-facing CLI on top of `internal/client`:

- `internal/arguments` — `--json` / `--json-file` / repeated `--arg KEY=VALUE` (mutual exclusion; JSON-object enforcement; `--arg` JSON-or-string parsing with the documented numeric footguns).
- `internal/cli/flags.go` — server selection (`--server` vs. `--stdio` vs. `--url`, mutually exclusive) and the ephemeral-stdio `--` grammar (§4.3.1); build a `client.StdioSpec` from a named `config.ServerConfig` or ephemeral flags. `--url` returns an unsupported-transport error until Phase 3.
- `internal/output` — `human` and `json` rendering for tool lists, descriptions, and call results, preserving the full MCP result (§11.1); `--protocol-version` returns the unsupported-option error (§7).
- `internal/cli/tools.go` — `tools list`, `tools describe` (all-pages lookup, not-found → exit 7), `tools call` (list-lookup for not-found before calling; `IsError` → exit 9), wired with command-scoped timeouts/signals.
- End-to-end tests building the real binary against the test server.
