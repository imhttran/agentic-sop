# Recovery

**Type:** Normative specification

## Purpose

This document is the normative specification for bounded retries and durable
recovery: the attempt budget, progress-based accounting, requeueing, resume
semantics, SQLite as the durable source of truth, restart reconciliation, and plan
provenance, handoff, and archive. The state graph and remediation rules are owned by
[WORKFLOW.md](WORKFLOW.md); how a run executes is owned by [EXECUTION.md](EXECUTION.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §5 Task State
  Machine, §6 Retry Architecture, §12 Data Model.
- [../PRD.md](../PRD.md) — FR-17 Bounded retry, FR-18 Blocked state, FR-23 Resume,
  §9 Recoverability and Idempotency, §12 Sources of Truth.
- [WORKFLOW.md](WORKFLOW.md) — state vocabulary and terminal states.
- [EXECUTION.md](EXECUTION.md) — `sop run` preparation and lifecycle.
- [MODEL-ROUTING.md](MODEL-ROUTING.md) — model-class selection (Phase 3.5).
- [QUALITY.md](QUALITY.md) — the gate and fix-loop budget.
- [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md) — the human boundary.
- [../reference/STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md) and
  [../reference/CLI.md](../reference/CLI.md) — command reference.
- [../README.md](../README.md) — documentation index.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Bounded Retries and the Attempt Budget

- Every retry loop MUST be bounded by `max_attempts`, whose default MUST be **3**.
- A `needs_human` outcome MUST NOT be terminal: SOP requeues the task to `PLANNED`
  so the same `sop run …` retries it automatically.
- A retry that changes its outcome (makes progress) MUST spend one attempt; once
  the budget is spent the task MUST become `BLOCKED` instead of looping.
- A retry that reproduces the **same** outcome is "no progress" and MUST spend
  nothing: the task stays `PLANNED` and a later run still retries it, so a boundary
  resolved later is picked up without a manual unlock. It is reported as
  `NEEDS_HUMAN (no change since the previous attempt: …)`.
- A hard `FAIL` — including a claimed change with none produced — MUST leave the
  task `BLOCKED`.

## 2. Explicit Requeue

- `sop retry <id>` MUST requeue a `BLOCKED` task.
- `sop retry --all` MUST requeue every `BLOCKED` task that still has budget.
- `sop retry <id> --force` MUST be available for a task whose budget is spent; it
  raises `max_attempts` rather than requiring `state.db` surgery or a fresh start.
- A retried task SHOULD be handed the previous attempt's outcome as context (a
  `# Previous attempt` section in the implement request), so the agent can address
  the blocker instead of repeating the request that stopped it.

## 3. Attempt Records

Each attempt SHOULD record its operation, start/end, result, failure summary,
logs/artifacts where practical, agent action, and next state; see
[WORKFLOW.md](WORKFLOW.md) §2 and [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) §12.

When bounded model escalation or strategy replanning is enabled (§8, §9), SOP
additionally persists one **execution-attempt record** per lifecycle attempt as non-secret diagnostic
evidence under `.agent-sdlc/runs/<run>/attempts/NNN.json` — `<run>` is the task run,
or `prompts/<id>` for an `implement` prompt (§8). Each record names the
attempt number, the model class, provider, model, and locality the attempt ran on,
the deterministic reason its class was chosen, the result, the failure stage, and
the recovery action SOP applied afterwards. Attempt records:

- MUST NOT carry a credential (API key, token, authorization header, provider
  credential);
- MUST NOT be read back to drive a decision;
- MUST NOT replace the initial routing decision (`routing.json` records what the
  router first chose and MUST NOT be overwritten by an escalated attempt);
- MUST NOT create a second source of task state.

## 4. Resume Semantics

- `sop run` MUST resume an in-flight (interrupted) task from the same persisted
  state → next-action rule that `sop resume` reports (`resume.ActionFor`), instead
  of stopping with `ACTIVE_TASK`.
- A task whose gates already passed MUST be completed locally without re-invoking
  the agent.
- `ACTIVE_TASK` MUST still prevent starting a _different_ task; zero or multiple
  in-flight tasks MUST be reported as an actionable error naming the task and the
  recovery command, never guessed.

## 5. Durable Source of Truth

- SQLite (`.agent-sdlc/state.db`) MUST be the durable source of truth for task
  execution state — the database, **not** model conversation history.
- Markdown MUST NOT be the only source of runtime state. Runs are stored on the
  filesystem under `.agent-sdlc/runs/<id>/` so they stay readable without the tool.
- The schema MUST be versioned (`PRAGMA user_version`) so migrations add columns without losing data.

## 6. Restart and Reconciliation

- After a process interruption SOP MUST be able to restart and determine where it
  left off from persisted state.
- `sop resume` MUST reconcile a task's persisted status with the resources that
  actually exist and report the next legal action, recovering a lost state write
  without duplicating a branch or pull request. Idempotent resume MUST NOT create
  duplicate branches, pull requests, commits, or task records.

## 7. Plan Provenance, Handoff, and Archive

- Each plan MUST carry its own identity (source path, `source_sha256`, and a plan
  id), so SOP never mixes tasks from two plans.
- When a different plan is requested while another is active, SOP MUST hand off
  automatically once every task in the active plan is complete, archiving the
  completed plan's records under `.agent-sdlc/archive/<plan-id>/`; if the active
  plan still has unresolved work, SOP MUST stop with an actionable `NEEDS_HUMAN`
  instead of discarding history.
- If the active plan's own PLAN file changed after its task graph was created,
  `sop run` MUST stop with `NEEDS_HUMAN: plan changed …` rather than silently
  rebuilding the graph. `sop reconcile docs/PLAN.md` MUST apply the change: it
  preserves every unchanged task and its history, updates only tasks that have
  never executed, adds and removes unexecuted tasks, and stops with `NEEDS_HUMAN`
  (naming the task and the difference) when an executed task's definition changed
  or was removed. It MUST NOT delete `.agent-sdlc/state.db` or discard run history.
- `sop reconcile docs/PLAN.md --accept-changed <id>` MUST replace a changed executed
  task's definition while preserving its lifecycle state, attempts, and history, and
  the approval MUST be recorded in `plan.meta.json`. The flag MUST be repeatable, and
  every changed executed task MUST be named, so one approval never silently covers another.
- `sop reconcile docs/PLAN.md --list-changed` MUST report what a reconciliation would
  change **without changing anything**: it MUST name every changed executed task (whose
  approval `--accept-changed` would take) and every executed task the requested plan
  removes (which cannot be approved and MUST be kept or completed first). It MUST NOT
  write the task graph, the machine plan, or the provenance, and MUST NOT record an
  approval. The listing and the reconciliation MUST be the same diff, so a preview can
  never disagree with what applying it does.

## 8. Bounded Model Escalation

Phase 5 adds a deterministic, bounded recovery action above the existing requeue and
fix loops: after an attempt fails a quality gate, SOP MAY retry the task on the next
larger model class in the same invocation. The full policy is owned by
`internal/recovery`; model-class selection remains owned by
[MODEL-ROUTING.md](MODEL-ROUTING.md).

The policy applies wherever the governed lifecycle runs: a task, and an `implement`
prompt, which runs the same lifecycle ([PROMPT-EXECUTION.md](PROMPT-EXECUTION.md) §8).
A read-only prompt is a single bounded call with no quality gate, so escalation never
applies to it.

- Escalation MUST be OFF by default (`SOP_MODEL_ESCALATION_ENABLED` unset and
  `models.escalation_enabled` absent), so an installation that does not opt in
  behaves exactly as before.
- The ladder MUST be one-way and MUST NOT wrap: `small → medium → large → human`.
  There is no class above `large`, and an attempt MUST NOT be downgraded. In
  particular, an escalated attempt MUST run the class the ladder selected: the model
  layer's fallback policy (which MAY resolve a different class for ordinary
  selection) MUST NOT be used for recovery, and a resolution that yields another
  class MUST be refused rather than silently accepted.
- Escalation MUST be bounded by `models.max_escalations` (default **2**):
  `small → medium → large` is exactly two escalations, after which the existing
  human/terminal boundary applies. `0` disables escalation on either layer.
- The decision MUST be a deterministic function of typed evidence only (reused from
  `internal/failure`), never of agent prose or of a class a model proposed.
- A safety, approval, destructive-operation, or invalid-plan boundary MUST NOT be
  escalated: risk policy outranks model escalation. A transient
  provider/infrastructure failure MUST retry the same class, and unfinished but
  productive work MUST continue, rather than spend a larger model.
- An attempt MUST be escalated only when the failure classifier produced an
  authoritative classification for its gate failure. SOP's OWN deterministic verdict
  that a mutating invocation claimed success without changing the repository MUST be
  classified (`NO_CHANGES_PRODUCED`, an implementation failure), so it MAY be
  escalated like any other. A failure with no authoritative classification at all
  MUST NOT be escalated; it keeps the existing human/block path (fail closed).
- Every escalated selection MUST be resolved and validated (Phase 4) before it runs,
  and MUST be built and capability-guarded before use: the selected model MUST equal
  the executing model. If the escalated class cannot be resolved, validated, or built,
  escalation MUST stop and the existing recovery path MUST apply — SOP MUST NOT
  silently continue on the previous model.
- A manual `--model-class` override MUST disable automatic escalation: the
  operator's explicit class is never silently replaced.
- The escalated attempt MUST receive bounded context from the failed attempt (the
  previous class/model, the failure stage, the deterministic reason, and bounded
  validation/review evidence) rather than restarting from zero.
- Escalation MUST NOT grant authority: it selects a stronger model only, and MUST NOT
  bypass approval, safety, validation, review, or quality gates.

## 9. Bounded Strategy Replanning

Phase 7 adds a second deterministic, bounded recovery action: after an attempt fails
a quality gate with a **recoverable** implementation failure, SOP MAY change
**strategy** — the approach — once, on the **same** model class, within the same
bounded attempt loop. The policy is owned by `internal/recovery` (`ActionReplan`).

Replanning MUST stay distinct from the other recoveries:

- **retry** re-runs the same strategy on the same class;
- **replan** changes the strategy before another attempt, on the same class;
- **escalate** changes the execution resource (a larger class);
- **needs-human** and **block** end automated recovery.

- Replanning MUST be OFF by default (`SOP_MODEL_REPLAN_ENABLED` unset and
  `models.replan_enabled` absent), so an installation that does not opt in behaves
  exactly as before.
- Replanning MUST be bounded by `models.max_replans` (default **1**). `0` disables it
  on either layer; a negative value MUST be rejected. The attempt loop's own
  backstop (`maxEscalationAttempts`) additionally bounds total attempts, so a replan
  MUST NOT multiply the run's execution envelope.
- Only a genuinely recoverable implementation failure MAY replan — the same
  condition the escalation policy already treats as recoverable. A `BLOCK`
  (including `IMPLEMENT_NO_PROGRESS`), a `NEEDS_HUMAN` boundary, a
  safety/approval/destructive boundary, an invalid plan, a transient
  provider/infrastructure failure, and a failure with no authoritative
  classification MUST NOT replan; the existing disposition applies. Replanning MUST
  NOT be used to obtain more budget.
- A replan MUST keep the model class: it MUST NOT change the provider or model,
  increase a budget, expand a tool permission, bypass validation, bypass review, or
  bypass human approval. The model MUST NOT be able to authorize or extend its own
  replan; the harness owns eligibility and the bound. AGENT-004's execution limits
  are per-invocation; a replan runs a new bounded invocation like any other attempt,
  and the fixed attempt-loop backstop bounds the total, so the combined envelope
  stays finite.
- A replanned attempt MUST receive bounded context from the failed attempt (a
  `# Replan` instruction and the deterministic failure evidence) rather than
  restarting from zero, so the model changes approach instead of repeating it.
- Replanning and escalation MAY compose: with both enabled, a recoverable failure
  MAY replan once on the same class and then, if it fails again, escalate.
- The replan MUST be recorded observationally: `trace.json` (schema 4) records each
  bounded strategy change under `replans[]` (sequence, reason, from/to attempt), and
  the attempt record carries the `replan` action. Neither is read back to drive a
  decision.

## 10. See Also

- [MODEL-ROUTING.md](MODEL-ROUTING.md) — class selection and the routing table.
- [PROVIDERS.md](PROVIDERS.md) — provider capability and availability evidence.
- [QUALITY.md](QUALITY.md) — the gate and the bounded fix loop.
- [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md) — the human boundary the ladder ends at.
- [WORKFLOW.md](WORKFLOW.md) — state vocabulary and terminal states.
- [../reference/EVALUATION.md](../reference/EVALUATION.md) — the deterministic
  evaluation harness and the `replans` expectation.
