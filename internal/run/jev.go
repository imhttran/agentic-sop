package run

import (
	"context"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/jev"
)

// JEVInvocation is the bounded, read-only context SOP hands to JEV at the
// quality seam. It carries the same task-specific evidence the quality gate
// sees, so JEV can add analysis without owning any lifecycle decision.
type JEVInvocation struct {
	// Purpose is the checkpoint/purpose the analysis runs under (for example
	// TASK_TRIAGE or PRE_EXECUTION). It is provenance for the analyzer and
	// carries no authority; an empty value means the caller did not declare one.
	Purpose jev.Purpose
	// Task is the task description.
	Task string
	// Criteria is the task's acceptance criteria.
	Criteria string
	// ChangedFiles are the paths the task's implementation changed.
	ChangedFiles []string
	// RepositoryContext is read-only repository context (for example the diff).
	RepositoryContext string
	// ValidationResult summarizes the deterministic validation outcome.
	ValidationResult string
	// ReviewResult summarizes the review outcome.
	ReviewResult string
}

// request converts the invocation into the bounded jev.Request. It is a pure
// mapping: no runtime handle, no persistence, no state path crosses the
// boundary.
func (inv JEVInvocation) request() jev.Request {
	return jev.Request{
		Purpose:           inv.Purpose,
		Task:              inv.Task,
		Criteria:          inv.Criteria,
		ChangedFiles:      inv.ChangedFiles,
		RepositoryContext: inv.RepositoryContext,
		ValidationResult:  inv.ValidationResult,
		ReviewResult:      inv.ReviewResult,
	}
}

// Diagnostics is an optional capability an Analyzer may implement to name what
// performed the analysis and what it cost, so SOP can record JEV invocation
// metrics. It is diagnostic metadata only: a missing implementation simply
// yields no metrics, and missing metrics never imply PASS or FAIL.
type Diagnostics interface {
	// JEVProvider names the provider (for example "ollama").
	JEVProvider() string
	// JEVModel names the model the analysis used.
	JEVModel() string
	// JEVToolCalls reports the number of tool/provider calls the analysis made.
	JEVToolCalls() int
}

// JEVOutcome is the result of an optional JEV invocation. It never carries a
// lifecycle transition: SOP policy decides what, if anything, a result means.
//
// It deliberately exposes the analyzer error separately from the result and
// marks an errored run as fail-closed, so a caller cannot mistake a provider
// outage (or a zero value) for a clean pass: Ran with an error always yields
// FailClosed, and an errored Result is the zero value, never a PASS.
type JEVOutcome struct {
	// Ran reports whether a JEV analyzer was present and invoked.
	Ran bool
	// Result is the structured result when Ran is true and the analyzer
	// succeeded. When the analyzer errored or was absent, Result is the zero
	// value and Error explains why.
	Result jev.Result
	// Error is the analyzer failure, if any. It is recorded as evidence for the
	// lifecycle (and reported), never raised as a lifecycle error.
	Error error
	// DurationMS is the invocation's measured duration in milliseconds. It is
	// diagnostic only and is zero when JEV did not run.
	DurationMS int64
	// Provider, Model, and ToolCalls are diagnostic metadata about the
	// invocation, recorded when the analyzer reports them. They are never read
	// to drive a decision, and their absence never implies PASS or FAIL.
	Provider  string
	Model     string
	ToolCalls int
}

// FailClosed reports whether the outcome must be treated as a non-pass: an
// analyzer that ran but did not produce a clean result. An outcome that never
// ran (absent analyzer) is not fail-closed — there is simply no evidence.
func (o JEVOutcome) FailClosed() bool {
	return o.Ran && o.Error != nil
}

// RunJEV invokes the optional JEV analysis at the quality seam. It runs the
// analyzer only when one is present; an absent analyzer (JEV disabled or no
// implementation) is a no-op, never an error, so the lifecycle proceeds
// unchanged.
//
// RunJEV performs no state transition, persistence mutation, or Git/PR/merge
// action. It only calls the Analyzer's read-only Analyze and returns its result.
// On error it returns a fail-closed outcome with a zero Result, so the error
// path can never be mistaken for a pass. It also measures the invocation's
// duration and, when the analyzer implements Diagnostics, records its
// provider/model/tool calls — diagnostic metrics only.
func RunJEV(ctx context.Context, a jev.Analyzer, inv JEVInvocation) JEVOutcome {
	if a == nil {
		return JEVOutcome{Ran: false}
	}
	start := time.Now()
	res, err := a.Analyze(ctx, inv.request())
	out := JEVOutcome{Ran: true, DurationMS: time.Since(start).Milliseconds()}
	if d, ok := a.(Diagnostics); ok {
		out.Provider = d.JEVProvider()
		out.Model = d.JEVModel()
		out.ToolCalls = d.JEVToolCalls()
	}
	if err != nil {
		// A provider/analyzer error is never a pass: discard any partial result
		// so a caller can only observe the fail-closed outcome.
		out.Error = err
		return out
	}
	out.Result = res
	return out
}

// Summary renders the validation outcome as bounded text for JEV.
func Summary(results []string) string {
	var b strings.Builder
	for _, r := range results {
		if s := strings.TrimSpace(r); s != "" {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}
