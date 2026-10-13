//go:build !windows

package verifcache

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on f. It blocks until the lock is
// acquired, so concurrent shared-store writers serialize their read-modify-write.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// unlockFile releases the advisory lock on f.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
