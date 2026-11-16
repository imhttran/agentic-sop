# Decision packet — OWASP W6–W8 (execution boundary, clean-filter policy, scanner enforcement)

**Status:** decision packet. **W6 is authorized and implemented (option B, with
mandatory sandbox-enabled integration verification); W7 is implemented; W8 stays
deferred.**
**Base:** `origin/main` @ `fe5b61cf` (2026-11-16), darwin arm64, Go 1.27.1, Git 2.50.1.

**W6 blocker (measured, 2026-11-16).** The GitHub `ubuntu-latest` runner image
ships no `bubblewrap` (verified against `Ubuntu2404-Readme.md`, image
`20261111.370.1`), and `sandboxArgv` fails closed when `exec.LookPath("bwrap")
fails. Under M1 + G3 every `run_command`, `git_status`, `git_diff`, and per-turn
fingerprint call would therefore be refused on CI, which fails the acceptance
criterion "`test-fast`and the scanner integration job stay green". The
repository already records this:`internal/ollamaagent/main_test.go` pins the
suite to policy-only because "CI has no bwrap". Resolving it needs one of the
options below, which is a decision, not an implementation detail.

**Companion evidence:**
[PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md](PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md) (§6, §8, §11),
[PROPOSAL-SCANNER-PIN-POLICY.md](PROPOSAL-SCANNER-PIN-POLICY.md) (items 2–5),
[git-output-integrity.md](../reports/owasp-llm/git-output-integrity.md) (residual 1),
[PLAN-OWASP-LLM-E2E-REMEDIATION.md](PLAN-OWASP-LLM-E2E-REMEDIATION.md) (W6–W8).

## Decision 1 — M1 + G3 + G4

The authorization text is §11 of the isolation proposal, verbatim. It authorizes
sandboxing every model-reachable execution (`run_command`, `git_status`,
`git_diff`, per-turn fingerprints), refusing where no backend exists, removing the
`SOP_AGENT_RUN_COMMAND_ISOLATION` / `..._UNSANDBOXED_ACK` override and its notice,
and denying model writes under `.git`. It does **not** accept the D4-5 residual,
add a baseline entry, enable Option C, or change `security.mode` /
`required_categories`.

**Why it is not optional hardening.** Today a `SAFE`, fixed-argv git call can make
git execute a repository-chosen program with SOP's full environment, with **no
operator action**. W1 removed four of the five mechanisms; a clean filter remains
and has no disabling flag.

| Aspect           | Detail                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Scope            | `isolation.go` (mandatory predicate; delete `IsolationEnv`, `UnsandboxedAckEnv`, `isOffValue`, `isAcknowledged`, `isolationOverrideOff`, `IsolationPolicyNotice`, `controlEnvNames`); `harness.go` (route `git`, `execFingerprintCmd`, `WorkingTreeChanged` through one `execIsolated`); `files.go` (deny `.git/` writes); `run.go`/`prompt.go` (drop the notice call sites)                                                                                                                                                                                                                                                                                  |
| Compatibility    | `git_status`/`git_diff` become unavailable on Windows and on Linux hosts where `bwrap` cannot start (for example Ubuntu 24.04's AppArmor default); `run_command` loses its escape hatch on those hosts; one sandbox exec per git call (2 tools + 2–3 fingerprints per turn, low-ms measured)                                                                                                                                                                                                                                                                                                                                                                  |
| Rollback         | Revert the predicate, the isolated-exec routing, and the `.git` deny. No schema, state, or artifact migration                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Acceptance tests | Existing: `TestIsolationDecisionSupportedPlatforms`, `TestIsolationDecisionUnsupportedPlatforms`, `TestRunCommandIsolationIsOnByDefault`, `TestRunCommandChildEnvironmentDropsIsolationControls`, `TestRunCommandIsolationOnWindowsRefuses`, and W1's git canary tests. To add: `TestModelCommandSandboxedDespiteOverrides`, `TestModelCommandRefusedWithoutBackend`, `TestSafeBuildAndTestCannotBypassIsolation`, `TestGitToolsAreSandboxed`, `TestGitToolsRefusedWithoutBackend`, `TestGitToolsIgnoreCleanFilters`, `TestSandboxedGitFilterCannotReadCredentialsOrEnv`, `TestGitMetadataIsReadOnlyToModelTools`, `TestOperatorConfiguredExecutionStillRuns` |
| Scan             | Still **1 blocking finding**; containment mitigates behaviour, not the call site, and it is not suppressed                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |

**Recommendation: approve, with one amendment.** Do **not** delete the W1 hardening
(`-c core.fsmonitor=false`, `--no-ext-diff --no-textconv`, the environment scrub)
when G3 lands, despite §6 saying to remove it. Containment stops a helper from
_reaching_ things; it does not stop `diff.external` from _replacing the reported
diff_. The flags preserve raw diff output, are already merged and tested, and cost
nothing.

**Operator decision (2026-11-16): approved**, including the amendment. Implementation
is blocked on the CI backend question below.

### Blocking options for W6

| Option                    | Effect                                                                                                     | Security note                                                                           |
| ------------------------- | ---------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| **A — give CI a backend** | Install `bubblewrap` and set `kernel.apparmor_restrict_unprivileged_userns=0` in the `check` and race jobs | Enables unprivileged user namespaces on the runner; confined to CI, keeps full coverage |
| **B — accept skips**      | Gate execution-dependent tests on the existing `needSandbox(t)` helper                                     | No security weakening, but Linux CI loses most agent-loop execution coverage            |
| **C — narrow G3**         | Sandbox `run_command` only, keep git tools policy-hardened                                                 | Rejects the authorized G3 scope                                                         |
| **D — defer**             | Hold W6 until CI can run a backend                                                                         | Boundary stays as merged today (git unsandboxed)                                        |

**Recommendation: A**, because it preserves both the authorized boundary and the
coverage, and the change is confined to CI. It cannot be validated on macOS; the
PR would need to prove it in the Linux job.

**Operator decision (2026-11-16): option B**, with mandatory sandbox-enabled
integration verification. The override and acknowledgement are removed and
isolation is mandatory; sandbox-dependent tests skip on a host with no runnable
backend, and a separate `sandbox-linux` CI job (Ubuntu 22.04, which permits
unprivileged user namespaces without touching AppArmor) runs them with
`SOP_SANDBOX_REQUIRED=1` so a skip is a failure. See
[execution-boundary-w6.md](../reports/owasp-llm/execution-boundary-w6.md).

**Trade-off.** The only real loss is Windows and `bwrap`-less hosts. The alternative
in the packet — removing the git tools on those platforms — is strictly worse, and
G2 is measurably not a boundary.

## Decision 2 — Clean-filter fingerprint policy

A clean filter runs on `status` and `diff` and can transform or **under-report**
changes. A hostile filter could make a working-tree fingerprint report "no change"
after a real edit, producing an `IMPLEMENT_NO_CHANGES` false negative.

| Option                                            | Behaviour                                                                                               | git-lfs                                                     | Security                            |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- | ----------------------------------- |
| A — label only (today)                            | Fingerprints unchanged                                                                                  | Works                                                       | A hostile filter can hide changes   |
| B — fail closed on any active filter              | `ChangedSince` always true when a filter is active                                                      | **Broken**: mutation detection dead on every LFS repository | Closes the hole                     |
| C — verified-LFS allowlist, fail closed otherwise | git-lfs treated as expected; any other configured driver makes the fingerprint inconclusive ("changed") | **Preserved**                                               | Closes the hole for everything else |

**Recommendation: Option C.** An active driver requires a configured
`filter.<name>.clean|smudge|process` command (already implemented). If the only
active driver is `lfs` with the standard `git-lfs` invocation resolved outside the
repository, mutation detection stays normal; any other configured driver makes the
fingerprint inconclusive and keeps the label.

**Operator decision (2026-11-16): approved and implemented.** See
[git-output-integrity.md](../reports/owasp-llm/git-output-integrity.md#option-c--verified-git-lfs-filter-policy-w7-approved-2026-11-16).

**Trade-off.** Custom non-LFS filters (encryption, normalisation) lose change
detection until explicitly allowlisted — bounded and visible. Option A is rejected
on security grounds: it leaves a model-controlled bypass of SOP's own evidence
integrity (LLM04/LLM09).

## Decision 3 — Scanner required-mode activation

All prerequisites are open:

1. **Decision 1 must land first.** With `fail_on: [critical, high]`, requiring `sast`
   today would block every run on the single `dangerous-exec-command` HIGH finding —
   the finding D4-5 exists to address.
2. **Items 2–5** of the scanner pin policy: publisher signatures, rules source, CI
   version alignment, and committed pins for cargo-audit, Semgrep, and Windows.
3. **Offline OSV database provisioning** must be stable in CI and locally before
   `dependencies` can be required.
4. **A clean-tree gate baseline** and a suppression expiry policy. (The integrity
   evidence was collected on a dirty tree.)
5. **An explicit decision about the `run_command` fingerprint**: leave it blocking,
   or record a justified baseline entry. A suppression is itself a policy change.

**Recommendation: do not activate required mode now.** Sequence Decision 1 → re-scan
→ require `sast` + `secrets` → add `dependencies` after item 3. Keep `report-only`,
`fail_on`, and `required_categories: []` unchanged until then.

**Trade-off.** Report-only means a known finding can merge. That risk is recorded and
bounded; activating early produces a permanently red gate, which trains reviewers to
ignore it.

## Status summary

| Decision          | Recommendation                                           | Operator decision             | State                                                                |
| ----------------- | -------------------------------------------------------- | ----------------------------- | -------------------------------------------------------------------- |
| 1 — M1+G3+G4      | Approve as written, **keep** the W1 hardening flags      | Approved 2026-11-16, option B | **Implemented** (Linux containment `NOT_RUN` until the new job runs) |
| 2 — Clean filter  | Option C (verified-LFS allowlist, fail closed otherwise) | Approved 2026-11-16           | **Implemented**                                                      |
| 3 — Required mode | Defer; sequence after Decision 1                         | Deferred                      | Not started; `report-only` unchanged                                 |

W6 and W7 are implemented. W8 remains deferred, so `security.mode: report-only`,
`required_categories: []`, and the single blocking `dangerous-exec-command`
finding are unchanged.
