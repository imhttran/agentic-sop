# HIP comparison harness

`sop compare` is the opt-in HIP-003 baseline/candidate comparison harness. It is
not part of `sop run`, the scheduler, or any default execution path. It runs one
task manifest twice: once with the baseline instruction set and once with the
candidate, in separate disposable worktrees from the same starting commit.

## Invocation

```bash
sop compare \
  --manifest MANIFEST.json \
  --baseline BASELINE.json \
  --candidate CANDIDATE.json \
  --out RESULT_DIR \
  --commit STARTING_SHA \
  --executor EXECUTOR_NAME \
  --model MODEL_NAME \
  --executor-command 'your-arm-command'
```

`--pairs N`, `--held-out-pairs N`, `--per-run-seconds N`, and
`--per-run-tokens N` may lower the fixed limits. They cannot raise them.
`--budget-ledger PATH` selects the shared reservation ledger; it defaults to
`.hipcompare-budget.json` in the repository root. The ledger is shared across
output directories and restarts, and the first run fixes its ceiling.

The held-out allowance is conditional on promotion and is not automatically
available.

The command writes `comparison.json` and `comparison.md` into `RESULT_DIR`.

## Inputs

- `MANIFEST.json` names the task file, its accepted inputs, the trusted
  verification commands, and the allowed candidate prompt/skill paths.
- The baseline instruction set must declare no file edits.
- The candidate instruction set may declare only files in the manifest's
  `allowed_candidate_paths`.
- A changed task input, starting commit, instruction hash, executor/model
  setting, tool permission, or verification command changes the fingerprint and
  refuses a resume.

## Safety and limits

- Both arms run from the same commit in separate disposable worktrees.
- The candidate file is applied inside the arm worktree and checked before it is
  written; any other instruction path is rejected.
- Ordinary task implementation edits are allowed and recorded; they are not
  confused with candidate-instruction edits.
- The executor receives a scrubbed environment and a per-run time/token limit.
- A shared reservation ledger records each arm before launch. A failed, timed-out,
  or interrupted arm remains counted; restarting does not reset the budget, and a
  different output directory cannot bypass it. Raising the ceiling is refused.
- The time limit is enforced by the harness. The token limit is enforced only
  when the executor reports token usage; a command executor that cannot count
  tokens records `tokens_reported: false` and the report shows `UNAVAILABLE`.
- The harness never pushes, merges, publishes, or changes external state.
- Fixed maximums: 3 pairs, 1 held-out pair, 8 executions, 900 seconds per run,
  200000 tokens per run, and zero automatic retries.

## Evidence

Each comparison records per-arm completion, acceptance, human interventions,
reported tokens (or `UNAVAILABLE`), elapsed time, failures, changed paths,
evidence paths, the instruction hashes, the fingerprint, the budget usage, and a
`verdict`. `completed` means all planned arms ran, not that the experiment
succeeded. The harness copies each arm's SOP run artifacts and executor output
into `RESULT_DIR/evidence/<arm>/` before removing the disposable worktree; the
output is redacted and bounded. A failed or timed-out arm is recorded and never
retried automatically.

See `docs/plans/HIP-006-pilot-evidence.md` and
`docs/plans/HIP-007-adoption-report.md` for the bounded pilot result:
**No candidate promoted; pilot inconclusive for effectiveness.**
