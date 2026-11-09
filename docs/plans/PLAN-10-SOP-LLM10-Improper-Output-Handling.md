# Plan 10 — SOP improper output handling (LLM10)

Status: EXECUTING; adapted to the SOP plan schema from the planning document.
Date: 2026-11-08 (America/Chicago).
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm10-improper-output-handling.

## Project

agentic-sop

## Summary

Treat model and tool output as untrusted at execution, persistence, and display
sinks. Verify the existing validation, encoding, and authorization boundaries
without creating a second command-policy layer. Reproduce and repair demonstrated
violations at SOP-owned sinks only.

## Goal and scope

Trace structured tool arguments, shell and subprocess construction, file and patch
paths, database queries, templates, terminal output, Markdown and HTML rendering,
notifications, and protocol errors. Record which sinks SOP owns and which belong
to external clients. Fix only demonstrated gaps using existing abstractions.

## Dependency reconciliation

- Plan 03 (LLM02 redaction) is merged at this baseline. Its redaction surfaces and
  tests are reused; this plan does not re-implement them.
- Plan 01 (action authority, LLM03) merged to `main` at `985472a` (PR #271)
  while this branch was in review. Its report and evidence are linked from the
  cumulative matrix. No acceptance criterion here depends on that code; the
  existing command policy and path resolver remain the authority this plan
  verifies.
- Plan 07 (evidence claims) does not exist in this checkout. No acceptance
  criterion here depends on it.
- No dependency blocks execution. The absent work is recorded as a boundary, not
  treated as satisfied.

## OUTPUT-001 — Inventory output sinks

Trace structured tool arguments, shell and subprocess construction, file and patch
paths, database queries, templates, terminal output, Markdown and HTML rendering,
notifications, and protocol errors. Record which sinks SOP owns and which belong to
external clients.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm10-output-handling.md`

### Acceptance Criteria

- Each sink lists input origin, parser, validation or encoding, effect, and policy owner.
- Intentional approved shell execution is identified separately from accidental interpolation.
- Terminal control sequences and any automatic resource fetching in owned renderers are identified.
- The output-handling report `docs/reports/owasp-llm/llm10-output-handling.md` is delivered.

### Acceptance Checks

- `git grep -q "Improper output handling" -- docs/reports/owasp-llm/llm10-output-handling.md`

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm10-output-handling.md §Sink inventory
- criterion 2: manual: docs/reports/owasp-llm/llm10-output-handling.md §Intentional shell execution versus interpolation
- criterion 3: manual: docs/reports/owasp-llm/llm10-output-handling.md §Terminal controls and resource fetching in owned renderers
- criterion 4: check 1

### Execution

- verify-first — the report is present in the working tree and the configured validation proves the repository still builds.

## OUTPUT-002 — Repair demonstrated sink violations

Use inert execution recorders, disposable files and databases, and controlled
render fixtures. Test metacharacters in data arguments, malformed structured calls,
path traversal, terminal escape sequences, and remote-resource references only
where a reachable sink exists. Fix only demonstrated gaps, using sink-specific
encoding or validation.

### Dependencies

- OUTPUT-001

### Deliverables

- `internal/cli/terminal.go`
- `internal/cli/terminal_output_test.go`
- `internal/cli/cli.go`
- `internal/cli/activity.go`
- `internal/notify/slack.go`
- `internal/notify/slack_test.go`

### Acceptance Criteria

- Data arguments cannot become unintended shell syntax; approved shell behavior still follows existing authority.
- Invalid structured operations cannot partially execute.
- Supported file operations retain existing workspace and symlink boundaries.
- Database data stays parameterized at applicable sinks.
- Owned display paths cannot execute terminal controls or silently fetch attacker-selected resources contrary to their security contract.
- Legitimate formatting and valid authorized operations remain usable.
- Fixes use sink-specific encoding or validation; persisted structured data is not globally escaped.

### Acceptance Checks

- `go test ./internal/toolharness -run TestSplitCommandRejectsShellMetacharacters -count=1`
- `go test ./internal/toolharness -run TestDeniedOperationsCauseNoSideEffect -count=1`
- `go test ./internal/toolharness -run TestReadRejectsEscapes -count=1`
- `go test ./internal/store -count=1`
- `go test ./internal/cli -run Terminal -count=1`
- `go test ./internal/notify -run Slack -count=1`
- `go test ./internal/cli -run TestPromptJSONKeepsModelResultIntact -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3
- criterion 4: check 4
- criterion 5: check 5
- criterion 6: check 7
- criterion 7: manual: docs/reports/owasp-llm/llm10-output-handling.md §Reproduced gaps and fixes and TestPromptJSONKeepsModelResultIntact; the terminal sink filters console writes only and JSON/artifacts keep their exact data

### Execution

- verify-first — the fix and its regression tests are present in the working tree; the focused checks and the configured validation prove them.

## OUTPUT-003 — Verify adapters and close the coverage matrix

Verify changed behavior through the actual CLI, MCP, ACP, and notification
serialization paths. Do not send live Slack messages. Record external clients that
cannot be controlled as explicit limitations. Update the cumulative matrix without
overstating coverage.

### Dependencies

- OUTPUT-001
- OUTPUT-002

### Deliverables

- `docs/reports/owasp-llm/llm10-output-handling.md`
- `docs/reports/owasp-llm/README.md`

### Acceptance Criteria

- Protocol data remains valid; diagnostics do not corrupt machine-readable output.
- Synthetic canaries and dangerous sequences stay contained in tested paths.
- Required checks pass; the PR records exact OS and client coverage.
- The cumulative OWASP report is updated with every category that has a plan or report in this checkout, preserving incomplete and historical evidence.
- A category is never marked fully covered merely because its plan or PR exists.

### Acceptance Checks

- `go test ./internal/cli -run TestPromptJSONKeepsModelResultIntact -count=1`
- `go test ./internal/acp -count=1`
- `go test ./internal/mcp -count=1`
- `go test ./internal/cli -run Terminal -count=1`
- `git grep -q "Improper Output Handling (LLM10)" -- docs/reports/owasp-llm/README.md`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 4
- criterion 3: manual: the PR body records the Linux x86_64 CLI/ACP/MCP verification scope and the unverified external clients
- criterion 4: check 5
- criterion 5: manual: docs/reports/owasp-llm/README.md §What this matrix does not claim and the Plan 06–09 rows

### Execution

- verify-first — the matrix and report updates are present in the working tree.

## Risks and completion

Shell-capable tools intentionally execute operator-configured code; argument arrays
alone do not constrain an interpreter. SOP cannot guarantee how every third-party
terminal, editor, IDE, Slack client, or remote renderer presents content. The plan
is complete for SOP-owned sinks with focused evidence; unsupported external
rendering remains an explicit gap.

## Execution contract

- Reuse existing abstractions. Record ALREADY_SATISFIED only with current evidence.
- Do not introduce another gate, evaluator, permission system, or service.
- Fix only reproducible gaps against existing contracts.
- Preserve configured security modes, routing, autonomy, and permissions.
- Use synthetic fixtures, inert recorders, disposable data, and no live Slack
  message, exfiltration endpoint, production write, or downloaded artifact.
- Run focused regression tests, then the repository-required checks once. Repeat
  only when a change or failure justifies it.
- Open a focused PR through the established workflow. Do not merge, install,
  deploy, or remove worktrees.

## Baseline and evidence rules

Baseline: `imhttran/agentic-sop` `main` at
`24be0687b0ac0dec5e2e69584be67f1795c0f6f1`, merged with `main` at `985472a`
before review; Linux `x86_64`, Go 1.27.1. Historical observations from the
planning document are context, not runtime evidence.

Use `IMPLEMENTED`, `ENABLED`, `VERIFIED` (with platform and path), `NOT_RUN`, `GAP`,
and `NOT_APPLICABLE` as separate dimensions. A deterministic test is not live-model
evidence; a scan `PASS` is not complete category coverage. Missing required
evidence leaves the associated criterion incomplete.

Category numbers and names follow the planning document's article-based taxonomy,
not a verified formal OWASP compliance mapping.

## Sources

- https://cycode.com/blog/owasp-top-10-llm-applications/
- Repository: https://github.com/imhttran/agentic-sop
- Earlier plans: PLAN-01-SOP-LLM03-Excessive-Agency.md; PLAN-02-SOP-LLM01-Prompt-Injection.md; PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md; PLAN-04-SOP-LLM04-Supply-Chain.md.
- Policy: docs/specs/SECURITY.md and docs/specs/QUALITY.md.
- Evidence: docs/reports/owasp-llm/README.md and docs/reports/owasp-llm/llm10-output-handling.md.
