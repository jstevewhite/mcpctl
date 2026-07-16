package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"Error": slog.LevelError,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = (%v, %v), want (%v, nil)", in, got, err, want)
		}
	}
	if _, err := ParseLevel("nope"); err == nil {
		t.Error("ParseLevel(\"nope\") should error")
	}
}

func TestSetupFiltersBelowLevel(t *testing.T) {
	var buf bytes.Buffer
	log, err := Setup(&buf, "warn")
	if err != nil {
		t.Fatal(err)
	}
	log.Info("hidden")
	log.Warn("shown")
	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Error("info should be filtered at warn level")
	}
	if !strings.Contains(out, "shown") {
		t.Error("warn should be emitted at warn level")
	}
}
