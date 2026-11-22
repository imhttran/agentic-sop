# Decision 5 — Operator Approval Proposal: Case Checker, Sealed Manifest, Construction (Independent Design Review / EGM)

**Type:** Operator approval proposal (non-normative). It turns the existing Decision 5 readiness
assessment into decision-ready wording. It designates nobody, selects no location, creates no
manifest or key, constructs no case, and authorizes nothing.
**Status:** **PROPOSED — PENDING OPERATOR APPROVAL.**
**Baseline:** `origin/main` @ `868ef0b2` (merge of PR #619).
**Analysis (not duplicated here):** [DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md](DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md).
**Decision 5 record:** [DECISION-RECORD-005-SEEDER.md](DECISION-RECORD-005-SEEDER.md).
**Case construction:** [CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md).
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md).
**Decision 7 (approved):** [DECISION-RECORD-007-THRESHOLDS.md](DECISION-RECORD-007-THRESHOLDS.md).

Decision 5 already designates **Tom Tran** as seeder (and corpus owner). That decision is
preserved. The remaining controls are D5-A (case checker), D5-B (sealed manifest), and D5-C
(construction authorization). No person is named by this proposal, no location is selected, no
manifest exists, and `case_construction_authorized` remains **false**. The corpus is **NOT
FROZEN** and the experiment is **NOT YET READY**.

## 1. Decision state and prerequisite timing

| Control | State | Needed before |
| --- | --- | --- |
| Seeder: Tom Tran | **APPROVED** (Decision 005) | — |
| D5-A Case checker | **PENDING** — this proposal | **Construction** |
| D5-B Sealed-manifest location | **PENDING** — this proposal | **Construction** |
| D5-C Construction authorization | **PENDING** — this proposal; stays `false` | **Construction** |
| Canonicalization and integrity method | **PENDING** (G2/G3) | Before the first case is sealed |
| Analyst | Not designated | Before the unblinding key exists (freeze/execution) |
| Two adjudicators (Decision 6) | **PENDING** | **Freeze** |
| Thresholds and call ceiling (Decision 7) | **APPROVED 2026-11-21** | — |
| Freeze (Decision 8) / execution (Decision 9) | **PENDING** | Execution |

**Mandatory before construction** (per Decision 005 §5 and the case specification): checker
designated, sealed-manifest location selected, canonicalization/integrity method fixed,
explicit operator construction authorization. **Mandatory only before freeze/execution**:
adjudicators, analyst, freeze record, execution authorization. No new mandatory gate is added
by this proposal.

## 2. D5-A — Independent case checker

### 2.1 Registered requirement vs proposed safeguard

The case checker is a **Decision 5 governance control**, not a protocol-named role; the protocol
names the corpus owner, seeder, analyst, and adjudicators.

| Requirement | Basis |
| --- | --- |
| A human, not a model | [RECORDED] Decision 005 §3 and case spec §2; the protocol's model prohibition is for adjudicators (§5.4) |
| Not the seeder of any case they check (so not Tom Tran) | [RECORDED] Decision 005 §3; case spec §2 |
| Not an adjudicator of that case | [RECORDED] case spec §2 |
| Not an adjudicator of **any** case (the checker sees the sealed key) | [PROPOSED] stronger than the per-case wording |
| Not an author/editor of a corpus plan | [PROPOSED] |
| Kept separate from the analyst where possible | [PROPOSED] |
| No access to reviewer outputs, arm labels, run ids, or the unblinding key | [PROPOSED] output restriction; the key access itself is [RECORDED] Decision 005 §4 |
| Per-case signed record: determinability basis, no-leak result, control check, recomputed hashes | [RECORDED] case spec §7, §10 steps 8–11 |
| Designated by a written operator decision with the qualification and conflict-of-interest record attached | [RECORDED] Decisions 1–5 form |

### 2.2 Verified repository support

- The case specification defines the checker's duties, the sealed-manifest access split, and the
  per-case checklist (spec §1, §2, §6.2, §10).
- Decision 005 records the checker as pending and lists it among the key holders.
- The Decision 6 qualification package already provides a conflict-of-interest declaration
  shape; it is an input, not edited here.

### 2.3 Recommended designation process (simplest compliant)

1. Operator names a human who is not Tom Tran, not an adjudicator, and not the analyst.
2. Record: relevant experience (one paragraph), a completed conflict declaration, a statement of
   no corpus-plan authorship/editing, a statement of no reviewer-output access, and
   acknowledgement of the access rules.
3. Record the designation in a written decision record; the checker signs each case before it
   counts.

### 2.4 Exact proposed operator wording

> Decision 5A — APPROVED: independent case checker
>
> Checker: `<name>`.
> Qualification: `<relevant design/specification review experience>`. Completed conflict-of-interest
> declaration. Confirms no authorship or direct editing of any corpus plan. Confirms no access to
> reviewer outputs, arm labels, run ids, or the unblinding key.
> Independence: not the seeder or corpus owner (Tom Tran); not an adjudicator of any case; kept
> separate from the analyst where possible.
> Duties: independently check each seeded and control case before it counts; sign a per-case
> record stating the determinability basis, the no-leak result for the artifact, permitted
> references and exported tree, the control-check result, and the recomputed hashes.
> This designation does not authorize case construction, adjudication, freeze, or execution.

### 2.5 Alternatives and remaining choice

- **Stricter (recommended):** not an adjudicator of any case. Prevents the checker from
  unblinding cases they later adjudicate.
- **Spec-only:** not an adjudicator of the cases they check. Simpler, but permits a later
  conflict if the checker also adjudicates other cases.
- **External checker:** strongest separation; requires repository access and a key handover.

**Remaining operator choice:** the person, and whether to adopt the stricter [PROPOSED] safeguards
or only the [RECORDED] ones.

## 3. D5-B — Sealed manifest

### 3.1 Registered requirement vs proposed safeguard

| Requirement | Basis |
| --- | --- |
| Outside Git, reviewer projects, exported reviewer trees, and model-accessible workspaces | [RECORDED] Decision 005 §4 |
| Manifest hash in the freeze record; manifest itself not | [RECORDED] case spec §6.2; freeze §3.4 |
| Access split: seeder/owner, checker, analyst; not adjudicators or reviewers | [RECORDED] case spec §6.2 |
| SHA-256 integrity, immutable identity, versioning, backup/recovery, macOS/Linux | [RECORDED] case spec §6.1; Decision 005 §4 |
| Encrypted copies for handoff/backup; append-only ledger; recovery-key holder | [PROPOSED] |

**Detailed D5-B package (added after PR #628).** The canonicalization rule, the
location-selection policy, the seven-step recovery procedure, the residual-risk matrix, and
the exact D5-B1–B4 wording are in
[DECISION-005-B-OPERATOR-APPROVAL-PACKAGE.md](DECISION-005-B-OPERATOR-APPROVAL-PACKAGE.md).
Nothing in it is approved, and it selects no location.

### 3.2 Verified repository support

- The readiness report compares five storage options and recommends **Option A + B**: an
  operator-controlled local directory outside every checkout, with immutable content-hashed
  files, an append-only ledger, and encrypted copies for handoff and backup.
- No new service or dependency is required; `shasum -a 256` / `sha256sum` / Go `crypto/sha256`
  cover integrity on macOS and Linux.
- Gaps G1–G3 (schema validation, canonicalization, integrity verifier) remain; they must be
  resolved before the first case is sealed.

### 3.3 Exact proposed operator wording

> Decision 5B — APPROVED: sealed-manifest storage
>
> Design: Option A — an operator-controlled local directory outside every checkout, worktree,
> exported reviewer tree, and model-accessible workspace — with immutable, content-hashed files
> (`0400`, directory `0700`), an append-only ledger, and Option B encrypted copies for handoff and
> backup.
> Location: `<absolute path>`; recorded in the freeze record **as a path only**.
> Integrity: SHA-256 per file; the canonicalization rule is `<rule>`; the manifest hash is
> recorded in the freeze record.
> Access: seeder/corpus owner, checker (a scoped encrypted copy for the cases they check), and
> analyst; adjudicators receive only the blinded defect list; reviewers receive neither.
> Backup/recovery: one encrypted copy on a different device, verified by hash; recovery-key holder
> `<name>`.
> This decision selects a location and access model only. It creates no manifest, answer key, or
> secret, and does not authorize construction.

### 3.4 Alternatives and remaining choice

| Option | Trade-off |
| --- | --- |
| **A + B (recommended)** | Simple, portable, no service; access separation is procedural on one machine |
| Separate private Git repo | Free history/audit, but risks exposure and contradicts "outside Git" unless carefully isolated |
| Encrypted disk image | Strong at rest, platform-specific, mount-state risk |
| Hosted secret manager | Real access control, but a new service, credential, and trust boundary — rejected |

**Remaining operator choices:** the exact path, the canonicalization rule, the recovery-key
holder, and whether to build a small offline verifier for G1–G3 or use a manual `shasum`
checklist.

## 4. D5-C — Construction authorization

### 4.1 Prerequisites

**Mandatory before construction:**

- Seeder designated — **met** (Tom Tran).
- Independent case checker designated — **pending** (D5-A).
- Sealed-manifest location selected — **pending** (D5-B).
- Canonicalization/integrity method fixed — **pending** (G2; verifier or manual).
- Case specification and schema readiness — spec exists; schema validation gap G1.
- Leakage/independence controls and evidence plan — defined in the spec; the leakage-scan
  vocabulary home (G4) is pending.
- Explicit operator construction authorization — **pending** (this item).

**Required only before freeze/execution:** two adjudicators (Decision 6), the analyst, the
freeze record with the manifest hash/access list, the approved thresholds (met, Decision 7), and
Decision 9.

**Required counts:** about **8** seeded cases and about **6** no-known-defect controls; a case
counts only when constructed, independently checked, and recorded in the freeze record.

### 4.2 Exact proposed operator wording

> Decision 5C — APPROVED: case-construction authorization
>
> `case_construction_authorized: true`
>
> Prerequisites confirmed: seeder (Tom Tran, Decision 005); independent case checker (Decision
> 5A); sealed-manifest location and access model (Decision 5B); canonicalization and integrity
> method; case specification and schema readiness; leakage and independence controls; evidence
> preservation.
>
> This authorizes construction of about eight seeded and about six no-known-defect cases under
> the case specification. It does not authorize adjudication, corpus freeze, inference, or
> experiment execution. Decision 6, Decision 8, and Decision 9 remain separate.

### 4.3 Consequences and remaining choice

- Before approval, `case_construction_authorized` remains **false** and counts stay **0 / 0**.
- Approving D5-C starts case construction; it does not create a run.
- **Remaining choice:** whether to authorize construction now or after the checker and manifest
  location are recorded. The records require D5-A and D5-B first.

## 5. Offline verification

Run at `868ef0b2` in an isolated worktree, synthetic fixtures only:

| Check | Result |
| --- | --- |
| `go test ./internal/expharness -run 'Scans|Render|Preflight|Hazard' -count=1` | **PASS** |

**What exists:** `ScanRequest`, `ScanTree`, `CheckRefs` flag forbidden terms, unscanned files,
oversized files, and symlinks; the preflight and render tests pass.

**Follow-up (experiment-local tooling):** `internal/egmmanifest` and `cmd/egmmanifest` now
implement the schema (G1), canonicalization (G2), integrity verifier (G3), a term-safe leakage
interface (G4), Unix access checks (G5), corrupt-manifest handling (G6), and export/tree hashing
(G7) against synthetic fixtures. See
[DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md](DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md).
The tooling is not wired into SOP and is **not operator-approved**; the canonicalization rule
and the manifest location remain operator decisions.

## 6. Remaining operator choices

1. Name the case checker and adopt (or decline) the stricter independence safeguards.
2. Select the sealed-manifest path and recovery-key holder.
3. Fix the canonicalization rule; choose a small verifier or a manual `shasum` checklist.
4. Decide where the leakage-scan vocabulary lives and whether scan output must redact the term.
5. Record D5-A and D5-B, then decide D5-C.
6. Designate the analyst (before freeze/execution, not construction).

## 7. PENDING status and non-authorizations

- **No decision is approved by this proposal.**
- No case checker is designated; no sealed-manifest location is selected; no manifest, answer
  key, or secret is created.
- `case_construction_authorized` remains **false**; no seeded or control case is constructed.
- No candidate is promoted; no adjudication; no freeze; no inference; no execution.
- Decision 5's seeder designation (Tom Tran) is unchanged.
- Decision 7's thresholds, 613-call ceiling, and seven-call configuration are preserved.
- Decision 6, Decision 8, and Decision 9 remain pending.
- The corpus is **NOT FROZEN**; the experiment is **NOT YET READY**; H1/H2 are **NOT
  ESTABLISHED**.

## 8. Validation performed

- The readiness assessment, Decision 005 record, case specification, freeze procedure, and
  Decision 7 record were read at `868ef0b2`; the registered protocol is byte-identical
  (`abba919e…`).
- The focused offline tests above passed with synthetic fixtures; no real manifest or key exists.
- No person, location, case, or authorization was created or changed.
