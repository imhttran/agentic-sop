# Planner live-test findings: `needs_human` → `CONTINUE`, and a later successful ChatGPT loop

Status: three live runs recorded. The newest, `planner-2q-live`, is a two-question loop with a
real executor and is recorded first. The `planner-chatgpt` single-question run follows, and the
earlier `planner-real` attempt (the `needs_human` → `CONTINUE` policy) is preserved unchanged
below. A planner-context correction and its regression tests were added; the product-code diff
and proposed commit file list are at the end.

## Live result: `planner-2q-live` — two questions, real executor (2026-10-18)

A real model-driven executor raised **two** planner questions in **one** task and run epoch.
ChatGPT answered each over the planner-only MCP tunnel, SOP applied each answer, and the final
round completed with both decisions. This closes the gap left by the scripted-executor
`planner-2q` demo (SPIKE-006 §9.2, §9.3). Runbook:
[`../testing/PLANNER-TWO-QUESTION-LIVE.md`](../testing/PLANNER-TWO-QUESTION-LIVE.md) (attempt 3).

### Setup

- Binary `sop-2q` built from `59e67ec` (`claude/planner-decision-history`,
  [imhttran/agentic-sop#68](https://github.com/imhttran/agentic-sop/pull/68)), which passes the
  run's earlier applied decisions to a resumed stage.
- Fixture from `scripts/planner-2q-fixture-setup.sh` at `4f9b60b`: one task `T001`, planner
  enabled (`round_limit: 3`, no deadline), `ollama` / `deepseek-v4.1-flash:cloud`, native
  `tool` harness, acceptance check `go test ./...` mapped to the one criterion.
- Transport: Secure MCP Tunnel, profile `sop-status-spike`, wrapper
  `.run/tunnel/sop-mcp-planner.sh` running `sop-2q mcp --planner` in the fixture.

### Timeline (captured)

| Round | SOP / executor                                                                                                                                        | ChatGPT                                                             |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| 1     | `T001 PAUSED: WAITING_FOR_PLANNER`; `q-1` "Which ordering should normalized.txt use?" (options `sort=true`, `sort=false`); no deliverables            | `a-2` = `sort=true`, key `planner-2q-live-r2-q1-001`, `ACCEPTED`    |
| 2     | `q-1` `APPLIED`; "resumed from IMPLEMENTING; yielded to a new planner question, continuation superseded"; `q-5` about duplicates; **no deliverables** | `a-6` = `dedupe=false`, key `planner-2q-live-r2-q5-001`, `ACCEPTED` |
| 3     | "resumed from IMPLEMENTING and completed"; `1 completed this run`; final gate `PASS`                                                                  | —                                                                   |

### Final machine states (captured)

- `T001` `LOCAL_DONE`; outcome `status: "VERIFIED"`, `lifecycle: "LOCAL_DONE"`.
- `choices.txt` = `sort=true` / `dedupe=false`; `normalized.txt` = `apple`, `apple`, `banana`,
  `banana`, `cherry` (sorted, duplicates kept), matching both answers.
- Continuations: `q-1` `SUPERSEDED`, `q-5` `COMPLETED`; none `CLAIMED` or `PENDING`.
- Final executor summary (`implementation.md`): "Wrote choices.txt with the planner decisions
  (sort=true, dedupe=false) …".

### What this proves, and what it does not

- **Decision history reaches the final round.** Each round is a fresh model call, and no
  deliverable existed after round 2, so the executor kept the round-1 decision nowhere. The
  final round still wrote `sort=true` and attributed it to the planner. The only path for it
  is SOP's resumed input, which #68 adds.
- `sort=true` is also the value the model chose when it guessed in earlier attempts, so
  `choices.txt` alone is weaker evidence for that line; the empty working tree after round 2
  and the executor's attribution carry it. `dedupe=false` is the opposite of the model's
  earlier guess.
- The run trace does not record prompt text, so "the executor was shown both decisions" is
  inferred from the code path and the executor's summary, not from a captured prompt.
- Still operator-assisted: a human relayed each question to ChatGPT. Unattended operation
  remains unproven.

### Issues found during the run

- **Repository identity is not symlink-normalized.** `repoFingerprint`
  (`internal/cli/question.go`) compares absolute paths without resolving symlinks. With the
  question bound to `/var/folders/...` (the `$TMPDIR` spelling) and the planner server
  started in `/private/var/folders/...`, three answer submissions were refused as `STALE`
  ("repository changed"). It failed closed; the workaround is the `/var` spelling with `PWD`
  exported in the wrapper. A fix should resolve symlinks on both sides.
- **The planner server must be restarted after a fixture recreate**, or it keeps serving
  the deleted directory.
- **Report and gate disagree on failed attempt 2:** `report.md` listed "No findings" while
  `gate.json` failed on "1 unresolved finding(s) at a failing severity". Not yet diagnosed.

## Live result: `planner-chatgpt` — successful operator-assisted ChatGPT loop

A real model-driven executor raised one planner question through the real lifecycle, an answer
arrived from the **actual ChatGPT connector** over the planner-only MCP tunnel, SOP applied it,
and the executor resumed the same run to completion.

### Binary and source fingerprints

- Runner HEAD: `c890796db9b23fb479d7a37fbb71fb1dcc5a4240`.
- Source patch fingerprint (tracked diff plus the new test/doc files):
  `sha256=055c45face7e2caeb59134ae0f38ea1550dc600b01cf24e8bfe236dc9522a36d`.
- Dedicated binary `.run/spike/sop-planner-chatgpt` (built from the tested working tree, not the
  installed `sop`): `sha256=89d73e25ab23baea204e60a2a2e73846fd5e5612c60f4cc4c70497bb4e001fb4`.
- Model/provider: `ollama` / `deepseek-v4.1-flash:cloud`, native `tool` harness.

### The exchange (captured)

- Question `q-1` (revision 1, `PENDING`), project `planner-chatgpt`, task `T001`,
  run epoch `2026-10-16T03:40:51.057609Z`, plan revision `planner-chatgpt@chatgpt0001`,
  repository binding `/Users/imhttran/agentic-workspace/agentic-sop/.run/planner-chatgpt`,
  `return_stage=IMPLEMENTING`, deadline `2026-10-16T03:56:04.758852Z` (finite).
- Question text: "Should normalized.txt be sorted alphabetically (sort=true) or preserve the
  input order of input.txt (sort=false)? This choice will be recorded in choices.txt."
- Options: `sort=true (sorted alphabetically)`, `sort=false (preserve input order)`.
- Answer `a-2` = `sort=true (sorted alphabetically)`, `answered_by=chatgpt`, idempotency key
  `chatgpt-planner-chatgpt-q1-001`, `expected_revision=1`, submitted `2026-10-16T03:43:05.859221Z`;
  outcome `o-3 ACCEPTED`, then `o-4 APPLIED`.

### Provenance

The answer arrived over the planner-only MCP tunnel from the **actual ChatGPT connector**, not
from a local MCP stdio call (no `sop_answer_submit` was issued locally). Caveat: SOP stores
`answered_by` as a caller-supplied label with no identity verification, so the ChatGPT
attribution rest on the connector/tunnel path and the operator's confirmation, not on a
SOP-verified identity.

### Final machine states (captured)

- `run`: stage `PASSED`, `created_at 2026-10-16T03:40:51.057609Z` (epoch unchanged on resume).
- `T001`: `LOCAL_DONE`; run header `Done: 1` / `1 completed this run`; final gate `PASS`.
- `q-1`: `APPLIED` (outcomes `o-3 ACCEPTED`, `o-4 APPLIED`).
- Continuation `cont:q-1:2026-10-16T03:40:51.057609Z`: `COMPLETED` (generation 1, attempts 1,
  `completed_at 2026-10-16T03:44:22.995746Z`).
- Deliverables: `choices.txt` = `sort=true`; `normalized.txt` = `apple`/`banana`/`cherry`.
- No pending approval (`approvals --json` → `[]`).
- `trace.json`: `START → IMPLEMENT → DISCOVER → CHANGE×2 → VALIDATE → REVIEW → QUALITY PASS →
COMPLETE` — **no PLAN phase** (PLAN was not repeated on resume), `repository_mutations=2`.

### Live evidence vs deterministic tests

- **Live evidence (this run):** the persisted `questions.json`, `runs/T001/state.json`,
  `trace.json`, `validation.json`, `review.json`, `gate.json`, `outcome.json`,
  `implementation.md`, `state.db`, and the deliverable files.
  - The decision reaching IMPLEMENT is captured in `implementation.md`: "Planner decided
    sort=true (sorted alphabetically)…".
  - The decision reaching REVIEW is captured in `review.json`: "…The planner decision in the
    input is sort=true (sorted alphabetically)…".
  - FIX **did not run** (no `fix-*.md`; no `FIX` phase in the trace), so the FIX-input path is
    not exercised by this run.
- **Deterministic tests (not live):** the planner-context regressions in
  `internal/cli/planner_context_test.go` drive the same lifecycle with a fake executor and
  assert the decision reaches REVIEW and FIX, plus exact byte-equality and cache-key behavior.
  They prove the plumbing, never model behavior.

### Acceptance-check record (verbatim)

`go test ./...` was recorded as an **objective task acceptance check**, distinct from the
`validation` block. From `runs/T001/outcome.json`:

- `check-1`, `Required: true`, `Boundary: false`, `Met: true`,
  `Description: "acceptance check: go test ./..."`, `Command: "go test ./..."`,
  `Argv: ["go","test","-json","./..."]`, `ExitCode: 0`, with the captured raw JSON output.
- `criterion-1` (the free-text acceptance criterion), `Required: false`, `Met: false`,
  `Evidence: "unverifiable: no deterministic check supplied"` — informational; the
  deterministic `check-1` is what verifies the behavior.
- Overall outcome verbatim: `status: "VERIFIED"`,
  `reasons: ["every required criterion was independently met"]`, `lifecycle: "LOCAL_DONE"`.

The `validation.json` `UNIT_TEST` entry (`go test ./...`, `PASS`) is the separate configured
validation; the plan's `acceptance_checks: ["go test ./..."]` is the `check-1` above.

### Remaining limitations

- FIX did not run; the FIX-input decision path is covered only by deterministic tests.
- **Unattended operation remains unproven**: this was an operator-assisted loop (a human relayed
  the question to ChatGPT and the answer back through the connector). No unattended multi-round
  planner loop has been demonstrated.
- The run trace does not record prompt text, so "the model was shown the decision" is inferred
  from the review's own summary quoting it and from the code path, not from a captured prompt.

## Earlier failed attempt: `planner-clarify` — resumed run failed in the review→fix cycle

This run exercised the **local-MCP answer path** (the opposite of the successful run's connector
path) and exposed a product context gap. It is preserved as evidence.

### The exchange

- Project `planner-clarify`, task `T001`, question `q-1` (revision 1), run epoch
  `2026-10-16T03:18:21.561068Z`.
- Answer `a-2` = `sort=true (alphabetical ascending sort)`, idempotency key
  `chatgpt-planner-clarify-q1-001`; outcomes `o-3 ACCEPTED` → `o-4 APPLIED`.
- `planner continue --apply` → `T001 APPLIED q-1 (continuation pending execution)`.

### Provenance (correction)

The answer was submitted by a **local MCP stdio call** — a `printf … | .run/tunnel/sop-mcp-planner.sh`
(`sop_answer_submit`) request issued from a local shell — **not** by a ChatGPT connector. The
`answered_by="chatgpt"` value is a caller-supplied label stored by SOP with **no identity
verification**. This is the deliberate contrast with the later `planner-chatgpt` run, whose
answer did arrive from the actual ChatGPT connector.

### What happened

- The resumed executor produced **correct** deliverables (`normalized.txt` = apple/banana/cherry;
  `choices.txt` = `sort=true`); validation (`go build`, `go test`) PASSed.
- REVIEW (self engine) raised 1 HIGH + 1 MEDIUM + 1 LOW finding. The HIGH finding ("Ordering
  decision was not validated with the planner") was driven by **fixture contamination**: shell
  and audit logs placed **inside** the repository (`.run-clarify-audit.log`, `.run-clarify.log`)
  entered the invocation's change set and carried the **first run's** stale evidence (a failed
  `needs_planner`-as-a-tool attempt and an earlier failing `go test`).
- Gate `FAIL` → one FIX cycle. The FIX invocation claimed `ALREADY_SATISFIED` twice **without
  reading `choices.txt`** → rejected `UNREAD_EVIDENCE_PATH`. Classification
  `INCOMPLETE_IMPLEMENTATION / CONTINUE / HIGH`; run ended "resumed from IMPLEMENTING but NOT
  completed (stage FAILED); continuation retained".
- Final states: run stage `FAILED`; continuation `cont:q-1:2026-10-16T03:18:21.561068Z` `CLAIMED`
  (attempts 1); `T001` `READY`; **0 completions**; deliverables correct and `go test` passing.

### Corrective action

The review/fix inputs omitted the applied planner decision (only the resumed IMPLEMENT received
it), so a reviewer could not see that the delegated choice had been answered rather than guessed.
The **planner-context correction** (`internal/cli/run.go`) now carries the applied decision into
the REVIEW and FIX inputs (a strict no-op when no answer is present), with regression tests in
`internal/cli/planner_context_test.go`. Fixture hygiene was also corrected: future demo logs are
kept **outside** the target repository (as in `planner-chatgpt`).

## Earlier failed attempt: `needs_human` → `CONTINUE`

Investigation complete. No product-code change was warranted for the behavior below (it follows
an explicit, documented policy). The failed run and its fixture are preserved unchanged.

## What was run

A bounded live test drove a real model-driven executor through the native tool harness:

- Provider/model: `ollama` / `deepseek-v4.1-flash:cloud`, `harness: tool`.
- Capability: `IMPLEMENT`; `planner.enabled: true` (so `PlannerAvailable: true`), `round_limit: 3`.
- Result: **no planner question was created**; the run did not park at `WAITING_FOR_PLANNER`.

The model recognised the ambiguity and stopped, but returned a structured `needs_human` outcome
instead of `needs_planner`:

```json
{
  "reason": "The task's central semantics — whether normalized.txt is sorted and whether duplicate lines are removed — are explicitly declared undecided operator preferences, and the task instructs 'Do not guess; clarify the intended ordering and duplicate handling before implementing.' … An operator must specify (a) ordering (preserve input order vs sorted) and (b) duplicate handling (preserve duplicates vs remove duplicates) so I can write choices.txt and the matching normalized.txt.",
  "status": "needs_human"
}
```

SOP then reported `run T001: CONTINUE`, `classification: CONTINUE (INCOMPLETE_IMPLEMENTATION,
MEDIUM)`, requeued T001 (`continuation: 1/4`), run stage `FAILED`, and recorded **no** approval.

## Trace of the captured response

| Stage                                | Code                                      | Effect on the response                                                                                                   |
| ------------------------------------ | ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Native harness, reconcile            | `internal/ollamaagent/outcome.go:59-69`   | A non-`completed` outcome is returned **byte-for-byte unchanged**.                                                       |
| Native harness, planner interception | `internal/ollamaagent/harness.go:138-148` | Keyed on `outcomeStatus == "needs_planner"`; not taken.                                                                  |
| Native harness, no-change retry      | `internal/ollamaagent/outcome.go:134`     | Only rewrites a `failed` outcome. Not taken.                                                                             |
| Native harness, structured wrap      | `internal/ollamaagent/outcome.go:175-178` | A recognised `needs_human` is returned unchanged.                                                                        |
| `ParseOutcome`                       | `internal/agent/command.go:131`           | → `Outcome{Status: needs_human, Reason: <verbatim>}`. Status preserved.                                                  |
| Lifecycle                            | `internal/cli/run.go:775-785`             | Non-`completed`, no mutation → `outcomeResult(...)`.                                                                     |
| `failure.Classify` → `fromOutcome`   | `internal/failure/failure.go:443-479`     | No boundary action in the text → bare `needs_human` branch (`474-477`) → `IncompleteImplementation / Continue / Medium`. |
| `classifiedOutcome`                  | `internal/cli/run.go:1549-1555`           | `needs_human` **and** `Retryable()` → `CONTINUE`, stage `FAILED`.                                                        |
| Driver                               | `internal/cli/drive.go`                   | `CONTINUE` requeues the task (`PLANNED`); no human gate recorded.                                                        |

The structured `needs_human` **status is not lost** anywhere: it survives the harness and
`ParseOutcome` intact. What changes is the _disposition_: the classifier treats the status alone
as non-authoritative.

## Determination: explicit policy, not a lost signal

`fromOutcome` documents the rule directly (`internal/failure/failure.go:470-477`):

> SOP's own harness uses `needs_human` to request a bounded requeue — it is not authoritative
> that a human is needed — so an unexplained `needs_human` is unfinished work, not a human
> decision.

This is deliberate for two reasons visible in the code:

1. **SOP overloads `needs_human` as its own requeue signal.** `retryNoChangeFailure`
   (`outcome.go:141`) rewrites a no-change `failed` into `needs_human`, and `Generate` maps a
   `changeIncompleteError` to `needs_human` (`bridge.go:88-91`). Those carry the deterministic
   marker `"no repository change was made; retrying"` (`failure.go:601-607`), which is why the
   marker is consulted **before** the bare-status branch.
2. **Approval is structured, never inferred from prose** (`failure.go:642-649`,
   `classifiedOutcome` at `run.go:1539-1544`). A model's `needs_human` escalates to
   `NEEDS_HUMAN` only when its reason names an explicit human-boundary _action_ — a security
   decision (`failure.go:670-680`), a destructive/irreversible action (`651`), or an unresolved
   requirements conflict (`657-661`).

The captured reason ("undecided operator preferences", "an authoritative operator decision",
"An operator must specify…") contains none of those markers, so it matched the bare-status
branch. **This is the contract working as written, not a bug.**

Note (out of scope, no change): there is a usability tension worth flagging — the model-facing
contract invites `needs_human` for "<decision a human must make>" and the planner contract
routes approval decisions to `needs_human`, yet the classifier requires boundary-action wording
to honour it. That is a documentation/messaging mismatch, not a contract violation.

## Expected behavior

For the evidence actually supplied (`Source: "IMPLEMENT"`, `Outcome{needs_human}` with no
boundary-action markers, no current-run approval boundary, default `BALANCED` autonomy):

- Disposition `CONTINUE`, kind `INCOMPLETE_IMPLEMENTATION`, confidence `MEDIUM`.
- Run reported as `CONTINUE`; the task requeued to `PLANNED`.
- **No** `NEEDS_HUMAN`, **no** `WAITING_FOR_HUMAN`, **no** approval request created.

This is exactly what the live run produced.

## Regression test

`internal/cli/planner_contract_cli_test.go` → `TestCapturedNeedsHumanOutcomeContinues`.

- Uses the **captured** response verbatim (`capturedPlannerNeedsHuman`) parsed through
  `agent.ParseOutcome`, driven through the real lifecycle with the same relevant configuration
  (one-task graph, default `BALANCED` autonomy, `IMPLEMENT`, no mutation).
- Asserts: exit is `exitError`; stdout contains `run S001: CONTINUE`,
  `classification: CONTINUE`, `S001 CONTINUE (requeued)`; `NEEDS_HUMAN` is absent; the recorded
  classification is `CONTINUE / INCOMPLETE_IMPLEMENTATION`; `sop approvals --json` lists **zero**
  approvals.

Result: **PASS** (deterministic). `go build ./...`, `go vet`, and the `cli` / `failure` /
`ollamaagent` suites are green.

## Corrected live-test report (for the earlier `planner-real` attempt)

> **Superseded in part.** The first bullet below was true of the `planner-real` attempt only.
> The later `planner-chatgpt` run did produce a live `needs_planner` outcome from the real model
> and parked at `WAITING_FOR_PLANNER` (see the section at the top). The remaining bullets stay
> as recorded for that earlier attempt.

- ~~**`needs_planner` emission remains unproven.**~~ **Superseded:** the `planner-chatgpt` run
  produced a live `needs_planner` question from `deepseek-v4.1-flash:cloud` through the native
  harness. For the `planner-real` attempt specifically, the model chose `needs_human` instead.
- **Fixture framing is a hypothesis, not a proven cause.** The fixture deliberately framed the
  ambiguity as an "operator decision" / "undecided operator preferences". That plausibly steered
  the model to the `needs_human` (human-authority) channel rather than `needs_planner`
  (scope/implementation clarification). This is **not** established as the cause: only one
  sample was run, with no controlled variable.
- **Captured evidence vs code-path inference:**
  - _Captured_ (durable, from the run): the model's `needs_human` response
    (`.run/planner-real/.agent-sdlc/runs/T001/implementation.md`), the run trace
    (`trace.json`), the recorded classification (`report.md` / `classification.json`), the
    requeue (`continuations.txt`, task status `PLANNED`), and the empty approval listing.
  - _Inferred from the code path_ (the run trace does **not** record prompt text): that the
    model was shown the `needs_planner` contract. This follows from `planner.enabled: true` →
    `PlannerAvailable: true` (`run.go:727`) → `plannerAdvertised` (`prompt.go:179-181`) →
    `plannerOutcomeContract` (`prompt.go:187-196`), and is pinned by
    `TestPlannerAvailabilityReachesTheImplementRequest` and
    `TestContractAdvertisesPlannerForSupportedStages`. It is not a captured live prompt.

## Non-changes

No product code was changed (no contract violation). The failed run, its fixture, the tunnel,
the installed binary, the archived spike, and the previous demos are unchanged. Nothing was
committed or pushed.

## Product-code diff and proposed commit file list

The working tree (HEAD `c890796`) carries the planner-outcome contract, the planner-context
correction, and their tests. Tracked diff: 5 files, +125/-9.

**Proposed commit file list (exact):**

```
internal/agent/agent.go                          # Request.PlannerAvailable; PlannerRequest.Validate
internal/cli/planner_raise.go                    # raisePlannerQuestion uses req.Validate
internal/cli/run.go                              # IMPLEMENT sets PlannerAvailable; REVIEW/FIX carry the applied decision
internal/ollamaagent/harness.go                  # needs_planner interception; explicit rejection outcome
internal/ollamaagent/prompt.go                   # contractFor/plannerAdvertised/plannerOutcomeContract
internal/cli/planner_context_test.go             # resumed REVIEW/FIX carry the decision; byte-equality; cache key
internal/cli/planner_contract_cli_test.go        # PlannerAvailable reaches IMPLEMENT; captured needs_human regression
internal/ollamaagent/planner_contract_test.go    # contract advertisement; needs_planner bridge; rejection/malformed cases
docs/plans/planner-live-test-findings.md         # this document
```

**Excluded (must stay untracked):** `.run/` in its entirety — disposable demo repositories,
run artifacts, shell/audit/trace logs, the tunnel wrapper and its backups, and the dedicated
`spike/` binaries — plus any credentials or other local state. No generated binary is committed.

**Change summary:**

1. The native harness advertises `needs_planner` only when the planner is enabled AND the
   capability has a supported continuation (IMPLEMENT), and it never wraps a planner request as
   `completed`; an unsupported or malformed request becomes an explicit `failed` outcome.
2. The lifecycle passes planner availability across the agent-request boundary and injects the
   applied decision into the resumed IMPLEMENT, REVIEW, and FIX inputs (a strict no-op when no
   answer is present, so non-planner runs are byte-identical).
3. Regression tests cover the contract, the bridge, rejection/malformed cases, resumed-stage
   context propagation, exact byte-equality, and the review cache key.
