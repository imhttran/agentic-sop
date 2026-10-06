package orchestration

// This file implements ORCH-004: the Worker Execution Coordinator.
//
// The coordinator runs an ordered set of WorkAssignments through the existing
// agent.Agent capability boundary (via WorkerAdapter / FromAgentResponse),
// collects WorkResults, and reports one explicit, typed AssignmentOutcome per
// assignment. It is an orchestration-layer (S2/S3) stage: it introduces no
// second execution path to the agent and names no provider, model, or
// transport.
//
// SINGLE-LIFECYCLE-AUTHORITY INVARIANT
//
// The coordinator grants workers NO lifecycle authority. It carries no
// transition, approval, or commit operation and holds no handle to workflow
// state. Acceptance of an assignment and acceptance of a result remain
// harness-side deterministic decisions delegated to ValidateAssignment,
// ValidateResult, and internal/domain. A WorkResult is a claim, never success:
// a report containing only completed outcomes does not by itself change task
// state.
//
// WRITE-OWNERSHIP ADMISSION (ORCH-005)
//
// Before any assignment executes, the coordinator applies the ORCH-005
// write-ownership policy to the assignment set. Under the default single-writer
// policy at most one repository-mutating assignment is admitted; under disjoint
// multi-writer every pair of mutating scopes must be provably disjoint. A
// violation is an explicit, typed, order-independent rejection that prevents ALL
// workers from executing (no partial dispatch). This invariant is enforced in Go
// code, never by prompt instructions or caller convention.
//
// ORCHESTRATION BUDGET ADMISSION (ORCH-008)
//
// At the SAME pre-dispatch boundary as ownership admission, the coordinator
// applies the ORCH-008 orchestration envelope (OrchestrationBudget), including
// the operator overrides resolved through Resolve, to bound the assignment set,
// the distinct worker count, and the active (mutating) worker count. When the
// envelope is exceeded, EVERY assignment is rejected with an explicit
// FailureBudget outcome (no worker is invoked) in caller order, so multi-agent
// execution can never multiply execution resources beyond the deterministic
// bound. In parallel mode the effective active-worker pool is the tighter of
// OrchestrationBudget.EffectiveActiveWorkers and ExecutionPolicy.MaxConcurrency.
//
// PROGRESS RECORDS (ORCH-008)
//
// Every report carries a deterministic Progress slice in caller order, derived
// from the outcomes by ProgressFromReport, so consumers can observe worker
// ACTIVITY separately from worker PROGRESS. Progress records are non-
// authoritative observations: they grant no transition, approval, or commit, and
// ClassNoProgress is not the harness no-progress/BLOCK stop.
//
// CONFLICT DETECTION (ORCH-006)
//
// RunWithConflicts applies ORCH-006 conflict detection and its explicit decision
// at the SAME pre-dispatch boundary as ownership admission, before any worker
// executes. A detected conflict (overlapping files/symbols, incompatible patches,
// stale repository identity, changed dependencies, or an outdated-HEAD worker
// result) produces one explicit FailureConflict outcome per involved assignment
// and invokes NO worker (no partial dispatch). Conflict handling grants no
// lifecycle authority: it only prevents a conflicting result from being applied
// silently.
//
// PROVIDER NEUTRALITY AND DETERMINISM
//
// The only execution dependency is the provider-neutral agent.Agent interface.
// Outcomes are always reassembled into the caller-supplied assignment order, so
// the report shape is independent of completion order and of goroutine
// scheduling.

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// ExecutionMode selects how the coordinator runs a set of assignments.
// Sequential is the default: it is deterministic and runs everything one at a
// time. Parallel runs only non-mutating (reasoning/analysis) assignments
// concurrently under a bounded pool while repository-mutating assignments
// remain strictly sequential.
type ExecutionMode string

const (
	// ExecutionSequential runs every assignment one at a time, in caller order.
	ExecutionSequential ExecutionMode = "sequential"
	// ExecutionParallel runs non-mutating assignments concurrently (bounded) and
	// mutating assignments sequentially. Results are still integrated in caller
	// order.
	ExecutionParallel ExecutionMode = "parallel"
)

// FailureCode is a stable, machine-checkable identifier for why an assignment
// did not yield an acceptable result. It is provider-neutral and never implies
// success.
type FailureCode string

const (
	// FailureNone is the zero code: the assignment produced a validated result.
	FailureNone FailureCode = ""
	// FailureInvalidAssignment: ValidateAssignment rejected the assignment before
	// it was ever sent to the agent.
	FailureInvalidAssignment FailureCode = "invalid_assignment"
	// FailureNoAgent: the coordinator had no agent configured (nil agent).
	FailureNoAgent FailureCode = "no_agent"
	// FailureTransport: the agent boundary returned an error while generating.
	FailureTransport FailureCode = "transport_error"
	// FailureInvalidResult: ValidateResult rejected the returned WorkResult.
	FailureInvalidResult FailureCode = "invalid_result"
	// FailureCancelled: the context was cancelled before the assignment produced a
	// result.
	FailureCancelled FailureCode = "cancelled"
	// FailureOwnership: the ORCH-005 write-ownership admission rejected the
	// assignment set before any worker executed. No partial dispatch occurred.
	FailureOwnership FailureCode = "ownership_rejected"
	// FailureConflict: the ORCH-006 conflict detector found a conflict involving
	// this assignment, so no worker executed for it and the conflicting result is
	// never applied silently. It is a distinct, stable code from FailureOwnership.
	FailureConflict FailureCode = "conflict_rejected"
	// FailureBudget: the ORCH-008 orchestration budget admission rejected the
	// assignment set (too many assignments, distinct workers, or active workers)
	// before any worker executed. No partial dispatch occurred.
	FailureBudget FailureCode = "budget_exceeded"
)

// ExecutionPolicy is the caller-supplied, provider-neutral execution policy.
// A zero value is sequential with a concurrency bound of one, which is always
// safe, and applies the single-writer/integrator write-ownership default. A zero
// OrchestrationBudget resolves to the bounded, documented defaults, so the total
// execution envelope is always bounded.
type ExecutionPolicy struct {
	// Mode selects sequential or controlled-parallel execution.
	Mode ExecutionMode
	// MaxConcurrency bounds how many non-mutating assignments may run at once in
	// parallel mode. Values below one are treated as one.
	MaxConcurrency int
	// Ownership selects the ORCH-005 write-ownership policy. The zero value is
	// OwnershipSingleWriter, which admits at most one repository-mutating writer.
	Ownership WriteOwnership
	// Budget is the ORCH-008 orchestration envelope. The zero value resolves to the
	// documented bounded defaults; a malformed value can never make a limit
	// unbounded. It carries no token or provider-specific field.
	Budget OrchestrationBudget
}

// concurrency returns the effective worker-pool bound.
func (p ExecutionPolicy) concurrency() int {
	if p.MaxConcurrency < 1 {
		return 1
	}
	return p.MaxConcurrency
}

// ownership returns the effective write-ownership policy, defaulting to the
// single-writer/integrator mode.
func (p ExecutionPolicy) ownership() WriteOwnership {
	if p.Ownership == "" {
		return OwnershipSingleWriter
	}
	return p.Ownership
}

// AssignmentOutcome is the explicit, typed outcome of executing one assignment.
// Exactly one of Result / Failure is meaningful: when Failure is FailureNone the
// Result carries a claim that passed ValidateResult; otherwise Failure names the
// stable reason and no success is implied. The type exposes no transition,
// approval, or commit field or method.
type AssignmentOutcome struct {
	// AssignmentID and TaskID are the caller-owned correlation IDs.
	AssignmentID string
	TaskID       string
	// Capability is the deterministic capability that was requested.
	Capability agent.Capability
	// Result is the worker's claim. It is only meaningful (and only non-zero) when
	// Failure is FailureNone. It is a claim the harness still validates; it is
	// never itself success.
	Result WorkResult
	// Failure is FailureNone when a validated result was produced, else the stable
	// failure code.
	Failure FailureCode
	// Err is the underlying typed error, when one exists. It is informational;
	// Failure is the authoritative, stable code.
	Err error
}

// OK reports whether the outcome carries a validated (non-failed) result claim.
// It is a statement about the coordinator's own validation, not a lifecycle
// acceptance decision.
func (o AssignmentOutcome) OK() bool { return o.Failure == FailureNone }

// ExecutionReport is the aggregated, provider-neutral result of a coordinator
// run. Outcomes are always in caller-supplied assignment order. The report
// carries no transition, approval, or commit field or method: it is input to
// harness-side validation, never a lifecycle decision.
type ExecutionReport struct {
	// Outcomes are the per-assignment outcomes in caller order.
	Outcomes []AssignmentOutcome
	// Progress are the ORCH-008 non-authoritative progress records derived from
	// Outcomes in caller order. They record worker activity separately from worker
	// progress and grant no lifecycle authority.
	Progress []ProgressRecord
	// Completed, NeedsHuman, and Failed count validated result claims by status.
	// Cancelled counts outcomes that failed because their context was cancelled.
	// Errored counts outcomes that failed for any other typed reason.
	Completed  int
	NeedsHuman int
	Failed     int
	Cancelled  int
	Errored    int
}

// Succeeded reports whether every assignment produced a validated result claim
// with status completed. It is a convenience for callers; it grants no
// authority and changes no state.
func (r ExecutionReport) Succeeded() bool {
	return len(r.Outcomes) > 0 && r.Completed == len(r.Outcomes)
}

// Coordinator executes ordered assignments through the agent.Agent boundary and
// collects typed outcomes. It depends only on the provider-neutral agent.Agent
// interface and the ORCH-002 value types; it holds no provider handle, no
// filesystem handle, and no workflow-state handle.
type Coordinator struct {
	agent agent.Agent
}

// NewCoordinator builds a coordinator backed by the given agent. A nil agent is
// allowed: every assignment then yields an explicit FailureNoAgent outcome,
// never a silent success.
func NewCoordinator(a agent.Agent) *Coordinator { return &Coordinator{agent: a} }

// assignmentWorker is the minimal execution dependency the coordinator needs: an
// object that can Assign a WorkAssignment through the agent boundary. The
// existing *WorkerAdapter satisfies it; tests supply fakes. It is intentionally
// narrow so the coordinator never assumes any lifecycle method.
type assignmentWorker interface {
	Assign(ctx context.Context, a WorkAssignment) (WorkResult, error)
}

// workerFor returns the execution dependency for one assignment. It reuses the
// existing WorkerAdapter (which itself invokes only agent.Agent.Generate) so the
// coordinator introduces no second execution path to the agent. When that
// adapter cannot be constructed (no agent) an explicit no-agent outcome is
// produced by runOne instead.
func (c *Coordinator) workerFor(a WorkAssignment) assignmentWorker {
	if c == nil || c.agent == nil {
		return nil
	}
	unit := WorkUnit{TaskID: a.TaskID, Capability: a.Capability}
	return NewWorkerAdapter(unit, c.agent, a)
}

// Run executes assignments under the policy and returns an aggregated report.
// Outcomes are always in the caller-supplied order regardless of mode or
// completion timing. Running the same assignment set twice with the same
// deterministic agent yields identical reports.
//
// Before any assignment executes, Run applies the ORCH-008 orchestration budget
// and then the ORCH-005 write-ownership admission. If the orchestration envelope
// is exceeded, EVERY assignment is rejected with an explicit FailureBudget
// outcome (no worker is invoked) in caller order. If the ownership policy is
// violated, EVERY assignment is rejected with an explicit FailureOwnership
// outcome (no worker is invoked). In both cases the run can never partially
// dispatch. Both admissions depend only on the assignment set, so the decision
// is independent of assignment order.
func (c *Coordinator) Run(ctx context.Context, policy ExecutionPolicy, assignments []WorkAssignment) ExecutionReport {
	ordered := make([]WorkAssignment, len(assignments))
	copy(ordered, assignments)

	// ORCH-008: enforce the orchestration envelope before any worker executes. This
	// runs first so the total execution envelope is bounded even if ownership would
	// also reject the set.
	if err := ApplyOrchestrationBudget(policy.Budget, ordered); err != nil {
		return summarise(rejectAllBudget(ordered, err))
	}

	if err := AdmitWriteOwnership(policy.ownership(), ordered); err != nil {
		return summarise(rejectAll(ordered, err))
	}

	var outcomes []AssignmentOutcome
	if policy.Mode == ExecutionParallel {
		outcomes = c.runParallel(ctx, policy, ordered)
	} else {
		outcomes = c.runSequential(ctx, ordered)
	}
	return summarise(outcomes)
}

// RunWithConflicts applies ORCH-006 conflict detection and its explicit decision
// at the pre-dispatch boundary, then runs the surviving assignments. It follows
// the same no-partial-dispatch pattern as the ORCH-005 ownership admission: a
// detected conflict produces one explicit FailureConflict outcome per involved
// assignment and invokes NO worker. When no conflict is detected the run is
// identical to Run (which also applies the ORCH-008 budget). Conflict handling
// grants no lifecycle authority and records no transition, approval, or commit.
//
// Outcomes are always in the caller-supplied assignment order. A conflicting
// assignment is never marked applicable: its Failure is FailureConflict and its
// Result is empty.
func (c *Coordinator) RunWithConflicts(ctx context.Context, policy ExecutionPolicy, input ConflictInput, assignments []WorkAssignment) ExecutionReport {
	ordered := make([]WorkAssignment, len(assignments))
	copy(ordered, assignments)

	resolution := DecisionForConflicts(DetectConflicts(input))
	if len(resolution.Findings) > 0 {
		// At least one conflict: no worker runs for any involved assignment. A
		// conflicting result is never applied silently.
		return summarise(rejectConflicts(ordered, resolution))
	}

	// No conflict: behave exactly as Run (including ORCH-008 budget admission).
	return c.Run(ctx, policy, ordered)
}

// ConflictError is the explicit typed error recorded on a FailureConflict
// outcome. It names the aggregate decision, the assignments that actually
// conflicted (the non-applicable set from the resolution), and the assignments
// that were set-wide blocked even though they were not individually involved.
// The involved/blocked split makes the per-assignment conflict semantics visible
// and testable. It is never a success.
type ConflictError struct {
	// Decision is the aggregate conflict decision taken.
	Decision ConflictDecision
	// Assignments lists the non-applicable (actually conflicting) assignment IDs.
	Assignments []string
	// BlockedBy lists the assignment IDs that were not individually involved in a
	// finding but are set-wide blocked because the decision applies to the whole
	// set (no partial dispatch).
	BlockedBy []string
}

func (e *ConflictError) Error() string {
	return "conflict rejected (" + string(e.Decision) + ")"
}

// rejectConflicts produces one explicit FailureConflict outcome per assignment in
// caller order without invoking any worker. The decision is set-wide (no partial
// dispatch): every assignment is rejected. The ConflictError records which
// assignments actually conflicted (involved) and which were blocked only because
// the decision is set-wide, so the intended per-assignment semantics are visible
// and testable rather than collapsed into two identical branches.
func rejectConflicts(assignments []WorkAssignment, resolution ConflictResolution) []AssignmentOutcome {
	involved := make(map[string]bool, len(resolution.NonApplicable))
	for _, id := range resolution.NonApplicable {
		involved[id] = true
	}

	involvedIDs := sortedUnique(resolution.NonApplicable)
	blocked := blockedByDecision(assignments, involved)

	outcomes := make([]AssignmentOutcome, len(assignments))
	for i, a := range assignments {
		outcomes[i] = AssignmentOutcome{
			AssignmentID: a.AssignmentID,
			TaskID:       a.TaskID,
			Capability:   a.Capability,
			Failure:      FailureConflict,
			Err:          &ConflictError{Decision: resolution.Decision, Assignments: involvedIDs, BlockedBy: blocked},
		}
	}
	return outcomes
}

// blockedByDecision returns the sorted assignment IDs that were not individually
// involved in a conflict finding but are set-wide blocked: the decision applies
// to the whole assignment set, so a non-involved assignment is never silently
// executed against a conflicting set.
func blockedByDecision(assignments []WorkAssignment, involved map[string]bool) []string {
	var out []string
	for _, a := range assignments {
		if involved[a.AssignmentID] {
			continue
		}
		out = append(out, a.AssignmentID)
	}
	return sortedUnique(out)
}

// rejectAll produces an explicit FailureOwnership outcome for every assignment
// in caller order without invoking any worker. It is used when ownership
// admission rejects the whole set: no partial dispatch is possible.
func rejectAll(assignments []WorkAssignment, err error) []AssignmentOutcome {
	outcomes := make([]AssignmentOutcome, len(assignments))
	for i, a := range assignments {
		outcomes[i] = AssignmentOutcome{
			AssignmentID: a.AssignmentID,
			TaskID:       a.TaskID,
			Capability:   a.Capability,
			Failure:      FailureOwnership,
			Err:          err,
		}
	}
	return outcomes
}

// rejectAllBudget produces an explicit FailureBudget outcome for every
// assignment in caller order without invoking any worker. It is used when the
// ORCH-008 orchestration budget rejects the whole set: no partial dispatch is
// possible, so the total execution envelope stays within its deterministic bound.
func rejectAllBudget(assignments []WorkAssignment, err error) []AssignmentOutcome {
	outcomes := make([]AssignmentOutcome, len(assignments))
	for i, a := range assignments {
		outcomes[i] = AssignmentOutcome{
			AssignmentID: a.AssignmentID,
			TaskID:       a.TaskID,
			Capability:   a.Capability,
			Failure:      FailureBudget,
			Err:          err,
		}
	}
	return outcomes
}

// runSequential executes assignments one at a time in caller order.
func (c *Coordinator) runSequential(ctx context.Context, assignments []WorkAssignment) []AssignmentOutcome {
	outcomes := make([]AssignmentOutcome, len(assignments))
	for i, a := range assignments {
		outcomes[i] = c.runOne(ctx, a)
	}
	return outcomes
}

// runParallel runs non-mutating assignments concurrently under a bounded pool
// and mutating assignments sequentially, then reassembles outcomes into caller
// order. It never runs a repository-mutating assignment concurrently with any
// other assignment. The parallel pool is bounded by the tighter of the ORCH-008
// budget's EffectiveActiveWorkers and ExecutionPolicy.MaxConcurrency, so
// MaxActiveWorkers always constrains multi-agent execution.
func (c *Coordinator) runParallel(ctx context.Context, policy ExecutionPolicy, assignments []WorkAssignment) []AssignmentOutcome {
	outcomes := make([]AssignmentOutcome, len(assignments))

	// Phase 1: mutating assignments run strictly sequentially, in order.
	for i, a := range assignments {
		if agent.IsRepositoryMutation(a.Capability) {
			outcomes[i] = c.runOne(ctx, a)
		}
	}

	// Phase 2: non-mutating assignments run concurrently under a bounded pool. The
	// bound is the tighter of the budget's MaxActiveWorkers and the policy's
	// MaxConcurrency, so the orchestration envelope always constrains the pool.
	limit := EffectiveActiveWorkers(policy.Budget, policy)
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, a := range assignments {
		if agent.IsRepositoryMutation(a.Capability) {
			continue
		}
		idx, assignment := i, a
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			outcomes[idx] = c.runOne(ctx, assignment)
		}()
	}
	wg.Wait()
	return outcomes
}

// runOne executes a single assignment: it validates the assignment first, runs
// it through the agent boundary, and validates the result. Each distinct failure
// maps to its own stable FailureCode; no path yields a silent success.
func (c *Coordinator) runOne(ctx context.Context, a WorkAssignment) AssignmentOutcome {
	outcome := AssignmentOutcome{
		AssignmentID: a.AssignmentID,
		TaskID:       a.TaskID,
		Capability:   a.Capability,
	}

	// An invalid assignment never reaches the agent.
	if err := ValidateAssignment(a); err != nil {
		outcome.Failure = FailureInvalidAssignment
		outcome.Err = err
		return outcome
	}

	worker := c.workerFor(a)
	if worker == nil {
		outcome.Failure = FailureNoAgent
		outcome.Err = errors.New("coordinator has no agent configured")
		return outcome
	}

	if err := ctx.Err(); err != nil {
		outcome.Failure = FailureCancelled
		outcome.Err = err
		return outcome
	}

	result, err := worker.Assign(ctx, a)
	if err != nil {
		// Distinguish context cancellation from a genuine transport failure.
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			outcome.Failure = FailureCancelled
			outcome.Err = err
			return outcome
		}
		outcome.Failure = FailureTransport
		outcome.Err = err
		return outcome
	}

	// The result is a claim: validate it against the assignment before recording
	// it. A rejected result is an explicit typed failure, never a success.
	if err := ValidateResult(a, result); err != nil {
		outcome.Failure = FailureInvalidResult
		outcome.Result = result
		outcome.Err = err
		return outcome
	}

	outcome.Result = result
	outcome.Failure = FailureNone
	return outcome
}

// summarise aggregates outcomes into a report, preserving caller order, and
// derives the report's non-authoritative progress records in the same order.
func summarise(outcomes []AssignmentOutcome) ExecutionReport {
	report := ExecutionReport{Outcomes: outcomes}
	for _, o := range outcomes {
		if o.Failure == FailureNone {
			switch o.Result.Status {
			case agent.OutcomeCompleted:
				report.Completed++
			case agent.OutcomeNeedsHuman:
				report.NeedsHuman++
			default:
				report.Failed++
			}
			continue
		}
		if o.Failure == FailureCancelled {
			report.Cancelled++
			continue
		}
		report.Errored++
	}
	report.Progress = ProgressFromReport(report, NewProgressBaseline())
	return report
}

// Canonicalize returns a stable, comparable representation of the report's
// outcomes and counts. It is used by determinism tests; it carries no authority
// and mutates nothing.
func (r ExecutionReport) Canonicalize() string {
	lines := make([]string, len(r.Outcomes))
	for i, o := range r.Outcomes {
		lines[i] = o.AssignmentID + "|" + string(o.Capability) + "|" + string(o.Failure) + "|" + string(o.Result.Status)
	}
	sort.Strings(lines)
	out := ""
	for _, line := range lines {
		out += line + "\n"
	}
	return out
}
