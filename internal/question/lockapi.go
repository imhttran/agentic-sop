package question

import (
	"errors"
	"os"
	"path/filepath"
)

// AcquireLock takes an exclusive advisory lock on path and returns a release function.
//
// It coordinates EXECUTION ownership, not the question store's read-modify-write: a
// worker holds this lock for the whole duration of resumed stage work, so a second
// worker cannot execute the same continuation concurrently even after the store lease
// expires. Because it is a different file from the store lock, holding it never blocks
// question creation or answer submission.
func AcquireLock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}

// TryAcquireLock attempts path's execution lock without blocking. It returns ok=false
// when another worker already holds it, so a second executor stands down instead of
// running the same continuation concurrently. A platform that cannot lock fails closed
// (ok=false with ErrLockUnsupported).
func TryAcquireLock(path string) (release func(), ok bool, err error) {
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return nil, false, mkErr
	}
	f, openErr := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if openErr != nil {
		return nil, false, openErr
	}
	if lockErr := tryLockFile(f); lockErr != nil {
		_ = f.Close()
		if errors.Is(lockErr, ErrLockUnsupported) {
			return nil, false, lockErr
		}
		return func() {}, false, nil // held by another worker
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, true, nil
}
