# Proposal — LLM08 model-input credential policy

Status: **PROPOSAL — decision required. No executable policy change is included.**
Date: 2026-11-09.
Baseline: `origin/main` at `c87e45f4d3777756ba7bd9bd8f62fd9b0cb3aaad`, after PR #333.
Platform: macOS arm64; Go 1.27.1. Configuration: `security.mode: report-only`,
`security.evaluation.enabled: false`, `advisory_retrieval.enabled: false`,
`agent.provider: ollama`.

Related evidence: Plan 03 [llm02-disclosure.md](../reports/owasp-llm/llm02-disclosure.md)
§Tool-output `.env` canary reconciliation; Plan 08
[llm08-hidden-context.md](../reports/owasp-llm/llm08-hidden-context.md); the active
Plan 08B `CONTEXT-005` GAP record (uncommitted in its own worktree); and the current
Plan 08 matrix row.

## Problem

SOP currently lets a model read credential-bearing repository content through
several supported tool paths. The previously reported `.env` canary result is
confirmed:

- `model_visible=true` through the real `read_file` path.
- The harness audit does not persist tool result content (`in_artifacts=false` for
  the audit), but that is not general artifact protection.
- `implementation.md` and `result.md` persist model output verbatim, so a model that
  repeats the tool result can still put the value in a run artifact.

A read_file-only fix is not comprehensive. At the proposal baseline,
`internal/toolharness/disclosure_content_paths_test.go` observed the same synthetic
canary through five distinct supported paths (`go test ./internal/toolharness -run
TestDisclosureContentPathsObservation -count=1 -v`):

| Path | How content reaches the model | Observed |
| --- | --- | --- |
| `read_file` | Direct file body | `model_visible=true` |
| `search_files` | Matching lines, including file content | `model_visible=true` |
| `copy_file` + `read_file` | Copy a credential file to an ordinary name, then read it | `model_visible=true` |
| `git_diff` | Tracked-file diff body | `model_visible=true` |
| `run_command` with a SAFE `git show :path` | Command stdout | `model_visible=true` |

The same test's positive controls confirm ordinary source content remains visible
through `read_file` and `search_files`, and its audit assertion confirms the harness
audit records the call but not the result body. No real credential was used.

## Existing controls and precedent

- `internal/handoff.Redact` removes exact secret-named environment values.
- `internal/redact.ShapesValues` removes recognisable credential shapes with the
  established marker.
- `internal/cli/redaction.go` `redactDiagnosticText` composes those two functions
  for diagnostic text; it is not applied to ordinary tool results.
- `internal/cli/review_existing.go` `isSensitiveReviewPath` already excludes a
  conservative credential-path vocabulary from **review context only**. It is not
  applied to `read_file`, `search_files`, `copy_file`, `git_diff`, or commands.
- `docs/specs/SECURITY.md` §5 states that SOP MUST NOT expose secrets to agent
  prompts unnecessarily. That is a principle, not the specific decision required
  here: it does not define credential-file paths, tool-result redaction,
  command-output coverage, or the compatibility rule.
- `internal/toolharness` is the single tool execution boundary; `internal/ollamaagent`
  `toolResultMessage` is the single rendering boundary before a tool result becomes a
  model message.
- `commandpolicy.Classify` treats `git diff`, `git show`, `git grep`, `git log`, and
  `git cat-file` as SAFE. `cat` is not SAFE and is denied unless an approval exists.

There is **no recorded decision** authorizing a general credential-file deny rule or
ordinary tool-result redaction. The active Plan 08B `CONTEXT-005` task deliberately
records the tool-output `.env` canary as a `GAP` with options and says: do not
implement a new confidentiality policy or general DLP without that decision.

## Policy options

### A. Redact every model-bound tool result (recommended primary)

Apply the existing composition — `handoff.Redact` for known secret-named
environment values, then `redact.ShapesValues` for credential shapes — to every tool
result before it is appended as a model message. Use the existing `[redacted]`
marker. Apply it at the shared `toolResultMessage` boundary used by both
`internal/ollamaagent/harness.go` and `internal/ollamaagent/orchestrate.go`.

Coverage: `read_file`, `search_files`, `git_diff`, `run_command` stdout/stderr,
errors, and any future tool that returns content.

Advantages: one boundary covers every content path; no new permission model; uses the
existing shared detector; ordinary source content survives; command review workflows
(`git diff`, `git show`, `git grep`) keep working.

Limits: shape matching is not general DLP. An unknown-format secret or arbitrary
confidential prose can still pass. It may over-redact source or test fixtures that
look like credentials. It does not protect persisted model output artifacts or
provider-side retention.

### B. Deny known credential paths for direct content tools (recommended complement)

Extract the existing `isSensitiveReviewPath` vocabulary into a shared package and
apply a **narrow tool-read profile** at the harness for:

- `read_file` target;
- `search_files` starting path (and skip matching files below a credential path);
- `copy_file` source (otherwise the copy-then-read path bypasses a read-only rule).

The narrow profile should hard-deny the well-known credential files and directories
where a false positive is acceptable: `.env` (except `.env.example`/`.env.sample`),
`.npmrc`, `.netrc`, `.git-credentials`, `credentials*`, `id_rsa*`/`id_dsa*`/
`id_ecdsa*`/`id_ed25519*`, `.aws/`, `.ssh/`, `.kube/`, `.gnupg/`, and key-material
extensions (`.pem`, `.key`, `.p12`, `.pfx`, `.keystore`, `.jks`).

Do **not** silently reuse the broader review heuristics (`secrets/` directories,
`secret*` source names) for general tool reads without the operator accepting the
compatibility cost: the review exclusion table currently rejects
`internal/secrets/manager.go` and `secrets.yaml`, which may be ordinary source.

Advantages: prevents arbitrary secrets in known credential files from reaching the
model, not just credential-shaped strings. Ordinary source-file access is preserved.

Limits: filename-based; misses a hardcoded key in an ordinary source or config file.
Path filtering does not cover command output or a diff body unless the tool filters
individual paths. It adds a deterministic denial, not an approval gate.

### C. Restrict content-printing commands (not recommended first)

Reclassify `git show`, `git grep`, `git diff`, `git log -p`, and `git cat-file` as
`REQUIRES_APPROVAL`, or inspect command arguments for credential paths.

This would cover command output more strongly but breaks ordinary review workflows and
is brittle (a secret can live in an ordinary path; a command can print it indirectly).
It should be a separate decision if A+B are insufficient.

### D. Accept the current gap

Leave repository and tool content model-visible. This preserves full compatibility but
does not address the demonstrated gap. It is the current disposition, not a fix.

### E. Redact run artifacts (separate decision)

`implementation.md`/`result.md` can persist model output verbatim. This proposal is
about **model input**, not artifact redaction. Changing artifact persistence is a
separate sink decision and is not included in A+B.

## Recommendation

Adopt **A + B** as one narrow, existing-boundary policy:

1. Redact known environment secret values and credential shapes from every
   model-bound tool result, using the existing composition and `[redacted]` marker.
2. Deny direct reads/searches/copy sources under the narrow credential-path profile,
   reusing the existing review vocabulary in a shared package.
3. Keep `commandpolicy` unchanged. Command output is covered by A only; do not claim
   path-based protection for it.
4. Keep run artifacts verbatim; do not claim artifact protection.
5. Keep retrieval, handoff capsules, `SOP_TASK_INPUTS`, provider retention, and
   notification prose as separate sinks with their existing limits.

### Why A+B

- A is the only practical way to cover command output and future tools without
  breaking normal `git` review commands.
- B is necessary for arbitrary secrets in known credential files that the shape
  detector would miss.
- Neither changes provider authentication, model routing, security mode, required
  categories, approval gates, or the sandbox posture.
- Both reuse existing code: `handoff.Redact`, `redact.ShapesValues`, and the
  `isSensitiveReviewPath` vocabulary (extracted, not duplicated).

## Compatibility impact

- **Ordinary source access is preserved.** Positive controls must prove ordinary
  `read_file` and `search_files` content still reaches the model.
- **Credential-file access becomes a deterministic denial.** Tasks that legitimately
  inspect `.env` or private keys will fail with a clear path-specific refusal. The
  operator can use a non-model workflow for that inspection. `.env.example` and
  `.env.sample` remain readable.
- **Some source/test text may be over-redacted.** The model sees `[redacted]` in place
  of credential-shaped strings. Tasks needing exact credential bytes cannot rely on
  the model.
- **Command output remains partially covered.** A SAFE command that prints an
  unknown-format secret can still expose it. If the operator requires stronger
  command-output protection, option C is a separate, higher-impact decision.
- **No new service, evaluator, DLP layer, approval step, or model route** is required.
- **Performance** is bounded by the existing tool-output limits and a small
  deterministic detector pass per result.

## Coverage limits and non-claims

- Not general DLP. Arbitrary confidential prose, unknown token formats, and
  non-credential secrets are not reliably detected.
- Not prompt secrecy. The system/developer prompt and tool schema remain disclosable.
- Not artifact protection. Model output remains persisted verbatim.
- Not retrieval protection. A retrieved record in an authorized scope can still carry
  content; retrieval scope is Plan 09's evidence.
- Not command-path protection. `git show`/`git grep`/`git diff`/`git log`/`git cat-file`
  remain SAFE; A redacts only what the detector recognises.
- Not provider retention or enterprise DLP. Those remain provider-owned.
- `copy_file` must be covered as a source path; otherwise the copy-then-read path
  defeats a read-only rule.

## Test plan for the authorized implementation

The baseline `TestDisclosureContentPathsObservation` currently asserts the gap
(canary visible through all five paths). The authorized implementation must invert
those assertions to require blocking/redaction and keep the ordinary-source positive
controls.

1. **Detector positive control:** a deliberately exposed synthetic `sk-…` canary is
   replaced with `[redacted]`; ordinary text survives.
2. **Model-input paths:** synthetic canaries are blocked or redacted through
   `read_file`, `search_files`, `copy_file`+`read_file`, `git_diff`, and
   `run_command` `git show`.
3. **Positive controls:** ordinary source files remain readable and searchable;
   `.env.example`/`.env.sample` are not denied; a non-secret Slack/webhook payload
   remains intact if notification behavior is touched (it is not in this policy).
4. **Separate measurements:** assert model visibility, harness-audit persistence, and
   run-artifact persistence independently. A false `in_artifacts` must not be reported
   as artifact protection.
5. **No real credentials:** disposable synthetic fixtures only; no external receiver.
6. **Required gates:** focused package tests, `go vet ./...`, `go build ./...`,
   `go test ./... -count=1`, and the repository pre-push link/format check.

## Decision required

The operator/security owner must choose one:

- **A+B (recommended):** redact every model-bound tool result and deny direct
  credential-path reads/searches/copy sources with the narrow profile.
- **A only:** tool-result redaction, no path denial. Lower compatibility impact,
  higher residual risk for arbitrary secrets in credential files.
- **B only:** credential-path denial, no tool-result redaction. Direct reads are
  protected, but command output and diffs remain exposed.
- **Accept the gap:** no change; record the residual risk explicitly.
- **C or E:** request the separate command-output or artifact-redaction decision.

Until that decision is recorded, no executable policy change is authorized. The
baseline observation test and this proposal are evidence for the decision, not
acceptance of the risk.

## SOP governance note

The active Plan 08B worktree has `CONTEXT-004`, `CONTEXT-005`, and `CONTEXT-006`
`LOCAL_DONE`; `CONTEXT-007` is `PLANNED` and parked at a `NEEDS_HUMAN` approval gate
(`approval.json`, kind `NEEDS_HUMAN`, reason: required manual acceptance evidence for
`criterion-3` is missing). That gate is about Plan 08B's bounded-claim acceptance, not
this policy. This proposal does not modify that worktree, approve that gate, create a
new SOP plan, or edit state/archives. The policy implementation should be scheduled
only after the operator records the decision above.
