# SOP RAG Decision Memory Plan

Date: 2026-10-19
Revision: 5 — canonical rendered layout; acceptance checks added for implemented tasks.
Status: Final planning document, ready for implementation. No repository changes or rollout performed.
Target: `imhttran/agentic-sop`; reuse the existing RAG integration where suitable.

## Project

agentic-sop

## Summary

Use verified run history to improve planning and failure recovery. Then evaluate whether that history supports useful development suggestions. Keep retrieval advisory: current policy, approvals, and verification remain authoritative.

Success means better task outcomes than no historical retrieval at acceptable latency and cost. Semantic retrieval must also improve outcomes over local keyword retrieval. More retrieved text or higher retrieval scores alone do not establish success.

Implementation order: prove that historical evidence improves decisions before expanding installation choices. Define the retrieval interface early, implement SQLite first, and preserve existing PostgreSQL behavior throughout.

Storage and retrieval design. PostgreSQL is not a prerequisite for local decision memory. Preserve the existing PostgreSQL integration for users who select it. Do not remove it or migrate existing data as part of this plan.

| Component                   | Initial implementation                                                  | Contract                                                                                         |
| --------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Authoritative evidence      | Versioned local JSON outcome files plus referenced run artifacts        | Preserve identity, provenance, access scope, and original verification evidence                  |
| Search index                | Separate, rebuildable SQLite database with FTS5 and structured metadata | Index failure signatures, task context, rationale, outcomes, and correction summaries            |
| Retrieval boundary          | Small interface adapted to existing repository conventions              | Accept scoped query and budgets; return bounded evidence records, citations, and explicit errors |
| Optional semantic retrieval | Deferred local vector implementation                                    | Add only after the keyword evaluation identifies a semantic retrieval gap                        |
| Existing PostgreSQL backend | Preserve as an explicitly selected option                               | Share retrieval safety contracts; remain optional for local installation and operation           |

The outcome files are authoritative for recorded history, not for current policy or approval state. The SQLite index is disposable. Rebuilding it must not require PostgreSQL, an embedding service, or changes to SOP execution state.

Keep the memory index separate from the execution-state database. Reuse the existing SQLite driver only if its FTS5 support and supported-platform builds are suitable. Confirm this during preflight; do not assume extension support or add an external SQLite CLI prerequisite.

Scope and constraints.

- The local path must install, start, ingest, query, and rebuild without a PostgreSQL service, credentials, or connection attempt.
- Do not download embedding models or start background services on the keyword path.
- Start with planning and failure recovery. Defer development suggestions until outcome records and retrieval evaluation pass.
- Reuse existing configuration and telemetry contracts. Confirm `SOP_RAG_ENABLED`, `rag.enabled`, and `rag.json` before making changes.
- Do not introduce a new agent framework, graph database, model training pipeline, or autonomous roadmap executor.
- Do not let retrieved text change policy, grant approval, suppress a required check, or authorize a retry.
- A previous passing result is historical evidence. Reuse a check only when its relevant inputs and required gate rules permit reuse.
- Keep optional retrieval disabled by default during the experiment. Preserve required-evidence fail-closed behavior.
- Treat documents, summaries, and retrieved instructions as untrusted data. Apply repository and corpus access filters before retrieval.
- Exclude secrets and sensitive raw logs from ingestion. Do not send local-only evidence to a remote provider.

Execution contract. Implement the authorized scope end to end. Continue through inspection, implementation, affected verification, evaluation, and a reviewable result without routine continuation prompts. Use supported SOP plan and reconciliation commands discovered in the repository; do not invent CLI syntax or edit execution state directly. Checkpoint after each task with revision, status, evidence paths, checks, and the next action. Resume from that checkpoint. Reuse valid verification when relevant inputs are unchanged. Stop only for a real blocker: missing required access, an unresolved policy boundary, unsafe data exposure, missing required evidence, or an action that requires authorization. An evaluation no-go is a valid outcome: complete its report and close the experiment without enabling the feature.

Conditional tasks. `RDM-004B` and `RDM-002C` are conditional on the `RDM-004` decision. When that decision is no-go or inconclusive, they are not required and must be closed with `sop task skip --reason ... --evidence ...` rather than executed for the sake of completing the task list. `RDM-005` depends on `RDM-002C`; on a no-go result it is likewise not required.

Completion definition. The work is complete when the implementation and required verification are reviewable, the comparison establishes a documented go/no-go result, and remaining limitations are explicit. A no-go result is acceptable. Enabling RAG without demonstrated benefit is not.

## RDM-001 — Confirm the baseline

Inspect repository instructions, branch state, current RAG code, tests, configuration, and telemetry. Record the base commit and dependency versions. Locate prior evaluation dataset, results, and operator decision. Map each existing capability to its implementation and verification evidence. Identify the smallest implementation delta and supported artifact locations. Preflight the SQLite driver, FTS5 availability, build tags, and supported-platform packaging. Map PostgreSQL coupling in startup, configuration, health checks, retrieval, and tests. Define backend selection using existing configuration conventions, including `SOP_RAG_ENABLED`, `rag.enabled`, and `rag.json`. Do not silently change explicitly configured PostgreSQL users to SQLite.

### Dependencies

- none

### Deliverables

- Baseline inventory with implemented, missing, and unverified items
- Base commit and dependency version record
- Prior evaluation artifact status record
- Minimal implementation delta and artifact location map
- FTS5 and SQLite driver preflight result
- PostgreSQL coupling map and backend selection definition

### Acceptance Criteria

- Produce a baseline inventory with implemented, missing, and unverified items.
- Preserve unrelated work; use an isolated branch or worktree for implementation.
- Do not treat missing historical artifacts as evidence of a passing evaluation.
- Continue with a documented fresh evaluation if old artifacts are unavailable; stop only if current correctness or required access cannot be established.
- Confirm `SOP_RAG_ENABLED`, `rag.enabled`, and `rag.json` behavior before proposing changes.
- Record whether FTS5 is available on supported platforms without adding an external SQLite CLI prerequisite.

### Acceptance Mapping

- criterion 1: manual: baseline inventory recorded in the RDM-001 run report
- criterion 2: manual: isolated branch `feat/rag-decision-memory` preserves unrelated work
- criterion 3: manual: prior evaluation artifacts checked; a missing artifact is never read as a passing result
- criterion 4: manual: a fresh documented evaluation continues when historical artifacts are unavailable
- criterion 5: manual: `SOP_RAG_ENABLED`, `rag.enabled`, and `rag.json` behavior confirmed before changes
- criterion 6: manual: SQLite driver and FTS5 preflight recorded for supported platforms

## RDM-002 — Record verified outcomes

Define a versioned outcome record and generate it from existing run evidence. Include identity, context, decision, result, recovery, provenance, and lifecycle fields as specified. Treat structured evidence as the source of truth. Make any generated rationale cite its sources and stay distinguishable from observed facts. Repeated ingestion of the same source must be idempotent. Failed, partial, blocked, and successful outcomes remain distinct. Missing evidence is explicit; unsupported records cannot support claims of a successful fix. Source deletion, supersession, or access changes invalidate affected index entries.

### Dependencies

- RDM-001

### Deliverables

- Versioned outcome record schema
- Ingestion implementation that writes outcome records from existing run evidence
- Tests for deduplication, provenance, redaction, status preservation, and invalidation

### Acceptance Criteria

- Repeated ingestion of the same source is idempotent.
- Failed, partial, blocked, and successful outcomes remain distinct.
- A gate pass cannot silently convert a partial outcome into a verified success.
- Missing evidence is explicit; unsupported records cannot support claims of a successful fix.
- Source deletion, supersession, or access changes invalidate affected index entries.
- Test deduplication, provenance, redaction, status preservation, and invalidation with representative records.

### Acceptance Checks

- `go test ./internal/outcomefile -run TestIngestionIsIdempotent`
- `go test ./internal/outcomefile -run TestStatusesRemainDistinct`
- `go test ./internal/outcomefile -run TestGatePassCannotConvertPartialToSuccess`
- `go test ./internal/outcomefile -run TestMissingEvidenceIsExplicit`
- `go test ./internal/outcomefile -run TestSourceDeletionSupersessionAndAccessInvalidate`
- `go test ./internal/outcomefile -run TestRedactionAndProvenance`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3
- criterion 4: check 4
- criterion 5: check 5
- criterion 6: check 6

## RDM-002B — Build the local index and retrieval boundary

Write versioned outcome files atomically and index only complete, eligible records in a separate local SQLite database with structured metadata tables and an FTS5 index. Use stable record IDs and content hashes for idempotent updates. Apply corpus scope and eligibility constraints before ranking and limiting. Implement bounded, parameterized queries with safe FTS query handling. Provide operations to rebuild the index and inspect its status. Detect index/source drift and exclude invalid or superseded evidence. Adapt existing PostgreSQL retrieval to the shared boundary only where necessary; the local path must never initialize a PostgreSQL client or read database credentials.

### Dependencies

- RDM-002

### Deliverables

- Atomic versioned outcome-file writer and reader
- Separate local SQLite index with structured metadata and an FTS5 table
- Idempotent ingest and update using stable record IDs and content hashes
- Corpus- and eligibility-scoped, bounded, parameterized search with safe FTS syntax handling
- Index rebuild and status operations
- Drift detection that excludes deleted, superseded, modified, or unauthorized evidence
- Backend-neutral bounded retrieval boundary with typed errors

### Acceptance Criteria

- A clean local setup ingests and retrieves representative history with PostgreSQL absent and no database credentials configured.
- Automated checks verify that the local path never initializes a PostgreSQL client or attempts a PostgreSQL connection.
- Search handles exact errors, task IDs, punctuation, empty queries, and unmatched queries without SQL or FTS syntax failures.
- Duplicate ingestion and interrupted ingestion preserve consistent, complete records.
- Rebuilding from the same frozen source records returns the same eligible IDs and ranking under the pinned implementation, with deterministic tie-breaking.
- Deleted, superseded, or unauthorized source records cannot be returned from a stale index.
- Concurrent readers and bounded writer contention behave correctly. Corruption or lock timeouts produce explicit errors with the existing optional/required-evidence behavior.
- Rebuild and rollback do not write to SOP execution-state tables or alter approvals.
- Local builds pass required supported-platform gates. Existing PostgreSQL integration checks remain required for affected code; missing infrastructure is a reported blocker for those checks, not grounds to skip them.

### Acceptance Checks

- `go test ./internal/memindex -run TestLocalIngestAndRetrieveWithoutPostgres`
- `go test ./internal/memindex -run TestLocalIndexHasNoPostgresDependency`
- `go test ./internal/memindex -run TestSearchHandlesExactErrorsTaskIDsAndPunctuation`
- `go test ./internal/memindex -run TestInterruptedIngestionPreservesRecords`
- `go test ./internal/memindex -run TestRebuildIsDeterministic`
- `go test ./internal/memindex -run TestStaleEvidenceIsNotReturned`
- `go test ./internal/memindex -run TestConcurrentReadersAndWriter`
- `go test ./internal/memindex -run TestIndexNeverCreatesExecutionStateTables`
- `go test ./internal/quality -run TestPostgresIntegrationRequiredMatchesDatabaseSensitivePaths`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3
- criterion 4: check 4
- criterion 5: check 5
- criterion 6: check 6
- criterion 7: check 7
- criterion 8: check 8
- criterion 9: check 9

### Execution

- verify-first

## RDM-003 — Add bounded advisory retrieval

Retrieve applicable history before planning and after a failed attempt when recovery is being considered. Filter by authorized repository or corpus, task context, and version compatibility. Retrieve failure records as warnings and verified correction records as candidate remedies. Attach source references and compatibility limits to each result. Bound retrieval time, result count, and injected tokens. Extend current telemetry only where needed to associate retrieved records with the current attempt and actual prompt inclusion. Use explicit experimental configuration for implementation and evaluation; installer selection is not a prerequisite.

### Dependencies

- RDM-002B

### Deliverables

- Bounded retrieval integration for planning and failure recovery
- Corpus and repository scoping with compatibility limits
- Source references and explicit typed error handling
- Telemetry associating retrieved records with the current attempt and actual prompt inclusion
- Focused regression coverage for disabled, optional-failure, required-evidence, and cross-corpus behavior

### Acceptance Criteria

- Disabled mode performs zero retrieval and preserves existing behavior.
- Optional timeout or retrieval failure records a reason and follows the existing fallback path.
- Missing required evidence blocks execution through the existing supported mechanism.
- Cross-corpus, stale-record, malicious-instruction, and concurrent-attempt fixtures preserve policy and evidence boundaries.
- Retrieved approvals never satisfy current approval requirements.
- Existing passing tests are reused when their inputs are unchanged. Rerun affected tests after changes and required full gates at integration milestones.

### Acceptance Checks

- `go test ./internal/cli -run TestAdvisoryDisabledRetrievesNothing`
- `go test ./internal/cli -run TestAdvisoryFailureFallsBackWithReason`
- `go test ./internal/cli -run TestAdvisorySkipsTasksWithRequiredEvidence`
- `go test ./internal/cli -run TestAdvisoryDropsOutOfScopeAndApprovalRecords`
- `go test ./internal/cli -run TestAdvisoryPlanningAddsUntrustedCitedItems`
- `go test ./internal/cli -run TestAdvisoryLocalIndexEndToEnd`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3
- criterion 4: check 4
- criterion 5: check 4
- criterion 6: check 6

### Execution

- verify-first

## RDM-004 — Evaluate decision quality

Create at least 30 held-out cases: 10 planning, 10 recovery, and 10 negative cases covering irrelevant evidence, obsolete fixes, missing evidence, or policy manipulation. Keep tuning cases separate. Freeze the corpus, labels, expected outcomes, and scoring rules before the final run. For each historical case, expose only evidence available before its decision point. Exclude its eventual solution and near-duplicate solution records. If sufficient independent cases do not exist, report insufficient evidence and keep the feature experimental. First compare A and B with the same model, prompt template, execution constraints, and comparable evidence-token budget. Freeze a local keyword evaluation before considering semantic retrieval.

Evaluation arms: A is current task context without historical retrieval; B is current task context plus SQLite FTS5 keyword retrieval; C (conditional) is current task context plus local hybrid keyword and semantic retrieval; P (optional) is current task context plus the existing PostgreSQL backend, with its retrieval mode explicitly recorded. Run each case three times per evaluated arm. Report unimplemented or unavailable optional arms as not evaluated. Record model version and sampling settings. Repeated runs measure variability; they do not increase the number of independent cases. Use sandboxed replay for executable tasks and a fixed, arm-blind review rubric for plans.

Measure: primary is the percentage of cases with a correct plan or verified recovery, averaged across repeats. Secondary measures repeated failed actions, unnecessary escalations, human interventions, unsupported claims, and citation accuracy. Operational measures added p50/p95 latency, token use, and actual or estimated cost. Diagnostic only: recall@5, precision@5, and MRR.

Proposed pilot gate, frozen before evaluation: the selected retrieval arm improves the primary score by at least 10 percentage points over A; C must also improve the primary score by at least 10 percentage points over B to justify hybrid retrieval; there are zero approval bypasses, cross-corpus disclosures, or required-evidence bypasses; unsupported-claim and unnecessary-escalation rates do not exceed the comparison baseline; added p95 retrieval latency is at most 2 seconds, and total token use is at most 20% above A; report case counts, paired differences, variability, and uncertainty. A small pilot supports a limited trial, not a general reliability claim.

Decision: if C is evaluated and passes, recommend an opt-in local hybrid pilot. If B passes against A and C is not evaluated or does not beat B, recommend local keyword retrieval. If no evaluated retrieval arm passes, retain disabled defaults and document the result. A passing B result is sufficient to complete the initial experiment; C and P are not mandatory deliverables. Do not tune against held-out cases and reuse them as an independent final evaluation. Freeze a new held-out set after material tuning. If failure analysis identifies relevant evidence missed because of wording differences, implement the smallest local semantic extension as task `RDM-004B`, using separate tuning cases and a fresh held-out set if earlier results informed its design.

### Dependencies

- RDM-003

### Deliverables

- Frozen evaluation corpus, labels, harness, and comparison protocol
- Go/no-go report with case counts, paired differences, variability, and uncertainty
- Explicit record of unimplemented or unavailable optional arms

### Acceptance Criteria

- At least 30 held-out cases exist with the required planning, recovery, and negative distribution; fewer is reported as insufficient evidence.
- The corpus, labels, expected outcomes, and scoring rules are frozen before the final run.
- Each case exposes only evidence available before its decision point, excluding its eventual solution and near-duplicate solutions.
- A and B are compared under the same model, prompt template, execution constraints, and comparable evidence-token budget.
- The report states a documented go/no-go decision under the frozen pilot gate.
- A small pilot is not presented as a general reliability claim.

### Acceptance Checks

- `go test ./internal/decisionquality -run TestFrozenCorpusSatisfiesContract`
- `go test ./internal/decisionquality -run TestFreezeSidecarsMatchFrozenInputs`
- `go test ./internal/dqharness -run TestRunArmThreeRepeatsAndNoLeakage`
- `go test ./internal/dqharness -run TestRunArmThreeRepeatsAndNoLeakage`
- `go test ./internal/decisionquality -run TestInsufficientEvidenceIsExplicit`
- `go run ./cmd/rdm004-eval --verify-only`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3
- criterion 4: check 4
- criterion 5: check 6
- criterion 6: check 5

## RDM-004B — Evaluate local semantic retrieval

Evaluate local semantic retrieval only if failure analysis from `RDM-004` identifies relevant evidence missed because of wording differences. Implement the smallest local semantic extension, use separate tuning cases, and use a fresh held-out set when earlier results informed its design. Do not introduce a vector service by default, and do not download embedding models automatically. When the condition is not satisfied, the task is not required and is closed with `sop task skip --reason ... --evidence ...`.

### Dependencies

- RDM-004

### Deliverables

- Failure analysis identifying missed evidence caused by wording differences
- Smallest local semantic extension, if justified
- Fresh held-out evaluation comparing C against B
- Report stating whether hybrid retrieval is recommended

### Acceptance Criteria

- Semantic retrieval is implemented only with documented evidence of a keyword-retrieval gap.
- Evaluation uses separate tuning cases and a fresh held-out set.
- No vector service or embedding model is introduced by default.
- The report states whether C improves the primary score by at least 10 percentage points over B.

## RDM-002C — Configure memory during setup

Offer Off, Local memory, and PostgreSQL during supported setup. Off is the default during the pilot and disables historical retrieval. Local memory configures local outcome files and SQLite FTS5, verifies directory access and index support, and requires no database server or embedding model. PostgreSQL uses an existing backend, collects connection settings through supported secure mechanisms, and validates connectivity and required schema capabilities. Inspect current installers and configuration flows before choosing the setup entry point and preserve existing unattended behavior. Offer equivalent noninteractive configuration for every choice. On a fresh unattended installation with no explicit choice, select Off and never wait for terminal input. Preserve an existing valid choice during upgrades. Validate the selected option before committing configuration; on failure return a clear error and preserve the previous working configuration. Provide a supported way to change the choice later. Keep backend changes nondestructive. Do not install PostgreSQL, start a database service, or download embedding models automatically. Keep passwords and connection secrets out of prompts that echo input, logs, telemetry, generated reports, and committed configuration. Skip installer expansion if the evaluation is no-go or inconclusive.

### Dependencies

- RDM-004

### Deliverables

- Interactive setup choices for Off, Local memory, and PostgreSQL
- Equivalent unattended configuration for every choice
- Nondestructive backend switching with documented identifiers
- Setup validation and actionable failure behavior

### Acceptance Criteria

- Fresh interactive setup exposes Off, Local memory, and PostgreSQL with their prerequisites and data locations explained.
- Every choice works through supported noninteractive configuration. Missing PostgreSQL settings fail clearly without a prompt or automatic backend substitution.
- Local memory setup succeeds with PostgreSQL absent. Off performs no historical retrieval or backend connection attempt.
- Invalid credentials, unavailable PostgreSQL, or incompatible schema produce actionable setup errors. Setup does not silently select Local memory or Off.
- Unwritable local storage and unavailable FTS5 fail validation before configuration is committed.
- Repeated installation and upgrades preserve valid existing settings and stored history.
- Switching Off, Local memory, and PostgreSQL changes retrieval configuration without deleting or implicitly migrating records.
- Switching Off does not satisfy or remove an independent required-evidence gate. Such a gate continues to block when its required evidence is unavailable.
- Setup failure behavior remains distinct from runtime optional retrieval fallback. Runtime failure follows the existing optional/required-evidence policy and never silently changes the configured backend.
- Local memory remains opt-in during the pilot. Making it recommended or default requires a passing evaluation and an explicit rollout decision.

## RDM-005 — Evaluate development suggestions

Use structured queries to count failures, repeated attempts, and human interventions. Use retrieval to find related explanations, decisions, and verified fixes. Generate an on-demand advisory report; do not create issues or execute proposed work automatically. Each suggestion must include the observed problem and supporting run references, affected runs and total eligible runs in the analysis window, the proposed change and the mechanism by which it could help, expected benefit labeled as an estimate plus effort and risk, and an acceptance test and current-code or issue checks for duplication.

### Dependencies

- RDM-004
- RDM-002C

### Deliverables

- On-demand advisory development-suggestion report
- Traceable evidence for every suggestion and rejected candidate
- Duplication checks against current code and open issues

### Acceptance Criteria

- Frequency claims reconcile with structured queries.
- Every suggestion has traceable evidence and a testable acceptance criterion.
- Suggestions distinguish repeated instances of one incident from independent incidents.
- Existing fixes and open issues are checked before recommending new work.
- Review at least 10 candidates. At least 7 must be judged actionable and nonduplicate by the operator before recommending continued use. Fewer than 10 is insufficient evidence.
- Report rejected candidates and reasons. Do not manufacture suggestions to meet the sample size.

## RDM-006 — Close out and propose rollout

Run required repository integration gates on the final implementation. Document backend selection, local data location, index rebuild, backup of authoritative records, evidence handling, limitations, and the measured decision. Prepare a focused PR with immutable evaluation artifact references. Provide rollback instructions using the existing optional-retrieval configuration. Confirm that disabling optional retrieval stops its reads without deleting evidence or weakening required-evidence rules. On a go result, also require `RDM-002C` and the completed or explicitly inconclusive `RDM-005` evaluation. On a no-go or inconclusive retrieval result, skip those expansion tasks and close out with the evidence report.

### Dependencies

- RDM-004
- RDM-004B
- RDM-002C
- RDM-005

### Deliverables

- Closeout report with the measured decision and limitations
- Focused PR or reviewable patch with immutable evaluation artifact references
- Rollback instructions using existing optional-retrieval configuration
- Rollout or no-go decision separated from implementation completion

### Acceptance Criteria

- If setup expansion proceeds, a fresh local installation and a preconfigured PostgreSQL installation are both verified. Local operation requires no PostgreSQL service; explicit PostgreSQL configuration preserves its behavior.
- All required checks pass on the reviewed revision.
- The PR distinguishes implementation completion from evaluation success and rollout authorization.
- Defaults remain disabled unless an explicit rollout decision authorizes a change.
- No merge, publication, or external issue creation occurs without the applicable authorization.

## DMP-001 — Build the source-backed planning corpus

Build the planning-only decision-memory pilot corpus from recorded SOP run artifacts, replacing the unevaluable RDM-004 held-out experiment. Each case must carry the recorded task input, a decision timestamp, pre-decision execution and validation evidence, and run-artifact provenance. Labels are the recorded dispositions. The classification kind, disposition, and reason are excluded from evidence.

### Dependencies

- none

### Deliverables

- `evals/decision-memory-pilot/corpus-tuning.json`
- `evals/decision-memory-pilot/corpus-heldout.json`
- `cmd/rdm-pilot-corpus/main.go`
- `evals/decision-quality/CORPUS-AUDIT.md`

### Acceptance Criteria

- Every case is source-backed with task input, decision timestamp, evidence, and provenance.
- Evidence excludes the recorded disposition and classification kind.
- Cases that cannot be established are excluded, never imputed.

### Acceptance Checks

- `go test ./internal/decisionquality -run TestPilotCorpusSatisfiesPilotContract`
- `go test ./internal/decisionquality -run TestPilotEvidenceDoesNotContainTheLabel`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 1

### Execution

- verify-first

## DMP-002 — Freeze the pilot protocol and corpus

Freeze the planning-only protocol, rubric, and both corpus splits, and prove that tuning and held-out task families never overlap.

### Dependencies

- DMP-001

### Deliverables

- `evals/decision-memory-pilot/PROTOCOL.md`
- `evals/decision-memory-pilot/RUBRIC.json`
- `evals/decision-memory-pilot/PROTOCOL.sha256`
- `evals/decision-memory-pilot/CORPUS.sha256`

### Acceptance Criteria

- Protocol and corpus freeze sidecars verify against the frozen files.
- Tuning and held-out splits share no task family.

### Acceptance Checks

- `go test ./internal/decisionquality -run TestPilotFreezeSidecars`
- `go test ./internal/decisionquality -run TestPilotFamiliesDoNotStraddleSplits`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2

### Execution

- verify-first

## DMP-003 — Execute the planning-only comparison

Run arms A and B over the frozen held-out split with the production advisory retrieval path, matched model and limits, durable checkpoint and usage accounting.

### Dependencies

- DMP-002

### Deliverables

- `evals/decision-memory-pilot/run/run-A.json`
- `evals/decision-memory-pilot/run/run-B.json`
- `evals/decision-memory-pilot/run/checkpoint.jsonl`
- `evals/decision-memory-pilot/run/usage.json`

### Acceptance Criteria

- Every held-out case and repeat is present in both arms with no duplicate record.
- Requests and tokens stay within the declared durable limits.

### Acceptance Checks

- `go test ./internal/dqharness -run TestPilotMeasuredReportVerifies`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1

### Execution

- verify-first

## DMP-004 — Report the pilot decision

Generate and verify the measured report, record GO, NO-GO, or INSUFFICIENT EVIDENCE against the frozen gates, and state that the result does not generalize to executable recovery.

### Dependencies

- DMP-003

### Deliverables

- `evals/decision-memory-pilot/run/REPORT.md`
- `docs/reports/rdm-decision-memory/ACCEPTANCE-EVIDENCE.md`

### Acceptance Criteria

- Report metrics match the recomputed values from the raw run artifacts.
- The decision follows the frozen gates and is not generalized to executable recovery.

### Acceptance Checks

- `go test ./internal/dqharness -run TestPilotMeasuredReportVerifies`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1

### Execution

- verify-first
