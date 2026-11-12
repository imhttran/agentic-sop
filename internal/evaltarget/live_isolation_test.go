//go:build darwin

package evaltarget

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runIsolated runs argv under the default or opt-in live sandbox and returns its
// combined output and exit error.
func runIsolated(t *testing.T, live bool, root string, argv []string, env []string) (string, error) {
	t.Helper()
	ctx := context.Background()
	var (
		cmd *exec.Cmd
		err error
	)
	if live {
		cmd, err = IsolateLiveCommand(ctx, root, argv, env, root)
	} else {
		cmd, err = IsolateCommand(ctx, root, argv, env, root)
	}
	if err != nil {
		t.Fatalf("build isolation command: %v", err)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestLiveIsolationDarwin verifies the opt-in live sandbox at the OS boundary:
// exactly the model endpoint is reachable, every other network destination is
// denied, filesystem reads outside the root fail, secret environment variables
// are scrubbed, and the default sandbox still denies all network.
func TestLiveIsolationDarwin(t *testing.T) {
	requireSandbox(t)
	root := t.TempDir()
	env := IsolateEnv(append(os.Environ(), "OPENAI_API_KEY=sk-canary-should-not-appear", "SOP_LIVE_CANARY=visible"), "")

	nc := func(host, port string) (string, error) {
		return runIsolated(t, true, root, []string{"/usr/bin/nc", "-z", "-G", "2", host, port}, env)
	}

	if out, err := nc("127.0.0.1", "11434"); err != nil {
		t.Fatalf("model endpoint must be reachable: %v\n%s", err, out)
	}
	if out, err := nc("127.0.0.1", "11435"); err == nil {
		t.Fatalf("another loopback port must be denied\n%s", out)
	}
	if out, err := nc("1.1.1.1", "443"); err == nil {
		t.Fatalf("a non-loopback destination must be denied\n%s", out)
	}

	// The default sandbox is unchanged: it must still deny the model endpoint.
	if out, err := runIsolated(t, false, root, []string{"/usr/bin/nc", "-z", "-G", "2", "127.0.0.1", "11434"}, env); err == nil {
		t.Fatalf("the default isolation must deny all network\n%s", out)
	}

	// Filesystem restrictions: a file outside the sandbox root is unreadable.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(wd, "..", "..", "go.mod")
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside sentinel: %v", err)
	}
	if out, err := runIsolated(t, true, root, []string{"/bin/cat", outside}, env); err == nil {
		t.Fatalf("a file outside the root must be unreadable\n%s", out)
	}

	// Environment scrubbing: the secret is removed, a non-secret marker survives.
	out, err := runIsolated(t, true, root, []string{"/usr/bin/env"}, env)
	if err != nil {
		t.Fatalf("env probe: %v\n%s", err, out)
	}
	if strings.Contains(out, "sk-canary-should-not-appear") {
		t.Fatalf("secret environment variable reached the isolated child")
	}
	if !strings.Contains(out, "SOP_LIVE_CANARY=visible") {
		t.Fatalf("non-secret environment variable did not reach the isolated child")
	}
}
