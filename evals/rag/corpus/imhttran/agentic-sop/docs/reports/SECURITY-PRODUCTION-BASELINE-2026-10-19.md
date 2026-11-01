# Production Dependency Vulnerability Baseline — 2026-10-19

**Type:** Historical evidence (non-normative, point-in-time)
**Date:** 2026-10-19
**Tested revision:** `1dbaa3a307fe970c0d13a928141b456d78aa6105` (`origin/main`)
**Includes:** PR #79 merge `225b76799201b436a6c0b9cbfc3b97bb08d114dd` and correction head `920bf37b58782fe79a0306f39683c6cd82d4c057`
**Related:** [#40](https://github.com/imhttran/agentic-sop/issues/40), [PR #79](https://github.com/imhttran/agentic-sop/pull/79), [PLAN-Security-Production-Baseline.md](../history/plans/PLAN-Security-Production-Baseline.md)
**Mode:** report-only (`required` disabled)
**Raw artifacts:** `.agent-sdlc/runs/prod-baseline-1dbaa3a3/security/` (local, gitignored)

This is a production-data baseline, not a fixture demonstration. It does not
overwrite the earlier controlled-database evidence in
[SECURITY-SCANNER-RESIDUAL-FINDINGS.md](SECURITY-SCANNER-RESIDUAL-FINDINGS.md)
or [SECURITY-DEPENDENCY-COVERAGE.md](SECURITY-DEPENDENCY-COVERAGE.md).

## Merge verification

| Check | Result |
| --- | --- |
| PR #79 correction head `920bf37` is an ancestor of `origin/main` | yes |
| PR #79 merge commit `225b7679` is an ancestor of `origin/main` | yes |
| Current `origin/main` at scan time | `1dbaa3a3` (PR #81 merged after #79) |
| PR #79 head CI (`920bf37`) | 12/12 check runs succeeded |
| PR #79 merge-commit CI (`225b7679`) | 11/11 check runs succeeded (separate run; includes the race detector) |

The PR-head and merge-commit checks are distinct. The earlier PR #74 head
(`cca30c9`) is not the tested revision; PR #79 delivered the correction.

## Build identity

The scan CLI was built from a detached, clean worktree at the tested revision:

| Property | Value |
| --- | --- |
| Binary | `sop` built from `./cmd/sop` |
| SHA-256 | `59c2679bc9c25398de1efed64cc5ea9304ade7c0105c22871948513f8873fc63` |
| Go toolchain | `go1.27.1` |
| Platform | `darwin/arm64` |
| `vcs.revision` | `1dbaa3a307fe970c0d13a928141b456d78aa6105` |
| `vcs.modified` | `false` |

An initial scan from the primary worktree was discarded because untracked
operator scratch (`.run/*`, `sop-harden-001`) made the content identity dirty and
introduced duplicate obligations. The final baseline uses a clean worktree and
reports `content.dirty: false`.

## Advisory data and scanner provisioning

Scanners were provisioned with the repository's pinned manifests. Advisory data
was downloaded in a separate preparation step; scan execution was offline.

| Input | Identity |
| --- | --- |
| OSV-Scanner | `2.7.0`, `darwin_arm64` asset SHA-256 `d4926ef8da134332543be276dff157e4c170f99ad91fc48e89448a8816bdf70b`; embedded osv-scalibr `0.5.3`, commit `bfe2f84abd2f09fe0b83ba7f6dad40d296762f40` |
| cargo-audit | `0.22.2`, `aarch64-apple-darwin` asset SHA-256 `ec7ca4263769593df4d909be85b94a6b79efa2897be5d2bb8ebd516e823175af` |
| OSV `Go` | `https://osv-vulnerabilities.storage.googleapis.com/Go/all.zip`; SHA-256 `0a62d12d2edaf0e2194f45e679affdbe116a83a3dc4560ff001bacaa353fa5b7`; 12,471,958 bytes; downloaded `2026-10-19T04:10:26Z`; local mtime `2026-10-19T04:10:05Z` |
| OSV `crates.io` | `https://osv-vulnerabilities.storage.googleapis.com/crates.io/all.zip`; SHA-256 `955bc0e2c5df43595f4d79616bde4fc90743fa428dfb485ac64760481471ca86`; 3,614,156 bytes; downloaded `2026-10-19T04:12:29Z`; local mtime `2026-10-19T04:12:12Z` |
| RustSec advisory database | `https://github.com/RustSec/advisory-db` at commit `999ec89d5d3f10f20be14750ec8c2d8b44e0ac9b` (committed `2026-10-17T15:23:59Z`); archive SHA-256 `503a3f9e3b3976ce96e976fcaec69f90e0be63ce7ba6ad8a6645afc7ca944fa3`; 1,318 advisories; content digest `5847a94e365563b79456c871703aefd2f9fadf54a209ef9e54823006b7999ed1`; local acquisition `2026-10-19T06:13:28Z` |

The RustSec `crates.io` OSV archive was prepared but not exercised by a
production Rust input at this revision. The RustSec database was used for the
cargo-audit adapter's explicit no-applicable-content path only.

## Scope

The tested revision has one maintained production dependency input:

| In scope | Kind | Input | Present |
| --- | --- | --- | --- |
| repository root | Go module | `go.mod` | yes |

OSV reported 12 dependency packages for the root module. No `Cargo.lock` or
maintained Rust package exists at this revision.

Documented exclusions:

| Exclusion | Reason |
| --- | --- |
| `internal/security/testdata` | Synthetic scanner fixtures (detection, cargo-audit, and real-scanner fixtures); not application or tool packages. |
| `testdata` | Test-only fake decision provider fixture. |
| `docs/reports/pre-performance-closure/workloads` | Performance benchmark workloads, not maintained application packages. |
| `.run` | Tracked SOP reference/demonstration projects and local operator scratch; not production packages. |

The primary worktree also contained untracked `.run/prod-security`, `.run/tunnel`,
and `sop-harden-001`; the clean worktree removed them from the scan. No
production input was excluded to obtain a passing result, and no lockfile was
generated.

## Baseline result

Temporary configuration (not committed; project configuration unchanged and
report-only):

- explicit `osv-scanner` `2.7.0` for Go/Rust, explicit `cargo-audit` `0.22.2` for
  Rust;
- explicit OSV and RustSec database paths;
- workspace scope, `stale: deny`, no automatic selection, no SAST or secrets
  categories.

| Result | Value |
| --- | --- |
| Verdict / coverage | `PASS` / `COMPLETE` |
| Findings | 0 (0 suppressed, 0 execution failures) |
| Duration | 479 ms total; OSV 425 ms, cargo-audit 8 ms |
| Content identity | commit `1dbaa3a3`, tree `9aab3725...`, dirty `false` |
| OSV obligation identity | `3ddd24a3706a281d4e7d9c55fa0a32d39709e9eaaae210662b15a4d1f12de292` |
| OSV configuration digest | `de15a108b8b9c5b4fd657390c041875cf974476ad276a2c54a6dd920603b2b65` |
| OSV lockfile identity | `path=go.mod;language=go;digest=94ef62a22066af7eb6d65263d5cae0af3fb6f526075e400e5aafd9c71c11af1e` |
| OSV database identity | `ecosystem=Go;digest=0a62d12d...;size=12471958;modified=2026-10-19T04:10:05Z` |
| OSV native report digest | `d5bc7c46e84a67594f12999aa1e3e73e3dd9863292f1faa4e527f8ebbf9bdc92` |
| cargo-audit configuration digest | `e1cbd7df1f95deb2c4eed78414ea594004d01f492876cda5c3adcfb0615dd1c4` |
| cargo-audit obligation identity | empty obligation set (`e3b0c442...`, SHA-256 of empty input) |
| Policy identity | `633b2d5681e8d115c850badeb26aa5c9f47dd06c3b3398775ad0d0e2a6c9ce0f` |
| Required-mode readiness | `false`; projected `INDETERMINATE` (no required category configured) |

Adapter detail:

- **osv-scanner** — `COMPLETED`/`COMPLETE`, exit 0, Go only, 12 packages,
  0 advisory groups. It loaded the local Go database and made no network
  request.
- **cargo-audit** — `COMPLETED`/`COMPLETE` with
  `no_applicable_content: true`; the scanner was not executed because no
  production Rust input exists.

Declared overlap is `dependencies`/`rust`: `cargo-audit` and `osv-scanner` are
both explicitly selected. The overlap was not exercised because there is no
production Rust input; no duplicate scan occurred. The overlap diagnostic's
"will run" wording describes the declared selection, not the executed
no-applicable-content path.

## Triage

There are **no findings** to triage at this revision.

- Advisory IDs, affected versions, input files, fixed versions, and severity
  evidence: not applicable (zero findings).
- Vulnerable-version matches versus demonstrated reachability: no matches;
  reachability is **not evaluated** and is not inferred from the absence of
  findings.
- Withdrawn advisories: none reported.
- Unsupported inputs: no maintained production Rust input; cargo-audit reported
  this explicitly rather than treating it as a clean dependency scan.
- Execution failures: none.
- Transitive dependencies were not dismissed; OSV scanned the resolved module
  graph, and no advisory was reported for any of the 12 packages.
- No dependencies were updated, no findings were suppressed, and no risks were
  accepted.

## Integrity and secret checks

| Check | Result |
| --- | --- |
| `go.mod` object hash | `afb79b59596d68b48225874a5bcab34e42bd7b29` (matches `HEAD:go.mod`) |
| `go.sum` object hash | `5384ee9297f6e733958907593df92e12c5868dd2` (matches `HEAD:go.sum`) |
| Tracked worktree diff after scan | empty |
| Untracked after scan | `.agent-sdlc/` scan artifacts only |
| Secret-pattern check of saved scan artifacts | 6 files checked, 0 hits |

No source, manifest, lockfile, or database was modified by the scan. Project
security configuration remains unchanged and report-only.

## Remaining decisions

- Refresh the advisory snapshots and rerun the baseline after dependency changes;
  the recorded OSV/RustSec identities are point-in-time.
- If a maintained Rust component is added, run OSV and cargo-audit against it and
  record the declared overlap as executed coverage.
- Required mode remains disabled; enabling it is a separate policy decision and
  would need required categories plus a coverage review.
- Automated periodic baselines are not part of this evidence; the raw artifacts
  are local and the report is the durable record.
