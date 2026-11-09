package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
