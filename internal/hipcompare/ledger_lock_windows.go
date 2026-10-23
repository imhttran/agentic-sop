//go:build windows

package hipcompare

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockLedger takes an exclusive lock on the ledger file so concurrent
// reservations across processes and output directories serialize the
// read-modify-write.
func lockLedger(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &ol)
}

func unlockLedger(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
