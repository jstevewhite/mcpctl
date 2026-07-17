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
