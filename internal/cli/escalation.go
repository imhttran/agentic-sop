package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/recovery"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// Bounded model escalation (Phase 5).
//
// When an attempt fails a quality gate, SOP's deterministic recovery policy
// (internal/recovery) decides whether to retry the same class, escalate to the
// next larger class, or hand the task to a human. This file is the seam that
// applies that decision: it re-runs the lifecycle on the escalated class, with the
// failed attempt's bounded context handed forward, and persists one attempt record
// per try.
//
// It carries no lifecycle authority of its own. The decision is policy
// (recovery.Decide); the lifecycle, the gate, the approval boundary, and the
// autonomy policy remain unchanged and continue to own every transition. An
// escalated attempt is validated (Phase 4) before it runs and never silently
// substitutes a provider or model: if the escalated class cannot be resolved,
// validated, or executed, escalation stops and the existing recovery path applies.
//
// Escalation is OFF by default and only ever applies to the class a task actually
// ran with, so an installation that does not opt in is byte-for-byte unchanged. An
// explicit --model-class override pins the class and therefore disables automatic
// escalation: the operator's choice is never silently replaced.

// escalationAttempt is the provenance of a lifecycle attempt that SOP's recovery
// policy selected, rather than the router or an operator override. It is threaded
// into the lifecycle so the attempt runs on the escalated class and receives the
// previous attempt's bounded failure context.
type escalationAttempt struct {
	// Decision is the recovery decision that selected this attempt.
	Decision recovery.Decision
	// Selection is the resolved, validated, non-secret model selection this attempt
	// runs on. It is resolved and validated before the agent is built, so the
	// selected model always equals the executing model.
	Selection model.Selection
	// FailureContext is the bounded context handed forward from the failed attempt.
	FailureContext string
}

// escalationContextMax bounds the failure context handed to an escalated attempt,
// so a large validation log is never replayed in full (Phase 5 §24).
const escalationContextMax = 4000

// maxEscalationAttempts is a hard backstop on the attempt loop. The ladder
// (small -> medium -> large) and the configured escalation budget already bound
// it; this only guarantees the loop can never run away.
const maxEscalationAttempts = 4

// runAttempts runs the task lifecycle, applying the bounded escalation policy
// between attempts. It returns the last attempt's result and error, exactly as a
// single executeLifecycle call would, so the caller's existing recovery
// (requeue/block/summarize) is unchanged.
//
// Escalation is transparent: a disabled policy, a passing attempt, an
// infrastructure error, a manual override, or a decision other than ESCALATE all
// return after exactly one lifecycle — the pre-Phase-5 behavior.
func runAttempts(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, spec *taskfile.Spec, rn *runpkg.Run, sess *runSession, approval failure.ApprovalBoundary, tri earlyGateResult, stdout io.Writer) (lifeResult, error) {
	if !recoverableLoop(d) {
		return executeLifecycle(ctx, dir, cfg, a, d, spec, rn, sess, approval, tri, stdout)
	}

	cur, curD := a, d
	var esc *escalationAttempt
	var res lifeResult
	var err error
	escalations := 0
	replans := 0

	for attempt := 1; attempt <= maxEscalationAttempts; attempt++ {
		curD.attempt = esc
		res, err = executeLifecycle(ctx, dir, cfg, cur, curD, spec, rn, sess, approval, tri, stdout)

		// Compute the recovery decision for this attempt. It applies only when the
		// attempt loop is active and a class is known, so an infrastructure failure and
		// an unrouted task keep their existing recovery exactly.
		var dec recovery.Decision
		applies := false
		if class := attemptClass(curD, res); class != "" {
			dec, applies = escalationDecisionFor(curD, res, err, class, escalations, replans, attempt)
		}

		if !applies || (dec.Action != recovery.ActionEscalate && dec.Action != recovery.ActionReplan) {
			if applies {
				printRecoveryDecision(stdout, dec)
			}
			// Persist this attempt's non-secret evidence, so every attempt — the initial
			// routing attempt, each replan, and each escalated retry — is recorded. A
			// write failure is surfaced but never changes the decision.
			recordAttempt(rn, curD, res, dec, attempt, stdout)
			return res, err
		}

		// A bounded replan: change the strategy, not the resource. The SAME agent
		// (class) runs the next attempt, handed the failed attempt's evidence and a
		// strategy-change instruction. The per-invocation budget is intentionally a
		// fresh invocation scope (AGENT-004); the run-level bound is MaxReplans plus
		// the attempt-loop backstop, so a replan cannot multiply the envelope.
		if dec.Action == recovery.ActionReplan {
			recordAttempt(rn, curD, res, dec, attempt, stdout)
			printReplan(stdout, dec, attempt+1)
			replans++
			esc = &escalationAttempt{
				Decision:       dec,
				Selection:      attemptSelection(curD, res),
				FailureContext: boundedReplanContext(res, curD, dec),
			}
			continue
		}

		// The decision is to escalate. Resolve, validate, build, and guard the escalated
		// model BEFORE the attempt is recorded or announced, so an escalation that
		// cannot be honored never claims to have run.
		na, sel, eerr := escalateTo(ctx, cfg, curD, dec)
		if eerr != nil {
			printEscalationUnavailable(stdout, dec, eerr)
			recordAttempt(rn, curD, res, escalationNotApplied(dec), attempt, stdout)
			return res, err
		}

		// The escalation is honored: record the attempt with its true action, then
		// announce it.
		recordAttempt(rn, curD, res, dec, attempt, stdout)
		printRecoveryDecision(stdout, dec)
		printRetrying(stdout, sel, attempt+1)

		escalations++
		esc = &escalationAttempt{
			Decision:       dec,
			Selection:      sel,
			FailureContext: boundedEscalationContext(res, curD, dec),
		}
		cur = na
	}
	return res, err
}

// escalateTo resolves the next class to an agent: it resolves the class, validates
// the selection against the runtime, builds the agent, and guards the IMPLEMENT
// capability. It returns an actionable error instead of substituting a model or a
// provider, so the caller stops escalation and keeps the existing recovery path;
// the selected model therefore always equals the executing model.
func escalateTo(ctx context.Context, cfg config.Config, d deps, dec recovery.Decision) (agent.Agent, model.Selection, error) {
	sel, err := resolveClassSelection(cfg, dec.ToClass, dec.Reason)
	if err != nil {
		return nil, model.Selection{}, err
	}
	// Validate before any agent work, exactly like the initial per-task selection: a
	// model the runtime cannot serve stops the escalation instead of failing
	// mid-execution.
	if err := validateSelection(ctx, cfg, sel); err != nil {
		return nil, model.Selection{}, fmt.Errorf("class %s selection: %w", dec.ToClass, err)
	}
	if d.newAgent == nil {
		// Fail closed (§14): without an agent factory the escalated class cannot be
		// executed, and SOP must never silently continue on the previous model while
		// claiming the task was escalated.
		return nil, model.Selection{}, fmt.Errorf("no agent factory is available to build the %s model", dec.ToClass)
	}
	na, err := d.newAgent(cfg.Agent.Harness, sel.Provider, sel.Model)
	if err != nil {
		return nil, model.Selection{}, fmt.Errorf("build agent for class %s: %w", dec.ToClass, err)
	}
	if err := guardCapability(na, agent.Implement); err != nil {
		return nil, model.Selection{}, err
	}
	return na, sel, nil
}

// escalationNotApplied is the attempt record's decision when the policy chose to
// escalate but the escalation could not be honored: no recovery action was applied
// by this decision, and the existing recovery path continues. Its empty action is
// deliberate — the record must not claim an escalation that never ran.
func escalationNotApplied(dec recovery.Decision) recovery.Decision {
	return recovery.Decision{FromClass: dec.FromClass, Attempt: dec.Attempt}
}

// recoverableLoop reports whether the bounded attempt loop applies at all: the
// recovery policy must permit a recovery action (escalation or replanning). With
// both off — the default — a single executeLifecycle runs, so behavior is
// unchanged.
func recoverableLoop(d deps) bool {
	return escalatable(d) || d.escalation.Replan
}

// escalatable reports whether the bounded-escalation policy applies to this
// invocation at all: it must be enabled, and no explicit --model-class override
// may be pinning the class.
func escalatable(d deps) bool {
	if !d.escalation.Enabled {
		return false
	}
	// An explicit override is the highest-precedence routing input; honoring it
	// means the operator's chosen class is never silently replaced by recovery.
	return strings.TrimSpace(d.modelClass) == ""
}

// escalationDecisionFor computes the recovery decision for a failed attempt, and
// whether recovery applies. It applies only when escalation is enabled, the attempt
// produced a deterministic gate failure (not an infrastructure error), and a class
// is known for the attempt — so an infrastructure failure and an unrouted task keep
// their existing recovery exactly.
func escalationDecisionFor(d deps, res lifeResult, err error, class model.Class, escalations, replans, attempt int) (recovery.Decision, bool) {
	if !recoverableLoop(d) {
		return recovery.Decision{}, false
	}
	// An explicit --model-class override pins the class and disables ESCALATION (the
	// operator's choice is never silently replaced by a stronger model). It does not
	// disable a replan, which keeps the class and only changes the strategy.
	p := d.escalation
	if !escalatable(d) {
		p.Enabled = false
	}
	if err != nil {
		// An agent/infrastructure error is not a quality-gate failure; the existing
		// classification and autonomy policy own it. A bigger model does not fix a
		// transport error.
		return recovery.Decision{}, false
	}
	if res.gate.Decision == quality.Pass {
		return recovery.Decision{}, false
	}
	ev := recovery.EvidenceFrom(res.classification, class, attemptFailureStage(res), escalations, attempt, replans)
	return recovery.Decide(p, ev), true
}

// attemptClass returns the model class the attempt ran on: the recovery-selected
// class for an escalated attempt (which need not re-run the router), otherwise the
// per-task routing decision's class when routing applied, otherwise the run-level
// selection's class when that is active. It is empty when no class is known, in
// which case there is nothing to escalate.
func attemptClass(d deps, res lifeResult) model.Class {
	if d.attempt != nil {
		return d.attempt.Decision.ToClass
	}
	if res.routing != nil {
		return res.routing.Class
	}
	if d.routing.Active {
		return d.routing.Selection.Class
	}
	return ""
}

// attemptSelection returns the model selection the attempt ran on, for the attempt
// record's non-secret evidence.
func attemptSelection(d deps, res lifeResult) model.Selection {
	if d.attempt != nil {
		return d.attempt.Selection
	}
	if res.routing != nil {
		return res.routing.Selection
	}
	if d.routing.Active {
		return d.routing.Selection
	}
	return model.Selection{}
}

// attemptReason returns the deterministic reason the attempt's class was chosen: a
// routing reason (the router's or an operator's) for the first attempt, or the
// recovery reason for an escalated one.
func attemptReason(d deps, res lifeResult) string {
	if d.attempt != nil {
		return d.attempt.Decision.Reason
	}
	if res.routing != nil {
		if len(res.routing.Reasons) > 0 {
			return strings.Join(res.routing.Reasons, "; ")
		}
		return res.routing.Selection.Reason
	}
	if d.routing.Active {
		return d.routing.Selection.Reason
	}
	return ""
}

// attemptFailureStage names where a failed attempt stopped, from SOP's own
// structured evidence (the validation categories, then the review findings). It is
// descriptive only and never selects an action.
func attemptFailureStage(res lifeResult) string {
	if res.gate.Decision == quality.Pass {
		return ""
	}
	if hasCategory(res.suite, testrunner.Build) && !categoryPassed(res.suite, testrunner.Build) {
		return "build"
	}
	if hasCategory(res.suite, testrunner.UnitTest) && !categoryPassed(res.suite, testrunner.UnitTest) {
		return "test"
	}
	if hasCategory(res.suite, testrunner.Lint) && !categoryPassed(res.suite, testrunner.Lint) {
		return "lint"
	}
	if len(res.report.Findings) > 0 {
		return "review"
	}
	return ""
}

// resolveClassSelection resolves a model class to its configured provider/model
// through the same layered resolution every other selection uses. It fails with an
// actionable error rather than substituting a model.
//
// An escalated attempt MUST run the class the ladder selected, so a resolution that
// yields a different class (the model layer's fallback policy) is rejected: silently
// accepting it would downgrade, or mislabel, the escalation.
func resolveClassSelection(cfg config.Config, class model.Class, reason string) (model.Selection, error) {
	res, err := model.Resolve(model.Inputs{
		Config:       cfg.Models,
		Lookup:       os.Getenv,
		RoutedClass:  class,
		RoutedReason: reason,
	})
	if err != nil {
		return model.Selection{}, err
	}
	if !res.Active {
		return model.Selection{}, fmt.Errorf("class %s has no configured model", class)
	}
	if res.Selection.Class != class || res.Selection.Fallback {
		return model.Selection{}, fmt.Errorf("class %s resolved to %q, not the escalated class; recovery refuses a substituted class", class, res.Selection.Class)
	}
	return res.Selection, nil
}

// recordAttempt persists one attempt's non-secret evidence. It is diagnostic only
// (nothing reads it back to drive a decision); a write failure is reported but
// never changes the run.
func recordAttempt(rn *runpkg.Run, d deps, res lifeResult, dec recovery.Decision, attempt int, stdout io.Writer) {
	if rn == nil {
		return
	}
	sel := attemptSelection(d, res)
	result := runpkg.AttemptFailed
	if res.gate.Decision == quality.Pass {
		result = runpkg.AttemptPassed
	}
	rec := runpkg.AttemptRecord{
		Version:      runpkg.AttemptRecordVersion,
		Attempt:      attempt,
		Class:        string(attemptClass(d, res)),
		Provider:     sel.Provider,
		Model:        sel.Model,
		Locality:     string(sel.Locality),
		Reason:       attemptReason(d, res),
		Result:       result,
		FailureStage: attemptFailureStage(res),
		Action:       string(dec.Action),
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := rn.WriteAttemptRecord(rec); err != nil {
		fmt.Fprintf(stdout, "warning: attempt record not persisted: %v\n", err)
	}
}

// printRecoveryDecision renders the deterministic recovery decision, so an
// operator sees why SOP did or did not escalate. Only structured policy reasons
// are shown; no chain-of-thought is ever surfaced.
func printRecoveryDecision(stdout io.Writer, dec recovery.Decision) {
	fmt.Fprintln(stdout, "Recovery:")
	fmt.Fprintf(stdout, "  %-7s %s\n", "Action:", recoveryActionText(dec.Action))
	if dec.FromClass != "" {
		fmt.Fprintf(stdout, "  %-7s %s\n", "From:", classText(dec.FromClass))
	}
	if dec.ToClass != "" {
		fmt.Fprintf(stdout, "  %-7s %s\n", "To:", classText(dec.ToClass))
	}
	if dec.Reason != "" {
		fmt.Fprintf(stdout, "  %-7s %s\n", "Reason:", dec.Reason)
	}
}

// printRetrying announces the escalated attempt and the model it runs on.
func printRetrying(stdout io.Writer, sel model.Selection, attempt int) {
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Retrying (attempt %d):\n", attempt)
	fmt.Fprintf(stdout, "  %-9s %s\n", "Class:", classText(sel.Class))
	fmt.Fprintf(stdout, "  %-9s %s\n", "Provider:", sel.Provider)
	fmt.Fprintf(stdout, "  %-9s %s\n", "Model:", sel.Model)
	fmt.Fprintln(stdout)
}

// printEscalationUnavailable reports that an escalation could not be honored and
// that the existing recovery path continues instead. SOP never silently continues
// on the previous model while reporting an escalation.
func printEscalationUnavailable(stdout io.Writer, dec recovery.Decision, cause error) {
	fmt.Fprintf(stdout, "Recovery: escalation to %s could not be applied: %v\n", classText(dec.ToClass), cause)
	fmt.Fprintln(stdout, "Recovery: keeping the existing recovery path (no silent model substitution)")
}

// recoveryActionText renders a recovery action in the operator-facing upper-case
// style. It never fabricates an action.
func recoveryActionText(a recovery.Action) string {
	return strings.ToUpper(string(a))
}

// classText renders a model class in the operator-facing upper-case style.
func classText(c model.Class) string {
	return strings.ToUpper(string(c))
}

// printReplan announces a bounded strategy change: the same class runs again,
// with a new approach and the failed attempt's evidence. SOP never changes the
// model for a replan (that is escalation) and never resets a budget.
func printReplan(stdout io.Writer, dec recovery.Decision, attempt int) {
	fmt.Fprintf(stdout, "Recovery: replanning (attempt %d) on the same class %s: %s\n", attempt, classText(dec.ToClass), dec.Reason)
}

// boundedReplanContext builds the bounded context handed to a replanned attempt:
// why the previous attempt failed, so the model can change approach rather than
// repeat it. It is evidence and an instruction, never authority: it grants no
// budget, no class change, no tool permission, and no approval.
func boundedReplanContext(res lifeResult, d deps, dec recovery.Decision) string {
	var b strings.Builder
	b.WriteString("# Replan\n\n")
	b.WriteString("A previous attempt at this task did not pass. Change strategy: do not repeat the same approach as before.\n\n")
	b.WriteString("## Why the previous attempt failed\n\n")
	b.WriteString(string(res.gate.Decision))
	for _, r := range res.gate.Reasons {
		b.WriteString("\n- ")
		b.WriteString(r)
	}
	if stage := attemptFailureStage(res); stage != "" {
		b.WriteString("\n- failure stage: ")
		b.WriteString(stage)
	}
	b.WriteString("\n")
	out := b.String()
	if len(out) > escalationContextMax {
		out = out[:escalationContextMax] + "\n… (truncated)\n"
	}
	return out
}

// boundedEscalationContext builds the bounded context handed forward to an
// escalated attempt: what the previous attempt was, why it failed, and the
// structured evidence for it — never the whole repository history or an unbounded
// log.
func boundedEscalationContext(res lifeResult, d deps, dec recovery.Decision) string {
	var b strings.Builder
	b.WriteString("# Escalation\n\n")
	b.WriteString("A previous attempt at this task did not pass, so SOP escalated it to a stronger model.\n")
	b.WriteString("Address the failure below rather than repeating the same change.\n\n")
	b.WriteString("Previous attempt:\n")
	fmt.Fprintf(&b, "  class: %s\n", classText(dec.FromClass))
	if sel := attemptSelection(d, res); sel.Model != "" {
		fmt.Fprintf(&b, "  model: %s\n", sel.Model)
	}
	if stage := attemptFailureStage(res); stage != "" {
		fmt.Fprintf(&b, "  failure stage: %s\n", stage)
	}
	fmt.Fprintf(&b, "\nReason: %s\n", dec.Reason)

	if !res.suite.Passed() {
		b.WriteString("\n")
		b.WriteString(validationFailureContext(res.suite))
	}
	if len(res.report.Findings) > 0 {
		b.WriteString("\n\n# Blocking findings\n\n")
		for _, f := range res.report.Findings {
			location := f.File
			if f.Line > 0 {
				location = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			fmt.Fprintf(&b, "- %s %s — %s: %s\n", f.Severity, location, f.Title, f.Detail)
		}
	}
	return boundedText(strings.TrimSpace(b.String())+"\n", escalationContextMax)
}

// boundedText trims s to at most max bytes at a rune boundary, so an escalated
// attempt is never handed an unbounded log.
func boundedText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n…(truncated)\n"
}
