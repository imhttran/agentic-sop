# SOP planner spike — read-only closure audit

Date: 2026-10-19 • Audited revision: `origin/main` @ `2262b48` • Mode: read-only.
The original plan ([SOP-Planner-Connection-Spike.md](../../plans/SOP-Planner-Connection-Spike.md)),
the SPIKE-001–006 documents, and all historical run artifacts are unchanged. Nothing was
rerun live and no SOP state was modified. The only commands run were `go test` (temp dirs).

## Verdict: PARTIAL — closure not yet verified

The code, deterministic tests and a documented live loop all exist on `main`. Closure is
PARTIAL because the live run evidence is not committed, the live transcript is
operator-attested, unattended operation is unproven, and two defects are open. This is a
verification status, not a claim the implementation is incomplete.

## 1. Three evidence layers (kept separate)

| Layer | What it is | Where |
| --- | --- | --- |
| A. Original acceptance | Plan criteria and the 10-item checklist | `SOP-Planner-Connection-Spike.md` |
| B. Historical SOP execution | Early SOP runs of SPIKE-001–006 (LOCAL_DONE / PARTIAL / NEEDS_HUMAN 3/3) | `SPIKE-001…003`, `SPIKE-003-closure-checklist.md` |
| C. Later ChatGPT MCP tests and merged fixes | Live loops `planner-live`, `planner-2q`, `planner-chatgpt`, `planner-2q-live`; PRs #68, #78, #80 | `SPIKE-004-live-evidence.md`, `SPIKE-006-*`, `planner-live-test-findings.md` |

Layer B outcomes (SPIKE-002 initial PARTIAL/NO-GO, SPIKE-003 NEEDS_HUMAN after 3/3 fix
cycles) are historical. Later work in layer C superseded them; it did not rewrite them.

## 2. SPIKE-001–006 mapping

| Stage | Merged code / tests | Run or live evidence | Status |
| --- | --- | --- | --- |
| 001 baseline and environment | n/a (inventory) | `SPIKE-001-baseline-inventory.md`, `SPIKE-001-disposable-environment.md` | Complete (documents) |
| 002 client and transport | `internal/mcp`, `internal/cli/mcp.go`; `TestMCPPlannerServerSurface` | `SPIKE-002-connection-discovery.md` §9: ChatGPT read `sop_status` over the Secure MCP Tunnel | Verified (operator-attested; reconnect-twice not independently checked) |
| 003 question/decision contract, approval isolation | `internal/question/*`, `internal/cli/planner*.go`; `TestMCPPlannerSurfaceShape`, `TestSubmitAnswerStale`, `TestAtomicSubmission*` | Closure checklist; planner surface lists 4 tools, no approve/decline | Verified (deterministic) |
| 004 WAITING_FOR_PLANNER and resume | `planner.go`, `planner_raise.go`; `TestPlannerIntegrationTwoRoundsOneRun`, `TestEarlierPlannerDecisionsScope` | `SPIKE-004-live-evidence.md`; `planner-live` (1 question) | Verified |
| 005 restart/stale/duplicate/authority/budgets | `planner_spike005_test.go`, `planner_timeout_test.go`, `continuation_test.go`, `reconcile_test.go` | `SPIKE-005-verification-report.md` exists and is committed | Verified deterministically; crash recovery is modeled, no real SIGKILL |
| 006 live demo and final report | n/a | `SPIKE-006-feasibility-report.md` (+ `-live-transcript.md`), `planner-live-test-findings.md`, `testing/PLANNER-TWO-QUESTION-LIVE.md` | Report exists. Transcript is operator-attested; no machine-captured copy |

Correction to the working status table: SPIKE-005 and SPIKE-006 are **not** unrecovered.
Both reports are committed on `main`. Their remaining weakness is the evidence base above.

## 3. Two-question lifecycle (including actual completion)

- **Live** (`planner-2q-live`, 2026-10-18, real model-driven executor, binary with #68):
  `q-1` (order) then `q-5` (duplicates) in one task and run epoch; ChatGPT answered both;
  `T001` reached `LOCAL_DONE`, outcome `VERIFIED`; continuations `q-1` SUPERSEDED, `q-5`
  COMPLETED; `normalized.txt` matched both answers. Source: `planner-live-test-findings.md`.
- **Re-executed today:** `TestPlannerIntegrationTwoRoundsOneRun` passes (task `LOCAL_DONE`,
  no CLAIMED/PENDING continuation, no re-execution).
- **Caveat:** `.run/planner-live`, `.run/planner-2q*` artifacts are **not tracked** in git
  (only `.run/reference/**` is). The live state files cited in the reports cannot be
  re-inspected from the repository. The run trace does not record prompt text.

## 4. Boundary checks (all re-run today, all pass)

| Boundary | Test(s) | Result |
| --- | --- | --- |
| Restart while waiting | `TestPlannerRestartWhileWaitingReparks` | pass |
| Restart after apply, at-most-once | `TestContinuationCreatedAtomicallyWithApplication`, `TestApplyIdempotentNoDoubleDispatch`, `TestPlannerResumeCountsOneCompletion` | pass |
| Stale answer | `TestCLIPlannerStaleBindingSkipped`, `TestSubmitAnswerStale`, `TestRepoFingerprintResolvesSymlinks` | pass |
| Duplicate answer | idempotency tests above | pass |
| Timeout | `TestPlannerTimeoutParksWithoutRetry`, `TestDecidePauseOnTimeout` | pass |
| Exhausted rounds | `TestPlannerExhaustedRoundsParkAtHumanBoundary` | pass |
| Human approval | `TestPlannerAnswerCannotSatisfyHumanGate`, `TestPlannerIntegrationHumanGateBlocks`, `TestPlannerSupersedeRecoveryRespectsHumanGate` | pass |
| Budgets / disabled mode | `TestPlannerClarificationPreservesFailureBudget`, `TestPlannerDisabledIgnoresExistingQuestion` | pass |

Run: `go test -count=1 ./internal/question/... ./internal/mcp/...` and
`go test -count=1 -run 'Planner|Question|Answer|RepoFingerprint|Earlier' ./internal/cli/`.

## 5. Gap report

| # | Gap | Evidence | Severity |
| --- | --- | --- | --- |
| G1 | `TestCrossModuleTimeoutAndCancellation/cancellation` **reproduced** under full `go test ./...` (`parse pid "": EOF`, `decision_evidence_crossmodule_test.go:454`); passes in isolation. The reports call it unresolved. Looks like a pid-file read race | full-suite run today | Medium (flaky gate) |
| G2 | HTTP 409 `mcp_initialization_required` from `tunnel-client` on a parallel `question-get` is unresolved and not reproduced | `SPIKE-005` §6 | Medium |
| G3 | Live run state is untracked; transcript is operator-attested, not machine-captured | `.run` tracking, `SPIKE-006` §9.4 | Medium (closure evidence) |
| G4 | Unattended operation unproven; every answer was human-relayed | `SPIKE-006` §8.2 | Known, out of scope |
| G5 | Crash recovery modeled, not tested with a real process kill | `SPIKE-006` §7 | Low |
| G6 | Reports still call the SPIKE-006 disposition PARTIAL in places and GO elsewhere (§0 vs §15) | `SPIKE-006-feasibility-report.md` | Low (doc consistency) |
| G7 | `TestWriteEarlyJEVArtifactFailureDoesNotPanicOrAlterOtherFiles` fails when the suite runs as root (unwritable-dir assumption). Environmental, not a planner finding | `internal/run/early_jev_artifact_test.go:205` | Low |

## 6. Proposed follow-up tasks (narrow, none restarts completed work)

1. **PLN-F1** Fix the pid-file race in `TestCrossModuleTimeoutAndCancellation` (G1).
2. **PLN-F2** Reproduce or close the 409 with a parallel-call test against `sop mcp --planner` and tunnel-client logs (G2).
3. **PLN-F3** Commit a sanitized evidence bundle for `planner-2q-live` (questions, answers, continuations, outcome) under `docs/reports/evidence/` (G3).
4. **PLN-F4** Reconcile `SPIKE-006` §0/§15 wording (G6).
5. **PLN-F5** Skip the unwritable-dir assertion when running as root (G7).
6. Deferred: unattended invocation (G4) and real-kill recovery test (G5) belong to the production milestone.

## 7. Boundaries honored

No plan, SPIKE document, run artifact, `.run` directory, or SOP state was modified or
rerun. This file is the only addition.
