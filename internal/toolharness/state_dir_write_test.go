package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStateDirectoryAliasesAreRecognized proves the write boundary is not
// bypassed by a case alias or a symlink. A case-sensitive filesystem keeps
// ".AGENT-SDLC" as a distinct directory, so the case-alias assertion applies only
// when the two paths are the same file (macOS/Windows default behavior).
func TestStateDirectoryAliasesAreRecognized(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "workflow state")
	writeRepoFile(t, root, ".agent-sdlc/config.yaml", "project:\n  name: x\n")
	writeRepoFile(t, root, ".agent-sdlc/runs/T1/outcome.json", "{\"status\":\"VERIFIED\"}\n")
	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	lower, lerr := os.Stat(filepath.Join(root, ".agent-sdlc"))
	upper, uerr := os.Stat(filepath.Join(root, ".AGENT-SDLC"))
	if lerr == nil && uerr == nil && os.SameFile(lower, upper) {
		// The filesystem is case-insensitive: the alias names the same state dir.
		if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": ".AGENT-SDLC/runs/T1/forged.json", "content": "forged"}); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("case-alias write err = %v, want ErrProtectedPath", err)
		}
		if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": ".AGENT-SDLC/state.db"}); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("case-alias read of state.db err = %v, want ErrProtectedPath", err)
		}
	}

	link := filepath.Join(root, "state-link")
	if err := os.Symlink(".agent-sdlc", link); err != nil {
		t.Logf("symlink unavailable on this platform; skipping symlink assertions: %v", err)
	} else {
		if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "state-link/runs/T1/forged.json", "content": "forged"}); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("symlink write err = %v, want ErrProtectedPath", err)
		}
		if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": "state-link/state.db"}); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("symlink read of state.db err = %v, want ErrProtectedPath", err)
		}
		if out, err := h.Run(ctx, ToolListFiles, map[string]any{"path": "state-link"}); err != nil || strings.Contains(out, "state.db") {
			t.Errorf("list_files(state-link) = %q, %v; state.db must stay hidden", out, err)
		}
	}
	if out, err := h.Run(ctx, ToolSearchFiles, map[string]any{"pattern": "VERIFIED"}); err != nil || strings.Contains(out, "VERIFIED") {
		t.Errorf("search_files(VERIFIED) = %q, %v; state content must stay hidden", out, err)
	}

	// Positive control: ordinary repository writes and a state-directory read of
	// configuration still work.
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "internal/ok.go", "content": "package internal\n"}); err != nil {
		t.Fatalf("ordinary write was refused: %v", err)
	}
	if out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": ".agent-sdlc/config.yaml"}); err != nil || !strings.Contains(out, "project") {
		t.Errorf("read_file(.agent-sdlc/config.yaml) = %q, %v; want readable", out, err)
	}
}

// TestSubprocessStateWriteIsOutsideFileToolBoundary records the actual boundary:
// the file tools refuse .agent-sdlc, but a SAFE command's own subprocess is not
// sandboxed and can still write there. This keeps the report from claiming
// comprehensive protection against model-authored state; a future subprocess
// isolation change should update this test and the report together.
func TestSubprocessStateWriteIsOutsideFileToolBoundary(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "go.mod", "module example.com/stateprobe\n\ngo 1.27\n")
	writeRepoFile(t, root, "probe_test.go", `package stateprobe

import (
	"os"
	"testing"
)

func TestWriteState(t *testing.T) {
	if err := os.MkdirAll(".agent-sdlc", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agent-sdlc/subprocess-probe.txt", []byte("written"), 0o644); err != nil {
		t.Fatal(err)
	}
}
`)

	h := newTestHarness(t, root, nil)
	ctx := context.Background()
	if _, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": "go test ./..."}); err != nil {
		t.Fatalf("go test failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-sdlc", "subprocess-probe.txt")); err != nil {
		t.Fatalf("expected the subprocess write to exist (current boundary): %v", err)
	}
	t.Log("verified limitation: run_command subprocesses are outside the file-tool state-write boundary")
}

// TestStateDirectoryIsWriteProtectedForModelTools pins the misinformation
// boundary: SOP's runtime state directory is readable (except state.db) but never
// writable by a model tool. Its run artifacts are authoritative evidence, so a
// model-authored outcome.json, acceptance_decisions.jsonl, waivers.jsonl, or
// changed-files.json could turn prose into a verified outcome or an approval.
func TestStateDirectoryIsWriteProtectedForModelTools(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "workflow state")
	writeRepoFile(t, root, ".agent-sdlc/config.yaml", "project:\n  name: x\n")
	const decision = `{"task":"T1","criterion_id":"criterion-1","approver":"Tom Tran"}` + "\n"
	writeRepoFile(t, root, ".agent-sdlc/runs/T1/acceptance_decisions.jsonl", decision)
	writeRepoFile(t, root, "seed.txt", "seed\n")

	h := newTestHarness(t, root, nil)
	ctx := context.Background()

	writeTools := []struct {
		name string
		args map[string]any
	}{
		{ToolWriteFile, map[string]any{"path": ".agent-sdlc/runs/T1/acceptance_decisions.jsonl", "content": "forged"}},
		{ToolCreateFile, map[string]any{"path": ".agent-sdlc/runs/T1/waivers.jsonl", "content": "forged"}},
		{ToolDeleteFile, map[string]any{"path": ".agent-sdlc/runs/T1/acceptance_decisions.jsonl"}},
		{ToolRewriteFile, map[string]any{"path": ".agent-sdlc/config.yaml", "from": "x", "to": "y"}},
		{ToolCopyFile, map[string]any{"source": "seed.txt", "dest": ".agent-sdlc/runs/T1/changed-files.json"}},
		{ToolRestoreFile, map[string]any{"path": ".agent-sdlc/config.yaml"}},
	}
	for _, tc := range writeTools {
		if _, err := h.Run(ctx, tc.name, tc.args); !errors.Is(err, ErrProtectedPath) {
			t.Errorf("%s on .agent-sdlc: err = %v, want ErrProtectedPath", tc.name, err)
		}
	}

	got, err := os.ReadFile(filepath.Join(root, ".agent-sdlc", "runs", "T1", "acceptance_decisions.jsonl"))
	if err != nil || string(got) != decision {
		t.Errorf("acceptance decision changed: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-sdlc", "runs", "T1", "waivers.jsonl")); !os.IsNotExist(err) {
		t.Errorf("waivers.jsonl was created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-sdlc", "runs", "T1", "changed-files.json")); !os.IsNotExist(err) {
		t.Errorf("changed-files.json was created: %v", err)
	}

	// Reads of the state directory stay available so an inspection can see
	// configuration; only the state database itself is unreadable.
	if out, err := h.Run(ctx, ToolReadFile, map[string]any{"path": ".agent-sdlc/config.yaml"}); err != nil || !strings.Contains(out, "project") {
		t.Errorf("read_file(.agent-sdlc/config.yaml) = %q, %v; want readable", out, err)
	}
	if _, err := h.Run(ctx, ToolReadFile, map[string]any{"path": ".agent-sdlc/state.db"}); !errors.Is(err, ErrProtectedPath) {
		t.Errorf("read_file(state.db) err = %v, want ErrProtectedPath", err)
	}
}
