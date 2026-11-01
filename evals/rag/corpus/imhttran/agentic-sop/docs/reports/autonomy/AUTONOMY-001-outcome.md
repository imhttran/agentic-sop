# AUTONOMY-001 — Autonomous Decision Contract: outcome report

**Milestone:** AUTONOMY-001 — one prompt → one autonomous execution → one verified outcome.
**Outcome status:** **VERIFIED** (milestone core) — with named `PARTIAL` items below (§9).
**Scope note:** not a production-readiness claim; built on `feature/autonomy-001` (`d9f7722`).

---

## 1. Objective

Make one authorized engineering objective (for example "clean up unused code") run end to end —
chosen approach, execution, bounded recovery, independent acceptance, one consolidated result —
without the operator managing intermediate implementation choices. The agent owns routine
engineering decisions; SOP enforces deterministic governance; the human owns genuine authority
and safety boundaries.

## 2. Authorization envelope (recorded at start)

| Field | Value |
|---|---|
| Objective | Implement the Autonomous Decision Contract for AUTONOMY-001 |
| Repository / worktree | `feature/autonomy-001` / `sop-autonomy-001` at `d9f7722` |
| Allowed files / operations | edit `docs/plans/PLAN-SOP-Autonomy.md`; create `docs/reports/autonomy/**`; create `internal/outcome/**`; create `internal/cli/*_test.go` |
| Protected / forbidden | other worktrees (`main`, `sop-harden-001`, `sop-preview-001`); `.agent-sdlc` SOP state (no manual edits); `harden-001` (no merge); production CLI/orchestration code (no edits) |
| Validation requirements | `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `git diff --check` |
| Recovery / attempt budgets | existing bounded budgets (`quality.max_fix_cycles`, `autonomy.max_continuations`); no unbounded loops |
| External side effects | **none authorized** — no commit/push/merge/PR/release |
| Approval boundaries | commit/PR (human-gated via `human.approval_before_commit`); authority boundaries (human) |

Missing authorization is **not** implicit permission.

## 3. Decision matrix (contract → existing components)

| Decision class | Owner | Mechanism |
|---|---|---|
| Equivalent designs, smallest change, task ordering, discovery, bounded retry/fallback, deferring ambiguous candidates | **Agent** (no ask) | `internal/autonomy` `Policy` (`AUTO_RETRY`/`AUTO_CONTINUE`/`AUTO_FIX`/`AUTO_RECONCILE`) |
| Envelope, dependencies, budgets, approval state, protected paths, deterministic checks, acceptance evidence, lifecycle transitions | **SOP** (deterministic) | `internal/toolharness` (confinement, `isProtectedPath`, allow/deny audit), `internal/validate`, `internal/review`, `internal/quality`, `internal/scheduler`, approval store |
| Commit/push/merge/release/deploy; destructive/irreversible; sensitive/security; scope expansion; exhausted budget | **Human** | `failure` boundary kinds (`SecurityBoundary`/`DestructiveOperation`/`ApprovalRequired`/`AmbiguousContract`/`ReplanRequired`) → `autonomy` human decision; `internal/commitgate` |

A model `PASS`, confidence score, or recommendation cannot override a deterministic block.

## 4. Decision log (this pass)

| # | Decision | Alternatives considered | Evidence / reason |
|---|---|---|---|
| D1 | Reconcile the two plans by making this doc authoritative while keeping the three SOP stages unchanged | supersede (rejected: resets `LOCAL_DONE`, archives history); activate a competing plan (rejected: implicit archive) | `sop continue --check` → `RECONCILE_CLEAN`, `Plan changed: no` |
| D2 | Implement the narrow outcome verifier as a new stdlib-only `internal/outcome` | extend/merge `harden-001` (forbidden); test-only helper (rejected: not reusable); new orchestrator (forbidden) | composes existing deterministic checks; no new engine |
| D3 | Defer the uncertain documented-API candidate (`strx.Legacy`) instead of removing it | remove it (rejected: documented supported API → uncertain) | `COMPAT.md` documents `Legacy` as supported |
| D4 | Scope independent acceptance to the cleanup fixture; mark generic enforcement `PARTIAL` | claim generic enforcement (rejected: not implemented; `harden-001` unmerged) | `internal/outcome` is fixture-driven |
| D5 | Mark AUTONOMY-002 `NO_CHANGE_REQUIRED` and AUTONOMY-005 `NOT_REQUIRED` | implement anyway (rejected: duplicates working functionality) | existing autonomy tests + existing single-prompt entry point |

## 5. Changes

| Path | Change |
|---|---|
| `docs/plans/PLAN-SOP-Autonomy.md` | Consolidated authoritative plan; absorbs `PLAN-SOP-AUTONOMY-001-UPDATED.md`; Decision Contract mapping |
| `docs/reports/autonomy/AUTONOMY-001-erratum.md` | Baseline corrections (erratum; original preserved) |
| `docs/reports/autonomy/AUTONOMY-001-baseline.md` | Revision note referencing the erratum (corrected in place last pass) |
| `internal/outcome/outcome.go`, `outcome_test.go` | Narrow outcome-level acceptance verifier (VERIFIED/PARTIAL/HOLD) |
| `internal/cli/autonomy_outcome_contract_test.go` | Regression: success + deferred candidate + false-success + boundary→HOLD |
| `internal/cli/autonomy_cleanup_scenario_test.go` | (prior pass) deterministic multi-package cleanup |

No production CLI/orchestration code changed; the Go changes are the verifier package and tests.

## 6. Verification — the three required outcomes

| Outcome | Test | Result |
|---|---|---|
| **Autonomous success** — one prompt → authorized cleanup → tests → independent acceptance → VERIFIED, no intermediate approval | `TestAutonomousCleanupOutcomeVerified` | **PASS** — `exitOK`, gate `PASS`, no `NEEDS_HUMAN`/`WAITING_FOR_HUMAN`, commit boundary held, `outcome.Verify` → **VERIFIED** |
| **Autonomous recovery** — correctable failure → bounded fix → completion, no operator prompt | `TestRunNonCompletedFixWithRedBuildIsAutoFixed` (existing) | **PASS** — red build auto-fixed within the budget, no human boundary |
| **Real authority boundary** — protected/out-of-scope action → stop, no side effect | `TestRunSecurityDecisionRequiresHuman` (existing) + `TestAutonomousCleanupBoundaryViolationIsHold` (new) | **PASS** — security decision → human; protected-file removal → independent outcome **HOLD** |

Additional regressions:

| Test | File | Result |
|---|---|---|
| `TestAllRequiredMetIsVerified`, `TestUnmetRequiredIsPartial`, `TestBoundaryViolationIsHold`, `TestNilCheckCannotBeSatisfiedByAClaim`, `TestNoRequiredCriterionIsPartial` | `internal/outcome/outcome_test.go` | PASS |
| `TestAutonomousMultiPackageUnusedCodeCleanup` | `internal/cli/autonomy_cleanup_scenario_test.go` | PASS |
| `TestAutonomousCleanupFalseSuccessIsNotVerified` | `internal/cli/autonomy_outcome_contract_test.go` | PASS — a fabricated claim cannot yield VERIFIED |

## 7. Validation results

`gofmt -l .` clean · `go vet ./...` OK · `go build ./...` OK · `go test -count=1 ./...`
**all packages ok** · `go test -race -count=1 ./...` **all packages ok** · `git diff --check`
clean. No environmental blockers.

## 8. Attempts, recoveries, protected/skipped items

- **Recovery:** bounded automatic recovery is unchanged (existing budgets); the recovery outcome
  is proven by an existing deterministic test. No human confirmation was requested for routine
  authorized decisions.
- **Skipped (deferred) candidate:** `strx.Legacy` — unreferenced in code but documented in
  `COMPAT.md` as supported API → deferred, not removed. Recorded in the regression.
- **Protected item:** `LICENSE` — a boundary criterion; its removal yields HOLD.

## 9. Named `PARTIAL` / outstanding items

1. **Independent acceptance is narrow** (`PARTIAL`) — `internal/outcome` verifies caller-supplied
   criteria; generic, lifecycle-enforced acceptance is **not** implemented on this branch (owned
   by unmerged `harden-001`). The milestone does not rely on the gate as proof.
2. **001D (from the updated plan)** — no explicit authorization-scope or dry-run/preview command
   affordance for a dedicated bounded-autonomous mode.
3. **G13 / G14** — no single assembled workspace-result artifact; the end-to-end run is not
   exercised inside the in-repo governed tool harness (repo-root confined).
4. **`PLAN_COMPLETE` pending** — `sop continue --check` reports the plan satisfied; completion is
   **not** performed (operator approval required).
5. **Operator plan file** `docs/plans/PLAN-SOP-AUTONOMY-001-UPDATED.md` lives untracked in the
   `main` worktree; it is superseded by reference and should be deleted by the operator so one
   plan remains.

## 10. Outcome status

**VERIFIED (milestone core).** The Autonomous Decision Contract is implemented by composing
existing components; one prompt drives an authorized cleanup through decisions, deterministic
checks, and independent acceptance to `VERIFIED` with no intermediate human input; bounded
recovery is proven; and a protected/out-of-scope action still stops (`HOLD`/human). Task
`PASS`/`LOCAL_DONE` is explicitly **not** treated as objective completion — the verdict comes
from the independent verifier.

Named `PARTIAL` items (§9) do not block the milestone but remain for a follow-up.

## 11. Boundaries respected

No second orchestration engine; no broad hardening campaign; no merge of `harden-001`; no edits
to other worktrees; no `.agent-sdlc` manual edits; no commit/push/merge/release. `plan-sop-autonomy`
remains **ACTIVE** with its three tasks `LOCAL_DONE`.
