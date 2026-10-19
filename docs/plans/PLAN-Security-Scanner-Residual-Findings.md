# PLAN — Security Scanner Residual Findings

**Type:** Correctness plan (dependency-layout detection and freshness evidence).
**Status:** IN PROGRESS — implementation and verification underway.
**Baseline:** `agentic-sop` `main` @ `d4d8b8df` (includes PR #72 @ `a56b7239` and PR #73 @ `d4d8b8df`).
**Branch:** `fix/security-residual-findings`.

## Project

agentic-sop

## Summary

The review of the dependency-coverage work left three residual defects:

1. `package.workspace` references that are absolute or escape the repository are
   silently reinterpreted: normalization fails, the reference is skipped, and the
   package may look independent instead of unsupported.
2. A `members` glob that matches a non-package file (for example
   `crates/README.txt` under `members = ["crates/*"]`) is invisible whenever
   another match is a valid package directory, so discovery reports coverage it
   cannot prove.
3. cargo-audit freshness tests use fixed `last-updated` fixture timestamps
   compared against `time.Now()`, so their meaning changes with the calendar
   date.

Reproduction on `d4d8b8df` (temporary repro tests, before the fix):

```text
TestReproUnresolvedWorkspaceReference
  workspace = "/tmp/outside"        -> detection.Issues = [] (no issue)
  workspace = "../../../outside"    -> dangling-reference issue only, no
                                       unsupported-layout classification
TestReproMemberGlobMatchesFile
  members = ["crates/*"], crates/app + crates/README.txt
                                    -> no issue mentioning README.txt
TestReproFreshnessCalendarDependence
  advisory-db last-updated 2026-10-01 is fresh at 2026-10-18 and stale at +120d
                                    -> freshness is decided by the wall clock
```

Pinned Cargo 1.98.0 behavior verified in isolated fixtures before changing
semantics:

```text
crates/* with crates/app + crates/README.txt        -> exit 0 (files ignored)
crates/* with crates/app + crates/empty (no manifest) -> exit 101 (fatal)
crates/** with crates/app/src (no manifest)          -> exit 101 (fatal)
valid package.workspace = "../.." in a member        -> resolves
```

This plan reuses the existing detection, adapter, policy, evidence, resume, and
CLI surfaces. It adds no scanner framework, does not execute Cargo during
discovery, and does not weaken required-mode gates.

## SEC-RES-001 — Reject unresolved package.workspace references

Reproduce absolute and repository-escaping `package.workspace` references. When
normalization cannot produce an in-repository path, record an explicit
unsupported-layout issue instead of silently treating the package as independent.
Preserve valid in-repository references, including a reference to a workspace
root that is not an ancestor of the package.

### Deliverables

- `internal/security/cargo.go`: emit an unsupported-layout issue for absolute or
  repository-escaping `package.workspace` references.
- Keep valid in-repository references working, including nested workspace roots
  referenced from outside their directory subtree.
- Regression tests covering detection issues, readiness projection, OSV,
  cargo-audit, and required-mode rejection.

### Acceptance Criteria

- Invalid workspace references cannot produce complete dependency coverage.
- Detection, adapter execution, inspection readiness, and required gates agree.
- Valid in-repository references remain issue-free.
- Discovery remains read-only and confined to the repository.

### Acceptance Checks

- `go test ./internal/security`
- `go vet ./internal/security`

### Dependencies

- none

## SEC-RES-002 — Diagnose invalid member-glob matches

Reproduce `members = ["crates/*"]` with a valid `crates/app` package and a
matching `crates/README.txt` file. Verify pinned Cargo behavior in isolated
fixtures, then make discovery account for matching files as well as matching
directories. A valid match must not hide an invalid one. Exclusions, explicit
members, repository boundaries, and read-only discovery are preserved.

### Deliverables

- `internal/security/detect.go`: retain in-scope file paths for Cargo analysis.
- `internal/security/cargo.go`: report member-glob matches that are files, while
  ignoring content below an already-matched directory and honoring `exclude`.
- Regression tests for a file matching alongside a valid member, excluded
  matches, and no Cargo execution during discovery.

### Acceptance Criteria

- Invalid member-glob matches remain visible even when valid members also match.
- Nested content below a matched directory does not produce noise.
- Excluded glob matches are not treated as in-scope members.
- Discovery does not fetch dependencies, generate lockfiles, or modify source.

### Acceptance Checks

- `go test ./internal/security`
- `go vet ./internal/security`

### Dependencies

- SEC-RES-001

## SEC-RES-003 — Remove calendar dependence from freshness tests

Replace wall-clock freshness decisions in cargo-audit tests with an injected
clock. Cover fresh, stale, unknown freshness, the exact 90-day boundary, and
resume invalidation when a previously fresh database ages without changing its
content identity. Production freshness policy is unchanged.

### Deliverables

- `internal/security/cargoaudit.go`: injectable freshness clock used by
  `currentProvenance` and `Scan`; production defaults to `time.Now`.
- `internal/security/cargoaudit_test.go`: fixed test clock for adapter tests.
- Regression tests for the boundary, unknown metadata, fixture aging, and resume
  invalidation.

### Acceptance Criteria

- Freshness tests produce the same result regardless of execution date.
- Fresh, stale, unknown, and the exact 90-day boundary are explicit.
- Aged evidence invalidates reuse even when content identity is unchanged.
- Tracked fixtures are not edited by tests and production policy is not weakened.

### Acceptance Checks

- `go test ./internal/security`
- `go vet ./internal/security`

### Dependencies

- SEC-RES-002

## SEC-RES-004 — Verify, document, and deliver

Run focused and full gates, real-scanner integration, documentation updates,
final diff review, and delivery.

### Deliverables

- Focused regression tests plus the repository's documented full gate and real
  OSV/cargo-audit integration gate.
- Documentation notes where corrected behavior is observable.
- Review of the complete diff for scope and compatibility.
- Focused PR referencing PRs #72 and #73, CI monitored on the final head.

### Acceptance Criteria

- Required checks and real-scanner integration pass at the final milestone.
- Project configuration remains report-only; required mode is tested only in
  isolated fixtures.
- No merge, auto-merge, or branch/worktree deletion occurs.

### Acceptance Checks

- `go test ./...`
- `make test-fast`
- `bash scripts/checks/security-integration.sh`
- `bash scripts/checks/check-doc-links.sh`
- `go build ./...`
- `go vet ./...`

### Dependencies

- SEC-RES-003
