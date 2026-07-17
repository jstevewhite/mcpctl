//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

func setup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Put the child in its own process group (pgid == child pid) so a single
	// negative-pid signal reaches the whole tree.
	cmd.SysProcAttr.Setpgid = true
}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// Negative pid targets the process group led by the child.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
