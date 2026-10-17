# PLAN — Security Scanner Add-on

**Status:** ACTIVE. Requirements source:
[`docs/specs/SOP-Security-Scanner-Addon-PRD.md`](../specs/SOP-Security-Scanner-Addon-PRD.md).
**Rule:** the add-on is disabled unless configured. A failed, unavailable, timed-out,
malformed, or partial scan MUST NOT produce a passing required security gate.

## Project

agentic-sop

## Summary

Add an optional, report-only-by-default security scanning add-on that reuses SOP's
existing execution controls rather than introducing a second runner or plugin
framework. The MVP wraps two external scanner CLIs: Semgrep Community Edition for
source analysis (SAST) and Gitleaks for secret detection. SOP owns orchestration,
normalization, evidence identity, redaction, baseline/suppression handling, and the
optional blocking gate.

The implementation reuses `.agent-sdlc/config.yaml` (strict configuration),
`internal/cli` command registration, `.agent-sdlc/runs/<id>/` artifact storage,
`internal/git` for content identity, `internal/quality` for the single verdict, and
the existing bounded run/retry/continuation behavior. It does not add a mandatory
service, model call, scanner engine, automatic install, or automatic fix path.

## Capabilities

### Existing SOP configuration and CLI
- **Status:** EXISTS
- **Location:** `internal/config/config.go`, `internal/cli/cli.go`
- **Evidence:** strict YAML decoding with `KnownFields(true)`, a single command
  dispatcher, and an opt-in configuration convention used by `verification`,
  `early_jev`, `rag`, and other optional blocks.

### Existing run artifact storage
- **Status:** EXISTS
- **Location:** `internal/run/run.go`, `internal/cli/drive.go`
- **Evidence:** `run.New`/`run.Open` create `.agent-sdlc/runs/<id>/`, `Run.Write`
  stores inspectable artifacts, and `report.md`/`report.json` are the existing
  run-level report surfaces.

### Existing deterministic quality gate
- **Status:** EXISTS
- **Location:** `internal/quality/quality.go`, `internal/cli/run.go`
- **Evidence:** `quality.Evaluate` is the single verdict authority and already
  folds optional evidence (JEV, PostgreSQL validation) into PASS/FAIL/NEEDS_HUMAN.

### Existing bounded execution controls
- **Status:** EXISTS
- **Location:** `internal/testrunner/command.go`, `internal/decision/command/command.go`,
  `internal/ragclient/exec.go`, `internal/run/run.go`
- **Evidence:** direct argv process execution, context deadlines, bounded output,
  and persisted attempt/continuation counters already exist.

### Semgrep Community Edition and Gitleaks CLIs
- **Status:** EXISTS
- **Evidence:** `scripts/checks/security-integration.sh` provisions pinned
  Semgrep CE `1.136.0` and Gitleaks `8.30.1` in an isolated tools directory and
  runs the tagged real-CLI checks with `SOP_SECURITY_INTEGRATION_REQUIRED=1`.

## Assumptions

### The adapter contract can reuse the existing external-process pattern
- **Evidence:** `internal/ragclient` and `internal/decision/command` already define
  versioned JSON request/response subprocess boundaries with bounded output.
- **Consequence:** the add-on adds a security-specific contract, not a plugin
  framework or dynamic loader.

### Scanner output is untrusted input
- **Evidence:** the PRD requires bounded reports, safe paths, and non-executable
  report text; the repository already treats provider and review output as data.
- **Consequence:** reports are parsed, sanitized, and never executed or used as
  prompt authority.

## SEC-001 — Discover and define the adapter contract

Inspect the runner hooks, adapter/extension patterns, policy gates, artifact storage,
and tests listed in the capability inventory. Define the versioned security adapter
protocol (`security-scan/v1`), its request/response objects, execution statuses,
coverage states, policy verdicts, and state transitions. Record exact integration
paths, the supported Semgrep CE and Gitleaks version policy, local-rules/offline
behavior, license decisions, and the meaningful validation commands. Make no claim
about current behavior that was not inspected.

### Dependencies

None

### Deliverables

- `docs/specs/SECURITY-SCANNER-ADDON.md` — normative contract, state transitions,
  integration paths, version policy, and license decisions.
- `docs/guides/SECURITY-SCANNER-ADDON.md` — setup, adapter extension, report-only
  rollout, and rollback.
- `docs/plans/PLAN-Security-Scanner-Addon.md` — this executable plan.

### Acceptance Criteria

- The contract names the exact SOP integration paths: configuration, CLI, run
  artifacts, quality gate, recovery/continuation, and Git identity.
- Protocol v1 defines Descriptor, Request, Execution, Finding, and Provenance
  minimum fields, and separates execution status from policy verdict.
- The document states that Semgrep CE and Gitleaks are pinned external CLIs, that
  rule downloads and hosted uploads are explicit, and that no scanner engine is
  built.
- Every validation command named in the document exists in the repository.
- No unsupported claim is made about tool availability or current code.

### Acceptance Checks

- `test -f docs/specs/SECURITY-SCANNER-ADDON.md`
- `test -f docs/guides/SECURITY-SCANNER-ADDON.md`
- `grep -q 'security-scan/v1' docs/specs/SECURITY-SCANNER-ADDON.md`

### Execution

- implement

## SEC-002 — Build the execution layer

Add the opt-in `security` configuration block, descriptor and response validation,
direct-argv process execution, deadline/cancellation handling, bounded output,
content identity, and artifact persistence. Keep the add-on disabled unless
configured and keep disabled runs byte-identical to the previous behavior. Prove
that a replacement adapter runs through configuration alone and that failure,
timeout, missing-binary, malformed-output, oversized-output, and partial-coverage
cases remain distinct from a clean result.

### Dependencies

- SEC-001

### Deliverables

- `internal/config/config.go` — optional `security` block, defaults, and validation.
- `internal/security/contract.go` — protocol v1 descriptors, request/execution
  types, statuses, coverage, and verdicts.
- `internal/security/runner.go` — bounded orchestration over multiple adapters.
- `internal/security/identity.go` — commit, tree, and dirty-content identity.
- `internal/security/exec.go` — direct-argv, context-aware, bounded subprocess
  execution.
- `internal/security/artifacts.go` — sanitized scan artifacts under
  `.agent-sdlc/runs/<id>/security/`.
- `internal/security/*_test.go` — replacement, failure, timeout, cancellation,
  malformed, oversized, partial-coverage, and disabled-mode tests.
- `internal/config/config_test.go` — security configuration tests.

### Acceptance Criteria

- With no `security` block and no `SOP_SECURITY_*` configuration, every run and
  configuration load behaves exactly as before.
- An operator can replace an adapter by changing `security.adapters` only; no
  runner code changes are needed.
- Multiple adapters configured for one category each retain an independent
  execution result and sanitized report.
- Missing binary maps to `UNAVAILABLE`, deadline to `TIMED_OUT`, cancellation to
  `CANCELLED`, malformed report to `FAILED`, and oversized output to `FAILED`;
  none maps to `COMPLETED`/`PASS`.
- Partial coverage is reported as `PARTIAL`, independently of execution status.
- Evidence records the commit and tree identity, plus a content digest and
  dirty-state marker when the working tree is dirty.

### Acceptance Checks

- `go test ./internal/security/...`
- `go test ./internal/config/...`
- `go build ./...`

### Execution

- implement

## SEC-003 — Add the Semgrep CE and Gitleaks adapters

Wrap pinned Semgrep Community Edition and Gitleaks CLI versions behind the protocol
v1 adapter interface. Invoke approved executables with argument arrays and no shell.
Parse Semgrep JSON and Gitleaks JSON into normalized findings, map provider
severities without discarding provider meaning, produce stable non-secret
fingerprints, and sanitize both normalized evidence and retained native reports.
Keep rule downloads, hosted uploads, and automatic installation explicit.

### Dependencies

- SEC-002

### Deliverables

- `internal/security/semgrep.go` — Semgrep CE adapter.
- `internal/security/gitleaks.go` — Gitleaks adapter.
- `internal/security/normalize.go` — severity mapping, safe paths, fingerprinting,
  and untrusted-text bounds.
- `internal/security/redact.go` — secret-safe native-report sanitization.
- `internal/security/testdata/` — synthetic Semgrep/Gitleaks reports, local
  Semgrep rules, an optional local Gitleaks config, and seeded insecure-code and
  secret fixtures.
- `internal/security/adapters_test.go` — conformance, normalization, fixture, and
  redaction tests.
- `internal/security/integration_test.go` (`security_integration` tag) — real
  pinned-CLI clean/seeded, coverage, severity, redaction, and resume checks.

### Acceptance Criteria

- Semgrep detects a known supported insecure-code fixture when the real CLI is
  available; the deterministic fixture test proves the adapter parses and maps the
  same report shape without a live binary.
- Gitleaks detects a synthetic secret fixture when the real CLI is available; its
  fixture test proves the adapter parses and redacts the same report shape.
- Clean fixture reports produce no unexpected findings.
- Both built-in adapters satisfy the same protocol conformance checks as an
  external adapter.
- Seeded secret values are absent from persisted artifacts, captured diagnostics,
  and normalized findings.
- Rule identity and scanner/configuration identities are recorded in provenance.

### Acceptance Checks

- `go test ./internal/security/...`
- `go vet ./internal/security/...`
- `bash scripts/checks/security-integration.sh`

### Execution

- implement

## SEC-004 — Integrate policy, baselines, resume, and the SOP gate

Connect report-only and required modes to SOP's existing deterministic quality gate.
Add reviewed baseline/suppression handling where existing findings stay visible and
a suppression requires a reason, owner, and expiry. Add bounded resume that reuses a
completed result only when content, scanner, rules, configuration, and policy
identities still match. Preserve existing approval boundaries, retry budgets,
continuation counters, and cancellation behavior.

### Dependencies

- SEC-003

### Deliverables

- `internal/security/policy.go` — verdict and coverage evaluation matrix.
- `internal/security/baseline.go` — reviewed baseline/suppression loading and expiry.
- `internal/security/resume.go` — identity-checked result reuse.
- `internal/security/report.go` — normalized JSON and Markdown report rendering.
- `internal/cli/security.go` — `sop security scan` command and artifacts.
- `internal/cli/cli.go` — command registration and help.
- `internal/cli/run.go` — optional security evidence at the existing quality seam.
- `internal/quality/quality.go` — optional security evidence in the single verdict.
- `internal/security/*_test.go`, `internal/quality/*_test.go`,
  `internal/cli/security_test.go` — gate matrix, baseline, expiry, stale-resume,
  report-only, required, disabled, and rollback tests.
- `internal/cli/security_integration_test.go` (`security_integration` tag) — real
  `sop security scan` and `sop run` report-only/required checks.

### Acceptance Criteria

- Report-only records PASS/FAIL/INDETERMINATE/NOT_APPLICABLE without blocking the
  run; required mode blocks on FAIL or INDETERMINATE.
- The full severity, coverage, and execution-status matrix passes; an unknown
  severity follows an explicit policy and never silently becomes low.
- A required category is not satisfied by an adapter from a different category.
- Baseline and suppression entries remain visible, never hide new findings, and an
  expired suppression is reported and no longer effective.
- Resume reuses only a completed result whose content, scanner, rules,
  configuration, and policy identities all still match; any mismatch reruns.
- Existing approval, retry, continuation, and disabled-mode behavior is unchanged.
- A failed or incomplete required scan never produces a passing gate.

### Acceptance Checks

- `go test ./internal/security/...`
- `go test ./internal/quality/...`
- `go test ./internal/cli/...`
- `go build ./...`

### Execution

- implement

## SEC-005 — Dogfood in report-only mode, document, and prepare the PR

Run the add-on against agentic-sop in report-only mode, record the scanned revision,
content identity, coverage, tool availability, actual timings, and findings. Document
setup, adapter extension, limitations, and rollback. Keep the change focused and
prepare a PR without merging it.

### Dependencies

- SEC-004

### Deliverables

- `docs/reports/SECURITY-SCANNER-DOGFOOD.md` — report-only evidence, timings,
  availability, findings, and limitations.
- `docs/specs/SECURITY-SCANNER-ADDON.md`, `docs/guides/SECURITY-SCANNER-ADDON.md` —
  final documentation.
- `docs/README.md`, `docs/reference/CONFIGURATION.md`, `docs/reference/CLI.md` —
  index and reference links.
- A focused branch/PR containing only intended changes.

### Acceptance Criteria

- The dogfood evidence identifies the scanned commit, tree/content identity, scope,
  coverage, execution statuses, actual scanner versions, rules identity, and
  actual timings.
- Missing tools and skipped local checks are reported explicitly; no unavailable
  scanner is described as clean.
- Repository-required checks pass: formatting, vet, build, test, documentation
  links, and the pinned real-CLI integration script.
- The PR contains only intended changes and is not merged.
- Rollback is documented as disabling the add-on through configuration and leaving
  non-security verification unchanged.

### Acceptance Checks

- `go test ./...`
- `go build ./...`
- `go vet ./...`
- `bash scripts/checks/check-doc-links.sh`
- `go test ./internal/planner/... -run TestSecurityScannerPlanCompiles`
- `bash scripts/checks/security-integration.sh`

### Execution

- implement
