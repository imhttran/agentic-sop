package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// continuationVerdict classifies a continuation's bindings against live state. The three
// outcomes are deliberately distinct: only a provably stale obligation may be left in place
// on a completed task, and an unprovable binding fails the completion closed.
type continuationVerdict int

const (
	// continuationDischargeable: every binding matches live state.
	continuationDischargeable continuationVerdict = iota
	// continuationStale: the record provably belongs to another task, plan revision, or run
	// epoch, so it can never be resumed against this run and is not actionable.
	continuationStale
	// continuationAmbiguous: the bindings cannot be proven either way, so the obligation
	// might still be actionable. It must not be discharged, and the completion must not be
	// recorded while it remains.
	continuationAmbiguous
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
// The contract it enforces:
//
//   - a continuation is discharged ONLY when every binding matches live state;
//   - a provably stale continuation (another task, another plan revision, another run
//     epoch) is reported and left in place, because it can never be resumed against this
//     run and is therefore not actionable;
//   - an ambiguous binding (missing question, missing plan or epoch proof, or a live worker
//     holding the continuation) fails the completion closed, because leaving a possibly
//     actionable obligation behind would break the invariant below;
//   - each discharge records EXTERNALLY_COMPLETED — never COMPLETED — because no stage ran.
//
// INVARIANT: a recorded external completion leaves no actionable continuation. Any error
// return means the task was NOT completed, so the operator can retry.
//
// RECOVERY: discharge is idempotent and each continuation is independent. If a run
// discharges some continuations and then fails, a retry re-reads the task, skips the
// already-terminal records untouched, discharges the rest, and records the completion.
// Ordering is deliberate — discharge happens BEFORE the completion is recorded — so a
// crash in the gap leaves the task incomplete and retryable rather than complete with owed
// work.
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

	var discharged int
	for _, c := range conts {
		if strings.TrimSpace(c.TaskID) != task.ID {
			continue // another task's obligation: never touched
		}
		if c.State.Terminal() {
			continue // already terminal: history preserved, never re-discharged
		}

		verdict, reason := externalContinuationVerdict(svc, task, c, planRevision, epoch)
		switch verdict {
		case continuationStale:
			fmt.Fprintf(stdout, "continuation %s: not discharged (%s; not actionable against this run)\n", c.ID, reason)
			continue
		case continuationAmbiguous:
			return fmt.Errorf("continuation %s has an ambiguous binding: %s; refusing to record completion with possible owed work", c.ID, reason)
		}

		// Execution ownership first, exactly as the planner resume path does: a worker that
		// still holds the continuation must not have its obligation terminalized underneath
		// it, and completing the task underneath it would strand that work.
		release, acquired, lerr := question.TryAcquireLock(plannerEffectPath(dir, task.ID, c.ID) + ".lock")
		if lerr != nil {
			return lerr
		}
		if !acquired {
			return fmt.Errorf("continuation %s is held by another worker; refusing to record completion with actionable owed work", c.ID)
		}

		reason = fmt.Sprintf(
			"external completion: implementation commit %s is contained in HEAD; owed stage %q was not executed",
			commit, c.ReturnStage)
		got, changed, derr := svc.DischargeExternalContinuation(c.ID, reason)
		release()
		if derr != nil {
			return derr
		}
		if !changed {
			// Another actor terminalized it while we held the lock: the obligation is no
			// longer actionable, so completion may proceed on the observed state.
			state := "unknown"
			if got != nil {
				state = string(got.State)
			}
			fmt.Fprintf(stdout, "continuation %s: already terminal (%s); nothing to discharge\n", c.ID, state)
			continue
		}
		discharged++
		fmt.Fprintf(stdout, "continuation %s: EXTERNALLY_COMPLETED (owed stage %q not executed)\n", c.ID, c.ReturnStage)
	}
	if discharged > 0 {
		fmt.Fprintf(stdout, "continuations: %d discharged\n", discharged)
	}
	return nil
}

// externalContinuationVerdict classifies c's bindings against live state. It never mutates.
//
// Binding rules:
//
//   - task: the bound question must name this task. A different named task is stale; a
//     missing one is ambiguous.
//   - plan: the question's plan revision must be present on both sides and equal. A
//     disagreement is stale; a missing side is ambiguous, because an unprovable plan
//     binding cannot establish which plan the owed work belongs to.
//   - run epoch: a continuation and its question must not disagree. When BOTH are empty the
//     record predates epoch binding (the legacy case, e.g. `cont:q-1:`); the plan binding
//     above remains the proof and the record is accepted. When either side carries an epoch,
//     it must equal the live run epoch; a missing live epoch is ambiguous.
func externalContinuationVerdict(svc *question.Service, task *domain.Task, c *question.Continuation, livePlan, liveEpoch string) (continuationVerdict, string) {
	q, _, err := svc.ReadQuestion(c.QuestionID)
	if err != nil || q == nil {
		return continuationAmbiguous, "bound question is not readable"
	}

	qTask := strings.TrimSpace(q.Identity)
	switch {
	case qTask == "":
		return continuationAmbiguous, "question has no task binding"
	case qTask != task.ID:
		return continuationStale, fmt.Sprintf("question is bound to task %q, not %q", qTask, task.ID)
	}

	qPlan := strings.TrimSpace(q.PlanRevision)
	livePlan = strings.TrimSpace(livePlan)
	switch {
	case qPlan == "" || livePlan == "":
		return continuationAmbiguous, "plan revision binding is missing"
	case qPlan != livePlan:
		return continuationStale, fmt.Sprintf("plan revision changed: bound %s, live %s", qPlan, livePlan)
	}

	cEpoch := strings.TrimSpace(c.RunEpoch)
	qEpoch := strings.TrimSpace(q.RunEpoch)
	if cEpoch == "" && qEpoch == "" {
		return continuationDischargeable, "" // legacy: neither side recorded a run epoch
	}
	if cEpoch != "" && qEpoch != "" && cEpoch != qEpoch {
		return continuationStale, fmt.Sprintf("continuation run epoch %s disagrees with question run epoch %s", cEpoch, qEpoch)
	}
	bound := cEpoch
	if bound == "" {
		bound = qEpoch
	}
	liveEpoch = strings.TrimSpace(liveEpoch)
	if liveEpoch == "" {
		return continuationAmbiguous, "run epoch binding is missing"
	}
	if bound != liveEpoch {
		return continuationStale, fmt.Sprintf("run epoch changed: bound %s, live %s", bound, liveEpoch)
	}
	return continuationDischargeable, ""
}
