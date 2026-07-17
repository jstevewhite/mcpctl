package client

import "github.com/modelcontextprotocol/go-sdk/mcp"

func toToolInfo(t *mcp.Tool) ToolInfo {
	return ToolInfo{
		Name:         t.Name,
		Description:  t.Description,
		Title:        t.Title,
		InputSchema:  t.InputSchema,
		OutputSchema: t.OutputSchema,
	}
}

func toToolResult(r *mcp.CallToolResult) ToolResult {
	out := ToolResult{
		Structured: r.StructuredContent,
		IsError:    r.IsError,
	}
	for _, c := range r.Content {
		out.Content = append(out.Content, toContentBlock(c))
	}
	return out
}

// toContentBlock converts one SDK content item. Text/image/audio are captured
// in full; other kinds (embedded resource, resource link) are recorded as
// KindResource with kind only — richer resource rendering is added when
// binary/resource output is built (Plan 2B / Phase 4).
func toContentBlock(c mcp.Content) ContentBlock {
	switch v := c.(type) {
	case *mcp.TextContent:
		return ContentBlock{Kind: KindText, Text: v.Text}
	case *mcp.ImageContent:
		return ContentBlock{Kind: KindImage, MIMEType: v.MIMEType, Data: v.Data}
	case *mcp.AudioContent:
		return ContentBlock{Kind: KindAudio, MIMEType: v.MIMEType, Data: v.Data}
	case *mcp.EmbeddedResource, *mcp.ResourceLink:
		return ContentBlock{Kind: KindResource}
	default:
		return ContentBlock{Kind: KindUnknown}
	}
}
