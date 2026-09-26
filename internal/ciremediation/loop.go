// Package ciremediation fixes ordinary CI failures within a bounded loop. It
// decides whether a failure is actionable (deterministically) and, if so, hands
// it to an explicit Remediator port; it never lets a model decide whether a CI
// failure is a code defect.
package ciremediation

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sdlc/internal/github"
)

// Outcome is the terminal outcome of a remediation run.
type Outcome string

const (
	// Pass: CI reported no failing checks.
	Pass Outcome = "PASS"
	// Blocked: CI still failing and cannot/should not be remediated further.
	Blocked Outcome = "BLOCKED"
)

// Failure is the collected CI failure.
type Failure struct {
	Checks []github.Check
	Logs   string
}

// Result reports the outcome and how many remediation attempts were made
// (a failed attempt still counts).
type Result struct {
	Outcome  Outcome
	Attempts int
}

// CI observes the current CI status.
type CI interface {
	Checks(ctx context.Context) ([]github.Check, error)
	FailureLogs(ctx context.Context) (string, error)
}

// Remediator performs one bounded remediation attempt (diagnose/fix/local
// test/commit/push) and records it.
type Remediator interface {
	Attempt(ctx context.Context, failure Failure) error
}

// Classifier decides whether a failure is actionable.
type Classifier func(failure Failure) bool

// DefaultClassifier treats a failure with collected logs as actionable.
func DefaultClassifier(failure Failure) bool {
	return strings.TrimSpace(failure.Logs) != ""
}

// Loop runs the bounded remediation loop.
type Loop struct {
	ci          CI
	remediator  Remediator
	maxAttempts int
	classify    Classifier
}

// NewLoop builds a Loop. maxAttempts is coerced to at least 1; a nil classifier
// uses DefaultClassifier.
func NewLoop(ci CI, remediator Remediator, maxAttempts int, classify Classifier) *Loop {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if classify == nil {
		classify = DefaultClassifier
	}
	return &Loop{ci: ci, remediator: remediator, maxAttempts: maxAttempts, classify: classify}
}

// Run checks CI, remediates actionable failures, and re-checks, until CI passes
// or the attempt budget is exhausted.
func (l *Loop) Run(ctx context.Context) (Result, error) {
	attempts := 0
	for {
		if err := ctx.Err(); err != nil {
			return Result{Attempts: attempts}, err
		}

		checks, err := l.ci.Checks(ctx)
		if err != nil {
			return Result{Attempts: attempts}, err
		}
		failing := failingChecks(checks)
		if len(failing) == 0 {
			return Result{Outcome: Pass, Attempts: attempts}, nil
		}

		logs, err := l.ci.FailureLogs(ctx)
		if err != nil {
			return Result{Attempts: attempts}, err
		}
		failure := Failure{Checks: failing, Logs: logs}

		if !l.classify(failure) {
			return Result{Outcome: Blocked, Attempts: attempts}, nil
		}
		if attempts >= l.maxAttempts {
			return Result{Outcome: Blocked, Attempts: attempts}, nil
		}
		if err := l.remediator.Attempt(ctx, failure); err != nil {
			attempts++
			return Result{Attempts: attempts}, err
		}
		attempts++
	}
}

func failingChecks(checks []github.Check) []github.Check {
	var failing []github.Check
	for _, check := range checks {
		if check.State == github.CheckFail || check.State == github.CheckCancel {
			failing = append(failing, check)
		}
	}
	return failing
}
