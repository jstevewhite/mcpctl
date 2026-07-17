# mcpctl Phase 3 — Streamable HTTP Transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Streamable HTTP as a second transport — static headers, env-sourced headers, bearer tokens, safe TLS/redirect/proxy behavior with cross-origin credential stripping, and HTTP-aware exit codes — reachable via `--url` (ephemeral) and `--server` (a `streamable-http` config entry), reusing the existing `tools list/describe/call` commands unchanged.

**Architecture:** The `Client` interface and every session method (`ListTools`/`ListAllTools`/`CallTool`/`ServerInfo`) are transport-agnostic — only dial and close differ. So we first extract a shared `mcpSession` (Task 1); `stdioClient` and the new `httpClient` embed it and add their own `Close`. HTTP support is a caller-built `*http.Client` assigned to `mcp.StreamableClientTransport.HTTPClient` (the SDK genuinely uses it — spike-proven): a custom RoundTripper injects the resolved headers **only for same-origin requests** (so credentials never follow a cross-origin redirect, spec §9) and records the raw HTTP status (so we can map 401/403→auth despite the SDK's opaque errors). Header/env/bearer resolution and secret redaction live in a new `internal/auth`. The command layer resolves a named/ephemeral target to either a stdio or HTTP spec and dispatches.

**Tech Stack:** Go 1.25, `github.com/modelcontextprotocol/go-sdk` v1.5.0, standard library `net/http`.

## Global Constraints

From `mcpctl-spec.md` §5, §9, §12, §15 and the Phase 3 spike (SDK v1.5.0).

- **SDK confinement:** MCP SDK types only in `internal/client`. `internal/auth` and `internal/cli` do not import the SDK (they may import `net/http`).
- **No CGo.** Go 1.25.
- **Verified HTTP transport API (spike — use exactly):**
  - `mcp.StreamableClientTransport{Endpoint string, HTTPClient *http.Client, MaxRetries int, DisableStandaloneSSE bool}` — a plain struct, no constructor. Endpoint is the URL string; `HTTPClient` is genuinely used for every request (RoundTripper + `CheckRedirect` honored). Set `DisableStandaloneSSE: true` (a one-shot CLI needs no server→client notification stream, and it keeps status tracking single-threaded).
  - Session methods are the same as stdio: `client.Connect(ctx, transport, nil)`, `session.InitializeResult()`, `session.ListTools`, `session.CallTool`, `session.Close()`.
  - Errors are opaque Go errors, NOT status-carrying. `errors.Is(err, mcp.ErrSessionMissing)` ⇔ HTTP 404. Transient 5xx/429 ⇒ `errors.As(err, &jsonrpc.Error{})` with `Code == -32005` (cannot tell which 5xx). 401 vs 403 have no sentinel. → We record the raw `resp.StatusCode` in our RoundTripper and classify from it.
  - `CheckRedirect` on the injected client fires on 3xx; ctx cancellation aborts an in-flight call promptly.
- **§9 HTTP requirements:** support http/https; normal TLS verification (no `--insecure` in v1); static + env headers; bearer auth; honor ctx cancellation/deadlines; use a **cloned** `http.Transport` (never mutate `http.DefaultTransport`); proxy from environment; TLS-handshake timeout; **no** short response-body timeout (the command context is the overall bound); idle-conn cleanup on close; `User-Agent: mcpctl/<version>`; **do not forward credentials to a different origin on redirect.**
- **§5.4 auth resolution:** `header_env` value is an env var name whose contents become the header value; missing referenced env var ⇒ auth/config error **before** connecting; bearer token read from its env var → `Authorization: Bearer <token>`; reject a config that sets `Authorization` via more than one mechanism (bearer + header/header_env). Never log tokens.
- **§12.2 redaction:** never log bearer tokens, `Authorization`/`Proxy-Authorization`/`Cookie`/`Set-Cookie`/`X-API-Key`, or names containing `token`/`secret`/`password`/`key` (case-insensitive).
- **Exit codes:** auth → 4; connection/transport → 5; protocol/init → 6; timeout → 10; interrupt → 130 (context errors mapped first, as in stdio).
- **`--url` transport:** absolute `http`/`https` URL. `--server` may now name a `streamable-http` config entry.
- **connect-timeout:** stays folded into `--timeout` (single command context; the session's lifetime is tied to that context). Update the flag help to drop the "arrives with HTTP" promise.

---

## File Structure

- Modify: `internal/client/stdio.go` — extract `mcpSession`; `stdioClient` embeds it.
- Modify: `internal/client/types.go` — add `HTTPSpec`.
- Create: `internal/auth/headers.go`, `internal/auth/redaction.go` (+ tests).
- Create: `internal/client/streamable_http.go` — the `*http.Client` builder, `authRoundTripper`, `statusRecorder`, `DialHTTP`, `httpClient`, HTTP error classifier (+ tests).
- Modify: `internal/cli/target.go` — resolve to a stdio OR http target; add HTTP flags handling.
- Modify: `internal/cli/tools.go` — `addServerFlags` gains `--header-env`/`--header-literal`/`--bearer-env`; `dial` dispatches stdio vs http.
- Modify: `internal/cli/root.go` — connect-timeout help text.

---

### Task 1: Extract a shared `mcpSession` (refactor; behavior-preserving)

**Files:**
- Modify: `internal/client/stdio.go`
- Modify: `internal/client/stdio_test.go` (cast `*stdioClient`; keep `c.cmd`)

**Interfaces:**
- Produces:
  - `type mcpSession struct { sess *mcp.ClientSession; info ServerInfo; wrapErr func(error, string) error }` with `ServerInfo()`, `ListTools`, `ListAllTools`, `CallTool`.
  - `type stdioClient struct { *mcpSession; cmd *exec.Cmd }` with its own `Close`.
  - `func stdioWrapErr(err error, op string) error`.

- [ ] **Step 1: Refactor stdio.go**

Replace the `stdioClient` type + its `ServerInfo`/`ListTools`/`ListAllTools`/`CallTool`/`Close` methods with a shared `mcpSession` plus a thin `stdioClient`. The session methods keep their existing bodies, but wrap errors via `s.wrapErr(err, op)` instead of an inline `classifyErr(...)`:
```go
// mcpSession holds a live MCP session and the transport-agnostic tool
// operations. stdioClient and httpClient embed it and add their own Close.
type mcpSession struct {
	sess    *mcp.ClientSession
	info    ServerInfo
	wrapErr func(err error, op string) error
}

func (s *mcpSession) ServerInfo() ServerInfo { return s.info }

func (s *mcpSession) ListTools(ctx context.Context, cursor string) (ToolPage, error) {
	res, err := s.sess.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
	if err != nil {
		return ToolPage{}, s.wrapErr(err, "list tools")
	}
	page := ToolPage{NextCursor: res.NextCursor}
	for _, t := range res.Tools {
		page.Tools = append(page.Tools, toToolInfo(t))
	}
	return page, nil
}

func (s *mcpSession) ListAllTools(ctx context.Context, maxPages int) ([]ToolInfo, error) {
	return collectAllTools(maxPages, func(cursor string) (ToolPage, error) {
		return s.ListTools(ctx, cursor)
	})
}

func (s *mcpSession) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	res, err := s.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return ToolResult{}, s.wrapErr(err, fmt.Sprintf("call tool %q", name))
	}
	return toToolResult(res), nil
}

// stdioClient is a session backed by a spawned child process.
type stdioClient struct {
	*mcpSession
	cmd *exec.Cmd
}

// Close gracefully closes the session (the SDK terminates the direct child),
// then sweeps the process group for orphaned descendants. It may return the
// child's signal/exit error; callers must not treat that as command failure.
func (c *stdioClient) Close() error {
	err := c.sess.Close()
	process.KillGroup(c.cmd)
	return err
}

// stdioWrapErr classifies stdio session errors: context cancel/timeout map to
// interrupt/timeout, everything else to a protocol error.
func stdioWrapErr(err error, op string) error {
	return classifyErr(err, apperror.KindProtocol, "%s", op)
}
```
Update `DialStdio` to construct the new shape (keep the connect + capability logic; only the return value changes):
```go
	init := session.InitializeResult()
	return &stdioClient{
		mcpSession: &mcpSession{
			sess: session,
			info: ServerInfo{
				Name:            init.ServerInfo.Name,
				Version:         init.ServerInfo.Version,
				ProtocolVersion: init.ProtocolVersion,
				SupportsTools:   init.Capabilities.Tools != nil,
			},
			wrapErr: stdioWrapErr,
		},
		cmd: cmd,
	}, nil
```
Delete the old standalone `ListTools`/`ListAllTools`/`CallTool`/`ServerInfo`/`Close` methods on `stdioClient` (they now live on `mcpSession` / the new `Close`). `classifyErr` and `collectAllTools` are unchanged. Ensure `"fmt"` is imported.

- [ ] **Step 2: Update the stdio test's type + field access**

In `internal/client/stdio_test.go`, `dialTestServer` currently returns `c.(*stdioClient)`. That still works. `TestCloseTerminatesChild` uses `c.cmd.Process.Pid` — `cmd` is still a field on `stdioClient`, so no change is needed. If any test referenced a now-removed method directly, adjust it; otherwise leave the tests as-is.

- [ ] **Step 3: Run the client tests (behavior must be unchanged)**

Run: `go test ./internal/client/ -v && go test ./... && go vet ./... && gofmt -l internal/client/`
Expected: PASS — all existing 2A/2B client tests still pass; the refactor is behavior-preserving.

- [ ] **Step 4: Commit**

```bash
git add internal/client/stdio.go internal/client/stdio_test.go
git commit -m "refactor(client): extract shared mcpSession; stdioClient embeds it"
```

---

### Task 2: Header/bearer resolution + redaction (`internal/auth`)

**Files:**
- Create: `internal/auth/headers.go`, `internal/auth/redaction.go`
- Test: `internal/auth/headers_test.go`, `internal/auth/redaction_test.go`

**Interfaces:**
- Consumes: `apperror`, `net/http`, `os` (via an injected lookup for testability).
- Produces:
  - `type auth.Spec struct { Headers map[string]string; HeaderEnv map[string]string; BearerEnv string }`
  - `func auth.Resolve(s Spec, lookup func(string) (string, bool)) (http.Header, error)` — resolves literals + env + bearer into a header set; errors on missing env var (KindAuth) and on multiple Authorization mechanisms (KindConfig).
  - `func auth.IsSensitive(headerName string) bool` — the §12.2 redaction predicate.
  - `func auth.RedactValue() string` — the placeholder `"<redacted>"`.

- [ ] **Step 1: Write the failing tests**

Create `internal/auth/headers_test.go`:
```go
package auth

import "testing"

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestResolveStaticAndEnvAndBearer(t *testing.T) {
	h, err := Resolve(Spec{
		Headers:   map[string]string{"Accept-Language": "en-US"},
		HeaderEnv: map[string]string{"X-Api-Key": "MCP_KEY"},
		BearerEnv: "MCP_TOKEN",
	}, env(map[string]string{"MCP_KEY": "secret", "MCP_TOKEN": "tok"}))
	if err != nil {
		t.Fatal(err)
	}
	if h.Get("Accept-Language") != "en-US" {
		t.Errorf("static header wrong: %q", h.Get("Accept-Language"))
	}
	if h.Get("X-Api-Key") != "secret" {
		t.Errorf("env header wrong: %q", h.Get("X-Api-Key"))
	}
	if h.Get("Authorization") != "Bearer tok" {
		t.Errorf("bearer wrong: %q", h.Get("Authorization"))
	}
}

func TestResolveMissingEnvErrors(t *testing.T) {
	if _, err := Resolve(Spec{HeaderEnv: map[string]string{"X-Api-Key": "NOPE"}}, env(nil)); err == nil {
		t.Fatal("missing env var must error")
	}
	if _, err := Resolve(Spec{BearerEnv: "NOPE"}, env(nil)); err == nil {
		t.Fatal("missing bearer env var must error")
	}
}

func TestResolveConflictingAuthErrors(t *testing.T) {
	if _, err := Resolve(Spec{
		Headers:   map[string]string{"Authorization": "Bearer x"},
		BearerEnv: "MCP_TOKEN",
	}, env(map[string]string{"MCP_TOKEN": "tok"})); err == nil {
		t.Fatal("bearer + Authorization header must conflict")
	}
	if _, err := Resolve(Spec{
		Headers:   map[string]string{"authorization": "a"},
		HeaderEnv: map[string]string{"Authorization": "MCP_AUTH"},
	}, env(map[string]string{"MCP_AUTH": "b"})); err == nil {
		t.Fatal("Authorization via header + header_env must conflict")
	}
}
```
Create `internal/auth/redaction_test.go`:
```go
package auth

import "testing"

func TestIsSensitive(t *testing.T) {
	for _, s := range []string{"Authorization", "authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "X-API-Key", "X-Auth-Token", "my-secret", "PASSWORD", "api_key"} {
		if !IsSensitive(s) {
			t.Errorf("%q should be sensitive", s)
		}
	}
	for _, s := range []string{"Accept-Language", "Content-Type", "User-Agent"} {
		if IsSensitive(s) {
			t.Errorf("%q should not be sensitive", s)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/auth/ -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Write the implementation**

Create `internal/auth/redaction.go`:
```go
// Package auth resolves HTTP authentication headers and identifies secrets to
// redact from logs. It does not import the MCP SDK.
package auth

import "strings"

var exactSensitive = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
}

var sensitiveSubstrings = []string{"token", "secret", "password", "key"}

// IsSensitive reports whether a header (or variable) name likely carries a
// credential and must be redacted from logs (spec §12.2).
func IsSensitive(name string) bool {
	l := strings.ToLower(name)
	if exactSensitive[l] {
		return true
	}
	for _, s := range sensitiveSubstrings {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// RedactValue is the placeholder used in place of a secret value in logs/output.
func RedactValue() string { return "<redacted>" }
```
Create `internal/auth/headers.go`:
```go
package auth

import (
	"net/http"
	"strings"

	"mcpctl/internal/apperror"
)

// Spec describes the auth inputs for an HTTP server, before env resolution.
type Spec struct {
	Headers   map[string]string // literal name -> value
	HeaderEnv map[string]string // header name -> env var name
	BearerEnv string            // env var name holding a bearer token ("" = none)
}

// Resolve turns the spec into a concrete header set, reading env values via
// lookup. Missing referenced env vars are an auth error; configuring
// Authorization through more than one mechanism is a config error (§5.4).
func Resolve(s Spec, lookup func(string) (string, bool)) (http.Header, error) {
	authMechanisms := 0
	h := http.Header{}

	for name, val := range s.Headers {
		if strings.EqualFold(name, "Authorization") {
			authMechanisms++
		}
		h.Set(name, val)
	}
	for name, envVar := range s.HeaderEnv {
		val, ok := lookup(envVar)
		if !ok {
			return nil, apperror.New(apperror.KindAuth,
				"environment variable %q (for header %q) is not set", envVar, name)
		}
		if strings.EqualFold(name, "Authorization") {
			authMechanisms++
		}
		h.Set(name, val)
	}
	if s.BearerEnv != "" {
		tok, ok := lookup(s.BearerEnv)
		if !ok {
			return nil, apperror.New(apperror.KindAuth,
				"bearer token environment variable %q is not set", s.BearerEnv)
		}
		authMechanisms++
		h.Set("Authorization", "Bearer "+tok)
	}

	if authMechanisms > 1 {
		return nil, apperror.Config(
			"multiple Authorization mechanisms configured; use exactly one of a bearer token or an Authorization header")
	}
	return h, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/ -v && go vet ./... && gofmt -l internal/auth/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat(auth): resolve static/env/bearer headers; redaction predicate"
```

---

### Task 3: HTTP client builder + auth RoundTripper + status recorder (`internal/client`)

**Files:**
- Create: `internal/client/streamable_http.go`
- Modify: `internal/client/types.go` (add `HTTPSpec`)
- Test: `internal/client/streamable_http_test.go`

**Interfaces:**
- Consumes: `buildinfo.Version`, `net/http`, `net/url`, `sync/atomic`.
- Produces:
  - `type client.HTTPSpec struct { URL string; Header http.Header }` (in types.go).
  - `func buildHTTPClient(endpoint *url.URL, header http.Header) (*http.Client, *statusRecorder, error)` — cloned transport, origin-gated `authRoundTripper` (adds header for same-origin, strips for cross-origin), status recording, `User-Agent`.
  - `type statusRecorder` with `last() int`.

- [ ] **Step 1: Add HTTPSpec to types.go**

Append to `internal/client/types.go`:
```go
// HTTPSpec describes a Streamable HTTP server. Header holds the already-resolved
// request headers (env/bearer resolved by the caller); the URL is absolute.
type HTTPSpec struct {
	URL    string
	Header http.Header
}
```
Add `"net/http"` to the imports of types.go.

- [ ] **Step 2: Write the failing test**

Create `internal/client/streamable_http_test.go`:
```go
package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAuthRoundTripperSameOriginAddsHeaders(t *testing.T) {
	var gotAuth, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer tok")
	hdr.Set("X-Api-Key", "secret")
	c, rec, err := buildHTTPClient(mustURL(t, srv.URL), hdr)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer tok" || gotKey != "secret" {
		t.Fatalf("headers not injected: auth=%q key=%q", gotAuth, gotKey)
	}
	if rec.last() != 200 {
		t.Fatalf("recorder = %d, want 200", rec.last())
	}
}

func TestAuthRoundTripperStripsCrossOrigin(t *testing.T) {
	var otherAuth, otherKey string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherAuth = r.Header.Get("Authorization")
		otherKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(200)
	}))
	defer other.Close()

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer tok")
	hdr.Set("X-Api-Key", "secret")
	// Endpoint is a DIFFERENT origin than `other`.
	c, _, err := buildHTTPClient(mustURL(t, "http://127.0.0.1:1"), hdr)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(other.URL) // request to a non-endpoint origin
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if otherAuth != "" || otherKey != "" {
		t.Fatalf("credentials leaked cross-origin: auth=%q key=%q", otherAuth, otherKey)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/client/ -run TestAuthRoundTripper -v`
Expected: FAIL — `undefined: buildHTTPClient`.

- [ ] **Step 4: Write the implementation**

Create `internal/client/streamable_http.go`:
```go
package client

import (
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"mcpctl/internal/buildinfo"
)

// statusRecorder holds the most recent HTTP response status seen by the
// transport, so the error classifier can distinguish 401/403 (auth) from other
// failures despite the SDK's opaque errors.
type statusRecorder struct{ code atomic.Int64 }

func (r *statusRecorder) record(c int) { r.code.Store(int64(c)) }
func (r *statusRecorder) last() int    { return int(r.code.Load()) }

// authRoundTripper injects the resolved request headers for same-origin
// requests only (credentials never follow a cross-origin redirect, §9), sets a
// descriptive User-Agent, and records response statuses.
type authRoundTripper struct {
	base   http.RoundTripper
	origin string // scheme://host[:port] of the configured endpoint
	header http.Header
	rec    *statusRecorder
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	if originOf(r.URL) == t.origin {
		for k, vs := range t.header {
			r.Header[k] = append([]string(nil), vs...)
		}
	} else {
		// Cross-origin (e.g. after a redirect): ensure none of our configured
		// headers ride along, even if net/http copied one.
		for k := range t.header {
			r.Header.Del(k)
		}
	}
	if r.Header.Get("User-Agent") == "" {
		r.Header.Set("User-Agent", "mcpctl/"+buildinfo.Version)
	}
	resp, err := t.base.RoundTrip(r)
	if resp != nil {
		t.rec.record(resp.StatusCode)
	}
	return resp, err
}

// originOf returns scheme://host[:port], the identity used for same-origin checks.
func originOf(u *url.URL) string {
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// buildHTTPClient constructs the *http.Client for a Streamable HTTP endpoint: a
// cloned transport (never mutating http.DefaultTransport), proxy-from-env, a
// TLS-handshake timeout, and NO response-body timeout (the command context is
// the overall bound). Credentials are stripped on cross-origin redirects.
func buildHTTPClient(endpoint *url.URL, header http.Header) (*http.Client, *statusRecorder, error) {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = http.ProxyFromEnvironment
	base.TLSHandshakeTimeout = 15 * time.Second
	// Deliberately no ResponseHeaderTimeout: long-running tool calls hold the
	// response open; the command context bounds the whole operation.

	rec := &statusRecorder{}
	rt := &authRoundTripper{
		base:   base,
		origin: originOf(endpoint),
		header: header,
		rec:    rec,
	}
	c := &http.Client{
		Transport: rt,
		// Belt-and-suspenders with the RoundTripper's origin gate: refuse to
		// carry credentials across origins. net/http already strips
		// Authorization/Cookie cross-domain; this also handles custom headers.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	return c, rec, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/client/ -run TestAuthRoundTripper -v && go vet ./... && gofmt -l internal/client/`
Expected: PASS — same-origin injects headers; cross-origin strips them; status recorded.

- [ ] **Step 6: Commit**

```bash
git add internal/client/streamable_http.go internal/client/types.go internal/client/streamable_http_test.go
git commit -m "feat(client): HTTP client builder with origin-gated header injection and status recording"
```

---

### Task 4: `DialHTTP` + HTTP client + error classification + basic integration test

**Files:**
- Modify: `internal/client/streamable_http.go` (add `DialHTTP`, `httpClient`, `httpWrapErr`)
- Test: `internal/client/http_integration_test.go`

**Interfaces:**
- Consumes: the SDK, `buildHTTPClient`, `clientInfo`, `apperror`, `mcp.ErrSessionMissing`.
- Produces:
  - `func DialHTTP(ctx context.Context, spec HTTPSpec) (Client, error)`
  - `type httpClient struct { *mcpSession; httpc *http.Client }` with `Close`.
  - `func httpWrapErr(rec *statusRecorder) func(err error, op string) error`.

- [ ] **Step 1: Write the failing integration test (uses an httptest MCP server)**

Create `internal/client/http_integration_test.go`:
```go
package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newHTTPTestServer returns an httptest.Server running an MCP server with an
// `echo` tool, plus a wrap hook the caller can use to inject failures/headers.
func newHTTPTestServer(t *testing.T, wrap func(http.Handler) http.Handler) *httptest.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "http-test-server", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo"},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			Message string `json:"message"`
		}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Message}}}, nil, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	var h http.Handler = handler
	if wrap != nil {
		h = wrap(h)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPDialListCall(t *testing.T) {
	srv := newHTTPTestServer(t, nil)
	ctx := context.Background()
	c, err := DialHTTP(ctx, HTTPSpec{URL: srv.URL, Header: http.Header{}})
	if err != nil {
		t.Fatalf("DialHTTP: %v", err)
	}
	defer c.Close()

	if !c.ServerInfo().SupportsTools {
		t.Fatal("expected SupportsTools")
	}
	tools, err := c.ListAllTools(ctx, 1000)
	if err != nil {
		t.Fatalf("ListAllTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	res, err := c.CallTool(ctx, "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "hi" {
		t.Fatalf("unexpected result: %+v", res.Content)
	}
}
```
NOTE for the implementer: confirm the exact signature of `mcp.NewStreamableHTTPHandler` via `go doc github.com/modelcontextprotocol/go-sdk/mcp.NewStreamableHTTPHandler` and adjust the `getServer`/opts arguments if they differ from the snippet; the spike confirmed this constructor exists and returns an `http.Handler`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/client/ -run TestHTTPDialListCall -v`
Expected: FAIL — `undefined: DialHTTP`.

- [ ] **Step 3: Write the implementation (append to streamable_http.go)**

Add the imports `context`, `errors`, `fmt`, `github.com/modelcontextprotocol/go-sdk/mcp`, `mcpctl/internal/apperror`, and:
```go
// DialHTTP connects to a Streamable HTTP MCP server and returns a live Client.
func DialHTTP(ctx context.Context, spec HTTPSpec) (Client, error) {
	endpoint, err := url.Parse(spec.URL)
	if err != nil || !endpoint.IsAbs() || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, apperror.New(apperror.KindConfig, "invalid server URL %q (want an absolute http/https URL)", spec.URL)
	}
	httpc, rec, err := buildHTTPClient(endpoint, spec.Header)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindConnection, err, "build http client")
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             spec.URL,
		HTTPClient:           httpc,
		DisableStandaloneSSE: true,
		MaxRetries:           3,
	}
	wrap := httpWrapErr(rec)
	cl := mcp.NewClient(clientInfo(), nil)
	session, err := cl.Connect(ctx, transport, nil)
	if err != nil {
		httpc.CloseIdleConnections()
		return nil, wrap(err, "connect to "+spec.URL)
	}
	init := session.InitializeResult()
	return &httpClient{
		mcpSession: &mcpSession{
			sess: session,
			info: ServerInfo{
				Name:            init.ServerInfo.Name,
				Version:         init.ServerInfo.Version,
				ProtocolVersion: init.ProtocolVersion,
				SupportsTools:   init.Capabilities.Tools != nil,
			},
			wrapErr: wrap,
		},
		httpc: httpc,
	}, nil
}

// httpClient is a session backed by an HTTP transport.
type httpClient struct {
	*mcpSession
	httpc *http.Client
}

func (c *httpClient) Close() error {
	err := c.sess.Close()
	c.httpc.CloseIdleConnections()
	return err
}

// httpWrapErr classifies HTTP session errors. Context cancel/timeout map first;
// then the recorded HTTP status distinguishes auth (401/403 → 4) from other
// transport failures (→ 5); a 404 session-missing is a transport error; the
// rest are protocol errors.
func httpWrapErr(rec *statusRecorder) func(err error, op string) error {
	return func(err error, op string) error {
		switch {
		case errors.Is(err, context.Canceled):
			return apperror.Wrap(apperror.KindInterrupted, err, "%s", op)
		case errors.Is(err, context.DeadlineExceeded):
			return apperror.Wrap(apperror.KindTimeout, err, "%s", op)
		}
		switch code := rec.last(); {
		case code == http.StatusUnauthorized || code == http.StatusForbidden:
			return apperror.Wrap(apperror.KindAuth, err, "%s", op)
		case code >= 400:
			return apperror.Wrap(apperror.KindConnection, err, "%s", op)
		}
		if errors.Is(err, mcp.ErrSessionMissing) {
			return apperror.Wrap(apperror.KindConnection, err, "%s", op)
		}
		return apperror.Wrap(apperror.KindProtocol, err, "%s", op)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/client/ -run TestHTTP -v && go test ./... && go vet ./... && gofmt -l internal/client/`
Expected: PASS — dial/list/call over real HTTP works.

- [ ] **Step 5: Commit**

```bash
git add internal/client/streamable_http.go internal/client/http_integration_test.go
git commit -m "feat(client): DialHTTP over Streamable HTTP with status-based error classification"
```

---

### Task 5: HTTP integration matrix — auth, errors, headers, redirects (§16.3)

**Files:**
- Modify: `internal/client/http_integration_test.go` (add the matrix)

**Interfaces:**
- Consumes: `newHTTPTestServer`, `DialHTTP`, `apperror.ExitCode`.

- [ ] **Step 1: Add the integration tests**

Append to `internal/client/http_integration_test.go`:
```go
// statusWrap returns a wrap hook that responds to every request with the given
// status (before the MCP handler runs).
func statusWrap(status int) func(http.Handler) http.Handler {
	return func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})
	}
}

func TestHTTP401IsAuthError(t *testing.T) {
	srv := newHTTPTestServer(t, statusWrap(http.StatusUnauthorized))
	_, err := DialHTTP(context.Background(), HTTPSpec{URL: srv.URL, Header: http.Header{}})
	if err == nil {
		t.Fatal("expected an auth error")
	}
	if code := apperror.ExitCode(err); code != 4 {
		t.Fatalf("exit code = %d, want 4 (auth)", code)
	}
}

func TestHTTP403IsAuthError(t *testing.T) {
	srv := newHTTPTestServer(t, statusWrap(http.StatusForbidden))
	_, err := DialHTTP(context.Background(), HTTPSpec{URL: srv.URL, Header: http.Header{}})
	if code := apperror.ExitCode(err); code != 4 {
		t.Fatalf("exit code = %d, want 4 (auth); err=%v", code, err)
	}
}

func TestHTTP500IsConnectionError(t *testing.T) {
	srv := newHTTPTestServer(t, statusWrap(http.StatusInternalServerError))
	_, err := DialHTTP(context.Background(), HTTPSpec{URL: srv.URL, Header: http.Header{}})
	if code := apperror.ExitCode(err); code != 5 {
		t.Fatalf("exit code = %d, want 5 (connection); err=%v", code, err)
	}
}

func TestHTTPHeadersReachServer(t *testing.T) {
	var gotAuth, gotLang string
	srv := newHTTPTestServer(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if gotAuth == "" {
				gotAuth = r.Header.Get("Authorization")
				gotLang = r.Header.Get("Accept-Language")
			}
			next.ServeHTTP(w, r)
		})
	})
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer tok")
	hdr.Set("Accept-Language", "en-US")
	c, err := DialHTTP(context.Background(), HTTPSpec{URL: srv.URL, Header: hdr})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if gotAuth != "Bearer tok" || gotLang != "en-US" {
		t.Fatalf("headers not received: auth=%q lang=%q", gotAuth, gotLang)
	}
}

func TestHTTPRedirectDoesNotForwardCredentials(t *testing.T) {
	// Target server records whether any credential header arrived.
	var leakedAuth, leakedKey string
	target := newHTTPTestServer(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if leakedAuth == "" && leakedKey == "" {
				leakedAuth = r.Header.Get("Authorization")
				leakedKey = r.Header.Get("X-Api-Key")
			}
			next.ServeHTTP(w, r)
		})
	})
	// Redirector server (different origin) 307-redirects everything to target.
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redir.Close)

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer tok")
	hdr.Set("X-Api-Key", "secret")
	c, err := DialHTTP(context.Background(), HTTPSpec{URL: redir.URL, Header: hdr})
	if err != nil {
		t.Fatalf("DialHTTP through redirect: %v", err)
	}
	c.Close()
	if leakedAuth != "" || leakedKey != "" {
		t.Fatalf("credentials leaked across redirect: auth=%q key=%q", leakedAuth, leakedKey)
	}
}
```

- [ ] **Step 2: Run the matrix + race**

Run:
```bash
go test ./internal/client/ -run TestHTTP -v
go test -race ./internal/client/
go test ./... && go vet ./... && gofmt -l internal/client/
```
Expected: PASS — 401/403→exit 4, 500→exit 5, headers arrive, and no credential leaks across the cross-origin redirect. If the redirect test fails (credentials leaked), the origin gate in `authRoundTripper` is wrong — do not weaken the test.

- [ ] **Step 3: Commit**

```bash
git add internal/client/http_integration_test.go
git commit -m "test(client): HTTP auth/error/header/redirect integration matrix"
```

---

### Task 6: CLI wiring — `--url`, ephemeral HTTP flags, config http, dial dispatch

**Files:**
- Modify: `internal/cli/target.go`, `internal/cli/tools.go`, `internal/cli/root.go`
- Test: `internal/cli/target_test.go` (extend), `internal/cli/e2e_test.go` (add an HTTP e2e)

**Interfaces:**
- Consumes: `auth.Resolve`, `client.DialStdio`/`client.DialHTTP`, `config.ServerConfig`.
- Produces: `resolveTarget` now returns a `Target` (stdio or http); `dial` dispatches.

- [ ] **Step 1: Introduce a Target and HTTP flags in target.go**

In `internal/cli/target.go`, add the HTTP flag fields to `ServerFlags` and change `resolveTarget` to return a `Target`:
```go
type ServerFlags struct {
	Server string
	URL    string
	Stdio  bool
	// HTTP auth (ephemeral or applied to a --url):
	HeaderEnv     []string // NAME=ENVVAR
	HeaderLiteral []string // NAME=VALUE
	BearerEnv     string
}

// Target is a resolved connection target: exactly one of Stdio / HTTP is set.
type Target struct {
	Stdio *client.StdioSpec
	HTTP  *client.HTTPSpec
}
```
Rewrite `resolveTarget` to build a `Target`. For `--url` and for a `--server` naming a `streamable-http` config entry, resolve auth via `internal/auth` and `os.LookupEnv`, and build a `client.HTTPSpec`. Return `(Target, toolArgs []string, error)`:
```go
func resolveTarget(sf ServerFlags, toolSide, afterDash []string, hasDash bool, configPath string) (Target, []string, error) {
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
		return Target{}, nil, apperror.Usage("exactly one of --server, --stdio, or --url is required")
	}

	switch {
	case sf.Stdio:
		if !hasDash || len(afterDash) == 0 {
			return Target{}, nil, apperror.Usage("--stdio requires a server command after `--`, e.g. --stdio -- npx -y server")
		}
		return Target{Stdio: &client.StdioSpec{Command: afterDash[0], Args: afterDash[1:]}}, toolSide, nil

	case sf.URL != "":
		spec, err := httpSpecFromFlags(sf)
		if err != nil {
			return Target{}, nil, err
		}
		return Target{HTTP: spec}, toolSide, nil

	default: // --server
		return targetFromConfig(sf.Server, configPath, toolSide)
	}
}

func httpSpecFromFlags(sf ServerFlags) (*client.HTTPSpec, error) {
	as, err := authSpecFromFlags(sf.HeaderEnv, sf.HeaderLiteral, sf.BearerEnv)
	if err != nil {
		return nil, err
	}
	hdr, err := auth.Resolve(as, os.LookupEnv)
	if err != nil {
		return nil, err
	}
	return &client.HTTPSpec{URL: sf.URL, Header: hdr}, nil
}

// authSpecFromFlags parses NAME=VALUE / NAME=ENVVAR flag pairs into an auth.Spec.
func authSpecFromFlags(headerEnv, headerLiteral []string, bearerEnv string) (auth.Spec, error) {
	as := auth.Spec{Headers: map[string]string{}, HeaderEnv: map[string]string{}, BearerEnv: bearerEnv}
	for _, kv := range headerLiteral {
		name, val, ok := splitPair(kv)
		if !ok {
			return auth.Spec{}, apperror.Usage("invalid --header-literal %q: expected NAME=VALUE", kv)
		}
		as.Headers[name] = val
	}
	for _, kv := range headerEnv {
		name, envVar, ok := splitPair(kv)
		if !ok {
			return auth.Spec{}, apperror.Usage("invalid --header-env %q: expected NAME=ENVVAR", kv)
		}
		as.HeaderEnv[name] = envVar
	}
	return as, nil
}

func splitPair(kv string) (name, val string, ok bool) {
	i := strings.IndexByte(kv, '=')
	if i <= 0 {
		return "", "", false
	}
	return kv[:i], kv[i+1:], true
}
```
Add a config path that handles both transports:
```go
func targetFromConfig(name, configPath string, toolSide []string) (Target, []string, error) {
	cfg, err := config.LoadResolved(configPath)
	if err != nil {
		return Target{}, nil, err
	}
	sc, ok := cfg.Servers[name]
	if !ok {
		return Target{}, nil, apperror.Config("no server named %q in configuration", name)
	}
	switch sc.Transport {
	case config.TransportStdio:
		return Target{Stdio: &client.StdioSpec{Command: sc.Command, Args: sc.Args, CWD: sc.CWD, Env: sc.Env}}, toolSide, nil
	case config.TransportHTTP:
		as := auth.Spec{Headers: sc.Headers, HeaderEnv: sc.HeaderEnv}
		if sc.BearerToken != nil {
			as.BearerEnv = sc.BearerToken.Env
		}
		hdr, err := auth.Resolve(as, os.LookupEnv)
		if err != nil {
			return Target{}, nil, err
		}
		return Target{HTTP: &client.HTTPSpec{URL: sc.URL, Header: hdr}}, toolSide, nil
	default:
		return Target{}, nil, apperror.Config("server %q has unknown transport %q", name, sc.Transport)
	}
}
```
Update imports: add `"os"`, `"strings"`, `"mcpctl/internal/auth"`. Delete the old `specFromConfig` (replaced by `targetFromConfig`).

- [ ] **Step 2: Dispatch in dial + add the HTTP flags**

In `internal/cli/tools.go`, update `dial` to resolve a `Target` and dispatch, and extend `addServerFlags`:
```go
func addServerFlags(cmd *cobra.Command, sf *ServerFlags) {
	cmd.Flags().StringVar(&sf.Server, "server", "", "named server from configuration")
	cmd.Flags().StringVar(&sf.URL, "url", "", "ephemeral Streamable HTTP URL")
	cmd.Flags().BoolVar(&sf.Stdio, "stdio", false, "ephemeral stdio server (command follows `--`)")
	cmd.Flags().StringArrayVar(&sf.HeaderEnv, "header-env", nil, "HTTP header from an env var: NAME=ENVVAR (repeatable)")
	cmd.Flags().StringArrayVar(&sf.HeaderLiteral, "header-literal", nil, "HTTP header literal (writes a secret to your shell history): NAME=VALUE")
	cmd.Flags().StringVar(&sf.BearerEnv, "bearer-env", "", "bearer token from an env var (sets Authorization: Bearer)")
}

func dial(ctx context.Context, cmd *cobra.Command, g *GlobalFlags, sf ServerFlags, args []string) (client.Client, []string, error) {
	dash := cmd.ArgsLenAtDash()
	var toolSide, afterDash []string
	hasDash := dash >= 0
	if hasDash {
		toolSide, afterDash = args[:dash], args[dash:]
	} else {
		toolSide = args
	}
	target, toolArgs, err := resolveTarget(sf, toolSide, afterDash, hasDash, g.Config)
	if err != nil {
		return nil, nil, err
	}
	var c client.Client
	switch {
	case target.Stdio != nil:
		c, err = client.DialStdio(ctx, *target.Stdio)
	case target.HTTP != nil:
		c, err = client.DialHTTP(ctx, *target.HTTP)
	}
	if err != nil {
		return nil, nil, err
	}
	if !c.ServerInfo().SupportsTools {
		c.Close()
		return nil, nil, apperror.New(apperror.KindProtocol, "server does not support tools")
	}
	return c, toolArgs, nil
}
```

- [ ] **Step 3: Honest connect-timeout help (root.go)**

In `internal/cli/root.go`, change the `--connect-timeout` flag help to drop the HTTP promise:
```go
	f.DurationVar(&g.ConnectTimeout, "connect-timeout", 15*time.Second, "connection/initialization timeout (currently applied as part of --timeout)")
```

- [ ] **Step 4: Update target tests for the Target return + add HTTP cases**

In `internal/cli/target_test.go`, update the existing tests to the `Target` return shape (e.g. `target.Stdio.Command`), and add:
```go
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
```
Adjust `TestResolveEphemeralStdio`/`TestResolveStdioRequiresServerArgv`/`TestResolveMutualExclusion` to read `target.Stdio` and to accept the new `Target` return.

- [ ] **Step 5: Add an HTTP e2e test**

In `internal/cli/e2e_test.go`, add a test that runs the real binary against an in-process `httptest` MCP server. Add the needed imports (`context`, `net/http`, `net/http/httptest`, `github.com/modelcontextprotocol/go-sdk/mcp`) — NOTE: this makes `e2e_test.go` import the SDK, which is acceptable **in a test file** (the confinement rule is about non-test package code); if you prefer to keep the SDK out of `internal/cli` entirely, place this HTTP e2e in a separate `internal/client` or top-level test instead. Simplest: reuse the pattern from `internal/client/http_integration_test.go`'s `newHTTPTestServer` by copying a minimal server builder here.
```go
func TestE2EToolsListHTTP(t *testing.T) {
	srv := newCLIHTTPServer(t) // minimal echo server over Streamable HTTP
	mcpctl, _ := buildBinaries(t)
	stdout, _, code := run(t, mcpctl, "tools", "list", "--url", srv.URL)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, stdout)
	}
	if !strings.Contains(stdout, "echo") {
		t.Fatalf("expected echo tool over HTTP:\n%s", stdout)
	}
}
```
Provide `newCLIHTTPServer` in the test file mirroring `newHTTPTestServer`. (If SDK-in-cli-tests is undesirable, skip this e2e and rely on the `internal/client` HTTP integration tests plus a `resolveTarget` unit test — note the choice in your report.)

- [ ] **Step 6: Run the full gate**

Run:
```bash
go test ./internal/cli/ -v
go test ./... && go test -race ./internal/client/ ./internal/cli/ && go vet ./... && gofmt -l ./internal/
```
Expected: PASS — `--url` resolves + connects; missing env errors before connecting; the three selectors stay mutually exclusive; stdio still works.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/ 
git commit -m "feat(cli): --url and ephemeral HTTP auth flags; dial dispatches stdio/http"
```

---

## Phase 3 Acceptance

Phase 3 is complete when:

- `make check` passes (build, test, `-race`, vet, staticcheck) under Go 1.25.
- `mcpctl tools list --url http://127.0.0.1:<port>/mcp` initializes and lists tools; `tools call` works over HTTP; a `streamable-http` config entry works via `--server`.
- Auth: `--bearer-env`/`--header-env` send the right headers; a missing referenced env var errors (exit 4) **before** connecting; a config setting Authorization two ways errors (exit 3).
- Errors map: HTTP 401/403 → exit 4; 5xx/transport → exit 5.
- **§9 redirect protection:** credentials are not forwarded to a different origin on redirect (the integration test proves both a bearer and a custom header are stripped).
- SDK confined to `internal/client` (grep: no `go-sdk` import in `internal/auth`, `internal/cli` non-test code, `internal/output`).

---

## After Phase 3

- Whole-branch review of `v3-streamable-http` on the strongest model; triage/fix; merge to `main`.
- Phase 4 (UX & validation): `server add/list/show/remove` (writes http configs), JSON Lines + YAML output, JSON-Schema argument validation, pagination safeguards surfaced, terminal control-char sanitization, secret redaction wired into logging via `auth.IsSensitive`.
- Phase 5: release (README, completions, GoReleaser).
- On GitHub-repo creation: migrate the module path from `mcpctl` to `github.com/<owner>/mcpctl`.
