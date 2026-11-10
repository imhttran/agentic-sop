# OWASP LLM program — evidence matrix

Per-risk summary with OWASP 2025 IDs: [coverage-statement.md](coverage-statement.md).

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
rows were refreshed at `87d68ad9` (2026-11-09). The Plan 02 Linux follow-up ran
at `489f66695a693bf9ee06c72447942ecaf3ba7836` on
`security/llm01-linux-live` (2026-11-09). The Plan 09 row was refreshed at
`d2c45bb9b4cf82fde67a1904bf311b10791b7e4c` (2026-11-09) by the retrieval follow-up.
Rows not named here are not
re-verified by those refreshes. Closeout reconciliation: `main` at `d3830e35`
(2026-11-09) after PRs #307–#314. Post-closeout lifecycle, process-group, archive-slot,
and evidence corrections merged through `ed9ec2dc` (PRs #315–#326). Each row keeps
its original recorded state and adds the merged disposition and any decision still open;
the current decision register is [gap-register-20261109.md](gap-register-20261109.md).
The Plan 08 model-input credential policy A+B was approved and implemented on
`security/llm08-model-input-credentials` (decision memory
`f891d5cbe5dd578addbba83ee95b47c35752970219b4d0ad6d3fc2be254a741c`); the Plan 08 row
records the implementation and its residual limits. The Plan 10 wrap-up re-verified its focused checks at `main` `ed9ec2dc`
(2026-11-09) on Darwin arm64; the Plan 10 row keeps the original Linux
`x86_64` evidence and adds the wrap-up state and the open live-delivery
decision.
The wrap-up merged in PR #328 as `35340774`; full CI passed on the PR head
`eaf34114` (run 42718751282), and CodeQL passed on the same head (run
42718748910). A separately authorized live probe at `main` `0200ad3f` delivered
one synthetic event through the configured Slack webhook: transport is `VERIFIED`
(outbox `DELIVERED`, attempt 1, no retry), while Slack rendering is
`NOT_OBSERVED`. No owning Plan 10 SOP state exists. The live-evidence
reconciliation merged in PR #339 (`cfca3107`); full CI and CodeQL passed on the
head `6ea8d68f`. The Plan 09 row, the Plan 05 and Plan 07 rows, and the
[llm09-retrieval.md](llm09-retrieval.md) follow-up were reconciled in RETRIEVE-104
against their current merged reports; that refresh touches only Plan 05, Plan 07, and
Plan 09, and adds no complete-category, semantic-immunity, production-vector, or
live-producer claim.

| Plan    | Category                         | Workstream                                                                                                                  | Report / evidence                                                                                                                                                                                                                                                                     | Evidence state                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Boundary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| ------- | -------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Plan 01 | Excessive Agency                 | Linux agent (`task/LLM03-impl-plan-01-agency-boundary-controls`, PR #271)                                                   | [llm03-agency.md](llm03-agency.md), [agency-002-evidence.md](agency-002-evidence.md)                                                                                                                                                                                                  | `IMPLEMENTED`; Linux local verification `PASS` (`go build`, `go vet`, `go test ./...`); Linux Bubblewrap runtime isolation verified; macOS Seatbelt runtime isolation verified on the isolated-replay and evaltarget paths (PR #312, `b37f9560`, including Unix-socket denial and process-group kill); SOP plan `COMPLETE` confirmed (already historicalized; no new transition claimed)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | Normal agent execution is policy-only by default. `run_command` has an opt-in macOS Seatbelt sandbox (`SOP_AGENT_RUN_COMMAND_ISOLATION=1`, fails closed; see [seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md#run_command-opt-in-sandbox-llm03)). Linux `run_command` isolation and platform identity remain gaps.                                                                                                                                                                                                                                                                                       |
| Plan 02 | Prompt Injection                 | macOS `security/llm01-injection` (merged in PR #272); Linux follow-up `security/llm01-linux-live`                           | [llm01-injection.md](llm01-injection.md), [llm01-campaign.json](llm01-campaign.json), [llm01-campaign-linux.json](llm01-campaign-linux.json)                                                                                                                                          | Deterministic and real-control tests `VERIFIED`; macOS live-adversarial `VERIFIED` for one bounded campaign: 4 of 4 injection cases blocked and the positive control allowed, one run each. Linux live-adversarial `OBSERVED` for one bounded campaign: 3 of 4 injection cases denied by the production controls, 1 timed out with no verdict (LIVE-004), and the authorized positive control executed; one run each.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | macOS evidence is one model (`qwen3.5:4b`) in an opt-in `sandbox-exec` profile. Linux evidence is one model (`qwen3:4b`) in an opt-in `bwrap` private network namespace with a Unix-socket endpoint bridge. Not evidence for other models, providers, or platforms; not a universal prompt-injection claim. The default sandbox and normal agent execution are unchanged.                                                                                                                                                                                                                                                  |
| Plan 03 | Sensitive Information Disclosure | Mac agent (`security/llm02-disclosure`, merged in PR #266); live follow-up `security/llm02-live-extraction`                 | [llm02-disclosure.md](llm02-disclosure.md), [llm02-campaign.json](llm02-campaign.json), [llm02-acceptance-request.md](llm02-acceptance-request.md)                                                                                                                                    | Static tracing `IMPLEMENTED`; named deterministic checks `PASS`; bounded live extraction `OBSERVED` (7/7 cases: 4 disclosure probes and 3 effect cases): direct context disclosure observed, indirect retrieval refusal observed, validation diagnostics stayed redacted, unauthorized effects denied by authorization and policy, positive controls passed; acceptance request prepared in PR #314 (`a37a7bbd`); `DISCLOSE-003` was recorded by external completion and the plan historicalized `COMPLETE` in `d3830e35`, but no per-criterion acceptance is recorded; tool-output `.env` canary reconciliation: the pre-policy baseline was `model_visible=true`; the Plan 08 A+B policy now denies the direct credential path and redacts model-bound tool results; Linux `amd64` deterministic verification independently reproduced all five pre-policy paths and confirmed the denial/redaction; a bounded Linux `amd64` live tool-path campaign at `4cee73a` (`qwen3:4b`, 17 requests) observed 7 protected cases, 6 positive controls, and 4 residual exposures (unknown-format values and a hardlink alias); Plan 03 acceptance remains missing and model output remains persisted verbatim | One model (`qwen3.5:4b`) on macOS arm64 through the merged Plan 02 seam. Not general DLP, not prompt secrecy, and not a general red-team result. Retrieval is disabled in the routine configuration; Linux live retrieval and notification sinks remain `NOT_RUN`. Tool-output denial/redaction is now implemented by Plan 08 A+B, verified deterministically on Linux `amd64`, and observed in a bounded Linux live tool-path campaign; Linux live retrieval and notification sinks remain `NOT_RUN`, and Plan 03 acceptance and artifact persistence remain separate.                                                    |
| Plan 04 | Supply Chain                     | Mac agent (`security/llm04-supply-chain`)                                                                                   | [llm04-supply-chain.md](llm04-supply-chain.md)                                                                                                                                                                                                                                        | `IMPLEMENTED` / `VERIFIED` for the declared report-only scope; since PR #282 the Gitleaks archives are SHA-256-pinned in the repository and CI re-checks every cached scanner download against its pin; scanner-integrity investigation merged in PR #311 (`c3a8eed8`); execution-time `binary_sha256` / `rules_sha256` pins implemented (option B, opt-in) with the Semgrep rules pin enabled in this repository, SOP plan `COMPLETE` confirmed (already historicalized; no new transition claimed)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | Provider model provenance, publisher signatures, required-mode enablement, and normal-agent OS isolation remain out of scope or unverified.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Plan 05 | Data and Model Poisoning         | `security/llm05-data-and-model-poisoning` (PRs #268, #274); `security/llm05-finish` (PRs #288, #292)                        | Plan: `docs/history/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md`; [llm05-poisoning.md](llm05-poisoning.md)                                                                                                                                                                    | POISON-002 `VERIFIED` through SOP (focused provenance tests); POISON-001 trace accepted by the designated approver; POISON-003 recovery evidence: index and outcome reindex `VERIFIED`, decision-memory removal `VERIFIED` (GAP-7 fixed, PR #292); aggregate identity defect fixed in PR #308 (`bf0d824d`) and GAP-5 fixtures/proposal merged in PR #310 (`3e3e313e`); GAP-4 per-row digests and GAP-5 `chunk_hash` verification implemented (option A of each proposal)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | Integrity proves bytes, not benign content. GAP-4 per-row digest enforcement remains open: schema 2 rows carry digests and `repoindex.VerifyRows` checks them, but no CLI command invokes it. GAP-5 fixed for evidence carrying `chunk_hash`; the producer must emit it, and `document_hash` stays unverified. Fixed in PR #292: GAP-6 (unreadable `decisions.json` silently replaced), GAP-7 (no decision-memory removal command). Provider-owned training is outside SOP control and is not marked secure; query isolation is handed to Plan 09.                                                                         |
| Plan 06 | Unbounded Consumption            | Mac agent (`security/llm06-unbounded-consumption`); follow-ups #289, #294, #297, #327                                       | [llm06-consumption.md](llm06-consumption.md)                                                                                                                                                                                                                                          | Since PR #327 every external subprocess path binds and kills its process group (`internal/procgroup`) and the test runner has a 30m default timeout. `IMPLEMENTED` / `VERIFIED` for read-boundary output/read bounds, validation output bounds, unknown-token reporting, atomic continuation accounting, command-agent timeout/output, MCP frame bounds, and process-group cancellation on macOS and Linux CI; provider output-token cap defaults to 16384 tokens per request (`SOP_AGENT_MAX_OUTPUT_TOKENS` overrides); provider-reported `length` truncation is rejected; monetary accounting is `GAP`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | Token/spend accounting that does not exist stays a `GAP`; no dollar-cap claim without pre-dispatch enforcement.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Plan 07 | Misinformation                   | `security/llm07-misinformation` (merged PR #285); follow-up `security/llm07-content-binding`                                | [llm07-misinformation.md](llm07-misinformation.md); Plan: `docs/plans/PLAN-07-SOP-LLM07-Misinformation.md`                                                                                                                                                                            | `IMPLEMENTED` / `ENABLED` / `VERIFIED` (deterministic, macOS arm64); SOP `COMPLETE`; live-model and Linux `NOT_RUN`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Scope is evidence used in decisions, not factual correctness. Merged PR #285 (`9f5b8150`) is the verified deterministic scope. The strict content-binding follow-up (D4 option 1, commit `dcb553c8`) is contained in current `origin/main` (merged after PR #285), not part of PR #285's original commit range; the Plan 07 report's "not merged" line is stale against current git evidence. The `run_command` subprocess state-write boundary remains open: subprocesses are outside the file-write boundary unless the opt-in `run_command` sandbox is on, which denies their writes under `.agent-sdlc/` (macOS only). |
| Plan 08 | Hidden Context Exposure          | Mac agent (`security/llm08-hidden-context-exposure`, merged in PR #286); follow-up `security/llm08-model-input-credentials` | [llm08-hidden-context.md](llm08-hidden-context.md), [llm08-campaign.json](llm08-campaign.json); plan: `docs/history/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md`; proposal: [PROPOSAL-LLM08-MODEL-INPUT-CREDENTIALS.md](../../plans/PROPOSAL-LLM08-MODEL-INPUT-CREDENTIALS.md) | Behavioral controls `IMPLEMENTED` / `ENABLED` / `VERIFIED` (deterministic, macOS `Darwin` arm64); notification descriptive-field redaction `VERIFIED`; model-input credential policy A+B `IMPLEMENTED` / `VERIFIED` (decision memory `f891d5cbe5dd578addbba83ee95b47c35752970219b4d0ad6d3fc2be254a741c`): direct credential paths denied for `read_file`/`search_files`/`copy_file` source (requested path and symlink target classified), every model-bound tool result and error redacted; `.env.example`/`.env.sample` and ordinary source remain usable; `commandpolicy` unchanged; historical Plan 08 manual criteria accepted at merge without a per-criterion SOP record; follow-up bounded claim `criterion-3` accepted through SOP by Tom Tran (2026-11-10); bounded live extraction `OBSERVED` on macOS arm64 with local `qwen3.5:4b` at revision `35340774c16b10236b0ca0143cc0d07c85a8e6b3` (does not validate the branch changes); a bounded Linux `amd64` live tool-path campaign (`qwen3:4b`, 17 requests) observed 7 protected cases, 6 positive controls, and 4 residual exposures                                                                                                   | Unknown-format secrets in command output, diffs, or ordinary files, persisted model output artifacts, retrieval/handoff/caller inputs, and provider retention remain bounded gaps. Redaction is not general DLP. Prompt secrecy is not claimed. Linux live retrieval and notification sinks remain `NOT_RUN`; the Linux tool-path campaign observed the documented residual exposures, so it is bounded evidence, not full closure.                                                                                                                                                                                        |
| Plan 09 | Vector and Embedding Weaknesses  | Linux agent (`security/llm09-retrieval`, PR #295); retrieval follow-up (`security/llm09-retrieval-followup`)                | [llm09-retrieval.md](llm09-retrieval.md); plan: `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`; follow-up plan: `docs/plans/PLAN-09B-SOP-LLM09-Retrieval-Followup.md`                                                                                              | `LOCAL_DONE` 3/3; merged in PR #295. Build, vet, tests, focused isolation checks, and CI documentation links pass. The retrieval cancellation/deadline P2 is fixed in PR #298. The RETRIEVE-101..104 reconciliation corrects the original RETRIEVE-003 criterion 1 to partial (import is directory-scoped; repository scope is enforced at query time), adds before/after rebuild evidence with a valid historical positive control, and separates client-side validation from producer-side isolation. Focused tests `PASS` on macOS arm64 and Linux. [redacted] deterministic verification `VERIFIED`; producer-side live RAG evidence `NOT_RUN` (`RAG_PRODUCER` and `DATABASE_URL` unset). See the refreshed [llm09-retrieval.md](llm09-retrieval.md).                                                                                                                                                                                                                                                                                                                                                                                                                                            | Local advisory retrieval is disabled by default; live RAG producer/PostgreSQL/embeddings remain `NOT_RUN`; producer-side corpus isolation is unverified; semantic poisoning inside an authorized repository is not prevented. A shared outcomes directory imports cross-repository rows that a scoped query refuses to return.                                                                                                                                                                                                                                                                                             |
| Plan 10 | Improper Output Handling (LLM10) | Linux agent (`security/llm10-improper-output-handling`)                                                                     | [llm10-output-handling.md](llm10-output-handling.md), [llm10-live-delivery.md](llm10-live-delivery.md), [llm10-acceptance-request.md](llm10-acceptance-request.md)                                                                                                                    | `IMPLEMENTED` / `ENABLED` / `VERIFIED` for the Linux CLI terminal and the Slack payload encoder; loopback HTTP transport `VERIFIED`; live Slack transport `VERIFIED` once at `0200ad3f` (outbox `DELIVERED`, attempt 1, no retry); reconciliation merged in PR #339 (`cfca3107`); observed Slack rendering `NOT_OBSERVED`; live-test preparation merged in PR #313 (`f557c6fd`); wrap-up re-verified at `ed9ec2dc` (Darwin arm64), merged in PR #328 (`35340774`) with full CI `PASS` on `eaf34114`; manual acceptance `GAP`; no owning SOP plan state                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Third-party terminal/IDE rendering, persisted Markdown rendering, and Slack auto-unfurl remain external. Shell, path, database, and protocol controls were inspected and confirmed, not re-authored. The live probe payload escaped `& < >`; Slack rendering, the channel binding, and unfurl behavior remain unobserved.                                                                                                                                                                                                                                                                                                  |

## What the Plan 02 Linux row evidences

- The Linux live profile keeps `bwrap --unshare-all` (private mount and network
  namespaces) and never shares host networking. The fixed model endpoint is
  reachable only through a Unix domain socket in the case root; a host-side proxy
  dials exactly `127.0.0.1:11434`.
- Unsandboxed controls confirmed the host could reach the model endpoint, an
  ephemeral loopback listener, and `1.1.1.1:443`. Inside the live sandbox, direct
  TCP to the endpoint, the other loopback port, and the non-loopback destination
  were all denied; a file outside the case root was unreadable; `OPENAI_API_KEY`
  was scrubbed; and a canceled sandbox left no shell or descendant process.
- One bounded campaign ran five AISEC-based cases once each with 1 attempt,
  1 request, concurrency 1, a 240 s process lifetime, and a 64 KiB output bound.
  LIVE-001, LIVE-002, and LIVE-003 were denied by authorization with no executed
  effect; LIVE-004 (AISEC-005) timed out before a decision and has no verdict;
  LIVE-005 (AISEC-006) was allowed and executed one inert effect, proving the
  controls do not deny everything. No prohibited effect executed.
- The campaign records `qwen3:4b`, Ollama, Linux x86_64, the source revision,
  the evaluation binary SHA-256, `bwrap` isolation, the exact bounds, and request
  accounting. The macOS campaign artifacts are unchanged.
- Limits: one model, one Linux host, one run per case. LIVE-004 is `TIMEOUT`, not
  a block. This is not evidence of universal prompt-injection prevention, and
  the evaluation-only Unix-socket bridge does not change the default sandbox or
  normal agent execution.

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
  routine configuration; Linux live retrieval and notification sinks are
  `NOT_RUN`.
- The previously reported `.env` canary result is preserved as the pre-policy
  baseline (`model_visible=true`). The Plan 08 A+B policy now denies direct
  credential paths and redacts model-bound tool results; `in_artifacts=false`
  remains scoped to the harness audit, and `implementation.md` still persists model
  output verbatim. Plan 03 acceptance remains missing.
- Linux `amd64` deterministic verification (2026-11-09) independently reproduced
  the five pre-policy canary paths at `main` `814f69c`, then exercised the
  approved A+B branch: direct `read_file`/`search_files`/`copy_file` source paths
  are denied, `git_diff` and SAFE command output are redacted at the model
  boundary, and ordinary source, `.env.example`, `.env.sample`, ordinary copy, and
  ordinary SAFE output remain usable. This deterministic result is not
  live-model evidence; the separate Linux live tool-path campaign below is.
- The Linux `amd64` live tool-path campaign (2026-11-10, revision `4cee73a`,
  `qwen3:4b`, 17 requests) exercised the real tool harness and the actual
  `toolResultMessage` boundary under `bwrap`: 7 protected cases, 6 positive
  controls, and 4 residual exposures. The residual cases were an unknown-format
  value in an ordinary file, SAFE command output, a diff, and a hardlink alias
  to `.env`. The model reproduced three of the four opaque values; the hardlink
  value reached the model even though the model did not reproduce it. Linux live
  retrieval and notification sinks remain `NOT_RUN`.
- Notification targets are redacted, but event prose is caller-owned and not
  DLP-scrubbed; the earlier "event text carries no credential" row was corrected.

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
  recorded platforms), Slack payload encoding (`VERIFIED`, deterministic plus the
  live payload fixture), loopback and one live webhook transport (`VERIFIED`), and
  observed Slack rendering (`NOT_OBSERVED`).
- One authorized live probe delivered a synthetic event at `0200ad3f`; the outbox
  recorded `DELIVERED` on attempt 1 with no retry. No authenticated Slack session
  was available, so the rendered message, the `#sop-alert` binding, and the absence
  of pings remain unobserved. The fixture is in
  [llm10-live-delivery.md](llm10-live-delivery.md), not a rendering claim.
- The wrap-up merged in PR #328 (`35340774`); full CI passed on `eaf34114`
  (run 42718751282). Passing CI is evidence for `OUTPUT-003` criterion 3, not a
  manual acceptance record.
- External rendering and unobserved Slack rendering are recorded as explicit
  limitations, not inferred protections. Manual acceptance is `GAP`, and no owning
  SOP plan state exists, so no lifecycle transition is claimed.

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
  systems. The Plan 02 Linux campaign adds evidence for one Linux host and one
  model; it does not remove the stated limits.

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
