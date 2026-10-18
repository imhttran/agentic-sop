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
- Go and Rust dependency scanning with OSV-Scanner: the pinned binary plus an
  explicit offline OSV cache. Provision the binary with
  `scripts/security/fetch-osv-scanner.sh` and the required ecosystem archives
  with `scripts/security/fetch-osv-databases.sh`. Scans never download either.

OSV and cargo-audit database formats are separate inputs. Do not point an OSV
adapter at `security.advisory_database_path` or a cargo-audit adapter at
`security.osv_database_path`.

The MVP wrappers target Semgrep Community Edition 1.x, Gitleaks 8.x,
cargo-audit 0.21+ JSON output, and OSV-Scanner 2.7.x JSON output. Pin the versions
you actually run:

```bash
semgrep --version
gitleaks version
osv-scanner --version
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
  # Local advisory database for cargo-audit. Explicit; no fetch.
  advisory_database_path: .agent-sdlc/advisory-db
  # Separate OSV-Scanner offline cache. Explicit; no fetch or live API fallback.
  osv_database_path: .agent-sdlc/osv-db
  stale_advisory_database: deny
  auto: false
  # Category controls. Omitted entries inherit enabled: true; explicit false
  # disables the whole category and cannot be undone by auto.
  categories:
    sast: true
    secrets: true
    dependencies: true
  adapters:
    - id: semgrep
      category: sast
      protocol: builtin
      command: [semgrep]
      rules: .agent-sdlc/security-rules
      languages: [go, rust]
      version: "1.99.0" # replace with the pinned version
    - id: gitleaks
      category: secrets
      protocol: builtin
      command: [gitleaks]
      # Optional local Gitleaks config. Omit it to use Gitleaks' built-in rules.
      # rules: security/gitleaks.toml
      version: "8.30.1" # replace with the pinned version
    # Recommended for new configurations: OSV covers Go go.mod and Rust Cargo.lock.
    - id: osv-scanner
      category: dependencies
      protocol: builtin
      command: [osv-scanner]
      database: .agent-sdlc/osv-db
      languages: [go, rust]
      # inputs: [go.mod, Cargo.lock] # optional; otherwise resolved from content
      stale: deny
      version: "2.7.0" # replace with the pinned version
    # Existing Rust-only configurations keep cargo-audit. It is never replaced by
    # OSV automatically; select the replacement explicitly if that is intended.
    # - id: cargo-audit
    #   category: dependencies
    #   protocol: builtin
    #   command: [cargo, audit]
    #   database: .agent-sdlc/advisory-db
    #   lockfile: Cargo.lock
    #   languages: [rust]
    #   stale: deny
    #   version: "0.22.2"
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
manifests, Cargo workspaces, actual and missing lockfiles, configured and selected
adapters, effective enable/disable state and reason, capability/language
coverage, explicit overlaps, missing prerequisites, required-mode readiness, and
proposed adapters. It executes no scanner and reads no database content.

`--auto` fills only unconfigured, enabled coverage from detected content. It
never replaces or re-adds an explicitly configured adapter, and it cannot undo an
explicit disable. Go dependencies use OSV; Rust dependencies use the documented
legacy cargo-audit choice. OSV for Rust requires an explicit selection so
overlapping automatic coverage is not run twice. Go-only Semgrep rules are never
applied to Rust source; the adapter reports unsupported coverage.

Automatic OSV selection does not invent a version pin. Without an explicitly
configured `osv-scanner` adapter carrying `version`, inspection reports
`unsupported` with `osv-scanner requires an exact version pin`, required-mode
readiness is not ready, and the scan fails closed. Configure the OSV adapter
explicitly with its `version` before relying on OSV coverage.

### Selecting scanners by capability and language

Selection is deterministic and keeps three things separate:

- **category** (`sast`, `secrets`, `dependencies`),
- **language** (`go`, `rust`), and
- **adapter identity** (`semgrep`, `gitleaks`, `cargo-audit`, `osv-scanner`, or an
  external id).

A category may contain multiple adapters. An adapter may cover multiple
languages. Secrets scanners are repository-wide. A dependency or SAST adapter
that declares no languages cannot prove language-specific required coverage; a
legacy external adapter is reported as undeclared rather than assumed to cover Go
or Rust.

Enable/disable precedence:

1. `security.enabled: false` prevents every scanner from running.
2. `security.categories.<category>: false` disables every adapter in it.
3. `security.adapters[].enabled: false` disables that adapter.
4. An omitted `enabled` value is not the same as `false`; it inherits the
   category/global state.
5. Explicit selection wins over automatic selection.
6. Automatic selection never re-adds an explicitly disabled adapter or creates
   overlapping coverage.
7. Explicit overlapping coverage is allowed, is shown in `overlaps`, and both
   adapters run.

### Configuration examples

**1. Semgrep + Gitleaks + OSV for Go and Rust**

```yaml
security:
  enabled: true
  osv_database_path: .agent-sdlc/osv-db
  adapters:
    - {
        id: semgrep,
        category: sast,
        command: [semgrep],
        rules: .agent-sdlc/security-rules,
        version: "1.99.0",
      }
    - {
        id: gitleaks,
        category: secrets,
        command: [gitleaks],
        version: "8.30.1",
      }
    - {
        id: osv-scanner,
        category: dependencies,
        command: [osv-scanner],
        version: "2.7.0",
      }
```

**2. OSV for Go and cargo-audit for Rust**

```yaml
security:
  enabled: true
  osv_database_path: .agent-sdlc/osv-db
  advisory_database_path: .agent-sdlc/advisory-db
  adapters:
    - {
        id: osv-scanner,
        category: dependencies,
        command: [osv-scanner],
        languages: [go],
        version: "2.7.0",
      }
    - {
        id: cargo-audit,
        category: dependencies,
        command: [cargo, audit],
        languages: [rust],
        database: .agent-sdlc/advisory-db,
        version: "0.22.2",
      }
```

Mixed Go/Rust coverage is evaluated separately. cargo-audit cannot satisfy Go
dependency coverage.

**3. One disabled adapter with an enabled replacement**

```yaml
security:
  enabled: true
  adapters:
    - {
        id: cargo-audit,
        category: dependencies,
        command: [cargo, audit],
        languages: [rust],
        enabled: false,
        version: "0.22.2",
      }
    - {
        id: osv-scanner,
        category: dependencies,
        command: [osv-scanner],
        languages: [rust],
        version: "2.7.0",
      }
```

**4. A disabled category**

```yaml
security:
  enabled: true
  categories:
    dependencies: false
  adapters:
    - {
        id: osv-scanner,
        category: dependencies,
        command: [osv-scanner],
        version: "2.7.0",
      }
```

The disabled adapter is recorded as `SKIPPED` with `disabled: true`. If the
category has detected content and no enabled replacement, coverage is reported as
disabled and is not treated as clean.

**5. Globally disabled scanning**

```yaml
security:
  enabled: false
```

`security scan` reports `NOT_APPLICABLE` and executes nothing. If a required gate
is also configured (`mode: required` with `required_categories`), global disable
is not an exemption: the scan reports `INDETERMINATE` and required mode fails
closed.

**6. An external replacement adapter**

```yaml
security:
  enabled: true
  adapters:
    - id: private-sast
      category: sast
      protocol: external
      command: [/opt/security/private-sast]
      languages: [go, rust]
      version: "1.0.0"
```

An external adapter declares its languages explicitly. Without `languages` or
`repository_wide: true`, it remains compatible but cannot prove language-specific
required coverage.

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

- a required category has no enabled adapter,
- a required adapter is not `COMPLETED`,
- coverage is not `COMPLETE`,
- a required detected language (Go or Rust) has no `COMPLETE` coverage in that
  category, even when another language passed,
- a required category is explicitly disabled,
- the scope is `changed-files` (a full-scan policy cannot pass on optimization
  evidence),
- a dependency adapter has a missing input, unavailable advisory database, or
  stale advisory database (either stale policy),
- an unsuppressed finding is at a `fail_on` severity,
- unknown severity follows an explicit `fail` or `indeterminate` policy.

Global `enabled: false` does not bypass an active required gate. One successful
adapter does not hide another failed required coverage obligation. Removing a
required category from configuration remains an explicit policy change.

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

## 5. Go and Rust dependency scanning

### Dependency advisories (OSV-Scanner)

OSV-Scanner scans explicit Go `go.mod` files and present Rust `Cargo.lock` files.
It resolves nested Go modules and Cargo workspaces from content, respects
declared exclusions, and never generates or modifies a manifest or lockfile. A
detected Rust ecosystem without a `Cargo.lock` is a failure, not a clean result.

The adapter runs with `--offline`, `--all-packages`, `--no-resolve`, and
`--no-call-analysis=all`, against an explicit local cache. It never uses the live
OSV API and never downloads or refreshes a database. It passes a controlled empty
`--config` so a repository `osv-scanner.toml` cannot suppress findings. Each input
is scanned independently.

A version match is not proof that vulnerable code is reachable. Call analysis is
disabled, and findings record `reachability: not-evaluated` unless the scanner
supplied explicit analysis.

Provision the pinned binary and databases explicitly:

```bash
scripts/security/fetch-osv-scanner.sh --dest .agent-sdlc/osv-scanner
scripts/security/fetch-osv-databases.sh --dest .agent-sdlc/osv-db
```

The database layout is:

```text
.agent-sdlc/osv-db/osv-scalibr/Go/all.zip
.agent-sdlc/osv-db/osv-scalibr/crates.io/all.zip
```

Each archive has a sibling `.metadata.json` recording its source URL, SHA-256,
size, and acquisition time. A scan verifies the archive digest against that
metadata. A missing ecosystem, stale archive, malformed output, truncation,
timeout, or scanner failure is never clean.

### Dependency advisories (cargo-audit)

cargo-audit resolves the Cargo workspace and its actual `Cargo.lock`; a workspace
member does not own a lockfile. The adapter always passes `--file`, `--json`,
`--no-fetch`, and an explicit `--db`. It never generates or modifies a lockfile,
never runs `fix`, and never uploads source. It remains supported for existing
Rust-only configurations and is never replaced by OSV automatically.

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
- SOP owns `selection`, `languages`, `repository_wide`, and `disabled`. A provider
  response cannot claim an automatic/explicit selection or a disable state; SOP
  overwrites those fields from validated configuration and selection.
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

`--resume <run-id>` reuses a completed adapter only when content identity, adapter
ID, selection mode, declared language scope, scanner version, rules or
controlled-config identity, adapter configuration digest, policy identity
(including exclusions and execution bounds), resolved input identity, advisory
database identity, and rechecked database stale state all still match. A changed
lockfile invalidates reused dependency evidence. An unchanged database can still
become stale over time; freshness is rechecked on reuse. Disabled or
non-completed scans are never reused as passed scans.

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

- OSV-Scanner supports Go `go.mod` and Rust `Cargo.lock` dependency advisories.
  It does not add container, image, runtime API, or automatic remediation
  scanning.
- Call analysis and reachability are disabled. A dependency-version match does
  not prove the vulnerable code is reachable.
- OSV and cargo-audit databases are separate formats with separate paths. They
  are never substituted for one another.
- A legacy external adapter without language metadata remains compatible but
  cannot prove language-specific required coverage.
- Coverage reflects the declared scope. Adding exclusions is a reviewed scope
  decision, not a coverage fix.
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
- Dependency scanning covers Go `go.mod` and Rust `Cargo.lock` with OSV-Scanner,
  and Rust `Cargo.lock` with cargo-audit. Other ecosystems are unsupported and
  are reported as such instead of being assumed clean.
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

`scripts/checks/security-integration.sh` provisions pinned Semgrep CE, Gitleaks,
cargo-audit, and OSV-Scanner versions in an isolated tools directory, uses
explicit local rules, and runs the tagged real-CLI checks. It sets
`SOP_SECURITY_INTEGRATION_REQUIRED=1`: a missing binary or version mismatch fails
the check. The same script runs in CI. The OSV tests build a controlled offline
database fixture, assert clean and vulnerable Go/Rust results, verify inputs are
unchanged, assert missing-ecosystem behavior, and confirm that a repository
`osv-scanner.toml` cannot suppress SOP findings. They never depend on the live OSV
API.

The real cargo-audit check is gated on `SOP_SECURITY_CARGO_ADVISORY_DB` (and
optionally `SOP_SECURITY_CARGO_BIN`). Without a pinned local database it reports a
skip and never a clean dependency result.
