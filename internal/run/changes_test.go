package run

import (
	"os"
	"path/filepath"
	"testing"
)

// Task-scoped change evidence tests (JEV task-evidence assembly).

func TestChangedFilesAbsentIsNil(t *testing.T) {
	r, err := New(t.TempDir(), "T001")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := r.ChangedFiles(); got != nil {
		t.Errorf("ChangedFiles on a fresh run = %v, want nil", got)
	}
}

func TestRecordChangedFilesPersistsAndMerges(t *testing.T) {
	dir := t.TempDir()
	r, _ := New(dir, "T001")

	if err := r.RecordChangedFiles([]string{"internal/a.go", "internal/b.go", "internal/a.go"}); err != nil {
		t.Fatalf("RecordChangedFiles: %v", err)
	}
	// A later invocation adds a file and repeats an earlier one: the union is kept
	// in first-seen order, so earlier task work survives a CONTINUE.
	if err := r.RecordChangedFiles([]string{"internal/b.go", "internal/c.go"}); err != nil {
		t.Fatalf("RecordChangedFiles: %v", err)
	}

	want := []string{"internal/a.go", "internal/b.go", "internal/c.go"}
	got := r.ChangedFiles()
	if len(got) != len(want) {
		t.Fatalf("ChangedFiles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ChangedFiles = %v, want %v", got, want)
		}
	}

	// Reopening the same task reads the accumulated set back (the run directory is
	// keyed by task id and survives every invocation).
	again, _ := New(dir, "T001")
	got = again.ChangedFiles()
	if len(got) != len(want) {
		t.Fatalf("reopened ChangedFiles = %v, want %v", got, want)
	}

	// The artifact is a plain, inspectable list of repository paths under the run
	// directory, beside the other run artifacts.
	if _, err := os.Stat(filepath.Join(r.Dir(), changedFilesName)); err != nil {
		t.Fatalf("change evidence artifact missing: %v", err)
	}
}

func TestRecordChangedFilesIsIdempotentAndBounded(t *testing.T) {
	r, _ := New(t.TempDir(), "T001")

	paths := make([]string, 0, maxTaskChangedFiles*2)
	for i := 0; i < maxTaskChangedFiles*2; i++ {
		paths = append(paths, "p"+string(rune('a'+i%26))+string(rune('a'+i/26))+".go")
	}
	if err := r.RecordChangedFiles(paths); err != nil {
		t.Fatalf("RecordChangedFiles: %v", err)
	}
	first := r.ChangedFiles()
	if len(first) > maxTaskChangedFiles {
		t.Fatalf("ChangedFiles = %d entries, want <= %d", len(first), maxTaskChangedFiles)
	}

	// Recording the same set again is a no-op: the accumulator is a set, so
	// repeated invocations never grow it or reorder it.
	if err := r.RecordChangedFiles(paths); err != nil {
		t.Fatalf("RecordChangedFiles (repeat): %v", err)
	}
	second := r.ChangedFiles()
	if len(second) != len(first) {
		t.Fatalf("repeat ChangedFiles = %d entries, want %d", len(second), len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("repeat reordered the set: %v then %v", first, second)
		}
	}
}

func TestChangedFilesIgnoresBlankAndCorruptArtifact(t *testing.T) {
	r, _ := New(t.TempDir(), "T001")
	if err := r.RecordChangedFiles([]string{"  ", "", "a.go"}); err != nil {
		t.Fatalf("RecordChangedFiles: %v", err)
	}
	if got := r.ChangedFiles(); len(got) != 1 || got[0] != "a.go" {
		t.Errorf("ChangedFiles = %v, want [a.go]", got)
	}

	// A corrupt artifact degrades to no evidence rather than an error: the writer
	// never fails the run over diagnostic evidence.
	if err := os.WriteFile(filepath.Join(r.Dir(), changedFilesName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.ChangedFiles(); got != nil {
		t.Errorf("ChangedFiles on a corrupt artifact = %v, want nil", got)
	}
}
