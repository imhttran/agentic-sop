# OWASP LLM — remaining gap register (2026-11-09)

**Current reconciliation:** `main` at `d3830e35` (2026-11-09 UTC), after the closeout
PRs merged. The original point-in-time baseline at `93c72857` is preserved verbatim
in [gap-register-20261109-baseline.md](gap-register-20261109-baseline.md). That file
is historical evidence; it is not the current disposition.

This register separates what merged from what is still open. **A merged proposal is
not a closed gap.** Implementation or investigation completion and the remaining
policy, trust, or acceptance decision are recorded separately.

## Current dispositions

| #   | Item                                         | Completed implementation or investigation (merge)                                                                                                                                                                                                                         | Still open                                                                                                                                                    | Retained limitation                                                                                                                                                                        |
| --- | -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1   | Plan 05 GAP-4 — per-document index integrity | Aggregate identity defect fixed: the resolved document rows (class, authority, lifecycle, authority source) now feed `Identity.Digest`, with a regression that fails before the fix (PR #308, `bf0d824d`). SOP state for this plan is not available in an owning checkout | **Decision:** per-row digest and schema v2 ([PROPOSAL-REPOINDEX-ROW-DIGESTS.md](../../plans/PROPOSAL-REPOINDEX-ROW-DIGESTS.md))                               | Rows still carry no digest of their own; no consumer loads `context/index.json` today; the aggregate digest is truncated to 16 hex and covers bytes plus resolved rows, not benign content |
| 2   | Plan 05 GAP-5 — RAG `document_hash`          | Deterministic fixtures pin the current non-verification, and an additive chunk-hash contract is proposed (PR #310, `3e3e313e`). SOP state for this plan is not available in an owning checkout                                                                            | **Decision:** contract option A/B/C in [PROPOSAL-RAG-EVIDENCE-HASH.md](../../plans/PROPOSAL-RAG-EVIDENCE-HASH.md)                                             | `document_hash` covers the full source document while SOP receives one returned chunk; a matching hash would prove bytes, not publisher authenticity or benign content                     |
| 3   | Plan 04 — local scanner integrity            | Verification record plus the binary-pin options, with no enforced-MUST deviation found (PR #311, `c3a8eed8`). Plan `plan-04-sop-llm04-supply-chain` is `HISTORICALIZED` / `COMPLETE`                                                                                      | **Decision:** keep the contract, optional `binary_sha256`, or platform trust ([PROPOSAL-SCANNER-BINARY-PINS.md](../../plans/PROPOSAL-SCANNER-BINARY-PINS.md)) | Same-version substitution in a writable local tools directory is undetected; report-only mode, empty `required_categories`, no execution-time binary pin, no publisher authenticity        |
| 4   | Plan 01 — macOS isolation evidence           | Boundary fixtures re-run with a loopback positive control; Unix-domain socket denial enabled on Seatbelt; process-group kill recorded (PR #312, `b37f9560`). Plan `plan-01-sop-llm03-excessive-agency` is `HISTORICALIZED` / `COMPLETE`                                   | No open evidence item for the isolated-replay and evaltarget paths. Normal-agent isolation and Linux remain out of scope                                      | macOS arm64 only, and only the isolated-replay and evaltarget paths; normal agent execution is policy-only and not OS-sandboxed; Linux Bubblewrap is `NOT_RUN` here                        |
| 5   | Plan 10 — Slack delivery                     | Local payload/transport verification is confirmed and the authorized live test is prepared (PR #313, `f557c6fd`). SOP state for this plan is not available in an owning checkout                                                                                          | **Decision:** authorization and channel confirmation for one synthetic message to `#sop-alert`; the send itself is not performed                              | Live delivery and rendering are `NOT_RUN`; 2xx proves endpoint acceptance, not Slack rendering; unfurl and client rendering are external; Linux CLI evidence only                          |
| 6   | Plan 03 — live-evidence acceptance           | The exact evidence identity and supported acceptance interface are prepared (PR #314, `a37a7bbd`). Plan `plan-03-sop-llm02-sensitive-information-disclosure` is `ACTIVE`                                                                                                  | **Decision:** designated approver must accept the exact campaign artifact (`0c8cc0ff…`, revision `9c32e3d2…`); no acceptance is recorded                      | One model (`qwen3.5:4b`) on macOS arm64, 7 requests; Linux live is `NOT_RUN`; no general DLP; model-visible repository content and persisted model output remain bounded gaps              |

## Deliverable closeout

| Deliverable                       | PR   | Merge      | Disposition                                                             |
| --------------------------------- | ---- | ---------- | ----------------------------------------------------------------------- |
| Gap register                      | #307 | `61b03790` | Merged; this file is its reconciled successor                           |
| Aggregate index identity defect   | #308 | `bf0d824d` | Fixed; per-row digest/schema decision open                              |
| macOS isolation evidence          | #312 | `b37f9560` | Complete for isolated replay and evaltarget only                        |
| RAG `document_hash` investigation | #310 | `3e3e313e` | Investigation and proposal complete; GAP-5 open                         |
| Scanner integrity investigation   | #311 | `c3a8eed8` | Investigation and proposal complete; binary-pin decision open           |
| Slack test preparation            | #313 | `f557c6fd` | Preparation complete; live delivery `NOT_RUN`                           |
| Plan 03 acceptance request        | #314 | `a37a7bbd` | Request prepared; acceptance pending                                    |
| Accepted-outcome resume fix       | #315 | `d3830e35` | Merged, but **not fully closed**: its P1 correction is #316, still open |

Outstanding: **#316** (`fix/lifecycle-resume-p1-followup`, `fix(cli): persist
accepted-outcome refresh (P1 follow-up)`) remains open. It is not an OWASP
deliverable and is owned by its author. This closeout did not duplicate it, and no
lifecycle closeout below relies on it.

## SOP lifecycle state (verified read-only, then supported closeout)

| Owning checkout                              | Plan                                                 | State                         | Readiness                                                              | Action taken                                       |
| -------------------------------------------- | ---------------------------------------------------- | ----------------------------- | ---------------------------------------------------------------------- | -------------------------------------------------- |
| `.worktrees/llm03-agency`                    | `plan-01-sop-llm03-excessive-agency`                 | `HISTORICALIZED` / `COMPLETE` | eligible (3 tasks `LOCAL_DONE`, no approvals or verification blockers) | `sop plan historicalize`; 3 task records preserved |
| `.worktrees/llm04-supply-chain`              | `plan-04-sop-llm04-supply-chain`                     | `HISTORICALIZED` / `COMPLETE` | eligible (3 tasks `LOCAL_DONE`, no approvals or verification blockers) | `sop plan historicalize`; 3 task records preserved |
| `.worktrees/llm02-disclosure`                | `plan-03-sop-llm02-sensitive-information-disclosure` | `ACTIVE`                      | ineligible: `DISCLOSE-003 (PLANNED)`                                   | None; acceptance remains pending                   |
| Main checkout                                | none                                                 | `NONE`                        | ineligible: no active plan                                             | None                                               |
| `fond-eel`, `stark-stoat`, closeout worktree | none                                                 | not initialized               | unverified                                                             | None                                               |
| Plan 05, Plan 10, Plans 02/06/07/08/09       | documents under `docs/`                              | owning SOP state unavailable  | unverified                                                             | None; a merged PR is not completion proof          |

No approval, per-criterion acceptance, task skip, retry, reconcile, or state-database
edit was performed. The two historicalizations were SOP's own eligible transition.

## Decisions still required

1. **Per-row index digests** — [PROPOSAL-REPOINDEX-ROW-DIGESTS.md](../../plans/PROPOSAL-REPOINDEX-ROW-DIGESTS.md): A (additive row digests, schema v2) recommended; B aggregate-only; C detached manifest.
2. **RAG evidence hash** — [PROPOSAL-RAG-EVIDENCE-HASH.md](../../plans/PROPOSAL-RAG-EVIDENCE-HASH.md): A (definition plus optional `chunk_hash`) recommended; B definition only; C schema v2 requiring `chunk_hash`.
3. **Scanner binary pins** — [PROPOSAL-SCANNER-BINARY-PINS.md](../../plans/PROPOSAL-SCANNER-BINARY-PINS.md): A keep the contract; B optional `binary_sha256` from a verified acquisition (recommended); C platform/publisher trust.
4. **Live Slack test** — authorize one synthetic message and confirm the webhook is bound to `#sop-alert`; the send remains not performed.
5. **Plan 03 acceptance** — a designated approver records acceptance for the exact campaign artifact and criteria; no acceptance is recorded.

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
