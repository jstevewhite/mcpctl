//go:build !windows

package process

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func alive(pid int) bool { return syscall.Kill(pid, syscall.Signal(0)) == nil }

// The child shell backgrounds a grandchild sleep in the same process group and
// prints the grandchild's PID. KillGroup must reap the grandchild, not just the
// direct child.
func TestKillGroupReapsGrandchild(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $!; wait")
	Setup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("reading grandchild pid: %v", err)
	}
	gpid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parsing grandchild pid %q: %v", line, err)
	}
	if !alive(gpid) {
		t.Fatalf("grandchild %d should be alive before KillGroup", gpid)
	}

	KillGroup(cmd)
	_ = cmd.Wait() // reap the direct child (killed by the group signal)

	deadline := time.Now().Add(2 * time.Second)
	for alive(gpid) {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still alive 2s after KillGroup", gpid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
