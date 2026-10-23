package evaltarget

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CommandRequest is one child command a case runs.
type CommandRequest struct {
	Argv []string
	// Dir is the working directory. It must resolve inside Root.
	Dir string
	// Root is the authorized sandbox root. RunChild refuses a Dir outside it.
	Root string
	// Env is the complete child environment. A nil value is replaced with an
	// isolated copy of the parent environment, so production secrets never reach
	// the child by accident.
	Env []string
	// Timeout bounds the command. A zero value inherits the case timeout.
	Timeout time.Duration
}

// CommandResult is the captured outcome of one child command.
type CommandResult struct {
	Output    string
	Exit      int
	TimedOut  bool
	Cancelled bool
}

// RunChild runs a child command in its own process group and returns when it exits
// or the parent context ends. On timeout or cancellation the whole process group is
// killed, so no grandchild survives the case. It does not kill the group after a
// normal exit.
func RunChild(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if len(req.Argv) == 0 {
		return CommandResult{Exit: -1}, errors.New("evaltarget: empty command")
	}
	dir, err := confinedDir(req.Root, req.Dir)
	if err != nil {
		return CommandResult{Exit: -1}, err
	}
	env := req.Env
	if env == nil {
		env = IsolateEnv(os.Environ(), "")
	}

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	cmd := exec.Command(req.Argv[0], req.Argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = sysProcAttrKill()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return CommandResult{Exit: -1}, fmt.Errorf("evaltarget: start command: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var runErr error
	select {
	case runErr = <-done:
	case <-runCtx.Done():
		// This is the one cancellation mechanism: terminate the whole process
		// group, then wait for the direct child to be reaped.
		_ = terminateGroup(cmd.Process.Pid)
		runErr = <-done
	}

	res := CommandResult{Output: buf.String(), Exit: processExitCode(runErr)}
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		res.Cancelled = true
		res.Exit = -1
		return res, context.Canceled
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		res.TimedOut = true
		res.Exit = -1
		return res, errors.New("evaltarget: command timed out")
	case runErr != nil:
		return res, fmt.Errorf("evaltarget: command failed: %w", runErr)
	}
	return res, nil
}

// confinedDir resolves dir inside root and refuses a path outside it.
func confinedDir(root, dir string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("evaltarget: command root is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = resolved
	}
	if strings.TrimSpace(dir) == "" {
		dir = rootAbs
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(dirAbs); err == nil {
		dirAbs = resolved
	}
	rel, err := filepath.Rel(rootAbs, dirAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("evaltarget: command directory %q is outside the sandbox %q", dirAbs, rootAbs)
	}
	return dirAbs, nil
}

// processExitCode returns a command's exit code, or -1 on an execution failure.
func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
