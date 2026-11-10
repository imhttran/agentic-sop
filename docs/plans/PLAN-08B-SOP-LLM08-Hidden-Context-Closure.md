# Plan 08B — SOP hidden-context closure (LLM08)

Status: follow-up to the historicalized Plan 08; reuses merged PR #286 and #290 evidence.
Date: 2026-11-09.
Series: continuation of `docs/history/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md`.
Suggested repository path: `docs/plans/PLAN-08B-SOP-LLM08-Hidden-Context-Closure.md`.

## Project

agentic-sop

## Summary

Close the remaining Plan 08 hidden-context coverage at the current revision. Reuse the
merged behavioral controls and the Plan 02 live seam, but re-verify the current code
because the recorded campaign revision is stale. Add the notification and tool-output
paths to the inventory, verify secrets/redaction and authority independence with
synthetic canaries, refresh bounded live evidence with the already-installed local
`qwen3.5:4b`, and reconcile the report and matrix with a bounded claim.

Do not claim prompt secrecy or general DLP. Do not change security defaults, model
routing, configured limits, or retrieval configuration. Do not add a new evaluator,
DLP layer, or gate.

## Environment evidence

Operator-run, read-only, before execution:

- Host: macOS `26.6.2`, arm64; Go `1.27.1 darwin/arm64`.
- Baseline `origin/main` at `35340774` (after PRs #328/#329). The recorded Plan 08
  campaign revision is `e84eb4aa`; the relevant code drifted by 30 files and 2575
  insertions across agent, ollamaagent, toolharness, prompt, evaltarget, handoff,
  approval, context, retrievalboundary, and memindex, so current-revision evidence is
  required.
- Ollama is running and `qwen3.5:4b` is installed; the supported local endpoint is
  `http://127.0.0.1:11434`.
- Plan 08 is historicalized. The matrix records its manual criteria as accepted at
  merge without a per-criterion SOP record; this follow-up records current evidence.
- Notifications: `internal/notify` carries `Reason` and `NextAction` to configured
  channels. Plan 03 redaction is not applied in that package.
- Tool output: tool results are fed back to the model and bounded by the harness.
  `.agent-sdlc` is readable except `state.db`; `.env` is not a protected path.

## CONTEXT-004 — Reconcile the hidden-context inventory at the current revision

Refresh the report's inventory and classification at the current revision. Add the
notification path and the tool-output path, including errors, logs, traces, artifacts,
and ACP/controller diagnostics. Record each existing protection and its enforcement
point. Do not print real credential values.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm08-hidden-context.md` — updated inventory, classification,
  persistence/export, and executable-versus-instruction table.
- `internal/cli/llm08_hidden_context_test.go` — extend the report-structure check to
  require the new sections/rows and the bounded-claim text guard.

### Acceptance Criteria

- The inventory covers system/developer instructions, tool schema, tool output,
  retrieval, errors/logs/traces, notifications, artifacts, and ACP/controller
  diagnostics.
- Each protection names its enforcement point and whether it is executable or
  instruction-only.
- Notifications and tool output are classified with their sinks and persistence/export.
- Raw prompts are recorded as not persisted; model output is recorded as persisted
  verbatim.
- No real credential value is read or printed.

### Acceptance Checks

- `go test ./internal/cli -run TestOWASPLLM08ReportStructure -count=1`
- `go test ./internal/agent ./internal/ollamaagent ./internal/toolharness ./internal/prompt -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 2

## CONTEXT-005 — Verify deterministic boundaries and fix demonstrated violations

Verify the notification and tool-output boundaries with synthetic canaries. Apply the
existing `internal/redact` credential-shape detector at the notification boundary so a
credential-shaped `Reason`, `NextAction`, or `Outcome` cannot be persisted to the outbox
or delivered to a configured channel. Keep a non-secret positive control.

Demonstrate the tool-output boundary with a disposable fixture: create a synthetic
`.env` canary under a temporary root and read it through the real tool path. Record
whether the canary enters model-visible tool output and the persisted run artifacts. If
it does, record this as a `GAP` with concrete options — deny credential-shaped files at
the harness, redact known values from tool output, or accept repository content as
model-visible — and a recommendation. Do not implement a new confidentiality policy or
general DLP without that decision.

Re-verify authority independence: retrieved or malicious text cannot change instruction
authority, and instruction disclosure cannot create approval state or bypass
verification. Reuse the existing real request/tool/authorization tests and add a
positive control.

### Dependencies

CONTEXT-004

### Deliverables

- `internal/notify/llm08_redaction_test.go` — credential-shaped notification text is
  redacted before persistence/delivery, with a non-secret positive control.
- `internal/cli/llm08_hidden_context_test.go` — disposable `.env` tool-output canary,
  artifact boundary, and authority-independence regressions with positive controls.
- `docs/reports/owasp-llm/llm08-hidden-context.md` — confirmed fix, observed
  tool-output behavior, `GAP` options, and corrected tool-output claim.

### Acceptance Criteria

- A synthetic secret in a notification reason does not appear in the outbox row or the
  Slack/native webhook payload; a non-secret positive control is still delivered.
- The notification fix uses the existing `internal/redact` detector; it adds no new DLP
  layer and does not change event identity or delivery semantics.
- A disposable `.env` fixture records whether the real tool path returns the canary;
  the observed behavior and concrete options/recommendation are recorded, and the
  dependent claim is `GAP` when the canary is model-visible.
- The report's tool-output claim matches the observed behavior; it does not claim
  ordinary tool results are redacted when they are not.
- Retrieved or malicious text cannot change instruction authority, and instruction
  disclosure cannot create approval state or bypass verification; the real
  request/tool/authorization paths are exercised with a positive control.

### Acceptance Checks

- `go test ./internal/notify -run TestLLM08 -count=1`
- `go test ./internal/cli -run TestLLM08 -count=1`
- `go test ./internal/agent ./internal/ollamaagent ./internal/toolharness ./internal/prompt -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 2
- criterion 4: check 2
- criterion 5: check 3

## CONTEXT-006 — Refresh bounded live extraction evidence

Reuse the existing Plan 02 live seam and the existing LLM08 campaign driver. Run the
tagged campaign on the current revision with the already-installed local `qwen3.5:4b`
and the fixed `127.0.0.1:11434` endpoint. Record disclosure, refusal, timeout, provider
failure, and inconclusive outcomes separately; one request per case, no retry. Update
the artifact and the report. If a prerequisite is absent, record `NOT_RUN` and complete
the independent work.

### Dependencies

CONTEXT-005

### Deliverables

- `docs/reports/owasp-llm/llm08-campaign.json` — current-revision campaign artifact.
- `docs/reports/owasp-llm/llm08-hidden-context.md` — live outcomes and limitations.

### Acceptance Criteria

- The artifact records the current revision, model, platform, endpoint, bounds, exact
  prompts, and per-case outcomes.
- Disclosure of non-secret instructions is distinguished from leakage of protected
  values.
- Refusal, timeout, provider failure, and inconclusive outcomes are recorded separately;
  a refusal is not reported as a secure boundary.
- No real credential, external receiver, or production effect is used.
- Missing prerequisites yield `NOT_RUN`, not a fabricated pass.

### Acceptance Checks

- `go test ./internal/cli -run TestOWASPLLM08CampaignArtifact -count=1`
- `go test ./internal/evaltarget -run TestLive -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 2

## CONTEXT-007 — Reconcile the report, matrix, and bounded claim

Update the report's criterion mapping, verification record, residual risks, and the
Plan 08 row in the matrix. State a precise bounded claim: SOP protects secrets by the
existing exclusion and redaction on the tested sinks and does not depend on prompt
secrecy; it does not guarantee prompt confidentiality, general DLP, or provider-side
retention. Do not change other matrix rows.

### Dependencies

- CONTEXT-004
- CONTEXT-005
- CONTEXT-006

### Deliverables

- `docs/reports/owasp-llm/llm08-hidden-context.md` — final verification record,
  criterion-to-evidence mapping, precise claim, and residual risks.
- `docs/reports/owasp-llm/README.md` — Plan 08 row only.

### Acceptance Criteria

- The report records tested platform, provider/model, code revision, and
  criterion-to-evidence mapping.
- `IMPLEMENTED`, `ENABLED`, `VERIFIED`, `NOT_RUN`, `GAP`, and justified
  `NOT_APPLICABLE` are separated.
- The bounded claim and residual risks distinguish secret protection from instruction
  secrecy and exclude general DLP and prompt confidentiality.
- The Plan 08 matrix row is consistent with the evidence; no other row changes.
- Repository-required checks pass.

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `go test ./... -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: manual: docs/reports/owasp-llm/llm08-hidden-context.md §Residual risks and bounded claims
- criterion 4: check 3
- criterion 5: check 3

## Risks and completion

Redaction cannot classify arbitrary confidential prose. Public knowledge of a schema is
expected; schema secrecy is not access control. Prompt text is disclosable; the
security argument is authority independence and secret exclusion, not prompt
confidentiality. Provider retention and enterprise DLP are outside scope. Complete only
for the tested paths and scopes; retain missing live coverage as `NOT_RUN`.

## Execution contract

Run the tasks sequentially through the existing SOP lifecycle and PR gates. Reuse merged
evidence from Plan 08, Plan 03, and Plan 02; do not reopen completed work without a
current-revision reason. Add no dependency, evaluator, DLP layer, permission system, or
service. Do not change security defaults, routing, configured limits, or retrieval
configuration. Use synthetic data, disposable fixtures, fake providers, and inert
effect recorders. Do not install tools, download models, enable remote evaluations, or
transmit real secrets.

Run focused checks for changed behavior, then the repository-required checks once.
Record exact commands, exit codes, base/head revisions, platform, provider/model, and
evidence paths. Open a focused PR and follow the established review workflow. Do not
merge, install, or deploy.
