# LLM04 — AI supply-chain coverage (SOP Mac workstream)

Status: IMPLEMENTED; historical SOP outcomes PARTIAL (preserved); declared report-only scan PASS. Provider-owned assurances remain unverified.
Baseline: `imhttran/agentic-sop` `main` at
`0750e8be0cf5f318a173abb34c2b752ac131521c`.
Host: macOS `Darwin` arm64, Go `1.27.1`.
Execution date: 2026-11-09 (UTC).
Branch: `security/llm04-supply-chain`.

This report records what SOP actually loads, who owns each component, which
identity/integrity checks exist, and what remains provider-owned. It does not
claim publisher authentication, immutable model digests, full OWASP coverage, or
runtime isolation for normal agent execution.

## Method

- Read the current configuration (`.agent-sdlc/config.yaml`), security policy
  (`docs/specs/SECURITY.md`, `docs/specs/SECURITY-SCANNER-ADDON.md`), and the
  preceding hardening evidence.
- Inventoried manifests, provisioning scripts, configured adapters, provider
  selection, and optional artifact loaders from the current checkout.
- Ran the configured scanner and focused adapter tests. No tool or database was
  downloaded during verification; the operator-provisioned local inputs were
  reused.
- Reproduced each candidate gap with a focused test before changing code.

## Trust-chain inventory

| Component | Owner | Acquisition source | Version / digest | Verification method | Update process | Trust limitation |
| --- | --- | --- | --- | --- | --- | --- |
| `sop` CLI | SOP repository | `go build ./cmd/sop`; `install.sh` builds from this checkout | Source commit `0750e8be`; no release signature | Build from reviewed source; `go.sum` verified by the Go toolchain | `git pull` + rebuild/reinstall | No publisher signature or reproducible-build attestation; local toolchain is trusted |
| Go module set | Upstream modules + Go toolchain | Go module proxy / `go.sum` | `go.mod` pins; `go.sum` hashes; `go list -m all` output | Go toolchain verifies `go.sum` during build | `go get` / `go mod tidy` | SOP consumes compiled code, not modules at runtime; the toolchain's checksum/transparency configuration is operator-owned |
| Semgrep CE | Semgrep upstream | Operator-installed executable on `PATH` | Configured pin `1.183.0`; observed `1.183.0` | Adapter records the report version and rules identity; the pin is descriptive, not enforced (see GAP-01) | Operator install/update | Binary authenticity is not checked by SOP; a same-version replacement would not be detected |
| Semgrep rules | SOP provisioning | `scripts/security/fetch-semgrep-rules.sh` from a pinned GitHub commit | `scripts/security/semgrep-rules-manifest.json`: commit, tarball SHA-256, per-file SHA-256, directory digest | Fetch script verifies the tarball and files; adapter records `rulesIdentity` | Update manifest + re-run fetch | Runtime scan records the digest but does not fail if it differs from the manifest; GitHub tarball checksum is same-source |
| Gitleaks | Gitleaks upstream | Operator-installed executable on `PATH` | Configured pin `8.30.1`; observed `8.30.1` | Adapter checks executable availability and optionally records a rules identity; the pin is descriptive, not enforced (see GAP-01) | Operator install/update | Binary authenticity is not checked by SOP |
| OSV-Scanner | OSV / Google | `scripts/security/fetch-osv-scanner.sh` from a pinned GitHub release | `scripts/security/osv-scanner-manifest.json`: release tag + per-target SHA-256; configured pin `2.7.0`; observed `2.7.0` | Fetch script verifies SHA-256 and reported version; adapter probes `--version` before scanning and requires an exact match | Update manifest + re-run fetch | Runtime binary hash is not re-checked; GitHub release checksum is same-source; no publisher signature |
| OSV offline database | OSV + operator | `scripts/security/fetch-osv-databases.sh` from `https://osv-vulnerabilities.storage.googleapis.com/<ecosystem>/all.zip` | `osv-scalibr/<ecosystem>/all.zip` with a sidecar; observed Go digest `f283a685c89d47740426435b66b93254f865023643eec83a98d85578cf5915ce`, 12,828,976 bytes, acquired `2026-11-09T00:21:08Z` | Zip integrity at fetch; digest/size match the sidecar; 90-day freshness; sidecar acquisition time is authoritative after this change; malformed metadata fails closed | Re-run the fetch script | HTTPS plus same-source metadata; no signed database; provider advisory completeness is not proven |
| cargo-audit | RustSec | `scripts/security/fetch-cargo-audit.sh` from a pinned GitHub release | `scripts/security/cargo-audit-manifest.json`: per-target SHA-256; configured pin `0.22.2`; observed `0.22.2` | Fetch script verifies SHA-256 and version; adapter probes `--version` and requires an exact match; `--no-fetch` uses an explicit local database | Update manifest + re-run fetch | Runtime binary hash is not re-checked; advisory database identity/freshness come from the scanner report |
| Promptfoo (optional evaluation) | Promptfoo | Operator `npm install -g promptfoo@0.118.0` | Pinned `0.118.0`; observed `0.118.0`; config/dataset hashes recorded | Adapter probes `--version` and requires the pinned version | Operator install/update | Disabled in the routine path; live-adversarial evidence is not implemented and the live process is not evaltarget-isolated |
| Security policy and baseline | SOP repository / operator | Tracked `.agent-sdlc/config.yaml`, `.agent-sdlc/security-baseline.json`, `.semgrepignore` | Repository commit content | Strict config decoding; baseline exact fingerprints with expiry; policy identity covers excludes and stale policy | Reviewed commit | Repository write access can change policy; no signature |
| Model providers and model files | Ollama / oMLX / llama.cpp / remote provider | Provider runtime | Configured provider `ollama`, model `deepseek-v4.1-flash:cloud` | `sop providers` health/model listing; SOP sends the model name and does not load weights | Provider-owned | No immutable model digest is exposed to SOP; a remote model can change without a SOP-visible revision |
| Agent executables and harnesses | Operator / external agent | Operator configuration (`agent.harness: tool`) | Not versioned by SOP | Command policy for agent tool calls; no OS sandbox for normal agent execution | Operator-owned | The agent process is outside SOP's supply chain; the harness is operator-trusted configuration |
| Agent skills / integrations | SOP repository | `scripts/install/install-skills.sh`; `integrations/claude/` | Repository content | Installer copies/links repository files; no remote fetch | Re-run installer | The agent loads skills, not SOP; local filesystem access is operator-owned |
| Go/Rust toolchains and OS runtime | Operator | System/toolchain install | Go `1.27.1`; macOS arm64 | Operator-owned | Operator-owned | NOT_APPLICABLE: SOP does not acquire or verify the toolchain or OS |

## SOP-owned versus provider-owned

**SOP owns** the policy and baseline files, the adapter selection and execution
contract, the pinned provisioning manifests, the scanner invocation and argument
construction, the local artifact identity it records, the dependency inventory,
and the policy verdict. It does **not** own the upstream scanner binaries,
upstream rule content, OSV advisory completeness, provider model weights, remote
model revisions, the host toolchain, or the external agent harness.

Provider authentication is separate from model-visible content: the provider API
key is used only for the `Authorization` header (`internal/agent/provider.go`,
`postJSON`) and is not part of the request JSON. Model-visible content is the
task, context, tool output, and diff the harness sends; that distinction is
carried into the Plan 03 disclosure trace.

## Verification record

### Focused adapter and provenance tests

Command (run in this worktree, exit 0):

```sh
go test ./internal/security -run 'TestOSV|TestCargoAudit|TestStaleDatabasePolicy|TestPolicyIdentityIncludesAdvisoryDatabase|TestRequiredMode|TestNoApplicableContent|TestOSVDatabase' -count=1
```

Result: `ok github.com/imhttran/agentic-sop/internal/security`.

| Check | Evidence | Result |
| --- | --- | --- |
| OSV executable version pin is enforced | `TestOSVVersionMismatchIsNotClean` | PASS (existing) |
| OSV missing path / ecosystem / database is not clean | `TestOSVMissingDatabasePathIsNotClean`, `TestOSVMissingEcosystemDatabaseIsNotClean` | PASS (existing) |
| OSV malformed output, exit codes, truncation are not clean | `TestOSVExitAndOutputFailuresAreNotClean` | PASS (existing) |
| OSV uses `--offline`, controlled empty config, explicit local DB | `TestOSVUsesControlledConfigAndExplicitOfflineFlags` | PASS (existing) |
| Stale database policy denies by default and cannot PASS | `TestStaleDatabasePolicy`, `TestOSVStaleDatabaseFollowsPolicy` | PASS (existing) |
| Resume rechecks freshness and selection | `TestOSVResumeRechecksFreshnessAndSelection` | PASS (existing) |
| Missing Rust lockfiles and omitted inputs are not clean | `TestOSVRequiresEveryRustLockfile`, `TestOSVPreservesFindingsWhenAnObligationIsMissing`, `TestOSVExplicitSubsetReportsRemainingObligations` | PASS (existing) |
| Required mode cannot pass unsupported or missing coverage | `TestRequiredModeRejectsUnsupportedCoverage`, `TestRequiredModeEvaluatesDetectedLanguagesSeparately`, `TestNoApplicableContentCannotSatisfyRequiredCoverage` | PASS (existing) |
| cargo-audit version mismatch / missing pin / unavailable probe fail closed | `TestCargoAuditVersionMismatchFailsClosed`, `TestCargoAuditUnpinnedVersionFailsClosed`, `TestCargoAuditVersionProbeUnavailable` | PASS (existing) |
| OSV acquisition time decides freshness | `TestOSVDatabaseFreshnessUsesAcquisitionTime` | PASS (new) |
| Corrupt OSV metadata fails closed | `TestOSVDatabaseMalformedMetadataFailsClosed` | PASS (new) |
| A sidecar digest that disagrees with the archive fails closed | `TestOSVDatabaseDigestMismatchFailsClosed` | PASS (new) |
| A sidecar size that disagrees with the archive fails closed | `TestOSVDatabaseSizeMismatchFailsClosed` | PASS (new) |
| Unreadable OSV acquisition time fails closed | `TestOSVDatabaseUnreadableAcquisitionTimeFailsClosed` | PASS (new) |
| A database without metadata keeps the mtime fallback | `TestOSVDatabaseWithoutMetadataUsesModified` | PASS (new) |

### Reproduced gap and fix

**GAP-02 (fixed): OSV database freshness trusted a rewritable mtime and corrupt
metadata was ignored.** The adapter read `all.zip.metadata.json` but used the
archive's filesystem mtime for the 90-day freshness decision, and silently fell
back to mtime when the sidecar was malformed. Copying or restoring an archive
rewrites its mtime, so a stale database could look fresh; a damaged sidecar could
not fail the scan.

Reproduction before the fix (the new tests were run against the unmodified
adapter; each failed):

```text
--- FAIL: TestOSVDatabaseFreshnessUsesAcquisitionTime
    database with a 120-day-old acquisition time was treated as fresh because its mtime was current
--- FAIL: TestOSVDatabaseMalformedMetadataFailsClosed
    malformed acquisition metadata was accepted
--- FAIL: TestOSVDatabaseUnreadableAcquisitionTimeFailsClosed
    unreadable downloaded_at was accepted
```

Fix in `internal/security/osv.go`:

- The sidecar's `downloaded_at` is parsed into the database record and is
  authoritative for freshness when present.
- A sidecar with malformed JSON, an unreadable `downloaded_at`, or a digest or
  size that disagrees with the archive fails closed. The supported fetch script
  writes RFC 3339 acquisition times; a sidecar that uses another format now fails
  closed instead of silently falling back to mtime.
- A database with no sidecar keeps the documented mtime fallback, so existing
  operator layouts remain supported.
- The database identity records the acquisition time when present, so resume
  cannot silently reuse evidence that crossed the freshness decision.

After the fix, the same command reported `ok`. No configuration, required
category, severity rule, suppression, or autonomy setting changed. No download
occurred in any test.

### Configured scan on this baseline

Command (report-only, the configured routine mode):

```sh
sop security scan --report-only
```

Observed (final run after the fix):

- Verdict `PASS`; coverage `COMPLETE`; duration 9,005 ms.
- Commit `0750e8be0cf5f318a173abb34c2b752ac131521c`; working tree dirty with the
  Plan 04 change set (the scan reports the exact content digest).
- `gitleaks`: `COMPLETED`, coverage `COMPLETE`, explicit selection,
  repository-wide.
- `semgrep`: `COMPLETED`, coverage `COMPLETE`, explicit selection, Go.
- `osv-scanner`: `COMPLETED`, coverage `COMPLETE`, explicit selection, Go.
- OSV database: ecosystem `Go`, digest
  `f283a685c89d47740426435b66b93254f865023643eec83a98d85578cf5915ce`,
  size 12,828,976 bytes, acquired `2026-11-09T00:21:08Z`.
- Findings: 67 total, 53 suppressed by the reviewed baseline, 0 unsuppressed
  blocking findings.
- Required-mode readiness: `false` (projected `INDETERMINATE`) because
  `security.required_categories` is empty. Required mode is **not enabled**.

Artifacts are written under `.agent-sdlc/runs/security-*/security/scan.{md,json}`
and are machine-local/git-ignored. The final run used
`.agent-sdlc/runs/security-20261109T005939Z/security/scan.{md,json}`.

### Scanner versions observed on this host

```text
semgrep 1.183.0
gitleaks 8.30.1
osv-scanner 2.7.0 (osv-scalibr 0.5.3)
cargo-audit 0.22.2
promptfoo 0.118.0 (installed; evaluation disabled)
ollama 0.40.11 (provider runtime; model files provider-owned)
```

The installed Semgrep/Gitleaks/OSV/cargo-audit versions match the configured pins.
That is an observation on this host, not a runtime enforcement claim for the
SAST/secrets adapters (see GAP-01).

## Acceptance mapping and PARTIAL semantics

Authoritative contract: SOP-003 in `internal/taskfile/taskfile.go`
(`AcceptanceMapping`), `internal/planner/plan.go`
(`ParseAcceptanceMapping` and its validation), and `internal/outcome/criteria.go`
(`VerifyAcceptance`, `LegacyCompatibility`), consumed by
`internal/cli/outcome.go` (`deriveMappedOutcome`, `specCriterionMappings`).

A stage with acceptance criteria but no `### Acceptance Mapping` is a legacy
stage: the quality gate can pass and every deliverable and check can pass, but the
criteria remain unmapped, so `VerifyAcceptance` records `PARTIAL` and never
`VERIFIED`. `PARTIAL` here means the objective-level acceptance verdict is
unverified; it does not mean the configured build, test, lint, or acceptance check
failed.

The first adaptation collapsed each task's original acceptance bullets into one
criterion and used a Markdown-presence test as the only check. That test verified
file existence, not the substantive criteria. This revision restores every
original acceptance criterion and maps it to the evidence that actually applies:

- SUPPLY-001: five documentation criteria, all mapped to exact sections of this
  report (trust-chain inventory, provider/model boundary, scanner authenticity,
  hash-matching limitation, and NOT_APPLICABLE boundaries). No Markdown-presence
  test is used.
- SUPPLY-002: five criteria. Criteria 1-2 are verified by
  `go test ./internal/security -count=1`; criteria 3-5 are manual evidence in this
  report and the unchanged baseline/configuration (`no network`, `no new
  suppression`, `fix limited to the OSV adapter`).
- SUPPLY-003: five criteria. All five are manual evidence: this report's observed
  checks and configured scan, the unchanged report-only policy, the passing
  focused/required checks plus PR summary, and the cumulative matrix. The
  `go test ./internal/security -count=1` check remains a required check but does
  not by itself satisfy the documentation criteria.

The mapping was added after the first execution. `sop reconcile --accept-changed`
accepted the changed executed definitions while preserving status and attempt
history, so the historical outcome artifacts remain `PARTIAL`. This report does
not rewrite those outcomes and does not claim `VERIFIED` from the final gate
alone. Manual mappings require explicit operator acceptance decisions before a
future run can resolve them; the final gate is validation evidence, not
acceptance evidence.

## Local cache and execution verification (2026-11-09)

Plan 04's remaining trust question is the long-lived tools directory, not the
download path. Re-checked on `93c72857`:

- `scripts/security/fetch-pinned.sh` re-hashes a cache hit and fails closed on a
  mismatch; `scripts/checks/test-fetch-pinned.sh` covers a tampered cache entry and
  runs before provisioning in CI.
- CI persists only the download cache. `SOP_SECURITY_TOOLS_DIR` is under
  `runner.temp` and is provisioned per job.
- Provisioning verifies the archive or binary pin; execution verifies the version.
  Dependency adapters MUST enforce an exact version, while the Semgrep/Gitleaks
  pins are descriptive by contract (`docs/reference/CONFIGURATION.md`).
- A same-version replacement inside a writable tools directory is therefore
  undetected. That is a trust-policy decision, not a deviation from an enforced
  MUST; the options are in
  [PROPOSAL-SCANNER-BINARY-PINS.md](../../plans/PROPOSAL-SCANNER-BINARY-PINS.md).

## Residual gaps and boundaries

| Item | State | Owner | Note |
| --- | --- | --- | --- |
| GAP-01: Semgrep/Gitleaks version pins are descriptive | GAP | Security add-on contract | Configuration says the `version` field is descriptive unless an adapter enforces it; the specification only requires dependency adapters to verify `--version`. Enforcing these pins would be a contract/policy change, not a demonstrated contract violation. Recorded, not silently enabled. |
| Semgrep rules runtime digest is recorded, not enforced against the manifest | GAP | Security add-on | Fetch-time verification is implemented; a post-provisioning change is detectable in provenance but does not fail the scan. |
| Scanner binary runtime hash is not re-checked | GAP / decision | Security add-on | Provisioning verifies SHA-256; scans verify the version only. Same-version replacement is not detected. Options: [PROPOSAL-SCANNER-BINARY-PINS.md](../../plans/PROPOSAL-SCANNER-BINARY-PINS.md). |
| Future-dated `downloaded_at` | GAP (low) | Security add-on | A sidecar timestamp later than the host clock is accepted as fresh (clock skew or a crafted sidecar). This is recorded, not silently normalized. |
| Advisory database authenticity | NOT_APPLICABLE / residual | OSV/provider | HTTPS and a same-source checksum do not establish publisher authentication. No signing infrastructure was added. |
| Model provenance | NOT_APPLICABLE to SOP | Ollama/oMLX/remote provider | SOP sends a model name; it does not load weights or receive an immutable digest. A model name is not a digest. |
| Promptfoo live adversarial evaluation | NOT_RUN | Evaluation owners | Disabled in the routine path; live-adversarial coverage is not implemented (see `docs/reports/security-shift-left/aisec-live-assessment.md`). |
| Required security mode | NOT ENABLED | Security policy owner | `security.mode` remains `report-only`; `required_categories` remains empty. Enabling blocking is a policy decision. |
| Linux Bubblewrap runtime isolation | NOT_RUN on this host | Linux runtime owners | macOS Seatbelt evidence is separate and does not imply Linux protection. |
| Normal agent execution OS isolation | NOT_APPLICABLE / UNVERIFIED | Operator/agent environment | SOP does not sandbox the agent process; the execution-path matrix is unchanged. |

## macOS Seatbelt runtime evidence available for the Linux handoff

Plan 01's Linux handoff was not available in this checkout, so no Plan 01 test
list was identified. The existing macOS evidence was re-run on this exact baseline
to make it usable when the handoff names its tests. This is evidence for the
isolated replay and evaltarget paths only; it is **not** evidence of Linux
protection and does **not** generalize to normal agent tool calls, configured
agent commands, or verification commands.

Commands (macOS arm64, commit `0750e8be`):

```sh
go test ./internal/dqharness/ -run 'Isolation|Isolated|Bubblewrap|Seatbelt' -v -count=1
go test ./internal/evaltarget/ -run 'Isolat' -v -count=1
```

Observed:

- `internal/dqharness`: PASS. All isolation, confinement, private-mount,
  credential-scrub, outside-write, network-denial, timeout, and fail-closed tests
  passed under the Seatbelt backend. `TestIsolatedReplayUnixSocket`,
  `TestIsolatedReplayToolchainProcAndTemp`, and `TestReplayIsolationRequiredEnv`
  skipped with their documented Linux/opt-in prerequisites.
- `internal/evaltarget`: PASS for `TestDarwinIsolationBlocksOrdinaryWorkspace`,
  `TestDarwinIsolationBlocksNetwork`, `TestDarwinIsolationStripsSecrets`,
  `TestSandboxIsolatesCasesAndResets`, `TestIsolateEnvRemovesProductionSecrets`,
  and `TestIsolationRequiredByDefault`.

Exact handoff needed from Linux: the Plan 01 test names/paths, the platform and
commit Linux ran, and any Plan 01 scenario that must be exercised under Seatbelt.
Until then, Plan 01 is `NOT_RUN` on this host.

## Update procedure for this inventory

When a pinned scanner, rule set, database source, provider, or model changes:

1. Update the relevant manifest under `scripts/security/` (or the configuration)
   in a reviewed commit.
2. Re-run the explicit provisioning script for that component; never let a scan
   download it.
3. Re-run `sop security inspect`, `sop security scan --report-only`, and the
   focused tests above.
4. Update the version/digest row in this report and the cumulative matrix.

No new AI-BOM service was added; this Markdown inventory and the existing
manifests are the maintained format.

## OWASP mapping caveat

This work addresses the 2026 LLM04 supply-chain category as described by the
supplied Cycode article. It confirms the SOP-owned loading and execution paths
above, not a formal OWASP edition mapping or a certification claim. Confirm the
official OWASP edition before publishing a compliance claim.
