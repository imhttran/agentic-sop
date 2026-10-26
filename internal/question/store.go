package question

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store is the minimal durable persistence contract for questions and answers.
// It reuses the existing SOP persistence pattern: a plain interface backed by an
// in-memory implementation, with answers stored distinctly from the questions
// they answer.
type Store interface {
	// CreateQuestion records a PENDING question. A duplicate identity key
	// (Identity+Kind) returns (existing, ErrDuplicate) without creating a second
	// record. A stale revision returns (existing, ErrStale).
	CreateQuestion(q *Question) (*Question, error)
	// GetQuestion reads a question by identity.
	GetQuestion(id string) (*Question, error)
	// ListQuestions lists questions filtered by state. An unrecognized state
	// fails closed with ErrUnknownState.
	ListQuestions(state State) ([]*Question, error)
	// CreateAnswer records an answer as a record distinct from the question.
	CreateAnswer(a *Answer) (*Answer, error)
	// AnswersFor returns the answers recorded for a question, by question id.
	AnswersFor(questionID string) ([]*Answer, error)
	// UpdateQuestion persists a state transition.
	UpdateQuestion(q *Question) error
	// ApplyTransition performs an atomic compare-and-set of the question state:
	// it transitions id from `from` to `to` only if the current stored state is
	// exactly `from`. It returns (record, true, nil) when the transition was
	// applied, (record, false, nil) when the current state differs from `from`
	// (already terminal/applied, or otherwise not in the expected state), and an
	// error only for a missing record or a structurally invalid `to`. The
	// compare-and-set is the seam that makes a concurrent double-apply
	// impossible: at most one caller observes applied=true.
	ApplyTransition(id string, from, to State) (*Question, bool, error)
	// AppendOutcome records an append-only audit outcome for one answer
	// submission. It never mutates a question, answer, or task.
	AppendOutcome(o *Outcome) error
	// Outcomes returns the audit outcomes recorded for a question, by question id,
	// in insertion order.
	Outcomes(questionID string) ([]*Outcome, error)
	// Atomic runs fn as a single read-modify-write transaction. fn receives a Store
	// view valid only for the call and reports whether durable state changed. A real
	// implementation (FileStore) holds its cross-process lock across the whole call,
	// so a submission's validation, deduplication, insertion, state advance, and audit
	// are one transaction rather than several separately locked calls.
	Atomic(fn func(tx Store) (bool, error)) error
	// CreateContinuation records a continuation, idempotent on its stable ID: an
	// existing ID returns the stored record unchanged.
	CreateContinuation(c *Continuation) (*Continuation, error)
	// GetContinuation reads a continuation by ID.
	GetContinuation(id string) (*Continuation, error)
	// ListContinuations lists continuations in a state ("" lists all), in order.
	ListContinuations(state ContinuationState) ([]*Continuation, error)
	// ClaimContinuation claims, or reclaims after lease expiry, a continuation for a
	// worker. On success it increments the generation and the attempt count. It
	// returns the record and whether it was claimed; a completed continuation or an
	// exhausted attempt budget yields claimed=false.
	ClaimContinuation(id, worker string, now time.Time, lease time.Duration, maxAttempts int) (*Continuation, bool, error)
	// VerifyClaim reports whether worker still holds the current generation of id.
	// It is the fence a worker checks before mutating the run stage/checkpoint.
	VerifyClaim(id string, generation int, worker string) (bool, error)
	// CompleteContinuation records completion under the worker's generation. It is
	// fenced: a stale generation, a different worker, or a non-CLAIMED state yields
	// completed=false without mutating the record.
	CompleteContinuation(id string, generation int, worker string, now time.Time) (*Continuation, bool, error)
	// SupersedeContinuation records the terminal, non-completion outcome for a
	// continuation whose owed work yielded to a later planner question in the same
	// task/run. It is fenced exactly like completion (CLAIMED -> SUPERSEDED under the
	// worker's generation). A superseded continuation is terminal, is never claimable, and
	// is never COMPLETED.
	SupersedeContinuation(id string, generation int, worker string, now time.Time) (*Continuation, bool, error)
	// ResolveYieldedContinuation is the RECOVERY counterpart of SupersedeContinuation. It
	// records the same terminal, non-completion outcome for a predecessor continuation
	// whose owed work demonstrably yielded to a LATER, durably persisted question for the
	// same task/run/plan (the caller has verified that successor exists) but whose
	// supersession was never recorded because the process stopped.
	//
	// INVARIANT (why no claim fence is required): a successor question is only ever raised by
	// the predecessor's OWN resumed stage, and the raise happens after that stage's work has
	// returned. A persisted successor therefore proves the predecessor is no longer running:
	// no worker can be executing the predecessor's stage work while this is called. The
	// caller must enforce the invariant by requiring a distinct, live successor bound to the
	// same task, run epoch, and plan revision (see internal/cli plannerResolvePredecessors).
	//
	// It transitions ONLY from CLAIMED, so it can never complete, resurrect, or replay work,
	// and it does not weaken the completion/checkpoint fence: the claim-generation fence on
	// CompleteContinuation/FenceAndMutate is unchanged, and a superseded continuation can
	// never afterwards be claimed, completed, or fence-mutated.
	ResolveYieldedContinuation(id string, now time.Time) (*Continuation, bool, error)
	// DischargeExternalContinuation records the terminal, NON-execution outcome for a
	// continuation whose task an external completion proved already merged. The owed
	// stage work is discharged without being run, so the record becomes
	// EXTERNALLY_COMPLETED — never COMPLETED, which would claim the stage executed.
	//
	// It transitions only from a non-terminal state (PENDING, or CLAIMED held by a worker
	// that will never resume), so it can never complete, resurrect, or replay work, and a
	// discharged continuation can never afterwards be claimed, completed, or
	// fence-mutated. The caller owns the binding checks: it must establish the same task,
	// the live plan revision, the live run epoch, and a contained implementation commit.
	DischargeExternalContinuation(id, reason string, now time.Time) (*Continuation, bool, error)
	// FenceAndMutate coordinates claim verification with a protected mutation: it runs
	// mutate() only while the caller still holds the presented generation, under the
	// store's cross-process lock, so a reclaimed worker cannot write afterward. The
	// mutation is the caller's checkpoint write; it is not one transaction with the
	// store's own commit across separate files, but no competing claim or reclaim can
	// interleave between the verification and the mutation.
	FenceAndMutate(id string, generation int, worker string, mutate func() error) error
}

// MemoryStore is the in-memory Store implementation used by the application
// service and tests.
type MemoryStore struct {
	mu        sync.Mutex
	questions map[string]*Question
	order     []string
	byKey     map[string]string
	answers   map[string]*Answer
	ansOrder  []string
	outcomes  map[string]*Outcome
	outOrder  []string
	conts     map[string]*Continuation
	contOrder []string
	nextID    int
	now       func() time.Time
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		questions: map[string]*Question{},
		byKey:     map[string]string{},
		answers:   map[string]*Answer{},
		outcomes:  map[string]*Outcome{},
		conts:     map[string]*Continuation{},
		now:       time.Now,
	}
}

// stamp returns the store clock in UTC.
func (m *MemoryStore) stamp() time.Time {
	if m.now == nil {
		return time.Now().UTC()
	}
	return m.now().UTC()
}

// CreateQuestion implements Store. It is deterministic: the duplicate rule is
// keyed on Identity+Kind, and the stale rule compares the submitted revision to
// the current record's revision.
//
// The create path never replaces an existing record. Once a record exists for an
// identity key, a re-submission carrying that same key is never advanced through
// CreateQuestion; the stored record is returned unchanged. The outcome is a pure
// function of the submitted revision relative to the stored revision:
//
//   - an equal revision is a benign duplicate, reported as ErrDuplicate;
//   - a different revision (higher or lower) is a stale submission, reported as
//     ErrStale.
//
// Both outcomes return the current, unmodified record with no error other than
// the sentinel, so the result is independent of arrival order and timing.
// Supersession of an existing question is the SUPERSEDED state transition, not a
// side effect of creation.
func (m *MemoryStore) CreateQuestion(q *Question) (*Question, error) {
	if q == nil {
		return nil, fmt.Errorf("%w: nil question", ErrInvalidRecord)
	}
	if strings.TrimSpace(q.Identity) == "" {
		return nil, fmt.Errorf("%w: missing identity", ErrInvalidRecord)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := q.Key()
	if id, ok := m.byKey[key]; ok {
		// The create path never replaces an existing record. An equal revision
		// is a benign duplicate; any other revision (higher or lower) is a stale
		// submission. Both leave the current record unchanged and return it, so
		// the outcome depends only on the submitted vs. stored revision and is
		// independent of arrival order.
		cur := m.questions[id]
		cp := *cur
		if q.Revision == cur.Revision {
			return &cp, ErrDuplicate
		}
		return &cp, fmt.Errorf("%w: revision %d != current %d", ErrStale, q.Revision, cur.Revision)
	}

	stored := *q
	stored.ContractVersion = ContractVersion
	if stored.State == "" {
		stored.State = StatePending
	}
	if !stored.State.Known() {
		return nil, fmt.Errorf("%w: %q", ErrUnknownState, stored.State)
	}
	if stored.State != StatePending {
		return nil, fmt.Errorf("%w: create must record PENDING, got %q", ErrInvalidTransition, stored.State)
	}
	if stored.ID == "" {
		m.nextID++
		stored.ID = fmt.Sprintf("q-%d", m.nextID)
	}
	if err := stored.Validate(); err != nil {
		return nil, err
	}
	stored.CreatedAt = m.stamp()
	stored.UpdatedAt = stored.CreatedAt
	// Derive the deadline from the record's OWN creation time, so created_at and deadline
	// share one clock reading (and are deterministic under the injectable store clock).
	// DeadlineAfter is an input only; the durable Deadline is what persists. A duplicate
	// creation never reaches here: it returns the existing record, preserving its deadline.
	if stored.Deadline.IsZero() && stored.DeadlineAfter > 0 {
		stored.Deadline = stored.CreatedAt.Add(stored.DeadlineAfter)
	}
	stored.DeadlineAfter = 0
	m.questions[stored.ID] = &stored
	m.byKey[key] = stored.ID
	m.order = append(m.order, stored.ID)
	return &stored, nil
}

// GetQuestion implements Store.
func (m *MemoryStore) GetQuestion(id string) (*Question, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q, ok := m.questions[id]
	if !ok {
		return nil, fmt.Errorf("%w: question %q", ErrNotFound, id)
	}
	out := *q
	return &out, nil
}

// ListQuestions implements Store. An unrecognized state fails closed.
func (m *MemoryStore) ListQuestions(state State) ([]*Question, error) {
	if !state.Known() {
		return nil, fmt.Errorf("%w: %q", ErrUnknownState, state)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Question
	ids := append([]string(nil), m.order...)
	sort.Strings(ids)
	for _, id := range ids {
		q := m.questions[id]
		if q != nil && q.State == state {
			cp := *q
			out = append(out, &cp)
		}
	}
	return out, nil
}

// CreateAnswer implements Store. The answer is a record distinct from the
// question and has its own identity.
func (m *MemoryStore) CreateAnswer(a *Answer) (*Answer, error) {
	if a == nil {
		return nil, fmt.Errorf("%w: nil answer", ErrInvalidRecord)
	}
	if strings.TrimSpace(a.QuestionID) == "" {
		return nil, fmt.Errorf("%w: missing question_id", ErrInvalidRecord)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.questions[a.QuestionID]; !ok {
		return nil, fmt.Errorf("%w: question %q", ErrNotFound, a.QuestionID)
	}
	stored := *a
	stored.ContractVersion = ContractVersion
	if stored.ID == "" {
		m.nextID++
		stored.ID = fmt.Sprintf("a-%d", m.nextID)
	}
	if err := stored.Validate(); err != nil {
		return nil, err
	}
	stored.CreatedAt = m.stamp()
	m.answers[stored.ID] = &stored
	m.ansOrder = append(m.ansOrder, stored.ID)
	return &stored, nil
}

// AnswersFor implements Store.
func (m *MemoryStore) AnswersFor(questionID string) ([]*Answer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Answer
	ids := append([]string(nil), m.ansOrder...)
	sort.Strings(ids)
	for _, id := range ids {
		a := m.answers[id]
		if a != nil && a.QuestionID == questionID {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

// UpdateQuestion implements Store. It re-validates the transition and returns
// ErrInvalidTransition/ErrUnknownState on a malformed or unrecognized target
// state, and it rejects an attempt to change the record's identity (ID/Identity/
// Kind), so a caller cannot silently fork or rename a record through an update.
func (m *MemoryStore) UpdateQuestion(q *Question) error {
	if err := q.Validate(); err != nil {
		return err
	}
	if !q.State.Known() {
		return fmt.Errorf("%w: %q", ErrUnknownState, q.State)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.questions[q.ID]
	if !ok {
		return fmt.Errorf("%w: question %q", ErrNotFound, q.ID)
	}
	if strings.TrimSpace(q.Identity) != strings.TrimSpace(cur.Identity) || strings.TrimSpace(q.Kind) != strings.TrimSpace(cur.Kind) {
		return fmt.Errorf("%w: update must not change identity of question %q", ErrInvalidTransition, q.ID)
	}
	if !validTransition(cur.State, q.State) {
		return fmt.Errorf("%w: %s -> %s for question %q", ErrInvalidTransition, cur.State, q.State, q.ID)
	}
	stored := *q
	stored.UpdatedAt = m.stamp()
	cur.State = stored.State
	cur.Reason = stored.Reason
	cur.SupersededBy = stored.SupersededBy
	cur.UpdatedAt = stored.UpdatedAt
	return nil
}

// validTransition is the store-side guard for the question lifecycle. It mirrors
// the service rules and fails closed: an unrecognized from/to pair is rejected.
// A no-op transition (same state) is permitted so an idempotent reader is not
// forced to mutate, but a transition out of a terminal state to a different
// state is not permitted.
func validTransition(from, to State) bool {
	if !from.Known() || !to.Known() {
		return false
	}
	if from == to {
		return true
	}
	if from.Terminal() {
		return false
	}
	switch to {
	case StateAnswered, StateApplied, StateRejected, StateSuperseded:
		return true
	default:
		return false
	}
}

// ApplyTransition implements Store. The whole read-check-write happens under the
// store mutex, so two concurrent callers cannot both observe `from` and both
// apply `to`: exactly one observes applied=true. A caller that observes
// applied=false must not dispatch continuation.
func (m *MemoryStore) ApplyTransition(id string, from, to State) (*Question, bool, error) {
	if !from.Known() {
		return nil, false, fmt.Errorf("%w: %q", ErrUnknownState, from)
	}
	if !to.Known() {
		return nil, false, fmt.Errorf("%w: %q", ErrUnknownState, to)
	}
	if !validTransition(from, to) {
		return nil, false, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.questions[id]
	if !ok {
		return nil, false, fmt.Errorf("%w: question %q", ErrNotFound, id)
	}
	cp := *cur
	if cur.State != from {
		// Another caller already advanced the record (or it was never in the
		// expected state). Report it deterministically without mutating.
		return &cp, false, nil
	}
	cur.State = to
	cur.UpdatedAt = m.stamp()
	cp = *cur
	return &cp, true, nil
}

// AppendOutcome implements Store. The outcome is append-only and never mutates a
// question, answer, or task.
func (m *MemoryStore) AppendOutcome(o *Outcome) error {
	if o == nil {
		return fmt.Errorf("%w: nil outcome", ErrInvalidRecord)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := *o
	stored.ContractVersion = ContractVersion
	if stored.ID == "" {
		m.nextID++
		stored.ID = fmt.Sprintf("o-%d", m.nextID)
	}
	if err := stored.Validate(); err != nil {
		return err
	}
	stored.At = m.stamp()
	m.outcomes[stored.ID] = &stored
	m.outOrder = append(m.outOrder, stored.ID)
	return nil
}

// Outcomes implements Store.
func (m *MemoryStore) Outcomes(questionID string) ([]*Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Outcome
	for _, id := range m.outOrder {
		o := m.outcomes[id]
		if o != nil && o.QuestionID == questionID {
			cp := *o
			out = append(out, &cp)
		}
	}
	return out, nil
}

// Atomic implements Store. The in-memory store is used single-threaded by the
// application service and tests; it runs fn directly. The cross-process atomicity
// that a submission depends on is provided by FileStore, which the CLI and MCP use.
func (m *MemoryStore) Atomic(fn func(tx Store) (bool, error)) error {
	_, err := fn(m)
	return err
}

// CreateContinuation implements Store. Idempotent on the stable ID.
func (m *MemoryStore) CreateContinuation(c *Continuation) (*Continuation, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: nil continuation", ErrInvalidRecord)
	}
	if strings.TrimSpace(c.ID) == "" {
		return nil, fmt.Errorf("%w: missing id", ErrInvalidRecord)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.conts[c.ID]; ok {
		cp := *cur
		return &cp, nil
	}
	stored := *c
	stored.ContractVersion = ContractVersion
	if stored.State == "" {
		stored.State = ContinuationPending
	}
	if err := stored.Validate(); err != nil {
		return nil, err
	}
	stored.CreatedAt = m.stamp()
	stored.UpdatedAt = stored.CreatedAt
	m.conts[stored.ID] = &stored
	m.contOrder = append(m.contOrder, stored.ID)
	cp := stored
	return &cp, nil
}

// GetContinuation implements Store.
func (m *MemoryStore) GetContinuation(id string) (*Continuation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conts[id]
	if !ok {
		return nil, fmt.Errorf("%w: continuation %q", ErrNotFound, id)
	}
	cp := *c
	return &cp, nil
}

// ListContinuations implements Store.
func (m *MemoryStore) ListContinuations(state ContinuationState) ([]*Continuation, error) {
	if state != "" && !state.Known() {
		return nil, fmt.Errorf("%w: %q", ErrUnknownState, state)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := append([]string(nil), m.contOrder...)
	sort.Strings(ids)
	var out []*Continuation
	for _, id := range ids {
		c := m.conts[id]
		if c == nil || (state != "" && c.State != state) {
			continue
		}
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

// ClaimContinuation implements Store. It claims a PENDING continuation or reclaims a
// CLAIMED one whose lease has expired, incrementing the generation so any previous
// worker is fenced.
func (m *MemoryStore) ClaimContinuation(id, worker string, now time.Time, lease time.Duration, maxAttempts int) (*Continuation, bool, error) {
	if strings.TrimSpace(worker) == "" {
		return nil, false, fmt.Errorf("%w: empty worker", ErrInvalidRecord)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conts[id]
	if !ok {
		return nil, false, fmt.Errorf("%w: continuation %q", ErrNotFound, id)
	}
	cp := *c
	switch {
	case c.State.Terminal():
		return &cp, false, nil
	case c.State == ContinuationPending:
	case c.State == ContinuationClaimed && leaseExpired(c, now, lease):
	default:
		return &cp, false, nil // actively claimed by another worker
	}
	if maxAttempts > 0 && c.Attempts >= maxAttempts {
		return &cp, false, fmt.Errorf("%w: %q", ErrClaimBudgetExhausted, id)
	}
	c.State = ContinuationClaimed
	c.Generation++
	c.ClaimedBy = strings.TrimSpace(worker)
	c.ClaimedAt = now.UTC()
	c.Attempts++
	c.UpdatedAt = now.UTC()
	cp = *c
	return &cp, true, nil
}

// VerifyClaim implements Store.
func (m *MemoryStore) VerifyClaim(id string, generation int, worker string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conts[id]
	if !ok {
		return false, fmt.Errorf("%w: continuation %q", ErrNotFound, id)
	}
	return c.State == ContinuationClaimed && c.Generation == generation && c.ClaimedBy == strings.TrimSpace(worker), nil
}

// CompleteContinuation implements Store. It reports transitioned=true only when it
// moved CLAIMED -> COMPLETED for the presented generation; an already-completed or
// fenced call returns transitioned=false with the current record.
func (m *MemoryStore) CompleteContinuation(id string, generation int, worker string, now time.Time) (*Continuation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conts[id]
	if !ok {
		return nil, false, fmt.Errorf("%w: continuation %q", ErrNotFound, id)
	}
	cp := *c
	if c.State == ContinuationCompleted {
		return &cp, false, nil
	}
	if c.State != ContinuationClaimed || c.Generation != generation || c.ClaimedBy != strings.TrimSpace(worker) {
		return &cp, false, nil // fenced
	}
	c.State = ContinuationCompleted
	c.CompletedAt = now.UTC()
	c.UpdatedAt = now.UTC()
	cp = *c
	return &cp, true, nil
}

// SupersedeContinuation implements Store. It records the terminal, non-completion
// outcome for a continuation whose owed work yielded to a later planner question: it
// moves CLAIMED -> SUPERSEDED for the presented generation, fenced exactly like
// completion. A superseded continuation is terminal, is never claimable, and is never
// COMPLETED, so unfinished work is not marked complete merely because it parked.
func (m *MemoryStore) SupersedeContinuation(id string, generation int, worker string, now time.Time) (*Continuation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conts[id]
	if !ok {
		return nil, false, fmt.Errorf("%w: continuation %q", ErrNotFound, id)
	}
	cp := *c
	if c.State.Terminal() {
		return &cp, false, nil // already terminal
	}
	if c.State != ContinuationClaimed || c.Generation != generation || c.ClaimedBy != strings.TrimSpace(worker) {
		return &cp, false, nil // fenced
	}
	c.State = ContinuationSuperseded
	c.Reason = "superseded by a later planner question in the same task/run"
	c.UpdatedAt = now.UTC()
	cp = *c
	return &cp, true, nil
}

// ResolveYieldedContinuation implements Store. It is the recovery counterpart of
// SupersedeContinuation: it terminalizes a predecessor continuation (CLAIMED ->
// SUPERSEDED) whose supersession was never recorded because the process stopped after the
// successor question was persisted. It transitions ONLY from CLAIMED, so it can never
// complete, resurrect, or replay work; a superseded continuation can never afterwards be
// claimed, completed, or fence-mutated.
func (m *MemoryStore) ResolveYieldedContinuation(id string, now time.Time) (*Continuation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conts[id]
	if !ok {
		return nil, false, fmt.Errorf("%w: continuation %q", ErrNotFound, id)
	}
	cp := *c
	if c.State != ContinuationClaimed {
		return &cp, false, nil // only a held continuation can yield
	}
	c.State = ContinuationSuperseded
	c.Reason = "superseded by a later planner question (recovered after restart)"
	c.UpdatedAt = now.UTC()
	cp = *c
	return &cp, true, nil
}

// DischargeExternalContinuation implements Store. It terminalizes a continuation whose
// task an external completion proved already merged, recording EXTERNALLY_COMPLETED
// (never COMPLETED) and preserving the question, answer, and outcome history untouched.
func (m *MemoryStore) DischargeExternalContinuation(id, reason string, now time.Time) (*Continuation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conts[id]
	if !ok {
		return nil, false, fmt.Errorf("%w: continuation %q", ErrNotFound, id)
	}
	cp := *c
	if c.State.Terminal() {
		return &cp, false, nil // terminal: never re-discharged or replayed
	}
	c.State = ContinuationExternallyCompleted
	c.Reason = strings.TrimSpace(reason)
	c.UpdatedAt = now.UTC()
	cp = *c
	return &cp, true, nil
}

// FenceAndMutate implements Store. It verifies the claim and runs mutate() while
// holding the store lock, so no competing claim/reclaim can interleave.
func (m *MemoryStore) FenceAndMutate(id string, generation int, worker string, mutate func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conts[id]
	if !ok {
		return fmt.Errorf("%w: continuation %q", ErrNotFound, id)
	}
	if c.State != ContinuationClaimed || c.Generation != generation || c.ClaimedBy != strings.TrimSpace(worker) {
		return fmt.Errorf("%w: %q", ErrClaimFenced, id)
	}
	return mutate()
}

// reserveNextIDLocked returns the next monotonic record ordinal. Callers hold the
// mutex.
func (m *MemoryStore) reserveNextIDLocked() int {
	m.nextID++
	return m.nextID
}

// restoreNextIDLocked raises nextID so it is strictly greater than every numeric
// suffix already present. It is how a reloaded store avoids reissuing an id that
// already exists on disk (the collision the review flagged). Callers hold the
// mutex.
func (m *MemoryStore) restoreNextIDLocked() {
	max := m.nextID
	bump := func(id string) {
		i := strings.LastIndexByte(id, '-')
		if i < 0 || i == len(id)-1 {
			return
		}
		n, err := strconv.Atoi(id[i+1:])
		if err == nil && n > max {
			max = n
		}
	}
	for id := range m.questions {
		bump(id)
	}
	for id := range m.answers {
		bump(id)
	}
	for id := range m.outcomes {
		bump(id)
	}
	m.nextID = max
}
