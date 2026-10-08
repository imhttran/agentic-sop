package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

const taskColumns = `id, title, objective, acceptance_criteria, execution_mode, status, blocked_reason,
	       attempt, max_attempts, created_at, updated_at`

type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path.
//
// Foreign-key enforcement is configured through the driver DSN so that every
// connection database/sql opens for this Store has foreign_keys enabled, not
// merely the first one.
func Open(path string) (*Store, error) {
	db, err := sql.Open(driverName, "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	store := &Store{db: db}

	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// schemaVersion is the current on-disk schema version.
const schemaVersion = 3

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("unsupported schema version: %d", version)
	}

	// Apply migrations in order, each recording its own version, so a failure
	// mid-way leaves a consistent, resumable database.
	migrations := map[int]func() error{
		0: s.createSchema,
		1: s.migrateHandoffs,
		2: s.migrateExecutionMode,
	}
	for version < schemaVersion {
		migrate, ok := migrations[version]
		if !ok {
			return fmt.Errorf("missing migration for schema version %d", version)
		}
		if err := migrate(); err != nil {
			return err
		}
		version++
	}
	return nil
}

func (s *Store) createSchema() error {
	schema := `
	CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		objective TEXT,
		acceptance_criteria TEXT,
		status TEXT NOT NULL,
		blocked_reason TEXT,
		attempt INTEGER NOT NULL DEFAULT 0,
		max_attempts INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE TABLE task_dependencies (
		task_id TEXT NOT NULL,
		dependency_task_id TEXT NOT NULL,
		PRIMARY KEY (task_id, dependency_task_id),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (dependency_task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE TABLE task_attempts (
		task_id TEXT NOT NULL,
		number INTEGER NOT NULL,
		status TEXT NOT NULL,
		reason TEXT,
		output TEXT,
		duration INTEGER NOT NULL DEFAULT 0,
		timestamp TEXT NOT NULL,
		PRIMARY KEY (task_id, number),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	PRAGMA user_version = 1;
	`

	_, err := s.db.Exec(schema)
	return err
}

// migrateHandoffs adds the handoff table (schema version 2).
func (s *Store) migrateHandoffs() error {
	schema := `
	CREATE TABLE handoffs (
		task_id TEXT PRIMARY KEY,
		capsule_json TEXT NOT NULL,
		status TEXT NOT NULL,
		content TEXT,
		references_json TEXT,
		compression_error TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	PRAGMA user_version = 2;
	`
	_, err := s.db.Exec(schema)
	return err
}

// migrateExecutionMode adds the per-task execution_mode column (schema version 3)
// so a verify-first task survives a restart. Existing rows default to the
// implement mode.
func (s *Store) migrateExecutionMode() error {
	if _, err := s.db.Exec(`ALTER TABLE tasks ADD COLUMN execution_mode TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	_, err := s.db.Exec("PRAGMA user_version = 3")
	return err
}

func (s *Store) Save(task *domain.Task) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertTask(tx, task); err != nil {
		return err
	}
	return tx.Commit()
}

// upsertTask writes a task row and its child collections (dependencies and
// attempts) into an open transaction. A true upsert (ON CONFLICT DO UPDATE)
// updates the row in place; INSERT OR REPLACE would delete the row first,
// cascade-deleting the dependency edges other tasks hold on this one. Child
// collections are replaced from the value, so a full task round-trips its
// history rather than losing it.
//
// It is composed of the phase-addressable helpers upsertTaskRow,
// replaceDependencies, and replaceAttempts so that callers needing
// order-independent multi-task writes (see ReplaceGraph) can run the row
// phase for every task before writing any dependency edge. Save keeps the
// original single-task sequence.
func upsertTask(tx *sql.Tx, task *domain.Task) error {
	if err := upsertTaskRow(tx, task); err != nil {
		return err
	}
	if err := replaceDependencies(tx, task); err != nil {
		return err
	}
	return replaceAttempts(tx, task)
}

// upsertTaskRow upserts only the tasks row for task, leaving child collections
// untouched. It uses INSERT ... ON CONFLICT(id) DO UPDATE (never INSERT OR
// REPLACE) so updating an existing task does not cascade-delete the dependency
// edges other tasks hold on it.
func upsertTaskRow(tx *sql.Tx, task *domain.Task) error {
	_, err := tx.Exec(`
		INSERT INTO tasks
		(id, title, objective, acceptance_criteria, execution_mode, status, blocked_reason, attempt, max_attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			objective = excluded.objective,
			acceptance_criteria = excluded.acceptance_criteria,
			execution_mode = excluded.execution_mode,
			status = excluded.status,
			blocked_reason = excluded.blocked_reason,
			attempt = excluded.attempt,
			max_attempts = excluded.max_attempts,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at
	`, task.ID, task.Title, task.Objective, task.AcceptanceCriteria, string(task.ExecutionMode),
		string(task.Status), string(task.BlockedReason), task.Attempt, task.MaxAttempts,
		task.CreatedAt.UTC().Format(time.RFC3339Nano), task.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

// replaceDependencies deletes this task's own dependency edges and reinserts
// them from the value. Other tasks' edges are never touched.
func replaceDependencies(tx *sql.Tx, task *domain.Task) error {
	if _, err := tx.Exec("DELETE FROM task_dependencies WHERE task_id = ?", task.ID); err != nil {
		return err
	}
	return insertDependencies(tx, task)
}

// replaceAttempts deletes this task's own attempt rows and reinserts them from
// the value. Other tasks' attempts are never touched.
func replaceAttempts(tx *sql.Tx, task *domain.Task) error {
	if _, err := tx.Exec("DELETE FROM task_attempts WHERE task_id = ?", task.ID); err != nil {
		return err
	}
	return insertAttempts(tx, task)
}

// ReplaceGraph applies a reconciled task set in a single transaction: every
// task row is upserted (preserving the dependencies and attempts carried on the
// value), its child collections are replaced, and the listed task IDs are
// removed. Deletions cascade to a removed task's dependency edges, attempts,
// and handoffs; no other row is touched.
//
// Writes are phased inside the one transaction: deletions, then every task row,
// then every task's dependency edges, then every task's attempts. Rows are
// written before dependency edges so the graph persists in any order: SQLite
// foreign keys require a referenced task to exist first, so a task listed
// earlier may depend on a task listed later (including a newly added task)
// without a foreign-key failure.
//
// It is the persistence primitive for explicit reconciliation, so a failure
// rolls the whole change back and leaves the previous graph authoritative.
// Callers must have validated the resulting graph first (see
// planflow.Reconcile).
func (s *Store) ReplaceGraph(tasks []*domain.Task, remove []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Phase 1: deletions. Cascades to the removed tasks' edges, attempts, and
	// handoffs.
	for _, id := range remove {
		if _, err := tx.Exec("DELETE FROM tasks WHERE id = ?", id); err != nil {
			return err
		}
	}

	// Phase 2: task rows, before any dependency edge references them.
	for _, task := range tasks {
		if err := upsertTaskRow(tx, task); err != nil {
			return err
		}
	}

	// Phase 3: dependency edges.
	for _, task := range tasks {
		if err := replaceDependencies(tx, task); err != nil {
			return err
		}
	}

	// Phase 4: attempts.
	for _, task := range tasks {
		if err := replaceAttempts(tx, task); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// SaveTasks persists a generated task set in a single transaction. It inserts
// new tasks only: if any task already exists, it fails and leaves the database
// unchanged, so existing execution state is never overwritten.
//
// Rows are written before dependency edges so the set can be persisted in any
// order: SQLite foreign keys require a referenced task to exist first.
func (s *Store) SaveTasks(tasks []*domain.Task) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Existing-ID protection, including duplicates within the batch.
	seen := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		if seen[task.ID] {
			return fmt.Errorf("duplicate task %s in batch", task.ID)
		}
		seen[task.ID] = true

		var exists int
		switch err := tx.QueryRow("SELECT 1 FROM tasks WHERE id = ?", task.ID).Scan(&exists); {
		case err == nil:
			return fmt.Errorf("task %s already exists", task.ID)
		case errors.Is(err, sql.ErrNoRows):
			// new task, expected
		default:
			return err
		}
	}

	// Phase 1: task rows.
	for _, task := range tasks {
		if err := insertTaskRow(tx, task); err != nil {
			return err
		}
	}

	// Phase 2: dependency edges.
	for _, task := range tasks {
		if err := insertDependencies(tx, task); err != nil {
			return err
		}
	}

	// Phase 3: attempts.
	for _, task := range tasks {
		if err := insertAttempts(tx, task); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ClearTasks removes every task from the active task graph in a single
// transaction. It is the completed-plan handoff primitive: the task graph is the
// active-plan association, so releasing it lets a different plan initialize
// without deleting the database or rewriting history. Child rows (dependencies,
// attempts, handoffs) cascade away with their task. Callers must preserve the
// records first (see planflow's archive); ClearTasks itself keeps no copy.
func (s *Store) ClearTasks() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM tasks"); err != nil {
		return err
	}
	return tx.Commit()
}

func insertTaskRow(tx *sql.Tx, task *domain.Task) error {
	_, err := tx.Exec(`
		INSERT INTO tasks
		(id, title, objective, acceptance_criteria, execution_mode, status, blocked_reason, attempt, max_attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.Title, task.Objective, task.AcceptanceCriteria, string(task.ExecutionMode),
		string(task.Status), string(task.BlockedReason), task.Attempt, task.MaxAttempts,
		task.CreatedAt.UTC().Format(time.RFC3339Nano), task.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func insertDependencies(tx *sql.Tx, task *domain.Task) error {
	for _, depID := range task.DependencyIDs {
		if _, err := tx.Exec(
			"INSERT INTO task_dependencies (task_id, dependency_task_id) VALUES (?, ?)",
			task.ID, depID,
		); err != nil {
			return err
		}
	}
	return nil
}

func insertAttempts(tx *sql.Tx, task *domain.Task) error {
	for _, attempt := range task.Attempts {
		if _, err := tx.Exec(`
			INSERT INTO task_attempts
			(task_id, number, status, reason, output, duration, timestamp)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, task.ID, attempt.Number, string(attempt.Status), attempt.Reason, attempt.Output,
			attempt.Duration.Nanoseconds(), attempt.Timestamp.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Get(id string) (*domain.Task, error) {
	row := s.db.QueryRow("SELECT "+taskColumns+" FROM tasks WHERE id = ?", id)

	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := s.loadTaskChildren(task); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *Store) List() ([]*domain.Task, error) {
	rows, err := s.db.Query("SELECT " + taskColumns + " FROM tasks ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		if err := s.loadTaskChildren(task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, allowing the same
// Task scanning logic to back Get and List.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanTask reconstructs the scalar fields of a Task from a single row of the
// tasks table. Child collections are loaded separately by loadTaskChildren.
func scanTask(sc rowScanner) (*domain.Task, error) {
	task := &domain.Task{}
	var status, blockedReason, executionMode string
	var createdAtStr, updatedAtStr string

	err := sc.Scan(&task.ID, &task.Title, &task.Objective, &task.AcceptanceCriteria,
		&executionMode, &status, &blockedReason, &task.Attempt, &task.MaxAttempts, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}

	taskStatus, ok := normalizeStatus(status)
	if !ok {
		return nil, fmt.Errorf("invalid task status: %s", status)
	}
	task.Status = taskStatus
	task.BlockedReason = domain.BlockedReason(blockedReason)
	task.ExecutionMode = domain.ExecutionMode(executionMode)

	if task.CreatedAt, err = parseTimestamp(createdAtStr); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if task.UpdatedAt, err = parseTimestamp(updatedAtStr); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return task, nil
}

// loadTaskChildren populates a Task's persisted child collections so that both
// Get and List return complete domain values.
func (s *Store) loadTaskChildren(task *domain.Task) error {
	if err := s.loadDependencies(task); err != nil {
		return err
	}
	return s.loadAttempts(task)
}

func (s *Store) loadDependencies(task *domain.Task) error {
	rows, err := s.db.Query(
		"SELECT dependency_task_id FROM task_dependencies WHERE task_id = ? ORDER BY dependency_task_id",
		task.ID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	task.DependencyIDs = []string{}
	for rows.Next() {
		var depID string
		if err := rows.Scan(&depID); err != nil {
			return err
		}
		task.DependencyIDs = append(task.DependencyIDs, depID)
	}

	return rows.Err()
}

func (s *Store) loadAttempts(task *domain.Task) error {
	rows, err := s.db.Query(`
		SELECT number, status, reason, output, duration, timestamp
		FROM task_attempts
		WHERE task_id = ?
		ORDER BY number
	`, task.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	task.Attempts = []domain.Attempt{}
	for rows.Next() {
		var number int
		var status, reason, output, timestampStr string
		var durationNs int64

		if err := rows.Scan(&number, &status, &reason, &output, &durationNs, &timestampStr); err != nil {
			return err
		}

		attStatus, ok := normalizeStatus(status)
		if !ok {
			return fmt.Errorf("invalid attempt status: %s", status)
		}

		timestamp, err := parseTimestamp(timestampStr)
		if err != nil {
			return fmt.Errorf("parse attempt timestamp: %w", err)
		}

		task.Attempts = append(task.Attempts, domain.Attempt{
			Number:    number,
			Status:    attStatus,
			Reason:    reason,
			Output:    output,
			Duration:  time.Duration(durationNs),
			Timestamp: timestamp,
		})
	}

	return rows.Err()
}

func parseTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

// legacyStatus maps pre-T001A status values that may still exist in older
// databases onto the current vocabulary. Mapping is kept at the persistence
// boundary so legacy values never leak into the domain model.
var legacyStatus = map[domain.TaskStatus]domain.TaskStatus{
	"IN_PROGRESS": domain.IMPLEMENTING,
	"CI_FAIL":     domain.FIX_REQUIRED,
}

// normalizeStatus maps a legacy value and reports whether the result is a valid
// current status.
func normalizeStatus(raw string) (domain.TaskStatus, bool) {
	status := domain.TaskStatus(raw)
	if mapped, ok := legacyStatus[status]; ok {
		status = mapped
	}
	return status, isValidStatus(status)
}

func isValidStatus(status domain.TaskStatus) bool {
	return status.Valid()
}
