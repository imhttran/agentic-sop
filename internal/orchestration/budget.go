package orchestration

// This file implements ORCH-008: orchestration-level budgets.
//
// BUDGET OWNERSHIP SEAM
//
// internal/budget (AGENT-004) remains the SOLE owner of invocation-scoped
// execution budgets (ImplementIterations, FixIterations, StaleIterations,
// ToolCalls). ORCH-008 adds an independent, orchestration-owned envelope that
// bounds MULTI-AGENT execution: how many workers may exist, how many may be
// active at once, how many assignments a run may dispatch, how many
// integration attempts the INTEGRATE stage may perform, and how many bounded
// strategy replans a run may perform. The two budgets are deliberately separate
// types: AGENT-004 budgets answer "how much execution is allowed inside one
// invocation?"; an OrchestrationBudget answers "how many
// workers/assignments/integration passes/replans may one orchestration envelope
// contain?". The orchestration budget neither duplicates nor replaces the
// invocation budget, and internal/budget is never modified to carry
// orchestration fields.
//
// NO TOKEN OR PROVIDER BUDGETS
//
// This type introduces NO token budget and NO provider-specific budget. Every
// field is a plain, provider-neutral integer count that is deterministically
// measurable in Go. There is no field (and no dependency) naming tokens, cost,
// money, a model name, a provider, a transport, or a filesystem path.
//
// DETERMINISTIC BOUND
//
// Every limit has a documented, bounded default and is enforced before any
// worker is invoked. A zero or absent value means "use the default", never
// "unlimited"; a malformed or non-positive override keeps the default, so an
// operator value can never make the envelope unbounded. Multi-agent execution
// therefore cannot multiply execution resources beyond this deterministic
// bound.
//
// ENFORCEMENT POINTS
//
//   - Pre-dispatch: ApplyOrchestrationBudget bounds the assignment/worker counts
//     before any worker executes (Coordinator.Run / RunWithConflicts).
//   - Parallel pool: EffectiveActiveWorkers bounds the concurrent worker pool.
//   - INTEGRATE: IntegrationAttempts accounts each integration pass against
//     MaxIntegrationAttempts so the integration bound is actually enforced.
//   - Replacement/replan: the assignment envelope is consumed by the bounded
//     replacement/replan helper so termination is deterministic, and the replan
//     count is bounded by the independent MaxReplans envelope.
//
// LIFECYCLE AUTHORITY
//
// The budget grants NO lifecycle authority. It defines no transition, approval,
// commit, budget-extension, or completion operation and holds no handle to
// workflow state. It is consumed by the coordinator's pre-dispatch admission and
// the INTEGRATE attempt accounting only.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// OrchestrationBudget is the canonical, provider-neutral bound on a multi-agent
// orchestration envelope. Every field has a deterministic default; a zero or
// negative value that is resolved through Resolve keeps the default, and
// Validate rejects an explicitly negative value so an invalid budget fails
// loudly rather than silently becoming unbounded. It carries no token, cost,
// model, provider, or transport field.
type OrchestrationBudget struct {
	// MaxWorkers bounds the total number of distinct worker units one envelope may
	// contain.
	MaxWorkers int `json:"max_workers"`
	// MaxActiveWorkers bounds how many workers may be active (executing) at once.
	// It composes with ExecutionPolicy.MaxConcurrency: the tighter of the two bounds
	// the parallel pool. It also bounds the sequential mutating-worker path: it is
	// enforced as an admission limit before execution.
	MaxActiveWorkers int `json:"max_active_workers"`
	// MaxAssignments bounds the total number of assignments one run may dispatch.
	// It is the deterministic cap on bounded worker replacement and replan.
	MaxAssignments int `json:"max_assignments"`
	// MaxIntegrationAttempts bounds how many times the INTEGRATE stage may run for
	// one envelope. Exhaustion is an explicit typed finding, never a transition.
	MaxIntegrationAttempts int `json:"max_integration_attempts"`
	// MaxReplans bounds how many bounded strategy replans a run may perform. It is
	// an independent counter from MaxAssignments, so a no-progress replan loop
	// terminates deterministically even when the assignment envelope is large.
	MaxReplans int `json:"max_replans"`
}

// Built-in, bounded defaults. These keep the total execution envelope bounded
// even when an operator configures nothing.
const (
	DefaultMaxWorkers             = 8
	DefaultMaxActiveWorkers       = 4
	DefaultMaxAssignments         = 16
	DefaultMaxIntegrationAttempts = 4
	DefaultMaxReplans             = 3
)

// Environment variable names, following the existing SOP_OLLAMA_* override
// convention. An unset, blank, non-numeric, or non-positive value keeps the
// default, so a malformed value can never disable a limit.
const (
	EnvMaxWorkers             = "SOP_ORCH_MAX_WORKERS"
	EnvMaxActiveWorkers       = "SOP_ORCH_MAX_ACTIVE_WORKERS"
	EnvMaxAssignments         = "SOP_ORCH_MAX_ASSIGNMENTS"
	EnvMaxIntegrationAttempts = "SOP_ORCH_MAX_INTEGRATION_ATTEMPTS"
	EnvMaxReplans             = "SOP_ORCH_MAX_REPLANS"
)

// Defaults returns the built-in orchestration budget. Every limit is positive
// and bounded.
func Defaults() OrchestrationBudget {
	return OrchestrationBudget{
		MaxWorkers:             DefaultMaxWorkers,
		MaxActiveWorkers:       DefaultMaxActiveWorkers,
		MaxAssignments:         DefaultMaxAssignments,
		MaxIntegrationAttempts: DefaultMaxIntegrationAttempts,
		MaxReplans:             DefaultMaxReplans,
	}
}

// Zero reports whether every field is its zero value, which is the "use the
// defaults" sentinel.
func (b OrchestrationBudget) Zero() bool {
	return b.MaxWorkers == 0 && b.MaxActiveWorkers == 0 && b.MaxAssignments == 0 &&
		b.MaxIntegrationAttempts == 0 && b.MaxReplans == 0
}

// Resolve returns base with operator environment overrides applied. A value that
// is absent, blank, non-numeric, or non-positive keeps base. A zero base is
// first replaced with Defaults() so an unconfigured project is byte-for-byte
// bounded: a zero value can never mean unlimited. Passing a nil getenv returns
// the resolved (defaulted) budget unchanged.
//
// ResolveFromEnv applies Resolve with the process environment. It is what the
// enforcement boundary uses, so the documented SOP_ORCH_MAX_* overrides are
// actually consulted rather than silently ignored.
func Resolve(getenv func(string) string, base OrchestrationBudget) OrchestrationBudget {
	if base.Zero() {
		base = Defaults()
	}
	if getenv == nil {
		return base
	}
	b := base
	b.MaxWorkers = positiveInt(getenv(EnvMaxWorkers), b.MaxWorkers)
	b.MaxActiveWorkers = positiveInt(getenv(EnvMaxActiveWorkers), b.MaxActiveWorkers)
	b.MaxAssignments = positiveInt(getenv(EnvMaxAssignments), b.MaxAssignments)
	b.MaxIntegrationAttempts = positiveInt(getenv(EnvMaxIntegrationAttempts), b.MaxIntegrationAttempts)
	b.MaxReplans = positiveInt(getenv(EnvMaxReplans), b.MaxReplans)
	return b
}

// ResolveFromEnv resolves base with the process environment applied via Resolve.
// It is the operator-facing entry point used at the enforcement boundary, so the
// SOP_ORCH_MAX_* environment variables have effect on a run.
func ResolveFromEnv(base OrchestrationBudget) OrchestrationBudget {
	return Resolve(os.Getenv, base)
}

// Effective returns the budget with every non-positive field replaced by its
// documented default, so a partially specified budget is still bounded on every
// axis. It is the accessor enforcement uses when the caller did not opt into
// environment overrides; a field can never be unbounded.
func (b OrchestrationBudget) Effective() OrchestrationBudget {
	d := Defaults()
	out := b
	if out.MaxWorkers <= 0 {
		out.MaxWorkers = d.MaxWorkers
	}
	if out.MaxActiveWorkers <= 0 {
		out.MaxActiveWorkers = d.MaxActiveWorkers
	}
	if out.MaxAssignments <= 0 {
		out.MaxAssignments = d.MaxAssignments
	}
	if out.MaxIntegrationAttempts <= 0 {
		out.MaxIntegrationAttempts = d.MaxIntegrationAttempts
	}
	if out.MaxReplans <= 0 {
		out.MaxReplans = d.MaxReplans
	}
	return out
}

// ResolveEffective returns the budget a coordinator actually enforces: the
// documented operator overrides (SOP_ORCH_MAX_*) applied to a zero-defaulted
// base, then non-positive fields replaced with the documented defaults. It is
// the single enforcement accessor so an operator cannot believe a limit was
// tightened when it was not.
func (b OrchestrationBudget) ResolveEffective() OrchestrationBudget {
	return ResolveFromEnv(b).Effective()
}

// Validate rejects an explicitly negative limit. A negative value can only come
// from an explicit caller (never from Resolve, which keeps the default), so an
// invalid budget fails loudly rather than silently becoming unbounded.
func (b OrchestrationBudget) Validate() error {
	fields := []struct {
		name  string
		value int
	}{
		{"max_workers", b.MaxWorkers},
		{"max_active_workers", b.MaxActiveWorkers},
		{"max_assignments", b.MaxAssignments},
		{"max_integration_attempts", b.MaxIntegrationAttempts},
		{"max_replans", b.MaxReplans},
	}
	for _, f := range fields {
		if f.value < 0 {
			return fmt.Errorf("orchestration budget: %s must not be negative (%d)", f.name, f.value)
		}
	}
	return nil
}

// positiveInt parses s as a positive integer, or returns fallback.
func positiveInt(s string, fallback int) int {
	v := strings.TrimSpace(s)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// BudgetError is the explicit, typed failure recorded when the orchestration
// envelope is exceeded before dispatch. It is never a success and grants no
// lifecycle authority.
type BudgetError struct {
	// Code is the stable, machine-checkable reason the envelope was exceeded.
	Code BudgetFailureCode
	// Message is a human-readable explanation.
	Message string
}

func (e *BudgetError) Error() string { return string(e.Code) + ": " + e.Message }

// BudgetFailureCode is a stable, machine-checkable identifier for why the
// orchestration budget admission rejected an assignment set.
type BudgetFailureCode string

const (
	// BudgetCodeTooManyAssignments: the assignment count exceeds MaxAssignments.
	BudgetCodeTooManyAssignments BudgetFailureCode = "too_many_assignments"
	// BudgetCodeTooManyWorkers: the distinct worker-unit count exceeds MaxWorkers.
	BudgetCodeTooManyWorkers BudgetFailureCode = "too_many_workers"
	// BudgetCodeTooManyActiveWorkers: the active (mutating) worker count exceeds
	// MaxActiveWorkers.
	BudgetCodeTooManyActiveWorkers BudgetFailureCode = "too_many_active_workers"
	// BudgetCodeIntegrationAttemptsExhausted: MaxIntegrationAttempts was reached.
	BudgetCodeIntegrationAttemptsExhausted BudgetFailureCode = "integration_attempts_exhausted"
	// BudgetCodeTooManyReplans: the bounded replan envelope (MaxReplans) was
	// exhausted by worker replacement/replanning.
	BudgetCodeTooManyReplans BudgetFailureCode = "too_many_replans"
)

// ApplyOrchestrationBudget deterministically applies the orchestration envelope
// to an assignment set before any worker executes, mirroring
// AdmitWriteOwnership. It returns an explicit *BudgetError (never a
// partial-admission) when the envelope is exceeded. It performs no I/O,
// consults no provider, mutates nothing, and grants no lifecycle authority.
//
// The decision depends only on the assignment set and the resolved budget, so it
// is independent of assignment order.
func ApplyOrchestrationBudget(budget OrchestrationBudget, assignments []WorkAssignment) error {
	b := budget.ResolveEffective()

	if len(assignments) > b.MaxAssignments {
		return &BudgetError{
			Code:    BudgetCodeTooManyAssignments,
			Message: fmt.Sprintf("assignment count %d exceeds max_assignments %d", len(assignments), b.MaxAssignments),
		}
	}

	workers := distinctWorkers(assignments)
	if len(workers) > b.MaxWorkers {
		return &BudgetError{
			Code:    BudgetCodeTooManyWorkers,
			Message: fmt.Sprintf("worker count %d exceeds max_workers %d", len(workers), b.MaxWorkers),
		}
	}

	// Bound the number of active (concurrently executing) workers. Repository-
	// mutating assignments run sequentially but are still active workers; the
	// active-worker bound must constrain the code path most associated with
	// resource multiplication, not just the non-mutating parallel pool.
	if active := activeWorkerCount(assignments); active > b.MaxActiveWorkers {
		return &BudgetError{
			Code:    BudgetCodeTooManyActiveWorkers,
			Message: fmt.Sprintf("active worker count %d exceeds max_active_workers %d", active, b.MaxActiveWorkers),
		}
	}

	return nil
}

// activeWorkerCount returns the number of repository-mutating assignments, which
// are the assignments the coordinator may execute with real resource cost. It is
// the active-worker demand the envelope must bound. The count is order-
// independent.
func activeWorkerCount(assignments []WorkAssignment) int {
	return len(mutatingAssignments(assignments))
}

// distinctWorkers returns the set of distinct (TaskID, Capability) worker units
// in an assignment set. Distinctness is deterministic and order-independent.
func distinctWorkers(assignments []WorkAssignment) []WorkUnit {
	seen := make(map[WorkUnit]bool, len(assignments))
	out := make([]WorkUnit, 0, len(assignments))
	for _, a := range assignments {
		u := WorkUnit{TaskID: a.TaskID, Capability: a.Capability}
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// EffectiveActiveWorkers returns the tighter of the orchestration budget's
// MaxActiveWorkers and the execution policy's MaxConcurrency. It is always at
// least one, so the parallel pool is always bounded.
func EffectiveActiveWorkers(budget OrchestrationBudget, policy ExecutionPolicy) int {
	b := budget.ResolveEffective()
	limit := b.MaxActiveWorkers
	if c := policy.concurrency(); c < limit {
		limit = c
	}
	if limit < 1 {
		return 1
	}
	return limit
}

// IntegrationAttempts is a bounded, non-authoritative integration-attempt
// counter. It accounts each INTEGRATE pass against MaxIntegrationAttempts so the
// declared integration bound is actually a bound on the total execution
// envelope. Exhaustion is an explicit typed finding (IntegrationRefusedError),
// never a lifecycle transition. It holds no workflow-state handle.
type IntegrationAttempts struct {
	budget   OrchestrationBudget
	sessions map[string]int
}

// NewIntegrationAttempts builds an integration-attempt tracker for the given
// envelope. A zero budget resolves to the documented bounded defaults.
func NewIntegrationAttempts(budget OrchestrationBudget) *IntegrationAttempts {
	return &IntegrationAttempts{budget: budget.ResolveEffective(), sessions: map[string]int{}}
}

// Attempt consumes one integration attempt for the named session (for example a
// task ID), returning nil while the envelope remains and an explicit, typed
// *IntegrationRefusedError once MaxIntegrationAttempts is exhausted. The
// (MaxIntegrationAttempts+1)-th attempt is refused and performs no integration.
func (a *IntegrationAttempts) Attempt(session string) error {
	if a == nil {
		return &IntegrationRefusedError{Budget: DefaultMaxIntegrationAttempts, Attempted: 0}
	}
	if a.sessions == nil {
		a.sessions = map[string]int{}
	}
	used := a.sessions[session]
	limit := a.budget.Effective().MaxIntegrationAttempts
	if used >= limit {
		return &IntegrationRefusedError{Budget: limit, Attempted: used + 1}
	}
	a.sessions[session] = used + 1
	return nil
}

// Remaining reports how many integration attempts remain for a session. It is an
// observation and grants no authority.
func (a *IntegrationAttempts) Remaining(session string) int {
	if a == nil {
		return 0
	}
	limit := a.budget.Effective().MaxIntegrationAttempts
	remaining := limit - a.sessions[session]
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IntegrationRefusedError is the explicit, typed, non-authoritative finding
// returned when further integration is deterministically refused because
// MaxIntegrationAttempts is exhausted. It is never a lifecycle transition and
// defines no BLOCK semantic of its own.
type IntegrationRefusedError struct {
	// Budget is the enforced MaxIntegrationAttempts.
	Budget int
	// Attempted is the ordinal of the refused attempt (1-based).
	Attempted int
}

func (e *IntegrationRefusedError) Error() string {
	return fmt.Sprintf("integration refused: max_integration_attempts %d exhausted (attempt %d)", e.Budget, e.Attempted)
}
