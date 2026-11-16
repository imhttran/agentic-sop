# LLM03 scanner integrity: execution-time evidence (OWASP gap 9)

**As of:** `main` at `6ea2221f` plus the rules-staging change in PR #462
(2026-11-12). **Scope:** execution-time integrity for local scanner binaries and
rules in `internal/security`. Policy items — committed per-platform binary pins,
publisher signatures, and required categories — are not decided here; they are in
[PROPOSAL-SCANNER-PIN-POLICY.md](../../plans/PROPOSAL-SCANNER-PIN-POLICY.md).

## Binary hash-then-exec race (closed by PR #410)

Before PR #410, the pin was hashed and the original path was then executed, so a
writer could replace the file between the hash and the exec. `pinnedExecutor.check`
now copies the resolved binary into a private directory while hashing the same
bytes, then executes that copy; the copy is removed after the run. A mismatch
fails before any process starts.

Named test: `TestPinnedExecutor` (`internal/security/pinned_test.go`) rewrites the
original after the check and asserts that the executed path is a private copy with
the pinned digest, that the copy is removed after the run, and that a one-byte
replacement fails closed and starts nothing.

## Committed per-platform binary pins (PR #461)

The operator approved item 1 on 2026-11-12. Gitleaks 8.30.1 and OSV-Scanner 2.7.0
now carry `binary_sha256_by_platform` digests for linux and darwin (amd64 and
arm64); the baseline run and the negative control are recorded in
[llm03-scanner-binary-pins.md](llm03-scanner-binary-pins.md). Cargo-audit, Semgrep,
and Windows remain unpinned.

## Rules-tree hash-then-read race (closed in PR #462)

The rules pin had the same shape: `rulesIdentity` hashed the rules tree, then the
scanner read the original writable path. A writer could change the rules between
the digest and the read. `stageRules` now copies the rules file or directory into a
private directory while computing the same digest, verifies the copy against the
pin, and hands that copy to Semgrep or Gitleaks. Provenance records the digest of
the staged bytes, which equals the pin on success. A symlinked rules root or any
symlink or other non-regular entry inside the tree is refused; a mismatch fails
closed before the scanner starts.

Named tests:

- `TestRulesPinStagesVerifiedTree` — the staged bytes survive an original rewrite,
  the copy is removed after use, a changed tree reports `rules pin mismatch`, and a
  symlink entry is refused.
- `TestSemgrepRulesPinStagesTheTreeItRuns` — a recording executor rewrites the
  original at execution time and asserts that Semgrep's `--config` points at the
  staged copy with the pinned bytes.
- `TestGitleaksRulesPinStagesTheConfigItRuns` — the same assertion for Gitleaks.

The existing `TestRulesPin` continues to cover the relative-path resolution and
the symlink refusal through the adapter.

## Real-scanner check (macOS arm64 host, 2026-11-12)

Fetched the pinned production rules, then ran the tracked configuration with the
local Semgrep 1.183.0:

```text
scripts/security/fetch-semgrep-rules.sh
# rules=d90ff1b1729b740b1754f6b6ba8d1c9da8525136aa4a2345797f43e770fc5958
# files=66 commit=17e31e11ebac99fb7dfe1cd288f7a32151b6f3f9

go run ./cmd/sop security scan --report-only
# adapter semgrep (sast): COMPLETED, coverage COMPLETE
# provenance rules_identity=d90ff1b1729b740b1754f6b6ba8d1c9da8525136aa4a2345797f43e770fc5958
# provenance scanner_version=1.183.0
```

Appending one line to one rule file changed the tree digest and made the adapter
`FAILED` before Semgrep started:

```text
rules pin mismatch: rules digest 7c6e8ea7b2a90c6b30d243fbd0b70378beaa7c70d67125b8f640b3a46b79596c
is not the pinned digest
```

The rules tree was then re-fetched and the manifest digest matched again. This is a
report-only observation on a dirty worktree; it is not a clean-gate baseline and it
says nothing about the 45 unsuppressed findings the dirty scan reported.

## Local compatibility observation (macOS arm64, 2026-11-12)

With the per-platform pins from PR #461, the tracked configuration fails the
Gitleaks adapter closed on this host: the Homebrew executable's digest
(`f414bc2f…`) is not the committed `darwin/arm64` release pin (`ba52fb1b…`).
Semgrep has no binary pin and completed normally with the staged rules.
Report-only mode records the failure without blocking; a developer who needs a
completed secrets adapter must provision the pinned release binary.

## Platform record

| Platform                              | Command                                                                                                                                                                                           | Result  | Notes                                                                                                                        |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------- |
| macOS arm64 (host)                    | `go test ./internal/security -run 'TestPinnedExecutor\|TestRulesPinStagesVerifiedTree\|TestSemgrepRulesPinStagesTheTreeItRuns\|TestGitleaksRulesPinStagesTheConfigItRuns\|TestRulesPin' -count=1` | PASS    | Host run for this evidence.                                                                                                  |
| Linux arm64 (`golang:1.27` container) | same focused command                                                                                                                                                                              | PASS    | `go1.27.4 linux/arm64`.                                                                                                      |
| Linux arm64 (container)               | `go test ./internal/security/... ./internal/config/... -count=1`                                                                                                                                  | PASS    | Full package suites.                                                                                                         |
| Linux amd64                           | not run                                                                                                                                                                                           | NOT_RUN | CI runs amd64; this record does not claim it.                                                                                |
| Windows                               | not run                                                                                                                                                                                           | NOT_RUN | Copying and executing a pinned scanner on Windows is not verified here.                                                      |
| noexec temp mount                     | not measured                                                                                                                                                                                      | NOT_RUN | Documented fail-closed behavior: the staged copy cannot start and the adapter reports `ErrUnavailable`. Not reproduced here. |

## Baseline history — W8 stage 1 (2026-11-16)

Stage 1 activation of the scanner gate (see
[PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md](../../plans/PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md))
retired three baseline entries that no longer matched any finding, and added one.
The retired records are preserved here verbatim so their approval history survives
the removal. The baseline is now 82 entries.

### Retired (match nothing at any current revision)

All three were `dangerous-exec-command` in
`internal/toolharness/isolation_test.go`, owner `imhttran`, expiring
`2027-06-30T00:00:00Z`. Their original reason was: *"Test-only harness helper that
starts a sandbox or probes isolation; it cannot run in production. D4-3 group B,
reviewed 2026-11-13."*

| Fingerprint | Retired because |
| --- | --- |
| `semgrep:11ef53417d1b9eedc8e24e1593bdfa38b6d8df035e3265fce7c61bff2aa3e5c2` | W6 (PR #499) rewrote the probe helper to name the backend literally, so the rule no longer fires there |
| `semgrep:765bec591c74b4bc0a7bfbee12daaeb711a7123391832670c4d7d8d2a99d9f28` | same |
| `semgrep:fbedd5f6a2ed946cd639a20ce584f6ab65c3788767ea09cbb60e2c08294313b2` | same |

An entry that suppresses a finding which cannot occur is not protection; it is an
inaccurate statement about the code that makes a future real finding easier to
miss.

### Added

| Fingerprint | Path | Expiry | Justification |
| --- | --- | --- | --- |
| `semgrep:3dcf22239e8d13ec5dcc3baab9b377f8f0e56ec5576ea07652217384910fea00` | `internal/toolharness/harness.go` | `2027-03-31` | The single model-reachable exec sink; every caller goes through `execIsolated`, and `TestModelReachableExecIsIsolated` fails the build if a bypass is introduced |

The added entry is not a risk acceptance: the call site stays visible in the scan
output as suppressed, expires on its own, and is backed by an executable guard.

### Verified activation (2026-11-16, merged `e24c8108`)

Clean tree, pinned adapters (Gitleaks 8.30.1, Semgrep 1.183.0, OSV-Scanner 2.7.0,
cargo-audit 0.22.2 reporting no applicable Rust content):

```
verdict PASS, coverage COMPLETE, 0 execution failures
findings 98, suppressed 82, unsuppressed 16, blocking 0
mode required, required_mode ready=true, verdict PASS
```

Negative controls exercised before merge: the entry removed ⇒ `FAIL` with 1
blocking (exit 1); the entry expired ⇒ `FAIL`, suppressed 82 → 81; a newly
introduced dynamic exec ⇒ `FAIL` with that path blocking. Baseline audit: 82
entries, 0 stale, 0 suppressions without an approved entry, every entry carrying
owner, reason and expiry.

## Provider model provenance is outside SOP

A scanner binary or rules pin proves the bytes SOP executes or reads, not where a
model came from. SOP names a model and does not load weights or receive an
immutable model digest. Publisher authentication of scanners and general DLP also
remain outside SOP. See [llm04-supply-chain.md](llm04-supply-chain.md).
