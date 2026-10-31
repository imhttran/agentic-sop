# Plans

Implementation and verification plans for SOP. This is the plan inventory; the
top-level documentation index is [../README.md](../README.md).

A completed plan stays in `plans/` rather than moving to `history/plans/` when SOP
or the repository still references its path: the archive under `.agent-sdlc/archive/`
records these sources, and the example commands in `scripts/agents/sop-agent.sh` name
one of them. Moving such a plan would break `sop run`/`resume`/`reconcile`, so the
path is kept and the status is declared here. Files that SOP has recorded as COMPLETE
or SUPERSEDED live under [../history/plans/](../history/plans/README.md) once no
active plan references them. Run `sop status` for the active plan in your checkout;
this index does not record operator-local lifecycle state.

## Notable plans

- [PLAN-Executore-Integration.md](PLAN-Executore-Integration.md) — the active four-stage planner–executor plan. PE-1 through PE-4 implementation and live acceptance are merged: planners can retrieve bound task/decision context and submit non-approval advice, executors can durably publish results and consume applied decisions, and continuation recovery is bound to the durable run epoch. The merged [PE-4 evidence](../reports/evidence/artifacts/pe4-acceptance/README.md) records all four tasks as `LOCAL_DONE`; plan completion remains a separate operator-local SOP lifecycle action. Unattended invocation is not claimed.
- [PROMPTFOO-COVERAGE-GAP.md](PROMPTFOO-COVERAGE-GAP.md) — the preserved lifecycle/evidence-association gap for the merged Promptfoo evaluation work; verify-first records do not rewrite the original task verdicts.
- [HIP-006-pilot-evidence.md](HIP-006-pilot-evidence.md) and [HIP-007-adoption-report.md](HIP-007-adoption-report.md) — the bounded `sop compare` pilot evidence and adoption verdict: no candidate promoted; effectiveness remains inconclusive.
- [BACKLOG.md](BACKLOG.md) — known gaps and future candidates (the running list between phases), including the decision-model evaluation follow-ups (DME-FU-001–005) and their promotion criteria [AI-DECISION-PROMOTION-CRITERIA.md](../reference/AI-DECISION-PROMOTION-CRITERIA.md).
- [PLAN-RAG-SOP-Integration.md](PLAN-RAG-SOP-Integration.md) — the RAG integration plan (RAG-SOP-001–004): architecture, adapter implementation, required PostgreSQL integration validation, comparative evaluation, and the operator rollout decision. The evaluation criteria are fixed in [RAG-EVALUATION.md](../specs/RAG-EVALUATION.md) and the recorded outcome is [RAG-SOP-004-DECISION.md](../specs/RAG-SOP-004-DECISION.md).
- [PHASE-9-MULTI-AGENT-ORCHESTRATION.md](../history/plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md) — Phase 9 (ORCH-001–ORCH-012): the deterministic multi-agent orchestration system. **Delivery record**: all twelve tasks are `LOCAL_DONE`; the adoption gate ([PHASE-9-MULTI-AGENT-ADOPTION.md](../reports/PHASE-9-MULTI-AGENT-ADOPTION.md)) keeps multi-agent **opt-in**, so single-agent stays the default.
- [PLAN-Pre-Performance-Closure.md](../history/plans/PLAN-Pre-Performance-Closure.md) — **COMPLETE**: the executed CLOSE-001–CLOSE-011 DAG reconciled and verified both SOP and sop-controller, dogfooded and resumed, inventoried existing metrics, measured four workloads, and recorded the readiness verdict. All eleven tasks are `LOCAL_DONE` and the plan's final gate passed; it introduces no new performance architecture and does not start the performance phase.
- [PLAN-Phase-3.5-Model-Routing.md](../history/plans/PLAN-Phase-3.5-Model-Routing.md) — Phase 3.5: JEV-guided per-task model routing. **Implemented**; the historical document preserves its task definitions; the router is OFF by default, opt-in via `SOP_MODEL_ROUTING_ENABLED=true`.
- [PLAN-Phase-4-Provider-Runtime.md](PLAN-Phase-4-Provider-Runtime.md) — Phase 4: provider/runtime abstraction, capability discovery, and opt-in model validation. **Implemented** (off by default); see [PROVIDERS.md](../specs/PROVIDERS.md).
- [PLAN-Phase-5-Execution-Recovery.md](PLAN-Phase-5-Execution-Recovery.md) — Phase 5: bounded model escalation and execution recovery. **Implemented** (off by default; opt-in via `SOP_MODEL_ESCALATION_ENABLED=true`); see [RECOVERY.md](../specs/RECOVERY.md) §8.
- [PLAN-Phase-5.4-Unified-Work-Items.md](PLAN-Phase-5.4-Unified-Work-Items.md) — Phase 5.4: unified work items, `sop prompt`, and the SOP skill. **Implemented**; see [WORK-ITEMS.md](../specs/WORK-ITEMS.md) and [PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md).
- [PLAN-Phase-5.5-Distribution.md](PLAN-Phase-5.5-Distribution.md) — Phase 5.5 (P55-001–P55-011): the unified `install.sh`, the per-agent skill installation it delegates, the Claude Code plugin package, and CI. **Implemented**; see [INSTALLATION.md](../guides/INSTALLATION.md) and [CLAUDE-PLUGIN.md](../guides/CLAUDE-PLUGIN.md).
- [PLAN-Phase-5.6-Windows-Distribution.md](PLAN-Phase-5.6-Windows-Distribution.md) — Phase 5.6 (P56-001–P56-012): native Windows installation and a script-organization cleanup. **Implemented except P56-011**, the clean-room test on a real Windows machine, which has not been run; see [WINDOWS-INSTALLATION.md](../guides/WINDOWS-INSTALLATION.md) and [WINDOWS-CLEAN-ROOM.md](../testing/WINDOWS-CLEAN-ROOM.md).
- [PLAN-Phase-6-Interactive-Approval.md](PLAN-Phase-6-Interactive-Approval.md) — Phase 6 (P6-001–P6-011): make SOP's human decision points operable and discoverable by a client — enumerate the gates, list a plan's changed executed tasks before mutating, name the gate a run parked at, and decide interactively. **Implemented**: the read surface (`P6-001`–`P6-004`, `sop approvals` and `sop reconcile --list-changed`) and the interactive surface (`P6-005`–`P6-011`) both shipped; the human workflow is in [APPROVALS.md](../guides/APPROVALS.md). See [HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) and [CLI.md](../reference/CLI.md).
- [PLAN-Phase-3-OpenJEV.md](../history/plans/PLAN-Phase-3-OpenJEV.md) — Phase 3 implementation plan (P3-001–P3-017): early JEV decision layer (task triage + pre-execution), disabled by default. **Implemented** (P3-001–P3-015) and committed; P3-016 records the dogfood.
- [PLAN-JEV-Implementation.md](../history/plans/PLAN-JEV-Implementation.md) — the V1 JEV implementation plan (JEV001–JEV017). **Implemented**; archived by SOP. Read the historical plan for the original task definitions; current execution instructions are in [GETTING-STARTED.md](../guides/GETTING-STARTED.md).
- [PLAN-Agent-Harness-V2.md](../history/plans/PLAN-Agent-Harness-V2.md) — the Agent Harness V2 workstream. **Implemented**; archived by SOP (`.agent-sdlc/archive/ahv2/`).
- [PLAN-Pre-JEV-Stabilization.md](../history/plans/PLAN-Pre-JEV-Stabilization.md) — the pre-JEV stabilization workstream. **Implemented**; archived by SOP.
- [PLAN-Automatic-Blocked-Task-Recovery.md](../history/plans/PLAN-Automatic-Blocked-Task-Recovery.md) — automatic blocked-task recovery. **Implemented**; archived by SOP.
- [PLAN-SOP-Performance.md](PLAN-SOP-Performance.md) — performance and timing. **Implemented** (PERF008 and PERF010–PERF012, PERF014 deferred with evidence); the shipped behavior is described in [PERFORMANCE.md](../reference/PERFORMANCE.md).
- [PLAN-Ollama-Agent-Plan-Synthesis.md](PLAN-Ollama-Agent-Plan-Synthesis.md) — Ollama PLAN discovery and synthesis. **Implemented**; PLAN runs the bounded DISCOVERY → tool-free SYNTHESIS lifecycle.
- [PLAN-Decision-Telemetry-Extension.md](PLAN-Decision-Telemetry-Extension.md) — Decision Telemetry (TEL-001–TEL-008): extend the run-metrics aggregator with the decisions SOP records (model tier, override, escalation, JEV invocation) without changing decision authority. **Complete**; the model-tier, override, escalation, and JEV dimensions are implemented in `internal/runmetrics`, and adapter agreement/confidence are read from the shadow-evidence artifact when present and reported UNAVAILABLE otherwise (see [TEL-003-integration.md](../reports/decision-telemetry/TEL-003-integration.md)).
- [PLAN-TEL-009-Telemetry-Reporting-Corrections.md](PLAN-TEL-009-Telemetry-Reporting-Corrections.md) — TEL-009: correct `decision_escalation_rate` (model escalation only, not human approvals), reconcile `decision_override_rate` provenance, make `retry_efficiency` a rate, and version `model-selection.json`. **Implemented**; reporting/evidence only.
- [PLAN-SOP-Developer-Preview.md](../history/plans/PLAN-SOP-Developer-Preview.md) — the Developer Preview workstream (PREVIEW-001–005). **SUPERSEDED**: disposed by SOP without executing PREVIEW-002–PREVIEW-005; the deliverables and the **HOLD** readiness verdict are preserved as evidence in [../history/developer-preview/](../history/developer-preview/README.md), and the still-open findings are carried in [BACKLOG.md](BACKLOG.md) § Developer Preview carry-forward.
- [PLAN-SOP-Autonomy.md](../history/plans/PLAN-SOP-Autonomy.md) — the SOP autonomy workstream (AUTONOMY-001/004/005), consolidated into a single authoritative plan. **HISTORICALIZED — COMPLETE** (2026-10-13): all three tasks reached `LOCAL_DONE` and SOP retained the audit record at `.agent-sdlc/archive/plan-sop-autonomy/`.
- [PLAN-Repository-Consolidation.md](../history/plans/PLAN-Repository-Consolidation.md) — the repository-consolidation plan (MIG-001–MIG-012): unify `sop-controller` and `sop-decision-adapters` into one Go module and one `sop` executable (with thin legacy wrappers) while preserving histories and governance. The consolidation merge is **committed** (2026-10-14) and the plan remains **HISTORICALIZED — COMPLETE**. The evidence is [../reports/consolidation/](../reports/consolidation/completion.md) and the guide [REPOSITORY-CONSOLIDATION.md](../guides/REPOSITORY-CONSOLIDATION.md); the file now lives under `history/plans/`, while SOP's archive records its original source path.

## Remaining plan files

The remaining plan files, each with the status its own header declares. Run
`sop status` for the active plan in your checkout; this index does not record
operator-local lifecycle state.

### Plans kept in `plans/`

Files whose path SOP or the repository still references, so they stay in `plans/`
rather than moving to the archive (see
[Canonical Locations and Kept Paths](../README.md#canonical-locations-and-kept-paths)).

| Plan | Declared status | Read it for |
| --- | --- | --- |
| [PLAN-Job-Lifecycle-Notifications.md](PLAN-Job-Lifecycle-Notifications.md) | IMPLEMENTED | durable job lifecycle notifications (NTFY-001–NTFY-008, issue #138); merged in PR #139; Slack-format webhook delivery was verified operator-locally on 2026-10-29, without default enablement, see [NOTIFICATIONS.md](../guides/NOTIFICATIONS.md) |
| [PLAN-Stale-Approval-Invalidation.md](PLAN-Stale-Approval-Invalidation.md) | IMPLEMENTED | audited invalidation of stale `PENDING` approval evidence after a proven accepted task-definition change; merged in PR #141 |
| [ACCEPTANCE-CHECKS-AUTHORING.md](ACCEPTANCE-CHECKS-AUTHORING.md) | guidance | how to author a plan stage's deterministic acceptance checks |
| [ISSUE-Prompt-Report-Deliverables.md](ISSUE-Prompt-Report-Deliverables.md) | OPEN (deferred) | the ad-hoc prompt report-deliverable `NO_CHANGES_PRODUCED` follow-up |
| [PLAN-Decision-Model-Effectiveness-Evaluation.md](PLAN-Decision-Model-Effectiveness-Evaluation.md) | EVALUATION COMPLETE | the decision-model effectiveness evaluation |
| [PLAN-End-to-End-One-Shot-Hardening.md](PLAN-End-to-End-One-Shot-Hardening.md) | IMPLEMENTED | the end-to-end one-shot hardening (ONESHOT-001) |
| [PLAN-Phase-9-Public-Launch.md](PLAN-Phase-9-Public-Launch.md) | (see file) | Phase 9 public launch and developer adoption |
| [PLAN-Provider-Neutral-Decision-Integration-Seam.md](PLAN-Provider-Neutral-Decision-Integration-Seam.md) | PLANNED | the provider-neutral decision integration seam |
| [PLAN-SOP-AUTONOMY-001-UPDATED.md](PLAN-SOP-AUTONOMY-001-UPDATED.md) | PROPOSED UPDATE | a proposed update to the SOP autonomy plan |
| [PLAN-SOP-Acceptance-Verification-Hardening.md](PLAN-SOP-Acceptance-Verification-Hardening.md) | IMPLEMENTED | SOP acceptance-verification hardening (SOP-HARD) |
| [PLAN-SOP-E2E-Reliability-Hardening.md](PLAN-SOP-E2E-Reliability-Hardening.md) | PROPOSED | SOP E2E reliability hardening |
| [PLAN-SOP-RUN-ACCEPT-001.md](PLAN-SOP-RUN-ACCEPT-001.md) | declared ACTIVE | native deterministic acceptance checks for `sop run` |
| [PLAN-SOP-Validation-Evidence-Reuse.md](PLAN-SOP-Validation-Evidence-Reuse.md) | IMPLEMENTED | SOP validation evidence reuse |
| [PLAN-Security-Scanner-Addon.md](PLAN-Security-Scanner-Addon.md) | IMPLEMENTED | the Semgrep CE and Gitleaks security scanner add-on (SEC-001–SEC-005); the path is kept because `internal/planner/security_plan_test.go` compiles this plan |
| [PLAN-Issue-158-Deliverable-Paths.md](PLAN-Issue-158-Deliverable-Paths.md) | PROPOSED | one outcome criterion per deliverable line and no descriptive symbol treated as a path (issue #158); the header still says PROPOSED although PR #163 merged the fix |
| [PLAN-Locality-Configuration-Cleanup.md](PLAN-Locality-Configuration-Cleanup.md) | PROPOSED | resolving model locality from explicit configuration or reliable evidence instead of repeating it; the header still says PROPOSED although PR #157 merged the change |
| [VALIDATION-SCOPED-TESTS.md](VALIDATION-SCOPED-TESTS.md) | design (implemented on `feat/scoped-validation`) | scoped task validation with a final full-suite gate |
| [S1-PROMPTFOO-INTEGRATION-DECISION.md](S1-PROMPTFOO-INTEGRATION-DECISION.md) | decision record | the selection of one optional Promptfoo adapter and no second workflow runner (stage S1) |
| [S3-AISEC-ACCEPTANCE.md](S3-AISEC-ACCEPTANCE.md) | VERIFY | verification of the AISEC suite against the current evaltarget (stage S3V) |
| [S4-ACCEPTANCE.md](S4-ACCEPTANCE.md) | VERIFY | verification of Promptfoo configuration, reporting, and normalization (stage S4V) |
| [S5-ACCEPTANCE.md](S5-ACCEPTANCE.md) | VERIFY | verification of Promptfoo delivery readiness (stage S5V) |
| [HIP-001-bootstrap-baseline.md](HIP-001-bootstrap-baseline.md) | BOOTSTRAP NOTE | the isolated environment and baseline record for the Harness Improvement Pilot |
| [HIP-002-existing-facilities-and-choices.md](HIP-002-existing-facilities-and-choices.md) | INSPECTION NOTE | the existing facilities, integration points, and implementation choices for the pilot |
| [HIP-005-pilot-selection.md](HIP-005-pilot-selection.md) | recorded 2026-10-23 | the fixed pilot task selection and candidate revision, recorded before any candidate ran |
| [HIP-008-delivery.md](HIP-008-delivery.md) | delivery record | the pilot's delivery record (PR #114) |

### Plans under `history/plans/`

Files kept in the historical archive. The location records the disposition; the
status column shows what each file's own header declares, which can still be
non-terminal, so read the file for its authority.

| Plan | Declared status | Read it for |
| --- | --- | --- |
| [PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md](../history/plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md) | PLANNED | decision-boundary readiness for Clef-Flash |
| [PLAN-EV-Context-Run-Evaluation.md](../history/plans/PLAN-EV-Context-Run-Evaluation.md) | PROPOSED | the context-engine live-path evaluation |
| [PLAN-Implementation-Convergence.md](../history/plans/PLAN-Implementation-Convergence.md) | PROPOSED | implementation-convergence work |
| [PLAN-JEV-Wave-2-Hardening.md](../history/plans/PLAN-JEV-Wave-2-Hardening.md) | (see file) | JEV Wave 2 end-to-end hardening |
| [PLAN-JEV-Wave-3-Adapter-Integration.md](../history/plans/PLAN-JEV-Wave-3-Adapter-Integration.md) | APPROVED for registration | JEV Wave 3 adapter integration |
| [PLAN-Post-Wave3-Production-Readiness.md](../history/plans/PLAN-Post-Wave3-Production-Readiness.md) | PROPOSED | post-Wave-3 production readiness |
| [PLAN-Review-Scope-And-Output-Hardening.md](../history/plans/PLAN-Review-Scope-And-Output-Hardening.md) | PROPOSED | review scope and malformed-output hardening |
| [PLAN-Risk-Based-Verification.md](../history/plans/PLAN-Risk-Based-Verification.md) | declared ACTIVE | risk-based verification and risk-tiered execution |
| [PLAN-RSH-Follow-Up-Hardening.md](../history/plans/PLAN-RSH-Follow-Up-Hardening.md) | PROPOSED | RSH follow-up hardening |
| [PLAN-Run-Metrics-And-Observability.md](../history/plans/PLAN-Run-Metrics-And-Observability.md) | PROPOSED | run metrics and observability |
| [PLAN-SOP-End-to-End-Reliability.md](../history/plans/PLAN-SOP-End-to-End-Reliability.md) | PROPOSED | SOP end-to-end reliability |
| [PLAN-SOP-Lifecycle-Consistency-Hardening.md](../history/plans/PLAN-SOP-Lifecycle-Consistency-Hardening.md) | PROPOSED | SOP lifecycle-consistency hardening |
| [PLAN-Security-Scan-Skill.md](../history/plans/PLAN-Security-Scan-Skill.md) | IMPLEMENTED (archived) | the Go/Rust security-scan skill and cargo-audit adapter; archived under `history/plans/`, while `.agent-sdlc/archive/plan-security-scan-skill/` keeps the original source path |
| [PLAN-Dependency-Coverage-Fixes.md](../history/plans/PLAN-Dependency-Coverage-Fixes.md) | IMPLEMENTED | Cargo workspace detection, dependency obligation inventory, and empty-analysis rejection |
| [PLAN-Security-Scanner-Residual-Findings.md](../history/plans/PLAN-Security-Scanner-Residual-Findings.md) | IMPLEMENTED | dependency-layout detection and freshness evidence correctness (PR #74) |
| [SOP-RAG-Decision-Memory-Plan.md](../history/plans/SOP-RAG-Decision-Memory-Plan.md) | IMPLEMENTED (opt-in, disabled by default) | local SQLite/FTS5 decision-memory capture and bounded advisory retrieval (PR #100); benefit remains unproven |
| [PROMPT-SOP-Harness-Improvement-Pilot.md](../history/plans/PROMPT-SOP-Harness-Improvement-Pilot.md) | prompt (historical) | the original implementation prompt for the Harness Improvement Pilot |
| [SOP-Promptfoo-Implementation-Prompt.md](../history/plans/SOP-Promptfoo-Implementation-Prompt.md) | prompt (historical) | the original implementation prompt for the optional Promptfoo security evaluations |

## Planner spike and retained execution records

These files combine plans and point-in-time evidence. Their existing paths and
recorded outcomes are preserved; they do not identify the currently active plan.

- [RAG-SOP-003-Phase-4B-Execution.md](RAG-SOP-003-Phase-4B-Execution.md) — read for the scoped runtime-context integration execution instructions and acceptance checks.
- [SOP-Planner-Connection-Spike.md](SOP-Planner-Connection-Spike.md) — read for the planner connection spike plan and its acceptance criteria.
- [SPIKE-001-baseline-inventory.md](SPIKE-001-baseline-inventory.md) — read for the recorded baseline inventory for the planner spike.
- [SPIKE-001-disposable-environment.md](SPIKE-001-disposable-environment.md) — read for the disposable target repository and artifact capture setup.
- [SPIKE-002-connection-discovery.md](SPIKE-002-connection-discovery.md) — read for the operator-designated planner client and connection discovery record.
- [SPIKE-002-planner-client-transport.md](SPIKE-002-planner-client-transport.md) — read for the original transport discovery and its superseded client-naming gap.
- [SPIKE-003-closure-checklist.md](SPIKE-003-closure-checklist.md) — read for the recorded human-gate closure checklist, without executing it.
- [SPIKE-003-repair-proposal.md](SPIKE-003-repair-proposal.md) — read for the proposed question retrieval and answer-submission repair.
- [SPIKE-004-implementation-proposal.md](SPIKE-004-implementation-proposal.md) — read for the proposed durable executor-question continuation design.
- [SPIKE-004-live-evidence.md](SPIKE-004-live-evidence.md) — read for the recorded live ChatGPT continuation round trip.
- [SPIKE-005-verification-report.md](SPIKE-005-verification-report.md) — read for the restart, timeout, stale-state, duplicate, and authority verification evidence.
- [SPIKE-006-feasibility-report.md](SPIKE-006-feasibility-report.md) — read for the operator-assisted planner feasibility verdict (GO for one or two questions with a real executor) and limitations.
- [SPIKE-006-live-transcript.md](SPIKE-006-live-transcript.md) — read for the sanitized live two-question demonstration transcript.
- [planner-live-test-findings.md](planner-live-test-findings.md) — read for the preserved live planner runs and their source-grounded findings.
- [closure-2026-10-16.md](../history/plans/closure-2026-10-16.md) — read for the dated repository consolidation closure reconciliation and its verification limits.
