//go:build windows

package cli

import "testing"

// createUnsupportedEntry is unavailable on Windows; the fingerprint-failure test
// skips there.
func createUnsupportedEntry(t *testing.T, dir string) bool { return false }
