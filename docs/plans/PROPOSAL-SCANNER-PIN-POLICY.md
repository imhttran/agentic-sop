# Decision packet — scanner pin policy (OWASP gap 9, LLM03 supply chain)

**Status:** DECISION PACKET. Item 1 (committed per-platform binary pins) was
approved by the operator on 2026-11-12 and implemented in PR #461. The
hash-then-exec race for scanner binaries is closed in code (PR #410), and the
analogous rules-tree race is closed in PR #462. Items 2–5 below remain open
because they change trusted identities, enforcement defaults, or dependencies.

**Gap:** OWASP gap 9 in [OWASP-LLM-GAP-PROMPTS.md](OWASP-LLM-GAP-PROMPTS.md).
**Evidence:** [llm03-scanner-integrity-evidence.md](../reports/owasp-llm/llm03-scanner-integrity-evidence.md)
and [llm04-supply-chain.md](../reports/owasp-llm/llm04-supply-chain.md).

## What is already enforced

| Control | Where | What it proves |
| --- | --- | --- |
| Download pin | `scripts/security/fetch-pinned.sh` | A fresh download and a cache hit are checked against the committed SHA-256. |
| Acquisition pins | `fetch-osv-scanner.sh`, `fetch-cargo-audit.sh`, the Gitleaks case in `scripts/checks/security-integration.sh`, the Semgrep `pip --require-hashes` lock | The acquired bytes match a reviewed pin. |
| Binary pin at execution | `internal/security/pinned.go` (`stageVerified`); `internal/config/security.go` (`binary_sha256_by_platform`) | A configured binary pin (the single value or the running platform's entry) is copied and hashed, the copy is verified, and the copy is executed. A writer racing the original cannot change what runs. |
| Rules pin at execution | `internal/security/pinned.go` (`stageRules`) | When `rules_sha256` is set, the rules file or tree is copied and verified, and the scanner reads the copy. A symlink or other non-regular entry is refused. |
| Dependency version | `internal/security/osv.go`, `cargoaudit.go` | The running dependency scanner reports the exact pinned version. |
| Descriptive version | `semgrep.go`, `gitleaks.go`; `CONFIGURATION.md` | The SAST/secrets version is recorded; it is not enforced, by contract. |
| Required gate | `internal/security/policy.go`; `internal/cli/security_gate.go` | `mode: required` with `required_categories` blocks on `FAIL`/`INDETERMINATE`; report-only never blocks. |

The repository's tracked `.agent-sdlc/config.yaml` enables the Semgrep rules pin
(`rules_sha256: d90ff1b1…`, the manifest's `rules_dir_sha256`) and commits
per-platform binary pins for Gitleaks 8.30.1 and OSV-Scanner 2.7.0 on linux and
darwin (amd64 and arm64) (PR #461).

## Item 1 — Committed per-platform binary pins (decided)

**Decision.** Approved by the operator on 2026-11-12 and implemented in PR #461.
`security.adapters[].binary_sha256_by_platform` accepts `linux/amd64`,
`linux/arm64`, `darwin/amd64`, and `darwin/arm64`; unknown keys, malformed
digests, and setting both `binary_sha256` and the map fail the config load.
`PlatformBinarySHA256` resolves the running platform's entry before the adapter
identity and executor are built. A platform with no entry runs unpinned, as
before.

**Current implementation.** The tracked configuration commits digests for
Gitleaks 8.30.1 (the extracted archive binary, verified against the committed
archive pins) and OSV-Scanner 2.7.0 (the committed release-asset pins) on both
platforms and architectures. Cargo-audit and Semgrep have no committed
execution-time binary pin: cargo-audit's manifest covers only linux x64 and
darwin arm64, and Semgrep is a Python venv verified by `pip --require-hashes`.
Windows is not pinned; the scanner fetch scripts and the Windows `run_command`
refusal keep it out of scope.

**Threat.** An attacker who can write the tools directory replaces a scanner with
a same-version binary. The version probe passes, and the scanner runs with
SOP's privileges, so it can suppress findings or emit a clean report.

**Existing controls.** Acquisition pins, cache re-hashing, dependency version
enforcement, running-platform pin resolution, and stage-and-execute.

**Missing protection.** Cargo-audit and Semgrep still rely on acquisition pins and
the descriptive version only. A local build whose executable differs from a
release-pinned binary fails closed only where a pin is committed.

**Operational and maintenance cost.** One digest per supported platform/version,
refreshed on every scanner bump; the binary and rules pins must stay in step with
the fetch manifests.

**Operator authorization.** Recorded for item 1 (operator chat, 2026-11-12).
Adding cargo-audit or Semgrep pins, or a Windows pin, is a further trusted-identity
change and needs operator approval.

## Item 2 — Publisher signature verification

**Current implementation.** None. The manifests record a release tag and SHA-256;
fetches use HTTPS from GitHub; the Semgrep lock uses `pip --require-hashes`.

**Threat.** A compromised upstream account or release can publish a malicious
artifact that a reviewer then pins. A digest proves the bytes match a prior
decision; it does not prove who produced them or that the publisher intended them.

**Existing controls.** Same-source checksums, human review of manifest bumps, and
the pinned GitHub release/commit URLs.

**Missing protection.** An independent trust root: a publisher signature,
attestation, or notarization, plus a verifier in SOP.

**Operational and maintenance cost.** A new tool or library (for example
Sigstore/cosign), keyless-verification network/OIDC or a stored public key, a
verification policy for each scanner, CI and local provisioning changes, and
failure handling when a publisher does not provide a bundle. This packet does not
claim that any current scanner release publishes a signature or attestation.

**Recommended disposition.** Defer. Keep the explicit statement that a digest is
not authenticity.

**Operator authorization.** Required: new dependency and a trust-root decision.

## Item 3 — Semgrep rules integrity

**Current implementation.** Rules are fetched from a pinned upstream commit; the
tarball, every selected file, and the directory digest are verified; the tracked
config pins the directory digest; `stageRules` (PR #462) hands the scanner a
verified private copy. A changed tree fails closed before Semgrep starts. See the real-run
evidence in
[llm03-scanner-integrity-evidence.md](../reports/owasp-llm/llm03-scanner-integrity-evidence.md).

**Threat.** Local rules substitution or a race between the digest and the scanner's
read changes the analysis. Upstream commit compromise is the same threat as item 2.

**Existing controls.** Acquire-time and execution-time digest enforcement, symlink
refusal, and the manifest/repo equality test
(`TestRepoSemgrepRulesPinMatchesManifest`).

**Missing protection.** Rules are not committed (license), so the trust base is the
GitHub tarball plus the reviewed manifest. There is no publisher signature.

**Operational and maintenance cost.** The rules tree is copied once per pinned
scan (66 files in the current manifest), and the manifest and config pin must be
updated together. Upstream authenticity is item 2.

**Recommended disposition.** The execution-time staging is implemented; no new
operator decision is needed for it. Any change to the pin value or source remains
an operator decision.

**Operator authorization.** Not required for the staging fix; required for changing
the pinned commit or digest.

## Item 4 — Report-only versus required categories

**Current implementation.** `security.mode: report-only` and
`security.required_categories: []` in the tracked configuration. A required gate
needs at least one category (`inspectRequiredReadiness`, config validation);
`SOP run` therefore records scan evidence without blocking. Required mode is
implemented and covered by `TestRealCLIScanReportOnlyAndRequired`.

**Threat.** A failing or incomplete scan does not block a change, so a known
finding can be merged while the report records it.

**Existing controls.** Report-only records verdict, findings, coverage, and
provenance; the reviewed baseline suppresses entries only until they expire;
required mode fails closed on missing coverage.

**Missing protection.** Enforcement. No clean baseline with the pins in place is
recorded in this packet; the report-only scan used for the integrity evidence ran
on a dirty worktree and is not a gate baseline.

**Operational and maintenance cost.** A baseline run and triage; an expiry policy
for suppressions; developer friction for legitimate findings; CI time; and
offline database availability for the dependency category.

**Recommended disposition.** Decide on a clean baseline, then require `sast` and
`secrets` first and add `dependencies` only after offline database provisioning is
stable in CI and locally.

**Operator authorization.** Required: this changes the enforcement default.

## Item 5 — CI and local developer compatibility

**Current implementation.** CI provisions pinned tools under `runner.temp` and
re-hashes cached downloads; local runs use the configured `command` from `PATH`.
The CI script pins Semgrep 1.136.0, while the tracked config records 1.183.0. The
Semgrep and Gitleaks version pins are descriptive by contract, so this drift does
not fail a scan; dependency adapters enforce an exact version. The OSV and
cargo-audit databases are operator-supplied.

**Threat.** A pin that cannot be satisfied on a developer's platform causes false
failures, and CI/local version drift means CI evidence does not describe the
scanner a developer runs.

**Existing controls.** Per-target acquisition pins, exact versions for dependency
adapters, failure-closed behavior, and no silent skip when a prerequisite is
missing in the integration job.

**Missing protection.** One version policy across CI and the tracked config. The
per-platform pin mechanism now exists (PR #461), but Semgrep's CI/local version
drift remains.

**Operational and maintenance cost.** Coordinating version bumps; adding platform
matrix legs; provisioning the pinned release binaries locally instead of
distribution builds; documenting the `noexec` temp behavior when a staged copy
cannot run. On macOS arm64, the Homebrew Gitleaks binary (`sha256 f414bc2f…`)
differs from the committed `darwin/arm64` pin (`sha256 ba52fb1b…`), so the adapter
reports `FAILED`; that is the pin working as intended, not a regression.

**Recommended disposition.** Align the CI Semgrep version with the tracked config
and add a platform matrix now that binary pins are committed. Document that a
noexec temp mount is a fail-closed `ErrUnavailable`, not a bypass.

**Operator authorization.** Required for changing a CI-pinned scanner version or
adding a trust requirement.

## Exact operator decisions

| # | Decision | Options | Recommendation |
| --- | --- | --- | --- |
| D1 | Committed binary pins | **Decided:** option B. Per-platform pins for Gitleaks and OSV-Scanner on linux/darwin (amd64+arm64) are committed (PR #461). Cargo-audit, Semgrep, and Windows remain unpinned. | Recorded (operator, 2026-11-12). Any further pin is a new decision. |
| D2 | Publisher signatures | **A.** Defer; keep "a digest is not authenticity". **B.** Approve Sigstore/cosign verification with a new dependency and a per-scanner policy. | A. |
| D3 | Semgrep rules source | **A.** Keep the pinned commit + manifest digest (current). **B.** Change the pinned commit or digest. | A. |
| D4 | Enforcement | **A.** Keep report-only. **B.** Require `sast` + `secrets` after a clean baseline. **C.** Also require `dependencies` once offline databases are stable. | A until the baseline is recorded; then B. |
| D5 | Version alignment | **A.** Keep CI Semgrep 1.136.0 and the tracked 1.183.0 descriptive. **B.** Align CI and the tracked config to one version. | B; aligns CI with the committed pins. |

**How to record a decision.** Reply on the pull request with the decision IDs and
the chosen option (for example `D1 recorded; D2=A, D3=A, D4=A, D5=B`). If a durable SOP
decision record is wanted, record it with:

```text
sop memory add --decision "LLM03 scanner pin policy: D1=… D2=… D3=… D4=… D5=…" \
  --reason "operator decision on OWASP gap 9" --scope repository --source operator
```

## Outside SOP

Provider model provenance is outside SOP. A binary pin proves the bytes of a local
scanner executable; it does not prove where a model came from, and SOP does not pin a
model digest. See [llm04-supply-chain.md](../reports/owasp-llm/llm04-supply-chain.md)
and the platform record in
[llm03-scanner-integrity-evidence.md](../reports/owasp-llm/llm03-scanner-integrity-evidence.md).

## Operator decision record

- **D1 (committed per-platform binary pins): approved** (operator, chat,
  2026-11-12), implemented in PR #461. Gitleaks and OSV-Scanner are pinned on
  linux/darwin; the baseline ran. See
  [llm03-scanner-binary-pins.md](../reports/owasp-llm/llm03-scanner-binary-pins.md).
- **D2 (publisher signatures): option A, deferred** (operator, chat, 2026-11-12).
  No signature dependency is added; a digest is not authenticity.
- **D4 (enforcement): not decided.** The baseline is triaged in
  `llm03-baseline-triage.md` (PR #464);
  the choice of required categories is still open.
- **D3 and D5:** no decision recorded; the current rules source and CI version
  remain.
- **Original recommendation (2026-11-09, retained):** do the binary pins next;
  defer signatures; decide required categories on the baseline.

## What this change does not do

- It adds no new binary pin; the per-platform pins in PR #461 were approved by the
  operator and are not changed here.
- It does not enable a signature requirement.
- It does not change `security.mode` or `required_categories`.
- It does not claim provider model provenance.
