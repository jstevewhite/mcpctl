// Package logging configures the slog logger used for diagnostics.
package logging

import (
	"io"
	"log/slog"
	"strings"

	"github.com/jstevewhite/mcpctl/internal/apperror"
)

// ParseLevel converts a level name to an slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, apperror.Usage("invalid log level %q (want debug, info, warn, or error)", s)
	}
}

// Setup builds a text logger writing to w, filtered at the given level.
func Setup(w io.Writer, level string) (*slog.Logger, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	return slog.New(h), nil
}
