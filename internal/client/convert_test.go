package client

import (
	"bytes"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToToolInfo(t *testing.T) {
	got := toToolInfo(&mcp.Tool{
		Name:         "echo",
		Description:  "echo back",
		Title:        "Echo",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "number"},
	})
	if got.Name != "echo" || got.Description != "echo back" || got.Title != "Echo" {
		t.Fatalf("unexpected ToolInfo: %+v", got)
	}
	if _, ok := got.InputSchema.(map[string]any); !ok {
		t.Fatalf("InputSchema not preserved: %T", got.InputSchema)
	}
	if _, ok := got.OutputSchema.(map[string]any); !ok {
		t.Errorf("OutputSchema not preserved: %T", got.OutputSchema)
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
	if !bytes.Equal(got.Content[1].Data, []byte{1, 2, 3}) {
		t.Errorf("image block data wrong: %+v", got.Content[1].Data)
	}
}

func TestToContentBlockKinds(t *testing.T) {
	audio := toContentBlock(&mcp.AudioContent{MIMEType: "audio/wav", Data: []byte{9}})
	if audio.Kind != KindAudio || audio.MIMEType != "audio/wav" || len(audio.Data) != 1 {
		t.Errorf("audio block wrong: %+v", audio)
	}

	resource := toContentBlock(&mcp.EmbeddedResource{})
	if resource.Kind != KindResource {
		t.Errorf("embedded resource block wrong: %+v", resource)
	}

	link := toContentBlock(&mcp.ResourceLink{})
	if link.Kind != KindResource {
		t.Errorf("resource link block wrong: %+v", link)
	}
}
