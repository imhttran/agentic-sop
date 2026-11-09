package evaltarget

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnvIsolation marks a process that was launched through the OS isolation
// boundary. The runner refuses to execute cases without it.
const EnvIsolation = "SOP_EVAL_ISOLATION"

// ErrIsolationUnavailable reports that the platform has no supported OS sandbox.
var ErrIsolationUnavailable = errors.New("evaltarget: no OS isolation mechanism is available (want macOS sandbox-exec or Linux bwrap)")

// RequireIsolation fails closed unless the current process was launched through
// IsolateCommand. Temporary directories alone are not an isolation boundary.
func RequireIsolation() error {
	if strings.TrimSpace(os.Getenv(EnvIsolation)) != "" {
		return nil
	}
	return errors.New("evaltarget: isolation boundary is not enforced; launch the evaluation through IsolateCommand")
}

// IsolationAvailable reports whether this platform can enforce the boundary.
func IsolationAvailable() bool { return isolationAvailable() }

// ErrLiveIsolationUnavailable reports that this platform cannot enforce the
// opt-in live inference restriction. Linux and other platforms keep live
// connectivity explicitly unsupported until an equivalent endpoint restriction
// is implemented and verified there.
var ErrLiveIsolationUnavailable = errors.New("evaltarget: live inference isolation is not supported on this platform")

// IsolateCommand builds a command that runs argv inside the OS sandbox rooted at
// root. The child working directory must resolve inside root, and its environment
// must be the isolated environment supplied by the caller. The default sandbox
// denies all network access.
func IsolateCommand(ctx context.Context, root string, argv []string, env []string, dir string) (*exec.Cmd, error) {
	return isolateCommandWith(ctx, root, argv, env, dir, isolationCommand)
}

// IsolateLiveCommand is the opt-in, evaluation-only variant used by the bounded
// live model path. It keeps the same filesystem and environment restrictions as
// IsolateCommand and adds exactly one permitted network destination: the fixed
// local inference endpoint on loopback. Every other network destination remains
// denied. It is never used by normal agent execution or the default case runner.
func IsolateLiveCommand(ctx context.Context, root string, argv []string, env []string, dir string) (*exec.Cmd, error) {
	return isolateCommandWith(ctx, root, argv, env, dir, liveIsolationCommand)
}

func isolateCommandWith(ctx context.Context, root string, argv []string, env []string, dir string, build func(context.Context, string, string, []string, []string) (*exec.Cmd, error)) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, errors.New("evaltarget: empty command")
	}
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("evaltarget: isolation root is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	dirAbs, err := confinedDir(rootAbs, dir)
	if err != nil {
		return nil, err
	}
	if env == nil {
		env = IsolateEnv(os.Environ(), "")
	}
	cmd, err := build(ctx, rootAbs, dirAbs, argv, env)
	if err != nil {
		return nil, err
	}
	cmd.Env = append(append([]string(nil), cmd.Env...), fmt.Sprintf("%s=%s", EnvIsolation, isolationKind()))
	return cmd, nil
}
