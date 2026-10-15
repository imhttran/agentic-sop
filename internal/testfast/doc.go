// Package testfast covers the repository's `make test-fast` developer tooling
// (scripts/checks/test-fast.sh).
//
// The tests here exercise the shell wrapper directly, using its documented test
// seams (SOP_TEST_PLATFORM, SOP_TEST_RAM_DIR, SOP_TEST_GO_BIN) so Linux and
// macOS behavior can be verified deterministically on any host, and a mock `go`
// command so no nested test run is ever spawned. They require a POSIX shell and
// are skipped on Windows.
package testfast
