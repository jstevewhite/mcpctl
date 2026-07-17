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

// The following three methods are temporary placeholders that exist only so
// *stdioClient satisfies the Client interface (defined in full in Task 1,
// before any implementation existed). They are NOT part of Task 5's scope and
// carry no real logic; Task 6 replaces ListTools/ListAllTools and Task 7
// replaces CallTool with real implementations against session.ListTools /
// session.CallTool.

func (c *stdioClient) ListTools(ctx context.Context, cursor string) (ToolPage, error) {
	return ToolPage{}, apperror.Internal("ListTools: not yet implemented (Task 6)")
}

func (c *stdioClient) ListAllTools(ctx context.Context, maxPages int) ([]ToolInfo, error) {
	return nil, apperror.Internal("ListAllTools: not yet implemented (Task 6)")
}

func (c *stdioClient) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	return ToolResult{}, apperror.Internal("CallTool: not yet implemented (Task 7)")
}

// Close gracefully closes the session (the SDK terminates the direct child)
// and then sweeps the process group to reap any orphaned descendants.
func (c *stdioClient) Close() error {
	err := c.session.Close()
	process.KillGroup(c.cmd)
	return err
}
