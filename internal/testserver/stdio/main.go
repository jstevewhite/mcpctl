// Command stdio is a deterministic MCP test server used by internal/client
// integration tests. It talks MCP over stdio and exposes a fixed set of tools.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoArgs struct {
	Message string `json:"message" jsonschema:"the message to echo back"`
}

type addArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

type slowArgs struct {
	Seconds int `json:"seconds"`
}

func main() {
	logger := log.New(os.Stderr, "[test-server] ", 0)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcpctl-test-server",
		Version: "0.0.1",
	}, &mcp.ServerOptions{PageSize: 2})

	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo back the given message"},
		func(ctx context.Context, req *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Message}}}, nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "add", Description: "add two integers, returning structured content"},
		func(ctx context.Context, req *mcp.CallToolRequest, args addArgs) (*mcp.CallToolResult, any, error) {
			sum := args.A + args.B
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%d", sum)}},
				StructuredContent: map[string]any{"sum": sum},
			}, nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "boom", Description: "always fails with a tool-level error"},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "tool failed on purpose"}},
			}, nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "slow", Description: "sleeps for the given number of seconds"},
		func(ctx context.Context, req *mcp.CallToolRequest, args slowArgs) (*mcp.CallToolResult, any, error) {
			select {
			case <-time.After(time.Duration(args.Seconds) * time.Second):
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "done"}}}, nil, nil
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		})

	// Padding tools so that with PageSize=2 and 8 tools the list spans 4 pages.
	for i := 1; i <= 4; i++ {
		name := fmt.Sprintf("pad_%d", i)
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: "padding tool"},
			func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pad"}}}, nil, nil
			})
	}

	logger.Printf("test server starting pid=%d", os.Getpid())
	err := server.Run(context.Background(), &mcp.StdioTransport{})
	// Normal client-initiated shutdown surfaces as an error wrapping io.EOF
	// (the SDK exports no sentinel for it). Treat EOF as a clean exit.
	if err != nil && !errors.Is(err, io.EOF) {
		logger.Fatalf("server exited with error: %v", err)
	}
}
