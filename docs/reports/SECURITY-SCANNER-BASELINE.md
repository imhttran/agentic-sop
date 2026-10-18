# Security Scanner Baseline — `agentic-sop`

**Date:** 2026-10-17
**Repository:** `agentic-sop`
**Mode:** `report-only`
**Target:** `origin/main` at `28cdf614ed2ead6f6226a317233fc2ef1c8bfa6b`
**Production run:** `security-baseline-prod-20261017`
**Fixture run:** `security-baseline-fixtures-20261017`

## Result

| Field               | Value                                      |
| ------------------- | ------------------------------------------ |
| Production verdict  | `FAIL`                                     |
| Production coverage | `PARTIAL` (Semgrep), `COMPLETE` (Gitleaks) |
| Production findings | 27 (26 SAST, 1 secret)                     |
| Blocking findings   | 15 (14 SAST `HIGH`, 1 secret `HIGH`)       |
| Suppressed findings | 0                                          |
| Execution failures  | 0                                          |
| Total duration      | 3204 ms (Semgrep 2587 ms, Gitleaks 558 ms) |
| Fixture verdict     | `FAIL` (3 synthetic findings)              |
| Fixture coverage    | `COMPLETE` for both adapters               |
| Fixture duration    | 1310 ms (Semgrep 1108 ms, Gitleaks 161 ms) |

Evidence:

- Production: `.agent-sdlc/runs/security-baseline-prod-20261017/security/scan.json`,
  `scan.md`, `semgrep.native.json`, `gitleaks.native.json`, `coverage-detail.md`
- Fixtures: `.agent-sdlc/runs/security-baseline-fixtures-20261017/security/scan.json`,
  `scan.md`, `semgrep.native.json`, `gitleaks.native.json`

The verdict is `FAIL` because 15 unsuppressed findings are at the configured
`fail_on` severities. Report-only recorded the verdict without blocking.

## Scanned content identity

The scan ran in a clean detached worktree at `origin/main`, so the target content
is the committed repository rather than the operator's dirty working tree.

| Field          | Value                                               |
| -------------- | --------------------------------------------------- |
| Commit         | `28cdf614ed2ead6f6226a317233fc2ef1c8bfa6b`          |
| Tree           | `a66f87df9d6e505d6017ea86a961b5b580c7e5ad`          |
| Working tree   | `CLEAN` (`dirty: false`)                            |
| Content digest | none (clean tree; commit and tree are the identity) |

The operator's main worktree contained unrelated untracked files
(`.run/`, two plan documents). They were preserved and were not part of the
baseline target. The fixture run used a separate temporary Git repository so its
content identity and findings are independent of production.

## Tools, rules, and provenance

| Adapter    | Version   | Binary SHA-256                                                     | Rules identity                                                     |
| ---------- | --------- | ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| Semgrep CE | `1.136.0` | `0ae2ab17a40adce7f3247f2b105fbb43a6e11680a67cfc779ad16c6e31824af7` | `d3602b6590db16b37b035ddacdbefc612a804ee9840b2cf7b705cf064b2dada7` |
| Gitleaks   | `8.30.1`  | `ba52fb1bfabbcde42f032afad3d6e0b19dff8ed105229a16e7caa338bbc0e84f` | built-in rules; binary digest is the identity proxy                |

The scan helper was built from `feat/security-baseline-exclusions`; its SHA-256
was `83d8fef7a94beaabf7e2f80b7b4a47c92ec1ae43ba27662ff19558b8900caadd`.
Adapter configuration digests: Semgrep `ce52a03b07bd5c5b997e06331dc47eae811bc6572c44fdc5e2d5f18435aad077`,
Gitleaks `c308356740a0f7c36903ef409b2eca80472fac2ecab924c4ab5ed0c260a08ae8`.
The shared policy identity was `936413f96f08eca7aa69cc70bf697ebd20a8898d1c28dfb121f01bc051ee6c47`.

### Semgrep rules

The rules are the maintained Go rules from
[`semgrep/semgrep-rules`](https://github.com/semgrep/semgrep-rules) at commit
`17e31e11ebac99fb7dfe1cd288f7a32151b6f3f9` (2026-10-15), limited to
`go/lang/security` and `go/template/security`: 56 rule files, 59 rules. They were
flattened into one local rules file so the rules identity is a single digest and
Semgrep is invoked with `--no-rewrite-rule-ids` for stable rule IDs.

The rules are used locally under the Semgrep Rules License. They are not
committed to this repository because that license does not permit redistribution.
Reproducing the rules identity therefore requires fetching the pinned upstream
commit and rebuilding the merged file.

The rules that produced findings were:

| Rule ID                               | Category | Findings |
| ------------------------------------- | -------- | -------- |
| `dangerous-exec-command`              | SAST     | 14       |
| `no-direct-write-to-responsewriter`   | SAST     | 3        |
| `cookie-missing-secure`               | SAST     | 3        |
| `open-redirect`                       | SAST     | 3        |
| `use-of-unsafe-block`                 | SAST     | 2        |
| `string-formatted-query`              | SAST     | 1        |
| `generic-api-key` (Gitleaks built-in) | Secrets  | 1        |

### Gitleaks rules

Gitleaks used its built-in ruleset from the pinned `8.30.1` binary. No local
Gitleaks configuration or hosted validation service was used. The adapter reports
an empty `rules_identity` for built-in rules; the binary digest above is the
reproducibility proxy.

## Exclusions and coverage

`security.exclude` is a declared scope boundary, not a suppression. The effective
list is recorded in `scope.excluded`.

| Pattern                                                                       | Why it is outside the production scope                             |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `.git`, `.agent-sdlc`                                                         | Built-in defaults: VCS metadata and SOP run state are not source.  |
| `internal/security/testdata`                                                  | Synthetic scanner fixtures; scanned separately in the fixture run. |
| `internal/security/integration_test.go`, `internal/security/security_test.go` | Tests that pin synthetic fixture literals.                         |
| `/.run`, `/.agents`, `/.claude`, `/.playwright-mcp`, `/.zed`                  | Local/ignored tooling and scratch directories.                     |
| `/sop-harden-001`                                                             | Ignored 10 MB scratch copy of the repository.                      |
| `/sop`, `/sop-*`                                                              | Ignored build binaries at the repository root.                     |
| `/.env`, `/.env.*`, `/.DS_Store`                                              | Local configuration/credentials and editor metadata.               |

The leading `/` anchors a pattern to the repository root. This matters: an
unanchored `sop-*` pattern initially excluded the four real `cmd/sop*` packages.
The anchored form excludes only root binaries. The integration fix and its test
cover this case.

### Semgrep coverage: `PARTIAL`

Semgrep scanned 400 non-test Go files. It reported 581 skipped targets by its
default `.semgrepignore` policy (recorded in
`security/coverage-detail.md`):

- 578 `*_test.go` files.
- `internal/tests/fixtures` and `tests/controller` directories.
- One minified JavaScript file (`internal/controller/assets/static/htmx.min.js`).

Semgrep CE 1.136 applies its default `.semgrepignore` patterns even with
`--no-git-ignore`; `*_test.go` is one of those defaults. The production SAST
baseline therefore covers non-test Go source, not test code. Gitleaks covers the
full tracked workspace, including tests, for secrets.

This is the only unresolved coverage gap. Options:

1. Accept test code as outside the SAST scope (recommended for this production
   baseline) and encode that boundary explicitly in policy.
2. Add a project `.semgrepignore` negation (`!**/*_test.go`) to re-include tests.
   A probe raised findings from 26 to 52, almost all test-only process-execution
   calls, so this needs additional triage.
3. Keep `PARTIAL` and do not enable required mode until 1 or 2 is decided.

### Gitleaks coverage: `COMPLETE`

Gitleaks scanned 13.09 MB of workspace content in 391 ms (adapter wall time
558 ms). It has no path-exclusion flag, so findings in declared excluded paths are
removed after parsing and counted in diagnostics. The declared list remains
visible in `scope.excluded`.

For comparison, the earlier dogfood run scanned about 161 MB, largely because it
included `.git` and local scratch content. The built-in `.git`/`.agent-sdlc`
exclusions and the clean worktree reduce the baseline to the actual source tree.

## Production findings and dispositions

Every finding was reviewed against the source at the recorded commit. Totals:
**27 reviewed, 0 confirmed application vulnerabilities, 27 false positives,
0 unresolved findings.** The absence of a confirmed issue is not proof that the
repository has none; see [Limitations](#limitations).

### `dangerous-exec-command` — 14 `HIGH`

All 14 are audit findings for non-static process arguments. Each call site was
checked for a shell boundary and for whether untrusted data can reach the argv.

| Location                                                 | Disposition | Source-backed evidence                                                                                                                                                                                                                      |
| -------------------------------------------------------- | ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/deceleval-recovery/main.go:204`                     | FP          | `adapter` and `provider` come from developer CLI flags (lines 44-48); structured argv, no shell.                                                                                                                                            |
| `internal/agent/command.go:81`                           | FP          | `a.command` comes from `SOP_AGENT_COMMAND` (lines 15-23, 56-62), trusted operator configuration; `sh -c` is the documented harness contract.                                                                                                |
| `internal/cli/outcome.go:446`                            | FP          | Reached only through `authorizeAcceptanceCommand` (lines 312-328): `toolharness.SplitCommand` rejects shell syntax and `commandpolicy.Classify` must return `SAFE`; the authorized vector runs without a shell.                             |
| `internal/controller/sopclient/commands.go:38`           | FP          | `c.bin` is fixed by `NewCommander`; arguments are structured and passed directly, no shell.                                                                                                                                                 |
| `internal/decision/command/command.go:133`               | FP          | `a.argv` is configuration supplied to `New(name, argv)`; structured argv, no shell.                                                                                                                                                         |
| `internal/decision/commandprofile/connector.go:172`      | FP          | `c.argv` is configuration; structured argv, no shell.                                                                                                                                                                                       |
| `internal/decisionadapter/providers/julia/runner.go:239` | FP          | `r.Command` comes from `JULIA_INFERENCE_CMD` or the repository helper; structured argv, no shell.                                                                                                                                           |
| `internal/github/command.go:31`                          | FP          | `name` is `gh` or `git`; arguments are structured, no shell.                                                                                                                                                                                |
| `internal/handoff/caveman/caveman.go:89`                 | FP          | Commands are explicitly documented as trusted configuration (“Commands are trusted configuration, never untrusted input”); no untrusted data reaches the shell string.                                                                      |
| `internal/ragclient/exec.go:17`                          | FP          | `cfg.Executable` must be an absolute, existing file (validated in `ragclient.go`); structured argv, no shell.                                                                                                                               |
| `internal/review/ocr.go:55`                              | FP          | `SOP_REVIEW_COMMAND` is documented as a trusted local command (lines 19-25); `sh -c` is intentional.                                                                                                                                        |
| `internal/security/exec.go:47`                           | FP          | The adapter command comes from `security.adapters[].command`; structured argv, no shell.                                                                                                                                                    |
| `internal/testrunner/command.go:34`                      | FP          | `check.Command` is built from `cfg.Validation` in `checkCommands`; it is operator/project configuration, and `sh -c` is the configured validation contract.                                                                                 |
| `internal/toolharness/harness.go:810`                    | FP          | Model-requested commands are tokenized by `SplitCommand`, path-checked by `checkCommandIn`, and must classify as `commandpolicy.Safe`; unknown programs require approval and are refused. This is a compensating control, not a free shell. |

Re-review trigger: if any of these commands can be set by an unauthenticated or
remote caller (for example a web-settable `SOP_*_COMMAND`), the disposition
changes from false positive to a real command-injection risk.

### `no-direct-write-to-responsewriter` — 3 `MEDIUM`

Locations: `internal/controller/web/activity_stream.go:227`, `:230`, `:260`.

False positives. The handler sets `Content-Type: text/event-stream` at line 162
and writes Server-Sent Events: JSON-marshaled payloads plus SSE framing. These
are not HTML responses, so the HTML-escaping concern does not apply.

### `cookie-missing-secure` — 3 `MEDIUM`

Locations: `internal/controller/web/middleware.go:45`,
`internal/controller/web/session.go:377`, `:389`.

False positives. `Secure` is set to the dynamic value `isHTTPS(r)`, not omitted.
`isHTTPS` honors TLS and only trusts `X-Forwarded-Proto: https` from a loopback
proxy; plain HTTP on localhost intentionally does not set `Secure`. The rule
matches the absence of a literal `Secure: true`.

### `open-redirect` — 3 `MEDIUM`

Locations: `internal/controller/web/session.go:327`, `:454`, `:479`.

False positives:

- `:327` redirects to the fixed path `/login?next=<escaped request URI>`; the
  destination host is not user-controlled.
- `:454` and `:479` use `safeNext`, which requires a leading `/`, rejects `//`,
  rejects CR/LF/backslash, and rejects any parsed scheme or host. The redirect
  target is therefore a same-origin path or the `/projects` default.

### `use-of-unsafe-block` — 2 `MEDIUM`

Locations: `internal/decision/commandprofile/procattr_windows.go:42`, `:43`.

False positives. The `unsafe.Pointer`/`unsafe.Sizeof` use is the required
Windows Job Object FFI pattern for
`windows.SetInformationJobObject`. The pointed-to struct is constructed locally;
no attacker-controlled memory is involved.

### `string-formatted-query` — 1 `MEDIUM`

Location: `internal/store/handoff.go:53`.

False positive. `handoffColumns` is a package-level constant (line 65) and all
values are bound with `?` placeholders. No user input is concatenated into the
SQL string.

### `generic-api-key` — 1 `HIGH`

Location: `internal/question/atomic_test.go:183`.

False positive. The match is the test helper environment assignment
`"QUESTION_KEY="+key`; `key` is the synthetic value `k-0`/`k-1` created at line 203. No credential is present. Gitleaks still classifies it `HIGH`, so it remains
unsuppressed and contributes to the `FAIL` verdict.

## Synthetic fixture run (separate)

Fixtures were copied to an isolated temporary Git repository, scanned with the
integration rules/config, and never mixed into the production result.

| Category | Rule ID                             | Location                                               | Disposition |
| -------- | ----------------------------------- | ------------------------------------------------------ | ----------- |
| SAST     | `sop-integration-command-execution` | `internal/security/testdata/real/seeded/insecure.go:7` | Expected    |
| Secret   | `sop-integration-synthetic-token`   | `internal/security/testdata/fixtures/secret.txt:1`     | Expected    |
| Secret   | `sop-integration-synthetic-token`   | `internal/security/testdata/real/seeded/secret.txt:3`  | Expected    |

The fixture run also demonstrated exclusion accounting: Gitleaks detected two
additional synthetic-token findings in excluded report/config fixtures and
reported that they were omitted from the normalized result.

## Secret handling

The saved evidence contains no secret values:

- All production and fixture evidence files were checked for every synthetic
  token variant; none was present.
- Gitleaks `Secret` and `Match` fields in both native reports are `[REDACTED]`.
- This report does not reproduce the synthetic token value.

The production baseline ran in a clean worktree, so local `.env` files were not
part of the target. The reusable configuration also excludes `/.env` and
`/.env.*`.

## Timings

| Run        | Total   | Semgrep | Gitleaks |
| ---------- | ------- | ------- | -------- |
| Production | 3204 ms | 2587 ms | 558 ms   |
| Fixtures   | 1310 ms | 1108 ms | 161 ms   |

## Required-mode readiness

**Required mode is not ready.** Report-only is the correct current mode.

Prerequisites before `mode: required` can be considered:

1. **Resolve the 15 blocking findings.** The 14 SAST findings are low-confidence
   audit findings with verified compensating controls, and the secret finding is
   a test false positive. Required mode has no review-disposition mechanism:
   choose rule tuning, a reviewed baseline with owner/expiry, or an explicit
   exclusion, and record the decision. Nothing was suppressed automatically.
2. **Decide the test-code SAST boundary.** Semgrep coverage is `PARTIAL` because
   its default `.semgrepignore` skips 578 test files. Required mode fails closed
   on `PARTIAL`. Either declare test code out of scope, re-include it with a
   `.semgrepignore` negation and triage the additional findings, or keep
   report-only.
3. **Make the rules reproducible.** The pinned upstream rules are local-only for
   licensing reasons. Add an explicit fetch/pin step (commit + digest) and pin
   scanner binaries in CI.
4. **Configure the required categories.** Add
   `required_categories: [sast, secrets]` and verify both adapters are
   `COMPLETED` with `COMPLETE` coverage.
5. **Review the exclusion semantics.** Declared exclusions are treated as scope,
   not coverage loss, and are recorded in `scope.excluded`. A required gate
   should only accept a reviewed exclusion list.

## Scanner-integration fixes in this branch

Found while establishing the baseline and fixed on
`feat/security-baseline-exclusions`:

1. **Declared exclusions.** Added `security.exclude` with repository-root
   anchoring, validation, effective defaults (`.git`, `.agent-sdlc`), and
   `scope.excluded` evidence.
2. **Adapter behavior.** Semgrep receives `--exclude`; Gitleaks filters excluded
   findings after parsing and accounts for them in diagnostics.
3. **Stable rule IDs.** Semgrep now runs with `--no-rewrite-rule-ids`, so
   fingerprints and future baselines do not change when the local rules directory
   moves.
4. **Correct coverage semantics.** A declared `--exclude` no longer makes
   coverage `PARTIAL`; scanner-side limits still do, and the parsed skip counts
   are recorded even when the raw diagnostic is truncated.
5. **Root-anchor regression.** Anchored patterns prevent `sop-*` from excluding
   real `cmd/sop*` source packages.

## Reproduction

```bash
# Clean target content
git worktree add --detach /tmp/sop-baseline-wt origin/main

# Pinned scanners (provisioned by scripts/checks/security-integration.sh)
semgrep --version    # 1.136.0
gitleaks version     # 8.30.1

# Local maintained rules: semgrep-rules 17e31e11ebac99fb7dfe1cd288f7a32151b6f3f9,
# go/lang/security + go/template/security, flattened to one file.

go build -o /tmp/sop-baseline-bin ./cmd/sop
env -C /tmp/sop-baseline-wt /tmp/sop-baseline-bin security scan \
  --config <report-only-config.yaml> --json \
  --run-id security-baseline-prod-20261017
```

The local config, scanner paths, and rules file are operator-local; the report
records their identities. The fixture run uses the integration rules/config from
`internal/security/testdata/real/` against an isolated copy.

## Limitations

- Semgrep SAST covers non-test Go source only; test code is skipped by the
  scanner default. Gitleaks covers tests for secrets.
- The Semgrep ruleset is maintained upstream but used locally under a license
  that does not allow redistribution. Reproducibility requires the pinned fetch.
- Gitleaks uses built-in rules; its rules identity is represented by the binary
  digest.
- Dependency, hosted, runtime/network, container, and asset-discovery scans are
  outside the add-on's MVP scope.
- Report-only is advisory. A `FAIL` verdict records evidence; it does not block
  delivery.
- A passing scan would be evidence for these tools and this scope, not proof that
  the repository has no vulnerabilities.

## Post-merge addendum (2026-10-17)

This addendum records a post-merge re-measurement of the baseline above. It is a
repeat scan of the merged code, not a new independent vulnerability review. The
source-backed dispositions in the original baseline carry forward unchanged.

| Field              | Value                                                        |
| ------------------ | ------------------------------------------------------------ |
| Target             | `origin/main` at `1b639ccfbcc604a933a650cba3d2dbff18a2499b`  |
| Tree               | `6b9bc0e562f23a3acdb891f54c67e0eb5210ac49`                   |
| Working tree       | `CLEAN` (`dirty: false`)                                     |
| Production run     | `security-baseline-postmerge-20261017`                       |
| Verdict            | `FAIL` (report-only; non-blocking)                           |
| Coverage           | Semgrep `PARTIAL`, Gitleaks `COMPLETE`                       |
| Findings           | 27 (26 SAST, 1 secret); same rule/location set               |
| Suppressed         | 0                                                            |
| Execution failures | 0                                                            |
| Total duration     | 3215 ms (Semgrep 2586 ms, Gitleaks 562 ms)                   |

### Provenance

| Adapter    | Version   | Rules identity                                                      | Configuration digest                                                | Sanitized report digest                                             |
| ---------- | --------- | ------------------------------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------- |
| Semgrep CE | `1.136.0` | `d3602b6590db16b37b035ddacdbefc612a804ee9840b2cf7b705cf064b2dada7`  | `ce52a03b07bd5c5b997e06331dc47eae811bc6572c44fdc5e2d5f18435aad077`  | `292b577cc9e531a0cb2a4240f05c906fdf475a5bdc4e27dd09f2a5c6216ae711`  |
| Gitleaks   | `8.30.1`  | built-in rules; binary identity from the original baseline          | `c308356740a0f7c36903ef409b2eca80472fac2ecab924c4ab5ed0c260a08ae8`  | `fbad7c7333169c6cd43a77c3d4f8a6976e579bf201046a00580dff57decb4370`  |

The shared policy identity remained
`936413f96f08eca7aa69cc70bf697ebd20a8898d1c28dfb121f01bc051ee6c47`.

### Findings comparison

No finding was added or removed by the merge:

| Rule ID                               | Original | Post-merge | Delta |
| ------------------------------------- | -------- | ---------- | ----- |
| `dangerous-exec-command`              | 14       | 14         | 0     |
| `no-direct-write-to-responsewriter`   | 3        | 3          | 0     |
| `cookie-missing-secure`               | 3        | 3          | 0     |
| `open-redirect`                       | 3        | 3          | 0     |
| `use-of-unsafe-block`                 | 2        | 2          | 0     |
| `string-formatted-query`              | 1        | 1          | 0     |
| `generic-api-key` (Gitleaks built-in) | 1        | 1          | 0     |
| **Total**                             | **27**   | **27**     | **0** |

The merged commit added the exclusion-matching code and the baseline report
documents; no new finding class or location appeared. The prior dispositions
(27 false positives, 0 confirmed, 0 unresolved findings) carry forward. This
addendum does not re-adjudicate any finding independently.

### Coverage and identity

Semgrep again reported `PARTIAL` with 581 targets skipped by its default
`.semgrepignore` (578 Go test files, two test/fixture directories, one minified
JavaScript file). Gitleaks reported `COMPLETE` over the same 13.09 MB source
scope. The declared exclusion list was unchanged.

### Artifacts

`.agent-sdlc/runs/security-baseline-postmerge-20261017/security/`:
`scan.json`, `scan.md`, `semgrep.native.json`, `gitleaks.native.json`, and
`coverage-detail.md`.

## Merged-main verification (`03426ce`, 2026-10-17)

This section records verification of the merged fix. The earlier baseline and
post-merge addendum are preserved unchanged.

### Commit and CI

| Field              | Value                                                                 |
| ------------------ | --------------------------------------------------------------------- |
| Merge commit       | `03426ce6ef2210f1a10468d96bf0980bfe641ea2`                            |
| Tree               | `3f7378fe2546de3236e66c6db16f169bc4b04532`                            |
| Working tree       | `CLEAN` (`dirty: false`)                                              |
| Merge PR           | [#44](https://github.com/imhttran/agentic-sop/pull/44)                |
| Pre-merge CI       | PR head `012ea94`; [run 39084322396](https://github.com/imhttran/agentic-sop/actions/runs/39084322396): 6/6 success |
| Merge-commit CI    | push run [39084599755](https://github.com/imhttran/agentic-sop/actions/runs/39084599755): 6/6 success |
| Checks             | `fmt, vet, build, test`; `race detector`; `real Semgrep + Gitleaks integration`; `installer + plugin packaging`; `windows (install.ps1)`; `tagged JEV evidence acceptance checks` |

The pre-merge run validated the PR head before merge. The merge-commit run is a
separate push workflow on `03426ce`; both completed successfully.

### Production baseline re-run

| Field              | Value                                                        |
| ------------------ | ------------------------------------------------------------ |
| Run                | `security-verify-03426ce-20261017`                           |
| Mode               | `report-only`                                                |
| Verdict            | `FAIL` (report-only; non-blocking)                           |
| Coverage           | Semgrep `PARTIAL`, Gitleaks `COMPLETE`                       |
| Findings           | 27 (26 SAST, 1 secret)                                       |
| Suppressed         | 0                                                            |
| Execution failures | 0                                                            |
| Total duration     | 3880 ms (Semgrep 2669 ms, Gitleaks 1083 ms)                  |

Scanner and rules identities:

| Adapter    | Version   | Rules identity                                                     | Configuration digest                                                | Sanitized report digest                                            |
| ---------- | --------- | ------------------------------------------------------------------ | ------------------------------------------------------------------- | ------------------------------------------------------------------ |
| Semgrep CE | `1.136.0` | `d3602b6590db16b37b035ddacdbefc612a804ee9840b2cf7b705cf064b2dada7` | `ce52a03b07bd5c5b997e06331dc47eae811bc6572c44fdc5e2d5f18435aad077` | `8056b89bf737dbfba744d8df686adf76cb21ac377713b5a63494ee6ee61c8ce4` |
| Gitleaks   | `8.30.1`  | built-in rules                                                     | `c308356740a0f7c36903ef409b2eca80472fac2ecab924c4ab5ed0c260a08ae8` | `69d1cd6d8a95a2f06fabc0431003d9f0de75f7c5080e4ecacdc66f328fddc82a` |

The shared policy identity remained
`936413f96f08eca7aa69cc70bf697ebd20a8898d1c28dfb121f01bc051ee6c47`.

### Findings comparison with the prior 27-finding baseline

| Rule ID                               | Prior | Verified | Delta |
| ------------------------------------- | ----- | -------- | ----- |
| `dangerous-exec-command`              | 14    | 14       | 0     |
| `no-direct-write-to-responsewriter`   | 3     | 3        | 0     |
| `cookie-missing-secure`               | 3     | 3        | 0     |
| `open-redirect`                       | 3     | 3        | 0     |
| `use-of-unsafe-block`                 | 2     | 2        | 0     |
| `string-formatted-query`              | 1     | 1        | 0     |
| `generic-api-key` (Gitleaks built-in) | 1     | 1        | 0     |
| **Total**                             | **27**| **27**   | **0** |

The comparison is by `(category, rule_id, path, line)` and also by fingerprint:
0 added, 0 removed, and all 27 fingerprints are identical. There is no finding
difference to explain.

The only observed differences are non-finding effects of the merged fix:

- Semgrep scanned 402 Go files instead of 400. The two new non-test files are
  `internal/security/exclude.go` and `internal/security/gitleaks_view.go`; neither
  produces a finding. The 581 `.semgrepignore` skips are unchanged.
- Gitleaks now stages a filtered view. It scanned 13,088,692 bytes (13.09 MB) in
  415 ms and reported that 4 files and 12 directories outside the declared scope
  were not staged. Adapter wall time rose from 562 ms to 1083 ms because of
  staging and the two new source files. Coverage remains `COMPLETE`.
- Semgrep wall time rose from 2586 ms to 2669 ms for the two additional files.
- Gitleaks native-report paths are repository-relative (`internal/question/atomic_test.go`);
  no temporary view path is retained.

The prior dispositions (27 false positives, 0 confirmed, 0 unresolved findings)
carry forward. This verification is a repeat measurement, not a new independent
vulnerability review.

### Focused exclusion fixtures

Both pinned real-CLI tests passed:

- `TestRealGitleaksExclusionsArePreScan`: an excluded file containing 2000
  synthetic tokens with a 1024-byte report bound completed with only the included
  `keep/secret.txt` finding. The control scan without exclusions failed at the
  1024-byte report bound, proving excluded content cannot exhaust the limit.
- `TestRealCLIExclusionConformance`: pinned Semgrep and Gitleaks agreed on
  anchored multi-component (`/internal/*/testdata`), wildcard subtree
  (`**/generated/**`), and anchored (`/root-only`) exclusions over the same tree.

The test log is retained at
`security/focused-fixtures.log` inside the verification artifact directory.

### Saved-evidence secret check

- Six verification evidence files were checked for every known synthetic token
  variant; none was present.
- The Gitleaks native report's `Secret` and `Match` fields are `[REDACTED]`.
- No temporary staged-view path appears in the retained native report.

### Remaining gaps

- Semgrep coverage remains `PARTIAL`: 578 Go test files plus two test/fixture
  directories and one minified JavaScript file are skipped by the scanner default.
- Required mode remains **not enabled and not ready**; the prerequisites in the
  original baseline still apply.
- Issue [#40](https://github.com/imhttran/agentic-sop/issues/40) remains open.
  Completed there: anchored/wildcard exclusion matching, Gitleaks scan-time
  exclusion, and cross-adapter conformance tests. Still open: gate classification,
  provisioning integrity (Semgrep hash pinning), coverage conformance/full-scan
  policy, and resumed-run native report handling.
- Cross-provider deduplication, dependency scanning, hosted analysis,
  runtime/network scanning, and asset discovery remain out of MVP scope.

### Artifacts

`.agent-sdlc/runs/security-verify-03426ce-20261017/security/`:
`scan.json`, `scan.md`, `semgrep.native.json`, `gitleaks.native.json`,
`coverage-detail.md`, and `focused-fixtures.log`.

## Related

- [Normative contract](../specs/SECURITY-SCANNER-ADDON.md)
- [Setup and extension guide](../guides/SECURITY-SCANNER-ADDON.md)
- [Earlier fixture-only dogfood](SECURITY-SCANNER-DOGFOOD.md)
- [Configuration reference](../reference/CONFIGURATION.md)
