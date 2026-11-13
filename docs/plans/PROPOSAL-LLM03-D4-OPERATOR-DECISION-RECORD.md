# D4 operator decision record — proposed (OWASP gap 9, LLM03)

**Status: APPROVED, IMPLEMENTED, and MERGED (report-only).** The operator approved §7 on
2026-11-13, the decisions were recorded in local decision memory, and the change merged as
`16c71ac8` (PR #482). The
proposal text below is preserved as written; the approval date is recorded here rather than
rewritten into the earlier sections, so no prior evidence implies it was approved sooner.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-13** — D4-1 A, D4-2 A, D4-3 B, D4-4 C, D4-5 R2, D4-6 omit, V-4 V-A, plus the stale-entry retirement and the memory record |
| Decision memory | **RECORDED locally** 2026-11-13, scope `architecture`, source `operator:d4-2026-11-13`, record id `a9f010095c7a5fd269614f6df3ba15d4040f1ae843a5a70cf469f7b9f9273e84`; machine-local, not in git |
| Committed implementation | **`8acdc6b8`** — adds 44 entries, retires 16, leaves configuration untouched |
| Merged implementation | **MERGED as `16c71ac8`** (PR #482) |
| Residual risk | **NOT ACCEPTED** — R2; `harness.go:926` stays unbaselined and blocking |
| Required-mode enforcement | **DEFERRED** — Option A; `security.mode` stays `report-only` |

**Verified at:** `origin/main` `0d8bac72` (2026-11-13), darwin arm64, Go 1.27.1, pinned
Gitleaks 8.30.1, Semgrep 1.183.0, OSV-Scanner 2.7.0.

**Source of the proposal:** [PROPOSAL-LLM03-D4-FINAL-DECISION-PACKAGE.md](PROPOSAL-LLM03-D4-FINAL-DECISION-PACKAGE.md).

## 1. Verification performed

| Check | Result |
| --- | --- |
| Full-workspace scan | 101 findings, **40 suppressed**, 61 unsuppressed, **45 blocking** |
| Adapters | gitleaks 8.30.1, semgrep 1.183.0, osv-scanner 2.7.0 — all `COMPLETED` / `COMPLETE` |
| Matrix reconciliation | 45/45 rows match the scan: fingerprint, path:line, rule, decision, expiry |
| Group counts | A 27 · B 5 · C 12 · D 1 · E 16 |
| Group overlap | A, B, C are pairwise disjoint; **D is not in A+B+C** |
| Overlap with existing baseline | **0** — none of the 44 is already baselined |
| Existing baseline | 56 entries: 40 matched, **16 stale** |
| `security.mode` | `report-only`; `required_categories` absent |
| Decision memory | 32 records, **0 applicable**, retrieval disabled, store excluded from git |

Full set digests, computed from that scan (SHA-256 over sorted, newline-joined fingerprints):

| Group | Count | Full digest |
| --- | --- | --- |
| A — Gitleaks fixtures | 27 | `4596c8ccc60f4d1b58d8ce3e9195d1455089dd320fdb4b5fe00ebdbf9fab1ef9` |
| B — test-only Semgrep HIGH | 5 | `2e5f765470497c31239a3203c5a4d40023e76a7f602ed847d21479727bec9b03` |
| C — production Semgrep | 12 | `b61b105f2838651008a731a2e89ef3902e76b6ea1c579fc245ec069776d094bc` |
| D — residual | 1 | `8a665937345d8e191e18691e55e4e4a02c6df9d3a5962ea0402d9faf1113b8ee` |
| E — non-blocking MEDIUM | 16 | `55268654137e2e2d65c7c06ecc101990f5157b07970fcf08543ecf6fb160fdce` |
| A+B+C | 44 | `d24bd19fef574619cf6f2fc392ef6acd0b2dde2df92ba53c7bfa76fe60224c31` |

**Expiries** (from the matrix, one per row): A and B — **2027-06-30**; C — **2027-03-31**;
D — `OPEN` (no entry proposed under R2); E — 2027-06-30 if ever approved.

**Composition of group A:** 21 findings in test files, 6 in documents
(`PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md` ×2 fixture-table rows and
`llm02-seven-format-campaign.json` ×4 planted canaries). Those 6 are deliberate fixture
documentation and campaign evidence, which is why baselining rather than redaction is
proposed for A — unlike the five decision-document fixtures that were redacted in PR #480.

## 2. Proposed decisions

| ID | Proposed | Exact scope | Effect if approved |
| --- | --- | --- | --- |
| **D4-1** | **Option A** — retain report-only | no configuration change | delivery stays unblocked; the scan still records `FAIL` while any blocking finding remains |
| **D4-2** | **Approve Group A** | 27 Gitleaks fixtures, digest `4596c8cc…`, expiry 2027-06-30 | those 27 stop counting as blocking |
| **D4-3** | **Approve Group B** | 5 test-only Semgrep HIGH, digest `2e5f7654…`, expiry 2027-06-30 | those 5 stop counting as blocking |
| **D4-4** | **Approve Group C** | 12 production Semgrep, digest `b61b105f…`, expiry **2027-03-31** | those 12 stop counting as blocking; the earlier date applies |
| **D4-5** | **R2** — defer acceptance | none | the residual stays unaccepted and unbaselined; Group D gets **no** entry |
| **D4-6** | **Omit Group E** | none | the 16 MEDIUM findings stay visible and non-blocking |
| **V-4** | **V-A** — keep current Semgrep handling | none | the lockfile stays authoritative; the config version stays descriptive |

## 3. Independence: A/B/C is not D, and is not Option C

Three checks, all against the real evaluator and configuration:

1. **Set identity.** `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771`
   is not a member of A, B, or C: `d24bd19f…` (A+B+C) covers 44 fingerprints while D's digest
   `8a665937…` covers 1. Approving the 44 cannot suppress the residual.
2. **Projection.** Applying exactly A+B+C to the verified findings leaves **1 blocking
   finding** — the residual (`internal/toolharness/harness.go:926`) — and the verdict stays
   `FAIL` in both report-only and required policy. D is untouched by design.
3. **No enforcement change.** `security.mode` stays `report-only`, `required_categories` stays
   empty, and required-mode readiness stays `false`. Approving baseline entries is a
   suppression change, not an enforcement change; Option C needs a separate, explicit
   configuration decision.

**Therefore:** these approvals cannot be read as accepting the residual, and cannot enable the
gate. A future D4-1 = C decision remains available and still requires its own authorization.

## 4. Resulting scan shape (predicted before approval, measured after)

| Measure | Now | After A+B+C |
| --- | --- | --- |
| Findings | 101 | 101 |
| Suppressed | 40 | 84 |
| Unsuppressed | 61 | 17 |
| Blocking | 45 | **1** (the residual) |
| Verdict (report-only) | `FAIL` | `FAIL` |

The verdict deliberately stays `FAIL`: D is open under R2. On expiry the corresponding
findings return as blocking — C first, on **2027-03-31**, then A and B on 2027-06-30. Because
C expires first, the 12 production entries must be re-reviewed by then.

**Measured after implementation:** 101 findings, 84 suppressed, 17 unsuppressed, **1
blocking**, verdict `FAIL`, 0 drift warnings — the predicted shape exactly.

## 5. The 16 stale baseline entries — separable, and safe to retire

They are the 13 sites from the 2026-11-03 bulk triage plus three 2026-11-08 individual
reviews (`internal/cli/outcome.go`, `.../julia/runner.go`, `.../dqharness/replay_exec.go`).
All expire 2027-06-30; none matches a current finding, so they suppress nothing. They are why
the advisory drift warning reports 16 entries on every complete workspace scan.

**Can they be retired without losing historical evidence? Yes, on three grounds:**

1. **Git history retains them.** `.agent-sdlc/security-baseline.json` is tracked; the exact
   rows, reasons, owners and expiries stay retrievable from the commits that added them.
2. **The matrix supersedes them.** Matrix §2 records a richer, per-site reason for all 13
   production sites, and the three specific 2026-11-08 reasons are narrower than the §2
   entries (policy gate, scrubbed environment, fail-closed backend). The triage and
   exec-site review reports keep the review narrative too.
3. **The bulk reason is partly wrong.** It says "argv is a constant … no shell", which is
   false for the six `sh -c` sites. Retiring it removes an inaccurate statement rather than
   preserving one.

**But retirement is a baseline change not covered by D4-2/D4-3/D4-4.** It needs its own
authorization line and should land in the same commit as the additions so the review sees one
diff: **56 − 16 + 44 = 84 entries**, all with 2027 expiries.

## 6. Decision memory — verified, not written

| Fact | Value |
| --- | --- |
| Store | `.agent-sdlc/context/decisions.json` (`decisionmemory/1`, limit 128) |
| Records | 32; identities split `aaff5cba…` (26), `b14deaa3…` (4), two single records |
| Applicable now | **0** — "32 stale decision(s) from another repository state" |
| Retrieval | disabled (`enabled=false`) |
| Version control | excluded by `.git/info/exclude`; machine-local |
| Existing D4 / isolation record | **none** |

Recording is optional and local. It is **not** required for a decision to take effect, and
this document is the durable, reviewable record. If you want the memory entry, it needs its
own authorization line, because it asserts an operator attribution.

## 7. Operator authorization — approved 2026-11-13

The operator approved the block below on 2026-11-13, including both optional additions
(stale-entry retirement and the decision-memory record). The text is preserved as sent; it
authorized the decisions in §2 and nothing further.

> I approve the following D4 decisions for OWASP LLM03 gap 9, verified at `main` `0d8bac72`:
> **D4-1 = A** (retain report-only enforcement); **D4-2 = approve Group A**, 27 Gitleaks
> fixtures, digest `4596c8ccc60f4d1b58d8ce3e9195d1455089dd320fdb4b5fe00ebdbf9fab1ef9`,
> expiry 2027-06-30; **D4-3 = approve Group B**, 5 test-only Semgrep HIGH, digest
> `2e5f765470497c31239a3203c5a4d40023e76a7f602ed847d21479727bec9b03`, expiry 2027-06-30;
> **D4-4 = approve Group C**, 12 production Semgrep, digest
> `b61b105f2838651008a731a2e89ef3902e76b6ea1c579fc245ec069776d094bc`, expiry 2027-03-31;
> **D4-5 = R2** (defer acceptance of the acknowledged-unsandboxed-execution residual);
> **D4-6 = omit Group E**; **V-4 = V-A** (retain the current Semgrep version handling).
>
> This authorizes adding exactly those 44 fingerprint entries with those expiries, and
> nothing else. It does not enable required mode or Option C, does not add a Group D entry,
> does not accept the residual, and does not change scanner versions, severities, or required
> categories.

Optional additions — each must be stated explicitly:

> **Stale retirement:** I also authorize removing the 16 stale baseline entries (the
> 2026-11-03/08 production-site rows that no longer match), in the same change, with their
> history preserved in git and superseded by matrix §2.

> **Decision memory:** I authorize recording the seven decisions and the R4 A+B
> implementation in decision memory with `--scope architecture` and source
> `operator:d4-<date>`, recording authorization and implementation only and no residual
> acceptance.

## 8. What happened after authorization (completed 2026-11-13)

1. Add the 44 entries to `.agent-sdlc/security-baseline.json` with the §2 reasons, owner, and
   the stated expiries.
2. If separately authorized, remove the 16 stale entries in the same commit.
3. No configuration change: `security.mode` stays `report-only`, `required_categories` stays
   empty.
4. Re-run `sop security scan --report-only` and record the expected shape from §4: 84
   suppressed, 1 blocking (the residual), verdict `FAIL`.
5. If separately authorized, write the decision-memory entries.

All five were completed on 2026-11-13; the record is in §9.

## 9. Implementation record

| Item | Result |
| --- | --- |
| Baseline entries | **84** = 40 retained + 44 added − 16 retired |
| Groups added | A 27 (`2027-06-30`), B 5 (`2027-06-30`), C 12 (`2027-03-31`) |
| Stale entries retired | 16; fingerprints and rationale preserved in git commit `1064b80f` and matrix §2 |
| Configuration | unchanged — `security.mode: report-only`, `required_categories` empty |
| Full-workspace scan | 101 findings, 84 suppressed, 17 unsuppressed, **1 blocking**, `FAIL`, 0 drift warnings — re-verified on merged `main` `16c71ac8` |
| Sole blocker | `internal/toolharness/harness.go:926`, `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771` |
| Decision memory | recorded locally 2026-11-13 (id `a9f01009…`); 32 earlier records untouched |
| Regression tests | `internal/security/baseline_repo_test.go` — approved-set digests, retirement, Group D/E guards, new-finding blocking, expiry return, scanner-failure fail-closed |

## What this document does not do

- Records no approval and applies no baseline change.
- Enables no required mode, adds no Group D entry, and accepts no residual.
- Changes no severity, `fail_on`, scanner version, or required category.
- Writes no decision memory and modifies no local SOP state.
- Alters no historical evidence: the stale entries and their rationale stay in git history and
  in matrix §2.
