# SPIKE-004 — live ChatGPT continuation round-trip (recorded evidence)

Status: **recorded**. This document preserves the evidence of the first live
ChatGPT → tunnel → planner MCP → executor continuation round-trip. It is the
supplemental acceptance evidence for SPIKE-004's "executor raises a question → a
planner answers → SOP applies it → the executor resumes and completes" loop.

The demo's own artifacts are **not** modified: the disposable project, its run,
and its original `report.md` (including the `PARTIAL` outcome) are preserved as
produced. This file only records what was observed.

## Environment

- Dedicated binary: `.run/spike/sop-planner` (built from this branch; the
  installed `~/go/bin/sop` was never replaced).
- Tunnel runtime: `sop-status-spike`; MCP command wrapper
  `.run/tunnel/sop-mcp-planner.sh` pinned to working directory
  `.run/planner-live`.
- Planner MCP surface exposed (and only this): `sop_status`,
  `sop_question_list`, `sop_question_get`, `sop_answer_submit`. No human
  approval tools are registered.
- Disposable project: `.run/planner-live` (its own git repo), planner enabled,
  `agent.provider: command` using the controlled executor
  `.run/planner-live-executor.sh`.

## 1. Executor-raised question (not seeded)

The controlled executor returned a structured `needs_planner` outcome during
IMPLEMENT through the real lifecycle. SOP persisted the question and parked the
run:

- Question `q-1`, `submitted_by: executor`, revision `1`, state `PENDING`.
- Body: "Which documentation format should this planner-live demo use?"
- Options: `Markdown`, `plain text`.
- Run epoch: `2026-10-15T20:15:44.301822Z`; `return_stage: IMPLEMENTING`;
  `plan_revision: planner-live@live0001`;
  repo `/Users/imhttran/agentic-workspace/agentic-sop/.run/planner-live`.
- Run stage after the raise: `WAITING_FOR_PLANNER`.

## 2. Live ChatGPT answer

Read back through the planner MCP surface and the durable store:

- `q-1` reached state `ANSWERED` (then `APPLIED` on resume).
- Answer `a-2`: body `Markdown`, `answered_by: chatgpt`, idempotency key
  `chatgpt-planner-live-q1-001`.

## 3. Executor continuation (one `sop run` invocation)

The preserved demo's verbatim output (this ran **before** the double-count fix, so it
shows the defect recorded in §6):

```
Running: T001 Disposable planner-live demo task
T001 planner: resumed from IMPLEMENTING and completed (generation 1)
Resumed: T001 Disposable planner-live demo task (already past the local gates; completed locally)
all tasks done (2 completed this run)
SOP COMPLETE
Project: planner-live
Tasks: 1/1 complete
Final gate: PASS
Report: .agent-sdlc/runs/T001/report.md
```

The `2 completed this run` for a single task is the double-count defect fixed in §6;
the regression test confirms the corrected count is `1` without re-running the demo.

- Continuation `cont:q-1:2026-10-15T20:15:44.301822Z` reached `COMPLETED`
  (stage `IMPLEMENTING`, generation `1`).
- Final task state: `T001 LOCAL_DONE`.
- Run stage: `PASSED`; run epoch preserved
  (`created_at = 2026-10-15T20:15:44.301822Z`).
- PLAN was **not** repeated: the executor log for the resume contains no PLAN
  call (only `IMPLEMENT planner_decision=Markdown`, then `REVIEW`), and the
  resume skipped the PLAN stage.
- Validation: `PASS BUILD true`, `PASS UNIT_TEST true`; review `clean`;
  gate `PASS`; fix cycles `0/3`.

### Executor log evidence

```
ts=2026-10-15T20:28:23Z capability=IMPLEMENT planner_decision=Markdown
ts=2026-10-15T20:28:23Z capability=REVIEW planner_decision=none
```

## 4. Supplemental acceptance evidence

The resumed executor wrote `docs-format.md` in the disposable project containing
exactly this line (verified with `od -c`; one trailing newline):

```
Documentation format chosen by the planner: Markdown
```

This is supplemental evidence that the ChatGPT planner answer reached and acted
in the resumed executor. It is **not** a substitute for a deterministic
acceptance command in the task contract (see the preserved `PARTIAL` outcome
below).

## 5. Original outcome preserved (`PARTIAL`)

The demo's `report.md` was left as produced; its outcome is `PARTIAL` because the
disposable task declared an acceptance criterion but no deterministic acceptance
command, so `objective-acceptance` is unverifiable:

```
- Status: `PARTIAL`
- Lifecycle: `LOCAL_DONE`
- `deterministic-checks` met — the configured build/test/lint checks and the review passed (met)
- `objective-acceptance` not met — the specification declares no executed, passing acceptance check
  (unverifiable: no deterministic check supplied)
```

This `PARTIAL` did not block: the run reached the `PASSED` stage, `LOCAL_DONE`,
and final gate `PASS`.

## 6. Defect found and fixed during this step

The one-task demo printed **`2 completed this run`**. Root cause: the planner
resume completed the run stage and the continuation but never finalized the
**task** (unlike the normal lifecycle path), so the scheduler finished the still
runnable task in a second pass and counted it again.

Fix (uncommitted, on this branch):

- `internal/cli/planner.go` — `plannerResume` now calls `completeTask` once the
  resumed work durably succeeds (both the `PASSED` path and the effect-marker
  recovery path).
- `internal/cli/drive.go` — `completeTask` promotes a not-yet-promoted
  (`PLANNED`) task to `READY` first, so completing it is legal for a direct
  caller that bypassed the scheduler.
- Regression test `TestPlannerResumeCountsOneCompletion`
  (`internal/cli/planner_integration_test.go`). Proven to fail before the fix
  (`all tasks done (2 completed this run)`) and pass after.

## Remaining limits

- Guarantees are not general exactly-once; an uncertain continuation stops for
  human review (see the implementation proposal, §13).
- The live demo uses a deterministic controlled executor, not a general model.
- Continuation execution is local (repository work only); no external side
  effects.
