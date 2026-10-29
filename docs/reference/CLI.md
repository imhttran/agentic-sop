# CLI Reference

The `sop` command surface: one entry per subcommand with its purpose and
documented flags and behavior. Lifecycle, validation, review, and quality
semantics are specified in
[`docs/specs/EXECUTION.md`](../specs/EXECUTION.md); requeue and recovery
semantics in [`docs/specs/RECOVERY.md`](../specs/RECOVERY.md).

## Commands

| Command                                                                                                        | Purpose                                                                                                                                                               | Key flags / behavior                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sop init`                                                                                                     | Initialize SOP state and generate the configuration template.                                                                                                         | Creates `.agent-sdlc/state.db` and, when absent, `.agent-sdlc/config.yaml`. Re-running is safe: existing state and a hand-edited configuration are never overwritten.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `sop plan [TASK.md]`                                                                                           | Generate an implementation plan from `PRD.md`, or from a Markdown task file.                                                                                          | A task file is parsed into ID, title, description, requirements, acceptance criteria, and constraints, then normalized before the agent reasons over it. Produces `PLAN.md` and `.agent-sdlc/plan.json`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `sop plan historicalize [PLAN.md] [--disposition COMPLETE\|SUPERSEDED] [--check] [--json]`                     | Historicalize a completed or explicitly disposed plan: validate readiness, then archive it and remove it from ACTIVE selection.                                       | The deterministic, model-free lifecycle transition. `--check` reports readiness only; `--disposition SUPERSEDED` explicitly disposes unfinished work. Fails closed unless every task is satisfied, no approval is pending, and verification evidence exists; idempotent (`ALREADY_HISTORICALIZED`). See [`../specs/PLAN-HISTORICALIZATION.md`](../specs/PLAN-HISTORICALIZATION.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `sop tasks`                                                                                                    | Convert the machine-readable Plan into persisted tasks.                                                                                                               | Reads `.agent-sdlc/plan.json`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `sop validate`                                                                                                 | Run the configured build/test/lint commands and report a deterministic result.                                                                                        | Commands come from `validation` in `.agent-sdlc/config.yaml`; they run in the project directory, in order (build, test, lint), and stop at the first failure. Exit code is non-zero on failure.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `sop security scan [--json] [--report-only\|--required] [--scope MODE] [--baseline PATH] [--resume RUN_ID] [--run-id ID] [--config PATH] [--auto]` | Run the opt-in Semgrep CE/Gitleaks/cargo-audit security stage against the workspace. | Disabled unless `security.enabled` (or `SOP_SECURITY_ENABLED`) is set. Report-only records PASS/FAIL/INDETERMINATE/NOT_APPLICABLE without blocking; required exits non-zero unless the verdict is PASS. `--auto` adds missing built-in categories from detected Go/Rust content while explicit adapters stay authoritative. Writes `.agent-sdlc/runs/<run-id>/security/scan.json` and `scan.md`; advisory-database identity is part of resume validation. `--config` loads a separate project configuration for one scan. See [`../specs/SECURITY-SCANNER-ADDON.md`](../specs/SECURITY-SCANNER-ADDON.md). |
| `sop security inspect [--json] [--config PATH]` | Read-only repository-content and capability inspection. | Reports detected languages, manifests, Cargo workspaces, actual and missing lockfiles, configured adapter status (`available`, `unavailable`, `unsupported`, `proposed`), and proposed adapters. Executes no scanner and changes no state. |
| `sop compare --manifest PATH --baseline PATH --candidate PATH --out DIR --commit SHA --executor NAME --model NAME --executor-command CMD` | Run the opt-in HIP baseline/candidate comparison harness. | Runs both arms from the same commit in disposable worktrees with fixed pair, execution, time and token ceilings. It is outside `sop run`, never promotes a candidate, and never pushes, merges or publishes. See [HIP comparison](../guides/HIP-COMPARE.md). |
| `sop result publish TASK --kind PROGRESS\|BLOCKER\|TEST\|COMPLETION --summary TEXT [--evidence PATH]... [--by NAME]`<br>`sop result list TASK [--json]` | Publish or inspect durable executor-result evidence. | Publications are bound to task, attempt, plan revision and run epoch; stale submissions fail closed and duplicate submissions are idempotent. A publication never changes lifecycle state, grants approval or triggers execution. |
| `sop planner ask TASK QUESTION`<br>`sop planner status`<br>`sop planner continue --apply` | Publish a bound executor question, inspect planner state, or apply a recorded planner decision. | Applying a decision records the durable continuation; `sop run` remains the only execution path. Advice is not approval, and unavailable or invalid decisions cannot trigger execution. |
| `sop mcp --planner` | Serve the restricted planner MCP surface. | Exposes status, question retrieval, task-result retrieval, decision listing and bound decision submission. It exposes no approval operation. Without `--start-work`, resumption is interactive and no unattended invocation or notification channel is claimed; with it, the planner can start `sop prompt` / `sop run` (see the next row). |
| `sop mcp [--planner] --start-work` | Also serve the opt-in start-work tools. | Adds `sop_start_work` (start `sop prompt` from prompt text, or `sop run PLAN` from a project PLAN.md, as a detached background job) and the read-only `sop_work_status`. Reports status instead of starting when the same work is running, already ran, or is the active plan; refuses while a task is mid-run or when `sop run` would stop at a plan gate. One started job per project at a time. Never approves a gate. Not allowed with `--status-only`. See [Starting work over MCP](#starting-work-over-mcp). |

| `sop review`                                                                                                   | Review the current working-tree changes with the configured engine.                                                                                                   | `self` (default) asks the agent for structured findings; `open-code-review` runs an external command (`SOP_REVIEW_COMMAND`). The model never decides the verdict: findings whose severity is named in `quality.fail_on` are blocking, and the exit code is non-zero when any remain. With no working-tree changes it reports “no changes to review”.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `sop run [--model-class small\|medium\|large] [PLAN.md \| --task TASK.md]`                                     | The one-command workflow: discover or resolve the planning source, initialize state, compile or generate `.agent-sdlc/plan.json`, create tasks, then drive the graph. | A file argument names an execution PLAN (authoritative for that run); `--task TASK.md` runs a single task through the local lifecycle instead; `--model-class` overrides the model-routing class for this invocation (see [`CONFIGURATION.md`](CONFIGURATION.md)). Each step is idempotent. A stage that declares acceptance checks has them executed and recorded as an objective-level `VERIFIED`/`PARTIAL`/`HOLD` outcome (see the acceptance-checks note below). See the precedence and multi-plan notes below.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `sop prompt [--capability CAP] [--model-class CLASS] [--json] [--deliverable PATH]... [--file PATH \| PROMPT]` | Run an ad-hoc operator prompt through SOP's governed execution (Phase 5.4).                                                                                           | Capability defaults to `plan` (read-only); `plan`, `review`, `diagnose_failure`, and `design_tests` run read-only, while `implement` runs the full governed implementation lifecycle. `--deliverable PATH` (repeatable, `implement` only) declares an explicit repository deliverable projected into the lifecycle; a declared report counts as a change only when its content changes. Routing reuses the `small`/`medium`/`large` classes; provider validation applies when `providers.validate` is on. Runs are written under `.agent-sdlc/runs/prompts/<run-id>/`. See [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `sop report [run-id] [--detailed]`<br>`sop report --decisions [--format text\|json]`                           | Print a concise, informational summary of a run (the latest of either kind by default), or the project-wide decision-telemetry aggregate.                             | Shows stage, provider, gate, per-check validation, findings by severity, and the `## Outcome` acceptance verdict when the stage declared acceptance checks. When model routing or bounded escalation applies, it also shows the `Model routing:` decision and an `Execution attempts:` section with the initial class, each attempt, and the escalation metrics. `--decisions` is an opt-in, read-only, project-wide aggregate of the recorded decisions (model tier, overrides, escalations, JEV invocation, and adapter agreement/confidence/latency/failure metrics) with explicit denominators and coverage; `--format json` emits the deterministic aggregate JSON. It takes no run id, makes no model call, and never changes execution, routing, or approval behavior. The human-readable output opens with a `Model Selection` summary (SMALL/MEDIUM/LARGE counts and one-decimal percentages over the runs with a valid recorded selection, plus an explicit `Missing` count) and then lists each decision metric. The aggregator remains off the production path (only `internal/cli/report_decisions.go` consumes it). The concise default is selected by `output.responses` (default `concise`); `--detailed` overrides it for one invocation, and machine-readable (JSON) output is never affected. See [`../specs/DECISION-TELEMETRY.md`](../specs/DECISION-TELEMETRY.md). |
| `sop commit [TASK.md] [--yes]`                                                                                 | Commit the current changes with a message derived from a task file (no push).                                                                                         | When `human.approval_before_commit` is on (the default), `--yes` is required.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `sop pr TASK.md [--yes]`                                                                                       | Push a task branch and open a pull request.                                                                                                                           | The branch is `task/<id>-<slug>` and the base is the integration branch. It never merges.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `sop mcp`                                                                                                      | Serve tools over the Model Context Protocol on stdio.                                                                                                                 | Backed by the same services as the CLI. Default exposes `sop_status`, `sop_validate`, `sop_review`, and the approval tools (`sop_approval`, `sop_approve`, `sop_decline`). `--status-only` exposes only `sop_status`; `--planner` exposes only the planner surface — `sop_status`, `sop_question_list`, `sop_question_get`, `sop_answer_submit` — and never the approval actions. `--status-only` and `--planner` are mutually exclusive. `--start-work` adds `sop_start_work` and `sop_work_status` to the default or planner surface and is refused with `--status-only`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `sop eval DIR`                                                                                                 | Run a corpus of task files through the lifecycle and report benchmark metrics.                                                                                        | Input is a directory of task files.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `sop status`<br>`sop status --detailed`                                                                        | Show project task state.                                                                                                                                              | Prints one line per task (`<ID> <STATE> <title>`). `--detailed` (or a configured `output.responses: detailed`) adds each task's dependencies and attempt count. See [`STATUS-AND-RECOVERY.md`](STATUS-AND-RECOVERY.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `sop task <id>`                                                                                                | Inspect a particular task.                                                                                                                                            | See [`STATUS-AND-RECOVERY.md`](STATUS-AND-RECOVERY.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `sop resume [task-id]`                                                                                         | Report the next legal action for interrupted work.                                                                                                                    | With no id it resumes the single in-flight task. The same action rule drives `sop run`. See [`STATUS-AND-RECOVERY.md`](STATUS-AND-RECOVERY.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `sop continue --check [PLAN.md] [--json]`                                                                      | Check, read-only, whether the active plan is safe to continue via `sop run`: reconcile before continue.                                                               | Deterministic and model-free: it reconciles the source plan, the compiled plan, and persisted state through the same read-only preview as `sop reconcile --list-changed`, and classifies the result (`RECONCILE_CLEAN`/`RECONCILE_CHANGED`/`RECONCILE_SEMANTIC_CHANGE`/`RECONCILE_FAILED`/`RECONCILE_NONDETERMINISTIC`) with a single next action (`CONTINUE_SAFE`, `HUMAN_REVIEW_REQUIRED`, `APPROVAL_REQUIRED`, `BLOCKED`, `PLAN_COMPLETE`). `--check` is required and the command never mutates state or executes a task; it exits zero only when the next action is `CONTINUE_SAFE`. See the `/sop-continue` skill.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `sop retry <task-id> \| --all [--force]`                                                                       | Requeue a `BLOCKED` task to `PLANNED` so the next `sop run` retries it, or requeue every `BLOCKED` task that still has retry budget at once.                          | `--force` also requeues a task whose `max_attempts` is spent, raising the budget. See [`../specs/RECOVERY.md`](../specs/RECOVERY.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `sop approvals [--json]`                                                                                       | List every task SOP reports at an applicable human approval gate.                                                                                                     | Read-only: it creates no request, resolves nothing, and never infers a gate from a task status, a blocked reason, or prose. A pending request whose task has already completed is reported as not applicable, matching the decision boundary; the text listing names it under "Stale" with the `sop approval supersede` command, and `--json` omits it. See [`../specs/HUMAN-APPROVAL.md`](../specs/HUMAN-APPROVAL.md) §6.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `sop approval <task-id>`                                                                                       | Show SOP's approval request (if any) for a task.                                                                                                                      | Prints the present-or-absent boundary and its fields: kind, target, stage, disposition, reason, evidence, status, and any recorded decision. Reports "Approval: none" when SOP recorded no request, so a task status alone is never treated as a gate.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `sop approval invalidate <task-id> --reason TEXT --by NAME` | Close a `PENDING` approval request whose evidence the revised plan no longer contains. | Eligible only for a `PENDING` request on a task that is not satisfied, and only when SOP holds a task-scoped accepted-change record for that task whose acceptance time is after the request, whose digests describe a real change, and which covers the current definition. This is required for every request, including one that recorded its own digest: a different digest alone never qualifies. Plan-wide `generated_at` and a legacy `reconciled_tasks` entry are not acceptance evidence. A provider failure, a blocked task, or a repaired implementation is never sufficient evidence on its own. The original request and its evidence are preserved verbatim in the append-only approval history, and the head becomes `INVALIDATED` with the reason, actor, plan fingerprints, and both definition digests. It is not an approval and not a waiver: the task is never written, so it is not completed, satisfied, or skipped, and fresh execution and outcome verification are still required. `PENDING` and `DECLINED` requests keep blocking execution. See [`../guides/APPROVALS.md`](../guides/APPROVALS.md#invalidating-a-gate-whose-evidence-is-obsolete). |
| `sop approval invalidate-attested <task-id> --reason TEXT --by NAME --expected-request ID --obsolete-requirement TEXT --replacement-definition TEXT --expect-definition-digest SHA` | Close a legacy `PENDING` approval request on a present-day operator attestation, when no task-scoped accepted-change evidence exists. | Legacy only: the request must be `PENDING`, carry no recorded definition digest, sit on a task that is not satisfied and is named by the plan's reconciliation history, and be a human-boundary request (`Stage WAITING_FOR_HUMAN`), never a failed execution. The obsolete requirement must appear in the request's own reason or evidence and must not appear in the current definition; the replacement definition must appear in it, which is a consistency check only, not proof of equivalence. The current definition digest must equal `--expect-definition-digest` and is re-verified under the project lock. Refused when usable accepted-change evidence exists (use `invalidate`). Records `kind: ATTESTED` with `Decision` nil, `source: UNKNOWN_LEGACY_RECONCILIATION`, the operator statements at their actual time, and preserves the original request verbatim in the append-only history. It changes no task status, attempt, or satisfaction, and fresh verification is still required. An identical replay returns the recorded result and writes nothing; any difference is refused. See [`../guides/APPROVALS.md`](../guides/APPROVALS.md#attested-invalidation-for-legacy-requests). |
| `sop approve [<task-id>                                                                                        | --select] [--by NAME] [--note TEXT] [--run]`                                                                                                                          | Record an approval of the task's active human approval gate.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | Delegates to the approval application boundary and resolves only the gate SOP raised: a `BLOCKED` task is resumed through the domain requeue without spending a retry, and a task already returned to `PLANNED` records the authorization as provenance. It never completes a task and never bypasses validation, review, or the quality gate. With no `<task-id>` (or `--select`) it chooses among the applicable gates at a terminal and fails closed off one; `--run` persists the decision first, then starts the ordinary run. Idempotent. See [`../specs/HUMAN-APPROVAL.md`](../specs/HUMAN-APPROVAL.md). |
| `sop decline [<task-id>                                                                                        | --select] [--by NAME] [--note TEXT]`                                                                                                                                  | Record a decline of the task's active human approval gate.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Preserves the truthful lifecycle state and never manufactures completion. With no `<task-id>` (or `--select`) it chooses among the applicable gates at a terminal and fails closed off one. Idempotent. See [`../specs/HUMAN-APPROVAL.md`](../specs/HUMAN-APPROVAL.md).                                                                                                                                                                                                                                                                                                                                         |
| `sop reconcile <PLAN.md> [--accept-changed <TASK_ID>] [--list-changed] [--json]`                               | Apply an intentional plan change.                                                                                                                                     | Preserves every unchanged task and its history; updates only never-executed tasks; adds and removes unexecuted tasks. `--accept-changed` is repeatable. `--list-changed` reports what a reconciliation would change **without changing anything**, naming the executed tasks that need an approval and the executed tasks the plan removes (which cannot be approved); `--json` makes that listing machine-readable. A reconciliation and its listing are the same diff. See [`../specs/RECOVERY.md`](../specs/RECOVERY.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `sop providers [--models]`                                                                                     | Inspect the configured provider runtimes (read-only).                                                                                                                 | Prints each provider's health and model count; `--models` also lists discovered models with their locality and capabilities. It never mutates state and never prints credentials. See [`../specs/PROVIDERS.md`](../specs/PROVIDERS.md) and [`CONFIGURATION.md`](CONFIGURATION.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `sop version`                                                                                                  | Print the SOP version.                                                                                                                                                | Listed among the available commands.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `sop help`                                                                                                     | Print command help.                                                                                                                                                   | Listed among the available commands.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `sop verify <task-id>`                                                                                         | Print a task’s recorded objective-level verdict and per-criterion evidence (read-only).                                                                               | Prints the recorded `VERIFIED`/`PARTIAL`/`HOLD` outcome, its criteria, and per-criterion `argv`/exit code/output. No state transition; changes no PASS/FAIL semantics. See [`LIFECYCLE-VERDICT-GATE.md`](LIFECYCLE-VERDICT-GATE.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `sop outcome <task-id> [--waive <criterion-id> --reason R --by NAME [--scope S]]`                              | Print a task’s recorded outcome, or record a designated-approver waiver.                                                                                              | Without `--waive`, read-only (same as `verify`). With `--waive`, only a `human.designated_approvers` identity may grant a criterion-scoped waiver; the record carries approver, reason, timestamp, scope, and affected criterion, appended to an append-only audit. An unauthorized/stale/invalid waiver is rejected and never converts an incomplete outcome into a verified one. Gate is default-off (`lifecycle.verdict_gate`). See [`OUTCOME-RECOVERY-WAIVER.md`](OUTCOME-RECOVERY-WAIVER.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `sop outcome <task-id> --accept <criterion-id> --evidence E --by NAME`                                         | Record a manual acceptance decision for a criterion mapped to manual evidence.                                                                                        | SOP-003. Only a `human.designated_approvers` identity may accept, with non-empty evidence, for a criterion the recorded outcome maps to manual evidence (`### Acceptance Mapping`: `- criterion N: manual: <evidence>`). The decision (task, criterion, evidence, approver, timestamp) is appended to `runs/<task>/acceptance_decisions.jsonl`; `outcome.json` is never rewritten. The next derived outcome meets the criterion only with an accepting decision. When a supported recovery advances the task attempt after the reviewed outcome, an append-only `runs/<task>/acceptance_recoveries.jsonl` binding preserves that decision for the same criterion and evidence; a changed criterion or evidence invalidates it. An invalid decision is rejected and nothing is written. |

## Planner question/answer surface

`sop question` and `sop answer` are the CLI surface over the durable
question/decision contract (`internal/question`, `.agent-sdlc/questions.json`).
They let a planner (for example, a ChatGPT conversation connected over the Secure
MCP Tunnel) retrieve a question and submit an answer without ever touching the
human approval boundary.

| Command                                                                                                     | Purpose                            | Behavior                                                                                                                                                                                                                                                                        |
| ----------------------------------------------------------------------------------------------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sop question <question-id>`                                                                                | Read one question and its answers. | Read-only. Prints the binding (task, run, plan revision), body, answers, and audit outcomes.                                                                                                                                                                                    |
| `sop question list [--state STATE] [--json]`                                                                | List questions.                    | Read-only. Defaults to `PENDING`. An unrecognized state fails closed.                                                                                                                                                                                                           |
| `sop answer <question-id> --body TEXT [--by NAME] [--idempotency-key KEY] [--expected-revision N] [--json]` | Submit a planner answer.           | Records the answer and advances `PENDING -> ANSWERED` only: it never applies the answer, approves a task, or resumes execution. The outcome is deterministic — `accepted`, `duplicate`, `stale`, or `rejected` — and every outcome is recorded in the append-only audit ledger. |

The binding is validated against **authoritative current state**, not merely the
caller's identifiers: the project identity must match; the bound task must exist in
SOP's live task store; the bound run must still be the task's current run and not
superseded; the repository identity must match; and the plan revision must still
match the live plan provenance (`.agent-sdlc/plan.meta.json`). A stale binding refuses
the answer and preserves the prior record even when every caller-supplied identifier
looks plausible. The repository identity is a **stable** fingerprint (the repository
root), not a working-tree hash, so ordinary edits and commits never invalidate a
question.

Each submission is **one atomic transaction**: validation, duplicate/conflict
detection, answer insertion, `PENDING -> ANSWERED`, and the audit outcome run under a
single cross-process lock, so two competing submissions cannot both observe `PENDING`.
Question and answer records are never mutated on a non-accepted outcome, but every
attempt is durably audited. Where no advisory file lock is available (Windows), durable
writes fail closed with an unsupported-operation error while reads remain available.

Over MCP, `sop mcp --planner` exposes the planner-only surface (`sop_status`,
`sop_question_list`, `sop_question_get`, `sop_answer_submit`) and never the human
approval actions; `sop mcp --status-only` stays status-only.

## Setting up the planner

One command builds a current `sop`, points the planner MCP wrapper
(`.run/tunnel/sop-mcp-planner.sh`) at a project with `--start-work`, checks it locally,
and reports whether the Claude desktop app runs that wrapper:

```text
sop-setup-planner [--no-build] [--no-start-work] [--register-claude] [--dry-run] [PROJECT_DIR]
make setup-planner [PROJECT=DIR] [SETUP_PLANNER_ARGS=--register-claude]
scripts/setup-planner.sh [...]
```

`PROJECT_DIR` defaults to the agentic-sop checkout. `sop-setup-planner` is installed by
`./install.sh --planner-setup` (or `--all`) as a shim that runs the checkout's
`scripts/setup-planner.sh`. The wrapper work is delegated to
`scripts/planner-tunnel-point.sh --build --start-work`, whose local check refuses a
binary that does not serve `sop_start_work` and leaves the live wrapper unchanged.
`--register-claude` adds `"sop-planner": {"command": "<wrapper>"}` to the Claude desktop
config after backing it up, keeps every other entry, and never overwrites a different
`sop-planner` entry. The command never restarts the Claude desktop app or the ChatGPT
tunnel; it prints how. The ChatGPT tunnel runs the same wrapper.

## Starting work over MCP

`sop mcp --start-work` (alone or with `--planner`) adds two tools. They are opt-in;
no default surface changes.

- `sop_start_work` takes **either** `prompt` (prompt text, for example the last
  prompt planned in a chat) with an optional `capability` (default `plan`, the same
  read-only default as `sop prompt`; `implement` runs the governed lifecycle), **or**
  `plan_path` (a PLAN.md inside the project). It launches the same command an
  operator would type — `sop prompt --capability CAP --file <job>/prompt.md` or
  `sop run PLAN` — as a detached child of the sop binary, so the work outlives the
  MCP connection. The prompt text is written to a file and passed as data, never as
  shell text. `plan_path` is confined to the project against symlink resolution.
- `sop_work_status` (read-only) reports one job by `job_id`, or the five most recent
  jobs, with state, exit code, a log excerpt, and the project's task status.

Before launching, `sop_start_work` checks what SOP already holds and returns a status
document instead of starting when:

| Outcome | When |
| --- | --- |
| `already_running` | The same work (same prompt text and capability, or same plan path and content) is running. |
| `busy` | Different MCP-started work is still running in this project. Only one job runs at a time. |
| `task_in_progress` | A task is in a mid-run state (`IMPLEMENTING`, `REVIEW`, `FIX_REQUIRED`, …), so a `sop run` started elsewhere may be working. Lists the tasks. |
| `plan_blocked` | `sop run PLAN` would stop at a plan gate: a different plan with unfinished work is active, or the active plan changed. Returns SOP's own message. |
| `in_memory` | The plan is SOP's active plan with the same content. Includes the task status. |
| `already_ran` | The same work ran before (completed, failed, or ended). |
| `started` | None of the above, or `rerun: true` was passed and the work is not running. |

`task_in_progress` is a stand-in for a shared run lock: SOP cannot tell a live run
from one that was interrupted mid-task, so it fails closed. If nothing is running,
recover from a terminal with `sop resume` and `sop run`. `rerun` skips neither this
check nor `plan_blocked`.

`rerun: true` starts the work again (for a held plan, `sop run` resumes its unfinished
tasks) but never starts a second copy of running work. Each job is recorded under
`.agent-sdlc/runs/mcp-work/<job-id>/` (`job.json`, written before launch; `prompt.md`; `output.log`; and
`exit.json` once the server observes the exit). A job whose process is gone with no
exit record — the server stopped first — is reported as `ended`.

Starting work never approves anything: the launched command stops at the human
approval boundary exactly as it does in a terminal, and approvals stay with
`sop approve` or the default surface's approval tools.

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

## `sop run`: new plans need a branch or worktree

A new plan (no `.agent-sdlc/state.db` yet) is refused on `main` in the primary
checkout. `sop run` prints the command to create a worktree from `main`:

```bash
git worktree add -b sop/<plan-slug> ../<repo>-<plan-slug> main
```

Run `sop run` again from that worktree. Resuming a plan with existing state, any
other branch, and any linked worktree run in place.

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
`review.json`, `report.md`, `report.json`, `metrics.json`, `trace.json`,
`state.json`). `trace.json` is the versioned structured run trace (schema 4): the run's
execution identity (the routing decision versus the actual execution target), the
observed iterations, the verification evidence, the termination, structured
progress signals (discovery, repository mutation, verification, state transition)
that separate progress from mere activity, any bounded strategy changes (`replans[]`),
and the Context Engine's supplied-context summary (`context`: items, files, bytes,
truncated, sources). `sop report` renders a concise `Trace:`
block, a `Progress:` summary, and a `Budgets:` block (the deterministic execution
limits that applied) from it. It is observational only — nothing reads
it back to drive a decision — and it is absent for older runs. A
failing check or blocking finding is sent back to the agent and re-checked, up
to `quality.max_fix_cycles` times; exhausting the budget yields `NEEDS_HUMAN`.
Plan lifecycle is explicit. `sop plan activate <PLAN.md>` validates and installs a
plan's task graph as the active plan and stops — it never executes the first task,
so activation and execution are separate operator actions; `sop run` then executes
the next runnable task. `sop plan supersede <PLAN.md>` explicitly replaces the active
plan, even one with unfinished work, archiving the previous plan as `SUPERSEDED`
(under `.agent-sdlc/archive/<plan-id>/`, with a `lifecycle.json`) while preserving its
task states, run evidence, and approvals verbatim — supersession never fabricates a
PASS or an approval. `sop plan complete` archives the active plan as `COMPLETE` once all of
its tasks are satisfied and releases it without installing a successor; it fails closed
while any task is unresolved and never fabricates work. `sop plan historicalize [<PLAN.md>]` is the general, deterministic historicalization of a completed (or explicitly disposed) plan: it evaluates readiness (task counts by state, unresolved tasks, pending approvals, and missing verification), archives the plan under `.agent-sdlc/archive/<plan-id>/`, and removes it from ACTIVE selection. `--check` reports readiness without mutating; `--disposition SUPERSEDED` records an explicit disposal of unfinished work; it is idempotent (`ALREADY_HISTORICALIZED`) and refuses a plan that changed since readiness was evaluated. With `SOP_HISTORICALIZE_ON_COMPLETION=true`, `sop run` performs it as the final stage of a successful run whose plan is fully satisfied and eligible (never across a pending approval gate). `sop status` prints the active plan's identity and state before
the task list. `sop task complete <task-id> --external [--commit <sha>]` records the completion of a task
whose work was performed and merged **outside** the SOP execution — an explicit, model-free
operator action. It fails closed unless the evidence establishes completion: the task exists
in the active plan and is not already complete, its dependencies are satisfied, the claimed
`--commit` (default HEAD) is contained by HEAD, and the configured validation passes. It
records the task in the local terminal state (`LOCAL_DONE`) and writes provenance
(`.agent-sdlc/runs/<task-id>/external-completion.json`); it fabricates no attempt, no model
run, and no approval, and it never touches retry, human-boundary, or approval state. It also
discharges the planner continuations that the completion makes unsatisfiable, before the
completion is recorded, so a recorded completion leaves no actionable continuation behind. A
continuation bound to the task, whose question matches the live plan revision and whose run
epoch matches the live run, is recorded `EXTERNALLY_COMPLETED` — never `COMPLETED`, because no
stage ran. A continuation whose bindings provably belong to another task, another plan
revision, or another run epoch is stale: it is reported and left untouched, because it can
never be resumed against this run. An unprovable binding (a missing question, or missing plan
or run-epoch proof) or a continuation held by a live worker refuses the completion instead, so
possibly-owed work is never stranded. A continuation and its question that both predate
run-epoch binding — the legacy empty-epoch case, as with `cont:q-1:` — are accepted on the plan
binding. Question, answer, and outcome history is preserved verbatim. Discharge is idempotent,
so retrying after a partial discharge skips the already-terminal records untouched; a
continuation-store failure aborts the completion rather than recording it with an owed
continuation.
Deterministic lexical retrieval (CTX-003) ranks the Structural Repository Index against
query terms with a fixed BM25 ranker: `sop retrieve <term>...` prints ranked candidates
(`rank`, `score`, `source`, `id`). Ranking is deterministic — scores are aggregate term
statistics and ties break by stable candidate identity, so filesystem iteration order never
affects the result. It is model-free and read-only and writes no artifact; the ranker is
also available programmatically (`internal/retrieval`) as a Context Engine input source.
`sop run` stops at the human gate and never commits, pushes, or merges. When JEV
is enabled (`quality.jev.enabled: true`), an optional read-only analysis stage
runs after validation and review and feeds the same gate and fix loop.

With `SOP_MODEL_ROUTING_ENABLED=true` each task's model class is chosen from typed
evidence and recorded in `routing.json`. With `SOP_MODEL_ESCALATION_ENABLED=true` a
task that fails the gate may be retried on the next larger class in the same
invocation (bounded by `SOP_MODEL_MAX_ESCALATIONS`, default 2). With
`SOP_MODEL_REPLAN_ENABLED=true` a recoverable failure may instead change strategy
once on the same class (bounded by `SOP_MODEL_MAX_REPLANS`, default 1). Each attempt
is recorded under `attempts/`. All three are OFF by default and apply equally to an
`implement` prompt. See
[`CONFIGURATION.md`](CONFIGURATION.md) and [`../specs/RECOVERY.md`](../specs/RECOVERY.md).

## `sop run`: acceptance checks and the outcome artifact

A plan stage may declare **operator-owned deterministic acceptance checks** — the
verifiable half of acceptance — as an `### Acceptance Checks` sub-section in a rendered
plan or `- **Acceptance check:** <command>` inline lines in a task-list plan (one command
per line, optionally backtick-wrapped). When a scheduled task's stage declares checks,
`sop run` executes each **exactly once** as a SAFE argument vector — never a shell — and
records an objective-level verdict beside the gate summary, so a task's
`PASS`/`LOCAL_DONE` is never equated with verified objective completion. A non-SAFE
command, a rejected flag form (`-exec`, `-toolexec`, `-vettool`, `-o`, `-overlay`,
`-modfile`), `go fmt`, or `git -c` / `--config-env` is a boundary (`HOLD`) and is never
executed; a `go test` check is judged from structured `go test -json` events from a
single execution (met only on exit 0 with at least one test-level pass). The verdict is
`VERIFIED`, `PARTIAL`, or `HOLD`. The full rules are normative in
[`../specs/EXECUTION.md`](../specs/EXECUTION.md) §6b.

The verdict is written to `.agent-sdlc/runs/<task>/outcome.json`, with one entry per
criterion (`ID`, `Required`, `Boundary`, `Met`, `Description`, `Command`, `Evidence`, and
— for an executed check — `Argv`, `ExitCode`, and a bounded `Output` excerpt):

```json
{
  "status": "PARTIAL",
  "criteria": [
    {
      "ID": "check-1",
      "Required": true,
      "Boundary": false,
      "Met": false,
      "Description": "acceptance check: go vet ./doesnotexist/",
      "Command": "go vet ./doesnotexist/",
      "Evidence": "acceptance check \"go vet ./doesnotexist/\" failed (exit 1): directory not found",
      "Argv": ["go", "vet", "./doesnotexist/"],
      "ExitCode": 1,
      "Output": "directory not found"
    }
  ],
  "reasons": ["required criterion check-1 not met: …"]
}
```

The run report renders the same evidence in its `## Outcome` section:

```text
## Outcome

- Status: `PARTIAL`
- `check-1` not met — go vet ./doesnotexist/ (acceptance check "go vet ./doesnotexist/" failed (exit 1): directory not found)
  - argv: `go vet ./doesnotexist/`
  - exit: 1
  - output: directory not found
```

Inspect a run's verdict with `sop report <task-id>` (see the `sop report` row above).

## `sop run`: caller observations and report deliverables

For tasks that need operator-observed facts outside the native agent's repository
boundary, `sop run` accepts optional `SOP_TASK_INPUTS`: a JSON object mapping task
IDs to redacted observation text (32 KiB maximum). For example:
`{"CLOSE-001":"Observed sibling HEAD: ...; captured at ..."}`. Only the matching
task receives that context, in planning, implementation, review/JEV and fixes.
Do not include credentials. The observations are data: they grant no commands,
completion, approval, or additional iterations. Mutation, validation and review
remain required. Unset input preserves normal execution.

A task's `Deliverables` section is retained in its execution request. An exact
Markdown report declared under a `docs/reports/` subdirectory is task-owned
implementation evidence when actually mutated by the invocation. Unrelated
reports, SOP-generated `docs/reports/<plan-id>.md`, and `.agent-sdlc/` remain
excluded. Pre-existing dirty reports do not establish a new mutation.

## `sop prompt`

`sop prompt` runs an ad-hoc request through the same governed machinery a task
uses. It is the entry point the SOP skill calls.

```bash
sop prompt "Explain this project's architecture"          # default: --capability plan
sop prompt --capability review "Review internal/provider for architectural issues"
sop prompt --capability diagnose_failure --file prompts/debug.md
sop prompt --capability implement "Add caching to provider discovery"
sop prompt --capability implement --deliverable docs/reports/baseline.md "Capture the baseline report"
```

The first three are read-only: the model produces text and the repository is never
modified. `--capability implement` runs the governed implementation lifecycle
(plan → implement → validate → review → gate → fix → approval), exactly as a planned
task does; it cannot run on a provider that does not declare `IMPLEMENT`. When model
routing, bounded escalation, and strategy replanning are enabled it follows the same
path as a task, including a retry on the next larger class or a strategy change on
the same class, and per-attempt records under
`.agent-sdlc/runs/prompts/<run-id>/attempts/`.

`--deliverable PATH` (repeatable, `implement` only) declares an explicit repository
deliverable and projects it into the lifecycle's task spec, reusing the same
report-deliverable mechanism a scheduled task uses. A declared report under
`docs/reports/` counts as a repository change only when its content actually
changes; an unchanged report, a merely announced output, an unrelated dirty file,
and SOP's generated run reports and state artifacts still fail the no-changes check.
The path must be an explicit, clean, repository-relative file inside the project:
traversal, absolute or external paths, directory-wide declarations (for example
`docs/reports`), and SOP's own state tree (`.agent-sdlc/...`) are refused.

`--json` prints the structured result document (`work_item_id`, `kind`,
`capability`, `status`, `routing`, `report_path`, `result_path`, `result`). Every run
is recorded under `.agent-sdlc/runs/prompts/<run-id>/`; inspect one with
`sop report prompts/<run-id>` (or just `sop report` for the latest run of either
kind), which works for a read-only run too.

The shipped command aliases (`/sop-plan`, `/sop-review`, `/sop-diagnose`,
`/sop-test`, `/sop-implement`, `/sop`, `/sop-prompt`) are thin wrappers over this
command: each fixes a capability and calls `sop prompt`. Install them with
`./install.sh --skills zed` (Zed), `./install.sh --skills claude` (Claude Code), or
`./install.sh --all` (the CLI, every supported agent present, and the Claude plugin); see
[`../guides/ZED-SKILLS.md`](../guides/ZED-SKILLS.md) and
[`../guides/CLAUDE-SKILLS.md`](../guides/CLAUDE-SKILLS.md).

## Recovery commands

`sop resume`, `sop retry`, `sop reconcile`, and `sop continue --check` operate on
persisted state for interrupted, blocked, or changed work. Their next-action,
requeue, reconciliation, and reconcile-before-continue semantics are in
[`docs/specs/RECOVERY.md`](../specs/RECOVERY.md) and summarized in
[`STATUS-AND-RECOVERY.md`](STATUS-AND-RECOVERY.md).

## Context efficiency caches and memory

The Phase 8 context-efficiency facilities are opt-in and correctness-first.

`sop validate --cache` consults the CTX-007 Verification Cache before running the
configured checks. The cache is keyed on the repository identity, the command set,
the relevant environment, and the SOP version; it is only consulted for a clean
working tree, and any difference is a miss, so a stale result is never reused.

`sop prompt --cache` (read-only capabilities only) consults the CTX-008 Prompt Result
Cache, keyed on the compiled prompt, the model identity, and the compiler version. A
mutating capability is never cached, and a cache hit is only a replay of model output,
never task success.

`sop memory add --decision <text> --reason <text> [--scope <scope>] [--source <id>]
[--evidence <text>]` records a durable engineering decision, and `sop memory list`
prints the decisions applicable to the current repository identity; a decision recorded
under another state is reported as stale and is never applied.

The artifacts live under `.agent-sdlc/context/`. Default behavior is unchanged: no
cache is consulted and no memory is applied unless these flags are used.

Decision memory can also inform the run context: setting `context.decision_memory: true`
includes applicable decisions in the context supplied to the implementation agent. It is
off by default, and a decision recorded under another repository state is never applied.
