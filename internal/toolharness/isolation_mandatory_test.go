package toolharness

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// W6 (M1 + G3 + G4) contract:
//   - every model-reachable execution runs inside the sandbox and is audited as such;
//   - a host with no usable backend refuses, and never runs unsandboxed;
//   - the model cannot author git metadata through the file tools.

// --- every model-reachable path is sandboxed --------------------------------

func TestGitToolsAreSandboxed(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	gitInitRepo(t, root)
	commitRepoFile(t, root, "f.txt", "x: y\n")
	writeRepoFile(t, root, "f.txt", "x: z\n")

	log := NewAuditLog(0)
	h := New(root, DefaultConfig(), log)
	for _, tool := range []string{ToolGitStatus, ToolGitDiff} {
		if out, err := h.Run(context.Background(), tool, nil); err != nil {
			t.Fatalf("%s: %v (%s)", tool, err, out)
		}
		if d := log.Records()[len(log.Records())-1].Detail; !strings.Contains(d, "isolate=") {
			t.Errorf("%s audit detail = %q, want a sandboxed run", tool, d)
		}
	}
}

// TestSafeBuildAndTestCannotBypassIsolation covers the code-execution channel the
// command allowlist cannot close: `go test`/`go build` are SAFE but run
// model-authored code, so each must carry the sandbox.
func TestSafeBuildAndTestCannotBypassIsolation(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	writeRepoFile(t, root, "go.mod", "module probe\n\ngo 1.21\n")
	writeRepoFile(t, root, "probe.go", "package probe\n")

	log := NewAuditLog(0)
	h := New(root, DefaultConfig(), log)
	for _, command := range []string{"go build ./...", "go vet ./...", "go test ./..."} {
		if out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": command}); err != nil {
			t.Fatalf("%s: %v (%s)", command, err, out)
		}
		if d := log.Records()[len(log.Records())-1].Detail; !strings.Contains(d, "isolate=") {
			t.Errorf("%q audit detail = %q, want a sandboxed run", command, d)
		}
	}
}

// --- a host without a backend refuses, never falls back --------------------

func TestGitToolsRefusedWithoutBackend(t *testing.T) {
	root := t.TempDir()
	gitInitRepo(t, root)
	commitRepoFile(t, root, "f.txt", "x: y\n")
	t.Setenv("PATH", t.TempDir()) // no sandbox backend resolvable
	log := NewAuditLog(0)
	h := New(root, DefaultConfig(), log)
	for _, tool := range []string{ToolGitStatus, ToolGitDiff} {
		if _, err := h.Run(context.Background(), tool, nil); !errors.Is(err, errIsolationUnavailable) {
			t.Errorf("%s err = %v, want errIsolationUnavailable", tool, err)
		}
		if r := log.Records()[len(log.Records())-1]; r.Action != ActionDeny {
			t.Errorf("%s audit action = %s, want deny", tool, r.Action)
		}
	}
}

// TestFingerprintFailsClosedWithoutBackend proves the per-turn observation path is
// contained too, and that the missing backend never yields "nothing changed".
func TestFingerprintFailsClosedWithoutBackend(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	h := newTestHarness(t, t.TempDir(), nil)
	ctx := context.Background()
	if _, err := h.WorkingTreeFingerprint(ctx); !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("fingerprint err = %v, want errIsolationUnavailable", err)
	}
	if changed, err := h.ChangedSince(ctx, "any-baseline"); err != nil || !changed {
		t.Fatalf("ChangedSince = (%v, %v), want (true, nil)", changed, err)
	}
	if changed, err := h.WorkingTreeChanged(ctx); err != nil || !changed {
		t.Fatalf("WorkingTreeChanged = (%v, %v), want (true, nil)", changed, err)
	}
}

// --- G4: git metadata is read-only to the model ----------------------------

func TestGitMetadataIsReadOnlyToModelTools(t *testing.T) {
	root := t.TempDir()
	gitInitRepo(t, root)
	commitRepoFile(t, root, "f.txt", "x: y\n")
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	writes := []struct {
		tool string
		args map[string]any
	}{
		{ToolWriteFile, map[string]any{"path": ".git/config", "content": "[diff]\n\texternal = ./probe.sh\n"}},
		{ToolCreateFile, map[string]any{"path": ".git/hooks/pre-commit", "content": "#!/bin/sh\n"}},
		{ToolRewriteFile, map[string]any{"path": ".git/HEAD", "from": "ref: refs/heads/main", "to": "ref: refs/heads/x"}},
	}
	for _, w := range writes {
		if _, err := h.Run(ctx, w.tool, w.args); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("%s(%v) err = %v, want ErrProtectedPath", w.tool, w.args["path"], err)
		}
	}
	if _, err := h.Run(ctx, ToolCopyFile, map[string]any{"source": ".git/config", "dest": ".git/config.copy"}); !errors.Is(err, ErrProtectedPath) {
		t.Errorf("copy_file into .git err = %v, want ErrProtectedPath", err)
	}

	// Reading git metadata stays allowed: inspecting config is ordinary work.
	if out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": ".git/config"}); err != nil || out == "" {
		t.Errorf("read_file(.git/config) = %q, %v; want readable", out, err)
	}
	// Negative control: a name that merely starts with ".git" is not metadata.
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": ".github/workflows/ci.yml", "content": "on: push\n"}); err != nil {
		t.Errorf("write_file(.github/...) err = %v, want allowed", err)
	}
}

// --- containment: a repository-configured filter cannot reach the host -----

// TestSandboxedGitFilterCannotReachHost is the G3 containment claim. A clean
// filter runs on `git status` with no disabling switch, so the boundary is the
// sandbox: the filter may run, but it cannot write SOP state or read a home
// credential. The plain `git status` leg is the positive control — the same
// filter does reach both when it is not contained.
func TestSandboxedGitFilterCannotReachHost(t *testing.T) {
	requirePOSIXShell(t)
	needSandbox(t)

	root := t.TempDir()
	gitInitRepo(t, root)
	commitRepoFile(t, root, "f.txt", "x: y\n")
	writeRepoFile(t, root, ".agent-sdlc/keep", "state")

	const canary = "sop-g3-filter-canary-7c1"
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", canary)
	t.Setenv("HOME", home)

	script := filepath.Join(root, "probe-filter.sh")
	writeExecutable(t, script, `if cat "$HOME/.ssh/probe" 2>/dev/null; then
  printf 'credential-read\n' >&2
else
  printf 'credential-denied\n' >&2
fi
{ printf 'x' > "$PWD/.agent-sdlc/filter-wrote"; } 2>/dev/null || true
exec cat`)
	runGit(t, root, "config", "filter.cf.clean", script)
	writeRepoFile(t, root, ".gitattributes", "f.txt filter=cf\n")
	writeRepoFile(t, root, "f.txt", "x: z\n")

	stateMarker := filepath.Join(root, ".agent-sdlc", "filter-wrote")

	// Positive control: uncontained, the filter reaches both targets.
	_ = exec.Command("git", "-C", root, "status", "--porcelain").Run()
	if _, err := os.Stat(stateMarker); err != nil {
		t.Fatalf("positive control failed: the filter did not write SOP state unsandboxed")
	}
	os.Remove(stateMarker)

	// Contained: the filter still runs, but neither target is reachable.
	h := newTestHarness(t, root, nil)
	out, err := h.Run(context.Background(), ToolGitStatus, nil)
	if err != nil {
		t.Fatalf("git_status: %v (%s)", err, out)
	}
	if _, err := os.Stat(stateMarker); err == nil {
		t.Error("a repository-configured filter wrote SOP state inside the sandbox")
	}
	if strings.Contains(out, canary) || strings.Contains(out, "credential-read") {
		t.Errorf("a repository-configured filter read a home credential inside the sandbox: %q", out)
	}
	if !strings.Contains(out, "credential-denied") {
		t.Errorf("the credential probe did not report a denial, so the canary is inconclusive: %q", out)
	}
}

// --- the platform table still refuses what it cannot enforce ---------------

// TestUnsupportedPlatformHasNoBackend keeps the platform invariant testable on any
// host, without the retired override table.
func TestUnsupportedPlatformHasNoBackend(t *testing.T) {
	for _, goos := range []string{"windows", "plan9", "aix"} {
		if name, bin := backendFor(goos); name != "" || bin != "" {
			t.Errorf("backendFor(%s) = (%q, %q), want none", goos, name, bin)
		}
	}
}
