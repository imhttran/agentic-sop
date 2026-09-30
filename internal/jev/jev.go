// Package jev defines the boundary between SOP and JEV (the optional
// engineering-analysis capability described in docs/plans/PLAN-JEV-Implementation.md and
// docs/requirements/PRD-JEV.md).
//
// # Boundary
//
// JEV is a non-owner analysis capability invoked by SOP. It may:
//
//   - inspect relevant repository files,
//   - inspect changes associated with the current task,
//   - inspect task acceptance criteria,
//   - inspect validation/review results,
//   - identify quality concerns,
//   - return structured findings.
//
// JEV may not:
//
//   - transition task state directly,
//   - mutate SOP persistence,
//   - mark validation or review successful,
//   - commit, push, open PRs, or merge,
//   - bypass human gates.
//
// SOP remains the lifecycle owner: it decides what a result means and which
// lifecycle transition (if any) follows. JEV only analyzes; IMPLEMENT/FIX change
// code. This package therefore exposes no method by which an implementation can
// transition task state, mutate SOP persistence, or mark validation/review
// successful — the Analyzer interface only receives bounded context and returns
// structured findings.
//
// The interface is additive and replaceable: implementations are selected
// outside core lifecycle code, and existing execution providers remain usable
// without JEV. Absence of a JEV implementation is never an error for the normal
// SOP lifecycle.
//
// See docs/specs/OPENJEV.md for the authoritative boundary specification.
package jev

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sop/internal/review"
)

// Status is the structured outcome of a JEV analysis. It is a result value, not
// a lifecycle transition: SOP policy decides what a status implies for a task.
type Status string

const (
	// StatusPass: JEV completed and found nothing worth reporting.
	StatusPass Status = "PASS"
	// StatusFindings: JEV completed and returned one or more findings.
	StatusFindings Status = "FINDINGS"
	// StatusIncomplete: JEV could not finish its bounded execution. It is never
	// treated as a pass.
	StatusIncomplete Status = "INCOMPLETE"
	// StatusError: JEV failed to run. It is never treated as a pass.
	StatusError Status = "ERROR"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusPass, StatusFindings, StatusIncomplete, StatusError:
		return true
	default:
		return false
	}
}

// Finding is a single, evidence-oriented JEV finding. It carries no authority:
// it describes a quality concern and where it was observed.
type Finding struct {
	ID       string
	Severity review.Severity
	Category string
	Path     string
	Line     int
	Message  string
	Evidence string
}

// Result is the structured outcome of a JEV analysis. It is machine-readable
// data that SOP policy consumes; it is never a lifecycle command.
//
// StructuredEvidence is an additive field: it carries the validated, versioned
// early JEV analysis evidence contract (see evidence.go). Existing consumers
// that ignore it keep compiling and observing the pre-existing serialized shape
// unchanged, so Result stays backward compatible. It is optional; absent
// evidence means "no structured evidence was computed" and is not a pass.
type Result struct {
	Status   Status
	Findings []Finding
	Summary  string

	// StructuredEvidence carries the validated early JEV analysis evidence, when
	// an analyzer computes it. It is validated through the fail-closed contract
	// validator and never derived from Summary text.
	StructuredEvidence *Evidence
}

// Validate rejects malformed results. Callers must treat an invalid result as a
// failure (fail closed) rather than a pass: malformed JEV output must never
// silently become PASS.
//
// Structured evidence, when present, is validated through the same fail-closed
// contract validator, so an unknown purpose, severity, or status in the
// evidence fails the whole result rather than being silently accepted. Summary
// text is never inspected: it is descriptive only and not a decision input.
func (r Result) Validate() error {
	if !r.Status.Valid() {
		return errInvalidStatus(string(r.Status))
	}
	for _, f := range r.Findings {
		if !f.Severity.Valid() {
			return errInvalidSeverity(string(f.Severity))
		}
	}
	if r.Status == StatusPass && len(r.Findings) > 0 {
		return errPassWithFindings
	}
	if r.Status == StatusFindings && len(r.Findings) == 0 {
		return errFindingsWithoutFindings
	}
	if r.StructuredEvidence != nil {
		if err := r.StructuredEvidence.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Request is the bounded context JEV needs to analyze a task. It deliberately
// excludes ownership of the SOP runtime and persistence layer: JEV receives
// read-only snapshots, never handles it could use to mutate task state.
type Request struct {
	// Purpose is the checkpoint/purpose the analysis is being run under (for
	// example TASK_TRIAGE or PRE_EXECUTION). It is provenance for the analyzer:
	// it names why the analysis ran, and it carries no authority. It is optional
	// and advisory; an empty value means the caller did not declare a purpose and
	// existing callers keep working unchanged.
	Purpose Purpose
	// Task is the task description.
	Task string
	// Criteria is the task's acceptance criteria.
	Criteria string
	// ChangedFiles are the paths changed by the task's implementation.
	ChangedFiles []string
	// RepositoryContext is read-only repository context (for example a diff or
	// file excerpts) assembled by SOP.
	RepositoryContext string
	// ValidationResult summarizes the deterministic validation outcome.
	ValidationResult string
	// ReviewResult summarizes the review outcome.
	ReviewResult string
}

// Analyzer is the boundary through which SOP invokes JEV. It is the only way
// SOP talks to JEV.
//
// An Analyzer must not transition task state, mutate SOP persistence, mark
// validation or review successful, or perform Git/PR/merge actions. It analyzes
// bounded context and returns structured findings; SOP decides what happens
// next. Implementations are selected outside core lifecycle code so JEV is
// replaceable and optional.
type Analyzer interface {
	Analyze(ctx context.Context, req Request) (Result, error)
}

// contextText renders the request into bounded prompt text. It is a helper for
// adapters and keeps the boundary's read-only, snapshot nature explicit.
func (r Request) contextText() string {
	parts := []string{}
	for _, p := range []string{r.Task, r.Criteria, r.RepositoryContext, r.ValidationResult, r.ReviewResult} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	if len(r.ChangedFiles) > 0 {
		parts = append(parts, "changed files:\n"+strings.Join(r.ChangedFiles, "\n"))
	}
	return strings.Join(parts, "\n\n")
}
