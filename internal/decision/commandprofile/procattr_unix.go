//go:build !windows

package commandprofile

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the adapter in its own process group so the whole tree can be
// terminated together on cancellation or timeout.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// afterStart is a no-op on Unix: the process group covers descendants.
func afterStart(_ *exec.Cmd) {}

// killProcessTree terminates the adapter and every descendant by signaling the
// whole process group.
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
