# Proposal: execution-time integrity for local scanner binaries

Status: **Decided — option B, implemented** (2026-11-09) as optional
`binary_sha256` and `rules_sha256` per adapter (`internal/security/pinned.go`). The
repository enables the rules pin; binary pins are platform-specific and opt-in.

Related: [Plan 04 report](../reports/owasp-llm/llm04-supply-chain.md) (GAP-01 and
the residual "scanner binary runtime hash is not re-checked"),
[SECURITY-SCANNER-ADDON.md](../specs/SECURITY-SCANNER-ADDON.md), and
[CONFIGURATION.md](../reference/CONFIGURATION.md).

## What is enforced today

| Control | Where | What it proves |
| ------- | ----- | -------------- |
| Download pin | `scripts/security/fetch-pinned.sh`; `scripts/checks/test-fetch-pinned.sh` | A fresh download **and** a cache hit are checked against the committed SHA-256; a tampered cache entry fails closed |
| Archive/binary pin at provisioning | `fetch-osv-scanner.sh`, `fetch-cargo-audit.sh`, the Gitleaks inline pin in `scripts/checks/security-integration.sh`, the Semgrep `pip --require-hashes` lock | The bytes extracted during provisioning match a reviewed pin |
| CI cache scope | `.github/workflows/ci.yml` | Only the **download cache** persists; `SOP_SECURITY_TOOLS_DIR` lives under `runner.temp` and is provisioned per job |
| Execution version check | `internal/security/osv.go`, `cargoaudit.go` | The running dependency scanner reports the exact pinned version |
| Descriptive pins | `internal/security/semgrep.go`, `gitleaks.go`; `CONFIGURATION.md` | The SAST/secrets version is recorded; it is not enforced, by contract |

No enforced MUST was found to be bypassed. The remaining gap is narrower: once a
binary exists in a writable tools directory, the scan re-executes whatever bytes
are there and checks only the version.

## Threat and why a checksum alone is not enough

If an attacker can replace `$tools/osv-scanner-2.7.0/osv-scanner` with a
same-version binary, every current check passes: the version probe returns
`2.7.0`, the archive pin is not consulted on the cache-hit path, and the scan
runs. The attacker already controls the tool directory, so a checksum recorded
in that same directory proves nothing.

A useful pin must come from **trusted provenance** — the committed manifest that
is already checked against the upstream release at provisioning time — and must be
checked before execution. A hash generated from the local binary being checked is
not evidence.

## Options

- **A — keep the current contract.** The tools directory is operator-owned; SOP
  verifies acquisition and version, and a writable tool directory is outside the
  SOP trust boundary. Cheapest, but leaves the substitution undetectable.
- **B (recommended) — optional `binary_sha256`, sourced from a verified
  acquisition.** Add `security.adapters[].binary_sha256`. When set, the adapter
  hashes the resolved executable immediately before execution and fails closed on
  a mismatch. Values must be committed in the reviewed manifest or configuration
  and derived from a freshly verified upstream release, never from the local
  installation. Start with the single-file binaries whose releases are already
  pinned (OSV-Scanner, cargo-audit, Gitleaks); Semgrep's venv needs a tree or
  package digest and can follow.
- **C — platform/publisher trust.** Signatures, notarization, or a package
  manager's own verification. Outside SOP's ownership; no new infrastructure here.

## Acceptance criteria for option B

1. The schema accepts an optional `binary_sha256` per adapter; omission preserves
   today's version-only behavior.
2. A configured hash is verified after path resolution and before the scan's
   first argument is executed; a missing executable, malformed pin, or mismatch is
   a non-clean execution failure that never starts the scan.
3. Pins are derived from a verified acquisition (the committed manifest checked
   against the upstream release), and the report states that provenance. A pin
   copied from a local binary is explicitly rejected as evidence.
4. Deterministic fixtures: matching hash runs; one-byte replacement fails closed;
   missing file, malformed hash, and absent pin retain their documented behavior.
5. Resume identity includes the verified binary identity; changing it re-executes.
6. Provenance records the observed binary identity without printing the path's
   contents or any secret.
7. No new service, gate, or permission framework; the existing adapter and
   manifest abstractions are reused.

## Decision required

Choose A, B, or C. B is recommended because it closes the substitution gap with
the abstraction that already exists, stays opt-in, and does not pretend that a
local checksum is authenticity.
