package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"mcpctl/internal/client"
)

const descWidth = 60

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func toolListHuman(w io.Writer, tools []client.ToolInfo) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDESCRIPTION")
	for _, t := range tools {
		fmt.Fprintf(tw, "%s\t%s\n", t.Name, truncate(t.Description, descWidth))
	}
	return tw.Flush()
}

func toolDescribeHuman(w io.Writer, t client.ToolInfo) error {
	fmt.Fprintf(w, "Name:        %s\n", t.Name)
	if t.Title != "" {
		fmt.Fprintf(w, "Title:       %s\n", t.Title)
	}
	fmt.Fprintf(w, "Description: %s\n", t.Description)
	if t.InputSchema != nil {
		fmt.Fprintf(w, "Input schema:\n%s\n", indentJSON(t.InputSchema))
	}
	if t.OutputSchema != nil {
		fmt.Fprintf(w, "Output schema:\n%s\n", indentJSON(t.OutputSchema))
	}
	if t.Annotations != nil {
		fmt.Fprintf(w, "Annotations:\n%s\n", indentJSON(t.Annotations))
	}
	return nil
}

func toolResultHuman(w io.Writer, r client.ToolResult) error {
	if r.IsError {
		fmt.Fprintln(w, "TOOL ERROR:")
	}
	for _, c := range r.Content {
		switch c.Kind {
		case client.KindText:
			fmt.Fprintln(w, c.Text)
		case client.KindImage, client.KindAudio:
			fmt.Fprintf(w, "[%s content, %s, %d bytes]\n", c.Kind, c.MIMEType, len(c.Data))
		case client.KindResource:
			fmt.Fprintf(w, "[resource %s %s]\n", c.URI, c.MIMEType)
		default:
			fmt.Fprintf(w, "[%s content]\n", c.Kind)
		}
	}
	if r.Structured != nil {
		fmt.Fprintf(w, "Structured:\n%s\n", indentJSON(r.Structured))
	}
	return nil
}

func indentJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
