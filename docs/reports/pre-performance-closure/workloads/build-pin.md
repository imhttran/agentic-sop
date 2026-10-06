# CLOSE-009 — Pinned SOP Measurement Build

This file records the single reproducible SOP build used for every CLOSE-009
repetition. It is aligned under CLOSE-005.

> **Status: UNAVAILABLE.** The reproducible build revision aligned under CLOSE-005
> and a pinned `sop` binary hash could not be observed on the pinned revision during
> this work. They are recorded `UNAVAILABLE` rather than estimated or fabricated.

## Recorded fields

| Field | Value | Reason |
| --- | --- | --- |
| Build reference | this file | — |
| Revision | `UNAVAILABLE` | CLOSE-005-aligned revision not resolvable from repository inspection on this run. |
| Binary hash | `UNAVAILABLE` | No pinned binary hash observed for the measurement build on this run. |
| Source repo | `go.mod` / `Makefile` / `cmd/` / `internal/` (Go SOP CLI) | present in repository |

## Rules

- All CLOSE-009 work uses this single pinned build; no measurement uses an
  unpinned or master native harness build.
- The baseline is comparable only for identical inputs, configuration and source.
- If the revision/hash are later resolved, this file and every referencing run
  record must be refreshed (see the remediation log in
  `CLOSE-009-performance-baseline.md`, section 11).
