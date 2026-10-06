package orchestration

// This file implements ORCH-008-06: bounded worker replacement and replanning
// within the orchestration envelope.
//
// DETERMINISTIC BOUND
//
// Worker replacement and replanning are bounded by the SAME counters that bound
// the whole envelope: MaxAssignments (total assignments dispatched), MaxWorkers
// (distinct workers), and MaxReplans (bounded strategy changes). The helper
// consumes the remaining assignment envelope on every replacement attempt and
// the independent replan envelope on every replan, and terminates
// deterministically with an explicit typed outcome once either envelope is
// exhausted. It never retries unboundedly and never consults a token or
// provider-specific budget.
//
// NO-PROGRESS REPLAN
//
// Replanning is triggered when a bounded sequence of attempts produced no
// evidence-based progress. The attempt count is bounded by MaxAssignments and
// the replan count by MaxReplans, so a no-progress sequence followed by
// replanning terminates in a bounded number of steps even when the assignment
// envelope is set very high: the replan loop halts with an explicit
// *EnvelopeExhaustedError rather than looping. The no-progress signal is an
// observation (see progress.go); it is NOT the harness no-progress/BLOCK stop,
// which stays with internal/failure and internal/domain.
//
// LIFECYCLE AUTHORITY
//
// This file grants NO lifecycle authority. It defines no transition, approval,
// commit, budget-extension, or completion operation and holds no handle to
// workflow state, providers, models, transports, or filesystems.

import "fmt"

// EnvelopeExhaustedError is the explicit, typed outcome recorded when the
// orchestration envelope is exhausted by worker replacement or replanning. It is
// never a success and defines no lifecycle transition.
type EnvelopeExhaustedError struct {
	// Code is the stable, machine-checkable reason the envelope was exhausted.
	Code BudgetFailureCode
	// Message is a human-readable explanation.
	Message string
}

func (e *EnvelopeExhaustedError) Error() string { return string(e.Code) + ": " + e.Message }

// EnvelopeTracker tracks the remaining assignment and replan envelope across
// bounded worker replacement and replanning. It is deterministic and grants no
// lifecycle authority.
type EnvelopeTracker struct {
	budget  OrchestrationBudget
	used    int
	replans int
}

// NewEnvelopeTracker builds a tracker over the given envelope. A zero budget
// resolves to the documented bounded defaults.
func NewEnvelopeTracker(budget OrchestrationBudget) *EnvelopeTracker {
	return &EnvelopeTracker{budget: budget.ResolveEffective()}
}

// RemainingAssignments is the number of assignments still permitted.
func (t *EnvelopeTracker) RemainingAssignments() int {
	if t == nil {
		return 0
	}
	remaining := t.budget.MaxAssignments - t.used
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RemainingReplans is the number of replans still permitted. It is bounded by
// the distinct MaxReplans envelope, not aliased to the assignment envelope, so
// the documented replan bound exists as its own counter.
func (t *EnvelopeTracker) RemainingReplans() int {
	if t == nil {
		return 0
	}
	remaining := t.budget.Effective().MaxReplans - t.replans
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Replacement consumes one assignment from the envelope for a worker-replacement
// attempt. It returns nil while the envelope remains and an explicit typed
// *EnvelopeExhaustedError once MaxAssignments is exhausted; the refused attempt
// invokes no agent.
func (t *EnvelopeTracker) Replacement() error {
	if t == nil {
		return &EnvelopeExhaustedError{Code: BudgetCodeTooManyAssignments, Message: "no envelope tracker"}
	}
	if t.used >= t.budget.MaxAssignments {
		return &EnvelopeExhaustedError{
			Code:    BudgetCodeTooManyAssignments,
			Message: fmt.Sprintf("worker replacement refused: max_assignments %d exhausted", t.budget.MaxAssignments),
		}
	}
	t.used++
	return nil
}

// Replan consumes one replan from its own bounded envelope. It is bounded by the
// distinct MaxReplans limit rather than the assignment envelope, so a
// no-progress replan loop cannot run longer than MaxReplans even when
// MaxAssignments is set very high, and terminates with an explicit typed
// outcome.
func (t *EnvelopeTracker) Replan() error {
	if t == nil {
		return &EnvelopeExhaustedError{Code: BudgetCodeTooManyReplans, Message: "no envelope tracker"}
	}
	limit := t.budget.Effective().MaxReplans
	if t.replans >= limit {
		return &EnvelopeExhaustedError{
			Code:    BudgetCodeTooManyReplans,
			Message: fmt.Sprintf("replan refused: max_replans %d exhausted", limit),
		}
	}
	t.replans++
	return nil
}

// ReplacementOutcome is the explicit result of a bounded replacement/replan
// sequence. It is non-authoritative and grants no lifecycle authority.
type ReplacementOutcome struct {
	// Attempts is the number of envelope-consuming attempts made.
	Attempts int
	// Replans is the number of replan attempts made.
	Replans int
	// Terminated reports whether the sequence halted because the envelope was
	// exhausted (true) rather than because progress was observed (false).
	Terminated bool
	// NoProgress reports whether the sequence halted after observing no
	// evidence-based progress across the bounded attempts.
	NoProgress bool
	// Err is the explicit typed exhaustion error, when the envelope was exhausted.
	Err error
}

// BoundedReplacement runs a deterministic worker-replacement / replan sequence
// bounded by the assignment and replan envelopes. Each attempt observes the
// supplied progress signal; the sequence stops as soon as evidence-based progress
// is observed, or halts with an explicit typed outcome once MaxAssignments or
// MaxReplans is exhausted. It never loops unboundedly and consults no token or
// provider-specific budget.
//
// Each loop iteration consumes EXACTLY ONE assignment from the envelope: an
// attempt that makes no progress converts into a replan (consuming the replan
// envelope, not a second assignment) and the loop retries. Both envelopes are
// therefore honoured independently and the total iteration count is bounded by
// min(MaxAssignments, MaxReplans+1).
//
// progressAt reports whether the attempt produced evidence-based progress. A nil
// progressAt is treated as "no progress", so the sequence always terminates.
func (t *EnvelopeTracker) BoundedReplacement(progressAt func(attempt int) bool) ReplacementOutcome {
	out := ReplacementOutcome{}
	for {
		if err := t.Replacement(); err != nil {
			out.Terminated = true
			out.NoProgress = true
			out.Err = err
			return out
		}
		out.Attempts++
		if progressAt != nil && progressAt(out.Attempts) {
			return out
		}
		// No progress this attempt: request a replan from its own bounded envelope.
		// A replan consumes the replan budget only (never a second assignment), so a
		// persistent no-progress signal is bounded by MaxReplans and terminates
		// deterministically even when MaxAssignments is very high.
		if err := t.Replan(); err != nil {
			out.Terminated = true
			out.NoProgress = true
			out.Err = err
			return out
		}
		out.Replans++
	}
}
