//go:build windows

package executionlease

// alive reports whether pid names a running process. Windows has no portable
// zero-signal probe, so a lease on this host is treated as live and only the
// heartbeat can make it stale. That errs towards refusing a start, never towards
// letting a second execution in.
func alive(int) bool { return true }
