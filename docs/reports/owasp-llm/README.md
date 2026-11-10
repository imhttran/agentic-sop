# OWASP LLM program — evidence matrix

This index links the ten security-plan workstreams. It is an evidence matrix,
not a compliance certification. A row is never marked fully covered from partial
evidence: `IMPLEMENTED`, `ENABLED`, `VERIFIED`, `NOT_RUN`, `GAP`, and
`NOT_APPLICABLE` are separate dimensions, and a missing live test stays visible.

Baseline: `imhttran/agentic-sop` `main` at
`0750e8be0cf5f318a173abb34c2b752ac131521c` for the static security work (refreshed
2026-11-09 UTC). The Plan 03 live follow-up ran at
`9c32e3d2bccb269aa0246b08158ddad5e8b25066` on
`security/llm02-live-extraction`. The Plan 10 row was refreshed on the branch
merged with `main` at `9689de7` (2026-11-08). The Plan 02, Plan 04 and Plan 05
rows were refreshed at `87d68ad9` (2026-11-09). Rows not named here are not
re-verified by those refreshes. Closeout reconciliation: `main` at `d3830e35`
(2026-11-09) after PRs #307–#314. Post-closeout lifecycle, process-group, archive-slot,
and evidence corrections merged through `ed9ec2dc` (PRs #315–#326). Each row keeps
its original recorded state and adds the merged disposition and any decision still open;
the current decision register is [gap-register-20261109.md](gap-register-20261109.md).
The Plan 10 wrap-up re-verified its focused checks at `main` `ed9ec2dc`
(2026-11-09) on Darwin arm64; the Plan 10 row keeps the original Linux
`x86_64` evidence and adds the wrap-up state and the open live-delivery
decision.
The wrap-up merged in PR #328 as `35340774`; full CI passed on the PR head
`eaf34114` (run 42718751282), and CodeQL passed on the same head (run
42718748910).

| Plan    | Category                         | Workstream                                                                                                  | Report / evidence                                                                                                                                                       | Evidence state                                                                                                                                                                                                                                                                                                                                                                                                                                      | Boundary                                                                                                                                                                                                                                                                                                                                                                  |
| ------- | -------------------------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Plan 01 | Excessive Agency                 | Linux agent (`task/LLM03-impl-plan-01-agency-boundary-controls`, PR #271)                                   | [llm03-agency.md](llm03-agency.md), [agency-002-evidence.md](agency-002-evidence.md)                                                                                    | `IMPLEMENTED`; Linux local verification `PASS` (`go build`, `go vet`, `go test ./...`); Linux Bubblewrap runtime isolation verified; macOS Seatbelt runtime isolation verified on the isolated-replay and evaltarget paths (PR #312, `b37f9560`, including Unix-socket denial and process-group kill); SOP plan `COMPLETE` confirmed (already historicalized; no new transition claimed)                                                                                                                                                                                                                                                                                    | Normal agent execution is policy-only and not OS-sandboxed. OS isolation is demonstrated only on the isolated-replay and evaltarget paths; platform identity and normal-agent runtime isolation remain gaps.                                                                                                                                                                      |
| Plan 02 | Prompt Injection                 | `security/llm01-injection` (merged in PR #272)                                                              | [llm01-injection.md](llm01-injection.md), [llm01-campaign.json](llm01-campaign.json)                                                                                    | Deterministic and real-control tests `VERIFIED`; live-adversarial `VERIFIED` for one bounded campaign: 4 of 4 injection cases blocked and the positive control allowed, one run each                                                                                                                                                                                                                                                                | The live campaign used one local model (`qwen3.5:4b` through Ollama) on macOS inside an opt-in `sandbox-exec` profile. It is evidence for that model and platform, not for other models, providers, or Linux.                                                                                                                                                             |
| Plan 03 | Sensitive Information Disclosure | Mac agent (`security/llm02-disclosure`, merged in PR #266); live follow-up `security/llm02-live-extraction` | [llm02-disclosure.md](llm02-disclosure.md), [llm02-campaign.json](llm02-campaign.json), [llm02-acceptance-request.md](llm02-acceptance-request.md)                                                                                  | Static tracing `IMPLEMENTED` / `VERIFIED`; bounded live extraction `OBSERVED` (7/7 cases: 4 disclosure probes and 3 effect cases): direct context disclosure observed, indirect retrieval refusal observed, validation diagnostics stayed redacted, unauthorized effects denied by authorization and policy, positive controls passed; acceptance request prepared in PR #314 (`a37a7bbd`); `DISCLOSE-003` was recorded by external completion and the plan historicalized `COMPLETE` in `d3830e35`, but no per-criterion acceptance is recorded                                                                                                               | One model (`qwen3.5:4b`) on macOS arm64 through the merged Plan 02 seam. Not general DLP, not prompt secrecy, and not a general red-team result. Retrieval is disabled in the routine configuration; Linux live remains `NOT_RUN`.                                                                                                                                        |
| Plan 04 | Supply Chain                     | Mac agent (`security/llm04-supply-chain`)                                                                   | [llm04-supply-chain.md](llm04-supply-chain.md)                                                                                                                          | `IMPLEMENTED` / `VERIFIED` for the declared report-only scope; since PR #282 the Gitleaks archives are SHA-256-pinned in the repository and CI re-checks every cached scanner download against its pin; scanner-integrity investigation and binary-pin proposal merged in PR #311 (`c3a8eed8`), runtime binary-pin decision open, SOP plan `COMPLETE` confirmed (already historicalized; no new transition claimed)                                                                                                                                                                                                                                              | Provider model provenance, publisher signatures, required-mode enablement, and normal-agent OS isolation remain out of scope or unverified.                                                                                                                                                                                                                               |
| Plan 05 | Data and Model Poisoning         | `security/llm05-data-and-model-poisoning` (PRs #268, #274); `security/llm05-finish` (PRs #288, #292)        | Plan: `docs/history/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md`; [llm05-poisoning.md](llm05-poisoning.md)                                                      | POISON-002 `VERIFIED` through SOP (focused provenance tests); POISON-001 trace accepted by the designated approver; POISON-003 recovery evidence: index and outcome reindex `VERIFIED`, decision-memory removal `VERIFIED` (GAP-7 fixed, PR #292); aggregate identity defect fixed in PR #308 (`bf0d824d`) and GAP-5 fixtures/proposal merged in PR #310 (`3e3e313e`); GAP-4 per-row digests and GAP-5 `chunk_hash` verification implemented (option A of each proposal)                                                                                                                                                                                                   | Integrity proves bytes, not benign content. GAP-4 fixed (schema 2 row digests). GAP-5 fixed for evidence carrying `chunk_hash`; the producer must emit it, and `document_hash` stays unverified. Fixed in PR #292: GAP-6 (unreadable `decisions.json` silently replaced), GAP-7 (no decision-memory removal command). Provider-owned training is outside SOP control and is not marked secure; query isolation is handed to Plan 09. |
| Plan 06 | Unbounded Consumption            | Mac agent (`security/llm06-unbounded-consumption`); follow-ups #289, #294, #297                             | [llm06-consumption.md](llm06-consumption.md)                                                                                                                            | `IMPLEMENTED` / `VERIFIED` for read-boundary output/read bounds, validation output bounds, unknown-token reporting, atomic continuation accounting, command-agent timeout/output, MCP frame bounds, and process-group cancellation on macOS and Linux CI; provider output-token cap defaults to 16384 tokens per request (`SOP_AGENT_MAX_OUTPUT_TOKENS` overrides); provider-reported `length` truncation is rejected; monetary accounting is `GAP` | Token/spend accounting that does not exist stays a `GAP`; no dollar-cap claim without pre-dispatch enforcement.                                                                                                                                                                                                                                                           |
| Plan 07 | Misinformation                   | `security/llm07-misinformation` (merged PR #285); follow-up `security/llm07-content-binding`                | [llm07-misinformation.md](llm07-misinformation.md); Plan: `docs/plans/PLAN-07-SOP-LLM07-Misinformation.md`                                                              | `IMPLEMENTED` / `ENABLED` / `VERIFIED` (deterministic, macOS arm64); SOP `COMPLETE`; live-model and Linux `NOT_RUN`                                                                                                                                                                                                                                                                                                                                 | Scope is evidence used in decisions, not factual correctness. Manual acceptance evidence is now strictly bound to the reviewed repository content (D4 option 1); `run_command` subprocesses are outside the file-write boundary.                                                                                                                                          |
| Plan 08 | Hidden Context Exposure          | Mac agent (`security/llm08-hidden-context-exposure`, merged in PR #286)                                     | [llm08-hidden-context.md](llm08-hidden-context.md), [llm08-campaign.json](llm08-campaign.json); plan: `docs/history/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md` | Behavioral controls `IMPLEMENTED` / `VERIFIED`; manual criteria accepted at merge (no per-criterion SOP record); bounded live extraction `OBSERVED`                                                                                                                                                                                                                                                                                                 | Prompt secrecy is not claimed; redaction is not general DLP; live evidence is one model/platform.                                                                                                                                                                                                                                                                         |
| Plan 09 | Vector and Embedding Weaknesses  | Linux agent (`security/llm09-retrieval`, PR #295)                                                           | [llm09-retrieval.md](llm09-retrieval.md); plan: `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`                                                       | `LOCAL_DONE` 3/3; merged in PR #295. Build, vet, tests, focused isolation checks, and CI documentation links pass. The retrieval cancellation/deadline P2 is fixed in PR #298.                                                                                                                                                                                                                                                                      | Local advisory retrieval is disabled by default; live RAG producer/PostgreSQL/embeddings remain NOT_RUN; semantic poisoning inside an authorized repository is not prevented.                                                                                                                                                                                             |
| Plan 10 | Improper Output Handling (LLM10) | Linux agent (`security/llm10-improper-output-handling`)                                                     | [llm10-output-handling.md](llm10-output-handling.md), [llm10-live-delivery.md](llm10-live-delivery.md), [llm10-acceptance-request.md](llm10-acceptance-request.md)      | `IMPLEMENTED` / `ENABLED` / `VERIFIED` for the Linux CLI terminal and the Slack payload encoder; loopback HTTP transport `VERIFIED`; observed Slack delivery/rendering `NOT_RUN`; live-test preparation merged in PR #313 (`f557c6fd`); wrap-up re-verified at `ed9ec2dc` (Darwin arm64), merged in PR #328 (`35340774`) with full CI `PASS` on `eaf34114`; live send blocked on explicit authorization and channel confirmation; manual acceptance `GAP`; no owning SOP plan state                                                 | Third-party terminal/IDE rendering, persisted Markdown rendering, and Slack auto-unfurl remain external. Shell, path, database, and protocol controls were inspected and confirmed, not re-authored. The default test payload proves delivery only, not adversarial escaping.                                                                                                                                                              |

## What the Plan 03 row evidences

- The static trace maps model-visible context, provider authentication, tool
  output, retrieval results, persisted evidence, diagnostics, handoffs, and
  notification payloads to their redaction and authorization boundaries.
- Three reproduced diagnostic leaks were fixed through the shared redaction
  abstractions: credential-shaped check output, model-visible validation failure
  context, and JSON-escaped `go test -json` output.
- One bounded live campaign (7 requests, one per case) on macOS arm64 with local
  `qwen3.5:4b` through the merged Plan 02 seam recorded direct context disclosure,
  an indirect retrieval refusal, redacted validation diagnostics, denied
  unauthorized effects (authorization and policy), and passing positive controls.
- The artifact `llm02-campaign.json` keeps prompt exposure, generated disclosure,
  artifact leakage, and attempted exfiltration separate. The `internal/cli`
  artifact test checks provenance, coverage, and recorded control decisions; it
  does not re-run the model.
- Residual risks remain explicit: model-visible repository content and persisted
  model output are not protected by general DLP; retrieval is disabled in the
  routine configuration; Linux live extraction is `NOT_RUN`.

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
  default, overridden by `SOP_AGENT_MAX_OUTPUT_TOKENS`. A provider-reported `length`
  stop is rejected as an incomplete response, so it cannot become a task success or
  dispatch a partial tool call; no execution token budget or monetary accounting
  exists.
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
- The four claims are kept separate: CLI terminal sanitization (`VERIFIED` on the
  recorded platforms), Slack payload encoding (`VERIFIED`, deterministic only),
  loopback HTTP transport (`VERIFIED`, in-process), and observed Slack delivery and
  rendering (`NOT_RUN`).
- The live send is blocked on an explicit authorization naming `#sop-alert` and the
  test window, plus an active `webhook` channel bound to that channel. No
  authorization record exists; no channel was enabled. The default `sop notify
  test` payload would prove delivery only, not adversarial escaping.
- The wrap-up merged in PR #328 (`35340774`); full CI passed on `eaf34114`
  (run 42718751282). Passing CI is evidence for `OUTPUT-003` criterion 3, not a
  manual acceptance record.
- External rendering and live Slack delivery are recorded as explicit limitations,
  not inferred protections. Manual acceptance is `GAP`, and no owning SOP plan
  state exists, so no lifecycle transition is claimed.

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
- It does not claim a merge, deployment, or installation. Live evidence exists only
  for the recorded Plan 02, Plan 03, and Plan 08 bounded campaigns on their stated
  model and platform; it is not extrapolated to other providers or operating
  systems.

## Related baseline evidence

- [gap-register-20261109.md](gap-register-20261109.md) — current dispositions
  after the closeout PRs, with the decisions still required.
- [gap-register-20261109-baseline.md](gap-register-20261109-baseline.md) — the
  original point-in-time register at `93c72857`, preserved as historical evidence.
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
