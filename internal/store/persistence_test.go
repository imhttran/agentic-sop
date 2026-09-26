package store

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// fixedTime returns a deterministic timestamp without a monotonic clock
// component so that round-tripped values compare predictably.
func fixedTime() time.Time {
	return time.Date(2026, 9, 25, 16, 30, 45, 123456789, time.UTC)
}

// saveDependency saves a completed Task that other Tasks may depend on.
func saveDependency(t *testing.T, s *Store, id string) {
	t.Helper()
	now := fixedTime()
	if err := s.Save(&domain.Task{
		ID:        id,
		Title:     "Dependency " + id,
		Status:    domain.DONE,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Save dependency %s failed: %v", id, err)
	}
}

func TestListHydratesCompleteTasks(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	saveDependency(t, s, "T001")

	now := fixedTime()
	t002 := &domain.Task{
		ID:                 "T002",
		Title:              "Corrections",
		Objective:          "Correct persistence",
		AcceptanceCriteria: "List hydrates children",
		Status:             domain.FIX_REQUIRED,
		BlockedReason:      domain.NO_REASON,
		Attempt:            2,
		MaxAttempts:        3,
		DependencyIDs:      []string{"T001"},
		CreatedAt:          now,
		UpdatedAt:          now.Add(time.Minute),
		Attempts: []domain.Attempt{
			{
				Number:    1,
				Status:    domain.IMPLEMENTING,
				Reason:    "started",
				Output:    "partial output",
				Duration:  1500 * time.Millisecond,
				Timestamp: now,
			},
			{
				Number:    2,
				Status:    domain.FIX_REQUIRED,
				Reason:    "ci red",
				Output:    "tests failed",
				Duration:  2500 * time.Millisecond,
				Timestamp: now.Add(time.Minute),
			},
		},
	}
	if err := s.Save(t002); err != nil {
		t.Fatalf("Save T002 failed: %v", err)
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	got := tasks[1]
	if got.ID != t002.ID {
		t.Fatalf("expected second task to be T002, got %q", got.ID)
	}

	if got.Title != t002.Title ||
		got.Objective != t002.Objective ||
		got.AcceptanceCriteria != t002.AcceptanceCriteria {
		t.Errorf("scalar fields mismatch: got %+v, want %+v", got, t002)
	}
	if got.Status != t002.Status {
		t.Errorf("status mismatch: got %s, want %s", got.Status, t002.Status)
	}
	if got.BlockedReason != t002.BlockedReason {
		t.Errorf("blocked reason mismatch: got %s, want %s", got.BlockedReason, t002.BlockedReason)
	}
	if got.Attempt != t002.Attempt || got.MaxAttempts != t002.MaxAttempts {
		t.Errorf("attempt counters mismatch: got (%d, %d), want (%d, %d)",
			got.Attempt, got.MaxAttempts, t002.Attempt, t002.MaxAttempts)
	}
	if !got.CreatedAt.Equal(t002.CreatedAt) || !got.UpdatedAt.Equal(t002.UpdatedAt) {
		t.Errorf("timestamps mismatch: got (%v, %v), want (%v, %v)",
			got.CreatedAt, got.UpdatedAt, t002.CreatedAt, t002.UpdatedAt)
	}

	if !reflect.DeepEqual(got.DependencyIDs, t002.DependencyIDs) {
		t.Errorf("dependencies mismatch: got %v, want %v", got.DependencyIDs, t002.DependencyIDs)
	}
	if !reflect.DeepEqual(got.Attempts, t002.Attempts) {
		t.Errorf("attempts mismatch:\n got %+v\nwant %+v", got.Attempts, t002.Attempts)
	}
}

func TestGetAndListReturnEquivalentTask(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	saveDependency(t, s, "T001")

	now := fixedTime()
	task := &domain.Task{
		ID:                 "T002",
		Title:              "Equivalent",
		Objective:          "Same shape via both reads",
		AcceptanceCriteria: "Get == List",
		Status:             domain.FIX_REQUIRED,
		BlockedReason:      domain.NO_REASON,
		Attempt:            2,
		MaxAttempts:        3,
		DependencyIDs:      []string{"T001"},
		CreatedAt:          now,
		UpdatedAt:          now.Add(time.Minute),
		Attempts: []domain.Attempt{
			{Number: 1, Status: domain.IMPLEMENTING, Reason: "started", Duration: time.Second, Timestamp: now},
			{Number: 2, Status: domain.FIX_REQUIRED, Reason: "ci red", Output: "boom", Duration: 2 * time.Second, Timestamp: now.Add(time.Minute)},
		},
	}
	if err := s.Save(task); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := s.Get("T002")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	var listed *domain.Task
	for _, cand := range tasks {
		if cand.ID == "T002" {
			listed = cand
			break
		}
	}
	if listed == nil {
		t.Fatal("T002 not found in List output")
	}

	if !reflect.DeepEqual(got, listed) {
		t.Errorf("Get and List disagree:\n Get  %+v\n List %+v", got, listed)
	}
}

func TestForeignKeyEnforcementRejectsMissingDependency(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	task := &domain.Task{
		ID:            "T002",
		Title:         "Missing dependency",
		Status:        domain.READY,
		DependencyIDs: []string{"T999"},
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// T999 does not exist, so the foreign key must reject the save.
	if err := s.Save(task); err == nil {
		t.Fatal("expected Save to fail for a dependency that does not exist")
	}

	// The whole transaction must roll back: no partial task row may remain.
	if _, err := s.Get("T002"); !IsNotFound(err) {
		t.Errorf("expected T002 to be absent after rolled-back Save, got err=%v", err)
	}

	// Persist the dependency, then the same save must succeed.
	saveDependency(t, s, "T999")
	if err := s.Save(task); err != nil {
		t.Fatalf("Save failed after dependency exists: %v", err)
	}

	loaded, err := s.Get("T002")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !reflect.DeepEqual(loaded.DependencyIDs, []string{"T999"}) {
		t.Errorf("dependencies mismatch: got %v, want [T999]", loaded.DependencyIDs)
	}
}

func TestMigrationSetsUserVersion(t *testing.T) {
	dbPath := testDB(t)
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("reading user_version failed: %v", err)
	}
	if version != 2 {
		t.Errorf("expected schema version 2 for a new database, got %d", version)
	}

	saveDependency(t, s, "T001")
	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopening an existing database must not recreate the schema or lose data.
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer s2.Close()

	if err := s2.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("reading user_version after reopen failed: %v", err)
	}
	if version != 2 {
		t.Errorf("expected schema version 2 after reopen, got %d", version)
	}

	if _, err := s2.Get("T001"); err != nil {
		t.Errorf("expected T001 to survive reopen, got err=%v", err)
	}
}

func TestAttemptsSurviveReopenInGetAndList(t *testing.T) {
	dbPath := testDB(t)

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	saveDependency(t, s1, "T001")

	now := fixedTime()
	if err := s1.Save(&domain.Task{
		ID:            "T002",
		Title:         "Survives reopen",
		Status:        domain.FIX_REQUIRED,
		Attempt:       2,
		MaxAttempts:   3,
		DependencyIDs: []string{"T001"},
		CreatedAt:     now,
		UpdatedAt:     now,
		Attempts: []domain.Attempt{
			{Number: 1, Status: domain.IMPLEMENTING, Reason: "started", Duration: time.Second, Timestamp: now},
			{Number: 2, Status: domain.FIX_REQUIRED, Reason: "ci red", Duration: 2 * time.Second, Timestamp: now.Add(time.Minute)},
		},
	}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer s2.Close()

	got, err := s2.Get("T002")
	if err != nil {
		t.Fatalf("Get failed after reopen: %v", err)
	}
	if len(got.Attempts) != 2 {
		t.Fatalf("Get after reopen: expected 2 attempts, got %d", len(got.Attempts))
	}

	tasks, err := s2.List()
	if err != nil {
		t.Fatalf("List failed after reopen: %v", err)
	}
	var listed *domain.Task
	for _, cand := range tasks {
		if cand.ID == "T002" {
			listed = cand
			break
		}
	}
	if listed == nil {
		t.Fatal("T002 not found in List after reopen")
	}
	if len(listed.Attempts) != 2 {
		t.Fatalf("List after reopen: expected 2 attempts, got %d", len(listed.Attempts))
	}

	if !reflect.DeepEqual(got, listed) {
		t.Errorf("Get and List disagree after reopen:\n Get  %+v\n List %+v", got, listed)
	}
}

// Re-saving a Task must not disturb other Tasks that depend on it. An upsert
// must update the row in place; replacing the row would cascade-delete the
// dependency edges that point at it.
func TestResavePreservesOtherTasksDependencies(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	saveDependency(t, s, "T001")
	if err := s.Save(&domain.Task{
		ID:            "T002",
		Status:        domain.READY,
		DependencyIDs: []string{"T001"},
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("Save T002 failed: %v", err)
	}

	// Re-save T001, e.g. to advance its status.
	saveDependency(t, s, "T001")

	got, err := s.Get("T002")
	if err != nil {
		t.Fatalf("Get T002 failed: %v", err)
	}
	if !reflect.DeepEqual(got.DependencyIDs, []string{"T001"}) {
		t.Errorf("re-saving T001 cleared T002's dependency: got %v, want [T001]", got.DependencyIDs)
	}
}

func TestPersistNewStatesRoundTrip(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime()
	states := []domain.TaskStatus{
		domain.BRANCH_CREATED, domain.RED_VERIFIED, domain.REVIEW,
		domain.CI_RUNNING, domain.FIX_REQUIRED, domain.MERGED,
	}

	for i, status := range states {
		id := fmt.Sprintf("T%03d", i+1)
		if err := s.Save(&domain.Task{ID: id, Status: status, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("Save %s (%s) failed: %v", id, status, err)
		}
	}

	for i, status := range states {
		id := fmt.Sprintf("T%03d", i+1)
		got, err := s.Get(id)
		if err != nil {
			t.Fatalf("Get %s failed: %v", id, err)
		}
		if got.Status != status {
			t.Errorf("%s: status = %s, want %s", id, got.Status, status)
		}
	}
}

func TestLegacyStatusesMapOnLoad(t *testing.T) {
	s, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	now := fixedTime().Format(time.RFC3339Nano)

	// Insert legacy values directly, as an older database would contain them.
	if _, err := s.db.Exec(
		"INSERT INTO tasks (id, title, objective, acceptance_criteria, status, blocked_reason, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"T001", "Legacy in progress", "", "", "IN_PROGRESS", "", now, now,
	); err != nil {
		t.Fatalf("insert legacy task failed: %v", err)
	}
	if _, err := s.db.Exec(
		"INSERT INTO tasks (id, title, objective, acceptance_criteria, status, blocked_reason, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"T002", "Legacy ci fail", "", "", "CI_FAIL", "", now, now,
	); err != nil {
		t.Fatalf("insert legacy task failed: %v", err)
	}
	if _, err := s.db.Exec(
		"INSERT INTO task_attempts (task_id, number, status, reason, output, duration, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"T002", 1, "CI_FAIL", "", "", 0, now,
	); err != nil {
		t.Fatalf("insert legacy attempt failed: %v", err)
	}

	got1, err := s.Get("T001")
	if err != nil {
		t.Fatalf("Get T001 failed: %v", err)
	}
	if got1.Status != domain.IMPLEMENTING {
		t.Errorf("IN_PROGRESS mapped to %s, want IMPLEMENTING", got1.Status)
	}

	got2, err := s.Get("T002")
	if err != nil {
		t.Fatalf("Get T002 failed: %v", err)
	}
	if got2.Status != domain.FIX_REQUIRED {
		t.Errorf("CI_FAIL mapped to %s, want FIX_REQUIRED", got2.Status)
	}
	if len(got2.Attempts) != 1 || got2.Attempts[0].Status != domain.FIX_REQUIRED {
		t.Errorf("legacy attempt status not mapped: %+v", got2.Attempts)
	}

	// Re-saving must persist the mapped value, not the legacy string.
	if err := s.Save(got1); err != nil {
		t.Fatalf("re-save failed: %v", err)
	}
	var raw string
	if err := s.db.QueryRow("SELECT status FROM tasks WHERE id = ?", "T001").Scan(&raw); err != nil {
		t.Fatalf("read raw status failed: %v", err)
	}
	if raw != string(domain.IMPLEMENTING) {
		t.Errorf("stored status = %q, want %q", raw, domain.IMPLEMENTING)
	}
}
