# OWASP LLM program — evidence matrix

This index links the security-plan workstreams that have a plan or report in this
checkout. It is an evidence matrix, not a compliance certification. A row is never
marked fully covered from partial evidence: `IMPLEMENTED`, `ENABLED`, `VERIFIED`,
`NOT_RUN`, `GAP`, and `NOT_APPLICABLE` are separate dimensions, and a missing live
test stays visible.

Baseline: `imhttran/agentic-sop` `main` at
`0750e8be0cf5f318a173abb34c2b752ac131521c` (refreshed 2026-11-09 UTC). The
Plan 10 row and the Plan 03 row were refreshed against `main` at
`24be0687b0ac0dec5e2e69584be67f1795c0f6f1` (2026-11-08). Older rows are not
re-verified by that refresh.

| Plan | Category | Workstream | Report / evidence | Evidence state | Boundary |
| --- | --- | --- | --- | --- | --- |
| Plan 01 | Excessive Agency (LLM03) | Linux agent (Mac workstream not started) | Source and implementation branch exist outside this checkout (`security/llm03-agency`, `task/LLM03-impl-plan-01-agency-boundary-controls`); not merged to this baseline | `NOT_RUN` in this checkout | Linux-owned. No Mac result is substituted. The Plan 01 handoff was not available. |
| Plan 02 | Prompt Injection (LLM01) | Linux agent | Existing deterministic controls: `docs/reports/security-shift-left/aisec-live-assessment.md`; partial seam on `security/llm01-injection` (not merged) | `NOT_RUN` for live adversarial; deterministic/real-control tests exist | Live-adversarial evidence needs an evaltarget-routed live path; a fixture or text assertion is not a substitute. |
| Plan 03 | Sensitive Information Disclosure (LLM02) | Mac agent (`security/llm02-disclosure`, merged) | [llm02-disclosure.md](llm02-disclosure.md) | Static tracing `IMPLEMENTED` / `VERIFIED` (macOS); live `NOT_RUN` | Live extraction depends on Plan 02's merged live-test infrastructure, which is absent. The report's historical outcomes remain `PARTIAL` where the acceptance mapping changed after execution. |
| Plan 04 | Supply Chain (LLM04) | Mac agent (`security/llm04-supply-chain`) | [llm04-supply-chain.md](llm04-supply-chain.md) | `IMPLEMENTED` / `VERIFIED` for the declared report-only scope | Provider model provenance, publisher signatures, required-mode enablement, and normal-agent OS isolation remain out of scope or unverified. |
| Plan 05 | Data and Model Poisoning (LLM05) | Linux agent (`security/llm05-data-and-model-poisoning`, merged) | Plan: `docs/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md`; no category report in this checkout | Implementation merged; category evidence `NOT_RUN` | The merged change hardens outcome/memory provenance at ingest. It is not re-verified here and does not claim poisoning detection or retrieval isolation. |
| Plan 06 | not recorded in this checkout | — | No plan or report present | `NOT_RUN` | No category claim is made. |
| Plan 07 | not recorded in this checkout | — | No plan or report present | `NOT_RUN` | Plan 10 was authored assuming Plan 07 owns evidence claims, but that work is absent from this baseline. |
| Plan 08 | not recorded in this checkout | — | No plan or report present | `NOT_RUN` | No category claim is made. |
| Plan 09 | not recorded in this checkout | — | No plan or report present | `NOT_RUN` | Plan 10's source document names Plan 09 as retrieval-query isolation, but that work is absent from this baseline. |
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
- It does not claim a formal OWASP edition mapping; confirm the official edition
  before publishing a compliance claim.
- It does not infer Linux protection from macOS Seatbelt results.
- It does not infer runtime protection from a specification, scanner `PASS`, or
  dependency plan alone.
- It does not infer a category is covered because a plan, branch, or PR exists.
- It does not claim a merge, deployment, installation, or live adversarial run.
- It does not invent category names for Plan 06–09 where the source document and
  repository do not record them.

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
