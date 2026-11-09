# OWASP LLM program — evidence matrix

This index links the ten security-plan workstreams. It is an evidence matrix,
not a compliance certification. A row is never marked fully covered from partial
evidence: `IMPLEMENTED`, `ENABLED`, `VERIFIED`, `NOT_RUN`, `GAP`, and
`NOT_APPLICABLE` are separate dimensions, and a missing live test stays visible.

Baseline: `imhttran/agentic-sop` `main` at
The Plan 10 row was refreshed on the branch merged with `main` at
`9689de7` (2026-11-08); earlier rows are not re-verified by that refresh.
`0750e8be0cf5f318a173abb34c2b752ac131521c` (refreshed 2026-11-09 UTC).

| Plan | Category | Workstream | Report / evidence | Evidence state | Boundary |
| --- | --- | --- | --- | --- | --- |
| Plan 01 | Excessive Agency | Linux agent (`task/LLM03-impl-plan-01-agency-boundary-controls`, PR #271) | [llm03-agency.md](llm03-agency.md), [agency-002-evidence.md](agency-002-evidence.md) | `IMPLEMENTED`; Linux local verification `PASS` (`go build`, `go vet`, `go test ./...`); Linux Bubblewrap runtime isolation verified; macOS Seatbelt `UNVERIFIED` | Normal agent execution is policy-only and not OS-sandboxed. OS isolation is demonstrated only on the isolated-replay and evaltarget paths; platform identity and macOS runtime evidence remain gaps. |
| Plan 02 | Prompt Injection | Linux agent | No report in this checkout; existing deterministic controls: `docs/reports/security-shift-left/aisec-live-assessment.md` | `NOT_RUN` for live adversarial; deterministic/real-control tests exist | Live-adversarial evidence needs an evaltarget-routed live path; a fixture or text assertion is not a substitute. |
| Plan 03 | Sensitive Information Disclosure | Mac agent (`security/llm02-disclosure`, merged in PR #266) | [llm02-disclosure.md](llm02-disclosure.md) | Static tracing `IMPLEMENTED` / `VERIFIED`; live extraction `NOT_RUN` | Plan 03 live tests depend on Plan 02's merged live-test infrastructure. Static tracing is independent. |
| Plan 04 | Supply Chain | Mac agent (`security/llm04-supply-chain`) | [llm04-supply-chain.md](llm04-supply-chain.md) | `IMPLEMENTED` / `VERIFIED` for the declared report-only scope | Provider model provenance, publisher signatures, required-mode enablement, and normal-agent OS isolation remain out of scope or unverified. |
| Plan 05 | Data and Model Poisoning | `security/llm05-data-and-model-poisoning` (merged in PR #268; follow-up fixes merged in PR #274) | Plan: `docs/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md`; report `llm05-poisoning.md` not yet written (POISON-001) | POISON-002 outcome-provenance fix `IMPLEMENTED` (CI and review clean); SOP acceptance `PARTIAL`, blocked on issue #273; POISON-001/003 `NOT_RUN` | Provider-owned training is outside SOP control and is not marked secure. |
| Plan 06 | Unbounded Consumption | Not started | Plan: `docs/plans/PLAN-06-SOP-LLM06-Unbounded-Consumption.md` | `NOT_RUN` (plan `PROPOSED`) | Token/spend accounting that does not exist stays a `GAP`; no dollar-cap claim without pre-dispatch enforcement. |
| Plan 07 | Misinformation | Not started | Plan: `docs/plans/PLAN-07-SOP-LLM07-Misinformation.md` | `NOT_RUN` (plan `PROPOSED`) | Scope is evidence used in decisions, not factual correctness of every generated sentence. |
| Plan 08 | Hidden Context Exposure | Not started | Plan: `docs/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md` | `NOT_RUN` (plan `PROPOSED`) | Reuses Plan 03 redaction and Plan 02 live evaluation; adds no second DLP layer. |
| Plan 09 | Vector and Embedding Weaknesses | Not started | Plan: `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md` | `NOT_RUN` (plan `PROPOSED`) | Plan 05 owns ingestion provenance; this plan owns query scope, cache scope, and stale results. |
| Plan 10 | Improper Output Handling (LLM10) | Linux agent (`security/llm10-improper-output-handling`) | [llm10-output-handling.md](llm10-output-handling.md) | `IMPLEMENTED` / `ENABLED` / `VERIFIED` for the Linux CLI terminal and the Slack payload encoder; live delivery `NOT_RUN` | Third-party terminal/IDE rendering, persisted Markdown rendering, and Slack auto-unfurl remain external. Shell, path, database, and protocol controls were inspected and confirmed, not re-authored. |

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

## What the Plan 10 row evidences

- Model, tool, provider, and configured-command output is inventoried against
  every SOP-owned sink: tool commands, file paths, SQL, HTML, the CLI terminal,
  notifications, MCP/ACP, and configured outbound HTTP.
- The model-facing `run_command` tool and acceptance checks use structured
  argument vectors with no shell; `SplitCommand` rejects shell metacharacters and
  `commandpolicy` must classify the command `SAFE`.
- File tools retain the symlink-resolved workspace boundary and protect
  `.agent-sdlc/state.db`; arguments are validated before any write.
- SQL uses bound placeholders; the controller uses `html/template` auto-escaping
  with no trusted-markup escape hatches; MCP/ACP use JSON encoders.
- Two display-sink gaps were reproduced and fixed: terminal control sequences in
  CLI output, and Slack mrkdwn mention/link injection in model-derived event text.
- External rendering and live Slack delivery are recorded as explicit limitations,
  not inferred protections.

## What this matrix does not claim

- It does not claim full OWASP Top 10 for LLM Applications coverage.
- Category IDs and names follow the OWASP Top 10 for LLM Applications 2026
  edition (published 2026-08-04), as listed in the Cycode guide
  (https://cycode.com/blog/owasp-top-10-llm-applications/). This is not a
  compliance mapping; the official document's framework appendix has not been
  checked against these rows.
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
