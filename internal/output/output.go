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

// ParseFormat validates a format name. jsonl and yaml are recognized but not
// yet implemented (Phase 4).
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatHuman, FormatJSON:
		return Format(s), nil
	case FormatJSONL, FormatYAML:
		return "", apperror.Usage("output format %q is not supported yet (arrives in a later version)", s)
	default:
		return "", apperror.Usage("unknown output format %q (want human or json)", s)
	}
}

func ToolList(w io.Writer, f Format, server string, tools []client.ToolInfo) error {
	switch f {
	case FormatJSON:
		return toolListJSON(w, server, tools)
	default:
		return toolListHuman(w, tools)
	}
}

func ToolDescribe(w io.Writer, f Format, tool client.ToolInfo) error {
	switch f {
	case FormatJSON:
		return writeJSON(w, tool)
	default:
		return toolDescribeHuman(w, tool)
	}
}

func ToolResult(w io.Writer, f Format, r client.ToolResult) error {
	switch f {
	case FormatJSON:
		return writeJSON(w, r)
	default:
		return toolResultHuman(w, r)
	}
}
