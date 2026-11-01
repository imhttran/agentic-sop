# SPIKE-005 — restart, timeout, stale-state, duplicate, and authority verification

Baseline commit: `64133c1302d6c6ae41077a1b7960511b8aaeff7f` (SPIKE-004).
Scope: verify the criteria in the active plan
`docs/plans/SOP-Planner-Connection-Spike.md` (§SPIKE-005 and the acceptance
checklist) using a scripted executor and injected deadlines/clocks. Disposable
fixtures only; the completed live demo under `.run/planner-live` is preserved.

This report distinguishes **tested behavior**, **modeled crash behavior**,
**live-client evidence**, and **remaining limitations**, and tracks the observed
MCP initialization error separately.

## 1. Acceptance criteria → evidence

### Criterion A — no duplicate application, implicit approval, premature completion, or budget reset

| Claim                    | Covering tests                                                                                                                                                                                                                                                                  | Kind   |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| No duplicate application | `TestApplyIdempotentNoDoubleDispatch`, `TestContinuationCreatedAtomicallyWithApplication`, `TestAtomicSubmissionConcurrentExactlyOneAccepted`, `TestAtomicSubmissionAcrossProcessesExactlyOneAccepted`, `TestSubmitAnswerDuplicateAndConflict`, `TestCLIAnswerConflictRejected` | tested |
| No implicit approval     | **`TestPlannerAnswerCannotSatisfyHumanGate`** (new), `TestCLIQuestionAndAnswerAccepted` (no approval gate created)                                                                                                                                                              | tested |
| No premature completion  | `TestPlannerIntegrationStageLabelAloneCannotPass`, `TestCLIPlannerReconcileDoesNotConsumeOwedWork`                                                                                                                                                                              | tested |
| No budget reset          | **`TestPlannerClarificationPreservesFailureBudget`** (new)                                                                                                                                                                                                                      | tested |

### Criterion B — timeout, disconnect, exhausted rounds preserve an explicit waiting/paused condition and evidence

| Claim                                          | Covering tests                                                                                                                                        | Kind   |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| Timeout pauses                                 | **`TestPlannerTimeoutParksWithoutRetry`** (new), `TestDecidePauseOnTimeout`; **production wiring now covered by `planner_timeout_test.go` — see §10** | tested |
| Disconnect / restart while waiting re-parks    | **`TestPlannerRestartWhileWaitingReparks`** (new), `TestContinuationPersistsAcrossReopen`                                                             | tested |
| Exhausted rounds pause at an explicit boundary | **`TestPlannerExhaustedRoundsParkAtHumanBoundary`** (new), `TestDecideGuards` ("rounds exhausted"), `TestCLIPlannerRoundLimit`                        | tested |

> **Correction (see §10).** As first written, this section implied the timeout was
> covered end to end. It was not: the mechanism was unit-tested, but the production path
> never populated `Question.Deadline` from `planner.timeout_seconds`, and
> `TestPlannerTimeoutParksWithoutRetry` seeded the deadline directly, so the config→deadline
> wiring was untested. §10 records the fix and the tests that now exercise the production
> creation paths.

### Criterion C — clarification leaves failure counters unchanged; disabled mode preserves prior behavior

| Claim                                  | Covering tests                                                                          | Kind   |
| -------------------------------------- | --------------------------------------------------------------------------------------- | ------ |
| Failure counters unchanged             | **`TestPlannerClarificationPreservesFailureBudget`** (new)                              | tested |
| Disabled mode preserves prior behavior | **`TestPlannerDisabledIgnoresExistingQuestion`** (new), `TestCLIPlannerDisabledRefuses` | tested |

### Other acceptance-checklist items in SPIKE-005's scope

| Item                                                                               | Covering tests                                                                                                                                                                                                                                       | Kind   |
| ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| Restart while waiting retains the question, blocks ordinary retry                  | `TestPlannerRestartWhileWaitingReparks` (new), `TestContinuationPersistsAcrossReopen`                                                                                                                                                                | tested |
| Restart after application resumes at most once; duplicate delivery no extra effect | `TestContinuationCreatedAtomicallyWithApplication`, `TestPlannerResumeCountsOneCompletion`, `TestApplyIdempotentNoDoubleDispatch`                                                                                                                    | tested |
| Changed repo/plan revision rejects a stale answer without executing                | `TestCLIPlannerStaleBindingSkipped`, `TestCLIAnswerWrongProjectStale`, `TestCLIAnswerSupersededRunStale`, `TestCLIAnswerMissingRunStale`, `TestSubmitAnswerStaleBindingPreservesRecord`, `TestSubmitAnswerExpectedRevisionStale`, `TestDecideGuards` | tested |
| A human-only authorization cannot be satisfied by the planner                      | `TestPlannerAnswerCannotSatisfyHumanGate` (new), `TestPlannerIntegrationHumanGateBlocks`, `TestCLIPlannerRespectsHumanGate`, `TestPlannerResumeStandsDownWithoutOwnership`                                                                           | tested |

## 2. Gaps found and closed

Before this work, SPIKE-005's criteria were only partially covered: the stale,
duplicate, and concurrency logic had strong unit coverage, but the runtime
_conditions_ (timeout, restart/disconnect, exhausted rounds, authority, budget
invariance, disabled mode) had no CLI/integration test, and one defect surfaced
once they were written.

| #   | Gap                                                                                                                                                          | Resolution                                                                                     |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------- |
| 1   | No integration test that an expired deadline parks the run (only a `Decide` unit test)                                                                       | `TestPlannerTimeoutParksWithoutRetry`                                                          |
| 2   | No test that re-entering while a question is PENDING re-parks (no dispatch, epoch preserved)                                                                 | `TestPlannerRestartWhileWaitingReparks`                                                        |
| 3   | **Defect**: `ActionRoundsExhausted` fell through to the normal lifecycle (`handled=false`), so an exhausted planner loop retried the task instead of pausing | Fixed in `internal/cli/planner.go`; covered by `TestPlannerExhaustedRoundsParkAtHumanBoundary` |
| 4   | No test that an answer cannot satisfy a human gate regardless of its text                                                                                    | `TestPlannerAnswerCannotSatisfyHumanGate`                                                      |
| 5   | No test that clarification leaves the failure counters, attempt evidence, and continuation budget intact                                                     | `TestPlannerClarificationPreservesFailureBudget`                                               |
| 6   | No test that disabled mode ignores a leftover question and preserves normal execution                                                                        | `TestPlannerDisabledIgnoresExistingQuestion`                                                   |

### Defect 3 (fixed)

`plannerLifecycleGate` now handles `question.ActionRoundsExhausted` explicitly: it
parks the run at the existing `WAITING_FOR_HUMAN` boundary and records an explicit
approval request (evidence), via the generalized `parkPlannerHuman` helper (the
previous `parkPlannerForReview` is renamed/reused). No approval is implied by the
park. Proven fail-before / pass-after: with the case removed, the regression test
observed `handled=false code=0` (fall-through); with the fix it parks.

## 3. Tested behavior

Deterministic tests with a scripted executor (`scriptedPlannerAgent`) and injected
deadlines/`Now`. New SPIKE-005 tests (`internal/cli/planner_spike005_test.go`):

- `TestPlannerTimeoutParksWithoutRetry` — expired deadline + `pause_on_timeout`:
  parks at `WAITING_FOR_PLANNER`, `implCalls==0`, the question stays `PENDING`.
- `TestPlannerRestartWhileWaitingReparks` — two re-entries: parks each time,
  `implCalls==0`, the run epoch is unchanged.
- `TestPlannerExhaustedRoundsParkAtHumanBoundary` — rounds exceed the limit: parks
  at `WAITING_FOR_HUMAN` with a pending approval request, `implCalls==0`.
- `TestPlannerAnswerCannotSatisfyHumanGate` — an "APPROVED" answer is recorded as
  `ANSWERED` only; the approval stays `PENDING` and listed; the planner does not
  act across the gate.
- `TestPlannerClarificationPreservesFailureBudget` — a park leaves
  `Attempt`/`MaxAttempts` unchanged, preserves `attempts/` evidence, and consumes
  no continuation budget.
- `TestPlannerDisabledIgnoresExistingQuestion` — with the planner off, a leftover
  question is untouched and the task runs to `LOCAL_DONE` normally.

## 4. Modeled crash behavior

Crash/uncertainty is **modeled**, not produced by a real process kill:

- Crash points are simulated by writing durable start/success markers and by
  goroutine/store-level concurrency: `TestContinuationRecoveryCrashCases`,
  `TestPlannerRecoveryGapEffectMarker`, `TestFenceAndMutateRejectsReclaimedWorker`,
  `TestExecutionLockBlocksReclaimedWorkerWhileRunning`,
  `TestPlannerRecoveryMalformedMarkersStopForReview`.
- Policy: on recovery the success marker completes without replay; a start marker
  without success — or a malformed/unreadable marker — is treated as **uncertain**
  and stops for human review (no replay, no completion). No test interleaves a real
  `SIGKILL` with an in-flight fsync.

## 5. Live-client evidence

- **Planner side = real client.** ChatGPT reached SOP over the Secure MCP Tunnel:
  SPIKE-002 proved `sop_status`; SPIKE-004 proved the extended loop — ChatGPT listed
  the planner tools, read the executor-raised question, and submitted the answer
  `a-2` = `Markdown` (`answered_by: chatgpt`, key `chatgpt-planner-live-q1-001`) to
  the disposable `.run/planner-live` project. Evidence:
  `docs/plans/SPIKE-004-live-evidence.md` and the durable artifacts under
  `.run/planner-live/.agent-sdlc/` (question/continuation, `runs/T001/`, the
  executor log).
- **Runner side = scripted.** The executor in that demo is a controlled script
  (`.run/planner-live-executor.sh`), not a general model.
- No automated Git publication was performed.

## 6. Observed MCP initialization error (tracked separately; cause not assigned)

An MCP initialization error was observed during a **live ChatGPT session**. The
captured detail:

- **HTTP 409**.
- `error_type`: **`mcp_initialization_required`**.
- **origin: `tunnel-client`** (the error is attributed to the tunnel client, not to
  SOP's MCP server).
- A **parallel** `question-get` call **failed**; the concurrent `status` call
  **succeeded**.
- A **sequential** `question-get` **retry succeeded**.
- **No answer was resubmitted** — the failure occurred on reads (question-get), not
  on the `answer` submission, so nothing was re-applied.

What was checked this session (no error reproduced here):

- The captured MCP request/response fixtures (`.run/mcp-*.jsonl`) show clean
  `initialize` → `notifications/initialized` → `tools/list` / `tools/call`
  sequences with no error frames.
- The tunnel runtime health is OK (`Healthz PASS`, `Readyz PASS`, control-plane
  poll `PASS`).
- The server implements `initialize` (returns `protocolVersion` `2024-11-05`) and
  does not enforce a "must initialize first" gate; its only "initialization"
  message is the tool **result** `project is not initialized; run `sop init``, a
  state message, not a protocol error.

Status: **open, root cause unresolved.** The evidence localizes the error to the
`tunnel-client` layer on a parallel read (`409` / `mcp_initialization_required`)
that a sequential retry cleared; it does not establish why a concurrent
question-get tripped an initialization-required condition while a concurrent
status call did not. This is tracked separately from the SPIKE-005 criteria. Next
diagnostic step (not taken here): capture the tunnel-client-side session lifecycle
around a parallel `question-get` vs `status` and correlate with the tunnel-client's
initialization state. No fix is proposed without that evidence.

## 7. Remaining limitations

- The executor used throughout is **scripted**; a real model executor is unverified.
- Crash/uncertainty is **modeled** (no real `SIGKILL`); markers are plain durable
  writes (temp + fsync + rename + dir fsync), not an fsync-interleaved crash test.
- Guarantees are **not** general exactly-once: an uncertain continuation stops for
  human review rather than replaying.
- Cross-process execution-lock concurrency is guaranteed on unix via `flock`;
  Windows fails closed.
- The integration timeout test uses a **past deadline**; the explicit injected-`Now`
  clock is exercised at the `Decide` unit level.
- No automatic Git publication; SPIKE-006 (live demonstration report) is not run.

## 8. How to reproduce

```sh
gofmt -l . | grep -v '^./.run/'      # clean
go build ./...
go vet ./...
go test ./...                         # all pass
go test ./internal/cli/ -run 'TestPlannerTimeoutParksWithoutRetry|TestPlannerRestartWhileWaitingReparks|TestPlannerExhaustedRoundsParkAtHumanBoundary|TestPlannerAnswerCannotSatisfyHumanGate|TestPlannerClarificationPreservesFailureBudget|TestPlannerDisabledIgnoresExistingQuestion' -v
```

## 9. Files changed by this step (relative to `64133c1`)

- `internal/cli/planner.go` — exhausted-rounds park at an explicit human boundary;
  `parkPlannerForReview` generalized to `parkPlannerHuman`.
- `internal/cli/planner_spike005_test.go` — the six new tests.
- `docs/plans/SPIKE-005-verification-report.md` — this report.

Uncommitted on branch `spike-004-planner-continuation`. The completed live demo
(`.run/planner-live`) and its original `PARTIAL` report are preserved unchanged.

## 10. PR #7 review follow-up — finding dispositions

Dispositions for the findings raised on draft PR #7 (head `05a1219`). This section
**corrects the timeout claims above** and records the fixes; it does not rewrite any live
evidence.

### D1 (HIGH) — the response timeout was never wired — FIXED

- **Finding:** `planner.timeout_seconds` was parsed but read nowhere in production, and no
  production path populated `Question.Deadline`, so `pause_on_timeout` could never fire.
  `TestPlannerTimeoutParksWithoutRetry` and `TestDecidePauseOnTimeout` seeded the deadline
  directly, so the config→deadline wiring was untested (the live demo's questions show
  `deadline: 0001-01-01T00:00:00Z` despite `timeout_seconds: 600`).
- **Fix:** a single derivation in the store's `CreateQuestion`
  (`internal/question/store.go`): when the submitted record carries `DeadlineAfter > 0`
  and no explicit deadline, the deadline is `created_at + DeadlineAfter`, using the record's
  **own** creation stamp and the store's injectable clock. Both creation paths set it via
  `plannerTimeout(cfg)` (`internal/cli/planner.go`, `internal/cli/planner_raise.go`).
- **Policies (now explicit):** `0` ⇒ no deadline (the default); negative or
  duration-overflowing values ⇒ rejected at creation (fail closed), so no question is
  recorded. A duplicate creation or a restart returns/loads the existing record, so the
  deadline is **preserved**; re-entry never extends it.
- **When expiry is evaluated:** only when SOP reads the planner state — command re-entry
  (the planner gate on `sop run`, or `sop continue`). There is **no background timer**;
  an expired question stays parked until that read, and is never auto-retried.
- **Tests (production creation paths; no direct deadline seeding):**
  `TestPlannerDeadlineDerivedOnExecutorRaise`, `TestPlannerDeadlineDerivedOnCLIAsk`,
  `TestPlannerZeroTimeoutMeansNoDeadline`, `TestPlannerInvalidTimeoutRejected`,
  `TestPlannerDeadlineNotExtendedOnReentry`, `TestPlannerTimeoutExpiryPausesWithEvidence`
  (`internal/cli/planner_timeout_test.go`). Fail-before-fix proven: with the derivation
  disabled, the first three fail with "has no deadline" / "no deadline to expire".

### D2 (MEDIUM) — recovery supersede — OWNERSHIP-PROTECTED

- The durable-successor invariant is documented at the store method
  (`ResolveYieldedContinuation`, `internal/question/store.go`).
- **The interleaving IS possible, and is now protected against.** `sop planner ask`
  (`internal/cli/planner.go`) persists a successor question for a task **without** holding
  the predecessor's execution ownership, while `plannerResume` holds the per-continuation
  execution lock across the whole resumed stage (acquired before the claim, released on
  return). A successor can therefore be persisted while the predecessor's own worker is
  still running its stage, so the successor check alone was **not** sufficient.
- **Fix:** `plannerResolvePredecessors` now acquires the predecessor's execution lock
  **first** — the same lock `plannerResume` holds, and the same ordering (execution lock,
  then the question store), so lock ordering is preserved — and revalidates the successor
  and binding guards **after** ownership is held. A worker that cannot acquire ownership
  leaves the continuation `CLAIMED` for its owner to finish.
- **Regression test:** `TestPlannerRecoveryDefersToHeldExecutionOwnership` — while the owner
  holds the lock, recovery leaves the predecessor `CLAIMED`; once the lock is released,
  recovery resolves it. Fail-before-fix proven (bypassing the acquisition supersedes
  in-flight work). The earlier `TestPlannerRecoveryLeavesActivePredecessorWhenNoSuccessor`
  covers only the no-successor case and is **not** used as proof of this invariant.

### D3 (LOW) — duplicate stale comment — FIXED

- Removed the repeated `activeQuestion` doc comment in `internal/cli/planner.go`.

### D4 (LOW) — handoff capsule missing on planner success — FIXED

- `plannerResume` now persists the RBV-011 handoff capsule under `context.handoff` on both
  success paths (normal `PASSED` and success-marker recovery), via `plannerSaveHandoff`,
  mirroring the normal path's best-effort, non-failing semantics.
- **Coverage:** `TestPlannerResumeSavesHandoffCapsule` is table-driven over all four cases —
  normal success and success-marker recovery, each with `context.handoff` **enabled and
  disabled** — asserting the capsule is present exactly when the flag is on.

### Checks after the fixes

`gofmt` clean; `go vet ./...`; `go build ./...`; `go test ./...`; `check-doc-links.sh`
(broken links: 0); `go test -race ./...`; a `GOOS=windows` cross-build — all pass.
