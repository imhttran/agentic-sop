# Proposal: verifiable retrieval evidence hashes

Status: **PROPOSAL — not approved.** This document defines the contract change
required by Plan 05 GAP-5. No verification is implemented by this proposal.

Related: [Plan 05 report](../reports/owasp-llm/llm05-poisoning.md) GAP-5,
`internal/ragclient`, and
[RAG-RETRIEVAL-CONTRACT.md](../specs/RAG-RETRIEVAL-CONTRACT.md).

## Goal

Let SOP check that the evidence it received is the evidence the producer says it
is, without inventing a second document store and without trusting a value that
cannot be recomputed.

## What the current contract supplies

| Item | Current contract | What SOP actually has |
| ---- | ---------------- | --------------------- |
| `document_hash` | `documents.content_hash`, described as the ingestion fingerprint of the source document (`RAG-RETRIEVAL-CONTRACT.md:100-121`) | A string carried verbatim; no algorithm, canonicalization, or scope is defined |
| `text` | The returned chunk (`RAG-RETRIEVAL-CONTRACT.md:103`); ingestion splits a document into overlapping sections (`RAG-STRATEGY.md:16-19`) | One excerpt, with no document bytes, chunk offsets, or chunk hash |
| Client validation | Corpus allowlist, response schema, chunk size, response size, timeouts, deadline | `decode` never reads `DocumentHash` (`internal/ragclient/ragclient.go:273-291`) |
| Current fixtures | `ragclient_test.go` round-trips the literal `sha256:abc` | No test compares a hash with any content; an arbitrary value is accepted |

Recomputing `document_hash` from `text` would be wrong: the hash covers the full
source document, and `text` is one overlapping chunk. Format checks are also
unsound because the current contract does not define the format.

The deterministic fixtures
`TestDocumentHashIsServerReportedAndNotRecomputed` and
`TestDocumentHashMayBeEmpty` pin this behavior.

## Proposed change (additive)

1. **Define `document_hash` normatively.** Scope: the full ingested source
   document, not a chunk. Algorithm: `"sha256:" + lowercase hex(SHA-256(raw
   ingested file bytes))`, with no line-ending, encoding, or Unicode
   normalization. The same value is reported for every chunk of one source. An
   empty value means **unknown**, never "verified".
2. **Add `chunk_hash` (optional, per evidence item).** Scope: the exact UTF-8
   bytes of the emitted `text`, with no trim, newline, or normalization.
   Algorithm: `"sha256:" + lowercase hex(SHA-256(text))`. A producer that emits
   it makes the returned evidence independently checkable.
3. **Wire compatibility.** `decode` uses `DisallowUnknownFields`
   (`internal/ragclient/ragclient.go:273`), so an unupgraded SOP client rejects a
   response carrying `chunk_hash`. Ship the client field first, then enable the
   producer field; otherwise bump the schema to `rag-retrieval/v2`. Never emit
   `chunk_hash` to an unupgraded client.
4. **Verification rule.** When `chunk_hash` is present, recompute it from `text`
   and reject the whole response on a mismatch with a typed code (for example
   `evidence_hash_mismatch`). An absent or empty `chunk_hash` is accepted as
   unverified and recorded as such; it is never reported as verified.
   `document_hash` is carried and compared across duplicate items only; SOP does
   not recompute it.

## Acceptance criteria

1. The contract states the algorithm, canonicalization, scope, and empty-value
   policy for both fields.
2. `Evidence.ChunkHash` is decoded and a mismatch fails the whole response with a
   distinguishable error; a malformed value is distinguished from a mismatch.
3. Deterministic fixtures prove: a correct `chunk_hash` is accepted; a one-byte
   mutation is rejected; a trailing newline or NFC/NFD difference is rejected; an
   absent `chunk_hash` is accepted as unverified; an arbitrary `document_hash`
   with no `chunk_hash` is still not verified.
4. `producer_acceptance_test.go` recomputes `chunk_hash` when the producer emits
   it.
5. The report records the reproducing command and keeps publisher authenticity,
   benign content, and DLP explicitly out of scope.
6. No new service, evaluator, gate, or permission framework is added.

## Decision required

- **A (recommended):** add the optional `chunk_hash` field as above, with an
  absent value treated as unverified.
- **B:** change `document_hash` semantics only (define the algorithm) without a
  chunk hash. This still cannot verify a chunk, so it improves provenance
  vocabulary but not evidence integrity.
- **C:** bump the retrieval schema to `rag-retrieval/v2` and require
  `chunk_hash`. This rejects every legacy producer and is a rollout decision, not
  a client change.

## Out of scope

- Publisher authenticity, signatures, and trust in the producer.
- Semantic correctness or benign content.
- General DLP or prompt-secrecy controls.
