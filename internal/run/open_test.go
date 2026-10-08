package run

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// TestOpenPreservesRunEvidence proves Open never resets an existing run: the
// persisted stage, the attempt records under attempts/, and the approval head all
// survive a call to Open, unlike New which clears attempts/ and rewrites
// state.json. This is the invariant external completion relies on so that
// recording provenance does not destroy prior run evidence.
func TestOpenPreservesRunEvidence(t *testing.T) {
	dir := t.TempDir()

	// Seed an existing run with a terminal stage, one attempt record, and a pending
	// approval — the evidence an external completion must not clobber.
	r, err := New(dir, "T001")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.SetStage(Failed); err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	if err := r.WriteAttemptRecord(AttemptRecord{
		Version: AttemptRecordVersion, Attempt: 1, Result: AttemptFailed,
		Timestamp: time.Unix(0, 0).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("WriteAttemptRecord: %v", err)
	}
	if err := r.SaveApproval(domain.ApprovalRequest{
		ID: "T001-gate", TaskID: "T001", Status: domain.ApprovalPending,
	}); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	stateBefore := readFileBytes(t, filepath.Join(r.Dir(), "state.json"))
	attemptBefore := readFileBytes(t, filepath.Join(r.Dir(), "attempts", "001.json"))
	approvalBefore := readFileBytes(t, filepath.Join(r.Dir(), approvalArtifactName))

	// Open binds the same directory without any destructive setup.
	got, err := Open(dir, "T001")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got.Dir() != r.Dir() {
		t.Fatalf("Open dir = %q, want %q", got.Dir(), r.Dir())
	}

	if after := readFileBytes(t, filepath.Join(r.Dir(), "state.json")); string(after) != string(stateBefore) {
		t.Errorf("Open rewrote state.json:\n before=%s\n after =%s", stateBefore, after)
	}
	if after := readFileBytes(t, filepath.Join(r.Dir(), "attempts", "001.json")); string(after) != string(attemptBefore) {
		t.Errorf("Open altered attempts/001.json:\n before=%s\n after =%s", attemptBefore, after)
	}
	if after := readFileBytes(t, filepath.Join(r.Dir(), approvalArtifactName)); string(after) != string(approvalBefore) {
		t.Errorf("Open altered %s:\n before=%s\n after =%s", approvalArtifactName, approvalBefore, after)
	}

	// The terminal stage is still observable after Open.
	if stage, ok := Load(dir, "T001"); !ok || stage != Failed {
		t.Errorf("Load after Open = %q, %v; want FAILED, true", stage, ok)
	}
}

// TestOpenCreatesDirectoryWithoutState proves Open still provides a home for a
// task that never ran: it creates the directory but writes no state.json, so it
// never fabricates a run stage.
func TestOpenCreatesDirectoryWithoutState(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir, "T002")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if fi, err := os.Stat(r.Dir()); err != nil || !fi.IsDir() {
		t.Fatalf("Open did not create the run directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.Dir(), "state.json")); !os.IsNotExist(err) {
		t.Errorf("Open wrote state.json for a fresh run: err=%v", err)
	}
}

// TestOpenRequiresID mirrors New's id validation.
func TestOpenRequiresID(t *testing.T) {
	if _, err := Open(t.TempDir(), "  "); err == nil {
		t.Error("expected an error for an empty id")
	}
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
