package testrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// execute runs command through the shell in dir.
//
// Verification commands are trusted project configuration and commonly need
// shell syntax (e.g. "go test ./...", "docker compose ... up"), so a shell is
// used here deliberately — unlike the Git adapter, whose arguments are
// structured by SOP. The command string is never built from untrusted input.
//
// Return contract:
//   - err == nil, exitCode == 0   → the command exited successfully
//   - err == nil, exitCode != 0   → the command exited non-zero (verification failure)
//   - err == ctx.Err(), code == -1 → canceled or deadline exceeded
//   - err != nil, exitCode == -1  → the check could not start (infrastructure error)
func execute(ctx context.Context, dir string, env map[string]string, command string) (exitCode int, stdout, stderr string, err error) {
	fi, statErr := os.Stat(dir)
	if statErr != nil {
		return -1, "", "", statErr
	}
	if !fi.IsDir() {
		return -1, "", "", fmt.Errorf("working directory %q is not a directory", dir)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = buildEnv(env)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout, stderr = outBuf.String(), errBuf.String()

	if ctxErr := ctx.Err(); ctxErr != nil {
		return -1, stdout, stderr, ctxErr
	}
	if runErr == nil {
		return 0, stdout, stderr, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), stdout, stderr, nil
	}
	return -1, stdout, stderr, runErr
}

// buildEnv returns the child environment: the current environment with any
// overrides applied last. It never logs or persists the environment.
func buildEnv(overrides map[string]string) []string {
	base := os.Environ()
	if len(overrides) == 0 {
		return base
	}

	env := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if _, ok := overrides[key]; ok {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}
