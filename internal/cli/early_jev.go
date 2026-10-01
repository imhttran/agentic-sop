package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// Early JEV checkpoints (Phase 3): task triage and pre-execution.
//
// These are optional, disabled-by-default checkpoints that obtain bounded early
// engineering evidence before implementation begins. They are analysis only:
//
//   - JEV produces structured evidence; SOP policy decides (autonomy.DecideEarly);
//   - this seam never transitions task state, mutates the repository, or touches
//     SOP persistence beyond a diagnostic run artifact;
//   - a disabled or absent checkpoint is a strict no-op, so existing behavior is
//     unchanged when the gates are off (PRD-Phase-3-OpenJEV FR-P3-6).
//
// The distinction between an analysis result and an infrastructure/provider
// failure is explicit: a provider failure (or malformed/invalid evidence) is
// recorded as a failure with no evidence payload and never interpreted as a
// finding (FR-P3-9, FR-P3-10).
//
// Observability: each checkpoint emits a stage-correct activity event (TRIAGE or
// PRE_EXECUTION, distinct from the quality QA stage) carrying a concise, secret-free
// summary of findings, the highest severity seen, and (when stated) the analysis
// confidence. The activity/confidence detail is human-readable only; it never
// feeds a decision, gate, or task state. A disabled checkpoint emits nothing, so
// JEV off adds no observability noise (FR-P3-6).

// earlyGateResult is the outcome of one early checkpoint. It is provenance plus a
// deterministic disposition; it carries no authority of its own.
type earlyGateResult struct {
	// Ran reports whether the checkpoint was attempted (enabled and an analyzer
	// present). A checkpoint that did not run is a no-op, not a failure.
	Ran bool
	// Task is the task the checkpoint evaluated.
	Task string
	// Provider names the analysis provider, when known.
	Provider string
	// Evidence is the validated structured evidence, when the analysis produced
	// any. It is zero when the analysis failed or produced no structured evidence.
	Evidence jev.Evidence
	// Decision is the deterministic autonomy disposition. It is zero when the
	// analysis did not produce usable evidence.
	Decision autonomy.Decision
	// Escalate is true when policy requires a human boundary.
	Escalate bool
	// ProviderFailed records that the analysis could not produce a usable result.
	// It is never a finding.
	ProviderFailed bool
	// Reason explains the disposition (descriptive provenance only).
	Reason string
}

// earlyGateEnabled reports whether the checkpoint is enabled. Both gates are OFF
// by default and each is gated on its own early_jev.gates.* flag ANDed with
// early_jev.enabled.
func earlyGateEnabled(cfg config.Config, checkpoint runpkg.Checkpoint) bool {
	switch checkpoint {
	case runpkg.CheckpointTaskTriage:
		return cfg.EarlyJEVTaskTriageEnabled()
	case runpkg.CheckpointPreExecution:
		return cfg.EarlyJEVPreExecutionEnabled()
	default:
		return false
	}
}

// earlyPurpose maps a checkpoint to the JEV analysis purpose it runs under
// (FR-P3-2), so evidence is never conflated across checkpoints.
func earlyPurpose(checkpoint runpkg.Checkpoint) jev.Purpose {
	if checkpoint == runpkg.CheckpointPreExecution {
		return jev.PurposePreExecution
	}
	return jev.PurposeTaskTriage
}

// earlyStage maps a checkpoint to its activity stage (PRD §10.2.4), so triage,
// pre-execution, and quality evidence are distinguishable in the activity stream.
func earlyStage(checkpoint runpkg.Checkpoint) string {
	if checkpoint == runpkg.CheckpointPreExecution {
		return activity.StagePreExecution
	}
	return activity.StageTriage
}

// earlyGateLabel is the human-readable checkpoint name for activity lines.
func earlyGateLabel(checkpoint runpkg.Checkpoint) string {
	if checkpoint == runpkg.CheckpointPreExecution {
		return "pre-execution JEV analysis"
	}
	return "JEV task analysis"
}

// buildEarlyJEVInvocation assembles the bounded, read-only request an early
// checkpoint sends JEV. It reuses runpkg.JEVInvocation (the boundary the quality
// seam uses) so there is one request shape across the JEV integration, and it
// carries only task-scoped read-only context: no runtime handle, no persistence.
func buildEarlyJEVInvocation(checkpoint runpkg.Checkpoint, spec *taskfile.Spec) runpkg.JEVInvocation {
	inv := runpkg.JEVInvocation{
		Purpose:  earlyPurpose(checkpoint),
		Task:     strings.TrimSpace(spec.Title + "\n\n" + spec.Description),
		Criteria: strings.Join(spec.AcceptanceCriteria, "\n"),
	}
	var ctx strings.Builder
	if len(spec.Dependencies) > 0 {
		ctx.WriteString("dependencies:\n- " + strings.Join(spec.Dependencies, "\n- "))
	}
	if len(spec.Requirements) > 0 {
		if ctx.Len() > 0 {
			ctx.WriteString("\n\n")
		}
		ctx.WriteString("requirements:\n- " + strings.Join(spec.Requirements, "\n- "))
	}
	inv.RepositoryContext = ctx.String()
	return inv
}

// runEarlyGate invokes one early JEV checkpoint when it is enabled, applies
// deterministic policy, and persists a diagnostic artifact. It is read-only and
// returns a zero result (a strict no-op) when the checkpoint is disabled, no
// analyzer is wired, or the analyzer cannot be built — none of which is fatal.
//
// rn is the task's run, used only to persist the diagnostic artifact; a nil rn
// skips persistence and changes nothing else.
func runEarlyGate(ctx context.Context, cfg config.Config, d deps, spec *taskfile.Spec, rn *runpkg.Run, checkpoint runpkg.Checkpoint) earlyGateResult {
	return runJEVCheckpoint(ctx, cfg, d, checkpoint, buildEarlyJEVInvocation(checkpoint, spec), spec.ID, rn)
}

// runJEVCheckpoint runs one early JEV analysis over an arbitrary bounded
// invocation and applies deterministic policy to the result. It is the shared core
// of every early checkpoint: runEarlyGate feeds it a task-derived invocation, and
// the prompt path (sop prompt) feeds it a prompt-derived one, so prompt and task
// evidence is interpreted identically and never diverges.
//
// It is read-only and returns a zero result (a strict no-op) when the checkpoint is
// disabled, no analyzer is wired, or the analyzer cannot be built — none of which
// is fatal. An infrastructure/provider failure is recorded distinctly and is never
// a finding (FR-P3-9, FR-P3-10).
func runJEVCheckpoint(ctx context.Context, cfg config.Config, d deps, checkpoint runpkg.Checkpoint, inv runpkg.JEVInvocation, taskID string, rn *runpkg.Run) earlyGateResult {
	if !earlyGateEnabled(cfg, checkpoint) {
		return earlyGateResult{}
	}
	if d.newJEVAnalyzer == nil {
		return earlyGateResult{}
	}
	analyzer, err := d.newJEVAnalyzer(cfg)
	if err != nil || analyzer == nil {
		return earlyGateResult{}
	}

	rec := activity.FromContext(ctx)
	rec.Emit(earlyStage(checkpoint), "analyzing", earlyGateLabel(checkpoint))

	out := runpkg.RunJEV(ctx, analyzer, inv)

	res := earlyGateResult{Ran: true, Task: taskID, Provider: out.Provider}
	switch {
	case out.FailClosed() || out.Result.Status == jev.StatusError || out.Result.Status == jev.StatusIncomplete:
		// An infrastructure/provider failure is recorded distinctly and is never a
		// finding. Policy owns the fallback: the advisory default continues.
		res.ProviderFailed = true
		res.Reason = "early JEV analysis could not run; continuing under deterministic SOP policy"
	case out.Result.StructuredEvidence != nil:
		ev := *out.Result.StructuredEvidence
		if err := ev.Validate(); err != nil {
			// Malformed/invalid evidence fails closed: it is never a pass and is
			// never interpreted as a finding.
			res.ProviderFailed = true
			res.Reason = "early JEV analysis returned invalid evidence; continuing under deterministic SOP policy"
		} else {
			res.Evidence = ev
			res.Decision = autonomy.DecideEarly(ev, cfg.EarlyJEVFailOn(), cfg.AutonomyPolicy())
			res.Escalate = res.Decision.RequiresHuman
			res.Reason = res.Decision.Reason
		}
	default:
		// The analysis completed without structured evidence. It is advisory and
		// carries no blocking signal; policy continues.
		res.Reason = "early JEV analysis produced no structured evidence; continuing under deterministic SOP policy"
	}

	rec.Emit(earlyStage(checkpoint), earlyGateAction(res), earlyGateDetail(res))

	if rn != nil {
		art := runpkg.EarlyArtifact{
			Version:        runpkg.EarlyArtifactVersion,
			Checkpoint:     checkpoint,
			Task:           taskID,
			Provider:       out.Provider,
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
			ProviderFailed: res.ProviderFailed,
			PolicyDecision: string(res.Decision.Action),
			PolicyReason:   res.Reason,
		}
		if !res.ProviderFailed {
			art.Evidence = res.Evidence
		}
		// Persistence is diagnostic and best-effort: a write failure never fails,
		// blocks, or alters the run.
		_ = rn.WriteEarlyJEVArtifact(art)
	}
	return res
}

// earlyGateAction summarizes the checkpoint outcome for the activity stream.
func earlyGateAction(res earlyGateResult) string {
	switch {
	case res.Escalate:
		return "escalating"
	case res.ProviderFailed:
		return "unavailable"
	default:
		return "completed"
	}
}

// earlyGateDetail renders a concise, secret-free activity detail. It surfaces the
// finding count, the highest severity seen, and (when the analysis stated one) the
// confidence, so triage/pre-execution evidence is visible in the activity stream
// without ever emitting raw prompts or model output. The confidence and severity
// are display-only: they describe the analysis, they never feed a decision.
func earlyGateDetail(res earlyGateResult) string {
	var b strings.Builder
	if res.ProviderFailed {
		b.WriteString("provider failure recorded")
	} else {
		b.WriteString("findings: " + strconv.Itoa(len(res.Evidence.Items)))
		if sev := earlyHighestSeverity(res.Evidence); sev != "" {
			b.WriteString("; severity: " + sev)
		}
		if conf, stated := earlyConfidence(res.Evidence); stated {
			b.WriteString("; confidence: " + conf)
		}
	}
	if action := strings.TrimSpace(string(res.Decision.Action)); action != "" {
		b.WriteString("; disposition: " + action)
	}
	return b.String()
}

// earlyHighestSeverity returns the highest severity among the evidence items (by
// the shared JEV severity rank), or "" when there are no findings. It only
// summarizes existing values, so it never invents a severity.
func earlyHighestSeverity(ev jev.Evidence) string {
	best := ""
	bestRank := -1
	for _, item := range ev.Items {
		sev := strings.ToUpper(strings.TrimSpace(string(item.Severity)))
		if sev == "" {
			continue
		}
		if rank := earlySeverityRank(sev); rank > bestRank {
			best, bestRank = sev, rank
		}
	}
	return best
}

// earlySeverityRank orders the shared severity vocabulary high-to-low. An unknown
// severity ranks lowest so it never outranks a known one.
func earlySeverityRank(sev string) int {
	switch jev.Severity(sev) {
	case jev.SeverityCritical:
		return 4
	case jev.SeverityHigh:
		return 3
	case jev.SeverityMedium:
		return 2
	case jev.SeverityLow:
		return 1
	case jev.SeverityInfo:
		return 0
	default:
		return -1
	}
}

// earlyConfidence renders the analysis confidence as a bounded 0.00–1.00 value
// and reports whether it was stated. Confidence is only rendered when the
// analysis actually produced a result (it carries status/severity); a stated
// confidence of exactly 0.00 is a real, if low, judgment and is rendered as
// "0.00" rather than silently dropped. When no analysis produced evidence there
// is nothing to report and the value is unstated.
func earlyConfidence(ev jev.Evidence) (string, bool) {
	if !ev.Status.Valid() {
		return "", false
	}
	if ev.Confidence < 0 {
		return "", false
	}
	if ev.Confidence > 1 {
		// Out-of-range confidence is not a stated judgment; never fabricate one.
		return "", false
	}
	return fmt.Sprintf("%.2f", ev.Confidence), true
}
