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
