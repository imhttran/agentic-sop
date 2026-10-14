package sopclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Commander runs SOP CLI verbs against a project root (FR-8).
type Commander struct {
	bin     string
	timeout time.Duration
}

// NewCommander returns a Commander that shells out to the SOP CLI.
func NewCommander(bin string, timeout time.Duration) *Commander {
	if bin == "" {
		bin = "sop"
	}
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	return &Commander{bin: bin, timeout: timeout}
}

// Exec runs `sop <verb> [args...]` in dir and returns its combined output. It
// defaults SOP_ACTIVITY=on (observer-only) so runs persist activity.jsonl; that
// is the only environment the controller adds.
func (c *Commander) Exec(ctx context.Context, dir, verb string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.bin, append([]string{verb}, args...)...)
	cmd.Dir = dir
	cmd.Env = activityEnv(os.Environ())
	var buf bytes.Buffer
	// Tee the child output into the buffered result and, when a live sink is attached
	// to the context, into it too, so the dashboard shows progress while a long
	// `sop run`/`sop resume` is still executing. Stdout and Stderr share one writer
	// value, so os/exec serializes the writes (no concurrent Write).
	var w io.Writer = &buf
	if sink := progressFrom(ctx); sink != nil {
		w = io.MultiWriter(&buf, sink)
	}
	cmd.Stdout = w
	cmd.Stderr = w

	err := cmd.Run()
	out := buf.String()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("sop %s timed out after %s", verb, c.timeout)
	}
	if err != nil {
		return out, fmt.Errorf("sop %s: %w", verb, err)
	}
	return out, nil
}

func activityEnv(env []string) []string {
	for _, kv := range env {
		if strings.HasPrefix(kv, "SOP_ACTIVITY=") {
			return env
		}
	}
	return append(env, "SOP_ACTIVITY=on")
}

// progressKey carries an optional live-output sink in the context.
type progressKey struct{}

// WithProgress returns a context that tees a command's output into sink as it runs.
// The sink receives raw bytes; it must be safe for the sequential Write calls
// os/exec makes when Stdout and Stderr share one writer. A command run without a
// sink behaves exactly as before.
func WithProgress(ctx context.Context, sink io.Writer) context.Context {
	return context.WithValue(ctx, progressKey{}, sink)
}

// ProgressWriter returns the live-output sink attached to ctx, or nil when none is set.
func ProgressWriter(ctx context.Context) io.Writer { return progressFrom(ctx) }

func progressFrom(ctx context.Context) io.Writer {
	if w, ok := ctx.Value(progressKey{}).(io.Writer); ok {
		return w
	}
	return nil
}
