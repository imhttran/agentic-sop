//go:build !windows

package hipcompare

import (
	"os"
	"syscall"
)

// lockLedger takes an exclusive advisory lock so concurrent reservations across
// processes and output directories serialize the read-modify-write.
func lockLedger(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockLedger(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
