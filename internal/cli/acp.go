package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/acp"
	"github.com/imhttran/agentic-sop/internal/projectlock"
)

// acpUsage is the one-line usage for `sop acp`.
const acpUsage = "usage: sop acp"

// acpBusyProbeTimeout bounds how long a session/prompt waits for the shared
// project writer lock before answering with a busy result. The adapter never
// queues conflicting work, so a held lock must fail fast instead of stalling
// the editor; a caller that wants to wait retries after the holder finishes.
// It is a variable so tests can shorten it.
var acpBusyProbeTimeout = 250 * time.Millisecond

// runACP serves the Agent Client Protocol over stdio so a protocol client can
// complete ACP initialization and session creation against SOP. Requests are
// read from stdin (d.stdin in tests) and responses written to stdout through
// the acp package's protocol-only writer; diagnostics go to stderr with the
// acp package's credential redaction. Serving ends at EOF, which is exit 0.
// No flags exist yet; an unknown flag is a usage error.
//
// The server is built with the governed prompt-submission bridge, so
// session/prompt reaches the same machinery `sop prompt` and MCP
// sop_start_work use. Without it the method is refused rather than silently
// unimplemented.
//
// The server is also built with the progress seam (acpProgress), so
// prompt-started work streams session/update frames to the client from the
// project's persisted started-work job records. The seam emits nothing when
// no record exists and never starts or changes workflow execution.
func runACP(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, acpUsage)
		return exitUsage
	}
	in := io.Reader(os.Stdin)
	if d.stdin != nil {
		in = d.stdin
	}
	env := defaultWorkEnv()
	if d.acpWorkEnv != nil {
		env = d.acpWorkEnv()
	}
	server := acp.NewServer(Version, stdout, stderr,
		acp.WithPromptSubmission(acpSubmission(env)),
		acp.WithProgress(acpProgress(env)),
	)
	if err := server.Serve(context.Background(), in); err != nil {
		fmt.Fprintf(stderr, "acp: %v\n", err)
		return exitError
	}
	return exitOK
}

// acpWorkOutcomeNotInitialized is the start-work outcome for a project that has
// no SOP state yet. mcp_start_work reports it as a literal string rather than a
// named constant, so it is named here for the ACP mapping.
const acpWorkOutcomeNotInitialized = "not_initialized"

// acpSubmission wires the ACP adapter's governed prompt-submission bridge to the
// machinery SOP already runs for `sop prompt` and MCP sop_start_work:
//
//   - LockTaker PROBES the shared project writer lock through
//     internal/projectlock with a short bound, then releases it immediately. The
//     probe answers only "is a plan or approval writer mutating this project right
//     now?", and a contended probe maps to acp.ErrProjectBusy so the adapter can
//     return a useful busy result instead of queueing.
//
//     The probe is NOT the execution-exclusion mechanism. The project writer lock
//     is taken by planflow and approval mutations; neither `sop prompt` nor
//     startWork takes it, and holding it across Starter would exclude nothing that
//     survives the launch. Two execution paths are kept apart by startWork's own
//     cross-process start lock (see Starter below), which is why the probe is
//     released before the state read and the start.
//
//   - StateReader exposes the project's persisted started-work records, newest
//     first, so the adapter reconciles before any new submission.
//
//   - Starter performs exactly one governed start through startWork. startWork
//     owns the cross-process start lock (`.agent-sdlc/runs/work/.lock`, created
//     O_EXCL): every ACP and MCP start serializes on it, re-reads the persisted
//     jobs while holding it, and refuses a duplicate or a competing running job.
//     It also refuses while a `sop run` is mid-task, detected from the persisted
//     task states. That start lock — not the project writer lock — is what makes
//     concurrent ACP/MCP starts create at most one work item.
//
// The bridge carries no completion semantics: it starts work and reports the
// identity SOP recorded. It never approves, satisfies, or completes a task.
func acpSubmission(env workEnv) *acp.PromptSubmission {
	return &acp.PromptSubmission{
		LockTaker: func(ctx context.Context, projectDir string) (func(), error) {
			lock, err := projectlock.AcquireWait(ctx, projectDir, acpBusyProbeTimeout)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					return nil, acp.ErrProjectBusy
				}
				return nil, err
			}
			// Release at once: this was a probe, not the start lock. startWork takes
			// `.agent-sdlc/runs/work/.lock` (O_EXCL) for the authoritative
			// duplicate/competing-job check, and that lock is what serializes starts.
			lock.Release()
			return func() {}, nil
		},
		StateReader: func(projectDir string) ([]acp.PersistedWork, error) {
			jobs, err := listWorkJobs(projectDir, env)
			if err != nil {
				return nil, err
			}
			out := make([]acp.PersistedWork, 0, len(jobs))
			for _, j := range jobs {
				out = append(out, acp.PersistedWork{
					JobID:       j.ID,
					Fingerprint: j.Fingerprint,
					State:       j.State,
					ExitCode:    j.ExitCode,
				})
			}
			return out, nil
		},
		Starter: func(sub acp.Submission) (acp.SubmissionResult, error) {
			doc, err := startWork(sub.ProjectDir, env, startWorkArgs{
				Prompt:     sub.Text,
				Capability: sub.Capability,
			})
			if err != nil {
				return acp.SubmissionResult{}, err
			}
			return acpStartWorkResult(doc)
		},
		Cancel: func(ctx context.Context, projectDir, jobID string) error {
			// Stops exactly the named started job's child and records the
			// cancellation in that job's own record. It never signals anything the
			// adapter did not start, and it honors the caller's CancelStopBound.
			return cancelWorkJob(ctx, projectDir, jobID, env)
		},
	}
}

// acpProgress is the ACP progress seam supplied through acp.WithProgress (ACP-004).
// It reads the project's persisted started-work job records through the same
// listWorkJobs read path the submission bridge's StateReader uses, and maps one
// record onto acp.ProgressSnapshot.
//
// The record's fields are the only persisted evidence this source claims: job
// id, derived state, and exit code. Task/attempt identity, the current stage,
// the verification outcome, blockers, and final evidence references have no
// persisted read path traced here, so they stay zero and the ACP translator
// reports them as unavailable instead of inventing them.
//
// A job id with no record reports found=false: no record, no frame, no invented
// state. A read error propagates; the ACP side records a diagnostic and emits
// no frame, and the prompt response itself is unaffected.
func acpProgress(env workEnv) *acp.ProgressSource {
	return &acp.ProgressSource{
		Progress: func(projectDir, jobID string) (acp.ProgressSnapshot, bool, error) {
			jobs, err := listWorkJobs(projectDir, env)
			if err != nil {
				return acp.ProgressSnapshot{}, false, err
			}
			for _, j := range jobs {
				if j.ID != jobID {
					continue
				}
				return acp.ProgressSnapshot{
					JobID:    j.ID,
					State:    mapWorkState(j.State),
					ExitCode: j.ExitCode,
				}, true, nil
			}
			return acp.ProgressSnapshot{}, false, nil
		},
	}
}

// mapWorkState maps a persisted started-work job record's derived state onto
// the ACP progress payload's SOP-state values. The record set is the closed
// one prompt.go pins (running, completed, failed, ended); any other spelling
// falls to ended, the ambiguous case, so the frame never invents a state the
// record does not carry. The derived states never assert verification: a
// completed record is not claimed as verified completion.
func mapWorkState(state string) string {
	switch state {
	case "running":
		return acp.ProgressStateRunning
	case "completed":
		return acp.ProgressStateCompleted
	case "failed":
		return acp.ProgressStateFailed
	case "ended":
		return acp.ProgressStateEnded
	default:
		return acp.ProgressStateEnded
	}
}

// acpStartWorkResult maps one sop_start_work outcome document onto the ACP
// submission result. The mapping is deliberate and total: an outcome the
// adapter does not understand is an error rather than a silently empty result.
//
//	started          -> started (one new work item, its job identity attached)
//	already_running  -> already_running
//	busy             -> busy (not started, not queued)
//	task_in_progress -> task_in_progress (a live run holds the project)
//	plan_blocked     -> busy (a plan reconciliation gate would stop the run)
//	already_ran      -> reconciled (a persisted prior attempt owns this work)
//	in_memory        -> reconciled (SOP already holds this plan)
//	not_initialized  -> not_initialized
func acpStartWorkResult(doc map[string]any) (acp.SubmissionResult, error) {
	outcome, _ := doc["outcome"].(string)
	message, _ := doc["message"].(string)
	res := acp.SubmissionResult{Message: message}
	if view, ok := doc["job"].(workJobView); ok {
		res.JobID = view.ID
		res.State = view.State
		res.ExitCode = view.ExitCode
	}
	switch outcome {
	case workOutcomeStarted:
		res.Outcome = acp.WorkOutcomeStarted
	case workOutcomeAlreadyRunning:
		res.Outcome = acp.WorkOutcomeAlreadyRunning
	case workOutcomeBusy, workOutcomePlanBlocked:
		res.Outcome = acp.WorkOutcomeBusy
	case workOutcomeTaskInProgress:
		res.Outcome = acp.WorkOutcomeTaskInProgress
	case workOutcomeAlreadyRan, workOutcomeInMemory:
		res.Outcome = acp.WorkOutcomeReconciled
	case acpWorkOutcomeNotInitialized:
		res.Outcome = acp.WorkOutcomeNotInitialized
	default:
		return acp.SubmissionResult{}, fmt.Errorf("start governed prompt: unexpected outcome %q", strings.TrimSpace(outcome))
	}
	return res, nil
}
