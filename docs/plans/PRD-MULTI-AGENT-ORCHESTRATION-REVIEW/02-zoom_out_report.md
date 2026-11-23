# Goldfish Review Report — Stage 5 (Initial Zoom Out)

**Target:** `docs/requirements/PRD-MULTI-AGENT-ORCHESTRATION.md` (full file)
**Mode:** REVIEW, Stage 5
**Date:** 2026-11-23

## Reviewer Status

| Reviewer | Status |
| --- | --- |
| `egm-zoom-out` | completed |
| `egm-inventive-catalyst` | completed |

Severity breakdown after deduplication: 0 Blocker, 11 Major, 3 Nit.

## Prioritized Findings

### Requirement IDs collide with delivered Phase 9 IDs — Major

**Lens:** `egm-zoom-out`, `egm-inventive-catalyst`, `egm-comprehension`
**Scope:** §6 requirements table
**Finding:** `ORCH-001` … `ORCH-016` reuse the IDs of the shipped Phase 9 tasks
with different meanings (`PHASE-9-MULTI-AGENT-ORCHESTRATION.md:16`). Every future
reference to `ORCH-005` or `ORCH-012` is ambiguous.
**Resolution:** Renumbered to `MAO-001` … `MAO-013`; Phase 9 IDs are cited as
history only.

### Discovery work is already done three times — Major

**Lens:** `egm-zoom-out`
**Scope:** §9 Phase 0, §12 open questions
**Finding:** The Phase 9 plan, adoption report, and `docs/specs/EXECUTION.md` §8
already record the inventory. Questions such as "which package is the scheduler"
are answered by `internal/scheduler` and `driveGraph`.
**Resolution:** Phase 0 is replaced by the verified baseline in the PRD; open
questions are reduced to operator decisions.

### `internal/parallel` already implements bounded worktree execution — Major

**Lens:** `egm-zoom-out`, `egm-inventive-catalyst`
**Scope:** §9 Phase 2, ORCH-005/006
**Finding:** `internal/parallel.Runner` has a bounded semaphore, dependency-aware
`Select`, and per-task `GitWorkspaces`. It has no production caller. It is a
one-shot batch, not a scheduler.
**Resolution:** Positioned as the executor; selection is a separate read-only
adapter; no second scheduler.

### Four requirements are already satisfied — Major

**Lens:** `egm-zoom-out`
**Scope:** ORCH-003/004/008/011
**Finding:** `domain.Task` carries ID, dependencies, attempts, acceptance checks,
and required evidence; `taskbuilder.ValidateDAG` rejects cycles; `resume` and
`recovery` handle restart; `FailureCode` classifies failures.
**Resolution:** Reclassified as Implemented or Partial; the remaining gaps are
narrowed to attribution, concurrency safety, and persistence.

### Provenance types exist but the producer is unwired — Major

**Lens:** `egm-zoom-out`, `egm-codebase-details`
**Scope:** ORCH-007/013
**Finding:** `ComposeOrchestrationEvents` and `ReconstructOrchestrationGraph` have
no production caller; `sop orchestrate` writes no run directory.
**Resolution:** MAOP-001 wires the producer and the run directory; MAOP-003 renders
the graph by reusing the existing types.

### The opt-in path never expresses more than one candidate — Major

**Lens:** `egm-zoom-out`
**Scope:** §5 Workflow A, ORCH-011
**Finding:** `orchestrateRequest` builds one `Plan` candidate with an empty scope,
so two independent implementations are unreachable.
**Resolution:** MAOP-002 generalizes selection; MAO-004 permits read-only
concurrency first, mutating units later.

### The adoption gate cannot justify concurrency — Major

**Lens:** `egm-inventive-catalyst`
**Scope:** MAO-012 (was ORCH-015)
**Finding:** ADOPT requires `candidate.WorkerCount <= baseline.WorkerCount` and
`ToolCalls <= baseline`, while a multi-unit decomposition emits one worker per
unit. Wall-clock and token cost are declared unmeasured. REJECT is a tautology.
**Resolution:** The gate is a safety/cost gate only; benefit requires a separate
operator-approved measurement record. No default flips on an unmeasured dimension.

### Two attempt histories would be introduced — Major

**Lens:** `egm-inventive-catalyst`
**Scope:** MAO-003, architecture persistence
**Finding:** A `work_units`/`work_unit_attempts` pair would be deleted by
`ClearTasks`' cascade just like `task_attempts`, so it buys no survival and adds a
second vocabulary every reader must union.
**Resolution:** Additive, defaulted attribution columns on `task_attempts`;
`domain.Attempt`, `insertAttempts`, and `loadAttempts` must all carry them.

### A per-endpoint limiter and queue are speculative — Major

**Lens:** `egm-inventive-catalyst`, `egm-zoom-out`
**Scope:** MAO-009
**Finding:** No measurement shows one endpoint saturating. A second admission
subsystem contradicts the "no complexity ahead of evidence" rule.
**Resolution:** Deferred with an activation trigger; if activated, it is a capacity
value or selection predicate, not a queue.

### An `unknown` outcome has no lifecycle mapping — Major

**Lens:** `egm-inventive-catalyst`
**Scope:** MAO-006
**Finding:** The domain has no `UNKNOWN` status; the PRD required reconciliation
without saying how.
**Resolution:** Record `unknown` on the attempt and map a required task change to
`BLOCKED` with a recoverable `BlockedReason`; no new status.

### The trace bound is partly manufactured — Nit

**Lens:** `egm-inventive-catalyst`
**Scope:** §14 trace risk
**Finding:** `runtrace.Collector` drops iterations beyond `maxIterations` silently;
`Trace.Context.Truncated` covers harness context, not iteration loss. The real
issue is silent truncation, not an invented event-count contradiction.
**Resolution:** MAOP-001 surfaces the iteration drop as an explicit flag.

### The selection predicate would be a third copy — Nit

**Lens:** `egm-inventive-catalyst`
**Scope:** MAO-002
**Finding:** `Next`, `NextBatch`, and `parallel.Select` would each implement
eligibility; the copies already disagree on `PLANNED` vs `READY`.
**Resolution:** One pure predicate in `internal/domain`; `Next` adds its promotion
check; `NextBatch` is read-only.

### "Add bounded parallelism" has no single owner — Major

**Lens:** `egm-structural`
**Scope:** §9 Phase 2
**Finding:** Three partial implementations exist (`scheduler`, `orchestration`
coordinator, `parallel`) and `EXECUTION.md` §10 requires a deliberate choice.
**Resolution:** `scheduler` selects, `orchestration` coordinates non-mutating
claims, `parallel` executes worktrees; one owner per responsibility.

### The controller wording would create a second authority — Major

**Lens:** `egm-structural`, `egm-inventive-catalyst`
**Scope:** §7 conceptual model
**Finding:** "The controller owns scheduling, state transitions, durable records"
contradicts `internal/domain` being the sole transition authority.
**Resolution:** The controller *requests* transitions; only `internal/domain`
decides them. The terminology section names the existing packages.

## Pairs with no false compromise

- One leased process vs. concurrency: genuine invariant; one process with N slots
  resolves it.
- Single lifecycle authority vs. controller: genuine invariant; the wording was
  corrected.
- Worktree isolation vs. integration cost: no false compromise; mutating work is
  serial and integration is claim-only in v1.
