# SOP Boundary

**Type:** Architecture documentation

## Purpose

Defines the ownership boundary between SOP and everything it drives — agents,
models, and optional capabilities such as OpenJEV. It records the invariant that
makes SOP deterministic: **SOP owns orchestration and lifecycle state; agents do
bounded work; models are replaceable reasoning engines.**

## Related Documentation

- [OVERVIEW.md](OVERVIEW.md) — system structure and components.
- [../PRD.md](../PRD.md) — product principles and requirements.
- [../specs/WORKFLOW.md](../specs/WORKFLOW.md) — state transitions and scheduling.
- [../specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md) — the agent boundary.
- [../specs/OPENJEV.md](../specs/OPENJEV.md) — the JEV analysis boundary.
- [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) — human authority.
- [../specs/SECURITY.md](../specs/SECURITY.md) — repository safety and command boundaries.

## The Core Rule

```text
The LLM creates content.

The application controls the process.
```

A model proposes; SOP validates and decides. SOP never lets a model decide
whether its own output is valid, whether a task advances, or whether a gate
passes.

## What SOP Owns

SOP is the orchestration and lifecycle authority. It owns:

- workflow state and legal state transitions (see [../specs/WORKFLOW.md](../specs/WORKFLOW.md));
- dependency scheduling and task selection;
- persistence — `.agent-sdlc/state.db` is the durable source of truth;
- validation, review, and the deterministic quality gate;
- retry and fix-loop budgets, and bounded autonomy;
- git branch/commit/PR state and CI/merge gates;
- human-approval requirements;
- environment bootstrap and recovery.

## What Agents Own

Agents perform **bounded work** inside the boundaries SOP defines: planning,
writing tests, implementing, reviewing, and fixing. An agent acts on a structured
outcome and hands control back; it does not decide workflow state. See
[../specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md).

## What Models and Providers Are

A model is a replaceable reasoning engine selected by a provider and wrapped by a
harness. SOP depends on small interfaces, not provider SDKs, so the model or
provider can change without rewriting planning or orchestration:

```text
Provider ≠ Agent Harness ≠ Model
```

## State Ownership

Markdown is for people; structured data is for executable state.

```text
Human                         Machine

PRD.md
PLAN.md          ←──────→     plan.json
                              tasks
                              dependencies
                              attempts
                              SQLite (.agent-sdlc/state.db)
```

The orchestrator MUST NOT depend on parsing human documentation to understand its
own state, and a consumer MUST NOT keep a parallel copy of SOP's task state.
SOP is the single source of truth for workflow state.

## Human Authority

Consequential actions remain governed by SOP policy and explicit human gates
(`human.approval_before_commit`, `NEEDS_HUMAN`, `WAITING_FOR_HUMAN`). No agent,
model, or optional capability may bypass a human gate. See
[../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md).

## Optional Capabilities

Optional capabilities (OpenJEV, external review, compression, parallelism) are
additive and MUST NOT gate core work. A failure in an optional capability must not
turn completed work into `BLOCKED` or block a merge; it is recorded and the
workflow continues. See [../specs/OPENJEV.md](../specs/OPENJEV.md).
