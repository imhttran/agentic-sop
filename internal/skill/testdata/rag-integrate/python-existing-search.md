scenario: Python service that already runs a search engine
expect: RAG
compare_existing: true
requires:
  - python
  - existing search
  - compare
---
A Python (FastAPI) application indexes its knowledge base in a search cluster the team
already operates. Lexical search works well for product names and error codes but is
reported to miss paraphrased support questions. The team wants to know whether adding
embeddings is worth it, and will accept a NO-RAG answer if their existing search is
already sufficient. Any proposal must be judged against the current search results on
the same questions, with latency and cost recorded.
