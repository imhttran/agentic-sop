scenario: Go service with no retrieval stack
expect: RAG
compare_existing: false
requires:
  - go
  - postgresql
  - retrieval service
---
A Go microservice serves an internal API over PostgreSQL and has no search or AI
component at all. It must answer operator questions over a growing set of runbooks and
incident notes — tens of thousands of short documents. The team is Go-only and does not
want a Python runtime in the deployment. The database is already PostgreSQL, and a
small internal retrieval service is acceptable if it keeps the embedding runtime out of
the API process.
