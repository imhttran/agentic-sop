package deepseekagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newTestToolbox(t *testing.T, root string) *toolbox {
	t.Helper()
	return newToolbox(root, testConfig(""))
}

// --- repository boundary ---

func TestResolveRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "secret.txt", "top secret")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	tb := newTestToolbox(t, root)

	for _, bad := range []string{
		"../outside.txt",
		"a/../../outside.txt",
		"/etc/passwd",
		"escape/secret.txt", // symlink that leaves the repository
	} {
		if _, err := tb.resolve(bad, false); err == nil {
			t.Errorf("resolve(%q) = nil error, want rejection", bad)
		}
	}
	if _, err := tb.resolve("inside.txt", false); err != nil {
		t.Errorf("resolve(inside.txt) = %v, want success", err)
	}
}

func TestResolveRejectsStateWritesButAllowsReads(t *testing.T) {
	root := t.TempDir()
	tb := newTestToolbox(t, root)

	if _, err := tb.resolve(filepath.Join(".agent-sdlc", "state.db"), true); err == nil {
		t.Error("writing .agent-sdlc/state.db was allowed")
	}
	if _, err := tb.resolve(filepath.Join(".agent-sdlc", "config.yaml"), false); err != nil {
		t.Errorf("reading .agent-sdlc/config.yaml was refused: %v", err)
	}
}

func TestToolWriteToStateIsRefused(t *testing.T) {
	root := t.TempDir()
	tb := newTestToolbox(t, root)
	_, err := tb.run(context.Background(), "write_file", map[string]any{
		"path":    filepath.Join(".agent-sdlc", "state.db"),
		"content": "tampered",
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to modify SOP state") {
		t.Fatalf("err = %v, want a state-write refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".agent-sdlc", "state.db")); statErr == nil {
		t.Error("the state file was created")
	}
}

// --- file tools ---

func TestReadWriteListSearch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, filepath.Join("pkg", "a.go"), "package pkg\nconst Marker = 1\n")
	tb := newTestToolbox(t, root)
	ctx := context.Background()

	out, err := tb.run(ctx, "read_file", map[string]any{"path": "pkg/a.go"})
	if err != nil || !strings.Contains(out, "const Marker = 1") {
		t.Fatalf("read_file = (%q, %v)", out, err)
	}

	if _, err := tb.run(ctx, "write_file", map[string]any{"path": "pkg/b.go", "content": "package pkg\n"}); err != nil {
		t.Fatalf("write_file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "pkg", "b.go")); err != nil {
		t.Fatalf("b.go not written: %v", err)
	}

	out, err = tb.run(ctx, "list_files", map[string]any{"path": "pkg"})
	if err != nil || !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") {
		t.Fatalf("list_files = (%q, %v)", out, err)
	}

	out, err = tb.run(ctx, "search_files", map[string]any{"pattern": "Marker"})
	if err != nil || !strings.Contains(out, "pkg/a.go:2") {
		t.Fatalf("search_files = (%q, %v)", out, err)
	}
}

func TestCreateFileRefusesExistingTool(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.txt", "one")
	tb := newTestToolbox(t, root)
	if _, err := tb.run(context.Background(), "create_file", map[string]any{"path": "a.txt", "content": "two"}); err == nil {
		t.Error("create_file overwrote an existing file")
	}
}

// --- command policy ---

func TestCommandPolicyAllowsSafeCommands(t *testing.T) {
	tb := newTestToolbox(t, t.TempDir())
	for _, argv := range [][]string{
		{"git", "status"},
		{"git", "diff"},
		{"git", "log", "--oneline", "-5"},
		{"go", "test", "./..."},
		{"go", "build", "./..."},
		{"go", "vet", "./..."},
		{"gofmt", "-l", "."},
	} {
		if err := tb.allowCommand(argv); err != nil {
			t.Errorf("allowCommand(%v) = %v, want allowed", argv, err)
		}
	}
}

func TestCommandPolicyRejectsDestructiveCommands(t *testing.T) {
	tb := newTestToolbox(t, t.TempDir())
	for _, argv := range [][]string{
		{"git", "commit", "-m", "x"},
		{"git", "push"},
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
		if err := tb.allowCommand(argv); err == nil {
			t.Errorf("allowCommand(%v) = nil, want rejection", argv)
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
		if _, err := splitCommand(bad); err == nil {
			t.Errorf("splitCommand(%q) = nil, want rejection", bad)
		}
	}

	argv, err := splitCommand(`go test -run 'TestThing' ./internal/...`)
	if err != nil {
		t.Fatalf("splitCommand failed on a safe command: %v", err)
	}
	want := []string{"go", "test", "-run", "TestThing", "./internal/..."}
	if strings.Join(argv, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("argv = %v, want %v", argv, want)
	}
}

func TestRunCommandRejectsDestructiveCall(t *testing.T) {
	tb := newTestToolbox(t, t.TempDir())
	_, err := tb.run(context.Background(), "run_command", map[string]any{"command": "git reset --hard"})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v, want a policy rejection", err)
	}
}

func TestRunCommandExecutesSafeCommand(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	tb := newTestToolbox(t, root)

	result, err := tb.run(context.Background(), "run_command", map[string]any{"command": "git status"})
	if err != nil {
		t.Fatalf("run_command: %v", err)
	}
	if !strings.Contains(result, "exit ") {
		t.Errorf("result = %q, want an exit status", result)
	}
}
