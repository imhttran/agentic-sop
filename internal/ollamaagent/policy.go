package ollamaagent

import (
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/budget"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// CapabilityPolicy bounds and constrains one capability's agent loop. It is the
// single authoritative statement of how long a capability may run and which
// tools it may use, so capability checks are not scattered through the harness.
type CapabilityPolicy struct {
	// MaxIterations is the maximum number of model turns for this capability. It
	// is a ceiling, not a target: the model is expected to finish well before it.
	MaxIterations int
	// ReadOnly is true when the capability must not modify the repository. It is
	// descriptive (the prompt tells the model) as well as enforced (AllowedTools
	// omits the mutation tools).
	ReadOnly bool
	// AllowedTools is the set of tool names the capability may call. A nil set
	// allows every tool.
	AllowedTools map[string]bool

	// The finalize fields apply only to the phased mutating capabilities
	// (IMPLEMENT, FIX); they are zero for every other capability, which never runs
	// the phased engine. They mirror the policy constants of the same meaning.

	// FinalizeAfter is the interaction count after which a mutated run that has
	// stopped writing becomes eligible to finalize.
	FinalizeAfter int
	// LateStageAfter is the interaction count at which an unmutated run enters
	// finalization so it can report truthfully that no change was needed.
	LateStageAfter int
	// CompletionWindow is how many non-mutating interactions after the last
	// mutation mean the model has stopped writing.
	CompletionWindow int
	// ForceFinalizeAfter is the iteration at which a mutated run is finalized
	// regardless, reserving the closing turns for a tool-free outcome.
	ForceFinalizeAfter int
	// FinalizeTurns bounds the model turns spent in FINALIZE before the run fails
	// with a finalization-specific diagnostic.
	FinalizeTurns int
}

// Allows reports whether the capability may call tool. An unknown tool is
// allowed through so the tool harness can reject it as unsupported, which is a
// more specific message than a capability denial.
func (p CapabilityPolicy) Allows(tool string) bool {
	if p.AllowedTools == nil || !allTools[tool] {
		return true
	}
	return p.AllowedTools[tool]
}

// ToolList returns the allowed tool names in the canonical order, for the prompt
// and for denial messages.
func (p CapabilityPolicy) ToolList() []string {
	if p.AllowedTools == nil {
		return toolharness.Tools()
	}
	out := make([]string, 0, len(p.AllowedTools))
	for _, t := range toolharness.Tools() {
		if p.AllowedTools[t] {
			out = append(out, t)
		}
	}
	return out
}

// The one centralized capability policy.
//
// This file is the single source of truth for how a capability completes and how
// long it may run. Budgets are safety rails, not the completion mechanism: every
// capability is expected to finish because its lifecycle reaches a defined end,
// long before a ceiling. Nothing here grows a budget because a model explored a
// lot — exploration never justifies more turns; only completing the work does.
//
// Two kinds of bound are kept visibly distinct:
//
//   - Hard ceilings stop a run that would otherwise never stop. Reaching one is
//     always an abnormal termination, reported by a *Limit / *Exhausted error.
//   - Soft thresholds steer an otherwise-healthy run: nudges, phase transitions,
//     and finalization windows. A soft threshold never finalizes a run that
//     changed nothing, and never truncates a run that is still writing.
//
// Completion policies (independent of any budget):
//
//	PLAN          DISCOVERY (bounded read-only)  → SYNTHESIS (tools disabled,
//	              bounded); completes by returning the plan document.
//	REVIEW        INSPECT   (bounded read-only, soft wrap-up nudge) →
//	              SYNTHESIZE (tools disabled, bounded); completes by returning
//	              the review result.
//	IMPLEMENT/FIX DISCOVER → CHANGE (entered on the first observed mutation) →
//	              FINALIZE (tools disabled, bounded); completes by returning the
//	              structured outcome. Completion is mutation-aware: a soft
//	              threshold alone never finalizes a run that changed nothing, and
//	              a run still writing is never finalized mid-change.
//	others        generic bounded loop (executeLoop); completes by returning the
//	              final JSON object.
//
// Lifecycle bounds live only here: the engines (orchestrate.go, harness.go) and
// the capability files (plan.go, review.go, implement.go, fix.go) reference these
// named constants rather than inline numbers. Tool-call limits and command
// timeouts are deployment configuration (config.go), not capability policy.
const (
	// --- Hard ceilings: stop a run that would otherwise not stop. ---
	//
	// A hard ceiling is never the completion mechanism; it only terminates a run
	// that would otherwise not stop, and reaching one is always abnormal.

	// IMPLEMENT/FIX hard iteration ceilings. Completion is returning the
	// structured outcome (early, or from FINALIZE); these only stop a run that
	// would not stop. IMPLEMENT's ceiling is larger because it may need several
	// writes plus focused checks within one invocation; FIX arrives with a
	// diagnosis, so it needs less.
	// The hard ceilings are the canonical budget defaults: the policy names them,
	// and the budget owns their operator configuration. They are not independent
	// numbers.
	maxIterationsImplement = budget.DefaultImplementIterations
	maxIterationsFix       = budget.DefaultFixIterations

	// maxNoProgressIterations bounds consecutive stale turns before the first
	// mutation. Novel successful repository discovery -- an inspection or a
	// non-mutating command -- may reset the streak only through implementNowAfter
	// model turns. Afterwards, all non-mutating turns are stale. The repetition
	// guard independently detects consecutive loops.
	maxNoProgressIterations = budget.DefaultStaleIterations

	// Other hard ceilings.
	maxIterationsDesignTests = 12
	maxIterationsDiagnose    = 12
	maxIterationsDefault     = 12

	// implementForceFinalizeAfter reserves the final model turns for tool-free
	// finalization. Once a mutation has been observed, a model may not consume the
	// entire invocation budget by continuing to request repository tools. It is a
	// hard cutoff enforced together with implementFinalizeTurns, not a nudge.
	implementForceFinalizeAfter = 28
	// implementFinalizeTurns bounds model turns inside FINALIZE before the run
	// fails with a finalization-specific diagnostic. Like the other ceilings, it
	// is a stop for a run that would not stop, not a completion target.
	implementFinalizeTurns = 3

	fixForceFinalizeAfter = 20
	fixFinalizeTurns      = 3

	// --- Soft thresholds: steer a healthy run toward its completion state. ---
	//
	// A soft threshold never finalizes a run that changed nothing, and never
	// truncates a run that is still writing. Soft thresholds are interaction
	// counts: they steer a productive run without withdrawing tools from it.

	// PLAN phase bounds. Discovery ends synthesis; synthesis is tool-free.
	planDiscoveryTurns = 8
	planSynthesisTurns = 2

	// REVIEW phase bounds. The change under review is in the request, so
	// inspection is short: at most reviewInspectTurns read-only tool calls. The
	// soft nudge at reviewInspectNudgeAfter tells a still-inspecting model to
	// wrap up; it does not disable anything. Synthesis is tool-free.
	reviewInspectTurns      = 8
	reviewInspectNudgeAfter = 6
	reviewSynthesizeTurns   = 2

	// IMPLEMENT soft thresholds. They are interaction counts, not turn counts of
	// good or bad behavior: they steer a healthy run without withdrawing tools
	// from a productive one.
	//
	// implementNudgeAfter: tell a long discovery to start writing.
	implementNudgeAfter = 6
	// implementNowAfter: an unmutated run is told, strongly, to implement now.
	// It sits below implementFinalizeAfter so a model that only starts writing
	// when told still has turns left to change and return. The instruction is
	// RE-STATED on every subsequent interaction (not sent once), because a one-shot
	// nudge is easy for a small model to outrun: it reads on and the instruction is
	// many turns stale by the time the run finalizes.
	implementNowAfter = 12
	// implementClosingAfter: the interaction count from which an unmutated run's
	// re-stated steering becomes closing — it states that this invocation is about
	// to end, so a model that has only been reading is told the consequence (this
	// invocation ends without a change; SOP continues the task later) while its
	// tools are still available. It sits below implementLateStageAfter so the
	// warning always precedes the cutoff, and above implementNowAfter so the
	// standard instruction is restated first.
	implementClosingAfter = 19
	// implementFinalizeAfter: a mutated run becomes eligible to finalize once it
	// has stopped writing for the completion window. Crossing it without a
	// mutation does not finalize: a count is not evidence the work is done.
	implementFinalizeAfter = 18
	// implementCompletionWindow: how many non-mutating interactions after the
	// last mutation mean the model has stopped writing. A model still writing
	// resets the count, so it is never finalized mid-change.
	implementCompletionWindow = 2
	// implementLateStageAfter: the late-stage decision point for a run that has
	// never mutated: one final instruction to implement or report truthfully. A
	// mutated run is never forced to conclude here — only once it stops writing.
	// For an unmutated run it is the point from which finalization may be entered;
	// the run then has implementFinalizeTurns to return an outcome.
	implementLateStageAfter = 22

	// --- Derived reference totals (not bounds in themselves). ---

	// PLAN/REVIEW total turn ceiling. PLAN and REVIEW run the two-phase engine
	// (executeTwoPhase), whose phase bounds above are authoritative; these are
	// their reference totals, used by policy introspection (PolicyFor).
	planTotalTurns   = planDiscoveryTurns + planSynthesisTurns
	reviewTotalTurns = reviewInspectTurns + reviewSynthesizeTurns
)

// allTools is every tool the shared harness implements, so a capability policy
// can tell a known-but-disallowed tool from an unknown one.
var allTools = toolset(toolharness.Tools()...)

var (
	// readTools can inspect the repository but never modify it.
	readTools = toolset(
		toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles,
		toolharness.ToolGitStatus, toolharness.ToolGitDiff,
	)
	// inspectTools add non-mutating command execution (to reproduce or observe
	// behavior) but still cannot write files.
	inspectTools = toolset(
		toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles,
		toolharness.ToolRunCommand, toolharness.ToolGitStatus, toolharness.ToolGitDiff,
	)
)

// Optional operator overrides for the IMPLEMENT/FIX iteration ceilings now live
// in the canonical budget (internal/budget), resolved from the SOP_OLLAMA_*
// environment. Raising a ceiling gives a model that narrates between actions more
// turns before the ceiling stops the run; the soft thresholds
// (finalize/late-stage/force-finalize) scale with it so their relative steering
// position is preserved. An unset, non-numeric, or non-positive value keeps the
// built-in ceiling, so behavior is unchanged when they are not set.

// implementIterations is the effective IMPLEMENT iteration ceiling.
func implementIterations(b budget.Budget) int { return b.ImplementIterations }

// fixIterations is the effective FIX iteration ceiling.
func fixIterations(b budget.Budget) int { return b.FixIterations }

// staleIterations is the effective consecutive-stale-turn bound (the no-progress
// guard), read from the canonical budget.
func staleIterations(b budget.Budget) int { return b.StaleIterations }

// scaleIterations maps a soft threshold defined against base onto a ceiling of n,
// rounding up, so a raised ceiling moves every threshold to the same relative
// position. A ceiling at or below the built-in base keeps the threshold verbatim.
func scaleIterations(n, threshold, base int) int {
	if n <= base {
		return threshold
	}
	scaled := (n*threshold + base - 1) / base
	if scaled > n {
		scaled = n
	}
	return scaled
}

// toolset builds a tool-name set.
func toolset(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// PolicyFor returns the policy for a capability. An unknown capability gets the
// conservative read-only default, so a future capability cannot accidentally gain
// write access before it is considered here.
//
// Each case names the completion policy the capability is expected to reach; the
// budget (MaxIterations) is only the safety ceiling behind it.
func PolicyFor(c agent.Capability) CapabilityPolicy {
	return policyFor(c, resolveBudget())
}

// resolveBudget resolves the canonical budget from the environment, so a
// package-level PolicyFor call matches what ConfigFromEnv would enforce.
func resolveBudget() budget.Budget {
	return budget.Resolve(os.Getenv, budget.Defaults())
}

// policyFor returns the capability policy for c under budget b. The hard
// iteration ceiling comes from b; the soft steering thresholds scale with it.
func policyFor(c agent.Capability, b budget.Budget) CapabilityPolicy {
	switch c {
	case agent.Plan:
		// Completion: DISCOVERY (bounded read-only) → SYNTHESIS (tool-free)
		// returning the plan document (executeTwoPhase, plan.go). MaxIterations is
		// the total turn ceiling for reference; the phase bounds are authoritative.
		return CapabilityPolicy{MaxIterations: planTotalTurns, ReadOnly: true, AllowedTools: readTools}
	case agent.Review:
		// Completion: INSPECT (bounded read-only, soft wrap-up nudge) → SYNTHESIZE
		// (tool-free) returning the review result (executeTwoPhase, review.go).
		// MaxIterations is the reference total.
		return CapabilityPolicy{MaxIterations: reviewTotalTurns, ReadOnly: true, AllowedTools: readTools}
	case agent.DesignTests:
		// Completion: return the final JSON object from the generic bounded loop.
		return CapabilityPolicy{MaxIterations: maxIterationsDesignTests, ReadOnly: true, AllowedTools: readTools}
	case agent.DiagnoseFailure:
		// Completion: return the final JSON object from the generic bounded loop.
		return CapabilityPolicy{MaxIterations: maxIterationsDiagnose, ReadOnly: true, AllowedTools: inspectTools}
	case agent.Implement:
		n := implementIterations(b)
		return CapabilityPolicy{
			MaxIterations:      n,
			AllowedTools:       allTools,
			FinalizeAfter:      scaleIterations(n, implementFinalizeAfter, maxIterationsImplement),
			LateStageAfter:     scaleIterations(n, implementLateStageAfter, maxIterationsImplement),
			CompletionWindow:   implementCompletionWindow,
			ForceFinalizeAfter: scaleIterations(n, implementForceFinalizeAfter, maxIterationsImplement),
			FinalizeTurns:      implementFinalizeTurns,
		}
	case agent.Fix:
		n := fixIterations(b)
		return CapabilityPolicy{
			MaxIterations:      n,
			AllowedTools:       allTools,
			FinalizeAfter:      scaleIterations(n, implementFinalizeAfter, maxIterationsImplement),
			LateStageAfter:     scaleIterations(n, implementLateStageAfter, maxIterationsImplement),
			CompletionWindow:   implementCompletionWindow,
			ForceFinalizeAfter: scaleIterations(n, fixForceFinalizeAfter, maxIterationsFix),
			FinalizeTurns:      fixFinalizeTurns,
		}
	default:
		// Conservative read-only default; completion is the generic bounded loop.
		return CapabilityPolicy{MaxIterations: maxIterationsDefault, ReadOnly: true, AllowedTools: readTools}
	}
}

// describeTools renders a tool set for a message.
func describeTools(p CapabilityPolicy) string {
	return strings.Join(p.ToolList(), ", ")
}
