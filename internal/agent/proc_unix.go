//go:build unix

package agent

import (
	"errors"
	"syscall"
)

// terminateGroup sends SIGKILL to the process group led by pid, so a command
// that started its own children cannot outlive the invocation's timeout or
// cancellation.
func terminateGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// sysProcAttrKill places a child in its own process group.
func sysProcAttrKill() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
