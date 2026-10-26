# PLAN — Post-Wave-3 Production Readiness

**Status:** PROPOSED — awaiting review and Gate G-1. Not registered or activated.

## Project

agentic-sop

## Summary

Close the remaining production-readiness gaps after Wave 3 (opt-in adapter decision
provider). This plan is **offline-first**: only RD-01, RD-03, and RD-04 touch a live
provider, and each is gated. Enabling the adapter by default stays **CLOSED** (Gate
W3-3), and deterministic SOP policy retains final authority.

Approval gates (operator-owned; SOP must never cross them automatically):

- Gate G-1 — register and activate this plan.
- Gate G-2 — implement the offline tasks (RD-02, RD-05, and the offline halves).
- Gate L — live inference authorization for RD-01, RD-03, RD-04 (equivalent to the
  Wave 3 Gate W3-4; one gate authorizes the bounded runs described below).
- Gate W3-3 — **remains closed**: no default adapter activation.

Evidence rule: every task is satisfied by native deterministic acceptance checks
that exercise production code, plus a machine-readable, credential-free evidence
artifact recording command provenance, the repository revision, and per-case
results where a report is the deliverable.

## Tasks

### RD-01 — Full Governed SOP Run Adapter Dogfood

- **Objective:** Exercise the full governed lifecycle (`sop run`) with the adapter provider enabled, proving that adapter evidence flows through SEAM-004 into deterministic governance.
- **Scope:** In a disposable project with `decision.provider: adapter`, run one bounded governed task and record the governed outcome, JEV/adapter evidence, and routing. Live inference requires Gate L. No default activation.
- **Files:** internal/cli/adapter_sop_run_evidence_test.go, docs/reports/evidence/JEV-WAVE4-SOP-RUN-DOGFOOD.json
- **Depends on:** —
- **Acceptance:** a governed run records provider/model/revision, adapter evidence, the governed outcome and approval requirement, with no credentials; a missing or incomplete artifact cannot be VERIFIED.
- **Acceptance check:** go test -count=1 -run TestAdapterSOPRunIntegrationHasEvidence ./internal/cli/
- **Failure / rollback:** live inference is never automatic; on provider failure the run fails closed and the deterministic default stands.

### RD-02 — Windows Process-Tree Termination

- **Objective:** Bound the adapter subprocess tree on Windows (no orphaned descendants), matching the Unix process-group behavior.
- **Scope:** Replace the Windows `killProcessTree` limitation with a Job-Object-based (or `taskkill /T`) strategy; add a Windows-only deterministic test that proves a forked descendant does not survive.
- **Files:** internal/decision/commandprofile/procattr_windows.go, internal/decision/commandprofile/procattr_windows_test.go
- **Depends on:** —
- **Acceptance:** on Windows, cancelling or timing out the adapter terminates its descendants, verified by **actual execution** of the Windows test in the CI windows job (not merely a cross-compile); the Unix path is unchanged; `GOOS=windows` builds.
- **Acceptance check:** the CI windows job executes `go test -count=1 ./internal/decision/commandprofile/` — which includes both `TestAdapterConnectorKillsProcessGroupWindows` and `TestAdapterJobHandleNoLeak` — and it passes.
- **Failure / rollback:** a platform regression is caught by the Windows CI job.

### RD-03 — Julia Runtime and Live Verification

- **Objective:** Provision the Julia ONNX runtime and verify a bounded live Julia decision through the connector.
- **Scope:** Provision an ONNX model + tokenizer (`JULIA_MODEL_PATH`) and `onnxruntime`; run a bounded live Julia dogfood (Gate L) and record the observation. Distinguish local ONNX inference from Ollama proxy counts.
- **Files:** docs/reports/evidence/JEV-WAVE4-JULIA-DOGFOOD.json, internal/cli/adapter_julia_dogfood_test.go
- **Depends on:** —
- **Acceptance:** a live Julia decision reaches deterministic governance and is recorded credential-free; Julia is no longer UNTESTED; local inference is distinguished from any proxy counts.
- **Acceptance check:** go test -count=1 -run TestJEVWave4JuliaDogfoodHasEvidence ./internal/cli/
- **Failure / rollback:** if the runtime is unavailable, Julia remains UNTESTED and the plan continues; no fabrication.

### RD-04 — Deterministic-versus-Adapter Quality Evaluation

- **Objective:** Compare deterministic routing with adapter-assisted decisions on shared scenarios.
- **Scope:** Run the shared scenario fixtures through both the deterministic provider and the adapter (Gate L); record correct-decision rate, unsafe-continuation rate, unnecessary-escalation rate, schema validity, repeatability, and latency, with an explicit measured/simulated/untested classification and sample sizes (no percentiles from small samples).
- **Files:** docs/reports/evidence/JEV-WAVE4-QUALITY.json, internal/decision/quality_benchmark_test.go
- **Depends on:** RD-01
- **Acceptance:** the artifact records both configurations on the shared scenarios with a measured/simulated/untested classification; small samples are reported as N, never as p50/p95; no credentials.
- **Acceptance check:** go test -count=1 -run TestAdapterQualityBenchmarkHasRequiredFields ./internal/decision/
- **Failure / rollback:** an incomplete run stays untested; the deterministic baseline is unaffected.

### RD-05 — Rollback and Observability

- **Objective:** Document and test production rollback and adapter observability.
- **Scope:** Add a rollback runbook (disable `decision.enabled`/remove the adapter config → strict no-op) and record adapter observability (invocations, latency, failure class, fallback) without secrets.
- **Files:** docs/reference/JEV-OPERATIONS.md, internal/cli/adapter_observability_test.go
- **Depends on:** —
- **Acceptance:** the runbook restores the deterministic baseline exactly; observability records class-only adapter metadata with no credentials; no behavior change when disabled.
- **Acceptance check:** go test -count=1 -run TestAdapterObservabilityMetricsExist ./internal/cli/
- **Failure / rollback:** documentation-only plus a class-only metric; harmless if reverted.

## Corrections

- **RD-02 acceptance (recorded before activation):** the Windows process-tree requirement is met only by **actual execution** of the Windows test in the CI windows job — a cross-compile (`GOOS=windows go build`) is **not** sufficient. The Windows CI job must run `go test ./internal/decision/commandprofile/`.
- **RD-02 acceptance check (reconciled 2026-10-12):** the original registered acceptance check named the literal command `go test -count=1 -run TestAdapterConnectorKillsProcessGroupWindows ./internal/decision/commandprofile/`. The CI windows job executes the whole `internal/decision/commandprofile` package (`.github/workflows/ci.yml`, step "adapter connector + process-tree tests (Windows)": `go test -count=1 -v ./internal/decision/commandprofile/`), which includes that test and `TestAdapterJobHandleNoLeak`. The acceptance check was reconciled to that package-level run. The literal original wording is preserved here, unmodified, for provenance.

## Inference limits and attribution

- **Limits:** ≤3 live attempts per provider (RD-01/RD-03/RD-04), 120 s per attempt, ≤20 min overall, process-group cleanup on timeout; the counting proxy caps and attributes Ollama inference.
- **Attribution:** each artifact records the exact command, revision, provider/model, per-provider attempt counts, observed inference counts, and latency boundaries. Julia local ONNX inference is distinguished from Ollama proxy counts.
- **No fabrication:** no percentiles, success rates, or speedups from small samples; failed/incomplete attempts are recorded as such.

## Parallel workstreams

- **Offline, independent now (Gate G-2):** RD-02 (Windows) and RD-05 (docs/observability) — disjoint files.
- **Live (Gate L):** RD-01 and RD-03 may run independently once Gate L is granted; **RD-04 depends on RD-01**.
- **Serialize:** SOP-state writes (one writer); edits under `internal/decision/commandprofile` are serialized.

## Rollback

Setting `decision.enabled: false` (or removing the adapter configuration) restores
the deterministic baseline exactly — a strict no-op, not an error. No schema
migration is introduced, so rollback is instantaneous.

## References

- docs/architecture/JEV-DECISION-CONTRACT.md (Gate 3, ratified)
- docs/reference/CONFIGURATION.md (adapter provider)
- docs/plans/PLAN-JEV-Wave-3-Adapter-Integration.md (Wave 3)
- docs/reports/evidence/JEV-WAVE3-ADAPTER-DOGFOOD.json (Wave 3 Nimble dogfood)
