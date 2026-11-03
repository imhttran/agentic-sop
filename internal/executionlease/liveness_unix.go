//go:build !windows

package executionlease

import (
	"errors"
	"syscall"
)

// alive reports whether pid names a running process on this host. A process this
// user may not signal still exists, so EPERM counts as alive.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
