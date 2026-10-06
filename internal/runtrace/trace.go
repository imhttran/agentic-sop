// Package runtrace defines the canonical, versioned structured run trace: the
// important execution trajectory of an SOP run, persisted beside the other run
// artifacts as trace.json.
//
// The trace observes execution; it never controls it. Nothing in this package is
// read back to drive a routing, lifecycle, retry, progress, approval, or
// termination decision, and a failure to write it must not change the run outcome.
// It composes evidence SOP already owns (routing/execution identity, activity
// events, validation results, and the failure/autonomy decision) rather than
// introducing a parallel execution model or an event framework.
package runtrace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SchemaVersion is the version of the persisted trace contract. It makes future
// trace evolution explicit; AGENT-001 only defines this initial version.
const SchemaVersion = 5

// FileName is the run artifact holding the structured trace.
const FileName = "trace.json"

// maxText bounds one free-text trace field, so a single long diagnostic cannot
// bloat the artifact. Producers already keep activity summaries short; this is a
// defensive cap, not a redaction mechanism.
const maxText = 4096

// Trace is the canonical structured trajectory of one run.
type Trace struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	TaskID        string    `json:"task_id,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`

	Execution Execution `json:"execution"`

	// RepositoryMutations and ChangedFiles summarize the invocation repository
	// effect. RepositoryMutations counts the mutating iterations; ChangedFiles is
	// the invocation-attributed, task-scoped set (never the whole dirty tree).
	RepositoryMutations int      `json:"repository_mutations"`
	ChangedFiles        []string `json:"changed_files,omitempty"`

	Iterations   []Iteration    `json:"iterations"`
	Verification []Verification `json:"verification"`
	Termination  Termination    `json:"termination"`

	// Progress classifies the evidence above into objectively-backed forms of
	// progress, with a deterministic summary. It is observation-only: it never
	// feeds a stale counter, a lifecycle decision, or a termination reason.
	Progress        []ProgressSignal `json:"progress,omitempty"`
	ProgressSummary ProgressCounts   `json:"progress_summary"`

	// Budgets records the deterministic execution limits that applied to the run.
	// It is observation-only: the harness owns and enforces the budgets, and
	// nothing reads this back to drive a decision.
	Budgets BudgetLimits `json:"budgets"`

	// Replans records the bounded strategy changes that occurred, in order. It is
	// observation-only: the harness owns replan eligibility and the bound, and
	// nothing reads this back to drive a decision.
	Replans []ReplanRecord `json:"replans,omitempty"`

	// Context records the Context Engine's observational summary: how much context
	// the harness supplied, from which sources, and whether a limit truncated it. It
	// is observation-only: the model cannot select its own context and nothing reads
	// this back to drive a decision.
	Context ContextInfo `json:"context"`
}

// ContextInfo is the observational summary of the context supplied to the agent.
type ContextInfo struct {
	// Items, Files, and Bytes are the retained context sizes: item count, distinct
	// repository file identities, and content bytes.
	Items int `json:"items"`
	Files int `json:"files"`
	Bytes int `json:"bytes"`
	// Truncated reports whether a context limit dropped or shortened any item.
	Truncated bool `json:"truncated"`
	// Sources is the per-source contribution, in the canonical source order.
	Sources []SourceCount `json:"sources,omitempty"`
}

// SourceCount is one context source's contribution to the supplied context.
type SourceCount struct {
	Source string `json:"source"`
	Items  int    `json:"items"`
	Bytes  int    `json:"bytes"`
}

// ReplanRecord is one bounded strategy change.

type ReplanRecord struct {
	Sequence    int    `json:"sequence"`
	Reason      string `json:"reason,omitempty"`
	FromAttempt int    `json:"from_attempt"`
	ToAttempt   int    `json:"to_attempt"`
}

// BudgetLimits is the applied budget: the deterministic execution limits the
// harness enforced for this run. It answers "how much execution was allowed?"; it
// never answers "why did execution stop?" (that is Termination) or "did useful
// evidence appear?" (that is Progress).
type BudgetLimits struct {
	// ImplementIterations and FixIterations are the hard iteration ceilings.
	ImplementIterations int `json:"implement_iterations"`
	FixIterations       int `json:"fix_iterations"`
	// StaleIterations is the consecutive-stale-turn bound (the no-progress guard).
	StaleIterations int `json:"stale_iterations"`
	// ToolCalls bounds executed tool calls in one invocation.
	ToolCalls int `json:"tool_calls"`
}

// Execution records what actually executed, keeping the routing DECISION (the
// model class) distinct from the actual EXECUTION target (provider/model/locality
// and whether an availability fallback supplied it).
type Execution struct {
	// Capability is the governed capability the run executed (for a task run, the
	// implementation lifecycle).
	Capability string `json:"capability,omitempty"`
	// ModelClass is the routing DECISION class; it is never rewritten by a fallback.
	ModelClass string `json:"model_class,omitempty"`
	// RoutingSource records what selected the class (policy, manual_override,
	// default, escalation). Empty when routing did not apply.
	RoutingSource string `json:"routing_source,omitempty"`
	// Provider, Model, Locality describe the EXECUTING target.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Locality string `json:"locality,omitempty"`
	// ExecutionSource records where the executing model came from (primary or
	// availability-fallback).
	ExecutionSource string `json:"execution_source,omitempty"`
	// Fallback reports whether a fallback supplied the executing selection.
	Fallback bool `json:"fallback"`
}

// Iteration is one observed step of the trajectory (an activity event).
type Iteration struct {
	Sequence           int       `json:"sequence"`
	Phase              string    `json:"phase"`
	Timestamp          time.Time `json:"timestamp"`
	Action             string    `json:"action,omitempty"`
	Observation        string    `json:"observation,omitempty"`
	RepositoryMutation bool      `json:"repository_mutation"`
	ChangedFiles       []string  `json:"changed_files,omitempty"`
	DurationMS         int64     `json:"duration_ms,omitempty"`
	// Signal is the producer-set progress marker for this iteration, when any.
	Signal string `json:"signal,omitempty"`
}

// Verification is one deterministic check the run performed.
type Verification struct {
	Command    string `json:"command"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
}

// Termination makes the run end state understandable, using the existing taxonomy
// (stage, failure kind, disposition, autonomy action, human-required).
type Termination struct {
	Stage         string `json:"stage,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Disposition   string `json:"disposition,omitempty"`
	Action        string `json:"action,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Diagnostic    string `json:"diagnostic,omitempty"`
	HumanRequired bool   `json:"human_required"`
}

// Inputs is the evidence a caller composes into a Trace. The trace is built from
// what the runner already knows; unknown fields are left empty rather than
// invented.
type Inputs struct {
	RunID       string
	TaskID      string
	StartedAt   time.Time
	CompletedAt time.Time

	Execution Execution

	RepositoryMutations int
	ChangedFiles        []string

	Iterations   []Iteration
	Verification []Verification
	Termination  Termination

	Budgets BudgetLimits

	Replans []ReplanRecord

	Context ContextInfo
}

// Build assembles a versioned Trace from the supplied evidence.
func Build(in Inputs) Trace {
	progress := classifyProgress(in)
	return Trace{
		SchemaVersion:       SchemaVersion,
		RunID:               in.RunID,
		TaskID:              in.TaskID,
		StartedAt:           in.StartedAt,
		CompletedAt:         in.CompletedAt,
		Execution:           in.Execution,
		RepositoryMutations: in.RepositoryMutations,
		ChangedFiles:        in.ChangedFiles,
		Iterations:          in.Iterations,
		Verification:        in.Verification,
		Termination:         in.Termination,
		Progress:            progress,
		ProgressSummary:     summarize(progress),
		Budgets:             in.Budgets,
		Replans:             in.Replans,
		Context:             in.Context,
	}
}

// Write persists the trace as trace.json in the run directory. It is best-effort
// observability: the caller treats a failure as diagnostic and never lets it
// change the run outcome.
func Write(runDir string, t Trace) error {
	if strings.TrimSpace(runDir) == "" {
		return fmt.Errorf("runtrace: run directory is required")
	}
	if t.SchemaVersion != SchemaVersion {
		return fmt.Errorf("runtrace: unsupported schema version %d", t.SchemaVersion)
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("runtrace: marshal: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, FileName), append(data, 0x0a), 0o644); err != nil {
		return fmt.Errorf("runtrace: write %s: %w", FileName, err)
	}
	return nil
}

// OneLine collapses a field to a single trimmed line and caps its length, so a
// stray newline or an oversized diagnostic cannot break the one-record contract
// or bloat the artifact.
func OneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, 0x0a); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if len(s) > maxText {
		s = s[:maxText]
	}
	return s
}
