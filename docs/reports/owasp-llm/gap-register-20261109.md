# OWASP LLM — remaining gap register (2026-11-09)

**Current reconciliation:** this register builds on `main` at `347a96f1`
(2026-11-09 UTC), after PR #316 merged at `1ace57a8` and PR #317 merged at
`347a96f1`. The Plan 10 row was re-verified at `main` `ed9ec2dc` (2026-11-09) on
Darwin arm64 and prepared the live-delivery blocker and acceptance request. The
Plan 10 wrap-up merged in PR #328 as
`35340774c16b10236b0ca0143cc0d07c85a8e6b3`; its full CI passed on the PR head
`eaf34114a5fcfe96088986d8939309c1efc75ffa` (CI
[42718751282](https://github.com/imhttran/agentic-sop/actions/runs/42718751282),
CodeQL
[42718748910](https://github.com/imhttran/agentic-sop/actions/runs/42718748910)).
This follow-up records that merged evidence and does not re-run it. The other
rows keep their recorded revision. The Plan 03 closeout re-verification at
`main` `466a92c2` (2026-11-09) re-ran the focused deterministic checks, reused
the live artifact, and reconciled the `.env` canary and notification-prose
claims. The Plan 08 model-input credential policy was approved as A+B and
implemented on `security/llm08-model-input-credentials` (decision memory
`f891d5cbe5dd578addbba83ee95b47c35752970219b4d0ad6d3fc2be254a741c`): direct credential
paths are denied for `read_file`/`search_files`/`copy_file` source, and every
model-bound tool result and error is redacted. The original point-in-time baseline at
`93c72857` is preserved verbatim in
[gap-register-20261109-baseline.md](gap-register-20261109-baseline.md). That
file is historical evidence; it is not the current disposition.

This register separates what merged from what is still open. **A merged proposal is
not a closed gap.** Implementation or investigation completion and the remaining
policy, trust, or acceptance decision are recorded separately.

## Current dispositions

| #   | Item                                         | Completed implementation or investigation (merge)                                                                                                                                                                                                                                                                                        | Still open                                                                                                                                                                                                                                                         | Retained limitation                                                                                                                                                                                                                                                                         |
| --- | -------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Plan 05 GAP-4 — per-document index integrity | **Fixed:** schema 2 adds per-row digests verified by `repoindex.VerifyRows`; the aggregate identity fix is PR #308 (`bf0d824d`) | Decided: option A of [PROPOSAL-REPOINDEX-ROW-DIGESTS.md](../../plans/PROPOSAL-REPOINDEX-ROW-DIGESTS.md) | No reader loads `context/index.json` yet; digests prove bytes plus resolved rows, not benign content |
| 2   | Plan 05 GAP-5 — RAG `document_hash`          | **Fixed for chunk-hashed evidence:** optional `chunk_hash` is recomputed and a mismatch rejects the response | Decided: option A of [PROPOSAL-RAG-EVIDENCE-HASH.md](../../plans/PROPOSAL-RAG-EVIDENCE-HASH.md) | The producer must emit `chunk_hash`; until it does evidence is unverified. `document_hash` stays unverified; a match proves bytes, not publisher authenticity or benign content |
| 3   | Plan 04 — local scanner integrity            | Option B implemented: optional `binary_sha256` / `rules_sha256` checked before every scanner execution; the repository enables the Semgrep rules pin. Earlier: verification record (PR #311, `c3a8eed8`) | Decided: option B of [PROPOSAL-SCANNER-BINARY-PINS.md](../../plans/PROPOSAL-SCANNER-BINARY-PINS.md) | Binary pins are opt-in and platform-specific (none committed); hash-then-exec race; report-only mode, empty `required_categories`, no publisher authenticity |
| 4   | Plan 01 — macOS isolation evidence           | Boundary fixtures re-run with a loopback positive control; Unix-domain socket denial enabled on Seatbelt; process-group kill recorded (PR #312, `b37f9560`). Plan `plan-01-sop-llm03-excessive-agency` is `HISTORICALIZED` / `COMPLETE` (the supported command confirmed the already-historicalized state; no new transition is claimed) | No open evidence item for the isolated-replay and evaltarget paths. Normal-agent isolation and Linux remain out of scope                                                                                                                                           | macOS arm64 only, and only the isolated-replay and evaltarget paths; normal agent execution is policy-only and not OS-sandboxed; Linux Bubblewrap is `NOT_RUN` here                                                                                                                         |
| 5   | Plan 10 — Slack delivery                     | Focused re-verification at `ed9ec2dc` (Darwin arm64): terminal sink, Slack payload encoder, loopback transport, toolharness, store, and ACP/MCP checks pass. PR #313 (`f557c6fd`) prepared the live test. SOP state for this plan is not available in an owning checkout                                                                 | **Decision:** explicit authorization naming `#sop-alert` and the test window, plus enabling the `webhook` channel with the `slack` payload and confirming the binding. The send is not performed                                                                   | Live delivery and rendering are `NOT_RUN`; 2xx proves endpoint acceptance, not Slack rendering; the default test payload is not an adversarial case; unfurl and client rendering are external; evidence is Linux `x86_64` (original) and Darwin arm64 (wrap-up); manual acceptance is `GAP` |
| 6   | Plan 03 — live-evidence acceptance           | Bounded campaign merged (PR #300, merge `6de95c33`); artifact SHA-256 `0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06`; `DISCLOSE-003` recorded as external completion (`d3830e35`, validation `PASS`, 2026-11-09T17:19:43Z) and plan historicalized `COMPLETE` at 2026-11-09T17:19:47Z; closeout re-verification at `466a92c2` re-ran the focused deterministic checks and reused the live artifact | **Decision:** no per-criterion acceptance is recorded for `criterion-1`–`criterion-4` and `criterion-6`; `COMPLETE` is lifecycle state, not acceptance evidence, and the supported `sop outcome --accept` path cannot append acceptance to the historicalized plan | One model (`qwen3.5:4b`) on macOS arm64, 7 requests; Linux live is `NOT_RUN`; no general DLP; model-visible repository content and persisted model output remain bounded gaps. The Plan 08 A+B policy now denies the direct `.env` path and redacts model-bound tool results; Plan 03 acceptance remains missing, and artifact persistence remains a separate limit. |
| 7   | Plan 08 — model-input credential policy      | Approved A+B and implemented on `security/llm08-model-input-credentials` (decision memory `f891d5cbe5dd578addbba83ee95b47c35752970219b4d0ad6d3fc2be254a741c`): `redact.ModelInput` at the single model-bound tool-result boundary (normal and error text) and narrow credential-path denial for `read_file`/`search_files`/`copy_file` source; ordinary source and `.env.example`/`.env.sample` remain readable; `commandpolicy` unchanged | No policy decision remains. Residual limits: unknown-format secrets in command output, diffs, or ordinary files; model output artifacts remain verbatim; retrieval/handoff/caller inputs and provider retention are separate | Unknown formats and general DLP remain bounded; command output is covered by redaction, not path exclusion; `.env.example`/`.env.sample` are explicit exceptions |

## Deliverable closeout

| Deliverable                       | PR                | Merge      | Disposition                                                                                                           |
| --------------------------------- | ----------------- | ---------- | --------------------------------------------------------------------------------------------------------------------- |
| Gap register                      | #307              | `61b03790` | Merged; this file is its reconciled successor                                                                         |
| Aggregate index identity defect   | #308              | `bf0d824d` | Fixed; per-row digest/schema decision open                                                                            |
| macOS isolation evidence          | #312              | `b37f9560` | Complete for isolated replay and evaltarget only                                                                      |
| RAG `document_hash` investigation | #310              | `3e3e313e` | Investigation and proposal complete; GAP-5 open                                                                       |
| Scanner integrity investigation   | #311              | `c3a8eed8` | Investigation and proposal complete; binary-pin decision open                                                         |
| Slack test preparation            | #313              | `f557c6fd` | Preparation complete; live delivery `NOT_RUN`                                                                         |
| Plan 03 acceptance request        | #314              | `a37a7bbd` | Request prepared; acceptance pending                                                                                  |
| Plan 10 wrap-up                   | #328 | `35340774c16b10236b0ca0143cc0d07c85a8e6b3` | Merged; reconciles the sink evidence and prepares the authorization/acceptance request; no live send, no lifecycle transition |
| Accepted-outcome resume fix       | #315              | `d3830e35` | Merged                                                                                                                |
| Accepted-outcome refresh P1 fix   | #316              | `1ace57a8` | Merged; closes the #315 follow-up                                                                                     |

Both the accepted-outcome resume fix (#315) and its P1 correction (#316) are merged.

Merging these PRs does not imply installation, deployment, or SOP plan completion.
The merged-evidence follow-up (`docs/llm10-merged-evidence`) is documentation-only:
it records the #328 identity and CI result, re-runs no test, sends no message,
records no acceptance, and creates no SOP state.

## SOP lifecycle state (verified; actual transitions marked)

| Owning checkout                                | Plan                                                 | State                                               | Readiness                                                                                                                      | Action taken                                                                                                                                                                                                                                                                                                           |
| ---------------------------------------------- | ---------------------------------------------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `.worktrees/llm03-agency`                      | `plan-01-sop-llm03-excessive-agency`                 | `HISTORICALIZED` / `COMPLETE` (confirmed)           | eligible (3 tasks `LOCAL_DONE`, no approvals or verification blockers)                                                         | `sop plan historicalize` returned `already historicalized`; 3 task records preserved; no new transition claimed                                                                                                                                                                                                        |
| `.worktrees/llm04-supply-chain`                | `plan-04-sop-llm04-supply-chain`                     | `HISTORICALIZED` / `COMPLETE` (confirmed)           | eligible (3 tasks `LOCAL_DONE`, no approvals or verification blockers)                                                         | `sop plan historicalize` returned `already historicalized`; 3 task records preserved; no new transition claimed                                                                                                                                                                                                        |
| `.worktrees/llm02-disclosure` (retired)        | `plan-03-sop-llm02-sensitive-information-disclosure` | `HISTORICALIZED` / `COMPLETE` (external completion) | `DISCLOSE-003` `LOCAL_DONE` via external completion; no pending approval; no per-criterion acceptance                          | `sop task complete DISCLOSE-003 --external --commit d3830e35` (validation `PASS`, 2026-11-09T17:19:43Z); `sop plan historicalize` (`COMPLETE`, 2026-11-09T17:19:47Z); evidence preserved at `.agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure/`; worktree/local branch removed |
| Main checkout                                  | none                                                 | `NONE`                                              | ineligible: no active plan                                                                                                     | None                                                                                                                                                                                                                                                                                                                   |
| `fond-eel`, `stark-stoat`, closeout worktree   | none                                                 | not initialized                                     | unverified                                                                                                                     | None                                                                                                                                                                                                                                                                                                                   |
| `worktrees/agentic-sop/dense-pond/agentic-sop` | `PLAN-10-SOP-LLM10-Improper-Output-Handling.md`      | not initialized                                     | ineligible: `sop status`, `sop plan historicalize --check`, and `sop continue --check` all report `project is not initialized` | None; no bootstrap, no state edit, no transition claimed; the merged-evidence follow-up also did not initialize state                                                                                                                                                                                                   |
| `worktrees/agentic-sop/plan10-followup/agentic-sop` | `PLAN-10-SOP-LLM10-Improper-Output-Handling.md` | not initialized                                 | ineligible: no state was created                                                                                               | None; documentation-only follow-up; no bootstrap, no state edit, no transition claimed                                                                                                                                                                                                                                  |
| Plan 05, Plan 10, Plans 02/06/07/08/09         | documents under `docs/`                              | owning SOP state unavailable                        | unverified                                                                                                                     | None; a merged PR is not completion proof                                                                                                                                                                                                                                                                              |

No approval, per-criterion acceptance, task skip, retry, reconcile, or state-database
edit was performed. The Plan 01 and Plan 04 `COMPLETE` states were confirmed through
SOP's supported eligibility and lifecycle check; the command reported them already
historicalized, so no new transition is claimed. The Plan 03 `COMPLETE` state is an
**actual transition** in the retired `llm02-disclosure` worktree: external completion
then historicalization, as recorded above. `COMPLETE` does not mean the manual
acceptance criteria were accepted; no acceptance decision exists.

## Decisions still required

1. **Per-row index digests** — [PROPOSAL-REPOINDEX-ROW-DIGESTS.md](../../plans/PROPOSAL-REPOINDEX-ROW-DIGESTS.md): A (additive row digests, schema v2) recommended; B aggregate-only; C detached manifest.
2. **RAG evidence hash** — [PROPOSAL-RAG-EVIDENCE-HASH.md](../../plans/PROPOSAL-RAG-EVIDENCE-HASH.md): A (definition plus optional `chunk_hash`) recommended; B definition only; C schema v2 requiring `chunk_hash`.
3. **Scanner binary pins** — [PROPOSAL-SCANNER-BINARY-PINS.md](../../plans/PROPOSAL-SCANNER-BINARY-PINS.md): A keep the contract; B optional `binary_sha256` from a verified acquisition (recommended); C platform/publisher trust.
4. **Live Slack test** — authorize one synthetic test event to the `#sop-alert` webhook for a named window with an empty pre-send outbox, enable the `webhook` channel with the `slack` payload, and confirm the binding; no authorization record exists and the send remains not performed. See [llm10-acceptance-request.md](llm10-acceptance-request.md).
5. **Plan 03 acceptance** — a designated approver records acceptance for the exact campaign artifact and criteria; no acceptance is recorded. The Plan 08 A+B policy now denies the direct `.env` path and redacts model-bound tool results, but that implementation is not Plan 03 acceptance.
6. **Plan 10 manual acceptance** — the designated approver accepts or declines the six manual criteria. Passing CI is evidence for `OUTPUT-003` criterion 3, not an acceptance record. No owning SOP state exists, so `sop outcome --accept` is not yet available. See [llm10-acceptance-request.md](llm10-acceptance-request.md).

Decided and implemented: **LLM08 model-input credential policy A+B** (decision memory `f891d5cbe5dd578addbba83ee95b47c35752970219b4d0ad6d3fc2be254a741c`). The residual limits are recorded in the Plan 08 report; no policy decision remains open.

## Cross-cutting boundaries (unchanged)

- No new service, evaluator, gate, or permission framework is required by any item.
- Provider model provenance, publisher authentication, and general DLP remain outside
  SOP ownership.
- The repository remains in report-only security mode with no required categories and
  retrieval disabled by default.
- The evidence matrix is not a compliance mapping and does not imply installation,
  deployment, or full OWASP coverage.

## Preserved baseline

The original register at `main` `93c72857`, including its dispositions and boundaries,
is preserved verbatim in
[gap-register-20261109-baseline.md](gap-register-20261109-baseline.md).
