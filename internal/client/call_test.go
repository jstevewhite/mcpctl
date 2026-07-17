package client

import (
	"context"
	"testing"
	"time"
)

func TestCallToolEchoText(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	res, err := c.CallTool(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("CallTool(echo): %v", err)
	}
	if res.IsError {
		t.Fatal("echo unexpectedly returned IsError")
	}
	if len(res.Content) != 1 || res.Content[0].Kind != KindText || res.Content[0].Text != "hello" {
		t.Fatalf("unexpected echo result: %+v", res.Content)
	}
}

func TestCallToolStructured(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	res, err := c.CallTool(ctx, "add", map[string]any{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("CallTool(add): %v", err)
	}
	if res.Structured == nil {
		t.Fatal("expected structured content from add")
	}
}

func TestCallToolIsError(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	res, err := c.CallTool(ctx, "boom", nil)
	if err != nil {
		t.Fatalf("boom should be a normal result, got Go error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError = true from boom")
	}
}

func TestCallToolUnknownIsGoError(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	_, err := c.CallTool(ctx, "does_not_exist", nil)
	if err == nil {
		t.Fatal("expected a Go error calling an unknown tool")
	}
}

func TestCallToolContextCancel(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.CallTool(cctx, "slow", map[string]any{"seconds": 5})
	if err == nil {
		t.Fatal("expected a cancellation error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("cancellation took too long: %v", time.Since(start))
	}
}
