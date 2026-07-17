package client

import "testing"

func TestClientInfo(t *testing.T) {
	info := clientInfo()
	if info.Name != "mcpctl" {
		t.Fatalf("clientInfo().Name = %q, want %q", info.Name, "mcpctl")
	}
	if info.Version == "" {
		t.Fatal("clientInfo().Version must not be empty")
	}
}
