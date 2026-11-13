# D4 operator approval packet — LLM03 scanner enforcement (OWASP gap 9)

**Status:** approval packet, read-only. **Nothing here is approved, accepted, or applied.**
`security.mode` is still `report-only`, `required_categories` is still empty, no baseline
entry is added, and the `internal/toolharness/harness.go:926` residual is **not accepted**.
The operator decides each numbered item independently.

**Supersedes:** the approval framing in
[PROPOSAL-LLM03-D4-DECISION-MATRIX.md](PROPOSAL-LLM03-D4-DECISION-MATRIX.md) §6 only. That
matrix remains the authoritative enumeration of the 45 findings and the 44 proposed
fingerprints; this packet does not restate or alter them.

**Evidence:** [llm03-d4-reconciliation.md](../reports/owasp-llm/llm03-d4-reconciliation.md)
independently reproduced the baseline at `origin/main` `bd2e0d45` and reconciled all 45
blocking findings. Every fingerprint in the matrix matched the scan and recomputed from
`(provider, rule, path, line, column)`.

> **Revision (2026-11-13, verified at `main` `986b9132`).** The residual moved from
> `harness.go:920` to `:926` when M1 added the environment scrub above `execCaptured`, so
> its fingerprint is now `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771`
> (was `semgrep:2e3c98676c0b89b137c13339ae43b6a5928dae9e7ff2fcf893de77038f473847`, preserved
> here as historical evidence). The other 73 Semgrep fingerprints are unchanged. The
> full-workspace report-only baseline is now **106 findings, 40 suppressed, 66
> unsuppressed, 50 blocking**: the residual's move did not change the count, and the five
> additional Gitleaks fixture findings are the decision documents' own quoted placeholders,
> pending disposition. Only the current fingerprint may be used for a new approval.

**Approval integrity.** Each group is approved as a _set digest_: SHA-256 over the group's
fingerprints, sorted, newline-joined. Approving "group C at digest X" is exact and immune to
a later edit that silently changes a member.

| Group                          | Rows (matrix §1) | Count | Set digest                                                         |
| ------------------------------ | ---------------- | ----- | ------------------------------------------------------------------ |
| A — Gitleaks fixtures          | 1–27             | 27    | `4596c8ccc60f4d1b58d8ce3e9195d1455089dd320fdb4b5fe00ebdbf9fab1ef9` |
| B — test-only Semgrep HIGH     | 33, 42–45        | 5     | `ec2380e348324bfa253015f29d2f51d213119cb6dfc318cbd4a176257308a0dc` |
| C — production Semgrep         | 28–32, 34–40     | 12    | `b61b105f2838651008a731a2e89ef3902e76b6ea1c579fc245ec069776d094bc` |
| D — residual                   | 41               | 1     | `8a665937345d8e191e18691e55e4e4a02c6df9d3a5962ea0402d9faf1113b8ee` |
| E — non-blocking MEDIUM (§3.4) | —                | 16    | `55268654137e2e2d65c7c06ecc101990f5157b07970fcf08543ecf6fb160fdce` |
| A+B+C together                 | —                | 44    | `ed4bd241f1e68f28489d0219f784bd6fc5014a3b5a816deca36f88c92e4a3f6a` |

## Corrections to the merged packet

Three substantive findings change the decision, and are the reason this packet exists:

1. **Option B is dominated, and this is now measured, not argued.** Required categories add
   coverage obligations; they do not narrow which findings enabled adapters contribute. A
   required `secrets` + `dependencies` gate blocks on the same 45 findings as `sast` would.
   The only way to make B pass without the same dispositions is to disable `sast`, which
   removes coverage of the exec/subprocess/file code this gap exists to review.
2. **The gate binds the SOP quality stage, not GitHub CI.** `security.mode: required` is
   enforced by `internal/quality`'s single gate. `.github/workflows/ci.yml` runs
   `security-integration.sh` (pinned scanners + integration tests), not a repository scan.
   The earlier "CI impact" column describes **lifecycle** impact.
3. **13 production sites were already reviewed and suppressed.** 16 baseline entries are now
   stale because their fingerprints drifted. Approval of group C re-affirms prior reviewed
   dispositions with refreshed, narrower reasons — it is not a first acceptance of unreviewed
   risk. It also _tightens_ expiry from the previous 2027-06-30 to 2027-03-31.

One further item is an option, not a correction: `internal/handoff/caveman/caveman.go` is
provably unimported, so deleting that package removes its finding without suppression (D4-4
alternative).

## D4-1 — Enforcement option

**Recommended: C, conditional on D4-2 through D4-5; A until then. Do not choose B.**

| Choice | Meaning                                                       | Measured result today                                     |
| ------ | ------------------------------------------------------------- | --------------------------------------------------------- |
| **A**  | Keep report-only, empty `required_categories`                 | `FAIL`, 45 blocking, delivery not blocked. Current state. |
| **B**  | Required `secrets` + `dependencies`                           | `FAIL`, **45** blocking — identical to C. Dominated.      |
| **C**  | Required `secrets` + `dependencies` + `sast`, workspace scope | `FAIL`, 45 → **1** after group A+B+C → **PASS** after D.  |

**Exact scope of C:** `security.mode: required`,
`required_categories: [secrets, dependencies, sast]`, `scope: workspace`, existing
`fail_on: [critical, high]`, `unknown_severity: indeterminate`, `stale_advisory_database: deny`,
existing three adapters and the existing `exclude` list unchanged.

**Supporting evidence.** Policy evaluation counts blocking findings across every enabled
adapter (`internal/security/policy.go`), and required categories only add per-language
coverage obligations (`requiredCoverageReasons`). The projection table in the reconciliation
report was produced with that function, including a positive control.

**Security consequence.** C is the only option that both blocks on real findings and
_guarantees_ that SAST, secrets, and dependency coverage were actually produced. A gives no
enforcement. B gives enforcement that is strictly narrower in assurance and equal in
blocking.

**Review condition.** Re-decide if D5 is not aligned, if a required adapter becomes
unavailable on a supported platform, or at the next baseline expiry.

**Implementation impact.** One config change (three keys) plus the approved baseline
entries. No code change.

## D4-2 — 27 Gitleaks fixture entries (group A)

**Recommended: approve, expiry 2027-06-30.**

**Exact scope.** Rows 1–27 of matrix §1 / matrix §3.1, digest
`4596c8cc…`. Fingerprint-exact; no path, directory, or rule-wide suppression.

**Supporting evidence.** All 27 matched values were inspected and are synthetic: one shared
`ASIA…` placeholder (6 findings), one shared `sk_test_…` fixture (6), sequential
placeholders (`glpat-abcdefghijklmnopqrst` ×3, `sk_live_abcdefghijklmnop`, the four
proposal "Fixture" column values), two explicit canary files, and `scrub_test.go:38`, whose
matched text is a natural-language phrase and is a false positive. The redaction hits are
input rows of `{name, in, want}` tables whose `want` is `[redacted]`.

**Security consequence.** Low: test and evidence fixtures. The residual risk is a real
credential later pasted into the same file. Fingerprint-exact entries keep that visible,
because a new line or column produces a new fingerprint that is not suppressed.

**Expiry and revalidation.** 2027-06-30. Revalidate on any change to the file, the redaction
policy, the Gitleaks version or rules, or when a file leaves test/report scope.

**Operational note.** Gitleaks fingerprints include line **and column**. Editing prose above
a fixture in `docs/plans/PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md` shifts the column and
invalidates that entry. The failure mode is fail-closed (the finding reappears and blocks),
but it will require re-review. Prefer not to edit these files casually once C is on.

**Implementation impact.** 27 entries in `.agent-sdlc/security-baseline.json`. This is the
first time the baseline carries `gitleaks:` fingerprints; `Baseline.Apply` is
provider-agnostic, so no code change is required, but a test should pin that behaviour.

## D4-3 — 5 test-only Semgrep HIGH entries (group B)

**Recommended: approve, expiry 2027-06-30.**

**Exact scope.** Rows 33 and 42–45, digest `ec2380e3…`. All are `_test.go` helpers:
`probeIsolation`, `TestLinuxIsolationCancellationKillsChild`, `needSandbox`,
`sandboxedOutput`, `TestIsolateScrubsSecretEnv` — verified at the flagged lines.

**Security consequence.** None for shipped behavior; these cannot run in production. The
residual risk is a new _production_ exec added to one of these test files, which the entry
would not cover because the fingerprint is exact.

**Expiry and revalidation.** 2027-06-30; remove if a helper moves into production code.

**Implementation impact.** 5 entries.

## D4-4 — 12 production Semgrep entries (group C)

**Recommended: approve, expiry 2027-03-31**, with the per-site reasons in matrix §2, and a
per-site revalidation trigger.

**Exact scope.** Rows 28–32 and 34–40, digest `b61b105f…`: the 12 production exec sites
other than `internal/toolharness/harness.go:926`. The six `sh -c` sites are
`internal/agent/command.go:104`,
`internal/handoff/caveman/caveman.go:90`, `internal/hipcompare/command.go:402`,
`internal/hipcompare/command.go:430`, `internal/review/ocr.go:57`,
`internal/testrunner/command.go:53`; the other six are structured argv.

**Supporting evidence.** Each site was re-read. The trust boundary is operator or project
configuration in 11 cases; no model text is interpolated into any `sh -c` string. The 16
stale entries show these sites were already reviewed (2026-11-03 bulk, 2026-11-08
individual), so this is a refresh, not a first acceptance.

**Security consequence.** These are intentional exec capabilities. The baseline records that
the injection path is absent; it does not remove the capability. Approving at 2027-03-31 is
_tighter_ than the previous 2027-06-30 for the same sites.

**Expiry and revalidation.** 2027-03-31. Revalidate on any change that introduces
interpolation of model, task, or manifest text into these command strings, and on any change
to the acceptance-command policy.

**Per-site choice — alternative for `caveman`.** `internal/handoff/caveman` is imported by no
production file and `SelectCompressor` has no production caller, so this `sh -c` site is
unreachable. The operator may instead authorize **deleting the unimported package** (and
retiring `handoff.ModeCaveman`) in a separate, narrowly scoped PR. That removes the finding
without suppression and without changing reachable behavior. It is a capability/API removal,
so it is not applied here. If chosen, this group is 11 entries, not 12.

**Implementation impact.** 12 entries (or 11 with the deletion) plus the 16 stale entries
retired in the same change, so the file does not accumulate inert entries.

## D4-5 — residual at `internal/toolharness/harness.go:926` (group D)

**Recommended: R1 (accept the bounded residual) with a baseline entry expiring 2027-03-31 —
and therefore Option C. If R1 is not acceptable, choose Option A (R2); do not choose B.**

| Choice | Meaning                                                                                                      | Consequence                                                                                 |
| ------ | ------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| **R1** | Accept the residual: one fingerprint entry, 2027-03-31, reason cites default-on isolation and command policy | Option C becomes achievable (`PASS`), contingent on the operator recording this acceptance. |
| **R2** | Keep Option A until the design changes                                                                       | Safe fallback; no enforcement.                                                              |
| **R3** | Remove or disable `run_command`                                                                              | Capability loss; separate product/security decision.                                        |
| **R4** | Authorize a code change to the exec mechanism or policy                                                      | Outside this gap's authorization; needs its own proposal.                                   |

**Exact scope.** One entry, fingerprint
`semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771`, digest
`8a665937…`.

**Supporting evidence.** This is the only blocking finding that is a true model-controlled
exec capability, and it is not a false positive. Current controls, verified by reading the
code and tests: `SplitCommand` rejects shell metacharacters and globs (no shell);
`checkCommandIn` requires `commandpolicy.Classify == SAFE` and confines paths to the
authorized root; isolation defaults on (Seatbelt on macOS, bubblewrap on Linux); Windows
refuses `run_command`. Pinned by `TestRunCommandIsolationIsOnByDefault`,
`TestRunCommandIsolationOnFailsClosedWithoutBackend`, `TestIsolationPolicyNotice`,
`TestIsolationDecisionUnsupportedPlatforms`, `TestRunCommandIsolationDeniesNetwork`, and
`TestRunCommandIsolationDeniesStateDirWrite`.

**Naming correction.** Matrix §5 cites `TestRunCommandIsolationUnsupportedPlatforms`, which
does not exist in the tree. The platform-refusal behavior is covered by
`TestIsolationDecisionUnsupportedPlatforms` and `TestRunCommandIsolationOnFailsClosedWithoutBackend`.
No behavior claim changes; the reference does.

**Security consequence, stated plainly.** Accepting R1 records that a model-driven command
executor is trusted to the extent of the policy and sandbox. The residual _returns_ if an
operator sets `SOP_AGENT_RUN_COMMAND_ISOLATION=off` together with
`SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK=1` on macOS/Linux (D4-5, PR #478), or on a platform
with no backend. Because the entry is fingerprint-exact, a _new_ model-reachable exec site is still
reported and still blocks.

**Expiry and revalidation.** 2027-03-31, and immediately on any change to `commandpolicy`,
`SplitCommand`, the isolation default, or the `run_command` surface.

**Implementation impact.** One entry. Without it, Option C remains `FAIL` with exactly 1
blocking finding.

**Boundary analysis.** A dedicated evaluation of every path that can disable isolation, what the
model can execute in each state, and the smallest containment is in
[PROPOSAL-LLM03-D4-5-ISOLATION-BOUNDARY.md](PROPOSAL-LLM03-D4-5-ISOLATION-BOUNDARY.md). It recommends a
confirmation-gated override and records the exact authorization; it changes nothing by itself.

## D4-6 — 16 non-blocking MEDIUM findings (group E)

**Recommended: no baseline entries.** Leave them visible and non-blocking.

**Exact scope.** Matrix §3.4, digest `55268654…`. All 16 are `_test.go` (verified): 7
`no-direct-write-to-responsewriter`, 4 `no-io-writestring-to-responsewriter`, 1 each
`unsafe-reflect-by-name`, `cookie-missing-httponly`, `cookie-missing-secure`,
`no-fprintf-to-responsewriter`, `string-formatted-query`.

**Supporting evidence.** They do not block under `fail_on: [critical, high]`, and required
coverage does not require a finding-free report. Option C reaches `PASS` without them.

**Security consequence.** A genuine MEDIUM in test code stays visible but non-blocking —
unchanged from today. If `fail_on` ever includes `medium`, these 16 become blocking and must
be dispositioned then.

**Expiry and review.** Not applicable while none are recorded. Revisit if the severity
policy changes or the report noise becomes a problem.

**Implementation impact.** None. Approving them instead would add 16 entries that must be
revalidated twice a year for no enforcement gain.

## Pending disposition — five Gitleaks fixtures in these documents

These five findings are **not** part of the original 45. They were introduced by the decision
documents themselves, which quote placeholder token shapes while explaining the fixture
category, and Gitleaks flags the quoted shape. They are synthetic, not credentials. No entry
is recorded and no decision is taken here; they are recorded so the operator can choose
between redacting the quoted shapes (no baseline entry needed) and baselining them.

| File | Line | Rule | Fingerprint |
| --- | --- | --- | --- |
| `docs/plans/PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` | 93 | `gitlab-pat` | `gitleaks:6587d88d04eb1d2edf7e2b3a5ab45ef82b2047ffe67e193a472b0f1cc9041696` |
| `docs/plans/PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` | 93 | `stripe-access-token` | `gitleaks:b3e345cb9b393167840c14eadd487e8f8f036126c27cf366c02621310b42194a` |
| `docs/reports/owasp-llm/llm03-d4-reconciliation.md` | 92 | `gitlab-pat` | `gitleaks:0e2afa016d2557ba5de881491a7eccbb564afa5d8981ba8af353e29031e9698a` |
| `docs/reports/owasp-llm/llm03-d4-reconciliation.md` | 93 | `stripe-access-token` | `gitleaks:a6ff52319833977476e1139483cbe6e591aeceeaea73c32d0156e61ae7fe860b` |
| `docs/reports/owasp-llm/llm03-d4-reconciliation.md` | 94 | `aws-access-token` | `gitleaks:d471a42320ffce3d911758ab7ac28c73de0fdbd653e873092da1d58c742e573a` |

Fingerprints are line- and column-based, so editing the surrounding prose moves them; the
values above were read from the verified workspace scan at `main` `986b9132`.

## Operational consequences

- **SOP lifecycle (`sop run`).** With C, the quality stage requires a `PASS` security scan;
  `FAIL` and `INDETERMINATE` both fail closed (`internal/quality`). Runs with no scan
  evidence fail as "not configured". Required mode also rejects changed-files evidence, so a
  full workspace scan is mandatory — expect roughly 8–16 s on this repository on darwin arm64.
- **GitHub CI.** Unchanged by C. CI keeps running `security-integration.sh`. Adding a CI gate
  is a separate workflow change and inherits the D5 drift below.
- **Local development.** Report-only is advisory today. Under C, a local scan that fails for
  environment reasons fails the task. A developer with `brew install gitleaks` gets `FAILED`
  on the binary pin (observed: Homebrew digest `f414bc2f…` vs pinned `ba52fb1b…`). The rules
  tree (`.agent-sdlc/security-rules`) and OSV database (`.agent-sdlc/osv-db`) are untracked
  and absent from a fresh worktree; they must be staged or the adapter is unavailable.
- **Scanner availability and failures.** Fail-closed everywhere: a `FAILED` or `UNKNOWN`
  adapter yields `INDETERMINATE`, never `PASS` (`internal/security/policy.go`;
  `TestOneSuccessfulAdapterCannotHideAFailedRequiredObligation`). A missing binary, a noexec
  temp mount, or a stale advisory database blocks rather than skips. This is correct, and it
  makes provisioning part of the gate.
- **New findings.** A finding at any new `(path, line, column)` is not suppressed by an exact
  fingerprint and blocks. Measured: one synthetic new HIGH → `FAIL`.
- **Expired entries.** An expired entry is reported as visible and stops suppressing
  (`Baseline.Apply`; `TestBaselineSuppressionAndExpiry`). The finding becomes blocking on the
  expiry date, so expiries are hard deadlines.
- **Fingerprint drift.** Line/column-based fingerprints mean unrelated edits can invalidate
  an entry. The observed result was 16 inert entries. Under C this is fail-closed but
  re-review heavy; the implementation plan adds a test that fails when an approved set no
  longer matches.
- **Platform.** Windows has no pins, no provisioning, and refuses `run_command`. No Windows
  scan evidence exists. The fingerprints are platform-independent, but this assessment is
  darwin arm64 plus CI linux/amd64.
- **D5 risk.** CI provisions Semgrep 1.136.0 while the config declares 1.183.0 (descriptive,
  not enforced). The 45 fingerprints came from 1.183.0. Align the version (D5-B) or verify
  the same 101/40/61/45 under 1.136.0 **before** enabling C, or the gate may block on
  findings the operator already reviewed.

## Smallest implementation plan for the approved outcome

Assumes C with groups A, B, C, D approved and E omitted.

1. **Baseline configuration** — `.agent-sdlc/security-baseline.json`:
   - add the 44 entries by exact fingerprint; `expires_at` as RFC3339
     (`2027-06-30T00:00:00Z` for A and B, `2027-03-31T00:00:00Z` for C and D);
   - replace the bulk `dangerous-exec-command` reason with the per-site reasons in matrix §2;
   - remove the 16 stale entries in the same commit; no entry may be path- or rule-wide.
2. **Enforcement** — `.agent-sdlc/config.yaml`: `security.mode: required`,
   `required_categories: [secrets, dependencies, sast]`, `scope: workspace`. No other key
   changes. `security.enabled` stays `true`, the `exclude` list is untouched.
3. **Expiry enforcement** — already implemented in `Baseline.Apply`; no change. A test pins
   it.
4. **Deterministic tests** (no scanner required):
   - `TestBaselineSuppressesGitleaksFingerprint` — provider-agnostic suppression, guarding
     D4-2.
   - `TestBaselineDoesNotSuppressDriftedFingerprint` — same path and rule, different line or
     column, remains unsuppressed.
   - `TestCommittedBaselineEntriesAreExactFingerprints` — every committed entry matches
     `^(semgrep|gitleaks):[0-9a-f]{64}$` with non-empty `rule_id`, `path`, `reason`, `owner`,
     `expires_at`; rejects a path-wide entry from ever being committed.
   - `TestCommittedBaselineCoversExactlyTheApprovedSet` — recomputes the set digest of the
     committed entries against the approved digests above, so an unreviewed addition fails.
5. **Positive controls** — a genuine new finding must still fail:
   - unit: inject an unsuppressed HIGH at a new path over the approved baseline →
     `FAIL` (measured today: `FAIL`, 1 blocking).
   - integration (tagged, pinned scanners): a fixture directory containing one new `sh -c`
     exec must produce a blocking finding under `--required`.
6. **Regression tests** — only approved findings are suppressed: assert the 44 fingerprints
   are suppressed, the residual is suppressed only if R1 was chosen, and no other finding in
   the recorded scan is suppressed beyond the 40 existing entries.
7. **Fail-closed verification** — `TestRequiredModeFailsClosedWhenAnAdapterFailsEvenWithNoFindings`:
   a `FAILED` adapter with zero findings must yield `INDETERMINATE`, and the quality gate
   must not pass; the existing `TestOneSuccessfulAdapterCannotHideAFailedRequiredObligation`
   and the `internal/quality` verdict table stay green.
8. **Evidence run** — on a clean tree with the pinned binaries,
   `sop security scan --required --scope workspace` must report `PASS` with `COMPLETE`
   coverage for all three categories, and `sop security inspect` must report
   `required_ready: true`. Record `scan.json` in the run artifacts.

**Sequencing:** D5 check → baseline + tests (one PR) → enforcement config (may be the same
PR, but the evidence run must be from a clean tree) → evidence. Enforcement must not be
enabled in a commit whose evidence run has not passed.

## What this packet does not do

- It records no approval, waiver, or residual acceptance.
- It adds no baseline entry, changes no `security.mode`, sets no `required_categories`,
  and changes no scanner command, pin, or exclusion.
- It does not modify PR #467/#468 or any historical decision; it supersedes their approval
  framing only.
- It applies no code fix, including the `caveman` deletion, which needs its own authorization.
