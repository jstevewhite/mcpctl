package output

import "strings"

// sanitize removes control characters that could corrupt or spoof a terminal
// (ANSI escapes, other C0/C1 controls) from untrusted server text, keeping
// ordinary whitespace (newline, tab). It is applied only to human-mode output;
// machine-readable formats carry structured data verbatim.
func sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f: // C0 controls + DEL
			// drop
		case r >= 0x80 && r <= 0x9f: // C1 controls
			// drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
