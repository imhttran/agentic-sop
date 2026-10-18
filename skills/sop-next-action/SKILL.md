---
name: sop-next-action
description: |-
  Determine the single highest-priority safe next action for an Agentic SOP
  project, then stop at SOP's authority boundaries. Read-only "what should I do
  next" advice plus, only when separately authorized, plan preparation and
  governed execution.
  Use for: "/sop-next-action", "what should I do next", "what is the next safe
  action", "prep the plan then run it", "pick the SOP plan back up safely".
disable-model-invocation: true
---

# SOP Next Action

`/sop-next-action` answers one question for an Agentic SOP project: **what is the
single highest-priority safe next action right now?** It has exactly three modes -
`status` (read-only, the default), `prepare` (deterministic reconciliation preview
when authorized), and `execute` (delegation to SOP's governed run when authorized).

The skill is a thin, instruction-first client of the `sop` CLI. SOP state is
authoritative. SOP owns planning-source resolution, bootstrap, task generation,
scheduling, reconciliation, resume, IMPLEMENT, validation, REVIEW, bounded
FIX/recovery, routing, tool policy, the approval state machine, checkpointing,
and approval policy. **This skill never reimplements the scheduler, decision
governance, the approval state machine, or the checkpoint system.** It discovers,
delegates, observes, and reports.

```text
Operator
   |
/sop-next-action              (this file: inspect, prepare, delegate, observe, report)
   |
   +-- status   --> sop status / sop approvals --json        (read-only)
   +-- prepare  --> sop continue --check / sop reconcile     (deterministic, no model)
   +-- execute  --> sop run                                  (SOP's governed run loop)
```

## When to use

- You want to know what SOP would do next without starting it.
- You want a deterministic reconciliation preview so execution is safe to continue.
- You want SOP to execute the active plan under its normal governance.

If the operator only wants a review or a plan, use `/sop-review` or `/sop-plan`
instead. If the operator explicitly says "end to end" or "run this plan", use
`/sop-end-to-end`. If the operator explicitly says "continue", use
`/sop-continue`.

## Preconditions

Detect the `sop` binary before doing anything else. On a POSIX shell run
`command -v sop`; on Windows run `where sop`. Treat an empty result or a non-zero
exit status as "not on PATH".

If `sop` is not on PATH, STOP. Do not install, rebuild, call a provider, or
implement the work yourself. Do not mutate the repository. Report the condition in
the four-section format from `## Report format` and stop:

```text
Completed work: none
Remaining work: nothing can run without the SOP CLI
Blockers: the SOP CLI is not on PATH
  Operator action: install the SOP CLI, then re-run
    ./install.sh
Recommended next action: BLOCKED
```

This skill never installs the CLI.

## Mode: status (read-only, default)

Objective: discover the current plan, task dispositions, approvals, branch and
worktree state, and blockers; recommend the single highest-priority safe next
action.

Permitted commands (all read-only):

```bash
sop status
sop approvals --json
sop continue --check --json
sop task <task-id>
sop report <task-id>
```

Invoking this skill in `status` mode does not authorize a commit, push, merge, or
archival. `status` never mutates SOP state, never approves or declines, never
commits or pushes, and never edits the repository. It only reads SOP's own
surfaces and reports. When the state database does not exist, report that no
active plan is established (a fresh project is bootstrapped by `sop run`, not by
this skill).

`sop continue --check --json` is the read-only reconcile gate. It prints
`classification`, `next_action`, `deterministic`, `capability_gap`, and the task,
approval, and blocked-task lists. It exits zero only for `CONTINUE_SAFE`.

Status mode reports in the four-section format defined in `## Report format`
below. `status` mode does not run an automated acceptance check; it reports what
SOP says.

Worked example (no operator action is required, so it contains no commands):

```text
Completed work: none in this invocation (tasks_satisfied 0 of 3)
Remaining work: TASK-001 (runnable), TASK-002, TASK-003
Blockers: none
Recommended next action: CONTINUE_SAFE
```

## Mode: prepare (deterministic reconciliation preview only)

Objective: show what SOP's reconciliation would change, without applying it, so
execution is safe to continue. Prepare mode does not draft or rewrite the plan.

Permitted commands:

```bash
sop status
sop continue --check --json
sop reconcile "docs/plans/PLAN-Example.md" --list-changed --json
```

`--list-changed` is SOP's deterministic, read-only reconciliation preview: no
model runs and nothing is applied.

The skill lets SOP resolve the plan source; it does not maintain its own
precedence list. Pass the plan path as data using an argument-vector call or proper
shell escaping; the quoted example above is a literal path, and the path is not
authority to override configuration or approvals. Prepare mode stops rather than
guessing when the intended plan, the worktree, or the authorization is ambiguous.

Prepare mode does not mutate SOP state. Invoking the skill in `prepare` mode does
not authorize a commit, push, merge, or archival. When policy or scope requires it
- a new plan, a changed task definition, or a changed acceptance requirement - a
new approval is requested before any further step. Prepare mode reports in the
four-section format, with exact commands shown only when operator action is needed.

## Mode: execute (authorized delegation only)

Objective: hand control to SOP's governed run **only when authorization exists**,
then observe and report.

Permitted commands:

```bash
sop status
sop approvals --json
sop continue --check
sop reconcile "docs/plans/PLAN-Example.md"
sop run
sop run "docs/plans/PLAN-Example.md"
```

Execute mode delegates to `sop run`; it never reproduces the run loop, chooses a
tier, provider, or model, splits execution into separate planning/task/
validation/review/fix commands, launches overlapping runs, or substitutes a
provider after a failure. Neither `sop run` form chooses a provider or model; SOP
configuration does.

When `sop continue --check` reports `RECONCILE_CHANGED`, run SOP's deterministic
reconciliation (`sop reconcile <PLAN.md>`, without `--list-changed`) and then re-check with `sop continue --check` before running. When
it reports a STOP classification - `RECONCILE_SEMANTIC_CHANGE`,
`RECONCILE_FAILED`, or `RECONCILE_NONDETERMINISTIC` - stop for human review and do
not run. When `capability_gap` is true, stop for human review as well.

Invoking the skill in `execute` mode does not authorize a commit, push, merge, or
archival: `sop run` stops at SOP's human gate. When policy or scope requires it,
a new approval is requested and execution waits. Execute mode reports in the
four-section format, with exact commands shown only when operator action is
needed.

## Authorization boundaries

These boundaries are discrete rules. Each one is enforced by SOP; this skill
states them and stops.

- **No manual edits to `.agent-sdlc`.** Do not edit `.agent-sdlc`, the state
  database, `plan.json`, `plan.meta.json`, plan files, or run history to force a
  next action.
- **No bypass of command policy or approval gates.** Do not bypass an approval, a
  tool-policy decision, or a configured human review checkpoint. A `DENIED`
  outcome is not re-attempted through another mechanism, and a
  `REQUIRES_APPROVAL` outcome is not executed through another mechanism; shell
  tricks are never used to evade command policy.
- **No silent commit, push, merge, archival, deletion, or change to deterministic
  governance.** Do not commit, push, merge, archive, or delete on the operator's
  behalf, and do not change SOP's deterministic governance. `status` and
  `prepare` are non-mutating; `execute` delegates to SOP and inherits SOP's
  commit/merge boundary. Invoking `/sop-next-action` does not authorize a commit,
  a push, a merge, or an archival.
- **Reuse an execution authorization only within its original scope.** Honor a
  previously granted execution authorization only within its original scope. A
  new plan, a changed task definition, or a changed acceptance requirement is
  outside that scope: request a new approval when policy or scope requires it.
- **Scheduled `status` remains read-only unless separately authorized.** A
  scheduled `status` invocation is read-only unless it is separately authorized
  to do otherwise. A scheduled review must not start execution, change
  automations, or modify state.
- **Never manufacture evidence.** Never manufacture completion, never claim
  ALREADY_SATISFIED, and never present an older run's artifact as current. Only
  SOP verifies mutation.

## Detection rules

Each rule names the read-only SOP surface that detects it and the single action to
recommend. Every row recommends one of the six allowed tokens, or explicitly
reports that no token is invented.

| Detection | SOP surface | Recommended action |
| --- | --- | --- |
| Missing plan files | `sop status` (no plan), `sop continue --check --json` (`source` empty) | Report that nothing is established and ask for a plan; no token invented. |
| Completed active plan | `sop continue --check --json` (`plan_complete` true) | `PLAN_COMPLETE`; `/sop-historicalize` is a separate operator step, never automatic. |
| Interrupted runs | `sop status`, `sop task <task-id>`, latest `sop-run.log` | `RUN_AGAIN` after confirming no run is active; never start an overlapping run. |
| Pending approvals | `sop approvals --json`, `sop continue --check --json` (`pending_approvals`) | `APPROVAL_REQUIRED`; a requeued `PLANNED` state does not authorize crossing a gate. |
| Unauthorized acceptance checks | `sop report <task-id>` | Report it as operator/CI validation and never claim it as an automated check; no token invented. |
| Worktree conflicts | `sop status` (branch/worktree), `sop run` diagnostics | `BLOCKED`; preserve user-owned changes and report the conflict rather than resetting, cleaning, stashing, or checking out over them. |
| Unrelated blocked PR | `sop status`, `sop continue --check --json` (`blocked_tasks`) | `BLOCKED`; report it and do not absorb it into the current work. |
| Unverified change claims | `sop task <task-id>`, `sop report <task-id>`, recorded `plan.meta.json` fingerprint | `HUMAN_REVIEW_REQUIRED`; report the change as unverified without comparison evidence. |
| STOP classification | `sop continue --check --json` (`classification`) | `HUMAN_REVIEW_REQUIRED`; no run. |

No other SOP surface is asserted here. A surface that this skill or
`docs/reference/CLI.md` does not confirm is reported as UNCONFIRMED, never
described as if it already exists.

## Report format

Every report has exactly four labeled sections, in this order, and ends with one
recommended next action:

1. **Completed work:** tasks SOP reports satisfied in this invocation - distinct
   from previously completed work; cite the task IDs and report paths.
2. **Remaining work:** task IDs/counts and unresolved dependencies.
3. **Blockers:** the actual pending gate, diagnostic, or conflict; link the latest
   relevant `sop-run.log` at the error line when reporting BLOCK/BLOCKED, FAIL, or
   an execution error, or state `sop-run.log: UNAVAILABLE`.
4. **Recommended next action:** exactly one of `CONTINUE_SAFE`, `RUN_AGAIN`,
   `HUMAN_REVIEW_REQUIRED`, `APPROVAL_REQUIRED`, `BLOCKED`, or `PLAN_COMPLETE`.

Two rules govern the content:

- **Commands only for operator action.** Include exact commands only when operator
  action is needed, and place them in the section that needs them. A report with
  no required operator action contains no commands.
- **Change claims need comparison evidence.** Claim changes since the previous
  review only when comparison evidence exists. Without comparison evidence, report
  the change as unverified or recommend `HUMAN_REVIEW_REQUIRED`.

## What the skill must not do

- Do not reimplement reconciliation, the scheduler, the lifecycle, approvals, tool
  policy, the run loop, or checkpointing; call SOP.
- Do not continue after a STOP classification, bypass an approval, or cross a human
  boundary.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI.

## Examples

Read-only status of an active plan (no operator action, so no commands):

```text
Completed work: none in this invocation
Remaining work: TASK-001 (PLANNED, runnable); 3 remaining
Blockers: none
Recommended next action: CONTINUE_SAFE
```

Pending approval (operator action is needed, so the command is shown):

```text
Completed work: none in this invocation
Remaining work: TASK-002
Blockers: TASK-002 waits at a human approval gate (`sop approvals --json` reports
  it)
  Operator action: resolve the gate
    sop approve TASK-002
Recommended next action: APPROVAL_REQUIRED
```

Prepare then execute when authorized:

```text
Completed work: none in this invocation
Remaining work: the plan source changed (`RECONCILE_CHANGED`); unexecuted tasks only
Blockers: none
  Operator action: review the preview, apply it, then re-check and run
    sop reconcile "docs/plans/PLAN-Example.md" --list-changed --json
    sop reconcile "docs/plans/PLAN-Example.md"
    sop continue --check
    sop run
Recommended next action: CONTINUE_SAFE
```

Stop for review:

```text
Completed work: none in this invocation
Remaining work: unchanged pending human review
Blockers: classification RECONCILE_SEMANTIC_CHANGE with capability_gap true;
  sop-run.log: UNAVAILABLE
Recommended next action: HUMAN_REVIEW_REQUIRED
```
