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
reports/        What was measured or decided at a point in time?
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
- point-in-time measurement or decision evidence → `reports/`

## Shipped RAG Skills vs. the RAG Retrieval Adapter

Read [RAG-SKILLS.md](guides/RAG-SKILLS.md) for the shipped read-only skills.
The [consolidation guide](guides/REPOSITORY-CONSOLIDATION.md#shipped-rag-skills-vs-the-rag-retrieval-adapter)
compares those skills with the optional runtime retrieval adapter; its rollout
decision is recorded in [RAG-SOP-004-DECISION.md](specs/RAG-SOP-004-DECISION.md).

## Getting Started

- [../README.md](../README.md) — project landing page: what SOP is and a minimal quick start.
- [guides/GETTING-STARTED.md](guides/GETTING-STARTED.md) — install the CLI and run SOP for the first time.
- [guides/PROJECT-SETUP.md](guides/PROJECT-SETUP.md) — the explicit setup path: init, PRD, plan, tasks, inspect.

## Requirements

- [PRD.md](PRD.md) — **the canonical product requirements** (goals, functional requirements, constraints, success criteria).
- [requirements/PRD-JEV.md](requirements/PRD-JEV.md) — the JEV workstream's product requirements (problem, vision, decision layer).
- [requirements/PRD-Phase-3-OpenJEV.md](requirements/PRD-Phase-3-OpenJEV.md) — Phase 3 product requirements: the early JEV task-triage and pre-execution checkpoints. **Implemented** (see [plans/PLAN-Phase-3-OpenJEV.md](history/plans/PLAN-Phase-3-OpenJEV.md) and [specs/OPENJEV.md](specs/OPENJEV.md) §18); items the PRD records as future work remain **proposed** and are listed in [specs/OPENJEV.md](specs/OPENJEV.md) §17.
- [requirements/PRD-Phase-4-Provider-Runtime.md](requirements/PRD-Phase-4-Provider-Runtime.md) — Phase 4 product requirements: the provider/runtime abstraction, capability discovery, and opt-in model-availability validation. **Implemented**; rules live in [specs/PROVIDERS.md](specs/PROVIDERS.md).

## Architecture

- [architecture/OVERVIEW.md](architecture/OVERVIEW.md) — the architecture: goal, system context, component map, the
  state-machine/retry/TDD/review/CI design, data model, and evolution.
- [architecture/COMPONENTS.md](architecture/COMPONENTS.md) — the top-level components and their responsibilities, one
  per section (the detail behind OVERVIEW §3–§4).
- [architecture/SOP-BOUNDARY.md](architecture/SOP-BOUNDARY.md) — what SOP owns vs what agents, models, and optional capabilities own; the single-source-of-truth rule.
- [architecture/model-routing.md](architecture/model-routing.md) — the **non-normative** implementation seam for per-task model routing (how the lifecycle selects the task's model); the authoritative rules live in [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md).
- [architecture/provider-runtime.md](architecture/provider-runtime.md) — the **non-normative** implementation seam for the provider/runtime layer (registry, inspection, validation); the authoritative rules live in [specs/PROVIDERS.md](specs/PROVIDERS.md).
- [architecture/execution.md](architecture/execution.md) — the **non-normative** implementation seam showing how tasks, prompts, and the SOP skill share one execution path (WorkItem → capability → JEV → router → provider validation → lifecycle); the authoritative rules live in [specs/WORK-ITEMS.md](specs/WORK-ITEMS.md) and [specs/PROMPT-EXECUTION.md](specs/PROMPT-EXECUTION.md).
- [architecture/orchestration.md](architecture/orchestration.md) — the **non-normative** Phase 9 orchestration domain model (ORCH-001): the single-lifecycle-authority invariant and the deterministic `SINGLE` / `SEQUENTIAL` / `PARALLEL` mode decision. The Phase 9 rules live in [plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md](plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md).
- [architecture/JEV-DECISION-CONTRACT.md](architecture/JEV-DECISION-CONTRACT.md) — the ratified JEV decision-contract alignment (Gate 3, 2026-10-11): the accepted contract requirements (§13–§19) and the ratified decisions D1–D7 (§20).
- [architecture/RAG-RETRIEVAL-INTEGRATION.md](architecture/RAG-RETRIEVAL-INTEGRATION.md) — **proposed** (unapproved) design for opt-in, read-only RAG knowledge retrieval: the adapter seam, ownership boundaries, and deterministic context-integration rules. It changes no runtime behaviour and awaits the RAG-SOP-001 architecture approval.

## Specifications

Normative behavior (MUST / MUST NOT / SHOULD / SHOULD NOT / MAY).

- [specs/WORKFLOW.md](specs/WORKFLOW.md) — task state vocabulary, legal transitions, remediation, scheduling, and plan handoff.
- [specs/PLAN-HISTORICALIZATION.md](specs/PLAN-HISTORICALIZATION.md) — the deterministic, model-free transition that moves a completed or explicitly disposed plan into its historical record: readiness, eligibility, dispositions, atomicity, idempotency, and the `/sop-historicalize` skill.
- [specs/TASK-LIFECYCLE.md](specs/TASK-LIFECYCLE.md) — the per-task lifecycle stages, TDD rules, and Git naming conventions.
- [specs/EXECUTION.md](specs/EXECUTION.md) — `sop run`: planning-source discovery, change detection, artifacts, verify-first, parallelism, and objective acceptance checks (the `VERIFIED`/`PARTIAL`/`HOLD` verdict and its evidence).
- [specs/WORK-ITEMS.md](specs/WORK-ITEMS.md) — the unified execution input (**WorkItem**): how tasks and prompts adapt to it, and what it MUST NOT contain.
- [specs/PROMPT-EXECUTION.md](specs/PROMPT-EXECUTION.md) — **the authoritative home for `sop prompt`**: the command surface, default capability, deterministic routing, capability enforcement, read-only vs. governed `implement` execution, provider validation, artifacts, and the structured result. The SOP skill ([skills/sop/SKILL.md](../skills/sop/SKILL.md)) and its per-agent command aliases call this command; installation starts at [guides/INSTALLATION.md](guides/INSTALLATION.md), with [guides/ZED-SKILLS.md](guides/ZED-SKILLS.md), [guides/CLAUDE-SKILLS.md](guides/CLAUDE-SKILLS.md), and [guides/CLAUDE-PLUGIN.md](guides/CLAUDE-PLUGIN.md).
- [specs/AGENT-PROVIDER.md](specs/AGENT-PROVIDER.md) — the harness/provider/model boundary, configuration precedence, and structured outcomes.
- [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md) — **the authoritative home for model-routing rules**: the model-class layer (`small`/`medium`/`large`), configuration precedence, the deterministic per-task router, the local-first fallback for a local class, and the routing boundary. The router is **opt-in and OFF by default**, enabled only by `SOP_MODEL_ROUTING_ENABLED=true` (overriding `models.routing_enabled`), with `--model-class` always winning. Every other page links here rather than restating the rules. The implementation seam (non-normative) is recorded in [architecture/model-routing.md](architecture/model-routing.md).
- [specs/PROVIDERS.md](specs/PROVIDERS.md) — **the authoritative home for the provider/runtime layer** (Phase 4): provider identity, the registry, health, model discovery, typed capabilities, selection validation, failure behavior, and the read-only `sop providers` surface. The provider layer sits underneath model routing and never chooses a class; the seam is recorded (non-normatively) in [architecture/provider-runtime.md](architecture/provider-runtime.md).
- [specs/VALIDATION.md](specs/VALIDATION.md) — the deterministic validation runner and its evidence rules.
- [specs/REVIEW.md](specs/REVIEW.md) — the review pipeline and engines, findings, and blocking severities.
- [specs/OPENJEV.md](specs/OPENJEV.md) — the OpenJEV (JEV) analysis boundary, configuration, and failure behavior; §18 is the implemented Phase 3 early checkpoints, §17 the proposed/future items.
- [specs/QUALITY.md](specs/QUALITY.md) — the deterministic quality gate and the bounded fix loop.
- [specs/HUMAN-APPROVAL.md](specs/HUMAN-APPROVAL.md) — human gates: when approval is required and what must not bypass it.
- [specs/RECOVERY.md](specs/RECOVERY.md) — bounded retries, resume, and durable recovery.
- [specs/DECISION-TELEMETRY.md](specs/DECISION-TELEMETRY.md) — **the authoritative home for decision telemetry**: the diagnostic aggregation of SOP's bounded decisions (routing class and source, adaptive adaptation, availability fallback, recovery escalation, gate disposition) reported through `internal/perf`. Diagnostic only: it never influences a decision and is not a second source of truth. Proposed (no implementation yet).
- [specs/SECURITY.md](specs/SECURITY.md) — repository safety, command policy, secrets, and state ownership.
- [specs/RAG-RETRIEVAL-CONTRACT.md](specs/RAG-RETRIEVAL-CONTRACT.md) — **proposed** versioned, retrieval-only request/response contract (schema `rag-retrieval/v1`): source metadata, size ceilings, error taxonomy, and fallback/required-evidence rules.
- [specs/RAG-SECURITY-THREAT-MODEL.md](specs/RAG-SECURITY-THREAT-MODEL.md) — **proposed** threat model for RAG retrieval (injection, cross-project leakage, scope, resource, malformed-response) and its fail-closed rules.
- [specs/RAG-CONFIGURATION.md](specs/RAG-CONFIGURATION.md) — **proposed** `SOP_RAG_*` configuration and feature-flag design: opt-in, disabled by default, bytes-based budgets, per-project corpus allowlisting.
- [specs/RAG-EVALUATION.md](specs/RAG-EVALUATION.md) — the **predeclared** evaluation approach and GO/CONDITIONAL/NO-GO criteria for the RAG retrieval adapter versus BM25-only, fixed before results.
- [specs/RAG-SOP-004-DECISION.md](specs/RAG-SOP-004-DECISION.md) — the recorded operator decision report for the RAG retrieval adapter (GO / CONDITIONAL / NO-GO), with per-case evidence, the operational fixture matrix, tradeoffs, and the explicit statement that default enablement is unchanged.
- [specs/RAG-POSTGRES-VALIDATION-POLICY.md](specs/RAG-POSTGRES-VALIDATION-POLICY.md) — the required PostgreSQL integration validation category as a deterministic file-pattern policy for migration/database-sensitive changes, kept strictly separate from retrieval quality; unit-only validation never satisfies it.

## Topic Ownership

Every fact has **one** authoritative home; other documents link to it rather than
restating it, so adding or changing a command, config key, or state means editing a
single owner. This table is that map.

| Topic                                                                                     | Authoritative document                                                                 | Supporting material                                                                                                                                                                             |
| ----------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Product goals                                                                             | [PRD.md](PRD.md)                                                                       | Workstream PRDs under requirements/                                                                                                                                                             |
| Architecture structure and the state graph                                                | [architecture/OVERVIEW.md](architecture/OVERVIEW.md)                                   | [COMPONENTS.md](architecture/COMPONENTS.md); [SOP-BOUNDARY.md](architecture/SOP-BOUNDARY.md)                                                                                                    |
| Ownership boundaries                                                                      | [SOP-BOUNDARY.md](architecture/SOP-BOUNDARY.md)                                        | Architecture implementation seams                                                                                                                                                               |
| Commands and flags                                                                        | [CLI.md](reference/CLI.md)                                                             | The specs that own each command's semantics                                                                                                                                                     |
| Configuration keys and defaults                                                           | [CONFIGURATION.md](reference/CONFIGURATION.md)                                         | The specs that own each block's policy                                                                                                                                                          |
| Task statuses, transitions, scheduling                                                    | [WORKFLOW.md](specs/WORKFLOW.md)                                                       | OVERVIEW §5–§6 (authoritative graph); [STATUS-AND-RECOVERY.md](reference/STATUS-AND-RECOVERY.md)                                                                                                |
| Per-task lifecycle, TDD rules, Git naming                                                 | [TASK-LIFECYCLE.md](specs/TASK-LIFECYCLE.md)                                           | OVERVIEW §7                                                                                                                                                                                     |
| Outcomes, verified mutation, already-satisfied proof, discovery progress, harness budgets | [AGENT-PROVIDER.md](specs/AGENT-PROVIDER.md)                                           | Resolved backlog observations in history/                                                                                                                                                       |
| Prompt execution and work items                                                           | [PROMPT-EXECUTION.md](specs/PROMPT-EXECUTION.md), [WORK-ITEMS.md](specs/WORK-ITEMS.md) | CLI.md                                                                                                                                                                                          |
| Task execution, execution modes, and objective acceptance checks                          | [EXECUTION.md](specs/EXECUTION.md)                                                     | Setup and usage guides                                                                                                                                                                          |
| Validation runner and evidence                                                            | [VALIDATION.md](specs/VALIDATION.md)                                                   | CONFIGURATION.md (`validation.*`); [RAG-POSTGRES-VALIDATION-POLICY.md](specs/RAG-POSTGRES-VALIDATION-POLICY.md) for the PostgreSQL integration category |
| Review pipeline and severities                                                            | [REVIEW.md](specs/REVIEW.md)                                                           | QUALITY.md                                                                                                                                                                                      |
| Quality gate and fix loop                                                                 | [QUALITY.md](specs/QUALITY.md)                                                         | REVIEW.md                                                                                                                                                                                       |
| Human approval                                                                            | [HUMAN-APPROVAL.md](specs/HUMAN-APPROVAL.md)                                           | [APPROVALS.md](guides/APPROVALS.md)                                                                                                                                                             |
| Recovery, retry, and resume                                                               | [RECOVERY.md](specs/RECOVERY.md)                                                       | [STATUS-AND-RECOVERY.md](reference/STATUS-AND-RECOVERY.md)                                                                                                                                      |
| Decision telemetry (routing/escalation/gate decisions)                                    | [DECISION-TELEMETRY.md](specs/DECISION-TELEMETRY.md)                                   | [PERFORMANCE.md](reference/PERFORMANCE.md)                                                                                                                                                      |
| Security, command policy, secrets                                                         | [SECURITY.md](specs/SECURITY.md)                                                       | OVERVIEW §14                                                                                                                                                                                    |
| Routing and provider rules                                                                | [MODEL-ROUTING.md](specs/MODEL-ROUTING.md), [PROVIDERS.md](specs/PROVIDERS.md)         | Configuration reference and architecture seams                                                                                                                                                  |
| JEV / OpenJEV analysis                                                                    | [OPENJEV.md](specs/OPENJEV.md)                                                         | [JEV-OPERATIONS.md](reference/JEV-OPERATIONS.md); [JEV-TROUBLESHOOTING.md](reference/JEV-TROUBLESHOOTING.md)                                                                                    |
| Installation                                                                              | [INSTALLATION.md](guides/INSTALLATION.md)                                              | Platform and agent-specific guides                                                                                                                                                              |
| Delivery status and limitations                                                           | [PROJECT-STATUS.md](reference/PROJECT-STATUS.md)                                       | Historical completion evidence                                                                                                                                                                  |
| Future work                                                                               | [BACKLOG.md](plans/BACKLOG.md)                                                         | Numbered implementation plans                                                                                                                                                                   |
| Performance measurements                                                                  | [PERFORMANCE.md](reference/PERFORMANCE.md)                                             | Closure plan and point-in-time baseline reports                                                                                                                                                 |
| Reliability evaluation                                                                    | [EVALUATION.md](reference/EVALUATION.md)                                               | Run trace (`trace.json`) and progress signals                                                                                                                                                   |
| Decision-model findings (advisory vs deterministic)                                       | [DECISION-MODEL-FINDINGS.md](reference/DECISION-MODEL-FINDINGS.md)                     | [EVALUATION.md](reference/EVALUATION.md); [OPENJEV.md](specs/OPENJEV.md)                                                                                                                        |
| AI decision-model promotion criteria                                                      | [AI-DECISION-PROMOTION-CRITERIA.md](reference/AI-DECISION-PROMOTION-CRITERIA.md)       | [DECISION-MODEL-FINDINGS.md](reference/DECISION-MODEL-FINDINGS.md)                                                                                                                              |
| Multi-agent orchestration                                                                 | [architecture/orchestration.md](architecture/orchestration.md)                         | [plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md](plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md); adoption record in [reports/PHASE-9-MULTI-AGENT-ADOPTION.md](reports/PHASE-9-MULTI-AGENT-ADOPTION.md) |
| Lifecycle verdict gate and outcome recovery                                               | [LIFECYCLE-VERDICT-GATE.md](reference/LIFECYCLE-VERDICT-GATE.md)                       | [OUTCOME-RECOVERY-WAIVER.md](reference/OUTCOME-RECOVERY-WAIVER.md); [EXECUTION.md](specs/EXECUTION.md) §6b                                                                                      |
| Validation evidence reuse                                                                 | [EVIDENCE-REUSE.md](reference/EVIDENCE-REUSE.md)                                       | [VALIDATION.md](specs/VALIDATION.md); [CLI.md](reference/CLI.md)                                                                                                                                |
| JEV decision contract                                                                     | [JEV-DECISION-CONTRACT.md](architecture/JEV-DECISION-CONTRACT.md)                      | [OPENJEV.md](specs/OPENJEV.md)                                                                                                                                                                  |
| Knowledge retrieval (RAG) integration — opt-in, retrieval-only                           | [architecture/RAG-RETRIEVAL-INTEGRATION.md](architecture/RAG-RETRIEVAL-INTEGRATION.md) | [RAG-RETRIEVAL-CONTRACT.md](specs/RAG-RETRIEVAL-CONTRACT.md); [RAG-SECURITY-THREAT-MODEL.md](specs/RAG-SECURITY-THREAT-MODEL.md); [RAG-CONFIGURATION.md](specs/RAG-CONFIGURATION.md); [RAG-EVALUATION.md](specs/RAG-EVALUATION.md); [RAG-SOP-004-DECISION.md](specs/RAG-SOP-004-DECISION.md) |
| Required PostgreSQL integration validation category (database-sensitive changes)          | [RAG-POSTGRES-VALIDATION-POLICY.md](specs/RAG-POSTGRES-VALIDATION-POLICY.md)           | [VALIDATION.md](specs/VALIDATION.md); [QUALITY.md](specs/QUALITY.md)                                                                                                                            |
| Shipped RAG skills vs. the RAG retrieval adapter                                          | [guides/RAG-SKILLS.md](guides/RAG-SKILLS.md)                                          | [guides/REPOSITORY-CONSOLIDATION.md](guides/REPOSITORY-CONSOLIDATION.md); [RAG-SOP-004-DECISION.md](specs/RAG-SOP-004-DECISION.md)                                                             |
| Repository consolidation and legacy transition                                            | [REPOSITORY-CONSOLIDATION.md](guides/REPOSITORY-CONSOLIDATION.md)                      | [LEGACY-REPOSITORY-TRANSITION.md](guides/LEGACY-REPOSITORY-TRANSITION.md); [reports/consolidation/](reports/consolidation/completion.md) — committed merge `a8082b6154dd88fd8ac9fd18af5df5e682a751a6` (parents core/controller/adapter)                                                        |

## Reference

- [reference/CLI.md](reference/CLI.md) — every `sop` command and its flags (including `sop providers`).
- [reference/CONFIGURATION.md](reference/CONFIGURATION.md) — `.agent-sdlc/config.yaml` schema, defaults, and environment overrides (including `early_jev`, the model-routing classes/precedence, and the `providers:` block, which link to [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md) and [specs/PROVIDERS.md](specs/PROVIDERS.md)).
- [reference/STATUS-AND-RECOVERY.md](reference/STATUS-AND-RECOVERY.md) — task statuses, `sop status`/`task`/`resume`, and recovery commands.
- [reference/JEV-OPERATIONS.md](reference/JEV-OPERATIONS.md) — enabling JEV, provider configuration, severity
  policy, lifecycle placement, and safety boundaries.
- [reference/JEV-TROUBLESHOOTING.md](reference/JEV-TROUBLESHOOTING.md) — diagnosing JEV: failure behavior, report/artifact
  locations, the terminology glossary, and JEV015 traceability.
- [reference/PROJECT-STATUS.md](reference/PROJECT-STATUS.md) — implemented capabilities and known limitations, separated from future work.
- [reference/TASK-INDEX.md](reference/TASK-INDEX.md) — every preserved task document, including PREJEV decomposed tasks.
- [reference/PERFORMANCE.md](reference/PERFORMANCE.md) — the performance measurement model and validation/review reuse rules.
- [reference/EVALUATION.md](reference/EVALUATION.md) — the deterministic evaluation harness: fixture contract, layout, and the failure → regression workflow.
- [reference/DECISION-MODEL-FINDINGS.md](reference/DECISION-MODEL-FINDINGS.md) — consolidated DME-003/004/005 and JEV shadow-mode findings: what was tested, verified results, architectural conclusions, and remaining investigation.
- [reference/AI-DECISION-PROMOTION-CRITERIA.md](reference/AI-DECISION-PROMOTION-CRITERIA.md) — proposed criteria for reconsidering AI-assisted failure disposition: separate evaluation scopes, hard safety gates, effectiveness thresholds, staged promotion, and decision outcomes.
- [reference/LIFECYCLE-VERDICT-GATE.md](reference/LIFECYCLE-VERDICT-GATE.md) — the lifecycle verdict gate (SOP-HARD-01): how the terminal `LOCAL_DONE` transition is tied to the objective-level `VERIFIED` / `PARTIAL` / `HOLD` outcome verdict.
- [reference/OUTCOME-RECOVERY-WAIVER.md](reference/OUTCOME-RECOVERY-WAIVER.md) — the supported recovery surface (SOP-HARD-04): the read-only `sop verify` and the designated-approver, criterion-scoped `sop outcome --waive`.
- [reference/EVIDENCE-REUSE.md](reference/EVIDENCE-REUSE.md) — validation evidence reuse (VER-001–VER-004): the portable cross-worktree store and `sop validate --cache`.
- [reference/E2E-ONE-SHOT-DIAGNOSIS.md](reference/E2E-ONE-SHOT-DIAGNOSIS.md) — the end-to-end one-shot diagnosis: the validation-evidence-reuse and ONESHOT-001 evidence with the recorded baseline.

## Guides

- [guides/INSTALLATION.md](guides/INSTALLATION.md) — **the installation entry point**: `./install.sh` on macOS/Linux, its options, where the binary goes, the agent skills it delegates, safety, and removal.
- [guides/WINDOWS-INSTALLATION.md](guides/WINDOWS-INSTALLATION.md) — `install.ps1`: the parameters, the binary directory and PATH guidance, how the skills are copied and marked, uninstall, and what the installer never does.
- [guides/CLAUDE-PLUGIN.md](guides/CLAUDE-PLUGIN.md) — the native Claude Code plugin package: what it exposes, the official local-install steps, and how it is generated from `skills/`.
- [guides/DEVELOPMENT.md](guides/DEVELOPMENT.md) — building, testing, and developing SOP.
- [guides/LESSONS.md](guides/LESSONS.md) — engineering lessons from building SOP V1 (working notes).
- [guides/SOP-CONTROLLER-DASHBOARD.md](guides/SOP-CONTROLLER-DASHBOARD.md) — run and control a SOP run from a local dashboard (and phone).
- [guides/ZED-SKILLS.md](guides/ZED-SKILLS.md) — install the SOP prompt aliases and `/sop-end-to-end` project entry point for Zed.
- [guides/GETTING-STARTED.md](guides/GETTING-STARTED.md) — install the CLI and run SOP against a project.
- [guides/PROJECT-SETUP.md](guides/PROJECT-SETUP.md) — the explicit setup path: init, PRD, plan, tasks, and inspection.
- [guides/APPROVALS.md](guides/APPROVALS.md) — see a human approval gate, read it, decide it (interactively or explicitly), and continue a run.
- [guides/CLAUDE-SKILLS.md](guides/CLAUDE-SKILLS.md) — the same `/sop*` prompt and project entry points for Claude Code.
- [guides/RAG-SKILLS.md](guides/RAG-SKILLS.md) — the `/rag-schema`, `/rag-test`, `/rag-ingest`, and `/sop-rag-integrate` RAG skills, their safety boundaries, and the controller changes needed to enforce database-backed integration tests.
- [guides/REPOSITORY-CONSOLIDATION.md](guides/REPOSITORY-CONSOLIDATION.md) — the unified repository and command surface: the one-module layout, `sop controller serve` / `sop decision`, the controller/adapter source mapping and import rewrites, the preserved legacy wrappers, and the distinction between the shipped RAG skills and the proposed/implemented RAG retrieval adapter.
- [guides/LEGACY-REPOSITORY-TRANSITION.md](guides/LEGACY-REPOSITORY-TRANSITION.md) — recommendations for the later, operator-approved transition of the legacy `sop-controller` and `sop-decision-adapters` repositories; **no** retirement is performed.
- [skills/sop/SKILL.md](../skills/sop/SKILL.md) — the shipped SOP agent skill: a thin client that invokes `sop prompt` (with `examples/` for plan, review, diagnose, and implement). Its one-per-capability aliases live beside it in [skills/](../skills/), installed for Zed and Claude Code by `scripts/install/install-skills.sh` and also packaged as a Claude Code plugin ([integrations/claude/](../integrations/claude)).

## Plans

Implementation and verification plans. A completed plan stays here rather than moving
to `history/` when SOP or the repository still references its path: the archive under
`.agent-sdlc/archive/` records these sources, and the example commands in
`scripts/agents/sop-agent.sh` name one of them. Moving such a plan would break
`sop run`/`resume`/`reconcile`, so the path is kept and the status is declared here.

- [plans/BACKLOG.md](plans/BACKLOG.md) — known gaps and future candidates (the running list between phases), including the decision-model evaluation follow-ups (DME-FU-001–005) and their promotion criteria [reference/AI-DECISION-PROMOTION-CRITERIA.md](reference/AI-DECISION-PROMOTION-CRITERIA.md).
- [plans/PLAN-RAG-SOP-Integration.md](plans/PLAN-RAG-SOP-Integration.md) — the RAG integration plan (RAG-SOP-001–004): architecture, adapter implementation, required PostgreSQL integration validation, comparative evaluation, and the operator rollout decision. The evaluation criteria are fixed in [specs/RAG-EVALUATION.md](specs/RAG-EVALUATION.md) and the recorded outcome is [specs/RAG-SOP-004-DECISION.md](specs/RAG-SOP-004-DECISION.md).
- [plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md](plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md) — Phase 9 (ORCH-001–ORCH-012): the deterministic multi-agent orchestration system. **Delivery record**: all twelve tasks are `LOCAL_DONE`; the adoption gate ([reports/PHASE-9-MULTI-AGENT-ADOPTION.md](reports/PHASE-9-MULTI-AGENT-ADOPTION.md)) keeps multi-agent **opt-in**, so single-agent stays the default.
- [plans/PLAN-Pre-Performance-Closure.md](history/plans/PLAN-Pre-Performance-Closure.md) — **COMPLETE**: the executed CLOSE-001–CLOSE-011 DAG reconciled and verified both SOP and sop-controller, dogfooded and resumed, inventoried existing metrics, measured four workloads, and recorded the readiness verdict. All eleven tasks are `LOCAL_DONE` and the plan's final gate passed; it introduces no new performance architecture and does not start the performance phase.
- [plans/PLAN-Phase-3.5-Model-Routing.md](history/plans/PLAN-Phase-3.5-Model-Routing.md) — Phase 3.5: JEV-guided per-task model routing. **Implemented**; the historical document preserves its task definitions; the router is OFF by default, opt-in via `SOP_MODEL_ROUTING_ENABLED=true`.
- [plans/PLAN-Phase-4-Provider-Runtime.md](plans/PLAN-Phase-4-Provider-Runtime.md) — Phase 4: provider/runtime abstraction, capability discovery, and opt-in model validation. **Implemented** (off by default); see [specs/PROVIDERS.md](specs/PROVIDERS.md).
- [plans/PLAN-Phase-5-Execution-Recovery.md](plans/PLAN-Phase-5-Execution-Recovery.md) — Phase 5: bounded model escalation and execution recovery. **Implemented** (off by default; opt-in via `SOP_MODEL_ESCALATION_ENABLED=true`); see [specs/RECOVERY.md](specs/RECOVERY.md) §8.
- [plans/PLAN-Phase-5.4-Unified-Work-Items.md](plans/PLAN-Phase-5.4-Unified-Work-Items.md) — Phase 5.4: unified work items, `sop prompt`, and the SOP skill. **Implemented**; see [specs/WORK-ITEMS.md](specs/WORK-ITEMS.md) and [specs/PROMPT-EXECUTION.md](specs/PROMPT-EXECUTION.md).
- [plans/PLAN-Phase-5.5-Distribution.md](plans/PLAN-Phase-5.5-Distribution.md) — Phase 5.5 (P55-001–P55-011): the unified `install.sh`, the per-agent skill installation it delegates, the Claude Code plugin package, and CI. **Implemented**; see [guides/INSTALLATION.md](guides/INSTALLATION.md) and [guides/CLAUDE-PLUGIN.md](guides/CLAUDE-PLUGIN.md).
- [plans/PLAN-Phase-5.6-Windows-Distribution.md](plans/PLAN-Phase-5.6-Windows-Distribution.md) — Phase 5.6 (P56-001–P56-012): native Windows installation and a script-organization cleanup. **Implemented except P56-011**, the clean-room test on a real Windows machine, which has not been run; see [guides/WINDOWS-INSTALLATION.md](guides/WINDOWS-INSTALLATION.md) and [testing/WINDOWS-CLEAN-ROOM.md](testing/WINDOWS-CLEAN-ROOM.md).
- [plans/PLAN-Phase-6-Interactive-Approval.md](plans/PLAN-Phase-6-Interactive-Approval.md) — Phase 6 (P6-001–P6-011): make SOP's human decision points operable and discoverable by a client — enumerate the gates, list a plan's changed executed tasks before mutating, name the gate a run parked at, and decide interactively. **Implemented**: the read surface (`P6-001`–`P6-004`, `sop approvals` and `sop reconcile --list-changed`) and the interactive surface (`P6-005`–`P6-011`) both shipped; the human workflow is in [guides/APPROVALS.md](guides/APPROVALS.md). See [specs/HUMAN-APPROVAL.md](specs/HUMAN-APPROVAL.md) and [reference/CLI.md](reference/CLI.md).
- [plans/PLAN-Phase-3-OpenJEV.md](history/plans/PLAN-Phase-3-OpenJEV.md) — Phase 3 implementation plan (P3-001–P3-017): early JEV decision layer (task triage + pre-execution), disabled by default. **Implemented** (P3-001–P3-015) and committed; P3-016 records the dogfood.
- [plans/PLAN-JEV-Implementation.md](history/plans/PLAN-JEV-Implementation.md) — the V1 JEV implementation plan (JEV001–JEV017). **Implemented**; archived by SOP. Read the historical plan for the original task definitions; current execution instructions are in [guides/GETTING-STARTED.md](guides/GETTING-STARTED.md).
- [plans/PLAN-Agent-Harness-V2.md](history/plans/PLAN-Agent-Harness-V2.md) — the Agent Harness V2 workstream. **Implemented**; archived by SOP (`.agent-sdlc/archive/ahv2/`).
- [plans/PLAN-Pre-JEV-Stabilization.md](history/plans/PLAN-Pre-JEV-Stabilization.md) — the pre-JEV stabilization workstream. **Implemented**; archived by SOP.
- [plans/PLAN-Automatic-Blocked-Task-Recovery.md](history/plans/PLAN-Automatic-Blocked-Task-Recovery.md) — automatic blocked-task recovery. **Implemented**; archived by SOP.
- [plans/PLAN-SOP-Performance.md](plans/PLAN-SOP-Performance.md) — performance and timing. **Implemented** (PERF008 and PERF010–PERF012, PERF014 deferred with evidence); the shipped behavior is described in [reference/PERFORMANCE.md](reference/PERFORMANCE.md).
- [plans/PLAN-Ollama-Agent-Plan-Synthesis.md](plans/PLAN-Ollama-Agent-Plan-Synthesis.md) — Ollama PLAN discovery and synthesis. **Implemented**; PLAN runs the bounded DISCOVERY → tool-free SYNTHESIS lifecycle.
- [plans/PLAN-Decision-Telemetry-Extension.md](plans/PLAN-Decision-Telemetry-Extension.md) — Decision Telemetry (TEL-001–TEL-007): extend the run-metrics aggregator with the decisions SOP records (model tier, override, escalation, JEV invocation) without changing decision authority. **In progress**; the model-tier, override, escalation, and JEV dimensions are implemented in `internal/runmetrics`, and adapter agreement/confidence remain UNAVAILABLE (no recorded source).
- [plans/PLAN-TEL-009-Telemetry-Reporting-Corrections.md](plans/PLAN-TEL-009-Telemetry-Reporting-Corrections.md) — TEL-009: correct `decision_escalation_rate` (model escalation only, not human approvals), reconcile `decision_override_rate` provenance, make `retry_efficiency` a rate, and version `model-selection.json`. **Implemented**; reporting/evidence only.
- [plans/PLAN-SOP-Developer-Preview.md](plans/PLAN-SOP-Developer-Preview.md) — the Developer Preview workstream (PREVIEW-001–005). **SUPERSEDED**: disposed by SOP without executing PREVIEW-002–PREVIEW-005; the deliverables and the **HOLD** readiness verdict are preserved as evidence in [history/developer-preview/](history/developer-preview/README.md), and the still-open findings are carried in [plans/BACKLOG.md](plans/BACKLOG.md) § Developer Preview carry-forward.
- [plans/PLAN-SOP-Autonomy.md](plans/PLAN-SOP-Autonomy.md) — the SOP autonomy workstream (AUTONOMY-001/004/005), consolidated into a single authoritative plan. **HISTORICALIZED — COMPLETE** (2026-10-13): all three tasks reached `LOCAL_DONE` and SOP retained the audit record at `.agent-sdlc/archive/plan-sop-autonomy/`.
- [plans/PLAN-Repository-Consolidation.md](plans/PLAN-Repository-Consolidation.md) — the repository-consolidation plan (MIG-001–MIG-012): unify `sop-controller` and `sop-decision-adapters` into one Go module and one `sop` executable (with thin legacy wrappers) while preserving histories and governance. The consolidation merge is **committed** (2026-10-14) and the plan remains **HISTORICALIZED — COMPLETE**. The evidence is [reports/consolidation/](reports/consolidation/completion.md) and the guide [guides/REPOSITORY-CONSOLIDATION.md](guides/REPOSITORY-CONSOLIDATION.md); the file stays under `plans/` because SOP's archive records its path.

### Other plans in `plans/`

The remaining plan files, each with the status its own header declares. Run
`sop status` for the active plan in your checkout; this index does not record
operator-local lifecycle state. They stay in `plans/` rather than moving to `history/` where that keeps an operational path working; see [Canonical Locations and Kept Paths](#canonical-locations-and-kept-paths).

| Plan                                                                                                                     | Declared status           | Read it for                                                          |
| ------------------------------------------------------------------------------------------------------------------------ | ------------------------- | -------------------------------------------------------------------- |
| [plans/ACCEPTANCE-CHECKS-AUTHORING.md](plans/ACCEPTANCE-CHECKS-AUTHORING.md)                                             | guidance                  | how to author a plan stage's deterministic acceptance checks         |
| [plans/ISSUE-Prompt-Report-Deliverables.md](plans/ISSUE-Prompt-Report-Deliverables.md)                                   | OPEN (deferred)           | the ad-hoc prompt report-deliverable `NO_CHANGES_PRODUCED` follow-up |
| [plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md](plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md) | PLANNED                   | decision-boundary readiness for Clef-Flash                           |
| [plans/PLAN-Decision-Model-Effectiveness-Evaluation.md](plans/PLAN-Decision-Model-Effectiveness-Evaluation.md)           | EVALUATION COMPLETE       | the decision-model effectiveness evaluation                          |
| [plans/PLAN-EV-Context-Run-Evaluation.md](plans/PLAN-EV-Context-Run-Evaluation.md)                                       | PROPOSED                  | the context-engine live-path evaluation                              |
| [plans/PLAN-End-to-End-One-Shot-Hardening.md](plans/PLAN-End-to-End-One-Shot-Hardening.md)                               | IMPLEMENTED               | the end-to-end one-shot hardening (ONESHOT-001)                      |
| [plans/PLAN-Implementation-Convergence.md](plans/PLAN-Implementation-Convergence.md)                                     | PROPOSED                  | implementation-convergence work                                      |
| [plans/PLAN-JEV-Wave-2-Hardening.md](plans/PLAN-JEV-Wave-2-Hardening.md)                                                 | (see file)                | JEV Wave 2 end-to-end hardening                                      |
| [plans/PLAN-JEV-Wave-3-Adapter-Integration.md](plans/PLAN-JEV-Wave-3-Adapter-Integration.md)                             | APPROVED for registration | JEV Wave 3 adapter integration                                       |
| [plans/PLAN-Phase-9-Public-Launch.md](plans/PLAN-Phase-9-Public-Launch.md)                                               | (see file)                | Phase 9 public launch and developer adoption                         |
| [plans/PLAN-Post-Wave3-Production-Readiness.md](plans/PLAN-Post-Wave3-Production-Readiness.md)                           | PROPOSED                  | post-Wave-3 production readiness                                     |
| [plans/PLAN-Provider-Neutral-Decision-Integration-Seam.md](plans/PLAN-Provider-Neutral-Decision-Integration-Seam.md)     | PLANNED                   | the provider-neutral decision integration seam                       |
| [plans/PLAN-Review-Scope-And-Output-Hardening.md](plans/PLAN-Review-Scope-And-Output-Hardening.md)                       | PROPOSED                  | review scope and malformed-output hardening                          |
| [plans/PLAN-Risk-Based-Verification.md](plans/PLAN-Risk-Based-Verification.md)                                           | declared ACTIVE           | risk-based verification and risk-tiered execution                    |
| [plans/PLAN-RSH-Follow-Up-Hardening.md](plans/PLAN-RSH-Follow-Up-Hardening.md)                                           | PROPOSED                  | RSH follow-up hardening                                              |
| [plans/PLAN-Run-Metrics-And-Observability.md](plans/PLAN-Run-Metrics-And-Observability.md)                               | PROPOSED                  | run metrics and observability                                        |
| [plans/PLAN-SOP-AUTONOMY-001-UPDATED.md](plans/PLAN-SOP-AUTONOMY-001-UPDATED.md)                                         | PROPOSED UPDATE           | a proposed update to the SOP autonomy plan                           |
| [plans/PLAN-SOP-Acceptance-Verification-Hardening.md](plans/PLAN-SOP-Acceptance-Verification-Hardening.md)               | IMPLEMENTED               | SOP acceptance-verification hardening (SOP-HARD)                     |
| [plans/PLAN-SOP-E2E-Reliability-Hardening.md](plans/PLAN-SOP-E2E-Reliability-Hardening.md)                               | PROPOSED                  | SOP E2E reliability hardening                                        |
| [plans/PLAN-SOP-End-to-End-Reliability.md](plans/PLAN-SOP-End-to-End-Reliability.md)                                     | PROPOSED                  | SOP end-to-end reliability                                           |
| [plans/PLAN-SOP-Lifecycle-Consistency-Hardening.md](plans/PLAN-SOP-Lifecycle-Consistency-Hardening.md)                   | PROPOSED                  | SOP lifecycle-consistency hardening                                  |
| [plans/PLAN-SOP-RUN-ACCEPT-001.md](plans/PLAN-SOP-RUN-ACCEPT-001.md)                                                     | declared ACTIVE           | native deterministic acceptance checks for `sop run`                 |
| [plans/PLAN-SOP-Validation-Evidence-Reuse.md](plans/PLAN-SOP-Validation-Evidence-Reuse.md)                               | IMPLEMENTED               | SOP validation evidence reuse                                        |

### Planner spike and retained execution records

These files combine plans and point-in-time evidence. Their existing paths and
recorded outcomes are preserved; they do not identify the currently active plan.

- [plans/RAG-SOP-003-Phase-4B-Execution.md](plans/RAG-SOP-003-Phase-4B-Execution.md) — read for the scoped runtime-context integration execution instructions and acceptance checks.
- [plans/SOP-Planner-Connection-Spike.md](plans/SOP-Planner-Connection-Spike.md) — read for the planner connection spike plan and its acceptance criteria.
- [plans/SPIKE-001-baseline-inventory.md](plans/SPIKE-001-baseline-inventory.md) — read for the recorded baseline inventory for the planner spike.
- [plans/SPIKE-001-disposable-environment.md](plans/SPIKE-001-disposable-environment.md) — read for the disposable target repository and artifact capture setup.
- [plans/SPIKE-002-connection-discovery.md](plans/SPIKE-002-connection-discovery.md) — read for the operator-designated planner client and connection discovery record.
- [plans/SPIKE-002-planner-client-transport.md](plans/SPIKE-002-planner-client-transport.md) — read for the original transport discovery and its superseded client-naming gap.
- [plans/SPIKE-003-closure-checklist.md](plans/SPIKE-003-closure-checklist.md) — read for the recorded human-gate closure checklist, without executing it.
- [plans/SPIKE-003-repair-proposal.md](plans/SPIKE-003-repair-proposal.md) — read for the proposed question retrieval and answer-submission repair.
- [plans/SPIKE-004-implementation-proposal.md](plans/SPIKE-004-implementation-proposal.md) — read for the proposed durable executor-question continuation design.
- [plans/SPIKE-004-live-evidence.md](plans/SPIKE-004-live-evidence.md) — read for the recorded live ChatGPT continuation round trip.
- [plans/SPIKE-005-verification-report.md](plans/SPIKE-005-verification-report.md) — read for the restart, timeout, stale-state, duplicate, and authority verification evidence.
- [plans/SPIKE-006-feasibility-report.md](plans/SPIKE-006-feasibility-report.md) — read for the scoped operator-assisted planner feasibility verdict and limitations.
- [plans/SPIKE-006-live-transcript.md](plans/SPIKE-006-live-transcript.md) — read for the sanitized live two-question demonstration transcript.
- [plans/planner-live-test-findings.md](plans/planner-live-test-findings.md) — read for the preserved live planner runs and their source-grounded findings.
- [plans/closure-2026-10-16.md](plans/closure-2026-10-16.md) — read for the dated repository consolidation closure reconciliation and its verification limits.

## Testing

- [testing/SOP-END-TO-END.md](testing/SOP-END-TO-END.md) — end-to-end skill assessment, authoritative delegation contract, twelve deterministic scenarios, and host-assistant evaluation limits.
- [testing/WINDOWS-CLEAN-ROOM.md](testing/WINDOWS-CLEAN-ROOM.md) — the clean-room Windows test checklist and its results log; real-machine results stay marked **NOT YET RUN** until they are actually performed, separately from CI.

## Reports

The [report index](reports/README.md) lists every tracked Markdown report, fixture
write-up, and run transcript by evidence group.

Point-in-time evidence reports produced by a task. They record what was measured or
decided at a moment in time and are non-normative: the current behavior they concern
is owned by the documents they link.

- [reports/PERFORMANCE-BASELINE.md](reports/PERFORMANCE-BASELINE.md) — the pre-performance baseline (CLOSE-010): the measured numbers future performance work is compared against. The measurement model is [reference/PERFORMANCE.md](reference/PERFORMANCE.md).
- [reports/PHASE-9-MULTI-AGENT-ADOPTION.md](reports/PHASE-9-MULTI-AGENT-ADOPTION.md) — the Phase 9 adoption decision (ORCH-012): multi-agent orchestration is verified, and the deterministic gate keeps it **opt-in** — default execution stays single-agent.
- [reports/pre-performance-closure/](reports/pre-performance-closure/) — the CLOSE-001–CLOSE-011 closure evidence (baselines, telemetry inventory, readiness); the verdict is [CLOSE-011-readiness.md](reports/pre-performance-closure/CLOSE-011-readiness.md).
- [reports/planner/SOP-PLANNER-CAP-001-capability-context.md](reports/planner/SOP-PLANNER-CAP-001-capability-context.md) — the per-task planner capability-context fix: the compiled capability inventory is now authoritative at the task-level planning boundary, so a repository discovery target is no longer promoted to an external prerequisite capability.
- [reports/decision-telemetry/TEL-001-reconciliation.md](reports/decision-telemetry/TEL-001-reconciliation.md) — the Decision Telemetry reconciliation and metric register (TEL-001/TEL-002): what existing telemetry already records, the new decision dimensions with their numerator/denominator/coverage, and the two dimensions (adapter agreement, confidence) that stay UNAVAILABLE because nothing records them.
- [reports/decision-telemetry/TEL-003-integration.md](reports/decision-telemetry/TEL-003-integration.md) — the TEL-003 decision-adapter integration: the `sop-decision-adapters` shadow harness emits a versioned `shadow-evidence.json` artifact, read read-only by `internal/runmetrics` to compute adapter invocation, agreement, confidence, latency, and invalid/indeterminate/timeout rates with explicit denominators. **Implemented** (both repositories).
- [reports/decision-telemetry/TEL-006-cli-report.md](reports/decision-telemetry/TEL-006-cli-report.md) — the TEL-006 CLI: the opt-in, read-only `sop report --decisions [--format text\|json]` command over the `internal/runmetrics` aggregator, with sample output, coverage limits, and verification evidence.
- [reports/decision-telemetry/TEL-008-model-selection.md](reports/decision-telemetry/TEL-008-model-selection.md) — the TEL-008 `Model Selection` percentage summary (SMALL/MEDIUM/LARGE counts and one-decimal percentages, explicit missing coverage) added to the `--decisions` text output; JSON and goldens unchanged.
- [reports/decision-model-evaluation-2026-10-13.md](reports/decision-model-evaluation-2026-10-13.md) — the 2026-10-13 decision-model evaluation rerun and reconciliation: frozen-versus-rerun results with explicit denominators, findings, reproducibility limits, and the retained DISABLE recommendation; follow-ups are DME-FU-004–DME-FU-007 in [plans/BACKLOG.md](plans/BACKLOG.md).
- [reports/consolidation/](reports/consolidation/completion.md) — the repository-consolidation evidence (MIG-001–MIG-012): baseline, source map, import rewrites/verification, decision compatibility, CI coverage, documentation, dogfood, performance and the completion report. The controller licensing record is [controller-provenance.md](reports/consolidation/controller-provenance.md); the durable recovery snapshot is [backup-manifest.md](reports/consolidation/backup-manifest.md).
- [reports/consolidation/closure-evidence.md](reports/consolidation/closure-evidence.md) — the repository-consolidation closure evidence report: the operator-authorized relative link check (`broken links: 0`) and `git diff --check` results, the `go build/test/vet` validation gates, provenance separating local, committed and merged evidence, the external-completion reasons for C5/C7, and the unverified boundaries (archived state, migration CI, legacy retirement, provider limits).

## Historical Documentation

The [historical plan index](history/plans/README.md) lists each retained plan with
its original scope and recorded disposition.

Point-in-time or superseded artifacts. Non-normative: they do not define current behavior.

- [history/PLAN-wrapup.md](history/PLAN-wrapup.md) — the original implementation plan.
- [history/OLLAMA-DOGFOOD.md](history/OLLAMA-DOGFOOD.md) — the Ollama/DeepSeek dogfood test.
- [history/PHASE-3-DOGFOOD.md](history/PHASE-3-DOGFOOD.md) — the Phase 3 early-JEV dogfood: the deterministic fake-analyzer demonstration and the (not performed) real-provider procedure.
- [history/PLAN-Model-Routing.md](history/PLAN-Model-Routing.md) — the early, superseded model-routing design (MODELRT001–MODELRT022); the shipped rules live in [specs/MODEL-ROUTING.md](specs/MODEL-ROUTING.md).
- [history/RESOLVED-BACKLOG.md](history/RESOLVED-BACKLOG.md) — completed backlog observations and superseded agent-provider contracts, preserved as non-normative evidence.
- [history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md](history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md) — Phase 8 (Context & Execution Efficiency): **COMPLETE**; all twelve CTX tasks reached a terminal state and SOP archived the plan via `sop plan complete`.
- [history/plans/POST8-001-PHASE-8-LIVE-PATH-INTEGRATION-GOVERNANCE.md](history/plans/POST8-001-PHASE-8-LIVE-PATH-INTEGRATION-GOVERNANCE.md) — POST8-001: **COMPLETE** post-Phase-8 governance that validated the merged live-path integration before Phase 9.
- [history/PREJEV-BASELINE.md](history/PREJEV-BASELINE.md) — read for Pre-JEV Baseline (PREJEV001) evidence and recorded scope.
- [history/PREJEV004-AHV2009-PROOF.md](history/PREJEV004-AHV2009-PROOF.md) — read for PREJEV004 — AHV2009 End-to-End Proof evidence and recorded scope.
- [history/PREJEV005-AHV2001-2007-RECONCILE.md](history/PREJEV005-AHV2001-2007-RECONCILE.md) — read for PREJEV005 — Reconcile AHV2001 Through AHV2007 evidence and recorded scope.
- [history/PREJEV006-AHV2010-2013-RECONCILE.md](history/PREJEV006-AHV2010-2013-RECONCILE.md) — read for PREJEV006 — Complete Remaining Harness V2 Tasks (AHV2010–AHV2013) evidence and recorded scope.
- [history/PREJEV012-RECONCILE.md](history/PREJEV012-RECONCILE.md) — read for PREJEV012 — Reconciliation State evidence and recorded scope.
- [history/PREJEV012-REGRESSION-DECOMPOSITION.md](history/PREJEV012-REGRESSION-DECOMPOSITION.md) — read for PREJEV012 — Full Agent Harness Regression Suite (Decomposition) evidence and recorded scope.
- [history/PREJEV016-RECOVERY-DECOMPOSITION.md](history/PREJEV016-RECOVERY-DECOMPOSITION.md) — read for PREJEV016 — Verify Recovery Paths (Decomposition) evidence and recorded scope.
- [history/PREJEV017-PERFORMANCE-BASELINE.md](history/PREJEV017-PERFORMANCE-BASELINE.md) — read for PREJEV017 — Pre-JEV Performance Baseline evidence and recorded scope.
- [history/PREJEV017-PERFORMANCE-DECOMPOSITION.md](history/PREJEV017-PERFORMANCE-DECOMPOSITION.md) — read for PREJEV017 — Capture Pre-JEV Performance Baseline (Decomposition) evidence and recorded scope.
- [history/PREJEV018-READINESS-GATE.md](history/PREJEV018-READINESS-GATE.md) — read for PREJEV018 — Pre-JEV Readiness Gate (Decomposition) evidence and recorded scope.
- [history/pre-performance-closure/](history/pre-performance-closure/) — the superseded interim closure reports (CLOSE-002/005/006/007); the final verdict is [reports/pre-performance-closure/CLOSE-011-readiness.md](reports/pre-performance-closure/CLOSE-011-readiness.md).
- [history/pre-performance-closure/CLOSE-005-controller-work-verdicts.md](history/pre-performance-closure/CLOSE-005-controller-work-verdicts.md) — read for the superseded interim controller-work verdicts.
- [history/pre-performance-closure/CLOSE-006-named-plan-dogfood.md](history/pre-performance-closure/CLOSE-006-named-plan-dogfood.md) — read for the superseded interim named-plan dogfood record.
- [history/pre-performance-closure/CLOSE-007-resume-idempotency.md](history/pre-performance-closure/CLOSE-007-resume-idempotency.md) — read for the superseded interim resume and idempotency record.
- [history/developer-preview/](history/developer-preview/README.md) — the superseded Developer Preview deliverables (SCOPE, QUICKSTART-VERIFICATION, KNOWN-ISSUES, READINESS) with their operator-authored provenance labelled; the readiness verdict is **HOLD**. The superseded plan remains at [plans/PLAN-SOP-Developer-Preview.md](plans/PLAN-SOP-Developer-Preview.md).

## Canonical Locations and Kept Paths

- **The closure plan's wrap-up reference targets the located historical plan.** [plans/PLAN-Pre-Performance-Closure.md](history/plans/PLAN-Pre-Performance-Closure.md) formerly named the absent `docs/plans/PLAN-Wrap-Up.md`; it now points to the located wrap-up plan [history/PLAN-wrapup.md](history/PLAN-wrapup.md). Earlier reconciliation recorded the old path as **NOT LOCATED** in [history/pre-performance-closure/CLOSE-002-status-reconciliation.md](history/pre-performance-closure/CLOSE-002-status-reconciliation.md), CLOSE-005 and CLOSE-006. The correction was reconciled through SOP (`sop reconcile … --accept-changed CLOSE-002 --accept-changed CLOSE-006`), so the recorded plan fingerprint matches the file and no executed task changed state.

To keep SOP working, a few paths are intentionally not under the categories above:

- SOP's recorded active source (`.agent-sdlc/plan.meta.json`) is operational. Never
  move or reconstruct it as documentation cleanup. A missing recorded source or
  unresolved approval is a separate SOP lifecycle issue; documentation cleanup
  does not repair the state database or declare the plan complete.
- Planner spike evidence remains beside its source plan under `plans/`; its
  cross-references and recorded execution paths are preserved.

- [PRD.md](PRD.md) and [PLAN.md](PLAN.md) stay at `docs/` because SOP's planner discovers `docs/PRD.md` and `docs/PLAN.md` (`internal/planflow`).
- Historical plans already under [history/plans/](history/plans/README.md) stay at their existing paths. Completed plans still referenced at `plans/` are not moved by documentation cleanup; lifecycle transitions belong to SOP.
- [plans/PLAN-Repository-Consolidation.md](plans/PLAN-Repository-Consolidation.md) is retained (HISTORICALIZED — COMPLETE) because the SOP archive `.agent-sdlc/archive/plan-repository-consolidation/` records its source path; the reports under [reports/consolidation/](reports/consolidation/completion.md) are its evidence.
- [tasks/](tasks/) stays at `docs/tasks/` because plans invoke `sop run --task docs/tasks/...`. These are per-task specifications and recorded evidence; later fixes can supersede their original expectations. [TASK-INDEX.md](reference/TASK-INDEX.md) points to current authority without changing those operational files.

## Checking Documentation Links

Documentation links and heading anchors are checked before publishing. Run the repository link check over `docs/**/*.md`
and `README.md` and their relative links:

```bash
scripts/checks/check-doc-links.sh
```

It prints `broken links: <count>` (for example `broken links: 0`) and exits
non-zero when any relative link does not resolve. An anchor-aware check can also be run with the doc-reconcile skill's `scripts/link_check.py`.
