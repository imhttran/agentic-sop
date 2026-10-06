package cli

import (
	"os"

	"github.com/imhttran/agentic-sop/internal/budget"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/recovery"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/runtrace"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// writeRunTrace composes and persists the canonical structured run trace
// (trace.json) beside the run other artifacts. It is best-effort observation: it
// is never read back to drive a decision, and a write failure never changes the
// run outcome. Every field is taken from evidence the lifecycle already produced;
// anything the runner does not know is left empty rather than invented.
func writeRunTrace(rn *runpkg.Run, res lifeResult, cfg config.Config, tc *runtrace.Collector) {
	in := runtrace.Inputs{
		RunID:        rn.State().ID,
		TaskID:       rn.State().ID,
		StartedAt:    rn.State().CreatedAt,
		CompletedAt:  rn.State().UpdatedAt,
		Execution:    traceExecution(res, cfg),
		Budgets:      traceBudgets(),
		Replans:      traceReplans(rn),
		ChangedFiles: rn.ChangedFiles(),
		Verification: traceVerification(res.suite.Results),
		Termination:  traceTermination(res),
	}
	if tc != nil {
		in.Iterations = tc.Iterations()
		in.RepositoryMutations = tc.Mutations()
	}
	// The collector observes per-mutation activity only for an agent that emits it
	// (the tool harness). When it observed none but the invocation attributed
	// changed files, the attributed set is the invocation repository effect, so the
	// count reflects it rather than understating a real change as zero.
	if in.RepositoryMutations == 0 && len(in.ChangedFiles) > 0 {
		in.RepositoryMutations = len(in.ChangedFiles)
	}
	_ = runtrace.Write(rn.Dir(), runtrace.Build(in))
}

// traceReplans reads the bounded strategy changes from the run attempt records,
// so the trace answers whether replanning occurred, why, and how many. It is
// observation-only and fabricates nothing when no attempt records exist.
func traceReplans(rn *runpkg.Run) []runtrace.ReplanRecord {
	if rn == nil {
		return nil
	}
	var out []runtrace.ReplanRecord
	recs, _ := rn.ReadAttemptRecords()
	for _, a := range recs {
		if a.Action != string(recovery.ActionReplan) {
			continue
		}
		out = append(out, runtrace.ReplanRecord{
			Sequence:    len(out) + 1,
			Reason:      replanReason(a.FailureStage),
			FromAttempt: a.Attempt,
			ToAttempt:   a.Attempt + 1,
		})
	}
	return out
}

// replanReason names why a bounded strategy change was permitted, from the
// deterministic failure cause SOP already recorded for the attempt. It is
// observation-only and fabricates nothing: with no recorded stage it falls back to
// the recovery policy reason.
func replanReason(stage string) string {
	if stage == "" {
		return recovery.ReasonReplan
	}
	return runtrace.OneLine(stage + " failure")
}

// traceBudgets returns the deterministic execution limits that applied to the
// run, resolved from the environment the same way the harness resolves them. It
// is observation-only.
func traceBudgets() runtrace.BudgetLimits {
	b := budget.Resolve(os.Getenv, budget.Defaults())
	return runtrace.BudgetLimits{
		ImplementIterations: b.ImplementIterations,
		FixIterations:       b.FixIterations,
		StaleIterations:     b.StaleIterations,
		ToolCalls:           b.ToolCalls,
	}
}

// traceExecution records what actually executed, keeping the routing DECISION
// (the model class) distinct from the actual EXECUTION target (provider, model,
// locality, and whether an availability fallback supplied it).
func traceExecution(res lifeResult, cfg config.Config) runtrace.Execution {
	// A task run drives the governed implementation lifecycle.
	ex := runtrace.Execution{Capability: "implement"}
	if res.routing != nil {
		ex.ModelClass = string(res.routing.Class)
		ex.RoutingSource = string(res.routing.Source)
		t := res.routing.ExecutionTarget
		if t.Provider != "" || t.Model != "" || t.Locality != "" {
			ex.Provider = t.Provider
			ex.Model = t.Model
			ex.Locality = string(t.Locality)
			ex.ExecutionSource = string(t.Source)
			ex.Fallback = t.Source == model.ExecutionSourceAvailabilityFallback || res.routing.Selection.Fallback
		} else {
			sel := res.routing.Selection
			ex.Provider = sel.Provider
			ex.Model = sel.Model
			ex.Locality = string(sel.Locality)
			ex.ExecutionSource = string(model.ExecutionSourcePrimary)
			ex.Fallback = sel.Fallback
		}
		return ex
	}
	if sel := res.modelSelection; sel != nil {
		ex.ModelClass = string(sel.Class)
		ex.Provider = sel.Provider
		ex.Model = sel.Model
		ex.Locality = string(sel.Locality)
		ex.ExecutionSource = string(model.ExecutionSourcePrimary)
		ex.Fallback = sel.Fallback
		return ex
	}
	// Routing inactive: the configured agent is the execution target. There is no
	// routing decision (no class) and no fallback.
	ex.Provider = cfg.Agent.Provider
	ex.Model = cfg.Agent.Model
	return ex
}

// traceVerification maps the run validation results onto trace records. It uses
// the results already produced; it never runs extra validation for tracing.
func traceVerification(results []testrunner.Result) []runtrace.Verification {
	if len(results) == 0 {
		return nil
	}
	out := make([]runtrace.Verification, 0, len(results))
	for _, r := range results {
		out = append(out, runtrace.Verification{
			Command:    r.Command,
			Status:     string(r.Status),
			ExitCode:   r.ExitCode,
			DurationMS: r.Duration.Milliseconds(),
		})
	}
	return out
}

// traceTermination records why the run stopped, using the existing taxonomy
// (run stage, failure kind, disposition, autonomy action, human-required) without
// redesigning it.
func traceTermination(res lifeResult) runtrace.Termination {
	return runtrace.Termination{
		Stage:         string(res.stage),
		Kind:          string(res.classification.Kind),
		Disposition:   string(res.classification.Disposition),
		Action:        string(res.decision.Action),
		Reason:        runtrace.OneLine(res.decision.Reason),
		Diagnostic:    runtrace.OneLine(res.classification.Reason),
		HumanRequired: res.decision.RequiresHuman,
	}
}
