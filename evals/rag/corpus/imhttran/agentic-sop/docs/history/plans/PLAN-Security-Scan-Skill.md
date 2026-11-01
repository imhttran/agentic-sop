# Implementation Plan

## Project

agentic-sop

## Summary

Add a reusable sop-security-scan skill and the supporting Go/Rust dependency-scanning capability to the existing internal/security adapter infrastructure. Reuse the existing adapter contract, runner, policy, redaction, provenance, baseline, artifact, and resume mechanisms as the single execution path. Add a built-in cargo-audit adapter under a new dependencies category, deterministic Go/Rust/mixed language detection and auto-selection, a SOP CLI inspection surface (sop security inspect --json) and auto mode (sop security scan --auto), a Semgrep language-coverage guard, packaging and documentation updates, and focused deterministic tests. No real scanner tools are installed; tests use fake executors and pinned fixtures.

## Capabilities

### security adapter contract and category model — EXISTS

- Location: internal/security/contract.go
- Owner: internal/security
- Evidence: internal/security/contract.go defines ProtocolVersion, Category (sast, secrets), Descriptor.Validate, Execution.Validate, Adapter interface.
- Gap: Category enum only accepts sast and secrets; a dependencies category does not exist yet.
- Resolution: Extend Category validation and descriptor/execution validation in a dedicated config/category stage.

### adapter builder and built-in adapters — PARTIAL

- Location: internal/security/adapter.go
- Owner: internal/security
- Evidence: internal/security/adapter.go BuildAdapter wires built-in ids semgrep and gitleaks and external protocol; unknown built-in ids return an error.
- Gap: No cargo-audit built-in adapter exists.
- Resolution: Add cargo-audit case in BuildAdapter plus a new adapter file implementing Adapter and provenanceProvider.

### bounded executor abstraction — EXISTS

- Location: internal/security/exec.go
- Owner: internal/security
- Evidence: Adapter structs take an Executor (a.exec.Run(ctx, root, argv, stdin, maxOutput)); exec.go holds execRunner and ErrUnavailable used by semgrep.go and gitleaks.go.
- Resolution: Reuse for cargo-audit with fake executors in tests.

### runner, resume, provenance reuse — EXISTS

- Location: internal/security/runner.go
- Owner: internal/security
- Evidence: internal/security/runner.go Runner.Scan, maybeReuse compares content, policy, configuration, rules identity and scanner version; provenanceProvider interface supports per-adapter provenance.
- Gap: MaybeReuse compares RulesIdentity and ScannerVersion only; advisory database identity is not part of resume validation.
- Resolution: Record advisory database identity in provenance (for example RulesIdentity or a dedicated field) so resume validation covers it; stage adds resume invalidation tests.

### policy evaluation and required-mode gating — EXISTS

- Location: internal/security/policy.go
- Owner: internal/security
- Evidence: internal/security/policy.go EvaluatePolicy handles enabled, mode, required_categories, fail_on, unknown_severity; required mode needs COMPLETED/COMPLETE per category.
- Gap: No stale-database deny policy; no explicit PARTIAL coverage diagnostic for allowed stale database.
- Resolution: Add stale database policy option to Policy and Identity; default deny, explicit allow yields PARTIAL coverage diagnostic.

### redaction and native report sanitization — EXISTS

- Location: internal/security
- Owner: internal/security
- Evidence: internal/security/normalize.go and gitleaks_view.go referenced by semgrep.go SanitizeNativeReport, SanitizeText, sanitizeExecution.
- Resolution: Apply same sanitization to cargo-audit output and diagnostics; keep secrets out of artifacts, logs, and model inputs.

### configuration model for security adapters and policy — EXISTS

- Location: internal/config
- Owner: internal/config
- Evidence: config.SecurityAdapterConfig used in internal/security/adapter.go with fields ID, Category, Protocol, Command, Rules, Version; policy fields mapped on security.Policy.
- Gap: No configuration keys for advisory database path, stale policy, or auto mode.
- Resolution: Extend configuration in a config/category stage and document as report-only.

### SOP security CLI surface — UNKNOWN

- Location: cmd/
- Owner: cmd (SOP CLI layer)
- Evidence: cmd/ exists; security package exposes Runner/policy APIs. The exact existing SOP security subcommand structure (inspect/scan flags, --json output, exit codes) was not resolved in discovery.
- Gap: Need to confirm existing subcommand wiring for sop security scan and whether inspect exists.
- Resolution: Discovery work inside the CLI stage objective: trace cmd/ wiring, then add inspect --json and scan --auto with explicit configuration authoritative.

### cargo-audit executable and local advisory database — MISSING

- Location: external (developer environment)
- Owner: external environment; adapter behavior owned by internal/security
- Evidence: No cargo-audit adapter or advisory database in the repository; constraint forbids installing tools.
- Gap: Real cargo-audit binary and advisory database are not present and must not be installed.
- Resolution: Implement cargo-audit adapter against a fake Executor and pinned fixtures; real-tool checks are gated and reported as not run. Not a stage prerequisite.

### Semgrep executable and Gitleaks executable — MISSING

- Location: external (developer environment)
- Owner: external environment; coverage behavior owned by internal/security
- Evidence: Adapters exist in internal/security/semgrep.go and gitleaks.go; constraint forbids installing the tools; semgrep adapter already returns StatusUnavailable when ErrUnavailable.
- Gap: Tools are not installed for verification runs.
- Resolution: Tests use fake executors; real-tool checks gated and reported as not run.

### language and manifest detection for Go/Rust/mixed — MISSING

- Location: internal/security (new detection code)
- Owner: internal/security
- Evidence: No detection code found in internal/security; adapter selection today is configuration only (BuildAdapters over cfgs).
- Gap: No go.mod/Cargo.toml/Cargo.lock/workspace detection or deterministic selection.
- Resolution: Add a detection/selection stage with unit tests for Go-only, Rust-only, and mixed repositories.

### canonical skills tree and skill command registration — PARTIAL

- Location: skills/, internal/skill, install.sh, install.ps1, .claude-plugin/
- Owner: internal/skill and packaging
- Evidence: skills/ directory exists; task requires addition to skills/, internal/skill.Commands, installer skill list, and generated Claude plugin mirror. Exact current registration points were not confirmed in discovery.
- Gap: sop-security-scan skill entry and registration do not exist.
- Resolution: Packaging stage inspects existing skill registration and mirror generation, then adds entries and drift checks.

### documentation set for security scanner — PARTIAL

- Location: docs/, .zed/
- Owner: docs
- Evidence: docs/ exists and the PRD names the security scanner spec, guide, CLI reference, and Zed skill guide as update targets; exact files were not opened in discovery.
- Gap: Dependency scanning, detection, auto mode, stale policy, network behavior, limitations, and extension instructions are not documented.
- Resolution: Documentation stage updates the named documents and passes link checks.

### issue #40 dependency-scanning tracking — EXISTS

- Location: issue tracker
- Owner: project maintainers
- Evidence: PRD states issue #40 covers overlapping dependency-scanning scope and must be referenced but not closed.
- Resolution: Reference only in documentation and release notes; do not close.

## Assumptions

### The existing internal/security adapter contract can carry a dependencies category without a second execution path.

- Evidence: Contract Validate rejects categories other than sast and secrets today, but the runner, policy, provenance, and resume code are category-generic.
- Consequence: The plan extends the category enum and validation rather than adding new execution code; required_gate semantics stay in EvaluatePolicy.

### Resume validation can cover advisory database identity through existing provenance fields.

- Evidence: runner.maybeReuse already compares Provenance.RulesIdentity and ScannerVersion.
- Consequence: The cargo-audit adapter records a deterministic advisory database identity in rules identity (or a new provenance field) so resume invalidates on identity change; tests assert invalidation.

### Auto mode is additive and explicit configuration stays authoritative.

- Evidence: BuildAdapters builds only configured adapters; nothing auto-selects.
- Consequence: Auto selection adds missing built-in categories only; configured categories are never replaced or removed.

### Real scanner tools are absent and must not be installed.

- Evidence: Task constraint plus no tool binaries in the repository.
- Consequence: All adapter tests use fake executors and pinned fixtures; real-tool checks are gated and reported as not run.

### Project security configuration remains report-only during this work.

- Evidence: Task constraint; Policy.Mode is a configuration value.
- Consequence: No stage changes mode to required, adds suppressions, owners, expiry, or risk acceptance.

### The SOP CLI already owns the security command entry point.

- Evidence: cmd/ exists; the PRD requests adding inspect and auto-mode flags to sop security.
- Consequence: The CLI stage first traces existing wiring, then adds subcommands and flags; missing CLI structure is resolved inside that stage, not as an external prerequisite.

## S1 — Security category and policy model extension

Extend internal/security contract and policy to support a dependencies category, stale advisory database policy, and explicit coverage diagnostics, without changing existing sast/secrets behavior.

### Dependencies

None

### Deliverables

- CategoryDependencies constant in internal/security/contract.go
- Descriptor.Validate and Execution.Validate accept dependencies
- Policy fields for stale database handling (deny default, explicit allow) included in Policy.Identity
- Configuration keys for cargo-audit advisory database path, stale policy, and auto mode in internal/config
- Unit tests for validation, policy identity, and deny/allow stale behavior

### Acceptance Criteria

- Unknown categories are still rejected; dependencies is accepted.
- Stale database defaults to deny and yields INDETERMINATE, never PASS.
- Explicit allow stale yields a PARTIAL coverage diagnostic in the evaluation reasons.
- Policy identity changes when stale policy or advisory database path changes.
- go test ./internal/security/... passes for the new model tests.

## S2 — Language, manifest, and lockfile detection

Implement deterministic repository-content detection for Go, Rust, Cargo workspaces, actual Cargo.lock locations, and available lockfiles, and expose capability status available/unavailable/unsupported/proposed.

### Dependencies

- S1

### Deliverables

- internal/security/detect.go with detection result types
- Deterministic selection function that maps detected content to candidate built-in adapters
- Capability status classifier for configured adapters
- Unit tests for Go-only, Rust-only, and mixed repositories
- Fixtures under internal/security/testdata for manifests and workspace layouts

### Acceptance Criteria

- Go-only, Rust-only, and mixed fixtures produce stable, identical results across repeated runs.
- Detection reports each actual Cargo.lock path, including Cargo workspace members.
- Missing lockfiles are reported explicitly instead of assumed present.
- A Rust-only repository never selects a Semgrep ruleset limited to Go source.
- go test ./internal/security/... passes for detection tests.

## S3 — Semgrep language-coverage guard

Guard Semgrep source analysis so Go-only rules never run against Rust source and unsupported Rust analysis is reported explicitly.

### Dependencies

- S2

### Deliverables

- Language guard in internal/security/semgrep.go or a helper it calls
- Explicit UNSUPPORTED or PARTIAL coverage diagnostic for unsupported language analysis
- Unit tests with fake executor for Go-only, Rust-only, and mixed content

### Acceptance Criteria

- A Rust-only repository never passes Go Semgrep rules; the run reports unsupported Rust analysis.
- Unsupported coverage cannot produce a PASS in required mode.
- No implicit rule download is introduced.
- go test ./internal/security/... passes for guard tests.

## S4 — cargo-audit adapter with advisory database identity

Add a built-in cargo-audit adapter in the dependencies category that inspects an explicit lockfile with --file, runs --no-fetch against an explicit local advisory database, parses structured JSON, and fails closed on missing lockfile, unavailable database, stale database under deny policy, malformed output, timeout, and tool failure.

### Dependencies

- S1
- S2

### Deliverables

- internal/security/cargoaudit.go implementing Adapter and provenanceProvider
- BuildAdapter wiring for id cargo-audit
- Finding normalization preserving advisory ID, package and version, severity when available, CVSS, aliases/CVEs, patched and untested versions, and references
- Advisory database identity (last-commit, last-updated, advisory count) recorded in provenance and used by resume validation
- Pinned JSON fixtures under internal/security/testdata for clean, seeded, malformed, timeout, stale, missing database, and missing lockfile cases

### Acceptance Criteria

- Adapter never generates or modifies a lockfile and never passes a fetch or database-update flag.
- Missing lockfile, unavailable database, stale database under deny, and failed execution produce a non-clean result and never PASS.
- Advisory identity change invalidates resume; unchanged identity reuses the completed result.
- Malformed JSON and timeout map to distinct diagnostics.
- Secrets and raw source are never written into artifacts or diagnostics.
- go test ./internal/security/... passes for cargo-audit tests using a fake executor.

## S5 — CLI inspection and auto-selection surface

Trace the existing cmd/ security command wiring, then add sop security inspect --json and sop security scan --auto that fill missing built-in categories from detected content while explicit configuration stays authoritative.

### Dependencies

- S2
- S3
- S4

### Deliverables

- Discovery note in the change record describing the existing security command wiring
- sop security inspect --json returning detection and capability status
- sop security scan --auto flag wired to the deterministic selection function
- CLI tests for inspector JSON shape and auto selection behavior

### Acceptance Criteria

- Inspect --json emits detected languages, lockfiles, and adapter capability statuses.
- Auto mode adds missing built-in categories only and never overrides explicit configuration.
- Without --auto, behavior is unchanged.
- CLI tests pass with a fake executor; real-tool checks are gated and reported as not run.

## S6 — sop-security-scan skill and packaging

Create the sop-security-scan skill that orchestrates configuration inspection, scanner selection, report-only execution through SOP, evidence review, and the final report, and register it consistently across packaging surfaces.

### Dependencies

- S5

### Deliverables

- skills/sop-security-scan/SKILL.md
- Entry in internal/skill.Commands
- Entry in the installer skill list (install.sh and install.ps1)
- Regenerated Claude plugin mirror under .claude-plugin/
- Packaging consistency test or check

### Acceptance Criteria

- Skill discovery, metadata, installer list, and plugin mirror contain the same skill with no drift.
- The skill runs from one request to the final report without manual continuation except at a real blocker or approval boundary.
- The skill keeps project configuration report-only and does not suppress findings, invent owners or expiry, accept risks, or launch fixes.
- Detected secrets stay out of skill artifacts, logs, diagnostics, and model inputs.

## S7 — Documentation updates

Update the security scanner spec, guide, CLI reference, and Zed skill guide for dependency scanning, detection, auto mode, stale policy, network behavior, limitations, and extension instructions, and reference issue #40 without closing it.

### Dependencies

- S5
- S6

### Deliverables

- Updated security scanner spec with dependencies category and cargo-audit contract
- Updated guide with setup, examples, and limitations
- Updated CLI reference for inspect --json and scan --auto
- Updated Zed skill guide entries
- Reference to issue #40 in dependency-scanning scope notes

### Acceptance Criteria

- Docs state network behavior: --no-fetch and explicit local advisory database, no implicit update or upload.
- Docs state stale database deny-by-default policy and the PARTIAL diagnostic for explicit allow.
- Docs include extension instructions for adding adapters and detection rules.
- Documentation link checks pass.
- Issue #40 is referenced and not closed.

## S8 — Full verification and packaging checks

Run the acceptance command set and fix any regression without widening scope.

### Dependencies

- S1
- S2
- S3
- S4
- S5
- S6
- S7

### Deliverables

- Verification record listing each command and result
- Note of gated real-tool checks reported as not run
- Focused test inventory covering Go-only, Rust-only, mixed, clean/seeded, unavailable, malformed, timeout, stale/missing database, missing lockfile, resume invalidation, and redaction

### Acceptance Criteria

- gofmt, go vet ./..., go build ./..., and go test ./... pass.
- Documentation link checks and packaging checks pass.
- No scanner tool is installed and no shell authorization or approval policy is changed.
- Unrelated changes and untracked state are preserved.
