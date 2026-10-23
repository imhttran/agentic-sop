// Package taskresult stores results an executor publishes to SOP outside the
// governed run loop. A publication is durable evidence, never a lifecycle change:
// recording one never marks a task complete, never approves, and never triggers
// execution. It exists so an external executor (for example a Zed coding agent)
// can report progress, a blocker, test evidence, or completion evidence that a
// planner can then read through the planner surface.
package taskresult

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// ContractVersion identifies the publication record shape.
const ContractVersion = 1

// resultsDirName is the per-run directory holding published results.
const resultsDirName = "results"

// Kind is the closed vocabulary of published result kinds.
type Kind string

const (
	KindProgress   Kind = "PROGRESS"
	KindBlocker    Kind = "BLOCKER"
	KindTest       Kind = "TEST"
	KindCompletion Kind = "COMPLETION"
)

// Known reports whether k is one of the closed kinds.
func (k Kind) Known() bool {
	switch k {
	case KindProgress, KindBlocker, KindTest, KindCompletion:
		return true
	default:
		return false
	}
}

// AllKinds returns the closed kind vocabulary in a stable order.
func AllKinds() []Kind {
	return []Kind{KindProgress, KindBlocker, KindTest, KindCompletion}
}

// ParseKind normalizes a caller-supplied kind (case-insensitive).
func ParseKind(s string) (Kind, bool) {
	k := Kind(strings.ToUpper(strings.TrimSpace(s)))
	return k, k.Known()
}

// Outcome is the closed vocabulary of publication outcomes. ACCEPTED and
// DUPLICATE both describe durably stored evidence; neither is a completion or an
// approval.
type Outcome string

const (
	// OutcomeAccepted: the publication was durably recorded.
	OutcomeAccepted Outcome = "ACCEPTED"
	// OutcomeDuplicate: an identical publication already exists; nothing was written.
	OutcomeDuplicate Outcome = "DUPLICATE"
	// OutcomeStale: the caller's expected plan revision or attempt is not current.
	OutcomeStale Outcome = "STALE"
	// OutcomeRejected: the publication is malformed or conflicts with an existing record.
	OutcomeRejected Outcome = "REJECTED"
)

// Fail-closed sentinels.
var (
	ErrInvalid = errors.New("taskresult: invalid publication")
	ErrStale   = errors.New("taskresult: stale publication")
)

// Publication is one durable executor-published result, bound to the task, the
// attempt, and the plan revision it was published against.
type Publication struct {
	ContractVersion int       `json:"contract_version"`
	ID              string    `json:"id"`
	TaskID          string    `json:"task_id"`
	Attempt         int       `json:"attempt"`
	PlanRevision    string    `json:"plan_revision,omitempty"`
	RunEpoch        string    `json:"run_epoch,omitempty"`
	Kind            Kind      `json:"kind"`
	Summary         string    `json:"summary"`
	Evidence        []string  `json:"evidence,omitempty"`
	PublishedBy     string    `json:"published_by,omitempty"`
	IdempotencyKey  string    `json:"idempotency_key,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// PublishInput is one publication request. Attempt and PlanRevision are the live
// values the caller is publishing against; ExpectedAttempt and
// ExpectedPlanRevision are optional assertions that refuse the publication when
// the caller's view is no longer current.
type PublishInput struct {
	TaskID               string
	Attempt              int
	PlanRevision         string
	RunEpoch             string
	Kind                 Kind
	Summary              string
	Evidence             []string
	PublishedBy          string
	IdempotencyKey       string
	ExpectedAttempt      int
	ExpectedPlanRevision string
	Now                  time.Time
}

// PublishResult reports the deterministic outcome of one publication.
type PublishResult struct {
	Outcome     Outcome
	Reason      string
	Publication *Publication
}

// Publish records one publication atomically. It refuses a publication whose
// declared plan revision or attempt is no longer current, and it is idempotent:
// the same identity (task, attempt, and idempotency key, or the same content when
// no key is supplied) never creates a second record.
func Publish(projectDir string, in PublishInput) (PublishResult, error) {
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		return PublishResult{}, fmt.Errorf("%w: missing task id", ErrInvalid)
	}
	if taskID != filepath.Base(taskID) || taskID == "." || taskID == ".." {
		return PublishResult{}, fmt.Errorf("%w: task id %q must be a single path element", ErrInvalid, taskID)
	}
	if !in.Kind.Known() {
		return PublishResult{Outcome: OutcomeRejected, Reason: fmt.Sprintf("unknown kind %q", in.Kind)}, nil
	}
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		return PublishResult{Outcome: OutcomeRejected, Reason: "empty summary"}, nil
	}
	if expected := strings.TrimSpace(in.ExpectedPlanRevision); expected != "" {
		if live := strings.TrimSpace(in.PlanRevision); live != "" && live != expected {
			return PublishResult{Outcome: OutcomeStale, Reason: fmt.Sprintf("plan revision changed: expected %s current %s", expected, live)}, nil
		}
	}
	if in.ExpectedAttempt != 0 && in.ExpectedAttempt != in.Attempt {
		return PublishResult{Outcome: OutcomeStale, Reason: fmt.Sprintf("attempt changed: expected %d current %d", in.ExpectedAttempt, in.Attempt)}, nil
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	evidence := make([]string, 0, len(in.Evidence))
	for _, e := range in.Evidence {
		if e = strings.TrimSpace(e); e != "" {
			evidence = append(evidence, e)
		}
	}
	pub := Publication{
		ContractVersion: ContractVersion,
		ID:              publicationID(taskID, in.Attempt, in.Kind, in.IdempotencyKey, summary, evidence),
		TaskID:          taskID,
		Attempt:         in.Attempt,
		PlanRevision:    strings.TrimSpace(in.PlanRevision),
		RunEpoch:        strings.TrimSpace(in.RunEpoch),
		Kind:            in.Kind,
		Summary:         summary,
		Evidence:        evidence,
		PublishedBy:     strings.TrimSpace(in.PublishedBy),
		IdempotencyKey:  strings.TrimSpace(in.IdempotencyKey),
		CreatedAt:       now.UTC(),
	}

	dir := ResultsDir(projectDir, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return PublishResult{}, fmt.Errorf("taskresult: %w", err)
	}
	path := filepath.Join(dir, pub.ID+".json")
	if existing, err := read(path); err == nil {
		if samePublication(existing, &pub) {
			return PublishResult{Outcome: OutcomeDuplicate, Publication: existing}, nil
		}
		return PublishResult{
			Outcome:     OutcomeRejected,
			Reason:      "idempotency key or content identity reused with a different payload",
			Publication: existing,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return PublishResult{}, err
	}

	data, err := json.MarshalIndent(pub, "", "  ")
	if err != nil {
		return PublishResult{}, fmt.Errorf("taskresult: %w", err)
	}
	if err := writeAtomic(path, append(data, '\n')); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{Outcome: OutcomeAccepted, Publication: &pub}, nil
}

// List returns a task's publications in creation order (then by id, so the order
// is deterministic for identical timestamps).
func List(projectDir, taskID string) ([]Publication, error) {
	entries, err := os.ReadDir(ResultsDir(projectDir, taskID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("taskresult: %w", err)
	}
	dir := ResultsDir(projectDir, taskID)
	out := make([]Publication, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p, err := read(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ResultsDir is the directory holding a task's published results.
func ResultsDir(projectDir, taskID string) string {
	return filepath.Join(projectDir, config.DirName, "runs", taskID, resultsDirName)
}

// publicationID derives a stable identity. With an idempotency key the identity
// is key-scoped, so a re-delivery collides by design; without one it is
// content-scoped, so republishing identical evidence is a duplicate rather than a
// new record.
func publicationID(taskID string, attempt int, kind Kind, key, summary string, evidence []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%s\x00", taskID, attempt, kind)
	if k := strings.TrimSpace(key); k != "" {
		fmt.Fprintf(h, "key\x00%s", k)
	} else {
		fmt.Fprintf(h, "content\x00%s\x00%s", summary, strings.Join(evidence, "\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// samePublication reports whether a stored record describes the same publication
// payload (ignoring the timestamp).
func samePublication(a, b *Publication) bool {
	return a.TaskID == b.TaskID &&
		a.Attempt == b.Attempt &&
		a.Kind == b.Kind &&
		a.Summary == b.Summary &&
		a.IdempotencyKey == b.IdempotencyKey &&
		a.PlanRevision == b.PlanRevision &&
		strings.Join(a.Evidence, "\x00") == strings.Join(b.Evidence, "\x00")
}

// read loads one stored publication, failing closed on a malformed record.
func read(path string) (*Publication, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Publication
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("taskresult: %s: %w", filepath.Base(path), err)
	}
	if p.ContractVersion != ContractVersion || !p.Kind.Known() || p.TaskID == "" || p.ID == "" {
		return nil, fmt.Errorf("taskresult: %s: %w", filepath.Base(path), ErrInvalid)
	}
	return &p, nil
}

// writeAtomic writes data to path via a temporary file in the same directory and
// a rename, so a reader never observes a partial record.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".result-*.tmp")
	if err != nil {
		return fmt.Errorf("taskresult: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("taskresult: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("taskresult: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("taskresult: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("taskresult: %w", err)
	}
	return nil
}
