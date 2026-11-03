//go:build windows

package executionlease

// alive reports that pid liveness cannot be probed on Windows. The caller falls
// back to the heartbeat, so a lease on this host can still become stale.
func alive(int) (live, known bool) { return false, false }
