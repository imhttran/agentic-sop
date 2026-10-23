# HIP-006 — Bounded pilot evidence and reconciliation

Recorded: 2026-10-23. This record corrects the earlier count. It reconciles every
launched baseline/candidate arm, including the discarded setup attempt.

## Execution reconciliation

| # | Attempt | Arm | Outcome | Classification | Evidence |
| --- | --- | --- | --- | --- | --- |
| 1 | S1 setup attempt, before guard correction | baseline | Harness rejected SOP state and staged `TASK.md` as edits; candidate never launched | setup failure / invalid | `docs/plans/pilot/logs/s1-setup-attempt.log` |
| 2 | S1 rerun | baseline | `CONTINUE`; S1 needs network verification of the Promptfoo pin | task acceptance failure | `docs/plans/pilot/out-s1/comparison.json` |
| 3 | S1 rerun | candidate | compile failure: `internal/ollamaagent/prompt.go:258:1: syntax error` | compile failure / invalid | `docs/plans/pilot/out-s1/comparison.json` |
| 4 | S2 | baseline | `IMPLEMENT_NO_PROGRESS`, 6 turns | task acceptance failure | `docs/plans/pilot/out-s2/comparison.json` |
| 5 | S2 | candidate | `IMPLEMENT_NO_PROGRESS`, 8 turns | task acceptance failure | `docs/plans/pilot/out-s2/comparison.json` |
| 6 | S3 | baseline | `IMPLEMENT_NO_PROGRESS`, 6 turns | task acceptance failure | `docs/plans/pilot/out-s3/comparison.json` |
| 7 | S3 | candidate | `CONTINUE`; no repository change | task acceptance failure | `docs/plans/pilot/out-s3/comparison.json` |

**Actual launched arm executions: 7.** Six appear in the three recorded
comparison files; one is the discarded S1 setup attempt recorded only in its log.
The intended three-pair budget was six executions, so this is **one execution over
the initial pair budget**. It is still below the absolute eight-execution ceiling.
No held-out pair was run. **The held-out allowance was conditional on promotion
and was not available because no promotion criteria were met.** No automatic retry
was used.

The S1 setup attempt's comparison file was removed before the rerun and is not
recoverable. Its harness log is preserved above; no result is fabricated for it.

## Candidate activation

- The baseline arm ran `/Users/imhttran/go/bin`-independent `go run ./cmd/sop`
  inside its own worktree. It used the unmodified source.
- The candidate executor applied `internal/ollamaagent/prompt.go` inside the
  candidate worktree, then ran `go run ./cmd/sop` from that same worktree.
- The S1 candidate compile error at the appended instruction line is direct proof
  that the compiler loaded the changed source. S2 and S3 candidate arms built and
  ran the same worktree source and recorded `instruction applied:
  internal/ollamaagent/prompt.go` in their arm evidence. The new harness records
  the same fact in `instructions_applied`.
- No immutable compiled-binary hash was captured. Activation is source-build
  activation, not proof of a signed or deployed binary identity.

## Token limits

- **Configured:** 200000 tokens per run.
- **Enforced:** the per-run time limit is enforced. The token limit is enforced
  only when the executor reports `TokensUsed`. The SOP command executor does not
  report token usage.
- **Unavailable:** token usage in all pilot runs. The `tokens_used: 0` values in
  the historical comparison JSON are unpopulated fields, not measured zeros.
- The pilot therefore has **no token-cost comparison** and no evidence that the
  token limit was reached or enforced.

## Results

| Pair | Baseline | Candidate | Fixed acceptance |
| --- | --- | --- | --- |
| S1 | task failure; network unavailable | invalid compile | invalid pair |
| S2 | no-progress, 6 turns, 22247 ms | no-progress, 8 turns, 23256 ms | both fail |
| S3 | no-progress, 6 turns, 25949 ms | continue, no change, 23686 ms | both fail |

Human interventions were 0 in all recorded arms. **No candidate promoted; pilot
inconclusive for effectiveness.** The candidate did not pass any fixed acceptance
check, and the recorded failures do not establish candidate quality or
reproducibility. S2 and S3 show similar task failure, but that is not evidence
that a correct candidate is reproducible.

## Evidence

- `docs/plans/pilot/out-s1/comparison.json` and `.md` (rerun only)
- `docs/plans/pilot/out-s2/comparison.json` and `.md`
- `docs/plans/pilot/out-s3/comparison.json` and `.md`
- `docs/plans/pilot/logs/s1-setup-attempt.log`
- `docs/plans/pilot/logs/s1-rerun.log`
- `docs/plans/pilot/logs/s2.log`
- `docs/plans/pilot/logs/s3.log`
- `docs/plans/pilot/budget-ledger.json` (retrospective seven-entry reconciliation;
  the original runs predate the shared ledger)
- `docs/plans/pilot/SANITIZATION.md` (credential scan and path-transformation record)

The harness now copies each arm's SOP run artifacts and executor output into the
comparison `evidence/` directory before removing the disposable worktree. The
historical pilot did not use that copy path; its evidence remains the comparison
files and logs above.
