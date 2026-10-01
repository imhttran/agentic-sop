# Execution

**Type:** Normative specification

## Purpose

This document is the normative specification for `sop run`, SOP's execution
entrypoint: planning-source discovery, idempotent preparation, machine-plan
compilation, change detection, run artifacts, the bounded fix loop, verify-first
execution, controlled parallelism, and the human gate. The task state graph and
remediation rules are owned by [WORKFLOW.md](WORKFLOW.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Plan Preparation,
  Task Runner, Execution Modes, Fix Loop, Graph Execution; §11 Parallelism.
- [../PRD.md](../PRD.md) — §5, §7 (FR-3, FR-4, FR-10, FR-22), §9 Idempotency.
- [WORKFLOW.md](WORKFLOW.md) — state vocabulary and remediation;
  [TASK-LIFECYCLE.md](TASK-LIFECYCLE.md) — per-task stages and Git conventions.
- [VALIDATION.md](VALIDATION.md) — configured checks; [QUALITY.md](QUALITY.md) — the
  gate and fix-loop budget; [RECOVERY.md](RECOVERY.md) — retries, resume, durable state.
- [../reference/CLI.md](../reference/CLI.md), [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md),
  [../reference/PERFORMANCE.md](../reference/PERFORMANCE.md) — command reference;
  [../README.md](../README.md) indexes all docs.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Planning-Source Discovery and Precedence

When no plan is named, `sop run` MUST select its planning source in this order: an
existing valid `.agent-sdlc/plan.json` (reused **only** if its source is unchanged),
then `docs/PLAN.md`, `PLAN.md`, `docs/PRD.md`, and finally `PRD.md`.

A human `PLAN.md` MUST be preferred over the PRD, and `plan.json` MUST NOT silently
override a newer PLAN: the source path and a content fingerprint are recorded, and a
changed document MUST trigger a rebuild. A file argument names an execution PLAN
authoritative for that run (`sop run docs/PLAN-Jev.md`); `--task TASK.md` runs one task.

## 2. Idempotent Preparation

`sop run` MUST discover the source, initialize SOP state if needed, compile or
generate the machine plan, create the tasks, and then drive the graph — each step
idempotent. Running it again MUST resume, and completed work MUST NOT be restarted.
Preparation MUST NOT require an agent: a `PLAN.md` is compiled and tasks created
even with no agent configured, and the missing agent MUST be reported only when
execution (or PRD-based plan generation) actually needs it.

## 3. Machine Plan Compilation

`sop run` MUST compile or generate `.agent-sdlc/plan.json` and build the task DAG
deterministically, without an agent; a compiled stage MUST carry its optional
`execution_mode` onto the task. Machine state and run reports MUST live under
`.agent-sdlc/` (self-ignoring for Git), and a PRD-generated plan MUST be written to
`docs/reports/<plan-id>.md` (`sop init`/`sop plan`/`sop tasks` stay lower-level).

## 4. Change Detection

Git MUST be the authority on what changed. Change detection MUST include untracked
files, so a file the agent creates counts even when no tracked file changed; SOP's
own `.agent-sdlc/` output MUST be excluded. Validation MUST fail fast before review.
A run MUST reject a provider that cannot `IMPLEMENT` before running any task.

## 5. Run Artifacts

Every stage SHOULD write an artifact under `.agent-sdlc/runs/<id>/` (`task.md`,
`plan.md`, `implementation.md`, `diff.patch`, `fix-N.md`, `validation.json`,
`review.json`, `report.md`, `report.json`, `metrics.json`, `state.json`); a run that
terminates MUST stay inspectable. Each task SHOULD end with a concise performance
line; see [../reference/PERFORMANCE.md](../reference/PERFORMANCE.md).

When model routing applies, `routing.json` records the routing decision, and when
bounded escalation applies, `attempts/NNN.json` records one execution attempt per
try. Both are non-secret diagnostic evidence: neither is read back to drive a
decision, and neither is a second source of task state.

## 6. Verify-First Execution Mode

`execution_mode: verify-first` MUST be explicit plan/task metadata — never inferred
from a task's title or prose. It runs the configured validation **before** any agent:

- a pass MUST invoke no agent (not the micro-planner, `IMPLEMENT`, or `FIX`) and completes locally only because its configured checks passed;
- a failure MUST hand the deterministic failure to the implementation agent as context; the ordinary lifecycle then continues (not automatically terminal);
- a verify-first task with **no** configured check MUST fall back to the ordinary implementation path — a fast path that verifies nothing is not a verification.

Without the mode a task keeps implement-first behaviour. See [VALIDATION.md](VALIDATION.md).

## 7. Fix Loop Bound

When validation fails, or review leaves blocking findings, `sop run` MUST send the
failure back to the agent with the plan, the deterministic failure (when a check
failed), the findings, and the current diff, then re-run validation and review. The
loop MUST be bounded by `quality.max_fix_cycles`; exhausting the budget MUST yield
`NEEDS_HUMAN` rather than looping forever. See [QUALITY.md](QUALITY.md), [RECOVERY.md](RECOVERY.md).

## 7a. Bounded Model Escalation

When `models.escalation_enabled` (or `SOP_MODEL_ESCALATION_ENABLED`) is on, a task
that fails a quality gate MAY be retried once more on the next larger model class in
the same invocation, before the existing human boundary applies. The escalation
happens outside the lifecycle: each retry is a fresh, bounded lifecycle on the
escalated class, with the previous attempt's bounded context handed forward, and
with the escalated selection resolved, validated, built, and capability-guarded
first. Escalation MUST NOT change state transitions, gates, approval, or autonomy
policy, MUST NOT wrap past `large`, and MUST be bounded by
`models.max_escalations`. It is OFF by default. See [RECOVERY.md](RECOVERY.md) §8 and
[MODEL-ROUTING.md](MODEL-ROUTING.md).

## 8. Controlled Parallelism

Only dependency-independent tasks MAY run concurrently, each in its own Git
worktree so no two tasks share a working directory. Parallelism MUST remain bounded
rather than allowing unrestricted agent spawning, and concurrency MUST be a
conservative, configurable limit (V1 SHOULD default to no more than two active
implementation tasks). **Partial:** the worktree-based runner (`internal/parallel`)
exists and is exercised by the e2e dogfood test, but `sop run` itself drives the
graph one task at a time and `max_parallel_tasks` is not yet a configuration key;
wiring the bounded runner to `sop run` remains future work.

## 9. Human Gate — No Remote Writes

`sop run` MUST stop at the human gate and MUST NOT commit, push, or merge. Local
execution has no remote PR/CI/merge, so a passing lifecycle completes the task at
`LOCAL_DONE` directly and MUST NOT fabricate remote states. The remote path is driven
by the explicit `sop commit`/`sop pr` commands.
