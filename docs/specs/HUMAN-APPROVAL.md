# Human Approval

**Type:** Normative specification

## Purpose

This document is the normative specification for SOP's explicit human boundaries:
the human gate that SOP owns, the approval requirement for Git workflow commands,
the prohibition on `sop run` committing, pushing, or merging, and how a
`needs_human` boundary is distinguished from a hard `FAIL`. The gate outcome
semantics are owned by [QUALITY.md](QUALITY.md); requeue and retry accounting are
owned by [RECOVERY.md](RECOVERY.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Commit Gate and
  Merge Gate, §14 Security and Permissions.
- [../PRD.md](../PRD.md) — FR-18 Blocked state, §13 Default Guardrails, §15 Out of
  Scope.
- [QUALITY.md](QUALITY.md) — `PASS`/`FAIL`/`NEEDS_HUMAN` and the fix-loop budget.
- [RECOVERY.md](RECOVERY.md) — requeue and the `needs_human`/`FAIL` distinction.
- [WORKFLOW.md](WORKFLOW.md) — the state graph and terminal states.
- [TASK-LIFECYCLE.md](TASK-LIFECYCLE.md) — the commit and pull-request stages.
- [../reference/CLI.md](../reference/CLI.md) — `sop run`, `sop commit`, `sop pr`, `--yes`.
- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) —
  `human.approval_before_commit`.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. The Human Gate Is Owned by SOP

- The human gate MUST be a SOP lifecycle decision. A client (the CLI, a controller,
  or MCP) MUST NOT manufacture approval and MUST NOT infer it from a task status.
- SOP MUST own the approval requirement, and no consumer MAY bypass it. The
  permission principles are stated in
  [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) §14.

## 2. Approval Before Commit

- `human.approval_before_commit` (default **true**) controls whether an explicit
  human approval is required for the Git workflow commands.
- When it is on, `sop commit` and `sop pr` MUST require `--yes`; without it they
  MUST stop and request approval rather than acting.
- `sop commit` MUST commit and MUST NOT push. `sop pr` MUST push the task branch
  and open or update the pull request, and only after approval. Neither command
  MUST merge and neither MUST force-push.

## 3. `sop run` Stops at the Gate

- `sop run` MUST stop at the human gate.
- `sop run` MUST NOT commit, MUST NOT push, and MUST NOT merge.
- The remote path (push → PR → CI → merge) MUST be driven by the explicit, gated
  commands and the GitHub adapter; a local run MUST NOT synthesize it.

## 4. `needs_human` Versus `FAIL`

- A `needs_human` outcome is a boundary, not a failure. It MUST be **non-terminal**:
  SOP requeues the task so a later run retries it (see [RECOVERY.md](RECOVERY.md)).
- A hard `FAIL` — including a claimed change with none produced — MUST leave the
  task `BLOCKED` and MUST NOT be requeued automatically.
- A `WAITING_FOR_HUMAN` run stage MUST persist a human gate across invocations: a
  task parked at that stage MUST stay at the gate on re-entry, independent of the
  agent's prose.
- Approval MUST be driven by structured state (an active approval request or the
  `WAITING_FOR_HUMAN` stage), never inferred from free-form prose.

## 5. Non-Negotiable

- SOP MUST own approval requirements and MUST NOT provide a path that bypasses them.
- Exhausting the fix or retry budget MUST yield `NEEDS_HUMAN`, never an unbounded
  loop (see [QUALITY.md](QUALITY.md) and [RECOVERY.md](RECOVERY.md)).

## 6. The Approval Surface Is Read-Only Until a Human Decides

- SOP MUST expose the gate as data: the present-or-absent request and its fields for
  one task, and the set of tasks at an applicable gate.
- Enumerating the gates MUST NOT create, refresh, or resolve a request. It MUST NOT
  infer a gate from a task status, a blocked reason, attempt counts, or prose — a task
  with no recorded request MUST NOT be listed.
- A request MUST be reported applicable only while it is `PENDING` **and** its task can
  still act on it. A request whose task has already completed is stale: it MUST be
  reported as not applicable, exactly as a decision on it is refused.
- A decision MUST go through SOP's single approval application boundary (the same one
  the CLI, MCP, and a controller use). A client MUST NOT write an approval decision of
  its own, and MUST NOT infer an approval from a task status.
- Reading a gate or listing the gates MUST NOT mutate the repository, the task graph,
  or run history.
