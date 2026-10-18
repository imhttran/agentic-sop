# Security Scanner Add-on

**Type:** Normative specification (MVP)
**Status:** Implemented for Semgrep Community Edition, Gitleaks, cargo-audit, and OSV-Scanner.
**Requirements source:** [PRD-Security-Scanner-Addon.md](../requirements/PRD-Security-Scanner-Addon.md)
**Plan:** [PLAN-Security-Scanner-Addon.md](../plans/PLAN-Security-Scanner-Addon.md)

## Purpose

This document defines the opt-in security scanner add-on: its adapter contract,
integration points, execution states, evidence, policy verdicts, baselines,
resume rules, and security boundaries. The add-on is disabled unless configured.
It reuses SOP's existing configuration, run artifacts, quality gate, retry and
continuation budgets, and Git boundary. It does not implement a scanner engine,
a second plugin framework, automatic installation, automatic fixes, or a
mandatory hosted service.

## Normative language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

## 1. Integration points

The add-on reuses these existing paths:

| Concern                     | Repository path                                                      | Behavior                                                                                                                                                     |
| --------------------------- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Configuration               | `internal/config/config.go`, `internal/config/security.go`           | Optional `security` block with strict unknown-key rejection. Disabled by default.                                                                            |
| CLI                         | `internal/cli/security.go`, `internal/cli/cli.go`                    | `sop security scan`; `sop security inspect`; `--json`, `--report-only`, `--required`, `--scope`, `--baseline`, `--resume`, `--run-id`, `--config`, `--auto`. |
| Run lifecycle               | `internal/cli/run.go`, `internal/cli/security_gate.go`               | Optional scan at the existing quality seam; report-only is advisory, required feeds the single quality gate.                                                 |
| Quality gate                | `internal/quality/quality.go`                                        | `SecurityEvidence` is folded into PASS/FAIL/NEEDS_HUMAN; it cannot add a second verdict authority.                                                           |
| Artifacts                   | `internal/run/run.go`, `internal/security/report.go`                 | Evidence under `.agent-sdlc/runs/<id>/security/`.                                                                                                            |
| Content identity            | `internal/git` (`Head`, `Tree`, `Status`, `DiffAll`, `ChangedFiles`) | Commit/tree identity; dirty content digest.                                                                                                                  |
| Execution controls          | `internal/run`, `internal/testrunner`, recovery specs                | One bounded scan per attempt; existing retry, approval, cancellation, and continuation controls apply.                                                       |
| Existing subprocess pattern | `internal/ragclient`, `internal/decision/command`                    | Direct argv, JSON request/response, bounded output, no shell.                                                                                                |

There is no second plugin framework. Adapter selection is configuration only.

## 2. Configuration

```yaml
security:
  enabled: true # disabled unless true (or SOP_SECURITY_ENABLED=true)
  mode: report-only # report-only | required
  scope: workspace # workspace | changed-files
  required_categories: [sast] # required when mode: required; sast | secrets | dependencies
  fail_on: [critical, high]
  unknown_severity: indeterminate # fail | indeterminate | ignore
  timeout_seconds: 300
  max_output_bytes: 1048576
  baseline: .agent-sdlc/security-baseline.json
  exclude: [internal/security/testdata] # declared scope; a leading "/" anchors to root
  # Local advisory database for cargo-audit. Explicit; never fetched implicitly.
  advisory_database_path: .agent-sdlc/advisory-db
  # Separate OSV-Scanner offline cache. OSV and cargo-audit database formats are
  # not interchangeable; OSV never falls back to advisory_database_path.
  osv_database_path: .agent-sdlc/osv-db
  stale_advisory_database: deny # deny (default) | allow
  auto: false # true fills unconfigured, enabled coverage from detected content
  # Category enable controls. Omitted entries inherit enabled: true; an explicit
  # false disables every adapter in that category and is not undone by auto.
  categories:
    sast: true
    secrets: true
    dependencies: true
  adapters:
    - id: semgrep
      category: sast
      protocol: builtin
      command: [semgrep]
      rules: .agent-sdlc/security-rules # local rules path; no implicit download
      languages: [go, rust] # optional; built-in default is go+rust
      version: "SET_THE_PINNED_VERSION"
    - id: gitleaks
      category: secrets
      protocol: builtin
      command: [gitleaks]
      version: "SET_THE_PINNED_VERSION"
    # Recommended dependency stack for new configurations: OSV for Go and Rust.
    - id: osv-scanner
      category: dependencies
      protocol: builtin
      command: [osv-scanner]
      database: .agent-sdlc/osv-db # explicit offline cache; no download on scan
      languages: [go, rust]
      inputs: [go.mod, Cargo.lock] # optional; otherwise resolved from content
      stale: deny # optional per-adapter override
      version: "SET_THE_PINNED_VERSION"
    # Existing Rust-only configurations keep cargo-audit; it is never replaced
    # by OSV automatically.
    # - id: cargo-audit
    #   category: dependencies
    #   protocol: builtin
    #   command: [cargo, audit]
    #   database: .agent-sdlc/advisory-db
    #   lockfile: Cargo.lock
    #   languages: [rust]
    #   stale: deny
    #   version: "SET_THE_PINNED_VERSION"
    # A private adapter replaces a built-in through configuration alone.
    # - id: private-sast
    #   category: sast
    #   protocol: external
    #   command: [/absolute/path/to/adapter]
    #   languages: [go, rust]
    #   version: "1.0.0"
```

### Selection and enable controls

Selection keeps capability, language, and adapter identity separate:

- A category (`sast`, `secrets`, `dependencies`) may have multiple adapters.
- An adapter may cover one or more languages (`go`, `rust`).
- A language-independent capability uses repository-wide scope. Secrets scanners
  are repository-wide by default; an external adapter declares
  `repository_wide: true` when it covers the whole repository.

Precedence is deterministic:

1. `security.enabled: false` prevents every adapter from running.
2. `security.categories.<category>: false` prevents every adapter in that
   category. It cannot be re-enabled by `auto` or by an adapter `enabled: true`.
3. `security.adapters[].enabled: false` prevents that adapter. `true` and an
   omitted value inherit the category/global state.
4. Explicit configuration wins over automatic selection.
5. `auto` fills only coverage that no explicit adapter, enabled or disabled, has
   claimed. It never re-adds an explicitly disabled adapter.
6. Automatic selection never creates overlapping coverage. Rust dependency
   coverage prefers the documented legacy `cargo-audit` choice; OSV for Rust
   requires an explicit selection.
7. Overlapping explicit coverage is allowed, is reported in `overlaps`, and both
   adapters run. Findings remain separate.

Automatic selection does not invent an OSV-Scanner version pin. An automatic OSV
candidate without a configured pin is reported `unsupported` with the pin
prerequisite; required-mode readiness fails closed, and the scan is not described
as ready. Version verification is never weakened, and no scan downloads a tool or
database.

A pointer distinguishes an omitted enable value from an explicit `false`. An
omitted value is not written back into configuration.

An adapter without language metadata (a legacy external adapter) can run and
report findings, but it cannot prove language-specific required coverage. It is
reported as undeclared rather than assumed to cover Go or Rust.

A category-level success never hides missing language coverage: dependency and
SAST required coverage is evaluated per detected language.

`SOP_SECURITY_ENABLED`, `SOP_SECURITY_MODE`, and `SOP_SECURITY_BASELINE` fill
values the project omitted. An explicit project value always wins. Invalid
values fail configuration load; they are never silently defaulted.

Semgrep rules MUST be local and explicit. The adapter never passes `--config
auto`, never downloads rules, and never uploads code. Rule downloads and hosted
services remain separate, explicit operator actions outside this add-on. The
repository provides `scripts/security/fetch-semgrep-rules.sh`, which fetches the
rules from the exact commit in `scripts/security/semgrep-rules-manifest.json`,
verifies the tarball, every selected file, and the deterministic directory digest,
and writes them to an ignored local directory. The pinned manifest includes Go and
Rust security rules. The upstream rules are license-restricted and are never
committed.

## 3. Adapter protocol v1

Every adapter speaks `security-scan/v1`. Built-in adapters and external
processes satisfy the same contract.

### Descriptor

| Field                           | Meaning                                               |
| ------------------------------- | ----------------------------------------------------- |
| `protocol_version`              | `security-scan/v1`                                    |
| `adapter_id`, `adapter_version` | Stable identity and pinned version                    |
| `categories`                    | `sast`, `secrets`, `dependencies`                     |
| `scope_modes`                   | `workspace`, `changed-files`                          |
| `languages`                     | Declared Go/Rust coverage; empty means undeclared     |
| `repository_wide`               | The adapter covers the repository, not a language set |
| `locality`, `network`           | Execution locality and data/network requirements      |

### Request

The request is sent either through the adapter interface (built-in) or
as JSON on stdin (external). It contains `run_id`, optional `task_id` and
`attempt_id`, `workspace_root`, content identity, scope, scanner configuration,
deadline, and output location. `scope` contains the mode, the changed-file list
when applicable, and `excluded`: the repository-relative paths or globs the
operator declared outside the scan. Exclusions are scope evidence, not a claim
that the scanner failed to read a file.

### Execution

The response contains `status`, `exit_code`, start/end time, `coverage`,
`selection`, `languages`, `repository_wide`, `disabled`, skipped files/reasons,
diagnostics, normalized findings, and provenance.

Execution statuses are `COMPLETED`, `FAILED`, `TIMED_OUT`, `UNAVAILABLE`,
`SKIPPED`, and `CANCELLED`. Coverage is `COMPLETE`, `PARTIAL`, or `UNKNOWN`,
reported independently of execution status. A disabled adapter is `SKIPPED` with
`disabled: true`; it is never a passed scan.

### Finding

A finding preserves provider, category, rule ID, original severity, normalized
severity, bounded message, safe relative location, stable non-secret
fingerprint, optional CWE/CVE/confidence, and sanitized provider metadata.
Provider identity is part of the fingerprint namespace. Cross-provider
deduplication is deferred; every source remains represented.

### Provenance

Provenance records scanner version, rules identity, configuration digest,
content identity, policy identity, selection mode, declared languages, advisory
database identity and metadata, stale state, lockfile/input identity, and the
sanitized native-report digest.

### Resume identity and freshness

Resume compares content identity, adapter ID, pinned scanner version, rules or
controlled-config identity, adapter configuration digest, policy identity
(including exclusions, execution bounds, database paths, and stale policy),
selection mode (`explicit`/`automatic`), declared language scope, resolved input
identity, advisory database identity, and database stale state. Any mismatch
re-executes the adapter.

Database freshness is rechecked when evidence is reused. An unchanged database
can become stale over time; the stale comparison therefore invalidates reuse.
The OSV and cargo-audit databases have separate identities and paths.

### External process rules

- The executable and its arguments are separate; no shell is used.
- Standard input carries one bounded JSON request; standard output carries one
  bounded JSON response; diagnostics stay on standard error.
- A response with a mismatched adapter ID/category, unsupported protocol,
  malformed JSON, empty output, non-zero exit, or output at the size limit is
  `FAILED`, never `COMPLETED`.
- The child environment is an allowlist (path/home/temp/locale/certificate
  variables). Credentials and provider tokens are not passed or logged.
- The external adapter has no lifecycle, approval, commit, merge, or policy
  authority.

## 4. Built-in adapters

### Semgrep Community Edition (`sast`)

- Invokes `semgrep scan --json --metrics=off --disable-version-check
--no-git-ignore --no-rewrite-rule-ids --config <local-rules> --timeout
<seconds> [--exclude <declared-scope-pattern> ...] <targets>` directly.
- `--no-rewrite-rule-ids` keeps the rule ID exactly as the local ruleset declares
  it, so a moved rules directory cannot change fingerprints or baselines.
- `--exclude` is passed for each `scope.excluded` pattern. A declared exclusion
  is scope, not a coverage gap; Semgrep's own `.semgrepignore`, size, git-target,
  parse, or access limitations still make coverage `PARTIAL`.
- Parses the native JSON report, maps `ERROR` to `HIGH`, `WARNING` to `MEDIUM`,
  and `INFO` to `INFO`; an unknown provider severity stays `UNKNOWN`.
- Accepts exit code `0` or `1` only when a valid JSON report is present.
- A missing rules path is `FAILED` with an explicit diagnostic; no rule
  download is attempted.
- Scanner errors or scanner-side skipped paths make coverage `PARTIAL`.
- Every reason in Semgrep's `Scan skipped:` summary is a gap unless it is a
  declared `--exclude` pattern. An unrecognized reason is still a gap: unknown
  scanner behavior never yields `COMPLETE`.

#### Language coverage

- Coverage is decided per detected source language from the `languages` the local
  rules declare. It MUST NOT be inferred from a path name or from another
  language's rules.
- A ruleset that declares no rule for any detected source language MUST fail with
  `FAILED` coverage `UNKNOWN` and an explicit `UNSUPPORTED` diagnostic. It is
  never a clean scan.
- When the ruleset covers only part of the detected source, the execution MUST be
  `COMPLETED` with coverage `PARTIAL` and a diagnostic naming the unsupported
  languages. Go and Rust coverage remain independent.
- Malformed rules, a rule pattern the pinned scanner rejects, a scanner failure,
  or a timeout MUST NOT produce `COMPLETE` coverage or a clean result.
- The pinned production ruleset (`scripts/security/semgrep-rules-manifest.json`)
  covers Go and Rust; its directory digest is recorded as `rules_identity`. The
  Rust scope is a small selected set of syntactic checks, not comprehensive Rust
  security analysis.

#### Exclusions and coverage evidence

Coverage evidence is the scanner's own report of what it skipped. It is
independent of the findings verdict: `COMPLETE` means the declared supported scope
was scanned, not that it is free of vulnerabilities.

- Declared `security.exclude` patterns are scope, not gaps. They are passed to the
  scanner as `--exclude`, recorded in `scope.excluded`, and recorded in provenance
  (`exclusions`). A skip attributed to a declared pattern is reported as declared
  out-of-scope and does not by itself make coverage `PARTIAL`.
- Every other skip inside the declared scope MUST make coverage `PARTIAL`. This
  includes `.semgrepignore` matches (Semgrep defaults and repository content),
  targets over the scanner's size limit, a target set restricted to git-tracked
  files, and any skip reason this build does not recognize.
- Missing, truncated, or unrecognized summary evidence MUST make coverage
  `UNKNOWN`, which the policy verdict reports as `INDETERMINATE`. A missing or
  unparseable summary never becomes `COMPLETE`.
- `COMPLETE` requires a recognized, terminated coverage summary and no in-scope
  skip. A summary that reports zero scanned targets while source languages were
  detected is `UNKNOWN`.
- Coverage MUST NOT be improved by widening exclusions. Adding an exclusion to
  obtain `COMPLETE` removes content from the declared scope; that is a scope
  decision, not a coverage fix.
- Observed skip reasons and counts are preserved as evidence. Target details are
  never invented beyond what the scanner reported.
- Generated output, vendored content, fixtures, and legacy or non-production trees
  are in scope by default. Excluding them requires an explicit `security.exclude`
  entry and a recorded reason; otherwise their skips are coverage gaps.

#### Coverage conformance

The adapter depends on Semgrep's human summary (the `Scan Summary` section, the
`Targets scanned:` count, the `Scan skipped:` reasons, and the closing `Ran ...`
line) because the JSON report does not expose those skip classes. A conformance
test runs the pinned scanner over no-skip, `.semgrepignore`, oversized, and
declared-exclusion fixtures and fails when that contract changes.

### Gitleaks (`secrets`)

- Invokes `gitleaks detect --source <root> --no-git --redact --report-format
json --report-path <restricted-temp> --exit-code 0 --no-banner` directly. When
  `security.adapters[].rules` is set, it is passed as `--config <local-config>`
  and its identity is recorded; otherwise Gitleaks uses its built-in rules.
- Parses the top-level array or `findings` object shape.
- Gitleaks has no path-exclusion flag, so SOP stages a filtered view: excluded
  paths are never linked into the scan view and are never read by Gitleaks. The
  adapter still filters any unexpected finding as a guard and records staged
  exclusions in diagnostics. Symbolic links are not followed; their presence
  makes coverage `PARTIAL`. Scanner-side skips reported by Gitleaks (for example
  an allowlist file skip) also make coverage `PARTIAL`. `.env` files are never
  excluded automatically; they are secret-scan targets unless the operator
  explicitly declares them outside the scope.
- Every secret finding is `HIGH`; original provider meaning (`secret`) is
  retained. Messages and metadata are sanitized.
- Raw report files live only in restricted temporary storage and are removed on
  completion, failure, and cancellation. Only a sanitized native report is
  persisted.

### Dependency obligations (`dependencies`)

Dependency analysis is defined by an explicit, deterministic inventory of
in-scope inputs, not by the lockfiles that happen to exist:

- Cargo workspace detection MUST parse `Cargo.toml` with a TOML library. A
  `[workspace]` declaration is a workspace even when it has no `members` list.
  Members, exclusions, globs, the root package of an ordinary workspace, explicit
  `package.workspace` references, and in-root path dependencies are resolved from
  content. Directory ancestry alone MUST NOT prove workspace membership.
- An excluded or undeclared nested package is an independent package with its own
  `Cargo.lock` obligation. A workspace member does not own a lockfile.
- The obligation inventory records Go `go.mod` inputs and Rust workspace/package
  `Cargo.lock` inputs, including inputs that are missing or unsupported. It is
  recorded in execution evidence and included in resume identity.
- Every adapters MUST account for every in-scope obligation. Present inputs are
  scanned; missing or explicitly omitted inputs are reported by path. A scan that
  covers only part of the inventory is `FAILED` with `PARTIAL` or `UNKNOWN`
  coverage, never `COMPLETED`/`COMPLETE`. Findings from present inputs remain in
  the report.
- An explicit `inputs` or `lockfile` setting is validated against the inventory.
  It may not name an out-of-scope input. A subset leaves the remaining obligations
  reported unless `security.exclude` removes them from scope; a subset never
  implies full-language coverage.
- Unsupported or ambiguous Cargo layouts (invalid TOML, unsafe or unmatched member
  patterns, nested workspaces, dangling `package.workspace` references) are
  explicit issues and cannot produce clean dependency coverage.
- Detected Go source without `go.mod` is an unsupported dependency input, not an
  empty scan. A genuine repository with no Go or Rust dependency content is
  reported as a completed no-op with `no_applicable_content: true`; no scanner ran
  and selection metadata does not claim language coverage. A completed no-op does
  not satisfy a required dependency category.

### cargo-audit (`dependencies`)

- Invokes the configured argv (default `cargo audit`) with `--file <Cargo.lock>`,
  `--json`, `--no-fetch`, and `--db <local-advisory-database>`. It never runs the
  experimental `fix` subcommand, never generates a lockfile, never fetches or
  updates a database, and never uploads source.
- Resolves the Cargo workspace and its actual `Cargo.lock` through the obligation
  inventory. A workspace member does not own a lockfile; the workspace root does.
  Excluded and undeclared nested packages keep separate lockfile obligations. A
  configured `lockfile` override is validated against the inventory and remains
  repository-relative.
- Parses the structured `vulnerabilities.list` report. Advisory IDs, affected
  package/version, CVSS/severity, aliases/CVEs, patched and unaffected versions,
  references, and the lockfile path are retained. A CVSS 3.x vector is mapped to
  the normalized severity; when neither an explicit severity nor a valid vector
  exists, severity is `UNKNOWN` and the explicit unknown-severity policy applies.
- Records the advisory database identity as a deterministic local fingerprint and
  records scanner-observed metadata (`last-commit`, `last-updated`,
  `advisory-count`) as evidence. The identity also covers local content changes,
  so an edited database cannot reuse earlier evidence.
- Records a lockfile identity: every resolved lockfile path with its content
  digest. Resume re-executes when the advisory database identity, its staleness,
  or the lockfile identity changes.
- Missing lockfiles, missing obligations omitted by an explicit subset,
  unavailable or invalid databases, a report without database identity, malformed
  output, timeouts, and tool failures are non-clean execution results and never
  `PASS`. Each condition has a distinguishable diagnostic naming the input path.
- A stale database (older than 90 days) follows the explicit
  `stale_advisory_database` / adapter `stale` policy. `deny` is the default and
  forces `INDETERMINATE`; `allow` runs with `--stale` and records `PARTIAL`
  coverage, which still cannot satisfy a required gate. A database whose freshness
  cannot be established is treated as stale.
- Dependency audit is workspace-wide. A changed-files scope records that the
  dependency graph was not limited to changed files and marks coverage `PARTIAL`.

Supported versions are pinned by the operator with
`security.adapters[].version`. Before any scan the adapter MUST execute the
configured command with `--version`, parse the reported version, and require an
exact match. A missing pin, an unrecognized version string, or a mismatch is a
non-clean execution failure (a mismatch never starts the scan). Provenance
records the observed version, and resume compares it.

Provisioning is separate from scanning: `scripts/security/fetch-cargo-audit.sh`
downloads the pinned release asset, verifies its committed SHA-256 from
`scripts/security/cargo-audit-manifest.json`, and checks the binary's version. It
is the only step that uses the network. A scan never downloads the tool and never
fetches or updates an advisory database.

### OSV-Scanner (`dependencies`)

- The pinned release is OSV-Scanner v2.7.0. It invokes
  `osv-scanner scan source --format json --offline --all-packages --no-resolve
--no-call-analysis=all --config <controlled-empty> --lockfile <input>` once per
  explicit input. `--offline` disables the live OSV API and database downloads;
  `--no-call-analysis=all` prevents Go/Rust toolchain invocation and therefore
  prevents network module resolution and build output. `--no-resolve` disables
  transitive resolution. `--config` points at an empty restricted file so a
  repository `osv-scanner.toml` cannot suppress SOP findings or reduce coverage.
- Inputs are Go `go.mod` files and present Rust `Cargo.lock` files resolved from
  the obligation inventory. Nested Go modules, ordinary/virtual/implicit-member
  Cargo workspaces, member globs, exclusions, and explicit workspace references
  are resolved from content. A configured `inputs` list is validated against the
  inventory; a subset reports the remaining obligations unless excluded. The
  adapter never generates or modifies a manifest or lockfile. A missing
  `Cargo.lock` or a detected Go source tree without `go.mod` is a failure, never a
  clean result.
- The offline database cache is explicit (`security.osv_database_path`, or the
  adapter `database` override). OSV v2.7 reads
  `<cache>/osv-scalibr/<ecosystem>/all.zip`; the older documented
  `<cache>/osv-scanner/<ecosystem>/all.zip` is also accepted. The ecosystems are
  `Go` and `crates.io`. A missing database is a non-clean execution result.
  The OSV cache is never used as a cargo-audit database.
- Database identity records the ecosystem, content digest, size, and modification
  time; optional acquisition metadata records the source URL and download time.
  A metadata digest that disagrees with the archive fails closed. Freshness uses
  the same 90-day boundary as the dependency-advisory policy. A missing freshness
  signal is treated as stale.
- Each input is scanned independently so one input's failure cannot hide another
  input's coverage. A valid report requires the `results` field, a matching
  source path, and a consistent exit contract: `0` means no vulnerabilities, `1`
  means vulnerabilities, `127`/`128` and other codes are failures. Empty output,
  malformed JSON, truncation, timeouts, and scanner errors are never clean.
- Findings preserve advisory ID and aliases, ecosystem, package, version, input
  path, severity and its evidence (explicit severity, CVSS vector, group maximum),
  fixed versions and affected ranges, references, and scanner provenance. OSV
  alias groups are represented once per package and input. Different packages,
  versions, and inputs are never merged by advisory ID alone. A version match is
  not reachability evidence: call analysis is disabled and the finding records
  `reachability: not-evaluated` unless the scanner supplied explicit analysis.
- Provisioning is explicit: `scripts/security/fetch-osv-scanner.sh` verifies the
  pinned binary SHA-256 from `scripts/security/osv-scanner-manifest.json`, and
  `scripts/security/fetch-osv-databases.sh` downloads and integrity-checks the
  `Go` and `crates.io` archives and records their source, digest, and acquisition
  time. Scans never download or refresh either input.

## 5. Execution and evidence

1. Validate configuration and requested categories.
2. Resolve adapters and inspect descriptors. With auto selection, detect
   languages, manifests, Cargo workspaces, and actual/missing lockfiles first;
   auto mode only adds missing built-in categories and explicit configuration
   remains authoritative.
3. Confirm the workspace root and compute content identity.
4. Execute each adapter once with a bounded deadline and output limit.
5. Sanitize native reports before persistence or model access.
6. Normalize findings and compute coverage.
7. Apply baseline/suppression policy.
8. Evaluate PASS/FAIL/INDETERMINATE/NOT_APPLICABLE.
9. Persist artifacts under `.agent-sdlc/runs/<id>/security/`.
10. Continue through the existing run lifecycle; no independent continuation
    loop is created.

A clean tree is identified by commit and tree. A dirty tree additionally records
a content digest and `dirty: true`. A non-Git workspace records a deterministic
file-tree digest and is treated as dirty because no commit identity exists.

Changed-file scope is an optimization. It records its file list and limitations.
A required full-scan policy cannot pass using changed-file evidence: the verdict
is `INDETERMINATE`.

Artifacts:

- `security/scan.json` — normalized evidence and verdict. `scope.excluded`
  records the declared exclusions. Auto-mode scans also include `detection`;
  every scan includes `required_mode` readiness evidence. `coverage_by_category`
  reports each category and detected language independently, and `overlaps`
  records visible explicit overlapping coverage. Disabled adapters appear as
  `SKIPPED` executions with `disabled: true` and a reason.
- `security/scan.md` — human-readable rendering.
- `security/<adapter>.native.json` — sanitized native report, when available.

Raw reports containing secret values MUST NOT enter ordinary run artifacts.
Redaction failure makes the scan result unusable.

## 6. Policy verdicts and the SOP gate

- `NOT_APPLICABLE`: add-on intentionally disabled and no required gate is active.
- `PASS`: every enabled adapter completed with complete coverage and no
  unsuppressed blocking finding.
- `FAIL`: at least one unsuppressed finding is at a `fail_on` severity (or the
  explicit unknown-severity policy says fail).
- `INDETERMINATE`: an adapter failed, timed out, was unavailable, was cancelled,
  had partial/unknown coverage, a required category or required language was not
  covered, a required category used changed-file evidence, explicitly disabled
  coverage was not replaced, or a dependency adapter observed a stale advisory
  database (under either stale policy).

A dependency adapter's unsupported language, missing input, unavailable database,
stale database, or undeclared external language coverage is never treated as
clean evidence. Required mode evaluates every detected language independently; a
successful Go adapter cannot satisfy a missing Rust obligation, and cargo-audit
cannot satisfy Go dependency coverage. Global `enabled: false` does not exempt an
active required gate: the stage reports `INDETERMINATE`, and the quality gate
fails closed. Removing a required category is a policy change, not a scan result.

The `required_mode` field projects what the same evidence would produce under a
required gate while the recorded verdict remains report-only.

A blocking finding produces `FAIL` even when another adapter is incomplete or
coverage is partial; the coverage state remains explicit. An incomplete scan
with no blocking finding produces `INDETERMINATE` and never `PASS`.

Report-only records the verdict as an advisory and never blocks the run.
Required mode fails closed: only `PASS` passes. `quality.Evaluate` remains the
single verdict authority; the security layer cannot approve, suppress, or bypass
a gate. A required security failure is classified as `SECURITY_SCAN_FAILURE` in
the existing failure model, with a terminal operator-review disposition. It does
not enter the code-fix loop and does not create an approval gate; SOP never
automatically remediates security findings. Approval boundaries still take
precedence over the security classification.

## 7. Baselines and suppressions

Baseline file format (version 1):

```json
{
  "version": 1,
  "entries": [
    {
      "fingerprint": "semgrep:<stable-id>",
      "rule_id": "optional-rule",
      "path": "optional/relative/path",
      "reason": "reviewed",
      "owner": "team-name",
      "expires_at": "2027-01-01T00:00:00Z"
    }
  ]
}
```

- Fingerprint matching is exact, so a new finding cannot be hidden by an
  unrelated entry.
- `reason`, `owner`, and a future `expires_at` are required.
- Existing findings remain visible and are marked `suppressed`.
- An expired entry is visible with an `expired` marker and is no longer
  effective.
- Invalid entries are ignored and reported.
- Automatic baseline creation is not part of the MVP.

## 8. Resume

`--resume <run-id>` loads `.agent-sdlc/runs/<run-id>/security/scan.json`. A
completed adapter result is reused only when all of these still match:

- content identity (commit/tree/digest/dirty),
- adapter ID, selection mode, and declared language scope,
- pinned scanner version,
- rules or controlled-config identity,
- advisory database identity and rechecked stale state for a dependency adapter,
- resolved obligation identity, including present/missing/unsupported inputs,
- resolved input/lockfile identity,
- adapter configuration digest, including enable and language metadata,
- policy identity, including baseline content, the configured execution bounds
  (timeout and output limit), the advisory database paths, and the stale policy.

Any mismatch re-executes that adapter. A prior non-completed or disabled result is
never reused. Resume does not change retry or continuation budgets.

## 9. Security boundaries

- Approved executables are invoked directly with argument arrays.
- Scanned paths are resolved within the workspace root; report paths that are
  absolute, escape the root, or contain NUL are rejected.
- Report size and individual diagnostic/message lengths are bounded.
- Report text is untrusted data. It is sanitized, never executed, and never
  becomes an instruction or workflow authority.
- Detected secrets are absent from normalized findings, persisted artifacts,
  diagnostics, prompts, and telemetry. Native reports use `--redact` and a
  second structural sanitization pass.
- Credentials are not part of the configuration schema and are not passed to
  adapters.
- The add-on does not claim sandbox isolation. Isolation is a property of the
  runtime, not this module.

## 10. License and provisioning decisions

- SOP remains Apache-2.0. The add-on invokes external CLIs; it does not vendor
  scanner engines or rule packs.
- Semgrep Community Edition is distributed under LGPL-2.1; rules and hosted
  services have their own terms. Gitleaks is distributed under MIT. Verify exact
  engine, rule, action, and redistribution terms against the pinned upstream
  releases before packaging.
- Free CLI access does not imply free hosted AI features.
- Scanner installation, rule downloads, and hosted uploads are explicit
  operator actions. The add-on performs none of them.

## 11. MVP exclusions

Verified-secret services, hosted code analysis, container scanning, runtime
network/API scanning, asset discovery, automatic fixes, automatic scanner
installation, cross-provider deduplication, a new vulnerability database, and a
scanner engine are excluded.

## 12. Meaningful validation commands

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

The last command provisions the pinned real CLIs and the pinned production rules,
including the pinned OSV-Scanner binary, and runs the tagged integration checks.
It sets `SOP_SECURITY_INTEGRATION_REQUIRED=1`, so missing prerequisites fail the
check instead of skipping it, and it exports `SOP_SECURITY_RULES_DIR` so the suite
verifies the pinned ruleset actually covers Rust. The OSV real-CLI tests use a
controlled offline database fixture and assert clean and vulnerable Go/Rust
results, unchanged inputs, missing-ecosystem behavior, and that a repository
`osv-scanner.toml` cannot suppress SOP findings. CI runs the same script.

Real Semgrep/Gitleaks execution requires the pinned CLIs and local rules in the
supported integration environment. When a tool is absent, the adapter reports
`UNAVAILABLE` or `FAILED` and the verdict is `INDETERMINATE`; the skipped local
check MUST be reported explicitly.
