# Decision 5-B — Operator Approval Package: Sealed Manifest, Integrity, Recovery

**Type:** Operator approval package (non-normative). It prepares exact decision text for the
operator. It approves nothing, selects no location, creates no manifest or key, and
authorizes no case construction.
**Status:** **PROPOSED — PENDING OPERATOR APPROVAL.**
**Baseline:** `origin/main` @ `59018d95` (merged #621, #623, #624, #628).
**Technical verification:** [DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md](DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md).
**Readiness assessment:** [DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md](DECISION-005-CASE-CHECKER-MANIFEST-READINESS.md).
**Decision 5 proposal (D5-A/5B/5C):** [DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md](DECISION-005-OPERATOR-APPROVAL-PROPOSAL.md).
**Decision 5 record:** [DECISION-RECORD-005-SEEDER.md](DECISION-RECORD-005-SEEDER.md).
**Case specification:** [CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md) §6.
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) §3.4.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.2–5.3.

Decision 5's seeder designation (Tom Tran, also corpus owner) is preserved and not revisited
here. No person is named, no path is selected, no manifest or answer key exists,
`case_construction_authorized` is **false**, the corpus is **NOT FROZEN**, and the experiment
is **NOT YET READY**. Independently adjudicated candidates remain **0**.

## 1. Current implementation — what is actually verified

Verified in this session at `59018d95`, in an isolated worktree, with synthetic fixtures only.

| Capability | Code | State |
| --- | --- | --- |
| Schema validation (G1) | `internal/egmmanifest/manifest.go` | **Implemented.** Fail-closed: unknown fields, trailing data, missing/partial/non-boolean `leak_checks` all rejected at `Load` |
| Canonicalization (G2) | `internal/egmmanifest/canonical.go` | **Implemented, not approved.** Rule frozen in §2 |
| Integrity verifier (G3) | `verify.go` + `cmd/egmmanifest` | **Implemented.** Recomputes every per-case digest; exit 1 on any problem |
| Leakage interface (G4) | `leakage.go` | **Implemented.** Term-safe: no term text in any output |
| Access checks (G5) | `access.go` | **Implemented as a library only — not exposed by the CLI** |
| Corrupt-manifest handling (G6) | negative tests | **Implemented.** `Load`/`Verify` fail closed |
| Export + tree hash (G7) | `export.go` | **Implemented.** Private fields excluded; symlinks rejected in a tree |
| **Manifest sealer / writer** | — | **Absent.** The CLI has exactly one subcommand, `verify` |

Focused results: `go test ./internal/egmmanifest/... -count=1` → **PASS**;
`go vet` → **clean**. Nothing in SOP imports this package; no network or model call is made.

### 1.1 Findings that change what the operator must approve

| # | Finding | Kind | Consequence |
| --- | --- | --- | --- |
| **F1** | **No sealer.** No library or CLI path writes a manifest or computes `sealed_sha256` for a new case. | Gap in authorized scope (sealing was never authorized) | A manifest cannot be produced with the shipped tooling. **Must be resolved before the first case is sealed**, by a small `seal`/`digest` subcommand or an approved scripted procedure |
| **F2** | **"Manifest hash" was undefined.** Per-case canonical digests exist; there is no CLI manifest digest, and a whole-file hash depends on formatting. | Ambiguity | Fixed by the §2 rule: the sealed file's exact bytes are canonical, so the file hash is a content function |
| **F3** | **Format mismatch.** `Load` parses **JSON only**, but the readiness report names `manifest-v<N>.<hash>.yaml` and the case spec's example is YAML. | Documentation inconsistency | D5-B1/B2 fix the sealed format as **JSON**; the YAML example remains illustrative of the field set |
| **F4** | **Digest-input shape is load-bearing.** The digest input contains `"sealed_sha256":""` (present, empty) and always contains `leak_checks`; `omitempty` fields are absent when empty. | Compatibility risk | Must be part of the approved rule, or an independent reimplementation would compute different digests |
| **F5** | Determinism and portability confirmed empirically (§2.3). | Positive | Supports adoption |

## 2. D5-B1 — Canonicalization recommendation: **ADOPT**

The algorithm needs no change. F2–F4 are *scope* questions that the approval wording settles;
none is a correctness or security defect.

### 2.1 The exact rule the operator would approve

> **Canonicalization and digest rule (schema version 1).**
>
> 1. **Encoding.** Canonical bytes are UTF-8 JSON, compact, with no insignificant
>    whitespace. Object keys are sorted lexicographically by byte order; array order is
>    preserved. HTML escaping is disabled (no `\u003c`-style escapes). Numbers are emitted as
>    their JSON literals, so an integer is not rewritten as a float.
> 2. **Case digest.** `sealed_sha256` is the lowercase hex SHA-256 of the canonical bytes of
>    the case record **with `sealed_sha256` set to the empty string**. Fields tagged
>    `omitempty` are absent when empty; `leak_checks` (all three booleans) and
>    `sealed_sha256` are always present.
> 3. **Manifest hash.** The sealed manifest file is written once, with bytes equal to the
>    canonical bytes of the whole manifest plus a single trailing newline. The manifest hash
>    is the lowercase hex SHA-256 of those exact file bytes, so it is a function of content
>    and not of formatting.
> 4. **Tree hash.** `exported_tree_sha256` is the lowercase hex SHA-256 over the sorted lines
>    `<relative path with "/" separators>` + TAB + `<blob sha256>`, one line per regular file,
>    newline-terminated. A non-regular file (including a symlink) is an error, not a skip.
> 5. **Version.** `schema_version` MUST equal `1`; any other value is rejected. A change to
>    this rule or to the schema requires a new version, a new decision record, and a new
>    freeze — it invalidates every earlier hash.

### 2.2 Compatibility with the registered protocol

The protocol fixes *what* must be recorded (per-case sealed answer keys, the manifest hash in
the freeze record, the manifest never in a reviewer project) and leaves the serialization
rule to the corpus owner. §2.1 satisfies §5.2 (isolation from the answer), §5.3 (freeze
contents), and the case specification §6.1–6.2. It changes no threshold, no eligibility rule,
and no protocol byte.

### 2.3 Evidence

Two semantically identical manifests — same record, different key order and whitespace —
verified in this session:

```
case digest     A = 43a8fb04e8a11810…   B = 43a8fb04e8a11810…   equal = true
whole-file hash A = 3a9139751333fc36…   B = 03aec90c74fa2cea…   equal = false
canonical manifest digest (both)          = d4c7eacea00f9462…
digest input includes                    = …"revision_pin":"p","sealed_sha256":"","seeded_defect":…
```

Reading: the case digest is stable under reformatting; the raw file hash is **not**, which is
why §2.1(3) fixes the file's bytes rather than relying on a "format the same way" promise.

### 2.4 Rejected alternatives

| Alternative | Why not |
| --- | --- |
| Whole-file hash only, no canonical rule | Two operators sealing identical content produce different hashes; the freeze record becomes non-reproducible |
| YAML canonicalization | `Load` is JSON-only (F3); YAML libraries differ on key order, anchors, and scalar typing |
| Hash the file bytes without fixing their form | Same failure as row 1 (proved above) |
| Omit `sealed_sha256` from the digest input instead of emptying it | Not wrong, but it is a *different* rule (F4) and would not match the implementation |

## 3. D5-B2 — Storage architecture: **APPROVE Option A + B, location pending**

### 3.1 Recommended architecture

Option A (operator-controlled local directory, immutable content-hashed files, append-only
ledger) with Option B (encrypted copies) for handoff and backup only. No new service,
database, or dependency; integrity uses `shasum -a 256` / `sha256sum` / Go `crypto/sha256`.

- **Files:** `manifest-v<N>.<first 12 hex of the manifest hash>.json`, written once, file
  mode `0400`, directory mode `0700`, never edited. A change is a new version and a new hash.
- **Ledger:** `LEDGER.txt` beside the files, append-only by convention: version, SHA-256,
  date, role of the person who created or received a copy. Records access, never contents.
- **Manifest content:** schema version 1 JSON, exactly as §2.1(3) requires (F3).

### 3.2 Location-selection policy (macOS and Linux) — the operator supplies the path

The package does **not** select a path. The operator applies this policy and records the
resolved absolute path.

1. **Resolve first.** `realpath` / `readlink -f` the chosen directory and apply every check
   below to the **resolved** path, so a symlink cannot hide the real location. (`CheckOutside`
   implements exactly this, including a not-yet-created file beneath a symlinked directory.)
2. **Outside every checkout and worktree.** Not inside any Git repository, any
   `.worktrees/*` checkout, or any directory a harness is given as a project.
3. **Outside reviewer exports and model-accessible workspaces.** Not inside a run directory,
   a prompt/run artifact tree, or anything reachable from a task request.
4. **Not cloud-synced and not network-mounted.** On macOS, avoid iCloud Drive
   (`~/Library/Mobile Documents`), `~/Desktop`, and `~/Documents` if either is synced; on
   Linux, avoid `~/.config` sync, NFS, and CIFS. Confirm with `stat -f -c %T <dir>` on Linux
   (must not be `nfs`/`cifs`) and `mount` / `df <dir>` on either platform.
5. **One filesystem, no boundary crossing.** The directory must not be a mount point that
   exposes a broader tree to another role.
6. **Plaintext never leaves the directory in plaintext.** Backups, handoffs, and any
   second-device copy are encrypted (Option B). If the host's Time Machine or a sync client
   covers the directory, either exclude it or accept the plaintext-backup exposure
   explicitly.
7. **Permissions.** Directory `0700`, files `0400`. Verify with `ls -ld` / `stat` and, in the
   library, `CheckDirMode` / `CheckFileMode` — the latter are **not exposed by the CLI** (G5).
8. **One OS account is not separation.** On a single-user account the seeder, owner, and
   likely analyst are the same principal; permissions do not separate them. Record this as a
   limitation (D5-B4), not as enforcement.
9. **Answer-derived vocabulary.** The leakage vocabulary is built from the answer key and is
   therefore answer material. It lives outside the repository, the reviewer tree, and any
   model-accessible workspace, and is supplied to the verifier at runtime (G4). Its home is an
   operator input under this decision.

### 3.3 Alternatives

| Option | Verdict |
| --- | --- |
| A + B (recommended) | Simple, portable, no service; role separation is procedural on one machine |
| Separate private Git repository | Free history, but a wrong remote or CI access exposes the key; contradicts "outside Git" |
| Encrypted disk image | Strong at rest, platform-specific, mount-state risk |
| Hosted secret manager | New service, credential, and trust boundary — rejected |

## 4. D5-B3 — Recovery controls: **APPROVE the procedure, key holder pending**

### 4.1 Minimum recovery procedure

1. **Verify backup availability.** Confirm the encrypted copy exists on the second device and
   is readable. Record device, path, and date.
2. **Verify the backup hash.** Compare the SHA-256 of the encrypted copy with the hash in the
   ledger. A mismatch is a hard stop.
3. **Restore to a controlled location.** Decrypt into a new `0700` directory outside every
   checkout; never restore over an existing manifest directory.
4. **Revalidate schema and canonicalization.** Run
   `egmmanifest verify -manifest <file>`. `Load` re-checks the schema (version, ids, hashes,
   explicit `leak_checks`, no trailing data) and `Verify` recomputes every per-case canonical
   digest, which re-validates the canonicalization rule.
5. **Recompute integrity hashes.** Compare the restored file's SHA-256 with the recorded
   manifest hash; compare every `sealed_sha256` with `verify`'s recomputation.
6. **Check access restrictions.** Re-apply `0700`/`0400`, re-resolve the path, and re-confirm
   the §3.2 conditions (not inside a checkout, not synced, not network-mounted).
7. **Record recovery evidence.** Date, operator, source device, ciphertext hash, plaintext
   hash, `verify` output (`cases= verified= problems=`), and the `0700`/`0400` check.

**Key loss is unrecoverable.** The manifest is the only place the answers live; if the
encrypted copy and the plaintext are both lost, the seeded and control strata must be rebuilt
as new cases and frozen again.

### 4.2 Tooling support without new code

| Step | Supported by |
| --- | --- |
| Encrypt / decrypt (1, 3) | Host tools already present (`age`, `gpg`, or `openssl enc`); no repository change |
| Hash (2, 5) | `shasum -a 256` (macOS), `sha256sum` (Linux), or `Digest` in the library |
| Schema + canonicalization (4) | `cmd/egmmanifest verify` — **yes, no new code** |
| Access checks (6) | Manual `stat` / `ls -ld`; the library checks exist but are **not** wired into the CLI |
| Evidence (7) | Operator record |

**No new storage service, database, or dependency is required.** Two small CLI surfaces would
make the procedure one-command instead of five (a `digest` for the manifest hash, and
`check-access` for G5); neither is a prerequisite for approval, and neither is authorized here.

## 5. D5-B4 — Residual-risk matrix

| # | Risk | Classification | Basis and required action |
| --- | --- | --- | --- |
| 1 | Same-account role separation | **Controlled procedurally** | One OS user; permissions do not separate seeder/owner/analyst. Operator accepts the limitation in writing |
| 2 | Seeder/owner/analyst overlap | **Controlled procedurally** | Decision 5 records the seeder/owner overlap as a limitation; the analyst is not designated yet. Operator accepts; analyst designated before freeze/execution |
| 3 | Programmatically built manifests bypassing load-time checks | **Controlled procedurally** | `Load` enforces explicit `leak_checks`; `Verify`/`ReviewerExport` call `Validate` but cannot detect a struct built in memory with zero-valued `LeakChecks`. Rule: a manifest enters the corpus **only** through `Load` from the sealed file |
| 4 | Leakage-vocabulary exposure | **Requires operator decision** | The vocabulary is answer material (G4). Its home is part of D5-B2; handling is procedural (never printed, never in a repo or reviewer tree) |
| 5 | Filesystem checks vs actual isolation | **Controlled procedurally** | `CheckOutside`/`CheckFileMode` detect location, symlink, and mode problems; they are not isolation and do not stop a determined insider, nor are they exposed by the CLI |
| 6 | Windows `ErrUnsupported` | **Requires operator decision** | The experiment targets macOS/Linux; the access checks return a documented unsupported error on Windows. Operator records Windows as out of scope (or authorizes a Windows check later) |
| 7 | Backup-key custody | **Requires operator decision** | A lost key loses the manifest. D5-B3 names the recovery-key holder and the second device |
| 8 | Manifest integrity vs independent defect determinability | **Controlled procedurally** | Hashing proves the record is unchanged, not that a case is a good artifact or that its defect is determinable. That stays the case checker's judgment (D5-A) |
| 9 | **No sealer (F1)** | **Requires remediation before sealing** | A manifest cannot be produced with the shipped tooling. Resolve with a small `seal` subcommand or an approved scripted procedure **before the first case is sealed** |
| 10 | Manifest-hash scope, format, digest-input shape (F2–F4) | **Requires operator decision** | Settled by approving §2.1 verbatim; declining or editing it produces a different, incompatible rule |

**Explicitly not accepted silently:** rows 1, 2, 5, and 8 are limitations the operator must
accept on the record; rows 4, 6, 7, and 10 are decisions only the operator can make; row 9 is
a prerequisite block, not a risk to accept.

## 6. Exact operator decision text

Each block is **PROPOSED — PENDING OPERATOR APPROVAL**. Approving one does not approve
another.

### D5-B1 — Canonicalization

> **Decision 5B1 — APPROVED: canonicalization and digest rule.**
>
> Adopt the rule in §2.1 verbatim: UTF-8 compact JSON with sorted object keys, preserved array
> order, disabled HTML escaping, and JSON-literal numbers; the per-case `sealed_sha256` over
> the canonical case bytes with `sealed_sha256` set to the empty string; the manifest hash as
> the SHA-256 of the sealed file's exact bytes, where those bytes are the canonical manifest
> bytes plus one trailing newline; the tree hash over sorted `path<TAB>blob-sha256` lines;
> `schema_version` fixed at `1`, with any other value rejected.
>
> This fixes a serialization rule only. It approves no location, creates no manifest, and does
> not authorize construction. A change to this rule requires a new decision and invalidates
> every earlier hash.

### D5-B2 — Storage architecture

> **Decision 5B2 — APPROVED: sealed-manifest storage.**
>
> Design: Option A — an operator-controlled local directory outside every checkout, worktree,
> exported reviewer tree, and model-accessible workspace — with immutable content-hashed JSON
> files (`0400`, directory `0700`), an append-only ledger, and Option B encrypted copies for
> handoff and backup.
> Location: `<absolute path>` (resolved), satisfying the §3.2 policy: not inside any
> repository or worktree, not cloud-synced, not network-mounted, one filesystem.
> Leakage vocabulary: `<absolute path or "operator-held outside any workspace">`.
> Access: seeder/corpus owner, checker (a scoped encrypted copy for the cases they check), and
> analyst; adjudicators receive only the blinded defect list; reviewers receive neither.
> Recorded in the freeze record **as a path only**.
>
> This selects a location and access model. It creates no manifest, answer key, or secret, and
> does not authorize construction.

### D5-B3 — Recovery controls

> **Decision 5B3 — APPROVED: encrypted-backup and recovery procedure.**
>
> Adopt the seven-step procedure in §4.1. One encrypted copy on a different device, verified by
> the SHA-256 of the ciphertext recorded in the ledger. Recovery-key holder: `<name or role>`.
> Recovery evidence records date, operator, source device, ciphertext hash, plaintext hash,
> `egmmanifest verify` output, and the permission check.
>
> Key loss is unrecoverable and requires rebuilding the seeded and control strata as new
> cases. This decision creates no key and holds no manifest.

### D5-B4 — Residual risks

> **Decision 5B4 — APPROVED: residual-risk disposition.**
>
> Accepted as procedural limitations, on the record: same-account role separation; seeder,
> owner, and (future) analyst overlap; programmatic manifests entering only through `Load`;
> filesystem checks being detection and not isolation; manifest integrity not establishing
> independent defect determinability.
> Decided: the leakage-vocabulary home (D5-B2); Windows out of scope — `<accepted or
> authorize a Windows check later>`.
> Required before the first case is sealed: a manifest sealer (§1.1 F1), by a small
> `seal` subcommand or an approved scripted procedure.
>
> This records dispositions only. It authorizes no construction, adjudication, freeze, or
> execution.

## 7. Prerequisites for D5-C (updated)

| Prerequisite | Before | State after D5-B1–B4 would be approved |
| --- | --- | --- |
| Seeder designated | Construction | **MET** — Tom Tran (Decision 5) |
| Canonicalization and integrity rule fixed | Sealing | **MET** — D5-B1 |
| Sealed-manifest location and access model | Construction | **MET** — D5-B2, once the operator supplies the path |
| Recovery procedure and key holder | Sealing | **MET** — D5-B3, once the holder is named |
| Residual-risk disposition | Construction | **MET** — D5-B4 |
| Manifest sealer exists (F1) | Sealing | **NOT MET** — remediation or approved procedure |
| Independent case checker designated | Construction | **NOT MET** — D5-A |
| Case specification and schema readiness | Construction | **MET** — spec §6; schema implemented |
| Leakage and independence controls | Construction | **PARTIAL** — vocabulary home from D5-B2 |
| Operator construction authorization | Construction | **NOT MET** — D5-C, stays `false` |

`case_construction_authorized` remains **false**. Approving D5-B1–B4 does not authorize
construction; D5-A and D5-C remain separate decisions.

## 8. Dependencies on D5-A and Decision 6

- **D5-A (case checker).** Unchanged and still pending. The checker must be a human who is not
  the seeder, not Tom Tran, and not an adjudicator of any case; they receive a scoped
  encrypted copy (D5-B2) and sign each case before it counts. No checker is designated here.
- **Decision 6 (two adjudicators).** Pending; required before **freeze**, not before
  construction. Adjudicators receive only the blinded defect list export — never the manifest
  or the answer key (§6.2).
- **Analyst.** Not designated; required before the unblinding key exists (freeze/execution).
  The analyst holds the unblinding key and applies the within-run merge.

## 9. Validation performed

- Read at `59018d95`: the registered protocol §5.2–5.3, the Decision 5 record, the D5 approval
  proposal, the readiness assessment, the technical verification report, the case
  specification §6, the freeze procedure §2–§5, and the whole of `internal/egmmanifest` and
  `cmd/egmmanifest`.
- `go test ./internal/egmmanifest/... -count=1` → **PASS**; `go vet` → **clean**.
- Canonicalization determinism, format sensitivity, and digest-input shape verified with a
  temporary probe test, removed before commit; synthetic fixtures only.
- The registered protocol is byte-identical (SHA-256 `abba919e…`); `candidates.json` and the
  count convention are untouched.
- No real manifest, answer key, case, location, or person was created or selected.

## 10. Non-authorization statement

This package approves nothing. It does not approve D5-A, D5-B, or D5-C; designate a case
checker, analyst, or adjudicator; select a manifest location or recovery-key holder; create a
manifest, answer key, vocabulary file, or secret; set `case_construction_authorized: true`;
construct a case; modify `candidates.json`, Decision 3, Decision 7 (including the 613-call
ceiling), the count convention, or the registered protocol; promote or adjudicate a candidate;
freeze the corpus; run model inference; or execute the experiment. Decision 6 remains
**PENDING**; the corpus remains **NOT FROZEN**; the experiment remains **NOT YET READY**;
H1/H2 remain **NOT ESTABLISHED**.
