scenario: Spring Boot service backed by PostgreSQL
expect: RAG
compare_existing: false
requires:
  - spring
  - java
  - postgresql
  - pgvector
---
A Spring Boot REST service on Java 21 persists domain records in PostgreSQL. Documents
are uploaded as PDFs and stored in object storage; their metadata rows live in the same
database. There is no search feature today — listing is paginated SQL over a `status`
column. Users will ask paraphrased questions ("what does our refund policy say about
gift cards?") that rarely reuse the wording of the document. Deployment is one
container beside the database, and the team does not want a second search cluster.
