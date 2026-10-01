# CLI Reference

The `sop` command surface: one entry per subcommand with its purpose and
documented flags and behavior. Lifecycle, validation, review, and quality
semantics are specified in
[`docs/specs/EXECUTION.md`](../specs/EXECUTION.md); requeue and recovery
semantics in [`docs/specs/RECOVERY.md`](../specs/RECOVERY.md).

## Commands

| Command                                                                                | Purpose                                                                                                                                                               | Key flags / behavior                                                                                                                                                                                                                                                                                                                                                                                                                     |
| -------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sop init`                                                                             | Initialize SOP state and generate the configuration template.                                                                                                         | Creates `.agent-sdlc/state.db` and, when absent, `.agent-sdlc/config.yaml`. Re-running is safe: existing state and a hand-edited configuration are never overwritten.                                                                                                                                                                                                                                                                    |
| `sop plan [TASK.md]`                                                                   | Generate an implementation plan from `PRD.md`, or from a Markdown task file.                                                                                          | A task file is parsed into ID, title, description, requirements, acceptance criteria, and constraints, then normalized before the agent reasons over it. Produces `PLAN.md` and `.agent-sdlc/plan.json`.                                                                                                                                                                                                                                 |
| `sop tasks`                                                                            | Convert the machine-readable Plan into persisted tasks.                                                                                                               | Reads `.agent-sdlc/plan.json`.                                                                                                                                                                                                                                                                                                                                                                                                           |
| `sop validate`                                                                         | Run the configured build/test/lint commands and report a deterministic result.                                                                                        | Commands come from `validation` in `.agent-sdlc/config.yaml`; they run in the project directory, in order (build, test, lint), and stop at the first failure. Exit code is non-zero on failure.                                                                                                                                                                                                                                          |
| `sop review`                                                                           | Review the current working-tree changes with the configured engine.                                                                                                   | `self` (default) asks the agent for structured findings; `open-code-review` runs an external command (`SOP_REVIEW_COMMAND`). The model never decides the verdict: findings whose severity is named in `quality.fail_on` are blocking, and the exit code is non-zero when any remain. With no working-tree changes it reports “no changes to review”.                                                                                     |
| `sop run [--model-class small\|medium\|large] [PLAN.md \| --task TASK.md]`             | The one-command workflow: discover or resolve the planning source, initialize state, compile or generate `.agent-sdlc/plan.json`, create tasks, then drive the graph. | A file argument names an execution PLAN (authoritative for that run); `--task TASK.md` runs a single task through the local lifecycle instead; `--model-class` overrides the model-routing class for this invocation (see [`CONFIGURATION.md`](CONFIGURATION.md)). Each step is idempotent. See the precedence and multi-plan notes below.                                                                                               |
| `sop prompt [--capability CAP] [--model-class CLASS] [--json] [--file PATH \| PROMPT]` | Run an ad-hoc operator prompt through SOP's governed execution (Phase 5.4).                                                                                           | Capability defaults to `plan` (read-only); `plan`, `review`, `diagnose_failure`, and `design_tests` run read-only, while `implement` runs the full governed implementation lifecycle. Routing reuses the `small`/`medium`/`large` classes; provider validation applies when `providers.validate` is on. Runs are written under `.agent-sdlc/runs/prompts/<run-id>/`. See [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md). |
| `sop report [run-id]`                                                                  | Print a concise, informational summary of a run (the latest of either kind by default).                                                                               | Shows stage, provider, gate, per-check validation, and findings by severity. When model routing or bounded escalation applies, it also shows the `Model routing:` decision and an `Execution attempts:` section with the initial class, each attempt, and the escalation metrics.                                                                                                                                                        |
| `sop commit [TASK.md] [--yes]`                                                         | Commit the current changes with a message derived from a task file (no push).                                                                                         | When `human.approval_before_commit` is on (the default), `--yes` is required.                                                                                                                                                                                                                                                                                                                                                            |
| `sop pr TASK.md [--yes]`                                                               | Push a task branch and open a pull request.                                                                                                                           | The branch is `task/<id>-<slug>` and the base is the integration branch. It never merges.                                                                                                                                                                                                                                                                                                                                                |
| `sop mcp`                                                                              | Serve tools over the Model Context Protocol on stdio.                                                                                                                 | Exposes `sop_status`, `sop_validate`, and `sop_review`, backed by the same services as the CLI.                                                                                                                                                                                                                                                                                                                                          |
| `sop eval DIR`                                                                         | Run a corpus of task files through the lifecycle and report benchmark metrics.                                                                                        | Input is a directory of task files.                                                                                                                                                                                                                                                                                                                                                                                                      |
| `sop status`                                                                           | Show project task state.                                                                                                                                              | Prints one line per task (`<ID> <STATE> <title>`). See [`STATUS-AND-RECOVERY.md`](STATUS-AND-RECOVERY.md).                                                                                                                                                                                                                                                                                                                               |
| `sop task <id>`                                                                        | Inspect a particular task.                                                                                                                                            | See [`STATUS-AND-RECOVERY.md`](STATUS-AND-RECOVERY.md).                                                                                                                                                                                                                                                                                                                                                                                  |
| `sop resume [task-id]`                                                                 | Report the next legal action for interrupted work.                                                                                                                    | With no id it resumes the single in-flight task. The same action rule drives `sop run`. See [`STATUS-AND-RECOVERY.md`](STATUS-AND-RECOVERY.md).                                                                                                                                                                                                                                                                                          |
| `sop retry <task-id> \| --all [--force]`                                               | Requeue a `BLOCKED` task to `PLANNED` so the next `sop run` retries it, or requeue every `BLOCKED` task that still has retry budget at once.                          | `--force` also requeues a task whose `max_attempts` is spent, raising the budget. See [`../specs/RECOVERY.md`](../specs/RECOVERY.md).                                                                                                                                                                                                                                                                                                    |
| `sop reconcile <PLAN.md> [--accept-changed <TASK_ID>]...`                              | Apply an intentional plan change.                                                                                                                                     | Preserves every unchanged task and its history; updates only never-executed tasks; adds and removes unexecuted tasks. `--accept-changed` is repeatable. See [`../specs/RECOVERY.md`](../specs/RECOVERY.md).                                                                                                                                                                                                                              |
| `sop providers [--models]`                                                             | Inspect the configured provider runtimes (read-only).                                                                                                                 | Prints each provider's health and model count; `--models` also lists discovered models with their locality and capabilities. It never mutates state and never prints credentials. See [`../specs/PROVIDERS.md`](../specs/PROVIDERS.md) and [`CONFIGURATION.md`](CONFIGURATION.md).                                                                                                                                                       |
| `sop version`                                                                          | Print the SOP version.                                                                                                                                                | Listed among the available commands.                                                                                                                                                                                                                                                                                                                                                                                                     |
| `sop help`                                                                             | Print command help.                                                                                                                                                   | Listed among the available commands.                                                                                                                                                                                                                                                                                                                                                                                                     |

## `sop run`: planning-source precedence

When no plan is named, `sop run` resolves the planning source in this order:

```text
1. an existing valid .agent-sdlc/plan.json (reused only if its source is unchanged)
2. docs/PLAN.md
3. PLAN.md
4. docs/PRD.md
5. PRD.md
```

A human PLAN is preferred over the PRD, and `plan.json` never silently overrides
a newer PLAN: the source path and a content fingerprint are recorded, and a
changed document triggers a rebuild. `sop init`, `sop plan`, and `sop tasks`
remain the explicit lower-level commands.

Preparation does not require an agent: `sop run` compiles a `PLAN.md` and
creates tasks even when no agent is configured, then reports the missing agent
only when it needs to execute (or to generate a plan from a PRD).

## `sop run`: multiple plans and handoff

Multiple plans may coexist under `docs/` (`PLAN.md`, `PLAN-Hardening.md`,
`PLAN-Jev.md`, …). Each carries its own identity (source path, `source_sha256`,
and a plan id), so `sop run docs/PLAN-Jev.md` runs that plan independently.

- If the named plan's content changed after tasks were created, `sop run` stops
  with an actionable `NEEDS_HUMAN` rather than silently rebuilding the graph.
- If a different plan is requested while another is active, `sop run` hands off
  automatically when every task in the active plan is complete (archiving its
  records under `.agent-sdlc/archive/<plan-id>/`); otherwise it stops with
  `NEEDS_HUMAN`. It never mixes tasks from two plans or silently switches away
  from unfinished work.

The **scheduler** selects the next ready task and runs the same lifecycle; a
passing gate marks the task `LOCAL_DONE` and a failing gate marks it `BLOCKED`,
repeating until no runnable work remains. When a task is already in flight,
`sop run` resumes it — from the same persisted state → next-action rule that
`sop resume` reports — instead of stopping with `ACTIVE_TASK`. `ACTIVE_TASK`
still prevents starting a _different_ task.

Every stage writes an artifact under `.agent-sdlc/runs/<id>/` (`task.md`,
`plan.md`, `implementation.md`, `diff.patch`, `fix-N.md`, `validation.json`,
`review.json`, `report.md`, `report.json`, `metrics.json`, `state.json`). A
failing check or blocking finding is sent back to the agent and re-checked, up
to `quality.max_fix_cycles` times; exhausting the budget yields `NEEDS_HUMAN`.
`sop run` stops at the human gate and never commits, pushes, or merges. When JEV
is enabled (`quality.jev.enabled: true`), an optional read-only analysis stage
runs after validation and review and feeds the same gate and fix loop.

With `SOP_MODEL_ROUTING_ENABLED=true` each task's model class is chosen from typed
evidence and recorded in `routing.json`. With `SOP_MODEL_ESCALATION_ENABLED=true` a
task that fails the gate may be retried on the next larger class in the same
invocation (bounded by `SOP_MODEL_MAX_ESCALATIONS`, default 2), and each attempt is
recorded under `attempts/`. Both are OFF by default and apply equally to an
`implement` prompt. See
[`CONFIGURATION.md`](CONFIGURATION.md) and [`../specs/RECOVERY.md`](../specs/RECOVERY.md).

## `sop prompt`

`sop prompt` runs an ad-hoc request through the same governed machinery a task
uses. It is the entry point the SOP skill calls.

```bash
sop prompt "Explain this project's architecture"          # default: --capability plan
sop prompt --capability review "Review internal/provider for architectural issues"
sop prompt --capability diagnose_failure --file prompts/debug.md
sop prompt --capability implement "Add caching to provider discovery"
```

The first three are read-only: the model produces text and the repository is never
modified. `--capability implement` runs the governed implementation lifecycle
(plan → implement → validate → review → gate → fix → approval), exactly as a planned
task does; it cannot run on a provider that does not declare `IMPLEMENT`. When model
routing and bounded escalation are enabled it follows the same path as a task,
including a retry on the next larger class and per-attempt records under
`.agent-sdlc/runs/prompts/<run-id>/attempts/`.

`--json` prints the structured result document (`work_item_id`, `kind`,
`capability`, `status`, `routing`, `report_path`, `result_path`, `result`). Every run
is recorded under `.agent-sdlc/runs/prompts/<run-id>/`; inspect one with
`sop report prompts/<run-id>` (or just `sop report` for the latest run of either
kind), which works for a read-only run too.

## Recovery commands

`sop resume`, `sop retry`, and `sop reconcile` operate on persisted state for
interrupted, blocked, or changed work. Their next-action, requeue, and
reconciliation semantics are in
[`docs/specs/RECOVERY.md`](../specs/RECOVERY.md) and summarized in
[`STATUS-AND-RECOVERY.md`](STATUS-AND-RECOVERY.md).
