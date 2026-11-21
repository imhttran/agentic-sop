# Historical Lead Investigation — Independent Design Review Corpus

**Type:** Extraction record (non-normative). It resolves the six leads listed in
[CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md) §2.2 against git history, and §8
records two additional leads investigated after PR #587. It promotes no candidate, freezes
nothing, and records no adjudication.
**Baseline:** `origin/main` @ `5a94dfe1` (merge of PR #585) for §1–§7; `origin/main` @
`ebe26a11` (merge of PR #587) for §8.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Manifest:** [candidates.json](candidates.json) — unchanged by this record; still 23 entries.
**Machine-readable extraction:** [lead-extraction.json](lead-extraction.json).
**Preparation plan:** [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md).
**Readiness:** [READINESS.md](READINESS.md).
**Verification:** [CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md).

## 1. Method

For each lead I applied the preparation plan's §2.1 procedure, in this order:

1. Enumerate every revision of the artifact with `git log --all --follow`, and select the
   **earliest** revision that predates the discovery of the defect. A revision is
   rejected when it already contains the correction, the remediation, or the disclosure.
2. Resolve the revision and recompute the artifact's SHA-256 from the pinned blob.
3. Read the artifact and list only the repository paths that existed at that revision.
   Each listed path is checked with `git cat-file -e <commit>:<path>` and its blob SHA-256
   is recorded in `lead-extraction.json`.
4. Search the artifact and those paths for the defect, **without** the later report, fix
   commit, or remediation plan. The later evidence is used only to confirm that the defect
   is real, never to establish that a reviewer could find it.
5. Verify ancestry with `git merge-base --is-ancestor`, so "later" is a git fact rather
   than a date comparison.
6. Classify the defect as **design** or **implementation-only**, and determinability as
   **strict** or **partial**. Both are proposals; the corpus owner records the final
   judgment.
7. Record a disposition. No lead is counted toward a floor, and no lead is eligible,
   until the corpus owner records it.

Two results are deliberately reported as negative: a lead that resolves to an
implementation-only defect is not a design-corpus input, and a lead with no single
pre-defect design artifact is not made one by re-reading the remediation plan harder.

## 2. Summary

| Lead        | Artifact @ revision                                         | Artifact SHA-256 | Later independent evidence                              | Defect kind                                     | Proposed determinability                                   | Disposition                                        |
| ----------- | ----------------------------------------------------------- | ---------------- | ------------------------------------------------------- | ----------------------------------------------- | ---------------------------------------------------------- | -------------------------------------------------- |
| `IDR-L-001` | `PLAN-Code-Retrieval-Context-Seam.md` @ `21564b27`          | `75513c86…677b`  | None beyond the artifact's own status line (`03a7d7d9`) | Design (incomplete acceptance criterion)        | Strict from the artifact, but evidence is self-referential | **Hold** — evidence-pointer rule                   |
| `IDR-L-002` | `PLAN-08-…-Hidden-Context-Exposure.md` @ `e84eb4aa`         | `befb303c…5125`  | `aa3e1d36` (08B) and the LLM08 report                   | Implementation-only (fixed); open residual GAP  | Not a design defect                                        | **Exclude** as a design-corpus input               |
| `IDR-L-003` | `PLAN-09-…-Vector-and-Embedding-Weaknesses.md` @ `4fb3b8a3` | `085a190a…f259`  | `PLAN-09B…` @ `05685308`, PR #363 `e90ecf97`            | Design (unverifiable acceptance criterion)      | **Strict (proposed)**                                      | **Extracted** — corpus-owner classification needed |
| `IDR-L-004` | `docs/specs/REVIEW.md` @ `5fce2b6a`                         | `356d83a4…de84`  | RSH-001, RSH-005, `4223e10b`, `ba7736d4`                | Design (scope violation + missing failure mode) | **Strict (proposed)**                                      | **Extracted** — corpus-owner classification needed |
| `IDR-L-005` | `PLAN-Post-Wave3-Production-Readiness.md`                   | —                | The plan itself is the gap record                       | Implementation / integration gaps               | Not determinable as a single design artifact               | **No artifact** — bounded negative                 |
| `IDR-L-006` | `PLAN-SOP-End-to-End-Reliability.md`                        | —                | The plan itself is the assessment backlog               | Integration gaps                                | Not determinable as a single design artifact               | **No artifact** — bounded negative                 |

**Counts after extraction (not eligible, not adjudicated):** four pre-defect artifacts
were extracted; two plans carry three proposed-strict design defects; one lead is
evidence-blocked; one is implementation-only; two have no single artifact. The manifest is
unchanged at 23 entries, 0 strict, 10 partial, 2 verified-defect holds, 3 unverified holds,
and 8 excluded. Independently adjudicated determinability remains **0**.

§8 records two further leads found after PR #587: one proposed-strict design defect and one
proposed-partial design limitation. Its counts are separate from the six-lead figures above.

## 3. Per-lead records

### 3.1 `IDR-L-001` — code-retrieval plan, implement/fix scope

- **Artifact:** `docs/plans/PLAN-Code-Retrieval-Context-Seam.md`
- **Revision:** `21564b27bd9f88c288a47ba71b492b77e9468db3` (2026-11-06), the commit that
  adds the plan. SHA-256 `75513c8629d8ad592acc4747e76f3d45250e082c184281e46d729fed4eb1677b`.
- **Why this revision:** the plan has exactly two revisions. `03a7d7d9` (2026-11-08) edits
  only the status line and adds the sentence that names the gap. `21564b27` is the
  earliest and does not contain that sentence.
- **Later evidence:** none independent. The only record of the defect is the artifact's own
  status line at `03a7d7d9`: "Post-merge limitation: retrieval is wired to IMPLEMENT but not
  FIX, despite the original implement/fix scope." `21564b27` is an ancestor of `03a7d7d9`;
  the latter is the same file, two days later.
- **Permitted references (exist at `21564b27`):** `internal/cli/run.go`
  (`b9e3b3aa…`), `internal/cli/retrieve.go` (`9efa4aac…`).
- **What the artifact says:** §2 goal — "every implement/fix attempt gets the top-k BM25
  hits"; §3 item 3 — "Built once per attempt so a fix attempt sees the code the previous
  attempt wrote"; §4 step 2 done-when — `go build && go test && go vet` clean.
- **What the permitted paths show at the same revision:** `implementContext` passes
  `Retrieved: codeRetrievalItems(...)` into `sopctx.Inputs` (`run.go:2433`). The fix path
  does not: it builds `fixContext(...)` (`run.go:1152`, defined at `run.go:1826`) and never
  reads `sopctx.Inputs.Retrieved` or calls `codeRetrievalItems`. The retrieval seam was
  wired into the implement context only.
- **Ancestry:** `21564b27` is an ancestor of `03a7d7d9`; both are on `main`.
- **Leakage:** the artifact at `21564b27` does not state the gap. The answer first appears
  at `03a7d7d9`.
- **Design vs implementation:** **design**. The plan's stated scope is implement/fix, but
  its step-2 acceptance criterion is a build/test/vet check that cannot detect a missing
  fix-path wiring. That is an incomplete acceptance criterion, not a coding slip.
- **Determinability:** a reviewer with the plan and the two permitted paths could identify
  the mismatch. The _defect_ is discoverable strictly; the _evidence_ is not independent.
- **Authorship / contamination:** Tom Tran `<trantom512@gmail.com>`, 2026-11-06. The
  operator is treated as an author. No agent co-author identifies a human adjudicator.
- **Disposition:** **Hold.** The preparation plan §2.3 rejects a document whose only defect
  evidence is its own later status line. This lead is extracted and its determinability is
  proposed strict, but it cannot enter the corpus unless the corpus owner records a
  different evidence pointer or an explicit rule that a self-recorded post-merge limitation
  counts as a "recorded gap" (decision 2). **It is not counted.**

### 3.2 `IDR-L-002` — Plan 08, hidden-context exposure

- **Artifact:** `docs/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md`
- **Revision:** `e84eb4aa43c3aa70b1203a6160d84ba997f13670` (2026-11-09), the last
  pre-follow-up revision and the revision the 08B plan records as the Plan 08 campaign
  baseline. SHA-256 `befb303c4068de400d69e4c692156738ad3685beb34ad8d67abc9b0ee3ae5125`.
- **Why this revision:** the earlier revisions `4fb3b8a3` (original, 80 lines) and
  `462ed9bb` (adapted, 246 lines) are also pre-defect; `e84eb4aa` adds the CONTEXT-003
  evidence and is the revision later cited. `87e7fad8` moves the file to `docs/history/`
  with no content change.
- **Later evidence:** `aa3e1d364d4bf6a6a5d575291a4739e35b956f00` (2026-11-09) adds the 08B
  closure plan, the refreshed LLM08 report, and `internal/notify/event.go` redaction.
- **Permitted references (exist at `e84eb4aa`):**
  `docs/reports/owasp-llm/llm08-hidden-context.md` (`ff1e4ac2…`),
  `internal/notify/event.go` (`0217fd4e…`), `internal/toolharness/harness.go`
  (`010a2165…`).
- **What the defect is:** the 08B report records one demonstrated and fixed defect — the
  notification event constructor copied `Reason`, `NextAction`, and `Outcome` verbatim, so
  a credential-shaped approval reason could be persisted to the outbox and delivered to a
  Slack channel or webhook — and one open GAP: a `.env` canary is model-visible through
  tool reads because no tool-output policy exists.
- **Leakage:** the artifact set at `e84eb4aa` does not contain either statement. The LLM08
  report there has no notification or tool-output row. The answer first appears at
  `aa3e1d36`.
- **Design vs implementation:** the **fixed** defect is **implementation-only**. Plan 08's
  stated scope is model-visible hidden context and authority independence; notification
  delivery is an outbound channel, not model-visible context, and the plan explicitly
  routes shared disclosure defects to Plan 03's owner. The plan did not claim to cover the
  notification path, so its omission is not a defective design artifact. The `.env` GAP is
  a **recorded residual risk**, not a corrected defect: the 08B plan deliberately
  introduced no tool-output policy. A residual risk is not a defect that a design review
  should have prevented.
- **Determinability:** not applicable as a design-corpus input. Counting it would require
  reading a code-level redaction call as a design omission, which would let almost any
  implementation fix become a historical design candidate.
- **Authorship / contamination:** Tom Tran `<trantom512@gmail.com>`, 2026-11-09.
- **Disposition:** **Exclude** as a design-corpus input. The verified defect evidence is
  preserved; `IDR-H-007` remains excluded and its permitted-reference list remains cleared.
  The open tool-output GAP stays recorded as a residual risk in the 08B evidence and is not
  promoted.

### 3.3 `IDR-L-003` — Plan 09, retrieval isolation

- **Artifact:** `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`
- **Revision:** `4fb3b8a3bff7e86e56eb7287fb6df800f6c2d737` (2026-11-08), the original plan.
  SHA-256 `085a190a3430d5cbd43f1cb2074a58f3fbd0f6f1cf123c019e33494d28eff259`.
- **Why this revision:** `4fb3b8a3` is the first revision and predates every execution
  commit. `536057ab` and `ca4c800e` are adaptation/execution revisions; `63a8c400`
  introduces the rebuild-import/query-scope separation; `05685308`
  archives the 09B follow-up. Using `ca4c800e` would be unsafe because the report it
  produces already discusses rebuild evidence.
- **Later evidence:** `docs/history/plans/PLAN-09B-SOP-LLM09-Retrieval-Followup.md`
  (`05685308`), PR #363 merged as `e90ecf9761c65222d15464319e1d25f5e6a42453`, and the
  corrected `docs/reports/owasp-llm/llm09-retrieval.md`.
- **Permitted references (exist at `4fb3b8a3`):** `internal/memindex/rebuild.go`
  (`d2649224…`), `internal/memindex/index.go` (`7ad1f073…`),
  `internal/outcomefile/outcomefile.go` (`9db4a0b7…`),
  `internal/retrievalboundary/boundary.go` (`aa3a4af4…`). The LLM09 report is **not** a
  permitted reference: it does not exist at `4fb3b8a3`.
- **What the artifact says:** `RETRIEVE-003` acceptance — "Rebuild preserves scope and does
  not silently import another workspace."
- **What the permitted paths show at the same revision:** `RebuildFromDir` reads every
  record in the directory it is given (`outcomefile.ReadAllContext(ctx, dir)`), applies
  eligibility and provenance gates, and indexes every survivor with no repository filter.
  Isolation is applied only at query time: `Index.Search` adds
  `AND repository = ?` when `ScopedQuery.Scope.Repository` is set. A shared outcomes
  directory therefore imports both repositories' rows by design, and a scoped query
  returns only the trusted one. The plan's acceptance criterion conflates import scope with
  query scope, so it is not verifiable as written.
- **Ancestry:** `4fb3b8a3` is an ancestor of `536057ab`, `ca4c800e`, `63a8c400`, and
  `05685308`; all are on `main`.
- **Leakage:** the artifact at `4fb3b8a3` does not mention the conflation. The corrected
  statement first appears in the execution line that leads to `63a8c400` and `05685308`.
- **A point that bears on the classification.** The criterion is unchanged in every later
  plan revision (`536057ab`, `ca4c800e`, `63a8c400`). The correction lands in the evidence
  report and in the 09B follow-up's re-description, **not** in the plan. The recorded gap
  names the report's conflated claim; reading the plan's wording as its upstream cause is a
  judgment the corpus owner must make. That is why determinability here is proposed, not
  recorded.
- **Design vs implementation:** **design**, on the following reading. The code behavior is
  intentional — import the directory you are given, enforce trust at query time. The
  defective element is the plan's acceptance criterion, which asserts a property the design
  does not and should not have, and which is not testable without fixing the directory
  layout. The class is **unverifiable acceptance criterion**.
- **Determinability:** a reviewer with the plan and `rebuild.go`/`index.go` could identify
  that rebuild imports the whole directory while scope is enforced at query time. The
  reading does not depend on the later report. **Proposed strict.**
- **Residual ambiguity (recorded, not resolved):** the criterion can be read as "a rebuild
  from a workspace-local directory does not import another workspace," which is true. The
  corpus owner may therefore record this as partial rather than strict. This record
  proposes strict and states the basis, so the decision is explicit rather than implicit.
- **Authorship / contamination:** Tom Tran `<trantom512@gmail.com>`, 2026-11-08.
- **Disposition:** **Extracted.** Proposed strict determinability; not eligible and not
  counted until the corpus owner records it (decisions 2 and 3). `IDR-H-008` remains
  excluded as the answer-bearing follow-up; its permitted-reference list remains cleared.

### 3.4 `IDR-L-004` — review-governance design artifact

- **Artifact:** `docs/specs/REVIEW.md`
- **Revision:** `5fce2b6adcb0783ad87875550a91654a264ffcbf` (2026-09-30), the commit that
  creates the file and the **earliest** revision that exists.
  SHA-256 `356d83a467dc45b57305a61f297f46949d907850f44ba754a34fa2002d57de84`.
- **Why this revision:** `4223e10b` (2026-10-07, RSH-002) adds §4 "Invocation-Scoped Review
  Input"; `ba7736d4` (2026-10-07, RSH-003..005) adds §6 "Malformed Review Output". Both are
  fix commits and both contain the disclosure. The manifest's `IDR-E-005` points at
  `ba7736d4`, which is **not** a pre-fix revision; see §4.
- **Later evidence:** `docs/reports/review-hardening/RSH-001-review-seam-baseline.md`
  (read-only baseline of the pre-fix seam), `RSH-005-verification.md`,
  `docs/history/plans/PLAN-Review-Scope-And-Output-Hardening.md` (the remediation plan),
  the EV-003 approval-boundary diagnostic (2026-10-08), and the fix commits `4223e10b` and
  `ba7736d4`. The RSH-001 baseline records the pre-fix scope as "working-tree diff
  (`readDiff`) + declared-deliverable diff (`git.New(dir).DiffFiles(...)`)".
- **Permitted references (exist at `5fce2b6a`):** `docs/specs/QUALITY.md` (`0fa56459…`),
  `docs/specs/EXECUTION.md` (`d4651b68…`), `docs/specs/VALIDATION.md` (`ec4e3883…`),
  `internal/review/provider.go` (`6ebd729e…`), `internal/cli/cli.go` (`ca052c77…`),
  `internal/cli/run.go` (`73827daf…`), `internal/git/git.go` (`7e1f815a…`).
- **Defect (a) — review-scope contamination.** §4 says only "Git MUST remain the authority
  on what changed" and "When the working tree is unchanged, review MUST report 'no changes
  to review'". At the same revision, `cli.go:88` resolves `readDiff` to
  `git.New(dir).DiffAll(ctx, config.DirName, planflow.ReportsDir)` — the whole working-tree
  diff. Unrelated pre-existing uncommitted files therefore enter every task's review. The
  specification defines no task attribution and no fail-closed rule. The later §4
  (invocation-scoped input, fail-closed) is the correction. Class: **scope violation**.
- **Defect (b) — missing malformed-output boundary.** §2 requires structured findings and
  §3 says a finding blocks when its severity is named in `quality.fail_on`, but the
  specification says nothing about a response that cannot be parsed. At the same revision,
  `internal/review/provider.go:59` `parseReport` returns an error for the **whole report**
  when one finding has an invalid severity, so a genuine `HIGH` finding in the same
  response is discarded and the outcome becomes an unclassifiable human boundary. The
  later §6 (per-finding parsing, bounded retry, never a PASS) is the correction. Class:
  **missing failure mode**.
- **Ancestry:** `5fce2b6a` is an ancestor of `4223e10b`, which is an ancestor of `ba7736d4`;
  all are on `main`.
- **Leakage:** the artifact at `5fce2b6a` contains neither correction. The words
  "invocation-scoped", "malformed", and the fail-closed rule do not appear.
- **Design vs implementation:** **design**. Both defects are in a normative specification.
  The implementations (`DiffAll`, `parseReport`) implement the specification as written;
  the specification omits the requirement. The remediation plan calls these "two defects
  in SOP's self-review seam that the EV-003 diagnostic proved."
- **Determinability:** both defects are readable from the specification plus the permitted
  paths at the same revision, without RSH-001 or the remediation plan. **Proposed strict.**
  This plan yields **two** distinct defects, not one.
- **Authorship / contamination:** Tom Tran `<trantom512@gmail.com>`, 2026-09-30. The
  specification predates the defect discovery by at least seven days.
- **Disposition:** **Extracted.** Proposed strict determinability for two defects; not
  eligible and not counted until the corpus owner records it (decisions 2 and 3).
  `IDR-E-005` remains excluded as recorded; this record supplies the correct pre-defect
  revision and does not retroactively change the exclusion.

### 3.5 `IDR-L-005` — post-Wave-3 production readiness

- **Artifact examined:** `docs/history/plans/PLAN-Post-Wave3-Production-Readiness.md`
  (archived at `b8221e09`).
- **Finding:** this is a **backlog of production-readiness tasks**, not a design artifact
  whose defect later appeared. Its tasks name implementation and integration gaps —
  Windows process-tree termination, a Julia runtime, an opt-in adapter dogfood, an
  operator console. Each gap is defined by the plan itself; there is no earlier design
  document that specified the wrong behavior and was later corrected.
- **Later evidence:** the plan is itself the gap record. No fix commit identifies a single
  pre-fix design artifact.
- **Determinability:** not determinable as a single-artifact design defect. Selecting one
  task and its target file would produce an **implementation-only** candidate.
- **Disposition:** **No artifact** — bounded negative. `IDR-E-011` remains excluded; this
  record confirms the "no single artifact" reason rather than replacing it.

### 3.6 `IDR-L-006` — end-to-end reliability

- **Artifact examined:** `docs/history/plans/PLAN-SOP-End-to-End-Reliability.md` (archived
  at `2e6a526d`).
- **Finding:** this is an **assessment and backlog plan**. It measures a disposable
  end-to-end run and produces a prioritized backlog. Its recorded gaps are integration
  availability (the decision-adapter module is not imported; the `decision:` section is
  absent) and missing live evidence, not a design defect in a prior specification.
- **Later evidence:** the plan's own status and the archived backlog. No fix commit
  identifies a single pre-fix design artifact.
- **Determinability:** not determinable as a single-artifact design defect. The gaps are
  integration-level and, at most, implementation-only.
- **Disposition:** **No artifact** — bounded negative. `IDR-E-012` remains unverified; this
  record confirms the "parent not identified" state rather than resolving it.

## 4. Correction to an existing record (`IDR-E-005`)

`CANDIDATE-VERIFICATION.md` records `IDR-E-005` as
`docs/specs/REVIEW.md` @ `ba7736d4`, excluded because "the pre-fix spec already required
invocation-scoped input and a deterministic malformed-output boundary, so the defect was in
code, not the design document."

That statement is true **of `ba7736d4` only because `ba7736d4` added those sections**. The
commit body of `ba7736d4` states: "docs/specs/REVIEW.md documents the scoped review and the
malformed-output boundary." `4223e10b` added the invocation-scoped requirement and
`ba7736d4` added the malformed-output boundary. At the earliest revision `5fce2b6a`,
neither requirement exists.

The exclusion is preserved as recorded: `IDR-E-005` still points at a post-fix revision and
still must not be used as a corpus input. The correction is that the repository **does**
contain an uncontaminated pre-defect revision, `5fce2b6a`, and that revision is the basis
for `IDR-L-004`. No historical count, exclusion, or prior decision is rewritten.

## 5. Counts after extraction

Nothing in this section is eligible or adjudicated. The corpus owner must record each
classification before any candidate enters a floor.

| Measure                                | As recorded in the manifest | After this extraction (proposed, not counted)                         |
| -------------------------------------- | --------------------------- | --------------------------------------------------------------------- |
| Manifest entries                       | 23                          | 23 (unchanged)                                                        |
| Strict proposed determinable           | 0                           | 2 plans / 3 defects (`IDR-L-003`, `IDR-L-004`)                        |
| Partially determinable                 | 10                          | 10 (unchanged)                                                        |
| Verified-defect holds                  | 2                           | 2 (unchanged; `IDR-L-001` is a lead-level hold, not a manifest entry) |
| Unverified holds                       | 3                           | 3 (unchanged)                                                         |
| Excluded                               | 8                           | 8 (unchanged); `IDR-L-002` is a lead-level exclusion                  |
| Leads with no single artifact          | 2                           | 2 (`IDR-L-005`, `IDR-L-006`)                                          |
| Independently adjudicated determinable | 0                           | **0**                                                                 |

The three proposed defects are:

1. `IDR-L-003` — `PLAN-09…` @ `4fb3b8a3`: `RETRIEVE-003` rebuild-scope acceptance criterion
   is unverifiable because import scope is directory-scoped and trust is query-scoped.
2. `IDR-L-004` (defect a) — `REVIEW.md` @ `5fce2b6a`: no invocation-scoped review input and
   no fail-closed rule; the entire working tree is reviewed.
3. `IDR-L-004` (defect b) — `REVIEW.md` @ `5fce2b6a`: no bounded malformed-output
   boundary; one invalid severity discards the whole report.

The per-plan defect counts are 1 (`IDR-L-003`) and 2 (`IDR-L-004`); see
[EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3, "per-plan
count".

## 6. What this record does not establish

- No candidate is eligible, frozen, or counted toward a floor.
- No defect is independently adjudicated. The adjudicator-confirmed count remains **0**.
- The strict classifications are proposals. The corpus owner records the final judgment,
  and the proposed eligibility amendment is not approved.
- The extraction does not show that a reviewer would find these defects in a run. That is
  the experiment's question.
- No seeded or no-known-defect case is constructed, and no answer key exists.

## 7. Validation performed

- All four pinned commits resolve on `main`, and ancestry was checked with
  `git merge-base --is-ancestor`.
- Every artifact SHA-256 was recomputed from the pinned blob and matches
  [lead-extraction.json](lead-extraction.json).
- Every permitted reference was checked with `git cat-file -e <commit>:<path>`; the LLM09
  report was rejected because it does not exist at `4fb3b8a3`.
- No permitted reference is also listed as later evidence, and no later fix commit is a
  permitted reference.
- The manifest remains valid JSON with 23 entries and the same state and inclusion counts.

## 8. Additional leads investigated after PR #587

PR #587 resolved the six original leads. This section records two additional leads found by
tracing commits that corrected a **normative specification**. Both are extracted here;
neither is eligible, counted, or adjudicated, and the manifest is still 23 entries.

### 8.1 `IDR-L-007` — plan archive slot keyed by a non-unique plan id

- **Artifact:** `docs/specs/PLAN-HISTORICALIZATION.md`
- **Revision:** `fcf53888109500b4a12f330b88f1e19ffcad3c3b` (2026-10-06), the commit that adds
  the spec. SHA-256 `a9d5f644487db1b3be45c531231ce1b6b7174647639471ef9c4b14f7e4a2458b`.
  The content is byte-identical at `f2cdffb0d013a4382b71f0c830bfe48fbd7a2210`, the parent of
  the fix, so the pin is unambiguous.
- **Why this revision:** the spec has two revisions. The second, `b14fcabe` (2026-11-09),
  is the fix and rewrites the slot layout, so it is not a pre-defect artifact.
- **Later evidence:** `b14fcabeccfbd0634989cbbfef8e650d40815854`, merged as PR #322
  (merge `49a8f589`), together with that fix's updates to `docs/specs/RECOVERY.md`, the CLI
  reference, and the two historicalization skills.
- **Permitted references (exist at the pin):** `internal/planflow/planflow.go`
  (`ae690f2e…`), `internal/planflow/historicalize.go` (`a5faf3d7…`),
  `docs/specs/EXECUTION.md` (`2d3f0517…`), `docs/specs/RECOVERY.md` (`4e6d6dde…`).
- **What the artifact says:** §1 "reuses the same archive storage
  (`.agent-sdlc/archive/<plan-id>/`)"; §5.5 writes the record under that path; §6 makes a
  re-run against a historicalized plan a deterministic no-op that MUST NOT duplicate
  history. `RECOVERY.md` §7 repeats the same `archive/<plan-id>/` path for handoff and
  supersession.
- **What the permitted paths show at the pin:** `planID(rel)` is a slug of the file
  **basename** (`internal/planflow/planflow.go:1826`), and `archiveID(id)` returns that id
  unchanged (`internal/planflow/historicalize.go:658`). Two plans whose basenames agree —
  for example `docs/plans/PLAN-X.md` and `docs/history/plans/PLAN-X.md` — and any
  re-activation of a plan therefore share one archive directory.
- **Defect:** one archive slot is keyed to a plan id that is not unique across activations
  or source directories. A later activation at the same id overwrites the earlier
  activation's lifecycle, task snapshot, and disposition; a staged-release check that reads
  "active + archived" can also skip archiving and clear the new activation's tasks. The fix
  gives the first activation `archive/<id>/` and each later one `archive/<id>.<n>/`.
- **Ancestry:** `fcf53888` is an ancestor of `b14fcabe`, which is an ancestor of `main`.
- **Leakage:** the pinned artifact contains no "slot", "collision", "re-activation", or
  `<id>.<n>` language.
- **Design vs implementation:** **design**. The specification defines the archive identity
  and the idempotency rule; the code implements them as written. The missing dimension is
  the activation identity.
- **Determinability:** a reviewer can derive the collision from the archive path, the plan-id
  derivation, and the idempotency rule without the fix. **Proposed strict.** Class:
  **missing identity dimension** (an idempotency rule that is unverifiable under
  re-activation).
- **Authorship / contamination:** Tom Tran `<trantom512@gmail.com>`, 2026-10-06. The spec
  predates the fix by about a month.
- **Disposition:** **Extracted.** Proposed strict; not eligible and not counted until the
  corpus owner records it (decisions 2 and 3).

### 8.2 `IDR-L-008` — RAG evidence hash covers the document, not the returned chunk

- **Artifact:** `docs/specs/RAG-RETRIEVAL-CONTRACT.md`
- **Revision:** `c6ef5f5651cb822d501cf8b095c9f6dd54f1755a` (2026-11-09), the parent of the
  fix. SHA-256 `20ab7c62867de9868b0b40bda056882b48c885b811cfc2b9e07f7455fcb05080`.
- **Why this revision:** the contract has two revisions; `e15b8b09` is the fix and adds the
  field and error code, so its parent is the last pre-defect revision.
- **Later evidence:** `e15b8b0942e20bd89e12a052fe1bf21932182dc0`, merged as PR #330
  (merge `97d08c1a`), with the LLM05 gap register and the two proposals it implements.
- **Permitted references (exist at the pin):** `internal/ragclient/ragclient.go`
  (`0c60c98c…`), `docs/specs/RAG-SECURITY-THREAT-MODEL.md` (`fdf3b7b9…`). The
  `migrations/002_ingestion_metadata.sql` path named by the contract does not exist at the
  pin and is **not** a permitted reference.
- **What the artifact says:** the response field `document_hash` is the **projected
  ingestion fingerprint** of the whole source document; §5 says "No new hash is invented";
  a NULL fingerprint is carried as `""` and the item is treated as lower-confidence.
- **What the permitted paths show at the pin:** the returned item carries `text` (a chunk),
  but the only hash is over the whole document, and `ragclient.go` does not recompute any
  hash over the returned text. SOP therefore cannot verify that the chunk it received is
  the chunk that was ingested.
- **Defect:** the evidence-integrity field is document-scoped while the evidence unit is a
  chunk, so the returned evidence is not checkable. The fix adds an optional `chunk_hash`
  over the exact `text` bytes, recomputes it, rejects a mismatch, and states that
  `document_hash` is never reported as verified.
- **Ancestry:** `c6ef5f56` is an ancestor of `e15b8b09`, which is an ancestor of `main`.
- **Leakage:** the pinned artifact contains no `chunk_hash`, `EVIDENCE_HASH`, or per-row
  digest language.
- **Design vs implementation:** **design**, but the artifact explicitly chose
  whole-document projection and a lower-confidence treatment, so whether the gap is a defect
  or a documented limitation is a judgment. Class: **missing integrity control**.
- **Determinability:** a reviewer can see that a whole-document hash cannot verify a chunk.
  Whether that is a defect depends on the evidence-integrity goal. **Proposed partial**, not
  strict.
- **Authorship / contamination:** Tom Tran `<trantom512@gmail.com>`, 2026-11-09.
- **Disposition:** **Extracted.** Proposed partial; not eligible and not counted.

### 8.3 Counts after the additional leads

| Measure | Manifest recorded | PR #587 proposals | This section | Combined, uncounted |
| --- | --- | --- | --- | --- |
| Strict proposed plans | 0 | 2 | 1 | 3 |
| Strict proposed defects | 0 | 3 | 1 | 4 |
| Partial proposed plans | 10 | 0 | 1 | 11 |
| Partial proposed defects | 10 | 0 | 1 | 11 |
| Independently adjudicated | 0 | 0 | 0 | **0** |

The three proposed-strict plans are `IDR-L-003` (1 defect), `IDR-L-004` (2 defects), and
`IDR-L-007` (1 defect). The eleven proposed-partial plans are the ten manifest partials plus
`IDR-L-008`. Nothing in this table is a corpus-owner determination, and no lead is counted
toward a floor.

### 8.4 Validation performed for this section

- `fcf53888`, `c6ef5f56`, `b14fcabe`, and `e15b8b09` all resolve on `main`; ancestry was
  checked with `git merge-base --is-ancestor`.
- Every artifact and permitted-reference SHA-256 was recomputed from the pinned blob and
  matches [lead-extraction.json](lead-extraction.json).
- The `migrations/002_ingestion_metadata.sql` path named by the RAG retrieval contract was checked and
  rejected because it does not exist at the pin.
- No permitted reference is later evidence, and the pinned artifacts contain none of the
  fix vocabulary.
