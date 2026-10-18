package security

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrUnavailable reports that a configured executable could not be started.
// Adapters map it to UNAVAILABLE, never to a clean scan.
var ErrUnavailable = errors.New("security: adapter executable unavailable")

// Executor runs one approved executable with an argument vector. No shell is
// involved. It must enforce ctx cancellation and return at most maxOut bytes on
// each stream.
type Executor interface {
	Run(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) (stdout, stderr []byte, exitCode int, err error)
}

// EnvExecutor is the optional extension used by adapters that must pass an
// explicit, non-secret environment value to a scanner. It exists because OSV's
// offline database location is an environment variable, not a CLI flag. An
// adapter that needs it fails closed when the executor does not support it.
type EnvExecutor interface {
	RunEnv(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) (stdout, stderr []byte, exitCode int, err error)
}

// execRunner is the production Executor.
type execRunner struct{}

const (
	// maxStderrBytes bounds captured diagnostics; adapters sanitize what they keep.
	maxStderrBytes = 16 << 10
	// waitDelay bounds I/O drain after a killed process so a child holding a pipe
	// cannot stall the scan past its deadline.
	waitDelay = 2 * time.Second
)

// Run executes argv directly in dir. A missing executable is ErrUnavailable. A
// cancelled or deadline-exceeded context returns ctx.Err(). A normal non-zero
// exit is returned with a nil error so the adapter can interpret its own exit
// contract.
func (execRunner) Run(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	return execRunner{}.run(ctx, dir, argv, nil, stdin, maxOut)
}

// RunEnv is Run with explicit additional environment entries. The entries are
// controlled by a built-in adapter, never by untrusted report content.
func (execRunner) RunEnv(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	return execRunner{}.run(ctx, dir, argv, env, stdin, maxOut)
}

func (execRunner) run(ctx context.Context, dir string, argv []string, extraEnv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, nil, -1, errors.New("security: empty command")
	}
	if maxOut <= 0 {
		maxOut = 1 << 20
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(scannerEnv(), extraEnv...)
	cmd.WaitDelay = waitDelay
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	stdout := newBoundedBuffer(maxOut)
	stderr := newBoundedBuffer(maxStderrBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return stdout.Bytes(), stderr.Bytes(), -1, ctxErr
	}
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode(), nil
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return stdout.Bytes(), stderr.Bytes(), -1, ErrUnavailable
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return stdout.Bytes(), stderr.Bytes(), -1, ErrUnavailable
	}
	return stdout.Bytes(), stderr.Bytes(), -1, err
}

// scannerEnv supplies only the environment a scanner needs to resolve its own
// runtime configuration. Credentials, provider tokens, and unrelated secrets are
// deliberately absent.
func scannerEnv() []string {
	allowed := []string{
		"PATH", "HOME", "USERPROFILE", "TMPDIR", "TEMP", "TMP",
		"SystemRoot", "COMSPEC", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR",
	}
	out := make([]string, 0, len(allowed))
	for _, key := range allowed {
		if value, ok := os.LookupEnv(key); ok {
			out = append(out, key+"="+value)
		}
	}
	if runtime.GOOS == "windows" && os.Getenv("SystemRoot") == "" {
		// Keep the child functional on Windows even when the caller sanitized its
		// environment; this is a path, not a credential.
		out = append(out, `SystemRoot=C:\Windows`)
	}
	return out
}

// boundedBuffer retains at most limit bytes and records overflow.
type boundedBuffer struct {
	buf      bytes.Buffer
	limit    int
	overflow bool
}

func newBoundedBuffer(limit int) *boundedBuffer { return &boundedBuffer{limit: limit} }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			_, _ = b.buf.Write(p[:room])
			b.overflow = true
		} else {
			_, _ = b.buf.Write(p)
		}
	} else if len(p) > 0 {
		b.overflow = true
	}
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *boundedBuffer) Overflow() bool { return b.overflow }
func (b *boundedBuffer) Len() int       { return b.buf.Len() }
func (b *boundedBuffer) String() string { return b.buf.String() }
