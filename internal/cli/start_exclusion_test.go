package cli

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/acp"
	"github.com/imhttran/agentic-sop/internal/mcp"
)

// This file covers proposal test 3 (docs/plans/PROPOSAL-SHARED-EXECUTION-
// EXCLUSION.md §6): an ACP `session/prompt` and an MCP `sop_start_work` start
// racing on one project must create at most one work item.
//
// Both interfaces are the production ones: `acpSubmission` is the bridge
// `sop acp` installs and `mcpStartWorkTool` is the tool `sop mcp` exposes, and
// both call `startWork`, so the O_EXCL start lock and the under-lock re-read of
// the job records are exercised for real. Only the launcher is faked, so no
// child process, model, or network is involved; the job bookkeeping, locking,
// pid liveness, and fingerprinting are the production implementations.
//
// The invariant holds for every interleaving: whichever racer takes the start
// lock launches once, and the other either fails to take the lock or observes
// the running job under it. One race per case is therefore enough.
//
// The execution lease plays no part here: neither interface acquires or checks
// it (execution.lease defaults to off and only `sop run`/`sop resume` reference
// internal/executionlease). This file proves start-lock exclusion, not execution
// exclusion — proposal tests 1, 2, 4 and 6 remain open, and the tests below do
// not establish cross-entrypoint execution exclusion on their own.

// racerResult is one racer's outcome, normalized across the two interfaces.
type racerResult struct {
	outcome string
	jobID   string
	err     error
}

// raceTwo releases two racers together and returns their results in order.
func raceTwo(t *testing.T, a, b func() racerResult) []racerResult {
	t.Helper()
	results := make([]racerResult, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() { defer wg.Done(); <-start; results[0] = a() }()
	go func() { defer wg.Done(); <-start; results[1] = b() }()
	close(start)
	wg.Wait()
	return results
}

// callMCPStart invokes sop_start_work through its real tool handler.
func callMCPStart(t *testing.T, tool mcp.Tool, args []byte) racerResult {
	t.Helper()
	out, err := tool.Handler(context.Background(), args)
	if err != nil {
		return racerResult{err: err}
	}
	var doc startDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Errorf("decode sop_start_work result: %v", err)
		return racerResult{}
	}
	r := racerResult{outcome: doc.Outcome}
	if doc.Job != nil {
		r.jobID = doc.Job.ID
	}
	return r
}

// startLaunchCount reports how many governed launches the fake recorded.
func startLaunchCount(f *fakeWork) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.launches)
}

// isStartLockRefusal reports whether err is startWork losing the start lock.
// Any other error is a real failure and must not be masked as a safe outcome.
func isStartLockRefusal(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "another start request is in progress") ||
		strings.Contains(msg, "could not take the start lock")
}

// TestACPPromptAndMCPStartWorkRaceCreateOneWorkItem races one ACP submission and
// one MCP sop_start_work call for the same work. Exactly one may launch.
func TestACPPromptAndMCPStartWorkRaceCreateOneWorkItem(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	f := newFakeWork()
	env := f.env()
	bridge := acpSubmission(env)
	tool := mcpStartWorkTool(dir, env)

	const text = "concurrent submission across interfaces"
	fp := acp.PromptFingerprint("plan", text)
	sub := acp.Submission{
		ProjectDir:    dir,
		Capability:    "plan",
		Text:          text,
		Fingerprint:   fp,
		SubmissionKey: fp,
	}
	raw, err := json.Marshal(map[string]any{"prompt": text, "capability": "plan"})
	if err != nil {
		t.Fatalf("marshal sop_start_work args: %v", err)
	}

	results := raceTwo(t,
		func() racerResult {
			res, err := bridge.Starter(sub)
			return racerResult{outcome: res.Outcome, jobID: res.JobID, err: err}
		},
		func() racerResult { return callMCPStart(t, tool, raw) },
	)

	if got := startLaunchCount(f); got != 1 {
		t.Fatalf("governed launches = %d, want 1: an ACP and an MCP start must not create a second work item", got)
	}
	started := 0
	for i, r := range results {
		if r.err != nil {
			if !isStartLockRefusal(r.err) {
				t.Errorf("racer %d error = %v, want a start-lock refusal or a non-starting disposition", i, r.err)
			}
			continue
		}
		switch r.outcome {
		case acp.WorkOutcomeStarted:
			started++
			if strings.TrimSpace(r.jobID) == "" {
				t.Errorf("racer %d reported started without a job id", i)
			}
		case acp.WorkOutcomeAlreadyRunning, acp.WorkOutcomeReconciled,
			acp.WorkOutcomeBusy, acp.WorkOutcomeTaskInProgress:
		default:
			t.Errorf("racer %d outcome = %q, want started or a non-starting disposition", i, r.outcome)
		}
	}
	if started != 1 {
		t.Errorf("started outcomes = %d, want exactly 1 (results=%+v)", started, results)
	}
}

// TestACPPromptAndMCPStartWorkRaceDifferentWorkStillOneLaunch races two
// different pieces of work. The loser must be refused as busy: the start lock is
// the only thing keeping two executions apart, and it covers both interfaces.
func TestACPPromptAndMCPStartWorkRaceDifferentWorkStillOneLaunch(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	f := newFakeWork()
	env := f.env()
	bridge := acpSubmission(env)
	tool := mcpStartWorkTool(dir, env)

	const (
		acpText = "explain the scheduler"
		mcpText = "review the store layer"
	)
	fp := acp.PromptFingerprint("plan", acpText)
	sub := acp.Submission{
		ProjectDir:    dir,
		Capability:    "plan",
		Text:          acpText,
		Fingerprint:   fp,
		SubmissionKey: fp,
	}
	raw, err := json.Marshal(map[string]any{"prompt": mcpText, "capability": "plan"})
	if err != nil {
		t.Fatalf("marshal sop_start_work args: %v", err)
	}

	results := raceTwo(t,
		func() racerResult {
			res, err := bridge.Starter(sub)
			return racerResult{outcome: res.Outcome, jobID: res.JobID, err: err}
		},
		func() racerResult { return callMCPStart(t, tool, raw) },
	)

	if got := startLaunchCount(f); got != 1 {
		t.Fatalf("governed launches = %d, want 1: one project runs one started job", got)
	}
	started := 0
	for i, r := range results {
		if r.err != nil {
			if !isStartLockRefusal(r.err) {
				t.Errorf("racer %d error = %v, want a start-lock refusal or busy", i, r.err)
			}
			continue
		}
		switch r.outcome {
		case acp.WorkOutcomeStarted:
			started++
		case acp.WorkOutcomeBusy:
		default:
			t.Errorf("racer %d outcome = %q, want started or %q", i, r.outcome, acp.WorkOutcomeBusy)
		}
	}
	if started != 1 {
		t.Errorf("started outcomes = %d, want exactly 1 (results=%+v)", started, results)
	}
}

// TestMCPAndACPStartsExcludeEachOtherInSequence proves the same exclusion in both
// directions without relying on scheduling. Both sides go through their real
// Starter callable and the real startWork, so the under-lock re-read of the
// persisted job records is what refuses the second start. The ACP adapter's own
// reconcile-before-start pass over those same records is covered separately by
// TestACPPromptSubmissionCLIIntegration; this test deliberately does not drive a
// Serve loop, because a live fake child plus disconnect-cancel reconciliation
// would make shutdown timing part of the assertion.
func TestMCPAndACPStartsExcludeEachOtherInSequence(t *testing.T) {
	const text = "sequential cross-interface submission"
	fp := acp.PromptFingerprint("plan", text)
	sub := acp.Submission{
		ProjectDir:    "",
		Capability:    "plan",
		Text:          text,
		Fingerprint:   fp,
		SubmissionKey: fp,
	}

	t.Run("mcp first, acp second", func(t *testing.T) {
		dir := t.TempDir()
		initProject(t, dir)
		f := newFakeWork()
		bridge := acpSubmission(f.env())

		doc, err := callStart(t, dir, f, map[string]any{"prompt": text, "capability": "plan"})
		if err != nil {
			t.Fatalf("sop_start_work: %v", err)
		}
		if doc.Outcome != workOutcomeStarted {
			t.Fatalf("first start outcome = %q, want %q", doc.Outcome, workOutcomeStarted)
		}

		sub.ProjectDir = dir
		res, err := bridge.Starter(sub)
		if err != nil {
			t.Fatalf("ACP start: %v", err)
		}
		if res.Outcome != acp.WorkOutcomeAlreadyRunning {
			t.Errorf("ACP outcome = %q, want %q", res.Outcome, acp.WorkOutcomeAlreadyRunning)
		}
		if got := startLaunchCount(f); got != 1 {
			t.Errorf("governed launches = %d, want 1", got)
		}
	})

	t.Run("acp first, mcp second", func(t *testing.T) {
		dir := t.TempDir()
		initProject(t, dir)
		f := newFakeWork()
		bridge := acpSubmission(f.env())

		sub.ProjectDir = dir
		res, err := bridge.Starter(sub)
		if err != nil {
			t.Fatalf("ACP start: %v", err)
		}
		if res.Outcome != acp.WorkOutcomeStarted {
			t.Fatalf("ACP outcome = %q, want %q", res.Outcome, acp.WorkOutcomeStarted)
		}

		doc, err := callStart(t, dir, f, map[string]any{"prompt": text, "capability": "plan"})
		if err != nil {
			t.Fatalf("sop_start_work: %v", err)
		}
		if doc.Outcome != workOutcomeAlreadyRunning {
			t.Errorf("MCP outcome = %q, want %q", doc.Outcome, workOutcomeAlreadyRunning)
		}
		if got := startLaunchCount(f); got != 1 {
			t.Errorf("governed launches = %d, want 1", got)
		}
	})
}
