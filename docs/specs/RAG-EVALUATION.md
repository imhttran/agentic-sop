# RAG Retrieval — Evaluation and GO/NO-GO Criteria

**Type:** Evaluation plan (RAG-SOP-001 deliverable).
**Status:** APPROVED (architecture) — operator-approved 2026-10-15 for RAG-SOP-002 implementation (documentation record; not a SOP lifecycle approval).
**Plan:** [`docs/plans/PLAN-RAG-SOP-Integration.md`](../plans/PLAN-RAG-SOP-Integration.md).

This document predeclares *how* RAG-SOP-004 will judge RAG vs BM25-only. Thresholds are
agreed here **before** seeing results, to avoid moving the goalposts. It does not invent
new thresholds during execution.

## 1. Frozen dataset

A frozen set of real SOP questions, each labelled with the permitted corpus and the
relevant evidence identities, spanning:

1. repository code questions;
2. architecture/history questions;
3. design-decision questions;
4. failure-investigation questions;
5. **no-answer** cases (no relevant evidence exists);
6. **injection** cases (retrieved text that tries to impersonate instructions).

## 2. Comparisons

- **A — BM25-only** (baseline).
- **B — BM25 + optional RAG** (the proposed feature).
- **C — RAG-only** (diagnostic only, never a rollout candidate).

## 3. Metrics

| Area | Metric |
| --- | --- |
| Retrieval relevance | recall@k, precision@k against labelled evidence |
| Citation/provenance | share of included items with correct `corpus/source/document_hash/section` |
| Context quality | bytes used, truncation events, informational-only vs required |
| Latency | p50/p95 request latency, added end-to-end time |
| Resource consumption | process/DB cost, response bytes |
| Failure/fallback | fallback rate + reasons; required-evidence fail-closed correctness |
| Isolation | zero cross-corpus leakage: a corpus-filtered query never returns rows from another corpus or a NULL-corpus (legacy) row; the pre-filter holds at any `top_k` |
| Determinism | identical context for frozen inputs+responses |

## 4. GO / CONDITIONAL / NO-GO

- **Hard blockers (any ⇒ NO-GO):** any governance regression; any cross-scope leak; any
  case where RAG changes approval/completion state; any required-evidence silent pass.
- **Proposed pilot GO threshold:** no hard blocker **and** at least one meaningful,
  per-case-evidenced improvement over BM25 on the frozen historical-knowledge subset
  **and** bounded p95 latency within the agreed target **and** reliable optional-mode
  fallback.
- **CONDITIONAL:** the improvement is real but a specific metric (e.g. latency or
  provenance) needs remediation first.
- Default rollout is **never automatic**: RAG stays disabled until an explicit operator
  rollout decision.

### 4.1 Numeric thresholds (operator-approved 2026-11-01)

These numbers make §4 testable. They were fixed before any run on the frozen dataset in `evals/rag/`. They don't change during execution.

- The historical-knowledge subset is the `architecture_history`, `design_decision`, `failure_investigation`, and `multi_hop` questions, including their reworded variants.
- Value: mean recall@5 on that subset is at least 0.10 higher in arm B than in arm A (absolute).
- Breadth: at least 5 questions in that subset have higher recall@5 in arm B than in arm A.
- Code regression: at most 1 `code` question has lower recall@5 in arm B than in arm A.
- Latency: the p95 of the RAG request time per question, as measured by SOP's client, is 500 ms or less.
- `no_answer` and `injection` questions don't count toward recall. They count toward the §4 hard blockers.
- Results for the reworded questions are also reported as their own subset. They are informational and add no extra gate.

## 5. Required validation

- Deterministic unit/contract/negative/failure-injection tests for the adapter (RAG-SOP-002), including: corpus pre-filter correctness (no cross-corpus or NULL-corpus rows at any `top_k`), `content_hash` projection (hashed rows and legacy-NULL rows), `retrieval_method`/`rank` labeling, and `UNAUTHORIZED_SCOPE` rejection.
- A **required PostgreSQL integration** validation category for database-sensitive changes,
  enforced for RAG-SOP-002/004 — unit-only validation is never accepted as proof of pgvector
  operations.
- Recorded end-to-end fixtures: resumed runs, cold start, stale index, unavailable
  Ollama/PostgreSQL, malformed response, cross-repository scope, permission boundaries.
- `go test ./...` and `go test -race ./...` (RAM-backed `TMPDIR`) where the change is in
  `agentic-sop`.

## 6. Rollback

- Feature-flagged: `SOP_RAG_ENABLED=false` restores the exact baseline (no code revert
  required).
- The `rag-template` producer is independently deployable/removable; SOP degrades to
  BM25-only.
- No destructive schema migration is implied by enabling/disabling retrieval.

## 7. Decision artefact

RAG-SOP-004 produces a decision report (GO / CONDITIONAL / NO-GO) with per-case evidence,
reproducible commands and outputs, and documented tradeoffs, for explicit operator
approval before expanding usage.
