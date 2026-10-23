//go:build unix

package evaltarget

import (
	"syscall"
)

// terminateGroup sends SIGKILL to the process group led by pid, so a child that
// started its own children cannot survive the case's timeout or cancellation.
func terminateGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

// sysProcAttrKill returns a SysProcAttr that places a child in its own process
// group, so the whole group can be killed on timeout or cancellation.
func sysProcAttrKill() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// processAlive reports whether pid is still running. Signal 0 probes existence
// without delivering a signal.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
