//go:build !unix

package security

import (
	"os"
	"syscall"
)

// terminateGroup kills the direct child on a platform without process groups.
func terminateGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
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
