//go:build !windows

package executionlease

import (
	"errors"
	"syscall"
)

// alive reports whether pid names a running process on this host. A process this
// user may not signal still exists, so EPERM counts as alive. known is false when
// the probe could not answer (for example a permission or signal limitation).
func alive(pid int) (live, known bool) {
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil:
		return true, true
	case errors.Is(err, syscall.EPERM):
		return true, true
	case errors.Is(err, syscall.ESRCH):
		return false, true
	default:
		return false, false
	}
}
