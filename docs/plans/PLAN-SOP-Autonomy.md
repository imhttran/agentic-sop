# PLAN — SOP Autonomous End-to-End Execution (AUTONOMY)

**Status:** ACTIVE (SOP plan `plan-sop-autonomy`) — **consolidated authoritative plan**.
**Basis:** reconciled against the verified existing implementation at `d9f7722`; consolidated
2026-10-09 with the operator-authored `docs/plans/PLAN-SOP-AUTONOMY-001-UPDATED.md`
("SOP Autonomous Outcome Execution" / Autonomous Decision Contract) into this single
authoritative document.

## Consolidation & reconciliation (2026-10-09)

This is the **single authoritative autonomy plan**. Its stages (AUTONOMY-001 / AUTONOMY-004 /
AUTONOMY-005) are unchanged and all `LOCAL_DONE`, so SOP reports `Plan changed: no`
(`sop continue --check`) and no lifecycle history is overwritten.

`docs/plans/PLAN-SOP-AUTONOMY-001-UPDATED.md` (operator-authored; untracked in the `main`
worktree) is **superseded by this document.** Its Autonomous Decision Contract is realized by
composing existing components (below); its workstreams map to delivered work as:

| Updated workstream                                      | Disposition              | Evidence                                                                                                                                                                           |
| ------------------------------------------------------- | ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AUTONOMY-001 Plan reconciliation & decision authority   | **SATISFIED**            | this consolidation + the Decision Contract mapping; `docs/reports/autonomy/AUTONOMY-001-outcome.md`                                                                                |
| AUTONOMY-002 Autonomous continuation & bounded judgment | **NO_CHANGE_REQUIRED**   | `internal/autonomy` policy + `internal/cli` wiring already automate routine decisions; `internal/cli/autonomy_test.go` and the `internal/{recovery,scheduler,continuation}` suites |
| AUTONOMY-003 Independent outcome acceptance             | **IMPLEMENTED (narrow)** | new `internal/outcome` verifier + tests; generic enforcement `PARTIAL` (owned by unmerged `harden-001`)                                                                            |
| AUTONOMY-004 Automated regression & outcome report      | **IMPLEMENTED**          | `internal/cli/autonomy_cleanup_scenario_test.go`, `internal/cli/autonomy_outcome_contract_test.go`, `docs/reports/autonomy/AUTONOMY-001-outcome.md`                                |
| AUTONOMY-005 Optional UX seam                           | **NOT_REQUIRED**         | `sop prompt --capability implement` is the single-prompt entry point (`internal/cli/prompt.go`); no usability gap prevents the contract                                            |

## Autonomous Decision Contract (realized by existing components)

The contract is a mapping, not a new subsystem:

- **Agent-owned routine decisions** -> `internal/autonomy` `Policy` (`AUTO_RETRY`, `AUTO_CONTINUE`, `AUTO_FIX`, `AUTO_RECONCILE`) at the configured level; the agent proceeds without asking for equivalent designs, task ordering, discovery, bounded retry/fallback, or deferring ambiguous candidates.
- **SOP-owned deterministic governance** -> `internal/toolharness` (workspace confinement, `isProtectedPath`, allow/deny audit), `internal/validate`, `internal/review`, `internal/quality`, `internal/scheduler` budgets, and approval state; a model `PASS`/confidence cannot override a deterministic block.
- **Human-only boundaries** -> `failure` boundary kinds (`SecurityBoundary`, `DestructiveOperation`, `ApprovalRequired`, `AmbiguousContract`, `ReplanRequired`) -> `autonomy` human decision; commit/PR gated by `internal/commitgate` + `human.approval_before_commit`.
- **Authorization envelope** -> recorded from operator config + the task spec (objective, validation, autonomy level, `quality.max_fix_cycles`, `autonomy.max_continuations`, protected paths via `toolharness`, side-effect permissions via `human.approval_before_commit`); missing authorization is not implicit permission.
- **Decision log** -> existing run artifacts (`classification.json` + its `autonomy` block, `activity.jsonl`, `report.md`) record decision, evidence, and reason.

## Reconciliation note

The first draft proposed five tasks. Two were **removed as duplicates of working,
already-passing functionality**:

- **AUTONOMY-002 (bounded low-risk recovery regressions) — REMOVED.** Already covered by
  `internal/autonomy/autonomy_test.go` (`TestRecoverableFailuresAreAutomaticUnderHigh`,
  `TestTransientRetryIsAutomaticAtEveryLevel`, `TestBuildFailureIsAutoFixLowRisk`,
  `TestExhaustedBuildFixIsBoundedNotUnknown`, `TestProductiveIncompleteIsAutoContinue`,
  `TestExhaustionIsTerminalUnderHigh`) and `internal/cli/autonomy_test.go` +
  `internal/cli/{recovery,recovery_continue,budget_continuation,no_change_continuation}_test.go`
  and the `internal/scheduler` / `internal/continuation` suites.
- **AUTONOMY-003 (high-risk / authority-boundary preservation) — REMOVED.** Already covered by
  `internal/autonomy/autonomy_test.go` (`TestHumanBoundariesAreNeverAutomatic`,
  `TestSecurityDecisionRequiresHumanAtEveryLevel`), `internal/autonomy/early_test.go`
  (boundary, ambiguity, security, destructive), and `internal/cli/security_boundary_continuation_test.go`
  (`TestRunSecurityDecisionRequiresHuman`, `TestRunGenuineAmbiguityStillNeedsHuman`) plus
  `TestAutomaticRecoveryDoesNotBypassNeedsHuman`.

What remains is the **demonstrable gap**: there is no _recorded, independently verified_
baseline or scenario for the autonomous end-to-end path, and no determination of whether a
single-command composition is needed.

## Project

SOP autonomous end-to-end execution

## Summary

Confirm, and record as evidence, SOP's ability to run a governed task end to end from a
single prompt, recovering automatically from bounded low-risk failures while preserving
human approval for high-risk actions — reusing the existing `internal/autonomy`,
`internal/recovery`, `internal/continuation`, `internal/scheduler`, `internal/approval`,
`internal/router`, and `internal/quality` components. No new orchestration framework.

## AUTONOMY-001 — End-to-end baseline and multi-package unused-code cleanup scenario

Record the current autonomous end-to-end path by code reference, and exercise a single
prompt against a realistic **multi-package** Go module containing unused code, capturing the
observed stage, gate, and workspace result. List every gap as VERIFIED, PARTIAL, or MISSING.

### Dependencies

None

### Deliverables

- `docs/reports/autonomy/AUTONOMY-001-baseline.md` — component map, gap list, and the recorded scenario.

### Acceptance Criteria

- The autonomous path is identified by code reference (`sop run` graph execution and `sop prompt --capability implement`), not asserted.
- A single prompt performs a multi-package unused-code cleanup on a disposable module, with the observed stage, gate, fix cycles, and workspace result recorded.
- Every gap between the request and the existing components is listed as VERIFIED, PARTIAL, or MISSING with evidence.
- The high-risk action (commit) remains behind the human approval boundary.
- No new orchestration framework is introduced.

### Validation Commands

`go build ./...`; `go test -count=1 ./internal/cli/... ./internal/autonomy/...`; `git diff --check`.

### Failure Behavior

If the scenario does not complete, record the failing stage and do not claim success.

## AUTONOMY-004 — Single-command composition (conditional)

Determine whether the existing commands can satisfy the acceptance scenario from a single
prompt. Implement a thin single-command composition **only if they cannot**; the seam must
delegate to the existing planner and graph runner and must contain no new scheduler,
recovery, approval, or routing logic.

### Dependencies

- AUTONOMY-001

### Deliverables

- A determination recorded in `docs/reports/autonomy/AUTONOMY-001-baseline.md` (this task adds a decision), plus the minimal CLI seam and its tests **only if** the gap is MISSING.

### Acceptance Criteria

- The determination is recorded with evidence: either the existing commands satisfy the scenario (NOT_REQUIRED), or a thin seam exists that produces and executes a task graph from one prompt.
- Any seam delegates to the existing planner and graph runner; it contains no new scheduler, recovery, approval, or routing logic.
- All existing autonomy and approval boundaries are unchanged.

### Validation Commands

`gofmt -l .`; `go vet ./...`; `go test -count=1 ./...`; `go build ./...`; `git diff --check`.

### Failure Behavior

If the gap is not MISSING, record NOT_REQUIRED and add no code.

## AUTONOMY-005 — Readiness and disposition

Produce the autonomy readiness report and a GO/HOLD recommendation.

### Dependencies

- AUTONOMY-001
- AUTONOMY-004

### Deliverables

- `docs/reports/autonomy/AUTONOMY-005-readiness.md`

### Acceptance Criteria

- A GO/HOLD verdict backed by recorded evidence, with VERIFIED, PARTIAL, MISSING, and BLOCKED items listed.
- No new framework and no acceptance-enforcement change is claimed.
- No plan is activated, completed, or historicalized by this task.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go test -count=1 ./...`; `git diff --check`.

### Failure Behavior

If readiness criteria are unmet, report HOLD rather than GO.

## Dependencies (graph)

```text
AUTONOMY-001 ──► AUTONOMY-004 ──► AUTONOMY-005
```

## Parallelizable groups

- AUTONOMY-001 is the only initially runnable task (AUTONOMY-004 depends on it). No
  independent parallel group exists in this reconciled plan; SOP executes the chain
  serially with bounded retry/recovery.

## Notes

- Out of scope (per `docs/PRD.md` §15): unlimited autonomous execution, large multi-agent
  swarms, automatic deployment, autonomous secret creation, bypassing branch protections,
  self-modifying policy, automatic acceptance of every AI review suggestion.
- `harden-001` is not merged and is out of scope for this plan.

## Delivered out-of-band (operator, 2026-10-09)

- **AUTONOMY-001 baseline corrected** — factual code-reference and fixture-detail corrections
  applied to `docs/reports/autonomy/AUTONOMY-001-baseline.md` (revision note at top); the
  original run evidence is preserved unmodified in `.agent-sdlc/runs/AUTONOMY-001/`.
- **Deterministic regression added — closes gap G11** —
  `internal/cli/autonomy_cleanup_scenario_test.go`
  (`TestAutonomousMultiPackageUnusedCodeCleanup`): a single `sop prompt --capability implement`
  removes unused code across a multi-package disposable fixture using a deterministic agent;
  the module is really built and vetted, and the commit boundary is asserted.
- **Independent verification** — `docs/reports/autonomy/AUTONOMY-independent-verification.md`.
- **Consolidated outcome report** — `docs/reports/autonomy/AUTONOMY-outcome-report.md`.

## Status & history

| Item                                | Status                                     | Evidence                         |
| ----------------------------------- | ------------------------------------------ | -------------------------------- |
| AUTONOMY-001                        | LOCAL_DONE                                 | `.agent-sdlc/runs/AUTONOMY-001/` |
| AUTONOMY-004                        | LOCAL_DONE (NOT_REQUIRED — no seam needed) | baseline §6                      |
| AUTONOMY-005                        | LOCAL_DONE (HOLD)                          | readiness report                 |
| operator `PLAN-SOP-AUTONOMY-001.md` | SUPERSEDED by this document                | table above                      |
| G11 automated scenario              | CLOSED (deterministic regression)          | this pass                        |

`plan-sop-autonomy` remains ACTIVE with all tasks `LOCAL_DONE`; this consolidation is a
**documentation** change — no plan was completed or historicalized in SOP, so prior run
evidence and lifecycle history are preserved.
