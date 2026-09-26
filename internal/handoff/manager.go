package handoff

import (
	"context"
	"os"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// Manager builds and persists handoffs at the task boundary. Compression is a
// bounded optimization: it never changes task state, and a compression failure
// leaves the capsule available and reports the failure on the record.
type Manager struct {
	store      Store
	compressor Compressor
	maxBytes   int
	now        func() time.Time
	// Env is the environment used to redact secret values from handoff content.
	// It defaults to the process environment.
	Env []string
}

// New returns a Manager. A non-positive maxBytes disables compression (handoffs
// are still recorded, with Status NOT_REQUESTED). A nil compressor is treated as
// compression disabled.
func New(store Store, compressor Compressor, maxBytes int) *Manager {
	return &Manager{
		store:      store,
		compressor: compressor,
		maxBytes:   maxBytes,
		now:        time.Now,
		Env:        os.Environ(),
	}
}

// Complete builds and persists a handoff for a completed task. Building and
// persistence errors propagate; a compression failure does not — it is recorded
// (Status FAILED) and Complete still succeeds, so completed work is never
// invalidated by optional compression.
func (m *Manager) Complete(ctx context.Context, task *domain.Task, facts Facts, artifacts []Artifact) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}

	capsule, err := NewBuilder().Build(task, facts)
	if err != nil {
		return Record{}, err
	}
	capsule = redactCapsule(capsule, m.Env)
	artifacts = RedactAll(artifacts, m.Env)

	record := Record{
		TaskID:    capsule.TaskID,
		Capsule:   capsule,
		Status:    StatusNotRequested,
		CreatedAt: m.now(),
	}

	if m.compressor != nil && m.overBudget(artifacts) {
		compressed, err := m.compressor.Compress(ctx, ContextBundle{Capsule: capsule, Artifacts: artifacts})
		switch {
		case err != nil && ctx.Err() != nil:
			return Record{}, ctx.Err()
		case err != nil:
			record.Status = StatusFailed
			record.CompressionError = boundError(Redact(err.Error(), m.Env))
		default:
			record.Status = StatusCompressed
			record.Content = compressed.Content
			record.References = compressed.References
		}
	}

	if err := m.store.SaveHandoff(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// maxErrorLen bounds a persisted compression error.
const maxErrorLen = 500

// boundError truncates an error string so a noisy compressor cannot bloat the
// persisted record. Errors are already redacted by the caller.
func boundError(message string) string {
	if len(message) > maxErrorLen {
		return message[:maxErrorLen]
	}
	return message
}

// overBudget reports whether the bulky artifacts exceed the configured budget. A
// non-positive budget disables compression.
func (m *Manager) overBudget(artifacts []Artifact) bool {
	if m.maxBytes <= 0 {
		return false
	}
	total := 0
	for _, artifact := range artifacts {
		total += len(artifact.Content)
		if total > m.maxBytes {
			return true
		}
	}
	return false
}
