//go:build !windows

package client

import (
	"context"
	"syscall"
	"testing"
	"time"
)

func procAlive(pid int) bool { return syscall.Kill(pid, syscall.Signal(0)) == nil }

func dialTestServer(t *testing.T) (*stdioClient, context.Context) {
	t.Helper()
	ctx := context.Background()
	c, err := DialStdio(ctx, StdioSpec{Command: testServerPath})
	if err != nil {
		t.Fatalf("DialStdio: %v", err)
	}
	return c.(*stdioClient), ctx
}

func TestDialAndServerInfo(t *testing.T) {
	c, _ := dialTestServer(t)
	defer c.Close()

	info := c.ServerInfo()
	if info.Name != "mcpctl-test-server" {
		t.Errorf("ServerInfo().Name = %q, want mcpctl-test-server", info.Name)
	}
	if !info.SupportsTools {
		t.Error("expected SupportsTools = true")
	}
	if info.ProtocolVersion == "" {
		t.Error("expected a negotiated protocol version")
	}
}

func TestCloseTerminatesChild(t *testing.T) {
	c, _ := dialTestServer(t)
	pid := c.cmd.Process.Pid
	if !procAlive(pid) {
		t.Fatalf("child %d should be alive after dial", pid)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for procAlive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d still alive 3s after Close", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
