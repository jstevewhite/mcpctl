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

func TestToToolInfoAnnotationsAndMeta(t *testing.T) {
	ro := true
	got := toToolInfo(&mcp.Tool{
		Name:        "t",
		Annotations: &mcp.ToolAnnotations{Title: "T", ReadOnlyHint: true, DestructiveHint: &ro},
		Meta:        mcp.Meta{"k": "v"},
	})
	if got.Annotations == nil || got.Annotations.Title != "T" || !got.Annotations.ReadOnlyHint {
		t.Fatalf("annotations not converted: %+v", got.Annotations)
	}
	if got.Annotations.DestructiveHint == nil || !*got.Annotations.DestructiveHint {
		t.Fatalf("destructiveHint pointer not preserved")
	}
	if got.Meta["k"] != "v" {
		t.Fatalf("meta not converted: %+v", got.Meta)
	}
}

func TestToContentBlockResource(t *testing.T) {
	emb := toContentBlock(&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{
		URI: "file:///x", MIMEType: "text/plain", Text: "hi", Blob: []byte{1},
	}})
	if emb.Kind != KindResource || emb.URI != "file:///x" || emb.Text != "hi" || len(emb.Data) != 1 {
		t.Fatalf("embedded resource not converted: %+v", emb)
	}
	link := toContentBlock(&mcp.ResourceLink{URI: "https://y", Name: "y", MIMEType: "text/html"})
	if link.Kind != KindResource || link.URI != "https://y" || link.Name != "y" {
		t.Fatalf("resource link not converted: %+v", link)
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
