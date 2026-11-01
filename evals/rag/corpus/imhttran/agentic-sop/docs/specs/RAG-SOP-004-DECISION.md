# RAG Retrieval — Evaluation and Rollout Decision (RAG-SOP-004)

**Type:** Operator decision report.
**Status:** Recorded — awaiting explicit operator rollout decision.
**Plan:** [`docs/plans/PLAN-RAG-SOP-Integration.md`](../plans/PLAN-RAG-SOP-Integration.md).
**Predeclared criteria:** [`docs/specs/RAG-EVALUATION.md`](RAG-EVALUATION.md).

This report records what has actually been measured for the optional RAG retrieval
adapter, the required-integration-validation policy for database-sensitive changes,
and the operator decision for expanding use of the adapter. Thresholds were
predeclared in `RAG-EVALUATION.md` before results and are not moved here.

**Evidence posture.** This report distinguishes three states so no claim is asserted
from anecdote:

- **Measured** — backed by a recorded, reproducible run in this repository.
- **Predeclared only** — the evaluation method exists but the required dataset/fixture
  does not yet exist in the repository, so no result is claimed.
- **Unavailable** — a required input could not be produced here; the reason is stated.

## 1. Required PostgreSQL integration validation category

**Measured.** The PostgreSQL/pgvector integration validation category is enforced as a
**deterministic file-pattern policy**, entirely separate from retrieval-quality metrics:

- A change whose paths match the migration/database-sensitive patterns is classified as
  requiring the PostgreSQL integration validation category
  (`internal/quality/postgres_policy.go`, `quality.PostgresIntegrationRequired`).
- Unit-only validation can **never** satisfy the category: the category is satisfied only
  by a configured command *classified* as real PostgreSQL/pgvector integration work
  (`internal/validate/postgres.go`, `ClassifyPostgresCommand`/`PostgresConfigured`).
- When the category is required and no classified command is configured, the
  deterministic quality gate fails closed with
  `required PostgreSQL integration validation is not configured`
  (`internal/quality/quality.go`). A configured-but-not-executed or all-skipped check
  also fails closed (`internal/testrunner/postgres_skip.go`; `parseGoTestExecuted`
  refuses exit code 0 without a passing test event).
- Non-database changes are unaffected: they do not acquire the category.

Deterministic patterns (see `postgresPathPatterns`): `**/migrations/**`, `**/*.sql`,
`**/migrate*.go`, `**/schema*.go`, `**/pg*.go`, `**/vector*.go`, `**/*pgvector*`.

The policy lives in configuration/files and never consults retrieval-quality output.

## 2. Frozen labeled dataset

**Predeclared only — not yet present.** The evaluation dataset is specified in
[`RAG-EVALUATION.md`](RAG-EVALUATION.md) and is intended to cover:

1. repository code questions;
2. architecture/history questions;
3. design-decision questions;
4. failure-investigation questions;
5. **no-answer** cases (no relevant evidence exists);
6. **injection** cases (retrieved text attempting to impersonate instructions).

Each case is intended to carry **permitted corpus** and **relevant evidence identity**
labels. At this revision the frozen fixture files (for example under `evals/rag/`) and
their content hashes are **not present in the repository**, so the comparative
relevance/recall tables that would depend on them are **Unavailable** — see §3.

## 3. Comparative evaluation

Arms: **A — BM25-only** (baseline), **B — BM25 + optional RAG** (candidate),
**C — RAG-only** (diagnostic only, never a rollout candidate).

### 3.1 What was measured

A controlled retrieval-only proof of concept was run against a real PostgreSQL 16.15 /
pgvector 0.6.0 store with real embeddings. Full method and reproducible commands are in
[`../reports/RAG-SOP-004-POC-EVALUATION.md`](../reports/RAG-SOP-004-POC-EVALUATION.md).
Headline per-query results (10 frozen `topicNN` queries, 12-document controlled corpus):

| Mode | recall@5 | precision@5 | MRR | latency p50/p95 |
| --- | --- | --- | --- | --- |
| keyword baseline (`ts_rank_cd`; **not** BM25) | 1.0 | 0.2 | 1.0 | n/a (SQL) |
| hybrid producer | 1.0 | 0.2 | 1.0 | 26 ms / 27 ms |

- Queries keyword missed but hybrid found: **none**.
- Observed improvement over keyword: **none**. Observed regression: **none**.
- Corpus isolation held (`corpus:"other"` ⇒ `UNAUTHORIZED_SCOPE`; legacy NULL-corpus rows
  excluded); every item carried `corpus/source/section/page/document_hash/retrieval_method/rank`.

> **Caveat, stated in the source report:** the keyword arm is PostgreSQL full-text
> `ts_rank_cd`, explicitly **NOT BM25**, so this is *not* the predeclared BM25-vs-RAG
> comparison. It is a controlled POC only.

### 3.2 Producer verification

A separate real-PostgreSQL/pgvector producer verification (authorized corpus, scope
rejection, unsupported-schema rejection, provenance and isolation) is recorded in
[`../reports/RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md`](../reports/RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md).

### 3.3 What is not measured (Unavailable)

The following predeclared metric areas have **no recorded per-case result in this
repository** and are therefore **Unavailable**, with the reason:

| Area | Status | Reason |
| --- | --- | --- |
| Grounded relevance/recall vs labelled evidence, per case | Unavailable | Requires the frozen labeled dataset of §2, which is not present. |
| Citation/provenance correctness rate, per case | Unavailable | Same dataset dependency; only spot checks exist (§3.1, §3.2). |
| Context budget / truncation events, per configuration | Unavailable | Not recorded per case for a full run. |
| Estimated model-token burden, per configuration | Unavailable | Not recorded. |
| Failure/fallback rate across the full case set | Unavailable | Same dataset dependency. |
| Isolation incl. NULL-corpus legacy rows at any `top_k` | Partially measured | Spot checks passed (§3.1, §3.2); not a full sweep. |
| Determinism over frozen inputs+responses | Unavailable | Not recorded. |

No aggregate claim is made where the backing per-case records do not exist. Single
positive examples are treated as anecdotes, not as success.

## 4. End-to-end operational fixtures

**Predeclared only — not yet present.** The scenarios to be exercised repeatedly and
recorded are: resumed runs; cold start; stale index; unavailable Ollama; unavailable
PostgreSQL; malformed response; cross-repository scope; and permission boundaries
(predeclared in [`RAG-EVALUATION.md`](RAG-EVALUATION.md)).

At this revision there are **no recorded end-to-end operational fixture runs** in the
repository, so no pass/fail claim is made for them. Expected behavior — degrade to
BM25-only, or fail closed for required evidence — is defined in the predeclared
criteria but is **not yet demonstrated**. Unit/CLI coverage of runtime BLOCKED/fallback
behavior exists, but that is not a substitute for the recorded end-to-end matrix.

## 5. Decision

**Decision: NO-GO (for expanding usage now) — insufficient evidence.**

Rationale, traceable to the evidence above:

- The only \(\text{measured}\) retrieval comparison (§3.1) showed **no measurable
  improvement** over the keyword baseline, and the baseline was not even the
  predeclared BM25 arm. The predeclared pilot value threshold was therefore not met.
- The frozen labeled dataset (§2) and the recorded end-to-end operational fixture
  matrix (§4) do **not** exist in the repository, so the per-case evidence the
  predeclared criteria require to consider a pilot is **Unavailable**.
- No fabrications are permitted in place of that evidence: absence of evidence is
  recorded as NO-GO, not as a conditional or successful verdict.

This decision **may be revisited to CONDITIONAL/GO** only after the missing evidence
exists: the frozen dataset, the BM25-vs-RAG (and diagnostic RAG-only) per-case
comparison, and the recorded operational fixture matrix.

### Tradeoffs

- A possible recall gain on historical/architectural questions where lexical overlap is
  weak remains **hypothetical** here; it has not been measured, so it is not counted.
- BM25-only remains the safe, deterministic default; nothing in this report changes that.

### Next steps (to convert NO-GO into an evidence-backed decision)

1. Land the frozen labeled dataset (§2) with content hashes and a loader integrity check.
2. Run the predeclared BM25-only vs BM25+RAG (and diagnostic RAG-only) comparison with
   per-case rows for relevance/recall, citation correctness, context budget, latency,
   failure rate, and estimated model-token burden.
3. Record the end-to-end operational fixture matrix (§4) with reproducible commands and
   captured outputs.
4. Re-issue this decision from those records.

## 6. Default enablement

Default behavior is **unchanged**. The adapter stays **disabled by default**; no code
path enables it automatically, and no change in this report flips a default. Expansion
of usage requires an explicit operator decision recorded here, and this report records
**NO-GO**, so no expansion is authorized.

## 7. Reproducibility

Reproducible commands and outputs that **exist** are recorded in the linked reports:

- [`../reports/RAG-SOP-004-POC-EVALUATION.md`](../reports/RAG-SOP-004-POC-EVALUATION.md)
- [`../reports/RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md`](../reports/RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md)

Any command whose output is not recorded in one of those reports is marked
**Unavailable** above rather than presented as measured.
