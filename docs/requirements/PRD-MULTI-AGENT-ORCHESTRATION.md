# PRD: Native Multi-Agent Orchestration for SOP

**Status:** Draft — repository-verified, EGM-reviewed; implementation unauthorized  
**Project:** [agentic-sop](https://github.com/imhttran/agentic-sop)  
**Canonical path:** `docs/requirements/PRD-MULTI-AGENT-ORCHESTRATION.md`  
**Date:** 2026-11-23  
**Owner:** SOP project maintainers  
**Scope:** Requirements, architecture validation, and phased planning only

## 0. How to read this revision

This revision replaces the unverified first draft. Every statement about current
behavior below was checked against the repository on 2026-11-23 and names its
evidence. Where the first draft assumed a capability was missing, this revision
records what actually exists and narrows the requirement to the real gap.

**Requirement IDs changed.** The first draft reused `ORCH-001` … `ORCH-016`, which
are the IDs of the **already delivered** Phase 9 tasks
([PHASE-9-MULTI-AGENT-ORCHESTRATION.md](../history/plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md)).
Two different meanings under one ID make `grep ORCH-005` ambiguous. This revision
uses `MAO-001` … `MAO-013` (Multi-Agent Orchestration) and cites Phase 9 `ORCH-*`
IDs only as history.

**What this document does not authorize.** It does not authorize code changes, a
default-on switch, a merge, or a new orchestration framework. It defines the
smallest extension of the existing SOP control plane and records the operator
decisions that must precede implementation.

### Terminology

- **SOP controller.** The existing execution control plane: the CLI driver
  (`internal/cli/drive.go`), the deterministic selector
  (`internal/scheduler`), and the sole lifecycle authority (`internal/domain`). It
  is not a new component, process, or service.
- **Work unit.** One bounded unit of work: a `domain.Task` plus one
  `agent.Capability`. In v1 it is a read-only projection of a task, 1:1, and
  `work_unit_id` is persisted as attempt attribution, not as a second entity.
  `orchestration.WorkUnit` and `orchestration.WorkAssignment` are the existing
  types.
- **Attempt outcome.** The result of one execution attempt, recorded on an attempt
  row: `succeeded`, `failed`, `cancelled`, or `unknown`. It is not a task status and
  does not replace the `internal/domain` lifecycle.
- **Claim.** A worker's self-reported result (`orchestration.WorkResult`,
  surfaced as a `WorkerClaim`). It is untrusted, non-authoritative data until SOP
  verification accepts it. "Claim-only integration" means v1 records and validates
  claims but applies no patch.
- **EGM.** Elephant-Goldfish Model: the independent, fresh-context review protocol
  used for this document. See
  [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md).

## 1. Summary

SOP already has a deterministic, provider-neutral orchestration model, a worker
assignment contract, decomposition, write-ownership admission, conflict detection,
a claim-only integration stage, budgets, progress classification, orchestration
provenance types, an execution coordinator, and an opt-in end-to-end path
(`internal/orchestration`, Phase 9 `ORCH-001` … `ORCH-012`). It also has a serial,
dependency-aware graph driver (`internal/scheduler`, `internal/cli/drive.go`), a
bounded per-task worktree runner that is **not wired to any command**
(`internal/parallel`), a durable task store (`internal/store`), a resumption and
recovery policy (`internal/resume`, `internal/recovery`), and a project-scoped
execution lease that enforces **one active execution per project**
(`internal/executionlease`).

The real gap is not a missing orchestration model. It is that `sop orchestrate` is not
connected to the lifecycle driver (`internal/cli/drive.go`): it is not
concurrency-safe, it is not durable, and it is not observable. It also reports none
of these limits.

- `sop orchestrate` runs one task file, one `Plan` candidate, with an empty
  (deny-all) write scope, and persists nothing (`internal/cli/orchestrate.go`).
- It does not acquire the execution lease.
- It passes `context.Background()`, so per-call token accounting is absent and a
  configured token budget fails closed (`internal/perf/budget.go`).
- The coordinator shares one `agent.Agent` across parallel worker goroutines, while
  the tool harness is documented as unsafe for concurrent calls
  (`internal/toolharness/harness.go`). `sop orchestrate` currently emits one
  candidate, so the hazard is latent rather than exercised.
- The task store replaces a task's attempt rows on every save and has no
  compare-and-set, so two writers can lose updates (`internal/store/sqlite.go`).
- The trace/orchestration-graph producers have no production caller
  (`internal/runtrace`).
- The adoption gate can only adopt a multi-agent candidate that uses no more
  workers or tool calls than the single-agent baseline, which no multi-unit
  decomposition can do (`internal/orchadopt/orchadopt.go`).

This PRD defines a minimal extension of the existing SOP control plane: **one
leased SOP process running a bounded number of independent work units, each with
its own execution identity, isolation, persistence, and evidence; single-agent and
serial behavior remain the default.**

## 2. Problem

Complex changes contain separable activities. SOP can already represent them as
`domain.Task` rows with dependencies and execute them serially in dependency order.
It cannot safely run two of them at once, for five verified reasons:

1. **The scheduler is single-slot by contract.** `scheduler.Next` returns
   `ActiveTask` and selects nothing while any task is in flight; the type is
   documented as not safe for concurrent use
   (`internal/scheduler/scheduler.go`).
2. **The store is not a concurrency boundary.** `Store.Save` deletes and reinserts
   the task's dependency and attempt rows; there is no version column or
   compare-and-set (`internal/store/sqlite.go`).
3. **The execution lease is a single-owner file and is off by default.** It is
   created `O_EXCL` and holds exactly one owner record
   (`internal/executionlease/lease.go`). Its documented callers are `sop run` and
   `sop resume`; `sop orchestrate` is not one of them.
4. **The tool harness is not concurrency-safe.** The `Harness` type documentation
   says it is "safe for sequential use by one agent loop; it is not designed for
   concurrent calls" (`internal/toolharness/harness.go`).
5. **The durable record is single-task scoped.** Task attempts carry no worker,
   role, provider, model, locality, or worktree; `run.New` clears the previous
   attempt directory; and `sop orchestrate` writes no run directory at all
   (`internal/domain/task.go`, `internal/run/run.go`, `internal/cli/orchestrate.go`).

Without these changes, concurrency would risk silent lost updates, overlapping
workspace writes, unattributable evidence, and unresumable runs.

## 3. Verified baseline

Status vocabulary: **Implemented** = the capability exists and is exercised on a
production path; **Partial** = a component exists but is unwired, not
concurrency-safe, or missing a required property; **Missing** = no implementation;
**Unnecessary** = already satisfied, so no new requirement.

| Area                              | Status      | Evidence                                                                                                                                                                                                               | Consequence                                                                                                                        |
| --------------------------------- | ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Single lifecycle authority        | Implemented | `internal/domain/task.go` (`transitions`, `Transition`, `Block`, `Requeue`, `IsSatisfied`, `ResolveDependencies`); guard test `internal/orchestration/lifecycle_guard_test.go`                                         | Orchestration must never define statuses or mutate `Task`.                                                                         |
| Task contract                     | Partial     | `domain.Task` has ID, dependencies, attempts, `AcceptanceChecks`, `RequiredEvidence`; `orchestration.WorkAssignment` adds scope, budget, output contract, repository identity (`internal/orchestration/assignment.go`) | No parent/workflow ID, persisted worker scope, owner/runtime, or role.                                                             |
| Dependency enforcement and cycles | Implemented | `Task.ResolveDependencies`; `taskbuilder.ValidateDAG` rejects cycles and unknown dependencies; `scheduler.Next`; `parallel.Select`                                                                                     | Reuse; do not add a second dependency language.                                                                                    |
| Bounded concurrency               | Partial     | `orchestration.ExecutionPolicy` + `OrchestrationBudget`; `internal/parallel.Runner` bounded by `max`                                                                                                                   | `internal/parallel` has no non-test caller; no per-provider limit; default concurrency resolves to 1.                              |
| Worker isolation                  | Partial     | `internal/parallel.GitWorkspaces` creates one worktree per task; `internal/toolharness/isolation.go` sandboxes tool calls                                                                                              | One shared `agent.Agent`/harness in `sop orchestrate`; worktree evidence archive is keyed by plan id.                              |
| Provenance                        | Partial     | `internal/runtrace` trace + sanitizer; `internal/run` artifacts; `store.task_attempts`; `internal/redact`                                                                                                              | Orchestration event producers have no production caller; `sop orchestrate` writes nothing.                                         |
| Failure handling                  | Partial     | `orchestration.FailureCode`; `internal/recovery.Decide`; retry budget in `domain.Task`; `internal/resume`                                                                                                              | No panic recovery per worker; no descendant-cancellation model; unknown external side effects not represented.                     |
| Safe integration                  | Partial     | `orchestration.ConflictInput`/`DetectConflicts`; `AdmitWriteOwnership`; claim-only `Integrate`                                                                                                                         | No patch application; `OwnershipDisjointMultiWriter` has no non-test caller.                                                       |
| Durability and restart            | Partial     | SQLite store; attempts; `resume`; `executionlease`                                                                                                                                                                     | Single-active-task assumptions; whole-row attempt replacement; `run.New` clears attempts; lease not acquired by `sop orchestrate`. |
| Operator visibility               | Partial     | `sop status --detailed` (dependencies and attempt count); `sop report`; `internal/runtrace` types (`ReconstructOrchestrationGraph` has no production caller)                                                           | No task-graph command; no blocked-reason aggregate; orchestration graph unwired.                                                   |
| Adoption gate                     | Partial     | `internal/orchadopt`; `sop gate orchestration` returns REJECT                                                                                                                                                          | ADOPT is unreachable for a multi-unit case; wall-clock and token cost are declared unmeasured.                                     |
| Role profiles                     | Partial     | `agent.Capability` (Plan, DesignTests, Implement, DiagnoseFailure, Fix, Review); `orchestration.Role`                                                                                                                  | Missing only a configuration mapping; no new type needed.                                                                          |
| Dynamic expansion                 | Unnecessary | `docs/plans/BACKLOG.md` (Dynamic Worker Action Space entry): "No orchestration complexity is added ahead of that evidence"                                                                                             | Keep out of scope; do not re-propose without measured evidence.                                                                    |

**Already satisfied, must be preserved (not rebuilt):** lifecycle authority,
dependency resolution, cycle rejection, per-task retry budget, task completion
predicates, failure classification, and the default-off switch. MAO-005 is a
**preservation** requirement for the last item, not a request to build a switch.

## 4. Goals

1. **Governed delegation:** the SOP control plane assigns bounded work units with
   explicit responsibility, input, output, scope, and limits.
2. **Dependency-aware bounded concurrency:** independent work units may run
   concurrently within one leased SOP process, under a configurable bound;
   dependent work waits.
3. **Per-worker isolation:** no shared mutable harness or workspace between
   concurrent workers; existing sandbox, credential, and permission controls are
   preserved.
4. **Attributable results:** every outcome, error, and evidence reference is
   attributable to a workflow, work unit, and attempt.
5. **Durable, resumable execution:** interruption is reconciled before work
   resumes; completed gated actions are not replayed.
6. **Governance preservation:** SOP remains the only lifecycle authority; agents
   never approve, accept, merge, or widen their own scope.
7. **Operator visibility:** the task/work-unit graph, assignments, attempts,
   outcomes, and blocked reasons are reconstructable from persisted records.
8. **Backward compatibility:** single-agent and serial workflows keep working with
   no mandatory migration or new configuration.
9. **Honest measurement:** no speedup is claimed until a representative workload
   measures it; "not measured" is a recorded outcome, not a benefit.

## 5. Non-goals

- Replacing `internal/scheduler`, `internal/domain`, or `internal/orchestration`
  with another framework.
- Allowing a worker or model to change lifecycle state, approve, merge, or grant
  itself tools, paths, credentials, budget, or scope.
- Running multiple `sop` processes against one project. The execution lease stays
  a **single-process** invariant.
- **Non-goals.** Unbounded or recursive delegation; dynamic task expansion
  requires its own evidenced proposal, consistent with the BACKLOG's Dynamic
  Worker Action Space entry.
- Sharing a mutable harness, undo registry, or working directory between
  concurrent mutating workers.
- Applying patches or merging branches inside this PRD's first release.
- Claiming wall-clock, token, or cost improvements the adoption gate does not
  measure.
- Selecting a provider, model, transport, or orchestration library here.

## 6. Users and key workflows

**Primary user:** an operator running governed work through SOP.

**Workflow A — independent read-only analysis.** The controller selects two or more
`PLANNED` tasks whose dependencies are satisfied and whose capabilities are
non-mutating, and runs their analysis units concurrently in one leased process.
Results are claims; SOP verification remains authoritative.

**Workflow B — staged specialist roles.** A planning task produces bounded work.
Implementation artifacts feed review and verification. Governance decisions stay
with SOP and the authorized operator. v1 may express roles through existing
`agent.Capability` values only.

**Workflow C — interrupted execution.** A process exits mid-workflow. On restart,
SOP reconciles completed, failed, running/unknown, and pending work, and does not
blindly repeat a non-idempotent side effect.

**Workflow D — constrained local inference.** Work units may use independently
configured classes/providers, subject to the global bound. A per-endpoint bound is
deferred (MAO-009) and activates only after a recorded saturation incident; a local
model host must not be oversubscribed.

**Workflow E — serial fallback.** With orchestration disabled, or with one work
unit, `sop run` behaves exactly as it does today.

**Operator surface (must be specified before implementation):**
`sop run` remains the lifecycle entry point; `sop orchestrate` remains opt-in.
Whether multi-work-unit execution is reached through `sop orchestrate` or a new
`--parallel` flag on `sop run` is operator decision **D1**.

## 7. Requirements

**Priority.** P0 = required before any concurrency; P1 = required before relying
on concurrent mutating results (it may ship before or with that capability);
P2 = optional or deferred.

### MAO-001 — One leased execution owns the project (P0)

**Verified gap.** The execution lease enforces one owner per project but
`sop orchestrate` does not acquire it (`internal/cli/orchestrate.go`), while
`sop run` does (`internal/cli/drive.go`). Parallelism must be **inside** one leased
process; it must not become multiple processes.

**Requirement.** Any orchestrated execution acquires the existing project
execution lease **according to the table below** and releases it on every exit
path. Workers never acquire the lease. A stale lease is released only through the
existing `executionlease.ReleaseStale` path. Because the lease is off by default,
an orchestrated run with `N > 1` refuses when the lease is off or advisory; it
never falls back to serial and never creates an unleased execution path. Add
`KindOrchestrate` as an owner kind (reuse `ArgvDigest`) so `sop resume` names the
holder; do not reuse `KindRun`.

Lease modes are the existing `executionlease.Mode` values: `off` (default; no
lease held), `advisory` (contention recorded but not blocked), and `required`
(a competing execution is refused). The rule is:

| Command                          | `N` | Lease `off`        | Lease `advisory`           | Lease `required`  |
| -------------------------------- | --- | ------------------ | -------------------------- | ----------------- |
| `sop run` (single task or graph) | 1   | allowed, unchanged | acquire; contention logged | acquire or refuse |
| `sop orchestrate`                | 1   | allowed, unchanged | acquire; contention logged | acquire or refuse |
| `sop orchestrate` / `sop run`    | >1  | **refuse**         | **refuse**                 | acquire or refuse |

**Acceptance criteria.**

- A second orchestrated execution is refused with the existing `BusyError` text
  while the first holds the lease.
- With orchestration enabled, `N > 1`, and `execution.lease: off`, the run refuses
  with an actionable message instead of running serially.
- `sop orchestrate` and `sop run` cannot run concurrently on one project under
  `execution.lease: required`.
- Killing the holder leaves a stale lease that `sop resume` can report and
  release; no live lease is stolen.
- A test proves the lease is released on success, error, and cancellation.

### MAO-002 — Bounded multi-slot scheduling (P0)

**Verified gap.** `scheduler.Next` selects at most one task and treats any active
task as occupying the only slot (`internal/scheduler/scheduler.go`). The driver
loop and `resumeActiveTask` assume at most one in-flight task
(`internal/cli/drive.go`).

**Requirement.** Provide **one** shared eligibility predicate for "a task's
dependencies are satisfied and it may be selected" and a bounded batch adapter
over it. `scheduler.Next` keeps its single-slot contract and its existing
`PLANNED → READY` promotion. `NextBatch(ctx, N)` is **read-only**: it returns up to
`N` mutually independent eligible tasks without promoting or saving them, so an
analysis unit does not become an active task. `internal/parallel.Select` delegates
to the same predicate. Eligibility is: status `PLANNED` or `READY`, and
`ResolveDependencies` satisfied; `Next` additionally requires `PLANNED` because it
promotes. Selection order stays deterministic (lowest task ID first).

**Bound.** The graph bound is the existing
`EffectiveActiveWorkers(policy.Budget, policy)`, which is
`min(max_active_workers, max_concurrency)` floored at 1. `orchestration.max_parallel_tasks`
is an optional alias that populates `ExecutionPolicy.MaxConcurrency`; a zero value
leaves the existing resolution (1). Do not add a fourth limit or a literal cap; the
value cannot exceed `orchestration.max_workers`.

**Acceptance criteria.**

- `Next`, `NextBatch`, and `parallel.Select` share one predicate; a guard test
  fails if a caller re-implements it.
- `NextBatch` performs no write: it returns tasks without promoting or saving
  them, so a selected analysis unit is not `READY` and is not treated as in flight.
- `N=1`: selection and execution are behavior-identical to today, including
  recovery and blocked-task handling.
- `N=2` selects two independent tasks and never two dependent tasks.
- The effective bound is the existing `EffectiveActiveWorkers`; a larger
  `max_parallel_tasks` cannot widen it, and no separate cap is introduced.
- A diamond graph progresses across iterations; a one-shot batch cannot be the
  only selection mechanism.
- Existing scheduler regression and guardrail tests pass unchanged.

### MAO-003 — Per-work-unit and per-attempt identity (P0)

**Verified gap.** `domain.Task` has no workflow/parent ID, role, owner, or runtime
(`internal/domain/task.go`). `store.task_attempts` is keyed `(task_id, number)` and
carries no worker or execution identity (`internal/store/sqlite.go`). `run.New`
clears the attempt directory (`internal/run/run.go`).

**Requirement.** Persist a stable `work_unit_id` and `attempt` identity for every
orchestrated execution using **additive, defaulted columns** on the existing
`task_attempts` table (schema migration 8). Derive `workflow_id` from the existing
`plan_id`; do not store a second workflow identity that can drift. In v1 a work
unit is 1:1 with a plan task, so `work_unit_id` defaults to the task ID and no
second attempt table is introduced. The columns carry at least: task ID,
capability/role, provider, model, locality, worktree identity, attempt outcome,
start/finish time, failure code, and evidence references. The orchestration path
appends attempts through a dedicated `AppendAttempt` (`INSERT ... ON
CONFLICT(task_id, number) DO UPDATE`) and never routes them through `Store.Save`'s
delete-and-reinsert path. The existing `(task_id, number)` key is sufficient while
exactly one worker owns a task; a later decision that allows two workers on one
task requires a new discriminator (operator decision D3).

**Acceptance criteria.**

- A retry or role variant of one task is distinguishable by the new columns
  without changing the primary key.
- A single-agent run populates no new column and creates no new artifact; its task
  row, status, attempt count, and dependencies are unchanged.
- Interleaving scheduler promotion with an attempt append at `N=2` loses no attempt
  row.
- A v7 database opens and behaves unchanged; migration 8 is idempotent and
  resumable.
- No secret value is persisted; redaction runs before write.

### MAO-004 — Per-worker isolation (P0)

**Verified gap.** `sop orchestrate` builds one `agent.Agent` and dispatches
concurrent non-mutating assignments through it
(`internal/cli/orchestrate.go`, `internal/orchestration/coordinator.go`). The
harness is documented as unsafe for concurrent calls
(`internal/toolharness/harness.go`). `internal/parallel` creates per-task
worktrees but has no production caller (`internal/parallel/parallel.go`).

**Requirement.** Every concurrent worker gets its own agent/harness instance and
its own authorized root. Non-mutating units run under `toolharness.RootReadOnly`,
which refuses write tools and repository-mutating shell commands; mutating units
run in their own Git worktree with a declared write scope. Do not share a
harness, undo registry, or working directory between concurrent workers. A
read-only unit that must run build or test commands gets a worktree as well; that is
a cost choice, not a substitute for the read-only profile. The default write-ownership
policy remains `OwnershipSingleWriter`; disjoint multi-writer is reachable only by
explicit configuration and proven-disjoint scopes.

**Acceptance criteria.**

- Two concurrent workers cannot observe or undo each other's file changes in a
  test.
- A non-mutating unit runs under `RootReadOnly`; a write attempt or a
  repository-mutating shell command is refused.
- A mutating unit runs in its own worktree; removal preserves `.agent-sdlc`
  lifecycle evidence under a per-work-unit archive identity.
- An overlapping write scope is rejected before dispatch, independent of
  assignment order.
- With `N=1`, no second harness or worktree is created.

### MAO-005 — Default-off backward compatibility (P0)

**Verified gap.** `orchestration.enabled` is false by default
(`internal/config/config.go`); `internal/parallel` is unwired; `sop run` drives the
graph serially (`docs/specs/EXECUTION.md` §8).

**Requirement.** Concurrency is opt-in. An omitted `orchestration` block, a config
without new keys, and a single work unit produce the current behavior and
artifacts. No new top-level configuration key is introduced in the first release;
new knobs live under the existing `orchestration:` block with conservative
defaults. Two version numbers are distinct: the configuration schema version
(`config.CurrentVersion`) and the SQLite schema version (`store.schemaVersion`).
Adding a configuration key makes an older SOP reject a config that contains it
(unknown fields are errors).

**Acceptance criteria.**

- A v1 config with no orchestration keys parses and runs identically.
- An older binary rejects a config that adds a new key; the release notes state
  the minimum binary.
- The SQLite migration is additive and keeps older databases readable by the new
  binary.
- With orchestration disabled, no orchestration, parallel-runner, lease, or new
  trace code is invoked.
- The existing single-agent lifecycle, recovery, and gate tests pass unchanged.

### MAO-006 — Deterministic failure, cancellation, and recovery (P0)

**Verified gap.** `FailureCode`, `recovery.Decide`, retry budget, and `resume`
exist. Parallel worker goroutines have no panic recovery, and there is no
descendant-cancellation or "uncertain external side effect" state.

**Requirement.** Every worker outcome maps to a stable failure code. A panic in
one worker is recovered, recorded, and does not discard sibling outcomes or kill
the process. Cancellation stops new selection, cancels in-flight contexts, and
leaves durable state resumable. Because `NextBatch` does not promote, a cancelled
analysis unit leaves its task `PLANNED`/`READY` and selectable again. An `unknown`
outcome is recorded on the attempt row; when a task status must change, it maps to
the existing `BLOCKED` state with a **recoverable** `BlockedReason`. No new status
is added. Before mutating concurrency ships, a durable dispatch binding and
multi-active `resumeActiveTask` are required; while mutating units stay serial,
that work is deferred with MAO-008.

**Acceptance criteria.**

- A panicking worker yields a typed failure and siblings still report.
- Cancellation mid-batch leaves every started analysis unit with a persisted
  outcome and no task stuck in an active status.
- An unknown outcome is recorded and is either recoverable through
  `BlockedReason` or escalated; it is never success.
- Mid-batch budget exhaustion stops new selection, records every started unit's
  outcome (including `unknown`), and leaves state resumable.
- A durable dispatch binding is required before any mutating unit can be in flight
  concurrently; until then the driver serializes mutating units.
- A retry after a partial external side effect is refused or escalated to a human.
- Recovery decisions reuse `internal/recovery` and `internal/resume`; no second
  policy is added.

### MAO-007 — Provenance, attribution, and redaction (P0)

**Verified gap.** `internal/runtrace` has sanitizers and graph reconstruction, but
`ComposeOrchestrationEvents` has no production caller; `sop orchestrate` writes no
run directory; `perf.Usage` is not attached to its context; per-task token usage is
recorded only on the `sop run` path.

**Requirement.** An orchestrated run writes a durable run directory and a trace
that records each work unit and attempt, including provider/model/locality, start
and completion, outcome, failure code, and evidence references. Token usage is
attributed to the same identity, or explicitly recorded as unreported. All
worker-supplied text is redacted before persistence and before stdout. Mid-batch
budget exhaustion stops new selection and records every started unit; it does not
silently discard a sibling's claim.

**Acceptance criteria.**

- A run's work-unit graph is reconstructable from persisted records alone.
- `sop orchestrate` leaves a run directory even when it fails.
- A configured token budget either enforces correctly or refuses the run; it never
  fails every worker call because no usage collector is attached.
- A budget exhausted mid-batch is visible in the run record, and started units have
  a recorded outcome.
- The `runtrace.Collector` iteration drop is surfaced as an explicit truncation
  flag; a truncated trace is never presented as complete.
- A secret-shaped `Reason` or `Findings` value does not appear in persisted or
  printed evidence.

### MAO-008 — Patch integration and stale-base detection (P2, deferred)

**Verified gap.** Conflict detection and single-writer admission exist, but
integration is claim-only and no patch is applied (`internal/orchestration/integrate.go`).
`OwnershipDisjointMultiWriter` has no production caller. While mutating units are
serialized and no patch is applied, there is no concurrent mutation to integrate.

**Requirement (deferred).** Do not add patch application or stale-base detection
until a patch path exists and mutating parallelism is authorized. In v1, integration
consolidates claims only, and overlapping write scopes are refused before dispatch
(MAO-004). If patch integration is added, it is one serialized lane; no
last-writer-wins merge is permitted.

**Activation trigger.** An approved plan that applies concurrent patches.

**Acceptance criteria (when activated).**

- Two incompatible proposed changes cannot both be applied silently.
- A stale base identity is detected and reported before integration.
- No new lifecycle status is introduced outside `internal/domain`.

### MAO-009 — Per-endpoint capacity limits (P2, deferred)

**Verified gap.** The global orchestration bounds exist
(`OrchestrationBudget`, `ExecutionPolicy.MaxConcurrency`), but no per-provider or
per-endpoint concurrency limit exists; `Providers` config carries only endpoints.
No measurement currently shows oversubscription of one local endpoint.

**Requirement (deferred).** Do not add a per-endpoint limiter or a work queue until
a recorded measurement names a real oversubscription incident. If one exists,
express the endpoint capacity as a value consumed by the existing global bound, or
as a selection predicate that simply does not select a saturated endpoint. No new
admission subsystem.

**Activation trigger.** A recorded measurement of one endpoint saturating under the
global bound.

**Acceptance criteria (when activated).**

- The capacity source and its safe default are documented.
- Saturation delays selection; it does not create a second scheduler or queue.
- Zero or malformed values resolve to a documented safe default, never unlimited.

### MAO-010 — Operator visibility (P1)

**Verified gap.** `sop status --detailed` shows dependencies and attempt counts,
and `sop report` exists. There is no task-graph view, no blocked-reason aggregate,
and the `runtrace` orchestration graph has no production caller.

**Requirement.** An operator can see the workflow/work-unit graph, dependencies,
assignment, attempt, outcome, blocked reason, and key timings through an existing
reporting surface, preferably `sop status --detailed` or `sop report`. The output
is redacted and machine-readable on request.

**Acceptance criteria.**

- At least one command reconstructs the graph and names the blocking predecessor for
  a blocked unit.
- The output renders the `internal/domain` task status plus the attempt outcome; it
  does not invent a `queued`/`running` lifecycle.
- No sensitive plaintext is required to reconstruct the graph.

### MAO-011 — Role profiles (P2, deferred)

**Verified gap.** `agent.Capability` and `orchestration.Role` already exist; only a
configuration mapping is absent.

**Requirement (deferred).** If a concrete mapping request exists, permit a role name
to map to an existing capability and model class through configuration. Do not
introduce a new provider-specific role type.

**Activation trigger.** An operator needs distinct planner/implementer/reviewer
classes that the existing `agent.class` selection cannot express.

**Acceptance criteria (when activated).**

- A role profile changes only capability/class mapping, not lifecycle authority.
- An unknown role or capability fails validation.
- Removing the profile restores current behavior.

### MAO-012 — Benefit measurement and adoption (P1, operator-gated)

**Verified gap.** `sop gate orchestration` measures twelve deterministic
dimensions over `orchadopt.Corpus` (a built-in set of independently parallelizable
tasks) and compares a single-agent baseline against a multi-agent candidate. It
returns REJECT. Its ADOPT branch requires the candidate to use no more workers or
tool calls than the baseline, which a multi-unit decomposition cannot satisfy;
wall-clock and token cost are declared unmeasured
(`internal/orchadopt/orchadopt.go`, `docs/reports/PHASE-9-MULTI-AGENT-ADOPTION.md`).

**Requirement.** Treat `sop gate orchestration` as a **safety/cost gate** for this
PRD, not a benefit benchmark: it may reject a candidate, but it never justifies
concurrency. A benefit claim requires a separate, operator-approved recorded
measurement that names the workload, provider, model, budget, and repetitions, and
records elapsed time, token usage, failures, and review effort. "Not measured"
remains a valid outcome.

**Acceptance criteria.**

- The gate's output never claims a benefit; it reports unmeasured dimensions
  explicitly.
- A recorded measurement names the workload, provider, model, budget, and
  repetition count.
- No default flips on an unmeasured dimension.

### MAO-013 — Dynamic task expansion (deferred; not a requirement)

Deferred by the BACKLOG's Dynamic Worker Action Space entry until measured usage
shows a limitation. Dynamic task expansion is not authorized by this PRD; it
requires its own evidenced proposal.

## 8. Security and trust invariants

- Agent-provided plans, messages, and artifacts are **untrusted data**, never
  controller instructions.
- A worker cannot grant itself tools, paths, credentials, budget, scope, or
  approval authority. Capability and scope admission are harness decisions
  (`internal/orchestration/ownership.go`, `internal/toolharness`).
- Model routing reuses `internal/agent`, `internal/model`, and the existing
  locality/confidentiality policy. A model name is not a security boundary.
- Cross-worker context exchange is explicit, minimal, attributable, and policy
  checked; no shared transcript by default.
- Every persistence and stdout surface redacts worker-supplied text using the
  existing `internal/redact` policy; redaction is a chokepoint, not a per-writer
  convention.
- A mutating worker has exactly one write scope and one worktree. Overlap is
  rejected before dispatch.
- The project execution lease is the outer exclusion boundary. Workers never hold
  it; a second SOP process is refused while it is held.
- Side-effecting work needs an explicit idempotency/reconciliation policy;
  unknown external state is escalated, never retried blindly.

## 9. Conceptual model (reuse first)

- **Lifecycle authority:** `internal/domain` only. The controller _requests_
  transitions; it never assigns status.
- **Task:** existing `domain.Task` with its dependencies, attempts, and predicates.
- **Work unit:** a read-only projection of a task plus one capability, expressed
  today as `orchestration.WorkUnit` / `WorkAssignment`; not a new persisted entity
  unless MAO-003 requires one.
- **Agent execution:** one `agent.Agent` call through a per-worker harness with a
  bounded request.
- **Integration:** a controller-managed, serialized step that validates claims;
  claim-only in v1.
- **Review/verification:** existing review and verification operations; reviewer
  output is evidence, not approval.
- **Scheduler:** the existing `internal/scheduler` extended with a bounded batch
  selection contract; not replaced.
- **Executor:** `internal/parallel` for per-task worktrees and bounded execution;
  not a second scheduler.
- **Lease:** `internal/executionlease` as the single-process exclusion boundary.

## 10. Architecture decisions to evaluate

The preliminary recommendation is recorded; the architecture document owns the
full comparison.

1. **Extend the scheduler or add orchestration?** Extend `internal/scheduler` with
   a bounded batch adapter over **one** shared eligibility predicate. Put the
   predicate in `internal/domain` as a pure function so `Next`, `NextBatch`, and
   `parallel.Select` share one implementation without inverting package layering.
   Keep `internal/orchestration` as the non-mutating coordination model. Do not add
   a framework. Phase 9's single-lifecycle-authority invariant forbids a second
   authority.
2. **Task-level parallelism or role pipelines?** One task graph with role metadata.
   A role pipeline is a dependency edge, not a second scheduler.
3. **Worktrees or another isolation mechanism?** Use the existing
   `toolharness.RootReadOnly` profile for non-mutating units and
   `internal/parallel.GitWorkspaces` for mutating units. A read-only unit that must
   run build or test commands gets a worktree as well. Fix the per-plan evidence
   archive collision first.
4. **Persistence and attempt identity.** Derive `workflow_id` from `plan_id`; add
   defaulted attribution columns (including `work_unit_id` and attempt outcome) to
   `task_attempts`. Do not add a second attempt table in v1; a work unit is 1:1 with
   a task. The `(task_id, number)` key is sufficient while one worker owns a task. A
   later decision that allows two workers on one task needs a discriminator.
5. **Context distribution.** Reuse `BuildWorkerContext` and explicit input
   references; no transcript sharing.
6. **Output integration.** Claim-only in v1; one serialized integration lane if
   patches are added. No last-writer-wins.
7. **Scheduling fairness and provider limits.** Deterministic lowest-ID order and
   the existing `EffectiveActiveWorkers` bound. `max_parallel_tasks` is an optional
   alias for `MaxConcurrency`, not a fourth limit. A per-endpoint bound is deferred
   until a measured saturation incident; if added, it is a capacity value or
   selection predicate, not a queue.
8. **Human approval placement.** No new approval for read-only analysis; explicit
   approval for enabling mutating parallelism, widening scope, or changing the
   default. The operator decisions are listed in §15.

## 11. Alternatives considered

| Alternative                                                      | Why it was rejected                                                                                                                                                                                                                                                          |
| ---------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Stay serial and fix only the honesty gaps (lease, usage, trace)  | Retained as Phase MAO-A and may ship independently. It is the smallest change, but it does not close the dependency-aware concurrency gap that motivates this PRD.                                                                                                           |
| Build a new multi-agent orchestration framework                  | Violates the single-lifecycle-authority invariant and `docs/specs/EXECUTION.md` §10 ("wire or drop them deliberately rather than add a parallel implementation"). The existing model already covers contracts, decomposition, ownership, conflict, budgets, and integration. |
| Run multiple `sop` processes, one per task                       | Contradicts the execution lease's single-owner contract and the single-writer SQLite store; produces lost updates and duplicate side effects.                                                                                                                                |
| Reuse `internal/parallel.Runner` unchanged as the execution path | It is a one-shot batch with no lease, budget, provenance, recovery, or loop; it returns after one `wg.Wait()` and cannot progress a diamond graph. It remains an executor with a shared eligibility predicate.                                                               |
| Make `internal/parallel` the scheduler                           | Same defect: a one-shot selector cannot own the bounded graph loop or the recovery policy.                                                                                                                                                                                   |
| Add a `CONFLICT`/`RECONCILING` task status                       | Adds a second lifecycle vocabulary; `internal/domain` is the only transition authority. Use `BLOCKED` plus an existing/new `BlockedReason`, or a non-lifecycle record.                                                                                                       |
| Enable `OwnershipDisjointMultiWriter` by default                 | No production caller today; conservative overlap detection would refuse many sets. Keep single-writer default and make multi-writer an explicit opt-in after evidence.                                                                                                       |
| Flip `orchestration.enabled` to true now                         | The adoption gate returns REJECT and cannot measure the dimensions that would justify it. A default flip would be an unmeasured behavior change.                                                                                                                             |
| Implement dynamic task expansion now                             | It is not authorized by this PRD and requires its own evidenced proposal, consistent with the BACKLOG's Dynamic Worker Action Space entry.                                                                                                                                   |

## 12. Rollout phases

Phase 0 (this document and its review) is complete when the EGM review and
operator decisions are recorded. No production change occurs in Phase 0.

**Phase MAO-A — Honest, safe opt-in path (no new concurrency; MAOP-001).**
Acquire the execution lease in `sop orchestrate`; attach per-call usage accounting;
write a run directory and orchestration trace; keep concurrency at 1 until
per-worker isolation exists; document the analysis-only scope. This phase fixes
live defects on the already-enabled opt-in path and **may proceed without any
operator decision**. Exit: the opt-in path is leased, attributed, and debuggable,
with unchanged single-agent behavior.

**Phase MAO-B — Attempt attribution and bounded selection (MAOP-002).**
Add the additive attribution-column migration and `AppendAttempt`; add the read-only
bounded batch adapter over the shared eligibility predicate; keep single-slot
behavior as the default and in tests. Exit: a two-task independent graph completes
under `N=2` with distinct attempts and a resumable state; `N=1` is behavior- and
artifact-identical (same task fields, same artifacts). **Gate:** D1; D4 is
resolved to one process with N slots.

**Phase MAO-C — Operator visibility (MAOP-003).**
Render the task/workflow graph, attempts, blocked reasons, and key timings through
`sop status --detailed` / `sop report`, using the existing claim consolidation. No
patch application. Exit: an operator reconstructs the graph and names the blocking
predecessor from persisted records. **Gate:** D10 (optional; the graph view may be
deferred without blocking execution).

**Phase MAO-D — Isolated mutating execution (MAOP-004).**
Per-worker agent/harness; `RootReadOnly` for non-mutating units; one worktree per
mutating unit; per-work-unit evidence archive; durable dispatch binding; panic
recovery, cancellation, and multi-active resume. Exit: two isolated mutating units
cannot clobber each other; conflicting scopes are refused before dispatch. **Gate:**
D2 and D7. If D2 selects analysis-only v1, this phase is deferred.

**Phase MAO-E — Benefit measurement (MAOP-005).**
Keep `sop gate orchestration` as a safety/cost gate. Run a separate,
operator-approved measurement of a representative workload and record elapsed
time, tokens, failures, and review effort. Exit: a recorded measurement; default
changes only by explicit operator decision. **Gate:** D6.

Do not create implementation phases for functionality the baseline proves already
complete.

## 13. Validation and acceptance scenarios

1. **Legacy path.** Existing single-agent `sop run` executes with unchanged
   lifecycle, artifacts, and exit codes.
2. **Serial equivalence.** With `N=1`, the new selection contract is behavior- and
   artifact-identical to `scheduler.Next`.
3. **Dependency block.** A dependent task never starts before its predecessor is
   satisfied.
4. **Bounded concurrency.** No more than `N` independent tasks run at once; a
   diamond graph progresses across iterations. `N` is the existing
   `EffectiveActiveWorkers` bound; no separate cap is added.
5. **Isolation.** Two concurrent workers cannot read or undo each other's changes;
   a mutating worker has its own worktree.
6. **Lease exclusion.** A second orchestrated execution is refused while the first
   holds the lease; a stale lease is reported and released without stealing a live
   one.
7. **Write conflict.** Two incompatible proposed changes are refused or surfaced
   for reconciliation; no silent overwrite.
8. **Authorization boundary.** An agent attempts to approve/merge or widen scope;
   the controller refuses absent established authorization.
9. **Injection boundary.** Untrusted output containing policy-override
   instructions does not affect scheduling or grants.
10. **Interruption.** Restart reconciles in-flight and unknown attempts; completed
    gated actions are not replayed.
11. **Retry safety.** A non-idempotent side effect is not repeated without
    reconciliation.
12. **Capacity (deferred with MAO-009).** When MAO-009 is activated, configured
    per-endpoint concurrency prevents overload of a constrained local endpoint.
    This scenario is inactive while MAO-009 is P2.
13. **Cleanup.** Worktree removal preserves ignored `.agent-sdlc` evidence and
    unrelated local changes.
14. **Observability.** A persisted run reconstructs the workflow graph and outcome
    without sensitive plaintext.
15. **Token budget.** A configured budget is enforced or the run refuses; it does
    not fail every worker call for lack of a usage collector.

Acceptance requires evidence from actual repository tests and supported
environments. A proposed scenario is not a claim that its test exists.

## 14. Risks and mitigations

| Risk                                   | Mitigation                                                                                                                               |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Lost updates from concurrent writers   | One leased process; bounded slots; append-only attempt writes; store single-writer; no whole-collection replacement on concurrent paths. |
| Shared harness corruption              | Per-worker agent/harness and root; concurrency stays at 1 until isolation is proven.                                                     |
| Unauthorized autonomy                  | `internal/domain` transition authority; scope/capability admission; agent output is untrusted data.                                      |
| Credential/context leakage             | Per-worker context; redaction chokepoint; negative tests for findings and reasons.                                                       |
| Unrecoverable partial external actions | Attempt states, reconciliation before retry, human escalation for unknown outcomes.                                                      |
| Local model exhaustion                 | Global bound now; per-endpoint bound deferred until a measured saturation incident (MAO-009, D9).                                        |
| Evidence loss on cleanup               | Per-work-unit archive identity; `worktreeevidence` preservation; never convert a succeeded task into a failed one.                       |
| Mid-batch budget exhaustion            | Stop new selection; record every started unit's outcome; leave state resumable; never discard a sibling's claim.                         |
| Trace truncation under fan-out         | Surface the `runtrace.Collector` iteration drop as an explicit truncation flag; do not present a truncated trace as complete.            |
| Unmeasured benefit claim               | Gate result and "not measured" recorded; no default change without a measurement.                                                        |
| Scope creep into dynamic expansion     | Explicitly out of scope; it requires its own evidenced proposal.                                                                         |

## 15. Operator decisions required

These are genuine authorization or design choices, not repository questions.

- **D1 — Entry point.** Does multi-work-unit execution live behind `sop orchestrate`,
  a new `sop run --parallel-units N` flag, or both? Recommendation: extend `sop run`
  with a default-1 bound and keep `sop orchestrate` as the analysis-only opt-in. Do
  not name it `--parallel`, which reads like the existing `--max-tasks`.
- **D2 — First concurrency scope.** Is v1 limited to non-mutating analysis, or does
  it include isolated mutating work? Recommendation: non-mutating first.
- **D3 — Persistence shape (resolved in this revision).** Additive nullable columns
  on `task_attempts`, with `work_unit_id = task_id` in v1. No second attempt table.
  Confirm only if you disagree; a model that permits two workers on one task needs
  a discriminator and a new decision.
- **D4 — Lease model (resolved in this revision).** One process with N in-memory
  slots; no counted lease. The SQLite store and the repository are single-owner
  resources. Confirm only if you want a counted lease instead.
- **D5 — Patch integration (deferred).** Claim-only v1; patch application and
  stale-base detection wait for an approved plan that applies concurrent patches.
  Confirm the deferral.
- **D6 — Adoption.** Keep `orchestration.enabled` false and the gate as a safety
  gate until a measured workload is approved? Recommendation: yes.
- **D7 — Evidence archive.** Per-work-unit archive identity, or merge into the
  existing per-plan archive? Recommendation: per-work-unit, with legacy archives
  still readable.
- **D8 — Token budget.** Require per-worker usage accounting before enabling
  concurrency, or treat a configured budget as a reason to refuse concurrency?
  Recommendation: both; refuse rather than run unmeasured.
- **D9 — Per-endpoint capacity (deferred).** Do not add a limiter or queue until a
  recorded measurement shows endpoint saturation. Confirm the deferral.
- **D10 — Operator visibility.** Ship the task/workflow graph in
  `sop status --detailed` / `sop report`, or defer it? Recommendation: ship it in
  Phase MAO-C; it is read-only and does not block execution.

## 16. EGM review record

The completed review artifacts are under
[PRD-MULTI-AGENT-ORCHESTRATION-REVIEW](../plans/PRD-MULTI-AGENT-ORCHESTRATION-REVIEW/):

- `00-review-context.md` — reading list and reviewer rules.
- `01-concepts-brief.md` — brainstorm synthesis across the seven lenses.
- `02-zoom_out_report.md`, `03-comprehension_report.md`,
  `04-critic_report.md`, `05-intermediate_zoom_out_report.md`,
  `06-implementation_readiness_report.md` — review stages.
- `07-consolidated_review_report.md` — deduplicated findings, resolutions, and
  outstanding decisions.
- `08-verification-evidence.md` — commands run and observed results.

All reviewers ran in fresh contexts, read only, and did not edit the repository.
The report records `not run` for any reviewer that did not complete; none did.

## 17. Definition of done for this PRD stage

- [x] Existing capabilities and gaps verified against repository source with
      references.
- [x] Requirements reconciled; redundant and satisfied work removed.
- [x] Requirement IDs no longer collide with delivered Phase 9 IDs.
- [x] Security and SOP authority invariants restated against the real code.
- [x] Minimal architecture selected with documented alternatives.
- [x] Fresh-context EGM review completed and recorded.
- [x] Risks, decisions, and acceptance cases reviewed.
- [x] Implementation plan created in the repository's plan format.
- [ ] Operator decisions D1, D2, D5–D10 resolved or accepted as deferred (D3 and
      D4 are resolved in this revision).
- [x] Documentation-only PR opened and linked; no implementation or merge implied:
      <https://github.com/imhttran/agentic-sop/pull/645> (draft).

---

**Decision requested after review:** approve or revise the architecture, the
first concurrency scope, and the phased implementation plan before any production
work begins.
