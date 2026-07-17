package client

import "github.com/modelcontextprotocol/go-sdk/mcp"

func toToolInfo(t *mcp.Tool) ToolInfo {
	return ToolInfo{
		Name:         t.Name,
		Description:  t.Description,
		Title:        t.Title,
		InputSchema:  t.InputSchema,
		OutputSchema: t.OutputSchema,
		Annotations:  toAnnotations(t.Annotations),
		Meta:         map[string]any(t.Meta),
	}
}

func toAnnotations(a *mcp.ToolAnnotations) *ToolAnnotations {
	if a == nil {
		return nil
	}
	return &ToolAnnotations{
		Title:           a.Title,
		ReadOnlyHint:    a.ReadOnlyHint,
		DestructiveHint: a.DestructiveHint,
		IdempotentHint:  a.IdempotentHint,
		OpenWorldHint:   a.OpenWorldHint,
	}
}

func toToolResult(r *mcp.CallToolResult) ToolResult {
	out := ToolResult{
		Structured: r.StructuredContent,
		IsError:    r.IsError,
		Meta:       map[string]any(r.Meta),
	}
	for _, c := range r.Content {
		out.Content = append(out.Content, toContentBlock(c))
	}
	return out
}

// toContentBlock converts one SDK content item. Text/image/audio are captured
// in full; embedded resources and resource links carry their URI (and, where
// present, MIME type / text / blob / name).
func toContentBlock(c mcp.Content) ContentBlock {
	switch v := c.(type) {
	case *mcp.TextContent:
		return ContentBlock{Kind: KindText, Text: v.Text}
	case *mcp.ImageContent:
		return ContentBlock{Kind: KindImage, MIMEType: v.MIMEType, Data: v.Data}
	case *mcp.AudioContent:
		return ContentBlock{Kind: KindAudio, MIMEType: v.MIMEType, Data: v.Data}
	case *mcp.EmbeddedResource:
		cb := ContentBlock{Kind: KindResource}
		if v.Resource != nil {
			cb.URI = v.Resource.URI
			cb.MIMEType = v.Resource.MIMEType
			cb.Text = v.Resource.Text
			cb.Data = v.Resource.Blob
		}
		return cb
	case *mcp.ResourceLink:
		return ContentBlock{Kind: KindResource, URI: v.URI, MIMEType: v.MIMEType, Name: v.Name}
	default:
		return ContentBlock{Kind: KindUnknown}
	}
}
