package arguments

import (
	"strings"
	"testing"
)

func TestParseMutualExclusion(t *testing.T) {
	if _, err := Parse(`{"a":1}`, "", []string{"b=2"}, nil); err == nil {
		t.Fatal("expected error when --json and --arg are both set")
	}
}

func TestParseJSONMustBeObject(t *testing.T) {
	if _, err := Parse(`[1,2]`, "", nil, nil); err == nil {
		t.Fatal("array top-level must be rejected")
	}
	got, err := Parse(`{"q":"x"}`, "", nil, nil)
	if err != nil || got["q"] != "x" {
		t.Fatalf("object parse failed: %v %v", got, err)
	}
}

func TestParseJSONFileStdin(t *testing.T) {
	got, err := Parse("", "-", nil, strings.NewReader(`{"n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if got["n"].(float64) != 1 {
		t.Fatalf("stdin json not parsed: %v", got)
	}
}

func TestParseArgs(t *testing.T) {
	got, err := Parse("", "", []string{
		`query=MCP`, `limit=10`, `enabled=true`, `tags=["go","mcp"]`, `zip=01234`, `str="true"`,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got["query"] != "MCP" {
		t.Errorf("query = %v, want string MCP", got["query"])
	}
	if got["limit"].(float64) != 10 {
		t.Errorf("limit = %v, want number 10", got["limit"])
	}
	if got["enabled"] != true {
		t.Errorf("enabled = %v, want bool true", got["enabled"])
	}
	if _, ok := got["tags"].([]any); !ok {
		t.Errorf("tags = %T, want array", got["tags"])
	}
	if got["zip"] != "01234" {
		t.Errorf("zip = %v, want string 01234 (leading zero is not valid JSON)", got["zip"])
	}
	if got["str"] != "true" {
		t.Errorf(`str = %v, want string "true"`, got["str"])
	}
}

func TestParseArgErrors(t *testing.T) {
	if _, err := Parse("", "", []string{"noequals"}, nil); err == nil {
		t.Error("missing = should error")
	}
	if _, err := Parse("", "", []string{"=v"}, nil); err == nil {
		t.Error("empty key should error")
	}
	if _, err := Parse("", "", []string{"a=1", "a=2"}, nil); err == nil {
		t.Error("duplicate key should error")
	}
}

func TestParseNoneIsNil(t *testing.T) {
	got, err := Parse("", "", nil, nil)
	if err != nil || got != nil {
		t.Fatalf("no args should be (nil, nil), got %v %v", got, err)
	}
}

func TestParseJSONRejectsTrailingGarbage(t *testing.T) {
	if _, err := Parse(`{"a":1}x`, "", nil, nil); err == nil {
		t.Fatal("trailing garbage after a JSON object must be rejected")
	}
}
