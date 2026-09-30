# SOP Documentation

Index of the SOP documentation tree. Each entry has a one-sentence description so
a human or an AI agent can decide what to read. New here? Start with
[guides/GETTING-STARTED.md](guides/GETTING-STARTED.md).

## How Documentation Is Organized

```text
requirements/   What are we building and why?
architecture/   How is the system structured?
specs/          How MUST the system behave?
plans/          What work needs to happen?
reference/      What are the exact commands, states, and values?
guides/         How do I use or develop it?
history/        What happened previously?
```

Authority flows **requirements → architecture → specs → plans → SOP execution**.
Specifications define required behavior; plans MUST NOT override them; historical
documents provide traceability only. The authoritative top-level description of the
ownership model is [architecture/SOP-BOUNDARY.md](architecture/SOP-BOUNDARY.md).

Where new documentation belongs:

- product intent / requirements → `requirements/` (or a new section of [PRD.md](PRD.md))
- structure, components, ownership → `architecture/`
- normative behavior (MUST/SHOULD/MAY) → `specs/`
- implementation work → `plans/`
- commands, config, states, values → `reference/`
- task-oriented instructions → `guides/`
- completed/superseded artifacts → `history/`

## Getting Started

- [../README.md](../README.md) — project landing page: what SOP is and a minimal quick start.
- [guides/GETTING-STARTED.md](guides/GETTING-STARTED.md) — install the CLI and run SOP for the first time.
- [guides/PROJECT-SETUP.md](guides/PROJECT-SETUP.md) — the explicit setup path: init, PRD, plan, tasks, inspect.

## Requirements

- [PRD.md](PRD.md) — **the canonical product requirements** (goals, functional requirements, constraints, success criteria).
- [requirements/PRD-JEV.md](requirements/PRD-JEV.md) — the JEV workstream's product requirements (problem, vision, decision layer).
- [requirements/PRD-Phase-3-OpenJEV.md](requirements/PRD-Phase-3-OpenJEV.md) — Phase 3 product requirements: the early JEV task-triage and pre-execution checkpoints. The document's own `Status: Proposed` header describes its intent at authoring time; the checkpoints it specifies are **implemented** (see [plans/PLAN-Phase-3-OpenJEV.md](plans/PLAN-Phase-3-OpenJEV.md) and [specs/OPENJEV.md](specs/OPENJEV.md) §18). Items this PRD records as future work remain **proposed** and are listed in [specs/OPENJEV.md](specs/OPENJEV.md) §17.

## Architecture

- [architecture/OVERVIEW.md](architecture/OVERVIEW.md) — components, responsibilities, data model, and the state-machine/retry/TDD/review/CI/security design.
- [architecture/SOP-BOUNDARY.md](architecture/SOP-BOUNDARY.md) — what SOP owns vs what agents, models, and optional capabilities own; the single-source-of-truth rule.

## Specifications

Normative behavior (MUST / MUST NOT / SHOULD / SHOULD NOT / MAY).

- [specs/WORKFLOW.md](specs/WORKFLOW.md) — task state vocabulary, legal transitions, remediation, scheduling, and plan handoff.
- [specs/TASK-LIFECYCLE.md](specs/TASK-LIFECYCLE.md) — the per-task lifecycle stages, TDD rules, and Git naming conventions.
- [specs/EXECUTION.md](specs/EXECUTION.md) — `sop run`: planning-source discovery, change detection, artifacts, verify-first, and parallelism.
- [specs/AGENT-PROVIDER.md](specs/AGENT-PROVIDER.md) — the harness/provider/model boundary, configuration precedence, and structured outcomes.
- [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md) — the model-class layer: classes, precedence, the deterministic per-task router, and the routing boundary (opt-in, OFF by default).
- [specs/VALIDATION.md](specs/VALIDATION.md) — the deterministic validation runner and its evidence rules.
- [specs/REVIEW.md](specs/REVIEW.md) — the review pipeline and engines, findings, and blocking severities.
- [specs/OPENJEV.md](specs/OPENJEV.md) — the OpenJEV (JEV) analysis boundary, configuration, and failure behavior; §18 is the implemented Phase 3 early checkpoints, §17 the proposed/future items.
- [specs/QUALITY.md](specs/QUALITY.md) — the deterministic quality gate and the bounded fix loop.
- [specs/HUMAN-APPROVAL.md](specs/HUMAN-APPROVAL.md) — human gates: when approval is required and what must not bypass it.
- [specs/RECOVERY.md](specs/RECOVERY.md) — bounded retries, resume, and durable recovery.
- [specs/SECURITY.md](specs/SECURITY.md) — repository safety, command policy, secrets, and state ownership.

## Reference

- [reference/CLI.md](reference/CLI.md) — every `sop` command and its flags.
- [reference/CONFIGURATION.md](reference/CONFIGURATION.md) — `.agent-sdlc/config.yaml` schema, defaults, and environment overrides (including `early_jev`).
- [reference/STATUS-AND-RECOVERY.md](reference/STATUS-AND-RECOVERY.md) — task statuses, `sop status`/`task`/`resume`, and recovery commands.
- [reference/JEV-OPERATIONS.md](reference/JEV-OPERATIONS.md) — enabling JEV, provider configuration, and severity policy.
- [reference/PERFORMANCE.md](reference/PERFORMANCE.md) — the performance measurement model and validation/review reuse rules.

## Guides

- [guides/DEVELOPMENT.md](guides/DEVELOPMENT.md) — building, testing, and developing SOP.
- [guides/LESSONS.md](guides/LESSONS.md) — engineering lessons from building SOP V1 (working notes).
- [guides/SOP-CONTROLLER-DASHBOARD.md](guides/SOP-CONTROLLER-DASHBOARD.md) — run and control a SOP run from a local dashboard (and phone).

## Plans

- [PLAN-JEV-Implementation.md](PLAN-JEV-Implementation.md) — **the active plan** (JEV001–JEV017). SOP records this as active, so it stays at `docs/`. Execute via `sop run docs/PLAN-JEV-Implementation.md`.
- [plans/PLAN-Phase-3-OpenJEV.md](plans/PLAN-Phase-3-OpenJEV.md) — Phase 3 implementation plan (P3-001–P3-017): early JEV decision layer (task triage + pre-execution), disabled by default. **Implemented** (P3-001–P3-015) and committed; P3-016 records the dogfood.
- [plans/PLAN-Agent-Harness-V2.md](plans/PLAN-Agent-Harness-V2.md) — the Agent Harness V2 workstream.
- [plans/PLAN-Pre-JEV-Stabilization.md](plans/PLAN-Pre-JEV-Stabilization.md) — pre-JEV stabilization workstream.

- [plans/PLAN-Automatic-Blocked-Task-Recovery.md](plans/PLAN-Automatic-Blocked-Task-Recovery.md) — automatic blocked-task recovery.
- [plans/PLAN-Model-Routing.md](plans/PLAN-Model-Routing.md) — model routing.
- [plans/PLAN-Phase-3.5-Model-Routing.md](plans/PLAN-Phase-3.5-Model-Routing.md) — Phase 3.5: JEV-guided per-task model routing (**implemented**; router OFF by default).
- [plans/PLAN-Ollama-Agent-Plan-Synthesis.md](plans/PLAN-Ollama-Agent-Plan-Synthesis.md) — Ollama PLAN discovery and synthesis.
- [plans/PLAN-SOP-Performance.md](plans/PLAN-SOP-Performance.md) — performance and timing.
- [plans/BACKLOG.md](plans/BACKLOG.md) — known gaps and future candidates.

## Historical Documentation

Point-in-time or superseded artifacts. Non-normative: they do not define current behavior.

- [history/PLAN-wrapup.md](history/PLAN-wrapup.md) — the original implementation plan.
- [history/OLLAMA-DOGFOOD.md](history/OLLAMA-DOGFOOD.md) — the Ollama/DeepSeek dogfood test.
- [history/PHASE-3-DOGFOOD.md](history/PHASE-3-DOGFOOD.md) — the Phase 3 early-JEV dogfood: the deterministic fake-analyzer demonstration and the (not performed) real-provider procedure.
- [history/](history/) — PREJEV reconciliation, decomposition, baseline, and readiness artifacts.
- [tasks/](tasks/) — per-task specifications used by SOP (`sop run --task docs/tasks/...`).

## Canonical Locations and Kept Paths

To keep SOP working, a few paths are intentionally not under the categories above:

- [PRD.md](PRD.md) and [PLAN.md](PLAN.md) stay at `docs/` because SOP's planner discovers `docs/PRD.md` and `docs/PLAN.md` (`internal/planflow`).
- [PLAN-JEV-Implementation.md](PLAN-JEV-Implementation.md) stays at `docs/` because SOP records it as the active plan.
- [tasks/](tasks/) stays at `docs/tasks/` because plans invoke `sop run --task docs/tasks/...`.
