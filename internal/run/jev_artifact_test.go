package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteJEVArtifactWritesLatestResult asserts the latest JEV result is
// available after a run, in the run directory's jev.json, and parses back to the
// value that was written (JEV010 S2).
func TestWriteJEVArtifactWritesLatestResult(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T010")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	doc := map[string]any{"status": "PASS", "findings": []any{}}
	if err := r.WriteJEVArtifact(doc); err != nil {
		t.Fatalf("WriteJEVArtifact failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(r.Dir(), jevArtifactFileName))
	if err != nil {
		t.Fatalf("jev.json not written: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("jev.json invalid: %v", err)
	}
	if got["status"] != "PASS" {
		t.Errorf("status = %v, want PASS", got["status"])
	}
}

// TestWriteJEVArtifactAppendsHistory asserts repeated results preserve earlier
// evidence append-only: jev.json holds the latest result while
// jev-history.jsonl retains every prior one, order preserved, no line rewritten
// (JEV010 S3).
func TestWriteJEVArtifactAppendsHistory(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T010")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	const results = 3
	for i := 1; i <= results; i++ {
		if err := r.WriteJEVArtifact(map[string]any{"status": "FAIL", "attempt": i}); err != nil {
			t.Fatalf("WriteJEVArtifact %d failed: %v", i, err)
		}
	}

	// Latest-result artifact reflects the last write.
	latest, err := os.ReadFile(filepath.Join(r.Dir(), jevArtifactFileName))
	if err != nil {
		t.Fatalf("jev.json not written: %v", err)
	}
	var last map[string]any
	if err := json.Unmarshal(latest, &last); err != nil {
		t.Fatalf("jev.json invalid: %v", err)
	}
	if last["attempt"] != float64(results) {
		t.Errorf("jev.json attempt = %v, want %d", last["attempt"], results)
	}

	// History retains every result, in order, one per line.
	hist, err := os.ReadFile(filepath.Join(r.Dir(), jevHistoryFileName))
	if err != nil {
		t.Fatalf("jev-history.jsonl not written: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(hist)), "\n")
	if len(lines) != results {
		t.Fatalf("history lines = %d, want %d", len(lines), results)
	}
	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("history line %d invalid: %v", i, err)
		}
		if rec["attempt"] != float64(i+1) {
			t.Errorf("history line %d attempt = %v, want %d", i, rec["attempt"], i+1)
		}
	}
}

// TestWriteJEVArtifactDoesNotTouchExistingPersistence asserts the JEV artifact
// path leaves the run's authoritative persistence untouched: state.json keeps its
// content and no state db appears or is altered (JEV010 S5).
func TestWriteJEVArtifactDoesNotTouchExistingPersistence(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T010")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := r.SetStage(Validating); err != nil {
		t.Fatalf("SetStage failed: %v", err)
	}

	statePath := filepath.Join(r.Dir(), stateFileName)
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state.json: %v", err)
	}

	if err := r.WriteJEVArtifact(map[string]any{"status": "PASS"}); err != nil {
		t.Fatalf("WriteJEVArtifact failed: %v", err)
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("re-read state.json: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("state.json changed: before=%q after=%q", before, after)
	}

	// No state database is created, deleted, or migrated by JEV persistence.
	if _, err := os.Stat(filepath.Join(r.Dir(), "state.db")); !os.IsNotExist(err) {
		t.Errorf("unexpected state.db presence after JEV write: %v", err)
	}

	// The run stage is unchanged: persisting evidence performs no state
	// transition.
	if r.State().Stage != Validating {
		t.Errorf("stage = %s, want VALIDATING", r.State().Stage)
	}
}
