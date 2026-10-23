# HIP-006 — Bounded pilot evidence

Recorded: 2026-10-23. This record is authoritative for the pilot result. It uses
the six initial arm executions only; no held-out pair was run and no retry or
budget increase was applied.

## Executions

| Pair | Arm | Completed | Acceptance | Human interventions | Tokens | Elapsed | Turns / outcome |
| --- | --- | --- | --- | --- | --- | --- | --- |
| S1 | baseline | no | no | 0 | UNAVAILABLE | 204650 ms | `CONTINUE`; S1 requires network verification of the Promptfoo pin, which is outside the allow-list |
| S1 | candidate | no | no | 0 | UNAVAILABLE | 544 ms | invalid candidate compile; see correction below |
| S2 | baseline | no | no | 0 | UNAVAILABLE | 22247 ms | `IMPLEMENT_NO_PROGRESS`, 6 turns |
| S2 | candidate | no | no | 0 | UNAVAILABLE | 23256 ms | `IMPLEMENT_NO_PROGRESS`, 8 turns |
| S3 | baseline | no | no | 0 | UNAVAILABLE | 25949 ms | `IMPLEMENT_NO_PROGRESS`, 6 turns |
| S3 | candidate | no | no | 0 | UNAVAILABLE | 23686 ms | `CONTINUE`, no repository change |

`Tokens` is `UNAVAILABLE` because the executor did not report a token measure.
It is not substituted with zero. The comparison files contain `0` as the
unpopulated field; the report treats that as missing telemetry.

## Evidence paths

- `docs/plans/pilot/out-s1/comparison.json`
- `docs/plans/pilot/out-s1/comparison.md`
- `docs/plans/pilot/out-s2/comparison.json`
- `docs/plans/pilot/out-s2/comparison.md`
- `docs/plans/pilot/out-s3/comparison.json`
- `docs/plans/pilot/out-s3/comparison.md`
- Arm run evidence is preserved in each comparison's `evidence_paths`.

## Invalid S1 candidate run

The first candidate content appended its instruction outside a Go string,
causing `internal/ollamaagent/prompt.go:258:1: syntax error: non-declaration
statement outside function body`. This is a harness input defect, not a model
result. The candidate was corrected by inserting the instruction inside the
IMPLEMENT guidance string. The corrected candidate was used for S2 and S3.
S1 was not re-run: the six-execution initial budget was already consumed and no
promotion criteria were met.

## Budget accounting

- Initial pairs attempted: 3 (S1, S2, S3).
- Arm executions: 6 of 8 absolute maximum.
- Held-out pair: not run; promotion criteria were not met.
- Automatic retries: 0.
- Per-run limits: 900 seconds and 200000 tokens; both arms were below the time
  limit, and token use is unavailable.

## Result

The candidate is not eligible for retention. It did not pass any task's fixed
acceptance check, did not reduce human interventions, and did not reduce elapsed
time without increasing discovery. S2's candidate used more discovery turns than
its baseline; S3's candidate made no repository change. The baseline is retained.
The pilot is negative/inconclusive, which is a valid outcome.
