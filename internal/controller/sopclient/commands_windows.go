//go:build windows

package sopclient

import "os/exec"

// configureCancellation leaves Windows on exec.CommandContext's default, which
// kills the `sop` process only. Windows has no POSIX process groups, and the
// Job Object equivalent (internal/decision/commandprofile) is more machinery than
// this fix warrants, so descendants may still outlive a cancelled command here.
func configureCancellation(_ *exec.Cmd) {}
