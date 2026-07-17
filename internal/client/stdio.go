package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jstevewhite/mcpctl/internal/apperror"
	"github.com/jstevewhite/mcpctl/internal/process"
)

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

// ListAllTools follows NextCursor to completion. It caps at maxPages and
// terminates with a protocol error if a cursor repeats (a misbehaving server
// that loops) rather than paginating forever.
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
		return nil, classifyErr(err, apperror.KindConnection, "connect to stdio server %q", spec.Command)
	}

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
}

// classifyErr maps an SDK call/connect error to an application error. Context
// cancellation and deadline are mapped to the interrupt/timeout kinds (so they
// reach exit codes 130/10); everything else uses defaultKind.
func classifyErr(err error, defaultKind apperror.Kind, format string, args ...any) error {
	switch {
	case errors.Is(err, context.Canceled):
		return apperror.Wrap(apperror.KindInterrupted, err, format, args...)
	case errors.Is(err, context.DeadlineExceeded):
		return apperror.Wrap(apperror.KindTimeout, err, format, args...)
	default:
		return apperror.Wrap(defaultKind, err, format, args...)
	}
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
