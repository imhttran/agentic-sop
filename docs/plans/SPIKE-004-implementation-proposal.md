# SPIKE-004 implementation proposal — executor questions, WAITING_FOR_PLANNER, durable continuation

Date: 2026-10-15 (revised) • Status: **proposal only — not implemented.**
Depends on SPIKE-003 (question/decision contract + CLI/planner MCP surface), currently
**paused at its human gate**; it must be closed by the operator first (see
`SPIKE-003-closure-checklist.md`). The active plan source is unchanged.

Covers the second half of the spike: the **executor-created question**, a real
**`WAITING_FOR_PLANNER`** run stage, **validated answer application**, and **durable
continuation from the saved stage**.

## 1. Scope

**In scope:** a task raises a structured question and parks without retrying; a
validated planner answer is applied and the executor continues from the saved stage;
restart, staleness, duplicate delivery, interrupted workers, bounded rounds, and
human-gate preservation are handled deterministically and durably.

**Out of scope:** the live ChatGPT round trip (SPIKE-006); any new planner tool;
external publication of any kind (no commit/push/merge in the continuation path);
multi-PR partitioning; unattended runners.

**A new run stage.** `internal/run/run.go` today defines CREATED / PLANNING /
IMPLEMENTING / VALIDATING / REVIEWING / FIXING / WAITING_FOR_HUMAN / PASSED / FAILED.
This adds **`WAITING_FOR_PLANNER`** — a distinct, non-terminal pause. It is **not**
`WAITING_FOR_HUMAN` and must never be mapped to one.

## 2. Executor-created questions

When the executor reaches a genuine choice the requirements do not determine (and
which is **not** a human-only authorization), it calls the question service:

- `CreateQuestion` with the bindings the contract already carries (`Project`,
  task `Identity`, `RunID`, `RunEpoch`, `PlanRevision`, `ReturnStage`, `Options`,
  `EvidenceRefs`, `Deadline`), state `PENDING`, revision 1.
- `ReturnStage` records the stage to resume.
- The run is parked at **`WAITING_FOR_PLANNER`** (`SetStage`) and the lifecycle returns
  a **paused, non-failure** result. Raising a question consumes a **planner round**
  (§6), never a failure retry.

A human-only authorization request (security, destructive/irreversible, approval
boundary) is **never** turned into a planner question — it stays `WAITING_FOR_HUMAN`.

## 3. Application + durable continuation (one transaction in the question store)

Application and the decision to continue are persisted **together** in the question
store, so a crash can never observe "applied" without "a continuation is owed".

`Service.ApplyAnswer` runs as **one transaction inside `question.FileStore`** (the
store's `Atomic` path — a single flock, one reload, one atomic rename):

1. verify the question is `ANSWERED` and the binding still matches live state
   (task / run epoch / repository / plan revision);
2. transition `ANSWERED -> APPLIED`;
3. create a **continuation record** in state `PENDING` with a **stable continuation ID**
   (§4);
4. append the application-result to the audit ledger.

Only after this transaction commits may any dispatch/continuation attempt happen.
Receiving an answer never itself approves a task, and continuation never publishes
anything externally.

## 4. Durable continuation protocol

A **continuation record** lives in the question store (`.agent-sdlc/questions.json`)
alongside the questions:

```
Continuation {
  ID          string   // stable, deterministic: "cont:" + QuestionID + ":" + RunEpoch
  QuestionID  string
  TaskID      string
  RunID       string
  RunEpoch    string
  ReturnStage string
  Status      PENDING | CLAIMED | COMPLETED
  ClaimedBy   string   // worker identity
  ClaimedAt   time     // lease start
  CompletedAt time
  Attempts    int      // bounded re-claims
}
```

- **Stable continuation ID** — derived from `(QuestionID, RunEpoch)`, **not** random, so
  repeated application or recovery always addresses the **same** continuation (no
  duplicate continuations for one applied answer).
- **Claiming** — a worker takes a continuation by an atomic **compare-and-set**
  `PENDING -> CLAIMED` (setting `ClaimedBy`/`ClaimedAt`). At most one worker claims a
  given continuation; a second concurrent claim sees `CLAIMED` and stands down.
- **Interrupted-worker recovery** — a `CLAIMED` continuation whose lease
  (`ClaimedAt`) is older than a bounded **lease** may be reclaimed
  (`CLAIMED -> PENDING`, then claimed again, or claimed directly). An interrupted
  worker therefore never strands the continuation; `Attempts` is bounded so reclaims
  cannot loop forever.
- **Completion recording** — after the resumed stage returns through the normal
  lifecycle path, the worker atomically records `CLAIMED -> COMPLETED` with
  `CompletedAt`. `COMPLETED` is terminal.
- **Repeated delivery is safe** because (a) the ID is stable, (b) **stage handling is
  idempotent** — re-entering `ReturnStage` converges rather than duplicating work, and
  (c) the compare-and-set guards prevent concurrent double-claim.

### 4.1 Honest guarantees

- **Within the question store:** application + PENDING continuation + audit are **one
  atomic transaction**.
- **Dispatch/continuation is at-least-once** _only for a safe (first) attempt_. It is
  **not** exactly-once in general. A first attempt writes a durable start marker before
  running the stage work and a success marker before completion; if the process is
  interrupted with a start marker but no success marker, replay safety **cannot be
  established**, so recovery does **not** re-run the stage and does **not** record
  completion — it stops for human review (§13). The success-marker-present case completes
  without replay.
- **External side effects are not covered.** If a resumed stage performed a
  non-idempotent external effect (network call, external commit/push), a crash and
  recovery could repeat it. Therefore external effects must be **idempotent, keyed on
  the stable continuation ID**, or must live **outside** the continuation path. The
  spike keeps continuation local (repository work only) and performs **no** external
  publication.

### 4.2 Claim-generation fencing for expired workers

Each continuation carries a monotonically increasing **claim generation**. A claim
records both the worker identity and the generation it took, and **every completion or
re-claim compare-and-set must present the current generation**:

- An **expired worker** whose lease was reclaimed (a newer generation exists) can no
  longer record `CLAIMED -> COMPLETED` — its compare-and-set presents a stale
  generation and is **fenced** (rejected). A slow, previously-fenced worker therefore
  cannot overwrite a newer worker's result or resurrect a completed continuation.
- Re-claiming after lease expiry **increments** the generation, so the previous
  worker's token is invalidated atomically with the reclaim.
- This is what makes concurrent writers safe: recovery proceeds, and the
  original worker's late write is discarded rather than racing. It does not, by itself,
  establish that a stage is safe to replay — see §13 for the uncertain-execution policy.

## 5. Storage transaction boundary and partial-write reconciliation

There is **no cross-store atomic transaction**. The stores are separate:

| Store                                              | Path                                            | Write mechanism                                                                                                        | In the application transaction?   |
| -------------------------------------------------- | ----------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- | --------------------------------- |
| Questions / answers / outcomes / **continuations** | `.agent-sdlc/questions.json`                    | `question.FileStore`: flock + temp-file + fsync + **atomic rename** (`internal/durablefs`)                             | **Yes — this is the transaction** |
| Run stage / approval / attempt evidence            | `.agent-sdlc/runs/<task>/state.json`            | `runpkg.Run.save()` = plain `os.WriteFile`; `PersistDurable()` = temp-file + fsync + rename for the planner checkpoint | No                                |
| Planner start/success markers                      | `.agent-sdlc/runs/<task>/planner-effect-*.json` | `internal/durablefs` (temp-file + fsync + atomic rename + dir fsync)                                                   | No (recovery signal)              |
| Task status / attempts                             | `.agent-sdlc/state.db`                          | SQLite (its own transaction)                                                                                           | No                                |
| Plan provenance                                    | `.agent-sdlc/plan.meta.json`                    | plain `os.WriteFile`                                                                                                   | No                                |

**Application is atomic only within `questions.json`.** A crash can therefore leave the
question store with `APPLIED` + a `PENDING` continuation while `runs/state.json` still
says `WAITING_FOR_PLANNER` and the task is still `PLANNED`.

**Reconciliation is one-directional and idempotent:**

- The **question store is authoritative** for "the answer is applied and a continuation
  is owed/claimed".
- On `sop run` startup and on `sop continue`, a **reconciler** reads the question store
  for continuations not `COMPLETED` and, for each, (re)drives the run to `ReturnStage`
  and runs the continuation through the normal path.
- The reconciler **re-derives** the run stage and, if needed, the task status rather
  than trusting a partially-written `runs/state.json`; `state.json` is treated as
  **advisory**. (The planner continuation does not rely on that alone: `runpkg.Run.PersistDurable()`
  now writes the run checkpoint via the shared `internal/durablefs` primitive before
  success is recorded. The generic `runpkg.save()` remains a plain write; hardening it is
  still an optional, separate item.)
- Ordering is fixed: the question-store transaction commits **before** any
  run-stage/state.db write, so an observer or crash between them always sees the
  applied-continuation record — never the reverse.

### 5.1 Reconciliation guards (what must NOT be resumed)

The reconciler is deliberately conservative. It must **not** resume or continue a
continuation when any of the following holds, and must instead leave the state to the
owning authority:

- **A human gate:** the task is at `WAITING_FOR_HUMAN` or has a pending approval — the
  human decision owns the task; reconciliation neither resolves nor bypasses it.
- **Cancellation:** an explicit cancellation / `cancelled` disposition was recorded.
- **Terminal task state:** the task is already terminal (`LOCAL_DONE`/`DONE`/`MERGED`/
  `NOT_REQUIRED`, or a terminal failure) — there is nothing to resume.
- **Changed bindings:** the current **plan revision, run epoch, or repository identity**
  no longer matches the continuation's binding — the continuation is **STALE** and is
  skipped (with evidence), never applied to the new state.

Only a continuation whose task is non-terminal, not human-gated, not cancelled, and
whose bindings still match is reconciled.

## 6. Bounded planner loops

- A **question-round limit** (suggested: three) bounds questions per task.
  **Clarification consumes a planner round, not a failure retry** — `attempt`/
  `max_attempts` and fix cycles are untouched.
- A **response timeout** and **pause-on-timeout**: a question past its `Deadline`
  leaves the task **paused at `WAITING_FOR_PLANNER`** with evidence — no auto-retry, no
  conversion to failure.
- Exhausting the round limit parks the task at an explicit paused/human boundary with
  evidence, never an infinite loop.
- The feature is **opt-in** (`SOP_PLANNER_ENABLED` / `SOP_PLANNER_MODE`); with it off,
  ordinary execution behavior is unchanged.

## 7. Restart, staleness, duplicate delivery

### 7.1 Restart while waiting

On re-entry for a task paused at `WAITING_FOR_PLANNER`:

- question `PENDING` → **re-park** (no retry, no budget spend, no new attempt); ordinary
  retry/recovery must **not** dispatch it;
- question `ANSWERED` (not applied) → apply (§3) then continue;
- question `APPLIED` with a non-`COMPLETED` continuation → run the continuation (§4);
- question `APPLIED` with the continuation `COMPLETED` → the work already resumed; do
  not redo;
- interrupted mid-stage → resume the **saved stage**.

This requires `WAITING_FOR_PLANNER` to be **active/resumable** (never blocked/failed/
human) in the scheduler's active set and in `currentApprovalBoundary` / `humanBoundary`.

### 7.2 Stale answers

Binding validation refuses an answer whose task/run-epoch/repository/plan revision no
longer matches, returning `STALE` and **preserving** the question and prior records. A
stale answer is **never applied**, so changed state cannot execute; the attempt is
audited.

### 7.3 Duplicate delivery / application

- The `ANSWERED -> APPLIED` compare-and-set is the sole arbiter; only the caller that
  commits §3 creates the continuation, and the stable ID makes a repeat target the same
  continuation.
- Duplicate answer with the same idempotency key + same payload → `DUPLICATE` (no
  effect); same key + different payload → `REJECTED` (SPIKE-003 semantics).
- Re-applying an already-`APPLIED` question returns the existing record and does not
  create a second continuation.

## 8. Crash tests (defined before implementation)

Each scenario interrupts the worker at a specific point and asserts: the question record is
consistent, the continuation is not silently consumed, and **no external side effect is
repeated** (the spike performs none). Recovery does **not** assume general exactly-once:
where replay safety cannot be established, it stops for human review rather than replaying
or marking completion (see §13).

1. **Before application persistence** — kill between the answer being recorded
   (`ANSWERED`) and the §3 transaction committing. Expect: question `ANSWERED`, **no**
   continuation record; on restart the normal apply path applies then continues. No
   partial application.
2. **After persistence, before dispatch** — §3 committed (`APPLIED` + continuation
   `PENDING`); kill before any stage work. Expect: the reconciler finds the `PENDING`
   continuation, claims it, and continues — **no loss, no duplicate** (idempotent stage
   handling), and `runs/state.json` is re-derived to `ReturnStage`.
3. **During execution** — continuation `CLAIMED`; kill mid-stage. Expect: the durable
   start marker exists with no success marker, so the outcome of the in-flight effect is
   **uncertain**. Recovery does **not** replay and does **not** complete; the run parks at
   `WAITING_FOR_HUMAN` with an approval request (implemented behavior — see §13).
4. **After execution, before completion recording** — stage work done; kill before the
   `CLAIMED -> COMPLETED` compare-and-set. Expect: the success marker was not persisted, so
   this is the same uncertain case as (3): **no replay, no completion**, park for human
   review. (The success-marker-present sub-case completes without replay, and is covered by
   `TestPlannerRecoveryGapEffectMarker`.)

Additional tests: concurrent claim (one wins); lease-expiry reclaim; a second question
round; duplicate answer delivery; stale answer refused; round-limit exhaustion; the
feature disabled (behavior unchanged).

## 9. Preservation of human approval gates

- `WAITING_FOR_PLANNER` is **never** an approval boundary: `currentApprovalBoundary`
  must return "none" for it, and `humanBoundary` must not fire on it — otherwise a
  planner pause would manufacture an approve/decline gate.
- The planner-scoped MCP surface still never registers `sop_approve` / `sop_decline`,
  and an answer is never an `approval`/`domain.ApprovalDecision`.
- A human-only authorization request cannot be satisfied by a planner answer (§2).
- A planner answer never bypasses validation, review, the quality gate, or
  `human.approval_before_commit`.

## 10. Affected files (proposed)

- `internal/run/run.go` — add `WaitingForPlanner`; optionally harden `save()` to atomic
  write.
- `internal/cli/run.go`, `internal/cli/drive.go` — executor question hook; park at
  `WAITING_FOR_PLANNER`; resume-from-saved-stage; classify the pause as non-failure.
- `internal/cli/approval.go` — exclude `WAITING_FOR_PLANNER` from
  `currentApprovalBoundary`/`humanBoundary`.
- `internal/scheduler/scheduler.go` — active/resumable classification for the new stage.
- `internal/question/{service,store,filestore,question}.go` — atomic apply + continuation
  record, stable ID, claim/lease/complete compare-and-set operations, reconciler hook.
- `internal/cli/continue.go` — next-action/reconcile path for a planner pause.
- `internal/config/config.go` — opt-in enablement, planner mode, round limit, timeout,
  pause-on-timeout (non-secret, persisted effective values).
- Tests: `internal/question/*_test.go`, `internal/cli/*_test.go` (the §8 crash tests
  and the §7/§9 cases), plus a disposable demonstration.
- `docs/reference/CLI.md` — document the new behavior.

## 11. Acceptance criteria

- [ ] A task pauses at `WAITING_FOR_PLANNER` **without retrying**; a validated answer
      resumes through normal checks; a **second** question round works.
- [ ] The application + `PENDING` continuation are **one transaction** in the question
      store; no observer/crash sees `APPLIED` without a pending continuation.
- [ ] Restart while waiting retains the question and **prevents ordinary
      retry/recovery from dispatching it**.
- [ ] Restart after application continues **exactly once in effect** (at-least-once
      execution with idempotent handling); duplicate delivery has no extra effect.
- [ ] A crash **during** execution or **after execution before completion** is recovered
      by reclaim and converges (no loss, no effective duplicate).
- [ ] Stale answers are **rejected without executing**.
- [ ] Timeout/exhausted rounds preserve an explicit **paused** condition with evidence.
- [ ] Existing failure counters are unchanged by clarification; disabled mode preserves
      prior behavior.
- [ ] A **human-only authorization request cannot be satisfied by the planner**.
- [ ] Cross-store partial writes are **reconciled** from the authoritative question
      store; `runs/state.json` is re-derived, not trusted.
- [ ] Guarantees are stated accurately, including the **external-side-effect limit**.

## 12. Dependency on closing SPIKE-003

SPIKE-004 builds on SPIKE-003's contract and planner surface, paused at a pending
`NEEDS_HUMAN` gate. SPIKE-003 must be closed by the operator first
(`SPIKE-003-closure-checklist.md`, separate). This proposal neither resolves that gate
nor runs SOP.

## 13. Implementation & verification status (branch `spike-004-planner-continuation`)

This section records what is **tested** on the branch, what is **modeled**, and the
**remaining limitations**. It is written from the commands actually run, not from the
design above.

### 13.1 Commands run (repository-required checks)

From the repository root:

- `gofmt -l . | grep -v '^./.run/'` — clean (no files listed).
- `go build ./...` — ok.
- `go vet ./...` — ok.
- `go test ./...` — ok (all packages).
- Targeted: `go test ./internal/cli/ -run 'Planner'` and
  `go test ./internal/question/ -run 'Continuation|Claim|Fence|ExecutionLock|StaleWorker'`.

### 13.2 Tested behavior

| Requirement                                                                                                                                                      | Test (package)                                                                                                                                                                                                       | What it actually asserts                                                                                                                                                                               |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| The executor raises **two** planner questions within ONE task/run, via a structured `OutcomeNeedsPlanner` outcome (not a pre-seeded question, not a manual ask). | `TestPlannerIntegrationTwoRoundsOneRun` (`internal/cli`)                                                                                                                                                             | Two questions raised during implementation across three invocations of one task; the task finally passes.                                                                                              |
| Each answer reaches the resumed executor.                                                                                                                        | same                                                                                                                                                                                                                 | Executor inputs contain `Planner decision: Markdown` **and** `Planner decision: yes`.                                                                                                                  |
| `ReturnStage` is honored; completed stages are not repeated.                                                                                                     | `TestPlannerIntegrationStructuredAskParksAndResumes`, `...TwoRoundsOneRun`                                                                                                                                           | `PLAN` runs exactly once (`planCalls==1`) across the pause/resume; the same run epoch is preserved.                                                                                                    |
| A resume preserves attempt accounting (uses `Resume`, not `New`).                                                                                                | `TestPlannerIntegrationStructuredAskParksAndResumes`                                                                                                                                                                 | An `attempts/` sentinel seeded after parking survives the resume (`New` would clear it).                                                                                                               |
| Unsupported return stages are explicitly rejected.                                                                                                               | `TestCLIPlannerRejectsUnsupportedReturnStage`; `supportedReturnStage` gates `raisePlannerQuestion`, `plannerResume`, `runPlannerAsk`, `runPlannerContinue`                                                           | `planner ask --return-stage REVIEWING` and a recorded `REVIEWING` continuation both error with "unsupported return stage"; the continuation is not completed.                                          |
| Execution ownership prevents overlapping workers after lease expiry.                                                                                             | `TestExecutionLockBlocksReclaimedWorkerWhileRunning` (`internal/question`)                                                                                                                                           | A holds the execution lock; the lease lapses; B reclaims (A is fenced from the checkpoint write and from completion) but B **cannot** acquire execution; only after A releases is the lock takeable.   |
| Holding execution ownership does not block the question store.                                                                                                   | `TestExecutionLockDoesNotBlockQuestionStore`                                                                                                                                                                         | Question creation and answer submission succeed while the execution lock is held (it is a separate file from the store lock).                                                                          |
| A stage-label change alone cannot pass.                                                                                                                          | `TestPlannerIntegrationStageLabelAloneCannotPass`                                                                                                                                                                    | With no successful stage work the continuation never becomes `COMPLETED`.                                                                                                                              |
| Recovery of the effect-before-completion gap does not replay.                                                                                                    | `TestPlannerRecoveryGapEffectMarker`                                                                                                                                                                                 | Effect marker present + completion not persisted → completion recorded with `implCalls==0` (no replay).                                                                                                |
| Reconciliation cannot consume owed work: `sop planner continue --apply` applies and reports but never claims, changes the checkpoint, or completes.              | `TestCLIPlannerReconcileDoesNotConsumeOwedWork`, `TestCLIPlannerAskStatusContinue`, `TestCLIPlannerCrashAfterApplyBeforeContinue` (`internal/cli`)                                                                   | After `continue --apply`, the continuation is **not** `COMPLETED`, the run stage is unchanged, and `planner status` still reports `CONTINUE` (still owed).                                             |
| Uncertain execution stops for human review — an effect that occurred before its success marker is neither replayed nor completed.                                | `TestPlannerRecoveryUncertainExecutionStopsForReview`                                                                                                                                                                | A durable start marker with no success marker → `implCalls==0` (no replay), continuation not `COMPLETED`, run parked at `WAITING_FOR_HUMAN` with a pending approval request, prior effect left intact. |
| Execution ownership is acquired BEFORE the checkpoint is modified; a worker without ownership leaves the checkpoint unchanged.                                   | `TestPlannerResumeStandsDownWithoutOwnership`                                                                                                                                                                        | With the execution lock held elsewhere, the gate stands down: no executor run, the run stage is byte-identical, and the continuation is not completed.                                                 |
| Markers are durable; stage work does not begin when the start marker cannot be persisted.                                                                        | `TestPlannerResumeStartMarkerWriteFailureDoesNotExecute`, `TestWriteFileRoundTripCreatesDirsAndOverwrites`, `TestWriteFileFailsOnDirectoryTarget` (`internal/durablefs`)                                             | A start-marker write failure surfaces, the executor does not run (`implCalls==0`), and no completion is recorded. The marker write is temp-file + file fsync + atomic rename + directory fsync.        |
| A malformed or unreadable marker is **uncertain**, never "absent" — so it cannot license a replay.                                                               | `TestPlannerRecoveryMalformedMarkersStopForReview` (table: malformed/truncated success and start markers)                                                                                                            | Each case: `implCalls==0` (no unsafe replay), continuation not `COMPLETED`, run parked at `WAITING_FOR_HUMAN`.                                                                                         |
| Reclaim/generation fencing across crash points.                                                                                                                  | `TestFenceAndMutateRejectsReclaimedWorker`, `TestClaimReclaimAndGenerationFencing`, `TestClaimBoundedAttempts`, `TestContinuationRecoveryCrashCases`, `TestContinuationConcurrentClaimOneWins` (`internal/question`) | One winner under concurrency; a stale generation is fenced; attempts are bounded; recovery converges on a completed continuation.                                                                      |
| Human gates still block continuation.                                                                                                                            | `TestPlannerIntegrationHumanGateBlocks`, `TestCLIPlannerRespectsHumanGate`                                                                                                                                           | The gate returns `handled=false` and the executor does not run across `WAITING_FOR_HUMAN`.                                                                                                             |

### 13.3 Actual integration boundary (honest)

- The executor hook (`plannerLifecycleGate`) runs **before** the destructive `runpkg.New`,
  so a parked run is neither reset nor advanced into a new attempt. The resume path uses
  `runpkg.Resume`; the question is bound to the epoch `New` wrote on the first,
  non-resumed invocation.
- The **completion boundary** is explicit: the continuation becomes `COMPLETED` only when
  the resumed lifecycle returns no error **and** the run reaches `PASSED`; the durable
  effect marker is written **before** the completion compare-and-set.
- Execution ownership is a **separate lock file** (`planner-effect-<cont>.lock`) held for
  the whole resumed work, distinct from the question-store lock, so planner-side reads and
  writes are never blocked by a running executor. Every lifecycle checkpoint write inside
  the resumed work happens under this exclusive ownership.
- `sop planner continue --apply` is a **reconciliation/reporting** entry, not a second
  executor: it applies an `ANSWERED` answer (idempotently) and reports an `APPLIED`
  continuation as owed. It does **not** claim, does **not** modify the run checkpoint, and
  does **not** mark the continuation `COMPLETED`. Only the guarded executor path
  (`sop run` → `plannerLifecycleGate` → `plannerResume`) claims, fences, writes the
  checkpoint, runs the stage work, and records completion. Completion semantics are
  therefore unified: a stage label alone never consumes owed work.
- `plannerResume` acquires execution ownership **first**; only then does it claim,
  revalidate the current guards (`Decide` against fresh state: human gate, cancellation,
  terminal task, plan-revision/run-epoch/repo binding), and — for a first attempt — write
  the checkpoint. A worker that cannot obtain ownership leaves both the claim and the
  checkpoint unchanged.
- **Uncertain execution is explicit.** The first attempt writes a durable _start marker_
  before running the stage work and a durable _success marker_ before the completion
  compare-and-set. On recovery: success marker present → complete without replay; start
  marker present with no success marker → the effect's outcome is uncertain → the run
  parks at `WAITING_FOR_HUMAN` with an approval request, and the continuation is neither
  replayed nor completed. A marker that is present but **unreadable or malformed** is
  treated the same as a started-without-success marker (uncertain), never as "absent".
- **Markers and the run checkpoint are durable.** Start/success markers and
  `runpkg.Run.PersistDurable()` use the shared `internal/durablefs` primitive (temp file +
  file fsync + atomic rename + directory fsync); `question.FileStore`'s existing
  `atomicWriteFile` now delegates to the same helper. Stage work does not begin if the
  start marker cannot be persisted, and success is recorded only after the run checkpoint
  (`state.json` at `PASSED`) has been durably persisted.

### 13.4 Modeled, not integration-tested

- **OS process crash**: the crash cases simulate interruption by manipulating store state
  and by goroutine/store-level concurrency; no test actually `kill -9`s a process
  mid-run. The durable records are exercised, but a real SIGKILL is not.
- **External side effects**: none are performed; the "external effect not covered"
  guarantee is a statement of scope, not an exercised path.
- **Real provider**: the integration tests use a deterministic fake executor
  (`scriptedPlannerAgent`); no network provider is called.
- **Windows fail-closed**: the store's fail-closed write is covered by a build-tagged
  test; `TryAcquireLock` returns `ErrLockUnsupported` on Windows (fail closed). This
  branch was verified on macOS/darwin.
- **Crash between the effect and its success marker** is _modeled_ by writing the start
  marker and an observable effect directly (and by writing corrupt markers), not by a real
  process kill; the durable-write path is exercised by `internal/durablefs` unit tests and
  the marker tests, but no test interleaves a real SIGKILL with an in-flight fsync.

### 13.5 Remaining limitations

- No live ChatGPT → answer application → executor continuation demonstration has been run
  yet. The tests prove the executor/continuation mechanics with a deterministic fake
  executor; a live planner round is still pending.
- Guarantees are **not** general exactly-once. Continuation dispatch is at-least-once only
  for a safe first attempt; when replay safety cannot be established (an effect may have
  occurred but its success marker is absent or unreadable), recovery stops for human review
  instead of replaying or recording completion. External (network) side effects remain
  outside the guarantee.
- Cross-process execution-lock concurrency is guaranteed on unix via `flock`; Windows
  fails closed rather than locking.
- A run parked for review (`WAITING_FOR_HUMAN`) is resolved by a human; a subsequent
  unqualified `sop run` sees the human gate and does not drive the planner continuation.
