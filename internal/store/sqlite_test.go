package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sdlc/internal/domain"
)

func testDB(t *testing.T) string {
	return filepath.Join(t.TempDir(), "test.db")
}

func TestOpen_creates_database(t *testing.T) {
	dbPath := testDB(t)
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	// Verify schema exists by checking tables
	row := store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'")
	var name string
	if err := row.Scan(&name); err != nil {
		t.Errorf("tasks table not created: %v", err)
	}
}

func TestOpen_existing_database(t *testing.T) {
	dbPath := testDB(t)

	// First open
	store1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("First open failed: %v", err)
	}
	store1.Close()

	// Second open should not fail
	store2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Second open failed: %v", err)
	}
	defer store2.Close()
}

func TestSave_and_Get_roundtrip(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	task := &domain.Task{
		ID:                 "T001",
		Title:              "Bootstrap",
		Objective:          "Set up project",
		AcceptanceCriteria: "Project builds",
		Status:             domain.READY,
		BlockedReason:      domain.NO_REASON,
		Attempt:            1,
		MaxAttempts:        3,
		DependencyIDs:      []string{},
		Attempts:           []domain.Attempt{},
		CreatedAt:          time.Now().UTC().Round(time.Nanosecond),
		UpdatedAt:          time.Now().UTC().Round(time.Nanosecond),
	}

	if err := store.Save(task); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Get("T001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if loaded.ID != task.ID || loaded.Title != task.Title || loaded.Status != task.Status {
		t.Errorf("Task data mismatch: got %+v, want %+v", loaded, task)
	}
}

func TestGet_nonexistent_task(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	_, err = store.Get("NONEXISTENT")
	if !IsNotFound(err) {
		t.Errorf("Expected not-found error, got: %v", err)
	}
}

func TestSave_multiple_tasks(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	for i := 1; i <= 3; i++ {
		task := &domain.Task{
			ID:        "T00" + string(rune('0'+i)),
			Title:     "Task " + string(rune('0'+i)),
			Status:    domain.READY,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := store.Save(task); err != nil {
			t.Fatalf("Save T%d failed: %v", i, err)
		}
	}

	tasks, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(tasks))
	}
}

func TestSave_with_dependencies(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	t1 := &domain.Task{
		ID:        "T001",
		Status:    domain.DONE,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.Save(t1); err != nil {
		t.Fatalf("Save T001 failed: %v", err)
	}

	t2 := &domain.Task{
		ID:            "T002",
		DependencyIDs: []string{"T001"},
		Status:        domain.READY,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := store.Save(t2); err != nil {
		t.Fatalf("Save T002 failed: %v", err)
	}

	loaded, err := store.Get("T002")
	if err != nil {
		t.Fatalf("Get T002 failed: %v", err)
	}

	if len(loaded.DependencyIDs) != 1 || loaded.DependencyIDs[0] != "T001" {
		t.Errorf("Dependencies mismatch: got %v, want [T001]", loaded.DependencyIDs)
	}
}

func TestSave_with_attempts(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC().Round(time.Nanosecond)
	task := &domain.Task{
		ID:        "T001",
		Status:    domain.FIX_REQUIRED,
		Attempt:   2,
		CreatedAt: now,
		UpdatedAt: now,
		Attempts: []domain.Attempt{
			{
				Number:    1,
				Status:    domain.IMPLEMENTING,
				Reason:    "first attempt",
				Duration:  time.Second,
				Timestamp: now,
			},
			{
				Number:    2,
				Status:    domain.FIX_REQUIRED,
				Reason:    "test failure",
				Duration:  2 * time.Second,
				Timestamp: now.Add(time.Second),
			},
		},
	}

	if err := store.Save(task); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Get("T001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(loaded.Attempts) != 2 {
		t.Errorf("Expected 2 attempts, got %d", len(loaded.Attempts))
	}

	if loaded.Attempts[0].Status != domain.IMPLEMENTING || loaded.Attempts[1].Status != domain.FIX_REQUIRED {
		t.Errorf("Attempt statuses mismatch")
	}
}

func TestUpdate_existing_task(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	task := &domain.Task{
		ID:        "T001",
		Status:    domain.READY,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := store.Save(task); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// Update status
	task.Status = domain.IMPLEMENTING
	if err := store.Save(task); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	loaded, err := store.Get("T001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if loaded.Status != domain.IMPLEMENTING {
		t.Errorf("Expected status IMPLEMENTING, got %s", loaded.Status)
	}
}

func TestSave_removes_stale_dependencies(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	// Create dependency tasks first
	for _, id := range []string{"T001", "T002"} {
		dep := &domain.Task{
			ID:        id,
			Status:    domain.DONE,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := store.Save(dep); err != nil {
			t.Fatalf("Save dep %s failed: %v", id, err)
		}
	}

	// Save with dependencies
	task := &domain.Task{
		ID:            "T003",
		DependencyIDs: []string{"T001", "T002"},
		Status:        domain.READY,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := store.Save(task); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// Update to only one dependency
	task.DependencyIDs = []string{"T001"}
	if err := store.Save(task); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	loaded, err := store.Get("T003")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(loaded.DependencyIDs) != 1 || loaded.DependencyIDs[0] != "T001" {
		t.Errorf("Dependencies not updated correctly: got %v, want [T001]", loaded.DependencyIDs)
	}
}

func TestSave_removes_stale_attempts(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()

	// Save with 2 attempts
	task := &domain.Task{
		ID:        "T001",
		Status:    domain.FIX_REQUIRED,
		Attempt:   2,
		CreatedAt: now,
		UpdatedAt: now,
		Attempts: []domain.Attempt{
			{Number: 1, Status: domain.IMPLEMENTING, Timestamp: now},
			{Number: 2, Status: domain.FIX_REQUIRED, Timestamp: now.Add(time.Second)},
		},
	}
	if err := store.Save(task); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// Update to only have 1 attempt
	task.Attempts = []domain.Attempt{
		{Number: 1, Status: domain.IMPLEMENTING, Timestamp: now},
	}
	if err := store.Save(task); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	loaded, err := store.Get("T001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(loaded.Attempts) != 1 {
		t.Errorf("Expected 1 attempt, got %d", len(loaded.Attempts))
	}
}

func TestInvalid_status_rejected(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	// Insert invalid status directly
	_, err = store.db.Exec(`
		INSERT INTO tasks (id, title, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "T001", "Test", "INVALID_STATUS", time.Now().Format(time.RFC3339Nano), time.Now().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Get should reject invalid status
	_, err = store.Get("T001")
	if err == nil {
		t.Errorf("Expected error for invalid status, got nil")
	}
}

func TestDatabase_survives_close_reopen(t *testing.T) {
	dbPath := testDB(t)

	// Save task
	store1, _ := Open(dbPath)
	task := &domain.Task{
		ID:        "T001",
		Title:     "Persist me",
		Status:    domain.DONE,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	store1.Save(task)
	store1.Close()

	// Reopen and verify
	store2, _ := Open(dbPath)
	defer store2.Close()

	loaded, err := store2.Get("T001")
	if err != nil {
		t.Fatalf("Get failed after reopen: %v", err)
	}

	if loaded.Title != "Persist me" || loaded.Status != domain.DONE {
		t.Errorf("Data not persisted: got %+v", loaded)
	}
}

func TestList_ordered_by_id(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	for _, id := range []string{"T003", "T001", "T002"} {
		task := &domain.Task{
			ID:        id,
			Status:    domain.READY,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		store.Save(task)
	}

	tasks, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if tasks[0].ID != "T001" || tasks[1].ID != "T002" || tasks[2].ID != "T003" {
		t.Errorf("Tasks not ordered by ID: got %v", []string{tasks[0].ID, tasks[1].ID, tasks[2].ID})
	}
}

func TestBlockedReason_persists(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	task := &domain.Task{
		ID:            "T001",
		Status:        domain.BLOCKED,
		BlockedReason: domain.RETRIES_EXHAUSTED,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := store.Save(task); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Get("T001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if loaded.BlockedReason != domain.RETRIES_EXHAUSTED {
		t.Errorf("BlockedReason mismatch: got %s, want %s", loaded.BlockedReason, domain.RETRIES_EXHAUSTED)
	}
}

func TestTimestamp_roundtrip(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 25, 16, 30, 45, 123456789, time.UTC)
	task := &domain.Task{
		ID:        "T001",
		Status:    domain.READY,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.Save(task); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Get("T001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !loaded.CreatedAt.Equal(now) || !loaded.UpdatedAt.Equal(now) {
		t.Errorf("Timestamps mismatch: got %v, want %v", loaded.CreatedAt, now)
	}
}

func TestDuration_in_nanoseconds(t *testing.T) {
	store, err := Open(testDB(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	duration := 2*time.Second + 500*time.Millisecond
	task := &domain.Task{
		ID:        "T001",
		Status:    domain.CI_PASS,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Attempts: []domain.Attempt{
			{
				Number:    1,
				Status:    domain.LOCAL_TESTS_PASS,
				Duration:  duration,
				Timestamp: time.Now().UTC(),
			},
		},
	}

	if err := store.Save(task); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Get("T001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if loaded.Attempts[0].Duration != duration {
		t.Errorf("Duration mismatch: got %v, want %v", loaded.Attempts[0].Duration, duration)
	}
}
