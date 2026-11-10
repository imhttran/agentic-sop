# Proposal: per-row digests for the Structural Repository Index

Status: **Decided — option A, implemented** (2026-11-09). Schema 2 and
`repoindex.VerifyRows` implement it; `TestRowDigests` covers the acceptance criteria.

Related: [Plan 05 report](../reports/owasp-llm/llm05-poisoning.md) GAP-4, and
`internal/repoindex` (`Build`, `Identity`).

## Goal

Give each indexed `File` and `Document` row its own digest, so a consumer can
verify one row without trusting the writer that produced the file, and so a
consumer can tell which row changed when the aggregate identity moves.

## What exists today

- `Identity.Digest` is a 16-hex SHA-256 over `(relative path, file content
  SHA-256)` pairs for every indexed file, plus the resolved document rows
  (`path`, class, authority, lifecycle, authority source). Fixing the aggregate
  coverage is in review; it is not a per-row digest.
- No `File` or `Document` row carries a digest. A reader that loads
  `.agent-sdlc/context/index.json` cannot verify a single row.
- No reader loads the artifact today: every current consumer (`internal/cli`
  `retrieve`, `internal/retrieval`, `internal/rageval`) rebuilds in memory. The
  gap becomes externally reachable when the planned Context Engine consumer starts
  reading the file.

## Why it matters (and what it does not prove)

A per-row digest detects a partial write, a hand-edited row, or a mixed-version
artifact. It does **not** prove that the content is benign, that the repository was
reviewed, or that the writer is authentic. It is byte integrity, not provenance or
semantic trust.

## Proposed schema (v2, additive)

Add one field to each row, and bump `SchemaVersion` to `2`:

```json
{
  "path": "docs/plans/ACTIVE.md",
  "document_class": "plan",
  "authority": "current",
  "lifecycle": "active",
  "authority_source": "sop",
  "digest": "sha256:<64 lowercase hex>"
}
```

Digest input, in order, with a `0x00` separator and no normalization:

1. the row kind (`file` or `document`),
2. the repository-relative slash path,
3. the entry's file content SHA-256 (untruncated),
4. for documents only: class, authority, lifecycle, authority source.

`File` rows use the same construction without step 4. The digest is the full
SHA-256 (64 hex) rather than the 16-hex aggregate, because a row digest is the
unit of verification and the truncation buys nothing.

The aggregate `Identity.Digest` stays as it is (16 hex) and incorporates the same
inputs, so the two cannot disagree about a row change.

## Rebuild and compatibility

`sop index` always rebuilds the artifact from the working tree. There is no
migration step and no in-repo reader to upgrade.

- A v2 writer overwrites the file on the next `sop index`.
- A future v1 reader must treat a missing `digest` as **unverified**, never as
  matching. It may accept the row for advisory use, but it must not report the row
  as verified.
- A future v2 reader rejects a row whose digest is absent, malformed, or does not
  match the recomputed value.

No other persisted state is touched, and no lifecycle or approval state changes.

## Acceptance criteria for the decision

1. `SchemaVersion` is `2`; `File` and `Document` carry `digest` with the stated
   construction.
2. Deterministic fixtures prove: identical trees produce identical rows; a
   one-byte file change changes that row's digest; a metadata-only authority change
   changes the affected document row's digest and no file-row digest.
3. A tampered row is detected by recomputation and fails with a distinguishable
   error; the malformed/absent digest is distinguished from a mismatch.
4. A v1 artifact without row digests is treated as unverified, not valid.
5. `sop index` remains model-free, read-only with respect to source files, and does
   not change any routing, retrieval, security, or limit setting.
6. The plan report and the evidence matrix record the row-level state and the
   v1/v2 handling.

## Decision required

Approve or reject the additive schema bump and the v1 handling. The alternatives
are:

- **A (recommended):** the v2 field above, with v1 rows unverified.
- **B:** no per-row digest; document that only the aggregate identity is
  verifiable, and require a future consumer to rebuild in memory.
- **C:** a detached `context/index.digests.json` manifest instead of per-row
  fields. This keeps the row shape unchanged but adds a second artifact that can
  drift, so it is not recommended.

## Out of scope

- Publisher authenticity, signatures, and trust in the writer.
- Semantic correctness or benign content.
- Any new service, evaluator, gate, or permission framework.
