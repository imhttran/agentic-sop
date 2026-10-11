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

// TestReplaceGraphForwardReferenceIsOrderIndependent proves ReplaceGraph persists a
// graph whose earlier-listed task depends on a task listed later (here both newly
// added): task rows are written before dependency edges, so the foreign key never
// trips regardless of the order the tasks appear in the input slice.
func TestReplaceGraphForwardReferenceIsOrderIndependent(t *testing.T) {
	now := fixedTime()
	// A is listed first and depends on B, which is listed second and is also new.
	a := &domain.Task{ID: "A", Title: "A", Status: domain.PLANNED, DependencyIDs: []string{"B"}, CreatedAt: now, UpdatedAt: now}
	b := &domain.Task{ID: "B", Title: "B", Status: domain.PLANNED, DependencyIDs: []string{}, CreatedAt: now, UpdatedAt: now}

	for _, tc := range []struct {
		name  string
		order []*domain.Task
	}{
		{"forward reference (dependant first)", []*domain.Task{a, b}},
		{"reversed (dependency first)", []*domain.Task{b, a}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Open(testDB(t))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer s.Close()

			if err := s.ReplaceGraph(tc.order, nil); err != nil {
				t.Fatalf("ReplaceGraph: %v", err)
			}
			gotA, err := s.Get("A")
			if err != nil {
				t.Fatalf("Get A: %v", err)
			}
			if len(gotA.DependencyIDs) != 1 || gotA.DependencyIDs[0] != "B" {
				t.Errorf("A dependencies = %v, want [B]", gotA.DependencyIDs)
			}
			if _, err := s.Get("B"); err != nil {
				t.Errorf("Get B: %v", err)
			}
		})
	}
}

// TestReplaceGraphPreservesOtherTasksEdges proves upserting a task does not cascade-
// drop the dependency edges OTHER tasks hold on it: the row write uses
// ON CONFLICT DO UPDATE, never INSERT OR REPLACE.
func TestReplaceGraphPreservesOtherTasksEdges(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.Save(&domain.Task{ID: "BASE", Title: "Base", Status: domain.DONE, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(&domain.Task{ID: "DEP", Title: "Dep", Status: domain.PLANNED, DependencyIDs: []string{"BASE"}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	// Upsert BASE (a title change) without listing DEP in the reconciled set.
	if err := s.ReplaceGraph([]*domain.Task{{ID: "BASE", Title: "Base (v2)", Status: domain.DONE, CreatedAt: now, UpdatedAt: now.Add(time.Minute)}}, nil); err != nil {
		t.Fatalf("ReplaceGraph: %v", err)
	}
	gotDep, err := s.Get("DEP")
	if err != nil {
		t.Fatalf("Get DEP: %v", err)
	}
	if len(gotDep.DependencyIDs) != 1 || gotDep.DependencyIDs[0] != "BASE" {
		t.Errorf("DEP dependencies = %v, want [BASE] (the upsert dropped another task's edge)", gotDep.DependencyIDs)
	}
}

// TestReplaceGraphRollsBackOnFailure proves a failed ReplaceGraph leaves the original
// graph unchanged: a missing dependency fails the edge phase and the transaction rolls
// back, so a surviving task is not partially updated and a new task is not left behind.
func TestReplaceGraphRollsBackOnFailure(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	if err := s.Save(&domain.Task{ID: "KEEP", Title: "Keep", Status: domain.LOCAL_DONE, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	// The batch updates KEEP and adds a task whose only dependency exists nowhere: the
	// edge phase must fail and roll the whole change back.
	renamed := &domain.Task{ID: "KEEP", Title: "Keep (changed)", Status: domain.LOCAL_DONE, CreatedAt: now, UpdatedAt: now.Add(time.Minute)}
	broken := &domain.Task{ID: "BAD", Title: "Bad", Status: domain.PLANNED, DependencyIDs: []string{"NOPE"}, CreatedAt: now, UpdatedAt: now}
	if err := s.ReplaceGraph([]*domain.Task{renamed, broken}, nil); err == nil {
		t.Fatal("expected ReplaceGraph to fail on a missing dependency")
	}

	gotKeep, err := s.Get("KEEP")
	if err != nil {
		t.Fatalf("Get KEEP: %v", err)
	}
	if gotKeep.Title != "Keep" {
		t.Errorf("KEEP = %q, want the original; the transaction did not roll back", gotKeep.Title)
	}
	if _, err := s.Get("BAD"); !IsNotFound(err) {
		t.Errorf("BAD must not survive a rolled-back transaction, err = %v", err)
	}
}
