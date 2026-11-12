//go:build linux

package toolharness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Linux verification for the PR #378 run_command credential read-deny policy.
// The bubblewrap backend already masks the fixed home-credential paths; these
// tests execute the real sandbox and distinguish process-level denial from
// model-input redaction. Synthetic canaries only; the fake HOME is a temp dir.

const linuxCredentialCanary = "sop-linux-credential-canary-81"

func TestLinuxIsolationOffAllowsCredentialRead(t *testing.T) {
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", linuxCredentialCanary)
	t.Setenv("HOME", home)

	out, err := exec.Command("cat", filepath.Join(home, ".ssh", "probe")).CombinedOutput()
	if err != nil {
		t.Fatalf("unsandboxed credential read failed: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), linuxCredentialCanary) {
		t.Fatalf("unsandboxed read did not return the canary: %q", out)
	}
}

func TestLinuxIsolationPreExistingCredentialHardlink(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", linuxCredentialCanary)
	alias := filepath.Join(home, "alias.txt")
	if err := os.Link(filepath.Join(home, ".ssh", "probe"), alias); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	t.Setenv("HOME", home)

	out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(alias))
	if err == nil && strings.Contains(out, linuxCredentialCanary) {
		t.Logf("observed pre-existing hardlink residual: the alias is readable inside the sandbox")
		return
	}
	t.Fatalf("the pre-existing hardlink no longer exposes the credential; update the Linux evidence (err=%v out=%q)", err, out)
}

func TestLinuxIsolationDeniesCredentialSymlink(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", linuxCredentialCanary)
	link := filepath.Join(home, "alias-link")
	if err := os.Symlink(filepath.Join(home, ".ssh", "probe"), link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(link))
	if err == nil && strings.Contains(out, linuxCredentialCanary) {
		t.Fatalf("symlink alias exposed the credential: %q", out)
	}
}

func TestLinuxIsolationDeniesCredentialReadInDescendant(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", linuxCredentialCanary)
	t.Setenv("HOME", home)

	inner := "cat " + strconv.Quote(filepath.Join(home, ".ssh", "probe"))
	out, err := sandboxedOutput(t, root, "sh -c "+strconv.Quote(inner))
	if err == nil && strings.Contains(out, linuxCredentialCanary) {
		t.Fatalf("descendant process exposed the credential: %q", out)
	}
}

func TestLinuxIsolationAllowsRepositoryTemplates(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	writeRepoFile(t, root, ".env.example", "SOP_TEST_API_KEY=replace-me\n")
	writeRepoFile(t, root, ".env.sample", "SOP_TEST_API_KEY=replace-me\n")

	for _, name := range []string{".env.example", ".env.sample"} {
		out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(filepath.Join(root, name)))
		if err != nil || !strings.Contains(out, "replace-me") {
			t.Fatalf("%s was not readable inside the sandbox: out=%q err=%v", name, out, err)
		}
	}
}

func TestLinuxIsolationCancellationKillsChild(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	argv, env, _, cleanup, err := isolate(root, []string{"sh", "-c", "sleep 30"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = root
	cmd.Env = env

	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("sandboxed sleep completed despite the timeout")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("cancellation took %s, want a bounded stop", elapsed)
	}
}

// TestLinuxIsolationSAFECommandCredentialRead drives the real harness with a
// SAFE `go test` command. The sandbox must deny the credential read at the
// process level: the canary must be absent from the raw command output, not
// merely redacted later at the model boundary.
func TestLinuxIsolationSAFECommandCredentialRead(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", linuxCredentialCanary)
	writeRepoFile(t, home, "notes.txt", "ordinary-home-marker")
	alias := filepath.Join(home, "alias.txt")
	if err := os.Link(filepath.Join(home, ".ssh", "probe"), alias); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	writeRepoFile(t, root, "go.mod", "module cred\n\ngo 1.27\n")
	writeRepoFile(t, root, "cred/cred_test.go", safeCommandCredentialFixture)
	t.Setenv("HOME", home)
	t.Setenv("TMPDIR", "/tmp")

	cfg := DefaultConfig()
	cfg.IsolateCommands = true
	h := New(root, cfg, NewAuditLog(0))
	ctx := context.Background()

	out, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": "go test ./cred -run TestCredentialRead -v"})
	if err != nil {
		t.Fatalf("the SAFE command did not run: %v (%s)", err, out)
	}
	if strings.Contains(out, linuxCredentialCanary) {
		t.Fatalf("the raw SAFE command output exposed the credential: %q", out)
	}
	if !strings.Contains(out, "no such file or directory") && !strings.Contains(out, "permission denied") {
		t.Fatalf("the credential read was not denied at the process level: %q", out)
	}

	out, err = h.Run(ctx, ToolRunCommand, map[string]any{"command": "go test ./cred -run TestOrdinaryRead -v"})
	if err != nil {
		t.Fatalf("ordinary SAFE command failed: %v (%s)", err, out)
	}
	if !strings.Contains(out, "ordinary-home-marker") {
		t.Fatalf("ordinary SAFE command did not read the positive control: %q", out)
	}

	// Documented limit: the credential path is hidden, but a pre-existing hardlink
	// with an ordinary name shares the inode and remains readable through the
	// SAFE command. Pin the observation; a future inode policy must update this.
	out, err = h.Run(ctx, ToolRunCommand, map[string]any{"command": "go test ./cred -run TestHardlinkRead -v"})
	if err != nil {
		t.Fatalf("SAFE hardlink command did not run: %v (%s)", err, out)
	}
	if !strings.Contains(out, linuxCredentialCanary) {
		t.Fatalf("the pre-existing hardlink SAFE path no longer exposes the credential; update the Linux evidence: %q", out)
	}
	t.Logf("observed SAFE-command hardlink residual: the alias is readable inside the sandbox")
}
