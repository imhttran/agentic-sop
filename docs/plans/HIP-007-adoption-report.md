# HIP-007 — Adoption decision and comparison report

Recorded: 2026-10-23. This report distinguishes implementation completion from
experimental validity.

## Decision

**Do not retain the candidate.** The baseline is preserved. No candidate prompt
or skill change is included in the PR.

The pilot is **negative for the candidate as tested** and **inconclusive** for
candidate quality: every recorded arm failed its fixed acceptance checks, the S1
pair was invalid because the candidate did not compile, token usage is unavailable,
and no held-out pair qualified.

## Execution accounting

- Actual launched arm executions: **7**.
- Recorded in comparison files: 6 (S1 rerun, S2, S3).
- Discarded S1 setup attempt: 1 baseline arm, log only; its comparison artifact was
  removed before preservation and is not reconstructed.
- Initial three-pair budget: 6 executions. Actual: 7, so **one over the initial
  pair budget**. Absolute ceiling: 8. No held-out pair, no automatic retries.
- Per-run limits: 900 seconds enforced; 200000 tokens configured. Token usage was
  not reported by the executor, so token-limit enforcement has no evidence.

## Candidate activation and validity

The candidate worktree applied `internal/ollamaagent/prompt.go` and then ran
`go run ./cmd/sop` from that worktree. The S1 candidate compile error at the
appended line proves the modified source was compiled. S2 and S3 candidate arms
built and ran that source. No compiled-binary hash was captured, so this is
source-build activation, not binary identity.

The S1 pair is invalid: the candidate injection did not compile, and the baseline
also required unavailable network verification. S2 and S3 are valid **execution**
pairs but neither arm passed acceptance; they are not evidence that the candidate
works or is reproducible.

## Comparison inputs

- Starting commit: `962a90cd`
- Executor setting: `sop-go-run`
- Model: `deepseek-v4.1-flash:cloud`
- Settings: `temperature=0`
- Tool permissions: `read_file`, `write_file`
- Baseline instruction hash: `b6f5d08194a12de3278ed8d3ae2c95485d4532dbf30b9d7b7a8832c0487fb865`
- Candidate instruction hash: `401dcaeee640bbf76b851d0905900cce196c1da1a8d01d2143742664a59812e6`
- Comparison fingerprints: S1 `8478224673ec…`, S2 `9a107ab5a8df…`, S3 `88ac73651520…`
- Limits: 900 seconds and 200000 tokens per run; six initial executions.

## Promotion criteria

- Pass every fixed acceptance check: **not met**.
- Reduce human interventions without a >10% token/time increase: **not met**.
- Reduce tokens or elapsed time by at least 10% with no intervention increase and
  no >10% increase in the other cost metric: **not met**; tokens unavailable.
- Held-out task passes and stays within 10%: **not run**, because the initial
  result did not meet promotion criteria.

## Lessons and provenance

1. Pre-mutation executor failure dominates the comparison. S2 and S3 stopped in
   discovery/no-progress before any repository mutation. Source: S2/S3 evidence.
2. Task selection must exclude network-dependent acceptance. S1's baseline could
   not verify the Promptfoo pin offline. Source: S1 baseline evidence.
3. Candidate instruction content must be syntax-checked before execution. The
   first S1 candidate appended text outside a Go string and failed to compile.
   Source: S1 candidate evidence and HIP-005 correction.
4. Token telemetry is missing; token-cost comparisons are unavailable. Source:
   every comparison JSON with an unpopulated `tokens_used` field.
5. Advisory memory does not support lesson provenance with source evidence and
   input fingerprints; lessons remain in this report. Source:
   `docs/plans/HIP-002-existing-facilities-and-choices.md`.

## Evidence

- `docs/plans/HIP-006-pilot-evidence.md`
- `docs/plans/pilot/out-s1/`, `out-s2/`, `out-s3/`
- `docs/plans/pilot/logs/`

No candidate was activated, promoted, or written to shared memory.
