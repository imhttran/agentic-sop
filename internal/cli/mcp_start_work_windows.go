//go:build windows

package cli

import (
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
