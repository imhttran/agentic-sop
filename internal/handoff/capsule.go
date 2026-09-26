// Package handoff produces a small, deterministic handoff capsule after a task
// completes, so later work can reuse the durable meaning of that task without
// re-reading its full history. Authoritative truth stays in SQLite workflow
// state, Git, verification results, and remote PR/CI state; a capsule is a
// derived summary, never workflow truth. Optional mechanical compression of
// bulky artifacts is a bounded optimization behind a provider-independent
// Compressor.
package handoff

import "github.com/imhttran/agentic-sop/internal/domain"

// VerificationSummary records the outcome of one verification check.
type VerificationSummary struct {
	Check  string `json:"check"`
	Status string `json:"status"`
}

// Capsule is the small deterministic schema describing a completed task. It is
// deliberately bounded: it captures meaning, not history.
type Capsule struct {
	TaskID       string                `json:"task"`
	Result       domain.TaskStatus     `json:"result"`
	Summary      string                `json:"summary,omitempty"`
	Changes      []string              `json:"changes,omitempty"`
	Decisions    []string              `json:"decisions,omitempty"`
	Files        []string              `json:"files,omitempty"`
	Verification []VerificationSummary `json:"verification,omitempty"`
	CarryForward []string              `json:"carry_forward,omitempty"`
}

// Facts are the explicit, caller-supplied details a capsule carries. Task id and
// result come from authoritative task state, never from Facts. Facts are
// normalized deterministically (trimmed, de-duplicated, bounded).
type Facts struct {
	Summary      string
	Changes      []string
	Decisions    []string
	Files        []string
	Verification []VerificationSummary
	CarryForward []string
}
