# Consolidated EGM Review Report

**Targets:**

- `docs/requirements/PRD-MULTI-AGENT-ORCHESTRATION.md`
- `docs/architecture/multi-agent-orchestration.md`
- `docs/plans/PLAN-MULTI-AGENT-ORCHESTRATION.md`

**Date:** 2026-11-23
**Baseline:** `main` @ `2462bdc1`
**Protocol:** Elephant-Goldfish Model, fresh-context read-only reviews

## Reviewer status

| Stage      | Reviewer                       | Status    |
| ---------- | ------------------------------ | --------- |
| Brainstorm | `egm-design`                   | completed |
| Brainstorm | `egm-zoom-out`                 | completed |
| Brainstorm | `egm-comprehension`            | completed |
| Brainstorm | `egm-implementation-readiness` | completed |
| Brainstorm | `egm-structural`               | completed |
| Brainstorm | `egm-codebase-details`         | completed |
| Brainstorm | `egm-inventive-catalyst`       | completed |
| Stage 5    | `egm-zoom-out`                 | completed |
| Stage 5    | `egm-inventive-catalyst`       | completed |
| Stage 6    | `egm-comprehension`            | completed |
| Stage 7    | `egm-design`                   | completed |
| Stage 7    | `egm-structural`               | completed |
| Stage 8    | `egm-zoom-out`                 | completed |
| Stage 9    | `egm-implementation-readiness` | completed |
| Stage 9    | `egm-codebase-details`         | completed |
| Stage 9    | `egm-style`                    | completed |

No reviewer failed to run. Each reviewer worked in a fresh context, read only, and
did not edit the repository. The orchestrator wrote the reports.

## Severity summary

| Severity | Found | Resolved in the documents | Deferred with a trigger | Open operator decision |
| -------- | ----- | ------------------------- | ----------------------- | ---------------------- |
| Blocker  | 6     | 6                         | 0                       | 0                      |
| Major    | 24    | 21                        | 3                       | 0                      |
| Nit      | 17    | 17                        | 0                       | 0                      |

No Blocker remains unresolved. The deferred Major items are MAO-009 (per-endpoint
capacity), MAO-011 (role profiles), and MAO-008 (patch integration); each has an
activation trigger and is excluded from the current plan.

## Traceability matrix

| Finding                                          | Severity | Resolution in the documents                                                      | Evidence                                    |
| ------------------------------------------------ | -------- | -------------------------------------------------------------------------------- | ------------------------------------------- |
| Requirement IDs collide with Phase 9             | Blocker  | Renumbered to `MAO-001…013`; Phase 9 IDs cited as history                        | PRD §0, §7                                  |
| `NextBatch` promotion corrupts resume            | Blocker  | `NextBatch` is read-only; `Next` keeps promotion                                 | PRD MAO-002; architecture §4; plan MAOP-002 |
| Bound formula collapses to 1                     | Blocker  | Bound is `EffectiveActiveWorkers`; `max_parallel_tasks` aliases `MaxConcurrency` | PRD MAO-002; architecture §4; plan MAOP-002 |
| `Store.Save` erases attribution                  | Blocker  | Extend `domain.Attempt`, `insertAttempts`, `loadAttempts`; use `AppendAttempt`   | PRD MAO-003; architecture §5; plan MAOP-002 |
| `internal/domain/selection.go` marked Create     | Blocker  | Marked Modify; symbol and body specified                                         | plan MAOP-002                               |
| `N > 1` would run mutating units in one checkout | Blocker  | MAOP-002 limits `N > 1` to non-mutating units; mutating stays serial             | plan MAOP-002                               |
| D5 used for two decisions                        | Blocker  | Added D10 for visibility; D5 remains patch integration                           | PRD §15; plan MAOP-003                      |
| Most requirements already exist                  | Major    | Verified baseline table; satisfied items marked Implemented/Partial              | PRD §3                                      |
| Opt-in path has live defects                     | Major    | MAOP-001 leases, accounts, and persists the path                                 | PRD MAO-001/007; plan MAOP-001              |
| Adoption gate cannot justify concurrency         | Major    | Gate is safety/cost only; benefit needs a separate measurement                   | PRD MAO-012; plan MAOP-005                  |
| Single-active-task assumptions                   | Major    | Mutating stays serial; multi-active resume deferred                              | PRD MAO-006; plan MAOP-004                  |
| Shared harness is not concurrent-safe            | Major    | Per-worker harness; `RootReadOnly` for analysis                                  | PRD MAO-004; architecture §6; plan MAOP-004 |
| `internal/parallel` is unwired                   | Major    | Reused as the executor; no second scheduler                                      | PRD §3/§10; architecture §2                 |
| Selection predicate would be a third copy        | Major    | One pure predicate in `internal/domain`                                          | PRD MAO-002; architecture §4; plan MAOP-002 |
| Patch integration was a no-op phase              | Major    | MAO-008 deferred; Phase MAO-C is visibility only                                 | PRD MAO-008; plan §12                       |
| Untested acceptance criteria                     | Major    | Cross-cutting verification table maps each to a test and file                    | plan "Cross-cutting verification"           |
| Mid-batch budget exhaustion undefined            | Major    | Stop new selection, record outcomes, leave state resumable                       | PRD MAO-006/007; plan tests                 |
| Trace truncation attributed to wrong package     | Major    | Uses `runtrace.Collector`; files and flag added                                  | PRD MAO-007; plan MAOP-001                  |
| `sop attempts` does not exist                    | Major    | Removed from the baseline                                                        | PRD §3                                      |
| `MaxWorkers` not enforced                        | Major    | Clamp added to `EffectiveActiveWorkers`                                          | plan MAOP-002                               |
| `WorkerFactory` breaks constructor               | Major    | `NewCoordinator(a)` kept as an adapter                                           | plan MAOP-004                               |
| Decision counts disagree                         | Major    | D1–D10 everywhere; D3/D4 resolved                                                | PRD §15; plan Assumptions                   |
| P1 vs phase order                                | Major    | P1 redefined; visibility precedes mutating execution                             | PRD §7/§12; plan                            |
| `MAO-005` vs "already satisfied"                 | Major    | MAO-005 is a preservation requirement                                            | PRD §3/MAO-005                              |
| Review artifacts claimed early                   | Major    | Reports written; DoD checked after                                               | PRD §16/§17                                 |
| Read-only snapshot unbacked                      | Major    | `RootReadOnly`; worktree for mutating units                                      | PRD MAO-004; architecture §6                |
| `sop orchestrate` has no concurrency flag        | Major    | Concurrency refusal tested on `sop run --parallel-units`                         | plan MAOP-001/002                           |
| Per-endpoint limiter/queue speculative           | Major    | Deferred with activation trigger                                                 | PRD MAO-009; D9                             |
| Role profiles over-specified                     | Major    | Deferred with activation trigger                                                 | PRD MAO-011                                 |
| "Controller" undefined                           | Major    | Terminology names the existing packages                                          | PRD §0                                      |
| "Claim" undefined                                | Major    | Terminology defines `WorkResult`/`WorkerClaim`                                   | PRD §0                                      |
| Outcome vocabulary was a second lifecycle        | Major    | Attempt outcome is separate; domain status preserved                             | PRD §0/MAO-010                              |
| Lease modes undefined                            | Major    | Defined with a command × N × mode table                                          | PRD MAO-001                                 |
| Capacity scenario contradicted deferral          | Major    | Marked inactive while MAO-009 is P2                                              | PRD §13                                     |
| Duplicate phase naming                           | Nit      | Renamed phases MAO-A…E and stages MAOP-*                                         | PRD §12; plan                               |
| "Byte-identical" unachievable                    | Nit      | Replaced with a testable comparison                                              | PRD MAO-003/005/§12                         |
| "Permanently" safety gate                        | Nit      | "For this PRD"; enforceable rule stated                                          | PRD MAO-012                                 |
| Duplicate file entries and bullets               | Nit      | Removed                                                                          | plan                                        |
| Undefined `archtest`                             | Nit      | "Guard test"                                                                     | PRD MAO-002                                 |
| Uncalibrated claims                              | Nit      | Calibrated (status, "smallest", "cannot measure", "would convert")               | PRD/plan                                    |

## Resolved design decisions

- **D3 — Persistence:** additive, defaulted attribution columns on `task_attempts`;
  `work_unit_id` is the task ID in v1; `workflow_id` derives from `plan_id`. No
  second attempt table.
- **D4 — Lease model:** one process with N in-memory slots; the lease is not
  extended to counted slots.

## Outstanding operator decisions

| ID  | Decision                                   | Recommendation                                                                  |
| --- | ------------------------------------------ | ------------------------------------------------------------------------------- |
| D1  | Entry point for multi-work-unit execution  | `sop run --parallel-units N` (default 1); `sop orchestrate` stays analysis-only |
| D2  | First concurrency scope                    | Non-mutating analysis first; mutating units serial until MAOP-004               |
| D5  | Patch integration and stale-base detection | Deferred until an approved patch path                                           |
| D6  | Adoption/default                           | Keep `orchestration.enabled` false until a recorded measurement                 |
| D7  | Evidence archive identity                  | Per work unit; legacy per-plan archives remain readable                         |
| D8  | Token budget                               | Require usage attribution; refuse rather than run unmeasured                    |
| D9  | Per-endpoint capacity                      | Deferred until a measured saturation incident                                   |
| D10 | Operator visibility                        | Ship the graph view in MAO-C; it is read-only                                   |

## Residual risks accepted

- The adoption gate's ADOPT branch remains unreachable for multi-unit cases. It is
  intentionally not used to justify concurrency.
- Mutating parallelism, patch application, and multi-active resume are deferred.
  The plan keeps mutating units serial, so these risks do not apply to v1.
- The plan's tests are planned, not implemented. The baseline tests pass; the new
  tests are acceptance criteria for the future stages.
- No live multi-agent measurement exists. The PRD records "not measured" rather
  than a benefit claim.

## Definition of done for this review

- [x] Existing capabilities and gaps verified against source with references.
- [x] Requirements reconciled; redundant and satisfied work removed.
- [x] Requirement IDs no longer collide with delivered Phase 9 IDs.
- [x] Security and SOP authority invariants restated against the real code.
- [x] Minimal architecture selected with documented alternatives.
- [x] Fresh-context EGM review completed and recorded.
- [x] Risks, decisions, and acceptance cases reviewed.
- [x] Implementation plan created in the repository's plan format.
- [ ] Operator decisions resolved.
- [x] Documentation-only PR opened and linked; no implementation or merge implied:
      <https://github.com/imhttran/agentic-sop/pull/645> (draft).
