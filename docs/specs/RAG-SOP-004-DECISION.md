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

**Measured.** The frozen labeled dataset now exists and was used by the recorded
`rag-eval` runs. Three primary runs at `TopK` 5 with producer commit
`acd1fb911ac8bdd4f425b61df16a495f4a5047c9` and embedding model `nomic-embed-text`
(corpus `sop-engineering-kb`, all `status: pass`):

- [`docs/reports/rag-eval/20261102T021652Z`](../reports/rag-eval/20261102T021652Z)
- [`docs/reports/rag-eval/20261102T021737Z`](../reports/rag-eval/20261102T021737Z)
- [`docs/reports/rag-eval/20261102T021822Z`](../reports/rag-eval/20261102T021822Z)

plus one `TopK` 10 control run with the same producer commit:

- [`docs/reports/rag-eval/20261102T160658Z`](../reports/rag-eval/20261102T160658Z)

All aggregate numbers below are transcribed from
[`docs/reports/rag-eval/summary/summary.json`](../reports/rag-eval/summary/summary.json)
and [`docs/reports/rag-eval/summary/summary.md`](../reports/rag-eval/summary/summary.md),
written by `rag-eval summarize` from those runs.

**Exclusions.** The producer could not ingest **37 excluded documents** (listed in the
header of [`docs/reports/rag-eval/20261102T021652Z/results.md`](../reports/rag-eval/20261102T021652Z/results.md));
none of them is expected evidence. Arms A (BM25-only) and D still index those documents.

## 3. Comparative evaluation

Arms: **A — BM25-only** (baseline), **B — BM25 + optional RAG** (candidate),
**C — RAG-only** (diagnostic only, never a rollout candidate), **D — SOP's BM25 over
full document text** (diagnostic only; it uses SOP's own `internal/retrieval` ranker,
not the producer, and is not a rollout candidate).

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

### 3.2 rag-eval comparative runs (Measured)

**Measured.** The predeclared four-arm comparison at `TopK` 5 was run three times
([run directories in §2](../reports/rag-eval/summary/summary.md)); mean recall@5 per
subset, identical across all three runs
([summary](../reports/rag-eval/summary/summary.json)):

| subset | A | B | C | D |
| --- | --- | --- | --- | --- |
| historical | 0.3846 | 0.3846 | 0.4679 | 0.6346 |
| code | 0.0909 | 0.0909 | 0.5000 | 0.5000 |
| multi_hop | 0.2143 | 0.2143 | 0.3810 | 0.5000 |
| reworded | 0.2500 | 0.2500 | 0.2500 | 0.2500 |

Arm B's top five equals arm A's because **SOP ranks repository items above RAG items**,
so the RAG candidates never displace the repository items in arm B's top five.

### 3.3 Producer verification

A separate real-PostgreSQL/pgvector producer verification (authorized corpus, scope
rejection, unsupported-schema rejection, provenance and isolation) is recorded in
[`../reports/RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md`](../reports/RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md).

### 3.4 What is not measured (Unavailable)

The following predeclared metric areas have **no recorded per-case result from the
rag-eval runs** and are therefore **Unavailable**, with the reason:

| Area | Status | Reason |
| --- | --- | --- |
| Citation/provenance correctness rate, per case | Unavailable | Not computed by `rag-eval summarize`; only spot checks exist (§3.1, §3.3). |
| Context budget / truncation events, per configuration | Partially measured | `truncated` is `false` in every arm B row of the runs; no per-configuration sweep. |
| Estimated model-token burden, per configuration | Unavailable | Not recorded. |
| Failure/fallback rate across the full case set | Unavailable | `fallback` is `unavailable` in every recorded row. |
| Isolation incl. NULL-corpus legacy rows at any `top_k` | Partially measured | Spot checks passed (§3.1, §3.3); not a full sweep. |
| Determinism over frozen inputs+responses | Partially measured | The three `TopK` 5 runs are identical per arm and subset (see §3.2); formal determinism is not separately certified. |

No aggregate claim is made where the backing per-case records do not exist. Single
positive examples are treated as anecdotes, not as success.

## 4. End-to-end operational fixtures

**Measured.** The eight predeclared scenarios are recorded as tests in
`internal/cli/rag_fixtures_test.go` (build tag `ragacceptance`), delivered and verified
by RAG-EVAL-003. Each runs SOP's real `run` path in-process through the `internal/cli`
test harness, with the real `ragclient` executing a producer script; only the agent is
faked. [`docs/reports/rag-eval/fixtures.md`](../reports/rag-eval/fixtures.md) lists each
case, how it is produced, and its asserted outcome.

- Seven cases use a scripted producer failure (cold start, stale index, embedding
  provider unavailable, malformed response, cross-repository scope, required evidence,
  resumed run). "Embedding provider unavailable" stands in for Ollama being down.
- One case is a real outage: `PostgresUnavailable` runs `psql` against an address with
  nothing listening.
- Asserted outcomes: an optional task falls back to BM25-only with telemetry
  `reason_code` `OPTIONAL_FALLBACK` and `used_in_context` false; the required-evidence
  task blocks with `REQUIRED_EVIDENCE_MISSING` and the agent is not invoked; no
  out-of-corpus item reaches the agent's prompt; a resumed run records the same outcome.

Not covered: a real Ollama outage, and a stale index produced by a real re-ingest.

**§4 hard blockers (from [`RAG-EVALUATION.md`](RAG-EVALUATION.md) §4).** "Not triggered" means
the blocker did not occur in the recorded evidence:

| Hard blocker | Triggered? | Evidence |
| --- | --- | --- |
| Governance regression | Not triggered | The full test suite passes on this branch. The fixtures (§4) show fallback and blocking driven by structured state. They don't separately assert approval records. |
| Cross-scope leak | Not triggered | No row in the four runs has a cross-corpus item. `TestRAGFixtureCrossRepositoryScope` shows an out-of-corpus item is rejected and never reaches the prompt. |
| RAG changes approval/completion state | Not triggered | The evaluation runs are retrieval-only. RAG items enter the context at the lowest priority, labeled untrusted. No fixture shows a RAG item changing approval or completion state. |
| Required-evidence silent pass | Not triggered | `TestRAGFixtureRequiredEvidenceBlocks` shows the task blocks with `REQUIRED_EVIDENCE_MISSING` and the agent is not invoked. |

## 5. Draft §4.1 rule comparison (as computed in the summary)

All numbers from [`summary.json`](../reports/rag-eval/summary/summary.json)
(section_4_1 block, run
[`docs/reports/rag-eval/20261102T021652Z`](../reports/rag-eval/20261102T021652Z),
status pass):

| §4.1 check | Threshold | Measured | Met? |
| --- | --- | --- | --- |
| Value (mean recall@5 B − A, historical) | >= 0.10 | 0 | **Not met** |
| Breadth (historical questions where B > A) | >= 5 | 0 | **Not met** |
| Code regressions (code questions where B < A) | <= 1 | 0 | **Met** |
| Latency p95, arm B | <= 500 ms | 306 ms | **Met** |
| arm_a_identical across runs | true | true | **Met** |

**Rule outcome:** the §4.1 rule requires the value and breadth checks; both are not
met, so the rule does **not** give GO. This section states only what the predeclared
rule gives; it makes no GO/NO-GO recommendation beyond that.

**Why arm B's top five equals arm A's:** SOP ranks repository items above RAG items,
so arm B's top five is identical to arm A's top five (reflected in the identical A/B
recall@5 columns in §3.2 and `arm_a_identical: true`).

## POST-HOC: not part of the §4.1 decision rule

The following metric was **chosen after seeing results** and is not part of the
predeclared §4.1 decision rule. Arm B uses **more context slots than arm A's top
five** (mean distinct documents in arm B's context: **8.4627**), so its full-context
score is reported **next to the depth-10 (`TopK` 10) controls for arms A and D** —
the depth-10 controls exist to make the comparison fair.

Arm B full-context mean recall (run
[`docs/reports/rag-eval/20261102T021652Z`](../reports/rag-eval/20261102T021652Z)),
with the depth-10 control mean RecallAt10 (run
[`docs/reports/rag-eval/20261102T160658Z`](../reports/rag-eval/20261102T160658Z)) for
arms A and D ([summary](../reports/rag-eval/summary/summary.json)):

| subset | B full-context | A @10 (control) | D @10 (control) |
| --- | --- | --- | --- |
| historical | 0.6506 | 0.4327 | 0.7212 |
| code | 0.5909 | 0.1818 | 0.6818 |
| multi_hop | 0.4881 | 0.3214 | 0.5357 |
| reworded | 0.4167 | 0.2500 | 0.2500 |

- Mean distinct documents in arm B's context: **8.4627**.
- Historical questions improved vs arm A recall@5: **17**; regressed: **0**.

### Multi-hop and reworded results (from the summary)

Mean recall@5 at `TopK` 5 (identical across the three primary runs): **multi_hop**
A 0.2143 / B 0.2143 / C 0.3810 / D 0.5000; **reworded** A 0.2500 / B 0.2500 /
C 0.2500 / D 0.2500. Multi-hop misses (recall@5 0.00 for arms A and B in
[`docs/reports/rag-eval/20261102T021652Z/results.md`](../reports/rag-eval/20261102T021652Z/results.md)):
**RQ-041, RQ-042, RQ-043, RQ-047, RQ-049, RQ-050, RQ-051, RW-010, RW-012**.

## 6. Decision

**Decision: NO-GO for RAG as currently integrated.** Recorded by Tom Tran, 2026-11-02.

- The predeclared §4.1 rule does not give GO. Arm B's recall@5 equals arm A's (value 0,
  breadth 0), because SOP ranks repository items above RAG items.
- Post-hoc, outside the rule: RAG adds recall at equal context depth (0.651 against
  BM25@10 at 0.433). It is the only arm that helps reworded questions (0.417 against
  0.250). But BM25 over full document text alone scores higher (0.721 at depth 10).
- `SOP_RAG_ENABLED` stays off by default.

**Next:** index document text in SOP's BM25 and down-rank historical documents. Then
re-evaluate RAG on top of full-text BM25 with this same frozen dataset.

## 7. Default enablement

Default behavior is **unchanged**. The adapter stays **disabled by default**
(`SOP_RAG_ENABLED` defaults are unchanged); no code path enables it automatically, and
no change in this report flips a default. Expansion of usage requires an explicit
operator decision recorded here.

## 8. Reproducibility

Reproducible commands and outputs that **exist** are recorded in the linked reports:

- [`../reports/RAG-SOP-004-POC-EVALUATION.md`](../reports/RAG-SOP-004-POC-EVALUATION.md)
- [`../reports/RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md`](../reports/RAG-SOP-004-PHASE2-PRODUCER-VERIFICATION.md)
- rag-eval runs and summary: [`../reports/rag-eval/`](../reports/rag-eval/)
  (primary `TopK` 5 runs `20261102T021652Z`, `20261102T021737Z`, `20261102T021822Z`;
  `TopK` 10 control `20261102T160658Z`; [`summary`](../reports/rag-eval/summary/))

Any command whose output is not recorded in one of those reports is marked
**Unavailable** above rather than presented as measured.
