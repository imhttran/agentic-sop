# LLM02 — Sensitive information disclosure (SOP Mac workstream)

Status: static tracing IMPLEMENTED and VERIFIED; live extraction `NOT_RUN`.
Baseline: `imhttran/agentic-sop` `main` at
`0750e8be0cf5f318a173abb34c2b752ac131521c`.
Host: macOS `Darwin` arm64.
Branch: `security/llm02-disclosure`.

This report traces sensitive information from inputs to model requests, tool
effects, persisted evidence, and operator output. It closes two reproduced leaks
at existing boundaries and leaves live adversarial extraction explicitly
incomplete because Plan 02's merged live-test infrastructure is not present in
this checkout.

## Method

- Traced file reads, process environment, configuration, tool stdout/stderr,
  retrieval results, provider request construction, diagnostics, run artifacts,
  handoffs, and notification payloads from the current code.
- Distinguished provider authentication (the `Authorization` header) from
  model-visible content (the task, context, tool results, and diff).
- Reviewed credential-redaction call sites, not only the shared helper.
- Used synthetic canaries only. No real credential value was read or printed.
- Reproduced each leak with a failing test before changing code.

## Data-flow trace

| Source | Transformation / redaction | Authorization boundary | Sink | Retention | Evidence |
| --- | --- | --- | --- | --- | --- |
| Repository files read by the agent's `read_file` / `list_files` / `search_files` | Bounded output; no general DLP | Path confinement to the primary/authorized roots; `.agent-sdlc/state.db` and `.agent-sdlc` search are protected | Model tool result (model-visible) | Not persisted by SOP; the model's report may echo it in `implementation.md` | `internal/toolharness/files.go`; `TestReadRejectsEscapes`, `TestStateDBProtectedForEveryAccessClass`, `TestReadWriteCreateListSearch` |
| Process environment (`os.Environ`) | Provider API keys are used only for the `Authorization` header; handoff/diagnostic surfaces redact secret-named values by exact value; the scanner child gets an allowlist environment | Provider auth is separate from the request body; scanner env allowlist; no OS sandbox for the agent | Provider HTTP header (not model-visible); handoff capsules; diagnostics | Provider header is not logged; handoff capsules are persisted after redaction | `internal/agent/provider.go` `postJSON`; `internal/security/exec.go` `scannerEnv`; `TestOpenAICompatibleExecutionReusesAuthorization`; `TestRedactExcludesSecrets`, `TestManagerRedactsBeforePersisting` |
| `SOP_TASK_INPUTS` caller observations | Inserted as task input; no automatic redaction | Operator-supplied task data; treated as data, not authority | Model request | Not persisted separately; model output may echo | `internal/cli/task_input.go`; task-input contract |
| Configuration (`.agent-sdlc/config.yaml`, policy) | Strict decode; notification status drops credential keys/values | Repository write access; config is policy, not secret storage | Model context only where a task names it; notification status output | Config is tracked; notification status is redacted | `docs/specs/SECURITY.md` §5; `internal/cli/notify.go` `redactNotificationStatusDoc` |
| Validation command stdout/stderr | **Fixed:** known environment secret values and credential-shaped text are redacted before the failure context is built and before check logs are persisted | Configured validation commands are trusted project configuration; the output is diagnostic data, never authority | Model request (failure context) and persisted `checks/*.log` | Check logs are written under the task run directory; failure context is sent, not separately persisted | `internal/cli/run.go` `validationFailureContext`; `internal/cli/outcome.go` `runAcceptanceArgv`; new `internal/cli/redaction.go`; `TestValidationFailureContextRedactsCredentialShapes`, `TestCheckEvidenceRedactsCredentialShapes` |
| Tool stdout/stderr (`run_command`) | Bounded output; no general DLP | Command policy allows only SAFE commands; path confinement; destructive Git denied | Model tool result (model-visible) | Model report may echo; tool audit records a summary, not raw output | `internal/toolharness/harness.go`; `TestCommandPolicyAllowsSafeCommands`, `TestToolExecutionIsAudited` |
| Retrieval results (RAG / advisory memory) | Bounded token/result budget; drift exclusion; evidence text is untrusted data | Corpus, repository, and task scope are applied before search; the ranker adapter refuses a scope it cannot honor; RAG is disabled by default | Model context | Optional RAG/advisory artifacts are run-scoped; disabled by default | `internal/retrievalboundary/boundary.go`, `internal/memindex/index.go` `Search`; `TestSearchRejectsUnrelatedEvidence`, `TestRankerAdapterRejectsScopeItCannotHonor`, `TestLocalRetrieverRequiresCurrent` |
| Provider request construction | Task, plan, context, prior attempt, validation failure, review findings, and diff are compiled into the request body; the API key is only the `Authorization` header | Provider selection and request construction are SOP code; model output is data, never authority | Provider HTTP body (model-visible) and header (auth only) | Request body is not persisted by SOP; provider retention is provider-owned | `internal/cli/run.go` `implementContext`, `fixContext`; `internal/agent/provider.go` `postJSON`; `TestOpenAICompatibleExecutionReusesAuthorization` |
| Activity stream / `activity.jsonl` | One-line summaries; command assignments are masked by secret-name; no prompts, model output, file contents, or raw tool arguments | Display-only; never feeds a decision | Operator terminal and run artifact | `activity.jsonl` is run-scoped | `internal/cli/activity.go`; `internal/ollamaagent/activity.go` `redactCommand` |
| `trace.json` | Bounded short text; opaque identifiers and enumerated labels; no raw prompt/completion content | Observation-only | Operator/API artifact | Run-scoped | `internal/runtrace/trace.go` |
| `implementation.md`, `diff.patch`, `plan.md`, `task.md`, `report.md` | Evidence is persisted as produced; no general DLP rewrite | SOP-owned run evidence; operator-controlled working tree | Operator/API artifact | `.agent-sdlc/runs/<task>/` | `internal/run/run.go` `Write`; run artifact inventory |
| Acceptance-check logs (`checks/*.log`) | **Fixed:** known environment secret values and credential-shaped text are redacted; otherwise the complete output is retained (bounded excerpt shown in the report) | Acceptance checks are authorized commands; the log is evidence | Persisted diagnostic and report excerpt | Run-scoped `checks/<id>.log` | `internal/cli/outcome.go`; `TestFailedCheckRecordsFullLogArtifact`, `TestCheckEvidenceRedactsSecrets`, `TestCheckEvidenceRedactsCredentialShapes` |
| Handoff capsules | Exact secret-named environment values are replaced with `[redacted]` before persistence and compression | Direct dependencies only; completed tasks only; bounded lists | Dependent task context and persisted capsule | `.agent-sdlc` handoff store | `internal/handoff/redact.go`, `internal/handoff/manager.go`; `TestManagerRedactsBeforePersisting`, `TestRedactUsesSharedSecretNames` |
| Outcome records / comparison artifacts | Typed, shape-aware redaction (`outcomefile`, `hipcompare`) | Read-only ingestion of SOP evidence | Persisted outcome/comparison artifacts | Run-scoped | `internal/outcomefile/outcomefile.go` `Redact`; `TestRedactKeepsTypedMarkers`; `internal/hipcompare/write.go` |
| ACP / controller diagnostics | Shared shape-aware line/value redaction; ACP additionally redacts long opaque blobs | Transport diagnostics; no lifecycle authority | Operator/client diagnostic text | Not persisted by SOP | `internal/acp/diagnostics.go`; `internal/controller/sopclient`; `TestDiagnosticsRedactCredentials`, `TestCommandErrorRedactsBeforeBounding` |
| Notification payloads | Configured webhook/file URLs and userinfo are redacted; lifecycle events carry SOP's recorded reason/next action, not raw command output | Event identity and kind come from recorded transitions; a `BLOCKED` event is not a human gate | Outbound webhook/file payload | Durable outbox; delivery attempts recorded | `internal/notify/webhook.go`, `internal/cli/lifecycle_notify.go`; `TestEventRenderingCarriesNoCredential`, `TestWebhookSlackPayloadCarriesNoURL` |

## Where redaction happens

- **Before sending to a model:** validation failure context and acceptance-check
  output now pass through `redactDiagnosticText` (known environment secret values
  plus credential-shaped text). Repository file content, tool output, the diff,
  and review findings are intentionally **not** generally DLP-scrubbed; they are
  the engineering content the model must see. This is a bounded claim, not a
  promise to detect every confidential string.
- **Before persistence:** handoff capsules redact exact secret-named environment
  values; outcome/comparison artifacts use the shared shape-aware detector;
  acceptance-check logs now use the same shape-aware detector plus environment
  values; notification status drops configured credential keys/values.
- **Before display:** activity lines are summaries with sensitive command
  assignments masked; ACP/controller diagnostics use the shared detector; the
  notification status document removes configured credential values.

## Authorization boundaries

- The tool harness confines file reads/writes and command arguments to the
  primary repository and explicitly authorized roots, protects
  `.agent-sdlc/state.db`, and refuses destructive Git. It does not provide an OS
  sandbox for normal agent execution (see
  [../security-shift-left/execution-path-matrix.md](../security-shift-left/execution-path-matrix.md)).
- Retrieval enforces corpus, repository, and task scope before search. The
  ranker adapter fails closed when asked to honor a narrower scope it cannot
  represent.
- Provider authentication is an `Authorization: Bearer` header. The API key is
  not placed in the JSON request body; the new assertion strengthens that test.
- Human approval remains the only authority for commit, push, and merge. This
  work adds no approval layer and changes no gate.

## Reproduced gaps and fixes

### GAP-01 (fixed): acceptance-check logs persisted credential-shaped text

The check-output boundary used `handoff.Redact`, which replaces exact
secret-named environment values but not a credential shape that is absent from
the environment. A synthetic `sk-…` canary printed by a failing check persisted in
`checks/<id>.log`, the report excerpt, and the outcome document.

Reproduction before the fix (new test against the unmodified code):

```text
--- FAIL: TestCheckEvidenceRedactsCredentialShapes
    log artifact did not redact the credential-shaped canary:
    ... fail_test.go:10: auth=sk-canary-not-a-real-secret budget exhausted
```

Fix: `redactDiagnosticText` applies the shared `redact.ShapesValues` detector
after the existing exact environment-value redaction, and `runAcceptanceArgv` uses
it. The marker remains `[redacted]`; ordinary text such as `budget exhausted`
survives.

### GAP-02 (fixed): model-visible validation failure carried secrets

`validationFailureContext` embedded the raw stdout/stderr of a failed validation
command into the next model request. A synthetic environment secret and a
credential-shaped canary were both present in that context.

Reproduction before the fix (new test against the unmodified code):

```text
--- FAIL: TestValidationFailureContextRedactsCredentialShapes
    validation failure context leaks "env-secret-value-42":
    ...
    token=env-secret-value-42 auth=sk-canary-not-a-real-secret token budget exhausted
```

Fix: `validationFailureContext` passes each stream through
`redactDiagnosticText`. The ordinary failure text remains available to the model.

### Provider authentication is not model content

`internal/agent/provider.go` `postJSON` sets `Authorization: Bearer <key>` on the
request and marshals only the model/messages payload. The existing
`TestOpenAICompatibleExecutionReusesAuthorization` now also asserts the raw
request body does not contain the key. No provider credential is copied into the
prompt or the request JSON.

## Verification record

Commands run in this worktree on macOS arm64:

```sh
go test ./internal/cli -run 'TestCheckEvidenceRedactsCredentialShapes|TestValidationFailureContextRedactsCredentialShapes|TestCheckEvidenceRedactsSecrets' -count=1
go test ./internal/agent -run 'TestOpenAICompatibleExecutionReusesAuthorization' -count=1
go test ./internal/retrievalboundary ./internal/memindex ./internal/handoff ./internal/notify ./internal/acp ./internal/controller/sopclient ./internal/outcomefile -count=1
```

Observed: all PASS.

| Check | Evidence | Result |
| --- | --- | --- |
| Exact environment secrets are redacted from check logs | `TestCheckEvidenceRedactsSecrets` | PASS (existing) |
| Credential-shaped text is redacted from check logs | `TestCheckEvidenceRedactsCredentialShapes` | PASS (new) |
| Model-visible validation failure redacts env secrets and shapes | `TestValidationFailureContextRedactsCredentialShapes` | PASS (new) |
| Provider key stays in the auth header, not the request body | `TestOpenAICompatibleExecutionReusesAuthorization` | PASS (strengthened) |
| Empty and ordinary diagnostic text is preserved | `TestRedactDiagnosticTextKeepsOrdinaryText` | PASS (new) |
| Handoff capsules redact before persistence | `TestManagerRedactsBeforePersisting`, `TestRedactUsesSharedSecretNames` | PASS (existing) |
| Retrieval rejects unrelated scope | `TestSearchRejectsUnrelatedEvidence`, `TestRankerAdapterRejectsScopeItCannotHonor` | PASS (existing) |
| Notification event text carries no credential | `TestEventRenderingCarriesNoCredential` | PASS (existing) |
| ACP/controller diagnostics redact credentials | `TestDiagnosticsRedactCredentials`, `TestCommandErrorRedactsBeforeBounding` | PASS (existing) |
| Outcome/comparison artifacts keep typed redaction | `TestRedactKeepsTypedMarkers` | PASS (existing) |

## Acceptance mapping and PARTIAL semantics

Authoritative contract: SOP-003 in `internal/taskfile/taskfile.go`
(`AcceptanceMapping`), `internal/planner/plan.go`
(`ParseAcceptanceMapping` and its validation), and `internal/outcome/criteria.go`
(`VerifyAcceptance`, `LegacyCompatibility`), consumed by
`internal/cli/outcome.go` (`deriveMappedOutcome`, `specCriterionMappings`).

A stage with acceptance criteria but no `### Acceptance Mapping` is a legacy
stage: the quality gate can pass and every deliverable and check can pass, but the
criteria remain unmapped, so `VerifyAcceptance` records `PARTIAL` and never
`VERIFIED`. `PARTIAL` here means the objective-level acceptance verdict is
unverified; it does not mean the configured build, test, lint, or acceptance check
failed.

The mapping now recorded for this workstream is:

- DISCLOSE-001 criterion 1 -> check 1 (`go test ./internal/cli -run TestOWASPLLMDisclosureReportPresent -count=1`).
- DISCLOSE-002 criterion 1 -> check 1 (`go test ./internal/cli -count=1`).
- DISCLOSE-003 criterion 1 -> manual: Plan 02 merged live-test infrastructure and recorded live extraction evidence (currently `NOT_RUN`).

The mapping for the two executed tasks was added after their first execution.
`sop reconcile --accept-changed` accepted the changed executed definition while
preserving status and attempt history, so the historical outcome artifact remains
`PARTIAL`. This report does not rewrite that outcome and does not claim `VERIFIED`
from the final gate alone. DISCLOSE-003 remains `PLANNED`; its manual mapping does
not mark live acceptance complete.

## Configured scan on this baseline

Command (report-only, the configured routine mode):

```sh
sop security scan --report-only
```

Observed on the final working tree:

- Verdict `PASS`; coverage `COMPLETE`; duration 9,042 ms.
- Commit `0750e8be0cf5f318a173abb34c2b752ac131521c`; working tree dirty with the Plan 03 change set.
- `gitleaks`, `semgrep`, and `osv-scanner`: `COMPLETED`, coverage `COMPLETE`.
- Findings: 67 total, 53 suppressed by the reviewed baseline, 0 unsuppressed blocking findings.
- Required-mode readiness: `false` because `security.required_categories` is empty. Required mode is **not enabled**.
- Artifact: `.agent-sdlc/runs/security-20261109T011008Z/security/scan.{md,json}` (machine-local/git-ignored).

The synthetic canaries are deliberately non-secret placeholder strings; they still
exercise the shared credential-shape detector without adding a scanner finding or
new suppression.

## Live extraction — NOT_RUN

Plan 03's live adversarial tests depend on Plan 02's merged live-test
infrastructure. That infrastructure is not present in this checkout: the
Promptfoo path is disabled in the routine configuration and cannot route a live
model tool decision through `evaltarget` (see
[../security-shift-left/aisec-live-assessment.md](../security-shift-left/aisec-live-assessment.md)).

- Direct extraction: `NOT_RUN`.
- Indirect injected extraction: `NOT_RUN`.
- Unauthorized tool effect under a live model proposal: `NOT_RUN`.
- Prerequisite: an evaltarget-routed live path whose observer reads back a model
  tool decision, plus the pinned binary and a reachable local model.

These acceptance items remain incomplete. No fixture or text assertion is
presented as live evidence.

## Residual risks and bounded claims

| Item | State | Note |
| --- | --- | --- |
| Repository file content read by the agent | GAP / bounded | The agent can read repository files, including a local `.env` if the operator placed one in the tree. SOP protects its own state database, not arbitrary credential files; this is a path-authorization question, not general DLP. |
| Tool output and command output seen by the model | GAP / bounded | Tool output is bounded and command-policy-constrained, but not generally DLP-scrubbed. A secret printed by a permitted command can reach the model. |
| `diff.patch` and `implementation.md` evidence | GAP / bounded | These are persisted as evidence and are not rewritten. An operator should not place real credentials in the working tree or model output. |
| Review findings and task input | GAP / bounded | Review findings and operator observations are model-visible data; SOP does not claim general DLP over arbitrary prose. |
| Notification `Reason` text | caller-owned | The emitter redacts configured targets and carries SOP's recorded reason; a caller that passes raw secret text would leak. Current lifecycle producers pass fixed/generic reasons. |
| Pattern matching | bounded | Credential-shape detection is not general DLP and cannot promise to protect every sensitive string. Unknown formats remain explicitly out of scope. |
| Normal agent OS isolation | NOT_APPLICABLE / UNVERIFIED | SOP does not sandbox the agent process; the execution-path matrix is unchanged. |
| Live extraction | NOT_RUN | Pending Plan 02's merged live-test infrastructure. |

## Handoff to Plan 04

The confirmed inventory observations are:

- Provider credentials are used for transport authentication only; they are not
  model-visible context.
- Scanner child processes receive an allowlisted environment, not the operator's
  full environment.
- Handoff capsules and acceptance-check logs are persisted after redaction;
  repository diffs and model reports are evidence and are not rewritten.

No credential value is included in this report.
