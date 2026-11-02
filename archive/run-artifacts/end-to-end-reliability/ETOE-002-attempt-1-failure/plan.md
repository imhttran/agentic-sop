# Implementation Plan

## Project

SOP End-to-End Reliability — ETOE-002 Core workflow contract audit

## Summary

Read-only production of docs/reports/end-to-end-reliability/ETOE-002-workflow-contract-matrix.md: a requirement-to-test matrix mapping every core workflow transition (IMPLEMENT, VALIDATE, REVIEW, FIX/retry, approval refusal, closure) and every approval-refusal path to existing test coverage in agentic-sop, listing each uncovered requirement as an explicit gap and distinguishing unit-test-verified behavior from behavior that requires a real disposable run (owned by ETOE-005/ETOE-006). Work is decomposed along natural boundaries: contract enumeration from authoritative SOP sources (docs/specs, docs/reference, docs/architecture) plus the persisted task/execution/approval state model (internal/domain); test inventory and coverage mapping across internal/domain, internal/quality, internal/review, internal/validate, internal/recovery, internal/completion, internal/taskbuilder, internal/e2e, internal/failure, internal/runtrace, internal/runmetrics; and report assembly with gap classification. ETOE-001's baseline report is treated as reusable dependency evidence (stage dependency, not an UNKNOWN prerequisite).

## Capabilities

### Core workflow state model and transitions — EXISTS

- Location: internal/domain/
- Owner: agentic-sop (state, policy, execution, gates, approvals)
- Evidence: internal/domain/task.go, status.go, execution.go, retry.go, selection.go, approval.go present with matching _test.go files (approval_test.go, complete_external_test.go, executed_test.go, execution_test.go, not_required_test.go, retry.go, selection_test.go, selection_extra_test.go, status.go, task_test.go)

### Documented workflow/validation/review/approval specs and CLI reference — EXISTS

- Location: docs/specs, docs/reference, docs/architecture, docs/testing
- Owner: agentic-sop documentation
- Evidence: docs/specs/ (RECOVERY.md, PLAN-HISTORICALIZATION.md named in ETOE-001 §5), docs/reference/ (CLI.md, STATUS-AND-RECOVERY.md), docs/architecture/, docs/testing/ directories present in docs/

### Validation / quality gate implementation and tests — EXISTS

- Location: internal/validate, internal/quality, internal/ci, internal/testrunner
- Owner: agentic-sop
- Evidence: internal/validate/, internal/quality/, internal/ci/, internal/ciremediation/, internal/archtest/, internal/testrunner/ packages present; SOP gate config surfaces as build/test/lint per plan (agentic-sop/.agent-sdlc/config.yaml, quality.fail_on [critical, high] cited in ETOE-001 §11)

### Review / FIX / retry path implementation and tests — EXISTS

- Location: internal/review, internal/recovery, internal/failure, internal/domain
- Owner: agentic-sop
- Evidence: internal/review/, internal/recovery/, internal/continuation/, internal/failure/, internal/resume/, internal/retry surface via internal/domain/retry.go and retry-related tests

### Approval / refusal / supersede handling — EXISTS

- Location: internal/approval, internal/domain/approval.go
- Owner: agentic-sop
- Evidence: internal/approval/ package; internal/domain/approval.go + approval_test.go + not_required_test.go + complete_external_test.go; `approval supersede <task-id> --reason --by` documented in ETOE-001 §3 (source binary only)
- Gap: Installed binary on PATH lacks `approval supersede`; only the source build exposes it (ETOE-001 §3). This audit reads code/tests, so it does not gate on the binary.
- Resolution: Record the installed-binary delta in the matrix as a coverage caveat sourced from ETOE-001 §3; do not require the installed binary.

### Closure / completion transitions — EXISTS

- Location: internal/completion, internal/domain/status.go
- Owner: agentic-sop
- Evidence: internal/completion/, internal/domain/status.go, docs/specs/PLAN-HISTORICALIZATION.md, archive dirs under .agent-sdlc/archive (LC and CONV COMPLETE per ETOE-001 §4.2)

### Trace and metrics instrumentation — EXISTS

- Location: internal/runtrace, internal/runmetrics, internal/activity
- Owner: agentic-sop
- Evidence: internal/runtrace/, internal/runmetrics/, internal/activity/, internal/run/, internal/agent/, internal/orchestration/ packages present
- Gap: Metric instrumentation coverage per transition is not yet known from package presence alone.
- Resolution: Enumeration of which trace/metrics fields each transition emits is work: inspect the packages and record EXISTS/PARTIAL per transition in the matrix, labeling uninstrumented metrics UNAVAILABLE.

### Existing test corpus (unit + e2e) for core workflow — EXISTS

- Location: internal/**/*_test.go, internal/e2e, testdata, evals
- Owner: agentic-sop
- Evidence: _test.go files across internal/domain, internal/e2e/, internal/archtest/, internal/failure/, internal/review/, internal/validate/, internal/quality/, internal/approval/, internal/recovery/, internal/completion/; testdata/ and evals/ fixture roots present
- Gap: No index exists mapping workflow requirements to covering tests; that mapping is the deliverable of this task.
- Resolution: Produce the matrix by inspecting the test corpus; uncovered requirements are listed as gaps in the report.

### ETOE-001 preflight baseline artifact — EXISTS

- Location: docs/reports/end-to-end-reliability/ETOE-001-preflight-baseline.md
- Owner: ETOE-001 stage (completed, LOCAL_DONE)
- Evidence: docs/reports/end-to-end-reliability/ETOE-001-preflight-baseline.md present with operator-verified Git/binary/plan-status facts (§1–§5), backlog finding §11, and re-review finding §12
- Resolution: Reused as dependency evidence via stage dependency, not rediscovered.

### Real disposable SOP run fixture — MISSING

- Location: owned by ETOE-005 (this plan's later stage)
- Owner: ETOE-005 (Disposable dogfood fixture and deterministic baseline)
- Evidence: PLAN-SOP-End-to-End-Reliability.md states the disposable dogfood fixture is owned by ETOE-005 and 'not inspected here' in ETOE-001 §6; ETOE-001 §7 lists it UNAVAILABLE for that stage
- Gap: No disposable fixture exists yet, so no behavior can be verified by real disposable run during ETOE-002.
- Resolution: ETOE-002 is scoped explicitly to existing-test coverage only and must classify every runtime-only requirement into a distinct 'requires real disposable run (ETOE-005/ETOE-006)' column rather than asserting it. No ETOE-002 stage requires the fixture at runtime.

### Decision-adapter integration into agentic-sop — UNKNOWN

- Location: internal/cli/decision_provider.go, .agent-sdlc/config.yaml, sop-decision-adapters
- Owner: SOP decision seam (agentic-sop) configuring sop-decision-adapters
- Evidence: ETOE-001 §7: no module import of sop-decision-adapters, no `decision:` section in agentic-sop/.agent-sdlc/config.yaml; only the process-level `decision.provider: command` seam exists (internal/cli/decision_provider.go)
- Gap: Wiring status unverified; integration is out of ETOE-002 scope (owned by ETOE-004/ETOE-008).
- Resolution: Informational only. ETOE-002 records decision routing as out of scope in the matrix and defers to ETOE-004/ETOE-008; no stage requires it.

### SOP CLI binary capable of supersede/--max-tasks — PARTIAL

- Location: cmd/sop, /Users/imhttran/go/bin/sop
- Owner: agentic-sop CLI (source build / operator environment)
- Evidence: ETOE-001 §3: installed /Users/imhttran/go/bin/sop (sop dev) lacks `run --max-tasks` and `approval supersede`; source build exposes both
- Gap: Installed binary does not expose the full audited command surface.
- Resolution: ETOE-002 is read-only documentation analysis and requires no binary; the delta is recorded in the matrix as a coverage caveat rather than gating any stage.

## Assumptions

### ETOE-002 remains strictly read-only: the only repository change is the new report file under docs/reports/end-to-end-reliability/.

- Evidence: Task statement: 'Core workflow contract audit: state transitions, validation/review/FIX/retry, approval refusal, closure, and trace/metrics. Read-only.'; PLAN-SOP-End-to-End-Reliability.md ETOE-002 mirrors this.
- Consequence: No source, config, test, or SOP-state mutation; findings that suggest code changes become gap entries / follow-up backlog items, not edits.

### ETOE-001's operator-corrected baseline is authoritative for cross-repository and binary facts and need not be re-derived.

- Evidence: ETOE-001-preflight-baseline.md §1–§5 marked [operator-verified]; §10 correction log supersedes model-generated UNAVAILABLE claims.
- Consequence: ETOE-002 cites ETOE-001 by section instead of re-running Git/binary probes, avoiding duplicated and conflicting evidence.

### Requirements are enumerated from authoritative SOP sources (docs/specs, docs/reference, docs/architecture) plus the persisted state model in internal/domain, not invented from tests.

- Evidence: docs/specs/RECOVERY.md, docs/reference/CLI.md, docs/reference/STATUS-AND-RECOVERY.md, docs/specs/PLAN-HISTORICALIZATION.md named in ETOE-001 §5; internal/domain contains task.go, status.go, execution.go, retry.go, selection.go, approval.go.
- Consequence: The matrix is requirement-driven; a requirement with no test is reported as a gap rather than the matrix being constrained to what happens to be tested.

### Test files are the coverage evidence for unit-verified behavior; runtime-only behavior cannot be marked covered without a real disposable run.

- Evidence: Safety invariant 7: 'Do not claim end-to-end success based solely on mocked unit tests.'; PLAN Existing boundaries note real disposable fixture is ETOE-005.
- Consequence: The matrix needs a three-way classification: unit-verified, real-disposable-run required, and uncovered/gap; the second class is explicitly not a gap in ETOE-002 terms but is flagged for ETOE-006.

### Package/directory presence indicates an implementation boundary but not the completeness of its test coverage for a specific transition.

- Evidence: internal/ listing shows packages such as validate, quality, review, recovery, completion, runtrace, runmetrics without per-transition coverage detail from the directory listing alone.
- Consequence: Per-transition coverage status is discovered during the test-inventory stage and each transition is classified individually; uninstrumented trace/metrics fields are labeled UNAVAILABLE.

### The report path docs/reports/end-to-end-reliability/ETOE-002-workflow-contract-matrix.md is the single exact deliverable path and the directory already exists.

- Evidence: Deliverable named in the task and in PLAN-SOP-End-to-End-Reliability.md; docs/reports/end-to-end-reliability/ currently contains ETOE-001-preflight-baseline.md.
- Consequence: No directory creation ambiguity; the stage writes exactly one file and preserves ETOE-001's report untouched.

## ETOE-002-A — Workflow requirement inventory (transitions, refusal paths, closure, trace/metrics)

Enumerate the core workflow requirements to be audited, from authoritative sources only: read docs/specs (RECOVERY.md, PLAN-HISTORICALIZATION.md and any validation/review/approval specs present), docs/reference (CLI.md, STATUS-AND-RECOVERY.md), docs/architecture, and the persisted state model in internal/domain (task.go, status.go, execution.go, retry.go, selection.go, approval.go). Produce a numbered requirement list covering: each state transition (IMPLEMENT, deterministic VALIDATE, REVIEW, FIX-if-triggered, retry/requeue, approval request/grant/refusal, plan/task closure), each approval-refusal path (including refusal without authorization, refusal on non-approvable state, stale-approval supersede), and the trace/metrics obligations emitted per transition. Cite the source file/section for every requirement. Where a requirement is only asserted by prose and not by a machine-checkable state rule, mark it as such. ETOE-001's operator-verified baseline is consumed as input (Git/binary/plan-status facts) rather than re-derived.

### Dependencies

- ETOE-002-0

### Deliverables

- Working requirement inventory (numbered, source-cited) covering transitions, approval-refusal paths, closure, and trace/metrics obligations
- List of requirements that are prose-only vs. state-rule-backed

### Acceptance Criteria

- Every numbered requirement names its authoritative source by file path and section.
- The inventory covers all three audit dimensions from the task statement: state transitions, approval refusal, and trace/metrics, plus closure.
- Requirements are written in the PRD's own terminology (IMPLEMENT, VALIDATE, REVIEW, FIX, retry, approval, closure, trace, metrics).
- No requirement is derived solely from the existence of a test.
- No repository file other than the ETOE-002 report deliverable is modified.

## ETOE-002-B — Test inventory across workflow-relevant packages

Inventory the existing test corpus that could cover the ETOE-002-A requirements. Enumerate *_test.go across internal/domain, internal/approval, internal/validate, internal/quality, internal/review, internal/recovery, internal/completion, internal/taskbuilder, internal/failure, internal/e2e, internal/archtest, internal/runtrace, internal/runmetrics, internal/run, internal/orchestration, plus testdata/ and evals/ fixtures; for each test file record package, test/helper names, the transition or refusal path it appears to exercise, and whether it uses mocks/fakes versus a real run harness. Distinguish unit-level tests from integration/e2e harness tests (e.g. internal/e2e) and from architecture tests (internal/archtest). Read ETOE-001 §6 fixture table as prior evidence for fixture availability.

### Dependencies

- ETOE-002-A

### Deliverables

- Test inventory table: file, package, test name, apparent target transition/refusal path, mock-vs-real classification
- Notes on harness type (unit, architecture, e2e-in-repo) per test file

### Acceptance Criteria

- Every _test.go file in the listed workflow-relevant packages appears in the inventory or is explicitly excluded with a reason.
- Each entry states whether the test is unit-level, architecture-level, or an in-repo e2e harness test.
- Fixtures under testdata/ and evals/ used by inventoried tests are named by path.
- The inventory does not assert coverage yet — that is ETOE-002-C.

## ETOE-002-C — Requirement-to-test coverage mapping and three-way classification

Join ETOE-002-A requirements to ETOE-002-B tests. For each requirement assign exactly one classification: (1) covered by unit tests, (2) requires a real disposable run (no unit test can establish it — e.g. end-to-end approval refusal, real gate execution, real trace/metrics emission), or (3) gap — no covering test of any kind. For trace/metrics requirements, additionally record whether the audit code inspection shows the field is actually emitted (EXISTS) or not instrumented (label UNAVAILABLE). Cite the specific test name(s) supporting class (1). Explicitly note where coverage relies on mocks and therefore cannot be promoted to class (1) evidence per safety invariant 7.

### Dependencies

- ETOE-002-B

### Deliverables

- Requirement-to-test mapping with one classification per requirement and supporting test names for unit-verified rows
- Explicit gap list: every requirement with no covering test
- Runtime-only list: requirements that can only be verified by a real disposable run (routed to ETOE-005/ETOE-006)

### Acceptance Criteria

- Every requirement from ETOE-002-A has exactly one classification: unit-verified, real-disposable-run required, or gap.
- Every unit-verified row cites at least one concrete test name and file path.
- Every requirement with no covering test appears in the explicit gap list.
- The gap list and the runtime-only list are disjoint and both are non-empty-or-explicitly-stated-empty with justification.
- No requirement is classified as unit-verified solely on the basis of a mocked test where the requirement concerns real gating, real approval refusal, or real closure.
- Trace/metrics rows state per-field EXISTS or UNAVAILABLE based on code inspection, with the inspected path cited.

## ETOE-002-D — Report assembly, acceptance self-check, and gap handoff

Write docs/reports/end-to-end-reliability/ETOE-002-workflow-contract-matrix.md containing: scope and method; the requirement-to-test matrix (requirement, source citation, classification, covering test(s), notes); the explicit gap list; the runtime-only list with the ETOE-005/ETOE-006 handoff labels; a trace/metrics instrumentation summary with UNAVAILABLE labels where not instrumented; and coverage caveats inherited from ETOE-001 §3 (installed binary lacks `approval supersede` and `run --max-tasks`) and ETOE-001 §11–§12 (acceptance-criteria completeness and re-review limitations) reproduced without re-deriving them. Preserve ETOE-001's report untouched. Verify the finished document against the three task acceptance criteria before considering the stage done.

### Dependencies

- ETOE-002-C

### Deliverables

- docs/reports/end-to-end-reliability/ETOE-002-workflow-contract-matrix.md

### Acceptance Criteria

- A requirement-to-test matrix maps each core workflow transition and each approval-refusal path to existing test coverage (or to an explicit gap).
- Every requirement with no covering test is listed explicitly as a gap.
- The matrix distinguishes behavior verified by unit tests from behavior requiring a real disposable run.
- ETOE-002's three acceptance criteria are each addressed with a pointer to the matrix section that satisfies them.
- Coverage caveats sourced from ETOE-001 §3, §11, and §12 are reproduced with section references and not re-litigated.
- Only docs/reports/end-to-end-reliability/ETOE-002-workflow-contract-matrix.md is created; ETOE-001-preflight-baseline.md and all other repository files and SOP state are unmodified.

## ETOE-002-0 — Audit working environment and source-of-truth confirmation

Establish the read-only working baseline for the audit within this repository only: confirm docs/reports/end-to-end-reliability/ exists and ETOE-001-preflight-baseline.md is present and unchanged, confirm the expected documentation roots (docs/specs, docs/reference, docs/architecture, docs/testing) and the workflow-relevant internal/ packages exist, and confirm the deliverable path from the task and PLAN-SOP-End-to-End-Reliability.md matches. No new external tooling is installed and no repository or SOP state is modified; ETOE-001's operator-verified facts are accepted as the external baseline input.

### Dependencies

None

### Deliverables

- Confirmed read-only baseline note listing the paths to be used and the deliverable path

### Acceptance Criteria

- docs/reports/end-to-end-reliability/ and ETOE-001-preflight-baseline.md are confirmed present and unmodified.
- The ETOE-002 deliverable path matches the task statement and the plan document.
- No file is created or modified by this stage.
- No external runtime, permission, or service beyond repository inspection is required by this stage.

