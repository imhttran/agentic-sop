//go:build !windows

package cli

import (
	"path/filepath"
	"syscall"
	"testing"
)

// createUnsupportedEntry creates a filesystem entry the content fingerprint
// deliberately rejects (a named pipe), so a fingerprint failure can be tested
// deterministically. It reports false when the platform cannot create one.
func createUnsupportedEntry(t *testing.T, dir string) bool {
	t.Helper()
	if err := syscall.Mkfifo(filepath.Join(dir, "content-identity.fifo"), 0o600); err != nil {
		t.Logf("mkfifo unavailable: %v", err)
		return false
	}
	return true
}
