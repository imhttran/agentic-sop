package ragclient

import (
	"bytes"
	"context"
	"os/exec"
)

// execRunner executes the producer as a child process with no shell. The
// executable is invoked directly (an absolute path), stdin carries the request
// JSON, stdout carries the response JSON, and stderr carries diagnostics. The
// process is killed when ctx is cancelled (exec.CommandContext).
type execRunner struct{}

// Run implements Runner.
func (execRunner) Run(ctx context.Context, executable string, stdin []byte, maxOut int) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, executable)
	cmd.Stdin = bytes.NewReader(stdin)
	out := &limitedBuffer{limit: maxOut}
	errOut := &limitedBuffer{limit: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = errOut
	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out.Bytes(), errOut.Bytes(), ctxErr
	}
	if err != nil {
		return out.Bytes(), errOut.Bytes(), err
	}
	return out.Bytes(), errOut.Bytes(), nil
}

// limitedBuffer is a bytes.Buffer that accepts at most limit bytes and silently
// discards the rest, so a runaway producer cannot exhaust memory. A caller
// detects truncation by comparing the returned length against the limit.
type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte { return b.buf.Bytes() }
