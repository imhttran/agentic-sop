# PLAN — EGM Sealed-Manifest Verification Tooling

**Status:** DRAFT — implementation plan for experiment-local tooling. No production
wiring, no governance decision, no experiment activity.
**Baseline:** `origin/main` @ `965ddc29`.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Decision 5 package:** [DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md](../reports/independent-design-review-corpus/DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md).
**Readiness assessment:** [DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md](../reports/independent-design-review-corpus/DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md).

## Objective

Close the justified technical gaps G1–G7 in the sealed-manifest readiness assessment with a
small, independently testable, experiment-local Go package and a thin operator command. The
tooling validates a synthetic or operator-supplied manifest offline; it never contacts a
model, network, or provider.

## Non-goals

- No governance approval (D5-A, D5-B, D5-C stay pending).
- No case-checker designation, manifest location selection, or construction authorization.
- No seeded/control case construction, real answer key, or sealed manifest.
- No change to the registered protocol, thresholds, the 613-call ceiling, or eligibility.
- No production wiring: nothing in SOP imports the package.
- No experiment execution or model inference.

## Tasks

### MP-1 — Schema and validation

Define the versioned manifest and case schema, mirroring the case-construction
specification §6.1. Validate required fields, schema version, unique ids, stratum, hash
format, seeded/control rules, severity values, unsafe paths, and leak-check booleans.
Reject unknown fields and trailing data on load.

**State:** Complete. `internal/egmmanifest/manifest.go`.

### MP-2 — Canonicalization

Define canonical JSON: UTF-8, compact, sorted object keys, no HTML escaping, preserved
numeric literals, explicit schema version, SHA-256 over the canonical bytes. Exclude
`sealed_sha256` from its own digest.

**State:** Complete. `internal/egmmanifest/canonical.go`.

### MP-3 — Integrity verifier

Load and validate a manifest from an explicit path; recompute each case digest; optionally
recompute artifact and permitted-reference hashes and the exported-tree hash; report
problems; fail closed.

**State:** Complete. `internal/egmmanifest/verify.go`.

### MP-4 — Leakage vocabulary

Operator-supplied terms; deterministic substring scan; hits carry offset and term index
only; no term is returned or printed. Positive and negative tests.

**State:** Complete. `internal/egmmanifest/leakage.go`.

### MP-5 — Access separation

Checks that a path is outside forbidden roots, resolving symlinks; Unix mode checks for
files and directories (no group/other access); symlinks rejected. Windows returns a
documented unsupported error.

**State:** Complete. `internal/egmmanifest/access.go`.

### MP-6 — Corrupt-manifest handling

Negative tests for missing file, invalid JSON, unsupported schema, digest mismatch,
truncated file, duplicate ids, unsafe paths, and unreadable files. Verification fails
closed.

**State:** Complete. `internal/egmmanifest/*_test.go`.

### MP-7 — Export reproducibility

Reviewer-visible export excludes private and answer-bearing fields; repeated exports are
byte-identical; private changes do not alter the export digest; public changes do. A
deterministic tree hash covers the exported files.

**State:** Complete. `internal/egmmanifest/export.go`.

### MP-8 — Operator command

`cmd/egmmanifest verify -manifest FILE [-root DIR] [-tree DIR] [-vocab FILE]`. Prints counts
and problem categories only; nonzero exit on failure.

**State:** Complete. `cmd/egmmanifest/main.go`.

### MP-9 — Technical report

Record the G1–G7 matrix, implementation decisions, schema and canonicalization, test
results, and residual risks.

**State:** Complete. `docs/reports/independent-design-review-corpus/DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md`.

## Acceptance criteria

- `go test ./internal/egmmanifest` passes with positive, negative, corruption,
  determinism, leakage, and export tests.
- `go vet` and `go build ./...` are clean.
- No real manifest, answer key, or case exists.
- No governance decision is recorded.
- The tooling is importable but not imported by SOP.

## Dependencies and status

| Task | State |
| --- | --- |
| MP-1 Schema | Complete |
| MP-2 Canonicalization | Complete |
| MP-3 Verifier | Complete |
| MP-4 Leakage | Complete |
| MP-5 Access | Complete |
| MP-6 Corrupt handling | Complete |
| MP-7 Export | Complete |
| MP-8 CLI | Complete |
| MP-9 Report | Complete |

These are task states in this plan, not SOP lifecycle transitions.
