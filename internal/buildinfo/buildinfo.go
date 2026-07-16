// Package buildinfo holds version metadata injected at build time.
package buildinfo

import (
	"fmt"
	"runtime"
)

// These are overridden at build time via -ldflags "-X".
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// GoVersion returns the Go runtime version the binary was built with.
func GoVersion() string { return runtime.Version() }

// Short returns the concise one-line version string.
func Short() string { return "mcpctl " + Version }

// Full returns the multi-line version block.
func Full() string {
	return fmt.Sprintf("mcpctl version %s\ncommit: %s\nbuilt: %s\ngo: %s",
		Version, Commit, Date, GoVersion())
}
