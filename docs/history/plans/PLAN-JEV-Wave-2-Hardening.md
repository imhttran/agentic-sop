# PLAN — JEV Wave 2 Hardening

## Project

agentic-sop

## Summary

Make JEV a reliable, provider-independent advisory analysis component: the
production Ollama analyzer emits validated structured evidence, early-JEV and
routing consume it, confirmed reliability defects are fixed, provider
configuration is documented, and decision classification is delegated to the
existing decision-adapter contract rather than a new JEV decision framework.
Deterministic SOP policy retains final authority. Existing JEV analysis,
fail-closed behavior, bounded retries, and historical artifacts are preserved.

Approval gates (operator-owned; SOP must never cross them automatically):

- Gate 1 — register and activate this plan.
- Gate 2 — implementation of JEV-201, JEV-203, JEV-204 (and JEV-202's non-breaking configuration rejection).
- Gate 3 — the authoritative neutral decision contract (JEV-202's contract choice and any breaking/API change), required before JEV-206.
- Gate 4 — live-provider authorization before JEV-207/JEV-208 incur real inference.
- Gate 5 — final integration (enabling JEV by default or changing production policy).

Cross-repository note: JEV-206 validates adapters in `sop-decision-adapters`; its
result is recorded in `agentic-sop` as a report artifact verified here. JEV-207 and
JEV-208 require live inference and are Gate-4 gated. These steps are not run by
`agentic-sop`'s in-repo acceptance checks beyond the report-evidence checks below.

Evidence rule: a report artifact satisfies a task only when it records the required
runtime/model configuration, the exact test command, the repository revision, and the
individual per-target results. A report that merely contains the required headings is
insufficient and cannot produce VERIFIED.

## Tasks

### JEV-201 — Structured Evidence

- **Objective:** The production Ollama JEV analyzer emits versioned StructuredEvidence that passes the existing evidence contract, so early-JEV and routing consume it.
- **Scope:** Extend internal/jev/ollama.go to set Result.StructuredEvidence from validated JSON aligned with the evidence contract; validate via evidence.Validate before use; preserve current findings/summary; a malformed or invalid evidence payload MUST fail closed and never authorize continuation; connect valid evidence to early-JEV and router.SignalsFrom. Do not change the quality gate unless separately approved.
- **Files:** internal/jev/ollama.go, internal/jev/evidence.go, internal/jev/ollama_test.go
- **Depends on:** —
- **Acceptance:** the real analyzer populates Result.StructuredEvidence; evidence passes evidence.Validate; invalid evidence fails closed; early-JEV and routing derive signals from validated evidence; existing quality-gate behavior unchanged.
- **Acceptance check:** go test -count=1 -run TestOllamaAnalyzerEmitsStructuredEvidence ./internal/jev/
- **Acceptance check:** go test -count=1 -run TestStructuredEvidenceInvalidFailsClosed ./internal/jev/
- **Acceptance check:** go vet ./...

### JEV-202 — Decision Contract Alignment

- **Objective:** Select one authoritative neutral decision contract and reject unsupported decision.provider values at configuration validation; the contract choice is an approval-gated decision.
- **Scope:** Compare SEAM-002 with the decision-adapter contract and record an ADR (docs/architecture/JEV-DECISION-CONTRACT.md) with the compatibility mapping for choice/boolean/score/confidence/probability; make configuration validation reject unsupported decision.provider values (the recommended non-breaking default). Adopting a shared or breaking contract is Gate 3, not automated here. No new JEV decision adapter.
- **Files:** docs/architecture/JEV-DECISION-CONTRACT.md, internal/config/config.go, internal/config/decision_provider_test.go
- **Depends on:** —
- **Acceptance:** an ADR records the authority and versioning rules; unsupported decision.provider values are rejected at configuration validation; no duplicate provider-selection logic; no weakening of deterministic approval gates; no new JEV decision adapter.
- **Acceptance check:** go test -count=1 -run TestDecisionProviderRejectsUnsupported ./internal/config/
- **Acceptance check:** go vet ./...

### JEV-203 — Reliability Hardening

- **Objective:** Fix the confirmed reliability defects (cancellation misclassification, silent analyzer-factory failure, quoted/space changed-file paths) while preserving bounded retries and fail-closed policy.
- **Scope:** distinguish context.Canceled from context.DeadlineExceeded in internal/jev/ollama.go; surface analyzer-construction failures as an observable, secret-free diagnostic in internal/cli/jev.go; handle quoted and space-containing diff paths in changedFiles; reconcile docs/reference/JEV-TROUBLESHOOTING.md with implemented behavior; add regression tests.
- **Files:** internal/jev/ollama.go, internal/jev/ollama_test.go, internal/cli/jev.go, internal/cli/jev_lifecycle_test.go, docs/reference/JEV-TROUBLESHOOTING.md
- **Depends on:** —
- **Acceptance:** cancellation is not classified as timeout; factory failures are observable without secrets; quoted and space paths are handled; provider failures do not silently produce successful decisions; retry bounds unchanged; regression tests cover each fix.
- **Acceptance check:** go test -count=1 -run TestCanceledContextIsNotTimeout ./internal/jev/
- **Acceptance check:** go test -count=1 -run TestJEVAnalyzerFactoryFailureIsObservable ./internal/cli/
- **Acceptance check:** go test -count=1 -run TestChangedFilesHandlesQuotedPaths ./internal/cli/
- **Acceptance check:** go vet ./...

### JEV-204 — Provider Configuration

- **Objective:** Document and verify JEV's actual provider/model selection and its independence from SOP execution-model routing; record secret-free provider metadata.
- **Scope:** document SOP_OLLAMA_* selection and the JEV-versus-SOP-model distinction in docs/reference/JEV-OPERATIONS.md; add a test asserting JEV provider/model metadata carries no secret fields and never implies an unsupported provider; confirm Claude is not required. No new provider framework.
- **Files:** docs/reference/JEV-OPERATIONS.md, internal/jev/ollama_test.go
- **Depends on:** —
- **Acceptance:** the JEV provider and model can be identified unambiguously; Claude is not required; configuration does not silently imply unsupported providers; secrets are never recorded in diagnostics; existing Ollama behavior remains compatible.
- **Acceptance check:** go test -count=1 -run TestJEVProviderMetadataNoSecrets ./internal/jev/
- **Acceptance check:** go vet ./...

### JEV-205 — Integration Validation

- **Objective:** Exercise the real production analyzer (not an injected fake) through the CLI lifecycle, proving structured-evidence generation, fail-closed behavior, and early-JEV/routing integration.
- **Scope:** construct the default analyzer (or a stub ChatProvider) and drive it through the lifecycle in a new test under internal/cli; cover malformed output, cancellation, provider failure, and recovery; assert invalid evidence never authorizes continuation.
- **Files:** internal/cli/jev_production_integration_test.go
- **Depends on:** JEV-201, JEV-203, JEV-204
- **Acceptance:** production analyzer wiring is covered; invalid evidence never authorizes continuation; tests run without Claude; results are reproducible.
- **Acceptance check:** go test -count=1 -run TestJEVProductionAnalyzerIntegration ./internal/cli/
- **Acceptance check:** go vet ./...

### JEV-206 — Adapter Compatibility

- **Objective:** Validate the selected neutral decision contract against the Julia-1 and Nimble adapters and record compatibility with full execution evidence, independently of JEV-202's configuration change.
- **Scope:** Owning repository for the validation is sop-decision-adapters (contract tests exercising Julia-1 and Nimble: choice/boolean/score, confidence/probability validation, deterministic fallback, no approval bypass). The result is recorded in agentic-sop as docs/reports/JEV-WAVE2-ADAPTER-COMPAT.md, which MUST contain, for the Julia-1 and Nimble validation individually: the exact test command executed, the sop-decision-adapters repository revision (git SHA), the process exit status, and the per-adapter result. A report that merely contains the required headings is insufficient. Requires the Gate 3 contract choice. No JEV decision provider is added.
- **Files:** docs/reports/JEV-WAVE2-ADAPTER-COMPAT.md, internal/jev/adapter_compat_report_test.go
- **Depends on:** JEV-202
- **Acceptance:** the report records the exact test command, the repository revision, the exit status, and the individual Julia-1 and Nimble results; a missing or heading-only report cannot be VERIFIED; no adapter bypasses SOP approval rules; compatibility gaps are documented.
- **Acceptance check:** go test -count=1 -run TestJEVAdapterCompatReportHasEvidence ./internal/jev/

### JEV-207 — Runtime Dogfood

- **Objective:** Run JEV end-to-end in a disposable SOP project against the configured Ollama model and record complete, secret-free, execution-backed evidence.
- **Scope:** GATE 4 (live provider) required before running. Disposable project only. Record per scenario the exact command and argv, ordered RFC3339 start/finish timestamps, process exit status, repository revision (40-hex), provider, model, endpoint category, latency, structured-evidence and routing outcomes, and references to run artifacts under .agent-sdlc/runs/. Scenarios: direct SOP-to-JEV invocation, provider selection, structured evidence, early-JEV routing, retry, provider unavailability, human escalation, fail-closed. Each scenario is classified measured (with its execution evidence), simulated (explicitly identified as simulated and reproducible from a recorded fixture), or untested (with a stated reason; no invented result). The evidence keeps recorded process execution (command, argv, exit status, timestamps) distinct from the governed SOP outcome (task state and autonomy decision); neither is inferred from the other. Evidence is machine-readable (not headings): docs/reports/evidence/JEV-WAVE2-DOGFOOD.json alongside the human report. No credentials recorded.
- **Files:** docs/reports/JEV-WAVE2-DOGFOOD.md, docs/reports/evidence/JEV-WAVE2-DOGFOOD.json, internal/jev/dogfood_evidence_test.go
- **Depends on:** JEV-205, JEV-206
- **Acceptance:** a machine-readable evidence file records command provenance, ordered RFC3339 timestamps, per-scenario exit status, revision, provider, model, endpoint category, latency, structured-evidence and routing results, and the fail-closed and human-escalation scenarios; every measured scenario carries execution evidence, every simulated scenario is identified and reproducible, and every untested scenario states a reason with no invented result; recorded process execution is distinct from the governed SOP outcome; every evidence artifact reference resolves to an existing file at the recorded revision; no credentials appear; a missing, incomplete, or heading-only report cannot be VERIFIED.
- **Acceptance check:** go test -tags=jev207_acceptance -count=1 -run '^TestJEVDogfoodEvidenceHasExecutionProof$' ./internal/jev/
- **Acceptance check:** go test -tags=jev207_acceptance -count=1 -run '^TestJEVDogfoodEvidenceHasNoCredentials$' ./internal/jev/

### JEV-208 — Comparative Benchmark

- **Objective:** Compare deterministic-only, JEV-assisted, Julia-1, and Nimble decisions on shared, reproducible scenarios and record machine-readable metrics.
- **Scope:** Depends on JEV-206 and JEV-207. Record the exact benchmark inputs (shared scenario fixtures with stable identifiers) and the provenance command, then per configuration (deterministic-only, JEV-assisted, Julia-1, Nimble) record correct-decision rate, unsafe-continuation rate, unnecessary-escalation rate, retry precision, schema validity, repeatability, latency p50/p95, and inference cost. Every configuration and scenario is classified measured, simulated, or untested: a measured result requires numeric metrics backed by execution evidence (command, exit status, timestamps); a simulated result is explicitly identified and reproducible from a recorded fixture; an untested configuration records a reason and no invented metrics. The evidence keeps recorded process execution distinct from the governed SOP outcome. A measured configuration declares its sample_size: at sample_size 1 only single-observation metrics are allowed (no percentiles or rates), at sample_size 2 or more the applicable multi-sample metrics are required. Latency is reported in microseconds from a monotonic clock. Every evidence artifact reference resolves to an existing file at the recorded revision. Evaluate JEV as an analysis component separately from adapter classification. No credentials recorded.
- **Files:** docs/reports/JEV-WAVE2-BENCHMARK.md, docs/reports/evidence/JEV-WAVE2-BENCHMARK.json, internal/jev/benchmark_evidence_test.go, internal/jev/benchmark_evidence_acceptance_test.go, internal/decision/benchmark_fixtures_test.go
- **Depends on:** JEV-206, JEV-207
- **Acceptance:** a machine-readable evidence file records the provenance command and revision, the shared scenario fixtures with stable identifiers, all four configurations with the applicable listed metrics, and an explicit measured/simulated/untested classification per configuration and scenario; a measured configuration declares its sample_size — at sample_size 1 only single-observation metrics are allowed (no fabricated percentiles or rates), at sample_size 2 or more the applicable multi-sample metrics and execution provenance are required, and latency is reported in microseconds from a monotonic clock; every simulated result is identified as simulated and reproducible from a recorded fixture; every untested configuration states a reason and carries no numeric performance metrics; recorded process execution is distinct from the governed SOP outcome; every evidence artifact reference resolves to an existing file; no credentials appear; a missing, incomplete, or heading-only report cannot be VERIFIED.
- **Acceptance check:** go test -tags=jev208_acceptance -count=1 -run '^TestJEVBenchmarkEvidenceHasRequiredFields$' ./internal/jev/
- **Acceptance check:** go test -tags=jev208_acceptance -count=1 -run '^TestJEVBenchmarkEvidenceHasNoCredentials$' ./internal/jev/
