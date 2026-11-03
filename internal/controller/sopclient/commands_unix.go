//go:build !windows

package sopclient

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureCancellation makes a timeout or cancellation reach every process the
// SOP command started, not just `sop` itself. The command leads its own process
// group, so Cancel can signal the whole tree: agents, git and validation commands
// spawned by SOP die with it instead of still changing the repo after the
// controller has already reported a timeout.
func configureCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			// Nothing left to stop: the group is already gone (ESRCH).
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
}
