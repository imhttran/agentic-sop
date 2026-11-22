# Decision 5 Follow-up — Case-Checker and Sealed-Manifest Readiness

**Type:** Readiness assessment and proposal (non-normative). It prepares the open Decision 5
controls for operator review. It designates nobody, selects no location, creates no manifest,
key or secret, constructs no case, and authorizes nothing.
**Baseline:** `origin/main` @ `868ef0b2a54ac7978706f8e94fba1b46d781c460`, 2026-11-21 (analysis
prepared at `7f8cb062`; reconciled after Decision 7).
**Decision-ready wording:** [DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md](DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md)
(SHA-256 `abba919e…2305c`, re-computed on this baseline and unchanged).
**Records read:** [DECISION-RECORD-005-SEEDER.md](DECISION-RECORD-005-SEEDER.md),
[CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md),
[FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md), [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md),
[READINESS.md](READINESS.md), [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md).

Labels used below: **[PROTOCOL]** is required by the registered protocol. **[RECORDED]** is
stated by an approved decision record or the construction specification. **[PROPOSED]** is an
additional safeguard this report suggests; it is **not** a gate until the operator adopts it.

## 1. Current Decision 5 state

| Item | State | Source |
| --- | --- | --- |
| Seeder | **Tom Tran** (APPROVED 2026-11-21); not reopened here | Decision 005 §1 |
| Corpus owner | **Tom Tran** (same person; permitted by protocol §5.4, recorded as a limitation) | Decisions 001, 005 |
| Independent case checker | **PENDING — NOT DESIGNATED** | Decision 005 §3 |
| Sealed-manifest location | **PENDING — NOT SELECTED** | Decision 005 §4 |
| `case_construction_authorized` | **false** | Decision 005 §5 |
| Seeded / control cases constructed | **0 / 0** (targets about 8 / about 6) | Specification §1 |
| Sealed answer key | **does not exist** | Decision 005 table |
| Adjudicators, analyst | none designated | Decision 005 table |
| Corpus / experiment / H1–H2 | NOT FROZEN / NOT YET READY / NOT ESTABLISHED | Decision 005 table |

Decisions 6–9 remain open. This report changes none of the above.

## 2. Requirements and source references

### 2.1 What the protocol itself requires [PROTOCOL]

- The seeded set is "seeded by someone other than the adjudicator, recorded in a sealed
  manifest before any run" (§5.3).
- The seeder creates the seeded defects **and the sealed manifest**; the analyst holds the
  unblinding key; the corpus owner records determinability (§5.4 role duties).
- Corpus owner, seeder, analyst and operator may overlap; **none may be an adjudicator**; the
  seeder of a defect may not adjudicate it; adjudicators stay blind (§5.4).
- Answer material and the manifest live **outside** the per-run project, which contains only
  its own request file (§5.1, pilot checklist; §5.8).
- A no-known-defect plan is "no known defect", not "no defect"; findings on it are
  adjudicated (§5.3).

**The protocol names no "independent case checker".** It is not in the §5.4 role list.

### 2.2 What the recorded documents add [RECORDED]

- [CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md) §2 calls the case
  checker a **"proposed control, not one of the protocol's named roles"**: it re-reads each
  sealed case against its key and confirms determinability and absence of leakage; it may not
  be the seeder of that case or an adjudicator of that case. Checklist step 11 requires the
  checker's sign-off before a case enters the freeze record.
- Decision 005 §3 then makes the checker a requirement of the operator's own decision: "Until
  a checker is designated and their check is recorded, no case counts and construction is not
  authorized." It also states designation alone does not satisfy the remaining prerequisites
  (checker, sealed location, explicit construction authorization) (§5).
- Decision 005 §4: the manifest resides outside Git, reviewer projects, exported reviewer
  trees and model-accessible workspaces; access is limited to seeder, checker, analyst and
  corpus owner. Specification §6.2: adjudicators receive only a blinded defect list
  (defect id, location, class, one-sentence description; no stratum, seeder, rationale,
  expected severity, alternatives or run mapping).
- FREEZE-PROCEDURE §3.4: the freeze record holds the sealed answer-key hashes, the manifest
  **location**, the access list and the access-control method; the manifest hash is recorded
  in the freeze record and the manifest itself is not.

So the checker's necessity comes from Decision 005, not from the protocol. Removing or
changing it is an operator decision (see §9); this report does not do either.

## 3. Case-checker qualification matrix

No person is named. A row is a question the operator would record an answer to for any
proposed checker.

| Area | Requirement | Basis |
| --- | --- | --- |
| Identity | A human, not a model; named and recorded in the freeze record | [RECORDED] Decision 005 §3; protocol disallows model adjudication (§5.4) |
| Not the seeder | Must not be the seeder of any case they check; since there is one seeder, must not be Tom Tran | [RECORDED] Decision 005 §3, Spec §2 |
| Not an adjudicator of that case | Stated limit in the specification | [RECORDED] Spec §2 |
| **Not an adjudicator of any case** | The checker reads the whole sealed key, which adjudicators must never see (Spec §6.2). A person who both checks and adjudicates would unblind themselves for every case, not only the ones they checked | **[PROPOSED]**; stronger than the specification's per-case wording. Needs an operator decision |
| Not an author of a corpus plan | Authors may not adjudicate (§5.4); an author knows the defects. The checker judges determinability against the artifact, so an author's prior knowledge weakens the check | **[PROPOSED]** (protocol applies this only to adjudicators) |
| Independence from the corpus owner and seeder | Overlap of owner/seeder is permitted by protocol; for the checker the whole point is a second pair of eyes. Recommend no supervisory or authorship relationship that lets the seeder steer the sign-off | **[PROPOSED]** |
| Domain competence | Can read an SOP-style plan/specification and judge: one determinable defect, no marker text, no answer leak, a plausible distractor, a control check that is not "nobody has looked" | [RECORDED] Spec §4, §5, §10 step 11 (competence is implied, not defined) |
| Conflict of interest | Declares any authorship, direct editing, or financial or reporting tie to the seeder/owner that could bias the sign-off; declares any contact with the reviewer outputs | **[PROPOSED]** (Decision 6 package already has a conflict-of-interest declaration form for adjudicators; reuse its shape, do not edit it) |
| Permitted access | Sealed answer key and artifact for the cases they check; **not** reviewer outputs, arm labels, run ids or the unblinding key | [RECORDED] Decision 005 §4 lists the checker among the key holders; the output restriction is **[PROPOSED]** (it protects blinding if the checker later helps in any other role) |
| Required evidence | A signed per-case record: determinability confirmed with a one-line basis; no-leak result for the artifact, permitted references and exported tree; for a control, the recorded independent check; exported-tree and artifact hashes re-computed independently and matching | [RECORDED] Spec §10 steps 8–11, §7 |
| Separation from analyst | Analyst holds the unblinding key and merges within a run. Combining checker and analyst is not prohibited by any record, but it concentrates key and unblinding knowledge | **[PROPOSED]** keep separate where possible; operator decision |
| Approval and designation | Written operator decision in the Decision 1–5 form, with the qualification record attached, naming the checker, the date and the cases covered. It must not be inferred from a conversation or a commit | [RECORDED] Decisions 1–5 form; mechanism per Decision 005 §10 |
| Timing | Designated **before** construction (Decision 005 §5), signs each case **before** it counts and before the freeze | [RECORDED] |

**Qualification evidence to attach** (proposed): a one-paragraph statement of relevant
experience; a completed conflict declaration; a statement that they have not authored or
directly edited any corpus candidate plan; confirmation they will not adjudicate and have not
seen reviewer outputs; acknowledgement of the access rules in §4.

## 4. Sealed-manifest storage options

### 4.1 Constraints derived from the records

Outside Git, reviewer projects, exported reviewer trees and model-accessible workspaces
(Decision 005 §4). Hash recorded in the freeze record, manifest not (Spec §6.2). Four role
classes with different access: seeder, checker, analyst, corpus owner (access); adjudicators
and reviewers (none). Reproducible on macOS and Linux. No new service or dependency without
demonstrated need.

### 4.2 Options

| Option | How it works | For | Against | Verdict |
| --- | --- | --- | --- | --- |
| **A. Operator-controlled local directory outside the repo, versioned immutable files** | One YAML/JSON file per sealed version in a directory the operator chooses and records. Files are named by content hash, written once, set read-only, never edited. A short plain-text ledger beside them lists version, SHA-256, date and who. The directory path is recorded in the freeze record (a path, never the contents) | No new tool or service; portable (a file and `shasum -a 256` / `sha256sum` or Go's `crypto/sha256`); immutable identity by construction; trivial to back up; nothing to leak through CI or Git | Access separation rests on filesystem permissions. On one machine and one user account the seeder, owner and analyst are the same OS user, so it separates the roles by procedure, not by enforcement. Confidentiality at rest depends on disk protection | **Recommended baseline** |
| **B. Option A plus encryption for handoff and backup** | Encrypt the file with a tool already on the host (for example `openssl enc`, `gpg` or `age` where present) when copying it to the checker or to a backup, keeping the plaintext only in the operator's directory | Protects the file in transit and at rest on shared or backed-up storage; the checker can be given a copy without exposing the seeder's directory | Adds a key-handling step that is easy to get wrong; tools differ between macOS and Linux; a lost key is a lost manifest (see §4.4) | **Recommended addition for handoff and backup copies only** |
| C. Private separate Git repository | A separate private repo holds versions | History, diff and audit trail for free | Needs hosting and credentials; a wrong remote, a mirrored fork or CI access exposes the key; contradicts "outside Git" unless it is a different repository the reviewer tooling never sees; branch protections do not give per-role read separation on a small repo | Not recommended unless several people need access |
| D. Encrypted disk image or volume | Create a macOS encrypted image or a Linux LUKS volume | Strong at-rest confidentiality for the whole directory | Platform-specific, not portable; mount state is a footgun; heavy for one small file | Not recommended |
| E. Hosted secret manager or cloud storage | Put the manifest in a managed store | Real access control and audit logs | A new service, a new credential, a new trust boundary and a cloud dependency; no demonstrated need for one file | **Rejected** (conflicts with the "no new service" guidance) |

### 4.3 Recommended design (A with B for copies)

- **Location:** one directory the operator selects and records in the freeze record **as a
  path only**. It must not be inside any checkout, worktree, exported reviewer tree, or a
  path that an agent or model harness can read. Do not choose it in this report; Decision 5
  keeps it "NOT SELECTED" until the operator records it.
- **Files:** `manifest-v<N>.<first 12 hex of sha256>.yaml`, written once with mode `0400`
  (directory `0700`), never edited. A change creates a new version, a new hash, and per the
  freeze procedure (§5) a new freeze record and operator approval once the corpus is frozen.
- **Ledger:** `LEDGER.txt` beside it, append-only by convention: version, SHA-256, date, role
  of the person who created or received it. It records access, not contents.
- **Access list:** the freeze record carries the names and roles (§3.4). Adjudicators get
  only the blinded defect list export (Spec §6.2), generated from the manifest, never the
  manifest. Reviewers (the models under test) get neither the list nor the manifest.
- **Role separation in practice:** the seeder and corpus owner are the same person, and the
  analyst may be too. The checker receives an encrypted copy (Option B) scoped to the cases
  they check. Because one person holds several key-bearing roles, the separation that matters
  most — **keys versus adjudicators and reviewers** — is procedural. The report states that as
  a limitation rather than claiming enforcement.
- **Backup and recovery:** at least one additional encrypted copy on a different device,
  verified by hash after copying. The manifest is the only place the answers live: if it is
  lost, the seeded and control strata are lost and must be rebuilt as new cases and a new
  freeze (an answer key cannot be regenerated). Record who holds the recovery key.
- **Audit:** the ledger, the hashes in the freeze record, and the encrypted-copy hashes. No
  claim of tamper-proofing; hash mismatch detects change, not who made it.

### 4.4 Accidental-disclosure paths and controls

| Path | Control |
| --- | --- |
| Git (commit, stash, branch, PR) | The manifest is never created inside a checkout. Only its SHA-256 appears in repository files. The existing repository secret scanner (gitleaks gate) is not a control for this: an answer key is prose, not a credential |
| CI logs and artifacts | CI never receives the manifest or the forbidden-term list built from it (see §6, gap G4) |
| Local logs and shell history | Do not print the manifest or key text; the checker and seeder work from files, and tool output is limited to hashes and case ids |
| Model-accessible workspaces | The directory is outside any project handed to a harness; the run project contains only its own request file (protocol §5.1) |
| Backups and cloud sync | Home-directory sync services must not cover the directory; an encrypted copy is the only form that leaves it |
| Reviewer-output leakage scans | The scan needs answer vocabulary, which is itself answer material (gap G4) |

## 5. Reproducible manifest-integrity verification (design only)

The following steps use only `shasum -a 256` (macOS) or `sha256sum` (Linux), or Go's standard
`crypto/sha256`. No new tool is required, and nothing below creates a manifest.

1. **Identity.** `SHA-256(file bytes)` of each sealed manifest version equals the hash in its
   file name, the ledger and the freeze record. Mismatch means a changed file.
2. **Per-case key hash.** Each case's `sealed_sha256` (Spec §6.1) equals the SHA-256 of that
   case record's canonical bytes. The canonical form must be fixed before the first case is
   sealed (for example: UTF-8, LF newlines, keys in a fixed order, no trailing whitespace),
   because two serializations of the same data hash differently. **This canonicalization rule
   is undefined in the records today** (gap G2).
3. **Artifact hashes.** `artifact_sha256` equals the hash of the exact bytes in the exported
   reviewer tree; every `permitted_references[].blob_sha256` matches; `exported_tree_sha256`
   is the SHA-256 over the sorted `path<TAB>blob_sha256` lines (Spec §7). The checker
   re-computes these independently of the seeder.
4. **Third-party rebuild.** A third party with the freeze record can rebuild the exact
   reviewer-visible tree at the pin and confirm the same tree hash (Spec §7).
5. **Leakage check.** The exported tree and the permitted references contain no key text and
   none of the expected-finding text (Spec §6.1 `leak_checks`); see §6 for the tooling gap.
6. **Missing or corrupted manifest.** A missing file, a hash mismatch, or an unparsable file
   **blocks** the run; it is never repaired in place and never regenerated from memory.
7. **Reproducibility.** Steps 1–4 are deterministic and need no network or model.

## 6. Offline validation: what exists and what does not

All checks below are offline, synthetic and deterministic. No inference, no real manifest.

**Run on this baseline:** `go test ./internal/expharness -count=1 -run 'Scans|Render|Preflight|Hazard'`
→ **PASS**. These tests cover the experiment-local leakage scans and request rendering from
Workstream B (PR #588), using synthetic fixtures only.

**What exists that is relevant:** `internal/expharness/request.go` (`ScanRequest`, `ScanTree`,
`CheckRefs`) can flag a forbidden term in a rendered request or a project tree and treats
unscanned files, oversized files and symlinks as leakage. Existing tests cover those cases.

**Gaps** (none changed here; recorded for operator decision):

| # | Gap | Detail |
| --- | --- | --- |
| G1 | **No schema validation** for the sealed case record | Spec §6.1 gives a YAML shape, but no code or test validates required fields, the stratum enum, severity values or `none` for controls |
| G2 | **No canonicalization rule** for hashing a case record | `sealed_sha256` is defined as "the hash of this record" with no byte-level canonical form; two correct tools can produce different hashes |
| G3 | **No integrity verifier** for the manifest | Nothing recomputes the per-case hash, the exported-tree hash (`path<TAB>blob_sha256`), or checks that a permitted reference exists at the pin |
| G4 | **The leakage scan echoes the answer vocabulary** | `ScanRequest` returns the matched forbidden term and `ScanTree` prints `… mentions <term>`. If the term list is built from the sealed key, a hit writes answer text to the terminal, logs or a CI job. The term list itself is answer material and has no defined home |
| G5 | **No access-separation check** | Nothing verifies that a reviewer project, run directory or exported tree contains no file whose hash equals a manifest hash, and nothing records who read the manifest |
| G6 | **No missing/corrupt-manifest behavior** | Behavior is described in prose (§5 step 6), with no test |
| G7 | **Reproducibility of export** | The method for excluding later history (exported tree vs shallow clone) is not yet recorded or tested (Spec §8); it must be tested before the first run |

A small standalone verifier covering G1–G3, G6 and a hit-redacting variant of the scan (G4)
could be written offline against synthetic fixtures without touching any real data. **This
report does not write it.** No approved plan authorizes production code for this, and Decision
5 authorizes governance documentation only. Whether to build it, or to do these checks by hand
with `shasum` and a checklist, is an operator decision (§9).

**Follow-up (later):** the experiment-local verifier is now implemented in
`internal/egmmanifest` and `cmd/egmmanifest`, with synthetic tests; see
[DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md](DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md).
It is not wired into SOP and is not operator-approved. G5 (access separation) remains partial
on a single OS account, and the canonicalization rule is still an operator decision.

## 7. Readiness and authorization checklist

### 7.1 Already approved

| Item | Where |
| --- | --- |
| Corpus owner: Tom Tran | Decision 001 |
| Historical inclusion/exclusion dispositions | Decision 002 |
| Eligibility rule `conservative` (partial and hindsight-only count toward neither floor; seeded set is additional) | Decision 003 |
| Protocol amendment rejected as unnecessary; protocol unchanged | Decision 004 |
| Seeder: Tom Tran; role overlap recorded as a limitation | Decision 005 |

### 7.2 Still pending

| Item | State | Needed before |
| --- | --- | --- |
| Independent case checker designation | PENDING | **Case construction** (Decision 005 §3, §5) |
| Sealed-manifest location | PENDING | **Case construction** (Decision 005 §4, §5) |
| Explicit case-construction authorization (`case_construction_authorized: true`) | PENDING | **Case construction** (Decision 005 §5) |
| Analyst designation | not designated | Before the unblinding key is created, i.e. before freeze/execution (protocol §5.4); not a construction gate in the records |
| Two independent human adjudicators | PENDING (Decision 6) | **Freeze** (precondition 6); not needed to construct cases |
| Thresholds and cost ceiling | **APPROVED 2026-11-21** (Decision 7) | — |
| Corpus and instruction freeze | PENDING (Decision 8) | Execution |
| Full experiment execution | PENDING (Decision 9) | Execution |

### 7.3 Which prerequisite applies when

| Stage | Mandatory prerequisites per the existing records |
| --- | --- |
| Before **constructing** any seeded/control case | Checker designated; sealed-manifest location selected; explicit operator construction authorization. (Seeder already designated.) Counts stay 0 / 0 until then |
| Before a case **counts** | Constructed, independently checked (checker's signed record), and entered in the freeze record (Spec §1, §10) |
| Before **freeze** | Freeze preconditions 1–7 (FREEZE-PROCEDURE §2) including adjudicators and thresholds, the sealed-manifest hash and access list in the freeze record (§3.4), and operator decision 8 |
| Before **execution** | A frozen corpus, approved thresholds and cost ceiling, a separate operator execution authorization (Decision 9), and the Workstream B pre-run checks |

**No new mandatory gate is added by this report.** Everything marked [PROPOSED] in §3 and §4 is
a recommendation the operator may adopt, modify or decline; none is a gate until adopted in a
written decision.

## 8. Risks and limitations

- **One person in several key-bearing roles.** Owner, seeder and likely analyst overlap. The
  protocol allows it. It means the answer key is held by one person and the integrity of the
  seeded and control strata rests on the checker's independence and on procedure, not on
  technical separation. This is recorded, not mitigated.
- **The checker is a Decision 005 control, not a protocol role.** If the operator later
  removes it, the seeded and control strata lose their only independent determinability check;
  the protocol alone would not require one.
- **Option A depends on discipline.** Filesystem permissions and hash checks detect change and
  protect against accidental disclosure; they do not stop a determined insider.
- **Lost manifest = lost strata.** An answer key cannot be regenerated; backups matter.
- **Leakage scanning has no safe vocabulary source yet** (G4).
- **Verification here is documentary.** One focused offline test run passed; no manifest code
  for this purpose exists, so nothing about the manifest controls is tested by code.
- **Scope truncation.** The assignment text ended at the deliverable's item 4 ("Sealed-manifest
  storage options"); items 5 onward are covered here from the preceding sections (offline
  validation, checklist, authorization boundaries). The operator should confirm that nothing
  further was intended.

## 9. Operator decisions needed (none is made here)

1. **Adopt, modify or decline** the proposed checker safeguards in §3: not an adjudicator for
   any case; not an author of a corpus plan; kept separate from the analyst; no access to
   reviewer outputs.
2. **Designate** an independent case checker (a person and a recorded qualification), when one
   is available.
3. **Select** the sealed-manifest location (Option A as the baseline; Option B for handoff and
   backup copies), recording a path only.
4. **Decide** whether to build a small offline verifier for G1–G3, G4 and G6, or to rely on a
   manual `shasum` checklist; and fix the canonicalization rule (G2) before any case is sealed.
5. **Decide** where the leakage-scan vocabulary lives and whether scan output must redact the
   matched term (G4).
6. **Authorize construction** (`case_construction_authorized`) only after 2 and 3 are recorded.
7. **Confirm** whether any further deliverable beyond §6 item 4 was intended (see §8).

## 10. Non-authorization statement

This report authorizes nothing. It does **not**: reopen the seeder designation; designate a
case checker, analyst or adjudicator; select a sealed-manifest location or create a path;
create a manifest, answer key, secret or placeholder; construct a seeded or control case;
authorize case construction; change any eligibility rule, threshold or cost limit; modify the
registered protocol, `candidates.json` or Decisions 1–6; promote a candidate; adjudicate;
freeze the corpus; run model inference; or execute the experiment. The corpus remains `NOT
FROZEN`, the experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`. Seeded and control
counts remain **0 / 0** and `case_construction_authorized` remains **false**.
