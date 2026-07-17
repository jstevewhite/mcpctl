package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"mcpctl/internal/client"
)

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"human", "json"} {
		if _, err := ParseFormat(ok); err != nil {
			t.Errorf("ParseFormat(%q) errored: %v", ok, err)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("yaml should be unsupported in Phase 2")
	}
	if _, err := ParseFormat("bogus"); err == nil {
		t.Error("bogus format should error")
	}
}

func TestToolListJSON(t *testing.T) {
	var buf bytes.Buffer
	err := ToolList(&buf, FormatJSON, "local", []client.ToolInfo{{Name: "echo", Description: "d"}})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Server string            `json:"server"`
		Tools  []client.ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if got.Server != "local" || len(got.Tools) != 1 || got.Tools[0].Name != "echo" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestToolListHuman(t *testing.T) {
	var buf bytes.Buffer
	if err := ToolList(&buf, FormatHuman, "local", []client.ToolInfo{{Name: "echo", Description: "say hi"}}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "echo") || !strings.Contains(out, "say hi") {
		t.Fatalf("human list missing content:\n%s", out)
	}
}

func TestToolResultHumanText(t *testing.T) {
	var buf bytes.Buffer
	err := ToolResult(&buf, FormatHuman, client.ToolResult{
		Content: []client.ContentBlock{{Kind: client.KindText, Text: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("expected text content, got:\n%s", buf.String())
	}
}

func TestToolResultJSONFaithful(t *testing.T) {
	var buf bytes.Buffer
	err := ToolResult(&buf, FormatJSON, client.ToolResult{
		IsError: true,
		Content: []client.ContentBlock{{Kind: client.KindText, Text: "boom"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("invalid JSON: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"isError":true`) {
		t.Fatalf("isError not preserved: %s", buf.String())
	}
}

func TestToolDescribeHumanAndJSON(t *testing.T) {
	tool := client.ToolInfo{Name: "echo", Description: "d", InputSchema: map[string]any{"type": "object"}}
	var h bytes.Buffer
	if err := ToolDescribe(&h, FormatHuman, tool); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.String(), "echo") {
		t.Fatalf("human describe missing name:\n%s", h.String())
	}
	var j bytes.Buffer
	if err := ToolDescribe(&j, FormatJSON, tool); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(j.Bytes()) {
		t.Fatalf("invalid JSON: %s", j.String())
	}
}

func TestToolResultHumanErrorAndResource(t *testing.T) {
	var buf bytes.Buffer
	err := ToolResult(&buf, FormatHuman, client.ToolResult{
		IsError: true,
		Content: []client.ContentBlock{
			{Kind: client.KindResource, URI: "file:///x", MIMEType: "text/plain"},
			{Kind: client.KindImage, MIMEType: "image/png", Data: []byte{1, 2}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "file:///x") {
		t.Errorf("missing resource uri:\n%s", out)
	}
	if !strings.Contains(out, "image/png") {
		t.Errorf("missing image mime:\n%s", out)
	}
}
