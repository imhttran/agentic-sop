# SEC-H-001 — Security Baseline and Finding Triage

**Stage:** read-only inspection (report-only). No source, configuration, or baseline file is modified by this stage.

**Repository HEAD:** `9cc4bf29ce7adedf93213ae9b21b4df3c9567a7a`
**Tree:** `95bc67b9a36e312abd22f2451161e6a361c0d43c`

## Scope of this baseline

This report records the point-in-time security baseline from the two executed
security runs already present in the run history. It is a triage record, not a new
vulnerability review. Every per-adapter claim below cites the captured run artifacts.

### Executed evidence

The two `sop security scan` runs whose artifacts are cited here:

| Run id | Mode | Artifacts |
| --- | --- | --- |
| `security-20261108T171321Z` | report-only | `.agent-sdlc/runs/security-20261108T171321Z/security/{scan.json,scan.md,semgrep.native.json,gitleaks.native.json}` |
| `security-20261108T171417Z` | report-only | `.agent-sdlc/runs/security-20261108T171417Z/security/{scan.json,scan.md,semgrep.native.json,gitleaks.native.json}` |

`security-20261108T171321Z` is the primary baseline cited for adapter
status/coverage/duration. `security-20261108T171417Z` is the full-workspace repeat
cited for the unsuppressed finding list (it scanned test files; the earlier run was
skipped by the default `.semgrepignore`, giving `PARTIAL`).

The `sop security inspect --json` command produces the `security.inspect` view that
reports adapter availability (`sast`/`secrets` available, `dependencies` unsupported,
`required_ready` false). That inspection view is reflected in the adapter table
below: the two runs show `gitleaks` and `semgrep` `COMPLETED`, and `osv-scanner`
`FAILED` with `dependencies` reported as unsupported.

## Per-adapter status, coverage, duration, scope, exclusions

Source: `.agent-sdlc/runs/security-20261108T171321Z/security/scan.md` and
`security-20261108T171417Z/security/scan.md`.

| Adapter | Category | Status | Coverage (171321Z) | Duration | Scope | Exclusions |
| --- | --- | --- | --- | --- | --- | --- |
| `gitleaks` | secrets | `COMPLETED` | `COMPLETE` | 2246 ms (171321Z) / 2282 ms (171417Z) | `workspace`, repository-wide, staged filtered view | `.agent-sdlc`, `.git`, `.run/reference`, `internal/security/testdata`, `testdata` |
| `semgrep` | sast | `COMPLETED` | `PARTIAL` (171321Z) / `COMPLETE` (171417Z) | 3336 ms (171321Z) / 5111 ms (171417Z) | `workspace`, language `go` | `.agent-sdlc`, `.git`, `.run/reference`, `internal/security/testdata`, `testdata` (declared) plus Semgrep `.semgrepignore` skips |
| `osv-scanner` | dependencies | `FAILED` | `UNKNOWN` | 14 ms (171321Z) / 15 ms (171417Z) | none scanned | none |

Run-level totals:

| Field | `security-20261108T171321Z` | `security-20261108T171417Z` |
| --- | --- | --- |
| Verdict | `FAIL` (report-only) | `FAIL` (report-only) |
| Coverage | `PARTIAL` | `UNKNOWN` |
| Total duration | 5917 ms | 7764 ms |
| Working tree | DIRTY (digest `a7aec9bb…`) | DIRTY (digest `46b75c64…`) |
| Unsuppressed blocking findings | 3 | 23 |

Scanner provenance: `gitleaks 8.30.1`, `semgrep 1.183.0`, `osv-scanner 2.7.0`.

### Baseline state

- Baseline file: `.agent-sdlc/security-baseline.json`, `version: 1`, 30 reviewed
  entries, every entry `owner: imhttran`, `expires_at: 2027-06-30T00:00:00Z`.
- The baseline applied to both runs: findings marked `[suppressed]` were matched by a
  baseline entry; findings without that marker are unsuppressed.
- No suppression was added or changed in this stage.

## Semgrep skip class (recorded verbatim)

From `security-20261108T171321Z/security/scan.md`:

> semgrep skipped target(s) inside the declared scope; coverage is PARTIAL: Files matching .semgrepignore patterns: 746

From `security-20261108T171417Z/security/scan.md`:

> semgrep omitted target(s) by configured exclusion patterns (declared out-of-scope): Matching --exclude patterns: 11

The skip class is therefore **`.semgrepignore`-based default skips** (746 targets in
the primary run: `go` test files and language-filtered content the default
`.semgrepignore` excludes). This is a scanner-side coverage limit, not a declared
exclusion. The declared `--exclude` patterns (11 matches in the full run) are
out-of-scope by configuration and do not reduce coverage below `COMPLETE`.

## OSV absence (recorded verbatim)

From both runs' `scan.md`:

> OSV database cache is unavailable: stat …/.agent-sdlc/osv-db: no such file or directory

`osv-scanner` reported `FAILED`, coverage `UNKNOWN`, and `dependencies: failed`.
No dependency findings are produced because no offline OSV database is provisioned on
this host. No vulnerability claim is made from OSV data. Provisioning is an
external, operator-authorized action (`scripts/security/fetch-osv-databases.sh`) and
was not run in this plan.

## Unsuppressed findings

Source of truth: `security-20261108T171417Z/security/scan.md` (full-workspace run; 23
unsuppressed findings at blocking severity). Each finding appears once. Every finding
has a disposition and the mechanism used.

| # | Severity | Rule | path:line | Disposition | Mechanism |
| --- | --- | --- | --- | --- | --- |
| 1 | HIGH | dangerous-exec-command | `internal/cli/outcome.go:720` | accepted-with-control | argv validated (SplitCommand + commandpolicy.SAFE); see trust-boundary T1 |
| 2 | HIGH | dangerous-exec-command | `internal/cli/security_integration_test.go:38` | accepted-with-control | test-only harness exec; fixture/scan helper; not production argv |
| 3 | HIGH | dangerous-exec-command | `internal/controller/web/activity_isolation_test.go:134` | accepted-with-control | test-only harness exec |
| 4 | HIGH | dangerous-exec-command | `internal/decision/commandprofile/connector_test.go:211` | accepted-with-control | test-only harness exec |
| 5 | HIGH | dangerous-exec-command | `internal/decisionadapter/providers/julia/runner.go:231` | accepted-with-control | structured config argv, no shell; see trust-boundary T2 |
| 6 | HIGH | dangerous-exec-command | `internal/dist/install_test.go:162` | accepted-with-control | test-only installer exec |
| 7 | HIGH | dangerous-exec-command | `internal/dist/install_test.go:287` | accepted-with-control | test-only installer exec |
| 8 | HIGH | dangerous-exec-command | `internal/dist/install_windows_test.go:120` | accepted-with-control | test-only installer exec |
| 9 | HIGH | dangerous-exec-command | `internal/dist/install_windows_test.go:138` | accepted-with-control | test-only installer exec |
| 10 | HIGH | dangerous-exec-command | `internal/dist/install_windows_test.go:217` | accepted-with-control | test-only installer exec |
| 11 | HIGH | dangerous-exec-command | `internal/dist/install_windows_test.go:256` | accepted-with-control | test-only installer exec |
| 12 | HIGH | dangerous-exec-command | `internal/dist/install_windows_test.go:503` | accepted-with-control | test-only installer exec |
| 13 | HIGH | dangerous-exec-command | `internal/dqharness/replay_exec.go:273` | accepted-with-control | replay harness exec; see trust-boundary T3 |
| 14 | HIGH | dangerous-exec-command | `internal/projectdetect/exec_test.go:56` | accepted-with-control | test-only harness exec |
| 15 | HIGH | dangerous-exec-command | `internal/projectlock/lock_test.go:133` | accepted-with-control | test-only harness exec |
| 16 | HIGH | dangerous-exec-command | `internal/question/atomic_test.go:178` | accepted-with-control | test-only harness exec |
| 17 | HIGH | dangerous-exec-command | `internal/rageval/ragacceptance_test.go:129` | accepted-with-control | test-only harness exec |
| 18 | HIGH | dangerous-exec-command | `internal/security/integration_test.go:55` | accepted-with-control | test-only integration exec |
| 19 | HIGH | dangerous-exec-command | `internal/security/integration_test.go:541` | accepted-with-control | test-only integration exec |
| 20 | HIGH | dangerous-exec-command | `internal/security/integration_test.go:677` | accepted-with-control | test-only integration exec |
| 21 | HIGH | dangerous-exec-command | `internal/testfast/testfast_test.go:72` | accepted-with-control | test-only harness exec |
| 22 | HIGH | dangerous-exec-command | `internal/testfast/testfast_test.go:355` | accepted-with-control | test-only harness exec |
| 23 | HIGH | dangerous-exec-command | `internal/toolharness/import_sequence_test.go:50` | accepted-with-control | test-only harness exec |

Additional MEDIUM/non-blocking unsuppressed findings from the same run (recorded for
completeness; not at the blocking severity):

| Severity | Rule | path:line | Disposition | Mechanism |
| --- | --- | --- | --- | --- |
| MEDIUM | no-direct-write-to-responsewriter | `internal/agent/openaicompatible_execution_test.go:44` | accepted-with-control | test HTTP handler; direct write of JSON body |
| MEDIUM | no-direct-write-to-responsewriter | `internal/agent/provider_test.go:199` | accepted-with-control | test HTTP handler |
| MEDIUM | no-direct-write-to-responsewriter | `internal/agent/provider_test.go:291` | accepted-with-control | test HTTP handler |
| MEDIUM | no-direct-write-to-responsewriter | `internal/cli/ollama_e2e_test.go:41` | accepted-with-control | test HTTP handler |
| MEDIUM | no-direct-write-to-responsewriter | `internal/cli/provider_validation_task_test.go:39` | accepted-with-control | test HTTP handler |
| MEDIUM | unsafe-reflect-by-name | `internal/cli/zz_operator_acceptance_acc008_test.go:158` | accepted-with-control | test-only dynamic field access |
| MEDIUM | cookie-missing-httponly | `internal/controller/web/recovery_test.go:474` | accepted-with-control | test cookie |
| MEDIUM | cookie-missing-secure | `internal/controller/web/recovery_test.go:474` | accepted-with-control | test cookie |
| MEDIUM | no-fprintf-to-responsewriter | `internal/decisionadapter/providers/clef/chat_test.go:39` | accepted-with-control | test HTTP handler |
| MEDIUM | no-io-writestring-to-responsewriter | `internal/decisionadapter/providers/nimble/score_test.go:77` | accepted-with-control | test HTTP handler |
| MEDIUM | no-io-writestring-to-responsewriter | `internal/ollamaagent/ollama_retry_test.go:70` | accepted-with-control | test HTTP handler |
| MEDIUM | no-io-writestring-to-responsewriter | `internal/ollamaagent/ollama_retry_test.go:73` | accepted-with-control | test HTTP handler |
| MEDIUM | no-io-writestring-to-responsewriter | `internal/provider/openai/provider_locality_test.go:24` | accepted-with-control | test HTTP handler |
| MEDIUM | string-formatted-query | `internal/store/notifications_test.go:40` | accepted-with-control | test SQL fixture |

## Suppressed findings (baseline cross-reference)

Findings marked `[suppressed]` in the scan are matched by a reviewed entry in
`.agent-sdlc/security-baseline.json` (owner `imhttran`, expiry `2027-06-30`). Each
suppressed finding and its baseline reason:

| path:line | Rule | Baseline reason |
| --- | --- | --- |
| `cmd/deceleval-recovery/main.go:204` | dangerous-exec-command | operator config/constant argv; no shell |
| `internal/agent/command.go:81` | dangerous-exec-command | `SOP_AGENT_COMMAND` trusted config; `sh -c` documented |
| `internal/cli/mcp_start_work.go:721` | dangerous-exec-command | operator config/constant argv |
| `internal/controller/sopclient/commands.go:108` | dangerous-exec-command | structured argv, no shell |
| `internal/controller/web/activity_stream.go:238` | no-direct-write-to-responsewriter | SSE frames from json.Marshal |
| `internal/controller/web/activity_stream.go:241` | no-direct-write-to-responsewriter | SSE frames from json.Marshal |
| `internal/controller/web/activity_stream.go:271` | no-direct-write-to-responsewriter | SSE frames from json.Marshal |
| `internal/controller/web/middleware.go:45` | cookie-missing-secure | Secure set by isHTTPS(r) |
| `internal/controller/web/session.go:327` | open-redirect | constant prefix / safeNext() |
| `internal/controller/web/session.go:377` | cookie-missing-secure | Secure set by isHTTPS(r) |
| `internal/controller/web/session.go:389` | cookie-missing-secure | Secure set by isHTTPS(r) |
| `internal/controller/web/session.go:454` | open-redirect | safeNext() same-origin path |
| `internal/controller/web/session.go:479` | open-redirect | safeNext() same-origin path |
| `internal/decision/command/command.go:133` | dangerous-exec-command | structured config argv, no shell |
| `internal/decision/commandprofile/connector.go:172` | dangerous-exec-command | structured config argv, no shell |
| `internal/decision/commandprofile/procattr_windows.go:42` | use-of-unsafe-block | Windows Job Object FFI, local struct |
| `internal/decision/commandprofile/procattr_windows.go:43` | use-of-unsafe-block | Windows Job Object FFI, local struct |
| `internal/decisionquality/paired.go:6` | math-random-used | seeded bootstrap resampling, not security |
| `internal/evaltarget/command.go:62` | dangerous-exec-command | structured config argv, no shell |
| `internal/github/command.go:31` | dangerous-exec-command | `gh`/`git` structured argv, no shell |
| `internal/handoff/caveman/caveman.go:89` | dangerous-exec-command | trusted config, no untrusted data |
| `internal/hipcompare/command.go:396` | dangerous-exec-command | structured config argv, no shell |
| `internal/hipcompare/command.go:423` | dangerous-exec-command | structured config argv, no shell |
| `internal/memindex/index.go:84` | string-formatted-query | package-constant int in PRAGMA |
| `internal/ragclient/exec.go:17` | dangerous-exec-command | absolute, stat-validated executable, no shell |
| `internal/review/ocr.go:55` | dangerous-exec-command | `SOP_REVIEW_COMMAND` trusted; `sh -c` documented |
| `internal/security/exec.go:65` | dangerous-exec-command | adapter argv from config, no shell |
| `internal/store/handoff.go:53` | string-formatted-query | constant columns + bound params |
| `internal/testrunner/command.go:34` | dangerous-exec-command | configured validation, `sh -c` contract |
| `internal/toolharness/harness.go:882` | dangerous-exec-command | SplitCommand + path check + commandpolicy.SAFE |

## Trust-boundary records — HIGH dangerous-exec-command sites

The HIGH production (non-test) `dangerous-exec-command` sites are recorded below. Test
files are excluded from the trust-boundary record because their argv is test-fixture
data that never crosses a runtime boundary. Line numbers are from the scan at the
recorded commit; the cited source files carry the call path.

### T1 — `internal/cli/outcome.go:720`

- **argv origin:** acceptance command argv arrives through the `authorizeAcceptanceCommand`
  path; it must pass `toolharness.SplitCommand` (rejects shell syntax) and
  `commandpolicy.Classify` must return `SAFE` before the vector is authorized
  (`internal/cli/outcome.go`, acceptance path).
- **Shell involved:** no. The authorized vector runs without a shell.
- **Authorizing control:** the `SplitCommand` + `commandpolicy.SAFE` gate.
- **Source reference:** `internal/cli/outcome.go`.

### T2 — `internal/decisionadapter/providers/julia/runner.go:231`

- **argv origin:** `r.Command` comes from `JULIA_INFERENCE_CMD` or the repository
  helper; it is developer/operator configuration, not remote input.
- **Shell involved:** no. Structured argv passed directly.
- **Authorizing control:** operator-supplied configuration; provider output is never
  interpolated into the command.
- **Source reference:** `internal/decisionadapter/providers/julia/runner.go`.

### T3 — `internal/dqharness/replay_exec.go:273`

- **argv origin:** the replay harness builds its command from harness configuration
  (replay spec) in `internal/dqharness/replay_exec.go`; it is operator/local-tooling
  configuration, invoked by the DQ harness, not from a network boundary.
- **Shell involved:** no. Structured argv passed directly to `exec.Command`.
- **Authorizing control:** the DQ harness configuration boundary; runtime isolation is
  provided by the evaltarget/dqharness isolation harness (macOS Seatbelt on this
  host).
- **Source reference:** `internal/dqharness/replay_exec.go`.

All other HIGH production `dangerous-exec-command` sites (the `internal/decision`,
`github`, `handoff`, `hipcompare`, `ragclient`, `review`, `security`, `testrunner`,
`toolharness`, and `cmd/deceleval-recovery` sites) are already baseline-suppressed
with source-backed reasons in the suppressed table above and carry the same
control pattern: trusted operator/project configuration, structured argv, and no shell
where a shell is not the documented contract.

## Required-mode projection

Both runs report required-mode readiness verbatim:

> Ready: `false`
> Projected verdict: `INDETERMINATE`
> required mode needs at least one required category

So under `--required` the current configuration would not gate: no required category
is configured, and the projection is `INDETERMINATE` rather than `FAIL`. If a required
category (`sast`, `secrets`) were configured, the `FAIL` verdict from the 23
unsuppressed blocking findings (all HIGH `dangerous-exec-command`, plus the MEDIUM set)
would gate. The `PARTIAL` semgrep coverage in the primary run and the `FAILED` OSV
adapter would also fail closed on coverage/dependency input. Before required mode can
be enabled, the test-file SAST boundary and the OSV database provisioning must be
resolved.

## Summary

- Per-adapter status/coverage/duration/scope/exclusions recorded for gitleaks,
  semgrep, and osv-scanner from two executed runs.
- 30 baseline suppressions cross-referenced; 23 unsuppressed blocking findings (all
  HIGH `dangerous-exec-command`) plus 14 non-blocking MEDIUM findings each have a
  disposition and mechanism.
- Trust-boundary records given for the three HIGH production dangerous-exec-command
  sites (`internal/cli/outcome.go`, `internal/decisionadapter/providers/julia/runner.go`,
  `internal/dqharness/replay_exec.go`); the remaining HIGH production sites are
  baseline-suppressed with source-backed reasons.
- Semgrep skip class, OSV absence, and required-mode projection recorded with verbatim
  captured output.
- No source, configuration, or baseline file was modified by this stage.
