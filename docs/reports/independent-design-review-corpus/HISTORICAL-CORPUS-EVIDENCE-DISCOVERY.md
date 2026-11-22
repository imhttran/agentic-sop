# Historical Corpus Evidence Discovery — Independent Design Review Corpus

**Type:** Investigation record (non-normative, read-only). It searches existing repository
evidence for historical plans and independently verifiable design defects. It is **not**
candidate promotion, adjudication, a corpus freeze, or an experiment result.
**Baseline:** `origin/main` @ `2e16416e712dbd02dfc26ece0faddaea6962b446` (merge of PR #606),
2026-11-21.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3.
**Eligibility rule:** [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md)
(`conservative`).
**Canonical records (unchanged by this report):** [candidates.json](candidates.json),
[CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md), [READINESS.md](READINESS.md),
[HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md).

The classifications below are **investigative proposals**. They do not change any canonical
disposition, and no candidate in this report is eligible, counted, or adjudicated.

## 1. Scope and baseline

### 1.1 Rules in force (re-read, not assumed)

- Registered floor: 15 determinable known defects across at least 15 plans with known
  defects (protocol §5.3); the historical target is 12–15.
- Decision 3 (`conservative`): partial and hindsight-only defects count toward **neither**
  floor; the recall denominator is the frozen determinable set; seeded cases are
  **additional** and count toward neither historical floor. Seeded / control cases
  constructed: **0 / 0**.
- Decision 3 §1 itself says it does not promote any candidate or establish determinability.

### 1.2 Baseline counts (re-derived from the files)

| Measure | Value | Source |
| --- | --- | --- |
| Manifest entries | 23 (8 `IDR-H`, 12 `IDR-E`, 3 `IDR-X`) | `candidates.json` |
| Manifest strict | 0 | CANDIDATE-VERIFICATION §5 |
| Manifest partial | 10 (`H-003`, `H-004`, `H-006`, `E-001`–`E-004`, `E-007`–`E-009`) | `candidates.json` |
| Leads extracted outside the manifest | 8 (`IDR-L-001`–`L-008`) | HISTORICAL-LEAD-INVESTIGATION §2, §8 |
| Proposed strict | **3 plans / 4 defects** (`L-003` ×1, `L-004` ×2, `L-007` ×1) | HISTORICAL-LEAD-INVESTIGATION §8.3; Decision 3 §4 |
| Proposed partial | **11 plans / 11 defects** (10 manifest + `L-008`) | same |
| Independently adjudicated | **0** | Decision 3 §4 |
| Strict shortfall (provisional) | **12 plans / 11 defects** | Decision 3 §4 |

Strict-proposed (`L-003`, `L-004`, `L-007`), partial, evidence-blocked/held (`H-001`,
`H-005`, `L-001`, `E-006`, `E-010`, `E-012`) and excluded (`H-002`, `H-007`, `H-008`,
`E-005`, `E-011`, `L-002`, `L-005`, `L-006`, `X-001`–`X-003`) leads are **not re-investigated
here**; this report only adds leads they do not cover.

### 1.3 Preflight

- `origin/main` fetched; this report was written in an isolated worktree
  (`docs/egm-historical-evidence-discovery`) cut from `2e16416e`.
- Existing worktrees preserved and not entered: `egm-decision-6`,
  `egm-decision-6-approval`, `egm-decision-6-clarification`, and `agentic-sop-zed-skills`.
  Nothing in the Decision 6 governance work was read or modified.
- The primary checkout's uncommitted `.agent-sdlc/security-baseline.json` change and the
  ignored `.agent-sdlc/` tree were left untouched (read-only `ls`/`python` inspection of
  `.agent-sdlc/archive` only).
- Prior discovery work checked to avoid duplication: `HISTORICAL-LEAD-INVESTIGATION.md`
  and `lead-extraction.json` (leads `L-001`–`L-008`). No earlier report covers the leads
  below; `PLAN-STATUS-AUDIT-2026-11-04.md` duplicates `IDR-H-003` and is not new.

## 2. Search methodology

1. **Unevaluated plans.** Listed `docs/plans`, `docs/history/plans`, and `docs/reports`,
   and diffed their names against `candidates.json` and `lead-extraction.json`.
2. **Commit-message search.** `git log origin/main --no-merges` over `docs/` and
   `docs/specs`/`docs/architecture` for fix, correction, gap, contradiction and
   stale-claim wording, to find commits that repair a design or spec statement.
3. **Pin test.** For each lead, resolve the parent of the fixing commit (or the commit that
   first adds the plan), and ask whether a design artifact existed there that a reviewer
   could read without the fix.
4. **Discoverability test.** Read the artifact and only repository paths that exist at the
   pin; do not use the later report, fix, or PR text as proof of discoverability (a later
   fix alone is not proof). A fix commit message is used only to identify the defect.
5. **Independent verification source.** Prefer a deterministic check against code or tests
   at the pin (`git show <pin>:<path>`, `git grep <pin>`). A model-authored finding is not
   independent verification.
6. **Archive evidence.** Listed `.agent-sdlc/archive` read-only to decide whether it can
   supply defect evidence (it cannot; see §3.2).

Depth is uneven by design: only `DISC-001` was verified against code at a pin. The others
were triaged from commit messages, diffstats and the artifact text, and each limitation is
stated in §4.

## 3. Evidence sources inspected

### 3.1 Repository sources

| Source | Used for |
| --- | --- |
| `DECISION-RECORD-003-ELIGIBILITY.md`, `CANDIDATE-VERIFICATION.md`, `candidates.json`, `HISTORICAL-LEAD-INVESTIGATION.md` | Baseline rules and counts |
| `docs/plans/` (108 entries), `docs/history/plans/` (47 entries) | Unevaluated-plan survey |
| `docs/plans/PLAN-0{1,5,6,7}-…`, `PLAN-10-…` at `4fb3b8a3` | OWASP plan pins |
| `docs/reports/owasp-llm/` (`llm05`, `llm06`, `llm07`, `llm10`, `gap-register-20261109-baseline.md`) | Later evidence for the OWASP plans |
| `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` at `ade6864a`; code at `ade6864a` | `DISC-001` |
| Commits `342c1593`, `ea01b10e`, `a6fdf820`, `e97f3e8a`, `d034b414`, `0c17f54f`, `8793fbd3`, `0d37762a`, `ea6efb07`, `93388f9b`, `3cd134c8`, `8a3f4f99`, `f32cfe7e` | Fix-commit triage |
| `docs/reports/PLAN-compiler-reconciliation-commit-review.md`, `PLAN-STATUS-AUDIT-2026-11-04.md` | Review and audit reports |

### 3.2 Archived SOP lifecycle evidence

`.agent-sdlc/archive/` holds 41 plan directories (998 files: `lifecycle.json`, `plan.json`,
`plan.meta.json`, `tasks.json`, and similar). The sampled `lifecycle.json` files record
`COMPLETE`. The directory is git-ignored and local to one checkout, so it cannot be an
independent, reproducible verification source for any other reader, and what was sampled
holds task and lifecycle state, not defect or discoverability evidence. **No candidate is
drawn from it**, and nothing in it was changed. Whether any run under `.agent-sdlc/runs/`
holds usable pre-fix evidence was not examined.

### 3.3 Not searched

GitHub PR review threads and issue comments were not searched in this pass (only git
history and repository files). This is a coverage limit, not a negative result.

## 4. Candidate-by-candidate findings

IDs use the `DISC-` prefix so they cannot be mistaken for manifest (`IDR-*`) entries.
Classification vocabulary: `STRICT_ELIGIBLE_PROPOSED`, `PARTIAL_ONLY`, `EVIDENCE_BLOCKED`,
`EXCLUDED`, `NEEDS_OPERATOR_REVIEW`.

### DISC-001 — ACP execution exclusion gap record — **NEEDS_OPERATOR_REVIEW**

- **Artifact:** `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` @
  `ade6864a78fc743c52d37017600790ed1fd40c86` (parent of `342c1593`; PR #571 merge). SHA-256 of the blob:
  `990ecfc7aeb1fde38acc7fb143e5d2b56b63bcdadd5532ae6ed3d8e53fafa461`.
- **Revision history:** `79acbfc1` (2026-11-02, adds the doc with `PLAN-SOP-ACP-ZED.md`) →
  `84bfd6ae`, `75e3df0f`, `3ad9d447` → `ade6864a` (EGM review fixes) → `342c1593` (the
  correction) → `ea01b10e` (further reconciliation). Ancestry of `ade6864a` on `origin/main`
  was checked.
- **Defects the correction records (three):**
  1. The doc names a `sop drive` command (lines 7, 51, 97 at the pin). **Verified by me:**
     no `"drive"` / `sop drive` command exists in `internal/cli` or `cmd` at the pin; graph
     mode is `runGraph` / `driveGraph` in `internal/cli/drive.go`.
  2. The doc claims `TestACPPromptConcurrentStartsCreateOneWorkItem` "proves **ACP-vs-MCP**
     and **ACP-vs-ACP** start exclusion" (line 39). **Verified by me:** the test's own
     comment says "Two ACP sessions submit the same prompt"; it races two ACP submissions
     through `acpSubmission`, with no MCP path.
  3. The doc says a mid-run `sop run` is detected through `midRunTasks`. **Partly
     verified:** `midRunTasks` (`internal/cli/mcp_start_work.go:571`) consults only persisted
     task rows whose status is in `workMidRunStatuses`, which supports the correction's
     point that detection depends on task rows. The consequence the fix states ("a fresh
     `sop run` is invisible for the whole task") rests on the fix commit's text and was
     **not independently re-derived here**.
- **Original version and timestamp:** the pin is a *later* revision, not the original
  (`79acbfc1`, 2026-11-02). Whether defects 1–3 were present at the original revision was
  **not checked**.
- **Evidence the defect existed before the fix:** defects 1 and 2 are code-checkable at the
  pin, as above.
- **Discoverable from permitted material:** yes for 1 and 2, using only the doc and
  `internal/cli/{drive.go,acp_test.go,mcp_start_work.go}` at `ade6864a` (blob prefixes
  `2076fb5d…`, `4906afaa…`, `b370a0df…`).
- **Independent verification source:** the code at the pin. The *discovery* was by a
  model-run `sop prompt --capability review` (named in the `342c1593` message), which is not
  an independent source; an adjudicator must re-run the code checks.
- **Why NEEDS_OPERATOR_REVIEW, not STRICT_ELIGIBLE_PROPOSED:**
  - The artifact is a **gap record** (a report about code), not a plan or design. Whether it
    is a valid "plan" input to the registered floor is a corpus-owner ruling this report
    cannot make.
  - The defects are **factual-accuracy** defects against code, not design defects; the
    protocol's design-review scope would have to cover them.
  - The pin was already EGM-reviewed (reports at `docs/plans/ACP-EXECUTION-EXCLUSION-GAP-REVIEW/`
    exist). Those reports must **not** be reviewer-visible input; the three defects survived
    that review, which is a fact about the pin, not a verdict on the review.
  - Relation to `IDR-H-001` (held, same subject, no uncontaminated pre-defect revision for
    `PLAN-SOP-ACP-ZED.md`): this is a different artifact and revision, so it is not a
    duplicate, but it should be ruled on together with `H-001`.
- **If ruled eligible:** potentially **1 plan / 3 defects**. Not counted until the corpus
  owner records it and independent adjudication occurs.

### DISC-002 — `PLAN-05` LLM05 data and model poisoning — **PARTIAL_ONLY**

- **Artifact:** `docs/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md` @ `4fb3b8a3`
  (added with Plans 06–10 on 2026-11-08; 202 lines; blob SHA-256 prefix `98ebdd3af03f`).
- **Finding:** the plan **states** its own baseline gaps (self-declared `outcome.status`
  ingested as verified; changed `report.json` keeps an obsolete digest; unchecked declared
  id; lenient `counts.json`). Those defects are in code, are disclosed by the artifact, and
  are therefore self-referential, not discoverable defects *of the plan*. The remaining
  later findings are `GAP-4` (no per-row index digest; fixed in `e15b8b09` with the
  `IDR-L-008` change) and `GAP-5` (the chunk-hash gap, a duplicate of `IDR-L-008`). `GAP-4`
  is partly derivable from POISON-001's "revision/content digest" wording against
  `internal/repoindex`, but the plan does not require per-row verification.
- **Why partial:** hindsight-only for `GAP-4`; `GAP-5` is already `IDR-L-008` (partial).
- **Missing evidence:** no independent artifact shows `GAP-4` was a design defect rather
  than an enhancement (the repository's own `PROPOSAL-REPOINDEX-ROW-DIGESTS.md` calls it a
  "schema decision").
- **Contribution:** 0 strict. Adds no new partial count (the usable partial is `L-008`).

### DISC-003 — `PLAN-06`, `PLAN-07`, `PLAN-10` @ `4fb3b8a3` — **EXCLUDED**

- Blob SHA-256 prefixes `527acbb39066` (06), `0474551ed967` (07), `25e9ba72c1b9` (10).
- Each plan is a generic audit-and-repair instruction ("Reproduce and repair limit
  bypasses", "report IMPLEMENTED/ENABLED/VERIFIED separately"). The later defects are
  **code** defects found by executing the plan: output buffered before truncation
  (`llm06 §2.1`), model tools able to write SOP state (`llm07` D1), a stale VERIFIED verdict
  after a failed run (D2), terminal control sequences and Slack mrkdwn in model output
  (`llm10` GAP-01/02). None is a defect in the plan text, and none is discoverable by
  reviewing the plan.
- **Rationale:** the defect is not in the artifact under review.

### DISC-004 — scanner binary pin, hash-then-exec race — **EXCLUDED**

- Fix `a6fdf820` (2026-11-11). The pre-fix `internal/security/pinned.go` carries its own
  `ponytail:` comment stating that "a writer racing between the two is not excluded;
  executing an opened file descriptor would close that window". The pre-fix spec
  (`SECURITY-SCANNER-ADDON.md` at `a6fdf820^`) says nothing about the race.
- **Rationale:** an implementation limitation the code already discloses, with no design
  artifact that is wrong.

### DISC-005 — `repoindex.Identity.Digest` coverage — **EXCLUDED**

- Fix `e97f3e8a`. The pre-fix artifact is a Go doc comment ("a stable digest over the
  indexed files"), which accurately describes what the code hashes. No design document
  asserts otherwise at the pin. Implementation-only; overlaps `DISC-002` `GAP-4`.

### DISC-006 — shared credential redaction — **EVIDENCE_BLOCKED**

- Fix `d034b414` (an ACP key rule ran before its bearer rule, leaving the token visible).
  The scoping proposal's first commit is `9e4e4d36`, written *after* the divergence was
  seen (PR #221). **No pre-defect design artifact was identified.** Only the commit message
  and proposal history were read; the ACP diagnostic code at the pin was not traced.

### DISC-007 — opt-in `run_command` OS isolation — **EXCLUDED**

- Proposal commit `70ea0664`. `AGENT-PROVIDER.md` at the parent specifies a "bounded
  allow-listed `run_command`"; the absence of OS isolation is recorded by the repository as
  a documented limitation ("normal agent execution is policy-only", gap register item 4),
  and the proposal is an enhancement. Hindsight-only threat-model judgement; not a defect
  at the pin.

### DISC-008 — stale-status corrections (`0c17f54f`, `8793fbd3`, `0d37762a`) — **EXCLUDED**

- Each commit repairs statements that "described the pre-#441/#444 state" or were "true
  before Stage 2 merged". They were accurate when written and became stale after later
  merges. A claim that went stale is not a defect in the pre-fix artifact.

### DISC-009 — behavior fixes that also edit a spec (`ea6efb07`, `93388f9b`, `3cd134c8`, `8a3f4f99`) — **EXCLUDED**

- Early-JEV routing, verify-first JEV skip, `NoChangesProduced` classification, and locality
  resolution. Each was found by running the system (a real-world bench, a failed
  `sop run`, a recorded limitation closed by the plan). The spec edit records the fix; the
  pre-fix spec was not shown to contradict the code. **Limitation:** triaged from commit
  messages and diffstats; the parent-revision specs were not read in full.

### DISC-010 — TEL-009 telemetry reporting corrections (`f32cfe7e`) — **EVIDENCE_BLOCKED**

- The corrections follow an audit of 219 runs under `.agent-sdlc/runs/` (git-ignored). The
  verification source is local run data a second reader cannot reproduce. The earlier plan
  (`PLAN-Decision-Telemetry-Extension.md` @ `3db62cfc`) names the metrics, but its text was
  only searched, not read for a definition conflict.

### DISC-011 — compiler/reconciliation pre-commit review — **EXCLUDED**

- `docs/reports/PLAN-compiler-reconciliation-commit-review.md` reports no blocking
  findings; its one HIGH finding was refuted. It records no defect.

### DISC-012 — Semgrep `PARTIAL` coverage from ignored artifacts (PR #524) — **EXCLUDED**

- The symptom is an environment-dependent verdict (long-lived checkout vs clean checkout),
  found empirically. Implementation behavior, not a defect in a design artifact. Read from
  the decision packet §1 only.

## 5. Eligibility assessment

| Class | Count | Leads |
| --- | --- | --- |
| `STRICT_ELIGIBLE_PROPOSED` | **0** | none new |
| `NEEDS_OPERATOR_REVIEW` | 1 | `DISC-001` |
| `PARTIAL_ONLY` | 1 | `DISC-002` |
| `EVIDENCE_BLOCKED` | 2 | `DISC-006`, `DISC-010` |
| `EXCLUDED` | 8 | `DISC-003` (3 plans), `004`, `005`, `007`, `008`, `009`, `011`, `012` |
| **Leads investigated** | **12** | (`DISC-003`, `009` each group several artifacts) |

No new lead is a proposed-strict plan. The only new lead that could reach the floors,
`DISC-001`, depends on rulings about artifact kind and defect kind that belong to the corpus
owner. No lead here is verified or independently adjudicated.

## 6. Evidence gaps

- **No further design-artifact defects with a pre-defect revision and a non-model
  verification source were found.** The pattern across the OWASP plans and the fix commits
  is that defects live in **code** found by execution, not in the plan or spec text.
- **`DISC-001` open points:** revision choice (original `79acbfc1` vs `ade6864a`);
  whether the defects existed at the original revision; whether a "gap record" is a
  valid input; defect 3 not re-derived; discovery was model-assisted.
- **Not searched:** GitHub PR review threads, issues, and CI logs; `.agent-sdlc/runs/`.
- **Depth:** `DISC-002`–`DISC-012` are commit-message and artifact-text triage, not
  code-verified.
- **Time-ordering:** every candidate shares one author and one Git identity
  (Tom Tran); the manifest already records that authorship cannot be proved from Git
  identity.

## 7. Updated provisional floor analysis

All figures are **provisional arithmetic**, not verified eligibility. Potential additions
are kept apart from verified and independently adjudicated members (**0**).

| Measure | Plans | Defects |
| --- | --- | --- |
| Registered floor | 15 | 15 |
| Proposed strict (baseline, unchanged) | 3 | 4 |
| Verified / independently adjudicated | **0** | **0** |
| Potential addition if `DISC-001` is ruled eligible | +1 | +3 |
| Proposed strict + potential `DISC-001` | 4 | 7 |
| Shortfall, baseline | 12 | 11 |
| Shortfall, if `DISC-001` were counted | 11 | 8 |
| New strict-proposed this pass | 0 | 0 |
| New leads investigated / potentially eligible / blocked / excluded | 12 / 1 / 2 / 8 | — |

Neither floor is satisfied, and finding leads does not satisfy one. The proposed-partial
set (11 plans / 11 defects) does not count under Decision 3. Even the potential additions
leave the strict plan floor 11 short and the defect floor 8 short. On this evidence the
historical corpus alone appears unlikely to reach 15/15 strict; what to do about that is a
corpus-owner decision and is not made here.

## 8. Recommended next verification work

(Proposals for the corpus owner; none is authorized or started by this report.)

1. Rule on `DISC-001` together with `IDR-H-001`: is a gap record a valid corpus input, and
   do factual-accuracy defects against code count as design defects?
2. If admitted, check defects 1–3 at the original revision `79acbfc1`, re-derive defect 3
   against `workMidRunStatuses`, and record permitted references excluding the
   `…-GAP-REVIEW/` reports and the later fix commits.
3. Search GitHub PR review threads and issues for traceable pre-fix design findings, which
   this pass did not cover.
4. Decide whether `.agent-sdlc/runs/` evidence may ever be a verification source; if not,
   `DISC-010` stays blocked.
5. Read the parent-revision specs for `DISC-009` in full before treating them as closed.

## 9. Risks and limitations

- Triage depth is uneven (see §4); a negative classification is based on the evidence read,
  not a proof that no defect exists.
- A fix commit message names a defect but does not prove it was discoverable earlier; this
  report did not treat it as such.
- `DISC-001`'s defects were first found by a model-assisted review; the check against code
  is the evidence, and it should be repeated by an adjudicator.
- The Decision 6 adjudicator/seeder designations are pending, so nothing here can be
  adjudicated yet.
- `candidates.json`, the dispositions, and every counted number outside this file are
  unchanged; a reader must not take this report's classes as canonical.

## 10. Non-authorization statement

This report authorizes nothing. It **does not**: promote any candidate; change any
eligibility decision or canonical disposition; modify the registered protocol, Decisions
1–6, or any threshold or cost limit; construct a seeded or control case; create or access a
sealed answer key; designate an adjudicator, seeder, or case checker; adjudicate; freeze the
corpus; run model inference; or execute the experiment. The corpus remains `NOT FROZEN`, the
experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.
