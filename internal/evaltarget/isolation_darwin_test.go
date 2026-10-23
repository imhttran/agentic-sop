//go:build darwin

package evaltarget

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestDarwinIsolationBlocksOrdinaryWorkspace proves the isolation boundary is
// enforced by the OS, not by a temporary directory convention: a sandboxed
// command can write inside its case root but cannot write to the ordinary
// workspace or read its SOP state.
func TestDarwinIsolationBlocksOrdinaryWorkspace(t *testing.T) {
	if !IsolationAvailable() {
		t.Skip("sandbox-exec is not available")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	env := IsolateEnv(os.Environ(), filepath.Join(root, "state"))

	inside, err := IsolateCommand(context.Background(), root, []string{sh, "-c", "printf inside > inside.txt"}, env, repo)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := inside.CombinedOutput(); err != nil {
		t.Fatalf("inside write failed: %v: %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(repo, "inside.txt")); err != nil {
		t.Fatalf("inside write was not applied: %v", err)
	}

	workspace, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(workspace, "isolation-probe-do-not-commit.txt")
	_ = os.Remove(probe)
	outside, err := IsolateCommand(context.Background(), root, []string{sh, "-c", "printf outside > " + probe}, env, repo)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := outside.CombinedOutput(); err == nil {
		t.Fatalf("outside write succeeded inside the sandbox: %s", out)
	}
	if _, err := os.Stat(probe); !os.IsNotExist(err) {
		_ = os.Remove(probe)
		t.Fatalf("ordinary workspace was modified: %v", err)
	}

	state := filepath.Join(workspace, ".agent-sdlc", "state.db")
	if _, err := os.Stat(state); err == nil {
		read, err := IsolateCommand(context.Background(), root, []string{sh, "-c", "cat " + state}, env, repo)
		if err != nil {
			t.Fatal(err)
		}
		if out, err := read.CombinedOutput(); err == nil {
			t.Fatalf("ordinary SOP state was readable inside the sandbox: %s", out)
		}
	}
}

// TestDarwinIsolationBlocksNetwork proves the sandbox cannot open a network
// connection to a listener that the unsandboxed process can reach.
func TestDarwinIsolationBlocksNetwork(t *testing.T) {
	if !IsolationAvailable() {
		t.Skip("sandbox-exec is not available")
	}
	nc, err := exec.LookPath("nc")
	if err != nil {
		t.Skip("nc is not available")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := exec.Command(nc, "-z", "-w", "1", "127.0.0.1", port).Run(); err != nil {
		t.Skipf("local nc probe is unavailable: %v", err)
	}

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := IsolateCommand(context.Background(), root, []string{nc, "-z", "-w", "1", "127.0.0.1", port}, IsolateEnv(os.Environ(), filepath.Join(root, "state")), repo)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("sandboxed network connection succeeded: %s", out)
	}
}

// TestDarwinIsolationStripsSecrets proves a production secret in the parent
// environment is not visible to the sandboxed child.
func TestDarwinIsolationStripsSecrets(t *testing.T) {
	if !IsolationAvailable() {
		t.Skip("sandbox-exec is not available")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	env := IsolateEnv([]string{"PATH=/usr/bin:/bin", "GITHUB_TOKEN=prod-token"}, filepath.Join(root, "state"))
	cmd, err := IsolateCommand(context.Background(), root, []string{sh, "-c", "env"}, env, repo)
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("env: %v: %s", err, out)
	}
	if strings.Contains(string(out), "prod-token") {
		t.Fatalf("production secret reached the sandbox: %s", out)
	}
	if !strings.Contains(string(out), EnvIsolation+"=") {
		t.Fatalf("isolation marker missing from child environment: %s", out)
	}
}
