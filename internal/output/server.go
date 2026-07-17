package output

import (
	"io"

	"mcpctl/internal/config"
)

// NamedServer pairs a server name with its (already-redacted) definition.
type NamedServer struct {
	Name   string              `json:"name"`
	Server config.ServerConfig `json:"server"`
}

// Servers renders a list of servers in a machine-readable format.
func Servers(w io.Writer, f Format, servers []NamedServer) error {
	switch f {
	case FormatYAML:
		return writeYAML(w, servers)
	case FormatJSONL:
		items := make([]any, len(servers))
		for i := range servers {
			items[i] = servers[i]
		}
		return writeJSONLines(w, items)
	default: // json
		return writeJSONIndent(w, servers)
	}
}

// Server renders a single server in a machine-readable format.
func Server(w io.Writer, f Format, name string, sc config.ServerConfig) error {
	ns := NamedServer{Name: name, Server: sc}
	switch f {
	case FormatYAML:
		return writeYAML(w, ns)
	default: // json or jsonl (single object)
		if f == FormatJSONL {
			return writeJSONCompact(w, ns)
		}
		return writeJSONIndent(w, ns)
	}
}
