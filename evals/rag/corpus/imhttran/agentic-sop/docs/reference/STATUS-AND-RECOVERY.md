# Status and Recovery Reference

How SOP reports task status and how interrupted, blocked, or changed work is
recovered. The authoritative state machine is in
[`docs/specs/WORKFLOW.md`](../specs/WORKFLOW.md); requeue and reconciliation
semantics are in [`docs/specs/RECOVERY.md`](../specs/RECOVERY.md); the human gate
in [`docs/specs/HUMAN-APPROVAL.md`](../specs/HUMAN-APPROVAL.md).

## Status vocabulary

| Status             | Meaning                                                            |
| ------------------ | ------------------------------------------------------------------ |
| `PLANNED`          | Task created from the plan; dependencies not yet satisfied.        |
| `READY`            | Dependencies satisfied; eligible for the scheduler.                |
| `BRANCH_CREATED`   | Task branch exists.                                                |
| `TESTS_WRITTEN`    | Failing tests written.                                             |
| `RED_VERIFIED`     | Red state verified.                                                |
| `IMPLEMENTING`     | Implementation in progress.                                        |
| `LOCAL_TESTS_PASS` | Local gates pass.                                                  |
| `REVIEW`           | Review stage running.                                              |
| `REVIEW_PASS`      | Review passed.                                                     |
| `LOCAL_DONE`       | Local lifecycle complete (no PR, no CI).                           |
| `PR_OPEN`          | Pull request opened.                                               |
| `CI_RUNNING`       | CI running on the pull request.                                    |
| `CI_PASS`          | CI passed.                                                         |
| `MERGED`           | Pull request merged.                                               |
| `DONE`             | Task complete.                                                     |
| `FIX_REQUIRED`     | Actionable failure routed back to `IMPLEMENTING`.                  |
| `BLOCKED`          | Terminal: retries exhausted or a hard failure.                     |
| `NEEDS_HUMAN`      | Not terminal: a human boundary; the task is requeued to `PLANNED`. |

## Transitions summary

```text
PLANNED → READY → BRANCH_CREATED → TESTS_WRITTEN → RED_VERIFIED → IMPLEMENTING
        → LOCAL_TESTS_PASS → REVIEW → REVIEW_PASS
        → LOCAL_DONE                                   (local, workflow.mode: local)
        → PR_OPEN → CI_RUNNING → CI_PASS → MERGED → DONE (remote)
```

Only legal transitions are allowed: `PLANNED → READY` is legal, `PLANNED →
MERGED` is not. From `REVIEW_PASS` a task either completes locally at
`LOCAL_DONE` (no PR opened, no CI run) or continues through the remote lifecycle
(`PR_OPEN` … `DONE`); a dependency is satisfied by `LOCAL_DONE` (local) or
`MERGED`/`DONE` (remote), and local runs never fabricate remote states.

Remediation keeps failures from terminating the workflow:

```text
IMPLEMENTING ───────┐
LOCAL_TESTS_PASS ───┤
REVIEW ─────────────┼──→ FIX_REQUIRED → IMPLEMENTING
CI_RUNNING ─────────┘
```

Retries are bounded; if recovery cannot continue safely the task eventually
becomes `BLOCKED`. `BLOCKED` and `DONE` are terminal.

## `sop status` / `sop task <id>`

`sop status` lists the project's tasks, one line per task; `sop task S002`
inspects a particular task.

```text
S001 PLANNED Application skeleton
S002 PLANNED Book ingestion
S003 PLANNED Embeddings
S004 PLANNED Retrieval
```

SQLite (`.agent-sdlc/state.db`) remains the durable source of truth for task
execution state.

## `sop resume`

Reports the next legal action for interrupted work; with no id it resumes the
single in-flight task.

```bash
sop resume
sop resume S001
```

The same action rule drives `sop run`: when a task is in flight, `sop run`
resumes it — completing it locally if its gates already passed, running the
normal lifecycle if it is still before them — rather than refusing to continue.

## `sop retry`

Requeue a `BLOCKED` task to `PLANNED` so the next `sop run` retries it, or
requeue every `BLOCKED` task that still has retry budget at once.

```bash
sop retry S001
sop retry --all
sop retry S001 --force
```

- `--force` also requeues a task whose `max_attempts` is spent, raising the
  budget.
- A requeue spends one attempt against `max_attempts`; a task whose budget is
  spent is reported and left `BLOCKED`.
- The retry resumes the same task and is given the previous attempt's outcome as
  context.

## `sop reconcile`

Applies an intentional change to a plan after its task graph exists.

```text
sop reconcile docs/PLAN.md
sop reconcile docs/PLAN.md --accept-changed S003
```

- Reconciliation preserves every unchanged task and its history, updates only
  tasks that have never executed, adds and removes unexecuted tasks, and stops
  with `NEEDS_HUMAN` (naming the task and the difference) when an executed
  task's definition changed or it was removed.
- It never deletes `.agent-sdlc/state.db` or discards run history.
- `--accept-changed <TASK_ID>` approves a changed executed task: its definition
  is replaced while its lifecycle state, attempts, and history are preserved,
  recorded in `plan.meta.json`. The flag is repeatable, and a changed executed
  task is approved only when named, so one approval never silently covers
  another task.
