# Outcome recovery and designated-approver waivers (SOP-HARD-04)

This documents the supported recovery surface for a task whose objective-level
outcome verdict is `PARTIAL` because a **required** deliverable or check was not
met: a read-only `sop verify` and a designated-approver `sop outcome --waive`. It
complements the verdict gate in
[`LIFECYCLE-VERDICT-GATE.md`](LIFECYCLE-VERDICT-GATE.md).

## `sop verify <task-id>` — read-only

Reprints the task's recorded objective-level verdict, per criterion, from the run
artifact `.agent-sdlc/runs/<task-id>/outcome.json`:

```
sop verify SOP-HARD-04
```

It prints the status, the resolved lifecycle (when the gate is on), each
criterion's state (`met` / `not met` / `informational`), and any recorded
waivers. It performs **no** state transition and writes **no** file: it is safe to
run at any time.

## `sop outcome` — print, or record a waiver

```
sop outcome <task-id>
sop outcome <task-id> --waive <criterion-id> --reason R --by NAME [--scope S]
```

Without `--waive`, it prints the same per-criterion verdict as `sop verify`.

With `--waive` it records one **criterion-scoped** waiver as durable, append-only
audit evidence in `.agent-sdlc/runs/<task-id>/waivers.jsonl`.

### Designated approvers

Only an identity named in configuration may grant a waiver:

```yaml
human:
  designated_approvers:
    - operator
```

The list is empty by default, so the waiver path fails closed. Matching is exact
and case-sensitive (surrounding whitespace is ignored).

### The waiver record (pinned schema)

Every grant appends one JSON line with exactly these fields:

| Field | Meaning |
| --- | --- |
| `task` | the task the waiver covers |
| `criterion_id` | the unmet **required** criterion it waives (criterion-scoped) |
| `scope` | the deliverable path or check id (defaults to the criterion text) |
| `reason` | the approver's recorded justification |
| `approver` | attribution: the designated approver |
| `recorded_at` | RFC 3339 timestamp |
| `outcome` | the verdict that was waived |

### What is enforced

A waiver is **never implied and never silently applied**. `sop outcome --waive`
writes nothing unless all of the following hold:

- the approver is a designated approver;
- a non-empty reason and a timestamp are present;
- the named criterion is an **unmet required** criterion of the recorded verdict
  (a met or informational criterion can never be waived).

Otherwise it fails with a clear error and no file is written.

The audit file is opened `O_APPEND|O_CREATE`, so it is only ever extended. It is
never rewritten or reordered, the plan archive is never edited, and the task's
recorded `PARTIAL` verdict is preserved — a waiver is an appended record, not a
rewrite of history.

### Surfacing

A recorded waiver is surfaced by:

- `sop outcome <task-id> --waive ...` (on record),
- `sop verify <task-id>` and `sop outcome <task-id>` (the `waivers:` section),
- `sop report <task-id>` (a `Waivers:` section),
- `sop status` (a `[waived: <criteria>]` marker beside the task).

### Effect on the lifecycle gate

When the operator-enabled verdict gate is **on**, a recorded waiver covering
every unmet required criterion resolves the gated `PARTIAL` to `LOCAL_DONE`. With
no recorded waiver the gate fails closed to `NEEDS_HUMAN` and reports the denial.
When the gate is **off** (the default), the lifecycle is unchanged; the waiver is
recorded and surfaced as evidence only.

## Manual acceptance across an approved recovery (SOP-003)

A manual `sop outcome <task-id> --accept ...` decision is bound to the attempt
whose outcome the operator reviewed. A supported recovery may advance that
attempt before the next run (for example the operator accepts the reviewed
outcome, approves the `NEEDS_HUMAN` gate, and the task is requeued). The next
verification would otherwise reject a still-valid decision on a stale attempt
binding.

SOP records an append-only recovery binding in
`.agent-sdlc/runs/<task-id>/acceptance_recoveries.jsonl` when the designated
approver records the acceptance or approves the gate. A recovery names the
criterion, its text, the manual-evidence description, the decision's evidence,
and the exact `from_attempt`/`to_attempt` transition. It never rewrites the
original decision or its attempt provenance.

The next verdict honors the earlier decision only when both hold:

- a recorded recovery connects the decision's attempt to the current attempt;
  and
- the decision's criterion text, manual-evidence description, and recorded
  evidence still match the current acceptance mapping exactly.

A changed criterion, changed manual-evidence description, changed decision
evidence, or a missing recovery leaves the decision unbound, so the criterion
stays unmet and the lifecycle stops at the human boundary. Pending and declined
approval gates are unchanged: they still block execution before verification.

A recovery write is idempotent; repeating `sop approve` appends no duplicate.
The recovery audit is evidence only: it never changes a task's status, attempt
counter, or budget.

## Relationship to `sop retry`

`sop retry` is unchanged: it still requeues a **BLOCKED** task, and only a
`BLOCKED` task. `sop verify` / `sop outcome` never transition a task's lifecycle.
