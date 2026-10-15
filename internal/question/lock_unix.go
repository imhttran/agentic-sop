//go:build !windows

package question

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on f. It blocks until the lock is
// acquired, so concurrent writers serialize their read-modify-write.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// unlockFile releases the advisory lock on f.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// tryLockFile attempts an exclusive advisory lock without blocking; it returns an
// error when the lock is already held, so a second executor stands down.
func tryLockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
