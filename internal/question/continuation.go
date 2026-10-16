package question

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ContinuationState is the closed vocabulary of a continuation's lifecycle. It is a
// durable-dispatch value and is never a run stage or an approval.
type ContinuationState string

const (
	// ContinuationPending: the answer is applied and a continuation is owed.
	ContinuationPending ContinuationState = "PENDING"
	// ContinuationClaimed: a worker holds the continuation under a lease and a
	// generation.
	ContinuationClaimed ContinuationState = "CLAIMED"
	// ContinuationCompleted: the continuation ran; the state is terminal.
	ContinuationCompleted ContinuationState = "COMPLETED"
	// ContinuationSuperseded: the continuation's owed work yielded to a LATER planner
	// question in the same task/run, so it was taken over by the subsequent round. It is
	// terminal and is explicitly NOT completed: unfinished work is never marked complete
	// merely because it parked. A superseded continuation is never claimable and is never
	// replayed.
	ContinuationSuperseded ContinuationState = "SUPERSEDED"
)

// Terminal reports whether a continuation state is terminal (no further transition).
func (s ContinuationState) Terminal() bool {
	return s == ContinuationCompleted || s == ContinuationSuperseded
}

// Known reports whether s is one of the closed continuation states.
func (s ContinuationState) Known() bool {
	switch s {
	case ContinuationPending, ContinuationClaimed, ContinuationCompleted, ContinuationSuperseded:
		return true
	default:
		return false
	}
}

// DefaultContinuationMaxAttempts bounds how many times one continuation may be
// claimed (initial claim plus lease-expiry reclaims) before it stops being retried.
const DefaultContinuationMaxAttempts = 5

// Continuation sentinels. None of these is an approval.
var (
	// ErrClaimFenced: a claim/complete presented a generation that is no longer
	// current (an expired worker whose lease was reclaimed). The operation is
	// rejected so the stale worker cannot mutate the run stage/checkpoint or record
	// completion.
	ErrClaimFenced = errors.New("question: continuation claim fenced by a newer generation")
	// ErrClaimNotHeld: the worker does not hold the continuation's current claim.
	ErrClaimNotHeld = errors.New("question: continuation not held by this worker")
	// ErrContinuationCompleted: the continuation is already completed.
	ErrContinuationCompleted = errors.New("question: continuation already completed")
	// ErrClaimBudgetExhausted: the continuation reached its bounded claim attempts.
	ErrClaimBudgetExhausted = errors.New("question: continuation claim attempts exhausted")
)

// Continuation is the durable record that an applied answer owes a continuation.
// It is created in the SAME transaction that applies the answer, so an observer (or
// a crash) never sees an applied answer without a pending continuation.
type Continuation struct {
	ContractVersion int `json:"contract_version"`
	// ID is the stable continuation identity, derived from the question and its run
	// epoch, so repeated application or recovery addresses the SAME continuation.
	ID string `json:"id"`
	// QuestionID is the question whose answer is being continued.
	QuestionID string `json:"question_id"`
	// TaskID / RunID / RunEpoch bind the continuation to the run it belongs to.
	TaskID   string `json:"task_id"`
	RunID    string `json:"run_id"`
	RunEpoch string `json:"run_epoch"`
	// ReturnStage is the stage to resume (recorded by the executor when it asked).
	ReturnStage string `json:"return_stage,omitempty"`
	// State is the closed continuation state.
	State ContinuationState `json:"state"`
	// Generation is the monotonic claim generation. It increments on every claim or
	// reclaim; a worker's completion/refresh must present the current generation.
	Generation int `json:"generation"`
	// ClaimedBy identifies the worker holding the current claim.
	ClaimedBy string `json:"claimed_by,omitempty"`
	// ClaimedAt is the lease start for the current claim.
	ClaimedAt time.Time `json:"claimed_at,omitempty"`
	// Attempts counts claims (initial + reclaims); bounded by max attempts.
	Attempts int `json:"attempts"`
	// CompletedAt is set once the continuation is recorded completed.
	CompletedAt time.Time `json:"completed_at,omitempty"`
	// Reason records why a continuation was abandoned or a claim refused.
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ContinuationID returns the stable continuation identity for a question and run
// epoch. It is deterministic: the same (question, epoch) always yields the same ID.
func ContinuationID(questionID, runEpoch string) string {
	return "cont:" + strings.TrimSpace(questionID) + ":" + strings.TrimSpace(runEpoch)
}

// Validate checks a continuation record against the contract, fail-closed.
func (c *Continuation) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: nil continuation", ErrInvalidRecord)
	}
	if c.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: unsupported contract version %d", ErrInvalidRecord, c.ContractVersion)
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidRecord)
	}
	if strings.TrimSpace(c.QuestionID) == "" {
		return fmt.Errorf("%w: missing question_id", ErrInvalidRecord)
	}
	if !c.State.Known() {
		return fmt.Errorf("%w: unknown continuation state %q", ErrInvalidRecord, c.State)
	}
	if c.Generation < 0 {
		return fmt.Errorf("%w: negative generation %d", ErrInvalidRecord, c.Generation)
	}
	return nil
}

// leaseExpired reports whether a CLAIMED continuation's lease has lapsed at now.
func leaseExpired(c *Continuation, now time.Time, lease time.Duration) bool {
	if c.State != ContinuationClaimed {
		return false
	}
	if lease <= 0 {
		return true
	}
	return !c.ClaimedAt.IsZero() && now.Sub(c.ClaimedAt) >= lease
}
