//go:build unix

package toolharness

import (
	"errors"
	"syscall"
)

// terminateGroup sends SIGKILL to the process group led by pid, so a command
// that started its own children cannot survive a timeout or cancellation.
func terminateGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil // the group already exited
	}
	return err
}

// sysProcAttrKill returns a SysProcAttr that places a child in its own process
// group, so the whole group can be terminated with the command.
func sysProcAttrKill() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
