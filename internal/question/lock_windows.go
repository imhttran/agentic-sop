//go:build windows

package question

import "os"

// lockFile fails closed on Windows, which has no flock in the standard library and
// therefore cannot provide the advisory lock the durable store's read-modify-write
// needs. Failing closed (rather than silently degrading to a no-op) makes the
// cross-process guarantee explicit: mutation, including answer submission, returns
// ErrLockUnsupported, while reads remain available and the atomic rename still keeps
// the document well-formed.
func lockFile(f *os.File) error { return ErrLockUnsupported }

// unlockFile is never reached when lockFile fails closed, but is defined for parity.
func unlockFile(f *os.File) error { return nil }
