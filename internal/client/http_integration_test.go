package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newHTTPTestServer returns an httptest.Server running an MCP server with an
// `echo` tool, plus a wrap hook the caller can use to inject failures/headers.
func newHTTPTestServer(t *testing.T, wrap func(http.Handler) http.Handler) *httptest.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "http-test-server", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo"},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			Message string `json:"message"`
		}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Message}}}, nil, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	var h http.Handler = handler
	if wrap != nil {
		h = wrap(h)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPDialListCall(t *testing.T) {
	srv := newHTTPTestServer(t, nil)
	ctx := context.Background()
	c, err := DialHTTP(ctx, HTTPSpec{URL: srv.URL, Header: http.Header{}})
	if err != nil {
		t.Fatalf("DialHTTP: %v", err)
	}
	defer c.Close()

	if !c.ServerInfo().SupportsTools {
		t.Fatal("expected SupportsTools")
	}
	tools, err := c.ListAllTools(ctx, 1000)
	if err != nil {
		t.Fatalf("ListAllTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	res, err := c.CallTool(ctx, "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "hi" {
		t.Fatalf("unexpected result: %+v", res.Content)
	}
}
