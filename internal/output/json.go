package output

import (
	"encoding/json"
	"io"

	"github.com/jstevewhite/mcpctl/internal/client"
)

func toolListDoc(server string, tools []client.ToolInfo) any {
	if tools == nil {
		tools = []client.ToolInfo{}
	}
	return struct {
		Server string            `json:"server"`
		Tools  []client.ToolInfo `json:"tools"`
	}{Server: server, Tools: tools}
}

func writeJSONIndent(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeJSONCompact(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v) // Encoder writes one line + newline
}

func writeJSONLines(w io.Writer, items []any) error {
	enc := json.NewEncoder(w)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return nil
}
