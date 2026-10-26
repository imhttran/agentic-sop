# Security Scanner Residual Findings — Evidence

**Type:** Historical evidence (non-normative)
**Date:** 2026-10-19
**Plan:** [PLAN-Security-Scanner-Residual-Findings.md](../history/plans/PLAN-Security-Scanner-Residual-Findings.md)
**Branch:** `fix/security-residual-findings`
**Implementation commit:** `458de839fbab02bce2d5c7ab51946f5c598a173a`
**PR:** [#74](https://github.com/imhttran/agentic-sop/pull/74) merged at `cca30c9`. The correction on this branch is **not** included and requires a follow-up PR.
**Baseline:** `main` @ `d4d8b8df` (includes PR #72 @ `a56b7239` and PR #73 @ `d4d8b8df`)

## Reported findings and reproduction

Two findings reproduced on the baseline before the fix. The third was invalidated
by reproduction against pinned Cargo.

| Finding                                                                                                                                                 | Reproduction evidence                                                                                                       |
| ------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Absolute or repository-escaping `package.workspace` references were silently skipped after normalization failed, so the package could look independent. | `workspace = "/tmp/outside"` produced no detection issue.                                                                   |
| **Invalidated:** a `members` glob matching an ordinary file was believed to hide a valid package match.                                                 | Pinned Cargo 1.98.0 accepts `members = ["crates/*"]` with `crates/app` and `crates/README.txt` (exit 0); files are ignored. |
| cargo-audit freshness tests compared fixed fixture timestamps with `time.Now()`.                                                                        | The advisory-db fixture was fresh at 2026-10-18 and stale at +120 days, so results depended on the execution date.          |

Pinned Cargo 1.98.0 behavior was verified in isolated fixtures before changing
discovery semantics:

```text
crates/* with crates/app + crates/README.txt          -> exit 0 (Cargo ignores files)
crates/* with crates/app + crates/empty (no manifest) -> exit 101 (fatal)
crates/** with crates/app/src (no manifest)            -> exit 101 (fatal)
valid package.workspace = "../.." in a workspace member -> resolves
```

The file-glob finding was therefore **invalidated by reproduction**: Cargo
tolerates ordinary files matched by a members glob and fails only on a matched
directory without `Cargo.toml`. Discovery now follows that behavior.

## Corrected behavior

- A `package.workspace` reference that is absolute or escapes the repository is
  recorded as an explicit unsupported-layout issue. It is not reinterpreted as an
  independent package, and dependency adapters fail closed instead of reporting
  clean coverage.
- Valid in-repository references are preserved, including a package that
  references a workspace root outside its own directory subtree.
- Cargo-compatible member-glob handling: ordinary files matched by a members
  glob are ignored, while a matched directory without `Cargo.toml` remains an
  explicit unsupported-layout issue. `exclude` still removes glob-expanded
  members.
- Discovery remains pure Go. A PATH-shim regression test proves Cargo is not
  executed during detection.
- cargo-audit freshness decisions use an injectable clock. Production defaults to
  `time.Now`; tests pin a fixed clock. The existing clean and seeded fixture tests
  use that controlled clock and assert the fixture is not stale, so they cannot
  drift with the real calendar. Freshness is covered for fresh, stale, unknown
  metadata, the exact 90-day boundary, and resume invalidation when a previously
  fresh database ages without changing its content identity.
- Detection issues, adapter evidence, inspection readiness, and required-mode
  projections agree: invalid layouts fail the adapter and required mode does not
  pass.

## Verification

| Check                                         | Result                                                                                                 |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `go test ./...`                               | PASS                                                                                                   |
| `go build ./...`                              | PASS                                                                                                   |
| `go vet ./...`                                | PASS                                                                                                   |
| `bash scripts/checks/check-doc-links.sh`      | `broken links: 0`                                                                                      |
| `bash scripts/checks/security-integration.sh` | PASS (pinned Semgrep 1.136.0, Gitleaks 8.30.1, cargo-audit 0.22.2, OSV-Scanner 2.7.0)                  |
| PR #74 CI                                     | PASS — 12/12 checks on the merged head `cca30c9`                                                       |
| Correction CI                                 | NOT RUN — PR #74 was already merged and closed; no checks are created for the correction on the branch |

The correction was verified locally after the edits: focused security tests, the
full `go test ./...` suite, build, vet, doc links, and the real-scanner
integration gate all passed. CI was not run for the correction because PR #74 was
merged and closed before the correction commit could be attached. A follow-up PR
is required to run CI on the corrected head.

## Delivery status

PR #74 was merged at `cca30c9` (merge commit `89a93111`) before the correction
was pushed. The correction on `fix/security-residual-findings` is not in `main`
and has no pull request. The smallest resume action is a follow-up PR from this
branch (rebased onto current `main`) for the correction only. No merge,
auto-merge, or branch deletion was performed by this work.

## Correction after PR review

The original PR treated a members glob matching an ordinary file as an ambiguous
layout. Reproduction against pinned Cargo 1.98.0 showed that Cargo accepts such a
repository: files are ignored, and only a matched directory without `Cargo.toml`
is fatal. The correction removed the file diagnostic and the file-collection
plumbing, kept the directory diagnostic, and updated the tests and documentation.
The original finding is recorded as invalidated rather than silently dropped. The
unsafe `package.workspace` fix and the production freshness policy are unchanged.

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
