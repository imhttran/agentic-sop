# SPIKE-006 — live planner-loop feasibility report

Baseline: SPIKE-005 commit `65955a82eaad10d7ac72812f0654dbbc863205aa` (SPIKE-004 =
`64133c1`, SPIKE-003 = `34bdc87`). Client: **this ChatGPT conversation**
(operator-designated; see `SPIKE-002-connection-discovery.md`). This report reuses
the preserved live demo and the operator's ChatGPT transcript; the demo was **not**
rerun for this report.

> **Reconciled 2026-10-18.** This report was extended in layers (§12–§14 after the
> first draft), and the earlier sections were not revised to match. This pass makes the
> disposition, the acceptance table, §3, §8.1, §9, and §13 consistent with §14 and with
> the later live run in
> [`planner-live-test-findings.md`](planner-live-test-findings.md). No live evidence was
> rerun or edited. §15 lists every change.

## 0. Disposition

**GO for the operator-assisted planner loop, one or two questions, with a real executor.**
The two-question result depends on decision-history delivery,
[imhttran/agentic-sop#68](https://github.com/imhttran/agentic-sop/pull/68), now merged; a
binary built before it delivers only the latest decision. The evidence:

- **Two-question loop with a real executor: GO** (`planner-2q-live`, 2026-10-18,
  [`planner-live-test-findings.md`](planner-live-test-findings.md)). A model-driven
  executor raised two questions in one task and run epoch; ChatGPT answered both over the
  tunnel; the final round wrote both decisions with no deliverable existing before it, and
  completed `VERIFIED` / `LOCAL_DONE`. The binary included #68 (§9.2).
- **Two-question loop with a scripted executor** (`planner-2q`, §14): superseded by the
  run above. Its script kept the first decision in its own scratch state, because SOP then
  delivered only the latest decision.
- **Single-question loop with a real executor: GO.** The later `planner-chatgpt` run
  ([`planner-live-test-findings.md`](planner-live-test-findings.md)) used a model-driven
  executor (`ollama` / `deepseek-v4.1-flash:cloud`): it raised one question, ChatGPT
  answered over the tunnel, SOP applied it, and the run resumed in the same epoch to
  `LOCAL_DONE` with a `VERIFIED` acceptance check.
- The deterministic durability/authority checks pass (§2, §12, §13).

Scope and explicit non-claims:

- **In scope (GO):** an operator-assisted ChatGPT planner loop — a human drives ChatGPT
  to read a planner question and submit a bounded answer; SOP applies it and the executor
  resumes. Durability, fencing, stale/duplicate/authority, timeout, and crash-gap recovery
  are covered by deterministic tests.
- **Not in scope (unverified):** **unattended** operation. Interactive success is not
  unattended success.
- **Open:** the HTTP 409 MCP initialization error (§6) and the timeout-test failure
  (§13) remain unresolved.

## 1. Client, transport, and setup

- **Intended planner client:** this ChatGPT conversation (real, external).
- **Transport:** OpenAI **Secure MCP Tunnel**, no public exposure.
  `tunnel-client` **v0.0.16** (`/Users/imhttran/.local/bin/tunnel-client`), profile/
  tunnel `sop-status-spike`, process-mode supervised runtime.
- **MCP command (tunnel side):** `.run/tunnel/sop-mcp-planner.sh` → runs the
  dedicated binary `.run/spike/sop-planner mcp --planner` with a **fixed working
  directory** `.run/planner-live`.
- **Exposed planner surface (verified):** `sop_status`, `sop_question_list`,
  `sop_question_get`, `sop_answer_submit`. **No** human approval tools
  (`sop_approve`/`sop_decline`/`sop_approval`) are registered — SPIKE-003
  approval-tool isolation, enforced server-side.
- **Setup to reproduce:** install `tunnel-client`; create a tunnel + runtime key;
  associate it with the ChatGPT workspace; run the wrapper above; in ChatGPT add the
  custom MCP server as a **Tunnel**; `@`-mention the plugin and instruct it to use
  the planner tools.

## 2. Acceptance criteria → evidence

| Criterion (plan)                                                                                        | Evidence                                                                                                                                                                                | Status                            |
| ------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- |
| Real-client transcript plus machine evidence, limitations, and a GO/PARTIAL/NO-GO outcome               | `SPIKE-002-connection-discovery.md` §9–§10.2; `SPIKE-004-live-evidence.md`; durable demo records (§4)                                                                                   | **met** (PARTIAL overall; §0)     |
| Report identifies real vs simulated components, transport limits, session requirement                   | §7; §8                                                                                                                                                                                  | **met**                           |
| A Zed-agent-only result is reported as PARTIAL, not the external-client gate                            | Result is **not** Zed-only: the external client (ChatGPT) is verified                                                                                                                   | **met**                           |
| Checklist: a real client lists SOP tools and reads correct repo status                                  | `SPIKE-002` §9 (`sop_status` → "no tasks")                                                                                                                                              | **met (live)**                    |
| Checklist: two distinct questions reach that planner and their answers resume the same task through SOP | Live in `planner-2q` (§14): `q-1` then `q-5` in one run epoch, both answered by ChatGPT and applied, task `LOCAL_DONE`; executor scripted. Also `TestPlannerIntegrationTwoRoundsOneRun` | **met (live, scripted executor)** |
| Checklist: restart while waiting retains the question, blocks retry                                     | `TestPlannerRestartWhileWaitingReparks`                                                                                                                                                 | **met (deterministic)**           |
| Checklist: restart after application resumes at most once; duplicate delivery no extra effect           | `TestContinuationCreatedAtomicallyWithApplication`, `TestPlannerResumeCountsOneCompletion`, `TestApplyIdempotentNoDoubleDispatch`                                                       | **met (deterministic)**           |
| Checklist: changed repo/plan revision rejects a stale answer without executing                          | `TestCLIPlannerStaleBindingSkipped`, `TestCLIAnswer*Stale`, `TestSubmitAnswer*Stale`, `TestDecideGuards`                                                                                | **met (deterministic)**           |
| Checklist: a human-only authorization cannot be satisfied by the planner                                | `TestPlannerAnswerCannotSatisfyHumanGate`, `TestPlannerIntegrationHumanGateBlocks`                                                                                                      | **met (deterministic)**           |
| Checklist: timeout/disconnect/exhausted rounds pause with evidence                                      | `TestPlannerTimeoutParksWithoutRetry`, `TestPlannerRestartWhileWaitingReparks`, `TestPlannerExhaustedRoundsParkAtHumanBoundary`, `TestDecidePauseOnTimeout`                             | **met (deterministic)**           |
| Checklist: failure counters unchanged; disabled mode preserves behavior                                 | `TestPlannerClarificationPreservesFailureBudget`, `TestPlannerDisabledIgnoresExistingQuestion`                                                                                          | **met (deterministic)**           |
| Checklist: targeted tests pass; missing/failed reported truthfully                                      | `go test ./...` green; gaps below reported                                                                                                                                              | **met**                           |

## 3. The first live sequence: one question (`planner-live`)

This section records the first, one-question demo. The two-question demo
(`planner-2q`) is in §14 and `SPIKE-006-live-transcript.md`; the real-executor run
(`planner-chatgpt`) is in [`planner-live-test-findings.md`](planner-live-test-findings.md).

Machine evidence (durable, `.run/planner-live/.agent-sdlc/`):

1. **Executor question (not seeded).** The controlled executor returned a structured
   `needs_planner` outcome during IMPLEMENT through the real lifecycle; SOP persisted
   `q-1` and parked the run.
   - `q-1`, `submitted_by: executor`, revision 1, body _"Which documentation format
     should this planner-live demo use?"_, options `Markdown` / `plain text`,
     `return_stage: IMPLEMENTING`.
2. **Live ChatGPT answer.** This ChatGPT conversation submitted the answer through
   the planner MCP surface: answer `a-2`, body `Markdown`, `answered_by: chatgpt`,
   idempotency key `chatgpt-planner-live-q1-001`. Question → `ANSWERED`.
3. **Application.** On the resume, SOP applied the answer (`ANSWERED → APPLIED`) and
   created the durable continuation `cont:q-1:2026-10-15T20:15:44.301822Z`.
4. **Executor continuation.** The executor resumed the same run/epoch and completed;
   continuation `COMPLETED` (generation 1). Executor log:
   `capability=IMPLEMENT planner_decision=Markdown`, then `capability=REVIEW`.

**Run epoch preserved; PLAN skipped.** The run's `created_at` is
`2026-10-15T20:15:44.301822Z` **before and after** the resume (stage `PASSED`), i.e.
the same epoch — the resume used `runpkg.Resume`, not a destructive `New`. The PLAN
stage was **not** repeated: the resumed attempt starts at `IMPLEMENTING`, the run's
`plan.md` from the parking invocation is reused, and the executor log for the resume
contains **no** `PLAN` call.

## 4. Final states and the original acceptance result

- **Question:** `q-1` → `APPLIED` (revision 1).
- **Answer:** `a-2` = `Markdown` (`answered_by: chatgpt`).
- **Continuation:** `cont:q-1:2026-10-15T20:15:44.301822Z` → `COMPLETED` (gen 1,
  return stage `IMPLEMENTING`).
- **Task:** `T001` → `LOCAL_DONE`.
- **Run:** stage `PASSED`, gate `PASS`, fix cycles `0/3`; validation
  `PASS BUILD true`, `PASS UNIT_TEST true`; review `clean`.

**Original PARTIAL result (preserved, unedited).** The demo's `report.md` records
outcome `Status: PARTIAL` / `Lifecycle: LOCAL_DONE` because the disposable task
declared an acceptance criterion but no deterministic acceptance command, so
`objective-acceptance` is unverifiable. This did not block `PASSED`/`LOCAL_DONE`/gate
`PASS`.

**Supplemental file check.** `docs-format.md` contains exactly
`Documentation format chosen by the planner: Markdown` (+ trailing newline; verified
with `od -c`). Recorded as _supplemental_ evidence that the answer reached the
resumed executor — not a substitute for a deterministic acceptance command.

## 5. Completion-count defect, fix, and regression evidence

The one-task demo printed **`2 completed this run`**. Root cause: the planner resume
completed the run stage and the continuation but never finalized the **task** (unlike
the normal lifecycle), so the scheduler finished the still-runnable task in a second
pass and counted it again. Fix (commit `65955a8`): `plannerResume` calls `completeTask`
on success, and `completeTask` promotes a not-yet-promoted `PLANNED` task to `READY`
first. Regression: `TestPlannerResumeCountsOneCompletion` — proven fail-before
(`2 completed this run`) and pass-after (`1 completed this run`). The preserved demo
report was **not** rewritten; the corrected behavior is verified by test, not by
rerunning the demo.

## 6. Open issue — HTTP 409 MCP initialization (root cause unresolved)

Observed during a live ChatGPT session: **HTTP 409**, `error_type:
mcp_initialization_required`, **origin `tunnel-client`**. A **parallel**
`question-get` failed while the concurrent `status` succeeded; a **sequential**
`question-get` retry succeeded; **no answer was resubmitted**. Not reproduced here
(MCP fixtures clean; tunnel health `PASS`; the server implements `initialize` and
enforces no "must initialize first" gate). **Tracked separately; cause not
assigned.** Details in `SPIKE-005-verification-report.md` §6.

## 7. Separation of what is proven

- **Proven live behavior:** the real external client (ChatGPT) listed SOP tools and
  read repository status (SPIKE-002 §9); and, for `.run/planner-live`, it retrieved
  the executor-raised question and submitted the answer that SOP applied and the
  (scripted) executor resumed to `LOCAL_DONE` (SPIKE-004 live evidence).
- **Deterministic test coverage:** restart/duplicate/stale/authority/timeout/exhausted
  behavior and the two-round loop, with a scripted executor (`internal/cli/planner_*_test.go`,
  `internal/question/*_test.go`).
- **Modeled crash recovery:** crash points simulated via durable start/success markers
  and store/goroutine concurrency (no real `SIGKILL`); an uncertain continuation stops
  for human review rather than replaying.
- **Unproven capabilities / remaining work:** a real (model-driven) executor; a live
  multi-round interactive loop in one task; unattended operation; and the broader
  workflow capabilities in §8.

## 8. Feasibility assessment

### 8.1 Operator-assisted ChatGPT planner loop

**Supported for one and two questions with a real executor** (two questions need #68, merged).
An operator can, in a ChatGPT session, list/read the planner question and submit an
answer; SOP applies it and the executor resumes, preserving the run epoch and skipping
PLAN. Both loops ran live with a model-driven executor (`planner-chatgpt`,
`planner-2q-live`). Remaining work: fix repository identity so a `/var` versus
`/private/var` path spelling on macOS does not refuse answers as stale (see
[`planner-live-test-findings.md`](planner-live-test-findings.md)).

### 8.2 Unattended multi-round planner loop

**Not supported — requires more work.** Answering requires an interactive ChatGPT
session (no unattended answer path), the tunnel runtime must stay healthy, and the
executor in the demo is scripted. Bounded multi-round behavior is proven only
deterministically. Unattended operation is out of scope for this spike.

### 8.3 Broader workflow (not exercised by this spike)

None of the following is established by this spike; each needs separate work.

| Capability                                                         | Assessment                          | Basis                                                                                                                                                                                    |
| ------------------------------------------------------------------ | ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| PR splitting (one task → multiple PRs)                             | **Requires more work / not proven** | `sop pr` exists (single task branch → PR, needs `--yes`); no multi-PR split capability was exercised, and the spike performed no Git publication                                         |
| Parallel branches                                                  | **Requires more work / not proven** | `internal/parallel`, `internal/orchestration` (opt-in, default off) and worktree support exist, but were not exercised with the planner; the planner loop is single-task sequential      |
| Skill waivers (designated-approver criterion waivers, SOP-HARD-04) | **Requires more work / not proven** | `sop outcome <task-id> --waive` exists and fails closed without designated approvers, but is not integrated with the planner; the planner surface deliberately excludes approval actions |
| PR preparation                                                     | **Requires more work / not proven** | `sop pr` exists behind the human commit/PR gate; the live demo used `workflow.mode: local` and performed no push/PR                                                                      |

Feasibility for the **broader workflow is therefore unproven**, and must not be read
as supported on the strength of the planner loop.

## 9. Evidence gaps (exact)

1. **Live two-question resume in one task — CLOSED (in the two-question demo).** The
   `planner-2q` live demo raised `q-1` then `q-5` and ChatGPT answered both, resuming the
   same task/run epoch to `LOCAL_DONE` (§3). (The earlier one-question demo and the
   recorded-only `q-docformat-chatgpt` remain as they were.)
2. **Decision-history delivery to the executor — fixed in code, not yet live.** In both
   demos SOP delivered only the _latest_ decision to the resumed stage; the controlled
   executor supplied the earlier decision from its own scratch state.
   [imhttran/agentic-sop#68](https://github.com/imhttran/agentic-sop/pull/68) passes the
   run's earlier applied decisions (same task, run epoch, and plan revision), oldest
   first, ahead of the latest one. It is covered by `TestPlannerIntegrationTwoRoundsOneRun`
   and `TestEarlierPlannerDecisionsScope`, and was exercised live in `planner-2q-live`
   (2026-10-18): no deliverable existed after round 2, and the final round wrote the
   round-1 decision. **Closed** (#68 merged).
3. **Real executor — single question only.** The SPIKE-006 demos used a controlled
   script. The later `planner-chatgpt` run used a model-driven executor for one question
   ([`planner-live-test-findings.md`](planner-live-test-findings.md)), and the
   `planner-2q-live` run used it for two questions. **Closed.**
4. **Client transcript as a durable artifact:** the live evidence references the
   operator's ChatGPT tool-call transcript; a machine-captured copy is not stored in the
   repository. `SPIKE-006-live-transcript.md` records the operator-attested exchange with
   the durable machine evidence and the `answered_by`-is-not-provenance caveat.
5. **HTTP 409 MCP initialization:** unresolved (§6).
6. **`TestCrossModuleTimeoutAndCancellation`:** failed once under full-suite load;
   unresolved, no cause assigned (§13).

## 10. Evidence references (verified present)

Plan: `docs/plans/SOP-Planner-Connection-Spike.md`. Discovery/transport:
`docs/plans/SPIKE-002-connection-discovery.md`, `docs/plans/SPIKE-002-planner-client-transport.md`.
Contracts: `docs/plans/SPIKE-004-implementation-proposal.md`. Live evidence:
`docs/plans/SPIKE-004-live-evidence.md`. Tests/limits:
`docs/plans/SPIKE-005-verification-report.md`. Demo artifacts:
`.run/planner-live/.agent-sdlc/questions.json`,
`.run/planner-live/.agent-sdlc/runs/T001/state.json`,
`.run/planner-live/.agent-sdlc/runs/T001/report.md`,
`.run/planner-live/.agent-sdlc/runs/T001/plan.md`, `.run/planner-live/docs-format.md`,
`.run/planner-live-executor.log`. Transport: `.run/tunnel/sop-mcp-planner.sh`,
`.run/spike/sop-planner`. Code/tests: `internal/cli/planner.go`,
`internal/cli/planner_spike005_test.go`, `internal/cli/planner_integration_test.go`,
`internal/cli/planner_test.go`, `internal/question/*_test.go`.

## 11. Boundaries honored

The live demo was reused, not rerun. No commit, push, merge, plan historicalization,
task completion, tunnel change, or installed-binary change was made for this report.
No implementation scope was expanded.

## 12. Retained-continuation finding and correction (post-live)

The live two-question demo left a **retained continuation**. This section records
the finding, the terminal outcome chosen, and the correction. **The original live
result above is preserved unchanged** — the demo's artifacts were not edited.

### Finding

After `T001` reached `LOCAL_DONE`, `cont:q-1:2026-10-15T20:55:25.091106Z` remained
**`CLAIMED`** (generation 1, never completed). Trace: on the round-1 resume,
`plannerResume` claimed `cont:q-1`, wrote the checkpoint, and ran the resumed stage —
which raised the **second** question and parked the run at `WAITING_FOR_PLANNER`. The
success branch requires `PASSED`, so the continuation was left `CLAIMED`; the round-2
resume only handled the active question (`q-5`), so `cont:q-1` was never revisited,
and once the task became terminal, reconciliation skipped it (`SkipTerminal`),
stranding it.

### Terminal outcome (defined)

A continuation that **yields to another planner question** gets a distinct terminal
state, **`SUPERSEDED`** — its owed work was taken over by the subsequent round. It is
**not** `COMPLETED`: unfinished work is never marked complete merely because it parked.
`SUPERSEDED` is terminal, is never claimable, and is never replayed.

### Correction (smallest)

- `internal/question/continuation.go` — new `ContinuationSuperseded` state (+ `Terminal()`).
- `internal/question/{store,filestore,service}.go` — fenced `SupersedeContinuation`
  (`CLAIMED -> SUPERSEDED` under the worker's generation; a stale generation is refused,
  leaving the claim intact). `ClaimContinuation` refuses terminal states.
- `internal/question/reconcile.go` and `activeQuestion` — a question whose continuation is
  terminal is no longer the active question, and `Decide` treats it as nothing owed.
- `internal/cli/planner.go` — on a resumed run that parks at `WAITING_FOR_PLANNER` (a new
  question was raised), the current continuation is superseded. A **failure** or a **human
  gate** is _not_ superseded (it stays `CLAIMED`, recoverable).

Claim fencing, audit history, and no-replay behavior are preserved.

### Test evidence

- `TestPlannerIntegrationTwoRoundsOneRun` (extended): both answers applied; task
  `LOCAL_DONE`; `cont:q-1` `SUPERSEDED`, `cont:q-5` `COMPLETED`; no continuation is
  `CLAIMED`/`PENDING` or claimable afterwards; reconciliation executes nothing again.
- `TestSupersedeContinuationFencedAndTerminal`: a fenced supersede is refused (claim
  intact); the holder's supersede terminals; never completed; never re-claimable; persists
  across reopen.
- `TestPlannerContinuationRetainedOnFailure`: a failed resume leaves the continuation
  `CLAIMED` and does not complete the task.
- **Fail-before-fix:** with the supersede call disabled, the extended two-round test fails
  exactly as the demo presented (`cont:q-1 … State:CLAIMED`).

### Decision-history caveat

The demo **executor** retained the first planner decision in its own **scratch state**
(`.run/planner-2q-executor.state/decision1`), not through SOP: on the final round the
resumed input carried only the latest planner decision, so the executor read the earlier
one from its scratch file. **SOP does not deliver a full decision history to the
executor** — that is not independently verified here.

## 13. Supersede handoff — crash-recovery review

Review of the supersede handoff (the §12 correction) for crash recovery. Both live demos
and their original evidence are preserved. (The code was later committed in `fb956e3` and
merged to `main` with PR #7.)

### Supersession requires a real successor (not the stage alone)

Enforced in code: supersession is gated on a **distinct, durably stored successor
question** — a later question bound to the **same task, run epoch, and plan revision**,
still live (`PENDING`/`ANSWERED`/`APPLIED`) — via `plannerHasSuccessor`.
`WAITING_FOR_PLANNER` alone is **not** sufficient: if no successor is found the
continuation is left `CLAIMED` (fail-closed). Both the inline supersede in `plannerResume`
and the gate's `plannerResolvePredecessors` use this guard.

### Crash gap (tested)

`TestPlannerSupersedeCrashGapRecovery` reproduces the gap exactly: the successor question
is persisted and the predecessor continuation is `CLAIMED`, but supersession never ran
(the process stopped). On re-entry, recovery:

- selects the successor question;
- resolves the predecessor to `SUPERSEDED` (terminal, **not** `COMPLETED`) without replaying
  its stage work (`implCalls == 0`);
- keeps the successor owed (non-terminal);
- leaves no `CLAIMED`/`PENDING` — and no claimable — continuation after final success.

`TestPlannerSupersedeRecoveryRespectsHumanGate` proves recovery does **not** bypass a human
gate: at a human gate the predecessor is left `CLAIMED` and the planner does not act.
`TestResolveYieldedContinuationRecoversCrashGap` (store) proves the recovery transition is
`CLAIMED -> SUPERSEDED` only (never from `PENDING`, never `COMPLETED`, never re-claimable)
and persists across reopen. Claim fencing and audit evidence are preserved.
**Fail-before-fix proven:** disabling the gate recovery leaves `cont:q-1 … State:CLAIMED`
and the crash-gap test fails.

### Checks, and an unresolved failure

`gofmt` clean; `go build ./...` ok; `go vet ./...` ok; the targeted planner and question
suites pass; the full `go test ./...` passed on this run. **One earlier full-suite run
failed `TestCrossModuleTimeoutAndCancellation`** under load. It passes in isolation and on
reruns, but a passing rerun does **not** explain the failure: it is recorded as
**unresolved** (no cause assigned). Observed only under full-suite load; not attributed to
the planner changes.

## 14. Final evidence separation and closure

### Live evidence (real external client)

- ChatGPT (this conversation) listed SOP tools and read repository status
  (`SPIKE-002` §9).
- The **live two-question loop** completed for `planner-2q`: the executor raised `q-1`
  (order) then `q-5` (duplicates) in **one run epoch**; ChatGPT answered both; SOP applied
  each and the executor resumed to `LOCAL_DONE`; the deterministic acceptance check passed
  (`VERIFIED`). See §3 and `SPIKE-006-live-transcript.md`.

### Post-demo regression tests (deterministic, not live)

The supersede / crash-gap / authority behavior is proven by **tests**, not by rerunning
any live demo: `TestPlannerIntegrationTwoRoundsOneRun` (extended),
`TestPlannerSupersedeCrashGapRecovery`, `TestPlannerSupersedeRecoveryRespectsHumanGate`,
`TestPlannerSupersedeRecoveryRespectsStaleBinding`, `TestSupersedeContinuationFencedAndTerminal`,
`TestResolveYieldedContinuationRecoversCrashGap`, plus the SPIKE-005 suites.

### Preserved original demo result

Both demos are preserved unchanged and were **not** rerun. `.run/planner-live` still
records `Status: PARTIAL` / `LOCAL_DONE`; `.run/planner-2q` still shows its original
records, including `cont:q-1:…` `CLAIMED` (the pre-correction state).

### Executor caveat (explicit)

The controlled demo executor **stored the first planner decision in its own scratch
state** (`.run/planner-2q-executor.state/decision1`), because SOP delivered only the latest
decision to the resumed stage. **SOP does not deliver a full decision history to the
executor** unless independently verified.

### Open items

- HTTP 409 `mcp_initialization_required` (§6) — unresolved.
- `TestCrossModuleTimeoutAndCancellation` failed once under full-suite load and passed on
  rerun; the failure is **unresolved** (no cause assigned) (§13).

## 15. Reconciliation changes (2026-10-18)

Editorial only; no live evidence was rerun or edited.

- §0: disposition changed from a scoped GO to **PARTIAL overall, GO for single-question
  use with a real executor**, applying the plan's rule that an unverified real executor
  is PARTIAL and citing the later `planner-chatgpt` run.
- §2: the acceptance-table rows for the outcome and the two-question checklist item now
  match §14.
- §3: retitled as the first, one-question demo, with pointers to the later runs.
- §8.1: remaining work updated (the two-question loop ran live with a scripted executor).
- §9.2: decision-history delivery is fixed in PR #68, not yet exercised live.
- §9.3: the later real-executor run is recorded.
- §13: "the code remains uncommitted" is no longer true; the commit and merge are noted.

Follow-up (2026-10-18, after the `planner-2q-live` run): §0 changed to **GO for one or
two questions with a real executor**, now that #68 is merged; §8.1, §9.2, and §9.3
updated to match.

Still open: the HTTP 409 MCP initialization error (§6) and the one-time
`TestCrossModuleTimeoutAndCancellation` failure (§13).
