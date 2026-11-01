# Promptfoo behavioral evaluation (security.evaluation)

This page documents SOP's optional AI behavioral evaluation adapter. It is the
single reference for the configuration keys, the normalized report schema, the
command exit behavior, and the outbound/telemetry behavior of the pinned
Promptfoo release.

The adapter is downstream and read-only (`internal/security/contract.go`). It
never feeds lifecycle, routing, retry, budget, or human-boundary decisions.

## Installation

SOP never downloads or updates Promptfoo during ordinary execution. Install the
pinned release once, out of band, then leave it alone:

```sh
npm install -g promptfoo@0.118.0
promptfoo --version   # must print 0.118.0
```

Prerequisites:

- **Node 22 is the supported runtime for this pin.** The pinned `0.118.0`
  package loads a native `better-sqlite3` module built for the Node 22 ABI
  (`NODE_MODULE_VERSION` 127). Node 24 failed in this environment because it
  requires ABI 137; the earlier verification also found Node 26 unusable. Put
  the Node 22 `bin` directory first on `PATH` for every scan (on Homebrew:
  `/opt/homebrew/opt/node@22/bin`):

  ```sh
  PATH="/opt/homebrew/opt/node@22/bin:$PATH" promptfoo --version
  ```

- **The local model must already exist.** For Ollama, `ollama list` must show
  the configured target model (for example `qwen3.5:4b`) and the Ollama
  endpoint must be reachable. The default endpoint is
  `http://127.0.0.1:11434`; if `OLLAMA_BASE_URL` is set to anything else, the
  scan uses that value.
- **No credential belongs in configuration.** Provider and model identifiers
  are not secrets. Keep any credential in the environment, never in a tracked
  file.

The pinned version is the `defaultSecurityEvalPromptfooVersion` constant in
`internal/config/security_eval.go`. The adapter runs the `promptfoo` binary
directly, never through `npx`, and sets `PROMPTFOO_DISABLE_UPDATE=true`. A
missing binary or a version mismatch is an execution error
(`internal/security/promptfoo.go`); nothing is installed or upgraded.

## Configuration keys

The block lives under `security.evaluation` and is disabled unless
`enabled: true`.

| Key                       | Default                                   | Environment                                           | Description                                                              |
| ------------------------- | ----------------------------------------- | ----------------------------------------------------- | ------------------------------------------------------------------------ |
| `enabled`                 | unset (disabled)                          | `SOP_SECURITY_ENABLED` enables the parent add-on only | Turns the evaluation adapter on.                                         |
| `profile`                 | `local`                                   | —                                                     | `local` or `remote`; `local` never falls back to a hosted provider.      |
| `target`                  | — (required when enabled)                 | —                                                     | Model identifier under test. Never a credential.                         |
| `promptfoo_version`       | `0.118.0`                                 | —                                                     | Pinned Promptfoo release; any other value is a load error.               |
| `install_reference`       | `docs/security-promptfoo.md#installation` | —                                                     | Documentation anchor for the install step. A reference, never a command. |
| `target_provider`         | — (required for `local`)                  | —                                                     | Recorded provider identity for the target.                               |
| `attack_provider`         | — (required for `local`)                  | —                                                     | Recorded provider identity for attack generation.                        |
| `grading_provider`        | — (required for `local`)                  | —                                                     | Recorded provider identity for grading.                                  |
| `config_path`             | — (required when enabled)                 | —                                                     | Repository-relative Promptfoo config file.                               |
| `dataset_path`            | unset                                     | —                                                     | Optional dataset; the dataset hash is `UNAVAILABLE` when unset.          |
| `request_limit`           | `10`                                      | —                                                     | Bound on evaluated cases; extra cases become `skipped`.                  |
| `concurrency`             | `4`                                       | —                                                     | Parallel case bound, passed as `--max-concurrency`.                      |
| `process_timeout_seconds` | `security.timeout_seconds`, else `900`    | —                                                     | Bound on one Promptfoo invocation.                                       |
| `remote_provider`         | `false`                                   | —                                                     | Acknowledges a remote provider; rejected for `local`.                    |
| `data_handling`           | `local-only`                              | —                                                     | `local-only` or `remote-allowed`; `local` requires `local-only`.         |
| `telemetry_disabled`      | forced `true` for `local`                 | —                                                     | Disables Promptfoo telemetry and sharing.                                |
| `report_only`             | `false`                                   | —                                                     | Records findings without blocking the workflow.                          |
| `severity_mode`           | `none`                                    | —                                                     | `none` never assigns a CVSS or automatic severity.                       |

Credentials never appear in configuration. The schema has no credential field;
provider and model identifiers are stored, secrets stay in the environment.
`IsCredentialKey` in `internal/config/security_eval.go` names the keys that
would commit a credential.

### Enabling and disabling

The evaluation adapter is inert until both the parent security add-on and the
`security.evaluation` block are enabled. Put machine-specific values in the
git-ignored `.agent-sdlc/config.yaml`, not in a tracked configuration file.

A complete local, report-only example:

```yaml
security:
  enabled: true
  mode: report-only
  scope: workspace
  timeout_seconds: 300
  # Optional: isolate the evaluation adapter from the add-on's default
  # semgrep/gitleaks selection. Remove this to keep the default scanners.
  categories:
    sast: false
  evaluation:
    enabled: true
    profile: local
    target: "ollama:chat:qwen3.5:4b"
    target_provider: "ollama:chat:qwen3.5:4b"
    attack_provider: "ollama:chat:qwen3.5:4b"
    grading_provider: "ollama:chat:qwen3.5:4b"
    promptfoo_version: "0.118.0"
    config_path: "evals/promptfoo/promptfooconfig.yaml"
    data_handling: local-only
    remote_provider: false
    telemetry_disabled: true
    report_only: true
    request_limit: 5
    concurrency: 1
    process_timeout_seconds: 300
    severity_mode: none
```

The `target`, `target_provider`, `attack_provider`, and `grading_provider`
values are recorded as identity and provenance. The local profile requires all
three provider keys even when the referenced Promptfoo config is a single
target-only smoke test. In that case no attack or grading provider is actually
invoked; see [Coverage levels](#coverage-levels).

These keys are **not injected** into the Promptfoo config. The file named by
`config_path` is authoritative for which provider runs. For a `local` trial,
inspect that file and confirm every provider it names is local; the SOP config
cannot rewrite or sandbox a config that names a remote provider.

Enabling `security.enabled: true` also applies the add-on's existing defaults.
If `security.adapters` is omitted, the built-in `semgrep` and `gitleaks`
adapters are selected. Semgrep fails closed when no local rules are configured,
which makes the overall scan verdict `INDETERMINATE` even when the Promptfoo
adapter passes. For a bounded evaluation-only trial, disable that category
explicitly (`categories: {sast: false}`) or configure local Semgrep rules.

To disable only the evaluation adapter while keeping the rest of the add-on:

```yaml
security:
  evaluation:
    enabled: false
```

To disable the whole add-on:

```yaml
security:
  enabled: false
```

Changing `security.evaluation.enabled` never changes the approval gate, routing,
or `security.mode`; `report_only` is independent of the add-on's mode.

## Invocation

Run from the repository root with the Node 22 runtime and the local endpoint:

```sh
PATH="/opt/homebrew/opt/node@22/bin:$PATH" \
OLLAMA_BASE_URL="http://127.0.0.1:11434" \
sop security scan --report-only
```

`OLLAMA_BASE_URL` is passed through the scanner environment because the scanner
strips provider-looking variables. It is not a credential, and a `local`
profile must not point it at a remote host. Validate the effective add-on
selection without executing a scanner with:

```sh
sop security inspect
```

`security inspect` reports the selected scanner adapters; the evaluation
adapter is appended at scan time and is validated when the config is loaded.

## Normalized report

`internal/security/evalreport.go` defines the normalized report written to
`<run dir>/security/promptfoo.case.json`, where `<run dir>` is
`.agent-sdlc/runs/<run-id>`. The control-character-sanitized Promptfoo output is
written next to it as `promptfoo.raw.json`, even when parsing fails, so the
failure stays inspectable. The runner also persists the same sanitized native
report as `promptfoo.native.json` through the ordinary adapter artifact path.

The CLI passes the run directory to the adapter. If an embedder calls the
adapter directly, it must set `Request.OutputPath` to the run directory; an
empty value skips the adapter-owned normalized and raw files and leaves only
`promptfoo.native.json`.

Every required field is populated or explicitly marked `UNAVAILABLE`:

| Field                                              | Meaning                                                                             |
| -------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `protocol_version`                                 | Adapter contract version.                                                           |
| `run_id`                                           | Run identifier.                                                                     |
| `task_id`                                          | Task identifier, when known.                                                        |
| `sop_revision`                                     | SOP revision under test, or `UNAVAILABLE`.                                          |
| `working_tree`                                     | Commit, tree, digest, and dirty marker.                                             |
| `promptfoo_version`                                | Pinned Promptfoo release.                                                           |
| `target_model`                                     | Target model identifier.                                                            |
| `attack_provider` / `grading_provider`             | Provider identifiers.                                                               |
| `config_hash` / `dataset_hash`                     | SHA-256 of the exact inputs used, or `UNAVAILABLE`.                                 |
| `profile` / `data_handling` / `telemetry_disabled` | Effective policy.                                                                   |
| `request_limit` / `concurrency`                    | Effective bounds.                                                                   |
| `started_at` / `ended_at` / `duration_ms`          | Timing.                                                                             |
| `completion`                                       | `COMPLETE`, `PARTIAL`, `FAILED`, or `SKIPPED`.                                      |
| `report_only`                                      | Echo of the configured mode.                                                        |
| `cases`                                            | Per-case `case_id`, `status`, `finding`, `message`, `duration_ms`, `evidence_link`. |
| `execution_error`                                  | Adapter execution error, kept separate from findings.                               |
| `verdict`                                          | `PASS`, `FAIL`, `INDETERMINATE`, `NOT_APPLICABLE`, or `REPORT_ONLY`.                |

Report files for a scan (`<run dir>/security/`):

- `scan.json` / `scan.md`: the normalized security scan and its human summary.
- `promptfoo.case.json`: the normalized evaluation report (cases, completion,
  verdict, config hash).
- `promptfoo.raw.json`: the control-character-sanitized, size-bounded raw
  Promptfoo results envelope.
- `promptfoo.native.json`: the runner-persisted copy of the same sanitized raw
  envelope. It is not a separate secret-scanned report.

Per-case status is exactly one of `passed`, `failed`, `error`, or `skipped`. A
timeout, unavailable provider, missing artifact, parsing failure, or skip never
becomes `passed`. There is no computed CVSS or severity field for behavioral
failures: a finding carries `severity` only when `severity_mode: explicit` and
the raw metadata names one of the normalized labels.

Evidence links point at the normalized report. The normalized report carries no
credential field. `promptfoo.raw.json` and `promptfoo.native.json` are the same
control-character-sanitized, size-bounded copy of the Promptfoo results
envelope; Promptfoo is not a secret scanner, so model output and Promptfoo
config values are retained as returned. Do not put credentials in the prompt,
dataset, or `config_path`. The adapter strips provider-looking variables from
the child environment, but it cannot redact a secret that a model echoes.

## Command exit behavior

Run the adapter through the security add-on command:

```sh
sop security scan --report-only
sop security scan --required
```

| Outcome                                                                                                                         | `report-only`                                        | `required` |
| ------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- | ---------- |
| Normal success (all cases passed)                                                                                               | exit 0                                               | exit 0     |
| Findings recorded                                                                                                               | exit 0 (non-blocking, findings in the report)        | non-zero   |
| Execution error (timeout, unavailable provider, missing artifact, parse failure, missing or mismatched Promptfoo, errored case) | non-zero, visible in output, verdict `INDETERMINATE` | non-zero   |
| Evaluation disabled                                                                                                             | exit 0, `NOT_APPLICABLE`, completion `SKIPPED`       | exit 0     |

An execution error can never yield a successful evaluation verdict in either
mode. In report-only mode a finding is recorded without blocking the ordinary
workflow.

The table describes the evaluation adapter's own behavior. The overall `sop
security scan` verdict is computed across every selected adapter, so an
unrelated adapter failure (for example, Semgrep with no local rules) makes the
scan `INDETERMINATE` even when `promptfoo.case.json` says `PASS`. Read the
per-adapter entries in `scan.json` and the evaluation report together.

## Outbound behavior and telemetry

Every Promptfoo invocation runs with a fixed environment
(`promptfooEnv` in `internal/security/promptfoo.go`):

```
PROMPTFOO_SELF_HOSTED=true
PROMPTFOO_DISABLE_TELEMETRY=true
PROMPTFOO_DISABLE_UPDATE=true
PROMPTFOO_DISABLE_REMOTE_GENERATION=true
PROMPTFOO_DISABLE_REDTEAM_REMOTE_GENERATION=true
```

Known outbound destinations:

- `local` profile: the SOP configuration rejects `remote_provider: true`,
  requires `data_handling: local-only`, and records the configured provider
  identities. The adapter does not select or rewrite providers inside
  `config_path`; whatever that file names is what Promptfoo runs. For the
  shipped smoke config the only provider is `ollama:chat:qwen3.5:4b`. An
  executor without explicit environment support is a fail-closed execution
  error, so SOP never claims telemetry or sharing was disabled without having
  set it.
- `remote` profile: only the providers declared in the Promptfoo config, after
  `remote_provider: true` and `data_handling: remote-allowed`; the
  `attack_provider`, `grading_provider`, and `target_provider` values are
  recorded identities. Telemetry, sharing, remote generation, and update checks
  stay disabled.
- Share links: never requested. The adapter does not pass `--share`, and the
  `0.118.0` release has no `--no-share` flag; sharing stays off unless the
  operator's Promptfoo config opts in. `PROMPTFOO_SELF_HOSTED=true` and the
  telemetry-disable environment are still set.
- Update checks: disabled by `PROMPTFOO_DISABLE_UPDATE=true`; SOP also never
  invokes a package manager.

Any further outbound call made by the pinned release itself is not verified for
this pin and must be treated as `UNAVAILABLE` evidence until an operator checks
the release notes for `0.118.0`.

This is configuration and process-environment evidence, not a packet capture.
The bounded live trial did not verify network isolation, so "no outbound
traffic" remains unproven beyond the explicit local provider selection and the
telemetry-disable environment above.

## Isolation boundary

This boundary belongs to the deterministic AISEC case runner. The Promptfoo
adapter does not route its live `promptfoo eval` process through `evaltarget`;
the live smoke test is not network-isolated. Do not cite this section as proof
that a live evaluation had no outbound traffic.

### Replay isolation (dqharness)

`IsolatedReplay` in `internal/dqharness/replay_exec.go` runs a verification
command under OS-level isolation. On Linux it uses `bubblewrap` (`bwrap`) with a
restricted filesystem view:

- only these read-only system and toolchain paths are bound, with
  `--ro-bind-try` so optional or symlinked paths are tolerated: `/usr`, `/bin`,
  `/sbin`, `/lib`, `/lib64`, `/etc`, `/sys`;
- `/proc` is a fresh procfs mount (`--proc /proc`) and `/dev` is a fresh devtmpfs
  mount (`--dev /dev`), so toolchains get a usable `/proc` and `/dev`;
- `/dev/shm` is a tmpfs remounted read-only (`--tmpfs /dev/shm` followed by
  `--remount-ro /dev/shm`);
- the canonical replay directory is bound writable at the fixed sandbox path
  `/replay` (`--bind <canonical replay dir> /replay`) and the command changes into
  it (`--chdir /replay`).

Host `/tmp`, `/run`, `/var/run`, `/home`, `/root`, `/opt`, `/srv`, `/mnt`,
`/media`, and other user-writable locations are absent from this view. Host Unix
domain sockets in those locations are therefore unreachable from inside the
sandbox, even though `--unshare-net` only governs TCP/IP. The replay directory is
the only writable filesystem location.

Writable temp storage is `/replay/.tmp`, created inside the replay directory
before launch (mode `0700`). `TMPDIR` points there on both backends, so
`os.CreateTemp` and `go test` keep temporary files inside the replay directory
and never in a host temp root or a private writable mount outside it. The child
environment sets `PATH` with `/usr/local/go/bin` first, `HOME=/replay`,
`TMPDIR=/replay/.tmp`, and `GOTOOLCHAIN=local`, and keeps `--unshare-net`,
`--unshare-pid`, `--unshare-ipc`, `--unshare-uts`, `--die-with-parent`,
`--new-session`, and `--clearenv`. A canonicalization failure returns an
actionable error and never launches a child with a raw path.

On macOS, `IsolatedReplay` uses the Seatbelt (`sandbox-exec`) profile. macOS
Seatbelt runtime isolation remains command-construction-only: argument and
profile construction is tested, but no macOS runtime isolation is claimed here.

The S2 target enforces the boundary in `internal/evaltarget`:

- Cases fail closed unless the process was launched through
  `evaltarget.IsolateCommand`, which marks the child with
  `SOP_EVAL_ISOLATION`.
- macOS uses `/usr/bin/sandbox-exec` with a deny-by-default profile: the case
  root and the executable are the only readable user paths; writes are limited to
  the case root and the OS temp areas; `/Users` (the ordinary workspace and home)
  is denied; network access is denied.
- Linux uses `bubblewrap` (`bwrap`) with a private mount and network namespace
  when it is installed. It is not exercised on this macOS host.
- Other platforms, or a host without the platform sandbox, fail closed rather
  than claiming isolation.
- Child commands are confined to the sandbox root, inherit `IsolateEnv` (secrets
  removed, `HOME`/`TMPDIR`/`SOP_STATE_DIR` pointed at the case), and use the same
  process-group cancellation path. The recording effect boundary is the only
  accepted effect implementation (`RecordingBoundary`).

Deterministic macOS tests prove: a write to the ordinary workspace and a read of
`.agent-sdlc/state.db` are denied; a connection to a reachable local listener is
denied; a production-looking secret in the parent environment is absent from the
child. The OS temp areas remain writable by design, and an in-process case body is
only acceptable because the process itself is inside the OS sandbox.

## Coverage levels

Do not treat a deterministic fixture as live-model evidence. The levels are:

| Level                    | Evidence                                                                                                       | Proves                                                                                                 | Does not prove                                                                                |
| ------------------------ | -------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------- |
| Deterministic fixtures   | `internal/evaltarget/aisec_test.go`, `internal/security/promptfoo_test.go`                                     | Parsing, policy, environment, artifact writing, and case classification.                               | Any model behavior.                                                                           |
| Real-control integration | `internal/evaltarget/production_controls_test.go`                                                              | Production authorization and command policy deny/allow the six cases correctly.                        | Any model behavior.                                                                           |
| Lifecycle verification   | `internal/cli/aisec_integration_test.go`                                                                       | A trusted-harness completion claim without execution evidence is refused.                              | Any model behavior.                                                                           |
| Live smoke test          | `internal/security/promptfoo_live_test.go` and `sop security scan` with `evals/promptfoo/promptfooconfig.yaml` | The pinned binary loads, a real local model answers one prompt, and the normalized report is complete. | Adversarial behavior, attack generation, or grading; it is one prompt and one assertion.      |
| Live adversarial         | Not implemented                                                                                                | Nothing.                                                                                               | The six AISEC scenarios through a live model; attack and grading providers are not exercised. |

## Initial AISEC suite

`internal/evaltarget/aisec.go` defines the bounded suite. AISEC-001, 003, 004,
and 005 are blocked attempts (approval, advisory-memory authorization, protected
secret, task scope); AISEC-002 rejects a completion claim with no executed
evidence; AISEC-006 is the authorized path.

Coverage is deliberately separated:

- **Evaluator unit tests** (`aisec_test.go`) use fixture controls. They prove the
  recording log, selective detection, and permissive-control behavior.
- **Real-control integration** (`production_controls_test.go`) uses a real
  `approval.Service` and `commandpolicy.Classify`. AISEC-001/003/004 are denied by
  authorization, AISEC-005 by command policy, and AISEC-006 executes with an
  approved target and a safe command. The harness does not duplicate either
  policy.
- **Real verification** (`internal/cli/aisec_integration_test.go`) submits a
  trusted-harness completion claim without execution evidence to the production
  lifecycle. The final verdict is `CONTINUE`, the report is not `PASS`, and the
  persisted gate is reconciled to `CONTINUE` rather than a successful completion
  gate. This proves refusal of a passed verdict; it is not a claim that every
  possible false claim is blocked outside the lifecycle.
- **Live-model adversarial evaluation** is not implemented. The live invocation
  below is a smoke-test path check, not the six AISEC scenarios. The smoke test
  does not generate attacks or run a grading provider.

## Live verification

The live check is opt-in and separate from the deterministic suite:

```sh
go test -tags=promptfoo_live ./internal/security/ -run TestPromptfooLiveInvocation -count=1 -v
```

The live test sets `OLLAMA_BASE_URL` and prepends Node 22 to `PATH`; it uses
`evals/promptfoo/promptfooconfig.yaml` and the local `ollama:chat:qwen3.5:4b`
provider. The build tag keeps the ordinary deterministic suite free of a live
model call. `OLLAMA_BASE_URL` is passed through because the scanner environment
strips provider-looking variables.

The same smoke path through the product command:

```sh
PATH="/opt/homebrew/opt/node@22/bin:$PATH" \
OLLAMA_BASE_URL="http://127.0.0.1:11434" \
sop security scan --report-only
```

A bounded local trial executed one case: `successes: 1`, `failures: 0`,
`errors: 0`, provider `ollama:chat:qwen3.5:4b`, response
`SOP_PROMPTFOO_LIVE_OK`, completion `COMPLETE`, verdict `PASS`, a real SHA-256
`config_hash` (not `UNAVAILABLE`), and `telemetry_disabled: true`. The
`promptfoo` adapter entry was `COMPLETED` with `COMPLETE` coverage.

The overall `sop security scan` verdict was `INDETERMINATE` in both
configurations. With the add-on defaults, the `semgrep` adapter failed with
"semgrep rules are not configured". With `categories: {sast: false}`, the scan
recorded disabled sast coverage and returned `PARTIAL` coverage. Neither reason
is an evaluation failure; read the per-adapter entries in `scan.json` together
with `promptfoo.case.json`.

Promptfoo's disk cache is enabled by default, and the adapter does not pass
`--no-cache`. A cached case can report a few milliseconds while a real local
generation takes seconds, and the normalized report does not record cache
state. For a true live timing check, run the pinned CLI directly with the
supported cache option:

```sh
PATH="/opt/homebrew/opt/node@22/bin:$PATH" \
OLLAMA_BASE_URL="http://127.0.0.1:11434" \
promptfoo eval -c evals/promptfoo/promptfooconfig.yaml \
  --no-cache --filter-first-n 1 --max-concurrency 1 \
  --output /tmp/promptfoo-nocache.json --no-progress-bar
```

A controlled comparison in this environment produced: cached case `2ms` with
`249` completion tokens; `--no-cache` case `5.7s` provider latency, `6.3s` wall
time, `240` completion tokens, still `PASS` with response
`SOP_PROMPTFOO_LIVE_OK`. Treat the cached latency as cache evidence, not model
latency.

This trial is **live smoke-test evidence**, not adversarial evidence. The
reused config sends one prompt and checks one substring. It does not generate
attacks, does not run a grading provider, and is **not** evidence that the six
AISEC fixtures passed through a live model. The `attack_provider` and
`grading_provider` settings are recorded identities only for this config.

`promptfoo` 0.118.0 writes the 0.118 results envelope (`results` is an object
containing `results` and `stats`); the parser accepts that envelope and the legacy
bare array. The release also exits `100` for failed tests; the adapter treats that
as a finding and parses the report, while other non-zero exits remain execution
errors.

## Troubleshooting

- **`ERR_DLOPEN_FAILED` / `NODE_MODULE_VERSION` mismatch.** Promptfoo's native
  `better-sqlite3` dependency was built for Node 22. Put Node 22 first on `PATH`
  and rerun. Do not use Node 24 or 26 for this pin.
- **`promptfoo version ... does not match pinned ...`.** Install exactly
  `promptfoo@0.118.0`. The adapter rejects another version and never upgrades or
  downloads it. Do not invoke it through `npx`.
- **`promptfoo is not installed locally`.** Install the pin as shown in
  [Installation](#installation); a scan never installs it.
- **`promptfoo exceeded the process timeout`.** Check the local endpoint and
  model first. Only then raise `process_timeout_seconds`; the default is
  `security.timeout_seconds`, else `900`.
- **`provider unavailable`, a case `error`, or an `INDETERMINATE` evaluation.**
  Confirm `ollama list` shows the configured model, `OLLAMA_BASE_URL` points at
  the local endpoint, and the Ollama server is running. An errored case is an
  execution error, never a pass.
- **Missing `promptfoo.case.json` or `promptfoo.raw.json`.** Confirm the CLI
  wrote `<run dir>/security/`. An embedder must set `Request.OutputPath` to the
  run directory; otherwise only `promptfoo.native.json` is persisted.
- **Overall scan is `INDETERMINATE` while `promptfoo.case.json` is `PASS`.**
  Another selected adapter failed or its coverage is disabled. Read `scan.json`
  and `scan.md`. With the add-on's default Semgrep adapter and no local rules,
  set `security.categories.sast: false` or configure `rules`; disabling the
  category removes the execution error but still records partial coverage.
- **The scan takes about two minutes although the model case is fast.** The
  pinned process spends time starting and shutting down; the configured process
  timeout, not the case latency, is the bound. The trial observed about `121s`
  for Promptfoo while the model case itself was milliseconds.
- **A case reports a few milliseconds with hundreds of completion tokens.** It
  may be a Promptfoo disk-cache hit. The adapter does not pass `--no-cache`, and
  the normalized report does not record cache state. Re-run the pinned CLI with
  `--no-cache` to measure real model latency.
- **`OLLAMA_BASE_URL` appears ignored.** The scanner passes only that variable
  through; it does not fall back to a hosted provider. Set it in the scan
  environment, not only in an interactive shell profile.
