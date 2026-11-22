# Decision 5 — Final Closeout: Technical Complete, Governance Pending

**Type:** Authoritative closeout record (non-normative). It consolidates one verified status and
one operator decision package for Decision 5. It records **no approval**, designates nobody,
selects no location, and creates no artifact.
**Status:** **TECHNICAL IMPLEMENTATION COMPLETE — GOVERNANCE PENDING OPERATOR APPROVAL.**
Decision 5 is **not** closed: D5-A, D5-B1–B4, and D5-C remain open.
**Baseline:** `origin/main` @ `ba69d947` (merged #621, #623, #624, #631, #633).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) —
byte-identical, SHA-256 `abba919ecb0938475c1731b419c4bddd8b1d1fe92ed97d51109c85f14e32305c`.
**Canonical status of each part lives here.** Earlier reports remain as historical evidence and
are pointed to, not rewritten.

## 1. The three-way status of Decision 5

| Part | State |
| --- | --- |
| **Technical implementation (G1–G7, F1)** | **Complete and verified** in this repository (§2) |
| **Governance recorded** | Seeder = **Tom Tran** (also corpus owner) — Decision 5, 2026-11-21 (§3) |
| **Governance pending** | **D5-A** checker · **D5-B1** canonicalization · **D5-B2** storage/location/access list · **D5-B3** recovery/key holder · **D5-B4** residual-risk dispositions · **D5-C** construction authorization (§4) |

`case_construction_authorized` is **false**. No case, manifest, answer key, vocabulary file, or
secret exists. The corpus is **NOT FROZEN**, the experiment **NOT YET READY**, H1/H2 **NOT
ESTABLISHED**, and the independently adjudicated candidate count is **0**.

## 2. Final technical verification

Verified by reading the merged code and running the focused suite at `ba69d947`; no new
implementation was added.

| Gap | Requirement | Implementation | Verified behaviour |
| --- | --- | --- | --- |
| **G1** | Schema validation | `internal/egmmanifest/manifest.go` | `schema_version` must be `1`; unknown fields and trailing data rejected; `leak_checks` object **and all three booleans** required explicitly at load; unique ids matching `[A-Za-z0-9][A-Za-z0-9._-]*`; stratum rules; severity enum; 64-hex hashes; clean relative paths |
| **G2** | Deterministic canonicalization | `canonical.go` | UTF-8 compact JSON, sorted object keys, preserved array order, HTML escaping off, JSON-literal numbers; `CaseDigest` clears `sealed_sha256` |
| **G3** | Integrity verification | `verify.go`, `cmd/egmmanifest verify` | Recomputes every case digest; optional artifact / permitted-reference / exported-tree hashes; optional term scan; any problem fails closed; prints counts and kinds only |
| **G4** | Leakage interface | `leakage.go` | Operator-supplied terms; no hardcoded vocabulary; `Hit` carries offset + term index only; no term text in any output |
| **G5** | Access checks | `access.go` | `CheckOutside` resolves the nearest existing ancestor, so a symlink above a not-yet-created path cannot hide the location; `CheckFileMode`/`CheckDirMode` reject symlinks and any group/other bits; **Unix only — Windows returns `ErrUnsupported`**; **not exposed by the CLI**; detection, not isolation |
| **G6** | Corrupt-manifest handling | negative tests | Missing, unreadable, invalid JSON, unsupported schema, truncated, digest mismatch, duplicate ids, unsafe path → fail closed |
| **G7** | Deterministic exports | `export.go` | `ReviewerExport` excludes `seeded_defect`, `determinability_rationale`, `expected_finding`, `distractors`, `control_check`, `leak_checks`, `authored_by`, `sealed_sha256`; canonical bytes; `TreeHash` over sorted `path<TAB>blob-sha256` lines and rejects non-regular files |
| **F1** | Offline sealing | `seal.go`, `cmd/egmmanifest seal` | Populates **only** `sealed_sha256`; refuses to reseal a record whose stored digest no longer matches; output is canonical bytes + exactly one newline; reports the exact-byte SHA-256; exclusive publication |

### 2.1 Shared implementation (no second canonicalizer)

`seal` and `verify` both call `CaseDigest` and `Validate` from `canonical.go` and
`manifest.go`; the sealer additionally calls the same `Canonical` and the same
`parseManifest` that `Load` uses. There is one canonicalization implementation and one
parser. **JSON only — no YAML support was added.**

### 2.2 Behaviour confirmed in this cycle

```
seal    sealed=…/s.json sha256=6e2291e9…8be78 verify=ok            exit=0
verify  cases=1 verified=1 problems=0                              exit=0
shape   bytes=735  last_byte=\n  mode=-r--------
reseal  egmmanifest: output already exists                         exit=1
reformat  seal(compact) == seal(pretty + sorted keys)              identical=true
```

Checks that follow from that evidence: schema version 1 enforced · per-case digest over the
canonical record with `sealed_sha256` emptied · canonical JSON · exactly one trailing newline
· digest over the exact output bytes · exclusive creation (`os.Link` fails on an existing
target) · no silent resealing · no answer-bearing text in errors (asserted by test) ·
fail-closed on every malformed-input case · platform limits documented (`0400` and symlink
checks are Unix semantics; Windows returns `ErrUnsupported`).

### 2.3 Test evidence

| Command | Result |
| --- | --- |
| `go test ./internal/egmmanifest/... ./cmd/egmmanifest/...` | **PASS** (54 library test functions / 85 cases; 6 CLI tests) |
| `go test -race` on both packages | **PASS** |
| `go vet`, `go build ./...` | clean |
| CI on the merged sealer head `7340b727` (PR #633) | **20/20 SUCCESS**; the one CodeQL alert (`go/allocation-size-overflow`) was fixed before merge |
| `scripts/checks/pre-push.sh --base origin/main` | OK, 0 broken links |

Cross-platform: executed on **macOS only**. Linux is exercised by CI. Windows was not executed;
the access checks return a documented unsupported error by design.

## 3. Governance already recorded

| Decision | State | Record |
| --- | --- | --- |
| Seeder (= corpus owner) | **APPROVED 2026-11-21** — Tom Tran; overlap permitted by protocol §5.4 and recorded as a limitation | [DECISION-RECORD-005-SEEDER.md](DECISION-RECORD-005-SEEDER.md) |

Nothing else in Decision 5 is approved. The technical completion above is **not** an approval:
every G/F item is tooling whose *use* still depends on the decisions in §4.

## 4. Outstanding operator decisions — copy-ready wording

The full analysis and alternatives are in
[DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md](DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md) (D5-A, D5-B, D5-C)
and [DECISION-005-B-OPERATOR-APPROVAL-PACKAGE.md](DECISION-005-B-OPERATOR-APPROVAL-PACKAGE.md) (D5-B1–B4).
Each block below is **PROPOSED — NOT APPROVED**; approving one does not approve another.

### D5-A — Independent case checker

**Requirements:** a **human** (never a model, never an AI substitute); not the seeder or corpus
owner (so not Tom Tran); not an adjudicator of **any** case; did not author or directly edit any
corpus plan; kept separate from the analyst where possible; a completed conflict-of-interest
declaration; no access to reviewer outputs, arm labels, run ids, or the unblinding key; signs a
per-case record (determinability basis, no-leak result, control check, recomputed hashes).

**Identity stays unresolved** — this closeout names nobody.

> **Decision 5A — APPROVED: independent case checker.**
>
> Checker: `<name>`. Qualification: `<relevant design/specification review experience>`.
> Independence: not the seeder or corpus owner (Tom Tran); not an adjudicator of any case; not
> an author or direct editor of any corpus plan; kept separate from the analyst where possible.
> Attestations: conflict-of-interest declaration completed; no access to reviewer outputs, arm
> labels, run ids, or the unblinding key; acknowledgement of the sealed-manifest access rules.
> Duties: independently check each seeded and control case before it counts, and sign a
> per-case record stating the determinability basis, the no-leak result for the artifact,
> permitted references and exported tree, the control-check result, and the recomputed hashes.
> This designation authorizes no construction, adjudication, freeze, or execution.

> **Decision 5A — NOT APPROVED: checker deferred.** No checker is designated; case construction
> cannot begin, because a checker is a mandatory pre-construction prerequisite.

### D5-B1 — Canonicalization

**Recommendation: ADOPT** the implemented rule — verified in §2 and unchanged since #624.

> **Decision 5B1 — APPROVED: canonicalization and digest rule (schema version 1).**
>
> 1. **Encoding.** Canonical bytes are UTF-8 JSON, compact, no insignificant whitespace;
>    object keys sorted lexicographically by byte order; array order preserved; HTML escaping
>    disabled; numbers emitted as their JSON literals.
> 2. **Case digest.** `sealed_sha256` is the lowercase hex SHA-256 of the canonical bytes of
>    the case record **with `sealed_sha256` set to the empty string**. Fields tagged
>    `omitempty` are absent when empty; `leak_checks` (all three booleans) and `sealed_sha256`
>    are always present.
> 3. **Manifest hash.** The sealed manifest file is written once with bytes equal to the
>    canonical bytes of the whole manifest plus a single trailing newline. The manifest hash is
>    the lowercase hex SHA-256 of those exact file bytes.
> 4. **Tree hash.** `exported_tree_sha256` is the lowercase hex SHA-256 over sorted
>    `<relative path with "/" separators>` + TAB + `<blob sha256>` lines, one per regular file,
>    newline-terminated; a non-regular file is an error.
> 5. **Version.** `schema_version` MUST equal `1`; any other value is rejected. Changing this
>    rule requires a new decision and invalidates every earlier hash.
>
> This fixes a serialization rule only. It approves no location and creates no manifest.

> **Decision 5B1 — NOT APPROVED: canonicalization deferred.** No manifest may be sealed, because
> a digest rule must be fixed before the first case is sealed.

### D5-B2 — Storage

**Recommendation: Option A + B** (operator-controlled local directory, immutable content-hashed
JSON files, `0400`/`0700`, append-only ledger; encrypted copies for handoff and backup only).
No new service, database, or dependency.

> **Decision 5B2 — APPROVED: sealed-manifest storage.**
>
> Design: Option A — an operator-controlled local directory outside every checkout, worktree,
> exported reviewer tree, and model-accessible workspace — with immutable content-hashed JSON
> files (`0400`, directory `0700`), an append-only ledger, and Option B encrypted copies for
> handoff and backup.
> Manifest directory: `<absolute path>` (resolved; not inside any repository or worktree, not
> cloud-synced, not network-mounted, one filesystem).
> Leakage-vocabulary location: `<absolute path or "operator-held outside any workspace">`.
> Authorized access list: `<role names>` — seeder/corpus owner, checker (scoped encrypted copy),
> analyst; adjudicators receive only the blinded defect list; reviewers receive neither.
> Recorded in the freeze record **as a path only**.
> This selects a location and access model; it creates no manifest, answer key, or secret.

> **Decision 5B2 — NOT APPROVED: storage deferred.** No location is selected and no manifest
> directory exists.

The operator supplies every concrete value above; this record selects none. Location policy
(resolve symlinks first; outside checkouts, exports, and model-accessible workspaces; not
synced or network-mounted; `0700`/`0400`; one OS account is **not** role separation) is in the
D5-B package §3.2.

### D5-B3 — Recovery

> **Decision 5B3 — APPROVED: encrypted-backup and recovery procedure.**
>
> One encrypted copy on a separate device, verified by the SHA-256 of the ciphertext recorded
> in the ledger. Recovery-key holder: `<name or role>`.
> Procedure: (1) verify backup availability; (2) verify the ciphertext hash; (3) restore to a
> new `0700` directory outside every checkout; (4) revalidate schema and canonicalization with
> `egmmanifest verify`; (5) recompute and compare the plaintext manifest hash and every
> `sealed_sha256`; (6) re-apply and confirm `0700`/`0400` and the location policy; (7) record
> date, operator, source device, ciphertext hash, plaintext hash, `verify` output, and the
> permission check.
> Key loss is unrecoverable: rebuilding the seeded and control strata as new cases would be
> required. This decision creates no key and holds no manifest.

> **Decision 5B3 — NOT APPROVED: recovery deferred.** No backup, key, or key holder exists.

### D5-B4 — Residual risks

> **Decision 5B4 — APPROVED: residual-risk disposition.**
>
> Accepted as procedural limitations, on the record: same-account role separation; seeder,
> owner, and (future) analyst overlap; programmatic manifests entering the corpus only through
> `Load`; filesystem checks being detection and not isolation; manifest integrity not
> establishing independent defect determinability; the leakage scan being a substring screen,
> not a classifier; tree hashing covering regular files without defining the intended tree.
> Decided: the leakage-vocabulary home (D5-B2); Windows out of scope — `<accepted OR authorize a
> Windows access check later>`; `<accept OR reject>` the synthetic-only verification limitation.
> No item is accepted by default. This records dispositions only.

> **Decision 5B4 — NOT APPROVED: residual risks not dispositioned.** Undispositioned risks block
> construction, because they are part of the approved control set.

### D5-C — Construction authorization

**Prerequisites (all mandatory before construction):** seeder (met) · checker designated (D5-A) ·
canonicalization fixed (D5-B1) · manifest directory and access list selected (D5-B2) ·
recovery procedure and key holder (D5-B3) · residual risks dispositioned (D5-B4) · case
specification and schema readiness (met) · leakage and independence controls (partial until
D5-B2) · explicit operator authorization (D5-C itself).

> **Decision 5C — APPROVED: case-construction authorization.**
>
> `case_construction_authorized: true`
>
> Prerequisites confirmed: seeder (Tom Tran, Decision 5); independent case checker (D5-A);
> canonicalization and digest rule (D5-B1); manifest directory, vocabulary location, and access
> list (D5-B2); recovery procedure and key holder (D5-B3); residual-risk dispositions (D5-B4);
> case specification and schema readiness; leakage and independence controls; evidence
> preservation.
> This authorizes construction of about eight seeded and about six no-known-defect cases under
> the case specification. It authorizes no adjudication, no freeze, no inference, and no
> experiment execution. Decisions 6, 8, and 9 remain separate.

> **Decision 5C — NOT APPROVED: construction unauthorized.** `case_construction_authorized`
> remains `false`; seeded and control counts remain 0 / 8 and 0 / 6.

**Technical readiness is not authorization.** The verified G1–G7 and F1 tooling makes sealing
*possible*; it does not make construction *authorized*.

## 5. Residual-risk reconciliation

Every risk recorded in the technical report §9, the D5-B package §5, and the readiness
assessment §7, in one place. No row is accepted until the operator records D5-B4.

| Risk | Classification | Basis / required action |
| --- | --- | --- |
| Canonicalization rule not yet approved | **Requires operator approval** | D5-B1; adopting a different rule invalidates every hash |
| Manifest-hash scope, format, digest-input shape (F2–F4) | **Requires operator approval** | Settled by approving D5-B1 verbatim; declining produces an incompatible rule |
| Same-account role separation | **Procedurally controlled** + operator acceptance | One OS user; permissions do not separate roles; D5-B4 records it |
| Seeder / owner / analyst overlap | **Procedurally controlled** + operator acceptance | Decision 5 records the seeder/owner overlap; analyst undesignated; D5-B4 |
| Programmatic manifest bypassing load-time checks | **Procedurally controlled** | `Load` enforces explicit `leak_checks`; `Validate` cannot detect an in-memory struct with zero-valued checks — rule: entry only via `Load` from the sealed file |
| Leakage-vocabulary exposure | **Requires operator approval** (location) + procedural handling | The vocabulary is answer material; never printed, never in a repo or reviewer tree |
| Filesystem checks vs actual isolation | **Procedurally controlled** | Detection of location/mode/symlink problems, not isolation; not exposed by the CLI; does not stop a determined insider |
| Windows `ErrUnsupported` | **Requires operator approval** | Experiment targets macOS/Linux; operator records Windows out of scope or authorizes a check later |
| Backup-key custody | **Requires operator approval** | D5-B3 names the holder and second device; key loss is unrecoverable |
| Manifest integrity vs independent defect determinability | **Procedurally controlled** | Hashing proves the record is unchanged, not that a case is good or its defect determinable — that is the checker's judgment (D5-A) |
| Leakage scan is a substring screen, not a DLP classifier | **Procedurally controlled** | A term absent from the vocabulary is not detected; the checker still inspects the artifact |
| Tree hash covers regular files; the intended tree is undefined | **Procedurally controlled** | The freeze record defines what the exported tree must contain |
| No real corpus has been verified or sealed | **Procedurally controlled** | Every fixture is synthetic; the first real manifest is verified by the checker and recorded at freeze |
| No Windows execution performed | **Requires operator approval** | Same decision as the Windows row above |
| Sealing is a derivation, not a content check | **Procedurally controlled** | The sealer copies the operator's answer fields; correctness is the checker's judgment |
| Post-publication verification failure leaves the file | **Technically addressed** | Documented, cannot occur for a manifest the sealer itself validated; the command exits non-zero |
| **F1 — no sealer** | **Technically addressed (closed)** | `egmmanifest seal`; see §2 |
| Same-account separation claimed as enforcement | **Procedurally controlled** | Explicitly **not** claimed: no document may state that filesystem permissions provide complete role isolation |

**No residual risk requires remediation.** The only item in that class before #633 was F1, which
is closed. If final verification had found a defect, the sealer work would have continued under
the normal SOP remediation path instead of this closeout.

## 6. Remaining dependency chain

Ordered; each step requires the previous one where noted.

1. **D5-A** — operator designates an independent human checker.
2. **D5-B1–B4** — operator approves the canonicalization rule, the manifest directory,
   vocabulary location and access list, the recovery procedure and key holder, and the
   residual-risk dispositions.
3. **D5-C prerequisites verified** — every mandatory item in §4 D5-C present.
4. **D5-C** — operator separately authorizes case construction.
5. **Construct and independently check** the seeded and control cases under the approved
   procedure; seal into the approved directory; record hashes.
6. **Decision 6** — two independent human adjudicators designated and authorship checks
   recorded, before the freeze/execution gates that need them.
7. **Historical corpus eligibility and minimum counts** — the protocol floors: 15 determinable
   known defects across at least 15 plans with known defects. Current classes are unchanged:
   C0 **3 / 4**, C1 **4 / 5**, C2 **13 / 15**, C3 **11 / 15** (see
   [CORPUS-COUNT-CONVENTION.md](CORPUS-COUNT-CONVENTION.md)). Only C2 reaches a floor, and only
   with zero slack and every conditional ruling favorable; plans remain 2 short.
8. **Decision 8** — freeze authorization, with the freeze record and the manifest hash.
9. **Decision 9** — execution authorization; the approved 613-call ceiling authorizes no
   spending by itself.

**Protocol confirmation.** Protocol §5.3 requires the corpus to be fixed and written down
before the first run, with a 15 / 15 minimum, and §5.4 requires two distinct human adjudicators
who are not authors, with a model never serving as adjudicator. The freeze procedure's
preconditions mirror that ordering (owner, inclusion list, eligibility, amendment, seeder,
adjudicators, thresholds), so the chain above matches the registered protocol. **No discrepancy
was found**; the protocol was not amended. Steps 1–5 are Decision 5's own gates; steps 6–9 are
not — **Decision 5 approval alone does not make the experiment executable.**

## 7. Readiness matrix

| # | Decision | Status |
| --- | --- | --- |
| 1 | Corpus owner — Tom Tran | **APPROVED** (2026-11-21) |
| 2 | Historical dispositions — 3 strict / 4 defects; 11 partial / 11 defects; holds and exclusions | **APPROVED** (2026-11-21); 0 adjudicated |
| 3 | Eligibility rule — `conservative` | **APPROVED** (2026-11-21) |
| 4 | Protocol amendment | **REJECTED (unnecessary)** (2026-11-21); protocol unchanged |
| 5 | Seeder — Tom Tran | **APPROVED** (2026-11-21) |
| 5 | Case checker (D5-A) | **PENDING OPERATOR APPROVAL** — nobody designated |
| 5 | Canonicalization, storage, recovery, residual risks (D5-B1–B4) | **PENDING OPERATOR APPROVAL** — technically implemented |
| 5 | Construction authorization (D5-C) | **UNAUTHORIZED** — `false` |
| 6 | Two independent human adjudicators | **PENDING**; the three procedural clarifications are **APPROVED**, the designation is not |
| 7 | Thresholds and provider-call ceiling | **APPROVED** (2026-11-21) — thresholds, 7-call config, 613-call ceiling; no dollar ceiling |
| 8 | Corpus freeze | **PENDING** — corpus **NOT FROZEN** |
| 9 | Experiment execution | **PENDING** — **NOT YET READY**; no inference performed |

Historical corpus counting (unchanged, from the merged convention): **C0** 3 plans / 4 defects
(shortfall 12 / 11) · **C1** 4 / 5 (11 / 10) · **C2** 13 / 15 (2 / 0, provisional maximum) ·
**C3** 11 / 15 (4 / 0, cluster sensitivity, never a total). No count or eligibility decision is
changed here.

## 8. Documentation inconsistencies resolved

Corrections in this cycle were clarifications and pointers; historical findings were left
intact.

| Where | Inconsistency | Resolution |
| --- | --- | --- |
| `DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md` §1 | Canonicalization/integrity method listed as "PENDING (G2/G3)" | Clarified: implemented, **approval** pending (D5-B1) |
| Same, §3.2 | "Gaps G1–G3 remain; they must be resolved" | Clarified with the current implementation state |
| Same, §2.4 / §3.3 | Blocks beginning "Decision 5A/5B — APPROVED" could read as recorded approvals | A qualifier line now precedes each block: template only, not an approval |
| Same, header | Linked only the readiness assessment | Added pointers to the D5-B package and this closeout (the D5-B link in §3 already existed) |
| `DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md` | `.yaml` file naming and a "no schema validation" gap table (true when written) | Added a current-status pointer at the top: JSON format, G1–G7 implemented, nothing approved; the historical text is preserved |
| `DECISION-005-B-OPERATOR-APPROVAL-PACKAGE.md` | F1 row and matrix row 9 described the sealer as absent | Updated to closed technically (already done in #633) |
| `DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md` | G-table had no sealing row | Added F1 row and §4.2 (already done in #633); header now points here |
| `READINESS.md` §16 | Recorded Decision 5 state but not the technical completion | Added a current-status pointer |
| `docs/reports/README.md` | No entry for the closeout | Added one index line |

**Claims explicitly checked and not present** (so nothing was softened): no document asserts
that filesystem permissions provide complete role isolation, and none asserts that technical
verification establishes defect determinability. The technical report §6 and the D5-B package
§5 both state the opposite.

## 9. Workstream closure and preserved state

| Item | State |
| --- | --- |
| Technical plan [PLAN-EGM-Manifest-Verification.md](../../plans/PLAN-EGM-Manifest-Verification.md) | **COMPLETE (technical)** — MP-1–MP-10, acceptance criteria verified at `f21cc0fc`; kept in `docs/plans/` because two documents reference its path; no `planflow.Complete` was applicable (the plan was never imported into SOP's plan store) |
| Governance portion of Decision 5 | **OPEN** — this record cannot close it, and no approval was recorded |
| Real artifacts | **None** — no case, manifest, answer key, vocabulary file, or secret exists |
| Case construction | **UNAUTHORIZED** — `case_construction_authorized: false` |

**Worktrees and branches.** Seven Decision 5 worktrees exist locally; all are clean and all of
their branches are merged into `origin/main` except one. `docs/egm-decision-5-readiness`
(`19568938`) is **unmerged and superseded**: its copy of the readiness assessment predates the
current-status pointer and still records Decision 7 as `PENDING`, which is wrong. It must not be
merged. It is preserved, not deleted, because deleting an unmerged branch or unique evidence is
not authorized here. Proposed cleanup for the operator: remove the six merged Decision 5
worktrees (`egm-manifest-verification`, `egm-manifest-sealer`, `egm-d5b-approval`,
`egm-d5-closeout`, `egm-decision-5-approval`, `egm-decision-5-readiness-fix`) after confirming
each is clean, and keep `egm-decision-5-readiness` for audit. Nothing was removed in this cycle,
and no worktree, uncommitted file, or ignored `.agent-sdlc/` evidence was touched.

## 10. Non-authorization statement

This closeout approves nothing. It does not approve D5-A, D5-B1–B4, or D5-C; designate a
checker, analyst, or adjudicator; select a manifest directory, vocabulary location, or
key holder; create a case, manifest, answer key, vocabulary file, or secret; set
`case_construction_authorized: true`; modify `candidates.json`, Decision 3, Decision 7
(including the 613-call ceiling), the count convention, or the registered protocol; promote or
adjudicate a candidate; freeze the corpus; run inference; or execute the experiment. Decision 6
remains **PENDING**; the corpus remains **NOT FROZEN**; the experiment remains **NOT YET
READY**; H1/H2 remain **NOT ESTABLISHED**.
