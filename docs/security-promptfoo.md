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
| `target_provider`         | — (required for `local`)                  | —                                                     | Provider that serves the target model.                                   |
| `attack_provider`         | — (required for `local`)                  | —                                                     | Provider that generates attacks.                                         |
| `grading_provider`        | — (required for `local`)                  | —                                                     | Provider that grades results.                                            |
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

Installation note: the pinned `0.118.0` package requires a Node runtime that can
build/load its `better-sqlite3` dependency. Node 26 failed in the verification
environment; Node 22 installed and ran the pin successfully. This is an operator
prerequisite, not a silent download.

Credentials never appear in configuration. The schema has no credential field;
provider and model identifiers are stored, secrets stay in the environment.
`IsCredentialKey` in `internal/config/security_eval.go` names the keys that
would commit a credential.

## Normalized report

`internal/security/evalreport.go` defines the report written to
`<run dir>/security/promptfoo.case.json`. The raw sanitized Promptfoo output is
written next to it as `promptfoo.raw.json`, even when parsing fails, so the
failure stays inspectable.

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

Per-case status is exactly one of `passed`, `failed`, `error`, or `skipped`. A
timeout, unavailable provider, missing artifact, parsing failure, or skip never
becomes `passed`. There is no computed CVSS or severity field for behavioral
failures: a finding carries `severity` only when `severity_mode: explicit` and
the raw metadata names one of the normalized labels.

Evidence links point at the redacted normalized report. Raw and normalized
output are sanitized before writing, so no credential string is persisted.

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

- `local` profile: none. The adapter selects explicit local providers and
  rejects a remote provider at load time and again when it resolves providers;
  there is no silent hosted fallback. An executor without explicit environment
  support is a fail-closed execution error, so SOP never claims telemetry or
  sharing was disabled without having set it.
- `remote` profile: only the providers the operator named in
  `attack_provider`, `grading_provider`, and `target_provider`, after
  `remote_provider: true` and `data_handling: remote-allowed`. Telemetry,
  sharing, remote generation, and update checks stay disabled.
- Share links: never requested. The adapter does not pass `--share`, and the
  `0.118.0` release has no `--no-share` flag; sharing stays off unless the
  operator's Promptfoo config opts in. `PROMPTFOO_SELF_HOSTED=true` and the
  telemetry-disable environment are still set.
- Update checks: disabled by `PROMPTFOO_DISABLE_UPDATE=true`; SOP also never
  invokes a package manager.

Any further outbound call made by the pinned release itself is not verified for
this pin and must be treated as `UNAVAILABLE` evidence until an operator checks
the release notes for `0.118.0`.

## Isolation boundary

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
- **Live-model adversarial evaluation** is not implemented in this PR. The one
  live adapter invocation below is a path check, not the six AISEC scenarios.

## Live verification

The live check is opt-in and separate from the deterministic suite:

```sh
go test -tags=promptfoo_live ./internal/security/ -run TestPromptfooLiveInvocation -count=1 -v
```

The live test sets `OLLAMA_BASE_URL` and prepends Node 22 to `PATH`; it uses
`evals/promptfoo/promptfooconfig.yaml` and the local `ollama:chat:qwen3.5:4b`
provider. The build tag keeps the ordinary deterministic suite free of a live
model call. `OLLAMA_BASE_URL` is passed through because the scanner environment
strips provider-looking variables. The live result is one passing case with a
complete report, a real config hash, and Promptfoo 0.118.0 identity; it is **not**
evidence that the six deterministic AISEC fixtures passed through a live model.

`promptfoo` 0.118.0 writes the 0.118 results envelope (`results` is an object
containing `results` and `stats`); the parser accepts that envelope and the legacy
bare array. The release also exits `100` for failed tests; the adapter treats that
as a finding and parses the report, while other non-zero exits remain execution
errors.
