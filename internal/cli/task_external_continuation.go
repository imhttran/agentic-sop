package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// reconcileExternalContinuations discharges the planner continuations a task still owes
// after an external completion proved its work already exists in shared history.
//
// The gap it closes: applying a planner answer creates a durable continuation whose owed
// work is the saved stage (usually IMPLEMENTING). When the implementation was merged
// outside the run loop, that obligation can never be satisfied — `sop run` would replay
// work that is already in history — so the task could not be completed without leaving an
// actionable continuation behind.
//
// It is deliberately conservative. A continuation is discharged only when EVERY binding
// still matches the authoritative state:
//
//   - the continuation belongs to this task;
//   - its question is readable and bound to this task;
//   - the question's plan revision matches the live plan revision;
//   - the continuation's run epoch matches the live run epoch.
//
// Anything stale, mismatched, already terminal, or held by another worker is reported and
// left untouched, so a bad binding can never terminalize owed work. Each discharge records
// EXTERNALLY_COMPLETED — never COMPLETED — because no stage ran: this records an accurate
// disposition, not execution.
//
// A store failure is returned so the caller can abort before recording completion; that
// keeps the invariant "an external completion never leaves an actionable continuation".
func reconcileExternalContinuations(dir string, task *domain.Task, commit string, stdout, stderr io.Writer) error {
	svc, _, err := plannerService(dir)
	if err != nil {
		return err
	}
	conts, err := svc.Continuations("")
	if err != nil {
		return err
	}
	planRevision := currentPlanRevision(dir)
	epoch := currentRunEpoch(dir, task.ID)

	var discharged, skipped int
	for _, c := range conts {
		if strings.TrimSpace(c.TaskID) != task.ID {
			continue // another task's obligation: never touched
		}
		if c.State.Terminal() {
			continue // already terminal: history preserved, never re-discharged
		}
		if reason, ok := externalContinuationBlocked(svc, task, c, planRevision, epoch); !ok {
			fmt.Fprintf(stdout, "continuation %s: not discharged (%s)\n", c.ID, reason)
			skipped++
			continue
		}

		// Execution ownership first, exactly as the planner resume path does: a worker
		// that still holds the continuation must not have its obligation terminalized
		// underneath it. A lock we cannot take means stand down and leave the record.
		release, acquired, lerr := question.TryAcquireLock(plannerEffectPath(dir, task.ID, c.ID) + ".lock")
		if lerr != nil {
			return lerr
		}
		if !acquired {
			fmt.Fprintf(stdout, "continuation %s: not discharged (execution held by another worker)\n", c.ID)
			skipped++
			continue
		}

		reason := fmt.Sprintf(
			"external completion: implementation commit %s is contained in HEAD; owed stage %q was not executed",
			commit, c.ReturnStage)
		got, changed, derr := svc.DischargeExternalContinuation(c.ID, reason)
		release()
		if derr != nil {
			return derr
		}
		if !changed {
			// Lost a race to another transition; report the observed state, do not retry.
			state := "unknown"
			if got != nil {
				state = string(got.State)
			}
			fmt.Fprintf(stdout, "continuation %s: not discharged (state is %s)\n", c.ID, state)
			skipped++
			continue
		}
		discharged++
		fmt.Fprintf(stdout, "continuation %s: EXTERNALLY_COMPLETED (owed stage %q not executed)\n", c.ID, c.ReturnStage)
	}
	if discharged+skipped > 0 {
		fmt.Fprintf(stdout, "continuations: %d discharged, %d skipped\n", discharged, skipped)
	}
	return nil
}

// externalContinuationBlocked reports whether c must be left alone, with the reason. It is
// the binding gate: it re-reads the bound question and compares the plan revision and run
// epoch against live state, using the same non-empty comparison the reconciler applies, so
// an unset (legacy) binding is treated as compatible rather than as a mismatch.
func externalContinuationBlocked(svc *question.Service, task *domain.Task, c *question.Continuation, planRevision, epoch string) (string, bool) {
	q, _, err := svc.ReadQuestion(c.QuestionID)
	if err != nil || q == nil {
		return "bound question is not readable", false
	}
	if id := strings.TrimSpace(q.Identity); id != "" && id != task.ID {
		return fmt.Sprintf("question is bound to task %q, not %q", id, task.ID), false
	}
	if b := strings.TrimSpace(q.PlanRevision); b != "" && planRevision != "" && b != planRevision {
		return fmt.Sprintf("plan revision changed: continuation bound %s, live %s", b, planRevision), false
	}
	if b := strings.TrimSpace(c.RunEpoch); b != "" && epoch != "" && b != epoch {
		return fmt.Sprintf("run epoch changed: continuation bound %s, live %s", b, epoch), false
	}
	return "", true
}
