---
name: sop-publish
description: |-
  Complete an authorized merge and publish of SOP-managed work, including safe
  cleanup. Use for: "/sop-publish", "finish the merge", "publish this", "clean up
  after publishing", "close out the merged feature" once a merge and publish are
  already authorized. It watches an open pull request's required checks, hands
  failures caused by the change to /sop-implement, merges it once green, verifies
  the published target contains the intended
  commits, historicalizes eligible plans through SOP, preserves lifecycle
  archives, and removes only fully-merged worktrees and branches. SOP owns the
  lifecycle; this skill orchestrates ordinary git cleanup and delegates
  historicalization.
disable-model-invocation: true
---

# SOP Merge & Publish Completion

Canonical skill name: `sop-publish`. Installed command: `/sop-publish`.

`/sop-publish` runs the completion half of a merge and publish. After an operator
has **authorized** the merge and publish of a feature, it watches the pull
request's checks, hands failures caused by the change to `/sop-implement`, merges
once green, verifies the
publication,
closes eligible plans through SOP, preserves evidence, and performs only safe
cleanup. It is a thin client of the `sop` CLI plus ordinary git: it never
reproduces SOP policy, never edits SOP state, and never rewrites history.

```text
/sop-publish
     |
     v
  CONFIRM CHECKOUT -> WATCH PR CHECKS / MERGE (if open) -> VERIFY PUBLICATION ->
  INSPECT WORKTREE -> HISTORICALIZE (governed) -> PRESERVE EVIDENCE ->
  REMOVE MERGED WORKTREES -> DELETE MERGED BRANCHES -> REPORT
```

Cleanup proceeds **without a further confirmation** for routine, safe steps
*inside the authorized merge/publish scope*. Stop and report only when evidence
shows unresolved work, potential loss, failed publication, or an actual policy
boundary.

Safety invariant: **authorization to merge and publish is the scope, not a
licence to destroy.** It never means "force-remove anything dirty", "delete a
remote branch", "mark a plan complete", or "rewrite history". SOP remains the
sole lifecycle authority.

## PREFLIGHT

Check that `sop` is on PATH; if missing, STOP and report the installation
prerequisite. Do not install or rebuild SOP, or implement the feature itself;
a failing check caused by the change is handed off to `/sop-implement` (section 2).
Confirm the repository root and that this is an SOP-managed repository (an
`.agent-sdlc/` directory exists). The PR-watch step also uses the GitHub CLI; if
`gh` is missing when it is needed, report that prerequisite rather than guessing
check status. Collect SOP state read-only:

```bash
sop status
sop continue --check --json
```

## 1. CONFIRM THE CHECKOUT

Find out where you are and where the feature lives before anything else. This step
only reads; it never switches or creates a branch:

```text
git rev-parse --show-toplevel
git branch --show-current                             # empty means detached HEAD
git symbolic-ref --quiet --short refs/remotes/origin/HEAD   # the default branch, e.g. origin/main
git worktree list                                     # which checkout holds which branch
gh pr view <branch-or-number> --json number,url,state,headRefName,baseRefName
```

- **The PR is already merged.** Being on the default branch is the normal state
  after a merge; continue to section 3. Do not create a branch: this skill only
  verifies and cleans up from here, and a new branch would just be one more thing
  section 8 has to delete.
- **The PR is still open.** Note its head branch (`headRefName`) and the worktree
  that has it checked out, if any. A fix handed off in section 2 must run there,
  never on the default branch or a detached HEAD, where it would bypass review or
  go nowhere, so the hand-off names that branch and path. Do not switch this
  checkout to it: `git switch` carries uncommitted changes along, and they belong
  to whatever was being done here.

## 2. WATCH THE PR CHECKS, FIX FAILURES, AND MERGE

When the authorized publish produced a pull request that is still open, watch its
checks, triage failures, and merge it once it is green, so the completion
steps that follow have something to verify. If the PR is already merged, skip this
section: a failing check on a merged PR is reported, never "fixed" by rewriting
history.

Only the required checks gate the merge, so ask for those explicitly:

```text
gh pr view <number> --json number,url,state,isDraft,mergeStateStatus,headRefName
gh pr checks <number> --required --json name,state,bucket,link
```

Poll at a fixed interval for a bounded window (for example 20 polls at 30
seconds); a check still pending when the window ends is reported, not assumed to
pass. Run the same command without `--required` once at the end, so failed
optional checks are reported too.

- **All required checks pass**, go to "Merge" below.
- **A required check fails**, work out whose failure it is before changing code.
  Each check's `link` ends in `/runs/<run-id>/job/<job-id>`:

```text
gh run view --job <job-id> --log-failed        # read the failing job log
gh run list --branch <default-branch> --limit 5 # is the default branch red too?
gh run rerun <run-id> --failed                 # once, to rule out a flaky test
```

  - **It also fails on the default branch, or on other open PRs, for the same
    reason**: the cause is outside this change (a runner image update, a broken
    dependency, a flaky test already on the default branch). STOP and report it.
    Patching it inside this PR would hide it from every other branch.
  - **It passes on the re-run**: report the flaky test by name and continue. Do
    not count it as a fix round.
  - **It fails again and only here**: the change itself is at fault. STOP and
    hand off. This skill never changes the repository: a fix is implementation
    work, and it goes through SOP's governed lifecycle via `/sop-implement`, run
    from the head branch's checkout. Put the hand-off in the report as the next
    action, with the check name, the run link, and a short log excerpt, and ask
    for a root-cause fix: not a behavior change made merely to satisfy an
    assertion, not a weakened test, and nothing outside the authorized change
    set. Once the fix is pushed, `/sop-publish` is run again and resumes here.
- **STOP and report without suggesting a code fix** when the failure is
  infrastructure or environment-specific (runner outage, rate limit, missing
  secret, expired credential), when the fix needs a product or behavior decision,
  or when the log is unavailable.
- If the same check is still failing after three fix rounds on this PR, say so in
  the report and recommend stopping to rethink rather than another round.

**Merge.** Merging is inside the authorization this skill runs under, but the
authorization covers the change as it was reviewed. So:

- If the PR has no commits added by a fix round since the merge was authorized,
  or those commits touch only tests or CI configuration, merge with the
  repository's usual method and continue: `gh pr merge <number> --merge` (or
  `--squash` / `--rebase` to match history).
- If a fix round changed product code, STOP before merging and report it as ready
  to merge with fixes needing review, so a person looks at code the original
  authorization did not cover.
- Never merge with a failing required check, and never pass `--admin` to get
  around a protection rule.

## 3. VERIFY PUBLICATION

Confirm the published target contains the intended commits and report local/remote
synchronization. Never assume a merge reached the remote.

```text
git fetch <remote> --prune
git rev-parse <published-target>          # e.g. origin/main
git rev-parse <feature-branch>
git merge-base --is-ancestor <feature-head> <published-target>
git rev-list --left-right --count <published-target>...<feature-branch>
```

If the intended commits are **not** contained in the published target, STOP:
publication failed. Do not clean up, remove worktrees, or delete branches until
publication is confirmed. Report local/remote divergence.

## 4. INSPECT THE FEATURE WORKTREE BEFORE REMOVAL

For every candidate worktree, enumerate tracked, untracked, and gitignored SOP
state *before* removing anything, so nothing unexplained is lost:

```text
git -C <worktree> rev-parse HEAD
git -C <worktree> status --short
git -C <worktree> status -sb
git -C <worktree> status --short --ignored     # includes gitignored SOP state
git -C <worktree> diff --check
```

Classify each item: tracked work, untracked files, and gitignored SOP state
(`.agent-sdlc/runs/`, `.agent-sdlc/archive/`, plan files, reports, traces). A
worktree with unexplained tracked or untracked changes, or with SOP state not
preserved elsewhere, is **not** removable — preserve it and report.

## 5. CLOSE ELIGIBLE PLANS THROUGH SOP

Close plans only through SOP's governed historicalization, and only when SOP
reports the plan is ready. Never infer `COMPLETE` from a merge.

```bash
sop plan historicalize --check
sop plan historicalize
```

- `--check` first. Historicalize only when readiness passes (every task
  satisfied, no pending approval, verification evidence exists). SOP's
  `ALREADY_HISTORICALIZED` is a no-op, not an error.
- **Preserve unfinished plans.** A merge does not complete a plan; do not
  historicalize a plan with unresolved tasks.
- **Never mark a plan `SUPERSEDED` automatically.** Disposal is an explicit
  operator decision, never a cleanup default.
- Never edit the state database, `plan.json`, `plan.meta.json`, or the archive to
  force an outcome.

## 6. PRESERVE EVIDENCE OUTSIDE THE WORKTREE

Before removing any worktree, copy the lifecycle archive and other required
evidence it holds to a durable location outside every worktree being removed, and
verify the copies (checksums or a diff). If preserved copies conflict with the
worktree's contents, STOP and report the conflict; do not overwrite either side.
Lifecycle archives and required evidence are protected: never delete or rewrite
them to obtain a clean tree.

## 7. REMOVE ONLY FULLY-MERGED CLEAN WORKTREES

Remove a worktree only when it is clean and its branch commits are fully contained
in the published target:

```text
git -C <worktree> status --short          # must be empty (tracked and untracked)
git merge-base --is-ancestor <worktree-head> <published-target>
git worktree remove <path>                # never --force
```

A Git-clean worktree is not proof that it holds no evidence: `.agent-sdlc` is
ignored, so `git status --short` stays empty while state, run, and approval
records are still present.

The Go `RemoveWorktree` path enforces this for SOP-managed removals: it refuses
to remove a worktree whose ignored `.agent-sdlc` lifecycle evidence has no
byte-identical archive under `.agent-sdlc/archive/worktree-evidence/<plan-id>/`.
A direct `git worktree remove` bypasses that guard and must perform the evidence
copy and verification above manually.

Never use force removal. A dirty worktree, an unmerged branch, or an ambiguous
result is retained and reported with its reason.

## 8. DELETE MERGED LOCAL BRANCHES

Delete a local branch only with the safe form, which refuses an unmerged branch:

```text
git branch -d <branch>
```

If deletion is refused because a stale upstream no longer matches, first verify
the branch's commits are contained in the published target, then unset that stale
upstream and retry the safe deletion:

```text
git rev-list --left-right --count <published-target>...<branch>
git branch --unset-upstream <branch>
git branch -d <branch>
```

Never force-delete a branch. If ancestry against the published target cannot be
established, retain the branch and report.

## 9. REMOTE BRANCHES AND UNRELATED WORK

Keep remote branches unless the operator explicitly authorizes their deletion;
never delete a remote branch as a cleanup default. Leave unrelated worktrees and
branches untouched — only the worktrees and branches in the authorized
merge/publish scope are eligible.

## 10. REPORT

Report: the merge/publish result; the PR check result, any flaky tests seen, and
any fix commits on the PR since authorization (or that the PR was already
merged), marking ones that changed product code as needing review; any fix
handed off to `/sop-implement`; any failure traced to the default branch
rather than this change; the plan disposition (historicalized,
preserved, or already closed); the retained-evidence location; the worktrees and
local branches removed; the remote branches kept; and anything retained with its
reason. End with exactly one next action.

## PROTECTED EVIDENCE

Never delete or rewrite, merely to obtain a clean tree:

```text
.agent-sdlc/runs/
.agent-sdlc/archive/
task reports
classification records
traces
lifecycle records
not-required evidence
```

Respect `.gitignore` boundaries: a gitignored path is not junk, and an unclassified
path is never removed.

## What the skill must not do

- Do not infer `COMPLETE` from a merge, and do not edit lifecycle state to force an
  outcome.
- Do not automatically mark a plan `SUPERSEDED`.
- Do not hand off a fix to run on the default branch or a detached HEAD; it
  belongs on the PR's head branch. Do not create new branches during cleanup.
- Do not bypass or disable required checks, skip CI, weaken or delete tests, or
  edit CI configuration to obtain a green build.
- Do not patch, inside this PR, a failure that also happens on the default branch;
  report it instead.
- Do not merge a fix that changed product code without review, and never merge
  with `--admin`.
- Do not force-push or rewrite published history while fixing a failing check.
- Do not fix a failing check yourself: no direct edit, commit, or push. Hand it
  off to `/sop-implement` and stop.
- Do not force-remove a worktree, force-delete a branch, or rewrite history.
- Do not delete a remote branch without explicit operator authorization.
- Do not delete lifecycle archives, required evidence, or an unclassified file.
- Do not touch unrelated worktrees or branches.
- Do not reproduce SOP policy, pick a model or provider, or call a provider
  directly; SOP resolves those.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see
  the project README / `./install.sh`). Never perform the cleanup in SOP's place.
