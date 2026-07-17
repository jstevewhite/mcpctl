package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// commandContext derives a context that is canceled on SIGINT/SIGTERM and that
// times out after the given duration. The returned CancelFunc must be called
// to release resources (and stop the signal handler).
func commandContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	sigCtx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(sigCtx, timeout)
	return ctx, func() {
		cancel()
		stop()
	}
}
