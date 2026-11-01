# PRD — Phase 23b: PDF, page provenance, OCR, and multilingual evaluation

**Type:** Product requirements document (planning only; makes no runtime change).
**Status:** **PROPOSED — NOT ACTIVE.** Nothing here is implemented, scheduled, or
approved. RAG-007, RAG-008, and RAG-016 remain **frozen** until the operator
explicitly opens Phase 23b.
**Supersedes:** none. It **extends** `docs/requirements/PRD.md` (§14) and sequences the
remaining M3/M6 work of `docs/plans/PLAN-RAG-Gap-Closure.md` (Amendments B/D).
**Related:** `docs/plans/PLAN-RAG-Phase-23b.md` (execution plan),
`docs/plans/PLAN-RAG-Gap-Closure.md` (audit; Amendment E records what already
landed).

---

## 1. Purpose

RAG-006…RAG-015 closed the robustness, model-independence, retrieval-quality,
observability, and security gaps. The remaining gaps are **document formats and
evaluation breadth**: the loader seam (RAG-006) exists but ships only Markdown and
plain text, so PDF (and scanned) records cannot enter the corpus, citations
cannot point at a page, non-English corpora are only partly addressed (RAG-009
added language-aware FTS but no evaluation), and the evaluation suite does not yet
cover the new capabilities.

This PRD defines the product requirements for closing them. It is deliberately
**model- and provider-independent** and reuses the existing pipeline wherever
possible.

## 2. Goals

1. **PDF text-layer ingestion** — a PDF with a text layer becomes sections, one
   per page (or per detected section), with its source identity preserved.
2. **Page provenance** — a chunk can be traced to the page it came from, from the
   loader through ingestion to retrieval and citations.
3. **Page-aware citations** — `cmd/rag` answers can cite a page, and the citation
   validator (RAG-013) validates page references the same way it validates
   source/section.
4. **Scanned-PDF detection and optional OCR** — a PDF with no text layer fails
   explicitly ("no text layer; OCR required"); an opt-in OCR path handles scans.
5. **Vietnamese/English handling** — a Vietnamese + English corpus ingests and
   retrieves acceptably, with the embedding-model decision recorded.
6. **Evaluation coverage** — the suite covers PDF, page citations, multilingual,
   and the RAG-009/010/011 capabilities, with variance reported.

## 3. Non-Goals

- **Genealogy entity/person extraction** (people, relationships, places, dates,
  GEDCOM/gramps) is **out of scope** for Phase 23b. The substrate built here
  (provenance, page/section citations, metadata scoping, multilingual handling)
  enables it, but the entity model is a **separate future phase** with its own PRD.
  This mirrors `PLAN-RAG-Gap-Closure.md` §8 (constraints only).
- No change to embedding **dimensions** or the default embedding **model** in this
  phase (a model swap is a recorded decision, not a default change — see §7).
- No new provider, no distributed/vector-platform work, no SaaS/document-management
  surface (`docs/requirements/PRD.md` §5).
- No destructive migration; all schema changes are additive.

## 4. Requirements

### 4.1 PDF text-layer ingestion
- A registered PDF loader extracts the text layer of a PDF and returns
  `[]document.Section`.
- Text-layer PDFs ingest successfully; the source identity is preserved
  (`a/doc.pdf` and `b/doc.pdf` stay distinct).
- A PDF with no extractable text layer returns an **explicit** error naming the
  file and stating that OCR is required — never a silent empty ingest.
- Malformed/truncated PDFs fail with an actionable error, not a panic.

### 4.2 Page provenance
- Each section carries an explicit **page** identifier (1-based) where the format
  has pages; Markdown/plain-text sections carry "no page" (unset), preserving
  current behaviour.
- Page provenance is stored on every chunk (`documents.page`, additive) and
  round-trips through ingestion idempotency (a re-ingest with unchanged content is
  still a no-op).
- Page provenance must be defined **before** `PDFLoader` is implemented (§6).

### 4.3 Page-aware citations
- `cmd/rag` may cite a page when the cited document has one; the rendering and
  parsing extend the existing `[source - section]` contract to an explicit page
  form **without breaking** the current form for documents that have no page.
- The citation validator (RAG-013, `internal/citations`) validates page references
  against the expanded set: a citation naming a page that was not retrieved is
  invalid and is repaired/stripped like any other invalid citation.
- Legacy answers with no page citations remain valid.

### 4.4 Scanned-PDF detection and optional OCR
- The PDF loader detects the absence of a text layer and reports it distinctly.
- OCR is **opt-in**: enabled by an explicit setting and an available engine; when
  the engine is absent, the OCR path is **skipped with a clear note**, not an error
  that aborts a batch.
- OCR output carries a documented quality caveat and preserves page provenance.

### 4.5 Vietnamese/English handling
- A mixed Vietnamese + English corpus ingests (language metadata per document) and
  retrieves; Vietnamese keyword search uses the `simple` FTS configuration
  (PostgreSQL has no Vietnamese configuration) and English keeps `english`.
- The embedding-model choice for Vietnamese is **measured**, not assumed: the
  English-centric default is acceptable only if the evaluation (§4.6) shows it; a
  multilingual model is adopted only with a recorded decision and a migration
  note (no default drift).

### 4.6 Evaluation
- `evals/retrieval.json` (or a successor) covers PDF cases, page-citation cases,
  a non-English corpus, and the RAG-009/010/011 capabilities.
- Metrics print for the new cases; run-to-run variance is reported for the
  nondeterministic paths (query rewrite, LLM judges).
- The suite runs within the documented time budget, or a documented fast subset is
  defined.

### 4.7 Backward compatibility
- Markdown and plain-text ingestion is **byte-identical** to today when no new
  format is used and no new setting is set.
- Every new setting defaults to the current behaviour.
- Existing `evals/retrieval.json` cases and their expectations are unchanged.

## 5. Acceptance Criteria (product level)

| # | Criterion |
|---|-----------|
| 1 | A text-layer PDF ingests with page provenance; a scanned PDF fails explicitly. |
| 2 | `documents.page` round-trips; a re-ingest of unchanged content is a no-op. |
| 3 | A page citation resolves to a retrieved page/section; an invalid page citation is never shown as valid. |
| 4 | With OCR disabled/unavailable, OCR is skipped with a note; with it enabled, a scanned fixture ingests. |
| 5 | A Vietnamese + English corpus ingests and retrieves; the embedding decision is recorded. |
| 6 | New evaluation cases exist and print; variance is reported; English baseline is unchanged. |
| 7 | Markdown/text behaviour and all existing tests are unchanged; no default drifts. |

## 6. Constraint: define page provenance before the PDF loader

The **page-provenance contract must be specified and landed before**
`internal/loader.PDFLoader` is written. Otherwise a PDF loader invents a page
encoding the rest of the pipeline (chunking, ingestion, citations, evaluation)
then has to unwind. The contract to fix first:

- `document.Section` gains an explicit page field (unset for Markdown/text).
- `migrations/004_*.sql` adds a nullable `documents.page` column (additive).
- `internal/chunking` and `internal/ingestion` carry the page through.
- `internal/rag.FormatContext` and `internal/citations` render/parse the page form.

## 7. Reuse vs new

| Capability | Reuse | New |
|-----------|-------|-----|
| PDF ingestion | `loader.Registry`/`Dispatch`, `cmd/ingest` dispatch, chunking, ingestion, provenance | PDF text extractor (new dependency) |
| Page provenance | provenance columns, idempotent re-index | `Section.Page`, `documents.page` (migration 004) |
| Page citations | `internal/citations` (RAG-013), `cmd/eval` citation metrics | page form in `FormatContext` + parser |
| OCR | loader seam | OCR engine (opt-in) |
| Vietnamese/English | `CORPUS_LANGUAGE`, `NormalizeFTSConfig`, `documents.language` (RAG-009) | eval cases; a possible multilingual embedding model (recorded decision) |
| Evaluation | `cmd/eval`, `evals/`, `make sweep` | new cases + variance control |
| Genealogy | — | **separate future phase** (not here) |

## 8. Risks

- **PDF extractor dependency** — a new third-party dependency with its own
  vulnerability surface; `govulncheck` (RAG-015) must stay clean after adding it.
- **Page provenance is a cross-cutting schema/format change** — must be additive
  and default-preserving; a wrong contract is expensive to unwind.
- **OCR** — an external binary/binding, slow, and quality-limited; must stay
  opt-in and never fail a batch.
- **Embedding quality for Vietnamese** — the default model is English-centric;
  retrieval quality is unproven until measured.
- **Evaluation nondeterminism** — rewrite/LLM judges vary run to run; single-run
  deltas must not be over-read (`docs/reference/experiments.md`).

## 9. Out of scope

- Genealogy entity/person/relationship extraction (separate phase).
- Changing embedding dimensions or the default embedding model without a recorded
  decision.
- Any non-additive/destructive migration.
- Enabling Phase 23b tasks: RAG-007/008/016 stay frozen until the operator opens
  the phase.
