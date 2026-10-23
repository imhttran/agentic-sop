// Package question defines SOP's smallest durable question/decision contract: a
// task may raise a structured question, an answer is recorded independently of
// the question, and the question carries a closed state vocabulary that is
// deliberately separate from any run stage (in particular, none of these states
// is the run's WAITING_FOR_PLANNER stage).
//
// The contract mirrors the SEAM-002 provider-neutral decision pattern
// (internal/decision): a closed vocabulary, explicit validation, and fail-closed
// semantics for unknown or malformed records.
package question

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ContractVersion is the question/decision contract version. A stored record
// must declare this version to be interpretable by SOP.
const ContractVersion = 1

// State is the closed question-state vocabulary. It is a question lifecycle
// value and is never a run stage. In particular, none of these values is the
// run's WAITING_FOR_PLANNER stage, and the two vocabularies must never alias.
type State string

const (
	// StatePending: the question is recorded and awaits an answer.
	StatePending State = "PENDING"
	// StateAnswered: an answer has been recorded but not yet applied.
	StateAnswered State = "ANSWERED"
	// StateApplied: the answer has been applied; continuation may dispatch.
	StateApplied State = "APPLIED"
	// StateRejected: the question (or its answer) was rejected with a reason.
	StateRejected State = "REJECTED"
	// StateSuperseded: the question was superseded by a later revision.
	StateSuperseded State = "SUPERSEDED"
)

// Known reports whether s is one of the closed question states. An unrecognized
// state fails closed everywhere in this package.
func (s State) Known() bool {
	switch s {
	case StatePending, StateAnswered, StateApplied, StateRejected, StateSuperseded:
		return true
	default:
		return false
	}
}

// Terminal reports whether s is a terminal question state (no further
// transition is permitted from it).
func (s State) Terminal() bool {
	switch s {
	case StateApplied, StateRejected, StateSuperseded:
		return true
	default:
		return false
	}
}

// OutcomeKind is the closed vocabulary of answer-submission outcomes recorded in
// the audit ledger. It is deliberately distinct from State: a submission outcome
// describes what happened to one submission, while State is the question's
// lifecycle. Neither is a run stage or an approval.
type OutcomeKind string

const (
	// OutcomeAccepted: the answer was durably recorded and the question advanced
	// PENDING -> ANSWERED. Acceptance is NOT application, approval, or execution.
	OutcomeAccepted OutcomeKind = "ACCEPTED"
	// OutcomeDuplicate: an answer with the same idempotency key already exists; the
	// existing answer is returned and no new record is written.
	OutcomeDuplicate OutcomeKind = "DUPLICATE"
	// OutcomeStale: the submission was refused because the question is bound to
	// state that has since changed (task/run/plan revision/repository), or because
	// the caller's expected revision is not current. The prior record is preserved.
	OutcomeStale OutcomeKind = "STALE"
	// OutcomeRejected: the submission was refused for a non-stale reason (for
	// example the question is already terminal, or the body was empty).
	OutcomeRejected OutcomeKind = "REJECTED"
	// OutcomeApplied: the answer was applied (ANSWERED -> APPLIED) and a durable
	// continuation was recorded. Application is not execution and is not an approval.
	OutcomeApplied OutcomeKind = "APPLIED"
)

// Known reports whether k is one of the closed submission outcomes.
func (k OutcomeKind) Known() bool {
	switch k {
	case OutcomeAccepted, OutcomeDuplicate, OutcomeStale, OutcomeRejected, OutcomeApplied:
		return true
	default:
		return false
	}
}

// AllOutcomes returns the closed outcome vocabulary in a stable order, so CLI
// help and MCP schemas can enumerate exactly the contract's outcomes.
func AllOutcomes() []OutcomeKind {
	return []OutcomeKind{OutcomeAccepted, OutcomeDuplicate, OutcomeStale, OutcomeRejected, OutcomeApplied}
}

// Fail-closed sentinels. None of these is ever an approval or a run stage.
var (
	// ErrInvalidRecord: a malformed or contract-violating record (unknown
	// contract version, unknown state, missing identity).
	ErrInvalidRecord = errors.New("question: invalid record")
	// ErrUnknownState: an unrecognized state string was supplied.
	ErrUnknownState = errors.New("question: unknown state")
	// ErrNotFound: no question/answer record exists for the identity.
	ErrNotFound = errors.New("question: record not found")
	// ErrDuplicate: a duplicate submission was observed. It is not fatal; the
	// existing record is returned and the caller observes this outcome.
	ErrDuplicate = errors.New("question: duplicate submission")
	// ErrStale: a submission is stale (its revision is not newer than the
	// current record's revision, or its binding no longer matches authoritative
	// state). It must not overwrite the current record.
	ErrStale = errors.New("question: stale submission")
	// ErrConflict: a submission conflicts with the current record (for example an
	// answer to an already-terminal question) without being a duplicate or stale.
	ErrConflict = errors.New("question: conflicting submission")
	// ErrTerminal: a transition was attempted from a terminal state.
	ErrTerminal = errors.New("question: terminal state")
	// ErrInvalidTransition: a transition is not permitted by the contract.
	ErrInvalidTransition = errors.New("question: invalid transition")
)

// Question is a durable question record. Identity and Kind form the identity
// key: a re-submission with an equal identity key is a duplicate and returns the
// existing record rather than creating a second one.
type Question struct {
	ContractVersion int `json:"contract_version"`
	// ID is the stable record identity.
	ID string `json:"id"`
	// Identity is the caller-supplied identity key component (for example a task
	// id). Identity+Kind is the duplicate key.
	Identity string `json:"identity"`
	// Kind distinguishes the question within an identity.
	Kind string `json:"kind"`
	// Body is the question text.
	Body string `json:"body"`
	// SubmittedBy is the role that submitted the question.
	SubmittedBy string `json:"submitted_by"`
	// Revision is the record's revision ordinal. The create path never replaces
	// an existing record: a re-submission on an existing identity key is a
	// duplicate when its revision equals the current record's, and a stale
	// submission otherwise (see Store.CreateQuestion). Supersession is recorded
	// as the SUPERSEDED state transition, not by re-creation.
	Revision int `json:"revision"`
	// State is the closed question state.
	State State `json:"state"`
	// SupersededBy links to the question that superseded this one, if any.
	SupersededBy string `json:"superseded_by,omitempty"`
	// Reason records why a question was rejected or superseded.
	Reason string `json:"reason,omitempty"`
	// Binding fields anchor the question to authoritative state so an answer can be
	// validated against the live system (task, run, plan revision, repository)
	// rather than trusting caller-supplied identifiers alone.
	Project string `json:"project,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	// RunEpoch identifies the bound run instance (its creation timestamp), so a
	// question whose run has since been superseded by a newer run for the same task
	// is detected as stale.
	RunEpoch string `json:"run_epoch,omitempty"`
	// Attempt narrows the binding to one execution attempt of the task. A decision
	// recorded against an earlier attempt is stale once the task has moved on, even
	// when the run itself was not replaced. Zero declares no attempt binding, so
	// existing records are unaffected.
	Attempt         int       `json:"attempt,omitempty"`
	PlanRevision    string    `json:"plan_revision,omitempty"`
	RepoFingerprint string    `json:"repo_fingerprint,omitempty"`
	ReturnStage     string    `json:"return_stage,omitempty"`
	Deadline        time.Time `json:"deadline,omitempty"`
	// DeadlineAfter is an INPUT to creation only. When it is positive and no explicit
	// Deadline is set, CreateQuestion derives Deadline from the record's OWN creation time
	// (created_at + DeadlineAfter), so both timestamps share one clock reading. It is never
	// persisted (json:"-"), so a reloaded record keeps its durable Deadline and never
	// re-derives one; a duplicate creation returns the existing record unchanged.
	DeadlineAfter time.Duration `json:"-"`
	Options       []string      `json:"options,omitempty"`
	EvidenceRefs  []string      `json:"evidence_refs,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// Key returns the duplicate identity key (Identity+Kind). It returns "" when
// the identity is empty, so an identity-less record never collides with another
// identity-less record.
func (q *Question) Key() string {
	if q == nil {
		return ""
	}
	identity := strings.TrimSpace(q.Identity)
	if identity == "" {
		return ""
	}
	return identity + "\x00" + strings.TrimSpace(q.Kind)
}

// IsDuplicateOf reports whether other has the same non-empty identity key as q.
func (q *Question) IsDuplicateOf(other *Question) bool {
	if q == nil || other == nil {
		return false
	}
	key := q.Key()
	return key != "" && key == other.Key()
}

// Answer is a durable answer record. It is recorded independently of the
// question: it has its own identity and lifecycle, and recording an answer never
// mutates the question's identity.
type Answer struct {
	ContractVersion int `json:"contract_version"`
	// ID is the stable answer identity (distinct from the question identity).
	ID string `json:"id"`
	// QuestionID references the question this answer addresses.
	QuestionID string `json:"question_id"`
	// Body is the answer text.
	Body string `json:"body"`
	// AnsweredBy is the role that supplied the answer.
	AnsweredBy string `json:"answered_by"`
	// IdempotencyKey makes a re-delivered submission a deterministic duplicate
	// rather than a second answer. Optional: an empty key is never deduplicated.
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	// ExpectedRevision is the question revision the answer was written against. A
	// mismatch at submission time is stale.
	ExpectedRevision int `json:"expected_revision,omitempty"`
	// ChosenAction names the option the planner chose, when the question offered
	// options.
	ChosenAction string `json:"chosen_action,omitempty"`
	// Rationale is the planner's explanation, recorded for audit.
	Rationale string    `json:"rationale,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Outcome is an append-only audit record of one answer submission. It captures
// the accepted/duplicate/stale/rejected outcome, its reason, and the applied
// answer reference, so the decision timeline is reconstructable without
// re-deriving state. Recording an outcome never changes a task's lifecycle.
type Outcome struct {
	ContractVersion int `json:"contract_version"`
	// ID is the stable outcome identity.
	ID string `json:"id"`
	// QuestionID references the question the submission addressed.
	QuestionID string `json:"question_id"`
	// AnswerID references the answer the submission recorded, when one was written.
	AnswerID string `json:"answer_id,omitempty"`
	// Kind is the closed submission outcome.
	Kind OutcomeKind `json:"kind"`
	// Reason explains a non-accepted outcome.
	Reason string `json:"reason,omitempty"`
	// Revision is the question revision observed at submission time.
	Revision int `json:"revision,omitempty"`
	// SubmittedBy is the planner identity that submitted.
	SubmittedBy string `json:"submitted_by,omitempty"`
	// IdempotencyKey is the submission's key, when one was supplied.
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	At             time.Time `json:"at"`
}

// Validate checks a question record against the contract, fail-closed.
func (q *Question) Validate() error {
	if q == nil {
		return fmt.Errorf("%w: nil question", ErrInvalidRecord)
	}
	if q.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: unsupported contract version %d", ErrInvalidRecord, q.ContractVersion)
	}
	if strings.TrimSpace(q.ID) == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidRecord)
	}
	if strings.TrimSpace(q.Identity) == "" {
		return fmt.Errorf("%w: missing identity", ErrInvalidRecord)
	}
	if !q.State.Known() {
		return fmt.Errorf("%w: unknown state %q", ErrUnknownState, q.State)
	}
	return nil
}

// Validate checks an answer record against the contract, fail-closed.
func (a *Answer) Validate() error {
	if a == nil {
		return fmt.Errorf("%w: nil answer", ErrInvalidRecord)
	}
	if a.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: unsupported contract version %d", ErrInvalidRecord, a.ContractVersion)
	}
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidRecord)
	}
	if strings.TrimSpace(a.QuestionID) == "" {
		return fmt.Errorf("%w: missing question_id", ErrInvalidRecord)
	}
	return nil
}

// Validate checks an outcome record against the contract, fail-closed.
func (o *Outcome) Validate() error {
	if o == nil {
		return fmt.Errorf("%w: nil outcome", ErrInvalidRecord)
	}
	if o.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: unsupported contract version %d", ErrInvalidRecord, o.ContractVersion)
	}
	if strings.TrimSpace(o.ID) == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidRecord)
	}
	if strings.TrimSpace(o.QuestionID) == "" {
		return fmt.Errorf("%w: missing question_id", ErrInvalidRecord)
	}
	if !o.Kind.Known() {
		return fmt.Errorf("%w: unknown outcome %q", ErrInvalidRecord, o.Kind)
	}
	return nil
}

// ParseState converts a string to a closed State, failing closed on an
// unrecognized value rather than returning an unrelated zero value.
func ParseState(s string) (State, error) {
	st := State(strings.ToUpper(strings.TrimSpace(s)))
	if !st.Known() {
		return "", fmt.Errorf("%w: %q", ErrUnknownState, s)
	}
	return st, nil
}

// AllStates returns the closed vocabulary in a stable order. It exists so CLI
// help and MCP InputSchema can enumerate exactly the contract's states.
func AllStates() []State {
	return []State{StatePending, StateAnswered, StateApplied, StateRejected, StateSuperseded}
}
