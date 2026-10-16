scenario: Multi-tenant SaaS requiring corpus isolation
expect: RAG
compare_existing: false
requires:
  - tenant isolation
  - corpus
---
A multi-tenant SaaS stores every customer's documents in one PostgreSQL database with a
tenant column. Support agents may search only their own tenant's documents, and a leak
across tenants would be a security incident. The product wants semantic search over
uploaded contracts, with answers that cite the exact clause. Any design must prove
tenant isolation with a negative test and enforce a per-tenant scope on every query,
and it must ship disabled by default behind configuration.
