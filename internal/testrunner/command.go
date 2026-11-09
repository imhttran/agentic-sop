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

// maxOutputBytes bounds the stdout and stderr captured from one validation
// command. It matches the provider response bound so a streaming check cannot
// allocate unbounded memory; a larger stream is drained and marked truncated.
const maxOutputBytes = 10 << 20

// execute runs command through the shell in dir. maxBytes bounds each captured
// stream at the read boundary; a non-positive value uses maxOutputBytes.
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
func execute(ctx context.Context, dir string, env map[string]string, command string, maxBytes int) (exitCode int, stdout, stderr string, err error) {
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

	if maxBytes <= 0 {
		maxBytes = maxOutputBytes
	}
	outBuf := newLimitedBuffer(maxBytes)
	errBuf := newLimitedBuffer(maxBytes)
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf

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

// limitedBuffer retains a bounded head and tail while counting every byte
// written, so a validation command cannot make the runner allocate its full
// output. The tail preserves trailing structured evidence (for example a
// `go test -json` package-completion event) that a prefix-only capture would
// lose. It implements io.Writer for os/exec.
type limitedBuffer struct {
	head    bytes.Buffer
	tail    []byte
	headMax int
	tailMax int
	total   int
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit <= 0 {
		limit = maxOutputBytes
	}
	tailMax := limit / 8
	if tailMax < 64 {
		tailMax = 64
	}
	if tailMax > limit {
		tailMax = limit
	}
	return &limitedBuffer{headMax: limit - tailMax, tailMax: tailMax}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	b.total += original
	if room := b.headMax - b.head.Len(); room > 0 {
		if len(p) > room {
			b.head.Write(p[:room])
			p = p[room:]
		} else {
			b.head.Write(p)
			return original, nil
		}
	}
	if len(p) > 0 {
		b.tail = append(b.tail, p...)
		if len(b.tail) > b.tailMax {
			b.tail = append(b.tail[:0], b.tail[len(b.tail)-b.tailMax:]...)
		}
	}
	return original, nil
}

func (b *limitedBuffer) String() string {
	truncated := b.total - b.head.Len() - len(b.tail)
	if truncated <= 0 {
		return b.head.String() + string(b.tail)
	}
	return b.head.String() + fmt.Sprintf("\n… [truncated %d bytes]\n", truncated) + string(b.tail)
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
