package store

import (
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func TestReplaceGraphUpsertsPreservingHistoryAndRemoves(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	keep := &domain.Task{
		ID: "KEEP", Title: "Keep", Status: domain.LOCAL_DONE,
		Attempt: 1, MaxAttempts: 3, DependencyIDs: []string{},
		CreatedAt: now, UpdatedAt: now,
		Attempts: []domain.Attempt{{Number: 1, Status: domain.LOCAL_DONE, Reason: "done", Timestamp: now}},
	}
	drop := &domain.Task{ID: "DROP", Title: "Drop", Status: domain.PLANNED, DependencyIDs: []string{}, CreatedAt: now, UpdatedAt: now}
	for _, task := range []*domain.Task{keep, drop} {
		if err := s.Save(task); err != nil {
			t.Fatalf("seed %s: %v", task.ID, err)
		}
	}

	// Re-upsert the preserved task with a new title while a new task depends on it,
	// and remove a task in the same call.
	renamed := *keep
	renamed.Title = "Keep (renamed)"
	renamed.UpdatedAt = now.Add(time.Minute)
	added := &domain.Task{ID: "NEW", Title: "New", Status: domain.PLANNED, DependencyIDs: []string{"KEEP"}, CreatedAt: now, UpdatedAt: now.Add(time.Minute)}

	if err := s.ReplaceGraph([]*domain.Task{&renamed, added}, []string{"DROP"}); err != nil {
		t.Fatalf("ReplaceGraph: %v", err)
	}

	gotKeep, err := s.Get("KEEP")
	if err != nil {
		t.Fatalf("Get KEEP: %v", err)
	}
	if gotKeep.Title != "Keep (renamed)" {
		t.Errorf("title = %q, want the upserted value", gotKeep.Title)
	}
	if gotKeep.Status != domain.LOCAL_DONE || gotKeep.Attempt != 1 || len(gotKeep.Attempts) != 1 {
		t.Errorf("history lost across upsert: %+v", gotKeep)
	}

	if _, err := s.Get("DROP"); !IsNotFound(err) {
		t.Errorf("DROP should have been removed, err = %v", err)
	}

	gotNew, err := s.Get("NEW")
	if err != nil {
		t.Fatalf("Get NEW: %v", err)
	}
	if len(gotNew.DependencyIDs) != 1 || gotNew.DependencyIDs[0] != "KEEP" {
		t.Errorf("NEW dependencies = %v, want [KEEP]", gotNew.DependencyIDs)
	}
}

func TestReplaceGraphLeavesUnlistedTasksUntouched(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.Save(&domain.Task{ID: "UNTOUCHED", Title: "Untouched", Status: domain.DONE, DependencyIDs: []string{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	other := &domain.Task{ID: "OTHER", Title: "Other", Status: domain.PLANNED, DependencyIDs: []string{}, CreatedAt: now, UpdatedAt: now}
	if err := s.ReplaceGraph([]*domain.Task{other}, nil); err != nil {
		t.Fatalf("ReplaceGraph: %v", err)
	}

	if _, err := s.Get("UNTOUCHED"); err != nil {
		t.Errorf("an unlisted task must survive reconciliation: %v", err)
	}
}
