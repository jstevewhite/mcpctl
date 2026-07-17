// Package output renders command results in the supported formats. It never
// imports the MCP SDK; it operates on the SDK-free client boundary types.
package output

import (
	"io"

	"mcpctl/internal/apperror"
	"mcpctl/internal/client"
)

type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
	FormatJSONL Format = "jsonl"
	FormatYAML  Format = "yaml"
)

// ParseFormat validates a format name.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatHuman, FormatJSON, FormatJSONL, FormatYAML:
		return Format(s), nil
	default:
		return "", apperror.Usage("unknown output format %q (want human, json, jsonl, or yaml)", s)
	}
}

func ToolList(w io.Writer, f Format, server string, tools []client.ToolInfo) error {
	switch f {
	case FormatJSON:
		return writeJSONIndent(w, toolListDoc(server, tools))
	case FormatYAML:
		return writeYAML(w, toolListDoc(server, tools))
	case FormatJSONL:
		items := make([]any, len(tools))
		for i := range tools {
			items[i] = tools[i]
		}
		return writeJSONLines(w, items)
	default:
		return toolListHuman(w, tools)
	}
}

func ToolDescribe(w io.Writer, f Format, tool client.ToolInfo) error {
	switch f {
	case FormatJSON:
		return writeJSONIndent(w, tool)
	case FormatYAML:
		return writeYAML(w, tool)
	case FormatJSONL:
		return writeJSONCompact(w, tool)
	default:
		return toolDescribeHuman(w, tool)
	}
}

func ToolResult(w io.Writer, f Format, r client.ToolResult) error {
	switch f {
	case FormatJSON:
		return writeJSONIndent(w, r)
	case FormatYAML:
		return writeYAML(w, r)
	case FormatJSONL:
		return writeJSONCompact(w, r)
	default:
		return toolResultHuman(w, r)
	}
}
