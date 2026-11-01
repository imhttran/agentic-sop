# ETOE-002 — Core workflow contract matrix (requirement-to-test audit)

Task: **ETOE-002** — Core workflow contract audit: state transitions, validation/review/FIX/retry, approval refusal, closure, and trace/metrics. **Read-only audit.**

## 1. Scope and method

- Scope is agentic-sop's own core lifecycle only: `internal/` + `cmd/sop`. The sibling repositories (sop-controller, sop-decision-adapters) are external context; no matrix rows are authored for their internals, and external adapter-driven decision behavior is classified as outside local test coverage (gap G4).
- Method: static source and test analysis. This audit executed **zero** real disposable runs. Every implementation reference was located this session by reading the file or by a repo-wide content search over `internal/` returning that exact file:line hit. Every test citation was verified present by observing the `func Test...` declaration (direct read or content-search hit). No citations were invented.
- Verification-level legend (used in the **Level** column; satisfies PRD acceptance criterion 3):
  - **U** — unit-verified: covered by in-process tests (domain unit tests, package unit tests, or CLI harness tests that stub the model/decision providers and seed fixtures).
  - **UR** — the asserted semantics are unit-verified, but the end-to-end occurrence additionally requires a real disposable run (not executed here).
  - **R** — requires a real disposable run: not locally executable in this task; **deferred by plan design to ETOE-005 (disposable dogfood fixture)**. The attempt-1 run artifacts under `docs/reports/end-to-end-reliability/ETOE-002-attempt-1-failure/` (trace.json, metrics.json, approval.json, classification.json, model-selection.json, activity.jsonl, report.json, state.json; listing verified this session) are cited as read-only shape evidence only, never as test coverage.
- Consulted contract documents (existence recorded in ETOE-001 §5): `docs/reference/CLI.md`, `docs/reference/STATUS-AND-RECOVERY.md`, `docs/specs/RECOVERY.md`. The authoritative transition contract is the source table `var transitions` in `internal/domain/task.go`.

## 2. Requirement inventory & coverage matrix

### 2.1 State transitions (authoritative table: `internal/domain/task.go`)

| ID | Requirement | Implementation reference | Covering test(s) | Level | Gap |
|----|-------------|--------------------------|------------------|-------|-----|
| A1 | PLANNED → READY | `internal/domain/task.go` (`transitions`, `Transition`) | `internal/domain/task_test.go` `TestLegalTransitions` | U | — |
| A2 | READY → BRANCH_CREATED | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A3 | BRANCH_CREATED → TESTS_WRITTEN | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A4 | TESTS_WRITTEN → RED_VERIFIED | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A5 | RED_VERIFIED → IMPLEMENTING | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A6 | IMPLEMENTING → LOCAL_TESTS_PASS | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A7 | IMPLEMENTING → FIX_REQUIRED | `internal/domain/task.go` | `TestLegalTransitions` (remediation cases) | U | — |
| A8 | LOCAL_TESTS_PASS → REVIEW | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A9 | LOCAL_TESTS_PASS → FIX_REQUIRED | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A10 | REVIEW → REVIEW_PASS | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A11 | REVIEW → FIX_REQUIRED | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A12 | REVIEW_PASS → PR_OPEN | `internal/domain/task.go` | `TestLegalTransitions` | U | — |
| A13 | REVIEW_PASS → LOCAL_DONE (local closure) | `internal/domain/task.go`; local drive `internal/cli/drive.go:911-927` (`localCompletionPath`, `completeTask`) | `TestLegalTransitions`; CLI fixtures `internal/cli/cli_test.go:1035-1049` (`TestRunGraphExecutesReadyTask`), `internal/cli/task_complete_test.go:83` | U | — |
| A14 | PR_OPEN → CI_RUNNING | `internal/domain/task.go` | `TestLegalTransitions` | UR | G3 |
| A15 | CI_RUNNING → CI_PASS | `internal/domain/task.go` | `TestLegalTransitions` | UR | G3 |
| A16 | CI_RUNNING → FIX_REQUIRED | `internal/domain/task.go` | `TestLegalTransitions` | UR | G3 |
| A17 | CI_PASS → MERGED | `internal/domain/task.go` | `TestLegalTransitions` | UR | G3 |
| A18 | MERGED → DONE | `internal/domain/task.go` | `TestLegalTransitions` | UR | G3 |
| A19 | FIX_REQUIRED → IMPLEMENTING (FIX retry loop) | `internal/domain/task.go`; review loop `internal/review`; CLI fix loop `internal/cli` | `TestLegalTransitions`; `internal/review/review_test.go` `TestLoopFixesThenPasses`; `internal/cli/cli_test.go` `TestRunFixLoopResolves:1082`, `TestRunFixLoopRepairsFailingValidation:1152`; `internal/cli/build_failure_test.go` `TestRunNonCompletedFixWithRedBuildIsAutoFixed:81`, `TestRunNonCompletedImplementWithRedBuildIsAutoFixed:204` | U | — |
| A20 | BLOCKED entry via `Block(reason)` only (reason required; completed-task refusal; unreachable via `Transition`) | `internal/domain/task.go` (`Block`) | `TestBlockRequiresReason`, `TestBlockSetsTerminalState`, `TestBlockRejectsCompletedTask`, `TestIllegalTransitions` (READY/IMPLEMENTING/FIX_REQUIRED → BLOCKED refused) | U | — |
| A21 | BLOCKED recovery via `Requeue` → PLANNED (spends attempt); `ErrRetryExhausted` when budget spent | `internal/domain/task.go` | `TestRequeue`, `TestRequeueBudgetExhausted`; CLI `internal/cli/cli_test.go` `TestRunRetryRequeuesBlocked:1963`, `TestRunRetryRejectsCompleted:1990`, `TestRunRetryBudgetExhausted:2044`, `TestRunGraphFailedBlocks:1935` | U | — |
| A22 | Continuation requeue returns to PLANNED **without** spending an attempt | `internal/domain/task.go` (`RequeueWithoutSpending`) | `TestRequeueWithoutSpending`, `TestContinuationRequeueDoesNotSpendAttempt`; CLI `TestRunGraphNeedsHumanNoProgressDoesNotSpend:2181` | U | — |
| A23 | BLOCKED(CONTINUATION_EXHAUSTED) is not auto-recovered; only explicit `sop retry` reopens it | `internal/domain/task.go` (`IsBlockedRecoverable`, `IsTerminalBlocked`) | **none found** in inspected domain/scheduler/CLI tests this session | UR (semantics) | **G5** |
| A24 | Explicit external completion → LOCAL_DONE, fails closed on completed task, no fabricated attempts/approval | `internal/domain/task.go` (`CompleteExternally`) | `internal/domain/complete_external_test.go`; `internal/cli/task_complete_test.go:83`; `internal/taskbuilder/taskbuilder_test.go:227-228` | U | — |
| A25 | MarkNotRequired → NOT_REQUIRED (only PLANNED/READY/BLOCKED; reason required; audited history) | `internal/domain/task.go` (`MarkNotRequired`) | `internal/domain/not_required_test.go` | U | — |
| A26 | Terminal states frozen; illegal `Transition` atomic/non-mutating; `CanTransitionTo` side-effect-free | `internal/domain/task.go` | `TestTerminalStatesRejectEveryTransition`, `TestIllegalTransitionIsAtomic`, `TestIllegalTransitions`, `TestCanTransitionToIsSideEffectFree` | U | — |
| A27 | Plan-completion predicate `AllSatisfied` (empty plan not complete) | `internal/domain/task.go` | `TestAllSatisfied`, `TestTaskIsSatisfied` | U | — |
| A28 | Dependency satisfaction: LOCAL_DONE/MERGED/DONE/NOT_REQUIRED satisfy; in-flight (CI_PASS, PR_OPEN, REVIEW_PASS, …) do not | `internal/domain/task.go` (`ResolveDependencies`) | `TestDependencyRequiresIntegratedWork`, `TestDependency_all_met/one_missing/partial_completion/empty`; `internal/orchestration/decision_test.go:131` | U | — |
| A29 | Scheduler selection contract: never selects completed (LOCAL_DONE/MERGED/DONE); retry-compatible | `internal/scheduler/scheduler.go:39,127` | `internal/scheduler/scheduler_regression_test.go:124`, `retry_compat_regression_test.go:72-120`, `integration_regression_test.go`, `guardrails_regression_test.go` | U | — |
| A30 | Reconciliation preserves executed lifecycle (LOCAL_DONE/BLOCKED/NOT_REQUIRED preserved; attempts kept) | `internal/planflow/historicalize.go:377,482` | `internal/planflow/reconcile_test.go:75-142`, `compiler_reconciliation_test.go:172-201`, `lifecycle_preservation_test.go:22`, `named_source_evidence_test.go:114-122`, `internal/continuation/continuation_test.go:221-235`, `internal/store/graph_test.go:171-177` | U | — |
| A31 | Resume/recovery (`sop retry`) recovers interleaved historically BLOCKED tasks to LOCAL_DONE | `internal/scheduler`; CLI recovery | `internal/cli/recovery_continue_test.go` (:15, :59, :128); `internal/scheduler/recovery_continue_test.go:44`; `internal/cli/cli_test.go TestRunRetryForceRaisesBudget:2058`, `TestRunRetryAll:2082` | U | — |
| A32 | `sop run` max-tasks bound is not a failure; un-entered task untouched; resume completes remainder | `internal/cli` run path | `internal/cli/run_max_tasks_test.go:100-307` | U | — |

### 2.2 Validation / review / FIX / retry flow

| ID | Requirement | Implementation reference | Covering test(s) | Level | Gap |
|----|-------------|--------------------------|------------------|-------|-----|
| V1 | Validation stage: configured validation commands run; fail-fast on failure | `internal/cli` validate command path | `internal/cli/cli_test.go` `TestRunValidatePass:631`, `TestRunValidateFailIsFailFast:643`, `TestRunValidateNoConfig:664`, `TestRunValidateNoCommands:675`, `TestRunValidateSubdirectoryCommand:1409` | U | toolchain execution *inside a live run*: G3/ETOE-005 |
| V2 | Validation failure enters the FIX loop and is retried; `sop run` graph requeues on validation failure | `internal/cli` run/fix loop | `TestRunValidationFailureEntersFixLoop:857`, `TestRunGraphValidationFailureRequeues:1056`, `internal/cli/build_failure_test.go` `TestRunNonCompletedFixExhaustsBudgetBoundedNotUnknown:106`, `TestRunRedBuildSkipsJEVUntilGreen:178` | U | — |
| V3 | Review gate: blocking findings prevent REVIEW_PASS; REVIEW → FIX_REQUIRED | `internal/review` (report parsing, severity blocking) | `internal/review/review_test.go` `TestReportBlocking:76`, `TestLoopPassesWithNoBlockingFindings:133`; `internal/cli/cli_test.go` `TestRunBlockingFindingNeedsHuman:878`, `TestRunReviewBlockingFinding:729`, `TestRunReviewNonBlockingFinding:744` | U | — |
| V4 | Review FIX loop: blocking finding → fix → re-review until pass; exhaustion and malformed output handled | `internal/review` loop | `TestLoopFixesThenPasses`, `TestLoopExhaustion`, `TestLoopFixBreaksTests`, `internal/review/malformed_test.go` `TestLoopMalformedOutputIsNeedsHuman`, `TestMalformedOutputNeverPasses`, `TestAgentProviderMalformedOutputBounded`, `TestAgentProviderRetriesThenSucceeds` | U | — |
| V5 | Validation exhaustion is classified (bounded, not unknown) | `internal/cli` classification | `internal/cli/classification_test.go` `TestRunValidationExhaustionIsClassified:200`, `TestRunGraphHardFailureStillBlocks:170`, `TestRunGraphContinueIsBounded:119` | U | — |
| V6 | Retry semantics: `sop retry` requeues blocked; refuses completed/not-blocked/unknown; `--force` raises budget; retry-all is bounded | `internal/cli` retry command; `internal/scheduler` | `internal/cli/cli_test.go` `TestRunRetryRequeuesBlocked:1963`, `TestRunRetryRejectsCompleted:1990`, `TestRunRetryRejectsNotBlocked:2004`, `TestRunRetryUnknownTask:2018`, `TestRunRetryForceRaisesBudget:2058`, `TestRunRetryAll:2082`, `TestRunRetryAllNothingBlocked:2127`; `internal/scheduler/retry_compat_regression_test.go` | U | — |
| V7 | Automatic recovery invariants: never bypass NEEDS-HUMAN, dependencies, or fabricate success; preserves working tree and run history | `internal/cli` recovery | `internal/cli/automatic_recovery_invariants_test.go:45-191` (`TestAutomaticRecoveryPreservesWorkingTreeAndRunHistory`, `TestAutomaticRecoveryDoesNotBypassNeedsHuman`, `TestAutomaticRecoveryDoesNotBypassDependencies`, `TestAutomaticRecoveryDoesNotFabricateSuccess`, `TestAutomaticRecoveryStopsAtFailedRecovery`) | U | — |

### 2.3 Approval and approval-refusal paths (`internal/approval`, `internal/cli`)

| ID | Requirement (each refusal path is its own row) | Implementation reference | Covering test(s) | Level | Gap |
|----|------------------------------------------------|--------------------------|------------------|-------|-----|
| P1 | Approve persists decision + history; request readable/applicable | `internal/approval` | `internal/approval/approval_test.go` `TestRequestIsReadableAndApplicable:86`, `TestApprovePersistsDecisionAndHistory:142`; CLI `internal/cli/approval_test.go` `TestHumanGateRecordsApprovableRequest:70`, `TestApproveRunRecordsBeforeContinuing:185` | U | — |
| P2 | Approve is idempotent | `internal/approval` | `TestApproveIsIdempotent:179`; CLI `TestCLIApprovalSupersedeIdempotent:175` | U | — |
| P3 | **Deny (approval refusal)**: decline persists decision and does **not** complete the task; lifecycle preserved | `internal/approval` | `TestDeclinePersistsAndDoesNotComplete:203`; CLI `TestDeclineHumanGatePreservesLifecycle:158`, `TestDeclineRunIsUsageError:173` | U | — |
| P4 | Approve resumes a BLOCKED task without spending retry budget | `internal/approval` + domain | `TestApproveResumesBlockedTaskWithoutSpendingRetry:236`; CLI `TestApproveBlockedTaskResumesIt:303` | U | — |
| P5 | Approve is scoped to its task (cross-task approval refused) | `internal/approval` | `TestApproveIsScopedToItsTask:259` | U | — |
| P6 | **Stale/superseded request refusal**: cannot be approved; not applicable | `internal/approval` | `TestStaleRequestCannotBeApproved:278`, `TestStaleRequestIsNotApplicable:301` | U | — |
| P7 | Supersede: request supersedes a resolved gate; idempotent for same pending gate; non-pending head refused ("already resolved") | `internal/approval`; CLI `sop approval supersede` | `TestRequestSupersedesResolvedGate:320`, `TestRequestIsIdempotentForSamePendingGate:352`; CLI `internal/cli/approval_supersede_test.go` `TestCLIApprovalSupersedeSuccess:64`, `TestCLIApprovalSupersedeRequiresReasonAndBy:103`, **`TestCLIApprovalSupersedeRefusals:125`** (incl. non-pending head), `TestCLIApprovalSupersedeMissingRecord:150`, `TestCLIApprovalExistingRecordsUnchanged:217` | U | — |
| P8 | **Missing-approval refusal**: unknown task not approvable; approve without request is an error | `internal/approval` | `TestUnknownTaskIsNotApprovable:375`; CLI `TestApproveWithoutRequestIsAnError:23`, `TestApprovalStatusWithoutRequest:53` | U | — |
| P9 | **Gate-block (approval pending)**: verified-but-unapproved work stops with "human approval required before commit"; approve does not bypass gates; interactive decision fails closed off TTY | `internal/cli` run loop; `internal/cli/run.go:868` writes gate.json | `internal/cli/cli_test.go:2311` (`TestVerifyFirstPassesWithoutAgent:2298` fixture), `TestRunCommitRequiresApproval:1195`, `TestRunCommitWithApproval:1215`, `TestRunCommitNoApprovalWhenDisabled:1233`, `TestRunPrRequiresApproval:1274`; `internal/cli/approval_test.go` `TestApproveDoesNotBypassGates:265`, `TestNeedsHumanClassificationWithoutRequestIsNotApproval:232`; `internal/cli/approval_interactive_test.go` `TestInteractiveDecisionFailsClosedOffTTY:77`, `TestInteractiveSelectRefusesNonApplicable:129`, `TestInteractiveDecisionCancelRecordsNothing:98`, `TestParkedRunNamesItsGate:227`; `TestApprovalsListsOnlyApplicableGates` (`internal/cli/approvals_test.go:58`), done tasks skip approvals `:63`; `TestAutomaticRecoveryDoesNotBypassNeedsHuman:77` | U | artifact emission: G1 |
| P10 | Gate classification output: tasks classified (e.g. LOCAL_DONE) with decision state | `internal/cli` decision/classification seam; `internal/cli/run.go:439` writes classification.json | `internal/cli/approval_classification_test.go:38-221` (`TestRunApprovalImplementationDiscoveryContinues`, `TestRunWaitingForHumanStageStaysHuman`, `TestRunActiveApprovalRequestStillBlocks`, `TestRunApprovalProseAloneContinues`); `internal/cli/classification_test.go` (reads classification.json via helper `:230`) | U | — |
| P11 | External `decision.provider` command adapters (real external decision behavior) | decision seam (process-level; deterministic default, per ETOE-001 §5) | **none locally** (out-of-process adapters, out of repo scope) | **R** | **G4** |
| P12 | End-to-end approval ordering in a live run (approval.json/classification.json persisted before commit; human gate observed in a real run) | run loop | archived shape evidence only (`ETOE-002-attempt-1-failure` approval.json, classification.json; ETOE-001) | **R** | **G4** (deferred ETOE-005) |

### 2.4 Closure / archive

| ID | Requirement | Implementation reference | Covering test(s) | Level | Gap |
|----|-------------|--------------------------|------------------|-------|-----|
| C1 | Completed-plan closure → archive with terminal vocabulary (LOCAL_DONE/DONE/MERGED), attempts and history preserved | `internal/planflow/historicalize.go` | `internal/planflow/historicalize_test.go:244`, `lifecycle_test.go:83`, `complete_test.go`, `historicalize_scope_test.go:179`; CLI `internal/cli/plan_complete_test.go`, `internal/cli/plan_historicalize_test.go:34` | U | — |
| C2 | Plan handoff on completion; unfinished plan stops; repeated run idempotent | `internal/cli` run path | `internal/cli/cli_test.go` `TestRunCompletedPlanHandsOff:1522`, `TestRunUnfinishedPlanStops:1575`, `TestRunPlainRunHandsOffCompletedPlan:1663`, `TestRunSamePlanContinuesAfterCompletion:1691`, `TestRunRepeatedRunIsIdempotent:1382` | U | — |
| C3 | Continuation vs attempt accounting: continuation requeue does not spend retries | `internal/domain/task.go` | `TestContinuationRequeueDoesNotSpendAttempt` | U | — |
| C4 | Live-run closure dispositions (archive disposition COMPLETE / failures closed on a real run) | CLI closure path | recorded behavior ETOE-001 §4; **no covering unit test found** | **R** | **G6** |

### 2.5 Trace / metrics

| ID | Requirement | Implementation reference | Covering test(s) | Level | Gap |
|----|-------------|--------------------------|------------------|-------|-----|
| T1 | trace.json written per run (best-effort orchestration observation) | `internal/runtrace/trace.go:3,28,253` (`FileName`, `Write`); collector `internal/cli/trace.go:16` | `internal/runtrace/trace_test.go` (valid JSON :39, stable fields :46, omit-empty replans :110, context summary :134) | UR | live emission ETOE-005 |
| T2 | Orchestration graph reconstructable from trace.json alone; secret scrubbing | `internal/runtrace/orchestration.go:10,100,228`, `orchestration_compose.go:129`, `orchestration_graph.go:66` | `trace_test.go` stable-field/composition assertions; `internal/cli/trace_test.go` **credential redaction** :202-205 | U | — |
| T3 | CLI trace reading/reporting (renders nothing when artifact absent) | `internal/cli/trace_report.go:15` | `internal/cli/trace_test.go:17-205` (loads `.agent-sdlc/runs/<id>/trace.json`) | U | — |
| T4 | metrics.json aggregate shape: absent/zero/present fixtures, reproducibility, malformed-artifact handling, category invariants, token-usage-not-policy | `internal/runmetrics/runmetrics.go:57,220-246`, `aggregate.go:71-79` | `internal/runmetrics/golden_test.go` `TestGoldenOutput` (+ `testdata/golden/{absent,zero,present}.json`), `runmetrics_test.go` (`TestCorpusAndMetricOrder`, `TestAbsentIsUnavailableAndReducesCoverage`, `TestZeroIsRecordedMeasurement`, `TestZeroArtifactPresenceClassification`, `TestTokenUsageUnavailable`, `TestReproducible`), `negative_test.go` (`TestMalformedArtifactsAreUnavailable`, `TestPartiallyPopulatedArtifact`, `TestMeasurementCategoryInvariants`, `TestTokenUsageNotPolicy`) | UR | live emission **G2** |
| T5 | metrics.json intentionally not production-wired yet | `internal/runmetrics/unwired_test.go` (`TestNoProductionImportOfRunmetrics:17`, `TestNoCLIModification:64`) | same (inverted guards prove absence of wiring) | UR | **G2** |
| T6 | activity.jsonl emission during run | `internal/activity` (recorder); `internal/cli/activity.go:116`; constant `internal/run/legacy_evidence.go:32` (`ActivityArtifactName`) | `internal/activity/activity_test.go` (recorder semantics :14-82); CLI `internal/cli/activity_test.go` `TestTaskActivityContextPersistsArtifact:109`, `TestRunStreamsActivityWhenEnabled:161`, `TestRunActivitySilentWhenDisabled:179`, `TestRenderActivityLine:18` | U | — (live-run context: ETOE-005) |
| T7 | gate.json emission at task gate | `internal/cli/run.go:868` (**single producer hit**; repo-wide content search of `internal/` found no test referencing gate.json) | **none found** | **R** | **G1** |
| T8 | classification.json emission at run termination | `internal/cli/run.go:430-439` (`rn.Write("classification.json", …)`) | `internal/cli/classification_test.go` (dedicated suite; helper `readClassification` :230 reads the artifact); fixture writes in `internal/cli/approval_test.go:246`, `internal/cli/autonomy_test.go:40` | U | — (live-run context: ETOE-005) |
| T9 | Decision/report boundaries (verification failure, no-progress, human boundary) drive classification & termination | `internal/cli` run loop; eval harness | `internal/cli/agentic_eval_test.go:54-118` (`TestAgenticEvalImplementationSuccess`, `TestAgenticEvalImplementationNoProgress`, `TestAgenticEvalHumanBoundary`, `TestAgenticEvalVerificationFailure`, `TestAgenticEvalContextSupplied`); `internal/cli/autonomy_test.go` (`TestHighAutonomyExhaustionIsTerminalNotHuman:55`, `TestBalancedAutonomyExhaustionStaysHuman:85`, `TestJEVFailureIsAutoRetryNotHuman:162`) | U | — (live-run context: ETOE-005) |

## 3. Explicit gaps (every requirement with no covering test)

| Gap | Requirement without covering test | Owning layer | Disposition |
|-----|-----------------------------------|---------------|-------------|
| G1 | gate.json artifact emission (T7): repo-wide content search found exactly one producer site (`internal/cli/run.go:868`) and **no test** referencing gate.json. No unit-level artifact-shape coverage; run presence unverifiable locally. | run/gate producer (`internal/cli` run loop) | Track for ETOE-005 run verification; unit-test candidate for a later, non-audit stage. |
| G2 | metrics.json live-run emission (T4/T5): the aggregate package is deliberately unwired from production (`TestNoProductionImportOfRunmetrics`). Shape/aggregate logic is unit-verified, but end-to-end emission during a real run has no covering test and cannot occur until wired. | metrics producer wiring (`internal/runmetrics` + run loop) | Deferred by plan design to ETOE-005; wiring is a coding-stage decision, out of scope for this read-only audit. |
| G3 | Full remote lifecycle PR_OPEN → CI_RUNNING → CI_PASS → MERGED → DONE inside a real repository with real PR/CI/merge (A14–A18); also real-toolchain validation inside a live run (V1). Transition legality is unit-verified; live execution is not locally testable. | remote lifecycle driver (`cmd/sop run`) + validation stage | Deferred to ETOE-005 (+ CI environment). |
| G4 | External `decision.provider` command adapters' real behavior (P11) and live approval ordering with artifacts (P12): process-level seam with deterministic default; genuine external decisions are out of local test coverage **by design**. | `internal/cli` decision seam + adapters | Classification only: run-level audit note for ETOE-005; no local test fix possible. |
| G5 | `IsBlockedRecoverable`/`IsTerminalBlocked` guard for BLOCKED(CONTINUATION_EXHAUSTED) (A23): no covering test identified in the inspected test files this session. | core lifecycle (`internal/domain`) + `sop retry` recovery path | Gap; candidate unit-test addition in a later stage. |
| G6 | Live-run closure dispositions (C4): archive disposition COMPLETE / failures closed observed on real runs only; no covering unit test found. | CLI closure path (`internal/cli`, `internal/planflow`) | Deferred to ETOE-005. |

 Corrections during S4: the original draft listed activity.jsonl emission (T6) and classification.json emission (T8) as missing-coverage candidates. Verification searches located dedicated coverage (`internal/cli/activity_test.go`, `internal/cli/classification_test.go`, producer `internal/cli/run.go:439`, constant `internal/run/legacy_evidence.go:32`), so those rows were reclassified as (U)nit-covered and the corresponding gap entries withdrawn; live-run context remains deferred to ETOE-005.

## 4. Unit-verified vs requires-real-disposable-run (summary)

- **Unit-verified (U):** the entire legal/illegal transition table incl. FIX loops and atomicity; Block/Requeue/RequeueWithoutSpending/CompleteExternally/MarkNotRequired; dependency satisfaction; scheduler selection and retry compatibility; reconciliation/lifecycle preservation; closure/archive unit paths incl. plan handoff; the validation/fix/retry CLI flow; the review engine incl. FIX loop, exhaustion, malformed handling; all local approval semantics incl. deny, stale/supersede, missing-approval, scoping, resume-without-spend, TTY fail-closed, gate-block ordering; trace.json shape/graph/redaction; activity.jsonl persistence; classification.json emission; metrics.json aggregate shape via goldens.
- **Requires a real disposable run (R / UR, deferred to ETOE-005):** live trace.json + gate.json emission inside a real run; metrics.json production emission (package deliberately unwired today); the remote PR/CI/merge lifecycle end-to-end; real decision.provider adapter behavior; live closure dispositions. Attempt-1 archive artifacts are shape evidence only.

## 5. Verification notes

To be appended after read-only validation spot-checks (go build / go test / go vet / gofmt) in S4.

## 6. Mutation statement

This audit is read-only. Its only intended repository change is this report file. No source, test, contract doc, or `.agent-sdlc` file was modified. Pre-existing worktree entries are user-owned and were left untouched.
