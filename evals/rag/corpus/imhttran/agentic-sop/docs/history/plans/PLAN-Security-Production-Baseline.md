# PLAN — Production Dependency Vulnerability Baseline

**Type:** Evidence plan (point-in-time production dependency baseline).
**Status:** IMPLEMENTED — baseline executed on `1dbaa3a3`; documentation delivery in progress.
**Baseline:** `agentic-sop` `origin/main` @ `1dbaa3a307fe970c0d13a928141b456d78aa6105`
(includes PR #79 merge `225b7679` and correction head `920bf37`).
**Branch:** `docs/production-dependency-baseline`.

## Project

agentic-sop

## Summary

Establish a reproducible, point-in-time dependency vulnerability baseline for
the maintained production packages in `agentic-sop`, using the security add-on
already merged through PR #79. The baseline is report-only, scans from a clean
worktree, uses pinned scanners and production advisory data, and records every
exclusion and coverage limitation. It does not change dependencies, suppress
findings, enable required mode, or overwrite earlier fixture evidence.

## PB-001 — Verify the merged revision and build identity

Confirm PR #79 and correction head `920bf37` are included in `origin/main`,
distinguish PR-head CI from merge-commit CI, and build the scan CLI from the
exact tested revision.

### Deliverables

- Merge ancestry evidence for `920bf37` and `225b7679`.
- PR-head and merge-commit CI results recorded separately.
- Scan CLI built from a clean worktree at the tested revision, with binary
  digest, Go version, platform, `vcs.revision`, and `vcs.modified` recorded.

### Acceptance Criteria

- The tested revision is exact and is included in `origin/main`.
- The CLI identity matches the tested revision and a clean worktree.
- PR-head CI is not presented as merge-commit CI.

### Acceptance Checks

- `git status --short --branch`
- `go build ./...`
- `bash scripts/checks/check-doc-links.sh`

### Dependencies

- none

## PB-002 — Provision pinned scanners and production advisory data

Provision OSV-Scanner and cargo-audit through the repository's supported
scripts, fetch production OSV data for Go and crates.io, and fetch the RustSec
advisory database as a separate preparation step. Record source, snapshot or
commit identity, digest, size, timestamps, and freshness.

### Deliverables

- Pinned OSV-Scanner 2.7.0 and cargo-audit 0.22.2 with manifest digests.
- OSV `Go` and `crates.io` archives with source URL, SHA-256, size, and
  acquisition time.
- RustSec advisory database at commit `999ec89`, with archive digest, advisory
  count, content digest, upstream timestamp, and acquisition metadata.

### Acceptance Criteria

- Every scanner and database has a recorded identity.
- Scans execute offline against the prepared data.
- Missing or stale data is not treated as clean.

### Acceptance Checks

- `bash scripts/checks/security-integration.sh`
- `go test ./internal/security`
- `go vet ./internal/security`

### Dependencies

- PB-001

## PB-003 — Define the production scope

Inspect the dependency-obligation inventory and separate maintained application
and tool packages from synthetic fixtures, benchmark workloads, and tracked
reference projects. Document every exclusion and preserve missing or unsupported
production inputs as explicit coverage gaps.

### Deliverables

- In-scope obligation inventory for the tested revision.
- Exclusion list with a reason for each path.
- Explicit statement of production Rust coverage (none at this revision).

### Acceptance Criteria

- Synthetic fixtures never contribute production findings.
- No lockfile is generated and no production input is excluded to obtain PASS.
- Missing or unsupported production inputs remain visible.

### Acceptance Checks

- `go test ./internal/security`
- `go vet ./internal/security`
- `go build ./...`

### Dependencies

- PB-002

## PB-004 — Run the report-only baseline

Run OSV-Scanner for the Go input and cargo-audit for applicable Rust inputs with
an explicit temporary report-only configuration. Record content, obligation,
scanner, database, configuration, and policy identities, plus coverage and
timings.

### Deliverables

- Report-only scan from a clean worktree at the tested revision.
- Adapter statuses, coverage, findings, provenance, and timings.
- Declared overlap recorded separately from executed overlap.

### Acceptance Criteria

- The project configuration is unchanged and remains report-only.
- Every in-scope obligation is scanned or explicitly reported as incomplete.
- Failed or incomplete analysis never appears clean.

### Acceptance Checks

- `go test ./internal/security`
- `go vet ./internal/security`
- `go build ./...`

### Dependencies

- PB-003

## PB-005 — Triage findings and verify integrity

Verify advisory identities and versions for any finding, separate findings from
withdrawn advisories, unsupported inputs, and execution failures, and check that
source, manifests, lockfiles, and artifacts are unchanged and contain no
detected secrets.

### Deliverables

- Finding triage (or an explicit zero-finding statement).
- Reachability recorded as not evaluated.
- Input object hashes before/after and a secret-pattern check of saved artifacts.
- Proposed remediation and verification for actionable findings.

### Acceptance Criteria

- Transitive findings are not dismissed.
- Reachability is not inferred from a version match.
- Inputs are unchanged and artifacts contain no detected secrets.

### Acceptance Checks

- `go test ./...`
- `go vet ./...`
- `bash scripts/checks/check-doc-links.sh`

### Dependencies

- PB-004

## PB-006 — Publish the baseline report and open the documentation PR

Add a point-in-time report without overwriting earlier fixture evidence, index
it, reference issue #40 and PR #79, and open a focused documentation PR.

### Deliverables

- `docs/reports/SECURITY-PRODUCTION-BASELINE-2026-10-19.md`.
- Updated report index.
- Focused documentation PR with the tested revision, data identities, findings,
  coverage, timings, and remaining decisions.

### Acceptance Criteria

- The report identifies the exact tested revision.
- The report distinguishes fixture evidence from production results.
- No merge, auto-merge, branch deletion, or required-mode change occurs.

### Acceptance Checks

- `bash scripts/checks/check-doc-links.sh`
- `go test ./...`
- `go build ./...`
- `go vet ./...`

### Dependencies

- PB-005
