# Implementation Plan

## Project

RAG-025 shaped migration fixture

## Summary

A plan shaped like the RAG-025 family: every stage declares deliverables but no
Acceptance Checks. It must load and validate while being flagged with warnings.

## RAG025-001 - Index the corpus

Build the retrieval index over the corpus.

### Dependencies

None

### Deliverables

- retrieval index
- index build command

### Acceptance Criteria

The index returns ranked results for a known query.

## RAG025-002 - Serve retrieval results

Expose retrieval over the indexed corpus.

### Dependencies

- RAG025-001

### Deliverables

- retrieval endpoint

### Acceptance Criteria

The endpoint returns the top-k results.
