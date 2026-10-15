package question

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/imhttran/agentic-sop/internal/durablefs"
)

// ErrLockUnsupported reports that the platform cannot provide the advisory file
// lock the durable question store requires. Because the cross-process read-modify-
// write guarantee cannot be upheld without it, mutation (including answer
// submission) fails closed with this error rather than risk a lost update. Reads
// remain available.
var ErrLockUnsupported = errors.New("question: durable writes require an advisory file lock, unsupported on this platform")

// FileStore is a durable Store that persists questions, answers, and the audit
// outcomes to a single project-scoped JSON file.
//
// Concurrency: every mutation runs as one read-modify-write transaction serialized
// by an intra-process mutex and a cross-process advisory lock, reloading under the
// lock and committing with an atomic rename after an fsync. A submission therefore
// validates, deduplicates, inserts the answer, advances PENDING -> ANSWERED, and
// appends the audit outcome as a single transaction: two competing submissions
// cannot both observe PENDING, so exactly one is accepted.
type FileStore struct {
	mu   sync.Mutex
	path string
	mem  *MemoryStore
	// lock takes the advisory lock; it is a package default and is overridable in
	// tests to exercise the fail-closed path. It must return ErrLockUnsupported (or
	// another error) when no lock can be taken.
	lock func(*os.File) error
}

// fileState is the on-disk shape.
type fileState struct {
	ContractVersion int             `json:"contract_version"`
	Questions       []*Question     `json:"questions"`
	Answers         []*Answer       `json:"answers"`
	Outcomes        []*Outcome      `json:"outcomes,omitempty"`
	Continuations   []*Continuation `json:"continuations,omitempty"`
}

// OpenFileStore opens (or creates) a durable store at path. A missing file is an
// empty store; a present but malformed file fails closed with ErrInvalidRecord.
func OpenFileStore(path string) (*FileStore, error) {
	f := &FileStore{path: path, lock: lockFile}
	if err := f.reload(); err != nil {
		return nil, err
	}
	return f, nil
}

// Path returns the store file path.
func (f *FileStore) Path() string { return f.path }

// reload rebuilds the in-memory view from disk, fail-closed.
func (f *FileStore) reload() error {
	mem := NewMemoryStore()
	if err := loadInto(mem, f.path); err != nil {
		return err
	}
	mem.mu.Lock()
	mem.restoreNextIDLocked()
	mem.mu.Unlock()
	f.mem = mem
	return nil
}

// beginRead serializes and reloads for a read. It takes no cross-process lock: a
// read either observes a complete prior document or the complete new one (the write
// is an atomic rename), so it never needs to block a writer.
func (f *FileStore) beginRead() (func(), error) {
	f.mu.Lock()
	if err := f.reload(); err != nil {
		f.mu.Unlock()
		return nil, err
	}
	return f.mu.Unlock, nil
}

// beginWrite serializes and reloads for a mutation under the cross-process lock. It
// fails closed (ErrLockUnsupported, or the lock's error) rather than proceed without
// the lock, because the atomicity of a submission depends on it.
func (f *FileStore) beginWrite() (func(), error) {
	f.mu.Lock()
	unlock, err := f.acquireLock()
	if err != nil {
		f.mu.Unlock()
		return nil, err
	}
	if rerr := f.reload(); rerr != nil {
		unlock()
		f.mu.Unlock()
		return nil, rerr
	}
	return func() {
		unlock()
		f.mu.Unlock()
	}, nil
}

// acquireLock takes an exclusive advisory lock on the store's lock file, failing
// closed when it cannot. The lock is what makes a submission's read-modify-write
// atomic across processes.
func (f *FileStore) acquireLock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return func() {}, err
	}
	lf, err := os.OpenFile(f.path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return func() {}, fmt.Errorf("question: open lock file: %w", err)
	}
	lock := f.lock
	if lock == nil {
		lock = lockFile
	}
	if err := lock(lf); err != nil {
		_ = lf.Close()
		return func() {}, err
	}
	return func() {
		_ = unlockFile(lf)
		_ = lf.Close()
	}, nil
}

// persist writes the in-memory state to disk durably (temp file + fsync + rename +
// directory fsync).
func (f *FileStore) persist() error {
	st := fileState{
		ContractVersion: ContractVersion,
		Questions:       f.mem.allQuestions(),
		Answers:         f.mem.allAnswers(),
		Outcomes:        f.mem.allOutcomes(),
		Continuations:   f.mem.allContinuations(),
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(f.path, append(data, '\n'))
}

// Atomic runs fn as one atomic transaction: it holds the intra-process mutex and
// the cross-process lock, reloads the store, runs fn against the in-memory view,
// and persists exactly once when fn reports a change. This is what makes a
// submission's validation, deduplication, insertion, state advance, and audit one
// transaction rather than several separately locked calls.
func (f *FileStore) Atomic(fn func(tx Store) (bool, error)) error {
	done, err := f.beginWrite()
	if err != nil {
		return err
	}
	defer done()
	changed, err := fn(f.mem)
	if err != nil {
		return err
	}
	if changed {
		return f.persist()
	}
	return nil
}

// CreateQuestion implements Store with write-through persistence. It persists only
// when a new record was created.
func (f *FileStore) CreateQuestion(q *Question) (*Question, error) {
	done, err := f.beginWrite()
	if err != nil {
		return nil, err
	}
	defer done()
	out, oerr := f.mem.CreateQuestion(q)
	if oerr == nil {
		if perr := f.persist(); perr != nil {
			return out, perr
		}
	}
	return out, oerr
}

// GetQuestion implements Store.
func (f *FileStore) GetQuestion(id string) (*Question, error) {
	done, err := f.beginRead()
	if err != nil {
		return nil, err
	}
	defer done()
	return f.mem.GetQuestion(id)
}

// ListQuestions implements Store.
func (f *FileStore) ListQuestions(state State) ([]*Question, error) {
	done, err := f.beginRead()
	if err != nil {
		return nil, err
	}
	defer done()
	return f.mem.ListQuestions(state)
}

// CreateAnswer implements Store with write-through persistence.
func (f *FileStore) CreateAnswer(a *Answer) (*Answer, error) {
	done, err := f.beginWrite()
	if err != nil {
		return nil, err
	}
	defer done()
	out, oerr := f.mem.CreateAnswer(a)
	if oerr == nil {
		if perr := f.persist(); perr != nil {
			return out, perr
		}
	}
	return out, oerr
}

// AnswersFor implements Store.
func (f *FileStore) AnswersFor(questionID string) ([]*Answer, error) {
	done, err := f.beginRead()
	if err != nil {
		return nil, err
	}
	defer done()
	return f.mem.AnswersFor(questionID)
}

// UpdateQuestion implements Store with write-through persistence.
func (f *FileStore) UpdateQuestion(q *Question) error {
	done, err := f.beginWrite()
	if err != nil {
		return err
	}
	defer done()
	if err := f.mem.UpdateQuestion(q); err != nil {
		return err
	}
	return f.persist()
}

// ApplyTransition implements Store with write-through persistence.
func (f *FileStore) ApplyTransition(id string, from, to State) (*Question, bool, error) {
	done, err := f.beginWrite()
	if err != nil {
		return nil, false, err
	}
	defer done()
	q, applied, aerr := f.mem.ApplyTransition(id, from, to)
	if aerr != nil {
		return nil, false, aerr
	}
	if applied {
		if perr := f.persist(); perr != nil {
			return q, false, perr
		}
	}
	return q, applied, nil
}

// AppendOutcome implements Store with write-through persistence.
func (f *FileStore) AppendOutcome(o *Outcome) error {
	done, err := f.beginWrite()
	if err != nil {
		return err
	}
	defer done()
	if err := f.mem.AppendOutcome(o); err != nil {
		return err
	}
	return f.persist()
}

// Outcomes implements Store.
func (f *FileStore) Outcomes(questionID string) ([]*Outcome, error) {
	done, err := f.beginRead()
	if err != nil {
		return nil, err
	}
	defer done()
	return f.mem.Outcomes(questionID)
}

// CreateContinuation implements Store with write-through persistence.
func (f *FileStore) CreateContinuation(c *Continuation) (*Continuation, error) {
	done, err := f.beginWrite()
	if err != nil {
		return nil, err
	}
	defer done()
	out, oerr := f.mem.CreateContinuation(c)
	if oerr == nil {
		if perr := f.persist(); perr != nil {
			return out, perr
		}
	}
	return out, oerr
}

// GetContinuation implements Store.
func (f *FileStore) GetContinuation(id string) (*Continuation, error) {
	done, err := f.beginRead()
	if err != nil {
		return nil, err
	}
	defer done()
	return f.mem.GetContinuation(id)
}

// ListContinuations implements Store.
func (f *FileStore) ListContinuations(state ContinuationState) ([]*Continuation, error) {
	done, err := f.beginRead()
	if err != nil {
		return nil, err
	}
	defer done()
	return f.mem.ListContinuations(state)
}

// ClaimContinuation implements Store with write-through persistence.
func (f *FileStore) ClaimContinuation(id, worker string, now time.Time, lease time.Duration, maxAttempts int) (*Continuation, bool, error) {
	done, err := f.beginWrite()
	if err != nil {
		return nil, false, err
	}
	defer done()
	out, claimed, cerr := f.mem.ClaimContinuation(id, worker, now, lease, maxAttempts)
	if cerr != nil {
		return out, false, cerr
	}
	if claimed {
		if perr := f.persist(); perr != nil {
			return out, false, perr
		}
	}
	return out, claimed, nil
}

// VerifyClaim implements Store.
func (f *FileStore) VerifyClaim(id string, generation int, worker string) (bool, error) {
	done, err := f.beginRead()
	if err != nil {
		return false, err
	}
	defer done()
	return f.mem.VerifyClaim(id, generation, worker)
}

// CompleteContinuation implements Store with write-through persistence.
func (f *FileStore) CompleteContinuation(id string, generation int, worker string, now time.Time) (*Continuation, bool, error) {
	done, err := f.beginWrite()
	if err != nil {
		return nil, false, err
	}
	defer done()
	out, transitioned, cerr := f.mem.CompleteContinuation(id, generation, worker, now)
	if cerr != nil {
		return out, false, cerr
	}
	if transitioned {
		if perr := f.persist(); perr != nil {
			return out, false, perr
		}
	}
	return out, transitioned, nil
}

// FenceAndMutate implements Store. It verifies the claim and runs mutate() while
// holding the cross-process lock, so a reclaimed worker cannot write afterward.
func (f *FileStore) FenceAndMutate(id string, generation int, worker string, mutate func() error) error {
	done, err := f.beginWrite()
	if err != nil {
		return err
	}
	defer done()
	held, verr := f.mem.VerifyClaim(id, generation, worker)
	if verr != nil {
		return verr
	}
	if !held {
		return fmt.Errorf("%w: %q", ErrClaimFenced, id)
	}
	return mutate()
}

// loadInto reads the on-disk state into m, fail-closed.
func loadInto(m *MemoryStore, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("question: read store: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	var st fileState
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("%w: malformed store: %v", ErrInvalidRecord, err)
	}
	if st.ContractVersion != 0 && st.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: unsupported store contract version %d", ErrInvalidRecord, st.ContractVersion)
	}
	for _, q := range st.Questions {
		q.ContractVersion = ContractVersion
		if err := m.insertLoadedQuestion(q); err != nil {
			return err
		}
	}
	for _, a := range st.Answers {
		a.ContractVersion = ContractVersion
		if err := m.insertLoadedAnswer(a); err != nil {
			return err
		}
	}
	for _, o := range st.Outcomes {
		o.ContractVersion = ContractVersion
		if err := m.insertLoadedOutcome(o); err != nil {
			return err
		}
	}
	for _, c := range st.Continuations {
		c.ContractVersion = ContractVersion
		if err := m.insertLoadedContinuation(c); err != nil {
			return err
		}
	}
	return nil
}

// atomicWriteFile writes data to path atomically: a temp file in the same directory
// is written, fsynced, renamed over the destination, and the directory is fsynced, so
// a crash cannot leave a truncated document and the most recent write is durable. It
// delegates to the shared durable-write primitive.
func atomicWriteFile(path string, data []byte) error {
	return durablefs.WriteFile(path, data)
}

// --- MemoryStore helpers used by FileStore (load/export) ---

// insertLoadedQuestion restores a persisted question without re-stamping times.
func (m *MemoryStore) insertLoadedQuestion(q *Question) error {
	if q == nil {
		return fmt.Errorf("%w: nil question", ErrInvalidRecord)
	}
	if err := q.Validate(); err != nil {
		return err
	}
	key := q.Key()
	if key == "" {
		return fmt.Errorf("%w: missing identity", ErrInvalidRecord)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.questions[q.ID] = q
	m.byKey[key] = q.ID
	m.order = append(m.order, q.ID)
	return nil
}

// insertLoadedAnswer restores a persisted answer.
func (m *MemoryStore) insertLoadedAnswer(a *Answer) error {
	if a == nil {
		return fmt.Errorf("%w: nil answer", ErrInvalidRecord)
	}
	if err := a.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answers[a.ID] = a
	m.ansOrder = append(m.ansOrder, a.ID)
	return nil
}

// insertLoadedOutcome restores a persisted audit outcome.
func (m *MemoryStore) insertLoadedOutcome(o *Outcome) error {
	if o == nil {
		return fmt.Errorf("%w: nil outcome", ErrInvalidRecord)
	}
	if err := o.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outcomes[o.ID] = o
	m.outOrder = append(m.outOrder, o.ID)
	return nil
}

// allQuestions returns every question in a stable order for persistence.
func (m *MemoryStore) allQuestions() []*Question {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := append([]string(nil), m.order...)
	sort.Strings(ids)
	out := make([]*Question, 0, len(ids))
	for _, id := range ids {
		if q := m.questions[id]; q != nil {
			cp := *q
			out = append(out, &cp)
		}
	}
	return out
}

// allAnswers returns every answer in a stable order for persistence.
func (m *MemoryStore) allAnswers() []*Answer {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := append([]string(nil), m.ansOrder...)
	sort.Strings(ids)
	out := make([]*Answer, 0, len(ids))
	for _, id := range ids {
		if a := m.answers[id]; a != nil {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out
}

// allOutcomes returns every audit outcome in a stable order for persistence.
func (m *MemoryStore) allOutcomes() []*Outcome {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := append([]string(nil), m.outOrder...)
	sort.Strings(ids)
	out := make([]*Outcome, 0, len(ids))
	for _, id := range ids {
		if o := m.outcomes[id]; o != nil {
			cp := *o
			out = append(out, &cp)
		}
	}
	return out
}

// insertLoadedContinuation restores a persisted continuation.
func (m *MemoryStore) insertLoadedContinuation(c *Continuation) error {
	if c == nil {
		return fmt.Errorf("%w: nil continuation", ErrInvalidRecord)
	}
	if err := c.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conts[c.ID] = c
	m.contOrder = append(m.contOrder, c.ID)
	return nil
}

// allContinuations returns every continuation in a stable order for persistence.
func (m *MemoryStore) allContinuations() []*Continuation {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := append([]string(nil), m.contOrder...)
	sort.Strings(ids)
	out := make([]*Continuation, 0, len(ids))
	for _, id := range ids {
		if c := m.conts[id]; c != nil {
			cp := *c
			out = append(out, &cp)
		}
	}
	return out
}
