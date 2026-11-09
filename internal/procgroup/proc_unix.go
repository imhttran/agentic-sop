//go:build unix

package procgroup

import (
	"errors"
	"syscall"
)

func killGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil // the group already exited
	}
	return err
}

func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
