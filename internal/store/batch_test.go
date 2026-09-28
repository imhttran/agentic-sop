package store

import (
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func batchTask(id string, deps ...string) *domain.Task {
	now := fixedTime()
	return &domain.Task{
		ID:            id,
		Title:         "Task " + id,
		Status:        domain.PLANNED,
		DependencyIDs: deps,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func TestSaveTasksPersistsBatch(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	if err := s.SaveTasks([]*domain.Task{batchTask("S001"), batchTask("S002", "S001")}); err != nil {
		t.Fatalf("SaveTasks failed: %v", err)
	}

	got, err := s.Get("S002")
	if err != nil {
		t.Fatalf("Get S002 failed: %v", err)
	}
	if !reflect.DeepEqual(got.DependencyIDs, []string{"S001"}) {
		t.Errorf("S002 dependencies = %v, want [S001]", got.DependencyIDs)
	}
	if got.Status != domain.PLANNED {
		t.Errorf("S002 status = %s, want PLANNED", got.Status)
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2", len(tasks))
	}
}

func TestSaveTasksHandlesArbitraryOrder(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// S003 appears first but depends on S002, which appears later.
	batch := []*domain.Task{
		batchTask("S003", "S002"),
		batchTask("S001"),
		batchTask("S002", "S001"),
	}
	if err := s.SaveTasks(batch); err != nil {
		t.Fatalf("SaveTasks failed: %v", err)
	}

	got, err := s.Get("S003")
	if err != nil {
		t.Fatalf("Get S003 failed: %v", err)
	}
	if !reflect.DeepEqual(got.DependencyIDs, []string{"S002"}) {
		t.Errorf("S003 dependencies = %v, want [S002]", got.DependencyIDs)
	}
}

func TestSaveTasksRejectsExistingTask(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	saveDependency(t, s, "S001") // existing task with DONE status

	err = s.SaveTasks([]*domain.Task{batchTask("S001"), batchTask("S002")})
	if err == nil {
		t.Fatal("expected error when a task already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want it to mention 'already exists'", err)
	}

	// Existing task untouched, no partial insert of S002.
	existing, err := s.Get("S001")
	if err != nil {
		t.Fatalf("Get S001 failed: %v", err)
	}
	if existing.Status != domain.DONE {
		t.Errorf("S001 status = %s, want DONE (must not be overwritten)", existing.Status)
	}
	if _, err := s.Get("S002"); !IsNotFound(err) {
		t.Errorf("S002 should not have been created, got err=%v", err)
	}
}

func TestSaveTasksRollsBackOnFailure(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// S002 depends on a task that is not in the batch or the database, so the
	// foreign key fails during the edge phase and the whole batch rolls back.
	err = s.SaveTasks([]*domain.Task{batchTask("S001"), batchTask("S002", "S999")})
	if err == nil {
		t.Fatal("expected SaveTasks to fail on a missing dependency")
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("database changed after rollback: %d task(s)", len(tasks))
	}
}

func TestSaveTasksRejectsDuplicateIDsInBatch(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	err = s.SaveTasks([]*domain.Task{batchTask("S001"), batchTask("S001")})
	if err == nil {
		t.Fatal("expected error for duplicate ids in the batch")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want it to mention 'duplicate'", err)
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("nothing should persist after duplicate-id failure, got %d", len(tasks))
	}
}

func TestClearTasksRemovesGraphAndChildren(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	saveDependency(t, s, "S001")
	s002 := batchTask("S002", "S001")
	s002.Attempts = []domain.Attempt{{Number: 1, Status: domain.PLANNED, Reason: "queued", Timestamp: fixedTime()}}
	if err := s.Save(s002); err != nil {
		t.Fatalf("Save S002 failed: %v", err)
	}

	if err := s.ClearTasks(); err != nil {
		t.Fatalf("ClearTasks failed: %v", err)
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("ClearTasks left %d task(s), want 0", len(tasks))
	}

	// Child rows cascade away: an id can be re-used with no stale children.
	if err := s.Save(batchTask("S001")); err != nil {
		t.Fatalf("re-save S001 failed: %v", err)
	}
	got, err := s.Get("S001")
	if err != nil {
		t.Fatalf("Get S001 failed: %v", err)
	}
	if len(got.DependencyIDs) != 0 || len(got.Attempts) != 0 {
		t.Errorf("S001 children = deps %v attempts %v, want empty", got.DependencyIDs, got.Attempts)
	}
}

func TestSaveTasksEmptyIsNoOp(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	if err := s.SaveTasks(nil); err != nil {
		t.Errorf("SaveTasks(nil) = %v, want nil", err)
	}
}
