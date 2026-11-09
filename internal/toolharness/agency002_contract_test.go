package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AGENCY-002 reproduction tests. Every case drives the production boundary
// (Harness.Run -> runCommandWithStatus -> checkCommandIn, or Harness.CheckCommand)
// rather than re-implementing the policy. Each denial asserts the specific
// sentinel with errors.Is and proves the governed sink was not reached: no
// process started (the EvidenceSink stays empty) and no repository file changed.

// agency002Repo builds a primary root with a state database and a sentinel file
// whose content must not change when a command is denied.
func agency002Repo(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "workflow state")
	writeRepoFile(t, root, "sentinel.txt", "original")
	return root
}

// countingSink returns an EvidenceSink and a pointer that records how many
// commands actually executed. A denial must leave it at zero.
func countingSink() (func(CommandEvidence), *int) {
	var n int
	return func(CommandEvidence) { n++ }, &n
}

func TestAgency002DeniedRunCommandHasNoSideEffect(t *testing.T) {
	root := agency002Repo(t)
	sink, executed := countingSink()
	cfg := DefaultConfig()
	cfg.EvidenceSink = sink
	h := New(root, cfg, nil)
	ctx := context.Background()

	cases := []struct {
		name     string
		command  string
		sentinel error
	}{
		{"unauthorized-program", "rm -rf sentinel.txt", errCommandNotAllowed},
		{"git-push-force", "git push --force origin main", errCommandNotAllowed},
		{"git-commit", "git commit -m x", errCommandNotAllowed},
		{"git-reset-hard", "git reset --hard", errCommandNotAllowed},
		{"git-c-override", "git -C /tmp status", errCommandNotAllowed},
		{"git-git-dir-override", "git --git-dir=/tmp/x status", errCommandNotAllowed},
		{"git-work-tree-override", "git --work-tree=/tmp/x status", errCommandNotAllowed},
		{"absolute-escape", "gofmt -w /etc/passwd", errCommandNotAllowed},
		{"relative-escape", "gofmt -w ../../victim.txt", errCommandNotAllowed},
		{"state-db", "cat .agent-sdlc/state.db", ErrProtectedPath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": tc.command}); !errors.Is(err, tc.sentinel) {
				t.Fatalf("Run(%q) err = %v, want errors.Is %v", tc.command, err, tc.sentinel)
			}
		})
	}

	// The governed sink is unchanged: nothing executed, and no file changed.
	if *executed != 0 {
		t.Fatalf("denied commands executed %d processes via the evidence sink, want 0", *executed)
	}
	if got, err := os.ReadFile(filepath.Join(root, "sentinel.txt")); err != nil || string(got) != "original" {
		t.Fatalf("sentinel.txt changed after denials: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, ".agent-sdlc", "state.db")); err != nil || string(got) != "workflow state" {
		t.Fatalf("state.db changed after denials: %q err=%v", got, err)
	}
}

func TestAgency002UnapprovedRootIsRefused(t *testing.T) {
	root := agency002Repo(t)
	sink, executed := countingSink()
	cfg := DefaultConfig()
	cfg.EvidenceSink = sink
	h := New(root, cfg, nil)

	other := t.TempDir()
	_, err := h.Run(context.Background(), ToolRunCommand, map[string]any{
		"command": "git status",
		"cwd":     other,
	})
	if !errors.Is(err, errUnauthorizedRoot) {
		t.Fatalf("run_command with an unapproved cwd err = %v, want errUnauthorizedRoot", err)
	}
	if *executed != 0 {
		t.Fatalf("an unapproved root executed %d commands, want 0", *executed)
	}
}

func TestAgency002ReadOnlyRootRefusesWrite(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "primary")
	external := filepath.Join(base, "external")
	for _, d := range []string{primary, external} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(t, filepath.Join(external, "go.mod"), "module fixture\n\ngo 1.21\n")
	writeFixture(t, filepath.Join(external, "bad.go"), "package fixture\n\nfunc  F(){}\n")

	cfg := DefaultConfig()
	cfg.Roots = []Root{{Path: external, Mode: RootReadOnly}}
	h := New(primary, cfg, nil)

	bad := filepath.Join(external, "bad.go")
	before, _ := os.ReadFile(bad)
	_, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "gofmt -w bad.go", "cwd": external})
	if !errors.Is(err, errReadOnlyRoot) {
		t.Fatalf("gofmt -w on a read-only root err = %v, want errReadOnlyRoot", err)
	}
	if after, _ := os.ReadFile(bad); string(before) != string(after) {
		t.Fatal("a denied write modified a read-only root file")
	}
}

// TestAgency002SafeCommandsStillRun is the authorized-operation positive control:
// tightening the boundary must not break the verification commands SOP relies on.
func TestAgency002SafeCommandsStillRun(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module agency002\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.go"), []byte("package agency002\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sink, executed := countingSink()
	cfg := DefaultConfig()
	cfg.EvidenceSink = sink
	h := New(root, cfg, nil)
	ctx := context.Background()

	for _, command := range []string{"go build ./...", "go vet ./...", "go test ./..."} {
		out, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": command})
		if err != nil {
			t.Fatalf("Run(%q) err = %v, want allowed", command, err)
		}
		if !strings.Contains(out, "exit 0") {
			t.Fatalf("Run(%q) output = %q, want exit 0", command, out)
		}
	}
	if *executed == 0 {
		t.Fatal("the safe commands did not execute through the evidence sink")
	}
}
