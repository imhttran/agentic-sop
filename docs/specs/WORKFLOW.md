# Workflow

**Type:** Normative specification

## Purpose

This document is the normative specification for SOP's workflow: state
vocabulary, transition legality, `FIX_REQUIRED` remediation, terminal states,
bounded autonomy, deterministic scheduling, and handoff between plans. The
authoritative graph is in [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md)
§5 and §6; the happy-path listing is in [../PRD.md](../PRD.md) §8.

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §5 Task State
  Machine (authoritative transition graph) and §6 Retry Architecture.
- [../PRD.md](../PRD.md) — §8 Task Lifecycle plus the scheduling, retry, branch,
  and merge requirements.
- [TASK-LIFECYCLE.md](TASK-LIFECYCLE.md) — per-task lifecycle stages, TDD rules,
  and Git conventions.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Task State Vocabulary

A task MUST be in exactly one of these named states at a time: `PLANNED`,
`READY`, `BRANCH_CREATED`, `TESTS_WRITTEN`, `RED_VERIFIED`, `IMPLEMENTING`,
`LOCAL_TESTS_PASS`, `REVIEW`, `REVIEW_PASS`, `PR_OPEN`, `CI_RUNNING`, `CI_PASS`,
`MERGED`, `DONE`. Local completion adds `LOCAL_DONE`; failure and recovery add
`FIX_REQUIRED`, `RETRY_WAIT`, and `BLOCKED`. `NEEDS_HUMAN` is a human boundary,
not a happy-path task state.

The authoritative legal-transition graph and its exact edges are defined in
[../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) §5; this vocabulary
MUST NOT be read as a substitute for it.

## 2. Transition Legality

- The orchestrator MUST decide the workflow state and what may happen next. An
  agent or LLM MAY propose content, but MUST NOT decide whether its own output is
  valid.
- A task MUST move only along transitions defined by the authoritative state
  machine; an illegal transition (for example, `PLANNED → MERGED`) MUST be
  rejected.
- Every attempt SHOULD record its operation, start/end, result, failure summary, agent action, and next state.

## 3. Remediation (`FIX_REQUIRED`)

- An actionable failure MAY move a task into `FIX_REQUIRED`; sources include
  `IMPLEMENTING`, `LOCAL_TESTS_PASS`, `REVIEW`, and `CI_RUNNING`.
- From `FIX_REQUIRED` the task returns to implementation and re-runs validation
  and review.
- Remediation loops MUST be bounded by a configurable maximum attempt count
  and/or wall-clock budget.

## 4. Terminal States

- `BLOCKED` and `DONE` are terminal; `LOCAL_DONE` is terminal for a task whose
  local lifecycle passed.
- A task MUST reach `BLOCKED`, with a recorded reason, when retry limits are
  exhausted or a failure requires human input.
- `NEEDS_HUMAN` is not terminal: the task is requeued to `PLANNED` for a later
  run.
- A local run MUST NOT fabricate remote states (`PR_OPEN`, `CI_RUNNING`,
  `CI_PASS`, `MERGED`); a dependency is satisfied by `LOCAL_DONE` (local) or
  `MERGED`/`DONE` (remote).

## 5. Bounded Autonomy

- SOP MUST NOT behave as an unrestricted autonomous coding agent; the
  orchestrator defines boundaries and validation precedes any state change.
- Every retry loop MUST have a maximum attempt count and/or time budget.
- Exhausting a budget MUST stop the task rather than loop indefinitely, yielding
  `BLOCKED` (or a bounded requeue in graph execution).
- Concurrency MUST be bounded; V1 SHOULD default to no more than two active
  implementation tasks.

## 6. Deterministic Scheduling

- The scheduler MUST select only tasks whose dependencies are satisfied and MUST
  be deterministic: it MUST NOT consult a model.
- It MUST select at most one legally runnable task; V1 runs one task at a time
  and bounds parallelism separately.
- When a dependency completes (`LOCAL_DONE`, `MERGED`, or `DONE`), dependents
  whose dependencies are then all satisfied MAY become `READY`; dependents with
  unmet dependencies MUST remain `PLANNED`.
- When a plan marks an environment stage, feature tasks MUST implicitly depend on it and stay blocked until bootstrap completes.

## 7. Handoff Between Plans

- On completion, SOP SHOULD record a small, deterministic capsule (handoff)
  describing the completed task, with optional provider-independent compression
  of bulky artifacts.
- Authoritative truth MUST remain the state store, Git, verification results, and
  PR/CI state.
- Handoffs MUST be best-effort and MUST NOT change task state.
