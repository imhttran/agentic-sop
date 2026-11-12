//go:build darwin

package toolharness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// macOS counterpart of TestLinuxIsolationSAFECommandCredentialRead. The R3
// residual is the SAFE `go test` vector: `go test` is SAFE, so model-written
// test code is ordinary authorized work and the credential read happens in a
// process the command policy already allowed. This pins all three legs:
//
//   - isolation off (the shipped default): the canary reaches the raw command
//     output, so the harness cannot claim any process-level protection;
//   - isolation on: the read is denied at the process level, the canary is
//     absent from the raw output, and an ordinary neighbour still reads;
//   - a pre-existing hardlink alias with an ordinary name stays readable, the
//     documented inode residual (Option D is Linux-scoped; this records that
//     macOS behaves the same way).
//
// Synthetic canaries only; the fake HOME is a temp dir under the test root.
func TestDarwinIsolationSAFECommandCredentialRead(t *testing.T) {
	needSandbox(t)
	const canary = "sop-darwin-safe-command-canary-4d2"
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", canary)
	writeRepoFile(t, home, "notes.txt", "ordinary-home-marker")
	alias := filepath.Join(home, "alias.txt")
	if err := os.Link(filepath.Join(home, ".ssh", "probe"), alias); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	if err := os.Symlink(filepath.Join(home, ".ssh", "probe"), filepath.Join(home, "link.txt")); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, root, "go.mod", "module cred\n\ngo 1.27\n")
	writeRepoFile(t, root, "cred/cred_test.go", safeCommandCredentialFixture)
	// Pin the off leg: an inherited override or a future default change must not
	// silently turn this run sandboxed.
	t.Setenv(IsolationEnv, "off")
	t.Setenv("HOME", home)
	ctx := context.Background()
	credentialCmd := "go test ./cred -run TestCredentialRead -v"

	// Isolation off: the shipped default exposes the credential through a command
	// the policy already classified SAFE. This is the R3 exposure, measured rather
	// than asserted.
	off := New(root, DefaultConfig(), NewAuditLog(0))
	out, err := off.Run(ctx, ToolRunCommand, map[string]any{"command": credentialCmd})
	if err != nil {
		t.Fatalf("the SAFE command did not run with isolation off: %v (%s)", err, out)
	}
	if !strings.Contains(out, canary) {
		t.Fatalf("isolation off no longer exposes the credential; the R3 evidence needs updating: %q", out)
	}

	onCfg := DefaultConfig()
	onCfg.IsolateCommands = true
	on := New(root, onCfg, NewAuditLog(0))

	out, err = on.Run(ctx, ToolRunCommand, map[string]any{"command": credentialCmd})
	if err != nil {
		t.Fatalf("the SAFE command did not run: %v (%s)", err, out)
	}
	if strings.Contains(out, canary) {
		t.Fatalf("the raw SAFE command output exposed the credential: %q", out)
	}
	// Require the fixture's own failure line, not just any error text, so a broken
	// `go` inside the profile cannot satisfy the denial leg.
	if !strings.Contains(out, "read credential:") {
		t.Fatalf("the credential read failed for an unrelated reason: %q", out)
	}
	if !strings.Contains(out, "operation not permitted") && !strings.Contains(out, "no such file or directory") {
		t.Fatalf("the credential read was not denied at the process level: %q", out)
	}

	out, err = on.Run(ctx, ToolRunCommand, map[string]any{"command": "go test ./cred -run TestOrdinaryRead -v"})
	if err != nil {
		t.Fatalf("ordinary SAFE command failed: %v (%s)", err, out)
	}
	if !strings.Contains(out, "ordinary-home-marker") {
		t.Fatalf("ordinary SAFE command did not read the positive control: %q", out)
	}

	// A symlink alias resolves to the denied path, so it is denied too.
	out, err = on.Run(ctx, ToolRunCommand, map[string]any{"command": "go test ./cred -run TestSymlinkRead -v"})
	if err != nil {
		t.Fatalf("the SAFE symlink command did not run: %v (%s)", err, out)
	}
	if strings.Contains(out, canary) {
		t.Fatalf("a symlink alias exposed the credential: %q", out)
	}
	if !strings.Contains(out, "read symlink:") {
		t.Fatalf("the symlink read failed for an unrelated reason: %q", out)
	}

	// Documented limit: the credential path is hidden, but a pre-existing hardlink
	// with an ordinary name shares the inode and remains readable through the SAFE
	// command. Pin the observation; a future inode policy must update this.
	out, err = on.Run(ctx, ToolRunCommand, map[string]any{"command": "go test ./cred -run TestHardlinkRead -v"})
	if err != nil {
		t.Fatalf("SAFE hardlink command did not run: %v (%s)", err, out)
	}
	if !strings.Contains(out, canary) {
		t.Fatalf("the pre-existing hardlink SAFE path no longer exposes the credential; update the macOS evidence: %q", out)
	}
	t.Logf("observed SAFE-command hardlink residual on macOS: the alias is readable inside the sandbox")
}
