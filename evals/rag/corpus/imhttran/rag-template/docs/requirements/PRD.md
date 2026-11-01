# RAG Template --- Product Requirements Document

## 1. Product Summary

RAG Template is a learning-oriented Retrieval-Augmented Generation (RAG)
reference system. It exposes the major mechanics of RAG instead of
hiding them behind a large framework.

The project demonstrates how documents are ingested, chunked, embedded,
retrieved, expanded into useful context, passed to an LLM, and evaluated
for retrieval quality, answerability, groundedness, and citation
quality.

## 2. Problem

A basic RAG demo can produce plausible answers while hiding important
engineering questions:

-   Did retrieval find the right source?
-   Did the final context actually contain the answer?
-   Did semantic search miss an exact identifier?
-   Did multiple chunks from one section crowd out other useful
    evidence?
-   Was a retrieved passage merely related, or did it actually answer
    the question?
-   Did the LLM use the evidence correctly?
-   Are citations valid and do they support the claims?
-   Did a retrieval change actually improve quality?

The project needs to make these questions observable and measurable.

## 3. Target User

The primary user is a backend/software engineer learning practical RAG
engineering who is already comfortable with services, APIs, SQL,
databases, and production architecture but wants a first-principles
understanding of RAG-specific concepts.

## 4. Product Goals

The project shall:

1.  Demonstrate the complete RAG lifecycle.
2.  Preserve source and document structure throughout ingestion and
    retrieval.
3.  Support semantic/vector retrieval.
4.  Support lexical/full-text retrieval.
5.  Combine complementary retrieval methods using hybrid search.
6.  Build larger answer context from smaller retrieval units.
7.  Distinguish retrieval relevance from answerability.
8.  Prevent unsupported generation when evidence is insufficient.
9.  Produce grounded answers with traceable citations.
10. Evaluate retrieval, evidence, generation, groundedness, and
    citations independently.
11. Support controlled experiments with retrieval strategies and
    parameters.
12. Keep the implementation explicit enough to study without a large RAG
    framework.

## 5. Non-Goals

The project is not intended to be:

-   a multi-user SaaS application;
-   a general document-management product;
-   a distributed vector-search platform;
-   an agent framework;
-   a production-scale ingestion system;
-   a microservices or Kubernetes demonstration;
-   a replacement for RAG frameworks such as LangChain or LlamaIndex.

## 6. Core User Stories

-   As a learner, I want to ingest documents and inspect how they become
    sections, chunks, and embeddings.
-   As a learner, I want to inspect vector retrieval independently.
-   As a learner, I want to inspect keyword retrieval independently.
-   As a learner, I want to combine retrieval methods and understand why
    hybrid retrieval works.
-   As a learner, I want retrieval results expanded into useful answer
    context.
-   As a learner, I want questions rejected when retrieved evidence
    cannot actually answer them.
-   As a learner, I want generated answers grounded in supplied
    evidence.
-   As a learner, I want citations tied to retrieved sources.
-   As a learner, I want repeatable evaluations so I can measure whether
    a change improves the system.

## 7. Functional Requirements

### FR-1 --- Structured ingestion

The system shall ingest documents while retaining source, section, and
chunk-order metadata.

### FR-2 --- Chunking

The system shall divide documents into searchable chunks suitable for
embedding and retrieval.

### FR-3 --- Embeddings

The system shall create embeddings for document chunks and user queries.

### FR-4 --- Vector retrieval

The system shall retrieve semantically similar chunks using pgvector
cosine similarity.

### FR-5 --- Keyword retrieval

The system shall retrieve lexically relevant chunks using PostgreSQL
full-text search.

### FR-6 --- Hybrid retrieval

The system shall combine independently ranked retrieval lists using
Reciprocal Rank Fusion (RRF), avoiding direct arithmetic combination of
incompatible vector and FTS score scales.

### FR-7 --- Section deduplication

The system shall avoid allowing repeated chunks from one logical section
to consume the final selection budget unnecessarily.

### FR-8 --- Section expansion

The system shall use selected retrieval results as seeds and retrieve
additional chunks from their logical sections before answer generation.

### FR-9 --- Shared retrieval pipeline

Production execution and evaluation shall use the same core retrieval
pipeline so evaluation reflects actual application behavior.

### FR-10 --- Similarity filtering

The system shall support configurable minimum semantic similarity
filtering where applicable.

### FR-11 --- Query rewriting

The system shall optionally support rewritten queries so original-query
and multi-query retrieval can be compared experimentally.

### FR-12 --- Reranking

The system shall optionally support reranking after candidate retrieval.

### FR-13 --- Answerability

The system shall determine whether final context contains sufficient
evidence to answer the question.

### FR-14 --- Grounded generation

The LLM shall be instructed to answer using supplied evidence rather
than unsupported external knowledge.

### FR-15 --- Citations

Generated answers shall identify supporting source/section information.

### FR-16 --- Retrieval evaluation

The evaluator shall measure vector and hybrid retrieval recall and
precision.

### FR-17 --- Evidence evaluation

Evaluation cases shall support expected answer-bearing evidence and
measure evidence recall before and after expansion.

### FR-18 --- Expected-fact evaluation

Answerable evaluation cases shall support expected facts and measure
whether generated answers contain supported expected facts.

### FR-19 --- Answerability evaluation

The evaluator shall measure answerable/unanswerable decisions and
confusion counts.

### FR-20 --- Groundedness evaluation

The evaluator shall determine whether generated factual claims are
supported by supplied context.

### FR-21 --- Citation validity

The evaluator shall determine whether citations refer to retrieved
source material.

### FR-22 --- Citation entailment

The evaluator shall determine whether cited evidence actually supports
the associated answer claim.

## 8. Core Retrieval Model

``` text
Question
   |
   +-------------------+
   |                   |
   v                   v
Vector Search      Keyword Search
   |                   |
   +---------+---------+
             |
             v
            RRF
             |
             v
      Section Dedup
             |
             v
      Section Expansion
             |
             v
       Answerability
             |
             v
          Context
             |
             v
            LLM
             |
             v
     Answer + Citations
```

An optional rewritten-query path and optional reranking stage may
augment this base flow.

## 9. Key Product Principles

### Retrieval relevance is not answerability

A semantically related chunk is not necessarily sufficient evidence for
an answer.

### Retrieval unit is not necessarily context unit

Small chunks can be effective for finding relevant locations while
larger section context can be better for answering.

### Hybrid retrieval combines complementary signals

Vector retrieval helps with semantic meaning; lexical retrieval helps
with exact language, identifiers, and terms.

### Ranking scores are not interchangeable

Vector similarity, FTS ranking, and RRF fusion scores represent
different concepts and should not be treated as one common confidence
scale.

### Evaluation must follow the entire RAG chain

``` text
Did we retrieve it?
        |
Did context contain the evidence?
        |
Did the model use the evidence?
        |
Did it introduce unsupported claims?
        |
Did it cite the correct evidence?
```

## 10. Quality Requirements

The implementation should favor:

-   explicit behavior over hidden framework behavior;
-   deterministic logic before LLM-as-judge checks where practical;
-   deterministic ranking tie-breaking;
-   shared production/evaluation behavior;
-   configurable experiment parameters;
-   reproducible evaluation;
-   readable component boundaries;
-   observable intermediate retrieval stages.

## 11. Evaluation Requirements

The evaluation dataset should contain a mix of:

-   straightforward answerable questions;
-   paraphrased questions;
-   exact-identifier questions;
-   multi-source questions;
-   multi-fact questions;
-   conflicting-source questions;
-   unanswerable questions;
-   questions against larger, more realistic documents.

Answerable cases should contain expected facts where practical.
Evidence-bearing cases should identify the phrases or passages required
to answer the question.

## 12. Success Criteria

The project is successful when the learner can:

1.  Explain ingestion and query-time RAG flows.
2.  Explain documents, sections, chunks, embeddings, and metadata.
3.  Inspect semantic and lexical retrieval independently.
4.  Explain why hybrid retrieval and RRF are used.
5.  Explain section deduplication and expansion.
6.  Distinguish relevance, similarity, answerability, and groundedness.
7.  Generate grounded answers with citations.
8.  Run the evaluator and interpret its metrics.
9.  Change a retrieval variable and objectively measure the result.
10. Trace a bad answer back to retrieval, context, answerability,
    generation, or citation behavior.

## 13. Current V1 Completion Definition

The core RAG learning system is considered functionally complete when it
includes:

-   document chunking and embeddings;
-   PostgreSQL + pgvector semantic retrieval;
-   PostgreSQL full-text retrieval;
-   RRF hybrid fusion;
-   section deduplication;
-   section expansion;
-   shared production/evaluation retrieval;
-   answerability gating;
-   grounded answer generation;
-   citation support;
-   retrieval and evidence metrics;
-   expected-fact evaluation;
-   groundedness evaluation;
-   citation validity and entailment evaluation;
-   optional query rewriting;
-   optional reranking;
-   retrieval pipeline tests.

The next product phase is optimization and experimentation rather than
adding unmeasured retrieval features.

## 14. Next-Phase Candidates

> **Status note (implementation):** the first six items below have landed since
> this PRD was written; they are kept for history and marked inline. The
> remaining product-shaping work (PDF, page provenance, OCR, multilingual
> evaluation, genealogy) is scoped in `docs/requirements/PRD-Phase-23b.md`.

Future work may include:

-   ~~true token/context budgeting~~ — **implemented** as a provider-neutral
    byte-estimate budget (RAG-010);
-   ~~controlled chunk-size experiments~~ — **implemented** (`make sweep`);
-   ~~overlap experiments~~ — **implemented**;
-   ~~Top-K and Final-K experiments~~ — **implemented**;
-   ~~query-rewrite A/B comparisons~~ — **implemented**;
-   ~~reranking A/B comparisons~~ — **implemented**;
-   larger and more diverse corpora;
-   additional embedding models;
-   latency and resource measurements;
-   production-oriented observability and deployment;
-   **PDF text-layer ingestion, page provenance, and optional OCR** — proposed in
    `docs/requirements/PRD-Phase-23b.md` (not implemented);
-   **multilingual (Vietnamese/English) retrieval evaluation** — proposed;
-   **genealogy entity/person extraction** — a separate future scope, not part of
    Phase 23b (`docs/requirements/PRD-Phase-23b.md` §"Out of scope").
