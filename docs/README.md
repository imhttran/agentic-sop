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
testing/        How was it verified on a real machine?
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
- clean-room test checklists and their results → `testing/`

## Getting Started

- [../README.md](../README.md) — project landing page: what SOP is and a minimal quick start.
- [guides/GETTING-STARTED.md](guides/GETTING-STARTED.md) — install the CLI and run SOP for the first time.
- [guides/PROJECT-SETUP.md](guides/PROJECT-SETUP.md) — the explicit setup path: init, PRD, plan, tasks, inspect.

## Requirements

- [PRD.md](PRD.md) — **the canonical product requirements** (goals, functional requirements, constraints, success criteria).
- [requirements/PRD-JEV.md](requirements/PRD-JEV.md) — the JEV workstream's product requirements (problem, vision, decision layer).
- [requirements/PRD-Phase-3-OpenJEV.md](requirements/PRD-Phase-3-OpenJEV.md) — Phase 3 product requirements: the early JEV task-triage and pre-execution checkpoints. **Implemented** (see [plans/PLAN-Phase-3-OpenJEV.md](plans/PLAN-Phase-3-OpenJEV.md) and [specs/OPENJEV.md](specs/OPENJEV.md) §18); items the PRD records as future work remain **proposed** and are listed in [specs/OPENJEV.md](specs/OPENJEV.md) §17.
- [requirements/PRD-Phase-4-Provider-Runtime.md](requirements/PRD-Phase-4-Provider-Runtime.md) — Phase 4 product requirements: the provider/runtime abstraction, capability discovery, and opt-in model-availability validation. **Implemented**; rules live in [specs/PROVIDERS.md](specs/PROVIDERS.md).

## Architecture

- [architecture/OVERVIEW.md](architecture/OVERVIEW.md) — components, responsibilities, data model, and the state-machine/retry/TDD/review/CI/security design.
- [architecture/SOP-BOUNDARY.md](architecture/SOP-BOUNDARY.md) — what SOP owns vs what agents, models, and optional capabilities own; the single-source-of-truth rule.
- [architecture/model-routing.md](architecture/model-routing.md) — the **non-normative** implementation seam for per-task model routing (how the lifecycle selects the task's model); the authoritative rules live in [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md).
- [architecture/provider-runtime.md](architecture/provider-runtime.md) — the **non-normative** implementation seam for the provider/runtime layer (registry, inspection, validation); the authoritative rules live in [specs/PROVIDERS.md](specs/PROVIDERS.md).
- [architecture/execution.md](architecture/execution.md) — the **non-normative** implementation seam showing how tasks, prompts, and the SOP skill share one execution path (WorkItem → capability → JEV → router → provider validation → lifecycle); the authoritative rules live in [specs/WORK-ITEMS.md](specs/WORK-ITEMS.md) and [specs/PROMPT-EXECUTION.md](specs/PROMPT-EXECUTION.md).

## Specifications

Normative behavior (MUST / MUST NOT / SHOULD / SHOULD NOT / MAY).

- [specs/WORKFLOW.md](specs/WORKFLOW.md) — task state vocabulary, legal transitions, remediation, scheduling, and plan handoff.
- [specs/TASK-LIFECYCLE.md](specs/TASK-LIFECYCLE.md) — the per-task lifecycle stages, TDD rules, and Git naming conventions.
- [specs/EXECUTION.md](specs/EXECUTION.md) — `sop run`: planning-source discovery, change detection, artifacts, verify-first, and parallelism.
- [specs/WORK-ITEMS.md](specs/WORK-ITEMS.md) — the unified execution input (**WorkItem**): how tasks and prompts adapt to it, and what it MUST NOT contain.
- [specs/PROMPT-EXECUTION.md](specs/PROMPT-EXECUTION.md) — **the authoritative home for `sop prompt`**: the command surface, default capability, deterministic routing, capability enforcement, read-only vs. governed `implement` execution, provider validation, artifacts, and the structured result. The SOP skill ([skills/sop/SKILL.md](../skills/sop/SKILL.md)) and its per-agent command aliases call this command; installation starts at [guides/INSTALLATION.md](guides/INSTALLATION.md), with [guides/ZED-SKILLS.md](guides/ZED-SKILLS.md), [guides/CLAUDE-SKILLS.md](guides/CLAUDE-SKILLS.md), and [guides/CLAUDE-PLUGIN.md](guides/CLAUDE-PLUGIN.md).
- [specs/AGENT-PROVIDER.md](specs/AGENT-PROVIDER.md) — the harness/provider/model boundary, configuration precedence, and structured outcomes.
- [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md) — **the authoritative home for model-routing rules**: the model-class layer (`small`/`medium`/`large`), configuration precedence, the deterministic per-task router, and the routing boundary. The router is **opt-in and OFF by default**, enabled only by `SOP_MODEL_ROUTING_ENABLED=true` (overriding `models.routing_enabled`), with `--model-class` always winning. Every other page links here rather than restating the rules. The implementation seam (non-normative) is recorded in [architecture/model-routing.md](architecture/model-routing.md).
- [specs/PROVIDERS.md](specs/PROVIDERS.md) — **the authoritative home for the provider/runtime layer** (Phase 4): provider identity, the registry, health, model discovery, typed capabilities, selection validation, failure behavior, and the read-only `sop providers` surface. The provider layer sits underneath model routing and never chooses a class; the seam is recorded (non-normatively) in [architecture/provider-runtime.md](architecture/provider-runtime.md).
- [specs/VALIDATION.md](specs/VALIDATION.md) — the deterministic validation runner and its evidence rules.
- [specs/REVIEW.md](specs/REVIEW.md) — the review pipeline and engines, findings, and blocking severities.
- [specs/OPENJEV.md](specs/OPENJEV.md) — the OpenJEV (JEV) analysis boundary, configuration, and failure behavior; §18 is the implemented Phase 3 early checkpoints, §17 the proposed/future items.
- [specs/QUALITY.md](specs/QUALITY.md) — the deterministic quality gate and the bounded fix loop.
- [specs/HUMAN-APPROVAL.md](specs/HUMAN-APPROVAL.md) — human gates: when approval is required and what must not bypass it.
- [specs/RECOVERY.md](specs/RECOVERY.md) — bounded retries, resume, and durable recovery.
- [specs/SECURITY.md](specs/SECURITY.md) — repository safety, command policy, secrets, and state ownership.

## Reference

- [reference/CLI.md](reference/CLI.md) — every `sop` command and its flags (including `sop providers`).
- [reference/CONFIGURATION.md](reference/CONFIGURATION.md) — `.agent-sdlc/config.yaml` schema, defaults, and environment overrides (including `early_jev`, the model-routing classes/precedence, and the `providers:` block, which link to [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md) and [specs/PROVIDERS.md](specs/PROVIDERS.md)).
- [reference/STATUS-AND-RECOVERY.md](reference/STATUS-AND-RECOVERY.md) — task statuses, `sop status`/`task`/`resume`, and recovery commands.
- [reference/JEV-OPERATIONS.md](reference/JEV-OPERATIONS.md) — enabling JEV, provider configuration, and severity policy.
- [reference/PERFORMANCE.md](reference/PERFORMANCE.md) — the performance measurement model and validation/review reuse rules.

## Guides

- [guides/INSTALLATION.md](guides/INSTALLATION.md) — **the installation entry point**: `./install.sh` on macOS/Linux, its options, where the binary goes, the agent skills it delegates, safety, and removal.
- [guides/WINDOWS-INSTALLATION.md](guides/WINDOWS-INSTALLATION.md) — `install.ps1`: the parameters, the binary directory and PATH guidance, how the skills are copied and marked, uninstall, and what the installer never does.
- [guides/CLAUDE-PLUGIN.md](guides/CLAUDE-PLUGIN.md) — the native Claude Code plugin package: what it exposes, the official local-install steps, and how it is generated from `skills/`.
- [guides/DEVELOPMENT.md](guides/DEVELOPMENT.md) — building, testing, and developing SOP.
- [guides/LESSONS.md](guides/LESSONS.md) — engineering lessons from building SOP V1 (working notes).
- [guides/SOP-CONTROLLER-DASHBOARD.md](guides/SOP-CONTROLLER-DASHBOARD.md) — run and control a SOP run from a local dashboard (and phone).
- [guides/ZED-SKILLS.md](guides/ZED-SKILLS.md) — install the SOP skills so Zed exposes `/sop`, `/sop-prompt`, `/sop-plan`, `/sop-review`, `/sop-diagnose`, `/sop-test`, and `/sop-implement`, and see how each calls `sop prompt`.
- [guides/GETTING-STARTED.md](guides/GETTING-STARTED.md) — install the CLI and run SOP against a project.
- [guides/CLAUDE-SKILLS.md](guides/CLAUDE-SKILLS.md) — the same commands for Claude Code: install the skills so Claude exposes the `/sop*` entry points, and how each delegates to `sop prompt`.
- [skills/sop/SKILL.md](../skills/sop/SKILL.md) — the shipped SOP agent skill: a thin client that invokes `sop prompt` (with `examples/` for plan, review, diagnose, and implement). Its one-per-capability aliases live beside it in [skills/](../skills/), installed for Zed and Claude Code by `scripts/install/install-skills.sh` and also packaged as a Claude Code plugin ([integrations/claude/](../integrations/claude)).

## Plans

Implementation plans, newest phase last. A completed plan stays here rather than moving
to `history/` when SOP or the repository still references its path: the archive under
`.agent-sdlc/archive/` records these sources, and the example commands in
`scripts/agents/sop-agent.sh` name one of them. Moving such a plan would break
`sop run`/`resume`/`reconcile`, so the path is kept and the status is declared here.

- [plans/BACKLOG.md](plans/BACKLOG.md) — known gaps and future candidates (the running list between phases).
- [plans/PLAN-Phase-3.5-Model-Routing.md](plans/PLAN-Phase-3.5-Model-Routing.md) — Phase 3.5: JEV-guided per-task model routing. **Implemented**; SOP's recorded active plan (`.agent-sdlc/plan.meta.json`); the router is OFF by default, opt-in via `SOP_MODEL_ROUTING_ENABLED=true`.
- [plans/PLAN-Phase-4-Provider-Runtime.md](plans/PLAN-Phase-4-Provider-Runtime.md) — Phase 4: provider/runtime abstraction, capability discovery, and opt-in model validation. **Implemented** (off by default); see [specs/PROVIDERS.md](specs/PROVIDERS.md).
- [plans/PLAN-Phase-5-Execution-Recovery.md](plans/PLAN-Phase-5-Execution-Recovery.md) — Phase 5: bounded model escalation and execution recovery. **Implemented** (off by default; opt-in via `SOP_MODEL_ESCALATION_ENABLED=true`); see [specs/RECOVERY.md](specs/RECOVERY.md) §8.
- [plans/PLAN-Phase-5.4-Unified-Work-Items.md](plans/PLAN-Phase-5.4-Unified-Work-Items.md) — Phase 5.4: unified work items, `sop prompt`, and the SOP skill. **Implemented**; see [specs/WORK-ITEMS.md](specs/WORK-ITEMS.md) and [specs/PROMPT-EXECUTION.md](specs/PROMPT-EXECUTION.md).
- [plans/PLAN-Phase-5.5-Distribution.md](plans/PLAN-Phase-5.5-Distribution.md) — Phase 5.5 (P55-001–P55-011): the unified `install.sh`, the per-agent skill installation it delegates, the Claude Code plugin package, and CI. **Implemented**; see [guides/INSTALLATION.md](guides/INSTALLATION.md) and [guides/CLAUDE-PLUGIN.md](guides/CLAUDE-PLUGIN.md).
- [plans/PLAN-Phase-5.6-Windows-Distribution.md](plans/PLAN-Phase-5.6-Windows-Distribution.md) — Phase 5.6 (P56-001–P56-012): native Windows installation and a script-organization cleanup. **Implemented except P56-011**, the clean-room test on a real Windows machine, which has not been run; see [guides/WINDOWS-INSTALLATION.md](guides/WINDOWS-INSTALLATION.md) and [testing/WINDOWS-CLEAN-ROOM.md](testing/WINDOWS-CLEAN-ROOM.md).
- [plans/PLAN-Phase-3-OpenJEV.md](plans/PLAN-Phase-3-OpenJEV.md) — Phase 3 implementation plan (P3-001–P3-017): early JEV decision layer (task triage + pre-execution), disabled by default. **Implemented** (P3-001–P3-015) and committed; P3-016 records the dogfood.
- [plans/PLAN-JEV-Implementation.md](plans/PLAN-JEV-Implementation.md) — the V1 JEV implementation plan (JEV001–JEV017). **Implemented**; archived by SOP. Execute with `sop run docs/plans/PLAN-JEV-Implementation.md`.
- [plans/PLAN-Agent-Harness-V2.md](plans/PLAN-Agent-Harness-V2.md) — the Agent Harness V2 workstream. **Implemented**; archived by SOP (`.agent-sdlc/archive/ahv2/`).
- [plans/PLAN-Pre-JEV-Stabilization.md](plans/PLAN-Pre-JEV-Stabilization.md) — the pre-JEV stabilization workstream. **Implemented**; archived by SOP.
- [plans/PLAN-Automatic-Blocked-Task-Recovery.md](plans/PLAN-Automatic-Blocked-Task-Recovery.md) — automatic blocked-task recovery. **Implemented**; archived by SOP.
- [plans/PLAN-SOP-Performance.md](plans/PLAN-SOP-Performance.md) — performance and timing. **Implemented** (PERF008 and PERF010–PERF012, PERF014 deferred with evidence); the shipped behavior is described in [reference/PERFORMANCE.md](reference/PERFORMANCE.md).
- [plans/PLAN-Ollama-Agent-Plan-Synthesis.md](plans/PLAN-Ollama-Agent-Plan-Synthesis.md) — Ollama PLAN discovery and synthesis. **Implemented**; PLAN runs the bounded DISCOVERY → tool-free SYNTHESIS lifecycle.

## Testing

- [testing/WINDOWS-CLEAN-ROOM.md](testing/WINDOWS-CLEAN-ROOM.md) — the clean-room Windows test checklist and its results log; real-machine results stay marked **NOT YET RUN** until they are actually performed, separately from CI.

## Historical Documentation

Point-in-time or superseded artifacts. Non-normative: they do not define current behavior.

- [history/PLAN-wrapup.md](history/PLAN-wrapup.md) — the original implementation plan.
- [history/OLLAMA-DOGFOOD.md](history/OLLAMA-DOGFOOD.md) — the Ollama/DeepSeek dogfood test.
- [history/PHASE-3-DOGFOOD.md](history/PHASE-3-DOGFOOD.md) — the Phase 3 early-JEV dogfood: the deterministic fake-analyzer demonstration and the (not performed) real-provider procedure.
- [history/PLAN-Model-Routing.md](history/PLAN-Model-Routing.md) — the early, superseded model-routing design (MODELRT001–MODELRT022); the shipped rules live in [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md).
- [history/](history/) — PREJEV reconciliation, decomposition, baseline, and readiness artifacts.

## Canonical Locations and Kept Paths

To keep SOP working, a few paths are intentionally not under the categories above:

- [PRD.md](PRD.md) and [PLAN.md](PLAN.md) stay at `docs/` because SOP's planner discovers `docs/PRD.md` and `docs/PLAN.md` (`internal/planflow`).
- [tasks/](tasks/) stays at `docs/tasks/` because plans invoke `sop run --task docs/tasks/...`. These are per-task specifications, not history.

## Checking Documentation Links

Documentation links MUST resolve. Run the repository link check over `docs/**/*.md`
and `README.md` and their relative links:

```bash
scripts/checks/check-doc-links.sh
```

It prints `broken links: <count>` (for example `broken links: 0`) and exits
non-zero when any relative link does not resolve. Routing documentation in
particular MUST pass with `broken links: 0`.
