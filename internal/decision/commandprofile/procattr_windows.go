//go:build windows

package commandprofile

import (
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// setProcGroup is a no-op on Windows: Windows has no POSIX process groups. The
// process tree is bounded instead by a Job Object assigned in afterStart.
func setProcGroup(_ *exec.Cmd) {}

// jobFor maps a started process id to its kill-on-close Job Object.
var (
	jobMu  sync.Mutex
	jobFor = map[int]windows.Handle{}
)

// afterStart creates a Job Object configured to kill the whole tree when closed
// (or terminated) and assigns the started adapter to it. If any step fails, no job
// is recorded and killProcessTree falls back to killing the direct child.
func afterStart(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(h)
		_ = windows.CloseHandle(job)
		return
	}
	_ = windows.CloseHandle(h)
	jobMu.Lock()
	jobFor[cmd.Process.Pid] = job
	jobMu.Unlock()
}

// killProcessTree terminates the adapter and every descendant by terminating its
// Job Object, falling back to killing the direct child when no job was assigned.
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	jobMu.Lock()
	job, ok := jobFor[cmd.Process.Pid]
	if ok {
		delete(jobFor, cmd.Process.Pid)
	}
	jobMu.Unlock()
	if ok {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
		return
	}
	_ = cmd.Process.Kill()
}

// releaseJob closes and forgets the Job Object on the NORMAL-completion path, so a
// successful invocation does not leak the handle. Closing the job is harmless once
// the process has exited, and KILL_ON_JOB_CLOSE still reclaims any surviving child.
func releaseJob(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	jobMu.Lock()
	job, ok := jobFor[cmd.Process.Pid]
	if ok {
		delete(jobFor, cmd.Process.Pid)
	}
	jobMu.Unlock()
	if ok {
		_ = windows.CloseHandle(job)
	}
}
