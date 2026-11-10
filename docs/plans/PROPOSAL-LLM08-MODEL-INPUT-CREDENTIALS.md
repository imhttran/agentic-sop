# Proposal — LLM08 model-input credential policy

Status: **APPROVED and IMPLEMENTED — A+B.** Decision memory
`f891d5cbe5dd578addbba83ee95b47c35752970219b4d0ad6d3fc2be254a741c`, recorded at
`origin/main` `814f69c8`. Implementation branch:
`security/llm08-model-input-credentials`. This file remains the decision record; it
does not accept residual risks or authorize artifact/notification changes.
Date: 2026-11-09 (proposal); 2026-11-10 (decision and implementation).
Baseline: `origin/main` at `c87e45f4d3777756ba7bd9bd8f62fd9b0cb3aaad` for the
proposal; the decision was recorded at `814f69c8`.
Platform: macOS arm64; Go 1.27.1. Configuration: `security.mode: report-only`,
`security.evaluation.enabled: false`, `advisory_retrieval.enabled: false`,
`agent.provider: ollama`.

Related evidence: Plan 03 [llm02-disclosure.md](../reports/owasp-llm/llm02-disclosure.md)
§Tool-output `.env` canary reconciliation; Plan 08
[llm08-hidden-context.md](../reports/owasp-llm/llm08-hidden-context.md); the active
Plan 08B `CONTEXT-005` GAP record (uncommitted in its own worktree); and the current
Plan 08 matrix row.

## Decision record

The operator approved A+B in the implementation thread. The authorization was
recorded with the supported decision-memory command (`sop memory add`) at
`814f69c8`, scope `repository`, source
`docs/plans/PROPOSAL-LLM08-MODEL-INPUT-CREDENTIALS.md`, decision ID
`f891d5cbe5dd578addbba83ee95b47c35752970219b4d0ad6d3fc2be254a741c`.

Approved scope:

1. Redact every model-bound tool result, including error text, with
   `redact.ModelInput` at the single `toolResultMessage` boundary.
2. Deny direct `read_file`, `search_files`, and `copy_file` source paths under the
   narrow credential-path profile, preserving `.env.example`/`.env.sample`.
3. Leave `commandpolicy` unchanged.

The authorization does not accept residual risks, resolve Plan 03 acceptance,
authorize artifact or notification changes, or authorize merge.

## Problem

At the pre-policy baseline, SOP let a model read credential-bearing repository
content through several supported tool paths. The previously reported `.env` canary result is
confirmed:

- `model_visible=true` through the real `read_file` path.
- The harness audit does not persist tool result content (`in_artifacts=false` for
  the audit), but that is not general artifact protection.
- `implementation.md` and `result.md` persist model output verbatim, so a model that
  repeats the tool result can still put the value in a run artifact.

A read_file-only fix is not comprehensive. The pre-policy observation (now
converted to enforcement tests) showed the same synthetic canary through five
distinct supported paths:

| Path | How content reaches the model | Observed |
| --- | --- | --- |
| `read_file` | Direct file body | `model_visible=true` |
| `search_files` | Matching lines, including file content | `model_visible=true` |
| `copy_file` + `read_file` | Copy a credential file to an ordinary name, then read it | `model_visible=true` |
| `git_diff` | Tracked-file diff body | `model_visible=true` |
| `run_command` with a SAFE `git show :path` | Command stdout | `model_visible=true` |

The pre-policy positive controls confirmed ordinary source content remained visible
through `read_file` and `search_files`, and the audit assertion confirmed the harness
audit records the call but not the result body. No real credential was used. The
enforcement tests now assert denial or redaction for each path.

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

At the proposal stage there was **no recorded decision** authorizing a
credential-file deny rule or ordinary tool-result redaction. The Plan 08B
`CONTEXT-005` task deliberately recorded the tool-output `.env` canary as a `GAP` and
required an operator confidentiality-policy decision before implementation. The
operator has since approved A+B; §Decision record and §Decision recorded below are the
satisfaction of that gate.

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

## Implemented tests and evidence

The pre-policy observation test was converted to enforcement tests:

- `TestCredentialPathDenialAndBoundaryDivision` (`internal/toolharness`) proves
  direct `read_file`, `search_files`, and `copy_file` source denial, template and
  ordinary-source positive controls, symlink resolution, and the boundary division
  for `git_diff`/command output.
- `TestModelInputToolBoundaries` (`internal/ollamaagent`) proves model-bound
  redaction for `git_diff` and SAFE command output, denial messages for the three
  path-denied tools, an ordinary-source control, and a redacted denial error.
- `TestModelInputRedactsKnownEnvAndShapes` (`internal/redact`) is the detector
  positive control.
- `TestToolOutputEnvCanaryDenied` (`internal/cli`) proves the `.env` denial and the
  audit separation.
- `TestRunArtifactPersistsModelOutputVerbatim` preserves the separate artifact
  persistence limit.

Observed on Darwin arm64, Go 1.27.1: all passed; the path-denial cases returned
`ErrSensitivePath`, the diff/command cases contained `[redacted]` instead of the
synthetic value, and ordinary source, `.env.example`, and `.env.sample` remained
readable. The full `go vet ./...`, `go build ./...`, `go test ./... -count=1`, and
pre-push checks passed.

## Decision recorded

A+B was approved and recorded as described in §Decision record. The alternatives
(A-only, B-only, accept the gap, or a separate command/artifact decision) were not
selected. The residual limits remain explicit in the Plan 08 report and §Coverage
limits; they are not accepted by this decision.

## SOP governance note

The owning Plan 08B worktree advanced to `091ae029` with `CONTEXT-004`,
`CONTEXT-005`, `CONTEXT-006`, and `CONTEXT-007` `LOCAL_DONE`. Its `CONTEXT-007`
approval records were inconsistent at the last read (`approval.json` DECLINED while
the latest `approval-history.json` entry was PENDING). That worktree is being driven
concurrently and was not modified or approved by this implementation. The policy
authorization is recorded in the implementation worktree's decision memory; it is not
Plan 08B acceptance, Plan 03 acceptance, or a merge authorization.
