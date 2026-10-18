# Security Scanner Add-on

**Type:** Normative specification (MVP)
**Status:** Implemented for Semgrep Community Edition, Gitleaks, and cargo-audit.
**Requirements source:** [SOP-Security-Scanner-Addon-PRD.md](SOP-Security-Scanner-Addon-PRD.md)
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
  # Local advisory database for dependency scanning. Explicit; never fetched implicitly.
  advisory_database_path: .agent-sdlc/advisory-db
  stale_advisory_database: deny # deny (default) | allow
  auto: false # true adds missing built-in categories from detected content
  adapters:
    - id: semgrep
      category: sast
      protocol: builtin
      command: [semgrep]
      rules: .agent-sdlc/security-rules # local rules path; no implicit download
      version: "SET_THE_PINNED_VERSION"
    - id: gitleaks
      category: secrets
      protocol: builtin
      command: [gitleaks]
      version: "SET_THE_PINNED_VERSION"
    - id: cargo-audit
      category: dependencies
      protocol: builtin
      command: [cargo, audit]
      database: .agent-sdlc/advisory-db # optional per-adapter override
      lockfile: Cargo.lock # optional; otherwise resolved from content
      stale: deny # optional per-adapter override
      version: "SET_THE_PINNED_VERSION"
    # A private adapter replaces a built-in through configuration alone.
    # - id: private-sast
    #   category: sast
    #   protocol: external
    #   command: [/absolute/path/to/adapter]
    #   version: "1.0.0"
```

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

| Field                           | Meaning                                          |
| ------------------------------- | ------------------------------------------------ |
| `protocol_version`              | `security-scan/v1`                               |
| `adapter_id`, `adapter_version` | Stable identity and pinned version               |
| `categories`                    | `sast`, `secrets`, `dependencies`                |
| `scope_modes`                   | `workspace`, `changed-files`                     |
| `locality`, `network`           | Execution locality and data/network requirements |

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
skipped files/reasons, diagnostics, normalized findings, and provenance.

Execution statuses are `COMPLETED`, `FAILED`, `TIMED_OUT`, `UNAVAILABLE`,
`SKIPPED`, and `CANCELLED`. Coverage is `COMPLETE`, `PARTIAL`, or `UNKNOWN`,
reported independently of execution status.

### Finding

A finding preserves provider, category, rule ID, original severity, normalized
severity, bounded message, safe relative location, stable non-secret
fingerprint, optional CWE/CVE/confidence, and sanitized provider metadata.
Provider identity is part of the fingerprint namespace. Cross-provider
deduplication is deferred; every source remains represented.

### Provenance

Provenance records scanner version, rules identity, configuration digest,
content identity, policy identity, and sanitized native-report digest.

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

### cargo-audit (`dependencies`)

- Invokes the configured argv (default `cargo audit`) with `--file <Cargo.lock>`,
  `--json`, `--no-fetch`, and `--db <local-advisory-database>`. It never runs the
  experimental `fix` subcommand, never generates a lockfile, never fetches or
  updates a database, and never uploads source.
- Resolves the Cargo workspace and its actual `Cargo.lock`. A workspace member
  does not own a lockfile; the workspace root does. A configured `lockfile`
  override is allowed but remains repository-relative.
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
- Missing lockfiles, unavailable or invalid databases, a report without database
  identity, malformed output, timeouts, and tool failures are non-clean execution
  results and never `PASS`. Each condition has a distinguishable diagnostic.
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
  every scan includes `required_mode` readiness evidence.
- `security/scan.md` — human-readable rendering.
- `security/<adapter>.native.json` — sanitized native report, when available.

Raw reports containing secret values MUST NOT enter ordinary run artifacts.
Redaction failure makes the scan result unusable.

## 6. Policy verdicts and the SOP gate

- `NOT_APPLICABLE`: add-on intentionally disabled.
- `PASS`: every configured adapter completed with complete coverage and no
  unsuppressed blocking finding.
- `FAIL`: at least one unsuppressed finding is at a `fail_on` severity (or the
  explicit unknown-severity policy says fail).
- `INDETERMINATE`: an adapter failed, timed out, was unavailable, was cancelled,
  had partial/unknown coverage, a required category was not covered, a required
  category used changed-file evidence, or a dependency adapter observed a stale
  advisory database (under either stale policy).

A dependency adapter's unsupported language, missing lockfile, unavailable
advisory database, or stale database is never treated as clean evidence. The
`required_mode` field projects what the same evidence would produce under a
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
- adapter ID and pinned scanner version,
- rules identity,
- advisory database identity and stale state for a dependency adapter,
- adapter configuration digest,
- policy identity, including baseline content, the configured execution bounds
  (timeout and output limit), the advisory database path, and the stale policy.

Any mismatch re-executes that adapter. A prior non-completed result is never
reused. Resume does not change retry or continuation budgets.

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

Verified-secret services, hosted code analysis, runtime network scanning, asset
discovery, automatic fixes, automatic scanner installation, cross-provider
deduplication, a new vulnerability database, and a scanner engine are excluded.

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

The last command provisions the pinned real CLIs and the pinned production rules
and runs the tagged integration checks. It sets `SOP_SECURITY_INTEGRATION_REQUIRED=1`,
so missing prerequisites fail the check instead of skipping it, and it exports
`SOP_SECURITY_RULES_DIR` so the suite verifies the pinned ruleset actually covers
Rust. CI runs the same script.

Real Semgrep/Gitleaks execution requires the pinned CLIs and local rules in the
supported integration environment. When a tool is absent, the adapter reports
`UNAVAILABLE` or `FAILED` and the verdict is `INDETERMINATE`; the skipped local
check MUST be reported explicitly.
