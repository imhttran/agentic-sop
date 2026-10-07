---
name: sop-continue
description: |-
  Safely continue an already-active Agentic SOP plan from its current governed
  state. Reconcile the source plan, the compiled plan, and persisted SOP state
  BEFORE continuing; stop for human review on any semantic change, nondeterminism,
  unsafe state, or existing human boundary.
  Use for: "/sop-continue", "continue this plan", "resume the active SOP plan",
  "pick the SOP plan back up".
disable-model-invocation: true
---

# SOP Continue

`/sop-continue` continues an **already-active** SOP plan from its current governed
state. It is not a generic "run SOP again": before any execution it reconciles the
source plan, the compiled plan, and the persisted lifecycle, and it continues only
when reconciliation shows the plan's meaning is preserved.

```text
/sop-continue
     |
     v
 PREFLIGHT -> RECONCILE -> VALIDATE CONTINUATION -> CONTINUE
                                                      |
                                                      v
                                                   OBSERVE
                                                      |
                                                      v
                                                STOP / REPORT
```

- Primary invariant: **reconcile before continue.** Do not continue execution
  until reconciliation has succeeded.
- Safety invariant: reconciliation must never silently rewrite plan semantics
  merely to make execution proceed.

SOP state is authoritative. SOP owns planning-source resolution, compilation,
reconciliation, scheduling, the lifecycle, approvals, the run loop, tool policy,
and reporting. This skill **orchestrates** them; it never reimplements them, never
edits the repository, and never edits the state database.

```text
Operator
   |
/sop-continue        (this file: preflight, reconcile, validate, delegate, observe, report)
   |
sop continue --check (SOP's deterministic, read-only reconcile-before-continue gate)
   |
sop run              (SOP's governed execution: the run loop, task execution, policy)
```

## PREFLIGHT

Check that `sop` is on PATH; if missing, STOP and report the installation
prerequisite. Do not install, rebuild, call a provider, or implement the work
yourself.

Collect, without mutating anything:

```bash
sop status
sop approvals --json
```

Record the repository root, current branch, `HEAD`, upstream, `git status`, the
active plan and its lifecycle state, the current task states, the current runnable
task, pending approvals, and existing run artifacts and deliverables. Preserve
user-owned changes: a dirty working tree is information, not automatically an
error. Do not require a clean tree unless normal SOP semantics require one.

Never `reset`, `clean`, `stash`, `checkout` over, overwrite, or delete user-owned
changes automatically.

## DIAGNOSTICS

Continuation runs should be observable. Before invoking a compiler, reconcile, or
run operation, configure the operator-set diagnostic sinks when the underlying
execution path supports them, using unique temporary paths per invocation:

```text
SOP_OLLAMA_TRACE_LOG   the per-turn trace sink
SOP_TOOL_AUDIT_LOG     the tool-audit sink

/tmp/sop-continue-<session>/trace.log
/tmp/sop-continue-<session>/audit.jsonl
/tmp/sop-continue-<session>/run.log
```

These are **operator-set diagnostic sinks, never SOP's state database and never a
task capability**: setting them does not add a prerequisite to the plan, does not
become a task, and does not change plan meaning. Do not commit the logs. If the
repository already has a standard diagnostics abstraction, use it rather than
duplicating one. State the diagnostic paths in the final report when they were
enabled.

## RECONCILE

THIS PHASE IS MANDATORY. Before another governed execution attempt, reconcile the
source plan, the compiled plan, and the persisted SOP state. Use SOP's own
deterministic reconciliation; do not reimplement it.

```bash
sop continue --check --json
```

With an explicit plan:

```bash
sop continue --check "docs/plans/PLAN-Example.md" --json
```

`sop continue --check` is read-only: it runs SOP's deterministic reconciliation
preview (no model, no mutation) against the active plan and classifies the result.
To see the raw preview it is based on, inspect without applying anything:

```bash
sop reconcile "docs/plans/PLAN-Example.md" --list-changed --json
```

It preserves completed task state, LOCAL_DONE, NOT_REQUIRED, attempts/history,
existing evidence, reports, approval state, and user-owned working-tree changes. It
does not apply a reconciliation, force a retry, reset attempts, or mark anything
complete.

## RECONCILIATION CLASSIFICATION

`sop continue --check` reports exactly one classification:

| Classification               | Meaning                                                                                                                                                                                                                                                              | Action                                                                                                                |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `RECONCILE_CLEAN`            | Source, compiled plan, and persisted state are compatible; nothing to reconcile.                                                                                                                                                                                     | Continue automatically.                                                                                               |
| `RECONCILE_CHANGED`          | A deterministic, semantics-preserving change (plan-level metadata, or descriptive text on an executed task).                                                                                                                                                         | Validate, then continue only if task identity, lifecycle meaning, dependencies, and acceptance meaning are unchanged. |
| `RECONCILE_SEMANTIC_CHANGE`  | Reconciliation would change plan meaning: a new prerequisite or capability, a changed dependency or acceptance requirement, changed deliverable semantics, a task added/removed, a changed task ID, a materially changed executed task, or a changed human boundary. | STOP for human review.                                                                                                |
| `RECONCILE_FAILED`           | Reconciliation cannot establish a valid continuation state (no active plan, an unreadable or uncompilable source, a store failure).                                                                                                                                  | STOP.                                                                                                                 |
| `RECONCILE_NONDETERMINISTIC` | Repeated reconciliation of the same source and state produces materially different results (recurring model-generated repair).                                                                                                                                       | STOP. Do not continue into `sop run`.                                                                                 |

Relay the classification and its evidence verbatim. Do not upgrade a STOP into a
continue.

### Capability repair

When the compiler infers a missing capability from task prose, do **not** add that
capability to the source plan. Determine whether it is a genuine external
prerequisite or repository discovery/work that belongs inside the task, and
remember that repository discovery (inspecting provider registration, CLI
selection, existing adapters, configuration, interfaces, tests, or package
structure) must not be converted into a prerequisite merely to satisfy the
compiler. If resolving it would change source-plan semantics, the result is
`RECONCILE_SEMANTIC_CHANGE`: STOP. If repeated compilation keeps generating the
same repair despite unchanged source and state, the result is
`RECONCILE_NONDETERMINISTIC`: STOP.

### Determinism

`sop continue --check` validates stability before reporting by reconciling the same
source and state a small, bounded number of times and comparing the results (same
machine task IDs, dependency graph, capability model, Requires, lifecycle meaning,
runnable task, and no new repair request). If stability cannot be demonstrated it
reports `RECONCILE_NONDETERMINISTIC` and STOPs. The skill never invents its own
reconciliation loop.

## VALIDATE CONTINUATION

After a successful reconciliation, `sop continue --check` reports the next action.
Verify the plan is eligible for continuation: the active plan still exists, the
runnable task is correctly identified, dependencies are satisfied, required
capabilities are available, existing artifacts are preserved, no pending approval
prohibits continuation, the task is not BLOCKED or terminal, and no human review
boundary is being crossed automatically.

An existing artifact does **not** automatically mean a task is complete. If a task
was requeued after a CONTINUE or AUTO_CONTINUE, preserve its artifact and let normal
SOP acceptance verification decide whether more work is required. Do not
manufacture ALREADY_SATISFIED.

## CONTINUE

Continue only when the gate authorizes it (`next_action` is `CONTINUE_SAFE`). SOP's
governed run accepts an unchanged source directly; a source that reconciliation
classified `RECONCILE_CHANGED` must be reconciled deterministically first, because
`sop run` refuses a changed source until the change is applied.

When the check reports `RECONCILE_CLEAN` (`plan_changed` false):

```bash
sop run
```

When the check reports `RECONCILE_CHANGED` (`plan_changed` true), apply SOP's
deterministic, semantics-preserving reconciliation and then run:

```bash
sop reconcile "docs/plans/PLAN-Example.md"
sop run "docs/plans/PLAN-Example.md"
```

Do not pass `--accept-changed`: replacing a changed **executed** task's definition
is a human decision. If `sop reconcile` stops (`NEEDS_HUMAN`) because that approval
is required, STOP for human review; do not work around it. After an applying
reconciliation, re-check (`sop continue --check`) before running; it should now be
`RECONCILE_CLEAN`.

SOP remains responsible for task execution, policy, tool authorization, retries,
continuation, acceptance, review, approvals, and lifecycle transitions. Do not
reproduce the run loop inside the skill. Pass a plan path as data, never as a shell
command.

Respect `run_command` decisions. A command that is `DENIED` is not bypassed; a
command that `REQUIRES_APPROVAL` is not silently executed through another
mechanism. Never use shell tricks to evade tool policy.

## OBSERVE

Observe the governed run and recognize at minimum: task completed, LOCAL_DONE,
CONTINUE, AUTO_CONTINUE, a requeued task, BLOCKED, approval required, gate failure,
no progress, plan repair, compiler repair, an unexpected lifecycle change, and plan
completed. Use targeted evidence rather than large transcripts:

```bash
sop task <task-id>
sop report <task-id>
sop approvals --json
```

If SOP exits after a legitimate CONTINUE/AUTO_CONTINUE requeue, determine whether
another invocation is safe. Continuation is **bounded**: prefer one governed
continuation per skill cycle and never implement an unbounded retry loop. If SOP
semantics explicitly provide a bounded continuation, respect that bound. If another
run is required, report it clearly as `RUN_AGAIN` and stop; do not loop.

## STOP BOUNDARIES

STOP for: human approval; a BLOCKED task; a failed required gate;
`RECONCILE_SEMANTIC_CHANGE`; `RECONCILE_FAILED`; `RECONCILE_NONDETERMINISTIC`; an
unexpected plan mutation; a task identity change; an unsafe lifecycle change; a
configured human review checkpoint; repository state that cannot be safely
preserved; or exhausted bounded continuation attempts.

Never force retries, edit the state database by hand, delete run history, reset
task attempts, mark tasks complete manually, manufacture evidence, bypass
approvals, bypass tool policy, bypass review, force-push, amend unrelated commits,
or discard user changes. Manual state editing, forced retries, silent lifecycle
mutation, and history rewriting are not acceptable recovery mechanisms.

## COMMIT / PUSH POLICY

Continuation does **not** imply authorization to commit or push.

Default: **NO COMMIT, NO PUSH.** Do not infer authorization from the fact that
`/sop-continue` was invoked. If the active SOP plan reaches an existing human
commit or merge boundary, respect that boundary and report it.

## REPORT

Report every invocation with:

- PREVIOUS STATE: plan, plan status, current task, task status, `HEAD`,
  working-tree summary.
- RECONCILIATION: classification; source-plan change detected; compiled-plan
  change detected; capability repair detected; deterministic; lifecycle preserved;
  human review required.
- EXECUTION: whether `sop run` was invoked; the task executed; the outcome; the
  continuation/requeue outcome; the approval outcome; gates encountered.
- EVIDENCE: artifacts created; artifacts preserved; diagnostic paths; relevant
  run/report paths.
- CURRENT STATE: plan status; task states; current/next task; pending approvals;
  working-tree state.
- NEXT ACTION: exactly one recommendation - `CONTINUE_SAFE`, `RUN_AGAIN`,
  `HUMAN_REVIEW_REQUIRED`, `APPROVAL_REQUIRED`, `BLOCKED`, or `PLAN_COMPLETE`.

Do not claim work completed unless SOP state and evidence support it. For a
BLOCK/BLOCKED, FAIL, or execution error, link the latest relevant `sop-run.log` at
the error line, following [SOP failure reporting](../sop/SKILL.md#failure-behavior).

## What the skill must not do

- Do not edit the state database, `plan.json`, `plan.meta.json`, plan files, or run
  history to force continuation.
- Do not reimplement reconciliation, the scheduler, the lifecycle, approvals, tool
  policy, or the run loop; call SOP.
- Do not continue after a STOP classification, bypass an approval or tool-policy
  decision, or manufacture completion.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see
  the project README / `./install.sh`). Never mutate the repository in SOP's place.

## Examples

Clean continuation:

```text
Plan ACTIVE
TASK-001 PLANNED after CONTINUE
existing report preserved
sop continue --check --json -> RECONCILE_CLEAN, deterministic, no approvals
-> CONTINUE_SAFE: `sop run` continues the plan
```

Semantic or nondeterministic reconciliation:

```text
compiler repeatedly invents a prerequisite capability
sop continue --check --json -> RECONCILE_NONDETERMINISTIC
  (or RECONCILE_SEMANTIC_CHANGE with capability_gap true)
-> STOP, HUMAN_REVIEW_REQUIRED
-> a human reviews the source plan before any run
```
