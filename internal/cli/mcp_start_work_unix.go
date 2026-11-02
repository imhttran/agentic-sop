//go:build !windows

package cli

import (
	"errors"
	"syscall"
)

// detachedProcAttr starts the child in its own session, so it keeps running when
// the MCP server (and the client that launched it) exits.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// workPIDAlive reports whether pid names a live process. EPERM means it exists
// but belongs to another user, which still counts as alive.
func workPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// signalWorkStop stops one started job's child. detachedProcAttr starts the
// child with Setsid, so the child is its own session and process-group leader
// and -pid addresses the whole group: no helper survives the stop. force selects
// SIGKILL. A child that is already gone is not an error.
func signalWorkStop(pid int, force bool) error {
	if pid <= 0 {
		return nil
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := syscall.Kill(-pid, sig); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil // already gone
		}
		return err
	}
	return nil
}
