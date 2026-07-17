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
