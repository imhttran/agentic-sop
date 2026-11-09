# OWASP LLM program — evidence matrix

This index links the four security-plan workstreams. It is an evidence matrix,
not a compliance certification. A row is never marked fully covered from partial
evidence: `IMPLEMENTED`, `ENABLED`, `VERIFIED`, `NOT_RUN`, `GAP`, and
`NOT_APPLICABLE` are separate dimensions, and a missing live test stays visible.

Baseline: `imhttran/agentic-sop` `main` at
`0750e8be0cf5f318a173abb34c2b752ac131521c` (refreshed 2026-11-09 UTC).

| Plan | Category | Workstream | Report / evidence | Evidence state | Boundary |
| --- | --- | --- | --- | --- | --- |
| Plan 01 | Excessive Agency | Mac agent (`security/llm03-agency`); Linux transfer PR #269 | [llm03-agency.md](llm03-agency.md) | `IMPLEMENTED` inventory refreshed by inspection; `VERIFIED` for the AGENCY-003 Mac run of `go build ./...`, `go vet ./...`, `go test ./...` at base `aa2321754d90bc67ee5eced332295089c48de841` / head `32982315dc6b9a96f40f76c5188b33b10fc85a49`; macOS evaltarget isolation presence `NOT_RUN`; PR [#270](https://github.com/imhttran/agentic-sop/pull/270) (draft; not merged) | Normal agent execution remains policy-only and is not sandboxed; only `internal/evaltarget` carries OS isolation. The Bubblewrap result is historical Linux evidence at `0750e8be`; the historical Linux full suite was incomplete by timeout and is not a pass. |
| Plan 02 | Prompt Injection | Linux agent | No report in this checkout; existing deterministic controls: `docs/reports/security-shift-left/aisec-live-assessment.md` | `NOT_RUN` for live adversarial; deterministic/real-control tests exist | Live-adversarial evidence needs an evaltarget-routed live path; a fixture or text assertion is not a substitute. |
| Plan 03 | Sensitive Information Disclosure | Mac agent (`security/llm02-disclosure`) | Plan: `docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md`; report owned by that workstream | Static tracing `IN PROGRESS`; live `NOT_RUN` | Plan 03 live tests depend on Plan 02's merged live-test infrastructure. Static tracing is independent. |
| Plan 04 | Supply Chain | Mac agent (`security/llm04-supply-chain`) | [llm04-supply-chain.md](llm04-supply-chain.md) | `IMPLEMENTED` / `VERIFIED` for the declared report-only scope | Provider model provenance, publisher signatures, required-mode enablement, and normal-agent OS isolation remain out of scope or unverified. |

## What the Plan 04 row evidences

- The SOP-owned trust chain is inventoried: build, Go modules, scanner
  executables, rule/database artifacts, policy/baseline, provider selection, and
  agent harness boundaries.
- The configured report-only scan completed with `PASS` / coverage `COMPLETE`
  for the declared SAST, secrets, and dependency scope.
- Existing version, stale-database, required-mode, and dependency-coverage checks
  are mapped to named tests and observed results.
- One reproduced gap was fixed: OSV freshness now uses the recorded acquisition
  time when present, and corrupt acquisition metadata fails closed.
- Residual risks are recorded rather than suppressed: descriptive Semgrep/Gitleaks
  pins, runtime binary hash re-checking, provider model provenance, and required
  mode.

## What this matrix does not claim

- It does not claim full OWASP Top 10 for LLM Applications coverage.
- It does not claim a formal OWASP edition mapping; confirm the official edition
  before publishing a compliance claim.
- It does not infer Linux protection from macOS Seatbelt results.
- It does not infer runtime protection from a specification, scanner `PASS`, or
  dependency plan alone.
- It does not claim a merge, deployment, installation, or live adversarial run.

## Related baseline evidence

- [../security-hardening/final.md](../security-hardening/final.md) — declared
  report-only scan scope and residual findings.
- [../security-hardening/SEC-H-004-dependency-coverage-readiness.md](../security-hardening/SEC-H-004-dependency-coverage-readiness.md)
  — dependency fail-closed behavior and provisioning record.
- [../security-shift-left/execution-path-matrix.md](../security-shift-left/execution-path-matrix.md)
  — which execution paths SOP does and does not sandbox.
- [../security-shift-left/aisec-live-assessment.md](../security-shift-left/aisec-live-assessment.md)
  — why live-adversarial evidence is `NOT_RUN`.
- [../security-shift-left/seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md)
  — macOS Seatbelt evidence for isolated replay and evaltarget only.
