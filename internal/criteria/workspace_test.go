package criteria

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDigestDeterministicAndContentSensitive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	d1, err := WorkspaceDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	d1b, _ := WorkspaceDigest(dir)
	if d1 != d1b {
		t.Fatal("digest must be deterministic for an unchanged workspace")
	}
	// An untracked-style addition must invalidate the fingerprint.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	d2, _ := WorkspaceDigest(dir)
	if d2 == d1 {
		t.Fatal("an added file must change the workspace digest")
	}
	// A content change must invalidate it.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("ONE"), 0o644); err != nil {
		t.Fatal(err)
	}
	d3, _ := WorkspaceDigest(dir)
	if d3 == d2 {
		t.Fatal("a content change must change the workspace digest")
	}
}

func TestWorkspaceDigestExcludesSOPAuditDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := WorkspaceDigest(dir)
	if err := os.MkdirAll(filepath.Join(dir, ".agent-sdlc", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agent-sdlc", "runs", "criteria.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, _ := WorkspaceDigest(dir)
	if after != before {
		t.Fatal("SOP audit artifacts under .agent-sdlc must be excluded from the fingerprint")
	}
}

func TestFreshRejectsStaleWorkspaceState(t *testing.T) {
	ev := Evidence{TaskID: "T", Attempt: 1, AttemptID: "T-a1", Revision: "r", Workspace: "/w", WorkspaceState: "A", BindingsDigest: "b"}
	ok := Freshness{TaskID: "T", Attempt: 1, AttemptID: "T-a1", Revision: "r", Workspace: "/w", WorkspaceState: "A", BindingsDigest: "b"}
	if !ev.Fresh(ok) {
		t.Fatal("matching context must be fresh")
	}
	stale := ok
	stale.WorkspaceState = "B"
	if ev.Fresh(stale) {
		t.Fatal("a changed workspace state must not be fresh")
	}
}
