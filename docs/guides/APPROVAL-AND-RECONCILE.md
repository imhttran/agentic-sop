# Human Approval and Reconcile

**Type:** Guide

When a human gate appears in the controller, what you are asked to decide, and
how approval and reconcile actions delegate to SOP. The controller shows the
gate and forwards your request; **SOP decides everything else**. Normative rules
are in [specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md).

## The Two Halves of a Human Gate

```text
controller shows the gate   (observation)
        |
        v
operator requests an action (delegated to SOP)
        |
        v
SOP decides the transition   (policy)
```

The controller **decides nothing**. It never decides that approval is required,
never fabricates or bypasses an approval, and never writes local task state.
Every action it offers is a request handed to SOP's own operation.

## When a Human Gate Appears

The controller presents a human gate **only when SOP reports one** in its
authoritative approval listing (`sop approvals --json`, decoded verbatim). SOP's structured `status`/`disposition` on a
listing entry is the sole source of truth for gate presence and applicability.

`BLOCKED` alone, a `NEEDS_HUMAN` classification, a `WAITING_FOR_HUMAN` run
stage, model prose, attempt counts, and inactivity **never** manufacture a gate.
When SOP reports no applicable gate, the controller shows an explicit absence and
offers no control.

See [../reference/STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md)
for the "Needs Human" callout rule and the surrounding statuses.

## What You Are Asked to Decide

When a gate appears, the controller shows the SOP-reported boundary and an
approve / decline control. You are deciding whether to let SOP proceed past the
gate; the resulting transition is still SOP's to make.

- **Approve** — delegates to `sop approve <task-id>` (route
  `POST /projects/{project}/tasks/{task}/commands/approve`).
- **Decline** — delegates to `sop decline <task-id>` (route
  `POST /projects/{project}/tasks/{task}/commands/decline`).
- **Commit gate** — when SOP's `human.approval_before_commit` is on, `sop run`
  stops at the human gate and never commits; the controller reports that state
  rather than proceeding.

Approve and decline are **two independent SOP operations**. The controller
offers a control only when SOP both reports the boundary _and_ exposes the
operation. When SOP reports a boundary but exposes no operation for an action,
the controller shows the gate **read-only**, advertises no control that cannot
succeed, and returns an explicit conflict if the action is invoked.

## Delegating a Decision

Both actions delegate through the controller's safe command boundary, using an
argv slice (never a shell string):

```text
sop approve <task-id> [--by NAME] [--note TEXT]
sop decline <task-id> [--by NAME] [--note TEXT]
```

The task id, actor (`--by`), and note (`--note`) are passed as discrete
arguments, so an untrusted id, actor, or note can never be shell-interpreted. No
`--run` is passed: recording a decision is **separate from execution**, and
approving does not start a run. Continuing is a distinct, explicit choice.

SOP validates the gate **at command time**. The controller does not pre-judge
staleness from its own read of the listing; it delegates the decision and
reports SOP's answer. When SOP rejects a decision (for example a stale or
no-longer-applicable gate), the controller surfaces it as an **actionable
conflict** carrying SOP's own message and the affected task id — never a `500`
and never a fabricated success.

The controller writes no approval state of its own: approving never marks a task
complete in controller state, declining never manufactures a failure, and there
is **no controller-side approval persistence**.

## Reconciliation Approval

**Reconcile** is offered when SOP has recorded an active plan in
`.agent-sdlc/plan.meta.json`. The dashboard passes that recorded path to SOP:

```text
sop reconcile <PLAN.md>
```

and lets SOP decide. SOP preserves every unchanged task and stops at the human
boundary when an executed task's definition changed.

- The changed-executed-task set is read from SOP's **authoritative listing**,
  `sop reconcile <PLAN.md> --list-changed --json`, resolved from the plan source
  SOP recorded in `.agent-sdlc/plan.meta.json`. The retired
  `.agent-sdlc/reconcile.json` artifact is never read (C2-003). The listing is a
  **pure read**: it mutates no task state, graph, active plan, acceptance, or
  provenance.
- When SOP reports changed executed tasks awaiting reconciliation, the controller
  obtains approval **only for tasks SOP reported**; it never approves a task SOP
  did not flag. Display title/stage are enriched from SOP's task store, never
  computed from a controller-side plan diff.
- When SOP reports **no** changed-executed-task listing (the verb is unsupported,
  exits non-zero, or emits unparsable output), the controller reports the
  absence/failure explicitly rather than showing an empty success.
- Accepting a changed executed task is a distinct operation
  (`sop reconcile <PLAN.md> --accept-changed <id>`), supported at the controller
  boundary: the controller delegates the decision to SOP and never simulates or
  bulk-accepts. Boundary support describes controller delegation; external-binary
  flag support was unverified in the recorded C2-009 sandbox run.

## The Cross-Project Approvals Inbox

`/approvals` lists every applicable gate SOP reports across all observed
projects, so a phone can review and decide without visiting each project. It is
the same read as the per-project decisions panel — `sop approvals --json` once
per project — and the same delegated routes; the page adds no new decision logic
and starts no command on `GET`.

- A project whose `sop approvals --json` cannot be read is shown as an explicit
  absence ("Could not be read"), never as "no gates".
- Each gate links to its task and offers the same approve / decline controls.
- It is reachable over the network only with an access token (network mode); see
  [../specs/SECURITY.md](../specs/SECURITY.md) and
  [GETTING-STARTED.md](GETTING-STARTED.md#phone--same-network-access).

## Related Documentation

- [specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) — normative approval
  boundary.
- [../reference/STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md) —
  when "Needs Human" is shown and the recovery actions.
- [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md) — the
  operation boundary (approve/decline supported; cancel still a gap).
- [../reference/CLI.md](../reference/CLI.md) — approve/decline and reconcile
  routes.
