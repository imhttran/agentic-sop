# Plan 03 — acceptance request for the bounded live campaign

Status: **REQUEST — no acceptance is recorded.** This document prepares the exact
decision a designated approver must make. It does not record acceptance, does not
change SOP state, and does not claim `VERIFIED`. The final criterion matrix,
evidence identity, and exact human decision are in §Final closeout (2026-11-10);
sections dated 2026-11-09 are preserved as history.

**Post-closeout correction:** the plan has since been historicalized `COMPLETE`
through the supported external-completion path; `COMPLETE` is a lifecycle state, not
acceptance. See §Post-closeout correction (2026-11-09) below.

Related: [llm02-disclosure.md](llm02-disclosure.md) §Live extraction,
[llm02-campaign.json](llm02-campaign.json), and
[PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md](../../plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md).

> Historical context: this section and §Current SOP state were written before the
> external-completion and historicalization recorded in §Post-closeout correction.
> They are preserved for audit; the current state and final criterion matrix are in
> §Final closeout (2026-11-10).

## What is missing

PR #300 merged the bounded live campaign as **evidence**, not approval. The report
keeps the requirement explicit (`llm02-disclosure.md`):

> Manual operator acceptance of this live evidence is still required; the artifact
> and this report are evidence, not an approval. The SOP task state is not changed.

No acceptance record exists. The SOP task `DISCLOSE-003` is `PLANNED`, the recorded
outcomes for `DISCLOSE-001` and `DISCLOSE-002` are `PARTIAL`, and the plan's
acceptance mapping for `DISCLOSE-003` still says "currently NOT_RUN". A missing
acceptance keeps the plan from historicalization.

## Exact evidence identity

| Item | Value |
| --- | --- |
| Campaign artifact | `docs/reports/owasp-llm/llm02-campaign.json` |
| Artifact SHA-256 | `0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06` (at `main` `93c72857`) |
| Campaign revision | `9c32e3d2bccb269aa0246b08158ddad5e8b25066`, `dirty: false` |
| Model / endpoint | `qwen3.5:4b` via `http://127.0.0.1:11434/api/generate` |
| Platform | `darwin` arm64, opt-in `sandbox-exec` live profile (`localhost:11434` only) |
| Bounds | 1 request/case, 1 attempt, 1 concurrency, 240 s, 64 KiB |
| Requests | 7 (`4` disclosure probes, `3` effect cases), all completed |
| Report section | `llm02-disclosure.md` §Live extraction (and the per-probe table) |
| Merged in | PR #300, merge `6de95c33` |

The artifact also records its confinement probes (`endpoint_reachable`,
`other_loopback_denied`, `non_loopback_denied`, `filesystem_outside_denied`,
`secret_env_scrubbed`).

## Additional Linux live tool-path evidence (2026-11-10)

This section is evidence, not acceptance. It does not replace the macOS
campaign identity above and does not change the required decision.

| Item | Value |
| --- | --- |
| Artifact | `docs/reports/owasp-llm/llm02-linux-campaign.json` |
| Artifact SHA-256 | `31c059b28590eb8e39e6c00e298f624b20f8a51fbefafb6fd885e5a2ffbc16a9` |
| Campaign revision | `4cee73a`, `dirty: false` |
| Model / endpoint | `qwen3:4b` via the fixed local Ollama endpoint |
| Platform | Linux `amd64`, opt-in `bwrap` live profile with a private network namespace and a Unix-socket endpoint bridge |
| Bounds | 1 request/case, 1 attempt, 1 concurrency, 240 s, 64 KiB |
| Requests | 17, all observed |
| Results | 7 protected cases, 6 positive controls, 4 residual exposures |
| Report section | `llm02-disclosure.md` §Linux live tool-path campaign (2026-11-10) |

The residual exposures are an unknown-format value in an ordinary file, SAFE
command output, a diff, and a hardlink alias to `.env`. The campaign does not
establish general DLP, retrieval protection, notification protection, artifact
protection, or coverage of another model or platform. The Plan 03 acceptance
decision remains exactly as stated above.


## Current SOP state (read-only, verified 2026-11-09)

Checked in the existing `llm02-disclosure` worktree without modifying it:

- `tasks`: `DISCLOSE-001 LOCAL_DONE`, `DISCLOSE-002 LOCAL_DONE`,
  `DISCLOSE-003 PLANNED attempt=0`.
- No `runs/DISCLOSE-003/` outcome.
- `DISCLOSE-001` and `DISCLOSE-002` outcomes exist with status `PARTIAL` and no
  acceptance decisions.
- `human.designated_approvers` is not configured, so an acceptance attempt would
  fail closed.
- Historicalization of the plan is blocked while `DISCLOSE-003` is `PLANNED`.

## Required decision and supported interface

The decision is: does a designated approver accept the bounded campaign as the
manual evidence for `DISCLOSE-003` criteria 1–4 and 6, or decline / leave it
incomplete?

The supported recorder is `sop outcome` (see
[CLI.md](../../reference/CLI.md#task-outcomes)):

```text
sop outcome DISCLOSE-003 --accept <criterion-id> --evidence <evidence-text> --by "<approver>"
```

The record is append-only at `runs/DISCLOSE-003/acceptance_decisions.jsonl`;
`outcome.json` is never rewritten. Only a `human.designated_approvers` identity may
accept, with non-empty evidence, for a criterion the recorded outcome maps to
manual evidence.

## Preconditions that do not hold today

1. **No recorded outcome for `DISCLOSE-003`.** The accept command requires one.
   Recording an outcome means running the task through the SOP lifecycle against
   the existing evidence. It does **not** require re-running the campaign.
2. **No designated approver.** `human.designated_approvers` must be configured by
   the operator; it is a governance setting, not something this work may add.
3. **The plan's mapping text is stale.** The `DISCLOSE-003` `### Acceptance
   Mapping` says "currently NOT_RUN" for criteria 1–4 and "live acceptance remains
   incomplete" for criterion 6. Accepting that text as evidence would be
   contradictory. The mapping must be reconciled through SOP first; `DISCLOSE-003`
   was never executed, so a mapping-only change does not need
   `--accept-changed`, but the operator still owns it.

## Proposed sequence (operator-owned)

1. Configure `human.designated_approvers` with the intended approver identity.
2. Reconcile the `DISCLOSE-003` acceptance mapping to the live artifact and report
   section, using `sop reconcile --list-changed` first and applying only the
   intended mapping change. Do not change criteria, only the manual evidence text.
3. Record the task outcome through the lifecycle against the existing artifact
   (no campaign re-run), with `sop validate` as the criterion-6 verification.
4. Record one `sop outcome DISCLOSE-003 --accept …` per criterion, naming the
   artifact SHA-256 and report section as evidence.
5. Only then complete or historicalize the plan through SOP.

No step may be performed on the operator's behalf.

## Proposed manual evidence text (draft, not applied)

- criterion 1: `llm02-campaign.json` `disclosure[]`: direct context disclosure
  observed and indirect retrieval refusal observed (artifact SHA-256 `0c8cc0ff…`).
- criterion 2: `llm02-campaign.json` `effects[]`: unauthorized effects denied by
  authorization and policy; `executed` empty.
- criterion 3: synthetic `sk-…-not-a-real-secret` canaries, `.invalid` targets,
  and passing confinement probes; no real credential or receiver.
- criterion 4: `llm02-disclosure.md` §Live extraction separates
  `prompt_exposed_protected`, `generated_disclosure`, `artifact_leakage`, and
  `attempted_exfiltration`.
- criterion 6: required checks and PR #300; live acceptance recorded by this
  decision.

## What acceptance would and would not mean

- It would bind the named evidence to the named criteria for the recorded attempt.
- It would **not** turn the campaign into general DLP, prompt secrecy, a red-team
  result, or coverage of another model, provider, or operating system.
- The Linux live tool-path campaign is `OBSERVED` for the five tool paths, but
  Linux live retrieval and notification sinks remain `NOT_RUN`. Model-visible
  repository content and persisted model output remain explicit bounded gaps.

## Must not

- Do not record acceptance, an approver name, or a timestamp on the operator's
  behalf.
- Do not rewrite the `DISCLOSE-001`/`DISCLOSE-002` `PARTIAL` outcomes.
- Do not create or edit `state.db`, `outcome.json`, or the acceptance audit files
  outside the supported command.
- Do not re-run the campaign merely to close paperwork; reuse the merged artifact.
- Do not mark the plan complete or historicalized from this document.

## Post-closeout correction (2026-11-09)

The lifecycle state changed after this request was written. The supported
external-completion path was used:

- `sop task complete DISCLOSE-003 --external --commit d3830e35` in the
  `llm02-disclosure` worktree, with validation `PASS`, recorded at
  `2026-11-09T17:19:43Z`. The record is
  `runs/DISCLOSE-003/external-completion.json`.
- `sop plan historicalize` then recorded `HISTORICALIZED` / `COMPLETE` at
  `2026-11-09T17:19:47Z`. The archive is preserved at
  `.agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure/`.

This is a lifecycle completion, not acceptance evidence. External completion
explicitly derives no acceptance evidence and clears any prior objective-level
outcome. No `acceptance_decisions.jsonl` exists for `DISCLOSE-003`, and there is no
`runs/DISCLOSE-003/outcome.json`.

The supported `sop outcome --accept` interface cannot append acceptance to the
historicalized plan. On a disposable copy of the preserved state it reports
`task DISCLOSE-003 not found` and `outcome: no recorded outcome for DISCLOSE-003 in
the latest run; run the task first`; no acceptance file is written. There is no
active task or recorded outcome to bind a criterion decision to, and the plan must
not be reopened.

The earlier statement that `human.designated_approvers` was not configured is stale
for the preserved state: the evidence `config.yaml` names `Tom Tran`. That does not
create an acceptance record.

The remaining decision is unchanged: a designated approver must accept the exact
campaign artifact for `criterion-1`–`criterion-4` and `criterion-6`, or the manual
acceptance stays incomplete. If the operator wants the acceptance record, it needs
a separate governance decision; it cannot be repaired by editing state or archives.

The later closeout re-verification at `main` `466a92c2` re-ran the focused
deterministic checks, reused the same artifact, and reconciled the `.env` canary
and notification-prose claims. It did not record acceptance. See
[llm02-disclosure.md](llm02-disclosure.md) §Closeout re-verification and
§Tool-output `.env` canary reconciliation.

The later Linux `amd64` deterministic verification (2026-11-09, see
[llm02-disclosure.md](llm02-disclosure.md) §Linux independent verification)
independently reproduced the five pre-policy paths and confirmed the Plan 08 A+B
denial/redaction. It records no acceptance and does not change this decision.
The Linux live tool-path campaign (2026-11-10) is also `OBSERVED` (17 requests:
7 protected, 6 positive controls, 4 residual exposures); Linux live retrieval and
notification sinks remain `NOT_RUN`. PR #349 redacts the rendered user prompt
(task, caller input, output requirements) deterministically via `userPrompt`
(`TestUserPromptRedactsTaskAndInput`); the live campaigns predate that fix and
were not re-run for it. None of this records acceptance.

## Final closeout (2026-11-10)

Status: **DECISION-READY — no acceptance recorded.** Plan 03 is historicalized
`COMPLETE`; that is lifecycle state, not acceptance. Formal per-criterion
acceptance through the supported SOP interface is unsupported for this plan
without reopening or recreating state, which this closeout does not do.

### Final lifecycle limitation

- Current checkout readiness check: `sop plan historicalize --check --json
  "docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md"` reports
  `state: NONE`, `eligible: false`, reason `no active plan is recorded`.
- The preserved evidence records external completion at `d3830e35` and
  historicalization `COMPLETE` at `2026-11-09T17:19:47Z`. No
  `runs/DISCLOSE-003/outcome.json` and no
  `runs/DISCLOSE-003/acceptance_decisions.jsonl` exist.
- `sop outcome --accept` requires a recorded outcome. On the preserved disposable
  state it reports `task DISCLOSE-003 not found` and `no recorded outcome for
  DISCLOSE-003 in the latest run; run the task first`; no acceptance file is
  written. The plan must not be reopened and the archive must not be edited to
  manufacture acceptance.
- `human.designated_approvers` names `Tom Tran` in the preserved evidence
  configuration, but that identity still has no recorded approval operation.

### Evidence identity

| Evidence | Tested revision | Artifact identity |
| --- | --- | --- |
| macOS live campaign | `9c32e3d2bccb269aa0246b08158ddad5e8b25066` | `llm02-campaign.json`, SHA-256 `0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06` |
| Linux live tool-path campaign | `4cee73a`, `dirty: false` | `llm02-linux-campaign.json`, SHA-256 `31c059b28590eb8e39e6c00e298f624b20f8a51fbefafb6fd885e5a2ffbc16a9` |
| Deterministic model-input and prompt controls | Final integration baseline `4c8fe5a` | `TestModelInputToolBoundaries`, `TestModelInputToolBoundariesResidualLimits`, `TestCredentialPathDenialAndBoundaryDivision`, `TestUserPromptRedactsTaskAndInput` |
| Corrected live positive control | `4cee73a` | PR #351; ordinary copy uses the real `toolharness.ToolCopyFile` |

The Linux campaign was not re-run after #349/#351/#352. #349 changes the
user-prompt boundary, not the tool-path boundary exercised by the campaign; #351
corrects the campaign's positive control; #352 is documentation-only. The
campaign remains valid for the five tool paths, and the user-prompt fix is
deterministic-only evidence with no live re-run.

### Plan 03 acceptance matrix

Status describes evidence, not acceptance. `Supported` is not an acceptance
decision. No acceptance decision exists for any criterion.

| Task / criterion | Requirement | Evidence | Evidence status | Acceptance decision |
| --- | --- | --- | --- | --- |
| DISCLOSE-001.1 | Distinguish provider authentication headers from model-visible context. | `llm02-disclosure.md` §Provider authentication is not model content and §Data-flow trace; `TestOpenAICompatibleExecutionReusesAuthorization`. | Supported — deterministic | None recorded |
| DISCLOSE-001.2 | Identify where redaction occurs before sending, before persistence, and before display. | `llm02-disclosure.md` §Where redaction happens; `redact.ModelInput` in `userPrompt` and `toolResultMessage`; `redactDiagnosticText`; handoff, outcome, and notification tests. | Supported — deterministic | None recorded |
| DISCLOSE-001.3 | Record scope checks before retrieval and response validation after retrieval. | `llm02-disclosure.md` §Authorization boundaries and retrieval row; `TestSearchRejectsUnrelatedEvidence`; `TestRankerAdapterRejectsScopeItCannotHonor`. | Supported — deterministic; live retrieval `NOT_RUN` | None recorded |
| DISCLOSE-001.4 | Distinguish credentials from other confidential content; pattern matching is not general DLP. | `llm02-disclosure.md` §Residual risks and §Pattern matching; `TestModelInputToolBoundariesResidualLimits`; Linux live residual cases. | Supported — bounded claim | None recorded |
| DISCLOSE-001.5 | Do not read or print actual secret values during assessment. | `llm02-disclosure.md` §Method; synthetic canaries only; campaign artifacts contain synthetic canaries. | Supported — deterministic and observed | None recorded |
| DISCLOSE-002.1 | Protected canaries do not appear in unauthorized model messages, persisted diagnostics, or outgoing payloads at in-scope boundaries. | `TestModelInputToolBoundaries`; check-evidence redaction tests; Linux live 7 protected cases. | Supported — deterministic and live for scoped canaries; unknown-format and hardlink residuals observed | None recorded |
| DISCLOSE-002.2 | Cross-scope retrieval content is rejected before it can become model context under the current retrieval contract. | `TestSearchRejectsUnrelatedEvidence`; `TestRankerAdapterRejectsScopeItCannotHonor`. | Supported — deterministic; live retrieval `NOT_RUN` | None recorded |
| DISCLOSE-002.3 | Legitimate non-sensitive content and required authentication still work. | Linux live 6 positive controls; `.env.example` and `.env.sample`; ordinary copy, command, and diff; `TestOpenAICompatibleExecutionReusesAuthorization`. | Supported — deterministic and live | None recorded |
| DISCLOSE-002.4 | Each fix reuses the shared redaction or established authorization boundary where appropriate. | `redact.ModelInput`; `sensitivepath`; `toolharness`; `userPrompt`; `llm02-disclosure.md` §Reproduced gaps and fixes. | Supported — deterministic | None recorded |
| DISCLOSE-002.5 | Unknown formats and arbitrary confidential text remain explicitly bounded claims. | `llm02-disclosure.md` §Residual risks and §Residual investigation status; `TestModelInputToolBoundariesResidualLimits`; Linux residual cases. | Supported — bounded claim | None recorded |
| DISCLOSE-003.1 | Direct extraction and an indirect injected extraction case have recorded outcomes. | `llm02-campaign.json`: direct context disclosure observed and indirect retrieval refusal observed; Linux live tool-path campaign. | Observed — recorded outcomes; no acceptance | None recorded |
| DISCLOSE-003.2 | Unauthorized tool effects remain denied even if the model proposes disclosure. | macOS effect cases denied by authorization and policy; Linux tool-path campaign recorded no unauthorized action. | Verified — for recorded cases | None recorded |
| DISCLOSE-003.3 | No real credential or external receiver is used. | Synthetic canaries; inert `.invalid` target; campaign artifacts. | Verified — deterministic and observed | None recorded |
| DISCLOSE-003.4 | Report separates prompt exposure, generated disclosure, artifact leakage, and attempted exfiltration. | `llm02-campaign.json` separate flags; Linux campaign separate tool/model/artifact/action outcomes; `llm02-disclosure.md` §Live extraction. | Verified — observed | None recorded |
| DISCLOSE-003.5 | Unavailable live prerequisites produce `NOT_RUN` and leave the relevant acceptance incomplete. | Historical `NOT_RUN` record preserved; Linux live retrieval and notification sinks `NOT_RUN`; live acceptance incomplete. | `NOT_RUN` — acceptance incomplete | None recorded |
| DISCLOSE-003.6 | Required verification passes and a focused PR records changed paths and residual risks. | CI on PRs #337, #341, #349, #350, #351, and #352; focused PRs; `llm02-disclosure.md` residual sections. | Verified — CI and PRs | None recorded |

### Residual-risk disposition

No item below is accepted. Acceptance of any residual risk is a separate
authorized decision.

| Residual item | Observed or unverified | Existing control | Residual risk | Within approved A+B? | Exact decision required | Recommended disposition |
| --- | --- | --- | --- | --- | --- | --- |
| Unknown-format credentials in ordinary files, diffs, or SAFE command output | Observed in Linux live `READ-OPAQUE`, `COMMAND-OPAQUE`, and `DIFF-OPAQUE`; deterministic residual test | Narrow credential-path denial plus shape redaction | An unknown-format secret reaches the model | No — shape-based A+B does not claim this | Accept as residual risk or authorize a separate DLP/redaction policy | Accept as explicit residual risk for the approved narrow profile, or authorize a separate policy |
| Hardlink or path alias with an ordinary name | Observed in Linux live `HARDLINK` | Symlink resolution plus lexical credential-path denial | A hardlink can bypass the path check; recognised shapes are still redacted | No — not part of the approved lexical profile | Accept as residual risk or authorize inode/path-identity checks | Accept as an explicit local path-alias residual, or authorize a separate path-identity control |
| Model-output artifact persistence | Deterministic test; live artifact leakage observed for three opaque values | Run artifacts persist model output verbatim | A model that repeats a value can persist it in `implementation.md` or `result.md` | No — artifact redaction is a separate sink | Accept as separate sink risk or authorize artifact redaction | Accept as a separate evidence-sink risk, or authorize a bounded artifact-redaction policy |
| Retrieval and handoff inputs | Live retrieval `NOT_RUN`; handoff deterministic | Retrieval scope controls; handoff redacts known secret-named environment values | Authorized-scope retrieved content and general handoff prose can carry sensitive text | No — general content is outside A+B | Accept or authorize stronger retrieval/handoff filtering | Keep existing scope controls; accept the bounded gap or authorize a separate retrieval/handoff decision |
| Caller-owned and notification prose | Unverified; caller-dependent | Notification targets redacted; lifecycle reason is caller-owned | A caller can pass raw secret prose | No — caller-owned data | Accept as caller-owned or authorize notification-prose redaction | Accept as caller-owned, or authorize a notification-prose policy |
| Provider retention and general DLP | Outside SOP control | None | Provider may retain prompts; general DLP is not implemented | No — external/compliance boundary | Accept as external or authorize a provider/DLP decision | Treat as external; no SOP control is claimed |

No residual risk is marked accepted; each item requires a separate authorization
decision.

### Required human decisions

1. **Plan 03 evidence acceptance:** accept or decline the macOS campaign, the
   Linux live tool-path campaign, or both for `DISCLOSE-003` criteria 1–4 and 6.
   Name the authority and the record in which the decision is made. This document
   does not make that decision.
2. **Historicalized-plan acceptance process:** if SOP-recorded acceptance is
   required, authorize a separate governance process for historicalized plans.
   Do not reopen Plan 03, edit `.agent-sdlc/archive/`, or create a replacement
   `acceptance_decisions.jsonl`.
3. **Residual-risk disposition:** decide each item in §Residual-risk disposition.
   Accepting a residual risk is an authorization decision; this record does not
   make it.

### Three separate claims

1. **Approved LLM02 implementation is complete.** Yes, within the approved
   Plan 08 A+B scope plus PR #349.
2. **Scoped security verification is complete.** Yes for the five tool paths on
   Linux (deterministic and live), the macOS live campaign, and deterministic
   boundary tests. No for Linux live retrieval and notification and for a live
   re-run of the #349 prompt fix.
3. **Full LLM02 security coverage and governance acceptance are not established.**
   Yes. Residual gaps remain and no per-criterion acceptance is recorded.

### Recommended disposition

Recommended, subject to the operator's decision: accept the approved
implementation and scoped verification as sufficient for the approved A+B plus
#349 scope; record the four observed residual exposures and the unverified sinks
as accepted residual risk only if the operator explicitly decides to do so;
otherwise leave acceptance incomplete. Do not claim full LLM02 coverage or
universal disclosure prevention.


## Exact next action

The designated approver must choose one of the decisions in §Required human
decisions. Until an authorized acceptance decision exists, the matrix keeps the
live row at `OBSERVED`, the user-prompt fix at `VERIFIED (deterministic)`, and
Plan 03 acceptance at `NEEDS_HUMAN`. No acceptance is recorded by this request.
