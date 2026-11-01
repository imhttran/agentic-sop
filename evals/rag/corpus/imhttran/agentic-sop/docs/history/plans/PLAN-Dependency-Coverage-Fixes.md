# PLAN — Dependency Coverage Obligations

**Type:** Correctness plan (dependency-coverage evidence).
**Status:** IMPLEMENTED — verification complete; PR #72 open for review.
**Baseline:** `agentic-sop` `main` @ `fca3e31` (PR #69 merged at `805dd5e`).
**Branch:** `fix/dependency-coverage-obligations`.

## Project

agentic-sop

## Summary

The scanner review found three dependency-coverage defects in the configurable
security scanner work merged by PR #65 and its follow-up PR #69:

1. Cargo workspace detection uses a handwritten TOML line parser, ignores
   single-quoted strings, only recognizes a workspace when `members` is present,
   and assigns every nested Rust package to its nearest workspace by directory
   ancestry.
2. Dependency adapters resolve only the lockfiles that happen to be present.
   With two independent Rust packages and one `Cargo.lock`, OSV and cargo-audit
   scan the present input and report complete Rust coverage, silently omitting
   the other package.
3. OSV returns `COMPLETED`/`COMPLETE` when a detected language has no supported
   dependency input. A Go source tree without `go.mod` therefore looks clean, and
   runner selection metadata can turn the unperformed analysis into declared
   language coverage.

Reproduction on current `main`:

```text
TestReproSingleQuotedWorkspaceAndExcludedPackage
  workspaces = [], want the single-quoted workspace detected
TestReproMissingSecondLockfileObligation
  OSV resolved only 1 input and reported no missing obligation
TestReproGoSourceWithoutManifestIsNotClean
  Go source without go.mod reported COMPLETED/COMPLETE
```

This plan replaces the parser with a TOML library, builds an explicit dependency
obligation inventory, makes adapters account for every in-scope obligation, and
rejects empty analysis for detected languages. It reuses the existing adapter,
policy, evidence, resume, and CLI surfaces. It does not add a scanner framework,
generate lockfiles, fetch Cargo dependencies, or run automatic fixes.

## SEC-COV-001 — Correct Cargo workspace detection

Replace the handwritten TOML parser with `github.com/pelletier/go-toml/v2`, detect
workspace declarations independently of `members`, resolve members/exclusions/
glob/path-dependency membership, and stop using directory ancestry as proof of
workspace membership. Keep detection read-only and inside the repository.

### Deliverables

- Add `github.com/pelletier/go-toml/v2` to `go.mod`/`go.sum`.
- Replace `cargoWorkspaceMembers`/`parseTomlStringArray` with TOML decoding.
- Record a workspace when `[workspace]` exists, including virtual and
  implicit-member workspaces.
- Resolve `members` and `exclude` globs, the root package of a non-virtual
  workspace, explicit `package.workspace` references, and in-root path
  dependencies that Cargo treats as implicit members.
- Treat excluded packages and undeclared nested packages as independent lockfile
  obligations, not workspace members.
- Add explicit workspace/layout issues to detection evidence; an unsupported or
  ambiguous layout must not produce clean dependency coverage.
- Add detection tests for single/double-quoted TOML, virtual/ordinary/implicit
  workspaces, excluded independent packages, member globs, and no-Cargo execution.

### Acceptance Criteria

- Single-quoted and double-quoted TOML produce equivalent workspace results.
- Ordinary, virtual, and implicit-member workspaces resolve their actual lockfiles.
- Excluded independent packages retain separate lockfile obligations.
- Invalid or unsupported layouts cannot produce clean dependency coverage.
- Discovery executes no Cargo command, fetches no dependency, and writes no file.

### Acceptance Checks

- `go test ./internal/security/... -run 'TestCargoWorkspace|TestDetect'`
- `go test ./internal/security/...`
- `go vet ./internal/security/...`

### Dependencies

- none

## SEC-COV-002 — Enforce every dependency input obligation

Depends on SEC-COV-001.

Build a deterministic obligation inventory and make OSV and cargo-audit account
for every in-scope Go/Rust input. Preserve findings from present inputs, report
missing inputs by path, and never report complete language coverage when an
obligation is unaudited.

### Deliverables

- Add a `DependencyObligation` inventory to detection, with language, root,
  manifest, expected input path, present state, workspace/package kind, and
  issues.
- Make OSV and cargo-audit consume the inventory: scan every present obligation,
  report every missing obligation with its path, and mark coverage partial or
  unknown instead of complete.
- Validate explicit OSV `inputs` and cargo-audit `lockfile` against the inventory;
  a subset reports remaining obligations unless `security.exclude` removes them.
- Record the resolved obligation inventory in execution evidence and a
  deterministic `ObligationIdentity` in provenance/resume comparison.
- Add adapter tests for two independent Rust packages with one and two lockfiles,
  excluded packages, explicit subsets, and resume invalidation when an obligation
  changes.

### Acceptance Criteria

- With two independent Rust packages and only one `Cargo.lock`, neither adapter
  reports complete Rust coverage.
- Clean results for one package cannot satisfy an unaudited package.
- All required lockfiles present permits complete coverage when scans succeed.
- Intentionally excluded packages do not create in-scope obligations.
- Adding, removing, or changing an obligation invalidates affected reused evidence.
- Required mode blocks incomplete input coverage.

### Acceptance Checks

- `go test ./internal/security/... -run 'TestDependencyObligation|TestOSV|TestCargoAudit|TestResume'`
- `go test ./internal/security/...`
- `go vet ./internal/security/...`

### Dependencies

- SEC-COV-001

## SEC-COV-003 — Reject empty analysis for detected languages

Depends on SEC-COV-002.

Distinguish no applicable content from detected content without a supported
dependency input. A detected Go source tree without `go.mod` is not a clean
dependency result, and runner selection metadata must not prove language coverage
for a scan that never ran.

### Deliverables

- Represent a detected language with no supported dependency input as an explicit
  missing obligation/issue, not an empty input set.
- Make OSV and cargo-audit return a non-clean execution for that state and retain
  the explicit diagnostic.
- Add `NoApplicableContent` (or equivalent) evidence so a genuine no-content
  repository is reported without claiming a scanner ran.
- Separate declared selection scope from observed coverage for enabled adapters:
  only completed observations may count as proven language coverage.
- Add regression tests for Go source without `go.mod`, mixed Go/Rust hiding an
  unsupported Go dependency input, no applicable content, missing tool/database,
  and required-mode projection/evidence agreement.

### Acceptance Criteria

- A repository containing Go source but no `go.mod` cannot pass a required
  dependency gate.
- A mixed repository cannot hide unsupported Go content behind a successful Rust scan.
- No applicable content is reported explicitly without claiming a scanner ran.
- Missing tools or databases cannot be masked by an empty-input success.
- Required-mode projection and recorded evidence agree.

### Acceptance Checks

- `go test ./internal/security/... -run 'TestEmpty|TestNoApplicable|TestRequired'`
- `go test ./internal/cli/... -run 'TestSecurity'`
- `go vet ./internal/security/... ./internal/cli/...`

### Dependencies

- SEC-COV-002

## SEC-COV-004 — Verify, dogfood, and deliver

Depends on SEC-COV-003.

Add end-to-end regression coverage, run real scanners against controlled advisory
databases, dogfood report-only scanning, update documentation, review the diff,
and deliver a focused PR while monitoring required CI.

### Deliverables

- Focused detection, adapter, runner, policy, CLI, and resume regression tests.
- Real OSV and cargo-audit integration tests using controlled local advisory
  databases and fixtures that leave sources/manifests/lockfiles unchanged.
- One report-only dogfood scan on the final implementation.
- Updated specification, security-scan skill copies, setup guide, and report
  evidence where behavior changed.
- Final diff review and focused PR; CI monitored to completion.

### Acceptance Criteria

- Unit, CLI, and real-scanner integration gates pass at the final milestone.
- Report-only dogfood evidence records the actual result and coverage.
- Documentation states the corrected workspace, obligation, and empty-analysis
  behavior.
- Fixtures prove no source, manifest, lockfile, or database was modified.
- The PR is open for review; no merge, auto-merge, or branch deletion occurs.

### Acceptance Checks

- `go test ./...`
- `make test-fast`
- `bash scripts/checks/security-integration.sh`
- `bash scripts/checks/check-doc-links.sh`
- `go build ./...`
- `go vet ./...`

### Dependencies

- SEC-COV-003
