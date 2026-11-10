# Plan 03 — acceptance request for the bounded live campaign

Status: **REQUEST — no acceptance is recorded.** This document prepares the exact
decision a designated approver must make. It does not record acceptance, does not
change SOP state, and does not claim `VERIFIED`.

**Post-closeout correction:** the plan has since been historicalized `COMPLETE`
through the supported external-completion path; `COMPLETE` is a lifecycle state, not
acceptance. See §Post-closeout correction (2026-11-09) below.

Related: [llm02-disclosure.md](llm02-disclosure.md) §Live extraction,
[llm02-campaign.json](llm02-campaign.json), and
[PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md](../../plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md).

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

## Exact next action

The operator names the designated approver and decides whether to accept the
campaign for `DISCLOSE-003`. Until that record exists, the matrix keeps the live
row at `OBSERVED` and the plan's live acceptance incomplete.
