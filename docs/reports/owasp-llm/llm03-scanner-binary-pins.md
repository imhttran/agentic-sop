# LLM03 scanner binary pins: committed per-platform digests and baseline (OWASP gap 9, item 1)

**Decision:** the operator approved item 1 of
[PROPOSAL-SCANNER-PIN-POLICY.md](../../plans/PROPOSAL-SCANNER-PIN-POLICY.md) in chat on
2026-11-12: commit per-platform binary pins for the scanners CI already pins, on linux
and darwin, and run the baseline. Items 2 (publisher signatures) and 3 (required
categories) are not decided here.

**Scope:** Gitleaks and OSV-Scanner, which are the configured adapters that ship a
single executable. Semgrep is not pinned: its install is a venv, which the proposal
says needs a tree digest and can follow later. Windows is not pinned.

## Derived digests

Each digest is the SHA-256 of the executable that the adapter resolves. For archive
releases, the archive was downloaded with `scripts/security/fetch-pinned.sh` against
the archive pin already committed in `scripts/checks/security-integration.sh` (Gitleaks)
or `scripts/security/cargo-audit-manifest.json` (cargo-audit), then extracted. The
digest is of the extracted binary. OSV-Scanner's release asset is the executable itself,
so its binary digest is the committed manifest value.

| Scanner | Platform | Source of the digest | Binary SHA-256 | Committed |
| --- | --- | --- | --- | --- |
| Gitleaks 8.30.1 | linux/amd64 | extracted `gitleaks_8.30.1_linux_x64.tar.gz` | `88f91962aa2f93ac6ab281d553b9e125f5197bbbce38f9f2437f7299c32e5509` | yes |
| Gitleaks 8.30.1 | linux/arm64 | extracted `gitleaks_8.30.1_linux_arm64.tar.gz` | `00e91bbe655bd7c47753e8cfe61cb76ea1a5d7e7702fe161ee40102b46b3823b` | yes |
| Gitleaks 8.30.1 | darwin/amd64 | extracted `gitleaks_8.30.1_darwin_x64.tar.gz` | `cee01fea7173f1b779dff188e1c26ecbcb4027d394acc573b23aaf0be260e291` | yes |
| Gitleaks 8.30.1 | darwin/arm64 | extracted `gitleaks_8.30.1_darwin_arm64.tar.gz` | `ba52fb1bfabbcde42f032afad3d6e0b19dff8ed105229a16e7caa338bbc0e84f` | yes |
| OSV-Scanner 2.7.0 | linux/amd64 | `osv-scanner-manifest.json` asset pin | `160c6253e2922e9e0a6a9771c5c4c40a4b3c6090b34a3cdd5e80c96cc2e4218b` | yes |
| OSV-Scanner 2.7.0 | linux/arm64 | `osv-scanner-manifest.json` asset pin | `4061ef4c021ed3c47f203b818f337e351336cc5cd67c0274fc36bd1dfc728dd7` | yes |
| OSV-Scanner 2.7.0 | darwin/amd64 | `osv-scanner-manifest.json` asset pin | `e82c5df7ab0a42dff29a1cd0660711447b56b4c0483ff66ad8634df10fd81a45` | yes |
| OSV-Scanner 2.7.0 | darwin/arm64 | `osv-scanner-manifest.json` asset pin | `d4926ef8da134332543be276dff157e4c170f99ad91fc48e89448a8816bdf70b` | yes |
| cargo-audit 0.22.2 | linux/amd64 | extracted `cargo-audit-x86_64-unknown-linux-gnu-v0.22.2.tgz` | `473b9a71e5cb5bde22f69c32f749c9b83931287d92dc36b91cb04f6705640ef2` | no |
| cargo-audit 0.22.2 | darwin/arm64 | extracted `cargo-audit-aarch64-apple-darwin-v0.22.2.tgz` | `33fbe81adca1b794f4ffe98574d59b0ebe6fcfdb310976fafed98a094c795111` | no |

cargo-audit digests are derived but not committed: the tracked configuration has no
cargo-audit adapter, so a pin would be inert. They are recorded for when one is added.

## Configuration shape

`binary_sha256` holds one value per adapter, which cannot pin several platforms. A new
key, `binary_sha256_by_platform`, maps `<GOOS>/<GOARCH>` to a digest. Only the four
platforms above are accepted, so a typo fails the config load instead of silently
running unpinned. Setting both forms is rejected. `BuildAdapter` resolves the running
platform's pin before it computes the adapter identity and builds the executor. A
platform with no entry runs without a binary pin, as before.

## Baseline run (2026-11-12, darwin arm64)

The scans used the pinned Gitleaks 8.30.1 and OSV-Scanner 2.7.0 first on `PATH`, and
the Semgrep rules pinned by `rules_sha256` (`d90ff1b1…`, commit `17e31e11…`). Semgrep
1.183.0 came from Homebrew, because Semgrep is not pinned. The OSV offline database
was provisioned with `scripts/security/fetch-osv-databases.sh` (Go and crates.io).

| Adapter | Status | Coverage | Findings (unsuppressed) |
| --- | --- | --- | --- |
| gitleaks 8.30.1 (pinned) | COMPLETED | COMPLETE | 27 HIGH |
| semgrep 1.183.0 (not pinned) | COMPLETED | COMPLETE | 18 HIGH, 16 MEDIUM |
| osv-scanner 2.7.0 (pinned) | COMPLETED | COMPLETE | none reported |

- Total: 101 finding records; 40 are suppressed by the reviewed baseline
  (`.agent-sdlc/security-baseline.json`, 56 entries, expiry 2027-06-30).
- 61 are unsuppressed. Verdict: `FAIL` with 45 unsuppressed findings at a blocking
  severity. The repository runs in report-only mode, so the verdict does not block a
  change.
- Gitleaks unsuppressed hits are in 9 files. They are credential-shaped strings in
  tests and reports, for example `internal/redact/policy_test.go` and
  `docs/reports/owasp-llm/llm02-seven-format-campaign.json`. **They have not been
  triaged here.** Triage is the operator's input to item 3.
- Semgrep HIGH findings are mostly `dangerous-exec-command`; MEDIUM findings include
  `open-redirect` and `no-direct-write-to-responsewriter`. Not triaged here.

No finding text or matched value is recorded in this page. The raw scan output is
local, not committed.

## Negative control: an unpinned binary fails closed

With the pins committed, `/opt/homebrew/bin/gitleaks` (version 8.30.1, digest
`f414bc2f…`) was first on `PATH`. The Gitleaks adapter returned `FAILED` with
`binary pin mismatch: the resolved executable is not the pinned binary`, and the
verdict became `INDETERMINATE`. Gitleaks did not run.

**Operational consequence:** a machine whose Gitleaks or OSV-Scanner is not the pinned
build will report those adapters as `FAILED` until the pinned binary is provisioned.
Use the fetch scripts (`scripts/security/fetch-osv-scanner.sh`, and the Gitleaks step in
`scripts/checks/security-integration.sh`) or CI's provisioning. Local package-manager
installs do not match the release digests.

## Platform verification

| Platform | What ran | Result |
| --- | --- | --- |
| darwin arm64 (host) | baseline scan with pinned Gitleaks and OSV-Scanner; negative control | PASS |
| linux arm64 (container) | pinned Gitleaks linux/arm64 extracted from its archive: sha256 matches the committed digest; `gitleaks version` = 8.30.1 | PASS |
| linux amd64 | not run | NOT_RUN |
| darwin amd64 | not run | NOT_RUN |
| linux arm64 OSV-Scanner | not run; the committed value is the manifest asset pin, which `fetch-osv-scanner.sh` checks before installing | NOT_RUN |

## Provider model provenance is outside SOP

A binary pin proves the bytes of a local scanner executable. It does not establish the
provenance of a model, and SOP does not pin a model digest. See
[llm04-supply-chain.md](llm04-supply-chain.md), "Model provenance".
