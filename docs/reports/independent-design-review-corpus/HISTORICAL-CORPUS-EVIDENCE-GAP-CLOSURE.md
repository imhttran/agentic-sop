# Historical Corpus — Evidence Gap Closure (held leads, a spec-amendment sweep)

**Type:** Investigation record (non-normative, read-only). It tests whether the already-identified but uncounted leads can supply the two plans the conditional stack is short. It is **not**
candidate promotion, adjudication, a corpus freeze, or an experiment result. **Every candidate named as new below is PROPOSED — NOT APPROVED.**
**Baseline:** `origin/main` @ `19d88973f6002deabd35fa45dcd4b9041383af31` (merge of PR #632), 2026-11-22.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3. **Rule:** [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md) (`conservative`).
**Count convention (merged):** [CORPUS-COUNT-CONVENTION.md](CORPUS-COUNT-CONVENTION.md) (C0–C4). **Decision package (merged):** [HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md](HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md).
**Evidence (not repeated):** [#613](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY.md), [#620](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY-2.md), [#626](HISTORICAL-CORPUS-STRICT-EVIDENCE-VERIFICATION.md),
[#627](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY-3.md), [#618](DISC-001-H-001-EVIDENCE-VERIFICATION.md), [candidates.json](candidates.json) (unchanged).

## 1. Result

**The held leads do not supply two more plans.** Nine leads were investigated or re-checked and a sweep of spec amendments was run. The outcome is **0 new strict candidates and 0 new determinable defects**.
Three held leads gained verified detail (one more defect on `GH2-001` and on `GH-004`, and a persistence check on `IDR-L-001`), but each stays behind an open ruling, so **C0–C3 are unchanged**:
C0 3 / 4, C1 4 / 5, C2 13 / 15 (2 plans short), C3 11 / 15. Nothing is added to C2. By artifact counting the held leads could still add plans **conditionally** (scenario S7 in §8: `IDR-L-001` and `GH-004`, reaching 15 plans / 19 defects without skills or guides), but only if R-D, R-B Option B, every packet kind including approval packets, R-A and every adjudication all go the permissive way, against the recommended rules. No strict plan was added; §10 says what could change the shortfall.

## 2. Preflight and ownership

- `origin/main` fetched. Open PR at the start: #631 (Decision 5-B approval package, another agent, not read). #628 (count convention) is merged; #629, #630, #632 (decision package and fixes) are merged.
- Active worktrees `egm-count-reconciliation`, `egm-d5b-approval`, `egm-manifest-*`, `egm-evidence-discovery-3` belong to other work and were not entered.
- `DISC-001` / `IDR-H-001` were verified by PR #618 and are corpus-owner rulings; they are **cited, not re-investigated**.
- Isolated worktree `docs/egm-evidence-gap-closure`. Decision 5-B tooling, Decision 6/7 files and `candidates.json` untouched.

## 3. Scope and method

Selected, in the order the brief listed them: `IDR-L-001`, `GH-004`, `GH-005`, `GH-006`, `IDR-L-008`, `GH2-001`, `GH2-002`, and the evidence-blocked manifest leads `IDR-E-006`, `IDR-E-010`, `IDR-E-012`.
`GH2-003`–`GH2-005` are left aside (they depend on the R-C ruling and were triaged only, per #627 §7).

Method: pre-fix SHA and artifact via `git show <sha>:<path>`; contemporaneous evidence via review thread text and repository files at the same SHA; deterministic checks only
(`git grep`, `git show` with line numbers); persistence checks on `origin/main`; additivity check against the C2 artifact paths and `candidates.json`. No historical code was executed.
In addition, a **spec-amendment sweep**: every commit on `docs/specs/*.md` for ten specs that adds a `MUST` / `SHALL` / `Required` line (45 commits listed), and a corrective-keyword
commit search over `docs/specs`, `architecture`, `reference`, `plans`, `guides` excluding SHAs already named in any corpus report.

## 4. Evidence matrix

| Lead | Artifact @ revision | Defects | Independent evidence | Classification (proposed) | Open ruling |
| --- | --- | --- | --- | --- | --- |
| IDR-L-001 | `docs/plans/PLAN-Code-Retrieval-Context-Seam.md` @ `21564b27bd9f88c288a47ba71b492b77e9468db3` | 1 | the artifact's own later status line (`03a7d7d97912f22b708622791568240242a7e76d`); gap **persists** on `origin/main` | **EVIDENCE_BLOCKED** | R-D: may a self-recorded post-merge limitation count as a "recorded gap"? |
| GH-004 | `docs/plans/PLAN-ACP-EXECUTION-LEASE-WIRING-OPERATOR-APPROVAL-PACKET.md` @ `c6307fd34d0957bd07349aa331819eb34692766f`, `022e7568555e1fc3990869d9c1ff95f4cfe15880` | 3 verified, 1 partial | bot threads, deterministic reads | **STRICT_CANDIDATE only if approval packets are admitted** (contrary to the recommended R-C-packet rule) | R-B, R-C-packet, R-A; subject overlap with `DISC-001` |
| GH-005 | `docs/reference/CLI.md` @ `d8aa908dd3a231f749497d9a32610f2cee52d748` | 1 | bot thread, `planner.go:162` | **EXCLUDED** under the recommended R-C rule | R-C, R-B |
| GH-006 | `docs/guides/PROMPTING.md` @ `cef3b8a910939f4c17664f9c9dec9fced6ee6b29` | 1 | bot thread; `acceptance_cargo_test.go` (#620 §5.2) | **EXCLUDED** under the recommended R-C rule | R-C, R-B |
| GH2-001 | `skills/decision-ab/SKILL.md` @ `62388bab9a52d95c65259e8e6afa713bc6ab3fcb` | 2 verified | bot threads, static reads | **EXCLUDED** under the recommended R-C rule | R-C, R-B |
| GH2-002 | `skills/sop-fix-conflicts/SKILL.md` @ `52c835dd854ed7db4f3f8da46ebb2b81ede575e6` | 1 (anchor-level) | bot thread; script anchors | **EXCLUDED** under the recommended R-C rule | R-C, R-B |
| IDR-L-008 | `docs/specs/RAG-RETRIEVAL-CONTRACT.md` @ `c6ef5f5651cb822d501cf8b095c9f6dd54f1755a` | 1 | Decision 2 §2.2 | **PARTIAL** (unchanged) | none; partial by design |
| IDR-E-006 | `PLAN-Review-Scope-And-Output-Hardening.md` (single commit) | 3 findings | self-recorded at closure | **EVIDENCE_BLOCKED**; finding 1 **DUPLICATE** subject of `IDR-L-004` | owner |
| IDR-E-010 | `docs/architecture/OVERVIEW.md` @ `7de7c68a1d32301ef952956dabe378610ec98df7` | 0 | an evaluation plan's measurement gaps | **EXCLUDED** (stays unverified) | none |
| IDR-E-012 | `PLAN-SOP-End-to-End-Reliability.md` | 0 | — | **DUPLICATE** of `IDR-L-006` (excluded) | none |
| DISC-001 / IDR-H-001 | see #618 | — | — | not re-investigated | corpus-owner rulings (#618) |

"Recommended R-C rule" and "recommended R-C-packet rule" are the options **PROPOSED — PENDING OPERATOR APPROVAL** in the decision package; they are not decisions. Classifications under the alternative options are in the package's matrix.

## 5. Per-lead reconstruction and deterministic results

### 5.1 `IDR-L-001` — retrieval seam wired into implement only (held; now with a persistence check)

- **Pin:** `21564b27…`, the commit that adds the plan (2026-11-06); the plan has two revisions, and `03a7d7d9…` (2026-11-08) only edits the status line (#610/#618-era record, [HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md) §3.1).
- **Passage:** §2 goal "every implement/fix attempt gets the top-k BM25 hits"; §3 item 3 "a fix attempt sees the code the previous attempt wrote"; §4 step 2 done-when is `go build && go test && go vet` clean.
- **Code at the pin (verified):** `internal/cli/run.go` has `fixContext` at line 1826 and the only `codeRetrievalItems(...)` call at line 2433 (the implement context); the fix context never reads the retrieved items.
- **New, verified on `origin/main`:** the gap **persists**: `codeRetrievalItems` has one call site, `internal/cli/run.go:2493`; `fixContext` (line 1883) does not reference it. **No merged commit corrects the defect**, so there is no merged-fix pointer to add; `git log -S'codeRetrievalItems' -- internal/cli` shows only the creation commit.
- **Determinable without hindsight:** yes (plan scope versus the permitted code paths). **Evidence limit:** the only record is the artifact's own post-merge status line, which the preparation plan rejects as an evidence pointer; this pass cannot supply another one.
- **Classification:** **EVIDENCE_BLOCKED.** It would count only if the owner rules that a self-recorded, still-open post-merge limitation is a "recorded gap" (named **R-D** here). **Overlap:** the same path has an excluded manifest entry (`IDR-H-002`); per the convention only the lead entry would count.

### 5.2 `GH-004` — ACP lease-wiring approval packet (two revisions)

- **(a), (b)** verified in #613/#620 (plan edits force a model recompile; the recommended `--list-changed` preview cannot parse the layout).
- **(c) verified now.** At the first pin `c6307fd…` the packet says the plan's `Status:` line "is replaced by an approval line" (about line 34) while its own state table says "the plan file is unchanged, so the digest still matches" (line 27) and Decision 3 Option A promises no plan revision.
  At the same pin `internal/planflow/planflow.go:296–305` (`reconcileGate`) treats any change to a recorded source as `planChangedError` unless `meta.SourceSHA256 == fingerprint(data)`. Editing the status line changes the fingerprint, so Option A's approval step contradicts the gate.
- **(d) PARTIAL, not verified.** At `022e7568…` `redefineTask` (`planflow.go:1218`) preserves "its ID, lifecycle state, attempts, budget, and creation time", which supports the finding that redefining `stage-1` does not retire it. But the same file's `hasExecution` comment says a task that has **never executed** may be removed, and the packet reports `stage-1` at attempt 0/3. Whether `stage-1` is removable was not established, so (d) is not counted.
- **Classification:** 3 verified defects across two revisions of one path. Strict only if approval packets are admitted; the recommended R-C-packet test says an approval request is not a design, so under that rule **EXCLUDED**. Pins are PR heads (R-A), and the second revision's defects depend on the first. **Subject overlap:** the ACP execution subject of `DISC-001` / `IDR-H-001`, so it is not additive to them.

### 5.3 `GH-005`, `GH-006` — reference and guide documents

- `GH-005`: `docs/reference/CLI.md:22` (at `d8aa908d…`) lists `sop planner ask TASK QUESTION`; `internal/cli/planner.go:162` at the same pin has `usage: sop planner ask <task-id> --question TEXT …`. **Verified.**
- `GH-006`: verified in #620 §5.2 (`docs/guides/PROMPTING.md:52` versus `acceptance_cargo_test.go`).
- Both are factual-accuracy defects in reference or guide documents with bot-only evidence: **EXCLUDED** under the recommended R-C rule; each counts only under R-C Option C together with R-B Option B.

### 5.4 `GH2-001`, `GH2-002` — skill procedures

- `GH2-001` second finding **verified now (static):** `skills/decision-ab/SKILL.md` @ `62388bab…` runs `go run ./cmd/deceleval-ab …` (lines 29, 46, 49, 52) with no step that locates the SOP checkout (no `cd`, `rev-parse` or root variable), while `cmd/deceleval-ab/main.go:38–40` defaults `-corpus`, `-freeze`, `-prompt-freeze` to repository-relative `evals/decision-model/…`. From another project directory the command and its frozen inputs do not resolve. Together with the verified `-endpoint` omission (#620) that is **2 verified defects** for `GH2-001`.
- `GH2-002`: script anchors verified in #620 (`pre-push.sh:93`, `--no-merge` at `:44`); the behaviour was not executed.
- Classification as §5.3.

### 5.5 `IDR-L-008` — retrieval contract (unchanged)

`docs/specs/RAG-RETRIEVAL-CONTRACT.md` @ `c6ef5f5651cb822d501cf8b095c9f6dd54f1755a`: partial **by design** (the contract chose a document-scoped hash for a chunk-scoped evidence unit); Decision 2 §2.2 records it as proposed partial. No new evidence; **PARTIAL**, counts toward neither floor (Decision 3).

### 5.6 Evidence-blocked manifest leads

- **`IDR-E-006`:** the "three residual governance findings" are listed in `docs/history/plans/PLAN-RSH-Follow-Up-Hardening.md`. The parent is `PLAN-Review-Scope-And-Output-Hardening.md`, added in **one** commit `ba7736d4c2cbaa2c64df6007428fcb4800adf978` (2026-10-07) together with the implementation (no earlier plan revision), and the residuals were recorded by the author the next day (`f348e289c328f3bd76e32a1f1016ab2e04cfaafd`). So there is no distinct pre-defect revision (the `IDR-H-005` pattern). Finding 1 (implicit malformed-output classification) is the subject of `IDR-L-004`'s defect (b) on `docs/specs/REVIEW.md`; findings 2 and 3 concern a test-coverage list and editor markdown variance (implementation/test level).
  **EVIDENCE_BLOCKED**; finding 1 **DUPLICATE** subject.
- **`IDR-E-010`:** the later evidence is `PLAN-EV-Context-Run-Evaluation.md`, a proposed evaluation plan; its "gaps" are missing measurements (`MISSING` metrics, instrumentation), not defects in a design document, and no narrower pre-fix artifact exists. Stays unverified: **EXCLUDED** as a design defect.
- **`IDR-E-012`:** the later evidence is `PLAN-SOP-End-to-End-Reliability.md`, the artifact of the already-excluded `IDR-L-006` ("the plan itself is the assessment backlog"). No new evidence: **DUPLICATE** of an exclusion.

### 5.7 Spec-amendment sweep

For ten specs (`AGENT-PROVIDER`, `MODEL-ROUTING`, `RECOVERY`, `EXECUTION`, `OPENJEV`, `PROVIDERS`, `VALIDATION`, `HUMAN-APPROVAL`, `DECISION-TELEMETRY`, `SECURITY-SCANNER-ADDON`) the commits that add a normative line are overwhelmingly **feature
commits** that introduce a capability and its requirement together (for example `fa3f5030` "specify the provider runtime layer", `615c898f` "deterministic phase 3.5 JEV-guided model routing"): a requirement arriving with its feature is not a defect in an earlier spec.
The few corrective ones are already `EXCLUDED` (`DISC-009`: `ea6efb07`, `3cd134c8`, `8a3f4f99`) or counted (`GH3-001a`, `aa53c882`). The corrective-keyword search over docs returned **212** SHAs named in no corpus report. The subjects of **110** of them were reviewed (the newest 60, and 50 older non-OWASP commits); the newest are the OWASP programme's own status, decision and fix commits (the bot-surfaced proposals already covered by #613 and #620), and the older ones are feature, test, tooling or implementation-fix commits. One older candidate was read in full: `ce1a7943` (2026-10-26) records, in `docs/plans/BACKLOG.md`, that `runPlannerAsk` reads the run epoch before `SetStageOnExisting` creates it, found during PE-3 live acceptance. That is an implementation ordering bug observed in a live run, not an omission in a design document: **EXCLUDED**. The remaining 102 subjects were not reviewed. **No new lead.**
This is a bounded negative: it covers commit subjects and added normative lines, not every spec edit, and 102 of the 212 corrective-keyword subjects are unreviewed.

## 6. Duplicate and contamination analysis

- **Path additivity (verified against the convention's C2 list):** none of the paths above is among the 13 C2 artifact paths. `IDR-L-001`'s path also has an excluded manifest entry (`IDR-H-002`), which contributes 0; `docs/specs/REVIEW.md` and `PLAN-09-…` are the convention's existing examples.
- **Defect additivity:** `GH-004` shares a subject with `DISC-001`/`IDR-H-001` (not additive until the unit is ruled); `IDR-E-006` finding 1 duplicates `IDR-L-004`(b). Nothing else overlaps.
- **Contamination:** pin trees contain no EGM or corpus files (checked for the C2 pins in #626; not re-run for `IDR-L-001`, whose 2026-11-06 pin predates the corpus work). The later status line for `IDR-L-001` and the closure records for `IDR-E-006` are answer-bearing and must stay out of any reviewer-visible input.

## 7. Eligibility interpretations that need the operator

Facts are in §5; the following are interpretations. All **PROPOSED — PENDING OPERATOR APPROVAL**:

- **R-D (new):** may a defect recorded only in the artifact's own later status line, with the gap still open, count as a "recorded gap"? Affects `IDR-L-001` (and `IDR-H-001`/`IDR-H-005`-type holds). Conservative: no (the preparation plan already rejects it). I recommend **no** unless the owner also records why a self-record is independent.
- R-B, R-C-packet, R-C and the counting unit: unchanged from the decision package; this pass adds no new dependency beyond R-D.

## 8. Count impact (convention classes)

| Class | Plans / defects | Shortfall vs 15 / 15 | Change from this pass |
| --- | --- | --- | --- |
| C0 official baseline | 3 / 4 | 12 / 11 | none |
| C1 conservative | 4 / 5 | 11 / 10 | none |
| C2 conditional, artifact | 13 / 15 | 2 / 0 | none (nothing added) |
| C3 cluster sensitivity | 11 / 15 | 4 / 0 | none |
| New verified strict proposals | **0 / 0** | | |
| C4, held, never totalled | adds only after rulings: `IDR-L-001` (+1 / +1 under R-D), `GH-004` (+1 plan / +3 defects by artifact counting; +0 plans by cluster counting, which is a C3-style view), `GH-005`, `GH-006`, `GH2-001` (2 defects), `GH2-002` | | `GH2-001` 1 → 2 verified defects; `GH-004` (c) verified |

Reference only: the decision package's permissive scenario S6 (17 / 19) used one verified defect for `GH2-001`; with the second verified here it would read 17 plans / 20 defects. That package is not edited by this report.
Not added to C2 automatically: `IDR-L-001` (needs R-D), `GH-004` (packet kind, R-B, R-A) and the R-C leads.

**Conditional scenario S7 (artifact counting; not a total, not added to C2).** Neither `IDR-L-001` nor `GH-004` overlaps a C2 path (§6). `GH-004`'s subject overlap with `DISC-001` / `IDR-H-001` does not reduce an artifact-count increment, because neither of those is in C2. C2 13 / 15 plus `IDR-L-001` (+1 / +1) plus `GH-004` (+1 / +3) = **15 plans / 19 defects** (shortfall 0 / 0), with no skills or guides. It needs R-D **yes** (the report recommends no), approval packets **admitted** (the package recommends rejecting them), R-B Option B, R-A resolved for `GH-004`'s PR-head pins, artifact counting, and every one of the 15 plans surviving adjudication, with no plan slack. It is the minimum-route maximum, not a forecast. Combined with S6 (skills and guides, 17 / 20 on the corrected `GH2-001` count) the conditional ceiling would be higher still.

## 9. Outstanding operator decisions

R-B, R-C-packet, counting unit, R-C (see the decision package), and R-D (§7). Separately: whether to authorize a different evidence source (§10).

## 10. Remaining evidence gaps and recommended next investigations

- **The registered floor of 15 plans is not reached by any scenario that follows the recommended rules** (4 / 5 strictest; 13 / 15 at most without R-C Option C or R-D). It is reachable only conditionally: by S6 (R-C Option C) or by S7 (R-D plus approval packets), each requiring several permissive rulings the packages recommend against and every adjudication to succeed. This pass found no way to add a plan whose evidence meets the conservative reading.
- Four of the six #620 leads (`GH-010`, `GH-011`, `GH-012`, `GH-014`) were verified once and not re-checked at the line level (#626 §11); that remains the cheapest way to firm up C2 but adds no plans.
- `GH2-003`–`GH2-005` remain triaged only; `GH-004` (d) is unresolved; no human has verified any item.

Next three, ranked (none authorized):
1. **Operator decision on a different evidence source.** The registered text does not say plans must come from this repository. The operator's sibling repositories (`inventory`, `medical-billing`, `portfolios`, `scheduling`) may hold specs with human-reviewed corrective history. They are separate repositories, one may hold sensitive data, and independence and authorship would need to be established again; this needs explicit authorization and a scoping decision before any look.
2. **Line-level re-check of `GH-010`, `GH-011`, `GH-012`, `GH-014`** (documentation-only, no authorization needed).
3. **Resolve `GH-004` (d)** by establishing whether a never-executed task (`stage-1`, attempt 0/3) can be removed by reconcile; this would settle whether the packet's fourth defect is real.

## 11. Non-authorization statement

This report authorizes nothing. It does not: promote any candidate; rule on `DISC-001`, `IDR-H-001`, R-B, R-C-packet, R-C, R-D or the counting unit; change Decision 3, any threshold (including the 613-call ceiling) or the registered protocol; modify `candidates.json`, the Decision 5-B manifest tooling or any frozen or sealed artifact;
construct a seeded or control case; create or access a sealed answer key; designate a case checker or adjudicator; authorize case construction; adjudicate; freeze the corpus; run model inference; or execute the experiment. The corpus remains `NOT FROZEN`, the experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.
