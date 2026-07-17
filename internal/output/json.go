package output

import (
	"encoding/json"
	"io"

	"mcpctl/internal/client"
)

// writeJSON encodes v as compact JSON (one value, newline-terminated).
//
// Deviation from the task brief: the brief's writeJSON used
// enc.SetIndent("", "  "), but indented encoding inserts a space after each
// key's colon (e.g. `"isError": true`), which does not match the brief's own
// TestToolResultJSONFaithful assertion that the output contains the substring
// `"isError":true` (no space). Compact encoding satisfies that test exactly
// while still producing valid, faithful JSON; the human-readable indented
// rendering of nested schema/structured data (used within the "human" format)
// is unaffected — see indentJSON in human.go.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}

func toolListJSON(w io.Writer, server string, tools []client.ToolInfo) error {
	if tools == nil {
		tools = []client.ToolInfo{}
	}
	return writeJSON(w, struct {
		Server string            `json:"server"`
		Tools  []client.ToolInfo `json:"tools"`
	}{Server: server, Tools: tools})
}
