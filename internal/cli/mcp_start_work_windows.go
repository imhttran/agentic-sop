//go:build windows

package cli

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// detachedProcAttr starts the child in its own process group with no console,
// so it keeps running when the MCP server exits.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
}

// stillActive is the exit code Windows reports for a running process.
const stillActive = 259

// workPIDAlive reports whether pid names a live process.
func workPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// signalWorkStop terminates one started job's child. A detached Windows child has
// no console to receive a control event, and Go's process groups carry no signal
// channel, so termination is the only stop this build can confirm. force has no
// separate meaning here; both paths terminate, and the bounded wait in
// cancelWorkJob confirms the result. A child that is already gone is not an error.
func signalWorkStop(pid int, force bool) error {
	if pid <= 0 {
		return nil
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_FOUND) {
			return nil // already gone
		}
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_FOUND) {
			return nil
		}
		return err
	}
	return nil
}
