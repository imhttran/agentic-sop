# Concepts Brief: Native Multi-Agent Orchestration

* **Author:** Tom Tran
* **Date:** 2026-11-23
* **Mode:** EGM BRAINSTORM (seven lenses)
* **Target concept:** `docs/requirements/PRD-MULTI-AGENT-ORCHESTRATION.md`

## Overview

The concept is governed multi-agent orchestration for SOP: one leased SOP process
running a bounded number of independent work units, each with its own execution
identity, isolation, persistence, and evidence. SOP remains the only lifecycle
authority; single-agent and serial behavior stay the default.

The brainstorm established that most of the proposed orchestration model already
exists in Phase 9 (`internal/orchestration`, ORCH-001 … ORCH-012). The real gap is
the connection to the lifecycle driver: the opt-in path is not leased, not
accounted, not durable, not observable, and not concurrency-safe.

## Analysis

Seven fresh-context lenses reviewed the concept read-only. The strongest
convergent findings:

1. **Requirement IDs collided with delivered work.** The first draft reused
   `ORCH-001` … `ORCH-016`, which are the IDs of the shipped Phase 9 tasks. This
   revision renumbers them `MAO-001` … `MAO-013`.
2. **Most requirements were already implemented or partially implemented.** The
   task contract, dependency resolution, cycle rejection, retry budget, failure
   classification, ownership, conflict detection, budgets, and integration view
   exist. The PRD now classifies each area and narrows the work.
3. **The opt-in path has live defects.** `sop orchestrate` does not acquire the
   execution lease, passes `context.Background()` so token accounting is absent,
   writes no run directory, and shares one `agent.Agent`/harness across parallel
   workers while the harness documents itself as unsafe for concurrent calls.
4. **The adoption gate cannot justify concurrency.** Its ADOPT branch requires the
   candidate to use no more workers or tool calls than the baseline, which no
   multi-unit decomposition can satisfy; wall-clock and token cost are declared
   unmeasured. REJECT is a safety/cost result, not evidence about economics.
5. **The persistence and lease models assume one active task.** `scheduler.Next`
   is single-slot; `Store.Save` replaces attempt rows; `run.New` clears attempts;
   `resumeActiveTask` refuses more than one active task. Concurrency must be
   designed against these, not layered on top.
6. **A new orchestration layer is unnecessary and forbidden.** The
   single-lifecycle-authority invariant and `docs/specs/EXECUTION.md` §10 require
   extending existing components rather than adding a parallel implementation.
7. **Per-endpoint capacity and role profiles are speculative.** No measurement
   shows local-endpoint saturation, and role profiles need only a config mapping.
   Both are deferred with activation triggers.

## Goals vs. Non-Goals

* **In-scope goals:** a leased orchestrated execution; bounded, dependency-aware
  selection; per-worker isolation; durable attempt attribution; provenance and
  redaction; deterministic failure/cancellation/recovery; operator visibility;
  backward compatibility; accurate benefit measurement.
* **Out-of-scope non-goals:** a new framework; multiple `sop` processes; unbounded
  or recursive delegation; shared mutable harness/workspace; automatic patch
  application or merge; default-on adoption; dynamic task expansion.

## Alternatives Weighed

| Alternative | Reason rejected |
| --- | --- |
| Stay serial and fix only the opt-in defects | Retained as Phase MAO-A and may ship independently. It does not close the dependency-aware concurrency gap. |
| New orchestration framework | Duplicates a verified model and violates the single-lifecycle-authority invariant and `EXECUTION.md` §10. |
| Multiple `sop` processes with a counted lease | Requires lease schema, heartbeat, and cross-process store coordination for an unmeasured gain. |
| `internal/parallel.Runner` as the scheduler | One-shot batch; no loop, lease, budget, provenance, or recovery. |
| New `CONFLICT` lifecycle status | Second lifecycle vocabulary; use `BLOCKED` + `BlockedReason`. |
| New attempt tables | A work unit is 1:1 with a task; additive columns on `task_attempts` are sufficient. |
| Per-endpoint limiter/queue now | No measured saturation incident; defer. |

## Key Decisions & Staging

- **One process, one lease, N in-memory slots.** The lease is not extended to
  counted slots.
- **`NextBatch` is read-only.** It must not promote tasks, or a cancelled batch
  becomes an unrecoverable `READY` task that resume routes to `CreateBranch`.
- **One shared eligibility predicate in `internal/domain`.** `Next`, `NextBatch`,
  and `parallel.Select` call it; no third copy.
- **Additive attribution columns, not a second table.** `workflow_id` derives from
  `plan_id`; `domain.Attempt`, `insertAttempts`, and `loadAttempts` must all carry
  the new fields or `Store.Save` erases them.
- **`RootReadOnly` for analysis; worktree for mutating units.** Capability labels
  are not an execution-time boundary.
- **The adoption gate stays a safety/cost gate.** A benefit claim needs a separate,
  operator-approved measurement record.
- **Staging:** MAO-A opt-in correctness (no decision needed), MAO-B attribution and
  bounded selection, MAO-C operator visibility, MAO-D isolated mutating execution,
  MAO-E benefit measurement.

## Operator Decisions

D1 entry point; D2 first concurrency scope; D5 patch integration (deferred); D6
adoption; D7 evidence archive; D8 token budget; D9 per-endpoint capacity
(deferred); D10 operator visibility. D3 (persistence) and D4 (lease model) are
resolved in this revision.
