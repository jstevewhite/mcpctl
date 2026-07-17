// Package process handles OS-specific process-group setup and teardown for
// spawned stdio MCP servers. The MCP SDK terminates only the direct child;
// this package ensures a misbehaving server's descendants are cleaned up too.
package process

import "os/exec"

// Setup configures cmd so its descendants can be terminated as a group.
// Must be called before cmd.Start().
func Setup(cmd *exec.Cmd) { setup(cmd) }

// KillGroup force-terminates cmd's entire process group (best-effort; errors
// are ignored because the group may already be gone). Call it after the SDK's
// own graceful session Close, to sweep any orphaned descendants.
func KillGroup(cmd *exec.Cmd) { killGroup(cmd) }
