# Report index

**Type:** Historical evidence index (non-normative)

Read these records for measurements, decisions, and task/run evidence at the
revision each file identifies. They do not establish current workflow state or
override [specifications](../README.md#specifications). Current delivery status is
in [PROJECT-STATUS.md](../reference/PROJECT-STATUS.md); future work is in
[BACKLOG.md](../plans/BACKLOG.md).

Captured `implementation.md`, `review.md`, and task/plan artifacts are indexed as
run evidence, not as instructions to execute. Associated JSON, logs, and patches
remain beside the Markdown records at their existing paths.

## Overview

- [CONV-001-convergence-baseline.md](CONV-001-convergence-baseline.md) — read for the recorded CONV-001 — Convergence Baseline (IMPLEMENT/FIX phased engine) evidence.
- [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](EGM-GOLDFISH-REVIEW-ASSESSMENT.md) — the authoritative independent design-review assessment and pre-registered A/B/C/D experiment protocol (nothing run; no quality claim). Records the verified review-path context limits, the capability comparison, and the protocol that owns the corpus, thresholds, PASS/FAIL/INCONCLUSIVE rules, and call budget. The conditional [PRD](../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md) links here and does not restate them.
- [INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md](INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md) — the feasibility assessment for the independent design-review experiment: corpus and determinability inventory, human-adjudication availability, the merged Workstream B execution evidence with its open runtime items, cost and INCONCLUSIVE risks, pilot readiness, and a **NOT YET READY** verdict. No experiment was run.
- [independent-design-review-corpus/READINESS.md](independent-design-review-corpus/READINESS.md) — the Workstream A corpus readiness summary: 23 candidate entries (5 `VERIFIED`, 7 `PROPOSED`, 3 `UNVERIFIED`, 8 `EXCLUDED`), with no strict proposed determinable defect, 10 partial candidates, and 2 held pending corpus-owner review (plus 3 unverified holds); the candidate manifest, verification, preparation plan, operator decision package, freeze procedure, and Workstream B reconciliation are linked. **NOT FROZEN.** §10 records the lead investigation and the strict and amended feasibility scenarios; §11 adds two post-#587 leads; §12 records Decision 1 (corpus owner); §13 records Decision 2 (historical dispositions); §14 records Decision 3 (eligibility); §15 records Decision 4 (amendment rejected); §16 records Decision 5 (seeder); §17 records Decision 6 preparation.
- [independent-design-review-corpus/HISTORICAL-LEAD-INVESTIGATION.md](independent-design-review-corpus/HISTORICAL-LEAD-INVESTIGATION.md) — read for the extraction of the six historical leads (§1–§7) and two post-#587 leads (§8): pinned revisions, artifact and permitted-reference SHA-256 values, ancestry checks, leakage checks, design-versus-implementation findings, and per-lead dispositions. Three plans carry four proposed-strict design defects and one plan carries a proposed-partial limitation; none is eligible, counted, or adjudicated. **NOT FROZEN.**
- [independent-design-review-corpus/CASE-CONSTRUCTION-SPECIFICATION.md](independent-design-review-corpus/CASE-CONSTRUCTION-SPECIFICATION.md) — read for how about eight seeded and six no-known-defect cases would be built, sealed, hashed, and independently checked, and for the role-independence and contamination controls. It constructs no case and creates no answer key. **0 constructed.**
- [independent-design-review-corpus/DECISION-RECORD-001-CORPUS-OWNER.md](independent-design-review-corpus/DECISION-RECORD-001-CORPUS-OWNER.md) — **APPROVED 2026-11-21**: Tom Tran is designated corpus owner for the EGM corpus and is excluded from independent adjudication. Records the decision, the operator identity, the date, what it does not authorize, and the pending state of decisions 2–9. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md](independent-design-review-corpus/DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md) — **APPROVED 2026-11-21**: the approved historical dispositions — 3 proposed strict plans / 4 defects and 11 proposed partial plans / 11 defects (include with review), 2 verified-defect holds, 3 unverified holds, 1 evidence-blocked hold, 8 confirmed exclusions, and 3 excluded leads. The record keeps approved disposition, proposed determinability, final eligibility, and independent adjudication separate; no candidate is promoted. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/DECISION-RECORD-003-ELIGIBILITY.md](independent-design-review-corpus/DECISION-RECORD-003-ELIGIBILITY.md) — **APPROVED 2026-11-21**: the conservative eligibility rule — partial and hindsight-only defects count toward neither historical minimum, the recall denominator is the frozen determinable defect set, and the seeded set is additional. The numeric minimums, arms, thresholds, and protocol text are unchanged; no candidate is promoted. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/DECISION-RECORD-004-PROTOCOL-AMENDMENT.md](independent-design-review-corpus/DECISION-RECORD-004-PROTOCOL-AMENDMENT.md) — **REJECTED (unnecessary) 2026-11-21**: the proposed eligibility amendment is rejected because Decision 3 already records the conservative rule; it would change no count. The registered protocol and its hash are unchanged; the proposal is preserved as historical evidence. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/DECISION-RECORD-005-SEEDER.md](independent-design-review-corpus/DECISION-RECORD-005-SEEDER.md) — **APPROVED 2026-11-21**: Tom Tran is designated seeder and is also corpus owner; the overlap is permitted by protocol §5.4 and recorded as a limitation. The independent case checker is not designated, the sealed-manifest location is not selected, and `case_construction_authorized` is **false**. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md](independent-design-review-corpus/DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md) — **PREPARATION ONLY; Decision 6 PENDING**: a source-referenced requirement matrix, per-candidate qualification checklist and blank records, role description, copy-ready recruitment message, conflict-of-interest declaration, blinding procedure, and the double-adjudication and kappa rules. No adjudicator is designated, contacted, or assessed. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md](independent-design-review-corpus/DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md) — **three clarifications APPROVED 2026-11-21; Decision 6 PENDING**: analyses the three adjudication ambiguities (pooled vs per-plan sampling, disagreement handling, second-adjudicator blindness) against protocol §§5.4, 6.1, and 6.2; recommends pooled sampling with `k = min(N, max(20, ceil(0.20 × N)))`, preserved independent labels, and a blind second submission; finds none requires a protocol amendment. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md](independent-design-review-corpus/DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md) — **APPROVED 2026-11-21, procedural clarifications only**: pooled sampling with `k = min(N, max(20, ceil(0.20 × N)))`, preserved independent labels with kappa reported and no reconciliation, and second-adjudicator blindness until both submissions are complete. Decision 6 adjudicator designation remains PENDING; no experiment activity is authorized. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/DECISION-006-ADJUDICATOR-RECRUITMENT-SHORTLIST.md](independent-design-review-corpus/DECISION-006-ADJUDICATOR-RECRUITMENT-SHORTLIST.md) — **NO VERIFIED CANDIDATES; Decision 6 PENDING**: records the candidate-discovery coverage and limitation (no public search available; only Tom Tran is a human Git identity and he is excluded), the repository identity evidence, the candidate profile, an empty shortlist with no invented names, the ten-step independence/COI workflow, and an operator-assisted outreach plan. **NOT FROZEN / NOT YET READY.**
- [independent-design-review-corpus/OPERATOR-DECISION-RECORD-DRAFT.md](independent-design-review-corpus/OPERATOR-DECISION-RECORD-DRAFT.md) — read for the prepared decision entries for decisions 1–6 (corpus owner, inclusion/exclusion, eligibility, amendment, seeder, adjudicators). Decisions 1–3 are approved, Decision 4 is rejected, and Decision 5 is approved in `DECISION-RECORD-001` through `DECISION-RECORD-005`; decision 6 remains an unrecorded draft with an empty approval block.
- [INDEPENDENT-DESIGN-REVIEW-PILOT-RESULTS.md](INDEPENDENT-DESIGN-REVIEW-PILOT-RESULTS.md) — read for the two four-call pilots of the independent design-review protocol (local oMLX and an Ollama cloud model): setup, per-run evidence hashes, the checks that passed, the invalid timing record in the first pilot, the single-model-call observation in the second, explicit ACP-ZED contamination limits, and what was not tested. Not an experiment result; nothing was adjudicated.
- [INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md](INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md) — read for the Workstream B verification of the independent design-review execution path: per-requirement results (model attribution, token accounting, harness, repository tools, run-directory collisions, retries, routing defaults), the pilot reconciliation, and the outstanding decisions. Not an experiment result.
- [independent-design-review-corpus/WORKSTREAM-B-RECONCILIATION.md](independent-design-review-corpus/WORKSTREAM-B-RECONCILIATION.md) — read for the post-merge mapping of the Workstream B evidence onto the protocol's execution-readiness requirements: VERIFIED, PARTIAL and UNVERIFIED results per requirement, the residual freeze dependencies, and the freeze/experiment effect. Not an experiment result and not an approval.
- [EVIDENCE-REUSE-VER-003-recovery.md](EVIDENCE-REUSE-VER-003-recovery.md) — read for the recorded VER-003 targeted recovery — provenance record evidence.
- [JEV-WAVE2-ADAPTER-COMPAT.md](JEV-WAVE2-ADAPTER-COMPAT.md) — read for the recorded JEV-206 — Adapter Compatibility Report (Julia-1 / Nimble) evidence.
- [PERFORMANCE-BASELINE.md](PERFORMANCE-BASELINE.md) — read for the recorded PERFORMANCE-BASELINE — Pre-Performance Baseline (CLOSE-010) evidence.
- [SECURITY-SCANNER-BASELINE.md](SECURITY-SCANNER-BASELINE.md) — read for the real-project report-only security scanner baseline: production/fixture separation, source-backed dispositions, coverage gaps, and required-mode readiness.
- [SECURITY-SCANNER-DOGFOOD.md](SECURITY-SCANNER-DOGFOOD.md) — read for the earlier report-only dogfood run: scanned content identity, measured duration, tool availability, the `INDETERMINATE` verdict, and the limitation that no live scanner completed.
- [SECURITY-SCAN-SKILL.md](SECURITY-SCAN-SKILL.md) — read for the Go/Rust security-scan skill and cargo-audit adapter evidence: detection, auto selection, Semgrep language coverage, pinned advisory-database fixtures, validation results, and remaining gaps.
- [SECURITY-FINDINGS-ISSUE-52.md](SECURITY-FINDINGS-ISSUE-52.md) — read for the issue #52 review: per-finding dispositions, the reviewed test-helper suppression, production-scope re-scan, and remaining coverage gaps.
- [SECURITY-DEPENDENCY-COVERAGE.md](SECURITY-DEPENDENCY-COVERAGE.md) — read for the dependency-coverage follow-up: Cargo workspace/obligation correctness, missing-input behavior, empty-analysis rejection, real-scanner evidence, and the report-only dogfood result.
- [SECURITY-SCANNER-RESIDUAL-FINDINGS.md](SECURITY-SCANNER-RESIDUAL-FINDINGS.md) — read for the residual scanner findings: unresolved workspace references, invalid member-glob matches, calendar-independent freshness tests, and real-scanner evidence.
- [SECURITY-SCANNER-FINAL-TRIAGE.md](SECURITY-SCANNER-FINAL-TRIAGE.md) — read for the final scanner triage: historical `.sop-evidence` HIGH classification, test-only MEDIUM advisories, and the latest-main coverage result.
- [SECURITY-PRODUCTION-BASELINE-2026-10-19.md](SECURITY-PRODUCTION-BASELINE-2026-10-19.md) — read for the point-in-time production dependency baseline at `1dbaa3a3`: merge verification, pinned scanner and advisory-data identities, production scope/exclusions, zero findings, coverage, and timings.
- [PHASE-9-MULTI-AGENT-ADOPTION.md](PHASE-9-MULTI-AGENT-ADOPTION.md) — read for the recorded Phase 9 — Multi-Agent Adoption Decision (ORCH-012) evidence.
- [PLAN-compiler-reconciliation-commit-review.md](PLAN-compiler-reconciliation-commit-review.md) — read for the recorded Compiler/reconciliation hardening: pre-commit review evidence.
- [PLAN-compiler-reconciliation-hardening.md](PLAN-compiler-reconciliation-hardening.md) — read for the recorded SOP Plan Compiler — Determinism and Reconciliation Hardening evidence.
- [RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md](RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md) — read for the recorded RAG-SOP-004 Phase 2 — Real PostgreSQL/pgvector Retrieval Verification evidence.
- [RAG-SOP-004-POC-EVALUATION.md](RAG-SOP-004-POC-EVALUATION.md) — read for the recorded RAG-SOP-004 POC Evaluation — PostgreSQL keyword vs hybrid (real embeddings) evidence.
- [SOP-RUN-ACCEPT-001-ACC-008-manual-intervention.md](SOP-RUN-ACCEPT-001-ACC-008-manual-intervention.md) — read for the recorded SOP-RUN-ACCEPT-001 — ACC-008 controlled manual intervention evidence.
- [SOP-EXECUTION-RELIABILITY-REVIEW.md](SOP-EXECUTION-RELIABILITY-REVIEW.md) — read for the SOP execution-reliability review evidence: direct reliability fixes, the bounded SOP-006 Rust demonstration, and the explicit non-claims about replaying earlier runs.
- [decision-model-evaluation-2026-10-13.md](decision-model-evaluation-2026-10-13.md) — read for the recorded Decision-Model Evaluation — 2026-10-13 evidence.

## Autonomy

- [autonomy/AUTONOMY-001-baseline.md](autonomy/AUTONOMY-001-baseline.md) — read for the recorded AUTONOMY-001 — End-to-end baseline and multi-package unused-code cleanup scenario evidence.
- [autonomy/AUTONOMY-001-erratum.md](autonomy/AUTONOMY-001-erratum.md) — read for the recorded AUTONOMY-001 — erratum for the governed baseline evidence.
- [autonomy/AUTONOMY-001-outcome.md](autonomy/AUTONOMY-001-outcome.md) — read for the recorded AUTONOMY-001 — Autonomous Decision Contract: outcome report evidence.
- [autonomy/AUTONOMY-001-production-integration.md](autonomy/AUTONOMY-001-production-integration.md) — read for the recorded AUTONOMY-001 — production integration: outcome verification and preventive safety evidence.
- [autonomy/AUTONOMY-005-readiness.md](autonomy/AUTONOMY-005-readiness.md) — read for the recorded AUTONOMY-005 — Readiness and disposition evidence.
- [autonomy/AUTONOMY-independent-verification.md](autonomy/AUTONOMY-independent-verification.md) — read for the recorded AUTONOMY — independent verification and plan reconciliation evidence.
- [autonomy/AUTONOMY-outcome-report.md](autonomy/AUTONOMY-outcome-report.md) — read for the recorded AUTONOMY — consolidated outcome report evidence.
- [autonomy/autonomy-001-scenario/README.md](autonomy/autonomy-001-scenario/README.md) — read for the recorded AUTONOMY-001 scenario evidence — one-prompt multi-package unused-code cleanup evidence.

## Clef readiness

- [clef-readiness/AS-CLEF-002-decision-path-trace.md](clef-readiness/AS-CLEF-002-decision-path-trace.md) — read for the recorded AS-CLEF-002 — Trace the Live Decision Path evidence.
- [clef-readiness/AS-CLEF-003-provider-neutrality-audit.md](clef-readiness/AS-CLEF-003-provider-neutrality-audit.md) — read for the recorded AS-CLEF-003 — Provider Neutrality Audit evidence.
- [clef-readiness/AS-CLEF-006-execution-decision-separation.md](clef-readiness/AS-CLEF-006-execution-decision-separation.md) — read for the recorded AS-CLEF-006 — Verify Execution/Decision Separation evidence.
- [clef-readiness/AS-CLEF-008-adapter-contract.md](clef-readiness/AS-CLEF-008-adapter-contract.md) — read for the recorded AS-CLEF-008 — Define the Adapter-Side Contract evidence.
- [clef-readiness/AS-CLEF-009-full-verification.md](clef-readiness/AS-CLEF-009-full-verification.md) — read for the recorded AS-CLEF-009 — Run Full Verification evidence.
- [clef-readiness/AS-CLEF-009-verification.md](clef-readiness/AS-CLEF-009-verification.md) — read for the recorded AS-CLEF-009 — Run Full Verification evidence.
- [clef-readiness/AS-CLEF-010-readiness-decision.md](clef-readiness/AS-CLEF-010-readiness-decision.md) — read for the recorded AS-CLEF-010 — Agentic-SOP Readiness Decision evidence.

## Concise responses v2

- [concise-responses-v2/RESPONSE-MODES.md](concise-responses-v2/RESPONSE-MODES.md) — read for the recorded Response modes: concise default, detailed override evidence.

## Consolidation

- [consolidation/backup-manifest.md](consolidation/backup-manifest.md) — read for the recorded SOP repository-consolidation — durable backup manifest evidence.
- [consolidation/baseline.md](consolidation/baseline.md) — read for the recorded MIG-001 — Reconciled Local Source and Native Baselines evidence.
- [consolidation/ci-coverage.md](consolidation/ci-coverage.md) — read for the recorded CI coverage and suite relocation (MIG-009) evidence.
- [consolidation/closure-evidence.md](consolidation/closure-evidence.md) — read for the recorded Closure evidence report — repository consolidation evidence.
- [consolidation/closure-preconditions.md](consolidation/closure-preconditions.md) — read for the recorded C1 — Closure preconditions verification and reconciliation edit enumeration evidence.
- [consolidation/completion.md](consolidation/completion.md) — read for the recorded Repository consolidation — completion report evidence.
- [consolidation/controller-provenance.md](consolidation/controller-provenance.md) — read for the recorded Controller provenance record evidence.
- [consolidation/decision-compatibility.md](consolidation/decision-compatibility.md) — read for the recorded Decision compatibility — local advisory integration and fallback (MIG-006) evidence.
- [consolidation/documentation.md](consolidation/documentation.md) — read for the recorded Documentation map (MIG-010) evidence.
- [consolidation/dogfood.md](consolidation/dogfood.md) — read for the recorded End-to-end dogfood regression (MIG-011) evidence.
- [consolidation/import-rewrites.md](consolidation/import-rewrites.md) — read for the recorded Import rewrites — migration-owned controller files (MIG-003B) evidence.
- [consolidation/import-verification.md](consolidation/import-verification.md) — read for the recorded Import Verification — MIG-003D evidence.
- [consolidation/performance.md](consolidation/performance.md) — read for the recorded Performance measurements (MIG-011) evidence.
- [consolidation/project-review-automation.md](consolidation/project-review-automation.md) — read for the recorded SOP Project Review automation — external update record evidence.
- [consolidation/source-map.md](consolidation/source-map.md) — read for the recorded MIG-002 — Preserved Histories and Package Mapping evidence.

## Decision integration

- [decision-integration/SEAM-001-current-boundary.md](decision-integration/SEAM-001-current-boundary.md) — read for the recorded SEAM-001 — Capture and Trace the Current Integration Boundary evidence.
- [decision-integration/SEAM-002-decision-contract.md](decision-integration/SEAM-002-decision-contract.md) — read for the recorded SEAM-002 — Provider-Neutral Decision Contract evidence.
- [decision-integration/SEAM-007-verification.md](decision-integration/SEAM-007-verification.md) — read for the recorded SEAM-007 — Full Verification and Architecture Review evidence.
- [decision-integration/SEAM-008-readiness-decision.md](decision-integration/SEAM-008-readiness-decision.md) — read for the recorded SEAM-008 — Integration-Seam Readiness Decision evidence.
- [decision-integration/SEAM-architecture-analysis.md](decision-integration/SEAM-architecture-analysis.md) — read for the recorded SEAM — Provider-Neutral Decision Integration Seam evidence.

## Decision telemetry

- [decision-telemetry/TEL-001-reconciliation.md](decision-telemetry/TEL-001-reconciliation.md) — read for the recorded TEL-001 — Telemetry Reconciliation and Common Metric Definitions evidence.
- [decision-telemetry/TEL-003-integration.md](decision-telemetry/TEL-003-integration.md) — read for the recorded TEL-003 — Decision Adapter Telemetry Integration evidence.
- [decision-telemetry/TEL-006-cli-report.md](decision-telemetry/TEL-006-cli-report.md) — read for the recorded TEL-006 — Decision Telemetry CLI (`sop report --decisions`) evidence.
- [decision-telemetry/TEL-008-model-selection.md](decision-telemetry/TEL-008-model-selection.md) — read for the recorded TEL-008 — Model Routing Percentage Summary evidence.

## End to end reliability

- [end-to-end-reliability/ETOE-001-preflight-baseline.md](end-to-end-reliability/ETOE-001-preflight-baseline.md) — read for the recorded ETOE-001 — Cross-repository preflight baseline evidence.
- [end-to-end-reliability/ETOE-002-attempt-1-failure/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-002-attempt-1-failure/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-002-attempt-1-failure/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-002-attempt-1-failure/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-002-attempt-1-failure/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-002-attempt-1-failure/report.md) — read for the recorded Run: ETOE-002 — Core workflow contract audit evidence.
- [end-to-end-reliability/ETOE-002-attempt-1-failure/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-002-attempt-1-failure/task.md) — read for the recorded ETOE-002 — Core workflow contract audit evidence.
- [end-to-end-reliability/ETOE-002-workflow-contract-matrix.md](end-to-end-reliability/ETOE-002-workflow-contract-matrix.md) — read for the recorded ETOE-002 — Core workflow contract matrix (requirement-to-test audit) evidence.
- [end-to-end-reliability/ETOE-003-controller-contract-matrix.md](end-to-end-reliability/ETOE-003-controller-contract-matrix.md) — read for the recorded ETOE-003 — Controller integration audit (controller contract matrix) evidence.
- [end-to-end-reliability/ETOE-004-adapter-integration-matrix.md](end-to-end-reliability/ETOE-004-adapter-integration-matrix.md) — read for the recorded ETOE-004 — Decision adapter integration matrix evidence.
- [end-to-end-reliability/ETOE-004-attempt-1-failure/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-004-attempt-1-failure/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-004-attempt-1-failure/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-004-attempt-1-failure/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-004-attempt-1-failure/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-004-attempt-1-failure/report.md) — read for the recorded Run: ETOE-004 — Decision adapter integration audit evidence.
- [end-to-end-reliability/ETOE-004-attempt-1-failure/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-004-attempt-1-failure/task.md) — read for the recorded ETOE-004 — Decision adapter integration audit evidence.
- [end-to-end-reliability/ETOE-005-accidental-run/fix-1.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-accidental-run/fix-1.md) — read for the recorded fix 1 evidence.
- [end-to-end-reliability/ETOE-005-accidental-run/fix-2.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-accidental-run/fix-2.md) — read for the recorded fix 2 evidence.
- [end-to-end-reliability/ETOE-005-accidental-run/fix-3.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-accidental-run/fix-3.md) — read for the recorded fix 3 evidence.
- [end-to-end-reliability/ETOE-005-accidental-run/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-accidental-run/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-005-accidental-run/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-accidental-run/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-005-accidental-run/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-accidental-run/report.md) — read for the recorded Run: ETOE-005 — Disposable dogfood fixture and deterministic baseline evidence.
- [end-to-end-reliability/ETOE-005-accidental-run/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-accidental-run/task.md) — read for the recorded ETOE-005 — Disposable dogfood fixture and deterministic baseline evidence.
- [end-to-end-reliability/ETOE-005-attempt-1-failure/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-attempt-1-failure/task.md) — read for the recorded ETOE-005 — Disposable dogfood fixture and deterministic baseline evidence.
- [end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/fix-1.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/fix-1.md) — read for the recorded fix 1 evidence.
- [end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/fix-2.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/fix-2.md) — read for the recorded fix 2 evidence.
- [end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/fix-3.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/fix-3.md) — read for the recorded fix 3 evidence.
- [end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/report.md) — read for the recorded Run: ETOE-005 — Disposable dogfood fixture and deterministic baseline evidence.
- [end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/task.md) — read for the recorded ETOE-005 — Disposable dogfood fixture and deterministic baseline evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/fix-1.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/fix-1.md) — read for the recorded fix 1 evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/fix-2.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/fix-2.md) — read for the recorded fix 2 evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/fix-3.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/fix-3.md) — read for the recorded fix 3 evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/report.md) — read for the recorded Run: FIX-FAIL evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/fail/runs/run-20261009-053030/task.md) — read for the recorded FIX-FAIL evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/gate/PLAN.md](end-to-end-reliability/ETOE-005-execution-evidence/gate/PLAN.md) — read for the recorded PLAN — ETOE-005 gate fixture evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/gate/runs/FIX-003/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/gate/runs/FIX-003/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/gate/runs/FIX-003/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/gate/runs/FIX-003/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/gate/runs/FIX-003/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/gate/runs/FIX-003/report.md) — read for the recorded Run: FIX-003 — Human-gated fixture task evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/gate/runs/FIX-003/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/gate/runs/FIX-003/task.md) — read for the recorded FIX-003 — Human-gated fixture task evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/success/runs/run-20261009-053027/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/success/runs/run-20261009-053027/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/success/runs/run-20261009-053027/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/success/runs/run-20261009-053027/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/success/runs/run-20261009-053027/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/success/runs/run-20261009-053027/report.md) — read for the recorded Run: FIX-SUCCESS evidence.
- [end-to-end-reliability/ETOE-005-execution-evidence/success/runs/run-20261009-053027/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-005-execution-evidence/success/runs/run-20261009-053027/task.md) — read for the recorded FIX-SUCCESS evidence.
- [end-to-end-reliability/ETOE-005-fixture-baseline.md](end-to-end-reliability/ETOE-005-fixture-baseline.md) — read for the recorded ETOE-005 — Disposable dogfood fixture and deterministic baseline evidence.
- [end-to-end-reliability/ETOE-006-e2e-evidence.md](end-to-end-reliability/ETOE-006-e2e-evidence.md) — read for the recorded ETOE-006 — Core end-to-end success and failure/recovery paths evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/fix-1.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/fix-1.md) — read for the recorded fix 1 evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/fix-2.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/fix-2.md) — read for the recorded fix 2 evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/fix-3.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/fix-3.md) — read for the recorded fix 3 evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/report.md) — read for the recorded Run: FIX-FAIL evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/fail/runs/run-20261009-054551/task.md) — read for the recorded FIX-FAIL evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/gate/runs/FIX-003/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/gate/runs/FIX-003/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/gate/runs/FIX-003/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/gate/runs/FIX-003/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/gate/runs/FIX-003/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/gate/runs/FIX-003/report.md) — read for the recorded Run: FIX-003 — Human-gated fixture task evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/gate/runs/FIX-003/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/gate/runs/FIX-003/task.md) — read for the recorded FIX-003 — Human-gated fixture task evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/success/runs/run-20261009-054551/implementation.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/success/runs/run-20261009-054551/implementation.md) — read for the recorded implementation evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/success/runs/run-20261009-054551/plan.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/success/runs/run-20261009-054551/plan.md) — read for the recorded Implementation Plan evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/success/runs/run-20261009-054551/report.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/success/runs/run-20261009-054551/report.md) — read for the recorded Run: FIX-SUCCESS evidence.
- [end-to-end-reliability/ETOE-006-execution-evidence/success/runs/run-20261009-054551/task.md](../../archive/run-artifacts/end-to-end-reliability/ETOE-006-execution-evidence/success/runs/run-20261009-054551/task.md) — read for the recorded FIX-SUCCESS evidence.
- [end-to-end-reliability/ETOE-007-controller-parity.md](end-to-end-reliability/ETOE-007-controller-parity.md) — read for the recorded ETOE-007 — Controller parity against the disposable project evidence.
- [end-to-end-reliability/ETOE-008-adapter-evidence.md](end-to-end-reliability/ETOE-008-adapter-evidence.md) — read for the recorded ETOE-008 — Decision adapter integration or documented absence evidence.
- [end-to-end-reliability/ETOE-009-metrics-review.md](end-to-end-reliability/ETOE-009-metrics-review.md) — read for the recorded ETOE-009 — Metrics and Regression Review evidence.
- [end-to-end-reliability/ETOE-010-final-reliability-report.md](end-to-end-reliability/ETOE-010-final-reliability-report.md) — read for the recorded ETOE-010 — Final end-to-end reliability report and prioritized backlog evidence.

## Ev context run

- [ev-context-run/EV-001-live-path-baseline.md](ev-context-run/EV-001-live-path-baseline.md) — read for the recorded EV-001 — Live-Path Baseline and State Verification evidence.
- [ev-context-run/EV-002-corpus-and-baseline-protocol.md](ev-context-run/EV-002-corpus-and-baseline-protocol.md) — read for the recorded EV-002 — Deterministic Evaluation Corpus and Baseline Protocol evidence.
- [ev-context-run/EV-003-retrieval-evaluation.md](ev-context-run/EV-003-retrieval-evaluation.md) — read for the recorded EV-003 — Retrieval-Enabled Evaluation Using Existing Capabilities evidence.
- [ev-context-run/EV-004-instrumentation-gaps.md](ev-context-run/EV-004-instrumentation-gaps.md) — read for the recorded EV-004 — Instrumentation Gap Assessment evidence.
- [ev-context-run/EV-005-acceptance-criteria.md](ev-context-run/EV-005-acceptance-criteria.md) — read for the recorded EV-005 — Acceptance Criteria, Decision Gate, and Approval Prerequisites evidence.

## Evidence

- [evidence/artifacts/direct_sop_jev_invocation/implementation.md](evidence/artifacts/direct_sop_jev_invocation/implementation.md) — read for the recorded implementation evidence.
- [evidence/artifacts/direct_sop_jev_invocation/plan.md](evidence/artifacts/direct_sop_jev_invocation/plan.md) — read for the recorded Implementation Plan evidence.
- [evidence/artifacts/direct_sop_jev_invocation/report.md](evidence/artifacts/direct_sop_jev_invocation/report.md) — read for the recorded Run: DOGFOOD-001 — Synthetic addition evidence.
- [evidence/artifacts/direct_sop_jev_invocation/task.md](evidence/artifacts/direct_sop_jev_invocation/task.md) — read for the recorded DOGFOOD-001 — Synthetic addition evidence.
- [evidence/artifacts/early_jev_routing/implementation.md](evidence/artifacts/early_jev_routing/implementation.md) — read for the recorded implementation evidence.
- [evidence/artifacts/early_jev_routing/plan.md](evidence/artifacts/early_jev_routing/plan.md) — read for the recorded Implementation Plan evidence.
- [evidence/artifacts/early_jev_routing/report.md](evidence/artifacts/early_jev_routing/report.md) — read for the recorded Run: DOGFOOD-001 — Synthetic addition evidence.
- [evidence/artifacts/early_jev_routing/task.md](evidence/artifacts/early_jev_routing/task.md) — read for the recorded DOGFOOD-001 — Synthetic addition evidence.
- [evidence/artifacts/provider_selection/implementation.md](evidence/artifacts/provider_selection/implementation.md) — read for the recorded implementation evidence.
- [evidence/artifacts/provider_selection/plan.md](evidence/artifacts/provider_selection/plan.md) — read for the recorded Implementation Plan evidence.
- [evidence/artifacts/provider_selection/report.md](evidence/artifacts/provider_selection/report.md) — read for the recorded Run: DOGFOOD-001 — Synthetic addition evidence.
- [evidence/artifacts/provider_selection/task.md](evidence/artifacts/provider_selection/task.md) — read for the recorded DOGFOOD-001 — Synthetic addition evidence.
- [evidence/artifacts/structured_evidence/implementation.md](evidence/artifacts/structured_evidence/implementation.md) — read for the recorded implementation evidence.
- [evidence/artifacts/structured_evidence/plan.md](evidence/artifacts/structured_evidence/plan.md) — read for the recorded Implementation Plan evidence.
- [evidence/artifacts/structured_evidence/report.md](evidence/artifacts/structured_evidence/report.md) — read for the recorded Run: DOGFOOD-001 — Synthetic addition evidence.
- [evidence/artifacts/structured_evidence/task.md](evidence/artifacts/structured_evidence/task.md) — read for the recorded DOGFOOD-001 — Synthetic addition evidence.

## Implementation convergence

- [implementation-convergence/CONV-001-convergence-baseline.md](implementation-convergence/CONV-001-convergence-baseline.md) — read for the recorded CONV-001 — Convergence Baseline (IMPLEMENT/FIX phased engine) evidence.
- [implementation-convergence/CONV-005-regression-safety.md](implementation-convergence/CONV-005-regression-safety.md) — read for the recorded CONV-005 — Regression and Safety Verification evidence.
- [implementation-convergence/CONV-006-clef-dogfood-comparison.md](implementation-convergence/CONV-006-clef-dogfood-comparison.md) — read for the recorded CONV-006 — Clef Dogfood Comparison (harness before vs after CONV-004) evidence.

## Lifecycle consistency

- [lifecycle-consistency/LC-001-approval-applicability-contract.md](lifecycle-consistency/LC-001-approval-applicability-contract.md) — read for the recorded LC-001 — Approval Applicability Contract evidence.
- [lifecycle-consistency/LC-006-verification.md](lifecycle-consistency/LC-006-verification.md) — read for the recorded LC-006 — Verification and Behavior-Preservation Audit evidence.

## Planner

- [planner/SOP-PLANNER-CAP-001-capability-context.md](planner/SOP-PLANNER-CAP-001-capability-context.md) — read for the recorded SOP-PLANNER-CAP-001 — Per-task planner capability context evidence.
- [planner/issue-90-tunnel-client-patch/README.md](planner/issue-90-tunnel-client-patch/README.md) — the proposed tunnel-client patch for issue #90, the observed baseline/candidate live results, and the agent-led candidate workflow; the failure remains intermittent and #90 stays open.
- [mcp-live-validation-portability.md](mcp-live-validation-portability.md) — the portable validation-handoff evidence: a real candidate artifact, an actual Claude artifact review, the simulated Claude/ChatGPT/second-agent scenarios, and the missing live MCP validator capability.

## Pre performance closure

- [pre-performance-closure/CLOSE-001-baseline.md](pre-performance-closure/CLOSE-001-baseline.md) — read for the recorded CLOSE-001 — Current Repository Baseline evidence.
- [pre-performance-closure/CLOSE-003-sop-deterministic-baseline.md](pre-performance-closure/CLOSE-003-sop-deterministic-baseline.md) — read for the recorded CLOSE-003 — SOP Deterministic Baseline evidence.
- [pre-performance-closure/CLOSE-004-controller-deterministic-baseline.md](pre-performance-closure/CLOSE-004-controller-deterministic-baseline.md) — read for the recorded CLOSE-004 — Controller Deterministic Baseline evidence.
- [pre-performance-closure/CLOSE-008-telemetry-inventory.md](pre-performance-closure/CLOSE-008-telemetry-inventory.md) — read for the recorded CLOSE-008 — Inventory Performance Telemetry evidence.
- [pre-performance-closure/CLOSE-009-performance-baseline.md](pre-performance-closure/CLOSE-009-performance-baseline.md) — read for the recorded CLOSE-009 — Performance Baseline evidence.
- [pre-performance-closure/CLOSE-011-readiness.md](pre-performance-closure/CLOSE-011-readiness.md) — read for the recorded CLOSE-011 — Readiness Verdict evidence.
- [pre-performance-closure/README.md](pre-performance-closure/README.md) — read for the recorded Pre-Performance Closure — Reports Index evidence.
- [pre-performance-closure/workloads/A/HASHES.md](pre-performance-closure/workloads/A/HASHES.md) — read for the recorded Workload A — Frozen Input Hashes evidence.
- [pre-performance-closure/workloads/A/PLAN-Workload-A.md](pre-performance-closure/workloads/A/PLAN-Workload-A.md) — read for the recorded PLAN-Workload-A.md — Workload A named plan (frozen) evidence.
- [pre-performance-closure/workloads/B/PLAN-Workload-B.md](pre-performance-closure/workloads/B/PLAN-Workload-B.md) — read for the recorded PLAN-Workload-B.md — Workload B named plan (frozen) evidence.
- [pre-performance-closure/workloads/build-pin.md](pre-performance-closure/workloads/build-pin.md) — read for the recorded CLOSE-009 — Pinned SOP Measurement Build evidence.

## Review hardening

- [review-hardening/H-FollowUp-hardening-verification.md](review-hardening/H-FollowUp-hardening-verification.md) — read for the recorded Follow-Up Hardening — Verification Report (H001..H004) evidence.
- [review-hardening/RSH-001-review-seam-baseline.md](review-hardening/RSH-001-review-seam-baseline.md) — read for the recorded RSH-001 — Baseline: Review Seam and Change Attribution evidence.
- [review-hardening/RSH-005-verification.md](review-hardening/RSH-005-verification.md) — read for the recorded RSH-005 — Verification, Neutrality, and Documentation evidence.

## Run metrics

- [run-metrics/RM-001-artifact-inventory.md](run-metrics/RM-001-artifact-inventory.md) — read for the recorded RM-001 — Run-Artifact Inventory and Schema Census evidence.
- [run-metrics/RM-002-metric-register.md](run-metrics/RM-002-metric-register.md) — read for the recorded RM-002 — Metric Register and Classification evidence.
- [run-metrics/RM-003-aggregation-results.md](run-metrics/RM-003-aggregation-results.md) — read for the recorded RM-003 — Offline Aggregation Results evidence.
- [run-metrics/RM-004-verification.md](run-metrics/RM-004-verification.md) — read for the recorded RM-004 — Reproducibility, Coverage, and Neutrality Verification evidence.
- [run-metrics/RM-005-decision.md](run-metrics/RM-005-decision.md) — read for the recorded RM-005 — Decision Record: Offline Aggregate vs Opt-in Command evidence.
