// Package procgroup makes an exec.Cmd's cancellation kill the whole process
// tree it started, not only the direct child.
package procgroup

import (
	"os/exec"
	"time"
)

// waitDelay bounds how long Wait blocks on the output pipes after the kill, so
// an orphan that kept a pipe open cannot stall the caller past its deadline.
const waitDelay = 2 * time.Second

// Bind starts cmd in its own process group and, when cmd's context ends, kills
// the group. On platforms without process groups only the child is killed.
// cmd must come from exec.CommandContext.
// ponytail: the exec/process-group binding is copied into security, toolharness,
// agent, evaltarget, controller/sopclient and decision/commandprofile; each copy
// can drift. Revisit when a change to waitDelay, SysProcAttr, killGroup or
// cmd.Cancel has to land in more than one copy: consolidate here first.
func Bind(cmd *exec.Cmd) {
	cmd.WaitDelay = waitDelay
	cmd.SysProcAttr = sysProcAttr()
	cmd.Cancel = func() error {
		if err := killGroup(cmd.Process.Pid); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
