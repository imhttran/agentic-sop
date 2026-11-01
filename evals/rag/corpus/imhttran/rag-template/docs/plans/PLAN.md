# RAG Template --- Engineering and Learning Plan

## Purpose

This plan reconstructs the logical progression of the existing RAG
Template project and records why each layer was added. Completed phases
document the architecture already built; remaining phases define the
next measured experiments.

``` text
Problem -> Concept -> Architecture Change -> Verification -> Next Problem
```

## Final Target Flow

``` text
Documents -> Structure -> Chunks -> Embeddings -> PostgreSQL/pgvector

Question
   +-> Vector Search ------+
   +-> Keyword Search -----+-> RRF -> Section Dedup -> Section Expansion
                                              |
                                      Optional Reranking
                                              |
                                        Answerability
                                              |
                                           Context
                                              |
                                             LLM
                                              |
                                    Answer + Citations
                                              |
                                         Evaluation
```

# Phase 1 --- Basic RAG Foundation

**Status: Complete**

Build the smallest complete loop:

``` text
Documents -> Chunks -> Embeddings -> Vector Search -> Context -> LLM
```

Learn document, chunk, embedding, vector, cosine similarity, context,
and RAG. The key lesson is that the LLM does not search the database;
the application retrieves evidence and supplies it to the model.

# Phase 2 --- Structured Ingestion

**Status: Complete**

Preserve source, section, chunk index, and content metadata.

``` text
Document -> Section -> Chunk -> Embedding + Metadata
```

This makes provenance, filtering, context reconstruction, citations, and
evaluation possible.

# Phase 3 --- Semantic Retrieval

**Status: Complete**

Use PostgreSQL/pgvector cosine search so questions can find semantically
similar passages even when wording differs.

Key lesson: embedding similarity is a retrieval signal, not proof that a
passage answers the question.

# Phase 4 --- Keyword Retrieval

**Status: Complete**

Add PostgreSQL full-text search for exact identifiers, names, codes, and
literal terms that semantic retrieval may rank poorly.

Key lesson: vector and lexical retrieval have complementary strengths.

# Phase 5 --- Hybrid Retrieval with RRF

**Status: Complete**

Combine independently ranked vector and keyword lists using Reciprocal
Rank Fusion.

``` text
Vector Ranking ---+
                  +-> RRF -> Fused Ranking
Keyword Ranking --+
```

Do not directly add vector similarity and FTS scores because they
represent different scales.

# Phase 6 --- Section Deduplication

**Status: Complete**

Prevent multiple chunks from one section from consuming the final
selection budget and excluding other useful sections.

Key lesson: chunk-level ranking and context diversity are different
concerns.

# Phase 7 --- Small-to-Big Section Expansion

**Status: Complete**

Use small chunks to locate relevant areas and larger section context to
answer.

``` text
Retrieval Chunk -> Section -> Section Chunks -> Answer Context
```

Key lesson: **retrieval unit != context unit**.

# Phase 8 --- Answerability Gate

**Status: Complete**

Distinguish semantic relevance from sufficient evidence.

``` text
Context -> Answerability -> Reject OR Generate
```

Similarity thresholds alone cannot reliably determine whether a question
is answerable.

# Phase 9 --- Shared Production/Evaluation Pipeline

**Status: Complete**

Centralize vector search, keyword search, RRF, section deduplication,
and expansion so production and evaluation exercise the same retrieval
behavior.

# Phase 10 --- Retrieval Evaluation

**Status: Complete**

Measure retrieval independently of generation with Recall@K and
Precision@K for vector and hybrid retrieval.

# Phase 11 --- Evidence Evaluation

**Status: Complete**

Label answer-bearing evidence and measure whether it survives candidate
retrieval and section expansion.

Track evidence recall before and after expansion.

# Phase 12 --- Expected-Fact Evaluation

**Status: Complete**

Define expected facts for answerable questions and check whether
generated answers contain the supported facts.

Key lesson: good retrieval does not automatically produce good
generation.

# Phase 13 --- Groundedness Evaluation

**Status: Complete**

Check whether generated factual claims are supported by supplied
context, including unsupported claims beyond the expected facts.

# Phase 14 --- Citation Evaluation

**Status: Complete**

Evaluate two separate properties:

``` text
Citation Validity   -> Does the cited source exist in retrieved context?
Citation Entailment -> Does that evidence actually support the claim?
```

# Phase 15 --- Harder Evaluation Cases

**Status: Complete**

Expand the dataset with paraphrases, exact identifiers, multi-source
questions, conflicts, multi-fact questions, larger documents, and
unanswerable questions.

# Phase 16 --- Query Rewriting

**Status: Complete as an optional experiment**

Compare original-query retrieval with rewritten/multi-query retrieval.
Treat rewriting as an experiment, not an assumed improvement.

# Phase 17 --- Reranking

**Status: Complete as an optional experiment**

Experiment with lexical/LLM reranking after candidate discovery and
before final context construction. Measure whether additional latency
and complexity improve quality.

# Phase 18 --- Pipeline and Answerability Tests

**Status: Complete**

Protect fusion, deduplication, expansion, multi-query behavior,
answerability parsing, and important integration behavior from
regressions.

# Phase 19 --- True Context Budgeting

**Status: Implemented** (RAG-010, `internal/contextbudget`). The budget is a
provider-neutral **byte estimate** (content length plus a fixed per-chunk
overhead), not character- or tokenizer-accurate counting; `CONTEXT_BUDGET=0`
(the default) keeps the chunk-count behaviour. See `docs/plans/PLAN-RAG-010.md`.

## Problem

Chunk-count limits approximate rather than measure the context actually
consumed by the model. Section expansion may also exceed a nominal total
chunk limit when many selected sections each receive at least one chunk.

## Goal

Create a deterministic context builder with an explicit total budget.

Questions to test: - tokens vs characters as the budget; - allocation
across sections; - whether higher-ranked sections receive more budget; -
how fairness interacts with the total limit.

## Verification

Compare evidence recall, answer quality, token usage, and latency
against the current baseline.

# Phase 20 --- Controlled Retrieval Experiments

**Status: Implemented** (`scripts/sweep.sh`, `make sweep`). Axes cover chunk
size/overlap, TOP_K, FINAL_K, rewrite, rerank, and MIN_SIMILARITY; results are
recorded in `docs/reference/experiments.md`.

Change one variable at a time and run the same evaluation suite.

  Variable          Experiments
  ----------------- ------------------------
  Chunk size        50 / 100 / 200
  Chunk overlap     controlled values
  Candidate Top-K   controlled range
  Final-K           controlled range
  Query rewrite     off / on
  Reranking         off / on
  Context budget    controlled token sizes

Process:

``` text
Baseline
   |
Change one variable
   |
Run full evaluation
   |
Compare metrics
   |
Inspect regressions
   |
Keep or reject
```

Compare retrieval recall/precision, evidence recall, answerability,
expected-fact support, groundedness, citation validity/entailment,
latency, and context/token usage.

# Phase 21 --- Larger Corpus Validation

**Status: Implemented** (RAG-018, RAG-021). The example corpus was expanded with
long-section and Vietnamese documents, the evaluation dataset grew to a bilingual
set with unanswerable and misleading-context cases, and a calibrated
embedding-model comparison was recorded (see `docs/plans/PLAN-RAG-018.md`,
`docs/plans/PLAN-RAG-021.md`, and `docs/experiments-eval-sweep.md`).

# Phase 22 --- Production-Oriented Concerns

**Status: Partly implemented.** Idempotent re-indexing and schema migrations
(RAG-003), provider abstraction (RAG-004/005), observability (RAG-014), and
failure handling and security limits (RAG-012/015) have landed, as have page
provenance (RAG-017), the expanded bilingual evaluation harness (RAG-018), and the
calibrated embedding-model comparison (RAG-021); deployment and access controls
remain future/optional.

Only after learning and optimization objectives are satisfied, consider
ingestion lifecycle, idempotent re-indexing, schema migrations,
observability, latency budgets, provider abstraction, failure handling,
deployment, and access controls.

# Completion Checklist

## Core RAG

-   [x] Structured documents
-   [x] Chunking
-   [x] Embeddings
-   [x] Vector search
-   [x] PostgreSQL FTS
-   [x] Hybrid retrieval
-   [x] RRF
-   [x] Section deduplication
-   [x] Section expansion
-   [x] Answerability
-   [x] Grounded generation
-   [x] Citations

## Evaluation

-   [x] Recall / precision
-   [x] Answerable and unanswerable cases
-   [x] Evidence labels and evidence recall
-   [x] Expected facts
-   [x] Groundedness
-   [x] Citation validity
-   [x] Citation entailment
-   [x] Conflict cases
-   [x] Larger realistic document cases

## Experiments

-   [x] Query rewriting capability
-   [x] Reranking capability
-   [x] True context budgeting (byte-estimate; RAG-010)
-   [x] Controlled chunk-size experiments
-   [x] Controlled overlap experiments
-   [x] Controlled Top-K / Final-K experiments
-   [x] Rewrite A/B results
-   [x] Reranking A/B results

# Working Method Going Forward

For each remaining experiment:

``` text
1. State the hypothesis
2. Record the baseline
3. Change one variable
4. Run the same evaluation suite
5. Compare relevant metrics
6. Inspect individual regressions
7. Keep or revert the change
8. Document what was learned
```

The goal now is not to make the RAG pipeline more complicated. It is to
determine which choices measurably improve it.
