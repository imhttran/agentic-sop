# Historical Corpus — Strict Evidence Verification (five leads)

**Type:** Verification record (non-normative, read-only). It re-verifies already-proposed leads at the line level. It is **not** candidate promotion,
adjudication, a corpus freeze, or an experiment result. **Every candidate below is PROPOSED — NOT APPROVED.**
**Baseline:** `origin/main` @ `1bda826d252c0322b4755ba84dc0bc282819af94` (merge of PR #625), 2026-11-22.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.2–5.3.
**Eligibility rule:** [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md) (`conservative`).
**Builds on:** [HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY.md](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY.md) (PR #613, #622) and
[HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY-2.md](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY-2.md) (PR #620), which it does not modify.
[candidates.json](candidates.json) is unchanged.

## 1. Result in one paragraph

All five selected leads reproduce at their pins, with exact file and line references. **No new lead was added**, so the proposed totals do not change: this pass converts
five proposals into reproducible evidence (**5 plans, 6 defects**). It does **not** make any of them eligible. Two things still decide every number: whether a defect surfaced by a
bot reviewer and checked by a model counts (R-B), and whether a decision packet or plan-prompt document is a "plan, spec or requirements" artifact (R-C-packet). Independently adjudicated stays **0 / 0**.

## 2. Preflight and ownership

- `origin/main` fetched; no open PRs at the start (no overlapping investigation). `DISC-001` / `IDR-H-001` were verified by PR #618 and are **not** selected; they are corpus-owner rulings.
- Decision 5-B tooling (#623, #624) and the Decision 6 and 7 files were not read for changes and not touched. Existing worktrees and ignored `.agent-sdlc/` untouched.
- Isolated worktree `docs/egm-strict-evidence-verification`.

## 3. Lead selection

Selected from the PR #613 and #620 proposals, because they were verified once by the discovering model and never at line level, and because the PR #620 ranking named them first:

| Lead | Why selected |
| --- | --- |
| GH-013 | the only lead with a **merged correction** (#486) and a reproducible behaviour |
| GH-002 | contradiction with a recorded operator decision, present in the pin tree |
| GH-001 | cheapest deterministic check (one `git grep`); defect never corrected, so a clean negative-control for "fix exists" |
| GH-003 | executable claim ("deletes nothing") that can be tested in a scratch directory |
| GH-009 | two defects in one plan; the shipped implementation later diverged from the proposal in the same two places |

Not selected: GH-010, GH-011, GH-012, GH-014 (verified in #620, line-level re-check recommended in §11), the held leads (`GH-004`, `GH-006`, `GH2-*`).

## 4. Method

For each lead: pin commit and commit time; reviewer thread creation time (`created_at` of the pin's review comment); passage by `git show <pin>:<path>` with line numbers; contradicting code or document
at the same pin; search of `origin/main` for a later correction of the passage; contamination checks on the pin tree (`git ls-tree`, `git grep`); duplicate check against `candidates.json` at `1bda826d`.
Everything is read-only against Git history except two scratch-directory reproductions (GH-003, GH-013) that touched only a temporary directory outside the repository.

### 4.1 Facts that hold for all five

| Check | Result |
| --- | --- |
| Pin is the commit the reviewer saw | `original_commit_id` equals the pin for every thread |
| Discovery postdates the pin | each thread was created **1–3 minutes** after the pin commit (the automated review of that push), so the pin is pre-discovery by construction |
| Reviewed revision = merged revision | true for GH-001, 003, 009, 013 (blob at pin equals blob at merge); GH-002 differs only by a later status-index edit and its prompt 15 text is unchanged |
| EGM or corpus material in the pin tree | **none**: `docs/reports/independent-design-review-corpus/` and `EGM-GOLDFISH-REVIEW-ASSESSMENT.md` are absent at all five pins (`git ls-tree`) |
| Already in `candidates.json` | **no** (artifact paths grepped at `1bda826d`: 0 hits) |
| Discovery source | automated reviewer `chatgpt-codex-connector[bot]`; **no human** reviewer; no reply by the owner on any of the five threads |

## 5. Evidence matrix

| Lead | Artifact @ pin | Defects | Independent discovery | Corrective change | Class (proposed) |
| --- | --- | --- | --- | --- | --- |
| GH-013 | `docs/plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md` @ `de7161f27f1abec2a82112b9caf2d29c744313c8` | 1 | PR #484 [thread](https://github.com/imhttran/agentic-sop/pull/484#discussion_r4568608535) | `54beaf27d7138bd93b4d04baf671f868de5db318` via #486 | **STRICT_CANDIDATE** |
| GH-002 | `docs/plans/OWASP-LLM-GAP-PROMPTS.md` @ `46f190f210bfc2b3fcb01bc498d2533a8d7f01bf` | 1 | PR #397 [thread](https://github.com/imhttran/agentic-sop/pull/397#discussion_r4532926068) | none to the passage | **STRICT_CANDIDATE** (R-B) |
| GH-001 | `docs/plans/PLAN-Ponytail-Complexity-Audit-Cleanup.md` @ `208b4e72203f437bb0c7431c5afcbc5bbf63d4c4` | 1 | PR #562 [thread](https://github.com/imhttran/agentic-sop/pull/562#discussion_r4645807748) | none (plan unchanged since the pin) | **STRICT_CANDIDATE** (R-B) |
| GH-003 | `docs/plans/PROPOSAL-SEMGREP-IGNORED-COVERAGE.md` @ `62e04e19a21c7ae441e1c8a7e7fa02924983932b` | 1 (minor) | PR #522 [thread](https://github.com/imhttran/agentic-sop/pull/522#discussion_r4603209734) | none (script still at `origin/main` lines 175–181) | **STRICT_CANDIDATE** (R-B) |
| GH-009 | `docs/plans/PROPOSAL-RUN-COMMAND-ISOLATION.md` @ `70ea0664bd65de90622b55a5f887c9cfaba1caec` | 2 | PR #323 threads [`4518476231`](https://github.com/imhttran/agentic-sop/pull/323#discussion_r4518476231), [`4518476239`](https://github.com/imhttran/agentic-sop/pull/323#discussion_r4518476239) | doc not corrected; **shipped code diverges** (§6.5) | **STRICT_CANDIDATE** (R-B) |

## 6. Per-defect historical reconstruction

### 6.1 GH-013 — D4-5 packet recommends G2 for git execution vectors (PR #484)

- **Pin:** `de7161f2…`, committed 2026-11-13 14:57:44 -06:00 (20:57:44Z); thread created 21:00:06Z.
- **Artifact passage (verified):** `PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md:121` — "**G2** … run every harness git invocation with execution vectors disabled (`--no-ext-diff`, `--no-textconv`,
  `-c core.fsmonitor=false`) … Covers external diff, textconv, and fsmonitor: the program-execution vectors reachable from `status`/`diff`". Lines 86 and 102 record that model code can write `.git/config`;
  line 123 (G4) rests on that. The packet contains **no mention of clean filters** (`grep -ci filter` = 0).
- **Defect:** the recommended control leaves a fourth vector open: `filter.<driver>.clean` runs during `status` and `diff` despite the G2 flags.
- **Check performed:** in a temporary repository with `.gitattributes` `f filter=probe`, `filter.probe.clean = touch <marker>; cat`, and a modified tracked file, `git -c core.fsmonitor=false -c core.pager=cat status`
  and `git diff --no-ext-diff --no-textconv` both created the marker. Git 2.54.0 here; the reviewer reported Git 2.43.0; the corrective commit reports Git 2.50.1.
- **Determinable without hindsight:** yes. Filter semantics are documented Git behaviour; the packet's own lines 86/102 give the injection route.
- **Corrective change:** `54beaf27…` ("correct the D4-5 packet after the clean-filter finding (G2 → G3)", merged via #486, `7c6f4f26`), which adds §3.4 "Clean filters execute from `status` and `diff` (measured)" and its row at line 150.
- **Contamination:** pin tree has no clean-filter text in `docs/` or `internal/` (`git grep -l -i "clean filter"` empty at the pin); the corrective text exists only in later commits.
- **Uncertainty:** behaviour was measured on Git 2.54.0 and cited for 2.43.0 and 2.50.1 by others; the experiment's Git version is not fixed. The second finding on #486 (G3 "contained, not disabled", thread `4570365270`) is on the corrected revision and is **PARTIAL**, not counted.

### 6.2 GH-002 — OWASP gap prompt 15 re-proposes a rejected design (PR #397)

- **Pin:** `46f190f2…`, 2026-11-10 19:44:22 -06:00 (01:44:22Z on the 11th); thread 01:45:48Z.
- **Artifact passage (verified):** `OWASP-LLM-GAP-PROMPTS.md:466–492`. Step 2 (≈lines 477–480): "digest the SOP state directory before and after each run_command and treat an unexpected change as evidence tampering …"; the Goal and
  Done-when clauses require detection to be "IMPLEMENTED and VERIFIED".
- **Contradicting document at the same pin (verified):** `docs/reports/owasp-llm/llm07-state-write-detection-phase1.md`. Its §1 asks exactly: "can a before/after fingerprint of `.agent-sdlc/` around each `run_command` reliably detect a write made by the subprocess …?";
  line 3 "operator decision **NO-GO** (2026-11-09)"; §2 "Decision: NO-GO"; line 87 "Phase 2 (the fingerprint check) was not implemented"; line 89 the plan was archived `SUPERSEDED`.
- **Defect:** the prompt directs implementation of the approach the operator already rejected, and does not gate it on reconsidering the decision.
- **Determinable without hindsight:** yes; the prompt's own step 1 points at the phase 1 report. **Uncertainty (interpretation):** an adjudicator may judge that step 1 ("State exactly what phase 1 detects and what it misses") already mitigates the problem for a careful implementer; the defect is then a missing explicit gate rather than an unknown contradiction. Severity: "changes approach".
- **Corrective change:** none to the passage found. The `llm07` document itself later records that the opt-in sandbox implements the equivalent control (its "Update (LLM03)" note), which is context, not a fix to the prompt.
- **Contamination:** the contradicting document is a **permitted reference** (it is in the pin tree and named by the prompt); the review thread is not in Git.

### 6.3 GH-001 — complexity-audit plan lists a wired predicate as dead code (PR #562)

- **Pin:** `208b4e72…`, 2026-11-20 11:47:24 -06:00 (17:47:24Z); thread 17:48:59Z. The plan's stated baseline is `e28348d3e75c27d68c08124dc610899e75947d94`.
- **Artifact passages (verified):** `PLAN-Ponytail-Complexity-Audit-Cleanup.md:4` baseline `e28348d3`; `:28` "with no behavior change"; `:79` "No behavior change on a wired path"; `:139–141` "remove `IsReady`, `IsDone`, and `IsBlockedSelectable` (or wire the latter into the scheduler in the same change)".
- **Code (verified):** `internal/scheduler/scheduler.go:211` `if task.IsBlockedSelectable(byID) {` at the pin **and** at the plan's own baseline `e28348d3`. `internal/domain/selection.go:16` defines it; `selection_extra_test.go` pins its semantics.
- **Defect:** the plan's premise ("dead code") is false for `IsBlockedSelectable`; deleting it breaks blocked-task recovery, contradicting the plan's no-behaviour-change rule.
- **Determinable without hindsight:** yes, by one `git grep` on the tree the plan names as its baseline.
- **Corrective change:** none; the plan file has no commits after the pin on `origin/main`. This is a **defect without a fix**, the weakest form of evidence pointer under §5.3 (which lists "merged fixes, erratum, reverted change or recorded gap"); it rests on R-B.
- **Contamination:** the only document mentioning the symbol at the pin is the plan itself.

### 6.4 GH-003 — "reversible, deletes nothing" relocation script can lose files (PR #522)

- **Pin:** `62e04e19…`, 2026-11-17 11:18:08 -06:00; thread 17:20:09Z.
- **Artifact passage (verified):** `PROPOSAL-SEMGREP-IGNORED-COVERAGE.md:175–186`: `scratch=~/sop-scratch/$(date +%Y%m%d)` (line 176 at the pin), `mv -- "$p" "$scratch/$p"` (line 182), "This is reversible (`mv` them back) and deletes nothing."
- **Check performed:** two source directories with the same relative path `x/f` (contents `first`, `second`) relocated with the script's `mv -- … "$scratch/$p"` into one same-day scratch directory, using `/bin/mv` under `env -i … bash --noprofile` on Darwin: the destination ended as `second`, the first file's content was lost, and the second source was gone. (A first attempt through the interactive shell prompted before overwriting because that shell evidently aliases `mv` to interactive mode; it was discarded and the clean run is the reported one.) GNU `mv` also overwrites by default.
- **Defect:** a second run on the same day, or from a second checkout with the same relative path, silently overwrites a previously preserved file, contradicting "deletes nothing".
- **Determinable without hindsight:** yes, from the document text and standard `mv` semantics. **Severity:** minor; the trigger needs a repeat run.
- **Corrective change:** none to the script; `40d52348` (2026-11-17, "record the A1 Semgrep scope fix") edited the file but the same script is at `origin/main` lines 175–181. Evidence pointer is the review finding; R-B.
- **Uncertainty:** the severity could lead the owner to treat it as immaterial.

### 6.5 GH-009 — run_command isolation proposal, two defects (PR #323)

- **Pin:** `70ea0664…`, 2026-11-09 15:56:50 -06:00; both threads 21:58:41Z.
- **(a) Sandbox root.** `PROPOSAL-RUN-COMMAND-ISOLATION.md:504–505` step 5: `sandbox.Wrap(h.root, root.path, argv, env)`. At the pin `Harness.root` is the canonical primary root (`internal/toolharness/harness.go:145`); `authorizedRoot` (`:197–201`) and `selectRoot` (`:262`) supply the selected root. The existing builder
  `internal/evaltarget/isolation_darwin.go:27,48` has signature `(…, root, dir string, argv, env []string …)`, renders `root` into both the read (`:75`) and write (`:76`) `subpath` rules (`:80`) and uses `dir` only for `cmd.Dir` (`:83`). A command in an additional root would run with that root outside the permitted paths and the primary repository inside.
- **(b) No-test diagnostic.** `:510–511` step 7 execution "unchanged, except `argv` may be the wrapped command". `noTestsMatched` (`harness.go:642–643`) returns false unless `filepath.Base(argv[0]) == "go"` and `argv[1] == "test"`; `runCommandWithStatus` calls it with the same `argv` (`:625`). A wrapped argv would suppress the zero-match warning.
- **Determinable without hindsight:** yes, from the proposal's step list and the named files at the pin. (a) needs two files; moderate effort.
- **Corrective evidence in code (not a document fix):** on `origin/main` `execIsolated(ctx, root.path, argv, nil)` isolates around the selected `root.path`, and `noTestsMatched(argv, result)` is called on the original `argv` (`internal/toolharness/harness.go`, around lines 681–689 and 934–940). The shipped implementation (`cf94f030`, 2026-11-11; `02da4ee2`, 2026-11-16) therefore diverges from the proposal in exactly the two places. The proposal text at `origin/main:511` still reads `sandbox.Wrap(h.root, root.path, …)`.
- **Uncertainty:** the proposal is a design intent; an adjudicator may read "isolate" loosely. Two defects count only if both are accepted.

## 7. Eligibility classification

| Lead | Class | Basis | What would change it |
| --- | --- | --- | --- |
| GH-013 | STRICT_CANDIDATE | merged defect, merged correction, reproduced, pre-fix pin has no leakage | R-C-packet "no" (it is a decision packet) |
| GH-002 | STRICT_CANDIDATE (R-B) | contradiction inside the pin tree | R-B "no"; adjudicator treating step 1 as mitigating |
| GH-001 | STRICT_CANDIDATE (R-B) | one-grep check; no corrective change | R-B "no"; §5.3 requiring a corrective change |
| GH-003 | STRICT_CANDIDATE (R-B) | reproduced; low severity | R-B or R-C-packet "no"; immateriality |
| GH-009 | STRICT_CANDIDATE (R-B), 2 defects | code at the pin; shipped code diverges | R-B or R-C-packet "no"; one defect dropped |

STRICT_CANDIDATE here means only that the evidence appears to satisfy Decision 3's conservative criteria pending an authorized eligibility determination. No lead was `PARTIAL`, `HINDSIGHT_ONLY`, `DUPLICATE`, `EXCLUDED` or `EVIDENCE_BLOCKED` in this pass; GH-013's second finding remains `PARTIAL` (§6.1).

## 8. Duplicate and contamination findings

- **Duplicates:** none against `candidates.json` or the existing reports. Subject proximity: GH-009 is in the Gap 13 isolation cluster with GH-010 (#620); GH-013 is in the LLM03 D4 family with GH-011, GH-012. By artifact they count separately; by cluster they would merge.
- **Contamination:** pin trees (3,195–3,395 files) contain **no** EGM or corpus-record files and **no** text of the later corrective changes. The review threads and these reports are outside the pin trees. **Open design question (not decided here):** the pin tree is the whole repository at that commit; what subset is "reviewer-visible" (the artifact only, or artifact plus referenced paths) is a case-construction decision. GH-002's determinability depends on the `llm07` report being in the visible set.
- **Independence limits:** every discovery source is one bot; the verifier is a model. Nothing here is independent adjudication.

## 9. Provisional corpus-count impact

Official counts unchanged; independently adjudicated **0 / 0**; seeded cases excluded. Plans / defects.

| Stack (by artifact) | Plans | Defects | Shortfall vs 15 / 15 |
| --- | --- | --- | --- |
| Baseline proposed strict | 3 | 4 | 12 / 11 |
| + PR #613 proposals (GH-001, 002, 003) | +3 | +3 | |
| + PR #620 proposals (GH-009 – GH-014) | +6 | +7 | |
| **Proposed, all stacked** (excludes `DISC-001`, `IDR-H-001`, held leads) | **12** | **14** | **3 / 1** |
| of which re-verified at line level here (GH-001, 002, 003, 009, 013) | 5 | 6 | |
| Not re-verified here (GH-010, 011, 012, 014) | 4 | 4 | |
| **Newly proposed by this pass** | **0** | **0** | |

Cluster-adjusted (PR #620 §9): the PR #620 layer counts 4 plans, so the stack is **10 / 14**, shortfall **5 / 1**. Conservative bounds from PR #620 stand (8 / 6 and 11 / 10 short). These are possible additions, not members.

## 10. Remaining blockers

1. R-B and R-C-packet rulings (they gate all 12 plans).
2. A defined "reviewer-visible" subset for each pin (case-construction decision; not authorized here).
3. Human adjudication of every check above.
4. Without further leads or rulings the floor is not reached: plans remain the binding constraint.

## 11. Next three investigations (ranked)

1. **Line-level re-verification of GH-010, GH-011, GH-012, GH-014** with full-history corrective searches (the other four of the six PR #620 leads). Highest value: they supply 4 plans / 4 defects.
2. **Reviewer-visible export design check:** on paper, define for GH-002 and GH-009 which referenced paths a reviewer would get, and whether the permitted-reference set can be stated without showing any later fix (needs the case-construction authorization the operator has not given; documentation only).
3. **GH-004 (c, d), GH-006 and `GH2-003`–`005`:** the remaining held or triaged-only defects; cheap, and the only untested defects that may add plans.

## 12. Non-authorization statement

This report authorizes nothing. It does not: promote any candidate; rule on `DISC-001` or `IDR-H-001`; change Decision 3, any threshold (including the 613-call ceiling), the registered protocol or any Decision 5–9 state; modify `candidates.json` or any frozen or sealed artifact; change
`case_construction_authorized`; construct a seeded or control case; create or access a sealed manifest; designate a case checker or adjudicator; adjudicate; freeze the corpus; run model inference; or execute the experiment. The corpus remains `NOT FROZEN`, the experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.
