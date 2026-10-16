scenario: Small stable corpus where full-text search already answers
expect: NO-RAG
compare_existing: false
requires:
  - no-rag
  - full-text
  - sql
---
An administrative tool keeps a few hundred policy documents in PostgreSQL. Staff look
them up by title and by exact clauses they already know ("clause 4.2"), and a ranked
full-text column returns the right row every time. The corpus changes a few times a
year. Nobody can name a question the current search fails to answer, and the team does
not want to operate an embedding provider or maintain an index.
