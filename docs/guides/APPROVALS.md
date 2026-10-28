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

## Invalidating a gate whose evidence is obsolete

A plan reconciliation can change a task's definition while a request raised for the
old definition is still pending. The request is then undecidable in substance: the
evidence it names no longer exists. When the task is already satisfied, use
`sop approval supersede`. When it is not satisfied, use:

```bash
sop approval invalidate P6-005 --reason "evidence names a deliverable the revised plan removed" --by reviewer
```

### Eligibility

Every rule is evaluated in order, and a refusal writes nothing:

1. `--reason` and `--by` are mandatory. An invalidation is an audited operator
   action, never an implicit one.
2. The request must be `PENDING`. An approved, declined, superseded, or already
   invalidated request is refused, so a recorded human decision is never overwritten.
3. The task must not be satisfied. A satisfied task is refused and points at
   `supersede`.
4. SOP must hold a **task-scoped accepted-change record** for this task, and that
   record must:
   - name this task (another task's reconciliation never qualifies),
   - carry an acceptance time strictly **after** the request was raised,
   - cover the **current** definition digest, and
   - describe a real change (its before and after digests must differ).

   This is required for **every** request, including one raised by this version that
   recorded its definition digest. A different digest alone is not enough: the
   change must have been accepted, and SOP records that acceptance per task when an
   operator reconciles with `--accept-changed`.

   Two things that look like evidence and are not:
   - `plan.meta.json`'s `generated_at` is a plan-wide write time, refreshed by any
     later plan write — including a no-op one. It never proves when a task's change
     was accepted, and is not used.
   - `reconciled_tasks` is a legacy task list. It records that a change was accepted
     at some point, with no time, so it cannot establish the ordering. A plan whose
     provenance has only that list has no usable acceptance evidence.

   When a digest was recorded on the request, it must additionally differ from the
   current one; a still-applicable request is refused even if an acceptance exists.

A provider failure, a blocked task, a repaired implementation, or an edited plan file
on disk is **not** evidence on its own. A provider failure says nothing about the
task definition, so it never qualifies; the request must then be decided. Missing or
ambiguous acceptance evidence is refused rather than backfilled.

### Refusal cases

| Situation | Result |
| --- | --- |
| Missing `--reason` or `--by` | Usage error; nothing is read or written |
| Request already `APPROVED`, `DECLINED`, `SUPERSEDED`, or `INVALIDATED` | Refused: a recorded decision is never overwritten |
| Task satisfied (`LOCAL_DONE`, `MERGED`, `DONE`) | Refused, pointing at `sop approval supersede` |
| Current definition digest equals the recorded one | Refused: the evidence is still applicable |
| No task-scoped accepted-change record for the task | Refused: no accepted definition change to prove |
| Acceptance recorded before the request (even if the plan file was rewritten afterwards) | Refused: a later plan write is not acceptance evidence |
| Acceptance belongs to another task | Refused: acceptance is task-scoped |
| Acceptance covers a different or older definition | Refused: the change it accepted is not this one |
| Acceptance has no time, or the plan has only a legacy `reconciled_tasks` entry | Refused: ambiguous evidence is never completed by inference |
| Current definition unreadable or absent | Refused: revision is unproven, never guessed |

### What it records

The original request and its evidence are appended **verbatim** to the append-only
approval history before the head changes, at most once: a retry after a partial
failure cannot duplicate the entry. The head then becomes `INVALIDATED` with an
invalidation record carrying:

- the operator's reason, identity, and timestamp;
- the plan source fingerprint before and after the revision;
- the task definition digest before and after the revision;
- the task-scoped acceptance time of the change being invalidated.

For a legacy request the "before" values are unknown and recorded as empty rather
than fabricated.

### What it never does

- **It is not a decision.** `Decision` stays empty and no lifecycle action is
  recorded, so nothing reads as approved or declined.
- **It never writes the task.** Status, attempt count, and satisfaction are
  untouched: the task is not completed, satisfied, or skipped.
- **It does not replace verification.** The next run must still execute the task and
  produce fresh outcome evidence. Invalidation removes a gate that can no longer
  authorize anything; it proves nothing about the work.
- **It does not weaken blocking.** `PENDING` and `DECLINED` requests still stop a run
  exactly as before. Only an invalidated request lets the run proceed.

### Reading a gate's state

`sop approvals` prints `<task> <kind> <stage> <reason>`, for example:

```text
APPR-004  NEEDS_HUMAN  FAILED  ollama returned an empty response
```

That line names the request **kind** (`NEEDS_HUMAN`) and the **run stage** at request
time (`FAILED`). Neither is the decision state. Read the decision state from
`sop approval <task-id>`, which reports `Approval: PENDING|APPROVED|DECLINED|SUPERSEDED|INVALIDATED`
and any recorded decision. A request whose reason is a provider failure is a normal
pending gate that needs a decision, not a stale-evidence case.

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
