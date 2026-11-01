# AUTONOMY — consolidated outcome report

**Milestone:** AUTONOMY-001 — one prompt, one execution, one verified outcome.
**Status:** bounded implementation pass complete. **Verdict: GO (scoped)** — see §6.
**Scope:** changes, validation, acceptance, approvals, and remaining issues for the autonomy
milestone. This is **not** a production-readiness claim.

---

## 1. Changes in this pass

| Change | Path | Notes |
|---|---|---|
| Consolidated authoritative plan | `docs/plans/PLAN-SOP-Autonomy.md` | Merges the operator plan `docs/plans/PLAN-SOP-AUTONOMY-001.md` (workstreams A–E) into one document; keeps the three SOP stages (AUTONOMY-001/004/005) unchanged, so SOP sees `Plan changed: no`. The operator file is **superseded by reference** (it lives untracked in the `main` worktree and was not modified). |
| Baseline corrections | `docs/reports/autonomy/AUTONOMY-001-baseline.md` | Revision note + corrections of four wrong/nonexistent code paths (`runGraph`→`drive.go:46`, `runAttempts`→`escalation.go:76`, `commitgate.go`→`gate.go`, `handoff.go`→`internal/handoff/`) and three fixture details. Original run evidence preserved in `.agent-sdlc/runs/AUTONOMY-001/`. |
| Deterministic regression | `internal/cli/autonomy_cleanup_scenario_test.go` | `TestAutonomousMultiPackageUnusedCodeCleanup` — closes gap G11 (see §3). |
| Independent verification | `docs/reports/autonomy/AUTONOMY-independent-verification.md` | (prior pass) |
| This report | `docs/reports/autonomy/AUTONOMY-outcome-report.md` | — |

No Go **production** code changed; the only Go change is a test. No new framework. `harden-001`
not merged. No commit/push/merge.

## 2. Validation results

| Check | Result |
|---|---|
| `gofmt -l .` | clean |
| `go vet ./...` | OK |
| `go build ./...` | OK |
| `go test -count=1 ./...` | **all packages ok** |
| `go test -race -count=1 ./...` | **all packages ok** (no failures; the intermittent `TestCrossModuleTimeoutAndCancellation` did not reproduce) |
| `git diff --check` | clean |
| Targeted: `go test ./internal/{autonomy,recovery,scheduler,continuation,cli}/...` | ok |

## 3. Acceptance results (independently verified)

| Item | Criterion | Verdict |
|---|---|---|
| AUTONOMY-001 | path by code reference; multi-package scenario recorded; gaps listed; commit boundary; no new framework | **MET** (after corrections) |
| AUTONOMY-004 | conditional composition determined with evidence; no new framework code | **MET** — NOT_REQUIRED (existing `sop prompt --capability implement` suffices); no code added |
| AUTONOMY-005 | GO/HOLD with VERIFIED/PARTIAL/MISSING/BLOCKED listed | **MET** — HOLD at the time (blocking-class G11); see §6 update |
| G11 automated scenario | deterministic, disposable fixture, no model | **CLOSED** — `TestAutonomousMultiPackageUnusedCodeCleanup` passes (0.2s): one prompt, unused symbols removed from both non-main packages, module really built and vetted, commit boundary asserted |
| Real-model scenario | retained as supporting evidence | **PASS** — `.agent-sdlc/runs/prompts/prompt-20261009-192346` (recorded under `autonomy-001-scenario/`) |

**Bounded recovery and approval behavior** — verified using **existing** tests (no new recovery
tests added, because coverage already exists): `internal/autonomy` (`TestRecoverableFailuresAreAutomaticUnderHigh`,
`TestTransientRetryIsAutomaticAtEveryLevel`, `TestBuildFailureIsAutoFixLowRisk`,
`TestExhaustionIsTerminalUnderHigh`, `TestHumanBoundariesAreNeverAutomatic`,
`TestSecurityDecisionRequiresHumanAtEveryLevel`), `internal/recovery`, `internal/scheduler`,
`internal/continuation`, and `internal/cli` (`TestHighAutonomyExhaustionIsTerminalNotHuman`,
`TestBalancedAutonomyExhaustionStaysHuman`, `TestRunSecurityDecisionRequiresHuman`,
`TestRunGenuineAmbiguityStillNeedsHuman`, `TestAutomaticRecoveryDoesNotBypassNeedsHuman`). The
only **demonstrated gap** (the automated multi-package scenario) is the one covered by the new
test.

## 4. Approvals and boundaries

- `human.approval_before_commit: true`; every governed run and the deterministic scenario stop at
  the commit boundary and do not commit. `sop approvals` → no pending approvals.
- Work confined to `feature/autonomy-001` / `sop-autonomy-001`. Other worktrees untouched
  (`main` user-owned changes + operator plans intact; `sop-harden-001` `1ed7189` unmerged;
  `sop-preview-001` untouched).
- `plan-sop-autonomy` remains **ACTIVE** with all three tasks `LOCAL_DONE`; **no** plan was
  completed, superseded, or historicalized in SOP (`sop continue --check` → `RECONCILE_CLEAN`,
  `Plan changed: no`).

## 5. Remaining issues (non-blocking for this milestone)

1. **001D PARTIAL** — no explicit authorization-scope or dry-run/preview affordance for a
   dedicated bounded-autonomous mode.
2. **001E PARTIAL** — the richer operator scenario (verify candidates against production
   imports, plans, docs, runtime entry points, migrations, compatibility surfaces) is not
   automated; only the simpler multi-package cleanup is.
3. **G13 PARTIAL** — no single assembled "workspace result" artifact.
4. **G14 PARTIAL** — the end-to-end run is not exercised *within* the in-repo governed tool
   harness (repo-root confined; cannot run a nested `sop prompt`).
5. **KI-01** — acceptance is not gate-enforced on `main` (a `PASS`/`LOCAL_DONE` is not proof);
   owned by the unmerged `harden-001`. This milestone explicitly does **not** rely on the gate as
   proof.
6. **Two plans** — the operator file `docs/plans/PLAN-SOP-AUTONOMY-001.md` lives in the `main`
   worktree and cannot be removed from this worktree; it should be deleted by the operator so
   this document is the only autonomy plan.
7. **`PLAN_COMPLETE` pending** — `sop continue --check` reports the plan is satisfied and ready
   to complete; completion/historicalization is **not** performed (needs operator approval).

## 6. Verdict: **GO (scoped)**

All AUTONOMY acceptance criteria are independently verified **MET**, the blocking-class gap
(G11) is **closed** by a deterministic regression, the multi-package cleanup is verified both
deterministically and on a real model, and full `gofmt`/`vet`/`build`/`test`/`race`/`diff`
validation is green. The AUTONOMY-001 milestone — *one prompt, one execution, one verified
outcome* — is achieved.

This GO is **scoped to the verified capability and the AUTONOMY-001 milestone**; it is **not** a
production-readiness claim and does **not** cover a dedicated bounded-autonomous product mode
(items 1–4 remain) or rely on gate-enforced acceptance (item 5). Those are follow-ups, not
blockers for this milestone.
