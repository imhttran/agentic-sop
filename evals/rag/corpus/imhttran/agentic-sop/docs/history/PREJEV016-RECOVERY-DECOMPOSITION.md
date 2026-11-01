# PREJEV016 — Verify Recovery Paths (Decomposition)

This document restructures the umbrella milestone **PREJEV016** into focused,
independently executable recovery-regression tasks. It is the updated PREJEV016
plan/task decomposition; the umbrella stays, and S1–S7 become tasks that can each
be understood, verified, and completed on their own, verify-first.

## Status

- **PREJEV016 remains the umbrella recovery milestone.** It completes only when
  S1–S7 have all been validated (see [Umbrella completion criteria](#umbrella-completion-criteria)).
- **The plan source now decomposes PREJEV016.** `docs/plans/PLAN-Pre-JEV-Stabilization.md`
  declares the umbrella plus PREJEV016-S1 … PREJEV016-S7, and `.agent-sdlc/plan.json`
  is regenerated from it so the machine plan matches.
- **S1–S7 are standalone task files** under `docs/tasks/prejev016/`, each runnable
  in isolation:

  ```bash
  sop run --task docs/tasks/prejev016/PREJEV016-S1.md
  sop run --task docs/tasks/prejev016/PREJEV016-S2.md
  sop run --task docs/tasks/prejev016/PREJEV016-S3.md
  sop run --task docs/tasks/prejev016/PREJEV016-S4.md
  sop run --task docs/tasks/prejev016/PREJEV016-S5.md
  sop run --task docs/tasks/prejev016/PREJEV016-S6.md
  sop run --task docs/tasks/prejev016/PREJEV016-S7.md
  ```

## Why this decomposition exists

The previous PREJEV016 IMPLEMENT invocation exhausted its tool budget during
discovery, before it wrote the recovery tests it had planned, and exited
`NEEDS_HUMAN` with no repository change:

- `.agent-sdlc/runs/PREJEV016/report.json` → `"decision": "NEEDS_HUMAN"`,
  `validation_runs: 0`, `agent_calls: 2`.
- `.agent-sdlc/runs/PREJEV016/report.md` → the gate: "Tool budget exhausted before
  the required repository change (recovery regression tests) could be written."

The fix is **not** a larger iteration budget. It is to make each recovery area a
small, single-responsibility task whose verification is deterministic, so an
already-covered area never needs an agent at all. No agent or tool budget was
increased.

## Design rules applied

Each task S1–S7 satisfies the decomposition requirements:

1. **One responsibility** — a single recovery area (see each task's _Objective_).
2. **One subsystem** — the relevant package(s) are named explicitly.
3. **Concrete acceptance criteria** — checkable statements, not prose.
4. **Explicit validation commands** — focused package tests first, broader
   regression checks after (see each task's _Requirements_ and this document's
   [validation matrix](#validation-matrix)).
5. **No unrelated discovery** — scope is limited to the named subsystem.
6. **Independently resumable and idempotent** — a task runs via `sop run --task`
   and writes to its own `.agent-sdlc/runs/<ID>/`; re-running is safe.
7. **No manufactured changes** — a requirement already covered by existing tests is
   _recorded_, not rewritten. Covered requirements complete with no change
   (`changes_expected=false`).
8. **No duplicated orchestration** — tasks are task files; the SOP lifecycle
   (`PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`) is reused unchanged.

### Execution mode and the per-task loop

Every sub-task follows the same three-step loop:

1. **Verify existing coverage first** — inspect the tests that already exist for
   the area before changing anything.
2. **Implement only the missing coverage** — add a test only where a real gap is
   demonstrated; otherwise change nothing.
3. **Validate independently** — SOP's own VALIDATE stage runs the deterministic
   validation; the agent's self-report is not the validation.

Execution mode is per-task:

- **S1–S5 run in `verify-first`.** Their behavior is already proven by the existing
  suite, so the deterministic validation passes, no implementation agent is
  invoked, and each completes with `changes_expected=false`. This is the fast path
  that removes the budget-exhaustion failure mode: an already-covered area needs no
  agent.
- **S6 and S7 run in `implement`.** S6 closes the one confirmed coverage gap and S7
  is the closure that may need to add coverage. `verify-first` would run the
  passing project validation first and short-circuit, so it could never add the
  missing test.

The deterministic validation SOP runs is the project-configured validation
(`validation.build` / `test` / `lint` in `.agent-sdlc/config.yaml`), not the
focused command listed on each task. The focused commands are the guidance for a
human or agent narrowing the area; the failing or passing gate is the configured
suite, so a regression in a recovery area fails that suite and pulls in the agent.

`resume`, `retry`, and the active-task recovery in `run` are sequential
store-backed paths, not concurrency-sensitive; no `-race` run is required for them.
The `-race` suite is still part of the broader gate run by S7.

## Existing coverage map

The recovery suite already exists (largely from the resume/retry and lifecycle
work). The table maps each PREJEV016 area to the tests that already cover it.

| Area                                           | Subsystem                           | Existing tests                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Status       |
| ---------------------------------------------- | ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------ |
| **S1** Resume continues valid work             | `internal/resume`, `internal/cli`   | `TestResolveActionsForEveryStatus`, `TestRecoverExistingBranch`, `TestRecoverExistingPR`, `TestResolvePersistsRecovery`, `TestRecoverySaveFailureIsAtomic`, `TestResolveActive`, `TestObserverErrorPropagates`, `TestCancellation`, `TestRunResumeReportsCreateBranchForReadyTask`, `TestRunResumeReportsNextAction`, `TestRunResumeRecoversExistingBranch`, `TestRunResumeNothingToResume`, `TestRunResumeConsistencyError`, `TestRunResumeBadArgs`, `TestRunResumeUninitialized`, `TestRunResumeIsIdempotent` | covered      |
| **S2** Retry requeues legal blocked work       | `internal/cli`, `internal/domain`   | `TestRunRetryRequeuesBlocked`, `TestRunRetryRejectsCompleted`, `TestRunRetryRejectsNotBlocked`, `TestRunRetryUnknownTask`, `TestRunRetryBadArgs`, `TestRunRetryBudgetExhausted`, `TestRunRetryForceRaisesBudget`, `TestRunRetryAll`, `TestRunRetryAllNothingBlocked`, `TestRunGraphNeedsHumanExhaustsRetryBudget`, `TestRequeue`, `TestRequeueWithoutSpending`, `TestRequeueBudgetExhausted`                                                                                                                    | covered      |
| **S3** Interrupted active-task recovery        | `internal/cli`                      | `TestRunSelectsPlannedTaskWhenNoActiveTask`, `TestRunResumesInterruptedActiveTask`, `TestRunResumesActiveTaskInsteadOfStartingAnother`, `TestRunCompletesActiveTaskThatAlreadyPassed`, `TestRunRejectsAmbiguousActiveTasks`, `TestRunRefusesRemoteParkedActiveTask`                                                                                                                                                                                                                                             | covered      |
| **S4** Named-plan provenance                   | `internal/cli`, `internal/planflow` | `TestRunNamedPlan`, `TestRunNamedPlanResolvesToDocs`, `TestRunNamedPlanAmbiguous`, `TestRunNamedPlanMissing`, `TestRunDifferentPlanStops`, `TestRunChangedPlanStops`, `TestPrepareRebuildsWhenSourceChanges`, `TestPrepareReconcilesChangedPlanWithTasks`                                                                                                                                                                                                                                                       | covered      |
| **S5** Safe task reset, if available           | `internal/cli`, `internal/store`    | none — no reset capability exists (see [Safe reset determination](#safe-reset-determination))                                                                                                                                                                                                                                                                                                                                                                                                                   | n/a          |
| **S6** Non-destructive recovery / working-tree | `internal/cli`, `internal/store`    | `TestCommandPolicyRejectsDestructiveGitAndOthers` (tool policy); **added by this milestone:** `TestRetryPreservesWorkingTreeAndRunHistory`, `TestResumePreservesWorkingTreeAndRunHistory`, `TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory`                                                                                                                                                                                                                                                            | gap → closed |
| **S7** Recovery regression closure             | all of the above                    | the full suite                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | to do        |

### Safe reset determination

No safe task-reset capability exists, so the requirement is recorded as **not
applicable** and no reset feature is added to satisfy PREJEV016. Evidence:

- `internal/cli/cli.go` has no `reset` subcommand — the recovery commands are
  `resume` and `retry` only.
- The only `Reset` symbols in the tree are unrelated: `bytes.Buffer.Reset` inside
  `internal/toolharness/harness.go` and `FakeProvider.Reset` in the test harness
  (`internal/e2e/harness/harness.go`).
- Recovery is served by `retry` (requeue a BLOCKED task) and `resume` (reconcile an
  interrupted task), both non-destructive, so a reset is not needed as a substitute.

## Coverage gaps (confirmed and closed)

One acceptance area had no explicit test; it was confirmed against the existing
suite and closed with three focused tests.

| ID  | Area | Gap                                                                                                                        | Proving tests (`internal/cli/recovery_test.go`)                                                                                                        |
| --- | ---- | -------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| G1  | S6   | No test asserted that recovery preserves a pre-existing working-tree change and does not delete `state.db` or run history. | `TestRetryPreservesWorkingTreeAndRunHistory`, `TestResumePreservesWorkingTreeAndRunHistory`, `TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory` |

The G1 tests run recovering commands over a real Git repository with a modified
tracked file and an untracked file, so a recovery that ran `git reset --hard` would
revert the tracked edit and one that ran `git clean` would delete the untracked
file; they also assert the state database and the run-history directory survive.

## Task definitions

### PREJEV016-S1 — Resume Continues Valid Work

- **Responsibility:** verify that `sop resume` resolves the in-flight task, reports
  the next legal action, persists the reconciliation, and continues instead of
  restarting or duplicating work.
- **Subsystem:** `internal/resume`, `internal/cli` (`resume.go`).
- **Dependencies:** none.
- **Execution:** `verify-first`.
- **Focused validation:** `go test ./internal/resume/...` then
  `go test ./internal/cli/ -run 'Resume'`.

### PREJEV016-S2 — Retry Requeues Legal Blocked Work

- **Responsibility:** verify that retry requeues only retryable BLOCKED work,
  preserves task history, and refuses completed or non-blocked statuses.
- **Subsystem:** `internal/cli` (`retry.go`), `internal/domain` (`Requeue`).
- **Dependencies:** none.
- **Execution:** `verify-first`.
- **Focused validation:** `go test ./internal/cli/ -run 'Retry'` then
  `go test ./internal/domain/ -run 'Requeue|Transitions|Terminal'`.

### PREJEV016-S3 — Interrupted Active-Task Recovery

- **Responsibility:** verify that `sop run` resumes exactly one in-flight task,
  refuses ambiguous or parked active tasks with a clear diagnostic, and never
  mutates state destructively.
- **Subsystem:** `internal/cli` (`drive.go`, `run.go`).
- **Dependencies:** S1.
- **Execution:** `verify-first`.
- **Focused validation:** `go test ./internal/cli/ -run 'Active|Interrupted|Resume'`
  then `go test ./internal/cli/ -run 'Run'`.

### PREJEV016-S4 — Named-Plan Provenance

- **Responsibility:** verify that a named plan resolves to its own graph, a
  different or changed plan produces a focused failure, and resume/retry cannot
  rebind the plan graph.
- **Subsystem:** `internal/cli` (`run.go` plan resolution), `internal/planflow`.
- **Dependencies:** none.
- **Execution:** `verify-first`.
- **Focused validation:**
  `go test ./internal/cli/ -run 'NamedPlan|DifferentPlan|ChangedPlan'` then
  `go test ./internal/planflow/...`.

### PREJEV016-S5 — Safe Task Reset, If Available

- **Responsibility:** determine whether a safe task-reset capability exists; verify
  it if present, otherwise record the requirement as not applicable and add nothing.
- **Subsystem:** `internal/cli`, `internal/domain`, `internal/store`.
- **Dependencies:** none.
- **Execution:** `verify-first`.
- **Focused validation:** `go test ./internal/cli/...` then
  `go test ./internal/store/...`.

### PREJEV016-S6 — Non-Destructive Recovery and Working-Tree Preservation

- **Responsibility:** verify that recovery preserves pre-existing user changes,
  does not invoke `git reset --hard` or `git clean`, and does not delete
  `.agent-sdlc`, `state.db`, or run history (gap G1).
- **Subsystem:** `internal/cli` (recovery commands), `internal/store`.
- **Dependencies:** S1, S2, S3.
- **Execution:** `implement` (may complete with `changes_expected=false` when no
  gap is confirmed).
- **Focused validation:** `go test ./internal/cli/ -run 'PreservesWorkingTree'`
  then `go test ./internal/cli/...` and `go test ./internal/resume/...`.

### PREJEV016-S7 — Recovery Regression Closure

- **Responsibility:** run the complete recovery suite, map every acceptance
  criterion to a passing verification, and add only genuinely missing coverage.
- **Subsystem:** the whole suite (`internal/resume/...`, `internal/cli/...`).
- **Dependencies:** S1, S2, S3, S4, S5, S6.
- **Execution:** `implement` (may complete with `changes_expected=false` when no
  gap is confirmed).
- **Focused validation:** `go test ./internal/resume/...`,
  `go test ./internal/cli/...`, then the full validation matrix below.

## Validation matrix

Focused commands per task are above; the umbrella gate is:

```bash
go test ./internal/resume/...
go test ./internal/cli/...
go test ./...
go vet ./...
go build ./...
```

Because the recovery paths are sequential and store-backed, no focused `-race` run
is required; the broader `go test -race ./...` remains available as an optional
extra check.

## Umbrella completion criteria

PREJEV016 is complete only when **all** of the following hold:

1. Every task PREJEV016-S1 … PREJEV016-S7 has run to a passing result
   (`Passed` / `LOCAL_DONE`) via `sop run --task …`, having verified existing
   coverage, added only real gaps, and validated independently. S1–S5 complete
   with `changes_expected=false`; S6 closes G1; S7 confirms closure.
2. Each recovery area — resume, retry, interrupted active-task recovery, named-plan
   provenance, safe reset, working-tree preservation — maps to at least one passing
   test (see the [coverage map](#existing-coverage-map)), or is recorded as not
   applicable (safe reset).
3. G1 is covered by its proving tests (listed above) and passes.
4. The validation matrix above passes.
5. No correct recovery behavior was rewritten merely to force a repository change,
   and no out-of-scope feature — in particular no new reset feature — was added.

### Acceptance criteria → task

| Acceptance criterion                                                         | Task   |
| ---------------------------------------------------------------------------- | ------ |
| resume continues valid work                                                  | S1     |
| retry requeues legal blocked work                                            | S2     |
| interrupted active-task recovery is safe and preserves valid repository work | S3, S6 |
| named plans cannot silently mix graphs                                       | S4     |
| safe reset is verified if available, else recorded not applicable            | S5     |
| recovery does not require deleting `state.db` or run history                 | S6     |
| working-tree changes are preserved across recovery                           | S6     |
| regression coverage closure                                                  | S7     |

## Workflow authority (unchanged)

This decomposition does not change SOP's workflow authority or lifecycle:

- Lifecycle remains `PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`.
- Human gates are unchanged; nothing is committed, pushed, or merged
  automatically.
- No destructive reset/cleanup; `.agent-sdlc/state.db` is never hand-edited.
- No regression task re-implements orchestration; each is a task file driven by
  the existing lifecycle.
- No CI/PR/merge result is fabricated.
