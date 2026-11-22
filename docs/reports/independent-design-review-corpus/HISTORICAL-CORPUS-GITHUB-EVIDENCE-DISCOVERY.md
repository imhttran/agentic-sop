# Historical Corpus Discovery — GitHub Review Evidence

**Type:** Investigation record (non-normative, read-only). Follow-up to
[HISTORICAL-CORPUS-EVIDENCE-DISCOVERY.md](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY.md) (PR #610).
It is **not** candidate promotion, adjudication, a corpus freeze, or an experiment result.
**Baseline:** `origin/main` @ `474979bae51e1aced9c47a41e57798137494d14a` (merge of PR #610), 2026-11-21.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3.
**Eligibility rule:** [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md) (`conservative`).
**Canonical records (unchanged):** [candidates.json](candidates.json), [CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md),
[HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md), [lead-extraction.json](lead-extraction.json).

Classifications are **investigative proposals**. They change no canonical disposition; nothing here is
eligible, counted, or adjudicated. Lead IDs use the `GH-` prefix so they cannot be confused with `IDR-*` or `DISC-*`.

## 1. Preflight

- `origin/main` fetched; PR #610 verified `MERGED` (2026-11-22T01:54:53Z) at `474979ba`.
- Registered protocol §5.3, Decision 3, and the #610 report were re-read, not assumed.
- Isolated worktree `docs/egm-github-evidence-discovery` cut from `474979ba`. No existing follow-up report or PR
  was found (`gh pr list` for open and merged PRs). A search for related open PRs found only #612, a CI change.
- Preserved and not entered: `.worktrees/egm-decision-6*` (Decision 6 governance), `agentic-sop-zed-skills`,
  `egm-historical-evidence-discovery`, the primary checkout, and the ignored `.agent-sdlc/` tree.
- Nothing in `candidates.json`, the decision records, or any threshold was edited.

## 2. Search methodology and coverage

Read-only `gh` / `gh api` against `imhttran/agentic-sop`, exported to a session scratchpad (not committed).

| Surface | Query | Inspected |
| --- | --- | --- |
| Pull requests | `gh pr list --state all --limit 700` (title, state, reviews, files) | **595** PRs (all) |
| Inline review comments | `GET /pulls/comments` (paginated) | **279** comments = **211** top-level threads + 68 replies, on **139** PRs |
| PR reviews | `reviews` field of every PR | **221** reviews, all state `COMMENTED`; **0** `CHANGES_REQUESTED`, **0** approvals |
| Conversation comments | `GET /issues/comments` (paginated) | **653** comments (573 bot, 80 owner); 493 are bot per-PR review summaries, 24 "no major issues" |
| Issues | `gh issue list --state all` | **17** issues (all), plus their **14** comments |

Method:

1. Partitioned all 211 top-level inline threads by path: **164** on code, **47** on `docs/` (40 PRs). Code-path threads are
   implementation-only by construction and were counted, not individually triaged.
2. Read every one of the 47 docs-path threads. Pinned each to its `original_commit_id` (the PR head the reviewer saw), so the
   artifact revision is exact and reviewer-visible. All pins were confirmed ancestors of `origin/main`.
3. For each design-artifact lead (`docs/plans`, `docs/specs`, `docs/reference`, `docs/guides`): read the passage at the pin
   and ran a deterministic check against the repository tree at the same pin (`git show`, `git grep`, `git ls-tree`).
4. Checked every lead's artifact path against `candidates.json`, `lead-extraction.json`, the investigation report, and the
   #610 report. No candidate artifact below appears in any of them.
5. Read the 17 issue bodies and comments for design-requirement defects.

### 2.1 Limitations of the evidence base (read before the findings)

- **No independent human review exists.** Every one of the 211 top-level threads is by an automated reviewer
  (`chatgpt-codex-connector[bot]` 210, `github-advanced-security[bot]` 1). The only human is the repository owner, and the owner's
  68 comments are replies. All 595 PRs and all 17 issues are owner-authored. "Prioritize human review evidence" yielded
  **no** human design-review findings.
- **The reviewer is a model.** A bot assertion is not independent verification. Every defect below was verified only by a
  deterministic check against repository state at the pin; where that check was not completed the lead is `EVIDENCE_BLOCKED`.
  The checks were done by a model in this session and **must be repeated by an adjudicator**.
- **Defects were caught pre-merge, in the PR-head revision.** The "fix" is a later commit on the same PR branch, not a merged
  correction of a merged document. Whether a PR-head draft counts as a "plan or spec" revision under §5.3 ("defect later
  appeared in merged fixes, erratum…") is a corpus-owner ruling (R-A, §7).
- **Single author.** Same limitation as the manifest: authorship and time-ordering cannot be proved from Git identity.
- Review bodies and conversation comments were sampled by classification (bot summaries vs owner), not read one by one.
  Code-path threads (164) were not triaged. A negative result is not proof of absence.

## 3. Findings

Verification depth is stated per lead: **V** = passage and code/tree check performed here; **T** = triaged from the thread and
passage only.

### 3.1 STRICT_ELIGIBLE_PROPOSED (3 leads, 3 plans / 3 defects, subject to R-A and R-B)

#### GH-001 — Ponytail complexity-audit cleanup plan (PR #562) — V

- **Artifact:** `docs/plans/PLAN-Ponytail-Complexity-Audit-Cleanup.md` @ `208b4e72203f437bb0c7431c5afcbc5bbf63d4c4`
  (the commit that adds it, i.e. the original revision; blob SHA-256 prefix `b2f0be314373`). Thread: PR #562, comment `4645807748`.
- **Defect:** PONY-001 lists `IsBlockedSelectable` for removal as dead code ("or wire the latter into the scheduler"), while the plan
  requires no behavior change. The plan's premise that it is unwired is false.
- **Existed at the pin:** `internal/scheduler/scheduler.go:211` calls `task.IsBlockedSelectable(byID)` at `208b4e72`, and also at the
  plan's own stated baseline `e28348d3` (checked with `git grep`). The scheduler uses it for blocked-task recovery.
- **Discoverable without hindsight:** yes. One `git grep` over the tree the plan names as its baseline. No fix commit needed.
- **Independent verification evidence:** the code at `208b4e72` / `e28348d3` (deterministic). The bot's wording is not relied on.
- **Design vs implementation:** design defect (wrong scope premise in the plan).
- **Duplicates:** none (no `IDR-*`, `DISC-*`, `L-*`).

#### GH-002 — OWASP LLM gap prompts, prompt 15 (PR #397) — V

- **Artifact:** `docs/plans/OWASP-LLM-GAP-PROMPTS.md` @ `46f190f210bfc2b3fcb01bc498d2533a8d7f01bf` (adds the file; blob prefix
  `a36d6fa3c8c0`). Thread: PR #397, comment `4532926068`.
- **Defect:** prompt 15 directs implementing before/after state-directory digest detection and marking it VERIFIED. The repository
  already records that exact approach as **operator-rejected NO-GO** (2026-11-09) with the plan archived `SUPERSEDED`.
- **Existed at the pin:** `docs/reports/owasp-llm/llm07-state-write-detection-phase1.md` is in the tree at `46f190f2` with
  `Status: … operator decision NO-GO`, `## 2. Decision: NO-GO`, and "archived with disposition `SUPERSEDED`". Prompt 15 step 1
  itself says to read that file.
- **Discoverable without hindsight:** yes. The contradicting document is named in the prompt and present at the pin.
- **Independent verification evidence:** the recorded decision document at the pin (deterministic).
- **Design vs implementation:** design defect (internal contradiction with a recorded decision).
- **Duplicates:** none. `IDR-H-007` is a different plan (`PLAN-08B`).

#### GH-003 — Semgrep ignored-coverage decision packet, relocation script (PR #522) — V

- **Artifact:** `docs/plans/PROPOSAL-SEMGREP-IGNORED-COVERAGE.md` @ `62e04e19a21c7ae441e1c8a7e7fa02924983932b` (adds the file;
  blob prefix `f8248519d458`). Thread: PR #522, comment `4603209734`.
- **Defect:** the advertised "reversible, deletes nothing" workaround moves files to `~/sop-scratch/$(date +%Y%m%d)/<relative path>`
  with `mv --`. A second run the same day, or from a second checkout with the same relative path, silently overwrites the earlier
  preserved file, so the workaround can lose local files.
- **Existed at the pin:** the script is in the document at the pin (checked); `mv` overwrites by default and `-n` / unique
  destination is not used.
- **Discoverable without hindsight:** yes, from the document text and `mv` semantics alone.
- **Independent verification evidence:** the document text plus standard `mv` behavior. Severity is lower (minor / changes approach).
- **Duplicates:** none. Distinct from `DISC-012` (PR #524 verdict behavior).

### 3.2 NEEDS_OPERATOR_REVIEW (3 leads)

| Lead | Artifact @ pin | Defect and check | Why not strict |
| --- | --- | --- | --- |
| **GH-004** (PRs #578, #590) — **V** (2 of up to 4 defects) | `docs/plans/PLAN-ACP-EXECUTION-LEASE-WIRING-OPERATOR-APPROVAL-PACKET.md` @ `c6307fd34d09…` (#578) and `022e7568555e…` (#590; later revision, blob prefixes `6759d860e0fa`, `8141f02df95e`) | **(a)** the packet says a plan edit forces a *model-based recompile*; `ErrUnsupportedReconcileSource` at both pins says SOP "will not call a model" and rejects unrecognized Markdown. Verified. **(b)** #590: the recommended `--list-changed` preview cannot parse a `## Steps` / `### N.` layout. Verified against the same diagnostic. **(c)** Option A's status edit changes the whole-source fingerprint. **(d)** `stage-1` stays BLOCKED after reconcile. (c), (d): triaged only. | Artifact is an operator decision packet (kind ruling R-C, like `DISC-001`). Two revisions; defects at the later revision depend on the earlier fix. Potential **1 plan / up to 4 defects**, 2 verified. |
| **GH-005** (PR #118) — **V** | `docs/reference/CLI.md` @ `d8aa908dd3a2…` (blob prefix `c8b4c2710fb7`) | Row says `sop planner ask TASK QUESTION`; `internal/cli/planner.go:162` at the pin says `usage: sop planner ask <task-id> --question TEXT`. Verified. (The second comment, a blank line in a table, is formatting and excluded.) | Reference-doc factual accuracy, not design (R-C). 1 plan / 1 defect if admitted. |
| **GH-006** (PR #189) — **V** | `docs/guides/PROMPTING.md` @ `cef3b8a91093…` (blob prefix `3ff1e8d54bb8`) | Guide says "every `cargo` command" is REQUIRES_APPROVAL; `TestAcceptanceCargoIndependentOfAgentTool` and the `acceptance_check` path in `internal/cli/preflight.go` exist at the pin and indicate a permitted exception. Test body not read. | Guide factual accuracy, not design (R-C); test behavior not re-derived. 1 plan / 1 defect if admitted. |

### 3.3 PARTIAL_ONLY (2 leads; count toward neither floor under Decision 3)

- **GH-007 — scanner pin policy decision packet (PR #465)** — V. `docs/plans/PROPOSAL-SCANNER-PIN-POLICY.md` @ `8b5b9b5e605e…`
  says baseline triage is in `llm03-baseline-triage.md` (PR #464); `git ls-tree` at the pin finds no such file. The file arrived
  in PR #464 (merged 2026-11-12T20:28:05Z, 32 seconds before #465). A cross-PR sequencing slip, not a design defect.
- **GH-008 — operator decision package amendment option (PR #572)** — T. `OPERATOR-DECISION-PACKAGE.md` @ `07b44191` describes the amendment as
  raising only the plan count, while `PROPOSED-PROTOCOL-AMENDMENT.md` §2 excludes partials from both floors. An internal
  inconsistency in an experiment-governance document, which this task does not re-open. Not re-derived.

### 3.4 EVIDENCE_BLOCKED (verification not completed; all code-checkable — T)

Code-checkable claims that could be adjudicated by a `git grep` at the pin, but were not completed here. They count as 0.

| Lead | Artifact @ pin | Claim (from thread) |
| --- | --- | --- |
| GH-009 (PR #323, 2 threads) | `PROPOSAL-RUN-COMMAND-ISOLATION.md` @ `70ea0664` | `sandbox.Wrap(h.root, root.path, …)` builds the sandbox around the primary root, not the selected root; replacing `argv` bypasses `noTestsMatched` |
| GH-010 (PR #446) | `PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md` @ `8495d54e` | Linux socket guarantee overstated vs `bubblewrapArgs` comments |
| GH-011 (PR #467) | `PROPOSAL-LLM03-D4-ENFORCEMENT.md` @ `68015e0a` | Disabling `sast` does not let Option B pass. Locus confirmed (`disabledCoverageGaps`, `policy.go:169`); full trace not done |
| GH-012 (PR #471) | `PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` @ `9aaa3a9c` | Same-location credential replacement stays suppressed (`StableFingerprint`) |
| GH-013 (PRs #484, #486) | `PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md` @ `de7161f2`, `54beaf27` | Clean filters / git helpers still run under the hardened flags. Reviewer reproduced on git 2.43.0; not reproduced here |
| GH-014 (PR #501) | `PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md` @ `e7a09f23` | OSV provisioning needed at stage 1. `.agent-sdlc/config.yaml` enables `osv-scanner` at the pin; policy trace not completed |

### 3.5 EXCLUDED

- **GH-015 — evidence reports and acceptance requests (27 threads, `docs/reports/owasp-llm/*`, `EGM-GOLDFISH-REVIEW-ASSESSMENT`,
  `independent-design-review-corpus`):** reports and decision records that restate or measure code, not plans or specs; same artifact-kind
  question as R-C, so they are excluded rather than assessed. The EGM / corpus threads (#564, #565, #567, #569, #587, #606) are governance
  drafts about this very experiment and are not used as corpus material.
- **GH-016 — `docs/testing/PLANNER-TWO-QUESTION-LIVE.md` (PR #128):** a live-test runbook, no design artifact.
- **GH-017 — 164 code-path threads:** implementation defects; the artifact under review is code.
- **GH-018 — issues (17):** `#140`, `#273`, `#158` describe tool behavior found during operation (implementation, later fixed in #141/#144/#277); `#40`, `#52`, `#54`–`#56` are scanner follow-ups tied to the
  subjects of `IDR-H-006` and `DISC-012` (not re-investigated); `#89`–`#93`, `#138`, `#334`, `#553` are test, process, or bot-finding trackers; `#575` is the open
  follow-up for the execution-exclusion gap, tied to `DISC-001` and not new. No issue documents a missing design requirement in a
  plan or spec that was pinned before the discovery.

## 4. Duplicate checks

No `GH-` artifact path appears in `candidates.json`, `lead-extraction.json`, `HISTORICAL-LEAD-INVESTIGATION.md`, or the #610 report
(`grep` by path and basename). Related but distinct: `PLAN-05` (`DISC-002`) and the OWASP plans are different files from `GH-001`–`GH-003`;
`IDR-H-006`'s subject (the scanner add-on plan) is not `GH-003`'s packet. `GH-004` is related by subject to `DISC-001` and `IDR-H-001`
(ACP execution) but is a different artifact; it should be ruled on with them.

## 5. Eligibility assessment

| Class | Leads | Count |
| --- | --- | --- |
| `STRICT_ELIGIBLE_PROPOSED` | GH-001, GH-002, GH-003 | **3** |
| `NEEDS_OPERATOR_REVIEW` | GH-004, GH-005, GH-006 | 3 |
| `PARTIAL_ONLY` | GH-007, GH-008 | 2 |
| `EVIDENCE_BLOCKED` | GH-009 – GH-014 | 6 |
| `EXCLUDED` | GH-015 – GH-018 (groups) | 4 groups |

Every strict proposal is conditional on R-A (PR-head draft pins) and R-B (bot-surfaced, model-verified). If either is rejected,
all three fall to zero. None is verified or adjudicated.

## 6. Updated provisional floor analysis

Provisional arithmetic only. Baseline from #610 is unchanged and kept apart from this pass.

| Measure | Plans | Defects |
| --- | --- | --- |
| Registered floor | 15 | 15 |
| Proposed strict, #610 baseline | 3 | 4 |
| Independently adjudicated | **0** | **0** |
| New strict-proposed this pass (GH-001–003) | +3 | +3 |
| Proposed strict incl. this pass | **6** | **7** |
| Shortfall, #610 baseline | 12 | 11 |
| **Shortfall incl. this pass** | **9** | **8** |
| Potential, if `DISC-001` also ruled eligible | +1 / +3 | |
| Potential, if GH-004 / GH-005 / GH-006 admitted | +1 / ≤+4, +1 / +1, +1 / +1 | |
| Upper bound with every potential lead (not a forecast) | 6 + 1 + 3 = 10 | 7 + 3 + 6 = 16 |

Even with every potential lead admitted, the strict plan count (10) stays 5 short of the 15-plan floor; the defect floor could be met only
by admitting multi-defect leads. `GH-009`–`GH-014` could add plans if their checks complete and pass R-A/R-B, but are not counted. Finding
leads does not satisfy the floor.

## 7. Unresolved operator decisions (proposals, none authorized)

- **R-A:** may a defect found in a PR-head draft and fixed before merge (no merged defective revision) count as a §5.3 historical defect?
- **R-B:** may a defect first surfaced by a model reviewer count, when the independent evidence is a deterministic repository check?
- **R-C:** artifact kind and defect kind (gap record `DISC-001`, approval packet `GH-004`, reference/guide docs `GH-005`/`GH-006`, evidence reports `GH-015`):
  do factual-accuracy defects in non-plan documents count? Rule together with `DISC-001` and `IDR-H-001`.
- Whether the owner (sole human author and sole human commenter) may be the only human link in the evidence chain.

## 8. Recommended additional verification (not authorized)

1. Rule R-A/R-B/R-C before further work; they gate every number above.
2. Have an adjudicator, not the discovering model, repeat the deterministic checks for GH-001–003 at the pins named.
3. Complete GH-009–GH-014 (`git grep` / one trace each) and re-derive GH-004(c,d), GH-006.
4. Do not treat bot reviewer text as reviewer-visible evidence; give a reviewer only the artifact and the tree at the pin.

## 9. Non-authorization statement

This report authorizes nothing. It does not: promote any candidate; rule on `DISC-001` or `IDR-H-001`; treat a gap record or approval
packet as eligible; change any eligibility criterion, decision, threshold, or canonical disposition; modify `candidates.json`; construct a
seeded or control case; designate an adjudicator, seeder, or case checker; access sealed answer-key material; adjudicate; freeze the corpus;
run model inference for the experiment; or execute the experiment. Decision 6 remains pending. The corpus remains `NOT FROZEN`, the
experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.
