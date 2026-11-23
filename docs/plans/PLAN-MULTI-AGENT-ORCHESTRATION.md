# Plan: Governed Multi-Agent Orchestration (MAO)

## Project

imhttran/agentic-sop

## Summary

This plan connects SOP's existing orchestration model to its existing lifecycle
driver without adding a second scheduler, lifecycle authority, or framework. It is
delivered in five bounded stages: make the opt-in path non-overclaiming (lease,
usage, evidence), add durable attempt attribution and bounded selection, add
operator visibility, add isolated mutating execution, and only then record a
benefit measurement. Every stage keeps single-agent and serial behavior as the
default; stages 002–005 require the operator decisions recorded in the PRD.

Date: 2026-11-23
Status: **DRAFT — implementation unauthorized.** Requirements:
[PRD-MULTI-AGENT-ORCHESTRATION.md](../requirements/PRD-MULTI-AGENT-ORCHESTRATION.md).
Architecture: [multi-agent-orchestration.md](../architecture/multi-agent-orchestration.md).
Review evidence:
[PRD-MULTI-AGENT-ORCHESTRATION-REVIEW](PRD-MULTI-AGENT-ORCHESTRATION-REVIEW/).
Baseline: `main` @ `2462bdc1`, 2026-11-23.

## Assumptions

- The execution lease remains the single-process exclusion boundary. This plan does
  not run multiple `sop` processes against one project.
- `internal/domain` remains the sole lifecycle authority; orchestration requests
  transitions and never assigns status.
- `internal/scheduler.Next` keeps its single-slot contract and its promotion path.
  `NextBatch` is read-only and must not promote.
- `internal/parallel` is an executor, not a scheduler. It has no production caller
  today, so wiring it is additive.
- `internal/store` is a single writer. The orchestration path appends attempts; it
  does not use `Store.Save`'s delete-and-reinsert path.
- The adoption gate's current REJECT is a safety/cost result, not evidence that
  concurrency lacks benefit. No stage flips a default on an unmeasured dimension.
- Operator decisions D1–D10 in PRD §15 are unresolved unless marked resolved. D3
  (additive columns) and D4 (one process with N slots) are resolved in this
  revision. Stage 001 may proceed without any decision because it only makes an
  existing opt-in path non-overclaiming.

## Capability inventory

| Capability                                     | Status                 | Evidence                                                                                                                |
| ---------------------------------------------- | ---------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| Single lifecycle authority                     | EXISTS                 | `internal/domain/task.go`; `internal/orchestration/lifecycle_guard_test.go`                                             |
| Dependency resolution and cycle rejection      | EXISTS                 | `Task.ResolveDependencies`; `taskbuilder.ValidateDAG`; `scheduler.Next`; `parallel.Select`                              |
| Provider-neutral worker contract               | EXISTS                 | `internal/orchestration/assignment.go` (`WorkAssignment`, `WorkResult`, `Scope`, `ValidateAssignment`)                  |
| Deterministic decomposition and budgets        | EXISTS                 | `internal/orchestration/decompose.go`, `budget.go`                                                                      |
| Write ownership and conflict detection         | EXISTS                 | `internal/orchestration/ownership.go`, `conflict.go`                                                                    |
| Claim-only integration                         | EXISTS                 | `internal/orchestration/integrate.go`                                                                                   |
| End-to-end opt-in orchestration                | EXISTS (analysis-only) | `internal/orchestration/orchestrate.go`; `internal/cli/orchestrate.go`                                                  |
| Adoption gate                                  | EXISTS (REJECT)        | `internal/orchadopt/orchadopt.go`; `sop gate orchestration`                                                             |
| Serial dependency-aware graph driver           | EXISTS                 | `internal/cli/drive.go` (`driveGraph`); `internal/scheduler`                                                            |
| Bounded per-task worktree runner               | PARTIAL (unwired)      | `internal/parallel/parallel.go`; `docs/specs/EXECUTION.md` §8                                                           |
| Execution lease                                | EXISTS (default off)   | `internal/executionlease/lease.go`; `internal/cli/drive.go`                                                             |
| Durable run and trace                          | PARTIAL                | `internal/run`; `internal/runtrace`; orchestration producers have no production caller                                  |
| Per-worker usage accounting on the opt-in path | MISSING                | `internal/cli/orchestrate.go` passes `context.Background()`; `internal/perf/budget.go` fails closed without a collector |
| Per-work-unit attempt identity                 | PARTIAL                | `task_attempts` keyed `(task_id, number)`; no worker/role/provider/worktree attribution                                 |
| Per-endpoint concurrency bound                 | DEFERRED               | `internal/config` `Providers` has endpoints only; no measured saturation incident                                       |
| Task-graph operator view                       | MISSING                | `sop status --detailed` shows dependencies and attempt count, but no graph or blocked predecessor                       |

## Requirement traceability

| Requirement                             | Priority | Stage      | Note                                                             |
| --------------------------------------- | -------- | ---------- | ---------------------------------------------------------------- |
| MAO-001 — One leased execution          | P0       | MAOP-001   | Adds `KindOrchestrate`; refuses `N > 1` without a lease.         |
| MAO-002 — Bounded selection             | P0       | MAOP-002   | `NextBatch` read-only; one shared predicate.                     |
| MAO-003 — Attempt identity              | P0       | MAOP-002   | Additive columns on `task_attempts`; `AppendAttempt`.            |
| MAO-004 — Per-worker isolation          | P0       | MAOP-004   | `RootReadOnly` for analysis; worktree for mutating units.        |
| MAO-005 — Backward compatibility        | P0       | All stages | Preservation requirement; the default-off switch already exists. |
| MAO-006 — Failure/cancellation/recovery | P0       | MAOP-004   | Panic recovery; durable dispatch binding; multi-active resume.   |
| MAO-007 — Provenance and redaction      | P0       | MAOP-001   | Run directory, trace, usage, redaction.                          |
| MAO-008 — Patch integration             | P2       | Deferred   | Claim-only v1; activation requires an approved patch path.       |
| MAO-009 — Per-endpoint capacity         | P2       | Deferred   | Requires a measured saturation incident.                         |
| MAO-010 — Operator visibility           | P1       | MAOP-003   | Task/workflow graph and blocked predecessor.                     |
| MAO-011 — Role profiles                 | P2       | Deferred   | Config mapping only; no new type.                                |
| MAO-012 — Benefit measurement           | P1       | MAOP-005   | Safety gate stays; measurement is a separate operator record.    |
| MAO-013 — Dynamic expansion             | Deferred | Deferred   | Requires its own evidenced proposal.                             |

## MAOP-001 — Make the opt-in path honest and safe

**Objective.** Make `sop orchestrate` participate in the same exclusion, evidence,
and accounting boundaries as `sop run`, without enabling new concurrency.

**Scope.** Acquire and release the execution lease (new `KindOrchestrate`); attach
per-call usage accounting; write a durable run directory and an orchestration trace;
keep the effective concurrency at 1 until per-worker isolation exists; print an
explicit "analysis only, N=1" note; document the limitation. This stage is a
correctness fix and may proceed without any operator decision.

**Files.**

- `internal/cli/orchestrate.go` [Modify] — call the existing `executionLease`
  helper (`internal/cli/execution_lease.go`) before building the agent; use a
  context carrying `perf.WithUsage`; create a `run.Run`; write `trace.json` via
  `runtrace`. `sop orchestrate` stays single-task and exposes no concurrency
  flag; the concurrency refusal is tested on `sop run --parallel-units`.
- `internal/executionlease/lease.go` [Modify] — add `KindOrchestrate`; keep
  `KindRun` and `KindPrompt` behavior unchanged.
- `internal/cli/orchestrate_test.go` [Modify] — lease refusal, lease release, run
  artifact, trace presence, usage attribution.
- `internal/cli/orchestrate_concurrency_test.go` [Create] — redaction and trace
  truncation tests for the opt-in path.
- `internal/cli/resume_lease_test.go` [Modify] — stale-lease report and release.
- `internal/runtrace/collector.go` [Modify] — surface the iteration drop as an
  explicit truncation flag.
- `internal/runtrace/trace.go` [Modify] — carry the truncation flag in the trace.
- `docs/reference/PROJECT-STATUS.md` [Modify] — record the analysis-only limit
  until MAOP-004.
- `docs/plans/BACKLOG.md` [Modify] — record the analysis-only limit.

**Depends on.** none.

**Acceptance.**

- `sop orchestrate` refuses while another execution holds a required lease, using
  the existing `BusyError` text.
- The lease is released on success, error, and cancellation.
- A failing run still leaves a run directory and a trace.
- With `SOP_AGENT_MAX_TOTAL_TOKENS` set, the run either attributes usage correctly
  or refuses before dispatch; it never fails every worker call for lack of a
  collector.
- With orchestration disabled, no new code runs.

**Tests.**

- `TestOrchestrateAcquiresAndReleasesExecutionLease`
- `TestOrchestrateRefusesWhileExecutionLeaseHeld`
- `TestOrchestrateWritesRunAndTraceOnFailure`
- `TestOrchestrateAttributesTokenUsage`
- `TestOrchestrateRefusesTokenBudgetWithoutUsage`
- `TestOrchestrateDisabledIsUnchanged`

**Operator gate.** none.

## MAOP-002 — Attempt attribution and bounded selection

**Objective.** Add durable attempt attribution and a read-only bounded selection
adapter, while preserving single-slot behavior as the default.

**Scope.** Add defaulted attribution columns (including `outcome`) to
`task_attempts`; extend `domain.Attempt` and `insertAttempts` to carry them; add
`AppendAttempt`; add the eligible-task predicate to `internal/domain/selection.go`;
add `scheduler.NextBatch(ctx, N)` as a read-only adapter; add the
`orchestration.max_parallel_tasks` alias for `MaxConcurrency`; wire the `sop run`
graph path for `N > 1` **only for non-mutating units**. Mutating units stay serial
until MAOP-004. Do not promote or save selected tasks in `NextBatch`.

**Files.**

- `internal/store/sqlite.go` [Modify] — migration 8 adds defaulted attribution
  columns to `task_attempts`; `insertAttempts` writes them; `loadAttempts` reads
  them; `AppendAttempt` (`INSERT ... ON CONFLICT(task_id, number) DO UPDATE`); no
  `ClearTasks` change; no new table. Unread columns cannot survive a save: the
  `loadAttempts` SELECT and `Scan` must include every new column.
- `internal/domain/task.go` [Modify] — extend `domain.Attempt` with the attribution
  fields (names and types matching the migration) so `Store.Save` cannot erase
  them.
- `internal/store/workunit_test.go` [Create] — migration idempotence, v7
  compatibility, and an interleaved promotion/append test at N=2.
- `internal/scheduler/scheduler.go` [Modify] — `Next` and `NextBatch` call the shared
  predicate; `NextBatch(ctx, N)` is read-only and ignores in-flight tasks so it can
  select while one task runs; it does not run the Tier-2 recovery requeue. `Next`
  keeps its promotion and recovery paths.
- `internal/domain/selection.go` [Modify] — add `EligibleTasks(tasks, max)` as the
  single predicate: status `PLANNED` or `READY` and `ResolveDependencies`
  satisfied; `Next` adds its own `IsRunnable()` (PLANNED) and `CanTransitionTo(READY)`
  check because it promotes.
- `internal/scheduler/batch_test.go` [Create] — independence, bound, determinism,
  diamond progression, predicate reuse, in-flight selection, no recovery requeue,
  and "NextBatch performs no write".
- `internal/parallel/parallel.go` [Modify] — `Select` delegates to
  `domain.EligibleTasks`.
- `internal/cli/drive.go` [Modify] — call `NextBatch` when configured `N > 1` and
  the selected units are non-mutating; keep the current path for `N = 1` and for
  mutating units.
- `internal/cli/run.go` [Modify] — add `--parallel-units N` (not `--parallel`) as
  the D1 entry point; require `orchestration.enabled: true` for `N > 1`.
- `internal/cli/drive_batch_test.go` [Create] — serial equivalence, N=2 completion,
  and mutating units remain serial.
- `internal/config/config.go` [Modify] — add `orchestration.max_parallel_tasks` as an
  alias for `max_concurrency` (zero keeps existing behavior); no top-level key.
- `internal/config/orchestration_test.go` [Modify] — default, invalid, and omitted
  values.
- `internal/orchestration/budget.go` [Modify] — clamp `EffectiveActiveWorkers` to
  `MaxWorkers`, so the documented bound is enforceable; test the clamp.
- `internal/orchestration/budget_test.go` [Modify] — max-workers clamp.
- `docs/specs/EXECUTION.md` [Modify] — replace the "Partial" parallelism statement
  with the new behavior and the `max_parallel_tasks` key.
- `internal/run/attempt.go` [Modify], `internal/runtrace/trace.go` [Modify],
  `internal/runtrace/orchestration_compose.go` [Modify] — carry the derived
  `workflow_id` (from `plan_id`) and `work_unit_id`; do not clear another worker's
  attempts.

**Depends on.** MAOP-001; operator decision D1 (entry point). D3 (additive
columns) and D4 (one process with N slots) are resolved in this revision.

**Acceptance.**

- A v7 database opens and behaves unchanged; migration 8 is idempotent.
- `N = 1` selection, artifacts, and exit codes are identical to today.
- `NextBatch` performs no write; a selected analysis unit is not `READY` and is not
  treated as in flight.
- `NextBatch` selects while another task is active and does not run the Tier-2
  recovery requeue; recovery remains on `Next`.
- `N = 2` selects two independent non-mutating tasks and never two dependent tasks.
- Mutating units remain serial until MAOP-004; a request for `N > 1` with a
  mutating unit is refused, not silently serialized.
- A diamond graph progresses across iterations; a one-shot batch is not the only
  selector.
- `Next`, `NextBatch`, and `parallel.Select` share one predicate; a test fails if a
  caller re-implements it.
- The effective bound is `EffectiveActiveWorkers` built from the resolved budget
  and policy; `max_parallel_tasks` maps to `MaxConcurrency`; `EffectiveActiveWorkers`
  is clamped to `MaxWorkers`; `SOP_ORCH_MAX_*` may clamp it; a larger value cannot
  widen it.
- `orchestration.enabled: true` is required for `N > 1`; `--parallel-units` is the
  only new CLI flag on `sop run` and does not alias `--max-tasks`.
- With orchestration enabled and `N > 1` while `execution.lease` is off or
  advisory, the run refuses with an actionable message.
- Existing scheduler regression, guardrail, and retry-compatibility tests pass.

**Tests.**

- `TestMigrateAttemptAttributionIsIdempotent`
- `TestV7DatabaseOpensUnchanged`
- `TestNextBatchPerformsNoWrite`
- `TestPromotionAndAttemptAppendDoNotLoseRows`
- `TestNextBatchRespectsN`
- `TestNextBatchNeverSelectsDependentTasks`
- `TestNextBatchDiamondProgresses`
- `TestSelectionPredicateIsShared`
- `TestSerialEquivalenceAtN1`
- `TestDriveCompletesTwoIndependentTasks`
- `TestRunRefusesConcurrencyWithoutLease`
- `TestEffectiveActiveWorkersClampedToMaxWorkers`

**Operator gate.** D1.

## MAOP-003 — Operator visibility

**Objective.** Make the task/workflow graph, attempts, blocked reasons, and key
timings inspectable through existing reporting surfaces. No patch application.

**Scope.** Workflow-scoped status and report output from persisted records; task
and attempt identity rendered from `task_attempts`; blocked reason and blocking
predecessor; orchestration graph rendered by reusing the existing
`runtrace.ReconstructOrchestrationGraph` type (the graph producer itself is wired in
MAOP-001/002; do not add a second graph implementation).

**Files.**

- `internal/cli/status.go` [Modify] — detailed view with workflow, work units, dependencies,
  attempts, blocked reason, and blocking predecessor.
- `internal/cli/report.go` [Modify], `internal/cli/trace_report.go` [Modify] — render
  the orchestration graph by calling `runtrace.ReconstructOrchestrationGraph`;
  render domain status plus attempt outcome.
- `internal/cli/workflow_view_test.go` [Create] — graph reconstruction and blocked
  predecessor.
- `docs/reference/PROJECT-STATUS.md` [Modify] — document the new view.

**Depends on.** MAOP-002; operator decision D10 (operator visibility).

**Acceptance.**

- At least one command reconstructs the workflow graph and names the blocking
  predecessor for a blocked unit.
- The output renders the `internal/domain` task status plus the attempt outcome; it
  does not invent a `queued`/`running` lifecycle.
- No sensitive plaintext is required to reconstruct the graph.

**Tests.**

- `TestStatusShowsWorkflowGraph`
- `TestStatusNamesBlockingPredecessor`
- `TestStatusRendersDomainStatusAndOutcome`

**Operator gate.** D10.

## MAOP-004 — Isolated mutating execution

**Objective.** Let a mutating work unit run in its own worktree with its own
agent/harness, without sharing mutable state with another worker.

**Scope.** Per-worker agent/harness factory and authorized root; `RootReadOnly` for
non-mutating units; one worktree per mutating unit (and for a non-mutating unit that
runs build/test commands); per-work-unit evidence archive identity; panic recovery;
cancellation. Mutating units remain serial, so the existing single-active-task
reconciliation is unchanged; multi-active resume is out of scope. Store
single-writer assumptions are documented and tested.

**Files.**

- `internal/cli/drive.go` [Modify] — build one agent/harness per worker; use a
  `RootReadOnly` profile for non-mutating units and `parallel.NewGitWorkspaces` for
  mutating units; keep mutating units serial; keep the existing single-active-task
  `resumeActiveTask` reconciliation.
- `internal/cli/drive_budget_test.go` [Create] — mid-batch budget exhaustion.
- `internal/cli/drive_isolation_test.go` [Create] — workers cannot see or undo each
  other's changes; cancellation and unknown-outcome tests.
- `internal/orchestration/coordinator.go` [Modify] — replace the shared
  `Coordinator.agent` field with a `WorkerFactory` interface
  (`New(ctx, unit) (agent.Agent, error)`); a nil factory yields an explicit
  `FailureNoAgent` outcome. Add panic recovery that yields a typed failure and
  preserves siblings; no lifecycle authority.
- `internal/orchestration/orchestrate.go` [Modify] — keep `NewCoordinator(a)` as a
  compatibility adapter that wraps a fixed agent in a `WorkerFactory`; no caller
  changes behavior.
- `internal/worktreeevidence/evidence.go` [Modify] — accept a per-work-unit archive
  identity while keeping the legacy per-plan layout readable.
- `internal/worktreeevidence/evidence_test.go` [Modify] — per-work-unit archive,
  legacy read, mismatch handling.
- `internal/parallel/parallel_test.go` [Modify] — evidence-preservation failure does
  not convert success into failure.

**Depends on.** MAOP-002 and MAOP-003; operator decisions D2 (mutating scope) and
D7 (archive identity).

**Acceptance.**

- Two concurrent units never share a working directory, harness, undo registry, or
  write scope.
- A non-mutating unit runs under `RootReadOnly`; a write attempt or a
  repository-mutating shell command is refused.
- A panic in one worker yields a typed failure and siblings still report.
- A mutating unit's evidence survives worktree removal under a per-work-unit
  archive.
- Overlapping write scopes are rejected before dispatch.
- A durable dispatch binding is required before a mutating unit can be in flight
  concurrently; this plan keeps mutating units serial, so `resumeActiveTask`
  continues to see at most one active task.
- With the feature disabled, no worktree or second harness is created.

**Tests.**

- `TestWorkersUseDistinctHarnesses`
- `TestReadOnlyProfileRefusesWrites`
- `TestWorkerPanicIsIsolated`
- `TestCancellationLeavesTasksSelectable`
- `TestUnknownOutcomeIsRecorded`
- `TestNonIdempotentRetryRequiresReconciliation`
- `TestMutatingUnitsUseDistinctWorktrees`
- `TestWorktreeEvidencePerWorkUnit`
- `TestEvidenceFailureDoesNotFailSucceededTask`
- `TestOverlappingScopesRejectedBeforeDispatch`
- `TestMidBatchBudgetExhaustionRecordsOutcomes`

**Operator gate.** D2, D7.

## MAOP-005 — Benefit measurement record

**Objective.** Keep the adoption gate honest and produce a recorded, operator-owned
measurement that can justify or reject concurrency for a named workload.

**Scope.** Keep `sop gate orchestration` as a safety/cost gate; do not make it claim
benefit and do not redesign it into a benchmark. Run a separate operator-approved
measurement and record elapsed time, tokens, failures, and review effort. Keep
`orchestration.enabled` false until a recorded operator decision.

**Files.**

- `internal/cli/gate.go` [Modify] — report the gate's scope and unmeasured dimensions
  without claiming benefit; no new benefit dimension in the gate.
- `internal/orchadopt/orchadopt_test.go` [Modify] — the gate never claims benefit and always
  reports unmeasured dimensions.
- `docs/reports/MAO-BENEFIT-MEASUREMENT.md` [Create] — recorded measurement and decision.
- `docs/plans/BACKLOG.md` [Modify] — update the orchestration entry.

**Depends on.** MAOP-002 and MAOP-004; operator decision D6 (adoption).

**Acceptance.**

- The gate reports "not measured" for wall-clock and token cost; it does not claim
  a benefit.
- A separate recorded measurement names workload, provider, model, budget, and
  repetitions.
- "Not measured" is never presented as a benefit.
- No default changes without an explicit operator decision.

**Tests.**

- `TestGateNeverClaimsBenefit`
- `TestGateRejectsARegression`
- `TestGateReportsUnmeasuredDimensions`

**Operator gate.** D6.

## Cross-cutting verification

These acceptance criteria span stages; each maps to a planned test and an owning
test file, so no criterion is untestable by construction.

| Criterion                                                | Test                                           | Stage    | Test file                                      |
| -------------------------------------------------------- | ---------------------------------------------- | -------- | ---------------------------------------------- |
| Stale lease reported and released; live lease not stolen | `TestResumeReportsAndReleasesStaleLease`       | MAOP-001 | `internal/cli/resume_lease_test.go`            |
| Unknown outcome recorded, never success                  | `TestUnknownOutcomeIsRecorded`                 | MAOP-004 | `internal/cli/drive_isolation_test.go`         |
| Cancellation leaves tasks selectable                     | `TestCancellationLeavesTasksSelectable`        | MAOP-004 | `internal/cli/drive_isolation_test.go`         |
| Mid-batch budget exhaustion records outcomes             | `TestMidBatchBudgetExhaustionRecordsOutcomes`  | MAOP-004 | `internal/cli/drive_budget_test.go`            |
| Trace truncation is explicit                             | `TestTraceTruncationIsReported`                | MAOP-001 | `internal/cli/orchestrate_concurrency_test.go` |
| Non-idempotent retry requires reconciliation             | `TestNonIdempotentRetryRequiresReconciliation` | MAOP-004 | `internal/cli/drive_isolation_test.go`         |
| Worker text is redacted before write/print               | `TestWorkerFindingsAreRedacted`                | MAOP-001 | `internal/cli/orchestrate_concurrency_test.go` |

## Verification gates

Every stage runs, at minimum:

```text
gofmt -l .                       (no output)
go vet ./...
go test ./internal/domain/... ./internal/scheduler/... ./internal/store/...
go test ./internal/orchestration/... ./internal/parallel/... ./internal/orchadopt/...
go test -race ./internal/orchestration/... ./internal/scheduler/... ./internal/store/...
go build ./...
bash scripts/checks/pre-push.sh --base origin/main
```

A documentation-only change (this PRD, architecture, plan, and review evidence)
runs only the formatting and documentation-link gate plus the docs-only
classifier:

```text
bash scripts/checks/test-docs-only.sh
bash scripts/checks/pre-push.sh --base origin/main
```

## Out of scope

- Multiple `sop` processes or a counted execution lease.
- Patch application, branch merging, or publication inside orchestration (MAO-008);
  claim-only v1, and stale-base detection waits for an approved patch path.
- Dynamic task expansion (MAO-013); it requires its own evidenced proposal.
- MAO-009 per-endpoint capacity until a measured saturation incident.
- MAO-011 role profiles until a concrete mapping request exists.
- A new orchestration framework, message bus, or external queue.
- Default-on multi-agent execution.
