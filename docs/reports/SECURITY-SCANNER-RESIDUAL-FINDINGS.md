# Security Scanner Residual Findings — Evidence

**Type:** Historical evidence (non-normative)
**Date:** 2026-10-19
**Plan:** [PLAN-Security-Scanner-Residual-Findings.md](../plans/PLAN-Security-Scanner-Residual-Findings.md)
**Branch:** `fix/security-residual-findings`
**Implementation commit:** `458de839fbab02bce2d5c7ab51946f5c598a173a`
**PR:** [#74](https://github.com/imhttran/agentic-sop/pull/74) (open for review)
**Baseline:** `main` @ `d4d8b8df` (includes PR #72 @ `a56b7239` and PR #73 @ `d4d8b8df`)

## Reported findings and reproduction

All three findings reproduced on the baseline before the fix:

| Finding                                                                                                                                                 | Reproduction evidence                                                                                              |
| ------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Absolute or repository-escaping `package.workspace` references were silently skipped after normalization failed, so the package could look independent. | `workspace = "/tmp/outside"` produced no detection issue.                                                          |
| A `members` glob matching a non-package file was invisible when another match was a valid package.                                                      | `members = ["crates/*"]` with `crates/app` and `crates/README.txt` produced no issue mentioning the file.          |
| cargo-audit freshness tests compared fixed fixture timestamps with `time.Now()`.                                                                        | The advisory-db fixture was fresh at 2026-10-18 and stale at +120 days, so results depended on the execution date. |

Pinned Cargo 1.98.0 behavior was verified in isolated fixtures before changing
discovery semantics:

```text
crates/* with crates/app + crates/README.txt          -> exit 0 (Cargo ignores files)
crates/* with crates/app + crates/empty (no manifest) -> exit 101 (fatal)
crates/** with crates/app/src (no manifest)            -> exit 101 (fatal)
valid package.workspace = "../.." in a workspace member -> resolves
```

## Corrected behavior

- A `package.workspace` reference that is absolute or escapes the repository is
  recorded as an explicit unsupported-layout issue. It is not reinterpreted as an
  independent package, and dependency adapters fail closed instead of reporting
  clean coverage.
- Valid in-repository references are preserved, including a package that
  references a workspace root outside its own directory subtree.
- Discovery retains in-scope file paths. A member glob that matches a file where
  a package directory is expected is reported even when another match is a valid
  package. Content below an already-matched directory is not reported separately,
  and `exclude` still removes glob-expanded members.
- Discovery remains pure Go. A PATH-shim regression test proves Cargo is not
  executed during detection.
- cargo-audit freshness decisions use an injectable clock. Production defaults to
  `time.Now`; tests pin a fixed clock. Freshness is covered for fresh, stale,
  unknown metadata, the exact 90-day boundary, and resume invalidation when a
  previously fresh database ages without changing its content identity.
- Detection issues, adapter evidence, inspection readiness, and required-mode
  projections agree: invalid layouts fail the adapter and required mode does not
  pass.

## Verification

| Check                                         | Result                                                                                |
| --------------------------------------------- | ------------------------------------------------------------------------------------- |
| `go test ./...`                               | PASS                                                                                  |
| `go build ./...`                              | PASS                                                                                  |
| `go vet ./...`                                | PASS                                                                                  |
| `bash scripts/checks/check-doc-links.sh`      | `broken links: 0`                                                                     |
| `bash scripts/checks/security-integration.sh` | PASS (pinned Semgrep 1.136.0, Gitleaks 8.30.1, cargo-audit 0.22.2, OSV-Scanner 2.7.0) |
| PR #74 CI                                     | PASS — 12/12 checks on `458de83`                                                      |

Focused regression coverage includes absolute and escaping workspace references,
valid nested-root references, member-glob file matches alongside valid members,
excluded glob matches, no-Cargo discovery, readiness and required-mode rejection,
fresh/stale/unknown freshness, the 90-day boundary, and resume invalidation after
database aging.

## Remaining limitations

- Automatic OSV selection for detected Go content creates a candidate without an
  exact version pin unless a configured OSV adapter supplies one. Readiness
  already reports `osv-scanner requires an exact version pin`, so the prerequisite
  is visible; inspection and documentation continue to require explicit
  configuration rather than treating the adapter as ready.
- The integration gate uses controlled offline advisory databases; it is
  behavior/coverage evidence, not a production vulnerability baseline.
- Dependency reachability remains not evaluated; a version match is not proof that
  vulnerable code is reachable.
- Other ecosystems, container scanning, runtime API scanning, and automatic
  remediation remain out of scope.
