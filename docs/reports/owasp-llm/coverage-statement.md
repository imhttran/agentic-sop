# OWASP LLM Top 10 — coverage statement

**As of:** `main` at `b2f837b1` (2026-11-12, through PR #472).
Scanner enforcement remains report-only. PR #472 added advisory baseline-drift
diagnostics but merged with an unresolved partial-scan P2, recorded below.

This statement answers one question for each of the ten OWASP LLM risks: what
SOP covers, where the evidence is, and what it does not cover, with the reason.
It is a summary of the [evidence matrix](README.md) and the
[gap register](gap-register-20261109.md), which stay authoritative. It is not a
compliance certification, and "covered" never means "the risk is eliminated".

The repository labels its plans with its own LLM numbers (`llm03-agency.md`
covers Excessive Agency, for example). The first column gives the OWASP Top 10
for LLM Applications 2025 ID. The second gives the repository's plan.

## Summary

| OWASP 2025 | Repo plan / label | Risk                                                  | Coverage                                                                                                                                                                                                                                                                                                                                                          | Main remaining gap                                                                                                                                                                                                                                                               |
| ---------- | ----------------- | ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| LLM01      | Plan 02 / LLM01   | Prompt Injection                                      | Covered (controls + bounded live campaigns)                                                                                                                                                                                                                                                                                                                       | Two models on macOS, one on Linux, one run per case; no universal claim                                                                                                                                                                                                          |
| LLM02      | Plan 03 / LLM02   | Sensitive Information Disclosure                      | Covered for the user prompt and tool results (trace + macOS/Linux live campaigns); bounded evidence acceptance `RECORDED`; C1 unknown-format redaction `IMPLEMENTED` opt-in (PRs #361/#362/#364) with bounded Linux live `PASS`; focused C1 hardlink verification `PASS` for 4 scoped cases; Option C Phase 1 hardlink-alias denial `VERIFIED` on Linux and macOS | Not general DLP; four pre-C1 Linux residual exposures; hardlink aliases outside the identity registry (outside-workspace origins, Windows) with one pre-Option-C unlisted-value residual; Plan 03 acceptance `RECORDED` 16/16 (2026-11-12); C1 disabled by default; broader coverage `OPEN` |
| LLM03      | Plan 04 / LLM04   | Supply Chain                                          | Partial; scanner hash races closed and per-platform binary pins implemented                                                                                                                                                                                                                                                                                        | D4 enforcement and 44 proposed baselines await operator approval; one run_command residual is unaccepted; drift warnings are unreliable for partial/failed scans until the PR #472 P2 is fixed; publisher signatures deferred                                                                                                                                                                                                                   |
| LLM04      | Plan 05 / LLM05   | Data and Model Poisoning                              | Partial                                                                                                                                                                                                                                                                                                                                                           | Integrity proves bytes, not benign content; provider training external                                                                                                                                                                                                           |
| LLM05      | Plan 10 / LLM10   | Improper Output Handling                              | Covered for SOP's sinks                                                                                                                                                                                                                                                                                                                                           | Third-party rendering (terminal, IDE, Slack) external                                                                                                                                                                                                                            |
| LLM06      | Plan 01 / LLM03   | Excessive Agency                                      | Covered (policy) + default-on sandbox on macOS and Linux                                                                                                                                                                                                                                                                                                          | Windows B1 refuses every override value and is fixed since PR #441's P2; cold build cache per run; residual hardlink, parent-environment and host-to-sandbox socket exposure                                                                                                                                                                                                                                 |
| LLM07      | Plan 08 / LLM08   | System Prompt Leakage (repo: Hidden Context Exposure) | Partial                                                                                                                                                                                                                                                                                                                                                           | Prompt secrecy is not claimed by design                                                                                                                                                                                                                                          |
| LLM08      | Plan 09 / LLM09   | Vector and Embedding Weaknesses                       | Covered for the shipped (disabled-by-default) path                                                                                                                                                                                                                                                                                                                | One live producer run (2026-11-12, `rag-template-rust`, PostgreSQL 17 + pgvector, `nomic-embed-text`): isolation, scope, positive controls and a poisoned chunk observed; `TIMEOUT` and cancellation not observed live                                                                                                                                                                                                                                     |
| LLM09      | Plan 07 / LLM07   | Misinformation                                        | Partial, by scope                                                                                                                                                                                                                                                                                                                                                 | Factual correctness of model output is out of scope                                                                                                                                                                                                                              |
| LLM10      | Plan 06 / LLM06   | Unbounded Consumption                                 | Covered for time/output bounds, selected process-group termination, and opt-in token budgets                                                                                                                                                                                                                                                                      | No dollar cap (no pricing data); every `exec.CommandContext` subprocess path binds a process group (guard-enforced; not on Windows)                                                                                                                                              |

## Per risk

### LLM01 Prompt Injection — covered, bounded

- **Covered:** tool authorization, command policy and sandboxed live campaigns.
  On macOS, 4 of 4 injection cases were blocked and the positive control was
  allowed. On Linux arm64 with `qwen3.5:4b`, all 4 injection cases were denied
  and the positive control was allowed after the campaign disabled model thinking
  for forced JSON output (PR #422). Earlier timeout evidence remains preserved.
  A second macOS campaign with `llama3.2:3b` gave the same result: 4 of 4
  injection cases denied and the positive control allowed (PR #424).
  Evidence: [llm01-injection.md](llm01-injection.md).
- **Not covered, and why:** prompt injection cannot be prevented in general. The
  evidence is two models on macOS and one on Linux, run once per case, so it is
  not a claim about other models or providers. Linux has no second-model run.

### LLM02 Sensitive Information Disclosure — covered, bounded

- **Covered:** tool results and tool errors are redacted before they reach the
  model, and credential paths are denied for `read_file`, `search_files` and
  `copy_file` (Plan 08 A+B). Option C Phase 1 extends that denial to hardlink aliases
  of protected workspace credentials on Linux (`VERIFIED`) and macOS (`VERIFIED`,
  attempt 3). Since the task-input fix, the whole user prompt
  (`userPrompt`: task, caller input, output requirements) passes the same
  model-input redaction (`TestUserPromptRedactsTaskAndInput`). Diagnostics and
  notifications are redacted. A macOS live extraction campaign ran 7 of 7 cases;
  a Linux live tool-path campaign ran 17 cases (7 protected, 6 positive controls,
  4 residual exposures), a focused C1 hardlink-alias campaign ran 6 cases
  (4 protected, 1 positive control, 1 unlisted-value residual), and the Option C
  Phase 1 campaign ran 6 cases on Linux and again on macOS (5 protected, 1 positive
  control, 0 residual). Evidence:
  [llm02-disclosure.md](llm02-disclosure.md) and
  [llm02-acceptance-request.md](llm02-acceptance-request.md).
- **Not covered, and why:**
  - The task-input fix has one live re-run per platform, both with `qwen3.5:4b`:
    macOS arm64 and Linux arm64
    ([llm02-disclosure.md](llm02-disclosure.md#live-task-input-re-run-2026-11-11)).
    Linux `amd64` was not re-run, and six git-dependent cases are `NOT_RUN` on
    macOS because git cannot run inside the live profile.
  - Other model-input routes are inventoried in
    [llm08-hidden-context.md](llm08-hidden-context.md#model-input-route-inventory-owasp-gap-4).
    The Ollama system prompt carries no caller text. The decision transports
    (CLEF and Nimble) are redacted with the baseline pass only, and their choices
    and criteria are not scrubbed. The decision command provider and the Julia
    runner are not redacted by design.
  - Redaction is pattern-based, not general DLP, so secrets in an unknown
    format can pass. The Linux live campaign observed unknown-format values in an
    ordinary file, SAFE command output, and a diff reaching the model.
  - A pre-existing hardlink with an ordinary name bypassed the lexical credential
    path denial. Option C Phase 1 now denies aliases to protected _workspace_
    credentials on Linux (`VERIFIED`) and macOS (`VERIFIED`, attempt 3 artifact
    `llm02-option-c-phase1-macos-campaign-attempt3.json`, SHA-256 `50cc9e09…`,
    6 requests, 5 protected, 1 positive control, 0 residual). The focused C1
    verification (PR #373, revision `ed1ba11`, artifact SHA-256 `417599e2…`,
    6 requests) still records the pre-Option-C split: C1 redacts known,
    inventoried, and contextual values after the read, but the alias is readable,
    and one unlisted opaque value reached the model. **Path authorization and
    model-input redaction are separate controls.** Outside-workspace origins,
    credentials created outside the model tools after the registry snapshot,
    external removal before the first build, and Windows remain outside the control; they are accepted as bounded limits (LLM02 decision packet C.4, 2026-11-12; gap 5 option 1). `Nlink > 1`
    is not implemented and is not approved. The unlisted-value residual is unchanged
    and remains open: Option C denies aliases to protected workspace credentials, but
    an unlisted value in an ordinary file is still outside C1 and outside identity.
  - SAFE `run_command` (`go test` is `SAFE`) can run model-written code that
    reads `$HOME` credential files directly, or `os.Link`s them into the repo.
    With the `run_command` sandbox on (the default), macOS denies these reads and
    the `ln`, scrubs credential-named and credential-shaped env values, resolves
    the credential locations named by
    `KUBECONFIG`, `DOCKER_CONFIG`, `GNUPGHOME`, `CLOUDSDK_CONFIG`,
    `GH_CONFIG_DIR`, `XDG_CONFIG_HOME`, `CARGO_HOME`, `NPM_CONFIG_USERCONFIG`,
    `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE`, `AZURE_CONFIG_DIR`, `NETRC`, and `XDG_CONFIG_HOME/git/credentials`, and denies the
    `com.apple.SecurityServer` mach lookup so the Keychain is not reachable. The read-deny rule is now on by default (macOS
    `VERIFIED`, Linux `amd64` `VERIFIED` in PR #384; see
    [llm02-disclosure.md](llm02-disclosure.md) §Sandbox credential-read denial).
    Still open on macOS: the parent process environment is readable with
    `kern.procargs2` (measured; `process-info*` does not cover it; an intermediate
    scrubbed shell hides the direct parent but not the SOP process above it, so the
    limit stands; see [seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md#launch-chain-options-owasp-gap-6-2026-11-12)). A pre-existing
    hardlink alias to a protected workspace credential is denied on Linux and macOS
    by Option C Phase 1 (macOS attempt 3 `PASS`). A SAFE command can still read an
    alias to a credential outside the workspace, and Windows is `UNVERIFIED`. This
    is a bounded residual exposure, recorded by operator decision on 2026-11-12
    (gap 5, option 1). The `Nlink > 1` check is not implemented and not approved;
    Windows and outside-workspace origins are outside the control and are accepted as bounded limits (LLM02 decision packet C.4, 2026-11-12).
    The SAFE-command vector itself is now measured on both platforms: with isolation
    on, `TestDarwinIsolationSAFECommandCredentialRead` (new) and
    `TestLinuxIsolationSAFECommandCredentialRead` deny the read at the process
    level, and the macOS test also pins that the isolation-off default exposes the
    canary and that a pre-existing hardlink alias stays readable inside the sandbox.
  - The Linux live retrieval sink has one run
    ([llm02-disclosure.md](llm02-disclosure.md#linux-retrieval-sink-run-2026-11-12)).
    The Linux live notification sink has one run with a local receiver
    ([llm02-disclosure.md](llm02-disclosure.md#linux-notification-sink-run-2026-11-12));
    it does not cover Slack rendering or channel binding.
  - Model-output artifacts remain verbatim; retrieval/handoff/caller prose and
    provider retention remain separate boundaries.
  - Bounded evidence acceptance is `RECORDED` in repository decision memory ID
    `59d986984142bef314ea784a401b204cd57b7ca1664f570b4763c13a5f580c98`; it is a
    separate scoped decision, not Plan 03 per-criterion acceptance.
  - Plan 03 per-criterion acceptance was recorded on 2026-11-12: 13 manual
    criteria accepted and 3 check-mapped criteria check-passed (16/16). This is
    separate from the bounded evidence acceptance and does not close the
    residual risks in this section. Broader LLM02 coverage remains `OPEN`.
  - The six-item residual-risk decision matrix records that the PR #376 review
    did not approve Option C as proposed, that the operator later approved
    Option C Phase 1 (PR #381, Linux `VERIFIED`; macOS `VERIFIED` in attempt 3),
    and that a SAFE `run_command` credential read with isolation off is a
    separate unaccepted residual, now reachable only through the operator
    override; it recommends
    `INVESTIGATE` for
    unknown-format and artifact risks; `MITIGATE` for notification
    prose; `INVESTIGATE`/`ACCEPT_PROPOSED` for retrieval/handoff; and
    `TRANSFER_PROPOSED` for provider retention. Only the bounded subprocess hardlink alias is accepted (Option D, PR #387; see [PROPOSAL-LLM02-SUBPROCESS-HARDLINK.md](../../plans/PROPOSAL-LLM02-SUBPROCESS-HARDLINK.md)); no other residual risk is accepted.
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
    `toolResultMessage`) and, since the OWASP gap 3 change, to every agent chat
    transport the CLI builds (`agent.Redacted`, `TestRedactedBeforeChatTransports`
    for OpenAI-compatible and Ollama chat, C1 on and off). The baseline
    known-format pass now also runs on those transports, which previously had none.
    No live campaign covers them yet. The repository
    configuration remains disabled, so the pre-C1 residual remains until an
    operator enables it. The Linux live retrieval and notification sinks have one
    run each; the macOS sinks are `NOT_RUN`
    ([llm02-disclosure.md](llm02-disclosure.md#linux-notification-sink-run-2026-11-12))
    that does not cover Slack rendering or channel binding.

### LLM03 Supply Chain — partial

- **Covered:** scanner archives are SHA-256-pinned in CI. Optional
  `binary_sha256` and `rules_sha256` pins are checked before every scanner run,
  and this repository enables the Semgrep rules pin (PR #338). Per-platform binary
  pins are committed for Gitleaks 8.30.1 and OSV-Scanner 2.7.0 on linux and darwin
  (amd64+arm64) (PR #461). PR #410 closes the
  binary hash-then-exec race by executing a verified private copy
  (`TestPinnedExecutor`); `stageRules` does the same for the rules file or tree,
  so Semgrep and Gitleaks read verified private bytes
  (`TestRulesPinStagesVerifiedTree`, `TestSemgrepRulesPinStagesTheTreeItRuns`,
  `TestGitleaksRulesPinStagesTheConfigItRuns`). On macOS arm64, a real Semgrep
  1.183.0 scan under the rules pin reported `COMPLETED` with `COMPLETE` coverage,
  and one changed rule file failed closed with `rules pin mismatch`. Evidence:
  [llm03-scanner-integrity-evidence.md](llm03-scanner-integrity-evidence.md),
  [llm04-supply-chain.md](llm04-supply-chain.md).
- **Not covered, and why:**
  - SOP cannot verify the provenance of a provider's hosted model.
  - Publisher signatures are not checked; a digest proves the bytes, not who
    produced them.
  - Cargo-audit and Semgrep have no committed execution-time binary pin (Semgrep
    is a venv; cargo-audit's manifest covers only linux x64 and darwin arm64);
    Windows is not pinned.
  - The repository runs report-only with empty `required_categories`; enabling a
    blocking gate is an operator decision
    ([PROPOSAL-SCANNER-PIN-POLICY.md](../../plans/PROPOSAL-SCANNER-PIN-POLICY.md)).

> **Current scanner-enforcement status (PRs #471–#474):** required categories
> remain disabled; the scan is report-only. The reproduced full-workspace baseline is
> **106 findings, 40 suppressed and 50 blocking** — the original 45 blocking findings
> (with the residual now at `internal/toolharness/harness.go:926`,
> `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771`), plus
> five Gitleaks fixture findings that the decision documents introduced themselves.
> Those five are synthetic and await a redact-or-baseline decision; they are listed in
> the [operator packet](../../plans/PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md).
> The advisory unmatched-baseline diagnostic is verdict-neutral and now reports only
> when a full-workspace scan observed every adapter the baseline relies on, so
> changed-files, partial, failed, disabled and indeterminate scans stay silent.

### LLM04 Data and Model Poisoning — partial

- **Covered:**
  - Provenance on decision memory.
  - RAG `chunk_hash` verification (PR #330). Per-row index digests (schema 2) exist,
    but no production code reads `context/index.json` back, so they are latent:
    `TestPersistedRepoIndexReadersVerifyRows` fails if a reader skips `repoindex.VerifyRows`.
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
  persisted Markdown, and Slack unfurl are outside SOP. A plain `@here` in Slack
  payload text notifies the channel (observed 2026-11-12; the encoder escapes `&`,
  `<` and `>` only). Accepted as a documented residual by the operator (2026-11-12): it
  causes channel noise, not data exposure. See the observation page. The operator observed a
  plain `sop notify test` event reach the intended Slack channel with literal text
  and no unexpected unfurl or mention (PR #412). The fixed test event contains no
  markup, link, or mention-shaped canary, so escaped-markup rendering remains
  `NOT_OBSERVED`. All six Plan 10 manual criteria were accepted, and Plan 10 was
  historicalized `COMPLETE`.

### LLM06 Excessive Agency — covered by policy, with default-on sandboxing on macOS and Linux

- **Covered:**
  - Root and command policy, with read-only roots.
  - OS isolation on the replay and evaltarget paths, on both macOS and Linux.
  - Since PR #343 (macOS, Seatbelt) and the bubblewrap follow-up (Linux),
    `run_command` runs inside an OS sandbox by default. In it, outbound network is
    blocked, writes are limited to the root and temp, `.agent-sdlc/` is denied, and
    it fails closed when no sandbox is available. Each run gets its own throwaway
    build cache and the shared module cache is read-only, so one run cannot poison
    a later run's Go caches.
  - Where a backend exists (macOS, Linux), the operator override
    `SOP_AGENT_RUN_COMMAND_ISOLATION=off` (also `0`, `false`, `no`) is the only way
    to run commands with policy checks alone, and `sop run` and `sop prompt` print
    a startup notice when it is set. The override is operator-scoped: a model
    cannot set its own parent's environment.
  - A platform with no backend refuses `run_command` rather than running it
    unsandboxed (Windows B1, selected by the operator). The refusal is
    unconditional there: no override value and no `Config.IsolateCommands` value
    can turn it into policy-only execution, and the startup notice is suppressed
    because the override does nothing. The PR #441 P2 recorded in #442 is fixed
    and pinned by `TestIsolationDecisionUnsupportedPlatforms` and, at runtime on
    Windows, `TestRunCommandIsolationOnWindowsRefuses`.

  Evidence: [llm03-agency.md](llm03-agency.md),
  [seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md#run_command-opt-in-sandbox-llm03).

- **Not covered, and why:**
  - Windows has no backend, so `run_command` is unavailable there under B1. Every
    override value and both `Config.IsolateCommands` values are refused; the
    compatibility cost, acknowledged with the decision, is that the agent's
    build/test loop and any validation that shells out cannot run on Windows.
    `VERIFIED` at runtime in CI on the merged implementation. Decision and impact:
    [PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md](../../plans/PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md),
    [llm06-sandbox-default-measurements.md](llm06-sandbox-default-measurements.md).
  - Platform coverage is uneven: macOS arm64 and Linux arm64 (privileged
    container) are `VERIFIED`; a Linux distribution host and Linux `amd64` are
    `NOT_RUN`. The container results are not evidence of real-host compatibility.
  - The operator override restores policy-only execution on request. Under it the
    direct-read vector is open again, visibly, and the residual exposures listed
    below stay open either way.
  - The sandbox wraps `run_command` only. Validation commands
    (`internal/testrunner`, `sh -c`), security scanners, and the `git`/`gh` sinks
    are not sandboxed and this row makes no claim about them.
  - With the sandbox on, the repository's own suite passes on macOS (87-91 s
    in-sandbox against 86 s unsandboxed, no failures) and has no sandbox-caused
    failures on Linux arm64 in a container (two container artifacts remain). Tests
    that need a nested sandbox or the process table skip with an explicit reason
    (Seatbelt cannot nest, nested bubblewrap cannot mount `/proc`, and a sandbox
    refuses setuid `ps`), so those inner boundaries are covered only outside the
    sandbox.
  - AF_UNIX `bind` is allowed only for socket paths under a writable dir; outbound
    AF_UNIX and non-loopback IP network access stay denied. A same-user host
    process can therefore dial a socket the sandbox created, which is a new
    direction and not a closed boundary.
  - On Linux the sandbox uses its own network namespace, so a test cannot
    reach a server on the host's loopback; one it starts itself works. Host
    Unix sockets under `/tmp` and `/run` are hidden. PR #421 also masks sockets
    found under the Linux home directory with a bounded, fail-closed scan.
    Sockets outside those masked roots remain connectable.
  - Turning the sandbox on does not close everything. A pre-existing hardlink
    alias to a `$HOME` credential stays readable on macOS and Linux (measured),
    and on macOS the parent process's launch-time environment is readable through
    `kern.procargs2` (measured), so a credential in SOP's own environment is still
    reachable from a sandboxed command.
  - The agent process itself runs outside SOP.
  - Every sandboxed build starts with a cold build cache (the measured overhead
    for the full suite is 1-6 s, not the earlier ~10 s for three packages), and a
    module missing from the module cache cannot be downloaded inside the sandbox;
    run `go mod download` first.

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
- **Live evidence:** one bounded run against a disposable PostgreSQL with pgvector
  and a local embedding model. Cross-corpus isolation held, an unauthorized corpus
  was rejected, the positive controls returned their documents, and an embedded
  instruction arrived as untrusted evidence text. Evidence:
  [llm09-retrieval.md](llm09-retrieval.md#live-producer-run-owasp-gap-14-2026-11-12),
  [llm09-live-rag-campaign.json](llm09-live-rag-campaign.json).
- **Not covered, and why:** one producer, one embedding model, one run, synthetic
  data. `TIMEOUT` and cancellation were not observed live. Semantic poisoning
  inside an authorized repository is not prevented (see LLM04).

### LLM09 Misinformation — partial, by scope

- **Covered:** evidence used in decisions is bound to real command output and
  to the reviewed repository content. A `go test -run` selection that matched no
  tests is flagged, so its exit code 0 is not read as tests passing. Evidence:
  [llm07-misinformation.md](llm07-misinformation.md).
- **Not covered, and why:** SOP checks that decisions rest on real evidence. It
  does not check that model prose is factually correct, which cannot be decided
  mechanically. With isolation on by default, a `run_command` subprocess cannot
  write SOP state (`TestSubprocessStateWriteIsDeniedByDefault`); the exposure
  returns only through the operator override
  ([llm07-state-write-detection-phase1.md](llm07-state-write-detection-phase1.md)).
  A bounded live run on Linux and macOS shows the opt-in `run_command` sandbox
  denies a state write from the allowed test command, and that the write goes
  through with the sandbox off (
  [llm07-state-write-detection-phase1.md](llm07-state-write-detection-phase1.md#live-evidence-gap-15-2026-11-12)).
  Isolation is on by default (PR #441); the residual applies only under the
  operator override `SOP_AGENT_RUN_COMMAND_ISOLATION=off`.

### LLM10 Unbounded Consumption — covered for time, output and process bounds

- **Covered:**
  - Read and output bounds, and a default cap on provider output tokens.
  - Per-command timeouts, MCP frame bounds and atomic continuation accounting.
  - Process-group termination covers the paths that bind `internal/procgroup`
    (test runner, handoff, HIP comparison, outcome/acceptance commands, review
    OCR, RAG client, Julia runner, dqharness replay, git and gh invocations) and
    packages with their own `Setpgid` helpers (`agent`, `security`,
    `toolharness`, `evaltarget`, `controller/sopclient`, and
    `decision/commandprofile`, and `decision/command`, which SEAM-005 allows to
    import `internal/procgroup` by operator approval). A guard test
    (`TestCommandContextBindsProcessGroup`) fails when a new `exec.CommandContext`
    file binds no group. The test runner has a 30-minute default timeout.

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

1. The remaining LLM02 risk decisions. Plan 03 per-criterion acceptance is
   recorded (16/16, 2026-11-12); see
   [llm02-decision-packet.md](llm02-decision-packet.md).
2. Whether to add a one-shot Slack probe for escaped markup and mention-shaped
   strings; plain rendering and channel binding are already observed in PR #412.
3. The macOS live notification and retrieval sinks (`NOT_RUN`), and a second
   live RAG deployment. The Linux sinks have one run each; the notification run
   covers a local receiver only.
4. No LLM06 residual decision remains open. R2 and R4 are recorded as
   bounded/known limits and H2S as a bounded architectural property under Option A
   (decision memory `a73749c2…`, 2026-11-12); none of the three is a risk
   acceptance, and the Option-D subprocess hardlink alias is accepted within its
   bounded scope. Tracking:
   [PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md](../../plans/PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md).

## Code follow-ups worth doing

- No dollar-cap follow-up is active: the operator selected token-budget-only
  option A in PR #417. Reconsidering it requires a new policy decision.
