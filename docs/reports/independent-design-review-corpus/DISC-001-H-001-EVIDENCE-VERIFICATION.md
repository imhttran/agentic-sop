# DISC-001 / IDR-H-001 Evidence Verification

**Type:** Investigation record and corpus-owner decision package (non-normative, read-only).
It verifies evidence and proposes rulings. It is **not** candidate promotion, an eligibility
decision, adjudication, a corpus freeze, or an experiment result.
**Baseline:** `origin/main` @ `0ae06e5cf20cddf3d9da351f2022cb3eb8b2d370`, 2026-11-21.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.2–§5.3
(SHA-256 `abba919e…2305c`, unchanged).
**Eligibility rule:** [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md)
(`conservative`).
**Related records (unchanged by this report):** [candidates.json](candidates.json),
[CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md),
[HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md),
[HISTORICAL-CORPUS-EVIDENCE-DISCOVERY.md](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY.md).

**Every eligibility conclusion below is PROPOSED — NOT APPROVED.** Verification results
(what the code and documents say at a named commit) are facts a reader can re-run; rulings
are the corpus owner's.

## 1. Historical revision timeline

All commits are by one Git identity ("Tom Tran"), so this report draws **no** authorship or
independence conclusion from Git identity. Times are as recorded by Git (US Central).

| Date | Commit | Event |
| --- | --- | --- |
| 2026-11-01 | — | `PLAN-SOP-ACP-ZED.md` carries `Date: 2026-11-01` in its own header. This is a self-stated date; no earlier Git revision exists |
| 2026-11-02 14:13 | `79acbfc1` | **First appearance in Git** of `PLAN-SOP-ACP-ZED.md`, `ACP-EXECUTION-EXCLUSION-GAP.md`, `PROPOSAL-SHARED-EXECUTION-EXCLUSION.md`, the ACP implementation (`internal/acp`, `internal/cli/acp.go`) and its tests. Parent `82ad60bc` has none of them |
| 2026-11-02 16:58 | `e462deed` | Index update; the plan's status line only |
| 2026-11-03 | `84bfd6ae`, `75e3df0f`, `3ad9d447` | Execution lease added behind `execution.lease`, wired into `sop run`/`resume`; gap record edited |
| 2026-11-21 12:37 | `ade6864a` | PR #571, "EGM review fixes" to the gap record. Adds the lease-wiring table and the text `sop drive`. The tree also gains `docs/plans/ACP-EXECUTION-EXCLUSION-GAP-REVIEW/` (EGM review reports) |
| 2026-11-21 12:50 | `342c1593` | Correction commit. Its message says three wrong claims were found by a model-run `sop prompt --capability review` |
| 2026-11-21 13:34 | `ea01b10e` | Further reconciliation and ACP-vs-MCP start coverage |
| 2026-11-21 16:27 | `d65ce63d` | The ZED plan's status line and acceptance rows are edited to record the known gap |

**PR #197** (the PR behind `79acbfc1`; merged 2026-11-02T20:35Z) has three commits:
`79acbfc1`, `6fb9b210` (report links) and `c8fb3bee` (allocation fix). **None is earlier than
`79acbfc1`**, so no pre-commit revision of the plan survives in Git. This is a coverage limit,
not proof that none existed elsewhere.

Blob identities used below: gap record `37625350c764` @ `79acbfc1`, `990ecfc7aeb1` @
`ade6864a`; ZED plan `c3723af8b19e` @ `79acbfc1`, `d9d5539c79ac` @ `e462deed` and `ade6864a`
(SHA-256 prefixes).

## 2. Evidence for each DISC-001 defect

Method for every row: read the document at the named commit, then verify against the code **at
that same commit** (`git show <sha>:<path>`, `git grep <sha>`). The fix commit `342c1593` is
used only to identify what to check; it is not treated as proof. The EGM review reports are
not used.

### Defect 1 — nonexistent command `sop drive`

| Question | Result |
| --- | --- |
| Present in the document at `79acbfc1`? | **No.** `git show 79acbfc1:docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` contains no `sop drive` |
| Present at `ade6864a`? | **Yes**, lines 7, 51, 97 (e.g. "wired for `sop run` and `sop drive`", table row "CLI `sop drive`") |
| Introduced by | `ade6864a` (`git log -S'sop drive'` lists `ade6864a`, which adds it, and `342c1593`, which removes it) |
| Command at the pin? | **Does not exist** at `79acbfc1` or `ade6864a`. `internal/cli/cli.go` dispatches `plan`, `run`, `prompt`, `resume` (`cli.go:394–418` at `ade6864a`); `git grep -w drive` over `cli.go` and `cmd/` is empty. Graph mode is `runGraph` / `driveGraph` in `internal/cli/drive.go` (`drive.go:49`, `:467` at `ade6864a`), reached through `sop run` |
| Verification method | Deterministic `git grep` on the pin; no model involved |
| Existed before the correction? | Yes, from `ade6864a` only |
| Discoverable without hindsight? | **Yes** at `ade6864a` (document + `cli.go`). Not applicable at `79acbfc1` |
| Uncertainty | Added by a review-fix commit, not by the original author of the record. Whether that makes it a "plan defect" is the artifact-eligibility question (§6) |

### Defect 2 — test coverage overclaimed

Document at `79acbfc1` line 35 and `ade6864a`: "This proves **ACP-vs-MCP** and **ACP-vs-ACP**
start exclusion only." (lines 31–33 correctly describe the test as racing two "submissions".)

| Question | Result |
| --- | --- |
| Present at `79acbfc1`? | **Yes**, identical wording |
| Test | `TestACPPromptConcurrentStartsCreateOneWorkItem`, `internal/cli/acp_test.go:416` at both commits |
| What it does | Its own comment (`acp_test.go` just above line 416) reads: "Two ACP sessions submit the same prompt at the same time." The body builds one `acpSubmission` bridge, one `acp.Submission`, and races `racers = 2` goroutines through it, then asserts `launcher.count() == 1`. An `awk` extract of the whole function body finds **no** MCP call path (`sop_start_work`, `handleStartWork`, `startWork(` directly) |
| Conclusion | The test proves **ACP-vs-ACP** only. "ACP-vs-MCP" is not exercised. The shared `startWork` lock makes ACP-vs-MCP exclusion plausible by inspection, but no test races it |
| Existed before the correction? | Yes, from `79acbfc1` |
| Discoverable without hindsight? | **Yes**: the document, the test file and `mcp_start_work.go` suffice |
| Uncertainty | Low. A reader could treat "by construction" as sufficient evidence for the claim; the document uses the word "proves" |

### Defect 3 — mid-run detection

Document at `79acbfc1` lines 29–30 and 52–54: a `sop run` "mid-task" is "detected from the
persisted task states (`midRunTasks`)", and an ACP start is excluded "if it happens to observe a
mid-run task state. Between task transitions it can start a second execution."

| Question | Result |
| --- | --- |
| Code (both commits) | `workMidRunStatuses` (`mcp_start_work.go:94`) lists eight statuses (`BRANCH_CREATED` … `FIX_REQUIRED`). `midRunTasks` (`:571`) returns tasks whose `Status` is in that set. It is called once, in `startWork` (`:359`), and refuses with `task_in_progress` |
| Does a fresh `sop run` persist those statuses? | **No.** `completeTask` (`internal/cli/drive.go:1200–1230` at `79acbfc1`) advances a copy of the task through `localCompletionPath` and calls `saver.Save(&staged)` **once**. The intermediate statuses are therefore not written for a fresh task. Only a task parked in one of them (an interrupted run) is seen |
| Verified claim | A fresh `sop run` is invisible to `midRunTasks` for the whole task |
| Is the original sentence false? | **Not strictly.** "Excluded only if it happens to observe a mid-run task state" is true. "Between task transitions" misleadingly implies intermediate transitions are persisted and that detection windows exist during a run. The defect is an **imprecise and misleading** description, not a plain falsehood |
| Existed before the correction? | Yes, from `79acbfc1` (wording unchanged through `ade6864a`) |
| Discoverable without hindsight? | **Yes, with effort**: needs reading `drive.go` and following the save path |
| Uncertainty | **Material.** The correction rests on a code reading, not a reproduced run; whether "misleading" is a determinable defect is a judgement. Proposed class: **PARTIAL**, not counted |

### Additional finding (not in the discovery report) — D4, wrong lock path

| Question | Result |
| --- | --- |
| Document at `79acbfc1` line 24 | "`startWork` takes a cross-process start lock, `.agent-sdlc/runs/work/.lock`" |
| Code at `79acbfc1` | `workDirName = "mcp-work"` (`mcp_start_work.go:44`), `workLockFile = ".start.lock"` (`:51`) → the lock is `.agent-sdlc/runs/mcp-work/.start.lock` |
| Note | The same wrong path appears in the **code comment** at `internal/cli/acp.go:~91` at `79acbfc1`, so a reader following the comment would reproduce it. The document and comment disagree with the constants |
| Fixed by | The `ade6864a` review-fix diff (`work/.lock` → `mcp-work/.start.lock`), **before** `342c1593`. So D4 is present at `79acbfc1` and absent at `ade6864a` |
| Discoverable without hindsight? | **Yes**, deterministic (`grep` of the constants) |
| Uncertainty | Low impact; a minor factual defect |

### Summary — defects by revision

| Revision | D1 `sop drive` | D2 ACP-vs-MCP | D3 mid-run | D4 lock path |
| --- | --- | --- | --- | --- |
| `79acbfc1` (original) | absent | **present, verified** | present, imprecise (partial) | **present, verified** |
| `ade6864a` (pre-`342c1593`) | **present, verified** | **present, verified** | present, imprecise (partial) | absent (fixed) |

The discovery report (PR #610) examined `ade6864a` and recorded D1–D3. Its open question
("whether defects 1–3 were present at the original revision") is answered: **D2 and D3 were;
D1 was not; and D4 was, though D4 was already gone at `ade6864a`.**

## 3. Independent code verification (summary)

All checks are deterministic and offline; none uses a model or the `…-GAP-REVIEW/` reports.

| Check | Command shape | Result |
| --- | --- | --- |
| No `drive` command | `git grep -n -i -w drive <sha> -- internal/cli/cli.go cmd` | empty at both |
| Test scope | `git show <sha>:internal/cli/acp_test.go` and extract the function | ACP only |
| `midRunTasks` behavior | `git show <sha>:internal/cli/mcp_start_work.go`; `git show <sha>:internal/cli/drive.go` (`completeTask`) | eight statuses; fresh run saves once |
| Lock path constants | `git grep -n -E 'workDirName|workLockFile' <sha> -- internal/cli/mcp_start_work.go` | `mcp-work`, `.start.lock` |
| Absence of lease at the original | `git ls-tree -r --name-only 79acbfc1 internal/executionlease` | empty (lease arrives `84bfd6ae`) |

**Not re-derived:** whether the doc's consequence statements (a double execution damages task
rows, run artifacts, and the working tree) are correct. They were added in `342c1593` and are
out of scope; they are not claimed as defects here.

## 4. Discoverability assessment

| Defect | Visible at pin | Hindsight needed? | Permitted references that suffice |
| --- | --- | --- | --- |
| D1 | `ade6864a` | no | the document; `internal/cli/cli.go` |
| D2 | both | no | the document; `internal/cli/acp_test.go`; `mcp_start_work.go` |
| D3 | both | partly (judgement) | the document; `mcp_start_work.go`; `drive.go` |
| D4 | `79acbfc1` | no | the document; `mcp_start_work.go` |

**Contamination to exclude from any reviewer-visible tree.** At `79acbfc1`, the same commit
adds `PROPOSAL-SHARED-EXECUTION-EXCLUSION.md`, which names the gap. At `ade6864a`, the tree
holds `docs/plans/ACP-EXECUTION-EXCLUSION-GAP-REVIEW/` (the EGM review reports; added by that
same commit). The corrected gap record's reviewer must not see the review reports, the
proposal's later rewrite, or any commit after the pin. Per protocol §5.2, an exported tree or
history-truncated clone is required (a plain clone exposes later history).

## 5. IDR-H-001 and the relation to DISC-001

### 5.1 What `IDR-H-001` is

`candidates.json`: artifact `docs/plans/PLAN-SOP-ACP-ZED.md` pinned at `e462deed`; defect
summary "The acceptance matrix claims the project lock protects CLI, MCP, and ACP equally. The
gap report shows the lock is metadata-only and is not held for execution." State `VERIFIED`,
proposed inclusion `hold`, determinability `pending-corpus-owner-review`.

### 5.2 Verified at the pin

- **Passages.** `PLAN-SOP-ACP-ZED.md` at `79acbfc1` (identical at `e462deed` apart from the
  status line): line 18 ("one active execution per project, including executions started
  through CLI or MCP"), line 86 ("Reuse project locking across ACP, CLI, and MCP"), line 106
  ("Competing CLI/MCP/ACP starts obey the same lock"), line 291 (acceptance row "Concurrent
  starts — Existing project lock protects CLI, MCP, and ACP equally").
- **Code at `79acbfc1`.** `internal/projectlock/lock.go`'s package comment states the lock
  "serializes the writers of a project's durable state" and "provides MUTUAL EXCLUSION ONLY";
  `acp.go:80` states "The probe is NOT the execution-exclusion mechanism … neither `sop prompt`
  nor `startWork` takes it". `git grep 'projectlock\.' 79acbfc1 -- internal/cli/{run,drive,prompt}.go`
  finds no acquisition for execution. The execution lease does not exist yet.
- **Conclusion.** The plan's acceptance claim (the existing project lock protects CLI, MCP and
  ACP equally) is **contradicted by the permitted reference code at the plan's own first
  revision**, independent of the gap record. The defect is **verified and determinable from
  the plan and the code alone**, with no later evidence.

### 5.3 Why it is still held

1. **No uncontaminated pre-defect revision.** `79acbfc1^` has no plan, and PR #197 has no
   earlier commit. Protocol §5.2 wants "the commit before the defect was discovered" and §5.3
   asks whether the defect was determinable "from the document and its parent revision". For
   this plan the parent revision has no document.
2. **The plan and the disclosing gap record entered Git together** (`79acbfc1`). The gap record
   is therefore not later evidence by commit order; it is the same-commit answer.
3. **Self-disclosure in code.** `acp.go:80` at the pin already says the probe is not the
   exclusion mechanism. That is determinability evidence (good) but also means the permitted
   references partly disclose the defect (a contamination question the owner should weigh).
4. **Authorship** cannot be inferred from Git identity (one identity throughout).

### 5.4 Duplicate analysis

| Comparison | Finding |
| --- | --- |
| `IDR-H-001` vs `DISC-001` D1 | **Distinct.** D1 is a nonexistent command in the gap record; H-001 is a false acceptance claim in the plan |
| `IDR-H-001` vs D2 | **Related, not duplicate.** Both concern whether exclusion across interfaces is proven. H-001 claims the lock protects all three interfaces; D2 claims a test proves ACP-vs-MCP. Different artifacts, passages and evidence |
| `IDR-H-001` vs D3 | **Related, not duplicate** for the same reason; D3 concerns detection of a running `sop run` |
| `IDR-H-001` vs D4 | **Distinct** (path constant) |
| `DISC-001` vs `IDR-L-001`–`L-008`, `DISC-002`–`DISC-012` | No overlap found in the manifest and lead files read |
| Shared root | H-001, D2 and D3 all stem from one design fact: **no project-scoped execution lock existed**. A corpus owner may treat that as one subject cluster. See §8 for floor arithmetic |

**Do the two need different rulings? Yes.** H-001 asks whether a *plan* with no earlier
revision and a same-commit disclosure is admissible. DISC-001 asks whether a *gap record* is an
admissible artifact, whether factual errors about code are admissible defects, and which pin to
use. They share a subject but not a question.

## 6. Artifact-eligibility alternatives

**Question A.** Does the registered protocol permit the ACP gap record to count as a historical
plan/specification/requirements input?

**Exact protocol language** (§5.3, "Historical set"): "plans, specs or requirements whose defect
later appeared in merged fixes, erratum, reverted change or recorded gap. Each defect has an
evidence pointer (commit, PR or file). Candidates to verify, not yet verified:
`docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md`, `docs/reports/PLAN-STATUS-AUDIT-2026-11-04.md`,
`docs/reports/CONV-001-convergence-baseline.md`." (§5.2: "For a historical plan, the clone is
checked out at the commit before the defect was discovered".)

| # | Interpretation | Supported by | Consequence | Amendment? |
| --- | --- | --- | --- | --- |
| A1 | **Not admissible.** The gap record is a report about code ("a recorded gap"), i.e. the *evidence pointer* for a plan's defect, not a "plan, spec or requirement" | The sentence structure: artifacts are "plans, specs or requirements"; "recorded gap" is a *way a defect later appears* | The record serves only as later evidence for `IDR-H-001`; DISC-001 contributes 0 plans / 0 defects; no contamination problem between the two | No |
| A2 | **Admissible** as a spec-like input: it sits in `docs/plans/`, states a "Requirement", an acceptance position and a closing mechanism, and the protocol itself names its path as a candidate to verify | The explicit candidate listing in §5.3; the doc's structure; Decision 4 found no amendment necessary | Counts as 1 plan if a defect is ruled eligible. But then the same record is **both** a reviewer-visible input (for DISC-001) and the *answer evidence* for `IDR-H-001`. Isolation per §5.2 handles this (one project per run), but the exported trees must differ and the owner must document it | No (the protocol lists the path); the owner records admissibility and determinability at freeze |
| A3 | **Admissible only at one pin**, chosen to avoid contamination (see §4), with H-001 and DISC-001 not both counted | Dependence of H-001 and D2/D3 on one design fact | 1 plan counted for the cluster; the other candidate stays as evidence | No |

**Recommended (PROPOSED — NOT APPROVED): A2, with A3's independence rule.** The protocol itself
names the record's path as a candidate; excluding it by A1 would read a restriction into the
text that the text does not state, and Decision 4 already rejected the amendment path as
unnecessary. A2 needs only the owner's recorded admissibility and determinability judgement at
freeze, not a protocol change. Treat H-001 and DISC-001 as separate artifacts but count their
overlap once (§8).

## 7. Defect-eligibility alternatives

**Question B.** Do factual inaccuracies about existing code qualify as design-review defects
under the registered evaluation?

**Exact protocol language.** §4: "Does a design-stage review find more **genuine design defects**
than SOP's existing generic ad-hoc review". §5.3 seeded classes: "unstated assumption,
nonexistent symbol, missing failure mode, unverifiable acceptance criterion, scope violation".
§5.3 defect record: "severity (blocks implementation / changes approach / minor)". §5.3: "the
corpus owner records whether each defect was determinable from the document and its parent
revision, with a one-line basis".

| # | Interpretation | Supported by | Consequence | Amendment? |
| --- | --- | --- | --- | --- |
| B1 | **Narrow.** Only defects in the design content (requirements, acceptance criteria, design choices) qualify; a wrong claim about existing code does not | The word "design defects" | D1, D2, D4 excluded. Only H-001-style defects (a false acceptance claim) count | No |
| B2 | **Broad, by class.** A statement in a design-stage document that is contradicted by the code at the pin counts when determinable against that code. The protocol's own seeded defect class **"nonexistent symbol"** is exactly D1; D2 is an overclaimed acceptance criterion ("unverifiable acceptance criterion" family) | The registered seeded classes; the severity scale includes "minor" | D1, D2 and D4 qualify; D3 qualifies only if "misleading" is accepted as determinable (this report proposes **partial**) | No |
| B3 | **Impact-gated.** Qualifies only when the error would change an implementation or acceptance decision ("blocks implementation" or "changes approach") | The severity scale | D2 (acceptance would be declared on a test that does not cover it) qualifies; D1 and D4 are "minor" and drop | No. The severity scale already allows "minor" as a recorded severity, so B3 would add a gate the protocol does not state |

**Recommended (PROPOSED — NOT APPROVED): B2.** It matches the protocol's own defect classes and
needs no amendment. Record D1 and D4 as "minor" and D2 as "changes approach". Keep D3
**PARTIAL**, uncounted, because the original sentence is misleading rather than false and its
correction depends on a code reading.

## 8. Provisional floor impact

All figures are **provisional arithmetic, not verified eligibility**. Independently adjudicated
remains **0 / 0**. Partial, hindsight-only and seeded cases are not counted (Decision 3). Counts
below use the **conservative** defect set (clean defects only; D3 excluded).

Baseline: proposed strict **3 plans / 4 defects**; required **15 / 15**; shortfall **12 / 11**.

| Scenario | Rulings | Plans added | Defects added | Proposed strict | Shortfall |
| --- | --- | --- | --- | --- | --- |
| S0 — no change | none | 0 | 0 | 3 / 4 | 12 / 11 |
| S1 — H-001 only | H-001 admitted (pin issue resolved); DISC-001 not admitted | +1 | +1 | 4 / 5 | 11 / 10 |
| S2a — DISC-001 only, original pin `79acbfc1` | A2 + B2; D2, D4 counted | +1 | +2 | 4 / 6 | 11 / 9 |
| S2b — DISC-001 only, pre-fix pin `ade6864a` | A2 + B2; D1, D2 counted | +1 | +2 | 4 / 6 | 11 / 9 |
| S3 — both, counted independently | S1 + S2a or S2b | +2 | +3 | 5 / 7 | 10 / 8 |
| S4 — both, one subject cluster | owner counts the cluster once; the larger set | +1 | +2 | 4 / 6 | 11 / 9 |
| S5 — upper bound, D3 also accepted | S3 plus D3 | +2 | +4 | 5 / 8 | 10 / 7 |
| S6 — neither admitted (A1/B1) | none | 0 | 0 | 3 / 4 | 12 / 11 |

**No scenario reaches 15 / 15.** The best case (S5) leaves a shortfall of **10 plans / 7
defects**. These are possible additions, not members.

## 9. Recommended corpus-owner decision (PROPOSED — NOT APPROVED)

1. **Admit `IDR-H-001`'s subject, with the pin issue ruled explicitly.** Because the plan, the
   implementation and the gap record share one commit, the protocol's "commit before the defect
   was discovered" cannot be met literally. Either (a) rule that the exported tree at `79acbfc1`
   **minus** `ACP-EXECUTION-EXCLUSION-GAP.md` and `PROPOSAL-SHARED-EXECUTION-EXCLUSION.md` is the
   reviewer-visible input, with `acp.go:80`'s comment left in as a permitted reference and
   recorded as partial self-disclosure, or (b) keep H-001 held. This is the owner's call; the
   determinability evidence is strong either way.
2. **Admit the gap record as an artifact under A2**, and defects under **B2**, at the pre-fix
   pin **`ade6864a`** (the commit before the defects were discovered, per §5.2), with an exported
   tree that omits `…-GAP-REVIEW/` and later commits. At that pin the clean defects are **D1 and
   D2**; D3 stays partial. Alternatively pin `79acbfc1` (D2 and D4). Do not mix defect sets from
   the two pins.
3. **Count the H-001 / DISC-001 overlap once** unless the owner records, per defect, why they are
   independent (§5.4).
4. **Record determinability at freeze** with a one-line basis per defect, using §2 as the
   evidence pointer. Re-run the §3 checks at freeze; do not rely on this report.

None of this is implemented by this report.

## 10. Unresolved evidence gaps

- **Original authorship timing.** No pre-`79acbfc1` revision exists in Git, PR #197 or the
  archive checked; the plan's own `Date: 2026-11-01` is self-stated.
- **D3 reproduction.** Code-read only; no run was executed (no inference or experiment is
  authorized here).
- **D1 origin.** Introduced by a review-fix commit; whether a document defect introduced by the
  review process counts as a "historical defect" of the plan is a ruling, not evidence.
- **Discovery was model-assisted** (`342c1593` message). The verification here is independent of
  that run, but the *finding* originated there.
- **Pin choice** (`79acbfc1` vs `ade6864a`) changes which defect set applies and is the owner's.
- **Same-author correlation** and a single Git identity limit any independence claim.
- **Not searched:** PR #571 and PR #197 review threads were not read for this report beyond PR
  #197's commit list; they could hold further pre-fix evidence.
- **Contamination to document** at freeze: `acp.go:80`'s self-disclosing comment; the
  same-commit proposal document; the review reports in the `ade6864a` tree.

## 11. Non-authorization statement

This report authorizes nothing. It does **not**: promote or approve `DISC-001` or `IDR-H-001`;
modify `candidates.json`, Decision 3 or any decision record; change a threshold or the registered
protocol; construct a seeded or control case; access or create a sealed answer key; designate an
adjudicator, seeder, case checker or analyst; adjudicate; freeze the corpus; run model
inference; or execute the experiment. All counts above are provisional arithmetic; independently
adjudicated remains **0**. The corpus remains `NOT FROZEN`, the experiment `NOT YET READY`, and
H1/H2 `NOT ESTABLISHED`.
