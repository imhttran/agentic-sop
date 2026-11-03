package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/executionlease"
	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/resume"
	"github.com/imhttran/agentic-sop/internal/store"
)

// runResume reports the next legal action for interrupted work. With an explicit
// id it resumes that task; otherwise it resumes the single in-flight task.
func runResume(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: sop resume [task-id]")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}

	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "resume: %v\n", err)
		return exitError
	}
	defer st.Close()

	resources, err := d.newResources(dir)
	if err != nil {
		fmt.Fprintf(stderr, "resume: %v\n", err)
		return exitError
	}

	// A crashed run under execution.lease: required leaves its lease behind. Resume
	// reports the holder and releases it only when it is stale; a live execution is
	// never released.
	if err := reportStaleExecutionLease(dir, stderr); err != nil {
		fmt.Fprintf(stderr, "resume: %v\n", err)
		return exitError
	}

	resumer := resume.New(st, resources)
	var decision resume.Decision
	if len(args) == 1 {
		decision, err = resumer.Resolve(context.Background(), args[0])
	} else {
		decision, err = resumer.ResolveActive(context.Background())
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	if decision.TaskID == "" {
		fmt.Fprintln(stdout, "nothing to resume")
		return exitOK
	}
	if decision.Recovered {
		fmt.Fprintf(stdout, "%s %s recovered=%s\n", decision.TaskID, decision.Action, decision.Status)
		return exitOK
	}
	fmt.Fprintf(stdout, "%s %s\n", decision.TaskID, decision.Action)
	return exitOK
}

// reportStaleExecutionLease reports the project's execution lease and releases it
// when its owner is gone. It runs whatever execution.lease is set to: a stale lock is
// stale either way, and leaving it would refuse every later start after a crash.
//
// A lease that is not stale is only reported, so resume can never release an
// execution that is still running. ErrLeaseChanged and ErrLeaseNotStale mean a
// recovery or a new execution moved first; that is reported and resume carries on.
func reportStaleExecutionLease(dir string, stderr io.Writer) error {
	state, err := executionlease.Read(dir)
	if err != nil || !state.Present {
		return err
	}

	// The inspection and the release use the same clock inputs, so a lease can only
	// become less stale in between; ReleaseStale re-checks under the claim.
	stale := state.Stale(time.Now(), executionlease.Hostname(), executionlease.Grace, executionlease.HeartbeatLimit)
	if !stale {
		if state.Unparsable {
			fmt.Fprintln(stderr, "resume: execution lease: a lease record is being written")
		} else {
			fmt.Fprintf(stderr, "resume: execution lease held by %s\n", state.Owner.Describe())
		}
		return nil
	}

	expect := state.Owner
	holder := state.Owner.Describe()
	if state.Unparsable {
		expect = executionlease.Owner{}
		holder = "an unreadable lease record"
	}
	fmt.Fprintf(stderr, "resume: execution lease is stale, releasing it: %s\n", holder)

	err = executionlease.ReleaseStale(dir, expect)
	if err == nil {
		return nil
	}
	if errors.Is(err, executionlease.ErrLeaseChanged) || errors.Is(err, executionlease.ErrLeaseNotStale) {
		// Someone else released or replaced it first; that is not a failure.
		fmt.Fprintf(stderr, "resume: execution lease: %v\n", err)
		return nil
	}
	return err
}

// branchLocator reports whether a branch exists locally (implemented by the Git
// adapter).
type branchLocator interface {
	BranchExists(ctx context.Context, branch string) (bool, error)
}

// pullRequestFinder returns the pull request for a head branch (implemented by
// the GitHub adapter).
type pullRequestFinder interface {
	GetPullRequest(ctx context.Context, head string) (github.PullRequest, error)
}

// gitHubResources observes a task's branch and pull request through the Git and
// GitHub adapters. A missing pull request is reported as nil so a REVIEW_PASS
// task without one is treated as needing PR creation.
type gitHubResources struct {
	branches branchLocator
	prs      pullRequestFinder
}

// Observe returns the task's branch existence and pull request.
func (r *gitHubResources) Observe(ctx context.Context, task *domain.Task) (resume.Observation, error) {
	branch, err := git.TaskBranchName(task.ID, task.Title)
	if err != nil {
		return resume.Observation{}, err
	}
	exists, err := r.branches.BranchExists(ctx, branch)
	if err != nil {
		return resume.Observation{}, err
	}
	var pr *github.PullRequest
	if got, err := r.prs.GetPullRequest(ctx, branch); err == nil {
		pr = &got
	}
	return resume.Observation{BranchExists: exists, PR: pr}, nil
}
