# PLAN — Automatic Blocked-Task Recovery

## Project

agentic-sop

## Summary

Make `sop run` the normal recovery command for completed-plan handoff and safe, bounded recovery of historical blocked tasks while preserving dependency ordering, human gates, working-tree safety, and persisted history.

## Objective

Make `sop run` the normal recovery command for a partially completed plan.

A user should not need to inspect SOP's internal task-state machinery or manually run `sop retry <task>` for historical blocked tasks that are now safe to re-evaluate.

The desired behavior is:

```text
sop run
   |
   +-- DONE / LOCAL_DONE -> skip
   |
   +-- PLANNED / READY -> execute when dependencies are satisfied
   |                      |
   |                      +-- PASS -> continue
   |                      +-- FAIL -> stop
   |
   +-- BLOCKED -> one bounded automatic recovery attempt
   |              |
   |              +-- PASS -> continue
   |              +-- FAIL -> stop
   |
   +-- NEEDS_HUMAN -> stop
   |
   +-- no unresolved work -> PLAN COMPLETE
```

This behavior must preserve dependency ordering, human approval gates, working-tree safety, persisted run history, and the existing explicit `sop retry <task>` workflow.

---

## Product Requirements

1. `sop run` skips tasks already in `DONE` or `LOCAL_DONE`.
2. Normal runnable `PLANNED` or `READY` work remains the first execution path.
3. When no normal runnable task exists, SOP may select the earliest recoverable `BLOCKED` task whose dependencies are satisfied.
4. A blocked task receives at most one automatic recovery attempt during a single `sop run` invocation.
5. If automatic recovery succeeds, SOP continues through the plan in the same invocation.
6. If automatic recovery fails, SOP stops immediately and does not execute later tasks.
7. `NEEDS_HUMAN` is never automatically bypassed or retried as ordinary blocked work.
8. Dependency constraints remain authoritative. A blocked task with unsatisfied dependencies is not recoverable yet.
9. Existing explicit `sop retry <task>` behavior remains supported.
10. Automatic recovery must not reset or rewrite SOP state, delete run history, or destructively alter the user's working tree.
11. When all tasks are satisfied, SOP reports plan completion rather than `no runnable task (BLOCKED)`.
12. A fully satisfied active plan must not prevent an explicitly requested different plan from starting.
13. Completed-plan handoff must preserve prior plan history and provenance and must not require deleting `.agent-sdlc/state.db`.
14. An unfinished active plan must continue to block automatic switching to a different plan.

---

## ABR000 — Verify Completed Active-Plan Handoff

ABR000 verifies the already-implemented completed active-plan handoff and its regression coverage. The capability now lives in `internal/planflow` (`reconcileState` → `handOff`) and has already been dogfooded: running `sop run docs/plans/PLAN-Automatic-Blocked-Task-Recovery.md` while the fully satisfied Pre-JEV plan was active handed off without deleting `.agent-sdlc/state.db`, validated this plan, and created its tasks. That event is evidence, not work ABR000 must reproduce, and this task must not re-implement the capability.

### Execution

- verify-first

### Already-implemented behavior

An active plan whose tasks are all in satisfied terminal states (`DONE`, `LOCAL_DONE`, `MERGED`) no longer locks out a different, explicitly requested plan. When a different plan is requested, SOP:

1. deterministically verifies that the active plan is complete,
2. preserves its task state, reports, metrics, run history, and provenance,
3. archives the completed record under `.agent-sdlc/archive/<plan-id>/`,
4. releases only the active-plan association needed to permit the next plan,
5. initializes the explicitly requested new plan using normal plan-loading rules, and
6. continues in the same invocation without requiring deletion of `.agent-sdlc/state.db`.

An active plan containing unresolved work (`PLANNED`, `READY`, `BLOCKED`, `NEEDS_HUMAN`, or another non-satisfied state) still stops with a focused diagnostic identifying the active and requested plans, and the same-plan request path is unchanged and does not archive.

### Safety invariants

Normal handoff must not require or recommend `rm .agent-sdlc/state.db`, `git reset --hard`, or `git clean`. It must not:

- erase the previous plan's task records, reports, metrics, or provenance,
- rewrite historical run ownership to the new plan,
- discard unrelated working-tree changes,
- mix tasks from two plans into one task graph,
- auto-switch away from an unfinished plan, or
- manufacture completion merely to permit a switch.

### Existing regression coverage

Verify these deterministic tests already prove the required behavior:

- `internal/planflow`: `TestPrepareHandsOffCompletedPlan`, `TestPrepareHandoffAcceptsSatisfiedStates`, `TestPrepareHandoffBlocksUnresolvedWork`, `TestPrepareHandoffArchivesCompletedRecord`, `TestPrepareHandoffSamePlanIsUnchanged`, `TestPrepareHandoffRejectsInvalidNewPlan`.
- `internal/cli`: `TestRunCompletedPlanHandsOff`, `TestRunUnfinishedPlanStops`, `TestRunSamePlanContinuesAfterCompletion`.
- `internal/domain`: `TestTaskIsSatisfied`, `TestAllSatisfied`.
- `internal/store`: `TestClearTasksRemovesGraphAndChildren`.

### Acceptance Criteria

- A completed active plan can hand off to an explicitly requested new plan.
- `DONE` and `LOCAL_DONE` are recognized as satisfied terminal states.
- An unfinished active plan still prevents switching to a different plan.
- Run history and plan provenance are preserved across handoff.
- `.agent-sdlc/state.db` is not deleted or recreated during normal handoff.
- Existing working-tree changes are preserved.
- Task graphs from different plans are never mixed.
- Failure activating a new plan does not corrupt the previous completed plan's historical record.
- Deterministic regression tests cover these behaviors.
- No application code is modified by this task.

---

## TASK001 — Define Task Selection Semantics

Make scheduler behavior explicit for every relevant persisted task state.

### Required behavior

```text
DONE        -> skip
LOCAL_DONE  -> skip
PLANNED     -> execute when dependencies are satisfied
READY       -> execute
BLOCKED     -> eligible for bounded recovery when dependencies are satisfied
NEEDS_HUMAN -> stop and require human action
```

Preserve any additional existing states and their established semantics unless a change is necessary for this feature.

### Acceptance Criteria

- Completed work is never rerun by normal `sop run`.
- A historical `BLOCKED` state does not permanently make an otherwise recoverable plan unrunnable.
- Dependency ordering remains authoritative.
- Human approval and `NEEDS_HUMAN` boundaries remain authoritative.
- No task is skipped merely to reach later work.

---

## TASK002 — Select the Earliest Recoverable Blocked Task

When normal runnable-task discovery finds no task, identify the earliest blocked task that is safe to re-evaluate.

### Required behavior

Given:

```text
T001 LOCAL_DONE
T002 LOCAL_DONE
T003 BLOCKED
T004 LOCAL_DONE
T005 BLOCKED
```

and satisfied dependencies for both blocked tasks, `sop run` selects `T003` first.

Selection must follow deterministic plan ordering and existing dependency rules.

### Acceptance Criteria

- Normal runnable work is selected before recovery work when normal runnable work exists.
- If no normal runnable work exists, the earliest recoverable blocked task is selected.
- A blocked task with unsatisfied dependencies is not selected.
- Later blocked tasks are not selected ahead of an earlier eligible blocked task.
- Selection is deterministic across equivalent runs.

---

## TASK003 — Add Bounded Automatic Recovery

Allow `sop run` to re-evaluate a blocked task without requiring the user to issue `sop retry <task>` manually.

### Requirements

Reuse the existing retry/requeue/recovery machinery. Do not create a second independent recovery state machine.

Recovery-attempt tracking must be invocation-scoped, conceptually equivalent to:

```go
attemptedRecovery map[TaskID]bool
```

A blocked task may receive one automatic recovery attempt per `sop run` invocation.

A later, separately invoked `sop run` may attempt recovery again.

### Acceptance Criteria

- Automatic recovery uses existing legal retry/requeue transitions.
- The same blocked task is not automatically retried repeatedly during one invocation.
- No unbounded retry loop is possible.
- Explicit `sop retry <task>` continues to work independently.
- Recovery state does not leak between separate `sop run` invocations.

---

## TASK004 — Stop on the First Failed Recovery

Preserve correctness by stopping when a blocked task cannot be recovered.

### Required behavior

Do not do this:

```text
T003 recovery -> FAIL
T005 recovery -> attempted anyway
```

Do this:

```text
T003 recovery -> FAIL
                 |
                 +-> STOP
```

### User-facing result

The failure should clearly identify:

- the task that remains blocked,
- that an automatic recovery attempt was made,
- why it failed,
- that later tasks were not executed,
- the relevant report path when available.

Example shape:

```text
T003 remains BLOCKED.
Automatic recovery failed.
No later tasks were executed.
Report: .agent-sdlc/runs/T003/report.md
```

### Acceptance Criteria

- No later task executes after a failed automatic recovery.
- The original failure diagnostic remains available.
- The user receives an actionable report location when one exists.
- Failure does not cause destructive cleanup or state reset.

---

## TASK005 — Continue After Successful Recovery

A successful recovery should allow the same `sop run` invocation to continue through the plan.

### Execution

- verify-first

### Required behavior

```text
BLOCKED task
    |
automatic recovery
    |
   PASS
    |
LOCAL_DONE / DONE
    |
continue scheduler
```

The scheduler must then skip already completed work and either execute the next normal runnable task or recover the next eligible blocked task.

### Acceptance Criteria

- A successful recovered task transitions through the same persisted state machinery as an explicit successful retry.
- SOP continues automatically after recovery success.
- Already completed tasks encountered afterward are skipped.
- Multiple historical blocked tasks may be recovered sequentially during one `sop run`, provided each succeeds.

---

## TASK006 — Prevent Recovery Loops

Guarantee bounded behavior even when a blocked task repeatedly fails.

### Required behavior

For one invocation:

```text
sop run
  -> T003 BLOCKED
  -> automatic recovery attempt
  -> T003 BLOCKED again
  -> STOP
```

Never:

```text
retry -> retry -> retry -> ...
```

### Acceptance Criteria

- One automatic recovery attempt per blocked task per invocation.
- A repeated block/failure stops the invocation.
- Starting a new `sop run` creates a new bounded recovery opportunity.
- Retry counters and existing retry-budget rules remain respected.

---

## TASK007 — Preserve Recovery and Safety Invariants

Ensure automatic blocked-task recovery does not weaken existing PREJEV016 recovery guarantees.

### Regression coverage must prove automatic recovery does not:

- reset `.agent-sdlc/state.db`,
- delete or rewrite unrelated run history,
- invoke `git reset --hard`,
- invoke `git clean`,
- overwrite unrelated dirty working-tree changes,
- bypass `NEEDS_HUMAN`,
- bypass dependency requirements,
- bypass required human approval,
- manufacture successful state without running required validation,
- retry indefinitely.

### Acceptance Criteria

All existing recovery tests continue to pass, and focused automatic-recovery tests cover these invariants where the new scheduler path could affect them.

---

## TASK008 — Scheduler and End-to-End Regression Coverage

Prove the complete behavior deterministically before dogfooding it against real persisted SOP state.

### Required test cases

1. `DONE` task is skipped.
2. `LOCAL_DONE` task is skipped.
3. `BLOCKED` task with satisfied dependencies is automatically retried when no normal runnable task exists.
4. Successful blocked-task recovery continues execution.
5. Failed blocked-task recovery stops execution.
6. Later tasks do not execute after recovery failure.
7. Multiple blocked tasks recover sequentially when each succeeds.
8. A blocked task receives at most one automatic recovery attempt per invocation.
9. `NEEDS_HUMAN` stops execution and is not automatically recovered.
10. A blocked task with unsatisfied dependencies is not recovered.
11. A fully completed plan reports plan completion.
12. Explicit `sop retry <task>` remains functional.
13. Normal `PLANNED`/`READY` work is preferred over blocked-task recovery when both are legally runnable.

### Representative integration scenario

Initial state:

```text
T001 LOCAL_DONE
T002 LOCAL_DONE
T003 BLOCKED
T004 LOCAL_DONE
T005 BLOCKED
T006 LOCAL_DONE
T007 BLOCKED
T008 LOCAL_DONE
```

Expected successful execution:

```text
skip T001
skip T002
recover T003 -> PASS
skip T004
recover T005 -> PASS
skip T006
recover T007 -> PASS
skip T008
PLAN COMPLETE
```

Expected failure variant:

```text
skip T001
skip T002
recover T003 -> PASS
skip T004
recover T005 -> FAIL
STOP
T006/T007/T008 are not executed or recovered
```

### Acceptance Criteria

- All 13 required scheduler and recovery test cases pass deterministically.
- The successful integration scenario recovers eligible blocked tasks in dependency order and reaches plan completion.
- The failure integration scenario stops immediately at the first failed recovery and does not execute later tasks.
- Completed tasks remain skipped and are never rerun.
- `NEEDS_HUMAN` and unsatisfied-dependency tasks are never automatically recovered.
- Explicit `sop retry <task>` behavior remains compatible with the automatic recovery path.
- The tests do not depend on an LLM, network access, destructive Git operations, or deletion of `.agent-sdlc/state.db`.

---

## CLI / UX Requirements

The normal user workflow should be only:

```bash
sop run
```

SOP should make recovery activity visible without requiring the user to understand internal state transitions.

A successful run may render conceptually as:

```text
Skipping completed tasks...

Recovering PREJEV006 Complete Remaining Harness V2 Tasks
run PREJEV006: PASS

Skipping completed tasks...

Recovering PREJEV013 Align sop-controller Configuration
run PREJEV013: PASS

Recovering PREJEV017 Capture Pre-JEV Performance Baseline
run PREJEV017: PASS

Plan complete.
18/18 tasks completed.
```

Exact wording may follow existing CLI conventions.

Do not require users to manually discover and run:

```bash
sop retry PREJEV006
sop retry PREJEV013
sop retry PREJEV017
```

for the normal recoverable case.

---

## Implementation Guidance

Discover and reuse the existing scheduler, runnable-task selection, retry/requeue, resume, dependency, persistence, and CLI reporting seams.

Do not duplicate orchestration logic solely for automatic recovery.

Prefer a flow conceptually equivalent to:

```go
for {
    if task := nextNormalRunnableTask(); task != nil {
        if err := execute(task); err != nil {
            return err
        }
        continue
    }

    blocked := earliestRecoverableBlockedTask()
    if blocked != nil && !attemptedRecovery[blocked.ID] {
        attemptedRecovery[blocked.ID] = true

        if err := recoverBlockedTask(blocked); err != nil {
            return err
        }

        continue
    }

    return finishPlanState()
}
```

This pseudocode is behavioral guidance, not a required implementation structure.

The implementation must respect the repository's existing state machine and persistence ownership.

---

## Validation

Run formatting on changed Go files, then:

```bash
go test ./internal/cli/...
go test ./internal/resume/...
go test ./...
go vet ./...
go test -race ./...
go build ./...
```

If package ownership differs after repository discovery, use the actual affected package paths while retaining the full-repository validation commands.

---

## Dogfood Acceptance Tests

### A. Completed-plan handoff — current real-world fixture

Use the current persisted Pre-JEV state before clearing or replacing it. At the time this plan was updated, all Pre-JEV tasks were complete:

```text
PREJEV001 LOCAL_DONE
PREJEV002 LOCAL_DONE
PREJEV003 LOCAL_DONE
PREJEV004 LOCAL_DONE
PREJEV005 LOCAL_DONE
PREJEV006 LOCAL_DONE
PREJEV007 LOCAL_DONE
PREJEV008 LOCAL_DONE
PREJEV009 LOCAL_DONE
PREJEV010 LOCAL_DONE
PREJEV011 LOCAL_DONE
PREJEV012 LOCAL_DONE
PREJEV013 LOCAL_DONE
PREJEV014 LOCAL_DONE
PREJEV015 LOCAL_DONE
PREJEV016 LOCAL_DONE
PREJEV017 LOCAL_DONE
PREJEV018 LOCAL_DONE
```

Do **not** delete `.agent-sdlc/state.db` to prepare this test. The persisted completed plan is the fixture.

Run:

```bash
sop run docs/plans/PLAN-Automatic-Blocked-Task-Recovery.md
```

Expected behavior:

```text
active Pre-JEV plan detected
18/18 tasks satisfied
completed plan closed/reconciled without deleting history
PLAN-Automatic-Blocked-Task-Recovery.md activated
new plan executes normally
```

The prior Pre-JEV reports, metrics, task state, and provenance must remain available.

### B. Blocked-task recovery fixture

Create the blocked-task scheduler scenario through deterministic test fixtures or a safe dogfood plan rather than rewriting historical Pre-JEV state. Representative state:

```text
T001 LOCAL_DONE
T002 LOCAL_DONE
T003 BLOCKED
T004 LOCAL_DONE
T005 BLOCKED
T006 LOCAL_DONE
T007 BLOCKED
T008 LOCAL_DONE
```

Expected successful execution:

```text
skip T001-T002
recover T003 -> PASS
skip T004
recover T005 -> PASS
skip T006
recover T007 -> PASS
skip T008
PLAN COMPLETE
```

If any recovered task genuinely fails, SOP must stop at that task and leave later tasks untouched.

---

## Definition of Done

This feature is complete when a user can move from a completed plan to a new explicitly requested plan without deleting state, and when a user can run:

```bash
sop run
```

against a partially completed plan containing historical blocked tasks and SOP will:

- automatically hand off from a fully completed active plan to an explicitly requested new plan,
- preserve completed-plan history and provenance across that handoff,
- refuse automatic plan switching while the active plan still has unresolved work,
- skip completed work,
- preserve dependency ordering,
- execute normal runnable work,
- automatically retry the earliest eligible blocked task,
- make only one automatic recovery attempt per blocked task per invocation,
- continue after successful recovery,
- stop immediately after genuine recovery failure,
- preserve `NEEDS_HUMAN` and human approval boundaries,
- preserve working-tree and state safety,
- retain explicit `sop retry <task>` support,
- avoid infinite retry loops,
- and report plan completion when all tasks are satisfied.

The target final UX is:

```text
Plan complete.
18/18 tasks completed.
```

This reliability behavior should be completed before moving on to JEV so future users can point SOP at a new PRD or PLAN and rely on `sop run` as the primary execution and recovery workflow.
