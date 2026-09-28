package ollamaagent

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
	ctx := context.Background()

	for _, bad := range []string{
		"../outside.txt",
		"a/../../outside.txt",
		"/etc/passwd",
		"escape/secret.txt", // symlink that leaves the repository
	} {
		if out, err := tb.run(ctx, "read_file", map[string]any{"path": bad}); err == nil {
			t.Errorf("read_file(%q) = (%q, nil), want rejection", bad, out)
		}
	}
}

// TestStateDBIsProtectedForEveryTool is the regression for the blocking finding:
// the state database must be refused for reads and listings, not only writes.
func TestStateDBIsProtectedForEveryTool(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".agent-sdlc")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.db"), []byte("workflow state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "config.yaml"), []byte("project:\n  name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tb := newTestToolbox(t, root)
	ctx := context.Background()

	// Direct and path-variant reads must be refused.
	for _, path := range []string{
		filepath.Join(".agent-sdlc", "state.db"),
		".agent-sdlc/state.db",
		"./.agent-sdlc/state.db",
		filepath.Join(root, ".agent-sdlc", "state.db"),
	} {
		if out, err := tb.run(ctx, "read_file", map[string]any{"path": path}); err == nil {
			t.Errorf("read_file(%q) succeeded: %q", path, out)
		}
	}

	// Writes and creates must be refused.
	for _, tool := range []string{"write_file", "create_file"} {
		if _, err := tb.run(ctx, tool, map[string]any{"path": ".agent-sdlc/state.db", "content": "tampered"}); err == nil {
			t.Errorf("%s of .agent-sdlc/state.db was allowed", tool)
		}
	}
	if got, err := os.ReadFile(filepath.Join(stateDir, "state.db")); err != nil || string(got) != "workflow state" {
		t.Errorf("state.db changed: %q, %v", got, err)
	}

	// list_files on the state directory must not reveal the database name.
	out, err := tb.run(ctx, "list_files", map[string]any{"path": ".agent-sdlc"})
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}
	if strings.Contains(out, "state.db") {
		t.Errorf("list_files exposed state.db:\n%s", out)
	}

	// search_files must not surface the database contents.
	out, err = tb.run(ctx, "search_files", map[string]any{"pattern": "workflow"})
	if err != nil {
		t.Fatalf("search_files: %v", err)
	}
	if strings.Contains(out, "workflow state") || strings.Contains(out, "state.db") {
		t.Errorf("search_files exposed the state database:\n%s", out)
	}

	// A command naming the database must be refused.
	if _, err := tb.run(ctx, "run_command", map[string]any{"command": "cat .agent-sdlc/state.db"}); err == nil {
		t.Error("run_command accessing .agent-sdlc/state.db was allowed")
	}

	// The configuration file stays readable: only the database is protected.
	if out, err := tb.run(ctx, "read_file", map[string]any{"path": ".agent-sdlc/config.yaml"}); err != nil || !strings.Contains(out, "project") {
		t.Errorf("read_file(.agent-sdlc/config.yaml) = (%q, %v), want the config", out, err)
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

// --- command policy through the shared harness ---

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
