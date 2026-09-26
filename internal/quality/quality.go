// Package quality is the deterministic quality gate. It decides whether a task
// passes, fails, or needs a human, from verification status, unresolved review
// findings, the fix-loop budget, and human-required flags. It never calls a
// model; policy lives in configuration, not in the verdict.
package quality

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sdlc/internal/config"
	"github.com/imhttran/agentic-sdlc/internal/review"
)

// Decision is the quality gate's verdict.
type Decision string

const (
	// Pass: every required check passed and no blocking finding remains.
	Pass Decision = "PASS"
	// Fail: a required check failed or a blocking finding remains within budget.
	Fail Decision = "FAIL"
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
}

// Evaluate applies the configured policy to the inputs. A human decision takes
// precedence over a plain failure; an exhausted fix loop turns a still-failing
// task into a human decision rather than looping forever.
func Evaluate(policy config.Quality, in Input) Result {
	reasons := []string{}

	if in.HumanRequired {
		return Result{Decision: NeedsHuman, Reasons: append(reasons, "human approval required")}
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

	blocking := blockingFindings(policy.FailOn, in.Unresolved)

	if in.FixCycles >= policy.MaxFixCycles && (checkFailed || blocking > 0) {
		reasons = append(reasons, fmt.Sprintf("fix-loop limit reached (%d/%d)", in.FixCycles, policy.MaxFixCycles))
		return Result{Decision: NeedsHuman, Reasons: reasons}
	}

	if blocking > 0 {
		reasons = append(reasons, fmt.Sprintf("%d unresolved finding(s) at a failing severity", blocking))
		checkFailed = true
	}

	if checkFailed {
		return Result{Decision: Fail, Reasons: reasons}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "all required checks passed")
	}
	return Result{Decision: Pass, Reasons: reasons}
}

// blockingFindings counts unresolved findings whose severity is named in the
// policy's fail_on list. An empty list blocks nothing.
func blockingFindings(failOn []string, findings []review.Finding) int {
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
