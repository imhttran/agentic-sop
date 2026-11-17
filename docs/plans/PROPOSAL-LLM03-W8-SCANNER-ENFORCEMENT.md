# Decision packet — W8 scanner enforcement (OWASP LLM03 supply chain)

**Status:** proposal, read-only. **No enforcement is enabled, no baseline entry is
added, and no residual is accepted by this document.**
**Base:** `origin/main` @ `33b07e33` (2026-11-16), darwin arm64, Go 1.27.1.
**Predecessors:** W1–W7 merged and verified. W8 was deferred by the operator on
2026-11-16; this packet is the decision material for enabling it.

> **Stage 1 implemented (2026-11-16, PR #503, merged at `e24c8108`).** The operator
> authorized §13 for stage 1 only: `security.mode: required`,
> `required_categories: [sast, secrets]`, three stale entries retired, one expiring
> entry added, and the `internal/archtest` guard landed first. Verified on the
> merged revision: verdict `PASS`, coverage `COMPLETE`, 98 findings / 82 suppressed
> / 16 unsuppressed / 0 blocking, 0 execution failures. Stage 2 (`dependencies`)
> remains unauthorized and the OSV provisioning prerequisite is unchanged.

**Companion evidence:**
[PROPOSAL-SCANNER-PIN-POLICY.md](PROPOSAL-SCANNER-PIN-POLICY.md) (items 2–5),
[DECISION-PACKET-OWASP-LLM-W6-W8.md](DECISION-PACKET-OWASP-LLM-W6-W8.md),
[execution-boundary-w6.md](../reports/owasp-llm/execution-boundary-w6.md),
[llm03-scanner-integrity-evidence.md](../reports/owasp-llm/llm03-scanner-integrity-evidence.md),
[git-output-integrity.md](../reports/owasp-llm/git-output-integrity.md).

## 1. Current verified state

Clean tree at `33b07e33` (`dirty=false`), pinned adapters on `PATH`:

```
verdict FAIL, coverage COMPLETE, execution failures 0
findings 98, suppressed 81, unsuppressed 17, blocking 1
mode report-only, required_mode ready=false
```

| Adapter | Version | Status | Coverage | Rules identity |
| --- | --- | --- | --- | --- |
| gitleaks | 8.30.1 | COMPLETED | COMPLETE | default rules |
| semgrep | 1.183.0 | COMPLETED | COMPLETE | `d90ff1b1…` (matches `rules_sha256`) |
| osv-scanner | 2.7.0 | COMPLETED | COMPLETE | n/a |

Coverage by category: `sast` covered, `secrets` covered, `dependencies` covered.

The 17 unsuppressed findings are:

- **1 HIGH** — `dangerous-exec-command` at `internal/toolharness/harness.go:970`,
  fingerprint
  `semgrep:3dcf22239e8d13ec5dcc3baab9b377f8f0e56ec5576ea07652217384910fea00`,
  CWE-94. This is the only finding in `fail_on` (`critical`, `high`).
- **16 MEDIUM**, all in `_test.go` files, all below `fail_on`. They do not block a
  gate and are unaffected by enforcement.

`security.mode: report-only`, `required_categories: []`, `fail_on: [critical, high]`,
`unknown_severity: indeterminate`, `stale_advisory_database: deny`.

## 2. Remaining technical blockers

| # | Blocker | Effect on W8 | Owner |
| --- | --- | --- | --- |
| B1 | The one HIGH finding is unsuppressed | Required `sast` can never pass | Operator (decision 3) |
| B2 | No publisher signature verification anywhere in the acquisition path | A digest proves bytes, not authorship; a compromised release would still be pinned | Operator (item 2 of the pin policy) |
| B3 | No committed binary pin for Semgrep (pip venv) or for Windows (Gitleaks, OSV) | Windows executes unpinned scanners; Semgrep relies on `pip --require-hashes` | Operator (item 5) |
| B4 | cargo-audit is **not** a configured adapter (manifest covers only `x86_64-unknown-linux-gnu` and `aarch64-apple-darwin`) | Not a W8 blocker, but its evidence is CI-only | Operator (item 5) |
| B5 | Offline OSV DB is machine-local and expires after 90 days | Required `dependencies` fails closed unless provisioned and refreshed | Operator/CI (decision 5) |
| B6 | Three baseline entries are stale | Misleading baseline; no effect on the verdict | Operator (decision 2) |
| B7 | `Harness.exec` / `execWithStatus` are non-isolated exec helpers in production code, currently reached only from tests | A latent unsandboxed path; weakens the justification for suppressing the call site | Code follow-up (decision 3 condition) |

## 3. Stale baseline entries (work item 2)

All three are `dangerous-exec-command` in `internal/toolharness/isolation_test.go`,
owner `imhttran`, expiring `2027-06-30`. They matched the old probe-style
`exec.Command(probe[0], …)` / `exec.Command(argv[0], …)` in that test file. W6
rewrote those helpers to name the backend literally, so the rule no longer fires
there and no finding carries these fingerprints.

| Fingerprint | Rule | Path |
| --- | --- | --- |
| `semgrep:11ef53417d1b9eedc8e24e1593bdfa38b6d8df035e3265fce7c61bff2aa3e5c2` | dangerous-exec-command | `internal/toolharness/isolation_test.go` |
| `semgrep:765bec591c74b4bc0a7bfbee12daaeb711a7123391832670c4d7d8d2a99d9f28` | dangerous-exec-command | `internal/toolharness/isolation_test.go` |
| `semgrep:fbedd5f6a2ed946cd639a20ce584f6ab65c3788767ea09cbb60e2c08294313b2` | dangerous-exec-command | `internal/toolharness/isolation_test.go` |

**Recommendation: retire all three** (option A), with the retirement recorded in
this packet and in the enforcement PR description. Historical approval is
preserved by keeping the fingerprints, rule id, path, owner and original expiry
in the record — the same convention PR #482 used for its 16 retirements. An entry
that suppresses a finding that cannot occur is not protection; it is a false
statement about the code that makes a future real finding easier to miss.

**Rejected alternative (B):** keep them until expiry. They would keep reporting as
unmatched, and any future finding in that file would need a *new* entry anyway.

Effect: baseline 84 → 81 entries; the scan's suppressed count is unchanged at 81
because none of them matches today.

## 4. The remaining HIGH finding (work items 3–4)

**What the call site is.** `harness.go:970` is the single `exec.CommandContext`
inside `execCaptured`. After M1 + G3 + G4 every model-reachable path reaches it
only through `execIsolated`:

- `run_command` → `execIsolated` (line 686)
- `git_status`, `git_diff` → `execIsolated` (line 445)
- `WorkingTreeFingerprint` → `execIsolated` (line 606)
- `WorkingTreeChanged` → `execIsolated` (line 506)

`execIsolated` calls `isolate()` first and returns a refusal when no backend can
be built, so there is no fallback branch. The remaining non-isolated caller,
`execWithStatus`, is reached only from `Harness.exec`, which is used only by tests
(blocker B7).

**Evidence that the mitigation is real, not asserted:**

- `TestGitToolsAreSandboxed`, `TestSafeBuildAndTestCannotBypassIsolation` — audits
  carry `isolate=` for git tools and for `go build|vet|test`.
- `TestGitToolsRefusedWithoutBackend`, `TestFingerprintFailsClosedWithoutBackend`
  — no backend ⇒ refusal / "changed", never an unsandboxed run.
- `TestSandboxedGitFilterCannotReachHost` — a repository clean filter cannot write
  SOP state or read a home credential; the unsandboxed leg is the positive control.
- `sandbox-linux` CI job — `pass=446 fail=0 sandbox_skip=0` with
  `SOP_SANDBOX_REQUIRED=1` on real bubblewrap.

**Options**

| Option | Effect | Assessment |
| --- | --- | --- |
| **A — keep blocking** | Required `sast` can never pass; W8 stays deferred | Honest, but leaves the gate permanently red |
| **B — narrow, expiring baseline entry** | Required `sast` can pass; the call site stays visible in provenance | **Recommended, with a guard** |
| **C — restructure the code so the rule stops firing** | Not possible honestly: `execCaptured` legitimately takes a dynamic argv; hiding it from Semgrep would be evasion |

**Recommendation: option B, conditional on a regression guard.** Add one entry
scoped to the exact fingerprint above, owner `imhttran`, expiring
`2027-03-31`, whose reason names the containment tests and states that it must be
re-reviewed if `execIsolated` is removed or a new non-isolated caller appears.

Condition: land a `internal/archtest` guard in the same change that fails when a
production file calls `execCaptured`/`exec.CommandContext` outside `execIsolated`,
and when `execWithStatus`/`exec` gain a non-test caller. That converts the
baseline entry from "trust us" into "enforced invariant". Without the guard, keep
the finding blocking.

Residual after the entry: the finding remains in the scan output as *suppressed*
with an expiry; it is not accepted and not removed.

## 5. Rejected alternative for the finding

Required mode with `sast` and an unsuppressed HIGH yields `FAIL` on every run, so
the gate would never be green. Recording it as an allowed failure would be a
weaker, undocumented policy change. Rejected.

## 6. Scanner integrity and provisioning evidence

| Prerequisite | State | Evidence |
| --- | --- | --- |
| Download / acquisition pins | **present** | `scripts/security/fetch-pinned.sh` re-checks every download and cache hit |
| Binary pin at execution | **present for Gitleaks, OSV** (linux/darwin × amd64/arm64) | `security.adapters[].binary_sha256_by_platform`; verified private copy executed (`TestPinnedExecutor`) |
| Binary pin for Semgrep | **absent** (pip venv) | mitigated by `semgrep-requirements.lock` with `--require-hashes` |
| Binary pin for Windows | **absent** | OSV manifest *does* carry `windows_amd64`/`windows_arm64` digests; the config has no entries |
| Rules pin at execution | **present** | `rules_sha256: d90ff1b1…`; `stageRules` gives Semgrep a verified private copy (`TestRulesPinStagesVerifiedTree`, `TestSemgrepRulesPinStagesTheTreeItRuns`) |
| Rules provenance | **pinned by commit** | `semgrep-rules-manifest.json`: upstream commit `17e31e11…`, tarball `9dfe6866…`, 66 per-file digests |
| CI ↔ config version alignment | **aligned** | CI: Semgrep 1.183.0, Gitleaks 8.30.1, OSV 2.7.0 — same as `security.adapters[].version` |
| Publisher signatures | **absent** | no cosign/minisign/GPG verification anywhere in `scripts/security/` or `internal/security/` |
| Fail-closed on a bad pin | **verified** | an unpinned Gitleaks binary produced `secrets FAILED`, `verdict FAIL`, coverage `UNKNOWN` — never `PASS` |

### Offline OSV database

- Provisioning is an explicit, separate action:
  `scripts/security/fetch-osv-databases.sh` writes
  `<dest>/osv-scalibr/<ecosystem>/all.zip` plus the source URL, SHA-256, size and
  acquisition time. A scan never downloads or refreshes.
- Freshness boundary: `osvScannerStaleMaxAge = 90 days`. A missing acquisition
  record is treated as stale.
- Policy `stale_advisory_database: deny` ⇒ stale DB forces `INDETERMINATE`, never
  `PASS`. With `allow` it records PARTIAL coverage and still cannot satisfy a
  required gate.
- **Prerequisite for required `dependencies`:** every environment that runs the
  gate must provision the DB and refresh it inside 90 days. CI's scanner job uses
  a controlled offline fixture; a required-mode CI job would need the same.

## 7. Enforcement baseline and expiry policy (work item 7)

**Baseline rules (proposed):**

1. Capture on a clean tree (`dirty=false`) at a merged commit; record the commit
   and tree in the PR so the baseline is reproducible.
2. An entry may only cover (a) a test-only finding, or (b) a production finding
   with a structural control and a regression guard.
3. Every entry carries `owner`, `reason`, `expires_at`. **Maximum lifetime 180
   days**; no permanent entries.
4. Expiry is enforced by the scanner (an expired entry stops suppressing), so an
   unreviewed entry becomes a blocking finding rather than silent protection.
5. Re-baseline after any scanner or rules bump, and re-verify fingerprints.
6. A suppression is never recorded to make a red gate green; it must name the
   control that makes the finding non-actionable.

**Proposed baseline content after W8:** 82 entries = 81 currently matched + 1 new
(the `harness.go:970` entry). Zero stale entries.

## 8. Staged activation (work item 8)

| Stage | Configuration | Entry condition | Observation |
| --- | --- | --- | --- |
| 0 (now) | `report-only`, `[]` | — | current 98/81/17/1 |
| **1** | `mode: required`, `required_categories: [sast, secrets]` | decisions 2 and 3 authorized; guard landed; clean-tree baseline captured | run 2 weeks; confirm no false `INDETERMINATE` from coverage |
| **2** | add `dependencies` | OSV DB provisioned and refreshed in every gate environment (CI + local docs) | run 2 weeks |

Rationale for `sast` + `secrets` first: both need no external database, both are
already `COMPLETE` on a clean tree, and both are the categories that catch the
classes this remediation targeted. `dependencies` is gated on data provisioning,
which is the only prerequisite that can silently regress.

**Do not** enable `mode: required` with an empty `required_categories`: config
validation rejects it (`required mode needs at least one required category`), and
`security.enabled: false` is explicitly **not** a required-gate exemption.

### Exact proposed configuration change (stage 1)

```yaml
security:
  enabled: true
  mode: required              # was: report-only
  scope: workspace            # unchanged; required mode rejects changed-files evidence
  required_categories:        # was: []
    - sast
    - secrets
  # everything else unchanged: fail_on [critical, high], unknown_severity
  # indeterminate, stale_advisory_database deny, baseline path, exclude list
```

### Exact proposed baseline changes

- **Retire** the three fingerprints in §3 (baseline 84 → 81).
- **Add** exactly one entry (81 → 82):

```json
{
  "fingerprint": "semgrep:3dcf22239e8d13ec5dcc3baab9b377f8f0e56ec5576ea07652217384910fea00",
  "rule_id": "dangerous-exec-command",
  "path": "internal/toolharness/harness.go",
  "reason": "Structurally controlled by mandatory isolation (M1+G3+G4): every model-reachable path reaches this call through execIsolated, which refuses when no sandbox backend exists. Guarded by internal/archtest; re-review if execIsolated is removed or a non-isolated caller appears.",
  "owner": "imhttran",
  "expires_at": "2027-03-31T00:00:00Z"
}
```

**Verified, not projected.** The exact proposal was exercised against an
alternate config and an untracked candidate baseline — the tracked config and
baseline were not modified, and the temp files were removed afterwards:

| Run | Setup | Result |
| --- | --- | --- |
| Proposal | `mode: required`, `required_categories: [sast, secrets]`, candidate baseline (82 entries) | **`PASS`**, coverage `COMPLETE`, `ready=true`, exit `0`, findings 98 / suppressed 82 / unsuppressed 16 |
| Negative 1 | same config, **current** baseline (1 blocking finding) | `FAIL`, exit `1`, "1 unsuppressed finding(s) at a blocking severity" |
| Negative 2 | proposal baseline with the new entry **expired** (`expires_at` in the past) | `FAIL`, suppressed drops 82 → 81, the finding returns |

The 16 remaining unsuppressed findings are all MEDIUM and below `fail_on`, so they
do not block. This is the evidence that stage 1 would be green on day one without
hiding anything.

## 9. Acceptance tests

**Positive (must pass):**

| Test | Asserts |
| --- | --- |
| `TestRealCLIScanReportOnlyAndRequired` | the real CLI honours both modes |
| `TestSecurityRequiredPassesOnlyOnPass` | required gate passes only on `PASS` |
| `TestRequiredGateRejectsPartialCoverage` | partial coverage cannot pass |
| `TestRequiredModeEvaluatesDetectedLanguagesSeparately` | a category-level pass cannot hide a missing language |
| `TestPinnedExecutor`, `TestRulesPinStagesVerifiedTree` | verified private copy is what executes |
| `TestBaselineSuppressionAndExpiry` | an unexpired entry suppresses, an expired one does not |
| **new** `TestRequiredGatePassesOnCleanTreeWithExpiringBaseline` | the proposed baseline yields `PASS` and the entry is inside `expires_at` |

**Negative (must fail closed):**

| Test | Asserts |
| --- | --- |
| `TestSecurityRequiredBlocksFailAndIndeterminate` | FAIL and INDETERMINATE both block |
| `TestSecurityRequiredUnconfiguredFailsClosed` | required without categories fails closed |
| `TestRequiredModeRejectsUnsupportedCoverage` | unsupported coverage ⇒ INDETERMINATE |
| `TestRunnerAllDisabledRequiredOutcome` | all adapters disabled ⇒ not clean |
| `TestGlobalDisableIsNotARequiredGateExemption` | `enabled: false` is not an exemption |
| `TestStaleDatabasePolicy` | stale DB under `deny` cannot pass |
| **new** `TestRequiredGateFailsOnUnpinnedBinary` | an unpinned scanner yields FAIL/INDETERMINATE, never PASS |
| (manual, verified here) | required mode + current baseline → `FAIL` exit 1; expired entry → the finding returns |
| **new** `TestRequiredGateRejectsChangedFilesScope` | a changed-files scan cannot satisfy the gate |
| **new** `TestArchtestModelReachableExecIsIsolated` | a new production caller of `execCaptured` outside `execIsolated` fails the build (the guard from §4) |

## 10. Rollback

1. Set `security.mode: report-only` and `required_categories: []` in one commit.
   This restores today's behaviour exactly; no state, schema, or artifact
   migration exists.
2. Baseline entries need no rollback: the new entry keeps suppressing until
   `2027-03-31` and then expires on its own; the three retirements only remove
   entries that match nothing.
3. Do **not** roll back by setting `security.enabled: false`: that is not a
   required-gate exemption and would misrepresent the state while leaving the gate
   active.
4. Verify the rollback with `sop security scan --json` expecting
   `mode: report-only`, `required_mode.ready: false`, and the §1 counts.

## 11. Compatibility impact

| Environment | Impact |
| --- | --- |
| **CI** | No workflow job runs `sop run`, so enforcement does not change CI verdicts today. A gate-running job would need the pinned scanners plus a fresh OSV DB; `scripts/checks/security-integration.sh` already provisions the pinned binaries, and the `sandbox-linux` job is unaffected. |
| **Local development** | `sop run` blocks unless Gitleaks 8.30.1, Semgrep 1.183.0 and OSV-Scanner 2.7.0 are on `PATH` **and** the OSV DB is provisioned within 90 days. `make security-integration` and `scripts/security/fetch-osv-databases.sh` cover this; the setup must be documented in the scanner guide before activation or every developer hits a fail-closed gate. |
| **Windows** | Gitleaks and OSV have **no committed Windows binary pins** (the OSV manifest carries Windows digests but the config does not), and `run_command` is unavailable under B1. Required mode would block `sop run` there. Options: add Windows pins and document support, or record Windows as unsupported for the gate. **Recommendation:** keep Windows out of the support claim until B3 is closed, and say so in the guide. |
| **Offline / air-gapped hosts** | Scans never download; the DB must be copied in and refreshed inside 90 days, or the gate is INDETERMINATE by design. |
| **Scanner unavailability** | Missing or unpinned binary ⇒ adapter `FAILED` ⇒ required gate INDETERMINATE. This is verified (`secrets FAILED` with a Homebrew Gitleaks), not assumed. |

## 12. Residual risk after W8 (unchanged by this packet)

- A digest proves the bytes, not the publisher; B2 stays open.
- Cargo-audit and Semgrep have no committed execution-time binary pin (B3).
- Three MEDIUM test-only findings stay unsuppressed and below `fail_on`.
- The `harness.go:970` call site remains visible and becomes suppressed only with
  an expiry and a guard, never accepted.

## 13. Operator authorization wording

Enabling enforcement requires this decision. Exact text:

> I authorize W8 scanner enforcement as specified in
> `docs/plans/PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md`, stage 1 only: set
> `security.mode: required` and `security.required_categories: [sast, secrets]`;
> retire the three stale `dangerous-exec-command` baseline entries identified in
> §3; add the single expiring baseline entry for
> `semgrep:3dcf22239e8d13ec5dcc3baab9b377f8f0e56ec5576ea07652217384910fea00`
> with `expires_at: 2027-03-31T00:00:00Z` and the reason in §8; and land the
> `internal/archtest` guard that fails when a production caller reaches
> `execCaptured` outside `execIsolated`. This authorizes that configuration and
> those two baseline operations only. It does not add publisher signature
> verification, does not add Semgrep or Windows binary pins, does not enable the
> `dependencies` category, and does not accept any residual risk.

Stage 2 (`dependencies`) needs its own authorization after the OSV provisioning
prerequisite is demonstrated. The decision-ready Stage 2 packet, with the measured
database matrix, the readiness/verdict divergence, and the exact configuration
change, is
[PROPOSAL-LLM03-W8-STAGE2-DEPENDENCIES.md](PROPOSAL-LLM03-W8-STAGE2-DEPENDENCIES.md).
