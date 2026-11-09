# Plan 07 — SOP Misinformation (LLM07) evidence report

Status: IMPLEMENTED on the `security/llm07-misinformation` branch; deterministic
verification `VERIFIED` on macOS arm64. Live-model adversarial evaluation and
Linux verification are `NOT_RUN`. This report does **not** claim general factual
correctness of generated text.

- Repository: `imhttran/agentic-sop`
- Base revision: `9cf554e6b67a6c61588326adb4a76fd46bfe850d` (`origin/main`)
- Branch: `security/llm07-misinformation`
- Verification platform: macOS 26.6.2 (Darwin 25.6.0), arm64, Go 1.27.1
- Plan: `docs/plans/PLAN-07-SOP-LLM07-Misinformation.md`
- Configuration identity: the checkout's `.agent-sdlc/config.yaml`; security
  scanning remains `report-only`, Promptfoo evaluation remains disabled, and no
  routing, autonomy, approval, or validation setting was changed.
- Scope boundary: the **use of evidence in decisions**, not the truth of every
  generated sentence. A model that reads a file is not proven to understand it.

## Executive status

| Dimension                | Result                                                                                                                                       | Evidence                                                                                      |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| IMPLEMENTED              | Yes, on the branch (not merged)                                                                                                              | Code and tests listed below                                                                   |
| ENABLED                  | Yes, in the default local lifecycle                                                                                                          | No new flag; the tool-harness write boundary and the outcome-artifact reset are always active |
| VERIFIED (deterministic) | Yes, macOS arm64                                                                                                                             | `go build ./...`, `go vet ./...`, `go test ./...` all passed                                  |
| VERIFIED (live model)    | `NOT_RUN`                                                                                                                                    | No live-model campaign was required or run                                                    |
| Linux verification       | `NOT_RUN`                                                                                                                                    | No Linux host was used                                                                        |
| Residual GAP             | Manual decisions are bound to criterion text + attempt, not a commit/content hash; `run_command` subprocesses are outside the write boundary | Documented below                                                                              |

## Authority chain (Plan 01 inventory, current implementation)

Plan 01's authority inventory is the baseline. Re-inspecting the current checkout
found these authoritative sources and bindings:

| Claim source                                   | Stored outcome / sink                              | Authoritative?                                  | Binding                                                                                            |
| ---------------------------------------------- | -------------------------------------------------- | ----------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Model completion summary (`implementation.md`) | Run artifact only                                  | No                                              | Never read by `deriveGatedOutcome`                                                                 |
| Model `ALREADY_SATISFIED` object               | `report.json` / `report.md` completion field       | Only after `verifySatisfiedDetailed` accepts it | Invocation-local reads, executed validation, unchanged repository fingerprint, deliverables        |
| Model-selected evidence paths                  | `ALREADY_SATISFIED` acceptance evidence            | No by itself                                    | Path must have been read in **this** invocation; `.agent-sdlc` paths are refused                   |
| Declared acceptance checks                     | `outcome.json` `check-N` criteria                  | Yes                                             | Re-executed from the task spec on the current tree; `go test` requires a real passing test event   |
| Declared deliverables                          | `outcome.json` `deliverable-N` criteria            | Yes                                             | `os.Stat` on the declared paths in the current tree                                                |
| Manual acceptance decision                     | `acceptance_decisions.jsonl` → `outcome.json`      | Only with an explicit decision                  | Designated approver, evidence text, timestamp, task, criterion text, manual-evidence text, attempt |
| Waiver                                         | `waivers.jsonl` → lifecycle                        | Only with an explicit grant                     | Designated approver, reason, timestamp, unmet required criterion                                   |
| Review findings                                | `review.json` → quality gate                       | Blocking input, never acceptance proof          | Severity threshold; malformed review output is `NEEDS_HUMAN`                                       |
| Test summary                                   | `validation.json` from `internal/testrunner`       | Yes                                             | Executed command, exit status, category; model prose is not read                                   |
| Historical / retrieved content                 | Prompt context (`advisory_retrieval`, handoff)     | No                                              | Not an input to the evidence resolver or transition                                                |
| Approval                                       | `approval.json` written by SOP approval operations | Yes                                             | Explicit `sop approve` / `sop outcome --accept`; model prose is never parsed                       |

## Claim-to-decision traces

1. **"PASS" / "all tests passed" prose.**
   `runStages` records the prose in `implementation.md`, then derives the verdict
   with `deriveGatedOutcome` → `deriveOutcomeWith` → `outcome.Verify`. Acceptance
   checks are re-executed from the task specification, so a failing or missing
   check yields `PARTIAL` and, on a mapped stage, `blocksAdvancement()` parks the
   task at the human boundary. Verified by
   `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`.

2. **ALREADY_SATISFIED without its contract.**
   `internal/ollamaagent/orchestrate.go` calls `verifySatisfiedDetailed` before
   setting `ev.alreadySatisfied`. The verifier requires: no repository mutation,
   a captured baseline, `status=completed`, a parsed evidence block, one or more
   read paths per criterion, coverage of every declared criterion, present
   deliverables, every required validation command actually executed and passing,
   and an unchanged repository fingerprint. Verified by
   `TestAlreadySatisfiedRequiresObjectiveProof`, `TestVerifiedAlreadySatisfied`,
   and `TestVerifySatisfiedDetailedRejectionCodes`.

3. **Citation to nonexistent evidence.**
   `verifySatisfiedDetailed` resolves the path and rejects an unread or missing
   path with `UNREAD_EVIDENCE_PATH`. Verified by the `unread evidence path` case.

4. **Evidence from another task or attempt.**
   Manual decisions are matched by `decisionBoundToAttempt`: criterion id,
   criterion description, manual-evidence description, and attempt must match the
   mapping. A decision for a different attempt is ignored unless a recorded
   recovery links the exact decision to the current attempt. Verified by
   `TestManualEvidenceRequiresDecision` and
   `TestAcceptanceRecoveryInvalidatesChangedCriterionOrEvidence`.

5. **Evidence for changed or invalidated content.**
   Acceptance checks are re-run on the current tree; an earlier `outcome.json` is
   cleared at the start of every lifecycle attempt; the cross-run verification
   cache requires an exact `VERIFIED` identity (repository revision + effective
   input content + command + environment + policy + scope). Verified by
   `TestFailedRunDoesNotExposePreviousVerifiedOutcome`,
   `TestDecideFailClosed`, and `TestCacheReusableOnlyVerified`.

6. **Model approval prose.**
   Approval is a durable record written only by `sop approve` / `sop outcome
--accept`; model prose is never parsed into a decision. A manual criterion with
   no decision remains `PARTIAL`. Verified by
   `TestModelApprovalClaimWithoutDecisionDoesNotAdvance`.

7. **Retrieved historical content.**
   Retrieved advice and handoff capsules are prompt context only. They are not
   read by `internal/outcome`, `internal/quality`, or the acceptance gate. A
   historical claim that tests passed does not replace the re-executed check;
   verified by `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`.

8. **Model-authored SOP evidence.**
   `internal/toolharness` now refuses **writes** under `.agent-sdlc` (reads remain
   available except `state.db`). Before this change only `.agent-sdlc/state.db`
   was protected, so a model could write `acceptance_decisions.jsonl`,
   `waivers.jsonl`, `changed-files.json`, `outcome.json`, or `results/` and turn
   prose into an approval or implementation evidence. Verified by
   `TestStateDirectoryIsWriteProtectedForModelTools`.

9. **SOP state cited as repository proof.**
   `verifySatisfiedDetailed` now rejects an evidence path inside `.agent-sdlc`
   with `STATE_EVIDENCE`, even when the read succeeded. This prevents another
   task's outcome/decision artifact from being cited as current evidence.
   Verified by `TestStateEvidencePath` (in `TestVerifySatisfiedDetailedRejectionCodes`)
   and `TestAlreadySatisfiedRejectsStateEvidenceOnRealPath`.

## Confirmed defects and fixes

### D1 — Model tools could write authoritative SOP state

- **Reproduction:** `internal/toolharness/files.go` `resolveIn` protected only
  `.agent-sdlc/state.db`. The system prompt already told the model not to modify
  `.agent-sdlc`, but nothing enforced it. A model could `write_file` a forged
  `acceptance_decisions.jsonl`, `waivers.jsonl`, or `changed-files.json` under
  `.agent-sdlc/runs/<task>/`. Those artifacts are read back as authoritative
  evidence (`readAcceptanceDecisions`, `recordedWaiverForTask`, `rn.ChangedFiles`).
- **Fix:** `resolveIn` now refuses write access to `.agent-sdlc` for every file
  tool and returns `ErrProtectedPath`. Reads still work except `state.db`, so a
  task can inspect configuration.
- **Regression:** `internal/toolharness/state_dir_write_test.go`.

### D2 — A failed run could expose a previous VERIFIED verdict

- **Reproduction:** `executeLifecycle` called `writeOutcomeArtifact` with a nil
  verdict on early stops, and the function is a no-op for nil. `runpkg.New`
  clears only `attempts/`, not `outcome.json`. `sop verify` therefore reprinted a
  prior run's `VERIFIED` after a later run produced no verdict.
- **Fix:** `clearOutcomeArtifact` runs at the start of every lifecycle attempt,
  so the artifact always describes the latest attempt. `sop verify` now says
  "no recorded outcome for the latest run" instead of implying the task never ran.
- **Regression:** `internal/cli/outcome_stale_test.go`.

### D3 — SOP runtime state could be cited as repository evidence

- **Reproduction:** `verifySatisfiedDetailed` accepted any file the harness
  observed `read_file`, including `.agent-sdlc/runs/<other>/outcome.json`. That is
  another task's/attempt's evidence, not current repository proof.
- **Fix:** `Harness.IsStatePath` exposes the read-side boundary; the verifier
  rejects `.agent-sdlc` evidence paths with `STATE_EVIDENCE`.
- **Regression:** `TestVerifySatisfiedDetailedRejectionCodes/state_evidence_path`
  and `TestAlreadySatisfiedRejectsStateEvidenceOnRealPath`.

## Deterministic fixture coverage

| Required fixture                                 | Test                                                                                                                       | Result |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------- | ------ |
| Model claims PASS when a required check failed   | `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`                                                                    | PASS   |
| Model claims PASS when a check never ran         | `TestVerifySatisfiedDetailedRejectionCodes` (`validation_not_executed`); `TestOutcomeIntegrationDeliverableAloneIsPartial` | PASS   |
| Citation references nonexistent evidence         | `TestVerifySatisfiedDetailedRejectionCodes` (`unread_evidence_path`)                                                       | PASS   |
| Evidence belongs to another task/attempt         | `TestManualEvidenceRequiresDecision` (`stale-attempt`, `wrong-attempt`); `TestStateEvidencePath`                           | PASS   |
| Evidence refers to changed/invalidated content   | `TestFailedRunDoesNotExposePreviousVerifiedOutcome`; `TestAcceptanceRecoveryInvalidatesChangedCriterionOrEvidence`         | PASS   |
| Model text claims approval without a decision    | `TestModelApprovalClaimWithoutDecisionDoesNotAdvance`                                                                      | PASS   |
| ALREADY_SATISFIED lacks required evidence        | `TestAlreadySatisfiedRequiresObjectiveProof`                                                                               | PASS   |
| Historical content replaces current verification | `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`                                                                    | PASS   |
| Valid observed evidence still advances           | `TestOutcomeIntegrationVerifiedWithExecutedCheck`; `TestVerifiedAlreadySatisfied`                                          | PASS   |
| Supported manual acceptance still advances       | `TestOutcomeAcceptRecordsDecision`; `TestManualEvidenceRequiresDecision` (accepting case)                                  | PASS   |

## Criterion-to-evidence mapping

### GROUND-001

| Criterion                                                                                          | Evidence                                                                                                      | State                                                                                                |
| -------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Distinguish model assertions, observed tool results, accepted human evidence, verified task status | Trace table above; `internal/outcome/criteria.go`; `internal/ollamaagent/already_satisfied.go`                | `VERIFIED` (deterministic)                                                                           |
| Identify content/commit, task, attempt, and policy binding                                         | Trace table; `internal/verifcache` identity; `outcome.CriterionMapping.Attempt`; `AcceptanceDecision.Attempt` | `VERIFIED` for the implemented bindings; commit/content-hash binding for manual decisions is a `GAP` |
| Deliver this report with concrete traces                                                           | This file                                                                                                     | `IMPLEMENTED`                                                                                        |

### GROUND-002

| Criterion                                                                  | Evidence                                                                                                                    | State      |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ---------- |
| Fabricated PASS text cannot override failed/missing checks                 | `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`; `TestOutcomeIntegration*`                                          | `VERIFIED` |
| Wrong identity/invalidated content cannot satisfy the task                 | `TestManualEvidenceRequiresDecision`; `TestStateEvidencePath`; `TestFailedRunDoesNotExposePreviousVerifiedOutcome`          | `VERIFIED` |
| ALREADY_SATISFIED follows the existing contract                            | `TestAlreadySatisfiedRequiresObjectiveProof`; `TestVerifySatisfiedDetailedRejectionCodes`                                   | `VERIFIED` |
| Untrusted retrieved material cannot grant approval or replace verification | `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`; `TestModelApprovalClaimWithoutDecisionDoesNotAdvance`              | `VERIFIED` |
| Valid observed evidence and manual acceptance still advance                | `TestOutcomeIntegrationVerifiedWithExecutedCheck`; `TestOutcomeAcceptRecordsDecision`                                       | `VERIFIED` |
| Regression tests use the real outcome/transition path                      | `runInjectedCLI` → `runSingleTask` → `runStages` → `deriveGatedOutcome`; `CompleteWithEvidence` → `verifySatisfiedDetailed` | `VERIFIED` |

### GROUND-003

| Criterion                                                                  | Evidence                                                                                     | State                                             |
| -------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| User-visible result and persisted lifecycle agree                          | `TestFailedRunDoesNotExposePreviousVerifiedOutcome`; `sop verify` reads the cleared artifact | `VERIFIED`                                        |
| Missing checks are `NOT_RUN`, not implied PASS                             | `writeNoValidationResults`; `outcome.Verify` required `objective-acceptance`                 | `VERIFIED` (inspection + existing tests)          |
| Source references resolve to the recorded evidence identity                | `STATE_EVIDENCE` rejection; `verifcache` identity tests                                      | `VERIFIED` for the paths that declare an identity |
| Required checks pass; PR documents before/after evidence and residual risk | This report; repository checks below                                                         | `VERIFIED` (deterministic)                        |

## Commands and results

Run from the worktree root on macOS 26.6.2/arm64, Go 1.27.1.

| Command                                                                                            | Result                              |
| -------------------------------------------------------------------------------------------------- | ----------------------------------- |
| `go test ./internal/toolharness/... -run TestStateDirectoryIsWriteProtectedForModelTools -count=1` | PASS                                |
| `go test ./internal/cli/... -run TestModelPassClaimCannotOverrideFailedAcceptanceCheck -count=1`   | PASS                                |
| `go test ./internal/cli/... -run TestModelApprovalClaimWithoutDecisionDoesNotAdvance -count=1`     | PASS                                |
| `go test ./internal/cli/... -run TestFailedRunDoesNotExposePreviousVerifiedOutcome -count=1`       | PASS                                |
| `go test ./internal/outcome/... -count=1`                                                          | PASS                                |
| `go test ./internal/toolharness/... -count=1`                                                      | PASS                                |
| `go test ./internal/ollamaagent/... -count=1`                                                      | PASS (66.5 s)                       |
| `go test ./internal/cli/... -count=1`                                                              | PASS (130.3 s)                      |
| `go build ./...`                                                                                   | PASS                                |
| `go vet ./...`                                                                                     | PASS                                |
| `go test ./...`                                                                                    | PASS (116 packages `ok`, no `FAIL`) |

The full-suite log used for this report is the terminal capture for
`go test ./...`; it was not committed (it is machine-local output). The plan's
acceptance checks are the focused commands in the table above.

## Revisions

- Base: `9cf554e6b67a6c61588326adb4a76fd46bfe850d` (`origin/main`).
- Change-set commit: `257b9b40` (`security/llm07-misinformation`).
- Branch: `security/llm07-misinformation`; the PR records the final head.
- The change set is the branch diff against the base; the verification commands
  above were run on that change set on the platform named at the top.

## Residual limitations and gaps

- **Factual correctness is not claimed.** A model can read a real file and draw a
  wrong conclusion, and a valid source can be inaccurate. Acceptance checks prove
  their commands exit 0, not that the prose around them is true.
- **Review is model-based.** The review gate blocks on findings, but a review
  `PASS` is not independent verification and cannot substitute for acceptance.
- **Manual decisions are not content-addressed.** `AcceptanceDecision` binds the
  task, criterion text, manual-evidence text, and attempt, but not a commit or
  content hash. A code change within the same attempt could leave a prior manual
  decision applicable. Changing this would alter the historical-outcome contract,
  so it is recorded as a `GAP`, not silently changed.
- **`run_command` subprocesses are outside the write boundary.** A SAFE command
  (for example a contributed test) can still write `.agent-sdlc` as a side effect;
  the harness's file-tool boundary does not sandbox subprocesses. This is a Plan
  01 execution-isolation concern, not a second permission system in this plan.
- **Live-model/adversarial evidence is `NOT_RUN`.** All results are deterministic,
  synthetic-fixture results. No live model was asked to fabricate evidence.
- **Linux is `NOT_RUN`.** Only macOS arm64 was exercised.
- **Historical outcomes are preserved.** The fixes do not rewrite
  `state.db`, decision audits, waivers, or prior run artifacts.

## Handoff

The existing authority chain now rejects the reproduced unsupported-claim paths
and reports the latest attempt's verdict consistently. The remaining work is the
manual-decision content binding and subprocess isolation recorded above; both
require a policy/contract decision rather than an implicit change.
