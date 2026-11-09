package toolharness

// AGENCY-002 reproductions. Every case drives the production boundary
// (Harness.Run / Harness.checkCommandIn / SplitCommand) against a real temp
// repository and asserts the denial sentinel plus the state of the governed sink
// after the call. No policy is re-implemented here: the cases call the same
// unexported entry points the agent loop calls.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// agency002Repo builds a temp git repository with one committed file and returns
// the harness plus the repo root.
func agency002Repo(t *testing.T) (*Harness, string) {
	t.Helper()
	root := t.TempDir()
	gitInitRepo(t, root)
	writeRepoFile(t, root, "tracked.txt", "committed content\n")
	runGit(t, root, "add", "tracked.txt")
	runGit(t, root, "commit", "-q", "-m", "init")
	return newTestHarness(t, root, nil), root
}

// agency002Head returns the repository's current HEAD ref or "" when unborn.
func agency002Head(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// agency002Read returns a file's contents, or "" when it does not exist.
func agency002Read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// TestAgency002UnauthorizedCommands records, per case, the denial sentinel and
// the governed sink (tracked file bytes and HEAD) before and after the call.
func TestAgency002UnauthorizedCommands(t *testing.T) {
	h, root := agency002Repo(t)
	ctx := context.Background()
	trackedPath := filepath.Join(root, "tracked.txt")

	cases := []struct {
		name     string
		args     map[string]any
		sentinel error
	}{
		{"shell-wrap", map[string]any{"command": "sh -c rm -rf ."}, errCommandNotAllowed},
		{"metacharacter", map[string]any{"command": "go test ./... ; rm -rf x"}, errCommandNotAllowed},
		{"pipe", map[string]any{"command": "go build ./... | tee out"}, errCommandNotAllowed},
		{"requires-approval-rm", map[string]any{"command": "rm -rf tracked.txt"}, errCommandNotAllowed},
		{"requires-approval-go-run", map[string]any{"command": "go run ./cmd/x"}, errCommandNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			beforeContent := agency002Read(t, trackedPath)
			beforeHead := agency002Head(t, root)
			_, err := h.Run(ctx, ToolRunCommand, c.args)
			if !errors.Is(err, c.sentinel) {
				t.Fatalf("run_command(%v) err = %v, want %v", c.args["command"], err, c.sentinel)
			}
			if got := agency002Read(t, trackedPath); got != beforeContent {
				t.Errorf("tracked.txt changed: %q -> %q", beforeContent, got)
			}
			if got := agency002Head(t, root); got != beforeHead {
				t.Errorf("HEAD changed: %q -> %q", beforeHead, got)
			}
		})
	}
}

// TestAgency002DestructiveGit records the same evidence for Git write cases.
func TestAgency002DestructiveGit(t *testing.T) {
	h, root := agency002Repo(t)
	ctx := context.Background()
	trackedPath := filepath.Join(root, "tracked.txt")
	writeRepoFile(t, root, "untracked.txt", "scratch\n")

	cases := []string{
		"git commit -m x",
		"git reset --hard",
		"git clean -fdx",
		"git push --force",
		"git push",
		"git merge main",
		"git checkout -- .",
	}
	for _, cmd := range cases {
		t.Run(strings.ReplaceAll(cmd, " ", "-"), func(t *testing.T) {
			beforeContent := agency002Read(t, trackedPath)
			beforeHead := agency002Head(t, root)
			_, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": cmd})
			if !errors.Is(err, errCommandNotAllowed) {
				t.Fatalf("run_command(%q) err = %v, want errCommandNotAllowed", cmd, err)
			}
			if got := agency002Read(t, trackedPath); got != beforeContent {
				t.Errorf("tracked.txt changed: %q -> %q", beforeContent, got)
			}
			if got := agency002Head(t, root); got != beforeHead {
				t.Errorf("HEAD changed: %q -> %q", beforeHead, got)
			}
			if _, err := os.Stat(filepath.Join(root, "untracked.txt")); err != nil {
				t.Errorf("untracked.txt removed by a denied clean: %v", err)
			}
		})
	}
}

// TestAgency002RepositoryEscape covers absolute and relative escape attempts.
// A probe file outside the root is placed first so the case can prove the denied
// command never reached it.
func TestAgency002RepositoryEscape(t *testing.T) {
	h, _ := agency002Repo(t)
	ctx := context.Background()
	outsideDir := t.TempDir()
	probe := filepath.Join(outsideDir, "probe.txt")
	if err := os.WriteFile(probe, []byte("untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		args     map[string]any
		sentinel error
	}{
		{"relative-parent", map[string]any{"command": "go test ../.."}, errCommandNotAllowed},
		{"absolute-outside", map[string]any{"command": "go test " + filepath.Join(outsideDir, "...")}, errCommandNotAllowed},
		{"absolute-file-outside", map[string]any{"command": "go build " + probe}, errCommandNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := h.Run(ctx, ToolRunCommand, c.args)
			if !errors.Is(err, c.sentinel) {
				t.Fatalf("run_command(%v) err = %v, want %v", c.args["command"], err, c.sentinel)
			}
			if got := agency002Read(t, probe); got != "untouched\n" {
				t.Errorf("probe outside root changed: %q", got)
			}
		})
	}
}

// TestAgency002LocationOverrides covers -C/--git-dir/--work-tree and an
// unauthorized cwd selector.
func TestAgency002LocationOverrides(t *testing.T) {
	h, _ := agency002Repo(t)
	ctx := context.Background()
	outsideDir := t.TempDir()
	probe := filepath.Join(outsideDir, "probe.txt")
	if err := os.WriteFile(probe, []byte("untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	commands := []map[string]any{
		{"command": "git -C " + outsideDir + " status"},
		{"command": "git --git-dir " + filepath.Join(outsideDir, ".git") + " status"},
		{"command": "git --work-tree " + outsideDir + " status"},
		{"command": "git --git-dir=" + filepath.Join(outsideDir, ".git") + " status"},
		{"command": "git --work-tree=" + outsideDir + " status"},
		{"command": "git status", "cwd": outsideDir},
	}
	for _, args := range commands {
		t.Run(SummarizeRequest(ToolRunCommand, args), func(t *testing.T) {
			_, err := h.Run(ctx, ToolRunCommand, args)
			if !errors.Is(err, errCommandNotAllowed) && !errors.Is(err, errUnauthorizedRoot) {
				t.Fatalf("run_command(%v) err = %v, want errCommandNotAllowed or errUnauthorizedRoot", args, err)
			}
			if got := agency002Read(t, probe); got != "untouched\n" {
				t.Errorf("probe outside root changed: %q", got)
			}
		})
	}

	// A root selector naming an unauthorized directory is refused before the
	// command runs.
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "probe.txt", "root": outsideDir}); !errors.Is(err, errUnauthorizedRoot) {
		t.Errorf("read_file(root=outside) err = %v, want errUnauthorizedRoot", err)
	}
}

// TestAgency002RuntimeStateMutation proves the fake state database is byte-equal
// before and after every access attempt.
func TestAgency002RuntimeStateMutation(t *testing.T) {
	h, root := agency002Repo(t)
	ctx := context.Background()
	statePath := filepath.Join(root, ".agent-sdlc", "state.db")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("state bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range []map[string]any{
		{"command": "cat .agent-sdlc/state.db"},
		{"command": "go build .agent-sdlc/state.db"},
	} {
		t.Run(SummarizeRequest(ToolRunCommand, args), func(t *testing.T) {
			_, err := h.Run(ctx, ToolRunCommand, args)
			if !errors.Is(err, ErrProtectedPath) && !errors.Is(err, errCommandNotAllowed) {
				t.Fatalf("run_command(%v) err = %v, want a policy refusal", args, err)
			}
			if got := agency002Read(t, statePath); got != "state bytes\n" {
				t.Errorf("state.db changed: %q", got)
			}
		})
	}

	// File tools refuse the same path.
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": ".agent-sdlc/state.db", "content": "tampered"}); !errors.Is(err, ErrProtectedPath) {
		t.Errorf("write_file err = %v, want ErrProtectedPath", err)
	}
	if got := agency002Read(t, statePath); got != "state bytes\n" {
		t.Errorf("state.db changed: %q", got)
	}
}

// TestAgency002RequiresApprovalCannotRun proves the classification label alone
// does not make a REQUIRES_APPROVAL command executable through the harness.
func TestAgency002RequiresApprovalCannotRun(t *testing.T) {
	h, root := agency002Repo(t)
	ctx := context.Background()
	trackedPath := filepath.Join(root, "tracked.txt")
	before := agency002Read(t, trackedPath)
	for _, cmd := range []string{"sh -c true", "rm tracked.txt", "go run ./cmd/x", "git commit -m x"} {
		t.Run(cmd, func(t *testing.T) {
			_, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": cmd})
			if !errors.Is(err, errCommandNotAllowed) {
				t.Fatalf("run_command(%q) err = %v, want errCommandNotAllowed", cmd, err)
			}
			if got := agency002Read(t, trackedPath); got != before {
				t.Errorf("tracked.txt changed: %q -> %q", before, got)
			}
		})
	}
}

// TestAgency002PositiveControl proves an authorized command still runs and its
// exit status is observable through the harness.
func TestAgency002PositiveControl(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "go.mod", "module agency002ctl\n\ngo 1.21\n")
	writeRepoFile(t, root, "ctl.go", "package agency002ctl\n\n// Value is exported for the positive control.\nvar Value = 1\n")
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	for _, cmd := range []string{"go build ./...", "go vet ./..."} {
		t.Run(cmd, func(t *testing.T) {
			out, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": cmd})
			if err != nil {
				t.Fatalf("run_command(%q) err = %v", cmd, err)
			}
			if !strings.HasPrefix(out, "exit 0") {
				t.Fatalf("result = %q, want exit 0", out)
			}
		})
	}
}
