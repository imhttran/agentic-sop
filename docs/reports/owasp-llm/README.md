# OWASP LLM program — evidence matrix

This index links the ten security-plan workstreams. It is an evidence matrix,
not a compliance certification. A row is never marked fully covered from partial
evidence: `IMPLEMENTED`, `ENABLED`, `VERIFIED`, `NOT_RUN`, `GAP`, and
`NOT_APPLICABLE` are separate dimensions, and a missing live test stays visible.

Baseline: `imhttran/agentic-sop` `main` at
`0750e8be0cf5f318a173abb34c2b752ac131521c` (refreshed 2026-11-09 UTC). The Plan 10
row was refreshed on the branch merged with `main` at `9689de7` (2026-11-08). The
Plan 02, Plan 04 and Plan 05 rows were refreshed at `87d68ad9` (2026-11-09). Rows not
named here are not re-verified by those refreshes.

| Plan    | Category                         | Workstream                                                                                   | Report / evidence                                                                                          | Evidence state                                                                                                                                                                                                                                                                                                                          | Boundary                                                                                                                                                                                                                                                                                                                                                |
| ------- | -------------------------------- | -------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Plan 01 | Excessive Agency                 | Linux agent (`task/LLM03-impl-plan-01-agency-boundary-controls`, PR #271)                    | [llm03-agency.md](llm03-agency.md), [agency-002-evidence.md](agency-002-evidence.md)                       | `IMPLEMENTED`; Linux local verification `PASS` (`go build`, `go vet`, `go test ./...`); Linux Bubblewrap runtime isolation verified; macOS Seatbelt `UNVERIFIED`                                                                                                                                                                        | Normal agent execution is policy-only and not OS-sandboxed. OS isolation is demonstrated only on the isolated-replay and evaltarget paths; platform identity and macOS runtime evidence remain gaps.                                                                                                                                                    |
| Plan 02 | Prompt Injection                 | `security/llm01-injection` (merged in PR #272)                                               | [llm01-injection.md](llm01-injection.md), [llm01-campaign.json](llm01-campaign.json)                       | Deterministic and real-control tests `VERIFIED`; live-adversarial `VERIFIED` for one bounded campaign: 4 of 4 injection cases blocked and the positive control allowed, one run each                                                                                                                                                    | The live campaign used one local model (`qwen3.5:4b` through Ollama) on macOS inside an opt-in `sandbox-exec` profile. It is evidence for that model and platform, not for other models, providers, or Linux.                                                                                                                                           |
| Plan 03 | Sensitive Information Disclosure | Mac agent (`security/llm02-disclosure`, merged in PR #266)                                   | [llm02-disclosure.md](llm02-disclosure.md)                                                                 | Static tracing `IMPLEMENTED` / `VERIFIED`; live extraction `NOT_RUN`                                                                                                                                                                                                                                                                    | Plan 03 live tests depend on Plan 02's merged live-test infrastructure. Static tracing is independent.                                                                                                                                                                                                                                                  |
| Plan 04 | Supply Chain                     | Mac agent (`security/llm04-supply-chain`)                                                    | [llm04-supply-chain.md](llm04-supply-chain.md)                                                             | `IMPLEMENTED` / `VERIFIED` for the declared report-only scope; since PR #282 the Gitleaks archives are SHA-256-pinned in the repository and CI re-checks every cached scanner download against its pin                                                                                                                                  | Provider model provenance, publisher signatures, required-mode enablement, and normal-agent OS isolation remain out of scope or unverified.                                                                                                                                                                                                             |
| Plan 05 | Data and Model Poisoning         | `security/llm05-data-and-model-poisoning` (PRs #268, #274); `security/llm05-finish` (PRs #288, #292) | Plan: `docs/history/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md`; [llm05-poisoning.md](llm05-poisoning.md) | POISON-002 `VERIFIED` through SOP (focused provenance tests); POISON-001 trace accepted by the designated approver; POISON-003 recovery evidence: index and outcome reindex `VERIFIED`, decision-memory removal `VERIFIED` (GAP-7 fixed, PR #292)                                                                                                                 | Integrity proves bytes, not benign content. Open: GAP-4 (no per-document index digest), GAP-5 (server-reported RAG hash not checked). Fixed in PR #292: GAP-6 (unreadable `decisions.json` silently replaced), GAP-7 (no decision-memory removal command). Provider-owned training is outside SOP control and is not marked secure; query isolation is handed to Plan 09. |
| Plan 06 | Unbounded Consumption | Mac agent (`security/llm06-unbounded-consumption`); follow-up `security/llm06-reconcile` | [llm06-consumption.md](llm06-consumption.md) | `IMPLEMENTED` / `VERIFIED` for read-boundary output/read bounds, validation output bounds, unknown-token reporting, atomic continuation accounting, command-agent timeout/output, MCP frame bounds, and process-group cancellation on macOS and Linux CI; provider output-token cap defaults to 16384 tokens per request (`SOP_AGENT_MAX_OUTPUT_TOKENS` overrides), and monetary accounting is `GAP` | Token/spend accounting that does not exist stays a `GAP`; no dollar-cap claim without pre-dispatch enforcement. |
| Plan 07 | Misinformation                   | `security/llm07-misinformation` (merged PR #285); follow-up `security/llm07-content-binding` | [llm07-misinformation.md](llm07-misinformation.md); Plan: `docs/plans/PLAN-07-SOP-LLM07-Misinformation.md` | `IMPLEMENTED` / `ENABLED` / `VERIFIED` (deterministic, macOS arm64); SOP `COMPLETE`; live-model and Linux `NOT_RUN`                                                                                                                                                                                                                     | Scope is evidence used in decisions, not factual correctness. Manual acceptance evidence is now strictly bound to the reviewed repository content (D4 option 1); `run_command` subprocesses are outside the file-write boundary.                                                                                                                        |
| Plan 08 | Hidden Context Exposure          | Mac agent (`security/llm08-hidden-context-exposure`, merged in PR #286)                      | [llm08-hidden-context.md](llm08-hidden-context.md), [llm08-campaign.json](llm08-campaign.json); plan: `docs/history/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md` | Behavioral controls `IMPLEMENTED` / `VERIFIED`; manual criteria accepted at merge (no per-criterion SOP record); bounded live extraction `OBSERVED`                                                                                                                                                                                     | Prompt secrecy is not claimed; redaction is not general DLP; live evidence is one model/platform.                                                                                                                                                                                                                                                       |
| Plan 09 | Vector and Embedding Weaknesses  | Not started                                                                                  | Plan: `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`                                    | `NOT_RUN` (plan `PROPOSED`)                                                                                                                                                                                                                                                                                                             | Plan 05 owns ingestion provenance; this plan owns query scope, cache scope, and stale results.                                                                                                                                                                                                                                                          |
| Plan 10 | Improper Output Handling (LLM10) | Linux agent (`security/llm10-improper-output-handling`)                                      | [llm10-output-handling.md](llm10-output-handling.md)                                                       | `IMPLEMENTED` / `ENABLED` / `VERIFIED` for the Linux CLI terminal and the Slack payload encoder; live delivery `NOT_RUN`                                                                                                                                                                                                                | Third-party terminal/IDE rendering, persisted Markdown rendering, and Slack auto-unfurl remain external. Shell, path, database, and protocol controls were inspected and confirmed, not re-authored.                                                                                                                                                    |

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
  mode. Since PR #282 the Gitleaks archives are SHA-256-pinned and CI re-checks
  cached scanner downloads; long-lived local tools directories are still only
  version-checked.

## What the Plan 06 row evidences

- The execution limits are inventoried by unit, owner, default, enforcement
  point, persistence, and overshoot: invocation turns/tool calls, command,
  provider, and validation timeouts, output bytes, ACP frames/prompts, MCP frames,
  attempts, continuations, fix cycles, orchestration counts, and the evaluation
  harnesses.
- Read-boundary defects were reproduced and fixed: command output was buffered in
  full before truncation, `read_file` read the whole file before truncating, the
  command agent retained unbounded stdout/stderr, MCP frames had no bound, and
  validation output was unbounded at both capture and prompt rendering.
- Missing provider token usage is now reported as `unknown`/unreported rather
  than a zero-token call; measured zero stays distinguishable.
- Continuation accounting is atomic under the project lock, so concurrent runs
  cannot spend the same continuation; the persisted count still survives resume.
- Every provider request carries an output-token cap before dispatch: 16384 by
  default, overridden by `SOP_AGENT_MAX_OUTPUT_TOKENS`. Truncation at the cap is
  not detected, and no execution token budget or monetary accounting exists.
- Tool command timeout and cancellation kill the command's process group on macOS
  and Linux CI. `dqharness` token counts are deterministic estimates, not provider
  usage. The internal snapshot undo path is outside the model-output criterion and
  remains a documented restore-policy risk.

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
