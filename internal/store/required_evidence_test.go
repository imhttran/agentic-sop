package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func newRequiredEvidenceTask(id string) *domain.Task {
	now := time.Now().UTC()
	return &domain.Task{
		ID: id, Title: "t", Objective: "o", AcceptanceCriteria: "ac",
		Status: domain.PLANNED, BlockedReason: domain.NO_REASON,
		MaxAttempts: 1, Attempts: []domain.Attempt{}, DependencyIDs: []string{},
		CreatedAt: now, UpdatedAt: now,
	}
}

func openRequiredEvidenceStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, path
}

func TestRequiredEvidenceSaveAndReload(t *testing.T) {
	s, _ := openRequiredEvidenceStore(t)
	defer s.Close()

	task := newRequiredEvidenceTask("T1")
	task.RequiredEvidence = []string{"corpus:kb", "source:docs/"}
	if err := s.Save(task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get("T1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.RequiredEvidence) != 2 || got.RequiredEvidence[0] != "corpus:kb" || got.RequiredEvidence[1] != "source:docs/" {
		t.Fatalf("RequiredEvidence = %v, want order-preserving [corpus:kb source:docs/]", got.RequiredEvidence)
	}
}

func TestRequiredEvidenceAbsentIsNil(t *testing.T) {
	s, _ := openRequiredEvidenceStore(t)
	defer s.Close()

	if err := s.Save(newRequiredEvidenceTask("T1")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get("T1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.RequiredEvidence) != 0 {
		t.Fatalf("task without declarations produced %v, want none", got.RequiredEvidence)
	}
}

func TestRequiredEvidenceUpdateAndReload(t *testing.T) {
	s, _ := openRequiredEvidenceStore(t)
	defer s.Close()

	task := newRequiredEvidenceTask("T1")
	task.RequiredEvidence = []string{"corpus:kb"}
	if err := s.Save(task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	task.RequiredEvidence = []string{"source:arch/", "corpus:kb"}
	if err := s.Save(task); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	got, err := s.Get("T1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.RequiredEvidence) != 2 || got.RequiredEvidence[0] != "source:arch/" || got.RequiredEvidence[1] != "corpus:kb" {
		t.Fatalf("after update = %v", got.RequiredEvidence)
	}
}

func TestRequiredEvidenceSurvivesReopen(t *testing.T) {
	s, path := openRequiredEvidenceStore(t)
	task := newRequiredEvidenceTask("T1")
	task.RequiredEvidence = []string{"corpus:kb"}
	if err := s.Save(task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.Get("T1")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if len(got.RequiredEvidence) != 1 || got.RequiredEvidence[0] != "corpus:kb" {
		t.Fatalf("after reopen = %v", got.RequiredEvidence)
	}
}

// TestRequiredEvidenceMigrationFromV4 simulates a version-4 database (no
// required_evidence column) and proves Open migrates it additively.
func TestRequiredEvidenceMigrationFromV4(t *testing.T) {
	s, path := openRequiredEvidenceStore(t)
	if err := s.Save(newRequiredEvidenceTask("T1")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.db.Exec("ALTER TABLE tasks DROP COLUMN required_evidence"); err != nil {
		t.Fatalf("simulate v4 (drop column): %v", err)
	}
	if _, err := s.db.Exec("ALTER TABLE tasks DROP COLUMN blocked_detail"); err != nil {
		t.Fatalf("simulate v4 (drop detail): %v", err)
	}
	if _, err := s.db.Exec("PRAGMA user_version = 4"); err != nil {
		t.Fatalf("simulate v4 (version): %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("migrate v4->v5: %v", err)
	}
	defer s2.Close()
	got, err := s2.Get("T1")
	if err != nil {
		t.Fatalf("Get on migrated db: %v", err)
	}
	if len(got.RequiredEvidence) != 0 {
		t.Fatalf("migrated default = %v, want none", got.RequiredEvidence)
	}
}

// TestRequiredEvidenceRepeatOpenIsIdempotent proves reopening a v5 database does
// not re-run or fail the migration.
func TestRequiredEvidenceRepeatOpenIsIdempotent(t *testing.T) {
	s, path := openRequiredEvidenceStore(t)
	if err := s.Save(newRequiredEvidenceTask("T1")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for i := 0; i < 2; i++ {
		s2, err := Open(path)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		if _, err := s2.Get("T1"); err != nil {
			t.Fatalf("Get reopen %d: %v", i, err)
		}
		s2.Close()
	}
}

// TestRequiredEvidenceInvalidJSONRejected proves corrupt stored data is surfaced
// as an error, never silently converted to an empty requirement list.
func TestRequiredEvidenceInvalidJSONRejected(t *testing.T) {
	s, _ := openRequiredEvidenceStore(t)
	defer s.Close()

	if err := s.Save(newRequiredEvidenceTask("T1")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.db.Exec("UPDATE tasks SET required_evidence = '{not json' WHERE id = 'T1'"); err != nil {
		t.Fatalf("corrupt row: %v", err)
	}
	if _, err := s.Get("T1"); err == nil {
		t.Fatal("Get accepted invalid required_evidence JSON, want error")
	}
}

// TestRequiredEvidencePreservesStatusAndDependencies proves the new column does
// not disturb status, dependencies, or execution history.
func TestRequiredEvidencePreservesStatusAndDependencies(t *testing.T) {
	s, _ := openRequiredEvidenceStore(t)
	defer s.Close()

	if err := s.Save(newRequiredEvidenceTask("T2")); err != nil {
		t.Fatalf("Save dep: %v", err)
	}
	task := newRequiredEvidenceTask("T1")
	task.RequiredEvidence = []string{"corpus:kb"}
	task.DependencyIDs = []string{"T2"}
	task.Status = domain.LOCAL_DONE
	task.Attempts = []domain.Attempt{{Number: 1, Status: domain.PLANNED, Timestamp: time.Now().UTC()}}
	if err := s.Save(task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get("T1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.LOCAL_DONE {
		t.Fatalf("status = %s, want LOCAL_DONE", got.Status)
	}
	if len(got.DependencyIDs) != 1 || got.DependencyIDs[0] != "T2" {
		t.Fatalf("deps = %v", got.DependencyIDs)
	}
	if len(got.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(got.Attempts))
	}
	if len(got.RequiredEvidence) != 1 || got.RequiredEvidence[0] != "corpus:kb" {
		t.Fatalf("required evidence = %v", got.RequiredEvidence)
	}
}

// TestRequiredEvidenceEmptyStringRejected proves an empty stored value is an
// error, so a missing value can never silently drop a requirement list.
func TestRequiredEvidenceEmptyStringRejected(t *testing.T) {
	s, _ := openRequiredEvidenceStore(t)
	defer s.Close()

	if err := s.Save(newRequiredEvidenceTask("T1")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.db.Exec("UPDATE tasks SET required_evidence = '' WHERE id = 'T1'"); err != nil {
		t.Fatalf("empty row: %v", err)
	}
	if _, err := s.Get("T1"); err == nil {
		t.Fatal("Get accepted an empty required_evidence value, want error")
	}
}

// TestBlockedDetailPersistsAndSurvivesReopen proves the exact missing identities
// recorded when a task is blocked survive a database close and reopen.
func TestBlockedDetailPersistsAndSurvivesReopen(t *testing.T) {
	s, path := openRequiredEvidenceStore(t)
	task := newRequiredEvidenceTask("T1")
	task.Status = domain.BLOCKED
	task.BlockedReason = domain.REQUIRED_EVIDENCE_MISSING
	task.BlockedDetail = "corpus:kb, source:docs/"
	if err := s.Save(task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.Get("T1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BlockedReason != domain.REQUIRED_EVIDENCE_MISSING || got.BlockedDetail != "corpus:kb, source:docs/" {
		t.Fatalf("after reopen = %q/%q", got.BlockedReason, got.BlockedDetail)
	}
}

// TestBlockedDetailClearedOnRequeue proves a stale detail cannot describe a later
// task state: requeuing clears the reason and the detail together.
func TestBlockedDetailClearedOnRequeue(t *testing.T) {
	task := newRequiredEvidenceTask("T1")
	task.Status = domain.BLOCKED
	task.BlockedReason = domain.REQUIRED_EVIDENCE_MISSING
	task.BlockedDetail = "corpus:kb"
	if err := task.Requeue(); err != nil {
		t.Fatalf("Requeue: %v", err)
	}
	if task.Status != domain.PLANNED || task.BlockedReason != domain.NO_REASON || task.BlockedDetail != "" {
		t.Fatalf("after requeue = %s/%q/%q", task.Status, task.BlockedReason, task.BlockedDetail)
	}
}
