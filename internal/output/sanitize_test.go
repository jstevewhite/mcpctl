package output

import (
	"strings"
	"testing"
)

func TestSanitizeStripsControlChars(t *testing.T) {
	// An ANSI escape sequence and a bell should be removed; text and newline kept.
	in := "hello\x1b[31mRED\x1b[0m\x07 world\nline2\ttab"
	got := sanitize(in)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\x07") {
		t.Fatalf("control chars not stripped: %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("visible text lost: %q", got)
	}
	if !strings.Contains(got, "\n") || !strings.Contains(got, "\t") {
		t.Fatalf("newline/tab should be preserved: %q", got)
	}
}

func TestTableCellCollapsesRowInjection(t *testing.T) {
	got := tableCell("evil\nNAME\tDESC")
	if strings.Contains(got, "\n") || strings.Contains(got, "\t") {
		t.Fatalf("table cell must not contain newlines/tabs: %q", got)
	}
}
