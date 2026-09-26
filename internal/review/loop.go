package review

import (
	"context"
	"errors"
	"fmt"

	"github.com/imhttran/agentic-sdlc/internal/testrunner"
)

// Fixer applies a fix for the blocking findings in a report.
type Fixer interface {
	Fix(ctx context.Context, report Report) error
}

// Tester re-verifies the working tree after a fix.
type Tester interface {
	Run(ctx context.Context) testrunner.Result
}

// Loop runs a bounded review/fix cycle. Only the configured threshold decides
// whether findings block; the model does not decide pass/fail.
type Loop struct {
	provider  Provider
	fixer     Fixer
	tester    Tester
	threshold Severity
	maxRounds int
}

// NewLoop builds a Loop with the given threshold and maximum review rounds.
// maxRounds is coerced to at least 1 so a review always happens once.
func NewLoop(provider Provider, fixer Fixer, tester Tester, threshold Severity, maxRounds int) *Loop {
	if maxRounds < 1 {
		maxRounds = 1
	}
	return &Loop{provider: provider, fixer: fixer, tester: tester, threshold: threshold, maxRounds: maxRounds}
}

// Run reviews, applying a fix and re-verifying after each blocking round, until
// there are no blocking findings or the round budget is exhausted.
func (l *Loop) Run(ctx context.Context, request Request) (Report, error) {
	var last Report
	for round := 0; round < l.maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return last, err
		}

		report, err := l.provider.Review(ctx, request)
		if err != nil {
			return last, err
		}
		last = report
		if !report.Blocking(l.threshold) {
			return report, nil
		}

		// Blocking findings: fix and re-verify, then re-review if budget remains.
		if round < l.maxRounds-1 {
			if err := l.fixer.Fix(ctx, report); err != nil {
				return report, err
			}
			result := l.tester.Run(ctx)
			if result.Status == testrunner.Canceled {
				return report, ctxErr(ctx)
			}
			if result.Status != testrunner.Pass {
				return report, fmt.Errorf("review fix did not verify: %s", result.Status)
			}
		}
	}

	return last, fmt.Errorf("review still has blocking findings after %d round(s)", l.maxRounds)
}

func ctxErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("operation canceled")
}
