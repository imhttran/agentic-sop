# Implementation Plan

## Project

SOP End-to-End Reliability — ETOE-004 Decision adapter integration audit

## Summary

Read-only, report-only audit plan (no code or SOP-state change) producing docs/reports/end-to-end-reliability/ETOE-004-adapter-integration-matrix.md, which explicitly distinguishes decision-layer capabilities INTEGRATED into SOP from STANDALONE-ONLY ones. The authoritative parent plan (docs/plans/PLAN-SOP-End-to-End-Reliability.md) binds the integration status to UNAVAILABLE: agentic-sop does not import the sop-decision-adapters module and its .agent-sdlc/config.yaml has no decision: section; the only integration surface is the process-level decision.provider: command seam. Four small stages: (1) evidence inventory and sibling-reachability probes, (2) the integrated-versus-standalone-only matrix over LOW/MEDIUM/HIGH routing, normalized results, fallbacks, and failure handling, (3) the explicit UNAVAILABLE/gap ledger with deferred-verification pointers to ETOE-008, (4) report assembly at the exact deliverable path plus recorded required validation. No stage assumes a capability the repository does not show; the standalone adapter repository may be unreachable this run and is kept informational, and runtime wiring exercise is owned by ETOE-008, not this audit.

## Capabilities

### decision-command configuration seam (decision.provider: command plus decision.command) — EXISTS

- Location: internal/config/config.go (schema :114, DecisionConfig :292-308, validation :581-588); internal/config/decision_test.go
- Owner: agentic-sop internal/config
- Evidence: internal/config/config.go:114 declares `Decision DecisionConfig yaml:"decision"`; config.go:292-308 documents DecisionConfig with a restricted Provider set and Command as "the executable argv of an external, provider-neutral decision" adapter, "empty unless a decision provider is explicitly configured"; config.go:581-588 validation rejects unknown providers ("want deterministic, jev, command") and errors "decision.provider \"command\" requires decision.command"; internal/config/decision_test.go:20-29 parses `provider: command` with a command argv.

### decision-layer policy: disabled-by-default deterministic provider and LOW/MEDIUM/HIGH thresholds — EXISTS

- Location: internal/config/config.go (:399, :528-529, :744-746); internal/config/decision_test.go
- Owner: agentic-sop internal/config + internal/decision
- Evidence: internal/config/config.go:399 and :528-529 default the thresholds to RouteToStrongModel 0.7 / RequireHuman 0.4 (the LOW/MEDIUM/HIGH routing bounds); internal/config/decision_test.go:6-13 asserts the default decision config is "disabled, deterministic, no command"; config.go:744-746 documents that decision-layer signals select or describe the layer and cannot turn the layer on themselves.

### internal/decision decision-layer package (normalized decision types/thresholds) — EXISTS

- Location: internal/decision (via the import at internal/config/config.go:23)
- Owner: agentic-sop internal/decision
- Evidence: internal/config/config.go:23 imports "github.com/imhttran/agentic-sop/internal/decision" and uses decision.Thresholds as the typed thresholds schema (:298, defaults wired at :399/:528-529), so the package exists in-tree with the normalized decision-layer types.

### SOP runtime decision-provider invocation path (execute configured decision.command argv, consume normalized results, fallbacks, failure handling) — PARTIAL

- Location: agentic-sop internal run loop (expected under internal/cli / internal/decision); to be located by repo-wide call-path search
- Owner: agentic-sop internal run loop (internal/cli, internal/decision)
- Evidence: Configuration-side of the path is proven (seam schema plus validation, capability 1), and the authoritative parent plan describes the process-level decision.provider: command seam as the only integration surface; however, the run-loop call site that invokes the configured argv, the normalized-result contract consumed there, and the fallback/failure handling were not directly observed in this planning session — ETOE-002's recorded gap defers decision routing to ETOE-004/ETOE-008 (docs/reports/end-to-end-reliability/ETOE-002-attempt-1-failure/plan.md:87-88: "Wiring status unverified; integration is out of ETOE-002 scope (owned by ETOE-004/ETOE-008)").
- Gap: Exact invocation site, normalized-result schema, and fallback/failure-handling behavior must be enumerated from code paths during the audit, not assumed present or absent.
- Resolution: Enumerated as E4-S1 inventory work (repo-wide search of internal/decision consumers and DecisionConfig call sites); classified integrated-versus-unavailable in E4-S2 matrix rows, cited only from reachable evidence.

### real SOP-to-adapter integration (configured and verified wiring between agentic-sop and sop-decision-adapters) — MISSING

- Location: agentic-sop .agent-sdlc/config.yaml (decision: section absent per binding plan) and go.mod (no sop-decision-adapters import per binding plan)
- Owner: agentic-sop project configuration (.agent-sdlc/config.yaml) + operator wiring; runtime exercise owned by ETOE-008 per the parent plan task graph
- Evidence: Binding statement in docs/plans/PLAN-SOP-End-to-End-Reliability.md § Existing boundaries: "`agentic-sop` does not import the `sop-decision-adapters` module, and its `.agent-sdlc/config.yaml` has no `decision:` section (the default is disabled / deterministic)" and "Until real SOP-to-adapter wiring is configured and verified, decision-adapter integration must be reported as UNAVAILABLE — never asserted from standalone adapter tests alone". To be re-verified in-session by reading .agent-sdlc/config.yaml and go.mod (E4-S1).
- Gap: Decision-adapter integration is UNAVAILABLE until real SOP-to-adapter wiring is configured and verified; standalone adapter tests are not evidence of integration.
- Resolution: The ETOE-004 report marks integration UNAVAILABLE and names the exact seam (decision.provider: command plus decision.command) with the audited-project configuration status; actual wiring exercise is ETOE-008's scope per the parent plan.

### sop-decision-adapters standalone adapter repository access — UNKNOWN

- Location: sibling repository sop-decision-adapters (outside agentic-sop)
- Owner: sop-decision-adapters repository / this run's authorized tool roots
- Evidence: The parent plan lists `sop-decision-adapters` as an integration target (PLAN-SOP-End-to-End-Reliability.md header and § Existing boundaries), but ETOE-003 §2.1 proved in that audit run that sibling roots are outside the authorized tool surface (three probe surfaces rejected: unauthorized root, path escape, git -C); ETOE-001 §1 records operator-verified reachability only for the ETOE-001 harness. Whether this ETOE-004 run can read the sibling checkout is unresolved.
- Gap: May be unreachable during execution, in which case no adapter-side code or document can be read directly.
- Resolution: Informational only — no stage requires it. E4-S1 probes it and records attempts and outcomes verbatim; if unreachable, adapter capabilities are assessed solely from reachable in-repo documents and labeled UNDOC/UNAVAILABLE per the ETOE-003 §1 basis-statement model, never asserted.

### in-repo standalone-adapter artifacts (evals/, integrations/, testdata/, skills/) — UNKNOWN

- Location: agentic-sop evals/, integrations/, testdata/, skills/
- Owner: agentic-sop repository (maintainers own their contents)
- Evidence: The repository listing shows top-level evals/, integrations/, testdata/, and skills/ directories, but their contents were not inventoried in this planning session; whether any hold standalone decision-adapter fixtures, eval suites, or skills is unresolved.
- Gap: Standalone-only versus integrated classification of these artifacts cannot be pre-decided.
- Resolution: Informational only — no stage requires it. E4-S1 inventories the decision-relevant contents; E4-S2 classifies each as integrated or standalone-only with citations; anything ambiguous is labeled UNDOC/UNAVAILABLE.

### decision spec and prior decision-integration documentation (reachable basis) — EXISTS

- Location: docs/reports/decision-integration/, docs/specs/MODEL-ROUTING.md, docs/specs/PROVIDERS.md, docs/reports/end-to-end-reliability/ETOE-001-preflight-baseline.md
- Owner: agentic-sop documentation layer
- Evidence: docs/reports/decision-integration/ exists (repository listing); ETOE-003 cites controller-boundary decision notes at docs/specs/MODEL-ROUTING.md:337 and docs/specs/PROVIDERS.md:354 (display-only routing posture); ETOE-001's preflight baseline exists at docs/reports/end-to-end-reliability/ETOE-001-preflight-baseline.md.

### completed ETOE matrix report conventions (evidence inventory / legend / mutation statement / acceptance mapping) — EXISTS

- Location: docs/reports/end-to-end-reliability/
- Owner: agentic-sop docs/reports/end-to-end-reliability
- Evidence: docs/reports/end-to-end-reliability/ETOE-003-controller-contract-matrix.md (read in full this session) establishes the audited-report structure: §1 scope/legend (DOC/UNDOC/UNAVAILABLE), §2 evidence inventory with reachability probes, §3 matrix, §4 analysis, §5 explicit gaps, §6 deferred test plan, §7 required-validation status, §8 mutation statement, §9 acceptance-criteria mapping; ETOE-002-workflow-contract-matrix.md is present in the same directory.

### Go toolchain (validation runtime for the read-only gate set) — EXISTS

- Location: development environment (go.mod at repository root)
- Owner: development environment (operator-supplied runtime)
- Evidence: go.mod/go.sum at the repository root; ETOE-003 §7 recorded the gate set `go build ./...`, `go test ./...`, `go vet ./...`, and the `gofmt -l .` emptiness test executing with exit 0 in this checkout, with go1.27.1 as the recorded toolchain (cited as prior evidence; not re-probed during planning).

### full plan-required validation command surface (go test -race -count=1 ./... and git diff --check) — PARTIAL

- Location: run command allow-list / SOP validation policy (parent plan § Required validation)
- Owner: the run's command allow-list / SOP validation configuration
- Evidence: The authoritative plan § Required validation requires `go test -race -count=1 ./...` and `git diff --check` and states they are "Required by this plan but NOT enforced by SOP's configured gate"; ETOE-003 §7 recorded both as UNAVAILABLE that run (not admitted by the command allow-list).
- Gap: Both commands may again be unadmitted during execution.
- Resolution: E4-S4 runs and records them if admitted, otherwise records them explicitly UNAVAILABLE with the gate-enforced versus plan-required distinction — never claiming they ran without execution.

## Assumptions

### The audited project for the seam-configuration question is the agentic-sop checkout itself (its own .agent-sdlc/config.yaml).

- Evidence: The binding plan § Existing boundaries speaks of "its `.agent-sdlc/config.yaml`" for agentic-sop; the task says "the audited project", and ETOE-003 audited agentic-sop as the reachable project.
- Consequence: The report's answer to "is the seam configured" is for agentic-sop's config; if the operator intends a different audited project, that acceptance-criterion answer changes and must be re-audited.

### The sibling sop-decision-adapters checkout may not be an authorized tool root during this run.

- Evidence: ETO E-003 §2.1 recorded all three probe surfaces (rooted read, relative escape, git -C) rejected for a sibling checkout in that audit run.
- Consequence: Adapter-side capabilities are assessed only from reachable in-repo documents (docs/reports/decision-integration/, docs/specs, in-repo adapter artifacts); anything beyond is UNDOC/UNAVAILABLE and no integration or adapter behavior is asserted.

### This task mutates nothing but the single report file; it is a documentation-only audit.

- Evidence: The sole deliverable is the report file; the binding plan's safety invariants forbid state mutation and bypasses, and ETOE-003 §8 demonstrates the mutation-statement pattern for read-only audits.
- Consequence: All audit findings become report rows and backlog candidates for ETOE-008/ETO E-010; no config, code, gate, or .agent-sdlc change is made from findings.

### The binding UNAVAILABLE integration status governs the whole report regardless of what standalone adapter tests or fixtures are found.

- Evidence: Parent plan § Existing boundaries: integration "must be reported as UNAVAILABLE — never asserted from standalone adapter tests alone (see ETOE-004 and ETOE-008)"; ETOE-004 acceptance criterion 2 repeats it.
- Consequence: The matrix keeps an explicit integration column marked UNAVAILABLE for SOP-to-adapter wiring; standalone-only rows are labeled as such and never cited as integration evidence; runtime wiring evidence is deferred to ETOE-008.

### ETO E-001's dependency is already satisfied by its completed in-tree report, cited as prior evidence rather than re-derivation.

- Evidence: The parent plan's task index makes ETOE-004 depend on ETOE-001, and docs/reports/end-to-end-reliability/ETOE-001-preflight-baseline.md exists; ETOE-003 cites the same report as prior operator-verified evidence.
- Consequence: No in-plan stage re-executes the preflight; E4-S1 cites ETOE-001's recorded revisions/reachability as labeled prior evidence and adds only what it probes itself.

## E4-S1 — Evidence inventory and reachability probes

Build the complete read-only evidence inventory: locate and cite the decision config schema, defaults, and validation (internal/config/config.go:114, :292-308, :399, :528-529, :581-588 and decision_test.go), the internal/decision package, and every SOP code path that consumes DecisionConfig or would invoke a configured decision.command argv (repo-wide call-path search of internal/decision and internal/cli); re-read .agent-sdlc/config.yaml and record verbatim whether any decision: section is present; search go.mod/go.sum for any sop-decision-adapters module import; enumerate decision-relevant contents of docs/specs/MODEL-ROUTING.md, docs/specs/PROVIDERS.md, docs/reports/decision-integration/, the ETOE-001/002 reports, and evals/, integrations/, testdata/, skills/; probe the sibling sop-decision-adapters checkout reachability with every attempt and outcome recorded verbatim (mirroring ETOE-003 §2.1).

### Dependencies

None

### Deliverables

- ETOE-004 report §2 'Evidence inventory' section content: config-schema and defaults citations, invocation-site call paths found, .agent-sdlc/config.yaml decision-state observation, go.mod import-absence record, adapter-artifact inventory, sibling probe transcript (staged; assembled into the final report by E4-S4)

### Acceptance Criteria

- Every inventory row carries an exact repository path (with line numbers where citeable) — no uncited assertion.
- The .agent-sdlc/config.yaml decision: section presence/absence is observed by directly reading the file in-session, not inferred from the plan alone.
- go.mod/go.sum are searched for the sop-decision-adapters import and the observed result is recorded explicitly.
- The sibling-reachability probe records each attempt and its exact outcome verbatim, or the unavailable probe surface itself is recorded as the reason no probe ran.
- The inventory covers all decision-relevant contents of evals/, integrations/, testdata/, skills/ and the in-repo decision documentation, so no surface is silently skipped.
- No repository file is modified in this stage.

## E4-S2 — Decision-layer integration matrix (integrated vs standalone-only)

Construct the integration matrix that classifies every decision-layer capability as INTEGRATED-in-SOP or STANDALONE-ONLY for actual SOP wiring versus a standalone adapter: deterministic default provider, jev provider, command provider seam (decision.provider: command plus decision.command, its argv validation and configuration state from E4-S1), LOW/MEDIUM/HIGH routing (RouteToStrongModel/RequireHuman thresholds), normalized results, fallbacks, and failure handling — each row labeled with its level and citations using the DOC/UNDOC/UNAVAILABLE legend.

### Dependencies

- E4-S1

### Deliverables

- ETOE-004 report §3 'Integration matrix' section content: per-capability rows with integrated/standalone-only classification, the SOP-to-adapter wiring row marked UNAVAILABLE and naming the exact seam with its audited-project configuration status (staged; assembled by E4-S4)

### Acceptance Criteria

- The matrix contains dedicated rows for LOW/MEDIUM/HIGH routing, normalized results, fallbacks, and failure handling, and every row is explicitly labeled integrated-in-SOP or standalone-only.
- Every integrated row cites an in-SOP code path or spec (exact path/line); every standalone-only row cites adapter-side or artifact-side reachable evidence and asserts nothing about SOP behavior.
- The SOP-to-adapter wiring row names the exact integration seam (decision.provider: command plus decision.command) and states from E4-S1's direct observation whether it is configured in the audited project; integration is marked UNAVAILABLE.
- No standalone adapter test or fixture is cited anywhere in the matrix as evidence of integration.
- Every label is backed by an E4-S1 evidence row; undocumented behavior is never asserted.

## E4-S3 — UNAVAILABLE integration statement and gap ledger

Write the explicit absence/gap ledger: the binding UNAVAILABLE integration statement (quoted from the authoritative plan § Existing boundaries), the named seam and its configuration status, the standalone-only findings, every UNDOC/UNAVAILABLE item with its reason (including sibling checkout reachability), and deferred-verification pointers that map each unverified row to ETOE-008 with adapter tests run offline first per the parent plan's required-validation note.

### Dependencies

- E4-S2

### Deliverables

- ETOE-004 report §4 'Explicit gaps' and §5 'Deferred verification' section content: gap ledger with owners and dispositions, UNAVAILABLE/UNDOC items with reasons, and ETOE-008 test-plan pointers (staged; assembled by E4-S4)

### Acceptance Criteria

- The report states, as a standalone section, that decision-adapter integration is UNAVAILABLE until real SOP-to-adapter wiring is configured and verified, and that standalone adapter tests are not evidence of integration.
- Each gap names its owning layer (e.g. agentic-sop project configuration + operator wiring for the seam; the run tool root for sibling access) and a disposition pointing to ETOE-008 or the operator.
- Every capability unverifiable in this run is labeled UNDOC or UNAVAILABLE with its reason — none is asserted present.
- No readiness, integration, or adapter-behavior claim appears beyond the DOC/UNDOC/UNAVAILABLE labels.

## E4-S4 — Report assembly and required validation

Assemble and finalize docs/reports/end-to-end-reliability/ETOE-004-adapter-integration-matrix.md with all staged sections, a scope-and-method header with the verification-level legend, a mutation statement treating pre-existing worktree entries as user-owned, and a §9-style acceptance-criteria mapping; then run and record the required validation set, distinguishing SOP-gate-enforced commands from plan-required-but-not-gate-enforced ones.

### Dependencies

- E4-S1
- E4-S2
- E4-S3

### Requires

- Go toolchain (validation runtime for the read-only gate set)

### Deliverables

- docs/reports/end-to-end-reliability/ETOE-004-adapter-integration-matrix.md — the final report with evidence inventory, integration matrix, explicit gaps, deferred-verification pointers, mutation statement, and acceptance-criteria mapping
- Recorded required-validation results for the audit's repository change (exit codes, with the gate-enforced versus plan-required distinction stated)

### Acceptance Criteria

- The report exists at the exact deliverable path and its acceptance-criteria mapping table covers all three ETOE-004 acceptance criteria, each pointing to the section that evidences it.
- The report explicitly distinguishes integrated versus standalone-only capabilities (criterion 1), marks decision-adapter integration UNAVAILABLE with standalone tests not cited as evidence (criterion 2), and names the seam decision.provider: command plus decision.command with the audited project's configuration status (criterion 3).
- The recorded validation covers the SOP-gate set (go build ./..., go test ./..., go vet ./..., gofmt -l . emptiness) with exit codes; go test -race -count=1 ./... and git diff --check are either run and recorded, or explicitly marked UNAVAILABLE as plan-required-but-not-gate-enforced — never claimed without running.
- The mutation statement identifies the report file as the sole intended repository change, preserves unrelated pre-existing worktree entries, and records zero .agent-sdlc/SOP-state modification.
- Report terminology preserves the task's vocabulary: actual SOP wiring versus a standalone adapter, LOW/MEDIUM/HIGH routing, normalized results, fallbacks, failure handling, integrated vs standalone-only, UNAVAILABLE.

