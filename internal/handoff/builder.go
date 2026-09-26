package handoff

import (
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sdlc/internal/domain"
)

// maxItems bounds every list so a capsule stays small and deterministic.
const maxItems = 20

// Builder builds capsules from authoritative task state plus explicit facts.
type Builder struct{}

// NewBuilder returns a Builder.
func NewBuilder() *Builder { return &Builder{} }

// Build produces a capsule for a completed task. The task must be DONE; a
// capsule is only meaningful for finished work. It never invents facts: task id
// and result are read from the task, everything else is the caller's explicit
// input, normalized deterministically.
func (b *Builder) Build(task *domain.Task, facts Facts) (Capsule, error) {
	if task == nil {
		return Capsule{}, errors.New("handoff: task is nil")
	}
	if task.Status != domain.DONE {
		return Capsule{}, fmt.Errorf("handoff: task %s is %s, not DONE", task.ID, task.Status)
	}

	return Capsule{
		TaskID:       task.ID,
		Result:       task.Status,
		Summary:      strings.TrimSpace(facts.Summary),
		Changes:      normalize(facts.Changes),
		Decisions:    normalize(facts.Decisions),
		Files:        normalize(facts.Files),
		Verification: normalizeVerification(facts.Verification),
		CarryForward: normalize(facts.CarryForward),
	}, nil
}

// normalize trims, drops empty entries, removes duplicates, and bounds the list,
// preserving first-seen order so output is deterministic.
func normalize(items []string) []string {
	var out []string
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
		if len(out) == maxItems {
			break
		}
	}
	return out
}

func normalizeVerification(items []VerificationSummary) []VerificationSummary {
	var out []VerificationSummary
	seen := make(map[VerificationSummary]bool, len(items))
	for _, item := range items {
		item.Check = strings.TrimSpace(item.Check)
		item.Status = strings.TrimSpace(item.Status)
		if (item.Check == "" && item.Status == "") || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
		if len(out) == maxItems {
			break
		}
	}
	return out
}
