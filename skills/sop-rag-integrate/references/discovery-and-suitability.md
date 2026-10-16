# Discovery and suitability — RAG or NO-RAG

Optional reference for `/sop-rag-integrate`. Read it when the request needs a
suitability decision, or when the project's current search is unclear. It is a
checklist, not a script: the operator or the SOP-governed implementation supplies any
command, in the target repository.

## Discovery checklist

Answer these before proposing anything. Record each answer and where it came from; an
assessment that cannot name the project's **current** search is incomplete.

### The application

- Language, framework, runtime, and build tool (for example Java/Spring Boot, a Python
  service, a Go binary, or a JavaScript/TypeScript app).
- Database and schema: which store holds the documents and their metadata today, and
  whether it already has a vector extension (for example PostgreSQL + pgvector) or a
  managed search/vector service.
- Deployment environment, operational appetite, and who operates it.

### The documents and the questions

- Where documents come from, how they arrive, and how they change or are deleted.
- Their size, count, growth rate, and whether they are text, code, or binary.
- The concrete questions users ask, and the evidence a good answer must cite.
- Whether answers must be grounded in the project's own corpus or may use general model
  knowledge.

### The existing search

Classify the current mechanism precisely — this drives the whole decision:

- **Conventional SQL** over the existing tables (filters, joins, `LIKE`, ranking columns).
- **Full-text / lexical search** the database already supports (for example a
  `tsvector` column and a ranked query).
- **An existing search service** (a search engine or hosted index the application runs).
- **Nothing** — the feature is greenfield.

### Security, tenancy, and governance

- Authentication and authorization: who may retrieve which documents.
- Tenants or projects: must one tenant's documents ever be visible to another?
- Data classification, residency, and whether embeddings may leave the environment.
- The approval path for consequential architectural, database, security, or deployment
  changes.

## Suitability rubric

Walk the options cheapest-first and stop at the first one that answers the in-scope
questions:

| Order | Option | Sufficient when |
| --- | --- | --- |
| 1 | Conventional SQL | The questions are structured and the relevant rows are already indexed by keys. |
| 2 | Full-text / lexical search | Users search by words, names, or identifiers that appear verbatim, and the corpus is a few thousand documents or fewer. |
| 3 | An existing search service | The application already runs a search engine that the team already operates. |
| 4 | Vector or hybrid retrieval | Questions are truly semantic or cross-lingual, the vocabulary does not overlap the documents, and options 1–3 have been shown to miss. |

## NO-RAG triggers

Recommend **NO-RAG** and stop when **any** of these holds:

- The corpus is small and stable and full-text search over it is already fast and
  accurate.
- The questions are structured and conventional SQL answers them.
- A search service the application already runs meets the requirement.
- The vocabulary matches: users query by the exact terms that appear in the documents.
- The operational cost (an embedding provider, an index to maintain, re-indexing on
  change, latency and cost budgets) is not justified by a measured need.
- No one can state a question the current search fails to answer.

NO-RAG is a first-class outcome. Record the trigger you relied on, so the decision is
reviewable later if the corpus or the questions change.

## Signals that justify retrieval augmentation

Retrieval augmentation is worth evaluating when **several** of these hold:

- Questions are paraphrased or cross-lingual and share little vocabulary with the
  documents.
- The corpus is large enough that lexical ranking surfaces the wrong documents.
- Users need grounded answers with citations across many sources.
- Options 1–3 have been tried and their misses are documented.

Even then, the recommendation is *evaluate*, not *assume*: the benefit must survive a
small, reproducible comparison against the existing search (see
[`evaluation.md`](evaluation.md)).

## Recording the decision

State the decision explicitly: **NO-RAG** or **retrieval augmentation**, the trigger or
signal that drove it, the current search you compared against, and the assumptions that
would change the answer. If the decision is NO-RAG, the deliverable is that finding and
the reasoning — there is no follow-on plan to produce.
