package sopclient

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"

	_ "modernc.org/sqlite"
)

// Store reads one project's SOP state; this package is the only place that
// knows SOP's schema.
type Store struct {
	root string
	db   *sql.DB
	// checkpoints remembers the last reported checkpoint per task (CTRL010); see
	// checkpoint_cache.go.
	checkpoints checkpointStore
}

// StatePath is the SOP state database for a project root.
func StatePath(root string) string {
	return filepath.Join(root, ".agent-sdlc", "state.db")
}

// OpenStore opens the SOP state database read-only. A missing database keeps the
// "is this an initialized SOP project?" message; any other failure is wrapped with
// the path and its cause, so a permission or I/O error is not mistaken for an
// uninitialized project.
func OpenStore(root string) (*Store, error) {
	path := StatePath(root)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no SOP state at %s (is this an initialized SOP project?)", path)
		}
		return nil, fmt.Errorf("SOP state at %s: %w", path, err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open SOP state at %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open SOP state at %s: %w", path, err)
	}
	return &Store{root: root, db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ProjectID is the same identity discovery assigns.
func (s *Store) ProjectID() string {
	return config.ProjectID(s.root)
}

// Summary builds the FR-1 overview. NeedsAttention counts tasks with a gate plus
// pending changed tasks when SOP reported a changed set. The caller supplies both
// SOP listings.
func (s *Store) Summary(ctx context.Context, changedTasks ChangedTasks, approvals ApprovalsListing) (ProjectSummary, error) {
	name, branch, metaErr := s.projectMeta()
	p := ProjectSummary{ID: s.ProjectID(), Name: name, Branch: branch, ApprovalsReported: approvals.Reported, ChangedTasksReported: changedTasks.Reported}
	p.ConfigError = oneLine(metaErr)

	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM tasks GROUP BY status`)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return p, err
		}
		p.Total += n
		switch {
		case IsTerminal(status):
			p.Completed += n
		case status == StatusBlocked:
			p.Blocked += n
		case status == StatusFixRequired:
			p.FixRequired += n
		case status == StatusReady:
			p.Ready += n
		case status == StatusPlanned:
			p.Planned += n
		default:
			p.Running += n
		}
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if err := rows.Err(); err != nil {
		return p, err
	}

	tasks, err := s.Tasks(ctx, approvals)
	if err != nil {
		return p, err
	}
	for _, t := range tasks {
		if t.NeedsHuman {
			p.NeedsAttention++
		}
	}
	if changedTasks.Reported {
		p.NeedsAttention += len(changedTasks.Pending())
	}
	return p, nil
}

// Tasks returns every task with dependencies resolved (FR-2), projecting
// NeedsHuman from the caller-supplied approval listing.
func (s *Store) Tasks(ctx context.Context, approvals ApprovalsListing) ([]TaskSummary, error) {
	deps, err := s.dependencyMap(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, status, COALESCE(blocked_reason, ''), attempt, max_attempts, updated_at
		FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[string]*TaskSummary{}
	var order []string
	for rows.Next() {
		var t TaskSummary
		var updated string
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.BlockedReason, &t.Attempt, &t.MaxAttempts, &updated); err != nil {
			return nil, err
		}
		t.UpdatedAt, err = parseTime(updated)
		if err != nil {
			t.UpdatedAtError = oneLine(err)
		}
		byID[t.ID] = &t
		order = append(order, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// BlockedBy: dependencies not yet terminal (FR-2 "why not eligible").
	for id, t := range byID {
		t.Dependencies = deps[id]
		for _, dep := range t.Dependencies {
			if d, ok := byID[dep]; !ok || !IsTerminal(d.Status) {
				t.BlockedBy = append(t.BlockedBy, dep)
			}
		}
		ri, riErr := s.runInfo(id)
		t.RunUnreadable = riErr != nil
		t.Stage = ri.Stage
		if ri.Classification != nil {
			t.Recovery = ri.Classification.Disposition
		}
		t.FixCycles = ri.FixCycles
		t.NeedsHuman, t.ApprovalKind = taskNeedsHuman(approvals, id)
	}
	out := make([]TaskSummary, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// Task returns one task with attempts, validation, review, handoff, and the
// CTRL003 status projections (FR-3). It uses the same approval listing as Tasks.
func (s *Store) Task(ctx context.Context, id string, approvals ApprovalsListing) (TaskDetail, error) {
	var d TaskDetail
	var updated string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, status, COALESCE(blocked_reason, ''), attempt, max_attempts,
		       COALESCE(objective, ''), COALESCE(acceptance_criteria, ''), updated_at
		FROM tasks WHERE id = ?`, id).
		Scan(&d.ID, &d.Title, &d.Status, &d.BlockedReason, &d.Attempt, &d.MaxAttempts,
			&d.Objective, &d.AcceptanceCriteria, &updated)
	if err == sql.ErrNoRows {
		return d, ErrTaskNotFound
	}
	if err != nil {
		return d, err
	}
	d.UpdatedAt, err = parseTime(updated)
	if err != nil {
		d.UpdatedAtError = oneLine(err)
	}

	deps, err := s.dependencyMap(ctx)
	if err != nil {
		return d, err
	}
	d.Dependencies = deps[id]
	if len(d.Dependencies) > 0 {
		statuses, err := s.statusMap(ctx)
		if err != nil {
			return d, err
		}
		for _, dep := range d.Dependencies {
			if st, ok := statuses[dep]; !ok || !IsTerminal(st) {
				d.BlockedBy = append(d.BlockedBy, dep)
			}
		}
	}
	if attempts, err := s.attempts(ctx, id); err == nil {
		d.Attempts = attempts
		if n := len(attempts); n > 0 {
			last := attempts[n-1]
			if last.Reason != "" {
				d.LatestFailure = last.Reason
			}
		}
	}
	if h, err := s.handoff(ctx, id); err == nil {
		d.Handoff = h
	}
	var runErr, activityErr error
	d.Run, runErr = s.runInfo(id)
	d.RunError = oneLine(runErr)
	d.Activity, d.ActivitySkipped, activityErr = s.readActivity(id)
	d.ActivityError = oneLine(activityErr)
	d.Stage, d.Recovery = d.Run.Stage, ""
	if d.Run.Classification != nil {
		d.Recovery = d.Run.Classification.Disposition
	}
	if d.LatestFailure == "" && d.Run.Classification != nil {
		d.LatestFailure = d.Run.Classification.Reason
	}
	var validationErr, reviewErr error
	d.Validation, validationErr = s.readValidation(id)
	d.ValidationError = oneLine(validationErr)
	d.Review, reviewErr = s.readReview(id)
	d.ReviewError = oneLine(reviewErr)

	d.RunStatus = d.Run.Status()
	d.ValidationStatus = aggregateValidation(d.Validation)
	d.ReviewStatus = aggregateReview(d.Review)
	d.QualityStatus = aggregateQuality(d.Run.JEV)
	d.Report = s.ReportRef(id)

	// CTRL010: the merge keeps a transient empty re-read from regressing progress.
	// A read error is damage, not an empty re-read, so it is reported and the stale
	// cached value is not replayed.
	cp, cpErr := s.Checkpoint(id)
	d.CheckpointError = oneLine(cpErr)
	if cpErr == nil {
		d.Checkpoint = s.mergeCheckpoint(id, cp)
	} else {
		d.Checkpoint = cp
	}

	d.Approval = s.approval(d, approvals)
	// Mirror the gate onto the embedded summary so list and detail agree.
	d.TaskSummary.NeedsHuman, d.TaskSummary.ApprovalKind = d.Approval.Present, d.Approval.Kind

	d.Performance = s.Performance(id)
	return d, nil
}

func (s *Store) mergeCheckpoint(taskID string, fresh CheckpointProgress) CheckpointProgress {
	return s.checkpoints.get().merge(taskID, fresh)
}

func (s *Store) attempts(ctx context.Context, id string) ([]Attempt, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT number, status, COALESCE(reason, ''), duration, timestamp
		FROM task_attempts WHERE task_id = ? ORDER BY number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attempt
	for rows.Next() {
		var a Attempt
		var ts string
		var dur int64
		if err := rows.Scan(&a.Number, &a.Status, &a.Reason, &dur, &ts); err != nil {
			return nil, err
		}
		a.Duration = time.Duration(dur)
		a.Timestamp, err = parseTime(ts)
		if err != nil {
			a.TimestampError = oneLine(err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) handoff(ctx context.Context, id string) (*Handoff, error) {
	var h Handoff
	var created string
	err := s.db.QueryRowContext(ctx, `
		SELECT status, COALESCE(content, ''), COALESCE(compression_error, ''), created_at
		FROM handoffs WHERE task_id = ?`, id).
		Scan(&h.Status, &h.Content, &h.CompressionError, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h.CreatedAt, err = parseTime(created)
	if err != nil {
		h.CreatedAtError = oneLine(err)
	}
	return &h, nil
}

// taskIDs enumerates task ids without building summaries.
func (s *Store) taskIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) statusMap(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, status FROM tasks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		m[id] = status
	}
	return m, rows.Err()
}

func (s *Store) dependencyMap(ctx context.Context) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT task_id, dependency_task_id FROM task_dependencies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string][]string{}
	for rows.Next() {
		var task, dep string
		if err := rows.Scan(&task, &dep); err != nil {
			return nil, err
		}
		m[task] = append(m[task], dep)
	}
	return m, rows.Err()
}

// projectMeta reads the project name and branch from .agent-sdlc/config.yaml. A
// missing file is a legitimate absence; an unreadable or unusable one is returned
// so the summary can report it instead of quietly presenting the id as the name.
func (s *Store) projectMeta() (name, branch string, err error) {
	cfg, err := config.ReadProjectConfig(s.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("config.yaml: %w", err)
	}
	return cfg.Name, cfg.Branch, nil
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparsable timestamp %q", s)
}
