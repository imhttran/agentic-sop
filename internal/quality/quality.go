// Package quality is the deterministic quality gate. It decides whether a task
// passes, fails, or needs a human, from verification status, unresolved review
// findings, the fix-loop budget, and human-required flags. It never calls a
// model; policy lives in configuration, not in the verdict.
//
// An optional JEV analysis result may be supplied as additional evidence (see
// JEVEvidence). JEV is non-authoritative: it can only contribute findings the
// gate already knows how to weigh, never a transition of its own. The verdict
// still derives from the checks, the review findings, the fix-loop budget, and
// the human-required flag — supplying JEV evidence cannot turn a fail into a
// pass, and a malformed/incomplete/errored JEV result never becomes a pass.
//
// # JEV quality policy (JEV008)
//
// JEV findings are classified by severity (see jev_policy.go): HIGH and CRITICAL
// fail the gate, while INFO, LOW, and MEDIUM are reported without blocking. A
// blocking JEV finding makes the gate FAIL, which enters the existing FIX
// lifecycle under the existing fix budget; the policy adds no JEV-specific retry
// counter and no unbounded loop.
package quality

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/review"
)

// Decision is the quality gate's verdict.
type Decision string

const (
	// Pass: every required check passed and no blocking finding remains.
	Pass Decision = "PASS"
	// Fail: a required check failed or a blocking finding remains within budget.
	Fail Decision = "FAIL"
	// Continue: the run did not pass, but no human decision is required: the
	// invocation stopped short of finishing (a budget/no-change exhaustion or a
	// transient provider failure) and the task should be requeued through the
	// existing bounded continuation path.
	//
	// Evaluate never returns Continue: like Pass and Fail it is decided from the
	// gate's evidence, whereas Continue describes a run that never reached a gate
	// verdict. It is produced by the lifecycle when a resumable continuation is
	// detected, so the operator is told SOP will continue on its own rather than
	// that a human is needed.
	Continue Decision = "CONTINUE"
	// NeedsHuman: a human must decide (approval requested or budget exhausted).
	NeedsHuman Decision = "NEEDS_HUMAN"
)

// Result is a verdict with the reasons that produced it.
type Result struct {
	Decision Decision
	Reasons  []string
}

// Input is the deterministic evidence the gate decides on.
type Input struct {
	// BuildPassed, TestPassed, and LintPassed are the verification outcomes.
	BuildPassed  bool
	TestPassed   bool
	LintRequired bool
	LintPassed   bool
	// Unresolved are the review findings still open after any fixes.
	Unresolved []review.Finding
	// FixCycles is how many fix cycles have run; the budget comes from policy.
	FixCycles int
	// HumanRequired forces a human decision regardless of the checks.
	HumanRequired bool
	// JEV is the optional structured analysis result supplied as additional
	// evidence. A nil value means no JEV analysis ran (disabled or absent), and
	// leaves the verdict exactly as before.
	JEV *JEVEvidence
	// ValidationRequired reports that this run deterministically requires
	// configured validation before it may pass: an ordinary task whose invocation
	// changed the repository. It is derived from the task's execution mode and
	// SOP's own mutation observation, never from model output.
	ValidationRequired bool
	// ValidationConfigured reports whether any validation command is configured for
	// the run. When a required validation set is absent, the gate must not pass: an
	// empty suite is not evidence that a change is correct.
	ValidationConfigured bool
}

// JEVEvidence is a validated snapshot of an optional JEV analysis, shaped for
// the gate. It is evidence only: the gate derives its verdict from its own
// inputs and treats JEV findings like additional findings at their severity.
//
// FailClosed marks a JEV run that could not produce a usable pass — a malformed
// result, or an INCOMPLETE/ERROR status. Such a result is never a pass.
type JEVEvidence struct {
	// Status is the reported result status (PASS, FINDINGS, INCOMPLETE, ERROR).
	Status jev.Status
	// Findings are the reported findings (empty unless FINDINGS).
	Findings []jev.Finding
	// Summary is an optional human-readable summary.
	Summary string
	// FailClosed is set when the result could not be validated or was not a
	// clean pass; a fail-closed result never produces a PASS verdict.
	FailClosed bool
	// Reason describes why a fail-closed result was not a pass.
	Reason string
}

// NewJEVEvidence validates a raw JEV result against the policy and shapes it for
// the gate. It fails closed: a malformed result, or an INCOMPLETE/ERROR status,
// yields a non-pass (FailClosed) evidence whose reason explains why. A nil or
// errored analysis is handled by the caller; a non-nil err here is only a
// malformed-result signal, never a pass.
func NewJEVEvidence(res jev.Result) JEVEvidence {
	if err := res.Validate(); err != nil {
		return JEVEvidence{FailClosed: true, Reason: "JEV result invalid (fail closed): " + err.Error()}
	}
	ev := JEVEvidence{Status: res.Status, Findings: res.Findings, Summary: res.Summary}
	switch res.Status {
	case jev.StatusPass:
		// A validated PASS is the only status that can contribute nothing.
	case jev.StatusFindings:
		// Findings are evidence; the gate weighs them by severity below.
	case jev.StatusIncomplete, jev.StatusError:
		ev.FailClosed = true
		ev.Reason = "JEV analysis did not complete (" + string(res.Status) + ")"
	}
	return ev
}

// Evaluate applies the configured policy to the inputs. A human decision takes
// precedence over a plain failure; an exhausted fix loop turns a still-failing
// task into a human decision rather than looping forever.
//
// JEV evidence, when supplied, is folded in under the JEV quality policy (see
// jev_policy.go): HIGH/CRITICAL findings and fail-closed results count like any
// other failed check and can only add reasons or reinforce a failure, while
// INFO/LOW/MEDIUM findings are surfaced as report-only reasons that never flip
// the verdict. JEV absence changes nothing.
func Evaluate(policy config.Quality, in Input) Result {
	reasons := []string{}

	if in.HumanRequired {
		return Result{Decision: NeedsHuman, Reasons: append(reasons, "human approval required")}
	}

	// A required validation set that is not configured is never a pass: an empty
	// suite proves nothing about the change. The gate fails closed (FAIL, not
	// NEEDS_HUMAN) so the failure is classified and handled deterministically as a
	// configuration failure. It never becomes an approval the operator did not ask
	// for, and it never lets an unverified change pass.
	if in.ValidationRequired && !in.ValidationConfigured {
		reasons = append(reasons, "required validation is not configured")
		return Result{Decision: Fail, Reasons: reasons}
	}

	checkFailed := false
	if !in.BuildPassed {
		reasons = append(reasons, "build failed")
		checkFailed = true
	}
	if policy.RequiresTests() && !in.TestPassed {
		reasons = append(reasons, "tests failed or were not run")
		checkFailed = true
	}
	if in.LintRequired && !in.LintPassed {
		reasons = append(reasons, "lint failed")
		checkFailed = true
	}

	blocking := BlockingFindings(policy.FailOn, in.Unresolved)

	// JEV evidence: blocking findings and fail-closed results count like any
	// other failed check. They are additive — they cannot clear a failure.
	jevBlocking := JEVBlockingFindings(policy.JEVFailOn(), in.JEV)
	jevFailClosed := in.JEV != nil && in.JEV.FailClosed

	if in.FixCycles >= policy.MaxFixCycles && (checkFailed || blocking > 0 || jevBlocking > 0 || jevFailClosed) {
		reasons = append(reasons, fmt.Sprintf("fix-loop limit reached (%d/%d)", in.FixCycles, policy.MaxFixCycles))
		return Result{Decision: NeedsHuman, Reasons: reasons}
	}

	if blocking > 0 {
		reasons = append(reasons, fmt.Sprintf("%d unresolved finding(s) at a failing severity", blocking))
		checkFailed = true
	}

	if jevBlocking > 0 {
		reasons = append(reasons, fmt.Sprintf("%d JEV finding(s) at a failing severity", jevBlocking))
		checkFailed = true
	}
	if jevFailClosed {
		reasons = append(reasons, jevReason(in.JEV))
		checkFailed = true
	}

	// Non-blocking JEV findings (INFO/LOW/MEDIUM) are report-only: they are
	// surfaced for visibility but never flip the verdict on their own.
	reasons = append(reasons, JEVReportFindings(policy.JEVFailOn(), in.JEV)...)

	if checkFailed {
		return Result{Decision: Fail, Reasons: reasons}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "all required checks passed")
	}
	return Result{Decision: Pass, Reasons: reasons}
}

// jevReason renders a fail-closed JEV evidence reason, falling back to a generic
// message when the evidence carries none.
func jevReason(ev *JEVEvidence) string {
	if ev == nil || strings.TrimSpace(ev.Reason) == "" {
		return "JEV analysis was not a clean pass"
	}
	return ev.Reason
}

// JEVBlockingFindings counts JEV findings whose severity is named in the JEV
// fail_on list (the blocking severities under the JEV quality policy). A nil
// evidence or an empty list blocks nothing. A fail-closed evidence contributes a
// single non-clearing reason instead of a count.
func JEVBlockingFindings(failOn []string, ev *JEVEvidence) int {
	if ev == nil || ev.FailClosed {
		return 0
	}
	return BlockingFindings(failOn, jevFindingsToReview(ev.Findings))
}

// jevFindingsToReview converts JEV findings into review findings so the gate
// reuses its single severity policy instead of maintaining a second one.
func jevFindingsToReview(findings []jev.Finding) []review.Finding {
	if len(findings) == 0 {
		return nil
	}
	out := make([]review.Finding, 0, len(findings))
	for _, f := range findings {
		out = append(out, review.Finding{
			Severity: f.Severity,
			Title:    f.Category,
			Detail:   f.Message,
			File:     f.Path,
			Line:     f.Line,
		})
	}
	return out
}

// BlockingFindings counts unresolved findings whose severity is named in the
// policy's fail_on list. An empty list blocks nothing. It is exported so the
// review stage and the gate agree on what blocks.
func BlockingFindings(failOn []string, findings []review.Finding) int {
	if len(failOn) == 0 {
		return 0
	}
	set := make(map[review.Severity]bool, len(failOn))
	for _, s := range failOn {
		set[review.Severity(strings.ToUpper(strings.TrimSpace(s)))] = true
	}
	n := 0
	for _, f := range findings {
		if set[f.Severity] {
			n++
		}
	}
	return n
}
