//go:build !windows

package projectlock

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock, blocking until it is granted. The
// kernel associates the lock with the open file description and releases it when
// the descriptor is closed or the process exits, so a crashed holder never leaves
// a stale lock behind.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// unlockFile releases the advisory lock.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
