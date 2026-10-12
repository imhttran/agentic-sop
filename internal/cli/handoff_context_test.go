package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/handoff"
	"github.com/imhttran/agentic-sop/internal/store"
)

// fakeHandoffStore is a taskSaver that also provides the handoff handoffReader/
// handoffWriter surface, for the RBV-011 wiring tests.
type fakeHandoffStore struct {
	records []handoff.Record
	saved   []handoff.Record
}

func (f *fakeHandoffStore) Save(*domain.Task) error             { return nil }
func (f *fakeHandoffStore) Handoffs() ([]handoff.Record, error) { return f.records, nil }
func (f *fakeHandoffStore) SaveHandoff(r handoff.Record) error {
	f.saved = append(f.saved, r)
	return nil
}

// plainSaver is a taskSaver with no handoff surface.
type plainSaver struct{}

func (plainSaver) Save(*domain.Task) error { return nil }

// TestDependencyHandoffItemsSelectsDependenciesOnly proves a task receives only its
// direct dependencies' capsules, never the full history of prior work.
func TestDependencyHandoffItemsSelectsDependenciesOnly(t *testing.T) {
	fs := &fakeHandoffStore{records: []handoff.Record{
		{TaskID: "S001", Capsule: handoff.Capsule{TaskID: "S001", Summary: "did the work", Files: []string{"a.go"}}},
		{TaskID: "S099", Capsule: handoff.Capsule{TaskID: "S099", Summary: "unrelated work"}},
	}}
	task := &domain.Task{ID: "S002", DependencyIDs: []string{"S001"}}

	items := dependencyHandoffItems(task, fs)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 (dependency only)", len(items))
	}
	if items[0].Identity != "handoff-S001" || !strings.Contains(items[0].Text, "did the work") {
		t.Errorf("item = %+v, want the S001 capsule", items[0])
	}
}

// TestDependencyHandoffItemsSkipsUnwiredStore proves an unwired store is a strict
// no-op.
func TestDependencyHandoffItemsSkipsUnwiredStore(t *testing.T) {
	if items := dependencyHandoffItems(&domain.Task{ID: "T", DependencyIDs: []string{"D"}}, plainSaver{}); items != nil {
		t.Errorf("items = %+v, want nil for a store without handoffs", items)
	}
}

// TestHandoffTextPrefersCompressedContent proves the compact rendering: the compressed
// content wins, otherwise the summary and files.
func TestHandoffTextPrefersCompressedContent(t *testing.T) {
	if got := handoffText(handoff.Record{Content: "small"}); got != "small" {
		t.Errorf("handoffText(compressed) = %q, want %q", got, "small")
	}
	got := handoffText(handoff.Record{Capsule: handoff.Capsule{Summary: "did x", Files: []string{"a.go", "b.go"}}})
	if want := "did x\nFiles: a.go, b.go"; got != want {
		t.Errorf("handoffText(capsule) = %q, want %q", got, want)
	}
}

// TestHandoffCapsuleProducedOnCompletion is the end-to-end proof of the producing
// side: with context.handoff enabled, a completed task persists a capsule a dependent
// task can then reuse.
func TestHandoffCapsuleProducedOnCompletion(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\ncontext:\n  handoff: true\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "Implement", Status: domain.PLANNED, MaxAttempts: 3})

	if code, _, stderr := runInjectedCLI(t, dir, "diff\n", &recordingAgent{}, "run"); code != exitOK {
		t.Fatalf("run failed: %s", stderr)
	}
	st, err := store.Open(filepath.Join(dir, stateDirName, stateFileName))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	records, err := st.Handoffs()
	if err != nil {
		t.Fatalf("handoffs: %v", err)
	}
	if len(records) != 1 || records[0].TaskID != "S001" {
		t.Fatalf("handoffs = %+v, want one record for S001", records)
	}
}
