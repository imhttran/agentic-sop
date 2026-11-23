# Multi-Agent Orchestration Architecture

**Status:** Proposed — implementation unauthorized  
**Authority:** non-normative implementation seam. Behavior is owned by
[EXECUTION.md](../specs/EXECUTION.md), [WORKFLOW.md](../specs/WORKFLOW.md),
[TASK-LIFECYCLE.md](../specs/TASK-LIFECYCLE.md), and
[HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md). Requirements are owned by
[PRD-MULTI-AGENT-ORCHESTRATION.md](../requirements/PRD-MULTI-AGENT-ORCHESTRATION.md).
**Related:** [orchestration.md](orchestration.md) (the Phase 9 domain model),
[execution.md](execution.md) (the unified execution path),
[SOP-BOUNDARY.md](SOP-BOUNDARY.md) (single-source-of-truth rules).

This document describes **how to extend the existing control plane**, not a new
orchestration framework. Every box below is an existing package or an additive
contract in one.

## 1. As-built map

```text
CLI                         Control plane                         Durable state
───                         ─────────────                         ─────────────
sop run ───────────────► internal/scheduler  (single slot) ────► internal/store (SQLite)
  │                         │
  ├─ internal/cli/drive.go ─┤  dependency-aware, serial graph driver
  │                         │
  ├─ internal/orchestration ┘  non-mutating model + coordinator
  │     model / decision / decompose / assignment / ownership /
  │     conflict / integrate / budget / progress / replacement /
  │     context_routing / orchestrate
  │
  ├─ internal/agent + internal/model + internal/provider   capability → class → provider
  ├─ internal/toolharness (sandbox) + internal/redact        execution boundary
  ├─ internal/executionlease (one owner per project)        exclusion boundary
  ├─ internal/run + internal/runtrace + internal/perf        evidence and metrics
  └─ internal/approval + mergegate + commitgate              human/publication gates
```

Key invariants, already enforced:

1. `internal/domain` is the only lifecycle authority
   (`docs/architecture/orchestration.md`), guarded by
   `internal/orchestration/lifecycle_guard_test.go`.
2. `internal/scheduler` selects at most one task and is not safe for concurrent
   use (`internal/scheduler/scheduler.go`).
3. `internal/store` replaces a task's child collections on save and has no
   compare-and-set (`internal/store/sqlite.go`).
4. `internal/executionlease` allows one owner record per project
   (`internal/executionlease/lease.go`).
5. `internal/toolharness.Harness` is safe for one agent loop, not concurrent calls
   (`internal/toolharness/harness.go`).
6. `internal/parallel` provides bounded, per-task worktree execution but has no
   production caller (`internal/parallel/parallel.go`;
   `docs/specs/EXECUTION.md` §8).
7. `internal/orchestration` is non-mutating and delegates every lifecycle question
   to `internal/domain`.

## 2. Target model

The target keeps **one process, one lease, one lifecycle authority, and N bounded
slots**.

```text
sop run / sop orchestrate (one process, holds execution lease)
  │
  ├── selector: scheduler.NextBatch(ctx, N)     read-only, dependency-aware
  │        └── returns up to N independent PLANNED/READY tasks; promotes nothing
  │
  ├── admission: EffectiveActiveWorkers + write ownership
  │
  ├── executor: for each selected unit
  │        ├── per-worker agent.Agent + toolharness.Harness + authorized root
  │        ├── non-mutating: RootReadOnly profile
  │        └── mutating: internal/parallel worktree + declared write scope
  │
  ├── journal: append-only work-unit attempt record
  │
  ├── integration: serialized, claim-only in v1
  │
  └── verification: existing deterministic path (authoritative)
```

The controller **requests** transitions; only `internal/domain` **decides** them.
No orchestration type defines a status, transition, or completion predicate.

## 3. Component responsibilities

| Component                                        | Responsibility in the target                                                                             | Explicitly not responsible for                      |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| `internal/domain`                                | Statuses, transitions, dependency resolution, retry budget, completion predicates                        | Scheduling, I/O, model calls                        |
| `internal/scheduler`                             | Deterministic selection of up to `N` independent runnable tasks; preserves `Next` for `N=1`              | Executing work, defining lifecycle                  |
| `internal/cli/drive.go`                          | Bounded loop that selects, admits, dispatches, integrates, and reconciles                                | Defining policy; bypassing gates                    |
| `internal/orchestration`                         | Non-mutating coordination for analysis/proposal units; assignment, ownership, conflict, integration view | Lifecycle mutation; persistence; process management |
| `internal/parallel`                              | Bounded executor and per-task Git worktrees for mutating units                                           | Scheduling; lease ownership; lifecycle decisions    |
| `internal/executionlease`                        | One owner per project; heartbeat; stale detection and release                                            | Per-worker exclusion; task state                    |
| `internal/store`                                 | Durable task and attempt state; single writer                                                            | Concurrency control beyond one writer               |
| `internal/run` / `internal/runtrace`             | Durable run directory, attempts, trace, orchestration graph                                              | Authority; redaction policy ownership               |
| `internal/agent` / `internal/model`              | Capability, class, provider, model, locality resolution                                                  | Scheduling; approval                                |
| `internal/toolharness`                           | Authorized roots, write scope, isolation, audit                                                          | Scheduling; model choice                            |
| `internal/approval` / `mergegate` / `commitgate` | Human and publication gates                                                                              | Worker execution                                    |

## 4. Concurrency model

**Decision:** one leased process, `N` in-memory slots. The lease is not extended to
counted slots.

Rationale: the lease protects the repository and the SQLite store; the store is a
single-writer resource. Running `N` processes would require a counted lease
schema, per-holder heartbeat, and cross-process store coordination. The in-process
pool achieves the same parallelism with a fraction of the failure modes.

Rules:

- `N` is a positive integer with a safe default of **1**, bounded by the existing
  `EffectiveActiveWorkers` (`min(max_active_workers, max_concurrency)` floored at
  1). `orchestration.max_parallel_tasks` is an optional alias for
  `ExecutionPolicy.MaxConcurrency`, not a fourth limit; a zero value leaves the
  existing resolution (1). No separate literal cap is introduced; the value cannot
  exceed `max_workers`.
- Selection uses **one** shared eligibility predicate over `PLANNED`/`READY` tasks
  whose dependencies are satisfied. The predicate lives in `internal/domain` as a
  pure function, so `Next`, `NextBatch`, and `parallel.Select` share it without a
  `parallel → scheduler` import. `Next` additionally requires `PLANNED` because it
  promotes; `NextBatch` is read-only and promotes nothing.
- `NextBatch` must not write. Promoting N analysis units would make them `READY`,
  which `scheduler.IsActive` treats as in flight and `resume.ActionFor(READY)`
  routes to `CreateBranch`; a cancelled batch would then be unrecoverable.
- Mutating work is serialized in v1. Analysis units may run concurrently; each
  runs under the harness's `RootReadOnly` profile, and a unit that must run build
  or test commands gets its own worktree.
- A worker never acquires the project lease.
- Selection order is deterministic (lowest task ID first), so no starvation.
- If the lease is off or advisory, `N > 1` is refused rather than silently run
  serially. The lease is the outer exclusion boundary, not an optional extra.

Backward compatibility: `Next` keeps its current single-slot contract. The driver
calls `NextBatch` only when `N > 1`; `N = 1` must be behavior- and
artifact-identical to today. `Next`, `NextBatch`, and `parallel.Select` share one
predicate; none re-implements it.

## 5. Persistence model

The task row remains the plan unit. In v1 a work unit is **1:1 with a plan task**,
so the durable identity is carried by additive, defaulted columns on the existing
`task_attempts` table. `workflow_id` is derived from the existing `plan_id`; do not
store a second workflow identity that can drift. No second attempt table is
introduced.

Proposed additive migration (migration 8, `user_version = 8`):

```sql
ALTER TABLE task_attempts ADD COLUMN workflow_id TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN work_unit_id TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN capability TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN role TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN provider TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN locality TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN worktree TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN outcome TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN failure_code TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN evidence TEXT NOT NULL DEFAULT '[]';
ALTER TABLE task_attempts ADD COLUMN started_at TEXT NOT NULL DEFAULT '';
ALTER TABLE task_attempts ADD COLUMN finished_at TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_task_attempts_work_unit ON task_attempts(work_unit_id);
```

Properties:

- Additive and defaulted. A v7 database opens unchanged; a single-agent run writes
  the same rows with empty default values.
- The existing `(task_id, number)` primary key stays. It is sufficient while exactly
  one worker owns a task.
- The orchestration path writes attempts through `AppendAttempt`
  (`INSERT ... ON CONFLICT(task_id, number) DO UPDATE`) and does **not** use
  `Store.Save`'s delete-and-reinsert path. `scheduler.Next` saves a staged copy on
  promotion, so a batch must not interleave a promotion save with an attempt
  append for another holder; the plan requires an interleaving test.
- A later design that allows two workers on one task needs a discriminator (for
  example `(task_id, work_unit_id, number)`) and a new operator decision. Do not
  silently overload `number`.
- `ClearTasks` is unchanged. Attempts are deleted with their task, exactly as
  today. A design that needs work-unit evidence to survive plan historicalization
  must change the plan-closure rule explicitly; do not assume a second table would
  survive a cascade.
- The store remains a single writer. If a future design uses multiple processes,
  WAL, a connection cap, and a compare-and-set or bounded retry are prerequisites.

## 6. Isolation model

| Work type                    | Workspace                        | Agent/harness  | Write scope               |
| ---------------------------- | -------------------------------- | -------------- | ------------------------- |
| Non-mutating analysis        | Shared root under `RootReadOnly` | One per worker | Deny-all                  |
| Non-mutating with build/test | Own Git worktree                 | One per worker | Deny-all                  |
| Mutating implementation      | Own Git worktree                 | One per worker | Declared, proven-disjoint |

`agent.Capability` is a label, not an enforced boundary. The enforcement is
`toolharness.RootReadOnly`, which refuses write tools and repository-mutating shell
commands. `Scope{}` remains a post-dispatch result filter; it is not the primary
boundary. A non-mutating unit that must run build or test commands gets a worktree
because those commands can create artifacts. Worktree isolation for every
concurrent unit is defense-in-depth, not a substitute for `RootReadOnly`.

- A per-worker `toolharness.Harness` owns its undo registry, identity registry, and
  audit sink. Sharing one harness between goroutines is forbidden.
- `internal/parallel.GitWorkspaces` already refuses unsafe task IDs and propagates
  evidence-preservation errors; reuse it.
- Worktree evidence is archived per **work unit**, not per plan, or the second
  removal of a plan's worktrees will fail the archive match
  (`internal/worktreeevidence/evidence.go`). Legacy per-plan archives remain
  readable.
- Cleanup never converts a succeeded task into a failed one; an evidence
  preservation failure is reported separately and escalated.

## 7. Failure, cancellation, and recovery

```text
selected → admitted → running → { succeeded | failed | cancelled | unknown }
                                  │
                                  ├─ succeeded: claim only; verification decides
                                  ├─ failed: stable FailureCode; recovery policy
                                  ├─ cancelled: context cancelled; state resumable
                                  └─ unknown: human escalation; no blind retry
```

- Each worker runs with panic recovery that converts a panic into a typed failure
  and preserves sibling outcomes.
- Cancellation stops new selection, cancels in-flight contexts, and leaves every
  started unit in a reconcilable state. Because `NextBatch` does not promote, a
  cancelled analysis unit changes no task status, so the task remains
  `PLANNED`/`READY` and is selectable again. Mutating concurrency requires a durable
  dispatch binding and `resumeActiveTask` support for N active tasks before it
  ships.
- Mid-batch budget exhaustion stops new selection, records each started unit's
  outcome (including `unknown`), and leaves state resumable. One worker crossing the
  token budget must not silently discard a sibling's claim.
- Recovery reuses `internal/recovery.Decide` and `internal/resume`. No second
  policy.
- A non-idempotent side effect is never retried without reconciliation. An unknown
  outcome is recorded on the attempt and, when a task status must change, maps to
  `BLOCKED` with a recoverable `BlockedReason`; no new status is added.

## 8. Security and trust boundaries

- Worker output is untrusted data. It cannot alter scheduling, scope, budget, or
  approval.
- Capability and scope admission happen before dispatch
  (`Assignment.Validate`, `AdmitWriteOwnership`), not through prompt instructions.
- Redaction is applied at one persistence/print chokepoint for worker-supplied
  text. Do not rely on each writer remembering to redact.
- Provider/model/locality resolution reuses `internal/agent`, `internal/model`, and
  the existing locality policy. A model name is not a security boundary.
- The execution lease is the outer exclusion boundary; the per-worker harness and
  worktree are the inner boundaries.

## 9. Observability

Persisted evidence for one orchestrated run:

```text
.agent-sdlc/runs/<workflow-or-task>/
  state.json                 run stage
  trace.json                 structured trace, including orchestration events
  attempts/<n>.json          per-attempt selection and recovery record
  report.md                  human report
  activity.jsonl             optional activity stream
```

Requirements:

- A workflow ID links every task/run/attempt record.
- The orchestration graph is reconstructable from `trace.json` alone, using the
  existing `ReconstructOrchestrationGraph`.
- Token usage is attributed to the same work-unit/attempt identity, or recorded as
  unreported. A configured token budget must never fail every call because no
  usage collector is attached.
- Trace truncation is explicit. `runtrace.Collector` drops events beyond
  `maxIterations`; the trace must record that it did. `Trace.Context.Truncated`
  describes context truncation only and is not a substitute.

## 10. Alternatives rejected

| Design                                      | Reason                                                                                                                                                  |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| New orchestration framework                 | Duplicates a verified model and violates the single-authority invariant.                                                                                |
| N processes + counted lease                 | Requires lease schema, heartbeat, and store coordination; no evidence it is needed.                                                                     |
| `internal/parallel.Runner` as the scheduler | One-shot batch, no loop, no lease, no budget, no provenance.                                                                                            |
| Second attempt table in v1                  | A work unit is 1:1 with a task; the existing attempt key plus nullable columns is sufficient. A second table adds a vocabulary every reader must union. |
| Shared harness with internal locking        | The harness's undo and identity registries are per-invocation and per-root; locking does not make them semantically isolated.                           |
| New `CONFLICT` lifecycle status             | Second lifecycle vocabulary; use `BLOCKED` + `BlockedReason` or a non-lifecycle record.                                                                 |
| Patch application in v1                     | No existing conflict-aware patch path; claim-only is the safe first step.                                                                               |
| Default-on adoption                         | The gate returns REJECT and cannot measure the justifying dimensions.                                                                                   |

## 11. Open decisions

D1–D10 are recorded in the PRD §15. The architecture is blocked on D2 (first
concurrency scope) before mutating execution; D3 (persistence) and D4 (lease model)
are resolved in this revision. D9 (per-endpoint capacity) is deferred until a
measured saturation incident. D1, D5–D8 and D10 shape the first release but do not
change the structure above.
