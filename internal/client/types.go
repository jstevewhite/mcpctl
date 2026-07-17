// Package client is the SDK-free boundary between mcpctl's command handlers
// and the official Go MCP SDK. Only this package imports the SDK.
package client

import "net/http"

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

// HTTPSpec describes a Streamable HTTP server. Header holds the already-resolved
// request headers (env/bearer resolved by the caller); the URL is absolute.
type HTTPSpec struct {
	URL    string
	Header http.Header
}
