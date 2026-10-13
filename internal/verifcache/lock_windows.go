//go:build windows

package verifcache

import "os"

// lockFile is a no-op on Windows, which has no flock. The atomic rename in
// atomicWrite remains the primary integrity guarantee; locking is only an
// optimization to avoid lost updates across concurrent writers.
func lockFile(f *os.File) error { return nil }

// unlockFile is a no-op on Windows.
func unlockFile(f *os.File) error { return nil }
