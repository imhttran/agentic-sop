# OWASP LLM Top 10 — coverage statement

**As of:** `main` at `e341c3e1` (2026-11-10, LLM02 C1 hardlink-alias verification).

This statement answers one question for each of the ten OWASP LLM risks: what
SOP covers, where the evidence is, and what it does not cover, with the reason.
It is a summary of the [evidence matrix](README.md) and the
[gap register](gap-register-20261109.md), which stay authoritative. It is not a
compliance certification, and "covered" never means "the risk is eliminated".

The repository labels its plans with its own LLM numbers (`llm03-agency.md`
covers Excessive Agency, for example). The first column gives the OWASP Top 10
for LLM Applications 2025 ID. The second gives the repository's plan.

## Summary

| OWASP 2025 | Repo plan / label | Risk                                                  | Coverage                                                                                                                                                                                                                                                                                    | Main remaining gap                                                                                                                                                                                   |
| ---------- | ----------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| LLM01      | Plan 02 / LLM01   | Prompt Injection                                      | Covered (controls + bounded live campaigns)                                                                                                                                                                                                                                                 | One model per platform; no universal claim                                                                                                                                                           |
| LLM02      | Plan 03 / LLM02   | Sensitive Information Disclosure                      | Covered for the user prompt and tool results (trace + macOS/Linux live campaigns); bounded evidence acceptance `RECORDED`; C1 unknown-format redaction `IMPLEMENTED` opt-in (PRs #361/#362/#364) with bounded Linux live `PASS`; focused C1 hardlink verification `PASS` for 4 scoped cases | Not general DLP; four pre-C1 Linux residual exposures; hardlink path bypass with one unlisted-value residual; full Plan 03 acceptance `NEEDS_HUMAN`; C1 disabled by default; broader coverage `OPEN` |
| LLM03      | Plan 04 / LLM04   | Supply Chain                                          | Partial                                                                                                                                                                                                                                                                                     | Model provenance and publisher signatures out of SOP's control                                                                                                                                       |
| LLM04      | Plan 05 / LLM05   | Data and Model Poisoning                              | Partial                                                                                                                                                                                                                                                                                     | Integrity proves bytes, not benign content; provider training external                                                                                                                               |
| LLM05      | Plan 10 / LLM10   | Improper Output Handling                              | Covered for SOP's sinks                                                                                                                                                                                                                                                                     | Third-party rendering (terminal, IDE, Slack) external                                                                                                                                                |
| LLM06      | Plan 01 / LLM03   | Excessive Agency                                      | Covered (policy) + opt-in sandbox on macOS and Linux                                                                                                                                                                                                                                        | Sandbox off by default; cold build cache per run                                                                                                                                                     |
| LLM07      | Plan 08 / LLM08   | System Prompt Leakage (repo: Hidden Context Exposure) | Partial                                                                                                                                                                                                                                                                                     | Prompt secrecy is not claimed by design                                                                                                                                                              |
| LLM08      | Plan 09 / LLM09   | Vector and Embedding Weaknesses                       | Covered for the shipped (disabled-by-default) path                                                                                                                                                                                                                                          | Live RAG / PostgreSQL / embeddings `NOT_RUN`                                                                                                                                                         |
| LLM09      | Plan 07 / LLM07   | Misinformation                                        | Partial, by scope                                                                                                                                                                                                                                                                           | Factual correctness of model output is out of scope                                                                                                                                                  |
| LLM10      | Plan 06 / LLM06   | Unbounded Consumption                                 | Covered for time/output bounds, selected process-group termination, and opt-in token budgets                                                                                                                                                                                                | No dollar cap (no pricing data); some subprocess paths lack process-group termination                                                                                                                |

## Per risk

### LLM01 Prompt Injection — covered, bounded

- **Covered:** tool authorization, command policy and sandboxed live campaigns.
  On macOS, 4 of 4 injection cases were blocked and the positive control was
  allowed. On Linux, 3 of 4 cases were denied and 1 timed out with no verdict.
  Evidence: [llm01-injection.md](llm01-injection.md).
- **Not covered, and why:** prompt injection cannot be prevented in general. The
  evidence is one model per platform, run once per case, so it is not a claim
  about other models or providers.

### LLM02 Sensitive Information Disclosure — covered, bounded

- **Covered:** tool results and tool errors are redacted before they reach the
  model, and credential paths are denied for `read_file`, `search_files` and
  `copy_file` (Plan 08 A+B). Since the task-input fix, the whole user prompt
  (`userPrompt`: task, caller input, output requirements) passes the same
  model-input redaction (`TestUserPromptRedactsTaskAndInput`). Diagnostics and
  notifications are redacted. A macOS live extraction campaign ran 7 of 7 cases;
  a Linux live tool-path campaign ran 17 cases (7 protected, 6 positive controls,
  4 residual exposures), and a focused C1 hardlink-alias campaign ran 6 cases
  (4 protected, 1 positive control, 1 unlisted-value residual). Evidence:
  [llm02-disclosure.md](llm02-disclosure.md) and
  [llm02-acceptance-request.md](llm02-acceptance-request.md).
- **Not covered, and why:**
  - The live campaigns predate the task-input fix; that fix is verified
    deterministically only and was not re-run live.
  - Context that reaches the model by another route than the user prompt or a
    tool result (for example a system prompt) is not redacted.
  - Redaction is pattern-based, not general DLP, so secrets in an unknown
    format can pass. The Linux live campaign observed unknown-format values in an
    ordinary file, SAFE command output, and a diff reaching the model.
  - A pre-existing hardlink with an ordinary name bypasses the lexical credential
    path denial. The focused C1 verification (PR #373, revision `ed1ba11`,
    artifact SHA-256 `417599e2…`, 6 requests) shows that C1 redacts known,
    inventoried, and contextual values after the read (4 protected cases), but
    the alias remains readable and one unlisted opaque value reached the model.
    **Path authorization and model-input redaction are separate controls.**
  - SAFE `run_command` (`go test` is `SAFE`) can run model-written code that
    reads `$HOME` credential files directly, or `os.Link`s them into the repo.
    The opt-in `run_command` sandbox restricts writes and network, not reads, so
    this works with or without it. This is a separate, unaccepted residual. The
    PR #376 review did not approve Option C Phase 1 as proposed; the revised
    sandbox credential read-deny plus `Nlink > 1` direction is materially
    different, is not implemented, and needs a new decision. The earlier C1 and
    bounded approvals remain unchanged.
  - Linux live retrieval and notification sinks are `NOT_RUN`.
  - Model-output artifacts remain verbatim; retrieval/handoff/caller prose and
    provider retention remain separate boundaries.
  - Bounded evidence acceptance is `RECORDED` in repository decision memory ID
    `59d986984142bef314ea784a401b204cd57b7ca1664f570b4763c13a5f580c98`; it is a
    separate scoped decision, not Plan 03 per-criterion acceptance.
  - The designated approver has not recorded Plan 03 per-criterion acceptance;
    the final closeout matrix marks it `NEEDS_HUMAN`. Historicalized `COMPLETE`
    is not acceptance, and broader LLM02 coverage remains `OPEN`.
  - The six-item residual-risk decision matrix records that the PR #376 review
    did not approve Option C Phase 1 as proposed, and that a SAFE `run_command`
    credential read is a separate unaccepted residual; the revised read-deny
    plus `Nlink > 1` direction needs a new decision. It recommends
    `INVESTIGATE` for
    unknown-format and artifact risks; `MITIGATE` for notification
    prose; `INVESTIGATE`/`ACCEPT_PROPOSED` for retrieval/handoff; and
    `TRANSFER_PROPOSED` for provider retention. No residual risk is accepted.
    The unknown-format proposal is
    [PROPOSAL-LLM02-UNKNOWN-FORMAT-REDACTION.md](../../plans/PROPOSAL-LLM02-UNKNOWN-FORMAT-REDACTION.md);
    C1 is now implemented opt-in (decision memory
    `e233daee7645d770d0813713b82523e4272d0b6ccb03c3e5974e5f434f0debe1`).
  - C1 unknown-format redaction is merged (PRs #361/#362/#364) and covers exact
    inventory values, contextual credential keys, and approved known formats;
    deterministic tests cover placeholders, fail-closed inventory, and the real
    harness/user-prompt wiring. Verification is `PASS` on macOS arm64 at
    `d69e9f63` and recorded `PASS` on Linux `amd64` at campaign baseline
    `5ff526a`; a bounded Linux `amd64` live campaign (PR #370, campaign revision
    `c5df5f0`, artifact SHA-256
    `14e41eaf1a7f23ffad37cadc35e7f7e45910777d2fea64115455475e2ca16cfa`) `PASS`es
    for 12 scoped cases with 3 positive controls and one bounded residual. The
    16-case campaign supplied inventory values in memory, so it did not exercise
    the external loader or the configuration path. The focused hardlink campaign
    loads an external `0600` inventory through `redact.LoadInventory` and confirms
    that C1 does not close the hardlink path bypass; the `config.RedactionPolicy`
    composition remains unit-tested only. C1 applies to the local Ollama
    tool-harness boundary (`userPrompt`,
    `toolResultMessage`), not the OpenAI-compatible provider path. The repository
    configuration remains disabled, so the pre-C1 residual remains until an
    operator enables it, and retrieval/notification sinks
    remain `NOT_RUN`.

### LLM03 Supply Chain — partial

- **Covered:** scanner archives are SHA-256-pinned in CI. Optional
  `binary_sha256` and `rules_sha256` pins are checked before every scanner run,
  and this repository enables the Semgrep rules pin (PR #338). Evidence:
  [llm04-supply-chain.md](llm04-supply-chain.md).
- **Not covered, and why:**
  - SOP cannot verify the provenance of a provider's hosted model.
  - Publisher signatures are not checked.
  - Binary pins are opt-in and platform-specific, so none are committed.
  - A small window exists between hashing a binary and running it.

### LLM04 Data and Model Poisoning — partial

- **Covered:**
  - Provenance on decision memory.
  - Per-row index digests (schema 2) and RAG `chunk_hash` verification (PR #330).
  - Recovery by reindexing and by removing decision-memory entries.

  Evidence: [llm05-poisoning.md](llm05-poisoning.md).

- **Not covered, and why:**
  - A digest proves the bytes are unchanged, not that the content is benign, so
    content poisoned inside an authorized repository is not detected.
  - Provider-side training is outside SOP's control.
  - `document_hash` stays unverified until the producer emits `chunk_hash`.

### LLM05 Improper Output Handling — covered for SOP's sinks

- **Covered:**
  - Terminal output and the Slack payload encoder are escaped.
  - Loopback transport is verified, and live Slack transport was verified once.
  - Shell, path, database and protocol controls were inspected.

  Evidence: [llm10-output-handling.md](llm10-output-handling.md).

- **Not covered, and why:** rendering in third-party terminals and IDEs,
  persisted Markdown, and Slack unfurl are outside SOP. How Slack rendered the
  message was not observed. All six Plan 10 manual criteria were accepted, and
  Plan 10 was historicalized `COMPLETE`; one Slack transport event was
  `DELIVERED` on attempt 1. Slack rendering and channel binding remain unobserved.

### LLM06 Excessive Agency — covered by policy, with opt-in sandboxing on macOS and Linux

- **Covered:**
  - Root and command policy, with read-only roots.
  - OS isolation on the replay and evaltarget paths, on both macOS and Linux.
  - Since PR #343 (macOS, Seatbelt) and the bubblewrap follow-up (Linux),
    `run_command` can run inside an opt-in sandbox
    (`SOP_AGENT_RUN_COMMAND_ISOLATION=1`). In it, outbound network is blocked,
    writes are limited to the root and temp, `.agent-sdlc/` is denied, and it
    fails closed when no sandbox is available. Each run gets its own throwaway
    build cache and the shared module cache is read-only, so one run cannot
    poison a later run's Go caches.

  Evidence: [llm03-agency.md](llm03-agency.md),
  [seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md#run_command-opt-in-sandbox-llm03).

- **Not covered, and why:**
  - The sandbox is off by default.
  - On Linux the sandbox uses its own network namespace, so a test cannot
    reach a server on the host's loopback; one it starts itself works. Host
    Unix sockets under `/tmp` and `/run` are hidden; one elsewhere (for example
    under `$HOME`) stays connectable.
  - The agent process itself runs outside SOP.
  - Every sandboxed build starts with a cold build cache (about 10 s for three
    of this repository's packages), and a module missing from the module cache
    cannot be downloaded inside the sandbox; run `go mod download` first.

### LLM07 System Prompt Leakage — partial, by design

- **Covered:**
  - Hidden context is not echoed into notifications or diagnostics.
  - Descriptive fields are redacted.
  - Model input is redacted at the single tool-result boundary.

  Evidence: [llm08-hidden-context.md](llm08-hidden-context.md).

- **Not covered, and why:** SOP does not treat its prompts as secret and does
  not rely on prompt secrecy for security, so prompt secrecy is not claimed.
  Secrets in an unknown format in command output, provider retention, and
  persisted model output remain bounded gaps.

### LLM08 Vector and Embedding Weaknesses — covered for the shipped path

- **Covered:** retrieval isolation and authorization, and bounded
  cancellation/deadline handling (PR #298). Advisory retrieval is disabled by
  default. Evidence: [llm09-retrieval.md](llm09-retrieval.md).
- **Not covered, and why:** a live RAG producer with PostgreSQL and embeddings
  is `NOT_RUN`, because no such deployment is part of the routine configuration.
  Semantic poisoning inside an authorized repository is not prevented (see
  LLM04).

### LLM09 Misinformation — partial, by scope

- **Covered:** evidence used in decisions is bound to real command output and
  to the reviewed repository content. A `go test -run` selection that matched no
  tests is flagged, so its exit code 0 is not read as tests passing. Evidence:
  [llm07-misinformation.md](llm07-misinformation.md).
- **Not covered, and why:** SOP checks that decisions rest on real evidence. It
  does not check that model prose is factually correct, which cannot be decided
  mechanically. A `run_command` subprocess can write SOP state unless the
  opt-in sandbox is on (macOS and Linux)
  ([llm07-state-write-detection-phase1.md](llm07-state-write-detection-phase1.md)).
  Live-model and Linux runs are `NOT_RUN`.

### LLM10 Unbounded Consumption — covered for time, output and process bounds

- **Covered:**
  - Read and output bounds, and a default cap on provider output tokens.
  - Per-command timeouts, MCP frame bounds and atomic continuation accounting.
  - Process-group termination covers the paths that bind `internal/procgroup`
    (test runner, handoff, HIP comparison command path, outcome/acceptance
    commands, review OCR, and RAG client) and packages with their own `Setpgid`
    helpers (`agent`, `security`, `toolharness`, `evaltarget`,
    `controller/sopclient`, and `decision/commandprofile`). It is not universal:
    `internal/decision/command/command.go`,
    `internal/decisionadapter/providers/julia/runner.go`, `internal/git`,
    `internal/github`, and `internal/hipcompare/changes.go` use
    `exec.CommandContext` without a process group, so a timeout kills only the
    direct child (or sets no group). The test runner has a 30-minute default
    timeout.

  Evidence: [llm06-consumption.md](llm06-consumption.md).
  - An opt-in per-run token budget, `SOP_AGENT_MAX_TOTAL_TOKENS`, is checked
    before every model request. It refuses the request once the run's reported
    input plus output tokens reach the budget, and fails closed when usage can't
    be measured (`TestCheckTokenBudget`, `TestTokenBudgetRefusesBeforeDispatch`).

- **Not covered, and why:**
  - The budget is off by default.
  - It is a token cap, not a dollar cap: SOP has no pricing data.
  - It is checked before each request, so the request that crosses the budget
    still completes. The overshoot is bounded by one request's input plus
    `SOP_AGENT_MAX_OUTPUT_TOKENS`.

## Open items that need an operator

These cannot be closed by code in this repository:

1. Plan 03 acceptance of the live campaign artifact
   ([llm02-acceptance-request.md](llm02-acceptance-request.md)).
2. Plan 10 Slack rendering and channel-binding observation
   ([llm10-acceptance-request.md](llm10-acceptance-request.md)).
3. Linux live runs for Plans 03, 07 and 08, and a live RAG deployment for
   Plan 09.

## Code follow-ups worth doing

- A dollar cap, if needed: it would need per-model pricing on top of the token
  budget.
