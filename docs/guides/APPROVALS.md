# Approvals

A task walks through SOP's lifecycle, and some outcomes are not SOP's to decide.
When a task reaches a human boundary, SOP parks it, records an explicit, resolvable
approval request, and stops. This guide walks through seeing such a gate, reading
it, deciding it, and continuing. It is a task-oriented walkthrough; the normative
rules live in [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md).

The mental model:

```text
sop run
   ↓
human decision required            SOP records a request and stops
   ↓
sop approvals                      see what is waiting on you
   ↓
sop approval <task-id>             read the gate
   ↓
sop approve <task-id>              record the human decision
sop decline <task-id>
   ↓
sop run                            the ordinary run continues from where it stopped
```

Two things do not happen automatically: SOP never approves on your behalf, and
approving never starts the run. Recording the decision and continuing are separate
steps, deliberately.

## Seeing a gate

A run that parks a task prints the gate and the commands that resolve it:

```text
Run paused: human approval required

Task: P6-005
Stage: WAITING_FOR_HUMAN
Reason: the operation is destructive and cannot be undone

Inspect:
  sop approval P6-005

Approve:
  sop approve P6-005

Decline:
  sop decline P6-005

After approving, explicitly continue with:
  sop run
```

The run also lists any gates it left behind at the end, pointing at `sop approvals`.
To list every gate waiting on you at any time:

```bash
sop approvals
```

That prints one line per applicable gate (`<task-id>  <kind>  <stage>  <reason>`), or
`no pending approvals`. It is read-only: it creates nothing, resolves nothing, and
never infers a gate from a task status, a blocked reason, or prose — a task with no
recorded request is not a gate.

For a script or a controller, prefer the machine-readable form:

```bash
sop approvals --json
```

## Reading a gate

To see SOP's authoritative view of one task:

```bash
sop approval P6-005
```

This prints the present-or-absent boundary and its fields: kind, target, stage,
disposition, reason, evidence, status, and any recorded decision. `Approval: none`
means SOP recorded no request — a task status alone is never treated as a gate.

## Deciding a gate

Record a decision explicitly:

```bash
sop approve P6-005 --by reviewer --note "checked the destructive change"
sop decline P6-005 --by reviewer
```

`--by` and `--note` are optional provenance, not authentication.

What each decision means:

- **Approve** authorizes only the operation the gate represents. It does not mean the
  task is complete, that validation, review, JEV, or the quality gate passed, or that
  a commit, push, or merge is allowed. Every subsequent lifecycle check still applies.
  A blocked task is returned to runnable work without spending a retry.
- **Decline** records the decision and leaves the task in the lifecycle state the
  domain dictates. It never manufactures completion.

Both commands are idempotent: repeating the same decision succeeds without changing
anything, and deciding the other way is refused.

## Deciding interactively

At a terminal, you can let SOP ask you which gate to decide, instead of naming one:

```bash
sop approve           # or: sop approve --select
sop decline --select
```

With one applicable gate it asks for a confirmation; with several it lists them and
asks you to pick one (by number or task id), then confirm. What you type is a
decision relayed to SOP, never a command.

Interactivity fails closed. When stdin is not a terminal — a pipe, a redirect, CI, or
an automation client — the command prints the usage and the gates, records nothing,
and exits non-zero. It never assumes yes, never picks a default, and never reads
end-of-input as approval. Use the explicit `sop approve <task-id>` form there.

## Continuing after an approval

Recording a decision does not run anything. Continue explicitly:

```bash
sop approve P6-005
sop run
```

If you want the run to start as soon as the decision is recorded, `--run` does that:

```bash
sop approve P6-005 --run
```

The decision is persisted first, so a failure to start the run leaves the approval
recorded and the task runnable. The run that follows is the ordinary run, and it
still stops at its own next gate. `--run` exists only on `approve`; a decline never
continues.

## Controller and MCP usage

A controller (such as `sop-controller`) and the MCP tools are clients of the same
application boundary these commands use. Discover pending gates with
`sop approvals --json`, inspect one with `sop approval <task-id>`, and resolve it with
`sop approve`/`sop decline` (or the MCP `sop_approve`/`sop_decline` tools). No
SOP policy — routing, providers, approval outcome — moves into the client.

Reconciliation has its own read-before-write path: `sop reconcile <PLAN.md>
--list-changed --json` names the executed tasks a reconciliation would change before
anything mutates. See [../specs/RECOVERY.md](../specs/RECOVERY.md) §7.

## See also

- [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) — the normative rules.
- [../reference/CLI.md](../reference/CLI.md) — the exact command flags.
- [../reference/STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md) —
  `sop status`, `resume`, and `retry`.
- [../specs/RECOVERY.md](../specs/RECOVERY.md) — requeue and the `--list-changed`
  listing.
