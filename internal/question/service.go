package question

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ContinuationDispatch is the continuation dispatcher invoked after a question
// is durably recorded as APPLIED. It is the seam S3 requires: the service records
// application first, then dispatches. The interface is deliberately tiny so the
// ordering is enforced in code, not by convention.
type ContinuationDispatch interface {
	// DispatchContinuation is invoked exactly once per applied question.
	DispatchContinuation(ctx context.Context, questionID string) error
}

// DispatchFunc adapts a function to ContinuationDispatch.
type DispatchFunc func(ctx context.Context, questionID string) error

// DispatchContinuation implements ContinuationDispatch.
func (f DispatchFunc) DispatchContinuation(ctx context.Context, questionID string) error {
	return f(ctx, questionID)
}

// Service is the application service shared by the CLI and MCP surfaces. It owns
// the transition rules and the apply-before-dispatch ordering so neither surface
// reimplements workflow logic.
type Service struct {
	store      Store
	dispatcher ContinuationDispatch
	now        func() time.Time
}

// NewService returns a service over the given store and optional dispatcher.
func NewService(store Store, dispatcher ContinuationDispatch) *Service {
	return &Service{store: store, dispatcher: dispatcher, now: time.Now}
}

// clock returns the service clock in UTC.
func (s *Service) clock() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

// CreateQuestion records a PENDING question. The duplicate/stale outcomes are
// deterministic and returned explicitly.
func (s *Service) CreateQuestion(q *Question) (*Question, error) {
	if q == nil {
		return nil, fmt.Errorf("%w: nil question", ErrInvalidRecord)
	}
	q.State = StatePending
	if q.ContractVersion == 0 {
		q.ContractVersion = ContractVersion
	}
	return s.store.CreateQuestion(q)
}

// CreateAnswer records an answer as a record distinct from the question it
// answers, then advances the question PENDING -> ANSWERED so the documented
// lifecycle is reachable through the public API alone. Recording the answer and
// the state advance are separate records: the answer keeps its own identity and
// the question's identity is never mutated.
func (s *Service) CreateAnswer(a *Answer) (*Answer, error) {
	if a == nil {
		return nil, fmt.Errorf("%w: nil answer", ErrInvalidRecord)
	}
	if a.ContractVersion == 0 {
		a.ContractVersion = ContractVersion
	}
	stored, err := s.store.CreateAnswer(a)
	if err != nil {
		return nil, err
	}
	// Advance the question to ANSWERED. If the question is already terminal the
	// compare-and-set reports applied=false and the answer still stands as its
	// own record; that is deterministic and never an error.
	_, _, err = s.store.ApplyTransition(stored.QuestionID, StatePending, StateAnswered)
	if err != nil {
		return nil, err
	}
	return stored, nil
}

// ReadQuestion returns a question together with its independently recorded
// answers, by identity.
func (s *Service) ReadQuestion(id string) (*Question, []*Answer, error) {
	q, err := s.store.GetQuestion(id)
	if err != nil {
		return nil, nil, err
	}
	answers, err := s.store.AnswersFor(id)
	if err != nil {
		return nil, nil, err
	}
	return q, answers, nil
}

// ListQuestions lists questions in a closed state.
func (s *Service) ListQuestions(state State) ([]*Question, error) {
	if !state.Known() {
		return nil, fmt.Errorf("%w: %q", ErrUnknownState, state)
	}
	return s.store.ListQuestions(state)
}

// ApplyAnswer records the ANSWERED-to-APPLIED transition and only then dispatches
// continuation. The APPLIED record is durably persisted before dispatch: an
// observer between the two calls always sees the APPLIED record first.
//
// Applying an already-APPLIED question is deterministic and idempotent: the
// existing APPLIED record is returned and continuation is NOT dispatched again.
func (s *Service) ApplyAnswer(ctx context.Context, questionID string) (*Question, error) {
	var (
		q       *Question
		applied bool
	)
	err := s.store.Atomic(func(tx Store) (bool, error) {
		// 1) Atomically record the ANSWERED -> APPLIED transition. The store's
		// compare-and-set is the single point that decides who applies: at most one
		// caller observes applied=true, so continuation is dispatched at most once.
		cur, ok, aerr := tx.ApplyTransition(questionID, StateAnswered, StateApplied)
		if aerr != nil {
			return false, aerr
		}
		q, applied = cur, ok
		if !ok {
			return false, nil
		}
		// 2) The applied answer owes a durable continuation, created in the SAME
		// transaction, so an observer or crash never sees APPLIED without a pending
		// continuation. The stable ID makes repeated application target one record.
		if _, cerr := tx.CreateContinuation(&Continuation{
			ID:          ContinuationID(cur.ID, cur.RunEpoch),
			QuestionID:  cur.ID,
			TaskID:      cur.Identity,
			RunID:       cur.RunID,
			RunEpoch:    cur.RunEpoch,
			ReturnStage: cur.ReturnStage,
			State:       ContinuationPending,
		}); cerr != nil {
			return false, cerr
		}
		if aerr := tx.AppendOutcome(&Outcome{QuestionID: cur.ID, Kind: OutcomeApplied, Reason: "answer applied"}); aerr != nil {
			return false, aerr
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if !applied {
		// Not in ANSWERED: deterministic, documented outcomes per current state.
		switch q.State {
		case StateApplied:
			// Idempotent repeat: the APPLIED record already exists; no second
			// dispatch.
			return q, nil
		case StateRejected, StateSuperseded:
			return nil, fmt.Errorf("%w: cannot apply %s question %q", ErrTerminal, q.State, questionID)
		case StatePending:
			return nil, fmt.Errorf("%w: question %q has no answer to apply", ErrInvalidTransition, questionID)
		default:
			return nil, fmt.Errorf("%w: %q", ErrUnknownState, q.State)
		}
	}

	// 3) Continuation dispatch happens only after the applied+continuation transaction
	// is durable. It is an advisory in-process trigger; the durable continuation record
	// is what recovery reconciles.
	if s.dispatcher != nil {
		if derr := s.dispatcher.DispatchContinuation(ctx, q.ID); derr != nil {
			return q, derr
		}
	}
	return q, nil
}

// ClaimContinuation claims or reclaims a continuation for worker under a lease. It
// increments the claim generation, so any previous worker is fenced.
func (s *Service) ClaimContinuation(id, worker string, lease time.Duration) (*Continuation, bool, error) {
	return s.store.ClaimContinuation(id, worker, s.clock(), lease, DefaultContinuationMaxAttempts)
}

// VerifyClaim reports whether worker still holds the current generation of a
// continuation. It is the fence a worker checks before mutating the run
// stage/checkpoint, so an expired worker cannot mutate state after it lost the lead.
func (s *Service) VerifyClaim(id string, generation int, worker string) (bool, error) {
	return s.store.VerifyClaim(id, generation, worker)
}

// CompleteContinuation records completion under the worker's generation, fenced. It
// reports transitioned=true only on an actual CLAIMED -> COMPLETED transition.
func (s *Service) CompleteContinuation(id string, generation int, worker string) (*Continuation, bool, error) {
	return s.store.CompleteContinuation(id, generation, worker, s.clock())
}

// FenceAndMutate coordinates claim verification with a protected mutation (the
// caller's checkpoint write): mutate runs only while the caller holds the presented
// generation, under the store's cross-process lock. It replaces a separate
// VerifyClaim-then-write, which a reclaim could interleave.
func (s *Service) FenceAndMutate(id string, generation int, worker string, mutate func() error) error {
	return s.store.FenceAndMutate(id, generation, worker, mutate)
}

// Continuations lists continuations in a state ("" lists all).
func (s *Service) Continuations(state ContinuationState) ([]*Continuation, error) {
	return s.store.ListContinuations(state)
}

// Continuation returns the continuation for a question in a run epoch.
func (s *Service) Continuation(questionID, runEpoch string) (*Continuation, error) {
	return s.store.GetContinuation(ContinuationID(questionID, runEpoch))
}

// RejectQuestion records REJECTED on the question with a reason.
func (s *Service) RejectQuestion(questionID, reason string) (*Question, error) {
	q, err := s.store.GetQuestion(questionID)
	if err != nil {
		return nil, err
	}
	if q.State.Terminal() {
		return nil, fmt.Errorf("%w: cannot reject %s question %q", ErrTerminal, q.State, questionID)
	}
	q.State = StateRejected
	q.Reason = strings.TrimSpace(reason)
	if err := s.store.UpdateQuestion(q); err != nil {
		return nil, err
	}
	return q, nil
}

// SupersedeQuestion records SUPERSEDED and links the superseding question id.
func (s *Service) SupersedeQuestion(questionID, byQuestionID, reason string) (*Question, error) {
	q, err := s.store.GetQuestion(questionID)
	if err != nil {
		return nil, err
	}
	if q.State.Terminal() {
		return nil, fmt.Errorf("%w: cannot supersede %s question %q", ErrTerminal, q.State, questionID)
	}
	q.State = StateSuperseded
	q.SupersededBy = strings.TrimSpace(byQuestionID)
	q.Reason = strings.TrimSpace(reason)
	if err := s.store.UpdateQuestion(q); err != nil {
		return nil, err
	}
	return q, nil
}

// BindingCheck validates a question's binding against authoritative current state
// (the live task, run, and plan revision), not merely the caller-supplied identifiers.
// It returns a non-empty reason when the binding is stale — the caller must refuse
// the answer and preserve the prior record — "" when the binding is current, and an
// error only for a hard I/O failure.
type BindingCheck func(ctx context.Context, q *Question) (string, error)

// SubmitInput is one planner answer submission.
type SubmitInput struct {
	QuestionID       string
	Body             string
	AnsweredBy       string
	IdempotencyKey   string
	ExpectedRevision int
	ChosenAction     string
	Rationale        string
}

// SubmitResult reports the deterministic outcome of one submission. It is never an
// approval and never implies that execution began.
type SubmitResult struct {
	Outcome  OutcomeKind
	Question *Question
	Answer   *Answer
	Reason   string
}

// SubmitAnswer records a planner answer as ONE atomic transaction: authoritative
// validation, duplicate/conflict detection, answer insertion, the PENDING -> ANSWERED
// transition, and the audit outcome all run under the store's single transaction, so
// two competing submissions cannot both observe PENDING and exactly one is accepted.
// It records the answer and advances the question and nothing more: it never applies
// the answer, never dispatches continuation, never approves a task, and never resumes
// execution.
//
// The outcome is deterministic: STALE when the binding (checked against authoritative
// state) or the caller's expected revision no longer matches; DUPLICATE when an answer
// with the same idempotency key AND the same payload already exists; REJECTED when the
// key is reused with a different payload, or the question is already answered or
// terminal; ACCEPTED otherwise. Question and answer records are never mutated on a
// non-accepted outcome, but every attempt is durably audited in the append-only ledger
// (which is why every submission reports a durable change).
func (s *Service) SubmitAnswer(ctx context.Context, in SubmitInput, check BindingCheck) (SubmitResult, error) {
	questionID := strings.TrimSpace(in.QuestionID)
	if questionID == "" {
		return SubmitResult{}, fmt.Errorf("%w: missing question_id", ErrInvalidRecord)
	}
	body := strings.TrimSpace(in.Body)

	var res SubmitResult
	err := s.store.Atomic(func(tx Store) (bool, error) {
		if body == "" {
			res = SubmitResult{Outcome: OutcomeRejected, Reason: "empty body"}
			return appendOutcome(tx, questionID, "", OutcomeRejected, "empty body", 0, in)
		}
		q, err := tx.GetQuestion(questionID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				res = SubmitResult{Outcome: OutcomeStale, Reason: "unknown question"}
				return appendOutcome(tx, questionID, "", OutcomeStale, "unknown question", 0, in)
			}
			return false, err
		}
		// 1) Authoritative binding check first: a stale binding refuses the answer even
		// if every caller-supplied identifier looks plausible.
		if check != nil {
			reason, cerr := check(ctx, q)
			if cerr != nil {
				return false, cerr
			}
			if reason != "" {
				res = SubmitResult{Outcome: OutcomeStale, Question: q, Reason: reason}
				return appendOutcome(tx, q.ID, "", OutcomeStale, reason, q.Revision, in)
			}
		}
		// 2) The caller's declared revision must be current.
		if in.ExpectedRevision != 0 && in.ExpectedRevision != q.Revision {
			reason := fmt.Sprintf("expected revision %d is not current %d", in.ExpectedRevision, q.Revision)
			res = SubmitResult{Outcome: OutcomeStale, Question: q, Reason: reason}
			return appendOutcome(tx, q.ID, "", OutcomeStale, reason, q.Revision, in)
		}
		// 3) Idempotency: the same key with the same payload is a duplicate; the same
		// key with a different payload is a rejected conflict, never a silent success.
		if key := strings.TrimSpace(in.IdempotencyKey); key != "" {
			answers, aerr := tx.AnswersFor(q.ID)
			if aerr != nil {
				return false, aerr
			}
			for _, a := range answers {
				if strings.TrimSpace(a.IdempotencyKey) != key {
					continue
				}
				if strings.TrimSpace(a.Body) == body {
					res = SubmitResult{Outcome: OutcomeDuplicate, Question: q, Answer: a, Reason: "duplicate idempotency key"}
					return appendOutcome(tx, q.ID, a.ID, OutcomeDuplicate, "duplicate idempotency key", q.Revision, in)
				}
				reason := "idempotency key reused with a different payload"
				res = SubmitResult{Outcome: OutcomeRejected, Question: q, Answer: a, Reason: reason}
				return appendOutcome(tx, q.ID, a.ID, OutcomeRejected, reason, q.Revision, in)
			}
		}
		// 4) Conflicting submission: a terminal or already-answered question cannot take
		// a new answer (a correction goes through supersede/reject, not re-answering).
		if q.State.Terminal() {
			reason := "question is " + string(q.State)
			res = SubmitResult{Outcome: OutcomeRejected, Question: q, Reason: reason}
			return appendOutcome(tx, q.ID, "", OutcomeRejected, reason, q.Revision, in)
		}
		if q.State != StatePending {
			reason := "question is already " + string(q.State)
			res = SubmitResult{Outcome: OutcomeRejected, Question: q, Reason: reason}
			return appendOutcome(tx, q.ID, "", OutcomeRejected, reason, q.Revision, in)
		}
		// 5) Accept: record the answer, then advance PENDING -> ANSWERED, both inside
		// this transaction. No apply, no continuation dispatch, no approval.
		stored, err := tx.CreateAnswer(&Answer{
			QuestionID:       q.ID,
			Body:             body,
			AnsweredBy:       strings.TrimSpace(in.AnsweredBy),
			IdempotencyKey:   strings.TrimSpace(in.IdempotencyKey),
			ExpectedRevision: in.ExpectedRevision,
			ChosenAction:     strings.TrimSpace(in.ChosenAction),
			Rationale:        strings.TrimSpace(in.Rationale),
		})
		if err != nil {
			return false, err
		}
		updated, _, err := tx.ApplyTransition(q.ID, StatePending, StateAnswered)
		if err != nil {
			return false, err
		}
		if updated == nil {
			updated = q
		}
		res = SubmitResult{Outcome: OutcomeAccepted, Question: updated, Answer: stored}
		return appendOutcome(tx, updated.ID, stored.ID, OutcomeAccepted, "", updated.Revision, in)
	})
	if err != nil {
		return SubmitResult{}, err
	}
	return res, nil
}

// appendOutcome records an audit outcome through tx. It always reports changed=true,
// because the appended outcome is itself durable state: a duplicate/stale/rejected
// submission preserves the question and answer records but still records the attempt.
func appendOutcome(tx Store, questionID, answerID string, kind OutcomeKind, reason string, rev int, in SubmitInput) (bool, error) {
	o := &Outcome{
		QuestionID:     questionID,
		AnswerID:       answerID,
		Kind:           kind,
		Reason:         reason,
		Revision:       rev,
		SubmittedBy:    strings.TrimSpace(in.AnsweredBy),
		IdempotencyKey: strings.TrimSpace(in.IdempotencyKey),
	}
	if err := tx.AppendOutcome(o); err != nil {
		return false, err
	}
	return true, nil
}
