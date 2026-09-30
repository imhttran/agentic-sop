# Recovery

**Type:** Normative specification

## Purpose

This document is the normative specification for bounded retries and durable
recovery: the attempt budget, progress-based accounting, requeueing, resume
semantics, SQLite as the durable source of truth, restart reconciliation, and plan
provenance, handoff, and archive. The state graph and remediation rules are owned by
[WORKFLOW.md](WORKFLOW.md); how a run executes is owned by [EXECUTION.md](EXECUTION.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §5 Task State
  Machine, §6 Retry Architecture, §12 Data Model.
- [../PRD.md](../PRD.md) — FR-17 Bounded retry, FR-18 Blocked state, FR-23 Resume,
  §9 Recoverability and Idempotency, §12 Sources of Truth.
- [WORKFLOW.md](WORKFLOW.md) — state vocabulary and terminal states.
- [EXECUTION.md](EXECUTION.md) — `sop run` preparation and lifecycle.
- [QUALITY.md](QUALITY.md) — the gate and fix-loop budget.
- [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md) — the human boundary.
- [../reference/STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md) and
  [../reference/CLI.md](../reference/CLI.md) — command reference.
- [../README.md](../README.md) — documentation index.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Bounded Retries and the Attempt Budget

- Every retry loop MUST be bounded by `max_attempts`, whose default MUST be **3**.
- A `needs_human` outcome MUST NOT be terminal: SOP requeues the task to `PLANNED`
  so the same `sop run …` retries it automatically.
- A retry that changes its outcome (makes progress) MUST spend one attempt; once
  the budget is spent the task MUST become `BLOCKED` instead of looping.
- A retry that reproduces the **same** outcome is "no progress" and MUST spend
  nothing: the task stays `PLANNED` and a later run still retries it, so a boundary
  resolved later is picked up without a manual unlock. It is reported as
  `NEEDS_HUMAN (no change since the previous attempt: …)`.
- A hard `FAIL` — including a claimed change with none produced — MUST leave the
  task `BLOCKED`.

## 2. Explicit Requeue

- `sop retry <id>` MUST requeue a `BLOCKED` task.
- `sop retry --all` MUST requeue every `BLOCKED` task that still has budget.
- `sop retry <id> --force` MUST be available for a task whose budget is spent; it
  raises `max_attempts` rather than requiring `state.db` surgery or a fresh start.
- A retried task SHOULD be handed the previous attempt's outcome as context (a
  `# Previous attempt` section in the implement request), so the agent can address
  the blocker instead of repeating the request that stopped it.

## 3. Attempt Records

Each attempt SHOULD record its operation, start/end, result, failure summary,
logs/artifacts where practical, agent action, and next state; see
[WORKFLOW.md](WORKFLOW.md) §2 and [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) §12.

## 4. Resume Semantics

- `sop run` MUST resume an in-flight (interrupted) task from the same persisted
  state → next-action rule that `sop resume` reports (`resume.ActionFor`), instead
  of stopping with `ACTIVE_TASK`.
- A task whose gates already passed MUST be completed locally without re-invoking
  the agent.
- `ACTIVE_TASK` MUST still prevent starting a _different_ task; zero or multiple
  in-flight tasks MUST be reported as an actionable error naming the task and the
  recovery command, never guessed.

## 5. Durable Source of Truth

- SQLite (`.agent-sdlc/state.db`) MUST be the durable source of truth for task
  execution state — the database, **not** model conversation history.
- Markdown MUST NOT be the only source of runtime state. Runs are stored on the
  filesystem under `.agent-sdlc/runs/<id>/` so they stay readable without the tool.
- The schema MUST be versioned (`PRAGMA user_version`) so migrations add columns without losing data.

## 6. Restart and Reconciliation

- After a process interruption SOP MUST be able to restart and determine where it
  left off from persisted state.
- `sop resume` MUST reconcile a task's persisted status with the resources that
  actually exist and report the next legal action, recovering a lost state write
  without duplicating a branch or pull request. Idempotent resume MUST NOT create
  duplicate branches, pull requests, commits, or task records.

## 7. Plan Provenance, Handoff, and Archive

- Each plan MUST carry its own identity (source path, `source_sha256`, and a plan
  id), so SOP never mixes tasks from two plans.
- When a different plan is requested while another is active, SOP MUST hand off
  automatically once every task in the active plan is complete, archiving the
  completed plan's records under `.agent-sdlc/archive/<plan-id>/`; if the active
  plan still has unresolved work, SOP MUST stop with an actionable `NEEDS_HUMAN`
  instead of discarding history.
- If the active plan's own PLAN file changed after its task graph was created,
  `sop run` MUST stop with `NEEDS_HUMAN: plan changed …` rather than silently
  rebuilding the graph. `sop reconcile docs/PLAN.md` MUST apply the change: it
  preserves every unchanged task and its history, updates only tasks that have
  never executed, adds and removes unexecuted tasks, and stops with `NEEDS_HUMAN`
  (naming the task and the difference) when an executed task's definition changed
  or was removed. It MUST NOT delete `.agent-sdlc/state.db` or discard run history.
- `sop reconcile docs/PLAN.md --accept-changed <id>` MUST replace a changed executed
  task's definition while preserving its lifecycle state, attempts, and history, and
  the approval MUST be recorded in `plan.meta.json`. The flag MUST be repeatable, and
  every changed executed task MUST be named, so one approval never silently covers another.
