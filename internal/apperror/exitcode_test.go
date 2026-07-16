package apperror

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"usage", Usage("x"), 2},
		{"config", Config("x"), 3},
		{"internal", Internal("x"), 1},
		{"toolNotFound", New(KindToolNotFound, "x"), 7},
		{"invalidArgs", New(KindInvalidArgs, "x"), 8},
		{"toolError", New(KindToolError, "x"), 9},
		{"timeout", New(KindTimeout, "x"), 10},
		{"interrupted", New(KindInterrupted, "x"), 130},
		{"wrappedTyped", fmt.Errorf("outer: %w", Config("x")), 3},
		{"ctxDeadline", context.DeadlineExceeded, 10},
		{"ctxCanceled", context.Canceled, 130},
		{"unknown", errors.New("plain"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Fatalf("ExitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
