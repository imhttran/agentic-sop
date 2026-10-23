# HIP-007 — Adoption decision and comparison report

Recorded: 2026-10-23.

## Decision

**Do not retain the candidate.** The baseline is preserved. No candidate prompt
or skill change is included in the PR.

The pilot is negative and partly inconclusive. Every arm failed its fixed
acceptance checks, no held-out pair qualifies, and token usage is unavailable.
The candidate did not reduce human interventions, tokens, or elapsed time while
preserving correctness.

## Comparison inputs

- Starting commit: `962a90cd`
- Executor setting: `sop-go-run`
- Model: `deepseek-v4.1-flash:cloud`
- Settings: `temperature=0`
- Tool permissions: `read_file`, `write_file`
- Baseline instruction hash: `b6f5d08194a12de3278ed8d3ae2c95485d4532dbf30b9d7b7a8832c0487fb865`
- Candidate instruction hash: `401dcaeee640bbf76b851d0905900cce196c1da1a8d01d2143742664a59812e6`
- Comparison fingerprints:
  - S1: `8478224673ec0b8ca18fef3c71f8a2f87ec4507feea46e5f1c7dc563495fcfee`
  - S2: `9a107ab5a8df3651bccdd76c519cd975caf953d4509a53a174c197b3c7b8e4e4`
  - S3: `88ac73651520d66d1c06383da991e087ee8a985732af85dd132b4c7dc056886a`
- Limits: 3 task pairs, 6 initial executions, no retries, 900 seconds and 200000
  tokens per run. No held-out pair was run.

## Results

| Pair | Baseline | Candidate | Fixed acceptance |
| --- | --- | --- | --- |
| S1 | `CONTINUE`; external Promptfoo verification unavailable | invalid candidate compile | fail |
| S2 | `IMPLEMENT_NO_PROGRESS`, 6 turns, 22247 ms | `IMPLEMENT_NO_PROGRESS`, 8 turns, 23256 ms | fail |
| S3 | `IMPLEMENT_NO_PROGRESS`, 6 turns, 25949 ms | `CONTINUE`, no change, 23686 ms | fail |

Human interventions were 0 in all arms. Tokens were not reported by the
executor and are recorded as `UNAVAILABLE`, not zero.

## Promotion criteria

- Pass every fixed acceptance check: **not met**.
- Reduce human interventions without a >10% token/time increase: **not met**.
- Reduce tokens or elapsed time by at least 10% with no intervention increase and
  no >10% increase in the other cost metric: **not met** (no accepted run; tokens
  unavailable).
- Held-out task passes and stays within 10%: **not run**, because the initial
  result did not meet promotion criteria.

## Lessons and provenance

1. The executor's pre-mutation failure dominates the comparison. S2 and S3 both
   stopped in discovery/no-progress before any repository mutation. A prompt
   revision cannot reduce cost when the agent does not reach implementation.
   Source: S2/S3 arm evidence and comparison JSON.
2. Task selection must exclude network-dependent acceptance. S1's baseline could
   not verify the Promptfoo pin because network access is outside the allow-list;
   S1 is not a suitable offline pilot task. Source: S1 baseline evidence.
3. Candidate instruction content must be syntax-checked before execution. The
   first S1 candidate appended text outside a Go string and failed to compile; the
   corrected candidate was used for S2/S3. Source: S1 candidate evidence and
   HIP-005 correction.
4. Token telemetry is missing. The pilot cannot make a token-cost decision from
   this executor. Source: every comparison JSON with `tokens_used: 0` and no
   reported token measure.
5. Advisory memory does not support lesson provenance with source evidence and
   input fingerprints; lessons remain in this report, as required by HIP-002.
   Source: `docs/plans/HIP-002-existing-facilities-and-choices.md`.

## Evidence

The authoritative execution records are:

- `docs/plans/HIP-006-pilot-evidence.md`
- `docs/plans/pilot/out-s1/comparison.json` and `.md`
- `docs/plans/pilot/out-s2/comparison.json` and `.md`
- `docs/plans/pilot/out-s3/comparison.json` and `.md`

No candidate was activated, promoted, or written to shared memory.
