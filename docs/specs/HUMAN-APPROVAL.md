# Human Approval Specification

**Type:** Normative specification

## Purpose

Defines the human-approval boundary: when the controller shows a human gate, how
approval actions delegate to SOP, and what the controller must never do.

## Related Specifications

- [WORKFLOW.md](WORKFLOW.md)
- [EXECUTION.md](EXECUTION.md)
- [REVIEW.md](REVIEW.md)
- [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md)

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

## Ownership

- Human approval boundaries MUST remain controlled by SOP.
- The controller MUST NOT decide that approval is required and MUST NOT
  fabricate, simulate, or bypass an approval.
- The controller MUST NOT take any action that bypasses SOP's validation,
  review, OpenJEV, or quality gates.
- The controller presents and delegates; SOP owns read and mutation authority.
  The controller determines how a decision is **presented**; SOP determines
  whether the decision is **applicable** and how it changes lifecycle state
  (see [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md)).

## When a Human Gate Is Reported

The controller MUST present a human gate only when SOP reports one, from
SOP-persisted evidence:

- SOP's run classification disposition is `NEEDS_HUMAN`, or
- SOP's run classification kind is a human kind, or
- SOP's run stage is `WAITING_FOR_HUMAN`, or
- the task status is `BLOCKED` **and** SOP recorded a human classification.

A `BLOCKED` task whose blocking reason is not a human reason (for example a
dependency still running, or an environment/tooling problem) MUST NOT be treated
as a human gate.

## Two Distinct Human-Decision Domains

Approval and reconciliation are **distinct human-decision domains**. They have
distinct SOP-owned evidence and distinct delegated operations, and neither
decision satisfies the other:

| Domain                                                  | SOP-owned evidence                                                                                                                  | Delegated operation                                  |
| ------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| **Approval** — the task-level human gate                | SOP-reported human classification / gate and stage (`NEEDS_HUMAN`, human classification kind, `WAITING_FOR_HUMAN`, human-`BLOCKED`) | `sop approve` / `sop decline`                        |
| **Reconciliation** — the changed-executed-task decision | `sop reconcile <PLAN.md> --list-changed --json`                                                                                     | `sop reconcile <PLAN.md> --accept-changed <TASK_ID>` |

- An approval decision MUST NOT be treated as satisfying a reconciliation
  decision, and a reconciliation decision MUST NOT be treated as satisfying an
  approval decision.
- Reconciliation evidence is SOP's authoritative changed-executed-task listing
  (`sop reconcile <PLAN.md> --list-changed --json`); the retired
  `.agent-sdlc/reconcile.json` artifact MUST NOT be read.

## Approval Actions

- Approve and decline are two independent SOP operations. Each control MUST be
  offered only when SOP both reports the boundary and exposes that operation.
- When SOP reports a boundary but exposes no operation for an action, the
  controller MUST show the gate read-only, MUST NOT advertise a control that
  cannot succeed, and MUST return an explicit conflict if the action is invoked.
- Every approval action MUST delegate to SOP's own application operation and
  MUST report SOP's answer verbatim; the controller MUST NOT write local task
  state.
- Human approval is a **supported** delegated operation: the controller
  delegates to `sop approve <task-id> [--by NAME] [--note TEXT]` and
  `sop decline <task-id> [--by NAME] [--note TEXT]` with no `--run`. A rejected
  (stale/not-applicable) decision MUST be surfaced as an explicit conflict, not
  a `500` and not a fabricated success.
- Actor attribution MUST NOT treat a browser-editable field as a verified identity. When
  `SOP_CONTROLLER_OPERATOR` is configured, the controller MUST record that actor for
  approve/decline and ignore the form's actor field (the forms render it read-only);
  otherwise the browser-provided actor is recorded as an unverified label. The decision
  note MUST always be taken from the request and forwarded unchanged.

## Commit Gate

- `human.approval_before_commit` (SOP configuration) controls whether a human
  must approve before committing. When it is on, `sop run` stops at the human
  gate and never commits; the controller MUST report that state rather than
  proceeding.

## Reconciliation Approval

- When SOP reports changed executed tasks awaiting reconciliation, the
  controller MUST obtain approval only for tasks SOP reported, and MUST NOT
  approve a task SOP did not flag.
- When SOP reports no changed-executed-task set, the controller MUST report the
  absence rather than showing an empty success.
- Reconciliation is its own decision domain (see above); the controller MUST
  present it as a changed-executed-task decision, not as a task approval gate,
  and MUST delegate it to SOP.
- `AcceptChangedTask` is supported at the controller boundary and MUST delegate
  to `sop reconcile <PLAN.md> --accept-changed <TASK_ID>`. Boundary support
  describes controller delegation; it MUST NOT be treated as verification that
  an external SOP binary supports the flags. In the recorded
  [C2-009 sandbox run](../history/C2-009-REPORT.md), external reconciliation-flag
  support was **UNVERIFIED** and real-binary scenarios were **NOT EXERCISED**.

## Related Documentation

- [../reference/STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md) —
  when "Needs Human" is shown and the recovery actions.
- [../reference/CLI.md](../reference/CLI.md) — approve/decline and retry routes.
- [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md) — the
  supported approval/reconciliation operations, unsupported cancellation, and
  the presentation-vs-applicability split.
