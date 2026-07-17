//go:build windows

package process

import "os/exec"

// On Windows there is no process-group signaling equivalent to Unix pgids
// without job objects. The SDK's CommandTransport.Close already terminates the
// direct child; full descendant cleanup via a Job Object is deferred (tracked).
func setup(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
