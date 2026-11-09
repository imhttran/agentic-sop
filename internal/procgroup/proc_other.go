//go:build !unix

package procgroup

import (
	"errors"
	"syscall"
)

// killGroup reports failure so Bind falls back to killing the direct child: this
// platform has no process groups.
func killGroup(int) error { return errors.New("procgroup: no process groups") }

func sysProcAttr() *syscall.SysProcAttr { return nil }
