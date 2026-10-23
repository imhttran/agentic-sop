//go:build !unix

package evaltarget

import (
	"os"
	"syscall"
)

// terminateGroup sends a kill to the child on a platform without process groups.
func terminateGroup(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// sysProcAttrKill returns nil: this platform has no process groups.
func sysProcAttrKill() *syscall.SysProcAttr {
	return nil
}

// processAlive reports whether pid is still running.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
