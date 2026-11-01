# SPIKE-006 — live two-question demonstration transcript (sanitized)

Durable, source-labeled record of the SPIKE-006 two-question live demonstration.
Sanitized: no credentials, tokens, or environment dumps. The chat client is the
real external **ChatGPT** conversation; the executor is a controlled **script**
(`.run/planner-2q-executor.sh`). The demonstration stops at the first `PENDING`
question until the client answers; this file is extended as the client answers.

## Setup

- Dedicated binary (built from the current committed source, HEAD `65955a8`):
  `.run/spike/sop-planner`.
- Transport: OpenAI Secure MCP Tunnel, `tunnel-client` v0.0.16, runtime/tunnel
  `sop-status-spike`; MCP command `.run/tunnel/sop-mcp-planner.sh` pinned to
  working directory `.run/planner-2q`.
- Planner surface: `sop_status`, `sop_question_list`, `sop_question_get`,
  `sop_answer_submit` (no approval tools).
- Fixture: `.run/planner-2q` (own git repo); `input.txt` =
  `banana/apple/cherry/apple`; deterministic acceptance check
  `output_test.go` (`go test ./...`), carried by the operator-owned
  `.agent-sdlc/plan.json`.

## Source legend

- **`[LOCAL]`** — produced in this workspace by the coding agent / SOP itself.
- **`[CHATGPT]`** — produced by the external ChatGPT client. **Not yet made** for
  this demonstration; slots below are reserved and must not be pre-filled.
- **`answered_by` is caller-supplied metadata, not client provenance.** It never
  proves which client made a call.

## `[LOCAL]` — fixture verification (before the run)

- `[LOCAL]` operator-owned plan and persisted task reconcile
  (`stage.id/objective/acceptance_criteria == task`), so the acceptance check is
  recovered by the lifecycle.
- `[LOCAL]` deterministic acceptance check validated standalone (throwaway copy):
  passes on a correct `normalized.txt` and **fails** (exit 1) on a wrong one.
- `[LOCAL]` controlled executor validated standalone: IMPLEMENT #1 → order
  question; IMPLEMENT #2 (decision present) → duplicates question; IMPLEMENT #3 →
  writes `choices.txt` + `normalized.txt`, reports `completed`.

## `[LOCAL]` — task started, parked at the first question

```
Running: T001 Two-question normalization demo
T001 PAUSED: WAITING_FOR_PLANNER (awaiting a planner answer)
```

Durable evidence:

- Question `q-1`: state `PENDING`, revision `1`, kind `order`,
  body _"Should the normalized list be sorted, or preserve input order?"_,
  options `sorted` / `preserve order`.
- Bound run epoch `2026-10-15T20:55:25.091106Z`; `return_stage` `IMPLEMENTING`;
  `plan_revision` `planner-2q@2q0001`.
- Run stage `WAITING_FOR_PLANNER`; task `T001` `READY`.
- Executor log (one IMPLEMENT call so far):
  `capability=PLAN planner_decision=none` /
  `capability=IMPLEMENT planner_decision=none`.
- No answer submitted, no continuation created (`answers: 0`,
  `continuations: 0`); repository unchanged.

## `[CHATGPT]` — Q1 retrieval and answer (PENDING — client-driven)

Sequential MCP calls the client is expected to make (one request at a time; do not
batch). Record the client's returned text verbatim here when it occurs.

1. `sop_status` → expect `T001 READY Two-question normalization demo`.
2. `sop_question_list` → expect `q-1 PENDING rev=1 …`.
3. `sop_question_get` `{ "question_id": "q-1" }` → expect the order question and
   options `sorted` / `preserve order`.
4. `sop_answer_submit` `{ "question_id": "q-1", "body": "sorted",
"idempotency_key": "chatgpt-planner-2q-q1-001", "expected_revision": 1 }`
   → `ACCEPTED`; question became `ANSWERED`.

Operator-attested result (per the operator's ChatGPT session): Q1 was retrieved
and answered by the ChatGPT client through the live plugin. Durable machine
evidence of the recorded answer (`[LOCAL]` store, not client provenance):

- Answer `a-2`: body `sorted`, `answered_by: chatgpt` (caller-supplied label),
  idempotency key `chatgpt-planner-2q-q1-001`; question `q-1` → `ANSWERED`.

## `[LOCAL]` — resume after the Q1 answer, second question raised

One `sop run` resume applied `q-1` (`ANSWERED -> APPLIED`), resumed the **same run
epoch**, and raised the **second** question:

- `q-1` → `APPLIED`; continuation `cont:q-1:2026-10-15T20:55:25.091106Z`
  (`CLAIMED`, generation 1) retained while the run parks again.
- **Second question `q-5`** (store-allocated id), state `PENDING`, revision `1`,
  kind `duplicates`, body _"Should duplicate entries be removed, or preserved?"_,
  options `remove duplicates` / `preserve duplicates`, bound to the **same** run
  epoch `2026-10-15T20:55:25.091106Z`, `return_stage` `IMPLEMENTING`.
- Run stage `WAITING_FOR_PLANNER`; run `created_at` (epoch) unchanged.
- Executor log: `capability=IMPLEMENT planner_decision=sorted` (2nd IMPLEMENT).
- Repository unchanged (no output written yet).

## `[CHATGPT]` — Q2 retrieval and answer

1. `sop_question_get` `{ "question_id": "q-5" }` (sequential).
2. `sop_answer_submit` `{ "question_id": "q-5", "body": "remove duplicates",
"idempotency_key": "chatgpt-planner-2q-q5-001", "expected_revision": 1 }`
   → `ACCEPTED`; `q-5` → `ANSWERED`.

Operator-attested result (per the operator's ChatGPT session). Durable machine
evidence of the recorded answer (`[LOCAL]` store, not client provenance):

- Answer `a-6`: body `remove duplicates`, `answered_by: chatgpt` (caller-supplied
  label), idempotency key `chatgpt-planner-2q-q5-001`.

## `[LOCAL]` — final resume, deterministic acceptance result (PASS)

One `sop run` resume applied `q-5` and completed the task in the **same run
epoch**:

```
T001 planner: resumed from IMPLEMENTING and completed (generation 1)
all tasks done (1 completed this run)
SOP COMPLETE — Final gate: PASS
```

Verified checks (not inferred from the stage label):

1. **Run epoch unchanged** — `created_at` `2026-10-15T20:55:25.091106Z` (stage
   `PASSED`).
2. **PLAN not repeated** — executor log has one `PLAN` (initial run); both resumes
   called only `IMPLEMENT` (plus one `REVIEW`).
3. **Both decisions reached the executor** — log: `IMPLEMENT planner_decision=sorted`
   then `IMPLEMENT planner_decision=remove duplicates`; `choices.txt` =
   `sort=true` / `dedup=true`; the first decision was carried in the executor's
   scratch state (`decision1=sorted`).
4. **`normalized.txt`** — exactly `apple\nbanana\ncherry\n` (sorted + deduplicated).
5. **Deterministic acceptance check** — `check-1` (`go test ./...`, argv
   `go test -json ./...`) exited **0**; outcome `VERIFIED` ("every required
   criterion was independently met"). The free-text `criterion-1` is informational
   (`Required=false`), superseded by the check's command.
6. **Both questions `APPLIED`** — `q-1` and `q-5`.
7. **Continuations** — `cont:q-5:2026-10-15T20:55:25.091106Z` `COMPLETED`
   (generation 1); `cont:q-1:2026-10-15T20:55:25.091106Z` remains `CLAIMED`
   (generation 1) — the first round's continuation was retained when the run
   parked on Q2 and was never reclaimed once the task reached a terminal state.
8. **Final task + completion count** — `T001 LOCAL_DONE`; `1 completed this run`
   (single count; the earlier double-count defect does not recur).

## Transport errors

None observed for either client call. Any transport error will be recorded here
verbatim, **without assigning a cause and without automatically resubmitting** an
answer.

## Post-demo finding and correction

- **Finding (preserved live result):** after `T001` reached `LOCAL_DONE`, the demo's
  `cont:q-1:2026-10-15T20:55:25.091106Z` remained **`CLAIMED`** (generation 1). This file
  preserves that original result; the demo artifacts were **not** edited.
- **Correction (verified by tests, not by rerunning the demo):** a continuation that
  yields to another planner question is now recorded **`SUPERSEDED`** — terminal, never
  `COMPLETED`, never claimable — so no continuation is left `CLAIMED` after success. A
  failure or a human gate is _not_ superseded (it stays `CLAIMED`, recoverable). See
  `SPIKE-006-feasibility-report.md` §12 for the trace, the correction, and the test
  evidence (including the fail-before-fix proof).
- **Executor note (explicit):** the demo executor retained the first planner decision in
  its own **scratch state** (`.run/planner-2q-executor.state/decision1`), because the
  resumed input carried only the latest planner decision. **SOP does not deliver a full
  decision history to the executor** unless independently verified.
