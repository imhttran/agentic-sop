# Plan 07 — SOP Misinformation (LLM07) evidence report

Status: COMPLETE for the deterministic scope. The operator selected option 1
(strict content binding) for manual acceptance evidence; it is implemented and
verified. Deterministic verification PASS on macOS arm64; live-model adversarial
evaluation and Linux verification are `NOT_RUN`. This report does **not** claim
general factual correctness of generated text.

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
- SOP lifecycle (`.agent-sdlc/state.db`, plan
  `plan-07-sop-llm07-misinformation`): all three tasks are `LOCAL_DONE` via
  external completion at `dcb553c8` (validation PASS); the plan is `COMPLETE`
  and archived at `.agent-sdlc/archive/plan-07-sop-llm07-misinformation`.
  No approval was fabricated.

## Executive status

| Dimension                | Result                                                                                                                     | Evidence                                                                                                             |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| IMPLEMENTED              | Yes, on `security/llm07-content-binding` (follow-up; not merged)                                                           | Code and tests listed below                                                                                          |
| ENABLED                  | Yes, in the default local lifecycle                                                                                        | No new flag; the tool-harness write boundary and the outcome-artifact reset are always active                        |
| VERIFIED (deterministic) | Yes, macOS arm64; external completions reported validation PASS (GROUND-001 at `e93751ef`; GROUND-002/003 at `dcb553c8`)   | `go build ./...`, `go vet ./...`, `go test ./...`                                                                    |
| VERIFIED (live model)    | `NOT_RUN`                                                                                                                  | No live-model campaign was required or run                                                                           |
| Linux verification       | `NOT_RUN`                                                                                                                  | No Linux host was used                                                                                               |
| SOP lifecycle            | `COMPLETE`: GROUND-001/002/003 `LOCAL_DONE` (external); plan archived                                                      | `.agent-sdlc/runs/GROUND-00{1,2,3}/external-completion.json`; `.agent-sdlc/archive/plan-07-sop-llm07-misinformation` |
| Resolved decision        | Strict content binding (Plan 07 D4, option 1) is implemented; a decision is honored only for the reviewed content identity | D4 below                                                                                                             |
| Subprocess boundary      | `run_command` subprocesses are outside the file-tool write boundary                                                        | `TestSubprocessStateWriteIsOutsideFileToolBoundary`                                                                  |

## Authority chain (Plan 01 inventory, current implementation)

Plan 01's authority inventory is the baseline. Re-inspecting the current checkout
found these authoritative sources and bindings:

| Claim source                                   | Stored outcome / sink                              | Authoritative?                                  | Binding                                                                                                             |
| ---------------------------------------------- | -------------------------------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Model completion summary (`implementation.md`) | Run artifact only                                  | No                                              | Never read by `deriveGatedOutcome`                                                                                  |
| Model `ALREADY_SATISFIED` object               | `report.json` / `report.md` completion field       | Only after `verifySatisfiedDetailed` accepts it | Invocation-local reads, executed validation, unchanged repository fingerprint, deliverables                         |
| Model-selected evidence paths                  | `ALREADY_SATISFIED` acceptance evidence            | No by itself                                    | Path must have been read in **this** invocation; `.agent-sdlc` paths are refused                                    |
| Declared acceptance checks                     | `outcome.json` `check-N` criteria                  | Yes                                             | Re-executed from the task spec on the current tree; `go test` requires a real passing test event                    |
| Declared deliverables                          | `outcome.json` `deliverable-N` criteria            | Yes                                             | `os.Stat` on the declared paths in the current tree                                                                 |
| Manual acceptance decision                     | `acceptance_decisions.jsonl` → `outcome.json`      | Only with an explicit decision                  | Designated approver, evidence, timestamp, task, criterion text, manual-evidence text, attempt, **content identity** |
| Waiver                                         | `waivers.jsonl` → lifecycle                        | Only with an explicit grant                     | Designated approver, reason, timestamp, unmet required criterion                                                    |
| Review findings                                | `review.json` → quality gate                       | Blocking input, never acceptance proof          | Severity threshold; malformed review output is `NEEDS_HUMAN`                                                        |
| Test summary                                   | `validation.json` from `internal/testrunner`       | Yes                                             | Executed command, exit status, category; model prose is not read                                                    |
| Historical / retrieved content                 | Prompt context (`advisory_retrieval`, handoff)     | No                                              | Not an input to the evidence resolver or transition                                                                 |
| Approval                                       | `approval.json` written by SOP approval operations | Yes                                             | Explicit `sop approve` / `sop outcome --accept`; model prose is never parsed                                        |

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
   cleared at the start of every lifecycle attempt and by a supported external
   completion; the cross-run verification cache requires an exact `VERIFIED`
   identity (repository revision, effective input content, command, environment,
   policy, and scope). Verified by
   `TestFailedRunDoesNotExposePreviousVerifiedOutcome`,
   `TestExternalCompletionClearsStaleOutcome`, `TestDecideFailClosed`, and
   `TestCacheReusableOnlyVerified`. For **manual acceptance decisions**, the decision
   is additionally bound to the content-complete repository identity (D4), so a
   same-attempt content change requires re-acceptance. Verified by
   `TestManualDecisionContentBinding` and
   `TestLegacyDecisionWithoutContentIdentityRequiresReacceptance`.

6. **Model approval prose.**
   Approval is a durable record written only by `sop approve` / `sop outcome --accept`;
   model prose is never parsed into a decision. A manual criterion with no decision
   remains `PARTIAL`. Verified by `TestModelApprovalClaimWithoutDecisionDoesNotAdvance`.

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
   prose into an approval or implementation evidence. The boundary is resolved by
   filesystem identity as well as path string, so a case alias (`.AGENT-SDLC` on a
   case-insensitive filesystem) or a symlink into the state directory cannot bypass
   it; `list_files`/`search_files` hide `state.db` and do not traverse the state
   directory. Verified by `TestStateDirectoryIsWriteProtectedForModelTools` and
   `TestStateDirectoryAliasesAreRecognized`.

9. **SOP state cited as repository proof.**
   `verifySatisfiedDetailed` now rejects an evidence path inside `.agent-sdlc`
   with `STATE_EVIDENCE`, even when the read succeeded. This prevents another
   task's outcome/decision artifact from being cited as current evidence. The
   check uses filesystem identity, so a case alias or symlink cannot substitute a
   repository path for state. Verified by `TestStateEvidencePath` (in
   `TestVerifySatisfiedDetailedRejectionCodes`) and
   `TestAlreadySatisfiedRejectsStateEvidenceOnRealPath`.

10. **Subprocess writes are outside the file-tool boundary.**
    A SAFE `run_command` (for example `go test ./...`) executes a subprocess that
    the harness does not sandbox for `.agent-sdlc` writes. The file tools refuse
    the state directory, but a contributed test or build step can still write it
    as a side effect. `TestSubprocessStateWriteIsOutsideFileToolBoundary` records
    this current boundary; the plan does not claim comprehensive protection
    against model-authored state. This is a Plan 01 execution-isolation concern.

## Confirmed defects and fixes

### D1 — Model tools could write authoritative SOP state

- **Reproduction:** `internal/toolharness/files.go` `resolveIn` protected only
  `.agent-sdlc/state.db`. The system prompt already told the model not to modify
  `.agent-sdlc`, but nothing enforced it. A model could `write_file` a forged
  `acceptance_decisions.jsonl`, `waivers.jsonl`, or `changed-files.json` under
  `.agent-sdlc/runs/<task>/`. Those artifacts are read back as authoritative
  evidence (`readAcceptanceDecisions`, `recordedWaiverForTask`, `rn.ChangedFiles`).
- **Fix:** `resolveIn` now refuses write access to `.agent-sdlc` for every file
  tool and returns `ErrProtectedPath`. The check is resolved by filesystem
  identity as well as path string, so a case alias (`.AGENT-SDLC`) on a
  case-insensitive filesystem and a symlink into the state directory are refused;
  `list_files`/`search_files` also hide `state.db` by identity. Reads still work
  except `state.db`, so a task can inspect configuration.
- **Regression:** `internal/toolharness/state_dir_write_test.go`, including
  `TestStateDirectoryAliasesAreRecognized` and a positive control for ordinary
  repository writes and configuration reads.
- **Boundary:** this is the **file-tool** boundary. A `run_command` subprocess is
  not sandboxed; see `TestSubprocessStateWriteIsOutsideFileToolBoundary` and the
  limitation below. SOP's own lifecycle writes are unaffected: the CLI writes
  `.agent-sdlc` directly (`state.db`, run artifacts) rather than through the model
  tool harness, and the external completion and lifecycle in this checkout wrote
  them successfully.

### D2 — A failed run could expose a previous VERIFIED verdict

- **Reproduction:** `executeLifecycle` called `writeOutcomeArtifact` with a nil
  verdict on early stops, and the function is a no-op for nil. `runpkg.New`
  clears only `attempts/`, not `outcome.json`. `sop verify` therefore reprinted a
  prior run's `VERIFIED` after a later run produced no verdict.
- **Fix:** `clearOutcomeArtifact` runs at the start of every lifecycle attempt,
  so the artifact always describes the latest attempt. A supported external
  completion (`sop task complete <id> --external`) also clears the prior verdict:
  it records commit and validation provenance but derives no acceptance verdict,
  so a stale `VERIFIED`/`PARTIAL` must not remain visible. `sop verify` now says
  "no recorded outcome for the latest run" instead of implying the task never ran.
- **Regression:** `internal/cli/outcome_stale_test.go`, including
  `TestExternalCompletionClearsStaleOutcome`. The same clear runs on the
  planner-resume path: `plannerResume` opens the existing run with
  `runpkg.Resume` and calls `executeLifecycle`, so a resumed attempt cannot
  inherit a prior verdict. `sop resume` itself is a read-only next-action
  command.

### D3 — SOP runtime state could be cited as repository evidence

- **Reproduction:** `verifySatisfiedDetailed` accepted any file the harness
  observed `read_file`, including `.agent-sdlc/runs/<other>/outcome.json`. That is
  another task's/attempt's evidence, not current repository proof.
- **Fix:** `Harness.IsStatePath` exposes the read-side boundary; the verifier
  rejects `.agent-sdlc` evidence paths with `STATE_EVIDENCE`. The check is
  identity-aware, so a case alias or symlink cannot substitute a repository path
  for state.
- **Regression:** `TestVerifySatisfiedDetailedRejectionCodes/state_evidence_path`
  and `TestAlreadySatisfiedRejectsStateEvidenceOnRealPath`.

### D4 — Manual acceptance evidence content binding (RESOLVED: option 1)

- **Reproduction:** `AcceptanceDecision` bound the task, criterion text,
  manual-evidence text, and attempt, but not the repository content. A manual
  criterion accepted at attempt N could still be satisfied after the content
  changed within the same attempt (a re-run does not increment `task.Attempt`
  until a failure/requeue is recorded).
- **Decision and implementation:** the operator selected **option 1, strict content
  binding**. `CriterionMapping.ContentIdentity` is the content-complete
  working-tree fingerprint (tracked and untracked files, excluding `.git` and
  SOP's own `.agent-sdlc` state). `AcceptanceDecision.ContentIdentity` is copied
  from the mapping the operator reviewed. A manual decision is honored only when
  both identities are non-empty and equal. An unavailable identity fails the
  verdict closed, and recording a decision against an identity-less outcome is
  refused with an actionable error.
- **Migration rule:** legacy `acceptance_decisions.jsonl` records without a
  content identity cannot satisfy a content-bound mapping and require
  re-acceptance after re-running the task. Historical `outcome.json` artifacts are
  not rewritten; a new derivation reports the legacy decision as unmet.
- **Verified by:** `TestManualDecisionContentBinding`,
  `TestOutcomeAcceptRecordsDecision` (decision carries the identity),
  `TestLegacyDecisionWithoutContentIdentityRequiresReacceptance`, and
  `TestAcceptanceRecoveryFromAlreadyRecordedDecision` (a content-bound decision
  still survives the supported attempt-recovery path).
- **Review follow-up (P1):** the identity is computed **after** every acceptance
  check executes, so a check that rewrites repository files cannot leave a
  decision bound to the pre-check tree. A fingerprint failure appends its
  diagnostic without downgrading a HOLD, so an authority boundary remains
  non-waivable. Verified by
  `TestContentIdentityIsComputedAfterAcceptanceChecks` and
  `TestContentIdentityFailurePreservesHold`.

## Deterministic fixture coverage

| Required fixture                                 | Test                                                                                                                                                                                               | Result                               |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ |
| Model claims PASS when a required check failed   | `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`                                                                                                                                            | PASS                                 |
| Model claims PASS when a check never ran         | `TestVerifySatisfiedDetailedRejectionCodes` (`validation_not_executed`); `TestOutcomeIntegrationDeliverableAloneIsPartial`                                                                         | PASS                                 |
| Citation references nonexistent evidence         | `TestVerifySatisfiedDetailedRejectionCodes` (`unread_evidence_path`)                                                                                                                               | PASS                                 |
| Evidence belongs to another task/attempt         | `TestManualEvidenceRequiresDecision` (`stale-attempt`, `wrong-attempt`); `TestStateEvidencePath`                                                                                                   | PASS                                 |
| Evidence refers to changed/invalidated content   | `TestFailedRunDoesNotExposePreviousVerifiedOutcome`; `TestExternalCompletionClearsStaleOutcome`; `TestAcceptanceRecoveryInvalidatesChangedCriterionOrEvidence`; `TestManualDecisionContentBinding` | PASS                                 |
| Model text claims approval without a decision    | `TestModelApprovalClaimWithoutDecisionDoesNotAdvance`                                                                                                                                              | PASS                                 |
| ALREADY_SATISFIED lacks required evidence        | `TestAlreadySatisfiedRequiresObjectiveProof`                                                                                                                                                       | PASS                                 |
| Historical content replaces current verification | `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`                                                                                                                                            | PASS                                 |
| Valid observed evidence still advances           | `TestOutcomeIntegrationVerifiedWithExecutedCheck`; `TestVerifiedAlreadySatisfied`                                                                                                                  | PASS                                 |
| Supported manual acceptance still advances       | `TestOutcomeAcceptRecordsDecision`; `TestManualEvidenceRequiresDecision` (accepting case)                                                                                                          | PASS                                 |
| Case alias/symlink into the state directory      | `TestStateDirectoryAliasesAreRecognized`                                                                                                                                                           | PASS                                 |
| External completion supersedes a stale verdict   | `TestExternalCompletionClearsStaleOutcome`                                                                                                                                                         | PASS                                 |
| Subprocess write outside the file-tool boundary  | `TestSubprocessStateWriteIsOutsideFileToolBoundary`                                                                                                                                                | PASS (gap characterized, not closed) |
| Legacy manual decision without content identity  | `TestLegacyDecisionWithoutContentIdentityRequiresReacceptance`                                                                                                                                     | PASS (requires re-acceptance)        |

## Criterion-to-evidence mapping

### GROUND-001

| Criterion                                                                                          | Evidence                                                                                                                                             | State                                                                      |
| -------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| Distinguish model assertions, observed tool results, accepted human evidence, verified task status | Trace table above; `internal/outcome/criteria.go`; `internal/ollamaagent/already_satisfied.go`                                                       | `VERIFIED` (deterministic)                                                 |
| Identify content/commit, task, attempt, and policy binding                                         | Trace table; `internal/verifcache` identity; `outcome.CriterionMapping.Attempt`; `AcceptanceDecision.Attempt`; manual-decision content identity (D4) | `VERIFIED` for identification; manual decisions are now content-bound (D4) |
| Deliver this report with concrete traces                                                           | This file                                                                                                                                            | `DELIVERED` (committed; file present)                                      |

### GROUND-002

| Criterion                                                                  | Evidence                                                                                                                                                                                                                                                                                                                           | State      |
| -------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- |
| Fabricated PASS text cannot override failed/missing checks                 | `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`; `TestOutcomeIntegration*`                                                                                                                                                                                                                                                 | `VERIFIED` |
| Wrong identity/invalidated content cannot satisfy the task                 | Wrong identity/attempt: `TestManualEvidenceRequiresDecision`, `TestStateEvidencePath`; checks/outcomes: `TestFailedRunDoesNotExposePreviousVerifiedOutcome`, `TestExternalCompletionClearsStaleOutcome`; manual content change: `TestManualDecisionContentBinding`, `TestLegacyDecisionWithoutContentIdentityRequiresReacceptance` | `VERIFIED` |
| ALREADY_SATISFIED follows the existing contract                            | `TestAlreadySatisfiedRequiresObjectiveProof`; `TestVerifySatisfiedDetailedRejectionCodes`                                                                                                                                                                                                                                          | `VERIFIED` |
| Untrusted retrieved material cannot grant approval or replace verification | `TestModelPassClaimCannotOverrideFailedAcceptanceCheck`; `TestModelApprovalClaimWithoutDecisionDoesNotAdvance`                                                                                                                                                                                                                     | `VERIFIED` |
| Valid observed evidence and manual acceptance still advance                | `TestOutcomeIntegrationVerifiedWithExecutedCheck`; `TestOutcomeAcceptRecordsDecision`                                                                                                                                                                                                                                              | `VERIFIED` |
| Regression tests use the real outcome/transition path                      | `runInjectedCLI` → `runSingleTask` → `runStages` → `deriveGatedOutcome`; `CompleteWithEvidence` → `verifySatisfiedDetailed`                                                                                                                                                                                                        | `VERIFIED` |

GROUND-002's SOP task is `LOCAL_DONE`: the strict content binding resolves
criterion 2 for manual acceptance evidence. GROUND-003's criteria also have
current evidence and its SOP task is `LOCAL_DONE`.

### GROUND-003

| Criterion                                                                  | Evidence                                                                                     | State                                             |
| -------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| User-visible result and persisted lifecycle agree                          | `TestFailedRunDoesNotExposePreviousVerifiedOutcome`; `sop verify` reads the cleared artifact | `VERIFIED`                                        |
| Missing checks are `NOT_RUN`, not implied PASS                             | `writeNoValidationResults`; `outcome.Verify` required `objective-acceptance`                 | `VERIFIED` (inspection + existing tests)          |
| Source references resolve to the recorded evidence identity                | `STATE_EVIDENCE` rejection; `verifcache` identity tests                                      | `VERIFIED` for the paths that declare an identity |
| Required checks pass; PR documents before/after evidence and residual risk | This report; external completion validation at `dcb553c8`                                    | `VERIFIED` (deterministic)                        |

GROUND-003's acceptance criteria are met and its SOP task is `LOCAL_DONE` through
external completion at `dcb553c8`.

## SOP lifecycle reconciliation

| Task       | SOP state    | Evidence                                                                                                                             | Reason                                                    |
| ---------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------- |
| GROUND-001 | `LOCAL_DONE` | `sop task complete GROUND-001 --external --commit e93751ef`; validation PASS; `.agent-sdlc/runs/GROUND-001/external-completion.json` | All three criteria have current deterministic evidence    |
| GROUND-002 | `LOCAL_DONE` | `sop task complete GROUND-002 --external --commit dcb553c8`; validation PASS; `.agent-sdlc/runs/GROUND-002/external-completion.json` | Strict content binding (D4 option 1) resolves criterion 2 |
| GROUND-003 | `LOCAL_DONE` | `sop task complete GROUND-003 --external --commit dcb553c8`; validation PASS; `.agent-sdlc/runs/GROUND-003/external-completion.json` | Criteria met; dependency satisfied                        |

Plan state: `COMPLETE`; archived at
`.agent-sdlc/archive/plan-07-sop-llm07-misinformation`. `sop plan complete`
reported three preserved task records. No task status was hand-edited, no approval
was fabricated, and no `state.db` row was edited directly.

## Commands and results

Run from the worktree root on macOS 26.6.2/arm64, Go 1.27.1.

| Command                                                                                            | Result                                                 |
| -------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `go test ./internal/toolharness/... -run TestStateDirectoryIsWriteProtectedForModelTools -count=1` | PASS                                                   |
| `go test ./internal/cli/... -run TestModelPassClaimCannotOverrideFailedAcceptanceCheck -count=1`   | PASS                                                   |
| `go test ./internal/cli/... -run TestModelApprovalClaimWithoutDecisionDoesNotAdvance -count=1`     | PASS                                                   |
| `go test ./internal/cli/... -run TestFailedRunDoesNotExposePreviousVerifiedOutcome -count=1`       | PASS                                                   |
| `go test ./internal/outcome/... -count=1`                                                          | PASS                                                   |
| `go test ./internal/toolharness/ -run TestStateDirectoryAliasesAreRecognized -count=1`             | PASS                                                   |
| `go test ./internal/toolharness/ -run TestSubprocessStateWriteIsOutsideFileToolBoundary -count=1`  | PASS (gap characterized)                               |
| `go test ./internal/cli/ -run TestExternalCompletionClearsStaleOutcome -count=1`                   | PASS                                                   |
| `go test ./internal/outcome/ -run TestManualDecisionContentBinding -count=1`                       | PASS                                                   |
| `go test ./internal/cli/ -run TestOutcomeAccept -count=1`                                          | PASS                                                   |
| `go test ./internal/cli/ -run TestAcceptanceRecovery -count=1`                                     | PASS                                                   |
| `go test ./internal/cli/ -run TestLegacyDecision -count=1`                                         | PASS                                                   |
| `go test ./internal/cli/ -run TestContentIdentity -count=1`                                        | PASS                                                   |
| `go test ./internal/toolharness/... -count=1`                                                      | PASS                                                   |
| `go test ./internal/ollamaagent/... -count=1`                                                      | PASS (66.5 s)                                          |
| `go test ./internal/cli/... -count=1`                                                              | PASS (130.3 s)                                         |
| `go build ./...`                                                                                   | PASS                                                   |
| `go vet ./...`                                                                                     | PASS                                                   |
| `go test ./...`                                                                                    | PASS (116 packages `ok`, no `FAIL`; run at `a91d7493`) |
| `sop task complete GROUND-001 --external --commit e93751ef`                                        | PASS; validation PASS                                  |
| `sop task complete GROUND-002 --external --commit dcb553c8`                                        | PASS; validation PASS                                  |
| `sop task complete GROUND-003 --external --commit dcb553c8`                                        | PASS; validation PASS                                  |
| `sop plan complete`                                                                                | COMPLETE; 3 task records preserved; plan archived      |

The external completions at `dcb553c8` ran the configured `go build ./...`,
`go test ./...`, and `go vet ./...` and reported validation PASS for GROUND-002
and GROUND-003. The earlier full-suite run at `a91d7493` also passed. The plan's
acceptance checks are the focused commands in the table above; the full-suite runs
are the repository's required validation, not a per-criterion substitute.

## Revisions

- Base: `9cf554e6b67a6c61588326adb4a76fd46bfe850d` (`origin/main`).
- Change-set commits on `security/llm07-misinformation`: `257b9b40` (core
  evidence/lifecycle fixes), `a91d7493` (report revisions), `e93751ef`
  (alias/symlink boundary and external-completion stale verdict).
- Follow-up commit on `security/llm07-content-binding`: `dcb553c8` (strict
  content binding for manual acceptance decisions).
- SOP external completion: GROUND-001 at `e93751ef`; GROUND-002 and GROUND-003 at
  `dcb553c8`; all validation PASS. Plan `plan-07-sop-llm07-misinformation` is
  `COMPLETE` and archived.
- Branch: `security/llm07-content-binding`; the follow-up PR records the final
  head. The original PR #285 was merged as `9f5b8150`.
- The change set is the branch diff against the base; the verification commands
  above were run on that change set on the platform named at the top.

## Residual limitations and gaps

- **Factual correctness is not claimed.** A model can read a real file and draw a
  wrong conclusion, and a valid source can be inaccurate. Acceptance checks prove
  their commands exit 0, not that the prose around them is true.
- **Review is model-based.** The review gate blocks on findings, but a review
  `PASS` is not independent verification and cannot substitute for acceptance.
- **Strict content binding is implemented (D4 option 1).** The binding is a
  content-complete working-tree fingerprint, not a signed attestation or a
  provider-verified commit. A legacy decision without an identity requires
  re-acceptance; historical outcome artifacts are not rewritten.
- **File-tool boundary, not a subprocess sandbox.** A SAFE `run_command`
  (for example `go test ./...`) can still write `.agent-sdlc` as a side effect;
  the harness does not sandbox subprocesses. `TestSubprocessStateWriteIsOutsideFileToolBoundary`
  records this boundary. This is a Plan 01 execution-isolation concern, not a
  second permission system in this plan. The report does **not** claim
  comprehensive protection against model-authored state. A before/after
  detection check was investigated and rejected (NO-GO, 2026-11-09): see
  [llm07-state-write-detection-phase1.md](llm07-state-write-detection-phase1.md).
- **SOP lifecycle is COMPLETE.** GROUND-001/002/003 are `LOCAL_DONE` through
  external completion; the plan is archived. No merge, install, deployment, or
  worktree removal is claimed for the follow-up PR.
- **Live-model/adversarial evidence is `NOT_RUN`.** All results are deterministic,
  synthetic-fixture results. No live model was asked to fabricate evidence.
- **Linux is `NOT_RUN`.** Only macOS arm64 was exercised.
- **Historical outcomes are preserved.** The fixes do not rewrite
  `state.db`, decision audits, waivers, or prior run artifacts.

## Handoff

The authority chain rejects the reproduced unsupported-claim paths for
model/observed evidence, the file-tool state boundary handles case aliases and
symlinks, and manual acceptance evidence is now strictly bound to the reviewed
repository content. All Plan 07 SOP tasks are `LOCAL_DONE` and the plan is
`COMPLETE`; the follow-up code is on `security/llm07-content-binding` for review.
The follow-up PR is **not merged**.
