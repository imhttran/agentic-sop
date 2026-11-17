# Decision packet — Semgrep `PARTIAL` coverage from git-ignored artifacts

**Status:** proposal, read-only. **No scanner behaviour, exclusion, ignored file,
worktree, or configuration is changed by this document.**
**Base:** `origin/main` @ `21943cd0` (2026-11-17), PR #519 merged.
**Required categories:** `sast`, `secrets`, `dependencies`; dependency readiness is
`true` on the primary checkout.

## 1. Symptom

| Checkout | Verdict | Coverage | SAST |
| --- | --- | --- | --- |
| Primary checkout (long-lived) | `INDETERMINATE` | `PARTIAL` | semgrep `PARTIAL` |
| Clean checkout at the same revision | `PASS` | `COMPLETE` | semgrep `COMPLETE` |

Semgrep's own summary in the primary checkout:

```
Files larger than 1.0 MB: 3
Files matching .semgrepignore patterns: 40
```

`.semgrepignore` in this repository is one line: `:include .gitignore`. Semgrep
also filters by git (its help text: "any filtering done by git (or other SCM)"), so
git-ignored files are skipped and counted under that class.

## 2. How SOP classifies it

`internal/security/semgrep_coverage.go` parses Semgrep's **human summary** (Semgrep
1.183.0's `--json` output exposes only `paths.scanned`; it does **not** report
`paths.skipped`, verified on this host). Skips are classified as:

| Class | Treatment |
| --- | --- |
| `declared-exclude` (a configured `--exclude`) | not a gap |
| `semgrepignore` | **gap** → coverage `PARTIAL` |
| `large-file` | **gap** → coverage `PARTIAL` |
| `unrecognized` | **gap** — an unknown limitation never yields `COMPLETE` |

The classifier is deliberately summary-level and conservative: it never uses a skip
reason to conclude that a skip is acceptable.

## 3. What is actually skipped (paths only; no file contents read)

Derived from git and from file sizes, without opening any skipped file:

| Class | Count in scope | Tracked? | Ignored? |
| --- | --- | --- | --- |
| `semgrepignore` matches | **67** git-ignored files, of which Semgrep targets 40 | **0** | all |
| `large-file` (≥ 1 MB) | 4: `./sop` (17.6 MB), `.run/spike/sop-planner` (29.5 MB), `.run/spike/sop-planner-main` (28.9 MB), `.run/tunnel/logs/issue-90-candidate.jsonl` (1.75 MB) | **0** | all |

Ignored files inside the declared scope, by prefix: `.run` 31, `.playwright-mcp`
29, plus `sop`, `.env`, `.env.bak`, `.DS_Store`, `CLAUDE.local.md`,
`docs/plans/*.local`, `.claude/scheduled_tasks.lock`.

**No tracked file is skipped.** Verified by checking the scanner's own
`paths.scanned` set: `internal/toolharness/harness.go`, `cmd/sop/main.go` and
`internal/security/policy.go` are all present, and zero of the ignored/skipped
paths is tracked (`git ls-files --error-unmatch` fails for every one).

## 4. Is the `PARTIAL` accounted for correctly?

**The skips are real, but they are not a coverage gap in repository source.**
Every skipped path is git-ignored. The repository already treats "ignored" as
out-of-scope for dependency evidence: `internal/git.Adapter.SnapshotFiles` defines
the source set as "every tracked file plus every untracked file that is not
ignored… Ignored generated output is deliberately absent", and the Gitleaks adapter
scans exactly that. Semgrep's accounting instead treats a git-ignored skip as a
gap, so a checkout that happens to hold ignored build/runtime scratch cannot reach
`COMPLETE` — while the same revision in a clean checkout does.

So: **the scanner is right that files were skipped; the accounting is inconsistent
with the scope definition the other adapters use.** It is not a false negative on
source — no tracked or untracked-non-ignored file was skipped — but it is an
over-count of out-of-scope content as a coverage gap.

## 5. Option A — coverage-accounting correction (recommended)

Align Semgrep's target scope with the existing `SnapshotFiles` definition: a skip of
a **git-ignored** path is out-of-scope content, not a gap.

Sound implementation constraint: the correction must be path-derived, not
count-derived. Semgrep's summary gives counts only, so a rule like "if skipped ≤
ignored count, accept" would be unsound — a tracked file could be skipped while an
ignored file was not. Two sound shapes:

- **A1 (preferred):** enumerate ignored paths (existing `internal/git` helpers or
  `git ls-files -o -i --exclude-standard --directory`) and pass them to Semgrep as
  `--exclude`, so they are reported as `declared-exclude`. Bounded by batching the
  list; `.run`, `.playwright-mcp`, `sop` collapse to a handful of patterns.
- **A2:** scan an explicit file list (the snapshot set) instead of a directory
  walk. More invasive to the invocation and its evidence.

**Fail-closed preserved:** tracked files and untracked non-ignored files stay in
scope; any skip of those still yields `PARTIAL`. If the ignored set cannot be
determined (no git, or a git error), behaviour stays as today — `PARTIAL`. A path
that git tracks is never excluded, even if an ignore rule matches it.

**Cost:** one git call per scan (Gitleaks already does this), a bounded `--exclude`
list, and tests. **Residual:** a file that is git-ignored is not scanned, which is
already true of the other adapters; a repository that ignores real source would
have that source unreported — the same limit Gitleaks already has.

## 6. Option B — narrow static exclusions

Add `.run`, `.playwright-mcp`, `/sop`, `.env`, `.DS_Store`, `CLAUDE.local.md` to
`security.exclude`. **Rejected as the primary fix:** it hard-codes one machine's
scratch into repository policy, grows every time a new artifact type appears, and
would exclude `.env`-shaped files even in a checkout where they are deliberate. It
also cannot distinguish "ignored" from "important" — that judgement already exists
in git.

## 7. Option C — preservation-first artifact relocation

Move the scratch out of the scan root (or delete the rebuildable `sop` binary):
`./sop`, `.run/spike/*`, `.run/tunnel/logs/*`, `.playwright-mcp/*`, plus the small
stragglers. **No policy change**, immediate, reversible. **Rejected as the only
fix:** it is a recurring chore (Playwright logs and built binaries reappear), and it
leaves every other long-lived checkout exposed to the same drift.

**Recommendation:** implement **A1** for durability, and use **C** as the immediate
unblock if the checkout is needed before A1 lands. Do not do B.

## 8. Regression tests for A1

| Test | Asserts |
| --- | --- |
| `TestSemgrepScopeExcludesGitIgnoredPaths` | an ignored path inside the walk is passed as a declared exclusion, so a skip of it is not a gap |
| `TestSemgrepScopeKeepsUntrackedNonIgnored` | a new (untracked, not ignored) source file is still in scope and a skip of it is a gap |
| `TestSemgrepScopeKeepsTrackedEvenIfIgnored` | a tracked file matching an ignore rule is never excluded |
| `TestSemgrepCoverageFailsClosedWithoutGit` | when the ignored set cannot be determined, coverage stays `PARTIAL`, never `COMPLETE` |
| `TestSemgrepCoverageCountsTrackedSkipsAsGap` | a skipped tracked file still yields `PARTIAL` (the anti-regression for this change) |
| `TestSemgrepCoverageLargeTrackedFileIsStillAGap` | a tracked file over the size limit remains a gap |

## 9. Acceptance criteria

- The primary checkout reports `PASS` / `COMPLETE` **without** deleting user data or
  editing `.gitignore`.
- A tracked file that Semgrep skips (any class) still yields `PARTIAL`.
- An untracked, non-ignored file that Semgrep skips still yields `PARTIAL`.
- With git unavailable or erroring, coverage is `PARTIAL`.
- No new static exclusion; `security.exclude`, the 82-entry baseline, required
  categories, OSV policy and W6/W7/Stage 1/Stage 2 controls unchanged.
- `stage2-gate-e2e`, `sandbox-linux` and the security integration job stay green.

## 10. Exact operator decision

> I authorize Option A1 from
> `docs/plans/PROPOSAL-SEMGREP-IGNORED-COVERAGE.md`: make the Semgrep adapter treat
> git-ignored paths inside the scan walk as declared out-of-scope content by passing
> them to Semgrep as exclusions derived from git, so a skip of ignored build or
> runtime scratch no longer degrades coverage. Tracked files and untracked
> non-ignored files stay in scope, any skip of those still yields `PARTIAL`, and an
> unavailable or failing git keeps the current fail-closed behaviour. This
> authorizes that adapter change and its tests only. It does not add or remove any
> static exclusion, does not change `security.exclude`, the baseline, required
> categories, the OSV policy, or any residual disposition.

To record it once decided:

```bash
sop memory add --scope architecture \
  --source "operator:semgrep-ignored-coverage-<date>" \
  --decision "<the authorization text above>" \
  --reason "Align Semgrep scope with the SnapshotFiles definition without adding static exclusions" \
  --evidence "docs/plans/PROPOSAL-SEMGREP-IGNORED-COVERAGE.md"
```

## 11. Immediate workaround while undecided

Every git-ignored path inside the declared scope has to leave the walk, not only the
large ones: the 40 `.semgrepignore` matches are the small stragglers. Relocate, do
not delete, preserving relative paths:

```bash
scratch=~/sop-scratch/$(date +%Y%m%d)
mkdir -p "$scratch"
git ls-files -o -i --exclude-standard \
  | grep -vE '^(\.git/|\.agent-sdlc/|\.worktrees/|\.claude/worktrees/|\.sop-evidence/|\.run/reference/|internal/security/testdata/|testdata/)' \
  | while read -r p; do
      mkdir -p "$scratch/$(dirname -- "$p")"
      mv -- "$p" "$scratch/$p"
    done
```

This is reversible (`mv` them back) and deletes nothing. Verify with
`sop security inspect` and `sop security scan`: the primary checkout should then
match the clean checkout. `.run/` directories left empty can be removed manually;
`./sop` is a rebuildable artifact. Until A1 lands, this is the only way to a green
gate in a checkout that has accumulated ignored scratch.
