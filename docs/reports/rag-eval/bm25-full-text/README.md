# rag-eval after full-text BM25 (RAG-SOP-004 follow-up)

**Type:** Measured follow-up to [`RAG-SOP-004-DECISION.md`](../../../specs/RAG-SOP-004-DECISION.md) §6 "Next".
**Change measured:** commit `bdcfb8bf`. SOP's production BM25 (`internal/retrieval`) now indexes each
Markdown document's full text, and a document with historical authority (`docs/history/`,
SOP-archived plans, or an `Authority: historical` header) scores at weight 0.5.

## Runs

Same frozen dataset (manifest hash `4c69d935…`), corpus `sop-engineering-kb`, and producer
binary (commit `acd1fb91`, `nomic-embed-text`) and 37 excluded sources as the original runs.

- `TopK` 5: [`20261102T204448Z`](20261102T204448Z), [`20261102T204453Z`](20261102T204453Z), [`20261102T204459Z`](20261102T204459Z)
- `TopK` 10 control: [`20261102T204504Z`](20261102T204504Z)
- Summary: [`summary/summary.md`](summary/summary.md), [`summary/summary.json`](summary/summary.json)

**Arms B and C are not measured.** The Postgres database that held the `sop-engineering-kb`
embeddings for the original runs no longer exists on this machine. The producer reached a
database with no rows for that corpus and returned no evidence, so in these runs arm B equals
arm A by construction and arm C is 0. Ignore the B, C, §4.1, and full-context columns in the
summary; they say nothing about RAG.

## Arm A against the previous runs (mean recall@5, `TopK` 5)

Identical across the three runs.

| subset | A before | A now | D (full text, no weight) |
| --- | --- | --- | --- |
| historical | 0.3846 | 0.5865 | 0.6346 |
| code | 0.0909 | 0.5000 | 0.5000 |
| multi_hop | 0.2143 | 0.4643 | 0.5000 |
| reworded | 0.2500 | 0.2500 | 0.2500 |

`TopK` 10 control: A 0.6731 / 0.6818 / 0.5000 / 0.2500, D 0.7212 / 0.6818 / 0.5357 / 0.2500
(previous A @10: 0.4327 / 0.1818 / 0.3214 / 0.2500).

Per question (historical subset, 38 questions): arm A improved on 11 and regressed on 4. It now
trails D on 2 and never beats it. Of the 5 regressions across all subsets, 1 (RQ-015) is the historical
down-weight pushing out an expected `docs/history/` plan that D still finds. The other 4 (RQ-030,
RQ-038, RQ-044, RW-007) are path-only matches that full text dilutes; D misses them too.

## Does RAG still earn a place?

Not shown yet, and the bar is now much higher. Previously, arm B's only real gain was filling
extra context slots (full-context historical 0.6506 against A@10 0.4327). Full-text arm A at
depth 10 now reaches 0.6731 on historical questions, above that old arm B full-context score, so
on this dataset the extra-slot benefit is covered by BM25 alone. The one area full-text BM25 does
not touch is **reworded** questions: A and D both stay at 0.2500, and old arm B reached 0.4167.
That is the only remaining case for RAG.

To settle it, re-ingest the frozen manifest into an isolated database and re-run all four arms.
B and C will then measure a rebuilt index, not the original, and the report should say so.
`SOP_RAG_ENABLED` stays off.
