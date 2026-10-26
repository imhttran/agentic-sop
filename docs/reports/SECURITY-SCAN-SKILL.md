# Security scan skill and Go/Rust adapter capability

**Date:** 2026-10-17
**Plan:** [PLAN-Security-Scan-Skill.md](../history/plans/PLAN-Security-Scan-Skill.md)
**Scope:** reusable `sop-security-scan` skill, content detection, auto selection,
Semgrep language coverage, and a `cargo-audit` dependency adapter.
**Tracking:** issue #40 contains overlapping dependency-scanning follow-up. This
work references it and does not close it.

## What changed

- Added `skills/sop-security-scan/SKILL.md` and registered it in the canonical
  skill surface, installer list, Zed/Claude guides, and generated plugin mirror.
- Added `sop security inspect --json`: a read-only detection and capability
  report for languages, manifests, Cargo workspaces, actual/missing lockfiles,
  configured adapters, and proposed adapters.
- Added `sop security scan --auto`: detection-driven addition of missing built-in
  categories. Explicit adapter configuration remains authoritative.
- Added the `dependencies` category and a built-in `cargo-audit` adapter using the
  existing executor, policy, redaction, provenance, artifact, and resume paths.
- Added Semgrep language coverage derived from local rule `languages` metadata.
  A Rust-only repository cannot be reported as analyzed by Go-only rules.
- Added the Rust rule set to the pinned Semgrep rules manifest at the existing
  upstream commit. Rule content is still fetched explicitly and never committed.
- Extended evidence with `detection`, `required_mode` readiness, advisory database
  identity/metadata/stale state, and policy identity for the database path and
  stale policy.

## Supported capabilities

| Capability | Status |
| --- | --- |
| Secrets via Gitleaks | Supported; unchanged contract. |
| Go source analysis via Semgrep | Supported with local Go rules. |
| Rust source analysis via Semgrep | Supported with local Rust rules; unsupported coverage is explicit. |
| Rust dependency advisories via cargo-audit | Supported with an explicit local advisory database and lockfile. |
| Go dependency advisories | Not implemented; reported as unsupported. |
| Mixed Go/Rust content | Supported; per-language coverage is independent. |

## Validation

- `gofmt`, `go vet ./...`, `go build ./...`: pass.
- `go test ./internal/security/... ./internal/config/... ./internal/cli/... ./internal/skill/...`: pass.
- Full `go test ./...`: pass.
- `bash scripts/checks/check-doc-links.sh`: 0 broken links.
- `scripts/packaging/build-claude-plugin.sh --check`: plugin mirror current.
- Pinned real Semgrep CE 1.136.0 + Gitleaks 8.30.1 integration:
  `scripts/checks/security-integration.sh` passed clean/seeded fixtures,
  exclusions, resume identity, and real CLI conformance.
- Real cargo-audit integration: **NOT RUN**. The environment has no pinned
  cargo-audit advisory database; the test is gated on
  `SOP_SECURITY_CARGO_ADVISORY_DB` and reports a skip rather than a clean scan.
- Deterministic cargo-audit coverage uses pinned JSON/DB fixtures for clean,
  advisory, malformed, missing database identity, missing lockfile, timeout,
  unavailable tool, stale deny/allow, and resume invalidation.
- Dogfooding on `rag-template-rust`: **NOT RUN**. The repository was not present
  or authorized in this environment, so no completed Rust scan is claimed.

## SOP execution note

`sop plan` produced the staged plan and `sop run` compiled eight tasks. S1 and S2
completed locally; S3 and S4 reported completion without executing their intended
file writes, and S5 stopped with `CONTINUE` after a bounded discovery failure. The
remaining implementation was completed manually on the same plan and branch, then
verified with the commands above. This is recorded as a SOP execution limitation,
not as evidence that the missing stages ran.

## Remaining gaps

- Real cargo-audit execution needs a pinned advisory database supplied by the
  operator or CI provisioning.
- Go module dependency advisories are not covered by a dependency adapter.
- Hosted analysis, automatic remediation, cross-provider deduplication, and
  runtime/network scanning remain out of scope.
- A passing scan is evidence for the configured tools, rules/database, and scope;
  it is not proof that a repository has no vulnerabilities.
