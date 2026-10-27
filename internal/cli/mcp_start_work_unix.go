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
