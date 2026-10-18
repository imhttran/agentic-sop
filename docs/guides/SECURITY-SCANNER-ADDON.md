# Security Scanner Add-on Guide

This guide covers setup, report-only rollout, adapter extension, baselines,
rollback, and the MVP's measured scope. The normative contract is
[SECURITY-SCANNER-ADDON.md](../specs/SECURITY-SCANNER-ADDON.md).

## 1. Prerequisites

- Go and Git (SOP's normal requirements).
- One or more scanner executables on `PATH`, or absolute paths in the adapter
  command.
- Local Semgrep rules. The add-on never downloads rules implicitly. Use
  `scripts/security/fetch-semgrep-rules.sh` to fetch the pinned upstream rules
  into `.agent-sdlc/security-rules` and verify the manifest and directory digest.
  The pinned manifest includes Go and Rust security rules. The rules are
  license-restricted and are not committed.
- Optional local Gitleaks configuration. Without one, Gitleaks uses its built-in
  rules.
- Rust dependency scanning: `cargo audit` and an explicit local advisory
  database. The add-on never fetches or updates the database. Use a pinned
  database checkout or a pinned fixture and set `security.advisory_database_path`
  (or an adapter `database` path).

The MVP wrappers target Semgrep Community Edition 1.x, Gitleaks 8.x, and
cargo-audit 0.21+ JSON output. Pin the versions you actually run:

```bash
semgrep --version
gitleaks version
```

The add-on does not install scanners. A missing executable is reported as
`UNAVAILABLE`; a missing Semgrep rules path is reported as `FAILED`.

## 2. Minimal report-only configuration

Add this to `.agent-sdlc/config.yaml`:

```yaml
security:
  enabled: true
  mode: report-only
  scope: workspace
  fail_on: [critical, high]
  unknown_severity: indeterminate
  timeout_seconds: 300
  max_output_bytes: 1048576
  # Declared scope boundary. Recorded in scan.json under scope.excluded.
  # A leading "/" anchors a pattern to the repository root.
  exclude:
    - internal/security/testdata
    - /sop
  # Local advisory database for Rust dependency scanning. Explicit; no fetch.
  advisory_database_path: .agent-sdlc/advisory-db
  stale_advisory_database: deny
  auto: false
  adapters:
    - id: semgrep
      category: sast
      protocol: builtin
      command: [semgrep]
      rules: .agent-sdlc/security-rules
      version: "1.99.0" # replace with the pinned version
    - id: gitleaks
      category: secrets
      protocol: builtin
      command: [gitleaks]
      # Optional local Gitleaks config. Omit it to use Gitleaks' built-in rules.
      # rules: security/gitleaks.toml
      version: "8.30.1" # replace with the pinned version
    - id: cargo-audit
      category: dependencies
      protocol: builtin
      command: [cargo, audit]
      database: .agent-sdlc/advisory-db
      lockfile: Cargo.lock # optional; otherwise resolved from content
      stale: deny
      version: "0.21.2" # replace with the pinned version
```

An omitted `security` block is disabled. The environment may fill omitted
values (`SOP_SECURITY_ENABLED=true`, `SOP_SECURITY_MODE=report-only`,
`SOP_SECURITY_BASELINE=...`), but an explicit project value wins.

`security.exclude` is a declared scope boundary, not a suppression. Excluded
paths are removed from the adapter target set, listed in `scope.excluded`, and
never turned into findings. Semgrep receives the patterns as `--exclude`; for
Gitleaks, SOP stages a filtered view so excluded files are never read by the
scanner. Exclusions do not by themselves make coverage `PARTIAL`; a scanner-side
skip (for example Semgrep's default `.semgrepignore` patterns, or a staged-view
symbolic link that was not followed) still does. `.git` and `.agent-sdlc` are
always excluded: VCS metadata is not source and SOP's run state is evidence, not
project content.

Run a scan:

```bash
sop security inspect --json
sop security scan
sop security scan --json
sop security scan --auto --report-only --json
```

`sop security inspect --json` is read-only. It reports detected languages,
manifests, Cargo workspaces, actual and missing lockfiles, configured adapter
status (`available`, `unavailable`, `unsupported`, `proposed`), and proposed
adapters. It executes no scanner.

`--auto` adds missing built-in categories from detected content; explicit
adapters are never replaced. Go content proposes Gitleaks + Semgrep with Go rules;
Rust content also proposes cargo-audit; mixed content is the union. Go-only rules
are never applied to Rust source; the adapter reports unsupported coverage.

Report-only exits `0` even when the verdict is `FAIL` or `INDETERMINATE`. It
still records the honest verdict and evidence. Required mode exits non-zero for
anything except `PASS`.

### Exclusions and coverage policy

Declared `security.exclude` patterns are the only things that are out of scope.
Everything else is inside the declared scope, and a scanner skip there is a
coverage gap — never clean coverage. Coverage is separate from findings: `COMPLETE`
means the declared scope was scanned, not that it has no vulnerabilities.

| Category                          | Default treatment                                                                                                               |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Declared `security.exclude` paths | Out of scope. Recorded in `scope.excluded` and provenance `exclusions`; a matching skip is not a gap.                           |
| Generated output, vendor trees    | In scope. Declare them explicitly if they should not be analyzed.                                                               |
| Fixtures and test data            | In scope. Declare them explicitly if they should not be analyzed.                                                               |
| Test files (`*_test.go`)          | Skipped by Semgrep's built-in `.semgrepignore` defaults, so coverage is `PARTIAL` unless the repository overrides that default. |
| Files over Semgrep's size limit   | Skipped by the scanner, so coverage is `PARTIAL`.                                                                               |
| Legacy or non-production trees    | In scope. Declare them explicitly if they are out of scanning scope.                                                            |

```text
COMPLETE     recognized summary, no in-scope skips
PARTIAL      any in-scope skip (.semgrepignore, size limit, unknown reason)
UNKNOWN      missing, truncated, or unrecognized summary -> verdict INDETERMINATE
             (also: zero targets scanned while source languages were detected)
```

Diagnostics name every configured exclusion and every observed skip class with
its count. Do not add exclusions merely to obtain `COMPLETE`: widening the scope
exclusion is a reviewed scope decision, not a coverage fix.

The command writes:

```text
.agent-sdlc/runs/<run-id>/security/scan.json
.agent-sdlc/runs/<run-id>/security/scan.md
.agent-sdlc/runs/<run-id>/security/<adapter>.native.json   # sanitized, optional
```

## 3. Enabling the blocking gate

```yaml
security:
  enabled: true
  mode: required
  scope: workspace
  required_categories: [sast, secrets]
  fail_on: [critical, high]
  unknown_severity: indeterminate
```

Required mode fails closed when:

- a required category has no configured adapter,
- a required adapter is not `COMPLETED`,
- coverage is not `COMPLETE`,
- the scope is `changed-files` (a full-scan policy cannot pass on optimization
  evidence),
- a dependency adapter has a missing lockfile, unavailable advisory database, or
  stale advisory database (either stale policy),
- an unsuppressed finding is at a `fail_on` severity,
- unknown severity follows an explicit `fail` or `indeterminate` policy.

A security failure enters the existing bounded fix loop and quality gate. It
does not create a new lifecycle, retry counter, or approval surface.

## 4. Using the add-on inside `sop run`

With `security.enabled: true`, every task reaches the security stage at the
existing quality seam after validation/review and before the gate. The task's
run directory holds `security/scan.json`; the run report includes a Security
section. No separate command is required.

If the scan cannot produce evidence, the result is `INDETERMINATE`:

- report-only adds an advisory and leaves the normal gate unchanged;
- required mode fails the gate.

## 5. Rust scanning

### Dependency advisories (cargo-audit)

cargo-audit resolves the Cargo workspace and its actual `Cargo.lock`; a workspace
member does not own a lockfile. The adapter always passes `--file`, `--json`,
`--no-fetch`, and an explicit `--db`. It never generates or modifies a lockfile,
never runs `fix`, and never uploads source.

Provision the pinned executable once, separately from scanning:

```bash
scripts/security/fetch-cargo-audit.sh --dest .agent-sdlc/cargo-audit
```

The script downloads the release asset pinned by
`scripts/security/cargo-audit-manifest.json`, verifies its SHA-256, and checks the
binary's reported version. It is the only networked step. Point `command` at the
provisioned binary (for example `[.agent-sdlc/cargo-audit/cargo-audit, audit]`)
and set an exact `version`. Before any scan the adapter runs `--version` and fails
closed when the pin is missing, the output is unrecognized, or the reported
version differs.

- `advisory_database_path` (global) or an adapter `database` path is required. A
  missing path is `FAILED`, not a clean scan. The database is operator-supplied
  and local; scans never fetch or update it.
- `stale_advisory_database` / adapter `stale` is `deny` by default. A database
  older than 90 days stays non-clean: `deny` marks it `INDETERMINATE`; `allow`
  passes `--stale`, records `PARTIAL` coverage, and still cannot satisfy a
  required gate. A missing freshness timestamp is treated as stale.
- The normalized finding preserves the advisory ID, affected package/version,
  CVSS/severity, aliases/CVEs, patched and unaffected versions, references, and
  the lockfile path. Advisories are separate from tool/database failures.
- Provenance records the observed tool version, the local database identity
  (including local content changes), and a lockfile identity of every resolved
  path plus its content digest. Resume re-executes when any of them changes.
- Unsupported target (no Rust content), missing or incompatible tool, missing
  lockfile, missing or invalid database, a stale database under `deny`, malformed
  output, a timeout, and a tool failure each produce a distinguishable non-clean
  result. None is a clean dependency scan.
- The real-tool integration check runs the pinned binary with `--no-fetch` against
  the controlled advisory database at
  `internal/security/testdata/cargo-audit/advisory-db`, which contains a known
  `RUSTSEC-2020-0071` entry for `time 0.1.44`. It also asserts that the scan
  leaves both the lockfile and the database byte-identical.

### Source analysis (Semgrep)

Semgrep coverage is decided per detected source language from the `languages` the
local rules declare. It is never inferred from a path name or from another
language's rules.

- The pinned production ruleset includes `go/...` and `rust/lang/security/...`, so
  a Rust-bearing workspace is analyzed by Rust rules instead of being reported
  `UNSUPPORTED` when the configured rules path covers it. The default provisioning
  destination is `.agent-sdlc/security-rules`, populated by
  `scripts/security/fetch-semgrep-rules.sh`; `security.adapters[].rules` must
  point at that path.
- Rust coverage is claimed only when the configured rules declare `rust`. A
  Go-only ruleset is never applied to Rust source: the adapter refuses before
  execution and reports `FAILED` with an explicit `UNSUPPORTED` diagnostic.
- A ruleset that covers only part of the detected source is `COMPLETED` with
  `PARTIAL` coverage and a diagnostic naming the unsupported languages, so a
  mixed Go/Rust workspace keeps independent coverage per language.
- Malformed rules, a rule pattern the pinned scanner rejects, a scanner failure,
  or a timeout never produce `COMPLETE` coverage or a clean result.
- `rules_identity` in the scan evidence is the digest of the configured rules
  path, so a rules change is visible and forces a resume re-execution.
- The repository-owned rules used to verify this behavior live in
  `internal/security/testdata/real/rules-rust.yaml` and declare only
  `languages: [rust]`. They are exercised by the tagged real-CLI integration
  suite, not by the default unit-test run.
- The Rust rules are a small set of syntactic checks, not comprehensive Rust
  security analysis. A clean Rust result means only that these checks found
  nothing.

## 6. Replacing or adding an adapter

Any adapter can be replaced through configuration alone. A private adapter
implements `security-scan/v1` as a local executable: one JSON request on stdin,
one JSON response on stdout, diagnostics on stderr, no shell.

```yaml
security:
  enabled: true
  mode: report-only
  adapters:
    - id: private-sast
      category: sast
      protocol: external
      command: [/opt/acme/bin/acme-sast-adapter]
      version: "1.0.0"
```

Request shape (abridged):

```json
{
  "protocol_version": "security-scan/v1",
  "run_id": "T001",
  "workspace_root": "/work/repo",
  "content": {
    "commit": "...",
    "tree": "...",
    "digest": "...",
    "dirty": false
  },
  "scope": { "mode": "workspace" },
  "deadline_ms": 300000
}
```

Response shape (abridged):

```json
{
  "adapter_id": "private-sast",
  "category": "sast",
  "status": "COMPLETED",
  "exit_code": 0,
  "coverage": "COMPLETE",
  "findings": [
    {
      "rule_id": "ACME-001",
      "original_severity": "error",
      "severity": "HIGH",
      "message": "bounded message",
      "path": "internal/example.go",
      "line": 10,
      "fingerprint": "private-sast:stable-id"
    }
  ],
  "provenance": {
    "scanner_version": "1.0.0",
    "rules_identity": "sha256:...",
    "configuration_digest": "sha256:..."
  }
}
```

Requirements:

- `adapter_id` and `category` must match the configuration.
- Paths must be repository-relative. Unsafe paths are removed and reported.
- Fingerprints must be stable and non-secret.
- Raw secret values must not appear in findings, messages, metadata, or
  diagnostics.
- A non-zero exit, malformed JSON, empty output, oversized output, or contract
  validation failure is `FAILED`, never `COMPLETED`.

The conformance behavior is covered by
`internal/security/security_test.go` and
`internal/cli/security_test.go`.

## 7. Baselines and suppressions

Create a reviewed file, for example `.agent-sdlc/security-baseline.json`:

```json
{
  "version": 1,
  "entries": [
    {
      "fingerprint": "semgrep:<stable-id>",
      "rule_id": "optional-rule",
      "path": "optional/path",
      "reason": "accepted for the migration window",
      "owner": "security-team",
      "expires_at": "2027-01-01T00:00:00Z"
    }
  ]
}
```

Then configure `security.baseline` or pass `--baseline PATH`. Findings remain
visible with `"suppressed": true`. Expired entries are visible with
`"expired": true` and do not suppress. Invalid entries are ignored and reported.
Automatic baseline creation is not part of the MVP.

## 8. Resume

```bash
sop security scan --resume security-20261017T120000Z
```

A completed adapter is reused only when content, adapter/scanner, rules,
advisory database identity and stale state, configuration, and policy identities
still match. Any change reruns that adapter. A prior `FAILED`, `TIMED_OUT`,
`UNAVAILABLE`, `SKIPPED`, or `CANCELLED` result is never reused. The prior run's
sanitized native reports are loaded and verified against their recorded digests.
A missing or mismatched report is not reused: the adapter is rescanned, and the
current run writes the verified report beside its own evidence.

## 9. Rollback

Disable the add-on through configuration:

```yaml
security:
  enabled: false
```

Or remove the block. Existing non-security verification, approvals, retry
budgets, and continuation counters are unchanged. Prior sanitized evidence stays
under `.agent-sdlc/runs/<id>/security/`. Revert implementation commits only if
the integration changed existing runner behavior unexpectedly.

## 10. Limitations

- Changed-file scope is an optimization and never satisfies a required
  full-scan policy.
- Declared `security.exclude` patterns are scope, not coverage. They are recorded
  in the evidence and must be reviewed like a baseline, but they do not hide a
  scanner-side skip.
- Semgrep CE applies its default `.semgrepignore` patterns even with
  `--no-git-ignore`; `*_test.go` files are skipped by default. A production
  baseline therefore covers non-test source unless the project overrides that
  default with a `.semgrepignore` negation. Test files remain covered by Gitleaks
  for secrets.
- A required security failure is classified as `SECURITY_SCAN_FAILURE` and stops
  the run for operator review. It does not enter the code-fix loop, does not
  create an approval gate, and is never auto-remediated.
- Semgrep rule IDs are recorded without a local path prefix
  (`--no-rewrite-rule-ids`), so fingerprints and baselines stay stable when the
  rules directory moves.
- Unknown provider severities follow an explicit policy; they never become low
  severity.
- Cross-provider deduplication is deferred. Grouped evidence retains every
  source.
- Dependency scanning covers Rust Cargo lockfiles. Go module or other ecosystem
  dependency advisories are not implemented; report them as unsupported instead
  of assuming a clean result. This overlaps the dependency-scanning note tracked
  in issue #40 without closing it.
- Rust Semgrep coverage is limited to the selected pinned `rust/lang/security`
  rules plus the repository-owned verification rules. It is not comprehensive
  Rust SAST, and a `PARTIAL` result means part of the detected source was not
  analyzed.
- Hosted, runtime/network, and asset-discovery scans are out of MVP scope.
- A passing scan is evidence for the configured tools and scope. It is not proof
  that the repository has no vulnerabilities.
- The MVP does not claim sandbox isolation.
- Real scanner validation requires the pinned CLIs and local rules. Absent tools
  are reported explicitly rather than treated as clean.

## 11. Validation

```bash
go test ./internal/security/...
go test ./internal/config/...
go test ./internal/quality/...
go test ./internal/cli/...
go test ./internal/skill/...
go test ./internal/planner/... -run TestSecurityScannerPlanCompiles
go build ./...
go vet ./...
bash scripts/checks/check-doc-links.sh
bash scripts/checks/security-integration.sh
```

`security-integration.sh` installs Semgrep from the hash-pinned
`scripts/security/semgrep-requirements.lock`, fetches the pinned production rules
through `scripts/security/fetch-semgrep-rules.sh`, verifies their manifest and
directory digest, and then runs the real-CLI tests.

`scripts/checks/security-integration.sh` provisions pinned Semgrep CE and Gitleaks
versions in an isolated tools directory, uses explicit local rules, and runs the
tagged real-CLI checks. It sets `SOP_SECURITY_INTEGRATION_REQUIRED=1`: a missing
binary or version mismatch fails the check. The same script runs in CI.

The real cargo-audit check is gated on `SOP_SECURITY_CARGO_ADVISORY_DB` (and
optionally `SOP_SECURITY_CARGO_BIN`). Without a pinned local database it reports a
skip and never a clean dependency result.
