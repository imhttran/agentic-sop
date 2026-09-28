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

func newTestHarness(t *testing.T, root string, auditor Auditor) *Harness {
	t.Helper()
	return New(root, DefaultConfig(), auditor)
}

func writeRepoFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- tools registry ---

func TestToolsListsExactlyTheInitialTools(t *testing.T) {
	want := []string{
		"read_file", "write_file", "create_file", "delete_file", "restore_file",
		"list_files", "search_files", "run_command", "git_status", "git_diff",
	}
	got := Tools()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Tools() = %v, want %v", got, want)
	}
}

// --- repository boundary ---

func TestReadRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeRepoFile(t, outside, "secret.txt", "top secret")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	for _, bad := range []string{
		"../outside.txt",
		"a/../../outside.txt",
		"/etc/passwd",
		"escape/secret.txt", // symlink that leaves the repository
	} {
		if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": bad}); err == nil {
			t.Errorf("read_file(%q) = nil error, want rejection", bad)
		}
	}
}

// --- state database protection ---

// TestStateDBProtectedForEveryAccessClass is the core guarantee: no tool may
// read, write, create, list, or search SOP's state database.
func TestStateDBProtectedForEveryAccessClass(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "workflow state")
	writeRepoFile(t, root, ".agent-sdlc/config.yaml", "project:\n  name: x\n")
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	// Direct and path-variant reads are refused with the sentinel error.
	for _, path := range []string{
		".agent-sdlc/state.db",
		"./.agent-sdlc/state.db",
		".agent-sdlc/./state.db",
		".agent-sdlc//state.db",
		"pkg/../.agent-sdlc/state.db",
		filepath.Join(root, ".agent-sdlc", "state.db"),
	} {
		_, err := h.Run(ctx, ToolReadFile, map[string]any{"path": path})
		if !errors.Is(err, ErrProtectedPath) {
			t.Errorf("read_file(%q) err = %v, want ErrProtectedPath", path, err)
		}
	}

	for _, tool := range []string{ToolWriteFile, ToolCreateFile} {
		_, err := h.Run(ctx, tool, map[string]any{"path": ".agent-sdlc/state.db", "content": "tampered"})
		if !errors.Is(err, ErrProtectedPath) {
			t.Errorf("%s err = %v, want ErrProtectedPath", tool, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(root, ".agent-sdlc", "state.db")); err != nil || string(got) != "workflow state" {
		t.Errorf("state.db changed: %q, %v", got, err)
	}

	// list_files does not reveal the name, even when listing the state dir.
	out, err := h.Run(ctx, ToolListFiles, map[string]any{"path": ".agent-sdlc"})
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}
	if strings.Contains(out, "state.db") {
		t.Errorf("list_files exposed state.db:\n%s", out)
	}

	// search_files does not surface the contents.
	out, err = h.Run(ctx, ToolSearchFiles, map[string]any{"pattern": "workflow"})
	if err != nil {
		t.Fatalf("search_files: %v", err)
	}
	if strings.Contains(out, "workflow state") || strings.Contains(out, "state.db") {
		t.Errorf("search_files exposed the state database:\n%s", out)
	}

	// run_command refuses a command that names the database.
	_, err = h.Run(ctx, ToolRunCommand, map[string]any{"command": "cat .agent-sdlc/state.db"})
	if !errors.Is(err, ErrProtectedPath) && !errors.Is(err, errCommandNotAllowed) {
		t.Errorf("run_command err = %v, want a policy refusal", err)
	}

	// The configuration file stays readable: only the database is protected.
	if out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": ".agent-sdlc/config.yaml"}); err != nil || !strings.Contains(out, "project") {
		t.Errorf("read_file(config.yaml) = (%q, %v), want the config", out, err)
	}
}

// --- file tools ---

func TestReadWriteCreateListSearch(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "pkg/a.go", "package pkg\nconst Marker = 1\n")
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "pkg/a.go"})
	if err != nil || !strings.Contains(out, "const Marker = 1") {
		t.Fatalf("read_file = (%q, %v)", out, err)
	}

	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "pkg/b.go", "content": "package pkg\n"}); err != nil {
		t.Fatalf("write_file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "pkg", "b.go")); err != nil {
		t.Fatalf("b.go not written: %v", err)
	}

	if _, err := h.Run(ctx, ToolCreateFile, map[string]any{"path": "pkg/a.go", "content": "x"}); err == nil {
		t.Error("create_file overwrote an existing file")
	}

	out, err = h.Run(ctx, ToolListFiles, map[string]any{"path": "pkg"})
	if err != nil || !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") {
		t.Fatalf("list_files = (%q, %v)", out, err)
	}

	out, err = h.Run(ctx, ToolSearchFiles, map[string]any{"pattern": "Marker"})
	if err != nil || !strings.Contains(out, "pkg/a.go:2") {
		t.Fatalf("search_files = (%q, %v)", out, err)
	}
}

// --- command policy ---

func TestCommandPolicyAllowsSafeCommands(t *testing.T) {
	h := newTestHarness(t, t.TempDir(), nil)
	for _, argv := range [][]string{
		{"git", "status"},
		{"git", "diff"},
		{"git", "log", "--oneline", "-5"},
		{"go", "test", "./..."},
		{"go", "build", "./..."},
		{"go", "vet", "./..."},
		{"gofmt", "-l", "."},
	} {
		if err := h.CheckCommand(argv); err != nil {
			t.Errorf("CheckCommand(%v) = %v, want allowed", argv, err)
		}
	}
}

func TestCommandPolicyRejectsDestructiveGitAndOthers(t *testing.T) {
	h := newTestHarness(t, t.TempDir(), nil)
	for _, argv := range [][]string{
		{"git", "commit", "-m", "x"},
		{"git", "push"},
		{"git", "push", "--force"},
		{"git", "merge", "main"},
		{"git", "rebase", "main"},
		{"git", "reset", "--hard"},
		{"git", "clean", "-fd"},
		{"git", "checkout", "--", "."},
		{"git", "restore", "."},
		{"rm", "-rf", "/"},
		{"go", "run", "./..."},
		{"git", "-C", "/tmp", "status"}, // directory override escapes the repo
	} {
		if err := h.CheckCommand(argv); err == nil {
			t.Errorf("CheckCommand(%v) = nil, want rejection", argv)
		}
	}
}

func TestSplitCommandRejectsShellMetacharacters(t *testing.T) {
	for _, bad := range []string{
		"go test ./...; rm -rf /",
		"go build ./... && echo hi",
		"go test $(whoami)",
		"cat file | mail",
		"ls *",
		"echo hi > out.txt",
	} {
		if _, err := SplitCommand(bad); err == nil {
			t.Errorf("SplitCommand(%q) = nil, want rejection", bad)
		}
	}

	argv, err := SplitCommand(`go test -run 'TestThing' ./internal/...`)
	if err != nil {
		t.Fatalf("SplitCommand failed on a safe command: %v", err)
	}
	want := []string{"go", "test", "-run", "TestThing", "./internal/..."}
	if strings.Join(argv, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("argv = %v, want %v", argv, want)
	}
}

func TestRunCommandExecutesSafeCommand(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	h := newTestHarness(t, root, nil)

	out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "git status"})
	if err != nil {
		t.Fatalf("run_command: %v", err)
	}
	if !strings.Contains(out, "exit ") {
		t.Errorf("result = %q, want an exit status", out)
	}
}

// --- git inspection ---

func TestGitInspectionToolsAreReadOnly(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	if out, err := h.Run(ctx, ToolGitStatus, nil); err != nil || !strings.Contains(out, "exit ") {
		t.Errorf("git_status = (%q, %v)", out, err)
	}
	if out, err := h.Run(ctx, ToolGitDiff, nil); err != nil || !strings.Contains(out, "exit ") {
		t.Errorf("git_diff = (%q, %v)", out, err)
	}
}

// --- working tree observation ---

func TestWorkingTreeChanged(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	if changed, err := h.WorkingTreeChanged(ctx); err != nil || changed {
		t.Fatalf("clean tree: changed=%v err=%v, want false", changed, err)
	}

	// SOP's own state directory is not a source change.
	writeRepoFile(t, root, ".agent-sdlc/state.db", "state")
	if changed, err := h.WorkingTreeChanged(ctx); err != nil || changed {
		t.Errorf("state directory only: changed=%v err=%v, want false", changed, err)
	}

	// A new source file is.
	writeRepoFile(t, root, "a.go", "package a\n")
	if changed, err := h.WorkingTreeChanged(ctx); err != nil || !changed {
		t.Errorf("untracked source: changed=%v err=%v, want true", changed, err)
	}
}

func TestWorkingTreeChangedOutsideRepo(t *testing.T) {
	h := newTestHarness(t, t.TempDir(), nil)
	if _, err := h.WorkingTreeChanged(context.Background()); err == nil {
		t.Error("want an error outside a git repository")
	}
}

func TestPorcelainPath(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		" M pkg/a.go":         "pkg/a.go",
		"?? .agent-sdlc/":     ".agent-sdlc/",
		"A  new.txt":          "new.txt",
		"R  old.go -> new.go": "new.go",
		`?? "a b.txt"`:        "a b.txt",
	}
	for line, want := range cases {
		if got := porcelainPath(line); got != want {
			t.Errorf("porcelainPath(%q) = %q, want %q", line, got, want)
		}
	}
}

// --- auditability ---

func TestToolExecutionIsAudited(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "secret state")
	writeRepoFile(t, root, "a.txt", "hello")
	log := NewAuditLog(0)
	h := newTestHarness(t, root, log)
	ctx := context.Background()

	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "a.txt"}); err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": ".agent-sdlc/state.db"}); err == nil {
		t.Fatal("reading the state database should have been denied")
	}
	if _, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": "git reset --hard"}); err == nil {
		t.Fatal("destructive git should have been denied")
	}

	records := log.Records()
	if len(records) != 3 {
		t.Fatalf("audit records = %d, want 3", len(records))
	}

	ok := records[0]
	if ok.Tool != ToolReadFile || ok.Action != ActionAllow || ok.Outcome != OutcomeOK {
		t.Errorf("record[0] = %+v, want an allowed read_file", ok)
	}
	if !strings.Contains(ok.Request, "path=a.txt") {
		t.Errorf("record[0] request = %q, want the path", ok.Request)
	}

	deniedPath := records[1]
	if deniedPath.Action != ActionDeny || deniedPath.Outcome != OutcomeDenied {
		t.Errorf("record[1] = %+v, want a denied state-db read", deniedPath)
	}

	deniedGit := records[2]
	if deniedGit.Tool != ToolRunCommand || deniedGit.Action != ActionDeny || deniedGit.Outcome != OutcomeDenied {
		t.Errorf("record[2] = %+v, want a denied command", deniedGit)
	}
}

// --- delete and restore ---

func TestDeleteFileRemovesFileAndRefusesDirectory(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "scratch.tmp", "junk")
	writeRepoFile(t, root, "pkg/a.go", "package pkg\n")
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	if _, err := h.Run(ctx, ToolDeleteFile, map[string]any{"path": "scratch.tmp"}); err != nil {
		t.Fatalf("delete_file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "scratch.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("scratch.tmp still present: %v", err)
	}
	if _, err := h.Run(ctx, ToolDeleteFile, map[string]any{"path": "pkg"}); err == nil {
		t.Error("delete_file removed a directory")
	}
	if _, err := h.Run(ctx, ToolDeleteFile, map[string]any{"path": ".agent-sdlc/state.db"}); !errors.Is(err, ErrProtectedPath) {
		t.Errorf("delete_file(state.db) err = %v, want ErrProtectedPath", err)
	}
}

func TestRestoreFileRecoversCommittedContent(t *testing.T) {
	root := t.TempDir()
	gitInitRepo(t, root)
	writeRepoFile(t, root, "README.md", "# Real readme\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-q", "-m", "init")

	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	// A mistaken clobber, then recovery from HEAD.
	writeRepoFile(t, root, "README.md", "PLACEHOLDER")
	if _, err := h.Run(ctx, ToolRestoreFile, map[string]any{"path": "README.md"}); err != nil {
		t.Fatalf("restore_file: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(got) != "# Real readme\n" {
		t.Errorf("README.md = %q, err=%v, want the committed content", got, err)
	}

	if _, err := h.Run(ctx, ToolRestoreFile, map[string]any{"path": ".agent-sdlc/state.db"}); !errors.Is(err, ErrProtectedPath) {
		t.Errorf("restore_file(state.db) err = %v, want ErrProtectedPath", err)
	}
}

// gitInitRepo initialises a repository with a deterministic identity so a commit
// works without relying on the machine's git config.
func gitInitRepo(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

func TestFileAuditorWritesJSONLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	h := New(dir, DefaultConfig(), NewFileAuditor(path))
	writeRepoFile(t, dir, "a.txt", "hi")

	if _, err := h.Run(context.Background(), ToolReadFile, map[string]any{"path": "a.txt"}); err != nil {
		t.Fatalf("read_file: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("audit file: %v", err)
	}
	if !strings.Contains(string(data), `"tool":"read_file"`) || !strings.Contains(string(data), `"outcome":"ok"`) {
		t.Errorf("audit line = %q", data)
	}
}
