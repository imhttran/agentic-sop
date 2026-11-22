# PLAN — EGM Sealed-Manifest Verification Tooling

**Status:** **COMPLETE (technical) — governance pending.** MP-1–MP-10 are implemented and
verified. No production wiring, no governance decision, and no experiment activity. See
§Closure.
**Baseline:** `origin/main` @ `965ddc29`; tasks completed through `f21cc0fc`.
**Authoritative Decision 5 status:** [DECISION-005-FINAL-CLOSEOUT.md](../reports/independent-design-review-corpus/DECISION-005-FINAL-CLOSEOUT.md).
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

### MP-10 — Sealer

Seal an unsigned manifest: reuse `parseManifest`, `CaseDigest`, `Canonical`, and `Validate`;
populate only `sealed_sha256`; emit canonical bytes plus one trailing newline; publish
exclusively (temporary file, `fsync`, `0400`, `os.Link`, no overwrite, no partial output);
report the exact-byte SHA-256 and the verification status of the written file. Refuse a
record whose stored digest no longer matches. Fail closed on every invalid-input and unsafe
path case.

**State:** Complete. `internal/egmmanifest/seal.go`, `cmd/egmmanifest/main.go`,
`internal/egmmanifest/seal_test.go`, `cmd/egmmanifest/main_test.go`.

### MP-9 — Technical report

Record the G1–G7 matrix, implementation decisions, schema and canonicalization, test
results, and residual risks.

**State:** Complete. `docs/reports/independent-design-review-corpus/DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md`.

## Acceptance criteria

- `go test ./internal/egmmanifest` passes with positive, negative, corruption,
  determinism, leakage, export, and sealer tests.
- `go vet` and `go build ./...` are clean.
- No real manifest, answer key, or case exists.
- No governance decision is recorded.
- The tooling is importable but not imported by SOP.

## Closure

Closed at `f21cc0fc` on 2026-11-22. Every acceptance criterion was checked against the
repository, not against a prior report:

| Criterion | Evidence |
| --- | --- |
| `go test ./internal/egmmanifest` covers positive, negative, corruption, determinism, leakage, export, and **sealer** cases | **PASS** — 54 test functions / 85 cases; `go test ./cmd/egmmanifest` 6 tests |
| `go vet` and `go build ./...` clean | **clean** |
| No real manifest, answer key, or case exists | Confirmed; every fixture is synthetic |
| No governance decision is recorded | Confirmed; D5-A, D5-B1–B4, and D5-C remain pending |
| The tooling is importable but not imported by SOP | Confirmed — only `cmd/egmmanifest` and the package's own tests import `internal/egmmanifest` |

**Lifecycle note.** This plan was never imported into SOP's plan store, so no
`planflow.Complete` step exists to run and none was fabricated. Per
[README.md](README.md), a completed plan whose path is still referenced stays in `docs/plans/`
with its status declared here: the technical verification report and the Decision 5 closeout
reference this path, so it is **not** moved to `docs/history/plans/`.

**Governance is outside this plan.** The tooling made sealing *possible*; case construction,
storage selection, checker designation, and residual-risk acceptance remain operator decisions.

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
| MP-10 Sealer | Complete |

These are task states in this plan, not SOP lifecycle transitions.
