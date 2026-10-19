# Decision-memory pilot — revision 2 (future evaluation)

**Status:** the inspected pilot is **development evidence**, not a fresh held-out
benchmark. Revision 1's corpus, protocol, rubric, and sidecars are preserved byte-for-byte.

## Defect found after the run

Arm B retrieved **no evidence** for any case. `BuildFTSQuery` joined every task-context
token with an implicit AND, so a natural-language task description matched almost no
record. The two arms therefore produced identical prompts and the measured +5.56 pp was
model noise.

## Correction (development evidence only)

- `BuildFTSQuery` now joins quoted tokens with `OR` and relies on bm25 ranking.
- `memindex.Search` applies a deterministic relevance floor: a record must share at least
  two distinct query terms (or the single term of a one-term query). Records below the
  floor are rejected, not merely ranked lower.
- Repository scope, corpus scope, eligibility, drift/freshness exclusion, and result/time/
  token bounds are unchanged.

Validation used the **tuning split only**:

```text
threshold-compliant items   57
below floor                  0
below-floor rejected       110
(lexical threshold compliance only; semantic relevance not judged)
```

Held-out coverage was audited deterministically, without model calls, and recorded in
`run/audit/retrieval-coverage.json` (8 of 24 cases; 25 threshold-compliant items;
63 below-floor rejected). Prompt equality for the original run is unverified, and the
frozen coverage gate fails on `AUTO_FIX` (2 cases, need 3). The held-out audit was **not** used to tune the query.

## Why this is not a result

The original run used the defective query and cannot be re-scored. The audit shows what a
corrected query *would* retrieve; it does not measure a model using that evidence.

## Requirements for a future evaluation

1. Freeze a **new** held-out corpus revision against the corrected harness, with its own
   `CORPUS.sha256` and binding.
2. Persist per-decision retrieved IDs, actual prompt inclusion, predictions, safety
   observations, and usage.
3. Keep tuning and held-out families disjoint; never tune against held-out labels.
4. Execute within declared durable limits and report GO / NO-GO / INSUFFICIENT EVIDENCE.
5. **Do not enlarge the sample to obtain a passing result.** If coverage is insufficient,
   report insufficient evidence.

## Scope

Planning-only decision classification. Executable recovery, installer expansion, semantic
retrieval, and development suggestions remain out of scope. Memory stays off by default.
