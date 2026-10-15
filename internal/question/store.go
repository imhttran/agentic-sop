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
