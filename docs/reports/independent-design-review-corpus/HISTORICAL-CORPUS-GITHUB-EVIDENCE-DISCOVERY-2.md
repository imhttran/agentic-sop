# Historical Corpus Discovery — GitHub Evidence, Second Pass

**Type:** Investigation record (non-normative, read-only). It is **not** candidate promotion, adjudication,
a corpus freeze, or an experiment result.
**File name:** `-2` follows [HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY.md](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY.md)
(PR #613, the first GitHub pass), which already holds the requested name and is **not modified**. This pass
closes that report's open items rather than repeating its sweep.
**Baseline:** `origin/main` @ `5600b38b81f77c6dbd304232645e54d4c8ef30f7` (merge of PR #618), 2026-11-21.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.2–5.3.
**Eligibility rule:** [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md) (`conservative`).
**Related:** [HISTORICAL-CORPUS-EVIDENCE-DISCOVERY.md](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY.md) (PR #610),
[DISC-001-H-001-EVIDENCE-VERIFICATION.md](DISC-001-H-001-EVIDENCE-VERIFICATION.md) (PR #618),
[candidates.json](candidates.json) (unchanged).

Lead IDs use `GH-` as in the first pass; `GH-009`–`GH-014` and `GH-006` keep their first-pass IDs. New leads use `GH2-`.
Classifications are **investigative proposals**. Nothing here is eligible, counted, or adjudicated.

## 1. Preflight

- `origin/main` fetched (`5600b38b`). Registered protocol §5.2–5.3, Decision 3, and all discovery and verification
  reports in this folder were re-read. `candidates.json` holds `IDR-H-001`–`008`, `IDR-E-001`–`012`, `IDR-X-001`–`003`;
  the discovery IDs are `DISC-001`–`012` and `GH-001`–`018`.
- Open PRs: only #619 (Decision 7 record, another agent). Existing worktrees (`.worktrees/egm-decision-*`,
  `egm-disc001-verification`, `egm-github-evidence-discovery`, `egm-historical-evidence-discovery`,
  `agentic-sop-zed-skills`) and the ignored `.agent-sdlc/` tree were not entered or changed.
- Work done in the new isolated worktree `docs/egm-github-evidence-discovery-2`, cut from `5600b38b`.

## 2. Search scope and methodology

Read-only `gh` / `gh api` against `imhttran/agentic-sop`; raw exports kept in a session scratchpad, not committed.

| Surface | Inspected |
| --- | --- |
| Pull requests | **602** (all states; files, reviews) |
| Inline review comments | **284** = **214** top-level threads + 70 replies, on **142** PRs; **0** by a human at top level |
| Docs-path top-level threads | **49** on **42** PRs (`docs/`), plus **8** on `skills/` and `scripts/` |
| PR reviews | **226** (all `COMMENTED`; none `CHANGES_REQUESTED`) |
| Conversation comments | **660**: 500 bot status summaries (no finding text; only `has_any_findings` flags), 56 usage-limit notices, 24 "no issues" receipts, 80 owner comments |
| Issues | **17** (unchanged since the first pass) |
| Merged PRs touching `docs/plans`, `specs`, `reference`, `guides`, `architecture` with a corrective title (`fix`, `correct`, `reconcile`, `align`, `stale`, `gap`, `repair`, `revert`) | **160** (title and PR body triaged; the ones with reviewer evidence read in full) |
| Commits on those paths mentioning Codex, review findings, or reviewers | **15** |

Work performed, in order:

1. **Completed the six `EVIDENCE_BLOCKED` leads (`GH-009`–`GH-014`)**, each by a deterministic check at the exact pin.
2. **Re-derived `GH-006`** (test body read).
3. **Merged-vs-pinned test (new).** For every lead, compared the blob at the reviewed pin with the blob at the PR's
   merge commit. `SAME` means the reviewed revision is the revision that merged, so the defect was **not** fixed
   inside the PR. This bears directly on the first pass's open question R-A (PR-head drafts).
4. **Thread state.** Queried review-thread resolution by GraphQL for the leads' PRs.
5. **Swept surfaces the first pass did not triage:** `skills/*` and `scripts/` threads (operator procedures), the
   post-#610 threads, the 160 corrective-titled PRs, owner conversation comments, and the 500 bot summaries.
6. Checked every lead against `candidates.json`, the #610 and #613 reports, and the #618 report for duplication.

### 2.1 Limits (read before the findings)

- **No independent human reviewer exists.** All 214 top-level threads are `chatgpt-codex-connector[bot]` (213) or
  `github-advanced-security[bot]` (1). The repository owner is the sole human author and commenter. Every finding below is
  **bot-surfaced and model-verified**. The deterministic checks here were run by a model in this session and **must be
  repeated by an adjudicator**.
- Single-author repository. Authorship and time-ordering cannot be proved from Git identity.
- This is the same repository the experiment's own tooling lives in; no external corpus was searched.
- A negative result on a surface is not proof of absence. The 164 code-path threads were not triaged (the artifact is code).

## 3. Verified facts that change the first pass's picture

1. **The reviewed revision is the merged revision.** For `GH-001`, `GH-003`, and `GH-009`–`GH-014` the blob at the review
   pin equals the blob at the merge commit (table in §5). The defective text merged; the finding thread was never resolved
   (`isResolved=false`, `isOutdated=false` on every lead thread in #323, #446, #467, #471, #484, #501, #522, #562, #397) and has **no reply**.
   `GH-002` (#397) differs only by a later status-index edit; the prompt 15 passage is unchanged. R-A is therefore **moot
   for these leads**: they are merged documents with a defect, not PR-head drafts fixed before merge.
2. **One lead has a merged correction.** `GH-013` was corrected by a separate merged PR (#486).
3. **The other five new leads have no corrective change.** The defect text is still in the merged artifact. Their only
   evidence pointer is the review finding plus a deterministic check. §5.3 lists "merged fixes, erratum, reverted change or
   recorded gap"; a review finding is none of these, so the **R-B ruling is load-bearing** for them.
4. **Classification inconsistency in the first pass, flagged not corrected.** #613 classed `GH-003`
   (`PROPOSAL-SEMGREP-IGNORED-COVERAGE.md`, a decision packet) as strict but `GH-004` (an operator approval packet) as
   `NEEDS_OPERATOR_REVIEW` on artifact kind. `GH-009`–`GH-014` are the same kind as `GH-003`. This report treats the
   whole decision-packet cohort together under one kind ruling (R-C-packet, §8). It does not edit #613.

## 4. Candidate evidence matrix

| Lead | Artifact (pin) | Defects | Discovery evidence | Corrective change | Class |
| --- | --- | --- | --- | --- | --- |
| **GH-013** | `PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md` @ `de7161f2` | 1 (+1 partial) | #484 [thread](https://github.com/imhttran/agentic-sop/pull/484#discussion_r4568608535), reproduced | #486 `54beaf27` (merged `7c6f4f26`) | **STRICT_CANDIDATE** (strongest) |
| **GH-009** | `PROPOSAL-RUN-COMMAND-ISOLATION.md` @ `70ea0664` | 2 | #323 [thread 1](https://github.com/imhttran/agentic-sop/pull/323#discussion_r4518476231), [thread 2](https://github.com/imhttran/agentic-sop/pull/323#discussion_r4518476239) | none found | **STRICT_CANDIDATE** (R-B) |
| **GH-010** | `PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md` @ `8495d54e` | 1 | #446 [thread](https://github.com/imhttran/agentic-sop/pull/446#discussion_r4552484077) | none found | **STRICT_CANDIDATE** (R-B) |
| **GH-011** | `PROPOSAL-LLM03-D4-ENFORCEMENT.md` @ `68015e0a` | 1 | #467 [thread](https://github.com/imhttran/agentic-sop/pull/467#discussion_r4556031113) | none found | **STRICT_CANDIDATE** (R-B) |
| **GH-012** | `PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` @ `9aaa3a9c` | 1 (minor) | #471 [thread](https://github.com/imhttran/agentic-sop/pull/471#discussion_r4557298192) | none found | **STRICT_CANDIDATE** (R-B) |
| **GH-014** | `PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md` @ `e7a09f23` | 1 | #501 [thread](https://github.com/imhttran/agentic-sop/pull/501#discussion_r4591960587) | none found | **STRICT_CANDIDATE** (R-B) |
| **GH-006** | `docs/guides/PROMPTING.md` @ `cef3b8a9` | 1 | #189 [thread](https://github.com/imhttran/agentic-sop/pull/189#discussion_r4435865492) | not searched | NEEDS_OPERATOR_REVIEW (kind) — now verified |
| **GH2-001…005** | `skills/*/SKILL.md` (five files) | 6 | #194, #304, #305, #306, #583 threads | not searched | NEEDS_OPERATOR_REVIEW (kind) |

## 5. Original artifacts, defect verification, and corrective references

All checks are `git show <sha>:<path>` / `git grep <sha>` against the pin. "Pin" is the PR head the reviewer saw; every pin is an
ancestor of `origin/main` (checked). Blob prefixes are SHA-256 of the file at the pin. **Verified** means the check was
run here; **interpretation** is labelled.

### 5.1 Pins, merge state, corrective reference

| Lead | Pin (full SHA) | Blob @ pin | PR merge | Pin = merged blob? | Later commits fixing the passage |
| --- | --- | --- | --- | --- | --- |
| GH-009 | [`70ea0664bd65de90622b55a5f887c9cfaba1caec`](https://github.com/imhttran/agentic-sop/commit/70ea0664bd65de90622b55a5f887c9cfaba1caec) | `7349b0b313c0` | #323 `a881ba9d` (2026-11-09) | SAME | none: `sandbox.Wrap(h.root, root.path, argv, env)` still at `docs/plans/PROPOSAL-RUN-COMMAND-ISOLATION.md:511` on `origin/main` |
| GH-010 | [`8495d54e26a78b28819ef062261efae6cfa610eb`](https://github.com/imhttran/agentic-sop/commit/8495d54e26a78b28819ef062261efae6cfa610eb) | `85b07b0e8e29` | #446 `5d6fad90` (2026-11-12) | SAME | none found (later edits record decisions; no socket qualification) |
| GH-011 | [`68015e0acd56c5b1318b7fffbe2f94705546980d`](https://github.com/imhttran/agentic-sop/commit/68015e0acd56c5b1318b7fffbe2f94705546980d) | `ac14f40b743e` | #467 `da033bcd` (2026-11-12) | SAME | none found |
| GH-012 | [`9aaa3a9c71653277b603043074177d5ea12cdc48`](https://github.com/imhttran/agentic-sop/commit/9aaa3a9c71653277b603043074177d5ea12cdc48) | `191a5d691c8c` | #471 `f92f5a08` (2026-11-12) | SAME | none found |
| GH-013 | [`de7161f27f1abec2a82112b9caf2d29c744313c8`](https://github.com/imhttran/agentic-sop/commit/de7161f27f1abec2a82112b9caf2d29c744313c8) | `9efbbf7c0a03` | #484 `0344c7cf` (2026-11-14) | SAME | [`54beaf27d7138bd93b4d04baf671f868de5db318`](https://github.com/imhttran/agentic-sop/commit/54beaf27d7138bd93b4d04baf671f868de5db318) via #486 (`7c6f4f26`): "correct the D4-5 packet after the clean-filter finding (G2 → G3)" |
| GH-014 | [`e7a09f23250666ec028c2cabc341c860e8e7bc8a`](https://github.com/imhttran/agentic-sop/commit/e7a09f23250666ec028c2cabc341c860e8e7bc8a) | `92e8ebe9b57c` | #501 `2e13ddea` (2026-11-16) | SAME | none found (`:15` restates the OSV prerequisite as "unchanged") |
| GH-006 | [`cef3b8a910939f4c17664f9c9dec9fced6ee6b29`](https://github.com/imhttran/agentic-sop/commit/cef3b8a910939f4c17664f9c9dec9fced6ee6b29) | `3ff1e8d54bb8` | #189 | not re-checked | not searched |

Dates are UTC merge times from `gh pr view`. "None found" means the passage was searched on `origin/main`; it does not prove no
later discussion exists.

### 5.2 Defect verification

#### GH-013 — D4-5 mandatory-isolation packet, G2 recommended (PR #484) — **verified, reproduced**

- **Original text:** line 121 recommends G2 (`--no-ext-diff`, `--no-textconv`, `-c core.fsmonitor=false`) as covering "the program-execution
  vectors reachable from `status`/`diff`". The packet never mentions clean filters (`grep -i filter` at the pin: 0 hits).
- **Check:** in a throwaway repository with `filter.probe.clean` set and a tracked, modified file, `git -c core.fsmonitor=false -c core.pager=cat status`
  and `git diff --no-ext-diff --no-textconv` both ran the clean command (a marker file appeared). Run here on Git 2.54.0. The reviewer reported Git 2.43.0;
  the corrective commit message reports Git 2.50.1. Three versions agree.
- **Discoverable without hindsight:** yes — gitattributes `filter` semantics are documented Git behaviour, and the packet itself says model code
  can write `.git/config`, which is how a filter gets installed.
- **Design vs implementation:** design defect (a recommended control does not cover the stated vector).
- **Second finding, not counted:** #486 thread [`4570365270`](https://github.com/imhttran/agentic-sop/pull/486#discussion_r4570365270) (G3 "contained, not disabled") is on the *corrected* revision
  `54beaf27` and depends on the first fix. **PARTIAL.**

#### GH-009 — run_command isolation proposal (PR #323) — **verified (2 defects)**

- **(a) Sandbox root.** Step 5 (line 504 at the pin) specifies `sandbox.Wrap(h.root, root.path, argv, env)`. At the pin `h.root` is the canonical
  **primary** repository (`harness.go:145`) and `root.path` the selected authorized root (`harness.go:197`, `selectRoot` at `:262`).
  The existing builders take `(root, dir, argv, env)` and render `root` into the read/write profile and `dir` only into `cmd.Dir`
  (`internal/evaltarget/isolation_darwin.go`, `isolationCommand` and `darwinIsolationCommand`). A command in an additional root would run with
  that root outside the sandbox's permitted paths and the primary repository inside them.
- **(b) No-test diagnostic.** Step 7 says execution is "unchanged, except `argv` may be the wrapped command". `noTestsMatched`
  (`harness.go:642`) returns false unless `filepath.Base(argv[0]) == "go"` and `argv[1] == "test"`, and `runCommandWithStatus`
  (`harness.go:625`) calls it with the same `argv`. A wrapped argv would suppress the zero-match warning.
- **Discoverable without hindsight:** yes, from the proposal's own step list and the files it names at the pin. **Interpretation:** (a)
  requires reading the sandbox builders; moderate effort.
- **Design vs implementation:** both are defects in the specified call sequence (design).

#### GH-010 — residual-risk assessment, Linux socket guarantee (PR #446) — **verified**

- **Original text (lines 119–121):** the reverse direction "remains **denied**, and is pinned by `TestRunCommandIsolationAllowsOwnUnixSocketOnly`"; line 129
  "a host socket is still unreachable from inside".
- **Check at the pin:** `internal/toolharness/isolation.go:288` carries the author's own comment: "a socket created after the scan, or deeper than the
  depth bound, is not masked". `homeSocketPaths` is bounded (`maxHomeSocketDepth = 4`, `maxHomeSocketEntries = 20000`, lines 277–278). The
  named test uses a socket under `/tmp` (`MkdirTemp("/tmp", …)`), so it does not cover a socket outside `/tmp`, `/run`, or the scanned home depth.
  The document discusses Linux elsewhere (lines 41, 49, 85) but not this residual.
- **Discoverable without hindsight:** yes (comment, constants, and test at the pin).
- **Design vs implementation:** design/assessment defect (an overstated guarantee in a decision packet).

#### GH-011 — D4 enforcement readiness (PR #467) — **verified**

- **Original text (line 194):** to make Option B pass the operator disables the `sast` category; then only the 27 Gitleaks findings block until approved.
- **Check at the pin:** `internal/security/policy.go` (≈lines 164–228): `disabledCoverageGaps` produces reasons when a disabled adapter is not replaced;
  after blocking findings reach 0, `if len(disabledGaps) > 0 { out.Verdict = VerdictIndeterminate }` runs before `VerdictPass`. Disabling `sast`
  therefore yields `INDETERMINATE`, not a pass; "required" mode still blocks delivery.
- **Discoverable without hindsight:** yes (one function at the pin).
- **Design vs implementation:** design defect (a recommended path to a green gate does not reach one).

#### GH-012 — D4 operator approval packet, fingerprint assurance (PR #471) — **verified (minor)**

- **Original text (lines 99–100):** fingerprint-exact entries keep a later pasted credential visible "because a new line or column produces a new fingerprint".
- **Check at the pin:** `StableFingerprint` (`internal/security/normalize.go`) hashes only provider, rule, path, line, column. A real credential replacing
  a fixture at the same line, column, rule and path produces the same fingerprint and stays suppressed; `Baseline.Apply` (`baseline.go:77`) matches on fingerprint only.
- **Discoverable without hindsight:** yes. **Severity:** minor (an omitted residual, not a false statement). **Interpretation:** a same-location, same-rule
  replacement is narrow; the owner may judge it too weak to count.

#### GH-014 — W8 scanner enforcement packet, stage 1 entry conditions (PR #501) — **verified**

- **Original text (§8 table, line 194):** stage 1 (`required`, `[sast, secrets]`) has no OSV prerequisite; "OSV DB provisioned" is a stage 2 entry condition.
- **Check at the pin:** `.agent-sdlc/config.yaml` enables `osv-scanner` (`category: dependencies`) with `stale_advisory_database: deny` (lines 65–66, 110–115);
  `policy.go` (≈lines 124–150) appends `adapter … is <status>` / `coverage is <…>` to `incompleteReasons` for **every enabled adapter**, independent of `required_categories`,
  and a stale database forces `INDETERMINATE` under `deny`. A stage 1 environment without a fresh OSV database cannot pass.
- **Discoverable without hindsight:** yes. **Interpretation:** whether an environment lacks the database is environmental; the entry conditions' omission is not.

#### GH-006 — `PROMPTING.md` Cargo statement (PR #189) — **now verified**

- Line 52 says "every `cargo` command" is `REQUIRES_APPROVAL`. `internal/cli/acceptance_cargo_test.go` at the pin asserts that with the acceptance permission
  enabled `cargo build` is authorized (`authorizeAcceptanceCommandWith("cargo build", true, root)`). The statement is false when `cargo.acceptance_check` is on.
  Still a guide-accuracy defect (kind ruling R-C), not a design defect.

### 5.3 Skill procedures (new surface, `GH2-`) — NEEDS_OPERATOR_REVIEW

`skills/*/SKILL.md` are operator procedures the agent executes, not plans or specs. Same kind question as `DISC-001` (R-C). The threads are bot-surfaced.

| Lead | Artifact @ pin | Finding | Check |
| --- | --- | --- | --- |
| GH2-001 | `skills/decision-ab/SKILL.md` @ `62388bab9a52d95c65259e8e6afa713bc6ab3fcb` (#583, threads `4654772202`, `4654780957`) | the three RUN commands (lines 46, 49, 52) omit `-endpoint`; the flag defaults to `127.0.0.1:8000` (`cmd/deceleval-ab/main.go:41`) while `-discover` (line 29) passes it. Second: the relative `go run ./cmd/…` resolves against the caller's directory | first **verified**; second triaged (T) |
| GH2-002 | `skills/sop-fix-conflicts/SKILL.md` @ `52c835dd854ed7db4f3f8da46ebb2b81ede575e6` (#304, `4514844512`) | prescribes `scripts/checks/pre-push.sh` (line 149); at the pin the script recomputes `git merge-tree --write-tree HEAD "$base"` (line 93) and has `--no-merge` (line 44) | **verified** (script anchors); not executed |
| GH2-003 | `skills/sop-ci-fix/SKILL.md` @ `8c81354e044b8cbc68a8bf07ee0373cd35726f96` (#305, `4515047914`) | same script, same merge-candidate behaviour for the repair verification | T |
| GH2-004 | `skills/codex-review-triage/SKILL.md` @ `3c1f2a2a75d0656853218450ed075c80d3277fc4` (#306, `4515204953`) | helper path is relative to the caller's project | T |
| GH2-005 | `skills/sop-cleanup/SKILL.md` @ `fe8a5e488ac0220d94e9457dd089c0125480edf9` (#194, `4443735146`) | promised retrieval exclusion not implemented by the index | T |

Up to 6 defects on 5 artifacts. Also `scripts/setup-planner.sh` (#136) and `scripts/agents/codex-review-triage.sh` (#306): code, excluded.

## 6. Duplicate analysis

- No `GH-009`–`GH-014` or `GH2-` artifact path appears in `candidates.json`, `lead-extraction.json`, `HISTORICAL-LEAD-INVESTIGATION.md`,
  or the #610 and #618 reports (`grep` by path); the only mention is the #613 report's blocked-lead table.
- **Subject clusters** (an owner may count by cluster, as in #618 §5.4): `GH-009` + `GH-010` (run_command isolation, Gap 13);
  `GH-011` + `GH-012` (LLM03 D4 baseline enforcement and approval); `GH-013` (D4-5 execution isolation, near but distinct from the D4 pair);
  `GH-014` (W8 stage plan). By artifact: 6 plans. By cluster: 4 plans.
- Related but distinct: `IDR-H-007` (`PLAN-08B`), `GH-003` (Semgrep packet, #522), `DISC-012`. `GH-004` is the ACP approval packet, related to `DISC-001`/`IDR-H-001`.
- No reviewed artifact is an EGM or corpus governance document.

## 7. Other leads inspected and excluded

- **Corrective-titled PRs (160).** No new strict lead. Examples read in full: #477 (a post-#474 text drift in a merged packet; found by the author, no independent
  report) — `HINDSIGHT_ONLY`; #442 / #444 (the Windows B1 refusal bypass; a Codex P2 on #441's **code**) — `EXCLUDED` (implementation); #494 — code; #486 — is `GH-013`; #568
  and #493 — EGM and status reconciliation, `EXCLUDED`; #573 / #576 — the `DISC-001` family, held for the corpus owner.
  The remainder are status reconciliations, behaviour-change doc edits, or `DISC-008`/`DISC-009`-class corrections already `EXCLUDED`.
- **Post-#610 threads** on #606, #614, #615 are governance drafts about this experiment: `EXCLUDED`.
- **Owner comments and issues.** No owner comment or issue documents a plan/spec defect with a pinned pre-defect revision beyond what `GH-018` already covers. `#273`/`#140`/`#158`
  describe tool behaviour (implementation).
- **Bot summaries (500).** Status tables only; no finding content to mine.
- **Reports (`docs/reports/*`, 29 threads, 20 under `owasp-llm/`).** Evidence reports and acceptance requests, as `GH-015`; not re-opened.

## 8. Classification and uncertainty

| Class | Leads | Plans | Defects |
| --- | --- | --- | --- |
| `STRICT_CANDIDATE` (strongest: merged defect + merged correction + reproduction) | GH-013 | 1 | 1 |
| `STRICT_CANDIDATE` (merged, uncorrected; evidence = reviewer finding + deterministic check) | GH-009, GH-010, GH-011, GH-012, GH-014 | 5 | 6 |
| `NEEDS_OPERATOR_REVIEW` | GH-006; GH2-001…005 | 6 | 7 |
| `PARTIAL` | GH-013's second finding (#486 thread) | 0 | 0 |
| `HINDSIGHT_ONLY` / `EXCLUDED` | #477; #442/#444/#494; post-#610 governance threads; code-path and report threads | 0 | 0 |

STRICT_CANDIDATE here means only "appears to meet Decision 3's conservative criteria, pending an authorized eligibility determination". Rulings the numbers depend on:

- **R-B (bot-surfaced, model-verified).** Now covers every strict-proposed lead in #613 and here. For `GH-013` the corrective merged commit and its independent
  reproduction soften it.
- **R-C-packet (artifact kind).** Whether an operator decision packet (`PROPOSAL-*`) is a "plan, spec or requirements" document. It governs `GH-003`, `GH-004`,
  and `GH-009`–`GH-014` together. If excluded, all of them drop.
- **Counting unit:** artifact or subject cluster (§6).
- **R-A** is largely moot for the leads in §3, but not for `GH-004`, whose pins are PR heads.
- Two further uncertainties: no lead has a reviewer-visible exclusion of later fixes proved beyond the pin (the pin tree predates every fix by construction, but an exported tree
  still has to be built and checked); and the six strict leads include four from one security programme, which may correlate model-review behaviour.

## 9. Provisional corpus-floor impact

Arithmetic only. **Official counts are unchanged; nothing is independently adjudicated (0 / 0); seeded cases do not count.**
Baseline per the task: proposed strict **3 plans / 4 defects**, shortfall **12 / 11**. "#618 S3" is that report's scenario "both ruled in, counted
independently" (+2 / +3), without the partial D3. "#613 layer" is `GH-001`–`GH-003` (+3 / +3, not in the 3/4 baseline).

| Scenario | Plans / defects, by artifact | Shortfall | By subject cluster | Shortfall |
| --- | --- | --- | --- | --- |
| A — baseline | 3 / 4 | 12 / 11 | 3 / 4 | 12 / 11 |
| B — A + this pass (all six) | 9 / 11 | 6 / 4 | 7 / 11 | 8 / 4 |
| C — B + #618 (S3 by artifact; S4, +1 / +2, by cluster) | 11 / 14 | 4 / 1 | 8 / 13 | 7 / 2 |
| D — C + #613 layer | 14 / 17 | 1 / 0 | 11 / 16 | 4 / 0 |
| E — D + `GH-004` (2 verified defects; by cluster it shares `DISC-001`'s subject, so +0 plans / +2 defects) | 15 / 19 | 0 / 0 | 11 / 18 | 4 / 0 |
| F — packets excluded (R-C-packet "no"): A + `GH-001`, `GH-002` + #618 (S3 / S4) | 7 / 9 | 8 / 6 | 6 / 8 | 9 / 7 |
| G — only the strongest new lead (`GH-013`) added to A | 4 / 5 | 11 / 10 | 4 / 5 | 11 / 10 |

Reading the table:

- Before this pass the best proposed case (#618 S5) left **10 plans / 7 defects** short. With the six leads the *proposed* count can reach the floor, but
  **only at E by artifact**, where every conditional ruling goes the favorable way (R-B yes, packets admitted, artifact counting, `DISC-001`/`H-001`
  admitted, `GH-004` admitted, and every proposed item surviving independent adjudication). One failed adjudication or one unfavorable ruling leaves it short.
- The defect floor is closer than the plan floor in every row; **plans are the binding constraint** because most leads are one-defect plans.
- Under subject-cluster counting `IDR-H-001` and `DISC-001` are one cluster, so #618's S4 (+1 / +2) replaces S3 in the cluster columns. Scenarios F and G are the conservative bounds: 8 / 6 and 11 / 10 short by artifact (9 / 7 for F by cluster). These are the ones to plan on until rulings arrive.
- Reaching 15/15 would also require the corpus owner to accept bot-surfaced, model-verified evidence as the discovery link for a majority of the corpus, which is a
  governance judgement this report does not make.

## 10. Unresolved evidence gaps

- No human review evidence exists anywhere in the repository history.
- No exported reviewer-visible tree has been built for any pin; exclusion of later fixes and of answer-key material (the PR threads, `candidates.json`, these reports) is unproven.
- `GH-004` (c, d), `GH2-001` second finding and `GH2-003`–`005` remain triaged only.
- Corrective-change searches for `GH-009`–`GH-014` (other than `GH-013`) were done by phrase search on `origin/main`, not by full history review.
- `GH-013` was reproduced on one Git version here; behaviour is long-standing but untested on the experiment's Git.

## 11. Recommended follow-up investigations (not authorized)

1. Corpus-owner rulings, in this order: **R-B**, **R-C-packet** (covers `GH-003`, `GH-004`, `GH-009`–`GH-014`), counting unit (artifact vs cluster), then `R-C` for skills and guides. They gate every number above.
2. Have an adjudicator, not the discovering model, repeat the §5.2 checks at the named pins (they take minutes each; `GH-013` needs a Git install).
3. Build a candidate reviewer-visible tree for `GH-013` and one uncorrected lead, to test whether the pin tree can exclude the thread, later fixes and these reports.
4. If more candidates are wanted, the remaining un-triaged surface is the 164 code-path threads that touch design-bearing files outside `docs/` (prompts, templates, config) and `GH2-003`–`005`.
5. Recognise the ceiling: the corpus owner is the sole human, and every discovery link is a bot. If R-B is refused, the historical set stays near 3–7 plans and the floor needs a different source (external or additional human-reviewed material) or a protocol decision; that is out of scope here.

## 12. Non-authorization statement

This report authorizes nothing. It does not: promote any candidate; rule on `DISC-001` or `IDR-H-001`; treat a gap record, approval packet, decision packet or skill as eligible; change any
eligibility criterion, decision, threshold, or canonical disposition; modify `candidates.json`, `DECISION-RECORD-003`, the registered protocol, or any Decision 7 file; record
operator approval; construct a seeded or control case; designate an adjudicator, seeder, or case checker; access sealed answer-key material; adjudicate; freeze the corpus; run model
inference for the experiment; or execute the experiment. Decision 6 remains pending. The corpus remains `NOT FROZEN`, the experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.
