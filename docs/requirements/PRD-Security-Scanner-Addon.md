# SOP Security Scanner Add-on

Status: Implemented • Version: 0.1 • Date: 2026-10-17

**Normative behavior:** [SECURITY-SCANNER-ADDON.md](../specs/SECURITY-SCANNER-ADDON.md).
This PRD is retained as the requirements source; where it and the specification
differ, the specification owns current behavior.

## 1. Problem and outcome

SOP needs a repeatable way to collect security evidence during software delivery. Teams use different scanners, budgets, and data policies. Binding the runner to one scanner would limit adoption and increase maintenance.

Add an optional security scanning module with replaceable adapters. Reuse existing scanner CLIs. SOP controls orchestration, evidence, and policy. Each scanner performs its own analysis.

Success means a user can select scanners by category, run them through SOP, and receive a reproducible result without manual continuation. A failed or incomplete scan must never produce a passing security gate.

## 2. Users and use cases

- A developer runs local code and secret scans before submitting a change.
- A maintainer enables a required security gate for a repository.
- A team replaces a scanner without changing the SOP runner.
- An organization adds a private adapter while retaining its own credentials and data policy.

## 3. Scope and priorities

| Priority | Capability | Decision |
| --- | --- | --- |
| MVP | Source analysis / SAST | Semgrep Community Edition CLI adapter |
| MVP | Secret detection | Gitleaks CLI adapter |
| MVP | Extension contract | Versioned external process protocol and adapter conformance tests |
| MVP | Evidence and policy | Normalized reports, provenance, baselines, optional blocking gate |
| Later | Dependency scanning | Evaluate Snyk Open Source and ecosystem-specific tools |
| Later | Alternative secret scanning | TruffleHog; declare active verification explicitly |
| Later | Hosted code analysis | Snyk Code, Corgea, Xygeni after API, licensing, and data review |
| Later | Runtime application scanning | ZAP and Nuclei with explicit targets and scan profiles |
| Later | Asset discovery | Nmap in a separate runtime/asset capability |

Scanners are interchangeable within a supported category. A secret scanner does not satisfy a SAST requirement. Several adapters can run in one security stage.

MVP excludes automatic fixes, automatic scanner installation, mandatory hosted services, active network scanning, and a new vulnerability database. Do not build a scanner engine.

## 4. Product requirements

| ID | Requirement | Acceptance evidence |
| --- | --- | --- |
| SEC-R01 | Keep the add-on disabled unless configured. | Existing runs retain their behavior when disabled. |
| SEC-R02 | Select adapters by category and stable adapter ID. | Replace an adapter through configuration without editing runner code. |
| SEC-R03 | Support multiple adapters in a stage. | Each adapter retains an independent execution result and report. |
| SEC-R04 | Separate execution status from policy verdict. | Timeout, malformed output, missing binary, and partial coverage cannot yield PASS for a required scan. |
| SEC-R05 | Record the exact scanned content. | Evidence includes commit and tree identity; dirty input includes a content digest and dirty-state marker. |
| SEC-R06 | Normalize findings without discarding provider meaning. | Preserve rule ID, original severity, normalized severity, location, and provider metadata. |
| SEC-R07 | Protect credentials and detected secrets. | Seeded secret values are absent from persisted artifacts, logs, prompts, and telemetry. |
| SEC-R08 | Reuse existing SOP execution controls. | Cancellation, deadlines, approvals, retries, and task state follow existing contracts. |
| SEC-R09 | Make policy reproducible. | Store policy, rules, adapter, scanner, and configuration identities with results. |
| SEC-R10 | Support reviewed baselines and suppressions. | Existing findings remain visible; suppression requires a reason, owner, and expiry. |
| SEC-R11 | Keep AI triage advisory. | AI cannot silently suppress findings, change gate policy, or authorize fixes. |
| SEC-R12 | Treat reports as untrusted input. | Bound report size, reject unsafe paths, and prevent report text from becoming executable instructions. |

## 5. Adapter architecture

Expose one versioned adapter contract. Use executable arguments rather than shell command interpolation. Reuse the existing extension mechanism if repository discovery finds a suitable one. Do not introduce a second plugin framework without a documented gap.

The external process protocol should accept a JSON request and return a JSON response. Keep diagnostics separate from protocol output. Built-in adapters wrap scanner CLI output and satisfy the same contract as third-party adapters.

| Contract object | Minimum fields |
| --- | --- |
| Descriptor | Protocol version, adapter ID/version, categories, supported scope modes, execution locality, network/data requirements |
| Request | Run/task/attempt IDs, workspace root, content identity, scope, scanner configuration, deadline, output location |
| Execution | Status, exit code, start/end time, coverage, skipped files/reasons, diagnostics |
| Finding | Provider, category, rule ID, original and normalized severity, message, relative location, fingerprint, optional CWE/CVE and confidence |
| Provenance | Scanner version, rules identity, configuration digest, content identity, policy identity, sanitized report digest |

Use SARIF 2.1.0 where supported. Parse native JSON when it preserves required scanner data better. Export normalized JSON for SOP. Do not require all providers to expose the same native format.

Fingerprints must use stable, non-secret identifiers. Keep provider identity in the fingerprint namespace. Cross-provider deduplication is deferred; grouped findings must retain every source.

Protocol and schema details are finalized during SEC-001. Illustrative names in this document are not existing SOP APIs or configuration keys.

## 6. Execution, evidence, and gate behavior

1. Validate configuration and required scan categories.
2. Resolve configured adapters and inspect their capabilities.
3. Confirm scanner availability and the requested content identity.
4. Execute scanners with bounded time and output.
5. Sanitize reports before persistence or model access.
6. Normalize findings and evaluate coverage.
7. Apply baseline, suppression, and gate policy.
8. Persist evidence through existing SOP run artifacts.
9. Continue automatically when existing policy permits it.

Execution statuses: COMPLETED, FAILED, TIMED_OUT, UNAVAILABLE, SKIPPED, or CANCELLED. Coverage is reported independently as complete, partial, or unknown.

Policy verdicts: PASS, FAIL, or INDETERMINATE. An intentionally disabled add-on is NOT_APPLICABLE. Report-only mode records the verdict but does not block delivery. Required mode blocks on FAIL or INDETERMINATE. Optional scanner failures remain visible.

Default MVP behavior: report-only, full workspace scan, no automatic baseline creation, and no automatic fixes. A blocking policy must define required categories and severity thresholds. Unknown severity must follow an explicit policy; it must not silently become low severity.

Changed-file scope is an optimization, not equivalent to a full scan. Record exclusions and limitations. A required full-scan policy cannot pass using changed-file evidence. Changes to relevant input invalidate prior evidence. Resume may reuse a completed result only when content, scanner, rules, configuration, and policy identities still match.

Sanitized native reports may be retained for debugging. Raw reports that contain secret values must not enter ordinary run artifacts. Use restricted temporary storage when unavoidable, and remove it on completion, failure, and cancellation. Redaction failures make the scan result unusable.

## 7. Operating constraints

- Invoke approved executables directly with argument arrays.
- Supply only required environment variables. Never log credential values.
- Resolve scanned paths within the configured workspace boundary.
- Pin supported scanner versions and record rules versions or content digests.
- Keep rule downloads and hosted uploads explicit. Local execution does not guarantee offline operation.
- Declare runtime requests and credential verification as separate capabilities. Keep them disabled in MVP.
- Bound retries. Reuse existing runner budgets; do not create an independent continuation loop.
- Do not claim sandbox isolation unless the runtime enforces it.
- Review engine, rules, action, and redistribution licenses separately before packaging. Free CLI access does not imply free hosted AI features.

## 8. Implementation plan

Execute these phases end to end in dependency order. Continue through implementation, verification, documentation, and PR preparation. Stop only for a real blocker or an existing approval requirement. Preserve unrelated changes.

| Task | Work | Completion criteria |
| --- | --- | --- |
| SEC-001 — Discover and define | Inspect runner hooks, adapter/extension patterns, policy gates, artifact storage, and tests. Define protocol v1 and state transitions. | Document exact integration paths, schema, compatibility policy, license decisions, and meaningful validation commands. No unsupported claims about current code. |
| SEC-002 — Build execution layer | Add configuration, descriptor validation, process execution, cancellation, timeout, output limits, and evidence identity. | Fake adapters prove replacement without runner edits. Failure and partial-coverage cases remain distinct from a clean result. Disabled behavior is preserved. |
| SEC-003 — Add scanner adapters | Wrap pinned Semgrep CE and Gitleaks versions. Add structured parsing, severity mapping, and secret-safe reporting. | Real CLI runs detect seeded findings. Both adapters satisfy protocol conformance checks. Redaction checks pass across artifacts and logs. |
| SEC-004 — Integrate policy and resume | Connect report-only and required modes to existing SOP verification. Add baseline/suppression handling and bounded resume. | Gate matrix passes. Expired suppression becomes visible. Stale evidence is rejected. Retry and cancellation preserve existing runner behavior. |
| SEC-005 — Dogfood and deliver | Run on agentic-sop in report-only mode. Document findings, limitations, setup, extension guide, and rollback. Prepare focused PR. | Required checks pass. Evidence identifies scanned revision and coverage. PR contains only intended changes. Report actual timings and tool availability. |

Avoid unrelated refactors. Use existing packages and libraries where suitable. Do not add another mandatory service or model call to the normal run path.

## 9. Verification and release acceptance

Release requires all of the following:

- A replacement adapter runs through configuration alone.
- Semgrep detects a known supported insecure-code fixture.
- Gitleaks detects a synthetic secret fixture without using live credentials.
- Clean fixtures produce no unexpected fixture findings.
- Missing tool, timeout, cancellation, malformed report, oversized output, and partial scan produce correct execution and gate states.
- Required-mode behavior passes the full severity, coverage, and execution-status matrix.
- Seeded secret values do not appear in persisted files, captured logs, prompts, or telemetry.
- Baselines do not hide new findings. Expired suppressions do not remain effective.
- Relevant input changes invalidate cached evidence.
- Existing approval boundaries, continuation budgets, and disabled-mode behavior remain correct.
- One SOP invocation completes the configured scan and report path without manual continuation, unless a real blocker or existing policy gate occurs.
- Repository-required checks pass. Real scanner validation runs in the supported integration environment; skipped local checks are reported explicitly.

Measure scan duration, adapter overhead, coverage, finding counts, execution failures, and manual continuations. Establish the first baseline during dogfooding. Do not invent a speed or accuracy target before measurement. A passing scan is evidence for the configured tools and scope, not proof that the code has no vulnerabilities.

## 10. Stop conditions and rollback

Stop dependent work when the existing runner cannot represent scan outcomes safely, a required tool/license is unavailable, redaction cannot be guaranteed, or an approval boundary requires a decision. Complete independent work and report the exact blocker with evidence and the smallest next action.

Do not weaken a gate, mark an unavailable scanner as clean, or auto-accept a baseline to finish the run.

Rollback: disable the add-on through configuration. Existing non-security verification must continue to work. Preserve prior sanitized evidence. Revert implementation commits if integration changes existing runner behavior unexpectedly.

## 11. Open decisions to resolve during discovery

- Exact runner hook and extension mechanism.
- Initial supported OS and scanner version matrix.
- Scanner/rules provisioning and offline configuration.
- Baseline storage location and ownership.
- Default blocking thresholds for an opt-in required profile.

These decisions do not block the PRD. Resolve them from repository evidence before implementation. Escalate only when available evidence and existing authorization cannot resolve a material choice.

## 12. Reference documentation

Provider features and plans can change. Recheck exact versions and terms during SEC-001.

- Semgrep CE and platform comparison: https://semgrep.dev/products/semgrep-vs-ce
- Semgrep engine and supported languages: https://github.com/semgrep/semgrep
- Snyk Code CLI structured output: https://docs.snyk.io/developer-tools/snyk-cli/commands/code-test
- Gitleaks implementation and reports: https://github.com/gitleaks/gitleaks
- TruffleHog and verification behavior: https://github.com/trufflesecurity/trufflehog
- ZAP Automation Framework: https://www.zaproxy.org/docs/automate/automation-framework/
- Nuclei templates: https://github.com/projectdiscovery/nuclei-templates

This document records the original security scanner requirements. Current
behavior is defined by [SECURITY-SCANNER-ADDON.md](../specs/SECURITY-SCANNER-ADDON.md);
repository integration details were reconciled during implementation.
