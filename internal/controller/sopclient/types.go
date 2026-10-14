// Package sopclient is the controller's only boundary to SOP. It reads SOP's
// persisted state (state.db and .agent-sdlc artifacts) read-only and delegates
// every action to the sop CLI.
//
// Ownership rule, stated once for the whole package: SOP owns workflow state,
// gates, and decisions. The controller reports SOP-reported values verbatim,
// never infers a gate or status from other signals (BLOCKED, prose, attempt
// counts, inactivity), and renders a missing value as an explicit absence
// rather than zero, PASS, or an empty success.
package sopclient

import "time"

// Task status values are SOP's vocabulary (agentic-sop domain.TaskStatus).
const (
	StatusPlanned        = "PLANNED"
	StatusReady          = "READY"
	StatusBranchCreated  = "BRANCH_CREATED"
	StatusTestsWritten   = "TESTS_WRITTEN"
	StatusRedVerified    = "RED_VERIFIED"
	StatusImplementing   = "IMPLEMENTING"
	StatusLocalTestsPass = "LOCAL_TESTS_PASS"
	StatusReview         = "REVIEW"
	StatusReviewPass     = "REVIEW_PASS"
	StatusPROpen         = "PR_OPEN"
	StatusCIRunning      = "CI_RUNNING"
	StatusCIPass         = "CI_PASS"
	StatusFixRequired    = "FIX_REQUIRED"
	StatusMerged         = "MERGED"
	StatusDone           = "DONE"
	StatusLocalDone      = "LOCAL_DONE"
	StatusBlocked        = "BLOCKED"
)

// IsTerminal reports whether a status is a completed end state. A DONE task was
// merged; a LOCAL_DONE task passed the local lifecycle without a merge.
func IsTerminal(status string) bool {
	switch status {
	case StatusDone, StatusLocalDone, StatusMerged:
		return true
	default:
		return false
	}
}

// TaskState is a coarse display grouping: DONE, RUNNING, BLOCKED, READY, or PLANNED.
func TaskState(status string) string {
	switch {
	case IsTerminal(status):
		return "DONE"
	case status == StatusBlocked:
		return "BLOCKED"
	case status == StatusFixRequired:
		return "FAILED"
	case status == StatusReady:
		return "READY"
	case status == StatusPlanned:
		return "PLANNED"
	default:
		return "RUNNING"
	}
}

// ProjectSummary is the FR-1 project overview.
type ProjectSummary struct {
	ID          string
	Name        string
	Branch      string
	Total       int
	Completed   int
	Running     int
	Ready       int
	Planned     int
	Blocked     int
	FixRequired int
	// NeedsAttention counts tasks with an applicable gate plus pending
	// changed-executed tasks.
	NeedsAttention int
}

// PercentComplete is completed/total as an integer percentage.
func (p ProjectSummary) PercentComplete() int {
	if p.Total == 0 {
		return 0
	}
	return p.Completed * 100 / p.Total
}

// State is a coarse overall execution state for the project.
func (p ProjectSummary) State() string {
	switch {
	case p.FixRequired > 0:
		return "attention"
	case p.Blocked > 0:
		return "blocked"
	case p.Running > 0:
		return "running"
	case p.Total > 0 && p.Completed == p.Total:
		return "complete"
	default:
		return "idle"
	}
}

// ProjectDetail is a project plus its tasks (FR-2).
type ProjectDetail struct {
	Summary ProjectSummary
	Tasks   []TaskSummary
	// PlanSource is the active plan path recorded by SOP, when known.
	PlanSource string
	// Plan is the active plan and its final gate; Recorded is false without metadata.
	Plan PlanGate
	// PlanPerformance is SOP's plan-level aggregate (diagnostic only).
	PlanPerformance PlanPerformance
	// ApprovalsReported is true when SOP returned an approval listing (possibly
	// empty). It is false when the listing could not be read, which is an ABSENCE:
	// a reader MUST NOT render it as "no gates"
	// (docs/specs/HUMAN-APPROVAL.md).
	ApprovalsReported bool
}

// ActiveTask returns the task SOP groups as RUNNING with an in-flight stage.
func (p ProjectDetail) ActiveTask() (string, bool) {
	for _, t := range p.Tasks {
		if t.State() == "RUNNING" && activeStage(t.Stage) {
			return t.ID, true
		}
	}
	return "", false
}

// HasRetryableBlocked reports whether any task is retryable BLOCKED work.
func (p ProjectDetail) HasRetryableBlocked() bool {
	for _, t := range p.Tasks {
		if t.Retryable() {
			return true
		}
	}
	return false
}

// TaskSummary is one row in the project workflow view (FR-2).
type TaskSummary struct {
	ID            string
	Title         string
	Status        string
	BlockedReason string
	Attempt       int
	MaxAttempts   int
	UpdatedAt     time.Time
	// Dependencies are task ids this task depends on.
	Dependencies []string
	// BlockedBy is the subset of dependencies not yet COMPLETED.
	BlockedBy []string
	// Stage is the latest run's lifecycle stage, or "" when the task has not run.
	Stage string
	// Recovery is SOP's recovery disposition for the latest failure, or "".
	Recovery string
	// FixCycles is the latest run's auto-fix cycle count.
	FixCycles int
	// NeedsHuman is true when SOP's approval listing has an applicable gate for
	// this task; it reads the same entry as TaskDetail.Approval.
	NeedsHuman bool
	// ApprovalKind is the gate's kind when NeedsHuman is true.
	ApprovalKind string
}

// State is the coarse display state for this task.
func (t TaskSummary) State() string { return TaskState(t.Status) }

// Retries returns attempt-1, or (0, false) when SOP recorded no attempt yet.
func (t TaskSummary) Retries() (int, bool) {
	if t.Attempt <= 0 {
		return 0, false
	}
	return t.Attempt - 1, true
}

// Eligible reports whether SOP would let this task run now.
func (t TaskSummary) Eligible() bool {
	return len(t.BlockedBy) == 0 &&
		(t.Status == StatusReady || t.Status == StatusPlanned)
}

// Retryable reports whether this is BLOCKED work with retry budget left.
func (t TaskSummary) Retryable() bool {
	return t.Status == StatusBlocked && t.Attempt < t.MaxAttempts
}

// TaskDetail is the FR-3 task view.
type TaskDetail struct {
	TaskSummary
	Objective          string
	AcceptanceCriteria string
	Attempts           []Attempt
	// LatestFailure is the most recent failure/review diagnostic, if any.
	LatestFailure string
	Validation    []ValidationCheck
	Review        Review
	Handoff       *Handoff
	// Run is the latest run's structured record (stage, decision, classification).
	Run RunInfo
	// Activity is the structured activity SOP persisted for the task.
	Activity []ActivityEvent

	// --- CTRL003 structured status ---

	// ValidationStatus is the aggregate validation result over Validation.
	ValidationStatus StatusLevel
	// ReviewStatus is the aggregate review result over Review.
	ReviewStatus StatusLevel
	// QualityStatus is the JEV/quality status SOP persisted, or StatusUnknown.
	QualityStatus StatusLevel
	// RunStatus is the current run's stage/decision; Present is false without
	// run artifacts.
	RunStatus RunStatus
	// Report locates SOP's report artifact for the latest run.
	Report ReportRef

	// Checkpoint is SOP's latest bounded-progress line. Partial progress is never
	// task completion.
	Checkpoint CheckpointProgress

	// Approval is SOP's gate for this task from the approval listing.
	Approval Approval

	// Performance is SOP's latest per-task record (diagnostic only).
	Performance Performance
}

// NeedsHuman reports whether SOP's approval listing has an applicable gate.
func (t TaskDetail) NeedsHuman() bool {
	return t.Approval.Present
}

// Recovering returns the disposition when SOP is recovering on its own
// (AUTO_FIX/CONTINUE/RETRY/REPLAN), or "".
func (t TaskDetail) Recovering() string {
	if t.Run.Classification == nil {
		return ""
	}
	d := t.Run.Classification.Disposition
	if d == DispositionNeedsHuman {
		return ""
	}
	return d
}

// Attempt is one SOP task attempt (FR-4 event source).
type Attempt struct {
	Number    int
	Status    string
	Reason    string
	Duration  time.Duration
	Timestamp time.Time
}

// ValidationCheck is one build/test/lint result (FR-6 CI view).
type ValidationCheck struct {
	Category string
	Command  string
	Status   string
	ExitCode int
	Stderr   string
}

// ReviewFinding is one review finding (FR-5).
type ReviewFinding struct {
	Severity string
	Title    string
	Detail   string
	Location string
}

// Review is the review result for a task (FR-5).
type Review struct {
	Summary  string
	Findings []ReviewFinding
}

// Handoff is the FR-7 handoff context for a task.
type Handoff struct {
	Status           string
	Content          string
	CompressionError string
	CreatedAt        time.Time
}
