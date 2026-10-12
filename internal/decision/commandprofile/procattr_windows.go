//go:build windows

package commandprofile

import "os/exec"

// setProcGroup is a no-op on Windows: Windows has no POSIX process groups, so the
// adapter is started normally and CommandContext-style cancellation terminates the
// direct child (see killProcessTree). A bounded adapter that does not fork is fully
// covered; a forking adapter on Windows is a documented, scoped limitation.
func setProcGroup(_ *exec.Cmd) {}

// killProcessTree terminates the adapter process. Descendants are not covered on
// Windows; this is a documented platform-scoped behavior.
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
