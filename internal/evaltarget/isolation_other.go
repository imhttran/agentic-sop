//go:build !darwin && !linux

package evaltarget

import (
	"context"
	"os/exec"
)

func isolationAvailable() bool { return false }

func isolationKind() string { return "" }

func isolationCommand(context.Context, string, string, []string, []string) (*exec.Cmd, error) {
	return nil, ErrIsolationUnavailable
}
