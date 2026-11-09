# Plan 05 — SOP Data and model poisoning (LLM05)

Status: PROPOSED; adapted to the SOP plan schema for execution; not yet executed.
Date: 2026-11-08 (America/Chicago).
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm05-data-and-model-poisoning.
Suggested repository path: docs/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md.

## Goal and scope

Establish provenance and integrity for data or artifacts SOP actually owns. Prevent untrusted or modified material from acquiring trusted evidence status. Distinguish model training poisoning from retrieval poisoning: most SOP-owned work may be document/memory ingestion rather than training.

Dependency: reuse Plan 04's component/owner inventory. Plan 09 owns retrieval query isolation; this plan owns source acceptance and provenance. Discovery can proceed independently; do not duplicate supply-chain authenticity checks.

## Project

agentic-sop

## Summary

Trace the data SOP ingests — planning documents, completed-run artifacts, historical
outcomes, persistent decision memory, the structural repository index, and external
RAG retrieval — to the consumers that treat it as evidence. Reproduce and repair
demonstrated cases where imported text can claim a verified result, where a modified
source keeps an obsolete identity, or where malformed input is accepted contrary to
the existing contract. The work stays inside the existing outcome-file, memory-index,
and plan-provenance abstractions; it adds no classifier, trust service, or poisoning
detection claim.

## Dependency reconciliation

- The task IDs and every acceptance criterion from the planning document are
  preserved verbatim. Nothing is removed, weakened, or re-mapped to a weaker
  evidence kind.
- POISON-002 is executed first. Its acceptance criteria are the only ones that are
  wholly deterministic, so the source-integrity repair and its regression fixtures
  land before any documentation-only stage can stop at the human acceptance
  boundary. POISON-001 then records the confirmed gaps against the repaired
  contracts, and POISON-003 exercises invalidation and records the handoff.
- Plan 04's inventory is not merged to this baseline. Its provider/owner
  classification is consumed as context from `origin/security/llm04-supply-chain`
  and is not re-derived here.

## POISON-002 — Preserve provenance at existing ingestion boundaries

Reuse the current outcome ingestion, content identity, eligibility gate, and
evidence authority. Repair demonstrated cases where imported report text can forge
a trusted result, where a modified run source keeps an obsolete verified identity,
or where malformed source metadata is accepted contrary to the existing record
contract. Do not build a content classifier that claims to detect poisoned text and
do not add a new trust service, permission layer, or duplicate gate.

The demonstrated ingestion gaps at the current baseline are:

- `outcomefile.IngestRunFor` maps a self-declared `outcome.status` to a success
  record without checking the report's own required criteria or validation results,
  so a forged success claim is ingested as verified.
- The eligibility gate accepts a success whose recorded validation contradicts it.
- `counts.json` is parsed leniently and a malformed file is silently ignored, so
  the record identity and attempt are derived from unverified metadata.
- The report's declared `id` is not checked against the run directory it was read
  from, so substituted content can be ingested under another task's identity.
- `GateWithSources` checks only that source artifacts still exist. A changed
  `report.json` therefore keeps the record's obsolete recorded digest and its
  verified identity instead of invalidating it.

The repair must keep partial and failed history usable as advisory data, keep a
rejected or malformed input from changing the previously written record, and treat
an operator-imported run tree as data whose self-declared success must still be
corroborated.

### Dependencies

None

### Deliverables

- `internal/outcomefile/outcomefile.go` — corroborate a claimed success from the report's required criteria and validation, reject a report whose declared id does not match its run directory, reject malformed counts metadata, reject a success whose recorded verification contradicts it, and complete the source-identity check so a changed source is invalidated rather than retained.
- `internal/outcomefile/outcomefile_test.go` — fixtures named with the `TestIngestProvenance` prefix for a tampered success claim, substituted source content, malformed counts metadata, a mismatched report id, a modified source, and a valid success control, all through the real ingest path.
- `internal/memindex/rebuild.go` — apply the source-aware eligibility gate when rebuilding the advisory index, so a missing or modified source is excluded and counted rather than indexed.
- `internal/memindex/memindex_test.go` — a before/after reindex test named with the `TestReindexProvenance` prefix that proves an ingested record is indexed, a modified source is excluded on rebuild, and a deleted source record is never returned.

### Acceptance Criteria

- A forged success/approval record cannot become authoritative merely by ingestion.
- A changed source does not retain an obsolete verified identity under the current contract.
- Malformed or rejected input leaves the previous valid state intact.
- Valid historical content remains usable as advisory data.
- Fixtures cover tampered metadata, substituted content, and a valid control using the real ingest path.

### Acceptance Checks

- `go test ./internal/outcomefile -run TestIngestProvenance -count=1`
- `go test ./internal/memindex -run TestReindexProvenance -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1

## POISON-001 — Identify owned poisoning boundaries

Trace supported training/fine-tuning or model-loading paths, if any, plus
ingestion of repository documents, external content, historical outcomes, and
persistent memory. Identify the writer, source identity, revision/content digest,
trust classification, and consumer. Determine whether imported records can claim
verified outcomes or override authoritative state.

Record the trace in the plan's report deliverable. The absence of a
training/fine-tuning/weight-loading path and the provider-owned boundary are
recorded as explicit designations, not as a security claim. Confirmed gaps name the
affected contract (package and function), the disposition (fixed or open), and the
reproducing test or command where one exists.

### Dependencies

- POISON-002

### Deliverables

- `docs/reports/owasp-llm/llm05-poisoning.md` — the source-to-consumer trace, ownership designations, confirmed gaps with affected contracts, and residual trust limits.

### Acceptance Criteria

- Every relevant path has a concrete source-to-consumer trace and ownership designation.
- Provider-owned training is documented as outside direct SOP control, not marked secure.
- Absence of a training/loading path is supported by code/configuration evidence.
- The report records confirmed gaps and affected contracts.

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm05-poisoning.md §Source-to-consumer trace and §Ownership designations
- criterion 2: manual: docs/reports/owasp-llm/llm05-poisoning.md §Provider-owned boundary and §Residual trust limits
- criterion 3: manual: docs/reports/owasp-llm/llm05-poisoning.md §Provider-owned boundary cites the absence search and the configuration surface it inspected
- criterion 4: manual: docs/reports/owasp-llm/llm05-poisoning.md §Confirmed gaps and affected contracts and §Disposition

## POISON-003 — Verify recovery and document remaining trust

Exercise supported invalidation and reindex behavior on a disposable copy.
Confirm removal or correction prevents future use under the existing contract
without deleting authoritative history. Do not add destructive cleanup commands.

The report's provenance section must keep provider model names, content checksums,
publisher authenticity, and semantic trust as separate dimensions, and must hand
retrieval query isolation to Plan 09.

### Dependencies

- POISON-001
- POISON-002

### Deliverables

- `docs/reports/owasp-llm/llm05-poisoning.md` — the recovery evidence section, the provenance-dimension table, and the Plan 09 handoff.

### Acceptance Criteria

- Supported remediation has reproducible before/after evidence; unsupported removal is an explicit gap.
- Provider model names, checksums, publisher authenticity, and semantic trust remain distinct.
- Focused and required checks pass; PR records provenance limits and a handoff to Plan 09.

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm05-poisoning.md §Recovery and invalidation evidence records the reproducible before/after run; the report names the unsupported removal command as an explicit gap
- criterion 2: manual: docs/reports/owasp-llm/llm05-poisoning.md §Provenance dimensions
- criterion 3: manual: the PR body records the provenance limits and the Plan 09 handoff; the focused and required checks are recorded in the report §Verification

## Risks and completion

Integrity proves bytes match a reference; it does not prove benign semantics. Provider training and backdoors remain outside direct assurance. Complete only the owned, evidenced scope; no training service, fine-tuning campaign, new model download, or universal poisoning-detection claim.

## Execution contract

Planning and schema adaptation only; no implementation, SOP ingestion, configuration change, or installation has occurred by writing this document.

When authorized, refresh origin/main, read AGENTS.md and current plan-authoring conventions, and create a new isolated worktree and branch. Preserve existing worktrees, active plans, uncommitted files, and lifecycle history. Reconcile through supported SOP interfaces; never edit state.db or fabricate approvals.

Execute the tasks below sequentially through existing lifecycle and PR gates. Continue autonomously within approved scope until completion or a genuine blocker. Reuse existing abstractions and record ALREADY_SATISFIED only with current evidence. Fix reproducible defects in existing contracts; do not introduce another gate, evaluator, permission system, or service. A missing control requiring new policy gets a concrete proposal through existing governance; stop only dependent work.

Preserve report-only/required settings, configured limits, routing, and autonomy defaults. Do not enable remote evaluations, install tools, download models, expand permissions, or change security policy implicitly. Use synthetic data, disposable fixtures, fake providers, and inert effect recorders. No production writes or real exfiltration.

Run focused regression tests for changed behavior, then repository-required checks once. Repeat only after relevant changes or failures. Record exact commands, exit codes, base/head commits, configuration identity, platform, and evidence paths. Open a focused PR; follow established review and merge governance. Do not claim installation or deployment from a PR result.

## Baseline and evidence

The four preceding planning files used historical baseline 0750e8be0cf5f318a173abb34c2b752ac131521c. Their observations are context, not current runtime evidence. These six plans do not claim a fresh code audit or that preceding work has merged. Refresh the baseline and inspect current implementations before choosing files or commands.

Report IMPLEMENTED, ENABLED, and VERIFIED separately, with the exact path and platform. Use GAP, NOT_RUN, and justified NOT_APPLICABLE explicitly. A deterministic test is not live-model evidence; a scan PASS is not complete category coverage. Missing required evidence leaves the associated criterion incomplete.

Category numbers and names follow the supplied Cycode article's displayed 2026 list, accessed November 8, 2026 (America/Chicago), consistent with Plans 01–04. This is an article-based planning taxonomy, not a verified formal OWASP compliance mapping.

## Sources and handoff

- https://cycode.com/blog/owasp-top-10-llm-applications/
- Repository: https://github.com/imhttran/agentic-sop
- Earlier plans: PLAN-01-SOP-LLM03-Excessive-Agency.md; PLAN-02-SOP-LLM01-Prompt-Injection.md; PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md; PLAN-04-SOP-LLM04-Supply-Chain.md.
- At execution, inspect current docs/specs/SECURITY.md, docs/specs/QUALITY.md, configuration, and relevant reports before relying on historical assertions.

Final handoff: changed behavior, criterion-to-evidence mapping, PR link, residual gaps, blocked criteria, and exact next action. Proposed report paths are deliverables, not claims that files already exist.
