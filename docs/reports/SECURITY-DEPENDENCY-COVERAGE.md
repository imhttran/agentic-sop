# Security Dependency Coverage — Workspace and Obligation Evidence

**Type:** Historical evidence (non-normative)
**Date:** 2026-10-18
**Plan:** [PLAN-Dependency-Coverage-Fixes.md](../plans/PLAN-Dependency-Coverage-Fixes.md)
**Branch:** `fix/dependency-coverage-obligations`
**Implementation commit:** `553ffb1`
**PR:** #72 (open for review)
**Baseline:** `main` @ `fca3e31` (PR #69 merged at `805dd5e`)

## Reported findings and reproduction

All three findings reproduced on the baseline before the fix:

| Finding | Reproduction evidence |
| --- | --- |
| Handwritten Cargo TOML parser missed single-quoted `members`, treated a workspace without `members` as absent, and assigned nested packages by directory ancestry. | `workspaces = [], want the single-quoted workspace detected` |
| OSV and cargo-audit resolved only present lockfiles. With two independent Rust packages and one `Cargo.lock`, they scanned one package and reported complete Rust coverage. | `OSV resolved only 1 input and reported no missing obligation` |
| OSV returned `COMPLETED`/`COMPLETE` for detected Go source with no `go.mod`. | `Go source without go.mod reported COMPLETED/COMPLETE` |

## Corrected behavior

- Cargo manifests are parsed with `github.com/pelletier/go-toml/v2`. Single- and
  double-quoted TOML produce equivalent results.
- `[workspace]` declarations are detected without requiring `members`. Ordinary,
  virtual, and implicit-member workspaces resolve their actual lockfiles.
- Members, member globs, exclusions, the workspace root package, explicit
  `package.workspace` references, and in-root path dependencies are resolved
  from content. Directory ancestry alone does not prove membership.
- Excluded and undeclared nested packages retain independent `Cargo.lock`
  obligations.
- Invalid TOML, unsafe/unmatched member patterns, nested workspaces, and dangling
  workspace references are explicit issues and cannot produce clean coverage.
- Detection builds a deterministic obligation inventory for Go `go.mod` and Rust
  `Cargo.lock` inputs. Every adapter scans present obligations and reports missing
  or explicitly omitted inputs by path.
- Explicit OSV `inputs` and cargo-audit `lockfile` settings are validated against
  the inventory. A subset reports the remaining obligations unless
  `security.exclude` removes them from scope.
- Findings from present inputs remain in the report when another obligation is
  missing; the execution is `FAILED` with `PARTIAL`/`UNKNOWN` coverage, never
  `COMPLETED`/`COMPLETE`.
- The obligation identity is recorded in provenance and used by resume. Adding,
  removing, or changing an obligation invalidates affected reused evidence.
- Go source without `go.mod` is an explicit unsupported input. A genuine
  no-content repository records `no_applicable_content: true` without claiming
  that a scanner ran. Selection metadata no longer fills languages for failed or
  no-op executions, and a no-op does not satisfy a required dependency category.

## Verification

| Check | Result |
| --- | --- |
| `go test ./...` | PASS |
| `make test-fast` (RAM-backed) | PASS |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `bash scripts/checks/check-doc-links.sh` | `broken links: 0` |
| `bash scripts/checks/security-integration.sh` | PASS |
| Real OSV two-package/one-lockfile test | PASS — `FAILED`/`PARTIAL`, missing `packages/b/Cargo.lock` reported, present finding preserved, inputs unchanged, no lockfile generated |
| Real cargo-audit two-package/one-lockfile test | PASS — `FAILED`/`PARTIAL`, missing path reported, present lockfile unchanged, no lockfile generated |

Focused regression coverage includes single/double-quoted TOML, virtual/ordinary/
implicit-member workspaces, member globs, exclusions, undeclared nested packages,
unsafe layouts, deterministic obligation identity, OSV/cargo-audit complete and
partial inventories, explicit subsets, excluded packages, Go source without
`go.mod`, mixed Go/Rust hiding unsupported Go input, no-applicable content,
required-mode blocking, and resume invalidation when an obligation changes.

## Dogfood

Report-only scan on the final implementation:

```text
Run ID:    dependency-coverage-dogfood-final
Verdict:   INDETERMINATE
Coverage:  PARTIAL
Adapters:  osv-scanner FAILED/PARTIAL (18 obligations)
           cargo-audit COMPLETED/PARTIAL (7 obligations)
```

The non-clean result is accurate: the controlled fixture set includes an OSV input
that produced empty output, and the controlled cargo-audit database is treated as
stale under the explicit `allow` policy. No missing input was silently omitted.
Local `.run` scratch content was excluded from the dogfood scope.

## Remaining limitations

- The dogfood intentionally uses controlled local databases, not production OSV
  data; it is coverage/behavior evidence, not a vulnerability baseline.
- Some repository fixture lockfiles are not accepted by the pinned OSV CLI and are
  correctly reported as failures rather than clean coverage.
- Dependency reachability remains not evaluated; a version match is not proof that
  vulnerable code is reachable.
- Other ecosystems, container scanning, runtime API scanning, and automatic
  remediation remain out of scope.
