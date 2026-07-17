package client

import (
	"context"
	"errors"
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
		return nil, classifyErr(err, apperror.KindConnection, "connect to stdio server %q", spec.Command)
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

func (c *stdioClient) ServerInfo() ServerInfo { return c.info }

func (c *stdioClient) ListTools(ctx context.Context, cursor string) (ToolPage, error) {
	res, err := c.session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
	if err != nil {
		return ToolPage{}, classifyErr(err, apperror.KindProtocol, "list tools")
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

func (c *stdioClient) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return ToolResult{}, classifyErr(err, apperror.KindProtocol, "call tool %q", name)
	}
	return toToolResult(res), nil
}

// Close gracefully closes the session (the SDK terminates the direct child)
// and then sweeps the process group to reap any orphaned descendants. Close
// may return a non-nil error (e.g. the child's "signal: killed" when it
// ignores graceful shutdown and has to be force-terminated); callers should
// not treat a non-nil return from Close as a command failure.
func (c *stdioClient) Close() error {
	err := c.session.Close()
	process.KillGroup(c.cmd)
	return err
}
