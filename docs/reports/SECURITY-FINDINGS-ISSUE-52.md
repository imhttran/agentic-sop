# Security findings review — issue #52

**Type:** Historical evidence (non-normative)

**Issue:** [imhttran/agentic-sop#52](https://github.com/imhttran/agentic-sop/issues/52)

**Source commit:** `4a575deec004deb8dde627fa0c0aff7a63df5289` (`main`)

**Tree:** `01db20b49892b66c3021cdb4598f6ee9e4028b9a`

This report dispositions the findings from the report-only scan recorded in issue
#52. It is a review result, not a claim that the repository has no
vulnerabilities and not a required-mode security gate.

## Result

| Field | Value |
| --- | --- |
| Original scan | `security-20261018T050632Z` — verdict `FAIL`, coverage `PARTIAL` |
| Reported findings | 24 `HIGH`, 13 `MEDIUM` (37 normalized findings) |
| Production-scope re-scan | `security-52-final2` — verdict `FAIL`, coverage `PARTIAL` |
| Production findings after exclusions | 14 `HIGH`, 12 `MEDIUM` (26) |
| Confirmed application vulnerabilities | **0** |
| Code fixes | 0 (no confirmed vulnerability required a boundary change) |
| Reviewed suppressions | 1 narrow inline fixture suppression (test helper only) |
| Execution failures | 0 |
| Unresolved findings | 0 confirmed; 26 reviewed false positives remain visible in the scan |

The production re-scan still reports `FAIL` because the reviewed
`dangerous-exec-command` findings are not suppressed in the scanner. The prior
reviewed baseline made the same choice:
[SECURITY-SCANNER-BASELINE.md](SECURITY-SCANNER-BASELINE.md). A `FAIL` verdict here
means "blocking-severity findings are present and unreviewed by the scanner", not
"a confirmed exploit was found".

## Source separation

The issue scan used workspace scope and included three non-production trees. The
review re-scanned production scope with these exclusions:

```text
internal/security/testdata
internal/security/integration_test.go
internal/security/security_test.go
/.run  /.agents  /.claude  /.playwright-mcp  /.zed
/sop-harden-001  /sop  /sop-*
/.DS_Store
```

| Content | Disposition |
| --- | --- |
| `internal/security/testdata/real/seeded/insecure.go:7` | Synthetic SAST fixture; expected to match. It never runs as production code. |
| `sop-harden-001/internal/...` (8 matches) | Legacy copy in a separate worktree, not this checkout's production source. |
| `internal/question/atomic_test.go:183` | Test-helper child environment; synthetic idempotency key, no credential. |

The original scan's `gitleaks` finding on `internal/question/atomic_test.go:183`
was the only secrets result. After the inline fixture suppression described below,
the production re-scan's `gitleaks` adapter reported `no leaks found`.

## Tools and policy identity

Both scans were report-only, workspace scope, `fail_on: [critical, high]`,
`unknown_severity: indeterminate`.

| Adapter | Actual version | Status | Coverage | Evidence identity |
| --- | --- | --- | --- | --- |
| `gitleaks` | `8.30.1` | `COMPLETED` | `COMPLETE` | config digest `6117052bc8fb03f544c51d00dbb8cd44089ae20b4b59db6780c5162b9846aa58` |
| `semgrep` | `1.181.0` | `COMPLETED` | `PARTIAL` | rules identity `5d5765f46b60a1ad57ac0f3876e888a5fbbac5407dd0b8ea27472a5189f5bb30`; config digest `c188447a84ff073045964287dbaffae5be969a9104406acfe5a213c75d569a90` |
| `cargo-audit` | not configured in the production re-scan; original scan `FAILED` | — | `UNKNOWN` | no local advisory database |

Policy identity for both adapters: `d4e10b777df0ae44640fae3bbb05a1e001df08c9de04e835958608bc76882f7b`.

The local venv directory is named `semgrep-1.136.0-venv`, but the executable
reports `1.181.0`; the provenance records the actual version. No network fetch or
advisory-database update was performed.

## Finding dispositions

Rule confidence for `dangerous-exec-command` is `LOW` in the Semgrep rule
metadata. Every row below was checked against the source at the recorded commit.

### `dangerous-exec-command` — 14 `HIGH`

| # | Location | Disposition | Source-backed evidence |
| --- | --- | --- | --- |
| 1 | `cmd/deceleval-recovery/main.go:204` | Not actionable | `adapter` and `provider` are developer CLI flags; structured argv, no shell. |
| 2 | `internal/agent/command.go:81` | Not actionable | `a.command` comes from `SOP_AGENT_COMMAND` / legacy `AGENT_SDLC_AGENT_COMMAND` (lines 15-23, 56-65), trusted operator configuration; the request is JSON on stdin. `sh -c` is the documented harness contract (SECURITY.md §2). |
| 3 | `internal/cli/outcome.go:636` | Not actionable | Reached only with an argv first authorized by `authorizeAcceptanceCommandWith` (lines 489-515): `toolharness.SplitCommand` rejects shell syntax, `rejectDangerousForm` rejects tooling-injection flags, and `commandpolicy.Classify` must return `SAFE`. The authorized argv runs without a shell. |
| 4 | `internal/controller/sopclient/commands.go:38` | Not actionable | `c.bin` is the fixed local `sop` executable; verb and arguments are structured and passed directly, no shell. |
| 5 | `internal/decision/command/command.go:133` | Not actionable | `a.argv` is trusted configuration supplied to `New(name, argv)` (lines 78-89); request JSON goes to stdin; structured argv, no shell. |
| 6 | `internal/decision/commandprofile/connector.go:172` | Not actionable | `c.argv` is configuration; the generated request path is a private `MkdirTemp` file passed as a separate argument; structured argv, no shell. |
| 7 | `internal/decisionadapter/providers/julia/runner.go:239` | Not actionable | `r.Command` is configured (`JULIA_INFERENCE_CMD` or repository helper); inputs are JSON on stdin; structured argv, no shell. |
| 8 | `internal/github/command.go:31` | Not actionable | `name` is `git` or `gh`; arguments are structured and passed directly, no shell (SECURITY.md §2 requires this form). |
| 9 | `internal/handoff/caveman/caveman.go:89` | Not actionable | `ShellRunner` is documented as trusted configuration; the bundle is JSON on stdin, never interpolated into the command. |
| 10 | `internal/ragclient/exec.go:17` | Not actionable | The executable is a validated absolute path; the request is JSON on stdin; structured argv, no shell. |
| 11 | `internal/review/ocr.go:55` | Not actionable | `SOP_REVIEW_COMMAND` is documented as trusted local configuration (lines 19-26); the request is JSON on stdin. `sh -c` is intentional. |
| 12 | `internal/security/exec.go:47` | Not actionable | Adapter argv comes from `security.adapters[].command`; structured argv, no shell, and the child receives a sanitized environment. |
| 13 | `internal/testrunner/command.go:34` | Not actionable | The command is configured validation (SECURITY.md §2 trusts it like a Makefile); `dir` is checked with `os.Stat` and must be a directory before execution. |
| 14 | `internal/toolharness/harness.go:829` | Not actionable | Model-requested commands are tokenized by `SplitCommand` (lines 604-644), path-confined and policy-checked by `checkCommandIn` (lines 661-715), and must classify `SAFE`; `execCaptured` only runs the already-authorized argv, no shell. |

Re-review trigger: if any `SOP_*_COMMAND` value, validation command, decision
adapter argv, or security adapter argv becomes settable by an unauthenticated or
remote caller, the disposition changes from false positive to a real
command-injection risk.

### `no-direct-write-to-responsewriter` — 3 `MEDIUM`

Locations: `internal/controller/web/activity_stream.go:227`, `:230`, `:260`.

Disposition: **Not actionable**. The handler sets
`Content-Type: text/event-stream` and writes Server-Sent Events. The dynamic
values are JSON-marshaled (`json.Marshal` at lines 222 and 253), so HTML escaping
does not apply. The only non-JSON field in an SSE frame is `payload.Cursor`, which
`activityCursor` derives as a 32-character lowercase hex hash
(`internal/controller/sopclient/activity_window.go:28-41`). No newline can enter
the `id:` line. A raw `Write` is required for SSE framing; `html/template` is not
an applicable control.

### `cookie-missing-secure` — 3 `MEDIUM`

Locations: `internal/controller/web/middleware.go:45`,
`internal/controller/web/session.go:377`, `:389`.

Disposition: **Not actionable**. `Secure` is set to `isHTTPS(r)`, not omitted.
`isHTTPS` returns true for TLS and trusts `X-Forwarded-Proto: https` only from a
loopback peer (`session.go:353-373`). Plain HTTP on localhost intentionally does
not set `Secure`; a plain-HTTP production deployment is unsupported and insecure
by construction, not a missing control in this code. The rule matches the absence
of a literal `Secure: true`.

### `open-redirect` — 3 `MEDIUM`

Locations: `internal/controller/web/session.go:327`, `:454`, `:479`.

Disposition: **Not actionable**.

- `:327` redirects to the fixed path `/login?next=<url.QueryEscape(request URI)>`;
  the destination host is not user-controlled.
- `:454` and `:479` pass the next value through `safeNext` (`session.go:417-430`):
  it requires a leading `/`, rejects `//`, CR/LF/backslash, and any parsed scheme
  or host, then falls back to `/projects`. Browser-relevant encoded-separator
  forms (`%2f`, `%5c`, `%09`) remain percent-encoded in the redirect target and
  are not resolved as a host by the URL parser; literal control characters are
  rejected by `url.Parse`. Existing coverage:
  `TestSafeNextRejectsOpenRedirect`.

### `use-of-unsafe-block` — 2 `MEDIUM`

Locations: `internal/decision/commandprofile/procattr_windows.go:42`, `:43`.

Disposition: **Not actionable**. The `unsafe.Pointer`/`unsafe.Sizeof` calls are
the required Windows Job Object FFI pattern for
`windows.SetInformationJobObject`. The pointed-to struct is constructed locally;
no attacker-controlled memory is involved.

### `string-formatted-query` — 1 `MEDIUM`

Location: `internal/store/handoff.go:53`.

Disposition: **Not actionable**. `handoffColumns` is a package-level constant
(line 70); all runtime values use `?` placeholders. No user input is concatenated
into the SQL string.

### `generic-api-key` — 1 `HIGH` (original scan only)

Location: `internal/question/atomic_test.go:183-184`.

Disposition: **Not actionable; narrow inline suppression added.** The match is the
test helper's child-process environment assignment `QUESTION_KEY=<synthetic key>`.
The value is `k-0`/`k-1` created at line 203 and exists only to test idempotency.
No credential is present. The reviewed fixture is marked with
`//gitleaks:allow: synthetic idempotency key, not a credential`; a targeted
Gitleaks 8.30.1 run over the file reports `no leaks found`. The production
re-scan's `gitleaks` adapter also reports `no leaks found`.

## Validation

| Check | Command | Result |
| --- | --- | --- |
| Format | `gofmt -l internal/question/atomic_test.go` | no output |
| Build | `go build ./...` | pass |
| Vet | `go vet ./...` | pass |
| Tests | `go test ./... -count=1` | pass (all packages) |
| Targeted secret check | `gitleaks detect --no-git --source internal/question/atomic_test.go --redact` | `no leaks found` |
| Production scan | `sop security scan --auto --report-only --json` | `FAIL` / `PARTIAL`; 26 reviewed findings; 0 execution failures |

The production scan was run with the candidate SOP binary against the fix
worktree. `cargo-audit` was not configured in that run because no explicit local
advisory database exists. The original issue scan recorded `cargo-audit` as
`FAILED` for the same reason; that is **not** a clean dependency result.

## Coverage gaps

- Rust Semgrep rules are unavailable. The production-scope detection found only
  Go; Rust source outside the excluded fixture trees is not analyzed.
- `cargo-audit` did not run: no pinned local advisory database was configured, and
  implicit fetch/update is disabled. Rust dependency auditing remains a coverage
  gap.
- Several non-production Go modules lack `go.sum`
  (`docs/reports/pre-performance-closure/workloads/A`, `.../B`,
  `testdata/fake-decision-provider`); dependency coverage for those modules is
  incomplete.
- Semgrep skipped 602 `.semgrepignore` targets in the production re-scan and
  reported `PARTIAL` coverage.
- The scanned tree was dirty because the reviewed source change (the inline
  suppression) was present; the base commit and tree are recorded above. A
  post-commit clean-tree scan is recorded with the pull request.

A passing scan would not prove the absence of vulnerabilities. The dispositions
above are evidence-backed reviews of the reported rules at the recorded source.

## SOP execution limitation

The installed `sop` binary reports `sop dev` and does not include the merged
reliability repairs. This review was completed manually with direct Go, Git, and
scanner commands. The candidate binary was used only to execute the report-only
scan; no SOP lifecycle, approval, commit, or state mutation was performed. No
`.agent-sdlc/state.db`, `.run/` state, or disposable fixture was modified.

## Artifacts

Original issue scan (local, ignored):

```text
.agent-sdlc/runs/security-20261018T050632Z/security/scan.json
.agent-sdlc/runs/security-20261018T050632Z/security/scan.md
.agent-sdlc/runs/security-20261018T050632Z/security/gitleaks.native.json
.agent-sdlc/runs/security-20261018T050632Z/security/semgrep.native.json
```

Production re-scan (local, ignored):

```text
.agent-sdlc/runs/security-52-final2/security/scan.json
.agent-sdlc/runs/security-52-final2/security/scan.md
.agent-sdlc/runs/security-52-final2/security/gitleaks.native.json
.agent-sdlc/runs/security-52-final2/security/semgrep.native.json
```

No secret values are reproduced in this report or in the saved evidence.
