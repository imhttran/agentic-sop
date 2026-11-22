# Decision 5-B — Sealed-Manifest Technical Verification (Independent Design Review / EGM)

**Type:** Technical verification report (non-normative). It records the offline tooling that
closes the justified G1–G7 gaps. It records no governance decision and authorizes nothing.
**Status:** **IMPLEMENTATION PROPOSAL — NOT OPERATOR-APPROVED.**
**Baseline:** `origin/main` @ `965ddc29`.
**Plan:** [PLAN-EGM-Manifest-Verification.md](../../plans/PLAN-EGM-Manifest-Verification.md).
**Readiness assessment:** [DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md](DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md).
**Approval proposal:** [DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md](DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md).
**Operator approval package (D5-B1–B4):** [DECISION-005-B-OPERATOR-APPROVAL-PACKAGE.md](DECISION-005-B-OPERATOR-APPROVAL-PACKAGE.md).
**Case specification:** [CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md) §6.1.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).

No real sealed manifest, answer key, checker, location, case, adjudicator, or model call is
involved. `case_construction_authorized` remains **false**. The corpus is **NOT FROZEN** and
the experiment is **NOT YET READY**.

## 1. G1–G7 requirement matrix

| Gap | Requirement source | Implementation | State |
| --- | --- | --- | --- |
| G1 — schema validation | Case spec §6.1; proposed D5-B | `internal/egmmanifest/manifest.go` | **Closed** |
| G2 — canonicalization | Case spec §6.1 `sealed_sha256`; proposed D5-B | `internal/egmmanifest/canonical.go` | **Closed (proposal)** |
| G3 — integrity verifier | Case spec §6.1, §7; freeze §3.4 | `internal/egmmanifest/verify.go` | **Closed** |
| G4 — leakage vocabulary | Case spec §6.2; readiness G4 | `internal/egmmanifest/leakage.go` | **Closed (interface)** |
| G5 — access separation | Decision 005 §4; freeze §3.4 | `internal/egmmanifest/access.go` | **Partially closed** (Unix checks; procedural on one account) |
| G6 — corrupt-manifest handling | Readiness G6 | Negative tests | **Closed** |
| G7 — export reproducibility | Case spec §6.1, §8 | `internal/egmmanifest/export.go` | **Closed (metadata/tree hash)** |

The canonicalization, schema, and export shapes are **implementation proposals**, not
operator-approved governance. The operator may adopt, modify, or decline them.

## 2. Existing abstractions reused

- **SHA-256 + lowercase hex** is the repository convention (`internal/planacceptance`,
  `internal/perf`, `internal/expharness`); this package uses the same encoding.
- **Path-under-root resolution** follows the `internal/planacceptance` evidence pattern
  (resolve, then verify the prefix).
- **Experiment-local isolation** follows `internal/expharness`: a package and a thin
  `cmd/` front end, imported by nothing in SOP.
- **Forbidden-term scanning** complements `internal/expharness`'s `ScanRequest`/`ScanTree`
  but does not reuse them: those return the matched term, which this package deliberately
  does not.

No new dependency, service, database, or network client is added.

## 3. Schema and canonicalization

### 3.1 Schema (version 1)

`schema_version: 1`, `corpus`, and a non-empty `cases` array. Each case mirrors the case
specification: `id`, `stratum` (`seeded` | `no_known_defect`), `artifact_path`,
`artifact_sha256`, `exported_tree_sha256`, `revision_pin`, `permitted_references[]`
(`path`, `blob_sha256`), `instruction_block_sha256`, `seeded_defect`,
`determinability_rationale`, `expected_finding`, `distractors[]`, `control_check`,
`leak_checks`, `authored_by`, `sealed_sha256`.

Validation rules: unique ids matching `[A-Za-z0-9][A-Za-z0-9._-]*`; a seeded case has a
defect class other than `none` and an expected finding with an allowed severity
(`INFO`/`LOW`/`MEDIUM`/`HIGH`/`CRITICAL`); a control has `seeded_defect: none` and a
control check; all hashes are 64 lowercase hex; paths are clean relative paths; every
leak-check boolean is false; unknown fields and trailing data are rejected.

`leak_checks` and all three of its booleans must be stated explicitly. Load requires the
object and each key to be present, so a stripped or partial record fails closed instead of
defaulting to `false` — the canonical form of the zero value is identical to an explicit
all-false record, so a silent default would let a tampered manifest verify.

### 3.2 Canonicalization

Canonical bytes are UTF-8, compact JSON with sorted object keys, no HTML escaping, and
numbers preserved as their JSON literals. `sealed_sha256` is excluded from its own digest.
`Digest(v)` is SHA-256 over those bytes. Two equivalent inputs produce identical bytes; a
meaningful change produces a different digest. `null` and `[]` are distinct; the schema
uses empty arrays where the distinction is irrelevant.

This is a **proposal**. The operator must record the rule before the first case is sealed;
a later change would invalidate every earlier hash.

## 4. Integrity verification behavior

`Load` reads an explicit path, rejects unknown fields and trailing data, and validates the
schema. Trailing data is detected by requiring a second decode to return `io.EOF`; an
`Decoder.More()` probe is not an end-of-input test (it returns `false` at a closing
delimiter even when bytes follow). `Verify` then:

1. recomputes every case digest and compares `sealed_sha256`;
2. optionally recomputes each artifact and permitted-reference hash under `ArtifactRoot`;
3. optionally recomputes a single-case exported-tree hash under `TreeRoot`;
4. optionally scans artifact and reference files with a supplied vocabulary;
5. returns a report; any problem fails closed.

The CLI prints `cases`, `verified`, `problems`, and one line per problem
(`case=… kind=… detail=…`). It never prints manifest contents or vocabulary terms.

**Missing/corrupt behavior:** missing, unreadable, invalid-JSON, unsupported-schema,
duplicate-id, digest-mismatch, artifact/reference mismatch, tree mismatch, unsafe path, and
leak conditions each produce a distinct problem or a load error.

## 5. Leakage-control design

`Vocabulary` is operator-supplied; the package hardcodes no terms and returns no term text.
`Hit` carries only a byte offset and a term index. Matching is case-sensitive and
deterministic. Tests assert that a formatted hit and a formatted problem never contain the
term.

**Where the vocabulary lives:** outside the repository, the reviewer tree, and any
model-accessible workspace; supplied to the verifier at runtime. This is an operator
decision; the package does not choose a location or create a vocabulary file.

## 6. Access-control assumptions and limitations

`CheckOutside` resolves the target and each forbidden root and rejects the path if it is
equal to or inside a root. For a path that does not exist yet, it resolves the nearest
existing ancestor and re-appends the missing components, so a symlink above a
not-yet-created directory or file cannot hide the real location. If the location cannot be
established (no existing ancestor, unreadable component, broken link) the check returns an
error rather than falling back to the unresolved path.

`CheckFileMode`/`CheckDirMode` reject symlinks and require no group/other permission bits;
an optional exact mode is checked. The checks are Unix-portable (macOS and Linux). On
Windows they return a documented unsupported error.

**Limitation:** these checks detect location, mode, and symlink problems. They do not
enforce separation between roles held by the same OS account, and they do not stop a
determined insider. Decision 005 already records that the seeder, owner, and likely analyst
overlap; the separation that matters most (keys versus adjudicators/reviewers) remains
procedural.

## 7. Synthetic test results

Run at `965ddc29` in an isolated worktree:

| Command | Result |
| --- | --- |
| `go test ./internal/egmmanifest -count=1` | **PASS** (39 test functions, 61 cases with subtests) |
| `go vet ./internal/egmmanifest ./cmd/egmmanifest` | **clean** |
| `go build ./...` | **clean** |

Coverage: schema acceptance/rejection (14 negative cases), load errors (unknown field,
trailing object/array/delimiter/scalar data, invalid JSON, missing file, missing/partial/
non-boolean `leak_checks`), canonical key-order stability and content sensitivity, sealing,
leakage positive/negative and no-term-echo, access outside/symlink/symlinked-missing-
descendant/mode checks, digest/artifact/reference/tree mismatch, fail-closed schema failure,
export private exclusion, export determinism, private-vs-public digest behavior, and
tree-hash determinism and skip behavior.

Three defects found in PR review (#623) were fixed with regression tests: unresolved
symlinked descendants below a not-yet-created path, trailing-delimiter acceptance, and
implicit defaulting of `leak_checks`.

No real manifest, answer key, artifact, or case was used; every fixture is synthetic.

## 8. Remaining operator decisions

1. Adopt, modify, or decline the canonicalization rule (G2).
2. Choose a small offline verifier (this package) or a manual `shasum` checklist.
3. Choose where the leakage vocabulary lives and whether scan output must redact the term.
4. Record the manifest location (D5-B) and the case checker (D5-A).
5. Decide whether to wire the verifier into any operator workflow; it is not wired now.
6. Decide whether a Windows access check is needed; the experiment targets macOS/Linux.

## 9. Residual risks

- The canonicalization rule is a proposal; adopting a different rule invalidates existing
  hashes.
- The verifier checks metadata and file hashes; it does not prove that a case is a good
  design artifact or that its defect is determinable. That remains the checker's judgment.
- Access separation is procedural on one account; filesystem checks are not isolation.
- The leakage scan is a substring screen, not a DLP classifier. A term absent from the
  vocabulary is not detected.
- Tree hashing covers regular files; it rejects symlinks and non-regular files but does not
  define what the exported tree *should* contain. That is the freeze record's job.
- No real corpus has been verified, so the tooling's behavior on a real manifest is
  unproven; only synthetic fixtures were exercised.
- No Windows execution was performed; the Windows path returns unsupported by design.

## 10. Governance boundaries

This report and the tooling do **not**: approve D5-A, D5-B, or D5-C; designate a checker;
select a manifest location; set `case_construction_authorized: true`; construct a case;
create or access a real answer key; modify `candidates.json`, eligibility, the protocol, or
Decision 7's thresholds or 613-call ceiling; designate adjudicators; freeze the corpus; run
inference; or execute the experiment. Decision 5's seeder designation (Tom Tran) is
unchanged. All technical choices remain implementation proposals until the operator
approves them.
