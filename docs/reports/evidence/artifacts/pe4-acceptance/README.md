# PE-4 live acceptance — complete

Live acceptance for `plan-executore-integration` PE-4 (Recovery and Hardening), run on
merged `main` at `de9d4df9` (merge of PR #132, `fix/pe4-planner-epoch-binding`).

The gated branch head `5acf07bc` and the merge commit have the identical tree
(`53b4cc4eedbdda8b9e542e93ccc63d1ba7d06368`), so the `make check` evidence and the Codex
review of `5acf07bc` apply to the merged revision unchanged.

## Environment

- Planner binary rebuilt from `de9d4df9` into `.run/spike/sop-planner-main`.
- Fixture `<FIXTURE>`: an isolated disposable SOP project with its own git repository,
  question store, and run state. It was never the main checkout.
- Transport: OpenAI Secure MCP Tunnel, tunnel id `tunnel_6ad00777bf3c8191a88091a07a0617b3`,
  seven-tool planner surface. During acceptance the tunnel served the fixture; it was
  restored to the main checkout afterwards and verified with `readyz` 200.
- Plan `plan@a09bfa55b609c235c9a7ce28f8c7557ce73fb65fb726bf90539230594a4e53ba`, task `PB-1`.

## Preflight proof (throwaway probe)

`<PROBE>` declared `go test ./...` and ran `sop run` before the acceptance fixture raised
a question: no setup blocker, model called, validation ran, task `PX-1` reached `PASSED` /
`VERIFIED` / `LOCAL_DONE`. `test -f` was never used as an acceptance check.

## Fixture and binding

| Field | Value |
| --- | --- |
| task | `PB-1`, attempt `0/3` |
| question | `q-1`, revision `1` |
| plan revision | `plan@a09bfa55b609c235c9a7ce28f8c7557ce73fb65fb726bf90539230594a4e53ba` |
| return stage | `IMPLEMENTING` |
| **question_run_epoch** | **`2026-10-26T17:24:12.633178Z`** |
| durable run epoch | `2026-10-26T17:24:12.633178Z` |

The question epoch and `state.json created_at` were equal and non-empty — the PE-4 ordering
fix in live effect. Under PE-3 the same fixture recorded `run_epoch: null`.

## Connector submission

The external planner submitted `a-2` (action `continue`, body `accepted`) through the
connector. The operator did not submit or resubmit it. The persisted answer, outcome
`o-3 ACCEPTED`, and binding match the report exactly. See `connector-submission.md`.

## Epoch-bound resume

1. `sop planner continue --apply` → `PB-1 APPLIED q-1 (continuation pending execution)`,
   outcome `o-4 APPLIED`, continuation `cont:q-1:2026-10-26T17:24:12.633178Z` `PENDING`
   at `return_stage IMPLEMENTING`.
2. `sop run --max-tasks 1` in the fixture → `PB-1 planner: resumed from IMPLEMENTING and
   completed (generation 1)`.

| Condition | Result |
| --- | --- |
| `accepted.txt` | `accepted` (9 bytes: `accepted\n`) |
| `go test ./...` | `ok pe4probe` |
| continuation | `cont:q-1:2026-10-26T17:24:12.633178Z` **`COMPLETED`**, generation 1, attempts 1 |
| continuation `completed_at` | `2026-10-26T17:34:35.887999Z` |
| task | `PB-1` `LOCAL_DONE` |
| run | `PASSED`; `created_at` unchanged at `2026-10-26T17:24:12.633178Z` |
| outcome / validation / gate | `VERIFIED` / `PASS` / `PASS` |
| history | `q-1` `APPLIED`; `a-2`, `o-3`, `o-4` preserved |

## No-duplicate-work result

- `sop planner continue` (read-only) → `SKIP_TERMINAL task is in a terminal state`.
- A second `sop run --max-tasks 1` → `all tasks done (0 completed this run)`; gate `PASS`.
- `accepted.txt` SHA-256 unchanged (`4825c38ba9e071bc3e19961e7c1bd0c1a2fcc575a5cff7e416d7f7c772597271`),
  continuation still `COMPLETED` generation 1 with the same `completed_at`, answers still 1,
  outcomes still 2.

No model call and no second completion occurred.

## Review evidence

PR #132 merged as `de9d4df9`. GitHub's formal `reviews` collection is empty, so the merge is
**not** offered as proof of review. The review is the repository's Codex connector summary
on reviewed head `5acf07bc`: `code_review.status: completed`,
`has_any_findings: false`, `has_unresolved_findings: false`. The merge tree equals the
reviewed tree. See `review-evidence.md`.

## Final task states

- `PE-1` `LOCAL_DONE`
- `PE-2` `LOCAL_DONE`
- `PE-3` `LOCAL_DONE`
- `PE-4` `LOCAL_DONE` (external completion at `de9d4df9`, validation `PASS`)

Main history preserved: `q-1` and `q-5` `APPLIED`; `cont:q-1:` and
`cont:q-5:2026-10-23T22:12:25.357618Z` `EXTERNALLY_COMPLETED`.

## Deterministic recovery tests vs live acceptance

The live checks above exercise the connector decision, the epoch-bound continuation, the
durable resume, and the no-duplicate-work rerun. The remaining PE-4 recovery semantics are
established deterministically, not by live fault injection:

| Criterion | Deterministic tests | Live check |
| --- | --- | --- |
| No duplicate execution | `TestMCPDecisionDuplicateIsIdempotent`, `TestDuplicateSubmissionDeterministic`, `TestApplyIdempotentNoDoubleDispatch`, `TestContinuationConcurrentClaimOneWins` | rerun completed 0 tasks, artifact hash unchanged |
| Stale decisions cannot affect newer attempts | `TestMCPDecisionAttemptBindingIsStale`, `TestMCPDecisionStaleRunEpochIsRefused`, `TestCLIAnswerStaleBindingPreservesQuestion`, `TestCLIAnswerSupersededRunStale`, `TestCLIAnswerMissingRunStale` | server-side binding only |
| Uncertain outcome reconciled from durable state | `TestContinuationRecoveryCrashCases`, `TestClaimReclaimAndGenerationFencing`, `TestFenceAndMutateRejectsReclaimedWorker`, `TestExecutionLockBlocksReclaimedWorkerWhileRunning`, `TestPlannerRecoveryUncertainExecutionStopsForReview`, `TestPlannerRecoveryDefersToHeldExecutionOwnership` | continuation persisted and consumed once |
| Interrupted tasks resume from checkpoints | `TestPlannerRestartWhileWaitingReparks`, `TestCLIPlannerCrashAfterApplyBeforeContinue`, `TestCLIPlannerCrashBeforeApply`, `TestAttemptsAreResetPerRun`, `TestPlannerResumeCountsOneCompletion` | parked then resumed in the same epoch |
| Approval boundaries enforced | `TestMCPPlannerSurfaceShape`, `TestPlannerHumanGatePendingApprovalBlocksAsk`, `TestPlannerAnswerCannotSatisfyHumanGate`, `TestPlannerHumanGateBlockedTaskRemainsGated` | seven-tool surface has no approval operation |
| Evidence attributable to the attempt | `TestDischargeExternalContinuationIsTerminalAndPreservesHistory`, `TestCaptureOutcomeIsIdempotent`, `TestPlannerResumeSavesHandoffCapsule` | answer, outcomes, epoch, run all attributed |
| Existing SOP gates | `make check` pass on `5acf07bc`; identical merge tree | focused recovery suite pass on `de9d4df9` |

## Files

| File | Contents |
| --- | --- |
| `questions.json` | final `q-1` (`APPLIED`), `a-2`, `o-3`, `o-4`, `COMPLETED` epoch-bound continuation |
| `run-state.json` | run `PASSED`, original `created_at` preserved |
| `plan.meta.json` | the plan revision every decision was bound to |
| `PLAN.md`, `acceptance_test.go`, `go.mod`, `config.yaml` | the fixture definition |
| `accepted.txt` | the produced deliverable |
| `outcome.json`, `validation.json`, `gate.json`, `report.md`, `report.json` | the run verdict and evidence |
| `planner-effect-*.json` | continuation start and completion markers |
| `connector-submission.md` | the external submission and persisted match |
| `review-evidence.md` | PR #132 merge and Codex review artifact |
| `preflight-probe-report.md` | the throwaway preflight proof |
| `read-surface.jsonl` | `sop_task_result` and `sop_decision_list` output |
| `resume-run.log`, `no-duplicate-run.log` | resume and no-duplicate-work output |
| `fixture-status.txt`, `question-q1.txt` | pre-consumption fixture state |
| `manifest.sha256` | SHA-256 of every archived file |

## Sanitization

Absolute local paths were replaced with `<FIXTURE>`, `<PROBE>`, `<CHECKOUT>`, and `<LOCAL>`.
No credentials, API keys, or tokens are included. The `/tmp` source files were preserved
unchanged; only this archive copy was sanitized.
