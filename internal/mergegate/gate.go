// Package mergegate decides whether validated work may merge and, if so, merges
// it through an explicit Merger port. It evaluates the required evidence
// deterministically; a model never decides whether a merge is allowed. An unmet
// condition blocks the merge rather than bypassing repository protection.
package mergegate

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sdlc/internal/github"
)

// Outcome is the terminal outcome of a merge attempt.
type Outcome string

const (
	// Merged: every required condition held and the merge was performed.
	Merged Outcome = "MERGED"
	// Blocked: a required condition was unmet; no merge was performed.
	Blocked Outcome = "BLOCKED"
)

// Reason identifies the condition that blocked a merge.
type Reason string

const (
	// ReasonNone: no blocking reason (a merge succeeded).
	ReasonNone Reason = ""
	// ReasonTests: local required tests have not passed.
	ReasonTests Reason = "LOCAL_TESTS_FAILED"
	// ReasonReview: required review has not passed.
	ReasonReview Reason = "REVIEW_FAILED"
	// ReasonChecks: GitHub checks are failing, pending, or unknown.
	ReasonChecks Reason = "CHECKS_NOT_PASSING"
	// ReasonMergeable: the PR is conflicting or of unknown mergeability.
	ReasonMergeable Reason = "NOT_MERGEABLE"
)

// Input is the evidence the gate evaluates before merging.
type Input struct {
	// PR is the remote pull request; its Number and Mergeable field are used.
	PR github.PullRequest
	// Checks are the GitHub checks for the PR.
	Checks []github.Check
	// TestsPassed reports whether the local required tests passed.
	TestsPassed bool
	// ReviewPassed reports whether the required review passed.
	ReviewPassed bool
}

// Result is the outcome of a merge attempt with the blocking reason, if any.
type Result struct {
	Outcome Outcome
	Reason  Reason
}

// Merger merges a pull request. The GitHub adapter owns the actual `gh pr merge`
// call and honours repository protection.
type Merger interface {
	Merge(ctx context.Context, prNumber int, method string) error
}

// Gate evaluates the merge preconditions and performs the merge.
type Gate struct {
	merger Merger
	method string
}

// New returns a Gate that merges through m. An empty method defaults to squash.
func New(m Merger, method string) *Gate {
	if strings.TrimSpace(method) == "" {
		method = "squash"
	}
	return &Gate{merger: m, method: method}
}

// Merge evaluates the required conditions and, only when all hold, merges the
// pull request. A blocked result performs no merge; merger errors and
// cancellation propagate.
func (g *Gate) Merge(ctx context.Context, in Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	if !in.TestsPassed {
		return Result{Outcome: Blocked, Reason: ReasonTests}, nil
	}
	if !in.ReviewPassed {
		return Result{Outcome: Blocked, Reason: ReasonReview}, nil
	}
	if checksBlock(in.Checks) {
		return Result{Outcome: Blocked, Reason: ReasonChecks}, nil
	}
	if !mergeable(in.PR.Mergeable) {
		return Result{Outcome: Blocked, Reason: ReasonMergeable}, nil
	}

	if err := g.merger.Merge(ctx, in.PR.Number, g.method); err != nil {
		return Result{}, err
	}
	return Result{Outcome: Merged}, nil
}

// checksBlock reports whether any check is not in a ready state. Pending,
// failing, cancelled, and unknown checks all block; only passing and skipping
// checks are ready.
func checksBlock(checks []github.Check) bool {
	for _, check := range checks {
		if check.State != github.CheckPass && check.State != github.CheckSkipping {
			return true
		}
	}
	return false
}

// mergeable reports whether GitHub considers the PR safe to merge. Anything
// other than an explicit MERGEABLE (including UNKNOWN) blocks.
func mergeable(state string) bool {
	return strings.EqualFold(strings.TrimSpace(state), "MERGEABLE")
}
