package worktreeevidence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func evidenceWorktree(t *testing.T) string {
	t.Helper()
	worktree := filepath.Join(t.TempDir(), "wt")
	write(t, worktree, ".agent-sdlc/plan.meta.json", `{"plan_id":"plan-evidence"}`)
	write(t, worktree, ".agent-sdlc/state.db", "state")
	write(t, worktree, ".agent-sdlc/runs/CONTEXT-001/trace.json", `{"stage":"PASSED"}`)
	write(t, worktree, ".agent-sdlc/config.yaml", "project:\n  name: demo\n")
	return worktree
}

func TestPreserveAndVerify(t *testing.T) {
	worktree := evidenceWorktree(t)
	archiveRoot := filepath.Join(t.TempDir(), "archive")

	has, err := HasLifecycleEvidence(worktree)
	if err != nil || !has {
		t.Fatalf("HasLifecycleEvidence = %v, %v; want true, nil", has, err)
	}
	archive, err := Preserve(worktree, archiveRoot)
	if err != nil {
		t.Fatalf("Preserve: %v", err)
	}
	wantArchive := filepath.Join(archiveRoot, "plan-evidence")
	if archive != wantArchive {
		t.Fatalf("archive = %q, want %q", archive, wantArchive)
	}
	if err := Verify(worktree, archiveRoot); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(archive, "state.db")); err != nil || string(got) != "state" {
		t.Fatalf("archived state.db = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(archive, "runs", "CONTEXT-001", "trace.json")); err != nil || !strings.Contains(string(got), "PASSED") {
		t.Fatalf("archived run trace = %q, err=%v", got, err)
	}

	// Idempotent while the archive matches.
	if again, err := Preserve(worktree, archiveRoot); err != nil || again != archive {
		t.Fatalf("second Preserve = %q, %v; want %q, nil", again, err, archive)
	}
}

func TestVerifyRejectsMismatchedArchive(t *testing.T) {
	worktree := evidenceWorktree(t)
	archiveRoot := filepath.Join(t.TempDir(), "archive")
	archive, err := Preserve(worktree, archiveRoot)
	if err != nil {
		t.Fatalf("Preserve: %v", err)
	}
	write(t, archive, "state.db", "tampered")
	if err := Verify(worktree, archiveRoot); err == nil {
		t.Fatal("Verify accepted a mismatched archive")
	}
	if _, err := Preserve(worktree, archiveRoot); err == nil {
		t.Fatal("Preserve overwrote a mismatched archive")
	}
}

func TestVerifyRequiresArchiveWhenEvidenceExists(t *testing.T) {
	worktree := evidenceWorktree(t)
	archiveRoot := filepath.Join(t.TempDir(), "archive")
	if err := Verify(worktree, archiveRoot); err == nil {
		t.Fatal("Verify accepted evidence without an archive")
	}
}

func TestNoLifecycleEvidenceNeedsNoArchive(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "wt")
	write(t, worktree, ".agent-sdlc/config.yaml", "project:\n  name: demo\n")
	write(t, worktree, ".agent-sdlc/security-baseline.json", "{}\n")
	has, err := HasLifecycleEvidence(worktree)
	if err != nil || has {
		t.Fatalf("HasLifecycleEvidence = %v, %v; want false, nil", has, err)
	}
	if err := Verify(worktree, filepath.Join(t.TempDir(), "archive")); err != nil {
		t.Fatalf("Verify without evidence: %v", err)
	}
	archive, err := Preserve(worktree, filepath.Join(t.TempDir(), "archive"))
	if err != nil || archive != "" {
		t.Fatalf("Preserve without evidence = %q, %v; want empty, nil", archive, err)
	}
}

func TestArchiveInsideWorktreeRejected(t *testing.T) {
	worktree := evidenceWorktree(t)
	inside := filepath.Join(worktree, ".agent-sdlc", "archive")
	if _, err := ArchivePath(worktree, inside); !errors.Is(err, ErrArchiveInsideWorktree) {
		t.Fatalf("ArchivePath inside err = %v, want ErrArchiveInsideWorktree", err)
	}
	if _, err := Preserve(worktree, inside); !errors.Is(err, ErrArchiveInsideWorktree) {
		t.Fatalf("Preserve inside err = %v, want ErrArchiveInsideWorktree", err)
	}
}

func TestArchiveIdentityFallsBackToWorktreeName(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "task-T1")
	write(t, worktree, ".agent-sdlc/state.db", "state")
	archiveRoot := filepath.Join(t.TempDir(), "archive")
	archive, err := Preserve(worktree, archiveRoot)
	if err != nil {
		t.Fatalf("Preserve: %v", err)
	}
	if want := filepath.Join(archiveRoot, "task-T1"); archive != want {
		t.Fatalf("archive = %q, want %q", archive, want)
	}
}
