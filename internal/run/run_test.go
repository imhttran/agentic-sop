package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCreatesDirAndState(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "T001")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if r.State().Stage != Created {
		t.Errorf("stage = %s, want CREATED", r.State().Stage)
	}

	raw, err := os.ReadFile(filepath.Join(r.Dir(), "state.json"))
	if err != nil {
		t.Fatalf("state.json not written: %v", err)
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("state.json invalid: %v", err)
	}
	if state.ID != "T001" || state.Stage != Created {
		t.Errorf("state = %+v", state)
	}
}

func TestSetStagePersists(t *testing.T) {
	dir := t.TempDir()
	r, _ := New(dir, "T001")
	if err := r.SetStage(Validating); err != nil {
		t.Fatalf("SetStage failed: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(r.Dir(), "state.json"))
	var state State
	_ = json.Unmarshal(raw, &state)
	if state.Stage != Validating {
		t.Errorf("persisted stage = %s, want VALIDATING", state.Stage)
	}
}

func TestWriteArtifact(t *testing.T) {
	dir := t.TempDir()
	r, _ := New(dir, "T001")
	if err := r.Write("task.md", "# T001\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(r.Dir(), "task.md"))
	if err != nil {
		t.Fatalf("artifact not written: %v", err)
	}
	if string(got) != "# T001\n" {
		t.Errorf("artifact = %q", got)
	}
}

func TestNewRequiresID(t *testing.T) {
	if _, err := New(t.TempDir(), "  "); err == nil {
		t.Error("expected error for an empty id")
	}
}
