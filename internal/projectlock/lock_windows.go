//go:build windows

package projectlock

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on a one-byte range at the start of the file,
// blocking until it is granted. Unlike the question store's fail-closed fallback,
// this package uses the real Windows primitive so project state writers keep
// working on Windows instead of regressing to a refusal.
func lockFile(f *os.File) error {
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, 1, 0,
		new(windows.Overlapped),
	)
}

// unlockFile releases the locked byte range.
func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0, 1, 0,
		new(windows.Overlapped),
	)
}
