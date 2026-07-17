package client

import (
	"testing"
)

func TestListToolsPaginates(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	// First page (server PageSize=2) must be a partial page with a cursor.
	page, err := c.ListTools(ctx, "")
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(page.Tools) != 2 {
		t.Fatalf("first page = %d tools, want 2", len(page.Tools))
	}
	if page.NextCursor == "" {
		t.Fatal("expected a NextCursor on the first page")
	}
}

func TestListAllToolsFollowsPages(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()

	all, err := c.ListAllTools(ctx, 100)
	if err != nil {
		t.Fatalf("ListAllTools: %v", err)
	}
	// Test server registers 8 tools (echo, add, boom, slow, pad_1..pad_4).
	if len(all) != 8 {
		t.Fatalf("ListAllTools = %d tools, want 8", len(all))
	}
	names := map[string]bool{}
	for _, tl := range all {
		names[tl.Name] = true
	}
	for _, want := range []string{"echo", "add", "boom", "slow"} {
		if !names[want] {
			t.Errorf("missing expected tool %q", want)
		}
	}
}

func TestListAllToolsPageCap(t *testing.T) {
	c, ctx := dialTestServer(t)
	defer c.Close()
	// maxPages=1 with PageSize=2 and 8 tools must hit the cap and error.
	_, err := c.ListAllTools(ctx, 1)
	if err == nil {
		t.Fatal("expected a page-cap error with maxPages=1")
	}
}
