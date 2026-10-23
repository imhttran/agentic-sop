//go:build windows

package hipcompare

import "os"

// Windows has no flock equivalent in the standard library. The ledger is still
// written atomically under a single process; cross-process reservation locking is
// not available on Windows without an additional platform dependency.
func lockLedger(_ *os.File) error   { return nil }
func unlockLedger(_ *os.File) error { return nil }
