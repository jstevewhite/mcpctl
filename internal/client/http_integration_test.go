package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcpctl/internal/apperror"
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

// statusWrap returns a wrap hook that responds to every request with the given
// status (before the MCP handler runs).
func statusWrap(status int) func(http.Handler) http.Handler {
	return func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})
	}
}

func TestHTTP401IsAuthError(t *testing.T) {
	srv := newHTTPTestServer(t, statusWrap(http.StatusUnauthorized))
	_, err := DialHTTP(context.Background(), HTTPSpec{URL: srv.URL, Header: http.Header{}})
	if err == nil {
		t.Fatal("expected an auth error")
	}
	if code := apperror.ExitCode(err); code != 4 {
		t.Fatalf("exit code = %d, want 4 (auth)", code)
	}
}

func TestHTTP403IsAuthError(t *testing.T) {
	srv := newHTTPTestServer(t, statusWrap(http.StatusForbidden))
	_, err := DialHTTP(context.Background(), HTTPSpec{URL: srv.URL, Header: http.Header{}})
	if code := apperror.ExitCode(err); code != 4 {
		t.Fatalf("exit code = %d, want 4 (auth); err=%v", code, err)
	}
}

func TestHTTP500IsConnectionError(t *testing.T) {
	srv := newHTTPTestServer(t, statusWrap(http.StatusInternalServerError))
	_, err := DialHTTP(context.Background(), HTTPSpec{URL: srv.URL, Header: http.Header{}})
	if code := apperror.ExitCode(err); code != 5 {
		t.Fatalf("exit code = %d, want 5 (connection); err=%v", code, err)
	}
}

func TestHTTPHeadersReachServer(t *testing.T) {
	var gotAuth, gotLang string
	srv := newHTTPTestServer(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if gotAuth == "" {
				gotAuth = r.Header.Get("Authorization")
				gotLang = r.Header.Get("Accept-Language")
			}
			next.ServeHTTP(w, r)
		})
	})
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer tok")
	hdr.Set("Accept-Language", "en-US")
	c, err := DialHTTP(context.Background(), HTTPSpec{URL: srv.URL, Header: hdr})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if gotAuth != "Bearer tok" || gotLang != "en-US" {
		t.Fatalf("headers not received: auth=%q lang=%q", gotAuth, gotLang)
	}
}

func TestHTTPRedirectDoesNotForwardCredentials(t *testing.T) {
	// Target server records whether any credential header arrived.
	var leakedAuth, leakedKey string
	target := newHTTPTestServer(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if leakedAuth == "" && leakedKey == "" {
				leakedAuth = r.Header.Get("Authorization")
				leakedKey = r.Header.Get("X-Api-Key")
			}
			next.ServeHTTP(w, r)
		})
	})
	// Redirector server (different origin) 307-redirects everything to target.
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redir.Close)

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer tok")
	hdr.Set("X-Api-Key", "secret")
	c, err := DialHTTP(context.Background(), HTTPSpec{URL: redir.URL, Header: hdr})
	if err != nil {
		t.Fatalf("DialHTTP through redirect: %v", err)
	}
	c.Close()
	if leakedAuth != "" || leakedKey != "" {
		t.Fatalf("credentials leaked across redirect: auth=%q key=%q", leakedAuth, leakedKey)
	}
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
