// Package auth resolves HTTP authentication headers and identifies secrets to
// redact from logs. It does not import the MCP SDK.
package auth

import "strings"

var exactSensitive = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
}

var sensitiveSubstrings = []string{"token", "secret", "password", "key"}

// IsSensitive reports whether a header (or variable) name likely carries a
// credential and must be redacted from logs (spec §12.2).
func IsSensitive(name string) bool {
	l := strings.ToLower(name)
	if exactSensitive[l] {
		return true
	}
	for _, s := range sensitiveSubstrings {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// RedactValue is the placeholder used in place of a secret value in logs/output.
func RedactValue() string { return "<redacted>" }
