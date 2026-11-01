# RAG Template --- Lessons Learned

> **Non-normative retrospective.** This records the reasoning behind decisions
> already made. The normative sources are the
> [requirements](requirements/PRD.md) and the
> [reference](reference/commands.md) documents; the
> [documentation index](README.md) lists the whole set.

## Purpose

This document captures the major lessons learned while building the RAG
Template project.

It is intentionally different from the other project documents:

```text
docs/requirements/PRD.md    -> What are we building and why?
docs/plans/PLAN.md          -> What steps did we take and what comes next?
docs/lessons.md             -> What did those steps teach us?
```

The goal is to preserve not just the final architecture, but the
reasoning that led to it.

A useful pattern throughout the project was:

```text
What we originally thought
        |
Problem we discovered
        |
What we changed
        |
What we learned
```

---

# 1. Start With the Simplest RAG Mental Model

## Original idea

At its simplest, Retrieval-Augmented Generation looks like:

```text
Question
   |
Retrieve relevant information
   |
Build context
   |
LLM
   |
Answer
```

The first important realization was that the LLM itself is not searching
the database.

The application is responsible for finding relevant information and
supplying it to the model.

## Principle

**RAG is an application-controlled evidence pipeline around an LLM.**

The model is only one component.

---

# 2. Documents Need Searchable Units

## Problem

An entire document is usually too large and too broad to use as one
retrieval item.

## Change

Break documents into smaller units:

```text
Document
   |
Section
   |
Chunk
```

A chunk becomes the normal unit for embedding and initial retrieval.

## Lesson

Chunking is not merely text splitting. It determines what the retrieval
system is capable of finding.

Too large:

```text
More context
but less precise retrieval
```

Too small:

```text
More precise retrieval
but potentially incomplete evidence
```

## Principle

**Chunk size is a retrieval design decision, not just a storage
decision.**

---

# 3. Preserve Structure and Metadata

## Problem

A retrieved string without metadata does not tell us where it came from.

That makes it difficult to:

- reconstruct surrounding context;
- cite the source;
- evaluate retrieval;
- distinguish sections;
- inspect failures.

## Change

Store metadata such as:

```text
source
section
chunk_index
content
embedding
```

## Lesson

The vector itself is not enough.

RAG requires both semantic representations and document structure.

## Principle

**Metadata provides provenance and makes later context construction
possible.**

---

# 4. Embeddings Enable Semantic Retrieval

## Problem

A user may ask a question using words that do not exactly appear in the
source.

Keyword matching alone may miss the relevant passage.

## Change

Represent both chunks and questions as embeddings.

```text
Chunk Text -> Embedding
Question   -> Embedding
```

Then compare them using vector similarity.

## Lesson

Embeddings allow retrieval based on semantic meaning rather than exact
wording.

For example, a question about:

```text
missed payment
```

may still find content discussing:

```text
delinquency
```

even if the wording differs.

## Principle

**Embeddings are representations of meaning used for retrieval.**

---

# 5. Similarity Is Not Answer Confidence

This became one of the most important lessons in the project.

## Original assumption

A high vector similarity score feels like it should mean:

> We found the answer.

## Problem

A chunk can be highly related to a question without containing the fact
required to answer it.

Example:

```text
Question:
What is the late fee?

Retrieved chunk:
Discusses late payments and delinquency,
but never states the fee amount.
```

The chunk is relevant.

It is not sufficient.

## Change

Separate retrieval from answerability.

```text
Similarity
    !=
Answerability
```

## Principle

**Retrieval confidence and answer confidence are different concepts.**

---

# 6. Vector Search Is Not Enough

## Problem

Semantic search is powerful, but some queries depend heavily on exact
language.

Examples include:

```text
LP-4827
account codes
specific policy names
exact identifiers
special terminology
```

Vector retrieval may not always rank these optimally.

## Change

Add PostgreSQL full-text search.

Now there are two retrieval signals:

```text
Vector Search
    +
Keyword Search
```

## Lesson

The two approaches have complementary strengths.

```text
Vector search  -> semantic meaning
Keyword search -> exact lexical matches
```

## Principle

**Good retrieval often combines semantic and lexical search.**

---

# 7. Do Not Directly Add Incompatible Scores

## Problem

Once vector and keyword retrieval existed, the obvious temptation was:

```text
final_score =
    vector_similarity
    +
    keyword_score
```

But these values do not have the same meaning or scale.

A vector cosine similarity and PostgreSQL `ts_rank` are different
measurements.

## Change

Combine rankings instead of raw scores using Reciprocal Rank Fusion
(RRF).

```text
Vector Ranking ----+
                   |
                   +-> RRF -> Combined Ranking
                   |
Keyword Ranking ---+
```

RRF rewards documents that rank highly in one or more result lists.

## Lesson

Sometimes ranking position is more useful than trying to normalize
unrelated scoring systems.

## Principle

**Fuse rankings when underlying score scales are not directly
comparable.**

---

# 8. Top Chunks Can Be Redundant

## Problem

Suppose the top results are:

```text
1. Section A / chunk 1
2. Section A / chunk 2
3. Section A / chunk 3
4. Section B / chunk 1
```

If the final retrieval budget is only two results, Section A consumes
everything.

This may reduce evidence diversity.

## Change

Deduplicate by logical section before final selection.

```text
Ranked Chunks
      |
Section Dedup
      |
Distinct Sections
```

## Lesson

The best individual chunks do not necessarily form the best set of
context.

## Principle

**Ranking quality and context diversity are separate concerns.**

---

# 9. Retrieval Unit Is Not Context Unit

This was another major architectural lesson.

## Problem

Small chunks are useful for precise retrieval.

But a small chunk may not contain enough surrounding information to
answer the question correctly.

## Change

Use small-to-big retrieval.

```text
Small chunk
   |
Find relevant section
   |
Retrieve surrounding section chunks
   |
Larger context
```

This became section expansion.

## Lesson

We do not have to use the same unit for searching and answering.

## Principle

**Retrieval unit != context unit.**

Search small.

Answer with enough surrounding evidence.

---

# 10. Expansion Can Recover Missing Evidence

## Problem

A retrieved chunk may point to the correct section but omit a nearby
sentence containing the actual answer.

## Change

After selecting sections, retrieve additional chunks belonging to those
sections.

## Lesson

Section expansion can improve evidence recall without requiring initial
retrieval to return every answer-bearing chunk directly.

This changes the question from:

> Did search retrieve the exact sentence?

to:

> Did search locate the correct evidence neighborhood?

## Principle

**Retrieval can locate the region; expansion can reconstruct the
evidence.**

---

# 11. Context Needs Fairness

## Problem

Expansion can allow one large section to consume too much of the context
budget.

## Change

Allocate expansion across selected sections rather than blindly taking
all chunks from the first section.

## Lesson

Context construction needs its own policy.

Retrieval ranking alone should not completely determine context
allocation.

## Principle

**Context construction is a separate engineering stage from retrieval.**

This leads naturally to a future improvement: true token-based context
budgeting.

---

# 12. Answerability Must Be Explicit

## Problem

Even after good retrieval and expansion, some questions simply cannot be
answered from the available documents.

Examples include asking for facts that do not exist in the corpus.

Without a gate, an LLM may still produce a plausible response.

## Change

Introduce an answerability stage:

```text
Context
   |
Answerability
   +------+
   |      |
   No     Yes
   |      |
Reject  Generate
```

## Lesson

The system needs to decide whether evidence exists before asking the LLM
to formulate an answer.

## Principle

**Related evidence is not the same as sufficient evidence.**

---

# 13. Deterministic Checks Should Come Before LLM Judgment

## Problem

LLMs can help judge evidence, but they add latency, cost,
nondeterminism, and another possible source of error.

## Change

Use deterministic checks where possible before relying on an LLM judge.

## Lesson

LLMs are useful evaluators for semantic questions, but ordinary program
logic should handle facts that can be determined reliably without them.

## Principle

**Use deterministic logic for deterministic questions; use LLM judgment
where semantic reasoning is actually required.**

---

# 14. Production and Evaluation Must Share the Same Pipeline

## Problem

An evaluator can look excellent while testing retrieval logic different
from the production application.

That creates false confidence.

## Change

Create a shared retrieval pipeline used by both production execution and
evaluation.

Conceptually:

```text
              Retrieval Pipeline
                     |
       +-------------+-------------+
       |                           |
Production RAG                 Evaluator
```

## Lesson

Evaluation is meaningful only if it measures the system actually being
used.

## Principle

**Do not maintain separate "real" and "evaluation" retrieval behavior.**

---

# 15. "The Answer Looks Good" Is Not an Evaluation Strategy

## Original approach

Run a question.

Read the answer.

Decide whether it seems reasonable.

## Problem

This does not reveal which part of the pipeline succeeded or failed.

## Change

Measure retrieval separately.

Important metrics:

```text
Recall@K
Precision@K
```

### Recall@K

Did we retrieve the relevant material?

### Precision@K

How much of what we retrieved was actually relevant?

## Lesson

A RAG system needs measurable retrieval quality before generation
quality can be understood.

## Principle

**Evaluate retrieval independently from generation.**

---

# 16. Finding the Correct Source Is Not Enough

## Problem

A retrieval result can come from the correct document or section while
still missing the exact fact needed to answer.

## Change

Add expected evidence to evaluation cases.

Example:

```text
Question
Expected Source
Expected Section
Expected Evidence
```

Then measure evidence recall.

## Lesson

There are multiple levels of retrieval success:

```text
Correct document
      |
Correct section
      |
Answer-bearing evidence
```

## Principle

**Source recall and evidence recall are different metrics.**

---

# 17. Measure Evidence Before and After Expansion

## Problem

If final context contains the answer, we still want to understand how it
got there.

Was the evidence retrieved directly?

Or did section expansion recover it?

## Change

Measure:

```text
Evidence Recall Before Expansion
Evidence Recall After Expansion
```

## Lesson

This makes the value of small-to-big retrieval measurable.

## Principle

**Measure intermediate pipeline stages, not just final output.**

---

# 18. Good Context Does Not Guarantee a Good Answer

## Problem

Even if the correct evidence reaches the LLM, the model may omit an
important fact.

## Change

Add expected facts to answerable evaluation cases.

Example:

```text
expected_facts:
- grace period is 10 days
- late fee is $25
```

## Lesson

Retrieval success and generation success must be evaluated separately.

## Principle

**Good retrieval is necessary, but not sufficient, for a good RAG
answer.**

---

# 19. Expected Facts Do Not Guarantee Groundedness

## Problem

An answer might include all expected facts and still add unsupported
claims.

For example:

```text
Expected:
The grace period is 10 days.

Answer:
The grace period is 10 days,
and customers always receive a warning call.
```

The first statement may be supported.

The second may be invented.

## Change

Add groundedness evaluation.

## Lesson

Completeness and faithfulness are different.

## Principle

**An answer can be complete and still hallucinate.**

---

# 20. Citations Are Not Automatically Trustworthy

## Original assumption

If the answer includes a citation, that seems safer.

## Problem

A citation can:

- point to the wrong source;
- point to a source that was not retrieved;
- point to text that does not support the claim.

## Change

Evaluate citations in two ways.

### Citation validity

```text
Does this citation point to an actual
retrieved source/section?
```

### Citation entailment

```text
Does the cited evidence actually
support the associated claim?
```

## Lesson

Citation formatting is not the same as evidence quality.

## Principle

**A useful citation must both exist and support the claim.**

---

# 21. Conflicting Sources Are a Real RAG Problem

## Problem

Documents can disagree.

Example:

```text
Source A:
Grace period = 10 days

Source B:
Grace period = 15 days
```

The system should not silently select one and present it as undisputed
truth.

## Change

Add conflict cases to the evaluation dataset and require the system to
preserve evidence from multiple sources when appropriate.

## Lesson

RAG is not merely fact lookup. It must sometimes represent uncertainty
or disagreement present in the source material.

## Principle

**Grounding means faithfully representing the corpus, including
conflicts.**

---

# 22. Unanswerable Questions Are Essential Test Cases

## Problem

If every evaluation question has an answer, the system can appear strong
even if it always attempts to answer.

## Change

Add questions whose answers are intentionally absent from the corpus.

Examples include missing rates, fees, credit requirements, or unrelated
account information.

## Lesson

A trustworthy RAG system needs to know when not to answer.

## Principle

**Refusal based on missing evidence is a feature, not a failure.**

---

# 23. Larger Documents Expose Different Problems

## Problem

Small artificial documents make retrieval easier than realistic corpora.

## Change

Add larger and more realistic source documents.

## Lesson

Document size introduces:

- more competing chunks;
- repeated terminology;
- similar sections;
- more difficult ranking;
- larger expansion choices.

## Principle

**Evaluate against realistic document structure, not only toy
examples.**

---

# 24. Query Rewriting Is an Experiment, Not an Automatic Upgrade

## Idea

Rewrite a user's question into a query that might retrieve better
evidence.

```text
Original Question
       |
     Rewrite
       |
Retrieval Query
```

## Risk

The rewrite may:

- improve terminology;
- remove useful details;
- introduce assumptions;
- drift from user intent.

## Change

Support both original and rewritten retrieval and evaluate them.
`EVAL_REWRITE_ONLY` measures the alternative of replacing the original outright.

## Result

Rewriting improved recall (hybrid R@1 0.68 → 0.73) and fusing both beat
replacing the original (R@4 0.91 → 0.93), so it is on by default. The cost is
one chat-model call per query.

## Lesson

More LLM processing does not automatically improve RAG — but a measured
improvement can justify turning it on.

## Principle

**Treat query rewriting as a measurable retrieval strategy, then let the
numbers decide the default.**

---

# 25. Reranking Is Also an Experiment

## Idea

Retrieve a broad candidate set and use a second stage to improve
ordering.

```text
Initial Retrieval
       |
Candidates
       |
Reranker
       |
Final Ranking
```

## Cost

Reranking adds:

- latency;
- complexity;
- potentially additional model calls.

## Result

On the example corpus the LLM reranker raised recall at the 4→2 stage
(0.86 → 0.91) at roughly the same precision. It is a measured win, but off by
default: one chat-model call per answerable case.

## Lesson

A reranker is valuable only if it measurably improves the evidence
reaching the model.

## Principle

**Every additional RAG stage should justify its complexity with
measurable improvement.**

---

# 26. Evaluation Labels Can Be Wrong

## Problem

When an evaluation fails, the instinct may be to change the system.

But sometimes:

```text
System behavior is correct
        |
Evaluation expectation is incomplete
```

## Lesson

Evaluation datasets are software artifacts and can contain bugs.

Expected sources, evidence, and facts need inspection just like
production code.

## Principle

**Do not optimize the system blindly against an incorrect benchmark.**

---

# 27. RAG Quality Is a Chain

By the end of the project, the most useful mental model became:

```text
                         RAG QUALITY

User Question
     |
     v
+--------------+
|  Retrieval   |  Did we find the right area?
+------+-------+
       |
       v
+--------------+
|   Evidence   |  Did we get the actual facts?
+------+-------+
       |
       v
+--------------+
|Answerability |  Is there enough to answer?
+------+-------+
       |
       v
+--------------+
|  Generation  |  Did the LLM use it correctly?
+------+-------+
       |
       v
+--------------+
| Groundedness |  Did it introduce unsupported claims?
+------+-------+
       |
       v
+--------------+
|  Citations   |  Can the claims be verified?
+--------------+
```

Every stage can fail independently.

That means a bad final answer does not automatically mean:

```text
"The LLM is bad."
```

The failure may actually be:

```text
bad chunking
bad retrieval
missing lexical match
bad fusion
context starvation
missing evidence
incorrect answerability decision
generation omission
hallucination
bad citation
incorrect evaluation label
```

## Principle

**Debug RAG stage by stage.**

---

# 28. The Final RAG Mental Model

The project started with:

```text
Question -> Vector Search -> LLM -> Answer
```

It evolved into:

```text
Question
   |
   +----------------------+
   |                      |
   v                      v
Semantic Retrieval    Lexical Retrieval
   |                      |
   +----------+-----------+
              |
             RRF
              |
       Diverse Sections
              |
       Context Expansion
              |
        Answerability
              |
          Evidence
              |
             LLM
              |
     Grounded Answer
              |
          Citations
              |
         Evaluation
```

The important conceptual shift is:

> **RAG is not vector search plus an LLM. It is an evidence pipeline.**

---

# 29. What We Should Optimize Next

The core architecture is now sufficiently complete that adding more
components is not the priority.

The next step is controlled experimentation. That first pass is done: chunk
size and overlap, Candidate Top-K, Final-K, query rewriting, and reranking were
each swept and the results are recorded in `docs/reference/experiments.md`. Context budgeting
is still open.

## Context budgeting

Current chunk-count limits are an approximation.

A stronger system should reason about the actual context/token budget:

```text
Candidate Sections
       |
Prioritize
       |
Allocate Token Budget
       |
Final Context
```

## Controlled experiments

Change one variable at a time:

```text
chunk size
chunk overlap
Candidate Top-K
Final-K
query rewriting
reranking
context budget
```

Then measure:

```text
retrieval recall
retrieval precision
evidence recall
answerability
expected fact support
groundedness
citation validity
citation entailment
latency
token usage
```

## Principle

**Once the pipeline is measurable, optimization should be experimental
rather than intuitive.**

---

# 30. The Overall Engineering Lesson

The largest lesson from this project extends beyond RAG.

We repeatedly followed this process:

```text
Build simplest thing
       |
Observe failure
       |
Understand why
       |
Introduce one concept
       |
Measure the result
       |
Repeat
```

That progression produced:

```text
Vector Search
    |
Keyword Search
    |
Hybrid Retrieval
    |
RRF
    |
Section Dedup
    |
Section Expansion
    |
Answerability
    |
Evidence Evaluation
    |
Generation Evaluation
    |
Groundedness
    |
Citation Evaluation
    |
Controlled Experiments
```

Each component exists because a simpler version exposed a specific
limitation.

That is the most useful way to understand the final architecture:

> **Do not memorize the components. Understand the failure that caused
> each component to exist.**

---

# Quick Reference

Concept Main Lesson

---

Chunking Defines searchable units
Embeddings Represent semantic meaning
Vector search Finds semantically related text
FTS Finds exact lexical matches
Hybrid retrieval Combines complementary search strategies
RRF Combines rankings without mixing incompatible raw scores
Section dedup Improves context diversity
Section expansion Search small, answer with larger context
Answerability Relevance does not mean sufficient evidence
Recall Did we find what we needed?
Precision Was what we retrieved useful?
Evidence recall Did answer-bearing evidence reach context?
Expected facts Did generation include required information?
Groundedness Did generation stay within the evidence?
Citation validity Does the citation point to retrieved material?
Citation entailment Does the cited material support the claim?
Query rewriting Measured; improves recall, on by default
Reranking Measured; small recall gain, optional
Evaluation Measure every major stage independently
Context budgeting Control what evidence reaches the model

---

# Documents in the RAG Template Project

The documentation now has five distinct purposes:

```text
README.md
   |
   +-> What is this repository and how do I run it?

docs/requirements/PRD.md
   |
   +-> What are we building and why?

docs/plans/PLAN.md
   |
   +-> What stages did we build and what comes next?

docs/lessons.md
   |
   +-> What did we learn and why does the architecture look this way?

docs/reference/experiments.md
   |
   +-> What did we measure, and what did the numbers change?
```

Together they provide the product intent, engineering progression,
implementation context, and accumulated RAG knowledge behind the
project, and the measured evidence behind the defaults.
