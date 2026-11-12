# M1/M3 hardening and Semgrep alignment (OWASP gap 9)

**Status:** implemented on this branch; **D4 stays report-only**, no baseline entry is added
or removed, and the `internal/toolharness/harness.go` residual is **not accepted**.

**Operator authorization recorded here** (continuation of PR #472): implement M1 and M3;
retain the advisory unmatched-baseline diagnostic and do not make it fail validation; align
CI and configuration on Semgrep 1.183.0 and regenerate the hash-locked requirements; keep D4
report-only; do not accept the residual.

## 1. M1 — scrub the child environment when isolation is disabled

**Change.** `sandboxEnv` was split into `scrubbedEnvironment` (inherited environment minus
credential-named variables and credential-shaped values, never `nil`) and `sandboxEnv` (that
plus the throwaway `GOCACHE`). `runCommandWithStatus` now uses `scrubbedEnvironment` in the
`!isolationRequested()` branch, where it previously passed `nil` and therefore inherited
everything.

**Why.** With isolation off, `cmd.Env = nil` inherits the parent environment, so a
model-authored command launched through `go test` could read SOP's own provider credentials.
The sandbox denied those; the override silently restored them. The override still leaves the
network and credential files reachable (§6), but it no longer hands the process its secrets.

**Scope.** No classification change; no sandboxed run changes (the sandbox already scrubbed);
the startup notice now says "policy checks only and a scrubbed credential environment".

**Tests** (`internal/toolharness`), fixture runs `go test` and prints three variables:

| Variable | Kind | isolation on | isolation off |
| --- | --- | --- | --- |
| `SOP_TEST_ORDINARY_VAR=ordinary-value` | positive control | inherited | inherited |
| `SOP_TEST_API_KEY=canary-named-secret` | credential name | absent | **absent** |
| `SOP_TEST_NEUTRAL_NAME` (an AWS-key-shaped value) | credential shape | absent | **absent** |

`TestRunCommandScrubsCredentialEnvironment` covers both states;
`TestScrubbedEnvironmentIsNeverNil` guards the `nil`-means-inherit trap. Removing the M1
branch makes the isolation-off case fail with both leak assertions, so the test is a real
regression test rather than a tautology.

## 2. M3 — inline git configuration is no longer SAFE

**Change.** `classifyGit` returns `REQUIRES_APPROVAL` when git's **global-option prefix**
carries `-c`, the attached `-ckey=value`, or `--config-env`. The prefix ends at the
subcommand, which is the only place git reads those options.

**Why.** `git -c diff.external=… diff` classified SAFE and ran. A read-only subcommand could
therefore launch a program through git's own configuration: pager, diff driver, fsmonitor,
ssh command, hook path, alias, or filter. Verified directly: in a repository with a tracked,
modified file, `git -c diff.external=./evil.sh diff` created the marker file.

**Preserved workflows.** `-c` after the subcommand belongs to the subcommand, so
`git log -c` (combined diff) and `git grep -c` (count) stay SAFE, as do `git status`,
`git diff`, `git -C repo diff`, and `git log --oneline`. Force-push stays `DENIED`.
`REQUIRES_APPROVAL` is the "explicit classification" route: the harness refuses it because it
executes only SAFE, and a caller with an approval path can still see the class.

**Relation to existing governance.** The acceptance-check path (`rejectDangerousForm`) already
rejected `-c` anywhere and still runs first, so its error message and behavior are unchanged.
This change brings the classifier in line with that existing intent; it loosens no control.

**Tests.** `TestClassifyGitConfigOverride` (17 cases: nine injections, eight preserved
workflows) and `TestRunCommandRejectsGitConfigInjection`, which refuses four injected forms at
the harness, asserts a legitimate `git status` still runs, and asserts the marker is absent.
Reverting the classifier makes the harness test fail with
`"git -c diff.external=./evil.sh diff" was allowed through SAFE classification: exit 0`.

## 3. Semgrep 1.183.0 alignment

**Change.**

- `scripts/checks/security-integration.sh`: `SEMGREP_VERSION="1.183.0"`; the interpreter guard
  now accepts 3.10 and 3.11 only (Semgrep 1.183.0 requires `>=3.10`) and the message says so.
- `scripts/security/semgrep-requirements.lock`: regenerated for `semgrep==1.183.0`.
- `internal/security/integration_test.go`: expected version constant moved to `1.183.0`.
- `.agent-sdlc/config.yaml`: already declared `1.183.0`; unchanged.
- `docs/guides/SECURITY-SCANNER-ADDON.md`: records the regeneration command, because the
  repository had no documented generator.

**Regeneration.** `pip-compile --generate-hashes` with no header, index URL, or annotations,
matching the existing artifact's format. The dependency closure grows from 45 to 65 packages
(Semgrep 1.183.0 adds `mcp`, `pydantic`, `httpx`, `starlette`, `uvicorn`, and others) and
`opentelemetry-instrumentation` moves from `0.46b0` to `0.58b0`.

**Reproducibility.** A clean Python 3.11 venv installed from the new lockfile with
`--require-hashes` reports `semgrep 1.183.0` and runs. That venv carries `setuptools 84.0.0`,
which has **no** `pkg_resources`, and Semgrep starts anyway — the `0.46b0`/`pkg_resources`
trap recorded in PR #472 is gone. The lockfile pins no `setuptools` and no longer needs to.

**Behavior.** The regenerated lockfile and the reference Homebrew 1.183.0 produce an
**identical fingerprint set** (106 findings each, 0 differences) on the same tree. Provenance
records `scanner_version 1.183.0` in both runs.

**Gate.** `scripts/checks/security-integration.sh` completes with exit 0 and reports
`semgrep=1.183.0 gitleaks=8.30.1 cargo-audit=0.22.2 osv-scanner=2.7.0`; the real-CLI
integration tests pass. With the host default `python3` (3.9) the script fails closed with a
clear message instead of provisioning an incompatible interpreter.

## 4. Verification summary

| Check | Result |
| --- | --- |
| `go test ./internal/commandpolicy/ ./internal/toolharness/ ./internal/security/... ./internal/quality/...` | pass |
| `go test ./internal/cli/` | pass (64.6 s) |
| `go vet ./internal/...`, `gofmt -l` | clean |
| `check-doc-links.sh` | 0 broken links |
| `security-integration.sh` | exit 0, `semgrep=1.183.0` |
| Python 3.9 guard | exit 1 with the version message |
| Lockfile vs reference 1.183.0 fingerprints | identical |
| M1 regression | fails without the change |
| M3 regression | fails without the change |

## 5. Two consequences for the existing D4 records

**(a) M1 moved the residual's fingerprint.** `exec.CommandContext` shifted from
`internal/toolharness/harness.go:920` to `:926`, because the scrub branch adds lines above
`execCaptured`.

| | Fingerprint |
| --- | --- |
| Packet / matrix (PRs #467/#468/#471) | `semgrep:2e3c98676c0b89b137c13339ae43b6a5928dae9e7ff2fcf893de77038f473847` |
| **Current tree** | `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771` |

Exactly one of the 74 Semgrep fingerprints changed; the other 73, including all 12 production
exec sites, are unchanged. The old fingerprint now suppresses nothing. **Any approval of the
residual must use the new fingerprint.** This is the drift the advisory diagnostic from #472
reports, and the reason the packet must be refreshed before a baseline is recorded.

**(b) Merged PR #471 added five Gitleaks findings.** The reconciliation and packet quote
placeholder tokens to explain them, and Gitleaks flags the quoted shapes:

| File | Line | Rule | Fingerprint |
| --- | --- | --- | --- |
| `docs/plans/PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` | 93 | `gitlab-pat` | `gitleaks:6587d88d04eb1d2edf7e2b3a5ab45ef82b2047ffe67e193a472b0f1cc9041696` |
| `docs/plans/PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` | 93 | `stripe-access-token` | `gitleaks:b3e345cb9b393167840c14eadd487e8f8f036126c27cf366c02621310b42194a` |
| `docs/reports/owasp-llm/llm03-d4-reconciliation.md` | 92 | `gitlab-pat` | `gitleaks:0e2afa016d2557ba5de881491a7eccbb564afa5d8981ba8af353e29031e9698a` |
| `docs/reports/owasp-llm/llm03-d4-reconciliation.md` | 93 | `stripe-access-token` | `gitleaks:a6ff52319833977476e1139483cbe6e591aeceeaea73c32d0156e61ae7fe860b` |
| `docs/reports/owasp-llm/llm03-d4-reconciliation.md` | 94 | `aws-access-token` | `gitleaks:d471a42320ffce3d911758ab7ac28c73de0fdbd653e873092da1d58c742e573a` |

They are synthetic placeholders, not credentials, and they are **not** changed here because
the documents belong to the preserved decision packet. They can be redacted (removing the
literal shapes) or baselined. Until one happens the tree's blocking count is **50** rather
than 45: the original 45 categories with the residual at its new fingerprint, plus these five.

## 6. Remaining unsandboxed exposure

M1 narrows the isolation-off state; it does not close it. With
`SOP_AGENT_RUN_COMMAND_ISOLATION=off` on macOS/Linux:

- **Network is available.** A model-authored test can reach the network.
- **Credential files are readable.** `$HOME` credential locations are no longer masked, and
  `HOME` is not a secret name, so it survives the scrub.
- **Arbitrary code still runs.** `go test`, `go build`, `go vet`, `go fmt`, and
  `go test -exec` remain SAFE; scrubbing the environment does not stop execution.
- **Repository configuration still reaches external programs.** M3 covers `git -c` only. A
  `.git/config` or `.gitattributes` the model can write can still make `git status` or
  `git diff` run a configured `fsmonitor`, `diff.external`, or filter. Closing that needs a
  different control (forbid writes under `.git`, or run read-only git with
  `-c core.fsmonitor=false` and a fixed attribute source).
- **Windows still refuses** `run_command` outright, so none of this applies there.

The residual therefore remains **unaccepted** and unchanged in kind: bounded by the sandbox by
default, unbounded when the sandbox is disabled.

## 7. D4 decisions still required

| ID | Decision |
| --- | --- |
| **D4-1** | Enforcement option A / B / C. Still report-only; nothing enabled here. |
| **D4-2** | Approve the 27 Gitleaks fixture entries (set digest `4596c8cc…`). |
| **D4-3** | Approve the 5 test-only Semgrep HIGH entries (`ec2380e3…`). |
| **D4-4** | Approve the 12 production Semgrep entries (`b61b105f…`), or authorize the `caveman` deletion instead. |
| **D4-5** | Residual still open. If accepted, it must name **`semgrep:3076a053…` at line 926**, not the packet's `semgrep:2e3c9867…`. |
| **D4-6** | The 16 non-blocking MEDIUM findings. |
| **NEW** | The five Gitleaks findings in §5(b): redact the quoted shapes, or baseline them. |
| **B-1** | Confirmed by this authorization: the drift diagnostic stays advisory. |
| **V-1..V-3** | Resolved for 1.183.0 and the Python guard; **V-4** (an enforced Semgrep version check) is still open. |
