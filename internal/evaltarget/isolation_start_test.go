package evaltarget

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// EnvIsolationRequired turns the skip below into a failure on hosts where the
// boundary must be enforced, mirroring dqharness's SOP_REPLAY_ISOLATION_REQUIRED.
const EnvIsolationRequired = "SOP_EVAL_ISOLATION_REQUIRED"

// sandboxProbeTimeout bounds the one-shot start probe.
const sandboxProbeTimeout = 30 * time.Second

// requireSandbox skips a boundary test unless the OS sandbox can actually run a
// command on this host. A backend that exists but cannot start is unusable: macOS
// Seatbelt refuses sandbox_apply inside another sandbox, and nested bubblewrap
// cannot mount /proc once a user and PID namespace are both unshared. The probe
// runs the same builder the tests use, so it reflects the real argument list
// instead of guessing. Production isolation is never skipped; it stays
// fail-closed.
func requireSandbox(t *testing.T) {
	t.Helper()
	err := sandboxBlocker()
	if err == nil {
		return
	}
	if v := strings.TrimSpace(os.Getenv(EnvIsolationRequired)); v == "1" || strings.EqualFold(v, "true") {
		t.Fatalf("required eval isolation cannot run here: %v", err)
	}
	t.Skipf("OS-level isolation cannot run here: %v", err)
}

var (
	sandboxOnce     sync.Once
	sandboxProbeErr error
)

// sandboxBlocker reports why the platform sandbox cannot run a command, or nil
// when it can. The result is computed once per test binary.
func sandboxBlocker() error {
	sandboxOnce.Do(func() { sandboxProbeErr = probeSandbox() })
	return sandboxProbeErr
}

func probeSandbox() error {
	if !IsolationAvailable() {
		return errors.New("the platform sandbox is not available")
	}
	root, err := os.MkdirTemp("", "sop-probe-")
	if err != nil {
		return fmt.Errorf("create probe root: %w", err)
	}
	defer os.RemoveAll(root)
	ctx, cancel := context.WithTimeout(context.Background(), sandboxProbeTimeout)
	defer cancel()
	cmd, err := IsolateCommand(ctx, root, []string{"true"}, os.Environ(), root)
	if err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s cannot run a sandbox here: %v: %s (a nested OS sandbox is the usual cause, and it is not supported)",
			cmd.Path, err, strings.TrimSpace(string(out)))
	}
	return nil
}
