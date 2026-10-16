# Required PostgreSQL Integration Validation Category — Deterministic File-Pattern Policy

**Type:** Validation policy specification (RAG-SOP-004 task 1).
**Status:** Implemented — deterministic, config/file driven.

This policy defines the required **PostgreSQL integration** validation category for
migration and database-sensitive changes. It is deliberately **separate from retrieval
quality**: it never inspects recall, citations, latency, or any RAG metric, and unit-only
validation is **never** accepted as proof of pgvector operations.

## 1. Deterministic classification

Classification is a pure function of the changed paths matched against file patterns.
No model output, no heuristics, no retrieval-quality signal participates.

| File pattern | Category |
| --- | --- |
| `**/migrations/**` | postgres-integration |
| `**/*.sql` | postgres-integration |
| `**/migrate*.go` | postgres-integration |
| `**/schema*.go` | postgres-integration |
| `**/pg*.go` | postgres-integration |
| `**/vector*.go` | postgres-integration |
| `**/*pgvector*` | postgres-integration |

- When at least one changed path matches, the run **requires** the
  `postgres-integration` validation category.
- A unit-only suite does not satisfy the category. The category is satisfied only by a
  validation command classified as the integration category (real PostgreSQL/pgvector
  operations).
- When the category is required and no such command is configured, the deterministic
  quality gate fails closed: `required validation is not configured`
  (see `internal/quality/quality.go`, `Input.ValidationRequired && !Input.ValidationConfigured`).

## 2. Non-database changes

A change whose paths match none of the patterns does **not** acquire the
`postgres-integration` category and is unaffected by this policy.

## 3. Separation from retrieval quality

- The policy is expressed in configuration/files only.
- It consumes changed paths, never retrieval-quality metrics.
- Passing or failing retrieval-quality evaluation cannot satisfy or waive this category.
- Conversely, this category is not a retrieval-quality metric and is not reported as one.

## 4. Enforcement summary

1. Compute changed paths for the task.
2. Match against the deterministic patterns above.
3. If matched: mark `postgres-integration` required; require a category-matching
   validation command; fail closed if none is configured.
4. If not matched: leave the category unrequired.

No code path lets unit-only validation stand in for the `postgres-integration` category.
