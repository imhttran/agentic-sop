---
name: sop-security-scan
description: |-
  Run a report-only security scan for Go, Rust, or mixed repositories through
  SOP. Detects languages, manifests, Cargo workspaces and lockfiles, selects
  Gitleaks, Semgrep, and cargo-audit by actual repository content, triages
  findings against source and dependency evidence, and saves sanitized evidence.
  /sop-security-scan, "security scan", "scan this repo for secrets and
  vulnerabilities", "audit Rust dependencies".
disable-model-invocation: true
---

# SOP Security Scan

Use this skill when the operator asks for a security scan, a secrets scan, a
source-analysis scan, or a Rust dependency audit. It orchestrates SOP's existing
security adapters. Scanner execution stays in the adapters: this skill never
invokes a scanner directly, installs a tool, suppresses a finding, or fixes code.

SOP state and evidence are authoritative. The skill reads SOP output and source
files, then reports what actually ran. Complete the workflow in one request and
stop only at a real blocker or an existing approval boundary.

## PREFLIGHT

1. Confirm `sop` is on `PATH`. If it is missing, STOP and report the installation
   prerequisite. Do not substitute direct scanner commands or answer from memory.
2. Identify the repository root and read its instructions (`AGENTS.md`,
   `CLAUDE.md`, contributing and security policy files). Preserve unrelated
   changes.
3. Read the existing SOP security configuration without modifying it. If the
   security block is absent or disabled, that is not an error; report it.
4. Inspect current approval state. Do not approve, decline, commit, or merge.

```bash
sop security inspect --json
sop approvals --json
```

## DETECT

Use `sop security inspect --json`. It is read-only and reports:

- detected languages (`go`, `rust`), manifests, and lockfiles;
- Cargo workspaces and the actual `Cargo.lock` locations;
- configured adapters and their capability status;
- proposed adapters for the detected content.

Classify each capability as one of:

- `available`: the category is configured, the scanner resolves, and the
  content/rules/database support it;
- `unavailable`: the scanner executable is not present;
- `unsupported`: the detected content cannot be covered, for example Rust source
  with only Go Semgrep rules, a missing lockfile, or a missing advisory database;
- `proposed`: content suggests an adapter that is not configured yet.

A missing lockfile or unsupported language is a coverage limitation, not a clean
result. Report it explicitly.

## CONFIGURE (REPORT-ONLY ONLY)

Keep the project security block in report-only mode. Configure only what the
repository actually needs:

- Gitleaks for secrets;
- Semgrep for source analysis with local, language-specific rules;
- cargo-audit for Rust dependency advisories.

Use the existing configuration keys; do not invent adapters or change scanner
policy. A local rules/database path is always explicit. Never let a scan download
rules or update an advisory database. Never install a scanner unless the operator
authorized it or an existing provisioning policy already provides it.

A Rust repository needs an explicit local advisory database path for cargo-audit.
If no database is configured, report cargo-audit as unsupported; do not fall back
to an implicit fetch or a live network database.

Example report-only block:

```yaml
security:
  enabled: true
  mode: report-only
  scope: workspace
  unknown_severity: indeterminate
  advisory_database_path: .agent-sdlc/advisory-db
  stale_advisory_database: deny
  adapters:
    - id: semgrep
      category: sast
      protocol: builtin
      command: [semgrep]
      rules: .agent-sdlc/security-rules
    - id: gitleaks
      category: secrets
      protocol: builtin
      command: [gitleaks]
    - id: cargo-audit
      category: dependencies
      protocol: builtin
      command: [cargo, audit]
```

Do not add `required_categories`, enable required mode, create a baseline, or add
owners/expiry dates merely to make a scan pass.

## SELECT

Select by actual repository content, not by convenience:

- Go content: Gitleaks and Semgrep with Go rules.
- Rust content: Gitleaks, Semgrep with Rust rules when available, and cargo-audit
  for dependency advisories.
- Mixed content: the union of the above. Coverage stays independent per language
  and category.

If Rust rules are not available, Semgrep must report unsupported Rust analysis.
Never reuse Go rules for Rust source. If cargo-audit is unavailable or has no
database, report that; do not replace it with a clean dependency result.

## SCAN

Run one governed, report-only scan through SOP. Use auto selection only to fill
missing built-in categories from detected content; explicit configuration stays
authoritative.

```bash
sop security scan --auto --report-only --json
```

If the scan reports a blocked prerequisite or approval boundary, STOP and report
it. Do not run scanners around SOP, and do not change shell authorization or
approval policy to make the command succeed.

## TRIAGE

Read the normalized evidence and the human report:

- `.agent-sdlc/runs/<run-id>/security/scan.json`
- `.agent-sdlc/runs/<run-id>/security/scan.md`
- `.agent-sdlc/runs/<run-id>/security/<adapter>.native.json` (sanitized)

For each finding, compare it with the current source and dependency evidence:

- source findings: read the named path and line range; state whether the rule is
  supported and whether the evidence still matches the current content;
- dependency findings: verify the advisory ID, affected package and version,
  lockfile, patched version, and CVSS/severity evidence;
- execution failures, missing tools, missing lockfiles, unavailable databases,
  stale databases, and unsupported languages are findings about coverage. They
  must not be reported as clean;
- distinguish advisories from tool/database failures.

Do not quote a detected secret value. Refer only to the rule ID, path, line,
fingerprint, and non-secret metadata. Do not mark a finding as accepted, false
positive, or suppressed unless a reviewed, unexpired baseline already says so.
Do not create a baseline or invent an owner or expiry date. Do not fix code.

## REPORT

Return a concise, evidence-backed report containing:

- repository and detected languages/manifests;
- Cargo workspaces and actual lockfiles, including missing ones;
- adapters that ran, their status and coverage, and the exact scanner, rules or
  advisory-database identity, configuration, and policy identity from the
  evidence;
- findings grouped by category with source/dependency evidence;
- declared exclusions and scope limitations;
- execution failures and unsupported capabilities;
- dispositions: unreviewed, coverage limitation, or already-reviewed baseline
  suppression (never a new risk acceptance);
- required-mode readiness from the scan's `required_mode` projection. A passing
  report-only scan is not automatically a passing required gate;
- artifact paths and the scan run ID;
- remaining gaps and the next operator action, if any.

A passing scan is evidence for the configured tools, rules/database, and scope.
It is not proof that the repository has no vulnerabilities.

## RUST REQUIREMENTS

- Resolve the Cargo workspace and its actual `Cargo.lock`; do not assume a member
  owns a lockfile.
- Never run cargo-audit without an explicit lockfile. Do not let it generate or
  modify a lockfile.
- Do not run a database fetch or update. Network access is explicit: cargo-audit
  uses `--no-fetch` and a local advisory database.
- Parse cargo-audit structured output. Separate advisories from missing database,
  stale database, malformed output, timeout, and tool failure.
- Preserve advisory IDs, affected packages/versions, severity when available,
  CVSS, aliases/CVEs, patched versions, and remediation references.
- Missing lockfiles, unavailable databases, stale databases under the deny policy,
  and failed executions are not clean. Include the advisory database identity in
  evidence and resume reasoning.
- If Rust Semgrep rules are unavailable, report source analysis as unsupported
  instead of implying full Rust SAST coverage.

## BOUNDARIES

- Keep the project configuration report-only.
- Do not install tools, download rules, fetch or update databases, or upload
  source.
- Do not suppress findings, invent owners or expiry dates, accept risks, or enable
  required mode automatically.
- Do not launch automatic fixes from the scan skill.
- Do not alter shell authorization or approval policy.
- Keep detected secrets out of artifacts, logs, diagnostics, and model inputs.
- Never merge, delete branches, or delete worktrees.

## EXTENSION

A private scanner remains an external `security-scan/v1` executable:
configuration selects it, SOP passes a bounded JSON request on standard input, and
the response is validated before use. A new built-in language or category belongs
in the adapter layer, not in this skill. The skill only reads detection and
evidence; it does not execute or interpret a private adapter itself.
