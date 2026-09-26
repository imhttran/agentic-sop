package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CommandError describes a failed git invocation.
type CommandError struct {
	Args     []string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *CommandError) Error() string {
	msg := fmt.Sprintf("git %s", strings.Join(e.Args, " "))
	if e.Stderr != "" {
		return fmt.Sprintf("%s: %s", msg, e.Stderr)
	}
	return fmt.Sprintf("%s: %v", msg, e.Err)
}

// Unwrap exposes the underlying process error.
func (e *CommandError) Unwrap() error { return e.Err }

// run executes git with explicit arguments in dir and returns captured stdout.
// It never goes through a shell, so arguments are not subject to interpolation,
// and it disables interactive prompting.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		exitCode := 0
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return "", &CommandError{
			Args:     args,
			ExitCode: exitCode,
			Stderr:   strings.TrimSpace(stderr.String()),
			Err:      err,
		}
	}

	return stdout.String(), nil
}
