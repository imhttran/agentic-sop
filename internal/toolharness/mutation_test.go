package toolharness

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMutationFingerprintState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, string)
		want   bool
	}{
		{"same content", func(t *testing.T, root string) { writeRepoFile(t, root, "nested/a.txt", "original") }, false},
		{"timestamps", func(t *testing.T, root string) {
			if err := os.Chtimes(filepath.Join(root, "nested/a.txt"), time.Unix(1, 0), time.Unix(1, 0)); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"inode", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "nested/a.txt")); err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, root, "nested/a.txt", "original")
		}, false},
		{"content", func(t *testing.T, root string) { writeRepoFile(t, root, "nested/a.txt", "modified") }, true},
		{"create empty", func(t *testing.T, root string) { writeRepoFile(t, root, "empty.txt", "") }, true},
		{"delete", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "nested/a.txt")); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"rename", func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, "nested/a.txt"), filepath.Join(root, "nested/b.txt")); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"mode", func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, "nested/a.txt"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"type", func(t *testing.T, root string) {
			path := filepath.Join(root, "nested/a.txt")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o644); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"git metadata", func(t *testing.T, root string) { writeRepoFile(t, root, ".git/index", "metadata") }, false},
		{"git worktree pointer", func(t *testing.T, root string) { writeRepoFile(t, root, ".git", "gitdir: elsewhere") }, false},
		{"SOP state", func(t *testing.T, root string) { writeRepoFile(t, root, ".agent-sdlc/state.db", "private runtime") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeRepoFile(t, root, "nested/a.txt", "original")
			h := newTestHarness(t, root, nil)
			before, err := h.mutationFingerprint(context.Background(), h.root)
			if err != nil {
				t.Fatal(err)
			}
			tc.change(t, root)
			after, err := h.mutationFingerprint(context.Background(), h.root)
			if err != nil {
				t.Fatal(err)
			}
			if (before != after) != tc.want {
				t.Errorf("changed=%v, want %v", before != after, tc.want)
			}
		})
	}
}

func TestMutationFingerprintHashesCompleteBinaryContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "binary.dat")
	data := make([]byte, maxFingerprintFileBytes+1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTestHarness(t, root, nil)
	before, err := h.mutationFingerprint(context.Background(), h.root)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] = 1 // Same-size modification beyond the older fingerprint cap.
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := h.mutationFingerprint(context.Background(), h.root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("binary tail modification was missed")
	}
}

func TestObservedMutationCanonicalTarget(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "real/a.txt", "original")
	if err := os.Symlink("real", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	h := newTestHarness(t, root, nil)
	for _, content := range []string{"original", "changed"} {
		_, observation, err := h.RunObservedMutation(context.Background(), ToolWriteFile, map[string]any{"path": "./alias/a.txt", "content": content})
		if err != nil {
			t.Fatal(err)
		}
		if !observation.Succeeded || !observation.Verified || observation.Changed != (content == "changed") {
			t.Errorf("observation=%+v for content %q", observation, content)
		}
	}
}

func TestObservedMutationUnavailableNeverCredits(t *testing.T) {
	for _, cancelled := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancelled context", false: "snapshot timeout"}[cancelled], func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			audit := NewAuditLog(0)
			cfg := DefaultConfig()
			if !cancelled {
				cfg.CommandTimeout = -time.Nanosecond
			}
			h := New(root, cfg, audit)
			if cancelled {
				cancel()
			}
			_, observation, err := h.RunObservedMutation(ctx, ToolCreateFile, map[string]any{"path": "new.txt", "content": "created once"})
			if err != nil {
				t.Fatal(err)
			}
			if !observation.Succeeded || observation.Verified || observation.Changed {
				t.Fatalf("unavailable snapshot fabricated evidence: %+v", observation)
			}
			if len(audit.Records()) != 1 {
				t.Errorf("audit=%+v, want once", audit.Records())
			}
			if content, err := os.ReadFile(filepath.Join(root, "new.txt")); err != nil || string(content) != "created once" {
				t.Fatalf("successful execution not preserved: %q %v", content, err)
			}
		})
	}
}

func TestObservedCommandPartialFailure(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "valid.go", "package a\nvar x=1\n")
	writeRepoFile(t, root, "invalid.go", "this is not Go")
	log := NewAuditLog(0)
	h := newTestHarness(t, root, log)
	result, observation, err := h.RunObservedMutation(context.Background(), ToolRunCommand, map[string]any{"command": "gofmt -w valid.go invalid.go"})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Succeeded || !observation.Verified || !observation.Changed {
		t.Fatalf("observation=%+v, want failed execution with observed partial change", observation)
	}
	if !strings.HasPrefix(result, "exit 2\n") {
		t.Errorf("result=%q, want nonzero exit", result)
	}
	content, err := os.ReadFile(filepath.Join(root, "valid.go"))
	if err != nil || string(content) != "package a\n\nvar x = 1\n" {
		t.Fatalf("partial change lost: %q %v", content, err)
	}
	if records := log.Records(); len(records) != 1 || records[0].Outcome != OutcomeError {
		t.Errorf("audit=%+v, want one failed execution", records)
	}
}

func TestObservedCommandIgnoresAuditWrites(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "a.go", "package a\n")
	path := filepath.Join(root, "tool-audit.jsonl")
	h := newTestHarness(t, root, NewFileAuditor(path))
	for i := 0; i < 2; i++ {
		_, observation, err := h.RunObservedMutation(context.Background(), ToolRunCommand, map[string]any{"command": "gofmt -w a.go"})
		if err != nil {
			t.Fatal(err)
		}
		if !observation.Succeeded || !observation.Verified || observation.Changed {
			t.Fatalf("audit write manufactured mutation: %+v", observation)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), "\n") != 2 {
		t.Fatalf("audit=%s, want two executions", content)
	}
}

func TestObservedMutationAfterSnapshotFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture requires a POSIX shell and FIFO")
	}
	root := t.TempDir()
	// This stand-in command succeeds once but leaves a FIFO that the observer
	// cannot hash safely. The after-snapshot failure must not count as change.
	writeRepoFile(t, root, "gofmt", "#!/bin/sh\necho invocation >> calls.txt\nmkfifo unsupported\n")
	if err := os.Chmod(filepath.Join(root, "gofmt"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := newTestHarness(t, root, nil)
	_, observation, err := h.RunObservedMutation(context.Background(), ToolRunCommand, map[string]any{"command": "./gofmt -w"})
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Succeeded || observation.Verified || observation.Changed {
		t.Fatalf("failed after-snapshot manufactured evidence: %+v", observation)
	}
	content, err := os.ReadFile(filepath.Join(root, "calls.txt"))
	if err != nil || string(content) != "invocation\n" {
		t.Fatalf("executed more than once or lost partial state: %q %v", content, err)
	}
}
