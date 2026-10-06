package ollamaagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// Authoritative mutation-path evidence tests (JEV task-evidence assembly).
//
// The harness records the repository paths a successful controlled mutation
// changed, so SOP can attribute changes to a task from observed mutations rather
// than inferring them from the working-tree diff (which may carry unrelated
// pre-existing edits). These tests pin that evidence for both file tools and the
// path-less command mutation.

func TestMutationPathNamesFileTools(t *testing.T) {
	for _, tool := range []string{
		toolharness.ToolWriteFile,
		toolharness.ToolCreateFile,
		toolharness.ToolDeleteFile,
		toolharness.ToolRestoreFile,
	} {
		path, ok := mutationPath(tool, map[string]any{"path": "internal/x.go"})
		if !ok || path != "internal/x.go" {
			t.Errorf("mutationPath(%s) = (%q, %v), want (internal/x.go, true)", tool, path, ok)
		}
	}
	for _, tool := range []string{toolharness.ToolReadFile, toolharness.ToolRunCommand, toolharness.ToolGitDiff} {
		if _, ok := mutationPath(tool, map[string]any{"path": "internal/x.go"}); ok {
			t.Errorf("mutationPath(%s) named a path; only file-mutating tools may", tool)
		}
	}
	if _, ok := mutationPath(toolharness.ToolWriteFile, map[string]any{"path": "  "}); ok {
		t.Error("mutationPath accepted a blank path")
	}
}

func TestMutationEvidenceRecordsEveryPathOnce(t *testing.T) {
	var ev mutationEvidence
	ev.recordPath("a.go")
	ev.recordPath("a.go") // duplicate
	ev.recordPath("  ")   // blank
	ev.recordPath("b.go")
	// record (the observed boolean) is first-wins, but paths must accumulate so a
	// multi-file change reports every file, not only the first.
	ev.record("write_file")
	ev.record("write_file")
	ev.recordPath("c.go")

	got := ev.mutationPaths()
	want := []string{"a.go", "b.go", "c.go"}
	if len(got) != len(want) {
		t.Fatalf("mutationPaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mutationPaths = %v, want %v", got, want)
		}
	}

	// The returned slice is a copy: mutating it cannot corrupt the accumulator.
	got[0] = "tampered"
	if again := ev.mutationPaths(); again[0] != "a.go" {
		t.Errorf("mutationPaths returned aliased state: %v", again)
	}

	var empty *mutationEvidence
	if got := empty.mutationPaths(); got != nil {
		t.Errorf("nil evidence mutationPaths = %v, want nil", got)
	}
}

func TestHarnessReportsOnlyMutationsItMade(t *testing.T) {
	dir := t.TempDir()

	// A pre-existing dirty file the invocation never touches: it must not appear in
	// the mutation evidence merely because it is dirty.
	if err := os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("user work\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"pkg/a.go","content":"package pkg\n"}}`,
		`{"tool":"write_file","args":{"path":"pkg/b.go","content":"package pkg\n"}}`,
		`{"status":"completed","summary":"wrote two files","changes_expected":true}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	ev := &mutationEvidence{}
	if _, _, err := New(cfg, dir).CompleteWithEvidence(context.Background(), implementRequest(), ev); err != nil {
		t.Fatalf("CompleteWithEvidence: %v", err)
	}

	got := ev.mutationPaths()
	want := []string{"pkg/a.go", "pkg/b.go"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("mutationPaths = %v, want %v (only the files the invocation wrote)", got, want)
	}
	for _, p := range got {
		if p == "unrelated.txt" {
			t.Errorf("a pre-existing dirty file was attributed to the invocation: %v", got)
		}
	}
}

func TestCommandMutationNamesNoPath(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nvar x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, srv := newFakeOllama(t,
		`{"tool":"run_command","args":{"command":"gofmt -w a.go"}}`,
		`{"status":"completed","summary":"formatted","changes_expected":true}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	ev := &mutationEvidence{}
	if _, _, err := New(cfg, dir).CompleteWithEvidence(context.Background(), implementRequest(), ev); err != nil {
		t.Fatalf("CompleteWithEvidence: %v", err)
	}
	// The command mutated the tree, but it names no path: the evidence records the
	// mutation without inventing a file. Callers fall back to the diff for paths.
	if !ev.observed {
		t.Error("the command mutation was not observed")
	}
	if got := ev.mutationPaths(); got != nil {
		t.Errorf("a run_command mutation named a path: %v", got)
	}
}
